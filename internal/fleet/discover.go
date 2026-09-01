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

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/transcript"
)

// SessionInfo is what compass knows about one session outside of its state.
type SessionInfo struct {
	ID             string // session uuid (filename stem)
	TranscriptPath string
	ProjectSlug    string // directory name under projects/

	// CWD is where the session is now — it moves when Claude changes
	// directory, and it is what the deck shows.
	//
	// OriginCWD is where the session was opened, and it never moves. That is
	// the one that finds a tmux pane: a claude process keeps the working
	// directory it was launched in for its whole life, so /proc reports the
	// origin however far the session has since wandered. Matching on CWD alone
	// loses a session the moment it cd's into a sibling repo — which is most
	// long sessions.
	CWD         string
	OriginCWD   string
	GitBranch   string
	Title       string    // first user prompt: first line, max 80 runes, "…" if cut
	StartedAt   time.Time // first event timestamp
	LastEventAt time.Time // last event timestamp (file mtime as fallback)
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

	// Class is the kind of work the session is doing right now, in the trail's
	// own vocabulary, so the fleet and the trail describe it the same way.
	// HasClass is false until an event says something classifiable — and for
	// every archived session, which is not doing anything.
	Class    journey.Class
	HasClass bool

	// Outcome is the last thing the session finished — "1216 passed · 2 failed",
	// a commit subject — as opposed to the tool call it currently has in
	// flight. Empty when it has not finished anything worth reporting.
	Outcome string
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
	at      time.Time
	cwd     string
	branch  string
	located bool // a line of this session's own named a cwd
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
			info.CWD, info.OriginCWD = ev.CWD, ev.CWD
		}
		if info.GitBranch == "" {
			info.GitBranch = ev.GitBranch
		}
		if info.StartedAt.IsZero() && !ev.Timestamp.IsZero() {
			info.StartedAt = ev.Timestamp
		}
		if info.Title == "" && ev.Type == transcript.EventUser {
			info.Title = promptTitle(ev)
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
	// Widen backwards while the location is still unanswered. One window is
	// enough for an ordinary transcript, but a Task's sidechain lines are
	// skipped here and a single large tool result fills 64KB on its own — so a
	// session mid-Task can have a whole window with nothing of its own in it.
	// Falling back to the head there would file the session at a directory it
	// left, which is exactly the failure this function exists to prevent.
	for end := int64(1); end <= peekWindows; end++ {
		start := size - end*peekTail
		if start < 0 {
			start = 0
		}
		scanTail(f, size, start, &out)
		if out.located || start == 0 {
			break
		}
	}
	return out
}

// peekWindows bounds the widening: a transcript whose last megabyte is all
// subagent keeps the head's answer rather than reading the whole file on every
// scan.
const peekWindows = 16

// scanTail walks one window backwards, filling whatever `out` still lacks.
func scanTail(f *os.File, size, start int64, out *tailState) {
	// Read one byte further back than the window needs. That byte decides
	// whether the window opens mid-line or exactly at the start of one: with
	// it included, the first slice of the split is the earlier line's tail in
	// the first case and empty in the second, so dropping it is right either
	// way. Splitting from the window's own first byte cannot tell the two
	// apart, and drops a whole line whenever the window lands on a boundary.
	from := start
	if start > 0 {
		from--
	}
	buf := make([]byte, size-from)
	if _, err := f.ReadAt(buf, from); err != nil {
		return
	}

	lines := strings.Split(string(buf), "\n")
	if start > 0 && len(lines) > 0 {
		lines = lines[1:]
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
		// A subagent writing right now is this session being busy, so every
		// line moves the clock — only the location is the session's alone.
		if out.at.IsZero() && !ev.Timestamp.IsZero() {
			out.at = ev.Timestamp
		}
		if !out.located && !ev.IsSidechain && ev.CWD != "" {
			out.cwd, out.branch, out.located = ev.CWD, ev.GitBranch, true
		}
		if !out.at.IsZero() && out.located {
			return
		}
	}
}

// promptTitle reduces a user turn to the one line worth showing, clipped to
// titleMax runes. It returns "" for anything that is not a person talking:
// the fleet asks its rows what you asked for, and the harness's own turns are
// not an answer to that.
func promptTitle(ev transcript.Event) string {
	if ev.Machinery() {
		return ""
	}
	text := ev.Text
	// A slash command is a real thing you typed; only its expansion is noise.
	if cmd, ok := transcript.SlashCommand(text); ok {
		text = cmd
	}
	return clipTitle(text)
}

// clipTitle reduces text to its first line, clipped to titleMax runes.
func clipTitle(text string) string {
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
