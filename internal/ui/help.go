package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

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
	{"r", "reply: options, stock lines, a typed line, stop; a dead session's remedy"},
	{"x", "hide a session — A lists it, x there brings it back"},
	{"a", "ask: a claude grounded in this session's transcript"},
	{"space", "reader: fold / unfold a tool output"},
	{"/ n N", "search: the fleet from a list or the deck; the text in the reader"},
	{"esc", "one level out · on the board or a list, a standing search clears first"},
	{"?", "this help"},
	{"q", "quit"},
}

// helpTwoCol is the width at which the overlay splits: below it the keys would
// be clipped to nonsense, so the legend goes compact instead.
const helpTwoCol = 150

// helpSplitMin is the width below which two columns are never worth it.
const helpSplitMin = 104

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
	// Two columns at a width that holds them whole, or at any width past
	// the split where one column would have to cut the keys: a 120x34
	// split clipped both halves, a 120x22 single column had no legend.
	oneColumn := len(keys) + 1 + len(helpLegendCore(helpLegendLines(w, false)))
	if (w >= helpTwoCol || (w >= helpSplitMin && h < oneColumn)) && h >= len(keys) {
		// The split follows the keys' longest line, not a fraction of the
		// width: a fixed half pads one column while it clips the other.
		left := helpKeysWidth(board)
		if left > w*3/5 {
			left = w*3/5 - gutterWidth
		}
		right := w - left - gutterWidth
		legend := helpLegendWrapped(right, true, h) // definitions wrap into the rows that are free
		return joinColumns(h, []column{
			{left, helpKeyLinesFor(left, board)},
			{right, legend},
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
		// Cut from the middle of the keys, never the tail: the way out
		// (esc, q) and the fleet's glyph line are what a newcomer on an
		// 80x24 split needs from this screen before anything else.
		keep := len(lines) - h
		var tail []string
		for _, l := range lines {
			plain := ansi.Strip(l)
			keep := strings.Contains(l, "fleet:")
			for _, k := range []string{"esc ", "q ", "? ", "/ n N", "x ", "r ", "space"} {
				keep = keep || strings.HasPrefix(plain, k)
			}
			if keep {
				tail = append(tail, l) // the way out, the fleet's glyphs, and the keys pressed at this width
			}
		}
		for len(tail) > 0 && h-len(tail) < 3 {
			tail = tail[1:] // the first key rows outrank the kept tail on a tiny body
		}
		cut := lines[:max(h-len(tail), 0)]
		var out []string
		for _, l := range cut {
			if !contains(tail, l) {
				out = append(out, l)
			}
		}
		lines = append(out, tail...)
		_ = keep
		if len(lines) > h {
			lines = lines[:h]
		}
	}
	return lines
}

func contains(list []string, s string) bool {
	for _, l := range list {
		if l == s {
			return true
		}
	}
	return false
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

// helpKeysWidth is the width of the keys column when nothing in it is cut.
func helpKeysWidth(board bool) int {
	w := 0
	for _, l := range helpKeyLinesFor(1000, board) {
		if n := ansi.StringWidth(l); n > w {
			w = n
		}
	}
	return w
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
				out = append(out, dimStyle.Render(clip("        ⌀ back, empty · ◌ planned · ⟲ compacted · 2nd failure · →3 · ↳ since you looked · ↪ sent · │ you were here · ⌁ its pane · unread", w)))
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
			case "esc":
				what = "one level out · from a list, a standing search clears first"
			case "m":
				what = "" // the mirror needs the board's width
			}
		}
		if what == "" {
			continue // not a key on this terminal
		}
		if i := strings.Index(what, "; "); i > 0 && len([]rune(what)) > w-10 {
			what = what[:i] // the aside goes whole before the sentence is cut
		}
		lines = append(lines, dimStyle.Render(pad(key, 10))+textStyle.Render(clip(what, w-10)))
	}
	return lines
}

// helpLegendLines is what the glyphs and the class tints mean. Spelled out when
// there is room; names only when there is not, because a reader who cannot see
// which class a leg is has lost the whole point of Lv1 (SPEC §2.2).
// helpLegendRaw is the legend's text, unclipped: the glyph lines carry their
// indent so a wrapped continuation can hang under the glyph.
func helpLegendRaw() []string {
	return []string{
		"compass observes; enter hands you the session.",
		"",
		focusMark + " marks the panel your keys are in — tab moves it",
		"fleet:  ● working  ▲ needs\u00a0you  ◍ stuck  ↻ circling  ⊘ dead\u00a0on\u00a0the\u00a0API  ○ idle",
		"        ⌁ dev:1.0 — its tmux pane · unread — finished today, not yet opened",
		"trail:  ◉ prompt  ◆ leg  ● now, \"for 2h\"  ◈ subagent",
		"        ◈ ⋯ out · ✓ back, finding beneath · ⌀ back, empty",
		"        ◌ planned — Claude's own next moves · →3 a live session on this lane",
		"        ◉ 3/12 — the 3rd of 12 prompts · [ ] steps them",
		"        ⟲ context compacted — a summary below · 16⚑ 10✗ 2⟲ ships · red · compactions",
		"        · 2nd failure — the same test in two legs · ? — no verdict parsed · edited since — files touched after that run",
		"        on you 40m today — its waits for your next prompt (3h+ = away)",
		"        ↪ sent — a line compass typed · ↪ answered 2 — the menu's digit · ↩ result of X — landed late; it is X's",
		"        │ you were here — the read-line · ↳ what came after · ⚠ two sessions, one thing",
		"board:  columns for what owes you, in that order; the rest in the strip",
		"",
		"every leg is one of seven classes, named on its row:",
	}
}

func helpLegendLines(w int, roomy bool) []string {
	var lines []string
	for _, l := range helpLegendRaw() {
		if l == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, dimStyle.Render(clip(l, w)))
	}
	return helpLegendClasses(lines, w, roomy)
}

// helpLegendWrapped is the legend with every line that would clip re-flowed
// onto continuation rows, hung under its glyph.
func helpLegendWrapped(w int, roomy bool, h int) []string {
	raw := helpLegendRaw()
	budget := h - len(raw) - len(legClasses) // the rows free for continuations
	var lines []string
	for _, l := range raw {
		if l == "" {
			lines = append(lines, "")
			continue
		}
		if ansi.StringWidth(l) <= w {
			lines = append(lines, dimStyle.Render(l))
			continue
		}
		indent := len(l) - len(strings.TrimLeft(l, " "))
		first := l[:indent]
		text := l[indent:]
		if i := strings.Index(text, ":  "); i > 0 && indent == 0 {
			// "fleet:  …" — the continuation hangs under the first glyph.
			first, text = text[:i+3], text[i+3:]
			indent = len([]rune(first))
		}
		rows := wrapPrefix(text, first, strings.Repeat(" ", indent+2), w)
		if extra := len(rows) - 1; extra > budget {
			lines = append(lines, dimStyle.Render(clip(l, w))) // no rows left: this one clips
			continue
		} else {
			budget -= extra
		}
		for _, row := range rows {
			lines = append(lines, dimStyle.Render(row))
		}
	}
	return helpLegendClasses(lines, w, roomy)
}

func helpLegendClasses(lines []string, w int, roomy bool) []string {
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
