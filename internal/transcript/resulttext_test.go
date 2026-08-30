package transcript_test

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/deephanson94/compass/internal/transcript"
)

// ---------------------------------------------------------------- T35 helpers
//
// These cover ToolResult.Text: "the result's text content (plain string or
// joined text blocks), clamped to 2048 bytes at each end with a `\n…\n` elision
// line, cut on rune boundaries" (docs/dev/M2-CONTRACT.md, package transcript).

// resultTextCap mirrors the contract's per-end byte budget.
const resultTextCap = 2048

// elision is the marker that replaces the middle of an over-long result.
const elision = "\n…\n"

// oneResult parses a user line carrying exactly one tool_result whose `content`
// is the raw JSON passed in, and hands back that result. An empty rawContent
// omits the `content` field entirely.
func oneResult(t *testing.T, rawContent string) transcript.ToolResult {
	t.Helper()
	block := `{"type":"tool_result","tool_use_id":"toolu_1","is_error":false`
	if rawContent != "" {
		block += `,"content":` + rawContent
	}
	block += `}`
	line := `{"parentUuid":"s1","isSidechain":false,"type":"user",` +
		`"message":{"role":"user","content":[` + block + `]},` +
		`"uuid":"u1","timestamp":"2026-08-30T12:00:00.000Z","sessionId":"s"}`

	ev := mustParse(t, line)
	if len(ev.ToolResults) != 1 {
		t.Fatalf("len(ToolResults) = %d, want 1\nline: %s", len(ev.ToolResults), line)
	}
	return ev.ToolResults[0]
}

// jsonString renders s as a JSON string literal, so no test hand-escapes a
// 6000-byte body.
func jsonString(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal test string: %v", err)
	}
	return string(b)
}

// assertClamped checks the shape of a clamped Text against its source: valid
// UTF-8, exactly one elision marker, a real prefix and suffix of the source,
// each end within the byte budget but not short-changed, and a bounded total.
func assertClamped(t *testing.T, got, src string) {
	t.Helper()
	if got == src {
		t.Fatalf("Text is the whole %d-byte body, want it clamped", len(src))
	}
	if !utf8.ValidString(got) {
		t.Errorf("Text is not valid UTF-8: the cut landed inside a rune (%d bytes)", len(got))
	}
	if n := strings.Count(got, elision); n != 1 {
		t.Fatalf("Text has %d %q markers, want exactly 1 (len %d)", n, elision, len(got))
	}
	i := strings.Index(got, elision)
	head, tail := got[:i], got[i+len(elision):]

	if !strings.HasPrefix(src, head) {
		t.Errorf("Text head (%d bytes) is not a prefix of the source", len(head))
	}
	if !strings.HasSuffix(src, tail) {
		t.Errorf("Text tail (%d bytes) is not a suffix of the source", len(tail))
	}
	if len(head) > resultTextCap {
		t.Errorf("Text head is %d bytes, want at most %d", len(head), resultTextCap)
	}
	if len(tail) > resultTextCap {
		t.Errorf("Text tail is %d bytes, want at most %d", len(tail), resultTextCap)
	}
	// Backing off a straddled rune costs at most 3 bytes; anything more means
	// the budget is being spent on something other than the boundary.
	if len(head) < resultTextCap-3 {
		t.Errorf("Text head is only %d bytes, want ~%d (a rune-boundary back-off costs ≤3)",
			len(head), resultTextCap)
	}
	if len(tail) < resultTextCap-3 {
		t.Errorf("Text tail is only %d bytes, want ~%d (a rune-boundary back-off costs ≤3)",
			len(tail), resultTextCap)
	}
	if max := 2*resultTextCap + len(elision); len(got) > max {
		t.Errorf("Text is %d bytes, want at most %d (2×%d + the marker)", len(got), max, resultTextCap)
	}
}

// ---------------------------------------------------------------- T35

// T35 — content as a plain JSON string lands in Text verbatim.
func TestT35ResultTextPlainStringContent(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"one line", "18 passed in 1.24s"},
		{"several lines", "FAILED tests/auth/test_refresh.py::test_refresh_expired_token\n2 failed, 18 passed"},
		{"leading blank lines", "\n\nScouted the payments module."},
		{"trailing newline kept", "ok  \tgithub.com/o/r/internal/journey\t0.01s\n"},
		{"multibyte", "résumé → 認証 ✓"},
		{"tabs and carriage returns", "a\tb\r\nc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := oneResult(t, jsonString(t, tc.body)).Text
			if got != tc.body {
				t.Errorf("Text = %q, want %q verbatim (it is under the clamp)", got, tc.body)
			}
		})
	}
}

// T35 — content as a block array: text blocks joined with "\n", every other
// block kind ignored.
func TestT35ResultTextBlockArray(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			"two text blocks join with a newline",
			`[{"type":"text","text":"first block"},{"type":"text","text":"second block"}]`,
			"first block\nsecond block",
		},
		{
			"non-text blocks are ignored",
			`[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBOR"}},` +
				`{"type":"text","text":"first block"},` +
				`{"type":"thinking","thinking":"secret scratch work"},` +
				`{"type":"text","text":"second block"}]`,
			"first block\nsecond block",
		},
		{
			"a single text block is not decorated",
			`[{"type":"text","text":"2 failed, 18 passed in 1.24s"}]`,
			"2 failed, 18 passed in 1.24s",
		},
		{
			"a block whose text is already multiline keeps its own newlines",
			`[{"type":"text","text":"FAILED a::one\nFAILED b::two"}]`,
			"FAILED a::one\nFAILED b::two",
		},
		{
			"empty array",
			`[]`,
			"",
		},
		{
			"array with no text blocks",
			`[{"type":"image","source":{}}]`,
			"",
		},
		{
			"array of empty text blocks",
			`[{"type":"text","text":""},{"type":"text","text":""}]`,
			"",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := oneResult(t, tc.content).Text; got != tc.want {
				t.Errorf("Text = %q, want %q", got, tc.want)
			}
		})
	}
}

// T35 — empty or absent content is "", never a panic and never a stray marker.
func TestT35ResultTextEmptyOrAbsentContent(t *testing.T) {
	tests := []struct {
		name    string
		content string // "" means: omit the field entirely
	}{
		{"field absent", ""},
		{"empty string", `""`},
		{"null", `null`},
		{"empty array", `[]`},
		{"number", `7`},
		{"object (not a documented shape: non-text results are empty)", `{"stdout":"hi"}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := oneResult(t, tc.content)
			if res.Text != "" {
				t.Errorf("Text = %q, want %q for a non-text result", res.Text, "")
			}
			if res.ToolUseID != "toolu_1" {
				t.Errorf("ToolUseID = %q, want %q — an empty body must not lose the rest",
					res.ToolUseID, "toolu_1")
			}
		})
	}
}

// T35 — the clamp boundary, in bytes.
func TestT35ResultTextClampBoundary(t *testing.T) {
	t.Run("exactly 2x the cap is untouched", func(t *testing.T) {
		body := strings.Repeat("a", 2*resultTextCap)
		got := oneResult(t, jsonString(t, body)).Text
		if got != body {
			t.Errorf("a %d-byte body came back %d bytes (%q…): both ends already cover it",
				len(body), len(got), got[:min(20, len(got))])
		}
		if strings.Contains(got, elision) {
			t.Errorf("Text carries an elision marker although nothing was elided")
		}
	})

	t.Run("one byte over the cap is clamped", func(t *testing.T) {
		body := strings.Repeat("a", 2*resultTextCap+1)
		assertClamped(t, oneResult(t, jsonString(t, body)).Text, body)
	})
}

// T35 — the adversarial one: a >4KB body whose runes straddle BOTH cut points.
// A byte-wise slice would hand back invalid UTF-8 at each end.
func TestT35ResultTextClampCutsOnRuneBoundariesAtBothEnds(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			// "→" is 3 bytes; 2048 % 3 == 2, so the head cut lands two bytes into
			// a rune, and the tail cut one byte into another.
			"three-byte runes throughout",
			strings.Repeat("→", 2000),
		},
		{
			// Four-byte runes at the head cut (offset by two ASCII bytes so 2048
			// is not a multiple of 4 from the start) and three-byte runes at the
			// tail cut.
			"four-byte rune at the head cut, three-byte at the tail",
			"ab" + strings.Repeat("🧭", 700) + strings.Repeat("→", 700),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.body
			if len(src) <= 2*resultTextCap {
				t.Fatalf("test bug: the body is only %d bytes, it would not be clamped", len(src))
			}
			// Preconditions: this test is worthless unless both naive cuts are
			// mid-rune.
			if utf8.ValidString(src[:resultTextCap]) {
				t.Fatalf("test bug: the head cut at byte %d is already on a rune boundary", resultTextCap)
			}
			if utf8.ValidString(src[len(src)-resultTextCap:]) {
				t.Fatalf("test bug: the tail cut at byte %d is already on a rune boundary",
					len(src)-resultTextCap)
			}

			got := oneResult(t, jsonString(t, src)).Text
			assertClamped(t, got, src)

			// The middle really is gone: a body of 2000 arrows keeps far fewer.
			if utf8.RuneCountInString(got) >= utf8.RuneCountInString(src) {
				t.Errorf("Text keeps %d runes of %d: nothing was elided",
					utf8.RuneCountInString(got), utf8.RuneCountInString(src))
			}
		})
	}
}

// T35 — the clamp applies to joined block text too, not just plain strings.
func TestT35ResultTextClampAppliesToBlockArrays(t *testing.T) {
	half := strings.Repeat("→", 1000) // 3000 bytes
	content := `[{"type":"text","text":` + jsonString(t, half) + `},` +
		`{"type":"text","text":` + jsonString(t, half) + `}]`

	src := half + "\n" + half
	assertClamped(t, oneResult(t, content).Text, src)
}

// IsError travels with the text: waypoint extraction needs both.
func TestT35ResultTextKeepsIsError(t *testing.T) {
	line := `{"type":"user","uuid":"u1","timestamp":"2026-08-30T12:00:00.000Z","sessionId":"s",` +
		`"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1",` +
		`"is_error":true,"content":"Exit code 1\nundefined: parseToken"}]}}`

	ev := mustParse(t, line)
	if len(ev.ToolResults) != 1 {
		t.Fatalf("len(ToolResults) = %d, want 1", len(ev.ToolResults))
	}
	res := ev.ToolResults[0]
	if !res.IsError {
		t.Error("IsError = false, want true")
	}
	if want := "Exit code 1\nundefined: parseToken"; res.Text != want {
		t.Errorf("Text = %q, want %q", res.Text, want)
	}
}
