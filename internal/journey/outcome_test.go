package journey_test

import (
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/journey"
)

// The badge form is composed beside the long form, from the same counts, at
// the one place both are made. These fixtures are real runner output, so the
// numbers travel the whole way: parser → waypoint → badge.
func TestRunSummariesCarryABadgeForm(t *testing.T) {
	for _, tc := range []struct {
		name, cmd, out string
		wantText       string
		wantShort      string
	}{
		{"pytest, some failed", "pytest tests/auth -x",
			"===== 2 failed, 18 passed in 0.42s =====",
			"18 passed · 2 failed", "18✓ 2✗"},
		{"pytest, all green", "pytest",
			"===== 1190 passed in 12.01s =====",
			"1190 passed", "1190✓"},
		{"pytest errors count as failures", "pytest",
			"===== 1 error, 3 passed in 0.10s =====",
			"3 passed · 1 failed", "3✓ 1✗"},
		{"go test, green", "go test ./...",
			"ok  \tgithub.com/x/y\t0.01s",
			"ok", "✓"},
		{"go test, failing", "go test ./...",
			"--- FAIL: TestOne (0.00s)\n--- FAIL: TestTwo (0.00s)\nFAIL",
			"2 failing", "2✗"},
		{"jest", "jest",
			"Tests:       2 failed, 18 passed, 20 total",
			"18 passed · 2 failed", "18✓ 2✗"},
		{"cargo", "cargo test",
			"test result: FAILED. 18 passed; 2 failed; 0 ignored; 0 measured",
			"18 passed · 2 failed", "18✓ 2✗"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := journey.NewOutcomes()
			o.Observe(bash(1*time.Minute, "t1", tc.cmd))
			o.Observe(okResult(2*time.Minute, "t1", tc.out))

			got, ok := o.Latest()
			if !ok {
				t.Fatalf("no outcome parsed from:\n%s", tc.out)
			}
			if got.Text != tc.wantText {
				t.Errorf("text is %q, want %q", got.Text, tc.wantText)
			}
			if got.Short != tc.wantShort {
				t.Errorf("badge is %q, want %q", got.Short, tc.wantShort)
			}
		})
	}
}

// Outcomes keeps the segmenter's own rules (M2, rules 2 and 3): only a
// remembered Test or Ship call's own result is read as a run, and a subagent's
// work is its own, not this session's.
func TestOutcomesKeepsTheSegmentersRules(t *testing.T) {
	green := "===== 1190 passed in 12.01s ====="
	red := "===== 2 failed, 18 passed in 0.42s ====="

	t.Run("a result nobody asked for is not a run", func(t *testing.T) {
		o := journey.NewOutcomes()
		o.Observe(okResult(1*time.Minute, "never-issued", red))
		if got, ok := o.Latest(); ok {
			t.Errorf("parsed %q out of an unremembered result", got.Text)
		}
	})

	t.Run("a non-test command's output is not a run", func(t *testing.T) {
		o := journey.NewOutcomes()
		o.Observe(bash(1*time.Minute, "t1", "cat last-run.log"))
		o.Observe(okResult(2*time.Minute, "t1", red))
		if got, ok := o.Latest(); ok {
			t.Errorf("read %q out of somebody else's log", got.Text)
		}
	})

	t.Run("a subagent's run is not this session's", func(t *testing.T) {
		o := journey.NewOutcomes()
		side := bash(1*time.Minute, "t1", "pytest")
		side.IsSidechain = true
		sideRes := okResult(2*time.Minute, "t1", red)
		sideRes.IsSidechain = true
		o.Observe(side)
		o.Observe(sideRes)
		if got, ok := o.Latest(); ok {
			t.Errorf("took %q from a subagent's own test run", got.Text)
		}
	})

	t.Run("the newest run wins", func(t *testing.T) {
		o := journey.NewOutcomes()
		o.Observe(bash(1*time.Minute, "t1", "pytest"))
		o.Observe(okResult(2*time.Minute, "t1", red))
		o.Observe(bash(3*time.Minute, "t2", "pytest"))
		o.Observe(okResult(4*time.Minute, "t2", green))

		got, _ := o.Latest()
		if got.Short != "1190✓" {
			t.Errorf("badge is %q, want the newer run's 1190✓", got.Short)
		}
	})

	t.Run("a commit is an outcome too", func(t *testing.T) {
		o := journey.NewOutcomes()
		o.Observe(bash(1*time.Minute, "t1", "git commit -m x"))
		o.Observe(okResult(2*time.Minute, "t1", "[main 1a2b3c4] wire the segmenter in"))
		got, ok := o.Latest()
		if !ok || got.Text != "wire the segmenter in" {
			t.Errorf("commit outcome is %+v", got)
		}
	})

	t.Run("a nil fold answers nothing", func(t *testing.T) {
		var o *journey.Outcomes
		if _, ok := o.Latest(); ok {
			t.Error("a nil Outcomes claimed a result")
		}
	})
}
