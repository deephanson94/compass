package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// jumpTestWidths is the deck's own width ladder (scenario_test.go's), so a
// jump key is pinned at every shape the deck actually draws.
var jumpTestWidths = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// docNonBlankRow finds the first row in dir (+1 from the front, -1 from the
// back) that is not blank filler, independent of the fix's own helpers — a
// direct read of what "the first/last row of the document" means, so the
// test does not just check the production code against itself.
func docNonBlankRow(doc []readerLine, dir int) int {
	start, end, step := 0, len(doc), 1
	if dir < 0 {
		start, end, step = len(doc)-1, -1, -1
	}
	for i := start; i != end; i += step {
		if doc[i].kind != readerBlank {
			return i
		}
	}
	return -1
}

// jumpToReader drives sc to Lv3 on its default selection, at w×h.
func jumpToReader(sc scene, w, h int) *Model {
	m := sceneModel(sc, w, h)
	toLv3(m)
	return m
}

// TestGLandsTheReaderCursorOnTheFirstRow pins #313's raised defect: `g` is
// named for the start of the conversation, and now takes the mark there
// (markOldestLine), not only the viewport — at every width, on a scene that
// scrolls (very-long) and one that fits whole on screen (first-session).
func TestGLandsTheReaderCursorOnTheFirstRow(t *testing.T) {
	forceASCII(t)
	for _, sc := range []scene{sceneVeryLong(), sceneFirstSession()} {
		for _, size := range jumpTestWidths {
			w, h := size[0], size[1]
			m := jumpToReader(sc, w, h)
			doc := m.doc(m.readerWidth())
			want := docNonBlankRow(doc, 1)
			if want < 0 {
				t.Fatalf("%s %dx%d: document has no row at all", sc.name, w, h)
			}
			pressKey(m, "g")
			if m.anchor != want {
				t.Errorf("%s %dx%d: after g the cursor stands on row %d, want %d (the first non-blank row)", sc.name, w, h, m.anchor, want)
			}
			if doc[m.anchor].kind == readerBlank {
				t.Errorf("%s %dx%d: after g the cursor stands on a blank row", sc.name, w, h)
			}
		}
	}
}

// TestGCapLandsTheReaderCursorOnTheLastRow is g's mirror: `G` is named for
// the end of the conversation, and the only `▸` used to stand on the first
// row regardless — on a fitting page the footer said "end of the
// conversation" while the mark sat under "the start of the conversation"
// two lines above it (#313, reproduced on first-session at 100x30 before
// this fix).
func TestGCapLandsTheReaderCursorOnTheLastRow(t *testing.T) {
	forceASCII(t)
	for _, sc := range []scene{sceneVeryLong(), sceneFirstSession()} {
		for _, size := range jumpTestWidths {
			w, h := size[0], size[1]
			m := jumpToReader(sc, w, h)
			doc := m.doc(m.readerWidth())
			want := docNonBlankRow(doc, -1)
			if want < 0 {
				t.Fatalf("%s %dx%d: document has no row at all", sc.name, w, h)
			}
			pressKey(m, "G")
			if m.anchor != want {
				t.Errorf("%s %dx%d: after G the cursor stands on row %d, want %d (the last non-blank row)", sc.name, w, h, m.anchor, want)
			}
			if doc[m.anchor].kind == readerBlank {
				t.Errorf("%s %dx%d: after G the cursor stands on a blank row", sc.name, w, h)
			}
		}
	}
}

// bareCursorCell reports the first frame line whose reader column is
// nothing but the cursor mark — markAnchor's own drawing of a blank row,
// the "▸ on the air between blocks" defect (#313's second gap, and the
// reviewer's Lv2 companion widening of it). A row that carries the mark
// beside real words never reads this way; only a blank one does.
func bareCursorCell(frame string) (line string, found bool) {
	for _, l := range strings.Split(frame, "\n") {
		for _, cell := range strings.Split(l, "│") {
			if strings.TrimSpace(ansi.Strip(cell)) == "▸" {
				return l, true
			}
		}
	}
	return "", false
}

// forceTrueColor is forceASCII's opposite: the mark is a literal rune either
// way (SPEC §4), and #313's sweep is asked to hold under both.
func forceTrueColor(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

// TestSpaceNeverLeavesTheReaderCursorOnABlankRow sweeps a few scenes, every
// width, a handful of short key routes ending in Space, under both colour
// profiles: after folding or unfolding, the document's row indices shift
// under the cursor, and it must land on a row of words — never blank air
// (`very-long` at 100x30 with `k,k,space` was exactly this before the fix).
func TestSpaceNeverLeavesTheReaderCursorOnABlankRow(t *testing.T) {
	routes := [][]string{
		{"space"},
		{"k", "k", "space"},
		{"j", "j", "space"},
		{"k", "k", "space", "space"},
		{"j", "j", "j", "j", "space", "k", "space"},
		{"G", "k", "k", "space"},
		{"g", "j", "j", "space"},
	}
	scenes := []scene{sceneVeryLong(), sceneSubagents(), sceneSecondDay()}
	for _, profile := range []struct {
		name  string
		force func(*testing.T)
	}{{"ascii", forceASCII}, {"truecolor", forceTrueColor}} {
		t.Run(profile.name, func(t *testing.T) {
			profile.force(t)
			for _, sc := range scenes {
				for _, size := range jumpTestWidths {
					w, h := size[0], size[1]
					for _, route := range routes {
						m := jumpToReader(sc, w, h)
						if len(m.doc(m.readerWidth())) == 0 {
							continue // no document to fold (the empty-lane page)
						}
						for _, k := range route {
							pressKey(m, k)
						}
						frame := m.View()
						if line, bad := bareCursorCell(frame); bad {
							t.Fatalf("%s/%s %s %dx%d %v: cursor mark stands alone on a blank row:\n%s", profile.name, sc.name, sc.name, w, h, route, line)
						}
					}
				}
			}
		})
	}
}

// TestLv2CompanionCursorNeverOnABlankRow pins the widened form of #313's
// second gap: at Lv2 in the archive's three-column layout (fleet, trail,
// reader companion) on a terminal between 110 and 150 columns, the reader's
// own width arithmetic (readerWidth) anticipates the two-column Lv3 layout
// and returns 77 cells, while the companion is actually drawn at the
// three-column width, 44 — a document rewrapped that much narrower moves
// every row after the difference, and the anchor's raw index landed on the
// blank line between two blocks (second-day at 120x34, reproduced on HEAD
// by the route below).
func TestLv2CompanionCursorNeverOnABlankRow(t *testing.T) {
	forceASCII(t)
	sc := sceneSecondDay()
	keys := append(append([]string{}, canonicalKeys...), "esc")
	keys = append(keys, sc.extra[:len(sc.extra)-1]...) // sc.extra's own route, short of its last `tab` (which would leave Lv2 for Lv3)
	m := sceneModel(sc, 120, 34)
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
	}
	if m.level != levelWaypoints {
		t.Fatalf("the route no longer lands on Lv2: level=%d", m.level)
	}
	frame := m.View()
	if line, bad := bareCursorCell(frame); bad {
		t.Fatalf("the Lv2 companion reader's cursor stands alone on a blank row:\n%s", line)
	}
}
