package journey

import (
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/transcript"
)

// A relayed agent-message is a prompt whose text is the message and whose
// From is the envelope's sender (#399).
func TestARelayedAgentMessageIsAPromptFromItsSender(t *testing.T) {
	s := NewSegmenter()
	at := time.Date(2026, 9, 19, 17, 0, 0, 0, time.UTC)
	s.Observe(transcript.Event{UUID: "u1", Type: transcript.EventUser, IsMeta: true, Timestamp: at,
		Text: "Another Claude session sent a message:\n<agent-message from=\"reviewer\">\nthe cursor bound looks off by one\nsecond line\n</agent-message>"})
	tr := s.Trail()
	if len(tr.Prompts) != 1 {
		t.Fatalf("prompts = %d, want 1", len(tr.Prompts))
	}
	p := tr.Prompts[0]
	if !p.Relayed || p.From != "reviewer" || p.Text != "the cursor bound looks off by one" {
		t.Errorf("prompt = %+v", p)
	}
}
