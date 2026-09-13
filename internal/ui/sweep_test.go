package ui

import (
	"fmt"
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
//
// The other thirty-one sweeps flip the profile too, and #339 found the
// flip still moving a neighbour's frame two days later — not because
// they were wrong to flip it, but because two things on the frame's own
// path spelled a row differently under the two profiles. That is fixed
// where it lived: a frame's cells no longer turn on the terminal's
// colours, so a pin that strips the styling cannot be moved by a profile
// it never set. This one still walks alone, because it is the pin that
// compares the two profiles and a third hand on the switch would be
// comparing neither.
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
//
// #339 made "the same words" the same cells. The walk used to trim each
// line's trailing blanks before comparing, because two things on the
// frame's own path asked what a row's *bytes* ended with rather than what
// its cells ended with — the joiner's `TrimRight(row, " ")` and the
// margin's `line == ""` — and a styled row ends on its reset, so the
// blanks a cursor bar or a title laid came off a bare frame and stayed on
// a coloured one. Thirty-one sweeps flip this one process-wide profile
// while their neighbours draw (#325, #331), and a flip that landed between
// two drawings of one model moved the row: 1062 rows of this corpus were
// two rows, and #327's pin, which reads the frame above the footer either
// side of a refused digit, failed on one of them about once in a thousand
// runs. The trim is gone: the two walks are compared byte for byte, and no
// pin that strips the styling can be moved by a profile it never set.
func TestTheCorpusSaysTheSameWordsWithColourOn(t *testing.T) {
	sweepAlone(t)
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	walk := func(sc scene, w, h int, prof termenv.Profile) []string {
		lipgloss.SetColorProfile(prof)
		// The frame with the styling stripped and nothing else taken off
		// it: the cells that remain are what the rule holds.
		m := sceneModel(sc, w, h)
		poll(m, sc)
		frames := []string{ansi.Strip(m.View())}
		for _, k := range canonicalKeys {
			pressKey(m, k)
			poll(m, sc)
			frames = append(frames, ansi.Strip(m.View()))
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
					t.Errorf("%s %dx%d frame %d (after %q): the cells differ with colour on%s",
						sc.name, size[0], size[1], i, keyBefore(i),
						frameMoved(strings.Split(plain[i], "\n"), strings.Split(colour[i], "\n")))
					break
				}
			}
		}
	}
	if frames < 1500 {
		t.Errorf("only %d frames compared — the parity walk went vacuous", frames)
	}
}

// TestAFrameIsTheSameFrameWhenTheProfileMovesUnderIt pins #339 in the shape
// the flake had. Dozens of pins draw one model twice — a key that must move
// nothing, a row read either side of a keypress, a refusal that may only
// change the footer — and compare the two frames byte for byte with the
// styling stripped. Thirty-one panel sweeps flip lipgloss's one
// process-wide colour profile while those pins draw (#325, #331), so the
// second drawing is not always under the profile the first was: a pin that
// never touched the profile then failed on a difference it was not about,
// once in a thousand runs, on whichever row the two profiles spelled
// differently. Nothing on the frame's path may ask what a row's bytes are
// when the question is what its cells are, and this is that rule said as a
// keypress: one model, two drawings, the profile moved between them, and
// the same frame both times.
//
// Two widths and five depths rather than a sweep's five widths and every
// key: it costs two seconds and flips the profile itself, so it runs in
// the sequential phase where nothing else is drawing (#331) and under
// -short as well, where the pin it guards is skipped.
func TestAFrameIsTheSameFrameWhenTheProfileMovesUnderIt(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	drawn := 0
	for _, sc := range allScenes() {
		for _, size := range [][2]int{{80, 24}, {220, 48}} {
			m := sceneModel(sc, size[0], size[1])
			poll(m, sc)
			// The three depths every deck has, and the archive beside
			// them: the flake was read off a session-view frame, and the
			// reader is the panel whose cursor bar laid the cells.
			for _, k := range []string{"", "tab", "tab", "A", "esc"} {
				if k != "" {
					pressKey(m, k)
					poll(m, sc)
				}
				lipgloss.SetColorProfile(termenv.Ascii)
				bare := strings.Split(ansi.Strip(m.View()), "\n")
				lipgloss.SetColorProfile(termenv.TrueColor)
				lit := strings.Split(ansi.Strip(m.View()), "\n")
				lipgloss.SetColorProfile(termenv.Ascii)
				again := strings.Split(ansi.Strip(m.View()), "\n")
				drawn++
				where := sc.name + " " + itoaPin(size[0]) + "x" + itoaPin(size[1]) + " Lv" + itoaPin(m.level)
				if k != "" {
					where += " after " + k
				}
				if strings.Join(bare, "\n") != strings.Join(lit, "\n") {
					t.Errorf("%s: the profile moved and the frame moved with it%s", where, frameMoved(bare, lit))
				}
				if strings.Join(bare, "\n") != strings.Join(again, "\n") {
					t.Errorf("%s: the frame did not come back when the profile did%s", where, frameMoved(bare, again))
				}
			}
		}
	}
	if drawn < 80 {
		t.Errorf("only %d frames drawn twice — the rule is unmeasured", drawn)
	}
}

// frameMoved names where two drawings of one frame part company: the
// first row that differs, said twice, with the cells counted. A bare "the
// frame moved" left the reader of a failing run — and #339's one-in-a-
// thousand was read off a CI log, not a screen — nothing to go on: the
// rows that move are often the ones whose difference is invisible, twenty
// blank cells at a row's end, and only their lengths tell them apart.
func frameMoved(before, after []string) string {
	for i := 0; i < len(before) || i < len(after); i++ {
		var was, now string
		if i < len(before) {
			was = before[i]
		}
		if i < len(after) {
			now = after[i]
		}
		if was == now {
			continue
		}
		return fmt.Sprintf("\n  row %d\n  -%q (%d cells)\n  +%q (%d cells)",
			i, was, len([]rune(was)), now, len([]rune(now)))
	}
	return "\n  (no row differs: the frames are of different heights)"
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
