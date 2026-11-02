package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// The help's second page: the first page's footer names it, → turns to it,
// ← turns back, and every key it lists is drawn whole at the smallest
// terminal the help is held to, 80x24.
func TestTheHelpHasASecondPageForTheNewerKeys(t *testing.T) {
	m := sceneModel(sceneCards(), 80, 24)
	press(m, "?")
	if foot := lastLine(ansi.Strip(m.View())); !strings.Contains(foot, "→ more keys") {
		t.Errorf("the first page's footer does not name the second: %q", foot)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	frame := ansi.Strip(m.View())
	for _, want := range []string{"more keys", "  t ", "  R ", "  z ", "10 – 99", "claude --resume", "until its next prompt", "compass config", "← the keys"} {
		if !strings.Contains(frame, want) {
			t.Errorf("page two has no %q:\n%s", want, frame)
		}
	}
	body := strings.Join(strings.Split(frame, "\n")[2:], "\n") // under the title bar, which clips names of its own
	if strings.Contains(body, "…") {
		t.Errorf("page two cut a sentence at 80x24:\n%s", frame)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if !strings.Contains(ansi.Strip(m.View()), "→ more keys") {
		t.Error("← did not turn back to the first page")
	}
	press(m, "l")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.showHelp {
		t.Fatal("esc did not close the help")
	}
	press(m, "?")
	if m.helpPage != 0 {
		t.Error("the help reopened on its second page")
	}
}
