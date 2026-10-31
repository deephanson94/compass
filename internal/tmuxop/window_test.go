package tmuxop_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/deephanson94/compass/internal/tmuxop"
)

// The window opens in the named session, beside the user's focus rather
// than over it, and the command reaches tmux as argv: the prompt is one
// argument however many quotes and spaces it carries.
func TestOpenWindowPassesTheCommandAsArgv(t *testing.T) {
	f := &fakeRunner{outputs: [][]byte{[]byte("%17 dev:4.0\n")}}
	pane, err := tmuxop.Open(f, tmuxop.PlaceWindow, "dev", "/home/u/api", "rate", []string{"claude", "-n", "rate", `it's "quoted" $HOME`})
	if err != nil {
		t.Fatal(err)
	}
	f.assertCalls(t, []string{"new-window", "-d", "-P", "-F", "#{pane_id} #{session_name}:#{window_index}.#{pane_index}",
		"-t", "=dev:", "-c", "/home/u/api", "-n", "rate", "--", "claude", "-n", "rate", `it's "quoted" $HOME`})
	if want := (tmuxop.Pane{Target: "dev:4.0", ID: "%17", Window: "rate"}); pane != want {
		t.Errorf("pane = %+v, want %+v", pane, want)
	}
}

// No command is the user's own shell, and no name is tmux's to give.
func TestOpenWindowWithNoCommandIsAShell(t *testing.T) {
	f := &fakeRunner{outputs: [][]byte{[]byte("%3 ops:2.0\n")}}
	if _, err := tmuxop.Open(f, tmuxop.PlaceWindow, "ops", "/srv", "", nil); err != nil {
		t.Fatal(err)
	}
	f.assertCalls(t, []string{"new-window", "-d", "-P", "-F", "#{pane_id} #{session_name}:#{window_index}.#{pane_index}",
		"-t", "=ops:", "-c", "/srv"})
}

func TestOpenWindowNeedsASession(t *testing.T) {
	f := &fakeRunner{}
	if _, err := tmuxop.Open(f, tmuxop.PlaceWindow, "", "/srv", "", nil); err == nil {
		t.Fatal("no session: want an error")
	}
	if len(f.calls) != 0 {
		t.Errorf("tmux was called: %v", f.calls)
	}
}

func TestOpenWindowSurfacesTmuxErrors(t *testing.T) {
	f := &fakeRunner{errs: []error{errors.New("can't find session: nope")}}
	if _, err := tmuxop.Open(f, tmuxop.PlaceWindow, "nope", "", "", nil); err == nil {
		t.Fatal("want tmux's error")
	}
}

func TestSessions(t *testing.T) {
	f := &fakeRunner{outputs: [][]byte{[]byte("dev\nops\nmy app\n")}}
	if got, want := tmuxop.Sessions(f), []string{"dev", "ops", "my app"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Sessions = %q, want %q", got, want)
	}
	if got := tmuxop.Sessions(&fakeRunner{errs: []error{errors.New("no server running")}}); got != nil {
		t.Errorf("no server: Sessions = %q, want none", got)
	}
}

// A pane splits beside the one named, side by side, and takes no name: the
// window it joins has one.
func TestOpenPaneSplitsBeside(t *testing.T) {
	f := &fakeRunner{outputs: [][]byte{[]byte("%8 dev:1.1\n")}}
	pane, err := tmuxop.Open(f, tmuxop.PlacePane, "%2", "/home/u/api", "rate", []string{"claude", "go"})
	if err != nil {
		t.Fatal(err)
	}
	f.assertCalls(t, []string{"split-window", "-d", "-h", "-P", "-F", "#{pane_id} #{session_name}:#{window_index}.#{pane_index}",
		"-t", "%2", "-c", "/home/u/api", "--", "claude", "go"})
	if pane.ID != "%8" || pane.Target != "dev:1.1" || pane.Window != "" {
		t.Errorf("pane = %+v", pane)
	}
	if _, err := tmuxop.Open(&fakeRunner{}, tmuxop.PlacePane, "", "", "", nil); err == nil {
		t.Error("a split with nothing to split beside: want an error")
	}
}

// A session is new, detached, and named after the work; its first window
// carries the same name.
func TestOpenSessionIsNamed(t *testing.T) {
	f := &fakeRunner{outputs: [][]byte{[]byte("%11 rate:0.0\n")}}
	pane, err := tmuxop.Open(f, tmuxop.PlaceSession, "ignored", "/srv", "rate", nil)
	if err != nil {
		t.Fatal(err)
	}
	f.assertCalls(t, []string{"new-session", "-d", "-P", "-F", "#{pane_id} #{session_name}:#{window_index}.#{pane_index}",
		"-s", "rate", "-c", "/srv", "-n", "rate"})
	if pane.Target != "rate:0.0" {
		t.Errorf("pane = %+v", pane)
	}
	if _, err := tmuxop.Open(&fakeRunner{}, tmuxop.PlaceSession, "", "", "", nil); err == nil {
		t.Error("an unnamed session: want an error")
	}
}

func TestParsePlace(t *testing.T) {
	for _, p := range tmuxop.Places {
		if got, ok := tmuxop.ParsePlace(p.String()); !ok || got != p {
			t.Errorf("ParsePlace(%q) = %v, %v", p, got, ok)
		}
	}
	if _, ok := tmuxop.ParsePlace("tab"); ok {
		t.Error("tab is no place")
	}
}

// Environment variables go to tmux as -e, before the command, one each:
// the new pane's process has them and nothing else does.
func TestOpenWithSetsTheEnvironment(t *testing.T) {
	f := &fakeRunner{outputs: [][]byte{[]byte("%9 pr:3.0\n")}}
	_, err := tmuxop.OpenWith(f, tmuxop.PlaceWindow, "pr", "/wt/130", "pr130_review",
		[]string{"ATTENTION_NO_IDLE=1", "AWS_REGION=us-west-2"}, []string{"claude", "-n", "pr130_review"})
	if err != nil {
		t.Fatal(err)
	}
	f.assertCalls(t, []string{"new-window", "-d", "-P", "-F", "#{pane_id} #{session_name}:#{window_index}.#{pane_index}",
		"-t", "=pr:", "-c", "/wt/130", "-n", "pr130_review",
		"-e", "ATTENTION_NO_IDLE=1", "-e", "AWS_REGION=us-west-2", "--", "claude", "-n", "pr130_review"})
}

// The pane a person left tmux from: the active pane of the active window
// in the session attached last — not the first session's, and not an
// inactive pane beside it.
func TestLastActivePane(t *testing.T) {
	f := &fakeRunner{outputs: [][]byte{[]byte(
		"1793437463 11 %0\n" + // alpha: attached first
			"1793437465 11 %1\n" + // beta: attached last, its active pane
			"1793437465 10 %2\n" + // beta: a pane beside it
			" 11 %9\n")}} // a session never attached
	if got := tmuxop.LastActivePane(f); got != "%1" {
		t.Errorf("LastActivePane = %q, want %%1", got)
	}
	if got := tmuxop.LastActivePane(&fakeRunner{errs: []error{errors.New("no server")}}); got != "" {
		t.Errorf("no server: %q, want none", got)
	}
}
