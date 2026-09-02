package ui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/deephanson94/compass/internal/journey"
)

// doc is the flattened reader document, cached between keypresses — scrolling,
// folding and searching all walk it, and a long transcript should be flattened
// once per change, not once per key.
func (m *Model) doc(width int) []readerLine {
	c := &m.docCache
	cwd := m.readerCWD()
	if c.valid && c.n == len(m.events) && c.w == width && c.ver == m.docVer && c.cwd == cwd {
		return c.lines
	}
	lines := readerDoc(m.events, ReaderOpts{Width: width, Unfolded: m.unfolded, CWD: cwd})
	m.docCache = readerCache{lines: lines, valid: true, n: len(m.events), w: width, ver: m.docVer, cwd: cwd}
	return lines
}

// readerCWD is the directory the reader shortens paths against: where the
// selected session was opened, which is where its paths are rooted however
// far it has since wandered.
func (m *Model) readerCWD() string {
	s, ok := m.selected()
	if !ok {
		return ""
	}
	if s.Info.OriginCWD != "" {
		return s.Info.OriginCWD
	}
	return s.Info.CWD
}

// readerWidth is the column the reader currently owns — the same arithmetic
// deckLines does, so Space and the anchor act on the document being drawn.
func (m *Model) readerWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	inner := w - 2*edgePad
	if inner < 10 {
		inner = w
	}
	// The width the reader has when it is drawn — at Lv3 — whatever level is
	// asking. Lv2 computes the anchor for a panel it does not draw, and if it
	// used its own layout the anchor would be found at the trail's width and
	// then shown at the middle's, and the reader would jump as it took focus.
	switch {
	case inner < minDeckCols:
		return inner // one column, and at Lv3 it is the reader's
	case m.boardFits() && !m.archiveView:
		companion, _ := sessionSplit(inner) // the session view, at Lv2 or Lv3
		return companion
	case m.width < deckWideCols:
		return inner // the reader alone at Lv3 (layout): the deck is too narrow for a middle panel
	case m.width >= deckWideCols && m.width < readerRoomCols:
		return inner - trailWidth - gutterWidth // the fleet's width is the reader's (layout)
	case m.width >= deckWideCols:
		fleet, trail := sidePanelWidths(inner)
		return inner - fleet - trail - 2*gutterWidth
	default:
		return inner - twoColumnFleet(inner) - gutterWidth // it stands where the trail did
	}
}

// readerColumn is the deck's Lv3 middle panel: whose conversation this is, the
// search when one is live, and the document.
func (m *Model) readerColumn(w, h int) []string {
	rows := []string{m.readerTitle(w), ""}
	if h > 2 && len(m.events) == 0 && len(m.trail.Legs) > 0 {
		// The trail is in hand and the conversation is not yet: it is being
		// read, not absent. "nothing to read yet … as it happens" claimed a
		// session with a day of legs had not started.
		return append(rows, dimStyle.Render(clip(glyphSaid+" reading the transcript…", w)))
	}
	if h > 2 {
		frame := RenderReader(m.events, ReaderOpts{
			Width:    w,
			Height:   h - 2,
			Scroll:   m.scroll,
			Unfolded: m.unfolded,
			Query:    m.query,
			Anchor:   m.anchor,
			CWD:      m.readerCWD(),
		})
		rows = append(rows, strings.Split(frame, "\n")...)
	}
	return rows
}

// readerTitle mirrors the trail's: READER · <name>, with the search state —
// the query being typed, or the one in force — on the right.
func (m *Model) readerTitle(w int) string {
	name := "—"
	if s, ok := m.selected(); ok {
		name = sessionName(s.Info)
	}
	right := ""
	switch {
	case m.searching:
		right = "/" + m.draft + "▏"
	case m.query != "":
		right = "/" + m.query
	case m.anchor >= 0 && !m.anchorAt.IsZero():
		// Where the reader is: the row it was anchored to and its moment,
		// so a reader scrolled to an hour can tell it is the hour. The row
		// gets whatever the name leaves, not half the panel.
		right = m.anchorAt.Local().Format("15:04")
		if m.anchorText != "" {
			room := w - 1 - len([]rune("READER · "+name)) - 1 - len([]rune(right)) - 3
			if room >= 8 {
				right = clip(m.anchorText, room) + " · " + right // clip marks the cut with …
			}
		}
	}
	mark := m.titleMark(panelReader)
	body := w - 1
	left := m.titleStyleFor(panelReader).Render(clip("READER · "+name, body-lipgloss.Width(right)-1))
	gap := body - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return mark + left + strings.Repeat(" ", gap) + dimStyle.Render(right)
}

// enterReader is Tab on a Lv2 row: the reader is already open on that moment —
// the middle panel has been following the cursor since Lv2 — so all this does
// is hand it the keys.
func (m *Model) enterReader() {
	m.level = levelReader
	m.anchorReader()
}

// anchorReader points the reader at the row the Lv2 cursor stands on: the
// middle panel is that moment of the conversation, and it re-anchors on every
// cursor move (M7 contract). A row the document does not reach yet leaves the
// reader where it is rather than jumping it somewhere arbitrary.
func (m *Model) anchorReader() {
	m.anchor = -1
	if m.cursor < 0 {
		return
	}
	rows := TrailRows(m.trail, m.level)
	if m.cursor >= len(rows) {
		return
	}
	opts := ReaderOpts{Width: m.readerWidth(), Unfolded: m.unfolded, CWD: m.readerCWD()}
	if line := ReaderAnchor(m.events, opts, rows[m.cursor].Time); line >= 0 {
		// The scroll is clamped to the last screenful at once: an offset
		// past it drew the same frame, and the first j after it moved the
		// number and nothing else.
		doc := m.doc(opts.Width)
		m.scroll, m.anchor, m.anchorAt = clampScroll(line, len(doc), m.readerHeight()), line, rows[m.cursor].Time
		m.anchorText = rows[m.cursor].Text
		if row := rows[m.cursor]; row.Kind == "leg" && row.Leg >= 0 && row.Leg < len(m.trail.Legs) {
			// The row as drawn — the commit a ship leg is named by, the
			// plan's name for HEAD — not the heuristic label beneath it.
			w, h := m.trailBox()
			m.anchorText, _ = legLabel(m.trail.Legs[row.Leg], m.trailOpts(w, h))
		}
	}
}

// scrollBy moves the reader, clamped to the document. It reports whether
// the viewport moved at all, so a key at either end can say so.
func (m *Model) scrollBy(delta int) bool {
	doc := m.doc(m.readerWidth())
	if len(doc) <= m.readerHeight() {
		m.note = "the whole conversation is on screen"
		return true // said; nothing more to say
	}
	was := clampScroll(m.scroll, len(doc), m.readerHeight())
	m.scroll = clampScroll(was+delta, len(doc), m.readerHeight())
	// A single step never lands the top of the page on a line of air: the
	// press that only pushed a blank in and a line out revealed nothing.
	if (delta == 1 || delta == -1) && m.scroll < len(doc) && doc[m.scroll].kind == readerBlank {
		m.scroll = clampScroll(m.scroll+delta, len(doc), m.readerHeight())
	}
	return m.scroll != was
}

// readerChapter is `[` / `]` in the reader: the previous or next turn of
// yours — the ❯ rows — which are the conversation's chapters as the
// prompts are the trail's.
func (m *Model) readerChapter(key string) {
	doc := m.doc(m.readerWidth())
	var turns []int
	for i, l := range doc {
		if l.kind == readerSaid && (i == 0 || doc[i-1].kind != readerSaid) {
			turns = append(turns, i)
		}
	}
	if len(turns) == 0 {
		m.note = "no turns of yours in this conversation"
		return
	}
	top := clampScroll(m.scroll, len(doc), m.readerHeight())
	if key == "]" {
		for _, t := range turns {
			if t > top {
				m.scroll = clampScroll(t, len(doc), m.readerHeight())
				return
			}
		}
		m.note = "no later turn"
		return
	}
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i] < top {
			m.scroll = clampScroll(turns[i], len(doc), m.readerHeight())
			return
		}
	}
	m.note = "no earlier turn"
}

// readerHeight is the rows the document gets: the deck body minus the
// column's title and its line of air.
func (m *Model) readerHeight() int {
	h := m.height
	if h <= 0 {
		h = 24
	}
	body := h - 5 - 2
	if body < 1 {
		body = 1
	}
	return body
}

// toggleFold is Space: the first folded result on screen opens (or the first
// open one closes) — the document's own order decides, top of the screen down.
func (m *Model) toggleFold() {
	width := m.readerWidth()
	doc := m.doc(width)
	top := clampScroll(m.scroll, len(doc), m.readerHeight())
	for i := top; i < len(doc) && i < top+m.readerHeight(); i++ {
		if !doc[i].foldable() {
			continue
		}
		m.unfolded[doc[i].event] = !m.unfolded[doc[i].event]
		m.docVer++
		m.docCache.valid = false
		return
	}
	m.note = "nothing to unfold on screen"
}

// jumpMatch is n/N: the next (or previous) document row the query appears in.
func (m *Model) jumpMatch(dir int) {
	if m.query == "" {
		m.note = "no search — / starts one"
		return
	}
	doc := m.doc(m.readerWidth())
	matches := readerMatches(doc, m.query)
	if len(matches) == 0 {
		m.note = "no matches"
		return
	}
	if dir > 0 {
		for _, line := range matches {
			if line > m.scroll {
				m.scroll = clampScroll(line, len(doc), m.readerHeight())
				return
			}
		}
		m.scroll = clampScroll(matches[0], len(doc), m.readerHeight()) // wrap
		return
	}
	for i := len(matches) - 1; i >= 0; i-- {
		if matches[i] < m.scroll {
			m.scroll = clampScroll(matches[i], len(doc), m.readerHeight())
			return
		}
	}
	m.scroll = clampScroll(matches[len(matches)-1], len(doc), m.readerHeight())
}

// searchKey handles a keypress while the query is being typed.
func (m *Model) searchKey(msg tea.KeyMsg) {
	switch msg.String() {
	case "enter":
		m.query = strings.TrimSpace(m.draft)
		m.draft = ""
		m.searching = false
		if m.query != "" {
			m.scroll = 0
			m.jumpMatch(1)
		}
	case "esc":
		m.draft = ""
		m.searching = false
	case "backspace":
		if r := []rune(m.draft); len(r) > 0 {
			m.draft = string(r[:len(r)-1])
		}
	default:
		if msg.Type == tea.KeyRunes {
			m.draft += string(msg.Runes)
		}
	}
}

// requestNarration asks the narrator to name the trail's closed legs, at most
// once per shape of the trail — the narrator dedupes harder still. Everything
// it hands over is keyed by the session's Key(): two sessions sharing an id
// must never read each other's labels (M6 contract).
func (m *Model) requestNarration() {
	if m.narrator == nil || m.selectedKey == "" {
		return
	}
	shape := trailShape(m.selectedKey, m.trail)
	if shape == m.narrated {
		m.refreshLabels()
		return
	}
	prompt := ""
	if n := len(m.trail.Prompts); n > 0 {
		prompt = m.trail.Prompts[n-1].Text
	}
	// Remember the shape only once it is spoken for. A refusal — the one
	// batch in flight was a board column's — used to be remembered as if it
	// were an answer, and the trail being read was never asked for again.
	if m.narrator.Request(m.selectedKey, m.trail, prompt) {
		m.narrated = shape
	}
	m.refreshLabels()
}

// refreshLabels pulls whatever narration has landed for the selected trail.
func (m *Model) refreshLabels() {
	if m.narrator == nil || m.selectedKey == "" {
		return
	}
	m.labels = m.narrator.Labels(m.selectedKey, m.trail)
}

// trailShape fingerprints a trail cheaply: a new closed leg, or a session
// switch, is a new shape worth narrating.
func trailShape(key string, tr journey.Trail) string {
	closed := len(tr.Legs)
	if closed > 0 && tr.Legs[closed-1].Current {
		closed--
	}
	last := ""
	if closed > 0 {
		last = tr.Legs[closed-1].Start.String()
	}
	return key + "|" + strconv.Itoa(closed) + "|" + last
}
