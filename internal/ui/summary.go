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

// The summary (#348–#377). A trail a day long is two hundred rows, and the
// question a person brings back to it — how many times did it ship, what
// did it build — is answered by walking them. The summary answers it
// standing: the trail's legs counted by class, one row per class the
// trail has, in the trail's own order of work — scout, design, build,
// fix, test, ship, docs — then the subagent lanes and the wait on you,
// drawn above the trail wherever a trail is drawn: the trail column at
// every level, and every column of the board. A class with one leg is
// that leg's own row, since a count of one says less than the label. The
// trail below gets the rows that are left, scrolled and walked as it
// always was. A trail with nothing to count — no leg, or one of each —
// draws no block: the trail says all there is. It was a view `s` opened
// (#348) and a toggle on the board (#376); #377 made it the trail's own,
// always, and the key went.

// summaryRow is one row of the summary: a class with its count, a leg
// under an open class (or standing for a class of one), the lanes' count,
// a lane under it, or a line of the question HEAD is asking, which the
// cursor steps over as it does on the trail.
type summaryRow struct {
	kind  string // "class", "leg", "lanes", "lane", "ask", "report" or "wait"
	class journey.Class
	key   string // what open is keyed by: the class's name, or summaryLanes
	leg   int    // a leg row: index into Trail.Legs; else -1
	lane  int    // a lane row: index into Trail.Branches; else -1
	solo  bool   // a leg row standing for its class: the class has one leg
	text  string // an ask row: its line of the question; a report row: the lane's finding, or what a lane still out says
	clock string // a report row of a lane still out: its clock, kept at the edge as the trail keeps it
}

// summaryOrder is the order the classes are counted in: the order work
// tends to happen in, which is the order the legend lists them.
var summaryOrder = []journey.Class{journey.Scout, journey.Design, journey.Build, journey.Fix, journey.Test, journey.Ship, journey.Docs}

// summaryLanes keys the lanes' row in the open set.
const summaryLanes = "agent"

// laneLine is what a lane still out says of itself under its row: its
// own file's last line and clock, as the trail hangs it (#352).
type laneLine struct {
	text  string
	clock string
}

// summaryRows lists the summary's rows for a trail: a row per class with
// at least one leg, its legs beneath it where open, and the lanes last.
// A question HEAD is asking hangs under HEAD's row, at every level (#17).
func summaryRows(tr journey.Trail, open map[string]bool, ask []string, lanes map[int]laneLine, wait string, findings int) []summaryRow {
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
			// one would say less than the trail did (#349).
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
					// Back with what: the finding, whole, as the trail hangs it —
					// the one line §4 exempts from the cut (#351, #355).
					for _, line := range wrapN(report, max(findings, trailMinLabel), 3) {
						out = append(out, summaryRow{kind: "report", key: summaryLanes, leg: -1, lane: i, text: line})
					}
				} else if line, ok := lanes[i]; ok {
					// Still out: what its own file says, mark and clock, as the trail hangs it (#352, #353).
					out = append(out, summaryRow{kind: "report", key: summaryLanes, leg: -1, lane: i, text: line.text, clock: line.clock})
				}
			}
		}
	}
	if wait != "" {
		// The time the session spent waiting on you: the one number of
		// the day the counts do not hold, a row of the document, so a
		// window that cuts it can be scrolled to it (#352).
		out = append(out, summaryRow{kind: "wait", leg: -1, lane: -1, text: wait})
	}
	return out
}

// group reports whether the row opens into rows of its own.
func (r summaryRow) group() bool {
	return r.kind == "class" || r.kind == "lanes"
}

// stands reports whether the cursor can stand on the row: a question's
// lines are HEAD's, not rows of their own (#350).
func (r summaryRow) stands() bool {
	return r.kind != "ask" && r.kind != "report" && r.kind != "wait"
}

// summaryCountsNothing says whether the summary would draw one row per
// leg: no class has two legs, there are no lanes to tally, and no wait
// on you worth a row — so every row would be the row the trail already
// drew (#350, #352).
func summaryCountsNothing(tr journey.Trail) bool {
	if len(tr.Branches) > 0 || promptWaits(tr) >= waitNotable {
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

// summaryLiveSaid reports whether the card above the summary already
// says how long HEAD has run — `· for 4m` in its verdict — so the class
// row does not say it again (#360). The fleet row beside, at 80 and
// 100, is the #352 hold.
func (m *Model) summaryLiveSaid(w int) bool {
	if !m.sessionView() {
		return false
	}
	for _, l := range m.trail.Legs {
		if l.Current {
			return strings.Contains(ansi.Strip(m.cardSecond(w)), "for "+relAge(m.now, l.Start))
		}
	}
	return false
}

// summaryLoopSaid reports whether the frame already names the loop — the
// card above the summary, or the fleet row beside it — so the class row
// does not say `4th failure` a second time on one frame (#351).
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

// summaryBackSaid reports whether the frame already says how many lanes
// came back — the card above the summary, or the fleet row beside it as
// it is drawn, which sheds `· 1 back` first at 80 and 100 — so the lanes
// row says it exactly where nothing else on the frame does (#361).
func (m *Model) summaryBackSaid(w int) bool {
	if m.sessionView() {
		return strings.Contains(ansi.Strip(m.cardSecond(w)), " back")
	}
	fw, _, _ := m.layout(m.width - 2*edgePad)
	if fw == 0 {
		return false
	}
	s, ok := m.selected()
	if !ok {
		return false
	}
	// The fleet row's second line, at the width entryLines draws it.
	return strings.Contains(ansi.Strip(m.secondLineUnder(s, fw-4, "")), " back")
}

// summaryClassRow is `◆ build   3 legs · 2 red        1h20m`: the count,
// the red runs and the loop where the class is test, and the time the
// class took, summed over its legs — a leg still running counted to now,
// as the trail's `for 18m` counts it, and said so, `· for 33m`, since the
// sum is mostly now on a session that is looping. The class that holds
// the present wears HEAD's own glyph: a class whose leg is silent or
// asking is not done (#349, #350).
func summaryClassRow(tr journey.Trail, c journey.Class, o TrailOpts, loopSaid, open, cardSaid bool, w int) string {
	n, red, runs := 0, 0, 0
	var span time.Duration
	glyph, live := glyphLeg, ""
	parked := false // HEAD's own row wears its lanes' clock, not the leg's span
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
			if strings.HasPrefix(live, "◈") || strings.HasPrefix(live, "for ") {
				span := "for " + relAge(o.Now, l.Start)
				// Parked is not "the tail begins ◈": a lead with lanes
				// out that has worked since the newest left wears
				// `◈3 out 20m · for 2h`, and that row carries the span
				// — and a narrow row sheds it. Ask HEAD's own row as
				// the summary draws it, under its class (#363, #366).
				oo := o
				oo.Width = w - trailWayWidth
				label, narrated := legLabel(l, oo)
				parked = !strings.Contains(ansi.Strip(legRow(l, label, narrated, oo)), span)
				live = span // the class's clause is how much of its sum is now; HEAD's tail is on HEAD's own row (#353)
			}
		}
		if end.After(l.Start) {
			span += end.Sub(l.Start)
		}
	}
	head := classStyle(c).Render(glyph + " " + pad(c.String(), trailVerbWidth))
	var badge []string
	if live != "" && !cardSaid && (!open || parked) {
		// Two yields, kept apart: the card above says it (#360), or the
		// class is open and HEAD's own row beneath says it (#355) —
		// unless that row does not carry the span, wearing the lanes'
		// clock alone or shedding the clause, and it is then on no row
		// but this one (#363, #366). Parked answers for HEAD's row only,
		// never for the card (#368).
		badge = append(badge, live)
	}
	if red > 0 {
		// The class's own red count, in the card's word: the title and
		// the card stand down for it while the summary is up, since a
		// total on the card is not the breakdown the rows are (#349,
		// #352, #371).
		badge = append(badge, fmt.Sprintf("%d red", red))
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
// not one (#350). The silent and the empty are the card's, said there;
// how many came back is the card's too, and the row's where the frame
// draws no card and the fleet row beside has shed it (#361).
func summaryLanesRow(tr journey.Trail, o TrailOpts, w int, backSaid, outSaid bool) string {
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
	if outSaid {
		// The frame already carries the oldest lane's clock — the card,
		// the fleet row beside, the column header or HEAD's own row
		// beneath — and one clock said twice on one frame is the thing
		// #22 and #64 forbid. The row spends the room on the tally instead.
		age = ""
	}
	back := 0
	for _, br := range tr.Branches {
		if br.Done {
			back++
		}
	}
	var badge []string
	if back > 0 && back < len(tr.Branches) && !backSaid {
		badge = append(badge, fmt.Sprintf("%d back", back))
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
	t := clip(text, room)
	row := head + " " + dimStyle.Render(t)
	// The pad is blanks: drawn unstyled, so a row that ends on them ends
	// on the same cells with colour on and off.
	if n := room - len([]rune(t)); n > 0 {
		row += strings.Repeat(" ", n)
	}
	if age != "" {
		row += " " + dimStyle.Render(age)
	}
	return row
}

// blockCounts says whether a trail draws the block: some class with
// more than one leg. A class of one is its leg's own row on the trail
// beneath, so the block never repeats a row the trail draws (#349, #377).
func blockCounts(tr journey.Trail) bool {
	for _, n := range classCounts(tr) {
		if n >= 2 {
			return true
		}
	}
	// Two lanes, or a wait on you worth a row, count too: the lanes row
	// and the wait row are counts the leg rows do not hold, and a trail
	// of one of each with a notable wait counted for `s` as well (#357,
	// #380).
	return len(tr.Branches) >= 2 || promptWaits(tr) >= waitNotable
}

// classCounts is how many legs a trail has of each class.
func classCounts(tr journey.Trail) map[journey.Class]int {
	out := map[journey.Class]int{}
	for _, l := range tr.Legs {
		out[l.Class]++
	}
	return out
}

// blockRows is the block's rows for a trail: a row per class with more
// than one leg, the lanes where there are two or more, and the wait on
// you where it is worth a row — the counts, never a row the trail beneath
// already draws (#377). The board's columns draw this, closed.
func blockRows(tr journey.Trail) []summaryRow {
	return blockRowsOpen(tr, nil, nil, 0)
}

// blockRowsOpen is the block with the groups in open opened into the
// rows they count (#393): a class's legs hung beneath it, oldest first,
// the lanes beneath the lanes row with what each came back with, or
// what a lane still out says of itself. A class of one and a lone lane
// stay the trail's own rows, open or not; the question HEAD is asking
// is the trail's too.
func blockRowsOpen(tr journey.Trail, open map[string]bool, lanes map[int]laneLine, findings int) []summaryRow {
	if !blockCounts(tr) {
		return nil
	}
	wait := ""
	if d := promptWaits(tr); d >= waitNotable {
		wait = spanText(d)
	}
	var out []summaryRow
	for _, r := range summaryRows(tr, open, nil, lanes, wait, findings) {
		switch {
		case r.kind == "ask", r.kind == "leg" && r.solo:
			continue // a class of one: the trail's own row
		case r.key == summaryLanes && len(tr.Branches) < 2:
			continue // one lane: the trail's own lane row
		}
		out = append(out, r)
	}
	return out
}

// blockHeight is how many rows the block spends above a trail: its rows
// and the seam under them (#377, #378).
func blockHeight(tr journey.Trail) int {
	if n := len(blockRows(tr)); n > 0 {
		return n + 1
	}
	return 0
}

// blockSaid is what the frame already says about a trail beside or
// above its block, so a row does not say it twice (#347, #356, #357):
// the loop, the running leg's `for` clause, and the lanes' tally.
type blockSaid struct {
	loop, live, back, out bool
}

// blockLines draws a trail's block w cells wide on its own head options:
// the rows the legs' summary draws, the running class's clause standing
// down where HEAD's own row is drawn beneath or the frame says it, the
// loop and the lanes' tally likewise (#347, #351, #356, #357, #377).
func blockLines(tr journey.Trail, o TrailOpts, w int, headDrawn bool, said blockSaid) []string {
	return blockLinesIn(tr, o, w, headDrawn, said, blockView{cursor: -1})
}

// blockView is what the trail column adds to the block the cursor can
// enter (#393): its rows with the open groups opened, the row the cursor
// stands on, and the window where the opened rows outrun the room — top
// is the first row drawn and cap how many the block may spend, the seam
// aside. A zero view is the board's: the closed rows, no cursor, all of
// them drawn.
type blockView struct {
	rows   []summaryRow
	cursor int
	top    int
	cap    int
}

// blockLinesIn draws the block through a view: an open class's legs and
// the lanes' lanes hang on the waypoint rail beneath their row, as #348
// hung them, so the group reads as one; the cursor's row is marked as the
// trail marks its own; a window that hides rows says so on its edges.
func blockLinesIn(tr journey.Trail, o TrailOpts, w int, headDrawn bool, said blockSaid, v blockView) []string {
	rows := v.rows
	if rows == nil {
		rows = blockRows(tr)
	}
	if len(rows) == 0 {
		return nil
	}
	top, end := 0, len(rows)
	head, tail := false, false
	if v.cap > 0 && len(rows) > v.cap {
		top = clampScroll(v.top, len(rows), v.cap)
		end = top + v.cap
		head, tail = top > 0, end < len(rows)
	}
	var lines []string
	for i := top; i < end; i++ {
		r := rows[i]
		var line string
		switch r.kind {
		case "class":
			// An open class whose running leg is drawn beneath it yields
			// its clause to that row, as it yields to HEAD's row on the
			// trail (#355, #377).
			under := headDrawn
			for j := i + 1; !under && j < end && rows[j].key == r.key && rows[j].kind == "leg"; j++ {
				under = tr.Legs[rows[j].leg].Current
			}
			line = summaryClassRow(tr, r.class, o, said.loop, under, said.live, w)
		case "wait":
			line = summaryFigureRow(dimStyle.Render("◉ waited"), "on you · "+plural(ownPrompts(tr), "prompt"), nil, r.text, w)
		case "lanes":
			line = summaryLanesRow(tr, o, w, said.back, said.out)
		case "leg":
			l := tr.Legs[r.leg]
			lo := o
			lo.Width = w - trailWayWidth
			// The trail sets the ask before it draws a leg, and a ship
			// row reads it (#189, #192, #350).
			lo.Ask = askBefore(tr, l.Start)
			label, narrated := legLabel(l, lo)
			line = summaryHang(rows, i) + legRow(l, label, narrated, lo)
		case "lane":
			lo := o
			lo.Width = w - trailWayWidth
			line = summaryHang(rows, i) + summaryLaneRow(tr.Branches[r.lane], lo, w-trailWayWidth)
		case "report":
			// The cut is the width of the rail the row draws, and a
			// clock keeps the edge, as the trail keeps it (#352, #353).
			hang := summaryHang(rows, i)
			body := w - lipgloss.Width(hang)
			text := r.text
			if r.clock != "" {
				if keep := body - len([]rune(r.clock)) - 2; keep >= trailMinLabel {
					text = pad(clip(text, keep), keep) + "  " + r.clock
				}
			}
			line = hang + dimStyle.Render(clip(text, body))
		}
		if i == v.cursor {
			line = summaryCursored(line, w)
		}
		lines = append(lines, line)
	}
	if head {
		// The edge takes the window's first row, never the cursor's:
		// the window is placed with the cursor inside its edges.
		lines[0] = dimStyle.Render(clip(fmt.Sprintf("↑ %d more above", top+1), w))
	}
	if tail {
		lines[len(lines)-1] = dimStyle.Render(clip(fmt.Sprintf("▾ %d more below", len(rows)-end+1), w))
	}
	return append(lines, seamRule(w)) // the block ends on its seam (#378)
}

// summaryHang is the rail a row under a group hangs on: `│  ├ ` for a leg
// or a lane with another beneath it, `│  └ ` for the last; a finding's
// line hangs one rail further in, under its lane.
func summaryHang(rows []summaryRow, i int) string {
	r := rows[i]
	mark := wayTee
	if i+1 >= len(rows) || rows[i+1].group() || rows[i+1].kind == "wait" {
		mark = wayEnd
	}
	if r.kind == "report" {
		if i+1 >= len(rows) || rows[i+1].kind != "report" {
			mark = wayEnd
		}
		return ruleStyle.Render(railStroke+"  "+railStroke+" "+mark) + " "
	}
	return ruleStyle.Render(railStroke+"  "+mark) + " "
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

// summaryLaneRow is a lane's row under the lanes row: its name, its
// link count, and its clock in the trail's own words — `⋯ 20m out`, `✓
// 7m ago`, `⌀ 1h ago` for one back with nothing to say (#348).
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

// summaryWindow places a window of h rows over total so the cursor's row
// is inside it, edges and all: a cut window spends its first row on `↑
// N more above` and its last on `▾ N more below`, and the cursor never
// stands on an edge's words. scroll is where the window stood.
func summaryWindow(total, h, cursor, scroll int) (top int, head, tail bool) {
	if total <= h {
		return 0, false, false
	}
	top = clampScroll(scroll, total, h)
	for range 4 {
		head = top > 0
		tail = top+h < total
		lo, hi := top, top+h-1
		if head {
			lo++
		}
		if tail {
			hi--
		}
		switch {
		case cursor < 0:
			return top, head, tail
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

// lanesClock is the clock the lanes row would wear: the oldest lane
// still out, or the last one back.
func lanesClock(tr journey.Trail, now time.Time) string {
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
	switch {
	case !oldest.IsZero():
		return relAge(now, oldest) + " out"
	case !latest.IsZero():
		return relAge(now, latest) + " ago"
	}
	return ""
}

// blockSaysOut reports whether the frame already carries that clock, in
// either idiom the fleet writes it in: "◈3 out 20m" or "⋯ 20m out".
func blockSaysOut(tr journey.Trail, now time.Time, lines ...string) bool {
	age := lanesClock(tr, now)
	if age == "" {
		return false
	}
	clock, word := strings.Fields(age)[0], strings.Fields(age)[1]
	for _, l := range lines {
		t := ansi.Strip(l)
		if word == "ago" {
			// The lane that came back last wears `✓ 1h ago` on its own
			// row, and the fold and the seam rows say it too: one clock,
			// said once (#22, #64, #379).
			if strings.Contains(t, clock+" ago") {
				return true
			}
			continue
		}
		if strings.Contains(t, clock+" out") || strings.Contains(t, "out "+clock) {
			return true
		}
	}
	return false
}

// blockShown says whether the trail column draws a block for the
// selected trail on this frame: the trail counts, and the reply box does
// not cover every row of it — the title and the card stand down for
// rows the frame draws, not rows it paints over (#17, #63, #380).
func (m *Model) blockShown() bool {
	if !blockCounts(m.trail) {
		return false
	}
	if m.replyBox.on {
		top := trailChrome
		if m.sessionView() {
			top = 3
		}
		for i := top; i < top+m.blockHeightHere(); i++ {
			if !m.boxCoversRow(i) {
				return true
			}
		}
		return false
	}
	return true
}

// trailBlock is the selected trail's block, for the trail column, over
// the trail rows drawn beneath it: the card above or the fleet row
// beside may already say the loop, the running leg's clause and the
// lanes' tally (#356, #357), and HEAD's own row says the clause where the
// viewport draws it (#351, #377).
func (m *Model) trailBlock(w int, below []string) []string {
	if !m.blockShown() {
		return nil
	}
	o := m.trailOpts(w, 1)
	said := blockSaid{loop: m.summaryLoopSaid(w), live: m.summaryLiveSaid(w), back: m.summaryBackSaid(w)}
	beside := []string{m.cardSecond(w)}
	if fw, _, _ := m.layout(m.width - 2*edgePad); fw > 0 {
		if s, ok := m.selected(); ok {
			beside = append(beside, m.secondLineUnder(s, fw-4, ""))
		}
	}
	said.out = blockSaysOut(m.trail, m.now, append(beside, below...)...)
	rows := m.blockRowsHere()
	v := blockView{rows: rows, cursor: m.blockCursorRow(rows), cap: m.blockCap()}
	v.top, _, _ = summaryWindow(len(rows), v.cap, v.cursor, m.blockScroll)
	m.blockScroll = v.top // the window is where the cursor left it
	lines := blockLinesIn(m.trail, o, w, headSaysLive(m.trail, m.now, o, below), said, v)
	if m.sessionView() && len(lines) > 0 {
		// Under the card the block opens on its own seam: the card's last
		// row leaves no air, and `◆ scout  11 legs` read as more card.
		// At Lv1 the title's air row stands above it, and on the board
		// the column header ends on its rung, so the seam is this
		// frame's alone (#393).
		lines = append([]string{countsRule(w)}, lines...)
	}
	return lines
}

// blockSeams is how many rows the block spends on seams: the one under it
// (#378), and the one over it in the session view (#393).
func (m *Model) blockSeams() int {
	if m.sessionView() {
		return 2
	}
	return 1
}

// headSaysLive reports whether the rows drawn beneath a block carry the
// running leg's own clause — `for 39m`, or the figure HEAD wears when it
// is stuck or waiting, `silent 4m`, `waiting 7m` — so the class row yields
// it, as it yields to the card (#351, #356, #377, #380).
func headSaysLive(tr journey.Trail, now time.Time, o TrailOpts, below []string) bool {
	for _, l := range tr.Legs {
		if !l.Current {
			continue
		}
		wants := []string{"for " + relAge(now, l.Start)}
		if _, fig := headMark(o, l); fig != "" {
			wants = append(wants, fig)
		}
		for _, want := range wants {
			for _, line := range below {
				if strings.Contains(ansi.Strip(line), want) {
					return true
				}
			}
		}
	}
	return false
}

// columnBlock is a board column's block: the column's header says the
// loop and the lanes' tally where it does, and HEAD's own row beneath
// says the running leg's clause where the column draws it (#376, #377).
func (m *Model) columnBlock(key string, tr journey.Trail, s fleet.Session, o TrailOpts, header, below []string, w int) []string {
	if !blockCounts(tr) {
		return nil
	}
	so := o
	so.HeadWaits = headWaits(tr)
	so.HeadTail = headTail(tr, m.now, s.Snap.State != state.Idle, m.agentsFor(key))
	text := ""
	for _, line := range header {
		text += ansi.Strip(line) + "\n"
	}
	said := blockSaid{loop: strings.Contains(text, " failure"), back: strings.Contains(text, " back")}
	said.out = blockSaysOut(tr, m.now, append([]string{text}, below...)...)
	for _, l := range tr.Legs {
		if l.Current {
			said.live = strings.Contains(text, "for "+relAge(m.now, l.Start))
		}
	}
	return blockLines(tr, so, w, headSaysLive(tr, m.now, so, below), said)
}

// The block's cursor (#393). The counts above a trail open into the rows
// they count: `k` off the trail's first row climbs into the block, `j`
// off its last row is the trail's first, `space` on a class or the lanes
// opens it into its legs or lanes and on a row under it folds the group.
// A leg or lane row in the block is that leg or lane — the reader
// follows it, `tab` reads it, `enter` attaches — so every key does at a
// block row what it does at a trail row. The board's columns keep the
// counts closed: the open set is the session view's, per session.

// blockTrailFloor is the fewest trail rows an open block leaves beneath
// the seam: past it the block's own rows are windowed instead.
const blockTrailFloor = 4

// openHere is the selected session's open set: nil while nothing is.
func (m *Model) openHere() map[string]bool {
	return m.blockOpen[m.selectedKey]
}

// setOpen opens or closes a group of the selected session's block.
func (m *Model) setOpen(key string, on bool) {
	if m.blockOpen == nil {
		m.blockOpen = map[string]map[string]bool{}
	}
	set := m.blockOpen[m.selectedKey]
	if set == nil {
		set = map[string]bool{}
		m.blockOpen[m.selectedKey] = set
	}
	if on {
		set[key] = true
	} else {
		delete(set, key)
	}
}

// blockRowsHere is the selected trail's block as the trail column draws
// it: the open groups opened, a lane still out saying what its own file
// says, a finding wrapped to the rail it hangs on.
func (m *Model) blockRowsHere() []summaryRow {
	open := m.openHere()
	if len(open) == 0 {
		return blockRows(m.trail)
	}
	w := m.trailBoxWidth()
	return blockRowsOpen(m.trail, open, m.blockLaneLines(), w-trailWayWidth-2)
}

// blockLaneLines is what each lane still out says of itself — its own
// file's last line and clock, as the trail hangs it under the lane — by
// index into the trail's branches (#352).
func (m *Model) blockLaneLines() map[int]laneLine {
	out := map[int]laneLine{}
	if s, ok := m.selected(); !ok || !s.Live || s.Snap.State == state.Idle {
		return out
	}
	agents := m.agentsFor(m.selectedKey)
	for i, br := range m.trail.Branches {
		if br.Done {
			continue
		}
		live, known := agents[br.ToolUseID]
		if g, text, clock := laneHead(live, br, known, m.now); text != "" {
			out[i] = laneLine{text: g + " " + text, clock: clock}
		}
	}
	return out
}

// blockCap is how many rows the block may spend on this frame, the seam
// aside: the trail keeps its floor beneath, and the closed block, which
// the frame always drew whole, is never cut shorter than it stood.
func (m *Model) blockCap() int {
	h := m.height
	if h <= 0 {
		h = 24
	}
	cap := h - 5 - trailChrome - m.blockSeams() - blockTrailFloor
	if closed := len(blockRows(m.trail)); cap < closed {
		cap = closed
	}
	return cap
}

// blockHeightHere is how many rows the block spends above the selected
// trail: its rows, windowed where the open groups outrun the room, and
// the seam under them (#377, #378, #393).
func (m *Model) blockHeightHere() int {
	n := len(m.blockRowsHere())
	if n == 0 {
		return 0
	}
	return min(n, m.blockCap()) + m.blockSeams()
}

// inBlock says whether the cursor is in the block: on the legs, on the
// session the cursor was put there on, over a trail that counts.
func (m *Model) inBlock() bool {
	return m.level >= levelWaypoints && m.blockCursor >= 0 && m.blockOn == m.selectedKey && blockCounts(m.trail)
}

// blockCursorRow is the block row the cursor stands on, held to a row it
// can stand on: -1 while the cursor is the trail's.
func (m *Model) blockCursorRow(rows []summaryRow) int {
	if !m.inBlock() {
		return -1
	}
	m.blockClamp(rows)
	return m.blockCursor
}

// blockClamp holds the block cursor to a standing row: a trail that grew
// a class or lost one moves the rows under it.
func (m *Model) blockClamp(rows []summaryRow) {
	if len(rows) == 0 {
		m.blockCursor = -1
		return
	}
	c := min(max(m.blockCursor, 0), len(rows)-1)
	if !rows[c].stands() {
		if up := blockStep(rows, c, -1); up >= 0 {
			c = up
		} else if down := blockStep(rows, c, 1); down >= 0 {
			c = down
		}
	}
	m.blockCursor = c
}

// blockStep is the next standing row from i in the direction of delta's
// sign, |delta| standing rows on, or as far as the block goes; -1 where
// no standing row lies that way at all.
func blockStep(rows []summaryRow, i, delta int) int {
	step, n := 1, delta
	if delta < 0 {
		step, n = -1, -delta
	}
	at := -1
	for j := i + step; j >= 0 && j < len(rows) && n > 0; j += step {
		if rows[j].stands() {
			at, n = j, n-1
		}
	}
	return at
}

// blockEnter puts the cursor on the block's last standing row: the way
// in is `k` off the trail's first row, so the first row it lands on is
// the one nearest the trail.
func (m *Model) blockEnter(rows []summaryRow) bool {
	last := -1
	for i := range rows {
		if rows[i].stands() {
			last = i
		}
	}
	if last < 0 {
		return false
	}
	m.blockCursor, m.blockOn = last, m.selectedKey
	m.anchorReader()
	return true
}

// blockLeave puts the cursor back on the trail, at its first row: `j`
// off the block's last row.
func (m *Model) blockLeave() {
	m.cursor = 0
	m.cursorMove(0) // records the block row left, and clears the block cursor
}

// blockJumpWord is what `s` does here: `counts` on a trail that draws a
// block, `trail` in the block, "" where the key refuses.
func (m *Model) blockJumpWord() string {
	switch {
	case m.level != levelWaypoints || m.showHelp || m.searching || m.replying:
		return ""
	case m.inBlock():
		return "trail"
	case m.blockShown():
		return "counts"
	}
	return ""
}

// blockFoldWord is what `space` does on the cursor's block row: `open` a
// closed group, `close` an open one or the group a row under it belongs
// to.
func (m *Model) blockFoldWord() string {
	rows := m.blockRowsHere()
	c := m.blockCursorRow(rows)
	if c < 0 {
		return ""
	}
	if rows[c].group() && !m.openHere()[rows[c].key] {
		return "open"
	}
	return "close"
}

// blockJump is `s` on the trail: the cursor to the counts, on the row it
// last stood on there, else the block's last row. False where the trail
// draws no block.
func (m *Model) blockJump() bool {
	if !m.blockShown() {
		return false
	}
	rows := m.blockRowsHere()
	if m.blockOn == m.selectedKey && m.blockRest >= 0 && m.blockRest < len(rows) {
		m.blockCursor = m.blockRest
		m.blockClamp(rows)
		m.anchorReader()
		return true
	}
	return m.blockEnter(rows)
}

// blockKey is the Lv2 keys while the cursor is in the block, and the
// three that take it there: `k` off the trail's first row, `s` from any
// row of the trail, and `space`, which on the trail says where it acts.
// The page key stops at the trail's first row as it always did — the
// walkthrough's `at the start` — and `k`, the step, is the way up. True
// when the key was the block's.
func (m *Model) blockKey(key string) bool {
	if !m.inBlock() {
		switch key {
		case "k", "up":
			if m.cursor == 0 && m.blockShown() && m.blockEnter(m.blockRowsHere()) {
				return true
			}
		case "s":
			if !m.blockJump() {
				m.note = "nothing to count" // no class with two legs: the trail says all there is (#349)
			}
			return true
		case " ", "space":
			if m.blockShown() {
				m.note = "the counts open · k up to them"
			} else {
				m.note = "nothing to open" // a trail with nothing to count draws no block
			}
			return true
		}
		return false
	}
	rows := m.blockRowsHere()
	m.blockClamp(rows)
	if m.blockCursor < 0 {
		return false
	}
	row := rows[m.blockCursor]
	switch key {
	case "j", "down":
		if next := blockStep(rows, m.blockCursor, 1); next >= 0 {
			m.blockCursor = next
			m.anchorReader()
		} else {
			m.blockLeave()
		}
		return true
	case "k", "up":
		if prev := blockStep(rows, m.blockCursor, -1); prev >= 0 {
			m.blockCursor = prev
			m.anchorReader()
		} else {
			m.note = "at the start"
		}
		return true
	case "ctrl+d":
		// Half a page of rows, as on the trail; off the block's end it
		// is the trail's first row, as `j` is.
		if blockStep(rows, m.blockCursor, 1) < 0 {
			m.blockLeave()
			return true
		}
		next := blockStep(rows, m.blockCursor, m.trailHalfPage())
		for n := m.trailHalfPage() - 1; next < 0 && n > 0; n-- {
			next = blockStep(rows, m.blockCursor, n)
		}
		m.blockCursor = next
		m.anchorReader()
		return true
	case "ctrl+u":
		prev := -1
		for n := m.trailHalfPage(); prev < 0 && n > 0; n-- {
			prev = blockStep(rows, m.blockCursor, -n)
		}
		if prev < 0 {
			m.note = "at the start"
			return true
		}
		m.blockCursor = prev
		m.anchorReader()
		return true
	case "G":
		// The present is the trail's newest row, from the block as from
		// anywhere on the trail.
		m.cursorToPresent()
		return true
	case "s":
		// Back to the trail, on the row the cursor left: `k` off the
		// first row came from the first row, `s` from wherever it was.
		m.cursorMove(0)
		return true
	case "]":
		// The block's chapters are its groups: the next class or the
		// lanes, past an open group's rows. Off the last group the next
		// chapter is the trail's first prompt (#393).
		for j := m.blockCursor + 1; j < len(rows); j++ {
			if rows[j].group() {
				m.blockCursor = j
				m.anchorReader()
				return true
			}
		}
		m.blockLeave()
		m.chapterFirst()
		return true
	case "[":
		// The previous group; from a row under an open group, the group
		// it is under. The trail's own `[` keeps its refusal at the first
		// prompt: the chapter key's question is prompts (#161).
		for j := m.blockCursor - 1; j >= 0; j-- {
			if rows[j].group() {
				m.blockCursor = j
				m.anchorReader()
				return true
			}
		}
		m.note = "at the start"
		return true
	case " ", "space":
		switch {
		case row.group():
			on := !m.openHere()[row.key]
			m.setOpen(row.key, on)
			if on {
				m.blockScroll = m.blockCursor // the group opens framed: its row first, its rows filling the window
			}
		default:
			// A row under a group folds the group and stands on it.
			m.setOpen(row.key, false)
			m.blockCursor = summaryGroupRow(rows, m.blockCursor)
		}
		m.anchorReader()
		return true
	}
	return false
}
