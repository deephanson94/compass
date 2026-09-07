package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/deephanson94/compass/internal/state"
)

// The pins the panel asked for (#53, #55): each fold of rounds thirty-one
// and thirty-two that a later width change could revert silently.

// A budgeted HEAD is named by its call, as a hung one is (#54).
func TestABudgetedHeadIsNamedByItsCall(t *testing.T) {
	forceASCII(t)
	m := boardModel(152, 30)
	api := sessionKey("s-api")
	for i := range m.sessions {
		if m.sessions[i].Info.Key() == api {
			m.sessions[i].Snap = state.Snapshot{State: state.Working, Since: fixtureBase.Add(34 * time.Minute),
				Reason: "Bash allowed 10m", Activity: "Bash: sleep 420", Allowed: 10 * time.Minute}
			m.sessions[i].Info.LastEventAt = fixtureBase.Add(34 * time.Minute)
		}
	}
	col := strings.Join(m.boardColumn(api, rowFor(t, m, api), 70, 20), "\n")
	if !strings.Contains(col, "Bash: sleep 420") || !strings.Contains(col, "for 6m of 10m") {
		t.Errorf("the budgeted HEAD does not name its call beside its budget:\n%s", col)
	}
}

// A clip never cuts a number in half and never leaves a bare separator
// or the space it stood on before the mark (#53, #55).
func TestAClipKeepsNumbersWholeAndNoBareSeparator(t *testing.T) {
	for in, want := range map[[2]any]string{
		{"API Error: 403 quota", 14}:    "API Error:…",
		{"a · b", 4}:                    "a…",
		{"x 12", 3}:                     "x…",
		{"Please run /login · API", 20}: "Please run /login…",
		{"hello world", 20}:             "hello world",
	} {
		if got := clip(in[0].(string), in[1].(int)); got != want {
			t.Errorf("clip(%q, %d) = %q, want %q", in[0], in[1], got, want)
		}
	}
	for _, n := range []int{16, 17} {
		if got := ansi.Strip(truncateWhole("↪ answered 1 · 50s ago", n)); strings.HasSuffix(got, "5") || strings.HasSuffix(got, "·") || strings.HasSuffix(got, " ") {
			t.Errorf("truncateWhole(%d) cut a number or left a bare separator: %q", n, got)
		}
	}
	if got := ansi.Strip(truncateWhole("↪ answered 1 · 50s ago", 16)); got != "↪ answered 1" {
		t.Errorf("truncateWhole(16) = %q, want the clause before the number", got)
	}
	if legend := strings.Join(helpLegendLines(200, true, true, true), "\n"); !strings.Contains(legend, "of 10m") {
		t.Errorf("the legend does not gloss the budget's clock")
	}
}

// A fleet of one has no board at any width: its wide help carries no
// board row, and none of the row's wrapped tail (#53, #55).
func TestAFleetOfOnesWideHelpNamesNoBoard(t *testing.T) {
	m := sceneModel(sceneSecondDay(), 152, 40)
	press(m, "?")
	view := ansi.Strip(m.View())
	if strings.Contains(view, "board:") || strings.Contains(view, "band below") {
		t.Errorf("the wide help of a fleet of one still teaches a board:\n%s", view)
	}
	if !strings.Contains(view, "recent") {
		t.Errorf("the help does not name the band's digit:\n%s", view)
	}
}

// The reply head keeps the pane it exists to name and sheds the tool at
// eighty columns; the archive's hidden row reads tool first; the lead's
// reader says an empty lane came back with no report (#54).
func TestTheReplyHeadKeepsItsPaneAndTheArchiveRowLeadsWithTheTool(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 80, 24)
	press(m, "2")
	press(m, "r")
	panel := ansi.Strip(strings.Join(m.replyPanel(78), "\n"))
	if !strings.Contains(panel, "⌁ dev:2.0") || strings.Contains(panel, "opencode") {
		t.Errorf("at 80 the reply head should keep the pane and shed the tool:\n%s", panel)
	}
	press(m, "esc")
	wide := sceneModel(sceneTwoTools(), 120, 34)
	press(wide, "2")
	press(wide, "r")
	if panel := ansi.Strip(strings.Join(wide.replyPanel(118), "\n")); !strings.Contains(panel, "api · opencode · ⌁ dev:2.0") {
		t.Errorf("at 120 the reply head should name the tool:\n%s", panel)
	}
	press(wide, "esc")
	press(wide, "x")
	press(wide, "A")
	if list := ansi.Strip(strings.Join(wide.fleetLines(46, 28), "\n")); !strings.Contains(list, "opencode · ⌁ dev:2.0 · main") {
		t.Errorf("the archive's hidden row does not lead with the tool:\n%s", list)
	}
}

// An agent back with nothing is ⌀ in the reader, in the trail's words; a
// fold of one leg is "1 leg"; the lane reader's assignment wears the
// lane's glyph (#54, #55).
func TestTheReadersLaneWordsMatchTheTrails(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSubagents(), 120, 34)
	press(m, "3") // harness: an agent came back with no report
	poll(m, sceneSubagents())
	pressTab(m)
	pressTab(m)
	if m.level < levelReader {
		pressTab(m)
	}
	doc := m.doc(m.readerWidth())
	found := false
	for _, l := range doc {
		if strings.Contains(l.text, "⌀ came back with no report") {
			found = true
		}
		if strings.Contains(l.text, "no output") {
			t.Errorf("the empty lane's result is call-shaped: %q", l.text)
		}
	}
	if !found {
		t.Errorf("the reader does not say the lane came back with no report")
	}
	if got := plural(1, "leg"); got != "1 leg" {
		t.Errorf("plural(1, leg) = %q", got)
	}
	n := sceneModel(sceneSubagents(), 80, 24)
	pressTab(n)
	pressTab(n) // the newest lane's own conversation
	if n.readerLane == "" {
		t.Fatalf("Tab on the newest lane did not open its conversation")
	}
	if view := ansi.Strip(n.View()); !strings.Contains(view, "◈ Red-team the plugin architecture; report") || strings.Contains(view, "❯ Red-team") {
		t.Errorf("the assignment does not wear the lane's glyph:\n%s", view)
	}
}

// The way back from the archive restores the reader's page and title once
// the conversation lands, and the archive names A below its list (#55).
func TestTheWayBackRestoresTheReadersTitle(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 120, 34)
	pressTab(m) // the reader of hello
	if m.level != levelReader {
		t.Fatalf("expected the reader, got level %d", m.level)
	}
	before := ansi.Strip(m.readerTitle(60))
	press(m, "2") // the band: api's archive
	pressTab(m)
	if foot := ansi.Strip(m.footerLine(118)); !strings.Contains(foot, "A fleet") {
		t.Errorf("the archive's legs footer does not name A: %q", foot)
	}
	press(m, "A")
	poll(m, sceneSecondDay())
	if m.level != levelReader || m.archiveView {
		t.Fatalf("A did not return to the reader: level %d archive %v", m.level, m.archiveView)
	}
	if after := ansi.Strip(m.readerTitle(60)); !strings.Contains(after, "17:59") || after != before {
		t.Errorf("the reader's title after the way back = %q, want %q", after, before)
	}
}

// The tag gives way to a whole digest before it takes the model (#55).
func TestTheTagYieldsToAWholeDigest(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 152, 40)
	press(m, "1")
	m.sent[sessionKey("infra")] = sentReply{text: "office CIDR", at: m.now, answer: 1} // the trace "↪ answered 1 · 0s ago"
	col := strings.Join(m.boardColumn(sessionKey("infra"), rowFor(t, m, sessionKey("infra")), 48, 20), "\n")
	if !strings.Contains(col, "↪ answered 1 · ") || !strings.Contains(col, "0s ago") || strings.Contains(col, "0s…") {
		t.Errorf("a 48-cell column clips the trace to keep the model:\n%s", col)
	}
}
