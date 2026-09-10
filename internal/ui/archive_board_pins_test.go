package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

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
	sweep(t)
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
				if prof == termenv.TrueColor && !sweepColour() {
					continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
				}
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
	// Whether `a` acts is read off the command the key returns, and the
	// deck refuses the key where no `claude` is on PATH: on a runner with
	// none the probe named no archive board and the pin was vacuous. The
	// pin asks about the row, not the machine, so it puts a `claude` on
	// its own PATH; the command it returns is never run here.
	stub := t.TempDir()
	if err := os.WriteFile(filepath.Join(stub, askBin), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
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
	sweep(t)
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
		if prof == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
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
				if prof == termenv.TrueColor && !sweepColour() {
					continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
				}
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
	sweep(t)
	forceASCII(t)
	routes := [][]string{
		{"A", "x", "x"},
		{"2", "x", "A", "x", "x"},
		{"2", "x", "A", "x", "tab", "tab"},
	}
	kept, whole, named, offered := 0, 0, 0, 0
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
	sweep(t)
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
		if prof == termenv.TrueColor && !sweepColour() {
			continue // the colour pass is TestTheCorpusSaysTheSameWordsWithColourOn's
		}
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
	if stands < 75 || refused < 75 { // half of the two-profile floor: the sweep walks one profile (#324)
		t.Fatalf("the walk reached only %d archive stands, %d of them refusals", stands, refused)
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

// ---- round 110, two-tools ----
// TestTheArchiveMirrorRefusalNamesTheDoorOnce holds the archive's `m`
// refusal to one saying of one fact.
//
// `m` in the archive has no mirror to turn on, and it says so. It also
// spelled out the way back — "no mirror in the archive · A returns to the
// fleet" — while `A fleet` stood on the very same row, twenty cells to its
// left: the same thing said twice on one line, which #95 and #96 settled
// and #299 and #303 cut out of the attach and reply refusals. Here the
// second saying was not free. The note is forty-nine cells; the row is one
// line; and at 120 five of the eleven keys the archive's own row drew a
// press earlier came off it to pay for them — `tab deeper`, the frame's
// only naming of the way deeper (#264, #296, #297), and `j/k move` among
// them — and four of thirteen at 152.
//
// Three sides, under both colour profiles (#215, #218):
//   - the refusal says the fact and not the door: no clause of it names a
//     key the keymap on the same row already names;
//   - the door is still on the frame: the row names `A fleet`;
//   - the shorter note costs the row nothing and buys some of it back: the
//     finished row names every key it would have named under the long
//     note, and on some stands names one the long note took away.
func TestTheArchiveMirrorRefusalNamesTheDoorOnce(t *testing.T) {
	forceASCII(t)
	const long = "no mirror in the archive · A returns to the fleet"
	stands, bought := 0, 0
	for _, prof := range []termenv.Profile{termenv.Ascii, termenv.TrueColor} {
		old := lipgloss.ColorProfile()
		lipgloss.SetColorProfile(prof)
		for _, sc := range allScenes() {
			for _, size := range r110ttBDoorSizes {
				w, h := size[0], size[1]
				for _, route := range r110ttBDoorRoutes {
					m := sceneModel(sc, w, h)
					for _, k := range route {
						pressKey(m, k)
						poll(m, sc)
					}
					if !m.archiveView || m.showHelp || m.searching || m.replying {
						continue
					}
					pressKey(m, "m")
					poll(m, sc)
					if !strings.HasPrefix(m.note, "no mirror in the archive") {
						continue
					}
					stands++
					where := fmt.Sprintf("%s %v m %dx%d", sc.name, route, w, h)
					rows := strings.Split(ansi.Strip(m.View()), "\n")
					foot := rows[len(rows)-1]
					for _, r := range rows {
						if lipgloss.Width(r) > w {
							t.Errorf("%s: a row runs past the terminal (%d of %d): %q", where, lipgloss.Width(r), w, r)
						}
					}

					// The fact, once: no clause of the note repeats a key
					// the same row already names.
					if strings.Contains(m.note, "A returns to the fleet") {
						t.Errorf("%s: the refusal spells out `A` while the row names it: note=%q\n  foot=%q",
							where, m.note, strings.TrimRight(foot, " "))
					}

					// The door is on the frame.
					if !strings.Contains(r110ttBDoorKeys(foot), "A fleet") {
						t.Errorf("%s: the row does not name the way back to the fleet\n  foot=%q", where, strings.TrimRight(foot, " "))
					}

					// And the short note costs nothing.
					was := m.note
					m.note = long
					under := ansi.Strip(m.footerLine(w))
					m.note = was
					if !footerNamesAll(under, foot) {
						t.Errorf("%s: the shorter note cost the row a key\n  now=%q\n  was=%q", where, r110ttBDoorKeys(foot), r110ttBDoorKeys(under))
					}
					if !footerNamesAll(foot, under) {
						bought++
					}
				}
			}
		}
		lipgloss.SetColorProfile(old)
	}
	if stands < 20 {
		t.Errorf("only %d archive `m` refusals walked; the row this pin is about is unmeasured", stands)
	}
	if bought < 4 {
		t.Errorf("only %d of the refusals bought a key back; the second saying's cost is unmeasured", bought)
	}
	t.Logf("archive `m` refusals: %d · rows that bought a key back: %d", stands, bought)
}

var r110ttBDoorSizes = [][2]int{{120, 34}, {152, 40}, {220, 48}}

// r110ttBDoorRoutes are ways into the archive: its board, its list, a row
// hidden on the way in, and one level deeper.
var r110ttBDoorRoutes = [][]string{
	{"A"},
	{"A", "tab"},
	{"x", "A"},
	{"2", "x", "A"},
	{"A", "j"},
	{"A", "tab", "tab"},
}

// r110ttBDoorKeys is the keymap half of a footer, the note read past at
// the gap the keymap never contains (#134).
func r110ttBDoorKeys(foot string) string {
	s := strings.TrimSpace(ansi.Strip(foot))
	if i := strings.Index(s, "  "); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
