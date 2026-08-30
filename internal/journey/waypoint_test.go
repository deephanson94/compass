package journey_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/transcript"
)

// ---------------------------------------------------------------- builders
//
// These extend the M1 builders in classify_test.go; `result` there stays as it
// is for the tests that only care about IsError.

// textResult builds a user event carrying one tool_result with a body.
func textResult(offset time.Duration, id, text string, isErr bool) transcript.Event {
	return transcript.Event{
		Type: transcript.EventUser, UUID: "u", SessionID: "s",
		Timestamp:   at(offset),
		ToolResults: []transcript.ToolResult{{ToolUseID: id, IsError: isErr, Text: text}},
	}
}

// okResult is a clean tool_result carrying output; errResult is a failing one.
func okResult(offset time.Duration, id, text string) transcript.Event {
	return textResult(offset, id, text, false)
}

func errResult(offset time.Duration, id, text string) transcript.Event {
	return textResult(offset, id, text, true)
}

// ---------------------------------------------------------------- assertions

// wp is one expected waypoint.
type wp struct {
	kind journey.WaypointKind
	text string
}

func kindName(k journey.WaypointKind) string {
	switch k {
	case journey.WaypointTestRun:
		return "TestRun"
	case journey.WaypointTestFail:
		return "TestFail"
	case journey.WaypointBug:
		return "Bug"
	case journey.WaypointCommit:
		return "Commit"
	}
	return fmt.Sprintf("WaypointKind(%d)", int(k))
}

func dumpWaypoints(leg journey.Leg) string {
	if len(leg.Waypoints) == 0 {
		return "  (no waypoints)\n"
	}
	var b strings.Builder
	for i, w := range leg.Waypoints {
		fmt.Fprintf(&b, "  [%d] %-8s %q at=+%v\n", i, kindName(w.Kind), w.Text, w.At.Sub(base))
	}
	return b.String()
}

// legOf returns leg i, failing loudly when the segmenter produced a different
// shape than the test assumed.
func legOf(t *testing.T, tr journey.Trail, i int) journey.Leg {
	t.Helper()
	if i >= len(tr.Legs) {
		t.Fatalf("wanted Legs[%d] but there are only %d legs:\n%s", i, len(tr.Legs), dumpLegs(tr))
	}
	return tr.Legs[i]
}

// checkWaypointInvariants holds for every waypoint, whatever produced it.
func checkWaypointInvariants(t *testing.T, leg journey.Leg) {
	t.Helper()
	if len(leg.Waypoints) > 8 {
		t.Errorf("leg carries %d waypoints, the cap is 8:\n%s", len(leg.Waypoints), dumpWaypoints(leg))
	}
	for i, w := range leg.Waypoints {
		if n := utf8.RuneCountInString(w.Text); n > 60 {
			t.Errorf("Waypoints[%d].Text is %d runes (%q), want at most 60", i, n, w.Text)
		}
		if strings.ContainsAny(w.Text, "\n\r") {
			t.Errorf("Waypoints[%d].Text = %q, want a single line", i, w.Text)
		}
		if w.Text == "" {
			t.Errorf("Waypoints[%d] (%s) has empty Text", i, kindName(w.Kind))
		}
		if w.At.IsZero() {
			t.Errorf("Waypoints[%d] (%s) has the zero At", i, kindName(w.Kind))
		}
		if i > 0 && w.At.Before(leg.Waypoints[i-1].At) {
			t.Errorf("Waypoints[%d] is older than Waypoints[%d]: waypoints are oldest first\n%s",
				i, i-1, dumpWaypoints(leg))
		}
	}
}

// assertWaypoints checks the leg's waypoints in exact order.
func assertWaypoints(t *testing.T, leg journey.Leg, want ...wp) {
	t.Helper()
	checkWaypointInvariants(t, leg)
	if len(leg.Waypoints) != len(want) {
		t.Fatalf("got %d waypoints, want %d:\n%s", len(leg.Waypoints), len(want), dumpWaypoints(leg))
	}
	for i, w := range want {
		got := leg.Waypoints[i]
		if got.Kind != w.kind || got.Text != w.text {
			t.Errorf("Waypoints[%d] = %s %q, want %s %q\n%s",
				i, kindName(got.Kind), got.Text, kindName(w.kind), w.text, dumpWaypoints(leg))
		}
	}
}

// assertWaypointsByKind checks the same thing but only within each kind. The
// contract fixes the order of same-kind waypoints (first-seen, deduped) and
// their order relative to older results, but says nothing about whether the
// TestRun of one result sorts before or after that result's TestFails — so
// tests about a single result use this instead of assertWaypoints.
func assertWaypointsByKind(t *testing.T, leg journey.Leg, want ...wp) {
	t.Helper()
	checkWaypointInvariants(t, leg)

	gotBy := map[journey.WaypointKind][]string{}
	for _, w := range leg.Waypoints {
		gotBy[w.Kind] = append(gotBy[w.Kind], w.Text)
	}
	wantBy := map[journey.WaypointKind][]string{}
	for _, w := range want {
		wantBy[w.kind] = append(wantBy[w.kind], w.text)
	}
	if len(leg.Waypoints) != len(want) {
		t.Fatalf("got %d waypoints, want %d:\n%s", len(leg.Waypoints), len(want), dumpWaypoints(leg))
	}
	for _, k := range []journey.WaypointKind{
		journey.WaypointTestRun, journey.WaypointTestFail,
		journey.WaypointBug, journey.WaypointCommit,
	} {
		g, w := gotBy[k], wantBy[k]
		if len(g) != len(w) {
			t.Errorf("got %d %s waypoints, want %d:\n%s", len(g), kindName(k), len(w), dumpWaypoints(leg))
			continue
		}
		for i := range w {
			if g[i] != w[i] {
				t.Errorf("%s[%d] = %q, want %q\n%s", kindName(k), i, g[i], w[i], dumpWaypoints(leg))
			}
		}
	}
}

// assertClipped checks a waypoint's 60-rune budget against the line it came
// from: a prefix of that line (a trailing "…" is allowed), and long enough that
// a byte-wise cut of a multibyte line would be caught.
func assertClipped(t *testing.T, got, full string) {
	t.Helper()
	if n := utf8.RuneCountInString(got); n > 60 {
		t.Errorf("Waypoint.Text is %d runes (%q), want at most 60", n, got)
	}
	full = strings.TrimSpace(full)
	if utf8.RuneCountInString(full) <= 60 {
		if got != full {
			t.Errorf("Waypoint.Text = %q, want the line untouched (%q)", got, full)
		}
		return
	}
	if n := utf8.RuneCountInString(got); n < 55 {
		t.Errorf("Waypoint.Text is only %d runes (%q); the budget is 60 RUNES, not bytes", n, got)
	}
	head := strings.TrimRight(strings.TrimSuffix(got, "…"), " ")
	if !strings.HasPrefix(full, head) {
		t.Errorf("Waypoint.Text = %q, want a (clipped) prefix of %q", got, full)
	}
}

// ---------------------------------------------------------------- fixtures

const pytestFailingTail = `============================= test session starts ==============================
platform linux -- Python 3.11.8, pytest-8.1.1, pluggy-1.4.0
rootdir: /w
collected 20 items

tests/auth/test_refresh.py ..F.........F......                           [100%]

=================================== FAILURES ===================================
_________________________ test_refresh_expired_token ___________________________

    def test_refresh_expired_token():
>       assert refresh(expired) is not None
E       AssertionError: assert None is not None

tests/auth/test_refresh.py:41: AssertionError
=========================== short test summary info ============================
FAILED tests/auth/test_refresh.py::test_refresh_expired_token - AssertionError
FAILED tests/auth/test_revoke.py::test_revoke_idempotent - AssertionError: 1 != 2
========================= 2 failed, 18 passed in 1.24s =========================
`

const pytestCleanTail = `============================= test session starts ==============================
platform linux -- Python 3.11.8, pytest-8.1.1, pluggy-1.4.0
collected 18 items

tests/auth/test_refresh.py ..................                            [100%]

============================== 18 passed in 1.10s ==============================
`

// pytestOutput builds a pytest tail with the given failures ("<path>::<name>").
func pytestOutput(passed int, failed ...string) string {
	var b strings.Builder
	b.WriteString("=========================== short test summary info ============================\n")
	for _, f := range failed {
		fmt.Fprintf(&b, "FAILED %s - AssertionError: assert False\n", f)
	}
	if len(failed) == 0 {
		fmt.Fprintf(&b, "============================== %d passed in 1.10s ==============================\n", passed)
	} else {
		fmt.Fprintf(&b, "===================== %d failed, %d passed in 1.24s ======================\n",
			len(failed), passed)
	}
	return b.String()
}

const goTestFailingOutput = `=== RUN   TestTailerResetsOnTruncation
    tailer_test.go:112: got 3 events, want 0
--- FAIL: TestTailerResetsOnTruncation (0.00s)
=== RUN   TestTailerHandlesPartialLine
    tailer_test.go:150: short read
--- FAIL: TestTailerHandlesPartialLine (0.01s)
FAIL
FAIL	github.com/deephanson94/compass/internal/transcript	0.014s
FAIL
`

const goTestOKOutput = `ok  	github.com/deephanson94/compass/internal/journey	0.012s
ok  	github.com/deephanson94/compass/internal/transcript	0.007s
`

// Note the trap: "Test Suites:" is also a counts line, and it disagrees with
// the "Tests:" line the contract names.
const jestFailingOutput = ` FAIL  src/auth/refresh.test.ts
  Auth
    ✕ refreshes expired token (12 ms)
    ✓ issues a new token (3 ms)

  ● Auth › refreshes expired token

    expect(received).toBe(expected)

Test Suites: 1 failed, 2 passed, 3 total
Tests:       1 failed, 12 passed, 13 total
Snapshots:   0 total
Time:        2.31 s
`

// The "failures:" block repeats both names — extraction must not double-count.
const cargoFailingOutput = `running 12 tests
test store::get ... ok
test auth::refresh ... FAILED
test auth::revoke ... FAILED

failures:

---- auth::refresh stdout ----
thread 'auth::refresh' panicked at src/auth.rs:88:9:
assertion failed: token.is_some()

failures:
    auth::refresh
    auth::revoke

test result: FAILED. 10 passed; 2 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.03s
`

// ---------------------------------------------------------------- T36 pytest

// T36 — a real pytest tail on a Test-voted result: one summary waypoint and one
// waypoint per failing test, named bare.
func TestT36PytestSummaryAndFailures(t *testing.T) {
	tr := segment(
		bash(1*time.Minute, "tu1", "pytest tests/auth -x"), // Test leg opens; tu1 is the runner
		errResult(2*time.Minute, "tu1", pytestFailingTail), // the suite failed
	)
	assertLegs(t, tr, legWant{journey.Test, 1 * time.Minute, 1 * time.Minute, 1, true})

	leg := legOf(t, tr, 0)
	assertWaypointsByKind(t, leg,
		wp{journey.WaypointTestRun, "18 passed · 2 failed"},
		wp{journey.WaypointTestFail, "test_refresh_expired_token"},
		wp{journey.WaypointTestFail, "test_revoke_idempotent"},
	)

	// Rule 4 only makes Bugs on Build/Fix legs: a failing suite on a Test leg is
	// a test result, not a bug.
	for _, w := range leg.Waypoints {
		if w.Kind == journey.WaypointBug {
			t.Errorf("a failing test run produced a Bug waypoint %q on a Test leg", w.Text)
		}
	}
	// The waypoints are stamped with the result, not the vote.
	for i, w := range leg.Waypoints {
		if !w.At.Equal(at(2 * time.Minute)) {
			t.Errorf("Waypoints[%d].At = %v, want the result's time %v", i, w.At, at(2*time.Minute))
		}
	}
}

// T36 — nothing failed: the zero part is omitted, not rendered as "0 failed".
func TestT36PytestZeroFailedOmitsThePart(t *testing.T) {
	tr := segment(
		bash(1*time.Minute, "tu1", "pytest tests/auth"),
		okResult(2*time.Minute, "tu1", pytestCleanTail),
	)
	assertWaypoints(t, legOf(t, tr, 0), wp{journey.WaypointTestRun, "18 passed"})
}

// T36 — at most three failing names survive, in first-seen order.
func TestT36PytestFailuresCapAtThree(t *testing.T) {
	tr := segment(
		bash(1*time.Minute, "tu1", "pytest tests -x"),
		errResult(2*time.Minute, "tu1", pytestOutput(13,
			"tests/auth/test_refresh.py::test_refresh_expired_token",
			"tests/auth/test_revoke.py::test_revoke_idempotent",
			"tests/store/test_cache.py::test_cache_evicts_lru",
			"tests/store/test_cache.py::test_cache_survives_restart",
			"tests/api/test_routes.py::test_routes[GET-/health]",
		)),
	)
	assertWaypointsByKind(t, legOf(t, tr, 0),
		wp{journey.WaypointTestRun, "13 passed · 5 failed"},
		wp{journey.WaypointTestFail, "test_refresh_expired_token"},
		wp{journey.WaypointTestFail, "test_revoke_idempotent"},
		wp{journey.WaypointTestFail, "test_cache_evicts_lru"},
	)
}

// T36 — the same failure twice is one waypoint.
func TestT36PytestFailuresDedupe(t *testing.T) {
	t.Run("identical FAILED lines", func(t *testing.T) {
		tr := segment(
			bash(1*time.Minute, "tu1", "pytest tests -x"),
			errResult(2*time.Minute, "tu1", pytestOutput(18,
				"tests/auth/test_refresh.py::test_refresh_expired_token",
				"tests/auth/test_refresh.py::test_refresh_expired_token",
			)),
		)
		assertWaypointsByKind(t, legOf(t, tr, 0),
			// The counts line is what the runner said, however many lines it printed.
			wp{journey.WaypointTestRun, "18 passed · 2 failed"},
			wp{journey.WaypointTestFail, "test_refresh_expired_token"},
		)
	})

	t.Run("same test name in two files", func(t *testing.T) {
		tr := segment(
			bash(1*time.Minute, "tu1", "pytest tests -x"),
			errResult(2*time.Minute, "tu1", pytestOutput(18,
				"tests/auth/test_refresh.py::test_roundtrip",
				"tests/store/test_cache.py::test_roundtrip",
			)),
		)
		// Dedupe is on the waypoint text, and the text is the bare name.
		assertWaypointsByKind(t, legOf(t, tr, 0),
			wp{journey.WaypointTestRun, "18 passed · 2 failed"},
			wp{journey.WaypointTestFail, "test_roundtrip"},
		)
	})
}

// T36 — rule 2: only the results of Test-voted tool_uses are test-parsed.
func TestT36TestOutputOnANonTestVoteIsNotParsed(t *testing.T) {
	t.Run("plain Bash vote", func(t *testing.T) {
		tr := segment(
			bash(1*time.Minute, "tu1", "npm run build"),       // Build vote, not a runner
			okResult(2*time.Minute, "tu1", pytestFailingTail), // same bytes, wrong provenance
		)
		assertLegs(t, tr, legWant{journey.Build, 1 * time.Minute, 1 * time.Minute, 1, true})
		assertWaypoints(t, legOf(t, tr, 0))
	})

	t.Run("a non-runner tool_use inside a Test leg", func(t *testing.T) {
		tr := segment(
			bash(1*time.Minute, "tu1", "pytest tests/auth -x"), // Test leg, runner is tu1
			read(2*time.Minute, "tu2", "/w/tests/conftest.py"), // scout pressure inside it
			okResult(3*time.Minute, "tu2", pytestFailingTail),  // tu2 is not the runner
		)
		assertWaypoints(t, legOf(t, tr, 0))
	})

	t.Run("a result nobody voted for", func(t *testing.T) {
		tr := segment(
			bash(1*time.Minute, "tu1", "pytest tests/auth -x"),
			okResult(2*time.Minute, "tu_unknown", pytestFailingTail),
		)
		assertWaypoints(t, legOf(t, tr, 0))
	})
}

// T36 — errors count into the failed part: a test that could not run did not
// pass, and an error-only run must never compose a green-looking summary
// (amended pytest bullet).
func TestT36PytestCountsLineWithErrors(t *testing.T) {
	const tail = `=========================== short test summary info ============================
FAILED tests/auth/test_refresh.py::test_refresh_expired_token - AssertionError
ERROR tests/store/test_cache.py::test_cache_evicts_lru
=============== 1 failed, 18 passed, 2 errors in 1.31s ===============
`
	tr := segment(
		bash(1*time.Minute, "tu1", "pytest tests -x"),
		errResult(2*time.Minute, "tu1", tail),
	)
	leg := legOf(t, tr, 0)
	checkWaypointInvariants(t, leg)

	var runs int
	for _, w := range leg.Waypoints {
		if w.Kind != journey.WaypointTestRun {
			continue
		}
		runs++
		// The amended contract (pytest bullet): errors count into the failed
		// part — a test that could not run did not pass.
		if want := "18 passed · 3 failed"; w.Text != want {
			t.Errorf("TestRun = %q, want %q (1 failed + 2 errors)", w.Text, want)
		}
	}
	if runs != 1 {
		t.Errorf("got %d TestRun waypoints, want exactly 1:\n%s", runs, dumpWaypoints(leg))
	}
}

// T36 — an error-only run never reads as green: with nothing passed, the
// summary is the failed part alone.
func TestT36PytestErrorOnlyRunIsNotGreen(t *testing.T) {
	const tail = `=========================== short test summary info ============================
ERROR tests/store/test_cache.py - ImportError: cannot import name Cache
ERROR tests/store/test_evict.py - ImportError: cannot import name Cache
============================== 2 errors in 0.42s ===============================
`
	tr := segment(
		bash(1*time.Minute, "tu1", "pytest tests -x"),
		errResult(2*time.Minute, "tu1", tail),
	)
	leg := legOf(t, tr, 0)
	checkWaypointInvariants(t, leg)
	if len(leg.Waypoints) != 1 {
		t.Fatalf("got %d waypoints, want 1 summary:\n%s", len(leg.Waypoints), dumpWaypoints(leg))
	}
	got := leg.Waypoints[0]
	if got.Kind != journey.WaypointTestRun {
		t.Fatalf("Waypoints[0] = %s, want TestRun\n%s", kindName(got.Kind), dumpWaypoints(leg))
	}
	if want := "2 failed"; got.Text != want {
		t.Errorf("TestRun = %q, want %q — a zero passed count is omitted and the errors are failures",
			got.Text, want)
	}
}

// ---------------------------------------------------------------- T37 go test

// T37 — go test: a name per "--- FAIL:" line and an "N failing" summary.
func TestT37GoTestFailures(t *testing.T) {
	tr := segment(
		bash(1*time.Minute, "tu1", "go test ./internal/..."),
		errResult(2*time.Minute, "tu1", goTestFailingOutput),
	)
	assertLegs(t, tr, legWant{journey.Test, 1 * time.Minute, 1 * time.Minute, 1, true})
	assertWaypointsByKind(t, legOf(t, tr, 0),
		wp{journey.WaypointTestRun, "2 failing"},
		wp{journey.WaypointTestFail, "TestTailerResetsOnTruncation"},
		wp{journey.WaypointTestFail, "TestTailerHandlesPartialLine"},
	)
}

// T37 — an all-green run is a single "ok", whatever the package count.
func TestT37GoTestAllOK(t *testing.T) {
	tr := segment(
		bash(1*time.Minute, "tu1", "go test ./..."),
		okResult(2*time.Minute, "tu1", goTestOKOutput),
	)
	assertWaypoints(t, legOf(t, tr, 0), wp{journey.WaypointTestRun, "ok"})
}

// T37 — the duration suffix is not part of the name, subtests keep their path,
// and a repeated FAIL line is one waypoint.
func TestT37GoTestNameShapes(t *testing.T) {
	t.Run("subtest names keep their slash", func(t *testing.T) {
		const out = `--- FAIL: TestClassify (0.00s)
    --- FAIL: TestClassify/lowercase_read_is_not_Read (0.00s)
FAIL
`
		tr := segment(
			bash(1*time.Minute, "tu1", "go test ./internal/journey -run TestClassify"),
			errResult(2*time.Minute, "tu1", out),
		)
		leg := legOf(t, tr, 0)
		checkWaypointInvariants(t, leg)
		var names []string
		for _, w := range leg.Waypoints {
			if w.Kind == journey.WaypointTestFail {
				names = append(names, w.Text)
			}
		}
		want := []string{"TestClassify", "TestClassify/lowercase_read_is_not_Read"}
		if strings.Join(names, "|") != strings.Join(want, "|") {
			t.Errorf("failing names = %v, want %v (no %q suffix, indentation trimmed)",
				names, want, " (0.00s)")
		}
	})

	t.Run("a repeated FAIL line is deduped", func(t *testing.T) {
		const out = `--- FAIL: TestTailerResetsOnTruncation (0.00s)
--- FAIL: TestTailerResetsOnTruncation (0.00s)
FAIL
`
		tr := segment(
			bash(1*time.Minute, "tu1", "go test ./internal/transcript"),
			errResult(2*time.Minute, "tu1", out),
		)
		leg := legOf(t, tr, 0)
		checkWaypointInvariants(t, leg)

		var fails, runs int
		for _, w := range leg.Waypoints {
			switch w.Kind {
			case journey.WaypointTestFail:
				fails++
				if w.Text != "TestTailerResetsOnTruncation" {
					t.Errorf("TestFail = %q, want %q", w.Text, "TestTailerResetsOnTruncation")
				}
			case journey.WaypointTestRun:
				runs++
				// Whether "N" counts lines or distinct names is not fixed by the
				// contract; the shape is.
				if !strings.HasSuffix(w.Text, " failing") {
					t.Errorf("TestRun = %q, want an \"N failing\" summary", w.Text)
				}
			}
		}
		if fails != 1 {
			t.Errorf("got %d TestFail waypoints, want 1 (deduped):\n%s", fails, dumpWaypoints(leg))
		}
		if runs != 1 {
			t.Errorf("got %d TestRun waypoints, want 1:\n%s", runs, dumpWaypoints(leg))
		}
	})

	t.Run("more than three failures cap at three", func(t *testing.T) {
		var b strings.Builder
		for _, n := range []string{"TestA", "TestB", "TestC", "TestD", "TestE"} {
			fmt.Fprintf(&b, "--- FAIL: %s (0.00s)\n", n)
		}
		b.WriteString("FAIL\n")

		tr := segment(
			bash(1*time.Minute, "tu1", "go test ./..."),
			errResult(2*time.Minute, "tu1", b.String()),
		)
		assertWaypointsByKind(t, legOf(t, tr, 0),
			wp{journey.WaypointTestRun, "5 failing"},
			wp{journey.WaypointTestFail, "TestA"},
			wp{journey.WaypointTestFail, "TestB"},
			wp{journey.WaypointTestFail, "TestC"},
		)
	})
}

// ---------------------------------------------------------------- T38

// T38 — jest. The "Tests:" line owns the counts, not "Test Suites:", and the
// failing name comes from the ✕ row.
func TestT38JestFamily(t *testing.T) {
	tr := segment(
		bash(1*time.Minute, "tu1", "npx jest --watch=false"),
		errResult(2*time.Minute, "tu1", jestFailingOutput),
	)
	assertLegs(t, tr, legWant{journey.Test, 1 * time.Minute, 1 * time.Minute, 1, true})
	assertWaypointsByKind(t, legOf(t, tr, 0),
		wp{journey.WaypointTestRun, "12 passed · 1 failed"},
		wp{journey.WaypointTestFail, "refreshes expired token"},
	)
}

// The multiplication-sign spelling of the same glyph is equally valid.
func TestT38JestMultiplicationSignFailures(t *testing.T) {
	const out = `  × parses a truncated line (4 ms)
  ✓ parses a whole line (1 ms)

Tests:       1 failed, 12 passed, 13 total
`
	tr := segment(
		bash(1*time.Minute, "tu1", "npx vitest run"),
		errResult(2*time.Minute, "tu1", out),
	)
	assertWaypointsByKind(t, legOf(t, tr, 0),
		wp{journey.WaypointTestRun, "12 passed · 1 failed"},
		wp{journey.WaypointTestFail, "parses a truncated line"},
	)
}

// T38 — cargo. The repeated "failures:" name list must not double-count.
func TestT38CargoFamily(t *testing.T) {
	tr := segment(
		bash(1*time.Minute, "tu1", "cargo test --all-features"),
		errResult(2*time.Minute, "tu1", cargoFailingOutput),
	)
	assertWaypointsByKind(t, legOf(t, tr, 0),
		wp{journey.WaypointTestRun, "10 passed · 2 failed"},
		wp{journey.WaypointTestFail, "auth::refresh"},
		wp{journey.WaypointTestFail, "auth::revoke"},
	)
}

// A green cargo run composes only the passed part.
func TestT38CargoAllPassing(t *testing.T) {
	const out = `running 12 tests
test auth::refresh ... ok

test result: ok. 12 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out; finished in 0.02s
`
	tr := segment(
		bash(1*time.Minute, "tu1", "cargo test"),
		okResult(2*time.Minute, "tu1", out),
	)
	assertWaypoints(t, legOf(t, tr, 0), wp{journey.WaypointTestRun, "12 passed"})
}

// T38 — nothing matched: a failing run still says so, a clean one says nothing.
func TestT38UnmatchedOutput(t *testing.T) {
	const garbage = `Traceback (most recent call last):
  File "/usr/lib/python3.11/runpy.py", line 198, in _run_module_as_main
ModuleNotFoundError: No module named pytest
`

	t.Run("IsError falls back to failed", func(t *testing.T) {
		tr := segment(
			bash(1*time.Minute, "tu1", "pytest tests/auth -x"),
			errResult(2*time.Minute, "tu1", garbage),
		)
		assertWaypoints(t, legOf(t, tr, 0), wp{journey.WaypointTestRun, "failed"})
	})

	t.Run("clean and unmatched produces nothing", func(t *testing.T) {
		tr := segment(
			bash(1*time.Minute, "tu1", "make test"),
			okResult(2*time.Minute, "tu1", "make: Nothing to be done for 'test'.\n"),
		)
		assertWaypoints(t, legOf(t, tr, 0))
	})

	t.Run("clean and empty produces nothing", func(t *testing.T) {
		tr := segment(
			bash(1*time.Minute, "tu1", "make test"),
			okResult(2*time.Minute, "tu1", ""),
		)
		assertWaypoints(t, legOf(t, tr, 0))
	})

	t.Run("an errored empty result still says failed", func(t *testing.T) {
		tr := segment(
			bash(1*time.Minute, "tu1", "npm test"),
			errResult(2*time.Minute, "tu1", ""),
		)
		assertWaypoints(t, legOf(t, tr, 0), wp{journey.WaypointTestRun, "failed"})
	})
}

// ---------------------------------------------------------------- T39 bugs

// T39 — an IsError result on a Build leg leaves a bug signature: the first
// non-empty line of its text. (The same error also upgrades the leg to Fix,
// per M1 rule 5 — the waypoint is what M2 adds.)
func TestT39BugFromErrorOnBuildLeg(t *testing.T) {
	const stderr = "\n\n" +
		"internal/journey/waypoint.go:41:9: undefined: parseTestRun in a package that is quite long\n" +
		"internal/journey/waypoint.go:52:2: declared and not used: n\n"
	const firstLine = "internal/journey/waypoint.go:41:9: undefined: parseTestRun in a package that is quite long"

	tr := segment(
		edit(1*time.Minute, "tu1", "/w/internal/journey/waypoint.go"),
		errResult(2*time.Minute, "tu1", stderr),
	)
	assertLegs(t, tr, legWant{journey.Fix, 1 * time.Minute, 1 * time.Minute, 1, true})

	leg := legOf(t, tr, 0)
	checkWaypointInvariants(t, leg)
	if len(leg.Waypoints) != 1 {
		t.Fatalf("got %d waypoints, want 1 bug:\n%s", len(leg.Waypoints), dumpWaypoints(leg))
	}
	if leg.Waypoints[0].Kind != journey.WaypointBug {
		t.Fatalf("Waypoints[0] = %s, want Bug\n%s", kindName(leg.Waypoints[0].Kind), dumpWaypoints(leg))
	}
	assertClipped(t, leg.Waypoints[0].Text, firstLine)
	if !leg.Waypoints[0].At.Equal(at(2 * time.Minute)) {
		t.Errorf("Bug.At = %v, want the result's time %v", leg.Waypoints[0].At, at(2*time.Minute))
	}
}

// T39 — the clip is 60 RUNES, counted on the first non-empty line.
func TestT39BugTextIsClippedByRunes(t *testing.T) {
	tests := []struct {
		name string
		body string
		line string
	}{
		{"short line untouched", "boom: nil map write", "boom: nil map write"},
		{
			"exactly sixty runes untouched",
			strings.Repeat("e", 60) + "\nmore detail below",
			strings.Repeat("e", 60),
		},
		{
			"sixty-one runes are cut",
			strings.Repeat("e", 61) + "\nmore detail below",
			strings.Repeat("e", 61),
		},
		{
			"multibyte error text is cut by rune, not byte",
			strings.Repeat("é", 90),
			strings.Repeat("é", 90),
		},
		{
			"cjk error text",
			strings.Repeat("認", 80),
			strings.Repeat("認", 80),
		},
		{
			"leading blank and whitespace lines are skipped",
			"\n   \n\tError: connection refused\n at dial()",
			"Error: connection refused",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := segment(
				edit(1*time.Minute, "tu1", "/w/parser.go"),
				errResult(2*time.Minute, "tu1", tc.body),
			)
			leg := legOf(t, tr, 0)
			checkWaypointInvariants(t, leg)
			if len(leg.Waypoints) != 1 {
				t.Fatalf("got %d waypoints, want 1 bug:\n%s", len(leg.Waypoints), dumpWaypoints(leg))
			}
			assertClipped(t, leg.Waypoints[0].Text, tc.line)
		})
	}
}

// T39 — the same error text twice is one bug; four distinct ones cap at three.
func TestT39BugsDedupeAndCapAtThree(t *testing.T) {
	t.Run("the same text twice", func(t *testing.T) {
		tr := segment(
			edit(1*time.Minute, "tu1", "/w/parser.go"),
			errResult(2*time.Minute, "tu1", "undefined: parseToken\n at line 12"),
			edit(3*time.Minute, "tu2", "/w/parser.go"),
			errResult(4*time.Minute, "tu2", "undefined: parseToken\n at line 44"), // same first line
		)
		assertWaypoints(t, legOf(t, tr, 0), wp{journey.WaypointBug, "undefined: parseToken"})
	})

	t.Run("four distinct errors keep the first three", func(t *testing.T) {
		var evs []transcript.Event
		for i := 1; i <= 4; i++ {
			id := fmt.Sprintf("tu%d", i)
			evs = append(evs,
				edit(time.Duration(2*i-1)*time.Minute, id, "/w/parser.go"),
				errResult(time.Duration(2*i)*time.Minute, id, fmt.Sprintf("undefined: symbol%d", i)),
			)
		}
		assertWaypoints(t, legOf(t, segment(evs...), 0),
			wp{journey.WaypointBug, "undefined: symbol1"},
			wp{journey.WaypointBug, "undefined: symbol2"},
			wp{journey.WaypointBug, "undefined: symbol3"},
		)
	})
}

// T39 — rule 4 names Build and Fix legs only.
func TestT39BugsOnlyOnBuildOrFixLegs(t *testing.T) {
	t.Run("scout leg gets no bug and stays scout", func(t *testing.T) {
		tr := segment(
			read(1*time.Minute, "tu1", "/w/auth.go"),
			errResult(2*time.Minute, "tu1", "Error: file not found: /w/auth.go"),
		)
		// M1 rule 5: the Fix upgrade is for Build legs; a Scout leg stays Scout,
		// and with no upgrade there is no waypoint either.
		assertLegs(t, tr, legWant{journey.Scout, 1 * time.Minute, 1 * time.Minute, 1, true})
		assertWaypoints(t, legOf(t, tr, 0))
	})

	t.Run("docs leg gets no bug", func(t *testing.T) {
		tr := segment(
			edit(1*time.Minute, "tu1", "/w/docs/spec.md"),
			errResult(2*time.Minute, "tu1", "Error: string not found in file"),
		)
		assertLegs(t, tr, legWant{journey.Docs, 1 * time.Minute, 1 * time.Minute, 1, true})
		assertWaypoints(t, legOf(t, tr, 0))
	})

	t.Run("ship leg gets no bug", func(t *testing.T) {
		tr := segment(
			bash(1*time.Minute, "tu1", "git push origin main"),
			errResult(2*time.Minute, "tu1", "! [rejected] main -> main (non-fast-forward)"),
		)
		assertLegs(t, tr, legWant{journey.Ship, 1 * time.Minute, 1 * time.Minute, 1, true})
		assertWaypoints(t, legOf(t, tr, 0))
	})

	t.Run("a fix leg keeps collecting bugs", func(t *testing.T) {
		tr := segment(
			edit(1*time.Minute, "tu1", "/w/parser.go"),
			errResult(2*time.Minute, "tu1", "undefined: one"), // upgrades the leg to Fix
			edit(3*time.Minute, "tu2", "/w/parser.go"),
			errResult(4*time.Minute, "tu2", "undefined: two"), // lands on a Fix leg
		)
		assertLegs(t, tr, legWant{journey.Fix, 1 * time.Minute, 3 * time.Minute, 2, true})
		assertWaypoints(t, legOf(t, tr, 0),
			wp{journey.WaypointBug, "undefined: one"},
			wp{journey.WaypointBug, "undefined: two"},
		)
	})
}

// T39 — attachment (rule 1): with no leg open the waypoint lands on the last
// leg; with no legs at all it is dropped.
func TestT39BugAttachment(t *testing.T) {
	t.Run("no leg open attaches to the last leg", func(t *testing.T) {
		tr := segment(
			edit(1*time.Minute, "tu1", "/w/parser.go"),
			prompt(2*time.Minute, "hold on — show me the stack trace first"), // closes the leg
			errResult(3*time.Minute, "tu1", "panic: runtime error: index out of range [3]"),
		)
		if len(tr.Legs) != 1 {
			t.Fatalf("got %d legs, want 1:\n%s", len(tr.Legs), dumpLegs(tr))
		}
		if tr.Legs[0].Current {
			t.Fatalf("Legs[0] is still open; the prompt should have closed it")
		}
		assertWaypoints(t, legOf(t, tr, 0),
			wp{journey.WaypointBug, "panic: runtime error: index out of range [3]"})
	})

	t.Run("before any leg the waypoint is dropped", func(t *testing.T) {
		tr := segment(
			prompt(0, "fix the parser"),
			errResult(1*time.Minute, "tu1", "panic: runtime error: index out of range [3]"),
		)
		if len(tr.Legs) != 0 {
			t.Fatalf("got %d legs, want 0 — a result is not a vote:\n%s", len(tr.Legs), dumpLegs(tr))
		}
	})

	t.Run("waypoints never migrate with pressure votes", func(t *testing.T) {
		// The three consecutive Scout votes migrate wholesale into a new leg
		// (M1 rule 4), taking the span from +3m with them. The bug that landed
		// at +4m — while the Fix leg was still the open one — stays where it
		// landed, even though that instant now falls inside the new leg.
		tr := segment(
			edit(1*time.Minute, "tu1", "/w/parser.go"), // Build leg 0
			errResult(2*time.Minute, "tu1", "undefined: parseToken"),
			read(3*time.Minute, "tu2", "/w/a.go"), // Scout pressure 1 of 3
			errResult(4*time.Minute, "tu2", "cannot read /w/a.go: no such file"),
			read(5*time.Minute, "tu3", "/w/b.go"), // Scout pressure 2 of 3
			read(6*time.Minute, "tu4", "/w/c.go"), // Scout pressure 3 of 3 → split
		)
		assertLegs(t, tr,
			legWant{journey.Fix, 1 * time.Minute, 1 * time.Minute, 1, false},
			legWant{journey.Scout, 3 * time.Minute, 6 * time.Minute, 3, true},
		)
		assertWaypoints(t, legOf(t, tr, 0),
			wp{journey.WaypointBug, "undefined: parseToken"},
			wp{journey.WaypointBug, "cannot read /w/a.go: no such file"},
		)
		assertWaypoints(t, legOf(t, tr, 1))
	})
}

// ---------------------------------------------------------------- T40 commits

// T40 — a git commit result on a Ship-voted tool_use yields the subject.
func TestT40CommitSubjectFromShipResult(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{
			"main branch",
			"[main abc1234] fix token refresh\n 3 files changed, 21 insertions(+), 4 deletions(-)\n",
			"fix token refresh",
		},
		{
			"branch name with a slash",
			"[feat/auth 1a2b3c4] wip: parse the refresh window\n 1 file changed, 2 insertions(+)\n",
			"wip: parse the refresh window",
		},
		{
			"root commit",
			"[main (root-commit) 0f1e2d3] initial import\n 12 files changed\n",
			"initial import",
		},
		{
			"preamble before the commit line",
			"husky > pre-commit\nrunning gofmt...\n[main abc1234] tidy the segmenter\n",
			"tidy the segmenter",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := segment(
				bash(1*time.Minute, "tu1", `git commit -m "fix token refresh"`),
				okResult(2*time.Minute, "tu1", tc.out),
			)
			assertLegs(t, tr, legWant{journey.Ship, 1 * time.Minute, 1 * time.Minute, 1, true})
			assertWaypoints(t, legOf(t, tr, 0), wp{journey.WaypointCommit, tc.want})
		})
	}
}

// A long commit subject obeys the same 60-rune budget.
func TestT40CommitSubjectIsClipped(t *testing.T) {
	subject := "fix the refresh window so expired tokens are rejected before the store is touched"
	tr := segment(
		bash(1*time.Minute, "tu1", `git commit -m "long"`),
		okResult(2*time.Minute, "tu1", "[main abc1234] "+subject+"\n 1 file changed\n"),
	)
	leg := legOf(t, tr, 0)
	checkWaypointInvariants(t, leg)
	if len(leg.Waypoints) != 1 {
		t.Fatalf("got %d waypoints, want 1:\n%s", len(leg.Waypoints), dumpWaypoints(leg))
	}
	assertClipped(t, leg.Waypoints[0].Text, subject)
}

// T40 — a PR URL anywhere in a Ship result becomes the waypoint, URL only.
func TestT40PullRequestURLFromShipResult(t *testing.T) {
	const out = `Creating pull request for claude/session-journey into main in o/r

https://github.com/o/r/pull/7
`
	tr := segment(
		bash(1*time.Minute, "tu1", "gh pr create --fill"),
		okResult(2*time.Minute, "tu1", out),
	)
	assertWaypoints(t, legOf(t, tr, 0), wp{journey.WaypointCommit, "https://github.com/o/r/pull/7"})
}

// The URL is a token on its line, not the whole line.
func TestT40PullRequestURLIsATokenNotALine(t *testing.T) {
	const out = "View this pull request at https://github.com/deephanson94/compass/pull/12 now\n"
	tr := segment(
		bash(1*time.Minute, "tu1", "gh pr create --fill"),
		okResult(2*time.Minute, "tu1", out),
	)
	leg := legOf(t, tr, 0)
	checkWaypointInvariants(t, leg)
	if len(leg.Waypoints) != 1 {
		t.Fatalf("got %d waypoints, want 1:\n%s", len(leg.Waypoints), dumpWaypoints(leg))
	}
	got := leg.Waypoints[0]
	if got.Kind != journey.WaypointCommit {
		t.Errorf("Waypoints[0] = %s, want Commit", kindName(got.Kind))
	}
	if want := "https://github.com/deephanson94/compass/pull/12"; got.Text != want {
		t.Errorf("Commit = %q, want the URL token %q", got.Text, want)
	}
}

// T40 — rule 5: the very same lines on a result that was not Ship-voted make no
// waypoint at all.
func TestT40CommitLinesOnANonShipResultAreIgnored(t *testing.T) {
	const out = "[main abc1234] fix token refresh\nhttps://github.com/o/r/pull/7\n"

	t.Run("scout vote", func(t *testing.T) {
		tr := segment(
			bash(1*time.Minute, "tu1", "cat .git/COMMIT_EDITMSG"),
			okResult(2*time.Minute, "tu1", out),
		)
		assertLegs(t, tr, legWant{journey.Scout, 1 * time.Minute, 1 * time.Minute, 1, true})
		assertWaypoints(t, legOf(t, tr, 0))
	})

	t.Run("build vote", func(t *testing.T) {
		tr := segment(
			bash(1*time.Minute, "tu1", "git log --oneline -1"),
			okResult(2*time.Minute, "tu1", out),
		)
		assertLegs(t, tr, legWant{journey.Build, 1 * time.Minute, 1 * time.Minute, 1, true})
		assertWaypoints(t, legOf(t, tr, 0))
	})

	t.Run("a non-runner tool_use inside a ship leg", func(t *testing.T) {
		tr := segment(
			bash(1*time.Minute, "tu1", "git push origin main"), // Ship leg; tu1 is the ship vote
			read(2*time.Minute, "tu2", "/w/CHANGELOG.md"),      // not a ship vote
			okResult(3*time.Minute, "tu2", out),
		)
		assertWaypoints(t, legOf(t, tr, 0))
	})
}

// A Ship result that says nothing about a commit or PR makes no waypoint.
func TestT40ShipResultWithNothingToReport(t *testing.T) {
	tr := segment(
		bash(1*time.Minute, "tu1", "git push origin main"),
		okResult(2*time.Minute, "tu1", "Everything up-to-date\n"),
	)
	assertWaypoints(t, legOf(t, tr, 0))
}

// ---------------------------------------------------------------- cap 8

// The per-leg cap is 8 and the ninth waypoint drops silently: three failing
// pytest runs on one Test leg produce 4 + 4 + 4 candidates.
func TestWaypointsCapAtEightPerLeg(t *testing.T) {
	first := pytestOutput(10, "tests/a.py::a_one", "tests/a.py::a_two", "tests/a.py::a_three")
	second := pytestOutput(10, "tests/b.py::b_one", "tests/b.py::b_two", "tests/b.py::b_three")
	third := pytestOutput(10, "tests/c.py::c_one", "tests/c.py::c_two", "tests/c.py::c_three")

	tr := segment(
		bash(1*time.Minute, "tu1", "pytest tests -x"),
		errResult(2*time.Minute, "tu1", first),
		bash(3*time.Minute, "tu2", "pytest tests -x"), // same class: one leg
		errResult(4*time.Minute, "tu2", second),
		bash(5*time.Minute, "tu3", "pytest tests -x"),
		errResult(6*time.Minute, "tu3", third),
	)
	assertLegs(t, tr, legWant{journey.Test, 1 * time.Minute, 5 * time.Minute, 3, true})

	leg := legOf(t, tr, 0)
	checkWaypointInvariants(t, leg)
	if len(leg.Waypoints) != 8 {
		t.Fatalf("got %d waypoints, want exactly 8 (the cap):\n%s", len(leg.Waypoints), dumpWaypoints(leg))
	}
	// The first two results fill the leg; the third contributes nothing.
	for i, w := range leg.Waypoints {
		wantAt := at(2 * time.Minute)
		if i >= 4 {
			wantAt = at(4 * time.Minute)
		}
		if !w.At.Equal(wantAt) {
			t.Errorf("Waypoints[%d] (%s %q) At = +%v, want +%v — the cap drops the newest, not the oldest",
				i, kindName(w.Kind), w.Text, w.At.Sub(base), wantAt.Sub(base))
		}
		if strings.HasPrefix(w.Text, "c_") {
			t.Errorf("Waypoints[%d] = %q came from the ninth candidate; it should have dropped", i, w.Text)
		}
	}
}

// The cap is per leg, not per trail.
func TestWaypointsCapIsPerLeg(t *testing.T) {
	tr := segment(
		bash(1*time.Minute, "tu1", "pytest tests -x"),
		errResult(2*time.Minute, "tu1", pytestOutput(10, "tests/a.py::a_one")),
		bash(3*time.Minute, "tu2", "git commit -m fix"), // strong split → Ship leg
		okResult(4*time.Minute, "tu2", "[main abc1234] fix it\n"),
	)
	assertLegs(t, tr,
		legWant{journey.Test, 1 * time.Minute, 1 * time.Minute, 1, false},
		legWant{journey.Ship, 3 * time.Minute, 3 * time.Minute, 1, true},
	)
	assertWaypointsByKind(t, legOf(t, tr, 0),
		wp{journey.WaypointTestRun, "10 passed · 1 failed"},
		wp{journey.WaypointTestFail, "a_one"},
	)
	assertWaypoints(t, legOf(t, tr, 1), wp{journey.WaypointCommit, "fix it"})
}

// Trail() is a snapshot: waypoints do not alias the segmenter's own slices.
func TestWaypointsAreSnapshotted(t *testing.T) {
	s := journey.NewSegmenter()
	for _, ev := range []transcript.Event{
		bash(1*time.Minute, "tu1", "pytest tests -x"),
		errResult(2*time.Minute, "tu1", pytestOutput(10, "tests/a.py::a_one")),
	} {
		s.Observe(ev)
	}

	first := s.Trail()
	if len(first.Legs) != 1 || len(first.Legs[0].Waypoints) == 0 {
		t.Fatalf("unexpected trail:\n%s", dumpLegs(first))
	}
	first.Legs[0].Waypoints[0].Text = "MUTATED"

	if got := s.Trail().Legs[0].Waypoints[0].Text; got == "MUTATED" {
		t.Errorf("Trail() aliases the segmenter's waypoints")
	}
}

// ---------------------------------------------------------------- T41 report

// T41 — Branch.Report is the first non-empty line of the Agent result's text.
func TestT41BranchReport(t *testing.T) {
	s := journey.NewSegmenter()
	for _, ev := range []transcript.Event{
		read(1*time.Minute, "tu1", "/w/auth.go"),
		agent(2*time.Minute, "tu_agent", "scout payment flows"),
	} {
		s.Observe(ev)
	}

	if got := s.Trail().Branches[0].Report; got != "" {
		t.Errorf("Branch.Report = %q before the result, want %q until Done", got, "")
	}

	s.Observe(okResult(3*time.Minute, "tu_agent", "\n\nScouted the payments module.\nDetails follow:\n- charges.go\n"))

	tr := s.Trail()
	if len(tr.Branches) != 1 {
		t.Fatalf("got %d branches, want 1: %+v", len(tr.Branches), tr.Branches)
	}
	br := tr.Branches[0]
	if !br.Done {
		t.Fatalf("Branch.Done = false after its tool_result")
	}
	if want := "Scouted the payments module."; br.Report != want {
		t.Errorf("Branch.Report = %q, want %q", br.Report, want)
	}
	// An Agent result is neither Test- nor Ship-voted, and it is not an error.
	assertWaypoints(t, legOf(t, tr, 0))
}

// T41 — the report obeys the 60-rune budget and the single-line rule.
func TestT41BranchReportShapes(t *testing.T) {
	tests := []struct {
		name string
		body string
		line string
	}{
		{"single line", "Found three call sites.", "Found three call sites."},
		{"blank lines first", "\n\n\nFound three call sites.\ntail", "Found three call sites."},
		{"whitespace-only lines first", "   \n\t\nFound three call sites.", "Found three call sites."},
		{
			"long first line is clipped",
			"The payments module routes every charge through a legacy adapter that nobody owns.\nmore",
			"The payments module routes every charge through a legacy adapter that nobody owns.",
		},
		{
			"multibyte first line is clipped by rune",
			strings.Repeat("é", 90),
			strings.Repeat("é", 90),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := segment(
				read(1*time.Minute, "tu1", "/w/auth.go"),
				agent(2*time.Minute, "tu_agent", "scout"),
				okResult(3*time.Minute, "tu_agent", tc.body),
			)
			if len(tr.Branches) != 1 {
				t.Fatalf("got %d branches, want 1", len(tr.Branches))
			}
			got := tr.Branches[0].Report
			if strings.ContainsAny(got, "\n\r") {
				t.Errorf("Branch.Report = %q, want a single line", got)
			}
			assertClipped(t, got, tc.line)
		})
	}
}

// T41 — only the branch's own result writes its report, and an empty result
// leaves it empty.
func TestT41BranchReportProvenance(t *testing.T) {
	t.Run("another tool's result does not report", func(t *testing.T) {
		tr := segment(
			read(1*time.Minute, "tu1", "/w/auth.go"),
			agent(2*time.Minute, "tu_agent", "scout"),
			okResult(3*time.Minute, "tu1", "Scouted the payments module."),
		)
		if got := tr.Branches[0].Report; got != "" {
			t.Errorf("Branch.Report = %q, want %q: that result belonged to tu1", got, "")
		}
		if tr.Branches[0].Done {
			t.Errorf("Branch.Done = true; tu1's result must not close tu_agent's branch")
		}
	})

	t.Run("an empty result leaves the report empty", func(t *testing.T) {
		tr := segment(
			read(1*time.Minute, "tu1", "/w/auth.go"),
			agent(2*time.Minute, "tu_agent", "scout"),
			okResult(3*time.Minute, "tu_agent", "\n \n"),
		)
		if !tr.Branches[0].Done {
			t.Errorf("Branch.Done = false, want true: the result did arrive")
		}
		if got := tr.Branches[0].Report; got != "" {
			t.Errorf("Branch.Report = %q, want %q when there is no non-empty line", got, "")
		}
	})

	t.Run("two branches keep their own reports", func(t *testing.T) {
		tr := segment(
			read(1*time.Minute, "tu1", "/w/auth.go"),
			agent(2*time.Minute, "tu_a1", "scout payments"),
			agent(3*time.Minute, "tu_a2", "scout auth"),
			okResult(4*time.Minute, "tu_a2", "Auth is three files deep."),
			okResult(5*time.Minute, "tu_a1", "Payments go through a legacy adapter."),
		)
		if len(tr.Branches) != 2 {
			t.Fatalf("got %d branches, want 2", len(tr.Branches))
		}
		if want := "Payments go through a legacy adapter."; tr.Branches[0].Report != want {
			t.Errorf("Branches[0].Report = %q, want %q", tr.Branches[0].Report, want)
		}
		if want := "Auth is three files deep."; tr.Branches[1].Report != want {
			t.Errorf("Branches[1].Report = %q, want %q", tr.Branches[1].Report, want)
		}
	})
}

// An Agent whose result is an error still reports, and — being an IsError
// result on a Build leg — also leaves a bug (rule 4 says "any tool").
func TestT41BranchReportOnAFailedAgent(t *testing.T) {
	tr := segment(
		edit(1*time.Minute, "tu1", "/w/parser.go"), // Build leg
		agent(2*time.Minute, "tu_agent", "scout"),
		errResult(3*time.Minute, "tu_agent", "Error: the subagent exceeded its budget"),
	)
	if want := "Error: the subagent exceeded its budget"; tr.Branches[0].Report != want {
		t.Errorf("Branch.Report = %q, want %q", tr.Branches[0].Report, want)
	}
	assertWaypoints(t, legOf(t, tr, 0),
		wp{journey.WaypointBug, "Error: the subagent exceeded its budget"})
}

// ---------------------------------------------------------------- end to end

// One session that exercises every extractor at once.
func TestWaypointsFullWalk(t *testing.T) {
	tr := segment(
		prompt(0, "the auth tests are failing, please fix them"),
		read(1*time.Minute, "tu1", "/w/auth.go"), // leg 0: scout
		agent(2*time.Minute, "tu_agent", "scout payment flows"),
		bash(3*time.Minute, "tu2", "pytest tests/auth -x"), // leg 1: test
		errResult(4*time.Minute, "tu2", pytestFailingTail),
		okResult(5*time.Minute, "tu_agent", "Payments never touch the refresh path.\nDetails…"),
		edit(6*time.Minute, "tu3", "/w/auth.go"), // leg 2: build → fix
		edit(7*time.Minute, "tu4", "/w/auth.go"),
		edit(8*time.Minute, "tu5", "/w/auth.go"),
		errResult(9*time.Minute, "tu5", "auth.go:12:2: undefined: refreshWindow"),
		bash(10*time.Minute, "tu6", "pytest tests/auth -x"), // leg 3: test
		okResult(11*time.Minute, "tu6", pytestCleanTail),
		bash(12*time.Minute, "tu7", `git commit -m "fix token refresh"`), // leg 4: ship
		okResult(13*time.Minute, "tu7", "[main abc1234] fix token refresh\n 2 files changed\n"),
	)

	assertLegs(t, tr,
		legWant{journey.Scout, 1 * time.Minute, 1 * time.Minute, 1, false},
		legWant{journey.Test, 3 * time.Minute, 3 * time.Minute, 1, false},
		legWant{journey.Fix, 6 * time.Minute, 8 * time.Minute, 3, false},
		legWant{journey.Test, 10 * time.Minute, 10 * time.Minute, 1, false},
		legWant{journey.Ship, 12 * time.Minute, 12 * time.Minute, 1, true},
	)

	assertWaypoints(t, legOf(t, tr, 0))
	assertWaypointsByKind(t, legOf(t, tr, 1),
		wp{journey.WaypointTestRun, "18 passed · 2 failed"},
		wp{journey.WaypointTestFail, "test_refresh_expired_token"},
		wp{journey.WaypointTestFail, "test_revoke_idempotent"},
	)
	assertWaypoints(t, legOf(t, tr, 2), wp{journey.WaypointBug, "auth.go:12:2: undefined: refreshWindow"})
	assertWaypoints(t, legOf(t, tr, 3), wp{journey.WaypointTestRun, "18 passed"})
	assertWaypoints(t, legOf(t, tr, 4), wp{journey.WaypointCommit, "fix token refresh"})

	if len(tr.Branches) != 1 {
		t.Fatalf("got %d branches, want 1", len(tr.Branches))
	}
	if want := "Payments never touch the refresh path."; tr.Branches[0].Report != want {
		t.Errorf("Branch.Report = %q, want %q", tr.Branches[0].Report, want)
	}
}
