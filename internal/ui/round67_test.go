package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// Round sixty-seven: the pair (#398). `tab` on a lane whose `→N` names a
// live session opens that session's own reader beside the lane's, where
// the width has room for two; the lane's reader keeps the keys and the
// other follows its mark by time; leaving the lane's reader closes it, and
// the session ending closes it with a note.

// pairStand opens the linked lane's reader on the pair scene.
func pairStand(w, h int) (*Model, scene) {
	sc := scenePair()
	m := sceneModel(sc, w, h)
	for _, k := range []string{"2", "tab", "G", "tab"} {
		pressKey(m, k)
		poll(m, sc)
	}
	return m, sc
}

// The pair opens where two readers fit, and not where they do not.
func TestALinkedLaneOpensThePair(t *testing.T) {
	forceASCII(t)
	for _, wh := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
		m, _ := pairStand(wh[0], wh[1])
		if m.level != levelReader || m.readerLane == "" {
			t.Fatalf("%dx%d: the stand is not the lane's reader: level %d lane %q", wh[0], wh[1], m.level, m.readerLane)
		}
		view := ansi.Strip(m.View())
		fits := wh[0] >= 152
		if got := strings.Contains(view, "READER · →1 builder · follows"); got != fits {
			t.Errorf("%dx%d: the pair drawn %v, want %v:\n%s", wh[0], wh[1], got, fits, view)
		}
		if !fits {
			continue
		}
		if m.pairKey == "" {
			t.Errorf("%dx%d: the pair is drawn and not held", wh[0], wh[1])
		}
		// The follower is the linked session's own conversation, with the
		// result the lane's file never received.
		if !strings.Contains(view, "30 passed · 1 failed") {
			t.Errorf("%dx%d: the follower does not show the linked session's own result:\n%s", wh[0], wh[1], view)
		}
		// The lane's reader keeps the keys: its title wears the mark.
		if !strings.Contains(view, focusMark+"READER · shop") {
			t.Errorf("%dx%d: the lane's reader lost the keys:\n%s", wh[0], wh[1], view)
		}
		// The fresher clock is said once: the follower's title carries
		// it, and the stub under the hung call keeps its silence alone.
		if strings.Contains(view, "→1 wrote") || !strings.Contains(view, "wrote 20s ago") {
			t.Errorf("%dx%d: the fresher clock is not said once, on the follower:\n%s", wh[0], wh[1], view)
		}
		// Every row is its width.
		for i, row := range strings.Split(view, "\n") {
			if n := ansi.StringWidth(row); n > wh[0] {
				t.Errorf("%dx%d: row %d is %d cells: %q", wh[0], wh[1], i, n, row)
			}
		}
	}
}

// The follower follows the mark by time: its row is the first at or after
// the mark's moment, or the earliest it has, and moving the mark moves it.
func TestTheFollowerFollowsTheMark(t *testing.T) {
	forceASCII(t)
	m, sc := pairStand(152, 40)
	_, rw := m.pairWidths()
	at := func() time.Time {
		doc := m.pairDoc(rw)
		row := m.pairRow(rw)
		if row < 0 || row >= len(doc) {
			t.Fatalf("no follower row")
		}
		return doc[row].at
	}
	first := at()
	if first.IsZero() || m.anchorAt.IsZero() {
		t.Fatalf("no clocks to follow: row %v mark %v", first, m.anchorAt)
	}
	if first.Before(m.anchorAt) && first.Sub(m.anchorAt) < -time.Minute {
		// A row before the mark is only the earliest the follower has,
		// or a call whose result the mark landed on.
		doc := m.pairDoc(rw)
		if row := m.pairRow(rw); row > 0 && !doc[0].at.IsZero() && doc[0].at.Before(first) {
			t.Errorf("the follower stands %v, before the mark %v, with earlier rows above it", first, m.anchorAt)
		}
	}
	moved := false
	for i := 0; i < 6; i++ {
		pressKey(m, "k")
		poll(m, sc)
		now := at()
		if now.After(first) {
			t.Errorf("k moved the mark earlier and the follower later: %v after %v", now, first)
		}
		if now.Before(first) {
			moved = true
		}
		first = now
	}
	if !moved {
		t.Errorf("six k's moved the mark and the follower never moved")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "the mark") {
		t.Errorf("the follower's above row does not say how it stands to the mark:\n%s", view)
	}
}

// Leaving the lane's reader closes the pair; coming back by h/l brings it.
// `l` is the next session in the board's order, not the follower: the
// digit on the follower's title is the key that goes there.
func TestThePairClosesWithTheLane(t *testing.T) {
	forceASCII(t)
	m, sc := pairStand(152, 40)
	if m.pairKey == "" {
		t.Fatal("no pair to close")
	}
	pressKey(m, "esc")
	poll(m, sc)
	if m.pairKey != "" || strings.Contains(ansi.Strip(m.View()), "follows") {
		t.Errorf("esc left the pair open")
	}
	pressKey(m, "tab")
	poll(m, sc)
	if m.pairKey == "" {
		t.Fatalf("tab did not reopen the lane's pair")
	}
	pressKey(m, "l")
	poll(m, sc)
	if m.pairKey != "" || m.readerLane != "" {
		t.Errorf("l to the linked session kept the lane's pair: lane %q pair %q", m.readerLane, m.pairKey)
	}
	pressKey(m, "h")
	poll(m, sc)
	if m.pairKey == "" || m.readerLane == "" {
		t.Errorf("h back did not bring the lane and its pair: lane %q pair %q", m.readerLane, m.pairKey)
	}
	if !strings.Contains(ansi.Strip(m.View()), "READER · →1 builder · follows") {
		t.Errorf("h back drew no follower")
	}
}

// A follower whose session ended closes, and the note says who.
func TestThePairClosesWhenTheSessionEnds(t *testing.T) {
	forceASCII(t)
	m, sc := pairStand(152, 40)
	if m.pairKey == "" {
		t.Fatal("no pair")
	}
	for i := range sc.sessions {
		if sc.sessions[i].Info.Key() == m.pairKey {
			sc.sessions[i].Live = false
		}
	}
	m.Update(fleetMsg{sessions: sc.sessions, at: m.now, trailFor: m.selectedKey, hasTrail: true, trail: sc.trails[m.selectedKey], trails: sc.trails, agents: sc.agents})
	if m.pairKey != "" {
		t.Errorf("the pair stayed open on a session that ended")
	}
	if m.note != "builder ended · reading alone" {
		t.Errorf("the note does not say who ended: %q", m.note)
	}
	if strings.Contains(ansi.Strip(m.View()), "follows") {
		t.Errorf("the frame still draws the follower")
	}
}
