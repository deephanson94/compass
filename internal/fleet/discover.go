// Package fleet finds every Claude Code session on the machine and keeps a live
// verdict for each one.
package fleet

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/transcript"
)

// SessionInfo is what compass knows about one session outside of its state.
type SessionInfo struct {
	ID             string // session uuid (filename stem)
	TranscriptPath string
	ProjectSlug    string // directory name under projects/
	CWD            string
	GitBranch      string
	Title          string    // first user prompt: first line, max 80 runes, "…" if cut
	StartedAt      time.Time // first event timestamp
	LastEventAt    time.Time // last event timestamp (file mtime as fallback)
}

// Key identifies a session uniquely. The session id does not: one id can own
// transcripts under several project slugs — a session that changes directory
// writes under the new slug, and both files answer to the same id. The
// transcript path always tells them apart, so every map, selection and cache
// that keys a session keys it by this (docs/dev/M6-CONTRACT.md).
//
// The id stays what it always was: the label the reader, the historian
// preamble and `claude --resume` use. It is a name, not a key.
func (i SessionInfo) Key() string { return i.TranscriptPath }

// Session pairs a session's identity with its current condition.
type Session struct {
	Info SessionInfo
	Snap state.Snapshot

	// Live says this session can still need you: it sits in a tmux pane, or
	// its transcript moved within the manager's live window. Everything else
	// is the archive — real, readable, and never amber
	// (docs/dev/M5-CONTRACT.md).
	Live bool
}

// titleMax is how much of a first prompt survives into the fleet list.
const titleMax = 80

// Discover scans <root>/projects/<slug>/*.jsonl. It does not recurse into
// session subdirectories (subagents are M1) and skips empty files. The result
// is sorted by LastEventAt descending. A missing root or projects directory
// returns (nil, nil) — an empty machine is not an error.
//
// Discover is uncached: its callers are one-shot. The Manager, which scans
// every second, uses the same walk with a cache (see scanProjects).
func Discover(root string) ([]SessionInfo, error) {
	out, _, err := scanProjects(root, nil)
	return out, err
}

// cachedInfo is one peeked transcript plus the file identity it was read from.
// A single Stat per file — the one the scan already does to skip empty
// transcripts — answers whether that read is still good.
type cachedInfo struct {
	size    int64
	modTime time.Time
	info    SessionInfo
}

// scanProjects walks the projects tree once. Given the previous scan's cache
// it reuses the SessionInfo of every transcript whose (size, mtime) has not
// moved, so an unchanged file is never opened; it returns the cache for the
// next call, pruned to the files that still exist. A nil prev simply peeks
// everything.
//
// Only discovery is cached. A live session's fresh lines still arrive through
// its tailer, which reads the file itself — so a stale peek can never make a
// running session look quiet.
func scanProjects(root string, prev map[string]cachedInfo) ([]SessionInfo, map[string]cachedInfo, error) {
	if root == "" {
		return nil, nil, nil
	}
	projects := filepath.Join(root, "projects")
	slugs, err := os.ReadDir(projects)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	var out []SessionInfo
	next := make(map[string]cachedInfo, len(prev))
	for _, slug := range slugs {
		if !slug.IsDir() {
			continue
		}
		dir := filepath.Join(projects, slug.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue // unreadable project: skip it, keep the fleet
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			fi, err := f.Info()
			if err != nil || fi.Size() == 0 {
				continue
			}
			path := filepath.Join(dir, f.Name())
			if c, ok := prev[path]; ok && c.size == fi.Size() && c.modTime.Equal(fi.ModTime()) {
				out = append(out, c.info)
				next[path] = c
				continue
			}
			info := SessionInfo{
				ID:             strings.TrimSuffix(f.Name(), ".jsonl"),
				TranscriptPath: path,
				ProjectSlug:    slug.Name(),
				LastEventAt:    fi.ModTime(), // refined to the last event time by the Manager
			}
			peek(&info, fi.Size())
			out = append(out, info)
			next[path] = cachedInfo{size: fi.Size(), modTime: fi.ModTime(), info: info}
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].LastEventAt.Equal(out[j].LastEventAt) {
			return out[i].LastEventAt.After(out[j].LastEventAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, next, nil
}

// Peeking reads both ends of a transcript and nothing in between: cwd, branch
// and the opening prompt live in the first handful of lines, the last event
// time in the last few. Transcripts reach tens of megabytes; both reads are
// bounded.
const (
	peekLines   = 64
	peekLineMax = 1 << 20
	peekTail    = 64 * 1024
)

// peek fills a session's identity fields from its transcript. Anything it
// cannot find stays as it was — a fleet entry is never an error.
func peek(info *SessionInfo, size int64) {
	f, err := os.Open(info.TranscriptPath)
	if err != nil {
		return
	}
	defer f.Close()

	peekHead(f, info)

	// The head only says where the session *began*. A session that changes
	// directory — or that Claude Code records differently later — keeps
	// writing its current cwd and branch on every event, so the tail is the
	// only honest answer to "where is this session now". It overrides the
	// head's, which stays as the fallback for a tail that carries neither.
	tail := peekTailState(f, size)
	if !tail.at.IsZero() {
		info.LastEventAt = tail.at // the mtime was only a stand-in
	}
	if tail.cwd != "" {
		info.CWD, info.GitBranch = tail.cwd, tail.branch
	}
}

// tailState is what the last few kilobytes of a transcript say about a session
// right now: when it last spoke, and where it was standing when it did.
type tailState struct {
	at     time.Time
	cwd    string
	branch string
}

func peekHead(f *os.File, info *SessionInfo) {
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), peekLineMax)
	for n := 0; n < peekLines && sc.Scan(); n++ {
		ev, err := transcript.ParseLine(sc.Bytes())
		if err != nil {
			continue
		}
		if info.CWD == "" {
			info.CWD = ev.CWD
		}
		if info.GitBranch == "" {
			info.GitBranch = ev.GitBranch
		}
		if info.StartedAt.IsZero() && !ev.Timestamp.IsZero() {
			info.StartedAt = ev.Timestamp
		}
		if info.Title == "" && ev.Type == transcript.EventUser {
			info.Title = promptTitle(ev.Text)
		}
		if info.CWD != "" && info.GitBranch != "" && info.Title != "" && !info.StartedAt.IsZero() {
			return
		}
	}
}

// lastEventTime walks the tail of a transcript backwards and returns the
// newest timestamp it carries, and the session's current location.
//
// Bookkeeping lines at the very end (mode latches, last-prompt markers) carry
// no timestamp, so it keeps walking until a real event answers.
//
// cwd and branch are taken together, from one line. Reading them independently
// looks harmless until a session leaves a git repository: Claude Code then
// writes an empty gitBranch, which is indistinguishable from "this line does
// not carry one", so the branch of the directory the session *left* would
// survive and be printed beside the directory it is now in. A line that names
// a cwd is a real event line, and its branch — empty or not — is the answer.
//
// Sidechain lines are a subagent's own conversation, not this session
// speaking, and while a Task is running they are the newest lines in the file.
// Everything else that reads transcripts skips them; so does this.
func peekTailState(f *os.File, size int64) tailState {
	var out tailState
	if size <= 0 {
		return out
	}
	start := size - peekTail
	if start < 0 {
		start = 0
	}
	buf := make([]byte, size-start)
	if _, err := f.ReadAt(buf, start); err != nil {
		return out
	}

	lines := strings.Split(string(buf), "\n")
	if start > 0 && len(lines) > 0 {
		lines = lines[1:] // the first slice is a fragment of an earlier line
	}
	located := false
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		ev, err := transcript.ParseLine([]byte(line))
		if err != nil || ev.IsSidechain {
			continue
		}
		if out.at.IsZero() && !ev.Timestamp.IsZero() {
			out.at = ev.Timestamp
		}
		if !located && ev.CWD != "" {
			out.cwd, out.branch, located = ev.CWD, ev.GitBranch, true
		}
		if !out.at.IsZero() && located {
			break
		}
	}
	return out
}

// promptTitle reduces a prompt to its first line, clipped to titleMax runes.
func promptTitle(text string) string {
	line := strings.TrimSpace(text)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	r := []rune(line)
	if len(r) <= titleMax {
		return line
	}
	return strings.TrimRight(string(r[:titleMax-1]), " ") + "…"
}

// SlugPath turns a project slug ("-home-user-compass") back into a plausible
// filesystem path. The encoding is lossy — dashes inside directory names are
// indistinguishable from separators — so this is only ever a display fallback
// for sessions whose transcript has not yet named its cwd.
func SlugPath(slug string) string {
	if slug == "" {
		return ""
	}
	return strings.ReplaceAll(slug, "-", "/")
}
