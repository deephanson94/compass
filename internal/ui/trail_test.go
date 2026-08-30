package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/todo"
)

// fixtureLv2Trail is the journey the SPEC §2.3 Lv2 mockup draws: a test leg
// that parsed its own results, a fix leg that names its bugs and the files it
// touched, a subagent that came back with a finding, and HEAD still building.
func fixtureLv2Trail(base time.Time) journey.Trail {
	return journey.Trail{
		Prompts: []journey.Prompt{
			{Text: "fix the 401 bug", At: base.Add(2 * time.Minute)},
		},
		Legs: []journey.Leg{
			{Class: journey.Scout, Label: "middleware.py", Start: base.Add(9 * time.Minute), End: base.Add(14 * time.Minute), Votes: 7,
				Files: []string{"middleware.py", "tokens.py"}},
			{Class: journey.Test, Label: "pytest", Start: base.Add(15 * time.Minute), End: base.Add(27 * time.Minute), Votes: 6,
				Waypoints: []journey.Waypoint{
					{Kind: journey.WaypointTestRun, Text: "18 passed · 2 failed", At: base.Add(26 * time.Minute)},
					{Kind: journey.WaypointTestFail, Text: "test_refresh_expired_token", At: base.Add(26 * time.Minute)},
					{Kind: journey.WaypointTestFail, Text: "test_refresh_revoked_token", At: base.Add(26 * time.Minute)},
				}},
			{Class: journey.Fix, Label: "token refresh", Start: base.Add(28 * time.Minute), End: base.Add(36 * time.Minute), Votes: 11,
				Files: []string{"middleware.py", "tokens.py", "conftest.py"},
				Waypoints: []journey.Waypoint{
					{Kind: journey.WaypointBug, Text: "syntax error in tokens.py:88", At: base.Add(30 * time.Minute)},
					{Kind: journey.WaypointBug, Text: "expiry compared in localtime", At: base.Add(34 * time.Minute)},
				}},
			{Class: journey.Build, Label: "tokens.py", Start: base.Add(37 * time.Minute), End: base.Add(39 * time.Minute), Votes: 5,
				Files: []string{"tokens.py"}, Current: true},
		},
		Branches: []journey.Branch{
			{ToolUseID: "toolu_1", Label: "scout the payments module", Start: base.Add(16 * time.Minute),
				End: base.Add(21 * time.Minute), Done: true, AfterLeg: 1,
				Report: "payments never touches refresh"},
		},
	}
}

// T43 — the Lv2 trail at 38x24: every leg unfolded. The test leg shows its
// parsed counts and its two failures, the fix leg numbers its bugs and says
// what it touched, the returned subagent reports its finding, and HEAD — a
// build leg with one file and nothing extracted yet — stays a single line.
func TestT43TrailLv2Golden(t *testing.T) {
	forceASCII(t)

	got := RenderTrail(fixtureLv2Trail(fixtureBase), TrailOpts{Todos: nil, Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: 24, Level: 2, Cursor: -1})
	compareGolden(t, "trail-lv2-38x24.txt", got)

	if *update {
		return
	}
	for _, want := range []string{
		"│  ├ 18 passed · 2 failed", // TestRun is undecorated
		"│  ├ ✗ test_refresh_expired_token",
		"│  └ ✗ test_refresh_revoked_token", // the last waypoint closes the leg
		"│  ├ bug1 syntax error in tokens.py:88",
		"│  ├ bug2 expiry compared in localtime", // numbered per leg, in At order
		"│  └ touched middleware.py",             // the synthetic row, clipped to the column
		"│  └ payments never touches refresh",    // the subagent's report
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Lv2 frame is missing %q", want)
		}
	}

	// The same journey at Lv1 is the M1 graph: no waypoint carries into it.
	lv1 := RenderTrail(fixtureLv2Trail(fixtureBase), TrailOpts{Todos: nil, Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: 24, Level: 1, Cursor: -1})
	for _, gone := range []string{"18 passed", "bug1", "touched", "payments never"} {
		if strings.Contains(lv1, gone) {
			t.Errorf("Lv1 frame leaked the Lv2 detail %q", gone)
		}
	}
}

// T43b — the height budget: waypoints go before legs do, and HEAD's go last.
func TestT43TrailLv2HeightBudget(t *testing.T) {
	forceASCII(t)

	tr := fixtureLv2Trail(fixtureBase)
	// Give HEAD detail of its own, so there is something to protect.
	tr.Legs[3].Files = []string{"tokens.py", "middleware.py"}

	full := strings.Split(RenderTrail(tr, TrailOpts{Todos: nil, Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: 24, Level: 2, Cursor: -1}), "\n")
	rows := 0
	for _, l := range full {
		if strings.TrimSpace(l) != "" {
			rows++
		}
	}

	for h := rows; h >= 4; h-- {
		frame := RenderTrail(tr, TrailOpts{Todos: nil, Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: h, Level: 2, Cursor: -1})
		lines := strings.Split(frame, "\n")
		if len(lines) != h {
			t.Fatalf("height %d: frame is %d lines", h, len(lines))
		}
		if !strings.Contains(lines[0], "build") {
			t.Errorf("height %d: HEAD was cropped: %q", h, lines[0])
		}
		// HEAD's own detail outlives every older leg's.
		if strings.Contains(frame, "18 passed") && !strings.Contains(frame, "touched tokens.py") {
			t.Errorf("height %d: an older leg's waypoints outlived HEAD's", h)
		}
		// A trimmed group still closes with └, never a dangling ├.
		var prev string
		for _, l := range lines {
			if strings.HasPrefix(prev, "│  ├") && !strings.HasPrefix(l, "│  ") {
				t.Errorf("height %d: waypoint group left dangling after %q", h, prev)
			}
			prev = l
		}
	}
}

// T44 — ghost waypoints at Lv1: the plan above HEAD, first pending nearest the
// present, the rest stacking upward into the future on a dashed rail.
func TestT44TrailGhostsGolden(t *testing.T) {
	forceASCII(t)

	// in_progress and completed items are not drawn: HEAD and the legs already
	// tell that story.
	items := []todo.Item{
		{Text: "read middleware.py", Status: todo.Completed},
		{Text: "fix the token refresh", Status: todo.InProgress},
		{Text: "add a regression test", Status: todo.Pending},
		{Text: "update the changelog", Status: todo.Pending},
	}

	got := RenderTrail(fixtureTrail(fixtureBase), TrailOpts{Todos: items, Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: 20, Level: 1, Cursor: -1})
	compareGolden(t, "trail-ghosts-38x20.txt", got)

	if *update {
		return
	}
	first := strings.Index(got, "add a regression test")
	later := strings.Index(got, "update the changelog")
	head := strings.Index(got, "● fix")
	switch {
	case first < 0 || later < 0 || head < 0:
		t.Fatalf("ghosts or HEAD missing from the frame:\n%s", got)
	case !(later < first && first < head):
		t.Errorf("the plan is out of order: the first pending todo must sit nearest HEAD\n%s", got)
	}
	for _, gone := range []string{"read middleware.py", "fix the token refresh"} {
		if strings.Contains(got, gone) {
			t.Errorf("a non-pending todo was drawn as a ghost: %q", gone)
		}
	}

	// No pending work, no ghosts: the frame is exactly the M1 trail.
	done := []todo.Item{{Text: "read middleware.py", Status: todo.Completed}}
	plain := RenderTrail(fixtureTrail(fixtureBase), TrailOpts{Todos: done, Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: 20, Level: 1, Cursor: -1})
	if want := RenderTrail(fixtureTrail(fixtureBase), TrailOpts{Todos: nil, Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: 20, Level: 1, Cursor: -1}); plain != want {
		t.Errorf("a plan with nothing pending changed the frame\n--- got ---\n%s\n--- want ---\n%s", plain, want)
	}
}

// T44b — a long plan: four ghosts, and one dim row for the rest of it.
func TestT44TrailGhostsMoreGolden(t *testing.T) {
	forceASCII(t)

	var items []todo.Item
	for _, text := range []string{
		"add a regression test",
		"update the changelog",
		"rerun the auth suite",
		"drop the debug logging",
		"open the PR",
		"ask about the rate limit",
	} {
		items = append(items, todo.Item{Text: text, Status: todo.Pending})
	}

	got := RenderTrail(fixtureTrail(fixtureBase), TrailOpts{Todos: items, Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: 20, Level: 1, Cursor: -1})
	compareGolden(t, "trail-ghosts-more-38x20.txt", got)

	if *update {
		return
	}
	if !strings.Contains(got, "┊ +2 more") {
		t.Errorf("a plan of six should collapse its tail into one row:\n%s", got)
	}
	if n := strings.Count(got, glyphGhost); n != maxGhosts {
		t.Errorf("drew %d ghosts, want %d", n, maxGhosts)
	}
	for _, gone := range []string{"open the PR", "ask about the rate limit"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q should have collapsed into the +N row", gone)
		}
	}
}

// The plan may never crowd out the present: on a short panel the ghosts give
// way, and HEAD keeps the top row.
func TestTrailGhostsYieldToHead(t *testing.T) {
	forceASCII(t)

	var items []todo.Item
	for _, text := range []string{"one", "two", "three", "four"} {
		items = append(items, todo.Item{Text: text, Status: todo.Pending})
	}
	for h := 12; h >= 2; h-- {
		frame := RenderTrail(fixtureTrail(fixtureBase), TrailOpts{Todos: items, Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: h, Level: 1, Cursor: -1})
		lines := strings.Split(frame, "\n")
		ghosts := strings.Count(frame, glyphGhost)
		if ghosts*2 > h {
			t.Errorf("height %d: %d ghosts take more than half the panel", h, ghosts)
		}
		if !strings.Contains(frame, "● fix") {
			t.Errorf("height %d: the future pushed HEAD off the panel:\n%s", h, frame)
		}
		if len(lines) != h {
			t.Errorf("height %d: frame is %d lines", h, len(lines))
		}
	}
}
