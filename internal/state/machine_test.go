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

// assistantTools is one assistant event carrying several calls, as a
// batched turn is written.
func assistantTools(offset time.Duration, uses ...transcript.ToolUse) transcript.Event {
	return transcript.Event{
		Type: transcript.EventAssistant, UUID: "a", SessionID: "s",
		Timestamp: at(offset), ToolUses: uses,
	}
}

func tool(id, name, input string) transcript.ToolUse {
	return transcript.ToolUse{ID: id, Name: name, Input: json.RawMessage(input)}
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
	for _, quiet := range []time.Duration{0, state.ApproveAfter, 10 * time.Minute, 3 * time.Hour} {
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
	assertReason(t, snap, "no result past the Bash timeout")
	if snap.Pending != state.PendingBash {
		t.Errorf("Pending = %q, want %q", snap.Pending, state.PendingBash)
	}
}

// T18 — a tool_result has landed but the model's next message has not been
// written yet: the turn is still in flight, never idle (contract rule 4,
// amended). At 5s this is Working; at 11m still working; at an hour Stuck.
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

	snap = m.Evaluate(at(11 * time.Minute))
	assertState(t, snap, state.Working)
	if !snap.Late || snap.Pending != state.PendingThink {
		t.Errorf("Late/Pending = %v/%q, want true/%q", snap.Late, snap.Pending, state.PendingThink)
	}
	snap = m.Evaluate(at(time.Hour + 10*time.Second))
	assertState(t, snap, state.Stuck)
	if !strings.HasPrefix(snap.Reason, "no output for ") {
		t.Errorf("Reason = %q, want a stuck reason", snap.Reason)
	}

	// The model's next message closes the window: back to plain turn-complete.
	m.Observe(assistantText(2*time.Hour, "All tests pass now."))
	snap = m.Evaluate(at(2*time.Hour + 10*time.Second))
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

// T7 — the same lone prompt, an hour old. At 120s it is still only early.
func TestT07StuckLonePrompt(t *testing.T) {
	assertState(t, evaluateScenario(t, "t06-lone-prompt.jsonl", at(120*time.Second)), state.Working)
	snap := evaluateScenario(t, "t06-lone-prompt.jsonl", at(time.Hour))

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

func TestTheTiersAreTenMinutesAndAnHour(t *testing.T) {
	if state.StillAfter != 10*time.Minute || state.AttnAfter != time.Hour || state.ApproveAfter != 90*time.Second {
		t.Fatalf("tiers = %v / %v / %v, want 10m / 1h / 90s", state.StillAfter, state.AttnAfter, state.ApproveAfter)
	}
}

// An open turn with no budget of its own is judged by its silence: working
// under ten minutes, still working (Late) under an hour, Stuck from the
// hour. The boundaries are strict: the exact threshold is already the next
// tier.
func TestTheTiersJudgeAnOpenTurnBySilence(t *testing.T) {
	tests := []struct {
		name  string
		quiet time.Duration
		want  state.State
		late  bool
	}{
		{"1s", time.Second, state.Working, false},
		{"9m59s", 10*time.Minute - time.Second, state.Working, false},
		{"10m exactly", 10 * time.Minute, state.Working, true},
		{"59m59s", time.Hour - time.Second, state.Working, true},
		{"1h exactly", time.Hour, state.Stuck, false},
		{"3h", 3 * time.Hour, state.Stuck, false},
	}

	for _, tc := range tests {
		check := func(t *testing.T, snap state.Snapshot, pending string) {
			t.Helper()
			assertState(t, snap, tc.want)
			if snap.Late != tc.late || !snap.Open || snap.Pending != pending {
				t.Errorf("Late/Open/Pending = %v/%v/%q, want %v/true/%q", snap.Late, snap.Open, snap.Pending, tc.late, pending)
			}
		}
		t.Run("subagent "+tc.name, func(t *testing.T) {
			m := machineWith(
				userPrompt(0, "refactor the parser"),
				assistantTool(0, "toolu_a", "Agent", `{"prompt":"refactor"}`),
			)
			check(t, m.Evaluate(at(tc.quiet)), state.PendingSubagent)
		})
		t.Run("mcp "+tc.name, func(t *testing.T) {
			m := machineWith(
				userPrompt(0, "file the issue"),
				assistantTool(0, "toolu_m", "mcp__github__issue_write", `{}`),
			)
			check(t, m.Evaluate(at(tc.quiet)), state.PendingMCP)
		})
		t.Run("lone prompt "+tc.name, func(t *testing.T) {
			m := machineWith(userPrompt(0, "build it"))
			check(t, m.Evaluate(at(tc.quiet)), state.PendingStart)
		})
	}
}

// The tiers are the config's.
func TestSetTiers(t *testing.T) {
	t.Cleanup(func() { state.StillAfter, state.AttnAfter = 10*time.Minute, time.Hour })
	state.SetTiers(5*time.Minute, 0)
	if state.StillAfter != 5*time.Minute || state.AttnAfter != time.Hour {
		t.Errorf("SetTiers(5m, 0) = %v/%v, want 5m/1h", state.StillAfter, state.AttnAfter)
	}
	state.SetTiers(2*time.Hour, 30*time.Minute)
	if state.AttnAfter != 2*time.Hour {
		t.Errorf("an attn budget under the still one = %v, want it raised to 2h", state.AttnAfter)
	}
}

// A call that answers at once and has been out ninety seconds is held
// behind a permission prompt: it needs you, not patience.
func TestAFastCallOutNinetySecondsAwaitsApproval(t *testing.T) {
	for _, tool := range []string{"Edit", "Write", "Read", "Grep", "ExitPlanMode"} {
		m := machineWith(
			userPrompt(0, "fix it"),
			assistantTool(10*time.Second, "toolu_e", tool, `{"file_path":"auth.go"}`),
		)
		assertState(t, m.Evaluate(at(10*time.Second+89*time.Second)), state.Working)
		snap := m.Evaluate(at(10*time.Second + state.ApproveAfter))
		assertState(t, snap, state.NeedsYou)
		assertReason(t, snap, "awaiting your approval")
		assertSince(t, snap, at(10*time.Second))
		if snap.Pending != state.PendingApprove || snap.APIError {
			t.Errorf("%s: Pending = %q, want %q", tool, snap.Pending, state.PendingApprove)
		}
	}
}

// An Edit issued beside an Agent call is not what the turn waits on: the
// quiet is the agent's.
func TestAFastCallBesideAnAgentIsNotApproval(t *testing.T) {
	m := machineWith(
		userPrompt(0, "fix it"),
		assistantTools(0, tool("toolu_a", "Agent", `{"prompt":"x"}`), tool("toolu_e", "Read", `{"file_path":"a.go"}`)),
	)
	snap := m.Evaluate(at(5 * time.Minute))
	assertState(t, snap, state.Working)
	if snap.Pending != state.PendingSubagent {
		t.Errorf("Pending = %q, want %q", snap.Pending, state.PendingSubagent)
	}
}

// While an Agent call is out, the subagent's own file is the session's
// clock: the lead's transcript is silent, and the agent writing is the
// session working. A write by an agent while none is out counts for nothing.
func TestASubagentWritingKeepsTheSessionWorking(t *testing.T) {
	m := machineWith(
		userPrompt(0, "refactor the parser"),
		assistantTool(0, "toolu_a", "Agent", `{"prompt":"refactor"}`),
	)
	if ids := m.PendingAgents(); len(ids) != 1 || ids[0] != "toolu_a" {
		t.Fatalf("PendingAgents = %v, want [toolu_a]", ids)
	}
	m.NoteAgentWrite(at(50 * time.Minute))
	snap := m.Evaluate(at(55 * time.Minute))
	if snap.State != state.Working || snap.Late {
		t.Errorf("an agent that wrote 5m ago = %v late=%v, want working", snap.State, snap.Late)
	}
	assertState(t, m.Evaluate(at(115*time.Minute)), state.Stuck)

	idle := machineWith(
		userPrompt(0, "build it"),
		assistantTool(0, "toolu_m", "mcp__db__query", `{}`),
	)
	idle.NoteAgentWrite(at(50 * time.Minute))
	if snap := idle.Evaluate(at(55 * time.Minute)); !snap.Late {
		t.Errorf("an MCP call with a stray agent write = late %v, want the MCP call's own 55m", snap.Late)
	}
}

// Esc closes the turn. Claude Code writes "[Request interrupted by user]"
// as a user line, and read as a prompt it opened a turn nobody would
// answer: an hour on, the session a person had stopped read as hung.
func TestAnInterruptClosesTheTurn(t *testing.T) {
	streaming := machineWith(
		userPrompt(0, "fix the login bug"),
		userPrompt(5*time.Second, "[Request interrupted by user]"),
	)
	duringTool := machineWith(
		userPrompt(0, "run the tests"),
		assistantTool(2*time.Second, "toolu_b", "Bash", `{"command":"npm test"}`),
		toolResult(9*time.Second, "toolu_b"),
		userPrompt(9*time.Second, "[Request interrupted by user for tool use]"),
	)
	for name, m := range map[string]*state.Machine{"streaming": streaming, "during a tool": duringTool} {
		snap := m.Evaluate(at(time.Hour))
		assertState(t, snap, state.Idle)
		assertReason(t, snap, "interrupted")
		if !snap.Interrupted || snap.Open {
			t.Errorf("%s: Interrupted/Open = %v/%v, want true/false", name, snap.Interrupted, snap.Open)
		}
		// The next prompt is a turn again.
		m.Observe(userPrompt(time.Hour, "try again"))
		if snap := m.Evaluate(at(time.Hour + time.Second)); snap.State != state.Working || snap.Interrupted {
			t.Errorf("%s: the next prompt = %v interrupted=%v, want working", name, snap.State, snap.Interrupted)
		}
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

// quotaLine has the shape of a real transcript line — a session that died when its
// gateway refused the call. Claude Code writes a failed API call as a
// synthetic assistant message carrying the error's own text, which is why it
// used to reach the fleet looking exactly like a finished turn.
const quotaLine = `{"type":"assistant","uuid":"8eb5faf2","timestamp":"2026-09-01T07:08:51.279Z",` +
	`"message":{"id":"9d03c5cf","model":"<synthetic>","role":"assistant",` +
	`"stop_reason":"stop_sequence","type":"message","content":[{"type":"text",` +
	`"text":"Please run /login · API Error: 403 your daily quota is exhausted, ` +
	`contact your administrator to increase quota."}],` +
	`"error":"authentication_failed","isApiErrorMessage":true,"apiErrorStatus":403},` +
	`"sessionId":"7cbf7d5c","cwd":"/hdd4/porter","version":"2.1.251","gitBranch":"HEAD"}`

// A call that failed is not a turn that finished. Before this, the synthetic
// message parsed as ordinary assistant text, so rule 4 called it "turn
// complete" and a session dead until tomorrow's quota window sat in the fleet
// wearing the same ○ as one that had done what you asked.
func TestAnAPIErrorIsNotAFinishedTurn(t *testing.T) {
	ev, err := transcript.ParseLine([]byte(quotaLine))
	if err != nil {
		t.Fatalf("the line does not parse: %v", err)
	}
	if !ev.APIError || ev.Status != 403 || ev.ErrorKey != "authentication_failed" {
		t.Fatalf("the error's own fields did not survive parsing: %+v", ev)
	}

	m := state.NewMachine()
	m.Observe(userPrompt(1*time.Minute, "carry on"))
	m.Observe(assistantText(2*time.Minute, "on it — checking the gate"))
	m.Observe(ev)

	got := m.Evaluate(ev.Timestamp.Add(9 * time.Hour))
	if got.State != state.NeedsYou {
		t.Errorf("state is %v, want needs-you: only a person can clear a 403", got.State)
	}
	if !strings.Contains(got.Reason, "403") || !strings.Contains(got.Reason, "authentication_failed") {
		t.Errorf("reason is %q; it should name the API's own status and key", got.Reason)
	}
	if !strings.Contains(got.Activity, "quota is exhausted") {
		t.Errorf("activity is %q; it should carry the words a person recognises", got.Activity)
	}
	// Nine hours on, and still not idle: the verdict does not decay into one.
	if got.State == state.Idle {
		t.Error("the session went idle; nothing about it finished")
	}
}

// The verdict belongs to the beat it came from. A session whose person logs in
// again and gets a real answer is not blocked any more.
func TestARealTurnClearsTheAPIError(t *testing.T) {
	ev, err := transcript.ParseLine([]byte(quotaLine))
	if err != nil {
		t.Fatal(err)
	}

	m := state.NewMachine()
	m.Observe(userPrompt(1*time.Minute, "carry on"))
	m.Observe(ev)
	if got := m.Evaluate(at(5 * time.Minute)); got.State != state.NeedsYou {
		t.Fatalf("state is %v, want needs-you while the call is still refused", got.State)
	}

	m.Observe(userPrompt(10*time.Minute, "logged in, try again"))
	m.Observe(assistantText(11*time.Minute, "back — the gate is clean"))

	got := m.Evaluate(at(12 * time.Minute))
	if got.State != state.Idle {
		t.Errorf("state is %v, want idle: the model answered", got.State)
	}
	if strings.Contains(got.Reason, "403") {
		t.Errorf("reason is %q; the old refusal outlived the turn that replaced it", got.Reason)
	}
}

// An error with no status and no key still says something, and never claims a
// number it does not have.
func TestAnUnlabelledAPIErrorStillReports(t *testing.T) {
	m := state.NewMachine()
	m.Observe(userPrompt(1*time.Minute, "go"))
	m.Observe(transcript.Event{
		Type: transcript.EventAssistant, UUID: "a", SessionID: "s",
		Timestamp: at(2 * time.Minute), Text: "the connection dropped",
		APIError: true,
	})

	got := m.Evaluate(at(3 * time.Minute))
	if got.State != state.NeedsYou {
		t.Errorf("state is %v, want needs-you", got.State)
	}
	if strings.Contains(got.Reason, "0") {
		t.Errorf("reason is %q; it invented a status the line did not carry", got.Reason)
	}
	if got.Activity != "the connection dropped" {
		t.Errorf("activity is %q", got.Activity)
	}
}

// The status is in the reason; spending the first thirty columns of the
// activity restating it costs the words a person recognises, which come last.
func TestTheAPIErrorMarkerIsNotRepeatedInTheActivity(t *testing.T) {
	ev, err := transcript.ParseLine([]byte(quotaLine))
	if err != nil {
		t.Fatal(err)
	}
	m := state.NewMachine()
	m.Observe(userPrompt(1*time.Minute, "carry on"))
	m.Observe(ev)
	got := m.Evaluate(at(5 * time.Minute))

	if strings.Contains(got.Activity, "API Error:") || strings.Contains(got.Activity, "403") {
		t.Errorf("activity %q repeats what the reason %q already says", got.Activity, got.Reason)
	}
	// Both halves survive: Claude Code's own hint, and the gateway's words.
	for _, want := range []string{"Please run /login", "your daily quota is exhausted"} {
		if !strings.Contains(got.Activity, want) {
			t.Errorf("activity %q lost %q", got.Activity, want)
		}
	}
}

// A gateway that says nothing but "API Error: 500" would otherwise be trimmed
// down to nothing at all, leaving the row with an empty second line.
func TestAnAPIErrorWithNoWordsKeepsWhatItHas(t *testing.T) {
	const bare = `{"type":"assistant","uuid":"u1","timestamp":"2026-09-01T07:08:51.279Z",` +
		`"message":{"role":"assistant","content":[{"type":"text","text":"API Error: 500"}],` +
		`"isApiErrorMessage":true,"apiErrorStatus":500},"sessionId":"s1"}`
	ev, err := transcript.ParseLine([]byte(bare))
	if err != nil {
		t.Fatal(err)
	}
	m := state.NewMachine()
	m.Observe(userPrompt(1*time.Minute, "carry on"))
	m.Observe(ev)
	got := m.Evaluate(at(5 * time.Minute))

	if strings.TrimSpace(got.Activity) == "" {
		t.Error("the activity is empty; the row would have nothing to say")
	}
	if !got.APIError {
		t.Error("the snapshot does not admit it is an api error")
	}
}

// "Continue from where you left off." is the harness's line, not the person's.
// Counting it as a prompt started a turn the model might never answer, and a
// resumed-then-abandoned session drifted to stuck on the strength of it.
func TestTheHarnessDoesNotStartATurn(t *testing.T) {
	ev, err := transcript.ParseLine([]byte(`{"type":"user","isMeta":true,"uuid":"c1",` +
		`"timestamp":"2026-09-01T07:00:00.000Z","message":{"role":"user",` +
		`"content":"Continue from where you left off."},"sessionId":"s"}`))
	if err != nil {
		t.Fatal(err)
	}
	m := state.NewMachine()
	m.Observe(ev)
	got := m.Evaluate(ev.Timestamp.Add(10 * time.Minute))
	if got.State != state.Idle {
		t.Errorf("state is %v after ten quiet minutes, want idle: nobody asked for anything", got.State)
	}
}

// An open AskUserQuestion is the question itself, with its options, not the
// tool's name: "AskUserQuestion" on a needs-you row told the person they had
// to go and look; the question tells them what to decide.
func TestAskedQuestionIsTheActivity(t *testing.T) {
	input := `{"questions":[{"question":"Open port 22 to the office CIDR only, or keep the bastion?\nSecond line nobody sees","header":"SSH","options":[{"label":"office CIDR","description":"x"},{"label":"keep bastion","description":"y"}]}]}`
	m := machineWith(
		userPrompt(0, "tighten the security groups"),
		assistantTool(time.Second, "toolu_q", "AskUserQuestion", input),
	)
	snap := m.Evaluate(at(5 * time.Second))
	assertState(t, snap, state.NeedsYou)
	assertActivity(t, snap, "Open port 22 to the office CIDR only, or keep the bastion? [office CIDR / keep bastion]")

	// A malformed or empty input falls back to the tool's name.
	m = machineWith(
		userPrompt(0, "go"),
		assistantTool(time.Second, "toolu_q", "AskUserQuestion", `{"questions":[]}`),
	)
	assertActivity(t, m.Evaluate(at(5*time.Second)), "AskUserQuestion")
}

// A shell command is allowed the budget the harness gave it before its
// silence means anything: "stuck" over a deliberate `sleep 420` was the word
// losing its meaning (#45).
func TestABashInsideItsTimeoutIsWorking(t *testing.T) {
	m := machineWith(
		userPrompt(0, "wait for the run"),
		assistantTool(0, "toolu_x", "Bash", `{"command":"sleep 420","timeout":600000}`),
	)
	snap := m.Evaluate(at(2 * time.Minute))
	assertState(t, snap, state.Working)
	assertReason(t, snap, "Bash allowed 10m")
	if snap.Allowed != 10*time.Minute {
		t.Fatalf("Allowed = %v, want 10m", snap.Allowed)
	}
	// Past the budget the harness has killed it and the silence is real.
	snap = m.Evaluate(at(11 * time.Minute))
	assertState(t, snap, state.Stuck)
	if snap.Allowed != 0 {
		t.Fatalf("Allowed = %v past the budget, want 0", snap.Allowed)
	}
}

// A Bash that names no timeout has the harness's two minutes; ninety
// seconds of silence inside them is still working, and past them stuck.
func TestABashWithNoTimeoutHasTwoMinutes(t *testing.T) {
	m := machineWith(
		userPrompt(0, "build it"),
		assistantTool(0, "toolu_x", "Bash", `{"command":"go build ./..."}`),
	)
	assertState(t, m.Evaluate(at(100*time.Second)), state.Working)
	assertState(t, m.Evaluate(at(3*time.Minute)), state.Stuck)
}

// The budget is the latest of every call in flight: Claude Code batches
// calls, and a Read issued beside the sleep is the oldest pending one (#51).
func TestTheBudgetIsTheLatestOfEveryPendingCall(t *testing.T) {
	m := machineWith(
		userPrompt(0, "wait for the run"),
		assistantTools(0, tool("toolu_r", "Read", `{"file_path":"main.go"}`), tool("toolu_b", "Bash", `{"command":"sleep 420","timeout":600000}`)),
	)
	for _, tc := range []struct {
		at   time.Duration
		want state.State
	}{{100 * time.Second, state.Working}, {5 * time.Minute, state.Working}, {11 * time.Minute, state.Stuck}} {
		snap := m.Evaluate(at(tc.at))
		assertState(t, snap, tc.want)
		if tc.want == state.Working && snap.Allowed != 10*time.Minute {
			t.Errorf("at %v Allowed = %v, want 10m", tc.at, snap.Allowed)
		}
	}
	// The budget is carried from the first second, so the row never blinks.
	if snap := m.Evaluate(at(10 * time.Second)); snap.Allowed != 10*time.Minute {
		t.Errorf("before the silence threshold Allowed = %v, want 10m", snap.Allowed)
	}
}

// The harness grants ten minutes at most, and a background call nothing:
// it returns at once, and its budget is not a promise the harness keeps.
func TestTheBudgetIsCappedAndABackgroundCallHasNone(t *testing.T) {
	m := machineWith(
		userPrompt(0, "build it"),
		assistantTool(0, "toolu_x", "Bash", `{"command":"make","timeout":3600000}`),
	)
	if snap := m.Evaluate(at(5 * time.Minute)); snap.State != state.Working || snap.Allowed != 10*time.Minute {
		t.Errorf("an hour's timeout = %v %v, want working with 10m", snap.State, snap.Allowed)
	}
	assertState(t, m.Evaluate(at(12*time.Minute)), state.Stuck)
	bg := machineWith(
		userPrompt(0, "build it"),
		assistantTool(0, "toolu_y", "Bash", `{"command":"make","run_in_background":true,"timeout":600000}`),
	)
	assertState(t, bg.Evaluate(at(3*time.Minute)), state.Stuck)
}

// The row counts from the call the budget belongs to: a Read pending since
// the turn began beside a sleep sent forty minutes in says "for 5m of
// 10m", not "for 45m of 10m" (#52).
func TestTheBudgetsClockIsItsOwnCalls(t *testing.T) {
	m := machineWith(
		userPrompt(0, "wait for the run"),
		assistantTool(0, "toolu_r", "Read", `{"file_path":"main.go"}`),
		assistantTool(40*time.Minute, "toolu_b", "Bash", `{"command":"sleep 420","timeout":600000}`),
	)
	snap := m.Evaluate(at(45 * time.Minute))
	assertState(t, snap, state.Working)
	if !snap.Since.Equal(at(40 * time.Minute)) {
		t.Errorf("Since = %v, want the sleep's own start %v", snap.Since, at(40*time.Minute))
	}
}
