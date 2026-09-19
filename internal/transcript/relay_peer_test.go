package transcript

import "testing"

// A message one session sends another arrives as a meta turn carrying an
// agent-message envelope: it is a relay, not machinery, and the envelope
// names who sent it (#399).
func TestARelayedMetaTurnIsNotMachinery(t *testing.T) {
	ev := Event{Type: EventUser, IsMeta: true, Text: "Another Claude session sent a message:\n<agent-message from=\"reviewer\">\nthe cursor bound looks off by one\n</agent-message>"}
	if ev.Machinery() {
		t.Errorf("a relayed meta turn read as machinery")
	}
	from, body, ok := ev.AgentMessage()
	if !ok || from != "reviewer" || body != "the cursor bound looks off by one" {
		t.Errorf("the envelope was not read: %q %q %v", from, body, ok)
	}
	if ev.RelayFrom() != "reviewer" {
		t.Errorf("RelayFrom = %q", ev.RelayFrom())
	}
	plain := Event{Type: EventUser, IsMeta: true, Text: "<system-reminder>the harness</system-reminder>"}
	if !plain.Machinery() {
		t.Errorf("a meta envelope that is no relay stopped being machinery")
	}
}
