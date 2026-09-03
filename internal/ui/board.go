package ui

import (
	"fmt"
	"sort"
	"strconv"
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
	boardColMax = 64 // wider than the single trail's cap: a column with the width carries its detail

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
	return m.boardFits() && (len(m.viewOrder()) > 0 || m.fleetQuery != "")
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
	if w > boardColMax && n >= 2 {
		// The cap is for a row of columns; a lone column takes the
		// width, since a filtered column at 65 of 220 was cutting its
		// prompt beside a field of nothing.
		w = boardColMax
	}
	return n, w
}

// viewOrder is the current view's sessions in the fleet's own order —
// needs-you, stuck, working, idle by recency — as indices into m.sessions.
// It is the board's order, column by column and then on into the strip.
func (m *Model) viewOrder() []int {
	if m.archiveView {
		// The archive is a list, never a board: its numbers run down the
		// list as drawn, not through an order nothing else shows.
		var out []int
		for _, g := range m.archiveGroups() {
			out = append(out, g.entries...)
		}
		return out
	}
	var out []int
	for i, s := range m.sessions {
		if m.onBoard(s) && m.matchesQuery(s) {
			out = append(out, i)
		}
	}
	// By obligation, then the fleet's own order within a rank (the longest
	// wait first among the alarms, the freshest first among the calm).
	sort.SliceStable(out, func(a, b int) bool {
		return m.obligation(m.sessions[out[a]]) < m.obligation(m.sessions[out[b]])
	})
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
	order := m.viewOrder()
	// Only what owes you gets a column; the shipped-and-read go to the
	// strip, however many columns are free. A board of nothing owed shows
	// everything, as before.
	owed := 0
	for _, i := range order {
		if m.archiveView || m.obligation(m.sessions[i]) <= owedRank {
			owed++
		}
	}
	for _, i := range order {
		if owed > 0 && !m.archiveView && m.obligation(m.sessions[i]) > owedRank {
			break
		}
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
	n, cw := boardColumns(w, m.drawnCount(order))
	rowOf := m.boardRows()
	body := h - 2 // the strip and its line of air
	if body < 1 {
		body = 1
	}
	// A tall board packs its columns into bands, each as tall as its
	// tallest trail: a day-long trail takes the height it needs, a band of
	// short ones takes what they need, and the strip names only what no
	// height was left for. Sized to the screen instead, a board of short
	// trails ended every column in eight blank rows while naming four
	// sessions in the strip.
	keys, heights := m.boardPack(n, cw, body)
	if len(keys) == 0 && m.fleetQuery != "" {
		// A search nothing answers keeps the board and says so, rather
		// than silently turning into the deck.
		return fit([]string{"", dimStyle.Render(clip("no session matches /"+m.fleetQuery+" · esc clears it", w))}, h)
	}
	var lines []string
	for b, bh := range heights {
		var cols []column
		bw := bandWidth(w, min(len(keys)-b*n, n), cw)
		for i := b * n; i < len(keys) && i < (b+1)*n; i++ {
			if r, ok := rowOf[keys[i]]; ok {
				cols = append(cols, column{bw, m.boardColumn(keys[i], r, bw, bh)})
			}
		}
		if len(cols) == 0 {
			break
		}
		if b > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, joinColumns(bh, cols)...)
	}
	// The strip sits under the last band, not on the screen's floor: a
	// calm board is a short board, and the strip is where the eye is.
	lines = append(lines, "", m.boardStrip(keys, rowOf, w))
	return fit(lines, h)
}

// boardPack packs the columns into bands for a body of the given height:
// the keys drawn, in order, n to a band, and each band's height.
func (m *Model) boardPack(n, cw, body int) (keys []string, heights []int) {
	order := m.viewOrder()
	var bands [][]string
	rem := body
	all := m.boardKeys(len(order))
	for pos := 0; pos < len(all) && rem >= boardBandMin; pos += n {
		band := all[pos:min(pos+n, len(all))]
		tallest := 0
		for _, key := range band {
			tallest = max(tallest, m.boardColumnRows(key, cw))
		}
		bh := min(tallest, rem) // as tall as its tallest trail, never padded to the minimum
		bands = append(bands, band)
		heights = append(heights, bh)
		rem -= bh + 1 // and the row of air under it
	}
	shown := 0
	for _, band := range bands {
		shown += len(band)
	}
	// The selected column is always drawn: boardKeys puts it among the
	// first shown, so pack once more over that order.
	keys = m.boardKeys(shown)
	// A band is as tall as its tallest trail, one band or three: the
	// strip follows it, rather than thirty rows of bare rail.
	return keys, heights
}

// boardPlace is where the selected column stands on the board: the
// column's left edge and its band's top row, so a panel about that
// session can sit beside it rather than over someone else's.
func (m *Model) boardPlace(w int) (x, y int, ok bool) {
	x, y, _, ok = m.boardBand(w)
	return x, y, ok
}

// boardBand is boardPlace with the height of the selected column's band.
func (m *Model) boardBand(w int) (x, y, bh int, ok bool) {
	x, y, bh, _, ok = m.boardBandAt(w)
	return x, y, bh, ok
}

// boardBandAt is boardBand with one more fact: whether the selected band is
// the board's last, so a panel knows if the rows under it are free or the
// next band's head.
func (m *Model) boardBandAt(w int) (x, y, bh int, last, ok bool) {
	n, cw := boardColumns(w, m.drawnCount(m.viewOrder()))
	if n == 0 {
		return 0, 0, 0, false, false
	}
	h := m.height - 5
	if h < 1 {
		h = 1
	}
	keys, heights := m.boardPack(n, cw, h-2)
	y = 0
	for i, key := range keys {
		if i > 0 && i%n == 0 {
			y += heights[i/n-1] + 1
		}
		if key == m.selectedKey {
			band := keys[(i/n)*n : min((i/n+1)*n, len(keys))]
			bw := bandWidth(w, len(band), cw)
			return (i % n) * (bw + gutterWidth), y, heights[i/n], i/n == (len(keys)-1)/n, true
		}
	}
	return 0, 0, 0, false, false
}

// bandWidth is a column's width in a band of k columns: the board's
// column width, or wider when the band is short of columns — a lone
// column under a full band was 38 wide beside 80 columns of nothing.
func bandWidth(w, k, cw int) int {
	if k <= 0 {
		return cw
	}
	_, bw := boardColumns(w, k)
	if k == 1 && bw > boardColMax && cw <= boardColMax {
		bw = boardColMax // a lone column under a full band: wider, capped like any other
	}
	if bw < cw {
		return cw
	}
	return bw
}

// drawnCount is how many of the view's sessions earn a column: the ones
// that owe you, or all of them when none does. The width is shared by
// these, not by every session: three owed columns on a 220-column board
// were 52 wide with 57 columns of air beside them.
func (m *Model) drawnCount(order []int) int {
	drawn := 0
	for _, i := range order {
		if m.archiveView || m.obligation(m.sessions[i]) <= owedRank {
			drawn++
		}
	}
	if drawn == 0 {
		drawn = len(order)
	}
	return drawn
}

// boardBandMin is the least height a band of columns is worth: the header's
// three rows and enough trail to read.
const boardBandMin = 10

// boardColumnRows is how many rows a column would take to show its whole
// trail: the header and the document.
func (m *Model) boardColumnRows(key string, w int) int {
	tr, ok := m.trails[key]
	if !ok {
		return 4
	}
	r, ok := m.boardRows()[key]
	if !ok {
		return 4
	}
	s := m.sessions[r.sess]
	doc := TrailLines(tr, TrailOpts{
		Todos: planItems(tr.Tasks), Head: m.headFor(s), HeadState: s.Snap.State, HeadSince: headSince(s),
		SessionKey: key, Now: m.now, Width: w, Height: 1000, Level: levelTrail, Cursor: -1, Pinned: true,
		Dense: true, Looked: m.looked(key),
	})
	return 3 + len(doc)
}

// boardRows numbers the board's sessions in the board's own order — the
// number beside a column is the key that selects it, `1` being the leftmost
// column and the strip carrying on from the last. The fleet list numbers its
// rows in its grouped order; a person on the board sees only these.
func (m *Model) boardRows() map[string]fleetRow {
	rows := map[string]fleetRow{}
	for pos, i := range m.viewOrder() {
		num := 0
		if m.archiveView {
			// As drawn, hidden rows included: a digit is a key, and a
			// hidden row wearing its fleet digit over a row numbered by
			// position gave one digit two sessions.
			if pos < 9 {
				num = pos + 1
			}
		} else {
			num = m.digits[m.sessions[i].Info.Key()]
		}
		rows[m.sessions[i].Info.Key()] = fleetRow{sess: i, num: num}
	}
	return rows
}

// assignDigits gives every live session its number on first sight and
// keeps it for the session's life: the rows re-sort as what they owe
// changes, the digits do not. A `3` from muscle memory then lands on the
// session it landed on this morning, however the board has moved since.
func (m *Model) assignDigits() {
	if m.digits == nil {
		m.digits = map[string]int{}
	}
	live := map[string]bool{}
	for _, s := range m.sessions {
		if s.Live {
			live[s.Info.Key()] = true
		}
	}
	for key := range m.digits {
		if !live[key] {
			delete(m.digits, key) // archived: its digit is free again
		}
	}
	taken := map[int]bool{}
	for _, d := range m.digits {
		taken[d] = true
	}
	// New sessions take the lowest free digit, in the board's own order.
	for _, i := range m.viewOrder() {
		key := m.sessions[i].Info.Key()
		if _, ok := m.digits[key]; ok {
			continue
		}
		for d := 1; d <= 9; d++ {
			if !taken[d] {
				m.digits[key], taken[d] = d, true
				break
			}
		}
	}
}

// boardSelect is `1`–`9` on the board: the column (or strip entry) wearing
// that number.
func (m *Model) boardSelect(i int) bool {
	order := m.viewOrder()
	if m.archiveView {
		if i < 0 || i >= len(order) {
			return false
		}
		m.point(m.sessions[order[i]].Info.Key())
		return true
	}
	for _, idx := range order {
		if m.digits[m.sessions[idx].Info.Key()] == i+1 {
			m.point(m.sessions[idx].Info.Key())
			return true
		}
	}
	return false
}

// overlaps is what two live sessions are doing to the same thing: a file
// both touched in the last twenty minutes, a test both are failing. The
// only thing in the room reading every transcript had two columns and let
// the person diff them by eye.
func (m *Model) overlaps() []string {
	const recent = 20 * time.Minute
	files := map[string][]string{}
	tests := map[string][]string{}
	for _, i := range m.viewOrder() {
		s := m.sessions[i]
		if !s.Live {
			continue
		}
		name := sessionName(s.Info)
		tr := m.trails[s.Info.Key()]
		seenFile := map[string]bool{}
		for _, l := range tr.Legs {
			end := l.End
			if l.Current || end.IsZero() {
				end = m.now
			}
			if m.now.Sub(end) > recent {
				continue
			}
			for _, f := range l.Files {
				if !seenFile[f] {
					seenFile[f] = true
					files[f] = append(files[f], name)
				}
			}
		}
		if test := failingNow(tr); test != "" {
			tests[test] = append(tests[test], name)
		}
	}
	var out []string
	for _, f := range sortedKeys(files) {
		if names := files[f]; len(names) > 1 {
			out = append(out, "⚠ "+strings.Join(names, " and ")+" both touched "+f+" in the last 20m")
		}
	}
	for _, t := range sortedKeys(tests) {
		if names := tests[t]; len(names) > 1 {
			out = append(out, "⚠ "+strings.Join(names, " and ")+" are both failing "+t)
		}
	}
	return out
}

// failingNow is the test the trail's newest red run failed, or "".
func failingNow(tr journey.Trail) string {
	if test, _, ok := circling(tr); ok {
		return test // a loop is failing its test whatever the last run said
	}
	for i := len(tr.Legs) - 1; i >= 0; i-- {
		badge := legBadge(tr.Legs[i])
		if badge == "" || badge == "?" {
			continue
		}
		if !strings.Contains(badge, "✗") {
			return ""
		}
		for _, w := range tr.Legs[i].Waypoints {
			if w.Kind == journey.WaypointTestFail {
				return w.Text
			}
		}
		return ""
	}
	return ""
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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
		glyph := m.rowGlyph(s)
		if m.archiveView && !s.Live {
			glyph = fleet.Glyph(state.Idle)
		}
		name := glyph + " " + sessionName(s.Info) + " " + m.age(s.Info.LastEventAt)
		if r, ok := rowOf[key]; ok && r.num > 0 {
			name = fmt.Sprintf("%d %s", r.num, name)
		}
		rest = append(rest, name)
	}
	// The clauses that carry a key — the overlaps, the hidden count, the
	// archive — are kept whole; the names of what owes nothing take the
	// width that is left, and are the part that is cut.
	var fixed []string
	if !m.archiveView {
		fixed = append(fixed, m.overlaps()...)
		if n := m.hiddenCount(); n > 0 {
			fixed = append(fixed, fmt.Sprintf("%d hidden · A, then x", n))
		}
		if n := m.archivedCount(); n > 0 {
			fixed = append(fixed, fmt.Sprintf("%d archived · A browses", n))
		}
	} else {
		fixed = append(fixed, "A live fleet")
	}
	tail := strings.Join(fixed, "   ")
	if len(rest) > 0 {
		names := fmt.Sprintf("+%d more · %s", len(rest), strings.Join(rest, " · "))
		room := w - lipgloss.Width(tail) - 3
		if lipgloss.Width(names) > room {
			// The overlap sentence goes compact before a session loses
			// its name on the screen: "+N more" is never shed.
			for i, f := range fixed {
				if strings.HasPrefix(f, "⚠") {
					fixed[i] = compactOverlap(f)
				}
			}
			tail = strings.Join(fixed, "   ")
			room = w - lipgloss.Width(tail) - 3
		}
		if room >= 12 {
			return dimStyle.Render(shedClauses(names, room)) + "   " + dimStyle.Render(tail)
		}
		return dimStyle.Render(clip(fmt.Sprintf("+%d more", len(rest))+"   "+tail, w))
	}
	return dimStyle.Render(clip(tail, w))
}

// boardColumn is one session's column: its two fleet rows as the header, a
// line of air, and its trail pinned to the present. A muted session's trail
// is drawn dim, glyphs and all: the shapes still carry the classes (SPEC §4),
// and the eye goes to the columns with something in them to read.
func (m *Model) boardColumn(key string, r fleetRow, w, h int) []string {
	s := m.sessions[r.sess]
	rows := m.columnHeader(key, r, w)
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
	headClass := ""
	if s.HasClass {
		headClass = s.Class.String()
	}
	opts := TrailOpts{
		HeadClass:    headClass,
		HeadDead:     s.Snap.APIError,
		HeadActivity: s.Snap.Activity,
		Todos:        planItems(tr.Tasks),
		Labels:       m.boardLabels[key],
		LaneLinks:    m.laneLinks(tr),
		Head:         m.headFor(s),
		HeadState:    s.Snap.State,
		HeadSince:    headSince(s),
		SessionKey:   key,
		Now:          m.now,
		Width:        w,
		Height:       h - 3,
		Level:        levelTrail,
		Cursor:       -1,
		Pulse:        m.pulse && working,
		Pinned:       true,
		Dense:        true, // the board always packs: a rail row between every leg halved what fit
		Looked:       m.looked(key),
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
		full := fmt.Sprintf("↑ %s above%s%s", plural(hidden, "leg"), began, foldTotals(tr, hidden, false))
		if len([]rune(full)) > w {
			full = fmt.Sprintf("↑ %s · %s%s", plural(hidden, "leg"), strings.TrimPrefix(began, " · began "), foldTotals(tr, hidden, true))
			full = strings.Replace(full, " ago", "", 1)
		}
		if len(lines) > 1 && isDetailRow(lines[1]) {
			// The fold took the row the parent stood on: the parent
			// takes the child's, so the column does not open on a child
			// with no name.
			lines[1] = lines[0]
		}
		lines[0] = dimStyle.Render(shedClauses(full, w))
	}
	if m.boardMuted(s) {
		for i, line := range lines {
			lines[i] = dimStyle.Render(ansi.Strip(line))
		}
	}
	return append(rows, lines...)
}

// columnHeader is a session's three-row card: the fleet row, the verdict
// (or the sentence a needs-you or stuck session owes), and what is new
// since the last look or where it lives. The board's columns wear it, and
// so does the session view, so the view reads as the column expanded.
func (m *Model) columnHeader(key string, r fleetRow, w int) []string {
	s := m.sessions[r.sess]
	entry := m.entryLines(r, w)
	second := entry[1]
	if tr, ok := m.trails[key]; ok && s.Snap.State == state.Working && !m.archiveView {
		// A working column shows its HEAD row anyway — pinned, it is always
		// the last row of the trail — so the header says what HEAD cannot:
		// how the suite stands, what shipped, what is still out.
		if parts := verdictParts(tr, m.now, true); len(parts) > 0 {
			second = "    " + dimStyle.Render(joinFit(parts, w-4))
		}
	}
	// The third row is the tag's row, always: the digest or the trace
	// takes the left of it and the tmux session the right, so where a
	// column lives never moves and is never evicted by what is new.
	tag := m.boardTag(s)
	room := w
	if tag != "" {
		room = w - lipgloss.Width(tag) - 2
	}
	third := ""
	if room >= 12 {
		third = m.boardDelta(key, s, room)
	}
	if tag != "" {
		third = pad(third, w-lipgloss.Width(tag)) + dimStyle.Render(tag)
	}
	return []string{entry[0], second, third}
}

// boardVerdict is how a journey came out, in words, from its own tail: what
// shipped, whether the suite is green, what is still out. It is the answer to
// "did it work" with zero keys and zero inference, which for eleven of twelve
// sessions on an afternoon board is the only thing anyone needed.
func boardVerdict(s fleet.Session, tr journey.Trail, now time.Time) string {
	parts := verdictParts(tr, now, s.Snap.State != state.Idle)
	if len(parts) == 0 {
		// Nothing countable: the newest completed leg, so a quiet column
		// still says what it last did.
		for i := len(tr.Legs) - 1; i >= 0; i-- {
			if l := tr.Legs[i]; !l.Current {
				label := l.Label
				if label == "" {
					label = l.Class.String()
				}
				parts = append(parts, l.Class.String()+" "+label)
				break
			}
		}
	}
	if promptWaits(tr) >= waitNotable {
		// The day's wait on you, the open one included: the number that
		// says which session your own turns are the bottleneck of. Only
		// when there is a day to add up — an idle session's open wait
		// alone is the age two rows above, and saying it twice taught
		// the eye to skip the clause. Last, so it is the first clause a
		// narrow column sheds.
		parts = append(parts, "on you "+relDuration(youWaited(tr, now, s))+" today")
	}
	return strings.Join(parts, " · ")
}

// circling reports whether a trail is going round the same failure: its
// newest run is red and the test it fails has failed in three legs or
// more. Stuck covers the session that went quiet; this is the one that
// fails loudly and keeps going, which the state machine calls healthy.
func circling(tr journey.Trail) (test string, runs int, ok bool) {
	// The last three runs with a verdict: a red one among them whose test
	// has failed in three legs is a loop, whatever a narrower green run
	// beside it says — "pytest tests/auth 312✓" between two red full
	// suites was reading as the loop ending.
	seen, greenest, greenArgs := 0, 0, 0
	for i := len(tr.Legs) - 1; i >= 0 && seen < 3; i-- {
		l := tr.Legs[i]
		badge := legBadge(l)
		if badge == "" || badge == "?" {
			continue
		}
		seen++
		if !strings.Contains(badge, "✗") {
			// A green run as big as the red one ends the loop; a smaller
			// one is a subset and says nothing about the failing test.
			if n := badgeCount(badge); n > greenest {
				greenest, greenArgs = n, len(strings.Fields(l.Label))
			}
			continue
		}
		if badgeCount(badge) <= greenest && greenArgs <= len(strings.Fields(l.Label)) {
			// The green run was as big and no narrower: "pytest
			// tests/auth 312✓" over a red "pytest" is a subset whatever
			// its count says, and the loop is still on.
			return "", 0, false
		}
		for _, w := range l.Waypoints {
			if w.Kind == journey.WaypointTestFail && w.Runs > runs {
				test, runs = w.Text, w.Runs
			}
		}
		if runs >= 3 {
			return test, runs, true
		}
	}
	return "", 0, false
}

// circlingSince is when a loop began: the first leg the failing test
// failed in. A loop's clock is its age, not its last write's.
func circlingSince(tr journey.Trail) time.Time {
	test, _, ok := circling(tr)
	if !ok {
		return time.Time{}
	}
	for _, l := range tr.Legs {
		for _, w := range l.Waypoints {
			if w.Kind == journey.WaypointTestFail && w.Text == test {
				return l.Start
			}
		}
	}
	return time.Time{}
}

// badgeCount is how many tests a badge counts: "310✓ 2✗" is 312.
func badgeCount(badge string) int {
	n := 0
	for _, f := range strings.Fields(badge) {
		digits := strings.TrimRight(f, "✓✗")
		if v, err := strconv.Atoi(digits); err == nil {
			n += v
		}
	}
	return n
}

// unfinished reports whether the session's plan has steps left.
func unfinished(tr journey.Trail) bool {
	for _, t := range tr.Tasks {
		if t.Status == "pending" || t.Status == "in_progress" {
			return true
		}
	}
	return false
}

// redNow reports whether the trail's newest verdict is red.
func redNow(tr journey.Trail) bool {
	for i := len(tr.Legs) - 1; i >= 0; i-- {
		badge := legBadge(tr.Legs[i])
		if badge == "" || badge == "?" {
			continue
		}
		return strings.Contains(badge, "✗")
	}
	return false
}

// obligation ranks a live session by what it owes you, for the board's
// order: a question, a hang, a loop, work in flight, an idle session that
// stopped red or with steps left, one you have not read, and the rest.
// The header's alarms sort first; among the calm, what stopped short of
// done comes before what shipped clean — "all calm" should mean nothing
// owes you, not merely nothing is amber.
func (m *Model) obligation(s fleet.Session) int {
	tr := m.trails[s.Info.Key()]
	switch s.Snap.State {
	case state.NeedsYou:
		if s.Snap.APIError {
			// Dead on the API: nothing you type clears it, so it sorts
			// under the question and the loop, which a keypress ends.
			return rankAPIError
		}
		return rankNeedsYou
	case state.Stuck:
		return rankStuck
	}
	if _, _, ok := circling(tr); ok {
		return rankCircling
	}
	if s.Snap.State == state.Working {
		if headWaits(tr) > 0 && parkedFor(tr, m.now) >= parkedNotable {
			return rankParked // a lead sitting on its agents past the quiet mark
		}
		return rankWorking
	}
	if !s.Info.LastEventAt.IsZero() && m.now.Sub(s.Info.LastEventAt) > boardFresh {
		return rankRest // a day old owes nothing today, red or not
	}
	if redNow(tr) || unfinished(tr) {
		return rankOwed
	}
	if m.unread(s) {
		return rankUnread
	}
	return rankRest
}

// The obligation ranks, in board order: what a keypress ends first, then
// what only time or a person elsewhere can clear, then work in flight, then
// what stopped short of done.
const (
	rankNeedsYou = iota
	rankStuck
	rankCircling
	rankAPIError
	rankParked
	rankWorking
	rankOwed
	rankUnread
	rankRest
)

// owedRank is the last obligation rank that earns a column of its own;
// past it a session is named in the strip.
const owedRank = rankUnread

// parkedNotable is how long a lead may sit on its agents before that is
// what it owes you: "◈3 out 20m · quiet 15m" above a session that is fine.
const parkedNotable = 10 * time.Minute

// parkedFor is how long HEAD has been quiet on its open lanes.
func parkedFor(tr journey.Trail, now time.Time) time.Duration {
	for i := len(tr.Legs) - 1; i >= 0; i-- {
		if tr.Legs[i].Current {
			return now.Sub(tr.Legs[i].End)
		}
	}
	return 0
}

// repeatRuns is how many legs the leg's most-repeated failing test has now
// failed in — 0 when nothing in it has failed before.
func repeatRuns(l journey.Leg) int {
	runs := 0
	for _, w := range l.Waypoints {
		if w.Kind == journey.WaypointTestFail && w.Runs > runs {
			runs = w.Runs
		}
	}
	return runs
}

// verdictParts is the verdict without its fallback: only what the trail
// can count. A working column's header uses this — its HEAD row already
// says what it is doing, and "test pytest" over it said less.
func verdictParts(tr journey.Trail, now time.Time, live bool) []string {
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
	switch {
	case out > 0 && !live:
		// The turn is over: what never came back is lost, not out.
		parts = append(parts, fmt.Sprintf("◈%d lost", out))
	case out > 0:
		// The same words HEAD uses, parked or not: "◈3 out 20m · quiet
		// 15m" is the header's to say as much as the trail's.
		parts = append(parts, strings.TrimPrefix(headTail(tr, now, true), "for "))
		if !strings.HasPrefix(parts[len(parts)-1], "◈") {
			parts[len(parts)-1] = fmt.Sprintf("◈%d out %s", out, relAge(now, oldest))
		}
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
	// A loop leads with its red: the newest run may be a narrower green
	// that did not end it, and a circling row over "✓ green" gave
	// opposite advice about interrupting.
	start, greenSince := len(tr.Legs)-1, ""
	if _, _, ok := circling(tr); ok {
		newest := -1
		for i := len(tr.Legs) - 1; i >= 0; i-- {
			badge := legBadge(tr.Legs[i])
			if badge == "" || badge == "?" {
				continue
			}
			if strings.Contains(badge, "✗") {
				if newest >= 0 {
					greenSince = strings.TrimSpace(tr.Legs[newest].Label) + " " + legBadge(tr.Legs[newest]) + " since"
				}
				start = i
				break
			}
			if newest < 0 {
				newest = i
			}
		}
	}
	for i := start; i >= 0; i-- {
		l := tr.Legs[i]
		badge := legBadge(l)
		if badge == "" || badge == "?" {
			continue // no verdict is not a verdict
		}
		red := strings.Contains(badge, "✗")
		word := "✓ green"
		if red {
			word = "✗ red"
		}
		verdict = word + " " + badge
		edited, shippedSince, rerunning := false, false, time.Time{}
		end := len(tr.Legs)
		if greenSince != "" {
			// The loop's red leads; what happened after the narrower
			// green run is that run's story, not the red's.
			for j := i + 1; j < len(tr.Legs); j++ {
				if b := legBadge(tr.Legs[j]); b != "" && b != "?" && !strings.Contains(b, "✗") {
					end = j
					break
				}
			}
		}
		for j := i + 1; j < end; j++ {
			switch c := tr.Legs[j]; {
			case c.Class == journey.Fix || c.Class == journey.Build:
				edited = true
			case c.Class == journey.Ship:
				shippedSince = true
			case c.Class == journey.Test && c.Current:
				rerunning = c.Start
			}
		}
		// The caveat is a clause of its own, so a narrow row sheds it whole
		// rather than cutting "edited sin…".
		suffix := ""
		switch {
		case red && shippedSince:
			suffix = "shipped on red"
			shipped = "" // the same fact, and this is the way to say it
		case !rerunning.IsZero():
			suffix = "rerunning for " + relAge(now, rerunning)
		case edited:
			suffix = "edited since"
		}
		if shipped != "" {
			parts = append(parts, shipped)
			shipped = ""
		}
		parts = append(parts, verdict)
		if runs := repeatRuns(l); red && runs >= 2 {
			// The same test red again: the loop is the column's news,
			// and a board column has no room for the test's name.
			// "· 3rd failure" — ten runes shorter than "same test 3rd
			// failure", which is what a 35-column board column shed. It
			// comes before the caveat: the count is what makes it a loop.
			parts = append(parts, ordinal(runs)+" failure")
		}
		if suffix != "" {
			parts = append(parts, suffix)
		}
		if greenSince != "" {
			parts = append(parts, greenSince) // the subset that ran green, last to stay
		}
		break
	}
	if shipped != "" {
		parts = append(parts, shipped) // no run to stand beside: the ship alone
	}
	// Agents all back: how many, and how many said nothing — the return
	// that never reached a fleet row.
	back, empty := 0, 0
	for _, b := range tr.Branches {
		if b.Done {
			back++
			if strings.TrimSpace(b.Report) == "" {
				empty++
			}
		}
	}
	if back > 0 && out == 0 {
		line := fmt.Sprintf("◈%d back", back)
		if empty > 0 {
			line += fmt.Sprintf(" · %d empty", empty)
		}
		parts = append(parts, line)
	}

	return parts
}

// boardSecondLine is the fleet's second row for the session with, where the
// fleet's grouping would have said it, the tmux session it lives in on the
// right: the one fact the list carried that the column otherwise loses.
func (m *Model) boardTag(s fleet.Session) string {
	if pane, ok := m.panes[s.Info.Key()]; ok && pane.Target != "" {
		group := tmuxSessionName(pane.Target)
		if m.sharesTmux(s) {
			// Two live sessions in one tmux session: the name alone is
			// the same string on both, and the pane is what tells them
			// apart — "⌁ harness:1.0", the address enter spends.
			group = pane.Target
		}
		if group == "" {
			return ""
		}
		return mirrorMark + " " + group // the pane mark, so "work" does not read as a state word
	}
	if s.Live && !m.archiveView {
		// No pane at all: said, not left as a blank where the tag goes.
		return "no pane"
	}
	return ""
}

// tookReply says whether a session's transcript shows a prompt at or after
// the moment a line was sent to it: the reply landed, and the trail says
// it better than the trace would.
func (m *Model) tookReply(key string, sent sentReply) bool {
	tr, ok := m.trails[key]
	if !ok {
		return false
	}
	for _, p := range tr.Prompts {
		if !p.At.Before(sent.at) {
			return true
		}
	}
	return false
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
	if sent, ok := m.sent[key]; ok && !m.tookReply(key, sent) {
		// A line compass typed and the session has not yet answered: the
		// row says so until the transcript shows the prompt landed — and
		// says which key it pressed: a menu's digit is not a typed line.
		verb := "↪ sent "
		if sent.answer > 0 {
			verb = fmt.Sprintf("↪ answered %d · ", sent.answer)
		}
		// The quote is the proof of what went and the clock says whether
		// it landed. Both when they fit. Narrow, a typed line keeps its
		// bytes over the clock (three dead rows reading "↪ sent · 0s ago"
		// could not tell /login from the resume turn) and an answer keeps
		// its digit and the clock; the quote is never cut inside.
		age := " · " + relAge(m.now, sent.at) + " ago"
		quote := `"` + sent.text + `"`
		trace := ""
		switch {
		case len([]rune(verb+quote+age)) <= w:
			trace = verb + quote + age
		case sent.answer == 0 && len([]rune(verb+quote)) <= w:
			trace = verb + quote
		case sent.answer == 0 && fitQuote(verb+quote, w) != "":
			trace = fitQuote(verb+quote, w) // the bytes clipped, never dropped for a cell
		default:
			trace = clip(strings.TrimSuffix(strings.TrimSuffix(verb, " · "), " ")+age, w)
		}
		// What is new rides after it when the row has room: the trace was
		// evicting the digest for as long as it stood.
		if digest := ansi.Strip(m.boardDigest(key, s, w)); digest != "" {
			for _, clause := range strings.Split(digest, " · ") {
				if len([]rune(trace+" · "+clause)) > w {
					break // the digest's clauses ride whole, as many as fit
				}
				trace += " · " + clause
			}
		}
		return dimStyle.Render(trace)
	}
	return m.boardDigest(key, s, w)
}

// boardDigest is the digest half of the third row: what came after the
// look, when both halves of that are known.
func (m *Model) boardDigest(key string, s fleet.Session, w int) string {
	seen, ok := m.seen[key]
	if at, had := m.lastLook[key]; had && key == m.selectedKey {
		seen, ok = at, true // the look before this one, while the session is open
	}
	if !ok {
		// No baseline: the brightness already says it is unread, and "↳ 4
		// legs · never opened" on every column of a fresh launch was a
		// constant wearing a row.
		return ""
	}
	if !seen.Before(s.Info.LastEventAt) {
		return ""
	}
	legs, ships, red, same := sinceLooked(m.trails[key], seen)
	if legs == 0 {
		return ""
	}
	// "↳ 38 legs · 1 ship · 6 red, all test_x · looked 4h ago": the
	// sentence a person back from lunch assembles by hand. The lanes
	// come first when there are any: a delegator reads the digest for
	// the agent that came back, and a narrow column shed that clause.
	var parts []string
	if out, back := lanesSince(m.trails[key], seen); out+back > 0 {
		switch {
		case out > 0 && back == 0:
			parts = append(parts, fmt.Sprintf("↳ %d agents out, none back", out))
		case out > 0:
			parts = append(parts, fmt.Sprintf("↳ %d agents out · %d back", out, back))
		default:
			parts = append(parts, "↳ "+plural(back, "agent")+" back")
		}
		parts = append(parts, plural(legs, "new leg"))
	} else {
		parts = append(parts, "↳ "+plural(legs, "new leg"))
	}
	if ships > 0 {
		parts = append(parts, plural(ships, "ship"))
	}
	if red > 0 {
		clause := fmt.Sprintf("%d red", red)
		if same != "" && red > 1 {
			clause += ", all " + same
		}
		parts = append(parts, clause)
	}
	parts = append(parts, "looked "+state.ShortDuration(m.now.Sub(seen))+" ago")
	return dimStyle.Render(joinFit(parts, w))
}

// lanesSince counts the lanes dispatched after the look: still out, and
// back.
func lanesSince(tr journey.Trail, looked time.Time) (out, back int) {
	for _, b := range tr.Branches {
		if !b.Start.After(looked) {
			continue
		}
		if b.Done {
			back++
		} else {
			out++
		}
	}
	return out, back
}

// sinceLooked adds up what happened after the last look: legs, ships, red
// runs — and the one test every red run failed, when it was one.
func sinceLooked(tr journey.Trail, looked time.Time) (legs, ships, red int, sameTest string) {
	tests := map[string]bool{}
	for _, l := range tr.Legs {
		if !l.Start.After(looked) {
			continue
		}
		legs++
		switch {
		case l.Class == journey.Ship:
			ships++
		case strings.Contains(legBadge(l), "✗"):
			red++
			for _, w := range l.Waypoints {
				if w.Kind == journey.WaypointTestFail {
					tests[w.Text] = true
				}
			}
		}
	}
	if len(tests) == 1 {
		for t := range tests {
			sameTest = t
		}
	}
	return legs, ships, red, sameTest
}

// foldTotals is what the day adds up to — the ships and the red runs of
// the whole trail — so a day that scrolled off still answers "what did it
// ship", with the same numbers the trail's title gives (counting only the
// hidden legs made the figures change with the terminal's width).
func foldTotals(tr journey.Trail, hidden int, compact bool) string {
	ships, red := 0, 0
	for _, l := range tr.Legs {
		switch {
		case l.Class == journey.Ship:
			ships++
		case strings.Contains(legBadge(l), "✗"):
			red++
		}
	}
	out := ""
	if compact {
		// "· 14⚑ 9✗ 2⟲" where "· 14 ships · 9 red · 2 compactions" does
		// not fit a column.
		if ships > 0 {
			out += fmt.Sprintf(" · %d⚑", ships)
		}
		if red > 0 {
			out += fmt.Sprintf(" %d✗", red)
		}
		if n := len(tr.Compactions); n > 0 {
			out += fmt.Sprintf(" %d%s", n, glyphCompact)
		}
		if d := promptWaits(tr); d >= waitNotable {
			out += " · on you " + relDuration(d)
		}
		return out
	}
	if ships > 0 {
		out += " · " + plural(ships, "ship")
	}
	if red > 0 {
		out += fmt.Sprintf(" · %d red", red)
	}
	if n := len(tr.Compactions); n > 0 {
		out += " · " + plural(n, "compaction")
	}
	if d := promptWaits(tr); d >= waitNotable {
		out += " · waited on you " + relDuration(d)
	}
	return out
}

// dayParts is the day's totals as clauses, for a row that sheds them whole:
// "3h", "16 ships", "10 red", "2 compactions", "waited on you 40m" — or the
// compact "16⚑ 10✗ 2⟲ · on you 40m".
func dayParts(tr journey.Trail, now time.Time, compact bool) []string {
	day := strings.TrimPrefix(trailDay(tr, now, compact), " · ")
	if day == "" {
		return nil
	}
	return strings.Split(day, " · ")
}

// headSince is the clock HEAD's figure counts from: for a hung session the
// last thing it wrote — "silent 4m" beside "no output for 4m" — and for the
// rest the state's own start.
func headSince(s fleet.Session) time.Time {
	if s.Snap.State == state.Stuck && !s.Info.LastEventAt.IsZero() {
		return s.Info.LastEventAt
	}
	return s.Snap.Since
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
	if m.lastLook == nil {
		m.lastLook = map[string]time.Time{}
	}
	if prev, ok := m.seen[key]; ok && !prev.Equal(m.now) {
		// The look before this one is the read-line while the trail is
		// open: marking the look on the way in erased the line the way in
		// was for.
		m.lastLook[key] = prev
	}
	m.seen[key] = m.now
	if m.opened == nil {
		m.opened = map[string]bool{}
	}
	m.opened[key] = true
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

// sharesTmux says whether another session on the board lives in the same
// tmux session as this one: when it does, the tag names the pane.
func (m *Model) sharesTmux(s fleet.Session) bool {
	pane, ok := m.panes[s.Info.Key()]
	if !ok || pane.Target == "" {
		return false
	}
	name := tmuxSessionName(pane.Target)
	for _, o := range m.sessions {
		if o.Info.Key() == s.Info.Key() || !o.Live {
			continue // hidden or not: the namesake is still in that tmux session
		}
		if p, ok := m.panes[o.Info.Key()]; ok && tmuxSessionName(p.Target) == name {
			return true
		}
	}
	return false
}

// shedClauses fits a " · "-joined line to w by dropping its trailing
// clauses whole, and clips only a lone clause that is still too long.
func shedClauses(s string, w int) string {
	for lipgloss.Width(s) > w {
		i := strings.LastIndex(s, " · ")
		if i < 0 {
			return clip(s, w)
		}
		s = s[:i]
	}
	return s
}
