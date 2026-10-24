package tmuxop_test

import (
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/tmuxop"
)

// threeInOneDir is #71's machine: three panes, a claude in each, all started
// in /w/app, and three sessions that have all written since every pane began.
// dev:0.0 really runs a, dev:1.0 b, dev:2.0 c — what Claude Code's registry
// says of the three claude pids.
func threeInOneDir() ([]fleet.SessionInfo, []tmuxop.Pane, *fakeProc, map[int]string) {
	p := &fakeProc{
		children: map[int][]int{100: {101}, 200: {201}, 300: {301}},
		comm:     map[int]string{100: "zsh", 101: "claude", 200: "zsh", 201: "claude", 300: "zsh", 301: "claude"},
		cwd:      map[int]string{101: "/w/app", 201: "/w/app", 301: "/w/app"},
		started: map[int]time.Time{
			101: at(ago(3 * time.Hour)),
			201: at(ago(2 * time.Hour)),
			301: at(ago(time.Hour)),
		},
	}
	panes := []tmuxop.Pane{
		{Target: "dev:0.0", ID: "%1", PID: 100, Command: "claude"},
		{Target: "dev:1.0", ID: "%2", PID: 200, Command: "claude"},
		{Target: "dev:2.0", ID: "%3", PID: 300, Command: "claude"},
	}
	sessions := []fleet.SessionInfo{
		sessionAt("a", "/w/app", ago(10*time.Second)),
		sessionAt("b", "/w/app", ago(20*time.Second)),
		sessionAt("c", "/w/app", ago(30*time.Second)),
	}
	registry := map[int]string{101: "a", 201: "b", 301: "c"}
	return sessions, panes, p, registry
}

// #71 itself. By directory alone the three are told apart by recency, and
// recency pairs them wrong; the registry pairs each pane with the session
// its own process runs.
func TestSessionsSharingADirectoryArePairedByTheRegistry(t *testing.T) {
	sessions, panes, p, registry := threeInOneDir()
	want := map[string]string{"a": "dev:0.0", "b": "dev:1.0", "c": "dev:2.0"}

	// The bug, reproduced: a cwd cannot say which is which.
	guess := tmuxop.MapSessions(sessions, panes, p)
	wrong := 0
	for id, target := range want {
		if guess[keyOf(id)].Target != target {
			wrong++
		}
	}
	if wrong == 0 {
		t.Fatal("the fixture does not reproduce #71: directory alone paired all three right")
	}

	got := tmuxop.MapSessionsBy(sessions, panes, p, registry)
	for id, target := range want {
		if got[keyOf(id)].Target != target {
			t.Errorf("session %s paired with %q, want %s (its own process's pane)", id, got[keyOf(id)].Target, target)
		}
	}
}

// A session the registry places in a process outside tmux — another
// terminal — is not handed a neighbour's pane by the directory rule, even
// a pane the registry says nothing about and nobody else claims.
func TestARegisteredSessionOutsideTmuxTakesNoPane(t *testing.T) {
	sessions, panes, p, registry := threeInOneDir()
	registry[999] = "c"   // c runs in a terminal of its own, not in tmux
	delete(registry, 301) // and dev:2.0's process is one the registry does not list
	got := tmuxop.MapSessionsBy(sessions, panes, p, registry)
	if pane, ok := got[keyOf("c")]; ok {
		t.Errorf("c, running outside tmux, wears %s", pane.Target)
	}
	if got[keyOf("a")].Target != "dev:0.0" || got[keyOf("b")].Target != "dev:1.0" {
		t.Errorf("got %+v", got)
	}
}

// A pane whose process the registry names, but whose session the fleet has
// no transcript for yet, is no neighbour's to take.
func TestAPaneOfANewSessionIsNoNeighboursPane(t *testing.T) {
	sessions, panes, p, registry := threeInOneDir()
	registry[301] = "brand-new"                                               // c's pane now runs a session just opened
	sessions = append(sessions, sessionAt("d", "/w/app", ago(5*time.Second))) // unregistered, recent
	got := tmuxop.MapSessionsBy(sessions, panes, p, registry)
	for _, id := range []string{"c", "d"} {
		if pane, ok := got[keyOf(id)]; ok && pane.Target == "dev:2.0" {
			t.Errorf("%s took dev:2.0, the pane the registry gives a session it has no transcript for", id)
		}
	}
}

// The registered pid can sit below the process ClaudeIn stops at — a
// wrapper between them — and still names the pane's session.
func TestTheRegisteredPidMaySitDeeperInThePane(t *testing.T) {
	sessions, panes, p, registry := threeInOneDir()
	p.children[101] = []int{102} // a's claude is a wrapper around the CLI
	p.comm[102] = "node"
	delete(registry, 101)
	registry[102] = "a"
	got := tmuxop.MapSessionsBy(sessions, panes, p, registry)
	if got[keyOf("a")].Target != "dev:0.0" {
		t.Errorf("a paired with %q, want dev:0.0", got[keyOf("a")].Target)
	}
}

// Where the registry says nothing of a pane — an older Claude Code, an
// OpenCode — the directory rule still pairs it.
func TestAnUnregisteredPaneIsStillPairedByDirectory(t *testing.T) {
	sessions, panes, p, registry := threeInOneDir()
	delete(registry, 301)
	sessions = sessions[:2]
	sessions = append(sessions, sessionAt("old-cli", "/w/app", ago(time.Minute)))
	got := tmuxop.MapSessionsBy(sessions, panes, p, registry)
	if got[keyOf("old-cli")].Target != "dev:2.0" {
		t.Errorf("the unregistered session got %+v, want dev:2.0", got[keyOf("old-cli")])
	}
}

// Two transcripts with the registered id: the one written last is the live one.
func TestARegisteredIdWithTwoTranscriptsTakesTheNewest(t *testing.T) {
	sessions, panes, p, registry := threeInOneDir()
	twin := sessionAt("a", "/w/app", ago(time.Hour))
	twin.TranscriptPath = "/x/projects/-w-other/a.jsonl"
	sessions = append(sessions, twin)
	got := tmuxop.MapSessionsBy(sessions, panes, p, registry)
	if got[keyOf("a")].Target != "dev:0.0" {
		t.Errorf("the newer transcript got %+v, want dev:0.0", got[keyOf("a")])
	}
	if _, ok := got[twin.Key()]; ok {
		t.Error("the older transcript with the same id took a pane too")
	}
}
