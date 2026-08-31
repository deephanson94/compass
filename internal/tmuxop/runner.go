// Package tmuxop talks to a tmux server the user already runs. compass creates
// no sessions, windows or panes: it lists what is there, mirrors one pane's
// screen, and — only on a keypress — moves the user's focus to it or hands the
// terminal over to it.
//
// Everything reaches the outside world through two seams, Runner (the tmux
// binary) and Proc (the process table), so the rest of compass and its tests
// never need a tmux server.
package tmuxop

import (
	"os/exec"
	"strings"
)

// Runner executes `tmux <args...>` and returns stdout.
type Runner interface {
	Output(args ...string) ([]byte, error)
}

// RealRunner shells out to the tmux binary on PATH.
type RealRunner struct{}

// Output runs tmux with the given arguments. A missing binary or a non-zero
// exit both surface as errors; deciding what they mean is the caller's job —
// for ListPanes they simply mean "no tmux here".
func (RealRunner) Output(args ...string) ([]byte, error) {
	return exec.Command("tmux", args...).Output()
}

// Capture runs: capture-pane -p -e -J -t <paneID>  and returns the raw
// ANSI-laden screen text. -e keeps the colors, -J rejoins wrapped lines.
func Capture(r Runner, paneID string) (string, error) {
	out, err := r.Output("capture-pane", "-p", "-e", "-J", "-t", paneID)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Reveal focuses the pane in the user's own tmux: select-window on the pane's
// window, then select-pane on the pane itself. This is one of the two writes
// compass ever makes, and both are keypress-gated.
func Reveal(r Runner, target, paneID string) error {
	if _, err := r.Output("select-window", "-t", windowTarget(target)); err != nil {
		return err
	}
	_, err := r.Output("select-pane", "-t", paneID)
	return err
}

// Attach hands the terminal to a pane. Outside tmux that means attaching this
// terminal to the pane's session; inside tmux it means switching this client to
// it — the same intent, the shape the situation allows.
//
// Both select the window and pane first, so the client lands where the caller
// pointed. It is one tmux invocation on purpose: the selects have to be in
// place before the terminal is handed over, and a separate command per step
// would hand it over first and focus afterwards, which is what made Enter feel
// dead (docs/dev/M6-CONTRACT.md). ";" is tmux's own command separator, passed
// as an argument of its own — glued to a neighbour it would just be text.
//
// Pure construction: it builds the command and starts nothing.
func Attach(target, paneID string, insideTmux bool) *exec.Cmd {
	handover := "attach-session"
	if insideTmux {
		handover = "switch-client" // the user's client moves; nothing is suspended
	}
	return exec.Command("tmux",
		"select-window", "-t", windowTarget(target),
		";", "select-pane", "-t", paneID,
		";", handover, "-t", sessionTarget(target),
	)
}

// windowTarget drops the pane index from a pane target: "dev:1.0" → "dev:1".
// A target without a pane index is already a window target.
func windowTarget(target string) string {
	if i := strings.LastIndexByte(target, '.'); i >= 0 {
		return target[:i]
	}
	return target
}

// sessionTarget keeps only the session a pane target names: "dev:1.0" → "dev".
// The split is on the last ":" — the coordinates are the part we are certain
// about, so whatever precedes them is the session, however it is spelled.
func sessionTarget(target string) string {
	if i := strings.LastIndexByte(target, ':'); i >= 0 {
		return target[:i]
	}
	return target
}
