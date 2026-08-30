package state_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/transcript"
)

// base is the instant every testdata/scenarios/*.jsonl fixture hangs off; all
// offsets below are relative to it so tests never touch the wall clock.
var base = time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

func at(offset time.Duration) time.Time { return base.Add(offset) }

// ---------------------------------------------------------------- helpers

func loadScenario(t *testing.T, name string) []transcript.Event {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "scenarios", name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read scenario %s: %v", name, err)
	}
	var evs []transcript.Event
	for i, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		ev, err := transcript.ParseLine([]byte(line))
		if err != nil {
			t.Fatalf("%s line %d: ParseLine: %v", name, i+1, err)
		}
		evs = append(evs, ev)
	}
	if len(evs) == 0 {
		t.Fatalf("scenario %s parsed to zero events", name)
	}
	return evs
}

func machineWith(evs ...transcript.Event) *state.Machine {
	m := state.NewMachine()
	for _, ev := range evs {
		m.Observe(ev)
	}
	return m
}

func evaluateScenario(t *testing.T, name string, now time.Time) state.Snapshot {
	t.Helper()
	return machineWith(loadScenario(t, name)...).Evaluate(now)
}

// Event builders for the tests that need precise timing rather than a fixture.

func userPrompt(offset time.Duration, text string) transcript.Event {
	return transcript.Event{
		Type: transcript.EventUser, UUID: "u", SessionID: "s",
		Timestamp: at(offset), Text: text,
	}
}

func assistantText(offset time.Duration, text string) transcript.Event {
	return transcript.Event{
		Type: transcript.EventAssistant, UUID: "a", SessionID: "s",
		Timestamp: at(offset), Text: text,
	}
}

func assistantTool(offset time.Duration, id, name, input string) transcript.Event {
	return transcript.Event{
		Type: transcript.EventAssistant, UUID: "a", SessionID: "s",
		Timestamp: at(offset),
		ToolUses:  []transcript.ToolUse{{ID: id, Name: name, Input: json.RawMessage(input)}},
	}
}

func toolResult(offset time.Duration, id string) transcript.Event {
	return transcript.Event{
		Type: transcript.EventUser, UUID: "u", SessionID: "s",
		Timestamp:   at(offset),
		ToolResults: []transcript.ToolResult{{ToolUseID: id}},
	}
}

func nonSubstantive(typ transcript.EventType, offset time.Duration) transcript.Event {
	return transcript.Event{Type: typ, UUID: "x", SessionID: "s", Timestamp: at(offset)}
}

func assertState(t *testing.T, got state.Snapshot, want state.State) {
	t.Helper()
	if got.State != want {
		t.Errorf("State = %s, want %s [reason=%q activity=%q]",
			got.State, want, got.Reason, got.Activity)
	}
}

func assertReason(t *testing.T, got state.Snapshot, want string) {
	t.Helper()
	if got.Reason != want {
		t.Errorf("Reason = %q, want %q", got.Reason, want)
	}
}

func assertSince(t *testing.T, got state.Snapshot, want time.Time) {
	t.Helper()
	if !got.Since.Equal(want) {
		t.Errorf("Since = %v, want %v", got.Since, want)
	}
}

func assertActivity(t *testing.T, got state.Snapshot, want string) {
	t.Helper()
	if got.Activity != want {
		t.Errorf("Activity = %q, want %q", got.Activity, want)
	}
}

// ---------------------------------------------------------------- T1–T7, T17

// T1 — an assistant tool_use (Bash) is pending and the session went quiet 5s ago.
func TestT01WorkingToolPending(t *testing.T) {
	snap := evaluateScenario(t, "t01-working-bash.jsonl", at(10*time.Second))

	assertState(t, snap, state.Working)
	assertActivity(t, snap, "Bash: pytest tests/auth -x")
	assertSince(t, snap, at(5*time.Second)) // the tool_use opened the wait
}

// T2 — the turn is complete and the closing text ends with a question.
func TestT02NeedsYouTurnEndedWithQuestion(t *testing.T) {
	snap := evaluateScenario(t, "t02-question.jsonl", at(20*time.Second))

	assertState(t, snap, state.NeedsYou)
	assertReason(t, snap, "turn ended with a question")
	assertSince(t, snap, at(8*time.Second)) // the closing assistant event
}

// T3 — a pending AskUserQuestion is NeedsYou immediately, with no quiet threshold.
func TestT03NeedsYouPendingAskUserQuestion(t *testing.T) {
	snap := evaluateScenario(t, "t03-ask-user-question.jsonl", at(4*time.Second))

	assertState(t, snap, state.NeedsYou)
	assertReason(t, snap, "waiting on your answer")
	assertSince(t, snap, at(4*time.Second)) // the tool_use that opened the wait
}

// Precedence: rule 2 outranks rule 3, so a long-unanswered question is still
// NeedsYou and never decays into Stuck.
func TestT03AskUserQuestionBeatsQuietTimeout(t *testing.T) {
	for _, quiet := range []time.Duration{0, state.StuckAfter, 10 * time.Minute, 3 * time.Hour} {
		now := at(4*time.Second + quiet)
		snap := evaluateScenario(t, "t03-ask-user-question.jsonl", now)

		if snap.State != state.NeedsYou {
			t.Errorf("after %v of quiet: State = %s, want needs-you (rule 2 has no threshold)",
				quiet, snap.State)
		}
		assertReason(t, snap, "waiting on your answer")
	}
}

// T4 — the turn is complete and the closing text is a statement.
func TestT04IdleTurnComplete(t *testing.T) {
	snap := evaluateScenario(t, "t04-turn-complete.jsonl", at(30*time.Second))

	assertState(t, snap, state.Idle)
	assertReason(t, snap, "turn complete")
	assertActivity(t, snap, "idle")
	assertSince(t, snap, at(6*time.Second)) // the closing assistant event
}

// T5 — a pending Bash with 120s of silence is Stuck.
func TestT05StuckPendingBash(t *testing.T) {
	snap := evaluateScenario(t, "t05-stuck-bash.jsonl", at(124*time.Second))

	assertState(t, snap, state.Stuck)
	assertSince(t, snap, at(4*time.Second)) // the tool_use that opened the wait

	if !strings.HasPrefix(snap.Reason, "no output for ") || !strings.HasSuffix(snap.Reason, " mid-turn") {
		t.Errorf("Reason = %q, want the shape %q", snap.Reason, "no output for <dur> mid-turn")
	}
	if !strings.Contains(snap.Reason, "2m") {
		t.Errorf("Reason = %q, want it to report the 2m of silence", snap.Reason)
	}
}

// T18 — a tool_result has landed but the model's next message has not been
// written yet: the turn is still in flight, never idle (contract rule 4,
// amended). At 5s this is Working; at 120s it is Stuck.
func TestT18ProcessingResultsIsNotIdle(t *testing.T) {
	m := machineWith(
		userPrompt(0, "fix the failing test"),
		assistantTool(2*time.Second, "tu_1", "Bash", `{"command":"pytest -x"}`),
		toolResult(10*time.Second, "tu_1"),
	)

	snap := m.Evaluate(at(15 * time.Second))
	assertState(t, snap, state.Working)
	assertReason(t, snap, "processing results")
	assertActivity(t, snap, "thinking…")
	assertSince(t, snap, at(10*time.Second)) // the result that reopened the wait

	snap = m.Evaluate(at(130 * time.Second))
	assertState(t, snap, state.Stuck)
	if !strings.HasPrefix(snap.Reason, "no output for ") {
		t.Errorf("Reason = %q, want a stuck reason", snap.Reason)
	}

	// The model's next message closes the window: back to plain turn-complete.
	m.Observe(assistantText(140*time.Second, "All tests pass now."))
	snap = m.Evaluate(at(150 * time.Second))
	assertState(t, snap, state.Idle)
	assertReason(t, snap, "turn complete")
}

// T6 — a lone user prompt 3s old: the model has not replied yet, but it is early.
func TestT06WorkingStartingTurn(t *testing.T) {
	snap := evaluateScenario(t, "t06-lone-prompt.jsonl", at(3*time.Second))

	assertState(t, snap, state.Working)
	assertReason(t, snap, "starting turn")
	assertSince(t, snap, base) // the prompt opened the wait
}

// T7 — the same lone prompt, 120s old.
func TestT07StuckLonePrompt(t *testing.T) {
	snap := evaluateScenario(t, "t06-lone-prompt.jsonl", at(120*time.Second))

	assertState(t, snap, state.Stuck)
	assertSince(t, snap, base)
	if !strings.HasPrefix(snap.Reason, "no output for ") {
		t.Errorf("Reason = %q, want it to start with %q", snap.Reason, "no output for ")
	}
}

// T17 — the closing text is a markdown-decorated question.
func TestT17NeedsYouMarkdownDecoratedQuestion(t *testing.T) {
	snap := evaluateScenario(t, "t17-markdown-question.jsonl", at(30*time.Second))

	assertState(t, snap, state.NeedsYou)
	assertReason(t, snap, "turn ended with a question")
	assertSince(t, snap, at(5*time.Second))
}

// The trim rule in detail: whitespace and the markdown decorations `*`, `_`,
// backtick and `)` come off the right before the `?` test.
func TestQuestionDetectionTrimsMarkdownDecoration(t *testing.T) {
	tests := []struct {
		name string
		text string
		want state.State
	}{
		{"plain question", "Shall I proceed?", state.NeedsYou},
		{"bold", "**ok?**", state.NeedsYou},
		{"underscore", "__ready to merge?__", state.NeedsYou},
		{"code fence tail", "done?`", state.NeedsYou},
		{"trailing whitespace", "want me to continue?  \n", state.NeedsYou},
		{"parenthesised italic", "*(want me to continue?)*", state.NeedsYou},
		{"mixed decoration", "Proceed?`_*", state.NeedsYou},
		{"statement", "Done. The parser handles empty input.", state.Idle},
		{"bold statement", "**All green.**", state.Idle},
		{"trailing paren is not a question", "See the fix in (parser.go)", state.Idle},
		{"decoration only", "```", state.Idle},
		{"question mark mid-sentence", "The ? glyph is now escaped.", state.Idle},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := machineWith(
				userPrompt(0, "have a look"),
				assistantText(5*time.Second, tc.text),
			)
			snap := m.Evaluate(at(30 * time.Second))

			assertState(t, snap, tc.want)
			if tc.want == state.NeedsYou {
				assertReason(t, snap, "turn ended with a question")
			} else {
				assertReason(t, snap, "turn complete")
			}
		})
	}
}

// ---------------------------------------------------------------- thresholds

func TestStuckAfterIsNinetySeconds(t *testing.T) {
	if state.StuckAfter != 90*time.Second {
		t.Fatalf("StuckAfter = %v, want 90s", state.StuckAfter)
	}
}

// The boundary is strict: `now - lastEventAt < StuckAfter` is Working, and the
// exact threshold is already Stuck.
func TestStuckAfterBoundary(t *testing.T) {
	tests := []struct {
		name  string
		quiet time.Duration
		want  state.State
	}{
		{"1s", time.Second, state.Working},
		{"89s", 89 * time.Second, state.Working},
		{"90s exactly", 90 * time.Second, state.Stuck},
		{"91s", 91 * time.Second, state.Stuck},
	}

	for _, tc := range tests {
		t.Run("pending tool "+tc.name, func(t *testing.T) {
			m := machineWith(
				userPrompt(0, "build it"),
				assistantTool(0, "toolu_x", "Bash", `{"command":"go build ./..."}`),
			)
			assertState(t, m.Evaluate(at(tc.quiet)), tc.want)
		})

		t.Run("lone prompt "+tc.name, func(t *testing.T) {
			m := machineWith(userPrompt(0, "build it"))
			snap := m.Evaluate(at(tc.quiet))
			assertState(t, snap, tc.want)
			if tc.want == state.Working {
				assertReason(t, snap, "starting turn")
			}
		})
	}
}

// ---------------------------------------------------------------- rules 1 & 3

func TestNoSubstantiveEventsIsIdle(t *testing.T) {
	t.Run("no events at all", func(t *testing.T) {
		snap := state.NewMachine().Evaluate(at(time.Hour))
		assertState(t, snap, state.Idle)
		assertReason(t, snap, "no activity yet")
		assertActivity(t, snap, "idle")
	})

	t.Run("only non-substantive events", func(t *testing.T) {
		m := machineWith(
			nonSubstantive(transcript.EventQueueOp, 0),
			nonSubstantive(transcript.EventAttachment, 1*time.Second),
			nonSubstantive(transcript.EventUnknown, 2*time.Second),
			toolResult(3*time.Second, "toolu_orphan"), // user line with no Text
		)
		snap := m.Evaluate(at(4 * time.Second))
		assertState(t, snap, state.Idle)
		assertReason(t, snap, "no activity yet")
	})
}

// Non-substantive lines do not start a turn, but they DO prove the session is
// alive, so they push lastEventAt forward and hold off Stuck.
func TestNonSubstantiveEventsRefreshLastEventAt(t *testing.T) {
	m := machineWith(
		userPrompt(0, "run the build"),
		assistantTool(0, "toolu_x", "Bash", `{"command":"go build ./..."}`),
		nonSubstantive(transcript.EventAttachment, 80*time.Second),
	)

	// 140s after the tool_use, but only 60s after the last sign of life.
	snap := m.Evaluate(at(140 * time.Second))
	assertState(t, snap, state.Working)
}

func TestMatchingToolResultClearsPending(t *testing.T) {
	m := machineWith(
		userPrompt(0, "fix the crash"),
		assistantTool(2*time.Second, "toolu_edit", "Edit", `{"file_path":"/x/parser.go"}`),
		toolResult(3*time.Second, "toolu_edit"),
		assistantText(6*time.Second, "Done. No more panic."),
	)

	// Long past StuckAfter: a completed turn never rots into Stuck.
	snap := m.Evaluate(at(30 * time.Minute))
	assertState(t, snap, state.Idle)
	assertReason(t, snap, "turn complete")
}

func TestUnmatchedToolResultLeavesOtherToolPending(t *testing.T) {
	m := machineWith(
		userPrompt(0, "look around"),
		assistantTool(1*time.Second, "toolu_a", "Read", `{"file_path":"/x/a.go"}`),
		assistantTool(2*time.Second, "toolu_b", "Bash", `{"command":"ls"}`),
		toolResult(3*time.Second, "toolu_a"),
	)

	snap := m.Evaluate(at(10 * time.Second))
	assertState(t, snap, state.Working)
	assertActivity(t, snap, "Bash: ls")
}

// ---------------------------------------------------------------- activity

func TestActivityFromMostRecentToolUse(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		input string
		want  string
	}{
		{"bash", "Bash", `{"command":"pytest tests/auth -x","description":"tests"}`, "Bash: pytest tests/auth -x"},
		{"read", "Read", `{"file_path":"/home/user/compass/internal/http/middleware.py"}`, "reading middleware.py"},
		{"edit", "Edit", `{"file_path":"/home/user/compass/internal/parse/parser.go"}`, "editing parser.go"},
		{"write", "Write", `{"file_path":"/home/user/compass/README.md"}`, "writing README.md"},
		{"other tool falls back to its name", "Grep", `{"pattern":"func "}`, "Grep"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := machineWith(
				userPrompt(0, "go"),
				assistantTool(time.Second, "toolu_x", tc.tool, tc.input),
			)
			assertActivity(t, m.Evaluate(at(5*time.Second)), tc.want)
		})
	}
}

func TestActivityBashIsFirstLineAndBounded(t *testing.T) {
	const cmd = "go test ./... -run TestEverythingUnderTheSunAndThenSome -v\nnever-shown second line"
	input, err := json.Marshal(map[string]string{"command": cmd})
	if err != nil {
		t.Fatal(err)
	}

	m := machineWith(
		userPrompt(0, "run everything"),
		assistantTool(time.Second, "toolu_x", "Bash", string(input)),
	)
	act := m.Evaluate(at(5 * time.Second)).Activity

	if !strings.HasPrefix(act, "Bash: ") {
		t.Fatalf("Activity = %q, want it to start with %q", act, "Bash: ")
	}
	rest := strings.TrimPrefix(act, "Bash: ")
	if strings.ContainsAny(act, "\n\r") {
		t.Errorf("Activity = %q, want a single line", act)
	}
	if strings.Contains(act, "never-shown second line") {
		t.Errorf("Activity = %q, want only the first line of the command", act)
	}
	if n := utf8.RuneCountInString(rest); n > 40 {
		t.Errorf("Activity command part is %d cols (%q), want at most 40", n, rest)
	}
	if !strings.HasPrefix(rest, "go test ./...") {
		t.Errorf("Activity = %q, want the head of the command preserved", act)
	}
}

// ---------------------------------------------------------------- State

func TestStateString(t *testing.T) {
	tests := map[state.State]string{
		state.Working:  "working",
		state.NeedsYou: "needs-you",
		state.Idle:     "idle",
		state.Stuck:    "stuck",
	}
	for st, want := range tests {
		if got := st.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", int(st), got, want)
		}
	}
}
