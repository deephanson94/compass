package tmuxop

import (
	"fmt"
	"strings"
)

// Sessions lists the tmux sessions on the server, in tmux's own order. A
// tmux error (no server, no binary) is an empty list, as for ListPanes.
func Sessions(r Runner) []string {
	out, err := r.Output("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names
}

// NewWindow opens a window in an existing tmux session and returns its one
// pane. It is the deck's third write, and like the other two it runs only
// from a keypress (or from `compass new`, which is one). It creates a
// window and nothing more: the session must already be there, and once the
// window is open compass treats it like any other pane — it never closes,
// renames or rearranges it.
//
// -d leaves the user's focus where it was; -P -F prints where the window
// landed, so the caller can find the pane without waiting for the next
// list-panes. dir is the window's working directory, name its name ("" lets
// tmux name it after its command). argv is the command, passed to tmux as
// separate arguments after "--": tmux runs two or more of them directly,
// with no shell between, so a prompt's quotes and dollars arrive as typed.
// No argv is the user's default shell.
// newWindowFormat is where the window landed: the pane id first, since it
// never holds a space, so the target after it may (a session name can).
// Not a tab: tmux 3.4 prints a control character in -F output as "_".
const newWindowFormat = "#{pane_id} #{session_name}:#{window_index}.#{pane_index}"

func NewWindow(r Runner, session, dir, name string, argv []string) (Pane, error) {
	if session == "" {
		return Pane{}, fmt.Errorf("no tmux session to open a window in")
	}
	args := []string{"new-window", "-d", "-P", "-F", newWindowFormat, "-t", "=" + session + ":"}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	if name != "" {
		args = append(args, "-n", name)
	}
	if len(argv) > 0 {
		args = append(args, "--")
		args = append(args, argv...)
	}
	out, err := r.Output(args...)
	if err != nil {
		return Pane{}, err
	}
	id, target, ok := strings.Cut(strings.TrimSpace(string(out)), " ")
	if !ok || !strings.HasPrefix(id, "%") {
		return Pane{}, fmt.Errorf("tmux said %q, not where the window opened", strings.TrimSpace(string(out)))
	}
	return Pane{Target: target, ID: id, Window: name}, nil
}
