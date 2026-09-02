package ui

import (
	"strings"

	"github.com/deephanson94/compass/internal/journey"
)

// helpKeys is the M2 keymap. Keys that arrive in later milestones are named
// here only when they already do something.
var helpKeys = [][2]string{
	{"1 – 9", "select a session"},
	{"j / k", "move down / up (↓ ↑ too)"},
	{"enter", "attach to its pane, at any level (prefix d returns)"},
	{"g", "grab the session waiting longest, and attach"},
	{"A", "browse the archive — every past session, by project"},
	{"tab", "zoom in: board → trail → legs → reader"},
	{"⇧ tab", "zoom out, back to the board (esc too)"},
	{"ctrl+d/u", "half a page: the trail at Lv1, the reader at Lv3"},
	{"G", "back to the present: the newest row"},
	{"m", "show / hide the live mirror (on one trail)"},
	{"a", "ask: a claude grounded in this session's transcript"},
	{"space", "reader: fold / unfold a tool output"},
	{"/ n N", "reader: search, next, previous"},
	{"esc", "one level out (a standing search clears first)"},
	{"?", "this help"},
	{"q", "quit"},
}

// helpTwoCol is the width at which the overlay splits: below it the keys would
// be clipped to nonsense, so the legend goes compact instead.
const helpTwoCol = 104

// helpLines renders the help overlay in place of the deck body: same margins,
// same alignment, nothing new to learn.
//
// It has to fit. The keys are the half a reader needs most, so they are never
// what gets cut: given the width, the legend moves alongside them; without it,
// the legend loses its explanations rather than the keys losing their rows.
func helpLines(w, h int) []string {
	keys := helpKeyLines(w)
	// Two columns only when the keys themselves fit: on a body too short for
	// them, splitting the width buys nothing and costs every key its tail.
	if w >= helpTwoCol && h >= len(keys) {
		left := w/2 - gutterWidth
		return joinColumns(h, []column{
			{left, helpKeyLines(left)},
			{w - left - gutterWidth, helpLegendLines(w-left-gutterWidth, true)},
		})
	}
	lines := append(helpKeyLines(w), "")
	legend := helpLegendLines(w, false)
	if h > 0 && len(lines)+len(legend) > h {
		// Too short for the whole legend: keep what a reader cannot infer —
		// the glyphs and the class names — and drop the sentences around
		// them, before any key is cut. A reader who cannot reach the keys
		// cannot leave the overlay.
		legend = helpLegendCore(legend)
	}
	lines = append(lines, legend...)
	if h > 0 && len(lines) > h {
		lines = lines[:h]
	}
	return lines
}

// helpLegendCore is the legend with its prose removed: the fleet and trail
// glyph lines and the class names, which are the lines nothing else on screen
// explains. Everything else in the legend is a sentence a reader can live
// without on a short terminal.
func helpLegendCore(legend []string) []string {
	var core []string
	for _, l := range legend {
		for _, keep := range []string{"fleet:", "trail:  ", "seven classes", "scout"} {
			if strings.Contains(l, keep) {
				core = append(core, l)
				break
			}
		}
	}
	return core
}

func helpKeyLines(w int) []string {
	lines := []string{textStyle.Render("keys"), ""}
	for _, k := range helpKeys {
		lines = append(lines, dimStyle.Render(pad(k[0], 10))+textStyle.Render(clip(k[1], w-10)))
	}
	return lines
}

// helpLegendLines is what the glyphs and the class tints mean. Spelled out when
// there is room; names only when there is not, because a reader who cannot see
// which class a leg is has lost the whole point of Lv1 (SPEC §2.2).
func helpLegendLines(w int, roomy bool) []string {
	lines := []string{
		dimStyle.Render(clip("compass observes; enter hands you the session.", w)),
		"",
		dimStyle.Render(clip(focusMark+" marks the panel your keys are in — tab moves it", w)),
		dimStyle.Render(clip("fleet:  ● working  ▲ needs you  ◍ stuck  ○ idle", w)),
		dimStyle.Render(clip("trail:  ◉ prompt  ◆ leg  ● now, \"for 2h\"  ◈ subagent", w)),
		dimStyle.Render(clip("        ◈ ⋯ still out · ✓ back, with its finding", w)),
		dimStyle.Render(clip("        ◌ planned — Claude's own next moves", w)),
		dimStyle.Render(clip("board:  bright = something unread · dim = read, or a day old", w)),
		"",
		dimStyle.Render(clip("every leg is one of seven classes, named on its row:", w)),
	}
	if !roomy {
		var names []string
		for _, c := range legClasses {
			names = append(names, classStyle(c.class).Render(c.class.String()))
		}
		return append(lines, "  "+strings.Join(names, "  "))
	}
	for _, c := range legClasses {
		lines = append(lines,
			classStyle(c.class).Render(pad("  "+c.class.String(), 10))+
				dimStyle.Render(clip(c.means, w-10)))
	}
	return lines
}

// legClasses is the Lv1 classification (SPEC §2.2), in the order a session
// tends to move through it. WAIT from the spec's table is missing on purpose:
// waiting is a session state, not a span of work, and it lives in the fleet
// column as ▲ or ○. The colour is reinforcement — the word on the row
// is what carries the meaning — so the legend names each one either way.
var legClasses = []struct {
	class journey.Class
	means string
}{
	{journey.Scout, "reading around: Read, Grep, Glob, subagents"},
	{journey.Design, "planning: plan mode, specs before code"},
	{journey.Build, "making: edits and new files"},
	{journey.Fix, "repairing: edit-run-edit, errors in results"},
	{journey.Test, "checking: test runs, with their results parsed"},
	{journey.Ship, "landing it: commits, pushes, PRs"},
	{journey.Docs, "writing it down: markdown, comments, READMEs"},
}
