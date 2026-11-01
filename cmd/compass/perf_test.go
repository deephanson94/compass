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

// `compass perf` times the launch on the machine's own transcripts and leaves
// the resume cache exactly as it found it — missing stays missing, and a saved
// one is read, not rewritten.
func TestPerfTimesTheLaunchAndWritesNothing(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "projects", "-home-user-alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	at := func(d time.Duration) string { return now.Add(-d).Format(time.RFC3339Nano) }
	const id = "aaaaaaaa-0000-4000-8000-000000000001"
	lines := []string{
		`{"type":"user","sessionId":"` + id + `","cwd":"/home/user/alpha","timestamp":"` + at(3*time.Minute) + `","message":{"role":"user","content":"file the issue"}}`,
		`{"type":"assistant","sessionId":"` + id + `","cwd":"/home/user/alpha","timestamp":"` + at(2*time.Minute) + `","message":{"role":"assistant","content":[{"type":"text","text":"on it"}]}}`,
	}
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cachePath := filepath.Join(t.TempDir(), "compass", "resume.json")
	fresh := func() *fleet.Manager { return fleet.NewManager(root) }

	var out bytes.Buffer
	printPerf(&out, root, cachePath, fleet.DefaultLiveWindow, fresh, now)
	got := out.String()
	for _, want := range []string{"LAUNCH", "refresh, from cache", "refresh, cold", "LIVE SESSIONS", "alpha aaaaaaaa", "all · no mark", "no resume cache"} {
		if !strings.Contains(got, want) {
			t.Errorf("perf without a cache does not say %q:\n%s", want, got)
		}
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("perf wrote a resume cache (stat: %v)", err)
	}

	// What a deck leaves behind: the scan and the session's mark.
	c := fleet.OpenResumeCache(cachePath)
	m := fresh()
	m.UseResumeCache(c)
	if _, err := m.Refresh(now); err != nil {
		t.Fatal(err)
	}
	c.Save()
	saved, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}

	out.Reset()
	printPerf(&out, root, cachePath, fleet.DefaultLiveWindow, fresh, now)
	got = out.String()
	for _, want := range []string{"1 cached · 0 peeked", "1 live marks"} {
		if !strings.Contains(got, want) {
			t.Errorf("perf with a cache does not say %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "no resume cache") {
		t.Errorf("perf with a cache says there is none:\n%s", got)
	}
	if after, _ := os.ReadFile(cachePath); !bytes.Equal(after, saved) {
		t.Error("perf rewrote the resume cache")
	}
}

// A transcript the scan reads but the fleet drops — the narrator's own
// headless sessions — is counted, so a scan of thousands beside a fleet of
// hundreds says where the rest went.
func TestPerfCountsTheTranscriptsTheFleetExcludes(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	write := func(slug, id, cwd string) {
		dir := filepath.Join(root, "projects", slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		line := `{"type":"user","sessionId":"` + id + `","cwd":"` + cwd + `","timestamp":"` + now.Add(-48*time.Hour).Format(time.RFC3339Nano) + `","message":{"role":"user","content":"label this leg"}}` + "\n"
		if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	narrator := filepath.Join(t.TempDir(), "narrator")
	write("-home-user-alpha", "aaaaaaaa-0000-4000-8000-000000000001", "/home/user/alpha")
	write("-narrator", "bbbbbbbb-0000-4000-8000-000000000001", narrator)
	fresh := func() *fleet.Manager {
		m := fleet.NewManager(root)
		m.ExcludeCWD(narrator)
		return m
	}
	var out bytes.Buffer
	printPerf(&out, root, filepath.Join(t.TempDir(), "resume.json"), fleet.DefaultLiveWindow, fresh, now)
	if got := out.String(); !strings.Contains(got, "0 live · 1 archived · 1 excluded") {
		t.Errorf("perf does not count the excluded transcript:\n%s", got)
	}
}
