package ui

import (
	"strings"

	"github.com/deephanson94/compass/internal/journey"
)

// helpKeys is the M2 keymap. Keys that arrive in later milestones are named
// here only when they already do something.
var helpKeys = [][2]string{
	{"1 – 9", "select a session"},
	{"j / k", "move down / up (↓ ↑ too) · h / l the next column, or session"},
	{"enter", "attach to its pane, at any level (prefix d returns)"},
	{"g", "grab the session waiting longest, and attach"},
	{"A", "browse the archive — every past session, by project"},
	{"tab", "zoom in: board → session → reader"},
	{"⇧ tab", "zoom out, back to the board (esc too)"},
	{"ctrl+d/u", "half a page: the trail, or the reader once the keys are in it"},
	{"G", "back to the present: the newest row"},
	{"[ ]", "previous / next prompt — the chapters of a trail"},
	{"m", "the live tmux pane beside the trail, instead of the conversation"},
	{"r", "quick reply: type a stock line into the session's pane (a digit picks)"},
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
	return helpLinesFor(w, h, true)
}

func helpLinesFor(w, h int, board bool) []string {
	keys := helpKeyLinesFor(w, board)
	// Two columns only when the keys themselves fit: on a body too short for
	// them, splitting the width buys nothing and costs every key its tail.
	if w >= helpTwoCol && h >= len(keys) {
		left := w/2 - gutterWidth
		return joinColumns(h, []column{
			{left, helpKeyLinesFor(left, board)},
			{w - left - gutterWidth, helpLegendLines(w-left-gutterWidth, true)},
		})
	}
	lines := helpKeyLinesFor(w, board)
	legend := helpLegendLines(w, false)
	if !board {
		// No board on this terminal: its legend line would describe a
		// brightness the person never sees.
		kept := legend[:0]
		for _, l := range legend {
			if !strings.Contains(l, "board:") {
				kept = append(kept, l)
			}
		}
		legend = kept
	}
	if h > 0 && len(lines)+1+len(legend) > h {
		// Too short for the whole legend: keep what a reader cannot infer —
		// the glyphs and the class names — and drop the sentences around
		// them and the line of air, before any key is cut. A reader who
		// cannot reach the keys cannot leave the overlay.
		legend = helpLegendCore(legend)
		if len(lines)+len(legend) > h {
			// Still too tall: the lane and plan glyphs share one row, and
			// the compaction mark rides with them.
			legend = helpLegendFold(legend, w)
		}
		if len(lines)+len(legend) > h && len(lines) > 1 && lines[1] == "" {
			// And the line of air under "keys" goes before any glyph does.
			lines = append(lines[:1], lines[2:]...)
		}
	} else {
		lines = append(lines, "")
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
		for _, keep := range []string{"fleet:", "trail:  ", "⌀ back", "◌ planned", "scout"} {
			if strings.Contains(l, keep) {
				core = append(core, l)
				break
			}
		}
	}
	return core
}

// helpLegendFold puts the ⌀ and ◌ lines on one row, with ⟲ beside them: the
// glyphs survive, the sentences around them do not.
func helpLegendFold(legend []string, w int) []string {
	var out []string
	folded := false
	for _, l := range legend {
		switch {
		case strings.Contains(l, "⌀ back") || strings.Contains(l, "◌ planned"):
			if !folded {
				out = append(out, dimStyle.Render(clip("        ⌀ back, empty · ◌ planned · ⟲ compacted · ✗ · 3rd time", w)))
				folded = true
			}
		default:
			out = append(out, l)
		}
	}
	return out
}

func helpKeyLines(w int) []string {
	return helpKeyLinesFor(w, true)
}

// helpKeyLinesFor is the key list for a terminal with or without a board:
// on one too narrow for it, "zoom in: board → trail" describes a level the
// person cannot reach.
func helpKeyLinesFor(w int, board bool) []string {
	lines := []string{textStyle.Render("keys"), ""}
	for _, k := range helpKeys {
		key, what := k[0], k[1]
		if !board {
			switch key {
			case "j / k":
				what = "move down / up (↓ ↑ too)"
			case "tab":
				what = "zoom in: trail → legs → reader"
			case "⇧ tab":
				what = "zoom out (esc too)"
			}
		}
		if what == "" {
			continue // not a key on this terminal
		}
		lines = append(lines, dimStyle.Render(pad(key, 10))+textStyle.Render(clip(what, w-10)))
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
		dimStyle.Render(clip("        ◈ ⋯ out · ✓ back, finding beneath · ⌀ back, empty", w)),
		dimStyle.Render(clip("        →3 a live session whose prompt is this lane's", w)),
		dimStyle.Render(clip("        ◌ planned — Claude's own next moves", w)),
		dimStyle.Render(clip("        ◉ 3/12 — the 3rd of 12 prompts · [ ] steps them", w)),
		dimStyle.Render(clip("        ⟲ compacted — the conversation was folded into a summary here", w)),
		dimStyle.Render(clip("        ✗ test · 3rd time — the same test has failed in three legs", w)),
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
