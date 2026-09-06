package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/state"
)

// A shell command inside the budget the harness gave it is a session doing
// what it said: HEAD says "for 2m of 10m", never "silent 2m", and the fleet
// row reads the same sentence (#45).
func TestABashInsideItsBudgetIsNotStuck(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	api := sessionKey("s-api")
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == api {
			m.sessions[i].Snap = state.Snapshot{State: state.Working, Since: fixtureBase.Add(34 * time.Minute),
				Reason: "Bash allowed 10m", Activity: "Bash: sleep 420", Allowed: 10 * time.Minute}
			m.sessions[i].Info.LastEventAt = fixtureBase.Add(34 * time.Minute)
		}
	}
	col := strings.Join(m.boardColumn(api, rowFor(t, m, api), 70, 20), "\n")
	if !strings.Contains(col, "for 6m of 10m") {
		t.Errorf("a column's HEAD inside its Bash budget does not say how much of it is spent:\n%s", col)
	}
	if strings.Contains(col, "silent") || strings.Contains(col, "◍") {
		t.Errorf("a Bash inside its budget is drawn as hung:\n%s", col)
	}
	m.point(api)
	openTrail(m)
	if got := strings.Join(m.trailColumn(70, 20), "\n"); !strings.Contains(got, "for 6m of 10m") || strings.Contains(got, "silent") {
		t.Errorf("the single trail's HEAD does not carry the budget:\n%s", got)
	}
}

// PgDn and PgUp are the half-page keys under other names: the person who
// reached for them on a long Lv2 trail got a dead key.
func TestThePageKeysMoveHalfAPage(t *testing.T) {
	forceASCII(t)
	m := boardModel(100, 24)
	openTrail(m)
	m.SetTrail(longTrail(60))
	press(m, "tab") // Lv2: the cursor is what moves
	was := m.cursor
	press(m, "pgup")
	if m.cursor >= was {
		t.Fatalf("PgUp moved the cursor from %d to %d, want up by half a page", was, m.cursor)
	}
	if m.cursor != was-m.trailHalfPage() {
		t.Errorf("PgUp moved %d rows, want the half page ctrl+u moves (%d)", was-m.cursor, m.trailHalfPage())
	}
	press(m, "pgdown")
	if m.cursor != was {
		t.Errorf("PgDn did not come back: cursor %d, want %d", m.cursor, was)
	}
}
