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
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/tmuxop"
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

// forceASCII pins the ASCII colour profile so a frame is byte-identical on
// every terminal and in CI — env inspection alone is racy against
// package-level styles. It is also the monochrome proof: every golden in this
// file has to read correctly with the colour switched off (SPEC §4).
func forceASCII(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.Ascii)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

// fixtureBase is the clock every golden in this file is drawn against.
var fixtureBase = time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

// fixtureSessions is the three-session fleet the deck goldens share: one
// waiting on the human, one working, one idle.
func fixtureSessions(base time.Time) []fleet.Session {
	return []fleet.Session{
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
}

// fixtureTrail is the journey the SPEC §2.1 mockup draws: a prompt, a scout leg
// that became a build, a test run, a subagent that forked off it and came back,
// and a fix leg still running.
func fixtureTrail(base time.Time) journey.Trail {
	return journey.Trail{
		Prompts: []journey.Prompt{
			{Text: "fix the 401 bug", At: base.Add(2 * time.Minute)},
		},
		Legs: []journey.Leg{
			{Class: journey.Scout, Label: "middleware.py", Start: base.Add(9 * time.Minute), End: base.Add(14 * time.Minute), Votes: 7,
				Files: []string{"middleware.py", "tokens.py"}},
			{Class: journey.Build, Label: "tokens.py", Start: base.Add(15 * time.Minute), End: base.Add(27 * time.Minute), Votes: 11,
				Files: []string{"tokens.py", "middleware.py"}},
			{Class: journey.Test, Label: "pytest", Start: base.Add(28 * time.Minute), End: base.Add(36 * time.Minute), Votes: 4},
			{Class: journey.Fix, Label: "tokens.py", Start: base.Add(37 * time.Minute), End: base.Add(39 * time.Minute), Votes: 5,
				Files: []string{"tokens.py"}, Current: true},
		},
		Branches: []journey.Branch{
			{ToolUseID: "toolu_1", Label: "scout the payments module", Start: base.Add(29 * time.Minute),
				End: base.Add(33 * time.Minute), Done: true, AfterLeg: 2},
		},
	}
}

// fixtureFrame is a captured pane: real ANSI colour, a line far wider than the
// mirror column, and the blank rows tmux pads a screen with.
const fixtureFrame = "\x1b[38;5;39m⏺ Read(src/auth/middleware.py)\x1b[0m\n" +
	"  ⎿ read 214 lines\n" +
	"\n" +
	"\x1b[31m✗ test_refresh_expired_token — AssertionError: expected 200, got 401, and it kept saying it\x1b[0m\n" +
	"\n" +
	"\x1b[38;5;213m✻\x1b[0m Churning… (23s · esc to interrupt)\n" +
	"\n\n\n"

// deckModel builds the M1 deck state the three deck goldens share: the api
// session selected, its trail loaded, and — unless panes is nil — its pane
// mapped and mirrored.
func deckModel(w, h int, panes map[string]tmuxop.Pane, frame string) *Model {
	m := New(nil)
	m.SetSize(w, h)
	m.SetSessions(fixtureSessions(fixtureBase), fixtureBase.Add(40*time.Minute))
	m.SetPanes(panes)
	m.selectIndex(1) // s-api — the session the SPEC mockup follows
	m.SetTrail(fixtureTrail(fixtureBase))
	m.SetMirror(frame)
	return m
}

// T16 — an 80x24 monochrome snapshot of the deck with a three-session fleet.
// M1 rewrote this frame twice over: the fleet's second line now names the tmux
// pane a session lives in, and the right-hand column is the trail rather than
// the M0 detail card. Regenerated deliberately.
func TestT16DeckViewGolden(t *testing.T) {
	forceASCII(t)

	m := New(nil)
	m.SetSize(80, 24)
	m.SetSessions(fixtureSessions(fixtureBase), fixtureBase.Add(40*time.Minute))

	compareGolden(t, "deck-80x24.txt", m.View())
}

// T31 — the trail renderer alone, at the sidecar's 38 columns: prompt, three
// closed legs, a returned subagent, and HEAD at the top. M2 widened the
// signature (a plan and a zoom level); with no todos at Lv1 the frame is the
// M1 frame, unchanged.
func TestT31RenderTrailGolden(t *testing.T) {
	forceASCII(t)

	got := RenderTrail(fixtureTrail(fixtureBase), TrailOpts{Todos: nil, Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: 20, Level: 1, Cursor: -1})
	compareGolden(t, "trail-38x20.txt", got)
}

// T32 — the whole deck at 120x30: fleet, live mirror, trail. The mirror's
// content is raw ANSI and one of its lines is wider than the column, which is
// the point: nothing may leak past the hairline into the trail.
func TestT32DeckThreeColumnGolden(t *testing.T) {
	forceASCII(t)

	m := deckModel(120, 30, map[string]tmuxop.Pane{
		"s-api": {Target: "dev:1.0", ID: "%5", PID: 4242, Path: "/home/user/api", Command: "claude"},
	}, fixtureFrame)

	got := m.View()
	compareGolden(t, "deck-120x30.txt", got)

	for _, line := range strings.Split(got, "\n") {
		if lipgloss.Width(line) > 120 {
			t.Errorf("line runs past the terminal (%d cols): %q", lipgloss.Width(line), line)
		}
	}
	if strings.Contains(got, "expected 200, got 401, and it kept saying it") {
		t.Error("the over-wide mirror line was not cropped to its column")
	}
}

// T33 — the same state at 80x24: too narrow for the mirror, so the deck falls
// back to fleet + trail.
func TestT33DeckNarrowGolden(t *testing.T) {
	forceASCII(t)

	m := deckModel(80, 24, map[string]tmuxop.Pane{
		"s-api": {Target: "dev:1.0", ID: "%5", PID: 4242, Path: "/home/user/api", Command: "claude"},
	}, fixtureFrame)

	got := m.View()
	compareGolden(t, "deck-80x24-narrow.txt", got)

	if strings.Contains(got, "· live") || strings.Contains(got, "Churning") {
		t.Error("the mirror should be hidden below 110 columns")
	}
	if !strings.Contains(got, "TRAIL · api") {
		t.Error("the trail column should hold the whole width the mirror gave up")
	}
}

// T34 — a session with no tmux pane: the mirror says where its content comes
// from instead, and shows the session's own words.
func TestT34MirrorNoPaneGolden(t *testing.T) {
	forceASCII(t)

	m := deckModel(120, 30, map[string]tmuxop.Pane{}, "")

	got := m.View()
	compareGolden(t, "mirror-nopane-120x30.txt", got)

	for _, want := range []string{
		"⌁ no pane · from transcript", // the header names the source
		`"fix the 401 bug"`,           // title
		"tool call in flight",         // reason
		"Bash: pytest tests/auth -x",  // activity
		"(no pane) · claude/auth-fx",  // and the fleet agrees
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no-pane mirror is missing %q", want)
		}
	}
	if strings.Contains(got, "· live") {
		t.Error("a session with no pane must not claim a live mirror")
	}
}
