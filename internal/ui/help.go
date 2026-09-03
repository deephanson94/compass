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
	{"g", "grab the oldest ▲ needs-you and attach — a ⊘ is skipped"},
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

// helpOpts is what the deck tells the help about itself: whether a board
// fits, which keys it would refuse, and the keymap its own footer is
// carrying — the last so a body too short for every row keeps the rows for
// the keys the person can see being offered.
type helpOpts struct {
	board   bool
	refused []string
	keymap  string
}

func helpLinesFor(w, h int, board bool, refused ...string) []string {
	return helpLinesWith(w, h, helpOpts{board: board, refused: refused})
}

// helpOffered says whether the deck's own footer is naming this key now.
func helpOffered(key, keymap string) bool {
	if keymap == "" {
		return false
	}
	for _, f := range map[string][]string{
		"j / k": {"j/k"}, "enter": {"enter"}, "g": {"g grab"},
		"tab": {"tab deeper", "tab session", "tab reader"}, "⇧ tab": {"⇧tab"}, "[ ]": {"[ ]"},
		"G": {"G is the present"}, "? / q": {"? help"}, "x / A": {"x hide", "x unhide", "A fleet", "A browses"},
		"m": {"m live pane", "m conversation"}, "r": {"r reply"}, "x": {"x hide", "x unhide"},
		"a": {"a ask"}, "space": {"space unfold"}, "/ n N": {"/ search", "n/N"},
		"A": {"A live fleet", "A fleet", "A browses", "A, then x"},
	}[key] {
		if strings.Contains(keymap, f) {
			return true
		}
	}
	return false
}

func helpLinesWith(w, h int, o helpOpts) []string {
	board, refused := o.board, o.refused
	keys := helpKeyLinesFor(w, board, refused...)
	// Two columns only when the keys themselves fit: on a body too short for
	// them, splitting the width buys nothing and costs every key its tail.
	// Two columns at a width that holds them whole, or at any width past
	// the split where one column would have to cut the keys: a 120x34
	// split clipped both halves, a 120x22 single column had no legend.
	oneColumn := len(keys) + 1 + 5 // the keys, a line of air, the glyph lines a column cannot do without
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
			{left, helpKeyLinesFor(left, board, refused...)},
			{right, legend},
		})
	}
	lines := helpKeyLinesFor(w, board, refused...)
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
		full := legend
		legend = helpLegendCore(legend)
		if len(lines)+len(legend) > h {
			// Still too tall: the lane and plan glyphs share one row, and
			// the compaction mark rides with them. A row left over after
			// that takes the tag's line.
			legend = helpLegendFold(legend, w)
		}
		if len(lines)+len(legend) >= h && len(lines) > 1 && lines[1] == "" && len(legend) < len(full) {
			// No row to spare and a legend line out: the air under
			// "keys" buys the tag's line back — the 100-column help kept
			// its blank row while ⌁ and unread went undefined.
			lines = append(lines[:1], lines[2:]...)
		}
		if len(lines)+len(legend) < h {
			// Rows to spare: the sentences come back in their own order,
			// as many as fit — a 120x34 help had three free rows under a
			// legend missing the marks on its own board.
			legend = helpLegendFill(legend, full, h-len(lines))
		}
		if len(lines)+len(legend) > h && len(lines) > 1 && lines[1] == "" {
			// And the line of air under "keys" goes before any glyph does.
			lines = append(lines[:1], lines[2:]...)
		}
	} else {
		lines = append(lines, "")
	}
	// One gloss per mark: the folded row leads with the tag only where the
	// tag's own row did not survive, and gives that lead to the trace when
	// it did — `↪` stood on thirty-two rows and was defined at no width
	// below the board's.
	if joined := strings.Join(legend, "\n"); strings.Contains(joined, "dev:1.0") {
		for i, l := range legend {
			if plain := ansi.Strip(l); strings.Contains(plain, "⌁ pane · unread · ") {
				legend[i] = dimStyle.Render(shedClauses(strings.Replace(plain, "⌁ pane · unread · ", "", 1), w))
			}
		}
	}
	lines = append(lines, legend...)
	if h > 0 && len(lines) > h && o.keymap != "" {
		// A key row the deck's own footer is not offering goes before any
		// row it is: an 80-column help kept `g`, named on no footer there,
		// while `a`, `space` and the search — named on nine between them —
		// were cut for the room. The order is how guessable the key is
		// without its row.
		order := append(append([]string(nil), o.refused...), "A", "g", "/ n N", "G", "ctrl+d/u", "⇧ tab", "m", "tab", "tab/⇧tab", "x", "x / A", "r", "a", "[ ]", "space")
		for _, key := range order {
			if len(lines) <= h {
				break
			}
			if !refuses(o.refused, key) && helpOffered(key, o.keymap) {
				continue
			}
			for i, l := range lines {
				if plain := ansi.Strip(l); len(plain) > 10 && strings.TrimSpace(plain[:10]) == key {
					lines = append(lines[:i], lines[i+1:]...)
					break
				}
			}
		}
	}
	if h > 0 && len(lines) > h {
		// Cut from the middle of the keys, never the tail: the way out
		// (esc, q) and the fleet's glyph line are what a newcomer on an
		// 80x24 split needs from this screen before anything else.
		keep := len(lines) - h
		var tail []string
		for _, l := range lines {
			plain := ansi.Strip(l)
			keep := strings.Contains(l, "fleet:") || strings.Contains(l, "trail:  ") || strings.Contains(l, "you were here") || strings.Contains(l, "scout")
			// The keys the deck's own footer is offering outrank the rest:
			// an 80-column help named `g`, which no footer there offers,
			// while `x` — on nine of them — was cut for the room.
			for _, k := range []string{"esc ", "q ", "? "} {
				keep = keep || strings.HasPrefix(plain, k)
			}
			if !keep && len(plain) > 10 {
				keep = helpOffered(strings.TrimSpace(plain[:10]), o.keymap)
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
		// The lanes' row did not survive: the trail's own row carries them
		// in place of the bare "◈ subagent" — at 80 the board shows all
		// three and the help defined none.
		joined := strings.Join(lines, "\n")
		for i, l := range lines {
			plain := ansi.Strip(l)
			switch {
			case !strings.Contains(joined, "⋯ out") && strings.Contains(l, "trail:  "):
				if lanes := strings.Replace(plain, "◈ subagent", "◈ ⋯ out · ✓ back · ⌀ back, empty", 1); ansi.StringWidth(lanes) <= w {
					lines[i] = dimStyle.Render(lanes)
				}
			case !strings.Contains(joined, "⌁ ") && strings.Contains(l, "you were here"):
				// Nor the tag's: the read-line's row carries it — the first
				// row under FLEET on every 80-column frame.
				if tag := plain + " · ⌁ its tmux pane"; ansi.StringWidth(tag) <= w {
					lines[i] = dimStyle.Render(tag)
				}
			}
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
		// The class names are not here: they are plain words on every leg
		// row, and their row cost the narrow help the lanes.
		for _, keep := range []string{"fleet:", "trail:  ", "⌀ back", "◌ planned", "you were here"} {
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
				// The tag and unread first — the words every namesake and
				// every finished row wears — and the clauses go whole.
				// The tag and unread lead: every row wears one or says "no
				// pane", where the lanes belong to a lead that delegates.
				fold := "        ⌁ pane · unread · ↪ sent · ⟲ compacted · ◈ ⋯ out · ✓ back · ⌀ empty · ◌ planned · │ you were here"
				if strings.Contains(strings.Join(legend, "\n"), "dev:1.0") {
					// The tag's own row survived: this one need not say it
					// twice, and the room goes to the marks below it.
					fold = "        ◈ ⋯ out · ✓ back · ⌀ back, empty · ◌ planned · ⟲ compacted · ↪ sent · │ you were here"
				}
				out = append(out, dimStyle.Render(shedClauses(fold, w)))
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

// refuses says whether the deck would refuse this key right now — a key it
// refuses is not a key on this terminal, and on a body too short for every
// row it is the first to go: a newcomer on a fleet of one read "g grab" and
// "x hide" in the help and got a refusal from both, while `[ ]` and `a`,
// which the deck's own footer offered, were cut for the room.
func refuses(refused []string, key string) bool {
	for _, r := range refused {
		if r == key {
			return true
		}
	}
	return false
}

// helpKeyLinesFor is the key list for a terminal with or without a board:
// on one too narrow for it, "zoom in: board → trail" describes a level the
// person cannot reach.
func helpKeyLinesFor(w int, board bool, refused ...string) []string {
	lines := []string{textStyle.Render("keys"), ""}
	for _, k := range helpKeys {
		key, what := k[0], k[1]
		// A key the deck would refuse still has a row where there is room:
		// it refuses in words ("nothing archived yet", "the only session
		// stays"), and those words name a thing the help is the only
		// place to look up. It is the first row cut when there is not.
		if !board {
			switch key {
			case "tab":
				// One row for both ways through the levels: below the
				// board's width the rows are scarce, and the row the
				// merge frees goes to a key the deck is offering.
				key, what = "tab/⇧tab", "zoom in / out: trail → legs → reader (esc out too)"
			case "⇧ tab":
				what = ""
			case "esc":
				what = "one level out · from a list, a standing search clears first"
			case "j / k":
				// Paired rows below the board's width: eighteen keys do
				// not fit fourteen rows, and a key with no row is a key
				// the person cannot learn.
				what = "move down / up (↓ ↑ too) · ctrl+d/u half a page · G newest"
			case "ctrl+d/u":
				what = ""
			case "G":
				what = "" // the newest row: named on the j / k row below
			case "x":
				key, what = "x / A", "hide a session · A browses the archive, x there brings it back"
			case "A":
				what = ""
			case "?":
				key, what = "? / q", "this help · quit"
			case "q":
				what = ""
			case "m":
				// The deck answers `m` here with a refusal, and the word
				// "mirror" is nowhere else on a narrow screen.
				what = "the live pane beside the trail — needs 110 columns"
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
		"fleet:  ● working  ▲\u00a0needs\u00a0you  ◍ stuck  ↻ circling  ⊘\u00a0dead\u00a0on\u00a0the\u00a0API  ○ idle",
		"        ⌁ dev:1.0 — its tmux pane · unread — finished today, not yet opened",
		"trail:  ◉ prompt  ◆ leg  ● now, \"for 2h\"  ◈ subagent",
		"        ◈ ⋯ out · ✓ back, finding beneath · ⌀ back, empty",
		"        ◌ planned — Claude's own next moves · →3\u00a0a\u00a0live\u00a0session on this lane",
		"        ◉ 3/12 — the 3rd of 12 prompts · [ ] steps them",
		"        ⟲ context compacted — a summary below · 16⚑\u00a010✗\u00a02⟲\u00a0ships\u00a0·\u00a0red\u00a0·\u00a0compactions",
		"        · 2nd\u00a0failure — the same test in two legs · ?\u00a0—\u00a0no\u00a0verdict\u00a0parsed · edited\u00a0since — touched after that run",
		"        on\u00a0you\u00a040m\u00a0today — its waits for your next prompt (3h+\u00a0=\u00a0away)",
		"        ↪ sent — a line compass typed · ↪ answered 2 — the menu's digit · ↩ result of X — landed late; it is X's",
		"        │\u00a0you\u00a0were\u00a0here — the read-line · ↳\u00a0what\u00a0came\u00a0after · ⚠\u00a0two\u00a0sessions, one thing",
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
		lines = append(lines, dimStyle.Render(shedClauses(strings.ReplaceAll(l, "\u00a0", " "), w)))
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
			lines = append(lines, dimStyle.Render(strings.ReplaceAll(l, "\u00a0", " ")))
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
			lines = append(lines, dimStyle.Render(shedClauses(strings.ReplaceAll(l, "\u00a0", " "), w))) // no rows left: clauses go whole
			continue
		} else {
			budget -= extra
		}
		for _, row := range rows {
			lines = append(lines, dimStyle.Render(strings.ReplaceAll(row, "\u00a0", " "))) // the binding is the wrap's, not the reader's
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

// helpLegendFill puts back the legend's lines the core dropped, in the
// legend's own order, while the rows allow.
func helpLegendFill(core, full []string, rows int) []string {
	kept := map[string]bool{}
	for _, l := range core {
		kept[l] = true
	}
	out := append([]string(nil), core...)
	// The marks a person opens the help for come back first — the rail's
	// rules and counts, the loop, the trace and the read-line — and the
	// sentences about the panel last.
	var order []string
	for _, want := range []string{"you\u00a0were\u00a0here", "⌁ dev", "⟲ context compacted", "↪ sent", "2nd\u00a0failure", "on\u00a0you", "board:", "▌", "compass observes"} {
		for _, l := range full {
			if strings.Contains(l, want) && !kept[l] {
				order = append(order, l)
			}
		}
	}
	order = append(order, full...)
	for _, l := range order {
		if kept[l] || l == "" || len(out) >= rows {
			continue
		}
		// Insert before the first core line that follows it in the full order.
		pos := len(out)
		after := false
		for _, f := range full {
			if f == l {
				after = true
				continue
			}
			if after && kept[f] {
				for i, o := range out {
					if o == f {
						pos = i
						break
					}
				}
				break
			}
		}
		out = append(out[:pos], append([]string{l}, out[pos:]...)...)
		kept[l] = true
	}
	return out
}
