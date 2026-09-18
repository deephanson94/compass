package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Round sixty-five: the counts open (#393). The block above a trail is
// the cursor's too: `k` off the trail's first row climbs into it, `space`
// on a class or the lanes opens the group into its rows, hung on the
// waypoint rail, and folds it again; a leg or lane row in the block is
// that leg or lane, so the reader, `tab` and `enter` do there what they
// do on the trail's own row for it. The board keeps its counts closed.

// climb walks the legs' cursor to the trail's first row and one `k`
// further, into the block.
func climb(t *testing.T, m *Model) {
	t.Helper()
	for i := 0; i < 1000 && m.cursor > 0; i++ {
		press(m, "k")
	}
	if m.cursor != 0 {
		t.Fatalf("the cursor should reach the trail's first row: at %d", m.cursor)
	}
	press(m, "k")
	if !m.inBlock() {
		t.Fatalf("k off the trail's first row should climb into the block")
	}
}

// blockCells is the trail column's rows above the seam.
func blockCells(m *Model) []string {
	var out []string
	for _, r := range trailCells(m) {
		if strings.Contains(r, "the trail ─") {
			break
		}
		out = append(out, r)
	}
	return out
}

// cursorRow is the one row of the column wearing the cursor's mark, or "".
func cursorRow(cells []string) string {
	for _, r := range cells {
		if rs := []rune(r); len(rs) > 1 && rs[1] == '▸' {
			return r
		}
	}
	return ""
}

func TestKOffTheTrailsFirstRowClimbsIntoTheBlock(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {152, 40}} {
		m, _ := longSession(t, size[0], size[1])
		climb(t, m)
		cells := trailCells(m)
		row := cursorRow(cells)
		if !strings.Contains(row, "docs") || !strings.Contains(row, "16 legs") {
			t.Errorf("%dx%d: the climb should land on the block's last standing row, the docs count: %q\n%s", size[0], size[1], row, strings.Join(cells, "\n"))
		}
		if n := strings.Count(strings.Join(cells, "\n"), "▸"); n != 1 {
			t.Errorf("%dx%d: one cursor on the column, %d:\n%s", size[0], size[1], n, strings.Join(cells, "\n"))
		}
		press(m, "k")
		if !strings.Contains(cursorRow(trailCells(m)), "ship") {
			t.Errorf("%dx%d: k in the block walks its rows: %q", size[0], size[1], cursorRow(trailCells(m)))
		}
		press(m, "j")
		press(m, "j")
		if m.inBlock() || m.cursor != 0 {
			t.Errorf("%dx%d: j off the block's last row should be the trail's first: in block %v, cursor %d", size[0], size[1], m.inBlock(), m.cursor)
		}
		if row := cursorRow(trailCells(m)); !strings.Contains(row, "1/12") {
			t.Errorf("%dx%d: the trail's first row should wear the cursor: %q", size[0], size[1], row)
		}
	}
}

func TestSpaceOpensAClassIntoItsLegsAndFoldsItAgain(t *testing.T) {
	forceASCII(t)
	m, _ := longSession(t, 152, 40)
	climb(t, m)
	closed := strings.Join(trailCells(m), "\n")
	press(m, " ")
	open := trailCells(m)
	hung := 0
	for _, r := range open {
		if strings.Contains(r, "├ ◆ docs") || strings.Contains(r, "└ ◆ docs") {
			hung++
		}
	}
	if hung != 16 {
		t.Errorf("space on `docs 16 legs` should hang its sixteen legs on the rail, %d:\n%s", hung, strings.Join(open, "\n"))
	}
	if !strings.Contains(strings.Join(open, "\n"), "the trail ─") {
		t.Errorf("the trail should still stand under the open block:\n%s", strings.Join(open, "\n"))
	}
	w, h := m.trailBox()
	if h+m.blockHeightHere()+trailChrome != 40-5 {
		t.Errorf("the trail's viewport should be the column less the chrome and the open block: %d rows at width %d, block %d", h, w, m.blockHeightHere())
	}
	press(m, " ")
	if got := strings.Join(trailCells(m), "\n"); got != closed {
		t.Errorf("space again should fold the class and draw the frame as it stood:\n%s", got)
	}
	// A leg row folds its class and stands on it.
	press(m, " ")
	press(m, "j")
	press(m, "j")
	if !strings.Contains(cursorRow(trailCells(m)), "├ ◆ docs") {
		t.Fatalf("j under an open class walks its legs: %q", cursorRow(trailCells(m)))
	}
	press(m, " ")
	if row := cursorRow(trailCells(m)); !strings.Contains(row, "16 legs") {
		t.Errorf("space on a leg should fold its class and stand on the count: %q", row)
	}
	if got := strings.Join(trailCells(m), "\n"); got != closed {
		t.Errorf("folded from a leg, the frame is as it stood:\n%s", got)
	}
}

func TestTheOpenBlockKeepsTheTrailsFloorAndWindowsItsRows(t *testing.T) {
	forceASCII(t)
	m, _ := longSession(t, 80, 24)
	climb(t, m)
	press(m, "k") // ship
	press(m, "k") // test, 32 legs: more than the frame holds
	press(m, " ")
	cells := trailCells(m)
	joined := strings.Join(cells, "\n")
	if !strings.Contains(joined, "↑ ") || !strings.Contains(joined, "▾ ") || !strings.Contains(joined, " more below") {
		t.Errorf("a window that hides rows should say so on its edges:\n%s", joined)
	}
	seam := -1
	for i, r := range cells {
		if strings.Contains(r, "the trail ─") {
			seam = i
		}
	}
	if seam < 0 {
		t.Fatalf("the seam should stand under the windowed block:\n%s", joined)
	}
	if below := 24 - 3 - seam; below < blockTrailFloor { // the frame's rows under the seam, before the rule and the footer
		t.Errorf("the trail keeps its floor of %d rows under an open block, %d:\n%s", blockTrailFloor, below, joined)
	}
	// The cursor stays on the frame, never on an edge, down the whole class.
	for i := 0; i < 40; i++ {
		press(m, "j")
		if !m.inBlock() {
			break
		}
		row := cursorRow(blockCells(m))
		if row == "" || strings.Contains(row, " more ") {
			t.Fatalf("j %d: the cursor's row should be drawn inside the window's edges: %q\n%s", i, row, strings.Join(trailCells(m), "\n"))
		}
	}
	if m.inBlock() {
		t.Errorf("forty j should walk off the open class, past the counts under it, and onto the trail")
	}
	press(m, "G")
	if m.inBlock() || !m.trailPinned {
		t.Errorf("G is the present from the block as from the trail")
	}
}

func TestALegRowInTheBlockIsThatLeg(t *testing.T) {
	forceASCII(t)
	m, _ := longSession(t, 152, 40)
	climb(t, m)
	press(m, " ")
	press(m, "j")
	rows := m.blockRowsHere()
	r := rows[m.blockCursor]
	if r.kind != "leg" {
		t.Fatalf("j under the open class should stand on a leg, %q", r.kind)
	}
	leg := m.trail.Legs[r.leg]
	if !m.anchorAt.Equal(leg.Start) || m.anchorText != leg.Label {
		t.Errorf("the reader should follow the leg the block row is: anchored %v %q, the leg %v %q", m.anchorAt, m.anchorText, leg.Start, leg.Label)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "READER · auth") {
		t.Fatalf("the reader stands beside the legs:\n%s", v)
	}
	press(m, "tab")
	if m.level != levelReader || !m.anchorAt.Equal(leg.Start) {
		t.Errorf("tab on a block leg row reads that leg: level %d, anchored %v", m.level, m.anchorAt)
	}
	pressKey(m, "esc")
	if m.level != levelWaypoints || !m.inBlock() {
		t.Errorf("esc from the reader comes back to the block row it left: level %d, in block %v", m.level, m.inBlock())
	}
}

func TestTheFooterNamesSpaceInTheBlockOnly(t *testing.T) {
	forceASCII(t)
	m, _ := longSession(t, 152, 40)
	foot := func() string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return rows[len(rows)-1]
	}
	if strings.Contains(foot(), "space") {
		t.Errorf("on the trail the row names no space key: %q", foot())
	}
	press(m, " ")
	if m.note != "the counts open · k up to them" {
		t.Errorf("space on the trail says where it acts: %q", m.note)
	}
	climb(t, m)
	if !strings.Contains(foot(), " · space open · ") {
		t.Errorf("on a closed count the row offers `space open`: %q", foot())
	}
	press(m, " ")
	if !strings.Contains(foot(), " · space close · ") {
		t.Errorf("on an open count the row offers `space close`: %q", foot())
	}
	press(m, "j")
	if !strings.Contains(foot(), " · space close · ") {
		t.Errorf("on a leg under it the row offers `space close`: %q", foot())
	}
	press(m, "G")
	if strings.Contains(foot(), "space") {
		t.Errorf("back on the trail the row names no space key: %q", foot())
	}
	// At eighty the fold key outlasts the level's other keys: it is what
	// the row is for while the cursor is in the block.
	m, _ = longSession(t, 80, 24)
	climb(t, m)
	if f := foot(); !strings.Contains(f, "space open") {
		t.Errorf("at eighty the row keeps the fold key: %q", f)
	}
}

func TestTheBoardKeepsItsCountsClosed(t *testing.T) {
	forceASCII(t)
	sc := sceneVeryLong()
	m := sceneModel(sc, 152, 40)
	pressKey(m, "1")
	poll(m, sc)
	// The baseline is a board come back to: entering the session marks
	// it seen, and the column says so.
	pressKey(m, "tab")
	poll(m, sc)
	pressKey(m, "shift+tab")
	board := ansi.Strip(m.View())
	pressKey(m, "tab")
	poll(m, sc)
	climb(t, m)
	press(m, " ")
	if !strings.Contains(strings.Join(trailCells(m), "\n"), "├ ◆ docs") {
		t.Fatalf("the class should be open on the legs")
	}
	pressKey(m, "shift+tab")
	if got := ansi.Strip(m.View()); got != board {
		t.Errorf("the board's columns keep their counts closed:\n%s", got)
	}
	pressKey(m, "tab")
	poll(m, sc)
	if !strings.Contains(strings.Join(trailCells(m), "\n"), "├ ◆ docs") {
		t.Errorf("back on the legs the class is still open: the open set is the session's")
	}
	if m.inBlock() {
		t.Errorf("tab into the legs opens on the present, the cursor the trail's")
	}
}

func TestSwitchingSessionsLeavesTheBlock(t *testing.T) {
	forceASCII(t)
	m, _ := longSession(t, 152, 40)
	climb(t, m)
	was := m.selectedKey
	press(m, "l")
	if m.selectedKey == was {
		t.Fatalf("l should move to the next session")
	}
	if m.inBlock() {
		t.Errorf("another session's legs open with the cursor on the trail")
	}
	if !m.trailPinned {
		t.Errorf("and at the present")
	}
}

func TestTheLanesOpenIntoTheirLanes(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	m := sceneModel(sc, 152, 40)
	for _, d := range []string{"1", "2", "3", "4", "5"} {
		pressKey(m, d)
		poll(m, sc)
		if s, ok := m.selected(); ok && sessionName(s.Info) == "porter" && len(m.trails[m.selectedKey].Branches) == 4 {
			break
		}
	}
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	climb(t, m)
	if row := cursorRow(trailCells(m)); !strings.Contains(row, "agent") || !strings.Contains(row, "4 lanes") {
		t.Fatalf("porter's block ends on its lanes row: %q\n%s", row, strings.Join(trailCells(m), "\n"))
	}
	press(m, " ")
	cells := strings.Join(blockCells(m), "\n")
	// Two of the lanes are silent and wear the stuck mark, as the trail
	// marks them (#49).
	if n := len(regexp.MustCompile(`│  [├└] [◈◍] `).FindAllString(cells, -1)); n != 4 {
		t.Errorf("space on the lanes row hangs its four lanes on the rail, %d:\n%s", n, cells)
	}
	if !strings.Contains(cells, "3 defects found") {
		t.Errorf("a lane back with a finding hangs the finding under it:\n%s", cells)
	}
	press(m, "j")
	press(m, "j")
	press(m, "j")
	rows := m.blockRowsHere()
	if r := rows[m.blockCursor]; r.kind != "lane" {
		t.Fatalf("j under the open lanes walks the lanes, standing on %q", r.kind)
	}
	lane := m.trail.Branches[rows[m.blockCursor].lane]
	press(m, "tab")
	if m.level != levelReader || m.readerLane != lane.ToolUseID {
		t.Errorf("tab on a lane row in the block reads that lane's own conversation: level %d, lane %q, wanted %q", m.level, m.readerLane, lane.ToolUseID)
	}
}
