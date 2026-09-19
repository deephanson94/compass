package transcript

import (
	"strings"
	"testing"
)

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
	// The harness's own shape for SendMessage between sessions: the name
	// is `from-name`, the `from` is a socket (the owner's screenshot).
	cross := Event{Type: EventUser, IsMeta: true, Text: "Another Claude session sent a message:\n<cross-session-message from=\"uds:/run/user/10014689/cc-socks/1044442.sock\" from-name=\"latency-breakdown\" from-mode=\"prompting\">\nReviewed against source. 1 CONFIRMED clean, 3 need a correction.\n\n**Claim 1**\n</cross-session-message>"}
	from, body, ok = cross.AgentMessage()
	if !ok || from != "latency-breakdown" || !strings.HasPrefix(body, "Reviewed against source.") || strings.Contains(body, "</cross") {
		t.Errorf("the cross-session envelope was not read: %q %q %v", from, body, ok)
	}
	if cross.Machinery() {
		t.Errorf("a cross-session message read as machinery")
	}
	plain := Event{Type: EventUser, IsMeta: true, Text: "<system-reminder>the harness</system-reminder>"}
	if !plain.Machinery() {
		t.Errorf("a meta envelope that is no relay stopped being machinery")
	}
}
