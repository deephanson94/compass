package ui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
)

func writeTranscript(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	body := `{"type":"user","uuid":"u1","timestamp":"2026-08-30T09:00:00.000Z","message":{"role":"user","content":"fix it"},"sessionId":"s"}` + "\n" +
		`{"type":"assistant","uuid":"a1","timestamp":"2026-08-30T09:00:30.000Z","message":{"role":"assistant","content":[{"type":"text","text":"On it."}]},"sessionId":"s"}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A board column keeps no events: the reader's ring is the selected session's
// alone. When a column becomes the selected session its feed replays the
// transcript once, so the reader opens on the whole conversation.
func TestAColumnKeepsNoEventsUntilItIsRead(t *testing.T) {
	path := writeTranscript(t)
	fs := newFeedStore()

	tr, events := fs.poll("k", path, false)
	if len(tr.Prompts) != 1 {
		t.Fatalf("the column's trail has %d prompts, want 1", len(tr.Prompts))
	}
	if len(events) != 0 {
		t.Errorf("a column kept %d events, want none", len(events))
	}

	_, events = fs.poll("k", path, true)
	if len(events) != 2 {
		t.Errorf("selecting the session gave the reader %d events, want the whole transcript (2)", len(events))
	}
}

// A feed nobody has polled for a while is dropped even though its session is
// still in the fleet; the next poll replays it.
func TestAnIdleFeedExpires(t *testing.T) {
	path := writeTranscript(t)
	fs := newFeedStore()
	fs.poll("k", path, false)
	fs.feeds["k"].polled = time.Now().Add(-feedIdle - time.Minute)
	fs.retain([]fleet.Session{{Info: fleet.SessionInfo{TranscriptPath: "k"}}})
	if _, ok := fs.feeds["k"]; ok {
		t.Error("a feed unpolled for longer than feedIdle was kept")
	}

	fs.poll("k", path, false)
	fs.retain([]fleet.Session{{Info: fleet.SessionInfo{TranscriptPath: "k"}}})
	if _, ok := fs.feeds["k"]; !ok {
		t.Error("a feed polled just now was dropped")
	}
}
