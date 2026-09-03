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
)

// Round sixteen: the fleet of one's way out of a search, the narrow trace's
// bytes, the loop's row without a board, and the looks compass takes.

// A fleet of one with a standing search the session fails clears it on the
// zoom-out refusal, since there is no board to go out to.
func TestTheOnlySessionClearsAFailingQuery(t *testing.T) {
	forceASCII(t)
	m := New(nil)
	m.SetSize(152, 40)
	s := sess("s-hello", "hello", "/home/user/hello", "main", "add a --version flag", state.Working, fixtureBase, journey.Scout, "", "tool call in flight", "thinking…")
	m.Update(fleetMsg{sessions: []fleet.Session{s}, at: fixtureBase.Add(40 * time.Second), trails: map[string]journey.Trail{}})
	press(m, "/")
	for _, r := range "zzz" {
		press(m, string(r))
	}
	press(m, "enter")
	press(m, "esc")
	if m.fleetQuery != "" || m.note != "search cleared" {
		t.Errorf("the refusal should clear the query: %q, note %q", m.fleetQuery, m.note)
	}
	// G on a one-row trail says so.
	press(m, "G")
	if m.note != "the trail is one row" {
		t.Errorf("G on a one-row trail: %q", m.note)
	}
}

// Below the board's width m does not flip the mirror, and says so every
// time; in the archive it refuses too.
func TestTheMirrorNeverFlipsOutOfSight(t *testing.T) {
	forceASCII(t)
	m := groupedModel(100, 30)
	press(m, "m")
	press(m, "m")
	if m.showMirror || !strings.Contains(m.note, "needs 110") {
		t.Errorf("m below 110: showMirror %v, note %q", m.showMirror, m.note)
	}
	m2 := boardModel(152, 40)
	press(m2, "A")
	press(m2, "m")
	if m2.showMirror || !strings.Contains(m2.note, "no mirror in the archive") {
		t.Errorf("m in the archive: showMirror %v, note %q", m2.showMirror, m2.note)
	}
}

// A narrow typed-line trace keeps its bytes over the clock; an answer keeps
// its digit and the clock; the digest rides after the trace when it fits.
func TestTheNarrowTraceKeepsTheBytes(t *testing.T) {
	m := boardModel(152, 40)
	api := sessionKey("s-api")
	m.sent[api] = sentReply{text: "/login", at: m.now}
	s := m.sessions[rowFor(t, m, api).sess]
	if row := ansi.Strip(m.boardDelta(api, s, 18)); row != `↪ sent "/login"` {
		t.Errorf("a narrow typed trace keeps its bytes: %q", row)
	}
	m.sent[api] = sentReply{text: "office CIDR", at: m.now, answer: 1}
	if row := ansi.Strip(m.boardDelta(api, s, 22)); row != "↪ answered 1 · 0s ago" {
		t.Errorf("a narrow answered trace keeps the digit and the clock: %q", row)
	}
	m.seen = map[string]time.Time{api: fixtureBase.Add(10 * time.Minute)}
	if row := ansi.Strip(m.boardDelta(api, s, 90)); !strings.Contains(row, `↪ answered 1 · "office CIDR" · 0s ago · ↳`) {
		t.Errorf("the digest should ride after the trace when the row has room: %q", row)
	}
}

// Without a board, a loop's row says the loop: the verdict with its count.
func TestTheLoopsRowSaysTheLoop(t *testing.T) {
	forceASCII(t)
	m := boardModel(100, 30)
	base := fixtureBase
	loop := hourlyTrail(base, 3)
	for i := range loop.Legs {
		loop.Legs[i].Class, loop.Legs[i].Current = journey.Test, false
		loop.Legs[i].Waypoints = []journey.Waypoint{
			{Kind: journey.WaypointTestRun, Text: "18 passed · 1 failed", Short: "18✓ 1✗", At: loop.Legs[i].End},
			{Kind: journey.WaypointTestFail, Text: "test_x", Runs: i + 1, At: loop.Legs[i].End},
		}
	}
	api := sessionKey("s-api")
	m.trails[api] = loop
	if view := ansi.Strip(m.View()); !strings.Contains(view, "✗ red 18✓ 1✗ · 3rd failure") {
		t.Errorf("the loop's fleet row should carry the count:\n%s", view)
	}
}

// Only a session the person opened has its look committed on leaving; an
// archive round-trip lands quietly; the panel stands above a row it would
// otherwise cover.
func TestTheLooksAndThePanelsPlace(t *testing.T) {
	forceASCII(t)
	m := groupedModel(100, 30)
	m.seen = map[string]time.Time{sessionKey("s-webapp"): fixtureBase.Add(5 * time.Minute)}
	m.pointQuiet(sessionKey("s-webapp")) // compass landed here; the person did not open it
	m.point(sessionKey("s-infra"))
	if !m.seen[sessionKey("s-webapp")].Equal(fixtureBase.Add(5 * time.Minute)) {
		t.Error("leaving a session the person never opened committed its look")
	}
	m.point(sessionKey("s-api"))
	press(m, "A")
	press(m, "A")
	if at, ok := m.seen[sessionKey("s-api")]; ok && at.Equal(m.now) && !m.opened[sessionKey("s-api")] {
		t.Error("an archive round-trip read the session it landed on")
	}
	// The panel above the row.
	b := boardModel(120, 34)
	b.point(sessionKey("s-tfstate")) // a second-band column at 120
	press(b, "r")
	view := strings.Split(ansi.Strip(b.View()), "\n")
	row, top := -1, -1
	for i, l := range view {
		if strings.Contains(l, "▸") && strings.Contains(l, "tfstate") && row < 0 {
			row = i
		}
		if strings.Contains(l, "┌ reply to") {
			top = i
		}
	}
	if row < 0 || top < 0 {
		t.Fatalf("no panel or row drawn:\n%s", strings.Join(view, "\n"))
	}
	if top < row && row-top < 3 {
		t.Errorf("the panel stands on its own row (row %d, panel top %d)", row, top)
	}
}

// The footer promises no attach without a pane, in the archive too; the
// hide note is the strip's own form; the help's aside goes whole.
func TestRoundSixteenWords(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	press(m, "A")
	if foot := ansi.Strip(m.View()); !strings.Contains(foot, "enter · no pane") {
		t.Errorf("an archived row with no pane promises no attach:\n%s", foot)
	}
	press(m, "A")
	m.point(sessionKey("s-webapp"))
	press(m, "x")
	if !strings.HasPrefix(m.note, "3 webapp hidden · A, then x") {
		t.Errorf("the hide note is the strip's form: %q", m.note)
	}
	help := strings.Join(helpKeyLinesFor(76, false), "\n")
	if !strings.Contains(help, "a typed line, stop") || strings.Contains(help, "a dead session's r…") || strings.Contains(ansi.Strip(help), "stop; a d…") {
		t.Errorf("the help's aside sheds whole at 80:\n%s", help)
	}
	_ = tea.KeyEnter
}
