package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

// The board is Lv0: every trail that fits, side by side, with the fleet at
// its floor on the left. It is the deck's default on any terminal wide enough
// for it, and Shift+Tab from a single trail returns to it (decision #16).
//
// It exists because a wide monitor showing one trail and a column of names
// left the person looking at it worried about everything it was not showing.
// The mirror was one answer to that worry and turned out to be the wrong one:
// a glimpse of one CLI's last screenful. The board is the other: the journeys
// themselves, all the ones that are moving, at a glance.
//
// Columns are in the fleet's own order — needs-you first, then stuck, then
// working, then idle by recency — so the sessions that can want you are on
// the left and the ones that have gone quiet fall off the right. The selected
// session always has a column. Idle sessions' trails are drawn dim: they are
// history until someone types into them, and the eye should land on the ones
// that are moving (SPEC §4).
const (
	boardColMin = 34 // narrower than this a trail row says nothing
	boardColMax = trailWidthMax

	// boardFresh is how long a finished session stays bright on the board
	// while nobody has opened it: a day, the tier the person watching asked
	// for. Past it a session is history whether or not it was read.
	boardFresh = 24 * time.Hour
)

// boardFits says whether the terminal is wide enough for the board: the
// level decision, made from the width alone, so a deck sized before its
// fleet arrives still opens on the board.
func (m *Model) boardFits() bool {
	n, _ := boardColumns(m.width-2*edgePad, 1)
	return m.width >= deckWideCols && n > 0
}

// boardShown says whether the board is drawn: it fits, and the view has a
// session to put in a column.
func (m *Model) boardShown() bool {
	return m.boardFits() && len(m.viewOrder()) > 0
}

// boardColumns says how many trail columns a board of the given inner width
// holds for count sessions, and how wide each is. Every column that fits at
// the minimum is opened, never more than there are sessions; the width left
// over goes to all of them evenly, up to the cap past which a row is padding
// rather than information.
func boardColumns(inner, count int) (n, w int) {
	if count <= 0 || inner < boardColMin {
		return 0, 0
	}
	n = (inner + gutterWidth) / (boardColMin + gutterWidth)
	if n > count {
		n = count
	}
	w = (inner - (n-1)*gutterWidth) / n
	if w > boardColMax {
		w = boardColMax
	}
	return n, w
}

// viewOrder is the current view's sessions in the fleet's own order —
// needs-you, stuck, working, idle by recency — as indices into m.sessions.
// It is the board's order, column by column and then on into the strip.
func (m *Model) viewOrder() []int {
	var out []int
	for i, s := range m.sessions {
		if s.Live != m.archiveView {
			out = append(out, i)
		}
	}
	return out
}

// boardKeys picks the sessions that get a column: the first n of the view in
// its own order, with the selected session always among them — it takes the
// last column when it would not otherwise have one, so `7` on a three-column
// board shows session 7's trail rather than nothing.
func (m *Model) boardKeys(n int) []string {
	if n <= 0 {
		return nil
	}
	keys := make([]string, 0, n)
	for _, i := range m.viewOrder() {
		keys = append(keys, m.sessions[i].Info.Key())
		if len(keys) == n {
			break
		}
	}
	if _, ok := m.selected(); ok && m.selectedKey != "" {
		present := false
		for _, k := range keys {
			if k == m.selectedKey {
				present = true
				break
			}
		}
		if !present {
			if len(keys) == n {
				keys[n-1] = m.selectedKey
			} else {
				keys = append(keys, m.selectedKey)
			}
		}
	}
	return keys
}

// boardMove is j/k on the board: one session along the board's own order,
// column by column and on into the strip, so the selection walks what is on
// screen left to right rather than the fleet list's grouping.
func (m *Model) boardMove(delta int) {
	order := m.viewOrder()
	if len(order) == 0 {
		return
	}
	at := -1
	for i, idx := range order {
		if m.sessions[idx].Info.Key() == m.selectedKey {
			at = i
			break
		}
	}
	at += delta
	if at < 0 {
		at = 0
	}
	if at >= len(order) {
		at = len(order) - 1
	}
	m.point(m.sessions[order[at]].Info.Key())
}

// boardTarget is one session the refresh polls for the board: its key and the
// transcript behind it.
type boardTarget struct{ key, path string }

// boardTargets is what refresh polls when the board can be drawn: every
// session with a column. The selected session is among them, so a single poll
// serves the board and the single trail alike.
func (m *Model) boardTargets() []boardTarget {
	if !m.boardShown() {
		return nil
	}
	n, _ := boardColumns(m.width-2*edgePad, len(m.viewOrder()))
	var out []boardTarget
	for _, key := range m.boardKeys(n) {
		if s, ok := m.session(key); ok && s.Info.TranscriptPath != "" {
			out = append(out, boardTarget{key: key, path: s.Info.TranscriptPath})
		}
	}
	return out
}

// session finds a session by key in the current fleet.
func (m *Model) session(key string) (fleet.Session, bool) {
	for _, s := range m.sessions {
		if s.Info.Key() == key {
			return s, true
		}
	}
	return fleet.Session{}, false
}

// boardLines lays the board: one trail column per session, each under the
// two rows the fleet gives that session — number, glyph, name, age, then its
// class and result — and a strip along the bottom naming the sessions that
// did not get a column, so nothing on the fleet is out of reach. The fleet
// list itself is not drawn: its rows are the column headers, and what it did
// beyond selecting a session the columns do better.
func (m *Model) boardLines(w, h int) []string {
	order := m.viewOrder()
	n, cw := boardColumns(w, len(order))
	rowOf := m.boardRows()
	keys := m.boardKeys(n)
	body := h - 2 // the strip and its line of air
	if body < 1 {
		body = 1
	}
	var cols []column
	for _, key := range keys {
		if r, ok := rowOf[key]; ok {
			cols = append(cols, column{cw, m.boardColumn(key, r, cw, body)})
		}
	}
	lines := joinColumns(body, cols)
	lines = append(lines, "", m.boardStrip(keys, rowOf, w))
	return fit(lines, h)
}

// boardRows numbers the board's sessions in the board's own order — the
// number beside a column is the key that selects it, `1` being the leftmost
// column and the strip carrying on from the last. The fleet list numbers its
// rows in its grouped order; a person on the board sees only these.
func (m *Model) boardRows() map[string]fleetRow {
	rows := map[string]fleetRow{}
	for pos, i := range m.viewOrder() {
		num := 0
		if pos < 9 {
			num = pos + 1
		}
		rows[m.sessions[i].Info.Key()] = fleetRow{sess: i, num: num}
	}
	return rows
}

// boardSelect is `1`–`9` on the board: the column (or strip entry) wearing
// that number.
func (m *Model) boardSelect(i int) bool {
	order := m.viewOrder()
	if i < 0 || i >= len(order) {
		return false
	}
	m.point(m.sessions[order[i]].Info.Key())
	return true
}

// boardStrip is the board's last row: the sessions without a column, in the
// board's order with their fleet numbers, then the way to the other view.
func (m *Model) boardStrip(keys []string, rowOf map[string]fleetRow, w int) string {
	shown := map[string]bool{}
	for _, k := range keys {
		shown[k] = true
	}
	var rest []string
	for _, i := range m.viewOrder() {
		s := m.sessions[i]
		key := s.Info.Key()
		if shown[key] {
			continue
		}
		st := s.Snap.State
		if m.archiveView {
			st = state.Idle
		}
		name := fleet.Glyph(st) + " " + sessionName(s.Info) + " " + m.age(s.Info.LastEventAt)
		if r, ok := rowOf[key]; ok && r.num > 0 {
			name = fmt.Sprintf("%d %s", r.num, name)
		}
		rest = append(rest, name)
	}
	var parts []string
	if len(rest) > 0 {
		parts = append(parts, fmt.Sprintf("+%d more · %s", len(rest), strings.Join(rest, " · ")))
	}
	if m.archiveView {
		parts = append(parts, "A live fleet")
	} else if n := m.archivedCount(); n > 0 {
		parts = append(parts, fmt.Sprintf("%d archived · A browses", n))
	}
	return dimStyle.Render(clip(strings.Join(parts, "   "), w))
}

// boardColumn is one session's column: its two fleet rows as the header, a
// line of air, and its trail pinned to the present. A muted session's trail
// is drawn dim, glyphs and all: the shapes still carry the classes (SPEC §4),
// and the eye goes to the columns with something in them to read.
func (m *Model) boardColumn(key string, r fleetRow, w, h int) []string {
	s := m.sessions[r.sess]
	entry := m.entryLines(r, w)
	rows := []string{entry[0], m.boardSecondLine(s, m.verdictLine(key, s, entry[1], w), w), m.boardDelta(key, s, w)}
	if h <= 3 {
		return fit(rows, h)
	}
	tr, loaded := m.trails[key]
	if !loaded {
		// The feed has not been polled yet. This is not the designed empty
		// state — that one says a session has never done anything, and a
		// column with four hundred legs behind it must not say so for the
		// second before its first poll lands.
		return append(rows, dimStyle.Render(clip(glyphGhost+" reading its transcript…", w)))
	}
	working := s.Snap.State == state.Working && !m.archiveView
	opts := TrailOpts{
		Todos:      planItems(tr.Tasks),
		Labels:     m.boardLabels[key],
		SessionKey: key,
		Now:        m.now,
		Width:      w,
		Height:     h - 3,
		Level:      levelTrail,
		Cursor:     -1,
		Pulse:      m.pulse && working,
		Pinned:     true,
	}
	frame := RenderTrail(tr, opts)
	lines := strings.Split(frame, "\n")
	// A column pinned to the present with a day above it says so on its
	// first row — the rail row that would otherwise be a bare stroke — so a
	// column that begins with ◉ and one that begins ten hours in are not
	// one character apart.
	if hidden := hiddenAbove(tr, opts); hidden > 0 && len(lines) > 0 {
		began := ""
		if len(tr.Prompts) > 0 {
			began = " · began " + relAge(m.now, tr.Prompts[0].At) + " ago"
		}
		lines[0] = dimStyle.Render(clip(fmt.Sprintf("↑ %d legs above%s", hidden, began), w))
	}
	if m.boardMuted(s) {
		for i, line := range lines {
			lines[i] = dimStyle.Render(ansi.Strip(line))
		}
	}
	return append(rows, lines...)
}

// verdictLine is the column header's second row: for a session you must not
// miss, the sentence that explains it (the fleet row already says that); for
// the rest, how it came out, read off the trail itself rather than the fleet
// feed — which on a day-long session had the header saying "ship" over a
// column that ended on a design leg.
func (m *Model) verdictLine(key string, s fleet.Session, fleetLine string, w int) string {
	if wantsAttention(s.Snap.State) || s.Snap.APIError || m.archiveView {
		return fleetLine
	}
	tr, ok := m.trails[key]
	if !ok {
		return fleetLine
	}
	v := boardVerdict(s, tr, m.now)
	if v == "" {
		return fleetLine
	}
	return dimStyle.Render(clip("    "+v, w))
}

// boardVerdict is how a journey came out, in words, from its own tail: what
// shipped, whether the suite is green, what is still out. It is the answer to
// "did it work" with zero keys and zero inference, which for eleven of twelve
// sessions on an afternoon board is the only thing anyone needed.
func boardVerdict(s fleet.Session, tr journey.Trail, now time.Time) string {
	var parts []string

	// Agents still out, oldest first: the number that changes what you do.
	out, oldest := 0, time.Time{}
	for _, b := range tr.Branches {
		if !b.Done {
			out++
			if oldest.IsZero() || b.Start.Before(oldest) {
				oldest = b.Start
			}
		}
	}
	if out > 0 {
		parts = append(parts, fmt.Sprintf("◈%d out · oldest %s", out, relAge(now, oldest)))
	}

	// The newest completed leg: shipped, or what it was.
	var last *journey.Leg
	for i := len(tr.Legs) - 1; i >= 0; i-- {
		if !tr.Legs[i].Current {
			last = &tr.Legs[i]
			break
		}
	}
	shipped := ""
	if last != nil && last.Class == journey.Ship {
		shipped = "✓ shipped " + relAge(now, last.End) + " ago"
	}

	// The newest test verdict, and what has happened to it since: edits it
	// has not been rerun over, a rerun in progress, or — the one to catch —
	// a commit made on top of a red run.
	verdict := ""
	for i := len(tr.Legs) - 1; i >= 0; i-- {
		l := tr.Legs[i]
		badge := legBadge(l)
		if badge == "" {
			continue
		}
		red := strings.Contains(badge, "✗")
		word := "✓ green"
		if red {
			word = "✗ red"
		}
		verdict = word + " " + badge
		edited, shippedSince, rerunning := false, false, time.Time{}
		for j := i + 1; j < len(tr.Legs); j++ {
			switch c := tr.Legs[j]; {
			case c.Class == journey.Fix || c.Class == journey.Build:
				edited = true
			case c.Class == journey.Ship:
				shippedSince = true
			case c.Class == journey.Test && c.Current:
				rerunning = c.Start
			}
		}
		switch {
		case red && shippedSince:
			verdict += " · shipped on red"
			shipped = "" // the same fact, and this is the way to say it
		case !rerunning.IsZero():
			verdict += " · rerunning for " + relAge(now, rerunning)
		case edited:
			verdict += " · edited since"
		}
		break
	}
	if shipped != "" {
		parts = append(parts, shipped)
	}
	if verdict != "" {
		parts = append(parts, verdict)
	}

	if len(parts) == 0 && last != nil {
		label := last.Label
		if label == "" {
			label = last.Class.String()
		}
		parts = append(parts, last.Class.String()+" "+label)
	}
	return strings.Join(parts, " · ")
}

// boardSecondLine is the fleet's second row for the session with, where the
// fleet's grouping would have said it, the tmux session it lives in on the
// right: the one fact the list carried that the column otherwise loses.
func (m *Model) boardSecondLine(s fleet.Session, line string, w int) string {
	group := ""
	if pane, ok := m.panes[s.Info.Key()]; ok && pane.Target != "" {
		group = tmuxSessionName(pane.Target)
	}
	if group == "" {
		return line
	}
	group = "⌁ " + group // the pane mark, so "work" does not read as a state word
	if lipgloss.Width(line)+2+lipgloss.Width(group) > w {
		return line
	}
	return pad(line, w-lipgloss.Width(group)) + dimStyle.Render(group)
}

// boardDelta is the header's third row: how much a column has grown since the
// person last opened it, when both halves of that are known. It is what makes
// the brightness a quantity — "bright" says there is something unread, this
// says how much and how far back. A column never opened has no baseline and
// says nothing (the whole trail is new, and the brightness already says so);
// a column with nothing new keeps the row as air.
func (m *Model) boardDelta(key string, s fleet.Session, w int) string {
	if m.archiveView {
		return ""
	}
	seen, ok := m.seen[key]
	if !ok {
		// No baseline, but a bright column that was never opened should
		// say so rather than keep the row as air: "never opened" is the
		// value the brightness stands for, and on a fresh launch it is the
		// only one that prints.
		if m.boardMuted(s) {
			return ""
		}
		if n := len(m.trails[key].Legs); n > 0 {
			return dimStyle.Render(clip(fmt.Sprintf("↳ %s · never opened", plural(n, "leg")), w))
		}
		return ""
	}
	if !seen.Before(s.Info.LastEventAt) {
		return ""
	}
	n := 0
	for _, l := range m.trails[key].Legs {
		if l.Start.After(seen) {
			n++
		}
	}
	if n == 0 {
		return ""
	}
	word := "legs"
	if n == 1 {
		word = "leg"
	}
	line := fmt.Sprintf("↳ %d new %s · looked %s ago", n, word, state.ShortDuration(m.now.Sub(seen)))
	return dimStyle.Render(clip(line, w))
}

// hiddenAbove counts the legs a pinned column keeps above its first row.
func hiddenAbove(tr journey.Trail, o TrailOpts) int {
	doc, sel := trailDoc(tr, o)
	return legsHiddenAbove(tr, o.Level, sel, trailTop(len(doc), o))
}

// boardMuted says whether a column is drawn dim. Bright means there is
// something in it to read: a session that is working, or waiting on you, or
// one that finished within the day and has not been opened since — "done
// two minutes ago and already dim" was the first thing the board got wrong.
// Opening the trail (Tab) or the pane (Enter) marks it read. Every archived
// column is dim: the archive is history by definition.
func (m *Model) boardMuted(s fleet.Session) bool {
	if m.archiveView {
		return true
	}
	switch s.Snap.State {
	case state.Working, state.NeedsYou, state.Stuck:
		return false
	}
	last := s.Info.LastEventAt
	if last.IsZero() || m.now.Sub(last) > boardFresh {
		return true
	}
	seen, ok := m.seen[s.Info.Key()]
	return ok && !seen.Before(last)
}

// markSeen records that the person opened this session — its trail or its
// pane — as of the deck's clock, so a column it has read can go quiet.
func (m *Model) markSeen(key string) {
	if key == "" {
		return
	}
	if m.seen == nil {
		m.seen = make(map[string]time.Time)
	}
	m.seen[key] = m.now
	m.saveSeen()
}

// refreshBoard folds a refresh's trails into the board: the trails themselves,
// and the narrated labels that have landed for each. Narration is requested
// for every column — one batch at a time, the narrator's own rule — so a
// column reads in prose rather than file names once it has been on screen a
// while, the same as the single trail does.
func (m *Model) refreshBoard(trails map[string]journey.Trail) {
	if trails == nil {
		return
	}
	// Merge, never replace: a column that leaves the poll for a tick — the
	// selection moved, the terminal narrowed — keeps the journey it had
	// rather than reverting to "reading its transcript…".
	if m.trails == nil {
		m.trails = make(map[string]journey.Trail, len(trails))
	}
	if m.boardLabels == nil {
		m.boardLabels = make(map[string]map[string]string, len(trails))
	}
	for key, tr := range trails {
		m.trails[key] = tr
	}
	if m.narrator == nil {
		return
	}
	// In the board's order, not the map's: the selected session was asked
	// first by the caller, and the columns nearest the left are the ones a
	// person is reading, so they are asked next.
	if m.boardShapes == nil {
		m.boardShapes = make(map[string]string, len(trails))
	}
	for _, i := range m.viewOrder() {
		key := m.sessions[i].Info.Key()
		tr, ok := trails[key]
		if !ok {
			continue
		}
		// Reading labels walks every leg of the trail. Once per shape, not
		// once per tick: a column with thousands of legs was costing
		// milliseconds and megabytes a second for the twenty rows it draws.
		if shape := trailShape(key, tr); shape != m.boardShapes[key] || m.boardLabels[key] == nil {
			m.boardLabels[key] = m.narrator.Labels(key, tr)
			m.boardShapes[key] = shape
		}
		if key != m.selectedKey {
			m.narrator.Request(key, tr, "")
		}
	}
}
