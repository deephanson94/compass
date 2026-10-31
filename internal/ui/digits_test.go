package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

// twelve is a fleet of twelve live sessions, numbered 1 to 12 in board
// order, with a wall clock the test moves by hand.
func twelve(t *testing.T) (*Model, *time.Time) {
	t.Helper()
	withCards(t)
	n := sceneNow
	var ss []fleet.Session
	for i := 1; i <= 12; i++ {
		id := fmt.Sprintf("s%02d", i)
		ss = append(ss, sess(id, id, "/home/user/"+id, "main", "task "+id, state.Idle, n.Add(-time.Duration(i)*time.Minute), journey.Build, "", "", ""))
	}
	m := New(nil)
	m.SetSize(290, 80)
	m.SetSessions(ss, n)
	clock := n
	m.clock = func() time.Time { return clock }
	return m, &clock
}

// Past nine, numbers go on, and 12 is 1 then 2: 1 selects at once as it
// always did, and a second digit inside the window moves on to 12.
func TestTwoDigitsSelectPastNine(t *testing.T) {
	m, clock := twelve(t)
	if d := m.digits[sessionKey("s12")]; d != 12 {
		t.Fatalf("the twelfth session wears %d, want 12", d)
	}

	press(m, "1")
	if got := selectedName(m); got != "s01" {
		t.Fatalf("1 selected %s, want s01 at once", got)
	}
	*clock = clock.Add(300 * time.Millisecond)
	press(m, "2")
	if got := selectedName(m); got != "s12" {
		t.Errorf("1 then 2 inside the window selected %s, want s12", got)
	}

	// 1 then 0 is 10.
	press(m, "1")
	press(m, "0")
	if got := selectedName(m); got != "s10" {
		t.Errorf("1 then 0 selected %s, want s10", got)
	}

	// Too slow: the second digit is a digit of its own.
	press(m, "1")
	*clock = clock.Add(2 * time.Second)
	press(m, "2")
	if got := selectedName(m); got != "s02" {
		t.Errorf("2 after the window selected %s, want s02", got)
	}

	// A digit no two-digit number starts with arms nothing: 3 then 4 is 4.
	press(m, "3")
	press(m, "4")
	if got := selectedName(m); got != "s04" {
		t.Errorf("3 then 4 selected %s, want s04", got)
	}

	// A lone 0 does nothing.
	press(m, "0")
	if got := selectedName(m); got != "s04" {
		t.Errorf("0 moved the selection to %s", got)
	}
}

// The card wears its two-digit number.
func TestACardWearsItsTwoDigitNumber(t *testing.T) {
	m, _ := twelve(t)
	m.point(sessionKey("s12"))
	if frame := ansi.Strip(m.View()); !strings.Contains(frame, "▸12 ") {
		t.Errorf("the selected twelfth card should read ▸12:\n%s", frame)
	}
}

// Nine or fewer: nothing is armed, so every digit is the one it always was.
func TestNineOrFewerArmNothing(t *testing.T) {
	m := sceneModel(sceneCards(), 290, 80) // eight sessions
	press(m, "1")
	if m.digitArmed != 0 {
		t.Errorf("a fleet of eight armed the two-digit window on 1")
	}
}
