package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/deephanson94/compass/internal/transcript"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
		// The reader's cursor opens on this very turn (there is only one),
		// and marks it — "❯▸add a --version flag" — the same convention
		// the trail's own cursor wears; either form of the row still names
		// the turn.
		trimmed := strings.TrimLeft(l, " ")
		if strings.HasPrefix(trimmed, "❯ add a --version flag") || strings.HasPrefix(trimmed, "❯▸add a --version flag") {
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
		// `[` lands the reader's cursor on this very turn, marking it
		// "❯▸…" rather than "❯ …" (markAnchor's convention, the same the
		// trail's cursor wears) — either form is still the turn row.
		trimmed := strings.TrimLeft(l, " ")
		if strings.HasPrefix(trimmed, "❯ ") || strings.HasPrefix(trimmed, "❯▸") {
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
		// `[` lands the reader's cursor on this turn, marking it "❯▸relayed"
		// rather than "❯ relayed" (markAnchor's convention); either is the row.
		trimmed := strings.TrimLeft(l, " ")
		if strings.HasPrefix(trimmed, "❯ relayed") || strings.HasPrefix(trimmed, "❯▸relayed") {
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
		// At 120 the key the reserve had cost the row was `n/N`, which
		// #223 then found refuses until a search stands; the reserve's
		// twelve cells now come back to `h/l session`, which moves 21
		// drawn rows from this stand. The reserve is what this pins, so
		// it points at the key that acts (#229).
		{120, 34, " · h/l session"},
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
	// box's own edge may stand on the same row (#176). The key half is
	// optional — while the panel is up the line wears no key, since `A`
	// does not act there (#282's rule on this row, archiveDoorKey) — and
	// what this test is about is the row the box must not cover.
	door := regexp.MustCompile(`\d archived(?: · [^·]*hidden)?(?: · A(?: browses)?)?(?:\s|$)`)
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
	// The count alone: the fold sheds its key clause while the quick
	// replies hold the keyboard (#280's rule, at Lv1).
	if !strings.Contains(view, "▾ 7 more below") {
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
		// Where a write does come back for it — `r reply` for the live row
		// the archive selects — the yield has bought something and #213
		// applies instead.
		if f := foot(m); !strings.Contains(f, "j/k move") && !strings.Contains(f, "r reply") {
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
		// {"two-tools", sceneTwoTools, 120, 34, 19, "h/l session"} stood
		// here: canonicalKeys[15:18] is `k`, `k`, `j` in this scene's
		// reader, and the reader's cursor (readerCursorMove) now steps
		// under those keys even on a page that fits, same as it must to
		// reach a second fold Space could not reach before. Landing off
		// the session's one turn leaves `turnStand` reporting `cur < 0`,
		// so `[ ] turns` reads as a key that still moves and is no
		// longer shed at this stand — the two cells `n/N` frees go to it
		// rather than to `h/l session`, and there is no longer room for
		// both. The other two rows still pin the trade this test is for.
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

// ---- round 83, second-day, the second finding ----
// The hairline runs beside the reply box. `joinColumns` stops the
// hairline where the columns' content stops — "a hairline stops where the
// content stops; empty rows stay empty" — but the reply box is
// composited over the body afterwards (#108), so a box reaching past
// every column's last row drew its own border with no hairline to its
// left. On the first session at eighty the fleet is one row and eleven of
// the box's fourteen rows stood past the hairline while the three above
// them carried it; on the second day it was the box's last row. The box's
// rows are the frame's content, so the hairline stops below them; where
// no box stands, an empty row is still empty.
func TestTheHairlineRunsBesideTheReplyBox(t *testing.T) {
	// The frame is the coloured one (#215, #218): the hairline is a drawn
	// cell either way, and this reads it through the profile a person has.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	const bodyTop = 3 // header, hairline, blank
	// at says whether the frame draws the hairline at cell col on row j.
	at := func(rows []string, j, col int) bool {
		if j < 0 || j >= len(rows) {
			return false
		}
		run := []rune(rows[j])
		return col < len(run) && run[col] == '│'
	}

	for _, c := range []struct {
		name string
		mk   func() scene
		w, h int
	}{
		{"first-session", sceneFirstSession, 80, 24},
		{"first-session", sceneFirstSession, 100, 30},
		{"second-day", sceneSecondDay, 80, 24},
		{"second-day", sceneSecondDay, 100, 30},
	} {
		sc := c.mk()
		m := sceneModel(sc, c.w, c.h)
		pressKey(m, "r")
		poll(m, sc)
		// `replyBox` is settled by View, which is where the box is placed.
		rendered := ansi.Strip(m.View())
		if !m.replyBox.on {
			t.Fatalf("%s %dx%d: `r` was expected to raise the reply box", c.name, c.w, c.h)
		}
		rows := strings.Split(rendered, "\n")
		top := bodyTop + m.replyBox.top
		last := top + m.replyBox.h - 1
		if last >= len(rows) {
			t.Fatalf("%s %dx%d: the box runs past the frame", c.name, c.w, c.h)
		}
		// The deck's hairline is the leftmost stroke on the row above the
		// box, left of where the box begins — the box draws strokes of
		// its own, so it cannot be found by counting.
		col := -1
		for i, r := range []rune(rows[top-1]) {
			if r == '│' && i < m.replyBox.left {
				col = i
				break
			}
		}
		if col < 0 {
			t.Fatalf("%s %dx%d: no hairline above the box on row %d: %q",
				c.name, c.w, c.h, top-1, rows[top-1])
		}
		for j := top; j <= last; j++ {
			if !at(rows, j, col) {
				t.Errorf("%s %dx%d: the box draws row %d and the hairline stops above it: %q",
					c.name, c.w, c.h, j, rows[j])
			}
		}
		// It is one stroke, and it stops: nothing below the box carries it.
		if at(rows, last+1, col) {
			t.Errorf("%s %dx%d: the hairline runs past the box to row %d: %q",
				c.name, c.w, c.h, last+1, rows[last+1])
		}
	}

	// Where no box stands the hairline still stops where the columns do:
	// the deck's blank tail carries none.
	for _, c := range []struct {
		name string
		mk   func() scene
		w, h int
	}{
		{"first-session", sceneFirstSession, 80, 24},
		{"second-day", sceneSecondDay, 80, 24},
	} {
		m := sceneModel(c.mk(), c.w, c.h)
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		tail := rows[len(rows)-3] // the row above the footer's hairline
		if strings.Contains(tail, "│") {
			t.Errorf("%s %dx%d: the hairline runs into the deck's blank tail: %q",
				c.name, c.w, c.h, tail)
		}
	}
}

// ---- round 83, second-day, the one thing ----
// A stuck key is tried again once another has yielded. `chapterYield`
// walked the stuck keys once, in the order their cells are spent, and
// judged each trade against the row as it stood at that moment; a key
// that bought nothing then was never looked at again, even after a later
// yield had moved the row under it. At eighty the first session's reader
// draws its whole conversation, so `[` and `]` answer `no earlier turn`
// and `no later turn` and move no drawn cell — with colour on as well as
// off (#215, #218) — yet the footer spent twelve of its seventy-nine
// cells naming them and named no way into the session's pane. The turn
// key was tried first, beside `space unfold` at the row's head, and
// bought nothing; the unfold key then yielded and bought `/ search`; by
// then the twelve cells were `enter attach`, and the pass had gone by.
// The pass repeats while any yield is taken. Under the turn key's own
// note the key stays (#24), and a reader whose turns move keeps it
// (#215).
func TestAStuckKeyIsTriedAgainOnceAnotherHasYielded(t *testing.T) {
	// The frame is the coloured one: every press below is measured with
	// the profile a person's terminal has (#215, #218).
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	foot := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	// body is the frame above the footer: what a press has to move for
	// the key to be acting rather than writing its own refusal.
	body := func(m *Model) string {
		rows := strings.Split(m.View(), "\n")
		return strings.Join(rows[:len(rows)-1], "\n")
	}
	stand := func(mk func() scene, w, h, n int) (*Model, scene) {
		sc := mk()
		m := sceneModel(sc, w, h)
		for _, k := range canonicalKeys[:n] {
			pressKey(m, k)
			poll(m, sc)
		}
		return m, sc
	}

	// The stands of the first session's reader at eighty whose note is the
	// scroll key's, not the turn key's. (18, 20 and 21 stood here too
	// until the reader had its own cursor: canonicalKeys[17], [19] and
	// [20] are `j`s that, under readerCursorMove, step the cursor off
	// this session's one turn onto the stub below it — `turnStand` then
	// reports `cur < 0`, and `[` legitimately lands back on the turn
	// rather than refusing, which is the key acting, not a stuck key
	// answering in place.)
	for _, n := range []int{16, 17} {
		m, _ := stand(sceneFirstSession, 80, 24, n)
		if m.level < levelReader {
			t.Fatalf("first-session 80x24 after %d keys: expected the reader, got Lv%d", n, m.level)
		}
		if m.note == "no earlier turn" || m.note == "no later turn" {
			t.Fatalf("first-session 80x24 after %d keys: the stand wears the turn key's own note %q — #24 keeps the key there",
				n, m.note)
		}
		// Neither turn key acts from this stand: each writes its refusal
		// and moves no drawn cell.
		for _, key := range []string{"[", "]"} {
			m2, sc2 := stand(sceneFirstSession, 80, 24, n)
			was := body(m2)
			pressKey(m2, key)
			poll(m2, sc2)
			if body(m2) == was && m2.note != "no earlier turn" && m2.note != "no later turn" {
				t.Fatalf("first-session 80x24 after %d keys: `%s` answered %q, not a turn refusal", n, key, m2.note)
			}
			if body(m2) != was {
				t.Fatalf("first-session 80x24 after %d keys: `%s` moves a drawn cell — the key acts", n, key)
			}
		}
		got := foot(m)
		if strings.Contains(got, "[ ] turns") {
			t.Errorf("first-session 80x24 after %d keys: the row offers a turn key that refuses on both sides: %q", n, got)
		}
		if !strings.Contains(got, "enter attach") {
			t.Errorf("first-session 80x24 after %d keys: the cells the turn key spends buy no `enter attach`: %q", n, got)
		}
		// No key the row drew is lost for the trade (#216).
		for _, keep := range []string{"/ search", "esc back", "? help", "q quit"} {
			if !strings.Contains(got, keep) {
				t.Errorf("first-session 80x24 after %d keys: the trade lost `%s`: %q", n, keep, got)
			}
		}
	}

	// #24 stands: under the turn key's own note the key the note is about
	// stays on the row.
	for _, n := range []int{14, 15} {
		m, _ := stand(sceneFirstSession, 80, 24, n)
		if m.note != "no earlier turn" && m.note != "no later turn" {
			t.Fatalf("first-session 80x24 after %d keys: expected a turn key's own note, got %q", n, m.note)
		}
		if got := foot(m); !strings.Contains(got, "[ ] turns") {
			t.Errorf("first-session 80x24 after %d keys: the turn key's own note shed the key it is about: %q", n, got)
		}
	}

	// And where the turn key acts it stays: three readers whose `[` moves
	// a drawn cell keep the key at the same width.
	for _, c := range []struct {
		name string
		mk   func() scene
	}{
		{"fleet-hygiene", sceneFleetHygiene},
		{"two-tools", sceneTwoTools},
		{"many-idle", sceneManyIdle},
	} {
		m, sc := stand(c.mk, 80, 24, 13)
		if m.level < levelReader {
			t.Fatalf("%s 80x24: expected the reader, got Lv%d", c.name, m.level)
		}
		if got := foot(m); !strings.Contains(got, "[ ] turns") {
			t.Errorf("%s 80x24: a reader whose turns move lost the turn key: %q", c.name, got)
		}
		was := body(m)
		pressKey(m, "[")
		poll(m, sc)
		if body(m) == was {
			t.Errorf("%s 80x24: `[` was expected to move the reader, drew the same frame", c.name)
		}
	}
}

// TestThePageKeyBuysAStuckKeysCells pins round eighty-three's second
// finding: a row that keeps a key which refuses and sheds the key that
// just moved its cursor. `keysActGained` refuses the page key as the gain
// that buys a trade — #42 and #51 rank it lowest, a shortcut for a
// distance `j` covers — but every level now drops it where it cannot move
// (#83 at Lv1, #221 at Lv2, #200 in the reader), so a page key still on
// the row is a key that acts. Two presses into the walkthrough at 152 the
// trail's footer named `[ ] chapters`, which answers `no earlier prompt`
// and `no later prompt` on a trail of one prompt, under the note `at the
// start` — which `ctrl+u` had just written by walking the cursor.
//
// Both sides. The rank is unchanged: the page key is still the first
// fragment a row sheds for width, and it never comes back for cells the
// row did not free by shedding a key that refuses. Where the chapter key
// acts it stays.
func TestThePageKeyBuysAStuckKeysCells(t *testing.T) {
	forceASCII(t)
	at := func(sc scene, w, h, n int, extra ...string) *Model {
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
	body := func(m *Model) []string {
		rows := strings.Split(m.View(), "\n")
		return rows[:len(rows)-1]
	}
	moves := func(sc scene, w, h, n int, k string) bool {
		a, b := body(at(sc, w, h, n, k)), body(at(sc, w, h, n))
		for i := range a {
			if i < len(b) && a[i] != b[i] {
				return true
			}
		}
		return false
	}
	for _, c := range []struct {
		name    string
		scene   func() scene
		w, h, n int
	}{
		{"two-tools", sceneTwoTools, 152, 40, 9},
		{"fleet-hygiene", sceneFleetHygiene, 152, 40, 9},
	} {
		sc := c.scene()
		// The chapter key refuses on both halves, at this stand.
		for k, want := range map[string]string{"[": "no earlier prompt", "]": "no later prompt"} {
			if note := at(c.scene(), c.w, c.h, c.n, k).note; note != want {
				t.Fatalf("%s %dx%d: %q was expected to answer %q, it said %q", c.name, c.w, c.h, k, want, note)
			}
		}
		// The page key walks the cursor: a drawn row moves.
		if !moves(sc, c.w, c.h, c.n, "ctrl+d") {
			t.Fatalf("%s %dx%d: `ctrl+d` moved no drawn row", c.name, c.w, c.h)
		}
		f := foot(at(c.scene(), c.w, c.h, c.n))
		if strings.Contains(f, "[ ] chapters") {
			t.Errorf("%s %dx%d: the row keeps a chapter key that refuses: %q", c.name, c.w, c.h, f)
		}
		if !strings.Contains(f, "ctrl+d/u half page") {
			t.Errorf("%s %dx%d: the freed cells were expected to name the page key, the row is %q", c.name, c.w, c.h, f)
		}
	}
	// The rank is unchanged: on a row that sheds only for width the page
	// key is still the first fragment to go (#42, #51). One `tab` in at
	// 120 the trail's footer names the chapter key, which acts there, and
	// not the page key.
	f := foot(at(sceneTwoTools(), 120, 34, 7))
	if !strings.Contains(f, "[ ] chapters") || strings.Contains(f, "ctrl+d/u half page") {
		t.Errorf("two-tools 120x34: the page key outranked a chapter key that acts: %q", f)
	}
}

// TestTheHideKeyYieldsWhereItCannotHide pins round eighty-three's one
// thing: `x hide` on a selection the key refuses to take off the board.
// `toggleHidden` keeps what owes you an alarm and says so — `infra stays ·
// it is asking`, `· it hangs`, `· it is looping`, `· dead on the API` —
// the same answer at every width and however many times it is pressed, so
// the nine cells the key spends buy a promise the next keypress refuses.
// The keymap already drops the key outright where the fleet is one
// session ("the keys that move between sessions answer no question"),
// which is this rule one rung up, asked of the fleet rather than of the
// selection; this is #210's, #211's, #213's, #219's, #222's and #223's
// rule at the one board key they did not reach.
//
// Both sides. Where `x` can hide the selection the key stays; under its
// own refusal the key stays where it is (#24, #57); and where nothing is
// shed the key stands however stuck it is (#193).
func TestTheHideKeyYieldsWhereItCannotHide(t *testing.T) {
	forceASCII(t)
	at := func(sc scene, w, h, n int, extra ...string) *Model {
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
		name    string
		scene   func() scene
		w, h, n int
		refusal string
		gained  string
	}{
		{"two-tools", sceneTwoTools, 80, 24, 0, "infra stays · it is asking", "g grab"},
		{"two-tools", sceneTwoTools, 100, 30, 0, "infra stays · it is asking", "a ask"},
		{"two-tools", sceneTwoTools, 120, 34, 2, "infra stays · it is asking", "/ search"},
		{"alarm-storm", sceneAlarmStorm, 120, 34, 2, "infra stays · it is asking", "/ search"},
	} {
		if note := at(c.scene(), c.w, c.h, c.n, "x").note; note != c.refusal {
			t.Fatalf("%s %dx%d: `x` was expected to refuse with %q, it said %q", c.name, c.w, c.h, c.refusal, note)
		}
		f := foot(at(c.scene(), c.w, c.h, c.n))
		if strings.Contains(f, "x hide") {
			t.Errorf("%s %dx%d: the row spends nine cells on a key that answers %q: %q", c.name, c.w, c.h, c.refusal, f)
		}
		if !strings.Contains(f, c.gained) {
			t.Errorf("%s %dx%d: the freed cells were expected to name %q, the row is %q", c.name, c.w, c.h, c.gained, f)
		}
	}
	// Where `x` can hide the selection the key stays: `2 api` is running,
	// not asking, and the same frame one jump over keeps the key.
	if f := foot(at(sceneTwoTools(), 120, 34, 2, "2")); !strings.Contains(f, "x hide") {
		t.Errorf("two-tools 120x34 on a session `x` can hide: the row sheds the key: %q", f)
	}
	// Under its own refusal the key stays where the width leaves it room
	// (#24, #57): the row refusing `x` names `x hide`.
	for _, w := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		g := foot(at(sceneTwoTools(), w[0], w[1], 2, "x"))
		if !strings.Contains(g, "x hide") {
			t.Errorf("two-tools %dx%d: the row refusing `x` does not name it: %q", w[0], w[1], g)
		}
	}
	// Where nothing is shed the key stands however stuck it is (#193).
	for _, w := range [][2]int{{152, 40}, {220, 48}} {
		f := foot(at(sceneTwoTools(), w[0], w[1], 2))
		if !strings.Contains(f, "x hide") {
			t.Errorf("two-tools %dx%d: a row that sheds nothing must still name `x`: %q", w[0], w[1], f)
		}
	}
}

// r83Lv2Prefix replays the walkthrough's first n keys and then walks the
// Lv2 cursor to the bottom of the trail, the stand both keys under test
// are asked from.
func r83Lv2Prefix(sc scene, w, h, n int) *Model {
	m := sceneModel(sc, w, h)
	keys := append(append([]string{}, canonicalKeys...), "esc")
	keys = append(keys, sc.extra...)
	for _, k := range keys[:n] {
		pressKey(m, k)
		poll(m, sc)
	}
	if m.level == levelWaypoints {
		m.cursorToPresent() // the stand both keys under test are asked from
	}
	m.note = ""
	return m
}

// TestTheTrailsEndIsTheLastRowTheDeckDraws pins round eighty-three's one
// thing. At Lv2 `j` and `G` decided they had nothing to do by comparing the
// cursor against `len(TrailRows(...))` — a list that counts rows the panel
// does not draw. Where the trail's last row is a waypoint the leg's own row
// already carries, the cursor's last stand is a row short of that count:
// the index test read "not at the end", `cursorMove` stepped onto the
// undrawn row and back, and the key drew nothing and said nothing at all —
// the dead key the branch's own comment forbids ("a key that moves nothing
// says why"). On many-idle at 220 the two `test_checkout_total` waypoints
// ride on their legs' rows, so seven counted rows are six drawn ones.
//
// The question is the cursor's own — did the move move — which is the
// device `ctrl+d` two cases below already uses.
func TestTheTrailsEndIsTheLastRowTheDeckDraws(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // the frame is the one a person sees (#215, #218)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, c := range []struct {
		name string
		sc   scene
		n    int
	}{
		{"many-idle", sceneManyIdle(), 30},
		{"many-idle", sceneManyIdle(), 36},
	} {
		for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			for _, key := range []string{"j", "G"} {
				m := r83Lv2Prefix(c.sc, w, h, c.n)
				if m.level != levelWaypoints {
					t.Fatalf("%s %dx%d prefix %d: not at Lv2", c.name, w, h, c.n)
				}
				before := m.View()
				pressKey(m, key)
				poll(m, c.sc)
				if m.View() == before {
					t.Errorf("%s %dx%d prefix %d: %q at the end of the trail drew nothing and said nothing (cursor %d of %d rows)",
						c.name, w, h, c.n, key, m.cursor, len(TrailRows(m.trail, m.level)))
					continue
				}
				if !strings.Contains(m.note, "at the present") {
					t.Errorf("%s %dx%d prefix %d: %q said %q, not the present", c.name, w, h, c.n, key, m.note)
				}
			}
		}
	}
}

// TestNoLv2MoveKeyIsSilentlyDead is the rule behind it, asked of the whole
// walkthrough: from the bottom of the trail at every canonical Lv2 stand,
// of every scene and width, `j` and `G` must change the frame — a note is a
// drawn cell, and a key that changes nothing at all is the dead key.
func TestNoLv2MoveKeyIsSilentlyDead(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, sc := range allScenes() {
		keys := append(append([]string{}, canonicalKeys...), "esc")
		keys = append(keys, sc.extra...)
		// A waypoint rides on its leg's own row only where the column is
		// wide enough to carry it; below 120 it always gets a row of its
		// own, so the wide decks are where the count and the drawing can
		// part.
		for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			// One walk finds the Lv2 stands; only those are replayed.
			var stands []int
			m := sceneModel(sc, w, h)
			for n := range keys {
				if m.level == levelWaypoints && !m.showHelp && !m.searching && !m.replying {
					stands = append(stands, n)
				}
				pressKey(m, keys[n])
				poll(m, sc)
			}
			if m.level == levelWaypoints && !m.showHelp && !m.searching && !m.replying {
				stands = append(stands, len(keys))
			}
			for _, n := range stands {
				m := r83Lv2Prefix(sc, w, h, n)
				for _, key := range []string{"j", "G"} {
					m.cursorToPresent() // back to the stand, without a key
					m.note = ""
					before := m.View()
					pressKey(m, key)
					poll(m, sc)
					if m.View() == before {
						t.Errorf("%s %dx%d after %d keys: %q at the end of the trail drew nothing and said nothing (cursor %d of %d rows)",
							sc.name, w, h, n, key, m.cursor, len(TrailRows(m.trail, m.level)))
					}
				}
			}
		}
	}
}

// ---- round 84, two-tools, the one thing ----
// A stuck key goes from a row that has already shed a key that acts.
// #210's gate — a stuck key yields only where a key that acts comes back
// — was written so that "a wide footer still names what `[` and `]` are"
// (#193). On a row that has already given a key that acts up for width
// that reason is spent, and the gate was the only thing left holding a
// key that cannot move: at eighty the reader stood at 79 of 80 on
// ` space unfold · [ ] turns · esc back · ? help · q quit` under `all of
// it is on screen`, where `[` answers `no earlier turn` and `]` answers
// `no later turn` and neither moves a drawn cell, having shed `/ search`,
// `n/N`, `r reply`, `a ask` and `enter attach` — none of which could come
// back, `enter attach` being one cell too wide and `a ask` ranked under
// it (#39). The cells are given up, not spent: nothing comes back, so
// #39's rank is untouched.
func TestAStuckKeyGoesFromARowAlreadyShed(t *testing.T) {
	forceASCII(t)
	walk := []string{"r", "1", "/", "pytest", "enter", "esc", "tab", "ctrl+u", "ctrl+u", "[", "]", "G", "tab", "[", "]", "k"}
	foot := func(t *testing.T, w, h int, keys []string) string {
		t.Helper()
		sc := sceneTwoTools()
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	// The reader at eighty, on a page all on screen with one turn: the
	// stuck chapter key goes and no key comes back.
	if got := foot(t, 80, 24, walk); strings.Contains(got, "[ ] turns") {
		t.Errorf("80x24 reader: the row keeps a turn key that refuses both halves: %q", strings.TrimSpace(got))
	} else if !strings.HasSuffix(got, "all of it is on screen") || !strings.Contains(got, "space unfold") {
		t.Errorf("80x24 reader: not the stand this pins: %q", strings.TrimSpace(got))
	}
	// The live list at a hundred, on an asking session `x` cannot hide,
	// with `a ask` and `/ search` already shed: the hide key goes.
	if got := foot(t, 100, 30, walk[:6]); strings.Contains(got, "x hide") {
		t.Errorf("100x30 list: the row keeps a hide key that answers %q: %q",
			"infra stays · it is asking", strings.TrimSpace(got))
	} else if !strings.HasSuffix(got, "search cleared") || !strings.Contains(got, "g grab") {
		t.Errorf("100x30 list: not the stand this pins: %q", strings.TrimSpace(got))
	}
	// The reader at 120 under `❯ 1/1`: the walk keys refuse with no
	// search to walk (#223), and the cells buy `h/l session`, which moves
	// 21 drawn rows from this stand.
	if got := foot(t, 120, 34, walk[:14]); strings.Contains(got, "n/N") {
		t.Errorf("120x34 reader: the row keeps a walk key with no search to walk: %q", strings.TrimSpace(got))
	} else if !strings.Contains(got, " · h/l session") {
		t.Errorf("120x34 reader: the freed cells were expected to name a key that acts: %q", strings.TrimSpace(got))
	}
	// The other side, #193: at 220 nothing is shed, so the stuck walk key
	// stands and the footer still names what `n` and `N` are.
	if got := foot(t, 220, 48, walk[:14]); !strings.Contains(got, " · n/N") {
		t.Errorf("220x48 reader: an unshed row dropped a stuck key: %q", strings.TrimSpace(got))
	}
}

// ---- round 84, second-day, the one thing ----
// The attach refusal goes before the search that acts. #206 taught the
// reader's and the waypoints' shed orders that "an attach that cannot
// work is a refusal, and a refusal goes before a key that acts", and
// moved `enter · no pane` above `a ask`; it was left ranked *below*
// `/ search` and `n/N` in all three orders, and `shedKeys` lets a key
// back only when every key ranked above it came back too. So the
// archive's own list at eighty — twelve finished sessions, four of them
// on screen and `▾ 8 more below · j` — spent eighteen of its
// seventy-six cells on a key that answers `attach needs a pane` at every
// width and named no way to search the archive, while `/` there filters
// the twelve to two and the header says `archive 2 of 12`. The
// eighteen cells buy `· / search` (eleven) with seven to spare.
//
// Both sides: where the row's own `enter` says `enter attach` nothing
// moves, and where the field is wide enough for both the refusal stays.
func TestTheAttachRefusalGoesBeforeTheSearchKey(t *testing.T) {
	// The frame is the coloured one (#215, #218): every press below is
	// measured with the profile a person's terminal has.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	footer := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	// body is the frame without its footer: a note lands there on every
	// press, so the body is what says whether a key moved a drawn cell.
	body := func(m *Model) string {
		rows := strings.Split(m.View(), "\n")
		return strings.Join(rows[:len(rows)-1], "\n")
	}
	stand := func(mk func() scene, w, h int, keys []string) (*Model, scene) {
		sc := mk()
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		return m, sc
	}

	for _, c := range []struct {
		name string
		mk   func() scene
		keys []string
		w, h int
		kept []string
	}{
		{"second-day archive list", sceneSecondDay, []string{"A"}, 80, 24,
			[]string{"j/k move", "tab deeper", "a ask", "A fleet", "? help", "q quit"}},
		{"second-day archive list, one row on", sceneSecondDay, []string{"A", "j"}, 80, 24,
			[]string{"j/k move", "tab deeper", "a ask", "A fleet", "? help", "q quit"}},
		{"second-day archive reader", sceneSecondDay, []string{"A", "8", "tab", "tab"}, 100, 30,
			[]string{"space unfold", "[ ] turns", "a ask", "esc back", "A fleet", "? help", "q quit"}},
		{"fleet-hygiene paneless reader", sceneFleetHygiene, []string{"3", "tab", "tab"}, 100, 30,
			[]string{"space unfold", "[ ] turns", "a ask", "esc back", "? help", "q quit"}},
	} {
		m, sc := stand(c.mk, c.w, c.h, c.keys)
		if m.enterKeymap() != "enter · no pane" {
			t.Fatalf("%s %dx%d: the row can be attached to: %q", c.name, c.w, c.h, m.enterKeymap())
		}
		// The refusal refuses: it moves no drawn cell of the body, with
		// colour on, and answers in words.
		before := body(m)
		e, se := stand(c.mk, c.w, c.h, append(append([]string(nil), c.keys...), "enter"))
		_ = se
		if body(e) != before {
			t.Errorf("%s %dx%d: `enter` moves a drawn cell — it is not a refusal here", c.name, c.w, c.h)
		}
		if e.note == "" {
			t.Errorf("%s %dx%d: `enter` answers nothing", c.name, c.w, c.h)
		}
		// The search acts from this very stand.
		s, _ := stand(c.mk, c.w, c.h, append(append([]string(nil), c.keys...), "/"))
		if body(s) == before && footer(s) == footer(m) {
			t.Errorf("%s %dx%d: `/` changes nothing — it is not a key that acts here", c.name, c.w, c.h)
		}
		foot := footer(m)
		if strings.Contains(foot, "enter · no pane") {
			t.Errorf("%s %dx%d: the row holds eighteen cells for a key that cannot work: %q",
				c.name, c.w, c.h, foot)
		}
		if !strings.Contains(foot, "/ search") {
			t.Errorf("%s %dx%d: the row names no way to search: %q", c.name, c.w, c.h, foot)
		}
		for _, k := range c.kept {
			if !strings.Contains(foot, k) {
				t.Errorf("%s %dx%d: the trade cost the row %q: %q", c.name, c.w, c.h, k, foot)
			}
		}
		if w := lipgloss.Width(foot); w > c.w-1 {
			t.Errorf("%s %dx%d: the footer overruns its field: %d cells", c.name, c.w, c.h, w)
		}
		_ = sc
	}

	// The rank, stated, at all three levels the refusal is offered at:
	// the refusal sheds before `/ search` and before `n/N`.
	for _, lv := range []struct {
		name string
		keys []string
		w, h int
	}{
		{"the archive's list", []string{"A"}, 80, 24},
		{"the waypoints", []string{"A", "8", "tab"}, 100, 30},
		{"the reader", []string{"A", "8", "tab", "tab"}, 100, 30},
	} {
		m, _ := stand(sceneSecondDay, lv.w, lv.h, lv.keys)
		order := m.shedOrder(false)
		at := func(frag string) int {
			for i, f := range order {
				if f == frag {
					return i
				}
			}
			return -1
		}
		refusal, search, walk := at(" · enter · no pane"), at(" · / search"), at(" · n/N")
		if refusal < 0 || search < 0 || walk < 0 {
			t.Fatalf("%s: shed order names %d/%d/%d", lv.name, refusal, search, walk)
		}
		if refusal > search || refusal > walk {
			t.Errorf("%s sheds a key that acts before the attach refusal: refusal %d, / search %d, n/N %d",
				lv.name, refusal, search, walk)
		}
	}

	// The other side. Where the row's own `enter` attaches, the keymap is
	// unchanged: the live list at eighty still names `enter attach`.
	m, _ := stand(sceneSecondDay, 80, 24, nil)
	if foot := footer(m); !strings.Contains(foot, "enter attach") {
		t.Errorf("the live list lost its attach key: %q", foot)
	}
	// And where the field holds both, the refusal stays: the same archive
	// list at 120 names the refusal and the search.
	w120, _ := stand(sceneSecondDay, 120, 34, []string{"A"})
	foot := footer(w120)
	if !strings.Contains(foot, "enter · no pane") || !strings.Contains(foot, "/ search") {
		t.Errorf("at 120 the archive's row should name both: %q", foot)
	}
}

// r84fhReaderStand replays the first n keys of a scene's own walkthrough.
func r84fhReaderStand(sc scene, w, h, n int) *Model {
	m := sceneModel(sc, w, h)
	keys := append(append([]string{}, canonicalKeys...), "esc")
	keys = append(keys, sc.extra...)
	for _, k := range keys[:n] {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// TestTheReadersPresentIsTheEndOfTheConversation pins round eighty-four's
// one thing. #228 gave `j` and `G` at Lv2 the question the move's own —
// did the move move — because an index compare had let the key draw
// nothing and say nothing. In the reader `G` still threw its result away:
//
//	case "G":
//	    m.scrollBy(1 << 30) // clamped to the last screenful
//
// `scrollBy` reports whether the page moved, which is exactly how
// `ctrl+d` two cases up reaches `end of the conversation`. On a page
// already showing the last screenful — where `tab` from a trail standing
// at the present lands — `G` returned a byte-identical frame with an
// empty note, while `j` and `ctrl+d`, which go to the same place a row
// and a half-page at a time, both answered `end of the conversation`.
// The key whose whole definition is "back to the present: the newest
// row" was the one key on that page that said nothing.
func TestTheReadersPresentIsTheEndOfTheConversation(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // the frame is the one a person sees (#215, #218)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, c := range []struct {
		name string
		sc   scene
		n    int
	}{
		{"many-idle", sceneManyIdle(), 13},
		{"many-idle", sceneManyIdle(), 33},
		{"very-long", sceneVeryLong(), 13},
	} {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}} {
			w, h := size[0], size[1]
			m := r84fhReaderStand(c.sc, w, h, c.n)
			if m.level != levelReader {
				t.Fatalf("%s %dx%d prefix %d: not in the reader", c.name, w, h, c.n)
			}
			m.note = ""
			before := m.View()
			pressKey(m, "G")
			poll(m, c.sc)
			if m.View() == before {
				t.Errorf("%s %dx%d prefix %d: %q on the last page drew nothing and said nothing",
					c.name, w, h, c.n, "G")
				continue
			}
			if m.note != "end of the conversation" && m.note == "" {
				t.Errorf("%s %dx%d prefix %d: G moved the page and said %q", c.name, w, h, c.n, m.note)
			}
		}
	}
}

// TestNoReaderPageKeyIsSilentlyDead is the rule behind it, asked of every
// canonical reader stand of every scene at every width: `G` must change
// the frame — a note is a drawn cell, and a key that changes nothing at
// all is the dead key SPEC's round-one rule bans.
func TestNoReaderPageKeyIsSilentlyDead(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, sc := range allScenes() {
		keys := append(append([]string{}, canonicalKeys...), "esc")
		keys = append(keys, sc.extra...)
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			// One walk finds the reader's stands; only those are replayed.
			var stands []int
			m := sceneModel(sc, w, h)
			for n := range keys {
				if m.level == levelReader && !m.showHelp && !m.searching && !m.replying {
					stands = append(stands, n)
				}
				pressKey(m, keys[n])
				poll(m, sc)
			}
			if m.level == levelReader && !m.showHelp && !m.searching && !m.replying {
				stands = append(stands, len(keys))
			}
			for _, n := range stands {
				m := r84fhReaderStand(sc, w, h, n)
				m.note = ""
				before := m.View()
				pressKey(m, "G")
				poll(m, sc)
				if m.View() == before {
					t.Errorf("%s %dx%d after %d keys: \"G\" in the reader drew nothing and said nothing", sc.name, w, h, n)
				}
			}
		}
	}
}

// r84fhListStand replays the first n keys of a scene's own walkthrough.
func r84fhListStand(sc scene, w, h, n int) *Model {
	m := sceneModel(sc, w, h)
	keys := append(append([]string{}, canonicalKeys...), "esc")
	keys = append(keys, sc.extra...)
	for _, k := range keys[:n] {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// r84fhFooterRow is the last drawn row of a frame, with the colour off.
func r84fhFooterRow(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	for i := len(rows) - 1; i >= 0; i-- {
		if strings.TrimSpace(rows[i]) != "" {
			return rows[i]
		}
	}
	return ""
}

// TestTheAttachRefusalYieldsToTheNoteThatSaysIt pins round eighty-four's
// second finding. On a paneless session the footer stands with
// `enter · no pane` — what Enter does here, since an attach that cannot
// work is not promised (#40). Press Enter and the note is `attach needs a
// pane`: the same sentence, twelve cells to the right, and the row pays
// for the repetition with keys that act — `/ search` and `x hide` at
// eighty, `a ask` at a hundred. #165 gave the note the key's own name, in
// the form `mirror needs 110 columns` already used, precisely so naming
// the key would buy a key back; the clause beside it buys nothing. A
// refusal goes before a key that acts (#52, #198, #206), so under the note
// that says it the clause is shed first — and, like every yield, only
// where a key that acts comes back (#193, #216).
func TestTheAttachRefusalYieldsToTheNoteThatSaysIt(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // the frame is the one a person sees (#215, #218)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	sc := sceneFleetHygiene()
	for _, c := range []struct {
		w, h  int
		gains string
	}{
		{80, 24, "x hide"},
		{100, 30, "a ask"},
	} {
		// The walkthrough's own stand on the paneless session, one key
		// before its `enter`.
		m := r84fhListStand(sc, c.w, c.h, 28)
		if m.level != levelTrail || !strings.Contains(r84fhFooterRow(m), "enter · no pane") {
			t.Fatalf("%dx%d: not the paneless list stand: %q", c.w, c.h, r84fhFooterRow(m))
		}
		pressKey(m, "enter")
		poll(m, sc)
		foot := r84fhFooterRow(m)
		if m.note != "attach needs a pane" {
			t.Fatalf("%dx%d: enter said %q", c.w, c.h, m.note)
		}
		if strings.Contains(foot, "enter · no pane") {
			t.Errorf("%dx%d: the row says the refusal twice — %q beside `enter · no pane`:\n%s", c.w, c.h, m.note, foot)
		}
		if !strings.Contains(foot, c.gains) {
			t.Errorf("%dx%d: the cells bought back no key that acts (wanted %q):\n%s", c.w, c.h, c.gains, foot)
		}
		if !strings.Contains(foot, "attach needs a pane") {
			t.Errorf("%dx%d: the refusal went and the note did not stand:\n%s", c.w, c.h, foot)
		}
	}
}

// TestTheAttachRefusalStandsWithNoNoteToSayIt is the other side of the same
// rule, and the reason the clause exists: with no note on the row — and
// under a note about another key that says nothing about the pane — the
// paneless session's footer still says what Enter does. Only a note that
// says what the clause says takes its cells, and the reply refusal is the
// other note that does (#165, #232, #299): `reply needs a pane` and `enter
// · no pane` are one fact — this session has no pane — so the clause goes
// there too, and a chapter key's refusal, which never mentions the pane,
// leaves it standing.
func TestTheAttachRefusalStandsWithNoNoteToSayIt(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	sc := sceneFleetHygiene()
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		w, h := size[0], size[1]
		m := r84fhListStand(sc, w, h, 28)
		if m.level != levelTrail {
			continue // the wide decks stand on the board here, with no list row
		}
		if !strings.Contains(r84fhFooterRow(m), "enter · no pane") {
			t.Errorf("%dx%d: with no note the paneless row does not say what enter does:\n%s", w, h, r84fhFooterRow(m))
		}
		stand := r84fhListStand(sc, w, h, 28)
		pressKey(stand, "[") // another key's refusal, and not about the pane
		poll(stand, sc)
		if !strings.HasPrefix(stand.note, "no earlier") {
			t.Fatalf("%dx%d: [ said %q", w, h, stand.note)
		}
		if !strings.Contains(r84fhFooterRow(stand), "enter · no pane") {
			t.Errorf("%dx%d: another key's note took the attach refusal's cells:\n%s", w, h, r84fhFooterRow(stand))
		}
		pressKey(m, "r") // the other no-pane refusal: it says what the clause says
		poll(m, sc)
		if m.note != "reply needs a pane" {
			t.Fatalf("%dx%d: r said %q", w, h, m.note)
		}
		if strings.Contains(r84fhFooterRow(m), "enter · no pane") {
			t.Errorf("%dx%d: the row says the same fact twice — %q beside `enter · no pane`:\n%s", w, h, m.note, r84fhFooterRow(m))
		}
	}
}

// Round eighty-five, the two-tools operator's one thing: the trace note
// leaves the destination to the row that draws it.
//
// #128 gave the note the destination because "the row answers what, the
// note answers where" — and `noteLeavesTheQuoteToTheRow` then trimmed the
// row's right-aligned pane clause off before comparing, on the stated
// reason that "the note's destination clause is never on the row". On the
// board that reason is false: where two sessions share a tmux session the
// card's trace row draws the full target, so at 120 on two-tools the
// frame drew `⌁ dev:2.0` on the header, on the row the trace is about and
// in the note, and the note's clause cost the footer `a ask`. Where a
// drawn row carries the destination in the note's own form the note keeps
// only its verb (#186, #205's condition); where no row does, the clause
// stays and proves where the line landed (#39, #128).
func TestTheTraceNoteLeavesItsDestinationToTheRow(t *testing.T) {
	scene := func(name string) scene {
		for _, sc := range allScenes() {
			if sc.name == name {
				return sc
			}
		}
		t.Fatalf("no scene %q", name)
		return scene{}
	}
	frame := func(name string, w, h, n int) (rows []string, footer string) {
		sc := scene(name)
		m := sceneModel(sc, w, h)
		for _, k := range canonicalKeys[:n] {
			pressKey(m, k)
			poll(m, sc)
		}
		for _, l := range strings.Split(m.View(), "\n") {
			rows = append(rows, ansi.Strip(l))
		}
		return rows, rows[len(rows)-1]
	}
	// The walkthrough's twenty-ninth key sends the typed line; the board
	// then draws the trace on the card of the session it went to.
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		rows, foot := frame("two-tools", 120, 34, 29)
		drawn := false
		for i, r := range rows[1 : len(rows)-1] {
			if strings.Contains(r, "↪ sent") && strings.Contains(r, "⌁ dev:2.0") {
				drawn = true
				_ = i
			}
		}
		if !drawn {
			t.Fatalf("two-tools 120: no drawn row carries the trace and its destination")
		}
		if strings.Contains(foot, "⌁ dev:2.0") {
			t.Errorf("two-tools 120 (%v): the note points at a pane the row beside it draws: %q", prof, foot)
		}
		if !strings.Contains(foot, "↪ sent") {
			t.Errorf("two-tools 120 (%v): the note no longer says the line went: %q", prof, foot)
		}
		if !strings.Contains(foot, " · a ask · ") {
			t.Errorf("two-tools 120 (%v): the cells the clause held did not buy `a ask`: %q", prof, foot)
		}
		lipgloss.SetColorProfile(old)
	}
	// The same rule one fleet over: the cells buy the hide key.
	if _, foot := frame("alarm-storm", 120, 34, 29); strings.Contains(foot, "⌁ work:3.0") || !strings.Contains(foot, " · x hide · ") {
		t.Errorf("alarm-storm 120: %q", foot)
	}
	// Held: where no drawn row carries the destination the note keeps it.
	// At a hundred the list row draws the trace without its pane.
	rows, foot := frame("two-tools", 100, 30, 29)
	for _, r := range rows[1 : len(rows)-1] {
		if strings.Contains(r, "↪ sent") && strings.Contains(r, "⌁ dev:2.0") {
			t.Fatalf("two-tools 100: a row does carry the destination: %q", r)
		}
	}
	if !strings.Contains(foot, "↪ sent to ⌁ dev:2.0") {
		t.Errorf("two-tools 100: the note gave up the one clause that says where the line went: %q", foot)
	}
	// And at eighty on second-day, where the pane is on no row at all.
	if _, foot := frame("second-day", 80, 24, 29); !strings.Contains(foot, "⌁ main:0.0") {
		t.Errorf("second-day 80: %q", foot)
	}
}

// r85sdArchiveHeadings reads the archive's fleet column off a drawn frame:
// the group headings in the order they are drawn, and, for each session row
// on screen, the heading it stands under. The fleet column is everything
// left of the deck's hairline; a heading is a row indented two cells that
// carries no session mark, a session row carries `○` after its digit.
func r85sdArchiveHeadings(m *Model) (headings []string, under map[string]string) {
	under = map[string]string{}
	cur := ""
	for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
		col := row
		if i := strings.Index(row, "│"); i >= 0 {
			col = row[:i]
		}
		t := strings.TrimRight(col, " ")
		if t == "" || !strings.HasPrefix(t, "  ") {
			continue
		}
		body := strings.TrimSpace(t)
		switch {
		case strings.Contains(body, "○"):
			// a session row: "3 ○ the checkout suite flakes on CI   4h"
			if i := strings.Index(body, "○"); i >= 0 {
				title := strings.TrimSpace(body[i+len("○"):])
				under[title] = cur
			}
		case strings.HasPrefix(t, "     "), strings.HasPrefix(body, "▾"), strings.HasPrefix(body, "▴"),
			strings.HasPrefix(body, "FLEET"), strings.HasPrefix(body, "▌"):
			// a tag line, a fold mark or the panel's own title
		default:
			cur = body
			headings = append(headings, body)
		}
	}
	return headings, under
}

// TestTheArchiveGroupsByProjectNotByTheNameItsPersonGave pins the archive's
// bucket to the directory the session was started in.
//
// #11: "Archive groups by project directory, newest first — I start
// sessions in the respective directory." #79 gave the row, the header and
// the band the name its person typed into `/rename`. `archiveGroups`
// bucketed on `sessionName`, which is that name where one was given, so the
// one renamed session of the second day left `webapp`'s bucket and took a
// heading of its own: at 220 the archive drew `checkout-flake-hunt` over
// one session and `webapp` over the other, ten rows and six other projects
// apart, for one directory's history. The bucket is the project; the name
// still stands on the strip, in the header and on the band, which is where
// #79 put it.
func TestTheArchiveGroupsByProjectNotByTheNameItsPersonGave(t *testing.T) {
	const (
		renamed = "the checkout suite flakes on CI"            // /home/user/webapp, renamed checkout-flake-hunt
		sibling = "why does the nightly build take 40 minutes" // /home/user/webapp, never renamed
		project = "webapp"
		name    = "checkout-flake-hunt"
	)
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, prof := range []struct {
		what string
		p    termenv.Profile
	}{{"forceASCII", termenv.Ascii}, {"colour on", termenv.TrueColor}} {
		lipgloss.SetColorProfile(prof.p)
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			where := func(s string) string { return prof.what + " " + s }

			// The archive, as `A` draws it.
			sc := sceneSecondDay()
			m := sceneModel(sc, w, h)
			pressKey(m, "A")
			poll(m, sc)
			headings, under := r85sdArchiveHeadings(m)
			for _, hd := range headings {
				if hd == name {
					t.Errorf("%s %dx%d: the archive heads a group with the name its person gave a session, not the directory it ran in: %v",
						where("archive"), w, h, headings)
				}
			}
			// The title is clipped at the narrow widths and carries its
			// age behind it, so the row is found on the head of its
			// prompt — twenty-one cells, inside eighty's own clip — or,
			// since the row keeps the name its person gave it before that
			// prompt, on the name.
			seenRenamed := false
			for title, hd := range under {
				if !strings.HasPrefix(title, renamed[:21]) && !strings.HasPrefix(title, name) {
					continue
				}
				seenRenamed = true
				if hd != project {
					t.Errorf("%s %dx%d: the renamed session stands under %q, the name its person gave it, not its project %q",
						where("archive"), w, h, hd, project)
				}
			}
			if !seenRenamed {
				t.Errorf("%s %dx%d: the archive's first frame does not draw the renamed session at all", where("archive"), w, h)
			}

			// Both of the project's sessions under one heading, nothing
			// between them: walk down from the renamed row to the sibling.
			m2 := sceneModel(sc, w, h)
			pressKey(m2, "A")
			poll(m2, sc)
			for i := 0; i < 14; i++ {
				hs, un := r85sdArchiveHeadings(m2)
				gotBoth := 0
				for title, hd := range un {
					if hd == project && title != "" {
						gotBoth++
					}
				}
				if gotBoth >= 2 {
					break
				}
				_ = hs
				pressKey(m2, "j")
				poll(m2, sc)
			}
			_, un := r85sdArchiveHeadings(m2)
			seen := 0
			for title, hd := range un {
				if hd == project {
					seen++
					_ = title
				}
			}
			if seen < 2 {
				t.Errorf("%s %dx%d: the project's two sessions never stand under one heading — %v",
					where("archive"), w, h, un)
			}

			// The other side: the name its person gave is still on the
			// board's recent strip and in the header that names the
			// selection (#79). Neither is the group's heading.
			m3 := sceneModel(sc, w, h)
			if !strings.Contains(ansi.Strip(m3.View()), name) {
				t.Errorf("%s %dx%d: the opening frame no longer names the renamed session %q", where("strip"), w, h, name)
			}
			m4 := sceneModel(sc, w, h)
			pressKey(m4, "A")
			poll(m4, sc)
			found := false
			for i := 0; i < 14 && !found; i++ {
				head := strings.SplitN(ansi.Strip(m4.View()), "\n", 2)[0]
				if strings.Contains(head, name) {
					found = true
					break
				}
				pressKey(m4, "j")
				poll(m4, sc)
			}
			if !found {
				t.Errorf("%s %dx%d: no archive stand names the renamed session %q in its header", where("header"), w, h, name)
			}

			// And the sibling is still drawn: nothing was grouped away.
			m5 := sceneModel(sc, w, h)
			pressKey(m5, "A")
			poll(m5, sc)
			ok := false
			for i := 0; i < 16 && !ok; i++ {
				if strings.Contains(ansi.Strip(m5.View()), sibling[:22]) {
					ok = true
					break
				}
				pressKey(m5, "j")
				poll(m5, sc)
			}
			if !ok {
				t.Errorf("%s %dx%d: the project's other session never appears", where("archive"), w, h)
			}
		}
	}
}

// TestTheSearchWalkSaysWhereItLanded pins round eighty-five's one thing.
// The reader's footer names `/ search · n/N` together — the key that
// starts a search and the pair that walks it — and #223 gave the pair its
// own refusal, `no search — / starts one`, for the row where no search
// stands. Start the search the row names and the pair goes silent
// instead: `jumpMatch` set `m.scroll` and asked nothing, so on a
// conversation the reader draws whole `clampScroll` pinned the page to
// nought and the press drew nothing and said nothing — the dead key
// SPEC's round-one rule bans and the rule #228 and #231 folded at `j` and
// at `G`; every one of the 1,901 silent presses stood on a page the match
// was already drawn on
// (#20). The walk asks the page whether it moved, as `ctrl+d` and `G`
// already do, and where it did not the row says the match is on screen.
func TestTheSearchWalkSaysWhereItLanded(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // the frame is the one a person sees (#215, #218)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, c := range []struct {
		name string
		sc   scene
		n    int
	}{
		{"fleet-hygiene", sceneFleetHygiene(), 13},
		{"fleet-hygiene", sceneFleetHygiene(), 15},
		{"many-idle", sceneManyIdle(), 13},
		{"many-idle", sceneManyIdle(), 33},
	} {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}} {
			w, h := size[0], size[1]
			m := r85fhReaderSearch(c.sc, w, h, c.n, "pytest")
			if m == nil {
				t.Fatalf("%s %dx%d prefix %d: no reader stand with a search to walk", c.name, w, h, c.n)
			}
			for _, k := range []string{"n", "n", "N"} {
				m.note = ""
				before := m.View()
				pressKey(m, k)
				poll(m, c.sc)
				if m.View() == before {
					t.Errorf("%s %dx%d prefix %d: %q on a running search drew nothing and said nothing",
						c.name, w, h, c.n, k)
					continue
				}
				if m.note != "" && !strings.HasPrefix(m.note, "match ") {
					t.Errorf("%s %dx%d prefix %d: %q on a running search said %q",
						c.name, w, h, c.n, k, m.note)
				}
			}
		}
	}
}

// TestNoReaderWalkKeyIsSilentlyDead is the rule behind it, asked of every
// canonical reader stand of every scene at every width: with a search
// running, `n` and `N` must change the frame — a note is a drawn cell,
// and a key that changes nothing at all is the dead key round one bans.
func TestNoReaderWalkKeyIsSilentlyDead(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, sc := range allScenes() {
		keys := append(append([]string{}, canonicalKeys...), "esc")
		keys = append(keys, sc.extra...)
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			// One walk finds the reader's stands; only those are replayed.
			var stands []int
			m := sceneModel(sc, w, h)
			for n := range keys {
				if m.level == levelReader && !m.showHelp && !m.searching && !m.replying {
					stands = append(stands, n)
				}
				pressKey(m, keys[n])
				poll(m, sc)
			}
			if m.level == levelReader && !m.showHelp && !m.searching && !m.replying {
				stands = append(stands, len(keys))
			}
			for _, n := range stands {
				mm := r85fhReaderSearch(sc, w, h, n, "pytest")
				if mm == nil {
					continue // nothing to walk: #223's own case
				}
				for _, k := range []string{"n", "N"} {
					mm.note = ""
					before := mm.View()
					pressKey(mm, k)
					poll(mm, sc)
					if mm.View() == before {
						t.Errorf("%s %dx%d after %d keys: %q on a running search drew nothing and said nothing",
							sc.name, w, h, n, k)
					}
				}
			}
		}
	}
}

// r85fhReaderSearch replays the first n keys of a scene's own walkthrough
// and then presses the two keys the reader's own footer names — `/`, the
// query, enter — returning nil where the search finds nothing to walk.
func r85fhReaderSearch(sc scene, w, h, n int, query string) *Model {
	m := sceneModel(sc, w, h)
	keys := append(append([]string{}, canonicalKeys...), "esc")
	keys = append(keys, sc.extra...)
	if n > len(keys) {
		n = len(keys)
	}
	for _, k := range keys[:n] {
		pressKey(m, k)
		poll(m, sc)
	}
	if m.level != levelReader || m.showHelp || m.searching || m.replying {
		return nil
	}
	pressKey(m, "/")
	for _, r := range query {
		pressKey(m, string(r))
	}
	pressKey(m, "enter")
	poll(m, sc)
	if m.query == "" || len(readerMatches(m.doc(m.readerWidth()), m.query)) == 0 {
		return nil
	}
	return m
}

// TestTheSearchWalkNamesTheMatchItLandedOn is the other half of #235.
// #235 gave `n` and `N` the question `ctrl+d` and `G` already ask — did
// the page move — and the answer for where it did not. Where it *does*
// move, the page still goes somewhere the person did not choose by hand
// and nothing on it names the match: over every canonical reader stand of
// every scene at every width, after the row's own `/` search, 675 presses
// moved the page with an empty note. That is the harm `landOnTurn` was
// written for at the other key that jumps — "`[` and `]` moved the page
// before, and nothing on the page said what they had moved to" (#20) —
// whose remedy there is the count `❯ 1/1 · "…"`. The walk's own form is
// `match 2/7`: no quote, because the row the page opens on is the match.
func TestTheSearchWalkNamesTheMatchItLandedOn(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // the frame is the one a person sees (#215, #218)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, c := range []struct {
		name string
		sc   scene
		n    int
	}{
		{"many-idle", sceneManyIdle(), 13},
		{"many-idle", sceneManyIdle(), 33},
		{"very-long", sceneVeryLong(), 13},
	} {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}} {
			w, h := size[0], size[1]
			m := r85fhReaderSearch(c.sc, w, h, c.n, "pytest")
			if m == nil {
				continue
			}
			for _, k := range []string{"n", "n", "N"} {
				before := r85fhReaderPage(m)
				m.note = ""
				pressKey(m, k)
				poll(m, c.sc)
				if r85fhReaderPage(m) == before {
					continue // #235's own case: the match is on screen
				}
				if !strings.HasPrefix(m.note, "match ") {
					t.Errorf("%s %dx%d prefix %d: %q moved the page and said %q",
						c.name, w, h, c.n, k, m.note)
				}
			}
		}
	}
}

// TestNoReaderWalkKeyMovesInSilence is the rule behind it, asked of every
// canonical reader stand of every scene at every width: a walk that moves
// the page says which match of how many it went to.
func TestNoReaderWalkKeyMovesInSilence(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, sc := range allScenes() {
		keys := append(append([]string{}, canonicalKeys...), "esc")
		keys = append(keys, sc.extra...)
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			var stands []int
			m := sceneModel(sc, w, h)
			for n := range keys {
				if m.level == levelReader && !m.showHelp && !m.searching && !m.replying {
					stands = append(stands, n)
				}
				pressKey(m, keys[n])
				poll(m, sc)
			}
			for _, n := range stands {
				mm := r85fhReaderSearch(sc, w, h, n, "pytest")
				if mm == nil {
					continue
				}
				for _, k := range []string{"n", "N"} {
					before := r85fhReaderPage(mm)
					mm.note = ""
					pressKey(mm, k)
					poll(mm, sc)
					if r85fhReaderPage(mm) != before && mm.note == "" {
						t.Errorf("%s %dx%d after %d keys: %q moved the page and said nothing",
							sc.name, w, h, n, k)
					}
				}
			}
		}
	}
}

// r85fhReaderPage is the frame without its footer: the note is a drawn
// cell, so a frame compare would call a note the page moving.
func r85fhReaderPage(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	for i := len(rows) - 1; i >= 0; i-- {
		if strings.TrimSpace(rows[i]) != "" {
			return strings.Join(rows[:i], "\n")
		}
	}
	return ""
}

// r86sdArchiveRows reads the archive's fleet column off a drawn frame:
// the heading each session row stands under, and the row's own title —
// everything the row draws right of its state glyph.
func r86sdArchiveRows(m *Model) (under map[string]string) {
	under = map[string]string{}
	cur := ""
	for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
		col := row
		if i := strings.Index(row, "│"); i >= 0 {
			col = row[:i]
		}
		t := strings.TrimRight(col, " ")
		if t == "" || !strings.HasPrefix(t, "  ") {
			continue
		}
		body := strings.TrimSpace(t)
		switch {
		case strings.Contains(body, "○"):
			if i := strings.Index(body, "○"); i >= 0 {
				under[strings.TrimSpace(body[i+len("○"):])] = cur
			}
		case strings.HasPrefix(t, "     "), strings.HasPrefix(body, "▾"), strings.HasPrefix(body, "▴"),
			strings.HasPrefix(body, "FLEET"), strings.HasPrefix(body, "▌"):
			// a tag line, a fold mark or the panel's own title
		default:
			cur = body
		}
	}
	return under
}

// TestTheArchiveRowKeepsTheNameItsPersonGave pins the archive's list row
// to the name its person typed into `/rename`.
//
// #79 gives that name four places — "the fleet row, the header, the band,
// and the archive, where a renamed session keeps its name before its
// prompt". The archive's row was the one that leaned on its heading for
// it: `fleetlist.go` spent the row on the ask because "an archived session
// is named after its project", which a renamed one is not. #234 made the
// heading the project, as #11 asks, and with it went the last place the
// archive said `checkout-flake-hunt`: at eighty the board's band draws
// ` 3 ○ checkout-flake-hunt…   ✗ 4h` and one `A` later the same digit
// draws ` 3 ○ the checkout suite fl…   4h`, under `webapp`.
//
// The row keeps its name before its prompt, in the form the archive
// already draws for a hidden live session one branch above
// (`1 ○ billing · "reconcile the inv…   3h`) and the form the header and
// the trail title beside it already use. The other side is pinned too:
// the bucket is still the project (#234, #11), a row nobody renamed is
// still titled by what it asked for (#56, #59), and the band still names
// the session it always did (#47, #79).
func TestTheArchiveRowKeepsTheNameItsPersonGave(t *testing.T) {
	const (
		name    = "checkout-flake-hunt"
		renamed = "the checkout suite flakes on CI"            // /home/user/webapp, renamed
		sibling = "why does the nightly build take 40 minutes" // /home/user/webapp, never renamed
		project = "webapp"
	)
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, prof := range []struct {
		what string
		p    termenv.Profile
	}{{"forceASCII", termenv.Ascii}, {"colour on", termenv.TrueColor}} {
		lipgloss.SetColorProfile(prof.p)
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			where := prof.what + " " + "archive"

			sc := sceneSecondDay()
			m := sceneModel(sc, w, h)
			pressKey(m, "A")
			poll(m, sc)
			rows := r86sdArchiveRows(m)

			// 1. The renamed session's row names it, and still stands
			//    under its project.
			named := ""
			for title, hd := range rows {
				if strings.HasPrefix(title, name) {
					named = title
					if hd != project {
						t.Errorf("%s %dx%d: the renamed row stands under %q, not its project %q",
							where, w, h, hd, project)
					}
				}
			}
			if named == "" {
				t.Errorf("%s %dx%d: no archive row names the session its person called %q — the rows are %v",
					where, w, h, name, rows)
			}

			// 2. The name comes before the prompt, never instead of the
			//    heading: no group is headed by it.
			for _, hd := range rows {
				if hd == name {
					t.Errorf("%s %dx%d: the archive heads a group with %q, the name its person gave (#234)", where, w, h, name)
				}
			}

			// 3. A row nobody renamed is still titled by what it asked
			//    for: the sibling in the same directory keeps its prompt
			//    and gains no name (#56, #59).
			seenSibling := false
			for title, hd := range rows {
				if strings.HasPrefix(title, sibling[:22]) {
					seenSibling = true
					if hd != project {
						t.Errorf("%s %dx%d: the sibling stands under %q, not %q", where, w, h, hd, project)
					}
				}
			}
			if !seenSibling && w >= 100 {
				t.Errorf("%s %dx%d: the archive's first frame no longer draws the sibling %q", where, w, h, sibling)
			}

			// 4. The other side: the board's band still names the session
			//    before its prompt, where #47 and #79 put it.
			m2 := sceneModel(sc, w, h)
			if !strings.Contains(ansi.Strip(m2.View()), name) {
				t.Errorf("%s %dx%d: the opening frame no longer names %q on its recent band", prof.what+" band", w, h, name)
			}
		}
	}
}

// Round eighty-six, the two-tools operator's one thing: the digit of the
// row you are already on says so.
//
// `1`–`9` selects session N (§3). Where N is the row the deck is already
// on, `boardSelect` found it, `point` returned at once and the handler set
// no note: the key drew nothing and said nothing — the dead key round one
// bans, and the rule #228, #231 and #235 folded for `j`, `G` and the
// search walk. The two-tools walkthrough ends on that press, so the last
// frame of all five files was byte-identical to the one before it. The
// digit now asks the selection whether it moved and says the deck's own
// sentence for the row you are on; a digit that moves, a digit no row
// wears and a hidden session's digit are untouched.
func r86ttScene(t *testing.T, name string) scene {
	t.Helper()
	for _, sc := range allScenes() {
		if sc.name == name {
			return sc
		}
	}
	t.Fatalf("no scene %q", name)
	return scene{}
}

// r86ttWalk plays a scene's whole walkthrough (canonical keys and the
// scene's own tail) minus the last n keys, and hands back the model.
func r86ttWalk(sc scene, w, h, drop int) (*Model, []string) {
	keys := canonicalKeys
	if len(sc.extra) > 0 {
		keys = append(append(append([]string(nil), keys...), "esc"), sc.extra...)
	}
	keys = keys[:len(keys)-drop]
	m := sceneModel(sc, w, h)
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
	}
	return m, keys
}

func r86ttFooter(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return rows[len(rows)-1]
}

func TestTheDigitOfTheRowYouAreOnSaysSo(t *testing.T) {
	const note = "the one you are on"
	sc := r86ttScene(t, "two-tools")
	for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
			old := lipgloss.ColorProfile()
			lipgloss.SetColorProfile(prof)
			// The walkthrough's last key is `3`, and `3 api` is already
			// the selection: at the stand before it the frame carries no
			// note of its own.
			m, _ := r86ttWalk(sc, wh[0], wh[1], 1)
			before := m.View()
			if strings.TrimSpace(m.note) != "" {
				t.Fatalf("%dx%d: the stand before the digit already carries a note %q", wh[0], wh[1], m.note)
			}
			pressKey(m, "3")
			poll(m, sc)
			if m.View() == before {
				t.Errorf("%dx%d (%v): `3` on the row the deck is on drew a byte-identical frame:\n%s",
					wh[0], wh[1], prof, r86ttFooter(m))
			}
			if !strings.Contains(r86ttFooter(m), note) {
				t.Errorf("%dx%d (%v): `3` said nothing: %q", wh[0], wh[1], prof, r86ttFooter(m))
			}
			lipgloss.SetColorProfile(old)
		}
	}
	// Held, three sides, at 120 where every one of them is on a frame.
	m, _ := r86ttWalk(sc, 120, 34, 1)
	before := m.View()
	pressKey(m, "1") // a digit that moves says nothing of the kind
	poll(m, sc)
	if m.View() == before {
		t.Errorf("`1` from `3 api` moved nothing")
	}
	if strings.Contains(r86ttFooter(m), note) {
		t.Errorf("a digit that moved wore the note: %q", r86ttFooter(m))
	}
	m, _ = r86ttWalk(sc, 120, 34, 1)
	pressKey(m, "2") // the hidden session's digit keeps its own refusal
	poll(m, sc)
	if !strings.Contains(r86ttFooter(m), "2 api is hidden") || strings.Contains(r86ttFooter(m), note) {
		t.Errorf("the hidden digit's refusal moved: %q", r86ttFooter(m))
	}
	m, _ = r86ttWalk(sc, 120, 34, 1)
	pressKey(m, "9") // a digit no row wears
	poll(m, sc)
	if !strings.Contains(r86ttFooter(m), "no session 9") || strings.Contains(r86ttFooter(m), note) {
		t.Errorf("the unused digit's refusal moved: %q", r86ttFooter(m))
	}
}

// The sweep: at every stand of two scenes' walkthroughs, press the digit
// the selected row wears and hold it to answering. One pass per width —
// the note the digit leaves is cleared by the next keypress (#24), so the
// walk carries on unchanged.
func TestNoDigitOfTheRowYouAreOnIsSilent(t *testing.T) {
	for _, name := range []string{"two-tools", "alarm-storm"} {
		sc := r86ttScene(t, name)
		for _, wh := range [][2]int{{80, 24}, {120, 34}} {
			keys := canonicalKeys
			if len(sc.extra) > 0 {
				keys = append(append(append([]string(nil), keys...), "esc"), sc.extra...)
			}
			m := sceneModel(sc, wh[0], wh[1])
			for stand := 0; stand <= len(keys); stand++ {
				if stand > 0 {
					pressKey(m, keys[stand-1])
					poll(m, sc)
				}
				foot := r86ttFooter(m)
				if strings.Contains(foot, "enter keeps it") || strings.Contains(foot, "enter sends") ||
					strings.Contains(foot, "a digit acts") || strings.Contains(foot, "closes help") {
					continue // a panel or the help owns the digits
				}
				d := m.digits[m.selectedKey]
				if m.archiveView {
					d = 0
					for i, idx := range m.viewOrder() {
						if m.sessions[idx].Info.Key() == m.selectedKey {
							d = i + 1
						}
					}
				}
				if d < 1 || d > 9 {
					continue
				}
				before := m.View()
				pressKey(m, fmt.Sprintf("%d", d))
				if m.View() == before && strings.TrimSpace(m.note) == "" {
					t.Fatalf("%s %dx%d stand %d: `%d` on the row the deck is on drew nothing and said nothing:\n%s",
						name, wh[0], wh[1], stand, d, foot)
				}
			}
		}
	}
}

// r86fhMatchNote reads the walk's note: which match of how many.
var r86fhMatchNote = regexp.MustCompile(`^match (\d+)/(\d+)$`)

// TestTheSearchWalkWalksItsWholeRun pins round eighty-six's one thing.
//
// #235 gave `n` and `N` the question `ctrl+d` and `G` ask — did the page
// move — and #236 gave the press that moved the page the count of the
// match it landed on. Both read the walk's place back off `m.scroll`, and
// `clampScroll` pins the page at the last screenful: so the moment the
// rest of the run shared one screen the walk stopped, and every further
// press recomputed the same match. On `many-idle`'s reader at 120x34,
// searching `the`, `n` counted `match 3/9` … `match 6/9` and then stood
// there — matches 7, 8 and 9 were never named, the seventh press came
// back byte for byte the same frame, and `n` never reached the wrap
// `walkTo` says it has. Over the corpus that was 552 of 588 search
// stands and 2,082 matches a person could not walk to.
//
// The walk keeps its own place, steps it one match a press, and names it
// every time (#20's `❯ 3/12`, at the other key that jumps).
func TestTheSearchWalkWalksItsWholeRun(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // the frame is the one a person sees (#215, #218)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, c := range []struct {
		name  string
		sc    scene
		n     int
		query string
	}{
		{"many-idle", sceneManyIdle(), 13, "the"},
		{"many-idle", sceneManyIdle(), 13, "pytest"},
		{"fleet-hygiene", sceneFleetHygiene(), 13, "the"},
		{"very-long", sceneVeryLong(), 13, "the"},
	} {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}} {
			w, h := size[0], size[1]
			m := r86fhSearch(sceneModel(c.sc, w, h), c.sc, c.n, c.query)
			if m == nil {
				continue
			}
			total := len(readerMatches(m.doc(m.readerWidth()), m.query))
			if total < 2 {
				continue
			}
			r86fhWalkRun(t, m, c.sc, total, c.name, c.query, w, h, true)
		}
	}
}

// TestNoReaderWalkKeyStandsStill is the rule behind it, asked of every
// canonical reader stand of every scene at eighty and at 120: with a
// search running, four presses of `n` stand on four different matches —
// the walk that reads its place off the page repeats one instead. One
// walkthrough per scene and width, the search taken and put back at each
// stand, and the note read rather than the frame, so the sweep runs in a
// few seconds.
func TestNoReaderWalkKeyStandsStill(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, sc := range allScenes() {
		keys := append(append([]string{}, canonicalKeys...), "esc")
		keys = append(keys, sc.extra...)
		for _, size := range [][2]int{{80, 24}, {120, 34}} {
			w, h := size[0], size[1]
			m := sceneModel(sc, w, h)
			for n := 0; n <= len(keys); n++ {
				if m.level == levelReader && !m.showHelp && !m.searching && !m.replying {
					for _, q := range []string{"pytest"} {
						// The search is taken and put back: every stand
						// starts a query of its own, which is what resets
						// the walk, so only the page and the note need
						// restoring.
						scroll, query, note := m.scroll, m.query, m.note
						if mm := r86fhSearch(m, sc, -1, q); mm != nil {
							if total := len(readerMatches(mm.doc(mm.readerWidth()), mm.query)); total >= 2 {
								r86fhWalkRun(t, mm, sc, total, sc.name, q, w, h, false)
							}
						}
						m.scroll, m.query, m.note = scroll, query, note
					}
				}
				if n < len(keys) {
					pressKey(m, keys[n])
					poll(m, sc)
				}
			}
		}
	}
}

// r86fhWalkRun presses `n` a lap (or the first few steps of one) and asks
// what a walk owes: a different match every press, counted out of the
// whole run, and — where the caller asks for the frame too — a frame that
// changes under every press.
func r86fhWalkRun(t *testing.T, m *Model, sc scene, total int, name, query string, w, h int, frame bool) {
	t.Helper()
	lap := total
	if frame && lap > 10 {
		lap = 10 // a 168-match run is the same rule, ten presses in
	}
	if !frame && lap > 4 {
		lap = 4
	}
	seen := map[int]bool{}
	for i := 0; i < lap; i++ {
		// The frame is taken as it stands, note and all: a key is dead
		// when the frame a person is looking at comes back the same, and
		// the note the last press left is part of that frame. A note left
		// standing cannot pass for a fresh one either — it repeats an
		// index, which the run below catches.
		before := ""
		if frame {
			before = m.View()
		}
		pressKey(m, "n")
		poll(m, sc)
		if frame && m.View() == before {
			t.Errorf("%s %dx%d /%s: press %d of %d drew nothing and said nothing",
				name, w, h, query, i+1, lap)
			return
		}
		g := r86fhMatchNote.FindStringSubmatch(m.note)
		if g == nil {
			t.Errorf("%s %dx%d /%s: press %d of %d said %q, not which match of how many",
				name, w, h, query, i+1, lap, m.note)
			return
		}
		at, _ := strconv.Atoi(g[1])
		of, _ := strconv.Atoi(g[2])
		if of != total {
			t.Errorf("%s %dx%d /%s: the note counts %d matches, the document holds %d", name, w, h, query, of, total)
			return
		}
		if seen[at] {
			t.Errorf("%s %dx%d /%s: press %d of %d stood on match %d again — the run does not walk",
				name, w, h, query, i+1, lap, at)
			return
		}
		seen[at] = true
	}
	if len(seen) != lap {
		t.Errorf("%s %dx%d /%s: %d presses of `n` named %d matches of %d", name, w, h, query, lap, len(seen), total)
	}
}

// r86fhSearch presses the two keys the reader's own footer names — `/`,
// the query, enter — on a model already at a reader stand, replaying the
// first n keys of the scene's walkthrough first where n is not negative.
// It returns nil where the search finds nothing to walk.
func r86fhSearch(m *Model, sc scene, n int, query string) *Model {
	if n >= 0 {
		keys := append(append([]string{}, canonicalKeys...), "esc")
		keys = append(keys, sc.extra...)
		if n > len(keys) {
			n = len(keys)
		}
		for _, k := range keys[:n] {
			pressKey(m, k)
			poll(m, sc)
		}
	}
	if m.level != levelReader || m.showHelp || m.searching || m.replying {
		return nil
	}
	pressKey(m, "/")
	for _, r := range query {
		pressKey(m, string(r))
	}
	pressKey(m, "enter")
	poll(m, sc)
	if m.query == "" || len(readerMatches(m.doc(m.readerWidth()), m.query)) == 0 {
		return nil
	}
	return m
}

// r87ttScene finds a walkthrough scene by name.
func r87ttScene(t *testing.T, name string) scene {
	t.Helper()
	for _, sc := range allScenes() {
		if sc.name == name {
			return sc
		}
	}
	t.Fatalf("no scene %q", name)
	return scene{}
}

// r87ttReader opens session d's reader (Lv3) in a fresh deck.
func r87ttReader(sc scene, w, h, d int) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range []string{fmt.Sprintf("%d", d), "tab", "tab"} {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// r87ttSearch types a query into the reader and presses enter.
func r87ttSearch(m *Model, sc scene, q string) {
	pressKey(m, "/")
	pressKey(m, q)
	pressKey(m, "enter")
	poll(m, sc)
}

func r87ttFooter(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return rows[len(rows)-1]
}

// TestTheSearchYouTypedOpensOnItsFirstMatch pins round eighty-seven's one
// thing.
//
// #239 gave the walk its own place and made `enter` on a typed query the
// walk's first press. But a fresh search stands at row nought and
// `walkStep`'s fallback steps to the first match *after* the top of the
// page — so a match on row nought, which is the opening prompt the person
// themselves typed, was walked straight past. On the second day's reader
// at eighty, `/the` answered `match 2/3` with `❯ fix the 401 on token
// refresh` two rows above the page: the search named a match it had
// skipped and did not draw. 2,556 of 5,422 fresh searches over the corpus
// landed past their first match, 532 of them with that match off the page.
//
// Where the walk has no place yet the first press is a landing, not a
// step: `landFirstMatch` opens on match one and says so.
func TestTheSearchYouTypedOpensOnItsFirstMatch(t *testing.T) {
	prev := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(prev)
	sc := r87ttScene(t, "second-day")
	for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
			lipgloss.SetColorProfile(prof)
			m := r87ttReader(sc, wh[0], wh[1], 2)
			if m.level < levelReader {
				t.Fatalf("%dx%d: never reached the reader", wh[0], wh[1])
			}
			r87ttSearch(m, sc, "the")
			foot := r87ttFooter(m)
			if !strings.Contains(foot, "match 1/") {
				t.Errorf("%dx%d (%v): the search opened past its first match: %q", wh[0], wh[1], prof, foot)
			}
			// And the match it names is on the page it opened.
			doc := m.doc(m.readerWidth())
			ms := readerMatches(doc, m.query)
			top := m.readerTop(doc)
			if len(ms) == 0 {
				t.Fatalf("%dx%d: /the found nothing", wh[0], wh[1])
			}
			if ms[0] < top || ms[0] >= top+m.readerHeight() {
				t.Errorf("%dx%d (%v): match 1 sits on row %d, off a page of rows %d..%d",
					wh[0], wh[1], prof, ms[0], top, top+m.readerHeight())
			}
			// The walk still steps on from there, and wraps back.
			pressKey(m, "n")
			poll(m, sc)
			if !strings.Contains(r87ttFooter(m), fmt.Sprintf("match 2/%d", len(ms))) {
				t.Errorf("%dx%d (%v): `n` off the landing: %q", wh[0], wh[1], prof, r87ttFooter(m))
			}
			pressKey(m, "N")
			poll(m, sc)
			if !strings.Contains(r87ttFooter(m), fmt.Sprintf("match 1/%d", len(ms))) {
				t.Errorf("%dx%d (%v): `N` back to the landing: %q", wh[0], wh[1], prof, r87ttFooter(m))
			}
			pressKey(m, "N")
			poll(m, sc)
			if !strings.Contains(r87ttFooter(m), fmt.Sprintf("match %d/%d", len(ms), len(ms))) {
				t.Errorf("%dx%d (%v): `N` off the landing wraps to the last: %q", wh[0], wh[1], prof, r87ttFooter(m))
			}
		}
	}
	// Held: a query nothing answers still refuses, and the refusal is not a count.
	lipgloss.SetColorProfile(termenv.Ascii)
	m := r87ttReader(sc, 120, 34, 2)
	r87ttSearch(m, sc, "zzzznotalive")
	if !strings.Contains(r87ttFooter(m), "no matches") {
		t.Errorf("a query nothing answers: %q", r87ttFooter(m))
	}
}

// The sweep: over every scene, every session's reader and a spread of
// queries, no fresh search opens past its first match.
func TestNoTypedSearchOpensPastItsFirstMatch(t *testing.T) {
	prev := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(prev)
	lipgloss.SetColorProfile(termenv.Ascii)
	queries := []string{"the", "a", "1", "read", "test"}
	for _, sc := range allScenes() {
		for _, wh := range [][2]int{{80, 24}, {120, 34}} {
			for d := 1; d <= 9; d++ {
				for _, q := range queries {
					m := r87ttReader(sc, wh[0], wh[1], d)
					if m.level < levelReader {
						continue
					}
					r87ttSearch(m, sc, q)
					note := strings.TrimSpace(m.note)
					if !strings.HasPrefix(note, "match ") {
						continue // `no matches` is its own refusal
					}
					if !strings.HasPrefix(note, "match 1/") {
						t.Fatalf("%s %dx%d session %d /%s: the search opened on %q, past its first match",
							sc.name, wh[0], wh[1], d, q, note)
					}
					doc := m.doc(m.readerWidth())
					ms := readerMatches(doc, m.query)
					top := m.readerTop(doc)
					if ms[0] < top || ms[0] >= top+m.readerHeight() {
						t.Fatalf("%s %dx%d session %d /%s: opened on %q with match 1 off the page (row %d, page %d..%d)",
							sc.name, wh[0], wh[1], d, q, note, ms[0], top, top+m.readerHeight())
					}
				}
			}
		}
	}
}

// The reader's five up-and-down keys are one family: `j`, `k`, `ctrl+d`,
// `ctrl+u` and `G` all ask the page whether it moved, and all answer when
// it did not — "start of the conversation", "end of the conversation",
// "all of it is on screen" (#24, #228, #232). `g`, the sixth, set
// `m.scroll = 0` and said nothing, ever: over every canonical reader
// stand of every scene at every width, 608 presses, `g` spoke on none,
// moved no line on 522, and on 87 of those the frame came back byte for
// byte the same — the dead key SPEC's round-one rule bans, on a frame
// where `k` and `ctrl+u` both answer.
//
// TestTheReaderStartKeyAnswersLikeItsPair is the fold on one frame: on
// fleet-hygiene's reader at eighty, where the whole conversation is on
// screen, `k` says so and `g` must say the same thing; one screenful down,
// `g` must carry the page back and `g` again must say where it stopped.
func TestTheReaderStartKeyAnswersLikeItsPair(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor) // the frame is the one a person sees (#215, #218)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, c := range []struct {
		name string
		sc   scene
		n    int
	}{
		{"fleet-hygiene", sceneFleetHygiene(), 13},
		{"many-idle", sceneManyIdle(), 13},
		{"very-long", sceneVeryLong(), 13},
	} {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}} {
			w, h := size[0], size[1]
			if r87fhPinAt(c.sc, w, h, c.n) == nil {
				t.Fatalf("%s %dx%d: the walkthrough reaches no reader stand at key %d", c.name, w, h, c.n)
			}
			// The page carried to the start by the key that already
			// answers there, so the two are asked the same question.
			top := func() *Model {
				mm := r87fhPinAt(c.sc, w, h, c.n)
				for i := 0; i < 4000 && mm.readerTop(mm.doc(mm.readerWidth())) > 0; i++ {
					pressKey(mm, "ctrl+u")
					poll(mm, c.sc)
				}
				return mm
			}
			k := top()
			pressKey(k, "k")
			poll(k, c.sc)
			want := k.note
			if want == "" {
				t.Fatalf("%s %dx%d: `k` said nothing at the start", c.name, w, h)
			}
			g := top()
			pressKey(g, "g")
			poll(g, c.sc)
			if g.note == "" {
				t.Errorf("%s %dx%d: `g` on a page already at the start said nothing (`k` said %q)", c.name, w, h, want)
			}
			if g.note != want {
				t.Errorf("%s %dx%d: `g` says %q where `k` on the same page says %q", c.name, w, h, g.note, want)
			}
			// And the note is on the frame, not only in the model.
			if !strings.Contains(ansi.Strip(g.View()), want) {
				t.Errorf("%s %dx%d: %q is not on the frame `g` drew", c.name, w, h, want)
			}
			// One screenful down and back: `g` carries the page to the
			// start, and the press after it says where it stopped.
			d := r87fhPinAt(c.sc, w, h, c.n)
			pressKey(d, "ctrl+d")
			poll(d, c.sc)
			if d.readerTop(d.doc(d.readerWidth())) == 0 {
				continue // the whole conversation is on screen; nowhere to come back from
			}
			pressKey(d, "g")
			poll(d, c.sc)
			if at := d.readerTop(d.doc(d.readerWidth())); at != 0 {
				t.Errorf("%s %dx%d: `g` left the page at line %d, not the start", c.name, w, h, at)
			}
			pressKey(d, "g")
			poll(d, c.sc)
			if d.note == "" {
				t.Errorf("%s %dx%d: the second `g` at the start said nothing", c.name, w, h)
			}
		}
	}
}

// TestNoReaderStartKeyIsSilent is the rule behind it, asked of every
// canonical reader stand of every scene at every width: `g` answers, every
// press, whether or not the page had anywhere to go. The key is pressed
// and the page put back, so one walkthrough per scene and width covers
// every stand and the sweep runs in a few seconds.
func TestNoReaderStartKeyIsSilent(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, sc := range allScenes() {
		keys := append(append([]string{}, canonicalKeys...), "esc")
		keys = append(keys, sc.extra...)
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			m := sceneModel(sc, w, h)
			for n := 0; n <= len(keys); n++ {
				if m.level == levelReader && !m.showHelp && !m.searching && !m.replying {
					scroll, note := m.scroll, m.note
					top := m.readerTop(m.doc(m.readerWidth()))
					pressKey(m, "g")
					poll(m, sc)
					moved := m.readerTop(m.doc(m.readerWidth())) != top
					if !moved && m.note == "" {
						t.Errorf("%s %dx%d stand %d: `g` moved no line and said nothing", sc.name, w, h, n)
					}
					m.scroll, m.note = scroll, note
				}
				if n < len(keys) {
					pressKey(m, keys[n])
					poll(m, sc)
				}
			}
		}
	}
}

// r87fhPinAt replays the first n keys of a scene's walkthrough and returns
// the model where that leaves it, or nil where it is not in the reader.
func r87fhPinAt(sc scene, w, h, n int) *Model {
	keys := append(append([]string{}, canonicalKeys...), "esc")
	keys = append(keys, sc.extra...)
	if n > len(keys) {
		n = len(keys)
	}
	m := sceneModel(sc, w, h)
	for _, k := range keys[:n] {
		pressKey(m, k)
		poll(m, sc)
	}
	if m.level != levelReader || m.showHelp || m.searching || m.replying {
		return nil
	}
	return m
}

// r87sdFoot is the last row of a frame, ansi stripped.
func r87sdFoot(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return rows[len(rows)-1]
}

// r87sdArchiveOnNothing puts the deck where a fleet query matches nothing
// archived and the archive is then opened: the list draws no row, and the
// selection the frame keeps drawing is the live session it was on.
func r87sdArchiveOnNothing(sc scene, w, h int) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range []string{"/", "pytest", "enter", "A"} {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// TestTheArchiveDoesNotDenyTheLiveRowItIsDrawing pins round eighty-seven's
// second-day one thing.
//
// With a fleet query nothing archived matches, `A` opens an archive that
// draws no row — and the deck keeps the live session selected: the header
// names it, the trail beside it draws it, and the footer offers
// `enter attach` for its pane. Pressing that session's own digit, the one
// the fleet gives it for life (SPEC §4, #32, assignDigits), answered
// `no session 1` about the very row on the frame, one `esc` from a board
// that calls it `1 hello`.
//
// The refusal names it instead, as the hidden session's twin one branch
// above already does (#57): `1 hello is live`.
func TestTheArchiveDoesNotDenyTheLiveRowItIsDrawing(t *testing.T) {
	forceASCII(t)
	for _, sc := range []scene{sceneSecondDay(), sceneFewOngoing(), sceneManyIdle()} {
		for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				old := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(prof) // the frame a person sees (#215, #218)
				m := r87sdArchiveOnNothing(sc, wh[0], wh[1])
				if !m.archiveView {
					lipgloss.SetColorProfile(old)
					t.Fatalf("%s %dx%d: `A` did not open the archive", sc.name, wh[0], wh[1])
				}
				s, ok := m.selected()
				if !ok || !s.Live {
					lipgloss.SetColorProfile(old)
					t.Fatalf("%s %dx%d: the empty archive is not drawing a live row", sc.name, wh[0], wh[1])
				}
				d := m.digits[s.Info.Key()]
				if d < 1 || d > 9 {
					lipgloss.SetColorProfile(old)
					continue
				}
				// The frame is drawing this session: its name is in the
				// header row, whatever the digit clause does.
				head := ansi.Strip(strings.SplitN(m.View(), "\n", 2)[0])
				if !strings.Contains(head, sessionName(s.Info)) {
					lipgloss.SetColorProfile(old)
					t.Fatalf("%s %dx%d: the header does not name the row the archive is drawing: %q",
						sc.name, wh[0], wh[1], head)
				}
				pressKey(m, fmt.Sprintf("%d", d))
				poll(m, sc)
				foot := r87sdFoot(m)
				if strings.Contains(foot, fmt.Sprintf("no session %d", d)) {
					t.Errorf("%s %dx%d (%v): `%d` denied the live row the frame is drawing: %q",
						sc.name, wh[0], wh[1], prof, d, foot)
				}
				want := fmt.Sprintf("%d %s is live", d, sessionName(s.Info))
				if !strings.Contains(foot, want) {
					t.Errorf("%s %dx%d (%v): the refusal does not name %q: %q",
						sc.name, wh[0], wh[1], prof, want, foot)
				}
				lipgloss.SetColorProfile(old)
			}
		}
	}
	// The three other sides, at 120 where each is on a frame.
	sc := sceneSecondDay()
	// A digit no session of any kind wears keeps the bare refusal.
	m := r87sdArchiveOnNothing(sc, 120, 34)
	pressKey(m, "7")
	poll(m, sc)
	if !strings.Contains(r87sdFoot(m), "no session 7") {
		t.Errorf("an unused digit lost its refusal: %q", r87sdFoot(m))
	}
	// Off the archive the live digit still selects rather than refuses.
	m = sceneModel(sc, 120, 34)
	pressKey(m, "1")
	poll(m, sc)
	if strings.Contains(r87sdFoot(m), "is live") {
		t.Errorf("the board's own digit wore the archive's refusal: %q", r87sdFoot(m))
	}
	// And in an archive that draws rows the digits are still the
	// archive's own (#32): `1` selects its first row, not the live one.
	m = sceneModel(sc, 120, 34)
	pressKey(m, "A")
	poll(m, sc)
	pressKey(m, "1")
	poll(m, sc)
	if s, ok := m.selected(); !ok || s.Live {
		t.Errorf("`1` in a drawn archive left the archive's own numbering")
	}
	if strings.Contains(r87sdFoot(m), "is live") {
		t.Errorf("a drawn archive row wore the refusal: %q", r87sdFoot(m))
	}
	// And the refutation this fold rests on: on the very frame that
	// answered `no session N`, the deck already names the digit — #57's
	// hide note says it one keypress away, in the same view, at the same
	// width. The scene is many-idle: second-day has one live session, and
	// `x` on the last live one is `the live one stays` in the archive as
	// on the board (#146, #156, and this round's rule), so on that scene
	// it is not the press that shows this.
	mi := sceneManyIdle()
	m = r87sdArchiveOnNothing(mi, 120, 34)
	sel, ok2 := m.selected()
	if !ok2 || !sel.Live {
		t.Fatalf("many-idle: the empty archive is not drawing a live row")
	}
	want := fmt.Sprintf("%d %s is hidden", m.digits[sel.Info.Key()], sessionName(sel.Info))
	pressKey(m, "x")
	poll(m, mi)
	if !strings.Contains(r87sdFoot(m), want) {
		t.Errorf("the hide note on the empty archive stopped naming the digit: %q", r87sdFoot(m))
	}
}

// r88sdBFoot is the footer row of the frame as a person sees it.
func r88sdBFoot(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return rows[len(rows)-1]
}

// r88sdBHead is the identity header row.
func r88sdBHead(m *Model) string {
	return ansi.Strip(strings.SplitN(m.View(), "\n", 2)[0])
}

// r88sdBSearchStand puts the deck where a fleet query matches nothing: the
// list draws no row, and the selection the frame keeps drawing — header,
// trail and `enter attach` — is the live session it was on.
func r88sdBSearchStand(sc scene, w, h int) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range []string{"/", "pytest", "enter"} {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// TestTheFleetDoesNotDenyTheDigitItsHeaderIsDrawing pins round eighty-eight's
// second-day one thing.
//
// Three of the canonical walkthrough's own keys — `/`, `pytest`, `enter` —
// leave the fleet list drawing `no session matches /pytest` while the deck
// keeps its live session selected: the header names it `1 hello`, the trail
// beside it draws it, and the footer offers `enter attach`. `selectIndex`
// walks the drawn rows and finds none, so the digit fell through to the bare
// refusal and answered `no session 1` about the digit its own header draws
// three cells from the name.
//
// The deck already has the sentence for this: where the query happens to
// match the row, the same press at the same stand answers `the session you
// are on` (#238). It answers so here too.
func TestTheFleetDoesNotDenyTheDigitItsHeaderIsDrawing(t *testing.T) {
	forceASCII(t)
	const want = "the one you are on"
	for _, sc := range []scene{sceneSecondDay(), sceneFirstSession()} {
		for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				old := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(prof) // the frame a person sees (#215, #218)
				m := r88sdBSearchStand(sc, wh[0], wh[1])
				if m.archiveView {
					lipgloss.SetColorProfile(old)
					t.Fatalf("%s %dx%d: the stand is the fleet's, not the archive's", sc.name, wh[0], wh[1])
				}
				if len(m.viewOrder()) != 0 {
					lipgloss.SetColorProfile(old)
					t.Fatalf("%s %dx%d: the query drew rows, so the stand is not the one", sc.name, wh[0], wh[1])
				}
				s, ok := m.selected()
				if !ok || !s.Live {
					lipgloss.SetColorProfile(old)
					t.Fatalf("%s %dx%d: the empty list is not drawing a live row", sc.name, wh[0], wh[1])
				}
				d := m.digits[s.Info.Key()]
				if d < 1 || d > 9 {
					lipgloss.SetColorProfile(old)
					t.Fatalf("%s %dx%d: the drawn live row wears no digit", sc.name, wh[0], wh[1])
				}
				// The frame draws the digit and the name in its header.
				head := r88sdBHead(m)
				if !strings.Contains(head, fmt.Sprintf("%d %s", d, sessionName(s.Info))) {
					lipgloss.SetColorProfile(old)
					t.Fatalf("%s %dx%d: the header does not draw %q: %q",
						sc.name, wh[0], wh[1], fmt.Sprintf("%d %s", d, sessionName(s.Info)), head)
				}
				pressKey(m, fmt.Sprintf("%d", d))
				poll(m, sc)
				foot := r88sdBFoot(m)
				if strings.Contains(foot, fmt.Sprintf("no session %d", d)) {
					t.Errorf("%s %dx%d (%v): `%d` denied the digit the header draws: %q",
						sc.name, wh[0], wh[1], prof, d, foot)
				}
				if !strings.Contains(foot, want) {
					t.Errorf("%s %dx%d (%v): the note is not %q: %q", sc.name, wh[0], wh[1], prof, want, foot)
				}
				lifted := r88sdBSearchStand(sc, wh[0], wh[1])
				pressKey(lifted, "7")
				poll(lifted, sc)
				if d != 7 && !strings.Contains(r88sdBFoot(lifted), "no session 7") {
					t.Errorf("%s %dx%d (%v): a digit nobody wears lost its refusal: %q",
						sc.name, wh[0], wh[1], prof, r88sdBFoot(lifted))
				}
				lipgloss.SetColorProfile(old)
			}
		}
	}

	// The four other sides, at 120 where each is on a frame.
	sd := sceneSecondDay()

	// Where the query matches the row the deck already said this, and
	// still does: the very sentence this fold borrows (#238).
	mi := sceneManyIdle()
	m := r88sdBSearchStand(mi, 120, 34)
	if len(m.viewOrder()) == 0 {
		t.Fatalf("many-idle: /pytest matched nothing, so the control is not a control")
	}
	if s, ok := m.selected(); ok {
		pressKey(m, fmt.Sprintf("%d", m.digits[s.Info.Key()]))
		poll(m, mi)
		if !strings.Contains(r88sdBFoot(m), want) {
			t.Errorf("the drawn-row twin lost %q: %q", want, r88sdBFoot(m))
		}
	}

	// Off the search the digit still selects rather than refusing.
	m = sceneModel(sd, 120, 34)
	pressKey(m, "1")
	poll(m, sd)
	if strings.Contains(r88sdBFoot(m), "no session 1") {
		t.Errorf("the board's own digit wore a refusal: %q", r88sdBFoot(m))
	}

	// #57's hidden note still comes first: on a fleet where a session can
	// be hidden, a hidden session's digit still says where it is under a
	// query that draws no row at all. (On the second day `x` refuses —
	// `the live one stays` — so the side needs a fleet of more than one.)
	m = sceneModel(mi, 120, 34)
	hidden, hd := "", 0
	if s, ok := m.selected(); ok {
		hidden, hd = sessionName(s.Info), m.digits[s.Info.Key()]
	}
	for _, k := range []string{"x", "/", "zzzznothing", "enter", fmt.Sprintf("%d", hd)} {
		pressKey(m, k)
		poll(m, mi)
	}
	if len(m.viewOrder()) != 0 {
		t.Fatalf("many-idle: the hidden side's query drew rows")
	}
	if wantHidden := fmt.Sprintf("%d %s is hidden", hd, hidden); !strings.Contains(r88sdBFoot(m), wantHidden) {
		t.Errorf("the hide note stopped naming the digit under an empty search: want %q in %q", wantHidden, r88sdBFoot(m))
	}

	// And #242's archive twin is untouched: in the archive the note is
	// still `1 hello is live`, not this one.
	m = sceneModel(sd, 120, 34)
	for _, k := range []string{"/", "pytest", "enter", "A", "1"} {
		pressKey(m, k)
		poll(m, sd)
	}
	if !strings.Contains(r88sdBFoot(m), "1 hello is live") {
		t.Errorf("the archive twin (#242) lost its note: %q", r88sdBFoot(m))
	}
	if strings.Contains(r88sdBFoot(m), want) {
		t.Errorf("the fleet's note leaked into the archive: %q", r88sdBFoot(m))
	}
}

// r88fhOnlyQuery is a query only this session answers and nothing archived
// does: typing it and pressing `A` opens an archive with no rows at all.
func r88fhOnlyQuery(sc scene, w, h int, key, title string) string {
	for _, cand := range strings.Fields(strings.ToLower(title)) {
		if len(cand) < 4 {
			continue
		}
		m := sceneModel(sc, w, h)
		m.fleetQuery = cand
		only, archived := true, 0
		for _, s := range m.sessions {
			if !m.matchesQuery(s) {
				continue
			}
			if s.Live {
				if s.Info.Key() != key {
					only = false
				}
			} else {
				archived++
			}
		}
		if only && archived == 0 {
			return cand
		}
	}
	return ""
}

// r88fhEmptyArchive puts the deck where the fleet search in force leaves the
// archive with no rows: the session is selected, the query typed and kept,
// and `A` pressed.
func r88fhEmptyArchive(sc scene, w, h int, key, query string) *Model {
	m := sceneModel(sc, w, h)
	m.pointQuiet(key)
	poll(m, sc)
	for _, k := range []string{"/", query, "enter", "A"} {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// r88fhTrailOwner is the session whose conversation the trail panel is
// drawing, read off the trail the model holds.
func r88fhTrailOwner(m *Model, sc scene) string {
	if len(m.trail.Prompts) == 0 {
		return "—"
	}
	for _, s := range m.sessions {
		tr := sc.trails[s.Info.Key()]
		if len(tr.Prompts) > 0 && tr.Prompts[0].Text == m.trail.Prompts[0].Text && len(tr.Legs) == len(m.trail.Legs) {
			return sessionName(s.Info)
		}
	}
	return "—"
}

// TestTheEmptyArchiveKeepsTheSessionItIsDrawing is the fold on one frame.
//
// On fleet-hygiene, `/watch` answers only the live `harness` at
// `⌁ harness:1.0`, and nothing archived answers it at all. Press `A` and the
// archive draws no row — but the panel beside it goes on drawing harness's
// conversation, because `toggleArchive` swapped the selection for a key that
// was never set and `clampSelection` keeps what an empty view had. With no
// key, `selectedIndex` falls to nought, so the header and the trail's own
// title named `porter` — the fleet's first row, a different session — over
// harness's trail, and `enter attach` offered porter's pane.
//
// The frame names the session it is drawing, at every width.
func TestTheEmptyArchiveKeepsTheSessionItIsDrawing(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	var fh scene
	for _, sc := range allScenes() {
		if sc.name == "fleet-hygiene" {
			fh = sc
		}
	}
	var key string
	for _, s := range sceneModel(fh, 80, 24).sessions {
		if s.Live && s.Info.Title == "add watch driver tests" {
			key = s.Info.Key()
		}
	}
	if key == "" {
		t.Fatal("fleet-hygiene has no live `add watch driver tests`")
	}
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		w, h := size[0], size[1]
		m := r88fhEmptyArchive(fh, w, h, key, "watch")
		if !m.archiveView {
			t.Fatalf("%dx%d: the archive did not open", w, h)
		}
		if got := len(m.fleetOrder()); got != 0 {
			t.Fatalf("%dx%d: the archive drew %d rows, not none", w, h, got)
		}
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		if owner := r88fhTrailOwner(m, fh); owner != "harness" {
			t.Errorf("%dx%d: the panel draws %q, not harness", w, h, owner)
		}
		if got := selectedName(m); got != "harness" {
			t.Errorf("%dx%d: the frame's identity is %q while the panel draws harness", w, h, got)
		}
		if !strings.Contains(rows[0], "harness") {
			t.Errorf("%dx%d: the header names nobody the panel is drawing: %q", w, h, rows[0])
		}
		if !strings.Contains(rows[3], "TRAIL · harness") {
			t.Errorf("%dx%d: the trail's title is not the trail's: %q", w, h, rows[3])
		}
		if pane, ok := m.selectedPane(); !ok || pane.Target != "harness:1.0" {
			t.Errorf("%dx%d: `enter attach` would go to %q, not harness:1.0", w, h, pane.Target)
		}
		// The digit refusal (#242) names the session the frame is on.
		pressKey(m, "2")
		poll(m, fh)
		if m.note != "2 harness is live" {
			t.Errorf("%dx%d: the digit of the session on screen answered %q", w, h, m.note)
		}
	}
}

// TestNoEmptyArchiveNamesAnotherSession is the rule behind it, asked of
// every scene, every width and every live session a fleet search can single
// out: where `A` opens an archive with no rows, the session the frame names
// is the session whose conversation the panel is drawing.
func TestNoEmptyArchiveNamesAnotherSession(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	stands := 0
	for _, sc := range allScenes() {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			base := sceneModel(sc, w, h)
			for _, i := range base.viewOrder() {
				s := base.sessions[i]
				q := r88fhOnlyQuery(sc, w, h, s.Info.Key(), s.Info.Title)
				if q == "" {
					continue
				}
				m := r88fhEmptyArchive(sc, w, h, s.Info.Key(), q)
				if !m.archiveView || len(m.fleetOrder()) != 0 {
					continue // there was an archive row to land on
				}
				stands++
				owner := r88fhTrailOwner(m, sc)
				if owner == "—" {
					continue
				}
				if got := selectedName(m); got != owner {
					t.Errorf("%s %dx%d /%s: the frame names %q while the panel draws %q",
						sc.name, w, h, q, got, owner)
				}
			}
		}
	}
	if stands == 0 {
		t.Fatal("no empty-archive stand measured")
	}
	t.Logf("%d empty-archive stands", stands)
}

// The help's row for a key says what that key does where the help is
// standing. `g` is the one key whose action changes with the level: on the
// board, a list and the session view it grabs the oldest `▲` needs-you and
// hands over the terminal (SPEC §3, #40), and in the Lv3 reader it is the
// start of the conversation, the other end of `G` (#231, #241). The reader
// never names `g` on its footer, so the help is the only place to learn it
// — and it was promising the grab three levels deep with `▲1 4m` standing
// in the header and no grab to be had.

var r88ttPinAnsi = regexp.MustCompile("\x1b\\[[0-9;]*m")

// r88ttPinKeyRow is the help's row for one key, ansi stripped, or "" where
// the help draws none. A key row is the key in a ten-cell column.
func r88ttPinKeyRow(m *Model, key string) string {
	for _, l := range strings.Split(m.View(), "\n") {
		t := strings.TrimLeft(r88ttPinAnsi.ReplaceAllString(l, ""), " ")
		if len(t) > 10 && strings.HasPrefix(t, key+" ") &&
			strings.TrimSpace(t[len(key):10]) == "" && t[10] != ' ' {
			return strings.TrimRight(t, " ")
		}
	}
	return ""
}

// r88ttPinScene is one scene by name.
func r88ttPinScene(t *testing.T, name string) scene {
	t.Helper()
	for _, sc := range allScenes() {
		if sc.name == name {
			return sc
		}
	}
	t.Fatalf("no scene %q", name)
	return scene{}
}

// r88ttPinIsGrab presses `g` and reports whether the key the deck bound
// here is the grab — read off the frame, not off the level: the grab says
// where it went (`→ infra · ops:0.0`) or refuses in words with nothing
// waiting (`nothing is waiting on you`), and a refusal in words is still
// the grab and still earns the help's row (#40). Anything else is another
// key wearing `g`.
func r88ttPinIsGrab(m *Model, sc scene) bool {
	pressKey(m, "g")
	poll(m, sc)
	note := r88ttPinAnsi.ReplaceAllString(m.note, "")
	return strings.HasPrefix(note, "\u2192 ") || note == "nothing is waiting on you"
}

// TestTheHelpInTheReaderSaysWhatTheGrabKeyDoes holds the row and the key to
// each other on the two-tools reader at every width and under both colour
// profiles: where `g` grabs the row says grab, and where it does not the row
// says what it does instead. The board's own row is held on the same frames,
// so the fix cannot be "stop saying grab everywhere".
func TestTheHelpInTheReaderSaysWhatTheGrabKeyDoes(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	sc := r88ttPinScene(t, "two-tools")
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		lipgloss.SetColorProfile(prof)
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			where := func(what string) string {
				return sc.name + " " + strconv.Itoa(w) + "x" + strconv.Itoa(h) + " (" + strconv.Itoa(int(prof)) + ") " + what
			}

			// The board's row is the grab's own, and stays.
			top := sceneModel(sc, w, h)
			pressKey(top, "?")
			poll(top, sc)
			if row := r88ttPinKeyRow(top, "g"); !strings.Contains(row, "grab") {
				t.Errorf("%s: the board's help row for `g` no longer names the grab: %q", where("board"), row)
			}

			// Three keys deep, on a session that is not the one waiting.
			m := sceneModel(sc, w, h)
			for _, k := range []string{"j", "tab", "tab"} {
				pressKey(m, k)
				poll(m, sc)
			}
			if m.level < levelReader {
				t.Fatalf("%s: not in the reader", where("reader"))
			}
			if m.needsYouCount() == 0 {
				t.Fatalf("%s: no session is waiting on the frame", where("reader"))
			}
			grabs := r88ttPinIsGrab(m, sc)

			help := sceneModel(sc, w, h)
			for _, k := range []string{"j", "tab", "tab", "?"} {
				pressKey(help, k)
				poll(help, sc)
			}
			row := r88ttPinKeyRow(help, "g")
			if row == "" {
				t.Fatalf("%s: the reader's help draws no row for `g`", where("reader"))
			}
			says := strings.Contains(row, "grab the oldest")
			if says && !grabs {
				t.Errorf("%s: the help promises a grab `g` does not make here: %q", where("reader"), row)
			}
			if !says && grabs {
				t.Errorf("%s: `g` grabs here and the row does not say so: %q", where("reader"), row)
			}
			if !grabs && !strings.Contains(row, "the start of the conversation") {
				t.Errorf("%s: the row does not say what `g` does here: %q", where("reader"), row)
			}
		}
	}
}

// TestNoHelpRowPromisesAGrabTheReaderWillNotMake is the rule behind it,
// asked of every canonical stand of every scene at eighty and at 120: the
// help's row for `g` says what `g` does from that stand. The binding is
// pressed for, once per level a walkthrough reaches — the key is bound by
// level, and `r88ttPinIsGrab` reads which key it is off the frame — so a
// fold that made `g` grab in the reader instead would pass this too, with
// the row left as it was.
func TestNoHelpRowPromisesAGrabTheReaderWillNotMake(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, sc := range allScenes() {
		keys := append(append([]string{}, canonicalKeys...), "esc")
		keys = append(keys, sc.extra...)
		for _, size := range [][2]int{{80, 24}, {120, 34}} {
			w, h := size[0], size[1]
			type stand struct {
				n, lv int
				row   string
			}
			var stands []stand
			m := sceneModel(sc, w, h)
			for n := 0; n <= len(keys); n++ {
				if !m.showHelp && !m.searching && !m.replying {
					was := m.showHelp
					m.showHelp = true
					row := r88ttPinKeyRow(m, "g")
					m.showHelp = was
					if row != "" {
						stands = append(stands, stand{n, m.level, row})
					}
				}
				if n < len(keys) {
					pressKey(m, keys[n])
					poll(m, sc)
				}
			}
			grabAt := map[int]bool{}
			for _, st := range stands {
				if _, seen := grabAt[st.lv]; seen {
					continue
				}
				probe := sceneModel(sc, w, h)
				for _, k := range keys[:st.n] {
					pressKey(probe, k)
					poll(probe, sc)
				}
				grabAt[st.lv] = r88ttPinIsGrab(probe, sc)
			}
			for _, st := range stands {
				says := strings.Contains(st.row, "grab the oldest")
				if says == grabAt[st.lv] {
					continue
				}
				what := "promises a grab `g` does not make"
				if grabAt[st.lv] {
					what = "drops the grab `g` does make"
				}
				t.Errorf("%s %dx%d stand %d (Lv%d): the help %s here: %q",
					sc.name, w, h, st.n, st.lv, what, st.row)
			}
		}
	}
}

// ---- round 88, two-tools, second finding, re-pointed by round 96 ----
// #245 answered the board's digit pressed on a hidden live row the archive
// draws with `the one you are on`, to keep #242's `2 api is live` from
// naming a digit the frame does not. The sentence names one too: on this
// very stand the frame draws one row, `▸1 ● api`, and both `1` and `2`
// answered it. In the archive the digits are the archive's own (#32), so
// the digit no row wears takes the deck's own refusal — the same one this
// frame gives it from any other caret.
func TestTheArchivesDrawnRowIsTheSessionYouAreOn(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{80, 24}, {120, 34}, {220, 48}} {
		m := sceneModel(sceneTwoTools(), size[0], size[1])
		for _, s := range m.sessions {
			if s.Live && sessionName(s.Info) == "api" && m.digits[s.Info.Key()] == 2 {
				m.point(s.Info.Key())
				break
			}
		}
		pressKey(m, "x")
		pressKey(m, "A")
		pressKey(m, "1")
		if m.note != "the one you are on" {
			t.Errorf("%dx%d: the archive answered %q to the digit its own row wears", size[0], size[1], m.note)
		}
		pressKey(m, "2")
		if m.note != "no session 2" {
			t.Errorf("%dx%d: the archive answered %q to a digit no row of the frame wears", size[0], size[1], m.note)
		}
	}
}

// A move that moves nothing says why (#24, #152, #213). Its sentences all
// count the fleet: `the only live one`, `the only session`, `the last
// session`, `the first session`. Under a standing query the view answers
// with nothing they are all false — the column draws `no session matches
// /q`, the header counts `0 of 4`, and `j` answered `the only live one`
// over a list with no rows at all. Where nothing is drawn the move says so
// in the object the frame's own footer names: `no row to move to` on a
// list, `no column to move to` on a board that runs sideways (`h/l
// columns`), the trail's `no leg to move to` one level down.

// r89fhOneCount is the move refusals that count the fleet.
var r89fhOneCount = map[string]bool{
	"the only session": true, "the only live one": true,
	"the last session": true, "the first session": true,
}

// r89fhStand puts the deck on a stand and presses one key.
func r89fhStand(sc scene, w, h int, keys ...string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// r89fhOnlyQuery is a query only this live session answers, which no
// archived session answers at all — the search that leaves the archive
// with no rows.
func r89fhOnlyQuery(sc scene, w, h int, key, title string) string {
	for _, cand := range strings.Fields(strings.ToLower(title)) {
		if len(cand) < 4 {
			continue
		}
		m := sceneModel(sc, w, h)
		m.fleetQuery = cand
		only, archived := true, 0
		for _, s := range m.sessions {
			if !m.matchesQuery(s) {
				continue
			}
			if s.Live {
				if s.Info.Key() != key {
					only = false
				}
			} else {
				archived++
			}
		}
		if only && archived == 0 {
			return cand
		}
	}
	return ""
}

// TestTheMoveOnAListWithNoRowsSaysSo is the fold on its frames: the
// fleet-hygiene scene at every width, live and archive.
func TestTheMoveOnAListWithNoRowsSaysSo(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	var fh scene
	for _, sc := range allScenes() {
		if sc.name == "fleet-hygiene" {
			fh = sc
		}
	}
	if fh.name == "" {
		t.Fatal("no fleet-hygiene scene")
	}
	for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		w, h := size[0], size[1]
		for _, k := range []string{"j", "k"} {
			// The live fleet answers nothing.
			m := r89fhStand(fh, w, h, "/", "zzqq", "enter", k)
			if got := len(m.fleetOrder()); got != 0 {
				t.Fatalf("%dx%d: /zzqq drew %d rows, not none", w, h, got)
			}
			want := "no row to move to"
			if m.level == levelBoard && m.boardShown() {
				want = "no column to move to"
			}
			if got := ansi.Strip(m.note); got != want {
				t.Errorf("%dx%d live %q: the move over a list with no rows says %q, not %q", w, h, k, got, want)
			}
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			miss := false
			for _, r := range rows {
				if strings.Contains(r, "matches /zzqq") {
					miss = true
				}
			}
			if !miss {
				t.Errorf("%dx%d live %q: no miss row on the frame the note answers", w, h, k)
			}
			// The archive answers nothing: `/watch` singles out the live
			// harness and nothing archived answers it.
			a := r89fhStand(fh, w, h, "/", "watch", "enter", "A", k)
			if !a.archiveView {
				t.Fatalf("%dx%d: the archive did not open", w, h)
			}
			if got := len(a.fleetOrder()); got != 0 {
				t.Fatalf("%dx%d: the archive drew %d rows, not none", w, h, got)
			}
			if got := ansi.Strip(a.note); got != "no row to move to" {
				t.Errorf("%dx%d archive %q: the move over an archive with no rows says %q", w, h, k, got)
			}
		}
	}
}

// TestNoMoveOnAnEmptyViewCountsASession is the rule behind it, asked of
// every scene, every width and every stand a fleet search can leave with
// no rows — live list, board and archive: a move that moved nothing never
// counts a session the frame does not draw.
func TestNoMoveOnAnEmptyViewCountsASession(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	stands := 0
	for _, sc := range allScenes() {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			check := func(m *Model, what, k string) {
				if len(m.fleetOrder()) != 0 || len(m.viewOrder()) != 0 {
					return
				}
				stands++
				if note := ansi.Strip(m.note); r89fhOneCount[note] {
					t.Errorf("%s %dx%d %s %q: %q over a view drawing no row", sc.name, w, h, what, k, note)
				}
			}
			for _, k := range []string{"j", "k"} {
				for _, q := range []string{"zzqq", "reconcile", "watch"} {
					m := r89fhStand(sc, w, h, "/", q, "enter", k)
					if m.archiveView {
						continue
					}
					check(m, "live/"+q, k)
				}
				base := sceneModel(sc, w, h)
				for _, i := range base.viewOrder() {
					s := base.sessions[i]
					q := r89fhOnlyQuery(sc, w, h, s.Info.Key(), s.Info.Title)
					if q == "" {
						continue
					}
					m := sceneModel(sc, w, h)
					m.pointQuiet(s.Info.Key())
					poll(m, sc)
					for _, p := range []string{"/", q, "enter", "A", k} {
						pressKey(m, p)
						poll(m, sc)
					}
					if !m.archiveView {
						continue
					}
					check(m, "archive/"+q, k)
				}
			}
		}
	}
	if stands == 0 {
		t.Fatal("no empty-view stand measured")
	}
	t.Logf("%d empty-view stands", stands)
}

// r89sdEmptyList drives a standing search the list cannot answer and, when
// arch is true, `A` on top of it: either way the list draws no row.
func r89sdEmptyList(sc scene, w, h int, arch bool) *Model {
	m := sceneModel(sc, w, h)
	keys := []string{"/", "zzzznothing", "enter"}
	if arch {
		keys = append(keys, "A")
	}
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// TestNoMoveKeyCallsAnEmptyListTheOnlySession pins round eighty-nine's
// second-day second finding.
//
// `onlyOrLast` refused a move on `len(viewOrder()) <= 1`, so a list a
// standing search had emptied — nought rows, not one — answered `the only
// session` in the archive and `the only live one` on the fleet: on the
// second day that sentence stood over a body reading `no session matches
// /pytest` and a header counting `archive 0 of 12`, and on few-ongoing
// `the only live one` stood over a fleet of four. Nought is not one; the
// move's own question is where the row would go, and the answer is the
// deck's own form for a move with nowhere to land (#24).
func TestNoMoveKeyCallsAnEmptyListTheOnlySession(t *testing.T) {
	forceASCII(t)
	const want = "no session to move to"
	for _, sc := range []scene{sceneSecondDay(), sceneManyIdle(), sceneFewOngoing()} {
		for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				for _, arch := range []bool{true, false} {
					old := lipgloss.ColorProfile()
					lipgloss.SetColorProfile(prof) // the frame a person sees (#215, #218)
					for _, mv := range []string{"j", "k"} {
						m := r89sdEmptyList(sc, wh[0], wh[1], arch)
						if len(m.viewOrder()) != 0 {
							lipgloss.SetColorProfile(old)
							t.Fatalf("%s %dx%d arch=%v: the query left rows, so the stand is not the one", sc.name, wh[0], wh[1], arch)
						}
						if m.level >= levelWaypoints && len(TrailRows(m.trail, m.level)) > 1 {
							continue // the keys are the trail's here, not the list's
						}
						pressKey(m, mv)
						poll(m, sc)
						if m.note == "the only session" || m.note == "the only live one" {
							t.Errorf("%s %dx%d (%v) arch=%v: `%s` called a list drawing no row %q",
								sc.name, wh[0], wh[1], prof, arch, mv, m.note)
						}
						if m.note != "" && m.note != want && !strings.HasPrefix(m.note, "no ") {
							t.Errorf("%s %dx%d (%v) arch=%v: `%s` answered %q, not %q",
								sc.name, wh[0], wh[1], prof, arch, mv, m.note, want)
						}
						rows := strings.Split(ansi.Strip(m.View()), "\n")
						if foot := rows[len(rows)-1]; strings.Contains(foot, "the only session") || strings.Contains(foot, "the only live one") {
							t.Errorf("%s %dx%d (%v) arch=%v: the footer still counts one: %q",
								sc.name, wh[0], wh[1], prof, arch, foot)
						}
					}
					lipgloss.SetColorProfile(old)
				}
			}
		}
	}

	// The other side, at 100: a list that really does draw one row keeps
	// the sentence #152 scoped — `the only live one` on the fleet of one,
	// and the unscoped word in the archive.
	sd := sceneSecondDay()
	m := sceneModel(sd, 100, 30)
	if len(m.viewOrder()) != 1 {
		t.Fatalf("second-day: the live list is not a list of one")
	}
	pressKey(m, "j")
	poll(m, sd)
	if m.note != "the only live one" {
		t.Errorf("the fleet of one lost #152's scoped word: %q", m.note)
	}
	fh := sceneFleetHygiene()
	m = sceneModel(fh, 100, 30)
	for _, k := range []string{"/", "closed", "enter", "A"} {
		pressKey(m, k)
		poll(m, fh)
	}
	if len(m.viewOrder()) == 1 {
		pressKey(m, "j")
		poll(m, fh)
		if m.note != "the only session" {
			t.Errorf("the archive of one lost #152's unscoped word: %q", m.note)
		}
	}
}

// r89sdHead is the identity header row, ansi stripped.
func r89sdHead(m *Model) string {
	return ansi.Strip(strings.SplitN(m.View(), "\n", 2)[0])
}

// r89sdFoot is the note row.
func r89sdFoot(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return rows[len(rows)-1]
}

// r89sdEmptyArchive puts the deck in an archive that draws no row at all: a
// standing fleet query the archive cannot answer, then `A`. #244 keeps the
// live session the person left selected there — the header names it, the
// trail beside it draws it, and `enter attach` is offered for its pane.
func r89sdEmptyArchive(sc scene, w, h int, query string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range []string{"/", query, "enter", "A"} {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// TestTheHeaderKeepsTheDigitOfTheLiveRowItIsDrawing pins round eighty-nine's
// second-day one thing.
//
// In the archive the numbers on the header are the archive's own, as drawn
// (#32) — but an archive drawing no row claims no number, and the guard
// `d > 0 && !m.archiveView` dropped the digit from the header of the live
// session the frame is still selecting, drawing the trail of and offering
// `enter attach` for (#244). The same frame's own refusal calls that session
// `1 hello is live` (#242) and the board one `esc` away calls it `1 hello`:
// a live session takes its number on first sight and keeps it (#30), and the
// header is the place the deck says it (#79, #233).
func TestTheHeaderKeepsTheDigitOfTheLiveRowItIsDrawing(t *testing.T) {
	forceASCII(t)
	for _, sc := range []scene{sceneSecondDay(), sceneManyIdle(), sceneFewOngoing()} {
		for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				old := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(prof) // the frame a person sees (#215, #218)
				m := r89sdEmptyArchive(sc, wh[0], wh[1], "pytest")
				if !m.archiveView || len(m.viewOrder()) != 0 {
					lipgloss.SetColorProfile(old)
					t.Fatalf("%s %dx%d: the stand is not an archive drawing no row", sc.name, wh[0], wh[1])
				}
				s, ok := m.selected()
				if !ok || !s.Live {
					lipgloss.SetColorProfile(old)
					t.Fatalf("%s %dx%d: the empty archive is not keeping a live row (#244)", sc.name, wh[0], wh[1])
				}
				d := m.digits[s.Info.Key()]
				if d < 1 || d > 9 {
					lipgloss.SetColorProfile(old)
					t.Fatalf("%s %dx%d: the live row wears no digit", sc.name, wh[0], wh[1])
				}
				want := fmt.Sprintf("%d %s", d, sessionName(s.Info))
				if head := r89sdHead(m); !strings.Contains(head, want) {
					t.Errorf("%s %dx%d (%v): the header dropped the live row's digit: want %q in %q",
						sc.name, wh[0], wh[1], prof, want, head)
				}
				// The header still fits its terminal, and no clause of it
				// was sold for the digit: the tool word and the standing
				// query stay (#79, #82).
				if x := lipgloss.Width(strings.SplitN(m.View(), "\n", 2)[0]); x > wh[0] {
					t.Errorf("%s %dx%d (%v): the header runs past the terminal (%d)", sc.name, wh[0], wh[1], prof, x)
				}
				if !strings.Contains(r89sdHead(m), "/pytest") {
					t.Errorf("%s %dx%d (%v): the header lost the standing query: %q",
						sc.name, wh[0], wh[1], prof, r89sdHead(m))
				}
				// #242 is untouched: the digit pressed there still says
				// where the session is, and now the header agrees with it.
				pressKey(m, fmt.Sprintf("%d", d))
				poll(m, sc)
				if wantNote := fmt.Sprintf("%d %s is live", d, sessionName(s.Info)); !strings.Contains(r89sdFoot(m), wantNote) {
					t.Errorf("%s %dx%d (%v): #242's note went: want %q in %q",
						sc.name, wh[0], wh[1], prof, wantNote, r89sdFoot(m))
				}
				lipgloss.SetColorProfile(old)
			}
		}
	}

	// The other side, at 120: where the archive DOES draw rows the numbers
	// stay the archive's own, as drawn (#32) — the header wears the number
	// the row beside it wears, not the session's fleet digit.
	sd := sceneSecondDay()
	m := sceneModel(sd, 120, 34)
	pressKey(m, "A")
	poll(m, sd)
	if len(m.viewOrder()) == 0 {
		t.Fatalf("second-day: `A` drew no archive, so the control is not a control")
	}
	s, ok := m.selected()
	if !ok {
		t.Fatalf("second-day: the archive selected nothing")
	}
	if s.Live {
		t.Fatalf("second-day: the archive opened on a live row")
	}
	if r, ok := m.boardRows()[s.Info.Key()]; !ok || r.num != 1 {
		t.Fatalf("second-day: the archive's first row is not numbered 1")
	}
	if head := r89sdHead(m); !strings.Contains(head, "1 fix the 401 on token refresh") {
		t.Errorf("the drawing archive lost its own numbering (#32): %q", head)
	}

	// And off the archive nothing moves: the board's header is the board's.
	m = sceneModel(sd, 120, 34)
	if head := r89sdHead(m); !strings.Contains(head, "1 hello") {
		t.Errorf("the board's header lost the digit: %q", head)
	}
}

// r89ttEmptyArchive puts the deck in the archive a standing fleet query has
// cut to no rows, with the live session the person left still selected —
// #244's stand, reached by four of the canonical walkthrough's own keys.
func r89ttEmptyArchive(sc scene, w, h int) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range []string{"/", "pytest", "enter", "A"} {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

func r89ttFoot(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return rows[len(rows)-1]
}

func r89ttHead(m *Model) string {
	return ansi.Strip(strings.SplitN(m.View(), "\n", 2)[0])
}

// TestTheArchiveKeepsTheLastLiveSessionToo pins round eighty-nine's
// two-tools one thing.
//
// `the live one stays` is a rule about the fleet: `liveCount` counts what
// is `onBoard`, and that number is the same in the archive as on the
// board. Its clause carried `&& !m.archiveView` from the round where the
// count it read was `len(m.viewOrder())` — the archive's own list — so in
// the archive the guard never fired, and `x` there took the one live
// session off a board the frame does not draw. The board that came back
// said `nothing live` and `○ all quiet` beside a trail still drawing
// `● scout thinking… for 40s`, while the same key on the same session one
// `A` away answered `the live one stays`.
func TestTheArchiveKeepsTheLastLiveSessionToo(t *testing.T) {
	sc := sceneSecondDay()
	for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
			old := lipgloss.ColorProfile()
			lipgloss.SetColorProfile(prof) // the frame a person sees (#215, #218)
			m := r89ttEmptyArchive(sc, wh[0], wh[1])
			if !m.archiveView {
				lipgloss.SetColorProfile(old)
				t.Fatalf("%dx%d: `A` did not open the archive", wh[0], wh[1])
			}
			s, ok := m.selected()
			if !ok || !s.Live {
				lipgloss.SetColorProfile(old)
				t.Fatalf("%dx%d: the empty archive is not drawing a live row", wh[0], wh[1])
			}
			if m.liveCount() != 1 {
				lipgloss.SetColorProfile(old)
				t.Fatalf("%dx%d: this stand needs a fleet of one live session, got %d", wh[0], wh[1], m.liveCount())
			}
			key := s.Info.Key()
			pressKey(m, "x")
			poll(m, sc)
			if m.hidden[key] {
				t.Errorf("%dx%d (%v): `x` in the archive took the last live session off the board", wh[0], wh[1], prof)
			}
			if m.liveCount() != 1 {
				t.Errorf("%dx%d (%v): the fleet went from one live session to %d", wh[0], wh[1], prof, m.liveCount())
			}
			if foot := r89ttFoot(m); !strings.Contains(foot, "the live one stays") {
				t.Errorf("%dx%d (%v): the archive did not answer `the live one stays`: %q", wh[0], wh[1], prof, foot)
			}
			// The board the person comes back to still has its row.
			pressKey(m, "A")
			poll(m, sc)
			view := ansi.Strip(m.View())
			if strings.Contains(view, "nothing live") {
				t.Errorf("%dx%d (%v): the board says `nothing live` with a live session running:\n%s", wh[0], wh[1], prof, view)
			}
			if head := r89ttHead(m); strings.Contains(head, "all quiet") {
				t.Errorf("%dx%d (%v): the header says `all quiet` with a live session running: %q", wh[0], wh[1], prof, head)
			}
			lipgloss.SetColorProfile(old)
		}
	}

	// The other side, so the fix cannot be "stop hiding in the archive":
	// where the fleet has more than one live session the archive's `x`
	// still takes one off the board and names it (#57), and the unhide
	// it undoes still works.
	mi := sceneManyIdle()
	m := r89ttEmptyArchive(mi, 120, 34)
	s, ok := m.selected()
	if !ok || !s.Live || m.liveCount() < 2 {
		t.Fatalf("many-idle: this side needs a live row and more than one live session")
	}
	want := fmt.Sprintf("%d %s is hidden", m.digits[s.Info.Key()], sessionName(s.Info))
	pressKey(m, "x")
	poll(m, mi)
	if !m.hidden[s.Info.Key()] {
		t.Errorf("many-idle: the archive stopped hiding where the fleet has more than one live session")
	}
	if foot := r89ttFoot(m); !strings.Contains(foot, want) {
		t.Errorf("many-idle: the hide note stopped naming the digit: %q", foot)
	}
	pressKey(m, "x")
	poll(m, mi)
	if m.hidden[s.Info.Key()] {
		t.Errorf("many-idle: `x` again in the archive stopped bringing it back")
	}
	// And the board's own refusal is untouched: the canonical `x` on
	// second-day still says it.
	b := sceneModel(sc, 120, 34)
	pressKey(b, "x")
	poll(b, sc)
	if !strings.Contains(r89ttFoot(b), "the live one stays") {
		t.Errorf("the board's own refusal moved: %q", r89ttFoot(b))
	}
}

// TestNoArchiveEmptiesTheLiveBoard is the sweep: over every scene at
// eighty and 120, at every stand of a run that reaches the board, the
// list, the archive with rows and the archive a query has cut to none,
// `x` pressed on a live session while the fleet has exactly one live
// session may never take that count to nought — in any view. It reads
// the count off the model rather than the note, so a fold that answered
// differently but still emptied the board would fail it, and it holds
// the board's own side of the rule at the same stands.
func TestNoArchiveEmptiesTheLiveBoard(t *testing.T) {
	forceASCII(t)
	stands, inArchive := 0, 0
	run := []string{"/", "pytest", "enter", "A", "x", "x", "esc", "A", "A", "j", "x", "A", "x", "j", "esc", "A", "x"}
	for _, sc := range allScenes() {
		for _, wh := range [][2]int{{80, 24}, {120, 34}} {
			for stand := 0; stand <= len(run); stand++ {
				m := sceneModel(sc, wh[0], wh[1])
				for _, k := range run[:stand] {
					pressKey(m, k)
					poll(m, sc)
				}
				s, ok := m.selected()
				if !ok || !s.Live || m.hidden[s.Info.Key()] || m.liveCount() != 1 {
					continue
				}
				stands++
				if m.archiveView {
					inArchive++
				}
				pressKey(m, "x")
				poll(m, sc)
				if m.liveCount() == 0 {
					t.Errorf("%s %dx%d stand %d (archive=%v): `x` emptied the live board: %q",
						sc.name, wh[0], wh[1], stand, m.archiveView, r89ttFoot(m))
				}
			}
		}
	}
	if inArchive == 0 {
		t.Fatal("the sweep reached no stand with one live session selected in the archive")
	}
	t.Logf("stands swept: %d, of them in the archive: %d", stands, inArchive)
}

// r89ttArchiveHide puts the deck in the archive a fleet query has cut to
// no rows with a live board session still selected (#244), and hides it:
// `j` to the second column, `x` to put one session behind the archive so
// `A` has a door, then the query, then `A`, then the hide.
func r89ttArchiveHide(sc scene, w, h int) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range []string{"j", "x", "/", "zzz", "enter", "A", "x"} {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// TestTheArchiveHideNoteNamesNoWayBackIn pins round eighty-nine's
// two-tools second finding.
//
// `x` in the archive answered `3 api is hidden · A, then x` — the strip's
// route, which starts by leaving the view the person is standing in, for
// a key the same frame's footer names `x unhide`. In the archive the note
// keeps the fact and leaves the way to the row (#232, #233); at eighty
// the cells the clause spent buy `x unhide` back onto the frame that had
// shed it.
func TestTheArchiveHideNoteNamesNoWayBackIn(t *testing.T) {
	sc := sceneTwoTools()
	for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
			old := lipgloss.ColorProfile()
			lipgloss.SetColorProfile(prof) // the frame a person sees (#215, #218)
			m := r89ttArchiveHide(sc, wh[0], wh[1])
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			foot := rows[len(rows)-1]
			if !m.archiveView {
				lipgloss.SetColorProfile(old)
				t.Fatalf("%dx%d: this stand is not the archive", wh[0], wh[1])
			}
			s, ok := m.selected()
			if !ok || !m.hidden[s.Info.Key()] {
				lipgloss.SetColorProfile(old)
				t.Fatalf("%dx%d: `x` did not hide the row the archive is drawing", wh[0], wh[1])
			}
			if !strings.Contains(foot, sessionName(s.Info)+" is hidden") {
				t.Errorf("%dx%d (%v): the hide note stopped naming the row: %q", wh[0], wh[1], prof, foot)
			}
			if strings.Contains(foot, "A, then x") {
				t.Errorf("%dx%d (%v): the archive's own hide note sends the person out and back: %q",
					wh[0], wh[1], prof, foot)
			}
			if !strings.Contains(foot, "x unhide") {
				t.Errorf("%dx%d (%v): the frame names no way back for the row it just hid: %q",
					wh[0], wh[1], prof, foot)
			}
			lipgloss.SetColorProfile(old)
		}
	}

	// The other side, so the fix cannot be "drop the route everywhere":
	// off the archive the strip's route is the only way named, and the
	// note keeps it wherever the row has the cells for it.
	m := sceneModel(sceneFleetHygiene(), 152, 40)
	for _, k := range []string{"j", "x"} {
		pressKey(m, k)
		poll(m, sceneFleetHygiene())
	}
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	if foot := rows[len(rows)-1]; !strings.Contains(foot, "A, then x") {
		t.Errorf("the board's own hide note lost the way to the archive: %q", foot)
	}
}

// r90ttFoot is the last drawn row of a frame, escapes off.
func r90ttFoot(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return rows[len(rows)-1]
}

// r90ttStand plays a key run into a fresh model of the scene at one size.
func r90ttStand(sc scene, w, h int, keys ...string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// TestNoDigitDeniesALiveSessionTheFleetNumbers pins round ninety's one
// thing. `no session 1` is the sentence for a digit no session ever had.
// Under a fleet query that matches nothing, and in an archive that draws
// no row, the view draws no row for the digit — but the fleet still
// numbers a live session by it: the board one `esc` away calls it
// `1 infra`, the chips on the same row count it (`▲1 4m`), and where the
// digit is the selected session's own the deck already answers (#238,
// #242, #243, #245). The refusal names it, as the hidden twin does (#57).
//
// The other side is held too: where the archive draws rows the digits are
// the archive's own (#32), so a digit past its last row is still refused,
// and a digit no session carries is still `no session 7`.
func TestNoDigitDeniesALiveSessionTheFleetNumbers(t *testing.T) {
	forceASCII(t)
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]

			// The board under a query that matches nothing: `1` is the
			// fleet's needs-you session, live, on the board one esc away.
			m := r90ttStand(sceneTwoTools(), w, h, "2", "/", "zzz", "enter", "1")
			foot := r90ttFoot(m)
			if strings.Contains(foot, "no session 1") {
				t.Errorf("%dx%d %v: the board denies the digit the fleet numbers: %q", w, h, prof, foot)
			}
			if !strings.Contains(foot, "1 infra") {
				t.Errorf("%dx%d %v: the refusal does not name session 1: %q", w, h, prof, foot)
			}
			// A digit no session carries is still refused.
			m = r90ttStand(sceneTwoTools(), w, h, "2", "/", "zzz", "enter", "7")
			if foot := r90ttFoot(m); !strings.Contains(foot, "no session 7") {
				t.Errorf("%dx%d %v: a digit no session carries lost its refusal: %q", w, h, prof, foot)
			}

			// An archive that draws no row: the numbers are nobody's, and
			// the header on that very frame wears the board's digit (#248).
			m = r90ttStand(sceneManyIdle(), w, h, "2", "/", "zzz", "enter", "A", "1")
			foot = r90ttFoot(m)
			if !m.archiveView {
				t.Fatalf("%dx%d %v: the run did not reach the archive", w, h, prof)
			}
			if strings.Contains(foot, "no session 1") {
				t.Errorf("%dx%d %v: the empty archive denies the digit the fleet numbers: %q", w, h, prof, foot)
			}
			if !strings.Contains(foot, "1 etl") {
				t.Errorf("%dx%d %v: the empty archive's refusal does not name session 1: %q", w, h, prof, foot)
			}

			// An archive that draws rows numbers them itself (#32): a
			// digit past its last row is refused, whatever the board
			// calls that number.
			m = r90ttStand(sceneTwoTools(), w, h, "2", "x", "A", "3")
			if !m.archiveView || len(m.viewOrder()) == 0 {
				t.Fatalf("%dx%d %v: the run did not reach an archive with rows", w, h, prof)
			}
			if foot := r90ttFoot(m); !strings.Contains(foot, "no session 3") {
				t.Errorf("%dx%d %v: the archive's own numbering lost its refusal: %q", w, h, prof, foot)
			}
		}
		lipgloss.SetColorProfile(old)
	}
}

// ---- round 90, the owner's look ----
// The board keeps its order while the selection moves along it. `boardKeys`
// appends the selected session to the owed so a digit always shows its
// trail, and the pack read that list as the board's order: a calm selected
// session took the head of the calm band, and `j` along four calm cards
// reshuffled the board on every press — the card just read jumped to the
// front and the others slid (#252). Where the selected column would be
// trimmed off the end it still takes the last drawn slot (#16, #90).
func TestTheBoardKeepsItsOrderWhileTheSelectionMoves(t *testing.T) {
	forceASCII(t)
	for _, size := range [][2]int{{152, 40}, {220, 48}} {
		m := sceneModel(sceneManyIdle(), size[0], size[1])
		inner := size[0] - 2*edgePad
		n, cw := boardColumns(inner, m.drawnCount(m.viewOrder()))
		if n < 2 {
			t.Fatalf("%dx%d: not a board of columns", size[0], size[1])
		}
		before, _ := m.boardPack(n, cw, size[1]-6)
		if len(before) < 4 {
			t.Fatalf("%dx%d: too few columns to walk: %v", size[0], size[1], before)
		}
		// Walk the selection along every drawn column: the drawn order
		// must not change while every selected column is already drawn.
		for step := 0; step < len(before); step++ {
			pressKey(m, "j")
			after, _ := m.boardPack(n, cw, size[1]-6)
			drawn := false
			for _, k := range after {
				if k == m.selectedKey {
					drawn = true
				}
			}
			if !drawn {
				break // the walk left the board for the strip
			}
			wasDrawn := false
			for _, k := range before {
				if k == m.selectedKey {
					wasDrawn = true
				}
			}
			want := before
			if !wasDrawn {
				// The selection walked to a column the pack trims off
				// the end: it takes the last drawn slot (#16, #90), and
				// that is the one change allowed.
				want = append(append([]string(nil), before[:len(before)-1]...), m.selectedKey)
			}
			if strings.Join(after, ",") != strings.Join(want, ",") {
				t.Fatalf("%dx%d: after %d presses of j the board reshuffled:\n before %v\n after  %v", size[0], size[1], step+1, before, after)
			}
			before = after
		}
	}
}

// r90sdFooter is the row the person reads the keys off.
func r90sdFooter(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return strings.TrimRight(rows[len(rows)-1], " ")
}

// r90sdWalk builds the deck at this size and presses the route.
func r90sdWalk(sc scene, w, h int, keys []string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// r90sdArchiveRoutes are the two ways an archive comes to draw or select a
// live row: a standing fleet query the archive cannot answer, then `A` — the
// empty archive #244 and #248 settled — and a hide, then `A`, where the
// hidden live session is the row the archive draws under `hidden · x brings
// one back` (#29).
var r90sdArchiveRoutes = [][]string{
	{"/", "zzqqnothing", "enter", "A"},
	{"x", "A"},
}

// TestTheArchiveOffersTheReplyKeyForTheLiveRowItSelects pins round ninety's
// second-day one thing.
//
// compass has two writes and the footer offers them on the same test: does
// the selected session have a pane. #53 took `r reply` off the row that says
// `enter · no pane` — "a row that says no pane does not offer the other write
// either" — but the archive's keymap string never carried `r reply` at all,
// so wherever the archive draws or selects a *live* row the footer offered
// `enter attach` for a pane and named nothing that types into it, while `r`
// pressed there opened the reply card and a digit sent the line. One level
// deeper, on the same session and the same pane, the footer names `r reply`.
//
// Both sides: where the archive's selection has a pane the footer names both
// writes; where it has none it names neither (#53).
func TestTheArchiveOffersTheReplyKeyForTheLiveRowItSelects(t *testing.T) {
	forceASCII(t)
	stands := 0
	for _, sc := range []scene{sceneSecondDay(), sceneManyIdle(), sceneFewOngoing(), sceneTwoTools()} {
		for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				old := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(prof) // the frame a person sees (#215, #218)
				for _, route := range r90sdArchiveRoutes {
					m := r90sdWalk(sc, wh[0], wh[1], route)
					s, ok := m.selected()
					if !m.archiveView || !ok {
						continue
					}
					foot := r90sdFooter(m)
					pane, has := m.panes[s.Info.Key()]
					if !has || pane.Target == "" {
						// #53's side: no pane, neither write.
						if strings.Contains(foot, "r reply") {
							t.Errorf("%s %dx%d (%v) %v: the archive offered `r reply` for a row with no pane: %q",
								sc.name, wh[0], wh[1], prof, route, foot)
						}
						continue
					}
					// The promise before any of it is shed for width: a
					// live row with a pane is offered both writes.
					if !strings.Contains(m.keymap(), "r reply") {
						t.Errorf("%s %dx%d (%v) %v: the archive's keymap names no `r reply` for %q: %q",
							sc.name, wh[0], wh[1], prof, route, sessionName(s.Info), m.keymap())
					}
					if !strings.Contains(foot, "enter attach") {
						continue
					}
					if lipgloss.Width(foot)+len(" · r reply") > wh[0] {
						// The row has no room for another key; #159's
						// shedding, not the archive's silence.
						continue
					}
					stands++
					if !strings.Contains(foot, "r reply") {
						t.Errorf("%s %dx%d (%v) %v: the archive named one write and not the other for %q: %q",
							sc.name, wh[0], wh[1], prof, route, sessionName(s.Info), foot)
					}
					// The key the footer names is the key that acts: `r` on
					// this very stand opens the reply card for that session.
					r := r90sdWalk(sc, wh[0], wh[1], append(append([]string{}, route...), "r"))
					if !r.replying {
						t.Errorf("%s %dx%d (%v) %v: `r` in the archive did not open the reply card for %q",
							sc.name, wh[0], wh[1], prof, route, sessionName(s.Info))
					}
					if x := lipgloss.Width(foot); x > wh[0] {
						t.Errorf("%s %dx%d (%v) %v: the footer runs past the terminal (%d): %q",
							sc.name, wh[0], wh[1], prof, route, x, foot)
					}
				}
				lipgloss.SetColorProfile(old)
			}
		}
	}
	if stands == 0 {
		t.Fatal("no archive stand selected a live row with a pane: the pin is not standing on its frame")
	}
}

// r90sdReplyCardRow is the reply card's title row, the one that names the
// session the typed line will go to.
func r90sdReplyCardRow(m *Model) string {
	for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(row, "reply to ") {
			return row
		}
	}
	return ""
}

// TestTheReplyCardKeepsTheDigitOfTheLiveRowItTypesInto pins round ninety's
// second-day second finding.
//
// #31 put the row's digit on the reply card because a card naming a session
// by board position stood over a list where that position was another
// session. The guard was `d > 0 && !m.archiveView` — the same guard #248
// took off the header: in the archive the numbers are the archive's own
// (#32), but an archive drawing no row claims no number, and the live
// session that frame still selects, draws the trail of and now offers both
// writes for wears the digit its own header draws (#248) and its own refusal
// calls it by (#242). The card three rows under that header said `reply to
// hello` while the header said `1 hello`.
func TestTheReplyCardKeepsTheDigitOfTheLiveRowItTypesInto(t *testing.T) {
	forceASCII(t)
	stands := 0
	for _, sc := range []scene{sceneSecondDay(), sceneManyIdle(), sceneFewOngoing()} {
		for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				old := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(prof) // the frame a person sees (#215, #218)
				m := sceneModel(sc, wh[0], wh[1])
				for _, k := range []string{"/", "pytest", "enter", "A", "r"} {
					pressKey(m, k)
					poll(m, sc)
				}
				s, ok := m.selected()
				if !ok || !m.archiveView || len(m.viewOrder()) != 0 || !s.Live || !m.replying {
					lipgloss.SetColorProfile(old)
					continue
				}
				d := m.digits[s.Info.Key()]
				if d < 1 || d > 9 {
					lipgloss.SetColorProfile(old)
					continue
				}
				stands++
				card := r90sdReplyCardRow(m)
				want := "reply to " + strconv.Itoa(d) + " · " + sessionName(s.Info)
				if !strings.Contains(card, want) {
					t.Errorf("%s %dx%d (%v): the reply card dropped the live row's digit: want %q in %q",
						sc.name, wh[0], wh[1], prof, want, card)
				}
				// The header of the same frame says it too, and the card
				// still fits its terminal.
				head := ansi.Strip(strings.SplitN(m.View(), "\n", 2)[0])
				if !strings.Contains(head, fmt.Sprintf("%d %s", d, sessionName(s.Info))) {
					t.Errorf("%s %dx%d (%v): #248's header is not standing: %q", sc.name, wh[0], wh[1], prof, head)
				}
				for _, row := range strings.Split(m.View(), "\n") {
					if x := lipgloss.Width(row); x > wh[0] {
						t.Errorf("%s %dx%d (%v): a row runs past the terminal (%d): %q",
							sc.name, wh[0], wh[1], prof, x, ansi.Strip(row))
					}
				}
				lipgloss.SetColorProfile(old)
			}
		}
	}
	if stands == 0 {
		t.Fatal("no empty archive opened a reply card on a live row: the pin is not standing on its frame")
	}
}

// ---- round 90, fleet-hygiene ----
// Round ninety, the fleet-hygiene operator, the one thing.
//
// #47 gave the recent band its digits — "numbered on from the live fleet's
// last digit … A digit opens its row in the archive, where the trail is
// already known how to draw, and `A` comes back to the live fleet where
// you were" — and #51 held only that such a digit "lands under a different
// number (#32)", which presumes it lands.
//
// It does not land on the board. `recentRows` refuses the board (#43), and
// `strandedBand` lifts that refusal for the one render call that draws a
// band under the strip where the columns have run out. `openRecent` calls
// `recentRows` outside that call, sees no band, and answers `no session 5`
// on a frame drawing `5 ○ api · "the pane I closed half an hour ago"` five
// rows above the footer — the digit denying a row the frame numbers (#243,
// #245), while the same digit on the same fleet twenty columns narrower
// opens it. The key now reads the band the frame drew, and no other row.

// r90fhBandRowRe matches a band row as drawn: two or three cells of air,
// the digit, the archived glyph.
var r90fhBandRowRe = regexp.MustCompile(`^\s{1,3}(\d) ○ `)

// r90fhBandDigits reads the digits the frame draws on its band — the rows
// under the band's own `A browses` header that no live column wears.
func r90fhBandDigits(m *Model) []int {
	live := map[int]bool{}
	for _, r := range m.boardRows() {
		live[r.num] = true
	}
	var out []int
	below := false
	for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(row, "· A browses") || strings.Contains(row, "archived · A") {
			below = true
			continue
		}
		if !below {
			continue
		}
		if mm := r90fhBandRowRe.FindStringSubmatch(row); mm != nil {
			d, _ := strconv.Atoi(mm[1])
			if !live[d] {
				out = append(out, d)
			}
		}
	}
	return out
}

// r90fhBandName is the session name the band's row for this digit draws.
func r90fhBandName(m *Model, d int) string {
	below := false
	for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
		if strings.Contains(row, "· A browses") || strings.Contains(row, "archived · A") {
			below = true
			continue
		}
		if !below {
			continue
		}
		if mm := r90fhBandRowRe.FindStringSubmatch(row); mm != nil {
			if n, _ := strconv.Atoi(mm[1]); n == d {
				rest := strings.TrimSpace(row[strings.Index(row, "○ ")+len("○ "):])
				if i := strings.Index(rest, " · "); i > 0 {
					return rest[:i]
				}
				return rest
			}
		}
	}
	return ""
}

// TestTheBoardsBandOpensOnItsDigit is the fold on its frame: fleet-hygiene
// at 120, 152 and 220, where the board strands a band under its strip.
func TestTheBoardsBandOpensOnItsDigit(t *testing.T) {
	forceASCII(t)
	var fh scene
	for _, sc := range allScenes() {
		if sc.name == "fleet-hygiene" {
			fh = sc
		}
	}
	if fh.name == "" {
		t.Fatal("no fleet-hygiene scene")
	}
	for _, wh := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
			old := lipgloss.ColorProfile()
			lipgloss.SetColorProfile(prof) // the frame a person sees (#215, #218)
			m := sceneModel(fh, wh[0], wh[1])
			if m.level != levelBoard || !m.boardShown() {
				lipgloss.SetColorProfile(old)
				t.Fatalf("%dx%d: fleet-hygiene does not open on the board", wh[0], wh[1])
			}
			digits := r90fhBandDigits(m)
			if len(digits) == 0 {
				lipgloss.SetColorProfile(old)
				t.Fatalf("%dx%d: the board strands no band, so the stand is not the stand", wh[0], wh[1])
			}
			for _, d := range digits {
				c := sceneModel(fh, wh[0], wh[1])
				_ = c.View() // the key is pressed on a drawn frame, as the deck presses it (#221)
				want := r90fhBandName(c, d)
				was := c.selectedKey
				pressKey(c, strconv.Itoa(d))
				poll(c, fh)
				if note := ansi.Strip(c.note); strings.HasPrefix(note, "no session ") {
					t.Errorf("%dx%d (%v): `%d` answered %q over the band row it draws (%s)",
						wh[0], wh[1], prof, d, note, want)
					continue
				}
				if !c.archiveView {
					t.Errorf("%dx%d (%v): `%d` did not open the archive", wh[0], wh[1], prof, d)
					continue
				}
				s, ok := c.selected()
				if !ok || s.Live {
					t.Errorf("%dx%d (%v): `%d` landed on no archived session", wh[0], wh[1], prof, d)
					continue
				}
				if got := sessionName(s.Info); got != want && !strings.Contains(archiveHeadline(s), want) {
					t.Errorf("%dx%d (%v): `%d` opened %q where its row names %q", wh[0], wh[1], prof, d, got, want)
				}
				// #47's other half: `A` comes back to the live fleet
				// where you were, at the level the digit was pressed (#54).
				pressKey(c, "A")
				poll(c, fh)
				if c.archiveView || c.level != levelBoard || c.selectedKey != was {
					t.Errorf("%dx%d (%v): after `%d` then `A` the deck is not back on the board it left (archive=%v level=%d)",
						wh[0], wh[1], prof, d, c.archiveView, c.level)
				}
			}
			lipgloss.SetColorProfile(old)
		}
	}
}

// TestNoBandRowRefusesTheDigitItWears is the rule behind it, over every
// scene and width: a digit the frame draws on its band opens that row, and
// a digit no row wears is still refused.
func TestNoBandRowRefusesTheDigitItWears(t *testing.T) {
	forceASCII(t)
	drawn, refusedRight := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := wh[0], wh[1]
				base := sceneModel(sc, w, h)
				_ = base.View()
				band := map[int]bool{}
				for _, d := range r90fhBandDigits(base) {
					band[d] = true
				}
				for d := 1; d <= 9; d++ {
					m := sceneModel(sc, w, h)
					_ = m.View() // pressed on a drawn frame (#221)
					pressKey(m, strconv.Itoa(d))
					poll(m, sc)
					note := ansi.Strip(m.note)
					refused := note == fmt.Sprintf("no session %d", d)
					if band[d] {
						drawn++
						if refused {
							t.Errorf("%s %dx%d (%v): `%d` answered %q over the band row it draws",
								sc.name, w, h, prof, d, note)
						}
					} else if refused {
						// The other side: a digit no row wears is still
						// refused, and the fold gives the board no band
						// it did not draw.
						refusedRight++
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if drawn < 40 {
		t.Fatalf("only %d band digits drawn: the sweep is not sweeping", drawn)
	}
	t.Logf("%d band digits drawn, %d digits no row wears still refused", drawn, refusedRight)
}

// Round ninety, the fleet-hygiene operator.
//
// The hide note names the session the way its row does — digit, name, and
// the pane where namesakes share a tmux session (#57, #62, #101). On the
// board that digit is the fleet's own and the row is gone the moment the
// key acts, so the note is the only place it stands. In the archive the
// numbers are the archive's own (#32) and the frame goes on drawing the
// hidden row under the cursor, so the note's `2 harness is hidden` stood
// over `▸1 ● harness` beneath a header reading `1 harness` — a number
// neither the row nor the header wears. That is the digit the frame does
// not draw: #245 took it out of
// the refusal one branch away, #248 kept the header true to the row, and
// `boardRows` already numbers a hidden row as drawn for the same stated
// reason — a digit is a key, and one digit must not name two sessions.
//
// The fold makes the note wear the number the frame draws, and only where
// the fleet gave the session a digit at all, so no note grows a cell.

// r90fhCursorNum is the number the frame draws on the row under the cursor.
var r90fhCursorNum = regexp.MustCompile(`^\s*[▸>]\s*(\d)\s`)

// r90fhNoteNum is the digit the note leads with, or 0.
var r90fhNoteNum = regexp.MustCompile(`^(\d) `)

// r90fhDrawnNum reads the cursor row's number off the frame itself.
func r90fhDrawnNum(m *Model) int {
	for _, r := range strings.Split(ansi.Strip(m.View()), "\n") {
		if mm := r90fhCursorNum.FindStringSubmatch(r); mm != nil {
			n, _ := strconv.Atoi(mm[1])
			return n
		}
	}
	return 0
}

// r90fhOnlyQuery is a query this live session alone answers and no
// archived session answers at all — the search that leaves the archive
// with no rows, so `A` keeps the live session selected (#244) and `x`
// there hides the row the archive then draws.
func r90fhOnlyQuery(sc scene, w, h int, key, title string) string {
	for _, cand := range strings.Fields(strings.ToLower(title)) {
		if len(cand) < 4 {
			continue
		}
		m := sceneModel(sc, w, h)
		m.fleetQuery = cand
		only, archived := true, 0
		for _, s := range m.sessions {
			if !m.matchesQuery(s) {
				continue
			}
			if s.Live {
				if s.Info.Key() != key {
					only = false
				}
			} else {
				archived++
			}
		}
		if only && archived == 0 {
			return cand
		}
	}
	return ""
}

// r90fhHideStand presses `/ q enter A x` on one live session and returns
// the deck, or nil where that stand does not exist on this scene.
func r90fhHideStand(sc scene, w, h int, key, query string) *Model {
	m := sceneModel(sc, w, h)
	m.point(key)
	for _, k := range []string{"/", query, "enter", "A"} {
		pressKey(m, k)
		poll(m, sc)
	}
	if !m.archiveView || len(m.viewOrder()) != 0 {
		return nil
	}
	if s, ok := m.selected(); !ok || !s.Live {
		return nil
	}
	pressKey(m, "x")
	poll(m, sc)
	if !m.archiveView || !strings.Contains(ansi.Strip(m.note), " is hidden") {
		return nil
	}
	return m
}

// r90fhFooter is the footer row of the frame as a person sees it.
func r90fhFooter(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return rows[len(rows)-1]
}

// r90fhHeader is the identity row.
func r90fhHeader(m *Model) string {
	return ansi.Strip(strings.SplitN(m.View(), "\n", 2)[0])
}

// TestTheArchivesHideNoteWearsTheNumberTheFrameDraws is the fold on its
// frame: fleet-hygiene at every width, `/watch`, `enter`, `A`, `x`. The
// archive draws the hidden `harness` under its own `1`, the header says
// `1 harness`, and the note must not be the frame's only `2`.
func TestTheArchivesHideNoteWearsTheNumberTheFrameDraws(t *testing.T) {
	forceASCII(t)
	var fh scene
	for _, sc := range allScenes() {
		if sc.name == "fleet-hygiene" {
			fh = sc
		}
	}
	if fh.name == "" {
		t.Fatal("no fleet-hygiene scene")
	}
	var key string
	for _, s := range sceneModel(fh, 80, 24).sessions {
		if s.Live && sessionName(s.Info) == "harness" {
			if q := r90fhOnlyQuery(fh, 80, 24, s.Info.Key(), "watch"); q != "" {
				key = s.Info.Key()
				break
			}
		}
	}
	if key == "" {
		t.Fatal("fleet-hygiene: no live harness a query singles out")
	}
	for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
			old := lipgloss.ColorProfile()
			lipgloss.SetColorProfile(prof) // the frame a person sees (#215, #218)
			m := r90fhHideStand(fh, wh[0], wh[1], key, "watch")
			if m == nil {
				lipgloss.SetColorProfile(old)
				t.Fatalf("%dx%d: the archive-hide stand did not stand", wh[0], wh[1])
			}
			drawn := r90fhDrawnNum(m)
			if drawn == 0 {
				lipgloss.SetColorProfile(old)
				t.Fatalf("%dx%d (%v): the frame draws no number on the row under the cursor", wh[0], wh[1], prof)
			}
			note := ansi.Strip(m.note)
			if mm := r90fhNoteNum.FindStringSubmatch(note); mm != nil {
				if got, _ := strconv.Atoi(mm[1]); got != drawn {
					t.Errorf("%dx%d (%v): the hide note says %q where the frame draws that row as %d",
						wh[0], wh[1], prof, note, drawn)
				}
			} else {
				t.Errorf("%dx%d (%v): the hide note dropped the number the frame draws: %q", wh[0], wh[1], prof, note)
			}
			// The frame the note answers: the row is drawn, the header
			// agrees with it, and the way back is the key this footer
			// names (#250), not a digit.
			if head := r90fhHeader(m); !strings.Contains(head, strconv.Itoa(drawn)+" harness") {
				t.Errorf("%dx%d (%v): the header does not draw the row's number: %q", wh[0], wh[1], prof, head)
			}
			if foot := r90fhFooter(m); !strings.Contains(foot, "x unhide") {
				t.Errorf("%dx%d (%v): the footer stopped offering the way back: %q", wh[0], wh[1], prof, foot)
			}
			if x := lipgloss.Width(r90fhFooter(m)); x > wh[0] {
				t.Errorf("%dx%d (%v): the footer runs past the terminal (%d)", wh[0], wh[1], prof, x)
			}
			lipgloss.SetColorProfile(old)
		}
	}
}

// TestNoArchiveHideNoteNamesADigitTheFrameDoesNotDraw is the rule behind
// it, asked of every scene, every width and every live session a query can
// single out: a hide inside the archive never names a number the frame
// does not draw.
func TestNoArchiveHideNoteNamesADigitTheFrameDoesNotDraw(t *testing.T) {
	forceASCII(t)
	stands := 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := wh[0], wh[1]
				for _, s := range sceneModel(sc, w, h).sessions {
					if !s.Live {
						continue
					}
					q := r90fhOnlyQuery(sc, w, h, s.Info.Key(), sessionName(s.Info)+" "+archiveHeadline(s))
					if q == "" {
						continue
					}
					m := r90fhHideStand(sc, w, h, s.Info.Key(), q)
					if m == nil {
						continue
					}
					stands++
					note := ansi.Strip(m.note)
					mm := r90fhNoteNum.FindStringSubmatch(note)
					if mm == nil {
						continue
					}
					got, _ := strconv.Atoi(mm[1])
					if drawn := r90fhDrawnNum(m); drawn != 0 && got != drawn {
						t.Errorf("%s %dx%d (%v): %q over a row the frame draws as %d",
							sc.name, w, h, prof, note, drawn)
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if stands < 100 {
		t.Fatalf("only %d archive-hide stands: the sweep is not sweeping", stands)
	}
	t.Logf("%d archive-hide stands", stands)
}

// ---- round 91, two-tools ----
// r91ttStand plays a key run on a scene and hands back the model.
func r91ttStand(sc scene, w, h int, keys ...string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// r91ttFoot is the frame's last drawn row, escapes stripped.
func r91ttFoot(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return rows[len(rows)-1]
}

// TestTheUnhideNoteNamesWhichSessionCameBack pins round ninety-one's one
// thing for the two-tools operator. `x` in the archive puts a hidden
// session back on the board, and the row leaves the archive the moment
// the key acts — so the frame that follows draws it nowhere. On a fleet
// with two live sessions called `api` the note said `api is back on the
// board` while every `api` still on the frame — the header's, and the one
// row the archive goes on listing — was the other one, the one still
// hidden. The note wears the number of the view it names, as the hide
// note does (#256) and the header (#248) and the refusal (#251) do: the
// board digit, which is the key that reaches the session there (#16) and
// the same digit the hide note spent one press earlier.
//
// The other side is held: where the archive is left with nothing to draw
// and the selection stays on the session that came back, the note still
// wears that session's board digit, and it is the digit the header on the
// same frame draws.
func TestTheUnhideNoteNamesWhichSessionCameBack(t *testing.T) {
	forceASCII(t)
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			where := fmt.Sprintf("%dx%d %v", w, h, prof)

			// Both `api` sessions off the board: the notes name them
			// `2 api` and `3 api`, the digits the board gave them.
			m := r91ttStand(sceneTwoTools(), w, h, "2", "x")
			if foot := r91ttFoot(m); !strings.Contains(foot, "2 api is hidden") {
				t.Fatalf("%s: the run did not hide session 2: %q", where, foot)
			}
			m = r91ttStand(sceneTwoTools(), w, h, "2", "x", "x")
			if foot := r91ttFoot(m); !strings.Contains(foot, "3 api is hidden") {
				t.Fatalf("%s: the run did not hide session 3: %q", where, foot)
			}

			// The archive lists both; `x` brings the first back.
			m = r91ttStand(sceneTwoTools(), w, h, "2", "x", "x", "A")
			if !m.archiveView || len(m.viewOrder()) < 2 {
				t.Fatalf("%s: the run did not reach an archive drawing both rows", where)
			}
			m = r91ttStand(sceneTwoTools(), w, h, "2", "x", "x", "A", "x")
			foot := r91ttFoot(m)
			if !strings.Contains(foot, "is back on the board") {
				t.Fatalf("%s: `x` did not put the session back: %q", where, foot)
			}
			if strings.Contains(foot, " api is back on the board") &&
				!strings.Contains(foot, "2 api is back on the board") {
				t.Errorf("%s: the unhide note names no session: %q", where, foot)
			}
			if !strings.Contains(foot, "2 api is back on the board") {
				t.Errorf("%s: the note does not name the session that came back: %q", where, foot)
			}
			// The frame it stands on draws the other `api`, still hidden:
			// the header names it and the archive still lists it, so a
			// note without a number names the session the frame does not.
			view := ansi.Strip(m.View())
			head := strings.Split(view, "\n")[0]
			if !strings.Contains(head, "api") || strings.Contains(head, "2 api") {
				t.Errorf("%s: the header does not draw the other api: %q", where, strings.TrimSpace(head))
			}
			if len(m.viewOrder()) != 1 {
				t.Errorf("%s: the archive no longer draws the row that stayed hidden", where)
			}

			// Held: with nothing left in the archive the note still wears
			// the digit, and it is the one the header draws.
			m = r91ttStand(sceneTwoTools(), w, h, "2", "x", "x", "A", "x", "x")
			foot = r91ttFoot(m)
			if !strings.Contains(foot, "3 api is back on the board") {
				t.Errorf("%s: the last unhide lost its number: %q", where, foot)
			}
			head = strings.Split(ansi.Strip(m.View()), "\n")[0]
			if !strings.Contains(head, "3 api") {
				t.Errorf("%s: the header does not draw the session the note names: %q", where, strings.TrimSpace(head))
			}
		}
		lipgloss.SetColorProfile(old)
	}
}

// TestTheReplyCardWearsTheNumberTheArchiveDraws pins round ninety-one's
// second finding. The reply panel prints the row's digit (#31), and in the
// archive the numbers are the archive's own (#32) — the header takes them
// from `boardRows` for exactly that reason (#248) and the hide note does
// too (#256). The card did not: over a row drawn `▸1 ● api`, under a
// header reading `1 api`, on a fleet listing two rows called `api`, it
// said only `reply to api`, because it read `m.digits` and then dropped
// the digit wherever the archive drew rows. #254's stand — the archive a
// query has cut to none, where the live session keeps its board digit —
// is held beside it.
func TestTheReplyCardWearsTheNumberTheArchiveDraws(t *testing.T) {
	forceASCII(t)
	head := func(m *Model) string {
		for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
			if strings.Contains(row, "reply to ") {
				return row
			}
		}
		return ""
	}
	stand := func(sc scene, w, h int, keys ...string) *Model {
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			where := fmt.Sprintf("%dx%d %v", w, h, prof)

			// The archive lists both hidden `api` rows: `1` and `2`.
			for i, keys := range [][]string{
				{"2", "x", "x", "A", "r"},
				{"2", "x", "x", "A", "j", "r"},
			} {
				m := stand(sceneTwoTools(), w, h, keys...)
				if !m.archiveView || len(m.viewOrder()) < 2 {
					t.Fatalf("%s: the run did not reach an archive drawing both rows", where)
				}
				card := head(m)
				if card == "" {
					t.Fatalf("%s: no reply card on the frame", where)
				}
				want := fmt.Sprintf("reply to %d · api", i+1)
				if !strings.Contains(card, want) {
					t.Errorf("%s: the card does not wear the number the archive draws (%q): %q", where, want, strings.TrimSpace(card))
				}
				// The header on the same frame draws that number.
				if first := strings.Split(ansi.Strip(m.View()), "\n")[0]; !strings.Contains(first, fmt.Sprintf("%d api", i+1)) {
					t.Errorf("%s: the header lost the archive's number: %q", where, strings.TrimSpace(first))
				}
			}

			// Held (#254): an archive drawing no row claims no number of
			// its own, and the card keeps the live session's board digit.
			m := stand(sceneTwoTools(), w, h, "2", "x", "A", "x", "r")
			if !m.archiveView || len(m.viewOrder()) != 0 {
				t.Fatalf("%s: the run did not reach an archive drawing no row", where)
			}
			if card := head(m); !strings.Contains(card, "reply to 2 · api") {
				t.Errorf("%s: the empty archive's card lost the board digit: %q", where, strings.TrimSpace(card))
			}
		}
		lipgloss.SetColorProfile(old)
	}
}

// ---- round 91, second-day ----
// A line sent from a view a search has emptied leaves the trace note and a
// row whose movement key cannot move. The row must not keep that key while
// giving up every key that acts: at eighty the footer read ` j/k move · ?
// help · q quit` beside `↪ sent "please continue" · to ⌁ main:0.0`, naming
// neither the pane it had just written to nor the way deeper, and one
// keypress later — under the shorter note `no row to move to` — both came
// back (#210, #213, #216; §5 keeps the destination clause).
func TestTheSentRowDoesNotKeepAMoveThatCannotMove(t *testing.T) {
	forceASCII(t)
	acts := []string{"enter attach", "tab deeper", "tab reader", "tab session", "r reply", "a ask", "/ search", "g grab", "x hide", "space unfold"}
	checked := 0
	for _, sc := range []scene{sceneSecondDay(), sceneFirstSession(), sceneSubagents(), sceneVeryLong()} {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				old := lipgloss.ColorProfile()
				m := sceneModel(sc, w, h)
				lipgloss.SetColorProfile(prof)
				for _, k := range []string{"/", "zzqqnothing", "enter", "r", "1"} {
					pressKey(m, k)
					poll(m, sc)
				}
				rows := strings.Split(ansi.Strip(m.View()), "\n")
				foot := rows[len(rows)-1]
				lipgloss.SetColorProfile(old)
				if !strings.Contains(foot, "↪ ") {
					t.Fatalf("%s %dx%d %v: no trace note on the sent row: %q", sc.name, w, h, prof, foot)
				}
				if !strings.Contains(foot, mirrorMark) {
					t.Errorf("%s %dx%d %v: the trace lost its destination: %q", sc.name, w, h, prof, foot)
				}
				checked++
				if !strings.Contains(foot, "j/k ") {
					continue
				}
				// The row keeps its movement key: on this frame the view
				// draws no row, so the key cannot move — it may stand
				// only beside a key that acts.
				if len(m.viewOrder()) > 1 {
					continue
				}
				named := false
				for _, a := range acts {
					if strings.Contains(foot, a) {
						named = true
					}
				}
				if !named {
					t.Errorf("%s %dx%d %v: the sent row kept a move that cannot move and named no key that acts: %q", sc.name, w, h, prof, foot)
				}
			}
		}
	}
	// The frame the rule was found on.
	lipgloss.SetColorProfile(termenv.Ascii)
	m := sceneModel(sceneSecondDay(), 80, 24)
	for _, k := range []string{"/", "pytest", "enter", "r", "1"} {
		pressKey(m, k)
		poll(m, sceneSecondDay())
	}
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	foot := rows[len(rows)-1]
	if !strings.Contains(foot, "enter attach") {
		t.Errorf("second-day 80x24 sent row names no way to the pane it wrote to: %q", foot)
	}
	if !strings.Contains(foot, `↪ sent "please continue" · to `+mirrorMark+" main:0.0") {
		t.Errorf("second-day 80x24 sent row lost its trace: %q", foot)
	}
	if checked == 0 {
		t.Fatal("nothing checked")
	}
}

// ---- round 91, fleet-hygiene ----
// Round ninety-one, the fleet-hygiene operator.
//
// #255 made the board's digit read the band the board drew. The list and
// the session view draw the band too — the list into the rows the live
// fleet leaves, oldest dropped first when they run out (#47), the session
// view into the rows the trail leaves — and both drew fewer rows than
// `recentRows(9)` returns, while `openRecent` went on asking that
// could-hold list. So a digit no row on the frame wears opened the
// archive: at eighty `fleet-hygiene` drew four live rows and no band at
// all, and `5` to `9` each opened an archived session the frame never
// named, in an order it never showed.
//
// The fold records the band where it is drawn — `recentLines` for the
// list and the board, the session view's own branch for the trail — and
// the digit reads that, as #255 wrote it: the digit is its row's.

// r91fhRowNum matches a row the frame draws with a number of its own.
var r91fhRowNum = regexp.MustCompile(`^\s{0,3}[▸ ]?(\d) [●○▲◍↻⊘]`)

// r91fhBandRow matches an archived row of the band as drawn.
var r91fhBandRow = regexp.MustCompile(`^\s{1,3}(\d) ○ `)

// r91fhDrawnNums is every number the frame draws on a row of its own.
func r91fhDrawnNums(m *Model) map[int]bool {
	out := map[int]bool{}
	for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
		if mm := r91fhRowNum.FindStringSubmatch(row); mm != nil {
			d, _ := strconv.Atoi(mm[1])
			out[d] = true
		}
		for _, mm := range regexp.MustCompile(`(\d) [●○▲◍↻⊘]`).FindAllStringSubmatch(row, -1) {
			d, _ := strconv.Atoi(mm[1]) // the strip carries the fleet's own digits on one line
			out[d] = true
		}
	}
	return out
}

// r91fhDrawnBandRows is the band as the frame drew it: number and the name
// its row gives, for the rows no live column wears.
func r91fhDrawnBandRows(m *Model) map[int]string {
	live := map[int]bool{}
	for _, r := range m.boardRows() {
		live[r.num] = true
	}
	out := map[int]string{}
	for _, row := range strings.Split(ansi.Strip(m.View()), "\n") {
		mm := r91fhBandRow.FindStringSubmatch(row)
		if mm == nil {
			continue
		}
		d, _ := strconv.Atoi(mm[1])
		if live[d] {
			continue
		}
		rest := strings.TrimSpace(row[strings.Index(row, "○ ")+len("○ "):])
		if i := strings.Index(rest, " · "); i > 0 {
			rest = rest[:i]
		}
		if _, seen := out[d]; !seen {
			out[d] = rest
		}
	}
	return out
}

func r91fhStand(sc scene, w, h int, keys []string) *Model {
	m := sceneModel(sc, w, h)
	_ = m.View()
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
		_ = m.View() // the key is pressed on a drawn frame (#221)
	}
	return m
}

// TestTheListsBandOpensOnlyTheRowsItDraws is the fold on its frame:
// fleet-hygiene at eighty, where the list has no room for the band and
// draws none, beside the same fleet at a hundred, where it draws five.
func TestTheListsBandOpensOnlyTheRowsItDraws(t *testing.T) {
	forceASCII(t)
	var fh scene
	for _, sc := range allScenes() {
		if sc.name == "fleet-hygiene" {
			fh = sc
		}
	}
	if fh.name == "" {
		t.Fatal("no fleet-hygiene scene")
	}
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof) // the frame a person sees (#215, #218)

		// Eighty: four live rows, no band row, and the archive's own line
		// naming `A`. A digit past the fleet's last is no row's.
		base := r91fhStand(fh, 80, 24, nil)
		if got := r91fhDrawnBandRows(base); len(got) != 0 {
			lipgloss.SetColorProfile(old)
			t.Fatalf("80x24: the list draws a band (%v), so the stand is not the stand", got)
		}
		for d := 5; d <= 9; d++ {
			c := r91fhStand(fh, 80, 24, nil)
			pressKey(c, strconv.Itoa(d))
			poll(c, fh)
			if c.archiveView {
				name := "—"
				if s, ok := c.selected(); ok {
					name = sessionName(s.Info)
				}
				t.Errorf("80x24 (%v): `%d` opened the archive on %q over a frame drawing no row that wears it", prof, d, name)
			}
			if note := ansi.Strip(c.note); note != "no session "+strconv.Itoa(d) {
				t.Errorf("80x24 (%v): `%d` said %q, not %q", prof, d, note, "no session "+strconv.Itoa(d))
			}
		}

		// A hundred: the same fleet, the band drawn 5 to 9, every digit
		// opening its own row, and `A` back where it was (#47, #54).
		wide := r91fhStand(fh, 100, 30, nil)
		band := r91fhDrawnBandRows(wide)
		if len(band) != 5 {
			lipgloss.SetColorProfile(old)
			t.Fatalf("100x30: the list draws %d band rows, want 5", len(band))
		}
		for d, want := range band {
			c := r91fhStand(fh, 100, 30, nil)
			was := c.selectedKey
			pressKey(c, strconv.Itoa(d))
			poll(c, fh)
			s, ok := c.selected()
			if !c.archiveView || !ok || s.Live {
				t.Errorf("100x30 (%v): `%d` did not open the archived row its own line names (%q)", prof, d, want)
				continue
			}
			if got := sessionName(s.Info); got != want && !strings.Contains(archiveHeadline(s), want) {
				t.Errorf("100x30 (%v): `%d` opened %q where its row names %q", prof, d, got, want)
			}
			pressKey(c, "A")
			poll(c, fh)
			if c.archiveView || c.selectedKey != was {
				t.Errorf("100x30 (%v): after `%d` then `A` the deck is not back where it was", prof, d)
			}
		}
		lipgloss.SetColorProfile(old)
	}
}

// TestNoDigitOffTheFrameOpensTheArchive is the rule behind it, over every
// scene, every width and both colour profiles, on the stands where each
// of the band's three drawers draws it — the list, the board's stranded
// band, and the session view's rule under the trail.
func TestNoDigitOffTheFrameOpensTheArchive(t *testing.T) {
	forceASCII(t)
	stands := [][]string{nil, {"x"}, {"tab"}}
	off, bandDigits := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := wh[0], wh[1]
				for _, pre := range stands {
					m := r91fhStand(sc, w, h, pre)
					if m.archiveView || m.showHelp || m.searching || m.replying {
						continue // in the archive the digits are its own (#32)
					}
					drawn := r91fhDrawnNums(m)
					band := r91fhDrawnBandRows(m)
					for d := 1; d <= 9; d++ {
						want, isBand := band[d]
						if drawn[d] && !isBand {
							continue // a live row's own digit: #238's question, not this one
						}
						pressKey(m, strconv.Itoa(d)) // pressed on the drawn frame (#221)
						opened := m.archiveView
						s, ok := m.selected()
						opened = opened && ok && !s.Live
						switch {
						case isBand:
							bandDigits++
							if !opened {
								t.Errorf("%s %dx%d %v (%v): `%d` did not open the band row it draws (%q); note %q",
									sc.name, w, h, pre, prof, d, want, ansi.Strip(m.note))
							}
						case opened:
							off++
							name := "—"
							if ok {
								name = sessionName(s.Info)
							}
							t.Errorf("%s %dx%d %v (%v): `%d` opened the archive on %q over a frame that draws no row wearing it",
								sc.name, w, h, pre, prof, d, name)
						}
						if m.archiveView {
							m = r91fhStand(sc, w, h, pre) // back to the frame for the next digit
						} else {
							m.note = "" // a refusal moved nothing: the frame, and its band, still stand
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if bandDigits < 40 {
		t.Fatalf("only %d band digits drawn: the sweep is not sweeping", bandDigits)
	}
	t.Logf("%d band digits drawn and opened, %d digits off the frame opened the archive", bandDigits, off)
}

// ---- round 92, fleet-hygiene ----
// r92fhStand walks a scene to a stand, key by key, as the deck does.
func r92fhStand(sc scene, w, h int, keys ...string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// r92fhFoot is the frame's last row, where the note is drawn.
func r92fhFoot(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return rows[len(rows)-1]
}

// r92fhBackNote is the unhide note's digit and name, or ("", "") where the
// frame carries no unhide note.
var r92fhBack = regexp.MustCompile(`(\d)? ?([^·]+?) is back on the board`)

// TestTheUnhideNoteWearsNoNumberTheFrameGivesAnother is the frame:
// fleet-hygiene, where the archive holds forty-one finished sessions and
// numbers its own rows from one. `x` there puts a hidden session back on the
// board, and the frame that follows draws that number on another session's
// row — and in its header. The note must not spend it (#245, #256).
func TestTheUnhideNoteWearsNoNumberTheFrameGivesAnother(t *testing.T) {
	forceASCII(t)
	sc := sceneFleetHygiene()
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			where := fmt.Sprintf("%dx%d %v", w, h, prof)

			// `1 porter` off the board, then the archive, then back.
			// The archive lists the still-hidden nothing and re-numbers
			// its own rows: `1` is the first archived session.
			m := r92fhStand(sc, w, h, "x", "A", "x")
			foot := r92fhFoot(m)
			if !strings.Contains(foot, "porter is back on the board") {
				t.Fatalf("%s: `x` did not put porter back: %q", where, foot)
			}
			if strings.Contains(foot, "1 porter is back on the board") {
				t.Errorf("%s: the note wears a number the frame gives another row: %q", where, foot)
			}
			if row, ok := m.boardRows()["porter"]; ok && row.num > 0 {
				t.Errorf("%s: the archive still draws porter as %d", where, row.num)
			}
			drawn := ""
			for key, row := range m.boardRows() {
				if row.num == 1 {
					drawn = key
				}
			}
			if drawn == "" || drawn == "porter" {
				t.Errorf("%s: the frame draws no other row wearing 1 (%q)", where, drawn)
			}

			// The two `harness` are the reason #257 gave the note a
			// digit. Hide the working one, come back, and the archive is
			// again drawing that number on a row of its own.
			m = r92fhStand(sc, w, h, "2", "x", "A", "x")
			foot = r92fhFoot(m)
			if !strings.Contains(foot, "harness is back on the board") {
				t.Fatalf("%s: `x` did not put harness back: %q", where, foot)
			}
			if strings.Contains(foot, "2 harness is back on the board") {
				t.Errorf("%s: the note wears a number the frame gives another row: %q", where, foot)
			}
			two := ""
			for key, row := range m.boardRows() {
				if row.num == 2 {
					two = key
				}
			}
			if two == "" || strings.HasPrefix(two, "harness") {
				t.Errorf("%s: the frame draws no other row wearing 2 (%q)", where, two)
			}
		}
		lipgloss.SetColorProfile(old)
	}
}

// TestNoUnhideNoteSpendsADigitAnotherRowWears is the rule behind it, over
// every scene and every width: whatever number an unhide note wears, no
// other row of the frame it stands on wears it. One digit, one session
// (#32, #245, #256).
func TestNoUnhideNoteSpendsADigitAnotherRowWears(t *testing.T) {
	forceASCII(t)
	checked := 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := size[0], size[1]
				for _, script := range [][]string{
					{"x", "A", "x"},
					{"2", "x", "A", "x"},
					{"3", "x", "A", "x"},
					{"x", "x", "A", "x", "x"},
					{"2", "x", "3", "x", "A", "x", "x"},
				} {
					m := sceneModel(sc, w, h)
					for _, k := range script {
						pressKey(m, k)
						poll(m, sc)
						if !strings.HasSuffix(m.note, " is back on the board") &&
							!strings.Contains(m.note, " is back on the board · ") {
							continue
						}
						checked++
						mm := r92fhBack.FindStringSubmatch(m.note)
						if mm == nil || mm[1] == "" {
							continue // no digit spent: nothing to collide
						}
						num, _ := strconv.Atoi(mm[1])
						for key, row := range m.boardRows() {
							if row.num == num && !strings.Contains(mm[2], sessionNameFor(m, key)) {
								t.Errorf("%s %dx%d %v %v: note %q wears %d, the number the frame draws on %q",
									sc.name, w, h, prof, script, m.note, num, key)
							}
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if checked == 0 {
		t.Fatal("no unhide note was reached: the sweep proves nothing")
	}
	t.Logf("unhide notes checked: %d", checked)
}

// sessionNameFor is the name the frame draws for a key.
func sessionNameFor(m *Model, key string) string {
	for _, s := range m.sessions {
		if s.Info.Key() == key {
			return sessionName(s.Info)
		}
	}
	return key
}

// ---- round 92, two-tools ----
// r92ttStand plays keys into a fresh scene and returns the deck.
func r92ttStand(sc scene, w, h int, keys ...string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// r92ttCells splits one drawn row into the board's columns, in order.
func r92ttCells(line string) []string {
	return strings.Split(line, "│")
}

// r92ttTools are the tool words a row can wear (#50, #80).
var r92ttTools = []string{"claude", "opencode", "codex"}

// r92ttSaysToolTwice walks a frame and reports every column where a row
// and the row directly under it, in the same column, both name the same
// tool word as one of their own clauses.
func r92ttSaysToolTwice(view string) []string {
	rows := strings.Split(ansi.Strip(view), "\n")
	var bad []string
	for i := 1; i < len(rows); i++ {
		above, below := r92ttCells(rows[i-1]), r92ttCells(rows[i])
		if len(above) != len(below) {
			continue
		}
		for k := range below {
			up, down := strings.TrimSpace(above[k]), strings.TrimSpace(below[k])
			if up == "" || down == "" {
				continue
			}
			for _, tool := range r92ttTools {
				if hasClause(up, tool) && hasClause(down, tool) {
					bad = append(bad, fmt.Sprintf("row %d: %q over %q", i, up, down))
				}
			}
		}
	}
	return bad
}

// hasClause says whether one of the row's " · "-separated clauses is word.
func hasClause(row, word string) bool {
	for _, c := range strings.Split(row, " · ") {
		if strings.TrimSpace(c) == word {
			return true
		}
	}
	return false
}

// TestTheArchiveBoardSaysItsToolOnce pins the archive board's card. The
// card's third row is the tag row, always (#107, #112): the tool word and
// its model stand there. The row above it is the archive list's own second
// line, which names the tool where the archive holds two (#80) — so the
// card drew `claude · main` over `claude · opus-4-1 · ⌁ dev:1.0`, and
// where the archived session has neither model nor pane a whole row that
// was the word and nothing else. #53 already took the pane off that row
// because the tag row below draws it, on the reason §4 states and #64
// names: a line answers a question once. The word goes the same way, and
// the branch and the verdict — which the tag row cannot say — take the
// cells back.
func TestTheArchiveBoardSaysItsToolOnce(t *testing.T) {
	forceASCII(t)
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			where := fmt.Sprintf("%dx%d %v", w, h, prof)

			// Two live `api` sessions hidden, the archive drawing both,
			// `⇧tab board` — the key that footer names — on the board.
			m := r92ttStand(sceneTwoTools(), w, h, "2", "x", "x", "A", "shift+tab")
			if !m.archiveView || !m.boardShown() || len(m.viewOrder()) != 2 {
				t.Fatalf("%s: the run did not reach an archive board of two rows", where)
			}
			view := ansi.Strip(m.View())
			for _, bad := range r92ttSaysToolTwice(view) {
				t.Errorf("%s: the archive board's card says its tool twice — %s", where, bad)
			}
			// The word is on the tag row, which is where the model and
			// the pane are: the card still answers which tool.
			for _, want := range []string{
				"opencode · sonnet-4-5 · " + mirrorMark + " dev:2.0",
				"claude · opus-4-1 · " + mirrorMark + " dev:1.0",
			} {
				if !strings.Contains(view, want) {
					t.Errorf("%s: the card's tag row lost its identity %q", where, want)
				}
			}
			// And the row above it keeps the branch, which the tag cannot say.
			for _, line := range strings.Split(view, "\n") {
				for _, c := range r92ttCells(line) {
					if strings.TrimSpace(c) == "opencode · main" || strings.TrimSpace(c) == "claude · main" {
						t.Errorf("%s: the card's row still names the tag row's word: %q", where, strings.TrimSpace(c))
					}
				}
			}

			// The archive of finished sessions, where the tag row is the
			// word alone: the same rule, and the row spends the cells it
			// gets back on the verdict the archive row exists for (#47).
			m = r92ttStand(sceneSecondDay(), w, h, "A", "shift+tab")
			if !m.archiveView || !m.boardShown() {
				t.Fatalf("%s: the run did not reach second-day's archive board", where)
			}
			view = ansi.Strip(m.View())
			for _, bad := range r92ttSaysToolTwice(view) {
				t.Errorf("%s: second-day's archive board says its tool twice — %s", where, bad)
			}
			if !strings.Contains(view, "feat/etl-v2 · ✓ green 212✓") {
				t.Errorf("%s: the row did not spend the cells on the verdict it could not fit", where)
			}

			// Held at HEAD: the archive *list* keeps its word, which is
			// what tells a row from its namesake where no tag row is
			// drawn (#80, #196).
			m = r92ttStand(sceneTwoTools(), w, h, "2", "x", "x", "A")
			list := ansi.Strip(m.View())
			if !strings.Contains(list, "claude · "+mirrorMark+" dev:1.0 · main") {
				t.Errorf("%s: the archive list lost the word that tells its two api rows apart", where)
			}
		}
		lipgloss.SetColorProfile(old)
	}
}

// ---- round 92, second-day ----
// Round ninety-two, the second-day operator.
//
// #259 folded the rule that a line sent from a list a search had emptied
// does not leave a footer whose only session key cannot move. In the
// archive the same search, the same reply and the same trace left
// ` j/k move · A fleet · ? help · q quit  ↪ sent "please continue" · to
// ⌁ main:0.0` — `j` there answers `no row to move to`, and one keypress
// later, under the shorter note, `enter attach` came back. #259's yield
// was taken and refused for width: once `j/k move · ` had left the row's
// head the separator-led ` · enter attach` matched nothing, so the
// yielded row could not shed the attach key and did not fit. The board
// and the list carry the attach key's head forms now, as the reader's own
// list has since #56 and #200.
func TestTheSentRowInTheArchiveDoesNotKeepAMoveThatCannotMove(t *testing.T) {
	forceASCII(t)
	acts := []string{"enter attach", "tab deeper", "tab reader", "tab session", "r reply", "a ask", "/ search", "g grab", "x hide", "space unfold"}
	checked := 0
	for _, sc := range []scene{sceneSecondDay(), sceneFirstSession(), sceneManyIdle(), sceneVeryLong()} {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				old := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(prof)
				m := sceneModel(sc, w, h)
				for _, k := range []string{"/", "zzqqnothing", "enter", "A", "r", "1"} {
					pressKey(m, k)
					poll(m, sc)
				}
				rows := strings.Split(ansi.Strip(m.View()), "\n")
				foot := rows[len(rows)-1]
				lipgloss.SetColorProfile(old)
				if !strings.Contains(foot, "↪ ") {
					t.Fatalf("%s %dx%d %v: no trace note on the sent row: %q", sc.name, w, h, prof, foot)
				}
				if !strings.Contains(foot, mirrorMark) {
					t.Errorf("%s %dx%d %v: the trace lost its destination: %q", sc.name, w, h, prof, foot)
				}
				checked++
				if !strings.Contains(foot, "j/k ") {
					continue
				}
				// The archive this frame draws holds no row, so the
				// movement key cannot move: it may stand only beside a
				// key that acts (#210, #213, #216, #259).
				if len(m.viewOrder()) > 1 {
					continue
				}
				named := false
				for _, a := range acts {
					if strings.Contains(foot, a) {
						named = true
					}
				}
				if !named {
					t.Errorf("%s %dx%d %v: the archive's sent row kept a move that cannot move and named no key that acts: %q", sc.name, w, h, prof, foot)
				}
			}
		}
	}
	// The frame the rule was found on: second-day at eighty, the archive
	// a search has emptied, one canned reply sent into hello's pane.
	lipgloss.SetColorProfile(termenv.Ascii)
	m := sceneModel(sceneSecondDay(), 80, 24)
	for _, k := range []string{"/", "pytest", "enter", "A", "1", "2", "3", "9", "r", "1"} {
		pressKey(m, k)
		poll(m, sceneSecondDay())
	}
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	foot := rows[len(rows)-1]
	if strings.Contains(foot, "j/k ") {
		t.Errorf("second-day 80x24 archive sent row keeps a move that cannot move: %q", foot)
	}
	if !strings.Contains(foot, `↪ sent "please continue" · to `+mirrorMark+" main:0.0") {
		t.Errorf("second-day 80x24 archive sent row lost its trace: %q", foot)
	}
	if !strings.Contains(foot, "A fleet") {
		t.Errorf("second-day 80x24 archive sent row lost the way home: %q", foot)
	}
	if checked == 0 {
		t.Fatal("nothing checked")
	}
}

// ---- round 93, second-day ----
// ---- round 93, second-day, the one thing ----
// The archive's hide refusal keeps the way deeper. `x` on an archived row
// answered `the archive is already off the board` — 36 cells, the longest
// note the deck writes, against the 21 the eighty-column archive footer
// leaves — and the row it left named `j/k move · A fleet · ? help · q
// quit`: `tab deeper` gone, the frame's only naming of the way deeper,
// and `a ask` and `/ search` with it. That is the harm #175, #187, #190,
// #194 and #198 each folded, on the one refusal with no yield. Its
// subject is the word the frame supplies twice — the column's own title
// `▌FLEET · archive` and the header's `archive 12` chip — so it yields,
// leaving `it is off the board` (19 cells), and only where a key naming a
// level comes back for it. Its routes never pressed `tab`, so the
// archive's session view went on paying: round 104 took the last three
// cells and the note answers the key's own question — `it is not
// hidden`, sixteen cells — which is what keeps `tab deeper` there at
// eighty.
func TestTheArchivesHideRefusalKeepsTheWayDeeper(t *testing.T) {
	forceASCII(t)
	levelKey := func(foot string) bool {
		for _, k := range []string{"tab deeper", "enter attach", "tab session", "tab reader"} {
			if strings.Contains(foot, k) {
				return true
			}
		}
		return false
	}
	foot := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	// The frame it was found on: the second day's archive at eighty, one
	// `A` and one `x` from the opening board.
	sc := sceneSecondDay()
	m := sceneModel(sc, 80, 24)
	for _, k := range []string{"A", "x"} {
		pressKey(m, k)
		poll(m, sc)
	}
	if got, want := foot(m), " j/k move · tab deeper · a ask · A fleet · ? help · q quit     it is not hidden"; got != want {
		t.Errorf("second-day 80x24 A x:\n got  %q\n want %q", got, want)
	}
	// The rule, over every scene at every width under both colour
	// profiles: where `x` in the archive is refused, the row it draws
	// keeps a key naming a level if the row one keypress earlier had one.
	routes := [][]string{{"A"}, {"A", "j"}, {"A", "j", "j"}, {"/", "pytest", "enter", "A"}}
	for _, sc := range allScenes() {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				old := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(prof)
				for _, route := range routes {
					m := sceneModel(sc, size[0], size[1])
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					before := foot(m)
					pressKey(m, "x")
					poll(m, sc)
					after := foot(m)
					if !strings.HasSuffix(after, "it is not hidden") || strings.Contains(after, "is hidden") {
						continue // the key acted, or answered something else
					}
					if levelKey(before) && !levelKey(after) {
						t.Errorf("%s %dx%d %v %v then x: the archive's hide refusal costs the frame its only naming of the way deeper:\n before %q\n after  %q",
							sc.name, size[0], size[1], prof, route, strings.TrimSpace(before), strings.TrimSpace(after))
					}
				}
				lipgloss.SetColorProfile(old)
			}
		}
	}
}

// ---- round 93, fleet-hygiene ----
// The archive's `x` refusal names the row, not the view, and pays for
// itself out of no key that acts (#24, #52, #210, and #263's own record).
//
// On an archived row `x` cannot take a session off a board it left long
// ago, and it answers. The answer was "the archive is already off the
// board" — thirty-six cells about the view, under a caret standing on one
// session — and at eighty the footer beside it shed `tab deeper`, `a ask`
// and `/ search`, three keys that act on that very row, for a sentence
// about a key the footer does not offer; at 100 and 120 it shed
// `/ search` too. The nineteen-cell form names the row the caret is on and
// buys them back.
//
// Both stands are measured with colour on as well as under `forceASCII`
// (#215, #218): the footer is read through `ansi.Strip`, so a styled
// clause must be found either way.

// r93fhArchiveRefusal presses `A`, `j`, `x` and hands back the footer
// before the press and the footer after it, or ok=false where that scene's
// archive has no archived row for `x` to refuse.
func r93fhArchiveRefusal(sc scene, w, h int) (before, after string, ok bool) {
	m := sceneModel(sc, w, h)
	for _, k := range []string{"A", "j"} {
		pressKey(m, k)
		poll(m, sc)
	}
	rows := strings.Split(m.View(), "\n")
	before = ansi.Strip(rows[len(rows)-1])
	pressKey(m, "x")
	poll(m, sc)
	rows = strings.Split(m.View(), "\n")
	after = ansi.Strip(rows[len(rows)-1])
	if !strings.Contains(after, "it is not hidden") || strings.Contains(after, "is back on the board") {
		return "", "", false
	}
	return before, after, true
}

// r93fhNoteOf is the note the footer carries — what stands after the wide
// gap that separates the keys from the sentence.
func r93fhNoteOf(footer string) string {
	f := strings.TrimRight(footer, " ")
	if i := strings.LastIndex(f, "  "); i >= 0 {
		return strings.TrimSpace(f[i:])
	}
	return ""
}

var r93fhSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

var r93fhProfiles = []struct {
	name string
	prof termenv.Profile
}{{"ascii", termenv.Ascii}, {"colour", termenv.TrueColor}}

// TestTheArchivesOffTheBoardNoteNamesTheRowNotTheView is the frame: the
// note is about the session the caret stands on and fits in sixteen
// cells, at every width and both profiles. (Round 104 took the last
// three: the note answers the key's own question, `it is not hidden`,
// which is what keeps `tab deeper` on the archive's session view at
// eighty — the row this pin's routes never reached.)
func TestTheArchivesOffTheBoardNoteNamesTheRowNotTheView(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	stands := 0
	for _, p := range r93fhProfiles {
		for _, sc := range allScenes() {
			for _, size := range r93fhSizes {
				lipgloss.SetColorProfile(p.prof)
				_, after, ok := r93fhArchiveRefusal(sc, size[0], size[1])
				if !ok {
					continue
				}
				stands++
				note := r93fhNoteOf(after)
				if strings.Contains(note, "archive") {
					t.Errorf("%s %s %dx%d: the note about the row names the view instead: %q",
						p.name, sc.name, size[0], size[1], note)
				}
				if n := lipgloss.Width(note); n > 16 {
					t.Errorf("%s %s %dx%d: the note is %d cells, sixteen is the room the footer can spare: %q",
						p.name, sc.name, size[0], size[1], n, note)
				}
			}
		}
	}
	if stands == 0 {
		t.Fatal("no scene reached the archive's off-the-board refusal")
	}
	t.Logf("off-the-board notes checked: %d", stands)
}

// TestTheArchivesOffTheBoardNoteCostsNoKeyThatActs is the rule: the keys
// the frame named one press earlier and that act on the row it stands on
// are still named beside the note.
func TestTheArchivesOffTheBoardNoteCostsNoKeyThatActs(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	stands := 0
	for _, p := range r93fhProfiles {
		for _, sc := range allScenes() {
			for _, size := range r93fhSizes {
				lipgloss.SetColorProfile(p.prof)
				before, after, ok := r93fhArchiveRefusal(sc, size[0], size[1])
				if !ok {
					continue
				}
				stands++
				// `tab deeper` and `a ask` at every width; `/ search`
				// from a hundred columns up. At eighty the keys alone
				// fill sixty-nine of the eighty cells, so no sentence
				// that says anything can keep `/ search` there — the
				// sixteen-cell form buys back the two that fit.
				keys := []string{"tab deeper", "a ask"}
				if size[0] >= 100 {
					keys = append(keys, "/ search")
				}
				for _, key := range keys {
					if strings.Contains(before, key) && !strings.Contains(after, key) {
						t.Errorf("%s %s %dx%d: the refusal spent %q, a key that acts on the row it stands on\n  before: %q\n  after : %q",
							p.name, sc.name, size[0], size[1], key,
							strings.TrimRight(before, " "), strings.TrimRight(after, " "))
					}
				}
			}
		}
	}
	if stands == 0 {
		t.Fatal("no scene reached the archive's off-the-board refusal")
	}
	t.Logf("off-the-board footers checked: %d", stands)
}

// ---- round 93, two-tools ----
// ---- round 93, two-tools ----

// r93ttStand plays keys into a fresh scene and returns the deck.
func r93ttStand(sc scene, w, h int, keys ...string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// r93ttScene finds a scene by name.
func r93ttScene(t *testing.T, name string) scene {
	t.Helper()
	for _, sc := range allScenes() {
		if sc.name == name {
			return sc
		}
	}
	t.Fatalf("no scene %q", name)
	return scene{}
}

// r93ttCells splits a board row into its columns, keyed by the cell the
// column begins at, so a head row and the ◉ row three rows below it are
// compared inside one card and never across the gutter.
func r93ttCells(row string) map[int]string {
	out := map[int]string{}
	start, cell := 0, 0
	bare := ansi.Strip(row)
	for i, ch := range bare {
		if ch == '│' {
			out[start] = bare[start:i]
			start = cell + 1
		}
		cell = i + len(string(ch))
	}
	out[start] = bare[start:]
	return out
}

var r93ttClock = regexp.MustCompile(`\s+\S+ ago$`)

// r93ttSaid is what a drawn ◉ row says, less its glyph, quotes and clock.
func r93ttSaid(seg string) string {
	s := strings.TrimSpace(seg)
	rest, ok := strings.CutPrefix(s, "◉ ")
	if !ok {
		return ""
	}
	return strings.Trim(strings.TrimSpace(r93ttClock.ReplaceAllString(rest, "")), `"…`)
}

// TestTheArchiveBoardSaysWhatWasAskedOnce: on the archive's board (`A`,
// then `⇧tab board`) the card draws the prompt on its own ◉ row, with the
// ask's own clock. Its head row drew the same sentence three rows up —
// whole on a wide terminal, the clipped copy of a whole one at 120 — so a
// card nine rows tall spent two of them on one sentence (#64, #107). The
// head keeps the name the archive gives the session: the one its person
// typed (#79, #234), else the project it is grouped under (#11), which the
// archive's board draws on no other row. The archive's *list*, which draws
// no ◉ row of its own beside the row, keeps the ask (#56, #86) — the held
// side, and it passes at HEAD too.
func TestTheArchiveBoardSaysWhatWasAskedOnce(t *testing.T) {
	forceASCII(t)
	tt := r93ttScene(t, "two-tools")
	sd := r93ttScene(t, "second-day")
	cases := []struct {
		sc   scene
		keys []string
	}{
		{tt, []string{"2", "x", "x", "A", "shift+tab"}},
		{sd, []string{"A", "shift+tab"}},
	}
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
			for _, c := range cases {
				m := r93ttStand(c.sc, size[0], size[1], c.keys...)
				rows := strings.Split(ansi.Strip(m.View()), "\n")
				if !strings.Contains(rows[0], "· board") {
					t.Fatalf("%s %dx%d: not on the archive's board: %q", c.sc.name, size[0], size[1], strings.TrimSpace(rows[0]))
				}
				said := 0
				for j, row := range rows {
					if !strings.Contains(row, "◉") {
						continue
					}
					for at, seg := range r93ttCells(row) {
						ask := r93ttSaid(seg)
						if len(strings.Fields(ask)) < 2 {
							continue
						}
						said++
						for k := max(0, j-4); k < j; k++ {
							head, ok := r93ttCells(rows[k])[at]
							if !ok {
								continue
							}
							bare := strings.TrimSpace(strings.Trim(strings.TrimSpace(head), "…"))
							if !strings.Contains(bare, "○") && !strings.Contains(bare, "●") {
								continue
							}
							if strings.Contains(bare, ask) || strings.Contains(bare, strings.TrimRight(ask, "…")) {
								t.Errorf("%s %dx%d %v: the archive board's card says what was asked twice — %q over %q",
									c.sc.name, size[0], size[1], prof, strings.TrimSpace(head), strings.TrimSpace(seg))
							}
						}
					}
				}
				if said == 0 {
					t.Errorf("%s %dx%d %v: no card drew a ◉ row — the stand is wrong", c.sc.name, size[0], size[1], prof)
				}
				// The head still names the session, and the board now
				// names the project it never drew.
				body := strings.Join(rows, "\n")
				want := map[string][]string{
					"two-tools":  {"1 ● api", "2 ● api"},
					"second-day": {"3 ○ checkout-flake-hunt", "○ webapp", "○ etl"},
				}[c.sc.name]
				for _, w := range want {
					if !strings.Contains(body, w) {
						t.Errorf("%s %dx%d %v: the head lost the name the archive gives the session: no %q", c.sc.name, size[0], size[1], prof, w)
					}
				}
			}
			// The archive's list draws no ◉ row beside its rows, so it
			// keeps the ask (#56, #86): the held side.
			l := r93ttStand(sd, size[0], size[1], "A")
			if got := ansi.Strip(l.View()); !strings.Contains(got, "○ port the client to the new sdk") {
				t.Errorf("second-day %dx%d %v: the archive list lost the ask its row is named by", size[0], size[1], prof)
			}
		}
		lipgloss.SetColorProfile(old)
	}
}

// ---- round 94, fleet-hygiene ----
// r94fhStand opens a scene at a size and presses the keys, polling as the
// deck does after every press.
func r94fhStand(sc scene, w, h int, keys ...string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// r94fhScene is the scene of that name, from the walkthrough's own set.
func r94fhScene(t *testing.T, name string) scene {
	t.Helper()
	for _, sc := range allScenes() {
		if sc.name == name {
			return sc
		}
	}
	t.Fatalf("no scene %q", name)
	return scene{}
}

// r94fhHeads is every card head row the board draws: the segments between
// the column rules that begin with a state glyph, less the trailing air.
func r94fhHeads(view string) []string {
	var out []string
	for _, row := range strings.Split(ansi.Strip(view), "\n") {
		for _, seg := range strings.Split(row, "│") {
			bare := strings.TrimRight(seg, " ")
			trimmed := strings.TrimLeft(bare, " ▸0123456789")
			if !strings.HasPrefix(trimmed, "○ ") && !strings.HasPrefix(trimmed, "● ") {
				continue
			}
			out = append(out, strings.TrimSpace(bare))
		}
	}
	return out
}

// TestNoTwoArchiveBoardCardsDrawTheSameHeadRow is the rule #265 owes its
// own reason: the head yields what was asked only where the row it leaves
// still tells this card from every other card the frame draws. The band
// caps at nine digits (#260), so on an archive of one project the yield
// drew five cards headed `○ api  2d` and nothing else — one row, five
// sessions (#86).
func TestNoTwoArchiveBoardCardsDrawTheSameHeadRow(t *testing.T) {
	forceASCII(t)
	stands := 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				m := r94fhStand(sc, size[0], size[1], "A", "shift+tab")
				rows := strings.Split(ansi.Strip(m.View()), "\n")
				if !strings.Contains(rows[0], "· board") {
					continue // no board under 110 columns
				}
				stands++
				seen := map[string]bool{}
				for _, head := range r94fhHeads(m.View()) {
					if seen[head] {
						t.Errorf("%s %dx%d %v: two archive cards draw the same head row — %q",
							sc.name, size[0], size[1], prof, head)
					}
					seen[head] = true
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if stands < 40 {
		t.Fatalf("only %d archive-board stands — the stand is wrong", stands)
	}
}

// TestTheArchiveBoardKeepsTheAskWhereTheNameCannotTellTheCardsApart is the
// frame: on an archive whose sessions share one project the head keeps
// what was asked, and where the name does tell them apart it still yields
// it to the card's own ◉ row (#265).
func TestTheArchiveBoardKeepsTheAskWhereTheNameCannotTellTheCardsApart(t *testing.T) {
	forceASCII(t)
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, size := range [][2]int{{152, 40}, {220, 48}} {
			// One project, forty archived sessions: the head keeps the ask.
			m := r94fhStand(r94fhScene(t, "fleet-hygiene"), size[0], size[1], "A", "shift+tab")
			got := ansi.Strip(m.View())
			if !strings.Contains(got, "○ port the client to the n") && !strings.Contains(got, "○ port the client to the new sdk") {
				t.Errorf("fleet-hygiene %dx%d %v: the archive board's card head lost the ask the project cannot say", size[0], size[1], prof)
			}
			// The typed name still stands where the archive gives it one (#79).
			if !strings.Contains(got, "○ the pane I closed half a") {
				t.Errorf("fleet-hygiene %dx%d %v: the archive board's card head lost the name its person typed", size[0], size[1], prof)
			}
			// Many projects: the head still yields the ask to the ◉ row (#265).
			sd := r94fhStand(r94fhScene(t, "second-day"), size[0], size[1], "A", "shift+tab")
			for _, want := range []string{"○ webapp", "○ etl", "○ checkout-flake-hunt"} {
				if !strings.Contains(ansi.Strip(sd.View()), want) {
					t.Errorf("second-day %dx%d %v: the head lost the name the archive gives the session: no %q", size[0], size[1], prof, want)
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
}

// ---- round 94, second-day ----
// ---- round 94, second-day, the one thing ----
// The row that shipped does not say the ask it shortened. #192 folded
// `◆ ship   fix the 401 on token refresh…` — a clipped copy of the ask the
// identity header and the ◉ row both draw whole, with `(commit)` thrown
// away — but keyed the fold on `nameAndBracket`, the *whole* ask with a
// bracket after it. A commit subject is the ask shortened at a word, so
// every ask longer than its own commit subject fell through: one `j` below
// the row #192 fixed, `A` then `2` at eighty drew
// `◆ ship   port the client to the new…` under a header reading
// `2 port the client to the new sdk` and a ◉ row drawing it whole again.
// Where the label's subject is the ask's own leading words cut at a word
// and the row will not fit, the row draws the bracket's word: `◆ ship
// commit` (#64's device on the reader's title, #189, #192).
func TestTheShipRowIsNotAClippedCopyOfTheAskItShortened(t *testing.T) {
	forceASCII(t)

	// The frame it was found on.
	m := sceneModel(sceneSecondDay(), 80, 24)
	for _, k := range []string{"A", "2"} {
		pressKey(m, k)
	}
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	var ship string
	for _, l := range rows {
		for _, seg := range strings.Split(l, "│") {
			if strings.Contains(seg, "◆ ship") {
				ship = strings.TrimSpace(seg)
			}
		}
	}
	if ship == "" {
		t.Fatalf("80x24 A,2: no ship row on the frame:\n%s", strings.Join(rows, "\n"))
	}
	if !strings.HasPrefix(ship, "◆ ship   commit") {
		t.Errorf("80x24 A,2: the ship row spends itself on the ask again: %q", ship)
	}

	// The rule, over every scene, five widths, both colour profiles and
	// four routes: a ship row's clipped clause is never a clause the same
	// frame draws whole somewhere else.
	clipped := regexp.MustCompile(`ship\s+(\S[^│]*?)…`)
	routes := [][]string{{"A"}, {"A", "2"}, {"A", "2", "tab"}, {"A", "shift+tab"}}
	for _, sc := range allScenes() {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				old := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(prof)
				for _, route := range routes {
					m := sceneModel(sc, size[0], size[1])
					for _, k := range route {
						pressKey(m, k)
					}
					rows := strings.Split(ansi.Strip(m.View()), "\n")
					for _, l := range rows {
						for _, seg := range strings.Split(l, "│") {
							mt := clipped.FindStringSubmatch(seg)
							if mt == nil {
								continue
							}
							clause := strings.TrimSpace(mt[1])
							if len([]rune(clause)) < 10 {
								continue
							}
							for _, other := range rows {
								if other == l {
									continue
								}
								i := strings.Index(other, clause)
								if i < 0 {
									continue
								}
								if !strings.HasPrefix(other[i+len(clause):], "…") {
									t.Errorf("%s %dx%d %v %v: the ship row is a clipped copy of a clause the frame draws whole: %q under %q",
										sc.name, size[0], size[1], prof, route,
										strings.TrimSpace(seg), strings.TrimSpace(other))
								}
							}
						}
					}
				}
				lipgloss.SetColorProfile(old)
			}
		}
	}
}

// ---- round 94, two-tools ----
// r94ttStand plays a key run on a scene at a size and hands back the model.
func r94ttStand(sc scene, w, h int, keys ...string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// r94ttScene finds a scene by name.
func r94ttScene(t *testing.T, name string) scene {
	t.Helper()
	for _, sc := range allScenes() {
		if sc.name == name {
			return sc
		}
	}
	t.Fatalf("no scene %q", name)
	return scene{}
}

// r94ttRows is the drawn frame, escapes stripped and trailing pad ignored.
func r94ttRows(m *Model) []string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	for i, r := range rows {
		rows[i] = strings.TrimRight(r, " ")
	}
	return rows
}

// r94ttFooter is the frame's last drawn row: the keymap and its note.
func r94ttFooter(rows []string) string {
	for i := len(rows) - 1; i >= 0; i-- {
		if strings.TrimSpace(rows[i]) != "" {
			return rows[i]
		}
	}
	return ""
}

// r94ttFleetTitle is the row the fleet column's title stands on, or "".
func r94ttFleetTitle(rows []string) string {
	for _, r := range rows {
		if strings.Contains(r, "FLEET · ") {
			return r
		}
	}
	return ""
}

// r94ttKeys is the set of key fragments a footer names, less any note: the
// fragments before the note's own gap, split on the separator the footer
// uses.
func r94ttKeys(footer string) map[string]bool {
	out := map[string]bool{}
	keys := footer
	if i := strings.Index(footer, "  "); i > 0 {
		keys = footer[:i]
	}
	for _, k := range strings.Split(keys, " · ") {
		if k = strings.TrimSpace(k); k != "" {
			out[k] = true
		}
	}
	return out
}

// TestTheEmptyArchiveIsNotABoard: an archive with nothing in it has no
// board, and the deck does not stand at the board's level over it.
//
// `⇧tab` on the archive's list dropped the deck to the board level while
// `boardShown` was false, so the deck drew the list again one level down:
// the same rows, the same board-less keymap, and the fleet's title stripped
// of `[fleet]`, the word that says where the keys are (#20, #63) — 96 such
// frames over the runs this round measured, none of them marking a row and
// none wearing the word, against 1,197 of 1,197 at the list's own level.
// The look on the trail beside it was committed on the way down, so `you
// were here` left a trail still drawn. `⇧tab` pressed again then answered
// `the board is the top` over a frame with no board on it.
//
// The level follows the frame: where the board fits but has no session to
// put in a column, `⇧tab` refuses with `no board` — eight cells, which
// costs no key the footer names at any width, the row that says why (`no
// archived sessions`) being on the frame already (#64) — and where the last
// row leaves the archive's board under the keys, the deck falls back to the
// list's level with it. The archive that still draws rows keeps its board
// (#262, #265): the held side, which passes at HEAD too.
func TestTheEmptyArchiveIsNotABoard(t *testing.T) {
	forceASCII(t)
	scenes := []scene{r94ttScene(t, "two-tools"), r94ttScene(t, "subagents")}
	sd := r94ttScene(t, "second-day")
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			for _, sc := range scenes {
				// The archive both `api`s were hidden into, emptied again.
				m := r94ttStand(sc, w, h, "2", "x", "x", "A", "x", "x")
				before := r94ttRows(m)
				if !strings.Contains(strings.Join(before, "\n"), "no archived sessions") {
					t.Fatalf("%s %dx%d %v: the stand is wrong — the archive is not empty", sc.name, w, h, prof)
				}
				if m.level == levelBoard {
					t.Fatalf("%s %dx%d %v: the stand is wrong — already at the board's level", sc.name, w, h, prof)
				}
				pressKey(m, "shift+tab")
				poll(m, sc)
				after := r94ttRows(m)
				if m.level == levelBoard {
					t.Errorf("%s %dx%d %v: ⇧tab took the deck to the board's level over an archive with no board on it", sc.name, w, h, prof)
				}
				if title := r94ttFleetTitle(after); !strings.Contains(title, "[fleet]") {
					t.Errorf("%s %dx%d %v: the drawn list lost the word that says where the keys are: %q", sc.name, w, h, prof, strings.TrimSpace(title))
				}
				foot := r94ttFooter(after)
				if !strings.HasSuffix(strings.TrimRight(foot, " "), "no board") {
					t.Errorf("%s %dx%d %v: ⇧tab answered %q, not `no board`", sc.name, w, h, prof, strings.TrimSpace(foot))
				}
				kept := r94ttKeys(foot)
				for k := range r94ttKeys(r94ttFooter(before)) {
					has := false
					for got := range kept {
						// A key the reserve backfilled says more, not
						// less: `enter attach (prefix d returns)` (#39).
						if got == k || strings.HasPrefix(got, k+" ") || strings.HasPrefix(k, got+" ") {
							has = true
						}
					}
					if !has {
						t.Errorf("%s %dx%d %v: the refusal cost the footer %q: %q", sc.name, w, h, prof, k, strings.TrimSpace(foot))
					}
				}
				// Nothing but the note moved: the same rows, the trail's
				// own marker among them.
				for i := 0; i < len(before)-1 && i < len(after)-1; i++ {
					if before[i] != after[i] && !strings.Contains(after[i], "[fleet]") {
						t.Errorf("%s %dx%d %v: the refused ⇧tab redrew a row: %q became %q", sc.name, w, h, prof, before[i], after[i])
					}
				}

				// The board emptied under the keys: the last row of the
				// archive's board unhidden while standing on it.
				b := r94ttStand(sc, w, h, "2", "x", "x", "A", "shift+tab")
				if b.level != levelBoard {
					t.Fatalf("%s %dx%d %v: the stand is wrong — ⇧tab did not reach the archive's board", sc.name, w, h, prof)
				}
				pressKey(b, "x")
				poll(b, sc)
				pressKey(b, "x")
				poll(b, sc)
				rows := r94ttRows(b)
				if !strings.Contains(strings.Join(rows, "\n"), "no archived sessions") {
					t.Fatalf("%s %dx%d %v: the stand is wrong — the archive's board did not empty", sc.name, w, h, prof)
				}
				if b.level == levelBoard {
					t.Errorf("%s %dx%d %v: the archive's board emptied and the deck stayed at the board's level", sc.name, w, h, prof)
				}
				if title := r94ttFleetTitle(rows); !strings.Contains(title, "[fleet]") {
					t.Errorf("%s %dx%d %v: the list the emptied board fell back to lost `[fleet]`: %q", sc.name, w, h, prof, strings.TrimSpace(title))
				}
			}
			// Held: an archive that still draws rows keeps its board.
			l := r94ttStand(sd, w, h, "A", "shift+tab")
			if l.level != levelBoard {
				t.Errorf("second-day %dx%d %v: the archive with rows in it lost its board", w, h, prof)
			}
			if got := ansi.Strip(l.View()); !strings.Contains(strings.Split(got, "\n")[0], "· board") {
				t.Errorf("second-day %dx%d %v: the archive's board no longer says it is one: %q", w, h, prof, strings.TrimSpace(strings.Split(got, "\n")[0]))
			}
		}
		lipgloss.SetColorProfile(old)
	}
}

// ---- round 95, second-day ----
// ---- round 95, second-day, the one thing ----
// The row that shipped says the ask once, at every width. #192 and #267
// drew the bracket's word only where the whole label would not fit, so
// the copy the fold was about stood untouched twenty columns wider: at 120
// `A` on the second-day scene draws
// `◆ ship   fix the 401 on token refresh (commit)` three rows under
// `◉ "fix the 401 on token refresh"`, beside `▸1 ○ fix the 401 on token
// refresh` and under an identity header saying it again — four whole
// copies of one sentence on one 34-row frame, where the same row eighty
// columns narrower says `◆ ship   commit`. Where the ship label is the
// ask the frame already draws (askBracket's own two arms), the row draws
// the bracket's word at every width: a card says its sentence once
// (#64, #107, #110, #265), and the bracket is the half the ask does not
// say (#189, #192, #267).
func TestTheShipRowSaysTheAskOnceAtEveryWidth(t *testing.T) {
	forceASCII(t)

	// The frame it was found on: one keypress from the opening, canonical
	// (scenes/second-day-120x34.txt:1422).
	m := sceneModel(sceneSecondDay(), 120, 34)
	pressKey(m, "A")
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	var ship string
	for _, l := range rows {
		for _, seg := range strings.Split(l, "│") {
			if strings.Contains(seg, "◆ ship") {
				ship = strings.TrimSpace(seg)
			}
		}
	}
	if ship == "" {
		t.Fatalf("120x34 A: no ship row on the frame:\n%s", strings.Join(rows, "\n"))
	}
	if !strings.HasPrefix(ship, "◆ ship   commit") {
		t.Errorf("120x34 A: the ship row draws the ask the frame already says: %q", ship)
	}

	// The rule, over every scene, five widths, both colour profiles and
	// four routes: a ship row's whole label is never a sentence the same
	// frame draws somewhere else.
	whole := regexp.MustCompile(`ship\s+(\S[^│]*?) \(([a-z]+)\)`)
	routes := [][]string{nil, {"A"}, {"A", "2"}, {"A", "2", "tab"}, {"A", "shift+tab"}}
	for _, sc := range allScenes() {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				old := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(prof)
				for _, route := range routes {
					m := sceneModel(sc, size[0], size[1])
					for _, k := range route {
						pressKey(m, k)
					}
					rows := strings.Split(ansi.Strip(m.View()), "\n")
					for _, l := range rows {
						for _, seg := range strings.Split(l, "│") {
							mt := whole.FindStringSubmatch(seg)
							if mt == nil {
								continue
							}
							clause := strings.TrimSpace(mt[1])
							if len([]rune(clause)) < 10 {
								continue
							}
							for _, other := range rows {
								if other == l || !strings.Contains(other, clause) {
									continue
								}
								t.Errorf("%s %dx%d %v %v: the ship row says a sentence the frame already draws: %q beside %q",
									sc.name, size[0], size[1], prof, route,
									strings.TrimSpace(seg), strings.TrimSpace(other))
								break
							}
						}
					}
				}
				lipgloss.SetColorProfile(old)
			}
		}
	}
}

// ---- round 95, two-tools ----
// slivers reports, for one board frame with the reply box on it, the cells
// standing between a board rail and the box's left edge where that gap is
// too narrow for a row to say anything — the width `panelHides` already
// calls the box beginning inside a row's own prefix. The leftmost rail
// whose tail is that narrow is the column's own; a rail inside the
// fragment is a card's continuation and belongs to the fragment.
func slivers(frame string) (frags []string, edges int) {
	rows := strings.Split(ansi.Strip(frame), "\n")
	if len(rows) == 0 || !strings.Contains(rows[0], " · board") {
		return nil, 0
	}
	// Rune counts, not byte offsets: every glyph on these rows is one cell.
	at := func(row, glyph string) int {
		i := strings.Index(row, glyph)
		if i < 0 {
			return -1
		}
		return len([]rune(row[:i]))
	}
	left, top, bot := -1, -1, -1
	for i, r := range rows {
		if strings.Contains(r, "┌ reply to ") {
			left, top = at(r, "┌"), i
		}
		if left >= 0 && at(r, "└") == left {
			bot = i
		}
	}
	if left < 1 || top < 0 {
		return nil, 0
	}
	if bot < 0 {
		bot = len(rows) - 1
	}
	for i := top; i <= bot; i++ {
		head := []rune(rows[i])
		if len(head) < left {
			continue
		}
		// The box is drawn one cell right of the row's own last cell.
		line := []rune(string(head[:left-1]))
		for j := 0; j < len(line); {
			k := -1
			for x := j; x < len(line); x++ {
				if line[x] == '│' {
					k = x
					break
				}
			}
			if k < 0 {
				break
			}
			j = k + 1
			gap := len(line[j:])
			if gap <= 1 {
				// The box begins at the column's own edge: the gutter
				// alone stands between, and it carries the mark (#62).
				if strings.TrimSpace(string(line[j:])) == "…" {
					edges++
				}
				break
			}
			if gap <= trailPrefixWidth {
				frags = append(frags, string(line[j:]))
				break
			}
		}
	}
	return frags, edges
}

// TestTheReplyBoxLeavesNoSliverOfAColumn: where the reply box begins inside
// a board column's own prefix, the cells it leaves cannot be a row — they
// are a fragment of one cut at the border. On alarm-storm at 152 the cut
// fell inside an opening quote (`◉ "…`), on subagents at 120 it left `◆ t…`
// for a leg with no label, and on 196 rows it left blanks under a mark,
// which is what #126 composes a column at the box's width to stop. The
// fragment goes blank and the mark goes with it. Held on the other side
// too: where the box begins at a column's own edge the mark still stands
// (#62, #64).
func TestTheReplyBoxLeavesNoSliverOfAColumn(t *testing.T) {
	forceASCII(t)
	keys := []string{"r", "esc", "l", "r", "esc", "l", "r", "esc", "l", "r", "esc", "l", "r", "esc", "l", "r"}
	sizes := map[string][][2]int{
		"alarm-storm": {{120, 34}, {152, 40}},
		"few-ongoing": {{120, 34}, {152, 40}},
		"many-idle":   {{120, 34}, {152, 40}},
		"subagents":   {{120, 34}},
		"very-long":   {{120, 34}},
	}
	stands, edges := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range sizes[sc.name] {
				m := sceneModel(sc, size[0], size[1])
				for ki, k := range keys {
					pressKey(m, k)
					poll(m, sc)
					frags, e := slivers(m.View())
					edges += e
					for _, f := range frags {
						stands++
						if strings.TrimSpace(f) != "" {
							t.Errorf("%s %dx%d prof=%v after %d keys: the reply box left a sliver of a column: %q",
								sc.name, size[0], size[1], prof, ki+1, f)
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if stands == 0 {
		t.Fatalf("vacuous: no frame put the reply box inside a column's prefix")
	}
	if edges == 0 {
		t.Fatalf("vacuous: no frame put the reply box at a column's own edge")
	}
	t.Logf("sliver stands %d, box-at-edge rows %d", stands, edges)
}

// TestTheCutAtTheReplyBoxsEdgeIsAClip: the border the reply box cuts a row
// at is a clip like any other. `truncateWhole` knew only a number and a
// separator, so at 152 on subagents the board drew
// `● scout  Red-teaming plugin/…` — a mark after a slash, reading as more
// of the path it cut — where `clip` has dropped the trailing dot, slash,
// bracket, comma and unclosed quote since #58, #61, #63, #64 and #80.
func TestTheCutAtTheReplyBoxsEdgeIsAClip(t *testing.T) {
	forceASCII(t)
	keys := []string{"r", "esc", "l", "r", "esc", "l", "r", "esc", "l", "r", "esc", "l", "r", "esc", "l", "r"}
	sizes := map[string][][2]int{
		"alarm-storm": {{120, 34}, {152, 40}},
		"few-ongoing": {{120, 34}, {152, 40}},
		"many-idle":   {{120, 34}, {152, 40}},
		"subagents":   {{120, 34}, {152, 40}},
		"very-long":   {{120, 34}, {152, 40}},
	}
	marks := 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range sizes[sc.name] {
				m := sceneModel(sc, size[0], size[1])
				for ki, k := range keys {
					pressKey(m, k)
					poll(m, sc)
					rows := strings.Split(ansi.Strip(m.View()), "\n")
					if len(rows) == 0 || !strings.Contains(rows[0], " · board") {
						continue
					}
					left := -1
					for _, r := range rows {
						if i := strings.Index(r, "┌ reply to "); i >= 0 {
							left = len([]rune(r[:i]))
						}
					}
					if left < 3 {
						continue
					}
					for _, r := range rows {
						run := []rune(r)
						// The mark stands one cell left of the box (#62).
						if len(run) < left-1 || run[left-2] != '…' {
							continue
						}
						marks++
						head := strings.TrimRight(string(run[:left-2]), " ")
						if head == "" {
							continue
						}
						last := []rune(head)[len([]rune(head))-1]
						if strings.ContainsRune("(./—–,;", last) {
							t.Errorf("%s %dx%d prof=%v after %d keys: the box's border cut after %q: %q",
								sc.name, size[0], size[1], prof, ki+1, string(last), head)
						}
						if last == '"' && strings.Count(head, `"`)%2 == 1 {
							t.Errorf("%s %dx%d prof=%v after %d keys: the box's border cut inside an opening quote: %q",
								sc.name, size[0], size[1], prof, ki+1, head)
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if marks == 0 {
		t.Fatalf("vacuous: no frame drew the box's border mark")
	}
	t.Logf("border marks examined: %d", marks)
}

// ---- round 96, two-tools ----
// TestTheArchiveRefusesADigitNoRowWears pins round ninety-six's one thing.
//
// In the archive the digits are the archive's own (#32): the frame draws
// `▸1 ● api` and its header says `1 api`. #245 answered `the session you
// are on` where the pressed digit was the selected session's *live* digit
// and the archive drew its row — so on a one-row archive both `1` and `2`
// said it, and on a two-card archive `2` and `3` did, one frame answering
// one sentence for two numbers while only one row is drawn. #245's reason
// was that refusing "denies a session this very frame has selected"; the
// deck already refuses that very digit on that very frame from any other
// caret (`no session 3`), so the refusal is the answer for a number no row
// wears, not a denial of the session.
//
// The rule: on an archive that draws rows, a digit past the last drawn row
// takes the deck's own refusal, from every caret. Held on the other side
// too — the digit the caret's own row wears still answers `the session you
// are on`, so a blanket refusal does not satisfy this.
func TestTheArchiveRefusesADigitNoRowWears(t *testing.T) {
	forceASCII(t)
	type stand struct {
		scene string
		tail  []string
	}
	stands := []stand{
		{"two-tools", []string{"x", "A"}},
		{"two-tools", []string{"2", "x", "x", "A"}},
		{"two-tools", []string{"2", "x", "x", "A", "shift+tab"}},
		{"subagents", []string{"x", "A"}},
		{"fleet-hygiene", []string{"x", "A"}},
		{"many-idle", []string{"x", "x", "A"}},
	}
	refusals, wearers, triggers := 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, st := range stands {
			var sc scene
			found := false
			for _, s := range allScenes() {
				if s.name == st.scene {
					sc, found = s, true
				}
			}
			if !found {
				lipgloss.SetColorProfile(old)
				t.Fatalf("no scene %q", st.scene)
			}
			for _, size := range [][2]int{{80, 24}, {120, 34}, {220, 48}} {
				m := sceneModel(sc, size[0], size[1])
				for _, k := range st.tail {
					pressKey(m, k)
					poll(m, sc)
				}
				if !m.archiveView {
					continue
				}
				n := len(m.viewOrder())
				if n == 0 || n > 9 {
					continue // the empty archive is #242's, not this one
				}
				for caret := 1; caret <= n; caret++ {
					pressKey(m, fmt.Sprintf("%d", caret))
					poll(m, sc)
					// The digit the caret's own row wears still answers.
					if m.note != "the one you are on" && m.note != "" {
						t.Errorf("%s %dx%d prof=%v: the archive's own digit %d answered %q",
							st.scene, size[0], size[1], prof, caret, m.note)
					}
					pressKey(m, fmt.Sprintf("%d", caret))
					poll(m, sc)
					if m.note != "the one you are on" {
						t.Errorf("%s %dx%d prof=%v: the digit the caret's row (%d) wears answered %q",
							st.scene, size[0], size[1], prof, caret, m.note)
					} else {
						wearers++
					}
					live := m.digits[m.selectedKey]
					for d := n + 1; d <= 9; d++ {
						pressKey(m, fmt.Sprintf("%d", d))
						poll(m, sc)
						if d == live {
							triggers++
						}
						refusals++
						if want := fmt.Sprintf("no session %d", d); m.note != want {
							t.Errorf("%s %dx%d prof=%v caret %d: `%d` on an archive drawing %d rows answered %q, not %q",
								st.scene, size[0], size[1], prof, caret, d, n, m.note, want)
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if refusals == 0 {
		t.Fatalf("vacuous: no archive stand had a digit past its last drawn row")
	}
	if wearers == 0 {
		t.Fatalf("vacuous: no archive row's own digit was pressed on it")
	}
	if triggers == 0 {
		t.Fatalf("vacuous: no pressed digit was the selected session's live digit")
	}
	t.Logf("refusals held %d, drawn digits held %d, live-digit collisions %d", refusals, wearers, triggers)
}

// ---- round 96, second-day ----
// TestTheHeaderSaysTheAskOnceAndKeepsTheToolWord: the identity header of a
// session its person named draws the name, not the name and a second copy
// of what it asked.
//
// The frame it was found on: `second-day` at eighty, `A` then `3` — the
// header drew ` ⌂ compass · 3 checkout-flake-hunt · "the checkout suite
// flake…`, a clipped copy of a sentence the trail's `◉` row three rows
// below drew whole, and paid for the fragment with `· claude`, the word
// every other archive header at that width keeps on a fleet holding two
// tools (#79, #80). Then the rule over every scene, every width and both
// profiles: where the header names a session by `name · "ask"`, the ask is
// drawn on a `◉` row of the same frame, so the header's copy is the second
// one — #265's cut at the archive board's card head and #269's at the ship
// row, on the row that is drawn at every level.
func TestTheHeaderSaysTheAskOnceAndKeepsTheToolWord(t *testing.T) {
	prev := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(prev)

	// 1 — the frame it was found on.
	lipgloss.SetColorProfile(termenv.Ascii)
	sc := sceneSecondDay()
	m := sceneModel(sc, 80, 24)
	for _, k := range []string{"A", "3"} {
		pressKey(m, k)
		poll(m, sc)
	}
	head := ansi.Strip(strings.Split(m.View(), "\n")[0])
	if strings.Contains(head, `"the checkout suite`) {
		t.Errorf("80x24 A,3: the header draws the ask the trail draws whole: %q", head)
	}
	if !strings.Contains(head, "3 checkout-flake-hunt") || !strings.Contains(head, "claude") {
		t.Errorf("80x24 A,3: the header should name the session and its tool: %q", head)
	}

	// 2 — the rule: no header names a session `name · "ask"` while a row of
	// the same frame draws that ask.
	routes := [][]string{
		{"A"}, {"A", "3"}, {"A", "j", "j"}, {"A", "j", "j", "tab"},
		{"A", "shift+tab"}, {"A", "shift+tab", "3"}, {"A", "3", "tab", "tab"},
	}
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"ascii", termenv.Ascii}, {"truecolor", termenv.TrueColor}} {
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				for _, route := range routes {
					m := sceneModel(sc, size[0], size[1])
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					rows := strings.Split(m.View(), "\n")
					h := ansi.Strip(rows[0])
					i := strings.Index(h, ` · "`)
					if !strings.HasPrefix(h, " ⌂ compass · ") || i < 0 {
						continue
					}
					rest := h[i+len(` · "`):]
					j := strings.Index(rest, `"`)
					if j < 0 {
						continue
					}
					ask := strings.TrimSuffix(rest[:j], "…")
					if len(ask) < 8 {
						continue
					}
					t.Errorf("%s %s %dx%d %v: the header quotes the ask beside the name: %q",
						prof.name, sc.name, size[0], size[1], route, h)
				}
			}
		}
	}
}

// ---- round 97, fleet-hygiene ----
// TestTheRelayedCardsHeadYieldsItsAskLikeAnyOther pins round ninety-seven's
// one thing.
//
// #265 sent the archive board card's ask down to the card's own `◉` row and
// left the head the name the archive gives the session; #266 kept the ask on
// the head only where the name and age it would fall back to are another
// drawn card's too. The one card whose prompt came from another session
// (#97) fell through both: its `◉` row wears the relay verb outside the
// quotes — `◉ relayed "the encoder is in…` — so `saidAsk` handed `sameAsk` a
// string beginning `relayed "`, the prefix compare could never match, and the
// head kept `porter · relayed "the enco…` over its own row three lines down.
// Its head key (`porter`, `1m`) is no other row's, so #266 is not what held
// it. At every board width both copies were clipped, so the card spent two
// rows on a sentence it said whole on neither.
//
// The rule: on the archive board no card's head draws the sentence its own
// `◉` row draws, whatever verb that row wears. Held on the other side too —
// the ask stays on the `◉` row with its verb, and a card whose archive title
// is not its first prompt still keeps that title on the head (#265's own
// `sameAsk` gate), so blanking every live head does not satisfy this.
func TestTheRelayedCardsHeadYieldsItsAskLikeAnyOther(t *testing.T) {
	var sc scene
	for _, s := range allScenes() {
		if s.name == "fleet-hygiene" {
			sc = s
		}
	}
	if sc.name == "" {
		t.Fatal("no fleet-hygiene scene")
	}
	headRow := regexp.MustCompile(`^▸?[1-9] [●○◍▲⊘] \S`)
	heads := func(frame string) []string {
		var out []string
		for _, line := range strings.Split(ansi.Strip(frame), "\n") {
			for _, seg := range strings.Split(line, "│") {
				seg = strings.TrimSpace(seg)
				if strings.HasPrefix(seg, "◉") || seg == "" {
					continue
				}
				if headRow.MatchString(seg) {
					out = append(out, seg)
				}
			}
		}
		return out
	}
	yielded, kept, asks := 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
			for _, tail := range [][]string{{"x", "A", "shift+tab"}, {"x", "x", "A", "shift+tab"}} {
				m := sceneModel(sc, size[0], size[1])
				for _, k := range tail {
					pressKey(m, k)
					poll(m, sc)
				}
				if !m.archiveView || m.level != levelBoard {
					lipgloss.SetColorProfile(old)
					t.Fatalf("%dx%d %v: not the archive board", size[0], size[1], tail)
				}
				frame := ansi.Strip(m.View())
				hs := heads(frame)
				relay, closed := "", ""
				for _, h := range hs {
					if strings.Contains(h, "porter") {
						relay = h
					}
					if strings.Contains(h, "the pane I closed") {
						closed = h
					}
				}
				if relay == "" {
					lipgloss.SetColorProfile(old)
					t.Fatalf("%dx%d %v: the relayed session draws no card head", size[0], size[1], tail)
				}
				if strings.Contains(relay, "encoder") || strings.Contains(relay, "relayed") {
					t.Errorf("%dx%d %v prof=%v: the relayed card's head says its own ◉ row over again: %q",
						size[0], size[1], tail, prof, relay)
				} else {
					yielded++
				}
				// The ask is not lost: it stays on the card's ◉ row, verb and all.
				if !strings.Contains(frame, `◉ relayed "the encoder`) {
					t.Errorf("%dx%d %v prof=%v: the relayed ask left the frame with the head", size[0], size[1], tail, prof)
				} else {
					asks++
				}
				// A card whose archive title is not its first prompt keeps it.
				if closed == "" {
					t.Errorf("%dx%d %v prof=%v: the closed-pane card stopped saying its own title", size[0], size[1], tail, prof)
				} else {
					kept++
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if yielded == 0 || kept == 0 || asks == 0 {
		t.Fatalf("vacuous: yielded %d, titles kept %d, asks drawn %d", yielded, kept, asks)
	}
	t.Logf("relayed heads yielded: %d · titles kept: %d · asks still drawn: %d", yielded, kept, asks)
}

// ---- round 97, second-day ----
// ---- round 97, second-day ----
// TestTheArchiveBoardNamesTheLevelItsTabReaches: a board footer's word for
// `tab` is the level the key reaches.
//
// The frame it was found on: `second-day` at 120, `A` then `⇧tab` — the
// archive's board drew `h/l columns · enter · no pane · tab session · …`,
// and `tab` there landed on `levelTrail`, the archive's list, whose own
// panel is chipped `[fleet]` and whose own footer then names `tab deeper`
// for the step that is left. `zoomIn` takes the board straight to the
// session view only off the live board (#18); wherever the archive is open
// it stops at the list, so the live board's word — true there, where the
// key lands on the panel chipped `[session]` — named a level this key does
// not reach (#40, #246). Then the rule over every scene, both profiles and
// every width a board fits at: no board footer says `tab session` unless
// `tab` pressed on that very frame lands at `levelWaypoints`, and none says
// `tab deeper` unless it lands at `levelTrail`.
func TestTheArchiveBoardNamesTheLevelItsTabReaches(t *testing.T) {
	prev := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(prev)

	// 1 — the frame it was found on.
	lipgloss.SetColorProfile(termenv.Ascii)
	sc := sceneSecondDay()
	m := sceneModel(sc, 120, 34)
	for _, k := range []string{"A", "shift+tab"} {
		pressKey(m, k)
		poll(m, sc)
	}
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	foot := rows[len(rows)-1]
	if m.level != levelBoard {
		t.Fatalf("120x34 A,⇧tab: the archive's board is the frame under test, got Lv%d", m.level)
	}
	if strings.Contains(foot, "tab session") {
		t.Errorf("120x34 A,⇧tab: the archive's board names a level its tab does not reach: %q", foot)
	}
	if !strings.Contains(foot, "tab deeper") {
		t.Errorf("120x34 A,⇧tab: the archive's board should name the step it takes: %q", foot)
	}
	pressKey(m, "tab")
	poll(m, sc)
	if m.level != levelTrail {
		t.Errorf("120x34 A,⇧tab,tab: the archive's board goes to the list, got Lv%d", m.level)
	}

	// 2 — the rule: a board footer's tab word is the level its tab reaches.
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"ascii", termenv.Ascii}, {"truecolor", termenv.TrueColor}} {
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
				for _, route := range [][]string{{"shift+tab"}, {"A", "shift+tab"}, {"A", "shift+tab", "j"}} {
					m := sceneModel(sc, size[0], size[1])
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					if m.level != levelBoard {
						continue
					}
					rows := strings.Split(ansi.Strip(m.View()), "\n")
					foot := rows[len(rows)-1]
					says := ""
					switch {
					case strings.Contains(foot, "tab session"):
						says = "tab session"
					case strings.Contains(foot, "tab deeper"):
						says = "tab deeper"
					default:
						continue
					}
					n := sceneModel(sc, size[0], size[1])
					for _, k := range route {
						pressKey(n, k)
						poll(n, sc)
					}
					pressKey(n, "tab")
					poll(n, sc)
					want := "tab deeper"
					if n.level == levelWaypoints {
						want = "tab session"
					}
					if says != want {
						t.Errorf("%s %s %dx%d %v: the footer says %q and tab lands at Lv%d: %q",
							prof.name, sc.name, size[0], size[1], route, says, n.level, foot)
					}
				}
			}
		}
	}
}

// ---- round 97, two-tools ----
// ---- round 97, two-tools ----
// TestTheArchiveRowDoesNotSayTheAskTheTrailDraws: in the archive the row a
// name of its own keeps does not draw the ask the trail beside it draws on
// its own ◉ row.
//
// The frame it was found on: `two-tools` at 220, `2`, `x`, `A` — the archive
// list drew
//
//	▸1 ● api · "add rate limiting to the token endpoin…  40s │ ╷
//
// four cells left of
//
//	◉ "add rate limiting to the token endpoint"    30m ago
//
// one letter short of the sentence its neighbour drew whole with a hundred
// blank cells after it. #111 is the rule for the present line — a row does
// not say what the trail beside it says — and it is switched off in the
// archive; #107's compare and #265's arrangement (the head yields, the ◉ row
// keeps) are the deck's own. The row keeps the name (#79): what the person
// went looking for, where `hidden · flake in the checkout suite` had lost it.
//
// It holds three ways so a blanket cut cannot pass: the ask stands once on
// every such frame, the row still wears its name, and an archived row with no
// trail beside it keeps its ask, which is what tells it from its neighbour
// (#86, #266).
func TestTheArchiveRowDoesNotSayTheAskTheTrailDraws(t *testing.T) {
	yields, names, keeps := 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {120, 34}, {220, 48}} {
				for _, run := range [][]string{{"2", "x", "A"}, {"2", "x", "x", "A"}} {
					m := sceneModel(sc, size[0], size[1])
					for _, k := range run {
						pressKey(m, k)
						poll(m, sc)
					}
					if !m.archiveView {
						continue
					}
					s, ok := m.selected()
					if !ok || !(s.Live || s.Info.Name != "") {
						continue
					}
					ask := archiveHeadline(s)
					if ask == "" {
						continue
					}
					rows := strings.Split(ansi.Strip(m.View()), "\n")
					var caret, said string
					for _, r := range rows {
						if said == "" {
							for _, seg := range strings.Split(r, "│") {
								if a := saidAsk(seg); a != "" && sameAsk(ask, a) {
									said = a
									break
								}
							}
						}
						if caret == "" && strings.Contains(r, "▸") && strings.Contains(r, sessionName(s.Info)) {
							caret = r
						}
					}
					where := sc.name + " " + itoa(size[0]) + "x" + itoa(size[1])
					if said == "" || caret == "" {
						continue // no trail row beside it: nothing to yield to
					}
					yields++
					// The caret's row is the fleet column's, left of the rule.
					left := caret
					if i := strings.Index(left, "│"); i >= 0 {
						left = left[:i]
					}
					if strings.Contains(left, `"`) {
						t.Errorf("%s: the archive row said the ask the trail draws: %q beside %q", where, strings.TrimRight(left, " "), said)
					}
					if !strings.Contains(left, sessionName(s.Info)) {
						names++
						t.Errorf("%s: the archive row lost its name: %q", where, strings.TrimRight(left, " "))
					} else {
						names++
					}
					// A row the caret is not on has no trail beside it and
					// keeps its ask, which is what tells it from its
					// namesake (#80, #266): two `api` rows in one archive.
					for _, r := range rows {
						body := r
						if i := strings.Index(body, "│"); i >= 0 {
							body = body[:i]
						}
						if strings.Contains(body, "▸") || !strings.Contains(body, `"`) {
							continue
						}
						keeps++
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if yields < 6 {
		t.Fatalf("the pin never reached a frame that yields: %d", yields)
	}
	if names < 6 {
		t.Fatalf("the pin never held a name: %d", names)
	}
	if keeps < 6 {
		t.Fatalf("the pin never held an unselected row's ask: %d", keeps)
	}
	t.Logf("rows that yield %d, names held %d, unselected rows keeping their ask %d", yields, names, keeps)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// ---- round 98, fleet-hygiene ----
// TestTheArchiveBoardNamesTheAskKeyItActsOn pins round ninety-eight's one
// thing.
//
// The archive's board footer was copied from the live board's before `a ask`
// existed (round 24 added the key to the live branch and not to the archive
// branch beside it), and never picked it up: `h/l columns · enter · no pane ·
// tab deeper · / search · A fleet · ? help · q quit`, 82 cells inside 120,
// with the key nowhere on it — while `a` acts there on the very row the
// caret is on, the archive's list one `tab deeper` away names it, the live
// board names it, and the shed's own comment calls the archive `a ask`'s
// reason to be there (#264, #24, #175, #187).
//
// The rule, over every scene with an archive at every width its board fits
// and under both colour profiles: where `a` pressed on the archive's board
// returns a command, the footer of that very frame names `a ask`. Held on
// the other side by counting the keys that stood before and requiring that
// none is lost — so satisfying this by emptying the footer fails — and by
// the live board keeping its own `a ask`.
func TestTheArchiveBoardNamesTheAskKeyItActsOn(t *testing.T) {
	named, live := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
				for _, route := range [][]string{{"A", "shift+tab"}, {"x", "A", "shift+tab"}, {"A", "j", "shift+tab"}} {
					m := sceneModel(sc, size[0], size[1])
					poll(m, sc)
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					if !m.archiveView || m.level != levelBoard || !m.boardShown() {
						continue
					}
					foot := r98fhFooterRow(m)
					before := r98fhKeyWords(foot)
					if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}); cmd == nil {
						continue
					}
					if !strings.Contains(foot, "a ask") {
						lipgloss.SetColorProfile(old)
						t.Errorf("%v %s %dx%d %v: `a` acts on the archive board and the footer does not name it: %q",
							prof, sc.name, size[0], size[1], route, strings.TrimSpace(foot))
						lipgloss.SetColorProfile(prof)
						continue
					}
					named++
					// Nothing the footer already said is paid for it: the
					// attach aside is not a key (#55, #211), so the keys
					// are counted by their first word.
					for _, want := range []string{"h/l", "enter", "tab", "/", "A", "?", "q"} {
						if !before[want] {
							continue
						}
						if !r98fhKeyWords(foot)[want] {
							lipgloss.SetColorProfile(old)
							t.Errorf("%v %s %dx%d %v: the archive board lost %q for the ask key: %q",
								prof, sc.name, size[0], size[1], route, want, strings.TrimSpace(foot))
							lipgloss.SetColorProfile(prof)
						}
					}
				}
				// The live board this branch was copied from keeps its own.
				m := sceneModel(sc, size[0], size[1])
				poll(m, sc)
				if m.level == levelBoard && m.boardShown() && !m.archiveView {
					if !strings.Contains(r98fhFooterRow(m), "a ask") {
						lipgloss.SetColorProfile(old)
						t.Errorf("%v %s %dx%d: the live board stopped naming the ask key: %q",
							prof, sc.name, size[0], size[1], strings.TrimSpace(r98fhFooterRow(m)))
						lipgloss.SetColorProfile(prof)
					} else {
						live++
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if named == 0 || live == 0 {
		t.Fatalf("vacuous: archive boards naming the ask %d, live boards keeping it %d", named, live)
	}
	t.Logf("archive boards naming `a ask`: %d · live boards keeping it: %d", named, live)
}

// r98fhFooterRow is the keymap row of the frame as drawn: the last row that
// carries the way out, which every footer ends with (#35).
func r98fhFooterRow(m *Model) string {
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.Contains(lines[i], "? help") {
			return lines[i]
		}
	}
	return ""
}

// r98fhKeyWords is the set of keys a footer names, each by the word it is
// pressed with, so a clause losing only its parenthetical loses no key.
func r98fhKeyWords(foot string) map[string]bool {
	out := map[string]bool{}
	for _, clause := range strings.Split(foot, "·") {
		if f := strings.Fields(clause); len(f) > 0 {
			out[f[0]] = true
		}
	}
	return out
}

// ---- round 98, second-day ----
// TestAChapterNoteOfOneChapterIsTheCountAlone: on a trail of one chapter the
// `[ ]` note is `❯ 1/1` — the count names the turn the key landed on by
// itself, because there is no other turn it could have been — and the quote
// beside it is the sentence the turn's own row draws whole on the same
// frame, under the same glyph.
//
// The frame it was found on: `second-day`, `A`,`1`,`tab`,`tab`,`[` at 220 —
// the archived session the person walked away from two hours ago, opened in
// the reader. The footer drew
//
//	space unfold · / search · n/N · [ ] turns · a ask · enter · no pane · esc back · A fleet · ? help · q quit   ❯ 1/1 · "fix the 401 on token refresh"
//
// on a frame that already drew that sentence whole four times: on the
// identity header, on the archive's selected row, on the trail's `◉` row,
// and on the reader's own `❯` row — the very row the note names, wearing
// the very glyph the note wears. #20 gave the note the quote to say which
// turn `[ ]` moved to; #128 took the clock off it because the row it landed
// on carries the clock at every width, and that reason reaches the quote
// where the count already names the turn; #134 took the cut quote off for
// saying less than the count alone.
//
// Then the rule, over every scene, five widths and both profiles: no footer
// draws a `1/1` chapter note with a quote, and on every frame that draws a
// `1/1` note the sentence it would have quoted stands on a row of the frame,
// so the count is never the only copy. And the other side: a note counting
// more than one chapter keeps its quote.
func TestAChapterNoteOfOneChapterIsTheCountAlone(t *testing.T) {
	chapterOneRoutes := [][]string{
		{"A", "1", "tab", "tab", "["},
		{"tab", "tab", "["},
		{"tab", "tab", "]"},
		{"tab", "tab", "G", "["},
		{"1", "tab", "tab", "["},
		{"tab", "j", "tab", "["},
	}
	chapterOneFooter := func(view string) string {
		rows := strings.Split(ansi.Strip(view), "\n")
		for i := len(rows) - 1; i >= 0; i-- {
			if strings.TrimSpace(rows[i]) != "" {
				return rows[i]
			}
		}
		return ""
	}
	prev := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(prev)

	// The frame it was found on.
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"forceASCII", termenv.Ascii}, {"colour on", termenv.TrueColor}} {
		lipgloss.SetColorProfile(prof.p)
		for _, w := range []int{152, 220} {
			sd := sceneSecondDay()
			m := sceneModel(sd, w, 40)
			for _, k := range []string{"A", "1", "tab", "tab", "["} {
				pressKey(m, k)
				poll(m, sd)
			}
			foot := chapterOneFooter(m.View())
			if !strings.Contains(foot, "❯ 1/1") {
				t.Fatalf("%s %d: the chapter note is gone: %q", prof.name, w, foot)
			}
			if strings.Contains(foot, `❯ 1/1 · "`) {
				t.Errorf("%s %d: the one chapter's note quotes the sentence its own ❯ row draws: %q", prof.name, w, foot)
			}
			// `[` lands the reader's cursor on this very turn, marking it
			// "❯▸fix the 401…" rather than "❯ fix the 401…" (markAnchor's
			// convention); either form is the turn the note names.
			view := ansi.Strip(m.View())
			if !strings.Contains(view, "❯ fix the 401 on token refresh") && !strings.Contains(view, "❯▸fix the 401 on token refresh") {
				t.Errorf("%s %d: the turn the note names is not drawn", prof.name, w)
			}
		}
	}

	// The rule.
	quoted := 0
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"forceASCII", termenv.Ascii}, {"colour on", termenv.TrueColor}} {
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				for _, route := range chapterOneRoutes {
					m := sceneModel(sc, size[0], size[1])
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					view := m.View()
					foot := chapterOneFooter(view)
					for _, g := range []string{glyphSaid, glyphBranch} {
						if strings.Contains(foot, g+` 1/1 · "`) {
							t.Errorf("%s %s %dx%d %v: the one chapter's note quotes a sentence its own row draws: %q",
								prof.name, sc.name, size[0], size[1], route, foot)
						}
					}
					if !strings.Contains(foot, glyphSaid+" 1/1") && !strings.Contains(foot, glyphBranch+" 1/1") {
						if i := strings.Index(foot, ` · "`); i > 0 &&
							(strings.Contains(foot, glyphSaid+" ") || strings.Contains(foot, glyphBranch+" ")) {
							// A note counting more than one chapter: the
							// quote goes where the row draws its sentence
							// (#279's rule over #278's); the count stays.
							quoted++
						}
						continue
					}
					// The count is not the only copy: the sentence the
					// note dropped stands on a row of the frame.
					want := strings.TrimSuffix(strings.TrimSpace(m.anchorText), "…")
					if want == "" {
						continue
					}
					found := false
					for _, r := range strings.Split(ansi.Strip(view), "\n") {
						if r != foot && strings.Contains(r, want) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("%s %s %dx%d %v: the note is the count alone and %q is on no row of the frame",
							prof.name, sc.name, size[0], size[1], route, want)
					}
				}
			}
		}
	}
	_ = quoted // the wider rule (#279) decides the quote on every count; the one-chapter note's is the count alone either way
}

// ---- round 98, two-tools ----
// r98ttQuoted is the sentence a chapter note quotes, and the note's glyph.
// A chapter note is `◉ 2/2 · "…" · 15:47`, `❯ 9/9 · "…"` or `◈ 1/1 · "…"`:
// one clause of it is a quoted sentence, and the rest — the count and the
// clock — is what no row of the frame draws.
func r98ttQuoted(note string) (glyph, said string) {
	for _, g := range []string{glyphPrompt, glyphSaid, glyphBranch} {
		if !strings.HasPrefix(note, g+" ") {
			continue
		}
		for _, c := range strings.Split(note, " · ") {
			if len(c) > 1 && strings.HasPrefix(c, `"`) && strings.HasSuffix(c, `"`) {
				return g, strings.Trim(c, `"`)
			}
		}
	}
	return "", ""
}

// TestTheChapterNoteLeavesItsQuoteToTheRow: `[` and `]` land the frame on the
// very turn they count — the viewport opens on the prompt at Lv1, the cursor
// stands on it at Lv2, the reader is on its page at Lv3 — so the row drawing
// that sentence is under the note. Where it is, the footer does not say it
// again: at 220 `❯ 1/1 · "add rate limiting to the token endpoint"` stood
// under a row drawing that sentence twice, once each side of the rule. The
// note keeps its count and its clock, which no row draws (#128, #134, #276).
func TestTheChapterNoteLeavesItsQuoteToTheRow(t *testing.T) {
	forceASCII(t)
	runs := [][]string{
		{"tab", "]", "]", "["},
		{"tab", "tab", "tab", "[", "]"},
		{"G", "[", "["},
	}
	seen, yielded, rowsWithQuotes := 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {120, 34}, {220, 48}} {
				for _, run := range runs {
					m := sceneModel(sc, size[0], size[1])
					for _, k := range run {
						pressKey(m, k)
						poll(m, sc)
						rows := strings.Split(ansi.Strip(m.View()), "\n")
						body, foot := rows[1:len(rows)-1], rows[len(rows)-1]
						for _, r := range body {
							if strings.Count(r, `"`) >= 2 {
								rowsWithQuotes++
							}
						}
						glyph, said := r98ttQuoted(m.note)
						if glyph == "" {
							continue
						}
						seen++
						drawn := ""
						for _, r := range body {
							if saysSame(said, r) {
								drawn = strings.TrimSpace(r)
							}
						}
						if drawn == "" {
							continue // nothing else says it: the note keeps its quote
						}
						yielded++
						if i := strings.Index(foot, glyph+" "); i >= 0 {
							note := strings.TrimSpace(foot[i:])
							if strings.Contains(note, `"`) {
								t.Errorf("%s %dx%d %v: the chapter note said the sentence the row draws: %q under %q",
									sc.name, size[0], size[1], prof, note, drawn)
							}
							// What no row draws stays: the chapter's own
							// number, and the prompt note's wall clock.
							if !strings.HasPrefix(note, glyph+" ") || !strings.Contains(note, "/") {
								t.Errorf("%s %dx%d %v: the chapter note lost its count: %q", sc.name, size[0], size[1], prof, note)
							}
							if glyph == glyphPrompt && !strings.Contains(m.note, `"`) {
								t.Errorf("%s %dx%d %v: the prompt note lost its clock: %q", sc.name, size[0], size[1], prof, note)
							}
						} else {
							t.Errorf("%s %dx%d %v: the chapter note went off the footer: %q", sc.name, size[0], size[1], prof, foot)
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	// Not vacuous three ways: the walk reaches chapter notes with a quote,
	// the rule fires on frames whose row draws that sentence, and the rows
	// themselves still carry their quoted sentences — a deck that stripped
	// every quote everywhere would pass the assertion above and fail here.
	if seen < 40 {
		t.Fatalf("the walk saw %d chapter notes with a quote, expected at least 40", seen)
	}
	if yielded < 20 {
		t.Fatalf("the rule fired on %d frames, expected at least 20", yielded)
	}
	if rowsWithQuotes < 500 {
		t.Fatalf("the frames drew %d quoted rows, expected at least 500", rowsWithQuotes)
	}
}

// ---- round 99, second-day ----
// TestTheFleetFoldNamesTheKeyOnlyWhereItMoves holds the fleet column's fold to
// the key it names. "▾ 3 more below · j" is the list saying it is cut and how
// to see the rest, and `j` is the list's key only while the keys are in the
// fleet panel — the one wearing `▌`. At Lv2 the same key walks the trail's
// rows and at Lv3 it scrolls the reader's page, so on those frames the fold
// named a key that leaves every folded row folded. The count is the row's own
// answer at every level and stays; the way back to the list is `esc back`,
// which those footers already name (#232, #250).
func TestTheFleetFoldNamesTheKeyOnlyWhereItMoves(t *testing.T) {
	foldRow := func(view string) string {
		for _, ln := range strings.Split(ansi.Strip(view), "\n") {
			if strings.Contains(ln, "more below") || strings.Contains(ln, "more above") {
				return strings.TrimRight(ln, " ")
			}
		}
		return ""
	}
	walk := func(sc scene, w, h int, route []string) *Model {
		m := sceneModel(sc, w, h)
		for _, k := range route {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}
	profiles := []struct {
		name string
		p    termenv.Profile
	}{{"forceASCII", termenv.Ascii}, {"colour on", termenv.TrueColor}}
	prev := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(prev)

	// The frames it was found on: the second day's archived session opened
	// one and two levels in, where the fleet column keeps its fold.
	for _, prof := range profiles {
		lipgloss.SetColorProfile(prof.p)
		for _, stand := range []struct {
			w, h  int
			route []string
			lv    int
		}{
			{100, 30, []string{"A", "1", "tab"}, 2},
			{152, 40, []string{"A", "1", "tab", "tab"}, 3},
			{220, 48, []string{"A", "1", "tab", "tab"}, 3},
		} {
			sd := sceneSecondDay()
			m := walk(sd, stand.w, stand.h, stand.route)
			if m.level != stand.lv {
				t.Fatalf("%s %d: %v landed at Lv%d, not Lv%d", prof.name, stand.w, stand.route, m.level, stand.lv)
			}
			row := foldRow(m.View())
			if row == "" {
				t.Fatalf("%s %d: no fold on %v", prof.name, stand.w, stand.route)
			}
			if strings.Contains(row, "· j") || strings.Contains(row, "· k") {
				t.Errorf("%s %d %v: the fold names a key the fleet does not hold: %q",
					prof.name, stand.w, stand.route, row)
			}
			if !strings.Contains(row, "more below") && !strings.Contains(row, "more above") {
				t.Errorf("%s %d %v: the fold lost its count: %q", prof.name, stand.w, stand.route, row)
			}
			// Pressed on that very frame, `j` leaves the fold as it was.
			after := walk(sd, stand.w, stand.h, append(append([]string(nil), stand.route...), "j"))
			if got := foldRow(after.View()); got != row {
				t.Errorf("%s %d %v: `j` moved the fold %q -> %q", prof.name, stand.w, stand.route, row, got)
			}
		}
	}

	// The rule, and its held side: off the fleet the fold is the count
	// alone; on the fleet it keeps the key that moves the list.
	routes := [][]string{
		{}, {"tab"}, {"tab", "tab"}, {"A"}, {"A", "1", "tab"}, {"A", "1", "tab", "tab"},
		{"A", "j"}, {"tab", "j"}, {"x"}, {"A", "shift+tab", "tab"},
	}
	keyed := 0
	for _, prof := range profiles {
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				for _, route := range routes {
					m := walk(sc, size[0], size[1], route)
					row := foldRow(m.View())
					if row == "" {
						continue
					}
					named := strings.Contains(row, "· j") || strings.Contains(row, "· k")
					if m.focus() == panelFleet {
						if named {
							keyed++
						}
						continue
					}
					if named {
						t.Errorf("%s %s %dx%d %v (Lv%d): the fold names a key the fleet does not hold: %q",
							prof.name, sc.name, size[0], size[1], route, m.level, row)
					}
				}
			}
		}
	}
	if keyed == 0 {
		t.Errorf("no fold kept its key where the fleet holds it: the yield took every one")
	}
}

// ---- round 99, two-tools ----
// r99ttUnit is the one word of a width refusal the person's own terminal
// supplies (#190): `mirror needs 110 columns`, `no board under 110 columns`.
const r99ttUnit = " columns"

// r99ttFooterKeys is the footer's keys as a set, read past the attach
// aside, which is not a key (#55).
func r99ttFooterKeys(view string) map[string]bool {
	rows := strings.Split(ansi.Strip(view), "\n")
	foot := rows[len(rows)-1]
	keys := map[string]bool{}
	for _, frag := range strings.Split(strings.ReplaceAll(foot, attachHint, ""), " · ") {
		if f := strings.TrimSpace(frag); f != "" {
			keys[f] = true
		}
	}
	return keys
}

// TestTheWidthRefusalKeepsItsUnitOnlyWhereItCostsNoKey walks every scene at
// the two widths a width refusal is drawn at, under both colour profiles
// (#215, #218). Wherever the note ends in the unit, the same frame is drawn
// again with the unit dropped: the shorter note must put no key on the row
// that the longer one left off. A key that acts and is not named is what a
// footer is for (#24, #165, #175, #187, #190, #194, #198, #264), and the
// eight cells are a word the terminal supplies.
func TestTheWidthRefusalKeepsItsUnitOnlyWhereItCostsNoKey(t *testing.T) {
	forceASCII(t)
	routes := [][]string{
		{"m"},
		{"tab", "m"},
		{"shift+tab"},
		{"j", "tab", "m"},
		{"j", "m"},
		{"tab", "tab", "m"},
		{"shift+tab", "shift+tab"},
	}
	refusals, checked := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}} {
				w, h := size[0], size[1]
				for ri, route := range routes {
					m := sceneModel(sc, w, h)
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					view := m.View()
					rows := strings.Split(ansi.Strip(view), "\n")
					foot := strings.TrimSpace(rows[len(rows)-1])
					if !strings.Contains(foot, "110") {
						continue
					}
					refusals++
					// The refusal still names its number and its key.
					if !strings.Contains(foot, "mirror needs 110") && !strings.Contains(foot, "no board under 110") {
						t.Errorf("%s %dx%d r%d p%v: the width refusal lost its words: %q", sc.name, w, h, ri, prof, foot)
						continue
					}
					if !strings.HasSuffix(m.note, r99ttUnit) {
						continue
					}
					checked++
					was := r99ttFooterKeys(view)
					m.note = strings.TrimSuffix(m.note, r99ttUnit)
					now := r99ttFooterKeys(m.View())
					var gained []string
					for k := range now {
						if !was[k] && !strings.Contains(k, "110") {
							gained = append(gained, k)
						}
					}
					if len(gained) > 0 {
						t.Errorf("%s %dx%d r%d p%v: the unit cost the footer %s: %q",
							sc.name, w, h, ri, prof, fmt.Sprint(gained), foot)
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if refusals < 40 {
		t.Fatalf("the walk reached only %d width refusals", refusals)
	}
	if checked < 10 {
		t.Fatalf("only %d refusals still wore the unit — the rule was not exercised", checked)
	}
}

// ---- round 100, fleet-hygiene ----
// ---- round 100, fleet-hygiene ----

// r100fhFold is the fleet column's fold row on a frame — "▾ 9 more below · j",
// "▴ ⌁ work · 1 more above · k" — read past the panel rule so the trail's own
// text never lands in it.
func r100fhFold(view string) string {
	for _, ln := range strings.Split(ansi.Strip(view), "\n") {
		t := strings.TrimSpace(ln)
		if !strings.HasPrefix(t, "▾ ") && !strings.HasPrefix(t, "▴ ") {
			continue
		}
		if !strings.Contains(t, "more below") && !strings.Contains(t, "more above") {
			continue
		}
		if i := strings.IndexAny(t, "│┃"); i >= 0 {
			t = strings.TrimSpace(t[:i])
		}
		return t
	}
	return ""
}

// r100fhCount is a fold row without its key clause: the count alone, which is
// what a press of `j` on a frame that holds the key would change.
func r100fhCount(row string) string {
	row = strings.TrimSuffix(row, " · j")
	return strings.TrimSuffix(row, " · k")
}

// r100fhFoot is the footer of a frame — the row that names the keys the frame
// holds.
func r100fhFoot(view string) string {
	rows := strings.Split(ansi.Strip(view), "\n")
	return strings.TrimSpace(rows[len(rows)-1])
}

// r100fhWalk presses a route on a fresh scene model, polling after each key
// the way the deck's own refresh does.
func r100fhWalk(sc scene, w, h int, route []string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range route {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// TestTheFleetFoldDropsItsKeyWhileALineIsBeingTyped holds the fleet column's
// fold to the key it names on frames where a search, a quick reply or a typed
// line has the keyboard. #280 shed the clause at Lv2 and Lv3, where `j` and
// `k` belong to the trail's rows and the reader's page; the same harm is at
// Lv1 whenever `m.searching` or `m.replying` is on, because every key then
// belongs to that line (`app.go`, "While a search query is being typed, every
// key belongs to it"). Pressed on such a frame `j` types the letter into the
// query or the line, or puts the quick replies away — it never moves the list,
// and every folded row stays folded, under a footer that names `esc` for the
// way back (#232, #250). The count is the row's own answer at every level
// (#137) and stays.
func TestTheFleetFoldDropsItsKeyWhileALineIsBeingTyped(t *testing.T) {
	profiles := []struct {
		name string
		p    termenv.Profile
	}{{"forceASCII", termenv.Ascii}, {"colour on", termenv.TrueColor}}
	prev := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(prev)

	// The routes that leave a line being typed at Lv1: the search, the
	// quick replies, and the reply's own typed line.
	captured := [][]string{{"/"}, {"r"}, {"r", "t"}}
	// The routes that leave the fleet holding its keys, for the held side.
	free := [][]string{{}, {"j"}, {"esc"}, {"x"}}

	shed, kept := 0, 0
	for _, prof := range profiles {
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := size[0], size[1]
				for _, route := range captured {
					m := r100fhWalk(sc, w, h, route)
					if !m.searching && !m.replying {
						continue
					}
					row := r100fhFold(m.View())
					if row == "" {
						continue
					}
					shed++
					if strings.Contains(row, "· j") || strings.Contains(row, "· k") {
						t.Errorf("%s %s %dx%d %v: the fold names a key the line has taken: %q over %q",
							prof.name, sc.name, w, h, route, row, r100fhFoot(m.View()))
					}
					if !strings.Contains(row, "more below") && !strings.Contains(row, "more above") {
						t.Errorf("%s %s %dx%d %v: the fold lost its count: %q",
							prof.name, sc.name, w, h, route, row)
					}
					// Pressed on that very frame, `j` leaves the list where
					// it stands: the fold is unmoved or gone with the line.
					after := r100fhWalk(sc, w, h, append(append([]string(nil), route...), "j"))
					got := r100fhFold(after.View())
					if got != "" && r100fhCount(got) != r100fhCount(row) {
						t.Errorf("%s %s %dx%d %v: `j` moved the list, %q -> %q",
							prof.name, sc.name, w, h, route, row, got)
					}
				}
				for _, route := range free {
					m := r100fhWalk(sc, w, h, route)
					if m.searching || m.replying || m.focus() != panelFleet {
						continue
					}
					if row := r100fhFold(m.View()); row != "" &&
						(strings.Contains(row, "· j") || strings.Contains(row, "· k")) {
						kept++
					}
				}
			}
		}
	}
	if shed < 20 {
		t.Fatalf("the walk reached only %d folds under a typed line", shed)
	}
	if kept < 20 {
		t.Fatalf("the fleet kept its key on only %d folds — the yield took too many", kept)
	}
}

// ---- round 100, second-day ----
// ---- round 100, second day: the digit that moved nothing costs no level ----
//
// `A` then `1` on the second day's archive at eighty drew
//
//	j/k move · a ask · A fleet · ? help · q quit            the session you are on
//
// under a frame whose body the digit had not moved by one cell: the answer to
// a key that did nothing cost the footer `tab deeper`, the frame's only
// naming of the way deeper, and `tab` pressed there walks straight to Lv2.
// That is the harm #156, #159, #175, #187, #190, #194, #198, #201 and #264
// each folded, and #156 and #159 folded it on this very scene at this very
// width by shortening the sentence. The answer is `the one you are on`: the
// digit and the name are the header's (#233, #238), and eighteen cells leave
// the way deeper standing.
func TestTheDigitRefusalKeepsTheWayDeeper(t *testing.T) {
	const said = "the one you are on"
	const wasSaid = "the session you are on"
	levelKeys := []string{"tab deeper", "tab session", "tab reader"}
	rows := func(m *Model) []string { return strings.Split(ansi.Strip(m.View()), "\n") }
	footerOf := func(m *Model) string { r := rows(m); return r[len(r)-1] }
	bodyOf := func(m *Model) string { r := rows(m); return strings.Join(r[:len(r)-1], "\n") }
	walk := func(sc scene, w, h int, route ...string) *Model {
		m := sceneModel(sc, w, h)
		for _, k := range route {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}

	// The frame it was found on, under both profiles (#215, #218).
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		sc := sceneSecondDay()
		m := walk(sc, 80, 24, "A")
		if !strings.Contains(footerOf(m), "tab deeper") {
			lipgloss.SetColorProfile(old)
			t.Fatalf("prof=%v: the archive's own footer names no way deeper: %q", prof, footerOf(m))
		}
		before := bodyOf(m)
		pressKey(m, "1")
		poll(m, sc)
		if m.note != said {
			t.Errorf("prof=%v: the digit of the row the archive is on answered %q", prof, m.note)
		}
		if bodyOf(m) != before {
			t.Errorf("prof=%v: the digit that says it moved nothing moved the frame", prof)
		}
		if f := footerOf(m); !strings.Contains(f, "tab deeper") {
			t.Errorf("prof=%v: the answer cost the frame the way deeper: %q", prof, f)
		}
		// And the way deeper it names is the one `tab` takes.
		lv := m.level
		pressKey(m, "tab")
		poll(m, sc)
		if m.level <= lv {
			t.Errorf("prof=%v: `tab` on the frame naming `tab deeper` went from Lv%d to Lv%d", prof, lv, m.level)
		}
		lipgloss.SetColorProfile(old)
	}

	// The rule, over every scene at five widths under both profiles: where
	// the digit answers that it is the row you are on, the answer costs the
	// footer no key naming a level.
	stood, seen := 0, 0
	for _, sc := range allScenes() {
		for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
				old := lipgloss.ColorProfile()
				lipgloss.SetColorProfile(prof)
				for _, route := range [][]string{{"1"}, {"3"}, {"A", "1"}} {
					m := walk(sc, wh[0], wh[1], route[:len(route)-1]...)
					was := footerOf(m)
					pressKey(m, route[len(route)-1])
					poll(m, sc)
					seen++
					if strings.Contains(footerOf(m), wasSaid) {
						t.Errorf("%s %dx%d prof=%v route=%v: the long form still stands", sc.name, wh[0], wh[1], prof, route)
					}
					if m.note != said {
						continue
					}
					stood++
					for _, k := range levelKeys {
						if strings.Contains(was, k) && !strings.Contains(footerOf(m), k) {
							t.Errorf("%s %dx%d prof=%v route=%v: the answer cost the frame %q: %q",
								sc.name, wh[0], wh[1], prof, route, k, footerOf(m))
						}
					}
				}
				lipgloss.SetColorProfile(old)
			}
		}
	}
	if stood == 0 {
		t.Fatalf("the sentence never stood over %d stands: the digit says nothing", seen)
	}
}

// ---- round 100, two-tools ----
// The session view and the reader name the hide key.
//
// `case "x"` is the deck's, not a level's: `toggleHidden` runs at the
// board, at the list, in the session view and in the reader alike. At the
// last two it takes the session out from under the person — at 220 on the
// two-tools scene, `2` `tab` and `x` swap `2 api · opencode · sonnet-4-5`
// for `3 api · claude · opus-4-1`, header, trail and reader together —
// over a footer 168 cells wide in a 219-cell field with fifty-one blank
// cells and no `x` on it. A key that acts and is never named is the one
// thing a footer is for (#24, #175, #187, #277), and the row that refuses
// `x` must name `x` (#227).
//
// Two sides, so the fold cannot be got by naming the key everywhere:
//   - where `x` acts and the drawn row has the cells free, the row names it;
//   - where the row names it, `x` acts — the key is never offered on a
//     refusal (#227) or on an archived row with nothing to bring back.
func r100ttFooterRow(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return strings.TrimRight(rows[len(rows)-1], " ")
}

// r100ttHideActs presses `x` from this stand on a copy and says whether
// the frame moved: the frame is what a person sees (#215, #218, #221).
// It also hands back the note the press left, so a key that moves nothing
// can be asked whether it said why (#24, #221, #227).
func r100ttHideActs(sc scene, w, h int, route []string) (bool, string) {
	m := sceneModel(sc, w, h)
	for _, k := range route {
		pressKey(m, k)
		poll(m, sc)
	}
	before := ansi.Strip(m.View())
	rows := strings.Split(before, "\n")
	body := strings.Join(rows[:len(rows)-1], "\n")
	pressKey(m, "x")
	poll(m, sc)
	after := strings.Split(ansi.Strip(m.View()), "\n")
	return strings.Join(after[:len(after)-1], "\n") != body, m.note
}

func TestTheSessionViewAndTheReaderNameTheHideKey(t *testing.T) {
	forceASCII(t)
	routes := [][]string{
		{"tab"},
		{"tab", "tab"},
		{"2", "tab"},
		{"2", "tab", "tab"},
		{"x", "A", "tab"},
	}
	deep, acted, named, full := 0, 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := size[0], size[1]
				for ri, route := range routes {
					m := sceneModel(sc, w, h)
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					if m.level < levelWaypoints || m.showHelp || m.searching || m.replying {
						continue
					}
					deep++
					foot := r100ttFooterRow(m)
					clause := "x hide"
					if m.archiveView {
						clause = "x unhide"
					}
					on := strings.Contains(foot, clause)
					acts, said := r100ttHideActs(sc, w, h, route)
					if acts {
						acted++
					}
					if on {
						named++
					}
					tag := fmt.Sprintf("%s %dx%d r%d Lv%d p%v", sc.name, w, h, ri, m.level, prof)
					// The row must not overrun its field, whatever it names.
					if x := lipgloss.Width(foot); x > w-1 {
						t.Errorf("%s: the footer overruns its field: %d cells: %q", tag, x, foot)
					}
					// The biting side, on the rows nothing has been shed
					// from: the attach aside is the first fragment the
					// footer gives up (`shedOrder`), so a row still
					// wearing it is a row shed of nothing at all. There
					// the key costs no other, and `x` acts, so the row
					// names it. A shed row is #39's — its backfill stops
					// at the first key too wide, the price the unit
					// refusal pays at eighty too (#281) — so those stands
					// are walked and counted and only this side passes
					// them by.
					whole := strings.Contains(foot, attachHint)
					if whole {
						full++
					}
					if acts && !on && whole {
						t.Errorf("%s: `x` acts — it takes this very session off the board — and the row, shed of nothing, says so nowhere (%d cells free): %q",
							tag, w-1-lipgloss.Width(foot), foot)
					}
					// The held side: a key on the row answers from the row.
					// Where `x` moves nothing it says why, and the row
					// keeps the key its own note is about (#24, #57, #227).
					if on && !acts && !strings.Contains(said, " stays · ") && said != "the live one stays" && said != "it is not hidden" {
						t.Errorf("%s: the row offers %q where `x` neither moves nor says why (note %q): %q", tag, clause, said, foot)
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if deep < 40 {
		t.Fatalf("the walk reached only %d stands at Lv2 or deeper", deep)
	}
	if acted < 20 {
		t.Fatalf("`x` acted at only %d of them — the rule was not exercised", acted)
	}
	if full < 10 {
		t.Fatalf("only %d stands drew a row shed of nothing — the biting side was not exercised", full)
	}
	t.Logf("stands: %d · `x` acts: %d · rows shed of nothing: %d · named: %d", deep, acted, full, named)
}

// ---- round 101, fleet-hygiene ----
// ---- round 101, fleet-hygiene ----

// r101fhDoorRow matches the archive's own line in every form it is drawn and
// sheds to — "recent · 41 archived · A browses", "0 of 300 archived · A",
// "41 archived · 1 hidden · A browses", "12 archived" — with the band's rule
// to the gutter and the panel's own edge already trimmed off it.
var r101fhDoorRow = regexp.MustCompile(`^(?:recent · )?\d+(?: of \d+)? archived(?: · \d+(?: of \d+)? hidden)?(?: · A(?: browses)?)?$`)

// r101fhDoor is the archive's line on a frame, read out of whichever column
// draws it: the fleet's last row, the band's header, the board's strip or the
// session view's own fallback. "" when the frame draws no such row.
func r101fhDoor(view string) string {
	for _, ln := range strings.Split(ansi.Strip(view), "\n") {
		for _, seg := range strings.Split(ln, "│") {
			seg = strings.Trim(seg, "─ \t")
			if r101fhDoorRow.MatchString(seg) {
				return seg
			}
		}
	}
	return ""
}

// r101fhFooter is the last row of a frame — the keys the frame offers.
func r101fhFooter(view string) string {
	rows := strings.Split(ansi.Strip(view), "\n")
	return strings.TrimSpace(rows[len(rows)-1])
}

// r101fhPress walks a route on a fresh scene model, polling after each key
// the way the deck's own refresh does.
func r101fhPress(sc scene, w, h int, route ...string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range route {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// TestTheArchiveLineDropsItsKeyWhileALineIsBeingTyped holds the archive's own
// line — the fleet's last row, the band's header, the board's strip — to the
// key it names. #282 shed the fold's `· j` at Lv1 wherever a search, the quick
// replies or the reply's typed line has the keyboard, because every key then
// belongs to that line ("While a search query is being typed, every key
// belongs to it", app.go). `A` is on the same side of that gate: pressed on
// such a frame it types the letter into the query or the line, or puts the
// replies away, and the archive does not open — while the row beside the fold
// went on saying `A browses`. The footer has been gated on these very two
// flags all along (app.go: the `A archive` clause), so the frame said with one
// row what it refused to say with another. What goes is the key; the count
// stays, so the archive is still discovered and the reply box still steps off
// the line it would cover (#62, #64, #126, #137).
func TestTheArchiveLineDropsItsKeyWhileALineIsBeingTyped(t *testing.T) {
	profiles := []struct {
		name string
		p    termenv.Profile
	}{{"forceASCII", termenv.Ascii}, {"colour on", termenv.TrueColor}}
	prev := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(prev)

	// The routes that leave a line being typed: the search, the quick
	// replies, and the reply's own typed line.
	captured := [][]string{{"/"}, {"r"}, {"r", "t"}}
	// The routes that leave the deck holding its keys, for the held side.
	free := [][]string{{}, {"j"}, {"esc"}}

	shed, kept := 0, 0
	for _, prof := range profiles {
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := size[0], size[1]
				for _, route := range captured {
					m := r101fhPress(sc, w, h, route...)
					if !m.searching && !m.replying || m.archiveView {
						continue
					}
					view := m.View()
					row := r101fhDoor(view)
					if row == "" {
						continue
					}
					shed++
					if strings.Contains(row, " · A") {
						t.Errorf("%s %s %dx%d %v: the archive line names a key the typed line has taken: %q over %q",
							prof.name, sc.name, w, h, route, row, r101fhFooter(view))
					}
					if !strings.Contains(row, "archived") {
						t.Errorf("%s %s %dx%d %v: the archive line lost its count: %q",
							prof.name, sc.name, w, h, route, row)
					}
					// Pressed on that very frame, `A` does not browse.
					after := r101fhPress(sc, w, h, append(append([]string(nil), route...), "A")...)
					if after.archiveView {
						t.Errorf("%s %s %dx%d %v: `A` opened the archive after all, so the key belongs on %q",
							prof.name, sc.name, w, h, route, row)
					}
				}
				for _, route := range free {
					m := r101fhPress(sc, w, h, route...)
					if m.searching || m.replying || m.archiveView || m.archivedCount() == 0 {
						continue
					}
					if row := r101fhDoor(m.View()); strings.Contains(row, " · A") {
						kept++
					}
				}
			}
		}
	}
	if shed < 20 {
		t.Fatalf("the walk reached only %d archive lines under a typed line", shed)
	}
	if kept < 20 {
		t.Fatalf("the archive line kept its key on only %d frames — the yield took too many", kept)
	}
}

// ---- round 101, two-tools ----
// r101ttSpare is the cells the keys leave in their own field: the note is
// right-aligned behind a gap the keymap never contains (#134's reserve),
// so the row's width with a note on it says nothing about what the keys
// have room for.
func r101ttSpare(foot string, w int) int {
	keys := foot
	if i := strings.Index(strings.TrimPrefix(keys, " "), "  "); i >= 0 {
		keys = strings.TrimPrefix(keys, " ")[:i]
	}
	return w - 1 - lipgloss.Width(keys)
}

func r101ttFoot(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return strings.TrimRight(rows[len(rows)-1], " ")
}

// r101ttPressX walks the route again on a fresh model, presses `x` and
// says whether the frame above the footer moved, with the note the press
// left: the frame is what a person sees (#215, #218, #221).
func r101ttPressX(sc scene, w, h int, route []string) (bool, string) {
	m := sceneModel(sc, w, h)
	for _, k := range route {
		pressKey(m, k)
		poll(m, sc)
	}
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	body := strings.Join(rows[:len(rows)-1], "\n")
	pressKey(m, "x")
	poll(m, sc)
	after := strings.Split(ansi.Strip(m.View()), "\n")
	return strings.Join(after[:len(after)-1], "\n") != body, m.note
}

// ---- round 101, two-tools, the one thing ----
// The archive names the hide key on the row it keeps. `x` is the deck's
// key at every level (#284): in the archive it brings a hidden row back,
// and on the live row an archive with nothing in it keeps (#244, #248) the
// same key takes that row off the board — the header's chips change, a row
// lands back in the archive under `hidden · x brings one back`, and at Lv2
// and Lv3 the trail and the reader go with it, while the only word about
// any of it is the note after the press. The clause was dropped there on
// the reasoning that "the cursor is not on a hidden row: the key answers
// no question", and the key answers it. The word is the row's: `x unhide`
// where the key brings one back, `x hide` where it takes one off, which is
// what the live list one `A` away already draws for the same key on the
// same session (#24, #175, #187, #277, #284). On an archived row `x`
// refuses in its own words and `hideKeyStuck` takes the clause under #93's
// rule, which this leaves alone.
//
// Two sides, so the pin cannot be got by naming the key everywhere:
//   - where `x` takes the archive's live row off the board and the drawn
//     row is shed of nothing, the footer names `x hide`;
//   - where the archive's footer names a hide clause, `x` acts or says
//     why in its own words (#227).
func TestTheArchiveNamesTheHideKeyOnTheRowItKeeps(t *testing.T) {
	forceASCII(t)
	routes := [][]string{
		{"A", "x", "x"},
		{"2", "x", "A", "x", "x"},
		{"2", "x", "A", "x", "tab", "tab"},
	}
	kept, whole, named, offered := 0, 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := size[0], size[1]
				for ri, route := range routes {
					m := sceneModel(sc, w, h)
					for i, k := range route {
						pressKey(m, k)
						poll(m, sc)
						if !m.archiveView || m.showHelp || m.searching || m.replying {
							continue
						}
						s, ok := m.selected()
						if !ok {
							continue
						}
						foot := r101ttFoot(m)
						tag := fmt.Sprintf("%s %dx%d r%d s%d Lv%d p%v", sc.name, w, h, ri, i+1, m.level, prof)
						if x := lipgloss.Width(foot); x > w-1 {
							t.Errorf("%s: the footer overruns its field: %d cells: %q", tag, x, foot)
						}
						on := strings.Contains(foot, " · x hide") || strings.Contains(foot, " · x unhide")
						if on {
							offered++
						}
						keeps := s.Live && m.onBoard(s)
						if !on && !keeps {
							continue // `x` is neither offered here nor the archive's own row: #93's rule holds it
						}
						// The held side: a key on the row answers from the
						// row — where `x` moves nothing it says why, and
						// the row keeps the key its own note is about
						// (#24, #57, #227).
						acts, said := r101ttPressX(sc, w, h, route[:i+1])
						if on && !acts && !strings.Contains(said, " stays · ") && said != "the live one stays" && said != "it is not hidden" {
							t.Errorf("%s: the archive offers the hide key where `x` neither moves nor says why (note %q): %q", tag, said, foot)
						}
						if !keeps {
							continue
						}
						// The row the archive keeps: `x` takes it off the
						// board, so the clause is `x hide` and it is the
						// row's own key.
						kept++
						if !acts {
							t.Errorf("%s: the archive's live row does not move under `x` (note %q) — the pin's premise is gone: %q", tag, said, foot)
						}
						if strings.Contains(foot, " · x unhide") {
							t.Errorf("%s: the archive says `x unhide` on a row that is on the board: %q", tag, foot)
						}
						// The biting side, on the rows nothing has been
						// shed from: the attach aside is the first
						// fragment `shedOrder` gives up, so a row still
						// wearing it is a row shed of nothing at all.
						// A shed row is #39's and is walked past here.
						if strings.Contains(foot, attachHint) {
							whole++
							if !strings.Contains(foot, " · x hide") {
								t.Errorf("%s: `x` takes this row off the board and the row, shed of nothing, says so nowhere (%d cells free): %q",
									tag, r101ttSpare(foot, w), foot)
							}
						}
						if strings.Contains(foot, " · x hide") {
							named++
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if kept < 12 {
		t.Fatalf("the walk stood on only %d archive rows the board still keeps", kept)
	}
	if whole < 6 {
		t.Fatalf("only %d of them drew a row shed of nothing — the biting side was not exercised", whole)
	}
	if offered < 12 {
		t.Fatalf("the archive offered a hide clause at only %d stands — the held side was not exercised", offered)
	}
	t.Logf("archive stands on a kept live row: %d · shed of nothing: %d · naming `x hide`: %d · hide clause offered: %d", kept, whole, named, offered)
}

// ---- round 101, second-day ----
// TestThePageKeyDownAnswersLikeItsSiblingsOnATrailOfOne pins round 101's
// second-day fold: on a trail of one row `ctrl+d` said `at the present ·
// k goes back`, naming a key that on the same frame answers `no leg to
// move to` — the answer `j`, `k` and `ctrl+u` all give at that end of that
// trail — and at eighty its fourteen extra cells cost the footer
// `tab deeper`, the frame's only naming of the way deeper (#175, #187,
// #190, #194, #198, #201, #264, #283).
//
// Two sides. The frame it was found on: second-day and first-session at
// eighty, `tab` then `ctrl+d`, under both colour profiles (#215, #218) —
// the note is the short one, `k` pressed there says the same thing (#221),
// and `tab deeper` stands. Then the rule over every scene, five widths and
// both profiles: wherever the session view's trail draws one row, all four
// movement keys answer alike, and the long form never stands on a frame
// where `k` moves nothing.
func TestThePageKeyDownAnswersLikeItsSiblingsOnATrailOfOne(t *testing.T) {
	const short = "no leg to move to"
	const long = "at the present · k goes back"

	profiles := []struct {
		name string
		p    termenv.Profile
	}{{"ascii", termenv.Ascii}, {"truecolor", termenv.TrueColor}}
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	// footer is the frame's last drawn row, colour stripped.
	footer := func(frame string) string {
		rows := strings.Split(ansi.Strip(frame), "\n")
		for i := len(rows) - 1; i >= 0; i-- {
			if strings.TrimSpace(rows[i]) != "" {
				return rows[i]
			}
		}
		return ""
	}
	// stand replays a route from a scene's opening frame.
	stand := func(sc scene, w, h int, keys ...string) *Model {
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}
	sceneNamed := func(name string) scene {
		for _, sc := range allScenes() {
			if sc.name == name {
				return sc
			}
		}
		t.Fatalf("no scene %q", name)
		return scene{}
	}

	// --- the frames it was found on -------------------------------------
	for _, pr := range profiles {
		lipgloss.SetColorProfile(pr.p)
		for _, name := range []string{"second-day", "first-session"} {
			sc := sceneNamed(name)
			m := stand(sc, 80, 24, "tab", "ctrl+d")
			if m.level != levelWaypoints {
				t.Fatalf("%s 80 %s: expected the session view, got Lv%d", name, pr.name, m.level)
			}
			if m.note != short {
				t.Errorf("%s 80 %s: ctrl+d on a trail of one says %q, want %q", name, pr.name, m.note, short)
			}
			row := footer(m.View())
			if strings.Contains(row, long) {
				t.Errorf("%s 80 %s: the long form still stands: %q", name, pr.name, row)
			}
			if !strings.Contains(row, "tab deeper") {
				t.Errorf("%s 80 %s: the way deeper is off the row: %q", name, pr.name, row)
			}
			// #221: the key the old note named, pressed on that frame.
			k := stand(sc, 80, 24, "tab", "ctrl+d", "k")
			if k.note != short {
				t.Errorf("%s 80 %s: k pressed on the noted frame says %q, want %q", name, pr.name, k.note, short)
			}
			// and the way deeper is a key that acts.
			deeper := stand(sc, 80, 24, "tab", "ctrl+d", "tab")
			if deeper.level <= m.level {
				t.Errorf("%s 80 %s: tab did not go deeper (Lv%d)", name, pr.name, deeper.level)
			}
		}
	}

	// --- the rule, everywhere -------------------------------------------
	sizes := [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}
	checked := 0
	for _, pr := range profiles {
		lipgloss.SetColorProfile(pr.p)
		for _, sc := range allScenes() {
			for _, size := range sizes {
				w, h := size[0], size[1]
				for _, route := range [][]string{{"tab"}, {"2", "tab"}, {"3", "tab"}} {
					base := stand(sc, w, h, route...)
					if base.level != levelWaypoints {
						continue
					}
					if len(TrailRows(base.trail, base.level)) > 1 {
						continue // the long form is honest where k moves
					}
					checked++
					for _, key := range []string{"ctrl+d", "ctrl+u", "j", "k"} {
						m := stand(sc, w, h, append(append([]string{}, route...), key)...)
						if m.note != short {
							t.Errorf("%s %dx%d %s route=%v: %s answers %q on a trail of one, want %q",
								sc.name, w, h, pr.name, route, key, m.note, short)
						}
						if strings.Contains(footer(m.View()), long) {
							t.Errorf("%s %dx%d %s route=%v: %s draws the long form where k moves nothing",
								sc.name, w, h, pr.name, route, key)
						}
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no trail of one was reached: the rule was never asked")
	}
}

// ---- round 102, fleet-hygiene ----
// ---- round 102, fleet-hygiene ----

// r102fhHiddenClause matches the hidden count in every form a drawn row
// wears it — "1 hidden", "1 hidden · A, then x", "0 of 2 hidden" — wherever
// it sits on the row: alone on the fleet's last line, between the overlaps
// and the archive count on the board's strip, or on the header's chips.
var r102fhHiddenClause = regexp.MustCompile(`\d+(?: of \d+)? hidden(?: · A, then x)?`)

// r102fhCounts is every hidden clause a frame draws, the footer left out:
// the footer is the keymap's own row and the note's, and this pin is about
// what the *body* claims while the footer is a typed line's.
func r102fhCounts(view string) []string {
	rows := strings.Split(ansi.Strip(view), "\n")
	last := len(rows) - 1
	for last > 0 && strings.TrimSpace(rows[last]) == "" {
		last--
	}
	var out []string
	for i, ln := range rows {
		if i == last {
			continue
		}
		out = append(out, r102fhHiddenClause.FindAllString(ln, -1)...)
	}
	return out
}

// r102fhFooter is the last drawn row of a frame — the keys it offers.
func r102fhFooter(view string) string {
	rows := strings.Split(ansi.Strip(view), "\n")
	for i := len(rows) - 1; i >= 0; i-- {
		if strings.TrimSpace(rows[i]) != "" {
			return strings.TrimSpace(rows[i])
		}
	}
	return ""
}

// r102fhWalk replays a route from a scene's opening frame, polling after
// each key the way the deck's own refresh does.
func r102fhWalk(sc scene, w, h int, route ...string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range route {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// TestTheHiddenCountDropsItsKeysWhileALineIsBeingTyped holds the hidden
// count's clause to the keys it names. #282 shed the fold's `· j` and #285
// the archive line's `· A browses` wherever a search query, the quick
// replies or the reply's own typed line has the keyboard, because every key
// then belongs to that line ("While a search query is being typed, every
// key belongs to it", app.go). `A, then x` is the last clause on the same
// side of that gate, and on the board's strip it sits on the very row #285
// folded: at HEAD one row reads `1 hidden · A, then x   300 archived`, the
// archive count shed of its key and the hidden count keeping two. Pressed
// there, `A` types the letter into the query or into the line the deck is
// about to send and `x` types `x`; neither goes near the archive. The
// footer names neither key in these states already, so what goes is the
// keys, not the count: the hidden sessions are still counted where they
// have always been counted (#137, #173, #199, #202, #285).
//
// Two sides, so the pin cannot be got by taking the clause everywhere:
//   - where a line is being typed, no drawn row names the keys, the count
//     still stands, and `A` pressed on that frame does not open the archive;
//   - where the deck holds its own keys, the clause is still drawn.
func TestTheHiddenCountDropsItsKeysWhileALineIsBeingTyped(t *testing.T) {
	profiles := []struct {
		name string
		p    termenv.Profile
	}{{"forceASCII", termenv.Ascii}, {"colour on", termenv.TrueColor}}
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	// `x` takes the selected session off the board, so every route below
	// stands on a fleet with something hidden. The captured routes then
	// give the keyboard to the search, the quick replies and the reply's
	// own line, at Lv0/Lv1 and again in the session view where the
	// header's chip carries the clause (#202).
	captured := [][]string{
		{"x", "/"},
		{"x", "r"},
		{"x", "r", "t"},
		{"x", "tab", "/"},
		{"x", "tab", "r"},
	}
	// The routes that leave the deck holding its keys, for the held side.
	free := [][]string{{"x"}, {"x", "j"}, {"x", "tab"}}

	shed, kept := 0, 0
	for _, prof := range profiles {
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := size[0], size[1]
				for _, route := range captured {
					m := r102fhWalk(sc, w, h, route...)
					if !m.searching && !m.replying {
						continue
					}
					if m.hiddenCount() == 0 || m.archiveView {
						continue
					}
					view := m.View()
					counts := r102fhCounts(view)
					if len(counts) == 0 {
						continue // the box covers the row on this frame
					}
					shed++
					for _, c := range counts {
						if strings.Contains(c, "A, then x") {
							t.Errorf("%s %s %dx%d %v: the hidden count names keys the typed line has taken: %q over %q",
								prof.name, sc.name, w, h, route, c, r102fhFooter(view))
						}
					}
					// Pressed on that very frame, `A` does not browse
					// (#221): it goes into the query or the line.
					after := r102fhWalk(sc, w, h, append(append([]string(nil), route...), "A")...)
					if after.archiveView {
						t.Errorf("%s %s %dx%d %v: `A` opened the archive after all, so the keys belong on %q",
							prof.name, sc.name, w, h, route, counts[0])
					}
				}
				for _, route := range free {
					m := r102fhWalk(sc, w, h, route...)
					if m.searching || m.replying || m.archiveView || m.hiddenCount() == 0 {
						continue
					}
					for _, c := range r102fhCounts(m.View()) {
						if strings.Contains(c, "A, then x") {
							kept++
						}
					}
				}
			}
		}
	}
	if shed < 20 {
		t.Fatalf("the walk reached only %d hidden counts under a typed line", shed)
	}
	if kept < 20 {
		t.Fatalf("the hidden count kept its keys on only %d rows — the yield took too many", kept)
	}
}

// ---- round 102, second-day ----
// TestTheSessionViewNamesTheSearchKey pins round 102's second-day fold:
// `/` opens the fleet search at Lv2 exactly as it does on the board, on a
// list and in the reader — it takes the header's `/query · n of m`,
// narrows the fleet beside the trail and swaps the row for `/▏ · enter
// keeps it · esc cancels` — and no key on the row said so, on the very
// frame a fleet of one opens at. `shedOrder` has ranked `· / search` among
// this level's own keys all along with nothing on the row to match: a key
// that acts and is never named is the one thing a footer is for (#24,
// #175, #187, #277, #284). The clause is taken where the finished row
// still names every key it named without it (#281, #284), so a row the
// width has already cut into keeps its keys and says nothing.
//
// Two sides. The frame it was found on: second-day and first-session at
// the three widths whose opening frame stands at Lv2, under both colour
// profiles (#215, #218) — the row names `/ search`, `/` pressed there
// opens the fleet search (#221), and the level's own keys still stand.
// Then the rule over every scene, five widths and both profiles: wherever
// the walk stands at Lv2 on a row shed of nothing (one still wearing the
// attach aside, the first fragment `shedOrder` gives up) and `/` opens the
// fleet search, the row names it — and, the held side, wherever a row at
// that level names `/ search`, the key it names acts from there.
func TestTheSessionViewNamesTheSearchKey(t *testing.T) {
	forceASCII(t)
	foot := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		for i := len(rows) - 1; i >= 0; i-- {
			if strings.TrimSpace(rows[i]) != "" {
				return rows[i]
			}
		}
		return ""
	}
	keysOf := func(row string) string {
		s := strings.TrimSpace(row)
		if i := strings.Index(s, "  "); i >= 0 {
			s = s[:i]
		}
		return s
	}
	// opensSearch presses `/` on the stand itself and takes the deck back
	// with the `esc` the search row names, so the walk goes on from where
	// it stood. It reports whether the fleet search opened and whether the
	// round trip put the deck back; where it did not, the caller walks the
	// route again rather than trusting a moved stand.
	type stand struct {
		level                     int
		cursor                    int
		query, note               string
		searching, replying, help bool
	}
	at := func(m *Model) stand {
		return stand{m.level, m.cursor, m.query, m.note, m.searching, m.replying, m.showHelp}
	}
	opensSearch := func(m *Model, sc scene) (bool, bool) {
		was := at(m)
		pressKey(m, "/")
		poll(m, sc)
		opened := m.searching && m.searchFleet
		pressKey(m, "esc")
		poll(m, sc)
		m.note = was.note // the round trip's own answer, not the stand's
		return opened, at(m) == was
	}
	sizes := [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}
	profiles := []termenv.Profile{termenv.Ascii, termenv.TrueColor}

	// --- the frame it was found on: the opening frame of a fleet of one ---
	found := 0
	for _, prof := range profiles {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			if sc.name != "second-day" && sc.name != "first-session" {
				continue
			}
			for _, size := range sizes {
				w, h := size[0], size[1]
				m := sceneModel(sc, w, h)
				if m.level != levelWaypoints {
					continue // the deck opens on the board or the list at this width
				}
				tag := fmt.Sprintf("%s %dx%d p%v opening", sc.name, w, h, prof)
				keys := keysOf(foot(m))
				opened, back := opensSearch(m, sc)
				if !opened {
					t.Fatalf("%s: `/` opens no fleet search here — the pin's premise is gone: %q", tag, keys)
				}
				if !back {
					t.Fatalf("%s: `/` then `esc` did not put the deck back", tag)
				}
				found++
				if !strings.Contains(keys, "/ search") {
					t.Errorf("%s: `/` opens the fleet search on the frame this scene opens at and the row names it nowhere: %q", tag, keys)
				}
				for _, k := range []string{"j/k legs", "m live pane", "r reply", "a ask", "tab reader", "? help", "q quit"} {
					if !strings.Contains(keys, k) {
						t.Errorf("%s: the search key cost the row %q: %q", tag, k, keys)
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if found < 12 {
		t.Fatalf("only %d opening frames stood at Lv2 — the frame side was not exercised", found)
	}

	// --- the rule, over every scene ---
	whole, named, offered := 0, 0, 0
	for _, prof := range profiles {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			if prof != termenv.Ascii && sc.name != "second-day" && sc.name != "first-session" {
				continue // colour is measured on this operator's own two scenes (#215, #218)
			}
			for _, size := range sizes {
				w, h := size[0], size[1]
				m := sceneModel(sc, w, h)
				route := []string{}
				for i := 0; i <= len(canonicalKeys); i++ {
					if i > 0 {
						pressKey(m, canonicalKeys[i-1])
						poll(m, sc)
						route = append(route, canonicalKeys[i-1])
					}
					if m.level != levelWaypoints || m.showHelp || m.searching || m.replying {
						continue
					}
					row := foot(m)
					keys := keysOf(row)
					tag := fmt.Sprintf("%s %dx%d p%v Lv%d stand %d", sc.name, w, h, prof, m.level, i)
					if x := lipgloss.Width(row); x > w {
						t.Errorf("%s: the footer overruns its terminal: %d cells: %q", tag, x, row)
					}
					acts, back := opensSearch(m, sc)
					if !back {
						// The round trip moved the stand (a query was
						// standing on it): walk the route again so the
						// rest of the walk is the walk.
						m = sceneModel(sc, w, h)
						for _, k := range route {
							pressKey(m, k)
							poll(m, sc)
						}
					}
					on := strings.Contains(keys, "/ search")
					if on {
						named++
						// The held side: a row names no key that does not
						// act from where it stands (#24, #227).
						if !acts {
							t.Errorf("%s: the row names `/ search` where `/` opens no fleet search: %q", tag, keys)
						}
					}
					if !acts {
						continue
					}
					offered++
					// The biting side, on the rows nothing has been shed
					// from: a row still wearing the attach aside has given
					// up no fragment at all, so it has the cells for the
					// clause and must carry it.
					if strings.Contains(row, attachHint) {
						whole++
						if !on {
							t.Errorf("%s: `/` opens the fleet search here and the row, shed of nothing, says so nowhere: %q", tag, keys)
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if offered < 100 {
		t.Fatalf("the walk stood at only %d Lv2 stands where `/` acts", offered)
	}
	if whole < 20 {
		t.Fatalf("only %d of them drew a row shed of nothing — the biting side was not exercised", whole)
	}
	t.Logf("Lv2 stands where `/` acts: %d · rows shed of nothing: %d · naming `/ search`: %d", offered, whole, named)
}

// ---- round 102, two-tools ----
// r102ttMirrorSizes are the five terminals the walkthrough is drawn at.
var r102ttMirrorSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r102ttMirrorRoutes are four ways into the reader from the opening frame.
var r102ttMirrorRoutes = [][]string{{"tab", "tab"}, {"2", "tab", "tab"}, {"3", "tab", "tab"}, {"j", "tab", "tab"}}

// r102ttMirrorFrame is one drawn frame with its escapes stripped, split into
// the rows above the footer and the footer itself.
func r102ttMirrorFrame(m *Model) (body, footer string) {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	if len(rows) == 0 {
		return "", ""
	}
	return strings.Join(rows[:len(rows)-1], "\n"), rows[len(rows)-1]
}

// r102ttMirrorOverruns is the first row of the frame wider than the terminal,
// or "" when every row fits.
func r102ttMirrorOverruns(m *Model, w int) string {
	for _, r := range strings.Split(ansi.Strip(m.View()), "\n") {
		if lipgloss.Width(r) > w {
			return r
		}
	}
	return ""
}

// TestTheReadersMirrorKeyIsNotASilentToggle pins both sides of `m` in the
// reader. The reader is a level, not a panel (#15), so the frame `m` is
// pressed on there draws the conversation whichever way the mirror's flag
// stands: switched on, `m` goes to the session view with the live pane and
// says `the live pane`; switched off, it left the reader byte for byte the
// same with an empty note and moved a panel the person only meets one `esc`
// later — a silent key of exactly the shape the width guard on the same key
// refuses in its own words (#62, #221, #241). In the reader `m` means the
// live pane on either press.
//
// The other side, so the pin cannot be got by making `m` one-way everywhere:
// in the session view the key is still the toggle it has been since #62 —
// pressed with the mirror standing it draws the conversation and says so —
// and at the board the flag still flips.
func TestTheReadersMirrorKeyIsNotASilentToggle(t *testing.T) {
	forceASCII(t)
	standing, first, toggles := 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range r102ttMirrorSizes {
				w, h := size[0], size[1]
				for _, route := range r102ttMirrorRoutes {
					m := sceneModel(sc, w, h)
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					if !(m.sessionView() && m.level >= levelReader) {
						continue
					}
					where := sc.name + " " + route[0] + " " + itoaPin(w) + "x" + itoaPin(h)

					// First press: the mirror is off. It goes to the pane.
					preBody, preFoot := r102ttMirrorFrame(m)
					pressKey(m, "m")
					poll(m, sc)
					first++
					if !m.showMirror || m.level != levelWaypoints {
						t.Errorf("%s: `m` in the reader did not go to the live pane: mirror=%v level=%d", where, m.showMirror, m.level)
						continue
					}
					if got := strings.TrimSpace(m.note); got != "the live pane" {
						t.Errorf("%s: `m` in the reader said %q, want %q", where, got, "the live pane")
					}
					if body, _ := r102ttMirrorFrame(m); body == preBody {
						t.Errorf("%s: `m` in the reader drew the same frame\n  foot=%q", where, strings.TrimSpace(preFoot))
					}
					if r := r102ttMirrorOverruns(m, w); r != "" {
						t.Errorf("%s: row over %d cells: %q", where, w, r)
					}

					// Back into the reader, the mirror standing: the same key
					// must answer the same way, not flip a panel in silence.
					pressKey(m, "tab")
					poll(m, sc)
					if !(m.sessionView() && m.level >= levelReader) {
						continue
					}
					standing++
					wasBody, wasFoot := r102ttMirrorFrame(m)
					pressKey(m, "m")
					poll(m, sc)
					body, _ := r102ttMirrorFrame(m)
					note := strings.TrimSpace(m.note)
					if body == wasBody && note == "" {
						t.Errorf("%s: `m` in the reader turned the mirror %v and drew the same frame with no note (%d cells free)\n  foot=%q",
							where, m.showMirror, w-lipgloss.Width(wasFoot), strings.TrimSpace(wasFoot))
						continue
					}
					if !m.showMirror || m.level != levelWaypoints || note != "the live pane" {
						t.Errorf("%s: `m` in the reader with the mirror standing gave mirror=%v level=%d note=%q, want the live pane at the session view",
							where, m.showMirror, m.level, note)
					}
					if r := r102ttMirrorOverruns(m, w); r != "" {
						t.Errorf("%s: row over %d cells: %q", where, w, r)
					}

					// The other side: in the session view the key is still a
					// toggle, and its way back is named on the row (#62).
					if _, foot := r102ttMirrorFrame(m); !strings.Contains(foot, " · m conversation") {
						t.Errorf("%s: the session view under the mirror does not name the way back\n  foot=%q", where, strings.TrimSpace(foot))
					}
					pressKey(m, "m")
					poll(m, sc)
					toggles++
					if m.showMirror {
						t.Errorf("%s: `m` in the session view did not draw the conversation back", where)
					}
					if got := strings.TrimSpace(m.note); got != "the conversation" {
						t.Errorf("%s: `m` in the session view said %q, want %q", where, got, "the conversation")
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	// Not vacuous: the stands the two sides are measured on.
	if first < 24 {
		t.Errorf("only %d reader stands to press `m` on; the pin measures nothing", first)
	}
	if standing < 24 {
		t.Errorf("only %d reader stands with the mirror standing; the silent press is unmeasured", standing)
	}
	if toggles < 24 {
		t.Errorf("only %d session-view stands; the toggle's other side is unmeasured", toggles)
	}
}

// itoaPin is strconv.Itoa under a name no other file in this package uses.
func itoaPin(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// ---- round 103, fleet-hygiene ----
// ---- round 103, fleet-hygiene ----

// r103fhGroupHeader is the archive's hidden-group header as a frame draws
// it: the fleet column's own row, read up to the panel rule, so the trail
// beside it is not part of the label.
func r103fhGroupHeader(view string) (string, bool) {
	rows := strings.Split(ansi.Strip(view), "\n")
	last := len(rows) - 1
	for last > 0 && strings.TrimSpace(rows[last]) == "" {
		last--
	}
	for i, ln := range rows {
		if i == last {
			continue // the footer is the keymap's own row, not the body's
		}
		cell := ln
		if j := strings.Index(cell, "│"); j >= 0 {
			cell = cell[:j]
		}
		cell = strings.TrimSpace(cell)
		if cell == "hidden" || strings.HasPrefix(cell, "hidden · ") {
			return cell, true
		}
	}
	return "", false
}

// r103fhLastRow is the last drawn row of a frame — the keys it offers.
func r103fhLastRow(view string) string {
	rows := strings.Split(ansi.Strip(view), "\n")
	for i := len(rows) - 1; i >= 0; i-- {
		if strings.TrimSpace(rows[i]) != "" {
			return strings.TrimSpace(rows[i])
		}
	}
	return ""
}

// r103fhWalk replays a route from a scene's opening frame, polling after
// each key the way the deck's own refresh does.
func r103fhWalk(sc scene, w, h int, route ...string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range route {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// TestTheArchivesHiddenHeaderDropsItsKeyWhileALineIsBeingTyped holds the
// archive's first group header to the key it names. #282 shed the fold's
// `· j`, #285 the archive line's `· A browses` and #288 the hidden count's
// `· A, then x` wherever a search query, the quick replies or the reply's
// own typed line has the keyboard, because every key then belongs to that
// line ("While a search query is being typed, every key belongs to it" and
// "While the quick replies are up, a digit picks one and anything else puts
// them away", app.go). `hidden · x brings one back` is the same door on the
// archive's own header and had not taken the rule: pressed on such a frame
// `x` types `x` into the query or into the line the deck is about to send,
// or puts the replies away — it brings nothing back. What goes is the key,
// not the group: the rows under the header are still the hidden ones, which
// is the header's whole question, and the way on is `esc`, which every one
// of those footers names (#137, #285, #288).
//
// Two sides, so the pin cannot be got by taking the clause everywhere:
//   - where a line is being typed, the header names no key, the group's own
//     word still stands, and `x` pressed on that very frame brings nothing
//     back (#221);
//   - where the deck holds its own keys, the header still names `x`.
func TestTheArchivesHiddenHeaderDropsItsKeyWhileALineIsBeingTyped(t *testing.T) {
	profiles := []struct {
		name string
		p    termenv.Profile
	}{{"forceASCII", termenv.Ascii}, {"colour on", termenv.TrueColor}}
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	// `x` takes the selected session off the board and `A` opens the
	// archive, where it is listed under the hidden group; the rest of each
	// route gives the keyboard to the search, the quick replies or the
	// reply's own line.
	captured := [][]string{
		{"x", "A", "/"},
		{"x", "A", "r"},
		{"x", "A", "r", "t"},
	}
	// The routes that leave the deck holding its keys, for the held side.
	free := [][]string{{"x", "A"}, {"x", "A", "j"}}

	shed, kept := 0, 0
	for _, prof := range profiles {
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := size[0], size[1]
				for _, route := range captured {
					m := r103fhWalk(sc, w, h, route...)
					if !m.archiveView || (!m.searching && !m.replying) {
						continue
					}
					view := m.View()
					label, ok := r103fhGroupHeader(view)
					if !ok {
						continue // the group is not on this frame
					}
					shed++
					if strings.Contains(label, "brings one back") {
						t.Errorf("%s %s %dx%d %v: the archive's hidden header names a key the typed line has taken: %q over %q",
							prof.name, sc.name, w, h, route, label, r103fhLastRow(view))
					} else if label != "hidden" {
						t.Errorf("%s %s %dx%d %v: the group lost more than its key: %q",
							prof.name, sc.name, w, h, route, label)
					}
					// Pressed on that very frame, `x` brings nothing
					// back (#221): it goes into the query or the line,
					// or it puts the quick replies away.
					after := r103fhWalk(sc, w, h, append(append([]string(nil), route...), "x")...)
					if after.hiddenCount() != m.hiddenCount() {
						t.Errorf("%s %s %dx%d %v: `x` unhid a session after all, so the key belongs on %q",
							prof.name, sc.name, w, h, route, label)
					}
				}
				for _, route := range free {
					m := r103fhWalk(sc, w, h, route...)
					if !m.archiveView || m.searching || m.replying {
						continue
					}
					if label, ok := r103fhGroupHeader(m.View()); ok && strings.Contains(label, "brings one back") {
						kept++
					}
				}
			}
		}
	}
	if shed < 20 {
		t.Fatalf("the walk reached only %d hidden group headers under a typed line", shed)
	}
	if kept < 20 {
		t.Fatalf("the archive's hidden header kept its key on only %d rows — the yield took too many", kept)
	}
}

// ---- round 103, second-day ----
// TestTheFleetsMissNamesEscOnlyWhereEscClearsIt pins both sides of the second
// row of the fleet list's miss.
//
// The row is a promise about a key: `esc clears it`. On the board and on a
// list Esc does clear a standing search first (SPEC §3), and there the row is
// true. Beside a session view Esc is one level out instead, and the query goes
// only where that step lands on the board: on a narrow deck (no board to land
// on) and in the archive it does not, so the frame said the key clears the
// search while the key pressed on that very frame walked out with the search
// still standing. A row may name a key only where the key keeps its word
// (#24, #175, #187, #277, #282, #285).
//
// Held side: wherever Esc on the frame does clear the query, the miss still
// names it — the fold takes the clause away nowhere else.
func TestTheFleetsMissNamesEscOnlyWhereEscClearsIt(t *testing.T) {
	forceASCII(t)

	run := func(sc scene, w, h int, keys ...string) *Model {
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}
	named := func(name string) scene {
		for _, sc := range allScenes() {
			if sc.name == name {
				return sc
			}
		}
		t.Fatalf("no scene %q", name)
		return scene{}
	}
	drawn := func(m *Model, s string) bool { return strings.Contains(m.View(), s) }

	// 1. The frames it was found on: the second day's own fleet of one, at
	//    the two widths that have no board, and the archive at the widest.
	sd := named("second-day")
	found := []struct {
		what  string
		w, h  int
		keys  []string
		miss  string
		outLv int
		outQ  string
	}{
		{"the session view on a deck too narrow for a board", 80, 24,
			[]string{"tab", "/", "api", "enter"}, "no live session matches /api", levelTrail, "api"},
		{"the same at a hundred", 100, 30,
			[]string{"tab", "/", "api", "enter"}, "no live session matches /api", levelTrail, "api"},
		{"the archive one Tab deep, where the board fits", 220, 48,
			[]string{"A", "tab", "/", "zz", "enter"}, "no session matches /zz", levelTrail, "zz"},
	}
	for _, f := range found {
		m := run(sd, f.w, f.h, f.keys...)
		if !drawn(m, f.miss) {
			t.Fatalf("%s (%dx%d): the miss is not on the frame", f.what, f.w, f.h)
		}
		if m.level != levelWaypoints {
			t.Fatalf("%s (%dx%d): the stand is Lv%d, not the session view", f.what, f.w, f.h, m.level)
		}
		if drawn(m, "esc clears it") {
			t.Errorf("%s (%dx%d): the miss promises `esc clears it` here", f.what, f.w, f.h)
		}
		pressKey(m, "esc")
		poll(m, sd)
		if m.level != f.outLv || m.fleetQuery != f.outQ {
			t.Errorf("%s (%dx%d): esc went to Lv%d with query %q, wanted Lv%d and %q — the reason the clause is not drawn",
				f.what, f.w, f.h, m.level, m.fleetQuery, f.outLv, f.outQ)
		}
	}

	// 2. The held side on the same scene: one level out, where Esc is the
	//    list's own key, the row keeps its promise and the key keeps it too.
	for _, keep := range []struct {
		what string
		w, h int
		keys []string
	}{
		{"the fleet list at eighty", 80, 24, []string{"/", "api", "enter"}},
		{"the archive list at 220", 220, 48, []string{"A", "/", "zz", "enter"}},
	} {
		m := run(sd, keep.w, keep.h, keep.keys...)
		if !drawn(m, "esc clears it") {
			t.Errorf("%s: the miss no longer names the key that clears it:\n%s", keep.what, m.View())
			continue
		}
		pressKey(m, "esc")
		poll(m, sd)
		if m.fleetQuery != "" {
			t.Errorf("%s: `esc clears it` was drawn and esc left the query %q standing", keep.what, m.fleetQuery)
		}
	}

	// 3. The rule, over every scene, five widths and both profiles: wherever
	//    the clause is drawn the key clears, and wherever the key clears on a
	//    frame drawing the miss the clause is drawn.
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	prefixes := [][]string{{}, {"tab"}, {"A", "tab"}}
	stands, clause := 0, 0
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"mono", termenv.Ascii}, {"colour", termenv.TrueColor}} {
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				for _, pre := range prefixes {
					keys := append(append([]string{}, pre...), "/", "zz", "enter")
					m := run(sc, size[0], size[1], keys...)
					frame := m.View()
					if !strings.Contains(frame, "matches /zz") {
						continue
					}
					stands++
					says := strings.Contains(frame, "esc clears it")
					if says {
						clause++
					}
					where := fmt.Sprintf("%s %s %dx%d %v Lv%d", prof.name, sc.name, size[0], size[1], pre, m.level)
					pressKey(m, "esc")
					poll(m, sc)
					clears := m.fleetQuery == ""
					if says && !clears {
						t.Errorf("%s: the miss says `esc clears it` and esc left %q standing at Lv%d", where, m.fleetQuery, m.level)
					}
					if !says && clears {
						t.Errorf("%s: esc cleared the search and no row on the frame said so", where)
					}
				}
			}
		}
	}
	lipgloss.SetColorProfile(prev)
	if stands < 60 || clause < 20 {
		t.Fatalf("only %d stands drew the miss (%d naming the key); the sweep is not measuring the row", stands, clause)
	}
	t.Logf("stands drawing the miss: %d · naming `esc clears it`: %d", stands, clause)
}

// ---- round 103, two-tools ----
// ---- round 103, two-tools ----

// r103ttRowSizes are the five terminals the walkthrough is drawn at.
var r103ttRowSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r103ttRowRoutes are four ways into the reader from the opening frame.
var r103ttRowRoutes = [][]string{{"tab", "tab"}, {"2", "tab", "tab"}, {"3", "tab", "tab"}, {"j", "tab", "tab"}}

// r103ttRowFoot is the drawn footer of a frame, escapes stripped.
func r103ttRowFoot(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	if len(rows) == 0 {
		return ""
	}
	return rows[len(rows)-1]
}

// r103ttRowKeys is the keymap half of a footer: what stands before the gap
// the keymap never contains, the attach aside read past (#55, #134).
func r103ttRowKeys(foot string) string {
	s := strings.TrimRight(foot, " ")
	s = strings.Replace(s, " (prefix d returns)", "", 1)
	if i := strings.Index(strings.TrimLeft(s, " "), "  "); i >= 0 {
		s = strings.TrimLeft(s, " ")[:i]
	}
	return strings.TrimSpace(s)
}

// r103ttRowWide is the clause's own width in cells, separator and all.
const r103ttRowWide = " · m live pane"

// TestTheReadersRowNamesTheMirrorKey pins the reader's row to the key that
// acts on it. `m` is the deck's key, not a level's: pressed in the reader it
// takes the deck to the session view with the live pane standing and says
// `the live pane` — on either press, since #290 — and no key on the row said
// so, on a row that stood 145 cells of 220 with the rest blank. A key that
// acts and is never named is the one thing a footer is for (#24, #175, #187,
// #277, #284, #289).
//
// Three sides, so the pin cannot be got by jamming the clause onto every row:
//   - where the drawn row leaves the clause's own cells blank, it must name
//     the key;
//   - where the row names it, the key must do what the clause says;
//   - the label is one-sided, because the key is: in the reader there is no
//     `m conversation` to offer, on either side of the flag — the way back is
//     the key the session view names (#62, #290).
func TestTheReadersRowNamesTheMirrorKey(t *testing.T) {
	forceASCII(t)
	stands, roomy, named, standing := 0, 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range r103ttRowSizes {
				w, h := size[0], size[1]
				for _, route := range r103ttRowRoutes {
					for _, mirror := range []bool{false, true} {
						m := sceneModel(sc, w, h)
						for _, k := range route {
							pressKey(m, k)
							poll(m, sc)
						}
						if mirror {
							// The other side of the flag: the pane on,
							// then back into the reader.
							if !(m.sessionView() && m.level >= levelReader) {
								continue
							}
							pressKey(m, "m")
							poll(m, sc)
							pressKey(m, "tab")
							poll(m, sc)
						}
						if !(m.sessionView() && m.level >= levelReader) {
							continue
						}
						where := sc.name + " " + route[0] + " " + itoaPin(w) + "x" + itoaPin(h)
						if mirror {
							where += " (pane on)"
							standing++
						}
						foot := r103ttRowFoot(m)
						keys := r103ttRowKeys(foot)
						stands++

						// The label is the reader's own, on either side.
						if strings.Contains(keys, "m conversation") {
							t.Errorf("%s: the reader's row offers `m conversation`, but `m` here draws the live pane\n  foot=%q", where, keys)
						}

						// Does the key act on this very frame (#221)?
						// The stand is spent here: the row above is read,
						// then the key is pressed on that very model.
						n := m
						before := ansi.Strip(n.View())
						pressKey(n, "m")
						poll(n, sc)
						acts := ansi.Strip(n.View()) != before
						has := strings.Contains(keys, "m live pane")

						// The biting side is the row that has given up
						// nothing: the attach aside is the first fragment
						// `shedOrder` drops, so a row still wearing it is a
						// row shed of nothing at all (#284's own measure),
						// and so is one with the clause's own cells still
						// blank beside the note's reserve. There the clause
						// costs no key, and `m` acts, so the row names it.
						free := w - lipgloss.Width(foot)
						whole := strings.Contains(foot, attachHint) || free >= lipgloss.Width(r103ttRowWide)
						if acts && whole {
							roomy++
							if !has {
								t.Errorf("%s: `m` acts in the reader — it takes this very frame to the live pane (level %d, note %q) — and the row, shed of nothing, says so nowhere (%d cells free)\n  foot=%q",
									where, n.level, strings.TrimSpace(n.note), free, keys)
								continue
							}
						}
						if !has {
							continue
						}
						named++
						// The clause is taken only where it costs no key,
						// so the row that names it still names the way out
						// and the help — the last keys any row gives up
						// (#24, #39, #281, #284).
						for _, must := range []string{"esc back", "? help", "q quit"} {
							if !strings.Contains(keys, must) {
								t.Errorf("%s: the row names `m live pane` and no longer names %q\n  foot=%q", where, must, keys)
							}
						}
						// The row names it: the key must do what the clause says.
						if !acts {
							t.Errorf("%s: the row names `m live pane` and `m` moved nothing", where)
							continue
						}
						if !n.showMirror || n.level != levelWaypoints || strings.TrimSpace(n.note) != "the live pane" {
							t.Errorf("%s: the row names `m live pane` and `m` gave mirror=%v level=%d note=%q",
								where, n.showMirror, n.level, strings.TrimSpace(n.note))
						}
						for _, r := range strings.Split(ansi.Strip(n.View()), "\n") {
							if lipgloss.Width(r) > w {
								t.Errorf("%s: the frame `m` landed on has a row over %d cells: %q", where, w, r)
							}
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if stands < 24 {
		t.Errorf("only %d reader stands; the pin measures nothing", stands)
	}
	if standing < 12 {
		t.Errorf("only %d reader stands with the pane already on; the flag's other side is unmeasured", standing)
	}
	if roomy < 12 {
		t.Errorf("only %d reader stands with the clause's cells free; the biting side is unmeasured", roomy)
	}
	t.Logf("reader stands: %d · with the pane on: %d · with the clause's cells free: %d · naming `m live pane`: %d", stands, standing, roomy, named)
}

// ---- round 104, fleet-hygiene ----
// ---- round 104, fleet-hygiene ----

// r104fhBodyRows is a frame's drawn rows with the footer dropped: the
// footer is the keymap's own row and is gated on these flags already, so
// the question here is only what the board's own body says.
func r104fhBodyRows(view string) []string {
	rows := strings.Split(ansi.Strip(view), "\n")
	last := len(rows) - 1
	for last > 0 && strings.TrimSpace(rows[last]) == "" {
		last--
	}
	if last <= 0 {
		return nil
	}
	return rows[:last]
}

// r104fhDoorRow is the archive board's strip row as a frame draws it — the
// row that carries the door back to the live fleet — or "" where the board
// draws no such row.
func r104fhDoorRow(view string) string {
	for _, ln := range r104fhBodyRows(view) {
		if strings.Contains(ln, "A live fleet") {
			return strings.TrimSpace(ln)
		}
	}
	return ""
}

// r104fhFooter is the last drawn row of a frame — the keys it offers.
func r104fhFooter(view string) string {
	rows := strings.Split(ansi.Strip(view), "\n")
	for i := len(rows) - 1; i >= 0; i-- {
		if strings.TrimSpace(rows[i]) != "" {
			return strings.TrimSpace(rows[i])
		}
	}
	return ""
}

// r104fhWalk replays a route from a scene's opening frame, polling after
// each key the way the deck's own refresh does.
func r104fhWalk(sc scene, w, h int, route ...string) *Model {
	m := sceneModel(sc, w, h)
	for _, k := range route {
		pressKey(m, k)
		poll(m, sc)
	}
	return m
}

// TestTheArchiveBoardsDoorBackDropsItsKeyWhileALineIsBeingTyped holds the
// archive board's strip to the key it names. #282 shed the fold's `· j`,
// #285 the archive line's `· A browses`, #288 the hidden count's
// `· A, then x` and #291 the archive's hidden header its `· x brings one
// back`, wherever a search query, the quick replies or the reply's own
// typed line has the keyboard, because every key then belongs to that line
// ("While a search query is being typed, every key belongs to it" and
// "While the quick replies are up, a digit picks one and anything else
// puts them away", app.go). `A live fleet` on the archive board's strip is
// the last clause of that family and had not taken the rule: pressed on
// such a frame `A` types the letter into the query or into the line the
// deck is about to send, and the live fleet does not come back.
//
// Unlike the archive count beside it on the live board's strip, this
// clause is a signpost whole — key and destination, with no count to keep
// — so what goes is the clause, exactly as the footer's own `A fleet`
// already goes on these two flags. The header still reads `board` under
// `archive N of M` and the way on is `esc`, which every one of those
// footers names, so where the frame is is never in doubt (#62, #126).
//
// Three sides, so the pin cannot be got by taking the clause everywhere:
//   - where a line is being typed the strip names no key;
//   - `A` pressed on that very frame does not bring the live fleet back
//     (#221);
//   - where the deck holds its own keys the strip still says `A live
//     fleet`, counted, so a yield that shed it always fails here.
func TestTheArchiveBoardsDoorBackDropsItsKeyWhileALineIsBeingTyped(t *testing.T) {
	profiles := []struct {
		name string
		p    termenv.Profile
	}{{"forceASCII", termenv.Ascii}, {"colour on", termenv.TrueColor}}
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })

	// `A` opens the archive and `⇧tab` puts its board up; the suffix then
	// gives the keyboard to the search, the quick replies or a typed line.
	toBoard := []string{"A", "shift+tab"}
	suffixes := [][]string{{"/"}, {"/", "a"}, {"r"}, {"r", "t"}, {"a"}}

	shed, kept := 0, 0
	for _, prof := range profiles {
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := size[0], size[1]
				base := r104fhWalk(sc, w, h, toBoard...)
				if !base.archiveView || base.searching || base.replying {
					continue
				}
				if r104fhDoorRow(base.View()) == "" {
					continue // this deck draws no archive board here
				}
				kept++
				for _, suffix := range suffixes {
					route := append(append([]string(nil), toBoard...), suffix...)
					m := r104fhWalk(sc, w, h, route...)
					if !m.archiveView || (!m.searching && !m.replying) {
						continue // the key opened no line on this deck
					}
					shed++
					if row := r104fhDoorRow(m.View()); row != "" {
						t.Errorf("%s %s %dx%d %v: the archive board's strip names a key the typed line has taken: %q over %q",
							prof.name, sc.name, w, h, route, row, r104fhFooter(m.View()))
					}
					// Pressed on that very frame, `A` does not bring the
					// live fleet back (#221): it goes into the query or
					// into the line the deck is about to send.
					after := r104fhWalk(sc, w, h, append(append([]string(nil), route...), "A")...)
					if !after.archiveView {
						t.Errorf("%s %s %dx%d %v: `A` left the archive after all, so the key belongs on the strip",
							prof.name, sc.name, w, h, route)
					}
				}
			}
		}
	}
	if shed < 20 {
		t.Fatalf("the walk reached only %d archive-board strips under a typed line", shed)
	}
	if kept < 20 {
		t.Fatalf("the archive board's strip kept its door on only %d frames — the yield took too many", kept)
	}
}

// ---- round 104, two-tools ----
// ---- round 104, two-tools ----

// r104ttGrabSizes are the five terminals the walkthrough is drawn at.
var r104ttGrabSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r104ttGrabRoutes are four ways into the session view from the opening
// frame, and the same four one step deeper into the reader.
var r104ttGrabRoutes = [][]string{{"tab"}, {"2", "tab"}, {"3", "tab"}, {"j", "tab"}, {"j", "j", "tab"}, {"l", "tab"}}

// r104ttGrabClause is the clause's own width in cells, separator and all.
const r104ttGrabClause = " · g grab"

// r104ttGrabFoot is the drawn footer of a frame, escapes stripped.
func r104ttGrabFoot(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	if len(rows) == 0 {
		return ""
	}
	return rows[len(rows)-1]
}

// r104ttGrabKeys is the keymap half of a footer: what stands before the gap
// the keymap never contains, the attach aside read past (#55, #134).
func r104ttGrabKeys(foot string) string {
	s := strings.TrimRight(foot, " ")
	s = strings.Replace(s, " (prefix d returns)", "", 1)
	if i := strings.Index(strings.TrimLeft(s, " "), "  "); i >= 0 {
		s = strings.TrimLeft(s, " ")[:i]
	}
	return strings.TrimSpace(s)
}

// TestTheSessionViewsRowNamesTheGrabKey pins the session view's row to the
// key that acts on it. `g` is the fleet's key, not the board's: pressed at
// Lv2 it takes the oldest session waiting on you, moves the header, the
// trail and the reader onto it and attaches — on a two-tool fleet the deck
// comes back standing on the other tool's row — and no key on the row said
// so, on a row that stood 188 cells of 220 with thirty-two blank and the
// attach aside still on it. A key that acts and is never named is the one
// thing a footer is for (#24, #175, #187, #277, #284, #289), and the help
// in the reader sends the person here for it ("the grab is a level out",
// #246).
//
// Three sides, so the pin cannot be got by jamming the clause onto every
// row:
//   - where the drawn row is shed of nothing and `g` grabs, the row names
//     `g grab`;
//   - where the row names it, the key must do what the clause says — the
//     selection moves to a session waiting on you, the note says where it
//     went — and the row must still name the way out, the help and the quit;
//   - the reader's row never names it, because one level deeper `g` is the
//     start of the conversation and not the grab (#241, #246).
func TestTheSessionViewsRowNamesTheGrabKey(t *testing.T) {
	forceASCII(t)
	stands, roomy, named, readers := 0, 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range r104ttGrabSizes {
				w, h := size[0], size[1]
				for _, route := range r104ttGrabRoutes {
					for _, deeper := range []bool{false, true} {
						m := sceneModel(sc, w, h)
						for _, k := range route {
							pressKey(m, k)
							poll(m, sc)
						}
						if deeper {
							pressKey(m, "tab")
							poll(m, sc)
							if m.level < levelReader {
								continue
							}
							// One level deeper the key is not the grab.
							readers++
							if keys := r104ttGrabKeys(r104ttGrabFoot(m)); strings.Contains(keys, "g grab") {
								t.Errorf("%s %s %dx%d: the reader's row offers `g grab`, but `g` here is the start of the conversation (#241, #246)\n  foot=%q",
									sc.name, route[0], w, h, keys)
							}
							continue
						}
						if m.level != levelWaypoints {
							continue
						}
						where := fmt.Sprintf("%s %s %dx%d", sc.name, route[0], w, h)
						foot := r104ttGrabFoot(m)
						keys := r104ttGrabKeys(foot)
						stands++

						// Does the key grab on this very frame (#221)?
						// The stand is spent here: the row above is read,
						// then the key is pressed on that very model.
						was := m.selectedKey
						pressKey(m, "g")
						poll(m, sc)
						// `g` found a session waiting on you and said where
						// it went; `moved` is the harm — the deck came back
						// standing on another session's row.
						found := strings.HasPrefix(strings.TrimSpace(m.note), "→ ")
						moved := m.selectedKey != was
						has := strings.Contains(keys, "g grab")

						// The biting side is the row that has given up
						// nothing: the attach aside is the first fragment
						// `shedOrder` drops, so a row still wearing it is a
						// row shed of nothing at all (#284's own measure),
						// and so is one with the clause's own cells still
						// blank beside the note's reserve.
						free := w - lipgloss.Width(strings.TrimRight(foot, " "))
						whole := strings.Contains(foot, attachHint) || free >= lipgloss.Width(r104ttGrabClause)
						if moved && whole {
							roomy++
							if !has {
								t.Errorf("%s: `g` grabs from the session view — it moves the deck onto %q and attaches (note %q) — and the row, shed of nothing, says so nowhere (%d cells free)\n  foot=%q",
									where, m.selectedKey, strings.TrimSpace(m.note), free, keys)
								continue
							}
						}
						if !has {
							continue
						}
						named++
						// The clause is taken only where it costs no key,
						// so the row that names it still names the way out
						// and the help — the last keys any row gives up
						// (#24, #39, #281, #284).
						for _, must := range []string{"esc board", "? help", "q quit"} {
							if !strings.Contains(keys, must) {
								t.Errorf("%s: the row names `g grab` and no longer names %q\n  foot=%q", where, must, keys)
							}
						}
						// The row names it: the key must do what the
						// clause says — find the session waiting on you and
						// say where it went (#78).
						if !found {
							t.Errorf("%s: the row names `g grab` and `g` said %q, not where it went\n  foot=%q", where, strings.TrimSpace(m.note), keys)
							continue
						}
						for _, r := range strings.Split(ansi.Strip(m.View()), "\n") {
							if lipgloss.Width(r) > w {
								t.Errorf("%s: the frame `g` landed on has a row over %d cells: %q", where, w, r)
							}
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if stands < 120 {
		t.Errorf("only %d session-view stands; the pin measures nothing", stands)
	}
	if readers < 120 {
		t.Errorf("only %d reader stands; the held side is unmeasured", readers)
	}
	if roomy < 24 {
		t.Errorf("only %d session-view stands where the grab acts on a row shed of nothing; the biting side is unmeasured", roomy)
	}
	t.Logf("session-view stands: %d · rows shed of nothing where `g` grabs: %d · naming `g grab`: %d · reader stands: %d", stands, roomy, named, readers)
}

// ---- round 104, second-day ----
// ---- round 104, second-day, the one thing ----
// The archive's hide refusal keeps the way deeper on the archive's own
// session view. Round 93 folded this very harm on the archive's list and
// yielded the note from thirty-six cells to nineteen, but its routes were
// `{A}`, `{A,j}`, `{A,j,j}` and `{/,pytest,enter,A}` — never one `tab`
// deeper — so the row it never measured went on paying: at eighty,
// `A` then `tab` then `x` took ` j/k rows · [ ] chapters · tab deeper ·
// esc back · A fleet · ? help · q quit` (76 of 80) down to ` j/k rows ·
// esc back · A fleet · ? help · q quit  it is off the board`, losing
// `tab deeper`, the frame's only naming of the way deeper, for a note
// answering a key that moved nothing — the frame is byte-identical
// otherwise. That the length is the cause is on the same frame: `G`
// there draws `at the present`, fourteen cells, and `tab deeper` stands.
// The note answers the key's own question instead of repeating where the
// row is: `x` in the archive brings a hidden row back (SPEC §3), the
// archive's own header says `hidden · x brings one back` of the rows it
// does bring back (#291), and this row is not one of them. `it is not
// hidden`, sixteen cells; where the row is is what the frame says three
// ways already — the column title `▌FLEET · archive`, the header's
// `archive 12` chip and the row's own `○` (#24, #52, #175, #187, #190,
// #194, #198, #201, #210, #264, #283, #287).
func TestTheArchivesHideRefusalNamesTheWayDeeper(t *testing.T) {
	forceASCII(t)
	r104sdWayDeeper := func(foot string) bool {
		for _, k := range []string{"tab deeper", "tab reader", "tab session", "enter attach"} {
			if strings.Contains(foot, k) {
				return true
			}
		}
		return false
	}
	r104sdFoot := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		for i := len(rows) - 1; i >= 0; i-- {
			if strings.TrimSpace(rows[i]) != "" {
				return strings.TrimRight(rows[i], " ")
			}
		}
		return ""
	}
	r104sdBodyOf := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		for i := len(rows) - 1; i >= 0; i-- {
			if strings.TrimSpace(rows[i]) != "" {
				rows[i] = ""
				break
			}
		}
		return strings.Join(rows, "\n")
	}
	r104sdWalk := func(sc scene, w, h int, route []string) *Model {
		m := sceneModel(sc, w, h)
		for _, k := range route {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}

	// The frame it was found on: the second day's archive at eighty, one
	// `A`, one `tab`, one `x`.
	sc := sceneSecondDay()
	before := r104sdWalk(sc, 80, 24, []string{"A", "tab"})
	after := r104sdWalk(sc, 80, 24, []string{"A", "tab", "x"})
	if got, want := r104sdFoot(after),
		" j/k rows · tab deeper · esc back · A fleet · ? help · q quit  it is not hidden"; got != want {
		t.Errorf("second-day 80x24 A tab x:\n got  %q\n want %q", got, want)
	}
	if r104sdBodyOf(after) != r104sdBodyOf(before) {
		t.Errorf("second-day 80x24 A tab x: `x` moved something other than the footer")
	}
	// The same row one keypress earlier named the way deeper, and the
	// note must not have cost it.
	if !r104sdWayDeeper(r104sdFoot(before)) {
		t.Fatalf("second-day 80x24 A tab: the row this is measured against names no way deeper: %q", r104sdFoot(before))
	}

	// The rule, over every scene at five widths under both profiles, on
	// the archive's list, its session view and its reader: where `x` is
	// refused on an archived row, the row it draws keeps a key naming
	// the way deeper if the row one keypress earlier had one, and the
	// refusal is the archive's own sentence.
	routes := [][]string{{"A"}, {"A", "tab"}, {"A", "tab", "tab"}, {"A", "j", "tab"}}
	old := lipgloss.ColorProfile()
	defer lipgloss.SetColorProfile(old)
	stands, refused := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, sz := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := sz[0], sz[1]
				for _, route := range routes {
					b := r104sdWalk(sc, w, h, route)
					if !b.archiveView {
						continue
					}
					a := r104sdWalk(sc, w, h, append(append([]string{}, route...), "x"))
					stands++
					if r104sdBodyOf(a) != r104sdBodyOf(b) {
						continue // `x` acted: this is the held side
					}
					note := a.note
					if note == "" {
						continue
					}
					// `x` is the key that was pressed, so the note is
					// `x`'s own answer, whatever words it wears.
					refused++
					fb, fa := r104sdFoot(b), r104sdFoot(a)
					if r104sdWayDeeper(fb) && !r104sdWayDeeper(fa) {
						t.Errorf("%s %dx%d %v then x: the refusal %q cost the frame its only naming of the way deeper\n before %q\n after  %q",
							sc.name, w, h, route, note, fb, fa)
					}
				}
			}
		}
	}
	if stands < 150 || refused < 150 {
		t.Fatalf("the walk reached only %d archive stands, %d of them refusals", stands, refused)
	}
}

// ---- round 105, second-day ----
// TestTheNoPaneRefusalKeepsTheWayDeeper holds the no-pane refusal to the
// rule #165 gave it: it wears the naming form `attach needs a pane` /
// `reply needs a pane` "precisely so that naming the key would buy a key
// back", and where the naming buys nothing it costs cells instead. In the
// archive's own session view at eighty — `second-day`, `A` then `tab`, the
// frame one keypress deeper than the list #264 measured — the nineteen
// cells took `tab deeper` off the row, the frame's only naming of the way
// deeper, for a key that moved nothing; the same refusal on the archive's
// list one level out keeps the key. The note there is `no pane`, seven
// cells, the word the row's own `enter · no pane` clause already uses.
//
// Three sides: the frame it was found on, both keys; the rule over every
// scene, five widths, two routes and both profiles — wherever a no-pane
// refusal is drawn, the row keeps a key naming a level if the row one
// keypress earlier had one; and the naming form still stands where it
// costs nothing, on the canonical walkthrough that carries it.
func TestTheNoPaneRefusalKeepsTheWayDeeper(t *testing.T) {
	forceASCII(t)
	r105sdLevelKey := func(row string) bool {
		for _, k := range []string{"enter attach", "tab deeper", "tab session", "tab reader"} {
			if strings.Contains(row, k) {
				return true
			}
		}
		return false
	}
	r105sdFrame := func(sc scene, w, h int, keys []string) (*Model, []string) {
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		return m, strings.Split(ansi.Strip(m.View()), "\n")
	}
	r105sdScene := func(name string) scene {
		for _, sc := range allScenes() {
			if sc.name == name {
				return sc
			}
		}
		t.Fatalf("no scene %q", name)
		return scene{}
	}

	// The frame it was found on: the archive's own session view at eighty.
	sd := r105sdScene("second-day")
	for _, key := range []string{"enter", "r"} {
		_, rowsBefore := r105sdFrame(sd, 80, 24, []string{"A", "tab"})
		if got := strings.TrimRight(rowsBefore[len(rowsBefore)-1], " "); !strings.Contains(got, "tab deeper") {
			t.Fatalf("second-day 80x24 [A tab]: the row before %q names no way deeper: %q", key, got)
		}
		m, rows := r105sdFrame(sd, 80, 24, []string{"A", "tab", key})
		foot := strings.TrimRight(rows[len(rows)-1], " ")
		for i := range rows[:len(rows)-1] {
			if strings.TrimRight(rows[i], " ") != strings.TrimRight(rowsBefore[i], " ") {
				t.Errorf("second-day 80x24: %q moved row %d: %q -> %q", key, i, rowsBefore[i], rows[i])
			}
		}
		if !strings.HasSuffix(foot, "no pane") || strings.Contains(foot, "needs a pane") {
			t.Errorf("second-day 80x24 [A tab %s]: the refusal the row draws is %q, want it to end in %q", key, foot, "no pane")
		}
		if !strings.HasSuffix(m.note, " needs a pane") {
			t.Errorf("second-day 80x24 [A tab %s]: the refusal set is %q: the note itself is unchanged, only the form the row draws", key, m.note)
		}
		if !strings.Contains(foot, "tab deeper") {
			t.Errorf("second-day 80x24 [A tab %s]: the row lost the way deeper: %q", key, foot)
		}
		if lipgloss.Width(foot) > 80 {
			t.Errorf("second-day 80x24 [A tab %s]: the row runs past the terminal (%d): %q", key, lipgloss.Width(foot), foot)
		}
	}

	// The rule, over every scene at five widths under both profiles.
	routes := [][]string{{"A", "tab"}, {"A"}}
	stands, refusals := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				for _, route := range routes {
					for _, key := range []string{"enter", "r"} {
						stands++
						m := sceneModel(sc, size[0], size[1])
						for _, k := range route {
							pressKey(m, k)
							poll(m, sc)
						}
						rowsBefore := strings.Split(ansi.Strip(m.View()), "\n")
						was := rowsBefore[len(rowsBefore)-1]
						pressKey(m, key)
						poll(m, sc)
						rows := strings.Split(ansi.Strip(m.View()), "\n")
						foot := rows[len(rows)-1]
						if m.note != "no pane" && !strings.HasSuffix(m.note, " needs a pane") {
							continue
						}
						refusals++
						if r105sdLevelKey(was) && !r105sdLevelKey(foot) {
							t.Errorf("%v %s %dx%d %v then %q: the no-pane refusal %q costs the row its only naming of a level: %q -> %q",
								prof, sc.name, size[0], size[1], route, key, m.note, strings.TrimRight(was, " "), strings.TrimRight(foot, " "))
						}
						if lipgloss.Width(foot) > size[0] {
							t.Errorf("%v %s %dx%d %v then %q: the row runs past the terminal (%d): %q",
								prof, sc.name, size[0], size[1], route, key, lipgloss.Width(foot), foot)
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if refusals < 100 {
		t.Fatalf("the rule reached only %d no-pane refusals over %d stands: it has gone vacuous", refusals, stands)
	}

	// And the naming form stands where it costs no key: the canonical
	// walkthrough that draws both (#165, #232).
	walk := walkthrough(r105sdScene("fleet-hygiene"), 80, 24, canonicalKeys)
	for _, said := range []string{"attach needs a pane", "reply needs a pane"} {
		if !strings.Contains(walk, said) {
			t.Errorf("fleet-hygiene 80x24: the walkthrough no longer says %q: the naming form yielded where it cost no key", said)
		}
	}
}

// ---- round 105, two-tools ----
// ---- round 105, two-tools, the one thing ----

// r105ttArchiveSizes are the five terminals the walkthrough is drawn at.
var r105ttArchiveSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r105ttArchiveRoutes are ways into the archive's three levels — its board,
// its list and its session view — with and without a hidden live row in it.
var r105ttArchiveRoutes = [][]string{
	{"A"}, {"A", "tab"}, {"x", "A"},
	{"2", "x", "A"}, {"2", "x", "A", "esc"}, {"2", "x", "A", "tab"},
}

// r105ttArchiveClause is the clause the archive's row was missing.
const r105ttArchiveClause = " · g grab"

// r105ttArchiveFoot is the drawn footer of a frame, escapes stripped.
func r105ttArchiveFoot(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	if len(rows) == 0 {
		return ""
	}
	return rows[len(rows)-1]
}

// r105ttArchiveKeys is the keymap half of a footer: what stands before the
// gap the note lives after (#134's reserve), with the attach aside — which
// is not a key (#55) — read past.
func r105ttArchiveKeys(foot string) string {
	s := strings.TrimRight(foot, " ")
	s = strings.Replace(s, " (prefix d returns)", "", 1)
	if i := strings.Index(strings.TrimLeft(s, " "), "  "); i >= 0 {
		s = strings.TrimLeft(s, " ")[:i]
	}
	return strings.TrimSpace(s)
}

// TestTheArchivesRowNamesTheGrabKey pins the archive's rows to the key that
// acts on them. `g` is the fleet's key, and the archive is not outside the
// fleet: pressed on the archive's board, its list or its session view it
// finds the session waiting on you, leaves the archive for the live fleet
// standing on that session's row and attaches — on the two-tools scene from
// the hidden `2 api · opencode` to `1 infra · claude · sonnet-4-5`, the
// other tool on the other model — and no key on the row said so, on a row
// that stood 152 of 220 cells with sixty-eight blank and the attach aside
// still on it (`scenes/two-tools-220x48.txt:2002`). The keymap's own reason
// for dropping the clause — "in the archive `g` has nothing to grab and `A`
// is the way home" — is refuted on that frame twice over: `g` grabs, and
// `A` is a different key with a different landing (it comes home on the row
// you left; `g` comes home on another session's and attaches).
//
// Three sides, so the pin cannot be got by jamming the clause onto every
// row:
//   - where the archive's drawn row has given nothing up — the attach aside
//     is the first fragment `shedOrder` drops (#284's own measure) — and `g`
//     acts on it, the row names `g grab`;
//   - where the row names it, the key must do what the clause says (find the
//     session waiting on you and say where it went) and the row must still
//     name the way home, the help and the quit;
//   - the archive's reader never names it, because one level deeper `g` is
//     not the grab (#241, #246).
func TestTheArchivesRowNamesTheGrabKey(t *testing.T) {
	forceASCII(t)
	stands, roomy, named, readers := 0, 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range r105ttArchiveSizes {
				w, h := size[0], size[1]
				for _, route := range r105ttArchiveRoutes {
					for _, deeper := range []bool{false, true} {
						m := sceneModel(sc, w, h)
						for _, k := range route {
							pressKey(m, k)
							poll(m, sc)
						}
						if !m.archiveView {
							continue
						}
						if deeper {
							pressKey(m, "tab")
							poll(m, sc)
							if !m.archiveView || m.level < levelReader {
								continue
							}
							// One level deeper the key is not the grab.
							readers++
							if keys := r105ttArchiveKeys(r105ttArchiveFoot(m)); strings.Contains(keys, "g grab") {
								t.Errorf("%s %v %dx%d: the archive's reader offers `g grab`, but `g` here is not the grab (#241, #246)\n  foot=%q",
									sc.name, route, w, h, keys)
							}
							continue
						}
						if m.level >= levelReader {
							continue
						}
						where := fmt.Sprintf("%s %v %dx%d Lv%d", sc.name, route, w, h, m.level)
						foot := r105ttArchiveFoot(m)
						keys := r105ttArchiveKeys(foot)
						stands++

						// The key is pressed on that very frame (#221),
						// after the row above it has been read.
						was, lvl := m.selectedKey, m.level
						pressKey(m, "g")
						poll(m, sc)
						// `g` found a session waiting on you and said
						// where it went; the harm is that it took the
						// deck out of the archive, onto another row.
						found := strings.HasPrefix(strings.TrimSpace(m.note), "→ ")
						acts := !m.archiveView || m.selectedKey != was || m.level != lvl
						has := strings.Contains(keys, "g grab")

						// The biting side is the row that has given up
						// nothing at all: the attach aside is the first
						// fragment `shedOrder` drops, so a row still
						// wearing it has shed nothing (#284's measure) —
						// and so is a row that still names its own attach
						// key with the clause's cells standing blank
						// beside the note's reserve.
						free := w - lipgloss.Width(strings.TrimRight(foot, " "))
						whole := strings.Contains(foot, attachHint) ||
							(strings.Contains(keys, "enter") && free >= lipgloss.Width(r105ttArchiveClause))
						if acts && whole {
							roomy++
							if !has {
								t.Errorf("%s: `g` grabs from the archive — it leaves for the live fleet standing on %q and attaches (note %q) — and the row, shed of nothing, says so nowhere (%d cells free)\n  foot=%q",
									where, m.selectedKey, strings.TrimSpace(m.note), free, keys)
								continue
							}
						}
						if !has {
							continue
						}
						named++
						// The clause is taken only where it costs no key,
						// so the row that names it still names the way
						// home and the help — the last keys any archive
						// row gives up (#24, #39, #56, #281, #284).
						for _, must := range []string{"A fleet", "? help", "q quit"} {
							if !strings.Contains(keys, must) {
								t.Errorf("%s: the row names `g grab` and no longer names %q\n  foot=%q", where, must, keys)
							}
						}
						if !found {
							t.Errorf("%s: the row names `g grab` and `g` said %q, not where it went\n  foot=%q", where, strings.TrimSpace(m.note), keys)
							continue
						}
						for _, r := range strings.Split(ansi.Strip(m.View()), "\n") {
							if lipgloss.Width(r) > w {
								t.Errorf("%s: the frame `g` landed on has a row over %d cells: %q", where, w, r)
							}
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if stands < 200 {
		t.Errorf("only %d archive stands; the pin measures nothing", stands)
	}
	if readers < 40 {
		t.Errorf("only %d archive reader stands; the held side is unmeasured", readers)
	}
	if roomy < 20 {
		t.Errorf("only %d archive stands where the grab acts on a row shed of nothing; the biting side is unmeasured", roomy)
	}
	t.Logf("archive stands: %d · rows shed of nothing where `g` grabs: %d · naming `%s`: %d · archive reader stands: %d",
		stands, roomy, strings.TrimPrefix(r105ttArchiveClause, " · "), named, readers)
}

// ---- round 105, fleet-hygiene ----
// r105fhAttachNote is the refusal `enter` draws where the selected session
// has no pane (#165), and r105fhPaneClause is the keymap's own clause for
// the same key on the same row. They are one sentence: the note names the
// key the clause names and says what the clause says.
const (
	r105fhAttachNote  = "attach needs a pane"
	r105fhPaneClause  = "enter · no pane"
	r105fhSearchKey   = "/ search"
	r105fhTypedFooter = "esc cancels"
)

// r105fhRoute is one walk to a frame whose `enter` cannot attach.
type r105fhRoute struct {
	keys []string
	what string
}

// r105fhWalks are the ways to a paneless row: the archive at its list, its
// board and its session view — every archived session is paneless — and,
// on the fleets that have one, the live session with no pane.
var r105fhWalks = []r105fhRoute{
	{[]string{"A"}, "archive list"},
	{[]string{"A", "shift+tab"}, "archive board"},
	{[]string{"A", "tab"}, "archive session view"},
	{[]string{"3"}, "live list, the third session"},
}

// r105fhFooter is the last drawn row of a frame, colour stripped.
func r105fhFooter(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return strings.TrimSpace(rows[len(rows)-1])
}

// TestTheAttachRefusalSaysTheNoPaneOnce holds the attach refusal to the
// rule every other refusal on the deck keeps: the row says the sentence
// once. `enter` on a paneless row draws `attach needs a pane`, nineteen
// cells naming the key; where the same row also draws its own
// `enter · no pane` the frame says one thing twice, and on the archive's
// session view at 152 the second saying costs `/ search`, a key that acts
// on that very row (#95, #96, #165, #264). Three sides: under the note the
// clause is gone; the note still stands, so the frame says why; and the
// walk actually reaches such frames, so a yield that never draws the note
// fails too.
func TestTheAttachRefusalSaysTheNoPaneOnce(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	scenes := allScenes()
	reached, twice := 0, 0
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"mono", termenv.Ascii}, {"colour", termenv.TrueColor}} {
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range scenes {
			for _, size := range [][2]int{{80, 24}, {120, 34}, {152, 40}, {220, 48}} {
				for _, walk := range r105fhWalks {
					m := sceneModel(sc, size[0], size[1])
					for _, k := range walk.keys {
						pressKey(m, k)
						poll(m, sc)
					}
					before := r105fhFooter(m)
					if strings.Contains(before, r105fhTypedFooter) {
						continue // a line has the keyboard, not the deck
					}
					pressKey(m, "enter")
					poll(m, sc)
					foot := r105fhFooter(m)
					if !strings.Contains(foot, r105fhAttachNote) {
						continue // this row could be attached
					}
					reached++
					if strings.Contains(foot, r105fhPaneClause) {
						twice++
						t.Errorf("%s %s %dx%d %s: the row says the same sentence twice — %q beside %q\n   foot=%q",
							prof.name, sc.name, size[0], size[1], walk.what,
							r105fhPaneClause, r105fhAttachNote, foot)
					}
					if strings.Contains(before, r105fhSearchKey) && !strings.Contains(foot, r105fhSearchKey) &&
						size[0] >= 152 {
						t.Errorf("%s %s %dx%d %s: the refusal cost the row %q, which the row named one keypress earlier\n   was =%q\n   now =%q",
							prof.name, sc.name, size[0], size[1], walk.what, r105fhSearchKey, before, foot)
					}
				}
			}
		}
	}
	if reached == 0 {
		t.Fatalf("the walk reached no frame drawing %q", r105fhAttachNote)
	}
	t.Logf("frames drawing the refusal: %d, of which saying it twice: %d", reached, twice)
}

// ---- round 106, two-tools ----
// r106ttOffSizes are the five terminals the walkthrough is drawn at. Below
// `deckWideCols` the key refuses in its own words (`mirror needs 110
// columns`) and never flips the flag, so those two widths measure the guard
// rather than the toggle.
var r106ttOffSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r106ttOffRoutes are ways to a frame `m` is pressed on: the board it opens
// on and two moves across it, and three walks a level or two in, where the
// panel the key puts away is replaced by something the frame draws.
var r106ttOffRoutes = [][]string{
	{}, {"j"}, {"1"},
	{"tab"}, {"tab", "tab"}, {"tab", "tab", "]"},
}

// r106ttOffOn and r106ttOffBoard are the two halves of the board's mirror
// sentence: the state the first press says, and the state the second owes.
// `the conversation` and `the live pane` are the same sentence one and two
// levels in, where a panel answers for the key.
const (
	r106ttOffOn    = "mirror on"
	r106ttOffBoard = "mirror off"
	r106ttOffDeep  = "the conversation"
	r106ttOffPane  = "the live pane"
)

// r106ttOffFrame is one drawn frame, escapes stripped and read once: the
// rows above the footer, the footer, its keymap half and its note.
type r106ttOffFrame struct {
	rows []string
	body string
	foot string
	keys string
	note string
}

func r106ttOffRead(m *Model) r106ttOffFrame {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	f := r106ttOffFrame{rows: rows}
	if len(rows) == 0 {
		return f
	}
	f.body = strings.Join(rows[:len(rows)-1], "\n")
	f.foot = rows[len(rows)-1]
	f.keys = r106ttOffKeys(f.foot)
	s := strings.TrimLeft(strings.TrimRight(f.foot, " "), " ")
	if i := strings.Index(s, "  "); i >= 0 {
		f.note = strings.TrimSpace(s[i:])
	}
	return f
}

// r106ttOffKeys is the keymap half of a footer: what stands before the gap
// the note lives after (#134's reserve), with the attach aside — which is
// not a key (#55) — read past.
func r106ttOffKeys(foot string) string {
	s := strings.TrimRight(foot, " ")
	s = strings.Replace(s, " (prefix d returns)", "", 1)
	if i := strings.Index(strings.TrimLeft(s, " "), "  "); i >= 0 {
		s = strings.TrimLeft(s, " ")[:i]
	}
	return strings.TrimSpace(s)
}

// r106ttOffClauses splits a keymap half into its clauses, so a row can be
// asked whether it still names everything it named before.
func r106ttOffClauses(keys string) []string {
	var out []string
	for _, c := range strings.Split(keys, " · ") {
		if c = strings.TrimSpace(c); c != "" {
			out = append(out, c)
		}
	}
	return out
}

// TestTheBoardsMirrorKeySaysTheFlagBothWays pins the board's `m` to the
// state it changed. The mirror is a panel of Lv1 and Lv2 and the board
// draws it at neither (#15), so on the board the flag is the whole of what
// the key does: the first press says `mirror on` (#37, #177) and the second
// said nothing at all — the frame above the footer came back byte for byte
// and the note went, on 237 of the 912 corpus stands where `m` arms the
// mirror, every one of them the board. That is the silent second press #241
// refused for the reader's `g` and #290 refused for the reader's own `m`,
// on the one level where no panel answers for the key. The board says the
// state the other way round: `mirror off`, ten cells, inside the note's own
// reserve.
//
// Three sides, so the sentence cannot be jammed onto every level:
//   - on the board, where `m` turns the mirror off, the note is
//     `mirror off` and the body above the footer is unchanged, so the note
//     is the only thing that can answer;
//   - the row pays nothing for it: the off frame names every key the on
//     frame named, and the frame fits its terminal;
//   - one and two levels in the key keeps its own words — the session view
//     says `the conversation` and the reader `the live pane` on either
//     press (#290) — and no frame off the board ever says `mirror off`.
func TestTheBoardsMirrorKeySaysTheFlagBothWays(t *testing.T) {
	forceASCII(t)
	boards, deeps := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range r106ttOffSizes {
				w, h := size[0], size[1]
				for _, route := range r106ttOffRoutes {
					m := sceneModel(sc, w, h)
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					if m.showHelp || m.searching || m.replying || m.archiveView {
						continue
					}
					where := fmt.Sprintf("%s %v %dx%d Lv%d", sc.name, route, w, h, m.level)

					// The first press arms the mirror; the second is the
					// one under test, pressed on the frame it is read
					// from (#221).
					pressKey(m, "m")
					poll(m, sc)
					if !m.showMirror {
						continue
					}
					board := m.level == levelBoard
					on := r106ttOffRead(m)
					if board && on.note != r106ttOffOn {
						t.Errorf("%s: `m` armed the mirror on the board and said %q, not %q", where, on.note, r106ttOffOn)
					}
					pressKey(m, "m")
					poll(m, sc)
					off := r106ttOffRead(m)
					note, offKeys := off.note, off.keys

					if !board {
						// One and two levels in the key keeps its own
						// words, and never the board's.
						deeps++
						if note == r106ttOffBoard {
							t.Errorf("%s: `m` off the board says %q; here a panel answers for the key (#290)", where, note)
						}
						if m.sessionView() && note != r106ttOffDeep && note != r106ttOffPane {
							t.Errorf("%s: `m` in the session view says %q, not %q or %q", where, note, r106ttOffDeep, r106ttOffPane)
						}
						continue
					}

					boards++
					if off.body != on.body {
						t.Errorf("%s: `m` on the board redrew the frame above the footer; the board draws no mirror either way (#15)", where)
					}
					if note == "" {
						t.Errorf("%s: `m` turned the mirror off on the board and said nothing; the frame above the footer came back byte for byte and only the note went\n  foot=%q",
							where, strings.TrimRight(off.foot, " "))
						continue
					}
					if note != r106ttOffBoard {
						t.Errorf("%s: the board's mirror-off note is %q, not the state %q", where, note, r106ttOffBoard)
					}
					// The note is inside the reserve: the row gives up no
					// key for it (#39, #177, #281, #284).
					for _, clause := range r106ttOffClauses(on.keys) {
						if !strings.Contains(offKeys, clause) {
							t.Errorf("%s: the mirror-off note cost the row %q\n  on =%q\n  off=%q", where, clause, on.keys, offKeys)
						}
					}
					for _, r := range off.rows {
						if lipgloss.Width(r) > w {
							t.Errorf("%s: the frame `m` landed on has a row over %d cells: %q", where, w, r)
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if boards < 100 {
		t.Errorf("only %d board stands where `m` arms the mirror; the biting side is unmeasured", boards)
	}
	if deeps < 150 {
		t.Errorf("only %d stands off the board; the held side is unmeasured", deeps)
	}
	t.Logf("board stands where `m` arms the mirror: %d · stands off the board: %d", boards, deeps)
}

// ---- round 106, second-day ----
// TestThePresentNoteKeepsTheWayDeeper holds the rule of #300: the drawn
// form of `at the present · k goes back` gives up its way-back clause —
// which the row it stands on already names, first of all its clauses, as
// `j/k rows` — wherever the clause would cost the row a key naming a
// level that the row named one keypress earlier, and only there.
func TestThePresentNoteKeepsTheWayDeeper(t *testing.T) {
	forceASCII(t)
	r106sdLevelKey := func(row string) bool {
		for _, k := range []string{"enter attach", "tab deeper", "tab session", "tab reader"} {
			if strings.Contains(row, k) {
				return true
			}
		}
		return false
	}
	r106sdFrame := func(sc scene, w, h int, keys []string) (*Model, []string) {
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		return m, strings.Split(ansi.Strip(m.View()), "\n")
	}
	r106sdScene := func(name string) scene {
		for _, sc := range allScenes() {
			if sc.name == name {
				return sc
			}
		}
		t.Fatalf("no scene %q", name)
		return scene{}
	}

	// The frame it was found on: the archive's own session view at eighty,
	// the cursor on the newest leg, where `j` and `ctrl+d` move nothing.
	sd := r106sdScene("second-day")
	for _, key := range []string{"j", "ctrl+d"} {
		_, before := r106sdFrame(sd, 80, 24, []string{"A", "tab"})
		if got := strings.TrimRight(before[len(before)-1], " "); !strings.Contains(got, "tab deeper") {
			t.Fatalf("second-day 80x24 [A tab]: the row before %q names no way deeper: %q", key, got)
		}
		m, rows := r106sdFrame(sd, 80, 24, []string{"A", "tab", key})
		foot := strings.TrimRight(rows[len(rows)-1], " ")
		for i := range rows[:len(rows)-1] {
			if strings.TrimRight(rows[i], " ") != strings.TrimRight(before[i], " ") {
				t.Errorf("second-day 80x24: %q moved row %d: %q -> %q", key, i, before[i], rows[i])
			}
		}
		if strings.Contains(foot, "k goes back") || !strings.HasSuffix(foot, "at the present") {
			t.Errorf("second-day 80x24 [A tab %s]: the row draws %q, want it to end in %q with no way-back clause", key, foot, "at the present")
		}
		if !strings.Contains(foot, "tab deeper") {
			t.Errorf("second-day 80x24 [A tab %s]: the row lost the way deeper: %q", key, foot)
		}
		if !strings.Contains(foot, "j/k rows") {
			t.Errorf("second-day 80x24 [A tab %s]: the clause went and the row does not name the key it named: %q", key, foot)
		}
		if m.note != "at the present · k goes back" {
			t.Errorf("second-day 80x24 [A tab %s]: the note itself is %q: only the form the row draws yields", key, m.note)
		}
		if lipgloss.Width(foot) > 80 {
			t.Errorf("second-day 80x24 [A tab %s]: the row runs past the terminal (%d): %q", key, lipgloss.Width(foot), foot)
		}
	}

	// The rule, over every scene at five widths under both profiles.
	stands, refusals := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				for _, route := range [][]string{{"tab"}, {"A", "tab"}} {
					for _, key := range []string{"j", "ctrl+d"} {
						stands++
						m, before := r106sdFrame(sc, size[0], size[1], route)
						was := before[len(before)-1]
						pressKey(m, key)
						poll(m, sc)
						rows := strings.Split(ansi.Strip(m.View()), "\n")
						foot := rows[len(rows)-1]
						if m.note != "at the present · k goes back" {
							continue
						}
						refusals++
						if r106sdLevelKey(was) && !r106sdLevelKey(foot) {
							t.Errorf("%v %s %dx%d %v then %q: the present note costs the row its only naming of a level: %q -> %q",
								prof, sc.name, size[0], size[1], route, key,
								strings.TrimRight(was, " "), strings.TrimRight(foot, " "))
						}
						if strings.Contains(foot, "k goes back") && !strings.Contains(foot, "j/k ") {
							t.Errorf("%v %s %dx%d %v then %q: the row draws the way-back clause and names the key nowhere: %q",
								prof, sc.name, size[0], size[1], route, key, strings.TrimRight(foot, " "))
						}
						if lipgloss.Width(foot) > size[0] {
							t.Errorf("%v %s %dx%d %v then %q: the row runs past the terminal (%d): %q",
								prof, sc.name, size[0], size[1], route, key, lipgloss.Width(foot), foot)
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if refusals < 100 {
		t.Fatalf("the rule reached only %d present notes over %d stands: it has gone vacuous", refusals, stands)
	}

	// And the clause still stands where it costs no key: one width up from
	// the frame above, the same press keeps both (#175's proviso), so a
	// yield taken everywhere fails here.
	if _, rows := r106sdFrame(sd, 120, 34, []string{"A", "tab", "j"}); !strings.Contains(rows[len(rows)-1], "at the present · k goes back") {
		t.Errorf("second-day 120x34 [A tab j]: the row no longer says the way back: %q — the clause yielded where it cost no key",
			strings.TrimRight(rows[len(rows)-1], " "))
	}
}

// ---- round 106, fleet-hygiene ----
// r106fhReplyNote is the refusal `r` draws where the selected session has no
// pane (#165), and r106fhClause is the keymap's own clause for the same
// fact: the session has no pane, which is why neither key can work. A row
// drawing both says it twice, and #232 and #299 took the attach half of it.
const (
	r106fhReplyNote = "reply needs a pane"
	r106fhClause    = "enter · no pane"
	r106fhSearch    = "/ search"
	r106fhTyping    = "esc cancels"
)

// r106fhWalk is one way to a row with no pane and what to call it.
type r106fhWalk struct {
	keys []string
	what string
}

// r106fhWalks are the ways to a paneless row: the archive at its list, its
// board and its own session view, and the live list's third session — the
// fleet-hygiene fleet's session with no pane. They are #299's four, one
// level out from the session view, whose own row keeps every key it names
// and says the fact twice for them (recorded, not folded).
var r106fhWalks = []r106fhWalk{
	{[]string{"A"}, "archive list"},
	{[]string{"A", "shift+tab"}, "archive board"},
	{[]string{"A", "tab"}, "archive session view"},
	{[]string{"3"}, "live list, the third session"},
}

// r106fhRow is the last drawn row of a frame, colour stripped.
func r106fhRow(m *Model) string {
	rows := strings.Split(ansi.Strip(m.View()), "\n")
	return strings.TrimSpace(rows[len(rows)-1])
}

// r106fhNamed is the keys a drawn footer names: the row up to the gap the
// keymap never contains, split on its own separator.
func r106fhNamed(row string) map[string]bool {
	if i := strings.Index(row, "  "); i >= 0 {
		row = row[:i]
	}
	out := map[string]bool{}
	for _, frag := range strings.Split(row, " · ") {
		if f := strings.TrimSpace(frag); f != "" {
			out[f] = true
		}
	}
	return out
}

// TestTheReplyRefusalSaysTheNoPaneOnce holds three sides of the reply
// refusal on a row with no pane, at four widths under both colour profiles
// (#215, #218): the row under the note does not also draw the keymap's own
// `enter · no pane`, since the note says the same fact (#95, #96, #165,
// #232, #299); at 152 and wider the refusal does not cost the row the search
// key it named one keypress earlier (#264); and the walk reaches the refusal
// at all, so a yield that stopped drawing the note fails too.
func TestTheReplyRefusalSaysTheNoPaneOnce(t *testing.T) {
	prev := lipgloss.ColorProfile()
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	reached := 0
	for _, prof := range []struct {
		name string
		p    termenv.Profile
	}{{"mono", termenv.Ascii}, {"colour", termenv.TrueColor}} {
		lipgloss.SetColorProfile(prof.p)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {120, 34}, {152, 40}, {220, 48}} {
				for _, walk := range r106fhWalks {
					m := sceneModel(sc, size[0], size[1])
					for _, k := range walk.keys {
						pressKey(m, k)
						poll(m, sc)
					}
					before := r106fhRow(m)
					if strings.Contains(before, r106fhTyping) {
						continue // a line has the keyboard, not the deck
					}
					pressKey(m, "r")
					poll(m, sc)
					row := r106fhRow(m)
					if !strings.Contains(row, r106fhReplyNote) {
						continue // this row could be replied to
					}
					reached++
					if strings.Contains(row, r106fhClause) {
						t.Errorf("%s %s %dx%d %s: the row says the same fact twice — %q beside %q\n   row=%q",
							prof.name, sc.name, size[0], size[1], walk.what,
							r106fhClause, r106fhReplyNote, row)
					}
					// At 152 and wider the clause's own cells are the
					// search key's twice over, so the refusal never costs
					// the key the row named one keypress earlier (#264).
					// Narrower the row is full and the note pays (#175).
					if size[0] < 152 {
						continue
					}
					was, now := r106fhNamed(before), r106fhNamed(row)
					if was[r106fhSearch] && !now[r106fhSearch] {
						t.Errorf("%s %s %dx%d %s: the refusal cost the row %q, which the row named one keypress earlier\n   was=%q\n   now=%q",
							prof.name, sc.name, size[0], size[1], walk.what, r106fhSearch, before, row)
					}
				}
			}
		}
	}
	if reached == 0 {
		t.Fatalf("the walk reached no frame drawing %q", r106fhReplyNote)
	}
	t.Logf("frames drawing the reply refusal: %d", reached)
}
