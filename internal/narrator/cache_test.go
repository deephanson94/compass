package narrator_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/deephanson94/compass/internal/narrator"
)

// cachePath is a fresh, not-yet-existing label file inside the test's temp dir.
func cachePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "labels.jsonl")
}

func openCache(t *testing.T, path string) *narrator.Cache {
	t.Helper()
	c, err := narrator.OpenCache(path)
	if err != nil {
		t.Fatalf("OpenCache(%s): %v", path, err)
	}
	if c == nil {
		t.Fatalf("OpenCache(%s) returned a nil cache and a nil error", path)
	}
	return c
}

func put(t *testing.T, c *narrator.Cache, key, label string) {
	t.Helper()
	if err := c.Put(key, label); err != nil {
		t.Fatalf("Put(%q, %q): %v", key, label, err)
	}
}

func assertGet(t *testing.T, c *narrator.Cache, key, want string) {
	t.Helper()
	got, ok := c.Get(key)
	if !ok {
		t.Fatalf("Get(%q) = _, false; want %q, true", key, want)
	}
	if got != want {
		t.Errorf("Get(%q) = %q, want %q", key, got, want)
	}
}

func assertMissing(t *testing.T, c *narrator.Cache, key string) {
	t.Helper()
	if got, ok := c.Get(key); ok {
		t.Errorf("Get(%q) = %q, true; want _, false", key, got)
	}
}

// ---------------------------------------------------------------- T47

// A machine that has never narrated anything has no label file. That is the
// normal first run, not an error.
func TestT47OpenCacheOnAMissingFileIsEmptyNotAnError(t *testing.T) {
	path := cachePath(t)
	c := openCache(t, path)
	assertMissing(t, c, "sess-alpha/1788091200000000000/build")

	// An empty cache is still a working one: the first Put creates the file.
	put(t, c, "sess-alpha/1788091200000000000/build", "maps the auth module")
	assertGet(t, c, "sess-alpha/1788091200000000000/build", "maps the auth module")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("after a Put the label file does not exist: %v", err)
	}
}

func TestT47PutGetRoundtrip(t *testing.T) {
	c := openCache(t, cachePath(t))
	put(t, c, "k1", "maps the auth module")
	put(t, c, "k2", "pins the token clock")

	assertGet(t, c, "k1", "maps the auth module")
	assertGet(t, c, "k2", "pins the token clock")
	assertMissing(t, c, "k3")
	assertMissing(t, c, "")
}

// The whole point of the file: a label narrated once is never paid for twice.
func TestT47ReopenReadsBackWhatWasPut(t *testing.T) {
	path := cachePath(t)
	first := openCache(t, path)
	put(t, first, "k1", "maps the auth module")
	put(t, first, "k2", "pins the token clock")

	second := openCache(t, path)
	assertGet(t, second, "k1", "maps the auth module")
	assertGet(t, second, "k2", "pins the token clock")
	assertMissing(t, second, "k3")
}

// Append-only means the same key is written twice; load resolves it by taking
// the last line, so a relabelled leg keeps its newest name across a restart.
func TestT47LastWriteWinsAfterReopen(t *testing.T) {
	path := cachePath(t)
	first := openCache(t, path)
	put(t, first, "k1", "first guess")
	put(t, first, "k1", "second guess")
	put(t, first, "k1", "final answer")

	// In memory, immediately.
	assertGet(t, first, "k1", "final answer")

	// And on disk, after a reload.
	assertGet(t, openCache(t, path), "k1", "final answer")
}

// The on-disk shape is part of the contract: append-only JSONL of {key,label}.
// Anything else and a second compass process (or a human with a text editor)
// cannot read the file.
func TestT47FileIsAppendOnlyJSONLOfKeyAndLabel(t *testing.T) {
	path := cachePath(t)
	c := openCache(t, path)
	put(t, c, "k1", "maps the auth module")
	put(t, c, "k2", "pins the token clock")
	put(t, c, "k1", "final answer")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := nonEmptyLines(string(raw))
	if len(lines) != 3 {
		t.Fatalf("file has %d lines, want 3 (append-only: the k1 rewrite is a new line, not an edit)\n%s", len(lines), raw)
	}
	var got []narrator.Label
	for i, line := range lines {
		var rec struct {
			Key   string `json:"key"`
			Label string `json:"label"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("line %d is not JSON: %v\n%s", i+1, err, line)
		}
		got = append(got, narrator.Label{Key: rec.Key, Text: rec.Label})
	}
	assertLabels(t, got,
		narrator.Label{Key: "k1", Text: "maps the auth module"},
		narrator.Label{Key: "k2", Text: "pins the token clock"},
		narrator.Label{Key: "k1", Text: "final answer"},
	)
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

// A half-written line (the process died mid-append, or something else wrote to
// the file) costs that one label and nothing else — the lines after it still
// load. The good lines are produced by Put itself so the test does not hardcode
// the record shape twice.
func TestT47MalformedLineIsSkippedAndLaterLinesStillLoad(t *testing.T) {
	path := cachePath(t)
	seed := openCache(t, path)
	put(t, seed, "k1", "maps the auth module")
	put(t, seed, "k2", "pins the token clock")
	put(t, seed, "k3", "unbreaks the tailer")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	good := nonEmptyLines(string(raw))
	if len(good) != 3 {
		t.Fatalf("seeded %d lines, want 3:\n%s", len(good), raw)
	}

	// Splice garbage of several flavours between the good lines.
	spliced := strings.Join([]string{
		good[0],
		`{"key":"k9","label":"truncated mid-`, // torn write
		good[1],
		"not json at all",
		"",
		"   ",
		`[1,2,3]`, // valid JSON, wrong shape
		good[2],
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(spliced), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	c := openCache(t, path)
	assertGet(t, c, "k1", "maps the auth module")
	assertGet(t, c, "k2", "pins the token clock")
	assertGet(t, c, "k3", "unbreaks the tailer")
	assertMissing(t, c, "k9")

	// And the reopened cache is still writable: a damaged file is not a
	// read-only file.
	put(t, c, "k4", "after the damage")
	assertGet(t, openCache(t, path), "k4", "after the damage")
}

// A label with a newline in it would end the JSONL record early and eat the
// next one. Whatever the model returns, the file has to survive it.
func TestT47AwkwardLabelsSurviveARoundtrip(t *testing.T) {
	path := cachePath(t)
	awkward := map[string]string{
		"k-newline": "two\nlines",
		"k-quote":   `he said "no"`,
		"k-tab":     "a\tb",
		"k-unicode": "réécrit le · chemin",
		"k-brace":   `{"key":"injected","label":"nope"}`,
	}
	first := openCache(t, path)
	for k, v := range awkward {
		put(t, first, k, v)
	}
	second := openCache(t, path)
	for k, v := range awkward {
		got, ok := second.Get(k)
		if !ok {
			t.Errorf("Get(%q) after reopen = _, false; want %q", k, v)
			continue
		}
		if got != v {
			t.Errorf("Get(%q) after reopen = %q, want %q", k, got, v)
		}
	}
	// The injected record must not have become a real one.
	assertMissing(t, second, "injected")
}

// A key nobody ever put stays absent even on a populated cache — Get must not
// invent a zero value.
func TestT47GetOnAnAbsentKeyIsFalse(t *testing.T) {
	c := openCache(t, cachePath(t))
	put(t, c, "k1", "maps the auth module")
	assertMissing(t, c, "k2")
	assertMissing(t, c, "k1 ")
	assertMissing(t, c, "K1")
}

// The Narrator writes from its batch goroutine while the UI reads from the
// render loop. Run under -race: 8 writers, 8 readers, one cache.
func TestT47ConcurrentPutsAreRaceFree(t *testing.T) {
	const (
		writers = 8
		perGoro = 25
	)
	path := cachePath(t)
	c := openCache(t, path)

	key := func(w, i int) string { return fmt.Sprintf("sess-%d/17880912000000000%02d/build", w, i) }
	label := func(w, i int) string { return fmt.Sprintf("label %d-%d", w, i) }

	var puts, gets sync.WaitGroup
	errs := make(chan error, writers*perGoro)
	stop := make(chan struct{})

	for w := 0; w < writers; w++ {
		puts.Add(1)
		go func(w int) {
			defer puts.Done()
			for i := 0; i < perGoro; i++ {
				if err := c.Put(key(w, i), label(w, i)); err != nil {
					errs <- fmt.Errorf("Put(%q): %w", key(w, i), err)
					return
				}
			}
		}(w)
	}
	// Concurrent readers, so Get racing Put is covered too. They spin until
	// the writers are done rather than for a fixed time.
	for r := 0; r < writers; r++ {
		gets.Add(1)
		go func(r int) {
			defer gets.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				c.Get(key(r, r))
			}
		}(r)
	}

	puts.Wait()
	close(stop)
	gets.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Put failed: %v", err)
	}

	// Every write is visible in memory…
	for w := 0; w < writers; w++ {
		for i := 0; i < perGoro; i++ {
			assertGet(t, c, key(w, i), label(w, i))
		}
	}
	// …and every write made it to disk intact: interleaved appends must not
	// have shredded each other's lines.
	reopened := openCache(t, path)
	for w := 0; w < writers; w++ {
		for i := 0; i < perGoro; i++ {
			assertGet(t, reopened, key(w, i), label(w, i))
		}
	}
}
