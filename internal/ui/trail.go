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
	"github.com/deephanson94/compass/internal/todo"
)

// The trail's vocabulary (SPEC §2.1). Every mark is a distinct silhouette, so
// the graph reads with the colour switched off.
const (
	glyphPrompt = "◉" // the human turn the journey started from
	glyphLeg    = "◆" // a closed leg
	glyphHead   = "●" // HEAD — the leg in progress
	glyphBreath = "◐" // HEAD on the breath's off-beat (SPEC §4's one animation)
	glyphBranch = "◈" // a subagent lane
	glyphGhost  = "◌" // planned work: a todo that has not happened yet
	railStroke  = "│" // rail between two nodes
	railHead    = "╷" // the rail's cap: the journey starts here, under the prompt
	railGhost   = "┊" // the dashed rail, below HEAD, into the future
	railFork    = "├─"
	branchOpen  = "⋯" // the branch is still out there
	branchDone  = "✓" // it came back
	branchEmpty = "⌀" // it came back with nothing to say
	wayTee      = "├" // a waypoint under its leg
	wayEnd      = "└" // the last one
	wayFail     = "✗" // a failing test
)

// Column geometry: the class verb sits in its own column so the glyphs, the
// verbs and the labels each line up vertically, and the ages hold the right
// margin (SPEC §4).
const (
	trailVerbWidth   = 6                  // "design" is the longest verb
	trailPrefixWidth = 3 + trailVerbWidth // glyph, space, verb column, space
	trailForkWidth   = 4                  // "├─◈ "
	trailWayWidth    = 5                  // "│  ├ "
	trailGhostWidth  = 2                  // "◌ "
	trailMinLabel    = 6                  // below this a label says nothing
	maxGhosts        = 6                  // the plan shows its next six moves, room allowing
)

// TrailOpts is everything the trail needs beyond the journey itself: the plan
// ahead of it, the narrated labels over it, the clock the ages are relative to,
// the block it has to fill, how deep the zoom is and where the cursor sits.
type TrailOpts struct {
	Todos  []todo.Item
	Labels map[string]string // narrated overlay, keyed by narrator LegKey; nil ok

	// SessionKey names whose journey this is — the session's Key(), the same
	// string the narrator was asked under. The Labels map is keyed by the
	// narrator's LegKey — session, leg start and class — so the renderer needs
	// the session to look a leg up. "" simply finds nothing.
	SessionKey string

	// Head names HEAD when the caller knows better than the trail what the
	// session is doing this minute: the call it is hung on, the question it
	// is asking. "" leaves HEAD to the plan and then to the leg's own label.
	Head string

	// HeadState and HeadSince are the session's state as the state machine
	// sees it, so HEAD can wear the fleet's glyph — ◍ for a hung session, ▲
	// for one waiting on you — and say "silent 4m" or "waiting 4m" instead
	// of "for 13m". A green ● on a stuck session's HEAD was the trail
	// contradicting the fleet beside it.
	HeadState state.State
	HeadSince time.Time

	// HeadWaits is how many agents HEAD has out with nothing of its own
	// written since it sent them: a parent parked on its children, which
	// "● build … for 2h" over three ⋯ lanes could not distinguish from a
	// parent working. HeadTail is HEAD's figure when it is working — "for
	// 2h", "◈3 out 20m · for 2h", "◈3 out 20m · quiet 15m" — the same
	// words the fleet row uses. trailDoc sets both from the trail.
	HeadWaits int
	HeadTail  string

	// LaneLinks maps a lane's label to the board number of a live session
	// whose first prompt begins with it: a teammate that looks like this
	// agent's, hedged as "→3" because the transcript carries no real link.
	LaneLinks map[string]int

	// NoInline keeps a leg's detail off its row whatever the width: for
	// asking what Lv2 would hang beneath the legs, not for drawing.
	NoInline bool

	// Dense drops the plain rail rows between legs. trailDoc sets it itself
	// when a Lv1 trail does not fit its viewport: under pressure the rail
	// gives up its air and twice as many legs fit, while the rows that say
	// something — forks, ticks, hour rules — keep theirs.
	Dense bool

	Now           time.Time
	Width, Height int
	Level         int  // 1, 2 or 3 (the trail renders identically at 2 and 3)
	Cursor        int  // index into TrailRows at Lv2+; -1 = no cursor
	Pulse         bool // the breath's off-beat: HEAD draws ◐ instead of ●

	// Scroll is the first document row the viewport draws, clamped to the
	// document (TrailLines). Pinned overrides it with the last screenful — the
	// resting state, so a journey that grows keeps its newest row on screen
	// without anybody pressing a key. A zero TrailOpts is not pinned: the panel
	// that owns the viewport says so, once, when it is created.
	Scroll int
	Pinned bool
}

// RenderTrail draws the trail into a width×height block: the journey's start at
// the top, time running downward, one line per node, the rail down the left.
// Level 1 is the graph of legs; level 2 unfolds each leg's waypoints beneath it.
// Pending todos hang below HEAD as ghosts at either level — the future is
// further down. The frame is exactly height lines, and it is a viewport onto
// TrailLines: nothing is dropped to make the trail fit, because everything can
// be scrolled to.
func RenderTrail(tr journey.Trail, o TrailOpts) string {
	return strings.Join(fit(trailRows(tr, o), o.Height), "\n")
}

// TrailLines returns the trail's full document — every row, uncropped, in the
// order it is drawn: the opening prompt, the legs as they happened, HEAD last
// of what has happened, and the plan below it. The panel is a window onto this,
// so the caller can measure it, scroll it and keep a cursor inside it.
func TrailLines(tr journey.Trail, o TrailOpts) []string {
	doc, _ := trailDoc(tr, o)
	return doc
}

// TrailCursorRow is where the cursor stands in that document: the index into
// TrailLines of the row TrailOpts.Cursor selects, so a caller can scroll far
// enough to keep it on screen. -1 means the cursor has no row of its own —
// no cursor, a cursor past the end, or a panel too narrow to draw its row.
func TrailCursorRow(tr journey.Trail, o TrailOpts) int {
	cursor := trailCursor(o)
	if cursor < 0 {
		return -1
	}
	_, sel := trailDoc(tr, o)
	for i, s := range sel {
		if s == cursor {
			return i
		}
	}
	return -1
}

// trailRows is the viewport RenderTrail draws, without the bottom padding, so a
// column that wants to know where its content stops can ask.
func trailRows(tr journey.Trail, o TrailOpts) []string {
	doc, _ := trailDoc(tr, o)
	top := trailTop(len(doc), o)
	end := top + o.Height
	if end > len(doc) {
		end = len(doc)
	}
	if end < top {
		end = top
	}
	// A detail row never leads the viewport: "│  └ ✗ test_logout" under
	// the title with its leg above the fold is a child with no parent. A
	// scrolled viewport opens on the parent instead, child and all; a
	// pinned one, which cannot give up its last row, puts the parent's
	// own row where the child was.
	// The parent's own row takes the top row's place, dim, so the child
	// beneath it has a name. (Opening the viewport on the parent instead
	// would move every offset by a row, and the offsets are a contract:
	// each draws exactly its slice, and the cursor's row is where
	// TrailCursorRow says.)
	rows := append([]string(nil), doc[top:end]...)
	if len(rows) > 0 && top > 0 && isDetailRow(rows[0]) {
		parent := top - 1
		for parent > 0 && isDetailRow(doc[parent]) {
			parent--
		}
		rows[0] = dimStyle.Render(ansi.Strip(doc[parent]))
	}
	return rows
}

// isDetailRow recognises a Lv2 child row by its hanger: "│  ├", "│  └" or
// their unrailed forms under HEAD.
func isDetailRow(line string) bool {
	plain := ansi.Strip(line)
	return strings.HasPrefix(plain, "│  ├") || strings.HasPrefix(plain, "│  └") ||
		strings.HasPrefix(plain, "   ├") || strings.HasPrefix(plain, "   └")
}

// trailTop resolves the viewport's first row: the last screenful while pinned,
// otherwise Scroll — either way held inside the document. A trail shorter than
// the panel has nothing to pin: it starts at the top, and the room below it is
// the future's.
func trailTop(total int, o TrailOpts) int {
	height := o.Height
	if height < 1 {
		height = 1
	}
	if o.Pinned {
		return clampScroll(total, total, height)
	}
	return clampScroll(o.Scroll, total, height)
}

// trailCursor is the selection the renderer honours: the cursor only exists
// from Lv2 down, where the waypoints it can land on are drawn.
func trailCursor(o TrailOpts) int {
	if o.Level >= levelWaypoints {
		return o.Cursor
	}
	return -1
}

// trailDoc builds the whole document once, and beside it the selectable row
// each line carries (-1 where a line is not one) — the two things every
// question about the viewport needs.
func trailDoc(tr journey.Trail, o TrailOpts) ([]string, []int) {
	width := o.Width
	if width < trailPrefixWidth {
		return nil, nil
	}
	nodes := trailNodes(tr)
	if len(nodes) == 0 {
		rows := trailEmptyRows(width)
		return rows, noSel(len(rows))
	}

	o.HeadWaits = headWaits(tr)
	o.HeadTail = headTail(tr, o.Now, o.HeadState != state.Idle)
	b := trailBuilder{cursor: trailCursor(o), width: width, dense: o.Dense}
	b.journey(tr, nodes, o)
	if len(tr.Legs) == 0 {
		// A journey that has only been asked for: say what comes next rather
		// than leaving the panel half empty (SPEC §4).
		b.node("")
		b.node(dimStyle.Render(clip("scouting will appear here", width)))
	}
	b.ghosts(o.Todos, width, o.Height)
	doc, sel := b.render()
	if !o.Dense && o.Height > 0 && len(doc) > o.Height {
		// Too long for the panel: draw it again without the air, so the
		// viewport holds twice the journey.
		o.Dense = true
		return trailDoc(tr, o)
	}
	return doc, sel
}

// noSel is the selection map of a block no cursor can land on.
func noSel(n int) []int {
	sel := make([]int, n)
	for i := range sel {
		sel[i] = -1
	}
	return sel
}

// trailEmptyRows is the designed empty state: never a blank panel (SPEC §4).
func trailEmptyRows(width int) []string {
	return []string{
		dimStyle.Render(clip("◌ nothing yet", width)),
		"",
		dimStyle.Render(clip("scouting will appear here", width)),
	}
}

// trailLine is one row of the trail. A detail row — a Lv2 waypoint, a touched
// list, a subagent's report — belongs to a group (the node it hangs under) and
// is drawn with the `├`/`└` its position in that group earns.
type trailLine struct {
	text  string // a node row's finished line, or a detail row's body
	group int    // detail rows: which group they belong to; -1 for node rows
	sel   int    // index into TrailRows, or -1 when the row is not selectable
}

// detailRow is one Lv2 body row before it is hung off its node: its text, and
// whether the cursor may land on it.
type detailRow struct {
	text string
	sel  int // index into TrailRows, or -1
}

// trailBuilder assembles the rows top-down, in the order time ran, keeping each
// detail group together so the last row of one can close it with `└`.
type trailBuilder struct {
	lines  []trailLine
	groups int

	cursor int  // the selectable row to invert; -1 for none
	picked int  // how many selectable rows have been handed out so far
	width  int  // the panel's columns, so the cursor's bar spans all of them
	dense  bool // no air between rows: the ghosts drop their rails too
}

// pick hands out the next selectable row index. It is called wherever a
// selectable row belongs, drawn or not, so the enumeration TrailRows returns
// and the rows RenderTrail draws never drift apart on a narrow panel.
func (b *trailBuilder) pick() int {
	b.picked++
	return b.picked - 1
}

func (b *trailBuilder) node(text string) {
	b.lines = append(b.lines, trailLine{text: text, group: -1, sel: -1})
}

// selNode is a node row the cursor can land on.
func (b *trailBuilder) selNode(sel int, text string) {
	b.lines = append(b.lines, trailLine{text: text, group: -1, sel: sel})
}

// details opens a new group and hangs the bodies off it, in order.
func (b *trailBuilder) details(bodies []detailRow) {
	if len(bodies) == 0 {
		return
	}
	b.groups++
	for _, body := range bodies {
		b.lines = append(b.lines, trailLine{text: body.text, group: b.groups, sel: body.sel})
	}
}

// render turns the lines into the document and its selection map. Nothing is
// dropped — the panel scrolls instead (M7 contract) — so every row the builder
// laid down is drawn, and the last of each detail group closes it with `└`.
func (b *trailBuilder) render() ([]string, []int) {
	out := make([]string, 0, len(b.lines))
	sel := make([]int, 0, len(b.lines))
	for i, l := range b.lines {
		if l.group < 0 {
			out = append(out, b.cursored(l, l.text))
		} else {
			mark := wayTee
			if b.lastOfGroup(i) {
				mark = wayEnd
			}
			out = append(out, b.cursored(l, ruleStyle.Render(railStroke+"  "+mark)+" "+l.text))
		}
		sel = append(sel, l.sel)
	}
	return out, sel
}

// cursored inverts the row the Lv2 cursor stands on. The row is stripped back
// to its plain text first: a reset left over from the class tint would cancel
// the inversion halfway across the line. Inversion is the whole mark — it costs
// no colour and it survives NO_COLOR (SPEC §4).
//
// The bar spans the whole panel rather than the row's own text. Trail rows run
// from three characters to thirty, and a cursor cut to each one's length reads
// as ragged debris; a full-width bar is one shape the eye can follow down the
// column.
func (b *trailBuilder) cursored(l trailLine, text string) string {
	if b.cursor < 0 || l.sel != b.cursor {
		return text
	}
	plain := strings.TrimRight(ansi.Strip(text), " ")
	// A shape as well as the inversion: the first cell of the row becomes
	// the cursor mark, so the cursor exists in a capture, over NO_COLOR, and
	// to anyone whose terminal renders reverse video faintly. Every row
	// opens with a glyph or a rail stroke whose meaning the rest of the row
	// repeats in words, so the cell is the cheapest one to spend.
	if r := []rune(plain); len(r) > 1 && r[1] == ' ' {
		r[1] = '▸' // after the glyph: "◉▸" keeps the row's shape
		plain = string(r)
	} else if len(r) > 0 {
		r[0] = '▸' // a rail stroke has nothing to keep
		plain = string(r)
	}
	if pad := b.width - lipgloss.Width(plain); pad > 0 {
		plain += strings.Repeat(" ", pad)
	}
	return cursorStyle.Render(plain)
}

// lastOfGroup reports whether row i is the last row of its group. A group's
// rows are laid down together, so its successor is enough to ask.
func (b *trailBuilder) lastOfGroup(i int) bool {
	return i+1 >= len(b.lines) || b.lines[i+1].group != b.lines[i].group
}

// ghosts draws the plan below HEAD: the next pending todo sits nearest the
// present and the rest stack downward into the future, on a dashed rail. Four
// at most; a longer plan says how much more of it there is, in the row that
// closes the rail. The block never takes more than half the panel — the future
// may not crowd out the present, and while the trail is pinned that is what
// keeps HEAD on screen — and a plan with nothing pending draws nothing at all.
func (b *trailBuilder) ghosts(items []todo.Item, width, height int) {
	var pending []string
	for _, it := range items {
		if it.Status == todo.Pending {
			pending = append(pending, it.Text)
		}
	}
	body := width - trailGhostWidth
	if len(pending) == 0 || body < trailMinLabel {
		return
	}

	show := min(maxGhosts, len(pending))
	for show > 0 && ghostCost(show, len(pending), b.dense) > height/2 {
		show--
	}
	if ghostCost(show, len(pending), b.dense) > height/2 {
		return
	}

	// The first dashed rail carries the denominator: three ghosts are three
	// of four or three of seven, and the difference is how far along it is.
	total := 0
	for _, it := range items {
		if it.Status != "deleted" {
			total++
		}
	}
	for i := 0; i < show; i++ {
		rail := ruleStyle.Render(railGhost)
		if i == 0 && total > 0 {
			count := fmt.Sprintf("%d to go", len(pending))
			if done := total - len(pending); done > 0 {
				count += fmt.Sprintf(" · %d done", done)
			}
			rail += " " + dimStyle.Render(clip(count, body))
		}
		if i == 0 || !b.dense {
			b.node(rail)
		}
		b.node(dimStyle.Render(glyphGhost + " " + clip(pending[i], body)))
	}
	if more := len(pending) - show; more > 0 {
		b.node(ruleStyle.Render(railGhost) + " " +
			dimStyle.Render(clip(fmt.Sprintf("+%d more", more), body)))
	}
}

// ghostCost is how many rows show ghosts (out of total pending) occupy: one
// each, one dashed rail each, plus the "+N more" row when the plan is longer.
func ghostCost(show, total int, dense bool) int {
	rows := 2 * show
	if dense && show > 1 {
		rows = show + 1
	}
	if total > show {
		rows++
	}
	return rows
}

// journey lays the graph itself: every node oldest first, its Lv2 detail
// beneath it, the subagents that forked off it, and the rail down to the next.
// The rail is drawn ahead of each node rather than behind it, because the cap
// belongs under the journey's first node — the one row that says "this is where
// it started" now that the start is at the top.
func (b *trailBuilder) journey(tr journey.Trail, nodes []trailNode, o TrailOpts) {
	forked := false
	ticked := false
	long := len(nodes) > 1 && nodes[len(nodes)-1].at.Sub(nodes[0].at) > longTrailSpan
	for i, n := range nodes {
		// A leg that produced nothing countable — no result, no files, no
		// narrated phrase — is drawn as its own rail segment with the class
		// word on it, in the one row the rail would have taken anyway. Three
		// "test  pytest" rows with no counts said nothing three times; as
		// ticks they still show that tests ran between the legs that did
		// something, at a third of the height (SPEC §4: every line answers a
		// question; anything that doesn't is dimmed or dropped).
		if n.leg >= 0 && tickLeg(tr.Legs[n.leg], o) {
			stroke := railStroke
			if i == 1 {
				stroke = railHead
			}
			if forked {
				stroke = " " // the fork row above already reached down to here
			}
			leg := tr.Legs[n.leg]
			b.selNode(b.pick(), tickRow(leg, stroke, o.Width))
			forked = b.branches(tr, n.leg, o) > 0
			ticked = !forked
			continue
		}

		switch {
		case i == 0:
			// The journey's first node. Nothing came before it.
		case compactedBetween(tr.Compactions, nodes[i-1].at, n.at):
			// The conversation ran out of context here and was folded into
			// a summary: everything below works from what the summary kept.
			// A session compacted twice is the one to read closely. Drawn
			// even under a fork or a tick — the one rule that means "the
			// memory changed here" is not the one to skip.
			b.node(compactRule(compactionIn(tr.Compactions, nodes[i-1].at, n.at), o.Width))
		case forked:
			// The fork row above is a rail segment of its own: `├` reaches down
			// to this node as well as right to the lane it opened.
		case ticked:
			// The tick above is a rail segment of its own too.
		case i == 1:
			b.node(ruleStyle.Render(railHead))
		case long && hourTurned(nodes[i-1].at, n.at):
			// A day-long trail has no clock in it otherwise: every leg's
			// figure is a duration and every one looks alike. The rail row
			// the hour turns on carries the hour, so "when" is readable by
			// eye down a long column (a rule every hour, no extra rows).
			b.node(hourRule(n.at, nodes[i-1].at, o.Width))
		case o.Dense:
			// No air between legs: the trail is longer than its panel.
		default:
			b.node(ruleStyle.Render(railStroke))
		}
		ticked = false

		if n.leg >= 0 {
			leg := tr.Legs[n.leg]
			label, narrated := legLabel(leg, o)
			b.selNode(b.pick(), legRow(leg, label, narrated, o))
			// A question that did not fit on HEAD's row is spelled out
			// beneath it, options and all, at every level: it is the one
			// line the deck exists to deliver, and clipping it at "the
			// office CIDR only,…" left the decision one attach away.
			var extra []detailRow
			if leg.Current && o.HeadState == state.NeedsYou && o.Head != "" && label != o.Head {
				rest := strings.TrimSpace(strings.TrimPrefix(o.Head, label))
				for _, line := range wrapN(rest, o.Width-trailWayWidth, 4) {
					extra = append(extra, detailRow{text: dimStyle.Render(line), sel: -1})
				}
			}
			if o.Level >= levelWaypoints {
				b.details(append(extra, legDetails(leg, label, o, b)...))
			} else {
				b.details(extra)
			}
			forked = b.branches(tr, n.leg, o) > 0
		} else {
			// Selectable from Lv2, like a leg: it is the row you wrote, and the
			// reader anchors to it as readily as to any leg's first line.
			b.selNode(b.pick(), promptRow(tr.Prompts[n.prompt], o.Now, o.Width, n.prompt+1, len(tr.Prompts), promptWait(tr, n.prompt)))
			forked = false
		}
	}
	// Branches that forked before any leg opened hang off the end of the rail.
	b.branches(tr, -1, o)
}

// longTrailSpan is the span past which a trail gets hour rules on its rail.
const longTrailSpan = 2 * time.Hour

// hourTurned reports whether the clock crossed an hour between two moments.
func hourTurned(a, b time.Time) bool {
	a, b = a.Local(), b.Local()
	return a.Truncate(time.Hour) != b.Truncate(time.Hour)
}

// hourRule is the rail row an hour turns on: "│ 14:00 ────" — and, on the
// first rule of a new day, "│ Tue 00:12 ────", so a trail two days long has
// its days as well as its hours.
func hourRule(at, prev time.Time, width int) string {
	stamp := at.Local().Format("15:04")
	if at.Local().YearDay() != prev.Local().YearDay() {
		stamp = at.Local().Format("Mon 15:04")
	}
	row := railStroke + " " + stamp + " "
	if rest := width - len([]rune(row)); rest > 0 {
		row += strings.Repeat("─", rest)
	}
	return ruleStyle.Render(clip(row, width))
}

// compactedBetween reports whether a compaction fell in (after, upTo].
func compactedBetween(compactions []time.Time, after, upTo time.Time) bool {
	_, ok := compactionAt(compactions, after, upTo)
	return ok
}

// compactionIn is the last compaction in (after, upTo]: the rail row names
// one moment, and the newest is the summary the session is working from.
func compactionIn(compactions []time.Time, after, upTo time.Time) time.Time {
	at, _ := compactionAt(compactions, after, upTo)
	return at
}

func compactionAt(compactions []time.Time, after, upTo time.Time) (time.Time, bool) {
	var found time.Time
	ok := false
	for _, c := range compactions {
		if c.After(after) && !c.After(upTo) {
			found, ok = c, true
		}
	}
	return found, ok
}

// compactRule is the rail row a compaction falls on: "│ ⟲ compacted 14:02 ────".
func compactRule(at time.Time, width int) string {
	row := railStroke + " " + glyphCompact + " context compacted " + at.Local().Format("15:04") + " "
	if rest := width - len([]rune(row)); rest > 0 {
		row += strings.Repeat("─", rest)
	}
	return ruleStyle.Render(clip(row, width))
}

// glyphCompact marks a compaction on the rail and in the totals.
const glyphCompact = "⟲"

// TrailRow is one selectable row of the trail: what it is, what it says, and
// the moment it stands for. Enter at Lv2 opens the reader at that moment
// (SPEC §3), so every selectable row has to name one.
type TrailRow struct {
	Time time.Time // the moment the row stands for
	Kind string    // "leg", "waypoint" or "branch"
	Text string    // what the row says, undecorated
	Leg  int       // index into Trail.Legs the row belongs to, or -1
}

// TrailRows enumerates the trail's selectable rows in the order RenderTrail
// draws them — oldest first, top-down, so the last row is the newest thing that
// has happened. Legs, their waypoints (from Lv2 down) and the subagent lanes
// can be selected; prompts, rails, ghosts and the synthetic "touched" row
// cannot: they name no moment of their own. The enumeration is geometry-free,
// so a cursor index means the same thing at any width, and TrailOpts.Cursor
// indexes straight into this slice.
func TrailRows(tr journey.Trail, level int) []TrailRow {
	var out []TrailRow
	branches := func(after int) {
		for i := range tr.Branches {
			if br := tr.Branches[i]; br.AfterLeg == after {
				out = append(out, TrailRow{Time: br.Start, Kind: "branch",
					Text: branchName(br.Label), Leg: after})
			}
		}
	}

	for _, n := range trailNodes(tr) {
		if n.leg < 0 {
			// Your own prompt. It is a boundary rather than a span of work, but
			// it is also the row you wrote yourself, and "show me what happened
			// after I asked this" is the most natural way into a session — so
			// the cursor stops on it. Leg -1 says it belongs to no leg.
			out = append(out, TrailRow{Time: tr.Prompts[n.prompt].At, Kind: "prompt",
				Text: tr.Prompts[n.prompt].Text, Leg: -1})
			continue
		}
		leg := tr.Legs[n.leg]
		out = append(out, TrailRow{Time: leg.Start, Kind: "leg", Text: leg.Label, Leg: n.leg})
		if level >= levelWaypoints {
			for _, w := range leg.Waypoints {
				if restates(leg, w) {
					continue
				}
				out = append(out, TrailRow{Time: w.At, Kind: "waypoint", Text: w.Text, Leg: n.leg})
			}
		}
		branches(n.leg)
	}
	branches(-1)
	return out
}

// trailNode is one row on the rail: a leg or a prompt, resolved to a time so
// the two can be interleaved.
type trailNode struct {
	at     time.Time
	leg    int // index into Trail.Legs, or -1
	prompt int // index into Trail.Prompts, or -1
}

// trailNodes interleaves legs and prompts, oldest first — the order they
// happened, which is now the order they are drawn (M7 contract). A leg that
// starts on the same tick as the prompt that provoked it sits below it: the
// prompt came first, and the sort is stable with the prompts laid down first.
func trailNodes(tr journey.Trail) []trailNode {
	nodes := make([]trailNode, 0, len(tr.Prompts)+len(tr.Legs))
	for i, p := range tr.Prompts {
		nodes = append(nodes, trailNode{at: p.At, leg: -1, prompt: i})
	}
	for i, l := range tr.Legs {
		nodes = append(nodes, trailNode{at: l.Start, leg: i, prompt: -1})
	}
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].at.Before(nodes[j].at) })
	return nodes
}

// legKey is the identity the narrator's cache is keyed by: session, the leg's
// start and its class (M3 contract, narrator.LegKey). The renderer forms it
// itself so the ui package stays free of the narrator's plumbing — the format
// is contract, not implementation detail.
func legKey(key string, l journey.Leg) string {
	return key + "/" + strconv.FormatInt(l.Start.UnixNano(), 10) + "/" + l.Class.String()
}

// inProgress is the plan's present tense: the first in-progress item's active
// form, its subject when it has none, "" when nothing is in progress.
func inProgress(items []todo.Item) string {
	for _, it := range items {
		if it.Status != todo.InProgress {
			continue
		}
		if s := strings.TrimSpace(it.Active); s != "" {
			return s
		}
		return strings.TrimSpace(it.Text)
	}
	return ""
}

// tickLeg reports whether a closed leg has nothing to say beyond its class:
// no parsed result, no files touched, and no narrated phrase for it.
func tickLeg(l journey.Leg, o TrailOpts) bool {
	if l.Current || len(l.Waypoints) > 0 || len(l.Files) > 0 {
		return false
	}
	if l.Class == journey.Ship || l.Class == journey.Test {
		// A ship leg is the landing: the row that answers "is it done?".
		// Demoting it to a word on the rail read as debris under the last
		// test run, and the answer to the question the trail exists for was
		// the one row nobody noticed. A test leg is the one class that can
		// be red: a run whose verdict never parsed is a row that says "?",
		// not a word on the rail.
		return false
	}
	_, narrated := legLabel(l, o)
	return !narrated
}

// tickRow is a tick leg's whole appearance: the rail stroke and the class
// word, dim, in the verb column where the leg's own row would have put it.
func tickRow(l journey.Leg, stroke string, width int) string {
	head := ruleStyle.Render(stroke) + " " + dimStyle.Render(l.Class.String())
	// The leg's own label rides on the tick when it has one: "│ build" ten
	// times down a column said nothing about what was built.
	if label := strings.TrimSpace(l.Label); label != "" && label != l.Class.String() {
		if room := width - lipgloss.Width(head) - 1 - len([]rune(legSpan(l))) - 4; room >= trailMinLabel {
			// Padded like a leg row's class, so the labels line up down
			// the column.
			head = ruleStyle.Render(stroke) + " " + dimStyle.Render(pad(l.Class.String(), trailVerbWidth)) + " " + dimStyle.Render(clip(label, room))
		}
	}
	// A test leg that reported nothing is a run with no verdict, which is a
	// fact, not an absence: "?" says so, where a bare word between a red run
	// and a green one read as if the tests had simply skipped a beat.
	tail := legSpan(l)
	if l.Class == journey.Test {
		tail = "?  " + tail
	}
	// The figure ends where every other row's does: flush with the edge.
	room := width - lipgloss.Width(head)
	if room < len([]rune(tail))+1 {
		return pad(head, width)
	}
	return head + padLeft(dimStyle.Render(tail), room)
}

// legLabel resolves what a leg says: the narrated line when one has landed for
// it, the heuristic label otherwise. HEAD always keeps its heuristic label —
// narration is for history, and the live leg is still changing its mind.
func legLabel(l journey.Leg, o TrailOpts) (string, bool) {
	if l.Current {
		if o.Head != "" {
			if o.HeadState == state.NeedsYou && len([]rune(o.Head)) > headLabelMax(o.Width) {
				// Too long for the row: the row carries what fits at a word,
				// and the rows beneath carry the rest.
				return wrapN(o.Head, headLabelMax(o.Width), 100)[0], false
			}
			return o.Head, false
		}
		// HEAD is named by the plan when the plan has a name for it: the
		// in-progress task's own present tense is what the session is doing
		// in its own words, and beats a file name — or nothing, which is what
		// a build leg twenty minutes into unfamiliar files used to show.
		// Unless it is parked on its agents: then the plan's task is the
		// work it gave away, and its own last work is the honest name.
		if doing := inProgress(o.Todos); doing != "" && o.HeadWaits == 0 {
			return doing, false
		}
		return l.Label, false
	}
	if l.Class == journey.Ship {
		// What shipped, in the commit's own words, beats "git commit".
		for i := len(l.Waypoints) - 1; i >= 0; i-- {
			if w := l.Waypoints[i]; w.Kind == journey.WaypointCommit && strings.TrimSpace(w.Text) != "" {
				return w.Text, false
			}
		}
	}
	if len(o.Labels) == 0 {
		return l.Label, false
	}
	// A cached label from before the narrator learned to refuse the class
	// name is still the class name; the heuristic label beats it.
	if text := strings.TrimSpace(o.Labels[legKey(o.SessionKey, l)]); text != "" && !strings.EqualFold(text, l.Class.String()) {
		return text, true
	}
	return l.Label, false
}

// headLabelMax is the longest label HEAD's row can carry whole at a width.
func headLabelMax(width int) int {
	return width - trailPrefixWidth - 1 - len("waiting 10m")
}

// legRow: glyph, class verb, label, and the age held at the right margin. HEAD
// points at itself — `← 3m` — because it is the only line that is still moving.
// The arrow is a "you are here", not a direction of travel: it means the same
// thing with the past above it as it did with the past below.
// A narrated leg spends the verb column on its own words: the prose already
// says what the class was for, and the glyph keeps the class's tint.
func legRow(l journey.Leg, label string, narrated bool, o TrailOpts) string {
	width := o.Width
	glyph := glyphLeg
	age := legSpan(l)
	if l.Current {
		glyph, age = headMark(o, l)
	}
	head := classStyle(l.Class).Render(glyph + " " + pad(l.Class.String(), trailVerbWidth))

	// One row shape, narrated or not. A narrated phrase is a better label than
	// the heuristic one, so it goes in the label column — it does not replace
	// the class. The class is what Lv1 classifies by (SPEC §2.2), and dropping
	// its word left colour as the only thing carrying it: seven tints on one
	// glyph, which says nothing at all in monochrome, over NO_COLOR, or to
	// anyone who does not separate teal from cyan (SPEC §4).
	if narrated {
		label = withoutClassVerb(label, l.Class.String())
	}

	// The leg's own result, at badge size, held between the label and the age:
	// "◆ test  pytest  18✓ 2✗  12m". SPEC §2.2's mockup has always shown this,
	// and the leg has always carried it — Lv1 rendered the label and dropped
	// the waypoints, so the one number that says whether the leg went well was
	// two keypresses away.
	badge := legBadge(l)
	badgeW := lipgloss.Width(badge)
	if badgeW > 0 {
		badgeW++ // a column of air before it
	}

	labelWidth := width - trailPrefixWidth - 1 - len([]rune(age)) - badgeW
	if labelWidth < trailMinLabel {
		badge, badgeW = "", 0
		labelWidth = width - trailPrefixWidth - 1 - len([]rune(age))
	}
	if labelWidth < trailMinLabel && l.Current && strings.Contains(age, " · ") {
		// HEAD's figure can be a sentence — "◈3 out 20m · quiet 15m". A
		// narrow column keeps the label and the first clause: a row that
		// says what it is doing and that agents are out beats one that
		// says how long, with no name. (The person reading the real thing
		// wanted the name.)
		first := strings.SplitN(age, " · ", 2)[0]
		if w := width - trailPrefixWidth - 1 - len([]rune(first)); w >= trailMinLabel {
			age, labelWidth = first, w
		}
	}
	if labelWidth < trailMinLabel {
		// Too narrow for a label: the verb and the figure still answer
		// "what, when".
		return head + " " + dimStyle.Render(clip(age, width-trailPrefixWidth-1))
	}
	// A wide panel spends its width inside the row: the leg's own detail —
	// the failing test, the bug, the files — beside the label, where a
	// keypress used to be the only way to it. Only at Lv1: at Lv2 the
	// details hang beneath the leg already.
	labelText := textStyle.Render(pad(clip(label, labelWidth), labelWidth))
	if inlineFits(l, label, labelWidth, o) {
		detail := legInline(l)
		used := len([]rune(label))
		labelText = textStyle.Render(label) + dimStyle.Render(pad(" · "+clip(detail, labelWidth-used-3), labelWidth-used))
	}
	return head + " " + labelText + badgeStyle(badge).Render(padLeft(badge, badgeW)) +
		" " + dimStyle.Render(age)
}

// inlineFits reports whether a leg's detail rides on its row: a wide enough
// panel, and room beside the label. At Lv2 the same rule holds, and the
// children the row already carries are not drawn beneath it again —
// "Lv2 is Lv1 re-split onto more rows" was the complaint.
func inlineFits(l journey.Leg, label string, labelWidth int, o TrailOpts) bool {
	if o.NoInline || o.Width < trailInlineWidth {
		return false
	}
	detail := legInline(l)
	if detail == "" {
		return false
	}
	return labelWidth-len([]rune(label))-3 >= trailInlineMin
}

// legLabelWidth is the room legRow gives a leg's label at a width: the
// same arithmetic, so legDetails can ask whether the row took the detail.
func legLabelWidth(l journey.Leg, o TrailOpts) int {
	age := legSpan(l)
	if l.Current {
		_, age = headMark(o, l)
	}
	badgeW := lipgloss.Width(legBadge(l))
	if badgeW > 0 {
		badgeW++
	}
	w := o.Width - trailPrefixWidth - 1 - len([]rune(age)) - badgeW
	if w < trailMinLabel {
		w = o.Width - trailPrefixWidth - 1 - len([]rune(age))
	}
	return w
}

// trailInlineMin is the room a leg's detail needs before it rides on the
// row, and trailInlineWidth the panel it takes to try: a wide deck's trail,
// never a board column.
const (
	trailInlineMin   = 24
	trailInlineWidth = 80
)

// legInline is the one detail a leg would show first at Lv2: its failing
// tests, its bugs, its commit, or the files it touched beyond its label.
func legInline(l journey.Leg) string {
	var fails, bugs []string
	for _, w := range l.Waypoints {
		switch w.Kind {
		case journey.WaypointTestFail:
			fails = append(fails, wayFail+" "+failText(w))
		case journey.WaypointBug:
			bugs = append(bugs, w.Text)
		}
	}
	switch {
	case len(fails) > 0:
		return strings.Join(fails, " · ")
	case len(bugs) > 0:
		return strings.Join(bugs, " · ")
	}
	switch l.Class {
	case journey.Build, journey.Fix, journey.Docs:
	default:
		return "" // a ship leg's commit is its label; a scout's files are its label
	}
	var others []string
	for _, f := range l.Files {
		if !sameFile(f, l.Label) {
			others = append(others, f)
		}
	}
	if len(others) == 0 {
		return ""
	}
	return "touched " + strings.Join(others, " · ")
}

// headMark is HEAD's glyph and figure: the fleet's glyph and wait when the
// state machine says the session is hung or waiting on you, the breathing ●
// and "for 2h" — how long it has been at this — when it is working.
func headMark(o TrailOpts, l journey.Leg) (glyph, figure string) {
	since := o.HeadSince
	if since.IsZero() {
		since = l.Start
	}
	switch o.HeadState {
	case state.NeedsYou:
		return fleet.Glyph(state.NeedsYou), "waiting " + relAge(o.Now, since)
	case state.Stuck:
		return fleet.Glyph(state.Stuck), "silent " + relAge(o.Now, since)
	}
	glyph = glyphHead
	if o.Pulse {
		glyph = glyphBreath
	}
	// "for 2h" — read beside the finished legs' durations it is the same
	// kind of number, where "← 2h" was read by everyone who tried it as
	// "two hours ago".
	figure = "for " + relAge(o.Now, l.Start)
	if o.HeadTail != "" {
		figure = o.HeadTail
	}
	return glyph, figure
}

// headTail is a working HEAD's figure, in the words the fleet row uses too:
// how long it has been at this, and — when it has agents out — how many,
// how long the oldest has been away, and whether the parent is parked on
// them ("quiet 15m": nothing of its own since the newest left) or still
// working ("for 2h"). One sentence at every depth; zooming in must not
// change the words.
func headTail(tr journey.Trail, now time.Time, live bool) string {
	var head *journey.Leg
	for i := range tr.Legs {
		if tr.Legs[i].Current {
			head = &tr.Legs[i]
		}
	}
	if head == nil {
		return ""
	}
	out, oldest, newest := 0, time.Time{}, time.Time{}
	for _, b := range tr.Branches {
		if !b.Done {
			out++
			if oldest.IsZero() || b.Start.Before(oldest) {
				oldest = b.Start
			}
			if b.Start.After(newest) {
				newest = b.Start
			}
		}
	}
	if out == 0 || !live {
		return "for " + relAge(now, head.Start)
	}
	tail := fmt.Sprintf("◈%d out %s", out, relAge(now, oldest))
	if head.End.After(newest) {
		return tail + " · for " + relAge(now, head.Start)
	}
	return tail + " · quiet " + relAge(now, head.End)
}

// headWaits counts the open lanes HEAD is parked on: agents out, and no
// vote of HEAD's own since the newest of them left.
func headWaits(tr journey.Trail) int {
	var head *journey.Leg
	for i := range tr.Legs {
		if tr.Legs[i].Current {
			head = &tr.Legs[i]
		}
	}
	if head == nil {
		return 0
	}
	out, newest := 0, time.Time{}
	for _, b := range tr.Branches {
		if !b.Done {
			out++
			if b.Start.After(newest) {
				newest = b.Start
			}
		}
	}
	if out == 0 || head.End.After(newest) {
		return 0
	}
	return out
}

func badgeStyle(badge string) lipgloss.Style {
	if strings.Contains(badge, "✗") {
		return textStyle
	}
	return dimStyle
}

// legSpan is the right-margin figure of a closed leg: its duration. It used to
// be the age of its start, and on a session that did its work in one burst
// every leg read "17h" — the same figure ten times, telling you nothing about
// where the time went. The prompts still carry ages, so "when" stays
// anchored; the legs now say "how long", which is the thing that tells them
// apart. HEAD keeps its age (legRow), because HEAD is still moving.
func legSpan(l journey.Leg) string {
	if l.End.IsZero() || l.End.Before(l.Start) {
		return "—"
	}
	return state.ShortDuration(l.End.Sub(l.Start))
}

// legBadge is the leg's newest run summary at badge size, or "" for a leg that
// produced no countable result. Only test runs get one: a commit's subject is
// already the leg's label, and repeating it in a narrower column would say the
// same thing worse.
func legBadge(l journey.Leg) string {
	for i := len(l.Waypoints) - 1; i >= 0; i-- {
		if w := l.Waypoints[i]; w.Kind == journey.WaypointTestRun && w.Short != "" {
			return w.Short
		}
	}
	if l.Class == journey.Test && !l.Current {
		// A run whose output never parsed is a run with no verdict, which
		// is a fact, not an absence: "?" says so, where a bare row between
		// a red run and a green one read as if the tests had skipped a beat.
		return "?"
	}
	return ""
}

// badgeStyle lifts a badge that carries a failure out of the greys. Not into
// red: the palette reserves warm colour for "needs you" and "stuck" alone, so
// that a healthy fleet holds none, and a failing test is not the session
// asking for you — it is still working on it. The ✗ is the fact; this only
// stops it reading as quietly as an age.

// withoutClassVerb drops a narrated phrase's first word when it is the class's
// own name, so "fix probe6 exit" under `fix` reads "fix    probe6 exit" rather
// than saying it twice. Only an exact match goes: "push to main" under `ship`
// keeps its verb, because "ship  to main" is not what happened.
func withoutClassVerb(label, class string) string {
	rest, ok := strings.CutPrefix(label, class+" ")
	if !ok || strings.TrimSpace(rest) == "" {
		return label
	}
	return strings.TrimSpace(rest)
}

// promptRow quotes the human turn — the only words on the trail that are not
// ours.
func promptRow(p journey.Prompt, now time.Time, width, nth, total int, waited time.Duration) string {
	// "2h ago", where a leg says "12m": the prompt is when, the leg is how
	// long, and without the word a column of figures reads as one kind.
	age := relAge(now, p.At) + " ago"
	if waited >= waitNotable {
		// How long the session sat waiting for this prompt, where the row
		// has the room for it and a readable prompt: "waited 40m · 2h ago".
		with := "waited " + relDuration(waited) + " · " + age
		if width-len([]rune(with))-8 >= trailInlineMin {
			age = with
		}
	}
	// The chapter: "◉ 9/13" is what `[` and `]` step through, and on a
	// trail with a dozen prompts it is the readout to steer by.
	lead := glyphPrompt
	if total > 1 {
		lead += fmt.Sprintf(" %d/%d", nth, total)
	}
	textWidth := width - len([]rune(lead)) - 1 - 1 - len([]rune(age))
	if textWidth < trailMinLabel {
		return dimStyle.Render(lead) + padLeft(dimStyle.Render(age), width-len([]rune(lead)))
	}
	text := textStyle.Render(pad(clip(`"`+p.Text+`"`, textWidth), textWidth))
	return dimStyle.Render(lead) + " " + text + " " + dimStyle.Render(age)
}

// legDetails is a leg's Lv2 body: its waypoints in the order they happened,
// then — for the classes that touch files — what it touched. Every row is dim:
// at Lv2 the legs are still the structure and the waypoints are what hangs off
// them.
func legDetails(l journey.Leg, label string, o TrailOpts, b *trailBuilder) []detailRow {
	width := o.Width
	body := width - trailWayWidth
	out := make([]detailRow, 0, len(l.Waypoints)+1)
	bugs := 0
	// What the row itself carries is not hung beneath it again. The picks
	// are still spent — TrailRows counts moments, not columns — and the
	// cursor steps over a row that is not drawn (cursorMove).
	onRow := inlineFits(l, label, legLabelWidth(l, o), o)
	for _, w := range l.Waypoints {
		if restates(l, w) {
			continue // TrailRows skips it too
		}
		// The index is spent whether or not the panel is wide enough to draw
		// the row: TrailRows counts moments, not columns.
		sel := b.pick()
		if w.Kind == journey.WaypointBug {
			bugs++
		}
		if body < trailMinLabel {
			continue
		}
		if onRow && (w.Kind == journey.WaypointTestFail || w.Kind == journey.WaypointBug) {
			continue
		}
		out = append(out, detailRow{text: waypointBody(w, bugs, body), sel: sel})
	}
	if body < trailMinLabel {
		return nil
	}
	if row := touchedBody(l, body); row != "" && !onRow {
		out = append(out, detailRow{text: row, sel: -1})
	}
	return out
}

// restates reports whether a waypoint is a row the leg's own row already
// says: the run summary the badge carries ("18 passed · 2 failed" under
// "18✓ 2✗"), the commit subject a ship leg is named by. A child that repeats
// its parent costs a row and a keypress to say nothing.
func restates(l journey.Leg, w journey.Waypoint) bool {
	switch w.Kind {
	case journey.WaypointTestRun:
		return w.Short != "" && w.Short == legBadge(l)
	case journey.WaypointCommit:
		if l.Class != journey.Ship {
			return false
		}
		for i := len(l.Waypoints) - 1; i >= 0; i-- {
			if c := l.Waypoints[i]; c.Kind == journey.WaypointCommit && strings.TrimSpace(c.Text) != "" {
				return c.Text == w.Text // the newest commit is the label
			}
		}
	}
	return false
}

// waypointBody decorates one waypoint: the Kind picks the prefix, the Text
// never carries one of its own. A failing test is the only thing on the trail
// allowed to spark — and the `✗` says it without the colour.
func waypointBody(w journey.Waypoint, bug, width int) string {
	switch w.Kind {
	case journey.WaypointTestFail:
		return stuckStyle.Render(wayFail) + " " + dimStyle.Render(clip(failText(w), width-2))
	case journey.WaypointBug:
		prefix := fmt.Sprintf("bug%d ", bug)
		return dimStyle.Render(prefix + clip(w.Text, width-len(prefix)))
	default:
		return dimStyle.Render(clip(w.Text, width))
	}
}

// failText is a failing test's name, and — when the session has been round
// this failure before — how many legs it has now failed in: "✗
// test_refresh_expired_token · 3rd time". It is the trail's plainest sign
// of a loop, and a row that says it is worth more than a third red row that
// looks like the first two.
func failText(w journey.Waypoint) string {
	if w.Runs >= 2 {
		// "10th failure", not "10th leg": beside "↑ 128 legs" the leg
		// count read as the leg's number.
		return w.Text + " · " + ordinal(w.Runs) + " failure"
	}
	return w.Text
}

// ordinal is 2nd, 3rd, 4th … 11th, 12th, 13th, 21st.
func ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return strconv.Itoa(n) + suffix
}

// touchedBody is the one Lv2 row no extractor produces: the files a leg that
// writes code left behind. One file is already in the label, so it takes two to
// be worth a row.
func touchedBody(l journey.Leg, width int) string {
	switch l.Class {
	case journey.Build, journey.Fix, journey.Docs:
	default:
		return ""
	}
	if len(l.Files) == 0 {
		return ""
	}
	if len(l.Files) == 1 && sameFile(l.Files[0], l.Label) {
		// "build router.py" over "touched router.py" is the label restated.
		return ""
	}
	return dimStyle.Render(clip("touched "+strings.Join(l.Files, " · "), width))
}

// sameFile reports whether a label is the file, by full path or by name.
func sameFile(file, label string) bool {
	if file == label {
		return true
	}
	if i := strings.LastIndex(file, "/"); i >= 0 {
		return file[i+1:] == label
	}
	return false
}

// wrapN breaks a sentence over at most n rows of width, each at the last
// word that fits, the last clipped. A finding cut at "two are the sa…" was
// the half a reader came for.
func wrapN(text string, width, n int) []string {
	var rows []string
	rest := []rune(strings.TrimSpace(text))
	for len(rest) > 0 {
		if len(rows) == n-1 || len(rest) <= width {
			rows = append(rows, clip(string(rest), width))
			break
		}
		cut := width
		for i := width; i > width/2; i-- {
			if rest[i] == ' ' {
				cut = i
				break
			}
		}
		rows = append(rows, string(rest[:cut]))
		rest = []rune(strings.TrimSpace(string(rest[cut:])))
	}
	return rows
}

// branches draws the subagent lanes that forked off leg index after (-1 for the
// ones that forked before any leg opened), oldest first, each hanging directly
// under the leg it left from. At Lv2 a returned agent also says what it found.
// It returns how many lanes it drew.
func (b *trailBuilder) branches(tr journey.Trail, after int, o TrailOpts) int {
	width, drawn := o.Width, 0
	for i := range tr.Branches {
		br := tr.Branches[i]
		if br.AfterLeg != after {
			continue
		}
		sel := b.pick()
		mark := branchOpen
		if br.Done {
			mark = branchDone
			if strings.TrimSpace(br.Report) == "" {
				mark = branchEmpty // back, with nothing: never a tick
			}
		}
		// The lane's clock: how long the agent has been out, or how long ago
		// it came back. An agent that never returns is the silent failure of
		// delegated work, and a lane without a clock could not show it.
		// One meaning per clock: "⋯ 20m out" is how long it has been away,
		// "✓ 2h ago" how long since it came back; a leg's bare figure is a
		// duration. Three kinds of number down one rail need their words.
		tail := mark + " " + relAge(o.Now, br.Start) + " out"
		if !br.Done && o.HeadState == state.Idle {
			// The turn is over and this never came back: lost, not out.
			// "◈1 out 3d" on a session idle for three days was a count of
			// nothing.
			tail = branchEmpty + " lost " + relAge(o.Now, br.Start) + " ago"
		}
		if br.Done {
			back := br.End
			if back.IsZero() {
				back = br.Start
			}
			tail = mark + " " + relAge(o.Now, back) + " ago"
		}
		labelWidth := width - trailForkWidth - 1 - len([]rune(tail))
		if labelWidth < trailMinLabel {
			continue
		}
		name := clip(branchName(br.Label), labelWidth)
		if n, ok := o.LaneLinks[br.Label]; ok && n > 0 {
			// A session that looks like this agent's: the link survives
			// the clip, because it is the three characters that go somewhere.
			link := fmt.Sprintf(" →%d", n)
			name = clip(branchName(br.Label), labelWidth-len([]rune(link))) + link
		}
		label := dimStyle.Render(pad(name, labelWidth))
		b.selNode(sel, ruleStyle.Render(railFork)+textStyle.Render(glyphBranch)+" "+
			label+" "+dimStyle.Render(tail))
		drawn++

		// A returned agent says what it found, at every level: a ✓ that
		// keeps its finding two keypresses down creates an obligation without
		// discharging it. And a ✓ with nothing to say says that, because
		// "came back empty", "report lost" and "not parsed" want three
		// different reactions and silence looks like all of them.
		if br.Done {
			report := strings.TrimSpace(br.Report)
			if report == "" {
				report = "came back with no report"
			}
			if body := width - trailWayWidth; body >= trailMinLabel {
				var rows []detailRow
				for _, line := range wrapN(report, body, 3) {
					rows = append(rows, detailRow{text: dimStyle.Render(line), sel: -1})
				}
				b.details(rows)
			}
		}
	}
	return drawn
}

// branchName never renders empty: an unnamed subagent is still "agent".
func branchName(label string) string {
	if strings.TrimSpace(label) == "" {
		return "agent"
	}
	return label
}

// relAge is the M0 age format, relative to the caller's clock.
func relAge(now, t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return state.ShortDuration(now.Sub(t))
}

// trailColumn is the deck's right-hand panel: the title, one line of air, and
// the graph.
func (m *Model) trailColumn(w, h int) []string {
	rows := []string{m.trailTitle(w), ""}
	if m.sessionView() {
		rows = m.sessionCard(w)
	}
	if h > 2 {
		rows = append(rows, trailRows(m.trail, m.trailOpts(w, h-2))...)
	}
	return rows
}

// sessionCard is the session view's title: the column's own header — the
// fleet row and the verdict — so one Tab from the board reads as that
// column expanded, with the level and the day on the right.
func (m *Model) sessionCard(w int) []string {
	r, ok := m.boardRows()[m.selectedKey]
	if !ok {
		return []string{m.trailTitle(w), ""}
	}
	// Where the keys are, in the words the help uses — board, session,
	// reader — not a level number: one Tab from the board read "[Lv2]",
	// and the person pressing it asked whether that was expected.
	// At Lv3 the card says nothing: the keys are in the reader, and the
	// word goes where the bar is (readerTitle).
	right := "[session]"
	if m.level >= levelReader {
		right = ""
	}
	if n := m.legsAbove(); n > 0 {
		right = strings.TrimSpace(fmt.Sprintf("↑ %d legs  %s", n, right))
	}
	if !m.trailPinned {
		right = strings.TrimSpace("↓ G  " + right)
	}
	body := w - 1
	hdr := m.columnHeader(m.selectedKey, r, body-lipgloss.Width(right)-1)
	// One session on screen: the fleet's selection arrow says nothing here.
	first := m.titleMark(panelTrail) + strings.Replace(hdr[0], "▸", " ", 1)
	gap := w - lipgloss.Width(first) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	first += strings.Repeat(" ", gap) + dimStyle.Render(right)
	return []string{first, m.cardSecond(w)}
}

// cardSecond is the card's second row: the verdict, then the day added up
// — "22h · 16 ships · 10 red · 2 compactions · waited on you 40m" — and
// the tmux session on the right. The board's column had no room for the
// day and the trail's own title has it below 110 columns; the session
// view is the one place a long day is read closely, and it was the one
// place the day was not said. Clauses are shed whole, the day's compact
// form tried before any clause goes, the tmux name before the day.
func (m *Model) cardSecond(w int) string {
	s, ok := m.selected()
	if !ok {
		return ""
	}
	room := w - 4
	verdict := strings.Split(boardVerdict(s, m.trail, m.now), " · ")
	if s.Snap.State != state.Idle && len(verdictParts(m.trail, m.now, true)) == 0 {
		// Nothing to count: the fleet row's own sentence — the hung call,
		// the question, the present — not the last finished leg. The
		// board's column says the present; zooming in lost it.
		if r, ok := m.boardRows()[m.selectedKey]; ok {
			// entryLines indents its rows by four: give it the card's
			// full width so the sentence is not cut for a margin it
			// then does not draw.
			if lines := m.entryLines(r, room+4); len(lines) > 1 {
				if head := oneSpace(ansi.Strip(lines[1])); head != "" {
					verdict = []string{head}
				}
			}
		}
	} else if s.Snap.State == state.Stuck || s.Snap.State == state.NeedsYou {
		if r, ok := m.boardRows()[m.selectedKey]; ok {
			if lines := m.entryLines(r, room+4); len(lines) > 1 {
				if head := oneSpace(ansi.Strip(lines[1])); head != "" {
					verdict = []string{head}
				}
			}
		}
	}
	tmux := ""
	if pane, ok := m.panes[s.Info.Key()]; ok && pane.Target != "" {
		tmux = "⌁ " + tmuxSessionName(pane.Target)
	}
	// The tmux session is always kept — `enter` attaches from here — and
	// the day is added after the verdict, so joinFit sheds the day's
	// clauses before the verdict's; the long form when it all fits, the
	// compact one otherwise.
	fit := room
	if tmux != "" {
		fit -= lipgloss.Width(tmux) + 2
	}
	best := ""
	for _, compact := range []bool{false, true} {
		day := dayParts(m.trail, m.now, compact)
		parts := append([]string{}, verdict...)
		if len(day) > 0 {
			// The day's total carries the wait; the verdict's clause is
			// the same hour twice on one row.
			parts = withoutPrefix(parts, "on you ")
		}
		parts = append(parts, day...)
		text := joinFit(parts, fit)
		if text == strings.Join(parts, " · ") {
			best = text
			break
		}
		if best == "" || len(text) > len(best) {
			best = text
		}
	}
	if tmux != "" {
		best = pad(best, fit+2) + tmux
	}
	return "    " + dimStyle.Render(best)
}

// oneSpace is a fleet row's sentence with its column padding taken out:
// "◍ fix    Bash: … --all        silent 4m" is one clause, and the padding
// made it wider than the card and clipped it to "sile…".
func oneSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// withoutPrefix drops the clauses that begin with prefix.
func withoutPrefix(parts []string, prefix string) []string {
	out := parts[:0:0]
	for _, p := range parts {
		if !strings.HasPrefix(p, prefix) {
			out = append(out, p)
		}
	}
	return out
}

// trailOpts is the model's state as the renderer wants it.
func (m *Model) trailOpts(w, h int) TrailOpts {
	head, headState, since := "", state.Working, time.Time{}
	if s, ok := m.selected(); ok && s.Live && !m.archiveView {
		head, headState, since = m.headFor(s), s.Snap.State, headSince(s)
	}
	return TrailOpts{
		Todos:      m.todos,
		Labels:     m.labels,
		LaneLinks:  m.laneLinks(m.trail),
		Head:       head,
		HeadState:  headState,
		HeadSince:  since,
		SessionKey: m.selectedKey,
		Now:        m.now,
		Width:      w,
		Height:     h,
		Level:      m.level,
		Cursor:     m.cursor,
		Pulse:      m.pulse,
		Scroll:     m.trailScroll,
		Pinned:     m.trailPinned,
	}
}

// legsAbove counts the legs the trail's viewport currently hides above its
// first row: zero when the whole journey is on screen.
func (m *Model) legsAbove() int {
	w, h := m.trailBox()
	o := m.trailOpts(w, h)
	doc, sel := trailDoc(m.trail, o)
	top := trailTop(len(doc), o)
	return legsHiddenAbove(m.trail, o.Level, sel, top)
}

// legsHiddenAbove counts leg rows among the document's first top lines.
func legsHiddenAbove(tr journey.Trail, level int, sel []int, top int) int {
	rows := TrailRows(tr, level)
	n := 0
	for i := 0; i < top && i < len(sel); i++ {
		if j := sel[i]; j >= 0 && j < len(rows) && rows[j].Kind == "leg" {
			n++
		}
	}
	return n
}

// trailDay is a long trail's sum: its span, its ships and its red runs.
// "" for a trail under two hours, which has nothing to add up yet.
func trailDay(tr journey.Trail, now time.Time, compact bool) string {
	if len(tr.Legs) == 0 {
		return ""
	}
	start := tr.Legs[0].Start
	if len(tr.Prompts) > 0 && tr.Prompts[0].At.Before(start) {
		start = tr.Prompts[0].At
	}
	if now.Sub(start) < longTrailSpan/2 {
		return "" // under an hour there is no day to add up
	}
	ships, red := 0, 0
	for _, l := range tr.Legs {
		switch {
		case l.Class == journey.Ship:
			ships++
		case strings.Contains(legBadge(l), "✗"):
			red++
		}
	}
	out := " · " + relAge(now, start)
	if compact {
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

// Waiting on you. A session that has finished its turn is waiting for your
// next prompt, and the gap between the two is time the session spent on
// you — the number that says which of a fleet your own turns are the
// bottleneck of. It is summed over the trail's prompts (promptWaits), with
// the wait still open added on the board (youWaited), and a gap longer
// than waitAway is not counted at all: that is you being away, not the
// session waiting.
const (
	waitNotable = 5 * time.Minute // below this a wait is not worth a word
	waitAway    = 3 * time.Hour   // above this you were away, not waited on
)

// promptWait is how long the trail sat idle before its i-th prompt: from the
// last thing that happened before it — a leg's end, a lane's return, the
// prompt before — to the prompt. Zero for the first prompt, for a gap the
// trail cannot see, and for one longer than waitAway.
func promptWait(tr journey.Trail, i int) time.Duration {
	if i < 0 || i >= len(tr.Prompts) {
		return 0
	}
	at := tr.Prompts[i].At
	var last time.Time
	if i > 0 {
		last = tr.Prompts[i-1].At
	}
	for _, l := range tr.Legs {
		if !l.End.IsZero() && !l.End.After(at) && l.End.After(last) {
			last = l.End
		}
	}
	for _, b := range tr.Branches {
		if b.Done && !b.End.After(at) && b.End.After(last) {
			last = b.End
		}
	}
	if last.IsZero() {
		return 0
	}
	if d := at.Sub(last); d > 0 && d <= waitAway {
		return d
	}
	return 0
}

// promptWaits is the trail's waits on you added up: every prompt's.
func promptWaits(tr journey.Trail) time.Duration {
	var total time.Duration
	for i := range tr.Prompts {
		total += promptWait(tr, i)
	}
	return total
}

// youWaited is promptWaits with the wait still open on top: a live session
// that has finished its turn — idle, or asking you something — has been
// waiting since its last event, and that wait is on you too.
func youWaited(tr journey.Trail, now time.Time, s fleet.Session) time.Duration {
	total := promptWaits(tr)
	if s.Live && (s.Snap.State == state.Idle || s.Snap.State == state.NeedsYou) && !s.Info.LastEventAt.IsZero() {
		if d := now.Sub(s.Info.LastEventAt); d > 0 && d <= waitAway {
			total += d
		}
	}
	return total
}

// relDuration is a duration the way the trail says ages: "40m", "1h20".
func relDuration(d time.Duration) string {
	return state.ShortDuration(d)
}

// trailTitle: whose trail this is, and how deep we are in it.
func (m *Model) trailTitle(w int) string {
	name := "—"
	if s, ok := m.selected(); ok {
		name = sessionName(s.Info)
		if m.archiveView {
			// The archive's rows are titled by what they asked for; the
			// project is the group header. "TRAIL · api" over a row that
			// read "why does the nightly build take 40 minutes" named the
			// group, not the row.
			name = archiveHeadline(s)
		}
	}
	level := "[trail]"
	switch {
	case m.level >= levelReader:
		level = "[reader]"
	case m.level >= levelWaypoints:
		level = "[legs]"
	}
	// Scrolled off the present, the title says so: the trail is no longer
	// showing the newest work, and `G` is the way back to it.
	// What the viewport hides, and where it stands: "↑ 128 legs" with a
	// day above the fold, "↓ G" when scrolled off the present — both,
	// because the hunt for an hour is exactly when the count matters.
	right := level
	if n := m.legsAbove(); n > 0 {
		right = fmt.Sprintf("↑ %d legs  %s", n, right)
	}
	if !m.trailPinned {
		right = "↓ G  " + right
	}
	mark := m.titleMark(panelTrail)
	body := w - 1
	// A day-long trail adds itself up in the title — "· 22h · 13 ships ·
	// 8 red" — at every level and while scrolled, where the board's fold
	// row is not.
	room := body - lipgloss.Width(right) - 1
	title := "TRAIL · " + name + trailDay(m.trail, m.now, false)
	if len([]rune(title)) > room {
		title = "TRAIL · " + name + trailDay(m.trail, m.now, true) // "· 22h · 16⚑ 10✗"
	}
	if len([]rune(title)) > room {
		title = "TRAIL · " + name // the day goes whole, never "· 22h ·…"
	}
	left := m.titleStyleFor(panelTrail).Render(clip(title, room))
	gap := body - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return mark + left + strings.Repeat(" ", gap) + dimStyle.Render(right)
}
