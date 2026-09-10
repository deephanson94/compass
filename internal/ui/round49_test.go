package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// An agent whose file holds no turn compass can draw is not an agent that
// has written nothing: the measurer's own page said "written nothing yet"
// while its head two panels left said it wrote forty seconds ago (#74).
func TestALanePageNeverDeniesTheWriteItsHeadReports(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	for _, w := range []int{80, 100, 120, 152, 220} {
		m := sceneModel(sc, w, 34)
		for _, k := range []string{"tab", "G", "k", "k", "tab"} {
			pressKey(m, k)
			poll(m, sc)
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "Measure moe_by_andy") || m.readerLane == "" {
			t.Fatalf("at %d the route does not open the measurer's page:\n%s", w, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if strings.Contains(line, "the agent has written nothing") {
				t.Errorf("at %d the measurer's page denies its own write (head: wrote 40s ago): %q", w, strings.TrimSpace(line))
			}
		}
	}
}
