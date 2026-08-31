package fleet_test

// M6 identity (docs/dev/M6-CONTRACT.md, "Identity: the transcript path").
//
// The field finding: two transcripts on the dogfood machine share one session
// id under different project slugs, because a session that changes directory
// keeps its id and starts writing under the new slug. compass keyed everything
// by id, so the pair drew two selection markers, shared one pane and fought
// over a single tailer — an archived session appeared live.
//
// The contract's fix is one line: `func (i SessionInfo) Key() string` returns
// the transcript path, and every map, selection and cache keys by it. These
// tests build that exact pair and hold the engine to it:
//
//	T62 — two SessionInfos, same ID, different slugs: distinct Key(); the
//	      Manager tracks both, each with its own state; neither inherits the
//	      other's liveness or snapshot, and repeat Refreshes do not flip-flop.
//	T63 — MarkPaneMapped + MapSessions under duplicate ids: only the KEYED
//	      session is live and mapped; the twin stays archived and paneless,
//	      and swapping the key swaps which twin wins — it follows the key, not
//	      the id.
//
// Deterministic and offline: every instant is an explicit `now` or a timestamp
// written into the fixture, no tmux server is contacted, and the process tree
// is a fake.

import (
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/tmuxop"
)

// ---------------------------------------------------------------- the twins

// idTwin is the one session id both transcripts carry. Two entries sharing an
// id are two sessions (contract, rule 2) — never one.
const idTwin = "7a5e0001-0000-4000-8000-00000000d00d"

// The two homes the same id ended up writing under: the session started in the
// repo root and cd'd into the worker, which is all it takes.
const (
	slugTwinRoot   = "-home-user-api"
	slugTwinWorker = "-home-user-api-worker"
	cwdTwinRoot    = "/home/user/api"
	cwdTwinWorker  = "/home/user/api/worker"
)

// twinNeedsYou writes a needs-you-shaped transcript for `id` under `slug` with
// cwd `cwd`, whose last event is `since` before fleetNow, and returns its path
// — which the contract says is its Key().
func twinNeedsYou(t *testing.T, root, slug, id, cwd string, since time.Duration) string {
	t.Helper()
	newTranscript(t, id, cwd, "main").
		prompt(ago(since+2*time.Minute), "review the migration plan").
		text(ago(since), "The plan is drafted. Shall I apply it now?").
		write(root, slug)
	return transcriptPath(root, slug, id)
}

// twinIdle is the same, ending on a completed turn.
func twinIdle(t *testing.T, root, slug, id, cwd string, since time.Duration) string {
	t.Helper()
	newTranscript(t, id, cwd, "main").
		prompt(ago(since+time.Minute), "tidy the imports").
		text(ago(since), "Done. Imports are grouped and gofmt is clean.").
		write(root, slug)
	return transcriptPath(root, slug, id)
}

// ---------------------------------------------------------------- key views

// keysOf, liveKeysOf and archivedKeysOf are the id-blind counterparts of
// sessionIDs / liveIDs / archivedIDs: under duplicate ids those three cannot
// tell the twins apart, which is the whole point.
func keysOf(sessions []fleet.Session) []string {
	out := make([]string, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.Info.Key())
	}
	return out
}

func liveKeysOf(sessions []fleet.Session) []string {
	var out []string
	for _, s := range sessions {
		if s.Live {
			out = append(out, s.Info.Key())
		}
	}
	return out
}

func archivedKeysOf(sessions []fleet.Session) []string {
	var out []string
	for _, s := range sessions {
		if !s.Live {
			out = append(out, s.Info.Key())
		}
	}
	return out
}

// pickByKey is `pick` for a fleet where the ids collide.
func pickByKey(t *testing.T, sessions []fleet.Session, key string) fleet.Session {
	t.Helper()
	var found []fleet.Session
	for _, s := range sessions {
		if s.Info.Key() == key {
			found = append(found, s)
		}
	}
	switch len(found) {
	case 1:
		return found[0]
	case 0:
		t.Fatalf("no session with key %s in fleet %v", key, keysOf(sessions))
	default:
		t.Fatalf("%d sessions share key %s: a key must identify exactly one session", len(found), key)
	}
	return fleet.Session{}
}

func infosOf(sessions []fleet.Session) []fleet.SessionInfo {
	out := make([]fleet.SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.Info)
	}
	return out
}

// mapKeys lists a MapSessions result's keys, sorted-insensitively via joined()
// only where the test has already pinned the count.
func mapKeys(m map[string]tmuxop.Pane) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ---------------------------------------------------------------- proc fake

// twinProc is a process tree in four maps, the same shape tmuxop's own tests
// use. Anything absent is simply unknown, exactly as an unreadable /proc entry
// would be.
type twinProc struct {
	children map[int][]int
	comm     map[int]string
	cmdline  map[int]string
	cwd      map[int]string
}

func (p *twinProc) Children(pid int) []int { return append([]int(nil), p.children[pid]...) }
func (p *twinProc) Comm(pid int) string    { return p.comm[pid] }
func (p *twinProc) Cmdline(pid int) string { return p.cmdline[pid] }
func (p *twinProc) Cwd(pid int) string     { return p.cwd[pid] }

var _ tmuxop.Proc = (*twinProc)(nil)

// paneProc builds a tree where each given pane pid owns a shell owning a claude
// sitting in the given cwd — the shape MapSessions pairs by.
func paneProc(claudeAt map[int]string) *twinProc {
	p := &twinProc{
		children: map[int][]int{},
		comm:     map[int]string{},
		cmdline:  map[int]string{},
		cwd:      map[int]string{},
	}
	for pid, cwd := range claudeAt {
		child := pid + 1
		p.children[pid] = []int{child}
		p.comm[pid] = "zsh"
		p.cmdline[pid] = "-zsh"
		p.comm[child] = "claude"
		p.cmdline[child] = "claude --resume"
		p.cwd[child] = cwd
	}
	return p
}

// ---------------------------------------------------------------- T62

// twinRoot writes the dogfood pair: one id, two slugs, two cwds. The one under
// the worker slug is fresh and needs-you-shaped; the one under the repo root is
// idle and hours old. Returns (freshKey, staleKey).
func twinRoot(t *testing.T) (root, freshKey, staleKey string) {
	t.Helper()
	root = t.TempDir()
	freshKey = twinNeedsYou(t, root, slugTwinWorker, idTwin, cwdTwinWorker, 1*time.Minute)
	staleKey = twinIdle(t, root, slugTwinRoot, idTwin, cwdTwinRoot, 4*time.Hour)
	if freshKey == staleKey {
		t.Fatalf("fixture broken: both twins landed on %s", freshKey)
	}
	return root, freshKey, staleKey
}

// Key() is the transcript path — the contract's whole definition of identity.
func TestT62KeyIsTheTranscriptPath(t *testing.T) {
	i := fleet.SessionInfo{
		ID:             idTwin,
		TranscriptPath: "/home/user/.claude/projects/-home-user-api/" + idTwin + ".jsonl",
		ProjectSlug:    slugTwinRoot,
		CWD:            cwdTwinRoot,
	}
	if got := i.Key(); got != i.TranscriptPath {
		t.Errorf("Key() = %q, want TranscriptPath %q", got, i.TranscriptPath)
	}
	if got := i.Key(); got == i.ID {
		t.Errorf("Key() = %q, which is the session id: the id is a label, not a key", got)
	}

	// Same id, other slug: a different session.
	j := i
	j.TranscriptPath = "/home/user/.claude/projects/-home-user-api-worker/" + idTwin + ".jsonl"
	j.ProjectSlug = slugTwinWorker
	j.CWD = cwdTwinWorker
	if i.Key() == j.Key() {
		t.Errorf("two transcripts of the same id share the key %q", i.Key())
	}
	if i.ID != j.ID {
		t.Fatalf("fixture broken: the ids differ (%q, %q), so nothing was proved", i.ID, j.ID)
	}

	// Key() is a pure read: calling it twice cannot drift, or every map misses.
	if a, b := i.Key(), i.Key(); a != b {
		t.Errorf("Key() is not deterministic: %q then %q", a, b)
	}
}

// T62 — the Manager tracks both twins, each with its own state, and the stale
// one inherits nothing from the fresh one.
func TestT62DuplicateIDIsTwoSessions(t *testing.T) {
	root, freshKey, staleKey := twinRoot(t)

	m := fleet.NewManager(root)
	sessions := mustRefresh(t, m, fleetNow)

	if len(sessions) != 2 {
		t.Fatalf("Refresh returned %d session(s) %v, want both twins %v",
			len(sessions), keysOf(sessions), []string{freshKey, staleKey})
	}

	fresh := pickByKey(t, sessions, freshKey)
	stale := pickByKey(t, sessions, staleKey)

	t.Run("one id, two keys", func(t *testing.T) {
		if fresh.Info.ID != idTwin || stale.Info.ID != idTwin {
			t.Fatalf("fixture broken: ids are %q and %q, want both %q — the id must survive as a label",
				fresh.Info.ID, stale.Info.ID, idTwin)
		}
		if fresh.Info.Key() == stale.Info.Key() {
			t.Fatalf("both twins report key %q", fresh.Info.Key())
		}
		if fresh.Info.Key() != fresh.Info.TranscriptPath || stale.Info.Key() != stale.Info.TranscriptPath {
			t.Errorf("Key() is not the transcript path: %q vs %q, %q vs %q",
				fresh.Info.Key(), fresh.Info.TranscriptPath,
				stale.Info.Key(), stale.Info.TranscriptPath)
		}
	})

	t.Run("each keeps its own identity", func(t *testing.T) {
		if fresh.Info.ProjectSlug != slugTwinWorker || fresh.Info.CWD != cwdTwinWorker {
			t.Errorf("fresh twin: slug %q cwd %q, want %q and %q",
				fresh.Info.ProjectSlug, fresh.Info.CWD, slugTwinWorker, cwdTwinWorker)
		}
		if stale.Info.ProjectSlug != slugTwinRoot || stale.Info.CWD != cwdTwinRoot {
			t.Errorf("stale twin: slug %q cwd %q, want %q and %q",
				stale.Info.ProjectSlug, stale.Info.CWD, slugTwinRoot, cwdTwinRoot)
		}
		if !fresh.Info.LastEventAt.Equal(ago(1 * time.Minute)) {
			t.Errorf("fresh twin LastEventAt = %v, want %v", fresh.Info.LastEventAt, ago(1*time.Minute))
		}
		if !stale.Info.LastEventAt.Equal(ago(4 * time.Hour)) {
			t.Errorf("stale twin LastEventAt = %v, want %v", stale.Info.LastEventAt, ago(4*time.Hour))
		}
		if fresh.Info.Title == stale.Info.Title {
			t.Errorf("both twins carry the title %q; each transcript has its own opening prompt",
				fresh.Info.Title)
		}
	})

	t.Run("each gets its own state", func(t *testing.T) {
		if fresh.Snap.State != state.NeedsYou {
			t.Errorf("fresh twin state = %s, want needs-you", fresh.Snap.State)
		}
		if fresh.Snap.Reason != "turn ended with a question" {
			t.Errorf("fresh twin Reason = %q, want %q", fresh.Snap.Reason, "turn ended with a question")
		}
		if !fresh.Snap.Since.Equal(ago(1 * time.Minute)) {
			t.Errorf("fresh twin Since = %v, want %v", fresh.Snap.Since, ago(1*time.Minute))
		}
	})

	// The load-bearing one. Under an id-keyed map the second entry overwrites
	// the first, and whichever survives lends the other its liveness and its
	// verdict — which is precisely how an archived session appeared live.
	t.Run("the stale twin does not inherit the fresh one's liveness", func(t *testing.T) {
		if !fresh.Live {
			t.Errorf("fresh twin Live = false; 1m old is inside the default 5m window")
		}
		if stale.Live {
			t.Fatalf("stale twin Live = true: 4h old with no pane, it borrowed its twin's liveness")
		}
		if got, want := joined(liveKeysOf(sessions)), freshKey; got != want {
			t.Errorf("live = %v, want only %v", got, want)
		}
		if got, want := joined(archivedKeysOf(sessions)), staleKey; got != want {
			t.Errorf("archive = %v, want only %v", got, want)
		}
	})

	t.Run("the stale twin does not inherit the fresh one's snapshot", func(t *testing.T) {
		assertArchivedSnap(t, stale)
		if stale.Snap.State == state.NeedsYou {
			t.Errorf("stale twin went amber: %+v", stale.Snap)
		}
		if stale.Snap.Since.Equal(fresh.Snap.Since) {
			t.Errorf("both twins are dated %v; the stale one must be dated by its own LastEventAt %v",
				stale.Snap.Since, stale.Info.LastEventAt)
		}
		if stale.Snap.Reason == fresh.Snap.Reason {
			t.Errorf("both twins report %q; the archive has its own reason", stale.Snap.Reason)
		}
	})

	t.Run("the live block comes first", func(t *testing.T) {
		if got, want := joined(keysOf(sessions)), joined([]string{freshKey, staleKey}); got != want {
			t.Errorf("fleet order = %v, want %v (live block, then the archive)", got, want)
		}
		assertLiveFlags(t, sessions, true, false)
	})

	// StatusLine counts live sessions; the archived twin must not be counted a
	// second time under its twin's verdict.
	t.Run("StatusLine counts one", func(t *testing.T) {
		if got, want := m.StatusLine(fleetNow), "▲1"; got != want {
			t.Errorf("StatusLine = %q, want %q", got, want)
		}
	})
}

// The flip-flop probe: an id-keyed map cannot hold both twins, so successive
// Refreshes overwrite each other and the fleet oscillates. Keyed by path it is
// simply stable.
func TestT62RepeatedRefreshesAreStable(t *testing.T) {
	root, freshKey, staleKey := twinRoot(t)
	m := fleet.NewManager(root)

	first := renderFleet(mustRefresh(t, m, fleetNow))
	if len(first) != 2 {
		t.Fatalf("first Refresh returned %d session(s), want 2:\n  %s", len(first), strings.Join(first, "\n  "))
	}

	for i := 1; i < 8; i++ {
		got := renderFleet(mustRefresh(t, m, fleetNow))
		if joined(got) != joined(first) {
			t.Fatalf("Refresh %d disagrees with Refresh 0:\n  got  %s\n  want %s",
				i, strings.Join(got, "\n       "), strings.Join(first, "\n       "))
		}
	}

	// And with time advancing inside the live window, which is how the ui
	// actually calls it — once a second.
	for i := 1; i <= 5; i++ {
		now := fleetNow.Add(time.Duration(i) * time.Second)
		sessions := mustRefresh(t, m, now)
		if len(sessions) != 2 {
			t.Fatalf("tick %d: fleet = %v, want both twins", i, keysOf(sessions))
		}
		if got, want := joined(liveKeysOf(sessions)), freshKey; got != want {
			t.Fatalf("tick %d: live = %v, want %v — the twins are trading places", i, got, want)
		}
		if got, want := joined(archivedKeysOf(sessions)), staleKey; got != want {
			t.Fatalf("tick %d: archive = %v, want %v", i, got, want)
		}
		assertArchivedSnap(t, pickByKey(t, sessions, staleKey))
	}
}

// Discovery itself, one level below the Manager: both transcripts are found and
// their keys differ. If they collide here, nothing above can recover.
func TestT62DiscoverFindsBothTwins(t *testing.T) {
	root, freshKey, staleKey := twinRoot(t)

	infos, err := fleet.Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("Discover returned %d record(s), want 2", len(infos))
	}

	seen := map[string]fleet.SessionInfo{}
	for _, i := range infos {
		if i.ID != idTwin {
			t.Errorf("record %s carries id %q, want %q", i.Key(), i.ID, idTwin)
		}
		if prev, dup := seen[i.Key()]; dup {
			t.Fatalf("two records share key %s (%q and %q)", i.Key(), prev.ProjectSlug, i.ProjectSlug)
		}
		seen[i.Key()] = i
	}
	for _, key := range []string{freshKey, staleKey} {
		if _, ok := seen[key]; !ok {
			t.Errorf("Discover missed %s; it found %v", key, func() []string {
				var out []string
				for k := range seen {
					out = append(out, k)
				}
				return out
			}())
		}
	}
}

// The twins' own tailers: appending to one moves that one only. A shared tailer
// is the third symptom the contract names.
func TestT62AppendingToOneTwinLeavesTheOtherAlone(t *testing.T) {
	root := t.TempDir()
	keyA := twinNeedsYou(t, root, slugTwinWorker, idTwin, cwdTwinWorker, 1*time.Minute)
	keyB := twinIdle(t, root, slugTwinRoot, idTwin, cwdTwinRoot, 2*time.Minute)

	m := fleet.NewManager(root)
	before := mustRefresh(t, m, fleetNow)
	if a, b := pickByKey(t, before, keyA), pickByKey(t, before, keyB); !a.Live || !b.Live {
		t.Fatalf("fixture broken: both twins should start live (%v, %v)", a.Live, b.Live)
	}
	beforeB := renderFleet([]fleet.Session{pickByKey(t, before, keyB)})[0]

	// Twin A's session runs a tool: it goes to work. Twin B is untouched.
	appendLines(t, root, slugTwinWorker, idTwin,
		continueTranscript(t, idTwin, cwdTwinWorker, "main", 2).
			tool(fleetNow, "toolu_twin", "Bash", map[string]any{"command": "go test ./..."}),
		fleetNow)

	after := mustRefresh(t, m, fleetNow.Add(time.Second))
	a := pickByKey(t, after, keyA)
	if a.Snap.State != state.Working {
		t.Errorf("twin A state = %s, want working — its own new tool_use", a.Snap.State)
	}
	b := pickByKey(t, after, keyB)
	if b.Snap.State != state.Idle {
		t.Errorf("twin B state = %s, want idle — nothing was written to its transcript", b.Snap.State)
	}
	if got := renderFleet([]fleet.Session{b})[0]; got != beforeB {
		t.Errorf("twin B moved when twin A did:\n  got  %s\n  want %s", got, beforeB)
	}
}

// ---------------------------------------------------------------- T63

// t63Root writes two twins that are BOTH too old for the recency door, so the
// only thing that can make either live is a pane. Returns (rootKey, workerKey).
func t63Root(t *testing.T) (root, rootKey, workerKey string) {
	t.Helper()
	root = t.TempDir()
	rootKey = twinIdle(t, root, slugTwinRoot, idTwin, cwdTwinRoot, 3*time.Hour)
	workerKey = twinIdle(t, root, slugTwinWorker, idTwin, cwdTwinWorker, 4*time.Hour)
	return root, rootKey, workerKey
}

// paneAt is the pane the twin at `cwd` owns.
func paneAt(target, id string, pid int, cwd string) tmuxop.Pane {
	return tmuxop.Pane{Target: target, ID: id, PID: pid, Path: cwd, Command: "claude"}
}

// T63 — one pane, two same-id sessions: MapSessions keys the pane by the
// winner's Key(), MarkPaneMapped takes that key, and only that twin is live.
func TestT63PaneBelongsToTheKeyedTwin(t *testing.T) {
	root, rootKey, workerKey := t63Root(t)

	m := fleet.NewManager(root)
	infos := infosOf(mustRefresh(t, m, fleetNow))
	if len(infos) != 2 {
		t.Fatalf("fixture broken: discovery found %d session(s), want the two twins", len(infos))
	}

	// One pane, sitting in the root twin's cwd.
	panes := []tmuxop.Pane{paneAt("dev:1.0", "%5", 500, cwdTwinRoot)}
	mapped := tmuxop.MapSessions(infos, panes, paneProc(map[int]string{500: cwdTwinRoot}))

	t.Run("MapSessions keys the pane by the transcript path", func(t *testing.T) {
		if len(mapped) != 1 {
			t.Fatalf("MapSessions returned %d pair(s) %v, want exactly one", len(mapped), mapKeys(mapped))
		}
		pane, ok := mapped[rootKey]
		if !ok {
			t.Fatalf("MapSessions = %v, want it keyed by %s", mapKeys(mapped), rootKey)
		}
		if pane.ID != "%5" || pane.Target != "dev:1.0" {
			t.Errorf("%s → %+v, want the full record for %%5", rootKey, pane)
		}
		if _, ok := mapped[workerKey]; ok {
			t.Errorf("the worker twin got a pane it has no claude for: %v", mapKeys(mapped))
		}
		if _, ok := mapped[idTwin]; ok {
			t.Errorf("MapSessions keyed a pair by the session id %q; the key is the transcript path", idTwin)
		}
	})

	m.MarkPaneMapped(mapped2bool(mapped))
	sessions := mustRefresh(t, m, fleetNow)

	t.Run("only the keyed twin is live", func(t *testing.T) {
		if got, want := joined(liveKeysOf(sessions)), rootKey; got != want {
			t.Errorf("live = %v, want only %v", got, want)
		}
		if got, want := joined(archivedKeysOf(sessions)), workerKey; got != want {
			t.Errorf("archive = %v, want only %v", got, want)
		}
		assertArchivedSnap(t, pickByKey(t, sessions, workerKey))
		if got, want := m.StatusLine(fleetNow), "○ all quiet"; got != want {
			t.Errorf("StatusLine = %q, want %q (one live idle session)", got, want)
		}
	})

	// Now the pane moves to the worker's cwd. Nothing about either file changed
	// and both still carry the same id: only the key decides.
	t.Run("the pane follows the key, not the id", func(t *testing.T) {
		panes := []tmuxop.Pane{paneAt("dev:1.0", "%5", 500, cwdTwinWorker)}
		swapped := tmuxop.MapSessions(infos, panes, paneProc(map[int]string{500: cwdTwinWorker}))
		if len(swapped) != 1 {
			t.Fatalf("MapSessions returned %d pair(s) %v, want exactly one", len(swapped), mapKeys(swapped))
		}
		if _, ok := swapped[workerKey]; !ok {
			t.Fatalf("MapSessions = %v, want it keyed by %s", mapKeys(swapped), workerKey)
		}

		m.MarkPaneMapped(mapped2bool(swapped))
		sessions := mustRefresh(t, m, fleetNow)

		if got, want := joined(liveKeysOf(sessions)), workerKey; got != want {
			t.Errorf("live = %v, want only %v — the pane moved and liveness must move with it", got, want)
		}
		if got, want := joined(archivedKeysOf(sessions)), rootKey; got != want {
			t.Errorf("archive = %v, want only %v", got, want)
		}
		assertArchivedSnap(t, pickByKey(t, sessions, rootKey))
	})
}

// mapped2bool is what the ui does between MapSessions and MarkPaneMapped: keys
// only, verbatim.
func mapped2bool(m map[string]tmuxop.Pane) map[string]bool {
	out := make(map[string]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

// MarkPaneMapped takes keys. A bare session id is not a key, and under
// duplicate ids it cannot be made into one — it names two sessions. Feeding it
// one must light up neither, which is the assertion an id-keyed Manager fails.
func TestT63MarkPaneMappedIgnoresABareSessionID(t *testing.T) {
	root, rootKey, workerKey := t63Root(t)

	m := fleet.NewManager(root)
	m.MarkPaneMapped(paneMap(idTwin))

	sessions := mustRefresh(t, m, fleetNow)
	if got := liveKeysOf(sessions); len(got) != 0 {
		t.Errorf("live = %v, want none: %q is a session id, not a key", got, idTwin)
	}
	for _, key := range []string{rootKey, workerKey} {
		assertArchivedSnap(t, pickByKey(t, sessions, key))
	}
}

// Two panes, one per cwd: the twins take one each and neither shares. Sharing
// one pane between them is the second symptom the contract names.
func TestT63EachTwinGetsItsOwnPane(t *testing.T) {
	root, rootKey, workerKey := t63Root(t)

	m := fleet.NewManager(root)
	infos := infosOf(mustRefresh(t, m, fleetNow))

	panes := []tmuxop.Pane{
		paneAt("dev:2.0", "%9", 900, cwdTwinWorker),
		paneAt("dev:1.0", "%5", 500, cwdTwinRoot),
	}
	mapped := tmuxop.MapSessions(infos, panes,
		paneProc(map[int]string{500: cwdTwinRoot, 900: cwdTwinWorker}))

	if len(mapped) != 2 {
		t.Fatalf("MapSessions returned %d pair(s) %v, want one per twin", len(mapped), mapKeys(mapped))
	}
	if got := mapped[rootKey].ID; got != "%5" {
		t.Errorf("%s → pane %q, want %%5 (the pane whose claude sits in %s)", rootKey, got, cwdTwinRoot)
	}
	if got := mapped[workerKey].ID; got != "%9" {
		t.Errorf("%s → pane %q, want %%9 (the pane whose claude sits in %s)", workerKey, got, cwdTwinWorker)
	}
	if mapped[rootKey].ID == mapped[workerKey].ID {
		t.Fatalf("both twins were handed pane %q; two sessions are two panes", mapped[rootKey].ID)
	}

	m.MarkPaneMapped(mapped2bool(mapped))
	sessions := mustRefresh(t, m, fleetNow)
	if got, want := joined(liveKeysOf(sessions)), joined([]string{rootKey, workerKey}); got != want {
		// Order within the live block is state-then-recency; both are idle, so
		// the newer (root, 3h) comes first.
		t.Errorf("live = %v, want both twins %v", got, want)
	}
	assertLiveFlags(t, sessions, true, true)
}

// The twins share a cwd — the same session id, the same directory, two
// transcripts, one pane. Exactly one may win it; the other stays paneless.
// (Under the M5 rule the newest LastEventAt takes the lowest target.)
func TestT63TwinsSharingACWDStillGetAtMostOnePaneEach(t *testing.T) {
	root := t.TempDir()
	newer := twinIdle(t, root, slugTwinWorker, idTwin, cwdTwinRoot, 3*time.Hour)
	older := twinIdle(t, root, slugTwinRoot, idTwin, cwdTwinRoot, 4*time.Hour)

	m := fleet.NewManager(root)
	infos := infosOf(mustRefresh(t, m, fleetNow))

	panes := []tmuxop.Pane{paneAt("dev:1.0", "%5", 500, cwdTwinRoot)}
	mapped := tmuxop.MapSessions(infos, panes, paneProc(map[int]string{500: cwdTwinRoot}))

	if len(mapped) != 1 {
		t.Fatalf("MapSessions returned %d pair(s) %v, want exactly one — there is one pane",
			len(mapped), mapKeys(mapped))
	}
	if _, ok := mapped[newer]; !ok {
		t.Errorf("MapSessions = %v, want the newer transcript %s to take the only pane", mapKeys(mapped), newer)
	}
	if _, ok := mapped[older]; ok {
		t.Errorf("the older transcript %s also got a pane: %v", older, mapKeys(mapped))
	}

	m.MarkPaneMapped(mapped2bool(mapped))
	sessions := mustRefresh(t, m, fleetNow)
	if got, want := joined(liveKeysOf(sessions)), newer; got != want {
		t.Errorf("live = %v, want only %v", got, want)
	}
	assertArchivedSnap(t, pickByKey(t, sessions, older))
}
