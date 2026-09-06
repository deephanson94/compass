package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/state"
)

// The pins of rounds thirty-three and thirty-four (#56, #57, #58): each
// fold that a width change could revert with the suite green.

// The card's sentence is built at the width it is drawn at, so a budgeted
// HEAD keeps "for 4m of 10m" across the card widths that once cut it (#56).
func TestTheCardsSentenceKeepsTheBudgetAtEveryWidth(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneAlarmStorm(), 152, 40)
	press(m, "7")
	pressTab(m)
	for _, w := range []int{50, 54, 58, 62, 70} {
		if card := ansi.Strip(m.cardSecond(w)); !strings.Contains(card, "for 4m of 10m") {
			t.Errorf("at %d the card's sentence lost its budget: %q", w, card)
		}
	}
}

// On a two-tool fleet the ladder never drops the tool word for the bare
// pane: at fourteen cells it is the word, not the pane (#56).
func TestTheLadderKeepsTheEarnedWord(t *testing.T) {
	m := sceneModel(sceneTwoTools(), 152, 40)
	var api fleet.Session
	for _, s := range m.sessions {
		if s.Info.ID == "api-oc" {
			api = s
		}
	}
	if got := m.tagFor(api, 14, 12, ""); got != "opencode" {
		t.Errorf("tagFor at 14 = %q, want the tool word", got)
	}
	ladder := m.tagLadder(api)
	if ladder[len(ladder)-1] != "⌁ dev:2.0" {
		t.Errorf("the bare pane is not the last rung: %q", ladder)
	}
}

// The archive's trail title never sheds its own name for the day (#56).
func TestTheArchiveTitleKeepsItsRow(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 120, 34)
	pressTab(m)
	press(m, "2")
	poll(m, sceneSecondDay())
	pressTab(m)
	if title := ansi.Strip(m.trailTitle(36)); !strings.Contains(title, "fix the 401") || strings.Contains(title, "TRAIL · api") {
		t.Errorf("the archive's title at 36 = %q", title)
	}
}

// The band decides its verdict form once: no row keeps its counts while a
// row beside it drops them, and no prompt is bought out below its floor (#57).
func TestTheBandsVerdictFormIsOne(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 100, 30)
	lines := m.recentLines(41, 12)
	counts, plain := 0, 0
	for _, l := range lines[1:] {
		s := ansi.Strip(l)
		switch {
		case strings.Contains(s, "✓") && strings.Contains(s, "✓ green") && strings.Count(s, "✓") > 1, strings.Contains(s, "✗ red ") && strings.Contains(s, "✓"):
			counts++
		case strings.Contains(s, "✗ red") || strings.Contains(s, "✓ green") || strings.Contains(s, "✓ shipped"):
			plain++
		}
	}
	if counts > 0 && plain > 0 {
		t.Errorf("the band mixes verdict forms:\n%s", ansi.Strip(strings.Join(lines, "\n")))
	}
	if strings.Contains(ansi.Strip(strings.Join(lines, "\n")), `"rewrite…`) {
		t.Errorf("a prompt was bought out below its floor:\n%s", ansi.Strip(strings.Join(lines, "\n")))
	}
}

// A finding's parent at the fold is the lane that brought it (#57).
func TestAFindingAtTheFoldWearsItsLane(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSubagents(), 80, 24)
	pressTab(m) // the legs, cursor on the present
	tr := m.trail
	// A viewport that opens on the returned lane's finding rows.
	for h := 6; h <= 20; h++ {
		rows := trailRows(tr, m.trailOpts(43, h))
		if len(rows) == 0 {
			continue
		}
		first := ansi.Strip(rows[0])
		if strings.Contains(first, "◆ scout") && len(rows) > 1 && strings.Contains(ansi.Strip(rows[1]), "3 defects found") {
			t.Errorf("at height %d the finding stands under the leg, not its lane:\n%s", h, ansi.Strip(strings.Join(rows[:3], "\n")))
		}
	}
}

// A digit whose session is hidden names it and the way to it (#57).
func TestADigitOnAHiddenSessionNamesIt(t *testing.T) {
	m := sceneModel(sceneTwoTools(), 120, 34)
	press(m, "2")
	press(m, "x")
	press(m, "2")
	if !strings.Contains(m.note, "2 api is hidden · A, then x") {
		t.Errorf("the refusal = %q", m.note)
	}
}

// Below the board's width the refused m row is cut before g's (#57), and
// the legend's rows never end on a hanging separator; a clip never marks
// a token that ends in a dot (#58).
func TestTheNarrowHelpKeepsGAndTheLegendBindsItsSeparators(t *testing.T) {
	m := sceneModel(sceneAlarmStorm(), 80, 24)
	press(m, "?")
	help := ansi.Strip(m.View())
	if !strings.Contains(help, "\n g ") {
		t.Errorf("the 80x24 help has no g row:\n%s", help)
	}
	if strings.Contains(help, "needs 110 columns") {
		t.Errorf("the 80x24 help spends a row on the refused m over g:\n%s", help)
	}
	for _, l := range helpLegendWrapped(58, true, 40) {
		if s := strings.TrimRight(ansi.Strip(l), " "); strings.HasSuffix(s, "·") {
			t.Errorf("a legend row ends on a hanging separator: %q", s)
		}
		if strings.Contains(ansi.Strip(l), "…") && strings.Contains(ansi.Strip(l), "board:") {
			t.Errorf("the board's gloss is clipped at the legend's width: %q", ansi.Strip(l))
		}
	}
	if got := clip("Bash: go test ./... -count=3", 20); strings.HasSuffix(got, "...…") || strings.HasSuffix(got, "./…") {
		t.Errorf("clip marks a token ending in dots: %q", got)
	}
	_ = state.Working
	_ = time.Second
}
