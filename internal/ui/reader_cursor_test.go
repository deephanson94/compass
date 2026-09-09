package ui

import (
	"strings"
	"testing"
)

// cursorPinEvents is fixtureEvents' own conversation: a turn, a wrapped
// thought, a clean fold and a failed one — two foldable rows with a call
// between them, so the second is never adjacent to the first (reader.go's
// readerDoc lays it out at document rows 6 and 8).
func cursorPinEvents() []readerLine {
	return readerDoc(fixtureEvents(fixtureBase), ReaderOpts{Width: 60})
}

// cursorPinModel is a Lv3 reader standing on fixtureEvents' document, wide
// enough that all twelve rows are on screen at once — Space's target and
// the mark are both about a row's place in the *document*, not about
// scrolling to it.
func cursorPinModel(t *testing.T) *Model {
	t.Helper()
	m := boardModel(120, 30)
	openTrail(m)
	m.events = fixtureEvents(fixtureBase)
	toLv3(m)
	if m.level != levelReader {
		t.Fatalf("did not reach the reader: Lv%d", m.level)
	}
	doc := m.doc(m.readerWidth())
	if len(doc) != len(cursorPinEvents()) {
		t.Fatalf("the fixture drew %d rows at the reader's own width, want %d — the layout this test assumes has changed", len(doc), len(cursorPinEvents()))
	}
	if !doc[6].foldable() || !doc[8].foldable() {
		t.Fatalf("row 6 or row 8 is not foldable any more — the layout this test assumes has changed:\n%v\n%v", doc[6], doc[8])
	}
	m.scroll = 0
	return m
}

// The reader draws exactly one cursor mark at Lv3 — the '▸' markAnchor cuts
// into the anchored row, the same cell the trail's own cursor spends
// (trailBuilder.cursored) — and it survives ASCII: unlike the reverse video
// alone, the mark is a literal rune, so it is still there with colour off
// (SPEC §4), which was the owner's actual complaint ("I still don't see the
// cursor").
func TestReaderDrawsExactlyOneCursorMark(t *testing.T) {
	forceASCII(t)
	m := cursorPinModel(t)
	m.anchor = 8 // "⎿ ✗ EACCES: permission denied…", not the first row on screen

	doc := m.doc(m.readerWidth())
	frame := RenderReader(m.events, ReaderOpts{
		Width: m.readerWidth(), Height: len(doc),
		Scroll: 0, Unfolded: m.unfolded, Anchor: m.anchor,
	})
	if n := strings.Count(frame, "▸"); n != 1 {
		t.Fatalf("the reader drew %d cursor marks under ASCII, want exactly 1:\n%s", n, frame)
	}
	lines := strings.Split(frame, "\n")
	if !strings.Contains(lines[8], "▸") {
		t.Errorf("the mark landed off row 8:\n%s", frame)
	}
	if !strings.Contains(lines[8], "EACCES") {
		t.Errorf("the marked row lost its own words: %q", lines[8])
	}
}

// `j` moves the reader's cursor down one row (readerCursorMove) — skipping
// the blank line the document opens on, the way scrolling always has.
func TestJMovesTheReaderCursorDownOneRow(t *testing.T) {
	forceASCII(t)
	m := cursorPinModel(t)
	m.anchor = 0 // "❯ fix the 401 bug…"

	press(m, "j")

	if m.anchor != 2 {
		t.Fatalf("j moved the cursor to %d, want 2 (row 1 is blank)", m.anchor)
	}
}

// Space unfolds the collapsed result the cursor stands on, whichever row of
// the screen that is — not the first foldable row from the top (#79's old
// rule, which is exactly the bug: "the space bar to unfold does not work
// for both lines (only top lines) since there's no cursor"). Landing the
// cursor on the *second* fold and pressing Space must open that one and
// leave the first alone.
func TestSpaceUnfoldsTheRowUnderTheCursorNotTheTopOne(t *testing.T) {
	forceASCII(t)
	m := cursorPinModel(t)
	doc := m.doc(m.readerWidth())
	top := m.readerTop(doc)
	if top != 0 {
		t.Fatalf("the page does not open on row 0: top=%d", top)
	}
	m.anchor = 8 // the second fold — row 6 (event 3) is the first, and is row 0's the page opens on it is not
	if m.anchor == top {
		t.Fatalf("row 8 is the top of the page; the test proves nothing")
	}

	firstEvent, secondEvent := doc[6].event, doc[8].event
	if m.unfolded[firstEvent] || m.unfolded[secondEvent] {
		t.Fatalf("the fixture opens with a fold already open")
	}

	press(m, " ")

	if !m.unfolded[secondEvent] {
		t.Errorf("space did not unfold the row under the cursor (row 8, event %d): unfolded=%v", secondEvent, m.unfolded)
	}
	if m.unfolded[firstEvent] {
		t.Errorf("space also unfolded the first result on screen (row 6, event %d) — it should have left it alone", firstEvent)
	}
}
