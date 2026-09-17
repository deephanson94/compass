package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// The session registry (#374).
//
// A question nobody has answered keeps its session live however long ago it
// asked — that is the ask door, and it reads the end of the transcript and
// nothing else. It cannot tell a question you walked away from, which is
// what it is for, from one you answered by quitting: `/exit` writes nothing
// to the transcript, so the model's question stays the last word in the
// file forever and the row stays on the board forever.
//
// Claude Code keeps a registry of its running sessions at
// `<root>/sessions/<pid>.json`, one file per live process, carrying the
// session's uuid and the pid's own start time. A session with no entry
// whose process is alive is not running — you quit it, or the machine
// restarted under it — and the door closes.
//
// It is read fail-soft, and that matters more here than usual: this file
// is Claude Code's own bookkeeping and nothing documents it. Where the
// directory is missing or unreadable — an older Claude Code, a session
// tree copied from another machine — the answer is "I do not know", and
// the door behaves as it did before this existed. Only a registry we can
// actually read is allowed to close it.

// runningSession is the part of a registry entry this reads.
type runningSession struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	ProcStart string `json:"procStart"`
}

// readRunning returns the ids of the sessions whose process is still alive,
// or nil where the registry cannot be read at all — which is the difference
// between "none are running" and "I cannot say", and only the first of
// those is allowed to archive anything.
func readRunning(root string) map[string]bool {
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(root, "sessions"))
	if err != nil {
		return nil
	}
	live := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue // the peer key files sit here too
		}
		raw, err := os.ReadFile(filepath.Join(root, "sessions", e.Name()))
		if err != nil {
			continue
		}
		var s runningSession
		if json.Unmarshal(raw, &s) != nil || s.SessionID == "" || s.PID <= 0 {
			continue
		}
		if !processAlive(s.PID, s.ProcStart) {
			continue // the entry outlived the process that wrote it
		}
		live[s.SessionID] = true
	}
	return live
}
