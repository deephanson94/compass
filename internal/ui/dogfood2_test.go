package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/deephanson94/compass/internal/journey"
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
	for _, want := range []string{"test_refresh_expired_token · 2nd time", "test_refresh_expired_token · 3rd time"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q at Lv2:\n%s", want, got)
		}
	}
	if strings.Count(got, "1st time") > 0 {
		t.Errorf("the first failure is not a repeat:\n%s", got)
	}
	// On a wide row the detail rides inline, and says it there too.
	if got := renderLv(tr, base.Add(30*time.Minute), 1, 120, 30); !strings.Contains(got, "· 3rd time") {
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
	for _, want := range []string{"⟲ compacted " + tr.Compactions[0].Local().Format("15:04"), "⟲ compacted " + tr.Compactions[1].Local().Format("15:04")} {
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
	foot := m.footerLine(150)
	for _, want := range []string{"reply to api", "1 please continue", "2 report status", "3 you were stuck", "esc"} {
		if !strings.Contains(foot, want) {
			t.Errorf("the footer should offer %q:\n%s", want, foot)
		}
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
	if foot := m.footerLine(150); !strings.Contains(foot, "1 ship it") || strings.Contains(foot, "please continue") {
		t.Errorf("configured replies should be what is offered:\n%s", foot)
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
