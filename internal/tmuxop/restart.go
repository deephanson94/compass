package tmuxop

import (
	"fmt"
	"strings"
	"time"
)

// Restarting a session: exit the CLI the way a person would and start it
// again on the same conversation, in the same pane. An API error that only
// a fresh process clears, or a Claude Code update, used to be the person's
// own five steps — go to the pane, /exit, wait, type the resume command
// with the session's id, come back. compass knows the pane and the id.

// resumeFlags are the flags a resumed claude keeps from the one it
// replaces, each with one value (or none): what decides how it runs — its
// model, its settings, its permissions, the directories it may touch. The
// first prompt, the name and an earlier --resume or --continue are not
// carried: the conversation they started is what --resume brings back.
var resumeFlags = map[string]bool{ // flag → takes a value
	"--model":                        true,
	"--settings":                     true,
	"--permission-mode":              true,
	"--add-dir":                      true,
	"--dangerously-skip-permissions": false,
}

// ResumeCommand is the shell line that resumes session id: claude --resume
// with the flags of cmdline (the old process's argv, space-joined) that
// resumeFlags keeps, each quoted for the shell it is typed into.
func ResumeCommand(cmdline, id string) string {
	args := []string{"claude"}
	f := strings.Fields(cmdline)
	for i := 0; i < len(f); i++ {
		flag, value, inline := strings.Cut(f[i], "=")
		takes, ok := resumeFlags[flag]
		if !ok {
			continue
		}
		switch {
		case !takes:
			args = append(args, flag)
		case inline:
			args = append(args, flag, shellQuote(value))
		case i+1 < len(f):
			args = append(args, flag, shellQuote(f[i+1]))
			i++
		}
	}
	return strings.Join(append(args, "--resume", shellQuote(id)), " ")
}

// shellQuote leaves a plain word as it is and single-quotes anything else.
func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return !(r == '-' || r == '_' || r == '.' || r == '/' || r == ':' || r == '=' || r == '~' ||
			r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9')
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// PaneCommand is what a pane is running now (pane_current_command): the
// CLI's name while it runs, the shell's once it has exited.
func PaneCommand(r Runner, paneID string) (string, error) {
	out, err := r.Output("display-message", "-p", "-t", paneID, "#{pane_current_command}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// RestartWait is how long Restart waits for the CLI to exit, and how often
// it looks. A minute: Claude Code runs its SessionEnd hooks on the way out,
// and a person's hooks (an indexer, a notifier) took longer than the fifteen
// seconds this once was, so the resume line was never typed.
const (
	RestartWait = 60 * time.Second
	restartPoll = 500 * time.Millisecond
)

// RestartPlan is how one pane is restarted.
type RestartPlan struct {
	PaneID string
	Resume string // the shell line that resumes the session
	Dir    string // its working directory, for a pane with no shell of its own
	// Direct says the pane runs the CLI itself, not a shell that runs it
	// (tmux new-window -- claude …, as `n` opens it): /exit ends the pane's
	// own process, and without remain-on-exit the pane — and a window of
	// one — closes with it, leaving nowhere to type the resume line.
	Direct bool
	// Env is the old CLI's environment, for a direct pane's respawn: what
	// `compass new -e` gave the pane's process is not tmux's to give again.
	Env []string
	// Exited says the CLI is gone from the pane: no claude anywhere under
	// it, by the process table. nil falls back to the pane's foreground
	// command changing, which a hook starting on the way out also does.
	Exited func() bool
}

// Restart exits the CLI in a pane and starts it again on the same
// conversation, in the same pane. Escape first, so a turn in flight or a
// dialog is closed and /exit lands on the prompt; then /exit, which saves
// and quits as the person's own would; then, once the CLI has gone — seen
// gone on two looks running, so a hook between two of its processes is
// not taken for the end — the resume line: typed into the shell that came
// back, or, in a pane with no shell, run in the pane kept open for it. If
// the CLI has not gone after RestartWait nothing more is done and the
// error says so. sleep is time.Sleep outside tests.
func Restart(r Runner, plan RestartPlan, sleep func(time.Duration)) error {
	before, err := PaneCommand(r, plan.PaneID)
	if err != nil {
		return err
	}
	exited := plan.Exited
	if plan.Direct {
		// The pane outlives its process for the respawn below.
		if _, err := r.Output("set-option", "-p", "-t", plan.PaneID, "remain-on-exit", "on"); err != nil {
			return err
		}
		defer r.Output("set-option", "-p", "-u", "-t", plan.PaneID, "remain-on-exit")
		exited = func() bool { return paneDead(r, plan.PaneID) }
	}
	if exited == nil {
		exited = func() bool {
			now, err := PaneCommand(r, plan.PaneID)
			return err == nil && now != "" && now != before
		}
	}
	if err := SendKey(r, plan.PaneID, "Escape"); err != nil {
		return err
	}
	sleep(300 * time.Millisecond)
	if err := SendKeys(r, plan.PaneID, "/exit"); err != nil {
		return err
	}
	seen := 0
	for waited := time.Duration(0); waited < RestartWait; waited += restartPoll {
		sleep(restartPoll)
		if !exited() {
			seen = 0
			continue
		}
		if seen++; seen < 2 {
			continue
		}
		if plan.Direct {
			args := []string{"respawn-pane", "-t", plan.PaneID}
			if plan.Dir != "" {
				args = append(args, "-c", plan.Dir)
			}
			for _, kv := range plan.Env {
				args = append(args, "-e", kv)
			}
			_, err := r.Output(append(args, plan.Resume)...) // one string: tmux runs it through sh -c
			return err
		}
		sleep(300 * time.Millisecond) // the shell's prompt, before its first keystroke
		return SendKeys(r, plan.PaneID, plan.Resume)
	}
	return fmt.Errorf("%s did not exit within %s · nothing typed after /exit", before, RestartWait)
}

// paneDead says the pane's own process has ended (remain-on-exit keeps it).
func paneDead(r Runner, paneID string) bool {
	out, err := r.Output("display-message", "-p", "-t", paneID, "#{pane_dead}")
	return err == nil && strings.TrimSpace(string(out)) == "1"
}
