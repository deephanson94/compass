package tmuxop_test

import (
	"testing"

	"github.com/deephanson94/compass/internal/tmuxop"
)

// An opencode process is a session's pane as a claude one is, and the
// pane knows which: a claude session never lands in an opencode's pane.
func TestAnOpencodeProcessIsAToolOfItsOwn(t *testing.T) {
	p := &fakeProc{
		comm:    map[int]string{1: "opencode", 2: "node", 3: "vim", 4: "claude"},
		cmdline: map[int]string{1: "opencode", 2: "node /usr/lib/node_modules/opencode-ai/bin/opencode", 3: "vim opencode-notes.md", 4: "claude"},
		cwd:     map[int]string{1: "/a", 2: "/b", 3: "/c", 4: "/d"},
	}
	for pid, want := range map[int]string{1: "opencode", 2: "opencode", 3: "", 4: "claude"} {
		if got := tmuxop.ToolIn(p, pid); got != want {
			t.Errorf("ToolIn(pid %d) = %q, want %q", pid, got, want)
		}
	}
}
