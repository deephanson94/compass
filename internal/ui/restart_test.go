package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/state"
)

// paneTmux is a pane whose claude exits on /exit; it records what is typed.
type paneTmux struct {
	typed  []string
	exited bool
}

func (p *paneTmux) Output(args ...string) ([]byte, error) {
	switch args[0] {
	case "display-message":
		if p.exited {
			return []byte("bash\n"), nil
		}
		return []byte("claude\n"), nil
	case "send-keys":
		k := args[len(args)-1]
		p.typed = append(p.typed, k)
		if k == "/exit" {
			p.exited = true
		}
	}
	return nil, nil
}

func restartScene(t *testing.T) (*Model, *paneTmux) {
	t.Helper()
	withCards(t)
	m := sceneModel(sceneCards(), 290, 80)
	tm := &paneTmux{}
	m.runner, m.sleep = tm, func(time.Duration) {}
	m.point(sessionKey("s3"))
	return m, tm
}

// R asks first, naming the session and the very line it will type; enter
// exits the CLI and types the resume line once the shell is back.
func TestRAsksThenRestartsInThePane(t *testing.T) {
	m, tm := restartScene(t)
	press(m, "R")
	if m.restarting == nil {
		t.Fatal("R did not ask")
	}
	if foot := ansi.Strip(m.View()); !strings.Contains(foot, "↻ restart s3: /exit, then claude --resume s3 · enter restarts · esc keeps it") {
		t.Errorf("the footer does not name the restart:\n%s", lastLine(foot))
	}
	if len(tm.typed) != 0 {
		t.Fatalf("R typed before the enter: %q", tm.typed)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	s, _ := m.session(sessionKey("s3"))
	if word, _, _ := m.cardState(s, m.trails[s.Info.Key()]); word != cardRestarting {
		t.Errorf("under way, the card reads %q, want %q", word, cardRestarting)
	}
	m.Update(cmd())
	if got := strings.Join(tm.typed, "|"); got != "Escape|/exit|Enter|claude --resume s3|Enter" {
		t.Errorf("typed %q", got)
	}
	if m.note != "↻ restarted s3" {
		t.Errorf("note %q", m.note)
	}
	if word, _, _ := m.cardState(s, m.trails[s.Info.Key()]); word == cardRestarting {
		t.Error("the card still reads restarting once the restart is done")
	}
}

// Any key but enter keeps the session running, and types nothing; a turn
// in flight is named in the question; read-only refuses.
func TestRKeepsTheSessionUnlessEnter(t *testing.T) {
	m, tm := restartScene(t)
	press(m, "R")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.restarting != nil || len(tm.typed) != 0 || !strings.Contains(m.note, "kept s3 running") {
		t.Errorf("esc: asking %v, typed %q, note %q", m.restarting != nil, tm.typed, m.note)
	}

	m.point(sessionKey("stages")) // working
	press(m, "R")
	if !strings.Contains(m.restartKeys(), "working — restarting stops the turn") {
		t.Errorf("a working session's question: %q", m.restartKeys())
	}
	press(m, "x") // not enter
	if len(tm.typed) != 0 {
		t.Errorf("a key other than enter typed %q", tm.typed)
	}

	m.readonly = true
	press(m, "R")
	if m.restarting != nil || !strings.Contains(m.note, "read-only") {
		t.Errorf("read-only: asking %v, note %q", m.restarting != nil, m.note)
	}
}

// Dead on the API, the reply panel offers the restart, and its digit asks
// as R does rather than restarting at once.
func TestTheReplyPanelOffersARestartWhenDeadOnTheAPI(t *testing.T) {
	m, tm := restartScene(t)
	for i, s := range m.sessions {
		if sessionName(s.Info) == "s3" {
			m.sessions[i].Snap = state.Snapshot{State: state.NeedsYou, APIError: true, APIStatus: 500, Reason: "api error 500", Activity: "API Error: 500"}
		}
	}
	choices := m.replyChoices()
	at := -1
	for i, c := range choices {
		if c.kind == replyRestart {
			at = i
		}
	}
	if at < 0 {
		t.Fatalf("no restart among %d choices", len(choices))
	}
	press(m, "r")
	press(m, string(rune('1'+at)))
	if m.restarting == nil || len(tm.typed) != 0 {
		t.Errorf("the panel's restart should ask first: asking %v, typed %q", m.restarting != nil, tm.typed)
	}
}

func lastLine(s string) string {
	l := strings.Split(strings.TrimRight(s, "\n"), "\n")
	return l[len(l)-1]
}
