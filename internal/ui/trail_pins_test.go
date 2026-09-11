package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

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
		// fleet-hygiene stands at 220. Since #327 the session view leaves
		// the band to the level above, so at 152 the footer names the
		// archive's door in its place (#203) on a row already full to its
		// last cell, and the page key goes at its rank for width. At 220
		// the row affords both, and the key under test is the one drawn.
		{"fleet-hygiene", sceneFleetHygiene, 220, 48},
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
		// alarm-storm holds the second stand. fleet-hygiene's row at 152
		// stood here until #327: the session view leaves the band to the
		// level above, the footer names the archive's door in its place
		// (#203), and the door outlasts the page key on the shed order —
		// so the cells the stuck chapter key frees go to the door and not
		// back to `ctrl+d/u`. alarm-storm has nothing archived, no door to
		// pay for, and the same stand: the chapter key refuses on both
		// halves and the page key walks the cursor.
		{"alarm-storm", sceneAlarmStorm, 152, 40, 9},
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
	sweep(t)
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

// r331Keys is the keys a drawn footer row names, read as the deck's own
// trades read them: past the note, which stands after the gap the keymap
// never contains (#134's reserve), and past the attach aside, which is
// not a key (#55).
func r331Keys(row string) []string {
	s := strings.ReplaceAll(ansi.Strip(row), attachHint, "")
	if i := strings.Index(s, "  "); i >= 0 {
		s = s[:i]
	}
	var out []string
	for _, frag := range strings.Split(s, " · ") {
		if f := strings.TrimSpace(frag); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// r331Gone is a keymap with these clauses taken out of it.
func r331Gone(keys string, clauses []string) string {
	for _, c := range clauses {
		keys = strings.Replace(keys, c, "", 1)
	}
	return keys
}

// ---- round 116, the two-tools operator ----
// A stuck key keeps no cells a row has already paid. #193's gate holds a
// key that cannot move on a row that has shed nothing, so that a wide
// footer still names what `[` and `]` are — but what a clause new to the
// row costs was counted key for key one layer out, with the refusing key
// among them. At 152 `fleet-hygiene`'s reader stood `space unfold ·
// / search · n/N · [ ] turns · h/l session · r reply · a ask · x hide ·
// enter attach · esc back · ? help · q quit  no later turn`: `j/k rows`,
// which walks the reader's mark and is the only key that reaches the row
// `space` unfolds (#300), had gone under the note, while six cells stood
// on an `n/N` that answers `no search — / starts one` to both halves at
// every width (#210, #216, #223) — and those six cells were what the
// mirror key was refused for, on a row fourteen cells short of naming it.
//
// The keys that cannot move pay for a clause that acts
// (`footerClauseTraded`, #331). So wherever a refusing `n/N` still
// stands, its cells buy nothing: the same stand drawn with the walk key
// given up names no key that acts which this row does not — the widest
// row of the stand says which keys those are. Both sides are walked: the
// rows that keep the key and the rows that have paid it over.
func TestAStuckKeyKeepsNoCellsARowHasAlreadyPaid(t *testing.T) {
	forceASCII(t)
	stand := func(sc scene, w, h int, route []string) *Model {
		m := sceneModel(sc, w, h)
		if !m.sessionView() {
			pressKey(m, "tab")
			poll(m, sc)
		}
		for _, k := range route {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}
	// The stands the walk key is measured on: the session view as the
	// deck opens it and the reader one press deeper, with the presses
	// that put a note on the row and move the shed with it.
	routes := [][]string{nil, {"ctrl+u"}, {"ctrl+u", "ctrl+u"}, {"]"}, {"j"},
		{"tab"}, {"tab", "]"}, {"tab", "j"}, {"tab", "[", "]"}, {"tab", "tab"}}
	stood, paid := 0, 0
	for _, sc := range allScenes() {
		for _, route := range routes {
			wide := stand(sc, 220, 48, route)
			if !wide.sessionView() {
				continue
			}
			var acting []string
			for _, k := range r328Keys(ansi.Strip(wide.View())) {
				if r328Acting(k) && k != "n/N" {
					acting = append(acting, k)
				}
			}
			for _, size := range [][2]int{{100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := size[0], size[1]
				m := stand(sc, w, h, route)
				if !m.sessionView() || m.level < levelWaypoints {
					continue // the narrow deck keeps its list
				}
				walk := m.walkKeyStuck(m.keymap())
				if walk == "" || len(m.stuckKeys(m.keymap())) > 1 {
					// The key acts here, the note is its own, or the row
					// refuses more than one key — where two refusals hold
					// cells between them what a clause is refused for is
					// both their business, and this pin measures the walk
					// key's own six cells.
					continue
				}
				frame := ansi.Strip(m.View())
				rows := strings.Split(frame, "\n")
				where := sc.name + " " + itoaPin(w) + "x" + itoaPin(h) + " Lv" + itoaPin(m.level)
				have := map[string]bool{}
				for _, k := range r328Keys(frame) {
					have[k] = true
				}
				if !have["n/N"] {
					paid++ // the row gave the refusing key up for one that acts
					continue
				}
				stood++
				// The cells it holds buy no key that acts: the row drawn
				// with the walk key given up names nothing this one does
				// not (#331). The door stands where this row stands it —
				// what the archive's twelve cells cost is its own trade's
				// question (#328, #329, #330), not the refusing key's.
				draw := m.footerTraded
				if have["A archive"] {
					draw = m.footerCursorTraded
				}
				freed := map[string]bool{}
				for _, k := range r331Keys(ansi.Strip(draw(r331Gone(m.keymap(), []string{walk}), w-2*edgePad))) {
					freed[k] = true
				}
				for _, k := range acting {
					if !have[k] && freed[k] {
						t.Errorf("%s: a walk key with no search to walk stands on six cells that name %q (#193, #216, #331)\n  foot=%q",
							where, k, strings.TrimSpace(rows[len(rows)-1]))
					}
				}
			}
		}
	}
	// Both sides measured: the rows that keep the refusing key, and the
	// rows that have paid its cells over to a key that acts.
	if stood < 20 {
		t.Errorf("only %d rows stood a walk key with no search to walk; the rule is unmeasured", stood)
	}
	if paid < 10 {
		t.Errorf("only %d rows gave the refusing walk key up; the other side is unmeasured", paid)
	}
	t.Logf("rows standing a refusing walk key: %d · rows that paid its cells over: %d", stood, paid)
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
	sweep(t)
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
		if prof.p == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
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
		if prof.p == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
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

// ---- round 106, second-day ----
// TestThePresentNoteKeepsTheWayDeeper holds the rule of #300: the drawn
// form of `at the present · k goes back` gives up its way-back clause —
// which the row it stands on already names, first of all its clauses, as
// `j/k rows` — wherever the clause would cost the row a key naming a
// level that the row named one keypress earlier, and only there.
func TestThePresentNoteKeepsTheWayDeeper(t *testing.T) {
	sweep(t)
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
		if prof == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
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

// ---- round 109, two-tools ----
// TestTheTrailsMoveKeyIsNamedForWhatItStandsOn pins the trail cursor's
// movement pair, at Lv2, to the one thing it does.
//
// At Lv2 the keymap has two forms of one row: the session view (the board
// fits, 120 and up) opened `j/k legs`, the fleet-and-trail layout of the
// very same level (80 and 100) `j/k rows`. One key, one act, one level,
// two names — the shape #307 folded in the reader a round ago and #220
// settled one level out.
//
// `legs` is the false one. The cursor steps `TrailRows`, whose rows come
// in four kinds — `prompt`, `leg`, `waypoint` and `branch` — and whose
// prompt row carries `Leg: -1` because, in the trail's own words, it "is a
// boundary rather than a span of work". SPEC §2.1 keeps `◉` out of the
// legs the same way ("journey start — the user's prompt, quoted"), and
// §2.2 builds legs out of activity. The mark stands on that row on the
// canonical walk at every width.
//
// Three sides, so the word cannot simply be swapped and the row left
// worse:
//   - no Lv2 row says `j/k legs`, at any width, in either layout;
//   - the word is earned: the mark stands on rows that are not legs, and
//     the row that draws it says `j/k rows`;
//   - it costs nothing: the same stand drawn with the old word names no
//     key the drawn row lacks and no key more, and no row runs past its
//     terminal.
func TestTheTrailsMoveKeyIsNamedForWhatItStandsOn(t *testing.T) {
	forceASCII(t)
	stands, split, notLeg := 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range r109ttLegSizes {
				w, h := size[0], size[1]
				inner := w - 2
				for _, route := range r109ttLegRoutes {
					m := sceneModel(sc, w, h)
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					if m.showHelp || m.searching || m.replying || m.level != levelWaypoints {
						continue
					}
					rows := strings.Split(ansi.Strip(m.View()), "\n")
					if len(rows) == 0 {
						continue
					}
					foot := rows[len(rows)-1]
					where := fmt.Sprintf("%s %v %dx%d", sc.name, route, w, h)
					stands++
					if m.sessionView() {
						split++
					}

					// One name for one act, in either layout of Lv2.
					if strings.Contains(foot, "j/k legs") {
						t.Errorf("%s: the trail's row calls the cursor pair %q; it steps TrailRows, and a prompt row is no leg\n  foot=%q",
							where, "j/k legs", strings.TrimRight(foot, " "))
					}
					for _, r := range rows {
						if lipgloss.Width(r) > w {
							t.Errorf("%s: a row runs past the terminal (%d of %d): %q", where, lipgloss.Width(r), w, r)
						}
					}

					// Earned: the stand the row is drawn over is often
					// not a leg at all.
					if kind := r109ttLegKind(m); kind != "" && kind != "leg" {
						notLeg++
						if !strings.Contains(foot, "j/k rows") && r109ttLegNames(foot) {
							t.Errorf("%s: the mark stands on a %s row and the row names neither movement key\n  foot=%q",
								where, kind, strings.TrimRight(foot, " "))
						}
					}

					// Costs nothing: the two words are the same width, so
					// the finished row must name exactly what it named
					// under the old one (#281, #284).
					whole := m.keymap()
					// Through the archive door's own trade as well
					// (#329), the outermost of the chain: the door is
					// the last clause to take spare room, so a row
					// measured without that trade keeps a door the drawn
					// row has given up, and the two rows are no longer
					// the same row under two words. The cursor clause's
					// trade under it reads the row's head, and this row
					// does not lead with `j/k rows`, so it stays out of
					// the way (#300).
					if longer := strings.Replace(whole, "j/k rows · ", "j/k legs · ", 1); longer != whole && m.sessionView() {
						was := strings.Replace(m.footerTraded(longer, inner), "j/k legs · ", "j/k rows · ", 1)
						if !footerNamesAll(was, foot) {
							t.Errorf("%s: the new word cost the row a key\n  now=%q\n  was=%q", where, r109ttLegKeys(foot), r109ttLegKeys(was))
						}
						if !footerNamesAll(foot, was) {
							t.Errorf("%s: the new word bought the row a key it had not earned\n  now=%q\n  was=%q", where, r109ttLegKeys(foot), r109ttLegKeys(was))
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if stands < 20 {
		t.Errorf("only %d Lv2 stands walked; the renamed side is unmeasured", stands)
	}
	if split < 20 {
		t.Errorf("only %d Lv2 stands in the session view; the layout the rename touches is unmeasured", split)
	}
	if notLeg < 20 {
		t.Errorf("only %d Lv2 stands whose marked row is not a leg; the word's own refutation is unmeasured", notLeg)
	}
	t.Logf("Lv2 stands: %d · session view: %d · marked row not a leg: %d", stands, split, notLeg)
}

var r109ttLegSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r109ttLegRoutes are ways onto a trail at Lv2: straight in, in on another
// row, and the cursor and chapter keys pressed once there, so the mark
// stands on legs and on rows that are not legs.
var r109ttLegRoutes = [][]string{
	{"tab"},
	{"tab", "ctrl+u"},
	{"tab", "j"},
	{"2", "tab", "ctrl+u"},
}

// r109ttLegKind is the kind of trail row the cursor stands on — the
// trail's own word for it, not the footer's.
func r109ttLegKind(m *Model) string {
	rows := TrailRows(m.trail, m.level)
	if m.cursor < 0 || m.cursor >= len(rows) {
		return ""
	}
	return rows[m.cursor].Kind
}

// r109ttLegNames reports whether the row names a movement pair at all: a
// row too narrow to name one is #39's shed, not this word's business.
func r109ttLegNames(foot string) bool {
	return strings.Contains(foot, "j/k rows") || strings.Contains(foot, "j/k legs")
}

// r109ttLegKeys is the keymap half of a footer, for a message.
func r109ttLegKeys(foot string) string {
	s := strings.TrimSpace(ansi.Strip(foot))
	if i := strings.Index(s, "  "); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// ---- round 327, every scene ----

// r327PastSizes are the five terminals the walkthrough is drawn at.
var r327PastSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r327PastDoor matches the archive's door as a drawn row wears it — "41
// archived · A browses", "0 of 300 archived · A", "41 archived · 1 hidden
// · A" — the count and the key that browses it (#169, #176).
var r327PastDoor = regexp.MustCompile(`archived(?: · [^·\n]+)? · A(?: browses)?`)

// r327PastBand is the band's own first row on this frame, or "" where the
// frame drew none: the header is the only row that says what the band is.
func r327PastBand(rows []string) string {
	for _, r := range rows {
		if strings.Contains(r, "recent ·") {
			return strings.TrimSpace(r)
		}
	}
	return ""
}

// r327PastNamesTheDoor says whether anything on the frame — one of its rows
// or the footer's own clause — names the archive and the key that opens it.
func r327PastNamesTheDoor(frame string) bool {
	return strings.Contains(frame, "· A archive") || r327PastDoor.MatchString(frame)
}

// TestTheSessionViewLeavesThePastToTheLevelAbove pins #327. The recent band
// is the past where the past has nowhere else to show: a fleet of one opens
// straight into the session, has no board behind it, and #47 put the band
// there for exactly that. With a fleet the deck draws a level up — the
// board with its stranded rows (#147), the narrow list with its group —
// the past is already on that level, and the session view drew it a second
// time one Tab in for no reason anyone asked for. It now leaves it there:
// no `recent ·` row on the frame, no digit opening a row the frame does not
// wear (#255), and the footer naming the archive's door where no row does
// (#203). The fleet of one keeps every row it had.
func TestTheSessionViewLeavesThePastToTheLevelAbove(t *testing.T) {
	sweep(t)
	forceASCII(t)
	stands, dropped, kept := 0, 0, 0
	refused, named := 0, 0 // #328's digits: refused in words, and the ones that name the way
	for _, sc := range allScenes() {
		for _, size := range r327PastSizes {
			w, h := size[0], size[1]
			m := sceneModel(sc, w, h)
			if !m.sessionView() {
				// A fleet of one is already one level in: the deck opened
				// the session for it (#47). Everything else takes a Tab.
				pressKey(m, "tab")
				poll(m, sc)
			}
			if !m.sessionView() || m.level != levelWaypoints {
				continue // the narrow deck keeps its list, and its own band with it
			}
			stands++
			where := sc.name + " " + itoaPin(w) + "x" + itoaPin(h)
			frame := ansi.Strip(m.View())
			rows := strings.Split(frame, "\n")
			band := r327PastBand(rows)
			foot := strings.TrimSpace(rows[len(rows)-1])
			if m.liveCount() > 1 {
				if band != "" {
					t.Errorf("%s: the session view drew the band the level above draws: %q", where, band)
				}
				if len(m.drawnBand) > 0 {
					t.Errorf("%s: no band drawn and %d digits still open one (#255)", where, len(m.drawnBand))
				}
				if m.archivedCount() > 0 {
					dropped++
					if !r327PastNamesTheDoor(frame) && r329DoorRoom(m, rows[len(rows)-1], w) {
						// #328 amends #203: the door is named where the
						// row can afford it and never at an acting key's
						// cost, so a full row goes without it. It is a
						// failure only where the cells were there — and
						// after #329 the cells are what the keys that
						// take only spare room leave, not the blank ones
						// on the row (`r329DoorRoom`).
						t.Errorf("%s: the band is gone, the row had room and nothing names the archive's door (#203, #328)\n  foot=%q", where, foot)
					}
				}
				// The digit the band carried one keypress ago: pressed
				// here it must refuse in words (#43), and where the
				// board one `esc` up wears that very row it must say so
				// and say the way (#328) — nothing else on the frame
				// tells a person whether `5` still lands, and twenty
				// columns narrower the same keypress opens it.
				if digit := m.liveCount() + 1; digit <= 9 {
					body, lvl := strings.Join(rows[:len(rows)-1], "\n"), m.level
					pressKey(m, itoaPin(digit))
					poll(m, sc)
					after := strings.Split(ansi.Strip(m.View()), "\n")
					refused++
					switch {
					case m.note == "":
						t.Errorf("%s: `%d` past the live fleet answered nothing (#43)", where, digit)
					case m.note == "no session "+itoaPin(digit):
					case strings.HasSuffix(m.note, " is on the board · esc, then "+itoaPin(digit)):
						named++
						// The way the note names is a way: one `esc` up,
						// the same digit opens that row in the archive.
						want := strings.TrimSuffix(strings.TrimPrefix(m.note, itoaPin(digit)+" "),
							" is on the board · esc, then "+itoaPin(digit))
						pressKey(m, "esc")
						poll(m, sc)
						m.View() // the board records the band it draws (#255)
						pressKey(m, itoaPin(digit))
						poll(m, sc)
						s, ok := m.selected()
						if !m.archiveView || !ok || sessionName(s.Info) != want {
							t.Errorf("%s: the refusal sent `esc, then %d` to %q and the board refused it: archive=%v note=%q",
								where, digit, want, m.archiveView, m.note)
						}
						continue // this stand has left its frame behind
					default:
						t.Errorf("%s: `%d` answered %q, which is neither refusal (#43, #328)", where, digit, m.note)
					}
					if m.level != lvl {
						t.Errorf("%s: the refused digit moved the deck to Lv%d (#255)", where, m.level)
					}
					if got := strings.Join(after[:len(after)-1], "\n"); got != body {
						t.Errorf("%s: the refused digit moved the frame above the footer (#255)", where)
					}
					if !strings.Contains(strings.TrimSpace(after[len(after)-1]), m.note) {
						t.Errorf("%s: the refusal is not on the footer: note=%q foot=%q",
							where, m.note, strings.TrimSpace(after[len(after)-1]))
					}
				}
				continue
			}
			// The fleet of one: the band is the only place its past shows,
			// and it is drawn as it was before (#47, #98, #102).
			if m.archivedCount() == 0 {
				continue // the first five minutes: nothing has ended yet
			}
			kept++
			if band == "" {
				t.Errorf("%s: the fleet of one lost the band that is its only past (#47)\n%s", where, frame)
			}
			if len(m.drawnBand) == 0 {
				t.Errorf("%s: the fleet of one drew a band no digit opens (#255)", where)
			}
		}
	}
	// #329: the same digit one press deeper. #327 took the band off 72
	// reader frames as well as the 66 at Lv2, `openRecent` finds no drawn
	// row there either, and the refusal stopped at Lv2 — so `5` in the
	// reader said `no session 5` on a deck whose board wears `5 ○ api`.
	// The note names the way that works from where it is said: `esc` is
	// one level out at both, so from the reader the board is two of them.
	deepRefused, deepNamed := 0, 0
	for _, sc := range allScenes() {
		for _, size := range r327PastSizes {
			w, h := size[0], size[1]
			m := sceneModel(sc, w, h)
			if !m.sessionView() {
				pressKey(m, "tab")
				poll(m, sc)
			}
			if !m.sessionView() || m.level != levelWaypoints || m.liveCount() <= 1 {
				continue
			}
			pressKey(m, "tab")
			poll(m, sc)
			if m.level < levelReader {
				continue
			}
			digit := m.liveCount() + 1
			if digit > 9 {
				continue
			}
			where := sc.name + " " + itoaPin(w) + "x" + itoaPin(h) + " Lv3"
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			body, lvl := strings.Join(rows[:len(rows)-1], "\n"), m.level
			pressKey(m, itoaPin(digit))
			poll(m, sc)
			after := strings.Split(ansi.Strip(m.View()), "\n")
			deepRefused++
			walked := false
			switch {
			case m.note == "":
				t.Errorf("%s: `%d` past the live fleet answered nothing (#43)", where, digit)
			case m.note == "no session "+itoaPin(digit):
			case strings.HasSuffix(m.note, " is on the board · esc, esc, then "+itoaPin(digit)):
				deepNamed++
				// The way the note names is a way: two `esc` out, the
				// same digit opens that row in the archive.
				want := strings.TrimSuffix(strings.TrimPrefix(m.note, itoaPin(digit)+" "),
					" is on the board · esc, esc, then "+itoaPin(digit))
				pressKey(m, "esc")
				poll(m, sc)
				pressKey(m, "esc")
				poll(m, sc)
				m.View() // the board records the band it draws (#255)
				pressKey(m, itoaPin(digit))
				poll(m, sc)
				sel, ok := m.selected()
				if !m.archiveView || !ok || sessionName(sel.Info) != want {
					t.Errorf("%s: the refusal sent `esc, esc, then %d` to %q and the board refused it: archive=%v note=%q",
						where, digit, want, m.archiveView, m.note)
				}
				walked = true
			case strings.Contains(m.note, "esc, then "+itoaPin(digit)):
				t.Errorf("%s: the reader's refusal names Lv2's way out: %q", where, m.note)
			default:
				t.Errorf("%s: `%d` answered %q, which is neither refusal (#43, #328, #329)", where, digit, m.note)
			}
			if walked {
				continue // this stand has left its frame behind
			}
			if m.level != lvl {
				t.Errorf("%s: the refused digit moved the deck to Lv%d (#255)", where, m.level)
			}
			if got := strings.Join(after[:len(after)-1], "\n"); got != body {
				t.Errorf("%s: the refused digit moved the frame above the footer (#255)", where)
			}
			if !strings.Contains(strings.TrimSpace(after[len(after)-1]), m.note) {
				t.Errorf("%s: the refusal is not on the footer: note=%q foot=%q",
					where, m.note, strings.TrimSpace(after[len(after)-1]))
			}
		}
	}

	// Not vacuous: the stands, and both sides of the rule measured on them.
	if stands < 20 {
		t.Errorf("only %d session-view stands walked; the rule is unmeasured", stands)
	}
	if dropped < 6 {
		t.Errorf("only %d stands where a fleet had a past to drop; the new rule is unmeasured", dropped)
	}
	if kept < 3 {
		t.Errorf("only %d fleet-of-one stands with an archive; #47 is unmeasured", kept)
	}
	// #328: the digit past the live fleet answers in words on every stand
	// where the band is gone, and on the stands whose board strands the
	// band it names the row and the way.
	if refused < 12 {
		t.Errorf("only %d digits pressed past the live fleet; the refusal is unmeasured", refused)
	}
	// #329's own floor: the reader answers the digit too, and on the
	// stands whose board strands the band it names the row and the longer
	// way.
	if deepRefused < 8 {
		t.Errorf("only %d digits pressed past the live fleet in the reader; #329's half is unmeasured", deepRefused)
	}
	if deepNamed < 2 {
		t.Errorf("only %d reader refusals named the row and the way; #329's note is unmeasured", deepNamed)
	}
	if named < 3 {
		t.Errorf("only %d refusals named the row and the way; #328's note is unmeasured", named)
	}
	t.Logf("session-view stands: %d · fleets that left the past above: %d · fleets of one keeping it: %d · digits refused: %d (naming the way: %d) · in the reader: %d (naming the way: %d)",
		stands, dropped, kept, refused, named, deepRefused, deepNamed)
}

// ---- round 328, the second-day and two-tools operators ----

// r328DoorCells is what the archive's door costs a footer: " · A archive".
const r328DoorCells = 12

// r328DoorRoom says whether a drawn footer had the twelve cells the door
// wants and named the archive nowhere all the same. The row is measured as
// drawn, its left pad included, against the field the footer is given: the
// terminal less its right pad.
func r328DoorRoom(foot string, w int) bool {
	return lipgloss.Width(strings.TrimRight(foot, " "))+r328DoorCells <= w-1
}

// r329DoorRoom is the room the door is owed after #329: the twelve cells,
// and the row that takes them naming every key this row names. The door is
// the last clause to take spare room — it goes after `x hide`, `/ search`,
// `n/N`, `g grab`, the mirror key and the reader's cursor key, which
// withdraw by their own trade rather than at a rank — so blank cells alone
// no longer say the door was owed: a row can stand twelve cells short of
// its width because the clause that would fill them costs a key, and then
// the door would cost the same key. The row is drawn again with the door
// held on (`footerMirrorTraded`, under the door's own trade) and owed only
// where that row gives nothing up for it.
func r329DoorRoom(m *Model, foot string, w int) bool {
	if !r328DoorRoom(foot, w) {
		return false // the cells are not there at all
	}
	keys := m.keymap()
	if !strings.Contains(keys, " · A archive") {
		return false // this frame names the archive on a row, not the footer
	}
	return footerNamesAll(foot, ansi.Strip(m.footerMirrorTraded(keys, w-2*edgePad)))
}

// r328Acting is whether a clause on the widest row is a key that acts —
// what the door must never cost. Read past:
//   - the door itself, and `ctrl+d/u half page`, the one key it outranks:
//     a shortcut for a distance `j` covers, which the help teaches (#42,
//     #51), and the key the panel calls the right payment at 152;
//   - `enter · no pane`, a refusal and not a key (#52), in both halves the
//     split leaves;
//   - `? help` and `q quit`, which no row ever sheds.
//
// #328 read past the clauses a row takes only where they cost it no key
// as well — `/ search` and `g grab` on the session view, the hide key,
// the reader's mirror key and its cursor key, and `n/N`, which rides with
// the search it walks (#281, #284, #289, #293, #300) — on the reasoning
// that they withdraw by their own trade rather than at a rank, and that
// trade is not the door's rank. #329 closed that gap: the door is no
// longer paid for at a rank at all, it is the last clause to take spare
// room, so those keys take theirs first and the door must never cost one
// of them either. They are inside the checked set now, and where the door
// stands and one of them is off the row the pin asks the one question
// that is left — whether giving the door up would put it back.
func r328Acting(k string) bool {
	switch k {
	case "A archive", "ctrl+d/u half page", "enter", "no pane", "? help", "q quit":
		return false
	}
	return true
}

// r329DoorBought is the keys off this row that the row would name if the
// door were given up: what the door is costing, measured the way #329
// measures it and #330 corrects it. A key the row cannot name either way
// is not the door's doing — the width is.
//
// #330: the counterfactual is the row drawn without the door *and*
// without `ctrl+d/u half page` wherever the drawn row has already shed
// it. The door outranks the page key and nothing else (#42, #51, #328),
// so the twelve cells it would vacate are the acting keys' to take;
// drawn with the page key free to walk back into them it took them,
// evicted a key that acts, and the trade read as a swap — which is the
// one shape #281's measure refuses — so the door went on standing on
// five rows at 152 that an acting key was off.
func r329DoorBought(m *Model, foot string, w int, lost []string) []string {
	bare := strings.Replace(m.keymap(), " · A archive", "", 1)
	if drawn := ansi.Strip(foot); pageKeyGone(drawn) == drawn {
		bare = pageKeyGone(bare) // the page key does not profit from the door's cells (#330)
	}
	without := ansi.Strip(m.footerTraded(bare, w-2*edgePad))
	shut := strings.Replace(ansi.Strip(foot), " · A archive", "", 1)
	if !footerNamesMore(shut, without) {
		// #331: and the counterfactual names the room-only keys that fit
		// once the door is gone. A clause that acts is not refused by a
		// key that cannot move from where the row stands
		// (`footerClauseTraded`), so the row without the door is drawn
		// with those keys given up as well: measured with a
		// `[ ] chapters` that refuses at every width still holding its
		// fifteen cells, `few-ongoing`'s Lv2 row at 152 under `at the
		// start` gave `/ search` up for it, the counterfactual named one
		// acting key fewer than the row standing the door, and the trade
		// read a swap and kept the door.
		without = ansi.Strip(m.footerTraded(r331Gone(bare, m.stuckKeys(m.keymap())), w-2*edgePad))
	}
	if !footerNamesMore(shut, without) {
		// A clause is given up for a key and never for a swap (#281,
		// #329): where the row without the door gives a key up of its
		// own, the door is not what the missing key went to.
		return nil
	}
	var bought []string
	for _, k := range lost {
		for _, c := range r328Keys(without) {
			if c == k {
				bought = append(bought, k)
				break
			}
		}
	}
	return bought
}

// r330PageHeld says whether this row gives the door up only under #330's
// measure: the row that stands the door has already shed `ctrl+d/u half
// page` at the one rank the door outranks, and the door goes when the
// counterfactual is drawn with the page key held off — where the same
// counterfactual drawn with the page key free would have kept the door,
// because the page key walked into the door's twelve cells and put an
// acting key off the row in its place. These are the rows the second-day
// operator counted: "The page key still decides the door, one step in."
func r330PageHeld(m *Model, w int) bool {
	keys := m.keymap()
	if !strings.Contains(keys, " · A archive") {
		return false // this frame names the archive on a row, not the footer
	}
	inner := w - 2*edgePad
	with := ansi.Strip(m.footerCursorTraded(keys, inner))
	if pageKeyGone(with) != with {
		return false // the row standing the door still names the page key
	}
	bare := strings.Replace(keys, " · A archive", "", 1)
	free := ansi.Strip(m.footerCursorTraded(bare, inner))
	held := ansi.Strip(m.footerCursorTraded(pageKeyGone(bare), inner))
	shut := doorGone(pageKeyGone(with))
	return footerNamesMore(shut, pageKeyGone(held)) && !footerNamesMore(shut, pageKeyGone(free))
}

// TestTheArchiveDoorYieldsToEveryKeyThatActs pins #328. #327 stopped
// drawing the recent band in the session view of a fleet the deck draws a
// level up, and the footer names the archive's door in its place (#203) —
// at the rank the door had had since #56 and #62, last of the level's own
// keys. Two operators read the frames: "`a ask` and `enter attach` both
// gone. `enter` is the three-keypress proof's own key (§3). Trading it for
// a door to a place one `esc` reaches is the wrong end of the shed order."
// And: "Both rows are 119 columns; the new one parks 11 blank columns
// between `q quit` and the note. Shedding `enter attach` alone (15) pays
// for `· A archive` (12) with 5 to spare."
//
// The door now sheds second, behind `ctrl+d/u half page` and before every
// key that acts. Both sides, over every scene at four sizes, at Lv2 and at
// Lv3: where the door stands, the row names every acting key the widest
// row names — the door never stands where an acting key was shed — and
// where the door is gone, the row was full to within its twelve cells, so
// it was not shed for nothing.
//
// #329 put the keys that take only spare room inside the checked set
// (`r328Acting`): the door is not paid for at a rank any more, it is the
// last clause to take spare room, so `x hide`, `/ search`, `n/N`,
// `g grab`, the mirror key and the reader's cursor key take theirs first.
// Where the door stands and one of them is off the row the question is
// not the rank but the trade — whether the row would name it if the door
// went — and the row that is full is measured with the door held on
// (`r329DoorRoom`), since twelve blank cells no longer mean the door was
// owed them.
//
// #330 fixed the counterfactual the trade is read against. "The page key
// still decides the door, one step in. The counterfactual row is drawn
// with `ctrl+d/u half page` re-entering the twelve cells the door
// vacates, so it evicts an acting key and the trade reads as a swap."
// The row without the door is drawn with the page key held off wherever
// the drawn row has already shed it (`r329DoorBought`, `r330PageHeld`),
// so the door's cells go to the keys that act and the comparison sees
// what the door really costs.
func TestTheArchiveDoorYieldsToEveryKeyThatActs(t *testing.T) {
	forceASCII(t)
	stand := func(sc scene, w, h int, route []string) *Model {
		m := sceneModel(sc, w, h)
		if !m.sessionView() {
			pressKey(m, "tab")
			poll(m, sc)
		}
		for _, k := range route {
			pressKey(m, k)
			poll(m, sc)
		}
		return m
	}
	// The stands the door is measured on: the session view as the deck
	// opens it, one press deeper in the reader, and the presses that put a
	// note on the row and move the shed with it (#329: the Lv2 rows at 152
	// the operators quoted are one `ctrl+u` and one `]` in).
	// #331 adds the second `ctrl+u`, the press the second-day operator
	// quoted: it moves nothing, so the row carries `at the start` and the
	// note's reserve is what the keys are shed against.
	routes := [][]string{nil, {"ctrl+u"}, {"ctrl+u", "ctrl+u"}, {"]"}, {"j"}, {"tab"}, {"tab", "]"}, {"tab", "j"}}
	stood, went, roomOnly, deep152 := 0, 0, 0, 0 // #329: and the rows a shed door bought a key back for
	pageHeld := 0                                // #330: the rows the door goes on only once the page key is held off
	noted := 0                                   // #331: the stands the door is decided on with a note on the row
	for _, sc := range allScenes() {
		for _, route := range routes {
			wide := stand(sc, 220, 48, route)
			if !wide.sessionView() || wide.liveCount() <= 1 || wide.archivedCount() == 0 {
				continue
			}
			var acting []string
			for _, k := range r328Keys(ansi.Strip(wide.View())) {
				if r328Acting(k) {
					acting = append(acting, k)
				}
			}
			for _, size := range [][2]int{{100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				w, h := size[0], size[1]
				m := stand(sc, w, h, route)
				if !m.sessionView() || m.level < levelWaypoints {
					continue // the narrow deck keeps its list, and its own band
				}
				frame := ansi.Strip(m.View())
				rows := strings.Split(frame, "\n")
				where := sc.name + " " + itoaPin(w) + "x" + itoaPin(h) + " Lv" + itoaPin(m.level)
				have := map[string]bool{}
				for _, k := range r328Keys(frame) {
					have[k] = true
				}
				var lost []string
				for _, k := range acting {
					if !have[k] {
						lost = append(lost, k)
					}
				}
				if have["A archive"] {
					stood++
					// The door stands only where giving it up buys
					// nothing (#329): a key the widest row names and
					// this one does not is the door's cost where the
					// row without the door names it, and the width's
					// where it does not.
					if bought := r329DoorBought(m, rows[len(rows)-1], w, lost); len(bought) > 0 {
						t.Errorf("%s: the archive's door stands and the row shed %v, which the row without it names (#328, #329, #330)\n  foot=%q",
							where, bought, strings.TrimSpace(rows[len(rows)-1]))
					}
					continue
				}
				went++
				// The other side of the same trade (#329): a door that
				// went and a key that stands where it stood — the row
				// drawn with the door held on does not name it.
				if !footerNamesAll(rows[len(rows)-1], ansi.Strip(m.footerMirrorTraded(m.keymap(), w-2*edgePad))) {
					roomOnly++
					if m.level == levelWaypoints && w == 152 {
						deep152++
					}
				}
				if w == 152 && r330PageHeld(m, w) {
					pageHeld++
				}
				if m.note != "" {
					noted++ // #331: measured with the note on the row
				}
				if len(lost) > 0 && r329DoorRoom(m, rows[len(rows)-1], w) {
					t.Errorf("%s: the door is gone, %v are gone and the row had room for it (#203, #328, #329)\n  foot=%q",
						where, lost, strings.TrimSpace(rows[len(rows)-1]))
				}
			}
		}
	}
	// Both sides measured: rows that afford the door, and rows that do not.
	if stood < 6 {
		t.Errorf("only %d rows stood the door; the rule is unmeasured", stood)
	}
	if went < 4 {
		t.Errorf("only %d rows gave it up; the other side is unmeasured", went)
	}
	// #329's own side: the rows where the door went and a key that takes
	// only spare room stands where it stood, and the Lv2 rows at 152 the
	// two-tools operator counted among them.
	if roomOnly < 8 {
		t.Errorf("only %d rows traded the door for a key that takes spare room; #329 is unmeasured", roomOnly)
	}
	if deep152 < 3 {
		t.Errorf("only %d Lv2 rows at 152 took a key back from the door; the width the operators quoted is unmeasured", deep152)
	}
	// #330's own side: the rows at 152 where the door goes only once the
	// page key is held off the counterfactual — the five the second-day
	// operator named.
	if pageHeld < 5 {
		t.Errorf("only %d rows at 152 gave the door up with the page key held off; #330 is unmeasured", pageHeld)
	}
	// #331's own side: the door is decided on rows a note's reserve has
	// already shed against, where a key that cannot move holds cells a
	// key that acts would take.
	if noted < 20 {
		t.Errorf("only %d rows carried a note while the door was decided; #331 is unmeasured", noted)
	}
	t.Logf("session-view rows standing the door: %d · rows that could not afford it: %d · rows that took a key back from it: %d (Lv2 at 152: %d) · rows at 152 the page key was deciding: %d · rows decided under a note: %d",
		stood, went, roomOnly, deep152, pageHeld, noted)
}

// r328Keys is the footer's key clauses as the frame draws them: the row's
// left half, up to the two blank columns that hold the note off, with the
// attach aside — which sheds before any key does (#56) — off the attach
// key's own clause.
func r328Keys(frame string) []string {
	rows := strings.Split(frame, "\n")
	foot := strings.TrimRight(rows[len(rows)-1], " ")
	if i := strings.Index(foot, "  "); i > 0 {
		foot = foot[:i]
	}
	var out []string
	for _, k := range strings.Split(strings.TrimSpace(foot), " · ") {
		if k != "" {
			out = append(out, strings.TrimSuffix(k, " (prefix d returns)"))
		}
	}
	return out
}
