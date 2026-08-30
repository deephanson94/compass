package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/narrator"
	"github.com/deephanson94/compass/internal/transcript"
)

// fixtureEvents is the little conversation every reader test reads: a prompt,
// a thought, a clean tool call with a multi-line result, and a failing one.
func fixtureEvents(base time.Time) []transcript.Event {
	bash := func(cmd string) json.RawMessage {
		raw, _ := json.Marshal(map[string]string{"command": cmd})
		return raw
	}
	return []transcript.Event{
		{Type: transcript.EventUser, Timestamp: base,
			Text: "fix the 401 bug in the auth middleware"},
		{Type: transcript.EventAssistant, Timestamp: base.Add(10 * time.Second),
			Text: "The refresh path looks wrong. Let me run the auth suite first and see which tests object."},
		{Type: transcript.EventAssistant, Timestamp: base.Add(20 * time.Second),
			ToolUses: []transcript.ToolUse{{ID: "tu1", Name: "Bash", Input: bash("pytest tests/auth -x")}}},
		{Type: transcript.EventUser, Timestamp: base.Add(40 * time.Second),
			ToolResults: []transcript.ToolResult{{ToolUseID: "tu1",
				Text: "collected 20 items\n\ntests/auth/test_refresh.py .F\n\nFAILED tests/auth/test_refresh.py::test_refresh_expired_token\n== 1 failed, 19 passed in 1.24s =="}}},
		{Type: transcript.EventAssistant, Timestamp: base.Add(50 * time.Second),
			ToolUses: []transcript.ToolUse{{ID: "tu2", Name: "Read", Input: json.RawMessage(`{"file_path":"src/auth/tokens.py"}`)}}},
		{Type: transcript.EventUser, Timestamp: base.Add(55 * time.Second),
			ToolResults: []transcript.ToolResult{{ToolUseID: "tu2", IsError: true,
				Text: "EACCES: permission denied reading src/auth/tokens.py\nthe file is owned by root"}}},
		{Type: transcript.EventAssistant, Timestamp: base.Add(70 * time.Second),
			Text: "The expiry check compares a UTC stamp against local time. Fixing tokens.py."},
		// A sidechain line must never reach the document.
		{Type: transcript.EventAssistant, IsSidechain: true, Timestamp: base.Add(75 * time.Second),
			Text: "subagent chatter that belongs to the branch lane"},
	}
}

// T50 — the reader document at 60×24: prompt chevron, wrapped prose, tool
// one-liners, a folded result with its line count, and a failed result leading
// with its first error line.
func TestT50ReaderGolden(t *testing.T) {
	forceASCII(t)

	got := RenderReader(fixtureEvents(fixtureBase), ReaderOpts{Width: 60, Height: 24})
	compareGolden(t, "reader-60x24.txt", got)
}

// T50 — unfolding spends rows on the result body, capped and honest.
func TestT50ReaderUnfoldedGolden(t *testing.T) {
	forceASCII(t)

	got := RenderReader(fixtureEvents(fixtureBase), ReaderOpts{
		Width: 60, Height: 24, Unfolded: map[int]bool{3: true},
	})
	compareGolden(t, "reader-60x24-unfolded.txt", got)
}

// T50 — a search inverts its matches and nothing else; the golden proves the
// highlight survives the ASCII profile as pure text.
func TestT50ReaderSearchGolden(t *testing.T) {
	forceASCII(t)

	got := RenderReader(fixtureEvents(fixtureBase), ReaderOpts{
		Width: 60, Height: 24, Query: "refresh",
	})
	compareGolden(t, "reader-60x24-search.txt", got)
}

// T51 — the Lv2 rows enumerate what the trail draws, top-down, and each names
// a moment the reader can anchor to.
func TestT51TrailRowsAndAnchor(t *testing.T) {
	tr := fixtureLv2Trail(fixtureBase)
	rows := TrailRows(tr, 2)
	if len(rows) == 0 {
		t.Fatal("TrailRows returned nothing for a populated trail")
	}

	// Newest first, like the rail itself; every row carries a real moment.
	for i, r := range rows {
		if r.Time.IsZero() {
			t.Errorf("rows[%d] (%s %q) has no time", i, r.Kind, r.Text)
		}
		if i > 0 && rows[i-1].Time.Before(r.Time) && rows[i-1].Kind == "leg" && r.Kind == "leg" {
			t.Errorf("legs out of order: rows[%d] %v before rows[%d] %v", i-1, rows[i-1].Time, i, r.Time)
		}
	}

	// The anchor maps a row's moment to the first document line at or after it.
	events := fixtureEvents(fixtureBase)
	opts := ReaderOpts{Width: 60}
	if line := ReaderAnchor(events, opts, fixtureBase); line != 0 {
		t.Errorf("anchor at the very start = %d, want 0 (the prompt's first line)", line)
	}
	if line := ReaderAnchor(events, opts, fixtureBase.Add(time.Hour)); line != -1 {
		t.Errorf("anchor past the end = %d, want -1", line)
	}
	mid := ReaderAnchor(events, opts, fixtureBase.Add(45*time.Second))
	if mid <= 0 {
		t.Errorf("anchor mid-conversation = %d, want a later document line", mid)
	}
}

// T52 — BuildAsk constructs the historian without starting it: the real CLI,
// briefed on whose journey it is reading, in that session's own directory.
func TestT52BuildAsk(t *testing.T) {
	info := fleet.SessionInfo{
		ID: "s-api", TranscriptPath: "/x/api.jsonl", CWD: "/home/user/api",
		GitBranch: "claude/auth-fx", Title: "fix the 401 bug",
		StartedAt: fixtureBase, LastEventAt: fixtureBase.Add(30 * time.Minute),
	}
	cmd := BuildAsk(info)

	if base := cmd.Args[0]; !strings.HasSuffix(base, "claude") {
		t.Errorf("Args[0] = %q, want the claude CLI", base)
	}
	if cmd.Dir != "/home/user/api" {
		t.Errorf("Dir = %q, want the session's cwd", cmd.Dir)
	}
	if len(cmd.Args) != 3 || cmd.Args[1] != "--append-system-prompt" {
		t.Fatalf("Args = %v, want [claude --append-system-prompt <preamble>]", cmd.Args)
	}
	preamble := cmd.Args[2]
	for _, want := range []string{"/x/api.jsonl", "fix the 401 bug", "claude/auth-fx", "historian"} {
		if !strings.Contains(preamble, want) {
			t.Errorf("preamble is missing %q", want)
		}
	}
}

// T53 — narrated labels replace the heuristic `verb label` on closed legs;
// HEAD keeps its live heuristic (narration is for history).
func TestT53NarratedOverlayGolden(t *testing.T) {
	forceASCII(t)

	tr := fixtureTrail(fixtureBase)
	labels := map[string]string{
		narrator.LegKey("s-api", tr.Legs[0]): "maps the auth module",
		narrator.LegKey("s-api", tr.Legs[1]): "wires the token refresh",
		// Legs[2] (test) stays heuristic; Legs[3] is HEAD and must not change.
		narrator.LegKey("s-api", tr.Legs[3]): "must never show on head",
	}
	got := RenderTrail(tr, TrailOpts{
		Labels: labels, SessionID: "s-api",
		Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: 20, Level: 1, Cursor: -1,
	})
	compareGolden(t, "trail-narrated-38x20.txt", got)

	if strings.Contains(got, "must never show on head") {
		t.Error("HEAD rendered its narrated label; the open leg keeps its live heuristic")
	}
	for _, want := range []string{"maps the auth module", "wires the token refresh"} {
		if !strings.Contains(got, want) {
			t.Errorf("narrated label %q did not render", want)
		}
	}
}
