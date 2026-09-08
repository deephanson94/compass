package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/deephanson94/compass/internal/journey"
)

// The pins of round thirty-seven (#62): each fold that a width change
// could revert with the suite green.

// The header's ladder has the rung between the full tag and none: the
// model goes before the tool word, and the archive header at 80 keeps
// "1 api · opencode · ⌁ dev:2.0" with a cell to spare (#62).
func TestTheHeaderShedsTheModelBeforeTheToolWord(t *testing.T) {
	m := sceneModel(sceneTwoTools(), 80, 24)
	press(m, "2")
	press(m, "x")
	press(m, "A")
	got := ansi.Strip(m.headerLine(78))
	if !strings.Contains(got, "1 api · opencode · ⌁ dev:2.0") {
		t.Errorf("the archive header at 80 = %q", got)
	}
	if wide := ansi.Strip(m.headerLine(118)); !strings.Contains(wide, "opencode · sonnet-4-5") {
		t.Errorf("the header at 120 = %q", wide)
	}
}

// The frame `m` is pressed on names `m`: its note keeps the key in the
// row, whichever side the toggle is on and whichever level it was pressed
// at (#62).
func TestTheMirrorNoteKeepsItsKey(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 120, 34)
	pressTab(m)
	press(m, "m")
	if foot := ansi.Strip(m.footerLine(118)); !strings.Contains(foot, "m conversation") || !strings.Contains(foot, "the live pane") {
		t.Errorf("m at Lv2: %q", foot)
	}
	press(m, "m")
	if foot := ansi.Strip(m.footerLine(118)); !strings.Contains(foot, "m live pane") || !strings.Contains(foot, "the conversation") {
		t.Errorf("m again: %q", foot)
	}
	pressTab(m)
	press(m, "m")
	if m.level != levelWaypoints {
		t.Fatalf("m at Lv3 should land on the trail, level %d", m.level)
	}
	if foot := ansi.Strip(m.footerLine(118)); !strings.Contains(foot, "m conversation") {
		t.Errorf("m from Lv3: %q", foot)
	}
}

// The live card's clock ends where every other row's age does, at Lv3 as
// at Lv2: the marker's cell is the clock's when the marker is gone (#62).
func TestTheCardsClockKeepsTheColumnAtLv3(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 120, 34)
	pressTab(m)
	lv2 := lipgloss.Width(m.sessionCard(52)[0])
	pressTab(m)
	lv3 := lipgloss.Width(m.sessionCard(52)[0])
	if lv2 != lv3 {
		t.Errorf("the card's first row is %d wide at Lv2 and %d at Lv3", lv2, lv3)
	}
}

// The overlay's mark stands at the cut, not where the row's text ran out:
// twelve cells left of the panel it read as a clipped prompt (#62).
func TestTheOverlaysMarkStandsAtTheCut(t *testing.T) {
	rows := []string{`◉ "rewrite the install page"` + strings.Repeat(" ", 30) + "55m ago"}
	overlay(rows, []string{"┌──┐"}, 43, 0)
	plain := ansi.Strip(rows[0])
	if i := lipgloss.Width(plain[:strings.Index(plain, "…")]); i != 42 {
		t.Errorf("the mark is at cell %d, want 42:\n%s", i, plain)
	}
}

// The refusal one press after a hide names the pane the hide note named:
// two notes about one session say it the same way (#62).
func TestTheRefusalNamesThePaneTheHideNoteNamed(t *testing.T) {
	m := sceneModel(sceneTwoTools(), 152, 40)
	press(m, "2")
	press(m, "x")
	hide := m.note
	press(m, "2")
	if !strings.HasSuffix(hide, "⌁ dev:2.0") || !strings.HasSuffix(m.note, "⌁ dev:2.0") {
		t.Errorf("hide %q, refusal %q", hide, m.note)
	}
}

// Below the board's width the live reader takes the whole screen and no
// band names the archive: its footer does, and not where the band is on
// screen (#62).
func TestTheLiveReaderNamesTheArchiveDoorBelowTheBoard(t *testing.T) {
	forceASCII(t)
	for _, w := range []int{80, 100} {
		m := sceneModel(sceneSecondDay(), w, 30)
		pressTab(m)
		pressTab(m)
		m.View() // the footer is composed against the rows the frame drew
		if foot := ansi.Strip(m.footerLine(w - 2)); !strings.Contains(foot, "A archive") {
			t.Errorf("at %d the live reader's footer = %q", w, foot)
		}
	}
	m := sceneModel(sceneSecondDay(), 120, 34)
	pressTab(m)
	pressTab(m)
	m.View()
	if foot := ansi.Strip(m.footerLine(118)); strings.Contains(foot, "A archive") {
		t.Errorf("at 120 the band names the archive; the footer = %q", foot)
	}
}

// A tick row's label takes every cell the verb and the span leave: the
// reserve for a `?` no tick row can carry is gone (#62).
func TestATickRowsLabelTakesItsWholeField(t *testing.T) {
	l := journey.Leg{Class: journey.Design, Label: "where the audit log lives", Start: sceneNow.Add(-8 * time.Minute), End: sceneNow.Add(-8 * time.Minute)}
	row := ansi.Strip(tickRow(l, "│", 37))
	if !strings.Contains(row, "where the audit log lives") || lipgloss.Width(row) != 37 {
		t.Errorf("the tick row at 37 = %q (%d wide)", row, lipgloss.Width(row))
	}
}

// `↪` is a fleet row's mark, and the 120x34 help glosses it on the fleet's
// own row rather than on a trail row the height cuts (#62).
func TestTheWideHelpGlossesTheSentMark(t *testing.T) {
	m := sceneModel(sceneTwoTools(), 120, 34)
	press(m, "?")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "↪ sent — a line compass typed") {
		t.Errorf("the 120x34 help never says what ↪ means:\n%s", view)
	}
}
