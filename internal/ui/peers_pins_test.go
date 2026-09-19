package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
)

// The peer join (#399): a message one session sends another is a relayed
// meta turn carrying `<agent-message from="…">`; it is a prompt, its row
// wears the sender's digit, and the reader pairs with the sender.

func peersStand(w, h int, keys ...string) (*Model, scene) {
	sc := scenePeers()
	m := sceneModel(sc, w, h)
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
	}
	return m, sc
}

// A relayed message from a live session draws `→N` on its prompt row, on
// the board's columns and on the trail, at every width the row has room.
func TestARelayedPromptWearsItsSendersDigit(t *testing.T) {
	forceASCII(t)
	for _, wh := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
		m, _ := peersStand(wh[0], wh[1])
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "→2") || !strings.Contains(view, "→1") {
			t.Errorf("%dx%d: the board's relayed rows do not name their senders:\n%s", wh[0], wh[1], view)
		}
		if strings.Contains(view, "<agent-message") {
			t.Errorf("%dx%d: the envelope's tag is on the frame:\n%s", wh[0], wh[1], view)
		}
	}
	m, _ := peersStand(100, 30, "1", "tab")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "→2") {
		t.Errorf("100x30: the trail's relayed row does not name its sender:\n%s", view)
	}
}

// The reader pairs with the peer from either side, and the follower's
// title carries the link and the peer's own state.
func TestTheReaderPairsWithThePeer(t *testing.T) {
	forceASCII(t)
	m, sc := peersStand(152, 40, "1", "tab", "G", "tab")
	if m.level != levelReader || m.readerLane != "" {
		t.Fatalf("the stand is not the session's own reader: level %d lane %q", m.level, m.readerLane)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "READER · →2 shop · follows") {
		t.Errorf("the reviewer's reader did not pair with shop:\n%s", view)
	}
	if !strings.Contains(view, "relayed from shop") || strings.Contains(view, "<agent-message") {
		t.Errorf("the reader does not say who a relayed turn is from, or draws the tag:\n%s", view)
	}
	// From the other side.
	pressKey(m, "esc")
	poll(m, sc)
	for _, k := range []string{"l", "tab"} {
		pressKey(m, k)
		poll(m, sc)
	}
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "READER · →1 reviewer · follows") {
		t.Errorf("shop's reader did not pair with the reviewer:\n%s", view)
	}
	// The peer's own state, in the fleet's words: stopped on a question,
	// the follower says so and not `wrote`.
	for i := range sc.sessions {
		if sessionName(sc.sessions[i].Info) == "reviewer" {
			sc.sessions[i].Snap = state.Snapshot{State: state.NeedsYou, Since: m.now.Add(-20 * time.Second), Reason: "waiting on your answer", Activity: "AskUserQuestion"}
			sc.sessions[i].Info.LastEventAt = m.now.Add(-20 * time.Second)
		}
	}
	poll(m, sc)
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "needs you 20s") || strings.Contains(view, "follows") && strings.Contains(view, "wrote 20s ago") {
		t.Errorf("a follower that needs you reads as progress:\n%s", view)
	}
	// Under the width, no pair: the reader reads alone.
	n, _ := peersStand(120, 34, "1", "tab", "G", "tab")
	if n.pairKey == "" || strings.Contains(ansi.Strip(n.View()), "follows") {
		t.Errorf("120: the pair is drawn under its width, or not held for it")
	}
}

// The join is the envelope's name, not a guess: a sender the board does
// not know draws no digit and opens no pair.
func TestAnUnknownSenderLinksNothing(t *testing.T) {
	forceASCII(t)
	sc := scenePeers()
	for key, tr := range sc.trails {
		for i := range tr.Prompts {
			if tr.Prompts[i].From != "" {
				tr.Prompts[i].From = "someone-else"
			}
		}
		sc.trails[key] = tr
	}
	m := sceneModel(sc, 152, 40)
	if view := ansi.Strip(m.View()); strings.Contains(view, "→") {
		t.Errorf("an unknown sender drew a link:\n%s", view)
	}
	for _, k := range []string{"1", "tab", "G", "tab"} {
		pressKey(m, k)
		poll(m, sc)
	}
	if m.pairKey != "" {
		t.Errorf("an unknown sender opened a pair with %q", m.pairKey)
	}
}

// The follower is a window, and the row under its title says what it is
// not showing: with a long linked session the mark stands near the
// dispatch and the newest line is below the fold (round 68).
func TestTheFollowerSaysWhatIsBelowIt(t *testing.T) {
	forceASCII(t)
	sc := scenePair()
	key := ""
	for _, s := range sc.sessions {
		if sessionName(s.Info) == "builder" {
			key = s.Info.Key()
		}
	}
	b := sc.trails[key]
	at := sceneNow.Add(-20 * time.Minute)
	more := trailOf(at, "carry on", true,
		legSpec{journey.Build, "db/backfill.py", 3 * time.Minute, []string{"db/backfill.py"}, "", nil},
		legSpec{journey.Test, "pytest tests/migrations", 2 * time.Minute, nil, "31✓", nil},
		legSpec{journey.Build, "the cursor's checkpoint", 4 * time.Minute, []string{"db/runner.py"}, "", nil},
		legSpec{journey.Test, "pytest -x", 2 * time.Minute, nil, "32✓", nil},
		legSpec{journey.Fix, "the resume path", 3 * time.Minute, []string{"db/runner.py"}, "", nil},
	)
	b.Legs = append(b.Legs, more.Legs...)
	sc.trails[key] = b
	for _, wh := range [][2]int{{152, 40}, {220, 48}} {
		m := sceneModel(sc, wh[0], wh[1])
		for _, k := range []string{"2", "tab", "G", "tab"} {
			pressKey(m, k)
			poll(m, sc)
		}
		_, rw := m.pairWidths()
		doc, row := m.pairDoc(rw), m.pairRow(rw)
		if len(doc) <= wh[1]-6 || row < 0 {
			t.Fatalf("%dx%d: the stand is not a cut follower: doc %d row %d", wh[0], wh[1], len(doc), row)
		}
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "lines below") {
			t.Errorf("%dx%d: the follower's page hides its newest line and says nothing:\n%s", wh[0], wh[1], view)
		}
	}
}

// The pair draws one cursor, not two: the keys' own row is inverted, and
// the follower's row carries the mark without the inversion (round 68).
func TestOnlyTheKeysHalfIsInverted(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	for _, wh := range [][2]int{{152, 40}, {220, 48}} {
		m, _ := pairStand(wh[0], wh[1])
		if m.pairKey == "" {
			t.Fatalf("%dx%d: no pair", wh[0], wh[1])
		}
		view := m.View()
		if n := strings.Count(view, "\x1b[7m"); n != 1 {
			t.Errorf("%dx%d: %d inverted rows in the pair, want 1 (the keys'):\n%s", wh[0], wh[1], n, view)
		}
		// And in glyphs alone: the keys' `▸` once, the follower's `▹`
		// once, so a frame with colour off still has one cursor.
		plain := ansi.Strip(view)
		if n := strings.Count(plain, "▸"); n != 1 {
			t.Errorf("%dx%d: %d `▸` on the pair frame, want 1", wh[0], wh[1], n)
		}
		if n := strings.Count(plain, "▹"); n != 1 {
			t.Errorf("%dx%d: %d `▹` on the pair frame, want 1 (the follower's)", wh[0], wh[1], n)
		}
	}
}

// The walkthrough reaches the pair on both scenes at 152 and 220, and never
// under: the corpus every review cites carries the fold (round 68).
func TestTheWalkthroughDrawsThePair(t *testing.T) {
	forceASCII(t)
	for _, sc := range []scene{scenePair(), scenePeers()} {
		for _, wh := range [][2]int{{120, 34}, {152, 40}, {220, 48}} {
			seen := false
			m := sceneModel(sc, wh[0], wh[1])
			for _, k := range append(append(append([]string(nil), canonicalKeys...), "esc"), sc.extra...) {
				pressKey(m, k)
				poll(m, sc)
				if strings.Contains(ansi.Strip(m.View()), "· follows") {
					seen = true
				}
			}
			if want := wh[0] >= 152; seen != want {
				t.Errorf("%s %dx%d: the walkthrough draws the pair %v, want %v", sc.name, wh[0], wh[1], seen, want)
			}
		}
	}
}

// The mark before the follower's first row says so; an empty follower says
// it is empty once polled; a search that leaves the follower out is on its
// title (round 68).
func TestTheFollowersRowsSayTheirCase(t *testing.T) {
	forceASCII(t)
	m, sc := pairStand(152, 40)
	for i := 0; i < 9; i++ {
		pressKey(m, "k")
		poll(m, sc)
	}
	_, rw := m.pairWidths()
	doc, row := m.pairDoc(rw), m.pairRow(rw)
	if row < 0 || !doc[row].at.After(m.anchorAt) {
		t.Fatalf("nine k's did not put the mark before the follower's first row: row %d", row)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "its first line") {
		t.Errorf("the follower on its first line does not say so:\n%s", view)
	}
	// Empty, polled: nothing to read yet, not reading forever.
	m.pairPoll(m.pairKey, nil)
	if view := ansi.Strip(m.View()); !strings.Contains(view, "nothing to read yet") || strings.Contains(view, "reading the transcript") {
		t.Errorf("an empty polled follower still says it is reading:\n%s", view)
	}
	// Outside the search: a standing fleet search the follower does not
	// answer, entered before the reader (in the reader, `/` is the text's).
	sc = scenePair()
	m = sceneModel(sc, 152, 40)
	for _, k := range []string{"/", "zzz", "enter", "2", "tab", "G", "tab"} {
		pressKey(m, k)
		poll(m, sc)
	}
	if view := ansi.Strip(m.View()); m.pairKey == "" || !strings.Contains(view, "follows · outside /zzz") {
		t.Errorf("the follower the search left out does not say so:\n%s", view)
	}
}

// A follower that changes directory writes under a new key: the pair
// follows the same id, and a follower that did end is named, not keyed
// (round 68, the reader's measure).
func TestThePairFollowsItsSessionAcrossANewKey(t *testing.T) {
	forceASCII(t)
	m, sc := pairStand(152, 40)
	if m.pairKey == "" {
		t.Fatal("no pair")
	}
	old := m.pairKey
	for i := range sc.sessions {
		if sc.sessions[i].Info.Key() == old {
			moved := sc.sessions[i]
			moved.Info.TranscriptPath = "/x/builder-moved.jsonl"
			moved.Info.CWD = "/home/user/shop/.worktrees/builder2"
			sc.sessions[i].Live = false
			sc.sessions = append(sc.sessions, moved)
			sc.trails[moved.Info.Key()] = sc.trails[old]
		}
	}
	m.Update(fleetMsg{sessions: sc.sessions, at: m.now, trailFor: m.selectedKey, hasTrail: true, trail: sc.trails[m.selectedKey], trails: sc.trails, agents: sc.agents})
	if m.pairKey != "/x/builder-moved.jsonl" {
		t.Errorf("the pair did not follow its session to its new key: %q, note %q", m.pairKey, m.note)
	}
	if strings.Contains(m.note, "ended") {
		t.Errorf("a session that moved was said to have ended: %q", m.note)
	}
	// Gone for good: the note names it, never its path.
	for i := range sc.sessions {
		sc.sessions[i].Live = false
	}
	m.Update(fleetMsg{sessions: sc.sessions, at: m.now, trailFor: m.selectedKey, hasTrail: true, trail: sc.trails[m.selectedKey], trails: sc.trails, agents: sc.agents})
	if m.pairKey != "" || strings.Contains(m.note, ".jsonl") || !strings.Contains(m.note, "ended · reading alone") {
		t.Errorf("the ended follower's note is wrong: %q (pair %q)", m.note, m.pairKey)
	}
}

// A follower whose newest line is before the mark says so, not only how
// far before (the owner's screenshot: a peer four hours quiet).
func TestTheFollowerOnItsNewestLineSaysSo(t *testing.T) {
	forceASCII(t)
	m, sc := peersStand(152, 40, "2", "tab", "G", "tab")
	if m.pairKey == "" {
		t.Fatal("no pair")
	}
	// The peer's every line before the mark: push its events back a day.
	for i := range m.pairEvents {
		m.pairEvents[i].Timestamp = m.pairEvents[i].Timestamp.Add(-24 * time.Hour)
	}
	m.pairCache.valid = false
	_ = sc
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "its newest line ·") || !strings.Contains(view, "before the mark") {
		t.Errorf("the follower past its newest line does not say so:\n%s", view)
	}
}
