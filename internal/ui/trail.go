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
	glyphBranch = "◈" // a subagent lane
	glyphGhost  = "◌" // planned work: a todo that has not happened yet
	railStroke  = "│" // rail between two nodes
	railEnd     = "╵" // the rail's tail, above the oldest node
	railGhost   = "┊" // the dashed rail, above HEAD, into the future
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

	// SessionID names whose journey this is. The Labels map is keyed by the
	// narrator's LegKey — session, leg start and class — so the renderer needs
	// the session to look a leg up. "" simply finds nothing.
	SessionID string

	Now           time.Time
	Width, Height int
	Level         int // 1, 2 or 3 (the trail renders identically at 2 and 3)
	Cursor        int // index into TrailRows at Lv2+; -1 = no cursor
}

// RenderTrail draws the trail into a width×height block: newest at the top like
// `git log`, one line per node, the rail running down the left. Level 1 is the
// graph of legs; level 2 unfolds each leg's waypoints beneath it. Pending todos
// hang above HEAD as ghosts at either level — the journey's future. The frame is
// exactly height lines; when the journey is longer than the panel the oldest
// rows fall off the bottom, so HEAD is never the thing that gets cropped.
func RenderTrail(tr journey.Trail, o TrailOpts) string {
	return strings.Join(fit(trailRows(tr, o), o.Height), "\n")
}

// trailRows is RenderTrail without the bottom padding, so a column that wants
// to know where its content stops can ask.
func trailRows(tr journey.Trail, o TrailOpts) []string {
	width, height := o.Width, o.Height
	if width < trailPrefixWidth || height < 1 {
		return nil
	}
	nodes := trailNodes(tr)
	if len(nodes) == 0 {
		return crop(trailEmptyRows(width), height)
	}

	b := trailBuilder{cursor: -1}
	if o.Level >= levelWaypoints {
		b.cursor = o.Cursor
	}
	b.ghosts(o.Todos, width, height)
	b.journey(tr, nodes, o)

	if len(tr.Legs) == 0 {
		// A journey that has only been asked for: say what comes next rather
		// than leaving the panel half empty (SPEC §4).
		b.node("")
		b.node(dimStyle.Render(clip("scouting will appear here", width)))
	}
	return crop(b.render(height), height)
}

// trailEmptyRows is the designed empty state: never a blank panel (SPEC §4).
func trailEmptyRows(width int) []string {
	return []string{
		dimStyle.Render(clip("◌ nothing yet", width)),
		"",
		dimStyle.Render(clip("scouting will appear here", width)),
	}
}

// trailLine is one row of the trail plus what the height budget needs to know
// about it. A detail row — a Lv2 waypoint, a touched list, a subagent's report —
// belongs to a group (the node it hangs under) and is drawn with the `├`/`└`
// that its position in the surviving group earns.
type trailLine struct {
	text    string // a node row's finished line, or a detail row's body
	group   int    // detail rows: which group they belong to; -1 for node rows
	head    bool   // this detail belongs to HEAD, so it is the last to go
	sel     int    // index into TrailRows, or -1 when the row is not selectable
	dropped bool
}

// detailRow is one Lv2 body row before it is hung off its node: its text, and
// whether the cursor may land on it.
type detailRow struct {
	text string
	sel  int // index into TrailRows, or -1
}

// trailBuilder assembles the rows, keeping the detail groups separable so the
// height budget can trim them before it touches the graph itself.
type trailBuilder struct {
	lines  []trailLine
	groups int

	cursor int // the selectable row to invert; -1 for none
	picked int // how many selectable rows have been handed out so far
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
func (b *trailBuilder) details(bodies []detailRow, head bool) {
	if len(bodies) == 0 {
		return
	}
	b.groups++
	for _, body := range bodies {
		b.lines = append(b.lines, trailLine{text: body.text, group: b.groups, head: head, sel: body.sel})
	}
}

// render applies the height budget and turns the lines into text: detail rows
// are dropped before node rows — the oldest leg's first, HEAD's last — and the
// last surviving detail of each group closes it with `└`.
func (b *trailBuilder) render(height int) []string {
	over := len(b.lines) - height
	for _, head := range []bool{false, true} {
		for i := len(b.lines) - 1; i >= 0 && over > 0; i-- {
			if l := b.lines[i]; l.group >= 0 && l.head == head && !l.dropped {
				b.lines[i].dropped = true
				over--
			}
		}
	}

	out := make([]string, 0, len(b.lines))
	for i, l := range b.lines {
		switch {
		case l.dropped:
		case l.group < 0:
			out = append(out, b.cursored(l, l.text))
		default:
			mark := wayTee
			if b.lastOfGroup(i) {
				mark = wayEnd
			}
			out = append(out, b.cursored(l, ruleStyle.Render(railStroke+"  "+mark)+" "+l.text))
		}
	}
	return out
}

// cursored inverts the row the Lv2 cursor stands on. The row is stripped back
// to its plain text first: a reset left over from the class tint would cancel
// the inversion halfway across the line. Inversion is the whole mark — it costs
// no colour and it survives NO_COLOR (SPEC §4).
func (b *trailBuilder) cursored(l trailLine, text string) string {
	if b.cursor < 0 || l.sel != b.cursor {
		return text
	}
	return cursorStyle.Render(strings.TrimRight(ansi.Strip(text), " "))
}

// lastOfGroup reports whether row i is the deepest surviving row of its group.
func (b *trailBuilder) lastOfGroup(i int) bool {
	for j := i + 1; j < len(b.lines); j++ {
		if b.lines[j].group == b.lines[i].group && !b.lines[j].dropped {
			return false
		}
	}
	return true
}

// ghosts draws the plan above HEAD: the next pending todo sits nearest the
// present and the rest stack upward into the future, on a dashed rail. Four at
// most; a longer plan says how much more of it there is. The block never takes
// more than half the panel — the future may not crowd out the present — and a
// plan with nothing pending draws nothing at all.
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

	if more := len(pending) - show; more > 0 {
		b.node(ruleStyle.Render(railGhost) + " " +
			dimStyle.Render(clip(fmt.Sprintf("+%d more", more), body)))
	}
	for i := show - 1; i >= 0; i-- {
		b.node(dimStyle.Render(glyphGhost + " " + clip(pending[i], body)))
		b.node(ruleStyle.Render(railGhost))
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

// journey lays the graph itself: every node newest first, its Lv2 detail
// beneath it, the subagents that forked off it, and the rail down to the next.
func (b *trailBuilder) journey(tr journey.Trail, nodes []trailNode, o TrailOpts) {
	last := len(nodes) - 1
	for i, n := range nodes {
		if n.leg >= 0 {
			leg := tr.Legs[n.leg]
			label, narrated := legLabel(leg, o)
			b.selNode(b.pick(), legRow(leg, label, narrated, o.Now, o.Width))
			if o.Level >= levelWaypoints {
				b.details(legDetails(leg, o.Width, b), leg.Current)
			}
		} else {
			b.node(promptRow(tr.Prompts[n.prompt], o.Now, o.Width))
		}

		forks := 0
		if n.leg >= 0 {
			forks = b.branches(tr, n.leg, o)
		}

		switch {
		case i == last:
			// The rail has nothing older to reach for.
		case i+1 == last:
			b.node(ruleStyle.Render(railEnd))
		case forks == 0:
			b.node(ruleStyle.Render(railStroke))
		}
	}
	// Branches that forked before any leg opened hang off the bottom of the rail.
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
// draws them — newest first, top-down. Legs, their waypoints (from Lv2 down)
// and the subagent lanes can be selected; prompts, rails, ghosts and the
// synthetic "touched" row cannot: they name no moment of their own. The
// enumeration is geometry-free, so a cursor index means the same thing at any
// width, and TrailOpts.Cursor indexes straight into this slice.
func TrailRows(tr journey.Trail, level int) []TrailRow {
	var out []TrailRow
	branches := func(after int) {
		for i := len(tr.Branches) - 1; i >= 0; i-- {
			if br := tr.Branches[i]; br.AfterLeg == after {
				out = append(out, TrailRow{Time: br.Start, Kind: "branch",
					Text: branchName(br.Label), Leg: after})
			}
		}
	}

	for _, n := range trailNodes(tr) {
		if n.leg < 0 {
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

// trailNodes interleaves legs and prompts, newest first. A leg that starts on
// the same tick as the prompt that provoked it sits above it: the prompt came
// first (decision log #1 — newest on top).
func trailNodes(tr journey.Trail) []trailNode {
	nodes := make([]trailNode, 0, len(tr.Prompts)+len(tr.Legs))
	for i, p := range tr.Prompts {
		nodes = append(nodes, trailNode{at: p.At, leg: -1, prompt: i})
	}
	for i, l := range tr.Legs {
		nodes = append(nodes, trailNode{at: l.Start, leg: i, prompt: -1})
	}
	// Ascending first — stable, so ties keep prompts before legs — then flipped.
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].at.Before(nodes[j].at) })
	for i, j := 0, len(nodes)-1; i < j; i, j = i+1, j-1 {
		nodes[i], nodes[j] = nodes[j], nodes[i]
	}
	return nodes
}

// legKey is the identity the narrator's cache is keyed by: session, the leg's
// start and its class (M3 contract, narrator.LegKey). The renderer forms it
// itself so the ui package stays free of the narrator's plumbing — the format
// is contract, not implementation detail.
func legKey(sessionID string, l journey.Leg) string {
	return sessionID + "/" + strconv.FormatInt(l.Start.UnixNano(), 10) + "/" + l.Class.String()
}

// legLabel resolves what a leg says: the narrated line when one has landed for
// it, the heuristic label otherwise. HEAD always keeps its heuristic label —
// narration is for history, and the live leg is still changing its mind.
func legLabel(l journey.Leg, o TrailOpts) (string, bool) {
	if l.Current || len(o.Labels) == 0 {
		return l.Label, false
	}
	if text := strings.TrimSpace(o.Labels[legKey(o.SessionID, l)]); text != "" {
		return text, true
	}
	return l.Label, false
}

// legRow: glyph, class verb, label, and the age held at the right margin. HEAD
// points at itself — `← 3m` — because it is the only line that is still moving.
// A narrated leg spends the verb column on its own words: the prose already
// says what the class was for, and the glyph keeps the class's tint.
func legRow(l journey.Leg, label string, narrated bool, now time.Time, width int) string {
	glyph := glyphLeg
	age := relAge(now, l.Start)
	if l.Current {
		glyph = glyphHead
		age = "← " + age
	}
	head := classStyle(l.Class).Render(glyph + " " + pad(l.Class.String(), trailVerbWidth))

	if narrated {
		// Narrated: one phrase across the verb and label columns.
		textWidth := width - 2 - 1 - len([]rune(age))
		if textWidth < trailMinLabel {
			return classStyle(l.Class).Render(glyph) + padLeft(dimStyle.Render(age), width-1)
		}
		return classStyle(l.Class).Render(glyph) + " " +
			textStyle.Render(pad(clip(label, textWidth), textWidth)) + " " + dimStyle.Render(age)
	}

	labelWidth := width - trailPrefixWidth - 1 - len([]rune(age))
	if labelWidth < trailMinLabel {
		// Too narrow for a label: the verb and the age still answer "what, when".
		return head + padLeft(dimStyle.Render(age), width-(trailPrefixWidth-1))
	}
	labelText := textStyle.Render(pad(clip(label, labelWidth), labelWidth))
	return head + " " + labelText + " " + dimStyle.Render(age)
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
// ones that forked before any leg opened), newest first, each hanging off the
// rail at the node it left from. At Lv2 a returned agent also says what it
// found. It returns how many lanes it drew.
func (b *trailBuilder) branches(tr journey.Trail, after int, o TrailOpts) int {
	width, drawn := o.Width, 0
	for i := len(tr.Branches) - 1; i >= 0; i-- {
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
				b.details([]detailRow{{text: dimStyle.Render(clip(br.Report, body)), sel: -1}}, false)
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

// crop keeps the newest rows — the head of the list — and drops the tail.
func crop(rows []string, height int) []string {
	if len(rows) > height {
		return rows[:height]
	}
	return rows
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
		Todos:     m.todos,
		Labels:    m.labels,
		SessionID: m.selectedID,
		Now:       m.now,
		Width:     w,
		Height:    h,
		Level:     m.level,
		Cursor:    m.cursor,
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
	left := dimStyle.Render(clip("TRAIL · "+name, w-len(level)-1))
	gap := w - lipgloss.Width(left) - len(level)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + dimStyle.Render(level)
}
