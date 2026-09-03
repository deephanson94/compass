package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/deephanson94/compass/internal/transcript"
)

// The session view: the trail leads and the conversation reads to its right.
// The trail is the column the board handed over, the keys are on it, and its
// card names the session; the companion — conversation or live pane — is
// what the cursor is pointing at, and it stands where a file stands beside
// its tree.
func TestTheTrailLeadsTheSessionView(t *testing.T) {
	forceASCII(t)

	build := func(w int, mirror bool) *Model {
		m := New(nil)
		m.SetSize(w, 30)
		m.SetSessions(fixtureGroupedFleet(fixtureBase), fixtureBase.Add(40*time.Minute))
		panes, list := fixtureGroupedPanes()
		m.SetPanes(panes)
		m.SetPaneOrder(list)
		m.point(sessionKey("s-api"))
		m.SetTrail(fixtureTrail(fixtureBase))
		openTrail(m)
		if mirror {
			press(m, "m")
		}
		return m
	}
	titleRow := func(m *Model) (string, string) {
		lines := strings.Split(m.View(), "\n")
		parts := strings.SplitN(lines[3], "│", 2)
		if len(parts) != 2 {
			t.Fatalf("the session view is not two columns:\n%s", lines[3])
		}
		return parts[0], parts[1]
	}

	left, right := titleRow(build(200, false))
	if !strings.Contains(left, "2 ● api") || !strings.Contains(right, "READER") {
		t.Errorf("the trail's card should lead and the reader follow:\n%q\n%q", left, right)
	}
	left, right = titleRow(build(200, true))
	if !strings.Contains(left, "2 ● api") || !strings.Contains(right, mirrorMark+" ") {
		t.Errorf("the trail's card should lead and the live pane follow:\n%q\n%q", left, right)
	}
	// And the keys' owner is marked where it is drawn.
	m := build(200, false)
	pressTab(m)
	left, right = titleRow(m)
	if strings.HasPrefix(strings.TrimSpace(left), "▌") || !strings.Contains(right, "▌READER") {
		t.Errorf("at Lv3 the mark belongs to the reader on the right:\n%q\n%q", left, right)
	}
}

// A call's path is shown relative to the session's directory, the way the
// CLI shows it: `Edit(internal/ui/reader.go)`, not the whole path from the
// root. The line's own cwd wins; the session's is the fallback; a path
// outside both stays as it is.
func TestCallPathsAreRelativeToTheSession(t *testing.T) {
	forceASCII(t)

	path := func(p string) json.RawMessage {
		raw, _ := json.Marshal(map[string]string{"file_path": p})
		return raw
	}
	base := fixtureBase
	ev := []transcript.Event{
		{Type: transcript.EventAssistant, Timestamp: base, CWD: "/home/user/api",
			ToolUses: []transcript.ToolUse{{ID: "a", Name: "Read", Input: path("/home/user/api/src/tokens.py")}}},
		{Type: transcript.EventAssistant, Timestamp: base.Add(time.Second),
			ToolUses: []transcript.ToolUse{{ID: "b", Name: "Edit", Input: path("/home/user/api/src/auth.py")}}},
		{Type: transcript.EventAssistant, Timestamp: base.Add(2 * time.Second), CWD: "/home/user/api",
			ToolUses: []transcript.ToolUse{{ID: "c", Name: "Write", Input: path("/etc/hosts")}}},
		// The tool's own wording names the whole path too; the preview
		// shortens it the same way. The unfolded body is left verbatim.
		{Type: transcript.EventUser, Timestamp: base.Add(3 * time.Second), CWD: "/home/user/api",
			ToolResults: []transcript.ToolResult{{ToolUseID: "b", Text: "The file /home/user/api/src/auth.py has been updated."}}},
	}
	got := RenderReader(ev, ReaderOpts{Width: 80, Height: 20, CWD: "/home/user/api/"})
	for _, want := range []string{"⏺ Read(src/tokens.py)", "⏺ Edit(src/auth.py)", "⏺ Write(/etc/hosts)", "⎿ The file src/auth.py has been updated."} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "/home/user/api/src") {
		t.Errorf("a path under the session's directory kept the directory:\n%s", got)
	}
}

// A folded result leads with the first line that says something and how
// much follows it; a file's contents are counted, not quoted; and an
// unfolded result's row is the count alone, so the first line is not read
// twice. "space unfolds" is the footer's line, not every row's.
func TestFoldRowsLeadWithTheOutput(t *testing.T) {
	forceASCII(t)

	got := RenderReader(fixtureEvents(fixtureBase), ReaderOpts{Width: 70, Height: 20})
	if !strings.Contains(got, "⎿ collected 20 items · 5 more lines") {
		t.Errorf("the pytest result should lead with its first line:\n%s", got)
	}
	if strings.Contains(got, "space unfolds") {
		t.Errorf("every row said how to unfold itself:\n%s", got)
	}
	got = RenderReader(fixtureEvents(fixtureBase), ReaderOpts{Width: 70, Height: 20, Unfolded: map[int]bool{3: true}})
	if !strings.Contains(got, "⎿ 6 lines\n    collected 20 items") {
		t.Errorf("an unfolded result should be counted, then shown:\n%s", got)
	}

	base := fixtureBase
	call := func(id, name, input string, at time.Time) transcript.Event {
		return transcript.Event{Type: transcript.EventAssistant, Timestamp: at,
			ToolUses: []transcript.ToolUse{{ID: id, Name: name, Input: json.RawMessage(input)}}}
	}
	result := func(id, text string, at time.Time) transcript.Event {
		return transcript.Event{Type: transcript.EventUser, Timestamp: at,
			ToolResults: []transcript.ToolResult{{ToolUseID: id, Text: text}}}
	}
	ev := []transcript.Event{
		call("r", "Read", `{"file_path":"a.go"}`, base),
		result("r", "package ui\n\nimport \"fmt\"", base.Add(time.Second)),
		call("p", "Bash", `{"command":"pytest -q"}`, base.Add(2*time.Second)),
		result("p", "..........\n1 passed in 0.4s", base.Add(3*time.Second)),
		call("g", "Bash", `{"command":"git push"}`, base.Add(4*time.Second)),
		result("g", "\nEverything up-to-date\n", base.Add(5*time.Second)),
		call("f", "Bash", `{"command":"pytest -q"}`, base.Add(6*time.Second)),
		{Type: transcript.EventUser, Timestamp: base.Add(7 * time.Second),
			ToolResults: []transcript.ToolResult{{ToolUseID: "f", IsError: true, Text: "..........F\nFAILED test_a.py::test_x"}}},
	}
	got = RenderReader(ev, ReaderOpts{Width: 70, Height: 20})
	for _, want := range []string{
		"⎿ 3 lines",                        // a file is counted, not quoted
		"⎿ 1 passed in 0.4s · 1 more line", // the dots are not what the run said
		"⎿ Everything up-to-date",          // one line is the line
		"⎿ ✗ FAILED test_a.py::test_x",     // a failure leads with what failed, not its dots
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// Prose wraps at a reading measure however wide the panel is: a paragraph
// across 150 columns is a wall. Calls and results still have the width.
func TestProseKeepsAReadingMeasure(t *testing.T) {
	forceASCII(t)

	long := strings.Repeat("the expiry check compares a UTC stamp against local time, ", 8)
	base := fixtureBase
	ev := []transcript.Event{
		{Type: transcript.EventUser, Timestamp: base, Text: long},
		{Type: transcript.EventAssistant, Timestamp: base.Add(time.Second), Text: long},
		{Type: transcript.EventAssistant, Timestamp: base.Add(2 * time.Second),
			ToolUses: []transcript.ToolUse{{ID: "b", Name: "Bash", Input: json.RawMessage(`{"command":"` + strings.Repeat("x", 130) + `"}`)}}},
	}
	got := RenderReader(ev, ReaderOpts{Width: 160, Height: 30})
	widest, call := 0, 0
	for _, line := range strings.Split(got, "\n") {
		w := lipgloss.Width(strings.TrimRight(line, " "))
		if strings.HasPrefix(line, glyphCall) {
			call = w
			continue
		}
		if strings.HasPrefix(line, glyphSaid) {
			continue // the turn's row carries its clock at the panel's edge
		}
		if w > widest {
			widest = w
		}
	}
	if widest > readerMeasure {
		t.Errorf("prose ran to %d columns; the measure is %d:\n%s", widest, readerMeasure, got)
	}
	if widest < readerMeasure-20 {
		t.Errorf("prose wrapped at %d columns on a 160-column panel; the measure is %d", widest, readerMeasure)
	}
	if call <= readerMeasure {
		t.Errorf("a call line wrapped at the prose measure (%d); it has the width", call)
	}
}

// The model's words are markdown more often than not. The reader honours
// the little a terminal can: a heading bold, a fenced block set off and left
// unwrapped, a bullet as a bullet, ** dropped rather than printed.
func TestProseHonoursTheMarkdownATerminalCan(t *testing.T) {
	text := "## Plan\n\n- **first** step\n  - nested\n- second\n\n```go\nfunc x() {\treturn }\n```\nDone."
	rows := proseRows(text, 60, 80)
	var got []string
	for _, r := range rows {
		got = append(got, r.text)
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"Plan\n", "• first step", "  • nested", "• second", "  func x() {    return }", "Done."} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
	for _, never := range []string{"```", "**", "## "} {
		if strings.Contains(joined, never) {
			t.Errorf("printed %q at the reader:\n%s", never, joined)
		}
	}
	if rows[0].kind != readerHead {
		t.Errorf("the heading is a %v row, not a heading", rows[0].kind)
	}
	code := false
	for _, r := range rows {
		if strings.Contains(r.text, "func x()") {
			code = r.kind == readerCode
		}
	}
	if !code {
		t.Error("the fenced block is not a code row")
	}
	if rows[len(rows)-1].kind == readerBlank {
		t.Error("the document keeps the air the text ended with")
	}
}

// A turn of yours carries its clock on the right, dim: the turns are the
// conversation's chapters, and `[ ]` should land on a moment with a name. A
// panel with no room for it says the words and drops the clock.
func TestYourTurnsCarryTheirClock(t *testing.T) {
	forceASCII(t)

	got := RenderReader(fixtureEvents(fixtureBase), ReaderOpts{Width: 60, Height: 20})
	first := strings.Split(got, "\n")[0]
	clock := fixtureBase.Local().Format("15:04")
	if !strings.HasPrefix(first, "❯ fix the 401 bug") || !strings.HasSuffix(strings.TrimRight(first, " "), clock) {
		t.Errorf("the turn should carry %s on its right: %q", clock, first)
	}
	got = RenderReader(fixtureEvents(fixtureBase), ReaderOpts{Width: 40, Height: 20})
	first = strings.Split(got, "\n")[0]
	if !strings.HasSuffix(strings.TrimRight(first, " "), clock) {
		t.Errorf("a turn that fills its row wraps and keeps the clock: %q", first)
	}
}

// A call's name is the row's accent and its argument is dim, so a stretch
// of calls reads as "Read, Read, Edit, Bash" and the prose between is the
// one thing in the page's own colour. The calls whose argument is the point
// — a question to you, an agent's assignment — keep it plain.
func TestACallsArgumentIsDim(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	base := fixtureBase
	ev := []transcript.Event{
		{Type: transcript.EventAssistant, Timestamp: base,
			ToolUses: []transcript.ToolUse{{ID: "a", Name: "Read", Input: json.RawMessage(`{"file_path":"src/tokens.py"}`)}}},
		{Type: transcript.EventAssistant, Timestamp: base.Add(time.Second),
			ToolUses: []transcript.ToolUse{{ID: "q", Name: "AskUserQuestion", Input: json.RawMessage(`{"questions":[{"question":"Narrow or wide?","options":[{"label":"narrow"}]}]}`)}}},
	}
	got := RenderReader(ev, ReaderOpts{Width: 80, Height: 10, Anchor: -1})
	var read, ask string
	for _, line := range strings.Split(got, "\n") {
		switch plain := ansi.Strip(line); {
		case strings.HasPrefix(plain, "⏺ Read("):
			read = line
		case strings.HasPrefix(plain, "⏺ AskUserQuestion("):
			ask = line
		}
	}
	dim := dimStyle.Render("(src/tokens.py)")
	if read == "" || !strings.Contains(read, dim) {
		t.Errorf("the argument should be dim: %q", read)
	}
	if strings.Contains(read, dimStyle.Render("⏺ Read")) {
		t.Errorf("the name should not be dim: %q", read)
	}
	// No escape between the name and the question: the whole row is one accent.
	if ask == "" || !strings.Contains(ask, "AskUserQuestion(Narrow or wide? [narrow])") {
		t.Errorf("the question is the call, and stays plain: %q", ask)
	}
	// The document is one string per row: a search still matches across
	// the seam between the name and the argument.
	doc := readerDoc(ev, ReaderOpts{Width: 80})
	if m := readerMatches(doc, "read(src"); len(m) != 1 {
		t.Errorf("a query across the seam matched %d rows", len(m))
	}
}

// The gateway's refusal is drawn warm in the reader, not as the model's
// words: a session dead on quota read as prose at the foot of the page.
func TestTheReaderDrawsAnAPIErrorWarm(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	ev := []transcript.Event{
		{Type: transcript.EventAssistant, Timestamp: fixtureBase, Text: "Please run /login · API Error: 403 your daily quota is exhausted", APIError: true, Status: 403},
	}
	got := RenderReader(ev, ReaderOpts{Width: 80, Height: 5, Anchor: -1})
	if !strings.Contains(ansi.Strip(got), "✗ Please run /login · API Error: 403") {
		t.Errorf("the refusal should lead with the failure mark:\n%s", got)
	}
	if !strings.Contains(got, stuckStyle.Render("✗ Please run /login · API Error: 403 your daily quota is exhausted")) {
		t.Errorf("the refusal should be drawn in the stuck colour:\n%s", got)
	}
}
