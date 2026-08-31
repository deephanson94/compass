package fleet_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
)

// ---------------------------------------------------------------- T49

// refreshIDs runs a Refresh and returns the surviving session ids.
func refreshIDs(t *testing.T, m *fleet.Manager) []string {
	t.Helper()
	sessions, err := m.Refresh(fleetNow)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	return sessionIDs(sessions)
}

func assertIDs(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("fleet = %v, want %v", got, want)
	}
}

// twoCWDs writes one session in /home/user/alpha (needs-you) and one in
// /home/user/beta (working) — the two states also make the StatusLine
// assertions unambiguous.
func twoCWDs(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	needsYouAt(t, root, "-home-user-alpha", idNeedsYou, 8*time.Minute)
	workingAt(t, root, "-home-user-beta", idWorking, 10*time.Second)
	return root
}

// T49 — ExcludeCWD hides the sessions living at that working directory and
// leaves every other session exactly where it was. This is what keeps compass
// from watching its own narrator narrate.
func TestT49ExcludeCWDHidesThatCWDOnly(t *testing.T) {
	root := twoCWDs(t)

	// Baseline: both are there, needs-you first.
	assertIDs(t, refreshIDs(t, liveManager(root)), idNeedsYou, idWorking)

	m := liveManager(root)
	m.ExcludeCWD("/home/user/alpha")
	assertIDs(t, refreshIDs(t, m), idWorking)

	// And the other way round, so the test cannot pass by hiding everything.
	other := liveManager(root)
	other.ExcludeCWD("/home/user/beta")
	assertIDs(t, refreshIDs(t, other), idNeedsYou)
}

// The exclusion is a live setting, not a constructor argument: a session
// already discovered and tailed must disappear on the next Refresh.
func TestT49ExcludeCWDAppliesToAlreadyTrackedSessions(t *testing.T) {
	root := twoCWDs(t)
	m := liveManager(root)

	assertIDs(t, refreshIDs(t, m), idNeedsYou, idWorking)
	m.ExcludeCWD("/home/user/alpha")
	assertIDs(t, refreshIDs(t, m), idWorking)

	// Stable across repeated refreshes — the exclusion is not consumed.
	assertIDs(t, refreshIDs(t, m), idWorking)
	assertIDs(t, refreshIDs(t, m), idWorking)
}

// Every session sharing the excluded directory goes, not just the first one.
// The narrator's dir accumulates one session per narration, so this is the
// real shape of the case.
func TestT49ExcludeCWDHidesEverySessionAtThatPath(t *testing.T) {
	root := t.TempDir()
	needsYouAt(t, root, "-home-user-alpha", idNeedsYou, 8*time.Minute) // /home/user/alpha
	stuckAt(t, root, "-home-user-alpha", idStuck, 5*time.Minute)       // /home/user/alpha
	workingAt(t, root, "-home-user-beta", idWorking, 10*time.Second)   // /home/user/beta

	m := liveManager(root)
	assertIDs(t, refreshIDs(t, m), idNeedsYou, idStuck, idWorking)

	m.ExcludeCWD("/home/user/alpha")
	assertIDs(t, refreshIDs(t, m), idWorking)
}

// Excluding a path nothing lives at changes nothing. Compass calls ExcludeCWD
// with the narrator dir on every start, long before that dir has ever been
// used.
func TestT49ExcludeCWDWithNoMatchIsANoOp(t *testing.T) {
	root := twoCWDs(t)
	m := liveManager(root)
	m.ExcludeCWD(filepath.Join(t.TempDir(), "compass", "narrator"))
	assertIDs(t, refreshIDs(t, m), idNeedsYou, idWorking)
}

// "CWD equals path" is equality, not prefix matching: a parent directory, a
// child directory and a truncated path must all leave the fleet alone.
// Otherwise excluding the narrator dir could swallow the user's whole tree.
func TestT49ExcludeCWDIsExactNotAPrefix(t *testing.T) {
	for _, path := range []string{
		"/home/user",
		"/home/user/",
		"/home/user/alpha/nested",
		"/home/user/alph",
		"/home/user/alphabet",
		"home/user/alpha",
		"",
	} {
		t.Run(path, func(t *testing.T) {
			root := twoCWDs(t)
			m := liveManager(root)
			m.ExcludeCWD(path)
			assertIDs(t, refreshIDs(t, m), idNeedsYou, idWorking)
		})
	}
}

// Excluding the same path twice is the same as excluding it once.
func TestT49ExcludeCWDIsIdempotent(t *testing.T) {
	root := twoCWDs(t)
	m := liveManager(root)
	m.ExcludeCWD("/home/user/alpha")
	m.ExcludeCWD("/home/user/alpha")
	assertIDs(t, refreshIDs(t, m), idWorking)
}

// Excluding every cwd leaves an empty fleet, not an error.
func TestT49ExcludingEverythingIsAnEmptyFleetNotAnError(t *testing.T) {
	root := twoCWDs(t)
	m := liveManager(root)
	m.ExcludeCWD("/home/user/alpha")
	m.ExcludeCWD("/home/user/beta")

	sessions, err := m.Refresh(fleetNow)
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("Refresh returned %v, want an empty fleet", sessionIDs(sessions))
	}
	if got, want := m.StatusLine(fleetNow), "○ all quiet"; got != want {
		t.Errorf("StatusLine = %q, want %q", got, want)
	}
}

// T49 — the status line is a Refresh underneath, so it has to agree with it:
// an excluded session is not counted anywhere.
func TestT49StatusLineReflectsTheExclusion(t *testing.T) {
	// Pins the SHIPPED status line, so the fixtures sit inside the default
	// live window and the Manager runs stock (see manager_test.go's note).
	root := t.TempDir()
	needsYouAt(t, root, "-home-user-alpha", idNeedsYou, 2*time.Minute)
	workingAt(t, root, "-home-user-beta", idWorking, 10*time.Second)

	if got, want := fleet.NewManager(root).StatusLine(fleetNow), "▲1 ●1"; got != want {
		t.Fatalf("baseline StatusLine = %q, want %q", got, want)
	}

	hidAlpha := fleet.NewManager(root)
	hidAlpha.ExcludeCWD("/home/user/alpha")
	if got, want := hidAlpha.StatusLine(fleetNow), "●1"; got != want {
		t.Errorf("StatusLine with /home/user/alpha excluded = %q, want %q", got, want)
	}

	hidBeta := fleet.NewManager(root)
	hidBeta.ExcludeCWD("/home/user/beta")
	if got, want := hidBeta.StatusLine(fleetNow), "▲1"; got != want {
		t.Errorf("StatusLine with /home/user/beta excluded = %q, want %q", got, want)
	}
}

// The wiring case, end to end: the narrator's own dir is a real directory under
// the user cache, and the session it leaves behind must never reach the panel.
func TestT49NarratorDirSessionIsHidden(t *testing.T) {
	root := t.TempDir()
	narratorDir := filepath.Join(t.TempDir(), "compass", "narrator")
	const narrationID = "ff00000a-0000-4000-8000-00000000000a"

	workingAt(t, root, "-home-user-beta", idWorking, 10*time.Second)
	newTranscript(t, narrationID, narratorDir, "").
		prompt(ago(20*time.Second), "name these legs: [{\"key\":\"k1\"}]").
		text(ago(5*time.Second), `[{"key":"k1","label":"maps the auth module"}]`).
		write(root, "-narrator")

	m := liveManager(root)
	// Without the exclusion compass watches itself.
	if got := refreshIDs(t, m); len(got) != 2 {
		t.Fatalf("baseline fleet = %v, want both the real session and the narration", got)
	}

	m.ExcludeCWD(narratorDir)
	assertIDs(t, refreshIDs(t, m), idWorking)
}
