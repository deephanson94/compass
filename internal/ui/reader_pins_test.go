package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/deephanson94/compass/internal/transcript"
	"github.com/muesli/termenv"
)

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
		if foot := footer(stand(sc, size[0], size[1])); !strings.Contains(foot, "j/k rows") {
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
	sweep(t)
	forceASCII(t)
	standing, first, toggles := 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		if prof == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
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
	sweep(t)
	forceASCII(t)
	stands, roomy, named, standing := 0, 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		if prof == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
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
						// The cells the clause could actually have are the
						// footer's own, not the terminal's: the frame pads
						// both edges (#330), and counting those two among
						// them read fourteen free where thirteen were, on
						// the one row that had just given the archive's
						// door up for `n/N` and left the gap standing.
						free := w - 2*edgePad - lipgloss.Width(strings.TrimSpace(foot))
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

// ---- round 107, second-day ----
// TestTheReaderMoveSaysWhatItDid holds the rule this round folds: in the
// reader, `all of it is on screen` (#83) is the answer to a press that moved
// nothing, and it is drawn only there. Since the cursor (#300) `j`, `k`,
// `ctrl+d` and `ctrl+u` step the mark on a page that fits as well as on one
// that does not, and the sentence stood over a press that moved the mark
// exactly as it stood over one that could not — one row for two opposite
// outcomes, so the person could not read from it whether the key had done
// anything (#221). A press that moves the mark says so by moving it and
// draws no note; a press that cannot keeps #83's word.
func TestTheReaderMoveSaysWhatItDid(t *testing.T) {
	sweep(t)
	forceASCII(t)

	r107sdScene := func(name string) scene {
		for _, sc := range allScenes() {
			if sc.name == name {
				return sc
			}
		}
		t.Fatalf("no scene %q", name)
		return scene{}
	}
	r107sdWalk := func(sc scene, w, h int, keys []string) (*Model, []string) {
		m := sceneModel(sc, w, h)
		for _, k := range keys {
			pressKey(m, k)
			poll(m, sc)
		}
		return m, strings.Split(ansi.Strip(m.View()), "\n")
	}
	r107sdFoot := func(rows []string) string { return strings.TrimRight(rows[len(rows)-1], " ") }
	// The rows every cursor mark on the frame stands on. The reader's mark is
	// the ▸ markAnchor cuts into its cursor row (#300), the same cell the
	// trail's cursor spends; a press that steps the reader's cursor is one
	// that puts a mark on a different row. The row's own words, not its
	// place on screen: a step that scrolls the page under the cursor can
	// leave the mark on the same screen line.
	r107sdMarks := func(rows []string) []string {
		var out []string
		for _, r := range rows[:len(rows)-1] {
			if strings.ContainsRune(r, '▸') {
				out = append(out, strings.TrimSpace(r))
			}
		}
		return out
	}
	r107sdSame := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if a[i] != b[i] {
				return false
			}
		}
		return true
	}

	// One: the frame it was found on. The live session's reader at eighty,
	// a page of five rows that fits whole: the first `j` walks the mark off
	// the prompt onto the row beneath it, the second cannot walk it any
	// further. The two presses must not draw the same row.
	sd := r107sdScene("second-day")
	_, opened := r107sdWalk(sd, 80, 24, []string{"tab", "tab"})
	if !strings.Contains(strings.Join(opened, "\n"), "READER · hello") {
		t.Fatalf("second-day 80x24 [tab tab]: not the reader: %q", r107sdFoot(opened))
	}
	_, one := r107sdWalk(sd, 80, 24, []string{"tab", "tab", "j"})
	_, two := r107sdWalk(sd, 80, 24, []string{"tab", "tab", "j", "j"})
	if r107sdSame(r107sdMarks(one), r107sdMarks(opened)) {
		t.Fatalf("second-day 80x24: the first `j` moved the mark nowhere — not the stand this pins")
	}
	if !r107sdSame(r107sdMarks(two), r107sdMarks(one)) {
		t.Fatalf("second-day 80x24: the second `j` moved the mark — not the stand this pins")
	}
	if strings.Contains(r107sdFoot(one), "all of it is on screen") {
		t.Errorf("second-day 80x24 [tab tab j]: `j` moved the mark and the row answers the page's question: %q", r107sdFoot(one))
	}
	// The sentence is the end's own, not the page's. No row draws that end
	// of the document, while the start is drawn over the cursor
	// (readerAbove), so #83's word keeps the end the frame already answers
	// and this one takes the reader's own — the word `j` gets on every page
	// that scrolls (#24, #228, #309). Part three below holds #83's word
	// where it still stands.
	if !strings.Contains(r107sdFoot(two), "end of the conversation") {
		t.Errorf("second-day 80x24 [tab tab j j]: `j` moved nothing and the row does not say why: %q", r107sdFoot(two))
	}
	if r107sdFoot(one) == r107sdFoot(two) {
		t.Errorf("second-day 80x24: one row for a press that moved the mark and a press that could not: %q", r107sdFoot(one))
	}
	// The cells the note gave up come back as keys, and none is lost.
	for _, key := range []string{"/ search", "esc back", "A archive", "? help", "q quit"} {
		if !strings.Contains(r107sdFoot(one), key) {
			t.Errorf("second-day 80x24 [tab tab j]: the row lost %q: %q", key, r107sdFoot(one))
		}
	}
	if lipgloss.Width(r107sdFoot(one)) > 80 {
		t.Errorf("second-day 80x24 [tab tab j]: the row runs past the terminal (%d): %q",
			lipgloss.Width(r107sdFoot(one)), r107sdFoot(one))
	}

	// Two: the rule, over every scene at five widths on all four keys under
	// both profiles. Wherever a reader press moves a drawn row, the row does
	// not answer the page's question; wherever it moves nothing, the row
	// says something.
	moves, stucks := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		if prof == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
				for _, key := range []string{"j", "k", "ctrl+d", "ctrl+u"} {
					m, before := r107sdWalk(sc, size[0], size[1], []string{"tab", "tab"})
					if m.level != 3 {
						continue
					}
					for n := 1; n <= 3; n++ {
						pressKey(m, key)
						poll(m, sc)
						rows := strings.Split(ansi.Strip(m.View()), "\n")
						foot := r107sdFoot(rows)
						was, now := r107sdMarks(before), r107sdMarks(rows)
						switch {
						case len(was) != len(now):
							// The first press after a jump only draws the mark
							// where the page is already looking (#300); it
							// steps the cursor nowhere.
						case !r107sdSame(was, now):
							moves++
							if strings.Contains(foot, "all of it is on screen") {
								t.Errorf("%v %s %dx%d [tab tab] then %d×%q: the press moved the mark and the row answers the page's question: %q",
									prof, sc.name, size[0], size[1], n, key, foot)
							}
						default:
							stucks++
							if m.note == "" {
								t.Errorf("%v %s %dx%d [tab tab] then %d×%q: the press moved the mark nowhere and said nothing: %q",
									prof, sc.name, size[0], size[1], n, key, foot)
							}
						}
						if lipgloss.Width(foot) > size[0] {
							t.Errorf("%v %s %dx%d [tab tab] then %d×%q: the row runs past the terminal (%d): %q",
								prof, sc.name, size[0], size[1], n, key, lipgloss.Width(foot), foot)
						}
						before = rows
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if moves < 60 || stucks < 60 {
		t.Fatalf("the rule reached %d moving presses and %d stuck ones: it has gone vacuous", moves, stucks)
	}

	// Three: #83's sentence still stands where the press moves nothing, so a
	// fold that drops the note from the reader altogether fails here.
	if _, rows := r107sdWalk(sd, 80, 24, []string{"tab", "tab", "k"}); !strings.Contains(r107sdFoot(rows), "all of it is on screen") {
		t.Errorf("second-day 80x24 [tab tab k]: `k` at the first row no longer says why it moved nothing: %q", r107sdFoot(rows))
	}
}

// ---- round 108, two-tools ----
// TestTheReadersMoveKeyIsNamedForWhatItMoves pins the reader's movement
// pair to the one thing it does.
//
// #83 named the pair `j/k scroll` because in the reader it moved the
// viewport; #300 gave the reader a cursor and `j`, `k`, `ctrl+d` and
// `ctrl+u` have stepped that cursor ever since (readerCursorMove), the
// viewport following only far enough to keep the mark on screen. #306 put
// the cursor's word on a page that fits and left `j/k scroll` on a page
// that scrolls, on the ground that "there the key does move the viewport"
// — which the frame refutes: on `two-tools` at eighty the reader opens
// with the mark five rows above the last drawn row, and three `j`s walk
// the mark while `↑ 4 lines above` and every row above the mark stand
// unmoved. One key, one act, one name: `j/k rows`, the word the same deck
// already uses for a cursor that walks rows one level out (#220).
//
// Three sides, so the word cannot simply be swapped everywhere and the
// row left worse:
//   - no reader row says `j/k scroll`, at any width, on either kind of
//     page;
//   - the word is earned: on a page that scrolls, `j` pressed where the
//     mark is not on the last drawn row moves the mark and leaves the
//     page's top where it was;
//   - it costs nothing: the same stand drawn with the old, longer word
//     names no key the drawn row lacks, and no row runs past its
//     terminal.
func TestTheReadersMoveKeyIsNamedForWhatItMoves(t *testing.T) {
	forceASCII(t)
	scrolls, fits, walked := 0, 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range r108ttWordSizes {
				w, h := size[0], size[1]
				inner := w - 2
				for _, route := range r108ttWordRoutes {
					m := sceneModel(sc, w, h)
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					if m.showHelp || m.searching || m.replying || m.level < levelReader {
						continue
					}
					rows := strings.Split(ansi.Strip(m.View()), "\n")
					if len(rows) == 0 {
						continue
					}
					foot := rows[len(rows)-1]
					where := fmt.Sprintf("%s %v %dx%d", sc.name, route, w, h)

					// One name for one act, on either kind of page.
					if strings.Contains(foot, "j/k scroll") {
						t.Errorf("%s: the reader's row calls the cursor pair %q; `j` there steps the mark (#300)\n  foot=%q",
							where, "j/k scroll", strings.TrimRight(foot, " "))
					}
					for _, r := range rows {
						if lipgloss.Width(r) > w {
							t.Errorf("%s: a row runs past the terminal (%d of %d): %q", where, lipgloss.Width(r), w, r)
						}
					}
					if m.readerPageFits() {
						fits++
						continue
					}
					scrolls++
					// The word costs no key: on the page the rename
					// touches, the row drawn with the old, longer word
					// names nothing this row lacks (#281, #284). The
					// counterfactual goes through the whole chain
					// (`footerTraded`), not the mirror's trade alone: the
					// archive door's trade is the outermost of it (#329)
					// and is decided by what the other side of it draws
					// (#330), so a row drawn short of that trade stood the
					// door where the row beside it had given the door up
					// for a key, and the word was blamed for the
					// difference. The long word's row does not take the
					// cursor trade's own branch — it no longer leads with
					// `j/k rows · ` — so what this adds is the door's
					// trade and nothing else.
					whole := m.keymap()
					if longer := strings.Replace(whole, "j/k rows · ", "j/k scroll · ", 1); longer != whole {
						was := strings.Replace(m.footerTraded(longer, inner), "j/k scroll · ", "j/k rows · ", 1)
						if !footerNamesAll(was, foot) {
							t.Errorf("%s: the shorter word cost the row a key\n  now=%q\n  was=%q", where, r108ttWordKeys(foot), r108ttWordKeys(was))
						}
					}
					// Earned: the press the row names moves the mark, and
					// on this stand it moves no line of the page.
					doc := m.doc(m.readerWidth())
					top, mark := m.readerTop(doc), r108ttWordMark(m)
					pressKey(m, "j")
					poll(m, sc)
					if m.readerTop(m.doc(m.readerWidth())) == top {
						if got := r108ttWordMark(m); got == mark {
							t.Errorf("%s: on a page that scrolls `j` moved neither the page nor the mark\n  mark=%q", where, mark)
						} else {
							walked++
						}
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if scrolls < 20 {
		t.Errorf("only %d reader stands on a page that scrolls; the renamed side is unmeasured", scrolls)
	}
	if fits < 20 {
		t.Errorf("only %d reader stands on a page that fits; the held side is unmeasured", fits)
	}
	if walked < 10 {
		t.Errorf("only %d scrolling stands where `j` walked the mark and scrolled nothing; the frame's own refutation is unmeasured", walked)
	}
	t.Logf("reader stands: %d scrolling · %d fitting · %d where `j` walked the mark and moved no line", scrolls, fits, walked)
}

var r108ttWordSizes = [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}}

// r108ttWordRoutes are ways onto a reader page: straight in, in on another
// row, and the chapter and cursor keys pressed once there, so both a page
// that fits and a page that scrolls are stood on.
var r108ttWordRoutes = [][]string{
	{"tab", "tab"},
	{"tab", "tab", "]"},
	{"tab", "tab", "j"},
	{"1", "tab", "tab"},
	{"2", "tab", "tab"},
}

// r108ttWordKeys is the keymap half of a footer, for a message.
func r108ttWordKeys(foot string) string {
	s := strings.TrimSpace(ansi.Strip(foot))
	if i := strings.Index(s, "  "); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// r108ttWordMark is the reader row the cursor stands on, by its own words.
func r108ttWordMark(m *Model) string {
	opts := ReaderOpts{Width: m.readerWidth(), Height: m.readerHeight(),
		Scroll: m.scroll, Anchor: m.anchor, Unfolded: m.unfolded,
		CWD: m.readerCWD(), Now: m.now, Lanes: m.laneClauses()}
	for _, line := range strings.Split(RenderReader(m.events, opts), "\n") {
		if plain := ansi.Strip(line); strings.ContainsRune(plain, '▸') {
			return strings.TrimRight(plain, " ")
		}
	}
	return ""
}
