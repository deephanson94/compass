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

// The porter column draws the lead's message as the ask, worn as a relay:
// the card and the trail say `relayed "…"`, not the harness's envelope, and
// not nothing (#97).
func TestARelayedAskIsWornAsARelay(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 220, 48)
	view := ansi.Strip(m.View())
	if strings.Contains(view, "Another Claude session") {
		t.Errorf("the relay's envelope is drawn as the ask:\n%s", view)
	}
	if !strings.Contains(view, `relayed "the encoder is in, run the gates"`) {
		t.Errorf("the relayed ask is not worn as one:\n%s", view)
	}
	press(m, "A")
	if view := ansi.Strip(m.View()); !strings.Contains(view, `relayed "`) && strings.Contains(view, "porter") {
		t.Errorf("the archive's strip drops the relay word:\n%s", view)
	}
}

// The alarm leg's wrap budget is the span the row draws: at 220 the
// question fits whole beside "waiting 4m" and the options row is the
// options alone (#113).
func TestTheQuestionWrapsAgainstTheSpanItDraws(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 220, 48)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Open port 22 to the office CIDR? waiting 4m") {
		t.Errorf("the question wraps inside itself a cell short of fitting:\n%s", view)
	}
	if strings.Contains(view, "└ CIDR? [") {
		t.Errorf("the question's tail rides the options row:\n%s", view)
	}
}

// On the reply frame the needs-you row leaves its question to the box,
// which says it under its own head one row up (#133).
func TestTheNeedsYouRowLeavesTheQuestionToTheReplyBox(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{100, 30}, {80, 24}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		pressKey(m, "r")
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "reply to 1") || strings.Count(view, "Open port 22") < 1 {
			t.Fatalf("at %dx%d not the reply frame on infra:\n%s", size[0], size[1], view)
		}
		lines := strings.Split(view, "\n")
		for i, l := range lines {
			if strings.Contains(l, "▸1 ▲ infra") && i+1 < len(lines) {
				if strings.Contains(strings.SplitN(lines[i+1], "│", 2)[0], "Open port 22") {
					t.Errorf("at %dx%d the row repeats the question the box says: %q", size[0], size[1], lines[i+1])
				}
			}
		}
	}
}

// The no-pane refusal names the key and keeps the way in: `reply needs a
// pane` leaves the eighty-column footer its `tab deeper`, and does not say
// `no pane` twice on one row (#165, #156, #159).
func TestTheNoPaneRefusalKeepsTheWayIn(t *testing.T) {
	forceASCII(t)
	for _, key := range []string{"r", "enter"} {
		m := sceneModel(sceneFleetHygiene(), 80, 24)
		pressKey(m, "3") // notebooks, the session with no pane
		pressKey(m, key)
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "needs a pane") {
			t.Fatalf("%s: not the no-pane refusal:\n%s", key, view)
		}
		foot := ""
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "? help · q quit") {
				foot = l
			}
		}
		if !strings.Contains(foot, "tab deeper") {
			t.Errorf("%s: the refusal cost the footer the way in: %q", key, strings.TrimSpace(foot))
		}
		if strings.Count(foot, "no pane") > 1 {
			t.Errorf("%s: the footer says no pane twice: %q", key, strings.TrimSpace(foot))
		}
	}
}

// A label that wrapped is reassembled whole: the options are not the
// label's, but the words before them on the same continuation row are.
func TestAWrappedLabelKeepsTheWordsBeforeTheOptions(t *testing.T) {
	rows := []string{
		" ▲ design Open port 22 to the office   waiting 4m",
		" │  └ CIDR? [office CIDR / keep bastion]",
	}
	if got, want := wrappedLabel(rows, 0, len(rows)), "▲ design Open port 22 to the office CIDR?"; got != want {
		t.Errorf("wrappedLabel = %q, want %q", got, want)
	}
	split := []string{
		" ▲ design Open port 22 to   waiting 4m",
		" │  ├ the office CIDR?",
		" │  └ [office CIDR / keep bastion]",
	}
	if got, want := wrappedLabel(split, 0, len(split)), "▲ design Open port 22 to the office CIDR?"; got != want {
		t.Errorf("wrappedLabel (two rows) = %q, want %q", got, want)
	}
}

// ---- round 78, second-day, the one thing ----
// An attach that cannot work is a refusal, not a key: it goes before the
// keys that act.
//
// `enter · no pane` is what the footer draws where the selected session
// has no pane to hand the terminal to (`enterKeymap`). The archive's own
// list already ranks it that way — "a refusal goes before the way in"
// (#52, #198) — but the reader's shed order ranked it *above* `a ask`,
// and `shedKeys` lets a key back only when every key ranked above it came
// back too. So at eighty the reader of a paneless session held eighteen
// cells for a refusal that could not be drawn at that width either and
// shed `a ask` — which in the archive is the reason to be there, "a
// claude on a session you can no longer attach to" (#52), and on a live
// paneless session is the one action left — leaving sixteen blank cells
// on the row. At a hundred, under a thirteen-cell note, the row spent
// eighteen cells on the refusal and shed the ask outright.
//
// The frames: second-day's archive reader at eighty
// (`space unfold · [ ] turns · esc back · A fleet · ? help · q quit`, 63
// cells in a 79-cell field) and fleet-hygiene's paneless live reader at
// eighty and, under `]`'s note, at a hundred.
func TestTheAttachRefusalYieldsToTheKeyThatActs(t *testing.T) {
	forceASCII(t)
	footer := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	cases := []struct {
		name  string
		scene func() scene
		keys  []string
		w, h  int
		title string
	}{
		{"second-day archive reader", sceneSecondDay, []string{"A", "8", "tab", "tab"}, 80, 24, "READER · cli"},
		{"second-day archive waypoints", sceneSecondDay, []string{"A", "8", "tab"}, 100, 30, "TRAIL · cli"},
		{"fleet-hygiene paneless reader", sceneFleetHygiene, []string{"3", "tab", "tab"}, 80, 24, "READER · notebooks"},
		{"fleet-hygiene paneless waypoints", sceneFleetHygiene, []string{"3", "tab"}, 80, 24, "TRAIL · notebooks"},
		{"fleet-hygiene paneless reader under a note", sceneFleetHygiene, []string{"3", "tab", "tab", "]"}, 100, 30, "READER · notebooks"},
	}
	for _, c := range cases {
		sc := c.scene()
		m := sceneModel(sc, c.w, c.h)
		for _, k := range c.keys {
			pressKey(m, k)
			poll(m, sc) // the refresh the deck does after every key
		}
		if !strings.Contains(ansi.Strip(m.View()), c.title) {
			t.Fatalf("%s %dx%d: not the reader: %q", c.name, c.w, c.h, footer(m))
		}
		// The selected session has no pane: `enter` is a refusal here.
		if m.enterKeymap() != "enter · no pane" {
			t.Fatalf("%s %dx%d: the row can be attached to: %q", c.name, c.w, c.h, m.enterKeymap())
		}
		foot := footer(m)
		if !strings.Contains(foot, "a ask") {
			t.Errorf("%s %dx%d: a session with no pane is offered no way to ask it: %q",
				c.name, c.w, c.h, foot)
		}
		// And the cells were there: the row the fold draws fits its field.
		if w := lipgloss.Width(foot); w > c.w-1 {
			t.Errorf("%s %dx%d: the footer overruns its field: %d cells", c.name, c.w, c.h, w)
		}
	}
	// The rank, stated: in the reader the refusal is shed before `a ask`,
	// as the archive's list sheds it before `tab deeper` and `a ask`.
	for _, lv := range []struct {
		name string
		keys []string
	}{{"the waypoints", []string{"A", "8", "tab"}}, {"the reader", []string{"A", "8", "tab", "tab"}}} {
		sc := sceneSecondDay()
		m := sceneModel(sc, 80, 24)
		for _, k := range lv.keys {
			pressKey(m, k)
			poll(m, sc)
		}
		order := m.shedOrder(false)
		refusal, ask := -1, -1
		for i, frag := range order {
			switch frag {
			case " · enter · no pane":
				refusal = i
			case " · a ask":
				ask = i
			}
		}
		if refusal < 0 || ask < 0 {
			t.Fatalf("%s shed order names neither the refusal nor the ask: %v", lv.name, order)
		}
		if refusal > ask {
			t.Errorf("%s sheds `a ask` before the attach refusal: refusal at %d, ask at %d", lv.name, refusal, ask)
		}
	}
}

// TestTheReplyBoxDoesNotStandOnTheStrip pins round seventy-eight's second
// finding: the reply box floats over the deck, and the board's leftmost
// column begins at cell zero, so a box placed by that column stood on the
// strip whole — the archive's own line, its count and the key that browses
// it. The panel's footer is its own and `A` does not act while the panel is
// up (#62, #64), so no key could say it either, and at 120, 152 and 220 the
// frame over forty-one archived sessions named neither, while the same
// keypress one column over named both. Both sides: the frame names the
// door, and the box does not move where it never covered it.
func TestTheReplyBoxDoesNotStandOnTheStrip(t *testing.T) {
	forceASCII(t)
	// The door in every form it sheds to, matched where it ends: the
	// box's own edge may stand on the same row (#176). The key half is
	// optional — while the panel is up the line wears no key, since `A`
	// does not act there (#282's rule on this row, archiveDoorKey) — and
	// what this test is about is the row the box must not cover.
	door := regexp.MustCompile(`\d archived(?: · [^·]*hidden)?(?: · A(?: browses)?)?(?:\s|$)`)
	for _, c := range []struct {
		name string
		sc   scene
	}{
		{"fleet-hygiene", sceneFleetHygiene()},
		{"many-idle", sceneManyIdle()},
	} {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			sc := c.sc
			m := sceneModel(sc, size[0], size[1])
			pressKey(m, "r")
			poll(m, sc)
			if !m.replying {
				t.Fatalf("%s %dx%d: r did not open the panel", c.name, size[0], size[1])
			}
			frame := ansi.Strip(m.View())
			if !strings.Contains(frame, "reply to ") {
				t.Fatalf("%s %dx%d: no reply box on the frame", c.name, size[0], size[1])
			}
			named := false
			for _, row := range strings.Split(frame, "\n") {
				if door.MatchString(row + " ") {
					named = true
				}
			}
			if !named {
				var foot string
				if rows := strings.Split(frame, "\n"); len(rows) > 0 {
					foot = strings.TrimSpace(rows[len(rows)-1])
				}
				t.Errorf("%s %dx%d: the reply box leaves the frame naming neither the archive nor its key (footer %q)", c.name, size[0], size[1], foot)
			}
			// The step is taken only where it buys the door: on a fleet
			// whose strip the box never covered, the box stays by the
			// column it is about.
			if c.name == "many-idle" && size[0] >= 120 && m.replyBox.left != 0 {
				t.Errorf("%s %dx%d: the box stepped where the strip already stood: left=%d", c.name, size[0], size[1], m.replyBox.left)
			}
		}
	}
}

// ---- round 83, second-day, the second finding ----
// The hairline runs beside the reply box. `joinColumns` stops the
// hairline where the columns' content stops — "a hairline stops where the
// content stops; empty rows stay empty" — but the reply box is
// composited over the body afterwards (#108), so a box reaching past
// every column's last row drew its own border with no hairline to its
// left. On the first session at eighty the fleet is one row and eleven of
// the box's fourteen rows stood past the hairline while the three above
// them carried it; on the second day it was the box's last row. The box's
// rows are the frame's content, so the hairline stops below them; where
// no box stands, an empty row is still empty.
func TestTheHairlineRunsBesideTheReplyBox(t *testing.T) {
	// The frame is the coloured one (#215, #218): the hairline is a drawn
	// cell either way, and this reads it through the profile a person has.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	const bodyTop = 3 // header, hairline, blank
	// at says whether the frame draws the hairline at cell col on row j.
	at := func(rows []string, j, col int) bool {
		if j < 0 || j >= len(rows) {
			return false
		}
		run := []rune(rows[j])
		return col < len(run) && run[col] == '│'
	}

	for _, c := range []struct {
		name string
		mk   func() scene
		w, h int
	}{
		{"first-session", sceneFirstSession, 80, 24},
		{"first-session", sceneFirstSession, 100, 30},
		{"second-day", sceneSecondDay, 80, 24},
		{"second-day", sceneSecondDay, 100, 30},
	} {
		sc := c.mk()
		m := sceneModel(sc, c.w, c.h)
		pressKey(m, "r")
		poll(m, sc)
		// `replyBox` is settled by View, which is where the box is placed.
		rendered := ansi.Strip(m.View())
		if !m.replyBox.on {
			t.Fatalf("%s %dx%d: `r` was expected to raise the reply box", c.name, c.w, c.h)
		}
		rows := strings.Split(rendered, "\n")
		top := bodyTop + m.replyBox.top
		last := top + m.replyBox.h - 1
		if last >= len(rows) {
			t.Fatalf("%s %dx%d: the box runs past the frame", c.name, c.w, c.h)
		}
		// The deck's hairline is the leftmost stroke on the row above the
		// box, left of where the box begins — the box draws strokes of
		// its own, so it cannot be found by counting.
		col := -1
		for i, r := range []rune(rows[top-1]) {
			if r == '│' && i < m.replyBox.left {
				col = i
				break
			}
		}
		if col < 0 {
			t.Fatalf("%s %dx%d: no hairline above the box on row %d: %q",
				c.name, c.w, c.h, top-1, rows[top-1])
		}
		for j := top; j <= last; j++ {
			if !at(rows, j, col) {
				t.Errorf("%s %dx%d: the box draws row %d and the hairline stops above it: %q",
					c.name, c.w, c.h, j, rows[j])
			}
		}
		// It is one stroke, and it stops: nothing below the box carries it.
		if at(rows, last+1, col) {
			t.Errorf("%s %dx%d: the hairline runs past the box to row %d: %q",
				c.name, c.w, c.h, last+1, rows[last+1])
		}
	}

	// Where no box stands the hairline still stops where the columns do:
	// the deck's blank tail carries none.
	for _, c := range []struct {
		name string
		mk   func() scene
		w, h int
	}{
		{"first-session", sceneFirstSession, 80, 24},
		{"second-day", sceneSecondDay, 80, 24},
	} {
		m := sceneModel(c.mk(), c.w, c.h)
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		tail := rows[len(rows)-3] // the row above the footer's hairline
		if strings.Contains(tail, "│") {
			t.Errorf("%s %dx%d: the hairline runs into the deck's blank tail: %q",
				c.name, c.w, c.h, tail)
		}
	}
}

// ---- round 84, second-day, the one thing ----
// The attach refusal goes before the search that acts. #206 taught the
// reader's and the waypoints' shed orders that "an attach that cannot
// work is a refusal, and a refusal goes before a key that acts", and
// moved `enter · no pane` above `a ask`; it was left ranked *below*
// `/ search` and `n/N` in all three orders, and `shedKeys` lets a key
// back only when every key ranked above it came back too. So the
// archive's own list at eighty — twelve finished sessions, four of them
// on screen and `▾ 8 more below · j` — spent eighteen of its
// seventy-six cells on a key that answers `attach needs a pane` at every
// width and named no way to search the archive, while `/` there filters
// the twelve to two and the header says `archive 2 of 12`. The
// eighteen cells buy `· / search` (eleven) with seven to spare.
//
// Both sides: where the row's own `enter` says `enter attach` nothing
// moves, and where the field is wide enough for both the refusal stays.
func TestTheAttachRefusalGoesBeforeTheSearchKey(t *testing.T) {
	// The frame is the coloured one (#215, #218): every press below is
	// measured with the profile a person's terminal has.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	footer := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	// body is the frame without its footer: a note lands there on every
	// press, so the body is what says whether a key moved a drawn cell.
	body := func(m *Model) string {
		rows := strings.Split(m.View(), "\n")
		return strings.Join(rows[:len(rows)-1], "\n")
	}
	stand := func(mk func() scene, w, h int, keys []string) (*Model, scene) {
		sc := mk()
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		return m, sc
	}

	for _, c := range []struct {
		name string
		mk   func() scene
		keys []string
		w, h int
		kept []string
	}{
		{"second-day archive list", sceneSecondDay, []string{"A"}, 80, 24,
			[]string{"j/k move", "tab deeper", "a ask", "A fleet", "? help", "q quit"}},
		{"second-day archive list, one row on", sceneSecondDay, []string{"A", "j"}, 80, 24,
			[]string{"j/k move", "tab deeper", "a ask", "A fleet", "? help", "q quit"}},
		{"second-day archive reader", sceneSecondDay, []string{"A", "8", "tab", "tab"}, 100, 30,
			[]string{"space unfold", "[ ] turns", "a ask", "esc back", "A fleet", "? help", "q quit"}},
		{"fleet-hygiene paneless reader", sceneFleetHygiene, []string{"3", "tab", "tab"}, 100, 30,
			[]string{"space unfold", "[ ] turns", "a ask", "esc back", "? help", "q quit"}},
	} {
		m, sc := stand(c.mk, c.w, c.h, c.keys)
		if m.enterKeymap() != "enter · no pane" {
			t.Fatalf("%s %dx%d: the row can be attached to: %q", c.name, c.w, c.h, m.enterKeymap())
		}
		// The refusal refuses: it moves no drawn cell of the body, with
		// colour on, and answers in words.
		before := body(m)
		e, se := stand(c.mk, c.w, c.h, append(append([]string(nil), c.keys...), "enter"))
		_ = se
		if body(e) != before {
			t.Errorf("%s %dx%d: `enter` moves a drawn cell — it is not a refusal here", c.name, c.w, c.h)
		}
		if e.note == "" {
			t.Errorf("%s %dx%d: `enter` answers nothing", c.name, c.w, c.h)
		}
		// The search acts from this very stand.
		s, _ := stand(c.mk, c.w, c.h, append(append([]string(nil), c.keys...), "/"))
		if body(s) == before && footer(s) == footer(m) {
			t.Errorf("%s %dx%d: `/` changes nothing — it is not a key that acts here", c.name, c.w, c.h)
		}
		foot := footer(m)
		if strings.Contains(foot, "enter · no pane") {
			t.Errorf("%s %dx%d: the row holds eighteen cells for a key that cannot work: %q",
				c.name, c.w, c.h, foot)
		}
		if !strings.Contains(foot, "/ search") {
			t.Errorf("%s %dx%d: the row names no way to search: %q", c.name, c.w, c.h, foot)
		}
		for _, k := range c.kept {
			if !strings.Contains(foot, k) {
				t.Errorf("%s %dx%d: the trade cost the row %q: %q", c.name, c.w, c.h, k, foot)
			}
		}
		if w := lipgloss.Width(foot); w > c.w-1 {
			t.Errorf("%s %dx%d: the footer overruns its field: %d cells", c.name, c.w, c.h, w)
		}
		_ = sc
	}

	// The rank, stated, at all three levels the refusal is offered at:
	// the refusal sheds before `/ search` and before `n/N`.
	for _, lv := range []struct {
		name string
		keys []string
		w, h int
	}{
		{"the archive's list", []string{"A"}, 80, 24},
		{"the waypoints", []string{"A", "8", "tab"}, 100, 30},
		{"the reader", []string{"A", "8", "tab", "tab"}, 100, 30},
	} {
		m, _ := stand(sceneSecondDay, lv.w, lv.h, lv.keys)
		order := m.shedOrder(false)
		at := func(frag string) int {
			for i, f := range order {
				if f == frag {
					return i
				}
			}
			return -1
		}
		refusal, search, walk := at(" · enter · no pane"), at(" · / search"), at(" · n/N")
		if refusal < 0 || search < 0 || walk < 0 {
			t.Fatalf("%s: shed order names %d/%d/%d", lv.name, refusal, search, walk)
		}
		if refusal > search || refusal > walk {
			t.Errorf("%s sheds a key that acts before the attach refusal: refusal %d, / search %d, n/N %d",
				lv.name, refusal, search, walk)
		}
	}

	// The other side. Where the row's own `enter` attaches, the keymap is
	// unchanged: the live list at eighty still names `enter attach`.
	m, _ := stand(sceneSecondDay, 80, 24, nil)
	if foot := footer(m); !strings.Contains(foot, "enter attach") {
		t.Errorf("the live list lost its attach key: %q", foot)
	}
	// And where the field holds both, the refusal stays: the same archive
	// list at 120 names the refusal and the search.
	w120, _ := stand(sceneSecondDay, 120, 34, []string{"A"})
	foot := footer(w120)
	if !strings.Contains(foot, "enter · no pane") || !strings.Contains(foot, "/ search") {
		t.Errorf("at 120 the archive's row should name both: %q", foot)
	}
}

// r84fhListStand replays the first n keys of a scene's own walkthrough.
func r84fhListStand(sc scene, w, h, n int) *Model {
	m := sceneModel(sc, w, h)
	keys := append(append([]string{}, canonicalKeys...), "esc")
	keys = append(keys, sc.extra...)
	for _, k := range keys[:n] {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// r84fhFooterRow is the last drawn row of a frame, with the colour off.
func r84fhFooterRow(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	for i := len(rows) - 1; i >= 0; i-- {
		if strings.TrimSpace(rows[i]) != "" {
			return rows[i]
		}
	}
	return ""
}

// TestTheAttachRefusalYieldsToTheNoteThatSaysIt pins round eighty-four's
// second finding. On a paneless session the footer stands with
// `enter · no pane` — what Enter does here, since an attach that cannot
// work is not promised (#40). Press Enter and the note is `attach needs a
// pane`: the same sentence, twelve cells to the right, and the row pays
// for the repetition with keys that act — `/ search` and `x hide` at
// eighty, `a ask` at a hundred. #165 gave the note the key's own name, in
// the form `mirror needs 110 columns` already used, precisely so naming
// the key would buy a key back; the clause beside it buys nothing. A
// refusal goes before a key that acts (#52, #198, #206), so under the note
// that says it the clause is shed first — and, like every yield, only
// where a key that acts comes back (#193, #216).
func TestTheAttachRefusalYieldsToTheNoteThatSaysIt(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // the frame is the one a person sees (#215, #218)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	sc := sceneFleetHygiene()
	for _, c := range []struct {
		w, h  int
		gains string
	}{
		{80, 24, "x hide"},
		{100, 30, "a ask"},
	} {
		// The walkthrough's own stand on the paneless session, one key
		// before its `enter`.
		m := r84fhListStand(sc, c.w, c.h, 28)
		if m.level != levelTrail || !strings.Contains(r84fhFooterRow(m), "enter · no pane") {
			t.Fatalf("%dx%d: not the paneless list stand: %q", c.w, c.h, r84fhFooterRow(m))
		}
		pressKey(m, "enter")
		poll(m, sc)
		foot := r84fhFooterRow(m)
		if m.note != "attach needs a pane" {
			t.Fatalf("%dx%d: enter said %q", c.w, c.h, m.note)
		}
		if strings.Contains(foot, "enter · no pane") {
			t.Errorf("%dx%d: the row says the refusal twice — %q beside `enter · no pane`:\n%s", c.w, c.h, m.note, foot)
		}
		if !strings.Contains(foot, c.gains) {
			t.Errorf("%dx%d: the cells bought back no key that acts (wanted %q):\n%s", c.w, c.h, c.gains, foot)
		}
		if !strings.Contains(foot, "attach needs a pane") {
			t.Errorf("%dx%d: the refusal went and the note did not stand:\n%s", c.w, c.h, foot)
		}
	}
}

// TestTheAttachRefusalStandsWithNoNoteToSayIt is the other side of the same
// rule, and the reason the clause exists: with no note on the row — and
// under a note about another key that says nothing about the pane — the
// paneless session's footer still says what Enter does. Only a note that
// says what the clause says takes its cells, and the reply refusal is the
// other note that does (#165, #232, #299): `reply needs a pane` and `enter
// · no pane` are one fact — this session has no pane — so the clause goes
// there too, and a chapter key's refusal, which never mentions the pane,
// leaves it standing.
func TestTheAttachRefusalStandsWithNoNoteToSayIt(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	sc := sceneFleetHygiene()
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		w, h := size[0], size[1]
		m := r84fhListStand(sc, w, h, 28)
		if m.level != levelTrail {
			continue // the wide decks stand on the board here, with no list row
		}
		if !strings.Contains(r84fhFooterRow(m), "enter · no pane") {
			t.Errorf("%dx%d: with no note the paneless row does not say what enter does:\n%s", w, h, r84fhFooterRow(m))
		}
		stand := r84fhListStand(sc, w, h, 28)
		pressKey(stand, "[") // another key's refusal, and not about the pane
		poll(stand, sc)
		if !strings.HasPrefix(stand.note, "no earlier") {
			t.Fatalf("%dx%d: [ said %q", w, h, stand.note)
		}
		if !strings.Contains(r84fhFooterRow(stand), "enter · no pane") {
			t.Errorf("%dx%d: another key's note took the attach refusal's cells:\n%s", w, h, r84fhFooterRow(stand))
		}
		pressKey(m, "r") // the other no-pane refusal: it says what the clause says
		poll(m, sc)
		if m.note != "reply needs a pane" {
			t.Fatalf("%dx%d: r said %q", w, h, m.note)
		}
		if strings.Contains(r84fhFooterRow(m), "enter · no pane") {
			t.Errorf("%dx%d: the row says the same fact twice — %q beside `enter · no pane`:\n%s", w, h, m.note, r84fhFooterRow(m))
		}
	}
}

// r90sdReplyCardRow is the reply card's title row, the one that names the
// session the typed line will go to.
func r90sdReplyCardRow(m *Model) string {
	for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(row, "reply to ") {
			return row
		}
	}
	return ""
}

// TestTheReplyCardKeepsTheDigitOfTheLiveRowItTypesInto pins round ninety's
// second-day second finding.
//
// #31 put the row's digit on the reply card because a card naming a session
// by board position stood over a list where that position was another
// session. The guard was `d > 0 && !m.archiveView` — the same guard #248
// took off the header: in the archive the numbers are the archive's own
// (#32), but an archive drawing no row claims no number, and the live
// session that frame still selects, draws the trail of and now offers both
// writes for wears the digit its own header draws (#248) and its own refusal
// calls it by (#242). The card three rows under that header said `reply to
// hello` while the header said `1 hello`.
func TestTheReplyCardKeepsTheDigitOfTheLiveRowItTypesInto(t *testing.T) {
	forceASCII(t)
	stands := 0
	for _, sc := range []scene{sceneSecondDay(), sceneManyIdle(), sceneFewOngoing()} {
		for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				old := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(prof) // the frame a person sees (#215, #218)
				m := sceneModel(sc, wh[0], wh[1])
				for _, k := range []string{"/", "pytest", "enter", "A", "r"} {
					pressKey(m, k)
					poll(m, sc)
				}
				s, ok := m.selected()
				if !ok || !m.archiveView || len(m.viewOrder()) != 0 || !s.Live || !m.replying {
					lipgloss.SetColorProfile(old)
					continue
				}
				d := m.digits[s.Info.Key()]
				if d < 1 || d > 9 {
					lipgloss.SetColorProfile(old)
					continue
				}
				stands++
				card := r90sdReplyCardRow(m)
				want := "reply to " + strconv.Itoa(d) + " · " + sessionName(s.Info)
				if !strings.Contains(card, want) {
					t.Errorf("%s %dx%d (%v): the reply card dropped the live row's digit: want %q in %q",
						sc.name, wh[0], wh[1], prof, want, card)
				}
				// The header of the same frame says it too, and the card
				// still fits its terminal.
				head := ansi.Strip(strings.SplitN(m.View(), "\n", 2)[0])
				if !strings.Contains(head, fmt.Sprintf("%d %s", d, sessionName(s.Info))) {
					t.Errorf("%s %dx%d (%v): #248's header is not standing: %q", sc.name, wh[0], wh[1], prof, head)
				}
				for _, row := range strings.Split(m.View(), "\n") {
					if x := lipgloss.Width(row); x > wh[0] {
						t.Errorf("%s %dx%d (%v): a row runs past the terminal (%d): %q",
							sc.name, wh[0], wh[1], prof, x, ansi.Strip(row))
					}
				}
				lipgloss.SetColorProfile(old)
			}
		}
	}
	if stands == 0 {
		t.Fatal("no empty archive opened a reply card on a live row: the pin is not standing on its frame")
	}
}

// ---- round 95, two-tools ----
// slivers reports, for one board frame with the reply box on it, the cells
// standing between a board rail and the box's left edge where that gap is
// too narrow for a row to say anything — the width `panelHides` already
// calls the box beginning inside a row's own prefix. The leftmost rail
// whose tail is that narrow is the column's own; a rail inside the
// fragment is a card's continuation and belongs to the fragment.
func slivers(frame string) (frags []string, edges int) {
	rows := strings.Split(ansi.Strip(frame), "\n")
	if len(rows) == 0 || !strings.Contains(rows[0], " · board") {
		return nil, 0
	}
	// Rune counts, not byte offsets: every glyph on these rows is one cell.
	at := func(row, glyph string) int {
		i := strings.Index(row, glyph)
		if i < 0 {
			return -1
		}
		return len([]rune(row[:i]))
	}
	left, top, bot := -1, -1, -1
	for i, r := range rows {
		if strings.Contains(r, "┌ reply to ") {
			left, top = at(r, "┌"), i
		}
		if left >= 0 && at(r, "└") == left {
			bot = i
		}
	}
	if left < 1 || top < 0 {
		return nil, 0
	}
	if bot < 0 {
		bot = len(rows) - 1
	}
	for i := top; i <= bot; i++ {
		head := []rune(rows[i])
		if len(head) < left {
			continue
		}
		// The box is drawn one cell right of the row's own last cell.
		line := []rune(string(head[:left-1]))
		for j := 0; j < len(line); {
			k := -1
			for x := j; x < len(line); x++ {
				if line[x] == '│' {
					k = x
					break
				}
			}
			if k < 0 {
				break
			}
			j = k + 1
			gap := len(line[j:])
			if gap <= 1 {
				// The box begins at the column's own edge: the gutter
				// alone stands between, and it carries the mark (#62).
				if strings.TrimSpace(string(line[j:])) == "…" {
					edges++
				}
				break
			}
			if gap <= trailPrefixWidth {
				frags = append(frags, string(line[j:]))
				break
			}
		}
	}
	return frags, edges
}

// TestTheReplyBoxLeavesNoSliverOfAColumn: where the reply box begins inside
// a board column's own prefix, the cells it leaves cannot be a row — they
// are a fragment of one cut at the border. On alarm-storm at 152 the cut
// fell inside an opening quote (`◉ "…`), on subagents at 120 it left `◆ t…`
// for a leg with no label, and on 196 rows it left blanks under a mark,
// which is what #126 composes a column at the box's width to stop. The
// fragment goes blank and the mark goes with it. Held on the other side
// too: where the box begins at a column's own edge the mark still stands
// (#62, #64).
func TestTheReplyBoxLeavesNoSliverOfAColumn(t *testing.T) {
	forceASCII(t)
	keys := []string{"r", "esc", "l", "r", "esc", "l", "r", "esc", "l", "r", "esc", "l", "r", "esc", "l", "r"}
	sizes := map[string][][2]int{
		"alarm-storm": {{120, 34}, {152, 40}},
		"few-ongoing": {{120, 34}, {152, 40}},
		"many-idle":   {{120, 34}, {152, 40}},
		"subagents":   {{120, 34}},
		"very-long":   {{120, 34}},
	}
	stands, edges := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range sizes[sc.name] {
				m := sceneModel(sc, size[0], size[1])
				for ki, k := range keys {
					pressKey(m, k)
					poll(m, sc)
					frags, e := slivers(m.View())
					edges += e
					for _, f := range frags {
						stands++
						if strings.TrimSpace(f) != "" {
							t.Errorf("%s %dx%d prof=%v after %d keys: the reply box left a sliver of a column: %q",
								sc.name, size[0], size[1], prof, ki+1, f)
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if stands == 0 {
		t.Fatalf("vacuous: no frame put the reply box inside a column's prefix")
	}
	if edges == 0 {
		t.Fatalf("vacuous: no frame put the reply box at a column's own edge")
	}
	t.Logf("sliver stands %d, box-at-edge rows %d", stands, edges)
}

// TestTheCutAtTheReplyBoxsEdgeIsAClip: the border the reply box cuts a row
// at is a clip like any other. `truncateWhole` knew only a number and a
// separator, so at 152 on subagents the board drew
// `● scout  Red-teaming plugin/…` — a mark after a slash, reading as more
// of the path it cut — where `clip` has dropped the trailing dot, slash,
// bracket, comma and unclosed quote since #58, #61, #63, #64 and #80.
func TestTheCutAtTheReplyBoxsEdgeIsAClip(t *testing.T) {
	forceASCII(t)
	keys := []string{"r", "esc", "l", "r", "esc", "l", "r", "esc", "l", "r", "esc", "l", "r", "esc", "l", "r"}
	sizes := map[string][][2]int{
		"alarm-storm": {{120, 34}, {152, 40}},
		"few-ongoing": {{120, 34}, {152, 40}},
		"many-idle":   {{120, 34}, {152, 40}},
		"subagents":   {{120, 34}, {152, 40}},
		"very-long":   {{120, 34}, {152, 40}},
	}
	marks := 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range sizes[sc.name] {
				m := sceneModel(sc, size[0], size[1])
				for ki, k := range keys {
					pressKey(m, k)
					poll(m, sc)
					rows := strings.Split(ansi.Strip(m.View()), "\n")
					if len(rows) == 0 || !strings.Contains(rows[0], " · board") {
						continue
					}
					left := -1
					for _, r := range rows {
						if i := strings.Index(r, "┌ reply to "); i >= 0 {
							left = len([]rune(r[:i]))
						}
					}
					if left < 3 {
						continue
					}
					for _, r := range rows {
						run := []rune(r)
						// The mark stands one cell left of the box (#62).
						if len(run) < left-1 || run[left-2] != '…' {
							continue
						}
						marks++
						head := strings.TrimRight(string(run[:left-2]), " ")
						if head == "" {
							continue
						}
						last := []rune(head)[len([]rune(head))-1]
						if strings.ContainsRune("(./—–,;", last) {
							t.Errorf("%s %dx%d prof=%v after %d keys: the box's border cut after %q: %q",
								sc.name, size[0], size[1], prof, ki+1, string(last), head)
						}
						if last == '"' && strings.Count(head, `"`)%2 == 1 {
							t.Errorf("%s %dx%d prof=%v after %d keys: the box's border cut inside an opening quote: %q",
								sc.name, size[0], size[1], prof, ki+1, head)
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if marks == 0 {
		t.Fatalf("vacuous: no frame drew the box's border mark")
	}
	t.Logf("border marks examined: %d", marks)
}

// ---- round 105, second-day ----
// TestTheNoPaneRefusalKeepsTheWayDeeper holds the no-pane refusal to the
// rule #165 gave it: it wears the naming form `attach needs a pane` /
// `reply needs a pane` "precisely so that naming the key would buy a key
// back", and where the naming buys nothing it costs cells instead. In the
// archive's own session view at eighty — `second-day`, `A` then `tab`, the
// frame one keypress deeper than the list #264 measured — the nineteen
// cells took `tab deeper` off the row, the frame's only naming of the way
// deeper, for a key that moved nothing; the same refusal on the archive's
// list one level out keeps the key. The note there is `no pane`, seven
// cells, the word the row's own `enter · no pane` clause already uses.
//
// Three sides: the frame it was found on, both keys; the rule over every
// scene, five widths, two routes and both profiles — wherever a no-pane
// refusal is drawn, the row keeps a key naming a level if the row one
// keypress earlier had one; and the naming form still stands where it
// costs nothing, on the canonical walkthrough that carries it.
func TestTheNoPaneRefusalKeepsTheWayDeeper(t *testing.T) {
	sweep(t)
	forceASCII(t)
	r105sdLevelKey := func(row string) bool {
		for _, k := range []string{"enter attach", "tab deeper", "tab session", "tab reader"} {
			if strings.Contains(row, k) {
				return true
			}
		}
		return false
	}
	r105sdFrame := func(sc scene, w, h int, keys []string) (*Model, []string) {
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		return m, strings.Split(ansi.Strip(m.View()), "\n")
	}
	r105sdScene := func(name string) scene {
		for _, sc := range allScenes() {
			if sc.name == name {
				return sc
			}
		}
		t.Fatalf("no scene %q", name)
		return scene{}
	}

	// The frame it was found on: the archive's own session view at eighty.
	sd := r105sdScene("second-day")
	for _, key := range []string{"enter", "r"} {
		_, rowsBefore := r105sdFrame(sd, 80, 24, []string{"A", "tab"})
		if got := strings.TrimRight(rowsBefore[len(rowsBefore)-1], " "); !strings.Contains(got, "tab deeper") {
			t.Fatalf("second-day 80x24 [A tab]: the row before %q names no way deeper: %q", key, got)
		}
		m, rows := r105sdFrame(sd, 80, 24, []string{"A", "tab", key})
		foot := strings.TrimRight(rows[len(rows)-1], " ")
		for i := range rows[:len(rows)-1] {
			if strings.TrimRight(rows[i], " ") != strings.TrimRight(rowsBefore[i], " ") {
				t.Errorf("second-day 80x24: %q moved row %d: %q -> %q", key, i, rowsBefore[i], rows[i])
			}
		}
		if !strings.HasSuffix(foot, "no pane") || strings.Contains(foot, "needs a pane") {
			t.Errorf("second-day 80x24 [A tab %s]: the refusal the row draws is %q, want it to end in %q", key, foot, "no pane")
		}
		if !strings.HasSuffix(m.note, " needs a pane") {
			t.Errorf("second-day 80x24 [A tab %s]: the refusal set is %q: the note itself is unchanged, only the form the row draws", key, m.note)
		}
		if !strings.Contains(foot, "tab deeper") {
			t.Errorf("second-day 80x24 [A tab %s]: the row lost the way deeper: %q", key, foot)
		}
		if lipgloss.Width(foot) > 80 {
			t.Errorf("second-day 80x24 [A tab %s]: the row runs past the terminal (%d): %q", key, lipgloss.Width(foot), foot)
		}
	}

	// The rule, over every scene at five widths under both profiles.
	routes := [][]string{{"A", "tab"}, {"A"}}
	stands, refusals := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		if prof == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				for _, route := range routes {
					for _, key := range []string{"enter", "r"} {
						stands++
						m := sceneModel(sc, size[0], size[1])
						for _, k := range route {
							pressKey(m, k)
							poll(m, sc)
						}
						rowsBefore := strings.Split(ansi.Strip(m.View()), "\n")
						was := rowsBefore[len(rowsBefore)-1]
						pressKey(m, key)
						poll(m, sc)
						rows := strings.Split(ansi.Strip(m.View()), "\n")
						foot := rows[len(rows)-1]
						if m.note != "no pane" && !strings.HasSuffix(m.note, " needs a pane") {
							continue
						}
						refusals++
						if r105sdLevelKey(was) && !r105sdLevelKey(foot) {
							t.Errorf("%v %s %dx%d %v then %q: the no-pane refusal %q costs the row its only naming of a level: %q -> %q",
								prof, sc.name, size[0], size[1], route, key, m.note, strings.TrimRight(was, " "), strings.TrimRight(foot, " "))
						}
						if lipgloss.Width(foot) > size[0] {
							t.Errorf("%v %s %dx%d %v then %q: the row runs past the terminal (%d): %q",
								prof, sc.name, size[0], size[1], route, key, lipgloss.Width(foot), foot)
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if refusals < 50 { // half of the two-profile floor: the sweep walks one profile (#324)
		t.Fatalf("the rule reached only %d no-pane refusals over %d stands: it has gone vacuous", refusals, stands)
	}

	// And the naming form stands where it costs no key: the canonical
	// walkthrough that draws both (#165, #232).
	walk := walkthrough(r105sdScene("fleet-hygiene"), 80, 24, canonicalKeys)
	for _, said := range []string{"attach needs a pane", "reply needs a pane"} {
		if !strings.Contains(walk, said) {
			t.Errorf("fleet-hygiene 80x24: the walkthrough no longer says %q: the naming form yielded where it cost no key", said)
		}
	}
}

// ---- round 105, fleet-hygiene ----
// r105fhAttachNote is the refusal `enter` draws where the selected session
// has no pane (#165), and r105fhPaneClause is the keymap's own clause for
// the same key on the same row. They are one sentence: the note names the
// key the clause names and says what the clause says.
const (
	r105fhAttachNote  = "attach needs a pane"
	r105fhPaneClause  = "enter · no pane"
	r105fhSearchKey   = "/ search"
	r105fhTypedFooter = "esc cancels"
)

// r105fhRoute is one walk to a frame whose `enter` cannot attach.
type r105fhRoute struct {
	keys []string
	what string
}

// r105fhWalks are the ways to a paneless row: the archive at its list, its
// board and its session view — every archived session is paneless — and,
// on the fleets that have one, the live session with no pane.
var r105fhWalks = []r105fhRoute{
	{[]string{"A"}, "archive list"},
	{[]string{"A", "shift+tab"}, "archive board"},
	{[]string{"A", "tab"}, "archive session view"},
	{[]string{"3"}, "live list, the third session"},
}

// r105fhFooter is the last drawn row of a frame, colour stripped.
func r105fhFooter(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return strings.TrimSpace(rows[len(rows)-1])
}

// TestTheAttachRefusalSaysTheNoPaneOnce holds the attach refusal to the
// rule every other refusal on the deck keeps: the row says the sentence
// once. `enter` on a paneless row draws `attach needs a pane`, nineteen
// cells naming the key; where the same row also draws its own
// `enter · no pane` the frame says one thing twice, and on the archive's
// session view at 152 the second saying costs `/ search`, a key that acts
// on that very row (#95, #96, #165, #264). Three sides: under the note the
// clause is gone; the note still stands, so the frame says why; and the
// walk actually reaches such frames, so a yield that never draws the note
// fails too.
func TestTheAttachRefusalSaysTheNoPaneOnce(t *testing.T) {
	sweep(t)
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	scenes := allScenes()
	reached, twice := 0, 0
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"mono", termenv.Ascii}, {"colour", termenv.TrueColor}} {
		if prof.p == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range scenes {
			for _, size := range [][2]int{{80, 24}, {120, 34}, {152, 40}, {220, 48}} {
				for _, walk := range r105fhWalks {
					m := sceneModel(sc, size[0], size[1])
					for _, k := range walk.keys {
						pressKey(m, k)
						poll(m, sc)
					}
					before := r105fhFooter(m)
					if strings.Contains(before, r105fhTypedFooter) {
						continue // a line has the keyboard, not the deck
					}
					pressKey(m, "enter")
					poll(m, sc)
					foot := r105fhFooter(m)
					if !strings.Contains(foot, r105fhAttachNote) {
						continue // this row could be attached
					}
					reached++
					if strings.Contains(foot, r105fhPaneClause) {
						twice++
						t.Errorf("%s %s %dx%d %s: the row says the same sentence twice — %q beside %q\n   foot=%q",
							prof.name, sc.name, size[0], size[1], walk.what,
							r105fhPaneClause, r105fhAttachNote, foot)
					}
					if strings.Contains(before, r105fhSearchKey) && !strings.Contains(foot, r105fhSearchKey) &&
						size[0] >= 152 {
						t.Errorf("%s %s %dx%d %s: the refusal cost the row %q, which the row named one keypress earlier\n   was =%q\n   now =%q",
							prof.name, sc.name, size[0], size[1], walk.what, r105fhSearchKey, before, foot)
					}
				}
			}
		}
	}
	if reached == 0 {
		t.Fatalf("the walk reached no frame drawing %q", r105fhAttachNote)
	}
	t.Logf("frames drawing the refusal: %d, of which saying it twice: %d", reached, twice)
}

// ---- round 106, fleet-hygiene ----
// r106fhReplyNote is the refusal `r` draws where the selected session has no
// pane (#165), and r106fhClause is the keymap's own clause for the same
// fact: the session has no pane, which is why neither key can work. A row
// drawing both says it twice, and #232 and #299 took the attach half of it.
const (
	r106fhReplyNote = "reply needs a pane"
	r106fhClause    = "enter · no pane"
	r106fhSearch    = "/ search"
	r106fhTyping    = "esc cancels"
)

// r106fhWalk is one way to a row with no pane and what to call it.
type r106fhWalk struct {
	keys []string
	what string
}

// r106fhWalks are the ways to a paneless row: the archive at its list, its
// board and its own session view, and the live list's third session — the
// fleet-hygiene fleet's session with no pane. They are #299's four, one
// level out from the session view, whose own row keeps every key it names
// and says the fact twice for them (recorded, not folded).
var r106fhWalks = []r106fhWalk{
	{[]string{"A"}, "archive list"},
	{[]string{"A", "shift+tab"}, "archive board"},
	{[]string{"A", "tab"}, "archive session view"},
	{[]string{"3"}, "live list, the third session"},
}

// r106fhRow is the last drawn row of a frame, colour stripped.
func r106fhRow(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return strings.TrimSpace(rows[len(rows)-1])
}

// r106fhNamed is the keys a drawn footer names: the row up to the gap the
// keymap never contains, split on its own separator.
func r106fhNamed(row string) map[string]bool {
	if i := strings.Index(row, "  "); i >= 0 {
		row = row[:i]
	}
	out := map[string]bool{}
	for _, frag := range strings.Split(row, " · ") {
		if f := strings.TrimSpace(frag); f != "" {
			out[f] = true
		}
	}
	return out
}

// TestTheReplyRefusalSaysTheNoPaneOnce holds three sides of the reply
// refusal on a row with no pane, at four widths under both colour profiles
// (#215, #218): the row under the note does not also draw the keymap's own
// `enter · no pane`, since the note says the same fact (#95, #96, #165,
// #232, #299); at 152 and wider the refusal does not cost the row the search
// key it named one keypress earlier (#264); and the walk reaches the refusal
// at all, so a yield that stopped drawing the note fails too.
func TestTheReplyRefusalSaysTheNoPaneOnce(t *testing.T) {
	sweep(t)
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	reached := 0
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"mono", termenv.Ascii}, {"colour", termenv.TrueColor}} {
		if prof.p == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {120, 34}, {152, 40}, {220, 48}} {
				for _, walk := range r106fhWalks {
					m := sceneModel(sc, size[0], size[1])
					for _, k := range walk.keys {
						pressKey(m, k)
						poll(m, sc)
					}
					before := r106fhRow(m)
					if strings.Contains(before, r106fhTyping) {
						continue // a line has the keyboard, not the deck
					}
					pressKey(m, "r")
					poll(m, sc)
					row := r106fhRow(m)
					if !strings.Contains(row, r106fhReplyNote) {
						continue // this row could be replied to
					}
					reached++
					if strings.Contains(row, r106fhClause) {
						t.Errorf("%s %s %dx%d %s: the row says the same fact twice — %q beside %q\n   row=%q",
							prof.name, sc.name, size[0], size[1], walk.what,
							r106fhClause, r106fhReplyNote, row)
					}
					// At 152 and wider the clause's own cells are the
					// search key's twice over, so the refusal never costs
					// the key the row named one keypress earlier (#264).
					// Narrower the row is full and the note pays (#175).
					if size[0] < 152 {
						continue
					}
					was, now := r106fhNamed(before), r106fhNamed(row)
					if was[r106fhSearch] && !now[r106fhSearch] {
						t.Errorf("%s %s %dx%d %s: the refusal cost the row %q, which the row named one keypress earlier\n   was=%q\n   now=%q",
							prof.name, sc.name, size[0], size[1], walk.what, r106fhSearch, before, row)
					}
				}
			}
		}
	}
	if reached == 0 {
		t.Fatalf("the walk reached no frame drawing %q", r106fhReplyNote)
	}
	t.Logf("frames drawing the reply refusal: %d", reached)
}
