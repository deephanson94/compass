package journey_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/transcript"
)

// ---------------------------------------------------------------- helpers

func segment(evs ...transcript.Event) journey.Trail {
	s := journey.NewSegmenter()
	for _, ev := range evs {
		s.Observe(ev)
	}
	return s.Trail()
}

// legWant describes one expected leg; times are offsets from base.
type legWant struct {
	class   journey.Class
	start   time.Duration
	end     time.Duration
	votes   int
	current bool
}

func assertLegs(t *testing.T, tr journey.Trail, want ...legWant) {
	t.Helper()
	if len(tr.Legs) != len(want) {
		t.Fatalf("got %d legs, want %d:\n%s", len(tr.Legs), len(want), dumpLegs(tr))
	}
	for i, w := range want {
		got := tr.Legs[i]
		if got.Class != w.class {
			t.Errorf("Legs[%d].Class = %v, want %v\n%s", i, got.Class, w.class, dumpLegs(tr))
		}
		if !got.Start.Equal(at(w.start)) {
			t.Errorf("Legs[%d].Start = %v, want %v (base+%v)", i, got.Start, at(w.start), w.start)
		}
		if !got.End.Equal(at(w.end)) {
			t.Errorf("Legs[%d].End = %v, want %v (base+%v)", i, got.End, at(w.end), w.end)
		}
		if got.Votes != w.votes {
			t.Errorf("Legs[%d].Votes = %d, want %d", i, got.Votes, w.votes)
		}
		if got.Current != w.current {
			t.Errorf("Legs[%d].Current = %v, want %v", i, got.Current, w.current)
		}
	}
	// At most one leg is open, and only the last one can be.
	for i, leg := range tr.Legs {
		if leg.Current && i != len(tr.Legs)-1 {
			t.Errorf("Legs[%d].Current = true, but it is not the last leg", i)
		}
	}
}

func dumpLegs(tr journey.Trail) string {
	if len(tr.Legs) == 0 {
		return "  (no legs)\n"
	}
	var b strings.Builder
	for i, leg := range tr.Legs {
		fmt.Fprintf(&b, "  [%d] %-6s label=%-12q start=+%v end=+%v votes=%d current=%v files=%v\n",
			i, leg.Class, leg.Label, leg.Start.Sub(base), leg.End.Sub(base),
			leg.Votes, leg.Current, leg.Files)
	}
	return b.String()
}

// ---------------------------------------------------------------- baseline

func TestSegmenterEmptyTrail(t *testing.T) {
	tr := journey.NewSegmenter().Trail()
	if len(tr.Legs) != 0 || len(tr.Prompts) != 0 || len(tr.Branches) != 0 {
		t.Fatalf("fresh segmenter Trail() = %+v, want everything empty", tr)
	}
}

// Non-voting events alone never open a leg.
func TestSegmenterNonVotingEventsOpenNothing(t *testing.T) {
	tr := segment(
		say(1*time.Minute, "Let me think about this."),
		use(2*time.Minute, "tu_todo", "TodoWrite", []byte(`{"todos":[]}`)),
		result(3*time.Minute, "tu_todo", false),
		transcript.Event{Type: transcript.EventAttachment, Timestamp: at(4 * time.Minute)},
	)
	if len(tr.Legs) != 0 {
		t.Fatalf("got %d legs, want 0:\n%s", len(tr.Legs), dumpLegs(tr))
	}
}

// Trail() is a snapshot: mutating what it handed back cannot reach inside.
func TestSegmenterTrailIsASnapshot(t *testing.T) {
	s := journey.NewSegmenter()
	s.Observe(prompt(0, "start"))
	s.Observe(read(1*time.Minute, "tu1", "/w/auth.go"))

	first := s.Trail()
	if len(first.Legs) != 1 || len(first.Prompts) != 1 {
		t.Fatalf("unexpected trail: %+v", first)
	}
	first.Legs[0].Label = "MUTATED"
	first.Legs[0].Class = journey.Ship
	first.Prompts[0].Text = "MUTATED"

	second := s.Trail()
	if second.Legs[0].Label == "MUTATED" || second.Legs[0].Class == journey.Ship {
		t.Errorf("Trail() aliases the segmenter's legs: %+v", second.Legs[0])
	}
	if second.Prompts[0].Text == "MUTATED" {
		t.Errorf("Trail() aliases the segmenter's prompts: %+v", second.Prompts[0])
	}
}

// One assistant event with several tool_uses votes once per tool_use.
func TestSegmenterVotesOncePerToolUse(t *testing.T) {
	tr := segment(
		uses(1*time.Minute,
			transcript.ToolUse{ID: "tu1", Name: "Edit", Input: pathInput("/w/a.go")},
			transcript.ToolUse{ID: "tu2", Name: "Edit", Input: pathInput("/w/b.go")},
			transcript.ToolUse{ID: "tu3", Name: "TodoWrite", Input: []byte(`{}`)}, // no vote
		),
	)
	assertLegs(t, tr, legWant{journey.Build, 1 * time.Minute, 1 * time.Minute, 2, true})
	if got := strings.Join(tr.Legs[0].Files, ","); got != "a.go,b.go" {
		t.Errorf("Files = %q, want %q", got, "a.go,b.go")
	}
}

// ---------------------------------------------------------------- T20

// T20 — weak votes are pressure, not a boundary: two stray Build votes fold
// into the open Scout leg, a Scout vote resets the streak, and only the third
// consecutive Build vote splits — with the new leg starting at the FIRST of
// the three.
func TestT20HysteresisThreeConsecutiveWeakVotesSplit(t *testing.T) {
	s := journey.NewSegmenter()
	open := []transcript.Event{
		read(1*time.Minute, "tu1", "/w/auth.go"),  // Scout leg opens here
		read(2*time.Minute, "tu2", "/w/token.go"), // Scout
		edit(3*time.Minute, "tu3", "/w/auth.go"),  // Build pressure 1 of 3
		edit(4*time.Minute, "tu4", "/w/auth.go"),  // Build pressure 2 of 3
		read(5*time.Minute, "tu5", "/w/token.go"), // Scout: the streak resets
	}
	for _, ev := range open {
		s.Observe(ev)
	}

	// Two strays folded in; the streak was broken before it reached three.
	assertLegs(t, s.Trail(), legWant{journey.Scout, 1 * time.Minute, 5 * time.Minute, 5, true})

	s.Observe(edit(6*time.Minute, "tu6", "/w/store.go")) // Build 1 of 3
	s.Observe(edit(7*time.Minute, "tu7", "/w/store.go")) // Build 2 of 3
	assertLegs(t, s.Trail(), legWant{journey.Scout, 1 * time.Minute, 7 * time.Minute, 7, true})

	s.Observe(edit(8*time.Minute, "tu8", "/w/store.go")) // Build 3 of 3 → split

	tr := s.Trail()
	assertLegs(t, tr,
		// The Scout leg keeps the votes that stayed in it and ends at its last one.
		legWant{journey.Scout, 1 * time.Minute, 5 * time.Minute, 5, false},
		// The new leg starts at the first of the three consecutive votes.
		legWant{journey.Build, 6 * time.Minute, 8 * time.Minute, 3, true},
	)

	// The migrated votes take their files with them.
	if got := strings.Join(tr.Legs[1].Files, ","); got != "store.go" {
		t.Errorf("Legs[1].Files = %q, want %q", got, "store.go")
	}
}

// A run of three weak votes that is not of the SAME class is not a boundary.
func TestT20MixedWeakVotesDoNotSplit(t *testing.T) {
	tr := segment(
		read(1*time.Minute, "tu1", "/w/auth.go"),        // Scout leg
		edit(2*time.Minute, "tu2", "/w/auth.go"),        // Build pressure
		edit(3*time.Minute, "tu3", "/w/docs/spec.md"),   // Docs pressure — different class
		edit(4*time.Minute, "tu4", "/w/auth.go"),        // Build pressure
		edit(5*time.Minute, "tu5", "/w/README.rst"),     // Docs pressure
		bash(6*time.Minute, "tu6", "cat /w/config.yml"), // Scout: same class as the leg
	)
	assertLegs(t, tr, legWant{journey.Scout, 1 * time.Minute, 6 * time.Minute, 6, true})
}

// Same-class weak votes never split, however many arrive.
func TestT20SameClassWeakVotesNeverSplit(t *testing.T) {
	var evs []transcript.Event
	for i := 1; i <= 8; i++ {
		evs = append(evs, edit(time.Duration(i)*time.Minute, fmt.Sprintf("tu%d", i), "/w/auth.go"))
	}
	assertLegs(t, segment(evs...), legWant{journey.Build, 1 * time.Minute, 8 * time.Minute, 8, true})
}

// ---------------------------------------------------------------- T21

// T21 — a strong vote (Test/Ship/Design) whose class differs from the open leg
// splits on the spot, with no hysteresis.
func TestT21StrongVoteSplitsImmediately(t *testing.T) {
	strong := []struct {
		name  string
		ev    transcript.Event
		class journey.Class
	}{
		{"Test", bash(3*time.Minute, "tu3", "pytest tests/auth -x"), journey.Test},
		{"Ship", bash(3*time.Minute, "tu3", "git push origin main"), journey.Ship},
		{"Design", use(3*time.Minute, "tu3", "ExitPlanMode", rawJSON(map[string]string{"plan": "do it"})), journey.Design},
	}
	for _, tc := range strong {
		t.Run(tc.name, func(t *testing.T) {
			tr := segment(
				edit(1*time.Minute, "tu1", "/w/auth.go"), // Build leg opens
				edit(2*time.Minute, "tu2", "/w/auth.go"),
				tc.ev, // one strong vote is enough
			)
			assertLegs(t, tr,
				legWant{journey.Build, 1 * time.Minute, 2 * time.Minute, 2, false},
				legWant{tc.class, 3 * time.Minute, 3 * time.Minute, 1, true},
			)
		})
	}
}

// A strong vote of the SAME class as the open leg is not a boundary.
func TestT21StrongVoteOfSameClassFolds(t *testing.T) {
	tr := segment(
		bash(1*time.Minute, "tu1", "pytest tests/auth -x"),
		bash(2*time.Minute, "tu2", "pytest tests/auth -x --lf"),
		bash(3*time.Minute, "tu3", "go test ./..."),
	)
	assertLegs(t, tr, legWant{journey.Test, 1 * time.Minute, 3 * time.Minute, 3, true})
}

// Weak pressure that has not yet reached three is discarded by an intervening
// strong split: the strong vote closes the leg the pressure was pushing on.
func TestT21StrongVoteAfterWeakPressure(t *testing.T) {
	tr := segment(
		read(1*time.Minute, "tu1", "/w/auth.go"), // Scout leg
		edit(2*time.Minute, "tu2", "/w/auth.go"), // Build pressure 1
		edit(3*time.Minute, "tu3", "/w/auth.go"), // Build pressure 2
		bash(4*time.Minute, "tu4", "go test ./..."),
	)
	assertLegs(t, tr,
		legWant{journey.Scout, 1 * time.Minute, 3 * time.Minute, 3, false},
		legWant{journey.Test, 4 * time.Minute, 4 * time.Minute, 1, true},
	)
}

// ---------------------------------------------------------------- T22

// T22 — Fix upgrade (a): an IsError tool_result observed while a Build leg is
// open retroactively reclassifies the WHOLE leg as Fix.
func TestT22FixUpgradeErrorDuringBuildLeg(t *testing.T) {
	tr := segment(
		edit(1*time.Minute, "tu1", "/w/parser.go"), // Build leg opens
		edit(2*time.Minute, "tu2", "/w/parser.go"),
		result(3*time.Minute, "tu2", true), // the compiler said no
	)
	// The error is not a vote: it moves neither End nor Votes.
	assertLegs(t, tr, legWant{journey.Fix, 1 * time.Minute, 2 * time.Minute, 2, true})
}

// The upgrade sticks and stays a single leg as the work continues.
func TestT22FixUpgradeSurvivesLaterVotes(t *testing.T) {
	tr := segment(
		edit(1*time.Minute, "tu1", "/w/parser.go"),
		result(2*time.Minute, "tu1", true),
		edit(3*time.Minute, "tu2", "/w/parser.go"),
	)
	if len(tr.Legs) != 1 {
		t.Fatalf("got %d legs, want 1 (a Build vote is not a boundary for the leg it belongs to):\n%s",
			len(tr.Legs), dumpLegs(tr))
	}
	if tr.Legs[0].Class != journey.Fix {
		t.Errorf("Legs[0].Class = %v, want fix", tr.Legs[0].Class)
	}
}

// Only a Build leg is upgraded — rule 5 says nothing about the other classes.
func TestT22FixUpgradeOnlyAppliesToBuildLegs(t *testing.T) {
	t.Run("scout leg stays scout", func(t *testing.T) {
		tr := segment(
			read(1*time.Minute, "tu1", "/w/auth.go"),
			result(2*time.Minute, "tu1", true),
		)
		assertLegs(t, tr, legWant{journey.Scout, 1 * time.Minute, 1 * time.Minute, 1, true})
	})

	t.Run("docs leg stays docs", func(t *testing.T) {
		tr := segment(
			edit(1*time.Minute, "tu1", "/w/docs/spec.md"),
			result(2*time.Minute, "tu1", true),
		)
		assertLegs(t, tr, legWant{journey.Docs, 1 * time.Minute, 1 * time.Minute, 1, true})
	})
}

// A clean tool_result upgrades nothing.
func TestT22CleanResultDoesNotUpgrade(t *testing.T) {
	tr := segment(
		edit(1*time.Minute, "tu1", "/w/parser.go"),
		result(2*time.Minute, "tu1", false),
		edit(3*time.Minute, "tu2", "/w/parser.go"),
	)
	assertLegs(t, tr, legWant{journey.Build, 1 * time.Minute, 3 * time.Minute, 2, true})
}

// ---------------------------------------------------------------- T23

// T23 — Fix upgrade (b): a Build leg that immediately follows a Test leg during
// which an IsError result was observed is a Fix leg.
func TestT23FixUpgradeAfterFailingTestLeg(t *testing.T) {
	tr := segment(
		bash(1*time.Minute, "tu1", "pytest tests/auth -x"), // Test leg opens
		result(2*time.Minute, "tu1", true),                 // the suite failed
		edit(3*time.Minute, "tu2", "/w/auth.go"),           // Build 1 of 3
		edit(4*time.Minute, "tu3", "/w/auth.go"),           // Build 2 of 3
		edit(5*time.Minute, "tu4", "/w/auth.go"),           // Build 3 of 3 → split
	)
	assertLegs(t, tr,
		legWant{journey.Test, 1 * time.Minute, 1 * time.Minute, 1, false},
		legWant{journey.Fix, 3 * time.Minute, 5 * time.Minute, 3, true},
	)
}

// The negative: after a CLEAN Test leg the same edits are plain Build.
func TestT23BuildAfterCleanTestLegStaysBuild(t *testing.T) {
	tr := segment(
		bash(1*time.Minute, "tu1", "pytest tests/auth -x"),
		result(2*time.Minute, "tu1", false), // green
		edit(3*time.Minute, "tu2", "/w/auth.go"),
		edit(4*time.Minute, "tu3", "/w/auth.go"),
		edit(5*time.Minute, "tu4", "/w/auth.go"),
	)
	assertLegs(t, tr,
		legWant{journey.Test, 1 * time.Minute, 1 * time.Minute, 1, false},
		legWant{journey.Build, 3 * time.Minute, 5 * time.Minute, 3, true},
	)
}

// "Immediately follows": a leg of another class in between breaks the chain.
func TestT23FixUpgradeNeedsTheTestLegImmediatelyBefore(t *testing.T) {
	tr := segment(
		bash(1*time.Minute, "tu1", "pytest tests/auth -x"), // Test leg
		result(2*time.Minute, "tu1", true),                 // failed
		bash(3*time.Minute, "tu2", "git push origin main"), // Ship leg (strong split)
		edit(4*time.Minute, "tu3", "/w/auth.go"),           // Build 1 of 3
		edit(5*time.Minute, "tu4", "/w/auth.go"),           // Build 2 of 3
		edit(6*time.Minute, "tu5", "/w/auth.go"),           // Build 3 of 3 → split
	)
	assertLegs(t, tr,
		legWant{journey.Test, 1 * time.Minute, 1 * time.Minute, 1, false},
		legWant{journey.Ship, 3 * time.Minute, 3 * time.Minute, 1, false},
		legWant{journey.Build, 4 * time.Minute, 6 * time.Minute, 3, true},
	)
}

// ---------------------------------------------------------------- T24

// T24 — a substantive user prompt is a boundary: it lands in Prompts and closes
// whatever leg was open, leaving no leg current until the next vote.
func TestT24PromptsAreBoundaries(t *testing.T) {
	const second = "please also cover the refresh path, and make the error message say which token expired"

	s := journey.NewSegmenter()
	for _, ev := range []transcript.Event{
		prompt(0, "add caching to the token store"),
		read(1*time.Minute, "tu1", "/w/token.go"),
		read(2*time.Minute, "tu2", "/w/cache.go"),
		prompt(3*time.Minute, second),
	} {
		s.Observe(ev)
	}

	// The leg is closed and nothing is open between the prompt and the next vote.
	tr := s.Trail()
	assertLegs(t, tr, legWant{journey.Scout, 1 * time.Minute, 2 * time.Minute, 2, false})

	if len(tr.Prompts) != 2 {
		t.Fatalf("got %d prompts, want 2: %+v", len(tr.Prompts), tr.Prompts)
	}
	if tr.Prompts[0].Text != "add caching to the token store" {
		t.Errorf("Prompts[0].Text = %q, want the prompt verbatim", tr.Prompts[0].Text)
	}
	if !tr.Prompts[0].At.Equal(base) {
		t.Errorf("Prompts[0].At = %v, want %v", tr.Prompts[0].At, base)
	}
	if !tr.Prompts[1].At.Equal(at(3 * time.Minute)) {
		t.Errorf("Prompts[1].At = %v, want %v", tr.Prompts[1].At, at(3*time.Minute))
	}
	assertPromptClipped(t, tr.Prompts[1].Text, second)

	// The next vote opens a fresh leg.
	s.Observe(edit(4*time.Minute, "tu3", "/w/token.go"))
	assertLegs(t, s.Trail(),
		legWant{journey.Scout, 1 * time.Minute, 2 * time.Minute, 2, false},
		legWant{journey.Build, 4 * time.Minute, 4 * time.Minute, 1, true},
	)
}

// assertPromptClipped checks the Prompt.Text rule: first line, ≤60 runes, "…"
// when it had to cut.
func assertPromptClipped(t *testing.T, got, full string) {
	t.Helper()
	if strings.ContainsAny(got, "\n\r") {
		t.Errorf("Prompt.Text = %q, want a single line", got)
	}
	if n := utf8.RuneCountInString(got); n > 60 {
		t.Errorf("Prompt.Text is %d runes (%q), want at most 60", n, got)
	}
	first := full
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	if utf8.RuneCountInString(first) <= 60 {
		if got != first {
			t.Errorf("Prompt.Text = %q, want the first line untouched (%q)", got, first)
		}
		return
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("Prompt.Text = %q, want it to end with %q when cut", got, "…")
	}
	// A byte-wise cut of a multibyte prompt would come back far short of 60.
	if n := utf8.RuneCountInString(got); n < 55 {
		t.Errorf("Prompt.Text is only %d runes (%q); the budget is 60 RUNES, not bytes", n, got)
	}
	head := strings.TrimSuffix(got, "…")
	if !strings.HasPrefix(first, strings.TrimRight(head, " ")) {
		t.Errorf("Prompt.Text = %q, want a prefix of %q", got, first)
	}
}

// The 60-rune budget is counted in runes and applies to the first line only.
func TestT24PromptTruncation(t *testing.T) {
	sixty := strings.Repeat("a", 60)
	tests := []struct {
		name string
		text string
	}{
		{"exactly 60 runes is untouched", sixty},
		{"61 runes is cut", sixty + "b"},
		{"75 multibyte runes are cut by rune, not byte", strings.Repeat("é", 75)},
		{"cjk first line", strings.Repeat("認", 80)},
		{"long first line with a tail below", strings.Repeat("z", 90) + "\nsecond line"},
		{"short first line with a long tail below", "rename the tailer\n" + strings.Repeat("q", 200)},
		{"leading and trailing space", "   tidy up the mirror panel   "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := segment(prompt(0, tc.text))
			if len(tr.Prompts) != 1 {
				t.Fatalf("got %d prompts, want 1", len(tr.Prompts))
			}
			got := tr.Prompts[0].Text
			if strings.ContainsAny(got, "\n\r") {
				t.Errorf("Prompt.Text = %q, want a single line", got)
			}
			if n := utf8.RuneCountInString(got); n > 60 {
				t.Errorf("Prompt.Text is %d runes (%q), want at most 60", n, got)
			}
			first := tc.text
			if i := strings.IndexByte(first, '\n'); i >= 0 {
				first = first[:i]
			}
			if utf8.RuneCountInString(strings.TrimSpace(first)) <= 60 {
				if strings.Contains(got, "…") {
					t.Errorf("Prompt.Text = %q, want no ellipsis: it fits", got)
				}
				return
			}
			if !strings.HasSuffix(got, "…") {
				t.Errorf("Prompt.Text = %q, want a trailing %q", got, "…")
			}
			if n := utf8.RuneCountInString(got); n < 55 {
				t.Errorf("Prompt.Text is only %d runes (%q); 60 is a RUNE budget", n, got)
			}
		})
	}
}

// Only substantive user prompts are boundaries. Tool results, blank lines,
// attachments and queue operations leave the open leg alone.
func TestT24NonSubstantiveUserEventsAreNotPrompts(t *testing.T) {
	tr := segment(
		read(1*time.Minute, "tu1", "/w/auth.go"),
		result(2*time.Minute, "tu1", false),
		prompt(3*time.Minute, "   \n\t "), // whitespace only
		transcript.Event{Type: transcript.EventAttachment, Timestamp: at(4 * time.Minute), Text: "image"},
		transcript.Event{Type: transcript.EventQueueOp, Timestamp: at(5 * time.Minute), Text: "queued: run the tests"},
		transcript.Event{Type: transcript.EventUnknown, Timestamp: at(6 * time.Minute), Text: "summary"},
		read(7*time.Minute, "tu2", "/w/token.go"),
	)
	if len(tr.Prompts) != 0 {
		t.Errorf("Prompts = %+v, want none: no substantive user prompt happened", tr.Prompts)
	}
	assertLegs(t, tr, legWant{journey.Scout, 1 * time.Minute, 7 * time.Minute, 2, true})
}

// A prompt arriving with no leg open is still recorded.
func TestT24PromptsBeforeAnyLeg(t *testing.T) {
	tr := segment(
		prompt(0, "look at the tailer"),
		prompt(1*time.Minute, "actually, look at the segmenter"),
	)
	if len(tr.Prompts) != 2 {
		t.Fatalf("got %d prompts, want 2: %+v", len(tr.Prompts), tr.Prompts)
	}
	if len(tr.Legs) != 0 {
		t.Errorf("got %d legs, want 0:\n%s", len(tr.Legs), dumpLegs(tr))
	}
}

// ---------------------------------------------------------------- T25

// Harness envelopes — scheduled wakes and task notifications arrive as user
// turns, but nobody asked anything: no ◉ node, no leg boundary (M4, found by
// running compass against its own build session).
func TestEnvelopeTurnsAreNotPrompts(t *testing.T) {
	tr := segment(
		prompt(0, "fix the failing test"),
		edit(1*time.Minute, "tu1", "/w/auth.go"),
		prompt(2*time.Minute, "<system-reminder>\n[SYSTEM NOTIFICATION - NOT USER INPUT]\n…"),
		prompt(3*time.Minute, "<task-notification>\n<task-id>abc</task-id>"),
		prompt(4*time.Minute, `<wake reason="external-event">…</wake>`),
		edit(5*time.Minute, "tu2", "/w/auth.go"),
	)
	if len(tr.Prompts) != 1 {
		t.Fatalf("Prompts = %d, want only the human one", len(tr.Prompts))
	}
	assertLegs(t, tr, legWant{journey.Build, 1 * time.Minute, 5 * time.Minute, 2, true})
}

// T25 — an Agent tool_use forks a branch off the open leg without splitting it,
// and its tool_result closes the branch.
func TestT25BranchForksWithoutSplittingTheLeg(t *testing.T) {
	s := journey.NewSegmenter()
	for _, ev := range []transcript.Event{
		read(1*time.Minute, "tu1", "/w/auth.go"), // Scout leg, index 0
		read(2*time.Minute, "tu2", "/w/token.go"),
		agent(3*time.Minute, "tu_agent", "scout payment flows"),
	} {
		s.Observe(ev)
	}

	tr := s.Trail()
	// The Agent did not vote and did not close the leg.
	assertLegs(t, tr, legWant{journey.Scout, 1 * time.Minute, 2 * time.Minute, 2, true})

	if len(tr.Branches) != 1 {
		t.Fatalf("got %d branches, want 1: %+v", len(tr.Branches), tr.Branches)
	}
	br := tr.Branches[0]
	if br.ToolUseID != "tu_agent" {
		t.Errorf("Branch.ToolUseID = %q, want %q", br.ToolUseID, "tu_agent")
	}
	if br.Label != "scout payment flows" {
		t.Errorf("Branch.Label = %q, want %q", br.Label, "scout payment flows")
	}
	if !br.Start.Equal(at(3 * time.Minute)) {
		t.Errorf("Branch.Start = %v, want %v", br.Start, at(3*time.Minute))
	}
	if br.AfterLeg != 0 {
		t.Errorf("Branch.AfterLeg = %d, want 0 (the leg open at the fork)", br.AfterLeg)
	}
	if br.Done {
		t.Errorf("Branch.Done = true, want false: no tool_result yet")
	}
	if !br.End.IsZero() {
		t.Errorf("Branch.End = %v, want the zero time until Done", br.End)
	}

	// Someone else's tool_result must not close this branch.
	s.Observe(result(4*time.Minute, "tu2", false))
	if s.Trail().Branches[0].Done {
		t.Fatalf("an unrelated tool_result marked the branch done")
	}

	// Its own result does.
	s.Observe(result(5*time.Minute, "tu_agent", false))
	br = s.Trail().Branches[0]
	if !br.Done {
		t.Errorf("Branch.Done = false after its tool_result, want true")
	}
	if !br.End.Equal(at(5 * time.Minute)) {
		t.Errorf("Branch.End = %v, want %v", br.End, at(5*time.Minute))
	}
	// Still one leg, still open, still two votes.
	assertLegs(t, s.Trail(), legWant{journey.Scout, 1 * time.Minute, 2 * time.Minute, 2, true})
}

// The Label falls back to "agent" when the Agent input names no description.
func TestT25BranchLabelFallsBackToAgent(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"no description field", []byte(`{"prompt":"go and look"}`)},
		{"empty description", []byte(`{"description":"","prompt":"go and look"}`)},
		{"empty input object", []byte(`{}`)},
		{"nil input", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := segment(
				read(1*time.Minute, "tu1", "/w/auth.go"),
				use(2*time.Minute, "tu_agent", "Agent", tc.input),
			)
			if len(tr.Branches) != 1 {
				t.Fatalf("got %d branches, want 1: %+v", len(tr.Branches), tr.Branches)
			}
			if tr.Branches[0].Label != "agent" {
				t.Errorf("Branch.Label = %q, want %q", tr.Branches[0].Label, "agent")
			}
		})
	}
}

// A fork before any leg exists records AfterLeg -1, and later legs do not
// retroactively adopt it.
func TestT25BranchAfterLegMinusOneBeforeAnyLeg(t *testing.T) {
	tr := segment(
		prompt(0, "find out how payments work"),
		agent(1*time.Minute, "tu_agent", "scout payment flows"),
		read(2*time.Minute, "tu1", "/w/auth.go"), // leg 0 opens AFTER the fork
	)
	if len(tr.Branches) != 1 {
		t.Fatalf("got %d branches, want 1: %+v", len(tr.Branches), tr.Branches)
	}
	if tr.Branches[0].AfterLeg != -1 {
		t.Errorf("Branch.AfterLeg = %d, want -1 (no leg was open at the fork)", tr.Branches[0].AfterLeg)
	}
}

// AfterLeg indexes the leg that was open at the fork, not the first or last one.
func TestT25BranchAfterLegIndexesTheOpenLeg(t *testing.T) {
	tr := segment(
		read(1*time.Minute, "tu1", "/w/auth.go"),           // leg 0: scout
		agent(2*time.Minute, "tu_a1", "scout payment"),     // fork off leg 0
		bash(3*time.Minute, "tu2", "go test ./..."),        // leg 1: test (strong split)
		agent(4*time.Minute, "tu_a2", "check flaky suite"), // fork off leg 1
	)
	if len(tr.Branches) != 2 {
		t.Fatalf("got %d branches, want 2: %+v", len(tr.Branches), tr.Branches)
	}
	if tr.Branches[0].AfterLeg != 0 {
		t.Errorf("Branches[0].AfterLeg = %d, want 0", tr.Branches[0].AfterLeg)
	}
	if tr.Branches[1].AfterLeg != 1 {
		t.Errorf("Branches[1].AfterLeg = %d, want 1", tr.Branches[1].AfterLeg)
	}
	// Oldest first.
	if tr.Branches[0].ToolUseID != "tu_a1" || tr.Branches[1].ToolUseID != "tu_a2" {
		t.Errorf("Branches out of order: %q then %q", tr.Branches[0].ToolUseID, tr.Branches[1].ToolUseID)
	}
	assertLegs(t, tr,
		legWant{journey.Scout, 1 * time.Minute, 1 * time.Minute, 1, false},
		legWant{journey.Test, 3 * time.Minute, 3 * time.Minute, 1, true},
	)
}

// ---------------------------------------------------------------- T26

// T26 — Label heuristics, rule 7.
func TestT26LabelHeuristics(t *testing.T) {
	tests := []struct {
		name string
		evs  []transcript.Event
		want string
	}{
		{
			"most frequent file wins",
			[]transcript.Event{
				edit(1*time.Minute, "tu1", "/w/internal/lexer.go"),
				edit(2*time.Minute, "tu2", "/w/internal/parser.go"),
				edit(3*time.Minute, "tu3", "/w/internal/parser.go"),
			},
			"parser.go",
		},
		{
			"tie goes to first seen",
			[]transcript.Event{
				edit(1*time.Minute, "tu1", "/w/internal/alpha.go"),
				edit(2*time.Minute, "tu2", "/w/internal/beta.go"),
			},
			"alpha.go",
		},
		{
			"reads count as files too",
			[]transcript.Event{
				read(1*time.Minute, "tu1", "/w/token.go"),
				read(2*time.Minute, "tu2", "/w/cache.go"),
				read(3*time.Minute, "tu3", "/w/token.go"),
			},
			"token.go",
		},
		{
			"test leg with no files uses the runner's first word: pytest",
			[]transcript.Event{bash(1*time.Minute, "tu1", "pytest tests/auth -x")},
			"pytest",
		},
		{
			"test leg with no files uses the runner's first word: go",
			[]transcript.Event{bash(1*time.Minute, "tu1", "go test ./... -run TestSegmenter")},
			"go",
		},
		{
			"ship leg uses the git subcommand: push",
			[]transcript.Event{bash(1*time.Minute, "tu1", "git push origin main")},
			"push",
		},
		{
			"ship leg uses the git subcommand: commit",
			[]transcript.Event{bash(1*time.Minute, "tu1", `git commit -m "wip"`)},
			"commit",
		},
		{
			"ship leg uses the gh subcommand: pr",
			[]transcript.Event{bash(1*time.Minute, "tu1", "gh pr create --fill")},
			"pr",
		},
		{
			"ship leg uses the gh subcommand: release",
			[]transcript.Event{bash(1*time.Minute, "tu1", "gh release create v1.2.0")},
			"release",
		},
		{
			"scout leg with no files has no label",
			[]transcript.Event{
				bash(1*time.Minute, "tu1", "ls -la internal/"),
				bash(2*time.Minute, "tu2", "tree -L 2"),
			},
			"",
		},
		{
			"build leg with no files has no label",
			[]transcript.Event{bash(1*time.Minute, "tu1", "mkdir -p internal/journey")},
			"",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := segment(tc.evs...)
			if len(tr.Legs) != 1 {
				t.Fatalf("got %d legs, want 1:\n%s", len(tr.Legs), dumpLegs(tr))
			}
			if got := tr.Legs[0].Label; got != tc.want {
				t.Errorf("Label = %q, want %q", got, tc.want)
			}
		})
	}
}

// A leg's Label is lowercase and bounded at 24 runes.
func TestT26LabelIsLowercaseAndBounded(t *testing.T) {
	t.Run("lowercase", func(t *testing.T) {
		tr := segment(writeFile(1*time.Minute, "tu1", "/w/README.md"))
		if len(tr.Legs) != 1 {
			t.Fatalf("got %d legs, want 1", len(tr.Legs))
		}
		if got := tr.Legs[0].Label; got != "readme.md" {
			t.Errorf("Label = %q, want %q (labels are lowercase)", got, "readme.md")
		}
	})

	t.Run("bounded at 24 runes", func(t *testing.T) {
		const long = "/w/internal/an_extremely_long_module_name_indeed.go"
		tr := segment(edit(1*time.Minute, "tu1", long))
		if len(tr.Legs) != 1 {
			t.Fatalf("got %d legs, want 1", len(tr.Legs))
		}
		got := tr.Legs[0].Label
		if n := utf8.RuneCountInString(got); n > 24 {
			t.Errorf("Label is %d runes (%q), want at most 24", n, got)
		}
		if got == "" {
			t.Errorf("Label = %q, want the (clipped) basename", got)
		}
	})
}

// Files: distinct basenames, first-seen order, capped at 5.
func TestT26FilesAreDistinctBasenamesCappedAtFive(t *testing.T) {
	tr := segment(
		edit(1*time.Minute, "tu1", "/w/a/one.go"),
		edit(2*time.Minute, "tu2", "/w/b/two.go"),
		edit(3*time.Minute, "tu3", "/w/a/one.go"), // repeat: still one entry
		edit(4*time.Minute, "tu4", "/w/c/three.go"),
		edit(5*time.Minute, "tu5", "/w/d/four.go"),
		edit(6*time.Minute, "tu6", "/w/e/five.go"),
		edit(7*time.Minute, "tu7", "/w/f/six.go"),   // over the cap
		edit(8*time.Minute, "tu8", "/w/g/seven.go"), // over the cap
	)
	if len(tr.Legs) != 1 {
		t.Fatalf("got %d legs, want 1:\n%s", len(tr.Legs), dumpLegs(tr))
	}
	got := tr.Legs[0].Files
	want := []string{"one.go", "two.go", "three.go", "four.go", "five.go"}
	if len(got) != len(want) {
		t.Fatalf("Files = %v, want %v (cap 5, first-seen order)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Files[%d] = %q, want %q (got %v)", i, got[i], want[i], got)
		}
	}
	// Basenames only: no directory ever survives.
	for _, f := range got {
		if strings.ContainsRune(f, '/') {
			t.Errorf("Files contains a path, not a basename: %q", f)
		}
	}
	// one.go was touched twice, so it also owns the label.
	if tr.Legs[0].Label != "one.go" {
		t.Errorf("Label = %q, want %q", tr.Legs[0].Label, "one.go")
	}
}

// Files belong to the leg whose votes touched them; a split does not smear them.
func TestT26FilesFollowTheirLeg(t *testing.T) {
	tr := segment(
		read(1*time.Minute, "tu1", "/w/token.go"),   // scout leg
		bash(2*time.Minute, "tu2", "go test ./..."), // strong split → test leg
		edit(3*time.Minute, "tu3", "/w/cache.go"),   // build pressure inside the test leg
		edit(4*time.Minute, "tu4", "/w/cache.go"),
		edit(5*time.Minute, "tu5", "/w/cache.go"), // third consecutive → build leg
	)
	assertLegs(t, tr,
		legWant{journey.Scout, 1 * time.Minute, 1 * time.Minute, 1, false},
		legWant{journey.Test, 2 * time.Minute, 2 * time.Minute, 1, false},
		legWant{journey.Build, 3 * time.Minute, 5 * time.Minute, 3, true},
	)
	if got := strings.Join(tr.Legs[0].Files, ","); got != "token.go" {
		t.Errorf("Legs[0].Files = %q, want %q", got, "token.go")
	}
	if len(tr.Legs[1].Files) != 0 {
		t.Errorf("Legs[1].Files = %v, want none: a Bash vote touches no file_path", tr.Legs[1].Files)
	}
	if got := strings.Join(tr.Legs[2].Files, ","); got != "cache.go" {
		t.Errorf("Legs[2].Files = %q, want %q", got, "cache.go")
	}
	if tr.Legs[1].Label != "go" {
		t.Errorf("Legs[1].Label = %q, want %q", tr.Legs[1].Label, "go")
	}
}

// A short end-to-end walk: the whole rule set on one session.
func TestSegmenterFullWalk(t *testing.T) {
	tr := segment(
		prompt(0, "the auth tests are failing, please fix them"),
		read(1*time.Minute, "tu1", "/w/auth.go"),
		read(2*time.Minute, "tu2", "/w/token.go"),
		agent(3*time.Minute, "tu_agent", "scout payment flows"),
		bash(4*time.Minute, "tu3", "pytest tests/auth -x"), // strong → test leg
		result(5*time.Minute, "tu3", true),                 // it failed
		edit(6*time.Minute, "tu4", "/w/auth.go"),
		edit(7*time.Minute, "tu5", "/w/auth.go"),
		edit(8*time.Minute, "tu6", "/w/auth.go"), // three consecutive → fix leg
		result(9*time.Minute, "tu_agent", false),
		bash(10*time.Minute, "tu7", "git commit -m fix"), // strong → ship leg
	)

	assertLegs(t, tr,
		legWant{journey.Scout, 1 * time.Minute, 2 * time.Minute, 2, false},
		legWant{journey.Test, 4 * time.Minute, 4 * time.Minute, 1, false},
		legWant{journey.Fix, 6 * time.Minute, 8 * time.Minute, 3, false},
		legWant{journey.Ship, 10 * time.Minute, 10 * time.Minute, 1, true},
	)
	if len(tr.Prompts) != 1 {
		t.Errorf("got %d prompts, want 1", len(tr.Prompts))
	}
	if len(tr.Branches) != 1 || !tr.Branches[0].Done || tr.Branches[0].AfterLeg != 0 {
		t.Errorf("Branches = %+v, want one done branch off leg 0", tr.Branches)
	}
	if tr.Legs[3].Label != "commit" {
		t.Errorf("Legs[3].Label = %q, want %q", tr.Legs[3].Label, "commit")
	}
	// Legs are in time order and never overlap.
	for i := 1; i < len(tr.Legs); i++ {
		if tr.Legs[i].Start.Before(tr.Legs[i-1].End) {
			t.Errorf("Legs[%d].Start %v is before Legs[%d].End %v", i, tr.Legs[i].Start, i-1, tr.Legs[i-1].End)
		}
		if tr.Legs[i].End.Before(tr.Legs[i].Start) {
			t.Errorf("Legs[%d] ends (%v) before it starts (%v)", i, tr.Legs[i].End, tr.Legs[i].Start)
		}
	}
}
