package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

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

// T43b — the panel is a window, not a budget: at every height the document is
// the same document, the frame is a contiguous slice of it, and a pinned panel
// keeps HEAD and its detail on screen. Nothing is trimmed to make it fit —
// that is what the scrollbar's worth of rows above are for (M7 contract).
func TestT43TrailLv2Viewport(t *testing.T) {
	forceASCII(t)

	tr := fixtureLv2Trail(fixtureBase)
	// Give HEAD detail of its own, so there is something to protect.
	tr.Legs[3].Files = []string{"tokens.py", "middleware.py"}

	opts := func(h int) TrailOpts {
		return TrailOpts{Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: h, Level: 2, Cursor: -1, Pinned: true}
	}
	doc := TrailLines(tr, opts(24))

	for h := len(doc); h >= 4; h-- {
		frame := RenderTrail(tr, opts(h))
		lines := strings.Split(frame, "\n")
		if len(lines) != h {
			t.Fatalf("height %d: frame is %d lines", h, len(lines))
		}
		if got := TrailLines(tr, opts(h)); len(got) != len(doc) {
			t.Errorf("height %d: the document changed with the panel (%d rows, want %d)", h, len(got), len(doc))
		}
		// Pinned: the last screenful, so HEAD and its own detail are the rows
		// that never leave.
		if !strings.Contains(lines[h-1], "touched tokens.py") {
			t.Errorf("height %d: the last row is %q, want HEAD's detail", h, lines[h-1])
		}
		if !strings.Contains(frame, "● build") {
			t.Errorf("height %d: HEAD was cropped:\n%s", h, frame)
		}
		// A cropped group still closes with └ where the document says it does,
		// never a dangling ├ at the foot of the frame.
		if last := strings.TrimRight(lines[h-1], " "); strings.HasPrefix(last, "│  ├") {
			t.Errorf("height %d: waypoint group left dangling at %q", h, last)
		}
	}
}

// T44 — ghost waypoints at Lv1: the plan below HEAD, first pending nearest the
// present, the rest stacking downward into the future on a dashed rail.
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
	case !(head < first && first < later):
		t.Errorf("the plan is out of order: it runs downward from HEAD, first pending nearest\n%s", got)
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

// The plan may never crowd out the present: the ghosts hang below HEAD and a
// pinned panel shows the last screenful, so on a short panel the ghosts give
// way rather than pushing HEAD off the top.
func TestTrailGhostsYieldToHead(t *testing.T) {
	forceASCII(t)

	var items []todo.Item
	for _, text := range []string{"one", "two", "three", "four"} {
		items = append(items, todo.Item{Text: text, Status: todo.Pending})
	}
	for h := 12; h >= 2; h-- {
		frame := RenderTrail(fixtureTrail(fixtureBase), TrailOpts{Todos: items, Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: h, Level: 1, Cursor: -1, Pinned: true})
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

// longTrail is a journey nobody's panel can hold: one prompt, n legs in the
// order they happened, a subagent forked off the middle one, and HEAD at the
// end. It is what the scrolling contract is about — before M7 the older half of
// this simply was not drawn.
func longTrail(n int) journey.Trail {
	classes := []journey.Class{journey.Scout, journey.Build, journey.Test, journey.Fix}
	tr := journey.Trail{
		Prompts: []journey.Prompt{{Text: "make the whole thing faster", At: fixtureBase}},
		Branches: []journey.Branch{{
			ToolUseID: "toolu_mid", Label: "read the profiler output", AfterLeg: n / 2,
			Start: fixtureBase.Add(time.Duration(n/2) * time.Minute), Done: true,
		}},
	}
	for i := 0; i < n; i++ {
		tr.Legs = append(tr.Legs, journey.Leg{
			Class:   classes[i%len(classes)],
			Label:   fmt.Sprintf("step %d", i),
			Start:   fixtureBase.Add(time.Duration(i+1) * time.Minute),
			End:     fixtureBase.Add(time.Duration(i+2) * time.Minute),
			Current: i == n-1,
		})
	}
	return tr
}

// grow closes HEAD and opens a newer leg after it — the trail growing under a
// panel that is already looking at it.
func grow(tr journey.Trail, label string) journey.Trail {
	out := tr
	out.Legs = append(append([]journey.Leg(nil), tr.Legs...), journey.Leg{
		Class: journey.Ship, Label: label, Current: true,
		Start: tr.Legs[len(tr.Legs)-1].Start.Add(time.Minute),
	})
	out.Legs[len(out.Legs)-2].Current = false
	return out
}

// trailAt finds the first document row containing sub, or -1.
func trailAt(doc []string, sub string) int {
	for i, l := range doc {
		if strings.Contains(l, sub) {
			return i
		}
	}
	return -1
}

// T69 — the document runs the way the conversation does: the opening prompt at
// the top, the legs in the order they happened, the fork directly under the leg
// it left from, HEAD last of what has happened, and the plan below it.
func TestT69TrailLinesOrder(t *testing.T) {
	forceASCII(t)

	items := []todo.Item{
		{Text: "add a regression test", Status: todo.Pending},
		{Text: "update the changelog", Status: todo.Pending},
	}
	o := TrailOpts{Todos: items, Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: 20, Level: 1, Cursor: -1, Pinned: true}
	doc := TrailLines(fixtureTrail(fixtureBase), o)
	if len(doc) == 0 {
		t.Fatal("TrailLines returned nothing for a populated trail")
	}

	// Every row of the frame comes out of the document, in the document's order.
	if frame := strings.Split(RenderTrail(fixtureTrail(fixtureBase), o), "\n"); !strings.HasPrefix(strings.Join(frame, "\n"), strings.Join(doc, "\n")) {
		t.Errorf("the frame is not the head of the document:\n%s", strings.Join(frame, "\n"))
	}

	prompt := trailAt(doc, `"fix the 401 bug"`)
	scout := trailAt(doc, "◆ scout")
	build := trailAt(doc, "◆ build")
	test := trailAt(doc, "◆ test")
	fork := trailAt(doc, railFork)
	head := trailAt(doc, "● fix")
	next := trailAt(doc, "add a regression test")
	later := trailAt(doc, "update the changelog")

	for _, step := range []struct {
		name  string
		a, bb int
	}{
		{"prompt before the first leg", prompt, scout},
		{"legs in the order they happened", scout, build},
		{"legs in the order they happened", build, test},
		{"HEAD last of what has happened", test, head},
		{"the plan below HEAD", head, next},
		{"the plan in the order it will happen", next, later},
	} {
		if step.a < 0 || step.bb < 0 || step.a >= step.bb {
			t.Errorf("%s: rows %d and %d are out of order\n%s", step.name, step.a, step.bb, strings.Join(doc, "\n"))
		}
	}
	if prompt != 0 {
		t.Errorf("the journey opens on row %d, want the very top", prompt)
	}
	if !strings.Contains(doc[1], railHead) {
		t.Errorf("the rail's cap does not mark the start: row 1 is %q", doc[1])
	}
	// The subagent forked off the test leg, so it hangs directly under it.
	if fork != test+1 {
		t.Errorf("the branch sits on row %d, want %d — directly under the leg it forked from", fork, test+1)
	}
	// Nothing selectable is below the plan: the ghosts are the last word.
	if last := trailAt(doc, glyphGhost); last < head {
		t.Errorf("a ghost was drawn above HEAD (row %d, HEAD is %d)", last, head)
	}
}

// T70 — the 38x20 golden, reversed: the SPEC §2.1 mockup upside down. The
// prompt opens the panel, HEAD closes what has happened, and the rail's cap
// marks the start rather than the end.
func TestT70TrailReversedGolden(t *testing.T) {
	forceASCII(t)

	got := RenderTrail(fixtureTrail(fixtureBase), TrailOpts{Todos: nil, Now: fixtureBase.Add(40 * time.Minute), Width: 38, Height: 20, Level: 1, Cursor: -1})
	compareGolden(t, "trail-38x20.txt", got)
	if *update {
		return
	}

	var rows []string
	for _, l := range strings.Split(got, "\n") {
		if strings.TrimSpace(l) != "" {
			rows = append(rows, l)
		}
	}
	if len(rows) < 3 {
		t.Fatalf("the frame is only %d rows:\n%s", len(rows), got)
	}
	if !strings.Contains(rows[0], glyphPrompt) {
		t.Errorf("the top row is %q, want the opening prompt", rows[0])
	}
	if !strings.Contains(rows[len(rows)-1], glyphHead) {
		t.Errorf("the bottom row is %q, want HEAD", rows[len(rows)-1])
	}
	if strings.Contains(got, "╵") {
		t.Error("the rail still ends with the old upward cap")
	}
}

// T71 — pinned: the last screenful, whatever Scroll says, and it stays the last
// screenful as the trail grows.
func TestT71PinnedViewport(t *testing.T) {
	forceASCII(t)

	tr := longTrail(40)
	o := TrailOpts{Now: fixtureBase.Add(time.Hour), Width: 38, Height: 12, Level: 1, Cursor: -1, Pinned: true, Scroll: 3}
	doc := TrailLines(tr, o)
	if len(doc) <= o.Height {
		t.Fatalf("the fixture is only %d rows; it has to overflow the panel", len(doc))
	}

	frame := strings.Split(RenderTrail(tr, o), "\n")
	want := doc[len(doc)-o.Height:]
	for i := range want {
		if frame[i] != want[i] {
			t.Fatalf("pinned row %d is %q, want %q (Scroll must be ignored)", i, frame[i], want[i])
		}
	}
	if !strings.Contains(frame[len(frame)-1], "step 39") {
		t.Errorf("the last row is %q, want the newest leg", frame[len(frame)-1])
	}

	// A leg lands. Pinned means the panel follows it without a keypress.
	after := strings.Split(RenderTrail(grow(tr, "tag v2"), o), "\n")
	if !strings.Contains(after[len(after)-1], "tag v2") {
		t.Errorf("the trail grew and the panel did not follow: last row is %q", after[len(after)-1])
	}
}

// T72 — unpinned: the view holds still while the trail grows underneath it, and
// the offset of the last screenful is the pinned view again — which is what the
// panel re-pins to when it scrolls back down.
func TestT72ScrollHoldsAndRepins(t *testing.T) {
	forceASCII(t)

	tr := longTrail(40)
	o := TrailOpts{Now: fixtureBase.Add(time.Hour), Width: 38, Height: 12, Level: 1, Cursor: -1, Scroll: 6}

	before := RenderTrail(tr, o)
	if after := RenderTrail(grow(tr, "tag v2"), o); after != before {
		t.Errorf("the view jumped when the trail grew\n--- before ---\n%s\n--- after ---\n%s", before, after)
	}
	// Reading upward is reading backwards in time now.
	up := RenderTrail(tr, TrailOpts{Now: o.Now, Width: 38, Height: 12, Level: 1, Cursor: -1, Scroll: 0})
	if !strings.Contains(up, `"make the whole thing faster"`) {
		t.Errorf("scrolling to the top does not reach the opening prompt:\n%s", up)
	}

	// Back at the bottom: the same frame as pinned, so the panel can re-pin
	// there and nothing moves.
	doc := TrailLines(tr, o)
	bottom := o
	bottom.Scroll = len(doc) - o.Height
	pinned := o
	pinned.Pinned = true
	if got, want := RenderTrail(tr, bottom), RenderTrail(tr, pinned); got != want {
		t.Errorf("the last screenful is not the pinned view\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	// And an offset past the end is held there rather than running off it.
	past := o
	past.Scroll = len(doc) * 2
	if got, want := RenderTrail(tr, past), RenderTrail(tr, pinned); got != want {
		t.Error("a scroll past the end was not clamped to the last screenful")
	}
}

// T73 — nothing is dropped: every row of a 200-row trail is reachable, and each
// offset draws exactly the slice of the document it names.
func TestT73NothingIsDropped(t *testing.T) {
	forceASCII(t)

	tr := longTrail(100)
	o := TrailOpts{Now: fixtureBase.Add(3 * time.Hour), Width: 38, Height: 20, Level: 1, Cursor: -1}
	doc := TrailLines(tr, o)
	if len(doc) < 200 {
		t.Fatalf("the fixture is %d rows, want at least 200", len(doc))
	}

	seen := make([]bool, len(doc))
	for top := 0; top <= len(doc)-o.Height; top++ {
		at := o
		at.Scroll = top
		frame := strings.Split(RenderTrail(tr, at), "\n")
		for i := 0; i < o.Height; i++ {
			if frame[i] != doc[top+i] {
				t.Fatalf("offset %d row %d is %q, want %q", top, i, frame[i], doc[top+i])
			}
			seen[top+i] = true
		}
	}
	for i, ok := range seen {
		if !ok {
			t.Fatalf("row %d (%q) can never be scrolled to", i, doc[i])
		}
	}
	// The oldest leg and the newest are both in there, whole.
	if trailAt(doc, "step 0") < 0 || trailAt(doc, "step 99") < 0 {
		t.Error("the document lost an end of the journey")
	}
}

// T74 — the renderer's half of "keep the cursor visible": every selectable row
// has one document row, TrailCursorRow finds it, and a viewport scrolled by
// that number shows it — and only ever moves as far as the row demands. The
// keys that do the moving are the app's (Model.keepCursorVisible), and landing
// on the last row re-pins there.
func TestT74TrailCursorRow(t *testing.T) {
	forceASCII(t)

	tr := longTrail(40)
	rows := TrailRows(tr, levelWaypoints)
	o := TrailOpts{Now: fixtureBase.Add(time.Hour), Width: 38, Height: 12, Level: levelWaypoints, Cursor: -1}
	doc := TrailLines(tr, o)

	if TrailCursorRow(tr, o) != -1 {
		t.Error("no cursor should name no row")
	}
	lv1 := o
	lv1.Level, lv1.Cursor = levelTrail, 0
	if TrailCursorRow(tr, lv1) != -1 {
		t.Error("Lv1 has no cursor, so it names no row either")
	}

	prev := -1
	for c, row := range rows {
		at := o
		at.Cursor = c
		line := TrailCursorRow(tr, at)
		if line < 0 || line >= len(doc) {
			t.Fatalf("cursor %d (%s %q) has no row", c, row.Kind, row.Text)
		}
		if line <= prev {
			t.Errorf("cursor %d lands on row %d, above the previous cursor's %d", c, line, prev)
		}
		prev = line

		// The smallest move that brings it on screen, from wherever we were.
		for _, from := range []int{0, line, len(doc)} {
			view := at
			view.Scroll = from
			switch {
			case line < clampScroll(from, len(doc), o.Height):
				view.Scroll = line
			case line >= clampScroll(from, len(doc), o.Height)+o.Height:
				view.Scroll = line - o.Height + 1
			}
			frame := strings.Split(RenderTrail(tr, view), "\n")
			shown := clampScroll(view.Scroll, len(doc), o.Height)
			if line < shown || line >= shown+o.Height {
				t.Fatalf("cursor %d: row %d is outside the viewport at %d", c, line, shown)
			}
			if !strings.Contains(frame[line-shown], strings.TrimSpace(ansi.Strip(row.Text))) &&
				row.Kind != "branch" {
				t.Errorf("cursor %d: row %d of the frame is %q, want %q", c, line-shown, frame[line-shown], row.Text)
			}
		}
	}

	// The last selectable row is the newest thing that happened; the pinned
	// panel is already showing it, so landing there needs no scroll at all.
	last := o
	last.Cursor, last.Pinned = len(rows)-1, true
	line := TrailCursorRow(tr, last)
	if top := len(doc) - o.Height; line < top || line >= top+o.Height {
		t.Errorf("the newest row (%d) is not in the pinned screenful [%d,%d)", line, top, top+o.Height)
	}
}
