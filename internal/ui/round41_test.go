package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/deephanson94/compass/internal/journey"
)

// The pins of round forty-one (#66): each fold that a width change could
// revert with the suite green.

// Below the board's width the reader owns the screen, and its tail carries
// the present the trail draws: a fresh session's reader says it is alive (#66).
func TestTheReaderAloneSaysTheSessionIsAlive(t *testing.T) {
	forceASCII(t)
	for _, w := range []int{80, 100} {
		m := sceneModel(sceneSecondDay(), w, 30)
		pressTab(m)
		pressTab(m)
		if m.level != levelReader {
			t.Fatalf("at %d expected the reader, level %d", w, m.level)
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "● scout  thinking…") || !strings.Contains(view, "for 40s") {
			t.Errorf("at %d the reader alone never says the session is working:\n%s", w, view)
		}
	}
	m := sceneModel(sceneSecondDay(), 120, 34)
	pressTab(m)
	pressTab(m)
	if view := ansi.Strip(m.View()); strings.Count(view, "thinking…") != 2 {
		t.Errorf("at 120 the card and the trail already carry the present, the reader does not:\n%s", view)
	}
}

// The reader's title sheds a clause that is the name and a bracket: the
// leg's "(commit)" is not worth a second copy of the name (#66).
func TestTheReaderTitleShedsTheNameAndABracket(t *testing.T) {
	forceASCII(t)
	sc := sceneSecondDay()
	for _, w := range []int{100, 220} {
		m := sceneModel(sc, w, 40)
		for _, k := range []string{"tab", "2", "tab", "j", "tab"} {
			pressKey(m, k)
			poll(m, sc)
		}
		title := ansi.Strip(m.readerTitle(w - 2))
		if strings.Count(title, "fix the 401 on token refresh") != 1 || !strings.Contains(title, "15:31") {
			t.Errorf("at %d the reader's title = %q", w, title)
		}
	}
	if nameAndBracket("auth", "auth: drop the legacy path") || !nameAndBracket("fix it", "fix it (commit)") {
		t.Error("nameAndBracket: only a bracket after the whole name")
	}
}

// The digest's lane words: a lane back with nothing is "back, empty", and
// the lanes counted since the look are "sent since" (#66).
func TestTheDigestsLaneWords(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSubagents(), 220, 48)
	view := ansi.Strip(m.View())
	for _, want := range []string{"↳ 1 back since, empty", "↳ 3 sent since, none back"} {
		if !strings.Contains(view, want) {
			t.Errorf("the board should say %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "↳ 1 agent back") || strings.Contains(view, "agents out, none back") {
		t.Errorf("the old words survive:\n%s", view)
	}
}

// At 120 the lead's card borrows the header's form of the out clause so the
// lane back is not shed by one cell (#66).
func TestTheCardBorrowsTheHeadersFormForTheLaneBack(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSubagents(), 120, 34)
	if view := ansi.Strip(m.View()); !strings.Contains(view, "◈3 out · 2 silent 18m · 1 back") {
		t.Errorf("the 120 card should keep the lane back:\n%s", view)
	}
	wide := sceneModel(sceneSubagents(), 220, 48)
	if view := ansi.Strip(wide.View()); !strings.Contains(view, "◈3 out 20m · 2 silent 18m · 1 back") {
		t.Errorf("with the room the dispatch age stays:\n%s", view)
	}
}

// The agent's own reader says the silence on the call it is hung on (#66).
func TestTheLaneReaderSaysTheSilenceOnItsHungCall(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	m := sceneModel(sc, 80, 24)
	seen := false
	for _, k := range append(append(append([]string(nil), canonicalKeys...), "esc"), sc.extra...) {
		pressKey(m, k)
		poll(m, sc)
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "⏺ Bash(pytest -x tests/plugins)") || m.readerLane == "" {
			continue
		}
		seen = true
		if !strings.Contains(view, "⋯ no result yet · silent") {
			t.Errorf("after %q the hung call says no silence:\n%s", k, view)
		}
	}
	if !seen {
		t.Fatal("the walkthrough never opens the silent lane's reader on its hung call")
	}
}

// At eighty columns the recent band keeps the verdict's mark where "✗ red"
// is a cell short of the prompt's floor (#67).
func TestTheNarrowBandKeepsTheVerdictsMark(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 80, 24)
	view := ansi.Strip(m.View())
	for _, want := range []string{"✗ 2h", "✓ 6h", "✓ 9h"} {
		if !strings.Contains(view, want) {
			t.Errorf("the 80 band should keep the verdict's mark %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "✗ red 2h") {
		t.Errorf("at 80 the whole verdict does not fit beside the prompt's floor:\n%s", view)
	}
}

// A lane's link survives its own file when the matched session is the
// fresher reading, and not otherwise (#67).
func TestTheLaneLinkFollowsTheFresherReading(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSubagents(), 220, 48)
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Red-team the plugin architecture →1") {
		t.Errorf("the 220 board should link the red-team lane to session 1:\n%s", view)
	}
	// The file written after the session's last event: no hedge.
	agents := m.agentsFor(sessionKey("porter"))
	for id, a := range agents {
		a.Wrote = m.now
		agents[id] = a
	}
	if links := m.laneLinks(m.trails[sessionKey("porter")], agents); len(links) != 0 {
		t.Errorf("a file fresher than every session is not a guess: %v", links)
	}
}

// Lanes back are counted by their return, lanes out by their dispatch (#67).
func TestLanesBackAreCountedByTheirReturn(t *testing.T) {
	look := sceneNow.Add(-time.Hour)
	tr := journey.Trail{Branches: []journey.Branch{
		{Label: "sent before, back since", Start: look.Add(-time.Hour), End: look.Add(10 * time.Minute), Done: true, Report: "found it"},
		{Label: "sent before, back before", Start: look.Add(-time.Hour), End: look.Add(-10 * time.Minute), Done: true},
		{Label: "sent since, still out", Start: look.Add(5 * time.Minute)},
		{Label: "sent before, still out", Start: look.Add(-5 * time.Minute)},
	}}
	if out, back := lanesSince(tr, look); out != 1 || back != 1 {
		t.Errorf("lanesSince = %d out, %d back; want 1 and 1", out, back)
	}
	if n := emptyLanesSince(tr, look); n != 0 {
		t.Errorf("emptyLanesSince = %d, want 0: the empty lane came back before the look", n)
	}
}

// The digest names its scope — "back since" — where it costs no clause,
// and keeps the short form where it would (#67).
func TestTheDigestNamesItsScopeWhereItCostsNoClause(t *testing.T) {
	forceASCII(t)
	wide := sceneModel(sceneSubagents(), 220, 48)
	if view := ansi.Strip(wide.View()); !strings.Contains(view, "↳ 1 back since, empty · 1 new leg · looked 1h ago") {
		t.Errorf("at 220 the digest should name its scope:\n%s", view)
	}
	narrow := sceneModel(sceneSubagents(), 80, 24)
	if view := ansi.Strip(narrow.View()); !strings.Contains(view, "↳ 1 back, empty") || strings.Contains(view, "back since") {
		t.Errorf("at 80 the scope would clip the clause:\n%s", view)
	}
}
