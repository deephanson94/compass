package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// On the board a column says its HEAD once: the card's fallback present is
// the trail's HEAD row five rows down, and the card leaves it there (#107).
func TestTheBoardCardSaysHeadOnce(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 152, 40)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "pytest tests/gates") {
		t.Fatalf("porter's column draws no HEAD:\n%s", view)
	}
	n := 0
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(oneSpace(l), "test pytest tests/g") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("porter's column says HEAD %d times, want once:\n%s", n, view)
	}
}

// A column whose trail rows the reply box covers keeps its card's sentence:
// the board must not hide a stuck session's hung call behind a panel (#108, #107).
func TestTheCardKeepsWhatTheReplyPanelCovers(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneAlarmStorm(), 120, 34)
	pressKey(m, "r")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "reply to") {
		t.Fatalf("the reply panel is not up:\n%s", view)
	}
	if !strings.Contains(oneSpace(view), "silent 6m") {
		t.Errorf("the stuck column's hung call is nowhere on the board:\n%s", view)
	}
}

// The row the card gave up (#107) takes the rungs the tag row shed: the
// tool word and its model, the pane staying on the tag row (#112).
func TestTheFreedRowTakesTheModelTheTagShed(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 120, 34)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "opencode · sonnet-4-5") {
		t.Errorf("the api column's model stands on no row:\n%s", view)
	}
	if !strings.Contains(view, "⌁ dev:2.0") {
		t.Errorf("the pane left the tag row:\n%s", view)
	}
}

// The `↳ N new legs` digest yields to the model where the column's own
// divider draws the count (#117).
func TestTheNewLegsDigestYieldsToTheModel(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 120, 34)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "you were here") {
		t.Fatalf("no divider on the board:\n%s", view)
	}
	if !strings.Contains(view, "opus-4-1") {
		t.Errorf("the model stands on no row of the frame:\n%s", view)
	}
}

// At eighty the api row keeps its pane where the rung fits the row exactly:
// the digest's floor is its shortest clause (#118).
func TestTheRowKeepsThePaneThatFitsExactly(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 80, 24)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "claude · ⌁ :1.0") {
		t.Errorf("the pane is on no row for 3 api:\n%s", view)
	}
}

// The header leaves two cells before its chips, their own separator's
// width, so the identity's last clause never abuts them (#121).
func TestTheHeaderLeavesTwoCellsBeforeTheChips(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 80, 24)
	keys := []string{"A"}
	for i := 0; i < 11; i++ {
		keys = append(keys, "j")
	}
	for _, k := range keys {
		pressKey(m, k)
	}
	head := strings.Split(ansi.Strip(m.View()), "\n")[0]
	if strings.Contains(head, "claude ●") || strings.Contains(head, "… ●") {
		t.Errorf("the identity ends one cell from the chips: %q", head)
	}
}

// A column the reply box begins inside is composed at the width the box
// leaves: its rows keep their clocks, and no mark stands over blanks (#126).
func TestTheCoveredColumnKeepsItsClocks(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 120, 34)
	for _, k := range []string{"j", "r"} {
		pressKey(m, k)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "reply to 2") {
		t.Fatalf("the reply box is not up on api:\n%s", view)
	}
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "rewrite the install page") {
			left := strings.SplitN(l, "┌", 2)[0]
			if strings.Contains(left, "…") || !strings.Contains(left, "55m ago") {
				t.Errorf("the covered row lost its clock to a mark over blanks: %q", l)
			}
		}
	}
}

// Where no rung stands above the pane, the `↳ N new legs` digest still
// yields to it: the divider below draws the count either way (#129, #117).
func TestTheNewLegsDigestYieldsToThePaneAlone(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneManyIdle(), 120, 34)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "you were here") {
		t.Fatalf("no divider on the board:\n%s", view)
	}
	for _, l := range strings.Split(view, "\n") {
		first := strings.SplitN(l, "│", 2)[0]
		if strings.Contains(first, "new legs") && strings.Contains(first, "⌁ work:0.0") {
			t.Errorf("the digest says the count the divider draws, over the pane: %q", first)
		}
	}
}

// Both columns called api draw the divider with one leg under it, so both
// leave the count to it: the column #112 hoisted no less than the other (#132).
func TestTheHoistedColumnAlsoYieldsTheCount(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "you were here") {
			t.Fatalf("at %dx%d the board draws no divider:\n%s", size[0], size[1], view)
		}
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "↳ 1 new leg") {
				t.Errorf("at %dx%d the tag row draws the count its divider draws: %q", size[0], size[1], l)
			}
		}
	}
}

// #129 yields only a digest that is the count. A digest whose count comes
// last — "↳ 3 sent since, none back · 1 new leg", the form at 152 where the
// look clause is shed — is not that digest: the divider draws neither of
// its clauses (#135, #38 at 220).
func TestTheDigestKeepsTheLanesItCountsAtOneFiftyTwo(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSubagents(), 152, 40)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "you were here · 2h ago") {
		t.Fatalf("no divider on the board:\n%s", view)
	}
	if !strings.Contains(view, "↳ 3 sent since, none back") {
		t.Errorf("the digest lost the lanes that never came back:\n%s", view)
	}
	if !strings.Contains(view, "↳ 1 back since, empty") {
		t.Errorf("the digest lost the lane that came back empty:\n%s", view)
	}
}

// The trailing look clause is the divider's own words (#85), so a digest of
// the count and the look age says nothing the divider does not (#136, #129).
func TestTheNewLegsDigestYieldsWithItsLookClause(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneManyIdle(), 220, 48)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "you were here · 1h ago") {
		t.Fatalf("no divider on the board:\n%s", view)
	}
	for _, l := range strings.Split(view, "\n") {
		for _, col := range strings.Split(l, "│") {
			if strings.Contains(col, "new legs · looked") && strings.Contains(col, "⌁ ") {
				t.Errorf("the digest says the count and the look the divider draws, over the pane: %q", strings.TrimSpace(col))
			}
		}
	}
}

// A folded list's slack — the rows an entry-whole window could not use —
// goes to the band, not to air over the archive's line (#137, #47, #92).
func TestTheFoldedListsSlackGoesToTheBand(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 80, 24)
	pressKey(m, "r")
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	last := -1
	for i, l := range rows {
		if strings.Contains(strings.SplitN(l, "│", 2)[0], "41 archived") {
			last = i
		}
	}
	if last < 2 {
		t.Fatalf("no archive line on the fleet column:\n%s", strings.Join(rows, "\n"))
	}
	air := 0
	for i := last - 1; i >= 0 && strings.TrimSpace(strings.SplitN(rows[i], "│", 2)[0]) == ""; i-- {
		air++
	}
	if air >= 2 {
		t.Errorf("%d blank fleet rows over %q, with sessions behind A", air, strings.TrimSpace(strings.SplitN(rows[last], "│", 2)[0]))
	}
}

// Past the reply box no lone clock stands with nothing left to time (#139, #56).
func TestTheBoxLeavesNoLoneClockBesideIt(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 152, 40)
	pressKey(m, "r")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "reply to 1") {
		t.Fatalf("the reply box is not up:\n%s", view)
	}
	for _, l := range strings.Split(view, "\n") {
		if i := strings.LastIndex(l, "┐"); i >= 0 {
			tail := strings.TrimSpace(strings.ReplaceAll(l[i+len("┐"):], "…", " "))
			if tail != "" && !strings.Contains(tail, " ") && strings.ContainsAny(tail, "0123456789") {
				t.Errorf("the box leaves a lone clock with nothing to time: %q past the box", tail)
			}
		}
	}
}

// The look clause is the divider's own words whether or not what is left of
// the digest is the count alone (#142, #136, #85).
func TestTheDigestDropsTheLookItsDividerDrawsAnyway(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneManyIdle(), 220, 48)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "you were here · 1h ago") {
		t.Fatalf("no divider on the board:\n%s", view)
	}
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "· looked 1h ago") {
			t.Errorf("the digest says the look its own divider draws: %q", strings.TrimSpace(l))
			break
		}
	}
	if !strings.Contains(view, "↳ 1 red") || !strings.Contains(view, "↳ 1 ship") {
		t.Errorf("a clause the divider does not draw left the row:\n%s", view)
	}
	// And the selected list row at a hundred, the fold's other half (#151).
	n := sceneModel(sceneManyIdle(), 100, 30)
	pressKey(n, "j")
	list := ansi.Strip(n.View())
	if strings.Contains(list, "new leg · 1 red") {
		t.Errorf("the selected row says the count its divider draws beside its other clause:\n%s", list)
	}
}

// The count yields before the rungs are hoisted, so both columns called api
// name tool, model and pane on one row (#143, #132, #112).
func TestTheHoistLeavesTheIdentityOnOneRow(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "you were here") {
			t.Fatalf("at %dx%d the board draws no divider:\n%s", size[0], size[1], view)
		}
		together := false
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "opencode · sonnet-4-5 · ⌁ dev:2.0") && strings.Contains(l, "claude · opus-4-1 · ⌁ dev:1.0") {
				together = true
			}
		}
		if !together {
			t.Errorf("at %dx%d the two api columns do not name tool, model and pane on one row:\n%s", size[0], size[1], view)
		}
	}
}

// The rows the board leaves blank belong to sessions, not to air (#43, #47):
// where every live session already has a column no further column can fill
// them, and the same band the list draws below the board's width takes
// them (#147).
func TestTheBoardsStrandedRowsTakeTheBand(t *testing.T) {
	forceASCII(t)
	list := ansi.Strip(sceneModel(sceneFleetHygiene(), 100, 30).View())
	if !strings.Contains(list, "5 ○ api") || !strings.Contains(list, "9 ○ notebooks") {
		t.Fatalf("the list at a hundred does not draw the band:\n%s", list)
	}
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		view := ansi.Strip(sceneModel(sceneFleetHygiene(), size[0], size[1]).View())
		if !strings.Contains(view, "archived · A browses") {
			t.Fatalf("at %dx%d not the board over its archive strip:\n%s", size[0], size[1], view)
		}
		if !strings.Contains(view, "5 ○ api") {
			t.Errorf("at %dx%d the board spends its blank rows on air and names none of the finished sessions the list names at a hundred:\n%s", size[0], size[1], view)
		}
	}
}

// A scrolled fleet column never opens on a row of air: the escape out of a
// cut entry is checked by the same guard that forbids it (#148).
func TestAScrolledColumnNeverOpensOnAir(t *testing.T) {
	forceASCII(t)
	sc := sceneFleetHygiene()
	m := sceneModel(sc, 80, 24)
	for _, k := range canonicalKeys[:25] { // the walkthrough to its scrolled list
		pressKey(m, k)
		poll(m, sc)
	}
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i, l := range lines {
		col := strings.SplitN(l, "│", 2)[0]
		if strings.Contains(col, "more above") && i+1 < len(lines) {
			if strings.TrimSpace(strings.SplitN(lines[i+1], "│", 2)[0]) == "" {
				t.Errorf("the scrolled column opens on air under %q", strings.TrimSpace(col))
			}
			return
		}
	}
	t.Fatalf("the list is not scrolled:\n%s", strings.Join(lines, "\n"))
}

// The note leaves the bytes to a row that says more: the row's right-aligned
// clause may be the tool word a two-tool fleet gives it, not a pane, and it
// may carry two trailing clauses — the compare takes both off (#149, #131).
func TestTheNoteLeavesTheBytesToARowThatSaysMore(t *testing.T) {
	forceASCII(t)
	for _, run := range []struct {
		sc   scene
		w, h int
	}{{sceneTwoTools(), 80, 24}, {sceneTwoTools(), 220, 48}, {sceneVeryLong(), 220, 48}} { // very-long's row carries two trailing clauses (#149)
		size := [2]int{run.w, run.h}
		m := sceneModel(run.sc, run.w, run.h)
		for _, k := range []string{"j", "r", "t", "go on", "enter"} { // api, a typed reply, sent
			pressKey(m, k)
		}
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		foot, body := "", false
		for i, l := range lines {
			if strings.Contains(l, "? help · q quit") {
				foot = l
				continue
			}
			if i < len(lines)-3 && strings.Contains(l, `sent "go on"`) {
				body = true
			}
		}
		if !body {
			t.Fatalf("at %dx%d no row draws the bytes:\n%s", size[0], size[1], strings.Join(lines, "\n"))
		}
		if strings.Contains(foot, `"go on"`) {
			t.Errorf("at %dx%d the note says the bytes a row above draws: %q", size[0], size[1], strings.TrimSpace(foot))
		}
	}
}

// The card's third row keeps its trace and leaves the count and the look
// to its own read-line six rows below (#150, #144, #142).
func TestTheCardsCountGoesBesideItsTrace(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		for _, k := range []string{"j", "r", "t", "go on", "enter", "tab"} { // api, a typed reply, sent, then its card
			pressKey(m, k)
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "sent \"go on\"") || !strings.Contains(view, "you were here · 25m ago") {
			t.Fatalf("at %dx%d not the sent card over its own divider:\n%s", size[0], size[1], view)
		}
		for _, l := range strings.Split(view, "\n") {
			left := strings.SplitN(l, "│", 2)[0]
			if !strings.Contains(left, "sent \"go on\"") {
				continue
			}
			if strings.Contains(left, "new leg") {
				t.Errorf("at %dx%d the card's trace row draws the count its own read-line draws: %q", size[0], size[1], strings.TrimSpace(left))
			}
			if strings.Contains(left, "looked 25m ago") {
				t.Errorf("at %dx%d the card's trace row draws the look its own read-line draws: %q", size[0], size[1], strings.TrimSpace(left))
			}
		}
	}
}

// On the board the count clause goes whether or not it stands alone — the
// divider's own words (#151, #129, #142); the clauses the divider does not
// draw stay, the `↳` moving on to them.
func TestTheBoardsCountGoesWithOtherClauses(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneManyIdle(), 220, 48)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "you were here") {
		t.Fatalf("no divider on the board:\n%s", view)
	}
	if strings.Contains(view, "new leg · 1 red") || strings.Contains(view, "new legs · 1 ship") {
		t.Errorf("a column's tag row says the count its divider draws beside its other clauses:\n%s", view)
	}
	if !strings.Contains(view, "↳ 1 red") || !strings.Contains(view, "↳ 1 ship") {
		t.Errorf("a clause the divider does not draw left the row:\n%s", view)
	}
}

// The board's band is a column too: where the reply box begins inside it,
// it is composed at the width the box leaves, so no row is split by the box
// and no verdict stands past it with no session (#154, #126, #147).
func TestTheBoardsBandIsComposedAtTheWidthTheBoxLeaves(t *testing.T) {
	forceASCII(t)
	peek := regexp.MustCompile(`[│└][^│└]*[│┘]\s+…\s+(?:[✓✗]|build)`)
	band := regexp.MustCompile(`^\s+[1-9] ○ [a-z-]+ · "`)
	clock := regexp.MustCompile(`\d+[smhd]\s*$`)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneFleetHygiene(), size[0], size[1])
		for _, k := range []string{"j", "r"} { // the second harness, its reply box
			pressKey(m, k)
		}
		for i, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			if peek.MatchString(l) {
				t.Errorf("at %dx%d row %d says a verdict past the reply box and no session: %q", size[0], size[1], i+1, strings.TrimSpace(l))
			}
			left := strings.SplitN(strings.SplitN(l, "│", 2)[0], "└", 2)[0]
			if band.MatchString(left) && !clock.MatchString(strings.TrimRight(left, " ")) {
				t.Errorf("at %dx%d the band row beside the box keeps neither verdict nor clock: %q", size[0], size[1], strings.TrimRight(left, " "))
			}
		}
	}
}

// The session card's tag row goes where every clause of it stands in the
// header two rows up; the trail and the reader each take a cell (#155, #100).
func TestTheCardsTagLeavesItsWordsToTheHeader(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		for _, k := range []string{"j", "tab"} { // api, then its card
			pressKey(m, k)
		}
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		if len(lines) < 8 || !strings.Contains(lines[0], "opencode") {
			t.Fatalf("at %dx%d not the api card:\n%s", size[0], size[1], strings.Join(lines, "\n"))
		}
		head := lines[0]
		for _, l := range lines[3:8] {
			row := strings.TrimSpace(strings.SplitN(l, "│", 2)[0])
			if row == "" || strings.Contains(row, "  ") || !strings.Contains(row, mirrorMark) {
				continue
			}
			said := true
			for _, c := range strings.Split(row, " · ") {
				if !strings.Contains(head, c) {
					said = false
					break
				}
			}
			if said {
				t.Errorf("at %dx%d the card's tag row says only what the header says: %q", size[0], size[1], row)
			}
		}
	}
}

// A hidden session does not cost the board its band: the hidden count is
// the band's own header's clause (#157, #147, #86).
func TestAHiddenSessionDoesNotCostTheBoardItsBand(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneFleetHygiene(), size[0], size[1])
		pressKey(m, "x")
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "hidden") {
			t.Fatalf("at %dx%d nothing was hidden:\n%s", size[0], size[1], view)
		}
		for _, want := range []string{"5 ○ api", "9 ○ notebooks"} {
			if !strings.Contains(view, want) {
				t.Errorf("at %dx%d a hide sent the board's rows back to air: %q is on no row:\n%s", size[0], size[1], want, view)
			}
		}
	}
}

// The sliver a covering reply box leaves of a board column is not a lone
// clock: the peek's first segment is tested on its own (#158, #139).
func TestACoveredColumnsSliverIsNotALoneClock(t *testing.T) {
	forceASCII(t)
	lone := regexp.MustCompile(`[│┘└]\s*…\s*(\d+[smhd]|[a-z]{2,5})\s*│`)
	for _, size := range [][2]int{{120, 34}, {220, 48}} {
		m := sceneModel(sceneManyIdle(), size[0], size[1])
		pressKey(m, "r")
		for i, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			if g := lone.FindStringSubmatch(l); g != nil {
				t.Errorf("at %dx%d row %d: the covered column's sliver says %q and nothing it belongs to: %q", size[0], size[1], i+1, g[1], strings.TrimSpace(l))
			}
		}
	}
}

// The zoom-out refusal keeps the way in: `nothing to zoom out to` leaves
// the eighty-column footer `enter attach` and `tab deeper` (#159, #152, #156).
func TestTheZoomOutRefusalKeepsTheWayIn(t *testing.T) {
	forceASCII(t)
	for _, sc := range []struct {
		name string
		m    func() *Model
	}{
		{"second-day", func() *Model { return sceneModel(sceneSecondDay(), 80, 24) }},
		{"first-session", func() *Model { return sceneModel(sceneFirstSession(), 80, 24) }},
	} {
		m := sc.m()
		pressKey(m, "shift+tab")
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "nothing to zoom out to") {
			t.Fatalf("%s: not the refusal frame:\n%s", sc.name, view)
		}
		foot := ""
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "? help · q quit") {
				foot = l
			}
		}
		if !strings.Contains(foot, "enter attach") || !strings.Contains(foot, "tab deeper") {
			t.Errorf("%s: the refusal cost the footer the way in: %q", sc.name, strings.TrimSpace(foot))
		}
	}
}

// A search the live fleet does not answer keeps the board's band: the
// finished sessions that answer it are named under a note that says the
// miss is the live board's (#163, #98, #147).
func TestTheBoardsMissKeepsTheBand(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneFleetHygiene(), size[0], size[1])
		for _, k := range []string{"/", "reconcile"} {
			pressKey(m, k)
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "/reconcile") {
			t.Fatalf("at %dx%d not the search frame:\n%s", size[0], size[1], view)
		}
		if !strings.Contains(view, "no live session matches /reconcile") {
			t.Errorf("at %dx%d the miss is the live board's and does not say so:\n%s", size[0], size[1], view)
		}
		if !strings.Contains(view, `"reconcile the state file"`) {
			t.Errorf("at %dx%d the board blanks on a search eight finished sessions answer:\n%s", size[0], size[1], view)
		}
	}
}

// #159 on the wide deck too: the zoom-out refusal from a board that fits
// keeps `enter attach` and `a ask` (#159).
func TestTheZoomOutRefusalKeepsTheWayInOnTheWideDeck(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneSecondDay(), size[0], size[1])
		for _, k := range []string{"tab", "tab", "shift+tab", "shift+tab", "shift+tab"} {
			pressKey(m, k)
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "nothing to zoom out to") {
			t.Fatalf("%dx%d: not the refusal frame:\n%s", size[0], size[1], view)
		}
		foot := ""
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "? help · q quit") {
				foot = l
			}
		}
		for _, k := range []string{"enter attach", "a ask"} {
			if !strings.Contains(foot, k) {
				t.Errorf("%dx%d: the refusal cost the footer %q: %q", size[0], size[1], k, strings.TrimSpace(foot))
			}
		}
	}
}

// The card's tag goes even beside another clause: a row that carries a
// trace and the tag the header says keeps the trace alone (#167, #155).
func TestTheCardsTagGoesEvenBesideAnotherClause(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		for _, k := range []string{"3", "tab"} {
			pressKey(m, k)
		}
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		head := lines[0]
		for i, l := range lines {
			if !strings.Contains(l, "[session]") || i+1 >= len(lines) {
				continue
			}
			left := strings.TrimRight(strings.Split(lines[i+1], "│")[0], " ")
			j := strings.LastIndex(left, "  ")
			if j <= 0 {
				continue
			}
			tag := strings.TrimSpace(left[j:])
			if tag == "" || !strings.Contains(tag, " · ") {
				continue
			}
			all := true
			for _, c := range strings.Split(tag, " · ") {
				if c == "" || !strings.Contains(head, c) {
					all = false
				}
			}
			if all {
				t.Errorf("%dx%d: the card's tag %q repeats the header %q", size[0], size[1], tag, strings.TrimSpace(head))
			}
		}
	}
}

// ---- round 69, fleet-hygiene ----
// No row of the band wears a bare mark beside a row that wears a word.
// The mark cannot say "shipped" — ✓ is the tick a merely green row wears —
// so the one row with that news stood wordless between two reading
// "✓ green" (#161, on the band; #57's device, band-wide).
func TestTheBandsShippedRowKeepsItsWord(t *testing.T) {
	row := regexp.MustCompile(`^\s*[1-9] ○ [a-z0-9-]+ · .*?\s\s+(\S.*?)\s\d+[smhd]$`)
	m := sceneModel(sceneFleetHygiene(), 152, 40)
	pressKey(m, "j")
	pressKey(m, "r") // the reply box; the band is composed at the width it leaves
	var bare, worded []string
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		for _, seg := range strings.Split(l, "│") {
			g := row.FindStringSubmatch(strings.TrimRight(seg, " "))
			if g == nil {
				continue
			}
			if v := g[1]; v == "✓" || v == "✗" || v == "⚑" {
				bare = append(bare, strings.TrimSpace(seg))
			} else if strings.ContainsAny(v[:3], "✓✗⚑") && len(strings.Fields(v)) > 1 {
				worded = append(worded, strings.TrimSpace(seg))
			}
		}
	}
	if len(worded) == 0 {
		t.Fatalf("no worded band row on the frame:\n%s", ansi.Strip(m.View()))
	}
	for _, b := range bare {
		t.Errorf("the band draws a bare mark beside %d worded rows: %q (worded: %q)", len(worded), b, worded[0])
	}
}

// ---- round 71, two-tools ----
// The board's `m` note says the flag flipped and nothing the frame already
// says. `mirror on · beside a session (tab)` is 34 cells against the twelve
// -cell floor: `beside a session` is the help's own `m` row and `(tab)` is
// the very footer's `tab session`, so every cell over the floor is a second
// copy — and it cost the 120 footer `/ search` and `g grab` and the 152
// footer `enter attach (prefix d returns)` (#166, #57, #165).
func TestTheBoardsMirrorNoteKeepsTheBoardsKeys(t *testing.T) {
	forceASCII(t)
	want := map[int][]string{
		120: {"/ search", "g grab"},
		152: {"(prefix d returns)"},
	}
	for _, size := range [][2]int{{120, 34}, {152, 40}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		pressKey(m, "m")
		view := ansi.Strip(m.View())
		foot := ""
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "? help · q quit") {
				foot = l
			}
		}
		if !strings.Contains(foot, "mirror on") {
			t.Fatalf("%dx%d: not the board's mirror note: %q", size[0], size[1], strings.TrimSpace(foot))
		}
		if !strings.Contains(foot, "tab session") {
			t.Errorf("%dx%d: the note's own row does not name the way to the mirror: %q",
				size[0], size[1], strings.TrimSpace(foot))
		}
		for _, k := range want[size[0]] {
			if !strings.Contains(foot, k) {
				t.Errorf("%dx%d: the board's mirror note sheds %q: %q",
					size[0], size[1], k, strings.TrimSpace(foot))
			}
		}
	}
}

// ---- round 72, fleet-hygiene ----
// The board's miss keeps the doors beside it. A search nothing on the
// board answers drew `no session matches /eda · esc clears it` and
// nothing else — no hidden count, no archive door, no `A` — while the
// same fleet and the same search twenty columns narrower drew
// `0 of 41 archived · 1 of 1 hidden · A` from the list's own tail. On
// that frame a live session did answer the search: the hidden one, whose
// only way back is the `A` the frame does not name (#168, #169, #176, #178).
func TestTheBoardsMissKeepsTheDoorsBesideIt(t *testing.T) {
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneFleetHygiene(), size[0], size[1])
		for _, s := range m.sessions {
			if s.Live && sessionName(s.Info) == "notebooks" {
				m.point(s.Info.Key())
			}
		}
		pressKey(m, "x") // the eda session leaves the board
		pressKey(m, "/")
		for _, r := range "eda" {
			pressKey(m, string(r))
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "no session matches /eda") {
			t.Fatalf("%dx%d: not the board's miss:\n%s", size[0], size[1], view)
		}
		if !strings.Contains(view, "hidden") {
			t.Errorf("%dx%d: the board's miss names no hidden session, over a hidden one the search matched:\n%s", size[0], size[1], view)
		}
		if !strings.Contains(view, "archived") {
			t.Errorf("%dx%d: the board's miss names no archive door:\n%s", size[0], size[1], view)
		}
	}
}

// ---- round 72, fleet-hygiene, second finding ----
// The fleet's search counts what the board can draw. `· /p · 3 of 4`
// stood over three columns that all answered the search and a strip
// saying `1 of 1 hidden` — four of four answered, three were drawn, and
// the fourth is a row the numerator can never reach (#164, #176, #178).
func TestTheFleetsSearchCountsWhatTheBoardCanDraw(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneFleetHygiene(), size[0], size[1])
		for _, s := range m.sessions {
			if s.Live && sessionName(s.Info) == "notebooks" {
				m.point(s.Info.Key())
			}
		}
		pressKey(m, "x")
		pressKey(m, "/")
		pressKey(m, "p")
		head := ansi.Strip(strings.SplitN(m.View(), "\n", 2)[0])
		if !strings.Contains(head, "/p · 3 of 3") {
			t.Errorf("%dx%d: the search's clause counts a row the list cannot hold: %q", size[0], size[1], strings.TrimSpace(head))
		}
	}
}

// ---- round 73, two-tools ----
// The mirror's undo is named on every frame the mirror owns, not only on the
// frame `m` was pressed on. #62 holds `m` back from the shed while its own
// note stands; one keypress later the note is gone, and at 120 columns `m`
// went with it — so the session view drew the pane's own panel
// ("⌁ dev:1.0 · the transcript, until the pane is captured") under a footer
// naming no way back to the conversation, while the same frame at 152 and 220
// named `m conversation`. The key the mirror replaced the panel with outlasts
// `h/l session`, and the row keeps `enter attach` and `a ask` (#168).
func TestTheMirrorsUndoIsNamedWhileTheMirrorStands(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		sc := sceneTwoTools()
		m := sceneModel(sc, size[0], size[1])
		// The mirror is turned on from the board, then a session is opened:
		// two keypresses later the note is gone and the pane is the panel.
		for _, k := range []string{"m", "2", "tab"} {
			pressKey(m, k)
			poll(m, sc)
		}
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		panel, foot := false, ""
		for _, l := range rows {
			if strings.Contains(l, "the transcript, until the pane is captured") {
				panel = true
			}
			if strings.Contains(l, "? help · q quit") {
				foot = strings.TrimRight(l, " ")
			}
		}
		if !panel {
			t.Fatalf("%dx%d: the mirror does not own the panel here", size[0], size[1])
		}
		if !strings.Contains(foot, " · m conversation") {
			t.Errorf("%dx%d: the mirror stands and no key undoes it: %q", size[0], size[1], strings.TrimSpace(foot))
		}
		for _, key := range []string{" · enter attach", " · a ask"} {
			if !strings.Contains(foot, key) {
				t.Errorf("%dx%d: naming `m` cost the row %q: %q", size[0], size[1], strings.TrimSpace(key), strings.TrimSpace(foot))
			}
		}
	}
}

// ---- round 74, fleet-hygiene, the one thing ----
// A width refused keeps the way deeper. `no board under 110 columns` (26
// cells) and `mirror needs 110 columns` (24) stand against the 22 the
// eighty-column fleet-list footer leaves beside `tab deeper`, so both
// cost that footer the frame's only naming of the way deeper — the harm
// #175 and #187 each folded, on the very form #165 held up as the one
// that "names the key and keeps the way in". The unit is the one word
// the person's own terminal supplies; the number and the key stay.
func TestTheWidthRefusalKeepsTheWayDeeper(t *testing.T) {
	for _, c := range []struct {
		name string
		sc   scene
	}{
		{"fleet-hygiene", sceneFleetHygiene()},
		{"many-idle", sceneManyIdle()},
	} {
		for _, k := range []string{"m", "shift+tab"} {
			m := sceneModel(c.sc, 80, 24)
			pressKey(m, k) // refused: no board, no mirror, under 110
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			foot := rows[len(rows)-1]
			if !strings.Contains(foot, "110") {
				t.Fatalf("%s %q: not the width refusal: %q", c.name, k, strings.TrimSpace(foot))
			}
			if !strings.Contains(foot, "tab deeper") {
				t.Errorf("%s %q: the width refusal's unit costs the frame its only naming of the way deeper: %q",
					c.name, k, strings.TrimSpace(foot))
			}
		}
	}
}

// ---- round 78, two-tools ----
//
// A board card says the question once, and keeps its model.
//
// #107 blanks a card's second row where a trail row of that same column
// already says it, and #112 then hoists the tool and its model onto the
// row the card gave up — which on a board whose question is which model
// is the card's only naming of it. #116's reassembly of a label that
// wrapped dropped the whole continuation row as soon as the options
// began on it, so where the question's tail and its options shared one
// row — `│  └ CIDR? [office CIDR / keep bastion]`, the shape a 48-cell
// column draws — the compare failed: the card drew the sentence a
// second time and the tag row fell to the bare tool word, while the
// same card at 120 and at 220 drew `claude · sonnet-4-5`.
//
// The pin is the property, not the literal: on every board width the
// card's second row is not a copy of the question its own trail row
// says, and the column names the model.
func TestTheBoardCardSaysTheQuestionOnce(t *testing.T) {
	forceASCII(t)
	// The words before the options, as the card would draw them.
	const question = "Open port 22 to the office CIDR?"
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		// Answer the waiting question — the trace the card's tag row
		// then draws is what leaves the tag no room — and hide the
		// namesake so the board draws three columns.
		for _, k := range []string{"r", "1", "j", "x"} {
			pressKey(m, k)
		}
		if m.level != levelBoard {
			t.Fatalf("%dx%d: not on the board (level %d)", size[0], size[1], m.level)
		}
		var col []string
		for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			col = append(col, strings.TrimRight(strings.SplitN(l, "│", 2)[0], " "))
		}
		head := -1
		for i, l := range col {
			if strings.Contains(l, "▲ infra") {
				head = i
				break
			}
		}
		if head < 0 || head+2 >= len(col) {
			t.Fatalf("%dx%d: no infra column on the board", size[0], size[1])
		}
		second := strings.TrimSpace(col[head+1])
		if second != "" && strings.HasPrefix(question, strings.TrimSuffix(second, "…")) {
			t.Errorf("%dx%d: the card's second row repeats the question its own trail row says: %q",
				size[0], size[1], second)
		}
		named := false
		for _, l := range col[head : head+3] {
			if strings.Contains(l, "sonnet-4-5") {
				named = true
			}
		}
		if !named {
			t.Errorf("%dx%d: the card names no model: %q", size[0], size[1], col[head:head+3])
		}
	}
}

// TestTheCardYieldsThePaneTheHeaderDrawsToTheClock pins round seventy-eight's
// one thing: the board card's third row is the tag's row, and #59's last-rung
// fallback took a bare pane even where taking it cost the trace a clause.
// #85's reason for that rung — "the pane is what attaches and is nowhere else
// on the board" — is false on the frame the identity header titles by that
// very pane: at 120 the selected column spent " · 0s ago", the send's clock
// and on no other row of the column, to draw "⌁ harness:1.0" a second time,
// one cell short. #90's device — the tag yields to the trace's clock where
// the frame says the tag elsewhere — from the tool word to the bare pane.
func TestTheCardYieldsThePaneTheHeaderDrawsToTheClock(t *testing.T) {
	forceASCII(t)
	pane := regexp.MustCompile(`⌁ [A-Za-z0-9_.:-]+`)
	for _, c := range []struct {
		name string
		sc   scene
	}{
		{"fleet-hygiene", sceneFleetHygiene()},
		{"subagents", sceneSubagents()},
	} {
		for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
			sc := c.sc
			m := sceneModel(sc, size[0], size[1])
			for _, k := range []string{"j", "r", "t", "go on", "enter"} {
				pressKey(m, k)
				poll(m, sc)
			}
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			if len(rows) < 4 {
				t.Fatalf("%s %dx%d: no frame", c.name, size[0], size[1])
			}
			said := pane.FindString(rows[0])
			if said == "" {
				t.Fatalf("%s %dx%d: the header names no pane: %q", c.name, size[0], size[1], strings.TrimSpace(rows[0]))
			}
			// The column that carries the send is the one whose trace the
			// reply wrote; it is the selected one, the one the header
			// titles.
			var trace string
			for _, r := range rows[1:] {
				for _, col := range strings.Split(r, "│") {
					if strings.Contains(col, `↪ sent "go on"`) {
						trace = strings.TrimSpace(col)
					}
				}
			}
			if trace == "" {
				t.Fatalf("%s %dx%d: no column carries the reply", c.name, size[0], size[1])
			}
			// Both sides. The clock is the send's own and is on no other
			// row of the column, so it stands at every width; and where
			// the column is wide enough for both, the pane stays — the
			// yield is priced in cells, not a shed of the tag (#85).
			if !strings.Contains(trace, " ago") {
				t.Errorf("%s %dx%d: the trace spends its clock to draw %s a second time: %q", c.name, size[0], size[1], said, trace)
			}
			if size[0] == 220 && !strings.Contains(trace, said) {
				t.Errorf("%s %dx%d: the pane went where both fit: %q", c.name, size[0], size[1], trace)
			}
		}
	}
}

// TestTheMarkSaysWhatThisColumnLost pins round eighty's one thing: the
// overlay's right-hand mark answers "what did the box hide of this column",
// so it stands where the box covered something of that column and nowhere
// else. #64 wrote that rule — no right-hand mark when the peek rule blanked
// the whole peek — but measured it on the whole rest of the row, and the
// rest of the row runs on through board columns the box never touched. On
// many-idle at 220 the reply box covered a column that was blank for six
// rows, and every one of them wore `…` because a column two rules further
// right had words in it; one was the box's own closing border. Both sides:
// where the box covered so much as the trail's rail the mark stands (#75).
func TestTheMarkSaysWhatThisColumnLost(t *testing.T) {
	forceASCII(t)
	step := func(m *Model, sc scene, keys ...string) {
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
	}

	for _, tc := range []struct{ w, h int }{{220, 48}, {152, 40}, {120, 34}} {
		sc := sceneManyIdle()
		m := sceneModel(sc, tc.w, tc.h)
		before := strings.Split(ansi.Strip(m.View()), "\n")
		step(m, sc, "r")
		got := strings.Split(ansi.Strip(m.View()), "\n")

		top, left, pw := -1, 0, 0
		for i, line := range got {
			if j := strings.Index(line, "┌ reply to"); j >= 0 {
				top = i
				left = len([]rune(line[:j]))
				pw = strings.Index(line[j:], "┐")
				pw = len([]rune(line[j:j+pw])) + 2 // the border, and its column of air
			}
		}
		if top < 0 {
			t.Fatalf("many-idle %dx%d: no reply box drawn:\n%s", tc.w, tc.h, strings.Join(got, "\n"))
		}

		marked, bare, checked := 0, 0, 0
		for i := top; i < len(got); i++ {
			runes := []rune(got[i])
			last := strings.Contains(got[i], "└─")
			if len(runes) <= left+pw {
				if last {
					break
				}
				continue
			}
			after := strings.TrimSpace(string(runes[left+pw:]))
			if after == "" {
				continue
			}
			checked++
			cut := ""
			if b := []rune(before[i]); len(b) > left {
				end := left + pw + 1
				if end > len(b) {
					end = len(b)
				}
				cut = string(b[left:end])
				if j := strings.LastIndex(cut, "│"); j >= 0 {
					cut = cut[j+len("│"):]
				}
			}
			if strings.TrimSpace(cut) != "" {
				marked++
				if !strings.HasPrefix(after, "…") {
					t.Errorf("many-idle %dx%d: the box hid %q of this column and the row does not say it was cut:\n%s",
						tc.w, tc.h, strings.TrimSpace(cut), got[i])
				}
				if last {
					break
				}
				continue
			}
			bare++
			if strings.HasPrefix(after, "…") {
				t.Errorf("many-idle %dx%d: the box cut nothing from this column and the row says it was cut:\n%s",
					tc.w, tc.h, got[i])
			}
			if last {
				break
			}
		}
		if checked == 0 || marked == 0 || bare == 0 {
			t.Fatalf("many-idle %dx%d: the frame does not carry both sides (checked %d, marked %d, bare %d):\n%s",
				tc.w, tc.h, checked, marked, bare, strings.Join(got, "\n"))
		}

		// The mark stands on the row whose column the box cut down to its
		// rail — #75's own case — and never on the box's closing border.
		for _, line := range got {
			if j := strings.Index(line, "└─"); j >= 0 {
				if strings.Contains(line[j:], "…") {
					t.Errorf("many-idle %dx%d: the box's closing border wears a cut mark for an empty column:\n%s",
						tc.w, tc.h, line)
				}
			}
		}
	}

	// #75: a peek that covered only the trail's rail keeps the mark.
	sc := sceneManyIdle()
	m := sceneModel(sc, 220, 48)
	step(m, sc, "r")
	rail := false
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(line, "● working for 1h") && strings.Contains(line, "…") {
			rail = true
		}
	}
	if !rail {
		t.Errorf("many-idle 220x48: the row whose column the box cut down to its rail lost its mark:\n%s",
			ansi.Strip(m.View()))
	}
}

// TestTheCardYieldsThePaneWithColourOn pins round eighty-two's one thing.
// #208's yield — the board card gives up a bare pane the identity header
// already draws rather than spend the trace's clock on it — is decided in
// `columnTag` by reading the trace back: `HasPrefix(full, "↪ ")`,
// `HasSuffix(full, " ago")` and `HasPrefix(full, beside)`. `boardDelta`
// returns `dimStyle.Render(trace)`, so with colour on those words sit
// inside escape sequences and all three tests miss: the guard never fires,
// the card draws `⌁ harness:1.0` a second time and the send's clock — the
// one fact on no other row of the column — is off the frame. #208's own
// pin could not see it because it renders under the ASCII profile, where
// `dimStyle.Render` is the identity.
//
// #215 settled which frame answers: the one a person sees, colour and all.
// So the trace is read through its style.
//
// Both sides: at 220 the column is wide enough for the pane and the clock
// and the pane stays (#85), which is the yield being priced in cells and
// not a shed of the tag.
func TestTheCardYieldsThePaneWithColourOn(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // the trace is drawn dim, not bare
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	pane := regexp.MustCompile(`⌁ [A-Za-z0-9_.:-]+`)
	for _, c := range []struct {
		name string
		sc   scene
	}{
		{"fleet-hygiene", sceneFleetHygiene()},
		{"subagents", sceneSubagents()},
	} {
		for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
			sc := c.sc
			m := sceneModel(sc, size[0], size[1])
			for _, k := range []string{"j", "r", "t", "go on", "enter"} {
				pressKey(m, k)
				poll(m, sc)
			}
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			if len(rows) < 4 {
				t.Fatalf("%s %dx%d: no frame", c.name, size[0], size[1])
			}
			said := pane.FindString(rows[0])
			if said == "" {
				t.Fatalf("%s %dx%d: the header names no pane: %q", c.name, size[0], size[1], strings.TrimSpace(rows[0]))
			}
			var trace string
			for _, r := range rows[1:] {
				for _, col := range strings.Split(r, "│") {
					if strings.Contains(col, `↪ sent "go on"`) {
						trace = strings.TrimSpace(col)
					}
				}
			}
			if trace == "" {
				t.Fatalf("%s %dx%d: no column carries the reply", c.name, size[0], size[1])
			}
			if !strings.Contains(trace, " ago") {
				t.Errorf("%s %dx%d: with colour on the trace spends its clock to draw %s a second time: %q",
					c.name, size[0], size[1], said, trace)
			}
			if size[0] == 220 && !strings.Contains(trace, said) {
				t.Errorf("%s %dx%d: the pane went where both fit: %q", c.name, size[0], size[1], trace)
			}
		}
	}
}

// TestTheBoardSaysTheSameWordsWithColourOn is the rule behind it, asked of
// the whole walkthrough rather than one frame: a style is how a row is
// drawn, never what it says, so the canonical walkthrough must read the
// same under the ASCII profile and under a colour one. Before the fold it
// differed on six rows in three files — every one a board trace that lost
// its clock to a pane the header draws.
func TestTheBoardSaysTheSameWordsWithColourOn(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	frames := func(sc scene, w, h int) []string {
		m := sceneModel(sc, w, h)
		out := []string{ansi.Strip(m.View())}
		for _, k := range canonicalKeys {
			pressKey(m, k)
			poll(m, sc)
			out = append(out, ansi.Strip(m.View()))
		}
		return out
	}
	trimAll := func(s string) string {
		rows := strings.Split(s, "\n")
		for i, r := range rows {
			rows[i] = strings.TrimRight(r, " ")
		}
		return strings.Join(rows, "\n")
	}
	for _, sc := range []scene{sceneFleetHygiene(), sceneSubagents()} {
		for _, size := range [][2]int{{120, 34}, {152, 40}} {
			lipgloss.SetColorProfile(termenv.Ascii)
			mono := frames(sc, size[0], size[1])
			lipgloss.SetColorProfile(termenv.TrueColor)
			colour := frames(sc, size[0], size[1])
			for i := range mono {
				a, b := strings.Split(trimAll(mono[i]), "\n"), strings.Split(trimAll(colour[i]), "\n")
				for j := range a {
					if j < len(b) && a[j] != b[j] {
						t.Errorf("%s %dx%d frame %d row %d: the colour profile changed what the row says\n ascii:  %q\n colour: %q",
							sc.name, size[0], size[1], i, j, a[j], b[j])
					}
				}
			}
		}
	}
}

// r90ttFoot is the last drawn row of a frame, escapes off.
func r90ttFoot(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return rows[len(rows)-1]
}

// r90ttStand plays a key run into a fresh model of the scene at one size.
func r90ttStand(sc scene, w, h int, keys ...string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// TestNoDigitDeniesALiveSessionTheFleetNumbers pins round ninety's one
// thing. `no session 1` is the sentence for a digit no session ever had.
// Under a fleet query that matches nothing, and in an archive that draws
// no row, the view draws no row for the digit — but the fleet still
// numbers a live session by it: the board one `esc` away calls it
// `1 infra`, the chips on the same row count it (`▲1 4m`), and where the
// digit is the selected session's own the deck already answers (#238,
// #242, #243, #245). The refusal names it, as the hidden twin does (#57).
//
// The other side is held too: where the archive draws rows the digits are
// the archive's own (#32), so a digit past its last row is still refused,
// and a digit no session carries is still `no session 7`.
func TestNoDigitDeniesALiveSessionTheFleetNumbers(t *testing.T) {
	forceASCII(t)
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]

			// The board under a query that matches nothing: `1` is the
			// fleet's needs-you session, live, on the board one esc away.
			m := r90ttStand(sceneTwoTools(), w, h, "2", "/", "zzz", "enter", "1")
			foot := r90ttFoot(m)
			if strings.Contains(foot, "no session 1") {
				t.Errorf("%dx%d %v: the board denies the digit the fleet numbers: %q", w, h, prof, foot)
			}
			if !strings.Contains(foot, "1 infra") {
				t.Errorf("%dx%d %v: the refusal does not name session 1: %q", w, h, prof, foot)
			}
			// A digit no session carries is still refused.
			m = r90ttStand(sceneTwoTools(), w, h, "2", "/", "zzz", "enter", "7")
			if foot := r90ttFoot(m); !strings.Contains(foot, "no session 7") {
				t.Errorf("%dx%d %v: a digit no session carries lost its refusal: %q", w, h, prof, foot)
			}

			// An archive that draws no row: the numbers are nobody's, and
			// the header on that very frame wears the board's digit (#248).
			m = r90ttStand(sceneManyIdle(), w, h, "2", "/", "zzz", "enter", "A", "1")
			foot = r90ttFoot(m)
			if !m.archiveView {
				t.Fatalf("%dx%d %v: the run did not reach the archive", w, h, prof)
			}
			if strings.Contains(foot, "no session 1") {
				t.Errorf("%dx%d %v: the empty archive denies the digit the fleet numbers: %q", w, h, prof, foot)
			}
			if !strings.Contains(foot, "1 etl") {
				t.Errorf("%dx%d %v: the empty archive's refusal does not name session 1: %q", w, h, prof, foot)
			}

			// An archive that draws rows numbers them itself (#32): a
			// digit past its last row is refused, whatever the board
			// calls that number.
			m = r90ttStand(sceneTwoTools(), w, h, "2", "x", "A", "3")
			if !m.archiveView || len(m.viewOrder()) == 0 {
				t.Fatalf("%dx%d %v: the run did not reach an archive with rows", w, h, prof)
			}
			if foot := r90ttFoot(m); !strings.Contains(foot, "no session 3") {
				t.Errorf("%dx%d %v: the archive's own numbering lost its refusal: %q", w, h, prof, foot)
			}
		}
		lipgloss.SetColorProfile(old)
	}
}

// ---- round 90, the owner's look ----
// The board keeps its order while the selection moves along it. `boardKeys`
// appends the selected session to the owed so a digit always shows its
// trail, and the pack read that list as the board's order: a calm selected
// session took the head of the calm band, and `j` along four calm cards
// reshuffled the board on every press — the card just read jumped to the
// front and the others slid (#252). Where the selected column would be
// trimmed off the end it still takes the last drawn slot (#16, #90).
func TestTheBoardKeepsItsOrderWhileTheSelectionMoves(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{152, 40}, {220, 48}} {
		m := sceneModel(sceneManyIdle(), size[0], size[1])
		inner := size[0] - 2*edgePad
		n, cw := boardColumns(inner, m.drawnCount(m.viewOrder()))
		if n < 2 {
			t.Fatalf("%dx%d: not a board of columns", size[0], size[1])
		}
		before, _ := m.boardPack(n, cw, size[1]-6)
		if len(before) < 4 {
			t.Fatalf("%dx%d: too few columns to walk: %v", size[0], size[1], before)
		}
		// Walk the selection along every drawn column: the drawn order
		// must not change while every selected column is already drawn.
		for step := 0; step < len(before); step++ {
			pressKey(m, "j")
			after, _ := m.boardPack(n, cw, size[1]-6)
			drawn := false
			for _, k := range after {
				if k == m.selectedKey {
					drawn = true
				}
			}
			if !drawn {
				break // the walk left the board for the strip
			}
			wasDrawn := false
			for _, k := range before {
				if k == m.selectedKey {
					wasDrawn = true
				}
			}
			want := before
			if !wasDrawn {
				// The selection walked to a column the pack trims off
				// the end: it takes the last drawn slot (#16, #90), and
				// that is the one change allowed.
				want = append(append([]string(nil), before[:len(before)-1]...), m.selectedKey)
			}
			if strings.Join(after, ",") != strings.Join(want, ",") {
				t.Fatalf("%dx%d: after %d presses of j the board reshuffled:\n before %v\n after  %v", size[0], size[1], step+1, before, after)
			}
			before = after
		}
	}
}

// ---- round 90, fleet-hygiene ----
// Round ninety, the fleet-hygiene operator, the one thing.
//
// #47 gave the recent band its digits — "numbered on from the live fleet's
// last digit … A digit opens its row in the archive, where the trail is
// already known how to draw, and `A` comes back to the live fleet where
// you were" — and #51 held only that such a digit "lands under a different
// number (#32)", which presumes it lands.
//
// It does not land on the board. `recentRows` refuses the board (#43), and
// `strandedBand` lifts that refusal for the one render call that draws a
// band under the strip where the columns have run out. `openRecent` calls
// `recentRows` outside that call, sees no band, and answers `no session 5`
// on a frame drawing `5 ○ api · "the pane I closed half an hour ago"` five
// rows above the footer — the digit denying a row the frame numbers (#243,
// #245), while the same digit on the same fleet twenty columns narrower
// opens it. The key now reads the band the frame drew, and no other row.

// r90fhBandRowRe matches a band row as drawn: two or three cells of air,
// the digit, the archived glyph.
var r90fhBandRowRe = regexp.MustCompile(`^\s{1,3}(\d) ○ `)

// r90fhBandDigits reads the digits the frame draws on its band — the rows
// under the band's own `A browses` header that no live column wears.
func r90fhBandDigits(m *Model) []int {
	live := map[int]bool{}
	for _, r := range m.boardRows() {
		live[r.num] = true
	}
	var out []int
	below := false
	for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(row, "· A browses") || strings.Contains(row, "archived · A") {
			below = true
			continue
		}
		if !below {
			continue
		}
		if mm := r90fhBandRowRe.FindStringSubmatch(row); mm != nil {
			d, _ := strconv.Atoi(mm[1])
			if !live[d] {
				out = append(out, d)
			}
		}
	}
	return out
}

// r90fhBandName is the session name the band's row for this digit draws.
func r90fhBandName(m *Model, d int) string {
	below := false
	for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(row, "· A browses") || strings.Contains(row, "archived · A") {
			below = true
			continue
		}
		if !below {
			continue
		}
		if mm := r90fhBandRowRe.FindStringSubmatch(row); mm != nil {
			if n, _ := strconv.Atoi(mm[1]); n == d {
				rest := strings.TrimSpace(row[strings.Index(row, "○ ")+len("○ "):])
				if i := strings.Index(rest, " · "); i > 0 {
					return rest[:i]
				}
				return rest
			}
		}
	}
	return ""
}

// TestTheBoardsBandOpensOnItsDigit is the fold on its frame: fleet-hygiene
// at 120, 152 and 220, where the board strands a band under its strip.
func TestTheBoardsBandOpensOnItsDigit(t *testing.T) {
	forceASCII(t)
	var fh scene
	for _, sc := range allScenes() {
		if sc.name == "fleet-hygiene" {
			fh = sc
		}
	}
	if fh.name == "" {
		t.Fatal("no fleet-hygiene scene")
	}
	for _, wh := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
			old := lipgloss.ColorProfile()
			lipgloss.SetColorProfile(prof) // the frame a person sees (#215, #218)
			m := sceneModel(fh, wh[0], wh[1])
			if m.level != levelBoard || !m.boardShown() {
				lipgloss.SetColorProfile(old)
				t.Fatalf("%dx%d: fleet-hygiene does not open on the board", wh[0], wh[1])
			}
			digits := r90fhBandDigits(m)
			if len(digits) == 0 {
				lipgloss.SetColorProfile(old)
				t.Fatalf("%dx%d: the board strands no band, so the stand is not the stand", wh[0], wh[1])
			}
			for _, d := range digits {
				c := sceneModel(fh, wh[0], wh[1])
				_ = c.View() // the key is pressed on a drawn frame, as the deck presses it (#221)
				want := r90fhBandName(c, d)
				was := c.selectedKey
				pressKey(c, strconv.Itoa(d))
				poll(c, fh)
				if note := ansi.Strip(c.note); strings.HasPrefix(note, "no session ") {
					t.Errorf("%dx%d (%v): `%d` answered %q over the band row it draws (%s)",
						wh[0], wh[1], prof, d, note, want)
					continue
				}
				if !c.archiveView {
					t.Errorf("%dx%d (%v): `%d` did not open the archive", wh[0], wh[1], prof, d)
					continue
				}
				s, ok := c.selected()
				if !ok || s.Live {
					t.Errorf("%dx%d (%v): `%d` landed on no archived session", wh[0], wh[1], prof, d)
					continue
				}
				if got := sessionName(s.Info); got != want && !strings.Contains(archiveHeadline(s), want) {
					t.Errorf("%dx%d (%v): `%d` opened %q where its row names %q", wh[0], wh[1], prof, d, got, want)
				}
				// #47's other half: `A` comes back to the live fleet
				// where you were, at the level the digit was pressed (#54).
				pressKey(c, "A")
				poll(c, fh)
				if c.archiveView || c.level != levelBoard || c.selectedKey != was {
					t.Errorf("%dx%d (%v): after `%d` then `A` the deck is not back on the board it left (archive=%v level=%d)",
						wh[0], wh[1], prof, d, c.archiveView, c.level)
				}
			}
			lipgloss.SetColorProfile(old)
		}
	}
}

// TestNoBandRowRefusesTheDigitItWears is the rule behind it, over every
// scene and width: a digit the frame draws on its band opens that row, and
// a digit no row wears is still refused.
func TestNoBandRowRefusesTheDigitItWears(t *testing.T) {
	sweep(t)
	forceASCII(t)
	drawn, refusedRight := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		if prof == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := wh[0], wh[1]
				base := sceneModel(sc, w, h)
				_ = base.View()
				band := map[int]bool{}
				for _, d := range r90fhBandDigits(base) {
					band[d] = true
				}
				for d := 1; d <= 9; d++ {
					m := sceneModel(sc, w, h)
					_ = m.View() // pressed on a drawn frame (#221)
					pressKey(m, strconv.Itoa(d))
					poll(m, sc)
					note := ansi.Strip(m.note)
					refused := note == fmt.Sprintf("no session %d", d)
					if band[d] {
						drawn++
						if refused {
							t.Errorf("%s %dx%d (%v): `%d` answered %q over the band row it draws",
								sc.name, w, h, prof, d, note)
						}
					} else if refused {
						// The other side: a digit no row wears is still
						// refused, and the fold gives the board no band
						// it did not draw.
						refusedRight++
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if drawn < 40 {
		t.Fatalf("only %d band digits drawn: the sweep is not sweeping", drawn)
	}
	t.Logf("%d band digits drawn, %d digits no row wears still refused", drawn, refusedRight)
}

// ---- round 96, second-day ----
// TestTheHeaderSaysTheAskOnceAndKeepsTheToolWord: the identity header of a
// session its person named draws the name, not the name and a second copy
// of what it asked.
//
// The frame it was found on: `second-day` at eighty, `A` then `3` — the
// header drew ` ⌂ compass · 3 checkout-flake-hunt · "the checkout suite
// flake…`, a clipped copy of a sentence the trail's `◉` row three rows
// below drew whole, and paid for the fragment with `· claude`, the word
// every other archive header at that width keeps on a fleet holding two
// tools (#79, #80). Then the rule over every scene, every width and both
// profiles: where the header names a session by `name · "ask"`, the ask is
// drawn on a `◉` row of the same frame, so the header's copy is the second
// one — #265's cut at the archive board's card head and #269's at the ship
// row, on the row that is drawn at every level.
func TestTheHeaderSaysTheAskOnceAndKeepsTheToolWord(t *testing.T) {
	sweep(t)
	prev := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(prev)

	// 1 — the frame it was found on.
	lipgloss.SetColorProfile(termenv.Ascii)
	sc := sceneSecondDay()
	m := sceneModel(sc, 80, 24)
	for _, k := range []string{"A", "3"} {
		pressKey(m, k)
		poll(m, sc)
	}
	head := ansi.Strip(strings.Split(m.View(), "\n")[0])
	if strings.Contains(head, `"the checkout suite`) {
		t.Errorf("80x24 A,3: the header draws the ask the trail draws whole: %q", head)
	}
	if !strings.Contains(head, "3 checkout-flake-hunt") || !strings.Contains(head, "claude") {
		t.Errorf("80x24 A,3: the header should name the session and its tool: %q", head)
	}

	// 2 — the rule: no header names a session `name · "ask"` while a row of
	// the same frame draws that ask.
	routes := [][]string{
		{"A"}, {"A", "3"}, {"A", "j", "j"}, {"A", "j", "j", "tab"},
		{"A", "shift+tab"}, {"A", "shift+tab", "3"}, {"A", "3", "tab", "tab"},
	}
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"ascii", termenv.Ascii}, {"truecolor", termenv.TrueColor}} {
		if prof.p == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				for _, route := range routes {
					m := sceneModel(sc, size[0], size[1])
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					rows := strings.Split(m.View(), "\n")
					h := ansi.Strip(rows[0])
					i := strings.Index(h, ` · "`)
					if !strings.HasPrefix(h, " ⌂ compass · ") || i < 0 {
						continue
					}
					rest := h[i+len(` · "`):]
					j := strings.Index(rest, `"`)
					if j < 0 {
						continue
					}
					ask := strings.TrimSuffix(rest[:j], "…")
					if len(ask) < 8 {
						continue
					}
					t.Errorf("%s %s %dx%d %v: the header quotes the ask beside the name: %q",
						prof.name, sc.name, size[0], size[1], route, h)
				}
			}
		}
	}
}

// ---- round 99, two-tools ----
// r99ttUnit is the one word of a width refusal the person's own terminal
// supplies (#190): `mirror needs 110 columns`, `no board under 110 columns`.
const r99ttUnit = " columns"

// r99ttFooterKeys is the footer's keys as a set, read past the attach
// aside, which is not a key (#55).
func r99ttFooterKeys(view string) map[string]bool {
	rows := strings.Split(ansi.Strip(view), "\n")
	foot := rows[len(rows)-1]
	keys := map[string]bool{}
	for _, frag := range strings.Split(strings.ReplaceAll(foot, attachHint, ""), " · ") {
		if f := strings.TrimSpace(frag); f != "" {
			keys[f] = true
		}
	}
	return keys
}

// TestTheWidthRefusalKeepsItsUnitOnlyWhereItCostsNoKey walks every scene at
// the two widths a width refusal is drawn at, under both colour profiles
// (#215, #218). Wherever the note ends in the unit, the same frame is drawn
// again with the unit dropped: the shorter note must put no key on the row
// that the longer one left off. A key that acts and is not named is what a
// footer is for (#24, #165, #175, #187, #190, #194, #198, #264), and the
// eight cells are a word the terminal supplies.
func TestTheWidthRefusalKeepsItsUnitOnlyWhereItCostsNoKey(t *testing.T) {
	forceASCII(t)
	routes := [][]string{
		{"m"},
		{"tab", "m"},
		{"shift+tab"},
		{"j", "tab", "m"},
		{"j", "m"},
		{"tab", "tab", "m"},
		{"shift+tab", "shift+tab"},
	}
	refusals, checked := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}} {
				w, h := size[0], size[1]
				for ri, route := range routes {
					m := sceneModel(sc, w, h)
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					view := m.View()
					rows := strings.Split(ansi.Strip(view), "\n")
					foot := strings.TrimSpace(rows[len(rows)-1])
					if !strings.Contains(foot, "110") {
						continue
					}
					refusals++
					// The refusal still names its number and its key.
					if !strings.Contains(foot, "mirror needs 110") && !strings.Contains(foot, "no board under 110") {
						t.Errorf("%s %dx%d r%d p%v: the width refusal lost its words: %q", sc.name, w, h, ri, prof, foot)
						continue
					}
					if !strings.HasSuffix(m.note, r99ttUnit) {
						continue
					}
					checked++
					was := r99ttFooterKeys(view)
					m.note = strings.TrimSuffix(m.note, r99ttUnit)
					now := r99ttFooterKeys(m.View())
					var gained []string
					for k := range now {
						if !was[k] && !strings.Contains(k, "110") {
							gained = append(gained, k)
						}
					}
					if len(gained) > 0 {
						t.Errorf("%s %dx%d r%d p%v: the unit cost the footer %s: %q",
							sc.name, w, h, ri, prof, fmt.Sprint(gained), foot)
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if refusals < 40 {
		t.Fatalf("the walk reached only %d width refusals", refusals)
	}
	if checked < 10 {
		t.Fatalf("only %d refusals still wore the unit — the rule was not exercised", checked)
	}
}

// ---- round 106, two-tools ----
// r106ttOffSizes are the five terminals the walkthrough is drawn at. Below
// `deckWideCols` the key refuses in its own words (`mirror needs 110
// columns`) and never flips the flag, so those two widths measure the guard
// rather than the toggle.
var r106ttOffSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r106ttOffRoutes are ways to a frame `m` is pressed on: the board it opens
// on and two moves across it, and three walks a level or two in, where the
// panel the key puts away is replaced by something the frame draws.
var r106ttOffRoutes = [][]string{
	{}, {"j"}, {"1"},
	{"tab"}, {"tab", "tab"}, {"tab", "tab", "]"},
}

// r106ttOffOn and r106ttOffBoard are the two halves of the board's mirror
// sentence: the state the first press says, and the state the second owes.
// `the conversation` and `the live pane` are the same sentence one and two
// levels in, where a panel answers for the key.
const (
	r106ttOffOn    = "mirror on"
	r106ttOffBoard = "mirror off"
	r106ttOffDeep  = "the conversation"
	r106ttOffPane  = "the live pane"
)

// r106ttOffFrame is one drawn frame, escapes stripped and read once: the
// rows above the footer, the footer, its keymap half and its note.
type r106ttOffFrame struct {
	rows []string
	body string
	foot string
	keys string
	note string
}

func r106ttOffRead(m *Model) r106ttOffFrame {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	f := r106ttOffFrame{rows: rows}
	if len(rows) == 0 {
		return f
	}
	f.body = strings.Join(rows[:len(rows)-1], "\n")
	f.foot = rows[len(rows)-1]
	f.keys = r106ttOffKeys(f.foot)
	s := strings.TrimLeft(strings.TrimRight(f.foot, " "), " ")
	if i := strings.Index(s, "  "); i >= 0 {
		f.note = strings.TrimSpace(s[i:])
	}
	return f
}

// r106ttOffKeys is the keymap half of a footer: what stands before the gap
// the note lives after (#134's reserve), with the attach aside — which is
// not a key (#55) — read past.
func r106ttOffKeys(foot string) string {
	s := strings.TrimRight(foot, " ")
	s = strings.Replace(s, " (prefix d returns)", "", 1)
	if i := strings.Index(strings.TrimLeft(s, " "), "  "); i >= 0 {
		s = strings.TrimLeft(s, " ")[:i]
	}
	return strings.TrimSpace(s)
}

// r106ttOffClauses splits a keymap half into its clauses, so a row can be
// asked whether it still names everything it named before.
func r106ttOffClauses(keys string) []string {
	var out []string
	for _, c := range strings.Split(keys, " · ") {
		if c = strings.TrimSpace(c); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// TestTheBoardsMirrorKeySaysTheFlagBothWays pins the board's `m` to the
// state it changed. The mirror is a panel of Lv1 and Lv2 and the board
// draws it at neither (#15), so on the board the flag is the whole of what
// the key does: the first press says `mirror on` (#37, #177) and the second
// said nothing at all — the frame above the footer came back byte for byte
// and the note went, on 237 of the 912 corpus stands where `m` arms the
// mirror, every one of them the board. That is the silent second press #241
// refused for the reader's `g` and #290 refused for the reader's own `m`,
// on the one level where no panel answers for the key. The board says the
// state the other way round: `mirror off`, ten cells, inside the note's own
// reserve.
//
// Three sides, so the sentence cannot be jammed onto every level:
//   - on the board, where `m` turns the mirror off, the note is
//     `mirror off` and the body above the footer is unchanged, so the note
//     is the only thing that can answer;
//   - the row pays nothing for it: the off frame names every key the on
//     frame named, and the frame fits its terminal;
//   - one and two levels in the key keeps its own words — the session view
//     says `the conversation` and the reader `the live pane` on either
//     press (#290) — and no frame off the board ever says `mirror off`.
func TestTheBoardsMirrorKeySaysTheFlagBothWays(t *testing.T) {
	sweep(t)
	forceASCII(t)
	boards, deeps := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		if prof == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range r106ttOffSizes {
				w, h := size[0], size[1]
				for _, route := range r106ttOffRoutes {
					m := sceneModel(sc, w, h)
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					if m.showHelp || m.searching || m.replying || m.archiveView {
						continue
					}
					where := fmt.Sprintf("%s %v %dx%d Lv%d", sc.name, route, w, h, m.level)

					// The first press arms the mirror; the second is the
					// one under test, pressed on the frame it is read
					// from (#221).
					pressKey(m, "m")
					poll(m, sc)
					if !m.showMirror {
						continue
					}
					board := m.level == levelBoard
					on := r106ttOffRead(m)
					if board && on.note != r106ttOffOn {
						t.Errorf("%s: `m` armed the mirror on the board and said %q, not %q", where, on.note, r106ttOffOn)
					}
					pressKey(m, "m")
					poll(m, sc)
					off := r106ttOffRead(m)
					note, offKeys := off.note, off.keys

					if !board {
						// One and two levels in the key keeps its own
						// words, and never the board's.
						deeps++
						if note == r106ttOffBoard {
							t.Errorf("%s: `m` off the board says %q; here a panel answers for the key (#290)", where, note)
						}
						if m.sessionView() && note != r106ttOffDeep && note != r106ttOffPane {
							t.Errorf("%s: `m` in the session view says %q, not %q or %q", where, note, r106ttOffDeep, r106ttOffPane)
						}
						continue
					}

					boards++
					if off.body != on.body {
						t.Errorf("%s: `m` on the board redrew the frame above the footer; the board draws no mirror either way (#15)", where)
					}
					if note == "" {
						t.Errorf("%s: `m` turned the mirror off on the board and said nothing; the frame above the footer came back byte for byte and only the note went\n  foot=%q",
							where, strings.TrimRight(off.foot, " "))
						continue
					}
					if note != r106ttOffBoard {
						t.Errorf("%s: the board's mirror-off note is %q, not the state %q", where, note, r106ttOffBoard)
					}
					// The note is inside the reserve: the row gives up no
					// key for it (#39, #177, #281, #284).
					for _, clause := range r106ttOffClauses(on.keys) {
						if !strings.Contains(offKeys, clause) {
							t.Errorf("%s: the mirror-off note cost the row %q\n  on =%q\n  off=%q", where, clause, on.keys, offKeys)
						}
					}
					for _, r := range off.rows {
						if lipgloss.Width(r) > w {
							t.Errorf("%s: the frame `m` landed on has a row over %d cells: %q", where, w, r)
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if boards < 50 { // half of the two-profile floor: the sweep walks one profile (#324)
		t.Errorf("only %d board stands where `m` arms the mirror; the biting side is unmeasured", boards)
	}
	if deeps < 75 { // half of the two-profile floor: the sweep walks one profile (#324)
		t.Errorf("only %d stands off the board; the held side is unmeasured", deeps)
	}
	t.Logf("board stands where `m` arms the mirror: %d · stands off the board: %d", boards, deeps)
}

// The board's recent band stands one row of air off the last band's
// columns (#326). The strip sat one row under the columns, and the band
// took the strip's row when it replaced it (#147), so on a tall screen the
// band's header sat against the columns' last rows and read as one more of
// them, over twenty rows of nothing. Where the board draws the band, the
// two rows above its header are air and the row above those is a column's.
func TestTheBoardsBandStandsARowOffTheColumns(t *testing.T) {
	forceASCII(t)
	stands := 0
	for _, sc := range allScenes() {
		for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}, {220, 60}} {
			m := sceneModel(sc, size[0], size[1])
			poll(m, sc)
			if m.level != levelBoard || !m.boardShown() {
				continue
			}
			lines := strings.Split(ansi.Strip(m.View()), "\n")
			head := -1
			for i, l := range lines {
				if strings.HasPrefix(strings.TrimSpace(l), "recent ·") {
					head = i
				}
			}
			if head < 3 {
				continue
			}
			stands++
			blank := func(i int) bool { return strings.TrimSpace(lines[i]) == "" }
			if !blank(head-1) || !blank(head-2) || blank(head-3) {
				t.Errorf("%s %dx%d: the band's header should stand under two rows of air and a column row\n%s",
					sc.name, size[0], size[1], strings.Join(lines[head-3:head+1], "\n"))
			}
		}
	}
	if stands < 3 {
		t.Errorf("only %d boards drew the band; the rule is unmeasured", stands)
	}
}
