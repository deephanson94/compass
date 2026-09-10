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
		if foot := footer(m); !strings.Contains(foot, "j/k rows") {
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
	sweep(t)
	forceASCII(t)
	stands := [][]string{nil, {"x"}, {"tab"}}
	off, bandDigits := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		if prof == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
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
	sweep(t)
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
		if prof.p == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
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
		if prof.p == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
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
	sweep(t)
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
		if prof == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
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
				for _, k := range []string{"j/k rows", "m live pane", "r reply", "a ask", "tab reader", "? help", "q quit"} {
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
	sweep(t)
	forceASCII(t)
	stands, roomy, named, readers := 0, 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		if prof == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
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
	if roomy < 12 { // half of the two-profile floor: the sweep walks one profile (#324)
		t.Errorf("only %d session-view stands where the grab acts on a row shed of nothing; the biting side is unmeasured", roomy)
	}
	t.Logf("session-view stands: %d · rows shed of nothing where `g` grabs: %d · naming `g grab`: %d · reader stands: %d", stands, roomy, named, readers)
}
