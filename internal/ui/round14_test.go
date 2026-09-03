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
	"github.com/deephanson94/compass/internal/transcript"
)

// Round fourteen: the seven operators on identity, the dead panel, and
// the places the read-line and the digits had not reached.

// The reply panel names the session by the digit its row wears, not by
// its position on the board.
func TestThePanelNamesTheRowsDigit(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	api := sessionKey("s-api")
	// api finishes and is read: it sorts to the strip, digit intact.
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == api {
			m.sessions[i].Snap = state.Snapshot{State: state.Idle, Since: m.now}
		}
	}
	m.markSeen(api)
	m.trails[api] = journey.Trail{}
	m.point(api)
	press(m, "r")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "reply to 2 · api") {
		t.Errorf("the panel should name api by its digit:\n%s", view)
	}
}

// A dead session's remedy is the bytes typed: the panel offers "/login"
// under its own head, the trace quotes it, and the quota line is named as
// a turn rather than offered as a reply.
func TestTheRemedyIsTheBytesTyped(t *testing.T) {
	forceASCII(t)
	m := followModel(152, 40)
	m.runner = &recordingTmux{}
	dead := sessionKey("s-api")
	deadOnAPI(m, dead)
	m.point(dead)
	press(m, "r")
	view := ansi.Strip(m.View())
	for _, want := range []string{"remedy · typed into the pane and entered", "1  /login", "lines · start a turn — only once the quota is back"} {
		if !strings.Contains(view, want) {
			t.Errorf("the panel should say %q:\n%s", want, view)
		}
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	if cmd == nil {
		t.Fatal("1 sent nothing")
	}
	m.Update(cmd())
	if !strings.Contains(m.note, `↪ sent "/login"`) {
		t.Errorf("the trace should quote the bytes typed: %q", m.note)
	}
}

// A dead session whose last leg closed still ends its trail on the refusal.
func TestADeadTrailEndsOnTheRefusal(t *testing.T) {
	base := fixtureBase
	tr := journey.Trail{Legs: []journey.Leg{{Class: journey.Build, Label: "invoice.py", Start: base, End: base.Add(20 * time.Minute), Files: []string{"invoice.py"}}}}
	got := ansi.Strip(RenderTrail(tr, TrailOpts{Now: base.Add(40 * time.Minute), Width: 60, Height: 20, Level: levelTrail, Cursor: -1,
		HeadDead: true, HeadState: state.NeedsYou, Head: "Please run /login · API Error: 403", HeadSince: base.Add(22 * time.Minute)}))
	if !strings.Contains(got, "⊘ api") || !strings.Contains(got, "dead 18m") {
		t.Errorf("the trail should end on the refusal:\n%s", got)
	}
}

// A working session with no leg yet shows its present on the trail and
// the fleet row alike, not a placeholder under a completed-leg glyph.
func TestAWorkingSessionWithNoLegsShowsItsPresent(t *testing.T) {
	row := ansi.Strip(bareHeadRow(TrailOpts{HeadState: state.Working, HeadActivity: "thinking…", HeadClass: "scout",
		HeadSince: fixtureBase, Now: fixtureBase.Add(40 * time.Second)}, 60))
	if !strings.HasPrefix(row, "● scout") || !strings.Contains(row, "thinking…") || !strings.Contains(row, "for 40s") {
		t.Errorf("the bare HEAD row should be the present: %q", row)
	}
}

// A fleet of one opens on the session view, and its footer offers no key
// that moves between sessions.
func TestAFleetOfOneOpensOnTheSession(t *testing.T) {
	forceASCII(t)
	m := New(nil)
	m.SetSize(152, 40)
	s := sess("s-hello", "hello", "/home/user/hello", "main", "add a --version flag", state.Working, fixtureBase, journey.Scout, "", "tool call in flight", "thinking…")
	m.Update(fleetMsg{sessions: []fleet.Session{s}, at: fixtureBase.Add(40 * time.Second), trails: map[string]journey.Trail{}})
	if m.level != levelWaypoints {
		t.Fatalf("one session should open on the session view, not level %d", m.level)
	}
	foot := ansi.Strip(m.View())
	for _, no := range []string{"h/l session", "esc board", "h/l columns"} {
		if strings.Contains(foot, no) {
			t.Errorf("a fleet of one offers %q:\n%s", no, foot)
		}
	}
	press(m, "x")
	if m.note != "the only session stays" {
		t.Errorf("note = %q", m.note)
	}
}

// The group header echoes the worst row as the board ranks it.
func TestTheGroupEchoIsTheWorstRow(t *testing.T) {
	m := boardModel(152, 40)
	deadOnAPI(m, sessionKey("s-api"))
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == sessionKey("s-webapp") {
			m.sessions[i].Snap = state.Snapshot{State: state.Stuck, Since: fixtureBase}
		}
	}
	g := fleetGroup{name: "dev", entries: []int{}}
	for i, s := range m.sessions {
		if k := s.Info.Key(); k == sessionKey("s-api") || k == sessionKey("s-webapp") {
			g.entries = append(g.entries, i)
		}
	}
	if echo := m.groupEcho(g); echo != "◍" {
		t.Errorf("a group of a hang and a corpse echoes the hang, not %q", echo)
	}
}

// The fold counts sessions: a second line that opens with a state glyph is
// not an entry, and a loop's or a dead session's row is.
func TestTheFoldCountsSessions(t *testing.T) {
	lines := []string{"▸1 ▲ infra   needs you  7m", "    ◍ build  Bash: py… silent 4m", " 7 ↻ api   circling  1h", " 3 ⊘ billing  quota 18m", " ⌁ work   ▲", "    ● fix  tokens.py"}
	if n := countEntries(lines); n != 3 {
		t.Errorf("counted %d entries, want 3", n)
	}
}

// Typing a search that moves the selection does not read the session it
// lands on.
func TestASearchDoesNotRead(t *testing.T) {
	forceASCII(t)
	m := groupedModel(100, 30)
	m.seen = map[string]time.Time{}
	m.point(sessionKey("s-infra"))
	press(m, "/")
	for _, r := range "flake" {
		press(m, string(r))
	}
	if m.selectedKey != sessionKey("s-webapp") {
		t.Fatalf("the search should have moved to webapp, not %q", m.selectedKey)
	}
	if _, read := m.seen[sessionKey("s-webapp")]; read {
		t.Error("a search moving the selection marked the session read")
	}
}

// A loop's verdict leads with its red, whatever the newest narrower run
// said, and names the green run as the trailing clause.
func TestALoopsVerdictLeadsWithTheRed(t *testing.T) {
	base := fixtureBase
	red := func(i int) journey.Leg {
		return journey.Leg{Class: journey.Test, Label: "pytest", Start: base.Add(time.Duration(i) * time.Hour), End: base.Add(time.Duration(i)*time.Hour + time.Minute),
			Waypoints: []journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "310 passed · 2 failed", Short: "310✓ 2✗"}, {Kind: journey.WaypointTestFail, Text: "test_logout", Runs: i + 1}}}
	}
	tr := journey.Trail{Legs: []journey.Leg{red(0), red(1), red(2),
		{Class: journey.Test, Label: "pytest tests/auth", Start: base.Add(3 * time.Hour), End: base.Add(3*time.Hour + time.Minute),
			Waypoints: []journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "312 passed", Short: "312✓"}}}}}
	parts := verdictParts(tr, base.Add(4*time.Hour), false)
	joined := strings.Join(parts, " · ")
	if !strings.HasPrefix(joined, "✗ red 310✓ 2✗") || !strings.Contains(joined, "3rd failure") || !strings.HasSuffix(joined, "pytest tests/auth 312✓ since") {
		t.Errorf("a loop's verdict = %q", joined)
	}
}

// The read-line stands among a leg's lanes: before the first lane that
// left after the look, so a lane back since is below it.
func TestTheReadLineStandsAmongTheLanes(t *testing.T) {
	base := fixtureBase
	tr := journey.Trail{Legs: []journey.Leg{
		{Class: journey.Design, Label: "a contract", Start: base, End: base.Add(20 * time.Minute), Files: []string{"c.md"}},
		{Class: journey.Docs, Label: "SKILL.md", Start: base.Add(30 * time.Minute), End: base.Add(40 * time.Minute), Files: []string{"SKILL.md"}},
	}, Branches: []journey.Branch{{ToolUseID: "a", Label: "draft the failure section", Start: base.Add(10 * time.Minute), End: base.Add(15 * time.Minute), Done: true, AfterLeg: 0}}}
	got := ansi.Strip(RenderTrail(tr, TrailOpts{Now: base.Add(time.Hour), Width: 70, Height: 20, Level: levelTrail, Cursor: -1, Looked: base.Add(5 * time.Minute)}))
	look, lane := strings.Index(got, "you were here"), strings.Index(got, "◈ draft")
	if look < 0 || lane < 0 || look > lane {
		t.Errorf("the read-line should stand before the lane that left after it:\n%s", got)
	}
	if strings.Count(got, "you were here") != 1 {
		t.Errorf("the read-line drew twice:\n%s", got)
	}
}

// Closing the session view is the look: the digest stops billing.
func TestClosingTheSessionCommitsTheLook(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	api := sessionKey("s-api")
	m.seen = map[string]time.Time{api: fixtureBase.Add(10 * time.Minute)}
	m.point(api)
	pressTab(m)
	press(m, "esc")
	if m.level != levelBoard {
		t.Fatalf("esc should return to the board, not level %d", m.level)
	}
	if row := m.boardDelta(api, m.sessions[rowFor(t, m, api).sess], 60); row != "" {
		t.Errorf("the digest should be spent after the session was closed: %q", row)
	}
}

// A footer sheds the attach parenthetical before its keys or its note.
func TestTheFooterShedsTheParentheticalFirst(t *testing.T) {
	forceASCII(t)
	m := groupedModel(80, 24)
	m.note = "webapp hidden · A, then x"
	foot := ansi.Strip(m.footerLine(76))
	if strings.Contains(foot, "prefix d") || !strings.Contains(foot, "A, then x") {
		t.Errorf("the parenthetical should go before the note is cut: %q", foot)
	}
}

// Below the board's width the list tags namesakes by pane on the row's
// third line, and the hide note names the digit and the pane.
func TestTheNarrowListTagsNamesakes(t *testing.T) {
	forceASCII(t)
	m := boardModel(100, 30)
	if view := ansi.Strip(m.View()); !strings.Contains(view, "⌁ dev:1.0") || !strings.Contains(view, "⌁ dev:2.1") {
		t.Errorf("api and webapp share dev: the list should carry their panes:\n%s", view)
	}
	m.point(sessionKey("s-webapp"))
	press(m, "x")
	if !strings.Contains(m.note, "3 webapp hidden") || !strings.HasSuffix(m.note, "⌁ dev:2.1") {
		t.Errorf("the hide note should name the digit, and the pane last: %q", m.note)
	}
	press(m, "A")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "▸1 ● webapp") {
		t.Errorf("the archive opens on the hidden row and numbers it as drawn:\n%s", view)
	}
}

// The reader says the reply has not come rather than leaving blank rows.
func TestTheReaderSaysNoReplyYet(t *testing.T) {
	ev := []transcript.Event{{Type: transcript.EventUser, Timestamp: fixtureBase, Text: "add a --version flag"}}
	doc := readerDoc(ev, ReaderOpts{Width: 60})
	found := false
	for _, l := range doc {
		if strings.Contains(l.text, "⋯ no reply yet") {
			found = true
		}
	}
	if !found {
		t.Error("a conversation ending on your turn should say the reply has not come")
	}
}

// Small helpers of the round.
func TestRoundFourteenHelpers(t *testing.T) {
	if got := shedClauses("↑ 143 legs · 22h · 16⚑ 10✗ 2⟲ · on you 1h", 30); got != "↑ 143 legs · 22h · 16⚑ 10✗ 2⟲" {
		t.Errorf("shedClauses = %q", got)
	}
	if got := compactOverlap("⚠ webapp and api both touched tokens.py in the last 20m"); got != "⚠ tokens.py · webapp, api · 20m" {
		t.Errorf("compactOverlap = %q", got)
	}
	if got := compactOverlap("⚠ auth and etl are both failing test_logout"); got != "⚠ test_logout · auth, etl" {
		t.Errorf("compactOverlap = %q", got)
	}
	if !isDetailRow("│▸ └ ✗ test_logout · 10th failure") {
		t.Error("a detail row with the cursor on it is still a detail row")
	}
	if w := bandWidth(148, 1, 35); w != boardColMax {
		t.Errorf("a lone column under a full band is %d wide, want the cap", w)
	}
	// The narrow help drops `m` (the mirror needs the board's width) and
	// keeps the search when the deck's own footer is offering it.
	help := strings.Join(helpLinesWith(76, 19, helpOpts{keymap: "j/k move · / search · ? help · q quit"}), "\n")
	if !strings.Contains(help, "/ n N") || strings.Contains(help, "live tmux pane") {
		t.Errorf("the narrow help keeps / and drops m:\n%s", help)
	}
}
