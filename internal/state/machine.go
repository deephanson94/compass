// Package state turns a stream of transcript events into the one thing the
// fleet strip must never get wrong: is this session working, does it need you,
// is it idle, or is it stuck?
package state

import (
	"encoding/json"
	"fmt"
	"path/filepath"
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

// StuckAfter is how long a mid-turn silence may last before a session is called
// stuck rather than working.
const StuckAfter = 90 * time.Second

// Snapshot is the machine's verdict at one instant.
type Snapshot struct {
	State    State
	Since    time.Time // timestamp of the event that established this condition
	Reason   string    // short human phrase, e.g. "turn ended with a question"
	Activity string    // hint, e.g. `Bash: pytest tests/auth -x`, `reading middleware.py`

	// APIError says the session is waiting on a call the API refused, not on
	// anything it did. A reader needs the error's own words here more than the
	// session's last good result, so the fleet lets this override its usual
	// preference for outcome over activity.
	APIError bool
}

// askUserQuestion is the tool whose pending call means "the model is literally
// asking you something" — no quiet threshold applies.
const askUserQuestion = "AskUserQuestion"

type pendingUse struct {
	use transcript.ToolUse
	at  time.Time
	seq int
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

	// awaitingModel is true between a tool_result landing and the model's next
	// assistant event: no tool is pending, but the turn is not over either.
	awaitingModel bool
	awaitingSince time.Time
}

// NewMachine returns an empty machine: no events, no opinions.
func NewMachine() *Machine {
	return &Machine{pending: make(map[string]pendingUse)}
}

// Observe feeds one event to the machine. Events must arrive in file order.
func (m *Machine) Observe(ev transcript.Event) {
	// Lines without a timestamp (mode, latch, bookkeeping) still arrive but must
	// not drag lastEventAt backwards to the zero time.
	if !ev.Timestamp.IsZero() && ev.Timestamp.After(m.lastEventAt) {
		m.lastEventAt = ev.Timestamp
	}

	for _, res := range ev.ToolResults {
		delete(m.pending, res.ToolUseID)
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
		m.pending[use.ID] = pendingUse{use: use, at: ev.Timestamp, seq: m.seq}
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
		return strings.TrimSpace(ev.Text) != ""
	default:
		return false
	}
}

// Evaluate applies the five state rules, in precedence order, as of now.
func (m *Machine) Evaluate(now time.Time) Snapshot {
	// 1. Nothing has happened yet.
	if !m.sawSubstantive {
		return Snapshot{State: Idle, Since: m.lastEventAt, Reason: "no activity yet", Activity: "idle"}
	}

	// 2. The model is holding a question open.
	if ask, ok := m.pendingNamed(askUserQuestion); ok {
		return Snapshot{
			State:    NeedsYou,
			Since:    m.since(ask.at),
			Reason:   "waiting on your answer",
			Activity: activityFor(ask.use),
		}
	}

	// 3. Some other tool call is still in flight.
	if oldest, ok := m.oldestPending(); ok {
		quiet := now.Sub(m.lastEventAt)
		act := activityFor(oldest.use)
		if m.hasLastUse {
			act = activityFor(m.lastUse)
		}
		if quiet < StuckAfter {
			return Snapshot{State: Working, Since: m.since(oldest.at), Reason: "tool call in flight", Activity: act}
		}
		return Snapshot{State: Stuck, Since: m.since(oldest.at), Reason: stuckReason(quiet), Activity: act}
	}

	// 4. The last substantive beat was the model's.
	if m.substantiveKind == transcript.EventAssistant {
		// Results came back and the model has not spoken since: the turn is
		// still in flight, not idle.
		if m.awaitingModel {
			quiet := now.Sub(m.lastEventAt)
			since := m.since(m.awaitingSince)
			if quiet < StuckAfter {
				return Snapshot{State: Working, Since: since, Reason: "processing results", Activity: "thinking…"}
			}
			return Snapshot{State: Stuck, Since: since, Reason: stuckReason(quiet), Activity: "thinking…"}
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
				State: NeedsYou, Since: since, APIError: true,
				Reason: apiErrorReason(m.substantiveStatus, m.substantiveErrorKey),
				// The error's own words are what a person recognises; the
				// status and key above are what a program should key on.
				Activity: apiErrorText(firstLine(m.substantiveText)),
			}
		}
		if endsWithQuestion(m.substantiveText) {
			return Snapshot{State: NeedsYou, Since: since, Reason: "turn ended with a question", Activity: "awaiting your reply"}
		}
		return Snapshot{State: Idle, Since: since, Reason: "turn complete", Activity: "idle"}
	}

	// 5. A prompt is in, the model has not spoken yet.
	quiet := now.Sub(m.lastEventAt)
	since := m.since(m.substantiveAt)
	if quiet < StuckAfter {
		return Snapshot{State: Working, Since: since, Reason: "starting turn", Activity: "thinking…"}
	}
	return Snapshot{State: Stuck, Since: since, Reason: stuckReason(quiet), Activity: "thinking…"}
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

func stuckReason(quiet time.Duration) string {
	return fmt.Sprintf("no output for %s mid-turn", ShortDuration(quiet))
}

// questionTrim is the trailing noise a model's closing question may wear:
// whitespace plus markdown decoration.
const questionTrim = " \t\r\n*_`)"

// endsWithQuestion reports whether text closes on a question mark once
// whitespace and markdown decoration are peeled off the right.
func endsWithQuestion(text string) bool {
	t := strings.TrimRight(text, questionTrim)
	return strings.HasSuffix(t, "?")
}

// activityFor renders the one-line hint for a tool call.
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
