package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A one-CLI fleet's legend is budgeted without the tool gloss: at 152 it
// spends the rows the gloss would have taken on the definitions it draws,
// so no definition is shed over a blank row of its own column (#92, #87).
func TestTheHelpsLegendSpendsTheRowsTheGlossLeft(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 152, 40)
	press(m, "?")
	view := ansi.Strip(m.View())
	if strings.Contains(view, "which CLI runs the session") {
		t.Fatalf("a one-CLI fleet's help defines the tool word:\n%s", view)
	}
	if !strings.Contains(view, "(3h+ = away)") {
		t.Errorf("the legend sheds a definition while its own column ends blank:\n%s", view)
	}
}

// At eighty the band's gate measures the rows the band is budgeted with:
// after a hide the fleet column draws the band under the archive's line
// instead of two blank rows (#92, #47).
func TestTheBandTakesTheRowsItHasTheBudgetForAtEighty(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 80, 24)
	press(m, "x")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "1 hidden") {
		t.Fatalf("the hide left no trace:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	head := -1
	for i, l := range lines {
		if strings.Contains(l, "archived · 1 hidden") {
			head = i
		}
	}
	if head < 0 {
		t.Fatalf("no band header:\n%s", view)
	}
	if !strings.Contains(lines[head], "hidden · A") {
		t.Errorf("the band's header lost its key to the clip: %q", lines[head])
	}
	if head+1 >= len(lines) || !strings.Contains(lines[head+1], " ○ ") {
		t.Errorf("the archive's line stands over blank rows the band had the budget for:\n%s", view)
	}
}

// A fleet of one with no trace to draw: the live entry never spends a
// row on the default tool's bare word, the one word on a frame whose
// OpenCode band rows draw none (#93, #80, #90).
func TestTheLiveEntryNeverSpendsARowOnTheBareToolWord(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{100, 30}, {80, 24}} {
		m := sceneModel(sceneSecondDay(), size[0], size[1])
		view := ansi.Strip(m.View())
		for _, l := range strings.Split(view, "\n") {
			fleet := strings.SplitN(l, "│", 2)[0]
			if strings.TrimSpace(fleet) == "claude" {
				t.Errorf("at %dx%d a whole fleet row is the word claude: %q\n%s", size[0], size[1], l, view)
				break
			}
		}
	}
}

// The band takes the rows the hide freed on the route the walkthrough
// walks: a replied-to fleet at eighty leaves one free row and two in the
// tail, and the gate is measured with the rows the band is budgeted with —
// a gate on the reduced body refused them, and the one-`x` pin above walks
// a route the header shed alone covers (#92, #47).
func TestTheBandTakesItsRowsOnTheHidesOwnRoute(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 80, 24)
	for _, k := range []string{"r", "1", "j", "x"} { // reply, then hide notebooks
		pressKey(m, k)
	}
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	head := -1
	for i, l := range lines {
		if strings.Contains(l, "archived · 1 hidden") {
			head = i
		}
	}
	if head < 0 {
		t.Fatalf("no archive line after the hide:\n%s", strings.Join(lines, "\n"))
	}
	if head+1 >= len(lines) || !strings.Contains(lines[head+1], " ○ ") {
		t.Errorf("the archive's line stands over blank rows the band had the budget for:\n%s", strings.Join(lines, "\n"))
	}
}

// A fleet of one has no board, and its legend is budgeted without the
// board's line: at 152 the help draws `⚠ two sessions, one thing` whole
// instead of shedding it over a blank row of its own column (#95, #92).
func TestTheHelpsLegendSpendsTheRowsTheBoardLineLeft(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 152, 40)
	press(m, "?")
	view := ansi.Strip(m.View())
	if strings.Contains(view, "board:") {
		t.Fatalf("a fleet of one's help describes a board it has not:\n%s", view)
	}
	if !strings.Contains(view, "⚠ two sessions, one thing") {
		t.Errorf("the legend sheds a definition while its own column ends blank:\n%s", view)
	}
}
