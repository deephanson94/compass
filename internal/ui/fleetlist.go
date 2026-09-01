package ui

import (
	"fmt"
	"github.com/charmbracelet/lipgloss"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/deephanson94/compass/internal/fleet"
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
	rows := []string{m.titleMark(panelFleet) + m.titleStyleFor(panelFleet).Render(clip(title, w-1)), ""}
	if h > 2 {
		rows = append(rows, m.fleetLines(w, h-2)...)
	}
	return rows
}

// fleetLines renders the fleet: grouped the way the user thinks of it, scrolled
// so the selection is always whole on screen, and — in the live view — closed
// by the dim line that says how much history is one keypress away.
func (m *Model) fleetLines(w, h int) []string {
	// The archive count is not part of the list: it is the column's last word,
	// so it survives any amount of scrolling — and it is how the archive is
	// discovered, so it stands even when nothing at all is live.
	var tail []string
	if !m.archiveView {
		if n := m.archivedCount(); n > 0 {
			tail = []string{"", dimStyle.Render(clip(fmt.Sprintf("%d archived · A browses", n), w))}
		}
	}

	rows := m.fleetRows()
	if len(rows) == 0 {
		note := "nothing live"
		if m.archiveView {
			note = "no archived sessions"
		}
		return append([]string{dimStyle.Render(clip(note, w))}, tail...)
	}

	body := h - len(tail)
	if body < 1 {
		body = 1
	}
	lines, selStart, selEnd := m.fleetBlock(rows, w)
	return append(m.scrollFleet(lines, selStart, selEnd, body), tail...)
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
		if m.sessions[r.sess].Info.Key() == m.selectedKey && selStart < 0 {
			selStart = len(lines)
			selEnd = selStart + entryLines - 1
		}
		lines = append(lines, m.entryLines(r, w)...)
	}
	return lines, selStart, selEnd
}

// scrollFleet windows the column onto h lines, following the selection: the
// offset moves as little as it can, so headers travel with their groups and
// the list only slides when the selection would otherwise fall off the edge.
func (m *Model) scrollFleet(lines []string, selStart, selEnd, h int) []string {
	if len(lines) <= h {
		m.fleetScroll = 0
		return lines
	}
	off := m.fleetScroll
	if selEnd >= 0 {
		if selStart < off {
			off = selStart
		}
		if selEnd >= off+h {
			off = selEnd - h + 1
		}
	}
	if off > len(lines)-h {
		off = len(lines) - h
	}
	if off < 0 {
		off = 0
	}
	// Never open the column on a line of air: the air between two entries reads
	// as a gap under the title. Losing it at the top costs nothing — the row it
	// would have pushed out is a blank at the bottom, where nobody sees it.
	if off > 0 && lines[off] == "" {
		off++
	}
	m.fleetScroll = off
	end := off + h
	if end > len(lines) {
		end = len(lines)
	}
	return lines[off:end]
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

	rows := make([]fleetRow, 0, len(m.sessions)+len(groups))
	n := 0
	for _, g := range groups {
		if headers {
			rows = append(rows, fleetRow{header: true, label: g.name, echo: m.groupEcho(g)})
		}
		for _, i := range g.entries {
			n++
			num := 0
			if n <= 9 {
				num = n
			}
			rows = append(rows, fleetRow{sess: i, num: num})
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
		if !s.Live {
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
	att := make([]int, 0, len(idx))
	rest := make([]int, 0, len(idx))
	for _, i := range idx {
		if wantsAttention(m.sessions[i].Snap.State) {
			att = append(att, i)
			continue
		}
		rest = append(rest, i)
	}
	sort.SliceStable(rest, func(a, b int) bool {
		wa, pa := m.paneCoords(rest[a])
		wb, pb := m.paneCoords(rest[b])
		if wa != wb {
			return wa < wb
		}
		return pa < pb
	})
	return append(att, rest...)
}

// archiveGroups buckets the archived sessions by project — where you started
// them, which is how you remember them — newest group first, newest first
// inside.
func (m *Model) archiveGroups() []fleetGroup {
	members := map[string][]int{}
	var names []string
	for i, s := range m.sessions {
		if s.Live {
			continue
		}
		name := sessionName(s.Info)
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
	echo := ""
	for _, i := range g.entries {
		switch m.sessions[i].Snap.State {
		case state.NeedsYou:
			return fleet.Glyph(state.NeedsYou)
		case state.Stuck:
			echo = fleet.Glyph(state.Stuck)
		}
	}
	return echo
}

// groupHeaderLine draws a header: dim, unnumbered, unselectable, sitting in the
// index column so the group name lines up with the numbers beneath it.
func (m *Model) groupHeaderLine(r fleetRow, w int) string {
	label := " " + clip(r.label, w-1-echoWidth(r.echo))
	if r.echo == "" {
		return dimStyle.Render(label)
	}
	accent := needsYouStyle
	if r.echo == fleet.Glyph(state.Stuck) {
		accent = stuckStyle
	}
	return dimStyle.Render(pad(label, w-echoWidth(r.echo))) + accent.Render(r.echo)
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
	s := m.sessions[r.sess]
	selected := s.Info.Key() == m.selectedKey
	st := s.Snap.State
	if m.archiveView {
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

	age := padLeft(m.age(s.Snap.Since), ageWidth)
	head := headline(s)
	if m.archiveView {
		// The group header already names the project, and an archived session is
		// named after its project: the row spends that width on the one thing the
		// header cannot say — what you asked for.
		age, head = padLeft(m.age(s.Info.LastEventAt), ageWidth), archiveHeadline(s)
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
	if m.archiveView {
		// No name: the whole line is the prompt the session was given.
		body = midStyle.Render(pad(clip(head, avail), avail))
	}

	first := marker + indexStyled + " " + accent.Render(fleet.Glyph(st)) + " " +
		body + " " + dimStyle.Render(age)

	return []string{first, strings.Repeat(" ", 4) + m.secondLine(s, w-4)}
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
	if m.archiveView {
		return dimStyle.Render(clip(branchOf(s.Info), w))
	}
	// Result before process. "1216✓ 2✗" answers whether the session is going
	// well; "Bash: pytest tests/auth -x" only answers whether it is busy, which
	// the glyph beside the name already said. The call in flight is the
	// fallback for a session that has not finished anything yet — and for a
	// quiet one, the prompt it was given, because "idle" is what the first line
	// already told you.
	act := s.Outcome
	if strings.TrimSpace(act) == "" {
		act = s.Snap.Activity
	}
	if strings.TrimSpace(act) == "" || act == "idle" {
		act = s.Info.Title
	}
	if strings.TrimSpace(act) == "" {
		act = s.Snap.Reason
	}
	if !s.HasClass {
		// Nothing classifiable yet — a session that has only been asked for, or
		// one woken from the archive whose replay has not reached a tool call.
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
	switch s.Snap.State {
	case state.NeedsYou, state.Stuck:
		return stateLabel(s.Snap.State)
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
