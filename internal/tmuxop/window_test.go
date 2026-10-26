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
func TestNewWindowPassesTheCommandAsArgv(t *testing.T) {
	f := &fakeRunner{outputs: [][]byte{[]byte("%17 dev:4.0\n")}}
	pane, err := tmuxop.NewWindow(f, "dev", "/home/u/api", "rate", []string{"claude", "-n", "rate", `it's "quoted" $HOME`})
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
func TestNewWindowWithNoCommandIsAShell(t *testing.T) {
	f := &fakeRunner{outputs: [][]byte{[]byte("%3 ops:2.0\n")}}
	if _, err := tmuxop.NewWindow(f, "ops", "/srv", "", nil); err != nil {
		t.Fatal(err)
	}
	f.assertCalls(t, []string{"new-window", "-d", "-P", "-F", "#{pane_id} #{session_name}:#{window_index}.#{pane_index}",
		"-t", "=ops:", "-c", "/srv"})
}

func TestNewWindowNeedsASession(t *testing.T) {
	f := &fakeRunner{}
	if _, err := tmuxop.NewWindow(f, "", "/srv", "", nil); err == nil {
		t.Fatal("no session: want an error")
	}
	if len(f.calls) != 0 {
		t.Errorf("tmux was called: %v", f.calls)
	}
}

func TestNewWindowSurfacesTmuxErrors(t *testing.T) {
	f := &fakeRunner{errs: []error{errors.New("can't find session: nope")}}
	if _, err := tmuxop.NewWindow(f, "nope", "", "", nil); err == nil {
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
