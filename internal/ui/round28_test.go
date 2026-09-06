package ui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
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

// headerOf is the deck's first row, plain.
func headerOf(m *Model) string {
	return ansi.Strip(strings.SplitN(m.View(), "\n", 2)[0])
}

// The selected session's digit and name ride the header at every level and
// every width — the one row that never moves under a zoom — and a namesake
// carries its ⌁ tag there, so two sessions called harness are never the
// same title (#46).
func TestTheHeaderNamesTheSelectedSessionAtEveryLevel(t *testing.T) {
	for _, w := range []int{80, 120, 220} {
		m := sceneModel(sceneFleetHygiene(), w, 34)
		for i := 0; i < 3; i++ {
			if got := headerOf(m); !strings.Contains(got, " · 1 porter") || strings.Contains(got, "⌁") {
				t.Errorf("at %d after %d tabs the header does not name the selection, or tags a session with no namesake: %q", w, i, got)
			}
			pressTab(m)
		}
		press(m, "2")
		if got := headerOf(m); !strings.Contains(got, "2 harness · ⌁ harness:1.0") {
			t.Errorf("at %d a digit press does not show its landing on the header: %q", w, got)
		}
		other := strconv.Itoa(m.digits[sessionKey("harness-b")])
		press(m, other)
		if got := headerOf(m); !strings.Contains(got, other+" harness · ⌁ harness:0.0") {
			t.Errorf("at %d the namesake is not told apart on the header: %q", w, got)
		}
	}
}

// The chips are the product: the header's identity sheds — the board word,
// the tag, the search, then the name clips around its digit — and the
// chips never do.
func TestTheHeaderShedsTheNameBeforeTheChips(t *testing.T) {
	m := sceneModel(sceneFleetHygiene(), 120, 34)
	press(m, "2") // the namesake, tagged
	full := ansi.Strip(m.headerLine(118))
	if !strings.Contains(full, "· board · 2 harness · ⌁ harness:1.0") {
		t.Fatalf("the board header does not carry the whole identity: %q", full)
	}
	chips := ansi.Strip(m.statusChips())
	for _, w := range []int{60, 40, 30} {
		got := ansi.Strip(m.headerLine(w))
		if !strings.HasSuffix(got, chips) {
			t.Errorf("at %d the chips gave way: %q", w, got)
		}
		if lipgloss.Width(got) > w {
			t.Errorf("at %d the header overflows: %q", w, got)
		}
	}
	if got := ansi.Strip(m.headerLine(60)); strings.Contains(got, "board") || !strings.Contains(got, "2 harness") {
		t.Errorf("the board word should go before the name: %q", got)
	}
	if got := ansi.Strip(m.headerLine(40)); strings.Contains(got, "⌁") || !strings.Contains(got, " 2 ") {
		t.Errorf("the tag should go before the digit: %q", got)
	}
}
