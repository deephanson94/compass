package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/transcript"
)

// Round thirteen, the drawing half: rules before ticks, the tag's row, the
// panel's box, the search that survives its own narrowing.

// The read-line and the compaction rule draw before a tick as before any
// other row: a look that fell in front of a tick never drew on a long
// trail, while the board billed for it.
func TestTheReadLineDrawsBeforeATick(t *testing.T) {
	base := fixtureBase
	tr := journey.Trail{Prompts: []journey.Prompt{{Text: "port it", At: base}}}
	for i := 0; i < 6; i++ {
		start := base.Add(time.Duration(i)*10*time.Minute + time.Minute)
		leg := journey.Leg{Class: journey.Build, Label: "thing", Start: start, End: start.Add(5 * time.Minute)}
		if i%2 == 0 {
			leg.Files = []string{"x.go"} // a row; the odd legs are ticks
		}
		tr.Legs = append(tr.Legs, leg)
	}
	looked := tr.Legs[2].End.Add(time.Minute) // between a row and the tick after it
	got := ansi.Strip(RenderTrail(tr, TrailOpts{Now: base.Add(2 * time.Hour), Width: 60, Height: 30, Level: levelTrail, Cursor: -1, Looked: looked, Dense: true}))
	if !strings.Contains(got, "you were here") {
		t.Errorf("the read-line did not draw before a tick:\n%s", got)
	}
	// Both rules in one gap: both drawn, in time order.
	tr.Compactions = []time.Time{looked.Add(30 * time.Second)}
	got = ansi.Strip(RenderTrail(tr, TrailOpts{Now: base.Add(2 * time.Hour), Width: 60, Height: 30, Level: levelTrail, Cursor: -1, Looked: looked, Dense: true}))
	look, compact := strings.Index(got, "you were here"), strings.Index(got, "context compacted")
	if look < 0 || compact < 0 || look > compact {
		t.Errorf("both rules should draw, the look first:\n%s", got)
	}
}

// The third row is the tag's row: the digest takes its left, the tmux
// session its right, and neither evicts the other.
func TestTheDigestAndTheTagShareTheThirdRow(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	api := sessionKey("s-api")
	m.seen = map[string]time.Time{api: fixtureBase.Add(10 * time.Minute)}
	col := m.columnHeader(api, rowFor(t, m, api), 60)
	third := ansi.Strip(col[2])
	if !strings.Contains(third, "↳ ") || !strings.HasSuffix(strings.TrimRight(third, " "), "⌁ dev:1.0") {
		t.Errorf("row three should carry the digest and end on the tag: %q", third)
	}
	if strings.Contains(ansi.Strip(col[1]), "⌁") {
		t.Errorf("the tag is not on row two: %q", col[1])
	}
}

// An answer is traced as the digit it pressed, not as a typed line.
func TestAnAnswerIsTracedAsTheDigit(t *testing.T) {
	forceASCII(t)
	m := followModel(152, 40)
	rec := &recordingTmux{}
	m.runner = rec
	api := sessionKey("s-api")
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == api {
			m.sessions[i].Snap = state_NeedsYou(fixtureBase.Add(30*time.Minute), "Narrow or wide? [narrow / wide]")
		}
	}
	m.SetEvents([]transcript.Event{{Type: transcript.EventAssistant, Timestamp: fixtureBase.Add(30 * time.Minute),
		ToolUses: []transcript.ToolUse{{ID: "q", Name: "AskUserQuestion", Input: []byte(`{"questions":[{"question":"Narrow or wide?","options":[{"label":"narrow"},{"label":"wide"}]}]}`)}}}})
	press(m, "r")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	if cmd == nil {
		t.Fatal("1 sent nothing")
	}
	m.Update(cmd())
	if !strings.Contains(m.note, "↪ answered 1 ·") {
		t.Errorf("the note should say which digit went: %q", m.note)
	}
	if row := ansi.Strip(m.boardDelta(api, m.sessions[rowFor(t, m, api).sess], 60)); !strings.Contains(row, `↪ answered 1 · "narrow"`) {
		t.Errorf("the trace should say the digit: %q", row)
	}
	// Narrow: the quote outlives the age.
	if row := ansi.Strip(m.boardDelta(api, m.sessions[rowFor(t, m, api).sess], 26)); !strings.Contains(row, `"narrow"`) || strings.Contains(row, "ago") {
		t.Errorf("a narrow trace keeps the quote and sheds the age: %q", row)
	}
}

// Typing a fleet search survives its own narrowing: the selection moves to
// the match, and the keys keep going into the query.
func TestTypingASearchSurvivesTheSelectionMoving(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 40)
	m.point(sessionKey("s-infra"))
	press(m, "/")
	for _, r := range "flake" {
		press(m, string(r))
	}
	if !m.searching || m.draft != "flake" || m.selectedKey != sessionKey("s-webapp") {
		t.Fatalf("the search should still be typing after the selection moved: searching %v, draft %q, selected %q", m.searching, m.draft, m.selectedKey)
	}
	if foot := ansi.Strip(m.View()); !strings.Contains(foot, "/flake▏ · enter keeps it") {
		t.Errorf("the footer should echo the query as typed:\n%s", foot)
	}
	press(m, "enter")
	if m.fleetQuery != "flake" || m.searching {
		t.Errorf("enter keeps the query: %q, searching %v", m.fleetQuery, m.searching)
	}
}

// A spelled-out question breaks at its options, never inside them.
func TestTheQuestionWrapsAtItsOptions(t *testing.T) {
	rows := wrapQuestion("to the office CIDR only, or keep the bastion? [office CIDR / keep bastion]", 34, 4)
	if rows[len(rows)-1] != "[office CIDR / keep bastion]" {
		t.Errorf("the options take a row of their own: %q", rows)
	}
	rows = wrapQuestion("keep the bastion? [office CIDR only / keep the bastion as it is]", 30, 4)
	for _, r := range rows {
		if strings.Contains(r, "[") && !strings.Contains(r, "/") && strings.Count(r, "[") > strings.Count(r, "]") && !strings.HasPrefix(r, "[") {
			t.Errorf("a row ends inside the brackets: %q", rows)
		}
	}
	if last := rows[len(rows)-1]; !strings.HasPrefix(last, "/ ") {
		t.Errorf("a broken option list begins its next row on the separator: %q", rows)
	}
}

// Beside a fleet list the reply panel leaves the list legible.
func TestThePanelLeavesTheFleetLegible(t *testing.T) {
	forceASCII(t)
	m := groupedModel(80, 24)
	m.replies = []string{"please continue"}
	press(m, "r")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "needs you") || !strings.Contains(view, "┌ reply to") {
		t.Errorf("the fleet row should survive beside the panel:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "┌ reply to") && strings.Index(line, "┌") < 34 {
			t.Errorf("the panel stands on the fleet: %q", line)
		}
	}
}

// Below the board's width a send leaves its trace under the fleet row.
func TestTheTraceRidesUnderTheFleetRow(t *testing.T) {
	forceASCII(t)
	m := groupedModel(100, 30)
	m.runner = &recordingTmux{}
	m.replies = []string{"please continue"}
	press(m, "r")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	if cmd == nil {
		t.Fatal("1 sent nothing")
	}
	m.Update(cmd())
	if view := ansi.Strip(m.View()); !strings.Contains(view, `↪ sent "please continue"`) {
		t.Errorf("the fleet entry should carry the trace:\n%s", view)
	}
}

// The overlap line is in the narrow fleet too, above the archive row.
func TestOverlapsAreNamedInTheNarrowFleet(t *testing.T) {
	forceASCII(t)
	m := boardModel(100, 30)
	base := fixtureBase
	shared := func(name string) journey.Trail {
		return journey.Trail{Legs: []journey.Leg{{Class: journey.Build, Label: name, Start: base.Add(30 * time.Minute), End: base.Add(35 * time.Minute), Files: []string{"tokens.py"}}}}
	}
	m.trails[sessionKey("s-api")] = shared("api")
	m.trails[sessionKey("s-webapp")] = shared("webapp")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "⚠") || !strings.Contains(view, "both touched") {
		t.Errorf("the narrow fleet should name the overlap:\n%s", view)
	}
}

// A band is as tall as its tallest trail, and the strip follows it: a
// board of short trails does not run its rails to the floor.
func TestABandIsAsTallAsItsTallestTrail(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 48)
	_, heights := m.boardPack(4, 35, 41)
	used := 0
	for _, h := range heights {
		used += h + 1
	}
	if used >= 41 {
		t.Errorf("bands of short trails took the whole height: %v", heights)
	}
	lines := m.boardLines(148, 41)
	strip := -1
	for i, l := range lines {
		if strings.Contains(ansi.Strip(l), "archived · A browses") {
			strip = i
		}
	}
	if strip < 0 || strip != used {
		t.Errorf("the strip should sit one row under the last band (bands %v, strip at %d)", heights, strip)
	}
}

// A lone column takes the width; a row of them is capped.
func TestALoneColumnTakesTheWidth(t *testing.T) {
	if _, w := boardColumns(220, 1); w != 220 {
		t.Errorf("one column at 220 is %d wide", w)
	}
	if _, w := boardColumns(220, 3); w != boardColMax {
		t.Errorf("three columns at 220 are %d wide, want the cap", w)
	}
}

// The reader's title keeps air between the name and the anchored row, and a
// wrapped turn keeps its clock.
func TestTheReaderTitleAndTheWrappedTurn(t *testing.T) {
	forceASCII(t)
	m := followModel(152, 40)
	m.anchor, m.anchorAt, m.anchorText = 3, fixtureBase.Add(30*time.Minute), "Another Claude session sent a message: the encoder is in, run the gates"
	title := ansi.Strip(m.readerTitle(70))
	if !strings.Contains(title, "api   ") {
		t.Errorf("the name runs into the row: %q", title)
	}
	ev := []transcript.Event{{Type: transcript.EventUser, Timestamp: fixtureBase.Add(30 * time.Minute),
		Text: "Another Claude session sent a message: the encoder is in, run the gates now please"}}
	doc := readerDoc(ev, ReaderOpts{Width: 60})
	for _, l := range doc {
		if strings.HasPrefix(l.text, glyphSaid) {
			if !strings.HasSuffix(strings.TrimRight(ansi.Strip(l.text), " "), fixtureBase.Add(30*time.Minute).Local().Format("15:04")) {
				t.Errorf("a wrapped turn lost its clock: %q", l.text)
			}
			break
		}
	}
	_ = lipgloss.Width
}

func state_NeedsYou(since time.Time, activity string) state.Snapshot {
	return state.Snapshot{State: state.NeedsYou, Since: since, Activity: activity}
}
