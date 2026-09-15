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
		tw, th := m.trailBox()
		_ = th
		if said := strings.Contains(view, plural(counts[journey.Test], "leg")+" · "+strconv.Itoa(red)+" red"); red > 0 && said == m.summaryRedSaid(tw) {
			t.Errorf("%dx%d: the test row should count its %d red runs where the card does not, and not where it does (#349): said %v\n%s", w, h, red, said, view)
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
	if view := ansi.Strip(narrow.View()); !strings.Contains(view, "the summary is the legs'") || strings.Contains(view, "esc, then s") || !strings.Contains(view, "esc back") || strings.Contains(view, "[summary]") {
		t.Errorf("s in the reader should say the summary is the legs' — the short note — and keep the reader's `esc back` (#346):\n%s", view)
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
	// The loop is said once per frame: on the class row where the card
	// above (or the fleet row beside) does not carry it, and not where
	// it does (#347).
	for _, size := range [][2]int{{80, 24}, {120, 34}, {152, 40}, {220, 48}} {
		m := summaryModel(t, size[0], size[1])
		press(m, "s")
		v := ansi.Strip(m.View())
		row := strings.Contains(v, "32 legs · 10 red · 10th failure")
		elsewhere := strings.Count(v, "10th failure") - map[bool]int{true: 1, false: 0}[row]
		if row == (elsewhere > 0) {
			t.Errorf("%dx%d: the loop is on the class row %v and elsewhere on the frame %d times:\n%s", size[0], size[1], row, elsewhere, v)
		}
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
	if strings.Contains(view, "[summary]") || !strings.Contains(view, "one of each") {
		t.Errorf("s on a trail of one of each should refuse with `one of each` (#348):\n%s", view)
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
	if v := ansi.Strip(fm.View()); !strings.Contains(v, "no leg yet") || strings.Contains(v, "[summary]") {
		t.Errorf("s on a trail with no leg should say `no leg yet` (#347):\n%s", v)
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
		t.Errorf("the running class's row should say how much of its sum is now, `for …` (#349):\n%s", v)
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

func TestTheSummarySaysTheWaitOnceAndToTheMinute(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {220, 48}} {
		m := summaryModel(t, size[0], size[1])
		press(m, "s")
		view := ansi.Strip(m.View())
		want := spanText(promptWaits(m.trail))
		if !strings.Contains(want, "h") || !strings.Contains(want, "m") {
			t.Fatalf("the fixture's wait %s would not show the floor", want)
		}
		if strings.Count(view, "on you") != 1 {
			t.Errorf("%dx%d: the wait on you is said %d times:\n%s", size[0], size[1], strings.Count(view, "on you"), view)
		}
		row := ""
		for _, r := range strings.Split(view, "\n") {
			if strings.Contains(r, "◉ waited on you · 12 prompts") {
				row = strings.TrimRight(summaryTrailCell(m, r), " ")
			}
		}
		if row == "" || !strings.HasSuffix(row, " "+want) {
			t.Errorf("%dx%d: the wait line is missing or floored (want %s): %q\n%s", size[0], size[1], want, row, view)
		}
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

func TestTheRedCountIsSaidOncePerFrame(t *testing.T) {
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
	// Where the card above carries the day's red count, the class row
	// does not; where there is no card, the title stands down and the
	// class row says it (#348).
	for _, size := range [][2]int{{80, 24}, {100, 30}} {
		m := find(size[0], size[1])
		rows := summaryFrameRows(m)
		title, row := "", ""
		for _, r := range rows {
			cell := summaryTrailCell(m, r)
			switch {
			case strings.Contains(cell, "TRAIL · "):
				title = cell
			case strings.Contains(cell, "test   2 legs"):
				row = cell
			}
		}
		if !strings.Contains(row, "2 legs · 1 red") || strings.Contains(title, " red") || strings.Contains(title, "✗") {
			t.Errorf("%dx%d: the red count should be on the class row alone: title %q, row %q", size[0], size[1], title, row)
		}
	}
	for _, size := range [][2]int{{120, 34}, {152, 40}} {
		m := find(size[0], size[1])
		v := ansi.Strip(m.View())
		if strings.Contains(v, "2 legs · 1 red") || strings.Contains(v, "2 legs · 1✗") {
			t.Errorf("%dx%d: the class row repeats the red count the card carries:\n%s", size[0], size[1], v)
		}
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

// The panel's third pass, folded (#347).

func TestAParkedHeadKeepsItsOwnNameInTheSummary(t *testing.T) {
	forceASCII(t)
	m := porterSummary(t, 120, 34)
	press(m, "j") // build, the class holding HEAD
	pressKey(m, "space")
	view := ansi.Strip(m.View())
	if strings.Contains(view, "Measuring DLA") {
		t.Errorf("the summary names the parked lead after the work it gave its lane (#347):\n%s", view)
	}
	if !strings.Contains(view, "● build  implement s6e encoder tests") {
		t.Errorf("HEAD's own row under its class does not carry its own name:\n%s", view)
	}
}

func TestADigitOntoAnotherSessionStartsTheSummaryAfresh(t *testing.T) {
	forceASCII(t)
	m := porterSummary(t, 120, 34)
	press(m, "j")
	pressKey(m, "space") // build open, cursor on it
	// Onto harness, the session with three lanes back: afresh.
	var harness string
	for d := 1; d <= 5; d++ {
		for _, s := range m.sessions {
			if m.digits[s.Info.Key()] == d && sessionName(s.Info) == "harness" && len(m.trails[s.Info.Key()].Branches) == 3 {
				harness = fmt.Sprint(d)
			}
		}
	}
	if harness == "" {
		t.Fatal("no digit for harness")
	}
	pressKey(m, harness)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "[summary]") || m.summaryCursor != 0 || len(m.summaryOpen) != 0 {
		t.Errorf("the digit carried porter's fold onto harness: cursor %d, open %v\n%s", m.summaryCursor, m.summaryOpen, view)
	}
	// Onto cli, one of each: the trail, on this frame, with the reason —
	// and the next key is the trail's.
	var cli string
	for d := 1; d <= 5; d++ {
		for _, s := range m.sessions {
			if m.digits[s.Info.Key()] == d && sessionName(s.Info) == "cli" {
				cli = fmt.Sprint(d)
			}
		}
	}
	if cli == "" {
		t.Fatal("no digit for cli")
	}
	pressKey(m, cli)
	view = ansi.Strip(m.View())
	if strings.Contains(view, "[summary]") || !strings.Contains(view, "one of each") {
		t.Errorf("a digit onto a trail of one of each should draw the trail with the reason on this frame (#347):\n%s", view)
	}
	was := m.cursor
	press(m, "k")
	if m.cursor == was || m.summary {
		t.Errorf("the key after the refusal was eaten: cursor %d (was %d), summary %v", m.cursor, was, m.summary)
	}
}

func TestAReturnedLaneSaysWhatItFound(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	m := sceneModel(sc, 120, 34)
	for _, d := range []string{"1", "2", "3", "4", "5"} {
		pressKey(m, d)
		poll(m, sc)
		if s, ok := m.selected(); ok && sessionName(s.Info) == "harness" && len(m.trails[m.selectedKey].Branches) == 3 {
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
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Seven of nine kickoffs") {
		t.Errorf("a lane back with a finding does not say it in the summary (#347):\n%s", view)
	}
	press(m, "j")
	if rows := m.summaryRowsHere(); rows[m.summaryCursor].kind != "lane" {
		t.Errorf("the cursor stands on a report line: %+v", rows[m.summaryCursor])
	}
}

func TestTheArchiveRoundTripKeepsTheSummary(t *testing.T) {
	forceASCII(t)
	// many-idle: an archive to go to, and a live session (etl, two build
	// legs) whose summary counts.
	sc := sceneManyIdle()
	m := sceneModel(sc, 152, 40)
	found := false
	for _, d := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"} {
		pressKey(m, d)
		poll(m, sc)
		if !summaryCountsNothing(m.trails[m.selectedKey]) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no live session that counts")
	}
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	press(m, "s")
	if !strings.Contains(ansi.Strip(m.View()), "[summary]") {
		t.Fatalf("the live session's summary did not open:\n%s", ansi.Strip(m.View()))
	}
	pressKey(m, "space")
	at := m.summaryCursor
	open := len(m.summaryOpen)
	pressKey(m, "A")
	poll(m, sc)
	if v := ansi.Strip(m.View()); strings.Contains(v, "[summary]") || !m.archiveView {
		t.Fatalf("A did not open the archive list, or the list draws the summary:\n%s", v)
	}
	pressKey(m, "A")
	poll(m, sc)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "[summary]") || m.summaryCursor != at || len(m.summaryOpen) != open {
		t.Errorf("A and back should return to the summary it left (#343, #347): cursor %d (was %d), open %v\n%s", m.summaryCursor, at, m.summaryOpen, v)
	}
}

func TestGPrefersTheRunningLegInsideAnOpenClass(t *testing.T) {
	forceASCII(t)
	m := porterSummary(t, 120, 34)
	press(m, "j")
	pressKey(m, "space")
	press(m, "k")
	press(m, "G")
	rows := m.summaryRowsHere()
	at := rows[m.summaryCursor]
	if at.kind != "leg" || !m.trail.Legs[at.leg].Current {
		t.Errorf("G with the class open should stand on the running leg's own row, not %+v", at)
	}
}

func TestTheSummaryRowNamesTheMirrorAndTheSearch(t *testing.T) {
	forceASCII(t)
	m := porterSummary(t, 220, 48)
	foot := summaryFrameRows(m)
	row := foot[len(foot)-1]
	for _, want := range []string{"m live pane", "/ search", "x hide"} {
		if !strings.Contains(row, want) {
			t.Errorf("the summary's row does not name %q where it acts and has the room: %q", want, strings.TrimSpace(row))
		}
	}
}

func TestTheTitleDoesNotRepeatTheAskRowsAge(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}} {
		m := summaryModel(t, size[0], size[1])
		press(m, "s")
		rows := summaryFrameRows(m)
		title, ask := "", ""
		for _, r := range rows {
			cell := summaryTrailCell(m, r)
			switch {
			case strings.Contains(cell, "TRAIL · "):
				title = cell
			case strings.Contains(cell, "◉ 1/"):
				ask = cell
			}
		}
		age := relAge(m.now, m.trail.Prompts[0].At)
		if !strings.Contains(ask, age+" ago") {
			t.Fatalf("%dx%d: the ask row does not carry the span: %q", size[0], size[1], ask)
		}
		if strings.Contains(title, " "+age+" ") || strings.Contains(title, " "+age+"  ") || strings.Contains(title, "on you") {
			t.Errorf("%dx%d: the title repeats the span the ask row carries, or the wait the summary carries: %q", size[0], size[1], title)
		}
	}
}

func TestTheHungQuestionIsOnTheTrailColumn(t *testing.T) {
	forceASCII(t)
	sc := sceneAlarmStorm()
	m := sceneModel(sc, 152, 40)
	pressKey(m, "1")
	poll(m, sc)
	if s, ok := m.selected(); !ok || sessionName(s.Info) != "infra" {
		t.Fatalf("1 is not infra")
	}
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	press(m, "s")
	rows := summaryFrameRows(m)
	at := -1
	for i, r := range rows {
		if strings.Contains(summaryTrailCell(m, r), "design Open port 22") {
			at = i
		}
	}
	if at < 0 || at+1 >= len(rows) {
		t.Fatalf("no design row in the summary:\n%s", strings.Join(rows, "\n"))
	}
	under := summaryTrailCell(m, rows[at+1])
	if !strings.Contains(under, "└ ") || !strings.Contains(strings.Join([]string{under, summaryTrailCell(m, rows[min(at+2, len(rows)-1)])}, " "), "[office CIDR / keep bastion]") {
		t.Errorf("the question is not hung under HEAD's row on the trail column (#346, #347):\n%s", strings.Join(rows, "\n"))
	}
	press(m, "G")
	press(m, "j")
	if r := m.summaryRowsHere(); !r[m.summaryCursor].stands() {
		t.Errorf("the cursor stands on a question line")
	}
}

func TestTheHiddenLiveRowKeepsItsShapeOverTheSummaryAtEveryWidth(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	for _, size := range [][2]int{{100, 30}, {152, 40}} {
		m := sceneModel(sc, size[0], size[1])
		for _, d := range []string{"1", "2", "3", "4", "5"} {
			pressKey(m, d)
			poll(m, sc)
			if s, ok := m.selected(); ok && sessionName(s.Info) == "porter" && len(m.trails[m.selectedKey].Branches) == 4 {
				break
			}
		}
		for _, k := range []string{"x", "A", "1"} {
			pressKey(m, k)
			poll(m, sc)
		}
		for m.level < levelWaypoints {
			pressKey(m, "tab")
			poll(m, sc)
		}
		row := func() string {
			for _, r := range summaryFrameRows(m) {
				if strings.Contains(r, "▸1 ") {
					return strings.Split(r, "│")[0]
				}
			}
			return ""
		}
		before := row()
		press(m, "s")
		if !strings.Contains(ansi.Strip(m.View()), "[summary]") {
			t.Fatalf("%dx%d: s did not open on the hidden porter", size[0], size[1])
		}
		if after := row(); after != before {
			t.Errorf("%dx%d: the archive's row changed across s:\n%q\n%q", size[0], size[1], before, after)
		}
	}
}

// The panel's fourth pass, folded (#348).

func TestTheSummaryIsSettledInTheArchiveToo(t *testing.T) {
	forceASCII(t)
	sc := sceneManyIdle()
	m := sceneModel(sc, 152, 40)
	found := false
	for _, d := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"} {
		pressKey(m, d)
		poll(m, sc)
		if !summaryCountsNothing(m.trails[m.selectedKey]) {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no live session that counts")
	}
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	press(m, "s")
	for i := 0; i < 8 && !m.summaryRowsHere()[m.summaryCursor].group(); i++ {
		press(m, "j") // onto a class row
	}
	pressKey(m, "space")
	if len(m.summaryOpen) == 0 {
		t.Fatalf("no class opened on etl's summary")
	}
	opened := m.summaryOpen
	pressKey(m, "A")
	poll(m, sc)
	pressKey(m, "tab") // an archived session's legs: one of each
	poll(m, sc)
	if m.level != levelWaypoints || !m.archiveView {
		t.Fatalf("tab in the archive did not open the legs: level %d, archive %v", m.level, m.archiveView)
	}
	if v := ansi.Strip(m.View()); strings.Contains(v, "[summary]") || !strings.Contains(v, "one of each") {
		t.Errorf("the archive's legs draw the summary over a trail of one of each, or say nothing (#348):\n%s", v)
	}
	pressKey(m, "esc")
	poll(m, sc)
	pressKey(m, "A")
	poll(m, sc)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "[summary]") || len(m.summaryOpen) != len(opened) {
		t.Errorf("A back should return to the summary it left, with its class open (#348): open %v (was %v)\n%s", m.summaryOpen, opened, v)
	}
}

// twoToolsWaiting is the two-tools scene on its claude api session: one
// of each leg, and four prompts four minutes apart — a 12m wait on you
// that no single prompt row wears, the summary's only count there, and
// the session whose card wears the verdict (#353).
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

func TestTheSummaryRowNamesTheGrab(t *testing.T) {
	forceASCII(t)
	m, _ := twoToolsWaiting(t, 220, 48) // a fleet with a session waiting on you: g acts
	press(m, "s")
	if !strings.Contains(ansi.Strip(m.View()), "[summary]") {
		t.Fatalf("the summary did not open:\n%s", ansi.Strip(m.View()))
	}
	foot := summaryFrameRows(m)
	if row := foot[len(foot)-1]; !strings.Contains(row, "g grab") {
		t.Errorf("the summary's row does not name g grab where it acts (#348): %q", strings.TrimSpace(row))
	}
}

func TestTheHungQuestionIsCutAtTheRail(t *testing.T) {
	forceASCII(t)
	sc := sceneAlarmStorm()
	m := sceneModel(sc, 80, 24)
	pressKey(m, "1")
	poll(m, sc)
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	press(m, "s")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "keep the bastion?") || strings.Contains(view, "keep the bast…") {
		t.Errorf("the question's line is cut short of the row it hangs on (#348):\n%s", view)
	}
}

func TestTheWaitIsARowOfTheDocument(t *testing.T) {
	forceASCII(t)
	m := summaryModel(t, 100, 30)
	press(m, "s")
	pressKey(m, "space") // scout open: the wait's row is below the fold
	rows := m.summaryRowsHere()
	if rows[len(rows)-1].kind != "wait" {
		t.Fatalf("the wait is not the document's last row: %+v", rows[len(rows)-1])
	}
	view := ansi.Strip(m.View())
	for _, r := range strings.Split(view, "\n") {
		if cell := summaryTrailCell(m, r); strings.Contains(cell, "TRAIL · ") && strings.Contains(cell, "on you") {
			t.Errorf("the title carries the wait while the summary's row does: %q", cell)
		}
	}
	press(m, "G")
	for i := 0; i < 40; i++ {
		press(m, "j")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "◉ waited on you · 12 prompts") {
		t.Errorf("the wait's row cannot be reached at the end of the document:\n%s", v)
	}
	// And a trail of one of each with a wait worth a row is not refused.
	tm, _ := twoToolsWaiting(t, 120, 34)
	press(tm, "s")
	if v := ansi.Strip(tm.View()); !strings.Contains(v, "[summary]") || !strings.Contains(v, "◉ waited on you") {
		t.Errorf("a trail whose only count is the wait on you should open the summary on it (#348):\n%s", v)
	}
}

func TestASilentLaneHangsItsOwnLine(t *testing.T) {
	forceASCII(t)
	m := porterSummary(t, 152, 40)
	toLanes(t, m)
	pressKey(m, "space")
	found := false
	for _, r := range summaryFrameRows(m) {
		if cell := summaryTrailCell(m, r); strings.Contains(cell, "nothing written") {
			found = true // the trail column's own row, not the conversation beside it (#351)
		}
	}
	if !found {
		t.Errorf("a lane gone quiet hangs nothing under it in the summary (#348):\n%s", ansi.Strip(m.View()))
	}
}

// The panel's fifth pass, folded (#349).

func TestTheScenesWalkTheSummarysRefusalsAndLanes(t *testing.T) {
	ends := func(extra []string, tail ...string) bool {
		if len(extra) < len(tail) {
			return false
		}
		for i := range tail {
			if extra[len(extra)-len(tail)+i] != tail[i] {
				return false
			}
		}
		return true
	}
	if !ends(sceneFleetHygiene().extra, "tab", "s", "esc") {
		t.Errorf("fleet-hygiene's walk does not end on the refusal `one of each` (#348)")
	}
	if !ends(sceneFirstSession().extra, "tab", "s") {
		t.Errorf("first-session's walk does not end on the refusal `no leg yet` (#349)")
	}
	if !ends(sceneAlarmStorm().extra, "1", "tab", "s", "esc") {
		t.Errorf("alarm-storm's walk does not end on the asking session's summary (#347)")
	}
	found := false
	for _, k := range sceneSubagents().extra {
		if k == "s" {
			found = true
		}
	}
	if !found {
		t.Errorf("subagents' walk never opens the summary")
	}
}

func TestTheHungLinesEndInsideTheColumn(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		m := porterSummary(t, size[0], size[1])
		toLanes(t, m)
		pressKey(m, "space")
		for _, r := range summaryFrameRows(m) {
			if x := lipgloss.Width(r); x > size[0] {
				t.Errorf("%dx%d: a row runs past the terminal (%d): %q", size[0], size[1], x, r)
			}
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "wrote 40s ago") && !strings.Contains(view, "wrote 4") {
			t.Errorf("%dx%d: the line under a lane still out lost its clock (#349):\n%s", size[0], size[1], view)
		}
	}
}

func TestHeadsTailIsOnHeadsOwnRowNotTheClassRow(t *testing.T) {
	forceASCII(t)
	m := porterSummary(t, 152, 40)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "2 legs · for ") {
		t.Errorf("the closed class row should say how much of its sum is now, `for …`:\n%s", v)
	}
	press(m, "j")
	pressKey(m, "space")
	view := ansi.Strip(m.View())
	if strings.Count(view, "◈3 out 20m") != 2 { // the card, and HEAD's own row under its class
		t.Errorf("HEAD's tail should be on the card and on HEAD's own row, %d times here (#349):\n%s", strings.Count(view, "◈3 out 20m"), view)
	}
	// HEAD's own row wears its lanes' clock, `◈3 out 20m`, not the leg's
	// span: the class row keeps `for 2h`, which is then on no other row
	// (#359). Where HEAD's row does carry the span the class row yields
	// it, pinned on very-long's design class (#352).
	if !strings.Contains(view, "2 legs · for ") {
		t.Errorf("the open class row drops `for …` while HEAD's own row beneath carries a different clock (#359):\n%s", view)
	}
	if strings.Count(view, "for 2h") != 1 {
		t.Errorf("the leg's span should be on the frame once, %d times here:\n%s", strings.Count(view, "for 2h"), view)
	}
}

func TestTheCardStandsDownForTheSummarysRows(t *testing.T) {
	forceASCII(t)
	m := summaryModel(t, 220, 48)
	press(m, "s")
	view := ansi.Strip(m.View())
	if strings.Count(view, "on you") != 1 {
		t.Errorf("the wait on you is said %d times at 220:\n%s", strings.Count(view, "on you"), view)
	}
	ships := 0
	for _, l := range m.trail.Legs {
		if l.Class == journey.Ship {
			ships++
		}
	}
	if strings.Contains(view, plural(ships, "ship")) || strings.Contains(view, fmt.Sprintf("%d⚑", ships)) {
		t.Errorf("the ships are counted twice, on the card and on the ship row (#349):\n%s", view)
	}
	// The red count: on the card where it carries it, and then not on the row.
	red := 0
	for _, l := range m.trail.Legs {
		if strings.Contains(legBadge(l), "✗") {
			red++
		}
	}
	tw, _ := m.trailBox()
	card := ansi.Strip(m.cardSecond(tw))
	onCard := strings.Contains(card, fmt.Sprintf(" %d red", red)) || strings.Contains(card, fmt.Sprintf(" %d✗", red))
	onRow := strings.Contains(view, fmt.Sprintf("32 legs · %d red", red))
	if onCard == onRow {
		t.Errorf("the red count should be said exactly once: card %v, row %v\n%s", onCard, onRow, view)
	}
}

// The panel's fifth pass, fleet-hygiene's late report, folded (#350).

func TestTheHidesNoteStandsAloneWhenTheSummaryCloses(t *testing.T) {
	forceASCII(t)
	sc := sceneManyIdle()
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}} {
		m := sceneModel(sc, size[0], size[1])
		found := false
		for _, d := range []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"} {
			pressKey(m, d)
			poll(m, sc)
			if !summaryCountsNothing(m.trails[m.selectedKey]) {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("no live session that counts")
		}
		for m.level < levelWaypoints {
			pressKey(m, "tab")
			poll(m, sc)
		}
		press(m, "s")
		if !strings.Contains(ansi.Strip(m.View()), "[summary]") {
			t.Fatalf("%dx%d: the summary did not open", size[0], size[1])
		}
		for i := 0; i < 4; i++ {
			pressKey(m, "x") // onto the next, until one of the scene's one-of-each trails
			poll(m, sc)
			if summaryCountsNothing(m.trail) {
				break
			}
		}
		if !summaryCountsNothing(m.trail) {
			t.Fatalf("%dx%d: x never landed on a trail of one of each", size[0], size[1])
		}
		rows := summaryFrameRows(m)
		foot := rows[len(rows)-1]
		if strings.Contains(ansi.Strip(m.View()), "[summary]") {
			t.Fatalf("%dx%d: the summary stayed up over a trail of one of each", size[0], size[1])
		}
		if !strings.Contains(foot, "is hidden") || strings.Contains(foot, "one of each") {
			t.Errorf("%dx%d: the hide's note should stand alone (#350): %q", size[0], size[1], strings.TrimSpace(foot))
		}
		way := strings.Contains(foot, "esc back") || strings.Contains(foot, "esc board") || strings.Contains(foot, "tab deeper") || strings.Contains(foot, "tab reader")
		if !way {
			t.Errorf("%dx%d: the row after the hide names no way on: %q", size[0], size[1], strings.TrimSpace(foot))
		}
	}
}

// The panel's sixth pass, folded (#351).

func TestTheSummaryResumesOnTheNextTrailThatCounts(t *testing.T) {
	forceASCII(t)
	m, _ := twoToolsWaiting(t, 120, 34)
	press(m, "s")
	if !strings.Contains(ansi.Strip(m.View()), "[summary]") {
		t.Fatalf("the summary did not open")
	}
	was := m.selectedKey
	press(m, "h") // the neighbour: one of each
	if v := ansi.Strip(m.View()); strings.Contains(v, "[summary]") || m.selectedKey == was || !strings.Contains(v, "[summary waits]") || strings.Count(v, "summary waits") != 1 {
		t.Fatalf("h did not land on a trail of one of each with the summary suspended and marked once (#353, #354):\n%s", v)
	}
	press(m, "j") // a key later the mark still stands
	if v := ansi.Strip(m.View()); !strings.Contains(v, "[summary waits]") {
		t.Errorf("the hold's mark did not outlive the note (#354):\n%s", v)
	}
	press(m, "l") // back onto the trail that counts
	if v := ansi.Strip(m.View()); !strings.Contains(v, "[summary]") || m.selectedKey != was {
		t.Errorf("the summary should resume on the next trail that counts (#351):\n%s", v)
	}
	// A deliberate close stays closed.
	pressKey(m, "esc")
	press(m, "h")
	press(m, "l")
	if strings.Contains(ansi.Strip(m.View()), "[summary]") {
		t.Errorf("a summary closed by hand came back on its own")
	}
	// And `s` on the suspended trail ends the hold (#354).
	press(m, "s")
	press(m, "h")
	if v := ansi.Strip(m.View()); !strings.Contains(v, "[summary waits]") {
		t.Fatalf("the summary is not held on the neighbour:\n%s", v)
	}
	press(m, "s")
	if v := ansi.Strip(m.View()); strings.Contains(v, "[summary waits]") || !strings.Contains(v, "one of each") {
		t.Errorf("s on a suspended trail should end the hold and say why (#354):\n%s", v)
	}
	press(m, "l")
	if strings.Contains(ansi.Strip(m.View()), "[summary]") {
		t.Errorf("the hold ended by s came back on its own")
	}
}

func TestTheEdgeRowShedsWholeClauses(t *testing.T) {
	forceASCII(t)
	m := summaryModel(t, 80, 24)
	press(m, "s")
	pressKey(m, "space") // scout open: the edge cuts scout, the classes and the wait
	for _, r := range summaryFrameRows(m) {
		cell := summaryTrailCell(m, r)
		if strings.Contains(cell, "▾ ") && strings.Contains(cell, "…") {
			t.Errorf("the edge row is cut inside a clause (#351): %q", strings.TrimSpace(cell))
		}
	}
	wide := summaryModel(t, 120, 34) // scout open cuts the classes and the wait here, with the room for the clause
	press(wide, "s")
	pressKey(wide, "space")
	found := false
	for _, r := range summaryFrameRows(wide) {
		if cell := summaryTrailCell(wide, r); strings.Contains(cell, "▾ ") && strings.Contains(cell, "· the wait") {
			found = true
		}
	}
	if !found {
		t.Errorf("the edge row at 120 does not count the wait it hides (#349):\n%s", ansi.Strip(wide.View()))
	}
}

func TestTheShipsStandDownWhereTheShipRowCountsThem(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}} {
		m := summaryModel(t, size[0], size[1])
		press(m, "s")
		for _, r := range summaryFrameRows(m) {
			cell := summaryTrailCell(m, r)
			if strings.Contains(cell, "TRAIL · ") && (strings.Contains(cell, "⚑") || strings.Contains(cell, " ships")) {
				t.Errorf("%dx%d: the title counts the ships the ship row counts (#349): %q", size[0], size[1], strings.TrimSpace(cell))
			}
		}
		if !strings.Contains(ansi.Strip(m.View()), "ship   16 legs") {
			t.Errorf("%dx%d: the ship row is missing", size[0], size[1])
		}
	}
}

func TestTheCardsWaitClauseStandsDownAtEveryWidth(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		m, _ := twoToolsWaiting(t, size[0], size[1])
		press(m, "s")
		v := ansi.Strip(m.View())
		if !strings.Contains(v, "[summary]") {
			t.Fatalf("%dx%d: the summary did not open", size[0], size[1])
		}
		if strings.Count(v, "on you") != 1 {
			t.Errorf("%dx%d: the wait on you is said %d times (#349, #351):\n%s", size[0], size[1], strings.Count(v, "on you"), v)
		}
	}
}

func TestAFindingIsHungWhole(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	m := sceneModel(sc, 80, 24)
	for _, d := range []string{"1", "2", "3", "4", "5"} {
		pressKey(m, d)
		poll(m, sc)
		if s, ok := m.selected(); ok && sessionName(s.Info) == "harness" && len(m.trails[m.selectedKey].Branches) == 3 {
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
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "should cache") || !strings.Contains(view, "└ it") || strings.Contains(view, "re-ran the same setup; t…") {
		t.Errorf("a finding is cut where the trail spells it whole (#351):\n%s", view)
	}
}

// The panel's seventh pass, folded (#352).

func TestTheOpenClassKeepsItsClockWhereHeadsRowIsBelowTheFold(t *testing.T) {
	forceASCII(t)
	m := summaryModel(t, 80, 24)
	press(m, "s")
	press(m, "j") // design, the running class
	pressKey(m, "space")
	rows := summaryFrameRows(m)
	class, present := "", false
	for _, r := range rows {
		cell := summaryTrailCell(m, r)
		switch {
		case strings.Contains(cell, "design 16 legs"):
			class = strings.TrimSpace(cell)
		case strings.Contains(cell, "● design ") && strings.Contains(cell, "for "):
			present = true
		}
	}
	if class == "" {
		t.Fatalf("no design class row on the frame:\n%s", strings.Join(rows, "\n"))
	}
	if present == strings.Contains(class, "· for ") {
		t.Errorf("the present should be on the frame exactly once — the class row's clause where HEAD's own row is below the fold (#352): row %q, HEAD's row drawn %v\n%s", class, present, strings.Join(rows, "\n"))
	}
	// At 80 the edge's word for the wait fits whole.
	pressKey(m, "space")
	pressKey(m, "space")
	for _, r := range summaryFrameRows(m) {
		if cell := summaryTrailCell(m, r); strings.Contains(cell, "▾ ") && strings.Contains(cell, "…") {
			t.Errorf("the edge row is cut inside a clause: %q", strings.TrimSpace(cell))
		}
	}
}

// The panel's eighth pass, alarm-storm, folded (#355).

func TestTheSummaryRowKeepsTheReplyOverTheAttach(t *testing.T) {
	forceASCII(t)
	sc := sceneAlarmStorm()
	m := sceneModel(sc, 80, 24)
	pressKey(m, "1")
	poll(m, sc)
	for m.level < levelWaypoints {
		pressKey(m, "tab")
		poll(m, sc)
	}
	press(m, "s")
	rows := summaryFrameRows(m)
	foot := rows[len(rows)-1]
	if !strings.Contains(foot, "[summary]") && !strings.Contains(strings.Join(rows, "\n"), "[summary]") {
		t.Fatalf("the summary did not open on infra")
	}
	if !strings.Contains(foot, "r reply") {
		t.Errorf("the summary's row on the asking session sheds `r reply` (#355): %q", strings.TrimSpace(foot))
	}
}

func TestTheEdgeSaysTheWaitLongWhereItFits(t *testing.T) {
	forceASCII(t)
	m := summaryModel(t, 80, 24)
	press(m, "s")
	pressKey(m, "space") // scout open: the long edge does not fit
	short := false
	for _, r := range summaryFrameRows(m) {
		if cell := summaryTrailCell(m, r); strings.Contains(cell, "▾ ") {
			if strings.Contains(cell, "the wait on you") || strings.Contains(cell, "…") {
				t.Errorf("the 80-column edge should take the short form whole: %q", strings.TrimSpace(cell))
			}
			short = strings.Contains(cell, "· the wait")
		}
	}
	if !short {
		t.Errorf("the 80-column edge dropped the wait where the short form fits")
	}
	wide := summaryModel(t, 120, 34)
	press(wide, "s")
	pressKey(wide, "space")
	long := false
	for _, r := range summaryFrameRows(wide) {
		if cell := summaryTrailCell(wide, r); strings.Contains(cell, "▾ ") && strings.Contains(cell, "the wait on you") {
			long = true
		}
	}
	if !long {
		t.Errorf("the 120-column edge should say the wait on you in full (#355)")
	}
}

// The panel's eighth pass, two-tools, folded (#356).

func TestTheClassRowYieldsItsClockToTheCard(t *testing.T) {
	forceASCII(t)
	sc := sceneFewOngoing()
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sc, size[0], size[1])
		found := false
		for _, d := range []string{"1", "2", "3", "4"} {
			pressKey(m, d)
			poll(m, sc)
			if s, ok := m.selected(); ok && sessionName(s.Info) == "webapp" && !summaryCountsNothing(m.trails[m.selectedKey]) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%dx%d: no webapp session that counts", size[0], size[1])
		}
		for m.level < levelWaypoints {
			pressKey(m, "tab")
			poll(m, sc)
		}
		press(m, "s")
		v := ansi.Strip(m.View())
		tw, _ := m.trailBox()
		card := ansi.Strip(m.cardSecond(tw))
		var cur journey.Leg
		for _, l := range m.trail.Legs {
			if l.Current {
				cur = l
			}
		}
		figure := "for " + relAge(m.now, cur.Start)
		onCard := strings.Contains(card, figure)
		onRow := strings.Contains(v, "legs · "+figure)
		if onCard && onRow {
			t.Errorf("%dx%d: the class row repeats the card's `%s` (#356):\n%s", size[0], size[1], figure, v)
		}
		if !onCard && !onRow {
			t.Errorf("%dx%d: the present is on no row: card %q\n%s", size[0], size[1], card, v)
		}
	}
}

func TestTheEdgeShedsWholeClausesWhereNoFormFits(t *testing.T) {
	long := "▾ 18 more scout · 6 classes · the wait on you"
	if got := summaryEdge(long, 60); got != long {
		t.Errorf("an edge that fits was changed: %q", got)
	}
	if got := summaryEdge(long, 40); got != "▾ 18 more scout · 6 classes · the wait" {
		t.Errorf("the short form should stand where the long one cannot: %q", got)
	}
	if got := summaryEdge(long, 30); got != "▾ 18 more scout · 6 classes" || strings.Contains(got, "…") {
		t.Errorf("clauses should shed whole, last first, where no form fits (#351, #356): %q", got)
	}
}

// The panel's ninth pass, folded (#357).

func TestTheLanesRowSaysHowManyCameBackWhereNothingElseDoes(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		m := porterSummary(t, size[0], size[1])
		back, out := 0, 0
		for _, br := range m.trails[m.selectedKey].Branches {
			if br.Done {
				back++
			} else {
				out++
			}
		}
		if back != 1 || out != 3 {
			t.Fatalf("porter should have one lane back and three out, not %d and %d", back, out)
		}
		rows := summaryFrameRows(m)
		v := strings.Join(rows, "\n")
		// Said once on the frame — on the card above where the deck
		// draws one, on the fleet row beside where that row keeps the
		// clause, and on the lanes row where neither does (#357).
		// The clause's own form, `· 1 back`: harness's `↳ 1 back, empty`
		// two rows down is another session's landing, not porter's tally.
		if n := strings.Count(v, "· 1 back"); n != 1 {
			t.Errorf("%dx%d: `· 1 back` is on the frame %d times, not once:\n%s", size[0], size[1], n, v)
		}
		lanes := ""
		for _, r := range rows {
			if cell := summaryTrailCell(m, r); strings.Contains(cell, "agent") && strings.Contains(cell, "lanes") {
				lanes = cell
			}
		}
		if lanes == "" {
			t.Fatalf("%dx%d: no lanes row on the summary:\n%s", size[0], size[1], v)
		}
		onRow := strings.Contains(lanes, "4 lanes · 1 back")
		if onRow == m.summaryBackSaid(m.width) {
			t.Errorf("%dx%d: the lanes row %q carries the clause %v while the frame says it elsewhere %v:\n%s", size[0], size[1], lanes, onRow, !onRow, v)
		}
		if size[0] <= 100 && !onRow {
			t.Errorf("%dx%d: no card is drawn and the fleet row sheds `1 back`; the lanes row must say it: %q", size[0], size[1], lanes)
		}
		if size[0] >= 120 && onRow {
			t.Errorf("%dx%d: the card says `1 back`; the lanes row says it again: %q", size[0], size[1], lanes)
		}
	}
	// A trail whose lanes are all back, or all out, has nothing to add up:
	// the count is the fleet row's `◈3 back` or the lanes row's `20m out`.
	if row := ansi.Strip(summaryLanesRow(journey.Trail{Branches: []journey.Branch{{Done: true}, {Done: true}}}, TrailOpts{}, 60, false)); strings.Contains(row, "back") {
		t.Errorf("all lanes back: the row should not count them: %q", row)
	}
}

func TestVeryLongsWalkEndsOnTheHelpTheSummaryOwesARowTo(t *testing.T) {
	extra := sceneVeryLong().extra
	help := -1
	for i := 1; i < len(extra); i++ {
		if extra[i] == "?" && extra[i-1] == "esc" {
			help = i
		}
	}
	if help < 0 {
		t.Fatalf("very-long's walk should close the summary and open the help, so the `s` gloss is on a shipped frame: %v", extra)
	}
	sc := sceneVeryLong()
	m := sceneModel(sc, 80, 24)
	for i, k := range extra {
		pressKey(m, k)
		poll(m, sc)
		if i == help {
			v := ansi.Strip(m.View())
			if !m.showHelp || !strings.Contains(v, "summary: the legs by class") {
				t.Errorf("the walk's help frame does not draw the `s` row:\n%s", v)
			}
		}
	}
	// And the walk ends on the idle session's summary scrolled off the
	// row `G` goes to, so #361's mark is on a shipped frame (#363).
	v := ansi.Strip(m.View())
	if s, ok := m.selected(); !ok || sessionName(s.Info) != "etl" || !strings.Contains(v, "↓ G  [summary]") {
		t.Errorf("the walk's last frame should be etl's summary wearing `↓ G`:\n%s", v)
	}
}

func TestTheSummaryScrolledOffThePresentSaysSo(t *testing.T) {
	forceASCII(t)
	titleOf := func(m *Model) string {
		for _, r := range summaryFrameRows(m) {
			// The title row, or on the wide deck the card's first row.
			if cell := summaryTrailCell(m, r); strings.Contains(cell, "[summary]") {
				return cell
			}
		}
		return ""
	}
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}} {
		m := summaryModel(t, size[0], size[1])
		press(m, "s")
		if title := titleOf(m); strings.Contains(title, "↓ G") {
			t.Errorf("%dx%d: the summary opens on the present; the title says it is off it: %q", size[0], size[1], title)
		}
		press(m, "space") // scout opens: 22 legs push the running class off the window
		v := ansi.Strip(m.View())
		if strings.Contains(v, "for 39m") {
			t.Fatalf("%dx%d: the present is still on the frame; the pin wants it off:\n%s", size[0], size[1], v)
		}
		if title := titleOf(m); !strings.Contains(title, "↓ G  [summary]") {
			t.Errorf("%dx%d: the running class is off the frame and the title does not say `↓ G` (#357): %q\n%s", size[0], size[1], title, v)
		}
		press(m, "G")
		v = ansi.Strip(m.View())
		if title := titleOf(m); strings.Contains(title, "↓ G") || !strings.Contains(v, "for 39m") {
			t.Errorf("%dx%d: `G` is the present; the title still says `↓ G` or the present is off: %q\n%s", size[0], size[1], title, v)
		}
		press(m, "tab")
		press(m, "s")
		if m.summaryOff {
			t.Errorf("%dx%d: the summary reopened on the present should not wear the mark", size[0], size[1])
		}
	}
	m := summaryModel(t, 220, 48)
	press(m, "s")
	press(m, "space")
	if title := titleOf(m); strings.Contains(title, "↓ G") {
		t.Errorf("220x48 has the room for every row; the title says `↓ G`: %q", title)
	}
	// A trail with nothing running: the present is the row `G` goes to,
	// the last one, and the mark says when the window hides it (#361).
	sc := sceneVeryLong()
	m = sceneModel(sc, 80, 24)
	pressKey(m, "3")
	poll(m, sc)
	pressKey(m, "tab")
	poll(m, sc)
	for _, l := range m.trail.Legs {
		if l.Current {
			t.Fatalf("etl should have no running leg")
		}
	}
	press(m, "s")
	if title := titleOf(m); strings.Contains(title, "↓ G") {
		t.Errorf("the idle summary opens with its last row on the frame; the title says `↓ G`: %q", title)
	}
	press(m, "space") // scout opens, the last row leaves the window
	if title := titleOf(m); !strings.Contains(title, "↓ G  [summary]") {
		t.Errorf("the idle summary's last row is off the window and the title does not say `↓ G` (#361): %q\n%s", title, ansi.Strip(m.View()))
	}
	press(m, "G")
	rows := m.summaryRowsHere()
	last := len(rows) - 1
	for last > 0 && !rows[last].stands() {
		last--
	}
	if title := titleOf(m); strings.Contains(title, "↓ G") || m.summaryCursor != last {
		t.Errorf("`G` on the idle summary is its last standing row (%d, cursor %d); the title still says `↓ G`: %q", last, m.summaryCursor, title)
	}
}

// The panel's ninth pass, two-tools, folded (#358).

func TestTheHelpOwesTheHeldSummaryARow(t *testing.T) {
	forceASCII(t)
	gloss := "the summary waits for the next trail that counts · s ends the hold"
	// At 80 too: the row keeps its place on the short body, where a
	// refused key's row is otherwise the first cut (#360).
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		m, _ := twoToolsWaiting(t, size[0], size[1])
		press(m, "s")
		if !strings.Contains(ansi.Strip(m.View()), "[summary]") {
			t.Fatalf("%dx%d: the summary did not open", size[0], size[1])
		}
		// The neighbour of one of each suspends it; on a narrow deck
		// `h` is not a key, so the digit of the other Claude session.
		if size[0] >= deckWideCols {
			press(m, "h")
		} else {
			press(m, "1")
		}
		if v := ansi.Strip(m.View()); !strings.Contains(v, "[summary waits]") {
			t.Fatalf("%dx%d: the summary is not suspended:\n%s", size[0], size[1], v)
		}
		press(m, "?")
		v := ansi.Strip(m.View())
		if !strings.Contains(v, gloss) {
			t.Errorf("%dx%d: the frame draws `[summary waits]` and the help has no row for it (#358):\n%s", size[0], size[1], v)
		}
		// The row fits the width that draws it, whole, at 80 as well:
		// the walkthrough's guard never opens the help over a hold.
		for _, line := range strings.Split(m.View(), "\n") {
			if x := lipgloss.Width(line); x > size[0] {
				t.Errorf("%dx%d: the held help runs past the terminal (%d): %q", size[0], size[1], x, ansi.Strip(line))
			}
		}
		press(m, "?")
		press(m, "s") // ends the hold
		if v := ansi.Strip(m.View()); strings.Contains(v, "summary waits") {
			t.Fatalf("%dx%d: `s` did not end the hold:\n%s", size[0], size[1], v)
		}
		press(m, "?")
		if v := ansi.Strip(m.View()); strings.Contains(v, "ends the hold") || strings.Contains(v, "summary: the legs by class") {
			t.Errorf("%dx%d: the hold is over and the help still draws a row for `s`, which refuses here (#344):\n%s", size[0], size[1], v)
		}
	}
	// The gloss keeps the keys column: no wider than the page keys' row.
	widest := 0
	for _, k := range helpKeys {
		if k[0] == "ctrl+d/u" {
			widest = len([]rune(k[1]))
		}
	}
	if len([]rune(gloss)) > widest {
		t.Errorf("the held gloss is %d cells, wider than the page keys' row (%d)", len([]rune(gloss)), widest)
	}
}

// The panel's tenth pass, subagents, folded (#362).

func TestTheClassRowAsksHeadsOwnRowForTheSpan(t *testing.T) {
	forceASCII(t)
	m := porterSummary(t, 120, 34)
	var head journey.Leg
	for _, l := range m.trail.Legs {
		if l.Current {
			head = l
		}
	}
	if !head.Current || head.Class != journey.Build {
		t.Fatalf("porter's HEAD should be a running build leg")
	}
	span := "for " + relAge(m.now, head.Start)
	row := func(w int, tail string) string {
		o := m.trailOpts(w, 1)
		o.HeadTail = tail
		return ansi.Strip(summaryClassRow(m.trail, journey.Build, o, true, true, true, false, w))
	}
	// The card above says the span and HEAD's own row has shed it: the
	// card's yield stands whatever HEAD's row wears, closed or open (#356,
	// #364).
	for _, open := range []bool{false, true} {
		o := m.trailOpts(100, 1)
		o.HeadTail = "◈3 out 20m"
		if r := ansi.Strip(summaryClassRow(m.trail, journey.Build, o, true, true, open, true, 100)); strings.Contains(r, "· for ") {
			t.Errorf("the card carries the span (open %v); the class row repeats it: %q", open, r)
		}
	}
	// The lanes' clock alone: the span is on no other row, the class
	// row keeps it (#359).
	if r := row(100, "◈3 out 20m"); !strings.Contains(r, "2 legs · "+span) {
		t.Errorf("HEAD's row wears the lanes' clock alone; the open class row drops the span: %q", r)
	}
	// The lanes' clock and the span: HEAD's own row carries it, the
	// class row yields (#351, #362).
	if r := row(100, "◈3 out 20m · "+span); strings.Contains(r, "· for ") {
		t.Errorf("HEAD's row carries `· %s`; the open class row repeats it: %q", span, r)
	}
	// The same tail on a row too narrow to keep the clause: HEAD's row
	// sheds it, so the class row keeps it.
	narrow := 46
	o := m.trailOpts(narrow-trailWayWidth, 1)
	o.HeadTail = "◈3 out 20m · " + span
	label, narrated := legLabel(head, o)
	if drawn := ansi.Strip(legRow(head, label, narrated, o)); strings.Contains(drawn, span) {
		t.Fatalf("at %d HEAD's row should shed the span, got %q", narrow, drawn)
	}
	if r := row(narrow, "◈3 out 20m · "+span); !strings.Contains(r, span) {
		t.Errorf("HEAD's narrow row sheds the span; the open class row drops it too: %q", r)
	}
}
