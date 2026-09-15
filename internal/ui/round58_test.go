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
}
