package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/transcript"
)

// The pair (#397): a lane's reader beside the linked session's own. A lane's
// `→N` says a live session looks like this agent — the same agent, seen
// through its own transcript, which is fresher than the lane's file when
// the file has gone quiet. The subagents operator's question of that frame
// was "how were things going between them", and one key could only flip
// between the two conversations. Where the width has the room, `tab` on a
// linked lane opens both: the lane's reader keeps the keys, and the linked
// session's reader follows its mark by time, so `j`/`k` through one answers
// "what was the other doing then". No key is added; the pair is a shape of
// Lv3, not a level.

// pairReaderMin is the least width each half of the pair may have: under it
// two conversations wrap into noise, and the reader keeps the frame alone.
const pairReaderMin = 60

// pairFits says whether the frame has room for two readers side by side:
// the session view at Lv3, each half at pairReaderMin or wider.
func (m *Model) pairFits() bool {
	inner := m.width - 2*edgePad
	return m.sessionView() && m.level >= levelReader && (inner-gutterWidth)/2 >= pairReaderMin
}

// pairShown says whether the pair is drawn: a linked lane is open in the
// reader, the link's session is known, and the width fits.
func (m *Model) pairShown() bool {
	return m.pairKey != "" && m.readerLane != "" && m.pairFits()
}

// pairWidths splits the inner width between the two readers, the gutter
// between them.
func (m *Model) pairWidths() (left, right int) {
	inner := m.width - 2*edgePad
	left = (inner - gutterWidth) / 2
	return left, inner - gutterWidth - left
}

// openPair sets the pair from the reader's lane: the session its `→N` names,
// or none. It runs where the lane reader opens or comes back.
func (m *Model) openPair() {
	was := m.pairKey
	m.pairKey = ""
	if m.readerLane == "" || m.level < levelReader {
		m.closePair()
		return
	}
	br, ok := m.laneOpen()
	if !ok {
		m.closePair()
		return
	}
	l, ok := m.laneMatches(m.trail, m.agentsFor(m.selectedKey))[br.Label]
	if !ok || l.key == "" || l.key == m.selectedKey {
		m.closePair()
		return
	}
	m.pairKey = l.key
	if was != l.key {
		m.pairEvents = nil
		m.pairCache.valid = false
	}
}

// closePair drops the pair and what it held.
func (m *Model) closePair() {
	m.pairKey, m.pairEvents = "", nil
	m.pairCache.valid = false
}

// pairCWD is the directory the follower shortens its paths against.
func (m *Model) pairCWD() string {
	s, ok := m.session(m.pairKey)
	if !ok {
		return ""
	}
	if s.Info.OriginCWD != "" {
		return s.Info.OriginCWD
	}
	return s.Info.CWD
}

// pairDoc is the follower's document, flattened once per change of events
// or width.
func (m *Model) pairDoc(w int) []readerLine {
	c := &m.pairCache
	cwd := m.pairCWD()
	if c.valid && c.n == len(m.pairEvents) && c.w == w && c.cwd == cwd {
		return c.lines
	}
	lines := readerDoc(m.pairEvents, ReaderOpts{Width: w, CWD: cwd, Now: m.now})
	m.pairCache = readerCache{lines: lines, valid: true, n: len(m.pairEvents), w: w, cwd: cwd}
	return lines
}

// pairRow is the follower's row: the one nearest the mark's moment, or the
// newest line where the reader has no mark.
func (m *Model) pairRow(w int) int {
	doc := m.pairDoc(w)
	if len(doc) == 0 {
		return -1
	}
	if !m.anchorAt.IsZero() {
		if row := ReaderAnchor(m.pairEvents, ReaderOpts{Width: w, CWD: m.pairCWD(), Now: m.now}, m.anchorAt); row >= 0 {
			return row
		}
	}
	return nonBlankRow(doc, len(doc)-1, -1)
}

// pairColumn draws the follower: whose conversation, that it follows, and
// the page around the row nearest the mark.
func (m *Model) pairColumn(w, h int) []string {
	s, ok := m.session(m.pairKey)
	if !ok {
		return fit(nil, h)
	}
	name := sessionName(s.Info)
	if d := m.digits[s.Info.Key()]; d > 0 {
		name = strconv.Itoa(d) + " " + name
	}
	// The clock is the session's own: what it wrote last and when, or its
	// silence — the fact the lane's file cannot carry.
	right := "wrote " + relAge(m.now, s.Info.LastEventAt) + " ago"
	if s.Snap.State == state.Stuck {
		right = "silent " + relAge(m.now, s.Info.LastEventAt)
	}
	body := w - 1
	title := "READER · " + name
	follows := " · follows"
	left := m.titleStyleFor(panelTrail).Render(clip(title, body-lipgloss.Width(right)-lipgloss.Width(follows)-1)) + dimStyle.Render(follows)
	gap := body - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
		right = clip(right, max(body-lipgloss.Width(left)-1, 0))
	}
	rows := []string{" " + left + strings.Repeat(" ", gap) + dimStyle.Render(right)}
	if len(m.pairEvents) == 0 {
		rows = append(rows, dimStyle.Render(clip(" "+glyphSaid+" reading the transcript…", w)))
		return fit(rows, h)
	}
	doc := m.pairDoc(w)
	row := m.pairRow(w)
	// The above row says how the follower's row stands to the mark.
	above := " the newest line"
	if row >= 0 && !m.anchorAt.IsZero() && !doc[row].at.IsZero() {
		above = " " + doc[row].at.Local().Format("15:04") + " · " + pairOffset(doc[row].at, m.anchorAt)
	}
	rows = append(rows, dimStyle.Render(clip(above, w)))
	if h <= 2 {
		return fit(rows, h)
	}
	page := h - 2
	top := 0
	if row >= 0 {
		top = readerTopIn(doc, clampScroll(row-page/2, len(doc), page), page)
	}
	frame := RenderReader(m.pairEvents, ReaderOpts{
		Width:  w,
		Height: page,
		Scroll: top,
		Anchor: row,
		CWD:    m.pairCWD(),
		Now:    m.now,
	})
	rows = append(rows, strings.Split(frame, "\n")...)
	return fit(rows, h)
}

// pairOffset says where the follower's row stands against the mark: the
// same minute, or how far before or after it.
func pairOffset(at, mark time.Time) string {
	d := at.Sub(mark)
	if d < 0 {
		d = -d
	}
	if d < time.Minute {
		return "the same minute as the mark"
	}
	if at.Before(mark) {
		return relDur(d) + " before the mark"
	}
	return relDur(d) + " after the mark"
}

// relDur is a duration in the deck's own short form: 40s, 3m, 2h.
func relDur(d time.Duration) string {
	return state.ShortDuration(d)
}

// pairPoll is the follower's events, landed by the poll.
func (m *Model) pairPoll(key string, events []transcript.Event) {
	if key == "" || key != m.pairKey {
		return
	}
	m.pairEvents = events
	m.pairCache.valid = false
}

// pairCheck closes the pair whose session is gone: half the frame as a
// dead transcript under a live clock says the wrong thing (the panel's
// stale case), so the reader reads alone and the note says who ended.
func (m *Model) pairCheck() {
	if m.pairKey == "" {
		return
	}
	s, ok := m.session(m.pairKey)
	if ok && s.Live {
		return
	}
	name := m.pairKey
	if ok {
		name = sessionName(s.Info)
	}
	m.note = name + " ended · reading alone"
	m.closePair()
}
