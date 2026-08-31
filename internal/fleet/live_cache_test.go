package fleet_test

// T56 — discovery caching, observed from the outside only.
//
// The contract (M5-CONTRACT.md, "Discovery caching") says: a cache on the
// Manager, keyed by path, that reuses the previous SessionInfo while a file's
// (size, mtime) is unchanged. These tests never look at the cache; they look at
// what a cache must not break — repeat Refreshes agreeing with themselves,
// growth being noticed, truncation being survived — plus one probe that only a
// real (size, mtime) key can pass.

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/state"
)

const (
	idCacheLive  = "6f00000c-0000-4000-8000-00000000f60c" // live, grows and is truncated
	idCacheOther = "70000010-0000-4000-8000-000000001070" // live, never touched
	idCacheArch  = "81000011-0000-4000-8000-000000001181" // archived, never touched
)

// infoKey renders a SessionInfo to a canonical string. Comparing these is the
// "byte-identical" check: every field, times normalised to UTC so a location or
// monotonic-clock difference cannot hide a real change or invent a fake one.
func infoKey(i fleet.SessionInfo) string {
	return strings.Join([]string{
		i.ID,
		i.TranscriptPath,
		i.ProjectSlug,
		i.CWD,
		i.GitBranch,
		i.Title,
		i.StartedAt.UTC().Format(time.RFC3339Nano),
		i.LastEventAt.UTC().Format(time.RFC3339Nano),
	}, "|")
}

// sessionKey adds the verdict, so a "nothing changed" assertion also covers the
// partition and the state machine.
func sessionKey(s fleet.Session) string {
	return strings.Join([]string{
		infoKey(s.Info),
		boolStr(s.Live),
		s.Snap.State.String(),
		s.Snap.Since.UTC().Format(time.RFC3339Nano),
		s.Snap.Reason,
		s.Snap.Activity,
	}, "|")
}

func boolStr(b bool) string {
	if b {
		return "live"
	}
	return "archived"
}

func renderFleet(sessions []fleet.Session) []string {
	out := make([]string, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, sessionKey(s))
	}
	return out
}

func infoKeys(sessions []fleet.Session) map[string]string {
	out := make(map[string]string, len(sessions))
	for _, s := range sessions {
		out[s.Info.ID] = infoKey(s.Info)
	}
	return out
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	return fi.Size()
}

// cacheRoot: one live session that will grow, one live session that must never
// move, one archived session that must never move.
func cacheRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	workingAt(t, root, slugBeta, idCacheLive, 10*time.Second) // 2 lines, pending Bash
	idleAt(t, root, slugBeta, idCacheOther, 1*time.Minute)
	idleAt(t, root, slugBeta, idCacheArch, 3*time.Hour)
	return root
}

// T56a — two Refreshes over an untouched tree must agree with themselves,
// field for field. A cache that returns a stale-but-different record, or that
// re-derives something non-deterministically, dies here.
func TestT56RefreshTwiceUnchangedIsIdentical(t *testing.T) {
	m := fleet.NewManager(cacheRoot(t))

	first := renderFleet(mustRefresh(t, m, fleetNow))
	second := renderFleet(mustRefresh(t, m, fleetNow))
	third := renderFleet(mustRefresh(t, m, fleetNow))

	if len(first) != 3 {
		t.Fatalf("fleet has %d sessions, want 3:\n%s", len(first), strings.Join(first, "\n"))
	}
	if strings.Join(first, "\n") != strings.Join(second, "\n") {
		t.Fatalf("second Refresh differs:\nfirst:\n%s\nsecond:\n%s",
			strings.Join(first, "\n"), strings.Join(second, "\n"))
	}
	if strings.Join(second, "\n") != strings.Join(third, "\n") {
		t.Fatalf("third Refresh differs:\nsecond:\n%s\nthird:\n%s",
			strings.Join(second, "\n"), strings.Join(third, "\n"))
	}

	// A fresh Manager over the same tree must see the same thing: the cache is
	// an optimisation, not a source of truth.
	cold := renderFleet(mustRefresh(t, fleet.NewManager(cacheRoot(t)), fleetNow))
	if len(cold) != len(first) {
		t.Fatalf("cold Manager saw %d sessions, want %d", len(cold), len(first))
	}
	// Paths differ (a different TempDir), so compare everything but the path.
	stripPath := func(keys []string) []string {
		out := make([]string, 0, len(keys))
		for _, k := range keys {
			parts := strings.Split(k, "|")
			parts[1] = ""
			out = append(out, strings.Join(parts, "|"))
		}
		return out
	}
	if strings.Join(stripPath(cold), "\n") != strings.Join(stripPath(first), "\n") {
		t.Errorf("a cold Manager disagrees with a warm one:\ncold:\n%s\nwarm:\n%s",
			strings.Join(stripPath(cold), "\n"), strings.Join(stripPath(first), "\n"))
	}
}

// T56b — one file grows; exactly one session moves.
func TestT56AppendedEventAdvancesOnlyThatSession(t *testing.T) {
	root := cacheRoot(t)
	m := fleet.NewManager(root)

	before := mustRefresh(t, m, fleetNow)
	beforeKeys := infoKeys(before)
	if got := pick(t, before, idCacheLive).Snap.State; got != state.Working {
		t.Fatalf("%s starts as %s, want working", idCacheLive, got)
	}

	// The session answers its own tool call and closes on a question.
	toolID := "toolu_" + idCacheLive[:4]
	appendLines(t, root, slugBeta, idCacheLive,
		continueTranscript(t, idCacheLive, "/home/user/beta", "feat/auth", 2).
			result(ago(2*time.Second), toolID, "2 failed, 8 passed").
			text(fleetNow, "Two tests fail. Want me to fix them?"),
		fleetNow)

	now := fleetNow.Add(time.Second)
	after := mustRefresh(t, m, now)
	afterKeys := infoKeys(after)

	grown := pick(t, after, idCacheLive)
	if grown.Snap.State != state.NeedsYou {
		t.Errorf("%s: state = %s, want needs-you — the appended lines were not read",
			idCacheLive, grown.Snap.State)
	}
	if !grown.Info.LastEventAt.Equal(fleetNow) {
		t.Errorf("%s: LastEventAt = %v, want the appended event's %v",
			idCacheLive, grown.Info.LastEventAt, fleetNow)
	}
	if beforeKeys[idCacheLive] == afterKeys[idCacheLive] {
		t.Errorf("%s: SessionInfo did not change even though the file grew:\n%s",
			idCacheLive, afterKeys[idCacheLive])
	}
	if !grown.Live {
		t.Errorf("%s: Live = false, want true", idCacheLive)
	}

	// Nobody else moved a byte.
	for _, id := range []string{idCacheOther, idCacheArch} {
		if beforeKeys[id] != afterKeys[id] {
			t.Errorf("%s changed while another file grew:\nbefore: %s\nafter:  %s",
				id, beforeKeys[id], afterKeys[id])
		}
	}
	other := pick(t, after, idCacheOther)
	if other.Snap.State != state.Idle || !other.Live {
		t.Errorf("%s: {%s live=%v}, want a live idle session", idCacheOther, other.Snap.State, other.Live)
	}
	assertArchivedSnap(t, pick(t, after, idCacheArch))
}

// T56c — the file shrinks and is rewritten. The cache key changes on size, the
// tailer must re-read from zero, and the verdict must be the new file's.
func TestT56TruncatedFileIsReReadCorrectly(t *testing.T) {
	root := cacheRoot(t)
	m := fleet.NewManager(root)
	path := transcriptPath(root, slugBeta, idCacheLive)

	mustRefresh(t, m, fleetNow)

	toolID := "toolu_" + idCacheLive[:4]
	appendLines(t, root, slugBeta, idCacheLive,
		continueTranscript(t, idCacheLive, "/home/user/beta", "feat/auth", 2).
			result(ago(2*time.Second), toolID, "2 failed, 8 passed").
			text(fleetNow, "Two tests fail. Want me to fix them?"),
		fleetNow)

	grownSize := fileSize(t, path)
	before := mustRefresh(t, m, fleetNow.Add(time.Second))
	beforeKeys := infoKeys(before)

	// Rewritten shorter: a fresh two-line conversation ending on a question. It
	// carries no tool call, so the expected verdict is the same whether the
	// implementation rebuilds the machine on truncation or keeps folding.
	askedAt := fleetNow.Add(60 * time.Second)
	newTranscript(t, idCacheLive, "/home/user/beta", "feat/auth").
		prompt(fleetNow.Add(30*time.Second), "start over").
		text(askedAt, "Reset. Re-run the suite?").
		write(root, slugBeta)

	shrunkSize := fileSize(t, path)
	if shrunkSize >= grownSize {
		t.Fatalf("fixture broken: file is %d bytes after the rewrite, was %d — nothing was truncated",
			shrunkSize, grownSize)
	}

	after := mustRefresh(t, m, fleetNow.Add(90*time.Second))
	afterKeys := infoKeys(after)

	s := pick(t, after, idCacheLive)
	if s.Snap.State != state.NeedsYou {
		t.Errorf("%s: state = %s, want needs-you from the rewritten file", idCacheLive, s.Snap.State)
	}
	if !s.Snap.Since.Equal(askedAt) {
		t.Errorf("%s: Since = %v, want the rewritten question's %v", idCacheLive, s.Snap.Since, askedAt)
	}
	if !s.Info.LastEventAt.Equal(askedAt) {
		t.Errorf("%s: LastEventAt = %v, want %v", idCacheLive, s.Info.LastEventAt, askedAt)
	}
	if !s.Live {
		t.Errorf("%s: Live = false; its last event is 30s old", idCacheLive)
	}

	for _, id := range []string{idCacheOther, idCacheArch} {
		if beforeKeys[id] != afterKeys[id] {
			t.Errorf("%s changed while another file was truncated:\nbefore: %s\nafter:  %s",
				id, beforeKeys[id], afterKeys[id])
		}
	}
	// idCacheOther's last event is 1m before fleetNow, so at +90s it is 2m30s
	// old: still inside the default 5m door.
	if !pick(t, after, idCacheOther).Live {
		t.Errorf("%s: Live = false, want true (2m30s old, door is 5m)", idCacheOther)
	}
}

// T56d — the (size, mtime) key, pinned.
//
// We rewrite an ARCHIVED transcript in place: same byte count, same mtime,
// different content — including a *later* last-event timestamp. An archived
// session is not tailed (rule 2/3), so discovery is the only thing that could
// read this file. If the Manager honours the (size, mtime) key it never opens
// it and the old SessionInfo survives; if it re-reads on every Refresh, the
// smuggled timestamp shows up.
//
// This is deliberately a change no honest producer would make — it exists to
// document the key, not to describe the world.
func TestT56UnchangedSizeAndMtimeMeansNoReopen(t *testing.T) {
	root := cacheRoot(t)
	m := fleet.NewManager(root)
	path := transcriptPath(root, slugBeta, idCacheArch)

	before := mustRefresh(t, m, fleetNow)
	beforeKeys := infoKeys(before)
	archived := pick(t, before, idCacheArch)
	assertArchivedSnap(t, archived)
	if !archived.Info.LastEventAt.Equal(ago(3 * time.Hour)) {
		t.Fatalf("%s: LastEventAt = %v, want %v", idCacheArch, archived.Info.LastEventAt, ago(3*time.Hour))
	}
	if archived.Info.Title != "tidy the imports" {
		t.Fatalf("%s: Title = %q, want %q", idCacheArch, archived.Info.Title, "tidy the imports")
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	mtime, size := fi.ModTime(), fi.Size()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	// Both replacements are exactly length-preserving.
	oldTS := ago(3 * time.Hour).UTC().Format(tsLayout)
	newTS := ago(1 * time.Hour).UTC().Format(tsLayout)
	smuggled := strings.Replace(string(body), oldTS, newTS, 1)
	smuggled = strings.Replace(smuggled, "tidy the imports", "TIDY THE IMPORTS", 1)
	if smuggled == string(body) {
		t.Fatalf("fixture broken: nothing was rewritten in %s", path)
	}
	if err := os.WriteFile(path, []byte(smuggled), 0o644); err != nil {
		t.Fatalf("rewrite %s: %v", path, err)
	}
	if got := fileSize(t, path); got != size {
		t.Fatalf("fixture broken: the rewrite changed the size (%d -> %d); the probe needs it identical",
			size, got)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
	fi2, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	if !fi2.ModTime().Equal(mtime) {
		t.Fatalf("fixture broken: mtime did not round-trip (%v -> %v)", mtime, fi2.ModTime())
	}

	after := mustRefresh(t, m, fleetNow.Add(time.Second))
	afterKeys := infoKeys(after)

	if beforeKeys[idCacheArch] != afterKeys[idCacheArch] {
		t.Errorf("the archived SessionInfo was re-derived although (size, mtime) never changed:\n"+
			"before: %s\nafter:  %s\n(the cache must be keyed on size and mtime — see M5-CONTRACT.md)",
			beforeKeys[idCacheArch], afterKeys[idCacheArch])
	}
	s := pick(t, after, idCacheArch)
	if !s.Info.LastEventAt.Equal(ago(3 * time.Hour)) {
		t.Errorf("%s: LastEventAt = %v, want the cached %v — the file was reopened",
			idCacheArch, s.Info.LastEventAt, ago(3*time.Hour))
	}
	if s.Info.Title != "tidy the imports" {
		t.Errorf("%s: Title = %q, want the cached %q — the file was reopened",
			idCacheArch, s.Info.Title, "tidy the imports")
	}
	assertArchivedSnap(t, s)

	// And the key does let go: bump the mtime (and the size) and the new content
	// is picked up.
	t.Run("a changed mtime invalidates the entry", func(t *testing.T) {
		grown := smuggled + string(mustLine(t, idCacheArch, ago(90*time.Minute)))
		if err := os.WriteFile(path, []byte(grown), 0o644); err != nil {
			t.Fatalf("grow %s: %v", path, err)
		}
		touched := ago(90 * time.Minute)
		if err := os.Chtimes(path, touched, touched); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}

		s := pick(t, mustRefresh(t, m, fleetNow.Add(2*time.Second)), idCacheArch)
		if !s.Info.LastEventAt.Equal(ago(90 * time.Minute)) {
			t.Errorf("%s: LastEventAt = %v, want %v — a grown file must invalidate the cache entry",
				idCacheArch, s.Info.LastEventAt, ago(90*time.Minute))
		}
		assertArchivedSnap(t, s) // 90m old and paneless: still the archive
	})
}

// The mirror image of the probe above: "the cache lives on the Manager
// (Discover the free function stays uncached — its one-shot callers don't
// loop)". The same in-place rewrite that a warm Manager must NOT see is one
// Discover must see, every time.
func TestT56DiscoverTheFreeFunctionStaysUncached(t *testing.T) {
	root := cacheRoot(t)
	path := transcriptPath(root, slugBeta, idCacheArch)

	first, err := fleet.Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	var before fleet.SessionInfo
	for _, s := range first {
		if s.ID == idCacheArch {
			before = s
		}
	}
	if before.Title != "tidy the imports" {
		t.Fatalf("%s: Title = %q, want %q", idCacheArch, before.Title, "tidy the imports")
	}

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
	mtime := fi.ModTime()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	rewritten := strings.Replace(string(body), "tidy the imports", "TIDY THE IMPORTS", 1)
	if err := os.WriteFile(path, []byte(rewritten), 0o644); err != nil {
		t.Fatalf("rewrite %s: %v", path, err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}

	second, err := fleet.Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, s := range second {
		if s.ID != idCacheArch {
			continue
		}
		if s.Title != "TIDY THE IMPORTS" {
			t.Errorf("Discover Title = %q, want the file's current %q — the free function must not cache",
				s.Title, "TIDY THE IMPORTS")
		}
		return
	}
	t.Fatalf("%s vanished from Discover", idCacheArch)
}

// mustLine renders one extra bookkeeping line (with a timestamp) for a session.
func mustLine(t *testing.T, id string, ts time.Time) []byte {
	t.Helper()
	b := continueTranscript(t, id, "/home/user/beta", "main", 9)
	b.bookkeeping(ts)
	return []byte(strings.Join(b.lines, ""))
}
