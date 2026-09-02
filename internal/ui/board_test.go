package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/tmuxop"
)

// boardModel is the M5 grouped fleet on a wide terminal with a trail for
// every session that has done anything: the deck as it opens.
func boardModel(w, h int) *Model {
	base := fixtureBase
	m := New(nil)
	m.SetSize(w, h)
	m.SetSessions(fixtureGroupedFleet(base), base.Add(40*time.Minute))
	panes, list := fixtureGroupedPanes()
	m.SetPanes(panes)
	m.SetPaneOrder(list)
	m.point(sessionKey("s-api"))

	webapp := journey.Trail{
		Prompts: []journey.Prompt{{Text: "flake in the checkout suite", At: base.Add(5 * time.Minute)}},
		Legs: []journey.Leg{
			{Class: journey.Test, Label: "pytest", Start: base.Add(6 * time.Minute), End: base.Add(9 * time.Minute),
				Waypoints: []journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "18 passed · 2 failed", Short: "18✓ 2✗"}}},
			{Class: journey.Fix, Label: "checkout.py", Start: base.Add(10 * time.Minute), Files: []string{"checkout.py"}, Current: true},
		},
	}
	infra := journey.Trail{
		Prompts: []journey.Prompt{{Text: "tighten the vpc security groups", At: base.Add(1 * time.Minute)}},
		Legs: []journey.Leg{
			{Class: journey.Scout, Label: "main.tf", Start: base.Add(2 * time.Minute), End: base.Add(20 * time.Minute), Files: []string{"main.tf"}},
			{Class: journey.Design, Label: "AskUserQuestion", Start: base.Add(21 * time.Minute), Current: true},
		},
	}
	tfstate := journey.Trail{
		Prompts: []journey.Prompt{{Text: "reconcile the state file", At: base.Add(3 * time.Minute)}},
		Legs: []journey.Leg{
			{Class: journey.Build, Label: "state.tf", Start: base.Add(4 * time.Minute), End: base.Add(25 * time.Minute), Files: []string{"state.tf"}},
		},
	}
	m.Update(fleetMsg{sessions: m.sessions, at: m.now, trailFor: sessionKey("s-api"), hasTrail: true,
		trail: fixtureTrail(base),
		trails: map[string]journey.Trail{
			sessionKey("s-api"):     fixtureTrail(base),
			sessionKey("s-webapp"):  webapp,
			sessionKey("s-infra"):   infra,
			sessionKey("s-tfstate"): tfstate,
		}})
	return m
}

// The deck opens on the board on a terminal wide enough for one, and on the
// single trail when it is not.
func TestTheDeckOpensOnTheBoardWhenItFits(t *testing.T) {
	if m := New(nil); m.level != levelBoard {
		t.Errorf("a new deck is at Lv%d, want the board", m.level)
	}
	wide := New(nil)
	wide.SetSize(152, 30)
	if wide.level != levelBoard {
		t.Errorf("a 152-column deck is at Lv%d, want the board", wide.level)
	}
	narrow := New(nil)
	narrow.SetSize(100, 30)
	if narrow.level != levelTrail {
		t.Errorf("a 100-column deck is at Lv%d, want the trail: the board needs %d columns", narrow.level, deckWideCols)
	}
	// And a narrow deck cannot zoom out past the trail.
	narrow.zoomOut()
	if narrow.level != levelTrail {
		t.Errorf("shift+tab on a narrow deck went to Lv%d", narrow.level)
	}
}

// Columns: as many as fit at the minimum, sharing the width evenly, capped.
func TestBoardColumnsShareTheWidth(t *testing.T) {
	for _, tc := range []struct{ inner, n, w int }{
		{100, 1, 52}, // one column, capped
		{116, 2, 40}, // a 120-column terminal
		{148, 3, 36}, // 152
		{196, 4, 38}, // 200
		{60, 0, 0},   // nothing beside the fleet
	} {
		n, w := boardColumns(tc.inner)
		if n != tc.n || w != tc.w {
			t.Errorf("boardColumns(%d) = %d×%d, want %d×%d", tc.inner, n, w, tc.n, tc.w)
		}
		if n > 0 && fleetWidth+gutterWidth+n*w+(n-1)*gutterWidth > tc.inner {
			t.Errorf("boardColumns(%d): %d columns of %d overflow the width", tc.inner, n, w)
		}
	}
}

// The columns are the fleet's order — needs-you, stuck, working, idle by
// recency — and the selected session always has one.
func TestBoardColumnsFollowTheFleetAndKeepTheSelected(t *testing.T) {
	m := boardModel(152, 30)
	// Three columns at 152. Fleet order in the fixture: infra (needs-you),
	// api, webapp (working), tfstate, scratch (idle).
	got := m.boardKeys(3)
	want := []string{sessionKey("s-infra"), sessionKey("s-api"), sessionKey("s-webapp")}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("board keys = %v, want %v", got, want)
	}

	// Select the fifth session: it takes the last column.
	m.point(sessionKey("s-scratch"))
	got = m.boardKeys(3)
	want = []string{sessionKey("s-infra"), sessionKey("s-api"), sessionKey("s-scratch")}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("with scratch selected, board keys = %v, want %v", got, want)
	}
}

// T80 — the board at 152x30: the fleet at its floor and three trails, each
// under its own fleet row.
func TestT80BoardGolden(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	got := m.View()
	compareGolden(t, "board-152x30.txt", got)
	if *update {
		return
	}
	for _, want := range []string{
		"3 ▲ infra      needs you", // the column header is the fleet row
		"◉ \"tighten the vpc",      // and its trail hangs under it
		"◉ \"fix the 401 bug\"",
		"◉ \"flake in the checkout",
		"● fix    checkout.py", // a working column's HEAD
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the board is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "TRAIL · api") {
		t.Error("the single trail's title is on the board")
	}
	if strings.Contains(got, "reconcile the state file") {
		t.Error("a fourth column appeared on a three-column board")
	}
}

// T80b — the same deck at 120x30 has two columns: the selected api and, ahead
// of it, infra which needs you.
func TestT80bBoardTwoColumnsGolden(t *testing.T) {
	forceASCII(t)
	m := boardModel(120, 30)
	got := m.View()
	compareGolden(t, "board-120x30.txt", got)
	if *update {
		return
	}
	if !strings.Contains(got, "fix the 401 bug") || !strings.Contains(got, "tighten the vpc") {
		t.Errorf("the two columns are not infra and api:\n%s", got)
	}
	if strings.Contains(got, "flake in the checkout") {
		t.Error("a third column appeared on a two-column board")
	}
}

// Tab opens the selected column as the single trail, with the trail already
// in hand; shift+tab comes back. Board → reader is three tabs (§3).
func TestTabWalksBoardTrailWaypointsReader(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	m.trail = journey.Trail{} // as if the single trail had never been polled

	pressTab(m)
	if m.level != levelTrail {
		t.Fatalf("tab from the board went to Lv%d", m.level)
	}
	if len(m.trail.Legs) == 0 {
		t.Error("the single trail opened empty; the board had its trail already")
	}
	if got := m.View(); !strings.Contains(got, "TRAIL · api") {
		t.Errorf("Lv1 is not the single trail:\n%s", got)
	}
	pressTab(m)
	pressTab(m)
	if m.level != levelReader {
		t.Errorf("three tabs from the board reached Lv%d, want the reader", m.level)
	}
	for m.level > levelBoard {
		m.zoomOut()
	}
	if got := m.View(); !strings.Contains(got, "flake in the checkout") {
		t.Errorf("shift+tab did not return to the board:\n%s", got)
	}
}

// refresh polls every column, not just the selected session: the targets are
// the board's keys with their transcripts.
func TestTheBoardPollsEveryColumn(t *testing.T) {
	m := boardModel(152, 30)
	targets := m.boardTargets()
	if len(targets) != 3 {
		t.Fatalf("got %d targets, want 3: %+v", len(targets), targets)
	}
	for _, tg := range targets {
		if tg.path == "" {
			t.Errorf("target %q has no transcript path", tg.key)
		}
	}
	narrow := boardModel(100, 30)
	if got := narrow.boardTargets(); got != nil {
		t.Errorf("a narrow deck polls the board: %+v", got)
	}
}

// The mirror is a Lv1 panel; on the board it is never captured into, even
// when it is switched on.
func TestNoCaptureOnTheBoard(t *testing.T) {
	m := boardModel(152, 30)
	press(m, "m")
	if m.capture() != nil {
		t.Error("the board polls a pane for a mirror it does not draw")
	}
	if m.note == "" {
		t.Error("m on the board said nothing about where the mirror shows")
	}
}

// An idle column is drawn dim, an archived one too; a working session's or
// one that needs you keeps its tints. The dimming itself is a style, which a
// test cannot see; the decision is what is checked.
func TestIdleColumnsAreDim(t *testing.T) {
	m := boardModel(152, 30)
	at := func(st state.State) fleet.Session { return fleet.Session{Snap: state.Snapshot{State: st}, Live: true} }
	for _, tc := range []struct {
		name  string
		s     fleet.Session
		muted bool
	}{
		{"idle", at(state.Idle), true},
		{"working", at(state.Working), false},
		{"needs you", at(state.NeedsYou), false},
		{"stuck", at(state.Stuck), false},
	} {
		if got := m.boardMuted(tc.s); got != tc.muted {
			t.Errorf("%s column muted = %v, want %v", tc.name, got, tc.muted)
		}
	}
	m.archiveView = true
	if !m.boardMuted(at(state.Working)) {
		t.Error("an archived column is not muted; the archive can never be moving")
	}
	_ = tmuxop.Pane{}
}
