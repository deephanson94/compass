package ui

import (
	"os/exec"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/tmuxop"
)

// pickerScene is the cards scene with a shell beside it: a pane no
// session holds, which is exactly what the picker is for.
func pickerScene(t *testing.T) (*Model, *[]string) {
	t.Helper()
	withCards(t)
	m := sceneModel(sceneCards(), 290, 80)
	m.paneList = append(m.paneList, tmuxop.Pane{Target: "sh:1.0", ID: "%50", Window: "bash", Command: "bash"})
	m.runner = &launchTmux{} // a server with sessions: the picker has somewhere to go
	var handed []string
	m.spawn = func(cmd *exec.Cmd, _ bool, done func(error) tea.Msg) tea.Cmd {
		handed = cmd.Args
		return nil
	}
	return m, &handed
}

// t with no jump command is the deck's picker: every pane, a card's with
// its glyph and state, a shell's with a dash, and enter goes there.
func TestTOpensThePickerOnEveryPane(t *testing.T) {
	m, handed := pickerScene(t)
	press(m, "t")
	if m.picking == nil {
		t.Fatal("t with no jump command did not open the picker")
	}
	frame := ansi.Strip(m.View())
	if strings.Contains(frame, "· harness") {
		t.Errorf("a pane with no window name should show the card's name alone:\n%s", frame)
	}
	for _, want := range []string{"┌ jump ", "jump> ", "▲ w:1.0", "harness", "needs you", "— sh:1.0", "bash", "no card"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the picker has no %q:\n%s", want, frame)
		}
	}

	typeText(m, "sh")
	if got := m.jumpMatches(); len(got) != 1 || got[0].pane.ID != "%50" {
		t.Fatalf("typing sh left %d panes, want the shell alone", len(got))
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.picking != nil {
		t.Error("enter left the picker up")
	}
	if got := strings.Join(*handed, " "); !strings.Contains(got, "attach-session") || !strings.Contains(got, "%50") {
		t.Errorf("enter handed %q, want an attach to the shell's pane", got)
	}
}

// The arrows walk the matches; esc closes without going anywhere; a card's
// name finds its pane too.
func TestThePickerWalksAndCloses(t *testing.T) {
	m, handed := pickerScene(t)
	press(m, "t")
	typeText(m, "gemma") // the card's name, not the window's
	if got := m.jumpMatches(); len(got) != 1 || got[0].key != sessionKey("gemma") {
		t.Fatalf("a card's name should find its pane, got %d", len(got))
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.picking.cursor != 2 {
		t.Errorf("two downs: cursor %d, want 2", m.picking.cursor)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.picking != nil || *handed != nil {
		t.Errorf("esc: picker %v, handed %q", m.picking != nil, *handed)
	}
}
