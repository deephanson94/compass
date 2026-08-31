package tmuxop_test

// M6 — "Enter goes to the session" (docs/dev/M6-CONTRACT.md).
//
//	tmuxop.Attach(target, paneID string, insideTmux bool) *exec.Cmd
//
// Outside tmux: select-window, select-pane, then `attach-session -t <sess>`.
// Inside tmux:  the same two selects, then `switch-client -t <sess>`, and
// never attach-session.
//
// The contract calls it "ONE command so the focus is set before the terminal is
// handed over (`tmux ... \; ... \; ...`)". The backslashes in that sentence are
// shell quoting: what tmux must actually receive is a bare ";" as an argument
// of its own. An argv element like "\\;" or ";select-pane" is not a separator —
// tmux takes it as text, the selects never run, and Enter lands the user in a
// pane they did not ask for. That is the bug these tests exist to catch, so the
// separators are pinned element by element.
//
// Everything here is pure construction: nothing is started, no tmux server is
// contacted, and the returned command is inspected and then dropped.

import (
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/deephanson94/compass/internal/tmuxop"
)

// ---------------------------------------------------------------- helpers

// tmuxSep is tmux's own command separator, as one argv element.
const tmuxSep = ";"

// assertTmuxBinary pins what is about to be executed. exec.Command resolves the
// name on PATH, so Path is either an absolute tmux or the bare name when tmux is
// not installed — the base is the invariant either way, and Args[0] is what the
// child sees as argv[0].
func assertTmuxBinary(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatalf("Attach returned nil, want a command")
	}
	if len(cmd.Args) == 0 {
		t.Fatalf("Attach returned a command with no args: %+v", cmd)
	}
	if got := filepath.Base(cmd.Path); got != "tmux" {
		t.Errorf("cmd.Path = %q (base %q), want the tmux binary", cmd.Path, got)
	}
	if cmd.Args[0] != "tmux" {
		t.Errorf("cmd.Args[0] = %q, want %q", cmd.Args[0], "tmux")
	}
}

// assertNotStarted pins "builds the command and starts nothing". A started
// command has a Process; a finished one also has a ProcessState. compass hands
// this command to tea.ExecProcess (or runs it itself, inside tmux); if Attach
// has already run it, the terminal has been handed over behind the ui's back.
func assertNotStarted(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	if cmd.Process != nil {
		t.Errorf("cmd.Process = %+v, want nil — Attach must not start anything", cmd.Process)
	}
	if cmd.ProcessState != nil {
		t.Errorf("cmd.ProcessState = %+v, want nil — Attach must not run anything", cmd.ProcessState)
	}
}

// assertArgs compares the whole argv, which is the only way to pin order and
// adjacency at once.
func assertArgs(t *testing.T, cmd *exec.Cmd, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Errorf("argv =\n  %q\nwant\n  %q", cmd.Args, want)
	}
}

// assertStandaloneSeparators fails on the glued forms: a separator fused to the
// verb beside it ("dev:1;"), or shell quoting that leaked into argv ("\\;").
func assertStandaloneSeparators(t *testing.T, cmd *exec.Cmd, want int) {
	t.Helper()
	seps := 0
	for i, a := range cmd.Args {
		if a == tmuxSep {
			seps++
			continue
		}
		if a == `\;` {
			t.Errorf("argv[%d] = %q: that is shell quoting, not a separator; tmux must be handed a bare %q", i, a, tmuxSep)
			continue
		}
		if strings.Contains(a, tmuxSep) {
			t.Errorf("argv[%d] = %q has a %q glued to it; tmux reads it as text and the step never runs",
				i, a, tmuxSep)
		}
	}
	if seps != want {
		t.Errorf("argv has %d standalone %q separator(s), want %d: %q", seps, tmuxSep, want, cmd.Args)
	}
}

// indexOfArg is the order probe: -1 when the verb is absent.
func indexOfArg(cmd *exec.Cmd, arg string) int {
	for i, a := range cmd.Args {
		if a == arg {
			return i
		}
	}
	return -1
}

// assertNoArg pins absence — "never attach-session" inside tmux.
func assertNoArg(t *testing.T, cmd *exec.Cmd, arg string) {
	t.Helper()
	for i, a := range cmd.Args {
		if a == arg || strings.Contains(a, arg) {
			t.Errorf("argv[%d] = %q mentions %q, which must appear nowhere: %q", i, a, arg, cmd.Args)
		}
	}
}

// ---------------------------------------------------------------- T65

// T65 — outside tmux: one tmux command carrying select-window, select-pane and
// attach-session, in that order.
func TestT65AttachOutsideTmux(t *testing.T) {
	cmd := tmuxop.Attach("dev:1.0", "%5", false)

	assertTmuxBinary(t, cmd)

	// The whole thing, exactly: the window target has lost the pane index, the
	// pane is addressed by its id, and the session is what attach-session takes.
	assertArgs(t, cmd,
		"tmux",
		"select-window", "-t", "dev:1",
		tmuxSep,
		"select-pane", "-t", "%5",
		tmuxSep,
		"attach-session", "-t", "dev",
	)

	t.Run("the separators are standalone arguments", func(t *testing.T) {
		assertStandaloneSeparators(t, cmd, 2)
	})

	t.Run("the three steps are in contract order", func(t *testing.T) {
		win := indexOfArg(cmd, "select-window")
		pane := indexOfArg(cmd, "select-pane")
		att := indexOfArg(cmd, "attach-session")
		if win < 0 || pane < 0 || att < 0 {
			t.Fatalf("argv is missing a step (select-window %d, select-pane %d, attach-session %d): %q",
				win, pane, att, cmd.Args)
		}
		if !(win < pane && pane < att) {
			t.Errorf("steps out of order (select-window %d, select-pane %d, attach-session %d): %q — "+
				"the focus must be set before the terminal is handed over",
				win, pane, att, cmd.Args)
		}
	})

	t.Run("it is one command, not three", func(t *testing.T) {
		// Three invocations would mean three Cmds; one invocation means the
		// binary is named once and everything after it is an argument to it.
		for i, a := range cmd.Args[1:] {
			if a == "tmux" {
				t.Errorf("argv[%d] = %q: the binary is named again mid-argv, so this is not one command: %q",
					i+1, a, cmd.Args)
			}
		}
	})

	t.Run("nothing has been started", func(t *testing.T) {
		assertNotStarted(t, cmd)
	})
}

// The pane id and the window index travel verbatim; only the pane index is cut.
func TestT65AttachOutsideTmuxTargets(t *testing.T) {
	cases := []struct {
		name       string
		target     string
		paneID     string
		wantWindow string
		wantSess   string
	}{
		{"single digit", "dev:1.0", "%5", "dev:1", "dev"},
		{"multi digit window and pane", "dev:12.3", "%17", "dev:12", "dev"},
		{"window zero", "work:0.0", "%1", "work:0", "work"},
		{"session name with a dash", "my-app:2.4", "%99", "my-app:2", "my-app"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tmuxop.Attach(tc.target, tc.paneID, false)
			assertTmuxBinary(t, cmd)
			assertArgs(t, cmd,
				"tmux",
				"select-window", "-t", tc.wantWindow,
				tmuxSep,
				"select-pane", "-t", tc.paneID,
				tmuxSep,
				"attach-session", "-t", tc.wantSess,
			)
			assertNotStarted(t, cmd)
		})
	}
}

// ---------------------------------------------------------------- T66

// T66 — inside tmux: the same two selects, then switch-client. attach-session
// from inside a client is the mistake that nests one tmux in another.
func TestT66AttachInsideTmux(t *testing.T) {
	cmd := tmuxop.Attach("dev:1.0", "%5", true)

	assertTmuxBinary(t, cmd)
	assertArgs(t, cmd,
		"tmux",
		"select-window", "-t", "dev:1",
		tmuxSep,
		"select-pane", "-t", "%5",
		tmuxSep,
		"switch-client", "-t", "dev",
	)

	t.Run("attach-session appears nowhere", func(t *testing.T) {
		assertNoArg(t, cmd, "attach-session")
	})

	t.Run("the separators are standalone arguments", func(t *testing.T) {
		assertStandaloneSeparators(t, cmd, 2)
	})

	t.Run("the three steps are in contract order", func(t *testing.T) {
		win := indexOfArg(cmd, "select-window")
		pane := indexOfArg(cmd, "select-pane")
		sw := indexOfArg(cmd, "switch-client")
		if win < 0 || pane < 0 || sw < 0 {
			t.Fatalf("argv is missing a step (select-window %d, select-pane %d, switch-client %d): %q",
				win, pane, sw, cmd.Args)
		}
		if !(win < pane && pane < sw) {
			t.Errorf("steps out of order (select-window %d, select-pane %d, switch-client %d): %q",
				win, pane, sw, cmd.Args)
		}
	})

	t.Run("nothing has been started", func(t *testing.T) {
		assertNotStarted(t, cmd)
	})
}

// The two modes differ in exactly one argv element: the handover verb.
func TestT65T66OnlyTheHandoverVerbDiffers(t *testing.T) {
	out := tmuxop.Attach("dev:1.0", "%5", false)
	in := tmuxop.Attach("dev:1.0", "%5", true)

	if len(out.Args) != len(in.Args) {
		t.Fatalf("argv lengths differ:\n  outside %q\n  inside  %q", out.Args, in.Args)
	}
	var diffs []int
	for i := range out.Args {
		if out.Args[i] != in.Args[i] {
			diffs = append(diffs, i)
		}
	}
	if len(diffs) != 1 {
		t.Fatalf("argvs differ at %v, want exactly one element (the handover verb):\n  outside %q\n  inside  %q",
			diffs, out.Args, in.Args)
	}
	if out.Args[diffs[0]] != "attach-session" || in.Args[diffs[0]] != "switch-client" {
		t.Errorf("handover verbs = %q outside / %q inside, want attach-session / switch-client",
			out.Args[diffs[0]], in.Args[diffs[0]])
	}
}

// ---------------------------------------------------------------- edges

// A target that already names a window, with no pane index to drop.
func TestAttachTargetWithNoPaneIndex(t *testing.T) {
	for _, inside := range []bool{false, true} {
		handover := "attach-session"
		if inside {
			handover = "switch-client"
		}
		t.Run(handover, func(t *testing.T) {
			cmd := tmuxop.Attach("dev:1", "%5", inside)
			assertTmuxBinary(t, cmd)
			assertArgs(t, cmd,
				"tmux",
				"select-window", "-t", "dev:1",
				tmuxSep,
				"select-pane", "-t", "%5",
				tmuxSep,
				handover, "-t", "dev",
			)
			assertNotStarted(t, cmd)
		})
	}
}

// A session name containing a dot. tmux forbids ":" in a session name but
// allows ".", so the split that finds the window is the LAST dot and the split
// that finds the session is the FIRST colon. Getting either backwards sends the
// user to a session that does not exist.
func TestAttachSessionNameContainingADot(t *testing.T) {
	cmd := tmuxop.Attach("my.app:2.1", "%7", false)
	assertTmuxBinary(t, cmd)
	assertArgs(t, cmd,
		"tmux",
		"select-window", "-t", "my.app:2",
		tmuxSep,
		"select-pane", "-t", "%7",
		tmuxSep,
		"attach-session", "-t", "my.app",
	)
	assertNotStarted(t, cmd)

	t.Run("inside tmux too", func(t *testing.T) {
		in := tmuxop.Attach("my.app:2.1", "%7", true)
		assertArgs(t, in,
			"tmux",
			"select-window", "-t", "my.app:2",
			tmuxSep,
			"select-pane", "-t", "%7",
			tmuxSep,
			"switch-client", "-t", "my.app",
		)
		assertNoArg(t, in, "attach-session")
		assertNotStarted(t, in)
	})
}

// An empty target cannot happen through the ui — Enter is gated on a mapped
// session — but Attach is exported and pure, so it must be total. The contract
// says nothing about what the argv should then be; what it must not do is
// panic, and it must still not start anything.
//
// CONTRACT AMBIGUITY: whether Attach refuses an empty/degenerate target (nil,
// or a command tmux will reject) is unspecified. Both readings pass here; only
// a panic or a started process fails.
func TestAttachDegenerateInputsDoNotPanic(t *testing.T) {
	cases := []struct {
		name   string
		target string
		paneID string
	}{
		{"empty target", "", "%5"},
		{"empty target and pane", "", ""},
		{"empty pane id", "dev:1.0", ""},
		{"target is only a session", "dev", "%5"},
		{"target is only separators", ":.", "%5"},
		{"leading dot", ".0", "%5"},
	}
	for _, tc := range cases {
		for _, inside := range []bool{false, true} {
			name := tc.name
			if inside {
				name += " inside tmux"
			}
			t.Run(name, func(t *testing.T) {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("tmuxop.Attach(%q, %q, %v) panicked: %v", tc.target, tc.paneID, inside, r)
					}
				}()
				cmd := tmuxop.Attach(tc.target, tc.paneID, inside)
				if cmd == nil {
					t.Logf("tmuxop.Attach(%q, %q, %v) = nil: a refusal, which the contract permits",
						tc.target, tc.paneID, inside)
					return
				}
				assertTmuxBinary(t, cmd)
				assertNotStarted(t, cmd)
			})
		}
	}
}

// Each call builds its own command: the ui may hold two of these at once (a
// pending attach and the one it is about to replace), and a shared Cmd — or a
// shared backing array under Args — would make the second rewrite the first.
func TestAttachReturnsAFreshCommandEveryTime(t *testing.T) {
	a := tmuxop.Attach("dev:1.0", "%5", false)
	b := tmuxop.Attach("work:2.3", "%9", false)

	if a == b {
		t.Fatalf("Attach returned the same *exec.Cmd twice")
	}
	if got := indexOfArg(a, "dev:1"); got < 0 {
		t.Errorf("the first command lost its target: %q", a.Args)
	}
	if got := indexOfArg(a, "work:2"); got >= 0 {
		t.Errorf("the first command picked up the second's target: %q", a.Args)
	}
	if got := indexOfArg(b, "%9"); got < 0 {
		t.Errorf("the second command lost its pane id: %q", b.Args)
	}
	assertNotStarted(t, a)
	assertNotStarted(t, b)
}
