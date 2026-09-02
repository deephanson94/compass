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

// sessionKey is the identity a fixture session is filed under: its transcript
// path, which is what SessionInfo.Key() returns (M6 contract). Panes, feeds,
// narration and the selection are all keyed by it — never by the id, which two
// sessions may share.
func sessionKey(id string) string { return "/x/" + id + ".jsonl" }

// fixtureSessions is the three-session fleet the deck goldens share: one
// waiting on the human, one working, one idle. All three are live and none of
// them is in a tmux session compass can see, so the M5 live view renders them
// as the degenerate one-group list the M1–M3 goldens were drawn from.
func fixtureSessions(base time.Time) []fleet.Session {
	return []fleet.Session{
		{
			Info: fleet.SessionInfo{
				ID: "s-infra", TranscriptPath: sessionKey("s-infra"), ProjectSlug: "-home-user-infra",
				CWD: "/home/user/infra", GitBranch: "tf/vpc",
				Title: "tighten the vpc security groups", StartedAt: base, LastEventAt: base.Add(38 * time.Minute),
			},
			Snap:  state.Snapshot{State: state.NeedsYou, Since: base.Add(38 * time.Minute), Reason: "waiting on your answer", Activity: "AskUserQuestion"},
			Live:  true,
			Class: journey.Design, HasClass: true,
		},
		{
			Info: fleet.SessionInfo{
				ID: "s-api", TranscriptPath: sessionKey("s-api"), ProjectSlug: "-home-user-api",
				CWD: "/home/user/api", GitBranch: "claude/auth-fx",
				Title: "fix the 401 bug", StartedAt: base, LastEventAt: base.Add(39 * time.Minute),
			},
			Snap:  state.Snapshot{State: state.Working, Since: base.Add(37 * time.Minute), Reason: "tool call in flight", Activity: "Bash: pytest tests/auth -x"},
			Live:  true,
			Class: journey.Test, HasClass: true, Outcome: "1216✓ 2✗",
		},
		{
			Info: fleet.SessionInfo{
				ID: "s-docs", TranscriptPath: sessionKey("s-docs"), ProjectSlug: "-home-user-docs",
				CWD: "/home/user/docs", GitBranch: "main",
				Title: "update the readme", StartedAt: base, LastEventAt: base.Add(18 * time.Minute),
			},
			Snap:  state.Snapshot{State: state.Idle, Since: base.Add(18 * time.Minute), Reason: "turn complete", Activity: "idle"},
			Live:  true,
			Class: journey.Docs, HasClass: true, Outcome: "46✓",
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
			// A real test leg always comes back with a run summary. This fixture
			// had none, which is why no golden noticed Lv1 was dropping it.
			{Class: journey.Test, Label: "pytest", Start: base.Add(28 * time.Minute), End: base.Add(36 * time.Minute), Votes: 4,
				Waypoints: []journey.Waypoint{
					{Kind: journey.WaypointTestRun, Text: "18 passed · 2 failed", Short: "18✓ 2✗", At: base.Add(35 * time.Minute)},
					{Kind: journey.WaypointTestFail, Text: "test_refresh_expired_token", At: base.Add(35 * time.Minute)},
				}},
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
	m.point(sessionKey("s-api")) // the session the SPEC mockup follows
	m.SetTrail(fixtureTrail(fixtureBase))
	// A real session always has a transcript behind it, pane or no pane — the
	// no-pane golden asserted an empty middle panel only because the fixture
	// had nothing to put in it.
	m.SetEvents(followEvents(fixtureBase))
	m.SetMirror(frame)
	openTrail(m)
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

	got := m.View()
	compareGolden(t, "deck-80x24.txt", got)
	if *update {
		return
	}

	// The footer is clipped to the deck's inner width, so a keymap that
	// overflowed would silently lose its tail rather than wrap. Eighty columns
	// is the floor, and the attach hint is the longest thing it carries: both
	// keymaps have to survive the clip whole.
	for _, want := range []string{
		"j/k move · enter attach (prefix d returns) · g grab · ? help · q quit",
		"j/k move · enter attach (prefix d returns) · A live fleet · ? help · q quit",
	} {
		m.archiveView = strings.Contains(want, "A live fleet")
		if frame := m.View(); !strings.Contains(frame, want) {
			t.Errorf("the footer does not fit an 80-column deck: %q", want)
		}
	}
	m.archiveView = false
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

// T32 — the whole deck at 120x30: fleet and trail, and no middle panel. The
// mirror is off by default (decision #15): the CLI it would show is one Enter
// away, and the trail is where the columns were being truncated.
func TestT32DeckTwoColumnGolden(t *testing.T) {
	forceASCII(t)

	m := deckModel(120, 30, map[string]tmuxop.Pane{
		sessionKey("s-api"): {Target: "dev:1.0", ID: "%5", PID: 4242, Command: "claude", Window: "auth-fix"},
	}, fixtureFrame)

	got := m.View()
	compareGolden(t, "deck-120x30.txt", got)

	if strings.Contains(got, mirrorMark+" dev:1.0 · live") || strings.Contains(got, "Churning") {
		t.Error("the mirror is on screen without being asked for")
	}
	// The trail has the middle's columns: the widest fixture label used to be
	// clipped at 38 and is whole here.
	if !strings.Contains(got, "scout the payments module") {
		t.Errorf("the trail did not get the width the mirror gave up:\n%s", got)
	}
}

// T32b — `m` brings the mirror back: fleet, live mirror, trail. The mirror's
// content is raw ANSI and one of its lines is wider than the column, which is
// the point: nothing may leak past the hairline into the trail.
func TestT32bMirrorToggledOnGolden(t *testing.T) {
	forceASCII(t)

	m := deckModel(120, 30, map[string]tmuxop.Pane{
		sessionKey("s-api"): {Target: "dev:1.0", ID: "%5", PID: 4242, Command: "claude", Window: "auth-fix"},
	}, fixtureFrame)
	press(m, "m")

	got := m.View()
	compareGolden(t, "deck-120x30-mirror.txt", got)

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
		sessionKey("s-api"): {Target: "dev:1.0", ID: "%5", PID: 4242, Command: "claude", Window: "auth-fix"},
	}, fixtureFrame)

	got := m.View()
	compareGolden(t, "deck-80x24-narrow.txt", got)

	// "· live" alone is now the fleet's own title; the mirror's claim is the
	// pane target beside the mirror mark.
	if strings.Contains(got, "dev:1.0 · live") || strings.Contains(got, "Churning") {
		t.Error("the mirror should be hidden below 110 columns")
	}
	if !strings.Contains(got, "TRAIL · api") {
		t.Error("the trail column should hold the whole width the mirror gave up")
	}
}

// T34 — a session with no tmux pane. The mirror says where its content comes
// from, and then shows that content: the session's own conversation, which is
// the same thing the pane would have been rendering.
//
// It used to show three dim facts the fleet column already carries, resting on
// the floor of a panel forty rows tall. This golden asserted that was correct
// because the fixture had no transcript to draw instead — on a real terminal
// it reads as an empty screen, which is how it was reported.
func TestT34MirrorNoPaneGolden(t *testing.T) {
	forceASCII(t)

	m := deckModel(120, 30, map[string]tmuxop.Pane{}, "")
	press(m, "m")

	got := m.View()
	compareGolden(t, "mirror-nopane-120x30.txt", got)

	for _, want := range []string{
		"⌁ no pane · from transcript", // the header names the source
		"moment30",                    // and the newest turn is on screen
		"◆ test   1216✓ 2✗",           // and says how its last run went
	} {
		if !strings.Contains(got, want) {
			t.Errorf("no-pane mirror is missing %q", want)
		}
	}
	if strings.Contains(got, mirrorMark+" dev:1.0 · live") {
		t.Error("a session with no pane must not claim a live mirror")
	}
	// With no pane anywhere the group headers would all read "elsewhere", which
	// is noise: the mirror's own header says it once, and says it better.
	if strings.Contains(got, "elsewhere") {
		t.Error("a fleet where nothing has a pane should not label every row")
	}
	// The panel is full, not three lines on the floor of an empty column.
	mid := middleColumn(got)
	if drawn := countNonBlank(mid); drawn < 10 {
		t.Errorf("the middle panel drew %d lines of %d; it reads as empty:\n%s",
			drawn, len(mid), strings.Join(mid, "\n"))
	}
}

// Before a transcript has been read there is nothing to mirror, and the panel
// says who the session is rather than going blank.
func TestMirrorWithoutATranscriptStillSaysWho(t *testing.T) {
	forceASCII(t)

	m := deckModel(120, 30, map[string]tmuxop.Pane{}, "")
	m.SetEvents(nil)
	press(m, "m")

	got := m.View()
	for _, want := range []string{
		"⌁ no pane · from transcript",
		`"fix the 401 bug"`,          // title
		"tool call in flight",        // reason
		"Bash: pytest tests/auth -x", // activity
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the fallback is missing %q", want)
		}
	}
}

// middleColumn pulls the mirror's column out of a rendered deck frame.
func middleColumn(frame string) []string {
	var out []string
	for _, line := range strings.Split(frame, "\n") {
		parts := strings.Split(line, "│")
		if len(parts) < 3 {
			continue
		}
		out = append(out, parts[1])
	}
	return out
}

func countNonBlank(lines []string) int {
	n := 0
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

// The help legend has to name every class the segmenter can actually produce.
// Lv1's whole job is the classification, and until now `?` said only "◆ leg" —
// there was nowhere to look up what the seven tints meant, which is how it was
// reported ("I don't remember all glyphs meaning").
//
// The loop walks the real enum rather than a copy of it, so adding a class
// fails here until the legend learns about it.
func TestHelpNamesEveryLegClass(t *testing.T) {
	forceASCII(t)

	got := strings.Join(helpLines(60, 40), "\n")

	var classes []journey.Class
	for c := journey.Class(0); ; c++ {
		if c.String() == "unknown" {
			break
		}
		classes = append(classes, c)
	}
	if len(classes) < 7 {
		t.Fatalf("only %d classes found; the enum walk is wrong", len(classes))
	}
	for _, c := range classes {
		if !strings.Contains(got, c.String()) {
			t.Errorf("help never names the %q class:\n%s", c, got)
		}
	}
	if len(legClasses) != len(classes) {
		t.Errorf("the legend lists %d classes, the segmenter produces %d",
			len(legClasses), len(classes))
	}
	// And it says how many there are, so the prose cannot drift from the list.
	if !strings.Contains(got, "seven classes") {
		t.Errorf("help does not say how many classes there are:\n%s", got)
	}
	if len(classes) != 7 {
		t.Errorf("there are now %d classes; the help text still says seven", len(classes))
	}
}

// The help overlay has to fit the body it is drawn into, and the keys are the
// half that must never be the part cut. Adding the class legend is what made
// this reachable — it pushed the top of the key list off a 26-row terminal.
func TestHelpFitsAndKeepsItsKeys(t *testing.T) {
	forceASCII(t)

	for _, tc := range []struct {
		w, h         int
		wantLegend   bool
		wantEveryKey bool
	}{
		{120, 22, true, true},   // two columns, everything
		{76, 24, true, true},    // one column, everything
		{120, 12, false, false}, // too short for the keys: they get the whole width
		{60, 8, false, false},   // barely anything
	} {
		lines := helpLines(tc.w, tc.h)
		if len(lines) > tc.h {
			t.Errorf("%dx%d: help drew %d lines into %d", tc.w, tc.h, len(lines), tc.h)
		}
		got := strings.Join(lines, "\n")

		// The keys always start first, whatever else is dropped.
		if !strings.Contains(got, "select a session") {
			t.Errorf("%dx%d: the keys were cut before the legend was:\n%s", tc.w, tc.h, got)
		}
		if tc.wantEveryKey && !strings.Contains(got, "quit") {
			t.Errorf("%dx%d: the key list is incomplete:\n%s", tc.w, tc.h, got)
		}
		if tc.wantLegend && !strings.Contains(got, "scout") {
			t.Errorf("%dx%d: the class legend is missing:\n%s", tc.w, tc.h, got)
		}
		// A body too short for the keys must spend its width on them, not on a
		// legend it has no room to finish.
		if !tc.wantLegend && strings.Contains(got, "one of seven classes") {
			t.Errorf("%dx%d: the legend crowded out the keys:\n%s", tc.w, tc.h, got)
		}
	}
}

// Tab moves the keys from one panel to another, and the deck has to say which
// one has them. Lv2 and Lv3 draw an identical trail — Lv3 is not a different
// view of the journey, it is the same view with the focus moved off it — so
// without a marker, zooming in looked like nothing had happened.
func TestFocusMarkerFollowsTheKeys(t *testing.T) {
	forceASCII(t)

	m := deckModel(120, 20, map[string]tmuxop.Pane{
		sessionKey("s-api"): {Target: "dev:1.0", ID: "%5", PID: 4242, Command: "claude", Window: "auth-fix"},
	}, fixtureFrame)

	for _, want := range []struct {
		level  int
		marked string
	}{
		{1, "FLEET · live"}, // j/k walk the fleet
		{2, "TRAIL · api"},  // …the trail's rows
		{3, "READER · api"}, // …and then the conversation
	} {
		for m.level < want.level {
			pressTab(m)
		}
		if m.level != want.level {
			t.Fatalf("could not reach Lv%d (at Lv%d)", want.level, m.level)
		}
		got := m.View()
		if !strings.Contains(got, focusMark+want.marked) {
			t.Errorf("Lv%d: %q is not marked as focused:\n%s", want.level, want.marked, got)
		}
		// Exactly one panel holds the keys.
		if n := strings.Count(got, focusMark); n != 1 {
			t.Errorf("Lv%d: %d panels marked, want exactly 1:\n%s", want.level, n, got)
		}
	}

	// Zooming back out hands them back.
	for m.level > levelTrail {
		m.zoomOut()
	}
	if got := m.View(); !strings.Contains(got, focusMark+"FLEET · live") {
		t.Errorf("shift+tab did not return the keys to the fleet:\n%s", got)
	}
}

// The panel marker must not be the fleet's row marker: at Lv1 both sit in the
// same column of the same panel, and one names a session while the other names
// a panel.
func TestFocusMarkerIsNotTheSelectionMarker(t *testing.T) {
	if focusMark == "▸" {
		t.Fatal("the focus marker is the fleet's selection marker")
	}
	forceASCII(t)
	m := deckModel(120, 20, map[string]tmuxop.Pane{}, "")
	got := m.View()
	if !strings.Contains(got, focusMark+"FLEET") {
		t.Fatalf("the fleet is not marked at Lv1:\n%s", got)
	}
	// The selected session still carries its own marker, unchanged.
	if !strings.Contains(got, "▸2 ● api") {
		t.Errorf("the selected row lost its marker:\n%s", got)
	}
}

// The mirror's one running cost was five capture-pane calls a second. With
// the mirror off screen there is nothing to capture into, so nothing is
// asked of tmux: not at Lv1 by default, not at Lv2 or Lv3 with it on, and
// not on a terminal too narrow to draw it.
func TestNoCaptureWhileTheMirrorIsOffScreen(t *testing.T) {
	forceASCII(t)
	pane := map[string]tmuxop.Pane{
		sessionKey("s-api"): {Target: "dev:1.0", ID: "%5", PID: 4242, Command: "claude", Window: "auth-fix"},
	}

	m := deckModel(120, 30, pane, fixtureFrame)
	if m.capture() != nil {
		t.Error("Lv1 with the mirror off still polls the pane")
	}
	press(m, "m")
	if m.capture() == nil {
		t.Fatal("m opened the mirror but nothing polls the pane")
	}
	pressTab(m) // Lv2: the mirror is a Lv1 panel
	if m.capture() != nil {
		t.Error("Lv2 polls the pane with no mirror on screen")
	}
	pressTab(m) // Lv3
	if m.capture() != nil {
		t.Error("Lv3 polls the pane with no mirror on screen")
	}

	narrow := deckModel(100, 30, pane, fixtureFrame)
	press(narrow, "m")
	if narrow.capture() != nil {
		t.Error("a terminal too narrow for the mirror still polls the pane")
	}
	if narrow.note == "" {
		t.Error("m on a narrow terminal said nothing about why nothing happened")
	}
}
