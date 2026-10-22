package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/prstate"
)

// prOf is the session's pull request where its row says it: a live session
// on the live board. The archive says nothing about pull requests.
func (m *Model) prOf(s fleet.Session) (prstate.Status, bool) {
	if m.archiveView || !s.Live {
		return prstate.Status{}, false
	}
	st, ok := m.prs[s.Info.Key()]
	return st, ok
}

// prYours says a reviewer answered the session's pull request, or approved it.
func (m *Model) prYours(s fleet.Session) bool {
	st, ok := m.prOf(s)
	return ok && st.Move == prstate.Yours
}

// prLine is the pull request in words, w wide: amber behind the needs-you
// glyph when the move is yours, dim like an idle row when it is theirs. The
// words say it either way — "changes requested", "waiting on reviewer1" —
// so it reads in monochrome too.
func (m *Model) prLine(st prstate.Status, w int) string {
	line := st.Line(m.now)
	if st.Move == prstate.Yours {
		line = fleet.GlyphNeedsYou + " " + line
	}
	if lipgloss.Width(line) > w {
		line = strings.Replace(line, " ago", "", 1) // "changes requested 2h" before a cut word
	}
	if st.Move == prstate.Yours {
		return needsYouStyle.Render(clip(line, w))
	}
	return dimStyle.Render(clip(line, w))
}
