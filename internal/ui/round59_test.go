package ui

import (
	"regexp"
	"time"

	"github.com/deephanson94/compass/internal/transcript"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
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
	// The selected row sheds the tool word the identity header of the
	// same frame draws (#196's split, on the archived half): what this
	// test asserts is the verdict, which is unmoved.
	if !strings.Contains(view, "fix/api-timeouts · ✗ red") {
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
	if strings.Contains(title, "17:59") {
		t.Errorf("the title keeps a third copy of the turn's clock (#115): %q", title)
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

// #111 for any fleet: the selected row leaves the present to the trail
// beside it wherever the trail's drawn rows say the sentence — a fleet of
// four at a hundred repeated it byte for byte, a fleet of one did not (#114).
func TestTheSelectedRowLeavesThePresentToTheTrailInAnyFleet(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 100, 30)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "▸1 ● porter") {
		t.Fatalf("porter is not the selected row:\n%s", view)
	}
	n := 0
	for _, l := range strings.Split(view, "\n") {
		n += strings.Count(oneSpace(l), "● test pytest tests/gates for 6m")
	}
	if n != 1 {
		t.Errorf("the present is said %d times, want once:\n%s", n, view)
	}
}

// The card's compare sees the question the trail wrapped across its rows:
// the card leaves it to the trail, and the freed row takes the model (#116).
func TestTheCardLeavesAWrappedQuestionToTheTrail(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 120, 34)
	view := ansi.Strip(m.View())
	lines := strings.Split(view, "\n")
	if !strings.Contains(view, "design Open port 22 to") || !strings.Contains(view, "├ the office CIDR?") {
		t.Fatalf("the trail does not wrap the question:\n%s", view)
	}
	for i, l := range lines {
		if strings.Contains(l, "▸1 ▲ infra") && i+1 < len(lines) {
			if strings.Contains(strings.SplitN(lines[i+1], "│", 2)[0], "Open port 22") {
				t.Errorf("the card repeats a question the trail spells over its wrap: %q", lines[i+1])
			}
		}
	}
}

// The `↳ N new legs` digest yields to the model where the column's own
// divider draws the count (#117).
func TestTheNewLegsDigestYieldsToTheModel(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 120, 34)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "you were here") {
		t.Fatalf("no divider on the board:\n%s", view)
	}
	if !strings.Contains(view, "opus-4-1") {
		t.Errorf("the model stands on no row of the frame:\n%s", view)
	}
}

// At eighty the api row keeps its pane where the rung fits the row exactly:
// the digest's floor is its shortest clause (#118).
func TestTheRowKeepsThePaneThatFitsExactly(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 80, 24)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "claude · ⌁ :1.0") {
		t.Errorf("the pane is on no row for 3 api:\n%s", view)
	}
}

// A live session selected while the archive list is on screen keeps its
// present on the trail: the empty state is for a trail with nothing, not
// for a view (#119).
func TestTheArchiveViewKeepsALiveSessionsPresent(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 80, 24)
	for _, k := range []string{"/", "pytest", "enter", "A"} {
		pressKey(m, k)
	}
	view := ansi.Strip(m.View())
	if !m.archiveView {
		t.Fatalf("not the archive:\n%s", view)
	}
	if strings.Contains(view, "will appear here") {
		t.Errorf("the live session's trail draws the empty state in the archive view:\n%s", view)
	}
	if !strings.Contains(view, "thinking…") {
		t.Errorf("the live session's present is on no row:\n%s", view)
	}
}

// In the archive at eighty the reader's title names the session alone
// where the turn row draws the ask (#120).
func TestTheArchiveReaderTitleLeavesTheAskToTheTurnRow(t *testing.T) {
	forceASCII(t)
	sc := sceneSecondDay()
	m := sceneModel(sc, 80, 24)
	for _, k := range []string{"2", "tab", "tab", "["} { // the archive's reader, back to its one turn
		pressKey(m, k)
		poll(m, sc)
	}
	view := ansi.Strip(m.View())
	if !m.archiveView || !strings.Contains(view, "READER · ") {
		t.Fatalf("not the archive's reader:\n%s", view)
	}
	turn := ""
	for _, l := range strings.Split(view, "\n") {
		if strings.HasPrefix(strings.TrimLeft(l, " "), "❯ ") {
			turn = l
			break
		}
	}
	if turn == "" {
		t.Fatalf("no turn row on the page:\n%s", view)
	}
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "READER · ") && strings.Contains(l, "fix the 401") {
			t.Errorf("the title repeats the ask its turn row draws: %q over %q", l, turn)
		}
	}
}

// The header leaves two cells before its chips, their own separator's
// width, so the identity's last clause never abuts them (#121).
func TestTheHeaderLeavesTwoCellsBeforeTheChips(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 80, 24)
	keys := []string{"A"}
	for i := 0; i < 11; i++ {
		keys = append(keys, "j")
	}
	for _, k := range keys {
		pressKey(m, k)
	}
	head := strings.Split(ansi.Strip(m.View()), "\n")[0]
	if strings.Contains(head, "claude ●") || strings.Contains(head, "… ●") {
		t.Errorf("the identity ends one cell from the chips: %q", head)
	}
}

// Wider than a hundred the archive reader's title keeps the ask but not
// the cursor's clock where the turn row draws the ask with its own (#122).
func TestTheArchiveReaderTitleDropsTheCursorsClock(t *testing.T) {
	forceASCII(t)
	sc := sceneSecondDay()
	m := sceneModel(sc, 152, 40)
	for _, k := range []string{"2", "tab", "tab"} { // the corpus's own route: `[` parked the cursor where the clocks agree, and the pin passed on the revert
		pressKey(m, k)
		poll(m, sc)
	}
	view := ansi.Strip(m.View())
	title, turn := "", ""
	for _, l := range strings.Split(view, "\n") {
		for _, seg := range strings.Split(l, "│") {
			if strings.Contains(seg, "READER · ") {
				title = seg
			}
			if strings.Contains(seg, "❯ fix the 401") {
				turn = seg
			}
		}
	}
	if title == "" || turn == "" {
		t.Fatalf("not the archive's reader on its turn:\n%s", view)
	}
	if strings.Contains(title, ":") {
		t.Errorf("the title times the ask with the cursor's clock: %q over %q", title, turn)
	}
}

// The bare archive title runs the trail's own ladder: the day's long form
// where it fits, so fifty-five free cells do not hold a glyph the narrow
// legend never names (#123).
func TestTheBareArchiveTitleSaysTheDayInWords(t *testing.T) {
	forceASCII(t)
	sc := sceneSecondDay()
	m := sceneModel(sc, 80, 24)
	for _, k := range []string{"2", "tab", "tab", "["} { // api: three hours, one ship, one red
		pressKey(m, k)
		poll(m, sc)
	}
	view := ansi.Strip(m.View())
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "READER · ") {
			if strings.Contains(l, "⚑") || strings.Contains(l, "✗") {
				t.Errorf("the bare title says the day in glyphs with room to spare: %q", l)
			}
			if !strings.Contains(l, "1 ship · 1 red") {
				t.Errorf("the bare title does not say the day in words: %q\n%s", l, view)
			}
		}
	}
}

// The needs-you row leaves its question to the trail beside it: whole at a
// hundred, wrapped at eighty (#124, #111, #116).
func TestTheNeedsYouRowLeavesTheQuestionToTheTrail(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{100, 30}, {80, 24}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "design Open port 22 to") {
			t.Fatalf("at %dx%d the trail does not draw the question:\n%s", size[0], size[1], view)
		}
		lines := strings.Split(view, "\n")
		for i, l := range lines {
			if strings.Contains(l, "▸1 ▲ infra") && i+1 < len(lines) {
				if strings.Contains(strings.SplitN(lines[i+1], "│", 2)[0], "Open port 22") {
					t.Errorf("at %dx%d the row repeats the question the trail draws: %q", size[0], size[1], lines[i+1])
				}
			}
		}
	}
}

// The compare sees the row's sentence without the verdict it spliced in:
// at a hundred `● fix tokens.py 18✓ 2✗ · for 22m` is the trail's test row
// and its HEAD row, both on the frame (#125).
func TestTheRowLeavesTheVerdictSplicedPresentToTheTrail(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 100, 30)
	for _, k := range []string{"/", "pytest", "enter"} {
		pressKey(m, k)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "▸3 ● api") || !strings.Contains(view, "● fix    tokens.py") {
		t.Fatalf("not the search's api row beside its trail:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	for i, l := range lines {
		if strings.Contains(l, "▸3 ● api") && i+1 < len(lines) {
			if strings.Contains(strings.SplitN(lines[i+1], "│", 2)[0], "tokens.py") {
				t.Errorf("the row says the present the trail draws, verdict spliced in: %q", lines[i+1])
			}
		}
	}
}

// A column the reply box begins inside is composed at the width the box
// leaves: its rows keep their clocks, and no mark stands over blanks (#126).
func TestTheCoveredColumnKeepsItsClocks(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 120, 34)
	for _, k := range []string{"j", "r"} {
		pressKey(m, k)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "reply to 2") {
		t.Fatalf("the reply box is not up on api:\n%s", view)
	}
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "rewrite the install page") {
			left := strings.SplitN(l, "┌", 2)[0]
			if strings.Contains(left, "…") || !strings.Contains(left, "55m ago") {
				t.Errorf("the covered row lost its clock to a mark over blanks: %q", l)
			}
		}
	}
}

// At Lv2 the trail's cursor lands on HEAD and draws its mark between the
// glyph and the class — "●▸test". The mark is not part of the row's
// sentence, so #114's compare is not fooled by it (#127, #104).
func TestTheSelectedRowLeavesThePresentToTheTrailUnderTheCursor(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 100, 30)
	pressTab(m)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "●▸test") {
		t.Fatalf("the trail's cursor is not on HEAD:\n%s", view)
	}
	n := 0
	for _, l := range strings.Split(view, "\n") {
		n += strings.Count(oneSpace(strings.Replace(l, "▸", " ", 1)), "● test pytest tests/gates for 6m")
	}
	if n != 1 {
		t.Errorf("the present is said %d times, want once:\n%s", n, view)
	}
}

// The `[ ]` note counts and quotes the turn it landed on and leaves the
// clock to the row, which is always drawn with it (#128, #20).
func TestTheTurnNoteLeavesTheClockToTheRow(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 80, 24)
	for _, k := range []string{"tab", "tab", "["} {
		pressKey(m, k)
	}
	turn := ""
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.HasPrefix(strings.TrimLeft(l, " "), "❯ relayed") {
			turn = strings.TrimRight(l, " ")
		}
	}
	if !strings.HasSuffix(turn, "17:48") {
		t.Fatalf("the turn the note landed on is not drawn with its clock: %q", turn)
	}
	if !strings.HasPrefix(m.note, "❯ 1/1") {
		t.Fatalf("the note does not count the turn: %q", m.note)
	}
	if strings.Contains(m.note, "17:48") {
		t.Errorf("the note keeps a second copy of the row's clock: %q over %q", m.note, turn)
	}
}

// Where no rung stands above the pane, the `↳ N new legs` digest still
// yields to it: the divider below draws the count either way (#129, #117).
func TestTheNewLegsDigestYieldsToThePaneAlone(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneManyIdle(), 120, 34)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "you were here") {
		t.Fatalf("no divider on the board:\n%s", view)
	}
	for _, l := range strings.Split(view, "\n") {
		first := strings.SplitN(l, "│", 2)[0]
		if strings.Contains(first, "new legs") && strings.Contains(first, "⌁ work:0.0") {
			t.Errorf("the digest says the count the divider draws, over the pane: %q", first)
		}
	}
}

// The archive row's verdict is the band's ladder, not all or nothing: the
// counts, the clause's two words, last the mark alone — so a narrow archive
// says green or red where it cannot say how green (#130, #103, #47).
func TestTheArchiveRowSaysGreenOrRed(t *testing.T) {
	forceASCII(t)
	sc := sceneSecondDay()
	m := sceneModel(sc, 100, 30)
	pressKey(m, "A")
	poll(m, sc)
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i, l := range lines {
		col := strings.SplitN(l, "│", 2)[0]
		if !strings.Contains(col, " ○ ") || i+1 >= len(lines) {
			continue
		}
		tag := strings.SplitN(lines[i+1], "│", 2)[0]
		if strings.TrimSpace(tag) == "" || !strings.Contains(tag, " · ") && !strings.Contains(tag, "/") {
			continue
		}
		if !strings.ContainsAny(tag, "✓✗⚑") {
			t.Errorf("the archive row says nothing about green or red: %q under %q", strings.TrimRight(tag, " "), strings.TrimRight(col, " "))
		}
	}
}

// The trace note leaves the quote to the row that draws it and keeps the
// destination it alone carries; the keys the second copy cost come back (#131).
func TestTheTraceNoteLeavesTheQuoteToTheRow(t *testing.T) {
	forceASCII(t)
	sc := sceneSecondDay()
	m := sceneModel(sc, 80, 24)
	for _, k := range []string{"r", "1"} {
		pressKey(m, k)
		poll(m, sc)
	}
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	foot := lines[len(lines)-1]
	row := ""
	for _, l := range lines[:len(lines)-1] {
		for _, seg := range strings.Split(l, "│") {
			if strings.Contains(seg, "↪ sent") {
				row = strings.TrimSpace(seg)
			}
		}
	}
	if row == "" || !strings.Contains(foot, "↪ sent") {
		t.Fatalf("not the frame the line landed on:\n%s", strings.Join(lines, "\n"))
	}
	if strings.Contains(foot, `"please continue"`) {
		t.Errorf("the note repeats the quote the row draws: %q over %q", strings.TrimSpace(foot), row)
	}
	if !strings.Contains(foot, "to ⌁") {
		t.Errorf("the note gave up the destination it alone carries: %q", strings.TrimSpace(foot))
	}
	if !strings.Contains(foot, "tab deeper") {
		t.Errorf("the keys the second copy cost are still shed: %q", strings.TrimSpace(foot))
	}
}

// Both columns called api draw the divider with one leg under it, so both
// leave the count to it: the column #112 hoisted no less than the other (#132).
func TestTheHoistedColumnAlsoYieldsTheCount(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "you were here") {
			t.Fatalf("at %dx%d the board draws no divider:\n%s", size[0], size[1], view)
		}
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "↳ 1 new leg") {
				t.Errorf("at %dx%d the tag row draws the count its divider draws: %q", size[0], size[1], l)
			}
		}
	}
}

// On the reply frame the needs-you row leaves its question to the box,
// which says it under its own head one row up (#133).
func TestTheNeedsYouRowLeavesTheQuestionToTheReplyBox(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{100, 30}, {80, 24}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		pressKey(m, "r")
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "reply to 1") || strings.Count(view, "Open port 22") < 1 {
			t.Fatalf("at %dx%d not the reply frame on infra:\n%s", size[0], size[1], view)
		}
		lines := strings.Split(view, "\n")
		for i, l := range lines {
			if strings.Contains(l, "▸1 ▲ infra") && i+1 < len(lines) {
				if strings.Contains(strings.SplitN(lines[i+1], "│", 2)[0], "Open port 22") {
					t.Errorf("at %dx%d the row repeats the question the box says: %q", size[0], size[1], lines[i+1])
				}
			}
		}
	}
}

// A chapter note whose quote does not fit the room is the count alone: the
// turn it landed on is drawn four rows up, so a cut quote said less (#134).
func TestTheChapterNoteIsTheCountWhereTheQuoteWouldBeCut(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneAlarmStorm(), 80, 24) // infra's ask is too long for the footer's room
	for _, k := range []string{"tab", "tab", "["} {
		pressKey(m, k)
	}
	foot := strings.Split(strings.TrimRight(ansi.Strip(m.View()), "\n"), "\n")
	last := foot[len(foot)-1]
	if !strings.Contains(last, "❯ 1/1") {
		t.Fatalf("the footer does not count the turn: %q", last)
	}
	if strings.Contains(last, `"`) {
		t.Errorf("the footer spends the row on a quote of the turn drawn above: %q", last)
	}
}

// #129 yields only a digest that is the count. A digest whose count comes
// last — "↳ 3 sent since, none back · 1 new leg", the form at 152 where the
// look clause is shed — is not that digest: the divider draws neither of
// its clauses (#135, #38 at 220).
func TestTheDigestKeepsTheLanesItCountsAtOneFiftyTwo(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSubagents(), 152, 40)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "you were here · 2h ago") {
		t.Fatalf("no divider on the board:\n%s", view)
	}
	if !strings.Contains(view, "↳ 3 sent since, none back") {
		t.Errorf("the digest lost the lanes that never came back:\n%s", view)
	}
	if !strings.Contains(view, "↳ 1 back since, empty") {
		t.Errorf("the digest lost the lane that came back empty:\n%s", view)
	}
}

// The trailing look clause is the divider's own words (#85), so a digest of
// the count and the look age says nothing the divider does not (#136, #129).
func TestTheNewLegsDigestYieldsWithItsLookClause(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneManyIdle(), 220, 48)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "you were here · 1h ago") {
		t.Fatalf("no divider on the board:\n%s", view)
	}
	for _, l := range strings.Split(view, "\n") {
		for _, col := range strings.Split(l, "│") {
			if strings.Contains(col, "new legs · looked") && strings.Contains(col, "⌁ ") {
				t.Errorf("the digest says the count and the look the divider draws, over the pane: %q", strings.TrimSpace(col))
			}
		}
	}
}

// A folded list's slack — the rows an entry-whole window could not use —
// goes to the band, not to air over the archive's line (#137, #47, #92).
func TestTheFoldedListsSlackGoesToTheBand(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 80, 24)
	pressKey(m, "r")
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	last := -1
	for i, l := range rows {
		if strings.Contains(strings.SplitN(l, "│", 2)[0], "41 archived") {
			last = i
		}
	}
	if last < 2 {
		t.Fatalf("no archive line on the fleet column:\n%s", strings.Join(rows, "\n"))
	}
	air := 0
	for i := last - 1; i >= 0 && strings.TrimSpace(strings.SplitN(rows[i], "│", 2)[0]) == ""; i-- {
		air++
	}
	if air >= 2 {
		t.Errorf("%d blank fleet rows over %q, with sessions behind A", air, strings.TrimSpace(strings.SplitN(rows[last], "│", 2)[0]))
	}
}

// The trail's title leaves the span to its own first row: "· 3h" stood over
// "◉ … 3h ago" two rows under it, and the repeat pushed the day into glyphs
// where the words fit (#138).
func TestTheTrailTitleLeavesTheSpanToItsFirstRow(t *testing.T) {
	forceASCII(t)
	sc := sceneSecondDay()
	m := sceneModel(sc, 120, 34)
	pressKey(m, "2")
	poll(m, sc)
	view := ansi.Strip(m.View())
	title := ""
	for _, l := range strings.Split(view, "\n") {
		for _, seg := range strings.Split(l, "│") {
			if strings.Contains(seg, "TRAIL · api") {
				title = strings.TrimSpace(seg)
			}
		}
	}
	if title == "" || !strings.Contains(view, "3h ago") {
		t.Fatalf("not the archive's api trail with its first row drawn:\n%s", view)
	}
	if strings.Contains(title, "· 3h") {
		t.Errorf("the title repeats the span its first row draws: %q", title)
	}
	if !strings.Contains(title, "1 ship · 1 red") {
		t.Errorf("the title says the day in glyphs where the words fit: %q", title)
	}
}

// Past the reply box no lone clock stands with nothing left to time (#139, #56).
func TestTheBoxLeavesNoLoneClockBesideIt(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 152, 40)
	pressKey(m, "r")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "reply to 1") {
		t.Fatalf("the reply box is not up:\n%s", view)
	}
	for _, l := range strings.Split(view, "\n") {
		if i := strings.LastIndex(l, "┐"); i >= 0 {
			tail := strings.TrimSpace(strings.ReplaceAll(l[i+len("┐"):], "…", " "))
			if tail != "" && !strings.Contains(tail, " ") && strings.ContainsAny(tail, "0123456789") {
				t.Errorf("the box leaves a lone clock with nothing to time: %q past the box", tail)
			}
		}
	}
}

// The selected row leaves what the trail beside it draws to the trail —
// whatever its state: an idle session with no verdict fell back to its last
// leg's label, which is the trail's own row (#140, #111, #114).
func TestTheIdleSelectedRowLeavesItsLegToTheTrail(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneFleetHygiene(), 80, 24)
	pressKey(m, "j")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "▸3 ○ notebooks") || !strings.Contains(view, "◆ docs   eda.ipynb") {
		t.Fatalf("not the idle notebooks row beside its trail:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	for i, l := range lines {
		if strings.Contains(l, "▸3 ○ notebooks") && i+1 < len(lines) {
			if strings.Contains(strings.SplitN(lines[i+1], "│", 2)[0], "eda.ipynb") {
				t.Errorf("the idle row repeats the leg the trail draws beside it: %q", lines[i+1])
			}
		}
	}
}

// The selected row's count yields to the trail beside it, and the ladder
// takes the cells: the model is on the row (#141, #117, #129).
func TestTheRowsCountYieldsToTheTrailBesideIt(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{100, 30}, {80, 24}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		for _, k := range []string{"/", "pytest", "enter"} {
			pressKey(m, k)
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "▸3 ● api") || !strings.Contains(view, "you were here") {
			t.Fatalf("at %dx%d not the api row beside its divider:\n%s", size[0], size[1], view)
		}
		lines := strings.Split(view, "\n")
		for i, l := range lines {
			if !strings.Contains(l, "▸3 ● api") {
				continue
			}
			said := ""
			for k := i + 1; k < len(lines) && k <= i+2; k++ {
				said += strings.SplitN(lines[k], "│", 2)[0] + "\n"
			}
			if strings.Contains(said, "new leg") {
				t.Errorf("at %dx%d the row spends its cells on the count its divider draws: %q", size[0], size[1], said)
			}
			if !strings.Contains(said, "opus-4-1") {
				t.Errorf("at %dx%d the row of the two called api does not name its model: %q", size[0], size[1], said)
			}
		}
	}
}

// The look clause is the divider's own words whether or not what is left of
// the digest is the count alone (#142, #136, #85).
func TestTheDigestDropsTheLookItsDividerDrawsAnyway(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneManyIdle(), 220, 48)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "you were here · 1h ago") {
		t.Fatalf("no divider on the board:\n%s", view)
	}
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "· looked 1h ago") {
			t.Errorf("the digest says the look its own divider draws: %q", strings.TrimSpace(l))
			break
		}
	}
	if !strings.Contains(view, "↳ 1 red") || !strings.Contains(view, "↳ 1 ship") {
		t.Errorf("a clause the divider does not draw left the row:\n%s", view)
	}
	// And the selected list row at a hundred, the fold's other half (#151).
	n := sceneModel(sceneManyIdle(), 100, 30)
	pressKey(n, "j")
	list := ansi.Strip(n.View())
	if strings.Contains(list, "new leg · 1 red") {
		t.Errorf("the selected row says the count its divider draws beside its other clause:\n%s", list)
	}
}

// The count yields before the rungs are hoisted, so both columns called api
// name tool, model and pane on one row (#143, #132, #112).
func TestTheHoistLeavesTheIdentityOnOneRow(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "you were here") {
			t.Fatalf("at %dx%d the board draws no divider:\n%s", size[0], size[1], view)
		}
		together := false
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "opencode · sonnet-4-5 · ⌁ dev:2.0") && strings.Contains(l, "claude · opus-4-1 · ⌁ dev:1.0") {
				together = true
			}
		}
		if !together {
			t.Errorf("at %dx%d the two api columns do not name tool, model and pane on one row:\n%s", size[0], size[1], view)
		}
	}
}

// The session card's third row goes where it is only the count its own
// trail's read-line draws below it (#144, #129, #136).
func TestTheCardsCountYieldsToItsOwnTrail(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		for _, k := range []string{"3", "tab"} {
			pressKey(m, k)
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "[session]") || !strings.Contains(view, "you were here · 25m ago") {
			t.Fatalf("at %dx%d not the session card over its divider:\n%s", size[0], size[1], view)
		}
		for _, l := range strings.Split(view, "\n") {
			left := strings.SplitN(l, "│", 2)[0]
			if strings.TrimSpace(left) == "↳ 1 new leg · looked 25m ago" || strings.TrimSpace(left) == "↳ 1 new leg" {
				t.Errorf("at %dx%d the card spends a row on the count its own divider draws: %q", size[0], size[1], l)
			}
		}
	}
}

// Every archive row says green or red: where not even the mark fits behind
// the whole branch, the branch yields the cells — the band at the same
// width already says it (#145, #130, #47).
func TestEveryArchiveRowSaysGreenOrRed(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 80, 24)
	pressKey(m, "A")
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	for i, l := range rows {
		col := strings.SplitN(l, "│", 2)[0]
		if !strings.HasPrefix(col, "     claude · ") && !strings.HasPrefix(col, "     opencode · ") {
			continue
		}
		if !strings.ContainsAny(col, "✓✗⚑") {
			t.Errorf("row %d of the archive says nothing about green or red: %q", i, strings.TrimRight(col, " "))
		}
	}
}

// The hide refusal counts only the live: "the only session stays" stood
// over a band naming twelve more (#146, #10).
func TestTheHideRefusalCountsOnlyTheLive(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 80, 24)
	pressKey(m, "x")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "12 archived") {
		t.Fatalf("not the deck with its band up:\n%s", view)
	}
	if strings.Contains(view, "the only session stays") {
		t.Errorf("the refusal calls it the only session over a band that names twelve: %q", m.note)
	}
	if !strings.Contains(view, "the live one stays") {
		t.Errorf("the refusal does not scope its word to the live: %q", m.note)
	}
}

// The rows the board leaves blank belong to sessions, not to air (#43, #47):
// where every live session already has a column no further column can fill
// them, and the same band the list draws below the board's width takes
// them (#147).
func TestTheBoardsStrandedRowsTakeTheBand(t *testing.T) {
	forceASCII(t)
	list := ansi.Strip(sceneModel(sceneFleetHygiene(), 100, 30).View())
	if !strings.Contains(list, "5 ○ api") || !strings.Contains(list, "9 ○ notebooks") {
		t.Fatalf("the list at a hundred does not draw the band:\n%s", list)
	}
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		view := ansi.Strip(sceneModel(sceneFleetHygiene(), size[0], size[1]).View())
		if !strings.Contains(view, "archived · A browses") {
			t.Fatalf("at %dx%d not the board over its archive strip:\n%s", size[0], size[1], view)
		}
		if !strings.Contains(view, "5 ○ api") {
			t.Errorf("at %dx%d the board spends its blank rows on air and names none of the finished sessions the list names at a hundred:\n%s", size[0], size[1], view)
		}
	}
}

// A scrolled fleet column never opens on a row of air: the escape out of a
// cut entry is checked by the same guard that forbids it (#148).
func TestAScrolledColumnNeverOpensOnAir(t *testing.T) {
	forceASCII(t)
	sc := sceneFleetHygiene()
	m := sceneModel(sc, 80, 24)
	for _, k := range canonicalKeys[:25] { // the walkthrough to its scrolled list
		pressKey(m, k)
		poll(m, sc)
	}
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i, l := range lines {
		col := strings.SplitN(l, "│", 2)[0]
		if strings.Contains(col, "more above") && i+1 < len(lines) {
			if strings.TrimSpace(strings.SplitN(lines[i+1], "│", 2)[0]) == "" {
				t.Errorf("the scrolled column opens on air under %q", strings.TrimSpace(col))
			}
			return
		}
	}
	t.Fatalf("the list is not scrolled:\n%s", strings.Join(lines, "\n"))
}

// The note leaves the bytes to a row that says more: the row's right-aligned
// clause may be the tool word a two-tool fleet gives it, not a pane, and it
// may carry two trailing clauses — the compare takes both off (#149, #131).
func TestTheNoteLeavesTheBytesToARowThatSaysMore(t *testing.T) {
	forceASCII(t)
	for _, run := range []struct {
		sc   scene
		w, h int
	}{{sceneTwoTools(), 80, 24}, {sceneTwoTools(), 220, 48}, {sceneVeryLong(), 220, 48}} { // very-long's row carries two trailing clauses (#149)
		size := [2]int{run.w, run.h}
		m := sceneModel(run.sc, run.w, run.h)
		for _, k := range []string{"j", "r", "t", "go on", "enter"} { // api, a typed reply, sent
			pressKey(m, k)
		}
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		foot, body := "", false
		for i, l := range lines {
			if strings.Contains(l, "? help · q quit") {
				foot = l
				continue
			}
			if i < len(lines)-3 && strings.Contains(l, `sent "go on"`) {
				body = true
			}
		}
		if !body {
			t.Fatalf("at %dx%d no row draws the bytes:\n%s", size[0], size[1], strings.Join(lines, "\n"))
		}
		if strings.Contains(foot, `"go on"`) {
			t.Errorf("at %dx%d the note says the bytes a row above draws: %q", size[0], size[1], strings.TrimSpace(foot))
		}
	}
}

// The card's third row keeps its trace and leaves the count and the look
// to its own read-line six rows below (#150, #144, #142).
func TestTheCardsCountGoesBesideItsTrace(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		for _, k := range []string{"j", "r", "t", "go on", "enter", "tab"} { // api, a typed reply, sent, then its card
			pressKey(m, k)
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "sent \"go on\"") || !strings.Contains(view, "you were here · 25m ago") {
			t.Fatalf("at %dx%d not the sent card over its own divider:\n%s", size[0], size[1], view)
		}
		for _, l := range strings.Split(view, "\n") {
			left := strings.SplitN(l, "│", 2)[0]
			if !strings.Contains(left, "sent \"go on\"") {
				continue
			}
			if strings.Contains(left, "new leg") {
				t.Errorf("at %dx%d the card's trace row draws the count its own read-line draws: %q", size[0], size[1], strings.TrimSpace(left))
			}
			if strings.Contains(left, "looked 25m ago") {
				t.Errorf("at %dx%d the card's trace row draws the look its own read-line draws: %q", size[0], size[1], strings.TrimSpace(left))
			}
		}
	}
}

// On the board the count clause goes whether or not it stands alone — the
// divider's own words (#151, #129, #142); the clauses the divider does not
// draw stay, the `↳` moving on to them.
func TestTheBoardsCountGoesWithOtherClauses(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneManyIdle(), 220, 48)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "you were here") {
		t.Fatalf("no divider on the board:\n%s", view)
	}
	if strings.Contains(view, "new leg · 1 red") || strings.Contains(view, "new legs · 1 ship") {
		t.Errorf("a column's tag row says the count its divider draws beside its other clauses:\n%s", view)
	}
	if !strings.Contains(view, "↳ 1 red") || !strings.Contains(view, "↳ 1 ship") {
		t.Errorf("a clause the divider does not draw left the row:\n%s", view)
	}
}

// Every refusal that names the fleet of one counts only the live: the band
// two rows under the note numbers twelve more (#152, #146, #10).
func TestEveryRefusalCountsOnlyTheLive(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 80, 24)
	pressKey(m, "shift+tab")
	if strings.Contains(m.note, "the only session") || !strings.Contains(m.note, "nothing to zoom out to") {
		t.Errorf("the zoom-out refusal calls it the only session: %q", m.note)
	}
	m = sceneModel(sceneSecondDay(), 80, 24)
	pressKey(m, "j")
	if strings.Contains(m.note, "the only session") {
		t.Errorf("the move refusal calls it the only session over a band naming twelve: %q", m.note)
	}
	m = sceneModel(sceneSecondDay(), 80, 24)
	pressKey(m, "A")
	pressKey(m, "shift+tab")
	if strings.Contains(m.note, "the only live one") {
		t.Errorf("in the archive the refusal scoped a word it need not: %q", m.note)
	}
}

// A trail of one prompt and no leg draws two glyph rows — the prompt and
// HEAD — so the refusal says what is true of it: no leg to move to (#153).
func TestTheOneRowTrailsRefusalNamesTheLeg(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 80, 24)
	pressTab(m)
	pressKey(m, "ctrl+u")
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "◉▸\"add a --version flag\"") || !strings.Contains(view, "● scout  thinking…") {
		t.Fatalf("not the two-row trail with the cursor on its prompt:\n%s", view)
	}
	if m.note == "the trail is one row" || !strings.Contains(m.note, "no leg") {
		t.Errorf("the refusal counts a row the panel does not draw as one: %q", m.note)
	}
}

// The board's band is a column too: where the reply box begins inside it,
// it is composed at the width the box leaves, so no row is split by the box
// and no verdict stands past it with no session (#154, #126, #147).
func TestTheBoardsBandIsComposedAtTheWidthTheBoxLeaves(t *testing.T) {
	forceASCII(t)
	peek := regexp.MustCompile(`[│└][^│└]*[│┘]\s+…\s+(?:[✓✗]|build)`)
	band := regexp.MustCompile(`^\s+[1-9] ○ [a-z-]+ · "`)
	clock := regexp.MustCompile(`\d+[smhd]\s*$`)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneFleetHygiene(), size[0], size[1])
		for _, k := range []string{"j", "r"} { // the second harness, its reply box
			pressKey(m, k)
		}
		for i, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			if peek.MatchString(l) {
				t.Errorf("at %dx%d row %d says a verdict past the reply box and no session: %q", size[0], size[1], i+1, strings.TrimSpace(l))
			}
			left := strings.SplitN(strings.SplitN(l, "│", 2)[0], "└", 2)[0]
			if band.MatchString(left) && !clock.MatchString(strings.TrimRight(left, " ")) {
				t.Errorf("at %dx%d the band row beside the box keeps neither verdict nor clock: %q", size[0], size[1], strings.TrimRight(left, " "))
			}
		}
	}
}

// The session card's tag row goes where every clause of it stands in the
// header two rows up; the trail and the reader each take a cell (#155, #100).
func TestTheCardsTagLeavesItsWordsToTheHeader(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		for _, k := range []string{"j", "tab"} { // api, then its card
			pressKey(m, k)
		}
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		if len(lines) < 8 || !strings.Contains(lines[0], "opencode") {
			t.Fatalf("at %dx%d not the api card:\n%s", size[0], size[1], strings.Join(lines, "\n"))
		}
		head := lines[0]
		for _, l := range lines[3:8] {
			row := strings.TrimSpace(strings.SplitN(l, "│", 2)[0])
			if row == "" || strings.Contains(row, "  ") || !strings.Contains(row, mirrorMark) {
				continue
			}
			said := true
			for _, c := range strings.Split(row, " · ") {
				if !strings.Contains(head, c) {
					said = false
					break
				}
			}
			if said {
				t.Errorf("at %dx%d the card's tag row says only what the header says: %q", size[0], size[1], row)
			}
		}
	}
}

// The hide refusal keeps the way in: `the live one stays` leaves the
// eighty-column footer its `tab deeper` (#156, #146).
func TestTheHideRefusalKeepsTheWayIn(t *testing.T) {
	forceASCII(t)
	for _, sc := range []struct {
		name string
		m    func() *Model
	}{
		{"second-day", func() *Model { return sceneModel(sceneSecondDay(), 80, 24) }},
		{"first-session", func() *Model { return sceneModel(sceneFirstSession(), 80, 24) }},
	} {
		m := sc.m()
		pressKey(m, "x")
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "live") || !strings.Contains(view, "stays") {
			t.Fatalf("%s: not the refusal frame:\n%s", sc.name, view)
		}
		foot := ""
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "? help · q quit") {
				foot = l
			}
		}
		if !strings.Contains(foot, "tab deeper") {
			t.Errorf("%s: the refusal note cost the footer the way in: %q", sc.name, strings.TrimSpace(foot))
		}
	}
}

// A hidden session does not cost the board its band: the hidden count is
// the band's own header's clause (#157, #147, #86).
func TestAHiddenSessionDoesNotCostTheBoardItsBand(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneFleetHygiene(), size[0], size[1])
		pressKey(m, "x")
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "hidden") {
			t.Fatalf("at %dx%d nothing was hidden:\n%s", size[0], size[1], view)
		}
		for _, want := range []string{"5 ○ api", "9 ○ notebooks"} {
			if !strings.Contains(view, want) {
				t.Errorf("at %dx%d a hide sent the board's rows back to air: %q is on no row:\n%s", size[0], size[1], want, view)
			}
		}
	}
}

// The sliver a covering reply box leaves of a board column is not a lone
// clock: the peek's first segment is tested on its own (#158, #139).
func TestACoveredColumnsSliverIsNotALoneClock(t *testing.T) {
	forceASCII(t)
	lone := regexp.MustCompile(`[│┘└]\s*…\s*(\d+[smhd]|[a-z]{2,5})\s*│`)
	for _, size := range [][2]int{{120, 34}, {220, 48}} {
		m := sceneModel(sceneManyIdle(), size[0], size[1])
		pressKey(m, "r")
		for i, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			if g := lone.FindStringSubmatch(l); g != nil {
				t.Errorf("at %dx%d row %d: the covered column's sliver says %q and nothing it belongs to: %q", size[0], size[1], i+1, g[1], strings.TrimSpace(l))
			}
		}
	}
}

// The zoom-out refusal keeps the way in: `nothing to zoom out to` leaves
// the eighty-column footer `enter attach` and `tab deeper` (#159, #152, #156).
func TestTheZoomOutRefusalKeepsTheWayIn(t *testing.T) {
	forceASCII(t)
	for _, sc := range []struct {
		name string
		m    func() *Model
	}{
		{"second-day", func() *Model { return sceneModel(sceneSecondDay(), 80, 24) }},
		{"first-session", func() *Model { return sceneModel(sceneFirstSession(), 80, 24) }},
	} {
		m := sc.m()
		pressKey(m, "shift+tab")
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "nothing to zoom out to") {
			t.Fatalf("%s: not the refusal frame:\n%s", sc.name, view)
		}
		foot := ""
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "? help · q quit") {
				foot = l
			}
		}
		if !strings.Contains(foot, "enter attach") || !strings.Contains(foot, "tab deeper") {
			t.Errorf("%s: the refusal cost the footer the way in: %q", sc.name, strings.TrimSpace(foot))
		}
	}
}

// A navigator stands to the left of what it navigates (#19): where the
// middle of the three-column deck is the reader, the trail stands between
// the fleet and it, and the tab to Lv3 keeps the trail on the reader's
// left (#160, #46).
func TestTheTrailStandsLeftOfTheReaderItNavigates(t *testing.T) {
	forceASCII(t)
	sc := sceneSecondDay()
	m := sceneModel(sc, 120, 34)
	for _, k := range []string{"2", "tab"} {
		pressKey(m, k)
		poll(m, sc)
	}
	head := strings.Split(ansi.Strip(m.View()), "\n")[3]
	if !strings.Contains(head, "READER") || !strings.Contains(head, "TRAIL") {
		t.Fatalf("not the three-column deck with the reader in it: %q", head)
	}
	if strings.Index(head, "TRAIL") > strings.Index(head, "READER") {
		t.Errorf("the trail stands right of the reader it navigates: %q", head)
	}
	pressKey(m, "tab")
	poll(m, sc)
	head = strings.Split(ansi.Strip(m.View()), "\n")[3]
	if strings.Index(head, "TRAIL") > strings.Index(head, "READER") {
		t.Errorf("at Lv3 the trail crossed to the reader's right: %q", head)
	}
}

// An archived row says whether it shipped: the branch yields its tail for
// a word the mark cannot say, since inside the archive the band is off the
// screen and the row is the verdict's only place (#161, #145, #47).
func TestTheArchiveRowSaysWhetherItShipped(t *testing.T) {
	forceASCII(t)
	for _, w := range []int{80, 100} {
		m := sceneModel(sceneSecondDay(), w, 30)
		pressKey(m, "A")
		view := ansi.Strip(m.View())
		var row string
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "opencode · spi") {
				row = strings.TrimSpace(l)
			}
		}
		if row == "" {
			t.Fatalf("%d: no billing row in the archive:\n%s", w, view)
		}
		if !strings.Contains(row, "shipped") {
			t.Errorf("%d: the archived row says a tick where the band says it shipped: %q", w, row)
		}
	}
}

// The chapter keys' refusal names the prompt they move by, not a leg
// (#162, #153).
func TestTheChapterKeysRefusalNamesThePrompt(t *testing.T) {
	forceASCII(t)
	for _, tc := range []struct {
		key  string
		want string
	}{{"[", "no earlier prompt"}, {"]", "no later prompt"}} {
		m := sceneModel(sceneSecondDay(), 80, 24)
		pressTab(m)
		pressKey(m, tc.key)
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "[ ] chapters") {
			t.Fatalf("%s: not the Lv2 footer that glosses the chapter keys:\n%s", tc.key, view)
		}
		if m.note != tc.want {
			t.Errorf("%s: the chapter key's refusal names a leg it never moves by: %q, want %q", tc.key, m.note, tc.want)
		}
	}
}

// A search the live fleet does not answer keeps the board's band: the
// finished sessions that answer it are named under a note that says the
// miss is the live board's (#163, #98, #147).
func TestTheBoardsMissKeepsTheBand(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneFleetHygiene(), size[0], size[1])
		for _, k := range []string{"/", "reconcile"} {
			pressKey(m, k)
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "/reconcile") {
			t.Fatalf("at %dx%d not the search frame:\n%s", size[0], size[1], view)
		}
		if !strings.Contains(view, "no live session matches /reconcile") {
			t.Errorf("at %dx%d the miss is the live board's and does not say so:\n%s", size[0], size[1], view)
		}
		if !strings.Contains(view, `"reconcile the state file"`) {
			t.Errorf("at %dx%d the board blanks on a search eight finished sessions answer:\n%s", size[0], size[1], view)
		}
	}
}

// The band narrowed by a search counts what the search left, in the
// header's own form (#164, #98).
func TestTheBandsHeaderCountsWhatTheSearchLeft(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneFleetHygiene(), size[0], size[1])
		for _, k := range []string{"/", "pytest", "enter"} {
			pressKey(m, k)
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "1 of 41 archived") {
			t.Errorf("at %dx%d the band's header counts the whole archive under a search:\n%s", size[0], size[1], view)
		}
	}
}

// The no-pane refusal names the key and keeps the way in: `reply needs a
// pane` leaves the eighty-column footer its `tab deeper`, and does not say
// `no pane` twice on one row (#165, #156, #159).
func TestTheNoPaneRefusalKeepsTheWayIn(t *testing.T) {
	forceASCII(t)
	for _, key := range []string{"r", "enter"} {
		m := sceneModel(sceneFleetHygiene(), 80, 24)
		pressKey(m, "3") // notebooks, the session with no pane
		pressKey(m, key)
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "needs a pane") {
			t.Fatalf("%s: not the no-pane refusal:\n%s", key, view)
		}
		foot := ""
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "? help · q quit") {
				foot = l
			}
		}
		if !strings.Contains(foot, "tab deeper") {
			t.Errorf("%s: the refusal cost the footer the way in: %q", key, strings.TrimSpace(foot))
		}
		if strings.Count(foot, "no pane") > 1 {
			t.Errorf("%s: the footer says no pane twice: %q", key, strings.TrimSpace(foot))
		}
	}
}

// The `]` refusal keeps the keys the `[` refusal beside it keeps: the help
// names G at every width, and the 34-cell clause cost a key (#166, #162).
func TestTheLaterChapterRefusalKeepsTheKeys(t *testing.T) {
	forceASCII(t)
	foot := func(view string) string {
		out := ""
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "? help · q quit") {
				out = l
			}
		}
		return out
	}
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		for _, k := range []string{"tab", "ctrl+u", "ctrl+u", "["} {
			pressKey(m, k)
		}
		before := foot(ansi.Strip(m.View()))
		if !strings.Contains(before, "no earlier prompt") {
			t.Fatalf("%dx%d: not the earlier refusal:\n%s", size[0], size[1], ansi.Strip(m.View()))
		}
		pressKey(m, "]")
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "no later prompt") {
			t.Fatalf("%dx%d: not the refusal frame:\n%s", size[0], size[1], view)
		}
		after := foot(view)
		for _, k := range []string{"j/k", "r reply", "a ask", "enter attach", "m live pane", "tab "} {
			if strings.Contains(before, k) && !strings.Contains(after, k) {
				t.Errorf("%dx%d: the ] refusal sheds %q where the [ refusal beside it keeps it:\n  before %q\n  after  %q", size[0], size[1], k, strings.TrimSpace(before), strings.TrimSpace(after))
			}
		}
	}
}

// #159 on the wide deck too: the zoom-out refusal from a board that fits
// keeps `enter attach` and `a ask` (#159).
func TestTheZoomOutRefusalKeepsTheWayInOnTheWideDeck(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneSecondDay(), size[0], size[1])
		for _, k := range []string{"tab", "tab", "shift+tab", "shift+tab", "shift+tab"} {
			pressKey(m, k)
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "nothing to zoom out to") {
			t.Fatalf("%dx%d: not the refusal frame:\n%s", size[0], size[1], view)
		}
		foot := ""
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "? help · q quit") {
				foot = l
			}
		}
		for _, k := range []string{"enter attach", "a ask"} {
			if !strings.Contains(foot, k) {
				t.Errorf("%dx%d: the refusal cost the footer %q: %q", size[0], size[1], k, strings.TrimSpace(foot))
			}
		}
	}
}

// The card's tag goes even beside another clause: a row that carries a
// trace and the tag the header says keeps the trace alone (#167, #155).
func TestTheCardsTagGoesEvenBesideAnotherClause(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		for _, k := range []string{"3", "tab"} {
			pressKey(m, k)
		}
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		head := lines[0]
		for i, l := range lines {
			if !strings.Contains(l, "[session]") || i+1 >= len(lines) {
				continue
			}
			left := strings.TrimRight(strings.Split(lines[i+1], "│")[0], " ")
			j := strings.LastIndex(left, "  ")
			if j <= 0 {
				continue
			}
			tag := strings.TrimSpace(left[j:])
			if tag == "" || !strings.Contains(tag, " · ") {
				continue
			}
			all := true
			for _, c := range strings.Split(tag, " · ") {
				if c == "" || !strings.Contains(head, c) {
					all = false
				}
			}
			if all {
				t.Errorf("%dx%d: the card's tag %q repeats the header %q", size[0], size[1], tag, strings.TrimSpace(head))
			}
		}
	}
}

// Under a search that leaves no finished session the archive's door counts
// what the search left, as the band's header does (#168, #164).
func TestTheArchiveDoorCountsWhatTheSearchLeft(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 80, 24)
	for _, k := range []string{"/", "pytest", "enter"} {
		pressKey(m, k)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "/pytest") {
		t.Fatalf("not the search frame:\n%s", view)
	}
	if !strings.Contains(view, "0 of 12 archived") {
		t.Errorf("the archive's door counts the whole archive under a search it did not answer:\n%s", view)
	}
}

// ---- round 69, fleet-hygiene ----
// A note whose quote the row clipped still leaves the quote to the row.
// The compare was made against the whole note, whose destination clause no
// row of the frame ever carries, so a clipped copy of the quote defeated it
// and the longer note took the eighty-column footer's way in and way deeper
// (#131's own doc: the row keeps the quote "whole or clipped").
func TestTheClippedRowStillTakesTheNotesQuote(t *testing.T) {
	for _, tc := range []struct {
		name string
		sc   scene
		w, h int
	}{
		{"many-idle", sceneManyIdle(), 80, 24},
		{"very-long", sceneVeryLong(), 80, 24},
		{"many-idle", sceneManyIdle(), 152, 40},
	} {
		m := sceneModel(tc.sc, tc.w, tc.h)
		pressKey(m, "r")
		pressKey(m, "1")
		view := ansi.Strip(m.View())
		foot := ""
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "? help · q quit") {
				foot = l
			}
		}
		if !strings.Contains(foot, "↪ sent") {
			t.Fatalf("%s at %dx%d: no send note on the footer: %q", tc.name, tc.w, tc.h, strings.TrimSpace(foot))
		}
		if strings.Contains(foot, `"please continue"`) {
			t.Errorf("%s at %dx%d: the note says the quote a row of this frame draws: %q",
				tc.name, tc.w, tc.h, strings.TrimSpace(foot))
		}
		want := []string{"enter attach", "tab deeper"}
		if tc.w > 110 {
			want = []string{"enter attach (prefix d returns)", "tab session"}
		}
		for _, key := range want {
			if !strings.Contains(foot, key) {
				t.Errorf("%s at %dx%d: the note cost the footer %q: %q",
					tc.name, tc.w, tc.h, key, strings.TrimSpace(foot))
			}
		}
	}
}

// ---- round 69, fleet-hygiene ----
// No row of the band wears a bare mark beside a row that wears a word.
// The mark cannot say "shipped" — ✓ is the tick a merely green row wears —
// so the one row with that news stood wordless between two reading
// "✓ green" (#161, on the band; #57's device, band-wide).
func TestTheBandsShippedRowKeepsItsWord(t *testing.T) {
	row := regexp.MustCompile(`^\s*[1-9] ○ [a-z0-9-]+ · .*?\s\s+(\S.*?)\s\d+[smhd]$`)
	m := sceneModel(sceneFleetHygiene(), 152, 40)
	pressKey(m, "j")
	pressKey(m, "r") // the reply box; the band is composed at the width it leaves
	var bare, worded []string
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		for _, seg := range strings.Split(l, "│") {
			g := row.FindStringSubmatch(strings.TrimRight(seg, " "))
			if g == nil {
				continue
			}
			if v := g[1]; v == "✓" || v == "✗" || v == "⚑" {
				bare = append(bare, strings.TrimSpace(seg))
			} else if strings.ContainsAny(v[:3], "✓✗⚑") && len(strings.Fields(v)) > 1 {
				worded = append(worded, strings.TrimSpace(seg))
			}
		}
	}
	if len(worded) == 0 {
		t.Fatalf("no worded band row on the frame:\n%s", ansi.Strip(m.View()))
	}
	for _, b := range bare {
		t.Errorf("the band draws a bare mark beside %d worded rows: %q (worded: %q)", len(worded), b, worded[0])
	}
}

// ---- round 69, fleet-hygiene ----
// The archive door counts what the search left wherever it is drawn.
// #168 gave the list's last line the count; the board's strip and a fleet
// of one's own door kept the whole archive, so at eighty the same fleet
// under the same search read "0 of 300 archived · A browses" and twenty
// columns wider "300 archived · A browses".
func TestTheArchiveDoorCountsTheSearchAtEveryWidth(t *testing.T) {
	door := regexp.MustCompile(`(\d+(?: of \d+)?) archived · A`)
	for _, tc := range []struct {
		name string
		sc   scene
		w, h int
	}{
		{"many-idle", sceneManyIdle(), 120, 34},
		{"many-idle", sceneManyIdle(), 152, 40},
		{"many-idle", sceneManyIdle(), 220, 48},
		{"second-day", sceneSecondDay(), 120, 34},
		{"few-ongoing", sceneFewOngoing(), 220, 48},
	} {
		m := sceneModel(tc.sc, tc.w, tc.h)
		pressKey(m, "/")
		for _, r := range "pytest" {
			pressKey(m, string(r))
		}
		pressKey(m, "enter")
		view := ansi.Strip(m.View())
		g := door.FindStringSubmatch(view)
		if g == nil {
			t.Fatalf("%s at %dx%d: no archive door on the frame:\n%s", tc.name, tc.w, tc.h, view)
		}
		if !strings.Contains(g[1], " of ") {
			t.Errorf("%s at %dx%d: the door counts the whole archive under a search: %q",
				tc.name, tc.w, tc.h, g[0])
		}
	}
}

// ---- round 70, second-day ----
// The archive row that shipped keeps its word beside a worded neighbour.
// On the three-column archive deck at 120 the fleet column is 26 cells of
// content — the narrowest anywhere — and #161's four-cell branch floor is
// two cells out of reach for a row whose tool word is `opencode`. So
// `opencode · spike/bill… · ✓` stood two rows under
// `claude · chor… · ✓ shipped`, wearing the same bare tick the green etl
// rows wear, on the one frame where the band that says `✓ shipped` is off
// the screen (#161, #171).
func TestTheArchiveRowThatShippedKeepsItsWordAt120(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneSecondDay(), 120, 34)
	for _, k := range []string{"A", "tab"} {
		pressKey(m, k)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "FLEET · archive") {
		t.Fatalf("not the archive frame:\n%s", view)
	}
	row := regexp.MustCompile(`(?m)^\s*opencode · \S+ · [^\n│]*`)
	g := row.FindString(view)
	if g == "" {
		t.Fatalf("no opencode archive row on the frame:\n%s", view)
	}
	worded := strings.Contains(view, "· ✓ shipped")
	if worded && !strings.Contains(g, "shipped") {
		t.Errorf("the archive row that shipped wears a bare tick beside a worded neighbour: %q", strings.TrimSpace(g))
	}
}

// ---- round 70, two-tools ----
// The way back is taught on the row that has room for it: where the
// strip's hidden count is drawn, the footer's hide note leaves
// `A, then x` to that row and keeps its keys (#64 the other way round,
// #57's measurement).
func TestTheWayBackIsTaughtOnTheRowWithRoom(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		for _, k := range []string{"j", "x"} {
			pressKey(m, k)
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "is hidden") {
			t.Fatalf("%dx%d: not the hide frame:\n%s", size[0], size[1], view)
		}
		lines := strings.Split(view, "\n")
		foot, strip := "", ""
		for _, l := range lines {
			if strings.Contains(l, "? help · q quit") {
				foot = l
			} else if strings.Contains(l, " hidden") && !strings.Contains(l, "is hidden") {
				strip = l
			}
		}
		if strip == "" {
			continue // no drawn row carries the count: the note keeps the way back
		}
		if strings.Contains(foot, "A, then x") {
			t.Errorf("%dx%d: the footer teaches the way back while %q has room for it:\n  %q",
				size[0], size[1], strings.TrimSpace(strip), strings.TrimSpace(foot))
		}
		if strings.Count(view, "A, then x") != 1 {
			t.Errorf("%dx%d: the way back is taught %d times", size[0], size[1], strings.Count(view, "A, then x"))
		}
	}
}

// The trail's top refusal costs the footer no key its neighbours keep:
// `at the start` is the floor a note is measured against, so the frame
// after ctrl+u names every key the frame before it named, less the one
// the note's own cells buy (#166, #159, #156's shape).
func TestTheTopOfTrailRefusalKeepsTheChapterKeys(t *testing.T) {
	forceASCII(t)
	// At eighty the key the note's cells buy is `tab deeper`: the trail's
	// own keys stand one cell over what the twelve-cell floor leaves, so
	// the chapter key yields to the way in there and the row keeps the
	// same count of keys (round 77).
	want := map[int]string{80: "tab deeper", 100: "r reply", 120: "a ask", 152: "m live pane"}
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		for _, k := range []string{"tab", "ctrl+u", "ctrl+u"} {
			pressKey(m, k)
		}
		view := ansi.Strip(m.View())
		foot := ""
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "? help · q quit") {
				foot = l
			}
		}
		if !strings.Contains(foot, "at the start") {
			t.Fatalf("%dx%d: not the top-of-trail refusal: %q", size[0], size[1], strings.TrimSpace(foot))
		}
		if k := want[size[0]]; !strings.Contains(foot, k) {
			t.Errorf("%dx%d: the top-of-trail refusal sheds %q: %q", size[0], size[1], k, strings.TrimSpace(foot))
		}
	}
}

// ---- round 70, fleet-hygiene ----
// The hide note does not cost the eighty-column footer the way deeper,
// on the fleets whose strip draws the hidden count beside the archive door.
// `3 notebooks is hidden · A, then x` is 33 cells; beside it the footer
// sheds `tab deeper`, the frame's only naming of the way deeper — the
// harm #57 named in the same round it wrote this note, and #156, #159,
// #165 and #166 have each folded since. The way back is the strip's own
// clause, and the strip takes it up again when the note lets go (#64).
func TestTheHideNoteKeepsTheWayDeeperBesideTheArchiveDoor(t *testing.T) {
	for _, tc := range []struct {
		name string
		sc   scene
		sess string
	}{
		{"fleet-hygiene", sceneFleetHygiene(), "notebooks"},
		{"many-idle", sceneManyIdle(), "webapp"},
		{"few-ongoing", sceneFewOngoing(), "billing"},
	} {
		m := sceneModel(tc.sc, 80, 24)
		hid := ""
		for _, s := range m.sessions {
			if s.Live && sessionName(s.Info) == tc.sess {
				hid = s.Info.Key()
			}
		}
		if hid == "" {
			t.Fatalf("%s: no live session named %s", tc.name, tc.sess)
		}
		m.point(hid)
		pressKey(m, "x")
		view := ansi.Strip(m.View())
		foot := ""
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "? help · q quit") {
				foot = l
			}
		}
		if !strings.Contains(foot, "is hidden") {
			t.Fatalf("%s: no hide note on the footer: %q", tc.name, strings.TrimSpace(foot))
		}
		if !strings.Contains(foot, "tab deeper") {
			t.Errorf("%s at 80x24: the hide note cost the footer the way deeper: %q",
				tc.name, strings.TrimSpace(foot))
		}
		if !strings.Contains(view, "A, then x") && !strings.Contains(view, "hidden · A") {
			t.Errorf("%s at 80x24: no row of the frame names the way to the hidden session:\n%s", tc.name, view)
		}
	}
}

// ---- round 70, fleet-hygiene ----
// The hidden count on the strip counts what the search left, as the
// archive door beside it does (#164, #168, #169). Under `/pytest` the
// strip said `1 of 41 archived · 1 hidden · A browses` while `A` on that
// search listed one archived row and no hidden group at all: the hidden
// session is a docs session the query does not match.
func TestTheHiddenCountCountsTheSearchToo(t *testing.T) {
	door := regexp.MustCompile(`(\d+(?: of \d+)?) hidden`)
	for _, w := range []int{100, 120, 152, 220} {
		m := sceneModel(sceneFleetHygiene(), w, 34)
		for _, s := range m.sessions {
			if s.Live && sessionName(s.Info) == "notebooks" {
				m.point(s.Info.Key())
			}
		}
		pressKey(m, "x") // the hidden one is the session /pytest does not match
		pressKey(m, "/")
		for _, r := range "pytest" {
			pressKey(m, string(r))
		}
		pressKey(m, "enter")
		view := ansi.Strip(m.View())
		g := door.FindStringSubmatch(view)
		if g == nil {
			t.Fatalf("at %d: no hidden count on the frame:\n%s", w, view)
		}
		if !strings.Contains(g[1], " of ") {
			t.Errorf("at %d: the strip counts every hidden session under a search: %q", w, g[0])
		}
	}
}

// ---- round 71, two-tools ----
// The board's `m` note says the flag flipped and nothing the frame already
// says. `mirror on · beside a session (tab)` is 34 cells against the twelve
// -cell floor: `beside a session` is the help's own `m` row and `(tab)` is
// the very footer's `tab session`, so every cell over the floor is a second
// copy — and it cost the 120 footer `/ search` and `g grab` and the 152
// footer `enter attach (prefix d returns)` (#166, #57, #165).
func TestTheBoardsMirrorNoteKeepsTheBoardsKeys(t *testing.T) {
	forceASCII(t)
	want := map[int][]string{
		120: {"/ search", "g grab"},
		152: {"(prefix d returns)"},
	}
	for _, size := range [][2]int{{120, 34}, {152, 40}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		pressKey(m, "m")
		view := ansi.Strip(m.View())
		foot := ""
		for _, l := range strings.Split(view, "\n") {
			if strings.Contains(l, "? help · q quit") {
				foot = l
			}
		}
		if !strings.Contains(foot, "mirror on") {
			t.Fatalf("%dx%d: not the board's mirror note: %q", size[0], size[1], strings.TrimSpace(foot))
		}
		if !strings.Contains(foot, "tab session") {
			t.Errorf("%dx%d: the note's own row does not name the way to the mirror: %q",
				size[0], size[1], strings.TrimSpace(foot))
		}
		for _, k := range want[size[0]] {
			if !strings.Contains(foot, k) {
				t.Errorf("%dx%d: the board's mirror note sheds %q: %q",
					size[0], size[1], k, strings.TrimSpace(foot))
			}
		}
	}
}

// ---- round 71, fleet-hygiene ----
// The archive's own header counts what its list holds. Under a search the
// chip said `archive 41 · 1 hidden` over one drawn row, no hidden group and
// no `x unhide` — the fourth site #176 did not reach — and the identity's
// clause counted the drawn rows, hidden ones included, against the archived
// total: `1 of 41` over a list with no archived row (#178, #169, #176).
func TestTheArchiveHeaderCountsWhatItsListHolds(t *testing.T) {
	for _, tc := range []struct {
		query, chip string
	}{
		{"pytest", "archive 1 of 41 · 0 of 1 hidden"},
		{"eda", "archive 0 of 41 · 1 of 1 hidden"},
	} {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			m := sceneModel(sceneFleetHygiene(), size[0], size[1])
			for _, s := range m.sessions {
				if s.Live && sessionName(s.Info) == "notebooks" {
					m.point(s.Info.Key())
				}
			}
			pressKey(m, "x") // the hidden one is the eda session, which /pytest does not match
			pressKey(m, "/")
			for _, r := range tc.query {
				pressKey(m, string(r))
			}
			pressKey(m, "enter")
			pressKey(m, "A")
			head := ansi.Strip(strings.SplitN(m.View(), "\n", 2)[0])
			if !strings.Contains(head, tc.chip) {
				t.Errorf("/%s at %dx%d: the archive's header does not count what its list holds: %q", tc.query, size[0], size[1], strings.TrimSpace(head))
			}
			if strings.Contains(head, "/"+tc.query+" · 1 of 41") {
				t.Errorf("/%s at %dx%d: the identity counts drawn rows against the archived total: %q", tc.query, size[0], size[1], strings.TrimSpace(head))
			}
		}
	}
}

// ---- round 71, second-day ----
// At 120 and up the archive reader's title drew the ask whole two
// rows over the turn row that draws it whole — the repeat #105 folded for
// the archive's title, #110 for the live reader's and #138 for the trail's.
// #122 kept it here because a bare title would repeat the trail's title
// beside it; the day is the half that repeats, not the name.
func TestTheArchiveReaderTitleLeavesTheAskToItsTurnRow(t *testing.T) {
	forceASCII(t)
	turnRow := regexp.MustCompile(`^\s*❯ (.+?)\s{2,}\d\d:\d\d\s*$`)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		sc := sceneSecondDay()
		m := sceneModel(sc, size[0], size[1])
		for _, k := range []string{"2", "tab", "tab"} {
			pressKey(m, k)
			poll(m, sc)
		}
		view := ansi.Strip(m.View())
		said, turns := "", []string{}
		for _, l := range strings.Split(view, "\n") {
			for _, seg := range strings.Split(l, "│") {
				if j := strings.Index(seg, "READER · "); j >= 0 && said == "" {
					s := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(seg[j:]), "[reader]"))
					said = strings.TrimSpace(strings.TrimPrefix(s, "READER ·"))
				}
				if mm := turnRow.FindStringSubmatch(seg); mm != nil {
					turns = append(turns, strings.TrimSpace(mm[1]))
				}
			}
		}
		if said == "" || len(turns) == 0 {
			t.Fatalf("%dx%d: not the archive reader on its turn:\n%s", size[0], size[1], view)
		}
		for _, turn := range turns {
			if turn == said {
				t.Errorf("%dx%d: the reader's title repeats the ask its own turn row draws: %q over %q",
					size[0], size[1], "READER · "+said, "❯ "+turn)
			}
		}
	}
}

// ---- round 72, two-tools ----
// A note shorter than the footer's twelve-cell reserve costs no key. The
// board's `m` note is `mirror on`, nine cells with no longer form to grow
// into, and the reserve held three cells back from the keymap that nothing
// could ever fill: at 120 the frame after `m` shed ` · a ask` — eight cells
// — and stood on twelve blank ones, so pressing the mirror key cost the
// board the ask key (#177's own reason, #159, #168).
func TestAShortNoteCostsTheFooterNoKey(t *testing.T) {
	forceASCII(t)
	m := sceneModel(sceneTwoTools(), 120, 34)
	pressKey(m, "m")
	foot := ""
	for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(l, "? help · q quit") {
			foot = l
		}
	}
	if !strings.Contains(foot, "mirror on") {
		t.Fatalf("not the board's mirror note: %q", strings.TrimSpace(foot))
	}
	if !strings.Contains(foot, " · a ask") {
		t.Errorf("the nine-cell note sheds `a ask`: %q", strings.TrimSpace(foot))
	}
	// The keys the note-free board names are the keys this frame names.
	bare := sceneModel(sceneTwoTools(), 120, 34)
	want := ""
	for _, l := range strings.Split(ansi.Strip(bare.View()), "\n") {
		if strings.Contains(l, "? help · q quit") {
			want = strings.TrimSpace(l)
		}
	}
	if got := strings.TrimSpace(strings.Split(foot, "  ")[0]); got != want {
		t.Errorf("the mirror note costs the board keys:\n note-free %q\n with note %q", want, got)
	}
}

// ---- round 72, second-day ----
// The archive reader's title names the ask (#59, #105), so a clock
// right-aligned beside it is read as that ask's moment. Where #64 drops the
// anchored row's clause because what survives its cut is the name already on
// the row, the clock it timed goes with it (#115): left alone it stood as
// "READER · fix the 401 on token refresh   15:31" over an ask its own turn
// row times 15:00.
func TestTheArchiveReaderTitleClockGoesWithItsClause(t *testing.T) {
	forceASCII(t)
	clock := regexp.MustCompile(`\d\d:\d\d\s*$`)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		sc := sceneSecondDay()
		m := sceneModel(sc, size[0], size[1])
		for _, k := range []string{"2", "tab", "tab"} {
			pressKey(m, k)
			poll(m, sc)
		}
		for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			for _, seg := range strings.Split(l, "│") {
				j := strings.Index(seg, "READER · ")
				if j < 0 {
					continue
				}
				row := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(seg[j:]), "[reader]"))
				if !clock.MatchString(row) {
					continue
				}
				said := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(row, "READER · "), clock.FindString(row)))
				if !strings.Contains(said, " · ") {
					t.Errorf("%dx%d: the reader's title wears a clock with no clause to time: %q",
						size[0], size[1], row)
				}
			}
		}
	}
}

// ---- round 72, fleet-hygiene ----
// The board's miss keeps the doors beside it. A search nothing on the
// board answers drew `no session matches /eda · esc clears it` and
// nothing else — no hidden count, no archive door, no `A` — while the
// same fleet and the same search twenty columns narrower drew
// `0 of 41 archived · 1 of 1 hidden · A` from the list's own tail. On
// that frame a live session did answer the search: the hidden one, whose
// only way back is the `A` the frame does not name (#168, #169, #176, #178).
func TestTheBoardsMissKeepsTheDoorsBesideIt(t *testing.T) {
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneFleetHygiene(), size[0], size[1])
		for _, s := range m.sessions {
			if s.Live && sessionName(s.Info) == "notebooks" {
				m.point(s.Info.Key())
			}
		}
		pressKey(m, "x") // the eda session leaves the board
		pressKey(m, "/")
		for _, r := range "eda" {
			pressKey(m, string(r))
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "no session matches /eda") {
			t.Fatalf("%dx%d: not the board's miss:\n%s", size[0], size[1], view)
		}
		if !strings.Contains(view, "hidden") {
			t.Errorf("%dx%d: the board's miss names no hidden session, over a hidden one the search matched:\n%s", size[0], size[1], view)
		}
		if !strings.Contains(view, "archived") {
			t.Errorf("%dx%d: the board's miss names no archive door:\n%s", size[0], size[1], view)
		}
	}
}

// ---- round 72, fleet-hygiene, second finding ----
// The fleet's search counts what the board can draw. `· /p · 3 of 4`
// stood over three columns that all answered the search and a strip
// saying `1 of 1 hidden` — four of four answered, three were drawn, and
// the fourth is a row the numerator can never reach (#164, #176, #178).
func TestTheFleetsSearchCountsWhatTheBoardCanDraw(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneFleetHygiene(), size[0], size[1])
		for _, s := range m.sessions {
			if s.Live && sessionName(s.Info) == "notebooks" {
				m.point(s.Info.Key())
			}
		}
		pressKey(m, "x")
		pressKey(m, "/")
		pressKey(m, "p")
		head := ansi.Strip(strings.SplitN(m.View(), "\n", 2)[0])
		if !strings.Contains(head, "/p · 3 of 3") {
			t.Errorf("%dx%d: the search's clause counts a row the list cannot hold: %q", size[0], size[1], strings.TrimSpace(head))
		}
	}
}

// ---- round 73, second-day ----
// A sent trace says when. The row that reports a line compass typed and
// the session has not yet taken carries the proof of what went and the
// clock that says how long it has stood (#33, #90). At eighty the quote
// was keeping its tail and dropping the clock, so `↪ sent "please
// continue"` stood untimed two rows under `40s` and `50s ago`, while the
// same column on the same walkthrough drew `↪ sent "go on" · 0s ago`
// once the person happened to type a shorter line. The quote yields its
// tail for the clock while a readable stub of it survives.
func TestTheSentTraceSaysWhen(t *testing.T) {
	forceASCII(t)
	quoted := regexp.MustCompile(`↪ sent "[^"]*"`)
	aged := regexp.MustCompile(`↪ sent "[^"]*"\s+·\s+\d+[smhd] ago`)
	for _, scene := range []struct {
		name string
		make func() scene
	}{{"second-day", sceneSecondDay}, {"first-session", sceneFirstSession}} {
		sc := scene.make()
		m := sceneModel(sc, 80, 24)
		for _, k := range []string{"r", "1"} {
			pressKey(m, k)
			poll(m, sc)
		}
		view := ansi.Strip(m.View())
		hit := false
		for _, l := range strings.Split(view, "\n") {
			for _, seg := range strings.Split(l, "│") {
				if !quoted.MatchString(seg) {
					continue
				}
				hit = true
				if !aged.MatchString(seg) {
					t.Errorf("%s 80x24: the row says a line went and not when: %q",
						scene.name, strings.TrimSpace(seg))
				}
			}
		}
		if !hit {
			t.Fatalf("%s 80x24: the route draws no sent trace:\n%s", scene.name, view)
		}
	}
}

// ---- round 73, two-tools ----
// The mirror's undo is named on every frame the mirror owns, not only on the
// frame `m` was pressed on. #62 holds `m` back from the shed while its own
// note stands; one keypress later the note is gone, and at 120 columns `m`
// went with it — so the session view drew the pane's own panel
// ("⌁ dev:1.0 · the transcript, until the pane is captured") under a footer
// naming no way back to the conversation, while the same frame at 152 and 220
// named `m conversation`. The key the mirror replaced the panel with outlasts
// `h/l session`, and the row keeps `enter attach` and `a ask` (#168).
func TestTheMirrorsUndoIsNamedWhileTheMirrorStands(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		sc := sceneTwoTools()
		m := sceneModel(sc, size[0], size[1])
		// The mirror is turned on from the board, then a session is opened:
		// two keypresses later the note is gone and the pane is the panel.
		for _, k := range []string{"m", "2", "tab"} {
			pressKey(m, k)
			poll(m, sc)
		}
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		panel, foot := false, ""
		for _, l := range rows {
			if strings.Contains(l, "the transcript, until the pane is captured") {
				panel = true
			}
			if strings.Contains(l, "? help · q quit") {
				foot = strings.TrimRight(l, " ")
			}
		}
		if !panel {
			t.Fatalf("%dx%d: the mirror does not own the panel here", size[0], size[1])
		}
		if !strings.Contains(foot, " · m conversation") {
			t.Errorf("%dx%d: the mirror stands and no key undoes it: %q", size[0], size[1], strings.TrimSpace(foot))
		}
		for _, key := range []string{" · enter attach", " · a ask"} {
			if !strings.Contains(foot, key) {
				t.Errorf("%dx%d: naming `m` cost the row %q: %q", size[0], size[1], strings.TrimSpace(key), strings.TrimSpace(foot))
			}
		}
	}
}

// ---- round 73, two-tools (second finding) ----
// The answer's digit is the row's. #128 gives a trace note's bytes to the
// row that draws them — "↪ sent to ⌁ dev:2.0" over a row saying
// `↪ sent "go on"` — and the digit is the answer's bytes: which line went.
// The footer kept it anyway, so `↪ answered 1 · to ⌁ ops:0.0` stood eight
// cells wider than it needed over a row three lines up saying
// `↪ answered 1 · 0s ago`, and cost the hundred-column footer `x hide` and
// the 152 footer the attach aside.
func TestTheAnswersDigitIsTheRows(t *testing.T) {
	forceASCII(t)
	// At 220 too: the shed fires where a drawn row carries the head,
	// not only where the note is costing a key (#205).
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		sc := sceneTwoTools()
		m := sceneModel(sc, size[0], size[1])
		for _, k := range []string{"r", "1"} {
			pressKey(m, k)
			poll(m, sc)
		}
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		foot := strings.TrimRight(rows[len(rows)-1], " ")
		row := false
		for _, l := range rows[:len(rows)-1] {
			for _, seg := range strings.Split(l, "│") {
				if strings.HasPrefix(strings.TrimSpace(seg), "↪ answered 1") {
					row = true
				}
			}
		}
		if !row {
			t.Fatalf("%dx%d: no drawn row carries the trace", size[0], size[1])
		}
		if !strings.Contains(foot, "⌁ ops:0.0") {
			t.Fatalf("%dx%d: not the trace note: %q", size[0], size[1], strings.TrimSpace(foot))
		}
		if strings.Contains(foot, "answered 1") {
			t.Errorf("%dx%d: the note repeats the row's digit: %q", size[0], size[1], strings.TrimSpace(foot))
		}
	}
}

// ---- round 73, fleet-hygiene, the one thing ----
// A hidden live column names its pane once. On the archive board the
// hidden session's row draws `⌁ work:2.0 · main` (#53's "where it lives")
// two rows over the column's tag row, which draws `⌁ work:2.0` again —
// the repeat #64, #105, #110, #120, #138 and #179 each folded. The tag
// row is the invariant ("the third row is the tag's row, always"); the
// row above it keeps the branch, which the tag cannot say.
func TestTheHiddenColumnNamesItsPaneOnce(t *testing.T) {
	tag := regexp.MustCompile(`⌁ [^ │]+`)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneManyIdle(), size[0], size[1])
		for _, s := range m.sessions {
			if s.Live && sessionName(s.Info) == "webapp" {
				m.point(s.Info.Key())
			}
		}
		pressKey(m, "x")   // webapp leaves the board
		pressKey(m, "A")   // the archive, as a list
		pressKey(m, "esc") // one level out: the archive as a board
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		found := false
		for i := 0; i+2 < len(rows); i++ {
			cols := strings.Split(rows[i], "│")
			second := strings.Split(rows[i+1], "│")
			third := strings.Split(rows[i+2], "│")
			for k := range cols {
				if k >= len(second) || k >= len(third) {
					break
				}
				if !strings.Contains(cols[k], "○ webapp") {
					continue
				}
				found = true
				a := tag.FindAllString(second[k], -1)
				b := tag.FindAllString(third[k], -1)
				for _, x := range a {
					for _, y := range b {
						if x == y {
							t.Errorf("%dx%d: the hidden column names %s twice:\n %s\n %s\n %s",
								size[0], size[1], x, cols[k], second[k], third[k])
						}
					}
				}
			}
		}
		if !found {
			t.Fatalf("%dx%d: the hidden column is not on the archive board:\n%s", size[0], size[1], strings.Join(rows, "\n"))
		}
	}
}

// ---- round 73, fleet-hygiene, second finding ----
// A hide refused keeps the way deeper. `billing stays · dead on the API`
// and `etl stays · it is looping` are 31 and 24 cells against the twelve
// the footer reserves, and at eighty they cost the footer `tab deeper` —
// the frame's only naming of the way deeper — while `etl stays · it
// hangs`, twenty cells on the same scene at the same width, kept it. The
// reason is the selected row's own state, drawn on that row; #175's rung,
// on the refusal.
func TestTheRefusalKeepsTheWayDeeper(t *testing.T) {
	for _, c := range []struct {
		name string
		sc   scene
		row  string
	}{
		{"alarm-storm", sceneAlarmStorm(), "billing"},
		{"very-long", sceneVeryLong(), "etl"},
	} {
		m := sceneModel(c.sc, 80, 24)
		for _, s := range m.sessions {
			if s.Live && sessionName(s.Info) == c.row {
				m.point(s.Info.Key())
			}
		}
		pressKey(m, "x") // refused: it owes an alarm
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		foot := rows[len(rows)-1]
		if !strings.Contains(foot, c.row+" stays") {
			t.Fatalf("%s: not the refusal: %q", c.name, foot)
		}
		if !strings.Contains(foot, "tab deeper") {
			t.Errorf("%s: the refusal's reason costs the frame its only naming of the way deeper: %q", c.name, strings.TrimSpace(foot))
		}
	}
}

// ---- round 74, second-day ----
// The archive reader's title says what the header does not. #105 kept the
// ask on the title where the page has scrolled past the turn row, "the one
// case where it is the only copy" — and at eighty it is not the only copy:
// the identity header two rows above titles the archived row by its ask,
// so `READER · fix the 401 on token refresh` stood under
// `⌂ compass · 1 fix the 401 on token refresh · claude` and the frame said
// neither which session it was, nor when it ran, nor how it ended, all of
// which the same route draws twenty columns wider as
// `READER · api · 3h · 1 ship · 1 red`.
func TestTheArchiveReaderTitleSaysWhatTheHeaderDoesNot(t *testing.T) {
	forceASCII(t)
	title := regexp.MustCompile(`READER · (.+?)\s*(?:\[reader\])?\s*$`)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		sc := sceneSecondDay()
		m := sceneModel(sc, size[0], size[1])
		for _, k := range []string{"2", "tab", "tab"} {
			pressKey(m, k)
			poll(m, sc)
		}
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		if len(rows) == 0 {
			t.Fatalf("%dx%d: no frame", size[0], size[1])
		}
		head := oneSpace(rows[0])
		hit := false
		for _, l := range rows {
			for _, seg := range strings.Split(l, "│") {
				mt := title.FindStringSubmatch(seg)
				if mt == nil {
					continue
				}
				hit = true
				clause := strings.TrimSpace(mt[1])
				if len(clause) >= 10 && strings.Contains(head, clause) {
					t.Errorf("%dx%d: the reader's title repeats the header: %q under %q",
						size[0], size[1], clause, strings.TrimSpace(head))
				}
			}
		}
		if !hit {
			t.Fatalf("%dx%d: the route draws no reader title:\n%s", size[0], size[1], strings.Join(rows, "\n"))
		}
	}
}

// ---- round 74, fleet-hygiene, the one thing ----
// A width refused keeps the way deeper. `no board under 110 columns` (26
// cells) and `mirror needs 110 columns` (24) stand against the 22 the
// eighty-column fleet-list footer leaves beside `tab deeper`, so both
// cost that footer the frame's only naming of the way deeper — the harm
// #175 and #187 each folded, on the very form #165 held up as the one
// that "names the key and keeps the way in". The unit is the one word
// the person's own terminal supplies; the number and the key stay.
func TestTheWidthRefusalKeepsTheWayDeeper(t *testing.T) {
	for _, c := range []struct {
		name string
		sc   scene
	}{
		{"fleet-hygiene", sceneFleetHygiene()},
		{"many-idle", sceneManyIdle()},
	} {
		for _, k := range []string{"m", "shift+tab"} {
			m := sceneModel(c.sc, 80, 24)
			pressKey(m, k) // refused: no board, no mirror, under 110
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			foot := rows[len(rows)-1]
			if !strings.Contains(foot, "110") {
				t.Fatalf("%s %q: not the width refusal: %q", c.name, k, strings.TrimSpace(foot))
			}
			if !strings.Contains(foot, "tab deeper") {
				t.Errorf("%s %q: the width refusal's unit costs the frame its only naming of the way deeper: %q",
					c.name, k, strings.TrimSpace(foot))
			}
		}
	}
}

// ---- round 74, two-tools, the one thing ----
// The reserve is what the note draws. #180 stopped the footer holding
// twelve cells for a note with no longer form to grow into; a note whose
// only longer form is far wider than twelve is the same case. In the
// reader the chapter note `❯ 1/1` has two forms — the count alone and the
// count beside the whole prompt — and nothing between, so at eighty the
// row stood `❯ 1/1` beside 22 blank cells with `space unfold`, the
// reader's own key, shed for them; at a hundred `a ask`, at 120 `n/N`.
// A key comes back while the note is drawn at the same width beside it.
func TestTheReserveIsWhatTheNoteDraws(t *testing.T) {
	forceASCII(t)
	// The walkthrough's route into the reader's previous turn.
	keys := []string{"r", "1", "/", "pytest", "enter", "esc", "tab", "ctrl+u", "ctrl+u", "[", "]", "G", "tab", "["}
	for _, want := range []struct {
		w, h int
		key  string
	}{
		// The page is all on screen, so it offers no scroll key and
		// `space unfold` leads the row: what this test asserts is that
		// the reader's own key is drawn, which is unmoved.
		{80, 24, "space unfold"},
		{100, 30, " · a ask"},
		{120, 34, " · n/N"},
	} {
		sc := sceneTwoTools()
		m := sceneModel(sc, want.w, want.h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		foot := strings.TrimRight(rows[len(rows)-1], " ")
		if !strings.HasSuffix(foot, "❯ 1/1") {
			t.Fatalf("%dx%d: not the note this pins: %q", want.w, want.h, strings.TrimSpace(foot))
		}
		if !strings.Contains(foot, want.key) {
			t.Errorf("%dx%d: %q shed for a reserve the note cannot grow into: %q",
				want.w, want.h, strings.TrimSpace(want.key), strings.TrimSpace(foot))
		}
		// The note is unchanged by the key coming back.
		if !strings.Contains(foot, "? help · q quit") {
			t.Errorf("%dx%d: the keymap went: %q", want.w, want.h, strings.TrimSpace(foot))
		}
	}
}

// ---- round 75, second-day, the one thing ----
// The row that shipped says what the frame does not. At eighty the
// archive's ship row drew `◆ ship   fix the 401 on token refresh…` — a
// clipped third copy of the ask the identity header and the ◉ prompt row
// both draw whole on the same frame — and the cut threw away `(commit)`,
// the only word on the row that says how the day ended. Where the label
// is the ask with only a bracket after it and will not fit, the row draws
// the bracket's word (#64's device on the reader's title, #189).
func TestTheShipRowIsNotAClippedCopyOfTheAsk(t *testing.T) {
	forceASCII(t)
	ship := regexp.MustCompile(`ship\s+(\S[^│]*?)…`)
	routes := [][]string{{"A"}, {"2", "tab"}} // the archive board, and the trail of the session that shipped
	for _, size := range [][2]int{{80, 24}, {120, 34}, {152, 40}} {
		for _, route := range routes {
			m := sceneModel(sceneSecondDay(), size[0], size[1])
			for _, k := range route {
				pressKey(m, k)
			}
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			seen := false
			for _, l := range rows {
				for _, seg := range strings.Split(l, "│") {
					if !strings.Contains(seg, "ship") {
						continue
					}
					mt := ship.FindStringSubmatch(seg)
					if mt == nil {
						seen = true
						continue
					}
					seen = true
					clause := strings.TrimSpace(mt[1])
					if len([]rune(clause)) < 10 {
						continue
					}
					for _, other := range rows {
						if other == l {
							continue
						}
						for i := strings.Index(other, clause); i >= 0; i = strings.Index(other[i+1:], clause) + i + 1 {
							rest := other[i+len(clause):]
							if !strings.HasPrefix(rest, "…") {
								t.Errorf("%dx%d %v: the ship row is a clipped copy of a clause the frame draws whole: %q under %q",
									size[0], size[1], route, strings.TrimSpace(seg), strings.TrimSpace(other))
								break
							}
							if i+1 >= len(other) {
								break
							}
						}
					}
				}
			}
			if !seen {
				t.Fatalf("%dx%d %v: no ship row on the frame:\n%s", size[0], size[1], route, strings.Join(rows, "\n"))
			}
		}
	}
}

// ---- round 75, second-day, second finding ----
// The archive board's footer names the chapter keys it answers to. `[`
// and `]` act on the archive's Lv1 list as on the live one and refuse
// with `no earlier prompt` there, but the archive keymap was written out
// without them: 133 idle cells at 220 and no `[ ] chapters`, while one
// `tab` deeper the footer named it with four cells to spare (#193).
func TestTheArchiveFooterNamesTheChapterKeys(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneSecondDay(), size[0], size[1])
		pressKey(m, "A")
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		foot := strings.TrimSpace(rows[len(rows)-1])
		if !strings.Contains(foot, "A fleet") {
			t.Fatalf("%dx%d: not the archive footer: %q", size[0], size[1], foot)
		}
		if !strings.Contains(foot, "[ ] chapters") {
			t.Errorf("%dx%d: the archive footer does not name the chapter keys it answers to: %q", size[0], size[1], foot)
		}
	}
}

// ---- round 75, fleet-hygiene, the one thing ----
// A trace keeps the way deeper. `↪ sent to ⌁ harness:1.0` is 23 cells
// against the 22 the eighty-column Lv1 footer leaves beside `tab deeper`,
// so a reply to the fleet's own namesake cost the frame its only naming
// of the way deeper while the same keys on `⌁ tinker:0.0` (22) kept it.
// The destination stays; the arrow is its preposition.
func TestTheTraceKeepsTheWayDeeper(t *testing.T) {
	for _, c := range []struct {
		name string
		sc   scene
		keys []string
		dest string
	}{
		{"fleet-hygiene", sceneFleetHygiene(), []string{"2", "r", "1"}, "⌁ harness:1.0"},
		{"alarm-storm", sceneAlarmStorm(), []string{"r", "1"}, "⌁ ops:0.0"},
		{"subagents", sceneSubagents(), []string{"j", "r", "t", "go on", "enter"}, "⌁ harness:1.0"},
	} {
		sc := c.sc
		m := sceneModel(sc, 80, 24)
		for _, k := range c.keys {
			pressKey(m, k)
			poll(m, sc)
		}
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		foot := rows[len(rows)-1]
		if !strings.Contains(foot, c.dest) {
			t.Fatalf("%s: not the trace note: %q", c.name, strings.TrimSpace(foot))
		}
		if !strings.Contains(foot, "tab deeper") {
			t.Errorf("%s: the trace's preposition costs the frame its only naming of the way deeper: %q",
				c.name, strings.TrimSpace(foot))
		}
	}
}

// ---- round 75, two-tools, the one thing ----
// The reader's title says what the page does not. #110 bared the title
// where the page draws the anchored turn; the anchored row is not always a
// turn, and a leg's is drawn as its tool call — `⏺ AskUserQuestion(Open
// port 22 to the office CIDR? [office CIDR / keep bastion])` two rows
// under a title saying the same words, the leg's own highlighted row
// between them. Where no turn row says the anchor and the page does, the
// clause goes with its clock (#115), as it does one keypress later when
// `[` clears the anchor and the same page carries a bare title.
func TestTheReaderTitleSaysWhatThePageDoesNot(t *testing.T) {
	forceASCII(t)
	// The walkthrough's route into the reader, on the session whose
	// anchored leg is the question.
	keys := []string{"r", "1", "/", "pytest", "enter", "esc", "tab", "ctrl+u", "ctrl+u", "[", "]", "G", "tab"}
	for _, size := range []struct{ w, h int }{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		sc := sceneTwoTools()
		m := sceneModel(sc, size.w, size.h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		title, col, at := "", -1, -1
		for i, r := range rows {
			for j, seg := range strings.Split(r, "│") {
				if strings.Contains(seg, "READER · ") {
					title, col, at = seg, j, i
				}
			}
			if at >= 0 {
				break
			}
		}
		if at < 0 {
			t.Fatalf("%dx%d: no reader title on the frame", size.w, size.h)
		}
		clause := ""
		if head, rest, cut := strings.Cut(strings.TrimSpace(strings.ReplaceAll(title, "[reader]", "")), "  "); cut {
			_ = head
			clause = strings.TrimSpace(rest)
		}
		if clause == "" {
			continue // nothing to repeat
		}
		var page []string
		for _, r := range rows[at+1:] {
			if segs := strings.Split(r, "│"); len(segs) > col {
				page = append(page, strings.TrimSpace(segs[col]))
			}
		}
		said := oneSpace(strings.Join(page, " "))
		core := strings.TrimSpace(clause)
		if i := strings.LastIndex(core, " · "); i > 0 {
			core = core[:i] // the clock
		}
		core = strings.TrimSuffix(core, "…")
		if len(strings.Fields(core)) >= 2 && strings.Contains(said, core) {
			t.Errorf("%dx%d: the reader's title repeats what its own page draws: %q over %q",
				size.w, size.h, strings.TrimSpace(title), core)
		}
	}
}

// ---- round 75, two-tools, third finding ----
// The archive's hidden row spends its cells on what the header does not
// say. Under `⌂ compass · 1 api · opencode · sonnet-4-5 · ⌁ dev:2.0` the
// hidden session's row drew `opencode · ⌁ dev:2.0 · main`, two of its
// three clauses the header's, drawn identically; `main` is the one word
// the frame does not otherwise say (#167, #189, #196).
func TestTheArchivesHiddenRowSaysWhatTheHeaderDoesNot(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		pressKey(m, "j")
		pressKey(m, "x") // api leaves the board
		pressKey(m, "A") // and is the archive's one row
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		head := rows[0]
		row := ""
		for i, l := range rows {
			if strings.Contains(l, "▸1 ") && i+1 < len(rows) {
				row = strings.TrimSpace(strings.Split(rows[i+1], "│")[0])
			}
		}
		if row == "" || !strings.Contains(row, "main") {
			t.Fatalf("%dx%d: not the hidden row's second line: %q", size[0], size[1], row)
		}
		for _, c := range []string{"opencode", "⌁ dev:2.0"} {
			if strings.Contains(head, c) && strings.Contains(row, c) {
				t.Errorf("%dx%d: the hidden row repeats the header's %q: %q under %q", size[0], size[1], c, row, strings.TrimSpace(head))
			}
		}
	}
}

// ---- round 76, second-day, the one thing ----
// The archive's selected row says what the header does not. #196 split the
// header from the row for the archive's hidden *live* row; the archived
// row three lines below it in secondLineTagged kept drawing the tool word
// the identity header of the same frame draws whole, and paid for it in
// the one clause the row alone carries: `claude · chor… · ✓ shipped` under
// `⌂ compass · 8 add --json to every command · claude`, four letters of
// `chore/deps`, and `claude · fix/api-timeou… · ✗` where the whole branch
// and the verdict's word both fit without it. An unselected row keeps its
// word — that is what tells it from its neighbour where the archive holds
// two tools (#80).
func TestTheArchiveSelectedRowShedsTheToolWordTheHeaderSays(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {120, 34}, {152, 40}} {
		for _, route := range [][]string{{"A"}, {"A", "8"}, {"A", "8", "tab"}} {
			m := sceneModel(sceneSecondDay(), size[0], size[1])
			for _, k := range route {
				pressKey(m, k)
			}
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			head := rows[0]
			sel := -1
			for i, l := range rows {
				if strings.Contains(strings.SplitN(l, "│", 2)[0], "▸") && strings.Contains(l, "○") {
					sel = i
					break
				}
			}
			if sel < 0 || sel+1 >= len(rows) {
				t.Fatalf("%dx%d %v: no selected archive row:\n%s", size[0], size[1], route, strings.Join(rows, "\n"))
			}
			second := strings.TrimSpace(strings.SplitN(rows[sel+1], "│", 2)[0])
			for _, word := range []string{"claude", "opencode"} {
				if strings.HasPrefix(second, word+" ·") && strings.Contains(head, word) {
					t.Errorf("%dx%d %v: the archive's selected row repeats the tool word its own header draws: %q under %q",
						size[0], size[1], route, second, strings.TrimSpace(head))
				}
			}
			if strings.Contains(second, "…") {
				t.Errorf("%dx%d %v: the archive's selected row clips the one clause it alone carries: %q under %q",
					size[0], size[1], route, second, strings.TrimSpace(head))
			}
		}
	}
}

// ---- round 76, two-tools, the one thing ----
// The archive's list keeps the way deeper. #52 ranks `a ask` with the
// archive's own keys because there it is "the reason to be there — a
// claude on a session you can no longer attach to", and on the eighty-
// column archive list that rank cost the footer `tab deeper`, the frame's
// only naming of the way deeper — on frames whose own `enter` key says
// `enter attach`, so the session is still there to attach to and the
// reason is not theirs. Where the row can be attached, `a ask` is a key
// that acts on a row, which the way in outlasts (#39).
func TestTheArchivesListKeepsTheWayDeeper(t *testing.T) {
	forceASCII(t)
	for _, tc := range []struct {
		name string
		sc   func() scene
		keys []string
	}{
		{"two-tools", sceneTwoTools, []string{"j", "x", "A"}},
		{"subagents", sceneSubagents, []string{"x", "A"}},
		{"many-idle", sceneManyIdle, []string{"x", "A"}},
	} {
		sc := tc.sc()
		m := sceneModel(sc, 80, 24)
		for _, k := range tc.keys {
			pressKey(m, k)
			poll(m, sc)
		}
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		foot := ""
		for _, r := range rows {
			if strings.Contains(r, "q quit") {
				foot = strings.TrimSpace(r)
			}
		}
		if foot == "" || !strings.Contains(foot, "A fleet") {
			t.Fatalf("%s: not the archive list's footer: %q", tc.name, foot)
		}
		if !strings.Contains(foot, "enter attach") {
			continue // #52's reason holds: this row cannot be attached to
		}
		named := false
		for _, k := range []string{"tab deeper", "tab session", "tab reader"} {
			if strings.Contains(foot, k) {
				named = true
			}
		}
		if !named {
			t.Errorf("%s at 80x24: the archive list's footer names no way deeper beside %q: %q",
				tc.name, "enter attach", foot)
		}
	}
}

// ---- round 76, fleet-hygiene, the one thing ----
// The reader's footer names the archive where no row does. #62 handed the
// door to the footer below the board's width, on the reason that there
// "the reader takes the whole screen and no band or fleet row names the
// archive". Above that width the door is the band's — the fleet's own last
// line is not drawn beside the reader — and the band takes the digits the
// live fleet has not used, so a fleet of nine or more live sessions leaves
// it none. Twelve live sessions and three hundred archived: at 120, 152
// and 220 the reader's frame named neither the count nor the key, while
// the same keypresses at a hundred columns named both, and `A` opens the
// archive at every depth. The question is what the frame drew, not how
// wide it is; the frame that names the door on a row does not name it
// twice.
func TestTheReadersFooterNamesTheArchiveWhereNoRowDoes(t *testing.T) {
	for _, c := range []struct {
		name string
		sc   scene
	}{
		{"many-idle", sceneManyIdle()},
		{"fleet-hygiene", sceneFleetHygiene()},
	} {
		for _, size := range [][2]int{{100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			sc := c.sc
			m := sceneModel(sc, size[0], size[1])
			for _, k := range []string{"tab", "tab"} {
				pressKey(m, k)
				poll(m, sc)
			}
			if m.level < levelReader {
				t.Fatalf("%s %dx%d: the route does not reach the reader", c.name, size[0], size[1])
			}
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			foot := rows[len(rows)-1]
			onRow := false
			for _, r := range rows[:len(rows)-1] {
				if strings.Contains(r, "archived · A") {
					onRow = true
				}
			}
			if inFoot := strings.Contains(foot, "A archive"); inFoot == onRow {
				if onRow {
					t.Errorf("%s %dx%d: the archive door is named twice: %q", c.name, size[0], size[1], strings.TrimSpace(foot))
				} else {
					t.Errorf("%s %dx%d: no row and no key names the archive: %q", c.name, size[0], size[1], strings.TrimSpace(foot))
				}
			}
		}
	}
}

// ---- round 77, second-day, the one thing ----
// A page that is all on screen offers no key that scrolls it. #83 dropped
// `ctrl+d/u half page` from the Lv1 footer on a frame whose trail fits,
// because there the page keys "do nothing" and a footer naming them read
// as though they paged the list; #56 drops the whole set from a lane's
// page with no turns. The reader's own page was left offering `j/k scroll`
// and `ctrl+d/u half page` on a page it fits whole — the app itself says
// `all of it is on screen` the moment either is pressed — and at eighty
// that cost the `[` refusal the archive door, the frame's only naming of
// the archive (#62, #199), while the `]` refusal two cells shorter kept
// it.
func TestAPageAllOnScreenOffersNoScrollKey(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		w, h := size[0], size[1]
		// Into the live session's reader: one prompt, no reply yet, a
		// page of five rows in every terminal the walkthrough draws.
		m := sceneModel(sceneSecondDay(), w, h)
		pressKey(m, "tab")
		pressKey(m, "tab")
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		foot := strings.TrimSpace(rows[len(rows)-1])
		if !strings.Contains(strings.Join(rows, "\n"), "READER · hello") {
			t.Fatalf("%dx%d: not the reader: %q", w, h, foot)
		}
		// The page fits: `k` says so rather than moving.
		probe := sceneModel(sceneSecondDay(), w, h)
		pressKey(probe, "tab")
		pressKey(probe, "tab")
		pressKey(probe, "k")
		prows := strings.Split(ansi.Strip(probe.View()), "\n")
		if !strings.Contains(prows[len(prows)-1], "all of it is on screen") {
			t.Fatalf("%dx%d: not a page that fits: %q", w, h, strings.TrimSpace(prows[len(prows)-1]))
		}
		for _, key := range []string{"j/k scroll", "ctrl+d/u half page"} {
			if strings.Contains(foot, key) {
				t.Errorf("%dx%d: the reader offers %q on a page that is all on screen: %q", w, h, key, foot)
			}
		}
		if strings.Contains(strings.TrimSpace(prows[len(prows)-1]), "j/k scroll") {
			t.Errorf("%dx%d: the footer names the scroll key beside its own refusal: %q",
				w, h, strings.TrimSpace(prows[len(prows)-1]))
		}
	}
	// The cell the idle key was spending: at eighty the `[` refusal on
	// that page named neither the archive count nor the key that browses
	// it, and no row of the frame named either.
	m := sceneModel(sceneSecondDay(), 80, 24)
	for _, k := range []string{"tab", "tab", "["} {
		pressKey(m, k)
	}
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	foot := strings.TrimSpace(rows[len(rows)-1])
	if !strings.Contains(foot, "no earlier turn") {
		t.Fatalf("80x24: not the chapter refusal: %q", foot)
	}
	named := strings.Contains(foot, "A archive")
	for _, r := range rows[:len(rows)-1] {
		if strings.Contains(r, "archived · A") {
			named = true
		}
	}
	if !named {
		t.Errorf("80x24: the reader's `[` refusal names the archive nowhere: %q", foot)
	}
}

// ---- round 77, two-tools, the one thing ----
// The trail's footer keeps the way deeper under a note of its own.
//
// At eighty the Lv2 keymap is 65 cells against the 64 the twelve-cell
// note floor leaves, and `[ ] chapters` — fifteen cells, the widest
// optional key on the row — outranked `tab deeper`, so every note the
// trail draws that is not a chapter key's own (`at the start`, `no leg to
// move to`, `mirror needs 110 columns`) cost the frame its only naming of
// the way deeper: the harm #175, #187, #190, #194 and #198 each folded.
// The chapter key yields to a key naming a level, and only where the key
// comes back; under a chapter key's own note the key the note is about
// stays where it is (#24, #57), which the second half asserts.
func TestTheTrailsFooterKeepsTheWayDeeperUnderItsOwnNote(t *testing.T) {
	forceASCII(t)
	footer := func(m *Model) string {
		for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			if strings.Contains(l, "? help · q quit") {
				return strings.TrimSpace(l)
			}
		}
		return ""
	}
	for _, sc := range []struct {
		name  string
		scene scene
	}{
		{"two-tools", sceneTwoTools()},
		{"second-day", sceneSecondDay()},
		{"first-session", sceneFirstSession()},
	} {
		// The note the frame draws, and whether it is a chapter key's own.
		for _, route := range []struct {
			keys    []string
			chapter bool
		}{
			{[]string{"tab", "ctrl+u", "ctrl+u"}, false},
			{[]string{"tab", "m"}, false},
			{[]string{"tab", "["}, true},
		} {
			m := sceneModel(sc.scene, 80, 24)
			plain := ""
			for i, k := range route.keys {
				pressKey(m, k)
				if i == 0 {
					plain = footer(m) // the footer with no news on it
				}
			}
			foot := footer(m)
			if m.note == "" {
				t.Fatalf("%s %v: no note on the frame: %q", sc.name, route.keys, foot)
			}
			if !strings.Contains(plain, "tab deeper") {
				t.Fatalf("%s: the noteless Lv2 footer names no way deeper: %q", sc.name, plain)
			}
			if route.chapter {
				if !strings.Contains(foot, "[ ] chapters") {
					t.Errorf("%s %v: the chapter key's own note sheds the key it is about: %q",
						sc.name, route.keys, foot)
				}
				continue
			}
			if !strings.Contains(foot, "tab deeper") {
				t.Errorf("%s %v: the note %q costs the trail's footer the way deeper: %q",
					sc.name, route.keys, m.note, foot)
			}
		}
	}
}

// ---- round 77, two-tools, second finding ----
// Above the board's width the session view names the hidden session. The
// hidden count and its door are the fleet's last line and the board's
// strip; at Lv2 and Lv3 above 110 columns no fleet body is drawn beside
// the trail, so a fleet of four counted three on the chips and no row,
// chip or key said where the fourth went, while the same keypresses at
// eighty and a hundred drew `1 hidden · A, then x`. Where no fleet body
// stands the chip carries the clause; where one does, the chip does not
// (#64: one frame, one lesson) (#202, #199).
func TestTheSessionViewNamesTheHiddenSession(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		for _, s := range m.sessions {
			if s.Live && sessionName(s.Info) == "api" {
				m.point(s.Info.Key())
				break
			}
		}
		pressKey(m, "x")
		m.note = ""
		pressKey(m, "tab")
		if m.level < levelTrail {
			t.Fatalf("%dx%d: the route does not reach the session view", size[0], size[1])
		}
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		head := rows[0]
		onRow := false
		for _, r := range rows[1 : len(rows)-1] {
			if strings.Contains(r, "hidden · A") {
				onRow = true
			}
		}
		inHead := strings.Contains(head, "hidden · A, then x")
		if onRow == inHead {
			t.Errorf("%dx%d: the hidden session is named %d times (row %v, chip %v): %q", size[0], size[1], map[bool]int{true: 2, false: 0}[onRow], onRow, inHead, strings.TrimSpace(head))
		}
	}
}

// The session view's footer names the archive where no row does. #62 gave
// the archive door to the footer "below the board's width", on the reason
// that there the reader takes the whole screen and no band or fleet row
// names it; #199 replaced the width test with a test of what the frame
// drew, but left the level test where #62 had put it — the reader alone.
// One press shallower, the session view draws the trail and the reader
// panel and no fleet row at all, and the band takes only the digits the
// live fleet has not used (`free := 9 - used`), so a fleet of twelve over
// three hundred archived left eleven frames at 120, 152 and 220 naming
// neither the count nor the key, while the same keypresses at a hundred
// columns — where the fleet list is still beside the trail — named both.
// The door is named once on the frame: on a row, or on the footer.
func TestTheSessionViewsFooterNamesTheArchiveWhereNoRowDoes(t *testing.T) {
	forceASCII(t)
	for _, c := range []struct {
		name string
		sc   scene
	}{
		{"many-idle", sceneManyIdle()},
		{"fleet-hygiene", sceneFleetHygiene()},
	} {
		for _, size := range [][2]int{{100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			sc := c.sc
			m := sceneModel(sc, size[0], size[1])
			pressKey(m, "tab")
			poll(m, sc)
			if m.level != levelWaypoints {
				t.Fatalf("%s %dx%d: one tab does not reach the session view", c.name, size[0], size[1])
			}
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			foot := rows[len(rows)-1]
			onRow := false
			for _, r := range rows[:len(rows)-1] {
				// every form the door sheds to: "41 archived · A browses",
				// "300 archived · 1 hidden · A" (#176).
				for _, seg := range strings.Split(r, "│") {
					i := strings.Index(seg, "archived · ")
					if i < 0 {
						continue
					}
					rest := strings.Trim(seg[i+len("archived · "):], "─ ")
					if j := strings.LastIndex(rest, " · "); j >= 0 {
						rest = rest[j+len(" · "):]
					}
					if rest == "A" || rest == "A browses" {
						onRow = true
					}
				}
			}
			switch inFoot := strings.Contains(foot, "A archive"); {
			case inFoot && onRow:
				t.Errorf("%s %dx%d: the archive door is named twice: %q", c.name, size[0], size[1], strings.TrimSpace(foot))
			case !inFoot && !onRow:
				t.Errorf("%s %dx%d: no row and no key names the archive: %q", c.name, size[0], size[1], strings.TrimSpace(foot))
			}
		}
	}
}

// ---- round 78, two-tools ----
//
// A board card says the question once, and keeps its model.
//
// #107 blanks a card's second row where a trail row of that same column
// already says it, and #112 then hoists the tool and its model onto the
// row the card gave up — which on a board whose question is which model
// is the card's only naming of it. #116's reassembly of a label that
// wrapped dropped the whole continuation row as soon as the options
// began on it, so where the question's tail and its options shared one
// row — `│  └ CIDR? [office CIDR / keep bastion]`, the shape a 48-cell
// column draws — the compare failed: the card drew the sentence a
// second time and the tag row fell to the bare tool word, while the
// same card at 120 and at 220 drew `claude · sonnet-4-5`.
//
// The pin is the property, not the literal: on every board width the
// card's second row is not a copy of the question its own trail row
// says, and the column names the model.
func TestTheBoardCardSaysTheQuestionOnce(t *testing.T) {
	forceASCII(t)
	// The words before the options, as the card would draw them.
	const question = "Open port 22 to the office CIDR?"
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		// Answer the waiting question — the trace the card's tag row
		// then draws is what leaves the tag no room — and hide the
		// namesake so the board draws three columns.
		for _, k := range []string{"r", "1", "j", "x"} {
			pressKey(m, k)
		}
		if m.level != levelBoard {
			t.Fatalf("%dx%d: not on the board (level %d)", size[0], size[1], m.level)
		}
		var col []string
		for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			col = append(col, strings.TrimRight(strings.SplitN(l, "│", 2)[0], " "))
		}
		head := -1
		for i, l := range col {
			if strings.Contains(l, "▲ infra") {
				head = i
				break
			}
		}
		if head < 0 || head+2 >= len(col) {
			t.Fatalf("%dx%d: no infra column on the board", size[0], size[1])
		}
		second := strings.TrimSpace(col[head+1])
		if second != "" && strings.HasPrefix(question, strings.TrimSuffix(second, "…")) {
			t.Errorf("%dx%d: the card's second row repeats the question its own trail row says: %q",
				size[0], size[1], second)
		}
		named := false
		for _, l := range col[head : head+3] {
			if strings.Contains(l, "sonnet-4-5") {
				named = true
			}
		}
		if !named {
			t.Errorf("%dx%d: the card names no model: %q", size[0], size[1], col[head:head+3])
		}
	}
}

// A label that wrapped is reassembled whole: the options are not the
// label's, but the words before them on the same continuation row are.
func TestAWrappedLabelKeepsTheWordsBeforeTheOptions(t *testing.T) {
	rows := []string{
		" ▲ design Open port 22 to the office   waiting 4m",
		" │  └ CIDR? [office CIDR / keep bastion]",
	}
	if got, want := wrappedLabel(rows, 0, len(rows)), "▲ design Open port 22 to the office CIDR?"; got != want {
		t.Errorf("wrappedLabel = %q, want %q", got, want)
	}
	split := []string{
		" ▲ design Open port 22 to   waiting 4m",
		" │  ├ the office CIDR?",
		" │  └ [office CIDR / keep bastion]",
	}
	if got, want := wrappedLabel(split, 0, len(split)), "▲ design Open port 22 to the office CIDR?"; got != want {
		t.Errorf("wrappedLabel (two rows) = %q, want %q", got, want)
	}
}

// ---- round 78, second-day, the one thing ----
// An attach that cannot work is a refusal, not a key: it goes before the
// keys that act.
//
// `enter · no pane` is what the footer draws where the selected session
// has no pane to hand the terminal to (`enterKeymap`). The archive's own
// list already ranks it that way — "a refusal goes before the way in"
// (#52, #198) — but the reader's shed order ranked it *above* `a ask`,
// and `shedKeys` lets a key back only when every key ranked above it came
// back too. So at eighty the reader of a paneless session held eighteen
// cells for a refusal that could not be drawn at that width either and
// shed `a ask` — which in the archive is the reason to be there, "a
// claude on a session you can no longer attach to" (#52), and on a live
// paneless session is the one action left — leaving sixteen blank cells
// on the row. At a hundred, under a thirteen-cell note, the row spent
// eighteen cells on the refusal and shed the ask outright.
//
// The frames: second-day's archive reader at eighty
// (`space unfold · [ ] turns · esc back · A fleet · ? help · q quit`, 63
// cells in a 79-cell field) and fleet-hygiene's paneless live reader at
// eighty and, under `]`'s note, at a hundred.
func TestTheAttachRefusalYieldsToTheKeyThatActs(t *testing.T) {
	forceASCII(t)
	footer := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	cases := []struct {
		name  string
		scene func() scene
		keys  []string
		w, h  int
		title string
	}{
		{"second-day archive reader", sceneSecondDay, []string{"A", "8", "tab", "tab"}, 80, 24, "READER · cli"},
		{"second-day archive waypoints", sceneSecondDay, []string{"A", "8", "tab"}, 100, 30, "TRAIL · cli"},
		{"fleet-hygiene paneless reader", sceneFleetHygiene, []string{"3", "tab", "tab"}, 80, 24, "READER · notebooks"},
		{"fleet-hygiene paneless waypoints", sceneFleetHygiene, []string{"3", "tab"}, 80, 24, "TRAIL · notebooks"},
		{"fleet-hygiene paneless reader under a note", sceneFleetHygiene, []string{"3", "tab", "tab", "]"}, 100, 30, "READER · notebooks"},
	}
	for _, c := range cases {
		sc := c.scene()
		m := sceneModel(sc, c.w, c.h)
		for _, k := range c.keys {
			pressKey(m, k)
			poll(m, sc) // the refresh the deck does after every key
		}
		if !strings.Contains(ansi.Strip(m.View()), c.title) {
			t.Fatalf("%s %dx%d: not the reader: %q", c.name, c.w, c.h, footer(m))
		}
		// The selected session has no pane: `enter` is a refusal here.
		if m.enterKeymap() != "enter · no pane" {
			t.Fatalf("%s %dx%d: the row can be attached to: %q", c.name, c.w, c.h, m.enterKeymap())
		}
		foot := footer(m)
		if !strings.Contains(foot, "a ask") {
			t.Errorf("%s %dx%d: a session with no pane is offered no way to ask it: %q",
				c.name, c.w, c.h, foot)
		}
		// And the cells were there: the row the fold draws fits its field.
		if w := lipgloss.Width(foot); w > c.w-1 {
			t.Errorf("%s %dx%d: the footer overruns its field: %d cells", c.name, c.w, c.h, w)
		}
	}
	// The rank, stated: in the reader the refusal is shed before `a ask`,
	// as the archive's list sheds it before `tab deeper` and `a ask`.
	for _, lv := range []struct {
		name string
		keys []string
	}{{"the waypoints", []string{"A", "8", "tab"}}, {"the reader", []string{"A", "8", "tab", "tab"}}} {
		sc := sceneSecondDay()
		m := sceneModel(sc, 80, 24)
		for _, k := range lv.keys {
			pressKey(m, k)
			poll(m, sc)
		}
		order := m.shedOrder(false)
		refusal, ask := -1, -1
		for i, frag := range order {
			switch frag {
			case " · enter · no pane":
				refusal = i
			case " · a ask":
				ask = i
			}
		}
		if refusal < 0 || ask < 0 {
			t.Fatalf("%s shed order names neither the refusal nor the ask: %v", lv.name, order)
		}
		if refusal > ask {
			t.Errorf("%s sheds `a ask` before the attach refusal: refusal at %d, ask at %d", lv.name, refusal, ask)
		}
	}
}

// TestTheReplyBoxDoesNotStandOnTheStrip pins round seventy-eight's second
// finding: the reply box floats over the deck, and the board's leftmost
// column begins at cell zero, so a box placed by that column stood on the
// strip whole — the archive's own line, its count and the key that browses
// it. The panel's footer is its own and `A` does not act while the panel is
// up (#62, #64), so no key could say it either, and at 120, 152 and 220 the
// frame over forty-one archived sessions named neither, while the same
// keypress one column over named both. Both sides: the frame names the
// door, and the box does not move where it never covered it.
func TestTheReplyBoxDoesNotStandOnTheStrip(t *testing.T) {
	forceASCII(t)
	// The door in every form it sheds to, matched where it ends: the
	// box's own edge may stand on the same row (#176).
	door := regexp.MustCompile(`archived · (?:[^·]*hidden · )?A(?: browses)?(?:\s|$)`)
	for _, c := range []struct {
		name string
		sc   scene
	}{
		{"fleet-hygiene", sceneFleetHygiene()},
		{"many-idle", sceneManyIdle()},
	} {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			sc := c.sc
			m := sceneModel(sc, size[0], size[1])
			pressKey(m, "r")
			poll(m, sc)
			if !m.replying {
				t.Fatalf("%s %dx%d: r did not open the panel", c.name, size[0], size[1])
			}
			frame := ansi.Strip(m.View())
			if !strings.Contains(frame, "reply to ") {
				t.Fatalf("%s %dx%d: no reply box on the frame", c.name, size[0], size[1])
			}
			named := false
			for _, row := range strings.Split(frame, "\n") {
				if door.MatchString(row + " ") {
					named = true
				}
			}
			if !named {
				var foot string
				if rows := strings.Split(frame, "\n"); len(rows) > 0 {
					foot = strings.TrimSpace(rows[len(rows)-1])
				}
				t.Errorf("%s %dx%d: the reply box leaves the frame naming neither the archive nor its key (footer %q)", c.name, size[0], size[1], foot)
			}
			// The step is taken only where it buys the door: on a fleet
			// whose strip the box never covered, the box stays by the
			// column it is about.
			if c.name == "many-idle" && size[0] >= 120 && m.replyBox.left != 0 {
				t.Errorf("%s %dx%d: the box stepped where the strip already stood: left=%d", c.name, size[0], size[1], m.replyBox.left)
			}
		}
	}
}

// TestTheCardYieldsThePaneTheHeaderDrawsToTheClock pins round seventy-eight's
// one thing: the board card's third row is the tag's row, and #59's last-rung
// fallback took a bare pane even where taking it cost the trace a clause.
// #85's reason for that rung — "the pane is what attaches and is nowhere else
// on the board" — is false on the frame the identity header titles by that
// very pane: at 120 the selected column spent " · 0s ago", the send's clock
// and on no other row of the column, to draw "⌁ harness:1.0" a second time,
// one cell short. #90's device — the tag yields to the trace's clock where
// the frame says the tag elsewhere — from the tool word to the bare pane.
func TestTheCardYieldsThePaneTheHeaderDrawsToTheClock(t *testing.T) {
	forceASCII(t)
	pane := regexp.MustCompile(`⌁ [A-Za-z0-9_.:-]+`)
	for _, c := range []struct {
		name string
		sc   scene
	}{
		{"fleet-hygiene", sceneFleetHygiene()},
		{"subagents", sceneSubagents()},
	} {
		for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
			sc := c.sc
			m := sceneModel(sc, size[0], size[1])
			for _, k := range []string{"j", "r", "t", "go on", "enter"} {
				pressKey(m, k)
				poll(m, sc)
			}
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			if len(rows) < 4 {
				t.Fatalf("%s %dx%d: no frame", c.name, size[0], size[1])
			}
			said := pane.FindString(rows[0])
			if said == "" {
				t.Fatalf("%s %dx%d: the header names no pane: %q", c.name, size[0], size[1], strings.TrimSpace(rows[0]))
			}
			// The column that carries the send is the one whose trace the
			// reply wrote; it is the selected one, the one the header
			// titles.
			var trace string
			for _, r := range rows[1:] {
				for _, col := range strings.Split(r, "│") {
					if strings.Contains(col, `↪ sent "go on"`) {
						trace = strings.TrimSpace(col)
					}
				}
			}
			if trace == "" {
				t.Fatalf("%s %dx%d: no column carries the reply", c.name, size[0], size[1])
			}
			// Both sides. The clock is the send's own and is on no other
			// row of the column, so it stands at every width; and where
			// the column is wide enough for both, the pane stays — the
			// yield is priced in cells, not a shed of the tag (#85).
			if !strings.Contains(trace, " ago") {
				t.Errorf("%s %dx%d: the trace spends its clock to draw %s a second time: %q", c.name, size[0], size[1], said, trace)
			}
			if size[0] == 220 && !strings.Contains(trace, said) {
				t.Errorf("%s %dx%d: the pane went where both fit: %q", c.name, size[0], size[1], trace)
			}
		}
	}
}

// Round 79, fleet hygiene. #111's compare is against the rows the frame
// draws, box and all (#108, #207). `presentBeside` asked the box's own
// rows and stopped there whenever the box was up — "the box covers the
// trail's row" — but the box covers only the rows it stands on. On
// `many-idle` at a hundred the box is thirteen rows of a twenty-five-row
// body and the trail's HEAD row is two rows under its bottom edge, so the
// deck drew the same present twice: whole in the trail
// (`● build  Wiring the filter into the loader        for 1h`) and cut in
// the list row beside it (`● build  Wiring the filter…  for 1h`), which is
// the harm #111 folded. The trail row the box leaves standing is on the
// frame; only the rows under the box are not.
//
// Both sides: where the box does stand on the trail's row — the same
// scene at eighty, and `second-day` at a hundred, where the box covers the
// trail whole — the row keeps its present, because there it is the only
// copy.
func TestTheRowLeavesThePresentToTheTrailTheBoxLeavesStanding(t *testing.T) {
	forceASCII(t)
	press := func(m *Model, sc scene, keys ...string) {
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
	}

	// The trail row the box leaves standing: the row beside it must not
	// say the same sentence a second time, cut.
	sc := sceneManyIdle()
	m := sceneModel(sc, 100, 30)
	press(m, sc, "r")
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	whole, cut := 0, 0
	for _, l := range rows {
		for _, cell := range strings.Split(l, "│") {
			c := oneSpace(strings.TrimSpace(cell))
			if strings.HasPrefix(c, "● build Wiring the filter into the loader") {
				whole++
			}
			if strings.HasPrefix(c, "● build Wiring the filter…") {
				cut++
			}
		}
	}
	if whole != 1 {
		t.Errorf("many-idle 100x30 under the box: the trail's present stands %d times, want once:\n%s",
			whole, strings.Join(rows, "\n"))
	}
	if cut != 0 {
		t.Errorf("many-idle 100x30 under the box: the row repeats, cut, the present the trail draws whole two rows under the box:\n%s",
			strings.Join(rows, "\n"))
	}

	// The row the compare frees belongs to a session, not to air (#43,
	// #111): the list draws one more of the twelve and its count falls.
	view := oneSpace(ansi.Strip(m.View()))
	if !strings.Contains(view, "5 ○ mobile") {
		t.Errorf("the freed row was not given to a session:\n%s", ansi.Strip(m.View()))
	}
	if !strings.Contains(view, "▾ 7 more below · j") {
		t.Errorf("the list's own count did not follow the row it gained:\n%s", ansi.Strip(m.View()))
	}

	// The other side: the box on the trail's own row, and the row's copy
	// is the only one on the frame.
	for _, tc := range []struct {
		name     string
		sc       scene
		w, h     int
		sentence string
	}{
		{"many-idle", sceneManyIdle(), 80, 24, "● build Wiring the…"},
		{"second-day", sceneSecondDay(), 100, 30, "● scout thinking… for 40s"},
	} {
		mm := sceneModel(tc.sc, tc.w, tc.h)
		press(mm, tc.sc, "r")
		view := oneSpace(ansi.Strip(mm.View()))
		if !strings.Contains(view, tc.sentence) {
			t.Errorf("%s %dx%d: the row gave up the present the box stands on, so the frame says it nowhere:\n%s",
				tc.name, tc.w, tc.h, ansi.Strip(mm.View()))
		}
	}
}

// TestTheChapterKeysYieldWhereTheyCannotMove pins round seventy-nine's one
// thing: on a trail of one chapter, with the trail standing on it, `[`
// answers `no earlier prompt` and `]` answers `no later prompt` — whichever
// is pressed, at every width — so the fifteen cells `[ ] chapters` spends,
// the widest optional key on the row, buy a promise the next keypress
// refuses. At eighty the Lv2 footer of the one live session drew
// `j/k rows · [ ] chapters · r reply · tab deeper · esc back · ? help ·
// q quit` and named no way to attach: `enter attach`, fifteen cells and
// §3's only write action, was shed for a key that cannot move. #83 dropped
// the page keys from a trail that fits, #56 the whole set from a lane's
// page with no turns, #200 the scroll keys from a page all on screen — a
// key that cannot move is not on the row. Both sides: where the cells buy
// nothing back the key stands (#193's wide archive footer), and under a
// chapter key's own note the key the note is about stays (#24, #57).
func TestTheChapterKeysYieldWhereTheyCannotMove(t *testing.T) {
	forceASCII(t)
	footer := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	for _, c := range []struct {
		name  string
		scene func() scene
		w, h  int
		// The route to a Lv2 footer whose cells are short: at eighty the
		// keymap alone overruns the row, at a hundred a note takes the
		// cells (`ctrl+u` on a trail that fits — `no leg to move to`).
		after []string
		gains string
	}{
		{"second-day", sceneSecondDay, 80, 24, nil, "enter attach"},
		{"first-session", sceneFirstSession, 80, 24, nil, "enter attach"},
		{"second-day", sceneSecondDay, 100, 30, []string{"ctrl+u"}, "enter attach"},
		{"first-session", sceneFirstSession, 100, 30, []string{"ctrl+u"}, "enter attach"},
	} {
		// The key cannot move: both chapter keys refuse from this stand.
		for _, r := range []struct{ key, want string }{
			{"[", "no earlier prompt"}, {"]", "no later prompt"},
		} {
			sc := c.scene()
			m := sceneModel(sc, c.w, c.h)
			pressKey(m, "tab")
			poll(m, sc)
			pressKey(m, r.key)
			if m.note != r.want {
				t.Fatalf("%s %dx%d: `%s` moves from this stand: %q", c.name, c.w, c.h, r.key, m.note)
			}
			// #24 still holds: the key the note is about stays on the row.
			if foot := footer(m); !strings.Contains(foot, "[ ] chapters") {
				t.Errorf("%s %dx%d: the chapter key's own note sheds the key it is about: %q",
					c.name, c.w, c.h, foot)
			}
		}
		// And the footer whose cells are short spends none on it.
		sc := c.scene()
		m := sceneModel(sc, c.w, c.h)
		pressKey(m, "tab")
		poll(m, sc)
		for _, k := range c.after {
			pressKey(m, k)
			poll(m, sc)
		}
		foot := footer(m)
		if strings.Contains(foot, "[ ] chapters") {
			t.Errorf("%s %dx%d: the footer offers a chapter key that refuses on both sides: %q",
				c.name, c.w, c.h, foot)
		}
		if !strings.Contains(foot, c.gains) {
			t.Errorf("%s %dx%d: the cells the chapter key spends buy no key that acts: %q",
				c.name, c.w, c.h, foot)
		}
	}
	// A step that buys nothing is not taken: at 220 the archive's own list
	// sheds no key, so the footer still names what `[` and `]` are (#193).
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		sc := sceneSecondDay()
		m := sceneModel(sc, size[0], size[1])
		pressKey(m, "A")
		poll(m, sc)
		if foot := footer(m); !strings.Contains(foot, "[ ] chapters") {
			t.Errorf("%dx%d: a yield that buys nothing took the archive footer's chapter keys: %q",
				size[0], size[1], foot)
		}
	}
	// And where a chapter key does move, it stays: the long day's trail
	// has a dozen prompts and `[` steps them.
	sc := sceneVeryLong()
	m := sceneModel(sc, 80, 24)
	pressKey(m, "tab")
	poll(m, sc)
	if foot := footer(m); !strings.Contains(foot, "[ ] chapters") {
		t.Errorf("very-long 80x24: a trail of many chapters lost the keys that step them: %q", foot)
	}
}

// TestTheTurnKeysYieldWhereTheyCannotMove pins round eighty's one thing:
// #210's rule one level in. The reader's `[ ] turns` is the same key as the
// trail's `[ ] chapters` — the ❯ rows are the conversation's chapters as
// the prompts are the trail's (`readerChapter`) — and on the first prompt
// of a live session, with the reader anchored on the only turn there is,
// `[` answers `no earlier turn` and `]` answers `no later turn`, whichever
// is pressed and at every width. At eighty the Lv3 footer drew
// `space unfold · [ ] turns · esc back · A archive · ? help · q quit`,
// sixty-six of seventy-nine cells, and named no key that acts on the
// session it was reading: `a ask` and `enter attach` were both shed for
// twelve cells spent on a key that refuses on both sides.
//
// Both sides, as #210 has them: where the cells buy nothing back the key
// stands (152 and 220 shed nothing), under a chapter key's own note the
// key the note is about stays (#24, #57), and where a turn key moves it
// stays — many-idle's reader lands on `❯ 1/1` and very-long's on
// `❯ 12/12`.
func TestTheTurnKeysYieldWhereTheyCannotMove(t *testing.T) {
	forceASCII(t)
	footer := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	// reader is the Lv3 conversation of the selected session: one `tab` to
	// the trail, one more to the reader.
	reader := func(sc scene, w, h int) *Model {
		m := sceneModel(sc, w, h)
		for i := 0; i < 2; i++ {
			pressKey(m, "tab")
			poll(m, sc)
		}
		return m
	}
	for _, c := range []struct {
		name  string
		scene func() scene
		w, h  int
		gains []string
	}{
		{"second-day", sceneSecondDay, 80, 24, []string{"a ask", "enter attach"}},
		{"first-session", sceneFirstSession, 80, 24, []string{"a ask", "enter attach"}},
		{"second-day", sceneSecondDay, 100, 30, []string{"/ search"}},
		{"first-session", sceneFirstSession, 100, 30, []string{"r reply"}},
		{"second-day", sceneSecondDay, 120, 34, []string{"r reply"}},
	} {
		// The key cannot move: both turn keys refuse from this stand.
		for _, r := range []struct{ key, want string }{
			{"[", "no earlier turn"}, {"]", "no later turn"},
		} {
			sc := c.scene()
			m := reader(sc, c.w, c.h)
			pressKey(m, r.key)
			if m.note != r.want {
				t.Fatalf("%s %dx%d: `%s` moves the reader from this stand: %q",
					c.name, c.w, c.h, r.key, m.note)
			}
			// #24 still holds: the key the note is about stays on the row.
			if foot := footer(m); !strings.Contains(foot, "[ ] turns") {
				t.Errorf("%s %dx%d: the turn key's own note sheds the key it is about: %q",
					c.name, c.w, c.h, foot)
			}
		}
		// And the footer whose cells are short spends none on it.
		foot := footer(reader(c.scene(), c.w, c.h))
		if strings.Contains(foot, "[ ] turns") {
			t.Errorf("%s %dx%d: the reader offers a turn key that refuses on both sides: %q",
				c.name, c.w, c.h, foot)
		}
		for _, gain := range c.gains {
			if !strings.Contains(foot, gain) {
				t.Errorf("%s %dx%d: the cells the turn key spends buy no `%s`: %q",
					c.name, c.w, c.h, gain, foot)
			}
		}
	}
	// A yield that buys nothing is not taken: at 152 and 220 the reader
	// sheds no key, so the row still names what `[` and `]` are (#193).
	for _, size := range [][2]int{{152, 40}, {220, 48}} {
		foot := footer(reader(sceneSecondDay(), size[0], size[1]))
		if !strings.Contains(foot, "[ ] turns") {
			t.Errorf("%dx%d: a yield that buys nothing took the reader's turn keys: %q",
				size[0], size[1], foot)
		}
	}
	// And where a turn key does move, it stays.
	for _, c := range []struct {
		name  string
		scene func() scene
		want  string
	}{
		{"many-idle", sceneManyIdle, "❯ 1/1"},
		{"very-long", sceneVeryLong, "❯ 12/12"},
	} {
		sc := c.scene()
		m := reader(sc, 80, 24)
		if foot := footer(m); !strings.Contains(foot, "[ ] turns") {
			t.Errorf("%s 80x24: a reader whose turn keys move lost them: %q", c.name, foot)
		}
		pressKey(m, "[")
		if !strings.HasPrefix(m.note, c.want) {
			t.Errorf("%s 80x24: `[` was expected to land on %s, answered %q", c.name, c.want, m.note)
		}
	}
}

// TestTheMarkSaysWhatThisColumnLost pins round eighty's one thing: the
// overlay's right-hand mark answers "what did the box hide of this column",
// so it stands where the box covered something of that column and nowhere
// else. #64 wrote that rule — no right-hand mark when the peek rule blanked
// the whole peek — but measured it on the whole rest of the row, and the
// rest of the row runs on through board columns the box never touched. On
// many-idle at 220 the reply box covered a column that was blank for six
// rows, and every one of them wore `…` because a column two rules further
// right had words in it; one was the box's own closing border. Both sides:
// where the box covered so much as the trail's rail the mark stands (#75).
func TestTheMarkSaysWhatThisColumnLost(t *testing.T) {
	forceASCII(t)
	step := func(m *Model, sc scene, keys ...string) {
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
	}

	for _, tc := range []struct{ w, h int }{{220, 48}, {152, 40}, {120, 34}} {
		sc := sceneManyIdle()
		m := sceneModel(sc, tc.w, tc.h)
		before := strings.Split(ansi.Strip(m.View()), "\n")
		step(m, sc, "r")
		got := strings.Split(ansi.Strip(m.View()), "\n")

		top, left, pw := -1, 0, 0
		for i, line := range got {
			if j := strings.Index(line, "┌ reply to"); j >= 0 {
				top = i
				left = len([]rune(line[:j]))
				pw = strings.Index(line[j:], "┐")
				pw = len([]rune(line[j:j+pw])) + 2 // the border, and its column of air
			}
		}
		if top < 0 {
			t.Fatalf("many-idle %dx%d: no reply box drawn:\n%s", tc.w, tc.h, strings.Join(got, "\n"))
		}

		marked, bare, checked := 0, 0, 0
		for i := top; i < len(got); i++ {
			runes := []rune(got[i])
			last := strings.Contains(got[i], "└─")
			if len(runes) <= left+pw {
				if last {
					break
				}
				continue
			}
			after := strings.TrimSpace(string(runes[left+pw:]))
			if after == "" {
				continue
			}
			checked++
			cut := ""
			if b := []rune(before[i]); len(b) > left {
				end := left + pw + 1
				if end > len(b) {
					end = len(b)
				}
				cut = string(b[left:end])
				if j := strings.LastIndex(cut, "│"); j >= 0 {
					cut = cut[j+len("│"):]
				}
			}
			if strings.TrimSpace(cut) != "" {
				marked++
				if !strings.HasPrefix(after, "…") {
					t.Errorf("many-idle %dx%d: the box hid %q of this column and the row does not say it was cut:\n%s",
						tc.w, tc.h, strings.TrimSpace(cut), got[i])
				}
				if last {
					break
				}
				continue
			}
			bare++
			if strings.HasPrefix(after, "…") {
				t.Errorf("many-idle %dx%d: the box cut nothing from this column and the row says it was cut:\n%s",
					tc.w, tc.h, got[i])
			}
			if last {
				break
			}
		}
		if checked == 0 || marked == 0 || bare == 0 {
			t.Fatalf("many-idle %dx%d: the frame does not carry both sides (checked %d, marked %d, bare %d):\n%s",
				tc.w, tc.h, checked, marked, bare, strings.Join(got, "\n"))
		}

		// The mark stands on the row whose column the box cut down to its
		// rail — #75's own case — and never on the box's closing border.
		for _, line := range got {
			if j := strings.Index(line, "└─"); j >= 0 {
				if strings.Contains(line[j:], "…") {
					t.Errorf("many-idle %dx%d: the box's closing border wears a cut mark for an empty column:\n%s",
						tc.w, tc.h, line)
				}
			}
		}
	}

	// #75: a peek that covered only the trail's rail keeps the mark.
	sc := sceneManyIdle()
	m := sceneModel(sc, 220, 48)
	step(m, sc, "r")
	rail := false
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(line, "● working for 1h") && strings.Contains(line, "…") {
			rail = true
		}
	}
	if !rail {
		t.Errorf("many-idle 220x48: the row whose column the box cut down to its rail lost its mark:\n%s",
			ansi.Strip(m.View()))
	}
}

// TestTheMovementKeyYieldsWhereItCannotMove pins round eighty-one's one
// thing: #210's and #211's rule at the key the row leads with. On a fleet
// of one `j` and `k` both answer `the only live one`, and on a trail whose
// one row the cursor is already on they both answer `no leg to move to` —
// whichever is pressed and at every width — yet the footer spent ten cells
// on `j/k move` and shed `a ask`, `r reply` and `/ search` to do it. #44
// already ranks the movement key below the way out and the way in ("the
// arrows move too, and the help says so"); this is the rank read the other
// way round: a key that cannot move is not on the row.
//
// Both sides, as #210 and #193 have them: where the cells buy nothing back
// the key stands (second-day at 120 sheds nothing), under the movement
// key's own note the key the note is about stays (#24, #57), and where the
// key moves it stays — many-idle's list walks eight sessions.
func TestTheMovementKeyYieldsWhereItCannotMove(t *testing.T) {
	forceASCII(t)
	footer := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	stand := func(sc scene, w, h int, keys ...string) *Model {
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}
	for _, c := range []struct {
		name   string
		scene  func() scene
		w, h   int
		keys   []string
		refuse string
		gains  []string
	}{
		{"second-day", sceneSecondDay, 80, 24, nil, "the only live one", []string{"a ask"}},
		{"first-session", sceneFirstSession, 80, 24, nil, "the only live one", []string{"a ask"}},
		{"second-day", sceneSecondDay, 80, 24, []string{"shift+tab"}, "the only live one", []string{"r reply"}},
		{"second-day", sceneSecondDay, 100, 30, []string{"shift+tab"}, "the only live one", []string{"a ask", "/ search"}},
		{"first-session", sceneFirstSession, 100, 30, []string{"shift+tab"}, "the only live one", []string{"a ask", "/ search"}},
		{"second-day", sceneSecondDay, 80, 24, []string{"tab", "shift+tab", "x"}, "the only live one", []string{"r reply"}},
	} {
		// The key cannot move: both movement keys refuse from this stand.
		for _, key := range []string{"j", "k"} {
			sc := c.scene()
			m := stand(sc, c.w, c.h, append(append([]string(nil), c.keys...), key)...)
			if m.note != c.refuse {
				t.Fatalf("%s %dx%d after %v: `%s` moves from this stand: %q",
					c.name, c.w, c.h, c.keys, key, m.note)
			}
			// #24 still holds: the key the note is about stays on the row.
			if foot := footer(m); !strings.Contains(foot, "j/k ") {
				t.Errorf("%s %dx%d after %v: the movement key's own note sheds the key it is about: %q",
					c.name, c.w, c.h, c.keys, foot)
			}
		}
		// And the footer whose cells are short spends none on it.
		sc := c.scene()
		foot := footer(stand(sc, c.w, c.h, c.keys...))
		if strings.Contains(foot, "j/k ") {
			t.Errorf("%s %dx%d after %v: the row offers a movement key that refuses on both sides: %q",
				c.name, c.w, c.h, c.keys, foot)
		}
		for _, gain := range c.gains {
			if !strings.Contains(foot, gain) {
				t.Errorf("%s %dx%d after %v: the cells the movement key spends buy no `%s`: %q",
					c.name, c.w, c.h, c.keys, gain, foot)
			}
		}
	}
	// A yield that buys nothing is not taken: at 120, 152 and 220 the trail
	// of one leg sheds no key, so the row still names what `j` and `k` are
	// (#193).
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		sc := sceneSecondDay()
		m := stand(sc, size[0], size[1])
		if foot := footer(m); !strings.Contains(foot, "j/k legs") {
			t.Errorf("%dx%d: a yield that buys nothing took the trail's movement key: %q",
				size[0], size[1], foot)
		}
		m2 := stand(sc, size[0], size[1], "j")
		if m2.note != "no leg to move to" {
			t.Errorf("%dx%d: the trail of one leg was expected to refuse `j`, answered %q",
				size[0], size[1], m2.note)
		}
	}
	// And where the movement key does move, it stays.
	sc := sceneManyIdle()
	m := stand(sc, 80, 24)
	if foot := footer(m); !strings.Contains(foot, "j/k move") {
		t.Errorf("many-idle 80x24: a list whose movement key moves lost it: %q", foot)
	}
	was := m.selectedKey
	pressKey(m, "j")
	poll(m, sc)
	if m.selectedKey == was {
		t.Errorf("many-idle 80x24: `j` was expected to move the list, stayed on %q", was)
	}
}

// TestTheTurnKeyThatMovesTheMarkStays pins round eighty-one's one thing:
// #214 called `[` a key that cannot move wherever the page is drawn whole,
// on the reason that "the reader draws no cursor on the turn it stands on,
// so that landing writes its note and changes no drawn cell". It draws
// one. `landOnTurn` marks the turn with the inverse bar the trail's cursor
// uses (#110, #211), and the frame before the press draws that bar on the
// line the reader is anchored to: pressing `[` on the two-tools reader at
// eighty moves seventy-nine cells of reverse video seven rows up, from
// `I need a decision before I change the rule.` to
// `❯ tighten the vpc security groups`, and the same is true on the
// fleet-hygiene reader the fold was pinned on. The guard could not see it
// because it rendered under the ASCII profile, where the bar is nothing
// but the trailing spaces its padding leaves, and then trimmed them.
//
// So the key acts and stays on the row. Both sides: standing on the only
// turn both keys refuse and #211's own pin holds the shed there.
func TestTheTurnKeyThatMovesTheMarkStays(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // the bar is a style, not a space
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	reader := func(sc scene, w, h int, extra ...string) *Model {
		m := sceneModel(sc, w, h)
		for _, k := range append([]string{"tab", "tab"}, extra...) {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}
	// bar is the body row the inverse mark stands on, -1 when none does.
	bar := func(m *Model) int {
		for i, r := range strings.Split(m.View(), "\n") {
			if strings.Contains(r, "\x1b[7m") {
				return i
			}
		}
		return -1
	}
	foot := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	for _, c := range []struct {
		name  string
		scene func() scene
		w, h  int
	}{
		{"two-tools", sceneTwoTools, 80, 24},
		{"fleet-hygiene", sceneFleetHygiene, 80, 24},
		{"fleet-hygiene", sceneFleetHygiene, 100, 30},
		{"few-ongoing", sceneFewOngoing, 80, 24},
	} {
		was := bar(reader(c.scene(), c.w, c.h))
		now := bar(reader(c.scene(), c.w, c.h, "["))
		if was < 0 || now < 0 || was == now {
			t.Fatalf("%s %dx%d: `[` was expected to move the reader's mark, it stood at row %d and row %d",
				c.name, c.w, c.h, was, now)
		}
		if f := foot(reader(c.scene(), c.w, c.h)); !strings.Contains(f, "[ ] turns") {
			t.Errorf("%s %dx%d: the row sheds a turn key that moves the mark %d rows: %q",
				c.name, c.w, c.h, now-was, f)
		}
	}
}

// TestAStuckKeyIsNotTheGainThatBuysTheTrade pins round eighty-one's one
// thing: #213 took the movement key's cells "only where a key that acts
// comes back for it", and `keysActGained` never asked whether the key that
// came back was one the same rule calls stuck. On the archive's list —
// one hidden session, and a trail of one prompt beside it — `j/k move`
// answers `the only session` and the eleven cells it gave up bought
// `[ ] chapters`, fifteen cells on a pair where `[` answers `no earlier
// prompt` and `]` answers `no later prompt`. The row spent four cells more
// on a key that refuses. #210, #211 and #213 are one rule: a key that
// cannot move is not the gain that buys the trade.
//
// Both sides: where a key that acts comes back the movement key still
// yields (#213), and where nothing acts the stuck movement key stands, so
// the row still names what `j` and `k` are (#193).
func TestAStuckKeyIsNotTheGainThatBuysTheTrade(t *testing.T) {
	forceASCII(t)
	foot := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	// archive is the walkthrough's route into the archive list: the fleet,
	// a session hidden, then `A`.
	archive := func(w, h, n int) (*Model, scene) {
		sc := sceneTwoTools()
		m := sceneModel(sc, w, h)
		for _, k := range canonicalKeys[:n] {
			pressKey(m, k)
			poll(m, sc)
		}
		return m, sc
	}
	const keys = 39 // ends on the first `A`: the archive's own list
	for _, size := range [][2]int{{100, 30}, {120, 34}} {
		w, h := size[0], size[1]
		// Neither key moves from this stand, whichever is pressed.
		for _, r := range []struct{ key, want string }{
			{"[", "no earlier prompt"}, {"]", "no later prompt"},
			{"j", "the only session"}, {"k", "the only session"},
		} {
			m, _ := archive(w, h, keys)
			pressKey(m, r.key)
			if m.note != r.want {
				t.Fatalf("%dx%d: `%s` moves from this stand: %q", w, h, r.key, m.note)
			}
		}
		m, _ := archive(w, h, keys)
		if f := foot(m); strings.Contains(f, "[ ] chapters") {
			t.Errorf("%dx%d: the movement key's cells bought a chapter key that refuses: %q", w, h, f)
		}
		// Nothing on this row acts in its place, so the key stands (#193).
		if f := foot(m); !strings.Contains(f, "j/k move") {
			t.Errorf("%dx%d: a yield that buys nothing took the movement key: %q", w, h, f)
		}
	}
	// #213 still holds where a key that acts comes back: at eighty the
	// search leaves one row and the cells buy `g grab`.
	m, _ := archive(80, 24, 5)
	if f := foot(m); strings.Contains(f, "j/k move") || !strings.Contains(f, "g grab") {
		t.Errorf("80x24: the movement key did not yield to a key that acts: %q", f)
	}
}

// TestTheLiveRowSaysWhatTheHeaderDoesNot pins round eighty-one's one thing:
// #196 and #197 split the header from the row on the archive's two halves
// and left the live list alone, where the gate is still #96's fleet of one
// (`liveCount() == 1`). On a fleet of four the selected row drew the tool
// word and the model the identity header of the same frame draws whole —
// `claude · sonnet-4-5` under `⌂ compass · 1 infra · claude · sonnet-4-5`
// — and at eighty it cost the trace its clock: `↪ answered 1 · 0s claude`
// where the freed cells draw `↪ answered 1 · 0s ago`, #90's own device.
//
// Both sides. An unselected row keeps its word, which is what tells it
// from its neighbour where the fleet runs two tools (#80): rows 2, 3 and 4
// still name theirs. And a tag the header does not draw whole stays — the
// row's pane is its suffix, `⌁ :2.0`, and the header says `⌁ dev:2.0`.
func TestTheLiveRowSaysWhatTheHeaderDoesNot(t *testing.T) {
	forceASCII(t)
	// list is the fleet column of the frame, the rows as they are drawn.
	list := func(m *Model) string {
		var keep []string
		for _, l := range strings.Split(ansi.Strip(m.View()), "\n") {
			i := strings.Index(l, "│")
			if i < 0 {
				continue // the header, the rules and the footer are not rows
			}
			keep = append(keep, l[:i])
		}
		return strings.Join(keep, "\n")
	}
	for _, size := range [][2]int{{80, 24}, {100, 30}} {
		w, h := size[0], size[1]
		sc := sceneTwoTools()
		m := sceneModel(sc, w, h)
		poll(m, sc)
		head := ansi.Strip(m.headerLine(m.width))
		if !strings.Contains(head, "claude · sonnet-4-5") {
			t.Fatalf("%dx%d: the header does not draw the selected session's tag: %q", w, h, head)
		}
		if got := list(m); strings.Contains(got, "claude · sonnet-4-5") {
			t.Errorf("%dx%d: the selected row draws the tag the header draws whole:\n%s", w, h, got)
		}
		// The neighbours keep their words (#80).
		for _, want := range []string{"opencode", "claude · ⌁ :1.0", "opencode · gpt-5 · ⌁ :3.0"} {
			if got := list(m); !strings.Contains(got, want) {
				t.Errorf("%dx%d: an unselected row lost %q:\n%s", w, h, want, got)
			}
		}
		// The cells go to the trace's clock (#90): `r`, then the answer.
		for _, k := range []string{"r", "1"} {
			pressKey(m, k)
			poll(m, sc)
		}
		got := list(m)
		if !strings.Contains(got, "↪ answered 1 · 0s ago") {
			t.Errorf("%dx%d: the trace lost its clock to a word the header says:\n%s", w, h, got)
		}
		for _, line := range strings.Split(got, "\n") {
			if strings.Contains(line, "↪ answered") && strings.Contains(line, "claude") {
				t.Errorf("%dx%d: the trace's row wears the header's own word: %q", w, h, line)
			}
		}
	}
	// A tag the header does not draw whole stays: the row's pane is its
	// suffix under the group header, and the identity header says the
	// full address.
	sc := sceneTwoTools()
	m := sceneModel(sc, 100, 30)
	poll(m, sc)
	pressKey(m, "2") // the second session: opencode, in the tmux session it shares
	poll(m, sc)
	head := ansi.Strip(m.headerLine(m.width))
	if !strings.Contains(head, "⌁ dev:2.0") {
		t.Fatalf("100x30: the header does not draw the second session's address: %q", head)
	}
	if got := list(m); !strings.Contains(got, "⌁ :2.0") {
		t.Errorf("100x30: a tag the header does not draw whole was shed:\n%s", got)
	}
}

// TestTheCardYieldsThePaneWithColourOn pins round eighty-two's one thing.
// #208's yield — the board card gives up a bare pane the identity header
// already draws rather than spend the trace's clock on it — is decided in
// `columnTag` by reading the trace back: `HasPrefix(full, "↪ ")`,
// `HasSuffix(full, " ago")` and `HasPrefix(full, beside)`. `boardDelta`
// returns `dimStyle.Render(trace)`, so with colour on those words sit
// inside escape sequences and all three tests miss: the guard never fires,
// the card draws `⌁ harness:1.0` a second time and the send's clock — the
// one fact on no other row of the column — is off the frame. #208's own
// pin could not see it because it renders under the ASCII profile, where
// `dimStyle.Render` is the identity.
//
// #215 settled which frame answers: the one a person sees, colour and all.
// So the trace is read through its style.
//
// Both sides: at 220 the column is wide enough for the pane and the clock
// and the pane stays (#85), which is the yield being priced in cells and
// not a shed of the tag.
func TestTheCardYieldsThePaneWithColourOn(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // the trace is drawn dim, not bare
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	pane := regexp.MustCompile(`⌁ [A-Za-z0-9_.:-]+`)
	for _, c := range []struct {
		name string
		sc   scene
	}{
		{"fleet-hygiene", sceneFleetHygiene()},
		{"subagents", sceneSubagents()},
	} {
		for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
			sc := c.sc
			m := sceneModel(sc, size[0], size[1])
			for _, k := range []string{"j", "r", "t", "go on", "enter"} {
				pressKey(m, k)
				poll(m, sc)
			}
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			if len(rows) < 4 {
				t.Fatalf("%s %dx%d: no frame", c.name, size[0], size[1])
			}
			said := pane.FindString(rows[0])
			if said == "" {
				t.Fatalf("%s %dx%d: the header names no pane: %q", c.name, size[0], size[1], strings.TrimSpace(rows[0]))
			}
			var trace string
			for _, r := range rows[1:] {
				for _, col := range strings.Split(r, "│") {
					if strings.Contains(col, `↪ sent "go on"`) {
						trace = strings.TrimSpace(col)
					}
				}
			}
			if trace == "" {
				t.Fatalf("%s %dx%d: no column carries the reply", c.name, size[0], size[1])
			}
			if !strings.Contains(trace, " ago") {
				t.Errorf("%s %dx%d: with colour on the trace spends its clock to draw %s a second time: %q",
					c.name, size[0], size[1], said, trace)
			}
			if size[0] == 220 && !strings.Contains(trace, said) {
				t.Errorf("%s %dx%d: the pane went where both fit: %q", c.name, size[0], size[1], trace)
			}
		}
	}
}

// TestTheBoardSaysTheSameWordsWithColourOn is the rule behind it, asked of
// the whole walkthrough rather than one frame: a style is how a row is
// drawn, never what it says, so the canonical walkthrough must read the
// same under the ASCII profile and under a colour one. Before the fold it
// differed on six rows in three files — every one a board trace that lost
// its clock to a pane the header draws.
func TestTheBoardSaysTheSameWordsWithColourOn(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	frames := func(sc scene, w, h int) []string {
		m := sceneModel(sc, w, h)
		out := []string{ansi.Strip(m.View())}
		for _, k := range canonicalKeys {
			pressKey(m, k)
			poll(m, sc)
			out = append(out, ansi.Strip(m.View()))
		}
		return out
	}
	trimAll := func(s string) string {
		rows := strings.Split(s, "\n")
		for i, r := range rows {
			rows[i] = strings.TrimRight(r, " ")
		}
		return strings.Join(rows, "\n")
	}
	for _, sc := range []scene{sceneFleetHygiene(), sceneSubagents()} {
		for _, size := range [][2]int{{120, 34}, {152, 40}} {
			lipgloss.SetColorProfile(termenv.Ascii)
			mono := frames(sc, size[0], size[1])
			lipgloss.SetColorProfile(termenv.TrueColor)
			colour := frames(sc, size[0], size[1])
			for i := range mono {
				a, b := strings.Split(trimAll(mono[i]), "\n"), strings.Split(trimAll(colour[i]), "\n")
				for j := range a {
					if j < len(b) && a[j] != b[j] {
						t.Errorf("%s %dx%d frame %d row %d: the colour profile changed what the row says\n ascii:  %q\n colour: %q",
							sc.name, size[0], size[1], i, j, a[j], b[j])
					}
				}
			}
		}
	}
}

// TestThePageKeyShedsAtItsOwnRankWhereverItLeadsTheRow pins round
// eighty-two's one thing. #213 taught the footer to give up a movement key
// that cannot move, and #193's rule takes that yield only where a key that
// acts comes back for it. On the trail — the level the second day opens
// one `tab` in — the yield never came back with anything, because
// `shedOrder` names the page key only in its separator-led form
// (` · ctrl+d/u half page`). The movement key is the row's head, so the
// moment it goes the page key becomes the head and that fragment matches
// nothing: the eleven cells bought back the twenty-two-cell key #83 and
// #200 both call the first fragment to go, `keysActGained` read the gain
// as nothing (a page key is not a key that acts) and the row kept a `j`
// that answers `no leg to move to`, whichever of `j` and `k` is pressed
// and at every width, while `a ask` stayed off it.
//
// The page key sheds at its own rank wherever it lands on the row — the
// head form #56 gave the attach key and #200 gave `space unfold`.
//
// Both other sides: under the movement key's own note the key the note is
// about stays (#24, #57); where the yield still buys nothing the key
// stands (#193, at 120, 152 and 220); and where `j` moves it stays.
func TestThePageKeyShedsAtItsOwnRankWhereverItLeadsTheRow(t *testing.T) {
	forceASCII(t)
	footer := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	stand := func(sc scene, w, h int, keys ...string) *Model {
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}
	for _, c := range []struct {
		name  string
		scene func() scene
		w, h  int
		keys  []string
		gains []string
	}{
		{"second-day", sceneSecondDay, 80, 24, []string{"tab"}, []string{"a ask"}},
		{"first-session", sceneFirstSession, 80, 24, []string{"tab"}, []string{"a ask"}},
		{"second-day", sceneSecondDay, 80, 24, []string{"tab", "["}, []string{"tab deeper", "[ ] chapters"}},
		{"first-session", sceneFirstSession, 80, 24, []string{"tab", "]"}, []string{"tab deeper", "[ ] chapters"}},
		{"second-day", sceneSecondDay, 100, 30, []string{"tab", "["}, []string{"enter attach", "[ ] chapters"}},
		{"first-session", sceneFirstSession, 100, 30, []string{"tab", "]"}, []string{"enter attach", "[ ] chapters"}},
	} {
		// The key cannot move: both movement keys refuse from this stand.
		for _, key := range []string{"j", "k"} {
			sc := c.scene()
			m := stand(sc, c.w, c.h, append(append([]string(nil), c.keys...), key)...)
			if m.note != "no leg to move to" {
				t.Fatalf("%s %dx%d after %v: `%s` moves from this stand: %q",
					c.name, c.w, c.h, c.keys, key, m.note)
			}
			// #24 still holds: the key the note is about stays on the row.
			if foot := footer(m); !strings.Contains(foot, "j/k ") {
				t.Errorf("%s %dx%d after %v: the movement key's own note sheds the key it is about: %q",
					c.name, c.w, c.h, c.keys, foot)
			}
		}
		foot := footer(stand(c.scene(), c.w, c.h, c.keys...))
		if strings.Contains(foot, "j/k ") {
			t.Errorf("%s %dx%d after %v: the trail's row offers a movement key that refuses on both sides: %q",
				c.name, c.w, c.h, c.keys, foot)
		}
		if strings.Contains(foot, "ctrl+d/u") {
			t.Errorf("%s %dx%d after %v: the cells the movement key spends bought the page key back: %q",
				c.name, c.w, c.h, c.keys, foot)
		}
		for _, gain := range c.gains {
			if !strings.Contains(foot, gain) {
				t.Errorf("%s %dx%d after %v: the cells the movement key spends buy no `%s`: %q",
					c.name, c.w, c.h, c.keys, gain, foot)
			}
		}
	}
	// A yield that buys nothing is not taken: at 120, 152 and 220 the row
	// draws every key it has, so the trail still names what `j` and `k`
	// are (#193) — and still refuses them.
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		sc := sceneSecondDay()
		if foot := footer(stand(sc, size[0], size[1])); !strings.Contains(foot, "j/k legs") {
			t.Errorf("second-day %dx%d: a yield that buys nothing took the trail's movement key: %q",
				size[0], size[1], foot)
		}
		if m := stand(sc, size[0], size[1], "j"); m.note != "no leg to move to" {
			t.Errorf("second-day %dx%d: the trail of one leg was expected to refuse `j`, answered %q",
				size[0], size[1], m.note)
		}
	}
	// And where the movement key does move, it stays — many-idle's trail
	// walks its legs, and its footer still names the page key.
	sc := sceneManyIdle()
	m := stand(sc, 80, 24, "tab")
	if foot := footer(m); !strings.Contains(foot, "j/k rows") {
		t.Errorf("many-idle 80x24: a trail whose movement key moves lost it: %q", foot)
	}
	was := ansi.Strip(m.View())
	pressKey(m, "j")
	poll(m, sc)
	if ansi.Strip(m.View()) == was {
		t.Errorf("many-idle 80x24: `j` was expected to move on the trail, drew the same frame")
	}
}

// ---- round 82, second-day, second finding ----
// The page key sheds from a trail the panel draws whole. #83 shed
// `ctrl+d/u half page` at Lv1 on a trail that fits and #200 in the reader
// on a page all on screen; Lv2, the level the keys actually page, was the
// gap: at 152 the session view spent 21 of 142 cells naming it over a
// two-row trail. Where the trail's document fits its box the key goes;
// where it does not, it stays (#220).
func TestThePageKeyShedsFromATrailDrawnWhole(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		// A fleet of one opens on its session view at the board's width.
		m := sceneModel(sceneSecondDay(), size[0], size[1])
		if m.level != levelWaypoints {
			pressKey(m, "tab")
		}
		if m.level != levelWaypoints {
			t.Fatalf("%dx%d: the route does not reach the session view (Lv%d)", size[0], size[1], m.level)
		}
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		foot := strings.TrimSpace(rows[len(rows)-1])
		if strings.Contains(foot, "ctrl+d/u half page") {
			t.Errorf("%dx%d: the footer names the page key over a trail it draws whole: %q", size[0], size[1], foot)
		}
	}
	// The other side: a trail longer than its box keeps the key.
	m := sceneModel(sceneVeryLong(), 220, 48)
	pressKey(m, "tab")
	w, h := m.trailBox()
	if doc, _ := trailDoc(m.trail, m.trailOpts(w, h)); len(doc) <= h {
		t.Skip("the very-long trail fits its box here; not this side's case")
	}
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	if foot := strings.TrimSpace(rows[len(rows)-1]); !strings.Contains(foot, "ctrl+d/u half page") {
		t.Errorf("220x48: a trail longer than its box lost the page key: %q", foot)
	}
}

// TestThePageKeyThatWalksTheCursorStays pins round eighty-two's one thing:
// #220 shed `ctrl+d/u half page` from every Lv2 trail whose document fits
// its box, on the reason that "on a trail the panel draws whole
// `ctrl+d/u` moves nothing". At Lv2 they do not move the viewport — they
// walk the cursor half a screenful of rows, as the handler says of itself
// ("the cursor is what the viewport follows here, so the cursor is what
// moves", §3) — so on a trail drawn whole the press still moves the `▸`.
// #83's and #200's viewport test belongs to the levels whose keys page;
// here the question is the cursor's, and it is the movement key's own test
// (#213, #219): only a trail of one row leaves both keys nothing.
//
// Both sides: on the two-tools legs at 152 and 220 the press moves the
// cursor and the key stays; on a trail of one row it refuses
// (`no leg to move to`) and the row sheds it.
func TestThePageKeyThatWalksTheCursorStays(t *testing.T) {
	forceASCII(t)
	legs := func(sc scene, w, h int, extra ...string) *Model {
		m := sceneModel(sc, w, h)
		for m.level != levelWaypoints {
			pressKey(m, "tab")
			poll(m, sc)
		}
		for _, k := range extra {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}
	foot := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	body := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.Join(rows[:len(rows)-1], "\n")
	}
	for _, c := range []struct {
		name  string
		scene func() scene
		w, h  int
	}{
		{"two-tools", sceneTwoTools, 152, 40},
		{"two-tools", sceneTwoTools, 220, 48},
		{"fleet-hygiene", sceneFleetHygiene, 152, 40},
	} {
		m := legs(c.scene(), c.w, c.h)
		if body(legs(c.scene(), c.w, c.h, "ctrl+u")) == body(m) {
			t.Fatalf("%s %dx%d: `ctrl+u` was expected to walk the cursor, it moved no drawn row", c.name, c.w, c.h)
		}
		if f := foot(m); !strings.Contains(f, "ctrl+d/u half page") {
			t.Errorf("%s %dx%d: the row sheds a page key that walks the cursor: %q", c.name, c.w, c.h, f)
		}
	}
	// The other side: a trail of one row leaves the cursor nowhere, both
	// keys answer `no leg to move to`, and the row sheds the key.
	for _, w := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m := legs(sceneSecondDay(), w[0], w[1])
		if len(TrailRows(m.trail, m.level)) > 1 {
			t.Fatalf("second-day %dx%d: this side wants a trail of one row, it has %d",
				w[0], w[1], len(TrailRows(m.trail, m.level)))
		}
		if f := foot(m); strings.Contains(f, "ctrl+d/u half page") {
			t.Errorf("second-day %dx%d: the row names a page key with nowhere to walk: %q", w[0], w[1], f)
		}
	}
}

// TestTheUnfoldKeyYieldsWhereItCannotUnfold pins round eighty-two's one
// thing: the reader's `space unfold` on a page where no row can fold or
// unfold. #210 took the chapter key that cannot move, #211 the turn key,
// #213 the movement key; Space is the fourth key of the same shape and the
// one they did not reach. On the second day's reader at eighty the page
// holds one prompt and one thinking leg — nothing foldable — so Space
// answers `nothing to unfold on screen` whichever row is on top and at
// every width, while the reader's shed order ranks the twelve cells it
// spends above `/ search`, `n/N` and `r reply`, all of which act on the
// conversation it reads.
//
// Both sides. Where the screen holds a fold the key stays and acts (the
// two-tools reader unfolds `Read(main.tf)`), and under its own refusal the
// key stays where it is (#24, #57): the row refusing Space must name Space.
func TestTheUnfoldKeyYieldsWhereItCannotUnfold(t *testing.T) {
	forceASCII(t)
	reader := func(sc scene, w, h, n int, extra ...string) *Model {
		m := sceneModel(sc, w, h)
		for i := 0; i < n; i++ {
			pressKey(m, canonicalKeys[i])
			poll(m, sc)
		}
		for _, k := range extra {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}
	foot := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	// n is the walkthrough prefix that lands in the reader: the thirteenth
	// key is the `tab` from the waypoints into the conversation.
	for _, c := range []struct {
		name   string
		scene  func() scene
		w, h   int
		n      int
		gained string
	}{
		{"second-day", sceneSecondDay, 80, 24, 13, "/ search"},
		{"first-session", sceneFirstSession, 80, 24, 13, "r reply"},
		{"second-day", sceneSecondDay, 100, 30, 13, "r reply"},
		{"first-session", sceneFirstSession, 100, 30, 16, "/ search"},
	} {
		m := reader(c.scene(), c.w, c.h, c.n)
		if note := reader(c.scene(), c.w, c.h, c.n, "space").note; note != "nothing to unfold on screen" {
			t.Fatalf("%s %dx%d: Space was expected to refuse, it said %q", c.name, c.w, c.h, note)
		}
		f := foot(m)
		if strings.Contains(f, "space unfold") {
			t.Errorf("%s %dx%d: the row spends twelve cells on a key that answers `nothing to unfold on screen`: %q",
				c.name, c.w, c.h, f)
		}
		if !strings.Contains(f, c.gained) {
			t.Errorf("%s %dx%d: the freed cells were expected to name %q, the row is %q", c.name, c.w, c.h, c.gained, f)
		}
	}
	// The key stays where it acts: the two-tools reader has a folded
	// result on screen and Space opens it.
	m := reader(sceneTwoTools(), 80, 24, 13)
	if note := reader(sceneTwoTools(), 80, 24, 13, "space").note; !strings.HasPrefix(note, "unfolded ") {
		t.Fatalf("two-tools 80x24: Space was expected to unfold, it said %q", note)
	}
	if f := foot(m); !strings.Contains(f, "space unfold") {
		t.Errorf("two-tools 80x24: the row sheds a key that acts: %q", f)
	}
	// And it stays under its own note: the row refusing Space names Space.
	if f := foot(reader(sceneFirstSession(), 80, 24, 0, "tab", "tab", "tab", "space")); !strings.Contains(f, "space unfold") {
		t.Errorf("first-session 80x24: the row refusing Space does not name Space: %q", f)
	}
}

// TestTheWalkKeyYieldsWithNoSearchToWalk pins round eighty-two's second
// finding: the reader's `n/N` on a conversation nobody has searched. Until
// `/` has been entered `jumpMatch` answers `no search — / starts one` to
// both keys, at every width, so the six cells the pair spends buy a
// promise the next keypress refuses — #210's, #211's and #213's rule at
// the pair they did not reach. `/ search` stands beside it and says how a
// walk begins, so the row loses nothing it was teaching.
//
// Both sides. Where a search is standing the pair walks it and stays;
// under its own refusal the key stays where it is (#24, #57); and a
// stuck key still buys nothing (#216) — the trade is taken only where
// the freed cells name a key that acts.
func TestTheWalkKeyYieldsWithNoSearchToWalk(t *testing.T) {
	forceASCII(t)
	reader := func(sc scene, w, h, n int, extra ...string) *Model {
		m := sceneModel(sc, w, h)
		for i := 0; i < n; i++ {
			pressKey(m, canonicalKeys[i])
			poll(m, sc)
		}
		for _, k := range extra {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}
	foot := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	for _, c := range []struct {
		name   string
		scene  func() scene
		w, h   int
		n      int
		gained string
	}{
		{"two-tools", sceneTwoTools, 100, 30, 13, "r reply"},
		{"subagents", sceneSubagents, 100, 30, 33, "r reply"},
		{"two-tools", sceneTwoTools, 120, 34, 19, "h/l session"},
	} {
		m := reader(c.scene(), c.w, c.h, c.n)
		for _, k := range []string{"n", "N"} {
			if note := reader(c.scene(), c.w, c.h, c.n, k).note; note != "no search — / starts one" {
				t.Fatalf("%s %dx%d: %q was expected to refuse, it said %q", c.name, c.w, c.h, k, note)
			}
		}
		f := foot(m)
		if strings.Contains(f, "n/N") {
			t.Errorf("%s %dx%d: the row names a pair that answers `no search — / starts one`: %q", c.name, c.w, c.h, f)
		}
		if !strings.Contains(f, c.gained) {
			t.Errorf("%s %dx%d: the freed cells were expected to name %q, the row is %q", c.name, c.w, c.h, c.gained, f)
		}
		// The row still teaches how a walk begins.
		if !strings.Contains(f, "/ search") {
			t.Errorf("%s %dx%d: the row that sheds the walk keys must keep the search: %q", c.name, c.w, c.h, f)
		}
		// Where a search is standing the pair walks it and stays.
		if g := foot(reader(c.scene(), c.w, c.h, c.n, "/", "e", "enter")); !strings.Contains(g, "n/N") {
			t.Errorf("%s %dx%d: the row sheds a walk key with a search to walk: %q", c.name, c.w, c.h, g)
		}
	}
	// Under its own refusal the key stays where the width leaves it room
	// (#24, #57): the row refusing `n` names `n/N`.
	for _, w := range [][2]int{{152, 40}, {220, 48}} {
		g := foot(reader(sceneTwoTools(), w[0], w[1], 13, "n"))
		if !strings.Contains(g, "no search — / starts one") || !strings.Contains(g, "n/N") {
			t.Errorf("two-tools %dx%d: the row refusing `n` does not name it: %q", w[0], w[1], g)
		}
	}
}
