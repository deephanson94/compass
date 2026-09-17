package ui

import (
	"fmt"
	"strings"
	"time"

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
// already draws (#377).
func blockRows(tr journey.Trail) []summaryRow {
	if !blockCounts(tr) {
		return nil
	}
	wait := ""
	if d := promptWaits(tr); d >= waitNotable {
		wait = spanText(d)
	}
	var out []summaryRow
	for _, r := range summaryRows(tr, nil, nil, nil, wait, 0) {
		switch {
		case r.kind == "leg", r.kind == "ask":
			continue // a class of one: the trail's own row
		case r.kind == "lanes" && len(tr.Branches) < 2:
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
	var lines []string
	rows := blockRows(tr)
	if len(rows) == 0 {
		return nil
	}
	for _, r := range rows {
		switch r.kind {
		case "class":
			lines = append(lines, summaryClassRow(tr, r.class, o, said.loop, headDrawn, said.live, w))
		case "wait":
			lines = append(lines, summaryFigureRow(dimStyle.Render("◉ waited"), "on you · "+plural(len(tr.Prompts), "prompt"), nil, r.text, w))
		case "lanes":
			lines = append(lines, summaryLanesRow(tr, o, w, said.back, said.out))
		case "leg":
			l := tr.Legs[r.leg]
			lo := o
			lo.Width = w
			lo.Ask = askBefore(tr, l.Start)
			label, narrated := legLabel(l, lo)
			lines = append(lines, legRow(l, label, narrated, lo))
		}
	}
	return append(lines, seamRule(w)) // the block ends on its seam (#378)
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
		for i := top; i < top+blockHeight(m.trail); i++ {
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
	return blockLines(m.trail, o, w, headSaysLive(m.trail, m.now, o, below), said)
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
