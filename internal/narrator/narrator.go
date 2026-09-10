// Package narrator upgrades the trail's heuristic leg labels to prose. It never
// talks to an API: it shells out to the user's own `claude` binary in headless
// mode, so whatever auth they already have (subscription or Bedrock) is the
// auth compass narrates with. Everything is best-effort — a missing binary, a
// timeout or a malformed answer costs the panel nothing but its heuristics.
package narrator

import (
	"strconv"
	"strings"
	"sync"

	"github.com/deephanson94/compass/internal/journey"
)

// Digest is what the model sees about one closed leg — enough to name it, small
// enough to batch.
type Digest struct {
	Key       string   // stable identity (LegKey)
	Class     string   // "fix", "test", …
	Label     string   // current heuristic label ("" ok)
	Files     []string // Leg.Files
	Waypoints []string // Waypoint texts, in order
	Prompt    string   // latest user prompt before the leg (context, ≤120 runes)
}

// Label is one narrated result.
type Label struct {
	Key  string
	Text string // ≤32 runes, lowercase-first prose, no trailing period
}

// labelText is how wide a narrated label may read: the trail gives it one row
// beside the class verb, and 32 runes is what fits at the narrowest panel.
const labelText = 32

// promptContext caps how much of the user's latest prompt travels with a batch.
const promptContext = 120

// Runner produces labels for digests. CLIRunner is the real one; tests fake it.
type Runner interface {
	Narrate(digests []Digest) ([]Label, error)
}

// LegKey is the cache/dedupe identity of a leg. Start is the one field that
// never moves once a leg is closed — the class can still be upgraded under it
// (Build → Fix), which is why it belongs in the key: a re-classified leg is
// honestly a different leg and deserves a fresh name.
//
// The first part is the session's KEY — fleet.SessionInfo.Key(), the transcript
// path — not its id: one id can own transcripts under several project slugs,
// and two such sessions must not narrate into each other's cached labels
// (docs/dev/M6-CONTRACT.md). The parameter keeps its old name; what callers
// pass has changed.
func LegKey(sessionID string, l journey.Leg) string {
	return sessionID + "/" + strconv.FormatInt(l.Start.UnixNano(), 10) + "/" + l.Class.String()
}

// Narrator orchestrates: dedupe against the cache, one batch in flight at a
// time, and a cooling-off list so a batch that failed does not get retried on
// the very next tick.
type Narrator struct {
	runner Runner
	cache  *Cache
	notify func()

	mu       sync.Mutex
	inFlight bool

	// cooling holds the keys of the batch that just came back empty-handed.
	// The next Request skips them and clears the set, so the Request after
	// that tries again: a backoff of exactly one call, which is enough to keep
	// a broken binary from being invoked on every frame.
	cooling map[string]bool
}

// New builds a narrator; notify is called (from the batch goroutine) after new
// labels land in the cache — the UI uses it to trigger a redraw. A nil notify
// is fine, as is a nil cache: the narrator then simply never has anything to
// say.
func New(r Runner, c *Cache, notify func()) *Narrator {
	return &Narrator{runner: r, cache: c, notify: notify}
}

// Labels returns the cached labels for the trail's CLOSED legs, keyed by
// LegKey — so sessionID here is the caller's session key, as everywhere in this
// package. Pure lookup, no I/O beyond the in-memory cache. HEAD is left out on
// purpose: an open leg is still changing, and narration is for history.
func (n *Narrator) Labels(sessionID string, tr journey.Trail) map[string]string {
	out := make(map[string]string)
	if n == nil || n.cache == nil {
		return out
	}
	for i := range tr.Legs {
		if tr.Legs[i].Current {
			continue
		}
		key := LegKey(sessionID, tr.Legs[i])
		if text, ok := n.cache.Get(key); ok {
			out[key] = text
		}
	}
	return out
}

// Request narrates the trail's closed, uncached, not-cooling legs in one async
// batch. No-op when everything is cached, when a batch is already running, or
// when there is no runner to ask. prompt is the latest user prompt — context
// for the whole batch; when it is empty the trail's own last prompt stands in.
//
// It reports whether the trail is now spoken for: a batch went out, or there
// was nothing left to ask. False means "ask again next tick" — the one batch
// in flight belonged to someone else, or everything askable is cooling off.
// A caller that remembers it asked, and stops asking, must not remember a
// refusal: with several sessions asking every tick, a refused one that
// stopped would never be named at all.
func (n *Narrator) Request(sessionID string, tr journey.Trail, prompt string) bool {
	if n == nil || n.runner == nil || n.cache == nil {
		return true // nobody to ask; nothing to wait for
	}

	n.mu.Lock()
	if n.inFlight {
		n.mu.Unlock()
		return false // one batch at a time; the next tick will pick up the rest
	}
	// Spend the backoff: this Request skips what the last failure left behind,
	// and by clearing the set now the next one will retry it.
	cooling := n.cooling
	n.cooling = nil

	digests := n.digests(sessionID, tr, prompt, cooling)
	if len(digests) == 0 {
		n.mu.Unlock()
		return len(cooling) == 0 // all cached, or all cooling
	}
	n.inFlight = true
	n.mu.Unlock()

	go n.run(digests)
	return true
}

// Idle reports whether no batch is in flight. Callers that need Request's
// cooling-off bookkeeping to be settled — tests, a status row — wait on this
// rather than guessing at goroutine timing.
func (n *Narrator) Idle() bool {
	if n == nil {
		return true
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return !n.inFlight
}

// digests builds the batch: every closed leg the cache has no name for and the
// cooling-off set is not holding back. Caller holds the mutex.
func (n *Narrator) digests(sessionID string, tr journey.Trail, prompt string, cooling map[string]bool) []Digest {
	if prompt == "" && len(tr.Prompts) > 0 {
		prompt = tr.Prompts[len(tr.Prompts)-1].Text
	}
	prompt = clip(prompt, promptContext)

	var digests []Digest
	for i := range tr.Legs {
		leg := tr.Legs[i]
		if leg.Current {
			continue
		}
		key := LegKey(sessionID, leg)
		if _, ok := n.cache.Get(key); ok {
			continue
		}
		if cooling[key] {
			continue
		}
		digests = append(digests, Digest{
			Key:       key,
			Class:     leg.Class.String(),
			Label:     leg.Label,
			Files:     append([]string(nil), leg.Files...),
			Waypoints: waypointTexts(leg.Waypoints),
			Prompt:    prompt,
		})
	}
	return digests
}

// run is one batch, start to finish: ask, store, tell the UI. Whatever happens,
// the in-flight flag comes back down.
func (n *Narrator) run(digests []Digest) {
	labels, err := n.runner.Narrate(digests)

	// The instruction says "name the work, not the class", and the model does
	// not always listen: "build" under build, "scout" under scout. Such a label
	// says nothing the verb column did not, so it is treated as declined —
	// never cached, and the leg cools off like any other the model would not
	// name. The heuristic label stays on the row.
	labels = withoutClassNames(labels, digests)

	landed := 0
	if err == nil {
		for _, l := range labels {
			if l.Key == "" || l.Text == "" {
				continue
			}
			if err := n.cache.Put(l.Key, l.Text); err == nil {
				landed++
			}
		}
	}

	// Anything we asked about and did not get a usable answer for cools off:
	// a failed batch entirely, a partial answer only where it stayed silent.
	// Without this a leg the model refuses to name would be re-asked forever.
	missing := missingKeys(digests, labels, err)

	n.mu.Lock()
	n.inFlight = false
	if len(missing) > 0 {
		if n.cooling == nil {
			n.cooling = make(map[string]bool, len(missing))
		}
		for _, key := range missing {
			n.cooling[key] = true
		}
	}
	n.mu.Unlock()

	if landed > 0 && n.notify != nil {
		n.notify()
	}
}

// withoutClassNames drops every label that is only its leg's class name.
func withoutClassNames(labels []Label, digests []Digest) []Label {
	class := make(map[string]string, len(digests))
	for _, d := range digests {
		class[d.Key] = d.Class
	}
	out := labels[:0]
	for _, l := range labels {
		if strings.EqualFold(strings.TrimSpace(l.Text), class[l.Key]) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// missingKeys are the batch's keys the run did not answer: all of them when the
// call failed, else the ones no label came back for.
func missingKeys(digests []Digest, labels []Label, err error) []string {
	answered := make(map[string]bool, len(labels))
	if err == nil {
		for _, l := range labels {
			if l.Text != "" {
				answered[l.Key] = true
			}
		}
	}
	var missing []string
	for _, d := range digests {
		if !answered[d.Key] {
			missing = append(missing, d.Key)
		}
	}
	return missing
}

// waypointTexts flattens a leg's waypoints to the bare lines the model reads:
// the kinds are decoration the panel adds, not information the namer needs.
func waypointTexts(wps []journey.Waypoint) []string {
	if len(wps) == 0 {
		return nil
	}
	out := make([]string, 0, len(wps))
	for _, w := range wps {
		if w.Text != "" {
			out = append(out, w.Text)
		}
	}
	return out
}

// clip truncates to max display runes, marking the cut with an ellipsis. The
// journey package keeps its own unexported copy; a shared one would only tie
// two packages together over four lines.
func clip(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return strings.TrimRight(string(r[:max-1]), " ") + "…"
}
