package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Round seventeen of the operator review: what the few-ongoing, the
// alarm-storm, the fleet-hygiene and the subagents operators found.

// The archive is a list at every width: the mirror's sticky flag stays with
// the session view, so a board press that said "the mirror shows beside a
// session" cannot surface two screens later squeezing the archive's list.
func TestTheArchiveDrawsNoMirror(t *testing.T) {
	m := sceneModel(sceneFewOngoing(), 120, 34)
	pressKey(m, "tab") // the session view, where the mirror lives
	m.showMirror = true
	if !m.mirrorShown() {
		t.Fatal("the mirror should show at Lv1 on 120 columns once switched on")
	}
	m.toggleArchive()
	if m.mirrorShown() {
		t.Error("the archive drew the mirror")
	}
	if view := ansi.Strip(m.View()); strings.Contains(view, "until the pane is captured") {
		t.Error("the archive's frame carries the mirror's title")
	}
}

// The reader's title cuts a question's bracket clause whole, as the card
// does: "[office CIDR / keep bas…" named options the menu does not have.
func TestTheReaderTitleCutsTheBracketClauseWhole(t *testing.T) {
	q := "Open port 22 to the office CIDR only, or keep the bastion? [office CIDR / keep bastion]"
	if got := clipQuestion(q, 200); got != q {
		t.Errorf("a question that fits is changed: %q", got)
	}
	if got := clipQuestion(q, 70); strings.Contains(got, "[") || !strings.HasPrefix(got, "Open port 22") {
		t.Errorf("the bracket clause should go whole: %q", got)
	}
	m := sceneModel(sceneFewOngoing(), 100, 30)
	m.anchor, m.anchorAt, m.anchorText = 0, sceneNow, q
	title := ansi.Strip(m.readerTitle(98))
	if strings.Contains(title, "[") && !strings.Contains(title, "[office CIDR / keep bastion]") {
		t.Errorf("the reader's title cuts inside the brackets: %q", title)
	}
	if !strings.Contains(title, "17:") && !strings.Contains(title, "·") {
		t.Errorf("the clock went with the clause: %q", title)
	}
}

// The reply panel keeps every head row on the board: over a band shorter
// than the box it stands under the head rows of the band below, its air
// shed, rather than over the names of sessions it is not about.
func TestTheReplyPanelKeepsEveryHeadOnTheBoard(t *testing.T) {
	m := sceneModel(sceneFewOngoing(), 120, 34)
	pressKey(m, "1")
	pressKey(m, "r")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "┌ reply to 1 · infra") {
		t.Fatalf("no panel for infra:\n%s", view)
	}
	for _, head := range []string{"4 ● api", "5 ○ billing", "▸1 ▲ infra"} {
		if !strings.Contains(view, head) {
			t.Errorf("the panel covers the head row %q:\n%s", head, view)
		}
	}
}

// A dead session's panel states itself on one row: the row beneath carries
// the refusal already, and a four-row state stood the box on two other
// alarms' rows.
func TestADeadSessionsPanelStatesItselfOnOneRow(t *testing.T) {
	m := sceneModel(sceneAlarmStorm(), 120, 34)
	pressKey(m, "4")
	pressKey(m, "r")
	panel := m.replyPanel(118)
	state := 0
	for _, row := range panel {
		if strings.Contains(row, "stopped on an API error") {
			state++
		}
	}
	if state != 1 {
		t.Errorf("the dead session's state takes %d rows:\n%s", state, strings.Join(panel, "\n"))
	}
	if len(panel) > 14 {
		t.Errorf("the dead session's panel is %d rows tall", len(panel))
	}
	view := ansi.Strip(m.View())
	for _, head := range []string{"1 ▲ infra", "2 ◍ etl", "7 ↻ api", "▸4 ⊘ mobile"} {
		if !strings.Contains(view, head) {
			t.Errorf("the panel covers the head row %q:\n%s", head, view)
		}
	}
}

// The fold names the group it cut into: "⌁ :0.0" under "▴ 1 more above"
// was a pane of no tmux session, its header being the first thing hidden.
func TestTheFoldNamesTheGroupItCutInto(t *testing.T) {
	lines := []string{" ⌁ harness", " 2 ● harness", "    second", "", " 4 ○ harness", "    second", "", " elsewhere", " 3 ○ notebooks"}
	if got := foldedHeader(lines, 4); got != "⌁ harness" {
		t.Errorf("the fold at an entry should name its group: %q", got)
	}
	if got := foldedHeader(lines, 7); got != "" {
		t.Errorf("a window opening on a header names nothing: %q", got)
	}
	if got := foldedHeader(lines, 6); got != "" {
		t.Errorf("a window opening on the air before a header names nothing: %q", got)
	}
	m := groupedModel(120, 30)
	rows := m.fleetRows()
	full, selStart, selEnd := m.fleetBlock(rows, fleetWidth)
	for i := 0; i < len(m.fleetOrder()); i++ {
		full, selStart, selEnd = m.fleetBlock(m.fleetRows(), fleetWidth)
		out := m.scrollFleet(full, selStart, selEnd, 8)
		if strings.HasPrefix(out[0], "▴") {
			plain := ansi.Strip(out[0])
			first := out[1]
			if !isHeaderLine(first) && first != "" && !strings.Contains(plain, " · ") {
				t.Errorf("the fold over an entry names no group: %q over %q", plain, first)
			}
			if !strings.Contains(plain, "more above · k") {
				t.Errorf("the fold lost its count: %q", plain)
			}
		}
		m.move(1)
	}
}

// A trace's destination — "to ⌁ ops:0.0", where the bytes went — outranks
// the optional keys and yields only to the help; a bare pane tail goes
// before any key.
func TestTheTraceKeepsItsDestinationOverTheOptionalKeys(t *testing.T) {
	m := sceneModel(sceneFewOngoing(), 100, 30)
	m.note = `↪ answered 1 · "office CIDR" · to ⌁ ops:0.0`
	wide := ansi.Strip(m.footerLine(98))
	if !strings.Contains(wide, "to ⌁ ops:0.0") || !strings.Contains(wide, "? help") || strings.Contains(wide, "x hide") {
		t.Errorf("at 100 the destination should outlast the optional keys: %q", wide)
	}
	narrow := ansi.Strip(m.footerLine(78))
	if !strings.Contains(narrow, "to ⌁ ops:0.0") || !strings.Contains(narrow, "? help") || strings.Contains(narrow, `"office CIDR"`) {
		t.Errorf("at 80 an answer sheds its quote before its destination: %q", narrow)
	}
	m.note = `↪ sent "please continue" · to ⌁ ops:0.0`
	typed := ansi.Strip(m.footerLine(78))
	if strings.Contains(typed, "to ⌁") || !strings.Contains(typed, "? help") || !strings.Contains(typed, `"please continue"`) {
		t.Errorf("at 80 a typed line's destination yields to the help: %q", typed)
	}
	m.note = "2 harness hidden · A, then x · ⌁ harness:1.0"
	hide := ansi.Strip(m.footerLine(78))
	if strings.Contains(hide, "⌁ harness") || !strings.Contains(hide, "A, then x") || !strings.Contains(hide, "? help") {
		t.Errorf("a bare pane tail goes before any key: %q", hide)
	}
}

// The one-column help defines the tag and unread among its first spare
// rows: they are the words every namesake and every finished row wears.
func TestTheOneColumnHelpDefinesTheTag(t *testing.T) {
	help := strings.Join(helpLinesFor(120, 29, true), "\n")
	for _, want := range []string{"⌁ dev:1.0", "unread —", "you were here"} {
		if !strings.Contains(help, want) {
			t.Errorf("the 120x34 help lacks %q:\n%s", want, help)
		}
	}
}

// The legend never breaks between a glyph and its term: "⊘" alone at the
// end of a row over "dead on the API ○ idle" read as two terms.
func TestTheLegendKeepsTheGlyphWithItsTerm(t *testing.T) {
	for _, w := range []int{140, 152, 160} {
		for _, line := range helpLinesFor(w, 35, true) {
			plain := strings.TrimRight(ansi.Strip(line), " ")
			if strings.HasSuffix(plain, "⊘") || strings.HasSuffix(plain, "▲") || strings.HasSuffix(plain, "dead on the") {
				t.Errorf("at %d the legend breaks inside a term: %q", w, plain)
			}
		}
	}
}

// A folded list leaves its slack above the strip: rows an entry-whole
// window could not use sit between the fold and the archive's line, not
// under it, where they read as room the fold could have shown.
func TestAFoldedListLeavesItsSlackAboveTheStrip(t *testing.T) {
	m := sceneModel(sceneFewOngoing(), 80, 24)
	lines := m.fleetLines(34, 12)
	if len(lines) != 12 {
		t.Fatalf("a folded list should fill its body: %d rows of 12\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if !strings.Contains(ansi.Strip(lines[len(lines)-1]), "archived") {
		t.Errorf("the archive's line is not the column's last word: %q", ansi.Strip(lines[len(lines)-1]))
	}
}

// The archive's trail title falls back to the project when the row's
// prompt does not fit, rather than "TRAIL" over nothing.
func TestTheArchiveTitleFallsBackToTheProject(t *testing.T) {
	m := sceneModel(sceneFewOngoing(), 80, 24)
	m.toggleArchive()
	title := strings.TrimSpace(ansi.Strip(m.trailTitle(24)))
	title = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(title, "▌")), "[trail]")
	if strings.TrimSpace(title) == "TRAIL" || !strings.HasPrefix(strings.TrimSpace(title), "TRAIL · ") {
		t.Errorf("the narrow archive title names nothing: %q", title)
	}
}
