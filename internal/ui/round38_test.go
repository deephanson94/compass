package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

// The pins of round thirty-eight (#63): each fold that a width change
// could revert with the suite green.

// The card's first row is the column's width at every level: the cell of
// air belongs to the marker, and with the marker gone the clock takes it
// without pushing the rail (#63).
func TestTheCardsRowIsTheColumnsWidthAtEveryLevel(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 120, 34)
	pressTab(m)
	for lv := 2; lv <= 3; lv++ {
		for _, w := range []int{52, 54, 60} {
			if got := lipgloss.Width(m.sessionCard(w)[0]); got != w {
				t.Errorf("at Lv%d the card's first row is %d wide for a %d column", lv, got, w)
			}
		}
		pressTab(m)
	}
	// And the frame: no row of the Lv3 deck reaches the terminal's last cell.
	for i, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		if lipgloss.Width(l) > 119 {
			t.Errorf("row %d is %d wide on a 120 deck:\n%s", i, lipgloss.Width(l), l)
		}
	}
}

// The board's last band takes an empty strip's row and its air: the 120
// subagents board draws its fourth column rather than naming it in a strip
// over six blank rows (#62, #63).
func TestTheLastBandTakesTheEmptyStripsRow(t *testing.T) {
	m := sceneModel(sceneSubagents(), 120, 34)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "4 ○ cli") || strings.Contains(view, "+1 more") {
		t.Errorf("the 120 board should draw the cli column, not a strip:\n%s", view)
	}
}

// HEAD's label outranks its sentence's second half whenever the label is
// being cut: the second half is on the card and the digest, the name is
// nowhere else (#51, #63).
func TestTheHeadLabelOutranksTheSecondClause(t *testing.T) {
	l := journey.Leg{Class: journey.Build, Label: "implement s6e encoder tests", Current: true}
	row := ansi.Strip(legRow(l, l.Label, false, TrailOpts{Width: 49, HeadTail: "◈3 out 20m · 2 silent 18m", HeadState: state.Working, Now: sceneNow}))
	if !strings.Contains(row, "implement s6e encoder tests") || strings.Contains(row, "silent") {
		t.Errorf("at 49 the row = %q", row)
	}
	wide := ansi.Strip(legRow(l, l.Label, false, TrailOpts{Width: 70, HeadTail: "◈3 out 20m · 2 silent 18m", HeadState: state.Working, Now: sceneNow}))
	if !strings.Contains(wide, "2 silent 18m") {
		t.Errorf("with the room the whole sentence stays: %q", wide)
	}
}

// A spaced dash is a separator: the mark never follows one, and a hyphen
// inside a token stays (#63).
func TestAClipNeverEndsOnADash(t *testing.T) {
	if got := clip("/kickoff porter_tui — the delegated port", 22); got != "/kickoff porter_tui…" {
		t.Errorf("clip at 22 = %q", got)
	}
	if got := clip("go test ./... -run TestX", 15); got != "go test…" {
		t.Errorf("clip at 15 = %q", got)
	}
}

// The refusal one press after a hide keeps the pane the hide note kept
// when the keys leave room for it, whether or not the attach hint was
// there to give up (#63).
func TestTheRefusalKeepsThePaneWhereTheKeysLeaveRoom(t *testing.T) {
	sc := sceneTwoTools()
	m := sceneModel(sc, 152, 40)
	for _, k := range append(append([]string(nil), canonicalKeys...), "esc", "2") {
		pressKey(m, k)
		poll(m, sc)
	}
	if foot := ansi.Strip(m.footerLine(150)); !strings.HasSuffix(foot, "⌁ dev:2.0") {
		t.Errorf("the refusal's footer at 152 = %q", foot)
	}
}

// The reader's word stands on the reader in the archive as in the live
// view, wherever the trail's title gave it up (#63).
func TestTheArchiveReaderWearsItsWord(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 120, 34)
	pressTab(m)
	press(m, "2")
	pressTab(m)
	pressTab(m)
	if m.level != levelReader || !m.archiveView {
		t.Fatalf("expected the archive's reader, level %d archive %v", m.level, m.archiveView)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "[reader]") {
		t.Errorf("the archive's Lv3 wears no word:\n%s", view)
	}
}

// The empty lane's reader offers no session key: the lane is never a
// session, and the key led the row once the reading keys were gone (#63).
func TestTheEmptyLaneOffersNoSessionKey(t *testing.T) {
	sc := sceneSubagents()
	m := sceneModel(sc, 120, 34)
	seen := false
	for _, k := range append(append(append([]string(nil), canonicalKeys...), "esc"), sc.extra...) {
		pressKey(m, k)
		poll(m, sc)
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "written nothing") {
			continue
		}
		seen = true
		if strings.Contains(view, "h/l session") {
			t.Errorf("after %q the empty lane's footer offers h/l session:\n%s", k, view)
		}
	}
	if !seen {
		t.Fatal("the walkthrough never opens the empty lane")
	}
}

// The help at 100 glosses `↪` on the fleet's own row (#62, #63).
func TestTheNarrowerHelpGlossesTheSentMark(t *testing.T) {
	m := sceneModel(sceneTwoTools(), 100, 30)
	press(m, "?")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "↪ sent — a line compass typed") {
		t.Errorf("the 100x30 help never says what ↪ means:\n%s", view)
	}
}

// The overlay paints nothing past the panel (#63).
func TestTheOverlayPaintsNothingPastThePanel(t *testing.T) {
	rows := []string{"abc", strings.Repeat("x", 30)}
	overlay(rows, []string{"┌─┐", "└─┘"}, 5, 0)
	for i, r := range rows {
		if strings.HasSuffix(r, " ") {
			t.Errorf("row %d ends in paint: %q", i, r)
		}
	}
}
