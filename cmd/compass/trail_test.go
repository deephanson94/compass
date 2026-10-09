package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/transcript"
)

// line is one transcript line in the shape Claude Code writes.
func line(kind string, at time.Time, content string) string {
	return fmt.Sprintf(`{"type":%q,"isSidechain":false,"timestamp":%q,"message":{"role":%q,"content":%s}}`,
		kind, at.Format(time.RFC3339Nano), kind, content)
}

// The snapshot a mod reads: the legs by class name, the test leg's run and
// failing test as waypoints, the block's counts, and the latest outcome.
func TestTrailSnapshot(t *testing.T) {
	t0 := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	lines := []string{
		line("user", t0, `"fix the auth tests"`),
		line("assistant", t0.Add(time.Minute), `[{"type":"tool_use","id":"r1","name":"Read","input":{"file_path":"/src/auth.py"}}]`),
		line("user", t0.Add(time.Minute), `[{"type":"tool_result","tool_use_id":"r1","content":"..."}]`),
		line("assistant", t0.Add(2*time.Minute), `[{"type":"tool_use","id":"b1","name":"Bash","input":{"command":"pytest tests/auth -x"}}]`),
		line("user", t0.Add(3*time.Minute), `[{"type":"tool_result","tool_use_id":"b1","is_error":true,"content":"FAILED tests/auth/test_refresh.py::test_expiry - AssertionError\n18 passed, 2 failed in 1.2s"}]`),
		line("user", t0.Add(4*time.Minute), `"note it"`),
		line("assistant", t0.Add(5*time.Minute), `[{"type":"tool_use","id":"w1","name":"Bash","input":{"command":"cat >> NOTES.md <<'EOF'\nred again\nEOF"}}]`),
	}
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tl := transcript.NewTailer(path)
	evs, err := tl.Poll()
	if err != nil {
		t.Fatal(err)
	}
	seg, outs, acts := journey.NewSegmenter(), journey.NewOutcomes(), &actLog{}
	for _, ev := range evs {
		seg.Observe(ev)
		outs.Observe(ev)
		acts.observe(ev)
	}
	var buf bytes.Buffer
	if err := writeSnapshot(&buf, path, seg, outs, acts, t0.Add(6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(buf.String(), "\n"); n != 1 {
		t.Fatalf("a snapshot is one NDJSON line, got %d", n)
	}
	var s trailSnapshot
	if err := json.Unmarshal(buf.Bytes(), &s); err != nil {
		t.Fatal(err)
	}

	if len(s.Prompts) != 2 || s.Prompts[0].Text != "fix the auth tests" {
		t.Errorf("prompts = %+v", s.Prompts)
	}
	if head := s.Legs[len(s.Legs)-1]; head.Class != "docs" || !head.Current {
		t.Errorf("HEAD = %+v, want the open docs leg the shell write opened", head)
	}
	if s.LastEdit == nil || !s.LastEdit.Equal(t0.Add(5*time.Minute)) {
		t.Errorf("lastEdit = %v, want the shell write at +5m", s.LastEdit)
	}
	last := s.Legs[len(s.Legs)-2]
	if last.Class != "test" {
		t.Fatalf("legs = %+v, want the test leg before HEAD", s.Legs)
	}
	var kinds []string
	for _, w := range last.Waypoints {
		kinds = append(kinds, w.Kind)
	}
	if got := strings.Join(kinds, ","); !strings.Contains(got, "testRun") || !strings.Contains(got, "testFail") {
		t.Errorf("waypoint kinds = %s, want a run and a failure", got)
	}
	if got := strings.Join(last.Acts, " | "); got != "$ pytest tests/auth -x" {
		t.Errorf("test leg acts = %q, want its own command alone", got)
	}
	if got := strings.Join(s.Legs[0].Acts, " | "); got != "read auth.py" {
		t.Errorf("scout leg acts = %q", got)
	}
	if c := s.Counts["test"]; c.Legs != 1 || c.Red != 1 {
		t.Errorf("counts[test] = %+v, want one red leg", c)
	}
	if s.Outcome == nil || s.Outcome.Kind != "testRun" || s.Outcome.Short != "18✓ 2✗" {
		t.Errorf("outcome = %+v, want the run's 18✓ 2✗", s.Outcome)
	}
}

func TestCommandCore(t *testing.T) {
	for in, want := range map[string]string{
		"M=/root/x/compass-trail; cat $M/diag.log": "cat $M/diag.log",
		"cd /home/user/compass && go test ./...":   "go test ./...",
		`A="x y" && B=2; cd /tmp && ls`:            "ls",
		"go vet ./...":                             "go vet ./...",
		"X=1":                                      "X=1",
	} {
		if got := commandCore(in); got != want {
			t.Errorf("commandCore(%q) = %q, want %q", in, got, want)
		}
	}
}
