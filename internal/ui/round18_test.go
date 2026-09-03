package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Round eighteen of the operator review: what the newcomer, the very-long,
// the subagents, the alarm-storm and the fleet-hygiene operators found.

// Below the board's width the legs and the reader name the help and the
// way out too: at 80 and 100 no session-view footer said `?` or `q`.
func TestTheNarrowSessionFootersNameTheWayOut(t *testing.T) {
	m := groupedModel(80, 24)
	for _, level := range []string{"legs", "reader"} {
		pressTab(m)
		foot := ansi.Strip(strings.Split(m.View(), "\n")[23])
		if !strings.HasSuffix(strings.TrimSpace(foot), "? help · q quit") {
			t.Errorf("the 80-column %s footer does not end on the way out: %q", level, foot)
		}
	}
}

// The keys are shed against the note's final form: a key shed for a clause
// the note then gave up came back — 13 blank columns stood where `r reply`
// had been.
func TestTheKeysComeBackWhenTheNoteSheds(t *testing.T) {
	m := sceneModel(sceneFewOngoing(), 80, 24)
	m.note = `↪ sent "please continue" · to ⌁ ops:0.0`
	foot := ansi.Strip(m.footerLine(78))
	if strings.Contains(foot, "to ⌁") || !strings.Contains(foot, "r reply") || !strings.Contains(foot, `"please continue"`) || !strings.Contains(foot, "? help") {
		t.Errorf("the reply key should come back once the destination is shed: %q", foot)
	}
	m.note = "billing stays · dead on the API"
	if foot := ansi.Strip(m.footerLine(78)); !strings.Contains(foot, "? help") || !strings.Contains(foot, "dead on the API") {
		t.Errorf("a refusal stands whole beside the help at 80: %q", foot)
	}
}

// A note's quote is clipped to the footer's room rather than capped where
// it was made: a 220-column footer cut a prompt at 38 with 23 to spare.
func TestTheNoteQuoteTakesTheRoomTheKeysLeave(t *testing.T) {
	m := sceneModel(sceneFewOngoing(), 220, 48)
	long := strings.Repeat("update the docs and ", 6)
	m.note = `◉ 10/12 · "` + long + `" · 14:47`
	foot := ansi.Strip(m.footerLine(218))
	if !strings.Contains(foot, "14:47") || !strings.Contains(foot, `"update the docs and update`) {
		t.Errorf("the quote should be clipped to its room, the clock kept: %q", foot)
	}
	if lipglossWidth(foot) > 218 {
		t.Errorf("the footer overflows: %d cells", lipglossWidth(foot))
	}
	if got := fitQuote(`◉ 1/2 · "abcdefghij" · 14:47`, 20); got != "" {
		t.Errorf("a quote clipped below nine cells is dropped instead: %q", got)
	}
}

// The fold names the group by its name alone — the header carries the
// group's echo glyph padded to the column — and a fold over a header and
// its air is no fold at all.
func TestTheFoldNamesTheGroupAlone(t *testing.T) {
	lines := []string{" ⌁ work                         ◍", " 2 ◍ etl", "    second", "", " 4 ○ api", "    second"}
	if got := foldedHeader(lines, 1); got != "⌁ work" {
		t.Errorf("the fold should name the group without its echo: %q", got)
	}
	m := groupedModel(120, 30)
	full, selStart, selEnd := m.fleetBlock(m.fleetRows(), fleetWidth)
	m.fleetScroll = 1 // a stale offset just past the first header
	out := m.scrollFleet(full, selStart, selEnd, len(full)-1)
	if strings.Contains(ansi.Strip(out[0]), "0 more above") {
		t.Errorf("a fold of nothing: %q", ansi.Strip(out[0]))
	}
}

// A result that lands after the person's next turn is named at its call —
// "↩ result below" at the call, "↩ result of Bash(…)" where it lands —
// rather than drawn under "❯" as the prompt's reply.
func TestAResultAfterATurnIsNamed(t *testing.T) {
	sc := sceneSubagents()
	for _, s := range sc.sessions {
		if sessionName(s.Info) != "porter" {
			continue
		}
		key := s.Info.Key()
		rows := readerDoc(eventsBehind(sc.trails[key], sc.activity(key)), ReaderOpts{Width: 96, Now: sceneNow})
		var text []string
		for _, r := range rows {
			text = append(text, r.text)
		}
		doc := strings.Join(text, "\n")
		if !strings.Contains(doc, "↩ result of Bash(") || !strings.Contains(doc, "↩ result below") {
			t.Errorf("the late pytest result is not named:\n%s", doc)
		}
		return
	}
	t.Fatal("no porter in the subagents scene")
}

// `m` in the session view says which side it turned to: the third press
// on the same state used to be silent.
func TestTheMirrorToggleSpeaksAtLv2(t *testing.T) {
	m := sceneModel(sceneFewOngoing(), 120, 34)
	pressKey(m, "tab")
	pressKey(m, "m")
	if m.note != "the live pane" {
		t.Errorf("m on: note %q", m.note)
	}
	pressKey(m, "m")
	if m.note != "the conversation" {
		t.Errorf("m off: note %q", m.note)
	}
}

// The narrow help spends its rows on the lanes, not the class names: the
// classes are plain words on every leg row.
func TestTheNarrowHelpKeepsTheLanes(t *testing.T) {
	help := strings.Join(helpLinesFor(98, 25, false), "\n")
	for _, want := range []string{"⋯ out", "⌀ back", "you were here", "trail:"} {
		if !strings.Contains(help, want) {
			t.Errorf("the 100-column help lacks %q:\n%s", want, help)
		}
	}
	if strings.Contains(help, "scout  design") {
		t.Errorf("the 100-column help spends a row on the class names:\n%s", help)
	}
	wide := strings.Join(helpLinesFor(218, 43, true), "\n")
	if strings.Contains(wide, "\u00a0") {
		t.Error("a non-breaking space leaks into the two-column help")
	}
}

// The reply panel sheds its air to stay on its own band's trail rows
// before it moves under the band below.
func TestTheReplyPanelShedsAirBeforeItMoves(t *testing.T) {
	m := sceneModel(sceneAlarmStorm(), 220, 48)
	pressKey(m, "1")
	pressKey(m, "r")
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	row := func(s string) int {
		for i, l := range lines {
			if strings.Contains(l, s) {
				return i
			}
		}
		return -1
	}
	top, head, next := row("┌ reply to 1"), row("▸1 ▲ infra"), row("⊘ docs-site")
	if top < 0 || head < 0 || next < 0 {
		t.Fatalf("rows missing: panel %d, head %d, next band %d", top, head, next)
	}
	if top <= head || top >= next {
		t.Errorf("the panel (row %d) should stand on its own band's trail rows, between %d and %d", top, head, next)
	}
}

func lipglossWidth(s string) int { return ansi.StringWidth(s) }
