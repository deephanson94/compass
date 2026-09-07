package ui

import (
	"time"

	"github.com/deephanson94/compass/internal/transcript"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The porter column draws the lead's message as the ask, worn as a relay:
// the card and the trail say `relayed "…"`, not the harness's envelope, and
// not nothing (#97).
func TestARelayedAskIsWornAsARelay(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 220, 48)
	view := ansi.Strip(m.View())
	if strings.Contains(view, "Another Claude session") {
		t.Errorf("the relay's envelope is drawn as the ask:\n%s", view)
	}
	if !strings.Contains(view, `relayed "the encoder is in, run the gates"`) {
		t.Errorf("the relayed ask is not worn as one:\n%s", view)
	}
	press(m, "A")
	if view := ansi.Strip(m.View()); !strings.Contains(view, `relayed "`) && strings.Contains(view, "porter") {
		t.Errorf("the archive's strip drops the relay word:\n%s", view)
	}
}

// A search narrows the band instead of blanking it: `/billing` keeps the
// band row it found, under a miss note that says what did not match (#98).
func TestASearchNarrowsTheBand(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 100, 30)
	for _, k := range []string{"/", "billing"} {
		pressKey(m, k)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "no live session matches /billing") {
		t.Errorf("the miss note does not say what missed:\n%s", view)
	}
	rows := 0
	for _, l := range strings.Split(view, "\n") {
		fleet := strings.SplitN(l, "│", 2)[0]
		if strings.Contains(fleet, " ○ ") {
			rows++
			if !strings.Contains(fleet, "billing") {
				t.Errorf("the band draws a row the search did not find: %q", l)
			}
		}
	}
	if rows != 1 {
		t.Errorf("the band under /billing draws %d rows, want the one it found:\n%s", rows, view)
	}
}

// At 120 the two-tools help glosses the tool word its rows wear: `?` and
// `q` share a row at every width, and the row they freed is the gloss's (#99).
func TestTheHelpMergesHelpAndQuitAtEveryWidth(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 120, 34)
	press(m, "?")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "? / q") {
		t.Errorf("? and q take a row each at 120:\n%s", view)
	}
	if !strings.Contains(view, "which CLI runs the session") {
		t.Errorf("the help leaves the word its rows wear undefined:\n%s", view)
	}
}

// The hide's note is the refusal's sentence (app.go's "%d %s is hidden"):
// a bare leading digit beside a strip that counts ("1 hidden") reads as a
// count, and the fleet with two sessions called harness is where it
// misreads (#101, #57, #31).
func TestTheHideNoteIsNotACount(t *testing.T) {
	m := sceneModel(sceneFleetHygiene(), 120, 34)
	press(m, "2")
	press(m, "x")
	if !strings.Contains(m.note, "2 harness is hidden · A, then x") {
		t.Errorf("the hide note reads as a count: %q", m.note)
	}
}

// In the session view the card keeps its tag and leaves the present to the
// trail, which draws HEAD three rows below in the same column (#100).
func TestTheCardLeavesThePresentToTheTrail(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 152, 40)
	view := ansi.Strip(m.View())
	if n := strings.Count(view, "thinking…"); n != 1 {
		t.Errorf("the column says the present %d times, want once:\n%s", n, view)
	}
	if !strings.Contains(view, "claude · ⌁ main") {
		t.Errorf("the card lost its tag with the sentence:\n%s", view)
	}
}

// In the session view the search's note says whose miss it is where the
// band beneath holds what the search found, as the list's note does (#102).
func TestTheSessionViewMissNoteSaysTheLiveOneMissed(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 152, 40)
	for _, k := range []string{"/", "billing"} {
		pressKey(m, k)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "billing · opencode") && !strings.Contains(view, " ○ billing") {
		t.Fatalf("the band draws no billing row under the search:\n%s", view)
	}
	if !strings.Contains(view, "no live session matches /billing") {
		t.Errorf("the card says no session matches over the band's match:\n%s", view)
	}
}

// The archive says a row's verdict: the second row ends on the band's own
// clause where the column has room (#103, #47).
func TestTheArchiveRowSaysItsVerdict(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 152, 40)
	press(m, "2")
	view := ansi.Strip(m.View())
	if !m.archiveView {
		t.Fatalf("2 did not open the archive:\n%s", view)
	}
	if !strings.Contains(view, "claude · fix/api-timeouts · ✗ red") {
		t.Errorf("the archive row says nothing of whether the day went red:\n%s", view)
	}
}

// #100 on every shape of the repeat: the cursor's mark and the leg's own
// class are not part of the sentence, so the card keeps only its tag where
// the trail draws the sentence below it under either (#104).
func TestTheCardLeavesThePresentToTheTrailUnderTheCursor(t *testing.T) {
	forceASCII(t)
	count := func(view, sentence string) int {
		n := 0
		for _, l := range strings.Split(view, "\n") {
			left := strings.SplitN(l, "│", 2)[0]
			if strings.Contains(oneSpace(strings.ReplaceAll(left, "▸", " ")), sentence) {
				n++
			}
		}
		return n
	}
	m := sceneModel(sceneTwoTools(), 120, 34)
	press(m, "2")
	press(m, "tab")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "[session]") || !strings.Contains(view, "●▸test") {
		t.Fatalf("not the session view with the cursor on HEAD:\n%s", view)
	}
	if n := count(view, "● test go test for 12m"); n != 1 {
		t.Errorf("the column says the present %d times, want once:\n%s", n, view)
	}
	m = sceneModel(sceneTwoTools(), 120, 34)
	press(m, "tab")
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "[session]") || !strings.Contains(view, "design Open port 22 to the office CIDR?") {
		t.Fatalf("not the session view of the needs-you session:\n%s", view)
	}
	if n := count(view, "Open port 22 to the office CIDR?"); n != 1 {
		t.Errorf("the column says the question %d times, want once:\n%s", n, view)
	}
}

// The archive's title names the session alone where the ◉ row draws the
// ask two rows below in the same panel (#105, #59).
func TestTheArchiveTitleLeavesTheAskToThePromptRow(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 100, 30)
	for _, k := range []string{"2", "tab", "x", "A"} { // the walkthrough's own route: api hidden, then the archive
		pressKey(m, k)
	}
	view := ansi.Strip(m.View())
	if !m.archiveView || !strings.Contains(view, "◉ \"add rate limiting") {
		t.Fatalf("not the archive with the ◉ row drawn:\n%s", view)
	}
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "TRAIL · ") && strings.Contains(l, "add rate limiting") {
			t.Errorf("the title copies the ask the ◉ row draws: %q", l)
		}
	}
}

// The reader draws a relayed ask the way the card and the trail do: the
// message, not the harness's envelope (#106, #97).
func TestTheReaderDrawsARelayedAskWithoutItsEnvelope(t *testing.T) {
	at := time.Date(2026, 9, 7, 17, 48, 0, 0, time.UTC)
	ev := []transcript.Event{{Type: transcript.EventUser, Timestamp: at,
		Text: "Another Claude session sent a message: the encoder is in, run the gates"}}
	var said string
	for _, l := range readerDoc(ev, ReaderOpts{Width: 60}) {
		if strings.HasPrefix(l.text, glyphSaid) {
			said = l.text
			break
		}
	}
	if strings.Contains(said, "Another Claude session") || !strings.Contains(said, "the encoder is in") {
		t.Errorf("the reader draws the envelope, not the ask: %q", said)
	}
}

// On the board a column says its HEAD once: the card's fallback present is
// the trail's HEAD row five rows down, and the card leaves it there (#107).
func TestTheBoardCardSaysHeadOnce(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 152, 40)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "pytest tests/gates") {
		t.Fatalf("porter's column draws no HEAD:\n%s", view)
	}
	n := 0
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(oneSpace(l), "test pytest tests/g") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("porter's column says HEAD %d times, want once:\n%s", n, view)
	}
}

// A column whose trail rows the reply box covers keeps its card's sentence:
// the board must not hide a stuck session's hung call behind a panel (#108, #107).
func TestTheCardKeepsWhatTheReplyPanelCovers(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneAlarmStorm(), 120, 34)
	pressKey(m, "r")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "reply to") {
		t.Fatalf("the reply panel is not up:\n%s", view)
	}
	if !strings.Contains(oneSpace(view), "silent 6m") {
		t.Errorf("the stuck column's hung call is nowhere on the board:\n%s", view)
	}
}

// Where the reader has the screen to itself, a relayed turn wears the word
// on its row: at 100 no trail stands beside it to say `◉ relayed` (#109).
func TestTheReaderMarksARelayedTurnOnItsRow(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 100, 30)
	for _, k := range []string{"1", "tab", "tab"} {
		pressKey(m, k)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "READER · porter") {
		t.Fatalf("not porter's reader:\n%s", view)
	}
	if !strings.Contains(view, "❯ relayed · the encoder is in") {
		t.Errorf("the relayed turn reads as the person's own:\n%s", view)
	}
	pressKey(m, "[")
	if strings.Contains(m.note, "relayed") {
		t.Errorf("the [ ] note quotes the mark with the turn: %q", m.note)
	}
}

// The reader's title keeps its clock alone where the page draws the
// anchored turn whole and no other turn to tell it from (#110, #65).
func TestTheReaderTitleLeavesTheTurnToThePage(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 80, 24)
	pressKey(m, "tab")
	pressKey(m, "tab")
	view := ansi.Strip(m.View())
	lines := strings.Split(view, "\n")
	title, turn := "", ""
	for _, l := range lines {
		if strings.Contains(l, "READER · hello") {
			title = l
		}
		if strings.HasPrefix(strings.TrimLeft(l, " "), "❯ add a --version flag") {
			turn = l
		}
	}
	if title == "" || turn == "" {
		t.Fatalf("not the reader on the first turn:\n%s", view)
	}
	if strings.Contains(title, "add a --version flag") {
		t.Errorf("the title says the turn the page draws two rows under it: %q over %q", title, turn)
	}
	if !strings.Contains(title, "17:59") {
		t.Errorf("the title lost its clock with the clause: %q", title)
	}
}

// A fleet of one at a hundred: the list row's present line goes where the
// trail beside it draws the same sentence on the same physical row, and
// the send trace moves up into the freed row (#111).
func TestTheRowLeavesThePresentToTheTrailBesideIt(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 100, 30)
	view := ansi.Strip(m.View())
	n := 0
	for _, l := range strings.Split(view, "\n") {
		n += strings.Count(oneSpace(l), "● scout thinking… for 40s")
	}
	if n != 1 {
		t.Errorf("the present is said %d times, want once:\n%s", n, view)
	}
	sc := sceneSecondDay()
	m = sceneModel(sc, 100, 30)
	for _, k := range []string{"r", "t", "go on", "enter"} {
		pressKey(m, k)
		poll(m, sc)
	}
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i, l := range lines {
		if strings.Contains(l, "▸1 ● hello") {
			if i+1 >= len(lines) || !strings.Contains(lines[i+1], "↪ sent") {
				t.Errorf("the trace did not move up into the freed row:\n%s", strings.Join(lines, "\n"))
			}
		}
	}
}

// The row the card gave up (#107) takes the rungs the tag row shed: the
// tool word and its model, the pane staying on the tag row (#112).
func TestTheFreedRowTakesTheModelTheTagShed(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 120, 34)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "opencode · sonnet-4-5") {
		t.Errorf("the api column's model stands on no row:\n%s", view)
	}
	if !strings.Contains(view, "⌁ dev:2.0") {
		t.Errorf("the pane left the tag row:\n%s", view)
	}
}

// The alarm leg's wrap budget is the span the row draws: at 220 the
// question fits whole beside "waiting 4m" and the options row is the
// options alone (#113).
func TestTheQuestionWrapsAgainstTheSpanItDraws(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 220, 48)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Open port 22 to the office CIDR? waiting 4m") {
		t.Errorf("the question wraps inside itself a cell short of fitting:\n%s", view)
	}
	if strings.Contains(view, "└ CIDR? [") {
		t.Errorf("the question's tail rides the options row:\n%s", view)
	}
}
