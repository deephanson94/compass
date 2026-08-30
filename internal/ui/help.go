package ui

// helpKeys is the M1 keymap. Keys that arrive in later milestones are named
// here only when they already do something.
var helpKeys = [][2]string{
	{"1 – 9", "select a session"},
	{"j / k", "move down / up (↓ ↑ too)"},
	{"enter", "reveal its pane in your tmux"},
	{"g", "grab the session waiting longest, and reveal it"},
	{"tab", "zoom into a leg (arrives in M2)"},
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
		dimStyle.Render(clip("fleet:  ● working  ▲ needs you  ◍ stuck  ○ idle", w)),
		dimStyle.Render(clip("trail:  ◉ prompt  ◆ leg  ● now  ◈ subagent", w)),
	)
	return lines
}
