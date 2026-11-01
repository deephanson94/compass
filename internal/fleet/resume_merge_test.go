package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/deephanson94/compass/internal/transcript"
)

func point(offset int64, name string) ResumePoint {
	return ResumePoint{Mark: transcript.Mark{Offset: offset}, Entry: EntryFold{Name: name}}
}

func cachePath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "resume.json")
}

// Two compasses of one person: what one saved is still there after the
// other saves. A deck left running for days rewrote its own old copy over
// the file and took every name a newer compass had saved since.
func TestASaveKeepsWhatAnotherCompassSaved(t *testing.T) {
	path := cachePath(t)
	stale := OpenResumeCache(path) // opened first: knows nothing of rag
	fresh := OpenResumeCache(path)
	fresh.record("rag", point(5000, "rag"))
	fresh.Save()

	stale.record("other", point(100, ""))
	stale.Save()

	back := OpenResumeCache(path)
	if p, ok := back.point("rag"); !ok || p.Entry.Name != "rag" {
		t.Errorf("the other compass's session after a stale save: %+v, %v", p, ok)
	}
	if _, ok := back.point("other"); !ok {
		t.Error("the saving compass's own session is missing")
	}
}

// Both have the session: the copy that has read further wins, whichever
// saves last.
func TestTheFurtherReadCopyWins(t *testing.T) {
	path := cachePath(t)
	ahead, behind := OpenResumeCache(path), OpenResumeCache(path)
	ahead.record("rag", point(9000, "rag"))
	ahead.Save()
	behind.record("rag", point(1000, ""))
	behind.Save()
	if p, _ := OpenResumeCache(path).point("rag"); p.Mark.Offset != 9000 || p.Entry.Name != "rag" {
		t.Errorf("a copy that had read less overwrote one that had read more: %+v", p)
	}

	behind.record("rag", point(12000, "rag2")) // it reads on, past the other
	behind.Save()
	if p, _ := OpenResumeCache(path).point("rag"); p.Mark.Offset != 12000 || p.Entry.Name != "rag2" {
		t.Errorf("the further copy did not win: %+v", p)
	}
}

// A field this build does not know (a newer build's) survives a save; a
// field it knows and leaves out (zero, omitempty) is not brought back.
func TestASaveKeepsANewerBuildsFields(t *testing.T) {
	path := cachePath(t)
	newer := `{"points":{"rag":{"mark":{"offset":100},"fold":{},"entry":{"name":"rag","has_class":true,"future_field":7},"future_point":"x"}},"future_section":{"a":1}}`
	if err := os.WriteFile(path, []byte(newer), 0o644); err != nil {
		t.Fatal(err)
	}
	c := OpenResumeCache(path)
	p, _ := c.point("rag")
	p.Mark.Offset = 200
	p.Entry.HasClass = false // cleared on purpose: omitted when saved
	c.record("rag", p)
	c.Save()

	raw, _ := os.ReadFile(path)
	var got struct {
		Points map[string]map[string]json.RawMessage `json:"points"`
	}
	_ = json.Unmarshal(raw, &got)
	rag := got.Points["rag"]
	var entry map[string]json.RawMessage
	_ = json.Unmarshal(rag["entry"], &entry)
	for _, want := range []string{`"future_section"`, `"future_point":"x"`, `"future_field":7`, `"offset":200`, `"name":"rag"`} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("after the save the file has no %s:\n%s", want, raw)
		}
	}
	if _, ok := entry["has_class"]; ok {
		t.Errorf("a field this build knows and cleared came back from the file: %s", rag["entry"])
	}
}

// A session this compass dropped on purpose (its transcript is gone) is
// not taken back from the file.
func TestASaveDoesNotBringBackWhatItDropped(t *testing.T) {
	path := cachePath(t)
	c := OpenResumeCache(path)
	c.record("gone", point(10, ""))
	c.record("kept", point(10, ""))
	c.Save()
	c.retain(map[string]bool{"kept": true})
	c.Save()
	back := OpenResumeCache(path)
	if _, ok := back.point("gone"); ok {
		t.Error("a dropped session came back from the file")
	}
	if _, ok := back.point("kept"); !ok {
		t.Error("the kept session is missing")
	}
}

// Saves side by side lose nothing: the read, the merge and the rename are
// one step under the lock.
func TestSavesSideBySideLoseNothing(t *testing.T) {
	path := cachePath(t)
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			c := OpenResumeCache(path)
			for i := 0; i < 25; i++ {
				c.record(string(rune('a'+w))+string(rune('a'+i)), point(int64(i), ""))
				c.Save()
			}
		}(w)
	}
	wg.Wait()
	back := OpenResumeCache(path)
	if n := len(back.points); n != 100 {
		t.Errorf("four compasses saved 100 sessions between them; the file holds %d", n)
	}
}
