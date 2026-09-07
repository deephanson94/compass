package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/transcript"
)

// The pins of the first live look after the panel (#78).

// The harness's error tag never reaches a label or a reader row.
func TestTheErrorTagNeverReachesARow(t *testing.T) {
	if got := journey.Untag("<tool_use_error>File has not been read yet.</tool_use_error>"); got != "File has not been read yet." {
		t.Errorf("Untag = %q", got)
	}
	lines := resultBody("<tool_use_error>File has not been read yet. Read it first.</tool_use_error>\n")
	if len(lines) != 1 || strings.Contains(lines[0], "<") {
		t.Errorf("resultBody keeps the tag: %q", lines)
	}
}

// A command that begins by entering the session's own directory is shown
// from what it does; a bare cd, or a cd elsewhere, stays.
func TestACommandsOwnCDIntoTheSessionIsNotTheNews(t *testing.T) {
	cwd := "/home/user/api"
	for cmd, want := range map[string]string{
		"cd /home/user/api; git diff tests/": "git diff tests/",
		"cd /home/user/api && go test ./...": "go test ./...",
		"cd /home/user/api/ ; pytest -x":     "pytest -x",
		"cd /home/user/api":                  "cd /home/user/api",
		"cd /home/user/other; make":          "cd /home/user/other; make",
		"git status":                         "git status",
	} {
		use := transcript.ToolUse{Name: "Bash", Input: json.RawMessage(`{"command":` + jsonQuote(cmd) + `}`)}
		got, _ := state.StripCD(cmd, cwd)
		if got != want {
			t.Errorf("StripCD(%q) = %q, want %q", cmd, got, want)
		}
		d := &docBuilder{width: 80, cwd: cwd}
		if got := d.argument(use, cwd); got != want {
			t.Errorf("argument(%q) = %q, want %q", cmd, got, want)
		}
	}
}

func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// The grab key is offered only while a session is waiting on you.
func TestGrabIsOfferedOnlyWhileSomethingWaits(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFewOngoing(), 120, 34)
	if foot := ansi.Strip(m.footerLine(118)); !strings.Contains(foot, "g grab") {
		t.Errorf("a fleet with a question open should offer g: %q", foot)
	}
	for i := range m.sessions {
		if m.sessions[i].Snap.State == state.NeedsYou {
			m.sessions[i].Snap.State = state.Working
		}
	}
	if foot := ansi.Strip(m.footerLine(118)); strings.Contains(foot, "g grab") {
		t.Errorf("nothing amber: g answers no question: %q", foot)
	}
}

// The band ranks a session with legs before one without.
func TestTheBandRanksWorkedSessionsFirst(t *testing.T) {
	m := sceneModel(sceneSecondDay(), 120, 34)
	key := ""
	for _, s := range m.sessions {
		if !s.Live {
			key = s.Info.Key()
			break
		}
	}
	// The newest archived session, emptied of its legs: it drops behind
	// the ones that did work.
	rows := m.recentRows(8)
	first := m.sessions[rows[0].sess].Info.Key()
	tr := m.trails[first]
	tr.Legs = nil
	m.trails[first] = tr
	if again := m.recentRows(8); m.sessions[again[0].sess].Info.Key() == first {
		t.Errorf("an empty session leads the band over worked ones: %v", key)
	}
}

// A session with no title and no prompt is off the band (#78).
func TestTheBandSkipsASessionWithNothingToGoBackTo(t *testing.T) {
	m := sceneModel(sceneSecondDay(), 120, 34)
	rows := m.recentRows(8)
	first := rows[0].sess
	s := m.sessions[first]
	s.Info.Title = ""
	m.sessions[first] = s
	tr := m.trails[s.Info.Key()]
	tr.Prompts = nil
	m.trails[s.Info.Key()] = tr
	if archiveHeadline(m.sessions[first]) != "" {
		t.Skip("the fixture names the session another way")
	}
	for _, r := range m.recentRows(8) {
		if r.sess == first {
			t.Errorf("a session with nothing to go back to is on the band")
		}
	}
}

// The band says which tool ran a session where the fleet runs two, on every
// row or on none, and never over the prompt's first words (#79).
func TestTheBandSaysTheToolWhereTheFleetRunsTwo(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 120, 34)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "billing · opencode · \"reconcile") || !strings.Contains(view, "api · claude · \"fix the 401") {
		t.Errorf("at 120 the band should name the tool on its rows:\n%s", view)
	}
	if strings.Contains(view, "checkout-flake-hunt · claude") {
		t.Errorf("a long name sheds its own word rather than its prompt:\n%s", view)
	}
	narrow := sceneModel(sceneSecondDay(), 80, 24)
	if v := ansi.Strip(narrow.View()); strings.Contains(v, "· opencode ·") || strings.Contains(v, "· claude ·") {
		t.Errorf("at 80 a tool word would leave no prompt; none is drawn:\n%s", v)
	}
	// A frame that draws the band: the 100-column list opens on it (#81).
	ones := sceneModel(sceneFleetHygiene(), 100, 30)
	if v := ansi.Strip(ones.View()); !strings.Contains(v, "recent ·") || strings.Contains(v, "· claude ·") {
		t.Errorf("a fleet of claudes needs no word on its band:\n%s", v)
	}
}

// The deck names a session by the name its person gave it (/rename), over
// the directory's, everywhere the name is drawn (#79).
func TestASessionWearsTheNameItsPersonGaveIt(t *testing.T) {
	forceASCII(t)
	info := fleet.SessionInfo{ID: "x", CWD: "/home/user/webapp", Name: "checkout-flake-hunt"}
	if got := sessionName(info); got != "checkout-flake-hunt" {
		t.Errorf("sessionName = %q", got)
	}
	info.Name = ""
	if got := sessionName(info); got != "webapp" {
		t.Errorf("without a name the directory names it: %q", got)
	}
	m := sceneModel(sceneSecondDay(), 120, 34)
	if view := ansi.Strip(m.View()); !strings.Contains(view, "checkout-flake-hunt · \"the che") {
		t.Errorf("the band should carry the renamed session's name:\n%s", view)
	}
	pressTab(m)
	press(m, "3")
	if head := ansi.Strip(m.headerLine(118)); !strings.Contains(head, `checkout-flake-hunt · "the checkout suite flakes on CI"`) {
		t.Errorf("the header should carry the name: %q", head)
	}
}

// Space names the call whose result it unfolded: the reader has no
// cursor, and the first folded result on screen is the one it takes (#79).
func TestSpaceNamesWhatItUnfolded(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	m := sceneModel(sc, 120, 34)
	for _, k := range []string{"3", "tab", "tab", " "} {
		pressKey(m, k)
		poll(m, sc)
	}
	if !strings.HasPrefix(m.note, "unfolded ") || !strings.Contains(m.note, "(") || strings.Contains(m.note, glyphCall) {
		t.Errorf("space should name the call it opened: %q", m.note)
	}
	pressKey(m, " ")
	if !strings.HasPrefix(m.note, "folded ") {
		t.Errorf("space again should name what it folded: %q", m.note)
	}
	// A route whose first folded result stands under a call row: the note
	// names the call without the reader's glyph (#80).
	sdc := sceneSecondDay()
	sd := sceneModel(sdc, 120, 34)
	for _, k := range []string{"2", "tab", "tab", " "} {
		pressKey(sd, k)
		poll(sd, sdc)
	}
	if sd.note != "unfolded Read(sched.go)" {
		t.Errorf("the note names the call without its glyph: %q", sd.note)
	}
}

// A default-tool row sheds its band word before an OpenCode row: where an
// OpenCode row cannot keep "opencode", no row says "claude" (#80).
func TestTheBandNeverSaysOnlyTheDefaultTool(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 100, 30)
	view := ansi.Strip(m.View())
	if strings.Contains(view, "· claude ·") && !strings.Contains(view, "· opencode ·") {
		t.Errorf("at 100 the band's only tool word is claude:\n%s", view)
	}
}

// An archived row says its tool where the archive holds two, and so does
// its header (#80).
func TestAnArchivedRowSaysItsTool(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 120, 34)
	pressTab(m)
	press(m, "4") // billing, the OpenCode one
	if !m.archiveView {
		t.Fatal("4 should open the archive on billing")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "opencode · ") || !strings.Contains(ansi.Strip(m.headerLine(118)), "opencode") {
		t.Errorf("the archive should say the tool on billing's row and header:\n%s\n%s", ansi.Strip(m.headerLine(118)), view)
	}
}

// An opening quote with nothing after it goes with the mark: a band row of
// a long name never spends cells on `· "…` (#80).
func TestAClipNeverEndsOnAnOpeningQuote(t *testing.T) {
	if got := clip(`checkout-flake-hunt · "the checkout suite`, 24); got != "checkout-flake-hunt…" {
		t.Errorf("clip at 24 = %q", got)
	}
	if got := clip(`say "hi" · then`, 12); got != `say "hi"…` {
		t.Errorf("a closed quote stays: %q", got)
	}
	m := sceneModel(sceneSecondDay(), 100, 30)
	if view := ansi.Strip(m.View()); strings.Contains(view, `· "…`) {
		t.Errorf("the band spends cells on an empty prompt clause:\n%s", view)
	}
}

// The band takes every row the column has: the two the archive's line gave
// up are the band's (#81).
func TestTheBandFillsTheRowsTheArchiveLineGaveUp(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 100, 30)
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	last := ""
	for _, l := range lines {
		if strings.Contains(l, " ○ ") && strings.Contains(l, " · ") {
			last = l
		}
	}
	if !strings.Contains(last, "9 ○") {
		t.Errorf("the band should reach its ninth row at 100x30, last band row %q:\n%s", last, strings.Join(lines, "\n"))
	}
}

// A session waiting on you but dead on the API is not one g would grab: the
// key is not offered for it (#81).
func TestGrabIsNotOfferedForADeadQuestion(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFewOngoing(), 120, 34)
	for i := range m.sessions {
		if m.sessions[i].Snap.State == state.NeedsYou {
			m.sessions[i].Snap.APIError = true
		}
	}
	if m.anyNeedsYou() {
		t.Fatal("a dead question should not count as waiting")
	}
	if foot := ansi.Strip(m.footerLine(118)); strings.Contains(foot, "g grab") {
		t.Errorf("g is offered for a question dead on the API: %q", foot)
	}
}

// The one live session says its tool where the fleet compass holds — live
// and archived — runs two (#82).
func TestTheLiveRowSaysItsToolWhereTheArchiveRunsAnother(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 120, 34)
	if head := ansi.Strip(m.headerLine(118)); !strings.Contains(head, "1 hello · claude") {
		t.Errorf("the header should name the live session's tool: %q", head)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "claude · ⌁ main") {
		t.Errorf("the card should name the tool beside the pane:\n%s", view)
	}
	one := sceneModel(sceneFirstSession(), 120, 34)
	if head := ansi.Strip(one.headerLine(118)); strings.Contains(head, "claude") {
		t.Errorf("a fleet of one claude needs no word: %q", head)
	}
}

// At Lv1 the page keys are offered only where the trail beside the list
// is long enough to page (#83).
func TestThePageKeysAreOfferedOnlyWhereTheTrailPages(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 152, 40)
	pressTab(m)
	press(m, "A")
	if m.level != levelTrail || !m.archiveView {
		t.Fatalf("expected the archive's list, level %d archive %v", m.level, m.archiveView)
	}
	if total, h, _ := m.trailView(); total > h {
		t.Skip("the archive's trail pages here; nothing to pin")
	}
	if foot := ansi.Strip(m.footerLine(150)); strings.Contains(foot, "ctrl+d/u") {
		t.Errorf("the page keys are offered where the trail fits: %q", foot)
	}
	// The non-board Lv1 keymap never offered the keys; the board deck's
	// archive list is the one place they were offered inert.

}

// The compact help names what the page keys page (#83).
func TestTheCompactHelpNamesWhatThePageKeysPage(t *testing.T) {
	for _, w := range []int{80, 100} {
		m := sceneModel(sceneFleetHygiene(), w, 30)
		press(m, "?")
		if view := ansi.Strip(m.View()); !strings.Contains(view, "page the trail") && !strings.Contains(view, "pages the trail") {
			t.Errorf("at %d the help's page keys name no object:\n%s", w, view)
		}
	}
}
