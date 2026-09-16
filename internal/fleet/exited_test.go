package fleet_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
)

// procStartOf reads this process's own start time out of /proc, in the same
// clock ticks the registry records. Where it cannot be read the entries
// below carry no start and the pid alone decides, which is the fallback
// this package documents.
func procStartOf(t *testing.T, pid int) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("/proc", fmt.Sprint(pid), "stat"))
	if err != nil {
		return ""
	}
	s := string(raw)
	i := len(s) - 1
	for i >= 0 && s[i] != ')' {
		i--
	}
	var fields []string
	for _, f := range splitFields(s[i+1:]) {
		fields = append(fields, f)
	}
	if len(fields) < 20 {
		return ""
	}
	return fields[19]
}

func splitFields(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// writeRegistry puts one `<pid>.json` under <root>/sessions, the way Claude
// Code's own bookkeeping does.
func writeRegistry(t *testing.T, root string, pid int, sessionID, procStart string) {
	t.Helper()
	dir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"pid":%d,"sessionId":%q,"cwd":"/home/user/alpha","procStart":%q,"kind":"interactive","status":"idle"}`,
		pid, sessionID, procStart)
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.json", pid)), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestAQuestionYouQuitIsNotAQuestionYouWalkedAwayFrom is the door's fourth
// rule (#374). The question is read off the end of the transcript, and
// `/exit` writes nothing there, so a session you quit while it was asking
// kept its row forever and could only be put down by hand. Claude Code
// keeps a registry of its running sessions; a question in a session with no
// process is a question you already answered by leaving.
func TestAQuestionYouQuitIsNotAQuestionYouWalkedAwayFrom(t *testing.T) {
	mine := os.Getpid()

	for _, c := range []struct {
		name  string
		entry func(t *testing.T, root string)
		want  bool
	}{
		{"the session is still running", func(t *testing.T, root string) {
			writeRegistry(t, root, mine, idWaitingDay, procStartOf(t, mine))
		}, true},
		{"the registry is there and this session is not in it", func(t *testing.T, root string) {
			writeRegistry(t, root, mine, "some-other-session", procStartOf(t, mine))
		}, false},
		{"the entry outlived its process", func(t *testing.T, root string) {
			writeRegistry(t, root, deadPID(t), idWaitingDay, "")
		}, false},
		{"the pid was handed to somebody else", func(t *testing.T, root string) {
			writeRegistry(t, root, mine, idWaitingDay, "1")
		}, false},
		{"there is no registry to read", func(t *testing.T, root string) {}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			at, _ := askedQuestionAt(t, root, slugAlpha, idWaitingDay, 26*time.Hour)
			c.entry(t, root)

			s := pick(t, mustRefresh(t, fleet.NewManager(root), fleetNow), idWaitingDay)
			if s.Waiting != c.want || s.Live != c.want {
				t.Errorf("Live/Waiting = %v/%v, want %v — the transcript is the same in every one of these",
					s.Live, s.Waiting, c.want)
			}
			if c.want {
				assertWaiting(t, s, at)
			}
		})
	}
}

// deadPID finds a pid with no process behind it.
func deadPID(t *testing.T) int {
	t.Helper()
	for pid := 4194300; pid > 4000000; pid-- {
		if _, err := os.Stat(filepath.Join("/proc", fmt.Sprint(pid))); os.IsNotExist(err) {
			return pid
		}
	}
	t.Skip("no free pid to test with")
	return 0
}
