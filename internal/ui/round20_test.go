package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Round twenty of the operator review.

// A step down never stalls on a call whose result is the next row, and the
// tail page keeps its last row: the present outranks the page's first row.
func TestTheReaderStepsPastAResultBlockAndKeepsItsTail(t *testing.T) {
	m := sceneModel(sceneVeryLong(), 80, 24)
	pressKey(m, "tab")
	pressKey(m, "tab")
	doc := m.doc(m.readerWidth())
	if top := m.readerTop(doc); top+m.readerHeight() < len(doc) {
		t.Errorf("the tail page is short of the present: top %d + %d rows < %d", top, m.readerHeight(), len(doc))
	}
	m.scroll = 0
	moved, stalled := 0, 0
	for i := 0; i < 60; i++ {
		was := m.readerTop(doc)
		if m.scrollBy(1) && m.readerTop(doc) > was {
			moved++
		} else {
			stalled++
		}
	}
	if stalled > 0 {
		t.Errorf("j stalled %d times in 60 steps from the top", stalled)
	}
}

// The reader's `]` refusal is a chapter note and keeps the chapter keys.
func TestTheReadersChapterRefusalKeepsItsKeys(t *testing.T) {
	m := sceneModel(sceneFewOngoing(), 80, 24)
	pressKey(m, "tab")
	pressKey(m, "tab")
	m.note = "no later turn"
	if foot := ansi.Strip(m.footerLine(78)); !strings.Contains(foot, "[ ] turns") || !strings.Contains(foot, "? help") {
		t.Errorf("the reader's refusal sheds its own key: %q", foot)
	}
}

// Shed keys come back where they fit: a list with 14 free cells offered
// none of `x hide` and `g grab`, and the archive's hidden row lost `x
// unhide` to the way in.
func TestShedKeysComeBackWhereTheyFit(t *testing.T) {
	m := groupedModel(80, 24)
	foot := ansi.Strip(strings.Split(m.View(), "\n")[23])
	if !strings.Contains(foot, "x hide") || ansi.StringWidth(strings.TrimSpace(foot)) > 78 {
		t.Errorf("the 80 list: %q", foot)
	}
	wide := groupedModel(100, 30)
	if foot := ansi.Strip(strings.Split(wide.View(), "\n")[29]); !strings.Contains(foot, "g grab") {
		t.Errorf("the 100 list: %q", foot)
	}
	if got := shedKeys("a · / search · n/N · b", []string{" · n/N", " · / search"}, func(k string) bool { return len(k) <= 12 }); strings.Contains(got, "n/N") {
		t.Errorf("n/N came back without its search: %q", got)
	}
	h := sceneModel(sceneFleetHygiene(), 80, 24)
	pressKey(h, "x")
	pressKey(h, "A")
	if foot := ansi.Strip(strings.Split(h.View(), "\n")[23]); !strings.Contains(foot, "x unhide") {
		t.Errorf("the archive's hidden row lost x unhide: %q", foot)
	}
}

// The board's `m` says what it did, as a state and not a refusal; the
// archive's empty state sheds its clause whole.
func TestTheBoardMirrorNoteIsAState(t *testing.T) {
	m := sceneModel(sceneFewOngoing(), 120, 34)
	pressKey(m, "m")
	if !strings.HasPrefix(m.note, "mirror on") {
		t.Errorf("m on the board: %q", m.note)
	}
	e := sceneModel(sceneFirstSession(), 80, 24)
	pressKey(e, "x")
	pressKey(e, "A")
	if view := ansi.Strip(e.View()); strings.Contains(view, "they land h…") || strings.Contains(view, "they…") {
		t.Errorf("the empty state cuts mid-word:\n%s", view)
	}
}

// A step past a call-and-result block lands on a line, never on the air
// after it: a page whose first row is blank opened on nothing.
func TestAStepPastABlockLandsOnALine(t *testing.T) {
	m := sceneModel(sceneManyIdle(), 80, 24)
	pressKey(m, "tab")
	pressKey(m, "tab")
	doc := m.doc(m.readerWidth())
	m.scroll = 0
	for i := 0; i < 40; i++ {
		if !m.scrollBy(1) {
			break
		}
		if top := m.readerTop(doc); top < len(doc) && doc[top].kind == readerBlank {
			t.Fatalf("step %d landed the page on a line of air at row %d", i, top)
		}
	}
}

// The pane clause outranks the attach hint: the parenthetical stood where
// the hidden session's pane should have been.
func TestThePaneClauseOutranksTheAttachHint(t *testing.T) {
	m := sceneModel(sceneManyIdle(), 152, 40)
	m.inTmux = false
	m.note = "3 webapp hidden · A, then x · ⌁ work:2.0"
	foot := ansi.Strip(m.footerLine(150))
	if !strings.Contains(foot, "⌁ work:2.0") || strings.Contains(foot, "prefix d") {
		t.Errorf("the hint stands where the pane should: %q", foot)
	}
}

// The reader's above-row keeps the turn count when it names an owner.
func TestTheAboveRowKeepsItsTurnCount(t *testing.T) {
	m := sceneModel(sceneManyIdle(), 100, 30)
	pressKey(m, "tab")
	pressKey(m, "tab")
	doc := m.doc(m.readerWidth())
	if top := m.readerTop(doc); top > 0 && isResultRow(doc[top]) && doc[top-1].kind == readerCall {
		above := ansi.Strip(m.readerAbove(98))
		if !strings.Contains(above, "of yours") || !strings.Contains(above, "⏺") {
			t.Errorf("the above-row drops one of the two facts: %q", above)
		}
	}
}

// Every level keeps its own keys longest, and under a note the keys
// everyone knows go first: the reader offered `a ask` on a row that could
// not fit `[ ] turns`, and a list named the chapters of a trail it had
// not opened.
func TestEachLevelKeepsItsOwnKeysLongest(t *testing.T) {
	m := sceneModel(sceneVeryLong(), 80, 24)
	pressKey(m, "tab")
	pressKey(m, "tab")
	m.note = "no later turn"
	if foot := ansi.Strip(m.footerLine(78)); !strings.Contains(foot, "[ ] turns") || strings.Contains(foot, "a ask") {
		t.Errorf("the reader's row: %q", foot)
	}
	list := groupedModel(80, 24)
	if foot := ansi.Strip(strings.Split(list.View(), "\n")[23]); strings.Contains(foot, "[ ] chapters") {
		t.Errorf("the list names a trail's keys: %q", foot)
	}
	if foot := ansi.Strip(strings.Split(groupedModel(120, 34).View(), "\n")[33]); !strings.Contains(foot, "? help") {
		t.Errorf("the board's row lost its help: %q", foot)
	}
}

// The 80-column help names the keys the deck offers and not the ones it
// refuses: a newcomer on a fleet of one read `g grab` and `x hide` there
// and got a refusal from both.
func TestTheNarrowHelpNamesWhatTheDeckOffers(t *testing.T) {
	one := sceneModel(sceneFirstSession(), 80, 24)
	help := strings.Join(helpLinesFor(78, 19, one.boardFits(), one.refusedKeys()...), "\n")
	for _, want := range []string{"[ ]", "a  ", "previous / next prompt", "ask: a claude"} {
		if !strings.Contains(help, want) {
			t.Errorf("the 80 help lacks %q:\n%s", want, help)
		}
	}
	for _, gone := range []string{"grab the session waiting longest", "hide a session"} {
		if strings.Contains(help, gone) {
			t.Errorf("the 80 help names a key the deck refuses: %q", gone)
		}
	}
}

// The help keeps the rows for the keys the deck's own footer is offering:
// an 80-column help named `g`, which no footer there offers, while `x` —
// on nine of them — was cut for the room.
func TestTheHelpKeepsWhatTheFooterOffers(t *testing.T) {
	m := groupedModel(80, 24)
	help := strings.Join(helpLinesWith(78, 19, helpOpts{board: m.boardFits(), refused: m.refusedKeys(), keymap: m.keymap()}), "\n")
	if !strings.Contains(m.keymap(), "x hide") {
		t.Fatalf("the fixture's footer does not offer x: %q", m.keymap())
	}
	if !strings.Contains(help, "hide a session") {
		t.Errorf("the help drops a key the footer offers:\n%s", help)
	}
}

// Every level asks what `enter` does rather than promising an attach: a
// paneless session said `enter · no pane` on its list and `enter attach`
// one keypress deeper.
func TestEveryLevelAsksWhatEnterDoes(t *testing.T) {
	m := sceneModel(sceneFleetHygiene(), 120, 34)
	for _, s := range m.sessions {
		if _, has := m.panes[s.Info.Key()]; !has {
			m.point(s.Info.Key())
		}
	}
	if strings.Contains(m.keymap(), "enter attach") {
		t.Fatalf("the fixture's row has a pane: %q", m.keymap())
	}
	for _, level := range []string{"legs", "reader"} {
		pressKey(m, "tab")
		if k := m.keymap(); !strings.Contains(k, "enter · no pane") {
			t.Errorf("the %s keymap promises an attach: %q", level, k)
		}
	}
}
