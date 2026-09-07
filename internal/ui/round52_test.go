package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/transcript"
)

// The pins of the first live look after the panel (#78).

// The harness's error tag never reaches a label or a reader row.
func TestTheErrorTagNeverReachesARow(t *testing.T) {
	if got := journey.Untag("<tool_use_error>File has not been read yet.</tool_use_error>"); got != "File has not been read yet." {
		t.Errorf("Untag = %q", got)
	}
	lines := resultBody("<tool_use_error>File has not been read yet. Read it first.</tool_use_error>\n")
	if len(lines) != 1 || strings.Contains(lines[0], "<") {
		t.Errorf("resultBody keeps the tag: %q", lines)
	}
}

// A command that begins by entering the session's own directory is shown
// from what it does; a bare cd, or a cd elsewhere, stays.
func TestACommandsOwnCDIntoTheSessionIsNotTheNews(t *testing.T) {
	cwd := "/home/user/api"
	for cmd, want := range map[string]string{
		"cd /home/user/api; git diff tests/": "git diff tests/",
		"cd /home/user/api && go test ./...": "go test ./...",
		"cd /home/user/api/ ; pytest -x":     "pytest -x",
		"cd /home/user/api":                  "cd /home/user/api",
		"cd /home/user/other; make":          "cd /home/user/other; make",
		"git status":                         "git status",
	} {
		use := transcript.ToolUse{Name: "Bash", Input: json.RawMessage(`{"command":` + jsonQuote(cmd) + `}`)}
		got, _ := state.StripCD(cmd, cwd)
		if got != want {
			t.Errorf("StripCD(%q) = %q, want %q", cmd, got, want)
		}
		d := &docBuilder{width: 80, cwd: cwd}
		if got := d.argument(use, cwd); got != want {
			t.Errorf("argument(%q) = %q, want %q", cmd, got, want)
		}
	}
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// The grab key is offered only while a session is waiting on you.
func TestGrabIsOfferedOnlyWhileSomethingWaits(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFewOngoing(), 120, 34)
	if foot := ansi.Strip(m.footerLine(118)); !strings.Contains(foot, "g grab") {
		t.Errorf("a fleet with a question open should offer g: %q", foot)
	}
	for i := range m.sessions {
		if m.sessions[i].Snap.State == state.NeedsYou {
			m.sessions[i].Snap.State = state.Working
		}
	}
	if foot := ansi.Strip(m.footerLine(118)); strings.Contains(foot, "g grab") {
		t.Errorf("nothing amber: g answers no question: %q", foot)
	}
}

// The band ranks a session with legs before one without.
func TestTheBandRanksWorkedSessionsFirst(t *testing.T) {
	m := sceneModel(sceneSecondDay(), 120, 34)
	key := ""
	for _, s := range m.sessions {
		if !s.Live {
			key = s.Info.Key()
			break
		}
	}
	// The newest archived session, emptied of its legs: it drops behind
	// the ones that did work.
	rows := m.recentRows(8)
	first := m.sessions[rows[0].sess].Info.Key()
	tr := m.trails[first]
	tr.Legs = nil
	m.trails[first] = tr
	if again := m.recentRows(8); m.sessions[again[0].sess].Info.Key() == first {
		t.Errorf("an empty session leads the band over worked ones: %v", key)
	}
}
