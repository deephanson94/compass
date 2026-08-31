package fleet_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/state"
)

// resumeRoot writes a transcript in two halves so a reader can be stopped
// between them. The FIRST half decides what the saved fold has to carry — a
// split on a finished turn carries nothing interesting, and a test built that
// way passes against a fold that drops half its fields.
func resumeRoot(t *testing.T, first, second func(*transcriptBuilder)) (root string, rest func()) {
	t.Helper()
	root = t.TempDir()
	const sess = "77777777-7777-4777-8777-777777777777"

	b := newTranscript(t, sess, "/home/user/app", "main").
		prompt(ago(20*time.Minute), "fix the 401 bug").
		text(ago(19*time.Minute), "reading the middleware")
	first(b)
	b.write(root, "-home-user-app")

	return root, func() {
		t.Helper()
		tail := newTranscript(t, sess, "/home/user/app", "main")
		second(tail)
		path := filepath.Join(root, "projects", "-home-user-app", sess+".jsonl")
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			t.Fatalf("append: %v", err)
		}
		defer f.Close()
		for _, line := range tail.lines {
			if _, err := f.WriteString(line); err != nil {
				t.Fatalf("append: %v", err)
			}
		}
	}
}

func snapOf(t *testing.T, m *fleet.Manager) state.Snapshot {
	t.Helper()
	sessions, err := m.Refresh(fleetNow)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("want one session, got %d", len(sessions))
	}
	return sessions[0].Snap
}

// The whole point of the cache: a process that resumes must reach exactly the
// verdict a process that replayed the file from byte zero would. Anything less
// and the status line is fast and wrong, which is worse than slow.
func TestResumedStateMatchesFullReplay(t *testing.T) {
	for _, tc := range []struct {
		name          string
		first, second func(*transcriptBuilder)
	}{
		// The split lands mid-turn, so the fold has to carry the pending call
		// across: without it the resumed machine sees no tool in flight.
		{"stopped with a tool call in flight",
			func(b *transcriptBuilder) {
				b.tool(ago(2*time.Minute), "t1", "Bash", map[string]any{"command": "pytest tests/auth -x"})
			},
			func(b *transcriptBuilder) {}},

		// The result for a call the resumed machine only knows about through
		// the fold.
		{"the call it was waiting on comes back",
			func(b *transcriptBuilder) {
				b.tool(ago(6*time.Minute), "t2", "Bash", map[string]any{"command": "go test ./..."})
			},
			func(b *transcriptBuilder) {
				b.result(ago(5*time.Minute), "t2", "ok")
			}},

		// Results are back and the model has not spoken: awaitingModel is true
		// at the split and must survive it, or the session reads idle.
		{"stopped between a result and the model",
			func(b *transcriptBuilder) {
				b.tool(ago(4*time.Minute), "t3", "Bash", map[string]any{"command": "go build ./..."})
				b.result(ago(3*time.Minute), "t3", "ok")
				// Bookkeeping after the result, so the moment the wait began is
				// no longer simply the last thing in the file.
				b.latch(ago(100 * time.Second))
			},
			func(b *transcriptBuilder) {}},

		{"a question held open across the split",
			func(b *transcriptBuilder) {
				b.tool(ago(time.Minute), "t4", "AskUserQuestion", map[string]any{"question": "which one?"})
			},
			func(b *transcriptBuilder) {}},

		{"stuck: the call has been in flight far too long",
			func(b *transcriptBuilder) {
				b.tool(ago(40*time.Minute), "t5", "Bash", map[string]any{"command": "sleep 9999"})
			},
			func(b *transcriptBuilder) {}},

		// Two calls in flight at once: the oldest decides the state, the newest
		// decides what the fleet says the session is doing. Both have to cross
		// the split, and so does the sequence that tells them apart.
		{"two calls in flight across the split",
			func(b *transcriptBuilder) {
				b.tool(ago(8*time.Minute), "t7", "Read", map[string]any{"file_path": "/home/user/app/mw.py"})
				b.tool(ago(7*time.Minute), "t8", "Bash", map[string]any{"command": "pytest tests/auth -x"})
			},
			func(b *transcriptBuilder) {}},

		// A finished turn before the split, with bookkeeping lines after it that
		// carry a clock but say nothing. The verdict dates from when the model
		// last *spoke*, which is no longer the last thing in the file — so the
		// moment has to cross the split on its own.
		{"the turn was already over at the split",
			func(b *transcriptBuilder) {
				b.text(ago(9*time.Minute), "that is the bug, and it is fixed")
				b.latch(ago(90 * time.Second))
			},
			func(b *transcriptBuilder) {}},

		// Three calls, the first already answered: the two still pending were
		// numbered before the split, and a call arriving after it must be
		// numbered behind them, not in front.
		{"a new call must not outrank the ones already in flight",
			func(b *transcriptBuilder) {
				b.tool(ago(12*time.Minute), "t9", "Read", map[string]any{"file_path": "/a.py"})
				b.tool(ago(11*time.Minute), "t10", "Read", map[string]any{"file_path": "/b.py"})
				b.tool(ago(10*time.Minute), "t11", "Read", map[string]any{"file_path": "/c.py"})
				b.result(ago(9*time.Minute), "t9", "read")
			},
			func(b *transcriptBuilder) {
				b.tool(ago(time.Minute), "t12", "Bash", map[string]any{"command": "pytest -x"})
			}},

		{"the turn finishes after the split",
			func(b *transcriptBuilder) {
				b.tool(ago(5*time.Minute), "t6", "Bash", map[string]any{"command": "go vet ./..."})
			},
			func(b *transcriptBuilder) {
				b.result(ago(4*time.Minute), "t6", "ok")
				b.text(ago(3*time.Minute), "all done — the suite is green")
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, rest := resumeRoot(t, tc.first, tc.second)
			cachePath := filepath.Join(t.TempDir(), "resume.json")

			// A first process reads the head of the file and writes down both
			// where it stopped and what it had concluded there.
			warm := liveManager(root)
			c := fleet.OpenResumeCache(cachePath)
			warm.UseResumeCache(c)
			snapOf(t, warm)
			c.Save()

			rest()

			resumed := liveManager(root)
			resumed.UseResumeCache(fleet.OpenResumeCache(cachePath))
			got := snapOf(t, resumed)

			want := snapOf(t, liveManager(root)) // no cache: the whole file
			if got != want {
				t.Errorf("resumed verdict\n  %+v\nfull replay\n  %+v", got, want)
			}
		})
	}
}

// A transcript that shrank is not the file the mark was taken in. Resuming
// into the middle of it would report a state that never existed, so the mark
// is refused and the file is read from the start.
func TestResumeRefusesATruncatedTranscript(t *testing.T) {
	root, _ := resumeRoot(t, midTurn, func(*transcriptBuilder) {})
	cachePath := filepath.Join(t.TempDir(), "resume.json")

	warm := liveManager(root)
	c := fleet.OpenResumeCache(cachePath)
	warm.UseResumeCache(c)
	snapOf(t, warm)
	c.Save()

	// The session is restarted and its transcript replaced with a shorter one.
	path := filepath.Join(root, "projects", "-home-user-app",
		"77777777-7777-4777-8777-777777777777.jsonl")
	fresh := newTranscript(t, "77777777-7777-4777-8777-777777777777", "/home/user/app", "main").
		prompt(ago(time.Minute), "start over")
	if err := os.WriteFile(path, []byte(fresh.lines[0]), 0o644); err != nil {
		t.Fatal(err)
	}

	resumed := liveManager(root)
	resumed.UseResumeCache(fleet.OpenResumeCache(cachePath))
	got := snapOf(t, resumed)
	want := snapOf(t, liveManager(root))
	if got != want {
		t.Errorf("a truncated transcript resumed anyway:\n  got  %+v\n  want %+v", got, want)
	}
	if got.Activity == "idle" && !got.Since.IsZero() {
		return
	}
}

// A cache is a cache: garbage in it costs time, never correctness.
func TestUnreadableResumeCacheIsIgnored(t *testing.T) {
	root, _ := resumeRoot(t, midTurn, func(*transcriptBuilder) {})
	dir := t.TempDir()

	for _, tc := range []struct{ name, body string }{
		{"not json", "}{ this is not json at all"},
		{"empty", ""},
		{"json, wrong shape", `{"points":"a string where a map belongs"}`},
		{"a mark past the end", `{"points":{"` +
			filepath.Join(root, "projects", "-home-user-app",
				"77777777-7777-4777-8777-777777777777.jsonl") +
			`":{"mark":{"offset":999999999},"fold":{"saw_substantive":true}}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".json")
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			m := liveManager(root)
			m.UseResumeCache(fleet.OpenResumeCache(path))
			got := snapOf(t, m)
			if want := snapOf(t, liveManager(root)); got != want {
				t.Errorf("a bad cache changed the verdict:\n  got  %+v\n  want %+v", got, want)
			}
		})
	}
}

// The discovery half of the cache is keyed on (size, mtime). A transcript that
// changed must be read again, not served from a scan taken before it moved.
func TestChangedTranscriptIsNotServedFromCache(t *testing.T) {
	root, rest := resumeRoot(t, midTurn, func(b *transcriptBuilder) {
		b.moveTo("/home/user/moved", "feature-y").
			prompt(ago(time.Minute), "over here now")
	})
	cachePath := filepath.Join(t.TempDir(), "resume.json")

	warm := liveManager(root)
	c := fleet.OpenResumeCache(cachePath)
	warm.UseResumeCache(c)
	snapOf(t, warm)
	c.Save()

	rest()

	m := liveManager(root)
	m.UseResumeCache(fleet.OpenResumeCache(cachePath))
	sessions, err := m.Refresh(fleetNow)
	if err != nil {
		t.Fatal(err)
	}
	if got := sessions[0].Info.CWD; got != "/home/user/moved" {
		t.Errorf("cwd is %q — the cache served a scan from before the session moved", got)
	}
	if got := sessions[0].Info.GitBranch; got != "feature-y" {
		t.Errorf("branch is %q, want feature-y", got)
	}
}

// midTurn leaves the first half of a transcript with a tool call in flight, so
// the saved fold has something in it that matters.
func midTurn(b *transcriptBuilder) {
	b.tool(ago(2*time.Minute), "t0", "Bash", map[string]any{"command": "pytest -x"})
}

// The discovery half of the cache has to actually spare the read, or it is a
// file that costs disk and buys nothing. Proving that directly means proving a
// negative, so this proves it from the other side: a cache entry whose stat
// still matches is trusted *without* opening the transcript, so a deliberately
// wrong title in the cache comes straight back out. That can only happen if
// the file was never read.
func TestPersistedScanSparesTheRead(t *testing.T) {
	root, _ := resumeRoot(t, midTurn, func(*transcriptBuilder) {})
	cachePath := filepath.Join(t.TempDir(), "resume.json")

	warm := liveManager(root)
	c := fleet.OpenResumeCache(cachePath)
	warm.UseResumeCache(c)
	snapOf(t, warm)
	c.Save()

	raw, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("the cache was never written: %v", err)
	}
	doctored := strings.Replace(string(raw), "fix the 401 bug", "SERVED FROM CACHE", 1)
	if doctored == string(raw) {
		t.Fatal("the cache does not carry the scan; there is nothing to spare")
	}
	if err := os.WriteFile(cachePath, []byte(doctored), 0o644); err != nil {
		t.Fatal(err)
	}

	m := liveManager(root)
	m.UseResumeCache(fleet.OpenResumeCache(cachePath))
	sessions, err := m.Refresh(fleetNow)
	if err != nil {
		t.Fatal(err)
	}
	if got := sessions[0].Info.Title; got != "SERVED FROM CACHE" {
		t.Errorf("title is %q — the transcript was re-read despite an unchanged stat", got)
	}
}

// The tail scan drops its first slice, because a 64KB window usually opens
// mid-line. When it opens exactly at the START of one, that slice is a whole
// line — and dropping it discards real data, here the only line that says
// where the session is.
//
// The window is pinned to a line boundary arithmetically rather than by
// search: with every line exactly 256 bytes, the window start (size - 64KB) is
// a multiple of 256, which is a line start.
func TestTailWindowOpeningOnALineKeepsIt(t *testing.T) {
	const (
		lineLen = 256
		lines   = 400
		window  = 64 * 1024
	)
	start := lines*lineLen - window
	if start%lineLen != 0 {
		t.Fatalf("the window does not open on a line boundary (%d)", start)
	}
	located := start / lineLen // the line the window opens on

	root := t.TempDir()
	dir := filepath.Join(root, "projects", "-home-user-start")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	var b strings.Builder
	for i := 0; i < lines; i++ {
		cwd, ts := "", ago(time.Duration(lines-i)*time.Second)
		switch {
		case i == 0:
			cwd = "/home/user/start" // the head's answer, and the fallback
		case i == located:
			cwd = "/home/user/here" // the only other line that carries one
		}
		b.WriteString(padded(t, ts, cwd, lineLen))
	}
	path := filepath.Join(dir, "99999999-9999-4999-8999-999999999999.jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != lines*lineLen {
		t.Fatalf("file is %d bytes, want %d", len(raw), lines*lineLen)
	}
	if raw[start-1] != '\n' {
		t.Fatalf("the window does not open at a line start")
	}

	infos, err := fleet.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := infos[0].CWD; got != "/home/user/here" {
		t.Errorf("cwd is %q: the window opened at a line start and that whole "+
			"line was discarded along with the fragment that was not there", got)
	}
}

// padded writes one transcript line of exactly n bytes including its newline,
// stretching a filler field to make up the difference.
func padded(t *testing.T, ts time.Time, cwd string, n int) string {
	t.Helper()
	build := func(fill int) string {
		o := map[string]any{
			"type":      "assistant",
			"timestamp": ts.UTC().Format("2006-01-02T15:04:05.000Z"),
			"pad":       strings.Repeat("x", fill),
		}
		if cwd != "" {
			o["cwd"] = cwd
		}
		raw, err := json.Marshal(o)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw) + "\n"
	}
	fill := 0
	for len(build(fill)) < n {
		fill++
	}
	line := build(fill)
	if len(line) != n {
		t.Fatalf("cannot make a %d-byte line (got %d)", n, len(line))
	}
	return line
}

// A Task's sidechain lines are skipped when deciding where a session is, and a
// single large tool result fills the 64KB window on its own. A session mid-Task
// can therefore have a whole window with nothing of its own in it — and falling
// back to the head there files it at a directory it left, which un-buckets it
// from its own pane in MapSessions. The scan widens instead.
func TestASidechainFloodDoesNotRewindTheLocation(t *testing.T) {
	root := t.TempDir()
	const sess = "aaaa0000-0000-4000-8000-000000000001"

	b := newTranscript(t, sess, "/home/user/opened-here", "main").
		prompt(ago(2*time.Hour), "start here").
		text(ago(119*time.Minute), "working")
	b.moveTo("/home/user/moved-here", "feature-z")
	b.prompt(ago(time.Hour), "moved, now scout it")
	b.text(ago(59*time.Minute), "spawning a subagent")

	// A subagent that writes well past one 64KB window.
	b.moveTo("/home/user/subagent-dir", "detached")
	flood := strings.Repeat("y", 2000)
	for i := 0; i < 80; i++ {
		b.text(ago(time.Duration(50-i/2)*time.Minute), flood)
		b.lines[len(b.lines)-1] = strings.Replace(
			b.lines[len(b.lines)-1], `"isSidechain":false`, `"isSidechain":true`, 1)
	}
	b.write(root, "-home-user-opened-here")

	path := filepath.Join(root, "projects", "-home-user-opened-here", sess+".jsonl")
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() < 64*1024 {
		t.Fatalf("the sidechain flood is only %d bytes; it must exceed one window", fi.Size())
	}

	infos, err := fleet.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := infos[0].CWD; got != "/home/user/moved-here" {
		t.Errorf("cwd is %q — a Task longer than one window rewound the session "+
			"to where it was opened", got)
	}
	if got := infos[0].GitBranch; got != "feature-z" {
		t.Errorf("branch is %q, want feature-z", got)
	}
}

// The location ignores subagent lines; the clock must not. A session whose
// subagent wrote a second ago is busy, not quiet — and it is that timestamp
// which decides whether the fleet calls it live at all.
func TestASubagentCountsAsActivityInDiscovery(t *testing.T) {
	root := t.TempDir()
	b := newTranscript(t, "aaaa0000-0000-4000-8000-000000000002", "/home/user/main", "main").
		prompt(ago(time.Hour), "go and scout").
		text(ago(59*time.Minute), "spawning a subagent")
	b.text(ago(time.Second), "still scouting")
	b.lines[len(b.lines)-1] = strings.Replace(
		b.lines[len(b.lines)-1], `"isSidechain":false`, `"isSidechain":true`, 1)
	b.write(root, "-home-user-main")

	infos, err := fleet.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := ago(time.Second); !infos[0].LastEventAt.Equal(want) {
		t.Errorf("last activity is %v, want the subagent's line at %v",
			infos[0].LastEventAt, want)
	}
}
