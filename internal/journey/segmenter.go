package journey

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/deephanson94/compass/internal/transcript"
)

// Prompt is one human turn — the moments the journey changed direction because
// somebody asked it to.
type Prompt struct {
	Text string // first line, max 60 runes, "…" if cut
	At   time.Time
}

// Leg is a contiguous span of one class of work: the unit the trail draws.
type Leg struct {
	Class   Class
	Label   string    // heuristic, lowercase, ≤24 runes (see label)
	Start   time.Time // first vote's event
	End     time.Time // last vote's event
	Votes   int       // number of votes folded in
	Files   []string  // distinct file basenames touched, first-seen order, cap 5
	Current bool      // still open — this is HEAD (at most one, the last leg)

	// Waypoints are the leg's Lv2 detail rows: parsed test results, bug
	// signatures, commits. Oldest first, cap 8 (docs/dev/M2-CONTRACT.md).
	Waypoints []Waypoint
}

// Branch is a subagent lane: it forks off the leg that was open and merges back
// when its tool_result lands.
type Branch struct {
	ToolUseID string // the Agent tool_use id
	Label     string // Agent input "description" field, else "agent"
	Start     time.Time
	End       time.Time // zero until Done
	Done      bool      // its tool_result has been observed
	AfterLeg  int       // index into Legs of the leg open when the fork happened; -1 if none yet
	Report    string    // first non-empty line of the agent's result, ≤60 runes; "" until Done
}

// Task is one entry of the plan Claude keeps for itself, read from the
// TaskCreate and TaskUpdate calls in the transcript. This is the "adventure
// it's going to do": pending tasks are the ghost waypoints ahead of HEAD, and
// the in-progress one is what HEAD is called when nothing else names it.
//
// It used to be read from ~/.claude/todos/<session>.json. Claude Code stopped
// writing that file; the calls that replaced it are in the transcript, which
// is data the trail already reads, and a better source than a side file was —
// the plan and the journey come from one place and cannot disagree.
type Task struct {
	ID      string // "1", "2", … — assigned by Claude Code in TaskCreate's result
	Subject string // imperative: "Fix the token refresh"
	Active  string // present tense, for the spinner: "Fixing the token refresh"; may be ""
	Status  string // "pending", "in_progress", "completed", "deleted"
	Owner   string // an agent name, when a teammate claimed it
}

// Trail is the whole journey as the panel needs it: what was asked, what was
// done, what forked off along the way, and what is still to come.
type Trail struct {
	Prompts  []Prompt // every substantive user prompt, oldest first
	Legs     []Leg    // oldest first
	Branches []Branch // oldest first
	Tasks    []Task   // creation order; deleted ones stay, marked
}

// maxFiles caps how many distinct basenames a leg remembers. Five is already
// more than the trail can show; the rest would only cost memory on a session
// that edits hundreds of files.
const maxFiles = 5

// fileCount is one basename and how often the leg touched it — the label
// heuristic wants the most frequent one, so the count travels with the name.
type fileCount struct {
	name string
	n    int
}

// legState is a leg while it is still being built. It keeps two classes: the
// one it is displayed as (which a Fix upgrade rewrites) and the one its votes
// carry, so an upgraded Fix leg still absorbs the Build votes it is made of
// instead of splitting against itself.
type legState struct {
	class    Class
	voted    Class
	start    time.Time
	end      time.Time
	votes    int
	files    []fileCount
	keyword  string // first runner/subcommand word seen, for the label
	hadError bool   // an IsError tool_result landed while this leg was open

	// waypoints are the Lv2 detail rows results left here (M2 rule 1). They
	// never migrate: a waypoint belongs to the leg that was open when its
	// result came back, whatever the pressure gauge decides afterwards.
	waypoints []Waypoint
}

// Segmenter folds a session's events into legs. Feed it events in file order;
// it holds only the running leg plus the little pressure gauge rule 4 needs.
type Segmenter struct {
	prompts  []Prompt
	legs     []legState
	branches []Branch
	byBranch map[string]int // Agent tool_use id → index into branches
	open     bool           // the last leg is still accepting votes

	// Runner memory (M2 rule 2): the tool_use ids of Test and Ship votes, so
	// their results — and only theirs — get parsed as test runs and commits.
	// Entries are dropped when their result lands; the cap is the backstop for
	// calls whose results never do.
	runners     map[string]runner
	runnerOrder []string // insertion order, for eviction

	// The plan. A TaskCreate call names a task; its result, a beat later,
	// assigns the id every TaskUpdate refers to. byCreate bridges the two.
	tasks    []Task
	byTask   map[string]int // task id → index into tasks
	byCreate map[string]int // TaskCreate tool_use id → index into tasks

	// Pressure from differing weak votes (rule 4). The votes are buffered, not
	// folded: if the streak dies they flush into the open leg, and on the third
	// they migrate wholesale into the new leg they open — Start, Votes and Files
	// travel with them. Trail() folds the buffer into the open leg for display
	// so a streak-in-progress still reads as part of the journey.
	press []pended
}

// pended is one buffered pressure vote.
type pended struct {
	v  vote
	at time.Time
}

// NewSegmenter returns an empty segmenter: no legs, no opinions.
func NewSegmenter() *Segmenter { return &Segmenter{} }

// Observe feeds one event to the segmenter. Events must arrive in file order.
func (s *Segmenter) Observe(ev transcript.Event) {
	// A sidechain line is a subagent talking inside its own branch; the main
	// trail already represents it as a Branch and must not be voted on twice.
	if ev.IsSidechain {
		return
	}

	// Results arrive on user-type lines: they close branches and are the only
	// place failure is visible.
	for _, res := range ev.ToolResults {
		s.observeResult(res, ev.Timestamp)
		s.resolveTask(res)
	}

	// A background agent's verdict arrives the same way a person's words do —
	// as a user turn — but it is the harness relaying, not the person, so it
	// is read here and never becomes a prompt. It is also read from the
	// queue-operation line that enqueued it: in a real transcript a quarter of
	// the notifications never reached a user turn at all, because the session
	// died — on quota, say — before the queue drained. The join is by id, so
	// reading both is harmless where both exist.
	if ev.Type == transcript.EventUser || ev.Type == transcript.EventQueueOp {
		if n, ok := transcript.ParseTaskNotification(ev.Text); ok {
			s.observeNotification(n, ev.Timestamp)
		}
	}

	// Rule 2: a human prompt is a hard boundary, whatever was running. Pressure
	// that never reached three stays with the leg it interrupted.
	if substantivePrompt(ev) {
		s.prompts = append(s.prompts, Prompt{Text: clip(promptText(ev), 60), At: ev.Timestamp})
		s.flushPress()
		s.closeLeg()
	}

	for _, use := range ev.ToolUses {
		switch use.Name {
		case agentTool:
			s.fork(use, ev.Timestamp)
			continue
		case taskCreateTool:
			s.createTask(use)
			continue
		case taskUpdateTool:
			s.updateTask(use)
			continue
		}
		if v, ok := classifyUse(use); ok {
			s.applyVote(v, ev.Timestamp)
		}
	}
}

// Trail returns a snapshot the caller owns: slices are copied, so nothing a
// renderer does can reach back into the segmenter's state.
func (s *Segmenter) Trail() Trail {
	var tr Trail
	if len(s.prompts) > 0 {
		tr.Prompts = append(make([]Prompt, 0, len(s.prompts)), s.prompts...)
	}
	if len(s.branches) > 0 {
		tr.Branches = append(make([]Branch, 0, len(s.branches)), s.branches...)
	}
	if len(s.tasks) > 0 {
		tr.Tasks = append(make([]Task, 0, len(s.tasks)), s.tasks...)
	}
	if len(s.legs) > 0 {
		tr.Legs = make([]Leg, len(s.legs))
		for i := range s.legs {
			l := &s.legs[i]
			if s.open && i == len(s.legs)-1 && len(s.press) > 0 {
				// A pressure streak in progress still reads as part of the
				// open leg until it either settles or migrates.
				clone := *l
				clone.files = append([]fileCount(nil), l.files...)
				for _, p := range s.press {
					foldInto(&clone, p.v, p.at)
				}
				l = &clone
			}
			tr.Legs[i] = Leg{
				Class:     l.class,
				Label:     l.label(),
				Start:     l.start,
				End:       l.end,
				Votes:     l.votes,
				Files:     l.fileNames(),
				Waypoints: l.waypointsCopy(),
			}
		}
		if s.open {
			tr.Legs[len(tr.Legs)-1].Current = true
		}
	}
	return tr
}

// substantivePrompt reports whether the event is a human turn with something in
// it. Tool-result-only user lines, attachments and queue bookkeeping carry no
// direction and must not break a leg — and neither do harness envelopes
// (scheduled wakes, task notifications): they arrive as user turns but nobody
// asked anything, so they mark no ◉ and close no leg.
func substantivePrompt(ev transcript.Event) bool {
	if ev.Type != transcript.EventUser {
		return false
	}
	text := strings.TrimSpace(ev.Text)
	if text == "" {
		return false
	}
	return !ev.Machinery()
}

// observeResult merges a returning branch, records failure against the open
// leg — the signal behind the Fix upgrade (rule 5a) — and hangs whatever the
// result was worth on the trail (M2 rules 1 and 4–6).
func (s *Segmenter) observeResult(res transcript.ToolResult, at time.Time) {
	if i, ok := s.byBranch[res.ToolUseID]; ok {
		b := &s.branches[i]
		// A background agent's tool_result is only its launch acknowledgement:
		// the agent is still running, and its verdict comes later as a
		// task-notification (observeNotification). Treating the ack as the
		// result put "Spawned successfully. (This tool result is internal
		// metadata…" on the trail as the agent's finding, and drew the lane
		// merged while the agent was minutes from done.
		if !launchAck(res.Text) {
			b.Done = true
			b.End = at
			if line := firstNonEmptyLine(res.Text); line != "" {
				b.Report = clip(line, waypointText)
			}
		}
	}
	if res.IsError && s.open {
		cur := &s.legs[len(s.legs)-1]
		cur.hadError = true
		if cur.class == Build {
			// Retroactive, whole leg: work that hit an error was always a fix.
			cur.class = Fix
		}
	}
	s.attach(res, at)
}

// observeNotification merges a background agent's lane when the harness says it
// stopped. The notification names the Agent call by its tool-use id, so this
// is a join, not a guess; a notification for an agent this trail never forked
// is ignored.
func (s *Segmenter) observeNotification(n transcript.TaskNotification, at time.Time) {
	i, ok := s.byBranch[n.ToolUseID]
	if !ok {
		return
	}
	b := &s.branches[i]
	b.Done = true
	b.End = at
	// The agent's own last words are the report; the harness's one-line
	// summary is the fallback for an agent that said nothing.
	if line := firstNonEmptyLine(n.Result); line != "" {
		b.Report = clip(line, waypointText)
	} else if line := firstNonEmptyLine(n.Summary); line != "" {
		b.Report = clip(line, waypointText)
	}
}

// The task tools, by name. Their shapes are Claude Code's own and were read
// off real transcripts: TaskCreate takes subject/description/activeForm and
// gets its id back in the result; TaskUpdate names a taskId and the fields
// that change.
const (
	taskCreateTool = "TaskCreate"
	taskUpdateTool = "TaskUpdate"
)

// createTask opens a pending task from a TaskCreate call. It has no id yet —
// that arrives with the result — so the call's own id remembers which task
// to finish naming.
func (s *Segmenter) createTask(use transcript.ToolUse) {
	var in struct {
		Subject    string `json:"subject"`
		ActiveForm string `json:"activeForm"`
	}
	if err := json.Unmarshal(use.Input, &in); err != nil || strings.TrimSpace(in.Subject) == "" {
		return
	}
	if s.byCreate == nil {
		s.byCreate = make(map[string]int)
		s.byTask = make(map[string]int)
	}
	s.byCreate[use.ID] = len(s.tasks)
	s.tasks = append(s.tasks, Task{Subject: in.Subject, Active: in.ActiveForm, Status: "pending"})
}

// resolveTask gives a created task its id, from the result's structured
// account first — {"task":{"id":"1"}} — and from the text Claude Code writes
// beside it ("Task #1 created successfully: …") when a transcript predates
// the structure.
func (s *Segmenter) resolveTask(res transcript.ToolResult) {
	i, ok := s.byCreate[res.ToolUseID]
	if !ok {
		return
	}
	delete(s.byCreate, res.ToolUseID)
	id := ""
	if len(res.Meta) > 0 {
		var meta struct {
			Task struct {
				ID string `json:"id"`
			} `json:"task"`
		}
		if err := json.Unmarshal(res.Meta, &meta); err == nil {
			id = meta.Task.ID
		}
	}
	if id == "" {
		if m := taskCreated.FindStringSubmatch(res.Text); m != nil {
			id = m[1]
		}
	}
	if id == "" {
		return // a task nothing can refer to is still a pending ghost
	}
	s.tasks[i].ID = id
	s.byTask[id] = i
}

// taskCreated is the text form of TaskCreate's result.
var taskCreated = regexp.MustCompile(`^Task #(\S+) created`)

// updateTask applies a TaskUpdate call: whichever of status, subject, active
// form and owner it carries. An update for a task this trail never saw
// created — a teammate's, in another transcript — is ignored.
func (s *Segmenter) updateTask(use transcript.ToolUse) {
	var in struct {
		TaskID     string `json:"taskId"`
		Status     string `json:"status"`
		Subject    string `json:"subject"`
		ActiveForm string `json:"activeForm"`
		Owner      string `json:"owner"`
	}
	if err := json.Unmarshal(use.Input, &in); err != nil {
		return
	}
	i, ok := s.byTask[in.TaskID]
	if !ok {
		return
	}
	t := &s.tasks[i]
	if in.Status != "" {
		t.Status = in.Status
	}
	if in.Subject != "" {
		t.Subject = in.Subject
	}
	if in.ActiveForm != "" {
		t.Active = in.ActiveForm
	}
	if in.Owner != "" {
		t.Owner = in.Owner
	}
}

// launchAck recognises the tool_result Claude Code writes the moment a
// background agent is spawned. The wording is Claude Code's own, not the
// agent's, and it is the same for every agent.
func launchAck(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "Spawned successfully")
}

// promptText is the one line of a prompt the trail quotes. A slash command is
// read as typed rather than as the tags it expands into.
func promptText(ev transcript.Event) string {
	if cmd, ok := transcript.SlashCommand(ev.Text); ok {
		return cmd
	}
	return firstLine(ev.Text)
}

// attach hangs a result's waypoints on the leg it came back to (M2 rule 1):
// the open leg, else the last one — both are the newest leg, since only that
// one can be open — and nowhere at all before the first leg exists.
func (s *Segmenter) attach(res transcript.ToolResult, at time.Time) {
	if len(s.legs) == 0 {
		return
	}
	leg := &s.legs[len(s.legs)-1]

	// Rules 3 and 5: only a remembered Test or Ship call's own result is read
	// as test output or as a commit.
	if r, ok := s.runners[res.ToolUseID]; ok {
		s.forget(res.ToolUseID) // resolved — the memory has done its job
		switch r.kind {
		case Test:
			leg.addWaypoints(testWaypoints(res.Text, r.keyword, res.IsError, at))
		case Ship:
			leg.addWaypoints(shipWaypoints(res.Text, at))
		}
	}

	// Rule 4: any tool's error is a bug, but only where bugs are being made.
	if res.IsError && (leg.class == Build || leg.class == Fix) {
		leg.addBug(firstNonEmptyLine(res.Text), at)
	}
}

// remember records a Test or Ship vote's tool_use id so the result that comes
// back can be parsed (rule 2).
func (s *Segmenter) remember(v vote) {
	if v.id == "" || (v.class != Test && v.class != Ship) {
		return
	}
	if _, dup := s.runners[v.id]; dup {
		return
	}
	if s.runners == nil {
		s.runners = make(map[string]runner)
	}
	if len(s.runnerOrder) >= maxRunners {
		delete(s.runners, s.runnerOrder[0])
		s.runnerOrder = s.runnerOrder[1:]
	}
	s.runners[v.id] = runner{kind: v.class, keyword: v.keyword}
	s.runnerOrder = append(s.runnerOrder, v.id)
}

// forget drops a resolved call from the runner memory.
func (s *Segmenter) forget(id string) {
	delete(s.runners, id)
	for i, held := range s.runnerOrder {
		if held == id {
			s.runnerOrder = append(s.runnerOrder[:i], s.runnerOrder[i+1:]...)
			return
		}
	}
}

// fork records a subagent branch. It never votes and never closes a leg: a
// branch is a second lane, not a change of class on this one (rule 2).
func (s *Segmenter) fork(use transcript.ToolUse, at time.Time) {
	after := -1
	if s.open {
		after = len(s.legs) - 1
	}
	if use.ID != "" {
		if s.byBranch == nil {
			s.byBranch = make(map[string]int)
		}
		s.byBranch[use.ID] = len(s.branches)
	}
	s.branches = append(s.branches, Branch{
		ToolUseID: use.ID,
		Label:     branchLabel(use.Input),
		Start:     at,
		AfterLeg:  after,
	})
}

// applyVote is the segmentation state machine: rules 1, 3 and 4 in order.
func (s *Segmenter) applyVote(v vote, at time.Time) {
	if !s.open {
		s.openLeg(v, at) // rule 1
		s.fold(v, at)
		return
	}

	if v.class == s.legs[len(s.legs)-1].voted {
		// The leg is being confirmed: buffered pressure dies here and settles
		// into the leg it interrupted.
		s.flushPress()
		s.fold(v, at)
		return
	}

	if strong(v.class) { // rule 3
		s.flushPress()
		s.closeLeg()
		s.openLeg(v, at)
		s.fold(v, at)
		return
	}

	// Rule 4: a differing weak vote is pressure, not a decision. A different
	// differing class restarts the streak; the interrupted one settles.
	if len(s.press) > 0 && s.press[0].v.class != v.class {
		s.flushPress()
	}
	s.press = append(s.press, pended{v: v, at: at})
	if len(s.press) < 3 {
		return
	}

	// Third consecutive: the buffered votes were the new leg all along.
	moved := s.press
	s.press = nil
	s.closeLeg()
	s.openLeg(moved[0].v, moved[0].at)
	for _, p := range moved {
		s.fold(p.v, p.at)
	}
}

// flushPress settles buffered pressure votes into the open leg (the streak
// died before reaching three).
func (s *Segmenter) flushPress() {
	if !s.open {
		s.press = nil
		return
	}
	for _, p := range s.press {
		s.fold(p.v, p.at)
	}
	s.press = nil
}

// openLeg starts a leg at start with the vote's class, applying the other half
// of the Fix upgrade: work resumed straight after a failing test run is fixing,
// not building (rule 5b).
func (s *Segmenter) openLeg(v vote, start time.Time) {
	l := legState{class: v.class, voted: v.class, start: start, end: start}
	if v.class == Build && s.afterFailingTest() {
		l.class = Fix
	}
	s.legs = append(s.legs, l)
	s.open = true
}

// closeLeg seals the running leg; the next vote will open a fresh one. The
// pressure buffer is the caller's business: it either settles (flushPress) or
// migrates into the next leg before this is called.
func (s *Segmenter) closeLeg() {
	s.open = false
}

// afterFailingTest reports whether the leg just before this one was a test run
// that produced an error.
func (s *Segmenter) afterFailingTest() bool {
	if len(s.legs) == 0 {
		return false
	}
	prev := &s.legs[len(s.legs)-1]
	return prev.voted == Test && prev.hadError
}

// fold merges one vote into the open leg. A Test or Ship vote also earns a
// place in the runner memory: its result is the only one worth parsing.
func (s *Segmenter) fold(v vote, at time.Time) {
	s.remember(v)
	foldInto(&s.legs[len(s.legs)-1], v, at)
}

// foldInto merges one vote into a leg — the open one, or a display clone.
func foldInto(l *legState, v vote, at time.Time) {
	l.votes++
	if !at.IsZero() {
		if l.start.IsZero() {
			l.start = at
		}
		l.end = at
	}
	if v.file != "" {
		l.addFile(v.file)
	}
	if l.keyword == "" {
		l.keyword = v.keyword
	}
}

// addFile records a touched basename, first-seen order, counting repeats so the
// label can pick the file the leg was really about.
func (l *legState) addFile(name string) {
	for i := range l.files {
		if l.files[i].name == name {
			l.files[i].n++
			return
		}
	}
	if len(l.files) >= maxFiles {
		return
	}
	l.files = append(l.files, fileCount{name: name, n: 1})
}

// addWaypoints appends what one result was worth, up to the leg's cap; the
// overflow drops silently, since a leg the panel cannot show more of has
// already said what it has to say.
func (l *legState) addWaypoints(wps []Waypoint) {
	for _, w := range wps {
		l.addWaypoint(w)
	}
}

func (l *legState) addWaypoint(w Waypoint) {
	if len(l.waypoints) >= maxWaypoints {
		return
	}
	l.waypoints = append(l.waypoints, w)
}

// addBug records one error signature (rule 4): the same error hit twice is one
// bug, and past three the leg is simply having a bad day.
func (l *legState) addBug(text string, at time.Time) {
	text = clip(text, waypointText)
	if text == "" {
		return
	}
	bugs := 0
	for i := range l.waypoints {
		if l.waypoints[i].Kind != WaypointBug {
			continue
		}
		if l.waypoints[i].Text == text {
			return
		}
		bugs++
	}
	if bugs >= maxBugs {
		return
	}
	l.addWaypoint(Waypoint{Kind: WaypointBug, Text: text, At: at})
}

// waypointsCopy hands the caller its own slice — the segmenter keeps folding
// into the original.
func (l *legState) waypointsCopy() []Waypoint {
	if len(l.waypoints) == 0 {
		return nil
	}
	return append(make([]Waypoint, 0, len(l.waypoints)), l.waypoints...)
}

func (l *legState) fileNames() []string {
	if len(l.files) == 0 {
		return nil
	}
	names := make([]string, len(l.files))
	for i := range l.files {
		names[i] = l.files[i].name
	}
	return names
}

// label is rule 7: the file the leg kept coming back to, else the runner or
// git subcommand that named it, else nothing — the renderer then falls back to
// the bare class verb.
func (l *legState) label() string {
	if name := l.dominantFile(); name != "" {
		return strings.ToLower(clip(name, 24))
	}
	switch l.class {
	case Test, Ship:
		return strings.ToLower(clip(l.keyword, 24))
	}
	return ""
}

// dominantFile is the most-frequent basename, ties broken by first seen.
func (l *legState) dominantFile() string {
	best := ""
	bestN := 0
	for i := range l.files {
		if l.files[i].n > bestN {
			best, bestN = l.files[i].name, l.files[i].n
		}
	}
	return best
}
