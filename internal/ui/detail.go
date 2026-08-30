package ui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/deephanson94/compass/internal/fleet"
)

// labelWidth aligns the card's field names into one quiet column.
const labelWidth = 11

// detailLines renders the card for the selected session: what it is, what it is
// doing, and where it lives.
func (m *Model) detailLines(w, h int) []string {
	if len(m.sessions) == 0 {
		return []string{dimStyle.Render(clip("no session selected", w))}
	}
	s := m.sessions[m.selectedIndex()]
	info := s.Info

	lines := make([]string, 0, h)

	// Heading: the session's name, its age held quietly at the right margin.
	age := m.age(s.Snap.Since)
	name := clip(sessionName(info), w-len([]rune(age))-2)
	lines = append(lines, pad(titleStyle.Render(name), w-lipgloss.Width(age))+dimStyle.Render(age))

	// The verdict — the one accent on this card.
	lines = append(lines, verdictLine(s, w))

	lines = append(lines, "")
	if info.Title != "" {
		lines = append(lines, textStyle.Render(clip("“"+info.Title+"”", w)))
		lines = append(lines, "")
	}

	lines = append(lines,
		field(w, "activity", s.Snap.Activity),
		field(w, "cwd", firstNonEmpty(info.CWD, fleet.SlugPath(info.ProjectSlug))),
		field(w, "branch", info.GitBranch),
		field(w, "session", info.ID),
		fieldTail(w, "transcript", shortenHome(info.TranscriptPath)),
	)

	if m.showTrail {
		ruleW := 28
		if w < ruleW {
			ruleW = w
		}
		lines = append(lines, "", rule(ruleW), dimStyle.Render(clip("trail arrives in M1", w)))
	}
	return lines
}

// verdictLine is the state in its accent colour, the reason dim beside it.
func verdictLine(s fleet.Session, w int) string {
	head := fleet.Glyph(s.Snap.State) + " " + stateLabel(s.Snap.State)
	line := stateStyle(s.Snap.State).Render(clip(head, w))
	rest := w - lipgloss.Width(line)
	if s.Snap.Reason != "" && rest > 4 {
		line += dimStyle.Render(clip(" · "+s.Snap.Reason, rest))
	}
	return line
}

// field renders one aligned "label   value" row; an empty value reads as "—".
func field(w int, label, value string) string {
	if strings.TrimSpace(value) == "" {
		value = "—"
	}
	return dimStyle.Render(pad(label, labelWidth)) + textStyle.Render(clip(value, w-labelWidth))
}

// fieldTail is field() for paths, where the tail carries the meaning.
func fieldTail(w int, label, value string) string {
	if strings.TrimSpace(value) == "" {
		value = "—"
	}
	return dimStyle.Render(pad(label, labelWidth)) + dimStyle.Render(clipLeft(value, w-labelWidth))
}

// shortenHome writes a path the way a person would say it.
func shortenHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || home == "/" {
		return path
	}
	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + path[len(home):]
	}
	return path
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
