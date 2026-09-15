package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

// Round fifty-nine, asked for: the question you walked away from (#344).
// A session that ends its turn on "which of these should I do next?" used to
// leave the board five minutes after its pane did, and the fleet forgot it
// was owed an answer. The fleet now keeps it — marked `Waiting` — and the
// board has to put it somewhere that does not bury today's work.

// waiting is a session held on the board by an old question of yours: amber,
// paneless, and asking since `since`.
func waiting(id, name, title string, since time.Time) fleet.Session {
	s := sess(id, name, "/home/user/"+name, "main", title, state.NeedsYou, since,
		journey.Build, "", "turn ended with a question", "awaiting your reply")
	s.Waiting = true
	s.Info.Asked, s.Info.AskedAt = true, since
	return s
}

// r59Fleet is a morning's board: one session asking you right now, one
// working, and two questions from days ago that nobody answered.
func r59Fleet() []fleet.Session {
	now := fixtureBase
	out := []fleet.Session{
		sess("s-api", "api", "/home/user/api", "main", "fix the 401 on token refresh",
			state.NeedsYou, now.Add(-4*time.Minute), journey.Fix, "", "turn ended with a question", "awaiting your reply"),
		sess("s-etl", "etl", "/home/user/etl", "main", "dedupe the nightly load",
			state.Working, now.Add(-20*time.Minute), journey.Build, "", "tool call in flight", "thinking…"),
		waiting("s-docs", "docs", "rewrite the install guide", now.Add(-26*time.Hour)),
		waiting("s-infra", "infra", "split the vpc module", now.Add(-3*24*time.Hour)),
	}
	// In the order the manager hands it over: the alarms of the moment, the
	// questions left behind (longest wait first), then the work in flight.
	fleet.SortFleet(out)
	return out
}

func r59Model(t *testing.T) *Model {
	t.Helper()
	forceASCII(t)
	m := New(nil)
	m.Update(tea.WindowSizeMsg{Width: 152, Height: 40})
	m.Update(fleetMsg{sessions: r59Fleet(), at: fixtureBase, trails: map[string]journey.Trail{}, paired: true})
	return m
}

func orderedNames(m *Model) []string {
	var out []string
	for _, i := range m.viewOrder() {
		out = append(out, sessionName(m.sessions[i].Info))
	}
	return out
}

// The board keeps the question and ranks it honestly: under the session
// asking you now, over the one that is working. Two sessions waiting sort
// oldest-question-first, as every other alarm does.
func TestAWaitingSessionKeepsItsColumnUnderTodaysAlarms(t *testing.T) {
	m := r59Model(t)
	if got, want := strings.Join(orderedNames(m), ","), "api,infra,docs,etl"; got != want {
		t.Fatalf("board order = %s, want %s — the waiting go under the live alarm and over the work", got, want)
	}
	view := ansi.Strip(m.View())
	for _, name := range []string{"docs", "infra"} {
		if !strings.Contains(view, name) {
			t.Errorf("the board does not draw the waiting session %q:\n%s", name, view)
		}
	}
	// The wait is said in the frame's own words, so the person can tell a
	// question from this morning from one from Tuesday.
	if !strings.Contains(view, "3d") {
		t.Errorf("the board never says how long the oldest question has waited:\n%s", view)
	}
}

// `x` is a decision on a question you are not going to answer, so it sticks:
// the fleet keeps saying needs-you, and the board keeps it off. An alarm from
// today still overrides a hide, which is the rule this one is carved out of.
func TestHidingAWaitingSessionSticks(t *testing.T) {
	m := r59Model(t)
	m.point(sessionKey("s-infra"))
	pressKey(m, "x")

	if !m.hidden[sessionKey("s-infra")] {
		t.Fatalf("x refused the waiting session: note %q", m.note)
	}
	m.Update(fleetMsg{sessions: r59Fleet(), at: fixtureBase.Add(time.Minute), trails: map[string]journey.Trail{}, paired: true})
	if got := strings.Join(orderedNames(m), ","); strings.Contains(got, "infra") {
		t.Errorf("board order = %s, want infra gone — a hide that comes back on the next poll is not a decision", got)
	}

	// The alarm of the moment is not hideable, and says so.
	m.point(sessionKey("s-api"))
	pressKey(m, "x")
	if m.hidden[sessionKey("s-api")] {
		t.Errorf("x hid the session that is asking you now")
	}
	if !strings.Contains(m.note, "it is asking") {
		t.Errorf("note = %q, want the refusal for a live alarm", m.note)
	}
}

// `X` is the pile: every question you have walked away from, off the board in
// one key, and back with `A` then `X`. It never empties the board.
func TestTheSweepTakesEveryWaitingSessionAndGivesThemBack(t *testing.T) {
	m := r59Model(t)
	pressKey(m, "X")

	for _, id := range []string{"s-docs", "s-infra"} {
		if !m.hidden[sessionKey(id)] {
			t.Fatalf("X left %s on the board: note %q", id, m.note)
		}
	}
	if m.hidden[sessionKey("s-api")] || m.hidden[sessionKey("s-etl")] {
		t.Errorf("X took a session that is not waiting on an old question")
	}
	if !strings.Contains(m.note, "2 waiting hidden") {
		t.Errorf("note = %q, want the count it hid", m.note)
	}
	if got, want := strings.Join(orderedNames(m), ","), "api,etl"; got != want {
		t.Fatalf("board order = %s, want %s", got, want)
	}

	// A second press has nothing to take, and says so rather than acting.
	pressKey(m, "X")
	if !strings.Contains(m.note, "nothing is waiting") {
		t.Errorf("note = %q, want the refusal", m.note)
	}

	// The archive is where they went, and `X` there brings every one back.
	pressKey(m, "A")
	pressKey(m, "X")
	if len(m.hidden) != 0 {
		t.Fatalf("X in the archive left %d sessions hidden", len(m.hidden))
	}
	if !strings.Contains(m.note, "2 sessions back on the board") {
		t.Errorf("note = %q, want the count it gave back", m.note)
	}
}

// The board is never swept empty: a fleet that is nothing but forgotten
// questions keeps the one the board draws first, exactly as `x` keeps the
// last live session.
func TestTheSweepLeavesOneQuestionStanding(t *testing.T) {
	forceASCII(t)
	m := New(nil)
	m.Update(tea.WindowSizeMsg{Width: 152, Height: 40})
	only := []fleet.Session{
		waiting("s-docs", "docs", "rewrite the install guide", fixtureBase.Add(-26*time.Hour)),
		waiting("s-infra", "infra", "split the vpc module", fixtureBase.Add(-3*24*time.Hour)),
	}
	fleet.SortFleet(only)
	m.Update(fleetMsg{sessions: only, at: fixtureBase, trails: map[string]journey.Trail{}, paired: true})
	pressKey(m, "X")

	if m.hidden[sessionKey("s-infra")] {
		t.Errorf("the sweep hid the question the board draws first and left it emptier than x would")
	}
	if !m.hidden[sessionKey("s-docs")] {
		t.Errorf("the sweep took nothing: note %q", m.note)
	}
	if !strings.Contains(m.note, "one stays") {
		t.Errorf("note = %q, want the sentence that says one stayed", m.note)
	}
}

// `g` is "unblock whichever session needs me". Today's question comes first;
// the ones you walked away from are what it reaches when nothing else asks.
func TestTheGrabTakesTodaysQuestionBeforeTheOldOnes(t *testing.T) {
	m := r59Model(t)
	if !m.selectOldestNeedsYou() {
		t.Fatalf("g found nothing to grab")
	}
	if got := sessionName(m.sessions[m.selectedIndex()].Info); got != "api" {
		t.Errorf("g grabbed %q, want the session asking you now", got)
	}

	// The live alarm answered: now `g` has the oldest question left behind.
	rest := r59Fleet()[1:]
	m.Update(fleetMsg{sessions: rest, at: fixtureBase.Add(time.Minute), trails: map[string]journey.Trail{}, paired: true})
	if !m.selectOldestNeedsYou() {
		t.Fatalf("g found nothing once the live alarm was answered")
	}
	if got := sessionName(m.sessions[m.selectedIndex()].Info); got != "infra" {
		t.Errorf("g grabbed %q, want the oldest question left behind", got)
	}

	// And a question you have put down with `x` is not one `g` picks up.
	m.hidden = map[string]bool{sessionKey("s-infra"): true}
	m.Update(fleetMsg{sessions: rest, at: fixtureBase.Add(2 * time.Minute), trails: map[string]journey.Trail{}, paired: true})
	if !m.selectOldestNeedsYou() {
		t.Fatalf("g found nothing with one question hidden")
	}
	if got := sessionName(m.sessions[m.selectedIndex()].Info); got != "docs" {
		t.Errorf("g grabbed %q, want the question still on the board", got)
	}
}
