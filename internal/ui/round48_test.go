package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A late result is named by its own call, whoever answered first: the lead's
// second agent came back after another agent's report and a file read, and
// its finding was drawn as a second output line of that read.
func TestALateResultIsNamedByItsOwnCall(t *testing.T) {
	sc := sceneSubagents()
	for _, wh := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sc, wh[0], wh[1])
		for _, k := range []string{"3", "tab", "j"} {
			pressKey(m, k)
			poll(m, sc)
		}
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		for i, l := range lines {
			if !strings.Contains(l, "The SDK renamed teammate") || !strings.Contains(l, "⎿") {
				continue
			}
			above := ""
			if i > 0 {
				above = lines[i-1]
			}
			if !strings.Contains(above, "Agent(Compare the contract") {
				t.Errorf("at %d the Compare agent's finding stands under %q, not under its own call:\n%s",
					wh[0], strings.TrimSpace(above), strings.Join(lines, "\n"))
			}
		}
	}
}

// The overlay's peek keeps a remainder that reads — a number — and blanks
// a lone word with no digit (#55, #73).
func TestTheOverlaysPeekKeepsOnlyARemainderThatReads(t *testing.T) {
	keep := []string{strings.Repeat("x", 20) + strings.Repeat(" ", 20) + "for 33m"}
	overlay(keep, []string{"┌──┐"}, 10, 0)
	if !strings.Contains(ansi.Strip(keep[0]), "…") || !strings.Contains(ansi.Strip(keep[0]), "33m") {
		t.Errorf("a number remainder should peek: %q", keep[0])
	}
	drop := []string{strings.Repeat("x", 20) + strings.Repeat(" ", 20) + "2h ago"}
	overlay(drop, []string{strings.Repeat("─", 34)}, 10, 0) // the panel ends inside "2h", leaving "ago"
	if plain := ansi.Strip(drop[0]); strings.Contains(plain, "ago") {
		t.Errorf("a lone word with no digit should not peek: %q", plain)
	}
}
