package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

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
//
// Amended by #328, which moved the stand at 100 and 120: "`a ask` and
// `enter attach` both gone. `enter` is the three-keypress proof's own key
// (§3). Trading it for a door to a place one `esc` reaches is the wrong
// end of the shed order." The door now yields to every key that acts, so
// the rule reads: named once where the row can afford it, and never at an
// acting key's cost. Where the row cannot take the door's twelve cells it
// goes unnamed on the frame — `? help` still lists `A` — and the pin
// measures that the cells really were not there.
func TestTheReadersFooterNamesTheArchiveWhereNoRowDoes(t *testing.T) {
	named, afforded := 0, 0
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
			switch inFoot := strings.Contains(foot, "A archive"); {
			case inFoot && onRow:
				t.Errorf("%s %dx%d: the archive door is named twice: %q", c.name, size[0], size[1], strings.TrimSpace(foot))
			case !inFoot && !onRow:
				// The room the door is owed is what is left after the
				// keys that take only spare room have taken theirs
				// (#329): twelve blank cells on the row no longer say
				// the door was owed them.
				if room := r329DoorRoom(m, foot, size[0]); room {
					t.Errorf("%s %dx%d: the row had room for the door and named the archive nowhere: %q",
						c.name, size[0], size[1], strings.TrimSpace(foot))
				}
				afforded++
			default:
				named++
			}
		}
	}
	// Both sides measured: the widths that afford the door name it, the
	// widths that do not are the ones #328 moved.
	if named < 3 || afforded < 2 {
		t.Errorf("%d stands named the door and %d could not afford it; the rule is unmeasured", named, afforded)
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
//
// Amended by #328: once where the row can afford it, and never at an
// acting key's cost. The 120 stand is the one that moved — "`a ask` and
// `enter attach` both gone. `enter` is the three-keypress proof's own key
// (§3). Trading it for a door to a place one `esc` reaches is the wrong
// end of the shed order" — and where the door goes unnamed the pin
// measures that the row really could not take its twelve cells.
func TestTheSessionViewsFooterNamesTheArchiveWhereNoRowDoes(t *testing.T) {
	forceASCII(t)
	named, afforded := 0, 0
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
				// The room is what the room-only keys leave (#329).
				if r329DoorRoom(m, foot, size[0]) {
					t.Errorf("%s %dx%d: the row had room for the door and named the archive nowhere: %q",
						c.name, size[0], size[1], strings.TrimSpace(foot))
				}
				afforded++
			default:
				named++
			}
		}
	}
	// Both sides measured: the widths that afford the door name it, the
	// widths that do not are the ones #328 moved.
	if named < 3 || afforded < 2 {
		t.Errorf("%d stands named the door and %d could not afford it; the rule is unmeasured", named, afforded)
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
