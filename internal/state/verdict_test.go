package state_test

import (
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/state"
)

// The closing line is read as the watcher writes it, and as a model
// decorates it; a verdict anywhere but the last line is not one.
func TestVerdictReadsTheClosingLine(t *testing.T) {
	for _, c := range []struct {
		text, verdict, reason string
	}{
		{"Reviewed 14 files.\n\nVERDICT: needs-you: changes the public C header", state.VerdictNeedsYou, "changes the public C header"},
		{"Draft saved.\nVERDICT: auto-ok", state.VerdictDone, ""},
		{"**VERDICT: needs-you: keep the old API?**\n", state.VerdictNeedsYou, "keep the old API?"},
		{"- `VERDICT: auto-ok`", state.VerdictDone, ""},
		{"VERDICT: needs-you: x\n\nanything after it", "", ""},
		{"VERDICT: maybe", "", ""},
		{"no verdict at all", "", ""},
		{"", "", ""},
	} {
		v, r := state.Verdict(c.text)
		if v != c.verdict || r != c.reason {
			t.Errorf("Verdict(%q) = %q, %q; want %q, %q", c.text, v, r, c.verdict, c.reason)
		}
	}
}

// A pattern of one's own: a reason group means needs-you, a match without
// one means done; "off" turns verdicts off; a bad pattern is refused and
// the one in force stays.
func TestVerdictPatternIsConfigurable(t *testing.T) {
	t.Cleanup(func() { _ = state.SetVerdictPattern(state.DefaultVerdictPattern) })

	if err := state.SetVerdictPattern(`^RESULT (?:BLOCKED (.+)|OK)$`); err != nil {
		t.Fatal(err)
	}
	if v, r := state.Verdict("RESULT BLOCKED needs a schema call"); v != state.VerdictNeedsYou || r != "needs a schema call" {
		t.Errorf("own pattern, blocked = %q, %q", v, r)
	}
	if v, _ := state.Verdict("RESULT OK"); v != state.VerdictDone {
		t.Errorf("own pattern, ok = %q", v)
	}
	if err := state.SetVerdictPattern(`(unclosed`); err == nil {
		t.Error("a pattern that does not compile was taken")
	}
	if v, _ := state.Verdict("RESULT OK"); v != state.VerdictDone {
		t.Error("a refused pattern replaced the one in force")
	}
	if err := state.SetVerdictPattern("off"); err != nil {
		t.Fatal(err)
	}
	if v, _ := state.Verdict("VERDICT: needs-you: x"); v != "" {
		t.Errorf("off still reads verdicts: %q", v)
	}
}

// Rule 4 reads the verdict before the question mark: needs-you is needs
// you with the reason as its activity, even when the reason ends on "?";
// done is idle.
func TestVerdictDecidesAFinishedTurn(t *testing.T) {
	needs := machineWith(
		userPrompt(0, "review PR 130"),
		assistantText(10*time.Second, "Reviewed.\nVERDICT: needs-you: keep the old API?"),
	).Evaluate(at(time.Minute))
	assertState(t, needs, state.NeedsYou)
	assertReason(t, needs, "verdict: needs you")
	assertActivity(t, needs, "keep the old API?")
	if needs.Verdict != state.VerdictNeedsYou {
		t.Errorf("Verdict = %q", needs.Verdict)
	}

	done := machineWith(
		userPrompt(0, "review PR 131"),
		assistantText(10*time.Second, "Draft saved.\nVERDICT: auto-ok"),
	).Evaluate(at(time.Minute))
	assertState(t, done, state.Idle)
	if done.Verdict != state.VerdictDone {
		t.Errorf("Verdict = %q", done.Verdict)
	}
}
