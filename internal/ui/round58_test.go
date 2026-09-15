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
)

// Round fifty-eight, asked for: the opening frame was one session where the
// fleet was three, and `⇧tab` from the archive's list drew the archive's
// own board where the person was looking for the live one (#340, #341).

// r58Fleet is a launch's fleet as the two polls see it: one session that
// spoke inside the recency door, and two that sit in panes but have been
// quiet ten minutes. Before the panes are paired the door alone admits
// the first; after it all three are live.
func r58Fleet() (door, paired []fleet.Session) {
	hello := sess("s-hello", "hello", "/home/user/hello", "main", "add a --version flag", state.Working, fixtureBase, journey.Scout, "", "tool call in flight", "thinking…")
	api := sess("s-api", "api", "/home/user/api", "main", "fix the 401 on token refresh", state.Idle, fixtureBase.Add(-10*time.Minute), journey.Test, "", "turn ended", "")
	etl := sess("s-etl", "etl", "/home/user/etl", "main", "dedupe the nightly load", state.Idle, fixtureBase.Add(-12*time.Minute), journey.Build, "", "turn ended", "")
	door = []fleet.Session{hello, gone("s-api", "api", "fix the 401 on token refresh", fixtureBase.Add(-10*time.Minute)),
		gone("s-etl", "etl", "dedupe the nightly load", fixtureBase.Add(-12*time.Minute))}
	paired = []fleet.Session{hello, api, etl}
	return door, paired
}

// TestTheFleetOfOneIsDecidedOnceThePanesArePaired pins #341. The launch's
// first poll runs before Init's pane listing has anything to pair with,
// so its liveness is the recency door's alone: a session in a pane that
// went quiet ten minutes ago is not yet live. The deck read that as a
// fleet of one, opened its session, and the next poll's fleet of three
// stood behind a session view until `⇧tab` found the board. The choice is
// made once, on the first poll that ran after the pairing.
func TestTheFleetOfOneIsDecidedOnceThePanesArePaired(t *testing.T) {
	forceASCII(t)
	door, paired := r58Fleet()
	m := New(nil)
	m.Update(tea.WindowSizeMsg{Width: 152, Height: 40})
	m.Update(fleetMsg{sessions: door, at: fixtureBase, trails: map[string]journey.Trail{}})
	if m.level != levelBoard {
		t.Fatalf("the first, unpaired poll chose a level: %d", m.level)
	}
	m.Update(panesMsg{against: len(door)})
	m.Update(fleetMsg{sessions: paired, at: fixtureBase.Add(time.Second), trails: map[string]journey.Trail{}, paired: true})
	if m.level != levelBoard {
		t.Fatalf("a fleet of three opened on one session: level %d", m.level)
	}
	view := ansi.Strip(m.View())
	for _, name := range []string{"hello", "api", "etl"} {
		if !strings.Contains(view, name) {
			t.Errorf("the opening board does not draw %q:\n%s", name, view)
		}
	}
	if !strings.Contains(view, "h/l columns") {
		t.Errorf("the opening frame is not the board's:\n%s", view)
	}
}

// The held side: a fleet that is one session once the panes are paired
// still opens on that session (#47), on the paired poll and not before.
func TestTheFleetOfOneStillOpensOnItsSessionOncePaired(t *testing.T) {
	forceASCII(t)
	_, paired := r58Fleet()
	one := paired[:1]
	m := New(nil)
	m.Update(tea.WindowSizeMsg{Width: 152, Height: 40})
	m.Update(fleetMsg{sessions: one, at: fixtureBase, trails: map[string]journey.Trail{}})
	if m.level != levelBoard {
		t.Fatalf("the unpaired poll opened the session: level %d", m.level)
	}
	m.Update(panesMsg{against: 1})
	m.Update(fleetMsg{sessions: one, at: fixtureBase.Add(time.Second), trails: map[string]journey.Trail{}, paired: true})
	if m.level != levelWaypoints {
		t.Fatalf("a fleet of one should open on the session view, not level %d", m.level)
	}
	// And decided once: a fleet that grows later is not reopened.
	m.Update(fleetMsg{sessions: paired, at: fixtureBase.Add(2 * time.Second), trails: map[string]journey.Trail{}, paired: true})
	if m.level != levelWaypoints {
		t.Errorf("a later poll moved the level to %d", m.level)
	}
}

// A key pressed before the pairing is the person at the deck: compass
// then chooses no level for them, whatever the paired poll says.
func TestAKeyBeforeThePairingKeepsTheLevel(t *testing.T) {
	forceASCII(t)
	_, paired := r58Fleet()
	one := paired[:1]
	m := New(nil)
	m.Update(tea.WindowSizeMsg{Width: 152, Height: 40})
	m.Update(fleetMsg{sessions: one, at: fixtureBase, trails: map[string]journey.Trail{}})
	press(m, "?")
	pressKey(m, "esc")
	m.Update(panesMsg{against: 1})
	m.Update(fleetMsg{sessions: one, at: fixtureBase.Add(time.Second), trails: map[string]journey.Trail{}, paired: true})
	if m.level != levelBoard {
		t.Errorf("the paired poll took the deck off the level the person was on: %d", m.level)
	}
}

// TestTheArchivesListZoomsOutToTheLiveBoard pins #340: the archive is a
// list under the live fleet, not a fleet with a board of its own. `⇧tab`
// and `esc` on its list land on the live board with the live selection
// back, and no column on that board is an archived session's. Three
// widths the board fits at, and both ways up.
func TestTheArchivesListZoomsOutToTheLiveBoard(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		for _, way := range []string{"shift+tab", "esc"} {
			sc := sceneFleetHygiene()
			m := sceneModel(sc, size[0], size[1])
			live := m.selectedKey
			pressKey(m, "A")
			poll(m, sc)
			for i := 0; i < 5; i++ {
				pressKey(m, "j")
				poll(m, sc)
			}
			if !m.archiveView || m.level != levelTrail {
				t.Fatalf("%dx%d: the stand is not the archive's list: archive %v level %d", size[0], size[1], m.archiveView, m.level)
			}
			where := fmt.Sprintf("%dx%d %s", size[0], size[1], way)
			pressKey(m, way)
			poll(m, sc)
			if m.archiveView {
				t.Errorf("%s: the archive kept the deck", where)
			}
			if m.level != levelBoard {
				t.Errorf("%s: not the board: level %d", where, m.level)
			}
			if m.selectedKey != live {
				t.Errorf("%s: the live selection did not come back: %q, not %q", where, m.selectedKey, live)
			}
			view := ansi.Strip(m.View())
			if strings.Contains(view, "FLEET · archive") || strings.Contains(view, "A live fleet") {
				t.Errorf("%s: the frame is the archive's:\n%s", where, view)
			}
			if !strings.Contains(view, "h/l columns") {
				t.Errorf("%s: the footer is not the live board's:\n%s", where, view)
			}
			for _, s := range sc.sessions {
				if s.Live {
					continue
				}
				if strings.Contains(view, "▸") && strings.Contains(view, "○ "+archiveHeadline(s)+" ") {
					t.Errorf("%s: an archived session has a column:\n%s", where, view)
				}
			}
			if !strings.Contains(view, fmt.Sprintf("%d archived", m.archivedCount())) {
				t.Errorf("%s: the live board's strip does not name the archive:\n%s", where, view)
			}
		}
	}
}

// Where the live fleet is one session, the way up from the archive's
// list is that session, as `A` lands it (#47): there is no board of one.
func TestTheArchivesListZoomsOutToTheOnlySession(t *testing.T) {
	forceASCII(t)
	sc := sceneSecondDay()
	m := sceneModel(sc, 152, 40)
	pressKey(m, "A")
	poll(m, sc)
	pressKey(m, "shift+tab")
	poll(m, sc)
	if m.archiveView || m.level != levelWaypoints {
		t.Errorf("a fleet of one should come back to its session: archive %v level %d", m.archiveView, m.level)
	}
}

// The empty archive — every hidden row brought back — goes the same way
// up: `⇧tab` on it is the live board, not a `no board` refusal over a
// list that then had no way up but `A` (#268's refusal was the archive
// board's; the archive has none).
func TestTheEmptyArchiveZoomsOutToTheLiveBoard(t *testing.T) {
	forceASCII(t)
	_, paired := r58Fleet() // three live, nothing archived
	m := New(nil)
	m.SetSize(120, 34)
	m.SetSessions(paired, fixtureBase.Add(time.Second))
	m.point(sessionKey("s-api")) // idle: hideable
	for _, k := range []string{"x", "A", "x"} {
		pressKey(m, k)
	}
	if !m.archiveView || m.archivedCount() != 0 || m.hiddenCount() != 0 {
		t.Fatalf("the stand is not the empty archive: archive %v, %d archived, %d hidden, note %q", m.archiveView, m.archivedCount(), m.hiddenCount(), m.note)
	}
	if foot := ansi.Strip(m.View()); !strings.Contains(foot, "⇧tab board") {
		t.Errorf("the empty archive's footer does not name the way up its key takes:\n%s", foot)
	}
	pressKey(m, "shift+tab")
	if m.archiveView || m.level != levelBoard {
		t.Errorf("the empty archive did not zoom out to the live board: archive %v level %d, note %q", m.archiveView, m.level, m.note)
	}
}

// The held side: under the board's width the archive's list is at the
// deck's top level, and `⇧tab` there refuses as the live list's does.
// `A fleet` is the named way back.
func TestTheNarrowArchivesListStillRefusesTheZoomOut(t *testing.T) {
	forceASCII(t)
	sc := sceneFleetHygiene()
	m := sceneModel(sc, 80, 24)
	pressKey(m, "A")
	poll(m, sc)
	pressKey(m, "shift+tab")
	poll(m, sc)
	if !m.archiveView || m.level != levelTrail {
		t.Errorf("a narrow deck left the archive on ⇧tab: archive %v level %d", m.archiveView, m.level)
	}
	if want := fmt.Sprintf("no board under %d columns", deckWideCols); m.note != want {
		t.Errorf("note = %q, want %q", m.note, want)
	}
}

// The archive is never at the board's level: a deck narrowed onto its
// list and widened again while the archive is open keeps the list, and
// the live board comes back with `A`.
func TestTheArchiveKeepsItsListWhenTheWidthComesBack(t *testing.T) {
	forceASCII(t)
	sc := sceneFleetHygiene()
	m := sceneModel(sc, 152, 40)
	m.SetSize(80, 24)
	if m.level != levelTrail || !m.boardForced {
		t.Fatalf("the narrow deck is not the forced list: level %d forced %v", m.level, m.boardForced)
	}
	pressKey(m, "A")
	poll(m, sc)
	m.SetSize(152, 40)
	if !m.archiveView || m.level != levelTrail {
		t.Errorf("the width's return put the archive at level %d (archive %v)", m.level, m.archiveView)
	}
	pressKey(m, "A")
	poll(m, sc)
	if m.archiveView || m.level != levelBoard {
		t.Errorf("A did not bring the live board back: archive %v level %d", m.archiveView, m.level)
	}
	// And the debt stands across the visit: narrowed, into the archive,
	// widened, narrowed, out of the archive, widened — the board the
	// width took away comes back with the width (#343).
	m = sceneModel(sc, 152, 40)
	m.SetSize(80, 24)
	pressKey(m, "A")
	poll(m, sc)
	m.SetSize(152, 40)
	m.SetSize(80, 24)
	pressKey(m, "A")
	poll(m, sc)
	m.SetSize(152, 40)
	if m.archiveView || m.level != levelBoard {
		t.Errorf("the width came back and the board did not: archive %v level %d forced %v", m.archiveView, m.level, m.boardForced)
	}
}

// ---- the panel's findings on the round, folded (#342, #343) ----

// r58type types a fleet search and keeps it, one rune at a time as a
// terminal sends them.
func r58type(m *Model, sc scene, query string) {
	pressKey(m, "/")
	for _, r := range query {
		pressKey(m, string(r))
	}
	pressKey(m, "enter")
	poll(m, sc)
}

// TestTheDigitsAreGivenOnThePoll pins #342: the deck that shipped never
// numbered a live session. SetSessions gave the digits and only a harness
// called it; the real deck's fleet arrives by fleetMsg, which gave none,
// so `1` on the opening frame opened the band's first archived row.
func TestTheDigitsAreGivenOnThePoll(t *testing.T) {
	forceASCII(t)
	_, paired := r58Fleet()
	m := New(nil)
	m.Update(tea.WindowSizeMsg{Width: 152, Height: 40})
	m.Update(fleetMsg{sessions: paired, at: fixtureBase, trails: map[string]journey.Trail{}, paired: true})
	if len(m.digits) != 3 {
		t.Fatalf("three live sessions polled, %d numbered: %v", len(m.digits), m.digits)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "1 ") || !strings.Contains(view, "3 ") {
		t.Errorf("the board draws no numbers:\n%s", view)
	}
	press(m, "3")
	if s, ok := m.selected(); !ok || !s.Live || m.archiveView {
		t.Errorf("3 did not select a live session: live %v archive %v", ok && s.Live, m.archiveView)
	}
}

// The digits are the live board's whatever view is open: a poll landing
// while the archive is up numbers no archived row.
func TestAPollInTheArchiveNumbersNoArchivedRow(t *testing.T) {
	forceASCII(t)
	sc := sceneFleetHygiene()
	m := sceneModel(sc, 120, 34)
	pressKey(m, "A")
	poll(m, sc)
	for key := range m.digits {
		if s, ok := m.session(key); !ok || !s.Live {
			t.Errorf("an archived session wears a digit: %s", key)
		}
	}
}

// TestAComesBackToTheLevelItWasPressedOn pins #343's first fold: `A` from
// the session view or the reader comes back to it on a wide deck too, the
// frame it left byte for byte. The board was where an `A` from any depth
// landed while the deck had a board, since the board was the archive's
// way up as well; now `⇧tab` is (#340), and `A` keeps its own word.
func TestAComesBackToTheLevelItWasPressedOn(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		for _, depth := range []int{1, 2} {
			sc := sceneFleetHygiene()
			m := sceneModel(sc, size[0], size[1])
			for i := 0; i < depth; i++ {
				pressKey(m, "tab")
				poll(m, sc)
			}
			from, key := m.level, m.selectedKey
			before := ansi.Strip(m.View())
			pressKey(m, "A")
			poll(m, sc)
			pressKey(m, "A")
			poll(m, sc)
			where := fmt.Sprintf("%dx%d from Lv%d", size[0], size[1], from)
			if m.archiveView || m.level != from || m.selectedKey != key {
				t.Errorf("%s: A came back to Lv%d on %q (archive %v)", where, m.level, m.selectedKey, m.archiveView)
			}
			if after := ansi.Strip(m.View()); after != before {
				t.Errorf("%s: the frame did not come back:\n%s\n---\n%s", where, before, after)
			}
		}
	}
}

// `g` in the archive leaves it as `A` does: at the level `A` was pressed
// on. Left at the list's level, a wide deck drew the live list where its
// board fits, with `⇧tab board` on its footer.
func TestGrabOutOfTheArchiveLandsAsADoes(t *testing.T) {
	forceASCII(t)
	_, paired := r58Fleet()
	asking := sess("s-ask", "ask", "/home/user/ask", "main", "which port", state.NeedsYou, fixtureBase.Add(-7*time.Minute), journey.Build, "", "asked a question", "")
	m := New(nil)
	m.runner = &recordingTmux{}
	m.SetSize(120, 34)
	m.SetSessions(append([]fleet.Session{asking}, paired...), fixtureBase.Add(time.Second))
	m.point(sessionKey("s-etl"))
	press(m, "x") // etl off the board: the archive has a row
	press(m, "A")
	if !m.archiveView {
		t.Fatalf("the archive did not open: note %q", m.note)
	}
	press(m, "g")
	if m.archiveView || m.level != levelBoard {
		t.Errorf("g left the deck at level %d (archive %v), not the board it was pressed from", m.level, m.archiveView)
	}
	if m.selectedKey != sessionKey("s-ask") {
		t.Errorf("g did not grab the asking session: %q", m.selectedKey)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "h/l columns") {
		t.Errorf("the frame after g is not the board's:\n%s", view)
	}
}

// A search typed in the archive comes out onto the live board only where
// the live selection answers it; where it does not, the board it would
// draw is `no session matches` over its keys and nothing else, so the
// search goes as `zoomOut` drops it out of a session view.
func TestTheArchivesSearchStaysBehindWhereTheLiveSelectionFailsIt(t *testing.T) {
	forceASCII(t)
	sc := sceneFleetHygiene()
	m := sceneModel(sc, 120, 34)
	live, _ := m.selected()
	pressKey(m, "A")
	poll(m, sc)
	r58type(m, sc, "zzz")
	pressKey(m, "shift+tab")
	poll(m, sc)
	if m.archiveView || m.level != levelBoard || m.fleetQuery != "" || m.note != "search cleared" {
		t.Errorf("the way up kept a search the board answers with nothing: archive %v level %d query %q note %q", m.archiveView, m.level, m.fleetQuery, m.note)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "h/l columns") || strings.Contains(view, "no session matches") {
		t.Errorf("the board after the way up:\n%s", view)
	}
	// And kept where the live selection answers it.
	m = sceneModel(sc, 120, 34)
	pressKey(m, "A")
	poll(m, sc)
	r58type(m, sc, sessionName(live.Info))
	pressKey(m, "shift+tab")
	poll(m, sc)
	if m.archiveView || m.fleetQuery != sessionName(live.Info) || m.selectedKey != live.Info.Key() {
		t.Errorf("a search the live selection answers did not come along: query %q selected %q", m.fleetQuery, m.selectedKey)
	}
	if m.querySel != "" {
		if s, ok := m.session(m.querySel); !ok || !m.onBoard(s) {
			t.Errorf("the search remembers an archive row to go back to: %q", m.querySel)
		}
	}
}

// A fleet of one is one whatever the search says: the way up lands on its
// session, never on a board of one column (#47), with the search it
// failed gone.
func TestAFleetOfOneWithASearchComesBackToItsSession(t *testing.T) {
	forceASCII(t)
	sc := sceneSecondDay()
	m := sceneModel(sc, 152, 40)
	pressKey(m, "A")
	poll(m, sc)
	r58type(m, sc, "dedupe")
	pressKey(m, "shift+tab")
	poll(m, sc)
	if m.archiveView || m.level != levelWaypoints || m.fleetQuery != "" {
		t.Errorf("archive %v level %d query %q", m.archiveView, m.level, m.fleetQuery)
	}
}

// Under the board's width the archive's list of a fleet of one refuses as
// the live list does: there is no board at any width to promise.
func TestTheNarrowArchiveOfAFleetOfOneRefusesLikeTheLiveList(t *testing.T) {
	forceASCII(t)
	sc := sceneSecondDay()
	m := sceneModel(sc, 80, 24)
	pressKey(m, "A")
	poll(m, sc)
	pressKey(m, "shift+tab")
	poll(m, sc)
	if !m.archiveView || m.note != "nothing to zoom out to" {
		t.Errorf("archive %v note %q", m.archiveView, m.note)
	}
}

// A launch that finds nothing on disk is settled: a session that appears
// minutes later is drawn on the board, and the deck does not zoom itself
// into it while the person is looking at it.
func TestAnEmptyLaunchIsSettled(t *testing.T) {
	forceASCII(t)
	_, paired := r58Fleet()
	m := New(nil)
	m.Update(tea.WindowSizeMsg{Width: 152, Height: 40})
	m.Update(fleetMsg{sessions: nil, at: fixtureBase})
	m.Update(panesMsg{against: 1})
	m.Update(fleetMsg{sessions: paired[:1], at: fixtureBase.Add(2 * time.Minute), trails: map[string]journey.Trail{}, paired: true})
	if m.level != levelBoard {
		t.Errorf("a session that appeared after an empty launch opened itself: level %d", m.level)
	}
}
