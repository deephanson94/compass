package ui

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// ---- round 107, fleet-hygiene ----
// r107fhCursorRoutes are the ways into the reader this pin walks: the live
// fleet's first session, the archive's, the second and third rows of the
// board — every one of them ends on a conversation whose prose the model
// wrote flush left, which is the row the cursor mark has no spare cell on.
var r107fhCursorRoutes = [][]string{
	{"tab", "tab"},
	{"A", "tab", "tab"},
	{"j", "tab", "tab"},
}

// r107fhDrawnCursorRow is the row RenderReader draws the cursor on: the one
// line of the panel carrying the mark. "" when the panel draws none.
func r107fhDrawnCursorRow(panel string) string {
	for _, line := range strings.Split(panel, "\n") {
		if strings.Contains(line, "▸") {
			return strings.TrimRight(ansi.Strip(line), " ")
		}
	}
	return ""
}

// TestTheReaderCursorSaysWhenItTakesACell holds the reader's cursor to the
// deck's own truncation discipline (SPEC §4): the mark may take a cell of a
// row that has no spare one, but a cell taken from the row's own words is a
// cut, and every cut in the reader carries the mark that says so.
//
// At HEAD the mark was pushed in front of a flush-left prose row and one cell
// came off the far end in silence: on `very-long` at 100 and 120 the canonical
// walkthrough drew "…the current shape" for "…the current shape;" and
// "…touches every caller" for "…touches every caller.", and on `many-idle` at
// 120 two presses of `j` drew "I'll take the narrower on" for "…the narrower
// one" — a different sentence, with nothing on the frame to say a letter had
// been taken.
//
// Three sides, under both colour profiles (#215, #218): the drawn cursor row
// carries every word the unmarked row carried, or else ends in the cut mark;
// the row never grows past the panel; and the walk actually reaches rows whose
// mark has to be pushed in front, so a reader that stopped drawing prose fails
// here too.
func TestTheReaderCursorSaysWhenItTakesACell(t *testing.T) {
	pushed := 0
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"mono", termenv.Ascii}, {"colour", termenv.TrueColor}} {
		prev := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := size[0], size[1]
				for _, route := range r107fhCursorRoutes {
					m := sceneModel(sc, w, h)
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					if m.level != levelReader {
						continue
					}
					for step := 0; step < 120; step++ {
						opts := ReaderOpts{Width: m.readerWidth(), Height: m.readerHeight(),
							Scroll: m.scroll, Anchor: m.anchor, Unfolded: m.unfolded,
							CWD: m.readerCWD(), Now: m.now, Lanes: m.laneClauses()}
						doc := m.doc(opts.Width)
						i := m.anchor
						if i < 0 || i >= len(doc) {
							break
						}
						words := strings.TrimRight(ansi.Strip(doc[i].render("")), " ")
						// Only the rows with no spare cell are this pin's:
						// where the mark takes an indent or the space after
						// a glyph, nothing of the row's own words moves.
						if r := []rune(words); len(r) > 1 && r[1] != ' ' && !strings.HasPrefix(words, "▸") {
							pushed++
							row := r107fhDrawnCursorRow(RenderReader(m.events, opts))
							if row != "" && !strings.Contains(row, words) && !strings.HasSuffix(row, "…") {
								t.Errorf("%s %s %dx%d %v +%d j: the cursor took a cell of the row's own words and said nothing\n  the row  =%q\n  the cursor drew=%q",
									prof.name, sc.name, w, h, route, step, words, row)
							}
							if lipgloss.Width(row) > opts.Width {
								t.Errorf("%s %s %dx%d %v +%d j: the cursor row runs past the panel (%d of %d): %q",
									prof.name, sc.name, w, h, route, step, lipgloss.Width(row), opts.Width, row)
							}
						}
						was := m.anchor
						pressKey(m, "j")
						poll(m, sc)
						if m.anchor == was {
							break
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(prev)
	}
	if pushed == 0 {
		t.Fatalf("the walk never reached a row whose mark has to be pushed in front: nothing was measured")
	}
	t.Logf("rows walked whose mark is pushed in front of the row's own words: %d", pushed)
}

// ---- round 107, two-tools ----
// r107ttCurSizes are the five terminals the walkthrough is drawn at.
var r107ttCurSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r107ttCurRoutes are ways down to a reader page: straight in, in on
// another row, and the chapter and cursor keys pressed once there, so both
// a page that fits and a page that scrolls are stood on.
var r107ttCurRoutes = [][]string{
	{"tab", "tab"},
	{"tab", "tab", "]"},
	{"tab", "tab", "j"},
	{"1", "tab", "tab"},
}

const (
	// The reader's two names for the same pair of keys: what they do on a
	// page that scrolls, and what they do on one that fits (#83, #300).
	r107ttCurScroll = "j/k scroll"
	r107ttCurRows   = "j/k rows"
	r107ttCurUnfold = "space unfold"
	r107ttCurPage   = "ctrl+d/u half page"
	// The room the clause needs, with margin: `j/k rows · ` is eleven
	// cells, and the narrowest gap the deck draws between the keymap and
	// the note is two, so a row with this much between them takes the
	// clause without shedding anything.
	r107ttCurRoom = 24
)

// r107ttCurKeys is the keymap half of a footer: what stands before the gap
// the note lives after (#134's reserve), with the attach aside — which is
// not a key (#55) — read past.
func r107ttCurKeys(foot string) string {
	s := strings.TrimLeft(strings.TrimRight(ansi.Strip(foot), " "), " ")
	if i := strings.Index(s, "  "); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// r107ttCurNote is the note half of the same row, or "" where the row draws
// none.
func r107ttCurNote(foot string) string {
	s := strings.TrimLeft(strings.TrimRight(ansi.Strip(foot), " "), " ")
	if i := strings.Index(s, "  "); i >= 0 {
		return strings.TrimSpace(s[i:])
	}
	return ""
}

// r107ttCurGap is the free cells between the keymap and the note on a row
// drawn `inner` cells wide: the room a new clause has to come out of.
func r107ttCurGap(foot string, inner int) int {
	return inner - lipgloss.Width(r107ttCurKeys(foot)) - lipgloss.Width(r107ttCurNote(foot))
}

// TestTheReadersFittingPageNamesTheCursorKeys pins the reader's movement
// keys to what they do on a page that is all on screen.
//
// #83 shed `j/k scroll` and `ctrl+d/u half page` from such a page because
// "the keys that move the viewport move nothing" — true while the reader
// had no cursor. #300 gave it one and drew it: on a fitting page `j` and `k`
// walk the `▸` a row at a time and `space` unfolds the row it stands on,
// so a second fold on that one screen is reachable by these keys and by
// nothing else. The row went on naming `space unfold` and naming no key
// that reaches it. #220 settled the words one level out, where the same
// keys move the cursor and not the viewport: `j/k rows`.
//
// Three sides, so the words cannot be jammed onto every page:
//   - a fitting reader page whose row has the room names `j/k rows`;
//   - a page that scrolls names `j/k rows` too and never says `j/k scroll`
//     (#307: the key walks the mark there and moves the viewport only
//     behind it), and a page that fits still offers no `ctrl+d/u half
//     page` — the shortcut #200 shed and the help teaches;
//   - the clause costs nothing: the finished row names every key the same
//     row names without it, and no row runs past its terminal.
func TestTheReadersFittingPageNamesTheCursorKeys(t *testing.T) {
	forceASCII(t)
	roomy, scrolls := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range r107ttCurSizes {
				w, h := size[0], size[1]
				inner := w - 2
				for _, route := range r107ttCurRoutes {
					m := sceneModel(sc, w, h)
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					if m.showHelp || m.searching || m.replying || m.level < levelReader {
						continue
					}
					rows := strings.Split(ansi.Strip(m.View()), "\n")
					if len(rows) == 0 {
						continue
					}
					foot := rows[len(rows)-1]
					keys := r107ttCurKeys(foot)
					where := fmt.Sprintf("%s %v %dx%d Lv%d", sc.name, route, w, h, m.level)

					fits := len(m.doc(m.readerWidth())) <= m.readerHeight()
					if !fits {
						// The page scrolls: the pair still walks the mark a
						// row at a time and moves the viewport only when the
						// mark would leave it (#300, #307), so the row names
						// what the keys move here as well.
						scrolls++
						if !strings.Contains(keys, r107ttCurRows) && lipgloss.Width(keys) < inner-lipgloss.Width(r107ttCurRows+" · ") {
							t.Errorf("%s: a reader page that scrolls names neither movement key on a row with the room\n  foot=%q", where, keys)
						}
						if strings.Contains(keys, r107ttCurScroll) {
							t.Errorf("%s: a reader page that scrolls says %q; the keys walk rows there too (#307)\n  foot=%q", where, r107ttCurScroll, keys)
						}
						continue
					}
					if strings.Contains(keys, r107ttCurScroll) {
						t.Errorf("%s: a reader page all on screen says %q; nothing scrolls there (#83)\n  foot=%q", where, r107ttCurScroll, keys)
					}
					// The page key stays shed here: a shortcut for a
					// distance `j` covers, the first thing this row gives
					// up and a key the help teaches (#42, #51, #200).
					if strings.Contains(keys, r107ttCurPage) {
						t.Errorf("%s: a reader page all on screen offers %q (#200)\n  foot=%q", where, r107ttCurPage, keys)
					}
					// A lane's page with no turns offers only the way out
					// (#56): it names no unfold key either, and no
					// movement key is owed on it.
					if !strings.Contains(keys, r107ttCurUnfold) {
						continue
					}
					if r107ttCurGap(foot, inner) >= r107ttCurRoom {
						roomy++
						if !strings.Contains(keys, r107ttCurRows) {
							t.Errorf("%s: the reader's page fits and the row has %d free cells, and it names %q with no key that reaches the row the unfold acts on (#300)\n  foot=%q",
								where, r107ttCurGap(foot, inner), r107ttCurUnfold, keys)
						}
					}
					// Whatever the row took, it paid no key for it:
					// the same stand drawn without the clause names
					// nothing this row does not (#281, #284, #295).
					whole := m.keymap()
					bare := strings.Replace(whole, r107ttCurRows+" · ", "", 1)
					if bare != whole {
						if !footerNamesAll(m.footerTraded(bare, inner), foot) {
							t.Errorf("%s: the cursor clause cost the row a key\n  with=%q\n  without=%q", where, keys, r107ttCurKeys(m.footerTraded(bare, inner)))
						}
					}
					for _, r := range rows {
						if lipgloss.Width(r) > w {
							t.Errorf("%s: a row runs past the terminal (%d of %d): %q", where, lipgloss.Width(r), w, r)
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if roomy < 30 {
		t.Errorf("only %d fitting reader stands with the room for the clause; the biting side is unmeasured", roomy)
	}
	if scrolls < 50 {
		t.Errorf("only %d reader stands on a page that scrolls; the held side is unmeasured", scrolls)
	}
	t.Logf("fitting reader stands with the room: %d · stands on a page that scrolls: %d", roomy, scrolls)
}

// ---- round 108, second-day ----
// TestTheReaderMovementKeyNamesRows holds the rule this round folds: the
// reader's movement key is named for what it moves, `j/k rows`, on every
// page. Since #300 `j` and `k` walk the drawn `▸` a row at a time and move
// the viewport only when the mark would leave it, so `scroll` named a job
// the key does sometimes and often not at all: on the archive's reader at
// eighty the first eight `k` presses from the opening stand step the mark
// and scroll nothing at all, under a row saying `j/k scroll` (#221). #220
// settled the word one level out and #306 brought it to the reader's
// fitting page; it is the same key on both pages.
func TestTheReaderMovementKeyNamesRows(t *testing.T) {
	forceASCII(t)

	r108sdScene := func(name string) scene {
		for _, sc := range allScenes() {
			if sc.name == name {
				return sc
			}
		}
		t.Fatalf("no scene %q", name)
		return scene{}
	}
	r108sdWalk := func(sc scene, w, h int, keys []string) (*Model, []string) {
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		return m, strings.Split(ansi.Strip(m.View()), "\n")
	}
	r108sdFoot := func(rows []string) string { return strings.TrimRight(rows[len(rows)-1], " ") }
	// The page under the cursor, with the cursor taken out of it: the mark
	// is the one cell markAnchor spends on the row it stands on (#300), so
	// two frames that differ only in where the mark is are the same page,
	// drawn at the same scroll.
	r108sdPage := func(rows []string) string {
		var out []string
		for _, r := range rows[:len(rows)-1] {
			// The mark takes the cell after the row's glyph where there is
			// one and is pushed in front of the row's own words where there
			// is not (#300, #305), so the cell it spends is put back and the
			// row's spacing normalised before the two pages are compared.
			out = append(out, strings.Join(strings.Fields(strings.Replace(r, "▸", " ", 1)), " "))
		}
		return strings.Join(out, "\n")
	}
	r108sdMarks := func(rows []string) string {
		var out []string
		for _, r := range rows[:len(rows)-1] {
			if strings.ContainsRune(r, '▸') {
				out = append(out, strings.TrimSpace(r))
			}
		}
		return strings.Join(out, "\n")
	}

	// One: the frame it was found on. `second-day` at eighty, the archive's
	// own reader — a conversation that does not fit, opened at its end with
	// four lines above the viewport. Eight presses of `k` walk the mark up
	// the page and not one of them moves the page.
	sd := r108sdScene("second-day")
	m, stand := r108sdWalk(sd, 80, 24, []string{"A", "tab", "tab"})
	if m.level != 3 || !strings.Contains(strings.Join(stand, "\n"), "READER · api") {
		t.Fatalf("second-day 80x24 [A tab tab]: not the archive's reader: %q", r108sdFoot(stand))
	}
	if m.readerPageFits() {
		t.Fatalf("second-day 80x24 [A tab tab]: the page fits — not the stand this pins")
	}
	if strings.Contains(r108sdFoot(stand), "j/k scroll") {
		t.Errorf("second-day 80x24 [A tab tab]: the row names the cursor key for a job it does not do: %q", r108sdFoot(stand))
	}
	if !strings.Contains(r108sdFoot(stand), "j/k rows") {
		t.Errorf("second-day 80x24 [A tab tab]: the row names no movement key at all: %q", r108sdFoot(stand))
	}
	// The row keeps every key it named under the longer word.
	for _, key := range []string{"space unfold", "[ ] turns", "esc back", "A fleet", "? help", "q quit"} {
		if !strings.Contains(r108sdFoot(stand), key) {
			t.Errorf("second-day 80x24 [A tab tab]: the row lost %q: %q", key, r108sdFoot(stand))
		}
	}
	if w := lipgloss.Width(r108sdFoot(stand)); w > 80 {
		t.Errorf("second-day 80x24 [A tab tab]: the row runs past the terminal (%d): %q", w, r108sdFoot(stand))
	}
	page, marks, moved := r108sdPage(stand), r108sdMarks(stand), 0
	for n := 1; n <= 8; n++ {
		pressKey(m, "k")
		poll(m, sd)
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		if r108sdPage(rows) != page {
			t.Errorf("second-day 80x24 [A tab tab] then %d×%q: the page scrolled — not the stand this pins", n, "k")
			break
		}
		if r108sdMarks(rows) == marks {
			t.Errorf("second-day 80x24 [A tab tab] then %d×%q: the mark stopped moving — not the stand this pins", n, "k")
			break
		}
		if strings.Contains(r108sdFoot(rows), "j/k scroll") {
			t.Errorf("second-day 80x24 [A tab tab] then %d×%q: the press scrolled nothing under a row saying so: %q",
				n, "k", r108sdFoot(rows))
		}
		marks = r108sdMarks(rows)
		moved++
	}
	if moved != 8 {
		t.Errorf("the found stand walked the mark %d rows without scrolling, want 8", moved)
	}

	// Two: the rule, over every scene at five widths under both profiles.
	// No reader row names `j/k scroll`, and the reader's rows that name a
	// movement key name `j/k rows` — the same word the trail's rows use for
	// the same key (#220).
	named, stands := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				for _, route := range [][]string{{"tab", "tab"}, {"A", "tab", "tab"}} {
					rm, rows := r108sdWalk(sc, size[0], size[1], route)
					if rm.level != 3 {
						continue
					}
					stands++
					foot := r108sdFoot(rows)
					if strings.Contains(foot, "j/k scroll") {
						t.Errorf("%v %s %dx%d %v: the reader's row names `j/k scroll`: %q", prof, sc.name, size[0], size[1], route, foot)
					}
					if strings.Contains(foot, "j/k rows") {
						named++
					}
					if w := lipgloss.Width(foot); w > size[0] {
						t.Errorf("%v %s %dx%d %v: the row runs past the terminal (%d): %q", prof, sc.name, size[0], size[1], route, w, foot)
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if stands < 100 || named < 60 {
		t.Fatalf("the rule reached %d reader stands, %d of them naming the key: it has gone vacuous", stands, named)
	}

	// Three: the word is a rename, not a shed. A fold that answers the
	// finding by dropping the movement key from the reader altogether fails
	// here: the page that scrolls names it, and so does the page that fits
	// where the width is there for it (#306).
	if _, rows := r108sdWalk(sd, 220, 48, []string{"tab", "tab"}); !strings.Contains(r108sdFoot(rows), "j/k rows") {
		t.Errorf("second-day 220x48 [tab tab]: the fitting page no longer names the cursor key: %q", r108sdFoot(rows))
	}
}

// ---- round 108, fleet-hygiene ----
// ---- round 108, fleet-hygiene ----
// r108fhOnScreen, r108fhEndWord and r108fhStartWord are the three sentences
// a reader press that moved nothing can draw: what the page holds (#83), and
// the reader's own words for its two ends (#24, #228).
const (
	r108fhOnScreen  = "all of it is on screen"
	r108fhEndWord   = "end of the conversation"
	r108fhStartWord = "start of the conversation"
)

// r108fhEndSizes are the five terminals the walkthrough is drawn at.
var r108fhEndSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r108fhEndRoutes are ways down to a reader page: straight in, in on another
// row, and the chapter and cursor keys pressed once there, so both a page
// that fits and a page that scrolls are stood on.
var r108fhEndRoutes = [][]string{
	{"tab", "tab"},
	{"tab", "tab", "]"},
	{"tab", "tab", "j"},
	{"1", "tab", "tab"},
}

// r108fhEndFoot splits a drawn footer into the keymap that stands before the
// gap and the note that stands after it.
func r108fhEndFoot(rows []string) (keys, note string) {
	if len(rows) == 0 {
		return "", ""
	}
	s := strings.TrimLeft(strings.TrimRight(ansi.Strip(rows[len(rows)-1]), " "), " ")
	if i := strings.Index(s, "  "); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i:])
	}
	return strings.TrimSpace(s), ""
}

// r108fhEndNoted is the finished footer for the stand the reader is on now,
// drawn with note in the note's place, so the trade the fold makes can be
// read key for key without pressing anything.
func r108fhEndNoted(m *Model, w int, note string) string {
	was := m.note
	m.note = note
	out := ansi.Strip(m.footerTraded(m.keymap(), w-2))
	m.note = was
	return out
}

// r108fhEndStand puts the reader's cursor on the row at the far end in
// direction step — the last line of the document for `j`, the first for `k`,
// never the air between blocks — with the page scrolled so that row is on
// it, then presses the key that can go no further twice: the first settles
// the viewport, the second is the press that must answer. It returns the
// frame that press came back with.
func r108fhEndStand(m *Model, sc scene, step int) []string {
	doc := m.doc(m.readerWidth())
	at := len(doc) - 1
	if step < 0 {
		at = 0
	}
	for at >= 0 && at < len(doc) && doc[at].kind == readerBlank {
		at -= step
	}
	if at < 0 || at >= len(doc) {
		return strings.Split(ansi.Strip(m.View()), "\n")
	}
	m.scroll = clampScroll(at-m.readerHeight()/2, len(doc), m.readerHeight())
	m.anchor, m.anchorAt = at, doc[at].at
	key := "j"
	if step < 0 {
		key = "k"
	}
	for i := 0; i < 2; i++ {
		pressKey(m, key)
		poll(m, sc)
	}
	return strings.Split(ansi.Strip(m.View()), "\n")
}

// TestTheReaderSaysWhichEndTheCursorIsAt holds the rule this round folds. On
// a reader page that scrolls, `j` and `k` at the two ends already answer with
// the reader's own words for them — "end of the conversation" and "start of
// the conversation" (#24, #228). On a page that fits, both ends drew #83's
// "all of it is on screen" instead: one sentence for two opposite refusals,
// so `k` on the first row and `j` on the last came back as the very same
// footer and the person could not read from it which end had been reached
// (#221, #304's own rule one level down). The top of a fitting page is
// answered on the frame's own face — `readerAbove` draws " the start of the
// conversation" directly over the cursor — so #83's word stays there, the
// way a note leaves to the row what the row already draws (#128, #134). The
// bottom has no such row, and takes the reader's own word for that end where
// the row has the cell for it (#281, #284, #306); where it has not, #83's
// word stands, and no key that acts is ever paid for the change (#264).
//
// Four sides, at five widths under both colour profiles (#215, #218).
func TestTheReaderSaysWhichEndTheCursorIsAt(t *testing.T) {
	sweep(t)
	forceASCII(t)
	fits, scrolls, roomy, tight := 0, 0, 0, 0
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"mono", termenv.Ascii}, {"colour", termenv.TrueColor}} {
		if prof.p == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
		prev := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range r108fhEndSizes {
				w, h := size[0], size[1]
				for _, route := range r108fhEndRoutes {
					m := sceneModel(sc, w, h)
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					if m.showHelp || m.searching || m.replying || m.level < levelReader {
						continue
					}
					where := fmt.Sprintf("%s %s %v %dx%d", prof.name, sc.name, route, w, h)
					page := len(m.doc(m.readerWidth())) <= m.readerHeight()

					bottom := r108fhEndStand(m, sc, 1)
					endKeys, endNote := r108fhEndFoot(bottom)
					// Measured on the very stand the press came to rest on,
					// before anything else is pressed.
					bare := r108fhEndNoted(m, w, r108fhOnScreen)
					with := r108fhEndNoted(m, w, r108fhEndWord)
					room := footerNamesAll(bare, with)

					top := r108fhEndStand(m, sc, -1)
					_, startNote := r108fhEndFoot(top)

					if !page {
						scrolls++
						// The held side: a page that scrolls already names
						// its two ends apart, and this fold moves none of it.
						if endNote != r108fhEndWord || startNote != r108fhStartWord {
							t.Errorf("%s: a reader page that scrolls stopped naming its ends: end=%q start=%q", where, endNote, startNote)
						}
						continue
					}
					fits++
					// One: #83's word is not lost. It keeps the end whose
					// marker the frame draws over the cursor.
					if !strings.Contains(startNote, r108fhOnScreen) {
						t.Errorf("%s: `k` at the first row of a page that fits no longer answers the page's question: %q", where, startNote)
					}
					joined := strings.Join(top, "\n")
					if !strings.Contains(joined, "the start of the conversation") &&
						!strings.Contains(joined, "the start of the agent's own conversation") {
						t.Errorf("%s: the top of a fitting page draws no start marker, so #83's word there answers nothing", where)
					}
					// Two: where the row has the cell, the end says which
					// end it is, and the two ends stop saying one thing.
					if room {
						roomy++
						if endNote != r108fhEndWord {
							t.Errorf("%s: the row has the cell and `j` at the last row still does not say which end it reached: %q", where, endNote)
						}
						if endNote == startNote {
							t.Errorf("%s: one sentence for both ends of a page that fits — `j` on the last row and `k` on the first say the same thing: %q", where, endNote)
						}
						// Three: it is paid for with no key that acts.
						if !footerNamesAll(bare, endKeys) {
							t.Errorf("%s: the end word cost the row a key\n  end =%q\n  bare=%q", where, endKeys, bare)
						}
					} else {
						// Four: and where it has not, #83's word stands.
						tight++
						if endNote != r108fhOnScreen {
							t.Errorf("%s: the row has no cell for the end's own word and did not keep #83's: %q", where, endNote)
						}
					}
					for _, frame := range [][]string{bottom, top} {
						for _, r := range frame {
							if lipgloss.Width(r) > w {
								t.Errorf("%s: a row runs past the terminal (%d of %d): %q", where, lipgloss.Width(r), w, r)
							}
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(prev)
	}
	if fits < 60 {
		t.Fatalf("only %d fitting reader stands reached: the biting side is unmeasured", fits)
	}
	if scrolls < 20 {
		t.Fatalf("only %d scrolling reader stands reached: the held side is unmeasured", scrolls)
	}
	if roomy < 60 {
		t.Fatalf("only %d fitting stands had the cell for the end's own word: the biting side is unmeasured", roomy)
	}
	t.Logf("fitting reader stands: %d (with the cell %d, without %d) · stands on a page that scrolls: %d", fits, roomy, tight, scrolls)
}

// ---- round 109, fleet-hygiene ----
// r109fhJumpSizes are the five terminals the walkthrough is drawn at.
var r109fhJumpSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r109fhJumpRoutes are two ways down to a reader page — straight in, and in
// on another row — so both a page that fits and a page that scrolls are
// stood on.
var r109fhJumpRoutes = [][]string{
	{"tab", "tab"},
	{"1", "tab", "tab"},
}

const (
	r109fhJumpEnd  = "end of the conversation"
	r109fhJumpTop  = "start of the conversation"
	r109fhJumpPage = "all of it is on screen"
)

// r109fhJumpFoot splits a drawn footer into the keymap before the gap and
// the note after it.
func r109fhJumpFoot(rows []string) (keys, note string) {
	if len(rows) == 0 {
		return "", ""
	}
	s := strings.TrimSpace(ansi.Strip(rows[len(rows)-1]))
	if i := strings.Index(s, "  "); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i:])
	}
	return s, ""
}

// r109fhJumpNoted is the finished footer for the stand the reader is on now,
// drawn with note in the note's place, so the trade can be read key for key
// without pressing anything.
func r109fhJumpNoted(m *Model, w int, note string) string {
	was := m.note
	m.note = note
	out := ansi.Strip(m.footerTraded(m.keymap(), w-2))
	m.note = was
	return out
}

// r109fhJumpPress presses key n times on the stand the reader is on and
// returns the frame the last press came back with.
func r109fhJumpPress(m *Model, sc scene, key string, n int) []string {
	for i := 0; i < n; i++ {
		pressKey(m, key)
		poll(m, sc)
	}
	return strings.Split(ansi.Strip(m.View()), "\n")
}

// The reader's jump keys are named for the two opposite ends of the
// conversation, and on a page that fits they said one thing for both: `g`
// and `G` came back as the same frame under `all of it is on screen`, the
// page's sentence, not the press's — the very thing #309 has just taken off
// `j` and `k` at those same two ends, and the thing #309's own footer trade
// says these two keys already do. On a page that scrolls both keys already
// name the ends apart, so the reader owns both words. `G` says the end here
// too, where the row has the cell for it (#308, #309); where it has not,
// #83's word stands; `g` keeps #83's word at the top, whose marker the
// frame draws over the cursor, as #309 left it.
func TestTheReaderJumpKeySaysWhichEndItReached(t *testing.T) {
	sweep(t)
	forceASCII(t)
	fits, scrolls, roomy, tight := 0, 0, 0, 0
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"mono", termenv.Ascii}, {"colour", termenv.TrueColor}} {
		if prof.p == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
		prev := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range r109fhJumpSizes {
				w, h := size[0], size[1]
				for _, route := range r109fhJumpRoutes {
					base := sceneModel(sc, w, h)
					for _, k := range route {
						pressKey(base, k)
						poll(base, sc)
					}
					if base.showHelp || base.searching || base.replying || base.level < levelReader {
						continue
					}
					where := fmt.Sprintf("%s %s %v %dx%d", prof.name, sc.name, route, w, h)
					page := base.readerPageFits()

					// Twice: on a page that scrolls the first press
					// travels and the second is the one that must answer;
					// on a page that fits neither can move. `G` is pressed
					// on the stand itself and `g` on a second walk down to
					// it, so each key is the last thing the frame saw.
					down := r109fhJumpPress(base, sc, "G", 2)
					keysG, noteG := r109fhJumpFoot(down)

					// The trade is read on the stand the press actually
					// comes to rest on — after it, not before: since #313
					// `G` carries the cursor to the document's last row,
					// not only the viewport, and a row's own key list can
					// depend on where the cursor stands (`[ ] turns`,
					// `enter attach` and the like), not just on the note's
					// width. Measuring `bare` and `with` here, on `base`
					// exactly as the two presses above left it, compares
					// the note's own reserve at the stand it is actually
					// drawn on, the same stand `keysG` came from.
					bare := r109fhJumpNoted(base, w, r109fhJumpPage)
					with := r109fhJumpNoted(base, w, r109fhJumpEnd)
					room := footerNamesAll(bare, with)
					mg := sceneModel(sc, w, h)
					for _, k := range route {
						pressKey(mg, k)
						poll(mg, sc)
					}
					up := r109fhJumpPress(mg, sc, "g", 2)
					_, noteg := r109fhJumpFoot(up)

					for _, frame := range [][]string{down, up} {
						for _, r := range frame {
							if lipgloss.Width(r) > w {
								t.Errorf("%s: a row runs past the terminal (%d of %d): %q", where, lipgloss.Width(r), w, r)
							}
						}
					}
					if !page {
						// The held side: a page that scrolls already
						// names its two ends apart under these keys, and
						// this fold moves none of it.
						scrolls++
						if noteG != r109fhJumpEnd || noteg != r109fhJumpTop {
							t.Errorf("%s: a reader page that scrolls stopped naming its ends under `g`/`G`: G=%q g=%q", where, noteG, noteg)
						}
						continue
					}
					fits++
					// One: the top keeps #83's word, and the frame draws
					// the marker that answers for it.
					if noteg != r109fhJumpPage {
						t.Errorf("%s: `g` on a page that fits no longer answers the page's question: %q", where, noteg)
					}
					joined := strings.Join(up, "\n")
					if !strings.Contains(joined, "the start of the conversation") &&
						!strings.Contains(joined, "the start of the agent's own conversation") {
						t.Errorf("%s: the top of a fitting page draws no start marker, so #83's word there answers nothing", where)
					}
					if room {
						roomy++
						// Two: where the row has the cell, `G` says which
						// end it reached, and the two keys stop saying
						// one thing.
						if noteG != r109fhJumpEnd {
							t.Errorf("%s: the row has the cell and `G` on a page that fits still does not say which end it reached: %q", where, noteG)
						}
						if noteG == noteg {
							t.Errorf("%s: one sentence for both ends of a page that fits — `G` and `g` say the same thing: %q", where, noteG)
						}
						// Three: and it is paid for with no key that acts.
						if !footerNamesAll(bare, keysG) {
							t.Errorf("%s: the end word cost the row a key\n  end =%q\n  bare=%q", where, keysG, bare)
						}
					} else {
						// Four: where the row has no cell for the longer
						// word, #83's word stands (#308, #309).
						tight++
						if noteG != r109fhJumpPage {
							t.Errorf("%s: the row has no cell for the end's own word and did not keep #83's: %q", where, noteG)
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(prev)
	}
	if fits < 40 {
		t.Fatalf("only %d fitting reader stands reached: the biting side is unmeasured", fits)
	}
	if roomy < 40 {
		t.Fatalf("only %d fitting stands had the cell for the end's own word: the biting side is unmeasured", roomy)
	}
	if scrolls < 10 {
		t.Fatalf("only %d scrolling reader stands reached: the held side is unmeasured", scrolls)
	}
	t.Logf("fitting reader stands: %d (with the cell %d, without %d) · stands on a page that scrolls: %d", fits, roomy, tight, scrolls)
}

// ---- round 109, second-day ----
// TestTheHelpNamesTheCursorMark pins the legend's gloss for `▸`, the row
// cursor the board, the trail and (since #300) the reader all draw.
//
// Three sides:
//
//   - the frame it was found on — `second-day` at eighty and at 220, where
//     the deck draws `▸` on the frame the help is opened from and the
//     legend under `?` named eighteen marks and never that one;
//   - the rule — over every scene, five widths and both colour profiles
//     (#215, #218): wherever the help draws the panel mark's line and the
//     width holds the clause, the line names `▸`, with a floor so a fold
//     that quietly stops drawing the help cannot pass;
//   - the price — the gloss is an addition and not a swap: the panel
//     mark keeps its own sentence, and every gloss the legend carried at
//     that width is still on the frame, so no mark is traded for this one.
func TestTheHelpNamesTheCursorMark(t *testing.T) {
	forceASCII(t)
	const panelGloss = "marks the panel your keys are in — tab moves it"
	const clause = "▸ its row"

	// helpFrameAt drives one scene at one size to the help and returns the
	// frame before `?` and the help frame itself, both stripped.
	helpFrameAt := func(sc scene, w, h int) (string, string) {
		m := sceneModel(sc, w, h)
		before := ansi.Strip(m.View())
		pressKey(m, "?")
		poll(m, sc)
		return before, ansi.Strip(m.View())
	}

	// 1 · the frame.
	for _, size := range [][2]int{{80, 24}, {220, 48}} {
		w, h := size[0], size[1]
		before, help := helpFrameAt(sceneSecondDay(), w, h)
		if !strings.Contains(before, "▸") {
			t.Errorf("second-day %dx%d: the frame the help is opened from draws no ▸ — this pin is standing on the wrong frame:\n%s", w, h, before)
		}
		if !strings.Contains(help, panelGloss) {
			t.Fatalf("second-day %dx%d: the help draws no panel-mark line at all:\n%s", w, h, help)
		}
		if !strings.Contains(help, clause) {
			for _, l := range strings.Split(help, "\n") {
				if strings.Contains(l, panelGloss) {
					t.Errorf("second-day %dx%d: the legend names the panel mark and never the row cursor the frame beneath it draws: %q", w, h, strings.TrimRight(l, " "))
				}
			}
		}
	}

	// 2 · the rule, and 3 · the price.
	sizes := [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}
	stands, named := 0, 0
	for _, sc := range allScenes() {
		for _, size := range sizes {
			w, h := size[0], size[1]
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				old := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(prof)
				_, help := helpFrameAt(sc, w, h)
				lipgloss.SetColorProfile(old)
				if !strings.Contains(help, panelGloss) {
					continue
				}
				stands++
				if strings.Contains(help, clause) {
					named++
				} else if w != 152 {
					t.Errorf("%s %dx%d %v: the legend names the panel mark and never `▸`", sc.name, w, h, prof)
				}
				// The price: nothing the legend already said is gone.
				// The `⚠` gloss is the tail of the legend's last line and
				// the first thing a clause that wraps pushes off the
				// overlay, so it is required wherever the clause was
				// taken and the width drew it (vacuous on a tree without
				// the clause, exact on one that paid for it).
				want := []string{panelGloss, "fleet:", "trail:", "you were here", "⌁ "}
				if w >= 100 && strings.Contains(help, clause) {
					want = append(want, "⚠ ")
				}
				for _, s := range want {
					if !strings.Contains(help, s) {
						t.Errorf("%s %dx%d %v: the row cursor's gloss cost the legend %q", sc.name, w, h, prof, s)
					}
				}
				// And the row it rides is still the panel mark's own.
				for _, l := range strings.Split(help, "\n") {
					if strings.Contains(l, clause) && !strings.Contains(l, panelGloss) {
						t.Errorf("%s %dx%d %v: `▸` took a row of its own instead of the panel mark's: %q", sc.name, w, h, prof, strings.TrimRight(l, " "))
					}
				}
			}
		}
	}
	if stands < 80 {
		t.Errorf("only %d help stands drew the panel mark's line, want at least 80 — the walk is not reaching the help any more", stands)
	}
	if named < 70 {
		t.Errorf("only %d of %d help stands name `▸`, want at least 70", named, stands)
	}
}

// ---- round 110, second-day ----
// TestTheLaneReaderOpensOnACursor pins #300's last gap: the reader opened on
// an agent's own conversation drew no cursor at all.
//
// A lane's reader opens at its end rather than on a trail row (#49) and the
// path that does it cleared the anchor, so fifteen canonical `subagents`
// frames — every `tab` onto a lane, at all five widths — drew no `▸` anywhere
// in the reader panel while every other one of the corpus's 608 Lv3 frames
// drew one, and the footer beside them named `j/k rows` and `space unfold`
// with nothing on the frame saying which row those keys act on.
//
// Three sides: the frame it was found on, the rule over every Lv3 stand of
// every scene, and the price — the mark is an addition, so the page still
// opens on its newest line and the rows around it are untouched, and a lane
// whose file holds no turn still draws no cursor, having no row to put one on.
func TestTheLaneReaderOpensOnACursor(t *testing.T) {
	forceASCII(t)

	// laneReaderAt drives one scene to a stand and hands back the model.
	laneReaderAt := func(sc scene, w, h int, keys []string) *Model {
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}
	// readerRows is the reader panel's own rows: at eighty and a hundred the
	// reader owns the screen, so they are the frame's.
	readerRows := func(frame string) []string {
		var out []string
		for _, l := range strings.Split(frame, "\n") {
			out = append(out, strings.TrimRight(l, " "))
		}
		return out
	}
	// markedRow is the one row of the reader panel carrying the cursor.
	markedRow := func(m *Model) (string, int) {
		frame := ansi.Strip(m.View())
		row, n := "", 0
		for _, l := range readerRows(frame) {
			if strings.Contains(l, "▸") {
				row, n = l, n+1
			}
		}
		return row, n
	}

	// The canonical walkthrough's own prefix to the first `tab` onto a lane.
	toLane := []string{"r", "1", "/", "pytest", "enter", "esc", "tab", "ctrl+u", "ctrl+u", "[", "]", "G", "tab"}

	// 1 · the frame: `subagents`, the lane reader, at the two widths where the
	// reader owns the screen.
	for _, size := range [][2]int{{80, 24}, {100, 30}} {
		w, h := size[0], size[1]
		sc := sceneSubagents()
		m := laneReaderAt(sc, w, h, toLane)
		if m.readerLane == "" {
			t.Fatalf("subagents %dx%d: this pin is not standing on a lane's reader", w, h)
		}
		doc := m.doc(m.readerWidth())
		if len(doc) == 0 {
			t.Fatalf("subagents %dx%d: the lane's reader draws no document", w, h)
		}
		row, n := markedRow(m)
		if n != 1 {
			t.Errorf("subagents %dx%d: the reader opened on a lane draws %d cursor marks, want exactly one:\n%s", w, h, n, ansi.Strip(m.View()))
		}
		if m.anchor < 0 || m.anchor >= len(doc) {
			t.Errorf("subagents %dx%d: the lane's reader opened with anchor %d — no row is the cursor's", w, h, m.anchor)
			continue
		}
		if doc[m.anchor].kind == readerBlank {
			t.Errorf("subagents %dx%d: the cursor opened on the air between blocks (row %d)", w, h, m.anchor)
		}
		// It stands where the page stands: the last row of the document,
		// the newest thing the agent has written (#49).
		last := len(doc) - 1
		for last > 0 && doc[last].kind == readerBlank {
			last--
		}
		if m.anchor != last {
			t.Errorf("subagents %dx%d: the page opens on its newest line and the cursor stands on row %d of %d (%q), not the newest (%q)",
				w, h, m.anchor, len(doc), readerRowText(doc, m.anchor), readerRowText(doc, last))
		}
		// And the row it marks is the last drawn row of the page.
		want := "⋯ no result yet"
		if !strings.Contains(row, want) {
			t.Errorf("subagents %dx%d: the cursor stands on %q, want the newest line %q", w, h, row, want)
		}
		// The price: the page is the page it was. It still opens at its
		// end, still says which end that is, and its footer still names
		// the keys that act on the marked row.
		frame := ansi.Strip(m.View())
		for _, s := range []string{"the start of the agent's own conversation", "space unfold", "[ ] turns", "esc back"} {
			if !strings.Contains(frame, s) {
				t.Errorf("subagents %dx%d: the cursor cost the page %q", w, h, s)
			}
		}
	}

	// 2 · the rule: over every Lv3 stand of the canonical walkthrough, a
	// reader with a document draws a cursor — `subagents` at five widths and
	// both colour profiles (#215, #218), every other scene at a hundred.
	type walk struct {
		sc    scene
		sizes [][2]int
		profs []termenv.Profile
	}
	walks := []walk{{sceneSubagents(), [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}, []termenv.Profile{termenv.Ascii, termenv.TrueColor}}}
	for _, sc := range allScenes() {
		if sc.name == "subagents" {
			continue
		}
		walks = append(walks, walk{sc, [][2]int{{100, 30}}, []termenv.Profile{termenv.Ascii}})
	}
	stands, lanes, empty := 0, 0, 0
	for _, wk := range walks {
		for _, size := range wk.sizes {
			w, h := size[0], size[1]
			for _, prof := range wk.profs {
				old := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(prof)
				sc := wk.sc
				m := sceneModel(sc, w, h)
				keys := canonicalKeys
				if len(sc.extra) > 0 {
					keys = append(append(append([]string(nil), keys...), "esc"), sc.extra...)
				}
				for _, k := range keys {
					pressKey(m, k)
					poll(m, sc)
					if m.level != levelReader {
						continue
					}
					doc := m.doc(m.readerWidth())
					if len(doc) == 0 {
						// A lane whose file holds no turn: no row, so no
						// cursor, and no key on its footer walks one.
						empty++
						if m.anchor >= 0 {
							t.Errorf("%s %dx%d %v after %q: a reader with no document opened with a cursor on row %d", sc.name, w, h, prof, k, m.anchor)
						}
						continue
					}
					stands++
					if m.readerLane != "" {
						lanes++
					}
					if m.anchor < 0 || m.anchor >= len(doc) || doc[m.anchor].kind == readerBlank {
						t.Errorf("%s %dx%d %v after %q: the reader draws %d rows and no cursor stands on one (anchor %d)", sc.name, w, h, prof, k, len(doc), m.anchor)
					}
				}
				lipgloss.SetColorProfile(old)
			}
		}
	}
	if stands < 100 {
		t.Errorf("only %d reader stands were reached, want at least 100 — the walk is not reaching the reader any more", stands)
	}
	if lanes < 20 {
		t.Errorf("only %d of the reader stands were a lane's own conversation, want at least 20 — this pin is not standing on the frame it was written for", lanes)
	}
	if empty < 5 {
		t.Errorf("only %d reader stands drew no document, want at least 5 — the empty lane is the price side of this pin", empty)
	}
}

// ---- round 111, two-tools ----
// ---- round 111, two-tools ----
// r111ttWordSizes is the deck's own width ladder.
var r111ttWordSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r111ttWordWalk is the walk this pin stands on, checked at every step:
// the deck's own way down to the reader — a session, its trail, the
// companion the trail hands over, the end of the conversation, the keys —
// and then the reader's own movement keys pressed on that landing.
func r111ttWordWalk() []string {
	return []string{"1", "tab", "tab", "G", "tab", "j", "g", "G", "space"}
}

// r111ttMarkedCells are the reader panel's own drawn rows carrying the
// cursor: at 80 and 100 the reader owns the screen, wider it stands right
// of the last panel rule.
func r111ttMarkedCells(frame string) []string {
	var out []string
	for _, l := range strings.Split(ansi.Strip(frame), "\n") {
		parts := strings.Split(l, "│")
		cell := strings.TrimRight(parts[len(parts)-1], " ")
		if strings.Contains(cell, "▸") {
			out = append(out, cell)
		}
	}
	return out
}

// r111ttMarkInsideWord reports the first drawn cell whose `▸` has a letter
// or a digit immediately before it — the mark standing inside the row's own
// words instead of in the cell its shape leaves free.
func r111ttMarkInsideWord(cells []string) (string, bool) {
	for _, cell := range cells {
		r := []rune(cell)
		for i, c := range r {
			if c == '▸' && i > 0 && (unicode.IsLetter(r[i-1]) || unicode.IsDigit(r[i-1])) {
				return cell, true
			}
		}
	}
	return "", false
}

// TestTheReaderCursorNeverStandsInsideAWord holds the reader's cursor mark
// to the row's shape and out of its words.
//
// markAnchor spends the cell right after a row's first rune — "❯▸", "⏺▸",
// "  ▸⎿" — and its own comment says that cell is "a leading glyph or an
// indent … never a letter of the row's own words". The test it made was
// positional and not that: it asked only whether the *second* rune was a
// space. Prose the model wrote can open on a one-letter word, and there
// the space after it belongs to the sentence — so "I need a decision
// before I change the rule." was drawn "I▸need a decision before I change
// the rule." on 42 canonical rows (`two-tools`, `alarm-storm`,
// `few-ongoing`, at all five widths, at Lv2 and Lv3 alike), the mark
// reading as a rune of the transcript the reader exists to quote (#19).
// The `else` branch markAnchor already carries — the mark pushed in front,
// a cell off the far end when the row is full — is the right one there.
//
// Three sides, under both colour profiles (#215, #218):
//   - the fault: on no stand is a drawn `▸` preceded by a letter or digit;
//   - the mark is still there and still true: every Lv3 stand whose
//     document has a row draws exactly one, on the anchor's own row;
//   - it costs the row nothing new: the marked row is no wider than its
//     terminal, and carries its whole text unless it ends on a cut mark.
//
// It refuses to be vacuous two ways: at least 200 stands are walked, and
// at least 8 of them are anchored on a row whose first word is one rune
// long — the case the fix is about.
func TestTheReaderCursorNeverStandsInsideAWord(t *testing.T) {
	sweep(t)
	stands, marked, oneRuneRows := 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		if prof == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range r111ttWordSizes {
				w, h := size[0], size[1]
				m := sceneModel(sc, w, h)
				for step, k := range r111ttWordWalk() {
					pressKey(m, k)
					poll(m, sc)
					stands++
					cells := r111ttMarkedCells(m.View())

					// Side 1 — the fault, at whatever level the walk stands on.
					if cell, bad := r111ttMarkInsideWord(cells); bad {
						t.Errorf("%v %s %dx%d after %d keys (%q, lv%d): the cursor mark stands inside the row's own words: %q",
							prof, sc.name, w, h, step+1, k, m.level, cell)
					}

					if m.level != levelReader {
						continue
					}
					doc := m.doc(m.readerWidth())
					if len(doc) == 0 || m.anchor < 0 || m.anchor >= len(doc) {
						continue // the lane with nothing written: no row to stand on (#313)
					}
					want := readerRowText(doc, m.anchor)
					if f := strings.Fields(want); len(f) > 0 && len([]rune(f[0])) == 1 {
						oneRuneRows++
					}

					// Side 2 — the mark is still there, on the anchor's row.
					// A stand whose anchored row has scrolled off the drawn
					// page draws none, which is a fault of its own and not
					// this one (raised, not cut, in round 111); the mark is
					// never doubled, and the vacuity floor below holds this
					// side to a real count of stands that do draw one.
					if len(cells) > 1 {
						t.Errorf("%v %s %dx%d after %d keys: the reader draws %d cursor marks, want at most one",
							prof, sc.name, w, h, step+1, len(cells))
						continue
					}
					if len(cells) == 0 {
						continue
					}
					marked++
					cell := cells[0]
					spaced := strings.Replace(cell, "▸", " ", 1)
					pushed := strings.Replace(cell, "▸", "", 1)
					if f := strings.Fields(want); len(f) > 0 &&
						!strings.Contains(spaced, f[0]) && !strings.Contains(pushed, f[0]) {
						t.Errorf("%v %s %dx%d after %d keys: the marked row %q does not carry the anchored row's own words (%q)",
							prof, sc.name, w, h, step+1, cell, want)
					}

					// Side 3 — the cost.
					if lipgloss.Width(cell) > w {
						t.Errorf("%v %s %dx%d after %d keys: the marked row is %d cells in a %d-cell terminal: %q",
							prof, sc.name, w, h, step+1, lipgloss.Width(cell), w, cell)
					}
					if !strings.Contains(spaced, want) && !strings.Contains(pushed, want) && !strings.Contains(cell, "…") {
						t.Errorf("%v %s %dx%d after %d keys: the marked row lost part of its text with nothing to say so: drew %q, the row is %q",
							prof, sc.name, w, h, step+1, cell, want)
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}

	t.Logf("stands walked: %d · Lv3 stands with a mark: %d · anchored rows opening on a one-rune word: %d",
		stands, marked, oneRuneRows)
	if stands < 200 {
		t.Errorf("this pin walked %d stands, too few to hold anything", stands)
	}
	if marked < 100 {
		t.Errorf("this pin saw the mark drawn on %d stands, too few to hold anything about it", marked)
	}
	if oneRuneRows < 8 {
		t.Errorf("this pin stood on %d rows opening on a one-rune word, too few to be about the defect it names", oneRuneRows)
	}
}

// ---- round 111, fleet-hygiene ----
// r111fhWidths is the deck's own width ladder, so the search's landing is
// pinned at every shape the deck draws.
var r111fhWidths = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r111fhQuery is a word the scene's own conversation says more than once, so
// the walk has somewhere to walk to. Read off the document rather than
// hard-coded, so the pin does not go vacuous when a fixture is reworded.
func r111fhQuery(doc []readerLine) string {
	count := map[string]int{}
	for _, l := range doc {
		if l.kind == readerBlank {
			continue
		}
		seen := map[string]bool{}
		for _, w := range strings.Fields(strings.ToLower(l.text)) {
			w = strings.Trim(w, "()[].,:;\"'·⎿⏺❯◆◍◈▸")
			if len([]rune(w)) < 5 || seen[w] {
				continue
			}
			seen[w] = true
			count[w]++
		}
	}
	best, bestN := "", 0
	for w, n := range count {
		if n > bestN || (n == bestN && w < best) {
			best, bestN = w, n
		}
	}
	if bestN < 2 {
		return ""
	}
	return best
}

// r111fhRowsWith is an independent read of "the document rows the query
// appears in": the pin does not take readerMatches' own answer on trust.
func r111fhRowsWith(doc []readerLine, q string) []int {
	var out []int
	for i, l := range doc {
		if l.kind == readerBlank {
			continue
		}
		if strings.Contains(strings.ToLower(l.text), strings.ToLower(q)) {
			out = append(out, i)
		}
	}
	return out
}

// r111fhMarkedLine is the drawn line carrying the reader's cursor. The
// reader is the frame's last column at every width — beside the trail's
// companion from 120 up, alone below it — so the mark is looked for after
// the row's last panel rule, never in the trail's own cursor.
func r111fhMarkedLine(frame string) (string, bool) {
	for _, l := range strings.Split(frame, "\n") {
		cells := strings.Split(ansi.Strip(l), "│")
		if strings.Contains(cells[len(cells)-1], "▸") {
			return cells[len(cells)-1], true
		}
	}
	return "", false
}

// r111fhSaysRow reports whether the drawn, cursor-bearing line carries the
// document row's own opening words — the mark replaces a cell rather than
// hiding one (#305), so the words survive it.
func r111fhSaysRow(line, text string) bool {
	flat := strings.ReplaceAll(line, "▸", " ")
	want := strings.TrimSpace(text)
	if r := []rune(want); len(r) > 12 {
		want = string(r[:12])
	}
	return want != "" && strings.Contains(flat, want)
}

// TestTheSearchWalkTakesTheReaderCursorToTheMatch pins the search's landing
// against #300's rule and #314's: `/`, `n` and `N` move the reader's page to
// a match and the note counts it (`match 3/4`, #236), and the mark that says
// which row that is must go with them.
//
// Before this, the mark stayed where the page left it: on 447 of the
// corpus's 452 search stands it stood on a row that is not the match the
// note counts, and on 333 of those it was off the page altogether, so the
// reader drew no cursor at all while the footer beside it named `j/k rows`
// and `space unfold`, and Space fell back to the top-down scan for want of
// one (#300, #313). The match's own highlight is a style (matchStyle), which
// no capture, no NO_COLOR terminal and no faint-reverse terminal carries
// (§4) — so on a monochrome deck nothing on the frame said which row
// `match 3/4` meant. Four sides, both colour profiles (#215, #218):
//
//  1. the mark is on the frame, in the reader's own column;
//  2. it stands on the row the walk counts, and that row is a match;
//  3. it is inside the drawn viewport, so the frame is not a page with no
//     cursor on it;
//  4. `N` walks back the way `n` came, mark and all.
func TestTheSearchWalkTakesTheReaderCursorToTheMatch(t *testing.T) {
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"mono", termenv.Ascii}, {"colour", termenv.TrueColor}} {
		prev := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof.p)
		forward, back := 0, 0
		for _, sc := range []scene{sceneManyIdle(), sceneFleetHygiene()} {
			for _, size := range r111fhWidths {
				w, h := size[0], size[1]
				m := sceneModel(sc, w, h)
				toLv3(m)
				q := r111fhQuery(m.doc(m.readerWidth()))
				if q == "" {
					t.Fatalf("%s %s %dx%d: no word this conversation says twice", prof.name, sc.name, w, h)
				}
				pressKey(m, "/")
				pressKey(m, q)
				pressKey(m, "enter")
				doc := m.doc(m.readerWidth())
				want := r111fhRowsWith(doc, q)
				if len(want) < 2 {
					t.Fatalf("%s %s %dx%d: %q appears on %d rows, want at least two", prof.name, sc.name, w, h, q, len(want))
				}
				seen := map[int]bool{}
				for step := 0; step <= len(want); step++ {
					if step > 0 {
						pressKey(m, "n")
					}
					doc = m.doc(m.readerWidth())
					at := m.anchor
					where := prof.name + " " + sc.name + " " + m.note
					// (2) the mark stands on a row the query appears in
					if at < 0 || at >= len(doc) || !strings.Contains(strings.ToLower(doc[at].text), strings.ToLower(q)) {
						t.Errorf("%s %dx%d /%s step %d: the note says %q and the cursor stands on row %d, which is not a match", where, w, h, q, step, m.note, at)
						continue
					}
					seen[at] = true
					// (3) and it is on the page the frame draws
					top, height := m.readerTop(doc), m.readerHeight()
					if at < top || at >= top+height {
						t.Errorf("%s %dx%d /%s step %d: the note says %q with the cursor on row %d, off the drawn page [%d,%d)", where, w, h, q, step, m.note, at, top, top+height)
					}
					// (1) and the frame draws it, in the reader's column
					line, ok := r111fhMarkedLine(m.View())
					if !ok {
						t.Errorf("%s %dx%d /%s step %d: the note says %q and the reader draws no cursor at all", where, w, h, q, step, m.note)
						continue
					}
					if !r111fhSaysRow(line, doc[at].text) {
						t.Errorf("%s %dx%d /%s step %d: the drawn cursor is on %q, not on the match %q", where, w, h, q, step, strings.TrimSpace(line), strings.TrimSpace(doc[at].text))
					}
					forward++
				}
				if len(seen) < 2 {
					t.Errorf("%s %s %dx%d /%s: the walk stood on %d distinct matches of %d", prof.name, sc.name, w, h, q, len(seen), len(want))
				}
				// (4) the walk comes back the way it went
				for step := 0; step < len(want); step++ {
					wasAt := m.anchor
					pressKey(m, "N")
					doc = m.doc(m.readerWidth())
					at := m.anchor
					if at == wasAt || at < 0 || at >= len(doc) || !strings.Contains(strings.ToLower(doc[at].text), strings.ToLower(q)) {
						t.Errorf("%s %s %dx%d /%s back-step %d: `N` left the cursor on row %d (was %d), which is not the previous match", prof.name, sc.name, w, h, q, step, at, wasAt)
						continue
					}
					back++
				}
			}
		}
		if forward < 20 || back < 20 {
			t.Errorf("%s: the pin measured %d forward stands and %d back — too few to mean anything", prof.name, forward, back)
		}
		lipgloss.SetColorProfile(prev)
	}
}

// ---- round 111, second-day ----
// TestTheJumpToTheStartLetsTheMarkSayIt pins #304's rule at the reader's top
// end, where #314 left a note standing over a press that acted.
//
// Since #314 `g` walks the cursor to the document's first row. On a page that
// fits, `scrollBy` answers the page's own question on the way and the row came
// back saying "all of it is on screen" — the sentence #304 took off `j`, `k`,
// `ctrl+d` and `ctrl+u` for standing over a press that moved the mark, and
// #309 left at this end only where the press moved nothing. `k` landing the
// mark on that same first row of that same page says nothing and keeps `a ask`
// and `enter attach`; under `g` the note cost the row both.
//
// Three sides: the frame it was found on (`second-day` at eighty, the
// walkthrough's own reader stand), the rule over every Lv3 stand of both
// second-day scenes at five widths and both colour profiles (#215, #218), and
// the price — the note is not lost, only outranked: where `g` moves nothing it
// still says its word, where the row would pay a key for the shed it keeps the
// word instead (#281, #308), and `G` at the other end is untouched (#310).
func TestTheJumpToTheStartLetsTheMarkSayIt(t *testing.T) {
	forceASCII(t)

	pageWord := "all of it is on screen"

	// standAt drives one scene to a stand and hands back the model.
	standAt := func(sc scene, w, h int, keys []string) *Model {
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}
	// drawnFooter is the frame's own last row.
	drawnFooter := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	// footerSaying is the row this stand draws with that note on it — the
	// keymap's own trades and nothing else, which is the row as it stood
	// before the fold, and the row the frame draws when the note stays.
	footerSaying := func(m *Model, note string) string {
		inner := m.width - 2*edgePad
		if inner < 10 {
			inner = m.width
		}
		was := m.note
		m.note = note
		row := strings.TrimRight(ansi.Strip(m.footerTraded(m.keymap(), inner)), " ")
		m.note = was
		return row
	}
	// markedRow is the one row of the reader panel carrying the cursor, and
	// how many rows carry one; at eighty the reader owns the screen.
	markedRow := func(m *Model) (string, int) {
		row, n := "", 0
		for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			if strings.Contains(l, "▸") {
				row, n = strings.TrimRight(l, " "), n+1
			}
		}
		return row, n
	}

	var secondDay, firstSession scene
	for _, sc := range allScenes() {
		switch sc.name {
		case "second-day":
			secondDay = sc
		case "first-session":
			firstSession = sc
		}
	}

	// 1 · the frame it was found on: `second-day` at eighty, the canonical
	// walkthrough's own reader stand, the cursor one row down.
	toReader := []string{"r", "1", "/", "pytest", "enter", "esc", "tab", "ctrl+u", "ctrl+u", "[", "]", "G", "tab", "[", "]", "k", "k", "j"}
	before := standAt(secondDay, 80, 24, toReader)
	if before.level != levelReader {
		t.Fatalf("the walkthrough's prefix no longer stands in the reader (Lv%d)", before.level)
	}
	at := before.anchor
	withG := standAt(secondDay, 80, 24, append(append([]string(nil), toReader...), "g"))
	withK := standAt(secondDay, 80, 24, append(append([]string(nil), toReader...), "k"))
	if withG.anchor == at {
		t.Fatalf("`g` moved no mark on the stand this is measured on (row %d)", at)
	}
	if withG.anchor != withK.anchor {
		t.Fatalf("`g` and `k` land the mark on different rows (%d and %d): the frames are not comparable", withG.anchor, withK.anchor)
	}
	gRow, gMarks := markedRow(withG)
	if kRow, kMarks := markedRow(withK); gRow != kRow || gMarks != 1 || kMarks != 1 {
		t.Errorf("the two presses draw the mark differently: `g` %q (%d marks), `k` %q (%d marks)", gRow, gMarks, kRow, kMarks)
	}
	gFoot, kFoot := drawnFooter(withG), drawnFooter(withK)
	if strings.Contains(gFoot, pageWord) {
		t.Errorf("second-day 80x24, `g` walked the mark to row %d and the row still says the page's word:\n  %q", withG.anchor, gFoot)
	}
	for _, key := range []string{"a ask", "enter attach"} {
		if !strings.Contains(kFoot, key) {
			t.Fatalf("the stand's own `k` row no longer names %q: %q", key, kFoot)
		}
		if !strings.Contains(gFoot, key) {
			t.Errorf("second-day 80x24: the note under `g` costs the row %q, which `k` on the same landing names:\n  g: %q\n  k: %q", key, gFoot, kFoot)
		}
	}

	// 2 · the rule: at every Lv3 stand of the canonical walkthrough over both
	// second-day scenes, five widths, both colour profiles, the row `g` draws
	// is the row without the page's word wherever that row still names every
	// key the row with it named, and the row with it wherever it does not.
	stands, walked, kept := 0, 0, 0
	for _, sc := range []scene{secondDay, firstSession} {
		keys := append(append(append([]string(nil), canonicalKeys...), "esc"), sc.extra...)
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				old := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(prof)
				for i := range keys {
					prefix := keys[:i+1]
					m := standAt(sc, w, h, prefix)
					if m.level != levelReader || len(m.doc(m.readerWidth())) == 0 {
						continue
					}
					stands++
					was, fits := m.anchor, m.readerPageFits()
					after := standAt(sc, w, h, append(append([]string(nil), prefix...), "g"))
					foot := strings.TrimSpace(drawnFooter(after))
					if !fits || after.anchor == was {
						// The price: a press that moved no mark keeps its
						// word, and so does a page that scrolls.
						if foot == "" || (after.note == "" && after.anchor == was) {
							t.Errorf("%s %dx%d %v after %q: `g` moved nothing and said nothing", sc.name, w, h, prof, prefix[len(prefix)-1])
						}
						continue
					}
					walked++
					with := strings.TrimSpace(footerSaying(after, pageWord))
					without := strings.TrimSpace(footerSaying(after, ""))
					want := with
					if footerNamesAll(with, without) {
						want = without
					} else {
						kept++
					}
					if foot != want {
						t.Errorf("%s %dx%d %v after %q: `g` walked the mark %d→%d and drew\n  %q\nwant\n  %q", sc.name, w, h, prof, prefix[len(prefix)-1], was, after.anchor, foot, want)
					}
					if !footerNamesAll(with, foot) {
						t.Errorf("%s %dx%d %v after %q: the row `g` draws names fewer keys than the row with the note:\n  %q\n  %q", sc.name, w, h, prof, prefix[len(prefix)-1], foot, with)
					}
				}
				lipgloss.SetColorProfile(old)
			}
		}
	}
	if stands < 300 {
		t.Errorf("only %d Lv3 stands were reached, want at least 300 — the walk is not reaching the reader any more", stands)
	}
	if walked < 100 {
		t.Errorf("`g` walked the mark on only %d stands, want at least 100 — the case this pins is not being reached", walked)
	}

	// 3 · the price: `G` at the other end still says the end it reached
	// (#310), and the mark is on the document's last row (#314).
	gg := standAt(secondDay, 120, 34, []string{"r", "1", "/", "pytest", "enter", "esc", "tab", "ctrl+u", "ctrl+u", "[", "]", "G"})
	if got := drawnFooter(gg); !strings.Contains(got, "end of the conversation") {
		t.Errorf("`G` no longer says the end it reached: %q", got)
	}
	doc := gg.doc(gg.readerWidth())
	last := -1
	for i := range doc {
		if doc[i].kind != readerBlank {
			last = i
		}
	}
	if gg.anchor != last {
		t.Errorf("`G` left the mark on row %d, the document's last row is %d", gg.anchor, last)
	}
	t.Logf("Lv3 stands %d · `g` walked the mark on %d · the word kept for a key on %d", stands, walked, kept)
}

// ---- round 112, second-day ----
// TestTheReaderPageFollowsItsCursorWhenTheWindowChangesSize pins #300's mark
// against the one thing that can still take it off the page: the window.
//
// A terminal dragged narrower draws fewer reader rows, and SetSize moved
// neither the page nor the mark: the cursor the person left down the page
// fell under it, so the frame came back with no `▸` anywhere while the
// footer beside it named `j/k rows` and `space unfold` — the same defect
// #313 cut for the lane's opening and #317 for the search walk, reached by
// the window instead of by a key. `j` there stepped from the page's top and
// not from the mark, thirteen rows back up the conversation.
//
// Three sides: the frame it was found on (`second-day`, the archive's `api`
// reader, 100x30 dragged to 80x24), the rule over every Lv3 stand of both
// second-day scenes at five widths resized to each of the other four under
// both colour profiles (#215, #218), and the price — the page moves only
// where the cursor would otherwise be off it, and the resize never moves the
// mark itself.
func TestTheReaderPageFollowsItsCursorWhenTheWindowChangesSize(t *testing.T) {
	forceASCII(t)

	sizes := [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

	// standAt drives one scene to a stand and hands back the model.
	standAt := func(sc scene, w, h int, keys []string) *Model {
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}
	// markedRow is the one row of the frame carrying the cursor, and how
	// many rows carry one; at eighty and a hundred the reader owns the
	// screen, so the frame's marks are the reader's.
	markedRow := func(m *Model) (string, int) {
		row, n := "", 0
		for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			if strings.Contains(l, "▸") {
				row, n = strings.TrimRight(l, " "), n+1
			}
		}
		return row, n
	}
	// drawnFooter is the frame's own last row.
	drawnFooter := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	// onPage says whether the cursor's row is inside the page the frame is
	// about to draw, and hands back the row and the page for the message.
	onPage := func(m *Model) (bool, int, int, int) {
		doc := m.doc(m.readerWidth())
		if len(doc) == 0 {
			return true, -1, 0, 0
		}
		row := m.readerAnchorAt(doc)
		top, h := m.readerTop(doc), m.readerHeight()
		if row < 0 {
			return true, row, top, top + h - 1
		}
		return row >= top && row <= top+h-1, row, top, top + h - 1
	}

	var secondDay, firstSession scene
	for _, sc := range allScenes() {
		switch sc.name {
		case "second-day":
			secondDay = sc
		case "first-session":
			firstSession = sc
		}
	}

	// 1 · the frame it was found on: the walkthrough's own way into the
	// archive's `api` reader, at a hundred, and the window dragged to eighty.
	toArchiveReader := append(append(append([]string(nil), canonicalKeys...), "esc"), "tab", "2", "tab", "tab")
	m := standAt(secondDay, 100, 30, toArchiveReader)
	if m.level != levelReader {
		t.Fatalf("the walkthrough's prefix no longer stands in the reader (Lv%d)", m.level)
	}
	was, wasRow := m.anchor, ""
	if doc := m.doc(m.readerWidth()); was >= 0 && was < len(doc) {
		wasRow = readerRowText(doc, was)
	}
	if row, n := markedRow(m); n != 1 || wasRow == "" {
		t.Fatalf("the stand this is measured on does not draw one cursor: %d marks, row %q", n, row)
	}
	m.SetSize(80, 24)
	row, n := markedRow(m)
	if n != 1 {
		t.Errorf("second-day 100x30 → 80x24 in the archive's reader: the frame draws %d cursor marks, want exactly one:\n%s", n, ansi.Strip(m.View()))
	}
	if n == 1 && !strings.Contains(row, wasRow) {
		t.Errorf("second-day 100x30 → 80x24: the mark came back on %q, the row it stood on was %q", row, wasRow)
	}
	if m.anchor != was {
		t.Errorf("the resize moved the mark itself: row %d → %d", was, m.anchor)
	}
	if foot := drawnFooter(m); !strings.Contains(foot, "j/k rows") {
		t.Errorf("the row under the resized reader no longer names the key that steps the cursor: %q", foot)
	}
	// And the next `j` steps from the mark, not from the top of the page.
	doc := m.doc(m.readerWidth())
	next := nonBlankRow(doc, m.anchor+1, 1)
	pressKey(m, "j")
	if next >= 0 && m.anchor != next {
		t.Errorf("after the resize `j` stepped to row %d, the row under the mark is %d", m.anchor, next)
	}
	if _, marks := markedRow(m); marks != 1 {
		t.Errorf("after the resize `j` left %d marks on the frame, want one:\n%s", marks, ansi.Strip(m.View()))
	}

	// 2 · the rule: every Lv3 stand of both second-day scenes at five widths,
	// resized to each of the other four, under both colour profiles.
	pairs, followed, still := 0, 0, 0
	for _, sc := range []scene{secondDay, firstSession} {
		keys := append(append(append([]string(nil), canonicalKeys...), "esc"), sc.extra...)
		for _, size := range sizes {
			w, h := size[0], size[1]
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				old := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(prof)
				for i := range keys {
					prefix := keys[:i+1]
					base := standAt(sc, w, h, prefix)
					if base.level != levelReader || len(base.doc(base.readerWidth())) == 0 {
						continue
					}
					for _, ns := range sizes {
						if ns == size {
							continue
						}
						after := standAt(sc, w, h, prefix)
						before, wasScroll := after.anchor, after.scroll
						after.SetSize(ns[0], ns[1])
						pairs++
						ok, r, top, bottom := onPage(after)
						if !ok {
							t.Errorf("%s %dx%d → %dx%d %v after %q: the cursor stands on row %d and the page draws %d..%d, so the frame carries no mark:\n  %q",
								sc.name, w, h, ns[0], ns[1], prof, prefix[len(prefix)-1], r, top, bottom, drawnFooter(after))
						}
						if after.anchor != before {
							t.Errorf("%s %dx%d → %dx%d %v: the resize moved the mark itself, row %d → %d", sc.name, w, h, ns[0], ns[1], prof, before, after.anchor)
						}
						if ns[0] <= 100 && r >= 0 {
							if _, marks := markedRow(after); marks != 1 {
								t.Errorf("%s %dx%d → %dx%d %v: the resized reader draws %d marks, want one", sc.name, w, h, ns[0], ns[1], prof, marks)
							}
						}
						// The price: the page moves only where the cursor
						// would otherwise have fallen off it.
						if after.scroll != wasScroll {
							followed++
							// What the page would have been had the resize
							// left the scroll where it was.
							doc, height := after.doc(after.readerWidth()), after.readerHeight()
							stood := readerTopIn(doc, wasScroll, height)
							if r >= stood && r <= stood+height-1 {
								t.Errorf("%s %dx%d → %dx%d %v: the page moved (scroll %d → %d) with the cursor already on it (row %d, page %d..%d)",
									sc.name, w, h, ns[0], ns[1], prof, wasScroll, after.scroll, r, stood, stood+height-1)
							}
						} else {
							still++
						}
						// And the first step after the resize lands on the
						// page the frame draws, not one row under it.
						pressKey(after, "j")
						if ok, r, top, bottom := onPage(after); !ok {
							t.Errorf("%s %dx%d → %dx%d %v: the `j` after the resize left the cursor on row %d with the page drawing %d..%d", sc.name, w, h, ns[0], ns[1], prof, r, top, bottom)
						}
					}
				}
				lipgloss.SetColorProfile(old)
			}
		}
	}
	if pairs < 1000 {
		t.Errorf("only %d (stand, new size) pairs were reached, want at least 1000 — the walk is not reaching the reader any more", pairs)
	}
	if still == 0 {
		t.Errorf("every resize moved the page: the fold is not the narrow one it claims to be")
	}
	t.Logf("resize pairs %d · the page followed the cursor on %d · stood still on %d", pairs, followed, still)
}

// ---- round 112, two-tools ----
// TestSpaceKeepsTheReaderCursorOnThePage holds Space to the rule #300 gave
// the reader's movement keys: the page moves only far enough to keep the
// cursor's row drawn.
//
// toggleFold resolves the anchor against the document the fold has just
// reshaped (#314) but never moved the page to it. Unfolding a result the
// cursor stood on near the end of a long conversation pushed the anchored
// row below the page's last drawn line: the reader drew no `▸` at all while
// the footer beside it named `j/k rows · space unfold`, and the note said
// `unfolded Edit(loader.py)` over a page whose last row was that very call,
// every line it opened below the screen (`many-idle` at 100x30 and 120x34,
// 20 of 270 stands over six routes). The next press was worse than a lost
// mark: with the anchor off the page readerCursorMove restarts from the
// page's own top, so `j` — the key named for the next row down — stepped
// the cursor twenty-two rows backwards.
//
// Four sides, under both colour profiles (#215, #218):
//   - the fault: after Space, a reader stand whose anchor is a real row
//     draws exactly one cursor mark;
//   - it is the right row: the marked row carries the anchored row's own
//     words;
//   - the press after it walks forward: `j` on that landing never moves the
//     anchor to a smaller row than it stood on;
//   - it costs the row nothing: no drawn row exceeds its terminal, and the
//     note still names what Space acted on.
//
// It refuses to be vacuous two ways: at least 100 Space presses are walked,
// and at least 8 of them act with the cursor inside the document's last
// screenful — the case the fix is about.
func TestSpaceKeepsTheReaderCursorOnThePage(t *testing.T) {
	sweep(t)
	presses, atEnd, marked := 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		if prof == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range r112ttEndSizes {
				w, h := size[0], size[1]
				for _, route := range r112ttEndRoutes() {
					m := sceneModel(sc, w, h)
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					if m.level != levelReader {
						continue
					}
					doc := m.doc(m.readerWidth())
					if len(doc) == 0 {
						continue
					}
					presses++
					if m.anchor >= len(doc)-m.readerHeight() {
						atEnd++
					}
					note := m.note
					before := m.anchor
					cells := r112ttEndMarkedCells(m.View())

					// Side 1 — the fault.
					switch {
					case before >= 0 && before < len(doc) && len(cells) != 1:
						t.Errorf("%v %s %dx%d after %v: the reader draws %d cursor marks over anchor %d of %d rows (top %d, page %d); note %q",
							prof, sc.name, w, h, route, len(cells), before, len(doc), m.readerTop(doc), m.readerHeight(), note)
					case len(cells) == 1:
						marked++
						cell := cells[0]

						// Side 2 — the right row.
						want := readerRowText(doc, before)
						spaced := strings.Replace(cell, "▸", " ", 1)
						pushed := strings.Replace(cell, "▸", "", 1)
						if f := strings.Fields(want); len(f) > 0 &&
							!strings.Contains(spaced, f[0]) && !strings.Contains(pushed, f[0]) {
							t.Errorf("%v %s %dx%d after %v: the marked row %q is not the anchored row %q",
								prof, sc.name, w, h, route, cell, want)
						}
					}

					// Side 4 — the cost, measured on the frame Space drew.
					for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
						if lipgloss.Width(row) > w {
							t.Errorf("%v %s %dx%d after %v: a drawn row is %d cells in a %d-cell terminal: %q",
								prof, sc.name, w, h, route, lipgloss.Width(row), w, row)
							break
						}
					}
					if note == "" {
						t.Errorf("%v %s %dx%d after %v: Space acted and said nothing",
							prof, sc.name, w, h, route)
					}

					// Side 3 — the press after it walks forward. Measured on
					// every stand, not only the ones side 1 passed: where the
					// mark is off the page it is exactly this that goes wrong.
					pressKey(m, "j")
					poll(m, sc)
					if before >= 0 && m.anchor < before {
						t.Errorf("%v %s %dx%d after %v: `j` moved the cursor from row %d back to row %d",
							prof, sc.name, w, h, route, before, m.anchor)
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}

	t.Logf("Space presses walked: %d · with the cursor in the last screenful: %d · stands drawing a mark: %d",
		presses, atEnd, marked)
	if presses < 100 {
		t.Errorf("this pin walked %d Space presses, too few to hold anything", presses)
	}
	if atEnd < 8 {
		t.Errorf("this pin unfolded with the cursor in the last screenful %d times, too few to be about the defect it names", atEnd)
	}
}
