package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// ctrl+o on the dir row browses from the typed path: typing narrows the
// subfolders, → goes in, ← goes up with the cursor on the folder just left,
// enter takes the folder under the cursor, esc leaves the row as it was.
func TestDirBrowserWalksAndPicks(t *testing.T) {
	m, _, _, _ := launchModel(t)
	root := t.TempDir()
	for _, d := range []string{"alpha", "tinker-gemma/src", "tinker-stages", ".hidden"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	press(m, "n")
	m.launching.field = launchDir
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	typeText(m, filepath.Join(root, "not-made-yet")) // the browser opens on the nearest folder that is there
	if panel := ansi.Strip(strings.Join(m.launchPanel(120), "\n")); !strings.Contains(panel, "ctrl+o browse") {
		t.Errorf("the dir row does not offer the browser:\n%s", panel)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	b := m.launching.browse
	if b == nil || b.dir != root {
		t.Fatalf("ctrl+o: browser %+v, want it on %s", b, root)
	}
	if got := strings.Join(b.entries, ","); got != "alpha,tinker-gemma,tinker-stages" {
		t.Errorf("listed %q: folders only, no hidden ones", got)
	}

	typeText(m, "gem")
	if got := strings.Join(b.matches(), ","); got != "tinker-gemma" || b.target() != filepath.Join(root, "tinker-gemma") {
		t.Fatalf("typing gem: matches %q, target %s", got, b.target())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if b.dir != filepath.Join(root, "tinker-gemma") || b.query != "" {
		t.Fatalf("→ did not go in: %s, query %q", b.dir, b.query)
	}
	panel := ansi.Strip(strings.Join(m.launchPanel(120), "\n"))
	if !strings.Contains(panel, "src/") || !strings.Contains(panel, ". this folder") {
		t.Errorf("the panel does not list the folder:\n%s", panel)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyBackspace}) // nothing typed: up
	if b.dir != root || b.target() != filepath.Join(root, "tinker-gemma") {
		t.Fatalf("backspace with nothing typed: in %s on %s, want %s on tinker-gemma", b.dir, b.target(), root)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.launching.browse != nil || m.launching.dirText != filepath.Join(root, "tinker-gemma") {
		t.Fatalf("enter: browser %v, dir %q", m.launching.browse != nil, m.launching.dirText)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlO})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.launching == nil || m.launching.browse != nil || m.launching.dirText != filepath.Join(root, "tinker-gemma") {
		t.Fatalf("esc should close the browser only, the row as it was")
	}
}
