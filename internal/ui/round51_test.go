package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A lane's page with no turns offers the attach key and the way out, and
// nothing else (#56). Under a note as long as the chapter keys' refusal
// the row must still name the way out: the way out is the last key to go
// (#24, #60), whichever key happens to lead the row.
func TestTheLanePageKeepsTheWayOutUnderAChapterNote(t *testing.T) {
	forceASCII(t)
	sc := sceneSubagents()
	for _, w := range []int{80, 100, 120, 152, 220} {
		m := sceneModel(sc, w, 34)
		for _, k := range []string{"tab", "G", "k", "k", "tab", "]"} {
			pressKey(m, k)
			poll(m, sc)
		}
		view := ansi.Strip(m.View())
		lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
		foot := lines[len(lines)-1]
		if !strings.Contains(foot, "no turns of yours") {
			t.Fatalf("at %d the route does not refuse the chapter key:\n%s", w, view)
		}
		if !strings.Contains(foot, "esc back") {
			t.Errorf("at %d the footer sheds the way out:\n%s", w, foot)
		}
	}
}
