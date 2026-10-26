package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/launch"
	"github.com/deephanson94/compass/internal/tmuxop"
)

// launchTmux is a tmux server with two sessions that records every call and
// opens every window at dev:5.0.
type launchTmux struct{ calls [][]string }

func (f *launchTmux) Output(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	switch args[0] {
	case "list-sessions":
		return []byte("ops\ndev\n"), nil
	case "new-window":
		return []byte("%42 dev:5.0\n"), nil
	}
	return nil, nil
}

func (f *launchTmux) newWindow() []string {
	for _, c := range f.calls {
		if c[0] == "new-window" {
			return c
		}
	}
	return nil
}

// launchModel is a deck of two sessions in two real directories, the
// selected one in a pane of the tmux session "dev".
func launchModel(t *testing.T) (*Model, *launchTmux, string, string) {
	t.Helper()
	api, web := t.TempDir(), t.TempDir()
	m := New(nil)
	m.SetSize(160, 40)
	m.SetSessions([]fleet.Session{
		{Info: fleet.SessionInfo{ID: "s-web", TranscriptPath: sessionKey("s-web"), CWD: web, LastEventAt: fixtureBase.Add(30 * time.Minute)}, Live: true},
		{Info: fleet.SessionInfo{ID: "s-api", TranscriptPath: sessionKey("s-api"), CWD: api, LastEventAt: fixtureBase}, Live: true},
	}, fixtureBase.Add(40*time.Minute))
	m.SetPanes(map[string]tmuxop.Pane{sessionKey("s-api"): {Target: "dev:1.0", ID: "%2"}})
	m.point(sessionKey("s-api"))
	tm := &launchTmux{}
	m.runner = tm
	m.harnesses = launch.Harnesses(nil)
	return m, tm, api, web
}

func typeText(m *Model, s string) {
	for _, r := range s {
		if r == ' ' {
			m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
			continue
		}
		press(m, string(r))
	}
}

// run carries a command's message back into the deck, as bubbletea would.
func run(m *Model, cmd tea.Cmd) {
	if cmd != nil {
		m.Update(cmd())
	}
}

// The common launch is three things: n, the prompt, enter. The window opens
// in the selected card's tmux session and directory, named from the
// prompt, running claude with the prompt as one argument.
func TestLaunchIsNThePromptEnter(t *testing.T) {
	m, tm, api, _ := launchModel(t)
	press(m, "n")
	if m.launching == nil {
		t.Fatalf("n opened no panel (note %q)", m.note)
	}
	typeText(m, `map the "auth" flow`)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.launching != nil {
		t.Error("the panel stayed up after enter")
	}
	run(m, cmd)

	got := tm.newWindow()
	want := []string{"new-window", "-d", "-P", "-F", "#{pane_id} #{session_name}:#{window_index}.#{pane_index}",
		"-t", "=dev:", "-c", api, "-n", "map-the-auth-flow", "--", "claude", "-n", "map-the-auth-flow", `map the "auth" flow`}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("new-window\n got %q\nwant %q", got, want)
	}
	if !strings.Contains(m.note, "map-the-auth-flow") || !strings.Contains(m.note, "dev:5.0") {
		t.Errorf("note = %q, want where it opened", m.note)
	}
}

// The rows are choices: the harness, the tmux session, the fleet's
// directories; the worktree is a toggle. A shell takes no prompt, so the
// prompt row is skipped and nothing typed earlier reaches it.
func TestLaunchChoices(t *testing.T) {
	m, tm, api, web := launchModel(t)
	press(m, "n")
	f := m.launching
	if f.dirText != api || len(f.dirs) != 2 || f.dirs[1] != web {
		t.Fatalf("dirs = %q (now %q), want the selected card's first", f.dirs, f.dirText)
	}
	typeText(m, "leftover")
	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // worktree
	m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	if !f.worktree {
		t.Error("space did not turn the worktree on")
	}
	m.Update(tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}})
	for f.field != launchDir {
		m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if f.dirText != web {
		t.Errorf("→ on dir = %q, want %q", f.dirText, web)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // session
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if f.sessions[f.session] != "ops" {
		t.Errorf("→ on session = %q, want ops (it wraps)", f.sessions[f.session])
	}
	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab}) // run
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.harnesses[f.harness].Name != "shell" {
		t.Fatalf("→ on run = %q, want shell", m.harnesses[f.harness].Name)
	}
	for i := 0; i < launchFields; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyTab})
		if f.field == launchPrompt {
			t.Fatal("tab landed on the prompt of a harness that takes none")
		}
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	run(m, cmd)
	got := tm.newWindow()
	if want := []string{"-t", "=ops:", "-c", web, "-n", "shell"}; strings.Join(got[5:], "|") != strings.Join(want, "|") {
		t.Errorf("new-window %q, want a bare shell %q", got, want)
	}
}

func TestLaunchPanelSaysWhatItWillDo(t *testing.T) {
	m := groupedModel(160, 40) // the selected session sits in a pane of dev
	m.runner = &launchTmux{}
	press(m, "n")
	typeText(m, "fix the 401")
	frame := ansi.Strip(m.View())
	for _, want := range []string{"new window", "‹ claude ›", "‹ dev ›", "fix-the-401", "worktree", "› fix the 401", "enter opens"} {
		if !strings.Contains(frame, want) {
			t.Errorf("the panel does not say %q:\n%s", want, frame)
		}
	}
}

// Esc puts the panel away and sends nothing; read-only never opens it; the
// reader keeps n for its matches.
func TestLaunchIsGated(t *testing.T) {
	m, tm, _, _ := launchModel(t)
	press(m, "n")
	typeText(m, "never mind")
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.launching != nil || tm.newWindow() != nil {
		t.Error("esc launched or left the panel up")
	}

	m, tm, _, _ = launchModel(t)
	m.readonly = true
	press(m, "n")
	if m.launching != nil || len(tm.calls) != 0 || !strings.Contains(m.note, "read-only") {
		t.Errorf("read-only: panel %v, calls %q, note %q", m.launching != nil, tm.calls, m.note)
	}

	m, _, _, _ = launchModel(t)
	m.runner = fakeTmux{} // a server with no sessions
	press(m, "n")
	if m.launching != nil || !strings.Contains(m.note, "tmux session") {
		t.Errorf("no tmux session: panel %v, note %q", m.launching != nil, m.note)
	}

	m, _, _, _ = launchModel(t)
	m.level = levelReader
	press(m, "n")
	if m.launching != nil {
		t.Error("n in the reader opened the panel")
	}
}

// Once the new window's session joins the fleet the deck selects it — but
// not if the person has moved on in the meantime.
func TestLaunchLandsOnTheNewSession(t *testing.T) {
	m, _, _, _ := launchModel(t)
	press(m, "n")
	typeText(m, "go")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	run(m, cmd)

	m.Update(panesMsg{panes: map[string]tmuxop.Pane{
		sessionKey("s-api"): {Target: "dev:1.0", ID: "%2"},
		sessionKey("s-web"): {Target: "dev:5.0", ID: "%42"},
	}})
	if m.selectedKey != sessionKey("s-web") {
		t.Errorf("selected %q, want the launched session", m.selectedKey)
	}

	m, _, _, _ = launchModel(t)
	press(m, "n")
	typeText(m, "go")
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	run(m, cmd)
	m.point(sessionKey("s-web")) // the person moved on before it joined
	m.Update(panesMsg{panes: map[string]tmuxop.Pane{sessionKey("s-api"): {Target: "dev:5.0", ID: "%42"}}})
	if m.selectedKey != sessionKey("s-web") {
		t.Errorf("the deck pulled the selection away from the person: %q", m.selectedKey)
	}
}

// The help names `n` where it has the rows, and gives its row up first
// where it does not.
func TestTheHelpNamesTheLaunchKey(t *testing.T) {
	tall := strings.Join(helpLinesWith(120, 60, helpOpts{board: true}), "\n")
	if !strings.Contains(ansi.Strip(tall), "new window in a tmux session") {
		t.Errorf("a tall help has no n row:\n%s", ansi.Strip(tall))
	}
	short := strings.Join(helpLinesWith(120, 20, helpOpts{board: true}), "\n")
	if strings.Contains(ansi.Strip(short), "new window in a tmux session") {
		t.Errorf("a short help kept n over the legend:\n%s", ansi.Strip(short))
	}
}

// A paste keeps its line breaks in the prompt and drops them from a path.
func TestLaunchPasteKeepsLinesOnlyInThePrompt(t *testing.T) {
	m, tm, api, _ := launchModel(t)
	press(m, "n")
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("first\nsecond"), Paste: true})
	m.launchStep(-1) // worktree
	m.launchStep(-1) // name
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("two\nlines"), Paste: true})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	run(m, cmd)
	got := tm.newWindow()
	if want := []string{"-c", api, "-n", "twolines", "--", "claude", "-n", "twolines", "first\nsecond"}; strings.Join(got[7:], "|") != strings.Join(want, "|") {
		t.Errorf("new-window %q, want %q", got[7:], want)
	}
}
