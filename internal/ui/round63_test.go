package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/journey"
)

// Round sixty-three: the board's summary (#373). `s` on the board swaps
// every column's trail for its legs counted by class; `s` or `esc` the
// trails back; `tab` into a column carries the counts in as the legs'
// summary. A column of one of each keeps its trail; a board of nothing
// but those refuses with the fact.

// boardSummaryModel opens very-long's board at w×h with the summary on.
func boardSummaryModel(t *testing.T, w, h int) (*Model, scene) {
	t.Helper()
	sc := sceneVeryLong()
	m := sceneModel(sc, w, h)
	if m.level != levelBoard || !m.boardShown() {
		t.Fatalf("%dx%d: the scene should open on the board", w, h)
	}
	press(m, "s")
	if !m.boardSummary {
		t.Fatalf("%dx%d: s on the board did not open its summary: %q", w, h, m.note)
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

// columnCells is the frame's rows cut to one board column, by the
// board's own geometry: column 0 is the selected one, at the place the
// band puts it, and the rest follow at the column width and a gutter.
func columnCells(m *Model, col int) []string {
	inner := m.width - 2*edgePad
	x, _, _, _, ok := m.boardBandAt(inner)
	_, cw := boardColumns(inner, m.drawnCount(m.viewOrder()))
	if !ok || cw == 0 {
		return nil
	}
	start := edgePad + x + col*(cw+gutterWidth)
	var out []string
	for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
		r := []rune(row)
		if start >= len(r) {
			out = append(out, "")
			continue
		}
		out = append(out, string(r[start:min(start+cw, len(r))]))
	}
	return out
}

func TestTheBoardCountsEveryColumn(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m, _ := boardSummaryModel(t, size[0], size[1])
		v := ansi.Strip(m.View())
		if !strings.Contains(strings.Split(v, "\n")[0], "· board · summary") {
			t.Errorf("%dx%d: the header does not say the board is counts:\n%s", size[0], size[1], v)
		}
		// Every class row of the selected column, in the trail's order,
		// with the trail's own counts and spans to the minute.
		col := strings.Join(columnCells(m, 0), "\n")
		counts := classCounts(m.trails[m.selectedKey])
		for _, c := range summaryOrder {
			if counts[c] < 2 {
				continue
			}
			if !strings.Contains(col, c.String()) || !strings.Contains(col, plural(counts[c], "leg")) {
				t.Errorf("%dx%d: the selected column lacks its %s row (%d legs):\n%s", size[0], size[1], c, counts[c], col)
			}
		}
		if !strings.Contains(col, "◉ waited on you") {
			t.Errorf("%dx%d: the column drops the wait the trail waited:\n%s", size[0], size[1], col)
		}
		if strings.Contains(col, "↑ ") || strings.Contains(col, "above") {
			t.Errorf("%dx%d: the column still draws the trail's fold row over its counts:\n%s", size[0], size[1], col)
		}
		// The footer names the way back, and the help the key.
		foot := strings.Split(v, "\n")
		if !strings.Contains(foot[len(foot)-1], "s/esc trails") {
			t.Errorf("%dx%d: the footer does not name the way back to the trails: %q", size[0], size[1], foot[len(foot)-1])
		}
		press(m, "?")
		if h := ansi.Strip(m.View()); !strings.Contains(h, "summary: every column's legs by class") {
			t.Errorf("%dx%d: the board's help has no row for s:\n%s", size[0], size[1], h)
		}
		press(m, "?")
		press(m, "esc")
		if m.boardSummary || strings.Contains(ansi.Strip(m.View()), "· summary") {
			t.Errorf("%dx%d: esc on the board should bring the trails back", size[0], size[1])
		}
		if m.level != levelBoard {
			t.Errorf("%dx%d: esc on the board's summary should stay on the board, not zoom: level %d", size[0], size[1], m.level)
		}
		press(m, "s")
		press(m, "s")
		if m.boardSummary {
			t.Errorf("%dx%d: the second s should close the board's summary", size[0], size[1])
		}
	}
}

func TestAColumnOfOneOfEachKeepsItsTrailUnderTheBoardsSummary(t *testing.T) {
	forceASCII(t)
	m, _ := boardSummaryModel(t, 152, 40)
	keys := m.viewOrderKeys()
	short := -1
	for i, k := range keys {
		if tr, ok := m.trails[k]; ok && !boardColumnCounts(tr) {
			short = i
		}
	}
	if short < 0 {
		t.Fatalf("very-long should have a column with nothing to count")
	}
	col := strings.Join(columnCells(m, short), "\n")
	if strings.Contains(col, " legs") || !strings.Contains(col, "◆ build  the flag") {
		t.Errorf("a column of one of each should keep its trail under the board's summary:\n%s", col)
	}
	// A board whose every column is one of each refuses with the fact.
	sc := sceneFleetHygiene()
	fh := sceneModel(sc, 152, 40)
	if fh.level != levelBoard || !fh.boardShown() {
		t.Skip("fleet-hygiene does not open on a board at 152")
	}
	if fh.boardCountsSomething() {
		t.Skip("fleet-hygiene's board has a column that counts")
	}
	press(fh, "s")
	if fh.boardSummary || !strings.Contains(ansi.Strip(fh.View()), "one of each · every column") {
		t.Errorf("a board of one of each should refuse with the fact: note %q", fh.note)
	}
}

func TestTabFromTheBoardsSummaryLandsOnTheLegsCounted(t *testing.T) {
	forceASCII(t)
	m, _ := boardSummaryModel(t, 152, 40)
	press(m, "tab")
	v := ansi.Strip(m.View())
	if m.level != levelWaypoints || !m.summary || !strings.Contains(v, "[summary]") {
		t.Errorf("tab from the board's summary should open the legs counted: level %d, summary %v\n%s", m.level, m.summary, v)
	}
	// The legs' own esc closes their summary first (#348); the next is
	// the board, still counted.
	press(m, "esc")
	if m.level != levelWaypoints || m.summary {
		t.Fatalf("esc on the legs' summary should be the legs: level %d, summary %v", m.level, m.summary)
	}
	press(m, "esc")
	if m.level != levelBoard || !m.boardSummary || !strings.Contains(strings.Split(ansi.Strip(m.View()), "\n")[0], "· board · summary") {
		t.Errorf("esc from the legs should land back on the board, still counted: level %d, board summary %v", m.level, m.boardSummary)
	}
	// A column selected by its digit is the same board, still counted.
	press(m, "3")
	if !m.boardSummary || m.level != levelBoard {
		t.Errorf("a digit on the board's summary should keep it: board summary %v, level %d", m.boardSummary, m.level)
	}
}

func TestTheBoardsSummaryIsTheFleetsOwnFooterTrade(t *testing.T) {
	forceASCII(t)
	// `s summary` is traded onto the board's row only where it costs no
	// key, no note and no pane (#63, #348).
	sc := sceneTwoTools()
	m := sceneModel(sc, 152, 40)
	for _, k := range append(append([]string(nil), canonicalKeys...), "esc", "2") {
		pressKey(m, k)
		poll(m, sc)
	}
	foot := ansi.Strip(m.footerLine(150))
	if !strings.HasSuffix(foot, "⌁ dev:2.0") {
		t.Errorf("the trade cost the refusal's footer its pane: %q", foot)
	}
	// And below the board's width the key is still the legs' alone.
	narrow := sceneModel(sceneVeryLong(), 80, 24)
	press(narrow, "s")
	if narrow.boardSummary || !strings.Contains(ansi.Strip(narrow.View()), "the summary is the legs'") {
		t.Errorf("s on the list should refuse with the way in: note %q", narrow.note)
	}
}

func TestSubagentsWalkEndsOnTheBoardsSummary(t *testing.T) {
	forceASCII(t)
	extra := sceneSubagents().extra
	if len(extra) < 2 || extra[len(extra)-1] != "s" || extra[len(extra)-2] != "shift+tab" {
		t.Fatalf("subagents' walk should end on the board's summary: %v", extra)
	}
	sc := sceneSubagents()
	m := sceneModel(sc, 120, 34)
	for _, k := range append(append(append([]string(nil), canonicalKeys...), "esc"), extra...) {
		pressKey(m, k)
		poll(m, sc)
	}
	v := ansi.Strip(m.View())
	if m.level != levelBoard || !m.boardSummary || !strings.Contains(v, "◈ agent  4 lanes") {
		t.Errorf("the walk's last frame should be the board counted, porter's lanes row on it: level %d\n%s", m.level, v)
	}
	var _ = journey.Build
}
