package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/state"
)

// Done is a finished turn that is news: unread and within done_for. Looked
// at, it is idle; left past done_for, it is idle too. An Esc is never done.
func TestDoneIsAFinishNobodyHasSeen(t *testing.T) {
	withCards(t)
	m := sceneModel(sceneCards(), 290, 80)
	s3 := sessionKey("s3") // idle, shipped, finished 4h ago, unread
	s, _ := m.session(s3)
	if word, _, _ := m.cardState(s, m.trails[s3]); word != cardDone {
		t.Fatalf("an unread finish within done_for: %q, want done", word)
	}

	m.markSeen(s3)
	if word, _, _ := m.cardState(s, m.trails[s3]); word != cardIdle {
		t.Errorf("looked at: %q, want idle", word)
	}

	demo, _ := m.session(sessionKey("demo")) // finished 29h ago, unread
	if word, _, _ := m.cardState(demo, m.trails[sessionKey("demo")]); word != cardIdle {
		t.Errorf("unread past done_for: %q, want idle", word)
	}

	SetDone(48*time.Hour, "")
	t.Cleanup(func() { doneFor = 4 * time.Hour })
	if word, _, _ := m.cardState(demo, m.trails[sessionKey("demo")]); word != cardDone {
		t.Errorf("done_for 48h, finished 29h ago: %q, want done", word)
	}

	esc := s
	esc.Snap = state.Snapshot{State: state.Idle, Since: sceneNow, Reason: "interrupted", Interrupted: true}
	if word, sub, _ := m.cardState(esc, m.trails[s3]); word != cardIdle || sub != "cancel" {
		t.Errorf("an Esc: %q · %q, want idle · cancel", word, sub)
	}
}

// Back from tmux, every session the person hopped through while away is
// read — no longer done — and the one they left from is selected.
func TestBackFromTmuxReadsEverySessionYouWereIn(t *testing.T) {
	withCards(t)
	m := sceneModel(sceneCards(), 290, 80)
	m.point(sessionKey("harness"))
	m.clock = func() time.Time { return time.Unix(1793437400, 0) }
	m.handedAt = m.wallNow()
	// s3 is %6, demo %7: the person was in s3 last, demo before it, and
	// in gemma (%8) yesterday, before the deck handed over.
	m.runner = viewedTmux{lines: "1793437460 11 %6\n1793437430 11 %7\n1793300000 11 %8\n"}
	_, cmd := m.Update(attachDoneMsg{target: "w:7.0"})
	m.Update(cmd())

	if got := selectedName(m); got != "s3" {
		t.Errorf("selected %s, want s3, where the person left from", got)
	}
	for _, name := range []string{"s3", "demo"} {
		s, _ := m.session(sessionKey(name))
		if m.unread(s) {
			t.Errorf("%s was visited while away and is still unread", name)
		}
	}
	if _, ok := m.seen[sessionKey("gemma")]; ok {
		t.Error("gemma was visited before the handover and was marked read")
	}
}

type viewedTmux struct{ lines string }

func (v viewedTmux) Output(args ...string) ([]byte, error) {
	if len(args) > 0 && args[0] == "list-panes" {
		return []byte(v.lines), nil
	}
	return nil, nil
}

// The badge and the quiet row say done in done's own colour.
func TestDoneHasItsOwnColour(t *testing.T) {
	withCards(t)
	m := sceneModel(sceneCards(), 290, 80)
	s, _ := m.session(sessionKey("s3"))
	if _, _, col := m.cardState(s, m.trails[sessionKey("s3")]); col != colDone {
		t.Errorf("done's colour is %v, want colDone", col)
	}
	if badge := m.cardBadge(s, m.trails[sessionKey("s3")]); !strings.Contains(badge, "done") {
		t.Errorf("badge %q", badge)
	}
}
