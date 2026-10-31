package ui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/deephanson94/compass/internal/tmuxop"
)

// The jump picker is `t` when the config names no jump command: every pane
// on the tmux server in one list — a shell, an editor, a watcher's review —
// typed down to the one you want, with the state of the session in it where
// a card has one. It is the jump a person binds to their prefix, drawn by
// the deck itself, so it can say which of the windows is waiting on them,
// enter goes straight to the pane, and esc leaves them on the board rather
// than in whatever session tmux attached first.

// jumpPicker is the picker's state while it is up.
type jumpPicker struct {
	query  string
	cursor int // into the matching entries
}

// jumpEntry is one pane the picker offers.
type jumpEntry struct {
	pane tmuxop.Pane
	key  string // the card's session, "" for a pane with no card
}

// openPicker is `t` with no jump command: the picker over the deck, on the
// panes the last listing found.
func (m *Model) openPicker() {
	m.picking = &jumpPicker{}
}

// jumpEntries is every pane in tmux's own order, each with the card it
// holds, if any.
func (m *Model) jumpEntries() []jumpEntry {
	byID := map[string]string{}
	for key, p := range m.panes {
		byID[p.ID] = key
	}
	out := make([]jumpEntry, 0, len(m.paneList))
	for _, p := range m.paneList {
		out = append(out, jumpEntry{pane: p, key: byID[p.ID]})
	}
	return out
}

// jumpMatches is the entries the query leaves: each of its words found,
// in any case, in the pane's address, its window's name or its card's name.
func (m *Model) jumpMatches() []jumpEntry {
	words := strings.Fields(strings.ToLower(m.picking.query))
	var out []jumpEntry
	for _, e := range m.jumpEntries() {
		hay := strings.ToLower(e.pane.Target + " " + e.pane.Window + " " + m.jumpCardName(e))
		ok := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, e)
		}
	}
	return out
}

// jumpCardName is the card's name for an entry with one, "" otherwise.
func (m *Model) jumpCardName(e jumpEntry) string {
	if s, ok := m.session(e.key); ok && e.key != "" {
		return sessionName(s.Info)
	}
	return ""
}

// pickerKey handles a key while the picker is up: what is typed filters,
// the arrows move, enter goes there, esc puts the picker away.
func (m *Model) pickerKey(msg tea.KeyMsg) tea.Cmd {
	p := m.picking
	matches := m.jumpMatches()
	switch key := msg.String(); key {
	case "ctrl+c":
		return tea.Quit
	case "esc":
		m.picking = nil
	case "enter":
		if len(matches) == 0 {
			return nil
		}
		e := matches[min(p.cursor, len(matches)-1)]
		m.picking = nil
		if e.key != "" {
			m.markSeen(e.key)
		}
		return m.attachPane(e.pane)
	case "up", "ctrl+p", "shift+tab":
		p.cursor = max(0, p.cursor-1)
	case "down", "ctrl+n", "tab":
		p.cursor = min(max(0, len(matches)-1), p.cursor+1)
	case "backspace":
		if r := []rune(p.query); len(r) > 0 {
			p.query = string(r[:len(r)-1])
			p.cursor = 0
		}
	case "ctrl+u":
		p.query, p.cursor = "", 0
	default:
		if msg.Type == tea.KeyRunes || key == " " {
			p.query += string(msg.Runes)
			p.cursor = 0
		}
	}
	return nil
}

// pickerPanel is the picker as a boxed list, at most rows tall: the query,
// then a row per matching pane — its card's glyph or a dash, its address,
// its window's name, and its card's state on the right.
func (m *Model) pickerPanel(inner, rows int) []string {
	p := m.picking
	body := min(replyPanelMax, max(30, inner-8))
	matches := m.jumpMatches()
	p.cursor = min(p.cursor, max(0, len(matches)-1))

	addrW := 0
	for _, e := range matches {
		addrW = max(addrW, lipgloss.Width(e.pane.Target))
	}
	lines := []readerLine{{text: "jump> " + p.query + "▏", kind: readerSaid}, {kind: readerBlank}}
	room := max(1, rows-2-len(lines)-2) // the box's edges, the query, the key line
	first := 0
	if p.cursor >= room {
		first = p.cursor - room + 1
	}
	if len(matches) == 0 {
		lines = append(lines, readerLine{text: "no pane matches", kind: readerBody})
	}
	for i := first; i < len(matches) && i < first+room; i++ {
		e := matches[i]
		glyph, state := "—", "no card"
		if s, ok := m.session(e.key); ok && e.key != "" {
			glyph = m.rowGlyph(s)
			word, sub, _ := m.cardState(s, m.trails[e.key])
			state = word
			if sub != "" {
				state += " · " + sub
			}
		}
		name := e.pane.Window
		if card := m.jumpCardName(e); card != "" && card != name {
			if name == "" {
				name = card
			} else {
				name += " · " + card // the window's name, then the card's
			}
		}
		mark, kind := "  ", readerText
		if i == p.cursor {
			mark, kind = "▸ ", readerSaid
		}
		left := mark + glyph + " " + pad(e.pane.Target, addrW) + "  "
		nameW := max(1, body-lipgloss.Width(left)-lipgloss.Width(state)-1)
		lines = append(lines, readerLine{text: left + pad(clip(name, nameW), nameW) + " " + state, kind: kind})
	}
	lines = append(lines, readerLine{kind: readerBlank},
		readerLine{text: "type to filter · ↑/↓ move · enter goes there · esc closes", kind: readerBody})

	title := " jump "
	box := func(s string, style lipgloss.Style) string {
		return " " + ruleStyle.Render("│") + " " + style.Render(pad(clip(s, body), body)) + " " + ruleStyle.Render("│") + " "
	}
	out := []string{" " + ruleStyle.Render("┌"+title+strings.Repeat("─", body+2-lipgloss.Width(title))+"┐") + " "}
	for _, l := range lines {
		out = append(out, box(l.text, readerStyle(l.kind)))
	}
	return append(out, " "+ruleStyle.Render("└"+strings.Repeat("─", body+2)+"┘")+" ")
}
