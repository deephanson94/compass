package journey

import (
	"time"

	"github.com/deephanson94/compass/internal/transcript"
)

// Outcome is the last thing a session finished rather than started: a test
// run's counts, or a commit. It is what the fleet wants to say about a session
// at a glance — "1216✓ 2✗" answers "is it going well", where the tool call in
// flight ("Bash: pytest tests/auth -x") only answers "is it busy", which the
// state glyph already did.
type Outcome struct {
	Kind  WaypointKind // WaypointTestRun or WaypointCommit
	Text  string       // "1216 passed · 2 failed"
	Short string       // "1216✓ 2✗" — empty when the kind has no badge form
	At    time.Time
}

// Outcomes folds a session's events into its latest finished result: the
// waypoint machinery without the legs.
//
// The fleet needs this for every live session at once, and segmenting fifteen
// transcripts to render fifteen rows would be paying for a journey to read one
// line of it. The rules it keeps are the segmenter's own (M2 contract, rules 2
// and 3): only a remembered Test or Ship call's own result is parsed, so a
// `cat` of somebody else's test log is never mistaken for a run.
type Outcomes struct {
	runners     map[string]runner
	runnerOrder []string

	latest Outcome
	has    bool
}

func NewOutcomes() *Outcomes {
	return &Outcomes{runners: make(map[string]runner)}
}

// Observe feeds one event in. Events must arrive in file order.
func (o *Outcomes) Observe(ev transcript.Event) {
	// A sidechain line is a subagent's own work. Its test run is not this
	// session's result, exactly as it is not this session's leg.
	if ev.IsSidechain {
		return
	}
	for _, res := range ev.ToolResults {
		o.result(res, ev.Timestamp)
	}
	for _, use := range ev.ToolUses {
		if use.Name == agentTool {
			continue
		}
		if v, ok := classifyUse(use); ok {
			o.remember(v)
		}
	}
}

// Latest is the newest outcome seen, if any.
func (o *Outcomes) Latest() (Outcome, bool) {
	if o == nil || !o.has {
		return Outcome{}, false
	}
	return o.latest, true
}

func (o *Outcomes) result(res transcript.ToolResult, at time.Time) {
	r, ok := o.runners[res.ToolUseID]
	if !ok {
		return
	}
	delete(o.runners, res.ToolUseID)

	var wps []Waypoint
	switch r.kind {
	case Test:
		wps = testWaypoints(res.Text, r.keyword, res.IsError, at)
	case Ship:
		wps = shipWaypoints(res.Text, at)
	}
	// The run summary is the headline; a list of failing names is Lv2's job.
	for _, w := range wps {
		if w.Kind == WaypointTestRun || w.Kind == WaypointCommit {
			o.latest, o.has = Outcome{Kind: w.Kind, Text: w.Text, Short: w.Short, At: w.At}, true
			return
		}
	}
}

func (o *Outcomes) remember(v vote) {
	if v.id == "" || (v.class != Test && v.class != Ship) {
		return
	}
	if _, dup := o.runners[v.id]; dup {
		return
	}
	if len(o.runnerOrder) >= maxRunners {
		delete(o.runners, o.runnerOrder[0])
		o.runnerOrder = o.runnerOrder[1:]
	}
	o.runners[v.id] = runner{kind: v.class, keyword: v.keyword}
	o.runnerOrder = append(o.runnerOrder, v.id)
}
