package fleet_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
)

// ProbeScan reports each transcript as the walk met it: answered by the cache,
// or peeked and why. `compass perf` prints these reasons, so each one is the
// stat that actually moved.
func TestProbeScanSaysWhyAFileWasRead(t *testing.T) {
	root, rest := resumeRoot(t, midTurn, func(b *transcriptBuilder) {
		b.text(ago(time.Minute), "still going")
	})
	path := filepath.Join(root, "projects", "-home-user-app", "77777777-7777-4777-8777-777777777777.jsonl")
	c := fleet.OpenResumeCache(filepath.Join(t.TempDir(), "resume.json"))
	quiet := fleetNow.Add(-fleet.DefaultLiveWindow) // the files were written after it: narrow peeks
	why := func(c *fleet.ResumeCache, quiet time.Time) fleet.ScanFile {
		t.Helper()
		_, files, err := fleet.ProbeScan(root, c, quiet)
		if err != nil || len(files) != 1 {
			t.Fatalf("ProbeScan: %v, %d files", err, len(files))
		}
		return files[0]
	}
	refresh := func() {
		m := liveManager(root)
		m.UseResumeCache(c)
		snapOf(t, m)
	}

	if f := why(nil, quiet); f.Cached || f.Why != "new" {
		t.Errorf("no cache: got cached=%v why=%q, want a peek because the file is new", f.Cached, f.Why)
	}
	refresh()
	if f := why(c, quiet); !f.Cached {
		t.Errorf("unchanged file: peeked (%q), want answered by the cache", f.Why)
	}
	if f := why(c, time.Now().Add(time.Hour)); f.Cached || f.Why != "fell quiet" {
		t.Errorf("file older than the quiet mark: got cached=%v why=%q, want the wide peek it is owed", f.Cached, f.Why)
	}
	rest()
	if f := why(c, quiet); f.Cached || f.Why != "grew" {
		t.Errorf("appended file: got cached=%v why=%q, want grew", f.Cached, f.Why)
	}
	refresh()
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	if f := why(c, quiet); f.Cached || f.Why != "touched" {
		t.Errorf("mtime moved, size did not: got cached=%v why=%q, want touched", f.Cached, f.Why)
	}
}

// A cache opened read-only is used like any other and never written: a
// diagnostic that measures the next launch must not change it.
func TestReadOnlyResumeCacheIsNeverWritten(t *testing.T) {
	root, _ := resumeRoot(t, midTurn, func(*transcriptBuilder) {})
	cachePath := filepath.Join(t.TempDir(), "resume.json")
	c := fleet.OpenResumeCacheReadOnly(cachePath)
	m := liveManager(root)
	m.UseResumeCache(c)
	snapOf(t, m)
	c.Save()
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("a read-only cache was written to %s (stat: %v)", cachePath, err)
	}
	if st := c.Stats(); st.Peeked != 1 || st.Points != 1 {
		t.Errorf("stats = %+v, want the refresh's one peek and one live mark in memory", st)
	}
}
