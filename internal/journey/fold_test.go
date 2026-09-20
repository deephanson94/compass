package journey_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/transcript"
)

// foldScenario is the shapes the fixtures under testdata/scenarios do not
// reach mid-flight: a pressure streak two votes in, a test runner whose
// result is still out, a TaskCreate waiting for its id, a lane still open —
// each of which a split lands inside.
func foldScenario() []transcript.Event {
	m := time.Minute
	evs := []transcript.Event{
		prompt(0, "fix the 401 bug"),
		read(1*m, "r1", "/w/auth.go"),
		read(2*m, "r2", "/w/token.go"),
		edit(3*m, "e1", "/w/auth.go"),  // pressure 1 of 3
		edit(4*m, "e2", "/w/auth.go"),  // pressure 2 of 3
		read(5*m, "r3", "/w/token.go"), // the streak dies
		bash(6*m, "t1", "pytest -x"),   // a runner in flight
		agent(7*m, "ag1", "scout the payments module"),
		taskCreate(8*m, "tc1", "Fix the token refresh", "Fixing the token refresh"),
		resultText(9*m, "t1", "3 passed, 1 failed"),
		taskCreated(10*m, "tc1", "1", "Fix the token refresh"),
		edit(11*m, "e3", "/w/store.go"), // pressure 1 of 3
		edit(12*m, "e4", "/w/store.go"), // 2 of 3
		edit(13*m, "e5", "/w/store.go"), // 3 of 3: the leg splits
		bash(14*m, "s1", "git commit -m done"),
		resultText(15*m, "s1", "[main abc1234] done"),
		resultText(16*m, "ag1", "payments uses stripe"),
		prompt(17*m, "now the other one"),
		bash(18*m, "t2", "go test ./..."),
	}
	return evs
}

// scenarioEvents reads every fixture transcript under testdata/scenarios,
// plus the synthetic one above.
func scenarioEvents(t *testing.T) map[string][]transcript.Event {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "..", "testdata", "scenarios", "*.jsonl"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no scenario fixtures: %v", err)
	}
	out := map[string][]transcript.Event{"synthetic": foldScenario()}
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 1<<20), 1<<24)
		var evs []transcript.Event
		for sc.Scan() {
			if ev, err := transcript.ParseLine(sc.Bytes()); err == nil {
				evs = append(evs, ev)
			}
		}
		f.Close()
		out[filepath.Base(p)] = evs
	}
	return out
}

// A segmenter restored from a fold taken at any point of a transcript, fed
// the rest, reaches exactly the trail a full replay does — through JSON, as
// the resume cache carries it.
func TestSegmenterFoldRoundTrip(t *testing.T) {
	// What the splits exercised: a fixture set whose folds never carry a
	// pressure streak or a runner in flight would pass against a Fold that
	// drops them.
	var sawPress, sawRunners, sawOpen, sawCreate int
	for name, evs := range scenarioEvents(t) {
		full := journey.NewSegmenter()
		for _, ev := range evs {
			full.Observe(ev)
		}
		want := full.Trail()
		for k := 0; k <= len(evs); k++ {
			head := journey.NewSegmenter()
			for _, ev := range evs[:k] {
				head.Observe(ev)
			}
			raw, err := json.Marshal(head.Fold())
			if err != nil {
				t.Fatalf("%s@%d: marshal: %v", name, k, err)
			}
			var f journey.Fold
			if err := json.Unmarshal(raw, &f); err != nil {
				t.Fatalf("%s@%d: unmarshal: %v", name, k, err)
			}
			if len(f.Press) > 0 {
				sawPress++
			}
			if len(f.Runners) > 0 {
				sawRunners++
			}
			if f.Open {
				sawOpen++
			}
			if len(f.ByCreate) > 0 {
				sawCreate++
			}
			tail := journey.RestoreSegmenter(f)
			for _, ev := range evs[k:] {
				tail.Observe(ev)
			}
			if got := tail.Trail(); !reflect.DeepEqual(got, want) {
				t.Fatalf("%s: trail restored at event %d of %d differs from a full replay\n got: %+v\nwant: %+v", name, k, len(evs), got, want)
			}
		}
	}
	if sawPress == 0 || sawRunners == 0 || sawOpen == 0 || sawCreate == 0 {
		t.Fatalf("the fixtures left part of the fold untested: press %d, runners %d, open %d, creates %d", sawPress, sawRunners, sawOpen, sawCreate)
	}
}

// The same for the fleet row's outcome.
func TestOutcomesFoldRoundTrip(t *testing.T) {
	for name, evs := range scenarioEvents(t) {
		full := journey.NewOutcomes()
		for _, ev := range evs {
			full.Observe(ev)
		}
		want, wantOK := full.Latest()
		for k := 0; k <= len(evs); k++ {
			head := journey.NewOutcomes()
			for _, ev := range evs[:k] {
				head.Observe(ev)
			}
			raw, err := json.Marshal(head.Fold())
			if err != nil {
				t.Fatal(err)
			}
			var f journey.OutcomesFold
			if err := json.Unmarshal(raw, &f); err != nil {
				t.Fatal(err)
			}
			tail := journey.RestoreOutcomes(f)
			for _, ev := range evs[k:] {
				tail.Observe(ev)
			}
			got, ok := tail.Latest()
			if ok != wantOK || got != want {
				t.Fatalf("%s: outcome restored at %d: got %+v/%v want %+v/%v", name, k, got, ok, want, wantOK)
			}
		}
	}
}

// A fold of an empty segmenter restores to one that behaves as new, and a
// fold's JSON does not churn between two takes of the same state.
func TestSegmenterFoldStable(t *testing.T) {
	a, _ := json.Marshal(journey.NewSegmenter().Fold())
	b, _ := json.Marshal(journey.RestoreSegmenter(journey.Fold{}).Fold())
	if string(a) != string(b) {
		t.Fatalf("empty folds differ: %s vs %s", a, b)
	}
	evs := scenarioEvents(t)["t18-teammates.jsonl"]
	s := journey.NewSegmenter()
	for _, ev := range evs {
		s.Observe(ev)
	}
	x, _ := json.Marshal(s.Fold())
	y, _ := json.Marshal(s.Fold())
	if string(x) != string(y) {
		t.Fatal("two folds of one state differ")
	}
}
