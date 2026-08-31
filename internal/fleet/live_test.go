package fleet_test

// M5 liveness (docs/dev/M5-CONTRACT.md, "package fleet — liveness"):
//
//	1. live = pane-mapped ∪ (LastEventAt within the live window); the rest is
//	   the archive.
//	2. an archived session's Snap is always {Idle, Since: LastEventAt,
//	   Reason: "archived", Activity: "idle"} — the archive can never be amber.
//	3. archive→live gets a tailer from scratch (full replay); live→archive
//	   drops it.
//	4. Refresh returns the live block first (today's state order) and then the
//	   archive, newest LastEventAt first.
//	5. StatusLine counts LIVE sessions only.
//
// Everything here is driven by explicit `now` arguments and by file timestamps
// we set ourselves: no sleeps, no wall clock, no network.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/state"
)

// ---------------------------------------------------------------- helpers

const (
	slugAlpha = "-home-user-alpha" // needsYouAt/stuckAt write here (cwd /home/user/alpha)
	slugBeta  = "-home-user-beta"  // workingAt/idleAt write here  (cwd /home/user/beta)
)

// tsLayout is the timestamp format transcriptBuilder writes. It is fixed width,
// which is what lets the cache probes below rewrite a timestamp in place without
// changing a file's size.
const tsLayout = "2006-01-02T15:04:05.000Z"

// The T54 cast. (a) is stale but sits in a pane; (b) is fresh and paneless;
// (c) and (d) are old and paneless — the archive.
const (
	idStalePane      = "a0000001-0000-4000-8000-0000000000a1" // (a) 3h old, pane-mapped, idle
	idFreshUnmapped  = "b0000002-0000-4000-8000-0000000000b2" // (b) 1m old, no pane, needs-you
	idArchived2h     = "c0000003-0000-4000-8000-0000000000c3" // (c) 2h old, no pane
	idArchived5h     = "d0000004-0000-4000-8000-0000000000d4" // (d) 5h old, no pane
	idMid30m         = "e0000005-0000-4000-8000-0000000000e5" // 30m old, no pane
	idBoundary5m     = "f0000006-0000-4000-8000-0000000000f6" // exactly 5m old, no pane
	idArchQuestion   = "1a000007-0000-4000-8000-00000000a107" // archived, needs-you shaped
	idLiveWorker     = "2b000008-0000-4000-8000-00000000b208" // fresh, working
	idLiveIdler      = "3c000009-0000-4000-8000-00000000c309" // fresh, idle
	idArchTieEarlier = "4d00000a-0000-4000-8000-00000000d40a" // tie on LastEventAt, lower id
	idArchTieLater   = "5e00000b-0000-4000-8000-00000000e50b" // tie on LastEventAt, higher id
)

// paneMap builds the map the ui hands to MarkPaneMapped after a MapSessions.
// It takes session KEYS — transcript paths (M6) — verbatim; a test naming an
// id resolves it with panesFor first.
func paneMap(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, key := range keys {
		m[key] = true
	}
	return m
}

// panesFor is paneMap for tests that think in session ids: it resolves each id
// to the transcript under root that carries it, the same way discovery does.
// Identity is the path, so a test may not hand MarkPaneMapped a bare id.
func panesFor(t *testing.T, root string, ids ...string) map[string]bool {
	t.Helper()
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		found, err := filepath.Glob(filepath.Join(root, "projects", "*", id+".jsonl"))
		if err != nil || len(found) == 0 {
			t.Fatalf("no transcript for %s under %s", id, root)
		}
		keys = append(keys, found...)
	}
	return paneMap(keys...)
}

func mustRefresh(t *testing.T, m *fleet.Manager, now time.Time) []fleet.Session {
	t.Helper()
	sessions, err := m.Refresh(now)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	return sessions
}

// pick returns the session with the given id, failing if the fleet dropped it.
// pick finds the one session carrying an id. Identity is the transcript path
// (M6), so an id CAN name two sessions; every fixture in this file gives each
// session its own, and a duplicate here would mean the fixture — or the
// manager — is not what the test thinks. Better loud than first-match.
func pick(t *testing.T, sessions []fleet.Session, id string) fleet.Session {
	t.Helper()
	var found []fleet.Session
	for _, s := range sessions {
		if s.Info.ID == id {
			found = append(found, s)
		}
	}
	switch len(found) {
	case 1:
		return found[0]
	case 0:
		t.Fatalf("session %s missing from fleet %v", id, sessionIDs(sessions))
	default:
		keys := make([]string, 0, len(found))
		for _, s := range found {
			keys = append(keys, s.Info.Key())
		}
		t.Fatalf("id %s names %d sessions (%v); pick needs exactly one", id, len(found), keys)
	}
	return fleet.Session{}
}

func liveFlags(sessions []fleet.Session) []bool {
	out := make([]bool, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.Live)
	}
	return out
}

// liveIDs / archivedIDs split the fleet the way the ui does.
func liveIDs(sessions []fleet.Session) []string {
	var out []string
	for _, s := range sessions {
		if s.Live {
			out = append(out, s.Info.ID)
		}
	}
	return out
}

func archivedIDs(sessions []fleet.Session) []string {
	var out []string
	for _, s := range sessions {
		if !s.Live {
			out = append(out, s.Info.ID)
		}
	}
	return out
}

func joined(ss []string) string { return strings.Join(ss, ",") }

func assertOrder(t *testing.T, sessions []fleet.Session, want ...string) {
	t.Helper()
	if got := sessionIDs(sessions); joined(got) != joined(want) {
		t.Fatalf("fleet order = %v\n            want = %v\n  live = %v states = %v",
			got, want, liveFlags(sessions), statesOf(sessions))
	}
}

func assertLiveFlags(t *testing.T, sessions []fleet.Session, want ...bool) {
	t.Helper()
	got := liveFlags(sessions)
	if len(got) != len(want) {
		t.Fatalf("fleet has %d sessions %v, want %d", len(got), sessionIDs(sessions), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("Live flags = %v, want %v (order %v)", got, want, sessionIDs(sessions))
		}
	}
}

// assertArchivedSnap pins rule 2 in full: an archived session is idle, dated by
// its own LastEventAt, and says so.
func assertArchivedSnap(t *testing.T, s fleet.Session) {
	t.Helper()
	if s.Live {
		t.Errorf("%s: Live = true, want false", s.Info.ID)
	}
	if s.Snap.State != state.Idle {
		t.Errorf("%s: archived state = %s, want idle — the archive can never be amber",
			s.Info.ID, s.Snap.State)
	}
	if s.Snap.Reason != "archived" {
		t.Errorf("%s: archived Reason = %q, want %q", s.Info.ID, s.Snap.Reason, "archived")
	}
	if s.Snap.Activity != "idle" {
		t.Errorf("%s: archived Activity = %q, want %q", s.Info.ID, s.Snap.Activity, "idle")
	}
	if !s.Snap.Since.Equal(s.Info.LastEventAt) {
		t.Errorf("%s: archived Since = %v, want Info.LastEventAt %v",
			s.Info.ID, s.Snap.Since, s.Info.LastEventAt)
	}
}

// bookkeeping appends a line the state machine must ignore (an unrecognised
// `type`, as mode latches and prompt markers have) that still carries a
// timestamp. It moves LastEventAt without moving the verdict — which is what
// lets a test tell "Since = LastEventAt" apart from "Since = the last real
// beat".
func (b *transcriptBuilder) bookkeeping(ts time.Time) *transcriptBuilder {
	o := b.common(ts)
	o["type"] = "mode"
	o["mode"] = "normal"
	return b.add(o)
}

// continueTranscript returns a builder whose uuid chain picks up after `written`
// lines, so its output can be appended to an existing transcript.
func continueTranscript(t *testing.T, sess, cwd, branch string, written int) *transcriptBuilder {
	t.Helper()
	b := newTranscript(t, sess, cwd, branch)
	b.n = written
	return b
}

func transcriptPath(root, slug, id string) string {
	return filepath.Join(root, "projects", slug, id+".jsonl")
}

// appendLines appends a builder's lines to an existing transcript and stamps the
// file's mtime, the way a running session grows its own file.
func appendLines(t *testing.T, root, slug, id string, b *transcriptBuilder, mtime time.Time) {
	t.Helper()
	path := transcriptPath(root, slug, id)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open %s for append: %v", path, err)
	}
	if _, err := f.WriteString(strings.Join(b.lines, "")); err != nil {
		f.Close()
		t.Fatalf("append to %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

// askedQuestionAt writes a transcript whose last *real* beat is the model asking
// a question — the shape that reads NeedsYou — followed by a timestamped
// bookkeeping line, so LastEventAt (age) and the question's instant (Since) are
// deliberately different values.
func askedQuestionAt(t *testing.T, root, slug, id string, age time.Duration) (askedAt, lastAt time.Time) {
	t.Helper()
	askedAt = ago(age + 5*time.Minute)
	lastAt = ago(age)
	newTranscript(t, id, "/home/user/alpha", "main").
		prompt(ago(age+10*time.Minute), "review the migration plan").
		text(askedAt, "The plan is drafted. Shall I proceed?").
		bookkeeping(lastAt).
		write(root, slug)
	return askedAt, lastAt
}

// ---------------------------------------------------------------- T54

// t54Root builds the four-session cast the contract's T54 row describes.
//
//	(a) idStalePane     — 3h stale, will be pane-mapped, idle  → LIVE
//	(b) idFreshUnmapped — 1m old, no pane (inside the 5m door)  → LIVE
//	(c) idArchived2h    — 2h old, no pane, needs-you SHAPED     → archived
//	(d) idArchived5h    — 5h old, no pane                       → archived
//
// The ages are chosen adversarially: (a) is *older* than (c) and both evaluate
// to idle, so a fleet sorted globally by state-then-recency would put the
// archived (c) above the live (a). Rule 4 says the live block comes first.
func t54Root(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	idleAt(t, root, slugBeta, idStalePane, 3*time.Hour)
	needsYouAt(t, root, slugAlpha, idFreshUnmapped, 1*time.Minute)
	needsYouAt(t, root, slugAlpha, idArchived2h, 2*time.Hour)
	idleAt(t, root, slugBeta, idArchived5h, 5*time.Hour)
	return root
}

// T54 — the partition, the Live flags, and the order.
func TestT54LivePartitionFlagsAndOrder(t *testing.T) {
	root := t54Root(t)
	m := fleet.NewManager(root)
	m.MarkPaneMapped(panesFor(t, root, idStalePane))

	sessions := mustRefresh(t, m, fleetNow)
	if len(sessions) != 4 {
		t.Fatalf("Refresh returned %d sessions %v, want 4", len(sessions), sessionIDs(sessions))
	}

	// Rule 4: the live block in today's state order (needs-you before idle),
	// then the archive newest-LastEventAt-first.
	assertOrder(t, sessions, idFreshUnmapped, idStalePane, idArchived2h, idArchived5h)
	assertLiveFlags(t, sessions, true, true, false, false)

	if got, want := joined(liveIDs(sessions)), joined([]string{idFreshUnmapped, idStalePane}); got != want {
		t.Errorf("live block = %v, want %v", got, want)
	}
	if got, want := joined(archivedIDs(sessions)), joined([]string{idArchived2h, idArchived5h}); got != want {
		t.Errorf("archive = %v, want %v", got, want)
	}

	t.Run("a: a pane keeps a stale session live", func(t *testing.T) {
		a := pick(t, sessions, idStalePane)
		if !a.Live {
			t.Fatalf("%s: Live = false; a pane-mapped session is live however stale it is", a.Info.ID)
		}
		if !a.Info.LastEventAt.Equal(ago(3 * time.Hour)) {
			t.Errorf("LastEventAt = %v, want %v", a.Info.LastEventAt, ago(3*time.Hour))
		}
		// Live means really evaluated, not stamped: this one is genuinely idle.
		if a.Snap.State != state.Idle || a.Snap.Reason != "turn complete" {
			t.Errorf("snap = {%s %q}, want {idle \"turn complete\"} — a live session carries its real state",
				a.Snap.State, a.Snap.Reason)
		}
	})

	t.Run("b: the recency door keeps a paneless session live", func(t *testing.T) {
		b := pick(t, sessions, idFreshUnmapped)
		if !b.Live {
			t.Fatalf("%s: Live = false; 1m old is inside the default 5m window", b.Info.ID)
		}
		if b.Snap.State != state.NeedsYou {
			t.Errorf("state = %s, want needs-you — a session writing right now must never be hidden",
				b.Snap.State)
		}
		if !b.Snap.Since.Equal(ago(1 * time.Minute)) {
			t.Errorf("Since = %v, want %v", b.Snap.Since, ago(1*time.Minute))
		}
	})

	t.Run("c and d: old and paneless is the archive", func(t *testing.T) {
		c := pick(t, sessions, idArchived2h)
		assertArchivedSnap(t, c)
		// (c)'s transcript ends on a question: the shape screams needs-you and
		// the archive must refuse to say so.
		if c.Snap.State == state.NeedsYou {
			t.Errorf("%s: the archive went amber", c.Info.ID)
		}
		if !c.Info.LastEventAt.Equal(ago(2 * time.Hour)) {
			t.Errorf("LastEventAt = %v, want %v", c.Info.LastEventAt, ago(2*time.Hour))
		}
		assertArchivedSnap(t, pick(t, sessions, idArchived5h))
	})

	t.Run("the partition is stable across refreshes", func(t *testing.T) {
		again := mustRefresh(t, m, fleetNow)
		assertOrder(t, again, idFreshUnmapped, idStalePane, idArchived2h, idArchived5h)
		assertLiveFlags(t, again, true, true, false, false)
	})
}

// The archive tie-break: equal LastEventAt falls back to the session id.
func TestT54ArchiveTiesBreakOnID(t *testing.T) {
	root := t.TempDir()
	// Same age to the nanosecond, written in the order that would fail if the
	// tie-break were "whatever the directory walk found first".
	idleAt(t, root, slugBeta, idArchTieLater, 3*time.Hour)
	idleAt(t, root, slugAlpha, idArchTieEarlier, 3*time.Hour)
	// One live session, so the assertion also covers "archive after live".
	workingAt(t, root, slugBeta, idLiveWorker, 10*time.Second)

	sessions := mustRefresh(t, fleet.NewManager(root), fleetNow)
	assertOrder(t, sessions, idLiveWorker, idArchTieEarlier, idArchTieLater)
	assertLiveFlags(t, sessions, true, false, false)

	a := pick(t, sessions, idArchTieEarlier)
	b := pick(t, sessions, idArchTieLater)
	if !a.Info.LastEventAt.Equal(b.Info.LastEventAt) {
		t.Fatalf("fixture broken: LastEventAt %v != %v, so the tie was never exercised",
			a.Info.LastEventAt, b.Info.LastEventAt)
	}
}

// An all-archive fleet is still a fleet: every entry present, none of them live.
func TestT54AllArchivedFleet(t *testing.T) {
	root := t.TempDir()
	needsYouAt(t, root, slugAlpha, idArchived2h, 2*time.Hour)
	idleAt(t, root, slugBeta, idArchived5h, 5*time.Hour)
	stuckAt(t, root, slugAlpha, idStalePane, 90*time.Minute)

	sessions := mustRefresh(t, fleet.NewManager(root), fleetNow)
	if len(sessions) != 3 {
		t.Fatalf("fleet = %v, want all three sessions", sessionIDs(sessions))
	}
	// Newest LastEventAt first: 90m, 2h, 5h.
	assertOrder(t, sessions, idStalePane, idArchived2h, idArchived5h)
	for _, s := range sessions {
		assertArchivedSnap(t, s)
	}
}

// ---------------------------------------------------------------- T55

// T55 — the archive is never amber, StatusLine ignores it, and crossing into
// live replays the transcript so the real state appears.
func TestT55ArchivedSnapshotIsAlwaysArchivedIdle(t *testing.T) {
	root := t.TempDir()
	askedAt, lastAt := askedQuestionAt(t, root, slugAlpha, idArchQuestion, 2*time.Hour)
	if askedAt.Equal(lastAt) {
		t.Fatalf("fixture broken: askedAt == lastAt, so Since cannot be told from LastEventAt")
	}

	sessions := mustRefresh(t, fleet.NewManager(root), fleetNow)
	if len(sessions) != 1 {
		t.Fatalf("fleet = %v, want one session", sessionIDs(sessions))
	}
	s := sessions[0]

	assertArchivedSnap(t, s)
	if !s.Snap.Since.Equal(lastAt) {
		t.Errorf("Since = %v, want LastEventAt %v (not the question at %v)", s.Snap.Since, lastAt, askedAt)
	}
	if s.Snap.Since.Equal(askedAt) {
		t.Errorf("Since = %v, which is the question's instant: the archived snapshot must be dated by LastEventAt", askedAt)
	}
	if s.Snap.Reason == "turn ended with a question" {
		t.Errorf("the archived snapshot leaked the real reason %q", s.Snap.Reason)
	}
	if !s.Info.LastEventAt.Equal(lastAt) {
		t.Errorf("Info.LastEventAt = %v, want %v", s.Info.LastEventAt, lastAt)
	}

	// The archive is browsable, so its identity fields must still be filled in.
	if s.Info.Title != "review the migration plan" {
		t.Errorf("Info.Title = %q, want the opening prompt — the archive is a feature, not a stub", s.Info.Title)
	}
	if s.Info.CWD != "/home/user/alpha" || s.Info.GitBranch != "main" {
		t.Errorf("Info cwd/branch = %q/%q, want /home/user/alpha and main", s.Info.CWD, s.Info.GitBranch)
	}
	if s.Info.ProjectSlug != slugAlpha {
		t.Errorf("Info.ProjectSlug = %q, want %q", s.Info.ProjectSlug, slugAlpha)
	}
}

// Rule 3: the archive→live crossing gets a tailer from scratch, so the real
// state — including amber — appears on the very next Refresh.
func TestT55ArchiveToLiveCrossingReplaysTheTranscript(t *testing.T) {
	root := t.TempDir()
	askedAt, lastAt := askedQuestionAt(t, root, slugAlpha, idArchQuestion, 2*time.Hour)

	m := fleet.NewManager(root)

	before := pick(t, mustRefresh(t, m, fleetNow), idArchQuestion)
	assertArchivedSnap(t, before)

	// The pane appears. Nothing about the file changed.
	m.MarkPaneMapped(panesFor(t, root, idArchQuestion))
	crossed := pick(t, mustRefresh(t, m, fleetNow), idArchQuestion)
	if !crossed.Live {
		t.Fatalf("after MarkPaneMapped: Live = false, want true")
	}
	if crossed.Snap.State != state.NeedsYou {
		t.Fatalf("after crossing to live: state = %s, want needs-you — the crossing must replay the transcript",
			crossed.Snap.State)
	}
	if !crossed.Snap.Since.Equal(askedAt) {
		t.Errorf("Since = %v, want the question's instant %v", crossed.Snap.Since, askedAt)
	}
	if crossed.Snap.Reason != "turn ended with a question" {
		t.Errorf("Reason = %q, want %q", crossed.Snap.Reason, "turn ended with a question")
	}
	if crossed.Snap.Activity != "awaiting your reply" {
		t.Errorf("Activity = %q, want %q", crossed.Snap.Activity, "awaiting your reply")
	}
	if !crossed.Info.LastEventAt.Equal(lastAt) {
		t.Errorf("Info.LastEventAt = %v, want %v", crossed.Info.LastEventAt, lastAt)
	}

	// Crossing back drops the tailer and the archive goes quiet again.
	m.MarkPaneMapped(nil)
	back := pick(t, mustRefresh(t, m, fleetNow), idArchQuestion)
	assertArchivedSnap(t, back)
	if !back.Snap.Since.Equal(lastAt) {
		t.Errorf("after live→archive: Since = %v, want LastEventAt %v", back.Snap.Since, lastAt)
	}

	// And the crossing is repeatable, not a one-shot.
	m.MarkPaneMapped(panesFor(t, root, idArchQuestion))
	againLive := pick(t, mustRefresh(t, m, fleetNow), idArchQuestion)
	if againLive.Snap.State != state.NeedsYou {
		t.Errorf("second crossing: state = %s, want needs-you", againLive.Snap.State)
	}
}

// Rule 5: StatusLine counts live sessions only.
func TestT55StatusLineIgnoresTheArchive(t *testing.T) {
	t.Run("mixed fleet counts the live session only", func(t *testing.T) {
		root := t.TempDir()
		askedQuestionAt(t, root, slugAlpha, idArchQuestion, 2*time.Hour) // needs-you shaped, archived
		workingAt(t, root, slugBeta, idLiveWorker, 10*time.Second)       // live

		if got, want := fleet.NewManager(root).StatusLine(fleetNow), "●1"; got != want {
			t.Errorf("StatusLine = %q, want %q — the archived needs-you must not be counted", got, want)
		}
	})

	t.Run("a live idle beside an archived question is all quiet", func(t *testing.T) {
		root := t.TempDir()
		askedQuestionAt(t, root, slugAlpha, idArchQuestion, 2*time.Hour)
		idleAt(t, root, slugBeta, idLiveIdler, 30*time.Second)

		if got, want := fleet.NewManager(root).StatusLine(fleetNow), "○ all quiet"; got != want {
			t.Errorf("StatusLine = %q, want %q", got, want)
		}
	})

	t.Run("an all-archived fleet is all quiet", func(t *testing.T) {
		root := t.TempDir()
		askedQuestionAt(t, root, slugAlpha, idArchQuestion, 2*time.Hour)
		stuckAt(t, root, slugAlpha, idArchived2h, 3*time.Hour)
		workingAt(t, root, slugBeta, idArchived5h, 6*time.Hour)
		idleAt(t, root, slugBeta, idLiveIdler, 8*time.Hour)

		if got, want := fleet.NewManager(root).StatusLine(fleetNow), "○ all quiet"; got != want {
			t.Errorf("StatusLine = %q, want %q — 280 dead transcripts are not four amber sessions", got, want)
		}
	})

	t.Run("a pane makes the same stale session count again", func(t *testing.T) {
		root := t.TempDir()
		askedQuestionAt(t, root, slugAlpha, idArchQuestion, 2*time.Hour)

		m := fleet.NewManager(root)
		if got, want := m.StatusLine(fleetNow), "○ all quiet"; got != want {
			t.Fatalf("archived StatusLine = %q, want %q", got, want)
		}
		m.MarkPaneMapped(panesFor(t, root, idArchQuestion))
		if got, want := m.StatusLine(fleetNow), "▲1"; got != want {
			t.Errorf("pane-mapped StatusLine = %q, want %q — liveness, not age, decides", got, want)
		}
	})
}

// ---------------------------------------------------------------- T57

// T57 — SetLiveWindow moves the recency door; 0 closes it entirely.
func TestT57SetLiveWindow(t *testing.T) {
	// One cast, three ages: 1m (fresh), 30m (middling), 3h (stale).
	build := func(t *testing.T) string {
		t.Helper()
		root := t.TempDir()
		needsYouAt(t, root, slugAlpha, idFreshUnmapped, 1*time.Minute)
		idleAt(t, root, slugBeta, idMid30m, 30*time.Minute)
		idleAt(t, root, slugBeta, idStalePane, 3*time.Hour)
		return root
	}

	t.Run("default is 5m: never calling SetLiveWindow", func(t *testing.T) {
		m := fleet.NewManager(build(t))
		sessions := mustRefresh(t, m, fleetNow)
		if got, want := joined(liveIDs(sessions)), idFreshUnmapped; got != want {
			t.Errorf("live = %v, want just the 1m-old session %v (default window is 5m)", got, want)
		}
		if got, want := joined(archivedIDs(sessions)), joined([]string{idMid30m, idStalePane}); got != want {
			t.Errorf("archive = %v, want %v", got, want)
		}
	})

	t.Run("window 0 is panes only", func(t *testing.T) {
		root := build(t)
		m := fleet.NewManager(root)
		m.SetLiveWindow(0)
		m.MarkPaneMapped(panesFor(t, root, idStalePane))

		sessions := mustRefresh(t, m, fleetNow)
		if got, want := joined(liveIDs(sessions)), idStalePane; got != want {
			t.Errorf("live = %v, want only the pane-mapped %v", got, want)
		}
		fresh := pick(t, sessions, idFreshUnmapped)
		if fresh.Live {
			t.Errorf("%s: Live = true with window 0; the door is shut, panes only", fresh.Info.ID)
		}
		assertArchivedSnap(t, fresh)
		// Order: the live block (however stale) precedes the archive.
		assertOrder(t, sessions, idStalePane, idFreshUnmapped, idMid30m)
	})

	t.Run("window 1h admits the 30m-old session", func(t *testing.T) {
		m := fleet.NewManager(build(t))
		m.SetLiveWindow(time.Hour)

		sessions := mustRefresh(t, m, fleetNow)
		if got, want := joined(liveIDs(sessions)), joined([]string{idFreshUnmapped, idMid30m}); got != want {
			t.Errorf("live = %v, want %v", got, want)
		}
		if got, want := joined(archivedIDs(sessions)), idStalePane; got != want {
			t.Errorf("archive = %v, want the 3h-old %v", got, want)
		}
		mid := pick(t, sessions, idMid30m)
		if mid.Snap.Reason != "turn complete" {
			t.Errorf("%s: Reason = %q, want the real %q — an admitted session is tailed for real",
				mid.Info.ID, mid.Snap.Reason, "turn complete")
		}
	})

	t.Run("the window is a live setting, not a constructor argument", func(t *testing.T) {
		m := fleet.NewManager(build(t))

		m.SetLiveWindow(0)
		if got := liveIDs(mustRefresh(t, m, fleetNow)); len(got) != 0 {
			t.Fatalf("window 0 with no panes: live = %v, want none", got)
		}
		m.SetLiveWindow(time.Hour)
		if got, want := joined(liveIDs(mustRefresh(t, m, fleetNow))), joined([]string{idFreshUnmapped, idMid30m}); got != want {
			t.Fatalf("after widening to 1h: live = %v, want %v", got, want)
		}
		m.SetLiveWindow(5 * time.Minute)
		if got, want := joined(liveIDs(mustRefresh(t, m, fleetNow))), idFreshUnmapped; got != want {
			t.Fatalf("after narrowing back to 5m: live = %v, want %v", got, want)
		}
	})

	// CONTRACT LITERAL (M5-CONTRACT.md): "a session with no pane still counts as
	// live while now−LastEventAt ≤ d" — the boundary is inclusive. A session
	// whose last event is exactly one window old is live.
	t.Run("the boundary is inclusive", func(t *testing.T) {
		root := t.TempDir()
		idleAt(t, root, slugBeta, idBoundary5m, 5*time.Minute)

		s := pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idBoundary5m)
		if !s.Live {
			t.Errorf("a session exactly 5m old is not live; the contract says now−LastEventAt ≤ d")
		}

		// One nanosecond past the door and it is the archive.
		later := pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow.Add(time.Nanosecond)), idBoundary5m)
		if later.Live {
			t.Errorf("a session 5m+1ns old is still live; the door must actually close")
		}
	})
}

// ---------------------------------------------------------------- interactions

// Exclusion beats liveness: an excluded session is gone, not archived.
func TestLiveExclusionBeatsPaneMapping(t *testing.T) {
	root := t.TempDir()
	needsYouAt(t, root, slugAlpha, idFreshUnmapped, 1*time.Minute) // cwd /home/user/alpha
	workingAt(t, root, slugBeta, idLiveWorker, 10*time.Second)     // cwd /home/user/beta

	m := fleet.NewManager(root)
	m.MarkPaneMapped(panesFor(t, root, idFreshUnmapped, idLiveWorker))
	assertOrder(t, mustRefresh(t, m, fleetNow), idFreshUnmapped, idLiveWorker)

	m.ExcludeCWD("/home/user/alpha")
	sessions := mustRefresh(t, m, fleetNow)
	assertOrder(t, sessions, idLiveWorker)
	for _, s := range sessions {
		if s.Info.ID == idFreshUnmapped {
			t.Fatalf("%s survived ExcludeCWD as Live=%v; excluded beats live", s.Info.ID, s.Live)
		}
	}
	if got, want := m.StatusLine(fleetNow), "●1"; got != want {
		t.Errorf("StatusLine = %q, want %q", got, want)
	}
}

// An excluded session cannot re-enter through the archive door either.
func TestLiveExclusionAlsoHidesArchivedSessions(t *testing.T) {
	root := t.TempDir()
	askedQuestionAt(t, root, slugAlpha, idArchQuestion, 2*time.Hour) // cwd /home/user/alpha
	idleAt(t, root, slugBeta, idArchived5h, 5*time.Hour)             // cwd /home/user/beta

	m := fleet.NewManager(root)
	m.ExcludeCWD("/home/user/alpha")
	assertOrder(t, mustRefresh(t, m, fleetNow), idArchived5h)
}

// MarkPaneMapped is fed straight from tmux, which knows nothing about our ids:
// unknown entries must be inert.
func TestMarkPaneMappedUnknownIDsAreHarmless(t *testing.T) {
	root := t54Root(t)

	baseline := mustRefresh(t, fleet.NewManager(root), fleetNow)

	m := fleet.NewManager(root)
	m.MarkPaneMapped(paneMap(
		"00000000-0000-4000-8000-000000000000",
		"not-a-uuid",
		"",
	))
	got := mustRefresh(t, m, fleetNow)

	if len(got) != len(baseline) {
		t.Fatalf("fleet = %v, want the same %v as with no panes at all", sessionIDs(got), sessionIDs(baseline))
	}
	assertOrder(t, got, sessionIDs(baseline)...)
	for i := range got {
		if got[i].Live != baseline[i].Live {
			t.Fatalf("live flags = %v, want %v — unknown pane ids changed the partition",
				liveFlags(got), liveFlags(baseline))
		}
	}
}

// MarkPaneMapped replaces the mapping; it does not accumulate.
func TestMarkPaneMappedReplacesRatherThanUnions(t *testing.T) {
	root := t.TempDir()
	idleAt(t, root, slugBeta, idStalePane, 3*time.Hour)
	idleAt(t, root, slugBeta, idArchived5h, 5*time.Hour)

	m := fleet.NewManager(root)

	m.MarkPaneMapped(panesFor(t, root, idStalePane))
	if got, want := joined(liveIDs(mustRefresh(t, m, fleetNow))), idStalePane; got != want {
		t.Fatalf("live = %v, want %v", got, want)
	}

	// The pane moved to the other session.
	m.MarkPaneMapped(panesFor(t, root, idArchived5h))
	if got, want := joined(liveIDs(mustRefresh(t, m, fleetNow))), idArchived5h; got != want {
		t.Fatalf("live = %v, want only %v — the previous mapping must not linger", got, want)
	}
}

// The zero state — never called, nil, or an empty map — means no panes are
// known, and a previously mapped session falls back to the recency door.
func TestMarkPaneMappedNilOrEmptyClearsTheMapping(t *testing.T) {
	for _, tc := range []struct {
		name  string
		clear map[string]bool
	}{
		{"nil", nil},
		{"empty", map[string]bool{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			idleAt(t, root, slugBeta, idStalePane, 3*time.Hour)            // only a pane can keep it live
			needsYouAt(t, root, slugAlpha, idFreshUnmapped, 1*time.Minute) // the door keeps it live

			m := fleet.NewManager(root)
			m.MarkPaneMapped(panesFor(t, root, idStalePane))
			if got, want := joined(liveIDs(mustRefresh(t, m, fleetNow))), joined([]string{idFreshUnmapped, idStalePane}); got != want {
				t.Fatalf("live = %v, want %v", got, want)
			}

			m.MarkPaneMapped(tc.clear)
			sessions := mustRefresh(t, m, fleetNow)
			if got, want := joined(liveIDs(sessions)), idFreshUnmapped; got != want {
				t.Fatalf("after MarkPaneMapped(%s): live = %v, want only the fresh %v",
					tc.name, got, want)
			}
			assertArchivedSnap(t, pick(t, sessions, idStalePane))
			// Order flips too: the live block is now the fresh one alone.
			assertOrder(t, sessions, idFreshUnmapped, idStalePane)
		})
	}
}

// Panes and the door are a union, not a choice: a session may qualify on both
// counts and must appear exactly once.
func TestLiveIsTheUnionOfPanesAndTheRecencyDoor(t *testing.T) {
	root := t.TempDir()
	needsYouAt(t, root, slugAlpha, idFreshUnmapped, 1*time.Minute)
	idleAt(t, root, slugBeta, idStalePane, 3*time.Hour)

	m := fleet.NewManager(root)
	m.MarkPaneMapped(panesFor(t, root, idFreshUnmapped, idStalePane))

	sessions := mustRefresh(t, m, fleetNow)
	if len(sessions) != 2 {
		t.Fatalf("fleet = %v, want two sessions and no duplicates", sessionIDs(sessions))
	}
	assertLiveFlags(t, sessions, true, true)
}

// BEYOND THE CONTRACT LETTER, and deliberately so: the ui builds its pane map
// once per tick and is entitled to reuse the same map. If the Manager retained
// the caller's map rather than a copy, the next MapSessions would silently
// rewrite the fleet's idea of what is live. The contract does not say "copied";
// it says the ui feeds this after every MapSessions, which only works if the
// map stops being shared the moment it is handed over.
func TestMarkPaneMappedDoesNotRetainTheCallersMap(t *testing.T) {
	root := t.TempDir()
	idleAt(t, root, slugBeta, idStalePane, 3*time.Hour)
	idleAt(t, root, slugBeta, idArchived5h, 5*time.Hour)

	m := fleet.NewManager(root)
	mapping := panesFor(t, root, idStalePane)
	m.MarkPaneMapped(mapping)

	// The caller reuses its map for the next tick, before Refresh runs.
	delete(mapping, idStalePane)
	mapping[idArchived5h] = true

	if got, want := joined(liveIDs(mustRefresh(t, m, fleetNow))), idStalePane; got != want {
		t.Errorf("live = %v, want %v — the Manager followed the caller's later edits", got, want)
	}
}
