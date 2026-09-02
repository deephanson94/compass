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
		{"shipped on red", journey.Trail{Legs: []journey.Leg{red, fix, ship}}, []string{"✗ red 18✓ 2✗ · shipped on red"}, []string{"✓ shipped", "edited since"}},
		{"rerunning", journey.Trail{Legs: []journey.Leg{red, fix, {Class: journey.Test, Label: "pytest", Start: base.Add(50 * time.Minute), Current: true}}},
			[]string{"✗ red 18✓ 2✗ · rerunning for 10m"}, []string{"edited since"}},
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

// The fleet row for a working session is its present: HEAD — class, what,
// how long — with the agents it has out. (The board column's second row is
// the verdict instead: see TestWorkingColumnHeaderSaysWhatHeadCannot.)
func TestFleetRowIsThePresentForAWorkingSession(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	api := sessionKey("s-api")
	if fleet := m.secondLine(m.sessions[rowFor(t, m, api).sess], 40); !strings.Contains(fleet, "● fix    tokens.py") || !strings.HasSuffix(fleet, "for 3m") {
		t.Errorf("the fleet row is not HEAD with its figure flush right: %q", fleet)
	}
	tr := m.trails[api]
	tr.Branches = append(tr.Branches, journey.Branch{Label: "measure", Start: fixtureBase.Add(30 * time.Minute), AfterLeg: 3})
	m.trails[api] = tr
	if fleet := m.secondLine(m.sessions[rowFor(t, m, api).sess], 40); !strings.Contains(fleet, "◈1 out · for 3m") {
		t.Errorf("a working row with an agent out does not say so: %q", fleet)
	}
	// A quiet session: the verdict.
	tf := sessionKey("s-tfstate")
	if fleet := m.secondLine(m.sessions[rowFor(t, m, tf).sess], 40); !strings.Contains(fleet, "build state.tf") {
		t.Errorf("an idle row's second line is not its verdict: %q", fleet)
	}
	// One that needs you: the question, whole, no class in front of it.
	infra := sessionKey("s-infra")
	col := m.boardColumn(infra, rowFor(t, m, infra), 40, 20)
	if !strings.HasPrefix(strings.TrimSpace(col[1]), "AskUserQuestion") || strings.Contains(col[1], "◆") {
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

// A never-opened column keeps its third row as air: the brightness already
// says it is unread, and a count on every column of a fresh launch was a
// constant wearing a row.
func TestNeverOpenedColumnKeepsTheRowAsAir(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	api := sessionKey("s-api")
	col := m.boardColumn(api, rowFor(t, m, api), 70, 20)
	if strings.TrimSpace(col[2]) != "" {
		t.Errorf("a never-opened column spends its third row: %q", col[2])
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
	// A test leg with no verdict is a row with "?": never a tick.
	tr.Legs[1] = journey.Leg{Class: journey.Test, Label: "pytest", Start: base.Add(10 * time.Minute), End: base.Add(11 * time.Minute)}
	if got := renderLv(tr, base.Add(30*time.Minute), 1, 44, 20); !strings.Contains(got, "◆ test   pytest") || !strings.Contains(got, "? 1m") {
		t.Errorf("a test leg with no verdict lost its row or its '?':\n%s", got)
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
	if !strings.Contains(got, "┊ 2 of 4 tasks to go") {
		t.Errorf("the ghost rail does not count the plan:\n%s", got)
	}
}

// A touched row says what the label did not: one file that is the label is
// the label restated, and a row that restates its parent is a dead row.
func TestTouchedRowNeverRestatesTheLabel(t *testing.T) {
	forceASCII(t)
	tr := fixtureLv2Trail(fixtureBase)
	opts := TrailOpts{Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: 24, Level: 2, Cursor: -1}
	if got := RenderTrail(tr, opts); strings.Contains(got, "└ touched tokens.py") {
		t.Errorf("'build tokens.py' restates itself as 'touched tokens.py':\n%s", got)
	}
	tr.Legs[3].Files = []string{"src/auth/tokens.py"}
	if got := RenderTrail(tr, opts); strings.Contains(got, "touched src/auth/tokens.py") {
		t.Errorf("the same file by its path is still the label:\n%s", got)
	}
	tr.Legs[3].Files = []string{"conftest.py"}
	if got := RenderTrail(tr, opts); !strings.Contains(got, "└ touched conftest.py") {
		t.Errorf("a file the label does not name is worth a row:\n%s", got)
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
	if !strings.HasPrefix(strings.TrimSpace(got), "Open 22 to the office CIDR") {
		t.Errorf("second line = %q, want the question, whole", got)
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

// A tick row's figure ends where every other row's does.
func TestTickRowIsFlushWithTheEdge(t *testing.T) {
	forceASCII(t)
	base := fixtureBase
	tr := journey.Trail{Legs: []journey.Leg{
		{Class: journey.Build, Label: "a.go", Start: base, End: base.Add(5 * time.Minute), Files: []string{"a.go"}},
		{Class: journey.Design, Label: "design", Start: base.Add(10 * time.Minute), End: base.Add(21 * time.Minute)},
		{Class: journey.Scout, Label: "b.go", Start: base.Add(30 * time.Minute), Current: true},
	}}
	var tick, leg string
	for _, line := range strings.Split(renderLv(tr, base.Add(40*time.Minute), 1, 44, 20), "\n") {
		switch {
		case strings.Contains(line, " design") && !strings.HasPrefix(line, "◆"):
			tick = line // on the rail, under the cap
		case strings.HasPrefix(line, "◆ build"):
			leg = line
		}
	}
	if tick == "" || leg == "" {
		t.Fatalf("fixture rows missing: tick %q, leg %q", tick, leg)
	}
	if strings.TrimRight(tick, " ") != tick || len([]rune(tick)) != len([]rune(leg)) {
		t.Errorf("the tick's figure is not flush with the leg's:\n%q\n%q", leg, tick)
	}
}

// A Lv1 trail longer than its panel gives up the air between its legs, so
// the panel holds twice the journey; Lv2 keeps its rails, and so does a
// trail that fits.
func TestLongTrailDrawsDense(t *testing.T) {
	forceASCII(t)
	rails := func(doc []string) int {
		n := 0
		for _, l := range doc {
			if strings.TrimSpace(l) == "│" {
				n++
			}
		}
		return n
	}
	long := longTrail(30)
	now := fixtureBase.Add(3 * time.Hour)
	if n := rails(TrailLines(long, TrailOpts{Now: now, Width: 44, Height: 12, Level: 1, Cursor: -1, Pinned: true})); n != 0 {
		t.Errorf("a trail that does not fit still has %d rail rows at Lv1", n)
	}
	if n := rails(TrailLines(long, TrailOpts{Now: now, Width: 44, Height: 12, Level: 2, Cursor: -1, Pinned: true})); n != 0 {
		t.Errorf("a trail that does not fit still has %d rail rows at Lv2", n)
	}
	if n := rails(TrailLines(fixtureLv2Trail(fixtureBase), TrailOpts{Now: now, Width: 44, Height: 40, Level: 2, Cursor: -1, Pinned: true})); n == 0 {
		t.Errorf("a Lv2 trail that fits lost its rails")
	}
	if n := rails(TrailLines(long, TrailOpts{Now: now, Width: 44, Height: 200, Level: 1, Cursor: -1, Pinned: true})); n == 0 {
		t.Errorf("a trail that fits lost its rails")
	}
	// The plan drops its rails too: one dashed rail with the count, then
	// the ghosts, where "◌ / ┊ / ◌ / ┊ / ◌" was five rows for three steps.
	planned := longTrail(30)
	planned.Tasks = []journey.Task{{ID: "1", Subject: "a", Status: "pending"}, {ID: "2", Subject: "b", Status: "pending"}, {ID: "3", Subject: "c", Status: "pending"}}
	dashed := 0
	for _, l := range TrailLines(planned, TrailOpts{Todos: planItems(planned.Tasks), Now: now, Width: 44, Height: 20, Level: 1, Cursor: -1, Pinned: true}) {
		if strings.HasPrefix(l, "┊") {
			dashed++
		}
	}
	if dashed != 1 {
		t.Errorf("a dense plan draws %d dashed rails, want 1", dashed)
	}
	// The rows that say something keep theirs: the hour rules survive.
	hourly := hourlyTrail(fixtureBase.Add(-30*time.Hour), 30)
	if got := strings.Join(TrailLines(hourly, TrailOpts{Now: fixtureBase, Width: 44, Height: 12, Level: 1, Cursor: -1, Pinned: true}), "\n"); !strings.Contains(got, "────") {
		t.Errorf("dense drawing dropped the hour rules:\n%s", got)
	}
}

// A prompt's figure says "ago": it is when, where a leg's is how long.
func TestPromptRowSaysAgo(t *testing.T) {
	forceASCII(t)
	got := renderLv(fixtureTrail(fixtureBase), fixtureBase.Add(40*time.Minute), 1, 44, 20)
	if !strings.Contains(got, "\"fix the 401 bug\"") || !strings.Contains(got, "38m ago") {
		t.Errorf("the prompt row has no 'ago':\n%s", got)
	}
}

// A finding wraps to a second and third row rather than losing its second half.
func TestFindingWrapsToTwoRows(t *testing.T) {
	forceASCII(t)
	tr := fixtureLv2Trail(fixtureBase)
	tr.Branches[0].Report = "3 defects found against the oracle; two are the same root cause"
	got := renderLv(tr, fixtureBase.Add(40*time.Minute), 1, 40, 30)
	if !strings.Contains(got, "same root cause") {
		t.Errorf("the finding lost its second half:\n%s", got)
	}
	// Never more than two rows, however long.
	tr.Branches[0].Report = strings.Repeat("word ", 40)
	rows := 0
	for _, l := range strings.Split(renderLv(tr, fixtureBase.Add(40*time.Minute), 1, 40, 40), "\n") {
		if strings.Contains(l, "word word") {
			rows++
		}
	}
	if rows != 3 {
		t.Errorf("a long finding takes %d rows, want 3", rows)
	}
}

// Agents still out are in the header, and a fleet with any out is not calm.
func TestHeaderCountsAgentsOut(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	for i := range m.sessions {
		if m.sessions[i].Snap.State == state.NeedsYou {
			m.sessions[i].Snap.State = state.Idle
		}
	}
	if got := m.statusChips(); !strings.Contains(got, "all calm") {
		t.Fatalf("chips = %q, want calm before any agent is out", got)
	}
	api := sessionKey("s-api")
	tr := m.trails[api]
	tr.Branches = append(tr.Branches, journey.Branch{Label: "measure", Start: fixtureBase.Add(20 * time.Minute), AfterLeg: 3})
	m.trails[api] = tr
	got := m.statusChips()
	if !strings.Contains(got, "◈1 out · oldest 20m") {
		t.Errorf("chips = %q, want the agent out", got)
	}
	if strings.Contains(got, "all calm") {
		t.Errorf("chips = %q, calm with an agent out", got)
	}
}

// A group header's clock is the wait of the state it echoes, and a header
// over a single row carries no figures at all.
func TestGroupHeaderClockIsTheEchoedWait(t *testing.T) {
	forceASCII(t)
	m := groupedModel(120, 30)
	// Make another ops session the freshest, so the two clocks differ.
	for i := range m.sessions {
		if m.sessions[i].Info.ID == "s-tfstate" {
			m.sessions[i].Info.LastEventAt = m.now.Add(-30 * time.Second)
		}
	}
	var ops, dev fleetRow
	for _, r := range m.fleetRows() {
		if r.header && r.label == "ops" {
			ops = r
		}
		if r.header && r.label == "dev" {
			dev = r
		}
	}
	if ops.echo != "▲" || ops.age != "2m" {
		t.Errorf("ops header = %q %q, want the needs-you echo and its 2m wait", ops.echo, ops.age)
	}
	_ = dev
	// A header over one row: nothing but the name.
	m.SetSessions(m.sessions[:1], m.now)
	for _, r := range m.fleetRows() {
		if r.header && (r.age != "" || r.echo != "") {
			t.Errorf("a header over one row carries figures: %+v", r)
		}
	}
}

// The archive numbers its rows down the list as drawn.
func TestArchiveNumbersRunDownTheList(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	// Hand the archive over in the reverse of the order it draws, so the
	// list's order and the slice's are not the same thing by accident.
	var live, gone []fleet.Session
	for _, s := range m.sessions {
		if s.Live {
			live = append(live, s)
		} else {
			gone = append([]fleet.Session{s}, gone...)
		}
	}
	m.SetSessions(append(live, gone...), m.now)
	press(m, "A")
	want := 1
	for _, r := range m.fleetRows() {
		if r.header {
			continue
		}
		if want <= 9 && r.num != want {
			t.Fatalf("archive row numbered %d, want %d", r.num, want)
		}
		want++
	}
	m.selectIndex(2)
	rows := m.fleetRows()
	third := ""
	n := 0
	for _, r := range rows {
		if !r.header {
			n++
			if n == 3 {
				third = m.sessions[r.sess].Info.Key()
			}
		}
	}
	if m.selectedKey != third {
		t.Errorf("3 selected %q, want the third row drawn %q", m.selectedKey, third)
	}
}

// The help does not describe a board to a terminal that has none.
func TestHelpFitsTheTerminalItIsOn(t *testing.T) {
	forceASCII(t)
	with := strings.Join(helpLinesFor(120, 40, true), "\n")
	without := strings.Join(helpLinesFor(100, 40, false), "\n")
	if !strings.Contains(with, "board → trail") {
		t.Errorf("help with a board does not name it:\n%s", with)
	}
	if strings.Contains(without, "board") {
		t.Errorf("help without a board describes one:\n%s", without)
	}
}

// HEAD is named by what the state machine knows when it knows more: the
// hung call of a stuck session, the question of a waiting one.
func TestStuckHeadNamesTheHungCall(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	api := sessionKey("s-api")
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == api {
			m.sessions[i].Snap = state.Snapshot{State: state.Stuck, Since: fixtureBase.Add(36 * time.Minute), Reason: "no output for 4m mid-turn", Activity: "Bash: python backfill.py --all"}
		}
	}
	col := strings.Join(m.boardColumn(api, rowFor(t, m, api), 70, 20), "\n")
	if !strings.Contains(col, "◍ fix    Bash: python backfill.py --all") || !strings.Contains(col, "silent 4m") {
		t.Errorf("a stuck column's HEAD is not the hung call, wearing the fleet's glyph and its silence:\n%s", col)
	}
	if strings.Contains(col, "for 3m") {
		t.Errorf("a stuck HEAD still says 'for 3m' as if it were working:\n%s", col)
	}
	if !strings.Contains(col, "no output for 4m mid-turn · Bash: python backfill.py --all") {
		t.Errorf("a stuck column's second row is not the reason and the call:\n%s", col)
	}
	m.point(api)
	openTrail(m)
	if got := strings.Join(m.trailColumn(70, 20), "\n"); !strings.Contains(got, "◍ fix    Bash: python backfill.py --all") {
		t.Errorf("the single trail's HEAD is not the hung call:\n%s", got)
	}
}

// `[` and `]` step between prompts: the chapters of a trail. At Lv1 the
// viewport opens on the prompt; at Lv2 the cursor lands on it; the note
// says which chapter and when.
func TestPromptsAreChapters(t *testing.T) {
	forceASCII(t)
	m := boardModel(120, 24)
	openTrail(m)
	tr := longTrail(60)
	tr.Prompts = append(tr.Prompts,
		journey.Prompt{Text: "now the audit log", At: fixtureBase.Add(20 * time.Minute)},
		journey.Prompt{Text: "fix all the failures first", At: fixtureBase.Add(40 * time.Minute)},
	)
	m.SetTrail(tr)
	// Lv1, pinned to the present: `[` opens on the last prompt.
	press(m, "[")
	if m.trailPinned {
		t.Fatalf("[ did not move the viewport")
	}
	if !strings.Contains(m.note, "◉ 3/3") || !strings.Contains(m.note, "fix all the failures first") {
		t.Errorf("note = %q, want the third chapter", m.note)
	}
	rows := m.trailColumn(60, 22)
	if !strings.Contains(rows[2], "fix all the failures first") {
		t.Errorf("the viewport did not open on the prompt:\n%s", strings.Join(rows, "\n"))
	}
	press(m, "[")
	if !strings.Contains(m.note, "◉ 2/3") {
		t.Errorf("note = %q, want the second chapter", m.note)
	}
	press(m, "]")
	if !strings.Contains(m.note, "◉ 3/3") {
		t.Errorf("note = %q, want the third chapter again", m.note)
	}
	press(m, "]")
	if !strings.Contains(m.note, "no later prompt") {
		t.Errorf("note = %q, want the end of the chapters", m.note)
	}
	// Lv2: the cursor lands on the prompt row.
	press(m, "G")
	pressTab(m)
	press(m, "[")
	all := TrailRows(m.trail, levelWaypoints)
	if m.cursor < 0 || all[m.cursor].Kind != "prompt" || all[m.cursor].Text != "fix all the failures first" {
		t.Errorf("cursor = %d, not on the last prompt", m.cursor)
	}
	press(m, "[")
	if all[m.cursor].Text != "now the audit log" {
		t.Errorf("second [ did not reach the earlier prompt: %q", all[m.cursor].Text)
	}
	press(m, "[")
	press(m, "[")
	if !strings.Contains(m.note, "no earlier prompt") {
		t.Errorf("note = %q, want the start of the chapters", m.note)
	}
}

// ---- round three

// A tall board with short trails wraps into a second band of columns; a
// board whose first band holds a day-long trail keeps the whole height.
func TestTallBoardWrapsIntoTwoBands(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 48)
	got := strings.Join(m.boardLines(148, 43), "\n")
	for _, name := range []string{"tfstate", "scratch"} {
		if !strings.Contains(got, "○ "+name) {
			t.Errorf("a 48-row board left %s in the strip:\n%s", name, got)
		}
	}
	if strings.Contains(got, "+1 more") || strings.Contains(got, "+2 more") {
		t.Errorf("the strip still names sessions the second band could hold:\n%s", got)
	}
	// Two bands: the second's headers sit below the first's trails.
	lines := m.boardLines(148, 43)
	second := -1
	for i, l := range lines {
		if strings.Contains(l, "○ scratch") {
			second = i
		}
	}
	if second < 15 {
		t.Errorf("the second band's header is on row %d, want it below the first band", second)
	}
	// A short board: one band, the strip.
	if got := strings.Join(boardModel(152, 30).boardLines(148, 25), "\n"); !strings.Contains(got, "+1 more") {
		t.Errorf("a 30-row board wrapped where it had no room:\n%s", got)
	}
	// A long trail in the first band: one band, however tall.
	tall := boardModel(152, 48)
	tall.trails[sessionKey("s-api")] = longTrail(60)
	if got := strings.Join(tall.boardLines(148, 43), "\n"); !strings.Contains(got, "+1 more") {
		t.Errorf("a day-long trail was cut in half for a second band:\n%s", got)
	}
}

// A working column's second row is its verdict when it has one — HEAD is
// the trail's last row anyway — and the tmux session lands on the third
// row when the second had no room for it.
func TestWorkingColumnHeaderSaysWhatHeadCannot(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	api := sessionKey("s-api")
	col := m.boardColumn(api, rowFor(t, m, api), 40, 20)
	if !strings.Contains(col[1], "✗ red 18✓ 2✗") {
		t.Errorf("a working column's second row is not its verdict:\n%s", strings.Join(col, "\n"))
	}
	if !strings.Contains(col[2], "⌁ dev") {
		t.Errorf("the tmux session did not fall to the third row:\n%s", strings.Join(col, "\n"))
	}
	// With room on the second row, it stays there and the third is air.
	wide := m.boardColumn(api, rowFor(t, m, api), 70, 20)
	if !strings.Contains(wide[1], "⌁ dev") || strings.TrimSpace(wide[2]) != "" {
		t.Errorf("a wide column moved the tmux session off its second row:\n%s", strings.Join(wide, "\n"))
	}
	// No verdict: the present, as the fleet says it.
	tr := m.trails[api]
	tr.Legs[2].Waypoints = nil
	tr.Branches = nil
	m.trails[api] = tr
	if col := m.boardColumn(api, rowFor(t, m, api), 40, 20); !strings.Contains(col[1], "● fix    tokens.py") {
		t.Errorf("a working column with no verdict lost its HEAD line:\n%s", strings.Join(col, "\n"))
	}
}

// Agents all back are a verdict too: how many, and how many said nothing.
func TestVerdictCountsAgentsBack(t *testing.T) {
	base := fixtureBase
	tr := journey.Trail{Legs: []journey.Leg{{Class: journey.Docs, Label: "SKILL.md", Start: base, End: base.Add(time.Minute), Files: []string{"SKILL.md"}}},
		Branches: []journey.Branch{
			{Label: "a", Start: base, End: base.Add(time.Minute), Done: true, Report: "found it"},
			{Label: "b", Start: base, End: base.Add(time.Minute), Done: true},
			{Label: "c", Start: base, End: base.Add(time.Minute), Done: true, Report: "nothing to add"},
		}}
	got := boardVerdict(fleet.Session{}, tr, base.Add(time.Hour))
	if !strings.Contains(got, "◈3 back · 1 empty") {
		t.Errorf("verdict = %q, want the agents back and the empty one", got)
	}
	tr.Branches[1].Report = "x"
	if got := boardVerdict(fleet.Session{}, tr, base.Add(time.Hour)); !strings.Contains(got, "◈3 back") || strings.Contains(got, "empty") {
		t.Errorf("verdict = %q, want '◈3 back' alone", got)
	}
	tr.Branches = append(tr.Branches, journey.Branch{Label: "d", Start: base})
	if got := boardVerdict(fleet.Session{}, tr, base.Add(time.Hour)); strings.Contains(got, "back") {
		t.Errorf("verdict = %q, counts agents back while one is still out", got)
	}
}

// A group header's clock never repeats the row beneath it.
func TestGroupHeaderClockNeverRepeatsItsFirstRow(t *testing.T) {
	forceASCII(t)
	m := groupedModel(120, 30)
	// Make the first dev row the freshest, so the header's clock would be
	// its clock.
	for i := range m.sessions {
		switch m.sessions[i].Info.ID {
		case "s-api":
			m.sessions[i].Info.LastEventAt, m.sessions[i].Snap.Since = m.now.Add(-40*time.Second), m.now.Add(-40*time.Second)
		case "s-webapp":
			m.sessions[i].Info.LastEventAt, m.sessions[i].Snap.Since = m.now.Add(-3*time.Minute), m.now.Add(-3*time.Minute)
		}
	}
	for _, r := range m.fleetRows() {
		if r.header && r.label == "dev" && r.age != "" {
			t.Errorf("dev header carries %q, the clock of the row beneath it", r.age)
		}
	}
	// Freshest deeper in the group: the header's clock is information.
	for i := range m.sessions {
		if m.sessions[i].Info.ID == "s-webapp" {
			m.sessions[i].Info.LastEventAt = m.now.Add(-10 * time.Second)
		}
	}
	found := false
	for _, r := range m.fleetRows() {
		if r.header && r.label == "dev" && r.age == "10s" {
			found = true
		}
	}
	if !found {
		t.Errorf("a header over a group whose freshest row is not first lost its clock")
	}
}

// A cut fleet never ends its window on a header.
func TestCutFleetNeverEndsOnAHeader(t *testing.T) {
	forceASCII(t)
	m := groupedModel(120, 30)
	openTrail(m)
	rows := m.fleetRows()
	full, selStart, selEnd := m.fleetBlock(rows, fleetWidth)
	for h := 4; h < len(full); h++ {
		m.fleetScroll = 0
		out := m.scrollFleet(full, selStart, selEnd, h)
		for i := len(out) - 1; i >= 0; i-- {
			l := out[i]
			if strings.HasPrefix(l, "▾") || l == "" {
				continue
			}
			if isHeaderLine(l) {
				t.Errorf("height %d: the window ends on the header %q", h, l)
			}
			break
		}
	}
}

// A from the live view with nothing remembered selects the first archived
// row, and the trail beside it is that row's.
func TestArchiveOpensOnItsFirstRow(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	press(m, "A")
	first := ""
	for _, r := range m.fleetRows() {
		if !r.header {
			first = m.sessions[r.sess].Info.Key()
			break
		}
	}
	if m.selectedKey != first {
		t.Errorf("the archive opened on %q, want its first row %q", m.selectedKey, first)
	}
	if got := m.trailTitle(60); !strings.Contains(got, sessionName(m.sessions[m.selectedIndex()].Info)) {
		t.Errorf("the trail beside the archive is not the selected row's: %q", got)
	}
}

// Keys that move nothing say why.
func TestDeadKeysSayWhy(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	openTrail(m)
	pressTab(m) // Lv2, cursor on the newest row
	press(m, "j")
	if !strings.Contains(m.note, "at the present") {
		t.Errorf("j on the newest row said %q", m.note)
	}
	m.cursor = 0
	press(m, "k")
	if !strings.Contains(m.note, "at the start") {
		t.Errorf("k on the first row said %q", m.note)
	}
	narrow := boardModel(100, 30)
	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	narrow.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if !strings.Contains(narrow.note, "no board under") {
		t.Errorf("shift+tab without a board said %q", narrow.note)
	}
}

// Prompt rows carry their chapter number when there is more than one.
func TestPromptRowsCountTheirChapter(t *testing.T) {
	forceASCII(t)
	tr := fixtureTrail(fixtureBase)
	if got := renderLv(tr, fixtureBase.Add(40*time.Minute), 1, 44, 20); strings.Contains(got, "◉ 1/1") {
		t.Errorf("a lone prompt is numbered:\n%s", got)
	}
	tr.Prompts = append(tr.Prompts, journey.Prompt{Text: "now the tests", At: fixtureBase.Add(30 * time.Minute)})
	got := renderLv(tr, fixtureBase.Add(40*time.Minute), 1, 44, 20)
	if !strings.Contains(got, "◉ 1/2 \"fix the 401 bug\"") || !strings.Contains(got, "◉ 2/2 \"now the tests\"") {
		t.Errorf("prompts are not numbered as chapters:\n%s", got)
	}
}

// The first hour rule of a new day names the day.
func TestHourRuleNamesANewDay(t *testing.T) {
	forceASCII(t)
	base := time.Date(2026, 9, 1, 22, 40, 0, 0, time.UTC)
	got := renderLv(hourlyTrail(base, 4), base.Add(4*time.Hour), 1, 44, 30)
	if !strings.Contains(got, "│ Wed 00:41") {
		t.Errorf("the rule past midnight does not name the day:\n%s", got)
	}
	if !strings.Contains(got, "│ 23:41 ") {
		t.Errorf("a rule inside the day carries a day name:\n%s", got)
	}
}

// A Lv2 child never restates its parent: the run summary the badge carries,
// the commit a ship leg is named by.
func TestLv2ChildNeverRestatesItsParent(t *testing.T) {
	forceASCII(t)
	base := fixtureBase
	tr := journey.Trail{Legs: []journey.Leg{
		{Class: journey.Test, Label: "pytest", Start: base, End: base.Add(2 * time.Minute),
			Waypoints: []journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "40 passed", Short: "40✓", At: base.Add(2 * time.Minute)}}},
		{Class: journey.Ship, Label: "git commit", Start: base.Add(3 * time.Minute), End: base.Add(4 * time.Minute),
			Waypoints: []journey.Waypoint{{Kind: journey.WaypointCommit, Text: "auth: audit every refresh", At: base.Add(4 * time.Minute)}}},
		{Class: journey.Scout, Label: "x", Start: base.Add(5 * time.Minute), Current: true},
	}}
	got := RenderTrail(tr, TrailOpts{Now: base.Add(10 * time.Minute), Width: 44, Height: 30, Level: 2, Cursor: -1})
	if strings.Contains(got, "└ 40 passed") || strings.Contains(got, "└ auth: audit every refresh") {
		t.Errorf("a child restates its parent:\n%s", got)
	}
	if n := len(TrailRows(tr, levelWaypoints)); n != 3 {
		t.Errorf("TrailRows counts %d rows, want the three legs only", n)
	}
	// An older run in the same leg is still a row: the badge is only the newest.
	tr.Legs[0].Waypoints = append([]journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "38 passed · 2 failed", Short: "38✓ 2✗", At: base.Add(time.Minute)}}, tr.Legs[0].Waypoints...)
	if got := RenderTrail(tr, TrailOpts{Now: base.Add(10 * time.Minute), Width: 44, Height: 30, Level: 2, Cursor: -1}); !strings.Contains(got, "38 passed · 2 failed") {
		t.Errorf("an earlier run in the leg lost its row:\n%s", got)
	}
}

// A wide Lv1 panel spends its width inside the row: the failing test, the
// bug, the files beyond the label, beside the label.
func TestWideTrailCarriesDetailOnTheRow(t *testing.T) {
	forceASCII(t)
	tr := fixtureLv2Trail(fixtureBase)
	wide := renderLv(tr, fixtureBase.Add(40*time.Minute), 1, 100, 30)
	for _, want := range []string{"pytest · ✗ test_refresh_expired_token · ✗ test_refresh_revoked_token", "token refresh · syntax error in tokens.py:88"} {
		if !strings.Contains(wide, want) {
			t.Errorf("a wide row lacks %q:\n%s", want, wide)
		}
	}
	narrow := renderLv(tr, fixtureBase.Add(40*time.Minute), 1, 60, 30)
	if strings.Contains(narrow, "· ✗ test_refresh") {
		t.Errorf("a narrow row carries inline detail:\n%s", narrow)
	}
	lv2 := RenderTrail(tr, TrailOpts{Now: fixtureBase.Add(40 * time.Minute), Width: 100, Height: 30, Level: 2, Cursor: -1})
	if strings.Contains(lv2, "pytest · ✗") {
		t.Errorf("Lv2 carries the detail inline and beneath:\n%s", lv2)
	}
}

// A question too long for HEAD's row is spelled out beneath it, options and
// all, at every level.
func TestLongQuestionIsSpelledOutUnderHead(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	infra := sessionKey("s-infra")
	q := "Open port 22 to the office CIDR only, or keep the bastion? [office CIDR / keep bastion]"
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == infra {
			m.sessions[i].Snap.Activity = q
		}
	}
	col := strings.Join(m.boardColumn(infra, rowFor(t, m, infra), 40, 20), "\n")
	if !strings.Contains(col, "▲ design asks you") || !strings.Contains(col, "waiting 2m") {
		t.Errorf("HEAD does not wear the fleet's glyph and name the question:\n%s", col)
	}
	if !strings.Contains(col, "keep bastion]") {
		t.Errorf("the question's options never reach the column:\n%s", col)
	}
	// One that fits rides on the row alone.
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == infra {
			m.sessions[i].Snap.Activity = "Port 22 open?"
		}
	}
	col = strings.Join(m.boardColumn(infra, rowFor(t, m, infra), 40, 20), "\n")
	if !strings.Contains(col, "▲ design Port 22 open?") || strings.Contains(col, "asks you") {
		t.Errorf("a short question is not on HEAD's row:\n%s", col)
	}
}
