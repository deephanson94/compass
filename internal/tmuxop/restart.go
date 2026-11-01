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
// it looks.
const (
	RestartWait = 15 * time.Second
	restartPoll = 500 * time.Millisecond
)

// Restart exits the CLI in a pane and types resume where its shell comes
// back. Escape first, so a turn in flight or a dialog is closed and /exit
// lands on the prompt; then /exit, which saves and quits as the person's own
// would; then, once the pane is running something else — its shell — the
// resume line. If the CLI has not exited after RestartWait nothing more is
// typed and the error says so. sleep is time.Sleep outside tests.
func Restart(r Runner, paneID, resume string, sleep func(time.Duration)) error {
	before, err := PaneCommand(r, paneID)
	if err != nil {
		return err
	}
	if err := SendKey(r, paneID, "Escape"); err != nil {
		return err
	}
	sleep(300 * time.Millisecond)
	if err := SendKeys(r, paneID, "/exit"); err != nil {
		return err
	}
	for waited := time.Duration(0); waited < RestartWait; waited += restartPoll {
		sleep(restartPoll)
		now, err := PaneCommand(r, paneID)
		if err != nil {
			return err
		}
		if now != before && now != "" {
			return SendKeys(r, paneID, resume)
		}
	}
	return fmt.Errorf("%s did not exit within %s · nothing typed after /exit", before, RestartWait)
}
