package ui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

// Round fifty-nine: the summary (#344). `s` on the legs counts them by
// class, `space` opens a class into its legs oldest first, `tab` on a leg
// is the trail with the cursor on it, and `s` or `esc` is the trail again.

// summaryModel is the very-long scene's day-long session on its legs.
func summaryModel(t *testing.T, w, h int) *Model {
	t.Helper()
	sc := sceneVeryLong()
	m := sceneModel(sc, w, h)
	pressKey(m, "1")
	poll(m, sc)
	pressKey(m, "tab")
	poll(m, sc)
	if m.level != levelWaypoints {
		t.Fatalf("tab from the board should land on the legs, not level %d", m.level)
	}
	if len(m.trail.Legs) < 100 {
		t.Fatalf("the scene's long session has %d legs; the summary is for a long one", len(m.trail.Legs))
	}
	return m
}

func classCounts(tr journey.Trail) map[journey.Class]int {
	n := map[journey.Class]int{}
	for _, l := range tr.Legs {
		n[l.Class]++
	}
	return n
}

func TestTheSummaryCountsTheLegsByClass(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		w, h := size[0], size[1]
		m := summaryModel(t, w, h)
		before := ansi.Strip(m.View())
		if strings.Contains(before, "[summary]") {
			t.Fatalf("%dx%d: the legs' frame says summary before s:\n%s", w, h, before)
		}
		press(m, "s")
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "[summary]") {
			t.Fatalf("%dx%d: s on the legs does not open the summary:\n%s", w, h, view)
		}
		if strings.Contains(view, "[session]") || strings.Contains(view, "[legs]") {
			t.Errorf("%dx%d: the summary's frame still wears the legs' word:\n%s", w, h, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if n := ansi.StringWidth(line); n > w {
				t.Errorf("%dx%d: a summary line runs past the terminal (%d): %q", w, h, n, line)
			}
		}
		counts := classCounts(m.trail)
		if len(counts) < 5 {
			t.Fatalf("the scene's long session has %d classes; the summary is for every class", len(counts))
		}
		for c, n := range counts {
			want := pad(c.String(), trailVerbWidth) + " " + plural(n, "leg")
			if !strings.Contains(view, want) {
				t.Errorf("%dx%d: the summary does not count %q:\n%s", w, h, want, view)
			}
		}
		// The classes come in the order work happens: scout before build,
		// build before ship.
		rows := strings.Split(view, "\n")
		at := func(class string) int {
			var c journey.Class
			for k := range counts {
				if k.String() == class {
					c = k
				}
			}
			for i, r := range rows {
				if strings.Contains(r, pad(class, trailVerbWidth)+" "+plural(counts[c], "leg")) {
					return i
				}
			}
			return -1
		}
		if s, b, sh := at("scout"), at("build"), at("ship"); !(s >= 0 && s < b && b < sh) {
			t.Errorf("%dx%d: the classes are out of order (scout %d, build %d, ship %d):\n%s", w, h, s, b, sh, view)
		}
		// Red runs are counted on the test row.
		red := 0
		for _, l := range m.trail.Legs {
			if l.Class == journey.Test && strings.Contains(legBadge(l), "✗") {
				red++
			}
		}
		if red > 0 && !strings.Contains(view, plural(counts[journey.Test], "leg")+" · "+strconv.Itoa(red)+" red") {
			t.Errorf("%dx%d: the test row does not count its %d red runs:\n%s", w, h, red, view)
		}
		// The footer names the fold key and the way back; the trail's
		// scroll clauses are not the summary's.
		foot := rows[len(rows)-1]
		for _, want := range []string{"space open", "s/esc trail"} {
			if !strings.Contains(foot, want) {
				t.Errorf("%dx%d: the summary's footer does not name %q: %q", w, h, want, foot)
			}
		}
		if strings.Contains(view, "↑ ") && strings.Contains(view, " legs  [summary]") {
			t.Errorf("%dx%d: the summary's title counts legs above a fold it has not got:\n%s", w, h, view)
		}
	}
}

func TestTheSummaryOpensAClassIntoItsLegsOldestFirst(t *testing.T) {
	forceASCII(t)
	m := summaryModel(t, 120, 34)
	press(m, "s")
	// The cursor opens on the first class row; space opens it.
	rows := m.summaryRowsHere()
	first := rows[0].class
	pressKey(m, "space")
	view := ansi.Strip(m.View())
	lines := strings.Split(view, "\n")
	var legs []journey.Leg
	for _, l := range m.trail.Legs {
		if l.Class == first {
			legs = append(legs, l)
		}
	}
	if len(legs) < 3 {
		t.Fatalf("the first class has %d legs; the order needs three", len(legs))
	}
	// Each open leg's row is the trail's own row for it — the class verb
	// and the label — hung on the waypoint rail, in the trail's order.
	w, h := m.trailBox()
	o := m.trailOpts(w-trailWayWidth, h)
	seen := -1
	for i, l := range legs {
		label, _ := legLabel(l, o)
		want := pad(first.String(), trailVerbWidth) + " " + label
		found := -1
		for j := seen + 1; j < len(lines); j++ {
			if strings.Contains(lines[j], "├ ") || strings.Contains(lines[j], "└ ") {
				if strings.Contains(lines[j], want) {
					found = j
					break
				}
			}
		}
		if found < 0 {
			if i < 3 {
				t.Fatalf("the %s class's leg %d (%q) is not drawn beneath it in order:\n%s", first, i, want, view)
			}
			break // the rest are below the fold
		}
		seen = found
	}
	if !strings.Contains(lines[len(lines)-1], "space close") {
		t.Errorf("an open class's footer does not offer to close it: %q", lines[len(lines)-1])
	}
	// Space on a leg row folds its class and lands on the class row.
	press(m, "j")
	pressKey(m, "space")
	if rows := m.summaryRowsHere(); rows[m.summaryCursor].kind != "class" || m.summaryOpen[first.String()] {
		t.Errorf("space on a leg should fold its class and stand on it: cursor %d, open %v", m.summaryCursor, m.summaryOpen)
	}
}

func TestTabOnASummaryLegIsTheTrailThere(t *testing.T) {
	forceASCII(t)
	m := summaryModel(t, 120, 34)
	press(m, "s")
	pressKey(m, "space")
	press(m, "j")
	press(m, "j") // the second leg of the first class
	rows := m.summaryRowsHere()
	if rows[m.summaryCursor].kind != "leg" {
		t.Fatalf("the cursor is not on a leg: %+v", rows[m.summaryCursor])
	}
	leg := rows[m.summaryCursor].leg
	pressKey(m, "tab")
	if m.summary || m.level != levelWaypoints {
		t.Fatalf("tab on a leg should leave the summary on the legs: summary %v, level %d", m.summary, m.level)
	}
	trail := TrailRows(m.trail, m.level)
	if m.cursor < 0 || m.cursor >= len(trail) || trail[m.cursor].Kind != "leg" || trail[m.cursor].Leg != leg {
		t.Fatalf("the trail's cursor is not on leg %d: cursor %d", leg, m.cursor)
	}
	view := ansi.Strip(m.View())
	w, h := m.trailBox()
	label, _ := legLabel(m.trail.Legs[leg], m.trailOpts(w, h))
	cursored := ""
	for _, line := range strings.Split(view, "\n") {
		if isCursorRow(line) || strings.Contains(line, "▸") {
			if strings.Contains(line, label) {
				cursored = line
			}
		}
	}
	if cursored == "" {
		t.Errorf("the trail's cursor row does not carry leg %d (%q):\n%s", leg, label, view)
	}
	if strings.Contains(view, "[summary]") {
		t.Errorf("the frame after tab still says summary:\n%s", view)
	}
	// And s is the summary again, esc the trail again.
	press(m, "s")
	if !strings.Contains(ansi.Strip(m.View()), "[summary]") {
		t.Errorf("s after tab does not reopen the summary")
	}
	pressKey(m, "esc")
	if m.summary || m.level != levelWaypoints || strings.Contains(ansi.Strip(m.View()), "[summary]") {
		t.Errorf("esc on the summary should be the legs again: summary %v, level %d", m.summary, m.level)
	}
}

func TestTheSummaryIsRefusedOffTheLegs(t *testing.T) {
	forceASCII(t)
	sc := sceneVeryLong()
	m := sceneModel(sc, 152, 40)
	if m.level != levelBoard {
		t.Fatalf("the scene does not open on the board: level %d", m.level)
	}
	press(m, "s")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "the summary is one trail's · tab into it") || strings.Contains(view, "[summary]") {
		t.Errorf("s on the board should say the way in:\n%s", view)
	}
	narrow := sceneModel(sc, 80, 24)
	pressKey(narrow, "1")
	poll(narrow, sc)
	if narrow.level != levelTrail {
		t.Fatalf("a digit on the narrow deck should land on the trail: level %d", narrow.level)
	}
	press(narrow, "s")
	if view := ansi.Strip(narrow.View()); !strings.Contains(view, "the summary is the legs'") || !strings.Contains(view, "tab deeper") || strings.Contains(view, "[summary]") {
		t.Errorf("s on the trail should say the summary is the legs' and keep `tab deeper` on the row:\n%s", view)
	}
	pressKey(narrow, "tab")
	press(narrow, "s")
	if view := ansi.Strip(narrow.View()); !strings.Contains(view, "[summary]") {
		t.Errorf("s on the narrow deck's legs should open the summary:\n%s", view)
	}
	// Tab on a class row is its legs, tab on a leg the trail there, and
	// the next tab the reader, which keeps its keys.
	pressKey(narrow, "tab")
	if rows := narrow.summaryRowsHere(); !narrow.summary || rows[narrow.summaryCursor].kind != "leg" {
		t.Fatalf("tab on a class row should open it and stand on its first leg: summary %v, cursor %d", narrow.summary, narrow.summaryCursor)
	}
	pressKey(narrow, "tab")
	if narrow.summary || narrow.level != levelWaypoints {
		t.Fatalf("tab on a leg row should be the trail there: summary %v, level %d", narrow.summary, narrow.level)
	}
	pressKey(narrow, "tab")
	if narrow.level != levelReader {
		t.Fatalf("tab on the legs should open the reader: level %d", narrow.level)
	}
	if strings.Contains(ansi.Strip(narrow.View()), "[summary]") {
		t.Errorf("the reader's frame says summary")
	}
	press(narrow, "s")
	if view := ansi.Strip(narrow.View()); !strings.Contains(view, "the summary is the legs'") || !strings.Contains(view, "esc back") || strings.Contains(view, "[summary]") {
		t.Errorf("s in the reader should say the summary is the legs' and keep the reader's `esc back` (#346):\n%s", view)
	}
}

func TestTheSummaryCountsTheLanes(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	m := sceneModel(sc, 120, 34)
	found := false
	for _, d := range []string{"1", "2", "3", "4", "5"} {
		pressKey(m, d)
		poll(m, sc)
		if len(m.trails[m.selectedKey].Branches) >= 3 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("the subagents scene has no session with three lanes")
	}
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	press(m, "s")
	view := ansi.Strip(m.View())
	want := pad(summaryLanes, trailVerbWidth) + " " + plural(len(m.trail.Branches), "lane")
	if !strings.Contains(view, want) {
		t.Fatalf("the summary does not count the lanes (%q):\n%s", want, view)
	}
	toLanes(t, m)
	pressKey(m, "space")
	view = ansi.Strip(m.View())
	seen := -1
	lines := strings.Split(view, "\n")
	w, h := m.trailBox()
	o := m.trailOpts(w-trailWayWidth, h)
	for i, br := range m.trail.Branches {
		name := string([]rune(branchName(br.Label))[:12])
		mark := glyphBranch
		if live, known := o.Agents[br.ToolUseID]; known && !br.Done {
			if _, hung := laneSilence(live, br, o.Now); hung {
				mark = fleet.Glyph(state.Stuck) // the trail's own mark for a lane gone quiet (#345)
			}
		}
		found := -1
		for j := seen + 1; j < len(lines); j++ {
			if strings.Contains(lines[j], mark+" "+name) {
				found = j
				break
			}
		}
		if found < 0 {
			t.Fatalf("lane %d (%q) is not drawn beneath the lanes' row in order:\n%s", i, name, view)
		}
		seen = found
	}
}

func TestTheHelpNamesTheSummaryOnTheLegs(t *testing.T) {
	forceASCII(t)
	// The footer names the key where the legs' row has the room — it is
	// the first key that row gives up, so the page key and the door keep
	// their cells at 152 — and the help names it on the legs at every
	// width, and nowhere else.
	wide := summaryModel(t, 220, 48)
	if foot := ansi.Strip(wide.View()); !strings.Contains(foot, "s summary") {
		t.Errorf("the legs' footer at 220 does not name s:\n%s", foot)
	}
	for _, size := range [][2]int{{80, 24}, {152, 40}} {
		m := summaryModel(t, size[0], size[1])
		press(m, "?")
		if view := ansi.Strip(m.View()); !strings.Contains(view, "summary: the legs by class") {
			t.Errorf("%dx%d: the help on the legs does not explain s:\n%s", size[0], size[1], view)
		}
	}
	board := sceneModel(sceneVeryLong(), 152, 40)
	press(board, "?")
	if view := ansi.Strip(board.View()); strings.Contains(view, "summary: the legs by class") {
		t.Errorf("the board's help explains a key that is refused there:\n%s", view)
	}
}

// The panel's findings on #344, folded (#345).

// summaryFrameRows is the frame's rows, stripped.
func summaryFrameRows(m *Model) []string {
	return strings.Split(ansi.Strip(m.View()), "\n")
}

// summaryTrailCell is the trail column's cells of a frame row, cut by the
// deck's own layout rather than by rules, since a hung row begins with one.
func summaryTrailCell(m *Model, row string) string {
	inner := m.width - 2*edgePad
	fw, mw, tw := m.layout(inner)
	start := edgePad
	if fw > 0 {
		start += fw + gutterWidth
		if mw > 0 && !(m.level >= levelWaypoints && !(m.sessionView() && m.showMirror)) {
			start += mw + gutterWidth
		}
	}
	r := []rune(row)
	if start >= len(r) {
		return ""
	}
	end := min(start+tw, len(r))
	return string(r[start:end])
}

func TestTheClassRowSumsItsSpanToTheMinute(t *testing.T) {
	forceASCII(t)
	m := summaryModel(t, 80, 24)
	press(m, "s")
	view := ansi.Strip(m.View())
	counts := classCounts(m.trail)
	for c, n := range counts {
		if n < 2 {
			continue
		}
		var span time.Duration
		for _, l := range m.trail.Legs {
			if l.Class != c {
				continue
			}
			end := l.End
			if l.Current {
				end = m.now
			}
			span += end.Sub(l.Start)
		}
		want := spanText(span)
		if !strings.Contains(want, "h") || !strings.Contains(want, "m") {
			t.Fatalf("the %s class's span %s is not one the floor would hide", c, want)
		}
		row := ""
		for _, r := range strings.Split(view, "\n") {
			if strings.Contains(r, pad(c.String(), trailVerbWidth)+" "+plural(n, "leg")) {
				row = strings.TrimRight(summaryTrailCell(m, r), " ")
			}
		}
		if row == "" || !strings.HasSuffix(row, " "+want) {
			t.Errorf("the %s row does not carry its span %q to the minute: %q", c, want, row)
		}
	}
	// The test row's badge: the red runs in the card's word, and the loop
	// where the row has the room, shed whole where it has not.
	if !strings.Contains(view, "32 legs · 10 red") || strings.Contains(view, "10✗   ") {
		t.Errorf("the test row does not say `10 red`:\n%s", view)
	}
	wide := summaryModel(t, 220, 48)
	press(wide, "s")
	if v := ansi.Strip(wide.View()); !strings.Contains(v, "32 legs · 10 red · 10th failure") {
		t.Errorf("the test row at 220 does not name the loop:\n%s", v)
	}
}

func TestARunningLegCountsToNowAndItsClassWearsTheHeadGlyph(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	m := sceneModel(sc, 120, 34)
	for _, d := range []string{"1", "2", "3", "4", "5"} {
		pressKey(m, d)
		poll(m, sc)
		if s, ok := m.selected(); ok && sessionName(s.Info) == "porter" {
			break
		}
	}
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	var cur journey.Leg
	for _, l := range m.trail.Legs {
		if l.Current {
			cur = l
		}
	}
	if !cur.Current || cur.Class != journey.Build || classCounts(m.trail)[journey.Build] != 2 {
		t.Fatalf("porter's present is not a second build leg: %+v", cur)
	}
	press(m, "s")
	view := ansi.Strip(m.View())
	var span time.Duration
	for _, l := range m.trail.Legs {
		if l.Class == journey.Build {
			end := l.End
			if l.Current {
				end = m.now
			}
			span += end.Sub(l.Start)
		}
	}
	want := " " + pad("build", trailVerbWidth) + " 2 legs"
	row := ""
	for _, r := range strings.Split(view, "\n") {
		if strings.Contains(r, want) {
			row = strings.TrimRight(summaryTrailCell(m, r), " ")
		}
	}
	if row == "" || strings.HasPrefix(row, glyphLeg) {
		t.Fatalf("the build row does not wear HEAD's glyph over %q: %q\n%s", want, row, view)
	}
	if !strings.HasSuffix(row, " "+spanText(span)) {
		t.Errorf("the build row's span stops at the running leg's recorded end: %q, want %s", row, spanText(span))
	}
}

func TestTheClassRowEndsWhereTheLegRowsEnd(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {120, 34}, {220, 48}} {
		m := summaryModel(t, size[0], size[1])
		press(m, "s")
		pressKey(m, "space")
		press(m, "j")
		rows := summaryFrameRows(m)
		class, leg := "", ""
		for _, r := range rows {
			cell := summaryTrailCell(m, r)
			switch {
			case strings.Contains(cell, "scout  32 legs"):
				class = strings.TrimRight(cell, " ")
			case leg == "" && strings.Contains(cell, "├ ◆ scout"):
				leg = strings.TrimRight(cell, " ")
			}
		}
		if class == "" || leg == "" {
			t.Fatalf("%dx%d: no class row and hung leg row to measure:\n%s", size[0], size[1], strings.Join(rows, "\n"))
		}
		if lipgloss.Width(class) != lipgloss.Width(leg) {
			t.Errorf("%dx%d: the class row's age ends a cell from the leg row's:\n%q\n%q", size[0], size[1], class, leg)
		}
	}
}

func TestSpaceFramesTheClassItOpens(t *testing.T) {
	forceASCII(t)
	m := summaryModel(t, 80, 24)
	press(m, "s")
	for i := 0; i < 6; i++ {
		press(m, "j") // docs, the last class
	}
	pressKey(m, "space")
	rows := summaryFrameRows(m)
	at, legs := -1, 0
	for i, r := range rows {
		cell := summaryTrailCell(m, r)
		if strings.Contains(cell, "docs   16 legs") {
			at = i
		}
		if strings.Contains(cell, "├ ◆ docs") || strings.Contains(cell, "└ ◆ docs") {
			legs++
		}
	}
	if at < 0 || legs < 10 {
		t.Fatalf("space on docs shows the class row at %d with %d of its legs — the group is not framed:\n%s", at, legs, strings.Join(rows, "\n"))
	}
	if !strings.Contains(rows[at-1], "↑ ") {
		t.Errorf("the framed class does not stand right under the edge row: %q", rows[at-1])
	}
	// The edge row names the class it cuts, then the classes after it.
	m = summaryModel(t, 80, 24)
	press(m, "s")
	pressKey(m, "space") // scout, the first class
	rows = summaryFrameRows(m)
	last := ""
	for _, r := range rows {
		if cell := summaryTrailCell(m, r); strings.Contains(cell, "▾ ") {
			last = strings.TrimSpace(cell)
		}
	}
	if !strings.HasPrefix(last, "▾ ") || !strings.Contains(last, "more scout · 6 classes") {
		t.Errorf("the edge row does not name the class it cuts and the classes after: %q", last)
	}
	// Half a page moves the window with the cursor.
	above := func() string {
		for _, r := range summaryFrameRows(m) {
			if cell := summaryTrailCell(m, r); strings.Contains(cell, "↑ ") {
				return strings.TrimSpace(cell)
			}
		}
		return ""
	}
	pressKey(m, "ctrl+d")
	pressKey(m, "ctrl+d")
	n := 0
	fmt.Sscanf(above(), "↑ %d above", &n)
	if n < m.trailHalfPage() {
		t.Errorf("two half pages moved the window to %q, less than a page", above())
	}
}

func TestAClassOfOneIsItsLegsOwnRow(t *testing.T) {
	forceASCII(t)
	m := porterSummary(t, 120, 34)
	view := ansi.Strip(m.View())
	counts := classCounts(m.trail)
	if counts[journey.Scout] != 1 || counts[journey.Test] != 1 || counts[journey.Build] != 2 {
		t.Fatalf("porter is not one scout, two builds and one test: %v", counts)
	}
	if strings.Contains(view, "1 leg") {
		t.Errorf("a class of one counts instead of naming its leg:\n%s", view)
	}
	w, h := m.trailBox()
	for _, l := range m.trail.Legs {
		if counts[l.Class] != 1 {
			continue
		}
		label, _ := legLabel(l, m.trailOpts(w, h))
		if !strings.Contains(view, pad(l.Class.String(), trailVerbWidth)+" "+string([]rune(label)[:min(12, len([]rune(label)))])) {
			t.Errorf("the %s leg's own row is not drawn for its class of one (%q):\n%s", l.Class, label, view)
		}
	}
	if !strings.Contains(view, "20✓") {
		t.Errorf("the test leg's row lost its passes:\n%s", view)
	}
	pressKey(m, "space")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "  one leg") || strings.Contains(v, "tab is the trail there") {
		t.Errorf("space on a class of one should say `one leg` and no more (#346):\n%s", v)
	}
	foot := summaryFrameRows(m)
	if row := foot[len(foot)-1]; strings.Contains(row, "space open") || !strings.Contains(row, "tab trail there") || !strings.Contains(row, "s/esc trail") {
		t.Errorf("the row on a class of one should name `tab trail there` and the way out, not `space open`: %q", strings.TrimSpace(row))
	}
}

// porterSummary is the subagents scene's porter on its summary: one
// scout, two builds, one test, four lanes.
func porterSummary(t *testing.T, w, h int) *Model {
	t.Helper()
	sc := sceneSubagents()
	m := sceneModel(sc, w, h)
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
	press(m, "s")
	if !strings.Contains(ansi.Strip(m.View()), "[summary]") {
		t.Fatalf("porter's legs do not open the summary")
	}
	return m
}

func TestTheArchiveTitleAndRowKeepTheirShapeOverTheSummary(t *testing.T) {
	forceASCII(t)
	sc := sceneSecondDay()
	// An archived trail that counts something: the scene's are one of
	// each, so a second scout leg goes on the one `1` opens.
	m := sceneModel(sc, 120, 34)
	pressKey(m, "tab")
	poll(m, sc)
	pressKey(m, "A")
	poll(m, sc)
	pressKey(m, "1")
	poll(m, sc)
	key := m.selectedKey
	tr := sc.trails[key]
	tr.Legs = append([]journey.Leg{{Class: journey.Scout, Label: "the callers", Start: tr.Legs[0].Start.Add(-10 * time.Minute), End: tr.Legs[0].Start.Add(-4 * time.Minute), Votes: 3}}, tr.Legs...)
	sc.trails[key] = tr
	m = sceneModel(sc, 120, 34)
	pressKey(m, "tab")
	poll(m, sc)
	pressKey(m, "A")
	poll(m, sc)
	pressKey(m, "1")
	poll(m, sc)
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	before := summaryFrameRows(m)
	press(m, "s")
	after := summaryFrameRows(m)
	if !strings.Contains(strings.Join(after, "\n"), "[summary]") {
		t.Fatalf("s did not open the summary on the archived trail:\n%s", strings.Join(after, "\n"))
	}
	title := ""
	for _, r := range after {
		if strings.Contains(r, "TRAIL · ") {
			title = summaryTrailCell(m, r)
		}
	}
	if !strings.Contains(title, "TRAIL · api") || strings.Contains(title, "fix the 401") {
		t.Errorf("the archive's title over the summary names the ask, not the session: %q", title)
	}
	for i := range before {
		if i < len(after) && strings.Contains(before[i], "▸1 ") {
			b, a := strings.Split(before[i], "│")[0], strings.Split(after[i], "│")[0]
			if a != b {
				t.Errorf("the archive's row changed across s:\n%q\n%q", b, a)
			}
		}
	}
	// The hidden live session in the archive: its row carries a relayed
	// ask beside the trail, and must not grow it over the summary.
	fh := sceneFleetHygiene()
	hm := sceneModel(fh, 152, 40)
	pressKey(hm, "tab")
	poll(hm, fh)
	live := hm.selectedKey
	ltr := fh.trails[live]
	if len(ltr.Legs) == 0 {
		t.Fatalf("the session tab opens has no leg")
	}
	ltr.Legs = append([]journey.Leg{{Class: ltr.Legs[0].Class, Label: "the gates", Start: ltr.Legs[0].Start.Add(-10 * time.Minute), End: ltr.Legs[0].Start.Add(-4 * time.Minute), Votes: 3}}, ltr.Legs...)
	fh.trails[live] = ltr
	hm = sceneModel(fh, 152, 40)
	for _, k := range []string{"tab", "x", "A", "1"} {
		pressKey(hm, k)
		poll(hm, fh)
	}
	for hm.level < levelWaypoints {
		pressKey(hm, "tab")
		poll(hm, fh)
	}
	rowBefore := ""
	for _, r := range summaryFrameRows(hm) {
		if strings.Contains(r, "▸1 ") {
			rowBefore = strings.Split(r, "│")[0]
		}
	}
	press(hm, "s")
	if !strings.Contains(ansi.Strip(hm.View()), "[summary]") {
		t.Fatalf("s did not open on the hidden live session:\n%s", ansi.Strip(hm.View()))
	}
	for _, r := range summaryFrameRows(hm) {
		if strings.Contains(r, "▸1 ") {
			if got := strings.Split(r, "│")[0]; got != rowBefore || strings.Contains(got, "relayed") {
				t.Errorf("the archive's row grew the ask over the summary (#345):\n%q\n%q", rowBefore, got)
			}
		}
	}
}

func TestTheLanesRowTalliesWhatTheCardDoesNot(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	find := func(name string, lanes int) *Model {
		m := sceneModel(sc, 120, 34)
		for _, d := range []string{"1", "2", "3", "4", "5"} {
			pressKey(m, d)
			poll(m, sc)
			if s, ok := m.selected(); ok && sessionName(s.Info) == name && len(m.trails[m.selectedKey].Branches) == lanes {
				for m.level < levelWaypoints {
					pressKey(m, "tab")
					poll(m, sc)
				}
				return m
			}
		}
		t.Fatalf("no session %s in the scene", name)
		return nil
	}
	porter := find("porter", 4)
	press(porter, "s")
	if v := ansi.Strip(porter.View()); !strings.Contains(v, "agent  4 lanes") || !strings.Contains(v, "20m out") || strings.Contains(v, "lanes · 2 silent") {
		t.Errorf("porter's lanes row should count the lanes with the oldest's clock and its word, and leave `2 silent 18m` to the card (#346):\n%s", v)
	}
	harness := find("harness", 3)
	press(harness, "s")
	if v := ansi.Strip(harness.View()); !strings.Contains(v, "agent  3 lanes") || !strings.Contains(v, "1h ago") || strings.Contains(v, "lanes · 1 empty") {
		t.Errorf("harness's lanes row should count the lanes with the last return's clock and its word (#346):\n%s", v)
	}
	// The lane list keeps the trail's own link mark.
	toLanes(t, porter)
	pressKey(porter, "space")
	if v := ansi.Strip(porter.View()); !strings.Contains(v, "→1") {
		t.Errorf("the open lane list drops the lane's →1 link:\n%s", v)
	}
}

func TestTheSummaryStandsWhereItWasLeftOnItsOwnSession(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	m := sceneModel(sc, 120, 34)
	for _, d := range []string{"1", "2", "3", "4", "5"} {
		pressKey(m, d)
		poll(m, sc)
		if s, ok := m.selected(); ok && sessionName(s.Info) == "porter" {
			break
		}
	}
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	press(m, "s")
	toLanes(t, m)
	pressKey(m, "space")
	press(m, "j")
	press(m, "j")
	at := m.summaryCursor
	pressKey(m, "tab") // the trail at that lane
	if m.summary {
		t.Fatal("tab on a lane did not leave the summary")
	}
	press(m, "s")
	if m.summaryCursor != at || !m.summaryOpen[summaryLanes] {
		t.Errorf("s back on the same session lost its place: cursor %d (was %d), open %v", m.summaryCursor, at, m.summaryOpen)
	}
	pressKey(m, "esc")
	was := m.selectedKey
	press(m, "l") // another session
	if m.selectedKey == was {
		press(m, "h")
	}
	if m.selectedKey == was {
		t.Fatal("no other session to move to")
	}
	press(m, "s")
	if m.summaryCursor != 0 || len(m.summaryOpen) != 0 {
		t.Errorf("s on another session kept the last one's place: cursor %d, open %v", m.summaryCursor, m.summaryOpen)
	}
}

func TestTheSummaryKeyTakesOnlySpareRoom(t *testing.T) {
	forceASCII(t)
	sc := sceneTwoTools()
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		for _, route := range [][]string{{"1", "tab"}, {"1", "tab", "ctrl+u"}, {"1", "tab", "3"}, {"1", "tab", "m"}, {"1", "tab", "["}} {
			m := sceneModel(sc, size[0], size[1])
			for _, k := range route {
				pressKey(m, k)
				poll(m, sc)
			}
			if m.level != levelWaypoints {
				continue
			}
			rows := summaryFrameRows(m)
			row := rows[len(rows)-1]
			bare := m.footerTraded(m.keymap(), size[0]-2*edgePad)
			if !footerNamesAll(bare, row) {
				t.Errorf("%dx%d after %v: the row with `s summary` on offer lost a key the row without it names:\n with    %q\n without %q", size[0], size[1], route, strings.TrimSpace(row), strings.TrimSpace(bare))
			}
			if size[0] == 220 && m.summaryRefusal() == "" && !strings.Contains(row, "s summary") {
				t.Errorf("%dx%d after %v: the row has the room and does not name s: %q", size[0], size[1], route, strings.TrimSpace(row))
			}
		}
	}
}

// toLanes walks the summary's cursor onto the lanes' row.
func toLanes(t *testing.T, m *Model) {
	t.Helper()
	for i := 0; i < 40; i++ {
		rows := m.summaryRowsHere()
		if rows[m.summaryCursor].kind == "lanes" {
			return
		}
		press(m, "j")
	}
	t.Fatal("no lanes row to stand on")
}

// The panel's second pass on #344 and #345, folded (#346).

func TestATrailOfOneOfEachIsRefused(t *testing.T) {
	forceASCII(t)
	sc := sceneFleetHygiene()
	m := sceneModel(sc, 120, 34)
	pressKey(m, "tab")
	poll(m, sc)
	if m.level != levelWaypoints || !summaryCountsNothing(m.trail) {
		t.Fatalf("the scene's first session is not one of each on its legs: level %d", m.level)
	}
	before := ansi.Strip(m.View())
	press(m, "s")
	view := ansi.Strip(m.View())
	if strings.Contains(view, "[summary]") || !strings.Contains(view, "nothing to count") {
		t.Errorf("s on a trail of one of each should refuse with `nothing to count`:\n%s", view)
	}
	if !strings.Contains(before, "◉") || !strings.Contains(view, "◉") {
		t.Errorf("the refusal took the ask off the frame")
	}
	// And a trail with no leg yet, whose only row is HEAD's from live state.
	first := sceneFirstSession()
	fm := sceneModel(first, 120, 34)
	for fm.level < levelWaypoints {
		pressKey(fm, "tab")
		poll(fm, first)
	}
	press(fm, "s")
	if v := ansi.Strip(fm.View()); !strings.Contains(v, "nothing to count") || strings.Contains(v, "[summary]") {
		t.Errorf("s on a trail with no leg should say `nothing to count`:\n%s", v)
	}
	// The legs' row names no summary key where the key is refused.
	if foot := summaryFrameRows(fm); strings.Contains(foot[len(foot)-1], "s summary") {
		t.Errorf("the row offers a refused key: %q", foot[len(foot)-1])
	}
}

func TestTheTopEdgeNamesWhatItHides(t *testing.T) {
	forceASCII(t)
	m := summaryModel(t, 80, 24)
	press(m, "s")
	pressKey(m, "space") // scout open
	press(m, "G")        // the present: design, under scout's 32 legs
	rows := summaryFrameRows(m)
	top := ""
	for _, r := range rows {
		if cell := summaryTrailCell(m, r); strings.Contains(cell, "↑ ") {
			top = strings.TrimSpace(cell)
		}
	}
	// Hidden above: the scout row and some of its legs — the legs by
	// name, then the classes — and the count is the count.
	sum := m.summaryRowsHere()
	hidden := sum[:m.summaryScroll+1]
	legs := 0
	for _, r := range hidden {
		if r.kind == "leg" && !r.solo {
			legs++
		}
	}
	want := fmt.Sprintf("↑ %d scout · 1 class", legs)
	if top != want {
		t.Errorf("the top edge says %q, want %q", top, want)
	}
	if strings.Contains(strings.Join(rows, "\n"), " above") {
		t.Errorf("the top edge still says `above` with no noun")
	}
}

func TestGOnTheSummaryIsThePresent(t *testing.T) {
	forceASCII(t)
	m := porterSummary(t, 120, 34)
	press(m, "G")
	rows := m.summaryRowsHere()
	at := rows[m.summaryCursor]
	if at.kind != "class" || at.class != journey.Build {
		t.Errorf("G should stand on the class holding HEAD (build), not %+v", at)
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "▸build  2 legs · for ") {
		t.Errorf("the running class's row does not say how long its leg has run:\n%s", v)
	}
}

func TestTheSummaryRowKeepsTheHideKey(t *testing.T) {
	forceASCII(t)
	m := porterSummary(t, 152, 40)
	foot := summaryFrameRows(m)
	if row := foot[len(foot)-1]; !strings.Contains(row, "x hide") {
		t.Errorf("the summary's row does not name x hide where it acts: %q", strings.TrimSpace(row))
	}
}

func TestTheSummaryKeepsTheAskWhereNoReaderStandsBeside(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}} {
		m := summaryModel(t, size[0], size[1])
		press(m, "s")
		rows := summaryFrameRows(m)
		first := ""
		for _, r := range rows {
			if cell := strings.TrimSpace(summaryTrailCell(m, r)); strings.HasPrefix(cell, "◉") {
				first = cell
				break
			}
		}
		if !strings.Contains(first, "◉ 1/") {
			t.Errorf("%dx%d: the summary drops the ask where nothing else on the frame says it:\n%s", size[0], size[1], strings.Join(rows, "\n"))
		}
	}
	wide := summaryModel(t, 120, 34)
	press(wide, "s")
	for _, r := range summaryFrameRows(wide) {
		if cell := strings.TrimSpace(summaryTrailCell(wide, r)); strings.HasPrefix(cell, "◉ 1/") {
			t.Errorf("120x34: the summary repeats the ask the reader beside it carries: %q", cell)
		}
	}
}

func TestTheSummarySaysTheWaitOnceAndOnlyWhereTheTitleDoesNot(t *testing.T) {
	forceASCII(t)
	m := summaryModel(t, 80, 24)
	press(m, "s")
	view := ansi.Strip(m.View())
	if strings.Count(view, "on you") != 1 {
		t.Errorf("80x24: the wait on you is said %d times:\n%s", strings.Count(view, "on you"), view)
	}
	if !strings.Contains(view, "◉ waited on you · 12 prompts") {
		t.Errorf("80x24: the summary does not say what the day's counts leave out:\n%s", view)
	}
	wide := summaryModel(t, 100, 30)
	press(wide, "s")
	if v := ansi.Strip(wide.View()); strings.Count(v, "on you") != 1 {
		t.Errorf("100x30: the wait on you is said %d times:\n%s", strings.Count(v, "on you"), v)
	}
}

func TestASoloShipRowSaysCommitNotTheAsk(t *testing.T) {
	forceASCII(t)
	sc := sceneSecondDay()
	m := sceneModel(sc, 120, 34)
	pressKey(m, "tab")
	poll(m, sc)
	pressKey(m, "A")
	poll(m, sc)
	pressKey(m, "1")
	poll(m, sc)
	key := m.selectedKey
	tr := sc.trails[key]
	tr.Legs = append([]journey.Leg{{Class: journey.Scout, Label: "the callers", Start: tr.Legs[0].Start.Add(-10 * time.Minute), End: tr.Legs[0].Start.Add(-4 * time.Minute), Votes: 3}}, tr.Legs...)
	sc.trails[key] = tr
	m = sceneModel(sc, 120, 34)
	for _, k := range []string{"tab", "A", "1"} {
		pressKey(m, k)
		poll(m, sc)
	}
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	press(m, "s")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "ship   commit") || strings.Contains(view, "ship   fix the 401") {
		t.Errorf("the solo ship row spells the ask where the trail said commit:\n%s", view)
	}
}

func TestTheSummaryHangsTheQuestionUnderHead(t *testing.T) {
	forceASCII(t)
	sc := sceneAlarmStorm()
	m := sceneModel(sc, 152, 40)
	// The session asking: find it, give its trail a second design leg so
	// the summary counts something, and open it.
	var key string
	for _, s := range sc.sessions {
		if s.Live && s.Snap.State == state.NeedsYou && !s.Snap.APIError && key == "" {
			key = s.Info.Key() // the one asking, not one dead on the API
		}
	}
	if key == "" {
		t.Fatal("no session asks in the scene")
	}
	tr := sc.trails[key]
	tr.Legs = append([]journey.Leg{{Class: journey.Scout, Label: "the vpc", Start: tr.Legs[0].Start.Add(-20 * time.Minute), End: tr.Legs[0].Start.Add(-14 * time.Minute), Votes: 3},
		{Class: journey.Scout, Label: "the subnets", Start: tr.Legs[0].Start.Add(-12 * time.Minute), End: tr.Legs[0].Start.Add(-6 * time.Minute), Votes: 3}}, tr.Legs...)
	sc.trails[key] = tr
	m = sceneModel(sc, 152, 40)
	found := false
	for _, d := range []string{"1", "2", "3", "4", "5", "6", "7"} {
		pressKey(m, d)
		poll(m, sc)
		if m.selectedKey == key {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no digit reaches the asking session")
	}
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	press(m, "s")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "[summary]") {
		t.Fatalf("the summary did not open:\n%s", view)
	}
	if !strings.Contains(view, "[office CIDR / keep bastion]") {
		t.Errorf("the question's options are not hung under HEAD's row in the summary (#346):\n%s", view)
	}
	// The cursor steps over the question's lines.
	press(m, "G")
	press(m, "j")
	if rows := m.summaryRowsHere(); rows[m.summaryCursor].kind == "ask" {
		t.Errorf("the cursor stands on a question line")
	}
}

func TestTheRedWordFollowsTheCard(t *testing.T) {
	forceASCII(t)
	sc := sceneManyIdle()
	find := func(w, h int) *Model {
		m := sceneModel(sc, w, h)
		for _, d := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"} {
			pressKey(m, d)
			poll(m, sc)
			if classCounts(m.trails[m.selectedKey])[journey.Test] == 2 {
				for m.level < levelWaypoints {
					pressKey(m, "tab")
					poll(m, sc)
				}
				press(m, "s")
				return m
			}
		}
		t.Fatal("no session with two test legs")
		return nil
	}
	narrow := find(120, 34)
	view := ansi.Strip(narrow.View())
	if !strings.Contains(view, "test   2 legs · 1✗") {
		t.Errorf("120x34: the class row should follow the card's `1✗`:\n%s", view)
	}
	wide := find(152, 40)
	if v := ansi.Strip(wide.View()); !strings.Contains(v, "test   2 legs · 1 red") {
		t.Errorf("152x40: the class row should say `1 red` as the card does:\n%s", v)
	}
}

func TestNoHelpGlossIsWiderThanThePageKeysRow(t *testing.T) {
	widest := 0
	for _, k := range helpKeys {
		if k[0] == "ctrl+d/u" {
			widest = len([]rune(k[1]))
		}
	}
	for _, k := range helpKeys {
		if n := len([]rune(k[1])); n > widest {
			t.Errorf("the %q gloss is %d cells, wider than the page keys' row (%d): it would narrow the keys column at 152 (#345, #346)", k[0], n, widest)
		}
	}
}
