package narrator_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/narrator"
)

// The session's goal (goal.go): one digest per session per prompt, riding
// the legs' batch, cached like a label.

func goalTrail(prompts ...string) journey.Trail {
	tr := twoClosedAndAHead()
	for i, p := range prompts {
		tr.Prompts = append(tr.Prompts, journey.Prompt{Text: p, At: nat(time.Duration(i) * time.Minute)})
	}
	return tr
}

const sessPath = "/x/sessions/alpha.jsonl"

var errBoom = errors.New("claude: exit status 1")

func TestGoalKeyMovesWithAPromptAndNothingElse(t *testing.T) {
	a := goalTrail("port the runner", "now the tests")
	b := a
	b.Legs = append(b.Legs[:len(b.Legs):len(b.Legs)], leg(journey.Build, 20*time.Minute, "more"))
	if narrator.GoalKey(sessPath, a) != narrator.GoalKey(sessPath, b) {
		t.Fatal("a new leg moved the goal's key: the goal would be re-asked every turn")
	}
	c := goalTrail("port the runner", "now the tests", "and ship it")
	if narrator.GoalKey(sessPath, a) == narrator.GoalKey(sessPath, c) {
		t.Fatal("a new prompt did not move the goal's key")
	}
	if narrator.GoalKey(sessPath, journey.Trail{}) != "" {
		t.Fatal("a session with no prompt has a goal key")
	}
}

func TestGoalRidesTheNextBatchAndLands(t *testing.T) {
	n, r, _, notified := newNarrator(t)
	tr := goalTrail("port the LFM2 runner to the cmdline", "fix the tests")

	if !n.RequestGoal(sessPath, tr) {
		t.Fatal("RequestGoal refused with nothing in flight")
	}
	batch := r.waitBatch(t)
	if len(batch) != 1 || batch[0].Kind != "goal" {
		t.Fatalf("the goal's batch = %+v, want one goal digest", batch)
	}
	if got := strings.Join(batch[0].Waypoints, " | "); !strings.Contains(got, "port the LFM2 runner") || !strings.Contains(got, "fix the tests") {
		t.Fatalf("the goal digest does not carry the prompts: %q", got)
	}
	r.answer(narrator.Label{Key: batch[0].Key, Text: "port LFM2 runner to cmdline"})
	waitNotify(t, notified)
	waitIdle(t, n)
	if g, ok := n.Goal(sessPath, tr); !ok || g != "port LFM2 runner to cmdline" {
		t.Fatalf("Goal = %q %v after it landed", g, ok)
	}
	// Cached: asking again is no call.
	n.RequestGoal(sessPath, tr)
	r.noBatch(t, "the goal is cached")
}

func TestGoalAskedInFlightRidesTheLegsBatch(t *testing.T) {
	n, r, _, notified := newNarrator(t)
	legs := twoClosedAndAHead()
	n.Request(sessAlpha, legs, "")
	first := r.waitBatch(t)
	// A goal asked while the legs are out waits for the next batch.
	tr := goalTrail("port the runner")
	n.RequestGoal(sessPath, tr)
	r.noBatch(t, "a batch is in flight; the goal queues")
	r.answer()
	waitIdle(t, n)
	// The next leg batch carries it.
	more := legs
	more.Legs = append(append([]journey.Leg(nil), legs.Legs[:2]...), leg(journey.Docs, 30*time.Minute, "readme"), head(leg(journey.Fix, 40*time.Minute, "x")))
	n.Request(sessBeta, more, "")
	second := r.waitBatch(t)
	kinds := map[string]int{}
	for _, d := range second {
		kinds[d.Kind]++
	}
	if kinds["goal"] != 1 || kinds[""] == 0 {
		t.Fatalf("the second batch should carry the queued goal beside the legs: %v (first %d)", kinds, len(first))
	}
	key := narrator.GoalKey(sessPath, tr)
	r.answer(narrator.Label{Key: key, Text: "port the runner"})
	waitNotify(t, notified)
	if g, ok := n.Goal(sessPath, tr); !ok || g != "port the runner" {
		t.Fatalf("Goal = %q %v", g, ok)
	}
}

func TestGoalKeepsTheLastOneWhileTheNextIsAsked(t *testing.T) {
	n, r, _, notified := newNarrator(t)
	tr := goalTrail("port the runner")
	n.RequestGoal(sessPath, tr)
	b := r.waitBatch(t)
	r.answer(narrator.Label{Key: b[0].Key, Text: "port the runner"})
	waitNotify(t, notified)
	waitIdle(t, n)
	next := goalTrail("port the runner", "and its tests")
	if g, ok := n.Goal(sessPath, next); !ok || g != "port the runner" {
		t.Fatalf("a new prompt blanked the goal until the next one lands: %q %v", g, ok)
	}
	// Another session never wears it.
	if g, ok := n.Goal("/x/sessions/beta.jsonl", next); ok {
		t.Fatalf("another session wears this session's goal: %q", g)
	}
}

func TestGoalFailureCoolsOffOneCall(t *testing.T) {
	n, r, _, _ := newNarrator(t)
	tr := goalTrail("port the runner")
	n.RequestGoal(sessPath, tr)
	r.waitBatch(t)
	r.fail(errBoom)
	waitIdle(t, n)
	if n.RequestGoal(sessPath, tr) {
		t.Fatal("a goal that just failed was asked again at once")
	}
	r.noBatch(t, "cooling off")
	n.RequestGoal(sessPath, tr)
	r.waitBatch(t) // the call after the backoff retries
	r.answer()
}

func TestGoalDigestTellsTheModelWhatItIs(t *testing.T) {
	r := &narrator.CLIRunner{}
	leg := narrator.Digest{Key: "k1", Class: "fix"}
	goal := narrator.Digest{Key: "goal/k", Kind: "goal", Class: "session", Waypoints: []string{"port the runner"}}
	args := r.Args([]narrator.Digest{leg, goal})
	prompt := args[len(args)-1]
	if !strings.Contains(prompt, `"Kind":"goal"`) || strings.Count(prompt, `"Kind"`) != 1 {
		t.Fatalf("only the goal digest should carry a Kind:\n%s", prompt)
	}
	if !strings.Contains(prompt, `Kind is "goal"`) {
		t.Fatalf("the instruction does not say what a goal entry is:\n%s", prompt)
	}
}
