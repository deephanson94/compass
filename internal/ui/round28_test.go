package ui

import (
	"os"
	"path/filepath"
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
	if !strings.Contains(full, "· 2 harness · ⌁ harness:1.0 · board") {
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

// A fleet of one keeps its past on screen: the rows the live list leaves
// blank carry the sessions that ended last, numbered on from the live
// fleet's digits, each with its verdict where there is room — and a digit
// opens one in the archive, where `A` comes back (#47).
func TestAFleetOfOneKeepsItsRecentPast(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 120, 34)
	if !m.sessionView() {
		t.Fatalf("a fleet of one at 120 should open on the session view")
	}
	col := strings.Join(m.trailColumn(55, 28), "\n")
	for _, want := range []string{"recent · 12 archived · A browses", ` 2 ○ api · "fix the 401 on token`, `  ✗ red 12✓ 1✗ 2h`, ` 9 ○ migrate`} {
		if !strings.Contains(col, want) {
			t.Errorf("the session view's band lacks %q:\n%s", want, col)
		}
	}
	if strings.Contains(col, "10 ○") || strings.Count(col, "\n ○") > 0 {
		t.Errorf("the band ran past the digits:\n%s", col)
	}
	// The narrow list draws the same band in the fleet column, without
	// the verdict there is no room for.
	n := sceneModel(sceneSecondDay(), 80, 24)
	list := strings.Join(n.fleetLines(33, 18), "\n")
	if !strings.Contains(list, "recent · 12 archived · A browses") || !strings.Contains(list, ` 2 ○ api · "fix the 40…  ✗ red 2h`) {
		t.Errorf("the narrow list's band is missing or misdrawn:\n%s", list)
	}
	if strings.Count(list, "archived") != 1 {
		t.Errorf("the archive's line is said twice:\n%s", list)
	}
	// A digit the live fleet does not use opens the band's row.
	press(n, "2")
	if !n.archiveView || n.selectedKey != sessionKey("p-api") {
		t.Errorf("2 did not open the archive on api: archive=%v selected=%q", n.archiveView, n.selectedKey)
	}
	if !strings.Contains(n.note, "A returns") {
		t.Errorf("the note does not say the way back: %q", n.note)
	}
	press(n, "A")
	if n.archiveView || n.selectedKey != sessionKey("hello") {
		t.Errorf("A did not return to the live fleet on hello: archive=%v selected=%q", n.archiveView, n.selectedKey)
	}
}

// The band is the live list's alone: never on the board, whose blank
// rows are more columns' (#43), and never under a search.
func TestTheRecentBandStaysOffTheBoard(t *testing.T) {
	m := sceneModel(sceneSecondDay(), 120, 34)
	m.level = levelBoard // the one-column board
	if !m.boardShown() {
		t.Fatalf("a 120-column deck should show the board")
	}
	if rows := m.recentRows(9); len(rows) != 0 {
		t.Errorf("the board draws a band of %d rows", len(rows))
	}
	press(m, "2")
	if m.archiveView {
		t.Errorf("a digit on the board opened the archive")
	}
	n := sceneModel(sceneSecondDay(), 80, 24)
	n.fleetQuery = "api"
	if rows := n.recentRows(9); len(rows) != 0 {
		t.Errorf("a search draws a band of %d rows", len(rows))
	}
}

// An open lane is judged by its own transcript, not the lead's silence
// (#49): a lane whose file has gone quiet wears ◍ and says so beneath it,
// a lane that wrote forty seconds ago says so, a lane that has written
// nothing says that, and the parked clause counts the agents — "1 silent
// 12m" — where "quiet 15m" restated the newest lane's age.
func TestAnOpenLaneIsJudgedByItsOwnFile(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSubagents(), 120, 34)
	col := strings.Join(m.boardColumn(sessionKey("porter"), rowFor(t, m, sessionKey("porter")), 37, 30), "\n")
	for _, want := range []string{"◈3 out 20m · 2 silent 18m", "├─◍ Red-team the plugin", "├─◍ Review /auto-resume"} {
		if !strings.Contains(col, want) {
			t.Errorf("the board column lacks %q:\n%s", want, col)
		}
	}
	if strings.Contains(col, "quiet 15m") {
		t.Errorf("the parked clause still restates the lead's silence:\n%s", col)
	}
	if chips := ansi.Strip(m.statusChips()); !strings.Contains(chips, "◈3 out · 2 silent 18m") {
		t.Errorf("the header chip does not count the silent lane: %q", chips)
	}
	pressTab(m) // the session view, cursor on the present: the newest lane
	trail := strings.Join(m.trailColumn(55, 30), "\n")
	for _, want := range []string{"└ ● Bash: python dla.py --model moe_…  wrote 40s ago", "└ ◍ nothing written", "silent 18m", "▸─◍ Red-team the plugin architecture ", "└ ◍ Bash: pytest -x tests/plugins"} {
		if !strings.Contains(trail, want) {
			t.Errorf("the session view's lanes lack %q:\n%s", want, trail)
		}
	}
	if strings.Contains(trail, "→1") {
		t.Errorf("a lane whose own file was read still wears the →N hedge:\n%s", trail)
	}
}

// Tab on a lane opens the reader on the agent's own conversation, in place
// of the lead's — the same depth, aimed at a lane — titled by the lane
// and clocked by it; leaving the reader brings the lead's back (#49).
func TestTabOnALaneReadsTheAgentsOwnConversation(t *testing.T) {
	forceASCII(t)
	for _, w := range []int{80, 120} {
		m := sceneModel(sceneSubagents(), w, 34)
		pressTab(m)
		if m.level < levelWaypoints {
			pressTab(m)
		}
		rows := TrailRows(m.trail, m.level)
		if m.cursor < 0 || rows[m.cursor].Kind != "branch" || rows[m.cursor].Lane != "a4" {
			t.Fatalf("at %d the cursor does not open on the newest lane: %+v", w, rows[m.cursor])
		}
		pressTab(m)
		if m.level != levelReader || m.readerLane != "a4" {
			t.Fatalf("at %d Tab on the lane did not open its conversation: level %d lane %q", w, m.level, m.readerLane)
		}
		view := ansi.Strip(m.View())
		for _, want := range []string{"◍ Red-team the plugin architectu", "the agent's own conversation", "❯ Red-team the plugin architecture", "Bash(pytest -x tests/plugins)"} {
			if !strings.Contains(view, want) {
				t.Errorf("at %d the lane's reader lacks %q:\n%s", w, want, view)
			}
		}
		if lead := strings.Count(view, "/kickoff porter_tui"); lead > 1 || (w < 110 && lead > 0) {
			t.Errorf("at %d the lane's reader shows the lead's conversation:\n%s", w, view)
		}
		press(m, "esc")
		if m.readerLane != "" || m.level != levelWaypoints {
			t.Errorf("at %d leaving the reader kept the lane: level %d lane %q", w, m.level, m.readerLane)
		}
		if view := ansi.Strip(m.View()); !strings.Contains(view, "/kickoff porter_tui") && w >= 110 {
			t.Errorf("at %d the lead's conversation did not come back:\n%s", w, view)
		}
	}
}

// The agents' files are paired to lanes by the call's id, from the meta
// file beside each transcript — never by the lane's name.
func TestAgentsArePairedByTheCallsID(t *testing.T) {
	dir := t.TempDir()
	tp := filepath.Join(dir, "sess.jsonl")
	sub := filepath.Join(dir, "sess", "subagents")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(sub, "agent-1.meta.json"), []byte(`{"agentType":"general-purpose","description":"Round 22: few ongoing","toolUseId":"toolu_A","spawnDepth":1}`), 0o644)
	os.WriteFile(filepath.Join(sub, "agent-1.jsonl"), []byte("{}\n"), 0o644)
	os.WriteFile(filepath.Join(sub, "agent-2.meta.json"), []byte(`{"toolUseId":"toolu_B"}`), 0o644) // no transcript beside it
	got := pairAgents(agentDir(tp))
	if len(got) != 1 || got["toolu_A"] != filepath.Join(sub, "agent-1.jsonl") {
		t.Errorf("pairAgents = %v, want toolu_A alone", got)
	}
	if pairAgents(agentDir("")) != nil || pairAgents(filepath.Join(dir, "nowhere")) != nil {
		t.Errorf("a missing directory should pair nothing")
	}
}
