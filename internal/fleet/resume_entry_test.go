package fleet_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
)

// A deck that resumes a live session from the cache must show the row it
// showed before it quit: its class and its last result come from the events
// the mark already covers, and a restore that replays nothing has nowhere
// else to learn them (round 71).
func TestAWarmWakeKeepsTheRowsClassAndOutcome(t *testing.T) {
	root := t.TempDir()
	const sess = "beef0000-0000-4000-8000-000000000071"
	newTranscript(t, sess, "/home/user/app", "main").
		prompt(ago(3*time.Minute), "run the suite").
		tool(ago(2*time.Minute), "t1", "Bash", map[string]any{"command": "pytest -x"}).
		result(ago(90*time.Second), "t1", "==== 3 passed, 1 failed in 0.4s ====").
		tool(ago(time.Minute), "t2", "Bash", map[string]any{"command": "pytest tests/auth -x"}).
		write(root, "-home-user-app")
	cachePath := filepath.Join(t.TempDir(), "resume.json")

	first := fleet.OpenResumeCache(cachePath)
	m := fleet.NewManager(root)
	m.UseResumeCache(first)
	before := onlySession(t, m)
	if !before.HasClass || before.Outcome == "" {
		t.Fatalf("the cold read gave class %v/%v and outcome %q; the test needs both", before.Class, before.HasClass, before.Outcome)
	}
	first.Save()

	// The next process: nothing replays.
	m2 := fleet.NewManager(root)
	m2.UseResumeCache(fleet.OpenResumeCache(cachePath))
	after := onlySession(t, m2)
	if !after.Live {
		t.Fatal("the session should still be live")
	}
	if after.HasClass != before.HasClass || after.Class != before.Class {
		t.Errorf("a warm wake shows class %v/%v, the cold read showed %v/%v", after.Class, after.HasClass, before.Class, before.HasClass)
	}
	if after.Outcome != before.Outcome {
		t.Errorf("a warm wake shows outcome %q, the cold read showed %q", after.Outcome, before.Outcome)
	}
	if after.Info.Model != before.Info.Model {
		t.Errorf("a warm wake shows model %q, the cold read showed %q", after.Info.Model, before.Info.Model)
	}
}
