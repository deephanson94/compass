package ui

import (
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

// boardModel is the M5 grouped fleet on a wide terminal with a trail for
// every live session: the deck as it opens. Board order, from the Manager's
// sort: infra (needs you), api (selected, working), webapp (working),
// tfstate, scratch (idle).
func boardModel(w, h int) *Model {
	base := fixtureBase
	m := New(nil)
	m.SetSize(w, h)
	m.SetSessions(fixtureGroupedFleet(base), base.Add(40*time.Minute))
	panes, list := fixtureGroupedPanes()
	m.SetPanes(panes)
	m.SetPaneOrder(list)
	m.point(sessionKey("s-api"))
	m.Update(fleetMsg{sessions: m.sessions, at: m.now, trailFor: sessionKey("s-api"), hasTrail: true,
		trail: fixtureTrail(base), trails: boardTrails(base)})
	return m
}

func boardTrails(base time.Time) map[string]journey.Trail {
	return map[string]journey.Trail{
		sessionKey("s-api"): fixtureTrail(base),
		sessionKey("s-webapp"): {
			Prompts: []journey.Prompt{{Text: "flake in the checkout suite", At: base.Add(5 * time.Minute)}},
			Legs: []journey.Leg{
				{Class: journey.Test, Label: "pytest", Start: base.Add(6 * time.Minute), End: base.Add(9 * time.Minute),
					Waypoints: []journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "18 passed · 2 failed", Short: "18✓ 2✗"}}},
				{Class: journey.Fix, Label: "checkout.py", Start: base.Add(10 * time.Minute), Files: []string{"checkout.py"}, Current: true},
			},
		},
		sessionKey("s-infra"): {
			Prompts: []journey.Prompt{{Text: "tighten the vpc security groups", At: base.Add(1 * time.Minute)}},
			Legs: []journey.Leg{
				{Class: journey.Scout, Label: "main.tf", Start: base.Add(2 * time.Minute), End: base.Add(20 * time.Minute), Files: []string{"main.tf"}},
				{Class: journey.Design, Label: "AskUserQuestion", Start: base.Add(21 * time.Minute), Current: true},
			},
		},
		sessionKey("s-tfstate"): {
			Prompts: []journey.Prompt{{Text: "reconcile the state file", At: base.Add(3 * time.Minute)}},
			Legs: []journey.Leg{
				{Class: journey.Build, Label: "state.tf", Start: base.Add(4 * time.Minute), End: base.Add(25 * time.Minute), Files: []string{"state.tf"}},
			},
		},
		sessionKey("s-scratch"): {
			Prompts: []journey.Prompt{{Text: "try the streaming api", At: base.Add(17 * time.Minute)}},
		},
	}
}

// rowFor is the board's row for a session: its number in the board's order.
func rowFor(t *testing.T, m *Model, key string) fleetRow {
	t.Helper()
	r, ok := m.boardRows()[key]
	if !ok {
		t.Fatalf("no board row for %q", key)
	}
	return r
}

// columnsOf splits a rendered board line at its hairlines.
func columnsOf(line string) []string {
	return strings.Split(line, "│")
}

// fakeNarrator records which sessions were asked, as a set: the order is the
// board's, but a test that asserted a sequence would be asserting an
// implementation detail.
type fakeNarrator struct {
	mu    sync.Mutex
	asked map[string]int
}

func (f *fakeNarrator) Labels(string, journey.Trail) map[string]string { return map[string]string{} }
func (f *fakeNarrator) Request(k string, _ journey.Trail, _ string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.asked == nil {
		f.asked = map[string]int{}
	}
	f.asked[k]++
	return true
}

// ---------------------------------------------------------------- opening

// The deck opens on the board on a terminal wide enough for one, and on the
// single trail when it is not.
func TestTheDeckOpensOnTheBoardWhenItFits(t *testing.T) {
	if m := New(nil); m.level != levelBoard {
		t.Errorf("a new deck is at Lv%d, want the board", m.level)
	}
	wide := New(nil)
	wide.SetSize(152, 30)
	wide.SetSessions(fixtureGroupedFleet(fixtureBase), fixtureBase)
	if wide.level != levelBoard {
		t.Errorf("a 152-column deck is at Lv%d, want the board", wide.level)
	}
	narrow := New(nil)
	narrow.SetSize(100, 30)
	if narrow.level != levelTrail {
		t.Errorf("a 100-column deck is at Lv%d, want the trail: the board needs %d columns", narrow.level, deckWideCols)
	}
	narrow.zoomOut()
	if narrow.level != levelTrail {
		t.Errorf("shift+tab on a narrow deck went to Lv%d", narrow.level)
	}
}

// A narrowing terminal takes the board away and a widening one gives it
// back — through the message bubbletea actually sends, not only SetSize.
func TestTheBoardReturnsWhenTheWidthDoes(t *testing.T) {
	m := boardModel(152, 30)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if m.level != levelTrail {
		t.Fatalf("narrowed to 80: level %d, want the trail", m.level)
	}
	m.Update(tea.WindowSizeMsg{Width: 152, Height: 30})
	if m.level != levelBoard {
		t.Errorf("widened again: level %d, want the board back", m.level)
	}
	// But a trail the person chose stays chosen.
	pressTab(m)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(tea.WindowSizeMsg{Width: 152, Height: 30})
	if m.level != levelTrail {
		t.Errorf("a chosen trail was replaced by the board on resize: level %d", m.level)
	}
}

// ---------------------------------------------------------------- columns

// Columns: as many as fit at the minimum, never more than there are sessions,
// sharing the width evenly, capped.
func TestBoardColumnsShareTheWidth(t *testing.T) {
	for _, tc := range []struct{ inner, count, n, w int }{
		{116, 9, 3, 36}, // a 120-column terminal
		{148, 9, 4, 34}, // 152
		{196, 9, 5, 36}, // 200
		{148, 2, 2, 52}, // two sessions: two columns, at the cap
		{60, 9, 1, 52},  // one column, capped
		{30, 9, 0, 0},   // nothing fits
		{148, 0, 0, 0},  // nothing to show
	} {
		n, w := boardColumns(tc.inner, tc.count)
		if n != tc.n || w != tc.w {
			t.Errorf("boardColumns(%d, %d) = %d×%d, want %d×%d", tc.inner, tc.count, n, w, tc.n, tc.w)
		}
		if n > 0 && n*w+(n-1)*gutterWidth > tc.inner {
			t.Errorf("boardColumns(%d): %d columns of %d overflow the width", tc.inner, n, w)
		}
	}
}

// The columns are the view's order — needs-you, stuck, working, idle by
// recency — and the selected session always has one.
func TestBoardColumnsFollowTheFleetAndKeepTheSelected(t *testing.T) {
	m := boardModel(152, 30)
	got := m.boardKeys(4)
	want := []string{sessionKey("s-infra"), sessionKey("s-api"), sessionKey("s-webapp"), sessionKey("s-tfstate")}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("board keys = %v, want %v", got, want)
	}
	m.point(sessionKey("s-scratch"))
	got = m.boardKeys(4)
	want = []string{sessionKey("s-infra"), sessionKey("s-api"), sessionKey("s-webapp"), sessionKey("s-scratch")}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("with scratch selected, board keys = %v, want %v", got, want)
	}
}

// And the board draws them in that order, left to right.
func TestBoardColumnsAreDrawnLeftToRightInOrder(t *testing.T) {
	forceASCII(t)
	got := boardModel(152, 30).View()
	var promptRow string
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "tighten the vpc") {
			promptRow = line
			break
		}
	}
	i, j, k, l := strings.Index(promptRow, "tighten the vpc"), strings.Index(promptRow, "fix the 401"),
		strings.Index(promptRow, "flake in the checkout"), strings.Index(promptRow, "reconcile the state")
	if !(i >= 0 && i < j && j < k && k < l) {
		t.Errorf("columns are not infra, api, webapp, tfstate left to right (%d/%d/%d/%d):\n%s", i, j, k, l, got)
	}
}

// The archive board shows archived sessions and nothing live.
func TestTheArchiveBoardShowsArchivedSessions(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	press(m, "A")
	if !m.archiveView {
		t.Fatal("A did not open the archive")
	}
	n, _ := boardColumns(152-2*edgePad, len(m.viewOrder()))
	if n == 0 {
		t.Fatal("the archive board has no columns")
	}
	for _, k := range m.boardKeys(n) {
		if s, ok := m.session(k); !ok || s.Live {
			t.Errorf("column %q is a live session on the archive board", k)
		}
	}
	got := m.View()
	for _, live := range []string{"fix the 401 bug", "tighten the vpc", "flake in the checkout"} {
		if strings.Contains(got, live) {
			t.Errorf("a live trail is on the archive board: %q", live)
		}
	}
	if !strings.Contains(got, "A live fleet") {
		t.Errorf("the archive board does not say how to get back:\n%s", got)
	}
}

// ---------------------------------------------------------------- goldens

// T80 — the board at 152x30: four trails, each under its own row, numbered in
// the board's order, and the strip naming what got no column.
func TestT80BoardGolden(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	got := m.View()
	compareGolden(t, "board-152x30.txt", got)
	if *update {
		return
	}
	for _, want := range []string{
		"1 ▲ infra", "▸2 ● api", "3 ● webapp", "4 ○ tfstate",
		"◉ \"tighten the vpc", "◉ \"fix the 401 bug\"", "◉ \"flake in the checkout", "◉ \"reconcile the state file\"",
		"● fix    checkout.py",
		"+1 more · 5 ○ scratch",
		"5 archived · A browses",
		"⌂ compass · board",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the board is missing %q:\n%s", want, got)
		}
	}
	for _, gone := range []string{"TRAIL · api", "FLEET · live", "try the streaming api", "nothing yet"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q is on the board:\n%s", gone, got)
		}
	}
	// The header really is a column header: it sits past a hairline.
	headed := false
	for _, line := range strings.Split(got, "\n") {
		cols := columnsOf(line)
		if len(cols) > 1 && strings.Contains(cols[0], "1 ▲ infra") && strings.Contains(cols[1], "▸2 ● api") {
			headed = true
		}
	}
	if !headed {
		t.Errorf("the column headers are not side by side across a hairline:\n%s", got)
	}
	assertBoardFits(t, m, got, 152)
}

// T80b — at 120x30 three columns fit, and two sessions go to the strip.
func TestT80bBoardThreeColumnsGolden(t *testing.T) {
	forceASCII(t)
	m := boardModel(120, 30)
	got := m.View()
	compareGolden(t, "board-120x30.txt", got)
	if *update {
		return
	}
	for _, want := range []string{"fix the 401 bug", "tighten the vpc", "flake in the checkout", "+2 more · 4 ○ tfstate · 5 ○ scratch"} {
		if !strings.Contains(got, want) {
			t.Errorf("the board is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "reconcile the state file") {
		t.Error("a fourth column appeared on a three-column board")
	}
	assertBoardFits(t, m, got, 120)
}

// assertBoardFits is the width discipline the mirror golden has: no line past
// the terminal, and no column row past its own hairline.
func assertBoardFits(t *testing.T, m *Model, got string, w int) {
	t.Helper()
	for _, line := range strings.Split(got, "\n") {
		if x := lipgloss.Width(line); x > w {
			t.Errorf("board line runs past the terminal (%d of %d cols): %q", x, w, line)
		}
	}
	_, cw := boardColumns(w-2*edgePad, len(m.viewOrder()))
	for _, key := range []string{sessionKey("s-api"), sessionKey("s-infra")} {
		for _, line := range m.boardColumn(key, rowFor(t, m, key), cw, 20) {
			if x := lipgloss.Width(line); x > cw {
				t.Errorf("a column row is %d wide, past the %d-col hairline: %q", x, cw, line)
			}
		}
	}
}

// A tall trail is pinned to its present and stops inside its column.
func TestATallColumnShowsThePresent(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	m.trails[sessionKey("s-api")] = longTrail(60)
	rows := m.boardColumn(sessionKey("s-api"), rowFor(t, m, sessionKey("s-api")), 34, 20)
	if len(rows) != 20 {
		t.Fatalf("a 20-row column has %d rows", len(rows))
	}
	body := strings.Join(rows, "\n")
	if !strings.Contains(body, "step 59") {
		t.Errorf("the column is not pinned to the newest leg:\n%s", body)
	}
	if strings.Contains(body, "step 0") {
		t.Errorf("the column shows the oldest leg; it should show the present:\n%s", body)
	}
}

// ---------------------------------------------------------------- keys

// Tab opens the selected column as the single trail, with the trail, plan and
// labels already in hand; shift+tab and esc come back. Board → reader is
// three tabs (§3).
func TestTabWalksBoardTrailWaypointsReader(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	m.trail = journey.Trail{} // as if the single trail had never been polled
	m.boardLabels[sessionKey("s-api")] = map[string]string{"k": "v"}

	pressTab(m)
	if m.level != levelTrail {
		t.Fatalf("tab from the board went to Lv%d", m.level)
	}
	if len(m.trail.Legs) == 0 {
		t.Error("the single trail opened empty; the board had its trail already")
	}
	if m.labels["k"] != "v" {
		t.Error("the column's narrated labels did not come along")
	}
	if got := m.View(); !strings.Contains(got, "TRAIL · api") {
		t.Errorf("Lv1 is not the single trail:\n%s", got)
	}
	pressTab(m)
	pressTab(m)
	if m.level != levelReader {
		t.Errorf("three tabs from the board reached Lv%d, want the reader", m.level)
	}
	for _, k := range []tea.KeyType{tea.KeyShiftTab, tea.KeyShiftTab, tea.KeyShiftTab} {
		m.Update(tea.KeyMsg{Type: k})
	}
	if m.level != levelBoard {
		t.Errorf("three shift+tabs from the reader reached Lv%d, want the board", m.level)
	}
	if got := m.View(); !strings.Contains(got, "flake in the checkout") {
		t.Errorf("shift+tab did not return to the board:\n%s", got)
	}
	pressTab(m)
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.level != levelBoard {
		t.Errorf("esc from the trail reached Lv%d, want the board", m.level)
	}
}

// j/k on the board walk the columns in the board's order, and on into the
// strip; 1–9 pick by the number a column wears.
func TestBoardKeysWalkTheColumns(t *testing.T) {
	m := boardModel(152, 30)
	press(m, "j")
	if m.selectedKey != sessionKey("s-webapp") {
		t.Errorf("j from api selected %q, want webapp (the next column)", m.selectedKey)
	}
	press(m, "k")
	press(m, "k")
	if m.selectedKey != sessionKey("s-infra") {
		t.Errorf("k k selected %q, want infra (the first column)", m.selectedKey)
	}
	press(m, "k")
	if m.selectedKey != sessionKey("s-infra") {
		t.Error("k past the first column moved the selection")
	}
	press(m, "5")
	if m.selectedKey != sessionKey("s-scratch") {
		t.Errorf("5 selected %q, want scratch (the strip's first entry)", m.selectedKey)
	}
	// The board's 1 is infra; the fleet list's 1 is api. The board wins here.
	press(m, "1")
	if m.selectedKey != sessionKey("s-infra") {
		t.Errorf("1 selected %q, want infra (the first column)", m.selectedKey)
	}
	// And j from infra is api, the next column — not tfstate, the next row
	// of the fleet's grouped list.
	press(m, "j")
	if m.selectedKey != sessionKey("s-api") {
		t.Errorf("j from infra selected %q, want api (the next column)", m.selectedKey)
	}
	press(m, "9")
	if !strings.Contains(m.note, "9") {
		t.Errorf("9 with five sessions said %q", m.note)
	}
}

// ctrl+d, ctrl+u and G on the board say what the board is rather than
// scrolling a trail that is not on screen.
func TestScrollKeysOnTheBoardDoNotTouchTheHiddenTrail(t *testing.T) {
	m := boardModel(152, 30)
	m.trail = longTrail(60)
	before := m.trailScroll
	pressCtrl(m, tea.KeyCtrlD)
	if m.trailScroll != before || m.note == "" {
		t.Errorf("ctrl+d on the board scrolled the hidden trail (%d → %d) or said nothing", before, m.trailScroll)
	}
	press(m, "G")
	if m.note == "" {
		t.Error("G on the board said nothing")
	}
}

// The mirror is a Lv1 panel; on the board it is never captured into, and m
// says where it shows.
func TestNoCaptureOnTheBoard(t *testing.T) {
	m := boardModel(152, 30)
	press(m, "m")
	if m.capture() != nil {
		t.Error("the board polls a pane for a mirror it does not draw")
	}
	if !strings.Contains(m.note, "tab") {
		t.Errorf("m on the board said %q; it should say the mirror shows on one trail (tab)", m.note)
	}
}

// The footer on the board and on the trail each name the way to the other.
func TestBoardAndTrailFootersNameEachOther(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	if got := m.View(); !strings.Contains(got, "tab one trail") || strings.Contains(got, "tab deeper") {
		t.Errorf("the board's footer does not offer one trail:\n%s", got)
	}
	pressTab(m)
	if got := m.View(); !strings.Contains(got, "⇧tab board") {
		t.Errorf("the trail's footer does not offer the board:\n%s", got)
	}
}

// ---------------------------------------------------------------- data

// refresh polls exactly the board's columns — the view's first n and the
// selected session — never merely the first n of the fleet.
func TestTheBoardPollsEveryColumn(t *testing.T) {
	m := boardModel(152, 30)
	keys := func() []string {
		var out []string
		for _, tg := range m.boardTargets() {
			if tg.path == "" {
				t.Errorf("target %q has no transcript path", tg.key)
			}
			out = append(out, tg.key)
		}
		return out
	}
	if got, want := strings.Join(keys(), ","), strings.Join(m.boardKeys(4), ","); got != want {
		t.Errorf("the board polls %v, want its columns %v", got, want)
	}
	m.point(sessionKey("s-scratch"))
	found := false
	for _, k := range keys() {
		if k == sessionKey("s-scratch") {
			found = true
		}
	}
	if !found {
		t.Error("the selected session's column is not polled")
	}
	narrow := boardModel(100, 30)
	if got := narrow.boardTargets(); got != nil {
		t.Errorf("a narrow deck polls the board: %+v", got)
	}
}

// A poll without trails — a narrow terminal's — leaves the board standing,
// and a poll with them merges rather than replaces.
func TestAPollWithoutTrailsLeavesTheBoardStanding(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	before := len(m.trails)
	m.Update(fleetMsg{sessions: m.sessions, at: m.now.Add(time.Second)})
	if len(m.trails) != before {
		t.Errorf("a poll without trails left %d columns, want %d", len(m.trails), before)
	}
	m.Update(fleetMsg{sessions: m.sessions, at: m.now.Add(2 * time.Second),
		trails: map[string]journey.Trail{sessionKey("s-infra"): boardTrails(fixtureBase)[sessionKey("s-infra")]}})
	if len(m.trails) != before {
		t.Errorf("a partial poll replaced the board: %d columns, want %d", len(m.trails), before)
	}
	if got := m.View(); !strings.Contains(got, "flake in the checkout") {
		t.Errorf("the board went blank after a partial tick:\n%s", got)
	}
}

// The first fleet message of a run carries the board's trails too.
func TestTheFirstPollFeedsTheBoard(t *testing.T) {
	forceASCII(t)
	m := New(nil)
	m.SetSize(152, 30)
	m.Update(fleetMsg{sessions: fixtureGroupedFleet(fixtureBase), at: fixtureBase.Add(40 * time.Minute),
		trails: boardTrails(fixtureBase)})
	if got := m.View(); !strings.Contains(got, "fix the 401 bug") {
		t.Errorf("the first frame has no trails:\n%s", got)
	}
}

// A refresh that lands after a newer one is dropped: its fleet and its clock
// are older than what is on screen.
func TestAStaleRefreshDoesNotOverwriteANewerOne(t *testing.T) {
	m := boardModel(152, 30)
	newer := m.now
	m.Update(fleetMsg{sessions: m.sessions, at: newer.Add(-30 * time.Second), trailFor: sessionKey("s-api"),
		hasTrail: true, trail: journey.Trail{}, trails: map[string]journey.Trail{sessionKey("s-api"): {}}})
	if !m.now.Equal(newer) {
		t.Errorf("the deck's clock went backwards to %v", m.now)
	}
	if len(m.trails[sessionKey("s-api")].Legs) == 0 {
		t.Error("a stale refresh emptied api's column")
	}
}

// A column whose feed has not been polled yet says so, rather than wearing
// the empty state designed for a session that never did anything.
func TestAnUnpolledColumnSaysItIsReading(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	delete(m.trails, sessionKey("s-webapp"))
	got := m.View()
	if !strings.Contains(got, "reading its transcript") {
		t.Errorf("the unpolled column does not say it is reading:\n%s", got)
	}
	if strings.Contains(got, "nothing yet") {
		t.Errorf("an unpolled column wears the never-did-anything state:\n%s", got)
	}
}

// Every column is handed to the narrator, the selected one by its own
// request first; a column is asked as a set, never a sequence.
func TestEveryBoardColumnIsNarrated(t *testing.T) {
	m := boardModel(152, 30)
	f := &fakeNarrator{}
	m.SetNarrator(f)
	m.Update(fleetMsg{sessions: m.sessions, at: m.now.Add(time.Second), trailFor: sessionKey("s-api"),
		hasTrail: true, trail: fixtureTrail(fixtureBase), trails: boardTrails(fixtureBase)})
	for _, k := range []string{sessionKey("s-infra"), sessionKey("s-webapp"), sessionKey("s-tfstate"), sessionKey("s-api")} {
		if f.asked[k] == 0 {
			t.Errorf("%q was never handed to the narrator", k)
		}
	}
}

// ---------------------------------------------------------------- brightness

// A column is bright while there is something in it to read: working, needs
// you, or finished within the day and not opened since. "porter finished two
// minutes ago and it's already dim" was the first thing the board got wrong.
func TestAColumnIsBrightWhileUnread(t *testing.T) {
	m := boardModel(152, 30)
	now := m.now
	at := func(st state.State, last time.Time) fleet.Session {
		return fleet.Session{Info: fleet.SessionInfo{ID: "x", TranscriptPath: "/k/x.jsonl", LastEventAt: last},
			Snap: state.Snapshot{State: st}, Live: true}
	}
	for _, tc := range []struct {
		name  string
		s     fleet.Session
		muted bool
	}{
		{"working", at(state.Working, now.Add(-30*time.Hour)), false},
		{"needs you", at(state.NeedsYou, now.Add(-30*time.Hour)), false},
		{"stuck", at(state.Stuck, now.Add(-30*time.Hour)), false},
		{"finished two minutes ago, unread", at(state.Idle, now.Add(-2*time.Minute)), false},
		{"finished eighteen hours ago, unread", at(state.Idle, now.Add(-18*time.Hour)), false},
		{"finished two days ago", at(state.Idle, now.Add(-48*time.Hour)), true},
	} {
		if got := m.boardMuted(tc.s); got != tc.muted {
			t.Errorf("%s: muted = %v, want %v", tc.name, got, tc.muted)
		}
	}
	fresh := at(state.Idle, now.Add(-2*time.Minute))
	m.markSeen(fresh.Info.Key())
	if !m.boardMuted(fresh) {
		t.Error("a column read since it finished is still bright")
	}
	fresh.Info.LastEventAt = now.Add(time.Minute)
	if m.boardMuted(fresh) {
		t.Error("a column with a newer event than the last look is dim")
	}
	m.archiveView = true
	if !m.boardMuted(at(state.Working, now)) {
		t.Error("an archived column is not muted; the archive can never be moving")
	}
}

// And the muting is real: a muted column carries the dim sequence and no
// class tint; a bright one keeps its tints. Styles are the subject, so this
// test does not force ASCII.
func TestAMutedColumnIsRenderedDim(t *testing.T) {
	m := boardModel(152, 30)
	old := fleet.Session{Info: fleet.SessionInfo{ID: "s-tfstate", TranscriptPath: sessionKey("s-tfstate"),
		LastEventAt: m.now.Add(-48 * time.Hour)}, Snap: state.Snapshot{State: state.Idle}, Live: true}
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == sessionKey("s-tfstate") {
			m.sessions[i] = old
		}
	}
	dimSeq := strings.TrimSuffix(dimStyle.Render("x"), "x\x1b[0m")
	if dimSeq == dimStyle.Render("x") || dimSeq == "" {
		t.Skip("no colour in this renderer; the muting is invisible here")
	}
	muted := strings.Join(m.boardColumn(sessionKey("s-tfstate"), rowFor(t, m, sessionKey("s-tfstate")), 34, 20)[3:], "\n")
	if !strings.Contains(muted, dimSeq) {
		t.Errorf("a muted column carries no dim sequence: %q", muted)
	}
	bright := strings.Join(m.boardColumn(sessionKey("s-api"), rowFor(t, m, sessionKey("s-api")), 34, 20)[3:], "\n")
	if ansi.Strip(bright) == bright {
		t.Error("a working column lost its tints")
	}
}

// Tab on a column marks it read; so does Enter.
func TestOpeningAColumnMarksItRead(t *testing.T) {
	m := boardModel(152, 30)
	if _, ok := m.seen[sessionKey("s-api")]; ok {
		t.Fatal("nothing has been opened yet")
	}
	pressTab(m)
	if _, ok := m.seen[sessionKey("s-api")]; !ok {
		t.Error("tab into api's trail did not mark it read")
	}
}

// Every working column breathes on the board; an archived one never does.
func TestTheBoardBreathesForAnyWorkingColumn(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	m.point(sessionKey("s-infra")) // needs you: the selected session is not working
	if !m.anyWorking() {
		t.Error("a board with two working columns does not breathe")
	}
	m.Update(breathTickMsg(m.now))
	if !m.pulse {
		t.Fatal("the breath tick did not turn the pulse on")
	}
	if got := m.View(); !strings.Contains(got, glyphBreath) {
		t.Errorf("no column's HEAD is breathing:\n%s", got)
	}
	m.archiveView = true
	if got := m.View(); strings.Contains(got, glyphBreath) {
		t.Errorf("an archived column is breathing:\n%s", got)
	}
}

// HEAD's figure is time since it started, not a duration it does not have.
func TestAMovingHeadIsAgedFromItsStart(t *testing.T) {
	forceASCII(t)
	now := fixtureBase.Add(40 * time.Minute)
	l := journey.Leg{Class: journey.Fix, Label: "checkout.py",
		Start: fixtureBase.Add(10 * time.Minute), End: fixtureBase.Add(11 * time.Minute), Current: true}
	got := legRow(l, l.Label, false, now, 40, false)
	if !strings.Contains(got, "← 30m") {
		t.Errorf("HEAD is not aged from its start: %q", got)
	}
}

// A narration request the narrator refused — its one batch in flight was a
// column's — is asked again next tick, not remembered as answered.
func TestARefusedNarrationIsAskedAgain(t *testing.T) {
	m := boardModel(152, 30)
	f := &refusingNarrator{}
	m.SetNarrator(f)
	m.requestNarration()
	m.requestNarration()
	if f.calls != 2 {
		t.Errorf("after two refusals the selected trail was asked %d times, want 2", f.calls)
	}
	f.accept = true
	m.requestNarration()
	m.requestNarration()
	if f.calls != 3 {
		t.Errorf("once accepted, the trail was asked %d times in total, want 3 (asked once, then remembered)", f.calls)
	}
}

type refusingNarrator struct {
	calls  int
	accept bool
}

func (r *refusingNarrator) Labels(string, journey.Trail) map[string]string {
	return map[string]string{}
}
func (r *refusingNarrator) Request(string, journey.Trail, string) bool {
	r.calls++
	return r.accept
}
