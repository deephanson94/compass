package ui

import (
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/tmuxop"
)

// These are the amendments the first operator review asked for — four
// readers, four fleets, one afternoon — each pinned by the test that fails
// when it is taken back out.

// A long trail: thirty closed legs an hour apart, so it does not fit any
// column and its rail crosses many hours.
func hourlyTrail(base time.Time, legs int) journey.Trail {
	tr := journey.Trail{Prompts: []journey.Prompt{{Text: "port the whole thing", At: base}}}
	for i := 0; i < legs; i++ {
		start := base.Add(time.Duration(i)*time.Hour + time.Minute)
		tr.Legs = append(tr.Legs, journey.Leg{
			Class: journey.Build, Label: "file" + string(rune('a'+i%26)) + ".go",
			Start: start, End: start.Add(30 * time.Minute), Files: []string{"x.go"},
		})
	}
	tr.Legs[len(tr.Legs)-1].Current = true
	return tr
}

func renderLv(tr journey.Trail, now time.Time, level, w, h int) string {
	return RenderTrail(tr, TrailOpts{Now: now, Width: w, Height: h, Level: level, Cursor: -1, Pinned: true})
}

// The board column's second row is the journey's own verdict, read off its
// tail: what shipped, how the suite stands, what is still out.
func TestBoardVerdictReadsTheTrailsTail(t *testing.T) {
	base := fixtureBase
	now := base.Add(60 * time.Minute)
	s := fleet.Session{Snap: state.Snapshot{State: state.Idle}}
	red := journey.Leg{Class: journey.Test, Label: "pytest", Start: base.Add(10 * time.Minute), End: base.Add(12 * time.Minute),
		Waypoints: []journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "18 passed · 2 failed", Short: "18✓ 2✗"}}}
	green := journey.Leg{Class: journey.Test, Label: "go test", Start: base.Add(20 * time.Minute), End: base.Add(22 * time.Minute),
		Waypoints: []journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "312 passed", Short: "312✓"}}}
	fix := journey.Leg{Class: journey.Fix, Label: "tokens.py", Start: base.Add(30 * time.Minute), End: base.Add(35 * time.Minute), Files: []string{"tokens.py"}}
	ship := journey.Leg{Class: journey.Ship, Label: "git commit", Start: base.Add(40 * time.Minute), End: base.Add(41 * time.Minute)}

	for _, tc := range []struct {
		name string
		tr   journey.Trail
		want []string
		not  []string
	}{
		{"green suite", journey.Trail{Legs: []journey.Leg{green}}, []string{"✓ green 312✓"}, []string{"edited since"}},
		{"red then fixed, not rerun", journey.Trail{Legs: []journey.Leg{red, fix}}, []string{"✗ red 18✓ 2✗ · edited since"}, nil},
		{"red then rerun green", journey.Trail{Legs: []journey.Leg{red, fix, green}}, []string{"✓ green 312✓"}, []string{"✗ red", "edited since"}},
		{"shipped", journey.Trail{Legs: []journey.Leg{green, ship}}, []string{"✓ shipped 19m ago", "✓ green 312✓"}, nil},
		{"agents out", journey.Trail{Legs: []journey.Leg{green}, Branches: []journey.Branch{
			{Label: "a", Start: base.Add(50 * time.Minute)}, {Label: "b", Start: base.Add(55 * time.Minute)}, {Label: "c", Start: base, Done: true, End: base}}},
			[]string{"◈2 out · oldest 10m"}, nil},
		{"nothing countable", journey.Trail{Legs: []journey.Leg{fix}}, []string{"fix tokens.py"}, nil},
		{"only HEAD", journey.Trail{Legs: []journey.Leg{{Class: journey.Scout, Start: base, Current: true}}}, nil, []string{"scout"}},
	} {
		got := boardVerdict(s, tc.tr, now)
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: verdict %q lacks %q", tc.name, got, w)
			}
		}
		for _, n := range tc.not {
			if strings.Contains(got, n) {
				t.Errorf("%s: verdict %q should not say %q", tc.name, got, n)
			}
		}
	}
}

// The verdict is drawn on a working or idle column and never over the
// sentence a needs-you or stuck column owes its reader.
func TestVerdictLineYieldsToAttention(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	api := sessionKey("s-api")
	if got := strings.Join(m.boardColumn(api, rowFor(t, m, api), 40, 20), "\n"); !strings.Contains(got, "✗ red 18✓ 2✗") {
		t.Errorf("a working column lost its verdict:\n%s", got)
	}
	infra := sessionKey("s-infra")
	col := m.boardColumn(infra, rowFor(t, m, infra), 40, 20)
	if !strings.Contains(col[1], "AskUserQuestion") || strings.Contains(col[1], "scout") {
		t.Errorf("a needs-you column's second row is not its question:\n%s", strings.Join(col, "\n"))
	}
}

// The tmux session the fleet's grouping named is on the column's second row,
// marked as a pane so "work" does not read as a state.
func TestBoardColumnNamesItsTmuxSession(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	key := sessionKey("s-infra")
	got := strings.Join(m.boardColumn(key, rowFor(t, m, key), 40, 20), "\n")
	if !strings.Contains(got, "⌁ ops") {
		t.Errorf("the column does not name its tmux session:\n%s", got)
	}
	delete(m.panes, key)
	if got := strings.Join(m.boardColumn(key, rowFor(t, m, key), 40, 20), "\n"); strings.Contains(got, "⌁") {
		t.Errorf("a paneless column invented a tmux session:\n%s", got)
	}
}

// A bright column that was never opened says what its brightness stands for.
func TestNeverOpenedColumnCountsItsLegs(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	api := sessionKey("s-api")
	if got := strings.Join(m.boardColumn(api, rowFor(t, m, api), 40, 20), "\n"); !strings.Contains(got, "↳ 4 legs · never opened") {
		t.Errorf("a never-opened column does not say so:\n%s", got)
	}
	// One leg is "1 leg".
	m.trails[api] = journey.Trail{Legs: m.trails[api].Legs[:1]}
	if got := strings.Join(m.boardColumn(api, rowFor(t, m, api), 40, 20), "\n"); !strings.Contains(got, "↳ 1 leg · never opened") {
		t.Errorf("one leg is not 'legs':\n%s", got)
	}
	// A dim column is history: nothing to count.
	tf := sessionKey("s-tfstate")
	m.markSeen(tf)
	if got := strings.Join(m.boardColumn(tf, rowFor(t, m, tf), 40, 20), "\n"); strings.Contains(got, "never opened") {
		t.Errorf("a column that was opened claims it never was:\n%s", got)
	}
}

// A column pinned to the present with a day above it says so on its first
// row, and how long ago the journey began.
func TestPinnedColumnSaysWhatIsAbove(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	api := sessionKey("s-api")
	m.trails[api] = hourlyTrail(fixtureBase.Add(-30*time.Hour), 30)
	got := m.boardColumn(api, rowFor(t, m, api), 40, 12)
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "legs above · began 1d ago") {
		t.Errorf("a cut column does not say what is above it:\n%s", joined)
	}
	if !strings.Contains(got[3], "↑") {
		t.Errorf("the notice is not the column's first trail row:\n%s", joined)
	}
	// Short enough to fit: no notice.
	m.trails[api] = fixtureTrail(fixtureBase)
	if got := strings.Join(m.boardColumn(api, rowFor(t, m, api), 40, 20), "\n"); strings.Contains(got, "legs above") {
		t.Errorf("a column showing everything claims to hide legs:\n%s", got)
	}
}

// A ship leg is never a tick — it is the landing — and it is named by its
// commit.
func TestShipLegIsTheLandingNotATick(t *testing.T) {
	forceASCII(t)
	base := fixtureBase
	ship := journey.Leg{Class: journey.Ship, Label: "git commit", Start: base.Add(10 * time.Minute), End: base.Add(11 * time.Minute)}
	tr := journey.Trail{Legs: []journey.Leg{
		{Class: journey.Build, Label: "a.go", Start: base, End: base.Add(5 * time.Minute), Files: []string{"a.go"}},
		ship,
		{Class: journey.Scout, Label: "b.go", Start: base.Add(20 * time.Minute), Current: true},
	}}
	got := renderLv(tr, base.Add(30*time.Minute), 1, 44, 20)
	if !strings.Contains(got, "◆ ship") {
		t.Errorf("a ship leg with nothing parsed was demoted to a tick:\n%s", got)
	}
	tr.Legs[1].Waypoints = []journey.Waypoint{{Kind: journey.WaypointCommit, Text: "board: bright while unread", At: base.Add(11 * time.Minute)}}
	if got := renderLv(tr, base.Add(30*time.Minute), 1, 44, 20); !strings.Contains(got, "ship   board: bright while unread") {
		t.Errorf("a ship leg does not say what shipped:\n%s", got)
	}
	// A test leg with no verdict is a run with no verdict: "?".
	tr.Legs[1] = journey.Leg{Class: journey.Test, Label: "pytest", Start: base.Add(10 * time.Minute), End: base.Add(11 * time.Minute)}
	if got := renderLv(tr, base.Add(30*time.Minute), 1, 44, 20); !strings.Contains(got, "test") || !strings.Contains(got, "?  1m") {
		t.Errorf("a test tick has no '?' verdict:\n%s", got)
	}
}

// A subagent lane carries its clock and its finding at every level: how long
// it has been out, or how long ago it came back and with what.
func TestBranchLanesCarryTheirClockAndFinding(t *testing.T) {
	forceASCII(t)
	base := fixtureBase
	now := base.Add(40 * time.Minute)
	tr := fixtureLv2Trail(base) // one returned branch with a report
	lv1 := renderLv(tr, now, 1, 44, 24)
	if !strings.Contains(lv1, "✓ 19m") {
		t.Errorf("a returned lane has no clock at Lv1:\n%s", lv1)
	}
	if !strings.Contains(lv1, "payments never touches refresh") {
		t.Errorf("a returned lane hides its finding at Lv1:\n%s", lv1)
	}
	tr.Branches[0].Report = ""
	if got := renderLv(tr, now, 1, 44, 24); !strings.Contains(got, "came back with no report") {
		t.Errorf("a lane that returned nothing does not say so:\n%s", got)
	}
	tr.Branches[0].Done, tr.Branches[0].End = false, time.Time{}
	got := renderLv(tr, now, 1, 44, 24)
	if !strings.Contains(got, "⋯ 24m") {
		t.Errorf("an open lane has no clock:\n%s", got)
	}
	if strings.Contains(got, "no report") {
		t.Errorf("an open lane is judged before it returns:\n%s", got)
	}
}

// The Lv2 cursor is a mark on the row, not a tint: the row it stands on
// begins with ▸, whatever the terminal's colours do.
func TestCursorRowIsMarked(t *testing.T) {
	forceASCII(t)
	tr := fixtureLv2Trail(fixtureBase)
	got := RenderTrail(tr, TrailOpts{Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: 24, Level: 2, Cursor: 1})
	rows := TrailRows(tr, 2)
	marked := 0
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "▸") {
			marked++
			if !strings.Contains(line, rows[1].Text[:8]) {
				t.Errorf("▸ marks %q, not the cursor row %q", line, rows[1].Text)
			}
		}
	}
	if marked != 1 {
		t.Errorf("%d rows carry ▸, want exactly one:\n%s", marked, got)
	}
	if strings.Contains(RenderTrail(tr, TrailOpts{Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: 24, Level: 1, Cursor: -1}), "▸") {
		t.Errorf("Lv1 has no cursor and draws one")
	}
}

// A trail that spans hours puts the clock on its rail where the hour turns.
func TestLongTrailCarriesHourRules(t *testing.T) {
	forceASCII(t)
	base := time.Date(2026, 9, 2, 9, 40, 0, 0, time.UTC)
	rule := regexp.MustCompile(`│ \d\d:\d\d ─+`)
	got := renderLv(hourlyTrail(base, 4), base.Add(4*time.Hour), 1, 44, 30)
	if n := len(rule.FindAllString(got, -1)); n != 3 {
		t.Errorf("a four-hour trail carries %d hour rules, want 3:\n%s", n, got)
	}
	// Three legs inside half an hour, across the hour: too short a trail
	// for a clock.
	short := journey.Trail{Legs: []journey.Leg{
		{Class: journey.Build, Label: "a.go", Start: base, End: base.Add(8 * time.Minute), Files: []string{"a.go"}},
		{Class: journey.Build, Label: "b.go", Start: base.Add(10 * time.Minute), End: base.Add(18 * time.Minute), Files: []string{"b.go"}},
		{Class: journey.Fix, Label: "a.go", Start: base.Add(25 * time.Minute), Current: true},
	}}
	if got := renderLv(short, base.Add(30*time.Minute), 1, 44, 30); rule.MatchString(got) {
		t.Errorf("a half-hour trail carries an hour rule:\n%s", got)
	}
}

// The first dashed rail says how far along the plan is.
func TestGhostRailCountsWhatIsToGo(t *testing.T) {
	forceASCII(t)
	tr := fixtureTrail(fixtureBase)
	tr.Tasks = []journey.Task{
		{ID: "1", Subject: "Find the bug", Status: "completed"},
		{ID: "2", Subject: "Fix the refresh", Status: "in_progress"},
		{ID: "3", Subject: "Add a regression test", Status: "pending"},
		{ID: "4", Subject: "Ship it", Status: "pending"},
		{ID: "5", Subject: "Old idea", Status: "deleted"},
	}
	got := RenderTrail(tr, TrailOpts{Todos: planItems(tr.Tasks), Now: fixtureBase.Add(40 * time.Minute), Width: 44, Height: 30, Level: 1, Cursor: -1, Pinned: true})
	if !strings.Contains(got, "┊ 2 of 4 to go") {
		t.Errorf("the ghost rail does not count the plan:\n%s", got)
	}
}

// One touched file is a Lv2 row: on a HEAD leg twenty minutes in, it is the
// only thing the row can say.
func TestOneTouchedFileIsARow(t *testing.T) {
	forceASCII(t)
	got := RenderTrail(fixtureLv2Trail(fixtureBase), TrailOpts{Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: 24, Level: 2, Cursor: -1})
	if !strings.Contains(got, "└ touched tokens.py") {
		t.Errorf("a one-file build leg has no touched row:\n%s", got)
	}
}

// The trail's title counts the legs its viewport hides above the fold.
func TestPinnedTrailTitleCountsLegsAbove(t *testing.T) {
	forceASCII(t)
	m := boardModel(120, 20)
	openTrail(m)
	m.SetTrail(hourlyTrail(fixtureBase.Add(-30*time.Hour), 30))
	if got := m.trailTitle(60); !strings.Contains(got, "↑ ") || !strings.Contains(got, "legs  [Lv1]") {
		t.Errorf("a cut trail's title does not count what is above:\n%s", got)
	}
	m.SetTrail(fixtureTrail(fixtureBase))
	if got := m.trailTitle(60); strings.Contains(got, "↑") {
		t.Errorf("a whole trail's title claims hidden legs:\n%s", got)
	}
}

// Tab into Lv2 lands the cursor on the first row on screen, not on a row
// thirty legs above the fold.
func TestZoomInLandsOnTheFirstVisibleRow(t *testing.T) {
	forceASCII(t)
	m := boardModel(120, 20)
	openTrail(m)
	m.SetTrail(hourlyTrail(fixtureBase.Add(-30*time.Hour), 30))
	// Pinned to the present: Tab lands on the newest row, as it always did.
	pressTab(m)
	if m.level != levelWaypoints {
		t.Fatalf("level = %d, want Lv2", m.level)
	}
	last := len(TrailRows(m.trail, levelWaypoints)) - 1
	if m.cursor != last {
		t.Errorf("cursor = %d on a pinned trail, want the newest row %d", m.cursor, last)
	}
	// Scrolled back: Tab lands where the eye is.
	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	pressCtrl(m, tea.KeyCtrlU)
	pressCtrl(m, tea.KeyCtrlU)
	if m.trailPinned {
		t.Fatalf("ctrl+u did not unpin the trail")
	}
	pressTab(m)
	want := m.firstRowInView()
	if want <= 0 || want >= last {
		t.Fatalf("firstRowInView = %d, want somewhere inside the trail (last %d)", want, last)
	}
	if m.cursor != want {
		t.Errorf("cursor = %d on a scrolled trail, want %d (the first row in view)", m.cursor, want)
	}
}

// ctrl+d / ctrl+u at Lv2 move the cursor half a screen, not one row.
func TestHalfPageMovesTheCursorAtLv2(t *testing.T) {
	forceASCII(t)
	m := boardModel(120, 30)
	openTrail(m)
	m.SetTrail(hourlyTrail(fixtureBase.Add(-30*time.Hour), 30))
	pressTab(m)
	m.cursor = 0
	pressCtrl(m, tea.KeyCtrlD)
	if want := m.trailHalfPage(); m.cursor != want {
		t.Errorf("ctrl+d moved the cursor to %d, want %d", m.cursor, want)
	}
	pressCtrl(m, tea.KeyCtrlU)
	if m.cursor != 0 {
		t.Errorf("ctrl+u brought the cursor to %d, want 0", m.cursor)
	}
}

// A from the board opens the archive as a list: three hundred columns of
// "reading its transcript…" answer nothing.
func TestArchiveFromTheBoardIsAList(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	if m.level != levelBoard {
		t.Fatalf("the deck did not open on the board")
	}
	press(m, "A")
	if !m.archiveView {
		t.Fatalf("A did not open the archive")
	}
	if m.level != levelTrail {
		t.Errorf("the archive opened at level %d, want the trail (Lv1)", m.level)
	}
}

// g says where it went, because the session it grabbed may not be the one on
// screen.
func TestGrabSaysWhereItWent(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	press(m, "g")
	if m.note != "→ infra · ops:1.0" {
		t.Errorf("note = %q, want the grabbed session and its pane", m.note)
	}
}

// A fleet list cut at the panel's edge says how many it is not showing.
func TestCutFleetSaysHowManyMore(t *testing.T) {
	forceASCII(t)
	base := fixtureBase
	m := New(nil)
	m.SetSize(100, 14)
	var ss []fleet.Session
	for i := 0; i < 9; i++ {
		n := string(rune('a' + i))
		ss = append(ss, sess("s-"+n, n, "/home/user/p"+n, "main", "task "+n, state.Idle, base.Add(time.Duration(i)*time.Minute), journey.Build, "", "turn complete", "idle"))
	}
	m.SetSessions(ss, base.Add(40*time.Minute))
	openTrail(m)
	col := strings.Join(fleetText(m, 100, 14), "\n")
	if !strings.Contains(col, "more below · j") {
		t.Errorf("a cut list does not count what is below:\n%s", col)
	}
	if strings.Contains(col, "more above") {
		t.Errorf("a list at its top claims rows above:\n%s", col)
	}
	for i := 0; i < 8; i++ {
		press(m, "j")
	}
	col = strings.Join(fleetText(m, 100, 14), "\n")
	if !strings.Contains(col, "more above · k") {
		t.Errorf("a scrolled list does not count what is above:\n%s", col)
	}
}

// The header's warm chips carry their wait; a calm fleet says so; the
// archive counts itself.
func TestHeaderChipsCarryTheWait(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	if got := m.statusChips(); !strings.Contains(got, "▲1 2m") {
		t.Errorf("chips = %q, want the needs-you wait", got)
	}
	if got := m.statusChips(); strings.Contains(got, "all calm") {
		t.Errorf("chips = %q, claim calm with a session waiting", got)
	}
	press(m, "A")
	if got := m.statusChips(); !strings.Contains(got, "archive 5") {
		t.Errorf("chips = %q, want the archive count", got)
	}
	press(m, "A")
	for i := range m.sessions {
		if m.sessions[i].Snap.State == state.NeedsYou {
			m.sessions[i].Snap.State = state.Idle
		}
	}
	if got := m.statusChips(); !strings.Contains(got, "all calm") || strings.Contains(got, "▲") {
		t.Errorf("chips = %q, want 'all calm' with nothing waiting", got)
	}
}

// The reader says it is reading while the trail is ahead of it.
func TestReaderSaysItIsReadingWhenTheTrailIsAhead(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	m.events = nil
	got := strings.Join(m.readerColumn(40, 10), "\n")
	if !strings.Contains(got, "reading the transcript…") {
		t.Errorf("a reader behind its trail claims there is nothing:\n%s", got)
	}
	m.trail = journey.Trail{}
	if got := strings.Join(m.readerColumn(40, 10), "\n"); strings.Contains(got, "reading the transcript") {
		t.Errorf("a session with no trail claims to be reading one:\n%s", got)
	}
}

// A needs-you row leads with the question, not with "waiting on your answer",
// which the first line already said.
func TestNeedsYouRowLeadsWithTheQuestion(t *testing.T) {
	forceASCII(t)
	s := fixtureGroupedFleet(fixtureBase)[0]
	s.Snap.Activity = "Open 22 to the office CIDR, or keep the bastion? [office / bastion]"
	m := New(nil)
	m.SetSize(120, 30)
	got := m.secondLine(s, 60)
	if !strings.HasPrefix(strings.TrimSpace(got), "◆ design Open 22 to the office CIDR") {
		t.Errorf("second line = %q, want the question first", got)
	}
	if strings.Contains(got, "waiting on your answer") {
		t.Errorf("second line = %q, repeats the state the glyph gave", got)
	}
	// A stuck row still leads with why: the command alone would not say it.
	s.Snap = state.Snapshot{State: state.Stuck, Reason: "no output for 4m mid-turn", Activity: "Bash: python backfill.py"}
	if got := m.secondLine(s, 80); !strings.Contains(got, "no output for 4m mid-turn · Bash: python backfill.py") {
		t.Errorf("stuck second line = %q, want reason then command", got)
	}
}

// Selecting a session the board already holds hands the trail over at once.
func TestSelectionTakesTheTrailTheBoardHolds(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	m.point(sessionKey("s-webapp"))
	if len(m.trail.Legs) != 2 || m.trail.Legs[1].Label != "checkout.py" {
		t.Errorf("point() left the trail empty: %+v", m.trail.Legs)
	}
	m.point(sessionKey("s-scratch"))
	if len(m.trail.Legs) != 0 || len(m.trail.Prompts) != 1 {
		t.Errorf("point() did not take scratch's own trail: %+v", m.trail)
	}
}

// Every panel the pane list can name, boardModel's panes name: keep the
// fixture honest.
func TestFixturePanesNameInfra(t *testing.T) {
	panes, _ := fixtureGroupedPanes()
	if p, ok := panes[sessionKey("s-infra")]; !ok || p.Target == "" {
		t.Fatalf("fixture: infra has no pane: %+v", tmuxop.Pane{})
	}
}
