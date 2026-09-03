package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

// Round thirteen: the panel's seven operators, on identity and alarms.

// deadOnAPI turns a board session into one the API refused.
func deadOnAPI(m *Model, key string) {
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == key {
			m.sessions[i].Snap = state.Snapshot{State: state.NeedsYou, Since: fixtureBase.Add(20 * time.Minute), APIError: true,
				Reason: "api error 403 · authentication_failed", Activity: "Please run /login · API Error: 403 your daily quota is exhausted"}
		}
	}
}

// A session dead on the API is its own state end to end: the row wears ⊘
// and the word "quota", it sorts under the question and the loop, the
// header counts it apart, `g` skips it, and the panel offers the remedy
// rather than "pick an answer".
func TestAPIErrorIsItsOwnState(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	dead := sessionKey("s-webapp")
	deadOnAPI(m, dead)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "⊘ webapp") || !strings.Contains(view, "quota") {
		t.Errorf("the row should wear ⊘ and say quota:\n%s", view)
	}
	chips := ansi.Strip(m.statusChips())
	if !strings.Contains(chips, "⊘1 quota") || !strings.Contains(chips, "▲1 ") {
		t.Errorf("the header should count the dead apart from the asking: %q", chips)
	}
	ranks := map[string]int{}
	for _, s := range m.sessions {
		if s.Live {
			ranks[sessionName(s.Info)] = m.obligation(s)
		}
	}
	if !(ranks["infra"] < ranks["webapp"] && ranks["webapp"] < ranks["api"]) {
		t.Errorf("a dead session sorts under the question and above the working: %v", ranks)
	}
	if got := tabTitle(1, 0, 0, 2); got != "⌂ compass ▲1 ⊘2" {
		t.Errorf("tabTitle = %q", got)
	}
	// g grabs the question, not the corpse.
	m.point(dead)
	press(m, "g")
	if m.selectedKey != sessionKey("s-infra") {
		t.Errorf("g went to %q, want the session asking", m.selectedKey)
	}
	// The panel names the refusal and offers the remedy.
	m.point(dead)
	press(m, "r")
	panel := ansi.Strip(m.View())
	for _, want := range []string{"⊘ stopped on an API error", "1  /login — log in again, in that pane"} {
		if !strings.Contains(panel, want) {
			t.Errorf("the panel should say %q:\n%s", want, panel)
		}
	}
	for _, no := range []string{"pick an answer", "please continue", "report status"} {
		if strings.Contains(panel, no) {
			t.Errorf("the panel should not offer %q to a dead session:\n%s", no, panel)
		}
	}
	press(m, "esc")
	// The trail ends on the refusal, not on the last finished leg.
	pressTab(m)
	if trail := ansi.Strip(m.View()); !strings.Contains(trail, "⊘") || !strings.Contains(trail, "dead 20m") {
		t.Errorf("HEAD should be the refusal:\n%s", trail)
	}
	// x leaves it: nothing you type clears it, so it is not hideable.
	m.point(dead)
	press(m, "x")
	if m.hidden[dead] || !strings.Contains(m.note, "dead on the API") {
		t.Errorf("x on a dead session: hidden %v, note %q", m.hidden[dead], m.note)
	}
}

// A session's digit is given on first sight and kept: the rows re-sort as
// what they owe changes, the numbers do not, and `3` lands on the session
// it landed on this morning.
func TestDigitsAreStable(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	before := map[string]int{}
	for k, v := range m.digits {
		before[k] = v
	}
	// The working session finishes and is read: it drops to the strip.
	api := sessionKey("s-api")
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == api {
			m.sessions[i].Snap = state.Snapshot{State: state.Idle, Since: m.now}
		}
	}
	m.markSeen(api)
	m.trails[api] = journey.Trail{}
	m.SetSessions(m.sessions, m.now)
	for k, v := range before {
		if m.digits[k] != v {
			t.Errorf("%s moved from %d to %d", k, v, m.digits[k])
		}
	}
	// The digit still selects it, wherever it stands.
	m.point(sessionKey("s-infra"))
	press(m, "2")
	if m.selectedKey != api {
		t.Errorf("2 selected %q, want the session that wore 2 all along", m.selectedKey)
	}
	// Hidden keeps its digit; archived frees it.
	m.point(sessionKey("s-tfstate"))
	press(m, "x")
	if m.digits[sessionKey("s-tfstate")] != before[sessionKey("s-tfstate")] {
		t.Error("hiding a session took its digit")
	}
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == sessionKey("s-tfstate") {
			m.sessions[i].Live = false
		}
	}
	m.SetSessions(m.sessions, m.now)
	if _, still := m.digits[sessionKey("s-tfstate")]; still {
		t.Error("an archived session keeps a digit nothing on the board can use")
	}
}

// The header's chips partition the board: a circling session is counted
// once, under ↻, with the loop's age; "all calm" never stands beside an
// unread count.
func TestHeaderChipsPartitionTheBoard(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	base := fixtureBase
	loop := hourlyTrail(base, 3)
	for i := range loop.Legs {
		loop.Legs[i].Class, loop.Legs[i].Current = journey.Test, false
		loop.Legs[i].Waypoints = []journey.Waypoint{
			{Kind: journey.WaypointTestRun, Text: "18 passed · 1 failed", Short: "18✓ 1✗", At: loop.Legs[i].End},
			{Kind: journey.WaypointTestFail, Text: "test_x", Runs: i + 1, At: loop.Legs[i].End},
		}
	}
	api := sessionKey("s-api")
	m.trails[api] = loop
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == api {
			m.sessions[i].Snap = state.Snapshot{State: state.Idle, Since: base}
		}
	}
	chips := ansi.Strip(m.statusChips())
	if !strings.Contains(chips, "↻1 ") || !strings.Contains(chips, "●1") || strings.Contains(chips, "●2") {
		t.Errorf("the loop is counted under ↻ and nowhere else: %q", chips)
	}
	if counted, total := chipSum(chips), len(m.viewOrder()); counted != total {
		t.Errorf("the chips count %d sessions, the board has %d: %q", counted, total, chips)
	}
	// The loop's row wears its age, and says whether a turn is in flight.
	if view := ansi.Strip(m.View()); !strings.Contains(view, "circling · idle") {
		t.Errorf("an idle loop says so:\n%s", view)
	}
	// Unread is owed: no "all calm" beside it.
	for i := range m.sessions {
		if m.sessions[i].Snap.State != state.Idle {
			m.sessions[i].Snap = state.Snapshot{State: state.Idle, Since: m.now}
		}
	}
	m.trails[api] = journey.Trail{}
	chips = ansi.Strip(m.statusChips())
	if strings.Contains(chips, "all calm") || !strings.Contains(chips, "unread") {
		t.Errorf("unread is owed: %q", chips)
	}
}

// A narrower green run does not end a loop: "pytest tests/auth 312✓"
// between two red "pytest" runs is a subset, whatever its count says.
func TestANarrowerGreenRunKeepsTheLoop(t *testing.T) {
	base := fixtureBase
	red := func(i int) journey.Leg {
		return journey.Leg{Class: journey.Test, Label: "pytest", Start: base.Add(time.Duration(i) * time.Hour), End: base.Add(time.Duration(i)*time.Hour + time.Minute),
			Waypoints: []journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "310 passed · 2 failed", Short: "310✓ 2✗"}, {Kind: journey.WaypointTestFail, Text: "test_logout", Runs: i + 1}}}
	}
	tr := journey.Trail{Legs: []journey.Leg{red(0), red(1), red(2),
		{Class: journey.Test, Label: "pytest tests/auth", Start: base.Add(3 * time.Hour), End: base.Add(3*time.Hour + time.Minute),
			Waypoints: []journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "312 passed", Short: "312✓"}}}}}
	if _, _, ok := circling(tr); !ok {
		t.Error("a narrower green run ended the loop")
	}
	if at := circlingSince(tr); !at.Equal(base) {
		t.Errorf("the loop began with its first red run, not %v", at)
	}
	tr.Legs[3].Label = "pytest"
	if _, _, ok := circling(tr); ok {
		t.Error("a green run of the same suite should end the loop")
	}
}

// x refuses what owes an alarm, out loud; a hide names the way back and
// moves the selection to the neighbour; the archive row keeps the name.
func TestHidingIsHonest(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	infra := sessionKey("s-infra")
	m.point(infra)
	press(m, "x")
	if m.hidden[infra] || !strings.Contains(m.note, "is asking · it stays") || m.selectedKey != infra {
		t.Errorf("x on a question: hidden %v, note %q, selected %q", m.hidden[infra], m.note, m.selectedKey)
	}
	if strings.Contains(ansi.Strip(m.View()), "x unhide") {
		t.Error("the footer offers x unhide with nothing hidden")
	}
	// The neighbour takes the selection, not the first column.
	order := m.viewOrder()
	webapp := sessionKey("s-webapp")
	pos := 0
	for i, idx := range order {
		if m.sessions[idx].Info.Key() == webapp {
			pos = i
		}
	}
	m.point(webapp)
	press(m, "x")
	if !m.hidden[webapp] || !strings.Contains(m.note, "webapp hidden · A, then x") {
		t.Errorf("hide: hidden %v, note %q", m.hidden[webapp], m.note)
	}
	if after := m.viewOrder(); m.selectedKey != m.sessions[after[min(pos, len(after)-1)]].Info.Key() {
		t.Errorf("the selection should land on the neighbour, not %q", m.selectedKey)
	}
	if strip := ansi.Strip(m.View()); !strings.Contains(strip, "1 hidden · A, then x") {
		t.Errorf("the strip should say the way back:\n%s", strip)
	}
	press(m, "A")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, `webapp · "flake in the checkout suite"`) || strings.Contains(view, "hidden · flake") {
		t.Errorf("the archive row keeps the name:\n%s", view)
	}
	if !strings.Contains(view, "⌁ dev:2.1") || !strings.Contains(view, "archive 5 · 1 hidden") {
		t.Errorf("the archive says where it lives and counts it:\n%s", view)
	}
	// An empty archive under the hidden group says so.
	press(m, "A")
	kept := m.sessions[:0]
	for _, s := range m.sessions {
		if s.Live {
			kept = append(kept, s)
		}
	}
	m.SetSessions(kept, m.now)
	press(m, "A")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "no finished sessions yet") || !strings.Contains(view, "archive 0 · 1 hidden") {
		t.Errorf("the empty archive names itself under the hidden:\n%s", view)
	}
}

// chipSum adds up the sessions the header's state chips count.
func chipSum(chips string) int {
	n := 0
	for _, f := range strings.Fields(chips) {
		r := []rune(f)
		if len(r) < 2 || !strings.ContainsRune("▲◍↻⊘●○", r[0]) {
			continue
		}
		v := 0
		for _, c := range r[1:] {
			if c < '0' || c > '9' {
				break
			}
			v = v*10 + int(c-'0')
		}
		n += v
	}
	return n
}

// Two live sessions in one tmux session: the tag names the pane, since the
// name alone is the same string on both.
func TestSharedTmuxNamesThePane(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "⌁ dev:1.0") || !strings.Contains(view, "⌁ dev:2.1") {
		t.Errorf("api and webapp share dev: the pane tells them apart:\n%s", view)
	}
	if !strings.Contains(view, "no pane") {
		t.Errorf("the paneless session says so where the tag goes:\n%s", view)
	}
	m.point(sessionKey("s-webapp"))
	press(m, "x")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "⌁ dev ") && !strings.Contains(view, "⌁ dev\n") {
		t.Errorf("alone in dev, the tag is the name again:\n%s", view)
	}
}

// A lead parked on its agents past the quiet mark owes more than a session
// that is fine, and sorts above it.
func TestAParkedLeadOwesMore(t *testing.T) {
	m := boardModel(152, 40)
	base := fixtureBase
	api := sessionKey("s-api")
	tr := m.trails[api]
	head := len(tr.Legs) - 1
	tr.Legs[head].End = m.now.Add(-15 * time.Minute)
	tr.Branches = []journey.Branch{{ToolUseID: "a", Label: "scout", Start: m.now.Add(-12 * time.Minute), AfterLeg: head}}
	m.trails[api] = tr
	webapp := sessionKey("s-webapp")
	var parked, fine fleet.Session
	for _, s := range m.sessions {
		switch s.Info.Key() {
		case api:
			parked = s
		case webapp:
			fine = s
		}
	}
	_ = base
	if m.obligation(parked) >= m.obligation(fine) {
		t.Errorf("parked %d should sort above working %d", m.obligation(parked), m.obligation(fine))
	}
}
