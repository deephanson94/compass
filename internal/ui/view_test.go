package ui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/state"
)

// -update rewrites the golden files under testdata/golden instead of comparing
// against them:  go test ./internal/ui -run TestT16 -update
var update = flag.Bool("update", false, "rewrite golden files under testdata/golden")

func goldenPath(name string) string {
	return filepath.Join("..", "..", "testdata", "golden", name)
}

// normalizeFrame makes a rendered terminal frame diffable: trailing spaces on
// every line are dropped (they are invisible padding and vary with lipgloss
// versions) and the frame always ends with exactly one newline.
func normalizeFrame(frame string) string {
	lines := strings.Split(strings.ReplaceAll(frame, "\r\n", "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// compareGolden compares got against testdata/golden/<name>, or rewrites the
// golden file when -update is passed.
func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	got = normalizeFrame(got)
	path := goldenPath(name)

	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		t.Logf("wrote golden %s (%d bytes)", path, len(got))
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run `go test ./internal/ui -update` to create it)", path, err)
	}
	if got != string(want) {
		t.Errorf("rendered frame does not match %s\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

// T16 — an 80x24 monochrome snapshot of the deck with a three-session fleet.
func TestT16DeckViewGolden(t *testing.T) {
	// Force the ASCII profile so the frame is byte-identical on every terminal
	// and in CI — env inspection alone is racy against package-level styles.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	base := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	now := base.Add(40 * time.Minute)

	sessions := []fleet.Session{
		{
			Info: fleet.SessionInfo{
				ID: "s-infra", TranscriptPath: "/x/infra.jsonl", ProjectSlug: "-home-user-infra",
				CWD: "/home/user/infra", GitBranch: "tf/vpc",
				Title: "tighten the vpc security groups", StartedAt: base, LastEventAt: base.Add(38 * time.Minute),
			},
			Snap: state.Snapshot{State: state.NeedsYou, Since: base.Add(38 * time.Minute), Reason: "waiting on your answer", Activity: "AskUserQuestion"},
		},
		{
			Info: fleet.SessionInfo{
				ID: "s-api", TranscriptPath: "/x/api.jsonl", ProjectSlug: "-home-user-api",
				CWD: "/home/user/api", GitBranch: "claude/auth-fx",
				Title: "fix the 401 bug", StartedAt: base, LastEventAt: base.Add(39 * time.Minute),
			},
			Snap: state.Snapshot{State: state.Working, Since: base.Add(37 * time.Minute), Reason: "tool call in flight", Activity: "Bash: pytest tests/auth -x"},
		},
		{
			Info: fleet.SessionInfo{
				ID: "s-docs", TranscriptPath: "/x/docs.jsonl", ProjectSlug: "-home-user-docs",
				CWD: "/home/user/docs", GitBranch: "main",
				Title: "update the readme", StartedAt: base, LastEventAt: base.Add(18 * time.Minute),
			},
			Snap: state.Snapshot{State: state.Idle, Since: base.Add(18 * time.Minute), Reason: "turn complete", Activity: "idle"},
		},
	}

	m := New(nil)
	m.SetSize(80, 24)
	m.SetSessions(sessions, now)

	compareGolden(t, "deck-80x24.txt", m.View())
}
