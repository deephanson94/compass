package ui

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/deephanson94/compass/internal/fleet"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

// Layout constants. The deck spends its width on content, never on chrome: one
// column of breathing room on each edge, a fixed fleet, a fixed trail, and
// every column the terminal can still spare for the live mirror.
const (
	fleetWidth = 30 // the floor; both side panels grow with the terminal
	trailWidth = 38

	// The side panels are what compass is for, and a very wide terminal used to
	// spend every extra column on the mirror — a pane rendering of something
	// you can already look at. Past mirrorEnough the surplus is split between
	// them instead, up to these caps: a fleet row wants its name, its state and
	// its work legible, and a trail row wants its class, its label and its age.
	fleetWidthMax   = 56
	trailWidthMax   = 52
	sessionTrailMax = 96 // the trail's share of a session view, past which a row is padding
	mirrorEnough    = 72 // the mirror keeps at least this before anyone else grows
	gutterWidth     = 3
	edgePad         = 1
	minDeckCols     = 62  // below this the second column is dropped, fleet only
	deckWideCols    = 110 // at or above this the mirror opens in the middle
	readerRoomCols  = 150 // below this the reader takes the fleet's width at Lv3

	// readerMinCols is the design floor for the Lv3 reader: a conversation
	// narrower than this is unreadable, so when the fixed columns would push it
	// below the floor the trail steps aside and the reader takes the room
	// (M3 contract: fleet | reader flex (min 46) | trail 38).
	readerMinCols = 46

	// mirrorMinCols is the design floor for the mirror: a pane narrower than
	// this shows nothing a person can read. Fleet + trail + two gutters cost 74
	// columns, so the floor is met from 116 columns up; between deckWideCols and
	// there the mirror takes what is left rather than closing (SPEC §2.5 puts
	// the fold at ~110).
	mirrorMinCols = 40
)

// Palette. Amber and red are reserved, exclusively, for "needs you" and
// "stuck": if the fleet is healthy the panel holds no warm colour at all.
// NO_COLOR and monochrome terminals are handled by lipgloss/termenv — the
// layout carries every meaning on glyph and position alone.
// Body text deliberately carries no colour: it inherits the terminal's own
// foreground, which is the one colour the user already chose. Only the quiet
// greys and the three state accents are ours.
var (
	// Grey used to come from the terminal's own palette (colour 8, "bright
	// black") rather than a hex value, so that it would stay grey on
	// 16-colour terminals where a hex approximation drifts into blue. It does
	// stay grey — at whatever contrast the theme happened to pick for index
	// 8, which for several popular ones is no contrast at all. Measured
	// against each theme's own background: Solarized Dark 1.15:1, Nord
	// 1.69:1, VS Code Dark+ 2.90:1, Dracula 3.03:1. Only a plain xterm
	// palette (5.24:1) is readable. Every dim thing on the deck went with it
	// — the work line, the leg counts, the ages, the rails.
	//
	// A *neutral* grey (r=g=b) fixes it without the drift the palette index
	// was guarding against. termenv degrades #9a9a9a to colour 8 on a
	// 16-colour terminal, which is exactly the old behaviour where the old
	// reasoning applied, and pins a known contrast on the 256-colour and
	// truecolour terminals where it did not: ≥4.4:1 on the darkest popular
	// dark theme, ≥6.7:1 on the common ones. A blue-tinted grey does drift —
	// #6b7280 degrades to bright blue (94) — so these stay on the axis.
	colDim = lipgloss.AdaptiveColor{Light: "#6e6e6e", Dark: "#9a9a9a"}

	// The rails are chrome, so they sit one step quieter than dim text — but
	// by colour, not by Faint. Faint on top of an already-dim foreground is
	// what made the hairlines vanish outright: terminals that implement SGR 2
	// as an alpha blend toward the background (kitty, foot, alacritty,
	// WezTerm, iTerm2) were blending a near-background grey into the
	// background. One explicit colour keeps the hierarchy and stays on
	// screen, at roughly the 3:1 WCAG asks of a non-text UI element.
	colRule = lipgloss.AdaptiveColor{Light: "#8a8a8a", Dark: "#808080"}

	colWorking  = lipgloss.AdaptiveColor{Light: "#15803d", Dark: "#4ade80"}
	colNeedsYou = lipgloss.AdaptiveColor{Light: "#b45309", Dark: "#fbbf24"}
	colStuck    = lipgloss.AdaptiveColor{Light: "#b91c1c", Dark: "#f87171"}
)

// SetDim overrides the grey, for a terminal or a pair of eyes the default
// still does not suit: `dim = "#c0c0c0"` in config.toml, or COMPASS_DIM in the
// environment. A `#rrggbb`, a `#rgb` or a palette index 0–255 is honoured and
// anything else is ignored — a typo leaves the deck readable rather than
// blanking it. The rails follow the override rather than staying one step
// behind it: a dim you asked for is a dim you get, rails included.
func SetDim(spec string) {
	if !validColor(spec) {
		return
	}
	colDim = lipgloss.AdaptiveColor{Light: spec, Dark: spec}
	colRule = colDim
	dimStyle = lipgloss.NewStyle().Foreground(colDim)
	ruleStyle = lipgloss.NewStyle().Foreground(colRule)
}

// validColor reports whether spec is a hex colour or an ANSI palette index,
// the two forms lipgloss reads from a string.
func validColor(spec string) bool {
	if h, ok := strings.CutPrefix(spec, "#"); ok {
		if len(h) != 3 && len(h) != 6 {
			return false
		}
		for _, r := range h {
			if !isDigit(r) && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
				return false
			}
		}
		return true
	}
	if spec == "" || len(spec) > 3 {
		return false
	}
	n := 0
	for _, r := range spec {
		if !isDigit(r) {
			return false
		}
		n = n*10 + int(r-'0')
	}
	return n <= 255
}

// One muted hue per leg class (SPEC §4). They are cool by construction: the
// warm end of the wheel belongs to needs-you and stuck alone, so a trail full
// of work still leaves the panel calm. The colour only tints the glyph and the
// verb — position and silhouette carry the meaning without it.
var classColors = map[journey.Class]lipgloss.AdaptiveColor{
	journey.Scout:  {Light: "#0e7490", Dark: "#22d3ee"}, // cyan — looking around
	journey.Design: {Light: "#6d28d9", Dark: "#a78bfa"}, // violet — thinking
	journey.Build:  {Light: "#1d4ed8", Dark: "#60a5fa"}, // blue — making
	journey.Fix:    {Light: "#a21caf", Dark: "#e879f9"}, // fuchsia — repairing
	journey.Test:   {Light: "#0f766e", Dark: "#2dd4bf"}, // teal — checking
	journey.Ship:   {Light: "#4d7c0f", Dark: "#a3e635"}, // lime — landing it
	journey.Docs:   {Light: "#475569", Dark: "#94a3b8"}, // slate — writing it down
}

// classStyle is the tint for one leg. An unknown class stays uncoloured rather
// than borrowing somebody else's meaning.
func classStyle(c journey.Class) lipgloss.Style {
	col, ok := classColors[c]
	if !ok {
		return textStyle
	}
	return lipgloss.NewStyle().Foreground(col)
}

var (
	textStyle  = lipgloss.NewStyle()
	dimStyle   = lipgloss.NewStyle().Foreground(colDim)
	ruleStyle  = lipgloss.NewStyle().Foreground(colRule)
	titleStyle = lipgloss.NewStyle().Bold(true)

	// The human's own turns lead the reader's document: bold, never coloured —
	// a prompt is the one thing on screen the user wrote.
	promptStyle = lipgloss.NewStyle().Bold(true)

	// Inversion is the panel's only selection mark: it carries the cursor and
	// the search hit without spending a colour, and it survives NO_COLOR.
	matchStyle  = lipgloss.NewStyle().Reverse(true)
	cursorStyle = lipgloss.NewStyle().Reverse(true)

	workingStyle  = lipgloss.NewStyle().Foreground(colWorking)
	needsYouStyle = lipgloss.NewStyle().Foreground(colNeedsYou)
	stuckStyle    = lipgloss.NewStyle().Foreground(colStuck)
)

// stateStyle is the one accent a row is allowed to carry.
func stateStyle(s state.State) lipgloss.Style {
	switch s {
	case state.NeedsYou:
		return needsYouStyle
	case state.Stuck:
		return stuckStyle
	case state.Working:
		return workingStyle
	default:
		return dimStyle
	}
}

// rule draws a hairline of the given width.
func rule(w int) string {
	if w <= 0 {
		return ""
	}
	return ruleStyle.Render(strings.Repeat("─", w))
}

// pad right-pads a (possibly styled) string to w display columns.
func pad(s string, w int) string {
	d := w - lipgloss.Width(s)
	if d <= 0 {
		return s
	}
	return s + strings.Repeat(" ", d)
}

// padLeft right-aligns a string within w display columns.
func padLeft(s string, w int) string {
	d := w - lipgloss.Width(s)
	if d <= 0 {
		return s
	}
	return strings.Repeat(" ", d) + s
}

// clip truncates plain text to w display runes, marking the cut with "…".
// Content is truncated, never wrapped (SPEC §4).
func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	// Cells, not runes: one CJK ideograph or emoji occupies two columns, and a
	// panel measured in runes overflows into its neighbour. Session titles,
	// branches and tmux window names are all arbitrary user text.
	var b strings.Builder
	used := 0
	runes := []rune(s)
	kept := 0
	for _, r := range runes {
		cw := lipgloss.Width(string(r))
		if used+cw > w-1 {
			break
		}
		b.WriteRune(r)
		used += cw
		kept++
	}
	if kept < len(runes) && kept > 0 && isDigit(runes[kept-1]) && isDigit(runes[kept]) {
		// Never cut a number in half: "API Error: 4…" read as a one-digit
		// status, and 403 against 429 is the difference the row is for.
		// The whole number goes instead (#53) — and only then: a number
		// whole on screen stays, however the row ends after it (#59).
		for kept > 0 && isDigit(runes[kept-1]) {
			kept--
		}
	}
	// "go test ./...…", "go test ./…": a mark after a dot or a slash reads
	// as more of the token it cut, whichever rune the cut fell on. The
	// trailing run of dots and slashes goes — "backfill." is "backfill…",
	// "go test ./..." is "go test…" — never the token, which cost a
	// wider column the name of the hung run (#58, #59, #61). The
	// separator, a bracket and the spaces they stood on go with it.
	// A dash that stands alone is a separator too ("porter_tui —…"
	// promised a phrase), where a hyphen inside a token is the token's
	// (`--all`, `-run`): the spaced dashes go, the hyphen stays (#63).
	// A comma or a semicolon promises the clause after it the same way (#64).
	// And a flag's leading dash with nothing after it: "--model x -…"
	// promised a flag; the hyphen inside a token stays (#72).
	head := strings.TrimRight(string(runes[:kept]), " ·(./—–,;")
	for strings.HasSuffix(head, " -") || strings.HasSuffix(head, " --") {
		head = strings.TrimRight(strings.TrimRight(head, "-"), " ·(./—–,;")
	}
	// An opening quote with nothing after it promises the words it was
	// about to hold: `checkout-flake-hunt · "…` spent four cells on no
	// prompt at all. The quote goes with the separator that led it, and
	// only while it is unclosed (#80).
	for strings.HasSuffix(head, `"`) && strings.Count(head, `"`)%2 == 1 {
		head = strings.TrimRight(strings.TrimSuffix(head, `"`), " ·(./—–,;")
	}
	return head + "…"
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// truncateWhole is ansi.Truncate that never cuts a number in half and
// never leaves a bare separator before the mark (#54).
func truncateWhole(line string, n int) string {
	full := []rune(ansi.Strip(line))
	for n > 0 {
		t := ansi.Truncate(line, n, "")
		kept := []rune(ansi.Strip(t))
		k := len(kept)
		if k < len(full) && k > 0 && isDigit(kept[k-1]) && isDigit(full[k]) {
			n--
			continue
		}
		if k > 0 && (kept[k-1] == '·' || kept[k-1] == ' ') && k < len(full) {
			n--
			continue
		}
		// The cut at the box's left edge is a clip like any other: a mark
		// after a dot, a slash, a bracket or a comma reads as more of the
		// token it cut, and an unclosed quote promises the words it was
		// about to hold — the very cutset `clip` has carried since #58,
		// #61, #63, #64 and #80. Without it the border drew
		// `● scout  Red-teaming plugin/…` and `◉ "…`.
		if k > 0 && k < len(full) && strings.ContainsRune("(./—–,;", kept[k-1]) {
			n--
			continue
		}
		if k > 0 && k < len(full) && kept[k-1] == '"' && strings.Count(string(kept), `"`)%2 == 1 {
			n--
			continue
		}
		return t
	}
	return ""
}

// askQuote is an ask the way a row quotes it: `"the prompt"`, or `relayed
// "the prompt"` when the ask was another session's message — a word, not a
// glyph, and outside the quotes so the quote is still the ask (#97).
func askQuote(text string, relayed bool) string {
	if relayed {
		return relayVerb + `"` + text + `"`
	}
	return `"` + text + `"`
}

// relayVerb is the word a relayed ask wears before its quotes (#97): the
// verb is the row's, the sentence inside the quotes is the session's.
const relayVerb = "relayed "

// askRelayed says whether an archived headline is the session's relayed
// title, rather than a state word the headline fell back to.
func askRelayed(s fleet.Session) bool {
	return s.Info.Relayed && s.Info.Title != "" && archiveHeadline(s) == s.Info.Title
}
