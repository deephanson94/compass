package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/state"
)

// poll delivers a fleet, as a refresh does.
func pollFleet(m *Model, at time.Time) {
	m.Update(fleetMsg{sessions: m.sessions, at: at})
}

func setSnap(m *Model, name string, snap state.Snapshot, last time.Time) {
	for i, s := range m.sessions {
		if sessionName(s.Info) == name {
			m.sessions[i].Snap = snap
			if !last.IsZero() {
				m.sessions[i].Info.LastEventAt = last
			}
		}
	}
}

// z takes an idle session off the board until its next prompt: a line of
// bookkeeping does not wake it, a prompt does, and once back it is not
// hidden again by its next idle hour.
func TestSnoozeHidesUntilTheNextPrompt(t *testing.T) {
	withCards(t)
	m := sceneModel(sceneCards(), 290, 80)
	m.point(sessionKey("s3"))
	press(m, "z")
	if !m.hidden[sessionKey("s3")] || !strings.Contains(m.note, "s3 is snoozed · back on its next prompt") {
		t.Fatalf("z: hidden %v, note %q", m.hidden[sessionKey("s3")], m.note)
	}
	if lineWith(ansi.Strip(m.View()), "○ s3 ") != "" { // a card's head row; the footer's note names it too
		t.Errorf("a snoozed session is still on the board")
	}

	// A hook writes a line: LastEventAt moves, the beat does not.
	s3, _ := m.session(sessionKey("s3"))
	setSnap(m, "s3", s3.Snap, sceneNow.Add(time.Minute))
	pollFleet(m, sceneNow.Add(time.Minute))
	if !m.hidden[sessionKey("s3")] {
		t.Fatal("a line of bookkeeping woke the snooze")
	}

	// The cron's prompt lands: a turn in flight.
	setSnap(m, "s3", state.Snapshot{State: state.Working, Since: sceneNow.Add(2 * time.Minute)}, sceneNow.Add(2*time.Minute))
	pollFleet(m, sceneNow.Add(2*time.Minute))
	if m.hidden[sessionKey("s3")] {
		t.Fatal("the next prompt did not bring the session back")
	}
	if _, ok := m.snoozed[sessionKey("s3")]; ok {
		t.Error("the snooze outlived the wake")
	}

	// Its turn ends; idle again, it stays on the board.
	setSnap(m, "s3", state.Snapshot{State: state.Idle, Since: sceneNow.Add(5 * time.Minute)}, sceneNow.Add(5*time.Minute))
	pollFleet(m, sceneNow.Add(6*time.Minute))
	if m.hidden[sessionKey("s3")] {
		t.Error("an idle session that woke from a snooze was hidden again")
	}
}

// z refuses what would wake at once (a working session) and what x refuses;
// in the archive x ends a snooze by hand.
func TestSnoozeRefusesAndXEndsIt(t *testing.T) {
	withCards(t)
	m := sceneModel(sceneCards(), 290, 80)
	m.point(sessionKey("stages")) // working
	press(m, "z")
	if m.hidden[sessionKey("stages")] || !strings.Contains(m.note, "is working") {
		t.Errorf("z on a working session: hidden %v, note %q", m.hidden[sessionKey("stages")], m.note)
	}

	m.point(sessionKey("harness")) // asking: x refuses it, so z does
	press(m, "z")
	if m.hidden[sessionKey("harness")] {
		t.Error("z hid a session x refuses to hide")
	}

	m.point(sessionKey("s3"))
	press(m, "z")
	press(m, "A")
	m.point(sessionKey("s3"))
	press(m, "x")
	if m.hidden[sessionKey("s3")] {
		t.Fatal("x in the archive did not bring the snoozed session back")
	}
	if _, ok := m.snoozed[sessionKey("s3")]; ok {
		t.Error("x brought it back but the snooze stayed")
	}
}
