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
	sweep(t)
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
	sweep(t)
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
			// `k` is the cursor's key since #300: it answers this end once
			// the cursor has reached it, not the moment the page has.
			for i := 0; i < 4000; i++ {
				pressKey(k, "k")
				poll(k, c.sc)
				if k.note != "" {
					break
				}
			}
			want := k.note
			if want == "" {
				t.Fatalf("%s %dx%d: `k` said nothing at the start", c.name, w, h)
			}
			g := top()
			// `g` is the cursor's key too since #313, and since the fold
			// that followed it answers this end on the same terms `k`
			// above does: the press that carries the mark to the start
			// says so by moving it, and the press after it — the one
			// asked at the end it names — draws the word (#304).
			for i := 0; i < 4000; i++ {
				pressKey(g, "g")
				poll(g, c.sc)
				if strings.Contains(ansi.Strip(g.View()), want) {
					break
				}
			}
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
