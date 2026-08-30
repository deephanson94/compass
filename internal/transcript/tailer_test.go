package transcript_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deephanson94/compass/internal/transcript"
)

// appendBytes appends a raw chunk (which may end mid-line) to path, creating it
// if needed — exactly what a live Claude Code session does to its transcript.
func appendBytes(t *testing.T, path, chunk string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("open %s for append: %v", path, err)
	}
	if _, err := f.WriteString(chunk); err != nil {
		f.Close()
		t.Fatalf("append to %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}

func poll(t *testing.T, tl *transcript.Tailer) []transcript.Event {
	t.Helper()
	evs, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	return evs
}

func uuidsOf(evs []transcript.Event) []string {
	out := make([]string, 0, len(evs))
	for _, ev := range evs {
		out = append(out, ev.UUID)
	}
	return out
}

func uuidsOfLines(t *testing.T, lines []string) []string {
	t.Helper()
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, mustParse(t, l).UUID)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// T8 — a transcript arriving in three chunks, two of which cut a JSON line in
// half. Every event must arrive exactly once, in file order.
func TestT08TailerHoldsPartialLineAcrossChunks(t *testing.T) {
	lines := fixtureLines(t, "scenarios/t01-working-bash.jsonl")
	if len(lines) != 4 {
		t.Fatalf("fixture t01 has %d lines, want 4", len(lines))
	}
	blob := strings.Join(lines, "")

	// Cut inside line 2 and inside line 4.
	cut1 := len(lines[0]) + len(lines[1])/2
	cut2 := len(lines[0]) + len(lines[1]) + len(lines[2]) + len(lines[3])/2

	path := filepath.Join(t.TempDir(), "session.jsonl")
	tl := transcript.NewTailer(path)

	var got []transcript.Event

	appendBytes(t, path, blob[:cut1])
	batch := poll(t, tl)
	if len(batch) != 1 {
		t.Fatalf("after chunk 1 (one whole line + half of the next): got %d events, want 1", len(batch))
	}
	got = append(got, batch...)

	appendBytes(t, path, blob[cut1:cut2])
	batch = poll(t, tl)
	if len(batch) != 2 {
		t.Fatalf("after chunk 2 (rest of line 2, line 3, half of line 4): got %d events, want 2", len(batch))
	}
	got = append(got, batch...)

	appendBytes(t, path, blob[cut2:])
	batch = poll(t, tl)
	if len(batch) != 1 {
		t.Fatalf("after chunk 3 (rest of line 4): got %d events, want 1", len(batch))
	}
	got = append(got, batch...)

	if batch := poll(t, tl); len(batch) != 0 {
		t.Errorf("poll with nothing new returned %d events, want 0", len(batch))
	}

	want := uuidsOfLines(t, lines)
	if gotUUIDs := uuidsOf(got); !equalStrings(gotUUIDs, want) {
		t.Errorf("events delivered out of order, lost or duplicated:\n got %v\nwant %v", gotUUIDs, want)
	}
	if tl.Skipped() != 0 {
		t.Errorf("Skipped() = %d, want 0 — a split line is held, not skipped", tl.Skipped())
	}
}

// T9 — a malformed line between valid ones is dropped and counted, and never
// aborts the batch.
func TestT09TailerSkipsMalformedLines(t *testing.T) {
	lines := fixtureLines(t, "scenarios/t01-working-bash.jsonl")
	path := filepath.Join(t.TempDir(), "session.jsonl")
	tl := transcript.NewTailer(path)

	appendBytes(t, path, lines[0]+"{\"type\":\"user\",  truncated garbage\n"+lines[1])
	evs := poll(t, tl)

	if len(evs) != 2 {
		t.Fatalf("got %d events, want 2 (the two valid lines around the garbage)", len(evs))
	}
	want := uuidsOfLines(t, []string{lines[0], lines[1]})
	if got := uuidsOf(evs); !equalStrings(got, want) {
		t.Errorf("events = %v, want %v", got, want)
	}
	if tl.Skipped() != 1 {
		t.Errorf("Skipped() = %d, want 1", tl.Skipped())
	}

	// Skipped() accumulates across polls.
	appendBytes(t, path, "}\n"+lines[2])
	evs = poll(t, tl)
	if len(evs) != 1 {
		t.Fatalf("got %d events after the second batch, want 1", len(evs))
	}
	if tl.Skipped() != 2 {
		t.Errorf("Skipped() = %d after a second malformed line, want 2 (cumulative)", tl.Skipped())
	}
}

// T10 — the file shrinks below the stored offset (truncate/rotate): the tailer
// resets to 0 and re-reads from the start rather than returning nothing forever.
func TestT10TailerResetsOnTruncation(t *testing.T) {
	lines := fixtureLines(t, "scenarios/t01-working-bash.jsonl")
	path := filepath.Join(t.TempDir(), "session.jsonl")
	tl := transcript.NewTailer(path)

	appendBytes(t, path, strings.Join(lines, ""))
	if evs := poll(t, tl); len(evs) != 4 {
		t.Fatalf("first poll returned %d events, want 4", len(evs))
	}

	// Rewrite the file shorter than the offset we already consumed.
	if err := os.WriteFile(path, []byte(lines[0]), 0o644); err != nil {
		t.Fatalf("truncate-rewrite %s: %v", path, err)
	}

	evs := poll(t, tl)
	if len(evs) != 1 {
		t.Fatalf("after truncation got %d events, want 1 (offset reset, file re-read)", len(evs))
	}
	if want := uuidsOfLines(t, lines[:1]); !equalStrings(uuidsOf(evs), want) {
		t.Errorf("after truncation got %v, want %v", uuidsOf(evs), want)
	}

	// And tailing continues normally from the new offset.
	appendBytes(t, path, lines[1])
	evs = poll(t, tl)
	if len(evs) != 1 {
		t.Fatalf("after post-truncation append got %d events, want 1", len(evs))
	}
	if want := uuidsOfLines(t, lines[1:2]); !equalStrings(uuidsOf(evs), want) {
		t.Errorf("after post-truncation append got %v, want %v", uuidsOf(evs), want)
	}
}

func TestTailerMissingFileIsNotAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never-flushed.jsonl")
	tl := transcript.NewTailer(path)

	evs, err := tl.Poll()
	if err != nil {
		t.Fatalf("Poll on a missing file returned error %v, want nil", err)
	}
	if len(evs) != 0 {
		t.Fatalf("Poll on a missing file returned %d events, want 0", len(evs))
	}

	// The same tailer must pick the file up once the session flushes.
	lines := fixtureLines(t, "scenarios/t06-lone-prompt.jsonl")
	appendBytes(t, path, lines[0])
	if evs := poll(t, tl); len(evs) != 1 {
		t.Errorf("after the file appeared got %d events, want 1", len(evs))
	}
}

func TestTailerEmptyFileYieldsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	tl := transcript.NewTailer(path)

	evs := poll(t, tl)
	if len(evs) != 0 {
		t.Errorf("Poll on an empty file returned %d events, want 0", len(evs))
	}
	if tl.Skipped() != 0 {
		t.Errorf("Skipped() = %d on an empty file, want 0", tl.Skipped())
	}
}

func TestTailerDeliversUnterminatedLineOnlyAfterItsNewline(t *testing.T) {
	lines := fixtureLines(t, "scenarios/t06-lone-prompt.jsonl")
	body := strings.TrimSuffix(lines[0], "\n")

	path := filepath.Join(t.TempDir(), "session.jsonl")
	tl := transcript.NewTailer(path)

	appendBytes(t, path, body) // complete JSON, but no newline yet
	if evs := poll(t, tl); len(evs) != 0 {
		t.Fatalf("got %d events for a line with no trailing newline, want 0", len(evs))
	}

	appendBytes(t, path, "\n")
	if evs := poll(t, tl); len(evs) != 1 {
		t.Fatalf("got %d events once the newline arrived, want 1", len(evs))
	}
}
