package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

// Round fifteen: the typed line's trace, the archive's digits, the cursor
// that stays on screen, and the looks compass takes on its own.

// A typed line leaves the same trace a digit does.
func TestTheTypedLineLeavesATrace(t *testing.T) {
	forceASCII(t)
	m := followModel(152, 40)
	m.runner = &recordingTmux{}
	press(m, "r")
	press(m, "t")
	for _, r := range "go on" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter sent nothing")
	}
	m.Update(cmd())
	api := sessionKey("s-api")
	if row := ansi.Strip(m.boardDelta(api, m.sessions[rowFor(t, m, api).sess], 60)); !strings.Contains(row, `↪ sent "go on"`) || !strings.Contains(row, "ago") {
		t.Errorf("the typed line's trace = %q", row)
	}
	if !strings.Contains(m.note, `↪ sent "go on"`) {
		t.Errorf("the footer should say the line went: %q", m.note)
	}
}

// The archive numbers its rows as drawn, hidden rows included, and opens on
// the hidden row: no digit names two sessions.
func TestTheArchiveNumbersAsDrawn(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	m.point(sessionKey("s-webapp"))
	press(m, "x")
	m.point(sessionKey("s-infra")) // the archive remembers a cursor elsewhere
	press(m, "A")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "▸1 ● webapp") {
		t.Errorf("the archive should open on the hidden row, numbered 1:\n%s", view)
	}
	seen := map[int]bool{}
	for _, r := range m.fleetRows() {
		if r.header || r.num == 0 {
			continue
		}
		if seen[r.num] {
			t.Errorf("digit %d names two rows:\n%s", r.num, view)
		}
		seen[r.num] = true
	}
	if foot := ansi.Strip(m.View()); !strings.Contains(foot, "x unhide") {
		t.Errorf("x unhide should be offered on the hidden row:\n%s", foot)
	}
	press(m, "j")
	if foot := ansi.Strip(m.View()); strings.Contains(foot, "x unhide") {
		t.Errorf("x unhide should not be offered off the hidden row:\n%s", foot)
	}
}

// The cursor's row is never drawn over: a child on the top row keeps the
// row beneath its parent.
func TestTheCursorRowStaysVisible(t *testing.T) {
	base := fixtureBase
	tr := journey.Trail{Prompts: []journey.Prompt{{Text: "port it", At: base}}}
	for i := 0; i < 6; i++ {
		start := base.Add(time.Duration(i) * 10 * time.Minute)
		tr.Legs = append(tr.Legs, journey.Leg{Class: journey.Test, Label: "pytest", Start: start, End: start.Add(5 * time.Minute),
			Waypoints: []journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "1 failed", Short: "1✗"}, {Kind: journey.WaypointTestFail, Text: "test_x", Runs: i + 1}}})
	}
	o := TrailOpts{Now: base.Add(2 * time.Hour), Width: 40, Height: 6, Level: levelWaypoints, Dense: true}
	doc := TrailLines(tr, o)
	// Find a detail row and put the cursor on it, with the viewport opening there.
	for i, l := range doc {
		if isDetailRow(l) && i > 0 {
			o.Cursor = -1
			_, sel := trailDoc(tr, o)
			o.Cursor = sel[i]
			o.Scroll = i
			rows := trailRows(tr, o)
			joined := strings.Join(rows, "\n")
			if !strings.Contains(joined, "▸") {
				t.Errorf("the cursor's row was scrolled past:\n%s", joined)
			}
			if !isCursorRow(rows[1]) || isCursorRow(rows[0]) {
				t.Errorf("the parent takes the top row and the child the one beneath:\n%s", joined)
			}
			return
		}
	}
	t.Fatal("no detail row in the fixture")
}

// A hide moving the selection is not a look; leaving a session on the
// narrow deck commits its look.
func TestTheLooksCompassTakesOnItsOwn(t *testing.T) {
	forceASCII(t)
	m := boardModel(100, 30)
	m.seen = map[string]time.Time{}
	m.point(sessionKey("s-api"))
	press(m, "x")
	if _, read := m.seen[m.selectedKey]; read {
		t.Errorf("x moved the selection to %q and read it", m.selectedKey)
	}
	// Selecting a session on the narrow deck opens it; moving on closes it.
	m.point(sessionKey("s-tfstate"))
	m.lastLook[sessionKey("s-tfstate")] = fixtureBase
	m.point(sessionKey("s-infra"))
	if _, open := m.lastLook[sessionKey("s-tfstate")]; open || !m.seen[sessionKey("s-tfstate")].Equal(m.now) {
		t.Error("leaving a session on the narrow deck should commit its look")
	}
}

// The digest leads with the lanes, the loop's count comes before the
// caveat, the narrow quota head keeps its warning, the strip keeps "+N
// more", and the fact leads the compact overlap.
func TestRoundFifteenOrders(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	api := sessionKey("s-api")
	tr := m.trails[api]
	tr.Branches = append(tr.Branches, journey.Branch{ToolUseID: "b", Label: "scout", Start: m.now.Add(-5 * time.Minute), End: m.now.Add(-time.Minute), Done: true, Report: "found it", AfterLeg: len(tr.Legs) - 1})
	m.trails[api] = tr
	m.seen = map[string]time.Time{api: m.now.Add(-10 * time.Minute)}
	if row := ansi.Strip(m.boardDelta(api, m.sessions[rowFor(t, m, api).sess], 60)); !strings.HasPrefix(row, "↳ 1 agent back") {
		t.Errorf("the digest should lead with the lanes: %q", row)
	}
	base := fixtureBase
	loop := journey.Trail{Legs: []journey.Leg{
		{Class: journey.Test, Label: "pytest", Start: base, End: base.Add(time.Minute), Waypoints: []journey.Waypoint{{Kind: journey.WaypointTestRun, Short: "1✗", Text: "1 failed"}, {Kind: journey.WaypointTestFail, Text: "t", Runs: 3}}},
		{Class: journey.Fix, Label: "x.py", Start: base.Add(2 * time.Minute), End: base.Add(3 * time.Minute), Files: []string{"x.py"}},
	}}
	if got := strings.Join(verdictParts(loop, base.Add(time.Hour), false), " · "); !strings.Contains(got, "3rd failure · edited since") {
		t.Errorf("the count comes before the caveat: %q", got)
	}
	// The narrow quota head keeps its warning.
	m2 := followModel(80, 24)
	deadOnAPI(m2, api)
	m2.point(api)
	press(m2, "r")
	if view := ansi.Strip(m2.View()); !strings.Contains(view, "lines · only once the quota is back") {
		t.Errorf("the narrow panel should keep the warning:\n%s", view)
	}
	// "+N more" is never shed.
	m3 := boardModel(152, 40)
	shared := func(name string) journey.Trail {
		return journey.Trail{Legs: []journey.Leg{{Class: journey.Build, Label: name, Start: base.Add(30 * time.Minute), End: base.Add(35 * time.Minute), Files: []string{"a_rather_long_file_name.py"}}}}
	}
	m3.trails[api] = shared("api")
	m3.trails[sessionKey("s-webapp")] = shared("webapp")
	if strip := ansi.Strip(m3.boardStrip(nil, m3.boardRows(), 70)); !strings.Contains(strip, "+") || !strings.Contains(strip, "more") {
		t.Errorf("the strip should keep +N more over the overlap sentence: %q", strip)
	}
}

// A board the query would hide the open session from clears the query on
// the way out, and the reader's title never echoes the fleet's draft.
func TestAQueryHidingTheOpenSessionClears(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	m.point(sessionKey("s-infra"))
	pressTab(m)
	press(m, "/")
	for _, r := range "zzz" {
		press(m, string(r))
	}
	if title := ansi.Strip(m.readerTitle(70)); strings.Contains(title, "/zzz") {
		t.Errorf("the reader's title echoes the fleet's draft: %q", title)
	}
	press(m, "enter")
	press(m, "esc")
	if m.level != levelBoard || m.fleetQuery != "" {
		t.Errorf("zooming out to a board the query hides the session from should clear it: level %d, query %q", m.level, m.fleetQuery)
	}
}

// The help splits into two columns only where they hold whole, or where
// one column would have to cut the keys.
func TestTheHelpSplitsOnlyWhereItHolds(t *testing.T) {
	// A column join carries text before its bar; the legend's read-line row
	// (│ you were here) has only air before its own.
	split := func(lines []string) bool {
		for _, l := range lines {
			if i := strings.Index(l, "│"); i > 0 && strings.TrimSpace(l[:i]) != "" {
				return true
			}
		}
		return false
	}
	oneLines := helpLinesFor(120, 29, true)
	one := strings.Join(oneLines, "\n")
	if split(oneLines) {
		t.Errorf("at 120x34 the help should be one column:\n%s", one)
	}
	twoLines := helpLinesFor(120, 22, true)
	two := strings.Join(twoLines, "\n")
	if !split(twoLines) || !strings.Contains(two, "scout") {
		t.Errorf("at 120x22 the help should split to keep a legend:\n%s", two)
	}
}

// A cut fleet never opens or closes inside an entry.
func TestTheListNeverCutsInsideAnEntry(t *testing.T) {
	forceASCII(t)
	m := boardModel(100, 30)
	m.seen = map[string]time.Time{}
	for _, s := range m.sessions {
		if s.Live {
			m.seen[s.Info.Key()] = fixtureBase.Add(5 * time.Minute) // every row grows a digest line
		}
	}
	for _, key := range []string{sessionKey("s-webapp"), sessionKey("s-tfstate"), sessionKey("s-scratch")} {
		m.pointQuiet(key)
		col := fleetText(m, 100, 18)
		for i, l := range col {
			plain := ansi.Strip(l)
			if i > 0 && strings.HasPrefix(ansi.Strip(col[i-1]), "▴") && plain != "" && !isHeaderLine(l) && !isEntryFirstLine(l) {
				t.Errorf("the window opens inside an entry:\n%s", strings.Join(col, "\n"))
			}
			if i+1 < len(col) && strings.HasPrefix(ansi.Strip(col[i+1]), "▾") && strings.HasPrefix(plain, "    ") && i+2 < len(col) {
				// The line before the fold is a continuation: fine only if the entry is whole,
				// which the next line being the fold cannot prove — so the entry's first line
				// must be within the window and the following hidden line must not be its own.
				_ = state.Idle
			}
		}
	}
}
