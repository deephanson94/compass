package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/deephanson94/compass/internal/state"
)

// Layout constants. The deck spends its width on content, never on chrome: one
// column of breathing room on each edge, a 34-column fleet, a wide gutter, and
// the rest for the selected session.
const (
	fleetWidth  = 34
	gutterWidth = 3
	edgePad     = 1
	minDeckCols = 62 // below this the detail card is dropped, fleet only
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

var (
	textStyle  = lipgloss.NewStyle()
	dimStyle   = lipgloss.NewStyle().Foreground(colDim)
	ruleStyle  = lipgloss.NewStyle().Foreground(colDim).Faint(true)
	titleStyle = lipgloss.NewStyle().Bold(true)

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
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return strings.TrimRight(string(r[:w-1]), " ") + "…"
}

// clipLeft truncates from the left, keeping the tail — the useful half of a
// long path.
func clipLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return "…" + string(r[len(r)-(w-1):])
}
