package ui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/tmuxop"
)

// `R` restarts the selected session's CLI in its own pane: /exit, then
// claude --resume with the session's id (tmuxop.Restart). It is for the
// error only a fresh process clears and the update that needs one, and it
// always asks first — the footer names the session and the very line it
// will type — since a stray key must never quit somebody's session.

// restartAsk is a restart waiting on the person's enter.
type restartAsk struct {
	key, name string
	pane      tmuxop.Pane
	resume    string // the line typed once the shell is back
	working   bool   // a turn is in flight: restarting stops it
	plan      tmuxop.RestartPlan
}

// restartDoneMsg says how a restart went.
type restartDoneMsg struct {
	key, name string
	err       error
}

// askRestart is `R`: the confirmation, with the resume line worked out
// from the running claude's own argv so the footer shows what will be
// typed. A session with no pane has no CLI here to restart.
func (m *Model) askRestart() {
	if m.readonly {
		m.note = "read-only · restart is off"
		return
	}
	s, ok := m.selected()
	if !ok {
		return
	}
	pane, ok := m.selectedPane()
	if !ok {
		m.note = "restart needs a pane"
		return
	}
	cmdline, dir, direct := "", s.Info.CWD, false
	var exited func() bool
	var env []string
	if proc := m.proc; proc != nil {
		if cwd, pid, ok := tmuxop.ClaudeIn(proc, pane.PID); ok {
			cmdline, dir = proc.Cmdline(pid), cwd
			direct = pid == pane.PID // the pane runs claude itself: `n` opened it so
			if e, ok := proc.(interface{ Environ(int) []string }); ok && direct {
				env = e.Environ(pid)
			}
		}
		// Gone is no claude left under the pane, by the process table: the
		// pane's foreground command changes as soon as an exit hook starts,
		// long before the CLI has finished going.
		panePID := pane.PID
		exited = func() bool { _, _, ok := tmuxop.ClaudeIn(proc, panePID); return !ok }
	}
	resume := tmuxop.ResumeCommand(cmdline, s.Info.ID)
	m.restarting = &restartAsk{
		key: s.Info.Key(), name: sessionName(s.Info), pane: pane,
		resume:  resume,
		working: s.Snap.State == state.Working,
		plan:    tmuxop.RestartPlan{PaneID: pane.ID, Resume: resume, Dir: dir, Direct: direct, Env: env, Exited: exited},
	}
}

// restartKey is the key after `R`: enter restarts, anything else keeps
// the session running.
func (m *Model) restartKey(msg tea.KeyMsg) tea.Cmd {
	ask := m.restarting
	m.restarting = nil
	if msg.String() != "enter" {
		m.note = "kept " + ask.name + " running"
		return nil
	}
	runner, sleep := m.runner, m.sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	m.note = "↻ restarting " + ask.name + "…"
	if m.restarts == nil {
		m.restarts = map[string]bool{}
	}
	m.restarts[ask.key] = true
	return func() tea.Msg {
		return restartDoneMsg{key: ask.key, name: ask.name, err: tmuxop.Restart(runner, ask.plan, sleep)}
	}
}

// restartKeys is the footer while a restart waits on enter: the session,
// what will be typed, and what a turn in flight costs.
func (m *Model) restartKeys() string {
	ask := m.restarting
	keys := "↻ restart " + ask.name + ": /exit, then " + ask.resume + " · enter restarts · esc keeps it"
	if ask.working {
		keys = "↻ restart " + ask.name + " · working — restarting stops the turn · enter restarts · esc keeps it"
	}
	return keys
}

// restarted is how the restart went, in the footer.
func (m *Model) restarted(msg restartDoneMsg) {
	delete(m.restarts, msg.key)
	if msg.err != nil {
		m.note = "restart " + msg.name + ": " + firstLine(msg.err.Error())
		return
	}
	m.note = "↻ restarted " + msg.name
}
