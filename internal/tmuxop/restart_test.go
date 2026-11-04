package tmuxop_test

import (
	"fmt"
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
	plan := tmuxop.RestartPlan{PaneID: "%5", Resume: "claude --resume 3f2a"}
	f := &restartTmux{exitAfter: 2}
	if err := tmuxop.Restart(f, plan, func(time.Duration) {}); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(f.typed(), "|"), "Escape|/exit|Enter|claude --resume 3f2a|Enter"; got != want {
		t.Errorf("typed %q, want %q", got, want)
	}

	f = &restartTmux{exitAfter: -1}
	err := tmuxop.Restart(f, plan, func(time.Duration) {})
	if err == nil || !strings.Contains(err.Error(), "did not exit") {
		t.Errorf("a CLI that never exits: err %v", err)
	}
	if strings.Contains(strings.Join(f.typed(), "|"), "--resume") {
		t.Errorf("the resume line was typed into a CLI that had not exited: %q", f.typed())
	}
}

// Gone is gone on two looks running: an exit hook between two processes
// (claude gone for one look, a hook's child claude the next) is not the end.
// A CLI that takes most of a minute over its hooks still gets its resume.
func TestRestartWaitsOutASlowExit(t *testing.T) {
	looks := []bool{false, true, false} // then gone for good
	n := 0
	exited := func() bool {
		n++
		if n <= len(looks) {
			return looks[n-1]
		}
		return n > 100 // ~50s of polls at 500ms
	}
	f := &restartTmux{exitAfter: 0} // the pane's command changes at once: not believed
	plan := tmuxop.RestartPlan{PaneID: "%5", Resume: "claude --resume 3f2a", Exited: exited}
	if err := tmuxop.Restart(f, plan, func(time.Duration) {}); err != nil {
		t.Fatal(err)
	}
	if n != 102 {
		t.Errorf("resumed after %d looks, want 102 (two gone running)", n)
	}
	if got := strings.Join(f.typed(), "|"); got != "Escape|/exit|Enter|claude --resume 3f2a|Enter" {
		t.Errorf("typed %q", got)
	}
}

// directTmux is a pane that runs claude itself: /exit kills the pane's
// process, which remain-on-exit keeps as a dead pane.
type directTmux struct {
	calls  [][]string
	remain bool
	dead   bool
}

func (f *directTmux) Output(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	switch args[0] {
	case "set-option":
		f.remain = args[len(args)-1] == "on"
	case "display-message":
		if args[len(args)-1] == "#{pane_dead}" {
			if f.dead {
				return []byte("1\n"), nil
			}
			return []byte("0\n"), nil
		}
		return []byte("claude\n"), nil
	case "send-keys":
		if args[len(args)-1] == "/exit" {
			if !f.remain {
				return nil, fmt.Errorf("can't find pane: %%5") // closed with its process
			}
			f.dead = true
		}
	}
	return nil, nil
}

// A pane `n` opened runs claude with no shell: kept open on exit, the
// resume runs in it as its new process, in the session's directory, and
// the pane goes back to closing when its process ends.
func TestRestartRespawnsADirectPane(t *testing.T) {
	f := &directTmux{}
	plan := tmuxop.RestartPlan{PaneID: "%5", Resume: "claude --resume 3f2a", Dir: "/home/u/rag", Direct: true, Env: []string{"PRW=1"}}
	if err := tmuxop.Restart(f, plan, func(time.Duration) {}); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range f.calls {
		if c[0] != "display-message" {
			got = append(got, strings.Join(c, " "))
		}
	}
	want := []string{
		"set-option -p -t %5 remain-on-exit on",
		"send-keys -t %5 Escape",
		"send-keys -t %5 -l /exit",
		"send-keys -t %5 Enter",
		"respawn-pane -t %5 -c /home/u/rag -e PRW=1 claude --resume 3f2a",
		"set-option -p -u -t %5 remain-on-exit",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("tmux calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
