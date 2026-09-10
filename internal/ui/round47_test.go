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
	// At eighty the rows are not there: the heads go, not the trail's head.
	m := sceneModel(sc, 80, 24)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "◉ 1/2") || strings.Contains(view, "silent 12m") {
		t.Errorf("at 80 the heads cost the trail its first prompt:\n%s", view)
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
