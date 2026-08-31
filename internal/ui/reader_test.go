package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/narrator"
	"github.com/deephanson94/compass/internal/tmuxop"
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

	// Oldest first, like the rail itself now reads (M7 contract); every row
	// carries a real moment.
	for i, r := range rows {
		if r.Time.IsZero() {
			t.Errorf("rows[%d] (%s %q) has no time", i, r.Kind, r.Text)
		}
		if i > 0 && rows[i-1].Time.After(r.Time) && rows[i-1].Kind == "leg" && r.Kind == "leg" {
			t.Errorf("legs out of order: rows[%d] %v after rows[%d] %v", i-1, rows[i-1].Time, i, r.Time)
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
		ID: "s-api", TranscriptPath: sessionKey("s-api"), CWD: "/home/user/api",
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
	for _, want := range []string{sessionKey("s-api"), "fix the 401 bug", "claude/auth-fx", "historian"} {
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
		narrator.LegKey(sessionKey("s-api"), tr.Legs[0]): "maps the auth module",
		narrator.LegKey(sessionKey("s-api"), tr.Legs[1]): "wires the token refresh",
		// Legs[2] (test) stays heuristic; Legs[3] is HEAD and must not change.
		narrator.LegKey(sessionKey("s-api"), tr.Legs[3]): "must never show on head",
	}
	got := RenderTrail(tr, TrailOpts{
		Labels: labels, SessionKey: sessionKey("s-api"),
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

// followEvents is a transcript whose moments line up with fixtureLv2Trail's
// own — its legs, its waypoints and its subagent — so every row the Lv2 cursor
// can stand on has a document line to anchor to. The prose is long enough to
// wrap, so two rows minutes apart land on different screenfuls.
func followEvents(base time.Time) []transcript.Event {
	said := func(min int, text string) transcript.Event {
		return transcript.Event{Type: transcript.EventAssistant,
			Timestamp: base.Add(time.Duration(min) * time.Minute), Text: text}
	}
	return []transcript.Event{
		{Type: transcript.EventUser, Timestamp: base.Add(2 * time.Minute),
			Text: "moment02 fix the 401 bug in the auth middleware; it only reproduces on the refresh path, never on a fresh login"},
		said(9, "moment09 reading middleware.py to find where the 401 is raised, and following the refresh helper it calls into"),
		said(15, "moment15 running the auth suite so the failure has a name before anything in the middleware is touched"),
		said(16, "moment16 sending a subagent at the payments module, in case it shares the same refresh helper"),
		said(21, "moment21 the subagent came back: payments never touches refresh, so the blast radius is the auth package alone"),
		said(26, "moment26 eighteen passed and two failed, both of them in the refresh path and both about an expired token"),
		said(28, "moment28 opening tokens.py, which is where the expiry comparison the failing tests object to actually lives"),
		said(30, "moment30 a syntax error at tokens.py line 88 was hiding the real fault; fixing that first and rerunning"),
		said(34, "moment34 the expiry check compares a UTC stamp against local time, which is why it only fails after a refresh"),
		said(37, "moment37 patching tokens.py to compare in UTC, then rerunning the two tests that were objecting"),
		said(39, "moment39 both refresh tests pass now; rerunning the whole auth suite to be sure nothing else moved"),
	}
}

// followModel is the M7 deck: a wide terminal, the api session selected, the
// Lv2 fixture trail on the right and a transcript that runs alongside it.
func followModel(w, h int) *Model {
	m := New(nil)
	m.SetSize(w, h)
	m.SetSessions(fixtureSessions(fixtureBase), fixtureBase.Add(40*time.Minute))
	m.SetPanes(map[string]tmuxop.Pane{
		sessionKey("s-api"): {Target: "dev:1.0", ID: "%5", PID: 4242, Path: "/home/user/api", Command: "claude"},
	})
	m.point(sessionKey("s-api"))
	m.SetTrail(fixtureLv2Trail(fixtureBase))
	m.SetEvents(followEvents(fixtureBase))
	m.SetMirror(fixtureFrame)
	return m
}

// walkTo moves the Lv2 cursor onto the row that names moment at, one `j` at a
// time — the rows are walked, never assigned, so the walk itself is under test.
// The trail's own order is nobody's business here.
func walkTo(t *testing.T, m *Model, at time.Time) {
	t.Helper()
	rows := TrailRows(m.trail, m.level)
	for i := 0; i < len(rows); i++ {
		press(m, "k") // back to the top, whichever end of the journey that is
	}
	for i := 0; i <= len(rows); i++ {
		if m.cursor >= 0 && m.cursor < len(rows) && rows[m.cursor].Time.Equal(at) {
			return
		}
		press(m, "j")
	}
	t.Fatalf("no trail row stands on %v", at)
}

// T75 — the middle panel follows the trail: the live mirror at Lv1, and from
// Lv2 the reader, re-anchored to the cursor's own moment on every move.
func TestT75Lv2ReaderFollowsTheCursor(t *testing.T) {
	forceASCII(t)

	m := followModel(120, 30)
	if got := m.View(); !strings.Contains(got, mirrorMark+" dev:1.0 · live") {
		t.Fatalf("Lv1 must keep the live mirror:\n%s", got)
	}

	pressTab(m)
	if m.level != levelWaypoints {
		t.Fatalf("tab left the deck at Lv%d", m.level)
	}
	got := m.View()
	for _, want := range []string{"READER · api", "TRAIL · api"} {
		if !strings.Contains(got, want) {
			t.Errorf("Lv2 deck is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, mirrorMark+" dev:1.0 · live") {
		t.Error("the mirror outlived Lv1; the middle panel is the reader there")
	}

	// Tab opens on the present: the newest row, where the pinned trail already was.
	rows := TrailRows(m.trail, levelWaypoints)
	if m.cursor != len(rows)-1 {
		t.Errorf("tab landed the cursor on row %d, want the newest at %d", m.cursor, len(rows)-1)
	}

	// Every row the cursor lands on re-anchors the reader to that row's moment.
	opts := ReaderOpts{Width: m.readerWidth(), Unfolded: m.unfolded}
	anchors := map[int]bool{}
	for step := 0; step <= len(rows); step++ {
		if m.cursor < 0 || m.cursor >= len(rows) {
			t.Fatalf("step %d: the Lv2 cursor left the trail (%d of %d rows)", step, m.cursor, len(rows))
		}
		row := rows[m.cursor]
		if want := ReaderAnchor(m.events, opts, row.Time); want >= 0 && m.scroll != want {
			t.Fatalf("cursor on the %s %q: the reader is at line %d, want its moment at %d",
				row.Kind, row.Text, m.scroll, want)
		}
		anchors[m.scroll] = true
		press(m, "k") // the cursor enters at the present, so it walks backwards
	}
	if len(anchors) < 3 {
		t.Errorf("the conversation barely moved: %d anchors over %d rows", len(anchors), len(rows))
	}

	// And the panel really is showing that moment, not merely holding a number.
	walkTo(t, m, fixtureBase.Add(9*time.Minute))
	early := m.View()
	walkTo(t, m, fixtureBase.Add(37*time.Minute))
	late := m.View()
	if !strings.Contains(early, "moment09") {
		t.Errorf("the reader is not showing the scout leg's moment:\n%s", early)
	}
	if !strings.Contains(late, "moment37") {
		t.Errorf("the reader did not follow the cursor to the last leg:\n%s", late)
	}
	if strings.Contains(late, "moment09") {
		t.Error("the panel never moved: the first leg's words are still on screen")
	}
}

// T75 — the Lv2 cursor drags the trail's viewport with it, and no further:
// a row already on screen scrolls nothing, and the last row re-pins the panel
// to the present (M7 contract, scrolling).
func TestT75Lv2CursorKeepsItsRowVisible(t *testing.T) {
	forceASCII(t)

	// A short deck, so the trail is genuinely taller than its panel.
	m := followModel(120, 14)
	pressTab(m)
	rows := TrailRows(m.trail, levelWaypoints)
	w, h := m.trailBox()
	if len(TrailLines(m.trail, m.trailOpts(w, h))) <= h {
		t.Skipf("the fixture trail fits the panel (%d rows); nothing to scroll", h)
	}

	for step := 0; step <= len(rows); step++ {
		before := m.trailScroll
		row := TrailCursorRow(m.trail, m.trailOpts(w, h))
		_, height, top := m.trailView()
		if row >= 0 && (row < top || row >= top+height) {
			t.Fatalf("step %d: the cursor's row %d is outside the viewport %d-%d",
				step, row, top, top+height-1)
		}
		if d := m.trailScroll - before; d < 0 {
			t.Fatalf("step %d: walking down scrolled the trail up by %d", step, -d)
		}
		press(m, "j")
	}
	if !m.trailPinned {
		t.Error("the cursor walked onto the last row and the panel did not re-pin")
	}
}

// T76 — at Lv3 the keys are the document's: they scroll, fold and search the
// conversation, and the trail cursor stays where it was, still marking the
// place (M7 contract, middle panel).
func TestT76Lv3KeysDriveTheReader(t *testing.T) {
	forceASCII(t)

	m := followModel(120, 30)
	pressTab(m)
	walkTo(t, m, fixtureBase.Add(15*time.Minute))
	cursor, anchored := m.cursor, m.scroll

	pressTab(m)
	if m.level != levelReader {
		t.Fatalf("tab left the deck at Lv%d", m.level)
	}
	if m.cursor != cursor {
		t.Errorf("taking focus moved the trail cursor to %d, want %d", m.cursor, cursor)
	}
	if m.scroll != anchored {
		t.Errorf("the reader jumped as it took focus: line %d, want the anchor at %d", m.scroll, anchored)
	}

	trail := strings.Join(m.trailColumn(trailWidth, m.height-5), "\n")
	scroll, pinned, key := m.trailScroll, m.trailPinned, m.selectedKey
	still := func(t *testing.T, what string) {
		t.Helper()
		if m.cursor != cursor {
			t.Errorf("%s moved the trail cursor to %d, want %d", what, m.cursor, cursor)
		}
		if m.trailScroll != scroll || m.trailPinned != pinned {
			t.Errorf("%s scrolled the trail: %d/%v, want %d/%v", what, m.trailScroll, m.trailPinned, scroll, pinned)
		}
		if m.selectedKey != key {
			t.Errorf("%s changed the selection to %q", what, m.selectedKey)
		}
		if got := strings.Join(m.trailColumn(trailWidth, m.height-5), "\n"); got != trail {
			t.Errorf("%s redrew the trail:\n--- got ---\n%s\n--- want ---\n%s", what, got, trail)
		}
	}

	press(m, "g") // the document's top, not the fleet's grab
	if m.scroll != 0 {
		t.Errorf("g at Lv3 = line %d, want the top of the document", m.scroll)
	}
	still(t, "g")

	press(m, "j")
	press(m, "j")
	if m.scroll != 2 {
		t.Errorf("j j at Lv3 = line %d, want 2", m.scroll)
	}
	still(t, "j")

	pressCtrl(m, tea.KeyCtrlD)
	if m.scroll <= 2 {
		t.Errorf("ctrl+d at Lv3 = line %d, want half a page further down", m.scroll)
	}
	still(t, "ctrl+d")

	press(m, "G")
	if m.scroll <= 2 {
		t.Errorf("G at Lv3 = line %d, want the last screenful", m.scroll)
	}
	still(t, "G")

	press(m, " ")
	still(t, "space")
}
