package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
)

// The board does not wait on the selected session's conversation. Reading it
// for the first time is a replay from the first byte, and every card's trail
// travelled in the same message as it, so at launch each card said "reading
// its transcript…" for as long as the longest file took. The poll brings the
// trail without the events and fires the next poll at once; that one reads the
// conversation whatever it costs, so the putting-off happens once.
func TestTheBoardDoesNotWaitOnTheSelectedConversation(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "projects", "-home-user-api")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	lines := []string{
		`{"type":"user","uuid":"u1","sessionId":"s","cwd":"/home/user/api","timestamp":"` + at + `","message":{"role":"user","content":"fix the 401"}}`,
		`{"type":"assistant","uuid":"a1","sessionId":"s","cwd":"/home/user/api","timestamp":"` + at + `","message":{"id":"m1","role":"assistant","content":[{"type":"text","text":"on it"}]}}`,
	}
	path := filepath.Join(dir, "s.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := New(fleet.NewManager(root))
	m.Update(m.refresh()()) // the launch's first poll: the fleet, no columns
	m.selectedKey = path

	// The test runs each poll itself rather than the batch Update fired it
	// in, so the in-flight guard is let go by hand.
	poll := func() fleetMsg {
		t.Helper()
		m.refreshing = false
		msg, ok := m.refresh()().(fleetMsg)
		if !ok {
			t.Fatal("refresh did not bring a fleet")
		}
		return msg
	}
	first := poll()
	if !first.hasTrail || !first.eventsOwed || first.events != nil {
		t.Fatalf("first poll of the selected session: hasTrail=%v eventsOwed=%v events=%d, want its trail now and its conversation owed",
			first.hasTrail, first.eventsOwed, len(first.events))
	}
	if _, cmd := m.Update(first); cmd == nil {
		t.Fatal("the owed conversation fired no poll")
	}
	second := poll()
	if second.eventsOwed || len(second.events) != 2 {
		t.Fatalf("the poll that was owed: eventsOwed=%v events=%d, want the conversation (2 events)", second.eventsOwed, len(second.events))
	}
	m.Update(second)
	if len(m.events) != 2 || m.eventsDue != "" {
		t.Errorf("after the owed poll: %d events, eventsDue=%q", len(m.events), m.eventsDue)
	}
	if third := poll(); third.eventsOwed {
		t.Error("a feed that keeps its conversation put it off again")
	}
}
