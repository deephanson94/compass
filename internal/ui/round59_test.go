package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/tmuxop"
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

// The board keeps the question and ranks it honestly: under everything
// happening today — the session asking you now and the one that is working
// — and over what is merely done. Two sessions waiting sort
// oldest-question-first, as every other alarm does.
func TestAWaitingSessionKeepsItsColumnUnderTodaysAlarms(t *testing.T) {
	m := r59Model(t)
	if got, want := strings.Join(orderedNames(m), ","), "api,etl,infra,docs"; got != want {
		t.Fatalf("board order = %s, want %s — the waiting go under today's alarm and today's work", got, want)
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
	if !strings.Contains(m.note, "2 waiting hidden · A, then x") {
		t.Errorf("note = %q, want the count and the route back in the grammar its neighbours use", m.note)
	}
	if strings.Contains(m.note, "then X") {
		t.Errorf("note = %q promises an undo X does not keep: it brings back every hidden row", m.note)
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
	if !strings.Contains(m.note, "infra stays") {
		t.Errorf("note = %q, want the name of the row that stayed", m.note)
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

// ---- the panel's round-59 folds ----

// THE ONE THING, named by two reviewers independently: the header counted
// the pile as alarms. `▲4 9d` stood on every frame where one session was
// asking, six minutes in, and three were a dismissible pile — and four
// keypresses that answered nothing walked it down to `▲1 6m`. The pile
// keeps its own chip, in the form the quota deaths wear.
func TestTheHeaderCountsTodaysAlarmsApartFromThePile(t *testing.T) {
	m := r59Model(t)
	chips := ansi.Strip(m.statusChips())
	if !strings.Contains(chips, "▲1 4m") {
		t.Errorf("chips = %q, want one alarm of the moment with its own wait", chips)
	}
	if !strings.Contains(chips, "▲2 unanswered 3d") {
		t.Errorf("chips = %q, want the pile counted apart, in its own word, at the oldest question's age", chips)
	}
	if strings.Contains(chips, "▲3") || strings.Contains(chips, "▲4") {
		t.Errorf("chips = %q: the pile is still being counted as alarms", chips)
	}
	// And the tab title, which is read from across the room, says the same.
	if got := m.needsYouCount(); got != 1 {
		t.Errorf("needsYouCount = %d, want 1 — the badge is today's alarms", got)
	}
}

// A board of nothing but questions from other days is not "all calm", and
// it is not an alarm either: the chip says what it is, and the word that
// would contradict it is gone.
func TestABoardOfOnlyWaitingIsNeitherCalmNorAnAlarm(t *testing.T) {
	forceASCII(t)
	m := New(nil)
	m.Update(tea.WindowSizeMsg{Width: 152, Height: 40})
	only := []fleet.Session{
		waiting("s-docs", "docs", "rewrite the install guide", fixtureBase.Add(-26*time.Hour)),
		waiting("s-infra", "infra", "split the vpc module", fixtureBase.Add(-3*24*time.Hour)),
	}
	fleet.SortFleet(only)
	m.Update(fleetMsg{sessions: only, at: fixtureBase, trails: map[string]journey.Trail{}, paired: true})
	chips := ansi.Strip(m.statusChips())
	if !strings.Contains(chips, "▲2 unanswered 3d") {
		t.Errorf("chips = %q, want the pile's own chip", chips)
	}
	if strings.Contains(chips, "all calm") {
		t.Errorf("chips = %q: `all calm` beside two unanswered questions is the header contradicting itself", chips)
	}
}

// The wall the fleet-hygiene reviewer measured: an archive where one
// session in four ended on a question comes back as ten amber rows on a
// 24-row terminal, with the one session that is working scrolled off. The
// pile never takes the columns of what is happening today.
func TestThePileNeverTakesTheColumnsOfTodaysWork(t *testing.T) {
	forceASCII(t)
	m := New(nil)
	m.Update(tea.WindowSizeMsg{Width: 152, Height: 40})
	ss := []fleet.Session{sess("s-etl", "etl", "/home/user/etl", "main", "dedupe the nightly load",
		state.Working, fixtureBase.Add(-20*time.Minute), journey.Build, "", "tool call in flight", "thinking…")}
	for i := 0; i < 12; i++ {
		ss = append(ss, waiting(fmt.Sprintf("s-old%02d", i), fmt.Sprintf("old%02d", i),
			"a question from another day", fixtureBase.Add(-time.Duration(i+2)*24*time.Hour)))
	}
	fleet.SortFleet(ss)
	m.Update(fleetMsg{sessions: ss, at: fixtureBase, trails: map[string]journey.Trail{}, paired: true})

	order := orderedNames(m)
	if len(order) == 0 || order[0] != "etl" {
		t.Fatalf("board order = %v, want the working session first: twelve old questions took the board", order)
	}
	n, _ := boardColumns(m.width-2*edgePad, len(order))
	keys := m.boardKeys(n)
	if len(keys) == 0 || keys[0] != sessionKey("s-etl") {
		t.Errorf("the first column is %v, want the session that is working", keys)
	}
}

// A row held by an old question says so. Four rows reading `▲ needs you`
// where one was asked six minutes ago and three days ago told the person
// nothing about which was which.
func TestAWaitingRowSaysItIsWaiting(t *testing.T) {
	m := r59Model(t)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "unanswered") {
		t.Fatalf("no row says the question is unanswered:\n%s", view)
	}
	for _, s := range m.sessions {
		got := headline(s)
		want := "needs you"
		if s.Waiting {
			want = "unanswered"
		}
		if s.Snap.State == state.Working {
			want = ""
		}
		if got != want {
			t.Errorf("%s: headline = %q, want %q", sessionName(s.Info), got, want)
		}
	}
}

// The group that holds the whole pile wears its alarm, like every other
// group — and with the echo comes the clock's silence: the header over a
// three-day-old question was reporting the group's newest row.
func TestTheGroupHoldingThePileWearsItsMark(t *testing.T) {
	forceASCII(t)
	m := New(nil)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	// The sessions that are live today sit in a tmux group, so `elsewhere`
	// holds the questions and nothing else: with an alarm of the moment in
	// it the echo is a `▲` either way, and the pin measured nothing (round
	// 60's finding on this very fold).
	panes := map[string]tmuxop.Pane{
		sessionKey("s-api"): {Target: "work:0.0"},
		sessionKey("s-etl"): {Target: "work:1.0"},
	}
	m.Update(panesMsg{panes: panes, against: len(r59Fleet())})
	m.Update(fleetMsg{sessions: r59Fleet(), at: fixtureBase, trails: map[string]journey.Trail{}, paired: true})
	var pile fleetGroup
	for _, g := range m.liveGroups() {
		if g.name == "elsewhere" {
			pile = g
		}
	}
	if len(pile.entries) == 0 {
		t.Fatalf("no elsewhere group: the paneless questions are grouped somewhere else")
	}
	for _, i := range pile.entries {
		if !m.sessions[i].Waiting {
			t.Fatalf("the elsewhere group holds %s, which is not waiting: the fixture measures nothing",
				sessionName(m.sessions[i].Info))
		}
	}
	if got := m.groupEcho(pile); got != fleet.GlyphNeedsYou {
		t.Errorf("the group holding every unanswered question echoes %q, want %q", got, fleet.GlyphNeedsYou)
	}
}

// The key this round added is named at every width. The narrow help — the
// width the dogfood happened at — carried `x / A` and nothing else, so `X`
// was learnable only by pressing it.
func TestTheNarrowHelpNamesTheSweep(t *testing.T) {
	forceASCII(t)
	for _, w := range []int{80, 100, 120, 152, 220} {
		lines := helpLinesWith(w, 40, helpOpts{board: w >= 120})
		found := false
		for _, l := range lines {
			if strings.Contains(ansi.Strip(l), "X") && strings.Contains(ansi.Strip(l), "hide a session") {
				found = true
			}
		}
		if !found {
			t.Errorf("%d columns: the help never names X:\n%s", w, strings.Join(lines, "\n"))
		}
	}
}

// And the row that teaches `g` says what `g` now does: it steps over a
// nine-day-old question to take the one asked six minutes ago.
func TestTheHelpRowForTheGrabSaysWhichQuestionItTakes(t *testing.T) {
	forceASCII(t)
	lines := helpLinesWith(152, 40, helpOpts{board: true})
	row := ""
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(ansi.Strip(l)), "g ") {
			row = ansi.Strip(l)
		}
	}
	if row == "" {
		t.Fatalf("no help row for g:\n%s", strings.Join(lines, "\n"))
	}
	if strings.Contains(row, "the oldest") {
		t.Errorf("row = %q: `g` takes today's question first, not the oldest", row)
	}
	if !strings.Contains(row, "today's question first") {
		t.Errorf("row = %q, want the row to say which question g takes", row)
	}
}

// The archive's own header names both keys, because `X` there brings back
// everything hidden — the rows put down one at a time with `x` included.
// The board's note no longer promises that undo.
func TestTheArchiveNamesTheKeyThatBringsThemAllBack(t *testing.T) {
	forceASCII(t)
	// On the frame, at the width that pays for it: the header was asserted
	// against the Go constant and stood green while eighty columns cut the
	// sentence to a bare `X…` welded against the group's own mark (round
	// 60). Eighty is where the label column is thirty cells.
	sc := sceneLeftBehind()
	for _, w := range []int{80, 100, 152} {
		// Tall enough that the list draws the group whatever order it
		// sorts in: the clip this pins is a function of the label column's
		// width, not of the rows.
		m := sceneModel(sc, w, 40)
		poll(m, sc)
		pressKey(m, "X")
		pressKey(m, "A")
		poll(m, sc)
		row := ""
		for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			if strings.Contains(l, "hidden ·") {
				row = l
			}
		}
		if row == "" {
			t.Fatalf("%d columns: the archive draws no header for what the sweep hid:\n%s", w, ansi.Strip(m.View()))
		}
		for _, want := range []string{"x one back", "X all"} {
			if !strings.Contains(row, want) {
				t.Errorf("%d columns: the header %q does not name %q where the key is pressed", w, strings.TrimSpace(row), want)
			}
		}
		if strings.Contains(row, "…") {
			t.Errorf("%d columns: the header is cut mid-sentence: %q", w, strings.TrimSpace(row))
		}
	}
}
