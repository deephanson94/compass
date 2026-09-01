package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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
	maxGhosts        = 4                  // the plan shows its next four moves
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
	return doc[top:end]
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

	b := trailBuilder{cursor: trailCursor(o), width: width}
	b.journey(tr, nodes, o)
	if len(tr.Legs) == 0 {
		// A journey that has only been asked for: say what comes next rather
		// than leaving the panel half empty (SPEC §4).
		b.node("")
		b.node(dimStyle.Render(clip("scouting will appear here", width)))
	}
	b.ghosts(o.Todos, width, o.Height)
	return b.render()
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

	cursor int // the selectable row to invert; -1 for none
	picked int // how many selectable rows have been handed out so far
	width  int // the panel's columns, so the cursor's bar spans all of them
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
	for show > 0 && ghostCost(show, len(pending)) > height/2 {
		show--
	}
	if ghostCost(show, len(pending)) > height/2 {
		return
	}

	for i := 0; i < show; i++ {
		b.node(ruleStyle.Render(railGhost))
		b.node(dimStyle.Render(glyphGhost + " " + clip(pending[i], body)))
	}
	if more := len(pending) - show; more > 0 {
		b.node(ruleStyle.Render(railGhost) + " " +
			dimStyle.Render(clip(fmt.Sprintf("+%d more", more), body)))
	}
}

// ghostCost is how many rows show ghosts (out of total pending) occupy: one
// each, one dashed rail each, plus the "+N more" row when the plan is longer.
func ghostCost(show, total int) int {
	rows := 2 * show
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
	for i, n := range nodes {
		switch {
		case i == 0:
			// The journey's first node. Nothing came before it.
		case forked:
			// The fork row above is a rail segment of its own: `├` reaches down
			// to this node as well as right to the lane it opened.
		case i == 1:
			b.node(ruleStyle.Render(railHead))
		default:
			b.node(ruleStyle.Render(railStroke))
		}

		if n.leg >= 0 {
			leg := tr.Legs[n.leg]
			label, narrated := legLabel(leg, o)
			b.selNode(b.pick(), legRow(leg, label, narrated, o.Now, o.Width, o.Pulse))
			if o.Level >= levelWaypoints {
				b.details(legDetails(leg, o.Width, b))
			}
			forked = b.branches(tr, n.leg, o) > 0
		} else {
			// Selectable from Lv2, like a leg: it is the row you wrote, and the
			// reader anchors to it as readily as to any leg's first line.
			b.selNode(b.pick(), promptRow(tr.Prompts[n.prompt], o.Now, o.Width))
			forked = false
		}
	}
	// Branches that forked before any leg opened hang off the end of the rail.
	b.branches(tr, -1, o)
}

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

// legLabel resolves what a leg says: the narrated line when one has landed for
// it, the heuristic label otherwise. HEAD always keeps its heuristic label —
// narration is for history, and the live leg is still changing its mind.
func legLabel(l journey.Leg, o TrailOpts) (string, bool) {
	if l.Current || len(o.Labels) == 0 {
		return l.Label, false
	}
	if text := strings.TrimSpace(o.Labels[legKey(o.SessionKey, l)]); text != "" {
		return text, true
	}
	return l.Label, false
}

// legRow: glyph, class verb, label, and the age held at the right margin. HEAD
// points at itself — `← 3m` — because it is the only line that is still moving.
// The arrow is a "you are here", not a direction of travel: it means the same
// thing with the past above it as it did with the past below.
// A narrated leg spends the verb column on its own words: the prose already
// says what the class was for, and the glyph keeps the class's tint.
func legRow(l journey.Leg, label string, narrated bool, now time.Time, width int, pulse bool) string {
	glyph := glyphLeg
	age := relAge(now, l.Start)
	if l.Current {
		glyph = glyphHead
		if pulse {
			glyph = glyphBreath
		}
		age = "← " + age
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
	if labelWidth < trailMinLabel {
		// Too narrow for a label: the verb and the age still answer "what, when".
		return head + padLeft(dimStyle.Render(age), width-(trailPrefixWidth-1))
	}
	labelText := textStyle.Render(pad(clip(label, labelWidth), labelWidth))
	return head + " " + labelText + badgeStyle(badge).Render(padLeft(badge, badgeW)) +
		" " + dimStyle.Render(age)
}

func badgeStyle(badge string) lipgloss.Style {
	if strings.Contains(badge, "✗") {
		return textStyle
	}
	return dimStyle
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
func promptRow(p journey.Prompt, now time.Time, width int) string {
	age := relAge(now, p.At)
	textWidth := width - 2 - 1 - len([]rune(age))
	if textWidth < trailMinLabel {
		return dimStyle.Render(glyphPrompt) + padLeft(dimStyle.Render(age), width-1)
	}
	text := textStyle.Render(pad(clip(`"`+p.Text+`"`, textWidth), textWidth))
	return dimStyle.Render(glyphPrompt) + " " + text + " " + dimStyle.Render(age)
}

// legDetails is a leg's Lv2 body: its waypoints in the order they happened,
// then — for the classes that touch files — what it touched. Every row is dim:
// at Lv2 the legs are still the structure and the waypoints are what hangs off
// them.
func legDetails(l journey.Leg, width int, b *trailBuilder) []detailRow {
	body := width - trailWayWidth
	out := make([]detailRow, 0, len(l.Waypoints)+1)
	bugs := 0
	for _, w := range l.Waypoints {
		// The index is spent whether or not the panel is wide enough to draw
		// the row: TrailRows counts moments, not columns.
		sel := b.pick()
		if w.Kind == journey.WaypointBug {
			bugs++
		}
		if body < trailMinLabel {
			continue
		}
		out = append(out, detailRow{text: waypointBody(w, bugs, body), sel: sel})
	}
	if body < trailMinLabel {
		return nil
	}
	if row := touchedBody(l, body); row != "" {
		out = append(out, detailRow{text: row, sel: -1})
	}
	return out
}

// waypointBody decorates one waypoint: the Kind picks the prefix, the Text
// never carries one of its own. A failing test is the only thing on the trail
// allowed to spark — and the `✗` says it without the colour.
func waypointBody(w journey.Waypoint, bug, width int) string {
	switch w.Kind {
	case journey.WaypointTestFail:
		return stuckStyle.Render(wayFail) + " " + dimStyle.Render(clip(w.Text, width-2))
	case journey.WaypointBug:
		prefix := fmt.Sprintf("bug%d ", bug)
		return dimStyle.Render(prefix + clip(w.Text, width-len(prefix)))
	default:
		return dimStyle.Render(clip(w.Text, width))
	}
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
	if len(l.Files) < 2 {
		return ""
	}
	return dimStyle.Render(clip("touched "+strings.Join(l.Files, " · "), width))
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
		}
		labelWidth := width - trailForkWidth - 2
		if labelWidth < trailMinLabel {
			continue
		}
		label := dimStyle.Render(pad(clip(branchName(br.Label), labelWidth), labelWidth))
		b.selNode(sel, ruleStyle.Render(railFork)+textStyle.Render(glyphBranch)+" "+
			label+" "+dimStyle.Render(mark))
		drawn++

		if o.Level >= levelWaypoints && strings.TrimSpace(br.Report) != "" {
			if body := width - trailWayWidth; body >= trailMinLabel {
				b.details([]detailRow{{text: dimStyle.Render(clip(br.Report, body)), sel: -1}})
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
	if h > 2 {
		rows = append(rows, trailRows(m.trail, m.trailOpts(w, h-2))...)
	}
	return rows
}

// trailOpts is the model's state as the renderer wants it.
func (m *Model) trailOpts(w, h int) TrailOpts {
	return TrailOpts{
		Todos:      m.todos,
		Labels:     m.labels,
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

// trailTitle: whose trail this is, and how deep we are in it.
func (m *Model) trailTitle(w int) string {
	name := "—"
	if s, ok := m.selected(); ok {
		name = sessionName(s.Info)
	}
	level := "[Lv1]"
	switch {
	case m.level >= levelReader:
		level = "[Lv3]"
	case m.level >= levelWaypoints:
		level = "[Lv2]"
	}
	// Scrolled off the present, the title says so: the trail is no longer
	// showing the newest work, and `G` is the way back to it.
	right := level
	if !m.trailPinned {
		right = "↓ G  " + level
	}
	mark := m.titleMark(panelTrail)
	body := w - 1
	left := m.titleStyleFor(panelTrail).Render(clip("TRAIL · "+name, body-lipgloss.Width(right)-1))
	gap := body - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return mark + left + strings.Repeat(" ", gap) + dimStyle.Render(right)
}
