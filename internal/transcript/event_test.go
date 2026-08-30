package transcript_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/transcript"
)

// testdataPath resolves a path under the repo-root testdata/ directory
// (docs/dev/M0-CONTRACT.md "Repo layout": fixtures live at <repo>/testdata).
func testdataPath(rel string) string {
	return filepath.Join("..", "..", "testdata", filepath.FromSlash(rel))
}

// fixtureLines returns the non-empty lines of a JSONL fixture, newline included,
// so tests can replay a realistic file byte-for-byte.
func fixtureLines(t *testing.T, rel string) []string {
	t.Helper()
	raw, err := os.ReadFile(testdataPath(rel))
	if err != nil {
		t.Fatalf("read fixture %s: %v", rel, err)
	}
	var out []string
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		out = append(out, l+"\n")
	}
	return out
}

func mustParse(t *testing.T, line string) transcript.Event {
	t.Helper()
	ev, err := transcript.ParseLine([]byte(line))
	if err != nil {
		t.Fatalf("ParseLine returned error for a well-formed line: %v\nline: %s", err, line)
	}
	return ev
}

// stamp is the fixture time base (all testdata timestamps hang off it).
func stamp(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("bad timestamp literal %q in test: %v", s, err)
	}
	return ts
}

// ---------------------------------------------------------------- line shapes

func TestParseLineUserPromptCommonFields(t *testing.T) {
	line := `{"parentUuid":null,"isSidechain":false,"promptId":"p-aaaa1111","type":"user",` +
		`"message":{"role":"user","content":"run the auth tests"},` +
		`"uuid":"aaaa1111-0001-4000-8000-000000000001","timestamp":"2026-08-30T12:00:00.000Z",` +
		`"permissionMode":"default","origin":"cli","promptSource":"user","userType":"external",` +
		`"entrypoint":"remote_desktop","cwd":"/home/user/compass",` +
		`"sessionId":"11111111-1111-4111-8111-111111111111","version":"2.1.251","gitBranch":"feat/auth"}`

	ev := mustParse(t, line)

	if ev.Type != transcript.EventUser {
		t.Errorf("Type = %q, want %q", ev.Type, transcript.EventUser)
	}
	if ev.Text != "run the auth tests" {
		t.Errorf("Text = %q, want %q", ev.Text, "run the auth tests")
	}
	if ev.UUID != "aaaa1111-0001-4000-8000-000000000001" {
		t.Errorf("UUID = %q", ev.UUID)
	}
	if ev.ParentUUID != "" {
		t.Errorf("ParentUUID = %q, want empty for a null parentUuid", ev.ParentUUID)
	}
	if want := stamp(t, "2026-08-30T12:00:00.000Z"); !ev.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", ev.Timestamp, want)
	}
	if ev.SessionID != "11111111-1111-4111-8111-111111111111" {
		t.Errorf("SessionID = %q", ev.SessionID)
	}
	if ev.CWD != "/home/user/compass" {
		t.Errorf("CWD = %q", ev.CWD)
	}
	if ev.GitBranch != "feat/auth" {
		t.Errorf("GitBranch = %q", ev.GitBranch)
	}
	if ev.Version != "2.1.251" {
		t.Errorf("Version = %q", ev.Version)
	}
	if ev.IsSidechain {
		t.Error("IsSidechain = true, want false")
	}
	if len(ev.ToolUses) != 0 || len(ev.ToolResults) != 0 {
		t.Errorf("plain prompt carried tools: uses=%d results=%d", len(ev.ToolUses), len(ev.ToolResults))
	}
}

func TestParseLineIsSidechainTrue(t *testing.T) {
	line := `{"parentUuid":"p1","isSidechain":true,"type":"user",` +
		`"message":{"role":"user","content":"scout the loader"},"uuid":"u1",` +
		`"timestamp":"2026-08-30T12:00:00.000Z","sessionId":"s1"}`

	ev := mustParse(t, line)
	if !ev.IsSidechain {
		t.Error("IsSidechain = false, want true")
	}
	if ev.ParentUUID != "p1" {
		t.Errorf("ParentUUID = %q, want %q", ev.ParentUUID, "p1")
	}
}

func TestParseLineAssistantJoinsTextBlocksAndIgnoresThinking(t *testing.T) {
	line := `{"parentUuid":"u1","isSidechain":false,"requestId":"req_1","type":"assistant",` +
		`"message":{"model":"claude-fable-5","id":"msg_01","type":"message","role":"assistant",` +
		`"content":[{"type":"thinking","thinking":"secret scratch work","signature":"c2ln"},` +
		`{"type":"text","text":"first block"},` +
		`{"type":"text","text":"second block"}],` +
		`"stop_reason":"end_turn","stop_sequence":null,"stop_details":null},` +
		`"uuid":"s1","timestamp":"2026-08-30T12:00:03.000Z","effort":"high",` +
		`"cwd":"/home/user/compass","sessionId":"s","version":"2.1.251","gitBranch":"main"}`

	ev := mustParse(t, line)

	if ev.Type != transcript.EventAssistant {
		t.Errorf("Type = %q, want %q", ev.Type, transcript.EventAssistant)
	}
	if want := "first block\nsecond block"; ev.Text != want {
		t.Errorf("Text = %q, want %q (text blocks joined with \\n, thinking ignored)", ev.Text, want)
	}
	if strings.Contains(ev.Text, "secret scratch work") {
		t.Error("Text included a thinking block; the contract says thinking is ignored")
	}
}

func TestParseLineAssistantToolUse(t *testing.T) {
	line := fixtureLines(t, "scenarios/t01-working-bash.jsonl")[3]
	ev := mustParse(t, line)

	if ev.Type != transcript.EventAssistant {
		t.Fatalf("Type = %q, want %q", ev.Type, transcript.EventAssistant)
	}
	if ev.Text != "" {
		t.Errorf("Text = %q, want empty for a tool_use-only assistant line", ev.Text)
	}
	if len(ev.ToolUses) != 1 {
		t.Fatalf("len(ToolUses) = %d, want 1", len(ev.ToolUses))
	}
	tu := ev.ToolUses[0]
	if tu.ID != "toolu_01T01Bash" {
		t.Errorf("ToolUses[0].ID = %q", tu.ID)
	}
	if tu.Name != "Bash" {
		t.Errorf("ToolUses[0].Name = %q, want Bash", tu.Name)
	}
	var input struct {
		Command     string `json:"command"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(tu.Input, &input); err != nil {
		t.Fatalf("ToolUses[0].Input is not the raw tool input object: %v (raw=%s)", err, tu.Input)
	}
	if input.Command != "pytest tests/auth -x" {
		t.Errorf("Input.command = %q, want %q", input.Command, "pytest tests/auth -x")
	}
}

func TestParseLineAssistantToolUsesKeepFileOrder(t *testing.T) {
	line := `{"type":"assistant","uuid":"s1","timestamp":"2026-08-30T12:00:00.000Z","sessionId":"s",` +
		`"message":{"role":"assistant","content":[` +
		`{"type":"tool_use","id":"toolu_a","name":"Read","input":{"file_path":"/x/a.go"}},` +
		`{"type":"tool_use","id":"toolu_b","name":"Bash","input":{"command":"ls"}}]}}`

	ev := mustParse(t, line)
	if len(ev.ToolUses) != 2 {
		t.Fatalf("len(ToolUses) = %d, want 2", len(ev.ToolUses))
	}
	if ev.ToolUses[0].ID != "toolu_a" || ev.ToolUses[1].ID != "toolu_b" {
		t.Errorf("ToolUses out of order: %q, %q", ev.ToolUses[0].ID, ev.ToolUses[1].ID)
	}
}

// T11 — a user-type line whose message.content is a block array of tool_result.
func TestT11ParseLineUserToolResultBlocks(t *testing.T) {
	line := `{"parentUuid":"s1","isSidechain":false,"promptId":"p-1","type":"user",` +
		`"message":{"role":"user","content":[{"type":"tool_result",` +
		`"content":"Exit code 128\nfatal: no commits yet","is_error":true,` +
		`"tool_use_id":"toolu_01T01Bash"}]},` +
		`"uuid":"u2","timestamp":"2026-08-30T12:00:09.000Z",` +
		`"toolUseResult":"Error: Exit code 128","sourceToolAssistantUUID":"s1",` +
		`"cwd":"/home/user/compass","sessionId":"s","version":"2.1.251","gitBranch":"main"}`

	ev := mustParse(t, line)

	if ev.Type != transcript.EventUser {
		t.Errorf("Type = %q, want %q", ev.Type, transcript.EventUser)
	}
	if ev.Text != "" {
		t.Errorf("Text = %q, want empty when user content is a block array", ev.Text)
	}
	if len(ev.ToolResults) != 1 {
		t.Fatalf("len(ToolResults) = %d, want 1", len(ev.ToolResults))
	}
	if got := ev.ToolResults[0].ToolUseID; got != "toolu_01T01Bash" {
		t.Errorf("ToolResults[0].ToolUseID = %q, want %q", got, "toolu_01T01Bash")
	}
	if !ev.ToolResults[0].IsError {
		t.Error("ToolResults[0].IsError = false, want true (is_error was true)")
	}
}

func TestParseLineSuccessfulToolResultIsNotAnError(t *testing.T) {
	line := fixtureLines(t, "scenarios/t02-question.jsonl")[2]
	ev := mustParse(t, line)

	if len(ev.ToolResults) != 1 {
		t.Fatalf("len(ToolResults) = %d, want 1", len(ev.ToolResults))
	}
	if ev.ToolResults[0].IsError {
		t.Error("ToolResults[0].IsError = true, want false for a successful tool_result")
	}
}

// T12 — an unrecognised top-level type must fail soft, not error.
func TestT12ParseLineUnknownTypeAtisLatch(t *testing.T) {
	line := `{"type":"atis-latch","atis":"","sessionId":"88888888-8888-4888-8888-888888888888"}`

	ev, err := transcript.ParseLine([]byte(line))
	if err != nil {
		t.Fatalf("ParseLine(atis-latch) returned error %v, want nil (unknown types fail soft)", err)
	}
	if ev.Type != transcript.EventUnknown {
		t.Errorf("Type = %q, want %q", ev.Type, transcript.EventUnknown)
	}
	if ev.SessionID != "88888888-8888-4888-8888-888888888888" {
		t.Errorf("SessionID = %q, want the common field to still be parsed", ev.SessionID)
	}
	if ev.Text != "" {
		t.Errorf("Text = %q, want empty for an unknown-type line", ev.Text)
	}
}

func TestParseLineQueueOperation(t *testing.T) {
	line := `{"type":"queue-operation","operation":"enqueue","timestamp":"2026-08-30T12:00:00.000Z",` +
		`"sessionId":"88888888-8888-4888-8888-888888888888","content":"look at the tailer"}`

	ev := mustParse(t, line)

	if ev.Type != transcript.EventQueueOp {
		t.Errorf("Type = %q, want %q", ev.Type, transcript.EventQueueOp)
	}
	if ev.SessionID != "88888888-8888-4888-8888-888888888888" {
		t.Errorf("SessionID = %q", ev.SessionID)
	}
	if want := stamp(t, "2026-08-30T12:00:00.000Z"); !ev.Timestamp.Equal(want) {
		t.Errorf("Timestamp = %v, want %v", ev.Timestamp, want)
	}
	// NOTE: the contract does not say whether a queue-operation's top-level
	// `content` string lands in Event.Text, so this test deliberately does not
	// assert on Text.
}

func TestParseLineAttachment(t *testing.T) {
	line := fixtureLines(t, "scenarios/t01-working-bash.jsonl")[1]
	ev := mustParse(t, line)

	if ev.Type != transcript.EventAttachment {
		t.Errorf("Type = %q, want %q", ev.Type, transcript.EventAttachment)
	}
	if ev.Text != "" {
		t.Errorf("Text = %q, want empty for an attachment line", ev.Text)
	}
	if ev.CWD != "/home/user/compass" {
		t.Errorf("CWD = %q, want the common field to be parsed on attachments too", ev.CWD)
	}
}

// ---------------------------------------------------------------- fail-soft

func TestParseLineInvalidJSONReturnsError(t *testing.T) {
	for _, line := range []string{
		`{"type":"user"`,
		`not json at all`,
		`{"type":"user","message":}`,
	} {
		if _, err := transcript.ParseLine([]byte(line)); err == nil {
			t.Errorf("ParseLine(%q) returned nil error, want an error for invalid JSON", line)
		}
	}
}

func TestParseLineMissingOrUnparseableTimestampIsZero(t *testing.T) {
	tests := map[string]string{
		"absent":      `{"type":"user","uuid":"u1","message":{"role":"user","content":"hi"},"sessionId":"s"}`,
		"unparseable": `{"type":"user","uuid":"u1","timestamp":"not-a-time","message":{"role":"user","content":"hi"},"sessionId":"s"}`,
	}
	for name, line := range tests {
		t.Run(name, func(t *testing.T) {
			ev, err := transcript.ParseLine([]byte(line))
			if err != nil {
				t.Fatalf("ParseLine returned error %v, want nil", err)
			}
			if !ev.Timestamp.IsZero() {
				t.Errorf("Timestamp = %v, want the zero time", ev.Timestamp)
			}
			if ev.Text != "hi" {
				t.Errorf("Text = %q, want %q — a bad timestamp must not lose the rest", ev.Text, "hi")
			}
		})
	}
}

// A realistic mixed file: every shape a live transcript contains must map to a
// known EventType without a single error.
func TestParseLineShapesFixtureFailsSoft(t *testing.T) {
	lines := fixtureLines(t, "scenarios/shapes.jsonl")
	if len(lines) != 11 {
		t.Fatalf("fixture shapes.jsonl has %d lines, want 11", len(lines))
	}

	want := []transcript.EventType{
		transcript.EventQueueOp,    // queue-operation
		transcript.EventUser,       // user prompt (string content)
		transcript.EventAttachment, // attachment
		transcript.EventUnknown,    // mode
		transcript.EventAssistant,  // assistant thinking-only
		transcript.EventAssistant,  // assistant text
		transcript.EventAssistant,  // assistant tool_use
		transcript.EventUser,       // user tool_result blocks
		transcript.EventUnknown,    // atis-latch
		transcript.EventUnknown,    // last-prompt
		transcript.EventAssistant,  // assistant text
	}

	for i, line := range lines {
		ev, err := transcript.ParseLine([]byte(line))
		if err != nil {
			t.Fatalf("line %d: ParseLine returned error %v (no real transcript line may error)", i, err)
		}
		if ev.Type != want[i] {
			t.Errorf("line %d: Type = %q, want %q", i, ev.Type, want[i])
		}
	}
}
