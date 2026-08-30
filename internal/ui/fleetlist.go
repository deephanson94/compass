package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/state"
)

// A fleet entry is two lines — the verdict and, quieter beneath it, where the
// session lives — plus one blank line of air.
const (
	entryLines  = 2
	entryStride = entryLines + 1
	nameWidth   = 9
	ageWidth    = 4
)

// fleetColumn is the deck's left panel: the title, one line of air, and the
// fleet itself.
func (m *Model) fleetColumn(w, h int) []string {
	rows := []string{dimStyle.Render(clip("FLEET", w)), ""}
	if h > 2 {
		rows = append(rows, m.fleetLines(w, h-2)...)
	}
	return rows
}

// fleetLines renders the fleet: one two-line entry per session, the attention
// cases already sorted to the top by the Manager.
func (m *Model) fleetLines(w, h int) []string {
	if len(m.sessions) == 0 {
		return []string{dimStyle.Render(clip("no sessions", w))}
	}

	capacity := (h + 1) / entryStride
	if capacity < 1 {
		capacity = 1
	}
	sel := m.selectedIndex()
	start := 0
	if len(m.sessions) > capacity {
		start = sel - capacity/2
		if start < 0 {
			start = 0
		}
		if start > len(m.sessions)-capacity {
			start = len(m.sessions) - capacity
		}
	}
	end := start + capacity
	if end > len(m.sessions) {
		end = len(m.sessions)
	}
	hidden := len(m.sessions) - end
	if hidden > 0 && end > start+1 {
		// Give the last row back so the overflow note has somewhere to sit.
		end--
		hidden = len(m.sessions) - end
	}

	lines := make([]string, 0, h)
	for i := start; i < end; i++ {
		if i > start {
			lines = append(lines, "")
		}
		lines = append(lines, m.entryLines(i, w)...)
	}
	if hidden > 0 {
		lines = append(lines, "", dimStyle.Render(clip(fmt.Sprintf("     +%d more", hidden), w)))
	}
	return lines
}

// entryLines renders one session: "N ● name  activity  age" over a dim line
// naming the project and branch.
func (m *Model) entryLines(i, w int) []string {
	s := m.sessions[i]
	selected := s.Info.ID == m.selectedID
	accent := stateStyle(s.Snap.State)

	marker := " "
	if selected {
		marker = "▸"
	}
	index := " "
	if i < 9 {
		index = fmt.Sprintf("%d", i+1)
	}
	indexStyled := dimStyle.Render(index)
	nameStyle := dimStyle
	if selected {
		indexStyled = textStyle.Render(index)
		nameStyle = textStyle
	}

	midWidth := w - 5 - nameWidth - 1 - 1 - ageWidth
	if midWidth < 6 {
		midWidth = 6
	}

	name := pad(clip(sessionName(s.Info), nameWidth), nameWidth)
	mid := pad(clip(headline(s), midWidth), midWidth)
	age := padLeft(m.age(s.Snap.Since), ageWidth)

	first := marker + indexStyled + " " + accent.Render(fleet.Glyph(s.Snap.State)) + " " +
		nameStyle.Render(name) + " " + dimStyle.Render(mid) + " " + dimStyle.Render(age)

	if selected {
		first = marker + indexStyled + " " + accent.Render(fleet.Glyph(s.Snap.State)) + " " +
			nameStyle.Render(name) + " " + textStyle.Render(mid) + " " + dimStyle.Render(age)
	}

	second := dimStyle.Render(clip(m.location(s.Info), w-4))
	return []string{first, strings.Repeat(" ", 4) + second}
}

// headline is the one thing worth saying about a session on its own line. The
// list answers "who wants me"; the card next to it answers "why".
func headline(s fleet.Session) string {
	switch s.Snap.State {
	case state.NeedsYou:
		return "needs you"
	case state.Working, state.Stuck:
		if s.Snap.Activity != "" {
			return s.Snap.Activity
		}
	}
	if s.Snap.Reason != "" {
		return s.Snap.Reason
	}
	return stateLabel(s.Snap.State)
}

// stateLabel is the human spelling of a state; State.String() stays the
// machine-readable one.
func stateLabel(s state.State) string {
	if s == state.NeedsYou {
		return "needs you"
	}
	return s.String()
}

// location is the dim second line: where the session lives, then what it is
// working on. "dev:1.0" is tmux session dev, window 1, pane 0 — so you always
// know where to go (SPEC §2.5). A session with no pane says so plainly.
func (m *Model) location(info fleet.SessionInfo) string {
	where := "(no pane)"
	if pane, ok := m.panes[info.ID]; ok && pane.Target != "" {
		where = pane.Target
	}
	if branch := info.GitBranch; branch != "" {
		return where + " · " + branch
	}
	if slug := strings.TrimPrefix(info.ProjectSlug, "-"); slug != "" {
		return where + " · " + slug
	}
	return where
}

// sessionName calls a session by the last segment of its working directory —
// the way its human thinks of it.
func sessionName(info fleet.SessionInfo) string {
	if info.CWD != "" {
		if base := filepath.Base(info.CWD); base != "." && base != string(filepath.Separator) {
			return base
		}
	}
	if p := fleet.SlugPath(info.ProjectSlug); p != "" {
		if base := filepath.Base(p); base != "." && base != string(filepath.Separator) {
			return base
		}
	}
	if len(info.ID) > 8 {
		return info.ID[:8]
	}
	return info.ID
}
