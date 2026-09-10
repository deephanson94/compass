package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Below the deck's width the lane's page owns the screen and no trail
// panel is on the frame to say the call the agent is inside — the row the
// person pressed Tab on said it, and the page said less than the row above
// it. Said once: where the trail panel is drawn, it stays the trail's.
func TestTheLanePageAloneSaysTheCallItIsInside(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	for _, w := range []int{80, 100, 120, 152, 220} {
		m := sceneModel(sc, w, 34)
		for _, k := range []string{"tab", "G", "k", "k", "tab"} {
			pressKey(m, k)
			poll(m, sc)
		}
		view := ansi.Strip(m.View())
		if m.readerLane == "" || !strings.Contains(view, "Measure moe_by_andy") {
			t.Fatalf("at %d the route does not open the measurer's page:\n%s", w, view)
		}
		page, trail := false, false
		for _, line := range strings.Split(view, "\n") {
			if !strings.Contains(line, "python dla.py") {
				continue
			}
			if strings.Contains(line, "nothing to read yet") || strings.HasPrefix(strings.TrimSpace(line), "\u25cf ") || strings.HasPrefix(strings.TrimSpace(line), "* ") {
				page = true
			}
			if strings.Contains(line, "wrote 40s ago") {
				trail = true
			}
		}
		alone := w < 120
		if page != alone {
			t.Errorf("at %d the lane page says the call in flight = %v, want %v:\n%s", w, page, alone, view)
		}
		if trail == alone {
			t.Errorf("at %d the trail sub-row says it = %v (the page should carry it only when the trail is not drawn)", w, trail)
		}
	}
}
