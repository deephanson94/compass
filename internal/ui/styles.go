package ui

import (
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
	// Grey comes from the terminal's own palette (colour 8) rather than a hex
	// value: it stays grey on 16-colour terminals, where a hex approximation
	// would drift into blue.
	colDim = lipgloss.Color("8")

	colWorking  = lipgloss.AdaptiveColor{Light: "#15803d", Dark: "#4ade80"}
	colNeedsYou = lipgloss.AdaptiveColor{Light: "#b45309", Dark: "#fbbf24"}
	colStuck    = lipgloss.AdaptiveColor{Light: "#b91c1c", Dark: "#f87171"}
)

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
	ruleStyle  = lipgloss.NewStyle().Foreground(colDim).Faint(true)
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
	for _, r := range s {
		cw := lipgloss.Width(string(r))
		if used+cw > w-1 {
			break
		}
		b.WriteRune(r)
		used += cw
	}
	return strings.TrimRight(b.String(), " ") + "…"
}
