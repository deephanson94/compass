package ui

// helpKeys is the M2 keymap. Keys that arrive in later milestones are named
// here only when they already do something.
var helpKeys = [][2]string{
	{"1 – 9", "select a session"},
	{"j / k", "move down / up (↓ ↑ too)"},
	{"enter", "attach to its pane (outside tmux, prefix d returns)"},
	{"g", "grab the session waiting longest, and attach"},
	{"A", "browse the archive — every past session, by project"},
	{"tab", "zoom in: unfold each leg's waypoints (Lv2)"},
	{"⇧ tab", "zoom back out to the trail (esc too)"},
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
		dimStyle.Render(clip("compass observes; enter hands you the session.", w)),
		dimStyle.Render(clip("fleet:  ● working  ▲ needs you  ◍ stuck  ○ idle", w)),
		dimStyle.Render(clip("trail:  ◉ prompt  ◆ leg  ● now  ◈ subagent", w)),
		dimStyle.Render(clip("        ◌ planned — Claude's own next moves", w)),
	)
	return lines
}
