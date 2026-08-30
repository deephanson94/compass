package ui

// helpKeys is the M0 keymap. Keys that arrive in later milestones are named
// here only when they already do something.
var helpKeys = [][2]string{
	{"1 – 9", "select a session"},
	{"j / k", "move down / up (↓ ↑ too)"},
	{"g", "grab the session waiting longest"},
	{"tab", "zoom into the trail"},
	{"?", "this help"},
	{"q", "quit"},
}

// helpLines renders the help overlay in place of the deck body: same margins,
// same alignment, nothing new to learn.
func helpLines(w, h int) []string {
	lines := []string{textStyle.Render("keys"), ""}
	for _, k := range helpKeys {
		lines = append(lines, dimStyle.Render(pad(k[0], 10))+textStyle.Render(clip(k[1], w-10)))
	}
	lines = append(lines,
		"",
		rule(min(w, 40)),
		dimStyle.Render(clip("compass observes; it never types for you.", w)),
		dimStyle.Render(clip("glyphs: ● working  ▲ needs you  ◍ stuck  ○ idle", w)),
	)
	return lines
}
