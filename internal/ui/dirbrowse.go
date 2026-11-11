package ui

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/deephanson94/compass/internal/launch"
)

// The folder browser is ctrl+o on the `n` panel's dir row: the person's own
// cd / ls / cd loop, inside the panel. A directory nobody had a session in
// lately is in no list ←/→ walks, and typing it meant knowing it — so the
// person left for a shell, looked around, and came back to type the path.
// The browser reads one folder at a time, the one on screen, so it reaches
// anywhere on the disk and costs nothing until it is opened.

// dirBrowser is the browser's state while it is up.
type dirBrowser struct {
	dir     string   // the folder being listed
	query   string   // typed: narrows the subfolders
	cursor  int      // into the rows: 0 is the folder itself, then the matches
	entries []string // dir's subfolders, by name, sorted
	err     string   // why dir could not be read, "" when it was
}

// browseRows is the most subfolders the browser draws at once.
const browseRows = 14

// openBrowser is ctrl+o on the dir row: the browser on the typed path, or
// on the nearest folder above it that exists, or on home.
func (m *Model) openBrowser() {
	f := m.launching
	start := ""
	if d, err := launch.ExpandHome(strings.TrimSpace(f.dirText)); err == nil && d != "" {
		for d = filepath.Clean(d); ; d = filepath.Dir(d) {
			if st, err := os.Stat(d); err == nil && st.IsDir() {
				start = d
				break
			}
			if d == filepath.Dir(d) {
				break
			}
		}
	}
	if start == "" {
		start, _ = os.UserHomeDir()
	}
	if start == "" {
		start = "/"
	}
	f.browse = &dirBrowser{}
	f.browse.enter(start)
}

// enter lists dir: its subfolders, hidden ones left out, a link to a folder
// counted as a folder.
func (b *dirBrowser) enter(dir string) {
	b.dir, b.query, b.cursor, b.entries, b.err = dir, "", 0, nil, ""
	list, err := os.ReadDir(dir)
	if err != nil {
		b.err = firstLine(err.Error())
		return
	}
	for _, e := range list {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		isDir := e.IsDir()
		if !isDir && e.Type()&os.ModeSymlink != 0 {
			if st, err := os.Stat(filepath.Join(dir, name)); err == nil {
				isDir = st.IsDir()
			}
		}
		if isDir {
			b.entries = append(b.entries, name)
		}
	}
	sort.Slice(b.entries, func(i, j int) bool {
		return strings.ToLower(b.entries[i]) < strings.ToLower(b.entries[j])
	})
}

// matches is the subfolders the query leaves: each of its words found in
// the name, in any case.
func (b *dirBrowser) matches() []string {
	words := strings.Fields(strings.ToLower(b.query))
	if len(words) == 0 {
		return b.entries
	}
	var out []string
	for _, name := range b.entries {
		low, ok := strings.ToLower(name), true
		for _, w := range words {
			if !strings.Contains(low, w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, name)
		}
	}
	return out
}

// target is the folder the cursor stands on: the listed folder itself on
// row 0, else the match under the cursor.
func (b *dirBrowser) target() string {
	ms := b.matches()
	if b.cursor == 0 || len(ms) == 0 {
		return b.dir
	}
	return filepath.Join(b.dir, ms[min(b.cursor, len(ms))-1])
}

// browseKey handles a key while the browser is up: what is typed narrows
// the list, ↑/↓ move, → goes into the folder under the cursor, ← (or
// backspace with nothing typed) goes up, enter takes the folder under the
// cursor for the dir row, esc goes back to the panel with the row as it was.
func (m *Model) browseKey(msg tea.KeyMsg) tea.Cmd {
	f := m.launching
	b := f.browse
	ms := b.matches()
	switch key := msg.String(); key {
	case "ctrl+c":
		return tea.Quit
	case "esc":
		f.browse = nil
	case "enter":
		f.dirText = homeShort(b.target())
		f.browse = nil
	case "up", "ctrl+p", "shift+tab":
		b.cursor = max(0, b.cursor-1)
	case "down", "ctrl+n", "tab":
		b.cursor = min(len(ms), b.cursor+1)
	case "right":
		if t := b.target(); t != b.dir {
			b.enter(t)
		}
	case "left":
		b.up()
	case "backspace":
		if r := []rune(b.query); len(r) > 0 {
			b.query = string(r[:len(r)-1])
			b.cursor = min(1, len(b.matches()))
		} else {
			b.up()
		}
	case "ctrl+u":
		b.query, b.cursor = "", 0
	default:
		if msg.Type == tea.KeyRunes || key == " " {
			if string(msg.Runes) == "/" && b.query == "" {
				return nil // a habit from the shell: the list is already the folder's
			}
			b.query += string(msg.Runes)
			b.cursor = min(1, len(b.matches())) // the first match, not the folder itself
		}
	}
	return nil
}

// up lists the folder above, with the cursor on the one just left.
func (b *dirBrowser) up() {
	parent := filepath.Dir(b.dir)
	if parent == b.dir {
		return
	}
	left := filepath.Base(b.dir)
	b.enter(parent)
	for i, name := range b.entries {
		if name == left {
			b.cursor = i + 1
			break
		}
	}
}

// dirBrowserPanel is the browser as a boxed list: the folder and what is
// typed, the folder itself as the first row, then its subfolders.
func (m *Model) dirBrowserPanel(inner int) []string {
	b := m.launching.browse
	body := min(replyPanelMax, max(30, inner-8))
	ms := b.matches()
	b.cursor = min(b.cursor, len(ms))

	lines := []readerLine{
		{text: keepTail(homeShort(b.dir), body-6) + "/  " + b.query + "▏", kind: readerSaid},
		{kind: readerBlank},
	}
	row := func(i int, text string) {
		mark, kind := "  ", readerText
		if i == b.cursor {
			mark, kind = "▸ ", readerSaid
		}
		lines = append(lines, readerLine{text: mark + text, kind: kind})
	}
	row(0, ". this folder")
	switch {
	case b.err != "":
		lines = append(lines, readerLine{text: "  can't read it: " + b.err, kind: readerBody})
	case len(b.entries) == 0:
		lines = append(lines, readerLine{text: "  no folders in it", kind: readerBody})
	case len(ms) == 0:
		lines = append(lines, readerLine{text: "  no folder matches", kind: readerBody})
	}
	first := 0
	if b.cursor > browseRows {
		first = b.cursor - browseRows
	}
	for i := first; i < len(ms) && i < first+browseRows; i++ {
		row(i+1, ms[i]+"/")
	}
	if rest := len(ms) - (first + browseRows); rest > 0 {
		lines = append(lines, readerLine{text: "  … " + strconv.Itoa(rest) + " more · type to narrow", kind: readerBody})
	}
	lines = append(lines, readerLine{kind: readerBlank},
		readerLine{text: "type filters · ↑/↓ move · → in · ← up · enter picks · esc back", kind: readerBody})

	title := " pick a folder "
	box := func(s string, style lipgloss.Style) string {
		return " " + ruleStyle.Render("│") + " " + style.Render(pad(clip(s, body), body)) + " " + ruleStyle.Render("│") + " "
	}
	out := []string{" " + ruleStyle.Render("┌"+title+strings.Repeat("─", body+2-lipgloss.Width(title))+"┐") + " "}
	for _, l := range lines {
		out = append(out, box(l.text, readerStyle(l.kind)))
	}
	return append(out, " "+ruleStyle.Render("└"+strings.Repeat("─", body+2)+"┘")+" ")
}
