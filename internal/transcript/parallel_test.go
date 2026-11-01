package transcript

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// A read large enough to be decoded side by side gives exactly what one pass
// gives: the same events in the same order, and the same count of malformed
// lines — whatever the blocks it was cut into.
func TestALargeReadDecodesInOrderAcrossWorkers(t *testing.T) {
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(4))
	pad := strings.Repeat("y", 3000)
	var b strings.Builder
	for i := 0; i < 2000; i++ {
		switch i % 50 {
		case 7:
			b.WriteString("{not json\n")
		case 8:
			b.WriteString("\n")
		case 9:
			fmt.Fprintf(&b, `{"type":"user","uuid":"u%d","timestamp":"2026-08-30T09:00:00.000Z","message":{"role":"user","content":"crlf %d"}}`+"\r\n", i, i)
		default:
			fmt.Fprintf(&b, `{"type":"assistant","uuid":"a%d","timestamp":"2026-08-30T09:00:00.000Z","message":{"id":"m%d","role":"assistant","content":[{"type":"text","text":"%d %s"}]}}`+"\n", i, i, i, pad)
		}
	}
	if b.Len() < 2*parallelMin {
		t.Fatalf("fixture is %d bytes; it must be large enough to take the parallel path", b.Len())
	}
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	tl := NewTailer(path)
	got, err := tl.Poll()
	if err != nil {
		t.Fatal(err)
	}
	var lines [][]byte
	for _, l := range strings.SplitAfter(b.String(), "\n") {
		if l != "" {
			lines = append(lines, []byte(strings.TrimSuffix(l, "\n")))
		}
	}
	want, skipped := parseBlock(lines)
	if len(got) != len(want) || !reflect.DeepEqual(got, want) {
		t.Fatalf("parallel decode gave %d events, one pass %d, or a different order", len(got), len(want))
	}
	if tl.Skipped() != skipped || skipped != 40 {
		t.Errorf("malformed lines: parallel %d, one pass %d, want 40", tl.Skipped(), skipped)
	}
	if got[0].UUID != "a0" || got[len(got)-1].UUID != "a1999" {
		t.Errorf("first and last events are %q and %q", got[0].UUID, got[len(got)-1].UUID)
	}
}
