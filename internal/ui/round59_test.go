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
	if !strings.Contains(view, "the only live one stays") {
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
	for _, size := range [][2]int{{80, 24}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
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
