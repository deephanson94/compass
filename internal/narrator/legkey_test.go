package narrator_test

// M6 — T68 (docs/dev/M6-CONTRACT.md, "Identity: the transcript path").
//
//	narrator.LegKey(sessionID, leg) → LegKey(key, leg) — signature unchanged,
//	callers pass the key.
//
// The session id does not identify a session: one id can own transcripts under
// several project slugs. LegKey's first argument is therefore the transcript
// path, and these tests hold it to the only property that matters for a cache
// — two same-id sessions must not be able to reach each other's labels. If they
// could, one session's narration would appear on its twin's trail, which is
// worse than no narration at all.
//
// Offline and deterministic: leg times are offsets from nbase, and the cache is
// a file under t.TempDir().

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/narrator"
)

// dupSessionID is the one id both transcripts carry; twinKeyA and twinKeyB are
// their paths — what M6 hands LegKey.
const (
	dupSessionID = "aa000001-0000-4000-8000-000000000001"
	twinKeyA     = "/home/user/.claude/projects/-home-user-api/" + dupSessionID + ".jsonl"
	twinKeyB     = "/home/user/.claude/projects/-home-user-api-worker/" + dupSessionID + ".jsonl"
)

// legKeyTail splits off the two fixed trailing fields — Start.UnixNano() and
// the class — and returns them with the session part that precedes them. It
// cannot use strings.Split: a transcript path contains "/" itself, so only the
// LAST two separators are structural.
func legKeyTail(t *testing.T, key string) (session, nanos, class string) {
	t.Helper()
	i := strings.LastIndex(key, "/")
	if i < 0 {
		t.Fatalf("LegKey = %q, want a %q-separated class suffix", key, "/")
	}
	class = key[i+1:]
	j := strings.LastIndex(key[:i], "/")
	if j < 0 {
		t.Fatalf("LegKey = %q, want a %q-separated start suffix", key, "/")
	}
	nanos = key[j+1 : i]
	session = key[:j]
	return session, nanos, class
}

// ---------------------------------------------------------------- T68

// T68 — the same leg under two transcript paths that share an id is two legs.
func TestT68LegKeyDistinguishesTwoSameIDSessions(t *testing.T) {
	l := leg(journey.Fix, 0, "auth.go")

	a := narrator.LegKey(twinKeyA, l)
	b := narrator.LegKey(twinKeyB, l)

	if a == b {
		t.Fatalf("two transcripts of the same session id share the leg key %q — "+
			"one session's narration would land on its twin", a)
	}

	t.Run("the same key twice is identical", func(t *testing.T) {
		// Or the cache never hits and every leg is narrated forever.
		if got := narrator.LegKey(twinKeyA, l); got != a {
			t.Errorf("LegKey is not deterministic: %q then %q", a, got)
		}
		if got := narrator.LegKey(twinKeyB, l); got != b {
			t.Errorf("LegKey is not deterministic: %q then %q", b, got)
		}
	})

	t.Run("the whole key is the session part", func(t *testing.T) {
		// The path contains "/", so the format is still key + "/" + nanos +
		// "/" + class — read from the right, not by splitting.
		for _, tc := range []struct{ key, got string }{{twinKeyA, a}, {twinKeyB, b}} {
			session, nanos, class := legKeyTail(t, tc.got)
			if session != tc.key {
				t.Errorf("LegKey(%q).session = %q, want the whole key", tc.key, session)
			}
			if nanos != "1788091200000000000" {
				t.Errorf("LegKey(%q) start part = %q, want Start.UnixNano()", tc.key, nanos)
			}
			if class != "fix" {
				t.Errorf("LegKey(%q) class part = %q, want %q", tc.key, class, "fix")
			}
			if !strings.HasPrefix(tc.got, tc.key+"/") {
				t.Errorf("LegKey(%q) = %q, want it to begin with the key", tc.key, tc.got)
			}
		}
	})

	t.Run("neither twin's key is the id's key", func(t *testing.T) {
		// The pre-M6 key, which both twins used to produce. Nothing may still
		// generate it, or the collapse is only half undone.
		collapsed := narrator.LegKey(dupSessionID, l)
		if a == collapsed || b == collapsed {
			t.Errorf("a twin still keys by the bare session id: %q", collapsed)
		}
	})
}

// A whole trail, both twins: every key distinct, none shared.
func TestT68NoLegKeyCollidesAcrossTwins(t *testing.T) {
	legs := []journey.Leg{
		leg(journey.Build, 0, "auth.go"),
		leg(journey.Fix, 2*time.Minute, "token.go"),
		leg(journey.Test, 5*time.Minute, "auth_test.go"),
		// Same instant as the Build leg, different class: still its own leg.
		leg(journey.Scout, 0, "router.go"),
	}

	seen := make(map[string]string, 2*len(legs))
	for _, key := range []string{twinKeyA, twinKeyB} {
		for i, l := range legs {
			k := narrator.LegKey(key, l)
			if k == "" {
				t.Fatalf("LegKey(%q, legs[%d]) = %q", key, i, k)
			}
			if prev, dup := seen[k]; dup {
				t.Errorf("leg key %q is produced twice: %s and %s/legs[%d]", k, prev, key, i)
			}
			seen[k] = fmt.Sprintf("%s/legs[%d]", key, i)
		}
	}
	if len(seen) != 2*len(legs) {
		t.Errorf("%d distinct keys for %d legs across two twins, want %d", len(seen), len(legs), 2*len(legs))
	}
}

// The cache is the thing the key protects: a label earned by one twin must be
// invisible to the other.
func TestT68CachedLabelsDoNotLeakBetweenTwins(t *testing.T) {
	l := leg(journey.Fix, 0, "auth.go")
	keyA := narrator.LegKey(twinKeyA, l)
	keyB := narrator.LegKey(twinKeyB, l)

	path := cachePath(t)
	c := openCache(t, path)
	put(t, c, keyA, "chases the token refresh bug")

	assertGet(t, c, keyA, "chases the token refresh bug")
	assertMissing(t, c, keyB)
	assertMissing(t, c, narrator.LegKey(dupSessionID, l))

	// And the other way round, with the labels reloaded from disk.
	put(t, c, keyB, "renames the worker queue")
	reopened := openCache(t, path)
	assertGet(t, reopened, keyA, "chases the token refresh bug")
	assertGet(t, reopened, keyB, "renames the worker queue")
}

// End to end: the Narrator dedupes and looks up by LegKey, so two twins must
// each be narrated on their own account.
func TestT68TwinsDoNotShareNarratedLabels(t *testing.T) {
	n, r, _, notified := newNarrator(t)
	tr := twoClosedAndAHead()

	n.Request(twinKeyA, tr, "keep going")
	batch := r.waitBatch(t)
	assertBatchKeys(t, batch,
		narrator.LegKey(twinKeyA, tr.Legs[0]),
		narrator.LegKey(twinKeyA, tr.Legs[1]),
	)
	r.answer(
		narrator.Label{Key: batch[0].Key, Text: "wires the token refresh"},
		narrator.Label{Key: batch[1].Key, Text: "watches the suite go red"},
	)
	waitNotify(t, notified)

	if got := n.Labels(twinKeyA, tr); len(got) != 2 {
		t.Fatalf("Labels for the first twin = %v, want its two labels", got)
	}

	// The twin: same id, same trail shape, no labels of its own yet.
	if got := n.Labels(twinKeyB, tr); len(got) != 0 {
		t.Errorf("Labels for %s = %v, want none — those labels belong to %s", twinKeyB, got, twinKeyA)
	}
	n.Request(twinKeyB, tr, "keep going")
	assertBatchKeys(t, r.waitBatch(t),
		narrator.LegKey(twinKeyB, tr.Legs[0]),
		narrator.LegKey(twinKeyB, tr.Legs[1]),
	)
	r.answer()
}
