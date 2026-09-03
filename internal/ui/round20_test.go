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
