package fleet_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/state"
)

// liveManager is the M0-era Manager these tests were written against: every
// session in the fixture root is live, however old its timestamps. The tests
// in this file exercise ordering, status lines and exclusion — liveness has
// its own suite (live_test.go, M5).
func liveManager(root string) *fleet.Manager {
	m := fleet.NewManager(root)
	m.SetLiveWindow(1000 * time.Hour)
	return m
}

// fleetNow is the single instant every Manager test evaluates at; every fixture
// timestamp below is expressed as a distance from it, so nothing depends on the
// wall clock.
var fleetNow = time.Date(2026, 8, 30, 15, 0, 0, 0, time.UTC)

func ago(d time.Duration) time.Time { return fleetNow.Add(-d) }

// ---------------------------------------------------------------- fixtures

// transcriptBuilder writes a JSONL transcript with the same line shapes a live
// Claude Code session produces.
type transcriptBuilder struct {
	t      *testing.T
	sess   string
	cwd    string
	branch string
	n      int
	lines  []string
}

func newTranscript(t *testing.T, sess, cwd, branch string) *transcriptBuilder {
	t.Helper()
	return &transcriptBuilder{t: t, sess: sess, cwd: cwd, branch: branch}
}

func (b *transcriptBuilder) uuid() (id, parent string) {
	b.n++
	head := b.sess[:8]
	id = fmt.Sprintf("%s-%04d-4000-8000-000000000001", head, b.n)
	if b.n > 1 {
		parent = fmt.Sprintf("%s-%04d-4000-8000-000000000001", head, b.n-1)
	}
	return id, parent
}

func (b *transcriptBuilder) common(ts time.Time) map[string]any {
	id, parent := b.uuid()
	var p any
	if parent != "" {
		p = parent
	}
	return map[string]any{
		"parentUuid":  p,
		"isSidechain": false,
		"uuid":        id,
		"timestamp":   ts.UTC().Format("2006-01-02T15:04:05.000Z"),
		"userType":    "external",
		"entrypoint":  "remote_desktop",
		"cwd":         b.cwd,
		"sessionId":   b.sess,
		"version":     "2.1.251",
		"gitBranch":   b.branch,
	}
}

func (b *transcriptBuilder) add(o map[string]any) *transcriptBuilder {
	b.t.Helper()
	raw, err := json.Marshal(o)
	if err != nil {
		b.t.Fatalf("marshal transcript line: %v", err)
	}
	b.lines = append(b.lines, string(raw)+"\n")
	return b
}

func (b *transcriptBuilder) prompt(ts time.Time, text string) *transcriptBuilder {
	o := b.common(ts)
	o["type"] = "user"
	o["promptId"] = "p-" + b.sess[:8]
	o["message"] = map[string]any{"role": "user", "content": text}
	return b.add(o)
}

func (b *transcriptBuilder) text(ts time.Time, text string) *transcriptBuilder {
	o := b.common(ts)
	o["type"] = "assistant"
	o["requestId"] = "req_" + b.sess[:8]
	o["message"] = map[string]any{
		"role": "assistant", "model": "claude-fable-5", "type": "message",
		"content":     []any{map[string]any{"type": "text", "text": text}},
		"stop_reason": "end_turn",
	}
	return b.add(o)
}

func (b *transcriptBuilder) tool(ts time.Time, toolID, name string, input map[string]any) *transcriptBuilder {
	o := b.common(ts)
	o["type"] = "assistant"
	o["requestId"] = "req_" + b.sess[:8]
	o["message"] = map[string]any{
		"role": "assistant", "model": "claude-fable-5", "type": "message",
		"content": []any{map[string]any{
			"type": "tool_use", "id": toolID, "name": name, "input": input,
		}},
		"stop_reason": "tool_use",
	}
	return b.add(o)
}

func (b *transcriptBuilder) result(ts time.Time, toolID, content string) *transcriptBuilder {
	o := b.common(ts)
	o["type"] = "user"
	o["promptId"] = "p-" + b.sess[:8]
	o["message"] = map[string]any{
		"role": "user",
		"content": []any{map[string]any{
			"type": "tool_result", "tool_use_id": toolID, "is_error": false, "content": content,
		}},
	}
	o["toolUseResult"] = content
	return b.add(o)
}

// write flushes the transcript to <root>/projects/<slug>/<sessionID>.jsonl.
func (b *transcriptBuilder) write(root, slug string) {
	b.t.Helper()
	dir := filepath.Join(root, "projects", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		b.t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, b.sess+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(b.lines, "")), 0o644); err != nil {
		b.t.Fatalf("write %s: %v", path, err)
	}
	// Keep the mtime consistent with the last event so an mtime fallback and the
	// event timestamps agree on ordering.
	last := fleetNow
	if n := len(b.lines); n > 0 {
		var o struct {
			Timestamp time.Time `json:"timestamp"`
		}
		if err := json.Unmarshal([]byte(b.lines[n-1]), &o); err == nil && !o.Timestamp.IsZero() {
			last = o.Timestamp
		}
	}
	if err := os.Chtimes(path, last, last); err != nil {
		b.t.Fatalf("chtimes %s: %v", path, err)
	}
}

const (
	idNeedsYou = "aa000001-0000-4000-8000-000000000001"
	idStuck    = "bb000002-0000-4000-8000-000000000002"
	idWorking  = "cc000003-0000-4000-8000-000000000003"
	idIdle     = "dd000004-0000-4000-8000-000000000004"
)

// needsYouAt / stuckAt / workingAt / idleAt each write one transcript that lands
// in exactly one state when evaluated at fleetNow.
func needsYouAt(t *testing.T, root, slug, id string, since time.Duration) {
	newTranscript(t, id, "/home/user/alpha", "main").
		prompt(ago(since+2*time.Minute), "review the migration plan").
		text(ago(since), "The plan is drafted. Shall I apply it now?").
		write(root, slug)
}

func stuckAt(t *testing.T, root, slug, id string, quiet time.Duration) {
	newTranscript(t, id, "/home/user/alpha", "release/v0").
		prompt(ago(quiet+time.Minute), "build the release binary").
		tool(ago(quiet), "toolu_"+id[:4], "Bash", map[string]any{"command": "go build ./..."}).
		write(root, slug)
}

func workingAt(t *testing.T, root, slug, id string, quiet time.Duration) {
	newTranscript(t, id, "/home/user/beta", "feat/auth").
		prompt(ago(quiet+30*time.Second), "run the auth tests").
		tool(ago(quiet), "toolu_"+id[:4], "Bash", map[string]any{"command": "pytest tests/auth -x"}).
		write(root, slug)
}

func idleAt(t *testing.T, root, slug, id string, since time.Duration) {
	newTranscript(t, id, "/home/user/beta", "main").
		prompt(ago(since+time.Minute), "tidy the imports").
		text(ago(since), "Done. Imports are grouped and gofmt is clean.").
		write(root, slug)
}

func statesOf(sessions []fleet.Session) []string {
	out := make([]string, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.Snap.State.String())
	}
	return out
}

func sessionIDs(sessions []fleet.Session) []string {
	out := make([]string, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.Info.ID)
	}
	return out
}

// ---------------------------------------------------------------- T14

// T14 — the fleet is sorted by state: NeedsYou, Stuck, Working, Idle.
func TestT14ManagerRefreshOrdersByState(t *testing.T) {
	root := t.TempDir()
	// Deliberately written in the wrong order and split across two projects.
	idleAt(t, root, "-home-user-beta", idIdle, 14*time.Minute)
	workingAt(t, root, "-home-user-beta", idWorking, 10*time.Second)
	stuckAt(t, root, "-home-user-alpha", idStuck, 5*time.Minute)
	needsYouAt(t, root, "-home-user-alpha", idNeedsYou, 8*time.Minute)

	sessions, err := liveManager(root).Refresh(fleetNow)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(sessions) != 4 {
		t.Fatalf("Refresh returned %d sessions %v, want 4", len(sessions), sessionIDs(sessions))
	}

	wantStates := []string{"needs-you", "stuck", "working", "idle"}
	if got := statesOf(sessions); strings.Join(got, ",") != strings.Join(wantStates, ",") {
		t.Fatalf("states = %v, want %v", got, wantStates)
	}
	wantIDs := []string{idNeedsYou, idStuck, idWorking, idIdle}
	if got := sessionIDs(sessions); strings.Join(got, ",") != strings.Join(wantIDs, ",") {
		t.Errorf("ids = %v, want %v", got, wantIDs)
	}

	// The Info half of a Session is the discovery record, fully populated.
	top := sessions[0]
	if top.Info.ProjectSlug != "-home-user-alpha" {
		t.Errorf("Info.ProjectSlug = %q, want %q", top.Info.ProjectSlug, "-home-user-alpha")
	}
	if top.Info.Title != "review the migration plan" {
		t.Errorf("Info.Title = %q, want the first user prompt", top.Info.Title)
	}
	if top.Info.CWD != "/home/user/alpha" {
		t.Errorf("Info.CWD = %q", top.Info.CWD)
	}
	if top.Snap.Reason != "turn ended with a question" {
		t.Errorf("Snap.Reason = %q, want %q", top.Snap.Reason, "turn ended with a question")
	}
	if !top.Snap.Since.Equal(ago(8 * time.Minute)) {
		t.Errorf("Snap.Since = %v, want %v", top.Snap.Since, ago(8*time.Minute))
	}
}

// Within each state band: needs-you and stuck longest-waiting first, working and
// idle most-recent first.
func TestManagerRefreshOrdersWithinEachState(t *testing.T) {
	root := t.TempDir()
	const slug = "-home-user-alpha"

	nyOld := "a1000000-0000-4000-8000-000000000001"
	nyNew := "a2000000-0000-4000-8000-000000000002"
	stLong := "b1000000-0000-4000-8000-000000000003"
	stShort := "b2000000-0000-4000-8000-000000000004"
	wkNew := "c1000000-0000-4000-8000-000000000005"
	wkOld := "c2000000-0000-4000-8000-000000000006"
	idNew := "d1000000-0000-4000-8000-000000000007"
	idOld := "d2000000-0000-4000-8000-000000000008"

	needsYouAt(t, root, slug, nyNew, 2*time.Minute)
	needsYouAt(t, root, slug, nyOld, 20*time.Minute)
	stuckAt(t, root, slug, stShort, 3*time.Minute)
	stuckAt(t, root, slug, stLong, 30*time.Minute)
	workingAt(t, root, slug, wkOld, 80*time.Second)
	workingAt(t, root, slug, wkNew, 5*time.Second)
	idleAt(t, root, slug, idOld, 45*time.Minute)
	idleAt(t, root, slug, idNew, 1*time.Minute)

	sessions, err := liveManager(root).Refresh(fleetNow)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	want := []string{nyOld, nyNew, stLong, stShort, wkNew, wkOld, idNew, idOld}
	if got := sessionIDs(sessions); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v\nwant     = %v\nstates   = %v", got, want, statesOf(sessions))
	}
}

func TestManagerRefreshOnMissingRootIsEmptyNotAnError(t *testing.T) {
	sessions, err := fleet.NewManager(filepath.Join(t.TempDir(), "no-such-home")).Refresh(fleetNow)
	if err != nil {
		t.Fatalf("Refresh on a missing root returned error %v, want nil", err)
	}
	if len(sessions) != 0 {
		t.Errorf("Refresh on a missing root returned %d sessions, want 0", len(sessions))
	}
}

func TestManagerRefreshIsRepeatableAndPicksUpNewLines(t *testing.T) {
	root := t.TempDir()
	const slug = "-home-user-alpha"
	id := "ee000005-0000-4000-8000-000000000005"

	// A pending Bash: working.
	newTranscript(t, id, "/home/user/alpha", "main").
		prompt(ago(30*time.Second), "run the auth tests").
		tool(ago(10*time.Second), "toolu_ee", "Bash", map[string]any{"command": "pytest -x"}).
		write(root, slug)

	mgr := liveManager(root)
	first, err := mgr.Refresh(fleetNow)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(first) != 1 || first[0].Snap.State != state.Working {
		t.Fatalf("first Refresh = %v, want one working session", statesOf(first))
	}

	// A second Refresh at the same instant must be stable, not double-counted.
	second, err := mgr.Refresh(fleetNow)
	if err != nil {
		t.Fatalf("second Refresh: %v", err)
	}
	if len(second) != 1 || second[0].Snap.State != state.Working {
		t.Fatalf("second Refresh = %v, want the same single working session", statesOf(second))
	}

	// Now the tool result and a closing question land; the session must flip.
	newTranscript(t, id, "/home/user/alpha", "main").
		prompt(ago(30*time.Second), "run the auth tests").
		tool(ago(10*time.Second), "toolu_ee", "Bash", map[string]any{"command": "pytest -x"}).
		result(ago(2*time.Second), "toolu_ee", "2 failed, 8 passed").
		text(fleetNow, "Two tests fail. Want me to fix them?").
		write(root, slug)

	third, err := mgr.Refresh(fleetNow.Add(time.Second))
	if err != nil {
		t.Fatalf("third Refresh: %v", err)
	}
	if len(third) != 1 {
		t.Fatalf("third Refresh returned %d sessions, want 1", len(third))
	}
	if third[0].Snap.State != state.NeedsYou {
		t.Errorf("after the closing question: state = %s, want needs-you", third[0].Snap.State)
	}
}

// ---------------------------------------------------------------- T15

// T15 — the one-shot status line for `compass status` / tmux status-right.
func TestT15StatusLine(t *testing.T) {
	t.Run("mixed fleet", func(t *testing.T) {
		root := t.TempDir()
		needsYouAt(t, root, "-home-user-alpha", idNeedsYou, 8*time.Minute)
		stuckAt(t, root, "-home-user-alpha", idStuck, 5*time.Minute)
		workingAt(t, root, "-home-user-beta", idWorking, 10*time.Second)
		workingAt(t, root, "-home-user-beta", "cc000009-0000-4000-8000-000000000009", 40*time.Second)

		got := liveManager(root).StatusLine(fleetNow)
		if want := "▲1 ◍1 ●2"; got != want {
			t.Errorf("StatusLine = %q, want %q", got, want)
		}
	})

	t.Run("all quiet", func(t *testing.T) {
		root := t.TempDir()
		idleAt(t, root, "-home-user-alpha", idIdle, 14*time.Minute)
		idleAt(t, root, "-home-user-beta", "dd000009-0000-4000-8000-000000000009", 2*time.Hour)

		got := liveManager(root).StatusLine(fleetNow)
		if want := "○ all quiet"; got != want {
			t.Errorf("StatusLine = %q, want %q", got, want)
		}
	})

	t.Run("empty fleet is also all quiet", func(t *testing.T) {
		got := fleet.NewManager(filepath.Join(t.TempDir(), "no-such-home")).StatusLine(fleetNow)
		if want := "○ all quiet"; got != want {
			t.Errorf("StatusLine = %q, want %q", got, want)
		}
	})

	t.Run("idle counted alongside active sessions", func(t *testing.T) {
		root := t.TempDir()
		needsYouAt(t, root, "-home-user-alpha", idNeedsYou, 8*time.Minute)
		idleAt(t, root, "-home-user-beta", idIdle, 14*time.Minute)

		// CONTRACT AMBIGUITY (M0-CONTRACT.md, Manager.StatusLine): the rule is
		// "counts in fleet-sort order, zero counts omitted", which reads as
		// "▲1 ○1" — idle is a non-zero count, so it is printed. The other
		// reading is that ○ is reserved for the "○ all quiet" sentinel and idle
		// sessions are never counted, which yields "▲1". This test pins the
		// literal wording; if the intent is the second reading, amend the
		// contract (and this assertion) rather than silently diverging.
		got := liveManager(root).StatusLine(fleetNow)
		if want := "▲1 ○1"; got != want {
			t.Errorf("StatusLine = %q, want %q — see the CONTRACT AMBIGUITY note above", got, want)
		}
	})
}
