package ui

import (
	"strings"
	"testing"

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
