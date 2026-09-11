package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// sweep marks a panel sweep: a pin that presses every key on every scene at
// five widths, often under both colour profiles, to hold a decision as a
// rule rather than on one frame. Thirty-two of them take three quarters of
// the package's four hundred seconds, so they step aside under -short and
// the default developer run stays under two minutes. CI and any run that
// wants the rules held runs without -short (with -timeout 40m).
func sweep(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("a panel sweep: run without -short to hold the rule")
	}
	if sweepColour() {
		return // the colour walk flips the one process-wide profile: sweeps take turns
	}
	// The sweeps run side by side (#325). Nothing they touch is shared: a
	// scene is built afresh by allScenes(), a model by sceneModel(), and
	// the one process-wide thing — lipgloss's colour profile — is Ascii
	// for every one of them, set here after the sequential tests have
	// finished and restored whatever they flipped.
	t.Parallel()
	forceASCII(t)
}

// sweepAlone marks the one sweep that cannot walk beside the others: it
// flips lipgloss's colour profile itself, and that profile is the single
// process-wide thing every frame in the package is drawn through. Under
// #325's `t.Parallel()` the flip landed in the middle of another sweep's
// walk, and a sweep that compares two frames byte for byte — the mirror
// key's, once, on 2026-09-10 — or reads a row above the footer then saw
// one frame styled and one bare, and failed on a difference neither pin
// was about. So this one steps aside under -short like the rest and then
// runs alone, in the sequential phase, where nothing else is drawing: a
// top-level test that never calls `t.Parallel()` finishes before any
// parallel test resumes. It sets both profiles itself and puts back the
// one it found, so it wants no `forceASCII` either (#325, #331).
func sweepAlone(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("a panel sweep: run without -short to hold the rule")
	}
}

// sweepColour says whether a sweep walks its frames a second time with
// colour on. Every panel report that measured both profiles found the two
// walks byte-identical once styling is stripped (#215, #218), and
// TestTheCorpusSaysTheSameWordsWithColourOn now holds that rule over the
// whole canonical corpus, so the sweeps walk once, under forceASCII, and
// the second pass is a switch: COMPASS_SWEEP_COLOUR=1 turns it back on.
func sweepColour() bool {
	return os.Getenv("COMPASS_SWEEP_COLOUR") != ""
}

// TestTheCorpusSaysTheSameWordsWithColourOn is the one colour pass the
// sweeps hand their second walk to: every canonical frame, on every scene
// at every width, drawn under termenv.Ascii and under termenv.TrueColor,
// says the same words once the styling is stripped. A width or a cut that
// depended on a style would show here as a frame that differs.
func TestTheCorpusSaysTheSameWordsWithColourOn(t *testing.T) {
	sweepAlone(t)
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	walk := func(sc scene, w, h int, prof termenv.Profile) []string {
		lipgloss.SetColorProfile(prof)
		// words strips the styling and the padding that the two profiles
		// lay differently at a line's end; the words that remain are what
		// the rule holds.
		words := func(frame string) string {
			lines := strings.Split(ansi.Strip(frame), "\n")
			for i, l := range lines {
				lines[i] = strings.TrimRight(l, " ")
			}
			return strings.Join(lines, "\n")
		}
		m := sceneModel(sc, w, h)
		poll(m, sc)
		frames := []string{words(m.View())}
		for _, k := range canonicalKeys {
			pressKey(m, k)
			poll(m, sc)
			frames = append(frames, words(m.View()))
		}
		return frames
	}
	frames := 0
	for _, sc := range allScenes() {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			plain := walk(sc, size[0], size[1], termenv.Ascii)
			colour := walk(sc, size[0], size[1], termenv.TrueColor)
			if len(plain) != len(colour) {
				t.Fatalf("%s %dx%d: %d frames under Ascii, %d under TrueColor", sc.name, size[0], size[1], len(plain), len(colour))
			}
			for i := range plain {
				frames++
				if plain[i] != colour[i] {
					t.Errorf("%s %dx%d frame %d (after %q): the words differ with colour on\n--- ascii ---\n%s\n--- colour ---\n%s",
						sc.name, size[0], size[1], i, keyBefore(i), plain[i], colour[i])
					break
				}
			}
		}
	}
	if frames < 1500 {
		t.Errorf("only %d frames compared — the parity walk went vacuous", frames)
	}
}

// keyBefore names the canonical key a frame index was drawn after.
func keyBefore(i int) string {
	if i == 0 {
		return "(opening)"
	}
	if i-1 < len(canonicalKeys) {
		return canonicalKeys[i-1]
	}
	return "?"
}
