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

// Session pairs a session's identity with its current condition.
type Session struct {
	Info SessionInfo
	Snap state.Snapshot
}

// titleMax is how much of a first prompt survives into the fleet list.
const titleMax = 80

// Discover scans <root>/projects/<slug>/*.jsonl. It does not recurse into
// session subdirectories (subagents are M1) and skips empty files. The result
// is sorted by LastEventAt descending. A missing root or projects directory
// returns (nil, nil) — an empty machine is not an error.
func Discover(root string) ([]SessionInfo, error) {
	if root == "" {
		return nil, nil
	}
	projects := filepath.Join(root, "projects")
	slugs, err := os.ReadDir(projects)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []SessionInfo
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
			info := SessionInfo{
				ID:             strings.TrimSuffix(f.Name(), ".jsonl"),
				TranscriptPath: filepath.Join(dir, f.Name()),
				ProjectSlug:    slug.Name(),
				LastEventAt:    fi.ModTime(), // refined to the last event time by the Manager
			}
			peek(&info, fi.Size())
			out = append(out, info)
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].LastEventAt.Equal(out[j].LastEventAt) {
			return out[i].LastEventAt.After(out[j].LastEventAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
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
	if at := lastEventTime(f, size); !at.IsZero() {
		info.LastEventAt = at // the mtime was only a stand-in
	}
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
// newest timestamp it finds. Bookkeeping lines at the very end (mode latches,
// last-prompt markers) carry none, so it keeps walking until a real event
// answers. A zero time means "use the mtime".
func lastEventTime(f *os.File, size int64) time.Time {
	if size <= 0 {
		return time.Time{}
	}
	start := size - peekTail
	if start < 0 {
		start = 0
	}
	buf := make([]byte, size-start)
	if _, err := f.ReadAt(buf, start); err != nil {
		return time.Time{}
	}

	lines := strings.Split(string(buf), "\n")
	if start > 0 && len(lines) > 0 {
		lines = lines[1:] // the first slice is a fragment of an earlier line
	}
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		ev, err := transcript.ParseLine([]byte(line))
		if err != nil {
			continue
		}
		if !ev.Timestamp.IsZero() {
			return ev.Timestamp
		}
	}
	return time.Time{}
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
