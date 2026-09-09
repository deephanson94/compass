package ui

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

// A fleet entry is two lines — the verdict and, quieter beneath it, where the
// session lives — plus one blank line of air. A group header is one line, glued
// to the entries beneath it: the header belongs to its group, not to the air.
const (
	entryLines = 2
	nameWidth  = 9
	ageWidth   = 4

	// elsewhereGroup holds the live sessions tmux does not account for. When it
	// is the only group its header is dropped: a degenerate tree is a list.
	elsewhereGroup = "elsewhere"
)

// fleetGroup is one bucket of the fleet column — a tmux session in the live
// view, a project directory in the archive — holding indices into m.sessions.
type fleetGroup struct {
	name    string
	entries []int
}

// fleetRow is one block of the rendered column: a group header, or a session.
// The rows are the rendered order, which is also the order `1`–`9` counts in
// and the order `j`/`k` walks (headers skipped).
type fleetRow struct {
	header bool
	label  string // header text
	age    string // header: how long since the freshest session in the group spoke
	echo   string // right-aligned ▲/◍ when the group holds one
	sess   int    // index into m.sessions
	num    int    // 1–9, or 0 for the rows past the ninth
}

// fleetColumn is the deck's left panel: the title, one line of air, and the
// fleet itself. The title says which fleet you are looking at — the live one
// or the archive.
func (m *Model) fleetColumn(w, h int) []string {
	title := "FLEET · live"
	if m.archiveView {
		title = "FLEET · archive"
	}
	if m.fleetQuery != "" {
		title += " · /" + m.fleetQuery
	}
	head := m.titleMark(panelFleet) + m.titleStyleFor(panelFleet).Render(clip(title, w-1))
	if m.level == levelTrail && !m.showHelp {
		// Where the keys are, in the help's own words (#20): at Lv1 they
		// are here, and the word stands with the mark (#63).
		if word := "[fleet]"; lipgloss.Width(head)+1+len(word) <= w {
			head = pad(head, w-len(word)) + dimStyle.Render(word)
		}
	}
	rows := []string{head, ""}
	if h > 2 {
		rows = append(rows, m.fleetLines(w, h-2)...)
	}
	return rows
}

// hiddenClause is the strip's word for its hidden sessions, with the way
// to them. On the frame whose note just said the same words the note is
// the one that stands down (#64 the other way round): the strip has the
// cells and the footer has none (#173).
func (m *Model) hiddenClause(n int) string {
	return fmt.Sprintf("%s hidden · A, then x", m.hiddenDoorCount(n))
}

// hiddenDoorCount is the strip's hidden count: the whole, or what the
// search left of it in the door's own form (`0 of 1`), as archiveDoorCount
// beside it — `1 of 41 archived · 1 hidden` promised a hidden session the
// archive under `/pytest` did not list (#176, #169).
func (m *Model) hiddenDoorCount(n int) string {
	if m.fleetQuery == "" {
		return strconv.Itoa(n)
	}
	matched := 0
	for _, s := range m.sessions {
		if s.Live && !m.onBoard(s) && m.matchesQuery(s) {
			matched++
		}
	}
	return strconv.Itoa(matched) + " of " + strconv.Itoa(n)
}

// shedHiddenSearch is the door's rung under the key: at eighty
// `1 of 41 archived · 0 of 1 hidden · A` is 34 cells in a 32-cell column,
// and the hidden count's search clause goes before the key does (#176).
func shedHiddenSearch(line string) string {
	return hiddenSearchRe.ReplaceAllString(line, "$1 hidden")
}

var hiddenSearchRe = regexp.MustCompile(`(\d+) of \d+ hidden`)

// fleetLines renders the fleet: grouped the way the user thinks of it, scrolled
// so the selection is always whole on screen, and — in the live view — closed
// by the dim line that says how much history is one keypress away.
// archiveDoorCount is the door's count: the whole archive, or what the
// search left of it in the header's own form (`0 of 300`). The door is
// one sentence drawn from three places — the list's last line, the
// board's strip and a fleet of one's own door — and at eighty the list
// said `0 of 300 archived · A browses` while the board twenty columns
// wider said `300 archived · A browses` over the same fleet and the same
// search (#164, #168).
func (m *Model) archiveDoorCount(n int) string {
	if m.fleetQuery == "" {
		return strconv.Itoa(n)
	}
	matched := 0
	for _, s := range m.sessions {
		if !s.Live && archiveHeadline(s) != "" && m.matchesQuery(s) {
			matched++
		}
	}
	return strconv.Itoa(matched) + " of " + strconv.Itoa(n)
}

func (m *Model) fleetLines(w, h int) []string {
	// The archive count is not part of the list: it is the column's last word,
	// so it survives any amount of scrolling — and it is how the archive is
	// discovered, so it stands even when nothing at all is live.
	var tail []string
	if !m.archiveView {
		archived, hidden := m.archivedCount(), m.hiddenCount()
		count := m.archiveDoorCount(archived)
		last := ""
		switch {
		case archived > 0 && hidden > 0:
			last = fmt.Sprintf("%s archived · %s hidden · A browses", count, m.hiddenDoorCount(hidden))
		case archived > 0:
			last = fmt.Sprintf("%s archived · A browses", count)
		case hidden > 0:
			// Below the board's width there is no strip: the list's last
			// line is where a hide stays said.
			last = m.hiddenClause(hidden)
		}
		if lipgloss.Width(last) > w {
			last = strings.Replace(last, " · A browses", " · A", 1) // the key survives whole
		}
		if lipgloss.Width(last) > w {
			last = shedHiddenSearch(last) // the hidden count's search clause goes before the key is clipped (#176)
		}
		if last != "" {
			tail = []string{"", dimStyle.Render(clip(last, w))}
		}
		if over := m.overlaps(); len(over) > 0 && !m.boardShown() {
			// The one sentence only compass can say, above the archive
			// row: it was board-only furniture, and two of five widths
			// have no board. Narrow, it keeps the fact and drops the prose.
			line := over[0]
			if lipgloss.Width(line) > w {
				line = shedClauses(compactOverlap(line), w) // the fact, then the names, then the clock
			}
			tail = append([]string{"", dimStyle.Render(clip(line, w))}, tail...)
		}
	}

	rows := m.fleetRows()
	if len(rows) == 0 {
		note := "nothing live"
		if m.archiveView {
			note = "no archived sessions"
		}
		if m.fleetQuery != "" {
			// Two rows, so the way out is never the clipped half. Under
			// them, the band holds what the search found among the
			// finished (#98): the miss is the live list's, and says so.
			miss := []string{dimStyle.Render(clip("no session matches /"+m.fleetQuery, w)), dimStyle.Render("esc clears it")}
			if band := m.recentLines(w, h-len(miss)-1); len(band) > 1 && !m.archiveView {
				miss[0] = dimStyle.Render(clip("no live session matches /"+m.fleetQuery, w))
				return append(append(miss, ""), band...)
			}
			return append(miss, tail...)
		}
		return append([]string{dimStyle.Render(clip(note, w))}, tail...)
	}
	if m.archiveView && m.archivedCount() == 0 {
		// Only the hidden on this list: the empty archive says so under
		// them rather than leaving the floor to say it.
		empty := "no finished sessions yet — they land here"
		if lipgloss.Width(empty) > w {
			empty = "no finished sessions yet" // the clause goes whole, never "they land h…"
		}
		tail = []string{"", dimStyle.Render(clip(empty, w))}
	}

	body := h - len(tail)
	if body < 1 {
		body = 1
	}
	lines, selStart, selEnd := m.fleetBlock(rows, w)
	if free := body - len(lines) - 1; free+len(tail) >= 2 && !m.archiveView && m.archivedCount() > 0 && len(m.overlaps()) == 0 {
		// The rows the live list leaves belong to the sessions that
		// ended last (#47): the band takes the archive's line as its
		// header, so the key is said once, and the rows below it.
		// The two rows the archive's line gave up are the band's too: it
		// folds that line into its own header, and a band sized against
		// the reduced body left two ranked sessions off for air (#81).
		// The gate measures the same rows the band is budgeted with: at
		// eighty a gate on the reduced body refused the band two rows it
		// had, and the column ended on the archive's line over two blank
		// rows (#92).
		if band := m.recentLines(w, free+len(tail)); len(band) > 1 {
			tail = nil
			lines = append(lines, "")
			lines = append(lines, band...)
			body = h
		}
	}
	win := m.scrollFleet(lines, selStart, selEnd, body)
	if slack := body - len(win); len(lines) > body && slack > 0 &&
		!m.archiveView && m.archivedCount() > 0 && len(m.overlaps()) == 0 {
		// The rows an entry-whole window could not use are still the
		// fleet's, not air (#47, #137): the band takes them, with the archive's
		// line folded into its header as it is when the list fits.
		if band := m.recentLines(w, slack+len(tail)-1); len(band) > 1 {
			win = append(win, "")
			win = append(win, band...)
			tail = nil
			body = h
		}
	}
	if len(lines) > body && len(win) < body {
		// A folded list short of its body — the rows an entry-whole
		// window could not use: the air goes between the fold and the
		// strip, where it reads as slack, not under the archive's line,
		// where it read as room the fold could have shown.
		win = append(win, make([]string, body-len(win))...)
	}
	return append(win, tail...)
}

// fleetBlock renders every row, and reports which lines the selected session
// occupies — the pair the scroller must never cut in half.
func (m *Model) fleetBlock(rows []fleetRow, w int) (lines []string, selStart, selEnd int) {
	selStart, selEnd = -1, -1
	prevHeader := false
	for i, r := range rows {
		if i > 0 && !prevHeader {
			lines = append(lines, "")
		}
		if r.header {
			lines = append(lines, m.groupHeaderLine(r, w))
			prevHeader = true
			continue
		}
		prevHeader = false
		entry := m.entryLines(r, w)
		if m.sessions[r.sess].Info.Key() == m.selectedKey && selStart < 0 {
			selStart = len(lines)
			selEnd = selStart + len(entry) - 1
		}
		lines = append(lines, entry...)
	}
	return lines, selStart, selEnd
}

// foldKey is the key clause the fold wears — " · j", " · k" — and it is worn
// only where that key is the list's own. `j` and `k` move the fleet at Lv1;
// at Lv2 and Lv3 the same keys have been handed to the trail's rows and the
// reader's page (`focus`), so pressing `j` on such a frame walks the cursor
// and leaves every folded row folded, while the fleet's title wears no `▌`.
// The count is the row's own answer at every level — the list is cut here,
// and by this much (#137) — and the way back to the list is `esc back`,
// which the footer of those frames already names (#232, #250).
//
// The panel is not the whole of it. At Lv1 the keys are the fleet's only
// while nothing else is being typed: a search, the quick replies and the
// reply's own line each take every key that reaches them ("While a search
// query is being typed, every key belongs to it", app.go), so `j` pressed
// on such a frame types a letter into the query or the line, or puts the
// replies away — the list stands still and every folded row stays folded,
// under a footer that names `esc` for the way back. The clause goes there
// for the same reason it goes at Lv2 (#280); the count stays (#137).
func (m *Model) foldKey(k string) string {
	if m.focus() != panelFleet {
		return ""
	}
	if m.searching || m.replying {
		return ""
	}
	return " · " + k
}

// scrollFleet windows the column onto h lines, following the selection: the
// offset moves as little as it can, so headers travel with their groups and
// the list only slides when the selection would otherwise fall off the edge.
//
// A list cut at the panel's edge says so — "▾ 5 more below" — on a row of
// its own, never over a session's: seven sessions with five more below them
// read as a fleet of seven, and nothing on the screen said otherwise.
func (m *Model) scrollFleet(lines []string, selStart, selEnd, h int) []string {
	if len(lines) <= h {
		m.fleetScroll = 0
		return lines
	}
	off := m.fleetScroll
	top, bottom, body := 0, 0, h
	// The notices take rows from the body, and a smaller body can move the
	// offset, which can change which notices are due: settle it in a few
	// passes — each one only moves the offset toward the selection.
	for pass := 0; pass < 4; pass++ {
		top, bottom = 0, 0
		if off > 0 {
			top = 1
		}
		if off+h-top < len(lines) {
			bottom = 1
		}
		body = h - top - bottom
		if body < 1 {
			body = 1
		}
		next := off
		if selEnd >= 0 {
			if selStart < next {
				next = selStart
			}
			if selEnd >= next+body {
				next = selEnd - body + 1
			}
		}
		if next > len(lines)-body {
			next = len(lines) - body
		}
		if next < 0 {
			next = 0
		}
		if next > 0 && countEntries(lines[:next]) == 0 {
			next = 0 // a fold over a header and its air alone is a fold of nothing
		}
		// Never open the column on a line of air: the air between two
		// entries reads as a gap under the title. Losing it at the top costs
		// nothing — the row it would have pushed out is a blank at the
		// bottom, where nobody sees it.
		if next > 0 && lines[next] == "" && (selEnd < 0 || next+1 <= selStart) {
			next++
		}
		// Nor inside an entry: a row's second or third line under "N more
		// above" was a tag on nothing.
		for next > 0 && next < len(lines) && lines[next] != "" && !isHeaderLine(lines[next]) && !isEntryFirstLine(lines[next]) {
			if selEnd >= 0 && selEnd-(next-1) >= body {
				// Backing up would drop the selection off the bottom:
				// open on the next entry instead, the cut entry counted
				// among the rows above.
				for next < len(lines) && lines[next] != "" && !isHeaderLine(lines[next]) && !isEntryFirstLine(lines[next]) && (selStart < 0 || next < selStart) {
					next++
				}
				break
			}
			next--
		}
		// The escape above walks forward out of a cut entry and stops on
		// the air that ends it: the same rule as the guard above, applied
		// after it, so a scrolled column never opens on a blank row.
		if next > 0 && next < len(lines) && lines[next] == "" && (selEnd < 0 || next+1 <= selStart) {
			next++
		}
		if next == off {
			break
		}
		off = next
	}
	end := 0
	for pass := 0; pass < 8; pass++ {
		top, bottom = 0, 0
		if off > 0 {
			top = 1
		}
		if off+h-top < len(lines) {
			bottom = 1
		}
		body = h - top - bottom
		if body < 1 {
			body = 1
		}
		end = off + body
		if end > len(lines) {
			end = len(lines)
		}
		// A window never ends on a header: "ops" over nothing but the notice
		// beneath it named a group with no rows.
		for end > off+1 && end < len(lines) && (isHeaderLine(lines[end-1]) || (lines[end-1] == "" && end > off+2 && isHeaderLine(lines[end-2])) || isEntryFirstLine(lines[end-1]) ||
			(lines[end] != "" && !isHeaderLine(lines[end]) && !isEntryFirstLine(lines[end]))) {
			end-- // never inside an entry either: its third line is not dropped without a mark
		}
		if selEnd < 0 || selEnd < end || off >= len(lines)-1 {
			break
		}
		// The trim above dropped the selected entry off the bottom: the
		// cursor was nowhere and "j" looked like it did nothing (#86).
		// Open the window on the next entry and settle again.
		off++
		for off < len(lines) && (lines[off] == "" || (!isHeaderLine(lines[off]) && !isEntryFirstLine(lines[off]))) {
			off++
		}
		if off > len(lines)-1 {
			off = len(lines) - 1
		}
	}
	m.fleetScroll = off
	var out []string
	if top > 0 {
		// The fold names the group it cut into: under "▴ 1 more above"
		// a row's pane tag read ":0.0" of no tmux session, its "⌁ harness"
		// header being the first thing the fold hid.
		count := fmt.Sprintf("%d more above%s", countEntries(lines[:off]), m.foldKey("k"))
		fold := "▴ " + count
		if group := foldedHeader(lines, off); group != "" {
			fold = "▴ " + group + " · " + count
		}
		fw := 0 // the column's width, as the rows were drawn to it
		for _, l := range lines {
			fw = max(fw, lipgloss.Width(l))
		}
		if lipgloss.Width(fold) > fw {
			fold = "▴ " + count // the group's name goes before the count does
		}
		out = append(out, dimStyle.Render(fold))
	}
	out = append(out, lines[off:end]...)
	if bottom > 0 {
		out = append(out, dimStyle.Render(fmt.Sprintf("▾ %d more below%s", countEntries(lines[end:]), m.foldKey("j"))))
	}
	return out
}

// foldedHeader is the group header the fold at off hid, when the window's
// first row is one of that group's entries rather than a header of its own.
func foldedHeader(lines []string, off int) string {
	if off >= len(lines) || isHeaderLine(lines[off]) || (lines[off] == "" && off+1 < len(lines) && isHeaderLine(lines[off+1])) {
		return ""
	}
	for i := off - 1; i >= 0; i-- {
		if isHeaderLine(lines[i]) {
			// The name alone: the header carries the group's echo glyph at
			// its far edge, padded to the column.
			return strings.TrimSpace(strings.SplitN(strings.TrimSpace(ansi.Strip(lines[i])), "  ", 2)[0])
		}
	}
	return ""
}

// fleetRows groups the sessions of the current view and flattens them into the
// rendered order: header, its entries, the next header. Numbering runs flat
// down the sessions, groups ignored (M5 contract).
func (m *Model) fleetRows() []fleetRow {
	groups := m.liveGroups()
	if m.archiveView {
		groups = m.archiveGroups()
	}
	// A single `elsewhere` group means there is no tmux to speak of: a tree with
	// one unnamed branch is just a list, so the header goes.
	headers := !(!m.archiveView && len(groups) == 1 && groups[0].name == elsewhereGroup)

	// The numbers are the view's order, not the list's: a session wears the
	// same digit here as on the board, however the list groups it.
	numbers := m.boardRows()
	rows := make([]fleetRow, 0, len(m.sessions)+len(groups))
	for _, g := range groups {
		if headers {
			hdr := fleetRow{header: true, label: g.name}
			if !m.archiveView && g.name != elsewhereGroup {
				// The group is the tmux session: say so, because "work"
				// over a row read as a word, not a place to attach to.
				hdr.label = "⌁ " + g.name
			}
			if len(g.entries) > 1 {
				// A header over one row would only repeat it — and so
				// would a clock equal to the first row's beneath it.
				hdr.age, hdr.echo = m.groupAge(g), m.groupEcho(g)
				if hdr.echo != "" || hdr.age == m.age(headSince(m.sessions[g.entries[0]])) {
					// The row beneath says it. And beside an echo, any
					// clock reads as the echo's — "◍ 1m" over "stuck 4m"
					// was three numbers for one fact.
					hdr.age = ""
				}
			}
			rows = append(rows, hdr)
		}
		for _, i := range g.entries {
			rows = append(rows, fleetRow{sess: i, num: numbers[m.sessions[i].Info.Key()].num})
		}
	}
	return rows
}

// fleetOrder is the rendered session order without the headers: what `1`–`9`
// counts and what `j`/`k` walks.
func (m *Model) fleetOrder() []int {
	rows := m.fleetRows()
	out := make([]int, 0, len(rows))
	for _, r := range rows {
		if !r.header {
			out = append(out, r.sess)
		}
	}
	return out
}

// liveGroups buckets the live sessions by their tmux session, in the order the
// pane list first mentions each one — tmux's own order, the one `ctrl-b s`
// shows. Live sessions with no pane form a final `elsewhere` group.
func (m *Model) liveGroups() []fleetGroup {
	var order []string
	seen := map[string]bool{}
	for _, p := range m.paneList {
		name := tmuxSessionName(p.Target)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		order = append(order, name)
	}

	members := map[string][]int{}
	for i, s := range m.sessions {
		if !m.onBoard(s) || !m.matchesQuery(s) {
			continue
		}
		name := elsewhereGroup
		if pane, ok := m.panes[s.Info.Key()]; ok && pane.Target != "" {
			if n := tmuxSessionName(pane.Target); n != "" {
				name = n
			}
		}
		if name != elsewhereGroup && !seen[name] {
			// A mapped pane the list never mentioned: keep it, at the end.
			seen[name] = true
			order = append(order, name)
		}
		members[name] = append(members[name], i)
	}

	groups := make([]fleetGroup, 0, len(order)+1)
	for _, name := range order {
		if len(members[name]) == 0 {
			continue // a tmux session compass has nothing in
		}
		groups = append(groups, fleetGroup{name: name, entries: m.orderLiveGroup(members[name])})
	}
	// A group holding an alarm — a question, a hang, a loop — floats
	// above the rest: below the board's width the list is the board, and
	// the circling session sat under a healthy one because of its tmux
	// session's place in the pane list.
	best := func(g fleetGroup) int {
		b := rankRest + 1
		for _, i := range g.entries {
			if r := m.obligation(m.sessions[i]); r < b {
				b = r
			}
		}
		return b
	}
	sort.SliceStable(groups, func(a, b int) bool { return best(groups[a]) < best(groups[b]) })
	if e := members[elsewhereGroup]; len(e) > 0 {
		groups = append(groups, fleetGroup{name: elsewhereGroup, entries: m.orderLiveGroup(e)})
	}
	return groups
}

// orderLiveGroup sorts one group: window.pane index order, except that a
// needs-you or stuck session floats to the top of its group — never out of it.
// Sessions already arrive in fleet order, so the floaters keep the Manager's
// longest-waiting-first ranking among themselves.
func (m *Model) orderLiveGroup(idx []int) []int {
	out := append([]int(nil), idx...)
	// The board's own order inside the group, then window.pane: the
	// digits ran 4, 2, 1, 3 down a panel when the tmux index decided.
	// The calm tiers — stopped short, unread, read — are one tier here:
	// below the board's width, selecting a row is reading it, and a row
	// that re-sorted to the bottom on every `j` was a list nobody could
	// walk.
	tier := func(i int) int {
		return min(m.obligation(m.sessions[i]), rankUnread) // what stopped short floats; read and unread are one tier
	}
	sort.SliceStable(out, func(a, b int) bool {
		return tier(out[a]) < tier(out[b]) // then the fleet's own order, as the board has it
	})
	return out
}

// hiddenGroup is the archive's first group: live sessions `x` took off the
// board, so the way back is where the strip said it was.
const hiddenGroup = "hidden · x brings one back"

// archiveGroups buckets the archived sessions by project — where you started
// them, which is how you remember them — newest group first, newest first
// inside.
func (m *Model) archiveGroups() []fleetGroup {
	members := map[string][]int{}
	var names []string
	for i, s := range m.sessions {
		if m.onBoard(s) || !m.matchesQuery(s) {
			continue
		}
		name := projectOf(s.Info)
		if s.Live {
			name = hiddenGroup // what `x` took off the board, first
		}
		if _, ok := members[name]; !ok {
			names = append(names, name)
		}
		members[name] = append(members[name], i)
	}

	newest := func(idx []int) (t int64) {
		for _, i := range idx {
			if u := m.sessions[i].Info.LastEventAt.UnixNano(); u > t {
				t = u
			}
		}
		return t
	}
	for _, name := range names {
		e := members[name]
		sort.SliceStable(e, func(a, b int) bool {
			return m.sessions[e[a]].Info.LastEventAt.After(m.sessions[e[b]].Info.LastEventAt)
		})
	}
	sort.SliceStable(names, func(a, b int) bool { return newest(members[names[a]]) > newest(members[names[b]]) })

	groups := make([]fleetGroup, 0, len(names))
	for _, name := range names {
		groups = append(groups, fleetGroup{name: name, entries: members[name]})
	}
	return groups
}

// archivedCount is how much history sits behind the `A` key.
func (m *Model) archivedCount() int {
	n := 0
	for _, s := range m.sessions {
		if !s.Live {
			n++
		}
	}
	return n
}

// groupEcho is the header's right-aligned reminder that something inside the
// group wants you — the one warm mark the fleet is allowed to repeat.
func (m *Model) groupEcho(g fleetGroup) string {
	// The group's worst row, as the board ranks it — a ▲ over a group of
	// ◍ ↻ ⊘ pointed at the wrong group — and nothing when that row is the
	// one right beneath the header.
	worst, rank := -1, rankAPIError+1
	for _, i := range g.entries {
		if r := m.obligation(m.sessions[i]); r < rank {
			worst, rank = i, r
		}
	}
	if worst < 0 {
		return ""
	}
	return m.rowGlyph(m.sessions[worst])
}

// isHeaderLine recognises a group header among rendered fleet lines: it
// opens with its name after one column of gutter, where an entry opens
// with a mark or a number and a second line with four spaces.
func isHeaderLine(l string) bool {
	r := []rune(ansi.Strip(l))
	return len(r) > 1 && r[0] == ' ' && (unicode.IsLetter(r[1]) || r[1] == '⌁')
}

// isEntryFirstLine recognises the first of an entry's two lines: a window
// that ends on it shows a name with its second line cut off.
func isEntryFirstLine(l string) bool {
	// A mark or a space, a digit or a space, a space, the glyph: a second
	// line's "◍ build …" after four spaces is not an entry, and a loop's ↻
	// or a dead session's ⊘ is.
	r := []rune(ansi.Strip(l))
	return len(r) > 3 && (r[0] == ' ' || r[0] == '▸') && (r[1] == ' ' || (r[1] >= '0' && r[1] <= '9')) &&
		r[2] == ' ' && strings.ContainsRune("●▲◍○↻⊘", r[3])
}

// countEntries counts the session rows among rendered fleet lines: the
// first line of each entry is the one that opens, past its cursor mark and
// number, with a state glyph. Headers open with a name and second lines
// with a class or an activity.
func countEntries(lines []string) int {
	n := 0
	for _, l := range lines {
		if isEntryFirstLine(l) {
			n++
		}
	}
	return n
}

// groupAge is how long since the freshest session in a group last spoke. Inside
// a group the rows are in window.pane order, so the list mirrors the screen
// `enter` takes you to — which means their ages do not descend and you cannot
// scan for the recent one. Hoisting the freshest to the header restores that
// without disturbing the order underneath it.
func (m *Model) groupAge(g fleetGroup) string {
	// The echo's own clock is never here: the row it echoes floats to the
	// top of its group, so its wait is the row beneath the header, and
	// fleetRows drops a clock that repeats that row.
	var newest time.Time
	for _, i := range g.entries {
		if at := m.sessions[i].Info.LastEventAt; at.After(newest) {
			newest = at
		}
	}
	if newest.IsZero() {
		return ""
	}
	return m.age(newest)
}

// groupHeaderLine draws a header: dim, unnumbered, unselectable, sitting in the
// index column so the group name lines up with the numbers beneath it. Its
// right edge carries the same two facts a session row's does, in the same
// columns: how long since anything happened, and whether anything wants you.
func (m *Model) groupHeaderLine(r fleetRow, w int) string {
	// The age sits in the same column the session rows put theirs, so the two
	// read as one column; the echo floats just left of it rather than taking
	// the edge, which is what made "2m▲" out of two separate facts.
	tail := ""
	if r.age != "" {
		tail = padLeft(r.age, ageWidth)
	}
	right := len(tail) + echoWidth(r.echo)
	if right == 0 {
		return dimStyle.Render(" " + clip(r.label, w-1))
	}
	label := " " + clip(r.label, w-1-right)
	head := dimStyle.Render(pad(label, w-right))
	if r.echo == "" {
		return head + dimStyle.Render(tail)
	}
	accent := needsYouStyle
	if r.echo == fleet.Glyph(state.Stuck) {
		accent = stuckStyle
	}
	return head + accent.Render(r.echo) + dimStyle.Render(tail)
}

// echoWidth is the room a header echo takes, air included.
func echoWidth(echo string) int {
	if echo == "" {
		return 0
	}
	return 1
}

// entryLines renders one session: "N ● name  activity  age" over a dim line
// saying where it lives.
func (m *Model) entryLines(r fleetRow, w int) []string {
	return m.entryLinesUnder(r, w, "")
}

// entryLinesUnder is entryLines under the tag the caller draws beneath it:
// the board's column draws one on its third row, the fleet list and the
// archive list draw none. What that tag already says, the row does not say
// again (#64) — the pane it took off the row from the first (#53), and the
// tool word beside it.
func (m *Model) entryLinesUnder(r fleetRow, w int, tag string) []string {
	s := m.sessions[r.sess]
	selected := s.Info.Key() == m.selectedKey
	st := s.Snap.State
	if m.archiveView && !s.Live {
		st = state.Idle // the archive can never be amber (M5 contract, fleet rule 2)
	}
	accent := stateStyle(st)

	marker := " "
	if selected {
		marker = "▸"
	}
	index := " "
	if r.num > 0 {
		index = strconv.Itoa(r.num)
	}
	indexStyled := dimStyle.Render(index)
	nameStyle := dimStyle
	if selected {
		indexStyled = textStyle.Render(index)
		nameStyle = textStyle
	}

	age := padLeft(m.age(headSince(s)), ageWidth)
	if at := circlingSince(m.trails[s.Info.Key()]); m.isCircling(s) && !at.IsZero() && st != state.Idle {
		// A loop in flight wears the loop's age, not its last write's; an
		// idle loop keeps its wait for a prompt, which is what decides
		// whether to type.
		age = padLeft(m.age(at), ageWidth)
	}
	head := headline(s)
	if head == "" && m.unread(s) {
		// Unread is a word, not only a brightness: monochrome has to read
		// it, and below the board's width there is no brightness at all.
		head = "unread"
	}
	if m.archiveView {
		// The group header already names the project, and an archived session is
		// named after its project: the row spends that width on the one thing the
		// header cannot say — what you asked for.
		age, head = padLeft(m.age(s.Info.LastEventAt), ageWidth), archiveHeadline(s)
		if m.askBelow {
			// On the board the card below this row draws the very same
			// ask on its own ◉ row, with its own clock — the sentence
			// stood twice, three rows apart, in one card (#64, #107: the
			// card yields what a row below it says). The head keeps the
			// name the archive gives the session: the one its person
			// typed (#79, #234), else the project it is grouped under
			// (#11), which the archive's board draws on no other row —
			// its list draws it as a heading (#234) and its board draws
			// no heading at all. #86's reason for the ask here — "four
			// project names cannot tell forty sessions apart" — is the
			// strip's, where nothing else says what was asked; on the
			// card the ask is three rows down and still tells them apart.
			head = ""
		} else if s.Live || s.Info.Name != "" {
			// A live session `x` took off the board: it keeps its glyph
			// and its name — the header over it says it is hidden, and
			// "hidden · add watch driver tests" lost the one word the
			// person went looking for. A session its person renamed is
			// the same case since the bucket became the project (#234):
			// the heading says `webapp`, the row says what was asked, and
			// the name the person typed was on neither. It keeps its name
			// before its prompt, as its header, its trail title and the
			// band already do (#79).
			//
			// Where the trail drawn beside this very row draws that ask
			// on its own ◉ row, the row says it a second time on the same
			// frame, once each side of the rule — #111's shape, and
			// #107's: the row yields what a row beside it says, and keeps
			// the name. At 220 the row cut the sentence one letter short
			// of its end while the ◉ row four cells to its right stood
			// whole with a hundred blank cells after it. The ask is the
			// trail's own row, as #265 sent it down to the card's and
			// #269 and #273 to the ship row's and the header's. An
			// unselected row has no trail beside it and keeps its ask,
			// which is what tells it from its neighbour (#196, #266).
			if m.askBeside(s) {
				head = sessionName(s.Info)
			} else {
				head = sessionName(s.Info) + " · " + askQuote(head, askRelayed(s))
			}
		}
	}
	if m.isCircling(s) && !m.archiveView {
		// "looping", not "circling": the person reading it asked what
		// circling meant, and a loop is the word a developer has for an
		// agent going round the same failure (#48). The verdict row says
		// which failure and how many times.
		head = "looping"
		// The loop, and whether a turn is in flight — §2.4 asks the row
		// for both. The floor is the name's own width, not the column's:
		// a column wide enough for a long name dropped the clause for a
		// short one, and "looping 3h" beside "looping 20h" says nothing
		// about which is a wait and which is a loop.
		if word := circlingState(st); word != "" {
			full := "looping · " + word
			if w-5-1-ageWidth-lipgloss.Width(full)-1 >= 6 { // the name's own floor, below
				head = full
			}
		}
		// The age beside the word is the state's own, here as everywhere:
		// hoisting the loop's age where "· idle" would not fit put
		// "looping  18h" on a card whose every other view said
		// "looping · idle  3h", and the loop's age is the chip's and
		// the verdict's to give ("↻1 18h", "4th failure").
	}

	// Everything between the state glyph and the age. The name takes it, less
	// whatever the state word needs — and the state word is held against the
	// age, so the words that matter line up on their right edge however long
	// the names beside them are. Freeing this width is the point of not
	// spelling "working": a name is what you are looking for, and "agentic_…"
	// was costing ten columns to say "● " a second time.
	avail := w - 5 - 1 - ageWidth
	headW := lipgloss.Width(head)
	if headW > 0 {
		headW++ // a column of air before it
	}
	nameW := avail - headW
	if nameW < 6 {
		nameW, headW = 6, max(avail-6, 0)
	}

	midStyle := dimStyle
	if selected {
		midStyle = textStyle
	}
	body := nameStyle.Render(pad(clip(sessionName(s.Info), nameW), nameW)) +
		midStyle.Render(padLeft(clip(head, headW), headW))
	if m.archiveView && !m.askBelow {
		// No name: the whole line is the prompt the session was given.
		body = midStyle.Render(pad(clip(head, avail), avail))
	}

	glyph := fleet.Glyph(st)
	if m.isCircling(s) {
		glyph, accent = glyphCircling, stuckStyle
	}
	if s.Snap.APIError && !(m.archiveView && !s.Live) {
		glyph = glyphAPIError
	}
	first := marker + indexStyled + " " + accent.Render(glyph) + " " +
		body + " " + dimStyle.Render(age)

	lines := []string{first, strings.Repeat(" ", 4) + m.secondLineUnder(s, w-4, tag)}
	if strings.TrimSpace(ansi.Strip(lines[1])) == "" {
		lines = lines[:1] // the trail beside says the present; the trace moves up (#111)
	}
	if !m.boardShown() && !m.archiveView && s.Live {
		// Below the board's width the row is the column: the digest and
		// the sent-trace ride under it, or a reply at eighty columns left
		// no trace at all — and the pane, when a namesake shares the tmux
		// session the header names.
		// The pane when a namesake shares the tmux session, and the tool
		// where the fleet runs two (#50): the ladder's longest form that
		// leaves the digest its floor, the model going first, then the
		// pane's session name (the group header says it two rows up).
		pane := ""
		if m.sharesTmux(s) {
			if p, ok := m.panes[s.Info.Key()]; ok {
				pane = mirrorMark + " " + p.Target
			}
		}
		tag := ""
		tool, word := m.toolTag(s), ""
		if tool != "" {
			word = strings.SplitN(tool, " · ", 2)[0]
			if word == shortModel(s.Info.Model) {
				word = ""
			}
		}
		short := ""
		if pane != "" {
			short = mirrorMark + " " + paneSuffix(strings.TrimPrefix(pane, mirrorMark+" "))
		}
		// The group header two rows up names the tmux session, so every
		// row's pane is its suffix — "⌁ :1.0" — one form for the group,
		// not a full address on one row and a suffix on the next (#62);
		// `enter` attaches by the row's own pane whatever the row shows.
		// The word before the pane: the header said the session.
		var ladder []string
		seen := map[string]bool{}
		for _, c := range []string{joinTag(tool, short), joinTag(word, short), word, short} {
			if c != "" && !seen[c] {
				ladder, seen[c] = append(ladder, c), true
			}
		}
		delta := func(room int) string {
			if room < digestFloor {
				return ""
			}
			d := m.boardDelta(s.Info.Key(), s, room)
			// The trail beside this row draws the divider the count
			// counts from (#117, #129 on the board): the row is the
			// board's column here, so the count yields its cells to the
			// ladder — the model, which is nowhere else on the row. The
			// clause goes whether or not it is alone (#142's shape): the
			// row keeps every clause the divider does not draw.
			return m.countLessBeside(s, d)
		}
		tag = tagBesideDigest(ladder, w-4, delta)
		if tag != "" && s.Info.Key() == m.selectedKey && m.headerDrawsTag(tag) {
			// The identity header of this very frame names this very
			// session: where it draws every clause of the tag, the row
			// sheds it and spends the cells on what the frame does not
			// say — the trace's clock here (#90's device, #167's split
			// between header and row, #196 and #197 on the archive's
			// halves). An unselected row keeps its word, which is what
			// tells it from its neighbour where the fleet runs two (#80).
			tag = ""
		}
		if tag != "" && !strings.Contains(tag, mirrorMark) && m.liveCount() == 1 && strings.Contains(m.headerLine(m.width), " · "+s.Info.ToolName()) {
			// A fleet of one: the header names the tool on this very
			// frame, so a bare tool word on the row says a thing the frame
			// already says — where it cost the trace its clock (#90), where
			// there was no trace and the word was a whole row (#93), and
			// where it cost nothing: the reason was never the cost, and a
			// row that ended in `claude` over two OpenCode rows that drew
			// no word was #80's shape still (#96).
			tag = ""
		}
		room := w - 4
		if tag != "" {
			room -= lipgloss.Width(tag) + 2
		}
		third := delta(room)
		if tag != "" {
			third = pad(third, w-4-lipgloss.Width(tag)) + dimStyle.Render(tag)
		}
		if strings.TrimSpace(ansi.Strip(third)) != "" {
			lines = append(lines, strings.Repeat(" ", 4)+third)
		}
	}
	return lines
}

// countBeside says whether the trail drawn beside this row already draws
// the divider its digest counts from: the fleet list is the board's column
// below the board's width, and #117's reason — the count is the divider's
// own words — is the same one panel over. The match is strict (#135) and
// the look clause the divider carries comes off first (#136). Not under
// the reply box, which covers the trail's rows (#108).
func (m *Model) countBeside(s fleet.Session, digest string) bool {
	return strings.TrimSpace(ansi.Strip(digest)) != "" && m.countLessBeside(s, digest) == ""
}

// countLessBeside is this row's digest less the clauses the trail beside it
// draws with its own read-line — the look (#136) and the count (#129) —
// and the digest whole where no such rule stands beside it.
func (m *Model) countLessBeside(s fleet.Session, digest string) string {
	if m.replyBox.on || m.archiveView || s.Info.Key() != m.selectedKey {
		return digest
	}
	divider := ""
	for _, r := range m.trailRows {
		if p := oneSpace(ansi.Strip(r)); strings.Contains(p, "you were here") {
			divider = p
			break
		}
	}
	if divider == "" {
		return digest
	}
	d := strings.TrimSpace(ansi.Strip(digest))
	cut := false
	if lk := lookRe.FindStringSubmatch(d); lk != nil && strings.Contains(divider, "you were here · "+lk[1]+" ago") {
		d, cut = strings.TrimSuffix(d, lk[0]), true
	}
	for i, c := range strings.Split(d, " · ") {
		if !newLegsClause.MatchString(c) {
			continue
		}
		parts := strings.Split(d, " · ")
		rest := append(append([]string{}, parts[:i]...), parts[i+1:]...)
		if strings.HasPrefix(c, "↳ ") && i < len(rest) && !strings.HasPrefix(rest[i], "↳ ") {
			rest[i] = "↳ " + rest[i]
		}
		d, cut = strings.Join(rest, " · "), true
		break
	}
	if !cut {
		return digest
	}
	if d == "" {
		return ""
	}
	return dimStyle.Render(d)
}

// verdictRe is the newest verdict a row rides beside the present (#125).
var verdictRe = regexp.MustCompile(` [0-9]+✓( [0-9]+✗)? · `)

// digestFloor is the least room the row's digest takes beside a tag: the
// width of its shortest clause, `↳ 1 new leg`. A floor of twelve refused
// the pane rung to a row where it fit in exactly the row's cells (#118).
const digestFloor = 11

// presentBeside says whether the row's present line is the trail's own HEAD
// row drawn beside it: a fleet of one at eighty and a hundred stands the
// trail beside the list, and the row's present said the sentence twice on
// one physical row, once each side of the rule (#111, #60's shape). The
// trail's is the survivor and the trace beneath moves up. The compare is
// against the rows the frame draws (#108, #207): the box says the sentence
// under its own head, and past its bottom edge the trail's own rows still
// stand — a box thirteen rows tall over a twenty-five-row body leaves the
// HEAD row on the frame, and the row beside it then said it twice.
func (m *Model) presentBeside(s fleet.Session, line string) bool {
	// Any state, not only the working ones: an idle row whose verdict is
	// empty falls back to its last leg's label, and that is the trail's
	// own row beside it. The compare decides, not the state (#111, #114).
	if !(!m.boardFits() && !m.archiveView && s.Live && s.Info.Key() == m.selectedKey) {
		return false
	}
	sentence := oneSpace(strings.TrimRight(ansi.Strip(line), " "))
	// The newest verdict the row splices in front of the present — "● fix
	// tokens.py 18✓ 2✗ · for 22m" — is on the trail's own test row; the
	// compare runs on the sentence without it too (#125).
	bare := verdictRe.ReplaceAllString(sentence, " ")
	if m.replyBox.on {
		// The box covers the trail's row (#108), but it says the sentence
		// itself under its own head: the row leaves it to the box.
		var said []string
		for _, r := range m.replyRows {
			said = append(said, oneSpace(strings.Trim(ansi.Strip(r), "│┌┐└┘─ ")))
		}
		box := strings.Join(said, " ")
		if saysSame(sentence, box) || saysSame(bare, box) {
			return true
		}
	}
	for i, r := range m.trailRows {
		if m.boxCoversRow(i) {
			// The box stands on this trail row: what it says is under
			// the box and the row beside it is the only copy (#108).
			continue
		}
		// The trail's cursor stands between the glyph and the class on
		// HEAD's own row at Lv2 — "●▸test" — and the mark is not part of
		// the row's sentence (#104, #127).
		row := oneSpace(strings.Replace(strings.TrimRight(ansi.Strip(r), " "), "▸", " ", 1))
		if saysSame(sentence, row) || saysSame(bare, row) ||
			saysSame(sentence, wrappedLabel(m.trailRows, i, len(m.trailRows))) {
			return true
		}
	}
	return false
}

// boxCoversRow says whether the reply box stands on the trail column's row
// i. #111's compare is against the rows the frame draws (#108, #207): a
// trail row the box leaves standing is on the frame, and the row beside it
// saying the same sentence says it twice.
func (m *Model) boxCoversRow(i int) bool {
	return m.replyBox.on && i >= m.replyBox.top && i < m.replyBox.top+m.replyBox.h
}

// presentBesideRow answers the same question for entryLines, which has the
// row's width but not the line.
func (m *Model) presentBesideRow(s fleet.Session, w int) bool {
	return m.presentBeside(s, m.journeyLine(s, w))
}

// askBeside says whether the trail drawn beside this row draws the row's own
// ask on its ◉ row. The archive's list stands beside the selected session's
// trail, and the row's head — the name and the ask — then says that sentence
// twice on one physical row, once each side of the rule: #111's rule for the
// present line, on the archive's half of the same list. The compare is the
// card's (#107): whole, or one clipped where the other is whole, two words
// the floor (#110), and a row the reply box covers is not on the frame
// (#108).
func (m *Model) askBeside(s fleet.Session) bool {
	if !m.archiveView || s.Info.Key() != m.selectedKey || len(m.trailRows) == 0 {
		return false
	}
	ask := archiveHeadline(s)
	for i, r := range m.trailRows {
		if m.boxCoversRow(i) {
			continue
		}
		// The trail's cursor stands between the glyph and the sentence on
		// its own row at Lv2 — `◉▸"fix the 401 on token refresh"` — and
		// the mark is not part of what the row says (#104, #127).
		if sameAsk(ask, saidAsk(strings.Replace(ansi.Strip(r), "▸", " ", 1))) {
			return true
		}
	}
	return false
}

// secondLine is what the session is actually doing, in the trail's own words:
// the class of work and the tool call it is in. The two panels then describe a
// session the same way, and the class is the same one Lv1 would draw.
//
// It used to be the tmux address and the git branch — ":0.0 claude · HEAD" —
// which answered a question nobody was asking. The address is what `enter`
// spends, not what a reader spends; the selected session's is in the mirror's
// own header, and `compass panes` has them all. A branch reading "HEAD" three
// times in a fleet says less than nothing.
func (m *Model) secondLine(s fleet.Session, w int) string {
	return m.secondLineUnder(s, w, "")
}

// secondLineUnder is secondLine under the tag the card draws two rows down.
func (m *Model) secondLineUnder(s fleet.Session, w int, tag string) string {
	tagged := strings.Contains(tag, mirrorMark)
	if m.archiveView {
		// A hidden live session: where it lives, and the tool where the
		// fleet runs two — that is how its namesake on the board is told
		// from it (#53). The word alone: the model is the card's.
		word := ""
		if tool := m.toolTag(s); tool != "" && strings.SplitN(tool, " · ", 2)[0] != shortModel(s.Info.Model) {
			word = " · " + strings.SplitN(tool, " · ", 2)[0]
		}
		// The tag row two rows down draws this very word. #53 put the
		// word on the row so a hidden session is told from its namesake
		// on the board; on the board the card's own tag row says it, and
		// the archive's card drew `claude · main` over
		// `claude · opus-4-1 · ⌁ dev:1.0`, and where the archived row has
		// no model and no pane a whole row that was the word and nothing
		// else. The row yields the word to the tag exactly as it yields
		// the address (#64: a line answers a question once), and keeps
		// the branch and the verdict, which the tag cannot say.
		for _, c := range strings.Split(tag, " · ") {
			if word != "" && c == strings.TrimPrefix(word, " · ") {
				word = ""
				break
			}
		}
		if pane, ok := m.panes[s.Info.Key()]; ok && s.Live && pane.Target != "" {
			addr := mirrorMark + " " + pane.Target
			if model := shortModel(s.Info.Model); model != "" && word != "" && s.Info.Key() == m.selectedKey && !strings.Contains(m.headerLine(m.width), model) {
				// The header of this very frame carries the pane and has
				// shed the model (#62): the row draws the fact the frame
				// has nowhere else (#91).
				if cand := strings.TrimPrefix(word+" · "+model, " · ") + " · " + branchOf(s.Info); lipgloss.Width(cand) <= w {
					addr = model
				}
			}
			if tagged && addr == mirrorMark+" "+pane.Target {
				// The tag row two rows down draws this very address:
				// the row keeps the branch, which the tag cannot say.
				return dimStyle.Render(clip(strings.TrimPrefix(word+" · "+branchOf(s.Info), " · "), w))
			}
			if s.Info.Key() == m.selectedKey {
				// The identity header two rows up names this very session:
				// where it says the tool word or the address the row sheds
				// it (#167's and #189's device), and keeps the branch, the
				// one word the frame does not otherwise say — `main`, not
				// `opencode · ⌁ dev:2.0 · main` under a header saying
				// `opencode · sonnet-4-5 · ⌁ dev:2.0` (#196).
				head := ansi.Strip(m.headerLine(m.width))
				line := ""
				for _, c := range []string{strings.TrimPrefix(word, " · "), addr} {
					if c != "" && !strings.Contains(head, c) {
						line += " · " + c
					}
				}
				return dimStyle.Render(clip(strings.TrimPrefix(line+" · "+branchOf(s.Info), " · "), w))
			}
			return dimStyle.Render(clip(strings.TrimPrefix(word+" · "+addr, " · ")+" · "+branchOf(s.Info), w))
		}
		if s.Live {
			return dimStyle.Render(clip(strings.TrimPrefix(word+" · no pane", " · ")+" · "+branchOf(s.Info), w))
		}
		// An archived row: the tool where the archive holds two, then the
		// branch — "opencode · spike/billing" (#80) — and the verdict the
		// band draws, which is what answers "do I want this one back"
		// (#47): eleven of twelve archived rows said nothing about
		// whether the day went green or red (#103). It goes last, so a
		// narrow archive sheds it first. On the selected row the identity
		// header of the same frame names this very session's tool, so the
		// word goes where the header says it and the branch and the
		// verdict take the cells — `chore/deps · ✓ shipped`, not
		// `claude · chor… · ✓ shipped` under a header ending `· claude`.
		// #196's split between header and row, on the archived half of
		// the same list; an unselected row keeps its word, which is what
		// tells it from its neighbour where the archive holds two (#80).
		if word != "" && s.Info.Key() == m.selectedKey &&
			strings.Contains(ansi.Strip(m.headerLine(m.width)), strings.TrimPrefix(word, " · ")) {
			word = ""
		}
		line := strings.TrimPrefix(word+" · "+branchOf(s.Info), " · ")
		if v := m.verdictClause(s); v != "" {
			// The band's own ladder, not all or nothing: the counts go
			// first, then the clause's two words, last the mark alone —
			// so a narrow archive says green or red where it cannot say
			// how green (#47's clause, #103's shed order, recentKeep).
			mark := ""
			if rs := []rune(v); len(rs) > 0 && strings.ContainsRune("✓✗⚑", rs[0]) {
				mark = string(rs[0])
			}
			fitted := false
			for _, form := range []string{v, firstWords(v, 2)} {
				if form == "" {
					continue
				}
				if cand := line + " · " + form; lipgloss.Width(cand) <= w {
					line = cand
					fitted = true
					break
				}
			}
			// A word the mark does not already say comes before the
			// branch's tail: "✗ red" and "✓ green" are the mark in
			// letters, but "✓" alone does not say the day shipped, and
			// the band a keypress up says "✓ shipped" of that very
			// session at that very width. The branch yields the cells as
			// it yields them for the mark (#145, #47). Three cells of
			// branch stand, not four: on the 26-cell archive column at
			// 120 the fourth was the two cells `opencode` costs over
			// `claude`, and the row that shipped wore the bare tick its
			// green neighbours wear (#172).
			two := firstWords(v, 2)
			if !fitted && mark != "" && two != mark && two != mark+" red" && two != mark+" green" {
				keep := w - lipgloss.Width(" · "+two)
				if floor := lipgloss.Width(line) - lipgloss.Width(branchOf(s.Info)) + 3; keep >= floor {
					line = clip(line, keep) + " · " + two
					fitted = true
				}
			}
			if !fitted && mark != "" {
				if cand := line + " · " + mark; lipgloss.Width(cand) <= w {
					line = cand
					fitted = true
				}
			}
			// And where not even the mark fits behind the whole branch,
			// the branch yields the cells: a clipped branch still names
			// where the day happened, a missing mark names nothing, and
			// green-or-red is the clause the archive row exists for
			// (#47). The band at the same width already says it.
			if !fitted && mark != "" {
				if room := w - lipgloss.Width(" · "+mark); room > 0 {
					line = clip(line, room) + " · " + mark
				}
			}
		}
		return dimStyle.Render(clip(line, w))
	}
	// Result before process. "1216✓ 2✗" answers whether the session is going
	// well; "Bash: pytest tests/auth -x" only answers whether it is busy, which
	// the glyph beside the name already said. The call in flight is the
	// fallback for a session that has not finished anything yet — and for a
	// quiet one, the prompt it was given, because "idle" is what the first line
	// already told you.
	act := s.Outcome
	// The journey's own words, when the board holds its trail: what it is
	// doing this minute, or how it came out. The fleet row, the column
	// header and the trail then describe one session one way — three
	// second lines that disagreed ("✓ green 215✓" here, "Edit: loader.py"
	// there, "wiring the filter · for 1h" below) were the first thing the
	// second review read.
	if line := m.journeyLine(s, w); line != "" {
		if m.presentBeside(s, line) {
			return ""
		}
		return line
	}
	// Unless the session is one you must not miss. Then the sentence that
	// explains it owns the line: "waiting on your answer · open 22 to the
	// office CIDR?", "no output for 4m mid-turn · Bash: python backfill.py
	// --all", "api error 403 · your daily quota is exhausted". The state
	// machine had every one of these and the row was showing "1216✓" over
	// them — work that finished before the wall went up.
	if wantsAttention(s.Snap.State) || s.Snap.APIError {
		act = s.Snap.Reason
		if txt := strings.TrimSpace(s.Snap.Activity); txt != "" && txt != "idle" {
			act += " · " + txt
			// A question is the whole line. The first line already said
			// "needs you", and "waiting on your answer" only repeats it;
			// "open 22 to the office CIDR?" is what the person has to read,
			// and a narrow column must not clip it for the repeat.
			if s.Snap.State == state.NeedsYou {
				act = txt
			}
		}
	}
	if strings.TrimSpace(act) == "" {
		act = s.Snap.Activity
	}
	if strings.TrimSpace(act) == "" || act == "idle" {
		act = s.Info.Title
		if s.Info.Relayed && act != "" {
			act = "relayed " + act // another session's ask, worn as such (#97)
		}
	}
	// The reason is worth a line when the state is one you must not miss —
	// "waiting on your answer", "api error 403". For working and idle it is
	// "turn complete" and "no activity yet": the glyph already said that.
	if strings.TrimSpace(act) == "" && wantsAttention(s.Snap.State) {
		act = s.Snap.Reason
	}
	if !s.HasClass || wantsAttention(s.Snap.State) || s.Snap.APIError {
		// Nothing classifiable yet — a session that has only been asked for, or
		// one woken from the archive whose replay has not reached a tool call.
		// Or a sentence the reader must not miss: the question, the hung
		// call, the error. It gets the width; the class is in the trail.
		if i := strings.Index(act, " ["); i > 0 && lipgloss.Width(act) > w {
			act = act[:i] // the options go whole: "[office CIDR / keep basti…" named no option
		}
		if line := clip(act, w); m.presentBeside(s, line) {
			return ""
		}
		return dimStyle.Render(clip(act, w))
	}
	class := s.Class.String()
	head := classStyle(s.Class).Render(glyphLeg + " " + pad(class, trailVerbWidth))
	rest := w - 2 - trailVerbWidth - 1
	if rest < 4 {
		return head
	}
	return head + " " + dimStyle.Render(clip(withoutClassVerb(act, class), rest))
}

// fleetLabelKeep is the label a working row keeps before it gives up its
// verdict for room.
const fleetLabelKeep = 20

// journeyLine is a live session's second line read off its trail: for a
// working session its HEAD — the class, what it is doing, how long, and the
// agents it has out — and for a quiet one the verdict. "" when the trail is
// not in hand, or the state owes the reader a sentence instead.
func (m *Model) journeyLine(s fleet.Session, w int) string {
	if m.archiveView || !s.Live || s.Snap.State == state.NeedsYou || s.Snap.APIError {
		return ""
	}
	tr, ok := m.trails[s.Info.Key()]
	if !ok {
		return ""
	}
	if m.isCircling(s) {
		// A loop's row says the loop: the count that makes it one was
		// off the fleet at every width without a board.
		if parts := verdictPartsWith(tr, m.now, s.Snap.State != state.Idle, m.agentsFor(s.Info.Key())); len(parts) > 0 {
			return dimStyle.Render(joinFit(parts, w))
		}
	}
	if s.Snap.State == state.Working || s.Snap.State == state.Stuck {
		head := -1
		for i := len(tr.Legs) - 1; i >= 0; i-- {
			if tr.Legs[i].Current {
				head = i
				break
			}
		}
		if head < 0 {
			// No leg yet: the present in the same words the trail's bare
			// HEAD row uses, not the completed-leg glyph over a placeholder.
			class := "scout"
			if s.HasClass {
				class = s.Class.String()
			}
			return bareHeadRow(TrailOpts{Head: m.headFor(s), HeadState: s.Snap.State, HeadClass: class,
				HeadActivity: s.Snap.Activity, HeadSince: headSince(s), HeadAllowed: s.Snap.Allowed, Now: m.now}, w)
		}
		l := tr.Legs[head]
		// HEAD's own row, in HEAD's own words: the same glyph, label and
		// figure the trail draws, so zooming in never changes the sentence.
		o := TrailOpts{Todos: planItems(tr.Tasks), Head: m.headFor(s), HeadState: s.Snap.State,
			HeadSince: headSince(s), HeadAllowed: s.Snap.Allowed, HeadWaits: headWaits(tr), HeadTail: headTail(tr, m.now, true, m.agentsFor(s.Info.Key())), Agents: m.agentsFor(s.Info.Key()), Now: m.now, Width: 1000}
		label, _ := legLabel(l, o)
		glyph, tail := headMark(o, l)
		// The newest verdict rides beside the present: below 110 columns
		// there is no board, and a red suite was a board-only fact.
		verdict := ""
		for i := len(tr.Legs) - 1; i >= 0; i-- {
			if badge := legBadge(tr.Legs[i]); badge != "" && badge != "?" {
				verdict = badge
				break
			}
		}
		// Narrow rows lose the verdict first, then clip the label — the
		// agents and the wait are what the row is for.
		full := tail
		if verdict != "" {
			full = verdict + " · " + tail
		}
		labelW := w - 2 - trailVerbWidth - 1 - 1 - len([]rune(full))
		if labelW < len([]rune(label)) {
			// The verdict goes before the label is cut at all: "Wiring the
			// filter into the l… 215✓" kept the result and lost the subject.
			full = tail
			labelW = w - 2 - trailVerbWidth - 1 - 1 - len([]rune(full))
		}
		lead := classStyle(l.Class).Render(glyph + " " + pad(l.Class.String(), trailVerbWidth))
		if labelW < 4 {
			// The narrowest row keeps the sentence and loses the label:
			// "◆ build  20✓" on a session parked on three agents was the
			// one row, at the one width with no board, that said nothing.
			// And when even that does not fit, the class word goes too.
			if len([]rune(full)) > w-2-trailVerbWidth-1 {
				return classStyle(l.Class).Render(glyph) + " " + dimStyle.Render(clip(full, w-2))
			}
			return lead + " " + dimStyle.Render(clip(full, w-2-trailVerbWidth-1))
		}
		return lead + " " + dimStyle.Render(pad(clip(label, labelW), labelW)) + " " + dimStyle.Render(full)
	}
	if v := boardVerdictWith(s, tr, m.now, m.agentsFor(s.Info.Key())); v != "" {
		return dimStyle.Render(joinFit(strings.Split(v, " · "), w))
	}
	return ""
}

// joinFit joins clauses with " · " as far as they fit the width, dropping
// the trailing ones whole: "✓ green 12…" told the reader less than
// "✓ shipped 56m ago" alone.
func joinFit(parts []string, w int) string {
	out := ""
	for i, p := range parts {
		next := p
		if i > 0 {
			next = out + " · " + p
		}
		if len([]rune(next)) > w {
			if i == 0 {
				return clip(p, w)
			}
			return out
		}
		out = next
	}
	return out
}

// headFor is what HEAD is called when the state machine knows: the call a
// stuck session is hung on, the question a waiting one is asking. "" for
// the rest — the plan and the leg's own label do.
func (m *Model) headFor(s fleet.Session) string {
	switch {
	case s.Snap.State == state.Stuck || s.Snap.State == state.NeedsYou,
		s.Snap.State == state.Working && s.Snap.Allowed > 0:
		// The hung call, the question — and the call a budget is for:
		// "for 4m of 10m" over a leg's label named the wait and not
		// what was waited on (#54).
		if txt := strings.TrimSpace(s.Snap.Activity); txt != "" && txt != "idle" {
			return txt
		}
	}
	return ""
}

// agentsOut counts the subagent lanes still open.
func agentsOut(tr journey.Trail) int {
	n := 0
	for _, b := range tr.Branches {
		if !b.Done {
			n++
		}
	}
	return n
}

// unread is an idle session that finished within the day and has not been
// opened since — the board's brightness, as a fact the fleet can spell.
func (m *Model) unread(s fleet.Session) bool {
	if m.archiveView || !s.Live || s.Snap.State != state.Idle {
		return false
	}
	last := s.Info.LastEventAt
	if last.IsZero() || m.now.Sub(last) > boardFresh {
		return false
	}
	seen, ok := m.seen[s.Info.Key()]
	return !ok || seen.Before(last)
}

// laneLinks pairs a trail's lanes with live sessions that look like them:
// a session whose first prompt begins with the lane's label. The transcript
// carries no real link between a lead and a teammate, so the mark is a
// hedge — "→3" — and `3` is the key that goes and looks.
func (m *Model) laneLinks(tr journey.Trail, agents map[string]agentLive) map[string]int {
	links := map[string]int{}
	for label, l := range m.laneMatches(tr, agents) {
		links[label] = l.num
	}
	return links
}

// laneLinkWrote is when each linked session last wrote, by the lane's
// label: the fresher reading's clock, for the lane's own sub-row (#68).
func (m *Model) laneLinkWrote(tr journey.Trail, agents map[string]agentLive) map[string]time.Time {
	wrote := map[string]time.Time{}
	for label, l := range m.laneMatches(tr, agents) {
		wrote[label] = l.wrote
	}
	return wrote
}

// laneLink is a lane's matched session: its board digit and its last event.
type laneLink struct {
	num   int
	wrote time.Time
}

func (m *Model) laneMatches(tr journey.Trail, agents map[string]agentLive) map[string]laneLink {
	if len(tr.Branches) == 0 {
		return nil
	}
	rows := m.boardRows()
	links := map[string]laneLink{}
	for _, b := range tr.Branches {
		label := strings.ToLower(strings.TrimSpace(b.Label))
		if len([]rune(label)) < 12 {
			continue
		}
		for _, s := range m.sessions {
			if !s.Live {
				continue
			}
			other, ok := m.trails[s.Info.Key()]
			if !ok || len(other.Prompts) == 0 {
				continue
			}
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(other.Prompts[0].Text)), label) {
				// A hedge where nothing better was read (#49) — and where
				// the lane's own file is in hand but the session it
				// matches wrote more recently: the file is then the
				// staler of two readings of one agent, and the board
				// called it silent while it worked two columns right (#67).
				if a, known := agents[b.ToolUseID]; known && !s.Info.LastEventAt.After(a.Wrote) {
					break
				}
				if r, ok := rows[s.Info.Key()]; ok && r.num > 0 {
					links[b.Label] = laneLink{num: r.num, wrote: s.Info.LastEventAt}
				}
				break
			}
		}
	}
	return links
}

// wantsAttention is the pair of states that float to the top of their group.
func wantsAttention(s state.State) bool {
	return s == state.NeedsYou || s == state.Stuck
}

// headline is the one thing worth saying about a session on its own line. The
// list answers "who wants me"; the card next to it answers "why".
// headline is the state, spelled — but only when spelling it adds something.
// The glyph already carries the state in shape and colour, so "● api working"
// says "working" twice and spends ten columns doing it. `needs you` and
// `stuck` earn their words: they are the two a reader must not miss, and the
// space is better spent on them than on the two that are simply fine.
func headline(s fleet.Session) string {
	if s.Snap.APIError {
		return apiWord(s)
	}
	switch s.Snap.State {
	case state.NeedsYou, state.Stuck:
		return stateLabel(s.Snap.State)
	}
	return ""
}

// apiWord is the state word of a session dead on the API: "quota" when
// that is what the refusal says, "api error" otherwise. It is the word that
// survives every truncation, so the row never reads as a question.
func apiWord(s fleet.Session) string {
	text := strings.ToLower(s.Snap.Activity + " " + s.Snap.Reason)
	if strings.Contains(text, "quota") || strings.Contains(text, "rate limit") {
		return "quota"
	}
	return "api error"
}

// circlingState is the word a circling row adds for what the session is
// doing under the loop: waiting for you, or in a turn. A state the loop
// does not qualify (a question, a hang, a dead API) says itself and adds
// nothing here.
func circlingState(st state.State) string {
	switch st {
	case state.Idle:
		return "idle"
	case state.Working:
		return "working"
	}
	return ""
}

// archiveHeadline is what an archived session is remembered for: what you asked
// it to do. A session that never got a prompt falls back to its last verdict.
func archiveHeadline(s fleet.Session) string {
	if s.Info.Title != "" {
		return s.Info.Title
	}
	return headline(s)
}

// stateLabel is the human spelling of a state; State.String() stays the
// machine-readable one. It is also the fleet row's headline: the state is what
// the first line answers, and what the session is *doing* is the second line's
// job (in the trail's own class vocabulary), so neither repeats the other.
func stateLabel(s state.State) string {
	if s == state.NeedsYou {
		return "needs you"
	}
	return s.String()
}

// location is the dim second line: where the session lives, then what it is
// working on. The group header already names the tmux session, so the line
// drops the prefix and says ":1.0" — window and pane, the rest of the address
// (SPEC §2.5). A session with no pane says so plainly.
func (m *Model) location(info fleet.SessionInfo) string {
	where := "no pane"
	if pane, ok := m.panes[info.Key()]; ok && pane.Target != "" {
		where = paneSuffix(pane.Target)
		// The window's name is what the user actually navigates by — they named
		// it "porter-test", not ":4.0" — so it sits right after the coordinates
		// and ahead of the branch, which is the part that may be clipped away.
		if pane.Window != "" {
			where += " " + pane.Window
		}
	}
	if b := branchOf(info); b != "" {
		return where + " · " + b
	}
	return where
}

// branchOf is the branch a session is on, or the project it lives in when git
// has nothing to say.
func branchOf(info fleet.SessionInfo) string {
	if info.GitBranch != "" {
		return info.GitBranch
	}
	return strings.TrimPrefix(info.ProjectSlug, "-")
}

// tmuxSessionName is the group a pane belongs to. tmux forbids ":" in a session
// name, but the split is on the LAST one anyway: the coordinates are the part
// we are certain about.
func tmuxSessionName(target string) string {
	i := strings.LastIndex(target, ":")
	if i < 0 {
		return target
	}
	return target[:i]
}

// paneSuffix is ":window.pane" — the target with its session prefix removed.
func paneSuffix(target string) string {
	i := strings.LastIndex(target, ":")
	if i < 0 {
		return target
	}
	return target[i:]
}

// paneCoords is a session's window and pane index, numerically — window 10
// sorts after window 9, which a string compare would get wrong. A session with
// no pane sorts last, which leaves the `elsewhere` group in fleet order.
func (m *Model) paneCoords(i int) (window, pane int) {
	p, ok := m.panes[m.sessions[i].Info.Key()]
	if !ok || p.Target == "" {
		return math.MaxInt32, math.MaxInt32
	}
	coords := strings.TrimPrefix(paneSuffix(p.Target), ":")
	win, pn, found := strings.Cut(coords, ".")
	w, err := strconv.Atoi(win)
	if err != nil {
		return math.MaxInt32, math.MaxInt32
	}
	if !found {
		return w, 0
	}
	n, err := strconv.Atoi(pn)
	if err != nil {
		return w, 0
	}
	return w, n
}

// sessionName calls a session by the last segment of its working directory —
// the way its human thinks of it.
func sessionName(info fleet.SessionInfo) string {
	if n := strings.TrimSpace(info.Name); n != "" {
		return n // the name the person gave it (/rename) over the directory's (#79)
	}
	if info.CWD != "" {
		if base := filepath.Base(info.CWD); base != "." && base != string(filepath.Separator) {
			return base
		}
	}
	if p := fleet.SlugPath(info.ProjectSlug); p != "" {
		if base := filepath.Base(p); base != "." && base != string(filepath.Separator) {
			return base
		}
	}
	if len(info.ID) > 8 {
		return info.ID[:8]
	}
	return info.ID
}

// projectOf is the directory a session was started in, whatever its person
// has since called it. The archive's bucket is the project (#11: "I start
// sessions in the respective directory"), and a `/rename` names the row,
// the header and the band (#79) — not the bucket. Grouping by the name
// gave the one renamed session a heading of its own and drew its project
// twice, ten rows apart, with six other projects between the two halves of
// one directory's history.
func projectOf(info fleet.SessionInfo) string {
	bare := info
	bare.Name = ""
	return sessionName(bare)
}

// compactOverlap is an overlap line with its prose dropped: "⚠ webapp and
// api both touched tokens.py in the last 20m" → "⚠ webapp, api · tokens.py
// · 20m" — the fact survives a narrow column, the sentence does not.
func compactOverlap(line string) string {
	body := strings.TrimSpace(strings.TrimPrefix(line, "⚠"))
	if i := strings.Index(body, " both touched "); i >= 0 {
		names := strings.Replace(body[:i], " and ", ", ", 1)
		rest := body[i+len(" both touched "):]
		if j := strings.Index(rest, " in the last "); j >= 0 {
			// The fact leads, the names follow: a narrow column sheds the
			// names whole and keeps the file.
			return "⚠ " + rest[:j] + " · " + names + " · " + rest[j+len(" in the last "):]
		}
		return "⚠ " + rest + " · " + names
	}
	if i := strings.Index(body, " are both failing "); i >= 0 {
		return "⚠ " + body[i+len(" are both failing "):] + " · " + strings.Replace(body[:i], " and ", ", ", 1)
	}
	return line
}
