package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Round sixty-four: the summary stands (#377). The legs counted by class
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
	// #377 took the summary view and its key; #393 gave `s` back as a
	// jump between the counts and the trail — never a view: the trail
	// stands under the block on both sides of the press.
	forceASCII(t)
	m, _ := longSession(t, 152, 40)
	before := ansi.Strip(m.View())
	press(m, "s")
	after := ansi.Strip(m.View())
	if !strings.Contains(after, "the trail ─") || strings.Contains(after, "[summary]") {
		t.Errorf("s should move the cursor, not swap the view (#377, #393):\n%s", after)
	}
	press(m, "?")
	h := ansi.Strip(m.View())
	if strings.Contains(h, "summary:") {
		t.Errorf("the help should have no row for a view that went:\n%s", h)
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

// The panel's first pass on #377, second-day, folded (#378).

func TestTheBlockEndsOnItsSeam(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		m, _ := longSession(t, size[0], size[1])
		cells := trailCells(m)
		seam := -1
		for i, c := range cells {
			if strings.HasPrefix(strings.TrimSpace(c), "│ the trail ─") {
				seam = i
			}
		}
		if seam < 0 {
			t.Fatalf("%dx%d: no seam under the block:\n%s", size[0], size[1], strings.Join(cells, "\n"))
		}
		if !strings.Contains(cells[seam-1], "◉ waited on you") {
			t.Errorf("%dx%d: the seam should follow the block's last row: %q", size[0], size[1], cells[seam-1])
		}
		if strings.TrimSpace(cells[seam+1]) == "" || strings.Contains(cells[seam+1], " legs") {
			t.Errorf("%dx%d: the trail should begin right under the seam: %q", size[0], size[1], cells[seam+1])
		}
		if n := strings.Count(strings.Join(cells, "\n"), "│ the trail ─"); n != 1 {
			t.Errorf("%dx%d: the seam is drawn %d times", size[0], size[1], n)
		}
	}
	// The board's columns end their blocks on the seam too; a column with
	// no block draws none.
	sc := sceneVeryLong()
	m := sceneModel(sc, 152, 40)
	if n := strings.Count(ansi.Strip(m.View()), "│ the trail ─"); n != 2 {
		t.Errorf("the board should draw one seam per counted column, %d drawn", n)
	}
	fresh := sceneModel(sceneFirstSession(), 80, 24)
	if strings.Contains(ansi.Strip(fresh.View()), "the trail ─") {
		t.Errorf("a trail with no block should draw no seam")
	}
}

// The panel's first pass on #377, subagents, folded (#379).

func TestTheLanesRowShedsTheClockTheFrameSays(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sc, size[0], size[1])
		for _, d := range []string{"1", "2", "3", "4", "5"} {
			pressKey(m, d)
			poll(m, sc)
			if s, ok := m.selected(); ok && sessionName(s.Info) == "porter" && len(m.trails[m.selectedKey].Branches) == 4 {
				break
			}
		}
		for _, level := range []string{"board or list", "legs"} {
			v := ansi.Strip(m.View())
			lanes := ""
			for _, line := range strings.Split(v, "\n") {
				if strings.Contains(line, "◈ agent  4 lanes") {
					lanes = line
				}
			}
			if lanes == "" {
				t.Fatalf("%dx%d %s: porter's lanes row is not on the frame:\n%s", size[0], size[1], level, v)
			}
			// The oldest lane's clock is on the frame once: on the fleet
			// row beside, the card, the column header or HEAD's own row —
			// and then not on the lanes row (#379).
			said := strings.Contains(v, "◈3 out 20m") || strings.Contains(v, "20m out")
			onRow := strings.Contains(lanes, "20m out")
			if !said {
				t.Errorf("%dx%d %s: no row carries the oldest lane's clock:\n%s", size[0], size[1], level, v)
			}
			if onRow && strings.Count(v, "20m out")+strings.Count(v, "◈3 out 20m") > 1 {
				t.Errorf("%dx%d %s: the lanes row repeats the clock the frame carries: %q\n%s", size[0], size[1], level, lanes, v)
			}
			if level == "board or list" {
				for m.level < levelWaypoints {
					pressKey(m, "tab")
					poll(m, sc)
				}
			}
		}
	}
	// The clock's other idiom: harness:0.0's lanes are all back, so its
	// row's clock is `1h ago`, and the lane that came back last wears the
	// same clock on its own row beneath (#379, #383).
	m := sceneModel(sc, 120, 34)
	v := ansi.Strip(m.View())
	lanes := ""
	for _, line := range strings.Split(v, "\n") {
		for _, cell := range strings.Split(line, "│") {
			if strings.Contains(cell, "◈ agent  3 lanes") {
				lanes = cell
			}
		}
	}
	if lanes == "" {
		t.Fatalf("harness:0.0's lanes row is not on the board at 120x34:\n%s", v)
	}
	if !strings.Contains(v, "✓ 1h ago") {
		t.Fatalf("no lane row carries the clock the lanes row would repeat:\n%s", v)
	}
	if strings.Contains(lanes, "1h ago") {
		t.Errorf("the lanes row repeats the clock the lanes beneath carry: %q\n%s", lanes, v)
	}
}

// The panel's second pass on #380 — two-tools, subagents, fleet-hygiene —
// folded (#382, #383, #384).

// The reply box's floor and the band below: where the box would paint over a
// band's head rows, the band stands off the box's floor, or, where it no
// longer fits, it is the strip's, and the strip stands off the floor too —
// so the session is named on the frame either way (#382).
func TestABandTheBoxWouldBeheadStandsOffItsFloor(t *testing.T) {
	forceASCII(t)
	sc := sceneTwoTools()
	// At 28 rows the six rows the band needs are the strip's and its air
	// plus four: nothing is hidden, so there is no strip to keep them for
	// and the band stands whole; at 26 four rows are left under the
	// floor, under a band's floor, and the strip names docs (#386).
	for _, c := range []struct {
		h    int
		want string
	}{{34, " 4 ○ docs "}, {28, " 4 ○ docs "}, {26, "+1 more · 4 ○ docs"}} {
		m := sceneModel(sc, 120, c.h)
		m.View()
		pressKey(m, "r")
		poll(m, sc)
		v := ansi.Strip(m.View())
		if !m.replyBox.on {
			t.Fatalf("120x%d: r opens no box on infra:\n%s", c.h, v)
		}
		lines := strings.Split(v, "\n")
		bottom, docs := -1, -1
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "└") {
				bottom = i
			}
			if strings.Contains(line, c.want) {
				docs = i
			}
		}
		if bottom < 0 {
			t.Fatalf("120x%d: no box bottom on the frame:\n%s", c.h, v)
		}
		if docs < 0 {
			t.Errorf("120x%d: the docs session is named nowhere under the box (#382):\n%s", c.h, v)
		} else if docs != bottom+2 {
			t.Errorf("120x%d: the docs row should stand one row of air off the box's floor (row %d), stands at %d:\n%s", c.h, bottom+2, docs, v)
		}
		if strings.Contains(v, "opencode · gpt-5") && docs < 0 {
			t.Errorf("120x%d: the docs trail stands with no row naming it:\n%s", c.h, v)
		}
		if c.h == 28 && !strings.Contains(v, "install.md") {
			t.Errorf("120x28: the band should stand whole in the rows the strip was keeping (#386):\n%s", v)
		}
		pressKey(m, "esc")
	}
}

// A card row nothing is left of goes: the rung the header says was dropped
// (#380) and no clause stood beside it, so the card is head and delta, as
// it was before the rung was dropped (#382).
func TestACardRowNothingIsLeftOfGoes(t *testing.T) {
	forceASCII(t)
	sc := sceneTwoTools()
	for _, w := range []int{120, 152, 220} {
		m := sceneModel(sc, w, 34)
		for _, d := range []string{"1", "2", "3", "4"} {
			pressKey(m, d)
			poll(m, sc)
			if s, ok := m.selected(); ok && sessionName(s.Info) == "api" && strings.Contains(m.toolTag(s), "opencode") {
				break
			}
		}
		for m.level < levelWaypoints {
			pressKey(m, "tab")
			poll(m, sc)
		}
		cells := trailCells(m)
		for i, c := range cells {
			if !strings.Contains(c, "[session]") {
				continue
			}
			// The row after the head is never blank: the delta where
			// there is one, the trail's first row where there is not —
			// false the moment the emptied rung row returns (#382,
			// two-tools 3 of the third pass).
			if i+1 >= len(cells) || strings.TrimSpace(cells[i+1]) == "" {
				t.Errorf("%dx34: the card keeps a blank row under its head (#382):\n%s", w, strings.Join(cells[i:min(i+3, len(cells))], "\n"))
			}
		}
	}
}

// The board spends its unspent rows on the bands' debt: a tight pack decides
// the columns, and the rows it did not spend go back to the bands, smallest
// debt first, so a column's ask is not folded over blank rows (#384).
func TestTheBoardSpendsItsSpareRowsOnTheBandsDebt(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneManyIdle(), 220, 48)
	v := ansi.Strip(m.View())
	if got := boardColumnsDrawn(v); got < 9 {
		t.Fatalf("many-idle at 220x48 should draw its columns as before, drew %d:\n%s", got, v)
	}
	for _, ask := range []string{`◉ "clean the exploration notebooks"`, `◉ "profile the hot loop"`} {
		if !strings.Contains(v, ask) {
			t.Errorf("the rows are there and the ask is folded anyway (#384): %q missing\n%s", ask, v)
		}
	}
	lines := strings.Split(v, "\n")
	blank := 0
	for i := len(lines) - 3; i >= 0 && strings.TrimSpace(lines[i]) == ""; i-- {
		blank++
	}
	if blank > 1 {
		t.Errorf("%d blank rows stand under the strip while a column folds (#384):\n%s", blank, v)
	}
}

// Where the fold takes only the ask, the rail stub under it is the fold's
// row — `↑ began …` — and never a bare stroke under the seam (#384).
func TestAFoldThatTakesOnlyTheAskSaysSo(t *testing.T) {
	forceASCII(t)
	for _, sc := range []scene{sceneManyIdle(), sceneAlarmStorm()} {
		for h := 40; h <= 48; h++ {
			m := sceneModel(sc, 220, h)
			lines := strings.Split(ansi.Strip(m.View()), "\n")
			for i := 1; i < len(lines); i++ {
				above, here := []rune(lines[i-1]), []rune(lines[i])
				for j, r := range here {
					// A stub in a column's first cell, under the cell the
					// seam begins in.
					if r != '╷' || j >= len(above) || !strings.HasPrefix(string(above[j:]), "│ the trail ─") {
						continue
					}
					t.Errorf("%s at 220x%d: a bare rail stub under the seam, its ask folded silently (#384):\n%s", sc.name, h, strings.Join(lines[i-1:i+1], "\n"))
				}
			}
		}
	}
}

// The panel's first pass on #377 — alarm-storm, fleet-hygiene, two-tools
// and second-day's second — folded (#380).

// boardColumnsDrawn counts the columns a board frame draws, by the digit
// and glyph that head each.
func boardColumnsDrawn(v string) int {
	n := 0
	for _, line := range strings.Split(v, "\n") {
		lead := strings.TrimLeft(line, " ")
		if strings.Contains(line, "│") || strings.HasPrefix(lead, "▸") || len(lead) > 2 && lead[0] >= '1' && lead[0] <= '9' && lead[1] == ' ' {
			// a header row: every column's first row carries a digit
			for _, cell := range strings.Split(line, "│") {
				t := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(cell), "▸"))
				if len(t) > 1 && t[0] >= '1' && t[0] <= '9' && t[1] == ' ' {
					n++
				}
			}
		}
	}
	return n
}

func TestTheBlockNeverCostsASessionItsColumn(t *testing.T) {
	forceASCII(t)
	// The columns of before the block, at the widths the block cost them:
	// counted from the pre-block corpus (#380, #383).
	for _, c := range []struct {
		sc   scene
		w, h int
		want int
	}{
		{sceneAlarmStorm(), 120, 34, 6}, {sceneManyIdle(), 120, 34, 6}, {sceneSubagents(), 120, 34, 4},
		{sceneAlarmStorm(), 220, 48, 7}, {sceneManyIdle(), 220, 48, 9}, {sceneSubagents(), 220, 48, 4},
	} {
		m := sceneModel(c.sc, c.w, c.h)
		v := ansi.Strip(m.View())
		if got := boardColumnsDrawn(v); got < c.want {
			t.Errorf("%s at %dx%d should draw its %d columns as it did before the block, drew %d:\n%s", c.sc.name, c.w, c.h, c.want, got, v)
		}
		if !strings.Contains(v, " legs") || !strings.Contains(v, "│ the trail ─") {
			t.Errorf("%s at %dx%d should keep its blocks while it keeps its columns:\n%s", c.sc.name, c.w, c.h, v)
		}
	}
	// Where the board has the rows, the block stands over the whole trail.
	m := sceneModel(sceneVeryLong(), 220, 48)
	if m.blockInTrail {
		t.Errorf("the flag should not outlive the frame")
	}
}

func TestTheBoardsFoldRowAndTheBlockClose(t *testing.T) {
	forceASCII(t)
	sc := sceneVeryLong()
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sc, size[0], size[1])
		v := ansi.Strip(m.View())
		for _, line := range strings.Split(v, "\n") {
			for _, cell := range strings.Split(line, "│") {
				c := strings.TrimSpace(cell)
				if !strings.HasPrefix(c, "↑ ") || !strings.Contains(c, " legs") {
					continue
				}
				var hidden int
				if _, err := fmt.Sscanf(c, "↑ %d legs", &hidden); err != nil {
					continue
				}
				// The fold row plus the legs drawn under it is the block's
				// sum: auth 160, etl 120.
				_ = hidden
			}
		}
		// Pin the arithmetic directly on the column the fold row is drawn
		// on, at every height a band can give it: the row the fold paints
		// over is a leg at some of them, and the count must close at all.
		key := m.viewOrderKeys()[0]
		tr := m.trails[key]
		r := m.boardRows()[key]
		_, cw := boardColumns(size[0]-2*edgePad, m.drawnCount(m.viewOrder()))
		for h := 16; h <= 40; h++ {
			col := m.boardColumn(key, r, cw, h)
			hidden, drawn := -1, 0
			for _, line := range col {
				c := ansi.Strip(line)
				if strings.Contains(c, "↑ ") && strings.Contains(c, " legs") {
					fmt.Sscanf(strings.TrimSpace(c), "↑ %d legs", &hidden)
					continue
				}
				// A leg row: its mark, or the rail where a run of one class
				// continues, then the class word — never a block row.
				if hidden >= 0 && legRowPattern.MatchString(c) && !strings.Contains(c, " legs") {
					drawn++
				}
			}
			if hidden < 0 {
				t.Fatalf("%dx%d at %d rows: no fold row on the column:\n%s", size[0], size[1], h, strings.Join(col, "\n"))
			}
			if hidden+drawn != len(tr.Legs) {
				t.Errorf("%dx%d at %d rows: the fold row (%d) and the legs beneath it (%d) do not close on the block's %d (#380):\n%s", size[0], size[1], h, hidden, drawn, len(tr.Legs), strings.Join(col, "\n"))
			}
		}
	}
}

func TestAWaitWorthARowCountsAlone(t *testing.T) {
	forceASCII(t)
	m, _ := twoToolsWaiting(t, 80, 24)
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "◉ waited on you · 4 prompts") || !strings.Contains(v, "│ the trail ─") {
		t.Errorf("a trail of one of each with a wait worth a row should draw the wait row and its seam (#357, #380):\n%s", v)
	}
	if strings.Contains(v, " legs ") {
		t.Errorf("a trail of one of each should draw no class row:\n%s", v)
	}
}

func TestTheCardKeepsTheClauseItShedForATagItDropped(t *testing.T) {
	forceASCII(t)
	m, _ := twoToolsWaiting(t, 120, 34)
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "on you 12m today") && !strings.Contains(v, "◉ waited on you") {
		t.Errorf("at 120 the api session's wait should be on the card or the block (#380):\n%s", v)
	}
	// The card row itself, on a session the block covers nowhere: the
	// harness card at 120 keeps its running clause on the row the tmux
	// rung left, and the card is two rows, not three (#380, #382).
	sc := sceneSubagents()
	m = sceneModel(sc, 120, 34)
	for _, d := range []string{"1", "2", "3", "4", "5"} {
		pressKey(m, d)
		poll(m, sc)
		if s, ok := m.selected(); ok && sessionName(s.Info) == "harness" && strings.Contains(ansi.Strip(m.View()), "⌁ harness:1.0") {
			break
		}
	}
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	cells := trailCells(m)
	head := -1
	for i, c := range cells {
		if strings.Contains(c, "[session]") {
			head = i
			break
		}
	}
	if head < 0 || head+2 >= len(cells) {
		t.Fatalf("no session card on the frame:\n%s", ansi.Strip(m.View()))
	}
	if !strings.Contains(cells[head+1], "for ") || !strings.Contains(cells[head+1], "scout") {
		t.Errorf("the card's second row should carry the running clause the rung gave way to (#380): %q", cells[head+1])
	}
	if strings.TrimSpace(cells[head+2]) == "" {
		t.Errorf("the card is head and delta, not a blank third row (#382):\n%s", strings.Join(cells[head:head+4], "\n"))
	}
}

func TestTheTitleKeepsItsRedsWhereTheReplyBoxCoversTheBlock(t *testing.T) {
	forceASCII(t)
	sc := sceneManyIdle()
	m := sceneModel(sc, 80, 24)
	// The corpus's own route: webapp is the session whose block counts a
	// red and whose pane takes a reply (`many-idle-80x24` after `r`).
	for _, d := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"} {
		pressKey(m, d)
		poll(m, sc)
		if s, ok := m.selected(); ok && sessionName(s.Info) == "webapp" {
			break
		}
	}
	if s, ok := m.selected(); !ok || sessionName(s.Info) != "webapp" {
		t.Fatalf("no digit reaches webapp")
	}
	if !blockCounts(m.trail) || !strings.Contains(trailDay(m.trail, m.now, false), " red") {
		t.Fatalf("webapp should count a block with a red: %q", trailDay(m.trail, m.now, false))
	}
	pressKey(m, "r")
	poll(m, sc)
	m.View()
	if !m.replyBox.on {
		t.Fatalf("r did not open the reply box on webapp:\n%s", ansi.Strip(m.View()))
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, " legs") && !strings.Contains(v, " red") {
		t.Errorf("the box covers the block and the title gave its reds away: nothing on the frame counts them (#380):\n%s", v)
	}
	if !strings.Contains(v, "TRAIL · webapp · 1h · 2 red") {
		t.Errorf("the title should carry the reds the covered block cannot (#380):\n%s", v)
	}
	pressKey(m, "esc")
}

func TestAStuckHeadsFigureIsSaidOnceAcrossTheSeam(t *testing.T) {
	forceASCII(t)
	sc := sceneFewOngoing()
	m := sceneModel(sc, 80, 24)
	found := false
	for _, d := range []string{"1", "2", "3", "4", "5", "6"} {
		pressKey(m, d)
		poll(m, sc)
		if strings.Contains(ansi.Strip(m.View()), "silent 4m") && blockCounts(m.trail) {
			found = true
			break
		}
	}
	if !found {
		t.Skip("no few-ongoing session with a stuck HEAD under a block")
	}
	cells := trailCells(m)
	col := strings.Join(cells, "\n")
	if strings.Count(col, "silent 4m") > 1 {
		t.Errorf("the class row repeats the stuck figure HEAD's own row says beneath the seam (#380):\n%s", col)
	}
}

// twoToolsWaiting opens two-tools' Claude api session, which waits on you
// over a trail of one of each, on its legs at w×h.
func twoToolsWaiting(t *testing.T, w, h int) (*Model, scene) {
	t.Helper()
	sc := sceneTwoTools()
	m := sceneModel(sc, w, h)
	key := sessionKey("api-claude")
	if promptWaits(sc.trails[key]) < waitNotable {
		t.Fatalf("the scene's claude api session waits %s, under the threshold", promptWaits(sc.trails[key]))
	}
	found := false
	for _, d := range []string{"1", "2", "3", "4"} {
		pressKey(m, d)
		poll(m, sc)
		if m.selectedKey == key {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no digit reaches the claude api session")
	}
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	return m, sc
}

// viewOrderKeys is the board's sessions, by key, in the board's order.
func (m *Model) viewOrderKeys() []string {
	var keys []string
	for _, i := range m.viewOrder() {
		keys = append(keys, m.sessions[i].Info.Key())
	}
	return keys
}

// legRowPattern is the shape of a leg row on a board column.
var legRowPattern = regexp.MustCompile(`^[◆●◍▲│]▸? ?(scout|design|build|fix|test|ship|docs) `)

// The panel's second pass, alarm-storm, folded (#381).

// blankTail counts the blank rows a trail column ends on, above the
// footer's rule (#385: the pin once counted from the rule itself, and
// so counted nothing).
func blankTail(cells []string) int {
	rule := len(cells) - 1
	for rule >= 0 && !strings.HasPrefix(strings.TrimSpace(cells[rule]), "─") {
		rule--
	}
	blank := 0
	for i := rule - 1; i >= 0 && strings.TrimSpace(cells[i]) == ""; i-- {
		blank++
	}
	return blank
}

func TestTheColumnReservesOnlyTheRowsTheBlockDraws(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}} {
		m, sc := longSession(t, size[0], size[1])
		pressKey(m, "r") // the offer's box, over the trail column
		poll(m, sc)
		cells := trailCells(m) // drawn: the box's place is the frame's
		if !m.replying || !m.replyBox.on {
			t.Fatalf("%dx%d: r did not open the reply box: replying %v, box %v", size[0], size[1], m.replying, m.replyBox.on)
		}
		cells = trailCells(m)
		// Under the box the block stands down, and the trail takes the
		// rows back: no blank tail under the column (#381).
		if blank := blankTail(cells); blank > 0 {
			t.Errorf("%dx%d: the column ends on %d blank rows while the block stands down:\n%s", size[0], size[1], blank, strings.Join(cells, "\n"))
		}
		// The block stands where a row of it is uncovered, and stands
		// down where the box covers all of it: either way the title and
		// the column agree (#380), and the rows are never blank.
		pressKey(m, "esc")
	}
	// The frames the fold was written for are the list's trail column at
	// Lv1 — `r` on the opening selection and on etl — not the session
	// view the walk above ends on (alarm-storm 3, #385).
	sc := sceneVeryLong()
	for _, size := range [][2]int{{80, 24}, {100, 30}} {
		for _, pick := range []string{"", "etl"} {
			m := sceneModel(sc, size[0], size[1])
			if pick != "" {
				for _, d := range []string{"1", "2", "3", "4", "5"} {
					pressKey(m, d)
					poll(m, sc)
					if s, ok := m.selected(); ok && sessionName(s.Info) == pick {
						break
					}
				}
			}
			if m.level != levelTrail {
				t.Fatalf("%dx%d: the deck should open on the list and its trail, level %d", size[0], size[1], m.level)
			}
			pressKey(m, "r")
			poll(m, sc)
			m.View()
			if !m.replyBox.on {
				t.Fatalf("%dx%d %q: r did not open the reply box at Lv1", size[0], size[1], pick)
			}
			cells := trailCells(m)
			if blank := blankTail(cells); blank > 0 {
				t.Errorf("%dx%d %q at Lv1: the column ends on %d blank rows while the block stands down (#381):\n%s", size[0], size[1], pick, blank, strings.Join(cells, "\n"))
			}
		}
	}
}

// The panel's third pass — alarm-storm and fleet-hygiene — folded (#385).

// The bands are paid out of the unspent rows: as many bands whole as the
// rows allow, and among those the pick that leaves the fewest rows blank.
func TestTheBandsArePaidAsManyAsTheRowsAllow(t *testing.T) {
	for _, c := range []struct {
		spare int
		need  []int
		idle  []int
		want  []int
	}{
		{4, []int{3, 2, 1}, nil, []int{3, 0, 1}}, // two bands either way: the pick that spends all four (fleet-hygiene 2)
		{3, []int{3, 2, 1}, nil, []int{0, 2, 1}}, // two bands beat one
		{1, []int{3, 2}, nil, []int{0, 0}},       // a band is paid whole or not at all
		{0, []int{1}, nil, []int{0}},
		{5, []int{0, 2}, nil, []int{0, 2}},
		{4, []int{3, 2, 1}, []int{0, 1, 0}, []int{0, 2, 1}}, // the idle column's ask before the fewest blank rows (#388)
		{4, []int{3, 2, 1}, []int{1, 1, 0}, []int{3, 0, 1}}, // idle asks equal: the fewest blank rows again
	} {
		got := payDebts(c.spare, c.need, c.idle)
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("payDebts(%d, %v) = %v, want %v", c.spare, c.need, got, c.want)
		}
	}
}

// A column dead on the quota is measured with the `dead` row it draws, so
// the rows it is owed are paid back and its ask stands: alarm-storm's
// mobile at 152x40 says what it was doing, with its column count unchanged.
func TestADeadColumnIsMeasuredAsItIsDrawn(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneAlarmStorm(), 152, 40)
	v := ansi.Strip(m.View())
	if got := boardColumnsDrawn(v); got != 7 {
		t.Fatalf("alarm-storm at 152x40 should draw seven columns, drew %d:\n%s", got, v)
	}
	if !strings.Contains(v, `◉ "fix the login crash`) {
		t.Errorf("mobile's ask is folded while the board has the rows (#385):\n%s", v)
	}
	if strings.Contains(v, "↑ began 49m ago") {
		t.Errorf("mobile still pays its missing row with its ask (#385):\n%s", v)
	}
	if !strings.Contains(v, "dead 9m") {
		t.Errorf("mobile's dead row should be on the frame:\n%s", v)
	}
}

// Where the fold took the ask and its stub both, the seam says when the
// trail began: `│ the trail · began 3d ago ────`, for no row at all.
func TestTheSeamSaysWhenTheTrailBeganWhereTheFoldTookTheAsk(t *testing.T) {
	forceASCII(t)
	sc := sceneManyIdle()
	m := sceneModel(sc, 120, 34)
	for _, k := range canonicalKeys {
		pressKey(m, k)
		poll(m, sc)
		if k != "x" {
			continue
		}
		v := ansi.Strip(m.View())
		if !strings.Contains(v, "│ the trail · began 3d ago ─") {
			t.Errorf("notebooks' column folds its ask and its stub and no row says when the trail began (#385):\n%s", v)
		}
		if !strings.Contains(v, "notebooks") {
			t.Errorf("the frame should draw notebooks' column:\n%s", v)
		}
		return
	}
	t.Fatal("the walk has no x")
}

// The panel's third pass, subagents — folded (#387).

// The board's fold keeps the head of a lane's finding: where the fold row
// takes the row a parent stood on, the parent takes the last row of its
// children's group, not the first, and the group closes on the cut with
// an ellipsis — so porter's one report is read from its opening words.
func TestTheFoldKeepsTheHeadOfAFinding(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	m := sceneModel(sc, 120, 34)
	check := func(when string) {
		v := ansi.Strip(m.View())
		if m.level != levelBoard {
			return
		}
		lines := strings.Split(v, "\n")
		for i, line := range lines {
			at := strings.Index(line, "├─◈ Score encoder gates")
			if at >= 0 && i+2 < len(lines) {
				// The row beneath, in the same column's cells.
				col := len([]rune(line[:at]))
				next := []rune(lines[i+1])
				if col < len(next) {
					next = next[col:min(col+40, len(next))]
				}
				if !strings.Contains(string(next), "3 defects found") {
					t.Errorf("%s: the finding's head is cut, the group opens mid-sentence (#387):\n%s", when, strings.Join(lines[i:i+3], "\n"))
				}
			}
			for _, bad := range []string{"│  └ root cause", "│  ├ oracle; two are"} {
				if strings.HasPrefix(strings.TrimSpace(line), bad) {
					t.Errorf("%s: a finding read from mid-sentence: %q (#387)", when, line)
				}
			}
		}
	}
	check("opening")
	for _, k := range canonicalKeys {
		pressKey(m, k)
		poll(m, sc)
		check("after " + k)
	}
	// The group closes on the cut: the last row it keeps wears the
	// group's last mark and an ellipsis.
	m = sceneModel(sc, 120, 34)
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "│  └ oracle; two are the same root…") {
		t.Errorf("the cut group should close on `└` and an ellipsis (#387):\n%s", v)
	}
}

// Where no strip is drawn, the two rows the board held back for it are
// unspent rows too: alarm-storm at 120x41 draws seven columns, no strip,
// and mobile's ask instead of `↑ began 49m ago` (#389).
func TestTheStripsRowsAreSpentWhereNoStripIsDrawn(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneAlarmStorm(), 120, 41)
	v := ansi.Strip(m.View())
	if got := boardColumnsDrawn(v); got != 7 {
		t.Fatalf("alarm-storm at 120x41 should draw seven columns, drew %d:\n%s", got, v)
	}
	if strings.Contains(v, "+1 more") {
		t.Fatalf("alarm-storm at 120x41 should draw no strip:\n%s", v)
	}
	if !strings.Contains(v, `◉ "fix the login crash`) || strings.Contains(v, "↑ began 49m ago") {
		t.Errorf("mobile's ask folds into the two rows held for a strip the board does not draw (#389):\n%s", v)
	}
}

// A one-row group the fold takes leaves nothing of the finding: the lane's
// own row carries its opening words, so a report is never taken without a
// word (#390, subagents' fourth pass).
func TestTheFoldNeverSwallowsAOneRowFinding(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	for _, size := range [][2]int{{152, 40}, {220, 48}} {
		m := sceneModel(sc, size[0], size[1])
		keys := append(append(append([]string(nil), canonicalKeys...), "esc"), sc.extra...)
		seen := false
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
			if m.level != levelBoard {
				continue
			}
			lines := strings.Split(ansi.Strip(m.View()), "\n")
			for i, l := range lines {
				at := strings.Index(l, "├─◈ Score encoder")
				if at < 0 || i == 0 {
					continue
				}
				col := len([]rune(l[:at]))
				above := []rune(lines[i-1])
				if col >= len(above) || !strings.HasPrefix(strings.TrimSpace(string(above[col:min(col+40, len(above))])), "↑ ") {
					continue
				}
				seen = true
				next := ""
				if i+1 < len(lines) {
					next = lines[i+1]
				}
				if !strings.Contains(l, "3 defects") && !strings.Contains(next, "3 defects") {
					t.Errorf("%dx%d after %q: the lane's whole report is gone and no row says so (#390):\n%s", size[0], size[1], k, strings.Join(lines[i-1:min(i+2, len(lines))], "\n"))
				}
			}
		}
		if !seen {
			t.Errorf("%dx%d: no frame of the walk stands porter's lane under a fold row", size[0], size[1])
		}
	}
}

// A strip the box paints over whole takes the first row clear of the box:
// two-tools at 120x22 and 120x25 after `r` names docs on the board's last
// row rather than under the box (#391).
func TestAStripTheBoxWouldCoverStandsClearOfIt(t *testing.T) {
	forceASCII(t)
	sc := sceneTwoTools()
	for _, h := range []int{22, 25} {
		m := sceneModel(sc, 120, h)
		m.View()
		pressKey(m, "r")
		poll(m, sc)
		v := ansi.Strip(m.View())
		if !m.replyBox.on {
			t.Fatalf("120x%d: r opens no box:\n%s", h, v)
		}
		lines := strings.Split(v, "\n")
		bottom, strip := -1, -1
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "└") {
				bottom = i
			}
			if strings.Contains(line, "+1 more · 4 ○ docs") {
				strip = i
			}
		}
		if strip < 0 {
			t.Errorf("120x%d: the docs session is named nowhere, the strip painted out by the box (#391):\n%s", h, v)
		} else if strip <= bottom {
			t.Errorf("120x%d: the strip stands under the box (row %d, box bottom %d):\n%s", h, strip, bottom, v)
		}
		pressKey(m, "esc")
	}
}

// The clause reads the finding from the trail: where the trail's own
// viewport opened on the finding and cut its head, the lane's row still
// carries the opening words, not the closing ones (#392).
func TestTheClauseReadsTheFindingsHead(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	for _, c := range []struct {
		w, h       int
		lane, head string
	}{{194, 26, "├─◈ Score encoder", "· 3 defects found"}, {120, 23, "├─◈ Read every kic", "· Seven of nine"}} {
		m := sceneModel(sc, c.w, c.h)
		v := ansi.Strip(m.View())
		found := false
		for _, line := range strings.Split(v, "\n") {
			at := strings.Index(line, c.lane)
			if at < 0 {
				continue
			}
			found = true
			rest := line[at:]
			if !strings.Contains(rest, c.head) {
				t.Errorf("%dx%d: the lane's clause should read the finding's head %q (#392): %q", c.w, c.h, c.head, strings.TrimRight(rest, " "))
			}
			if strings.Contains(rest, "· …") {
				t.Errorf("%dx%d: the clause reads the finding's closing words: %q", c.w, c.h, strings.TrimRight(rest, " "))
			}
		}
		if !found {
			t.Errorf("%dx%d: the lane %q is not on the frame:\n%s", c.w, c.h, c.lane, v)
		}
	}
}
