package ui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/tmuxop"
	"github.com/deephanson94/compass/internal/transcript"
)

// Second real look at the session view: the four things asked.

// The card says where the keys are in the help's own words — session,
// reader — not a level number. One Tab from the board read "[Lv2]", and
// the person pressing it asked whether that was expected.
func TestTheCardSaysWhereTheKeysAre(t *testing.T) {
	forceASCII(t)

	m := boardModel(152, 40)
	pressTab(m)
	if got := m.View(); !strings.Contains(got, "[session]") || strings.Contains(got, "[Lv") {
		t.Errorf("one Tab from the board should read [session]:\n%s", got)
	}
	pressTab(m)
	if got := m.View(); !strings.Contains(got, "[reader]") || strings.Contains(got, "[Lv") {
		t.Errorf("the second Tab should read [reader]:\n%s", got)
	}

	// And the narrow deck, which has no board, names its own depths.
	n := boardModel(100, 30)
	for _, want := range []string{"[trail]", "[legs]", "[reader]"} {
		if got := n.trailTitle(60); !strings.Contains(got, want) {
			t.Errorf("want %q in the title: %q", want, got)
		}
		pressTab(n)
	}
}

// `[` and `]` in the reader land on a turn of yours and say so: the turn
// wears the inverse bar, the title names it, and the footer counts it —
// before, the page moved and nothing on it said what to.
func TestReaderChaptersMarkTheTurn(t *testing.T) {
	forceASCII(t)

	m := followModel(160, 30)
	base := fixtureBase
	var ev []transcript.Event
	for i := 0; i < 3; i++ {
		at := base.Add(time.Duration(i) * 10 * time.Minute)
		ev = append(ev,
			transcript.Event{Type: transcript.EventUser, Timestamp: at, Text: "turn " + string(rune('A'+i))},
			transcript.Event{Type: transcript.EventAssistant, Timestamp: at.Add(time.Minute), Text: strings.Repeat("words of the answer ", 40)},
		)
	}
	m.SetEvents(ev)
	toLv3(m)
	m.scroll = 0
	press(m, "]")
	if !strings.HasPrefix(m.note, "❯ ") || !strings.Contains(m.note, `"turn B"`) {
		t.Errorf("] should say which turn it landed on: %q", m.note)
	}
	doc := m.doc(m.readerWidth())
	if m.anchor < 0 || !strings.HasPrefix(doc[m.anchor].text, "❯ turn B") {
		t.Errorf("] should mark the turn it landed on; anchor %d", m.anchor)
	}
	if m.anchorText != "turn B" {
		t.Errorf("the title should name the turn without its clock: %q", m.anchorText)
	}
	press(m, "[")
	if !strings.Contains(m.note, `"turn A"`) || m.anchor < 0 || !strings.HasPrefix(doc[m.anchor].text, "❯ turn A") {
		t.Errorf("[ should mark the earlier turn: note %q anchor %d", m.note, m.anchor)
	}
}

// A test that has failed before says so: "✗ test_x · 3rd time" is the
// trail's plainest sign of a loop, at Lv2 and on the wide row alike.
func TestRepeatedFailuresSayHowManyTimes(t *testing.T) {
	forceASCII(t)

	base := fixtureBase
	leg := func(at time.Time, runs int) journey.Leg {
		return journey.Leg{Class: journey.Test, Label: "pytest", Start: at, End: at.Add(time.Minute),
			Waypoints: []journey.Waypoint{
				{Kind: journey.WaypointTestRun, Text: "18 passed · 1 failed", Short: "18✓ 1✗", At: at},
				{Kind: journey.WaypointTestFail, Text: "test_refresh_expired_token", Runs: runs, At: at},
			}}
	}
	tr := journey.Trail{Legs: []journey.Leg{leg(base, 1), leg(base.Add(10*time.Minute), 2), leg(base.Add(20*time.Minute), 3)}}
	tr.Legs[2].Current = true
	got := renderLv(tr, base.Add(30*time.Minute), 2, 60, 30)
	for _, want := range []string{"test_refresh_expired_token · 2nd failure", "test_refresh_expired_token · 3rd failure"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q at Lv2:\n%s", want, got)
		}
	}
	if strings.Count(got, "1st failure") > 0 {
		t.Errorf("the first failure is not a repeat:\n%s", got)
	}
	// On a wide row the detail rides inline, and says it there too.
	if got := renderLv(tr, base.Add(30*time.Minute), 1, 120, 30); !strings.Contains(got, "· 3rd failure") {
		t.Errorf("the wide row should carry the count:\n%s", got)
	}
	for n, want := range map[int]string{1: "1st", 2: "2nd", 3: "3rd", 4: "4th", 11: "11th", 12: "12th", 13: "13th", 21: "21st", 22: "22nd", 103: "103rd"} {
		if got := ordinal(n); got != want {
			t.Errorf("ordinal(%d) = %q, want %q", n, got, want)
		}
	}
}

// A compaction is a rail row with its clock, and the day's totals count it:
// a session compacted twice is working from a summary of a summary, and
// that is worth knowing before reading its trail closely.
func TestCompactionsMarkTheRail(t *testing.T) {
	forceASCII(t)

	base := fixtureBase
	tr := hourlyTrail(base, 6)
	tr.Compactions = []time.Time{base.Add(2*time.Hour + 30*time.Minute), base.Add(4*time.Hour + 30*time.Minute)}
	now := base.Add(7 * time.Hour)
	got := renderLv(tr, now, 1, 60, 40)
	for _, want := range []string{"⟲ context compacted " + tr.Compactions[0].Local().Format("15:04"), "⟲ context compacted " + tr.Compactions[1].Local().Format("15:04")} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q on the rail:\n%s", want, got)
		}
	}
	if !strings.Contains(trailDay(tr, now, false), "2 compactions") {
		t.Errorf("the day's totals should count them: %q", trailDay(tr, now, false))
	}
	if !strings.Contains(trailDay(tr, now, true), "2⟲") {
		t.Errorf("the compact totals should count them: %q", trailDay(tr, now, true))
	}
	if !strings.Contains(foldTotals(tr, 3, false), "2 compactions") {
		t.Errorf("the fold row should count them: %q", foldTotals(tr, 3, false))
	}
	// The rail is plain where nothing was folded.
	tr.Compactions = nil
	if got := renderLv(tr, now, 1, 60, 40); strings.Contains(got, "compacted") {
		t.Error("a trail with no compactions drew one")
	}
}

// recordingTmux is a tmux that remembers what it was asked to do.
type recordingTmux struct{ calls [][]string }

func (r *recordingTmux) Output(args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	return nil, nil
}

// `r` puts the quick replies on the footer; a digit types one into the
// selected session's pane and says so; anything else sends nothing.
// Read-only keeps the second write to itself as it keeps the first.
func TestQuickRepliesGoToTheSessionsPane(t *testing.T) {
	forceASCII(t)

	build := func() (*Model, *recordingTmux) {
		m := followModel(152, 40) // s-api in dev:1.0, pane %5
		rec := &recordingTmux{}
		m.runner = rec
		return m, rec
	}

	m, rec := build()
	press(m, "r")
	if !m.replying {
		t.Fatal("r did not offer the replies")
	}
	view := m.View()
	for _, want := range []string{"┌ reply to 2 · api · ⌁ dev:1.0", "1  please continue", "2  report status", "3  you were stuck", "esc closes", "● working for"} {
		if !strings.Contains(view, want) {
			t.Errorf("the panel should offer %q:\n%s", want, view)
		}
	}
	// A panel, not a takeover: the deck is still there around it.
	if !strings.Contains(view, "READER") || !strings.Contains(view, "reply: 1–4 sends · t types a line") {
		t.Errorf("the deck should stay under the panel, and the footer name the keys:\n%s", view)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	if m.replying {
		t.Error("the menu should close once a reply is picked")
	}
	if cmd == nil {
		t.Fatal("picking a reply produced no command")
	}
	m.Update(cmd())
	want := [][]string{
		{"send-keys", "-t", "%5", "-l", "please continue"},
		{"send-keys", "-t", "%5", "Enter"},
	}
	if len(rec.calls) != len(want) {
		t.Fatalf("tmux was asked %v, want %v", rec.calls, want)
	}
	for i := range want {
		if strings.Join(rec.calls[i], " ") != strings.Join(want[i], " ") {
			t.Errorf("call %d: %v, want %v", i, rec.calls[i], want[i])
		}
	}
	if !strings.Contains(m.note, "sent to") || !strings.Contains(m.note, "dev:1.0") || !strings.Contains(m.note, "please continue") {
		t.Errorf("the footer should say what went where: %q", m.note)
	}

	// Esc sends nothing; neither does a key that means something else.
	m, rec = build()
	press(m, "r")
	press(m, "esc")
	press(m, "r")
	press(m, "j")
	if m.replying || len(rec.calls) != 0 {
		t.Errorf("esc or j sent something: %v", rec.calls)
	}
	press(m, "r")
	press(m, "q")
	if m.replying {
		t.Error("q should close the menu")
	}
	press(m, "r")
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd == nil {
		t.Error("ctrl+c should still quit with the menu up")
	} else if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Error("ctrl+c with the menu up did not quit")
	}
	press(m, "r")
	press(m, "7")
	if len(rec.calls) != 0 || !strings.Contains(m.note, "no reply 7") {
		t.Errorf("a digit past the list sent something, or said nothing: %v %q", rec.calls, m.note)
	}

	// Read-only, and a session with no pane, are told why.
	m, rec = build()
	m.readonly = true
	press(m, "r")
	if m.replying || !strings.Contains(m.note, "read-only") {
		t.Errorf("read-only offered the replies: %q", m.note)
	}
	m, rec = build()
	m.SetPanes(nil)
	press(m, "r")
	if m.replying || !strings.Contains(m.note, "no tmux pane") {
		t.Errorf("a paneless session offered the replies: %q", m.note)
	}
	_ = rec

	// The footer names the key where a reply can be sent from.
	m, _ = build()
	m.zoomOut()
	m.zoomOut()
	if foot := m.footerLine(150); !strings.Contains(foot, "r reply") {
		t.Errorf("the board's footer should offer r reply:\n%s", foot)
	}

	// Configured replies replace the stock ones.
	m, _ = build()
	m.replies = []string{"ship it"}
	press(m, "r")
	if view := m.View(); !strings.Contains(view, "1  ship it") || strings.Contains(view, "please continue") {
		t.Errorf("configured replies should be what is offered:\n%s", view)
	}
}

// The segmenter counts a failing test's legs and records the compactions —
// the ui reads both off the trail rather than walking the transcript again.
func TestTheSegmenterCountsRepeatsAndCompactions(t *testing.T) {
	base := fixtureBase
	s := journey.NewSegmenter()
	call := func(id string, at time.Time) transcript.Event {
		return transcript.Event{Type: transcript.EventAssistant, Timestamp: at,
			ToolUses: []transcript.ToolUse{{ID: id, Name: "Bash", Input: json.RawMessage(`{"command":"pytest tests/auth -x"}`)}}}
	}
	result := func(id string, at time.Time) transcript.Event {
		return transcript.Event{Type: transcript.EventUser, Timestamp: at,
			ToolResults: []transcript.ToolResult{{ToolUseID: id, IsError: true,
				Text: "FAILED tests/auth/test_refresh.py::test_refresh_expired_token\n== 1 failed, 19 passed in 1.2s =="}}}
	}
	edit := func(at time.Time) transcript.Event {
		return transcript.Event{Type: transcript.EventAssistant, Timestamp: at,
			ToolUses: []transcript.ToolUse{{ID: "e" + at.String(), Name: "Edit", Input: json.RawMessage(`{"file_path":"src/tokens.py","old_string":"a","new_string":"b"}`)}}}
	}
	for _, ev := range []transcript.Event{
		{Type: transcript.EventUser, Timestamp: base, Text: "fix the refresh bug"},
		call("t1", base.Add(1*time.Minute)), result("t1", base.Add(2*time.Minute)),
		edit(base.Add(3 * time.Minute)), edit(base.Add(4 * time.Minute)), edit(base.Add(5 * time.Minute)),
		call("t2", base.Add(6*time.Minute)), result("t2", base.Add(7*time.Minute)),
		{Type: transcript.EventUser, Timestamp: base.Add(8 * time.Minute), Text: "This session is being continued from a previous conversation that ran out of context. Summary: …"},
		edit(base.Add(9 * time.Minute)), edit(base.Add(10 * time.Minute)), edit(base.Add(11 * time.Minute)),
		call("t3", base.Add(12*time.Minute)), result("t3", base.Add(13*time.Minute)),
	} {
		s.Observe(ev)
	}
	tr := s.Trail()
	var runs []int
	for _, l := range tr.Legs {
		for _, w := range l.Waypoints {
			if w.Kind == journey.WaypointTestFail {
				runs = append(runs, w.Runs)
			}
		}
	}
	if len(runs) != 3 || runs[0] != 1 || runs[1] != 2 || runs[2] != 3 {
		t.Errorf("Runs across three legs = %v, want [1 2 3] (legs: %d)", runs, len(tr.Legs))
	}
	if len(tr.Compactions) != 1 || !tr.Compactions[0].Equal(base.Add(8*time.Minute)) {
		t.Errorf("Compactions = %v, want the one at +8m", tr.Compactions)
	}
	if len(tr.Prompts) != 1 {
		t.Errorf("the compaction became a prompt: %d prompts", len(tr.Prompts))
	}
}

// The time a session spent waiting for your next prompt is added up over
// the trail and said on the board, the day's totals and the prompt row.
// A gap over three hours is you being away and is not counted; a live
// session still waiting adds the open wait.
func TestWaitedOnYouCountsTheGapsBeforeYourPrompts(t *testing.T) {
	forceASCII(t)

	base := fixtureBase
	tr := journey.Trail{
		Prompts: []journey.Prompt{
			{Text: "start", At: base},
			{Text: "now the tests", At: base.Add(50 * time.Minute)},            // 40m after the leg ended
			{Text: "back from lunch", At: base.Add(5 * time.Hour)},             // 4h: away, not waited on
			{Text: "and the docs", At: base.Add(5*time.Hour + 32*time.Minute)}, // 2m: not worth a word
		},
		Legs: []journey.Leg{
			{Class: journey.Build, Label: "tokens.py", Start: base.Add(time.Minute), End: base.Add(10 * time.Minute)},
			{Class: journey.Test, Label: "pytest", Start: base.Add(51 * time.Minute), End: base.Add(60 * time.Minute)},
			{Class: journey.Fix, Label: "expiry", Start: base.Add(5*time.Hour + 1*time.Minute), End: base.Add(5*time.Hour + 30*time.Minute)},
			{Class: journey.Docs, Label: "README.md", Start: base.Add(5*time.Hour + 33*time.Minute), End: base.Add(5*time.Hour + 40*time.Minute), Current: true},
		},
	}
	if got := promptWaits(tr); got != 42*time.Minute {
		t.Errorf("promptWaits = %v, want 42m (40m, and the 2m before the docs)", got)
	}
	now := base.Add(6 * time.Hour)
	idle := fleet.Session{Live: true, Snap: state.Snapshot{State: state.Idle}, Info: fleet.SessionInfo{LastEventAt: base.Add(5*time.Hour + 40*time.Minute)}}
	if got := youWaited(tr, now, idle); got != 62*time.Minute {
		t.Errorf("youWaited with 20m still open = %v, want 1h2m", got)
	}
	working := fleet.Session{Live: true, Snap: state.Snapshot{State: state.Working}, Info: idle.Info}
	if got := youWaited(tr, now, working); got != 42*time.Minute {
		t.Errorf("a working session is not waiting: %v", got)
	}
	if got := boardVerdict(idle, tr, now); !strings.Contains(got, "on you 1h") {
		t.Errorf("the board's verdict should carry the wait: %q", got)
	}
	if got := trailDay(tr, now, false); !strings.Contains(got, "waited on you 42m") {
		t.Errorf("the day's totals should carry the wait: %q", got)
	}
	if got := trailDay(tr, now, true); !strings.Contains(got, "on you 42m") {
		t.Errorf("the compact totals should carry the wait: %q", got)
	}
	got := renderLv(tr, now, 1, 100, 30)
	if !strings.Contains(got, "waited 40m · ") {
		t.Errorf("the prompt row should say how long it waited:\n%s", got)
	}
	if strings.Count(got, "waited") != 1 {
		t.Errorf("only the notable wait under three hours is a word:\n%s", got)
	}
	if got := renderLv(tr, now, 1, 50, 30); strings.Contains(got, "waited") {
		t.Errorf("a narrow row has no room for the wait:\n%s", got)
	}
}

// ---- round nine

// `[` from a fresh page lands on the turn governing where the reader is
// anchored: on a conversation that fits the panel nothing scrolls, and
// "no earlier turn" with a turn in plain sight was false on its face.
func TestChapterFromAFreshPageLandsOnTheGoverningTurn(t *testing.T) {
	forceASCII(t)
	m := followModel(220, 48)
	base := fixtureBase
	m.SetEvents([]transcript.Event{
		{Type: transcript.EventAssistant, Timestamp: base.Add(-time.Minute), Text: "Resuming from the summary."},
		{Type: transcript.EventUser, Timestamp: base, Text: "dedupe the nightly load"},
		{Type: transcript.EventAssistant, Timestamp: base.Add(time.Minute), Text: "Looking at the loader."},
		{Type: transcript.EventAssistant, Timestamp: base.Add(2 * time.Minute), Text: "Wiring the filter."},
	})
	toLv3(m)
	doc := m.doc(m.readerWidth())
	if m.scroll != 0 || len(doc) > m.readerHeight() {
		t.Fatalf("the fixture should fit the panel: scroll %d, %d lines", m.scroll, len(doc))
	}
	m.anchor = len(doc) - 1 // anchored at the present, below the turn
	press(m, "[")
	if m.anchor != 2 || !strings.Contains(m.note, `"dedupe the nightly load"`) {
		t.Errorf("[ should land on the turn above the anchor: anchor %d, note %q", m.anchor, m.note)
	}
	press(m, "[")
	if m.note != "no earlier turn" {
		t.Errorf("a second [ has nothing earlier: %q", m.note)
	}
}

// The board says "on you" only when there is a day to add up: an idle
// session's open wait alone is the age already on the row above.
func TestOnYouNeedsADayToAddUp(t *testing.T) {
	base := fixtureBase
	now := base.Add(2 * time.Hour)
	tr := journey.Trail{
		Prompts: []journey.Prompt{{Text: "start", At: base}},
		Legs:    []journey.Leg{{Class: journey.Build, Label: "x.go", Start: base.Add(time.Minute), End: base.Add(10 * time.Minute)}},
	}
	idle := fleet.Session{Live: true, Snap: state.Snapshot{State: state.Idle}, Info: fleet.SessionInfo{LastEventAt: base.Add(10 * time.Minute)}}
	if got := boardVerdict(idle, tr, now); strings.Contains(got, "on you") {
		t.Errorf("an open wait alone is the row's age, not a clause: %q", got)
	}
	tr.Prompts = append(tr.Prompts, journey.Prompt{Text: "and again", At: base.Add(50 * time.Minute)})
	tr.Legs = append(tr.Legs, journey.Leg{Class: journey.Test, Label: "pytest", Start: base.Add(51 * time.Minute), End: base.Add(60 * time.Minute)})
	idle.Info.LastEventAt = base.Add(60 * time.Minute)
	if got := boardVerdict(idle, tr, now); !strings.Contains(got, "on you 1h today") {
		t.Errorf("a day of waits, the open one included, is the clause: %q", got)
	}
}

// `A` from the archive returns to the board it was pressed from; with
// nothing archived it changes nothing.
func TestArchiveReturnsToTheBoard(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	var live []fleet.Session
	for _, s := range m.sessions {
		if s.Live {
			live = append(live, s)
		}
	}
	m.SetSessions(live, fixtureBase.Add(40*time.Minute))
	press(m, "A")
	if m.level != levelBoard || m.archiveView || m.note != "nothing archived yet" {
		t.Fatalf("with nothing archived A should change nothing: level %d archive %v note %q", m.level, m.archiveView, m.note)
	}
	m = boardModel(152, 40)
	if m.archivedCount() == 0 {
		t.Fatal("the board fixture should have something archived")
	}
	press(m, "A")
	if !m.archiveView || m.level != levelTrail {
		t.Fatalf("A should open the archive as a list: archive %v level %d", m.archiveView, m.level)
	}
	press(m, "A")
	if m.archiveView || m.level != levelBoard {
		t.Errorf("A again should return to the board: archive %v level %d", m.archiveView, m.level)
	}
}

// The reply panel says what its target is doing, since the line lands in
// that session's input: a question gets the question itself.
func TestTheReplyPanelNamesTheTargetsState(t *testing.T) {
	forceASCII(t)
	m := followModel(152, 40)
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == sessionKey("s-api") {
			m.sessions[i].Snap = state.Snapshot{State: state.NeedsYou, Since: fixtureBase.Add(30 * time.Minute), Activity: "Open port 22 to the office CIDR only? [office CIDR / keep bastion]"}
		}
	}
	press(m, "r")
	if view := m.View(); !strings.Contains(view, "▲ on a question · Open port 22") || !strings.Contains(view, "typed into that prompt") {
		t.Errorf("the panel should say the target is on a question:\n%s", view)
	}
}

// A red run of a test that has failed before is a loop, and the board's
// verdict row says so where the test's name would not fit.
func TestTheVerdictNamesARepeatedFailure(t *testing.T) {
	base := fixtureBase
	leg := func(at time.Time, runs int) journey.Leg {
		return journey.Leg{Class: journey.Test, Label: "pytest", Start: at, End: at.Add(time.Minute),
			Waypoints: []journey.Waypoint{
				{Kind: journey.WaypointTestRun, Text: "18 passed · 1 failed", Short: "18✓ 1✗", At: at},
				{Kind: journey.WaypointTestFail, Text: "test_x", Runs: runs, At: at},
			}}
	}
	tr := journey.Trail{Legs: []journey.Leg{leg(base, 1), leg(base.Add(10*time.Minute), 3)}}
	s := fleet.Session{Snap: state.Snapshot{State: state.Idle}}
	if got := boardVerdict(s, tr, base.Add(20*time.Minute)); !strings.Contains(got, "✗ red 18✓ 1✗ · 3rd failure") {
		t.Errorf("the verdict should name the loop: %q", got)
	}
	tr.Legs = tr.Legs[:1]
	if got := boardVerdict(s, tr, base.Add(20*time.Minute)); strings.Contains(got, "failure") {
		t.Errorf("a first failure is not a loop: %q", got)
	}
}

// ---- round ten

// The reply panel blanks the rest of every row it covers: a column's tail
// sliced at the border read as a leg with no label.
func TestTheReplyPanelBlanksToTheEdge(t *testing.T) {
	forceASCII(t)
	// The board: the panel stands under the selected column, and the
	// columns to its right are what it must not slice.
	m := boardModel(152, 40)
	m.SetPanes(map[string]tmuxop.Pane{
		sessionKey("s-api"): {Target: "dev:1.0", ID: "%5", PID: 4242, Command: "claude", Window: "auth-fix"},
	})
	m.point(sessionKey("s-api"))
	before := strings.Split(m.View(), "\n")
	press(m, "r")
	if !m.replying {
		t.Fatalf("r did not offer the replies: %q", m.note)
	}
	got := m.View()
	covered := 0
	for i, line := range strings.Split(got, "\n") {
		trimmed := strings.TrimRight(line, " ")
		if !strings.HasPrefix(strings.TrimSpace(trimmed), "│") || !strings.Contains(trimmed, "│ ") || strings.Contains(trimmed, "READER") {
			continue // not a row of the panel's body
		}
		// A row that had a neighbour to the right before the panel came
		// now ends at the panel's own border.
		if lipgloss.Width(strings.TrimRight(before[i], " ")) > lipgloss.Width(trimmed) {
			covered++
		}
		if !strings.HasSuffix(trimmed, "│") && !strings.HasSuffix(trimmed, "┐") && !strings.HasSuffix(trimmed, "┘") {
			t.Errorf("text survives right of the panel: %q", line)
		}
	}
	if covered == 0 {
		t.Fatalf("the fixture drew nothing right of the panel:\n%s", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "a digit types the line") {
			after := line[strings.LastIndex(line, "│")+len("│"):]
			if strings.TrimSpace(after) != "" {
				t.Errorf("the row is not blank right of the panel: %q", line)
			}
		}
	}
}

// The panel's working line carries the turn's clock, not the last write's:
// "working for 40s" over a one-hour turn said the opposite of what it
// exists to say.
func TestTheReplyPanelClockIsTheTurns(t *testing.T) {
	forceASCII(t)
	m := followModel(152, 40)
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == sessionKey("s-api") {
			m.sessions[i].Snap = state.Snapshot{State: state.Working, Since: fixtureBase.Add(39 * time.Minute)}
			m.sessions[i].Info.LastEventAt = fixtureBase.Add(39*time.Minute + 20*time.Second)
		}
	}
	m.now = fixtureBase.Add(40 * time.Minute)
	press(m, "r")
	want := "● working " + headTail(m.trail, m.now, true) + " —"
	if view := m.View(); !strings.Contains(view, want) || strings.Contains(view, "working for 40s") {
		t.Errorf("the panel should carry HEAD's own figure %q:\n%s", want, view)
	}
}

// A compaction is drawn under a fork too: the one rule that means "the
// memory changed here" is not the one to skip.
func TestACompactionIsDrawnUnderAFork(t *testing.T) {
	forceASCII(t)
	base := fixtureBase
	tr := hourlyTrail(base, 4)
	tr.Branches = []journey.Branch{{ToolUseID: "b", Label: "scout", Start: base.Add(90 * time.Minute), End: base.Add(100 * time.Minute), Done: true, AfterLeg: 1, Report: "found it"}}
	tr.Compactions = []time.Time{base.Add(110 * time.Minute)} // after leg 1 (forked), before leg 2
	got := renderLv(tr, base.Add(5*time.Hour), 1, 60, 40)
	if !strings.Contains(got, "⟲ context compacted") {
		t.Errorf("a compaction after a forked leg is not drawn:\n%s", got)
	}
}

// The card's second row says the wait once, keeps the tmux session, and
// for a working session with nothing to count says the present.
func TestTheCardSaysThingsOnce(t *testing.T) {
	forceASCII(t)
	m := followModel(152, 40)
	base := fixtureBase
	tr := m.trail
	tr.Prompts = append(tr.Prompts, journey.Prompt{Text: "and now the docs", At: base.Add(3 * time.Hour)})
	m.SetTrail(tr)
	m.now = base.Add(3*time.Hour + 10*time.Minute)
	toLv2(m)
	card := ansi.Strip(m.sessionCard(96)[1])
	if strings.Count(card, "on you") > 1 {
		t.Errorf("the wait is said twice: %q", card)
	}
	if !strings.Contains(card, "⌁ dev") {
		t.Errorf("the tmux session should always be on the card: %q", card)
	}
	if !strings.Contains(card, "3h") {
		t.Errorf("the day should be on the card: %q", card)
	}
	// A working session with nothing to count says the present.
	w := followModel(152, 40)
	for i := range w.sessions {
		if w.sessions[i].Info.Key() == sessionKey("s-api") {
			w.sessions[i].Snap = state.Snapshot{State: state.Working, Since: base.Add(35 * time.Minute), Activity: "Edit: tokens.py"}
		}
	}
	plain := journey.Trail{Prompts: []journey.Prompt{{Text: "go", At: base}}, Legs: []journey.Leg{
		{Class: journey.Scout, Label: "loader.go", Start: base.Add(time.Minute), End: base.Add(5 * time.Minute)},
		{Class: journey.Build, Label: "tokens.py", Start: base.Add(6 * time.Minute), Current: true}}}
	w.SetTrail(plain)
	if w.trails == nil {
		w.trails = map[string]journey.Trail{}
	}
	w.trails[sessionKey("s-api")] = plain // the board's copy, as a refresh hands it over
	toLv2(w)
	card = ansi.Strip(w.sessionCard(96)[1])
	if strings.Contains(card, "scout loader.go") || !strings.Contains(card, "● build") {
		t.Errorf("a working card should say the present, not the last leg: %q", card)
	}
}

// The reader's title never clips its own name for the anchored row: the
// tag goes before the row, and the row before the name.
func TestTheReaderTitleKeepsItsName(t *testing.T) {
	forceASCII(t)
	m := followModel(120, 34)
	toLv3(m)
	m.anchor, m.anchorAt, m.anchorText = 3, fixtureBase, strings.Repeat("a long anchored row ", 6)
	title := ansi.Strip(m.readerTitle(50))
	if !strings.Contains(title, "READER · api") {
		t.Errorf("the name was clipped: %q", title)
	}
	if !strings.Contains(title, "[reader]") || !strings.Contains(title, "…") || lipgloss.Width(title) > 50 {
		t.Errorf("the row should be clipped with a mark and the tag kept, within the width: %q (%d)", title, lipgloss.Width(title))
	}
}

// A moment that lands on a result lands on its call: a page opened on
// "⎿ edited · +1 −1" named nothing.
func TestTheAnchorLandsOnTheCall(t *testing.T) {
	base := fixtureBase
	ev := []transcript.Event{
		{Type: transcript.EventAssistant, Timestamp: base, ToolUses: []transcript.ToolUse{{ID: "e", Name: "Edit", Input: json.RawMessage(`{"file_path":"a.go","old_string":"x","new_string":"y"}`)}}},
		{Type: transcript.EventUser, Timestamp: base.Add(time.Minute), ToolResults: []transcript.ToolResult{{ToolUseID: "e", Text: "The file a.go has been updated."}}},
	}
	o := ReaderOpts{Width: 80}
	line := ReaderAnchor(ev, o, base.Add(time.Minute))
	doc := readerDoc(ev, o)
	if line < 0 || doc[line].kind != readerCall {
		t.Errorf("the anchor landed on line %d (%v), not the call", line, doc[line].kind)
	}
}

// A call whose result lands after other calls says so at the call site,
// and a fold's remainder wears one shape on a clean result and a failed one.
func TestLateResultsAreMarkedAtTheCallAndFoldsWearOneShape(t *testing.T) {
	forceASCII(t)
	base := fixtureBase
	ev := []transcript.Event{
		{Type: transcript.EventAssistant, Timestamp: base, ToolUses: []transcript.ToolUse{{ID: "e", Name: "Edit", Input: json.RawMessage(`{"file_path":"a.go","old_string":"x","new_string":"y"}`)}}},
		{Type: transcript.EventAssistant, Timestamp: base.Add(time.Minute), ToolUses: []transcript.ToolUse{{ID: "b", Name: "Bash", Input: json.RawMessage(`{"command":"pytest"}`)}}},
		{Type: transcript.EventUser, Timestamp: base.Add(2 * time.Minute), ToolResults: []transcript.ToolResult{{ToolUseID: "b", IsError: true, Text: "FAILED test_x\nassert 1 == 2\nsecond line"}}},
		{Type: transcript.EventUser, Timestamp: base.Add(3 * time.Minute), ToolResults: []transcript.ToolResult{{ToolUseID: "e", Text: "The file a.go has been updated."}}},
	}
	got := RenderReader(ev, ReaderOpts{Width: 80, Height: 20, Anchor: -1})
	for _, want := range []string{"⏺ Edit(a.go)\n  ⎿ ↩ result below", "⎿ ✗ FAILED test_x · 2 more lines", "↩ result of Edit(a.go)\n  ⎿ edited · +1 −1"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "+2 lines") || strings.Contains(got, "· 3 lines") {
		t.Errorf("a fold's remainder should say 'N more lines':\n%s", got)
	}
}

// The day is on the trail's title and card from an hour in, not two.
func TestTheDayCountsFromAnHour(t *testing.T) {
	base := fixtureBase
	tr := hourlyTrail(base, 2)
	if got := trailDay(tr, base.Add(70*time.Minute), false); !strings.Contains(got, "1h") {
		t.Errorf("an hour-old trail should have a day figure: %q", got)
	}
	if got := trailDay(tr, base.Add(40*time.Minute), false); got != "" {
		t.Errorf("forty minutes is not a day: %q", got)
	}
}

// ---- round eleven

// A row the panel's edge cuts says it was cut, and the footer keeps the
// key a chapter note is about.
func TestThePanelsCutIsMarkedAndAChapterNoteKeepsItsKey(t *testing.T) {
	forceASCII(t)
	rows := []string{"✗ red 310✓ 2✗ · shipped on red                    ⌁ work"}
	overlay(rows, []string{"│ x │"}, 30, 0)
	if cut := strings.Index(rows[0], "…│ x │"); cut < 0 || !strings.HasPrefix("✗ red 310✓ 2✗ · shipped on red", rows[0][:cut]) {
		t.Errorf("the cut row should end in an ellipsis at the panel's edge: %q", rows[0])
	}
	rows = []string{"short"}
	overlay(rows, []string{"│ x │"}, 30, 0)
	if strings.Contains(rows[0], "…") {
		t.Errorf("a row the panel does not cut wears no mark: %q", rows[0])
	}

	m := boardModel(100, 30)
	openTrail(m)
	m.events = eventsFor(m.trail)
	toLv3(m)
	m.note = "❯ 3/12 · \"now the audit log\" · 14:02"
	if foot := m.footerLine(98); !strings.Contains(foot, "[ ] turns") {
		t.Errorf("a chapter note should keep the chapter key: %q", foot)
	}
	m.note = "no waypoints · reader at the present"
	if foot := m.footerLine(98); !strings.Contains(foot, "space unfold") {
		t.Errorf("another note should keep the reader's own key: %q", foot)
	}
}

// ---- round twelve: typed replies, answers, stop

// A session sitting on a question offers its own options first, as the
// CLI's digits; a typed line goes as typed; stop is escape. Every send
// leaves a trace on the board until the transcript shows the prompt landed.
func TestRepliesAnswerTypeAndStop(t *testing.T) {
	forceASCII(t)
	build := func() (*Model, *recordingTmux) {
		m := followModel(152, 40)
		rec := &recordingTmux{}
		m.runner = rec
		return m, rec
	}
	asking := func(m *Model) {
		for i := range m.sessions {
			if m.sessions[i].Info.Key() == sessionKey("s-api") {
				m.sessions[i].Snap = state.Snapshot{State: state.NeedsYou, Since: fixtureBase.Add(30 * time.Minute), Activity: "Narrow or wide? [narrow / wide]"}
			}
		}
		m.SetEvents([]transcript.Event{
			{Type: transcript.EventAssistant, Timestamp: fixtureBase.Add(30 * time.Minute),
				ToolUses: []transcript.ToolUse{{ID: "q", Name: "AskUserQuestion", Input: json.RawMessage(`{"questions":[{"question":"Narrow or wide?","options":[{"label":"narrow"},{"label":"wide"}]}]}`)}}},
		})
	}
	calls := func(rec *recordingTmux) []string {
		var out []string
		for _, c := range rec.calls {
			out = append(out, strings.Join(c, " "))
		}
		return out
	}

	// Answers first, as digits.
	m, rec := build()
	asking(m)
	press(m, "r")
	view := m.View()
	for _, want := range []string{"1  narrow", "2  wide", "3  please continue", "6  stop", "t  type a line"} {
		if !strings.Contains(view, want) {
			t.Errorf("the panel should offer %q:\n%s", want, view)
		}
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	m.Update(cmd())
	if got := calls(rec); len(got) != 2 || got[0] != "send-keys -t %5 -l 2" || got[1] != "send-keys -t %5 Enter" {
		t.Errorf("an answer should be the menu's own digit, then enter: %v", got)
	}
	if sent, ok := m.sent[sessionKey("s-api")]; !ok || sent.text != "wide" {
		t.Errorf("the send should be remembered by its label: %+v", m.sent)
	}

	// A typed line.
	m, rec = build()
	press(m, "r")
	press(m, "t")
	if !m.replyTyping || !strings.Contains(m.View(), "›") {
		t.Fatalf("t should open a line to type: typing %v", m.replyTyping)
	}
	for _, r := range "rebase on main" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should send the line")
	}
	m.Update(cmd())
	if got := calls(rec); len(got) != 2 || got[0] != "send-keys -t %5 -l rebase on main" {
		t.Errorf("the typed line should be sent as typed: %v", got)
	}
	if m.replying {
		t.Error("the panel should close after sending")
	}

	// Stop.
	m, rec = build()
	press(m, "r")
	n := len(m.replyChoices())
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strconv.Itoa(n))})
	m.Update(cmd())
	if got := calls(rec); len(got) != 1 || got[0] != "send-keys -t %5 Escape" {
		t.Errorf("stop should press escape and nothing else: %v", got)
	}

	// The trace on the board, until a prompt lands.
	m, _ = build()
	m.sent[sessionKey("s-api")] = sentReply{text: "please continue", at: m.now.Add(-2 * time.Minute)}
	if m.trails == nil {
		m.trails = map[string]journey.Trail{}
	}
	m.trails[sessionKey("s-api")] = m.trail
	r, ok := m.boardRows()[sessionKey("s-api")]
	if !ok {
		t.Fatal("no board row for api")
	}
	if got := ansi.Strip(m.boardDelta(sessionKey("s-api"), m.sessions[r.sess], 60)); !strings.Contains(got, `↪ sent "please continue" · 2m ago`) {
		t.Errorf("the board should carry the sent line: %q", got)
	}
	tr := m.trails[sessionKey("s-api")]
	tr.Prompts = append(tr.Prompts, journey.Prompt{Text: "please continue", At: m.now.Add(-time.Minute)})
	m.trails[sessionKey("s-api")] = tr
	if got := ansi.Strip(m.boardDelta(sessionKey("s-api"), m.sessions[r.sess], 60)); strings.Contains(got, "↪ sent") {
		t.Errorf("a prompt that landed clears the trace: %q", got)
	}
}

// ---- round twelve: the read-line, the digest, circling, obligation, hooks, hiding

// The board carries a read-line where the person last looked, and its
// third row adds up what came after: legs, ships, red runs, and the one
// test every red run failed.
func TestTheReadLineAndTheDigest(t *testing.T) {
	forceASCII(t)
	base := fixtureBase
	tr := hourlyTrail(base, 6)
	for i := 3; i < 6; i++ {
		tr.Legs[i].Class = journey.Test
		tr.Legs[i].Waypoints = []journey.Waypoint{
			{Kind: journey.WaypointTestRun, Text: "18 passed · 1 failed", Short: "18✓ 1✗", At: tr.Legs[i].End},
			{Kind: journey.WaypointTestFail, Text: "test_x", Runs: i - 2, At: tr.Legs[i].End},
		}
	}
	looked := base.Add(2*time.Hour + 40*time.Minute) // after leg 2, before leg 3
	got := RenderTrail(tr, TrailOpts{Now: base.Add(7 * time.Hour), Width: 60, Height: 40, Level: levelTrail, Cursor: -1, Pinned: true, Looked: looked})
	if !strings.Contains(got, "│ you were here · 4h ago") {
		t.Errorf("the read-line is missing:\n%s", got)
	}
	if got := RenderTrail(tr, TrailOpts{Now: base.Add(7 * time.Hour), Width: 60, Height: 40, Level: levelTrail, Cursor: -1, Pinned: true}); strings.Contains(got, "you were here") {
		t.Error("no look, no line")
	}

	m := boardModel(152, 40)
	key := sessionKey("s-api")
	m.trails[key] = tr
	if m.seen == nil {
		m.seen = map[string]time.Time{}
	}
	m.seen[key] = looked
	m.now = base.Add(7 * time.Hour)
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == key {
			m.sessions[i].Info.LastEventAt = base.Add(6 * time.Hour)
		}
	}
	r := m.boardRows()[key]
	if got := ansi.Strip(m.boardDelta(key, m.sessions[r.sess], 70)); got != "↳ 3 new legs · 3 red, all test_x · looked 4h ago" {
		t.Errorf("the digest should add the day up: %q", got)
	}
}

// A session going round the same failure is counted in the header and the
// title, and sorts with the alarms.
func TestCirclingIsAnAlarm(t *testing.T) {
	forceASCII(t)
	base := fixtureBase
	loop := hourlyTrail(base, 3)
	for i := range loop.Legs {
		loop.Legs[i].Class = journey.Test
		loop.Legs[i].Current = false
		loop.Legs[i].Waypoints = []journey.Waypoint{
			{Kind: journey.WaypointTestRun, Text: "18 passed · 1 failed", Short: "18✓ 1✗", At: loop.Legs[i].End},
			{Kind: journey.WaypointTestFail, Text: "test_x", Runs: i + 1, At: loop.Legs[i].End},
		}
	}
	if test, runs, ok := circling(loop); !ok || test != "test_x" || runs != 3 {
		t.Fatalf("three red runs of one test should circle: %q %d %v", test, runs, ok)
	}
	green := loop
	green.Legs = append(green.Legs, journey.Leg{Class: journey.Test, Label: "pytest", Start: base.Add(4 * time.Hour), End: base.Add(4*time.Hour + time.Minute),
		Waypoints: []journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "19 passed", Short: "19✓"}}})
	if _, _, ok := circling(green); ok {
		t.Error("a green run ends the loop")
	}

	m := boardModel(152, 40)
	key := sessionKey("s-api")
	m.trails[key] = loop
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == key {
			m.sessions[i].Snap = state.Snapshot{State: state.Working, Since: base}
		}
	}
	if chips := ansi.Strip(m.statusChips()); !strings.Contains(chips, "↻1 circling") || strings.Contains(chips, "all calm") {
		t.Errorf("the header should count the loop and not be calm: %q", chips)
	}
	if got := tabTitle(0, 1, 2); got != "⌂ compass ◍1 ↻2" {
		t.Errorf("tabTitle = %q", got)
	}
	if got := m.obligation(m.sessions[m.boardRows()[key].sess]); got != 2 {
		t.Errorf("a circling session ranks with the alarms: %d", got)
	}
}

// The board is ordered by what is owed, and sessions that owe nothing go
// to the strip however many columns are free.
func TestTheBoardIsRankedByObligation(t *testing.T) {
	forceASCII(t)
	m := boardModel(220, 40)
	base := fixtureBase
	ranks := map[string]int{}
	for _, i := range m.viewOrder() {
		s := m.sessions[i]
		ranks[sessionName(s.Info)] = m.obligation(s)
	}
	last := -1
	for _, i := range m.viewOrder() {
		if r := m.obligation(m.sessions[i]); r < last {
			t.Fatalf("the view order is not by obligation: %v", ranks)
		} else {
			last = r
		}
	}
	// Make one session owe nothing: shipped green, read, old.
	key := sessionKey("s-api")
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == key {
			m.sessions[i].Snap = state.Snapshot{State: state.Idle, Since: base}
			m.sessions[i].Info.LastEventAt = base.Add(-30 * time.Hour)
		}
	}
	m.trails[key] = journey.Trail{Legs: []journey.Leg{{Class: journey.Ship, Label: "commit", Start: base.Add(-31 * time.Hour), End: base.Add(-30 * time.Hour)}}}
	if m.seen == nil {
		m.seen = map[string]time.Time{}
	}
	m.seen[key] = base
	if got := m.obligation(m.sessions[m.boardRows()[key].sess]); got != 6 {
		t.Fatalf("a shipped, read, day-old session owes nothing: rank %d", got)
	}
	m.boardSelect(0) // the selection always has a column; look from somewhere else
	if m.selectedKey == key {
		t.Fatal("the fixture's first session should not be api")
	}
	n, _ := boardColumns(220-2*edgePad, len(m.viewOrder()))
	for _, k := range m.boardKeys(n) {
		if k == key {
			t.Errorf("a session that owes nothing took a column: %v", m.boardKeys(n))
		}
	}
	if strip := ansi.Strip(m.boardStrip(m.boardKeys(n), m.boardRows(), 200)); !strings.Contains(strip, "api") {
		t.Errorf("the strip should name it: %q", strip)
	}
}

// The hook fires on the crossings that matter and never on the launch.
func TestTheHookFiresOnCrossings(t *testing.T) {
	m := boardModel(152, 40)
	var fired []string
	m.hookRun = func(event, session, tmux, detail string) { fired = append(fired, event+" "+session) }
	m.fireHooks()
	if len(fired) != 0 {
		t.Fatalf("the first refresh is a baseline, not an event: %v", fired)
	}
	key := sessionKey("s-api")
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == key {
			m.sessions[i].Snap = state.Snapshot{State: state.NeedsYou, Since: m.now, Activity: "Narrow or wide?"}
		}
	}
	m.fireHooks()
	m.fireHooks() // the same state again is not a second crossing
	if len(fired) != 1 || fired[0] != "needs_you api" {
		t.Errorf("fired %v, want one needs_you", fired)
	}
	tr := m.trails[key]
	tr.Branches = append(tr.Branches, journey.Branch{ToolUseID: "b", Label: "scout", Start: m.now, End: m.now, Done: true})
	m.trails[key] = tr
	m.fireHooks()
	if len(fired) != 2 || fired[1] != "agents_back api" {
		t.Errorf("fired %v, want agents_back", fired)
	}
}

// `x` takes a session off the board; the archive lists it; `x` there
// brings it back; and a hidden session that needs you comes back on its own.
func TestHidingASession(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	key := sessionKey("s-api")
	m.point(key)
	before := len(m.viewOrder())
	press(m, "x")
	if !m.hidden[key] || len(m.viewOrder()) != before-1 || m.selectedKey == key {
		t.Fatalf("x should hide the session and move on: hidden %v, %d of %d shown, selected %q", m.hidden[key], len(m.viewOrder()), before, m.selectedKey)
	}
	if strip := ansi.Strip(m.View()); !strings.Contains(strip, "1 hidden · A lists them") {
		t.Errorf("the strip should count the hidden: %s", strip)
	}
	press(m, "A")
	if !m.archiveView {
		t.Fatal("A should open the archive, which lists hidden sessions")
	}
	if !strings.Contains(m.View(), "api") {
		t.Errorf("the archive should list the hidden session:\n%s", m.View())
	}
	m.point(key)
	press(m, "x")
	if m.hidden[key] {
		t.Error("x in the archive should unhide")
	}
	press(m, "x")
	if !m.hidden[key] {
		t.Fatal("x again should hide")
	}
	press(m, "A")
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == key {
			m.sessions[i].Snap = state.Snapshot{State: state.NeedsYou, Since: m.now}
		}
	}
	found := false
	for _, i := range m.viewOrder() {
		if m.sessions[i].Info.Key() == key {
			found = true
		}
	}
	if !found {
		t.Error("a hidden session that needs you should come back on its own")
	}
}

// `/` below the reader searches the fleet: the board, the list and the
// archive narrow to the sessions whose name, prompt, branch, leg or file
// matches; esc clears it. The reader keeps its own `/`.
func TestTheFleetSearch(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	all := len(m.viewOrder())
	if all < 3 {
		t.Skip("the fixture is too small to narrow")
	}
	press(m, "/")
	if !m.searching || !m.searchFleet {
		t.Fatal("/ on the board should start the fleet search")
	}
	for _, r := range "401" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.fleetQuery != "401" || m.searching {
		t.Fatalf("enter should set the query: %q searching %v", m.fleetQuery, m.searching)
	}
	got := m.viewOrder()
	if len(got) == 0 || len(got) >= all {
		t.Errorf("the query should narrow the fleet: %d of %d", len(got), all)
	}
	for _, i := range got {
		if !m.matchesQuery(m.sessions[i]) {
			t.Errorf("%s does not match /401", sessionName(m.sessions[i].Info))
		}
	}
	if head := ansi.Strip(m.headerLine(150)); !strings.Contains(head, "/401 · ") || !strings.Contains(head, fmt.Sprintf("%d of %d", len(got), all)) {
		t.Errorf("the header should carry the query and the count: %q", head)
	}
	press(m, "esc")
	if m.fleetQuery != "" || len(m.viewOrder()) != all {
		t.Errorf("esc should clear the query: %q, %d shown", m.fleetQuery, len(m.viewOrder()))
	}
	// A file a leg touched answers too.
	m.fleetQuery = "tokens.py"
	if len(m.viewOrder()) == 0 {
		t.Error("a touched file should match")
	}
	m.fleetQuery = ""

	// The reader's `/` is untouched.
	toLv3(m)
	press(m, "/")
	if !m.searching || m.searchFleet {
		t.Error("/ in the reader should be the reader's search")
	}
}

// Two live sessions on one file, or one failing test, are named in the
// strip: the only thing reading every transcript should say so.
func TestOverlapsAreNamed(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	base := fixtureBase
	m.now = base.Add(40 * time.Minute)
	shared := func(name string) journey.Trail {
		return journey.Trail{Legs: []journey.Leg{
			{Class: journey.Build, Label: name, Start: base.Add(30 * time.Minute), End: base.Add(35 * time.Minute), Files: []string{"tokens.py"}},
			{Class: journey.Test, Label: "pytest", Start: base.Add(36 * time.Minute), End: base.Add(37 * time.Minute),
				Waypoints: []journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "1 failed", Short: "1✗"}, {Kind: journey.WaypointTestFail, Text: "test_refresh"}}},
		}}
	}
	m.trails[sessionKey("s-api")] = shared("api")
	m.trails[sessionKey("s-webapp")] = shared("webapp")
	got := m.overlaps()
	joined := strings.Join(got, "\n")
	for _, want := range []string{"both touched tokens.py in the last 20m", "are both failing test_refresh"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in %v", want, got)
		}
	}
	if !strings.Contains(joined, "api and webapp") && !strings.Contains(joined, "webapp and api") {
		t.Errorf("the overlap should name both sessions: %v", got)
	}
	if strip := ansi.Strip(m.boardStrip(nil, m.boardRows(), 200)); !strings.Contains(strip, "⚠") {
		t.Errorf("the strip should carry the overlap: %q", strip)
	}
	// Old work is not an overlap.
	m.now = base.Add(3 * time.Hour)
	if got := m.overlaps(); strings.Contains(strings.Join(got, "\n"), "touched") {
		t.Errorf("a file touched hours ago is not an overlap: %v", got)
	}
}
