package ui

import (
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/transcript"
)

// The pair (#398): a lane's reader beside the linked session's own. A lane's
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

// pairShown says whether the pair is drawn: the reader has a pair — a
// linked lane's session, or the peer this session is talking to — and the
// width fits.
func (m *Model) pairShown() bool {
	return m.pairKey != "" && m.pairFits()
}

// namesSession says whether an envelope's `from` names a session: its id
// (whole, or the first eight or more characters), the name the person
// gave it, or the name the deck draws it under (#399).
func namesSession(from string, s fleet.Session) bool {
	from = strings.TrimSpace(from)
	if from == "" {
		return false
	}
	if from == s.Info.ID || (len(from) >= 8 && strings.HasPrefix(s.Info.ID, from)) {
		return true
	}
	if s.Info.Name != "" && strings.EqualFold(from, s.Info.Name) {
		return true
	}
	return strings.EqualFold(from, sessionName(s.Info))
}

// peerLinks is each relayed prompt's sender, where the sender is a live
// session on the board: the prompt's index to the session's digit and key.
// The join is the envelope's own name (#399), never a guess from words.
func (m *Model) peerLinks(key string, tr journey.Trail) map[int]int {
	links := map[int]int{}
	for i, p := range tr.Prompts {
		if !p.Relayed || p.From == "" {
			continue
		}
		for _, s := range m.sessions {
			if s.Live && m.onBoard(s) && s.Info.Key() != key && namesSession(p.From, s) {
				if d := m.digits[s.Info.Key()]; d > 0 {
					links[i] = d
				}
				break
			}
		}
	}
	return links
}

// peerKey is the session this one is talking to: the sender of its newest
// relayed message that is a live session on the board, or — where it has
// sent and not yet heard back — the live session whose trail carries a
// message from this one. "" when it talks to nobody the board can see.
func (m *Model) peerKey() string {
	tr := m.trails[m.selectedKey]
	for i := len(tr.Prompts) - 1; i >= 0; i-- {
		p := tr.Prompts[i]
		if !p.Relayed || p.From == "" {
			continue
		}
		for _, s := range m.sessions {
			if s.Live && m.onBoard(s) && s.Info.Key() != m.selectedKey && namesSession(p.From, s) {
				return s.Info.Key()
			}
		}
	}
	me, ok := m.session(m.selectedKey)
	if !ok {
		return ""
	}
	var best string
	var at time.Time
	for _, s := range m.sessions {
		if !s.Live || !m.onBoard(s) || s.Info.Key() == m.selectedKey {
			continue
		}
		for _, p := range m.trails[s.Info.Key()].Prompts {
			if p.Relayed && p.From != "" && namesSession(p.From, me) && p.At.After(at) {
				best, at = s.Info.Key(), p.At
			}
		}
	}
	return best
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
	if m.level < levelReader {
		m.closePair()
		return
	}
	key := ""
	if m.readerLane != "" {
		// A lane's reader pairs with the session its `→N` names.
		if br, ok := m.laneOpen(); ok {
			if l, ok := m.laneMatches(m.trail, m.agentsFor(m.selectedKey))[br.Label]; ok && l.key != m.selectedKey {
				key = l.key
			}
		}
	} else {
		// The session's own reader pairs with the peer it is talking to
		// (#399): the other side of the conversation, following by time.
		key = m.peerKey()
	}
	if key == "" {
		m.closePair()
		return
	}
	m.pairKey = key
	if s, ok := m.session(key); ok {
		m.pairID, m.pairName = s.Info.ID, sessionName(s.Info)
	}
	if was != key {
		m.pairEvents = nil
		m.pairPolled = false
		m.pairCache.valid = false
	}
}

// closePair drops the pair and what it held.
func (m *Model) closePair() {
	m.pairKey, m.pairEvents = "", nil
	m.pairID, m.pairName = "", ""
	m.pairPolled = false
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
		if row := anchorRow(doc, m.anchorAt); row >= 0 {
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
	// The name wears the link the trail drew — `→1 harness` — so the
	// frame says the two halves are one agent where the trail's row is
	// off it (round 68); the digit is the way there.
	name := sessionName(s.Info)
	if d := m.digits[s.Info.Key()]; d > 0 {
		name = "→" + strconv.Itoa(d) + " " + name
	}
	// The clock is the session's own, in the fleet's words and glyph: what
	// it wrote last and when, its silence, or the alarm it owes — a
	// follower stopped on a permission read as progress under `wrote 20s
	// ago` (round 68). §2.4 puts the state on the row.
	age := relAge(m.now, s.Info.LastEventAt)
	right := m.rowGlyph(s) + " wrote " + age + " ago"
	switch {
	case s.Snap.APIError, s.Waiting, s.Snap.State == state.NeedsYou:
		right = m.rowGlyph(s) + " " + headline(s) + " " + age
	case s.Snap.State == state.Stuck:
		right = m.rowGlyph(s) + " silent " + age
	}
	body := w - 1
	title := "READER · " + name
	follows := " · follows"
	if m.fleetQuery != "" && !m.matchesQuery(s) {
		// The board hides this session under the search; the pair draws
		// it anyway, and says so as the digit's landing does (#397).
		follows += " · outside /" + m.fleetQuery
	}
	left := m.titleStyleFor(panelTrail).Render(clip(title, body-lipgloss.Width(right)-lipgloss.Width(follows)-1)) + dimStyle.Render(follows)
	gap := body - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
		right = clip(right, max(body-lipgloss.Width(left)-1, 0))
	}
	rows := []string{" " + left + strings.Repeat(" ", gap) + dimStyle.Render(right)}
	if len(m.pairEvents) == 0 {
		// Late, then empty: "reading" until the first poll lands, and
		// after it the page says what it holds — nothing — under the
		// title's clock, said once (#53, #69; round 68).
		empty := " " + glyphSaid + " reading the transcript…"
		if m.pairPolled {
			empty = " ⋯ nothing to read yet"
		}
		rows = append(rows, dimStyle.Render(clip(empty, w)))
		return fit(rows, h)
	}
	doc := m.pairDoc(w)
	row := m.pairRow(w)
	// The above row says how the follower's row stands to the mark.
	above := " the newest line"
	if row >= 0 && !m.anchorAt.IsZero() && !doc[row].at.IsZero() {
		above = " " + doc[row].at.Local().Format("15:04") + " · " + pairOffset(doc[row].at, m.anchorAt)
		clock := doc[row].at.Local().Format("15:04")
		if first := nonBlankRow(doc, 0, 1); first >= 0 && row <= first && doc[row].at.After(m.anchorAt) {
			// The mark stands before anything the follower has: its
			// first line is what is drawn, and the row says so before
			// the offset, or an offset alone read as a line off-screen
			// (round 68).
			above = " " + clock + " · its first line · " + pairOffset(doc[row].at, m.anchorAt)
		} else if last := nonBlankRow(doc, len(doc)-1, -1); last >= 0 && row >= last && doc[row].at.Before(m.anchorAt) {
			// And past everything it has: a follower four hours quiet
			// read `3h before the mark` as if later lines existed (the
			// owner's screenshot).
			above = " " + clock + " · its newest line · " + pairOffset(doc[row].at, m.anchorAt)
		}
	}
	if h <= 2 {
		rows = append(rows, dimStyle.Render(clip(above, w)))
		return fit(rows, h)
	}
	page := h - 2
	top := 0
	if row >= 0 {
		top = readerTopIn(doc, clampScroll(row-page/2, len(doc), page), page)
	}
	// The page is a window on a conversation the mark cannot reach the
	// end of: a lane's file stops at its hung call, so the follower stands
	// where the mark stands and the newest line its own clock promises can
	// be below the fold. The row says so, as every other page does
	// (round 68, the subagents operator's one thing).
	if top > 0 {
		above = " ↑ " + plural(top, "line") + " above ·" + above
	}
	if below := len(doc) - top - page; below > 0 {
		above += " · ↓ " + plural(below, "line") + " below"
	}
	rows = append(rows, dimStyle.Render(clip(above, w)))
	frame := renderReaderDoc(doc, ReaderOpts{
		Width:  w,
		Height: page,
		Scroll: top,
		Anchor: row,
		Echo:   true, // the keys' cursor is the other half's (round 68)
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
	m.pairPolled = true
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
	// A session that changed directory writes under a new key (the M6
	// twin): the same id, live, is the same follower, and the pair
	// follows it rather than closing on a session that did not end
	// (round 68, the reader's measure).
	if m.pairID != "" {
		for _, t := range m.sessions {
			if t.Live && t.Info.ID == m.pairID && t.Info.Key() != m.pairKey {
				m.pairKey, m.pairName = t.Info.Key(), sessionName(t.Info)
				m.pairEvents, m.pairPolled, m.pairCache.valid = nil, false, false
				return
			}
		}
	}
	name := m.pairName
	if ok {
		name = sessionName(s.Info)
	}
	if name == "" {
		name = "the other side"
	}
	m.note = name + " ended · reading alone"
	m.closePair()
}
