package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/deephanson94/compass/internal/prstate"
)

// prScene is the few-ongoing scene with two pull requests: billing, idle,
// whose reviewer asked for changes, and webapp, working, which waits on its
// reviewer.
func prScene(t *testing.T, w, h int) *Model {
	t.Helper()
	sc := sceneFewOngoing()
	m := sceneModel(sc, w, h)
	m.SetPRs(map[string]prstate.Status{
		sessionKey("billing"): {Number: 1118, Move: prstate.Yours, Reason: "changes requested", At: sceneNow.Add(-2 * 3600e9)},
		sessionKey("webapp"):  {Number: 77, Move: prstate.Theirs, Who: "reviewer1", At: sceneNow.Add(-3 * 3600e9)},
	})
	return m
}

// The pull request is said under the session's name, on the board and in
// the fleet list, in words that read without colour.
func TestThePRLineIsDrawnUnderTheName(t *testing.T) {
	forceASCII(t)
	for _, c := range []struct {
		wh    [2]int
		wants []string
	}{
		{[2]int{220, 48}, []string{"▲ PR #1118 · changes requested 2h ago", "PR #77 · waiting on reviewer1 · 3h"}},
		// The fleet list: webapp is below the fold, and the clock sheds its "ago".
		{[2]int{100, 30}, []string{"▲ PR #1118 · changes requested 2h"}},
	} {
		wh := c.wh
		m := prScene(t, wh[0], wh[1])
		view := ansi.Strip(m.View())
		for _, want := range c.wants {
			if !strings.Contains(view, want) {
				t.Errorf("%dx%d: no %q:\n%s", wh[0], wh[1], want, view)
			}
		}
		for i, row := range strings.Split(view, "\n") {
			if n := ansi.StringWidth(row); n > wh[0] {
				t.Errorf("%dx%d: row %d is %d cells: %q", wh[0], wh[1], i, n, row)
			}
		}
	}
}

// A reviewer's answer sorts with the questions: billing, idle for hours,
// comes before the sessions that are working.
func TestYourMoveSortsWithNeedsYou(t *testing.T) {
	m := prScene(t, 220, 48)
	billing := -1
	for i, s := range m.sessions {
		if s.Info.Key() == sessionKey("billing") {
			billing = i
		}
	}
	if got := m.obligation(m.sessions[billing]); got != rankNeedsYou {
		t.Fatalf("billing ranks %d, want rankNeedsYou", got)
	}
	order := m.liveOrder(false)
	pos := map[string]int{}
	for p, i := range order {
		pos[m.sessions[i].Info.Key()] = p
	}
	if pos[sessionKey("billing")] > pos[sessionKey("api")] {
		t.Fatalf("billing (your move) sorts after api (working): %v", pos)
	}
	// Their move changes nothing: webapp keeps a working session's rank.
	for _, s := range m.sessions {
		if s.Info.Key() == sessionKey("webapp") && m.obligation(s) == rankNeedsYou {
			t.Fatal("a PR waiting on its reviewer sorts as needs you")
		}
	}
}

// With no pull request known, nothing about the deck changes.
func TestNoPRNoLine(t *testing.T) {
	forceASCII(t)
	sc := sceneFewOngoing()
	m := sceneModel(sc, 220, 48)
	if view := ansi.Strip(m.View()); strings.Contains(view, "PR #") {
		t.Fatalf("a PR line with no PR:\n%s", view)
	}
}
