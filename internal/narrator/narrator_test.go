package narrator_test

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/narrator"
)

// nbase is the single instant every narrator test builds its legs from; leg
// times are offsets from it, so nothing here depends on the wall clock.
// 2026-08-30 12:00:00Z is 1788091200000000000 in UnixNano — the LegKey tests
// spell that number out, so this constant and that literal must agree.
var nbase = time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

func nat(offset time.Duration) time.Time { return nbase.Add(offset) }

const (
	sessAlpha = "aa000001-0000-4000-8000-000000000001"
	sessBeta  = "bb000002-0000-4000-8000-000000000002"
)

// ---------------------------------------------------------------- fixtures

// leg builds one closed leg the way the segmenter would hand it over.
func leg(class journey.Class, start time.Duration, label string, files ...string) journey.Leg {
	return journey.Leg{
		Class: class,
		Label: label,
		Start: nat(start),
		End:   nat(start + 90*time.Second),
		Votes: 4,
		Files: files,
	}
}

// withWaypoints hangs Lv2 detail rows on a leg; the digest carries their texts.
func withWaypoints(l journey.Leg, texts ...string) journey.Leg {
	for i, text := range texts {
		l.Waypoints = append(l.Waypoints, journey.Waypoint{
			Kind: journey.WaypointTestRun,
			Text: text,
			At:   l.Start.Add(time.Duration(i+1) * time.Second),
		})
	}
	return l
}

// head marks a leg as HEAD — still open, never narrated.
func head(l journey.Leg) journey.Leg {
	l.Current = true
	return l
}

// twoClosedAndAHead is the T48 trail: two finished legs and the open one.
func twoClosedAndAHead() journey.Trail {
	return journey.Trail{Legs: []journey.Leg{
		withWaypoints(leg(journey.Build, 0, "auth.go", "auth.go", "token.go"), "cannot find package jwt"),
		withWaypoints(leg(journey.Test, 4*time.Minute, "pytest"), "18 passed, 2 failed", "TestRefreshRotatesTheToken"),
		head(leg(journey.Fix, 8*time.Minute, "tailer.go", "tailer.go")),
	}}
}

// ---------------------------------------------------------------- fake runner

type reply struct {
	labels []narrator.Label
	err    error
}

// fakeRunner records every batch it is handed and answers from a channel, so a
// test can hold a batch in flight for as long as it needs to.
type fakeRunner struct {
	entered chan []narrator.Digest // one send per Narrate call, on entry
	replies chan reply             // one receive per Narrate call, before return

	mu      sync.Mutex
	batches [][]narrator.Digest
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{
		entered: make(chan []narrator.Digest, 16),
		replies: make(chan reply, 16),
	}
}

func (f *fakeRunner) Narrate(digests []narrator.Digest) ([]narrator.Label, error) {
	batch := append([]narrator.Digest(nil), digests...)
	f.mu.Lock()
	f.batches = append(f.batches, batch)
	f.mu.Unlock()
	f.entered <- batch
	r := <-f.replies // blocks until the test answers
	return r.labels, r.err
}

func (f *fakeRunner) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.batches)
}

// answer queues one reply, so the next Narrate returns without blocking.
func (f *fakeRunner) answer(labels ...narrator.Label) { f.replies <- reply{labels: labels} }

func (f *fakeRunner) fail(err error) { f.replies <- reply{err: err} }

// waitBatch blocks until a batch reaches the runner, or fails the test. A
// deadlock here is a bug in the narrator, not a slow machine, so the wait is
// generous and bounded rather than a bare receive that would hang the suite.
func (f *fakeRunner) waitBatch(t *testing.T) []narrator.Digest {
	t.Helper()
	timer := time.NewTimer(longWait(t))
	defer timer.Stop()
	select {
	case b := <-f.entered:
		return b
	case <-timer.C:
		t.Fatalf("no batch reached the runner within %v (calls so far: %d)", longWait(t), f.calls())
		return nil
	}
}

// noBatch asserts that no batch reaches the runner in a short window. It is
// also the barrier that lets the following steps be deterministic: by the time
// it returns, any goroutine Request may have spawned has had its chance to run.
func (f *fakeRunner) noBatch(t *testing.T, why string) {
	t.Helper()
	timer := time.NewTimer(settle)
	defer timer.Stop()
	select {
	case b := <-f.entered:
		t.Fatalf("the runner was called with %d digests, want no call: %s\n%s", len(b), why, keysOf(b))
	case <-timer.C:
	}
}

// settle is how long a "this must not happen" assertion waits. Everything in
// these tests is in-process, so a batch that is coming at all arrives in
// microseconds.
const settle = 200 * time.Millisecond

// longWait is the ceiling on a positive wait, kept inside the test's own
// deadline so a hang reports as a failure instead of killing the package.
func longWait(t *testing.T) time.Duration {
	const want = 5 * time.Second
	d, ok := t.Deadline()
	if !ok {
		return want
	}
	left := time.Until(d) - 500*time.Millisecond
	if left < want {
		if left < 50*time.Millisecond {
			left = 50 * time.Millisecond
		}
		return left
	}
	return want
}

func keysOf(digests []narrator.Digest) []string {
	out := make([]string, 0, len(digests))
	for _, d := range digests {
		out = append(out, d.Key)
	}
	return out
}

func sortedKeys(digests []narrator.Digest) []string {
	out := keysOf(digests)
	sort.Strings(out)
	return out
}

func assertBatchKeys(t *testing.T, got []narrator.Digest, want ...string) {
	t.Helper()
	sort.Strings(want)
	if g := sortedKeys(got); strings.Join(g, "\n") != strings.Join(want, "\n") {
		t.Fatalf("batch keys =\n  %s\nwant\n  %s", strings.Join(g, "\n  "), strings.Join(want, "\n  "))
	}
}

// notifier returns a notify func and the channel it pokes. The send is
// non-blocking: notify must never be able to wedge the narrator's goroutine.
func notifier() (func(), chan struct{}) {
	ch := make(chan struct{}, 16)
	return func() {
		select {
		case ch <- struct{}{}:
		default:
		}
	}, ch
}

// waitIdle blocks until no batch is in flight — the barrier between answering
// (or failing) a batch and the next Request that must see its bookkeeping.
func waitIdle(t *testing.T, n *narrator.Narrator) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !n.Idle() {
		if time.Now().After(deadline) {
			t.Fatal("narrator never went idle")
		}
		time.Sleep(time.Millisecond)
	}
}

func waitNotify(t *testing.T, ch chan struct{}) {
	t.Helper()
	timer := time.NewTimer(longWait(t))
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
		t.Fatalf("notify was not called within %v after labels landed", longWait(t))
	}
}

func newNarrator(t *testing.T) (*narrator.Narrator, *fakeRunner, *narrator.Cache, chan struct{}) {
	t.Helper()
	r := newFakeRunner()
	c := openCache(t, cachePath(t))
	notify, notified := notifier()
	return narrator.New(r, c, notify), r, c, notified
}

// ---------------------------------------------------------------- LegKey

// LegKey is the identity everything else hangs off: the cache key, the dedupe
// key and the map key the renderer looks labels up by. Its format is spelled
// out in the contract, so it is spelled out here too.
func TestLegKeyFormat(t *testing.T) {
	l := leg(journey.Fix, 0, "auth.go")
	const want = "aa000001-0000-4000-8000-000000000001/1788091200000000000/fix"
	if got := narrator.LegKey(sessAlpha, l); got != want {
		t.Errorf("LegKey = %q, want %q", got, want)
	}

	// And structurally, so the literal above cannot drift silently. The key is
	// read from the RIGHT: since M6 the session part is a transcript path, which
	// contains slashes of its own, so counting "/"-separated fields would only
	// ever be true of the uuid this subtest happens to pass.
	got := narrator.LegKey(sessAlpha, l)
	class := got[strings.LastIndexByte(got, '/')+1:]
	rest := got[:strings.LastIndexByte(got, '/')]
	nanos := rest[strings.LastIndexByte(rest, '/')+1:]
	session := rest[:strings.LastIndexByte(rest, '/')]

	if session != sessAlpha {
		t.Errorf("LegKey session part = %q, want %q", session, sessAlpha)
	}
	if nanos != strconv.FormatInt(l.Start.UnixNano(), 10) {
		t.Errorf("LegKey time part = %q, want Start.UnixNano() base 10 (%d)", nanos, l.Start.UnixNano())
	}
	if class != l.Class.String() {
		t.Errorf("LegKey class part = %q, want %q", class, l.Class.String())
	}
}

// Two legs that started at the same instant in different classes are two legs.
// Collapsing them would make one steal the other's narration.
func TestLegKeyDistinguishesClassAtTheSameInstant(t *testing.T) {
	build := journey.Leg{Class: journey.Build, Start: nat(0)}
	fix := journey.Leg{Class: journey.Fix, Start: nat(0)}
	test := journey.Leg{Class: journey.Test, Start: nat(0)}

	keys := map[string]string{
		narrator.LegKey(sessAlpha, build): "build",
		narrator.LegKey(sessAlpha, fix):   "fix",
		narrator.LegKey(sessAlpha, test):  "test",
	}
	if len(keys) != 3 {
		t.Errorf("three same-instant legs of different classes produced %d distinct keys, want 3: %v", len(keys), keys)
	}
}

func TestLegKeyDistinguishesSessionAndStart(t *testing.T) {
	l := leg(journey.Build, 0, "auth.go")
	later := leg(journey.Build, time.Nanosecond, "auth.go")

	if a, b := narrator.LegKey(sessAlpha, l), narrator.LegKey(sessBeta, l); a == b {
		t.Errorf("the same leg in two sessions shares a key: %q", a)
	}
	if a, b := narrator.LegKey(sessAlpha, l), narrator.LegKey(sessAlpha, later); a == b {
		t.Errorf("legs one nanosecond apart share a key: %q", a)
	}
	// Deterministic: the same input always produces the same key, or the cache
	// never hits.
	if a, b := narrator.LegKey(sessAlpha, l), narrator.LegKey(sessAlpha, l); a != b {
		t.Errorf("LegKey is not deterministic: %q then %q", a, b)
	}
}

// ---------------------------------------------------------------- T48

// T48 — only closed legs are digested: HEAD keeps its live heuristic label,
// narration is for history.
func TestT48OnlyClosedLegsAreDigested(t *testing.T) {
	n, r, _, _ := newNarrator(t)
	tr := twoClosedAndAHead()

	n.Request(sessAlpha, tr, "make refresh rotate the token")
	batch := r.waitBatch(t)

	assertBatchKeys(t, batch,
		narrator.LegKey(sessAlpha, tr.Legs[0]),
		narrator.LegKey(sessAlpha, tr.Legs[1]),
	)
	headKey := narrator.LegKey(sessAlpha, tr.Legs[2])
	for _, d := range batch {
		if d.Key == headKey {
			t.Fatalf("HEAD (%q) was digested; narration is for closed legs only", headKey)
		}
	}
	r.answer()
}

// The digest is what the model sees: everything the contract lists, carried
// over from the leg.
func TestT48DigestCarriesTheLeg(t *testing.T) {
	n, r, _, _ := newNarrator(t)
	tr := twoClosedAndAHead()

	n.Request(sessAlpha, tr, "make refresh rotate the token")
	batch := r.waitBatch(t)

	byKey := map[string]narrator.Digest{}
	for _, d := range batch {
		byKey[d.Key] = d
	}
	build, ok := byKey[narrator.LegKey(sessAlpha, tr.Legs[0])]
	if !ok {
		t.Fatalf("the build leg is not in the batch: %v", keysOf(batch))
	}
	if build.Class != "build" {
		t.Errorf("Digest.Class = %q, want %q", build.Class, "build")
	}
	if build.Label != "auth.go" {
		t.Errorf("Digest.Label = %q, want the leg's heuristic label %q", build.Label, "auth.go")
	}
	if strings.Join(build.Files, ",") != "auth.go,token.go" {
		t.Errorf("Digest.Files = %v, want the leg's files in order", build.Files)
	}
	if strings.Join(build.Waypoints, "|") != "cannot find package jwt" {
		t.Errorf("Digest.Waypoints = %v, want the waypoint texts in order", build.Waypoints)
	}
	if build.Prompt != "make refresh rotate the token" {
		t.Errorf("Digest.Prompt = %q, want the prompt Request was given", build.Prompt)
	}

	test := byKey[narrator.LegKey(sessAlpha, tr.Legs[1])]
	if strings.Join(test.Waypoints, "|") != "18 passed, 2 failed|TestRefreshRotatesTheToken" {
		t.Errorf("Digest.Waypoints = %v, want both waypoint texts in order", test.Waypoints)
	}
	r.answer()
}

// Digest.Prompt is capped at 120 runes: it is context, not the payload.
func TestT48DigestPromptIsClippedTo120Runes(t *testing.T) {
	n, r, _, _ := newNarrator(t)
	long := strings.Repeat("é", 300)

	n.Request(sessAlpha, twoClosedAndAHead(), long)
	batch := r.waitBatch(t)
	for _, d := range batch {
		if n := utf8.RuneCountInString(d.Prompt); n > 120 {
			t.Errorf("Digest.Prompt is %d runes, want ≤120", n)
		}
		if !utf8.ValidString(d.Prompt) {
			t.Errorf("Digest.Prompt is not valid UTF-8 — it was clipped on bytes, not runes")
		}
	}
	r.answer()
}

// A key already in the cache is never sent again: that is what the file is for.
func TestT48CachedKeysAreNotRequested(t *testing.T) {
	n, r, c, _ := newNarrator(t)
	tr := twoClosedAndAHead()
	cached := narrator.LegKey(sessAlpha, tr.Legs[0])
	put(t, c, cached, "wires the token refresh")

	n.Request(sessAlpha, tr, "keep going")
	batch := r.waitBatch(t)
	assertBatchKeys(t, batch, narrator.LegKey(sessAlpha, tr.Legs[1]))
	r.answer()
}

// Nothing left to narrate is a no-op, not an empty call to the CLI.
func TestT48NoOpWhenNothingIsLeftToNarrate(t *testing.T) {
	cases := []struct {
		name  string
		trail journey.Trail
	}{
		{"empty trail", journey.Trail{}},
		{"only HEAD", journey.Trail{Legs: []journey.Leg{head(leg(journey.Build, 0, "auth.go"))}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n, r, _, _ := newNarrator(t)
			n.Request(sessAlpha, tc.trail, "anything")
			r.noBatch(t, "there is no closed, uncached leg to narrate")
		})
	}

	t.Run("everything cached", func(t *testing.T) {
		n, r, c, _ := newNarrator(t)
		tr := twoClosedAndAHead()
		put(t, c, narrator.LegKey(sessAlpha, tr.Legs[0]), "wires the token refresh")
		put(t, c, narrator.LegKey(sessAlpha, tr.Legs[1]), "watches the suite go red")
		n.Request(sessAlpha, tr, "keep going")
		r.noBatch(t, "both closed legs are already cached")
	})
}

// T48 — one batch in flight at a time. The UI calls Request on every trail
// change; a second call while the first is out must not open a second CLI.
func TestT48SecondRequestWhileInFlightDoesNotCallTheRunnerTwice(t *testing.T) {
	n, r, _, notified := newNarrator(t)
	tr := twoClosedAndAHead()

	// No reply is queued, so the runner blocks and the batch is held in flight.
	n.Request(sessAlpha, tr, "make refresh rotate the token")
	held := r.waitBatch(t)
	if len(held) != 2 {
		t.Fatalf("held batch has %d digests, want 2: %v", len(held), keysOf(held))
	}

	// Everything the second Request could ask for is in the held batch.
	n.Request(sessAlpha, tr, "make refresh rotate the token")
	r.noBatch(t, "a batch is already in flight")

	// Releasing the first batch completes it normally.
	r.answer(
		narrator.Label{Key: held[0].Key, Text: "wires the token refresh"},
		narrator.Label{Key: held[1].Key, Text: "watches the suite go red"},
	)
	waitNotify(t, notified)

	if got := r.calls(); got != 1 {
		t.Errorf("the runner was called %d times, want 1", got)
	}
	labels := n.Labels(sessAlpha, tr)
	if labels[held[0].Key] != "wires the token refresh" || labels[held[1].Key] != "watches the suite go red" {
		t.Errorf("Labels after the batch landed = %v", labels)
	}
}

// T48 — a failed batch is not retried immediately (that would hammer the CLI
// on every redraw) and is not abandoned either: the next Request skips those
// keys, the one after retries them.
func TestT48FailedBatchCoolsOffForOneRequestThenRetries(t *testing.T) {
	n, r, _, notified := newNarrator(t)
	tr := twoClosedAndAHead()
	k0 := narrator.LegKey(sessAlpha, tr.Legs[0])
	k1 := narrator.LegKey(sessAlpha, tr.Legs[1])

	// Request #1: the batch goes out and fails.
	n.Request(sessAlpha, tr, "make refresh rotate the token")
	assertBatchKeys(t, r.waitBatch(t), k0, k1)
	r.fail(errors.New("claude: exit status 1"))
	waitIdle(t, n) // the failing goroutine must finish its bookkeeping first —
	// a Request landing mid-flight bounces off without burning the backoff

	// Request #2 is the cooling-off: those keys are remembered as tried and
	// skipped, and they are the only keys there are, so nothing goes out.
	n.Request(sessAlpha, tr, "make refresh rotate the token")
	r.noBatch(t, "the failed keys are cooling off for one call")

	// Request #3 retries them.
	n.Request(sessAlpha, tr, "make refresh rotate the token")
	assertBatchKeys(t, r.waitBatch(t), k0, k1)
	r.answer(
		narrator.Label{Key: k0, Text: "wires the token refresh"},
		narrator.Label{Key: k1, Text: "watches the suite go red"},
	)
	waitNotify(t, notified)

	if got := r.calls(); got != 2 {
		t.Errorf("the runner was called %d times, want 2 (the failure and the retry)", got)
	}
	if got := n.Labels(sessAlpha, tr); got[k0] != "wires the token refresh" {
		t.Errorf("after the retry Labels = %v", got)
	}
}

// A leg narrated once is never narrated again, even across many Requests.
func TestT48SucceededKeysAreNeverRequestedAgain(t *testing.T) {
	n, r, _, notified := newNarrator(t)
	tr := twoClosedAndAHead()
	k0 := narrator.LegKey(sessAlpha, tr.Legs[0])
	k1 := narrator.LegKey(sessAlpha, tr.Legs[1])

	n.Request(sessAlpha, tr, "keep going")
	r.waitBatch(t)
	r.answer(
		narrator.Label{Key: k0, Text: "wires the token refresh"},
		narrator.Label{Key: k1, Text: "watches the suite go red"},
	)
	waitNotify(t, notified)

	for i := 0; i < 3; i++ {
		n.Request(sessAlpha, tr, "keep going")
	}
	r.noBatch(t, "both legs are cached from the first batch")
	if got := r.calls(); got != 1 {
		t.Errorf("the runner was called %d times, want 1", got)
	}
}

// A leg that closes after the first batch is picked up by the next Request,
// and only that leg.
func TestT48NewlyClosedLegIsNarratedOnTheNextRequest(t *testing.T) {
	n, r, _, notified := newNarrator(t)
	tr := twoClosedAndAHead()
	k0 := narrator.LegKey(sessAlpha, tr.Legs[0])
	k1 := narrator.LegKey(sessAlpha, tr.Legs[1])

	n.Request(sessAlpha, tr, "keep going")
	r.waitBatch(t)
	r.answer(
		narrator.Label{Key: k0, Text: "wires the token refresh"},
		narrator.Label{Key: k1, Text: "watches the suite go red"},
	)
	waitNotify(t, notified)

	// HEAD closes and a new HEAD opens.
	grown := journey.Trail{Legs: []journey.Leg{
		tr.Legs[0],
		tr.Legs[1],
		leg(journey.Fix, 8*time.Minute, "tailer.go", "tailer.go"), // was HEAD, now closed
		head(leg(journey.Ship, 12*time.Minute, "commit")),
	}}
	n.Request(sessAlpha, grown, "ship it")
	assertBatchKeys(t, r.waitBatch(t), narrator.LegKey(sessAlpha, grown.Legs[2]))
	r.answer()
}

// T48 — notify is what makes the label appear: it fires after the labels are in
// the cache, not before.
func TestT48NotifyFiresAfterLabelsLand(t *testing.T) {
	n, r, c, notified := newNarrator(t)
	tr := twoClosedAndAHead()
	k0 := narrator.LegKey(sessAlpha, tr.Legs[0])
	k1 := narrator.LegKey(sessAlpha, tr.Legs[1])

	n.Request(sessAlpha, tr, "keep going")
	r.waitBatch(t)

	select {
	case <-notified:
		t.Fatal("notify fired before the runner answered")
	default:
	}

	r.answer(
		narrator.Label{Key: k0, Text: "wires the token refresh"},
		narrator.Label{Key: k1, Text: "watches the suite go red"},
	)
	waitNotify(t, notified)

	// By the time notify fires the cache is already readable — a redraw
	// triggered by it must not find the labels missing.
	assertGet(t, c, k0, "wires the token refresh")
	assertGet(t, c, k1, "watches the suite go red")
}

// A failed batch stores nothing, so there is nothing to redraw for.
func TestT48NotifyDoesNotFireForAFailedBatch(t *testing.T) {
	n, r, _, notified := newNarrator(t)
	tr := twoClosedAndAHead()

	n.Request(sessAlpha, tr, "keep going")
	r.waitBatch(t)
	r.fail(errors.New("claude: executable file not found in $PATH"))
	r.noBatch(t, "a failed batch must not retry itself")

	select {
	case <-notified:
		t.Error("notify fired for a batch that produced no labels")
	default:
	}
	if got := n.Labels(sessAlpha, tr); len(got) != 0 {
		t.Errorf("Labels after a failed batch = %v, want none", got)
	}
}

// T48 — Labels is a pure lookup over the trail's closed legs. HEAD is never in
// it, whatever the cache happens to hold, and neither is a key from some other
// trail.
func TestT48LabelsCoversClosedLegsOnly(t *testing.T) {
	n, _, c, _ := newNarrator(t)
	tr := twoClosedAndAHead()
	k0 := narrator.LegKey(sessAlpha, tr.Legs[0])
	k1 := narrator.LegKey(sessAlpha, tr.Legs[1])
	kHead := narrator.LegKey(sessAlpha, tr.Legs[2])

	put(t, c, k0, "wires the token refresh")
	put(t, c, k1, "watches the suite go red")
	put(t, c, kHead, "narrated the open leg")            // must never surface
	put(t, c, "some-other-session/1/build", "not mine")  // not in this trail
	put(t, c, narrator.LegKey(sessBeta, tr.Legs[0]), "") // right leg, wrong session

	got := n.Labels(sessAlpha, tr)
	if len(got) != 2 {
		t.Fatalf("Labels returned %d entries, want 2: %v", len(got), got)
	}
	if got[k0] != "wires the token refresh" || got[k1] != "watches the suite go red" {
		t.Errorf("Labels = %v", got)
	}
	if _, ok := got[kHead]; ok {
		t.Errorf("Labels includes HEAD (%q) — the open leg always keeps its live heuristic label", kHead)
	}
}

// Half a trail narrated is a normal state: Labels returns what it has.
func TestT48LabelsReturnsOnlyWhatIsCached(t *testing.T) {
	n, _, c, _ := newNarrator(t)
	tr := twoClosedAndAHead()
	k0 := narrator.LegKey(sessAlpha, tr.Legs[0])

	if got := n.Labels(sessAlpha, tr); len(got) != 0 {
		t.Errorf("Labels on a cold cache = %v, want none", got)
	}
	put(t, c, k0, "wires the token refresh")
	got := n.Labels(sessAlpha, tr)
	if len(got) != 1 || got[k0] != "wires the token refresh" {
		t.Errorf("Labels = %v, want just the one cached leg", got)
	}
}

// Labels is called on every render; it must not reach for the runner.
func TestT48LabelsNeverCallsTheRunner(t *testing.T) {
	n, r, c, _ := newNarrator(t)
	tr := twoClosedAndAHead()
	put(t, c, narrator.LegKey(sessAlpha, tr.Legs[0]), "wires the token refresh")

	for i := 0; i < 5; i++ {
		n.Labels(sessAlpha, tr)
	}
	r.noBatch(t, "Labels is a pure lookup")
	if got := r.calls(); got != 0 {
		t.Errorf("Labels called the runner %d times, want 0", got)
	}
}

// The model answers with a key nobody asked for. Whatever the narrator does
// with it internally, it must never reach the trail — least of all as HEAD's
// label.
func TestT48LabelsForKeysNotInTheTrailNeverSurface(t *testing.T) {
	n, r, _, notified := newNarrator(t)
	tr := twoClosedAndAHead()
	k0 := narrator.LegKey(sessAlpha, tr.Legs[0])
	k1 := narrator.LegKey(sessAlpha, tr.Legs[1])
	kHead := narrator.LegKey(sessAlpha, tr.Legs[2])

	n.Request(sessAlpha, tr, "keep going")
	r.waitBatch(t)
	r.answer(
		narrator.Label{Key: k0, Text: "wires the token refresh"},
		narrator.Label{Key: kHead, Text: "renamed the open leg"},
		narrator.Label{Key: "invented/12345/build", Text: "a leg that does not exist"},
	)
	waitNotify(t, notified)

	got := n.Labels(sessAlpha, tr)
	if got[k0] != "wires the token refresh" {
		t.Errorf("Labels[%q] = %q, want the narrated text", k0, got[k0])
	}
	if _, ok := got[kHead]; ok {
		t.Errorf("Labels includes HEAD (%q) after the model answered for it", kHead)
	}
	if _, ok := got[k1]; ok {
		t.Errorf("Labels includes %q, which the model never answered for", k1)
	}
	if len(got) != 1 {
		t.Errorf("Labels = %v, want exactly the one leg the model actually named", got)
	}
}

// Two sessions share one narrator. Their legs must not collide, and a Request
// for one must not narrate the other's.
func TestT48SessionsDoNotShareLabels(t *testing.T) {
	n, r, _, notified := newNarrator(t)
	tr := twoClosedAndAHead()

	n.Request(sessAlpha, tr, "keep going")
	batch := r.waitBatch(t)
	assertBatchKeys(t, batch,
		narrator.LegKey(sessAlpha, tr.Legs[0]),
		narrator.LegKey(sessAlpha, tr.Legs[1]),
	)
	r.answer(
		narrator.Label{Key: batch[0].Key, Text: "wires the token refresh"},
		narrator.Label{Key: batch[1].Key, Text: "watches the suite go red"},
	)
	waitNotify(t, notified)

	// The same legs under a different session id are different legs.
	if got := n.Labels(sessBeta, tr); len(got) != 0 {
		t.Errorf("Labels for %s = %v, want none — those labels belong to %s", sessBeta, got, sessAlpha)
	}
	n.Request(sessBeta, tr, "keep going")
	assertBatchKeys(t, r.waitBatch(t),
		narrator.LegKey(sessBeta, tr.Legs[0]),
		narrator.LegKey(sessBeta, tr.Legs[1]),
	)
	r.answer()
}

// The UI calls Request from the update loop and Labels from the render; a
// narrator that is not safe for that is unusable. Run under -race.
func TestT48ConcurrentRequestsAndLabelsAreRaceFree(t *testing.T) {
	r := newFakeRunner()
	// Pre-answer generously: any batch that goes out returns at once. Fill to
	// the channel's capacity — after one successful batch per session the keys
	// are cached, so far fewer are ever consumed.
	for i := 0; i < cap(r.replies); i++ {
		r.replies <- reply{}
	}
	notify, _ := notifier()
	n := narrator.New(r, openCache(t, cachePath(t)), notify)
	tr := twoClosedAndAHead()

	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			sess := sessAlpha
			if g%2 == 1 {
				sess = sessBeta
			}
			for i := 0; i < 20; i++ {
				n.Request(sess, tr, "keep going")
				n.Labels(sess, tr)
			}
		}(g)
	}
	wg.Wait()

	// Drain whatever the runner recorded so the goroutines can finish.
	for {
		select {
		case <-r.entered:
			continue
		default:
		}
		break
	}
}
