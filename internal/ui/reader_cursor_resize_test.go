package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// r112ttEndSizes is the deck's own width ladder.
var r112ttEndSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r112ttEndRoutes are ways down to a reader standing at the end of its
// conversation — the trail's companion and a lane of its own, reached by
// `G` and by the turn keys — each ending on the Space this pin is about.
func r112ttEndRoutes() [][]string {
	return [][]string{
		{"1", "tab", "tab", "G", "tab", "space"},
		{"1", "tab", "tab", "tab", "G", "space"},
		{"2", "tab", "tab", "tab", "G", "space"},
		{"3", "tab", "tab", "tab", "G", "space"},
		{"1", "tab", "tab", "tab", "G", "space", "space"},
	}
}

// r112ttEndMarkedCells are the reader panel's own drawn rows carrying the
// cursor: at 80 and 100 the reader owns the screen, wider it stands right of
// the last panel rule, so the trail's own mark is never counted as one.
func r112ttEndMarkedCells(frame string) []string {
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

// ---- round 112, fleet-hygiene (converged with #319 and #320; kept as a guard) ----
// r112fhSizes is the deck's own width ladder, so the reader's follow is
// pinned at every shape the deck draws — the defect this pins was two
// widths wide and would have hidden at any one of them.
var r112fhSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r112fhWalk is a mixed cursor walk: a few single rows, then a page. The
// page key is the one that overshot, and it only overshot from a mark
// already low on a scrolling page, which is what the `j`s put it there
// for. `ctrl+u` and `k` walk the same ground back the other way.
// r112fhShrinks are terminals the deck can find itself in mid-read: a
// window snapped narrow, a tmux pane unzoomed, a font stepped up.
var r112fhShrinks = [][2]int{{90, 12}, {120, 20}}

var r112fhWalk = []string{
	"j", "j", "ctrl+d", "j", "ctrl+d", "ctrl+d", "j", "j", "ctrl+d",
	"k", "ctrl+u", "k", "k", "ctrl+u", "ctrl+u", "j", "ctrl+d", "j", "ctrl+d",
}

// r112fhReaderColumn is the frame's reader column: the reader is the last
// panel at every width — beside the trail's companion from 120 up, alone
// below it — so the mark is looked for after the row's last panel rule and
// never in the trail's own cursor.
func r112fhReaderColumn(line string) string {
	cells := strings.Split(ansi.Strip(line), "│")
	return cells[len(cells)-1]
}

// r112fhMarkedRow is the drawn reader line carrying the cursor, if the
// frame draws one at all.
func r112fhMarkedRow(frame string) (string, bool) {
	for _, l := range strings.Split(frame, "\n") {
		if col := r112fhReaderColumn(l); strings.Contains(col, "▸") {
			return col, true
		}
	}
	return "", false
}

// r112fhSaysRow reports whether the drawn, cursor-bearing line carries the
// document row's own opening words: the mark replaces a cell rather than
// hiding one (#305), so the row's words survive it.
func r112fhSaysRow(line, text string) bool {
	flat := strings.ReplaceAll(line, "▸", " ")
	want := strings.TrimSpace(text)
	if r := []rune(want); len(r) > 12 {
		want = string(r[:12])
	}
	return want != "" && strings.Contains(flat, want)
}

// TestTheReaderCursorStaysOnThePageTheFrameDraws pins the reader's cursor
// keys against #300's rule: the page a cursor key moves to is the page the
// mark is drawn on.
//
// `readerCursorMove` followed the cursor by setting `m.scroll`, but the
// page the frame draws is `readerTopIn`'s, which walks that raw scroll back
// off a transcript blank and off a result whose call is the row above (`readerTopIn`). Where it walked back, `ctrl+d` from a mark low on a scrolling
// page left the cursor one row below the drawn page: the body did not move
// a line, the mark left the frame altogether, the reader's title clause
// named a row the frame did not draw, and there was no note — four of the
// walkthrough's 608 Lv3 stands, in two scenes at two widths, identical
// under both colour profiles (#317, recorded and not cut). Three sides,
// both profiles (#215, #218), on every scene the deck has:
//
//  1. after every cursor press the mark is inside the page the frame
//     draws, so the frame is never a reader page with no cursor on it;
//  2. the frame draws it, in the reader's own column;
//  3. the drawn mark stands on the row the model counts as the cursor;
//  4. `space` on the document's last row — whose unfolded rows take the
//     page off its tail — keeps the mark on the page, so the next `j` steps
//     forward rather than back to the page's top (#316, recorded);
//  5. and a terminal resized under an open reader keeps it too (#319).
func TestTheReaderCursorStaysOnThePageTheFrameDraws(t *testing.T) {
	sweep(t)
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"mono", termenv.Ascii}, {"colour", termenv.TrueColor}} {
		if prof.p == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
		prev := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof.p)
		presses, drawn, scrolling, ends, resizes := 0, 0, 0, 0, 0
		for _, sc := range allScenes() {
			for _, size := range r112fhSizes {
				w, h := size[0], size[1]
				m := sceneModel(sc, w, h)
				toLv3(m)
				if len(m.doc(m.readerWidth())) == 0 {
					continue // a lane whose agent has written nothing
				}
				if len(m.doc(m.readerWidth())) > m.readerHeight() {
					scrolling++
				}
				for step, k := range r112fhWalk {
					pressKey(m, k)
					doc := m.doc(m.readerWidth())
					if len(doc) == 0 {
						break
					}
					top, height := m.readerTop(doc), m.readerHeight()
					at := m.anchor
					where := prof.name + " " + sc.name
					// (1) the mark is on the page the frame draws
					if at < top || at >= top+height {
						t.Errorf("%s %dx%d: after %s (step %d) the cursor stands on row %d, off the drawn page [%d,%d); note %q",
							where, w, h, k, step, at, top, top+height, m.note)
						continue
					}
					presses++
					// The frame itself is drawn where the page can move —
					// a conversation that fits draws its whole self and
					// its top is 0 whatever the scroll — and once per
					// reader besides, so a fitting page is seen too. The
					// frame is what costs the time here; this spends it
					// where the mark can be lost.
					if len(doc) <= height && step > 0 {
						continue
					}
					drawn++
					// (2) and the frame draws it
					line, ok := r112fhMarkedRow(m.View())
					if !ok {
						t.Errorf("%s %dx%d: after %s (step %d) the reader draws no cursor at all; note %q",
							where, w, h, k, step, m.note)
						continue
					}
					// (3) on the row the model counts
					if !r112fhSaysRow(line, doc[at].text) {
						t.Errorf("%s %dx%d: after %s (step %d) the drawn cursor is on %q, not on row %d %q",
							where, w, h, k, step, strings.TrimSpace(line), at, strings.TrimSpace(doc[at].text))
					}
				}
				// (4) and the fold's own repair keeps the mark on the page
				m = sceneModel(sc, w, h)
				toLv3(m)
				doc := m.doc(m.readerWidth())
				if len(doc) == 0 {
					continue
				}
				pressKey(m, "G")
				pressKey(m, "space")
				doc = m.doc(m.readerWidth())
				at, top, height := m.anchor, m.readerTop(doc), m.readerHeight()
				ends++
				if at < top || at >= top+height {
					t.Errorf("%s %s %dx%d: `space` on the document's last row left the cursor on row %d, off the drawn page [%d,%d); note %q",
						prof.name, sc.name, w, h, at, top, top+height, m.note)
					continue
				}
				if _, ok := r112fhMarkedRow(m.View()); !ok {
					t.Errorf("%s %s %dx%d: after `space` on the document's last row the reader draws no cursor at all; note %q",
						prof.name, sc.name, w, h, m.note)
					continue
				}
				was := m.anchor
				pressKey(m, "j")
				if m.anchor < was {
					t.Errorf("%s %s %dx%d: after `space` on the document's last row `j` stepped the cursor from row %d back to row %d",
						prof.name, sc.name, w, h, was, m.anchor)
				}
				// (5) and a resize under the open reader keeps it
				for _, to := range r112fhShrinks {
					m = sceneModel(sc, w, h)
					toLv3(m)
					if len(m.doc(m.readerWidth())) == 0 {
						continue
					}
					pressKey(m, "ctrl+d")
					pressKey(m, "j")
					if _, ok := r112fhMarkedRow(m.View()); !ok {
						continue
					}
					stood := m.anchorAt
					m.Update(tea.WindowSizeMsg{Width: to[0], Height: to[1]})
					doc := m.doc(m.readerWidth())
					if len(doc) == 0 {
						continue
					}
					resizes++
					_ = stood
					if _, ok := r112fhMarkedRow(m.View()); !ok {
						t.Errorf("%s %s %dx%d resized to %dx%d: the reader draws no cursor at all",
							prof.name, sc.name, w, h, to[0], to[1])
					}
				}
			}
		}
		if presses < 600 || drawn < 150 || scrolling < 8 || ends < 40 || resizes < 50 {
			t.Errorf("%s: the pin measured %d presses, %d of them on a drawn frame, over %d scrolling readers, %d ends and %d resizes — too few to mean anything",
				prof.name, presses, drawn, scrolling, ends, resizes)
		}
		lipgloss.SetColorProfile(prev)
	}
}

// ---- round 112, fleet-hygiene, the rewrap ----
// r112fhbFrom is the deck's own width ladder: every shape a reader can be
// standing in when the window changes.
var r112fhbFrom = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r112fhbTo are shapes it can change to — one wider, two narrower, two
// shorter — so the rewrap goes both ways and a height-only change is
// covered too.
var r112fhbTo = [][2]int{{80, 24}, {90, 12}, {120, 20}, {152, 18}, {220, 16}}

// r112fhbMarked is the drawn reader line carrying the cursor. The reader is
// the frame's last panel at every width — beside the trail's companion from
// 120 up, alone below it — so the mark is looked for after the row's last
// panel rule and never in the trail's own cursor.
func r112fhbMarked(frame string) (string, bool) {
	for _, l := range strings.Split(frame, "\n") {
		cells := strings.Split(ansi.Strip(l), "│")
		if col := cells[len(cells)-1]; strings.Contains(col, "▸") {
			return strings.TrimSpace(strings.ReplaceAll(col, "▸", " ")), true
		}
	}
	return "", false
}

// r112fhbLastRow is the document's last row that is not a transcript blank
// — the row `end of the conversation` is a word about.
func r112fhbLastRow(doc []readerLine) int {
	for i := len(doc) - 1; i >= 0; i-- {
		if doc[i].kind != readerBlank {
			return i
		}
	}
	return -1
}

// TestTheReaderMarkKeepsItsRowWhenTheWidthChanges pins the reader's cursor
// across a rewrap.
//
// #319 brought the page back to the mark when the window changed size, and
// #300's `▸` is on the frame again — but the mark it comes back on is found
// by its row *number*, and a row number belongs to one wrapping. Rewrap the
// document at another width and the same index is a different line: on
// `many-idle` the reader standing on ` ▸⎿ edited · +1 −1`, the conversation's
// last row, came back at another width on ` ▸Writing loader.py.` — two rows
// and twenty minutes earlier — under the note `end of the conversation`,
// which the frame then drew rows below. 43 of 360 resize pairs, both colour
// profiles (#215, #218). The mark is found again by what it stood on: its
// own moment, and among that moment's rows the one whose text it was.
//
// Four sides, nine scenes, five widths, five shapes to change into:
//
//  1. the resized frame draws a cursor at all (#319's own ground);
//  2. it stands on a row of the moment it stood on before;
//  3. where that moment's row still says what it said, on that very row;
//  4. and a note the resize carries over is still true of it — where the
//     row says `end of the conversation` the mark has not left the last
//     moment the conversation has.
func TestTheReaderMarkKeepsItsRowWhenTheWidthChanges(t *testing.T) {
	sweep(t)
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"mono", termenv.Ascii}, {"colour", termenv.TrueColor}} {
		if prof.p == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
		prev := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof.p)
		pairs, ends := 0, 0
		for _, sc := range allScenes() {
			for _, from := range r112fhbFrom {
				for _, to := range r112fhbTo {
					w, h := from[0], from[1]
					m := sceneModel(sc, w, h)
					toLv3(m)
					if len(m.doc(m.readerWidth())) == 0 {
						continue // a lane whose agent has written nothing
					}
					pressKey(m, "G") // the conversation's end: a row the note names
					was, ok := r112fhbMarked(m.View())
					if !ok {
						continue
					}
					stood, stoodText, note := m.anchorAt, m.anchorText, m.note
					m.Update(tea.WindowSizeMsg{Width: to[0], Height: to[1]})
					doc := m.doc(m.readerWidth())
					if len(doc) == 0 {
						continue
					}
					pairs++
					where := prof.name + " " + sc.name
					// (1) the frame draws a cursor
					now, ok := r112fhbMarked(m.View())
					if !ok {
						t.Errorf("%s %dx%d resized to %dx%d: the reader draws no cursor at all; note %q",
							where, w, h, to[0], to[1], m.note)
						continue
					}
					at := m.anchor
					if at < 0 || at >= len(doc) {
						t.Errorf("%s %dx%d resized to %dx%d: the cursor stands on row %d of a document of %d",
							where, w, h, to[0], to[1], at, len(doc))
						continue
					}
					// (2) on a row of the moment it stood on
					if !stood.IsZero() && !doc[at].at.Equal(stood) {
						t.Errorf("%s %dx%d resized to %dx%d: the mark stood on %q at %s and came back on %q at %s",
							where, w, h, to[0], to[1], was, stood.Format("15:04"), now, doc[at].at.Format("15:04"))
						continue
					}
					// (3) and on the very row, where that row still exists
					if stoodText != "" && readerRowText(doc, at) != stoodText {
						found := false
						for i := range doc {
							if doc[i].at.Equal(stood) && readerRowText(doc, i) == stoodText {
								found = true
								break
							}
						}
						if found {
							t.Errorf("%s %dx%d resized to %dx%d: the mark left %q for %q though the row is still drawn",
								where, w, h, to[0], to[1], was, now)
						}
					}
					// (4) and where the moment it stood on is the last the
					// document has, the mark is still inside it, so a row
					// saying `end of the conversation` is not drawn over a
					// mark that has left the end
					if strings.Contains(m.note, "end of the conversation") && note == m.note {
						ends++
						if last := r112fhbLastRow(doc); last >= 0 && !doc[last].at.Equal(doc[at].at) {
							t.Errorf("%s %dx%d resized to %dx%d: the row says %q and the mark stands on %q, of an earlier moment than the last row %q",
								where, w, h, to[0], to[1], m.note, now, strings.TrimSpace(doc[last].text))
						}
					}
				}
			}
		}
		if pairs < 150 || ends < 40 {
			t.Errorf("%s: the pin measured %d resize pairs and %d ends — too few to mean anything", prof.name, pairs, ends)
		}
		lipgloss.SetColorProfile(prev)
	}
}

// ---- round 113, fleet-hygiene (the reseat) and second-day (kept as a guard) ----
// r113fhSizes are the terminals a reader is opened in before the window is
// dragged; r113fhDrags are the sizes it is dragged through, in order, so the
// mark is asked to survive a chain of changes and not just one.
var r113fhSizes = [][2]int{{80, 24}, {120, 34}, {220, 48}}

var r113fhDrags = [][2]int{{70, 24}, {90, 30}, {110, 18}, {152, 40}, {200, 20}, {100, 30}, {90, 12}, {152, 44}}

// r113fhWalks put the cursor somewhere worth losing: where it opens, low on
// a scrolling page, and on the conversation's very last row.
var r113fhWalks = [][]string{{"ctrl+d", "j"}, {"G"}}

// r113fhReaderColumn is the frame's reader column: the reader is the last
// panel at every width, so the mark is looked for after the row's last panel
// rule and never in the trail's own cursor.
func r113fhReaderColumn(line string) string {
	cells := strings.Split(ansi.Strip(line), "│")
	return cells[len(cells)-1]
}

// r113fhMarkedRows is every drawn reader line carrying the cursor.
func r113fhMarkedRows(frame string) []string {
	var out []string
	for _, l := range strings.Split(frame, "\n") {
		if col := r113fhReaderColumn(l); strings.Contains(col, "▸") {
			out = append(out, col)
		}
	}
	return out
}

// r113fhLastRow is the document's last row a cursor can stand on.
func r113fhLastRow(doc []readerLine) int {
	for i := len(doc) - 1; i >= 0; i-- {
		if doc[i].kind != readerBlank {
			return i
		}
	}
	return -1
}

// r113fhSaysRow reports whether the drawn, cursor-bearing line carries the
// document row's own opening words: the mark replaces a cell rather than
// hiding one (#305), so the row's words survive it.
func r113fhSaysRow(line, text string) bool {
	flat := strings.ReplaceAll(line, "▸", " ")
	want := strings.TrimSpace(text)
	if r := []rune(want); len(r) > 12 {
		want = string(r[:12])
	}
	return want != "" && strings.Contains(flat, want)
}

// TestTheReaderCursorComesBackOnItsOwnRowWhenTheWindowChangesSize pins the
// reader's cursor against the rewrap.
//
// #319 brought the reader's *page* back to its cursor after a size change
// and left the cursor's row number alone. A row number belongs to one
// wrapping: dragged to another width, the same index is a different line of
// the conversation, so the mark came back on a different moment — on
// `many-idle` at 100×30 dragged to 90 wide it left ` ▸⎿ edited · +1 −1`
// (17:02) and came back on ` ⏺▸Edit(loader.py)` (16:42), with `⎿ edited ·
// +1 −1` drawn *below* it and the note beside it still reading `end of the
// conversation`. Four sides, both colour profiles (#215, #218), every scene
// the deck has:
//
//  1. after every size change the mark stands on the same transcript event
//     it stood on before the window moved;
//  2. the frame draws exactly one mark, in the reader's own column, on the
//     row the model counts as the cursor;
//  3. the mark is inside the page the frame draws;
//  4. where the note says `end of the conversation` the mark is on the
//     document's last row, so the frame does not say two things at once.
func TestTheReaderCursorComesBackOnItsOwnRowWhenTheWindowChangesSize(t *testing.T) {
	sweep(t)
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"mono", termenv.Ascii}, {"colour", termenv.TrueColor}} {
		if prof.p == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
		prev := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof.p)
		drags, ends, rewraps := 0, 0, 0
		for _, sc := range allScenes() {
			for _, size := range r113fhSizes {
				for _, walk := range r113fhWalks {
					w, h := size[0], size[1]
					m := sceneModel(sc, w, h)
					toLv3(m)
					doc := m.doc(m.readerWidth())
					if len(doc) == 0 {
						continue // a lane whose agent has written nothing (#313)
					}
					for _, k := range walk {
						pressKey(m, k)
						poll(m, sc)
					}
					doc = m.doc(m.readerWidth())
					at := m.readerAnchorAt(doc)
					if at < 0 || at >= len(doc) || doc[at].event < 0 {
						continue
					}
					want, wantText := doc[at].event, readerRowText(doc, at)
					rows := len(doc)
					where := prof.name + " " + sc.name
					for _, to := range r113fhDrags {
						m.SetSize(to[0], to[1])
						poll(m, sc)
						frame := m.View()
						doc = m.doc(m.readerWidth())
						if len(doc) == 0 {
							break
						}
						if len(doc) != rows {
							rewraps++
							rows = len(doc)
						}
						drags++
						got := m.readerAnchorAt(doc)
						if got < 0 || got >= len(doc) {
							t.Errorf("%s %dx%d %v dragged to %dx%d: the model has no cursor row at all",
								where, w, h, walk, to[0], to[1])
							continue
						}
						// (1) the same moment of the conversation
						if doc[got].event != want {
							t.Errorf("%s %dx%d %v dragged to %dx%d: the cursor left %q and came back on %q — a different moment",
								where, w, h, walk, to[0], to[1], wantText, readerRowText(doc, got))
						}
						// (2) one mark, in the reader's column, on that row
						marks := r113fhMarkedRows(frame)
						if len(marks) != 1 {
							t.Errorf("%s %dx%d %v dragged to %dx%d: the frame draws %d cursor marks in the reader column, want exactly one; note %q",
								where, w, h, walk, to[0], to[1], len(marks), m.note)
							continue
						}
						if !r113fhSaysRow(marks[0], readerRowText(doc, got)) {
							t.Errorf("%s %dx%d %v dragged to %dx%d: the drawn mark is on %q, the model counts row %d %q",
								where, w, h, walk, to[0], to[1], strings.TrimSpace(marks[0]), got, readerRowText(doc, got))
						}
						// (3) on the page the frame draws
						top, height := m.readerTop(doc), m.readerHeight()
						if got < top || got >= top+height {
							t.Errorf("%s %dx%d %v dragged to %dx%d: the cursor stands on row %d, off the drawn page [%d,%d)",
								where, w, h, walk, to[0], to[1], got, top, top+height)
						}
						// (4) the end-note is about the cursor's row (#309)
						if m.note == "end of the conversation" {
							ends++
							if last := r113fhLastRow(doc); got != last {
								t.Errorf("%s %dx%d %v dragged to %dx%d: the note says %q and the mark stands on row %d %q, with row %d %q drawn below it",
									where, w, h, walk, to[0], to[1], m.note, got, readerRowText(doc, got), last, readerRowText(doc, last))
							}
						}
					}
				}
			}
		}
		if drags < 300 {
			t.Errorf("%s: only %d size changes measured — the sweep went vacuous", prof.name, drags)
		}
		if rewraps < 60 {
			t.Errorf("%s: only %d of those re-wrapped the document — the sweep never tested the rewrap", prof.name, rewraps)
		}
		if ends < 5 {
			t.Errorf("%s: only %d stands carried the end-note — side 4 went vacuous", prof.name, ends)
		}
		lipgloss.SetColorProfile(prev)
	}
}

// TestTheReaderCursorIsARowOfTheDocumentTheFrameDraws pins the reader's
// stored cursor to the document the frame is about to draw (#319's site).
//
// A window that grows rewraps the conversation into fewer rows — the commit
// call that takes two lines at 152 takes one at 220 — so a document read to
// its end loses a row under a mark standing on its last. `readerAnchorAt`
// clamped for the drawing, so the `▸` came back on the row it stood on and
// the frame looked right; the stored index kept its old value, one past the
// end. Only the keys knew: `readerCursorMove` throws an out-of-range cursor
// away and restarts from the page's own top, so the `j` the frame's own row
// named stepped the mark nineteen rows *backwards* up the conversation, and
// `space` took #300's top-down fallback and said `unfolded Read(sched.go)`
// over a frame whose mark stood on the commit's result — 547 of the
// corpus's 4 824 (stand, new size) pairs, 4 of my two scenes' 1 424.
//
// Three sides, both colour profiles (#215, #218):
//
//  1. the frame it was found on — the archive's `api` reader at 152x40 read
//     to its end and widened to 220x48: the mark on the row it stood on, the
//     model counting that same row, `j` stepping forward from it and `space`
//     naming the call the mark stands on;
//  2. the rule — every Lv3 stand of the canonical walkthrough over
//     `second-day` and `first-session` at five widths, read to its end and
//     resized to each of the other four: the stored cursor is a row of the
//     new document and is the row the frame draws the mark on, and the
//     first `j` after the resize never steps backwards;
//  3. the price — where the new size rewraps the conversation into the same
//     number of rows, the mark does not move at all: the words the cursor
//     stood on before the resize are the words it stands on after. (Where
//     the rewrap splits or joins a row the index cannot mean the same row,
//     and side 2 is what holds there.)
func TestTheReaderCursorIsARowOfTheDocumentTheFrameDraws(t *testing.T) {
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"mono", termenv.Ascii}, {"colour", termenv.TrueColor}} {
		prev := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof.p)

		// (1) the frame it was found on.
		sc, ok := r113sdSceneNamed("second-day")
		if !ok {
			t.Fatalf("%s: the deck has no second-day scene", prof.name)
		}
		keys := r113sdWalk(sc)
		m := r113sdStandAt(sc, 152, 40, 47, keys)
		pressKey(m, "G")
		doc := m.doc(m.readerWidth())
		stood := ""
		if m.anchor >= 0 && m.anchor < len(doc) {
			stood = strings.TrimSpace(doc[m.anchor].text)
		}
		if !strings.Contains(stood, "commit") {
			t.Fatalf("%s: `G` on the archive's api reader at 152x40 left the mark on %q, not on the commit's result", prof.name, stood)
		}
		m.Update(tea.WindowSizeMsg{Width: 220, Height: 48})
		doc = m.doc(m.readerWidth())
		if m.anchor < 0 || m.anchor >= len(doc) {
			t.Errorf("%s: widened to 220x48 the reader's cursor is row %d of a %d-row document", prof.name, m.anchor, len(doc))
		} else {
			if drawn := m.readerAnchorAt(doc); drawn != m.anchor {
				t.Errorf("%s: widened to 220x48 the frame draws the mark on row %d and the model counts row %d", prof.name, drawn, m.anchor)
			}
			if got := strings.TrimSpace(doc[m.anchor].text); got != stood {
				t.Errorf("%s: widened to 220x48 the cursor stands on %q, not on the row it was left on, %q", prof.name, got, stood)
			}
			line, found := r113sdMarkedRow(m.View())
			if !found {
				t.Errorf("%s: widened to 220x48 the reader draws no cursor at all", prof.name)
			} else if !r113sdSaysRow(line, doc[m.anchor].text) {
				t.Errorf("%s: widened to 220x48 the drawn mark is on %q, not on row %d %q", prof.name, strings.TrimSpace(line), m.anchor, stood)
			}
		}
		was := m.anchor
		after := *m
		pressKey(&after, "j")
		if after.anchor < was {
			t.Errorf("%s: widened to 220x48 `j` stepped the cursor from row %d back to row %d", prof.name, was, after.anchor)
		}
		folded := *m
		pressKey(&folded, "space")
		if verb := folded.note; strings.HasPrefix(verb, "unfolded ") || strings.HasPrefix(verb, "folded ") {
			name := strings.TrimSpace(strings.SplitN(verb, " ", 2)[1])
			if head := strings.SplitN(name, "(", 2)[0]; head != "" && !r113sdNear(doc, was, head) {
				t.Errorf("%s: widened to 220x48 `space` said %q over a mark standing on %q", prof.name, verb, stood)
			}
		}

		// (2) the rule, and (3) the price.
		pairs, moved := 0, 0
		for _, name := range []string{"second-day", "first-session"} {
			sc, ok := r113sdSceneNamed(name)
			if !ok {
				continue
			}
			keys := r113sdWalk(sc)
			for _, from := range r113sdReaderSizes {
				w, h := from[0], from[1]
				probe := sceneModel(sc, w, h)
				var stands []int
				for i, k := range keys {
					pressKey(probe, k)
					poll(probe, sc)
					if probe.level >= levelReader {
						stands = append(stands, i+1)
					}
				}
				for _, n := range stands {
					for _, to := range r113sdReaderSizes {
						if to == from {
							continue
						}
						m := r113sdStandAt(sc, w, h, n, keys)
						pressKey(m, "G")
						doc := m.doc(m.readerWidth())
						if len(doc) == 0 || m.anchor < 0 || m.anchor >= len(doc) {
							continue
						}
						stood, wasLen := strings.TrimSpace(doc[m.anchor].text), len(doc)
						m.Update(tea.WindowSizeMsg{Width: to[0], Height: to[1]})
						doc = m.doc(m.readerWidth())
						if len(doc) == 0 || m.level < levelReader {
							continue
						}
						pairs++
						where := prof.name + " " + name
						if m.anchor < 0 || m.anchor >= len(doc) {
							t.Errorf("%s %dx%d step %d resized to %dx%d: the cursor is row %d of a %d-row document",
								where, w, h, n, to[0], to[1], m.anchor, len(doc))
							continue
						}
						if drawn := m.readerAnchorAt(doc); drawn != m.anchor {
							t.Errorf("%s %dx%d step %d resized to %dx%d: the frame draws the mark on row %d, the model counts row %d",
								where, w, h, n, to[0], to[1], drawn, m.anchor)
							continue
						}
						if got := strings.TrimSpace(doc[m.anchor].text); len(doc) == wasLen && got != stood {
							moved++
							t.Errorf("%s %dx%d step %d resized to %dx%d: the document kept its %d rows and the cursor still moved, to %q from %q",
								where, w, h, n, to[0], to[1], wasLen, got, stood)
						}
						was := m.anchor
						pressKey(m, "j")
						if m.anchor < was {
							t.Errorf("%s %dx%d step %d resized to %dx%d: `j` stepped the cursor from row %d back to row %d",
								where, w, h, n, to[0], to[1], was, m.anchor)
						}
					}
				}
			}
		}
		if pairs < 300 {
			t.Errorf("%s: the pin measured %d (stand, new size) pairs — too few to mean anything", prof.name, pairs)
		}
		t.Logf("%s: %d (stand, new size) pairs measured, %d moved the mark", prof.name, pairs, moved)
		lipgloss.SetColorProfile(prev)
	}
}

// r113sdReaderSizes are the five terminals the walkthrough is drawn at.
var r113sdReaderSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r113sdSceneNamed finds one of the deck's scenes by name.
func r113sdSceneNamed(name string) (scene, bool) {
	for _, sc := range allScenes() {
		if sc.name == name {
			return sc, true
		}
	}
	return scene{}, false
}

// r113sdWalk is the canonical walkthrough plus the scene's own tail.
func r113sdWalk(sc scene) []string {
	return append(append(append([]string(nil), canonicalKeys...), "esc"), sc.extra...)
}

// r113sdStandAt replays n keys of the walk on a fresh model.
func r113sdStandAt(sc scene, w, h, n int, keys []string) *Model {
	m := sceneModel(sc, w, h)
	for i := 0; i < n && i < len(keys); i++ {
		pressKey(m, keys[i])
		poll(m, sc)
	}
	return m
}

// r113sdMarkedRow is the drawn reader line carrying the cursor. The reader
// is the frame's last panel at every width, so the mark is looked for after
// the row's last column rule and never in the trail's own cursor.
func r113sdMarkedRow(frame string) (string, bool) {
	for _, l := range strings.Split(frame, "\n") {
		cells := strings.Split(ansi.Strip(l), "│")
		if col := cells[len(cells)-1]; strings.Contains(col, "▸") {
			return col, true
		}
	}
	return "", false
}

// r113sdSaysRow reports whether the drawn, cursor-bearing line carries the
// document row's own opening words: the mark replaces a cell rather than
// hiding one (#305), so the row's words survive it.
func r113sdSaysRow(line, text string) bool {
	flat := strings.ReplaceAll(line, "▸", " ")
	want := strings.TrimSpace(text)
	if r := []rune(want); len(r) > 12 {
		want = string(r[:12])
	}
	return want != "" && strings.Contains(flat, want)
}

// r113sdNear reports whether the call the fold's note names is the row the
// cursor stands on or the call that row belongs to — a result's own call is
// the row above it.
func r113sdNear(doc []readerLine, row int, head string) bool {
	for i := row; i >= 0 && i > row-3 && i < len(doc); i-- {
		if strings.Contains(doc[i].text, head) {
			return true
		}
	}
	return false
}

// ---- round 113, two-tools ----
// ---- round 113, two-tools ----
// TestTheFoldKeepsTheReaderCursorOnItsOwnRow holds Space to the last of the
// reader cursor's rules: a key that is not a movement key does not move the
// cursor.
//
// The anchor is an index into the document, and a fold above it renumbers
// every row after the fold. toggleFold repaired that index only where it
// came to rest on air (#314) — anywhere else the mark stayed at the same
// number and the number now named a different row. On `two-tools` at 120x34
// the mark stood on the ask's own second line, ` ▸CIDR / keep bastion])`,
// and `space` — with no fold under the cursor, falling back to the first
// folded result on screen seven rows above it (#300) — left the mark on
// `  ▸  5    func main() {`, a line of main.tf, while the note said
// `unfolded Read(main.tf)` and nothing said the cursor had moved.
//
// The row under the mark moves by exactly the rows the fold added or took
// back above it; a cursor above the fold is untouched, as it always was;
// and where the mark stands on the row the fold itself rewrote, the title's
// copy of that row is re-read from it, so `⎿ 215 passed in 4.21s · 1 more
// line` does not stand in the title over a row the same press opened to
// `⎿ 2 lines`.
//
// Four sides, under both colour profiles (#215, #218):
//   - the fault: after Space the marked row carries the words it carried
//     before, wherever the fold was not the cursor's own row;
//   - the frame agrees: exactly one mark is drawn, on the page (#320);
//   - the title says the row it is on: the anchored row's own words;
//   - it costs the frame nothing: no drawn row exceeds its terminal, and
//     Space still says what it did.
//
// It refuses to be vacuous two ways: at least 100 Space presses are walked,
// and at least 8 of them fold above the cursor — the case the fix is about.
func TestTheFoldKeepsTheReaderCursorOnItsOwnRow(t *testing.T) {
	sweep(t)
	forceASCII(t)

	presses, above := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		if prof == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range r113ttFoldSizes {
				w, h := size[0], size[1]
				for _, route := range r113ttFoldRoutes {
					m := r113ttFoldStand(sc, w, h, route)
					if m.level < levelReader {
						continue
					}
					for _, walk := range r113ttFoldWalks {
						for _, k := range walk {
							pressKey(m, k)
							poll(m, sc)
						}
						doc := m.doc(m.readerWidth())
						a0 := m.readerAnchorAt(doc)
						if a0 < 0 || a0 >= len(doc) {
							continue
						}
						wasText, wasEvent := strings.TrimSpace(readerRowText(doc, a0)), doc[a0].event
						wasRow, wasMarks := r113ttFoldMarkedRow(ansi.Strip(m.View()))
						if wasMarks != 1 {
							continue // only stands the frame already marks once
						}
						top, height := m.readerTop(doc), m.readerHeight()
						folded := -1
						if a0 >= top && a0 < top+height && doc[a0].foldable() {
							folded = a0
						} else {
							for j := top; j < len(doc) && j < top+height; j++ {
								if doc[j].foldable() {
									folded = j
									break
								}
							}
						}
						pressKey(m, "space")
						poll(m, sc)
						presses++
						if folded >= 0 && folded < a0 {
							above++
						}
						nd := m.doc(m.readerWidth())
						a1 := m.readerAnchorAt(nd)
						if a1 < 0 || a1 >= len(nd) {
							continue
						}
						nowText := strings.TrimSpace(readerRowText(nd, a1))
						frame := ansi.Strip(m.View())
						row, marks := r113ttFoldMarkedRow(frame)

						// 1 · the cursor did not move: the row it stands on
						// carries the words it carried, unless the press
						// rewrote that row itself.
						if nd[a1].event != wasEvent && nowText != wasText {
							t.Errorf("%v %s %dx%d after %v: Space moved the cursor off its own row — it stood on %q and now stands on %q (note %q)",
								prof, sc.name, w, h, append(append([]string(nil), route...), walk...), wasText, nowText, m.note)
						}
						// 2 · and the frame draws it, once, on the page.
						if marks != 1 {
							t.Errorf("%v %s %dx%d after %v: the reader draws %d cursor marks after Space, want one (it stood on %q)", prof, sc.name, w, h, route, marks, strings.TrimSpace(wasRow))
						}
						if drawn := readerTopIn(nd, m.scroll, m.readerHeight()); a1 < drawn || a1 > drawn+m.readerHeight()-1 {
							t.Errorf("%v %s %dx%d: after Space the cursor's row %d is off the page the frame draws (%d..%d)", prof, sc.name, w, h, a1, drawn, drawn+m.readerHeight()-1)
						}
						// 3 · the title's copy of the row is the row's own
						// words, not the ones the press has just replaced.
						if nd[a1].event == wasEvent && nowText != wasText && strings.TrimSpace(m.anchorText) == wasText {
							t.Errorf("%v %s %dx%d: the title still names %q, the row's words before the press, while that row now reads %q", prof, sc.name, w, h, wasText, nowText)
						}
						// 4 · and it costs the frame nothing.
						for _, l := range strings.Split(frame, "\n") {
							if lipgloss.Width(l) > w {
								t.Errorf("%v %s %dx%d: a row of %d cells after Space: %q", prof, sc.name, w, h, lipgloss.Width(l), l)
								break
							}
						}
						if m.note == "" {
							t.Errorf("%v %s %dx%d: Space says nothing about what it did", prof, sc.name, w, h)
						}
						if marks == 1 && strings.TrimSpace(strings.ReplaceAll(row, "▸", "")) == "" {
							t.Errorf("%v %s %dx%d: the mark stands on a row with no words after Space", prof, sc.name, w, h)
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if presses < 100 {
		t.Errorf("only %d Space presses were walked, want at least 100 — the walk is not reaching the reader", presses)
	}
	if above < 8 {
		t.Errorf("only %d of the Space presses folded above the cursor, want at least 8 — the case the fold is about is not being reached", above)
	}
	t.Logf("Space presses %d · folds above the cursor %d", presses, above)
}

var r113ttFoldSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

var r113ttFoldRoutes = [][]string{
	{"1", "tab", "tab", "tab"},
	{"A", "tab", "tab", "tab"},
}

var r113ttFoldWalks = [][]string{{}, {"j", "j"}}

// r113ttFoldStand drives one scene to a stand and hands back the model.
func r113ttFoldStand(sc scene, w, h int, keys []string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// r113ttFoldReaderCell is the frame row's reader column: the reader is the
// last panel at every width — beside the trail's companion from 120 up,
// alone below it — so the trail's own cursor is never read as the reader's.
func r113ttFoldReaderCell(line string) string {
	cells := strings.Split(line, "│")
	return strings.TrimRight(cells[len(cells)-1], " ")
}

// r113ttFoldMarkedRow is the reader's own cursor row and how many of its
// rows carry a mark.
func r113ttFoldMarkedRow(frame string) (string, int) {
	row, n := "", 0
	for _, l := range strings.Split(frame, "\n") {
		if cell := r113ttFoldReaderCell(l); strings.Contains(cell, "▸") {
			row, n = cell, n+1
		}
	}
	return row, n
}
