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
	for _, l := range helpLegendWrapped(58, true, 40, true, true) {
		if s := strings.TrimRight(ansi.Strip(l), " "); strings.HasSuffix(s, "·") {
			t.Errorf("a legend row ends on a hanging separator: %q", s)
		}
		if strings.Contains(ansi.Strip(l), "…") && strings.Contains(ansi.Strip(l), "board:") {
			t.Errorf("the board's gloss is clipped at the legend's width: %q", ansi.Strip(l))
		}
	}
	for _, w := range []int{17, 18, 20, 21} {
		if got := clip("Bash: go test ./... -count=3", w); strings.HasSuffix(got, "...…") || strings.HasSuffix(got, "./…") || strings.HasSuffix(got, "/…") {
			t.Errorf("clip(%d) marks a token ending in dots: %q", w, got)
		}
	}
	// A number whole on screen stays (#53): only a cut inside it backs out.
	if got := clip("Please run /login · API Error: 403 quota is exhausted", 35); !strings.Contains(got, "403") {
		t.Errorf("clip walked a whole status off the row: %q", got)
	}
	for _, w := range []int{8, 9} {
		if got := clip("took 4.03s to run the suite", w); strings.HasSuffix(got, ".…") {
			t.Errorf("the number guard handed back a trailing dot at %d: %q", w, got)
		}
	}
	// The dotted rule strips the dots, never the token: a wider column
	// never says strictly less (#61).
	if got := clip("Bash: python backfill.py --all", 22); !strings.Contains(got, "backfill") {
		t.Errorf("the dotted rule dropped the token: %q", got)
	}
	for _, w := range []int{23, 24, 28} {
		if got := clip("ok  github.com/example/cli  4.03s", w); !strings.Contains(got, "github.com/example") || strings.HasSuffix(got, "/…") || strings.HasSuffix(got, ".…") {
			t.Errorf("the dotted rule at %d dropped the path or marked a dot: %q", w, got)
		}
	}
	_ = state.Working
	_ = time.Second
}

// A lane row at the top of a pinned viewport stays: drawing its leg over
// it orphaned the finding beneath (#58).
func TestALaneRowAtTheFoldStays(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSubagents(), 80, 24)
	pressTab(m)
	tr := m.trail
	seen := false
	for h := 3; h < 60; h++ {
		o := m.trailOpts(43, h)
		doc, _ := trailDoc(tr, o)
		top := trailTop(len(doc), o)
		if top <= 0 || top >= len(doc) || !isDetailRow(doc[top]) || !strings.Contains(ansi.Strip(doc[top]), "same root cause") {
			continue
		}
		// The viewport opens on the finding's second row: the row drawn
		// over it must be the lane that brought it (#57, #58).
		seen = true
		rows := trailRows(tr, o)
		if first := ansi.Strip(rows[0]); !strings.Contains(first, "Score encoder") {
			t.Errorf("at height %d the finding's parent row is %q, not its lane", h, first)
		}
	}
	if !seen {
		t.Fatalf("no height opened the viewport on the finding's second row")
	}
}

// The tag beside a digest: the longest rung that costs the digest no
// clause; else the longest that draws the digest the shortest would;
// else the last (#60).
func TestTheTagBesideADigestTakesTheLongestHarmlessRung(t *testing.T) {
	ladder := []string{"opencode · sonnet-4-5 · ⌁ dev:2.0", "opencode · ⌁ dev:2.0", "opencode · sonnet-4-5", "opencode", "⌁ dev:2.0"}
	digest := func(room int) string {
		switch {
		case room >= 24:
			return "↪ sent \"go on\" · 0s ago"
		case room >= 14:
			return "↪ sent \"go on\""
		default:
			return ""
		}
	}
	if got := tagBesideDigest(ladder, 37, digest); got != "opencode" {
		t.Errorf("at 37 = %q, want the longest rung that keeps the digest whole", got)
	}
	if got := tagBesideDigest(ladder, 30, digest); got != "opencode" {
		t.Errorf("at 30 = %q, want the longest rung drawing what the shortest would", got)
	}
	// 12 cells: every rung leaves nothing, so the longest that draws
	// what the last would — the tool word, which tells rows apart.
	if got := tagBesideDigest(ladder, 12, digest); got != "opencode" {
		t.Errorf("at 12 = %q, want the longest rung drawing what the last would", got)
	}
	if got := tagBesideDigest([]string{"claude · ⌁ ops", "⌁ ops"}, 8, digest); got != "⌁ ops" {
		t.Errorf("at 8 = %q, want the last rung, the only one that fits", got)
	}
}
