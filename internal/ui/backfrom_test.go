package ui

import (
	"testing"
)

// backTmux answers list-panes with one line: the pane a person left from.
type backTmux struct{ pane string }

func (b backTmux) Output(args ...string) ([]byte, error) {
	if len(args) > 0 && args[0] == "list-panes" {
		return []byte("1793437465 11 " + b.pane + "\n"), nil
	}
	return nil, nil
}

// back is the deck getting its terminal back from tmux, as bubbletea
// delivers it: the attach's end, then what tmux says.
func back(m *Model) {
	_, cmd := m.Update(attachDoneMsg{target: "w:1.0"})
	if cmd != nil {
		m.Update(cmd())
	}
}

// Back from tmux, the card selected is the one the person left from,
// wherever their keys took them: here gemma's pane, %8.
func TestBackFromTmuxSelectsTheCardYouLeft(t *testing.T) {
	withCards(t)
	m := sceneModel(sceneCards(), 290, 80)
	m.point(sessionKey("harness"))
	m.runner = backTmux{pane: "%8"}
	back(m)
	if got := selectedName(m); got != "gemma" {
		t.Errorf("selected %s after coming back from gemma's pane", got)
	}
}

// A pane with no card (a shell) leaves the selection where it was; so
// does a tmux that cannot say.
func TestBackFromAPaneWithNoCardKeepsTheSelection(t *testing.T) {
	withCards(t)
	for _, pane := range []string{"%42", ""} {
		m := sceneModel(sceneCards(), 290, 80)
		m.point(sessionKey("harness"))
		m.runner = backTmux{pane: pane}
		back(m)
		if got := selectedName(m); got != "harness" {
			t.Errorf("back from %q moved the selection to %s", pane, got)
		}
	}
}
