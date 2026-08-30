package journey

import (
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
}

// Trail is the whole journey as the panel needs it: what was asked, what was
// done, and what forked off along the way.
type Trail struct {
	Prompts  []Prompt // every substantive user prompt, oldest first
	Legs     []Leg    // oldest first
	Branches []Branch // oldest first
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
}

// Segmenter folds a session's events into legs. Feed it events in file order;
// it holds only the running leg plus the little pressure gauge rule 4 needs.
type Segmenter struct {
	prompts  []Prompt
	legs     []legState
	branches []Branch
	byBranch map[string]int // Agent tool_use id → index into branches
	open     bool           // the last leg is still accepting votes

	// Pressure from differing weak votes (rule 4): which class is pushing, how
	// many in a row, and when the run started — the new leg inherits that time.
	pressClass Class
	pressCount int
	pressAt    time.Time
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
	}

	// Rule 2: a human prompt is a hard boundary, whatever was running.
	if substantivePrompt(ev) {
		s.prompts = append(s.prompts, Prompt{Text: clip(firstLine(ev.Text), 60), At: ev.Timestamp})
		s.closeLeg()
	}

	for _, use := range ev.ToolUses {
		if use.Name == agentTool {
			s.fork(use, ev.Timestamp)
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
	if len(s.legs) > 0 {
		tr.Legs = make([]Leg, len(s.legs))
		for i := range s.legs {
			l := &s.legs[i]
			tr.Legs[i] = Leg{
				Class: l.class,
				Label: l.label(),
				Start: l.start,
				End:   l.end,
				Votes: l.votes,
				Files: l.fileNames(),
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
// direction and must not break a leg.
func substantivePrompt(ev transcript.Event) bool {
	return ev.Type == transcript.EventUser && strings.TrimSpace(ev.Text) != ""
}

// observeResult merges a returning branch and records failure against the open
// leg — the signal behind the Fix upgrade (rule 5a).
func (s *Segmenter) observeResult(res transcript.ToolResult, at time.Time) {
	if i, ok := s.byBranch[res.ToolUseID]; ok {
		b := &s.branches[i]
		b.Done = true
		b.End = at
	}
	if !res.IsError || !s.open {
		return
	}
	cur := &s.legs[len(s.legs)-1]
	cur.hadError = true
	if cur.class == Build {
		// Retroactive, whole leg: work that hit an error was always a fix.
		cur.class = Fix
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
		// The leg is being confirmed: any pressure that had built up dies here.
		s.pressCount = 0
		s.fold(v, at)
		return
	}

	if strong(v.class) { // rule 3
		s.closeLeg()
		s.openLeg(v, at)
		s.fold(v, at)
		return
	}

	// Rule 4: a differing weak vote is pressure, not a decision.
	if s.pressCount > 0 && s.pressClass == v.class {
		s.pressCount++
	} else {
		s.pressClass, s.pressCount, s.pressAt = v.class, 1, at
	}
	if s.pressCount < 3 {
		s.fold(v, at) // hysteresis: it folds into the leg it interrupted
		return
	}
	s.closeLeg()
	s.openLeg(v, s.pressAt) // the new leg began with the first of the three
	s.fold(v, at)
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
	s.pressCount = 0
}

// closeLeg seals the running leg; the next vote will open a fresh one.
func (s *Segmenter) closeLeg() {
	s.open = false
	s.pressCount = 0
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

// fold merges one vote into the open leg.
func (s *Segmenter) fold(v vote, at time.Time) {
	l := &s.legs[len(s.legs)-1]
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
		return clip(name, 24)
	}
	switch l.class {
	case Test, Ship:
		return clip(l.keyword, 24)
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
