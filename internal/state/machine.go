// Package state turns a stream of transcript events into the one thing the
// fleet strip must never get wrong: is this session working, does it need you,
// is it idle, or is it stuck?
package state

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/deephanson94/compass/internal/transcript"
)

// State is a session's condition right now.
type State int

// The four session conditions (SPEC §2.4).
const (
	Working State = iota
	NeedsYou
	Idle
	Stuck
)

// String returns the lowercase name of the state.
func (s State) String() string {
	switch s {
	case Working:
		return "working"
	case NeedsYou:
		return "needs-you"
	case Idle:
		return "idle"
	case Stuck:
		return "stuck"
	default:
		return "unknown"
	}
}

// The silence budgets of an open turn. Silence is the time since the session
// last wrote — the session or one of its subagents. Under StillAfter it is
// working; from StillAfter it is still working (Snapshot.Late); from
// AttnAfter it is Stuck, which the card calls "needs attn". They are vars
// so the config can set them (SetTiers).
var (
	StillAfter = 10 * time.Minute
	AttnAfter  = time.Hour
)

// SetTiers sets the two budgets; a value of zero or less keeps its default,
// and an attn budget below the still one is raised to it.
func SetTiers(still, attn time.Duration) {
	if still > 0 {
		StillAfter = still
	}
	if attn > 0 {
		AttnAfter = attn
	}
	if AttnAfter < StillAfter {
		AttnAfter = StillAfter
	}
}

// ApproveAfter is how long a call that answers at once — an Edit, a Read —
// may stay out before its silence means Claude Code is holding it for your
// permission. It is also the grace a Bash call gets past its own timeout
// before the missing result counts.
const ApproveAfter = 90 * time.Second

// What an open turn is waiting on, as Snapshot.Pending names it.
const (
	PendingBash     = "bash"
	PendingSubagent = "subagent"
	PendingMCP      = "MCP"
	PendingWeb      = "web"
	PendingWorkflow = "workflow"
	PendingThink    = "think"   // tool results are in; the model has not answered
	PendingStart    = "start"   // a prompt is in; the model has not answered
	PendingApprove  = "approve" // a call that answers at once is held: a permission prompt
	PendingTool     = "tool"    // any other call
)

// Snapshot is the machine's verdict at one instant.
type Snapshot struct {
	State    State
	Since    time.Time // timestamp of the event that established this condition
	Reason   string    // short human phrase, e.g. "turn ended with a question"
	Activity string    // hint, e.g. `Bash: pytest tests/auth -x`, `reading middleware.py`

	// Allowed is the budget the call in flight was given — a Bash timeout —
	// when its silence is inside that budget, so a row can say "2m of 7m"
	// where it would otherwise have said "silent 2m". Zero when the call
	// has no budget or the budget has run out.
	Allowed time.Duration

	// APIError says the session is waiting on a call the API refused, not on
	// anything it did. A reader needs the error's own words here more than the
	// session's last good result, so the fleet lets this override its usual
	// preference for outcome over activity.
	APIError bool
	// APIStatus is the refused call's HTTP status, 0 when it carried none.
	APIStatus int

	// Open says a turn is in flight: a call is out, or the model owes a
	// reply. Pending names what it waits on (the Pending* constants); ""
	// when the turn is closed.
	Open    bool
	Pending string

	// Late is a working turn past StillAfter of silence: "still working".
	Late bool

	// Interrupted is a turn closed by Esc rather than by the model.
	Interrupted bool

	// Exited is an open turn whose process is gone (set by the fleet, which
	// reads the process registry; the machine only reads the transcript).
	Exited bool

	// Verdict is how the session said its work came out, from its closing
	// line (verdict.go): VerdictNeedsYou, with the reason as Activity, or
	// VerdictDone. "" for a turn that ended on no verdict.
	Verdict string
}

// askUserQuestion is the tool whose pending call means "the model is literally
// asking you something" — no quiet threshold applies.
const askUserQuestion = "AskUserQuestion"

// AskUserQuestion is that tool's name, for the readers that walk a transcript
// without building a machine: the fleet's peek asks the same question of a
// file's last lines that rule 2 asks of the fold.
const AskUserQuestion = askUserQuestion

type pendingUse struct {
	use transcript.ToolUse
	at  time.Time
	seq int
	msg string // the assistant message that made the call
}

// Machine folds a session's events into a Snapshot. Feed it events in file
// order; it keeps only the handful of facts the state rules need.
type Machine struct {
	pending map[string]pendingUse
	seq     int

	lastEventAt time.Time

	sawSubstantive  bool
	substantiveKind transcript.EventType
	substantiveAt   time.Time
	substantiveText string

	// The substantive beat was the API refusing rather than the model
	// answering. It is folded with the beat it belongs to, so a later real
	// turn clears it without any separate bookkeeping.
	substantiveAPIError bool
	substantiveStatus   int
	substantiveErrorKey string

	lastUse    transcript.ToolUse // most recent tool_use seen, pending or not
	hasLastUse bool
	cwd        string // the session's directory, from its events: a command's own "cd" into it is not the news (#78)

	// awaitingModel is true between a tool_result landing and the model's next
	// assistant event: no tool is pending, but the turn is not over either.
	awaitingModel bool
	awaitingSince time.Time

	// interrupted is a turn Esc closed: set by the interrupt line, cleared
	// by the next thing anyone says.
	interrupted bool

	// agentWrote is the newest write by a subagent's own file, which the
	// fleet reads (NoteAgentWrite): while an Agent call is out, the lead's
	// transcript is silent and the agent's is the session's clock.
	agentWrote time.Time
}

// NewMachine returns an empty machine: no events, no opinions.
func NewMachine() *Machine {
	return &Machine{pending: make(map[string]pendingUse)}
}

// Observe feeds one event to the machine. Events must arrive in file order.
func (m *Machine) Observe(ev transcript.Event) {
	if ev.CWD != "" {
		m.cwd = ev.CWD
	}
	// Lines without a timestamp (mode, latch, bookkeeping) still arrive but must
	// not drag lastEventAt backwards to the zero time.
	if !ev.Timestamp.IsZero() && ev.Timestamp.After(m.lastEventAt) {
		m.lastEventAt = ev.Timestamp
	}

	if ev.Interrupt() {
		// Esc: the harness abandoned whatever was out, and the turn is
		// over until somebody speaks again.
		m.interrupted = true
		m.awaitingModel = false
		clear(m.pending)
		return
	}
	if substantive(ev) || len(ev.ToolUses) > 0 {
		m.interrupted = false
	}

	for _, res := range ev.ToolResults {
		delete(m.pending, res.ToolUseID)
	}
	// A new message from the model means every call of an earlier one was
	// answered: the model does not go on past calls it is still waiting
	// for. Their results are not always in the file — SendMessage's were
	// not — and a call kept open on that account held a finished turn
	// open for good, needs attn over a session waiting on you. One
	// message's lines share its id, so its own parallel calls stand. A
	// subagent's lines are its own conversation, and close nothing here;
	// nor does a line with nothing said on it (thinking only), which the
	// fleet's walk of a file's tail does not count either.
	said := strings.TrimSpace(ev.Text) != "" || len(ev.ToolUses) > 0
	if ev.Type == transcript.EventAssistant && !ev.IsSidechain && ev.MessageID != "" && said {
		for id, p := range m.pending {
			if p.msg != ev.MessageID {
				delete(m.pending, id)
			}
		}
	}
	if ev.Type == transcript.EventUser && len(ev.ToolResults) > 0 {
		m.awaitingModel = true
		m.awaitingSince = ev.Timestamp
	}
	if ev.Type == transcript.EventAssistant {
		m.awaitingModel = false
	}
	for _, use := range ev.ToolUses {
		m.seq++
		m.pending[use.ID] = pendingUse{use: use, at: ev.Timestamp, seq: m.seq, msg: ev.MessageID}
		m.lastUse = use
		m.hasLastUse = true
	}

	if substantive(ev) {
		m.sawSubstantive = true
		m.substantiveKind = ev.Type
		m.substantiveAt = ev.Timestamp
		m.substantiveText = ev.Text
		m.substantiveAPIError = ev.APIError
		m.substantiveStatus, m.substantiveErrorKey = ev.Status, ev.ErrorKey
	}
}

// substantive reports whether an event is a real conversational beat: a human
// prompt or any assistant turn. Attachments, queue operations, unknown lines
// and tool-result-only user lines refresh the clock but say nothing.
func substantive(ev transcript.Event) bool {
	switch ev.Type {
	case transcript.EventAssistant:
		return true
	case transcript.EventUser:
		// Only a person's words open a turn. "Continue from where you left
		// off." is the harness's, and counting it started a turn that the
		// model might never answer — a resumed-then-abandoned session drifted
		// to stuck on the strength of a line nobody typed.
		return strings.TrimSpace(ev.Text) != "" && !ev.Machinery()
	default:
		return false
	}
}

// Evaluate applies the state rules, in precedence order, as of now.
func (m *Machine) Evaluate(now time.Time) Snapshot {
	// 1. Nothing has happened yet.
	if !m.sawSubstantive {
		return Snapshot{State: Idle, Since: m.lastEventAt, Reason: "no activity yet", Activity: "idle"}
	}

	// 1b. Esc closed the turn: nothing is coming until somebody speaks.
	if m.interrupted {
		return Snapshot{State: Idle, Since: m.lastEventAt, Reason: "interrupted", Activity: "idle", Interrupted: true}
	}

	// 2. The model is holding a question open.
	if ask, ok := m.pendingNamed(askUserQuestion); ok {
		return Snapshot{
			State:    NeedsYou,
			Since:    m.since(ask.at),
			Reason:   "waiting on your answer",
			Activity: activityFor(ask.use),
			Open:     true,
		}
	}

	// 3. Some other tool call is still in flight.
	if oldest, ok := m.oldestPending(); ok {
		quiet := now.Sub(m.lastActivity())
		act := activityFor(WithoutCD(oldest.use, m.cwd))
		if m.hasLastUse {
			act = activityFor(WithoutCD(m.lastUse, m.cwd))
		}
		// A shell command is allowed its own budget before its silence
		// means anything: the harness gives Bash a timeout (two minutes
		// unless the call names one, up to ten) and kills the command when
		// it runs out, so a `sleep 420` under a ten-minute budget is a
		// session doing what it said. The budget is the latest of every
		// call in flight: Claude Code batches calls, and a Read issued
		// beside the sleep is the oldest pending one.
		if allow, _, at := m.budget(now); allow > 0 {
			// "for 2m of 10m" counts from the call the budget belongs to.
			return Snapshot{State: Working, Since: at, Reason: "Bash allowed " + ShortDuration(allow), Activity: act, Allowed: allow, Open: true, Pending: PendingBash}
		}
		if quiet < ApproveAfter {
			return Snapshot{State: Working, Since: m.since(oldest.at), Reason: "tool call in flight", Activity: act, Open: true, Pending: m.pendingKind()}
		}
		// A call that answers at once and has not: Claude Code is holding
		// it behind a permission prompt. Nothing else keeps an Edit out
		// for ninety seconds, and the transcript has no line that says so.
		if held, ok := m.heldForApproval(); ok {
			return Snapshot{
				State: NeedsYou, Since: m.since(held.at), Reason: "awaiting your approval",
				Activity: activityFor(WithoutCD(held.use, m.cwd)), Open: true, Pending: PendingApprove,
			}
		}
		// Past its budget the harness should have written the command's
		// result; a Bash call still out is one the tiers do not excuse.
		if kind := m.pendingKind(); kind == PendingBash {
			return Snapshot{State: Stuck, Since: m.since(oldest.at), Reason: "no result past the Bash timeout", Activity: act, Open: true, Pending: kind}
		}
		snap := m.tiered(quiet, m.since(oldest.at), "tool call in flight")
		snap.Activity, snap.Pending = act, m.pendingKind()
		return snap
	}

	// 4. The last substantive beat was the model's.
	if m.substantiveKind == transcript.EventAssistant {
		// Results came back and the model has not spoken since: the turn is
		// still in flight, not idle.
		if m.awaitingModel {
			snap := m.tiered(now.Sub(m.lastEventAt), m.since(m.awaitingSince), "processing results")
			snap.Activity, snap.Pending = "thinking…", PendingThink
			return snap
		}
		since := m.since(m.substantiveAt)

		// The call failed rather than the model answering. Claude Code writes
		// that as a synthetic assistant message, so it reaches here looking
		// exactly like a finished turn — which is how a session dead on quota
		// reported "turn complete" and sat in the fleet indistinguishable from
		// one that had succeeded.
		//
		// It is needs-you rather than stuck: stuck is a symptom of an unknown
		// cause, and this is a known one that only a person can clear — log in
		// again, wait for the quota window, raise the limit. Needs-you also
		// puts it where `g` can reach it, which is the whole point of knowing.
		if m.substantiveAPIError {
			return Snapshot{
				State: NeedsYou, Since: since, APIError: true, APIStatus: m.substantiveStatus,
				Reason: apiErrorReason(m.substantiveStatus, m.substantiveErrorKey),
				// The error's own words are what a person recognises; the
				// status and key above are what a program should key on.
				Activity: apiErrorText(firstLine(m.substantiveText)),
			}
		}
		// A verdict is the session's own word on how it came out, and it
		// outranks a question mark: "VERDICT: needs-you: keep the old API?"
		// is the verdict's reason, not a question to the room.
		switch v, why := Verdict(m.substantiveText); v {
		case VerdictNeedsYou:
			return Snapshot{State: NeedsYou, Since: since, Reason: "verdict: needs you", Activity: why, Verdict: v}
		case VerdictDone:
			return Snapshot{State: Idle, Since: since, Reason: "verdict: done", Activity: "idle", Verdict: v}
		}
		if endsWithQuestion(m.substantiveText) {
			return Snapshot{State: NeedsYou, Since: since, Reason: "turn ended with a question", Activity: "awaiting your reply"}
		}
		return Snapshot{State: Idle, Since: since, Reason: "turn complete", Activity: "idle"}
	}

	// 5. A prompt is in, the model has not spoken yet.
	snap := m.tiered(now.Sub(m.lastEventAt), m.since(m.substantiveAt), "starting turn")
	snap.Activity, snap.Pending = "thinking…", PendingStart
	return snap
}

// tiered judges an open turn by its silence: working under StillAfter, still
// working (Late) under AttnAfter, Stuck past it.
func (m *Machine) tiered(quiet time.Duration, since time.Time, reason string) Snapshot {
	switch {
	case quiet < StillAfter:
		return Snapshot{State: Working, Since: since, Reason: reason, Open: true}
	case quiet < AttnAfter:
		return Snapshot{State: Working, Since: since, Reason: "silent " + ShortDuration(quiet) + " mid-turn", Open: true, Late: true}
	}
	return Snapshot{State: Stuck, Since: since, Reason: stuckReason(quiet), Open: true}
}

// lastActivity is the session's clock: its own last line, or — while an
// Agent call is out — its subagent's last write, whichever is newer.
func (m *Machine) lastActivity() time.Time {
	if !m.agentWrote.After(m.lastEventAt) || len(m.PendingAgents()) == 0 {
		return m.lastEventAt
	}
	return m.agentWrote
}

// NoteAgentWrite tells the machine a subagent's own file was written at t.
// Only the newest write counts, and only while an Agent call is out.
func (m *Machine) NoteAgentWrite(t time.Time) {
	if t.After(m.agentWrote) {
		m.agentWrote = t
	}
}

// PendingAgents is the ids of the Agent calls still out, oldest first: the
// keys a subagent's meta file pairs its transcript by.
func (m *Machine) PendingAgents() []string {
	var ps []pendingUse
	for _, p := range m.pending {
		if isAgentTool(p.use.Name) {
			ps = append(ps, p)
		}
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].seq < ps[j].seq })
	ids := make([]string, len(ps))
	for i, p := range ps {
		ids[i] = p.use.ID
	}
	return ids
}

// pendingKind names what the turn waits on, from its calls in flight: the
// slowest kind among them, so a Read issued beside an Agent call or a
// command reads as the agent or the command it waits for.
func (m *Machine) pendingKind() string {
	rank := map[string]int{PendingSubagent: 6, PendingWorkflow: 5, PendingMCP: 4, PendingWeb: 3, PendingTool: 2, PendingBash: 1}
	best, fast := "", false
	for _, p := range m.pending {
		if answersAtOnce(p.use.Name) {
			fast = true
			continue
		}
		if k := toolKind(p.use.Name); rank[k] > rank[best] {
			best = k
		}
	}
	if best == "" && fast {
		return PendingTool
	}
	return best
}

// heldForApproval is the newest call in flight that answers at once, when
// nothing slower is out beside it: with a command, an agent or a server out,
// the quiet is theirs.
func (m *Machine) heldForApproval() (pendingUse, bool) {
	var best pendingUse
	found := false
	for _, p := range m.pending {
		if !answersAtOnce(p.use.Name) {
			return pendingUse{}, false
		}
		if !found || p.seq > best.seq {
			best, found = p, true
		}
	}
	return best, found
}

// answersAtOnce is the tools that return in well under a second unless
// Claude Code is holding them for you: a permission prompt, or — for
// ExitPlanMode — the plan waiting on your yes.
func answersAtOnce(tool string) bool {
	switch tool {
	case "Edit", "MultiEdit", "Write", "NotebookEdit", "Read", "Glob", "Grep", "LS", "ExitPlanMode":
		return true
	}
	return false
}

// isAgentTool is the subagent call, by both the names it has had.
func isAgentTool(tool string) bool { return tool == "Agent" || tool == "Task" }

// toolKind is a call's kind as Snapshot.Pending names it.
func toolKind(tool string) string {
	switch {
	case tool == "Bash":
		return PendingBash
	case isAgentTool(tool):
		return PendingSubagent
	case strings.HasPrefix(tool, "mcp__"):
		return PendingMCP
	case tool == "WebFetch" || tool == "WebSearch":
		return PendingWeb
	case tool == "Workflow":
		return PendingWorkflow
	}
	return PendingTool
}

// since falls back to the last event's timestamp when the establishing event
// carried none.
func (m *Machine) since(at time.Time) time.Time {
	if at.IsZero() {
		return m.lastEventAt
	}
	return at
}

func (m *Machine) pendingNamed(name string) (pendingUse, bool) {
	var best pendingUse
	found := false
	for _, p := range m.pending {
		if p.use.Name != name {
			continue
		}
		if !found || p.seq < best.seq {
			best, found = p, true
		}
	}
	return best, found
}

// oldestPending is the tool call that has been waiting longest — the one that
// opened the current wait.
func (m *Machine) oldestPending() (pendingUse, bool) {
	var best pendingUse
	found := false
	for _, p := range m.pending {
		if !found || p.seq < best.seq {
			best, found = p, true
		}
	}
	return best, found
}

// apiErrorReason names the failure from the API's own fields, never from the
// message text: the status and the error key are Claude Code's, while the
// wording belongs to whichever gateway refused the call and is different for
// every organisation.
func apiErrorReason(status int, key string) string {
	switch {
	case status != 0 && key != "":
		return fmt.Sprintf("api error %d · %s", status, key)
	case status != 0:
		return fmt.Sprintf("api error %d", status)
	case key != "":
		return "api error · " + key
	}
	return "the api call failed"
}

// apiErrorText drops Claude Code's own "API Error: 403" marker from the message
// it wrote around the gateway's words. The status is already in the reason, and
// every column the marker spends is a column the words a person actually
// recognises — "your daily quota is exhausted" — does not get, because those
// come last and the fleet row is two dozen columns wide.
func apiErrorText(s string) string {
	i := strings.Index(s, apiErrorMarker)
	if i < 0 {
		return s
	}
	rest := s[i+len(apiErrorMarker):]
	rest = strings.TrimLeft(rest, " ")
	// Drop the status digits too, and the punctuation the marker was holding
	// together, so the two halves rejoin as one sentence.
	rest = strings.TrimLeft(rest, "0123456789")
	rest = strings.TrimLeft(rest, " ·:-")
	if strings.TrimSpace(rest) == "" {
		return s
	}
	head := strings.TrimRight(s[:i], " ·:-")
	if head == "" {
		return rest
	}
	return head + " · " + rest
}

// apiErrorMarker is the phrase Claude Code writes before the status it got.
const apiErrorMarker = "API Error:"

// bashTimeout is the budget the harness gives a Bash call that names none,
// and bashTimeoutMax the most it grants however large the call's own.
const (
	bashTimeout    = 2 * time.Minute
	bashTimeoutMax = 10 * time.Minute
)

// budget is the budget of the pending call that is allowed the longest,
// and the moment it runs out: zero when no call in flight has one, or
// every one has run out.
func (m *Machine) budget(now time.Time) (allow time.Duration, deadline, at time.Time) {
	for _, p := range m.pending {
		a := allowance(p.use)
		if a == 0 {
			continue
		}
		if d := m.since(p.at).Add(a); now.Before(d) && d.After(deadline) {
			allow, deadline, at = a, d, m.since(p.at)
		}
	}
	return allow, deadline, at
}

// allowance is how long a pending call may run before its silence counts —
// the Bash timeout, in milliseconds on the call's input, or the harness's
// default. Every other tool answers at once or not at all, so 0.
func allowance(use transcript.ToolUse) time.Duration {
	if use.Name != "Bash" {
		return 0
	}
	if rawBool(use.Input, "run_in_background") {
		return 0 // returns at once; the harness enforces no budget on it
	}
	if ms := rawNumber(use.Input, "timeout"); ms > 0 {
		return min(time.Duration(ms)*time.Millisecond, bashTimeoutMax)
	}
	return bashTimeout
}

// rawBool reads a boolean field off a tool call's input; false when absent.
func rawBool(input json.RawMessage, key string) bool {
	if len(input) == 0 {
		return false
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(input, &obj); err != nil {
		return false
	}
	var b bool
	return json.Unmarshal(obj[key], &b) == nil && b
}

// rawNumber reads a numeric field off a tool call's input; 0 when absent or
// not a number.
func rawNumber(input json.RawMessage, key string) float64 {
	if len(input) == 0 {
		return 0
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(input, &obj); err != nil {
		return 0
	}
	var n float64
	if err := json.Unmarshal(obj[key], &n); err != nil {
		return 0
	}
	return n
}

func stuckReason(quiet time.Duration) string {
	return fmt.Sprintf("no output for %s mid-turn", ShortDuration(quiet))
}

// questionTrim is the trailing noise a model's closing question may wear:
// whitespace plus markdown decoration.
const questionTrim = " \t\r\n*_`)"

// EndsWithQuestion is rule 4's test — "the turn ended with a question" — for
// callers outside the machine. The fleet's peek uses it on the last lines of a
// transcript nothing is tailing, so a question found in a file and a question
// found in the fold are the same question.
func EndsWithQuestion(text string) bool { return endsWithQuestion(text) }

// endsWithQuestion reports whether text closes on a question mark once
// whitespace and markdown decoration are peeled off the right.
func endsWithQuestion(text string) bool {
	t := strings.TrimRight(text, questionTrim)
	return strings.HasSuffix(t, "?")
}

// activityFor renders the one-line hint for a tool call.
// askedQuestion reads the first question out of an AskUserQuestion call's
// input — {"questions":[{"question":"…","options":[{"label":"…"},…]}]} —
// with its options after it, so the row says what is being decided.
func askedQuestion(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var in struct {
		Questions []struct {
			Question string `json:"question"`
			Options  []struct {
				Label string `json:"label"`
			} `json:"options"`
		} `json:"questions"`
	}
	if err := json.Unmarshal(input, &in); err != nil || len(in.Questions) == 0 {
		return ""
	}
	q := in.Questions[0]
	text := firstLine(q.Question)
	var labels []string
	for _, o := range q.Options {
		if l := strings.TrimSpace(o.Label); l != "" {
			labels = append(labels, l)
		}
	}
	if len(labels) > 0 {
		text += " [" + strings.Join(labels, " / ") + "]"
	}
	return text
}

// WithoutCD is a Bash call with a leading "cd <cwd>;" or "cd <cwd> &&"
// taken off its command when the directory is the session's own: the row
// "Bash: cd /home/user/api; git diff …" spent its cells entering the room
// the session was already in (#78). A bare "cd <cwd>" stays: there is
// nothing else to show.
func WithoutCD(use transcript.ToolUse, cwd string) transcript.ToolUse {
	if use.Name != "Bash" || cwd == "" {
		return use
	}
	cmd := rawField(use.Input, "command")
	rest, ok := StripCD(cmd, cwd)
	if !ok {
		return use
	}
	out := use
	out.Input = json.RawMessage(fmt.Sprintf(`{"command":%s}`, strconv.Quote(rest)))
	return out
}

// StripCD is the command after its leading "cd <cwd>;" / "cd <cwd> &&",
// and whether there was one to take.
func StripCD(cmd, cwd string) (string, bool) {
	cwd = strings.TrimRight(cwd, "/")
	if cwd == "" {
		return cmd, false
	}
	t := strings.TrimSpace(cmd)
	for _, dir := range []string{cwd, cwd + "/", strconv.Quote(cwd), "'" + cwd + "'"} {
		for _, sep := range []string{";", "&&"} {
			if rest, ok := strings.CutPrefix(t, "cd "+dir+" "+sep+" "); ok && strings.TrimSpace(rest) != "" {
				return strings.TrimSpace(rest), true
			}
			if rest, ok := strings.CutPrefix(t, "cd "+dir+sep+" "); ok && strings.TrimSpace(rest) != "" {
				return strings.TrimSpace(rest), true
			}
		}
	}
	return cmd, false
}

// ActivityFor is the one-line hint a row draws for a tool call, for the
// readers that build a call rather than fold a transcript — the other
// store's adapter, whose question tool has to reach the row as a question
// and not as the tool's own name (round 61).
func ActivityFor(use transcript.ToolUse) string { return activityFor(use) }

func activityFor(use transcript.ToolUse) string {
	switch use.Name {
	case "Bash":
		if cmd := firstLine(rawField(use.Input, "command")); cmd != "" {
			return "Bash: " + clip(cmd, 40)
		}
		return "Bash"
	case "Read", "Edit", "Write", "NotebookEdit":
		if path := rawField(use.Input, "file_path"); path != "" {
			return verbOf(use.Name) + " " + filepath.Base(path)
		}
		return verbOf(use.Name)
	case askUserQuestion:
		// The question itself, not the tool's name: "needs you" is the whole
		// game, and a row that said "AskUserQuestion" at every level left the
		// person unable to triage what was being asked without attaching.
		if q := askedQuestion(use.Input); q != "" {
			return q
		}
		return use.Name
	case "":
		return "working"
	default:
		return use.Name
	}
}

func verbOf(tool string) string {
	switch tool {
	case "Read":
		return "reading"
	case "Edit", "NotebookEdit":
		return "editing"
	case "Write":
		return "writing"
	default:
		return strings.ToLower(tool)
	}
}

// rawField pulls one string field out of a raw tool input object.
func rawField(input json.RawMessage, key string) string {
	if len(input) == 0 {
		return ""
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(input, &obj); err != nil {
		return ""
	}
	raw, ok := obj[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// clip truncates to max display runes, marking the cut with an ellipsis.
func clip(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return strings.TrimRight(string(r[:max-1]), " ") + "…"
}

// ShortDuration renders a duration the way a glanceable panel wants it: one
// unit, no seconds tacked onto minutes. 40s, 3m, 2h, 4d.
func ShortDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
