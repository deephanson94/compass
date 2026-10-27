package fleet_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/state"
)

// A turn in flight whose process is gone is exited, not working and not
// hung: Ctrl+C twice or a closed terminal leaves the open call in the file
// forever. The registry decides, and only where it can be read.
func TestATurnWhoseProcessIsGoneIsExited(t *testing.T) {
	mine := os.Getpid()
	for _, c := range []struct {
		name   string
		age    time.Duration
		entry  func(t *testing.T, root string)
		exited bool
	}{
		{"running", 2 * time.Minute, func(t *testing.T, root string) {
			writeRegistry(t, root, mine, idWaitingDay, procStartOf(t, mine))
		}, false},
		{"not in a readable registry", 2 * time.Minute, func(t *testing.T, root string) {
			writeRegistry(t, root, mine, "some-other-session", procStartOf(t, mine))
		}, true},
		{"not in it, but it wrote a moment ago", 30 * time.Second, func(t *testing.T, root string) {
			writeRegistry(t, root, mine, "some-other-session", procStartOf(t, mine))
		}, false},
		{"no registry to read", 2 * time.Minute, func(t *testing.T, root string) {}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			stalledCallAt(t, root, slugAlpha, idWaitingDay, c.age)
			c.entry(t, root)
			s := pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idWaitingDay)
			if !s.Live {
				t.Fatal("inside the recency window, want live")
			}
			if s.Snap.Exited != c.exited {
				t.Fatalf("Exited = %v (%s, %q), want %v", s.Snap.Exited, s.Snap.State, s.Snap.Reason, c.exited)
			}
			if c.exited && (s.Snap.State != state.Idle || s.Snap.Reason != "exited mid-turn" || s.Snap.Open) {
				t.Errorf("exited snap = %s %q open=%v, want idle, \"exited mid-turn\", closed", s.Snap.State, s.Snap.Reason, s.Snap.Open)
			}
		})
	}
}

// A subagent's own file is the session's clock while its Agent call is out:
// Claude Code writes it under <session-id>/subagents/, and the lead's
// transcript says nothing until the agent hands back.
func TestASubagentWritingKeepsItsSessionWorking(t *testing.T) {
	root := t.TempDir()
	calledAt := ago(40 * time.Minute)
	newTranscript(t, idWaitingDay, "/home/user/alpha", "main").
		prompt(ago(41*time.Minute), "refactor the parser").
		tool(calledAt, "toolu_agent1", "Agent", map[string]any{"prompt": "refactor"}).
		write(root, slugAlpha)

	m := fleet.NewManager(root)
	m.MarkPaneMapped(panesFor(t, root, idWaitingDay))
	s := pick(t, mustRefresh(t, m, fleetNow), idWaitingDay)
	if !s.Snap.Late || s.Snap.Pending != state.PendingSubagent {
		t.Fatalf("no subagent file: Late/Pending = %v/%q, want still working on the subagent", s.Snap.Late, s.Snap.Pending)
	}

	dir := filepath.Join(root, "projects", slugAlpha, idWaitingDay, "subagents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent-a1.meta.json"), []byte(`{"toolUseId":"toolu_agent1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	agent := filepath.Join(dir, "agent-a1.jsonl")
	if err := os.WriteFile(agent, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wrote := ago(30 * time.Second)
	if err := os.Chtimes(agent, wrote, wrote); err != nil {
		t.Fatal(err)
	}
	s = pick(t, mustRefresh(t, m, fleetNow), idWaitingDay)
	if s.Snap.State != state.Working || s.Snap.Late {
		t.Errorf("a subagent that wrote 30s ago: %s late=%v (%q), want working", s.Snap.State, s.Snap.Late, fmt.Sprint(s.Snap.Reason))
	}
}
