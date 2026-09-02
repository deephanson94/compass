package ui

import (
	"fmt"
	"github.com/deephanson94/compass/internal/narrator"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

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
					{Kind: journey.WaypointTestRun, Text: "18 passed · 2 failed", Short: "18✓ 2✗", At: base.Add(26 * time.Minute)},
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
	// A returned subagent's report is on both levels now — a ✓ that kept its
	// finding two keypresses down created an obligation without discharging
	// it — so only the leg waypoints and the touched row are Lv2's.
	for _, gone := range []string{"18 passed", "bug1", "touched"} {
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

	// Completed items are not drawn: the legs already tell that story. The
	// in-progress one is not a ghost either — it is HEAD's label, the plan's
	// own words for what the session is doing now.
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
	if strings.Contains(got, "read middleware.py") {
		t.Error("a completed todo was drawn as a ghost")
	}
	for _, line := range strings.Split(got, "\n") {
		if !strings.Contains(line, "fix the token refresh") {
			continue
		}
		if strings.Contains(line, glyphGhost) {
			t.Errorf("the in-progress todo was drawn as a ghost: %q", line)
		}
		if !strings.Contains(line, "● fix") {
			t.Errorf("the in-progress todo is not HEAD's label: %q", line)
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
			Class: classes[i%len(classes)],
			Label: fmt.Sprintf("step %d", i),
			// A leg that touched nothing and produced nothing is a tick on
			// the rail, not a row; this fixture wants rows.
			Files:   []string{fmt.Sprintf("step%d.go", i)},
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

// The Lv2 cursor is a bar the width of the panel, not a highlight cut to each
// row's own text. Trail rows run from three characters to thirty — a ragged
// cursor is debris to follow down a column, a full-width one is a place.
func TestLv2CursorBarSpansThePanel(t *testing.T) {
	forceASCII(t)

	tr := fixtureLv2Trail(fixtureBase)
	rows := TrailRows(tr, levelWaypoints)
	if len(rows) < 4 {
		t.Fatalf("the fixture has only %d selectable rows; too thin to judge", len(rows))
	}

	// The ASCII profile drops the inversion itself, so what is measured here is
	// the shape underneath it: the row the cursor stands on, padded out to the
	// panel. Every other row keeps its own length.
	const width = 34
	widest := 0 // the most padding any row needed, so a no-op cannot pass
	for i := range rows {
		o := TrailOpts{
			Now: fixtureBase.Add(40 * time.Minute), Width: width, Height: 200,
			Level: levelWaypoints, Cursor: i,
		}
		doc := TrailLines(tr, o)
		at := TrailCursorRow(tr, o)
		if at < 0 {
			t.Fatalf("row %d (%s %q) has no row in the document", i, rows[i].Kind, rows[i].Text)
		}
		if got := lipgloss.Width(doc[at]); got != width {
			t.Errorf("row %d (%s %q) draws a %d-column bar, want the panel's %d: %q",
				i, rows[i].Kind, rows[i].Text, got, width, doc[at])
		}
		// The same row drawn without a cursor is its natural length; the gap
		// between the two is the padding this row actually needed.
		plain := TrailOpts{
			Now: fixtureBase.Add(40 * time.Minute), Width: width, Height: 200,
			Level: levelWaypoints, Cursor: -1,
		}
		if pad := width - lipgloss.Width(TrailLines(tr, plain)[at]); pad > widest {
			widest = pad
		}
	}
	if widest < 4 {
		t.Errorf("every row already filled the panel (widest gap %d columns); "+
			"the padding was never exercised", widest)
	}
}

// And the bar is really an inversion when the terminal has one to give: the
// goldens are drawn in ASCII, which drops the attribute, so nothing else in
// this package would notice the cursor losing its mark entirely (SPEC §4).
func TestLv2CursorIsInverted(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	tr := fixtureLv2Trail(fixtureBase)
	o := TrailOpts{
		Now: fixtureBase.Add(40 * time.Minute), Width: 34, Height: 200,
		Level: levelWaypoints, Cursor: 0,
	}
	doc := TrailLines(tr, o)
	at := TrailCursorRow(tr, o)
	if !strings.Contains(doc[at], "\x1b[7m") {
		t.Errorf("the cursor row carries no inversion: %q", doc[at])
	}
	for i, line := range doc {
		if i != at && strings.Contains(line, "\x1b[7m") {
			t.Errorf("row %d is inverted too; only the cursor's row may be: %q", i, ansi.Strip(line))
		}
	}
}

// A leg that produced nothing countable — no result, no files, no narrated
// phrase — is a tick on the rail, not a row. Three "test  pytest" rows with no
// counts said nothing three times on a dogfooded trail; as ticks they still
// show that tests ran between the legs that did something, in the row the
// rail would have taken anyway.
func TestALegWithNothingToSayIsATickOnTheRail(t *testing.T) {
	forceASCII(t)
	base := fixtureBase
	tr := journey.Trail{
		Prompts: []journey.Prompt{{Text: "fix the 401 bug", At: base}},
		Legs: []journey.Leg{
			{Class: journey.Scout, Label: "middleware.py", Start: base.Add(1 * time.Minute), End: base.Add(4 * time.Minute), Files: []string{"middleware.py"}},
			{Class: journey.Test, Label: "pytest", Start: base.Add(5 * time.Minute), End: base.Add(6 * time.Minute)},
			{Class: journey.Build, Label: "tokens.py", Start: base.Add(7 * time.Minute), End: base.Add(15 * time.Minute), Files: []string{"tokens.py"}},
			{Class: journey.Fix, Label: "tokens.py", Start: base.Add(16 * time.Minute), Files: []string{"tokens.py"}, Current: true},
		},
	}
	o := TrailOpts{Now: base.Add(20 * time.Minute), Width: 38, Height: 20, Level: 1, Cursor: -1}
	doc := TrailLines(tr, o)

	// prompt, cap, scout, tick, build, rail, fix — the tick is one row where
	// a rail and a leg row used to be two.
	if len(doc) != 7 {
		t.Fatalf("the trail is %d rows, want 7:\n%s", len(doc), strings.Join(doc, "\n"))
	}
	// The tick carries its duration, and "?" for a test run with no verdict.
	if got := strings.TrimSpace(doc[3]); !strings.HasPrefix(got, "│ test") || !strings.HasSuffix(got, "?  1m") {
		t.Errorf("row 3 = %q, want the tick %q", got, "│ test … ?  1m")
	}
	if strings.HasPrefix(strings.TrimSpace(doc[4]), "◆ build") == false {
		t.Errorf("the leg after the tick should follow it directly, got %q", doc[4])
	}
	for _, row := range doc {
		if strings.Contains(row, "◆ test") {
			t.Errorf("the empty test leg still has a full row: %q", row)
		}
	}

	// The tick is still a moment: the enumeration keeps every leg, so `j`/`k`
	// can land on it and the reader can anchor there.
	legs := 0
	for _, r := range TrailRows(tr, 2) {
		if r.Kind == "leg" {
			legs++
		}
	}
	if legs != 4 {
		t.Errorf("TrailRows lists %d legs, want 4: the tick must stay selectable", legs)
	}

	// And a result, a file, or a narrated phrase each earn the row back.
	withResult := tr
	withResult.Legs = append([]journey.Leg(nil), tr.Legs...)
	withResult.Legs[1].Waypoints = []journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "12 passed", Short: "12✓"}}
	// prompt, cap, scout, rail, test, rail, build, rail, fix.
	if doc := TrailLines(withResult, o); len(doc) != 9 {
		t.Errorf("a test leg with a parsed run is %d rows, want a full row (9 total)", len(doc))
	}
	narrated := o
	narrated.SessionKey = sessionKey("s-x")
	narrated.Labels = map[string]string{narrator.LegKey(sessionKey("s-x"), tr.Legs[1]): "ran the auth suite"}
	if doc := TrailLines(tr, narrated); len(doc) != 9 {
		t.Errorf("a narrated test leg is %d rows, want a full row (9 total)", len(doc))
	}
}

// A closed leg's right-margin figure is how long it took, not how long ago it
// began. On a session that did its work in one burst every leg read "17h".
func TestAClosedLegShowsItsDuration(t *testing.T) {
	forceASCII(t)
	base := fixtureBase
	tr := journey.Trail{
		Prompts: []journey.Prompt{{Text: "go", At: base}},
		Legs: []journey.Leg{
			{Class: journey.Build, Label: "tokens.py", Start: base.Add(10 * time.Minute), End: base.Add(22 * time.Minute), Files: []string{"tokens.py"}},
		},
	}
	got := RenderTrail(tr, TrailOpts{Now: base.Add(17 * time.Hour), Width: 38, Height: 5, Level: 1, Cursor: -1})
	var legRow string
	for _, row := range strings.Split(got, "\n") {
		if strings.Contains(row, "build") {
			legRow = row
		}
	}
	if !strings.Contains(legRow, "12m") {
		t.Errorf("the leg does not show its 12m duration: %q", legRow)
	}
	// The prompt keeps its age (17h); the leg must not.
	if strings.Contains(legRow, "17h") {
		t.Errorf("the leg still shows its age: %q", legRow)
	}
}

// The plan reaches the trail from the transcript when the session kept one
// there, and from the todo file only when it did not.
func TestThePlanFromTheTranscriptFeedsTheGhosts(t *testing.T) {
	forceASCII(t)
	m := New(nil)
	m.SetSize(120, 30)
	m.SetSessions(fixtureSessions(fixtureBase), fixtureBase.Add(40*time.Minute))
	m.point(sessionKey("s-api"))
	openTrail(m)

	tr := fixtureTrail(fixtureBase)
	tr.Tasks = []journey.Task{
		{ID: "1", Subject: "Read middleware.py", Status: "completed"},
		{ID: "2", Subject: "Fix the token refresh", Active: "Fixing the token refresh", Status: "in_progress"},
		{ID: "3", Subject: "Add a regression test", Status: "pending"},
		{ID: "4", Subject: "Rewrite everything", Status: "deleted"},
	}
	fileTodos := []todo.Item{{Text: "from the todo file", Status: todo.Pending}}
	m.Update(fleetMsg{sessions: m.sessions, at: fixtureBase.Add(40 * time.Minute),
		trailFor: sessionKey("s-api"), hasTrail: true, trail: tr, todos: fileTodos})

	got := m.View()
	if !strings.Contains(got, "◌ Add a regression test") {
		t.Errorf("the pending task is not a ghost:\n%s", got)
	}
	if !strings.Contains(got, "Fixing the token refresh") {
		t.Errorf("the in-progress task does not name HEAD:\n%s", got)
	}
	for _, gone := range []string{"Rewrite everything", "from the todo file", "◌ Read middleware.py"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q reached the trail; deleted tasks, the todo file and completed tasks must not", gone)
		}
	}

	// No tasks in the transcript: the todo file still counts.
	m.Update(fleetMsg{sessions: m.sessions, at: fixtureBase.Add(41 * time.Minute),
		trailFor: sessionKey("s-api"), hasTrail: true, trail: fixtureTrail(fixtureBase), todos: fileTodos})
	if got := m.View(); !strings.Contains(got, "◌ from the todo file") {
		t.Errorf("with no tasks in the transcript the todo file should feed the ghosts:\n%s", got)
	}
}

// planItems is the seam between the transcript's tasks and what the trail
// draws: deleted tasks drop out entirely — not as a ghost, not in a count —
// and the rest keep both tenses.
func TestPlanItemsDropDeletedTasks(t *testing.T) {
	items := planItems([]journey.Task{
		{ID: "1", Subject: "Fix it", Active: "Fixing it", Status: "in_progress"},
		{ID: "2", Subject: "Never mind", Status: "deleted"},
		{ID: "3", Subject: "Test it", Status: "pending"},
	})
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2 (the deleted one gone): %+v", len(items), items)
	}
	if items[0].Text != "Fix it" || items[0].Active != "Fixing it" || items[0].Status != todo.InProgress {
		t.Errorf("items[0] = %+v", items[0])
	}
	if items[1].Text != "Test it" || items[1].Status != todo.Pending {
		t.Errorf("items[1] = %+v", items[1])
	}
}
