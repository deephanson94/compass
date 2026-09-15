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

// The summary (#344, #345, #346). A trail a day long is two hundred rows,
// and the question a person brings back to it — how many times did it
// ship, what did it build — is answered by walking them. `s` on the legs
// swaps the trail for its legs counted by class, one row per class the
// trail has, in the trail's own order of work: scout, design, build, fix,
// test, ship, docs, then the subagent lanes. `space` opens a class under
// the cursor into its legs, oldest first, each the row the trail draws
// for it; a class with one leg is that leg's own row, since a count of
// one says less than the label. `tab` on a leg is the trail with the
// cursor on that leg, the conversation beside it as it always is; `s` or
// `esc` is the trail as it was left. It is a view of the legs, not a
// level: the depth stays at three (board, session, summary; a class's
// legs are the summary's own rows, like a leg's waypoints are the
// trail's). A trail with nothing to count — no leg, or one of each — is
// refused: the summary would be the trail with its ask taken off.

// summaryRow is one row of the summary: a class with its count, a leg
// under an open class (or standing for a class of one), the lanes' count,
// a lane under it, or a line of the question HEAD is asking, which the
// cursor steps over as it does on the trail.
type summaryRow struct {
	kind  string // "class", "leg", "lanes", "lane", "ask" or "report"
	class journey.Class
	key   string // what open is keyed by: the class's name, or summaryLanes
	leg   int    // a leg row: index into Trail.Legs; else -1
	lane  int    // a lane row: index into Trail.Branches; else -1
	solo  bool   // a leg row standing for its class: the class has one leg
	text  string // an ask row: its line of the question; a report row: the lane's finding
}

// summaryOrder is the order the classes are counted in: the order work
// tends to happen in, which is the order the legend lists them.
var summaryOrder = []journey.Class{journey.Scout, journey.Design, journey.Build, journey.Fix, journey.Test, journey.Ship, journey.Docs}

// summaryLanes keys the lanes' row in the open set.
const summaryLanes = "agent"

// summaryRows lists the summary's rows for a trail: a row per class with
// at least one leg, its legs beneath it where open, and the lanes last.
// A question HEAD is asking hangs under HEAD's row, at every level (#17).
func summaryRows(tr journey.Trail, open map[string]bool, ask []string) []summaryRow {
	var out []summaryRow
	askUnder := func(i int) {
		if !tr.Legs[i].Current {
			return
		}
		for _, line := range ask {
			out = append(out, summaryRow{kind: "ask", leg: i, lane: -1, text: line})
		}
	}
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
			askUnder(legs[0])
			continue
		}
		out = append(out, summaryRow{kind: "class", class: c, key: c.String(), leg: -1, lane: -1})
		if !open[c.String()] {
			continue
		}
		for _, i := range legs {
			out = append(out, summaryRow{kind: "leg", class: c, key: c.String(), leg: i, lane: -1})
			askUnder(i)
		}
	}
	if len(tr.Branches) > 0 {
		out = append(out, summaryRow{kind: "lanes", key: summaryLanes, leg: -1, lane: -1})
		if open[summaryLanes] {
			for i := range tr.Branches {
				out = append(out, summaryRow{kind: "lane", key: summaryLanes, leg: -1, lane: i})
				if report := strings.TrimSpace(tr.Branches[i].Report); tr.Branches[i].Done && report != "" {
					// Back with what: the finding, as the trail hangs it (#347).
					out = append(out, summaryRow{kind: "report", key: summaryLanes, leg: -1, lane: i, text: report})
				}
			}
		}
	}
	return out
}

// group reports whether the row opens into rows of its own.
func (r summaryRow) group() bool {
	return r.kind == "class" || r.kind == "lanes"
}

// stands reports whether the cursor can stand on the row: a question's
// lines are HEAD's, not rows of their own (#346).
func (r summaryRow) stands() bool {
	return r.kind != "ask" && r.kind != "report"
}

// summaryShown reports whether the trail column draws the summary: it is
// switched on, and the keys are on the legs of one trail.
func (m *Model) summaryShown() bool {
	return m.summary && m.level == levelWaypoints && !m.showHelp
}

// summaryCountsNothing says whether the summary would draw one row per
// leg: no class has two legs and there are no lanes to tally, so every
// row would be the row the trail already drew (#346).
func summaryCountsNothing(tr journey.Trail) bool {
	if len(tr.Branches) > 0 {
		return false
	}
	seen := map[journey.Class]int{}
	for _, l := range tr.Legs {
		seen[l.Class]++
		if seen[l.Class] > 1 {
			return false
		}
	}
	return true
}

// summaryRefusal is why `s` does nothing here, or "" where it works: the
// summary is one trail's legs, so the board and the trail say the way in,
// and a trail with nothing to count has nothing to show for it. The notes
// keep to what the row beside them cannot say: `tab deeper` and `esc
// back` are on the row (#345, #346).
func (m *Model) summaryRefusal() string {
	tr := m.trail
	if t, ok := m.trails[m.selectedKey]; ok && m.level < levelWaypoints {
		tr = t // the board's selection, before its trail is the deck's
	}
	switch {
	case len(tr.Legs) == 0 && len(tr.Branches) == 0:
		return "no leg yet" // HEAD's row is the fleet's, not a leg (#347)
	case summaryCountsNothing(tr):
		return "one leg a class" // the fact, where `nothing to count` contradicted the legs on the frame (#347)
	case m.level == levelBoard && m.boardShown():
		return "the summary is one trail's · tab into it"
	case m.level == levelBoard || m.level == levelTrail || m.level >= levelReader:
		return "the summary is the legs'"
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
	m.summarySync()
}

// summarySync starts the summary afresh where the session under it is
// not the one it was opened on — a digit, `h`/`l` — so a session's first
// summary frame is its own counts, not another trail's fold (#346).
func (m *Model) summarySync() {
	if summaryCountsNothing(m.trail) {
		// Stepped onto a trail with nothing to count: the trail is what
		// there is to show, on this frame, not the next (#347).
		m.summary = false
		if m.note == "" {
			m.note = m.summaryRefusal()
		}
		return
	}
	if m.summaryOpen != nil && m.summaryOn == m.selectedKey {
		return
	}
	m.summaryOpen = map[string]bool{}
	m.summaryCursor, m.summaryScroll = 0, 0
	m.summaryOn = m.selectedKey
}

// summaryClamp keeps the cursor on a row the trail still has and one it
// can stand on: a poll can shorten nothing, but a session swap changes
// the trail under it.
func (m *Model) summaryClamp(rows []summaryRow) {
	if m.summaryCursor >= len(rows) {
		m.summaryCursor = len(rows) - 1
	}
	if m.summaryCursor < 0 {
		m.summaryCursor = 0
	}
	for m.summaryCursor > 0 && m.summaryCursor < len(rows) && !rows[m.summaryCursor].stands() {
		m.summaryCursor--
	}
}

// summaryStep moves the cursor by delta rows it can stand on; it reports
// whether it moved.
func (m *Model) summaryStep(rows []summaryRow, delta int) bool {
	i := m.summaryCursor
	for {
		i += delta
		if i < 0 || i >= len(rows) {
			return false
		}
		if rows[i].stands() {
			m.summaryCursor = i
			return true
		}
	}
}

// summaryPresent is the row of the class that holds the present — HEAD's
// class, or HEAD's own row where its class is one leg — or -1.
func summaryPresent(tr journey.Trail, rows []summaryRow) int {
	at := -1
	for i, r := range rows {
		switch {
		case r.kind == "leg" && tr.Legs[r.leg].Current:
			return i // HEAD's own row, solo or under its open class (#347)
		case r.kind == "class" && at < 0:
			for _, l := range tr.Legs {
				if l.Class == r.class && l.Current {
					at = i
				}
			}
		}
	}
	return at
}

// summaryKey handles a key while the summary is up. It reports whether the
// key was the summary's; the rest — attach, the digits, the session keys,
// reply, ask — keep their meaning at every level and fall through.
func (m *Model) summaryKey(key string) bool {
	m.summarySync()
	if !m.summary {
		return false // closed by the sync: the key is the trail's
	}
	rows := m.summaryRowsHere()
	m.summaryClamp(rows)
	row := rows[m.summaryCursor]
	switch key {
	case "s", "esc":
		m.summary = false
		return true
	case "j", "down":
		if !m.summaryStep(rows, 1) {
			m.note = "at the end · k goes back"
		}
		return true
	case "k", "up":
		if !m.summaryStep(rows, -1) {
			m.note = "at the start"
		}
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
		m.summaryClamp(rows)
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
		// The present: the class that holds HEAD, as the trail's `G` is
		// HEAD (#346) — the last row where nothing is running.
		at := summaryPresent(m.trail, rows)
		if at < 0 {
			at = len(rows) - 1
			for at > 0 && !rows[at].stands() {
				at--
			}
		}
		if m.summaryCursor == at {
			m.note = "at the present"
			return true
		}
		m.summaryCursor = at
		return true
	case " ", "space":
		if row.solo {
			m.note = "one leg" // the row names `tab trail there` itself (#346)
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

// summaryRowsHere is the summary's rows for the trail under it, the
// question HEAD is asking included, wrapped to the width its rows hang at.
func (m *Model) summaryRowsHere() []summaryRow {
	w, h := m.trailBox()
	return summaryRows(m.trail, m.summaryOpen, m.summaryAsk(m.trailOpts(w, h)))
}

// summaryAsk is the question HEAD is asking, the part its row does not
// carry, wrapped as the trail wraps it under HEAD (#17): options and all.
func (m *Model) summaryAsk(o TrailOpts) []string {
	if o.HeadState != state.NeedsYou || o.Head == "" {
		return nil
	}
	for _, l := range m.trail.Legs {
		if !l.Current {
			continue
		}
		label, _ := legLabel(l, o)
		if label == o.Head {
			return nil
		}
		rest := strings.TrimSpace(strings.TrimPrefix(o.Head, label))
		return wrapQuestion(rest, o.Width-trailWayWidth, 4)
	}
	return nil
}

// summaryFoldWord is what `space` does on the cursor's row: open a closed
// class, close an open one — or the group a leg is under. A class of one
// has nothing to open: `space` says so, and the row does not name it.
func (m *Model) summaryFoldWord() string {
	rows := m.summaryRowsHere()
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
	rows := m.summaryRowsHere()
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

// summaryLines draws the summary into a width×height block: the ask the
// trail opened on where no reader beside carries it, the rows, the
// cursor's on screen, the rest scrolled as the cursor moves. A window
// that hides rows says so on its own edges, as a cut column does (#17):
// `↑ 22 scout · 1 class` on its first row, `▾ 17 more scout · 6 classes`
// on its last — the legs cut of one class, and the counts beyond. Under
// the counts, where the room is, the time the session spent waiting on
// you: the one number of the day the counts do not hold (#346).
func (m *Model) summaryLines(w, h int) []string {
	rows := m.summaryRowsHere()
	m.summaryClamp(rows)
	if len(rows) == 0 {
		return fit([]string{dimStyle.Render(clip("nothing to count", w))}, h)
	}
	if h < 1 {
		h = 1
	}
	var out []string
	if !m.sessionView() && len(m.trail.Prompts) > 0 && h > 2 {
		// No reader beside to say what the session was asked: the
		// trail's own opening row keeps it (#346).
		out = append(out, promptRow(m.trail.Prompts[0], m.now, w, 1, len(m.trail.Prompts), promptWait(m.trail, 0)))
		h--
	}
	lines := make([]string, 0, len(rows))
	for i := range rows {
		text := m.summaryRow(rows, i, w)
		if i == m.summaryCursor {
			text = summaryCursored(text, w)
		}
		lines = append(lines, text)
	}
	var tail []string
	if d := promptWaits(m.trail); d >= waitNotable && len(lines)+1 < h {
		// To the minute, as every span in the column is (#347); the
		// title's own clause for it stands down while the summary is up.
		tail = append(tail, summaryFigureRow(dimStyle.Render("◉ waited"), "on you · "+plural(len(m.trail.Prompts), "prompt"), nil, spanText(d), w))
	}
	top, head, more := summaryWindow(len(lines), h, m.summaryCursor, m.summaryScroll)
	m.summaryScroll = top
	body := make([]string, 0, h)
	if head {
		body = append(body, dimStyle.Render(clip(summaryAbove(rows, top+1), w)))
	}
	end := min(top+h, len(lines))
	if more {
		end--
	}
	body = append(body, lines[top+len(body):end]...)
	if more {
		body = append(body, dimStyle.Render(clip(summaryBelow(rows, end), w)))
	} else {
		body = append(body, tail...)
	}
	return fit(append(out, body...), h+len(out))
}

// summaryAbove is the first row's word for what the window hides above
// it: the rows of the group cut at the edge, by its name, then the groups
// before it — the mirror of summaryBelow, and the count is the count.
func summaryAbove(rows []summaryRow, upto int) string {
	hidden := rows[:upto]
	if len(hidden) == 0 {
		return ""
	}
	last := hidden[len(hidden)-1]
	if last.group() || last.solo {
		return "↑ " + summaryGroups(len(summaryGroupsIn(hidden)))
	}
	same, groups := 0, 0
	for i := len(hidden) - 1; i >= 0; i-- {
		r := hidden[i]
		switch {
		case r.group() || r.solo:
			groups++
		case groups == 0 && r.kind != "ask":
			same++
		}
	}
	out := fmt.Sprintf("↑ %d %s", same, summaryNoun(last))
	if groups > 0 {
		out += " · " + summaryGroups(groups)
	}
	return out
}

// summaryBelow is the last row's word for what the window cuts: the rows
// of the group cut at the edge, by its name, then the groups after it.
func summaryBelow(rows []summaryRow, from int) string {
	rest := rows[from:]
	if len(rest) == 0 {
		return ""
	}
	if rest[0].group() || rest[0].solo {
		return "▾ " + summaryGroups(len(summaryGroupsIn(rest))) + " below"
	}
	same, groups := 0, 0
	for _, r := range rest {
		switch {
		case r.group() || r.solo:
			groups++
		case groups == 0 && r.kind != "ask":
			same++
		}
	}
	out := fmt.Sprintf("▾ %d more %s", same, summaryNoun(rest[0]))
	if groups > 0 {
		out += " · " + summaryGroups(groups)
	}
	return out
}

// summaryGroupsIn is the group rows — classes, the lanes, classes of one
// — among rows.
func summaryGroupsIn(rows []summaryRow) []summaryRow {
	var out []summaryRow
	for _, r := range rows {
		if r.group() || r.solo {
			out = append(out, r)
		}
	}
	return out
}

// summaryGroups is `6 classes`, `1 class`.
func summaryGroups(n int) string {
	if n == 1 {
		return "1 class"
	}
	return fmt.Sprintf("%d classes", n)
}

// summaryNoun is the word for a row hung under a group: its class, or
// `lanes`.
func summaryNoun(r summaryRow) string {
	if r.kind == "lane" {
		return "lanes"
	}
	return r.key
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
// one is that row, unhung. A question's line hangs under HEAD's row.
func (m *Model) summaryRow(rows []summaryRow, i, w int) string {
	r := rows[i]
	switch r.kind {
	case "class":
		return summaryClassRow(m.trail, r.class, m.trailOpts(w, 1), m.summaryRedWord(w), m.summaryLoopSaid(w), w)
	case "lanes":
		return summaryLanesRow(m.trail, m.trailOpts(w, 1), w)
	case "leg":
		width := w
		if !r.solo {
			width -= trailWayWidth
		}
		l := m.trail.Legs[r.leg]
		o := m.trailOpts(width, 1)
		// The trail sets the ask before it draws a leg, and a ship row
		// reads it: without it the row spells the ask a fourth time
		// where the trail said `commit` (#189, #192, #346).
		o.Ask = askBefore(m.trail, l.Start)
		label, narrated := legLabel(l, o)
		if r.solo {
			return legRow(l, label, narrated, o)
		}
		return summaryHang(rows, i) + legRow(l, label, narrated, o)
	case "lane":
		return summaryHang(rows, i) + summaryLaneRow(m.trail.Branches[r.lane], m.trailOpts(w-trailWayWidth, 1), w-trailWayWidth)
	case "ask", "report":
		return summaryHang(rows, i) + dimStyle.Render(clip(r.text, w-trailWayWidth-3))
	}
	return ""
}

// summaryLoopSaid reports whether the frame already names the loop — the
// card above the summary, or the fleet row beside it — so the class row
// does not say `4th failure` a second time on one frame (#347).
func (m *Model) summaryLoopSaid(w int) bool {
	if m.sessionView() {
		return strings.Contains(ansi.Strip(m.cardSecond(w)), " failure")
	}
	inner := m.width - 2*edgePad
	if fw, _, _ := m.layout(inner); fw == 0 {
		return false
	}
	live := false
	if s, ok := m.selected(); ok {
		live = s.Live
	}
	for _, p := range verdictParts(m.trail, m.now, live) {
		if strings.Contains(p, " failure") {
			return true
		}
	}
	return false
}

// summaryRedWord is the class row's word for a red run: the card's, where
// a card is drawn — `1 red`, or `1✗` where the card has shed to the
// badge's form — so one frame says the fact one way (#346).
func (m *Model) summaryRedWord(w int) string {
	if !m.sessionView() {
		return "red"
	}
	card := ansi.Strip(m.cardSecond(w))
	if strings.Contains(card, "✗") && !strings.Contains(card, " red") {
		return "✗"
	}
	return "red"
}

// summaryHang is the rail a row under a class hangs on: `│  ├ ` for one
// with more beneath it, `│  └ ` for the last, the trail's own marks. A
// question's line under a hung leg hangs a step deeper.
func summaryHang(rows []summaryRow, i int) string {
	r := rows[i]
	mark := wayTee
	if i+1 >= len(rows) || rows[i+1].group() || rows[i+1].solo {
		mark = wayEnd
	}
	if r.kind == "ask" || r.kind == "report" {
		if i+1 >= len(rows) || rows[i+1].kind != r.kind {
			mark = wayEnd
		}
		if r.kind == "report" || (rows[summaryGroupRow(rows, i)].kind == "class" && !summarySoloAbove(rows, i)) {
			return ruleStyle.Render(railStroke+"  "+railStroke+" "+mark) + " "
		}
		return ruleStyle.Render(railStroke+"  "+mark) + " "
	}
	return ruleStyle.Render(railStroke+"  "+mark) + " "
}

// summarySoloAbove reports whether the ask row at i hangs under a leg
// standing for a class of one.
func summarySoloAbove(rows []summaryRow, i int) bool {
	for j := i - 1; j >= 0; j-- {
		if rows[j].kind == "ask" {
			continue
		}
		return rows[j].solo
	}
	return false
}

// summaryClassRow is `◆ build   3 legs · 2 red        1h20m`: the count,
// the red runs and the loop where the class is test, and the time the
// class took, summed over its legs — a leg still running counted to now,
// as the trail's `for 18m` counts it, and said so, `· for 33m`, since the
// sum is mostly now on a session that is looping. The class that holds
// the present wears HEAD's own glyph: a class whose leg is silent or
// asking is not done (#345, #346).
func summaryClassRow(tr journey.Trail, c journey.Class, o TrailOpts, redWord string, loopSaid bool, w int) string {
	n, red, runs := 0, 0, 0
	var span time.Duration
	glyph, live := glyphLeg, ""
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
			glyph, live = headMark(o, l)
		}
		if end.After(l.Start) {
			span += end.Sub(l.Start)
		}
	}
	head := classStyle(c).Render(glyph + " " + pad(c.String(), trailVerbWidth))
	var badge []string
	if live != "" {
		badge = append(badge, live)
	}
	if red > 0 {
		if redWord == "✗" {
			badge = append(badge, fmt.Sprintf("%d✗", red))
		} else {
			badge = append(badge, fmt.Sprintf("%d %s", red, redWord)) // the card's word for a red run (#345)
		}
	}
	if runs >= 2 && !loopSaid {
		badge = append(badge, ordinal(runs)+" failure") // the fleet row's word for the loop, where the frame has not said it
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

// summaryLanesRow is `◈ agent   4 lanes       20m out`: the lanes sent
// out — the total, which the card does not add up — and the clock of the
// oldest still out, or of the last one back, with the lane rows' own word
// on it: the age column is a span everywhere else, and a lane's clock is
// not one (#346). The silent and the empty are the card's, said there.
func summaryLanesRow(tr journey.Trail, o TrailOpts, w int) string {
	var oldest, latest time.Time
	for _, br := range tr.Branches {
		if br.Done {
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
	}
	head := textStyle.Render(glyphBranch + " " + pad(summaryLanes, trailVerbWidth))
	age := ""
	switch {
	case !oldest.IsZero():
		age = relAge(o.Now, oldest) + " out"
	case !latest.IsZero():
		age = relAge(o.Now, latest) + " ago"
	}
	return summaryFigureRow(head, plural(len(tr.Branches), "lane"), nil, age, w)
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
