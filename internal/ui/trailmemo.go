package ui

import (
	"fmt"
	"reflect"
	"strings"
	"unsafe"

	"github.com/deephanson94/compass/internal/journey"
)

// trailMemo keeps the trail's bare document — trailDocBare's answer, the
// rows without the cursor's inversion — between the calls one frame makes
// for it.
//
// A keypress on the trail asked for the whole document six to ten times:
// the column, the title's count of hidden legs, the footer's chapter
// keys, the cursor's own row to keep it on screen, the day clause… each
// through trailDoc, each building every row of the journey again with a
// lipgloss render per row, and a trail longer than its panel built each
// of those three times over (the lane-heads retry, then the dense one).
// At 160 legs that was 78 ms a key; at 1,000 legs, 560 ms; at 3,000, two
// seconds — the lag the person feels walking a long conversation, and it
// was the trail, not the reader beside it (the reader has had its own
// cache since round 68).
//
// The document depends on the cursor in exactly one row, so the memo
// holds the cursorless document and the cursor is laid over it per call
// (withTrailCursor). Everything else it depends on is in the key: every
// scalar of TrailOpts but the cursor and the viewport (which choose rows
// from the document and never change it), the small maps and slices
// whole, and for the trail and the two big maps their size and identity.
// The memo lives until the state it is drawn from changes: every message
// that is not a key retires it (a poll, a tick, a narrated label, a new
// size — Update and every Set*). A key keeps it, and a key that swaps the
// trail or its labels in (a session selected, the board's Tab) misses on
// their identity in the key rather than retiring the other columns' rows.
// The one thing the key cannot see is a label or a lane changed inside
// its map, and nothing does that: both maps are replaced whole, by
// messages. A run of j/k between two ticks pays for each document once.
type trailMemo struct {
	gen  int
	docs map[string]trailMemoDoc
	rows map[string][]TrailRow // TrailRows, by level and trail
}

// trailMemoCap bounds the memo between retirements: the clock is in the
// key, so a deck left alone would otherwise grow it by a frame a second.
const trailMemoCap = 64

type trailMemoDoc struct {
	doc []string
	sel []int
}

// retire forgets every document: the model is about to change.
func (m *Model) retireTrailMemo() {
	m.trailMemo.gen++
	m.trailMemo.docs = nil
	m.trailMemo.rows = nil
}

// selRows is TrailRows — the selectable rows — over the Model's trail at its level, off the memo:
// sixteen callers ask for it on a keypress, each walking every leg.
func (m *Model) selRows() []TrailRow {
	key := fmt.Sprintf("%d|", m.level) + trailProbe(m.trail)
	rows, ok := m.trailMemo.rows[key]
	if !ok {
		rows = TrailRows(m.trail, m.level)
		if m.trailMemo.rows == nil || len(m.trailMemo.rows) >= trailMemoCap {
			m.trailMemo.rows = map[string][]TrailRow{}
		}
		m.trailMemo.rows[key] = rows
	}
	return rows
}

// trailDocOf is trailDoc over any trail — the board's columns draw many —
// memoized for the message. The key carries the trail's identity, so two
// columns never read each other's rows.
func (m *Model) trailDocOf(tr journey.Trail, o TrailOpts) ([]string, []int) {
	key := trailMemoKey(tr, o)
	d, ok := m.trailMemo.docs[key]
	if !ok {
		d.doc, d.sel = trailDocBare(tr, o)
		if m.trailMemo.docs == nil || len(m.trailMemo.docs) >= trailMemoCap {
			m.trailMemo.docs = map[string]trailMemoDoc{}
		}
		m.trailMemo.docs[key] = d
	}
	return withTrailCursor(d.doc, d.sel, trailCursor(o), o.Width), d.sel
}

// trailDoc is trailDoc over the Model's own trail, off the memo.
func (m *Model) trailDoc(o TrailOpts) ([]string, []int) { return m.trailDocOf(m.trail, o) }

// trailLines is TrailLines over the Model's trail, off the memo.
func (m *Model) trailLines(o TrailOpts) []string { return m.trailLinesOf(m.trail, o) }

// trailLinesOf is TrailLines over any trail, off the memo.
func (m *Model) trailLinesOf(tr journey.Trail, o TrailOpts) []string {
	doc, _ := m.trailDocOf(tr, o)
	return doc
}

// trailCursorRow is TrailCursorRow over the Model's trail, off the memo.
func (m *Model) trailCursorRow(o TrailOpts) int {
	_, sel := m.trailDoc(o)
	return cursorRowIn(sel, trailCursor(o))
}

// trailRowsAt is trailRows over the Model's trail, off the memo.
func (m *Model) trailRowsAt(o TrailOpts) []string {
	doc, _ := m.trailDoc(o)
	return trailRowsOf(doc, o)
}

// renderTrailOf is RenderTrail over any trail, off the memo.
func (m *Model) renderTrailOf(tr journey.Trail, o TrailOpts) string {
	doc, _ := m.trailDocOf(tr, o)
	return strings.Join(fit(trailRowsOf(doc, o), o.Height), "\n")
}

// hiddenAboveOf is hiddenAbove over any trail, off the memo.
func (m *Model) hiddenAboveOf(tr journey.Trail, o TrailOpts) int {
	doc, sel := m.trailDocOf(tr, o)
	return hiddenAboveIn(tr, o, doc, sel)
}

// trailMemoKey is everything the bare document is a function of, as one
// string. Reflection walks TrailOpts so a field added later is in the key
// without anyone remembering to put it there; only the three the document
// does not depend on are skipped by name.
func trailMemoKey(tr journey.Trail, o TrailOpts) string {
	var b strings.Builder
	v := reflect.ValueOf(o)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		switch f.Name {
		case "Cursor", "Scroll", "Pinned":
			continue
		}
		fv := v.Field(i)
		switch f.Name {
		case "Labels", "Agents":
			// Large, and replaced — never grown in place — by the message
			// that brings a new label or a fresh read of a lane's file,
			// which retires the memo: size and identity are enough.
			fmt.Fprintf(&b, "%s=%d@%x;", f.Name, fv.Len(), fv.Pointer())
		default:
			// Scalars, the small maps (a handful of links, printed sorted
			// by Go) and the todo list, whole.
			fmt.Fprintf(&b, "%s=%v;", f.Name, fv)
		}
	}
	b.WriteByte('|')
	b.WriteString(trailProbe(tr))
	return b.String()
}

// trailProbe is a trail's part of a memo key: sizes and the backing
// arrays' identity, plus the rows a live session changes in place — the
// open leg and the last prompt — so a test that swaps m.trail by hand, or
// a poll that stretched HEAD without adding a leg, misses too.
func trailProbe(tr journey.Trail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "legs=%d@%x prompts=%d@%x branches=%d@%x tasks=%v compactions=%v",
		len(tr.Legs), slicePtr(tr.Legs), len(tr.Prompts), slicePtr(tr.Prompts), len(tr.Branches), slicePtr(tr.Branches), tr.Tasks, tr.Compactions)
	if n := len(tr.Legs); n > 0 {
		fmt.Fprintf(&b, " last=%+v", tr.Legs[n-1])
	}
	if n := len(tr.Prompts); n > 0 {
		fmt.Fprintf(&b, " lastprompt=%+v", tr.Prompts[n-1])
	}
	return b.String()
}

// slicePtr is the address of a slice's backing array: the identity of the
// data behind it, which a new poll's segmenter output never shares.
func slicePtr[T any](s []T) uintptr {
	if cap(s) == 0 {
		return 0
	}
	return uintptr(unsafe.Pointer(unsafe.SliceData(s)))
}
