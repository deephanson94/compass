package ui

import (
	"fmt"
	"regexp"
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

// The panel's first pass on #374, second-day, folded (#375).

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

// The panel's first pass on #374, subagents, folded (#376).

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
			// and then not on the lanes row (#376).
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
}

// The panel's first pass on #374 — alarm-storm, fleet-hygiene, two-tools
// and second-day's second — folded (#377).

// boardColumnsDrawn counts the columns a board frame draws, by the digit
// and glyph that head each.
func boardColumnsDrawn(v string) int {
	n := 0
	for _, line := range strings.Split(v, "\n") {
		if strings.Contains(line, "│") || strings.HasPrefix(strings.TrimSpace(line), "▸") || len(line) > 2 && line[1] >= '1' && line[1] <= '9' {
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
	for _, sc := range []scene{sceneAlarmStorm(), sceneManyIdle()} {
		m := sceneModel(sc, 120, 34)
		v := ansi.Strip(m.View())
		if got := boardColumnsDrawn(v); got < 6 {
			t.Errorf("%s at 120x34 should draw six columns as it did before the block, drew %d:\n%s", sc.name, got, v)
		}
		if !strings.Contains(v, " legs") || !strings.Contains(v, "│ the trail ─") {
			t.Errorf("%s at 120x34 should keep its blocks while it keeps its columns:\n%s", sc.name, v)
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
		// Pin the arithmetic directly on the column the fold row is drawn on.
		key := m.viewOrderKeys()[0]
		tr := m.trails[key]
		r := m.boardRows()[key]
		_, cw := boardColumns(size[0]-2*edgePad, m.drawnCount(m.viewOrder()))
		col := m.boardColumn(key, r, cw, 30)
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
			t.Fatalf("%dx%d: no fold row on the column:\n%s", size[0], size[1], strings.Join(col, "\n"))
		}
		if hidden+drawn != len(tr.Legs) {
			t.Errorf("%dx%d: the fold row (%d) and the legs beneath it (%d) do not close on the block's %d (#377):\n%s", size[0], size[1], hidden, drawn, len(tr.Legs), strings.Join(col, "\n"))
		}
	}
}

func TestAWaitWorthARowCountsAlone(t *testing.T) {
	forceASCII(t)
	m, _ := twoToolsWaiting(t, 80, 24)
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "◉ waited on you · 4 prompts") || !strings.Contains(v, "│ the trail ─") {
		t.Errorf("a trail of one of each with a wait worth a row should draw the wait row and its seam (#357, #377):\n%s", v)
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
		t.Errorf("at 120 the api session's wait should be on the card or the block (#377):\n%s", v)
	}
}

func TestTheTitleKeepsItsRedsWhereTheReplyBoxCoversTheBlock(t *testing.T) {
	forceASCII(t)
	sc := sceneManyIdle()
	m := sceneModel(sc, 80, 24)
	found := false
	for _, d := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"} {
		pressKey(m, d)
		poll(m, sc)
		if blockCounts(m.trail) && strings.Contains(trailDay(m.trail, m.now, false), " red") {
			found = true
			break
		}
	}
	if !found {
		t.Skip("no many-idle session with a block and a red count")
	}
	pressKey(m, "r")
	poll(m, sc)
	if !m.replyBox.on {
		t.Skip("r did not open the reply box here")
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, " legs") && !strings.Contains(v, " red") {
		t.Errorf("the box covers the block and the title gave its reds away: nothing on the frame counts them (#377):\n%s", v)
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
		t.Errorf("the class row repeats the stuck figure HEAD's own row says beneath the seam (#377):\n%s", col)
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
