package ui

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The greys are the deck's most-used colour: the work line, the leg counts,
// the ages and the rails are all drawn in them. compass used to take them from
// the terminal's own palette index 8, which several popular themes set within
// a hair of their own background — Solarized Dark puts it at 1.15:1 — and the
// quiet half of the board disappeared. These tests pin what replaced it.

// relLum is the WCAG 2.x relative luminance of an #rrggbb colour.
func relLum(hex string) float64 {
	h := strings.TrimPrefix(hex, "#")
	if len(h) != 6 {
		panic("contrast test wants #rrggbb, got " + hex)
	}
	chan_ := func(i int) float64 {
		v, err := strconv.ParseUint(h[i:i+2], 16, 8)
		if err != nil {
			panic(err)
		}
		c := float64(v) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*chan_(0) + 0.7152*chan_(2) + 0.0722*chan_(4)
}

// contrast is the WCAG 2.x contrast ratio between two #rrggbb colours.
func contrast(a, b string) float64 {
	la, lb := relLum(a), relLum(b)
	hi, lo := math.Max(la, lb), math.Min(la, lb)
	return (hi + 0.05) / (lo + 0.05)
}

// darkThemes are the backgrounds compass is actually read on. Nord is the
// tightest of them, which is why the floor below is set by it.
var darkThemes = map[string]string{
	"solarized-dark": "#002b36",
	"nord":           "#2e3440",
	"github-dark":    "#0d1117",
	"gruvbox-dark":   "#282828",
	"vscode-dark":    "#1e1e1e",
	"dracula":        "#282a36",
	"black":          "#000000",
}

var lightThemes = map[string]string{
	"white":           "#ffffff",
	"solarized-light": "#fdf6e3",
	"github-light":    "#f6f8fa",
}

// Dim text is text, so it is held to the WCAG AA floor for text, 4.5:1 — with
// a hair of slack for Nord, the darkest-backgrounded of the popular themes,
// where anything that clears 4.5 everywhere would stop reading as dim.
func TestDimTextClearsTheContrastFloorOnEveryCommonTheme(t *testing.T) {
	const floor = 4.4
	for name, bg := range darkThemes {
		if got := contrast(colDim.Dark, bg); got < floor {
			t.Errorf("dim %s on %s (%s) is %.2f:1, under the %.1f:1 floor", colDim.Dark, name, bg, got, floor)
		}
	}
	for name, bg := range lightThemes {
		if got := contrast(colDim.Light, bg); got < floor {
			t.Errorf("dim %s on %s (%s) is %.2f:1, under the %.1f:1 floor", colDim.Light, name, bg, got, floor)
		}
	}
}

// The rails are chrome, not text, so they answer to the 3:1 WCAG asks of a
// non-text UI element rather than the text floor — but they do answer to it.
// The old ruleStyle (colour 8 plus Faint) cleared nothing at all.
func TestTheRailsClearTheNonTextContrastFloor(t *testing.T) {
	const floor = 2.7 // 3:1, less the slack Nord's #2e3440 needs
	for name, bg := range darkThemes {
		if got := contrast(colRule.Dark, bg); got < floor {
			t.Errorf("rule %s on %s (%s) is %.2f:1, under the %.1f:1 floor", colRule.Dark, name, bg, got, floor)
		}
	}
	for name, bg := range lightThemes {
		if got := contrast(colRule.Light, bg); got < floor {
			t.Errorf("rule %s on %s (%s) is %.2f:1, under the %.1f:1 floor", colRule.Light, name, bg, got, floor)
		}
	}
}

// The greys stay on the grey axis. A tinted grey is not a style preference
// here: on a 16-colour terminal termenv resolves it to the nearest palette
// entry, and #6b7280 — a perfectly reasonable-looking slate — lands on bright
// blue. r=g=b is what keeps the degradation honest.
func TestTheGreysAreNeutral(t *testing.T) {
	for _, c := range []struct {
		name string
		hex  string
	}{
		{"colDim.Dark", colDim.Dark}, {"colDim.Light", colDim.Light},
		{"colRule.Dark", colRule.Dark}, {"colRule.Light", colRule.Light},
	} {
		h := strings.TrimPrefix(c.hex, "#")
		if h[0:2] != h[2:4] || h[2:4] != h[4:6] {
			t.Errorf("%s = %s is not on the grey axis; it will drift when termenv degrades it", c.name, c.hex)
		}
	}
}

// On a 16-colour terminal the greys must land back on colour 8, which is what
// compass drew before and the best a 16-colour palette offers. The fix is for
// the 256-colour and truecolour terminals where the palette index was a
// lottery; it must not disturb the terminals where it was the only option.
func TestOnASixteenColourTerminalTheGreysDegradeToColour8(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, c := range []struct {
		name string
		hex  string
	}{{"colDim.Dark", colDim.Dark}, {"colRule.Dark", colRule.Dark}} {
		got := lipgloss.NewStyle().Foreground(lipgloss.Color(c.hex)).Render("x")
		if !strings.Contains(got, "\x1b[90m") {
			t.Errorf("%s (%s) degrades to %q, not colour 8", c.name, c.hex, got)
		}
	}
}

// Faint is never stacked on a grey again. Terminals that implement SGR 2 as a
// blend toward the background were blending a near-background grey into the
// background, and the hairlines simply were not there.
func TestTheRailsDoNotStackFaintOnTheGrey(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	if got := rule(4); strings.Contains(got, "\x1b[2;") || strings.Contains(got, ";2m") {
		t.Errorf("the hairline still carries SGR 2 (faint): %q", got)
	}
}

// A grey the default still does not suit is a config line away, and a typo in
// it leaves the deck readable rather than blanking it.
func TestSetDimTakesAColourAndIgnoresNonsense(t *testing.T) {
	dim, ruleCol, ds, rs := colDim, colRule, dimStyle, ruleStyle
	t.Cleanup(func() { colDim, colRule, dimStyle, ruleStyle = dim, ruleCol, ds, rs })

	for _, good := range []string{"#c0c0c0", "#ccc", "245", "8", "0"} {
		colDim = lipgloss.AdaptiveColor{Light: "#000000", Dark: "#000000"}
		SetDim(good)
		if colDim.Dark != good {
			t.Errorf("SetDim(%q) did not take: colDim.Dark = %q", good, colDim.Dark)
		}
		if colRule.Dark != good {
			t.Errorf("SetDim(%q) left the rails behind at %q", good, colRule.Dark)
		}
	}
	for _, bad := range []string{"", "  ", "grey", "#12345", "#gggggg", "256", "1000", "#", "-1"} {
		colDim = lipgloss.AdaptiveColor{Light: "#aaaaaa", Dark: "#aaaaaa"}
		SetDim(bad)
		if colDim.Dark != "#aaaaaa" {
			t.Errorf("SetDim(%q) was honoured; it should have been ignored (colDim.Dark = %q)", bad, colDim.Dark)
		}
	}
}

// Whatever the grey is, the deck still has to read with the colour switched
// off — the contract the goldens are drawn under (SPEC §4).
func TestTheDimFixDoesNotMoveAnythingOnAMonochromeTerminal(t *testing.T) {
	forceASCII(t)
	if got := rule(4); got != "────" {
		t.Errorf("the hairline carries style on an ASCII profile: %q", got)
	}
	if got := dimStyle.Render("15m"); got != "15m" {
		t.Errorf("dim text carries style on an ASCII profile: %q", got)
	}
}
