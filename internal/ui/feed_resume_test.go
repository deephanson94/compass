package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/transcript"
)

// A board column with a journey in the resume cache is restored from it,
// not replayed: a fold the cache carries that no replay could produce is
// what the column draws, and only the bytes past its mark are read.
func TestAColumnResumesItsJourneyFromTheCache(t *testing.T) {
	path := writeTranscript(t)
	size, _ := os.Stat(path)
	cache := fleet.OpenResumeCache(filepath.Join(t.TempDir(), "resume.json"))
	planted := journey.RestoreSegmenter(journey.Fold{Prompts: []journey.Prompt{{Text: "from the cache"}}}).Fold()
	cache.RecordJourney("k", fleet.JourneyPoint{Mark: transcript.Mark{Offset: size.Size()}, Fold: planted})

	fs := newFeedStore()
	fs.resume = cache
	tr, _ := fs.poll("k", path, false)
	if len(tr.Prompts) != 1 || tr.Prompts[0].Text != "from the cache" {
		t.Fatalf("the column replayed the file rather than resuming: prompts %+v", tr.Prompts)
	}

	// The reader's feed never resumes: its document is the events, which
	// no fold carries.
	_, events := fs.poll("k", path, true)
	if len(events) != 2 {
		t.Errorf("the reader got %d events, want the whole transcript (2)", len(events))
	}
}

// What a column records is what the next process resumes from: a column
// polled, saved and restored draws the trail a full replay draws, with the
// lines appended since folded in.
func TestARecordedJourneyRoundTripsThroughTheCacheFile(t *testing.T) {
	path := writeTranscript(t)
	cachePath := filepath.Join(t.TempDir(), "resume.json")
	cache := fleet.OpenResumeCache(cachePath)
	fs := newFeedStore()
	fs.resume = cache
	fs.poll("k", path, false)
	cache.Save()

	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString(`{"type":"user","uuid":"u2","timestamp":"2026-08-30T09:01:00.000Z","message":{"role":"user","content":"and this"},"sessionId":"s"}` + "\n")
	f.Close()

	next := newFeedStore()
	next.resume = fleet.OpenResumeCache(cachePath)
	got, _ := next.poll("k", path, false)
	want, _ := newFeedStore().poll("k", path, false)
	if len(got.Prompts) != 2 || len(got.Prompts) != len(want.Prompts) || got.Prompts[1].Text != want.Prompts[1].Text {
		t.Fatalf("resumed trail %+v, replayed trail %+v", got.Prompts, want.Prompts)
	}
	if next.feeds["k"].tailer.Mark().Offset != fsSize(t, path) {
		t.Errorf("the resumed tailer is at %d, the file is %d bytes", next.feeds["k"].tailer.Mark().Offset, fsSize(t, path))
	}
}

func fsSize(t *testing.T, path string) int64 {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Size()
}
