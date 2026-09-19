package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Round sixty-six: the standing search (#396). A search keeps every alarm
// on the board and says how many stayed; it lands on what it found, not
// on what stayed; a lane's `→N` is the session's own digit and survives a
// search that leaves that session out, and the digit still lands; the
// tool and the model answer the search; and `h`/`l` at Lv3 comes back to
// the lane and the row it left.

var round66Widths = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// Every session that owes an alarm stays in the view under a query it
// does not answer, at every width, and the header counts what stayed.
func TestASearchKeepsEveryAlarm(t *testing.T) {
	forceASCII(t)
	sc := sceneAlarmStorm()
	for _, wh := range round66Widths {
		m := sceneModel(sc, wh[0], wh[1])
		for _, k := range []string{"/", "zzqqnothing", "enter"} {
			pressKey(m, k)
			poll(m, sc)
		}
		alarms, shown := 0, map[string]bool{}
		for _, i := range m.viewOrder() {
			shown[m.sessions[i].Info.Key()] = true
		}
		for _, s := range m.sessions {
			if !m.onBoard(s) || !m.alarmed(s) {
				continue
			}
			alarms++
			if !shown[s.Info.Key()] {
				t.Errorf("%dx%d: %s owes an alarm and the search hid it", wh[0], wh[1], sessionName(s.Info))
			}
		}
		if alarms < 4 {
			t.Fatalf("%dx%d: the storm has %d alarms; the scene is not the one", wh[0], wh[1], alarms)
		}
		if n := len(m.viewOrder()); n != alarms {
			t.Errorf("%dx%d: the view holds %d rows under a query nothing answers, not the %d alarms", wh[0], wh[1], n, alarms)
		}
		if wh[0] < 120 {
			continue // the header sheds the search clause before the chips, and the list scrolls
		}
		head := ansi.Strip(m.headerLine(wh[0] - 2))
		if want := fmt.Sprintf("/zzqqnothing · 0 of %d · %d stay", len(m.sessions)-m.hiddenCount()-archivedIn(m), alarms); !strings.Contains(head, want) {
			t.Errorf("%dx%d: the header does not count what stayed: want %q in %q", wh[0], wh[1], want, head)
		}
		// The frame names them: every alarm's name is on the board, not
		// only in the chips.
		view := ansi.Strip(m.View())
		for _, s := range m.sessions {
			if m.onBoard(s) && m.alarmed(s) && !strings.Contains(view, sessionName(s.Info)) {
				t.Errorf("%dx%d: %s stayed and the frame does not name it:\n%s", wh[0], wh[1], sessionName(s.Info), view)
			}
		}
	}
}

func archivedIn(m *Model) int {
	n := 0
	for _, s := range m.sessions {
		if !s.Live {
			n++
		}
	}
	return n
}

// The search lands on what it found: with the needs-you session standing
// first, `/flake` selects the session with the flake, and a query nothing
// answers leaves the selection where it was.
func TestASearchLandsOnWhatItFound(t *testing.T) {
	forceASCII(t)
	sc := sceneTwoTools()
	m := sceneModel(sc, 152, 40)
	pressKey(m, "1") // infra, the needs-you session
	if s, _ := m.selected(); sessionName(s.Info) != "infra" {
		t.Fatalf("the stand is not on infra: %s", sessionName(s.Info))
	}
	for _, k := range []string{"/", "rate limiting"} {
		pressKey(m, k)
	}
	if s, _ := m.selected(); sessionName(s.Info) != "api" || s.Info.ToolName() != "opencode" {
		t.Errorf("/rate limiting did not land on the opencode api session: %s (%s)", sessionName(s.Info), s.Info.ToolName())
	}
	pressKey(m, "esc")
	pressKey(m, "3") // the claude api session, working
	was := m.selectedKey
	for _, k := range []string{"/", "zzz", "enter"} {
		pressKey(m, k)
		poll(m, sc)
	}
	if m.selectedKey != was {
		t.Errorf("a query nothing answers moved the selection off session 3")
	}
	if head := ansi.Strip(m.headerLine(150)); !strings.Contains(head, "3 api") || !strings.Contains(head, "/zzz · 0 of 4 · 1 stay") {
		t.Errorf("the header does not say the selection, the miss and what stayed: %q", ansi.Strip(m.headerLine(150)))
	}
}

// A lane's `→N` is the session's own digit: under a search that leaves
// the linked session out, the lane keeps its link and the digit lands.
func TestALanesLinkSurvivesTheSearch(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	for _, wh := range [][2]int{{152, 40}, {220, 48}} {
		w := wh[0]
		m := sceneModel(sc, w, wh[1])
		pressKey(m, "2")
		poll(m, sc)
		linked := regexp.MustCompile(`Red-team the plugin archite\S*\s+→1`)
		if view := ansi.Strip(m.View()); !linked.MatchString(view) {
			t.Fatalf("%d: the stand is not the linked lane:\n%s", w, view)
		}
		for _, k := range []string{"/", "porter", "enter"} {
			pressKey(m, k)
			poll(m, sc)
		}
		view := ansi.Strip(m.View())
		if !linked.MatchString(view) {
			t.Errorf("%d: /porter struck the lane's link:\n%s", w, view)
		}
		pressKey(m, "1")
		poll(m, sc)
		if s, _ := m.selected(); sessionName(s.Info) != "harness" {
			t.Errorf("%d: `1` under /porter did not land on the linked session: %s", w, sessionName(s.Info))
		}
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		if foot := rows[len(rows)-1]; !strings.Contains(foot, "1 harness is outside /porter · esc clears it") {
			t.Errorf("%d: the note does not say the search left session 1 out: %q", w, foot)
		}
	}
}

// The tool and the model answer the search, on the scene named for the
// question.
func TestTheToolAndTheModelAnswerTheSearch(t *testing.T) {
	forceASCII(t)
	sc := sceneTwoTools()
	for _, c := range []struct {
		query string
		want  int
	}{{"opencode", 2}, {"opus", 1}, {"gpt-5", 1}, {"claude", 3}} {
		m := sceneModel(sc, 152, 40)
		for _, k := range []string{"/", c.query, "enter"} {
			pressKey(m, k)
			poll(m, sc)
		}
		matched := 0
		for _, i := range m.viewOrder() {
			if m.matchesQuery(m.sessions[i]) {
				matched++
			}
		}
		if matched != c.want {
			t.Errorf("/%s matched %d sessions, want %d", c.query, matched, c.want)
		}
	}
}

// `l` from the lane reader to the teammate's own session and `h` back
// lands on the lane and the row it left, not on the lead's present.
func TestHLComesBackToTheLaneItLeft(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	m := sceneModel(sc, 152, 40)
	for _, k := range []string{"2", "tab", "G", "k", "k", "tab"} {
		pressKey(m, k)
		poll(m, sc)
	}
	if m.level != levelReader || m.readerLane == "" {
		t.Fatalf("the stand is not a lane's reader: level %d lane %q", m.level, m.readerLane)
	}
	lane, cursor, anchorAt := m.readerLane, m.cursor, m.anchorAt
	title := ansi.Strip(m.readerTitle(80))
	pressKey(m, "l")
	poll(m, sc)
	if s, _ := m.selected(); sessionName(s.Info) != "harness" || m.readerLane != "" {
		t.Fatalf("`l` did not open the neighbour's own reader: %s lane %q", sessionName(s.Info), m.readerLane)
	}
	pressKey(m, "h")
	poll(m, sc)
	if s, _ := m.selected(); sessionName(s.Info) != "porter" {
		t.Fatalf("`h` did not come back: %s", sessionName(s.Info))
	}
	if m.readerLane != lane || m.cursor != cursor {
		t.Errorf("`h` came back to lane %q row %d, not lane %q row %d", m.readerLane, m.cursor, lane, cursor)
	}
	if !m.anchorAt.Equal(anchorAt) {
		t.Errorf("the reader's mark moved: %v, was %v", m.anchorAt, anchorAt)
	}
	if got := ansi.Strip(m.readerTitle(80)); got != title {
		t.Errorf("the reader's title changed on the way back: %q, was %q", got, title)
	}
	// A digit still lands on the present (#70): `1`, then `2` back, is
	// not a return.
	for _, k := range []string{"1", "2"} {
		pressKey(m, k)
		poll(m, sc)
	}
	if m.readerLane != "" {
		t.Errorf("a digit came back to the lane; only h/l does (#70): %q", m.readerLane)
	}
}
