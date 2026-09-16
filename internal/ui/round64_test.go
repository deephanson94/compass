package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Round sixty-four: the summary stands (#374). The legs counted by class
// are drawn above every trail — the trail column at every level and every
// column of the board — and the trail takes the rows that are left. No
// key opens or closes it; a trail with nothing to count draws none.

// trailCells is the trail column's cells of every frame row, cut by the
// deck's own layout.
func trailCells(m *Model) []string {
	inner := m.width - 2*edgePad
	fw, mw, tw := m.layout(inner)
	start := edgePad
	if fw > 0 {
		start += fw + gutterWidth
		if mw > 0 && !(m.level >= levelWaypoints && !(m.sessionView() && m.showMirror)) {
			start += mw + gutterWidth
		}
	}
	var out []string
	for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
		r := []rune(row)
		if start >= len(r) {
			out = append(out, "")
			continue
		}
		out = append(out, string(r[start:min(start+tw, len(r))]))
	}
	return out
}

// longSession opens very-long's day-long session at w×h, on its legs.
func longSession(t *testing.T, w, h int) (*Model, scene) {
	t.Helper()
	sc := sceneVeryLong()
	m := sceneModel(sc, w, h)
	pressKey(m, "1")
	poll(m, sc)
	pressKey(m, "tab")
	poll(m, sc)
	if m.level != levelWaypoints || len(m.trail.Legs) < 100 {
		t.Fatalf("%dx%d: tab from the board should land on a day-long session's legs: level %d, %d legs", w, h, m.level, len(m.trail.Legs))
	}
	return m, sc
}

func TestTheBlockStandsAboveTheTrailAtEveryWidth(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		m, _ := longSession(t, size[0], size[1])
		cells := trailCells(m)
		col := strings.Join(cells, "\n")
		counts := classCounts(m.trail)
		// Every class the trail has, with its count, in the trail's order
		// of work, and the wait — before the first trail row.
		first := -1
		last := -1
		for i, c := range cells {
			if strings.Contains(c, "◆ scout  32 legs") && first < 0 {
				first = i
			}
			if strings.Contains(c, "◉ waited on you") {
				last = i
			}
		}
		if first < 0 || last < first {
			t.Fatalf("%dx%d: no block over the trail:\n%s", size[0], size[1], col)
		}
		order := []string{}
		for _, c := range summaryOrder {
			if counts[c] >= 2 {
				order = append(order, c.String())
			}
		}
		at := first
		for _, name := range order {
			if !strings.Contains(cells[at], name) || !strings.Contains(cells[at], " legs") {
				t.Errorf("%dx%d: block row %d should be the %s class row: %q", size[0], size[1], at, name, cells[at])
			}
			at++
		}
		if at != last {
			t.Errorf("%dx%d: the wait row should follow the class rows at %d, is at %d", size[0], size[1], at, last)
		}
		// The trail follows, with its own rows, and HEAD's row at the
		// present: the class row yields its clause to it.
		trail := strings.Join(cells[last+1:], "\n")
		if !strings.Contains(trail, "▸") {
			t.Errorf("%dx%d: the trail beneath the block has no cursor row:\n%s", size[0], size[1], trail)
		}
		if strings.Contains(col, "16 legs · for 39m") != !strings.Contains(trail, "for 39m") {
			t.Errorf("%dx%d: the present should be said once, on the class row or HEAD's own row:\n%s", size[0], size[1], col)
		}
		// The title gives the ships, the reds and the wait to the rows.
		title := cells[3]
		for _, twice := range []string{"16⚑", "10✗", "on you", " ships", " red"} {
			if strings.Contains(title, twice) {
				t.Errorf("%dx%d: the title repeats the block's %q: %q", size[0], size[1], twice, title)
			}
		}
	}
}

func TestTheTrailBeneathTheBlockStillScrollsAndWalks(t *testing.T) {
	forceASCII(t)
	m, _ := longSession(t, 80, 24)
	before := trailCells(m)
	press(m, "ctrl+u")
	after := trailCells(m)
	if before[5] != after[5] || !strings.Contains(strings.Join(after, "\n"), "↓ G") {
		t.Errorf("ctrl+u should scroll the trail under a standing block: block row %q → %q", before[5], after[5])
	}
	// Scrolled off HEAD's row, the class row says the clause.
	if !strings.Contains(strings.Join(after, "\n"), "for 39m") {
		t.Errorf("scrolled off the present, no row says how long HEAD has run:\n%s", strings.Join(after, "\n"))
	}
	press(m, "G")
	if strings.Contains(strings.Join(trailCells(m), "\n"), "↓ G") {
		t.Errorf("G should be back at the present")
	}
	press(m, "j")
	press(m, "k")
	w, h := m.trailBox()
	if h+blockHeight(m.trail)+trailChrome != 24-5 {
		t.Errorf("the trail's viewport should be the column less the chrome and the block: %d rows at width %d", h, w)
	}
}

func TestATrailOfOneOfEachDrawsNoBlock(t *testing.T) {
	forceASCII(t)
	sc := sceneFleetHygiene()
	m := sceneModel(sc, 120, 34)
	pressKey(m, "4")
	poll(m, sc)
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	if !summaryCountsNothing(m.trail) {
		t.Skip("fleet-hygiene's fourth session counts")
	}
	if blockHeight(m.trail) != 0 || strings.Contains(ansi.Strip(m.View()), " legs ") {
		t.Errorf("a trail of one of each should draw no block:\n%s", ansi.Strip(m.View()))
	}
	fresh := sceneModel(sceneFirstSession(), 80, 24)
	if blockHeight(fresh.trail) != 0 {
		t.Errorf("a trail with no leg should draw no block")
	}
}

func TestEveryBoardColumnWearsItsBlock(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		sc := sceneVeryLong()
		m := sceneModel(sc, size[0], size[1])
		if m.level != levelBoard || !m.boardShown() {
			t.Fatalf("%dx%d: the scene should open on the board", size[0], size[1])
		}
		v := ansi.Strip(m.View())
		for _, want := range []string{"◆ scout  32 legs", "◆ test   32 legs · 10 red", "◆ scout  24 legs", "◆ test   24 legs · 8 red", "◉ waited on you · 12 prompts", "◉ waited on you · 9 prompts"} {
			if !strings.Contains(v, want) {
				t.Errorf("%dx%d: the board lacks %q:\n%s", size[0], size[1], want, v)
			}
		}
		// The fold row gives the ships, the reds and the wait to the block.
		for _, line := range strings.Split(v, "\n") {
			if strings.Contains(line, "↑ ") && strings.Contains(line, " legs · ") && (strings.Contains(line, "⚑") || strings.Contains(line, "✗") || strings.Contains(line, "on you")) {
				t.Errorf("%dx%d: a column's fold row repeats the block's clauses: %q", size[0], size[1], line)
			}
		}
		// The short column keeps its trail alone.
		if strings.Contains(v, "2 legs") && !strings.Contains(v, "◆ build  the flag") {
			t.Errorf("%dx%d: the column of one of each should keep its trail:\n%s", size[0], size[1], v)
		}
	}
}

func TestNoKeyOpensOrClosesTheSummary(t *testing.T) {
	forceASCII(t)
	m, _ := longSession(t, 152, 40)
	before := ansi.Strip(m.View())
	press(m, "s")
	after := ansi.Strip(m.View())
	if before != after {
		t.Errorf("s should do nothing now (#374)")
	}
	press(m, "?")
	h := ansi.Strip(m.View())
	if strings.Contains(h, "summary:") || strings.Contains(h, " s  ") {
		t.Errorf("the help should have no row for a key that went:\n%s", h)
	}
	foot := strings.Split(before, "\n")
	if strings.Contains(foot[len(foot)-1], "s summary") || strings.Contains(foot[len(foot)-1], "s/esc") {
		t.Errorf("the footer names a key that went: %q", foot[len(foot)-1])
	}
}

func TestTheParkedLeadsBlockOnTheBoardAndItsLegs(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	m := sceneModel(sc, 120, 34)
	for _, d := range []string{"1", "2", "3", "4", "5"} {
		pressKey(m, d)
		poll(m, sc)
		if s, ok := m.selected(); ok && sessionName(s.Info) == "porter" && len(m.trails[m.selectedKey].Branches) == 4 {
			break
		}
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "◈ agent  4 lanes") {
		t.Fatalf("porter's column should carry its lanes row:\n%s", v)
	}
	if n := strings.Count(v, "· 1 back"); n != 1 {
		t.Errorf("the lanes' tally should be said once on the board, %d times:\n%s", n, v)
	}
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	v = ansi.Strip(m.View())
	if !strings.Contains(v, "◈ agent  4 lanes") || strings.Count(v, "◈3 out 20m") < 1 {
		t.Errorf("porter's legs should carry the lanes row over the trail, HEAD's tail on its own row:\n%s", v)
	}
	if strings.Count(v, "for 2h") > 1 {
		t.Errorf("the parked lead's span should be said once:\n%s", v)
	}
}
