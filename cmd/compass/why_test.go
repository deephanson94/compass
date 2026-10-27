package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
)

// `compass why` names the call a session holds open and the last line it
// counted as someone speaking.
func TestWhyNamesTheOpenCallAndTheLastBeat(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "projects", "-home-user-alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	at := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339Nano) }
	lines := []string{
		`{"type":"user","sessionId":"aaaaaaaa-0000-4000-8000-000000000001","cwd":"/home/user/alpha","timestamp":"` + at(3*time.Minute) + `","message":{"role":"user","content":"file the issue"}}`,
		`{"type":"assistant","sessionId":"aaaaaaaa-0000-4000-8000-000000000001","cwd":"/home/user/alpha","timestamp":"` + at(2*time.Minute) + `","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"SendMessage","input":{"to":"peer"}}]}}`,
	}
	path := filepath.Join(dir, "aaaaaaaa-0000-4000-8000-000000000001.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr := fleet.NewManager(root)
	sessions, err := mgr.Refresh(now)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	printWhy(&out, mgr, sessions, now)
	got := out.String()
	for _, want := range []string{"alpha", "state    working (open, pending=tool)", "open     SendMessage toolu_1, called 2m ago", `beat     assistant, 2m ago`} {
		if !strings.Contains(got, want) {
			t.Errorf("compass why does not say %q:\n%s", want, got)
		}
	}
}
