package ui

import "github.com/charmbracelet/lipgloss"

// panel names a column the keys can be pointed at.
type panel int

const (
	panelFleet panel = iota
	panelReader
	panelTrail
)

// focusMark is drawn beside the title of the panel the keys act on: a bar down
// the edge of the column, the way an editor marks its active pane.
//
// Deliberately not "▸", which the fleet already spends on the selected row —
// at Lv1 both would sit in the same column meaning different things, one
// naming a session and one naming a panel. A glyph rather than a tint, because
// which panel has focus has to survive monochrome as much as anything else
// does (SPEC §4).
const focusMark = "▌"

// focus is the column the keys act on: the fleet at Lv1, the trail at Lv2 —
// where j/k walk its rows — and the reader at Lv3, where the same keys scroll
// the conversation instead.
//
// Lv2 and Lv3 draw an identical trail, because Lv3 is not a different view of
// the journey; it is the same view with the focus moved off it. Until this
// marker the only signs that the keys had changed hands were the [Lv2]/[Lv3]
// tag and the footer, which is what made zooming in feel like nothing had
// happened.
func (m *Model) focus() panel {
	switch {
	case m.level >= levelReader:
		return panelReader
	case m.level >= levelWaypoints:
		return panelTrail
	default:
		return panelFleet
	}
}

// titleMark is the one-column gutter every panel title starts with: the marker
// when the keys are there, a space when they are not, so the titles stay in
// line with each other either way.
func (m *Model) titleMark(p panel) string {
	if m.focus() == p {
		return textStyle.Render(focusMark)
	}
	return " "
}

// titleStyleFor keeps an unfocused title quiet. The marker is what carries the
// meaning; this only reinforces it.
func (m *Model) titleStyleFor(p panel) lipgloss.Style {
	if m.focus() == p {
		return textStyle
	}
	return dimStyle
}
