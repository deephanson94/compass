package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

func withQuiet(t *testing.T, tmux []string, after time.Duration) {
	t.Helper()
	SetQuiet(tmux, after)
	t.Cleanup(func() { SetQuiet(nil, 0) })
}

// boardText is the board as the person sees it, styling off.
func boardText(m *Model) string {
	return ansi.Strip(m.View())
}

// The idle sessions of a quiet tmux session give up their cards for rows
// under the cards; the ones that owe you anything keep theirs.
func TestQuietSessionsAreRowsUnderTheCards(t *testing.T) {
	withCards(t)
	withQuiet(t, []string{"w"}, 0) // every pane of the scene is in tmux session w
	m := sceneModel(sceneCards(), 290, 80)
	m.point(sessionKey("harness")) // the asking one: never quiet anyway
	frame := boardText(m)

	if !strings.Contains(frame, "quiet · 2") {
		t.Fatalf("no quiet band for the two idle sessions:\n%s", frame)
	}
	for _, name := range []string{"s3", "demo"} {
		row := lineWith(frame, " ○ "+name+" · ")
		if row == "" || !strings.Contains(row, "⌁ w:") {
			t.Errorf("%s should be one quiet row with its pane, got %q\n%s", name, row, frame)
		}
	}
	// The asking, the hung and the working keep their cards.
	for _, name := range []string{"harness", "flow", "gemma"} {
		if lineWith(frame, " ○ "+name+" · ") != "" {
			t.Errorf("%s owes something and lost its card:\n%s", name, frame)
		}
	}
}

// Selecting a quiet session gives it its card back; nothing quiet, nothing
// changes.
func TestSelectingAQuietSessionDrawsItsCard(t *testing.T) {
	withCards(t)
	withQuiet(t, []string{"w"}, 0)
	m := sceneModel(sceneCards(), 290, 80)
	m.point(sessionKey("s3"))
	frame := boardText(m)
	if !strings.Contains(frame, "quiet · 1") || lineWith(frame, " ○ s3 · ") != "" {
		t.Errorf("the selected quiet session should be a card, the other one a row:\n%s", frame)
	}

	SetQuiet(nil, 0)
	m = sceneModel(sceneCards(), 290, 80)
	if strings.Contains(boardText(m), "quiet · ") {
		t.Error("nothing set quiet, and the board drew a quiet band")
	}
}

// idle_compact: any session idle that long is quiet, whatever its tmux
// session; one idle less long keeps its card.
func TestIdleLongEnoughIsQuiet(t *testing.T) {
	withCards(t)
	withQuiet(t, nil, 24*time.Hour)
	m := sceneModel(sceneCards(), 290, 80)
	m.point(sessionKey("harness"))
	frame := boardText(m)
	if lineWith(frame, " ○ demo · ") == "" { // idle 29h
		t.Errorf("demo, idle 29h, should be quiet:\n%s", frame)
	}
	if lineWith(frame, " ○ s3 · ") != "" { // idle 4h
		t.Errorf("s3, idle 4h, should keep its card:\n%s", frame)
	}
}

// lineWith is the first line of frame holding s, or "".
func lineWith(frame, s string) string {
	for _, l := range strings.Split(frame, "\n") {
		if strings.Contains(l, s) {
			return l
		}
	}
	return ""
}

// A watcher's finished review reads as it came out; and where the screen
// is too short for the rows, the strip names the quiet sessions as it
// names any session without a card, so none of them is out of reach.
func TestQuietRowsSayTheVerdictAndFallBackToTheStrip(t *testing.T) {
	withCards(t)
	withQuiet(t, []string{"w"}, 0)
	sc := sceneCards()
	for i, s := range sc.sessions {
		if sessionName(s.Info) == "demo" {
			sc.sessions[i].Snap = state.Snapshot{State: state.Idle, Since: sceneNow.Add(-time.Hour), Reason: "verdict: done", Verdict: state.VerdictDone}
		}
	}
	m := sceneModel(sc, 290, 80)
	m.point(sessionKey("harness"))
	if row := lineWith(boardText(m), " ○ demo · "); !strings.Contains(row, "✓ ok") {
		t.Errorf("an auto-ok review's quiet row should say ✓ ok, got %q", row)
	}

	m = sceneModel(sc, 290, 24)
	m.point(sessionKey("harness"))
	frame := boardText(m)
	if strings.Contains(frame, "quiet · ") {
		t.Fatalf("no room for the band, and it was drawn:\n%s", frame)
	}
	strip := lineWith(frame, "more · ")
	if !strings.Contains(strip, "s3") || !strings.Contains(strip, "demo") {
		t.Errorf("the strip should name the quiet sessions, got %q\n%s", strip, frame)
	}
}

// The arrows walk the cards and step past the quiet rows; a quiet session
// selected by its digit is drawn as one more card at the end, so the cards
// around it stand where they stood (the board reshuffled on every other
// arrow when it took its place in the order).
func TestArrowsSkipQuietRowsAndNothingMoves(t *testing.T) {
	withCards(t)
	withQuiet(t, []string{"w"}, 0) // s3 and demo are quiet
	m := sceneModel(sceneCards(), 290, 80)
	m.point(sessionKey("harness"))
	seen := map[string]bool{}
	for i := 0; i < 12; i++ {
		m.Update(tea.KeyMsg{Type: tea.KeyRight})
		seen[selectedName(m)] = true
	}
	for _, q := range []string{"s3", "demo"} {
		if seen[q] {
			t.Errorf("→ stepped onto quiet %s", q)
		}
	}

	m.point(sessionKey("harness"))
	before := ansi.Strip(m.View())
	m.point(sessionKey("s3")) // its digit
	after := ansi.Strip(m.View())
	for _, name := range []string{"harness", "flow", "etl", "api", "stages", "gemma"} {
		if b, a := cardColumn(before, name), cardColumn(after, name); b != a {
			t.Errorf("%s's card moved from %v to %v when a quiet session was selected", name, b, a)
		}
	}
	if !strings.Contains(after, "▸7 ") && !strings.Contains(after, "▸6 ") {
		t.Errorf("the selected quiet session should be a card:\n%s", after)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if got := selectedName(m); got == "s3" || got == "demo" {
		t.Errorf("← from the quiet card went to %s, want the last card", got)
	}
}

// cardColumn is where a card's head row stands: its line and its column.
func cardColumn(frame, name string) [2]int {
	for y, l := range strings.Split(frame, "\n") {
		if y < 2 {
			continue // the title bar names the selection, not a card
		}
		if x := strings.Index(l, " "+name+" "); x >= 0 && strings.ContainsAny(l, "○●▲◍↻") {
			return [2]int{y, len([]rune(l[:x]))}
		}
	}
	return [2]int{-1, -1}
}

// compact_bg: a finished turn whose agent still works in the background is
// a row, its bg job in working's colour; the same session without the
// agent, or with compact_bg off, keeps its card.
func TestBackgroundJobIsQuiet(t *testing.T) {
	withCards(t)
	SetQuietBG(true)
	t.Cleanup(func() { SetQuietBG(false) })
	m := sceneModel(sceneCards(), 290, 80)
	m.point(sessionKey("harness"))
	key := sessionKey("s3")
	tr := m.trails[key]
	tr.Branches = append(tr.Branches, journey.Branch{ToolUseID: "toolu_bg", Label: "indexer"})
	m.trails[key] = tr
	if m.agents == nil {
		m.agents = map[string]map[string]agentLive{}
	}
	m.agents[key] = map[string]agentLive{"toolu_bg": {Wrote: m.now.Add(-time.Minute)}}

	frame := boardText(m)
	row := lineWith(frame, " ○ s3 · ")
	if !strings.Contains(frame, "quiet · 1") || !strings.Contains(row, "bg job") {
		t.Fatalf("s3, its agent out in the background, should be a quiet bg job row, got %q:\n%s", row, frame)
	}
	if lineWith(frame, " ○ demo · ") != "" {
		t.Errorf("demo has no agent out and lost its card:\n%s", frame)
	}

	SetQuietBG(false)
	if strings.Contains(boardText(m), "quiet · ") {
		t.Error("compact_bg off, and the bg job is still a row")
	}
}

// compact_nopane: an idle session with no tmux pane is a row — compact_sessions
// goes by the pane's tmux session and can never reach it; an idle one with a
// pane keeps its card, and compact_nopane off, so does the pane-less one.
func TestNoPaneIsQuiet(t *testing.T) {
	withCards(t)
	SetQuietNoPane(true)
	t.Cleanup(func() { SetQuietNoPane(false) })
	m := sceneModel(sceneCards(), 290, 80)
	m.point(sessionKey("harness"))
	delete(m.panes, sessionKey("s3"))

	frame := boardText(m)
	row := lineWith(frame, " ○ s3 · ")
	if !strings.Contains(frame, "quiet · 1") || !strings.Contains(row, "no pane") {
		t.Fatalf("s3, idle with no pane, should be a quiet row saying so, got %q:\n%s", row, frame)
	}
	if lineWith(frame, " ○ demo · ") != "" {
		t.Errorf("demo has a pane and lost its card:\n%s", frame)
	}

	SetQuietNoPane(false)
	if strings.Contains(boardText(m), "quiet · ") {
		t.Error("compact_nopane off, and the pane-less session is still a row")
	}
}

// compact_working: a working session in a quiet tmux session is a row that
// says working and what it is at; the stuck and the asking keep their cards,
// a pinned one is a card, and off, working is a card again.
func TestWorkingInAQuietTmuxSessionIsARow(t *testing.T) {
	withCards(t)
	withQuiet(t, []string{"w"}, 0)
	SetQuietWorking(true)
	t.Cleanup(func() { SetQuietWorking(false) })
	m := sceneModel(sceneCards(), 290, 80)
	m.point(sessionKey("harness"))

	frame := boardText(m)
	for _, name := range []string{"stages", "gemma"} {
		row := lineWith(frame, " "+name+" · ")
		if !strings.Contains(row, " · working · build") {
			t.Errorf("%s, working in w, should be a row saying working · build, got %q\n%s", name, row, frame)
		}
	}
	for _, name := range []string{"harness", "flow"} {
		if s, _ := m.session(sessionKey(name)); m.quietBy(s) {
			t.Errorf("%s owes something and lost its card:\n%s", name, frame)
		}
	}

	m.point(sessionKey("gemma"))
	press(m, "p")
	m.point(sessionKey("harness"))
	if row := lineWith(boardText(m), " gemma · working"); row != "" {
		t.Errorf("gemma, pinned, should be a card, got row %q", row)
	}

	SetQuietWorking(false)
	if row := lineWith(boardText(m), " stages · working"); row != "" {
		t.Errorf("compact_working off, and stages is still a row: %q", row)
	}
}
