package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/prstate"
)

// WorkOf is where a session works, for the pull-request tracker: its cwd
// first, then the directories its own tool calls worked in, and the pull
// requests it was pointed at by address.
func WorkOf(s fleet.Session) prstate.Work {
	var w prstate.Work
	if s.Info.CWD != "" {
		w.Dirs = append(w.Dirs, s.Info.CWD)
	}
	for _, d := range s.Dirs {
		if d != s.Info.CWD {
			w.Dirs = append(w.Dirs, d)
		}
	}
	w.PRs = append(w.PRs, s.PRs...)
	return w
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

// prChips are the header's pull-request counts, as `compass status` says
// them: "PR▲1" for your own waiting on you, "RV▲2" for the ones you review.
// Amber while one waits on you, dim while all wait on someone else; absent
// when there are none.
func (m *Model) prChips() []string {
	live := map[string][]prstate.Status{}
	for _, s := range m.sessions {
		if sts := m.prs[s.Info.Key()]; s.Live && len(sts) > 0 {
			live[s.Info.Key()] = sts // the archive's own view counts them too
		}
	}
	var out []string
	for _, seg := range []string{prstate.Segment(live), prstate.ReviewSegment(m.reviews)} {
		switch {
		case seg == "":
		case strings.HasSuffix(seg, "▲0"):
			out = append(out, dimStyle.Render(seg))
		default:
			out = append(out, needsYouStyle.Render(seg))
		}
	}
	return out
}
