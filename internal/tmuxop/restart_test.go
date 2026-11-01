package tmuxop_test

import (
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/tmuxop"
)

// The resumed claude keeps what decides how it runs, and nothing of how
// the old one began: not its prompt, not its name, not an earlier resume.
func TestResumeCommand(t *testing.T) {
	for _, c := range []struct{ cmdline, want string }{
		{"claude", "claude --resume 3f2a"},
		{"claude -n tinker port the runner", "claude --resume 3f2a"},
		{"claude --resume old-id", "claude --resume 3f2a"},
		{"claude --model opus --settings /home/u/.config/pr-watcher/reviewer.json -n pr130 review this",
			"claude --model opus --settings /home/u/.config/pr-watcher/reviewer.json --resume 3f2a"},
		{"claude --permission-mode=plan --dangerously-skip-permissions", "claude --permission-mode plan --dangerously-skip-permissions --resume 3f2a"},
		{"/usr/bin/node /opt/claude/cli.js --add-dir /tmp/my dir", "claude --add-dir /tmp/my --resume 3f2a"},
	} {
		if got := tmuxop.ResumeCommand(c.cmdline, "3f2a"); got != c.want {
			t.Errorf("ResumeCommand(%q)\n got %q\nwant %q", c.cmdline, got, c.want)
		}
	}
	if got := tmuxop.ResumeCommand("claude --model it's", "id"); got != `claude --model 'it'\''s' --resume id` {
		t.Errorf("quoting: %q", got)
	}
}

// restartTmux is a pane running claude that exits after /exit, a few polls
// later, or never.
type restartTmux struct {
	calls     [][]string
	exitAfter int // polls after /exit before the shell is back; -1 never
	exited    bool
	polls     int
}

func (f *restartTmux) Output(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	switch args[0] {
	case "display-message":
		if f.exited {
			f.polls++
			if f.exitAfter >= 0 && f.polls > f.exitAfter {
				return []byte("bash\n"), nil
			}
		}
		return []byte("claude\n"), nil
	case "send-keys":
		if args[len(args)-1] == "/exit" {
			f.exited = true
		}
	}
	return nil, nil
}

func (f *restartTmux) typed() []string {
	var out []string
	for _, c := range f.calls {
		if c[0] == "send-keys" {
			out = append(out, c[len(c)-1])
		}
	}
	return out
}

// Escape, /exit, and the resume line once the shell is back — nothing
// typed into a CLI that has not exited.
func TestRestartExitsThenResumes(t *testing.T) {
	f := &restartTmux{exitAfter: 2}
	if err := tmuxop.Restart(f, "%5", "claude --resume 3f2a", func(time.Duration) {}); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(f.typed(), "|"), "Escape|/exit|Enter|claude --resume 3f2a|Enter"; got != want {
		t.Errorf("typed %q, want %q", got, want)
	}

	f = &restartTmux{exitAfter: -1}
	err := tmuxop.Restart(f, "%5", "claude --resume 3f2a", func(time.Duration) {})
	if err == nil || !strings.Contains(err.Error(), "did not exit") {
		t.Errorf("a CLI that never exits: err %v", err)
	}
	if strings.Contains(strings.Join(f.typed(), "|"), "--resume") {
		t.Errorf("the resume line was typed into a CLI that had not exited: %q", f.typed())
	}
}
