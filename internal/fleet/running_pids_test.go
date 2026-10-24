package fleet

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// RunningPIDs reads Claude Code's registry as pid → session id, leaving out
// entries whose process is gone; an unreadable registry is nil.
func TestRunningPIDs(t *testing.T) {
	root := t.TempDir()
	if RunningPIDs(root) != nil {
		t.Fatal("no registry directory read as an empty registry, not as unknown")
	}
	dir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	me := os.Getpid()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(strconv.Itoa(me)+".json", `{"pid":`+strconv.Itoa(me)+`,"sessionId":"alive"}`)
	write("999999.json", `{"pid":999999,"sessionId":"gone"}`)
	write("peer.key", "not json")
	got := RunningPIDs(root)
	if len(got) != 1 || got[me] != "alive" {
		t.Fatalf("got %v, want only this process as alive", got)
	}
}
