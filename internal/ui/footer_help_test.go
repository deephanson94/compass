package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

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

// ---- round 118, the second-day operator ----
// The two tests below pin #333. The shed gives up a key
// by taking its fragment out of the row, and every fragment is written
// separator-led (` · / search`); the head forms the order carries were
// added one key at a time, for the attach key (#56), the unfold key
// (#200), the page key (#213) and the movement keys, and no other key
// ever got one. So a key that had come to lead the row — its own head
// form gone above it — matched nothing: the shed marked it given up and
// the row kept it anyway, beside keys that outrank it and are off.
//
// The second-day operator quoted `second-day-80x24` line 522, the frame
// after the second `tab`: `/ search · esc back · A archive · ? help ·
// q quit` under `the deepest level`, 49 of 59 cells, where `[ ] turns`
// outranks `/ search` (#39's ranks) and stands in 50. `/ search` was not
// alone — `r reply`, `[ ] chapters`, `m live pane` and `x hide` head rows
// with no head form too.
//
// The rule: a key is shed wherever it stands — at the head (`key · `),
// mid-row (` · key`) or alone (`key`) — through one helper every removal
// goes through (`clauseGone`). The ranks are untouched; only the matching
// changes.
//
// Both halves are pinned, in two tests. TestTheShedDropsAKeyAtTheHeadOfTheRow
// takes the drop itself, on a row that leads with the key the shed is
// asked for and on a row that is one key. TestNoKeyStandsWhileOneThatOutranksItFits
// takes the rule over every scene at five widths, on every frame of the
// canonical walk: no key stands on a row while a key that outranks it in
// that level's shed order is off the row and would fit in the row's free
// cells and its own. Read past the keys whose place is not the rank's to
// settle — the attach aside, which is not a key (#55); a key that cannot
// move, whose cells the fold spends out of rank (#210, #216, #331); and a
// clause withdrawn at this stand by its own trade (#281's chain) or its
// own yield to a key naming a level (#297).
func TestTheShedDropsAKeyAtTheHeadOfTheRow(t *testing.T) {
	forceASCII(t)
	// A row that leads with the key the shed is asked for gives it up and
	// closes over the separator it led; a row of one key loses it whole.
	narrow := func(n int) func(string) bool {
		return func(k string) bool { return lipgloss.Width(k) <= n }
	}
	if got := shedKeys("x hide · enter attach · esc back", []string{" · x hide"}, narrow(25)); got != "enter attach · esc back" {
		t.Errorf("the head of the row keeps its key: %q", got)
	}
	if got := shedKeys("q quit", []string{" · q quit"}, narrow(0)); got != "" {
		t.Errorf("a row of one key keeps it: %q", got)
	}
	// ---- round 119, the two-tools operator ----
	// The readers that answer whether a key can move read the row the same
	// way (#334). Each was written with one separator-led form and nothing
	// else — `" · [ ] turns"`, `" · n/N"`, `" · x hide"` — so the very key
	// #333 now puts at the head of a row was invisible to them, which is
	// the case the fold beside this one reads. They ask `rowNames` now, as
	// every removal of a clause does.
	sc := sceneSecondDay()
	m := sceneModel(sc, 80, 24)
	for _, k := range canonicalKeys[:13] {
		pressKey(m, k)
		poll(m, sc)
	}
	if m.level < levelReader {
		t.Fatalf("the walk is not in the reader: Lv%d", m.level)
	}
	if m.turnKeysMove() {
		t.Fatalf("the turn keys move from this stand: the reader is not standing on its one turn")
	}
	for _, row := range []string{"[ ] turns · esc back", "j/k rows · [ ] turns · esc back"} {
		if got := m.chapterKeyStuck(row); got != " · [ ] turns" {
			t.Errorf("the chapter reader misses the turn key on %q: %q", row, got)
		}
	}
	if m.query != "" {
		t.Fatalf("a search stands: the walk keys are not the ones that cannot move")
	}
	for _, row := range []string{"n/N · esc back", "j/k rows · n/N · esc back"} {
		if got := m.walkKeyStuck(row); got != " · n/N" {
			t.Errorf("the walk reader misses the pair on %q: %q", row, got)
		}
	}
}

// ---- round 119, the second-day operator ----
// TestTheSearchNoteNamesTheKeyThatMadeIt pins #334: off the canonical walk,
// a search the conversation does not carry.
//
// Three facts of one frame, all of them the row's. The note reports a
// search and the panel's header draws the query beside it, and at eighty
// the footer named no key that makes one — the harm #24 named and #223
// pinned at the walk key, at the key that starts the walk. `esc` there
// clears the query before it goes anywhere (`case "esc"`), and the row
// promised `esc back`: the deck's one clause that was not the answer the
// press gives (#210's device, at the way out). And `n/N` cannot move for
// as long as that query stands — `jumpMatch` answers `no matches` to both
// halves at every width — while the gate called the pair acting the moment
// a search stood (#210, #216, #219, #223, #331).
func TestTheSearchNoteNamesTheKeyThatMadeIt(t *testing.T) {
	forceASCII(t)
	foot := func(m *Model) string {
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		return strings.TrimRight(rows[len(rows)-1], " ")
	}
	for _, mk := range []func() scene{sceneSecondDay, sceneFirstSession} {
		for _, size := range [][2]int{{80, 24}, {100, 30}} {
			sc, w, h := mk(), size[0], size[1]
			m := sceneModel(sc, w, h)
			for _, k := range []string{"tab", "tab"} {
				pressKey(m, k)
				poll(m, sc)
			}
			if m.level < levelReader {
				t.Fatalf("%s %dx%d: two tabs do not reach the reader (Lv%d)", sc.name, w, h, m.level)
			}
			for _, k := range []string{"/", "z", "z", "z", "enter"} {
				pressKey(m, k)
				poll(m, sc)
			}
			where := fmt.Sprintf("%s %dx%d", sc.name, w, h)
			if m.query != "zzz" || m.note != "no matches" {
				t.Fatalf("%s: the search did not miss: query %q, note %q", where, m.query, m.note)
			}
			f := foot(m)
			if !strings.Contains(f, "/ search") {
				t.Errorf("%s: the row reports a search and names no key that makes one: %q", where, f)
			}
			if !strings.Contains(f, "esc clears it") || strings.Contains(f, "esc back") {
				t.Errorf("%s: the way out promises a level and clears a query: %q", where, f)
			}
			// The walk cannot move from here, and says so, whichever half
			// is pressed — so its cells are the fold's to spend once the
			// note it is under has been answered (#24, #57, #210).
			for _, half := range []string{"n", "N"} {
				step := sceneModel(sc, w, h)
				for _, k := range []string{"tab", "tab", "/", "z", "z", "z", "enter", half} {
					pressKey(step, k)
					poll(step, sc)
				}
				if step.note != "no matches" {
					t.Errorf("%s: `%s` moves from this stand: %q", where, half, step.note)
				}
			}
			// One press on, the note answered and the query still standing:
			// the pair is off the row and the key that starts a search is
			// on it.
			pressKey(m, "j")
			poll(m, sc)
			if next := foot(m); strings.Contains(next, "n/N") || !strings.Contains(next, "esc clears it") {
				t.Errorf("%s: a walk that cannot move keeps its cells: %q", where, next)
			}
			// And `esc` does what the row now says: the query goes, the
			// level stays.
			pressKey(m, "esc")
			if m.query != "" || m.level < levelReader {
				t.Errorf("%s: esc left the query %q standing, at Lv%d", where, m.query, m.level)
			}
			// With a query the conversation does carry, the walk acts and
			// the note is the walk's own count (#223).
			hit := sceneModel(sc, w, h)
			for _, k := range []string{"tab", "tab"} {
				pressKey(hit, k)
				poll(hit, sc)
			}
			word := "" // a word the conversation on this page carries
			for _, l := range hit.doc(hit.readerWidth()) {
				for _, f := range strings.Fields(l.text) {
					if len(f) >= 4 && strings.Trim(f, "abcdefghijklmnopqrstuvwxyz") == "" {
						word = f
						break
					}
				}
				if word != "" {
					break
				}
			}
			if word == "" {
				t.Fatalf("%s: the reader's page carries no word to search for", where)
			}
			for _, k := range []string{"/", word, "enter"} {
				pressKey(hit, k)
				poll(hit, sc)
			}
			if hit.query != word {
				t.Fatalf("%s: the search was not entered: %q", where, hit.query)
			}
			pressKey(hit, "n")
			poll(hit, sc)
			if !strings.HasPrefix(hit.note, "match ") {
				t.Errorf("%s: `%s` is on the page and `n` does not walk to it: %q", where, word, hit.note)
			}
			if f := foot(hit); !strings.Contains(f, "n/N") {
				t.Errorf("%s: the walk acts here and the row does not name it: %q", where, f)
			}
		}
	}
}

// TestNoKeyStandsWhileOneThatOutranksItFits is #333's own walk: the rule
// on every footer the canonical walk draws, at five widths, on every
// scene. A panel sweep, so it steps aside under -short (`sweep`).
func TestNoKeyStandsWhileOneThatOutranksItFits(t *testing.T) {
	sweep(t)
	forceASCII(t)
	stands, checked, headless := 0, 0, 0
	stoodStuck, yieldedStuck := 0, 0
	for _, sc := range allScenes() {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			m := sceneModel(sc, w, h)
			for i, k := range append([]string{""}, canonicalKeys...) {
				if i > 0 {
					pressKey(m, k)
					poll(m, sc)
				}
				rows := strings.Split(ansi.Strip(m.View()), "\n")
				foot := rows[len(rows)-1]
				keys, note := r333Split(foot)
				named := footerKeysNamed(keys)
				whole := m.keymap()
				offered := map[string]bool{}
				for _, o := range footerKeysNamed(whole) {
					offered[o] = true
				}
				subset := len(named) > 0
				for _, n := range named {
					if !offered[n] {
						subset = false // the search line, the reply panel, the help
					}
				}
				if !subset {
					continue
				}
				stands++
				if r333Headless(m.footerDrops(m.chapterNote()), named[0]) {
					headless++ // the rows the fold is about: no head form was ever written for the key that leads them
				}
				on := map[string]bool{}
				for _, n := range named {
					on[n] = true
				}
				stuck := map[string]bool{}
				for _, s := range m.stuckKeys(whole) {
					stuck[keyWord(s)] = true
				}
				rank := r333Rank(m.footerDrops(m.chapterNote()))
				// The cells the keymap is drawn in: the row past its
				// edges, less the note and the two columns that hold it
				// off (`fitsWith`, #134's reserve).
				room := w - 2*edgePad
				if note != "" {
					room -= lipgloss.Width(note) + 2
				}
				free := room - lipgloss.Width(keys)
				for _, s := range m.stuckKeys(whole) {
					if on[keyWord(s)] {
						stoodStuck++
					} else {
						yieldedStuck++
					}
				}
				for _, lo := range named {
					lr, ok := rank[lo]
					if !ok {
						continue
					}
					// A key that cannot move is outranked by every key
					// that acts (#210, #216, #331, #332): its cells are
					// the fold's to spend whatever place the order gives
					// it, so it is weighed against every acting key that
					// is off the row and not only the ones above it.
					if stuck[lo] {
						lr = -1
					}
					for hi, hr := range rank {
						if hr <= lr || on[hi] || !offered[hi] || stuck[hi] || m.r333Traded(hi) {
							continue
						}
						checked++
						if lipgloss.Width(" · "+hi) <= free+lipgloss.Width(" · "+lo) {
							why := "which outranks it"
							if stuck[lo] {
								why = "which acts where it cannot move"
							}
							t.Errorf("%s %dx%d Lv%d after %q: %q stands while %q, %s, is off a row with %d free cells (#39, #333, #334)\n  foot=%q",
								sc.name, w, h, m.level, k, lo, hi, why, free, strings.TrimSpace(foot))
						}
					}
				}
			}
		}
	}
	if stands < 1500 {
		t.Errorf("only %d footers were read; the rule is unmeasured", stands)
	}
	// The stand the fold is about: a row led by a key the order spells one
	// way only. Before #333 the shed could not take that key off the row
	// at all, whatever its rank.
	if headless < 200 {
		t.Errorf("only %d rows were led by a key with no head form; #333 is unmeasured", headless)
	}
	// The floor #334 leaves: the keys that cannot move, counted where they
	// stand and where the fold has already spent their cells. Without both
	// the half of the rule they carry is unmeasured.
	if stoodStuck < 100 || yieldedStuck < 100 {
		t.Errorf("keys that cannot move: %d standing, %d yielded — the rule is unmeasured", stoodStuck, yieldedStuck)
	}
	t.Logf("footers read: %d · rows led by a key with no head form: %d · pairs of keys weighed by rank: %d · keys that cannot move: %d standing, %d yielded",
		stands, headless, checked, stoodStuck, yieldedStuck)
}

// r333Split is a drawn footer's keymap and its note, parted at the two
// columns that hold the one off the other (#134's reserve).
func r333Split(foot string) (string, string) {
	s := strings.TrimRight(strings.TrimPrefix(foot, " "), " ")
	if i := strings.Index(s, "  "); i >= 0 {
		return s[:i], strings.TrimSpace(s[i:])
	}
	return s, ""
}

// r333Rank is where a level's shed order stands each key: first to go
// first, so a later place outranks an earlier one. A key's rank is its
// own fragment's, the head forms beside it naming the same key (#333).
func r333Rank(order []string) map[string]int {
	rank := map[string]int{}
	for i, d := range order {
		if d == attachHint {
			continue
		}
		if k := keyWord(d); k != "" {
			if _, seen := rank[k]; !seen {
				rank[k] = i
			}
		}
	}
	return rank
}

// r333Headless says whether the order spells this key one way only — the
// stand #333 is about, where the key leads the row and the separator-led
// fragment is all the shed has to match it by.
func r333Headless(order []string, key string) bool {
	for _, d := range order {
		if keyWord(d) == key && strings.HasSuffix(d, " · ") {
			return false
		}
	}
	return true
}

// r333Traded says whether this key's place on the row is settled by its
// own trade at this stand rather than by its rank — the archive's door,
// the reader's cursor and mirror keys, and the hide, search and grab
// clauses at the levels they are new to (#281, #284, #289, #293, #295,
// #300, #328). An attach that cannot work is a refusal the note may
// already say, and goes the same way (#165, #299).
func (m *Model) r333Traded(k string) bool {
	switch k {
	case "A archive":
		return m.doorYields()
	case "j/k rows":
		return m.level >= levelReader && m.readerPageFits()
	case "m live pane", "m conversation":
		return m.level >= levelReader
	case "g grab":
		return m.level < levelReader && (m.level >= levelWaypoints || m.archiveView)
	case "/ search":
		return m.level >= levelWaypoints && m.level < levelReader
	case "x hide":
		return m.level >= levelWaypoints || m.archiveView
	case "x unhide":
		return m.level >= levelWaypoints
	case "enter · no pane":
		return true
	case "[ ] chapters":
		// The chapter key yields to a key naming a level, and only where
		// the key comes back: its rank on the row is the way in's to
		// settle, not the order's (#175, #187, #297, and
		// `chapterKeyAboveTheWayIn`).
		_, moved := chapterKeyAboveTheWayIn(m.footerDrops(m.chapterNote()))
		return moved && !m.chapterNote()
	}
	return false
}
