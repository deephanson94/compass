package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Round nineteen of the operator review.

// In the session view at 80, `enter` and `esc` go before the help under a
// note: five session-view frames ended "· q quit" alone, the one row an
// 80-column deck finds the help from.
func TestEnterAndEscYieldToTheHelpUnderANote(t *testing.T) {
	m := sceneModel(sceneFewOngoing(), 80, 24)
	pressKey(m, "tab")
	for _, note := range []string{"at the start of the trail", "mirror needs 110 columns", "the deepest level"} {
		m.note = note
		foot := ansi.Strip(m.footerLine(78))
		if !strings.Contains(foot, "? help") || !strings.Contains(foot, note) {
			t.Errorf("the legs footer under %q: %q", note, foot)
		}
		if strings.Contains(foot, "…") {
			t.Errorf("the note is clipped while a key stands: %q", foot)
		}
	}
	m.note = "no later prompt · G is the present"
	if foot := ansi.Strip(m.footerLine(98)); !strings.Contains(foot, "[ ] chapters") {
		t.Errorf("a chapter key's refusal keeps the chapter keys: %q", foot)
	}
}

// A note's quote is clipped at a word boundary, and under a dozen
// characters it goes whole: "please c…" answered nothing.
func TestTheQuoteClipsAtAWord(t *testing.T) {
	form := `◉ 10/12 · "please continue — our quota is back" · 14:47`
	got := fitQuote(form, 40)
	if got == "" || !strings.Contains(got, `"please continue`) || !strings.HasSuffix(got, ` · 14:47`) {
		t.Errorf("fitQuote(40) = %q", got)
	}
	inner := got[strings.Index(got, `"`)+1 : strings.LastIndex(got, `"`)]
	kept := strings.TrimSuffix(inner, "…")
	if !strings.HasPrefix("please continue — our quota is back", kept) || strings.HasSuffix(kept, " ") {
		t.Errorf("the clip is not at a word boundary: %q", inner)
	}
	if next := "please continue — our quota is back"[len(kept):]; next != "" && next[0] != ' ' {
		t.Errorf("the clip falls inside a word: %q then %q", kept, next)
	}
	if got := fitQuote(form, 24); got != "" {
		t.Errorf("under a dozen characters the quote should go whole, got %q", got)
	}
}

// The reader's turn note carries the whole turn: the footer clips it to
// its room, so a 220-column footer no longer cuts a prompt at 38.
func TestTheTurnNoteIsNotCapped(t *testing.T) {
	m := sceneModel(sceneVeryLong(), 220, 48)
	pressKey(m, "tab")
	pressKey(m, "tab")
	pressKey(m, "[")
	if m.anchorText == "" || !strings.Contains(m.note, `"`+m.anchorText+`"`) {
		t.Errorf("the turn note should quote the whole turn: note %q, turn %q", m.note, m.anchorText)
	}
}

// The 80-column help's trail row carries the lanes when the lanes' own row
// does not survive: the board shows all three and the help defined none.
func TestTheNarrowestHelpDefinesTheLanes(t *testing.T) {
	help := strings.Join(helpLinesFor(78, 19, false), "\n")
	if !strings.Contains(help, "⋯ out") || !strings.Contains(help, "⌀ ") {
		t.Errorf("the 80x24 help lacks the lanes:\n%s", help)
	}
}

// The reader's page never opens on a result row without its owner: the
// "↩ result of Bash(…)" and its "⎿" are one block.
func TestTheReaderPageKeepsAResultWithItsOwner(t *testing.T) {
	sc := sceneSubagents()
	m := sceneModel(sc, 100, 30)
	for _, s := range sc.sessions {
		if sessionName(s.Info) == "porter" {
			m.point(s.Info.Key())
			poll(m, sc)
		}
	}
	pressKey(m, "tab")
	pressKey(m, "tab")
	doc := m.doc(m.readerWidth())
	top := m.readerTop(doc)
	if top > 0 && isResultRow(doc[top]) && doc[top-1].kind == readerCall {
		// The tail page keeps its last row: the row above names the owner.
		if above := ansi.Strip(m.readerAbove(96)); !strings.Contains(above, "result of Bash") {
			t.Errorf("the page opens on a bare result and the row above does not name its owner: %q", above)
		}
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "result of Bash") {
		t.Errorf("a result at the top of the page has no owner on it:\n%s", view)
	}
}

// The 100-column help spends its row of air on the tag's line: ⌁ and
// unread went undefined while a blank row sat under "keys".
func TestTheHundredColumnHelpDefinesTheTag(t *testing.T) {
	help := strings.Join(helpLinesFor(98, 25, false), "\n")
	if !strings.Contains(help, "⌁ dev:1.0") || !strings.Contains(help, "unread —") {
		t.Errorf("the 100x30 help lacks the tag's line:\n%s", help)
	}
}

// A fleet of one offers no `x`: it refused when pressed. And the 80 help's
// read-line row names the tag, the first row under FLEET on every frame.
func TestAFleetOfOneOffersNoHideAndTheNarrowestHelpNamesTheTag(t *testing.T) {
	one := sceneModel(sceneFirstSession(), 100, 30)
	if foot := ansi.Strip(one.footerLine(98)); strings.Contains(foot, "x hide") || !strings.Contains(foot, "/ search") {
		t.Errorf("a fleet of one: %q", foot)
	}
	help := strings.Join(helpLinesFor(78, 19, false), "\n")
	if !strings.Contains(help, "⌁ pane") && !strings.Contains(help, "⌁ its pane") && !strings.Contains(help, "⌁ its tmux pane") {
		t.Errorf("the 80x24 help lacks the tag:\n%s", help)
	}
}
