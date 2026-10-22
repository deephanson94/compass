package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/prstate"
)

// WorkDirs is where a session works, for the pull-request tracker: its cwd
// first, then the directories its own tool calls worked in.
func WorkDirs(s fleet.Session) []string {
	var out []string
	if s.Info.CWD != "" {
		out = append(out, s.Info.CWD)
	}
	for _, d := range s.Dirs {
		if d != s.Info.CWD {
			out = append(out, d)
		}
	}
	return out
}

// prOf is the session's pull requests, most urgent first, where its row
// says them: a live session on the live board. The archive says nothing
// about pull requests.
func (m *Model) prOf(s fleet.Session) ([]prstate.Status, bool) {
	if m.archiveView || !s.Live {
		return nil, false
	}
	sts := m.prs[s.Info.Key()]
	return sts, len(sts) > 0
}

// prYours says a reviewer answered one of the session's pull requests, or
// approved it.
func (m *Model) prYours(s fleet.Session) bool {
	sts, ok := m.prOf(s)
	return ok && sts[0].Move == prstate.Yours
}

// prLine is the pull requests in words, w wide: amber behind the needs-you
// glyph when the most urgent is your move, dim like an idle row when every
// one is theirs. The words say it either way — "changes requested",
// "waiting on reviewer1" — so it reads in monochrome too.
func (m *Model) prLine(sts []prstate.Status, w int) string {
	yours := sts[0].Move == prstate.Yours
	lead := ""
	if yours {
		lead = fleet.GlyphNeedsYou + " "
	}
	// The longest form that fits; the shortest, clipped, where none does.
	var line string
	for _, form := range prstate.Summaries(sts, m.now) {
		line = lead + form
		if lipgloss.Width(line) <= w {
			break
		}
	}
	if yours {
		return needsYouStyle.Render(clip(line, w))
	}
	return dimStyle.Render(clip(line, w))
}
