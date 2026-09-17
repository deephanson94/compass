package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The board spends its spare rows on the lanes' own heads: where the column
// has the rows, an open lane says what it is on and how long it has been
// quiet, as it does from Lv2 down — and where it has not, the glyph stands
// alone and the trail keeps its first prompt (#49's floor).
func TestTheBoardSpendsItsSpareRowsOnTheLanesHeads(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	for _, w := range []int{152, 220} {
		m := sceneModel(sc, w, 48)
		view := ansi.Strip(m.View())
		blank := 0
		for _, line := range strings.Split(view, "\n") {
			if strings.TrimSpace(line) == "" {
				blank++
			}
		}
		if !strings.Contains(view, "silent 12m") {
			t.Errorf("at %d the board draws →1 on a lane whose own silence it never says, over %d blank rows:\n%s", w, blank, view)
		}
	}
	// At eighty the rows are not there: the heads go, not the trail's
	// head. The block over the trail takes the first prompt's row at 24
	// (#377); at 30 the prompt is back.
	m := sceneModel(sc, 80, 24)
	view := ansi.Strip(m.View())
	if strings.Contains(view, "silent 12m") {
		t.Errorf("at 80 the heads should go first:\n%s", view)
	}
	tall := sceneModel(sc, 80, 27) // the block's two rows and its seam over the 24 the rule was measured at
	if v := ansi.Strip(tall.View()); !strings.Contains(v, "◉ 1/2") || strings.Contains(v, "silent 12m") {
		t.Errorf("at 80x27 the heads cost the trail its first prompt:\n%s", v)
	}
}

// A clip never ends on a flag's leading dash: the hyphen inside a token
// stays, the bare one before the mark goes (#72).
func TestAClipNeverEndsOnAFlagsDash(t *testing.T) {
	if got := clip("python dla.py --model moe_by_andy --split dx6", 36); got != "python dla.py --model moe_by_andy…" {
		t.Errorf("clip at 36 = %q", got)
	}
	if got := clip("go test ./... -run TestX -count=1", 26); got != "go test ./... -run TestX…" {
		t.Errorf("clip at 26 = %q", got)
	}
}
