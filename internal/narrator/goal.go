package narrator

import (
	"strconv"
	"strings"

	"github.com/deephanson94/compass/internal/journey"
)

// A goal is the one thing narration says about a whole session rather than
// a leg: what it is for, in a few words — "port LFM2 runner to cmdline" —
// which a board card wears beside its state. Twenty-eight prompts into a
// session, no single prompt says it and no heuristic can; it is the one
// sentence on the card a model is for.
//
// It travels in the same batch as the legs (a digest of kind "goal"), is
// cached the same way, and is asked once per prompt: the key moves when a
// prompt lands, so a session is re-asked when its person said something
// new and never on a tick. The caller asks only when the turn is over —
// a working session's goal is still moving.

// goalKind marks a digest that is a whole session, not a leg.
const goalKind = "goal"

// goalPrompts caps how many prompts a goal digest carries: the first, which
// usually names the work, and the latest ones, which say where it went.
const goalPrompts = 12

// GoalKey is the cache identity of a session's goal at its current prompt:
// the session's key, how many prompts it has had, and when the last one
// landed. A new prompt is a new key; nothing else moves it.
func GoalKey(sessionKey string, tr journey.Trail) string {
	if len(tr.Prompts) == 0 {
		return ""
	}
	last := tr.Prompts[len(tr.Prompts)-1]
	return "goal/" + sessionKey + "/" + strconv.Itoa(len(tr.Prompts)) + "/" + strconv.FormatInt(last.At.UnixNano(), 10)
}

// Goal is the session's narrated goal: the one asked for its current prompt
// when it has landed, else the last one that landed for the session in this
// process — a goal rarely changes with one prompt, and the card is better
// for a goal one prompt stale than for none while the new one is asked.
func (n *Narrator) Goal(sessionKey string, tr journey.Trail) (string, bool) {
	if n == nil || n.cache == nil {
		return "", false
	}
	key := GoalKey(sessionKey, tr)
	if key == "" {
		return "", false
	}
	if text, ok := n.cache.Get(key); ok && text != "" {
		n.mu.Lock()
		if n.goals == nil {
			n.goals = map[string]string{}
		}
		n.goals[sessionKey] = text
		n.mu.Unlock()
		return text, true
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	text, ok := n.goals[sessionKey]
	return text, ok && text != ""
}

// RequestGoal asks for the session's goal at its current prompt. It rides
// the next batch — the one going out now when none is in flight, else the
// one after it — so a board of ten finished sessions costs one call, not
// ten. It reports whether the goal is spoken for: cached, queued or sent.
func (n *Narrator) RequestGoal(sessionKey string, tr journey.Trail) bool {
	if n == nil || n.runner == nil || n.cache == nil {
		return true
	}
	key := GoalKey(sessionKey, tr)
	if key == "" {
		return true
	}
	if _, ok := n.cache.Get(key); ok {
		return true
	}
	n.mu.Lock()
	if n.cooling[key] {
		// One call of backoff, as a leg gets: the next ask retries.
		delete(n.cooling, key)
		n.mu.Unlock()
		return false
	}
	if n.pendingGoals == nil {
		n.pendingGoals = map[string]Digest{}
	}
	n.pendingGoals[key] = goalDigest(key, tr)
	if n.inFlight {
		n.mu.Unlock()
		return true // queued: the batch in flight is someone else's, the next one carries it
	}
	digests := n.takeGoals()
	n.inFlight = true
	n.mu.Unlock()
	go n.run(digests)
	return true
}

// takeGoals empties the queue of goals into a batch. Caller holds the mutex.
func (n *Narrator) takeGoals() []Digest {
	if len(n.pendingGoals) == 0 {
		return nil
	}
	out := make([]Digest, 0, len(n.pendingGoals))
	for _, d := range n.pendingGoals {
		out = append(out, d)
	}
	n.pendingGoals = nil
	return out
}

// goalDigest is what the model reads about a session: its prompts, the
// first and the latest, and the names of its newest legs.
func goalDigest(key string, tr journey.Trail) Digest {
	var prompts []string
	for i, p := range tr.Prompts {
		if i != 0 && i < len(tr.Prompts)-(goalPrompts-1) {
			continue
		}
		text := clip(p.Text, promptContext)
		if p.Relayed {
			text = "(relayed) " + text
		}
		prompts = append(prompts, text)
	}
	var files []string
	for i := len(tr.Legs) - 1; i >= 0 && len(files) < maxGoalFiles; i-- {
		for _, f := range tr.Legs[i].Files {
			if len(files) < maxGoalFiles && !contains(files, f) {
				files = append(files, f)
			}
		}
	}
	return Digest{
		Key:       key,
		Kind:      goalKind,
		Class:     "session",
		Files:     files,
		Waypoints: prompts,
		Prompt:    clip(tr.Prompts[len(tr.Prompts)-1].Text, promptContext),
	}
}

// maxGoalFiles is how many of the newest files a goal digest names.
const maxGoalFiles = 5

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// goalSession is the session a goal key belongs to; false for a leg's key.
func goalSession(key string) (string, bool) {
	rest, ok := strings.CutPrefix(key, "goal/")
	if !ok {
		return "", false
	}
	// The session key is a path and may hold slashes; the last two parts
	// are the prompt count and the clock.
	parts := strings.Split(rest, "/")
	if len(parts) < 3 {
		return "", false
	}
	return strings.Join(parts[:len(parts)-2], "/"), true
}
