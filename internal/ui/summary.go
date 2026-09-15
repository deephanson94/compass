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

// The summary (#344, #345). A trail a day long is two hundred rows, and the
// question a person brings back to it — how many times did it ship, what
// did it build — is answered by walking them. `s` on the legs swaps the
// trail for its legs counted by class, one row per class the trail has,
// in the trail's own order of work: scout, design, build, fix, test,
// ship, docs, then the subagent lanes. `space` opens a class under the
// cursor into its legs, oldest first, each the row the trail draws for
// it; a class with one leg is that leg's own row, since a count of one
// says less than the label. `tab` on a leg is the trail with the cursor
// on that leg, the conversation beside it as it always is; `s` or `esc`
// is the trail as it was left. It is a view of the legs, not a level: the
// depth stays at three (board, session, summary; a class's legs are the
// summary's own rows, like a leg's waypoints are the trail's).

// summaryRow is one row of the summary: a class with its count, a leg
// under an open class (or standing for a class of one), the lanes' count,
// or a lane under it.
type summaryRow struct {
	kind  string // "class", "leg", "lanes" or "lane"
	class journey.Class
	key   string // what open is keyed by: the class's name, or summaryLanes
	leg   int    // a leg row: index into Trail.Legs; else -1
	lane  int    // a lane row: index into Trail.Branches; else -1
	solo  bool   // a leg row standing for its class: the class has one leg
}

// summaryOrder is the order the classes are counted in: the order work
// tends to happen in, which is the order the legend lists them.
var summaryOrder = []journey.Class{journey.Scout, journey.Design, journey.Build, journey.Fix, journey.Test, journey.Ship, journey.Docs}

// summaryLanes keys the lanes' row in the open set.
const summaryLanes = "agent"

// summaryRows lists the summary's rows for a trail: a row per class with
// at least one leg, its legs beneath it where open, and the lanes last.
func summaryRows(tr journey.Trail, open map[string]bool) []summaryRow {
	var out []summaryRow
	for _, c := range summaryOrder {
		var legs []int
		for i := range tr.Legs {
			if tr.Legs[i].Class == c {
				legs = append(legs, i)
			}
		}
		switch {
		case len(legs) == 0:
			continue
		case len(legs) == 1:
			// One leg: its own row, label and all, where a count of
			// one would say less than the trail did (#345).
			out = append(out, summaryRow{kind: "leg", class: c, key: c.String(), leg: legs[0], lane: -1, solo: true})
			continue
		}
		out = append(out, summaryRow{kind: "class", class: c, key: c.String(), leg: -1, lane: -1})
		if !open[c.String()] {
			continue
		}
		for _, i := range legs {
			out = append(out, summaryRow{kind: "leg", class: c, key: c.String(), leg: i, lane: -1})
		}
	}
	if len(tr.Branches) > 0 {
		out = append(out, summaryRow{kind: "lanes", key: summaryLanes, leg: -1, lane: -1})
		if open[summaryLanes] {
			for i := range tr.Branches {
				out = append(out, summaryRow{kind: "lane", key: summaryLanes, leg: -1, lane: i})
			}
		}
	}
	return out
}

// group reports whether the row opens into rows of its own.
func (r summaryRow) group() bool {
	return r.kind == "class" || r.kind == "lanes"
}

// summaryShown reports whether the trail column draws the summary: it is
// switched on, and the keys are on the legs of one trail.
func (m *Model) summaryShown() bool {
	return m.summary && m.level == levelWaypoints && !m.showHelp
}

// summaryRefusal is why `s` does nothing here, or "" where it works: the
// summary is one trail's legs, so the board and the trail say the way in,
// and a trail with no leg has nothing to count.
func (m *Model) summaryRefusal() string {
	switch {
	case m.level == levelBoard && m.boardShown():
		return "the summary is one trail's · tab into it"
	case m.level == levelBoard || m.level == levelTrail:
		// The row beside the note names `tab deeper`, so the note
		// keeps to what the row cannot say (#345).
		return "the summary is the legs'"
	case m.level >= levelReader:
		return "the summary is the legs' · esc, then s"
	case len(m.trail.Legs) == 0 && len(m.trail.Branches) == 0:
		return "no leg to count yet"
	}
	return ""
}

// toggleSummary is `s`: the summary where the trail was, the trail where
// the summary was. Reopened on the session it was left on, it stands
// where it stood, with the same classes open — the round trip `tab`
// invites, lane to trail and back. Opened on another session it starts
// afresh, or the first frame would depend on a session off screen (#345).
func (m *Model) toggleSummary() {
	if m.summary {
		m.summary = false
		return
	}
	if why := m.summaryRefusal(); why != "" {
		m.note = why
		return
	}
	m.summary = true
	if m.summaryOpen == nil || m.summaryOn != m.selectedKey {
		m.summaryOpen = map[string]bool{}
		m.summaryCursor, m.summaryScroll = 0, 0
		m.summaryOn = m.selectedKey
	}
}

// summaryClamp keeps the cursor on a row the trail still has: a poll can
// shorten nothing, but a session change (h/l, a digit) swaps the trail.
func (m *Model) summaryClamp(rows []summaryRow) {
	if m.summaryCursor >= len(rows) {
		m.summaryCursor = len(rows) - 1
	}
	if m.summaryCursor < 0 {
		m.summaryCursor = 0
	}
}

// summaryKey handles a key while the summary is up. It reports whether the
// key was the summary's; the rest — attach, the digits, the session keys,
// reply, ask — keep their meaning at every level and fall through.
func (m *Model) summaryKey(key string) bool {
	rows := summaryRows(m.trail, m.summaryOpen)
	m.summaryClamp(rows)
	if len(rows) == 0 {
		// The trail lost its legs from under the summary (a session swap
		// onto an empty one): the trail is what there is to show.
		m.summary = false
		m.note = "no leg to count yet"
		return true
	}
	row := rows[m.summaryCursor]
	switch key {
	case "s", "esc":
		m.summary = false
		return true
	case "j", "down":
		if m.summaryCursor >= len(rows)-1 {
			m.note = "at the end · k goes back"
			return true
		}
		m.summaryCursor++
		return true
	case "k", "up":
		if m.summaryCursor == 0 {
			m.note = "at the start"
			return true
		}
		m.summaryCursor--
		return true
	case "ctrl+d", "ctrl+u":
		// Half a page: the window moves with the cursor, so two presses
		// advance a long list by a page, not a row (#345).
		step := m.trailHalfPage()
		if key == "ctrl+u" {
			step = -step
		}
		was := m.summaryCursor
		m.summaryCursor = min(max(m.summaryCursor+step, 0), len(rows)-1)
		m.summaryScroll = max(m.summaryScroll+step, 0)
		if m.summaryCursor == was {
			if step < 0 {
				m.note = "at the start"
			} else {
				m.note = "at the end · k goes back"
			}
		}
		return true
	case "G":
		if m.summaryCursor == len(rows)-1 {
			m.note = "at the end"
			return true
		}
		m.summaryCursor = len(rows) - 1
		return true
	case " ", "space":
		if row.solo {
			m.note = "one leg · tab is the trail there"
			return true
		}
		// Open or close the group the cursor is in: a class or the lanes
		// from its own row, or from any row under it — closing from a leg
		// puts the cursor on the class it folded into; opening frames the
		// group, its row first and its legs filling the window (#345).
		m.summaryOpen[row.key] = !m.summaryOpen[row.key]
		if !m.summaryOpen[row.key] {
			m.summaryCursor = summaryGroupRow(rows, m.summaryCursor)
		} else {
			m.summaryScroll = m.summaryCursor
		}
		return true
	case "tab":
		switch {
		case row.group():
			// Deeper into a class is its legs: open it, framed, and stand
			// on the first. A class already open just moves the cursor on.
			if !m.summaryOpen[row.key] {
				m.summaryOpen[row.key] = true
				m.summaryScroll = m.summaryCursor
			}
			m.summaryCursor++
			return true
		case row.kind == "leg":
			m.summary = false
			m.cursorOnLeg(row.leg)
			return true
		case row.kind == "lane":
			m.summary = false
			m.cursorOnLane(m.trail.Branches[row.lane].ToolUseID)
			return true
		}
	case "[", "]":
		m.note = "chapters are the trail's · s or esc brings it back"
		return true
	}
	return false
}

// summaryGroupRow is the index of the class or lanes row the row at i
// belongs to: itself when it is one.
func summaryGroupRow(rows []summaryRow, i int) int {
	for j := i; j >= 0; j-- {
		if rows[j].group() {
			return j
		}
	}
	return 0
}

// cursorOnLeg puts the trail's cursor on a leg's own row: the trail
// scrolls to it and the conversation follows, as a j/k move would.
func (m *Model) cursorOnLeg(leg int) {
	for i, r := range TrailRows(m.trail, m.level) {
		if r.Kind == "leg" && r.Leg == leg {
			m.cursor = i
			break
		}
	}
	m.cursorMove(0)
}

// cursorOnLane puts the trail's cursor on a lane's own row.
func (m *Model) cursorOnLane(id string) {
	for i, r := range TrailRows(m.trail, m.level) {
		if r.Kind == "branch" && r.Lane == id {
			m.cursor = i
			break
		}
	}
	m.cursorMove(0)
}

// summaryFoldWord is what `space` does on the cursor's row: open a closed
// class, close an open one — or the group a leg is under. A class of one
// has nothing to open: `space` says so, and the row does not name it.
func (m *Model) summaryFoldWord() string {
	rows := summaryRows(m.trail, m.summaryOpen)
	if len(rows) == 0 {
		return "open"
	}
	m.summaryClamp(rows)
	switch {
	case rows[m.summaryCursor].solo:
		return ""
	case m.summaryOpen[rows[m.summaryCursor].key]:
		return "close"
	}
	return "open"
}

// summaryTabWord is what `tab` does on the cursor's row: into a class's
// legs (the lanes' lanes), or to the trail at a leg.
func (m *Model) summaryTabWord() string {
	rows := summaryRows(m.trail, m.summaryOpen)
	if len(rows) == 0 {
		return "legs"
	}
	m.summaryClamp(rows)
	switch rows[m.summaryCursor].kind {
	case "leg", "lane":
		return "trail there"
	case "lanes":
		return "lanes"
	}
	return "legs"
}

// summaryLines draws the summary into a width×height block: the rows,
// the cursor's on screen, the rest scrolled as the cursor moves. A window
// that hides rows says so on its own edges, as a cut column does (#17):
// `↑ 12 above` on its first row, `▾ 17 more scout · 6 classes` on its last
// — the legs still to come of the class it cuts, and the counts after.
func (m *Model) summaryLines(w, h int) []string {
	rows := summaryRows(m.trail, m.summaryOpen)
	m.summaryClamp(rows)
	if len(rows) == 0 {
		return fit([]string{dimStyle.Render(clip("no leg to count yet", w))}, h)
	}
	if h < 1 {
		h = 1
	}
	lines := make([]string, 0, len(rows))
	for i := range rows {
		text := m.summaryRow(rows, i, w)
		if i == m.summaryCursor {
			text = summaryCursored(text, w)
		}
		lines = append(lines, text)
	}
	top, head, tail := summaryWindow(len(lines), h, m.summaryCursor, m.summaryScroll)
	m.summaryScroll = top
	out := make([]string, 0, h)
	if head {
		out = append(out, dimStyle.Render(clip(fmt.Sprintf("↑ %d above", top), w)))
	}
	end := min(top+h, len(lines))
	if tail {
		end--
	}
	out = append(out, lines[top+len(out):end]...)
	if tail {
		out = append(out, dimStyle.Render(clip(summaryBelow(rows, end), w)))
	}
	return fit(out, h)
}

// summaryBelow is the last row's word for what the window cuts: the rows
// of the group cut at the edge, by its name, then the groups after it.
func summaryBelow(rows []summaryRow, from int) string {
	rest := rows[from:]
	if len(rest) == 0 {
		return ""
	}
	if rest[0].group() || rest[0].solo {
		return fmt.Sprintf("▾ %d more below", len(rest))
	}
	same, groups := 0, 0
	for _, r := range rest {
		switch {
		case r.group() || r.solo:
			groups++
		case groups == 0:
			same++
		}
	}
	noun := rest[0].key
	if rest[0].kind == "lane" {
		noun = "lanes"
	}
	out := fmt.Sprintf("▾ %d more %s", same, noun)
	if groups > 0 {
		out += fmt.Sprintf(" · %d classes", groups)
		if groups == 1 {
			out = strings.TrimSuffix(out, "es")
		}
	}
	return out
}

// summaryWindow is the first row a window of h rows draws, given where it
// last stood and where the cursor is, and whether its first and last rows
// are spent saying what is hidden above and below. The window stays put
// while the cursor is inside it, and moves the least that brings the
// cursor in — onto a row of its own, never onto an edge row's words.
func summaryWindow(total, h, cursor, scroll int) (top int, head, tail bool) {
	if total <= h {
		return 0, false, false
	}
	top = clampScroll(scroll, total, h)
	for range 4 {
		head = top > 0 && h >= 3
		tail = top+h < total && h >= 3
		lo, hi := top, top+h-1
		if head {
			lo++
		}
		if tail {
			hi--
		}
		switch {
		case cursor < lo:
			top -= lo - cursor
		case cursor > hi:
			top += cursor - hi
		default:
			return top, head, tail
		}
		top = clampScroll(top, total, h)
	}
	return top, head, tail
}

// summaryRow draws one row. A class row is the trail's leg row shape —
// glyph, verb, then the count where the label goes and the class's span
// where the age goes: `◆ build   3 legs · 2 red        1h20m`. A leg under
// it is the row the trail draws for that leg, hung on the trail's own
// waypoint rail so the group reads as one; a leg standing for a class of
// one is that row, unhung.
func (m *Model) summaryRow(rows []summaryRow, i, w int) string {
	r := rows[i]
	switch r.kind {
	case "class":
		return summaryClassRow(m.trail, r.class, m.trailOpts(w, 1), w)
	case "lanes":
		return summaryLanesRow(m.trail, m.trailOpts(w, 1), w)
	case "leg":
		width := w
		if !r.solo {
			width -= trailWayWidth
		}
		o := m.trailOpts(width, 1)
		l := m.trail.Legs[r.leg]
		label, narrated := legLabel(l, o)
		if r.solo {
			return legRow(l, label, narrated, o)
		}
		return summaryHang(rows, i) + legRow(l, label, narrated, o)
	case "lane":
		return summaryHang(rows, i) + summaryLaneRow(m.trail.Branches[r.lane], m.trailOpts(w-trailWayWidth, 1), w-trailWayWidth)
	}
	return ""
}

// summaryHang is the rail a row under a class hangs on: `│  ├ ` for one
// with more beneath it, `│  └ ` for the last, the trail's own marks.
func summaryHang(rows []summaryRow, i int) string {
	mark := wayTee
	if i+1 >= len(rows) || rows[i+1].group() || rows[i+1].solo {
		mark = wayEnd
	}
	return ruleStyle.Render(railStroke+"  "+mark) + " "
}

// summaryClassRow is `◆ build   3 legs · 2 red        1h20m`: the count,
// the red runs and the loop where the class is test, and the time the
// class took, summed over its legs — a leg still running counted to now,
// as the trail's `for 18m` counts it. The class that holds the present
// wears HEAD's own glyph: a class whose leg is silent or asking is not
// done (#345).
func summaryClassRow(tr journey.Trail, c journey.Class, o TrailOpts, w int) string {
	n, red, runs := 0, 0, 0
	var span time.Duration
	glyph := glyphLeg
	for _, l := range tr.Legs {
		if l.Class != c {
			continue
		}
		n++
		if strings.Contains(legBadge(l), "✗") {
			red++
		}
		for _, wp := range l.Waypoints {
			if wp.Kind == journey.WaypointTestFail && wp.Runs > runs {
				runs = wp.Runs
			}
		}
		end := l.End
		if l.Current {
			end = o.Now
			glyph, _ = headMark(o, l)
		}
		if end.After(l.Start) {
			span += end.Sub(l.Start)
		}
	}
	head := classStyle(c).Render(glyph + " " + pad(c.String(), trailVerbWidth))
	var badge []string
	if red > 0 {
		badge = append(badge, fmt.Sprintf("%d red", red)) // the card's word for a red run (#345)
	}
	if runs >= 2 {
		badge = append(badge, ordinal(runs)+" failure") // the fleet row's word for the loop
	}
	return summaryFigureRow(head, plural(n, "leg"), badge, spanText(span), w)
}

// spanText is a summed span at the minute: `1h45m`, `45m`, `1d3h`. The
// trail's ages floor to the hour, which is right for an age and wrong
// for a sum, where four classes twelve minutes apart all read `1h`.
func spanText(d time.Duration) string {
	switch {
	case d < time.Minute:
		return ""
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		h, mins := int(d.Hours()), int(d.Minutes())%60
		if mins == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, mins)
	default:
		days, h := int(d.Hours())/24, int(d.Hours())%24
		if h == 0 {
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dd%dh", days, h)
	}
}

// summaryLanesRow is `◈ agent   4 lanes · 2 silent 18m       20m`: the
// lanes sent out, the ones that went quiet or came back empty — the
// facts the card's own `◈3 out` does not carry — and the clock of the
// oldest still out, or of the last one back.
func summaryLanesRow(tr journey.Trail, o TrailOpts, w int) string {
	silent, empty := 0, 0
	var quiet time.Duration
	var oldest, latest time.Time
	for _, br := range tr.Branches {
		if br.Done {
			if strings.TrimSpace(br.Report) == "" {
				empty++
			}
			back := br.End
			if back.IsZero() {
				back = br.Start
			}
			if back.After(latest) {
				latest = back
			}
			continue
		}
		if oldest.IsZero() || br.Start.Before(oldest) {
			oldest = br.Start
		}
		live, known := o.Agents[br.ToolUseID]
		if d, hung := laneSilence(live, br, o.Now); known && hung {
			silent++
			if d > quiet {
				quiet = d
			}
		}
	}
	head := textStyle.Render(glyphBranch + " " + pad(summaryLanes, trailVerbWidth))
	var badge []string
	if silent > 0 {
		badge = append(badge, fmt.Sprintf("%d silent %s", silent, state.ShortDuration(quiet)))
	}
	if empty > 0 {
		badge = append(badge, fmt.Sprintf("%d empty", empty))
	}
	age := ""
	switch {
	case !oldest.IsZero():
		age = relAge(o.Now, oldest)
	case !latest.IsZero():
		age = relAge(o.Now, latest)
	}
	return summaryFigureRow(head, plural(len(tr.Branches), "lane"), badge, age, w)
}

// summaryFigureRow lays a count row out on the leg row's grid: the head,
// the count, the badge's clauses after dots — shed whole, last first,
// where the row is short — and the age at the right edge, ending where
// the leg rows' ages end.
func summaryFigureRow(head, count string, badge []string, age string, w int) string {
	body := w - trailPrefixWidth
	if body < 1 {
		return head
	}
	text := count
	for n := len(badge); n > 0; n-- {
		if with := count + " · " + strings.Join(badge[:n], " · "); len([]rune(with)) <= body-len([]rune(age))-1 {
			text = with
			break
		}
	}
	room := body - len([]rune(age))
	if age != "" {
		room--
	}
	if room < len([]rune(text)) {
		age, room = "", body
	}
	row := head + " " + dimStyle.Render(pad(clip(text, room), room))
	if age != "" {
		row += " " + dimStyle.Render(age)
	}
	return row
}

// summaryLaneRow is one lane, as the trail draws it: `◈ scouted payments
// ✓ 7m ago`, `⋯ 20m out` while it is out, `◍` where its own file has gone
// quiet, `→1` where a live session is on its lane, `⌀` back with nothing
// or lost with its lead gone idle.
func summaryLaneRow(br journey.Branch, o TrailOpts, w int) string {
	mark := branchOpen
	tail := mark + " " + relAge(o.Now, br.Start) + " out"
	if !br.Done && o.HeadState == state.Idle {
		tail = branchEmpty + " lost " + relAge(o.Now, br.Start) + " ago"
	}
	if br.Done {
		mark = branchDone
		if strings.TrimSpace(br.Report) == "" {
			mark = branchEmpty
		}
		back := br.End
		if back.IsZero() {
			back = br.Start
		}
		tail = mark + " " + relAge(o.Now, back) + " ago"
	}
	glyph := textStyle.Render(glyphBranch)
	live, known := o.Agents[br.ToolUseID]
	if _, hung := laneSilence(live, br, o.Now); !br.Done && known && hung {
		glyph = stuckStyle.Render(fleet.Glyph(state.Stuck)) // the trail's mark, said here too (#49)
	}
	labelWidth := w - 2 - 1 - len([]rune(tail))
	if labelWidth < trailMinLabel {
		return glyph + " " + dimStyle.Render(clip(tail, w-2))
	}
	name := clip(branchName(br.Label), labelWidth)
	if n, ok := o.LaneLinks[br.Label]; ok && n > 0 {
		link := fmt.Sprintf(" →%d", n)
		name = clip(branchName(br.Label), labelWidth-len([]rune(link))) + link
	}
	return glyph + " " + dimStyle.Render(pad(name, labelWidth)) + " " + dimStyle.Render(tail)
}

// summaryCursored marks the cursor's row as the trail marks its own: the
// cell after the glyph turns ▸ and the row is reversed to the edge.
func summaryCursored(text string, w int) string {
	plain := strings.TrimRight(ansi.Strip(text), " ")
	r := []rune(plain)
	switch {
	case len(r) > 4 && strings.HasPrefix(plain, railStroke):
		// A hung row: the mark goes where the rail's air is, `│▸ ├ ◆…`,
		// so the rail and the glyph both keep their cells.
		r[1] = '▸'
	case len(r) > 1 && r[1] == ' ':
		r[1] = '▸'
	case len(r) > 0:
		r[0] = '▸'
	}
	plain = string(r)
	if n := w - lipgloss.Width(plain); n > 0 {
		plain += strings.Repeat(" ", n)
	}
	return cursorStyle.Render(plain)
}
