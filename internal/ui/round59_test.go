package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The porter column draws the lead's message as the ask, worn as a relay:
// the card and the trail say `relayed "…"`, not the harness's envelope, and
// not nothing (#97).
func TestARelayedAskIsWornAsARelay(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 220, 48)
	view := ansi.Strip(m.View())
	if strings.Contains(view, "Another Claude session") {
		t.Errorf("the relay's envelope is drawn as the ask:\n%s", view)
	}
	if !strings.Contains(view, `relayed "the encoder is in, run the gates"`) {
		t.Errorf("the relayed ask is not worn as one:\n%s", view)
	}
	press(m, "A")
	if view := ansi.Strip(m.View()); !strings.Contains(view, `relayed "`) && strings.Contains(view, "porter") {
		t.Errorf("the archive's strip drops the relay word:\n%s", view)
	}
}

// A search narrows the band instead of blanking it: `/billing` keeps the
// band row it found, under a miss note that says what did not match (#98).
func TestASearchNarrowsTheBand(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 100, 30)
	for _, k := range []string{"/", "billing"} {
		pressKey(m, k)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "no live session matches /billing") {
		t.Errorf("the miss note does not say what missed:\n%s", view)
	}
	rows := 0
	for _, l := range strings.Split(view, "\n") {
		fleet := strings.SplitN(l, "│", 2)[0]
		if strings.Contains(fleet, " ○ ") {
			rows++
			if !strings.Contains(fleet, "billing") {
				t.Errorf("the band draws a row the search did not find: %q", l)
			}
		}
	}
	if rows != 1 {
		t.Errorf("the band under /billing draws %d rows, want the one it found:\n%s", rows, view)
	}
}

// At 120 the two-tools help glosses the tool word its rows wear: `?` and
// `q` share a row at every width, and the row they freed is the gloss's (#99).
func TestTheHelpMergesHelpAndQuitAtEveryWidth(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 120, 34)
	press(m, "?")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "? / q") {
		t.Errorf("? and q take a row each at 120:\n%s", view)
	}
	if !strings.Contains(view, "which CLI runs the session") {
		t.Errorf("the help leaves the word its rows wear undefined:\n%s", view)
	}
}

// The hide's note is the refusal's sentence (app.go's "%d %s is hidden"):
// a bare leading digit beside a strip that counts ("1 hidden") reads as a
// count, and the fleet with two sessions called harness is where it
// misreads (#101, #57, #31).
func TestTheHideNoteIsNotACount(t *testing.T) {
	m := sceneModel(sceneFleetHygiene(), 120, 34)
	press(m, "2")
	press(m, "x")
	if !strings.Contains(m.note, "2 harness is hidden · A, then x") {
		t.Errorf("the hide note reads as a count: %q", m.note)
	}
}

// In the session view the card keeps its tag and leaves the present to the
// trail, which draws HEAD three rows below in the same column (#100).
func TestTheCardLeavesThePresentToTheTrail(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 152, 40)
	view := ansi.Strip(m.View())
	if n := strings.Count(view, "thinking…"); n != 1 {
		t.Errorf("the column says the present %d times, want once:\n%s", n, view)
	}
	if !strings.Contains(view, "claude · ⌁ main") {
		t.Errorf("the card lost its tag with the sentence:\n%s", view)
	}
}

// In the session view the search's note says whose miss it is where the
// band beneath holds what the search found, as the list's note does (#102).
func TestTheSessionViewMissNoteSaysTheLiveOneMissed(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 152, 40)
	for _, k := range []string{"/", "billing"} {
		pressKey(m, k)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "billing · opencode") && !strings.Contains(view, " ○ billing") {
		t.Fatalf("the band draws no billing row under the search:\n%s", view)
	}
	if !strings.Contains(view, "no live session matches /billing") {
		t.Errorf("the card says no session matches over the band's match:\n%s", view)
	}
}

// The archive says a row's verdict: the second row ends on the band's own
// clause where the column has room (#103, #47).
func TestTheArchiveRowSaysItsVerdict(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 152, 40)
	press(m, "2")
	view := ansi.Strip(m.View())
	if !m.archiveView {
		t.Fatalf("2 did not open the archive:\n%s", view)
	}
	if !strings.Contains(view, "claude · fix/api-timeouts · ✗ red") {
		t.Errorf("the archive row says nothing of whether the day went red:\n%s", view)
	}
}
