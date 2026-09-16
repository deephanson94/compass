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
	Relayed     bool      // the title is another session's message, relayed (#97)
	Name        string    // the name the person gave the session (/rename), "" until they do (#79)
	StartedAt   time.Time // first event timestamp
	LastEventAt time.Time // last event timestamp (file mtime as fallback)

	// Tool is what runs the session — "" or "claude" for Claude Code,
	// "opencode" for an OpenCode session read out of its store — and
	// Model the model it last answered with, as the tool names it
	// ("claude-opus-4-1-20250805", "mock/mock-1"). The deck says both, so
	// two sessions in one directory are told apart by more than a name.
	Tool  string
	Model string

	// Asked says the last word in this transcript is the model's and it was a
	// question: the session stopped on something only you can answer, and
	// nobody has. It is read off the file itself, not off a machine, so it
	// survives the pane closing and compass restarting — which is the whole
	// point, since a question you forgot is one you were not watching
	// (#344). AskedAt is when it was asked.
	//
	// It clears itself: the moment you reply, the last word is yours.
	Asked   bool
	AskedAt time.Time
}

// ToolName is the tool a session runs under, "claude" when unsaid.
func (i SessionInfo) ToolName() string {
	if i.Tool == "" {
		return "claude"
	}
	return i.Tool
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

	// Waiting says this session is live only because it is holding a question
	// open: no pane, nothing written for longer than the live window, and the
	// last word in its transcript is the model's, asking you something. It is
	// the session you walked away from — amber, reachable, and ranked under
	// the alarms of the moment so today's fleet is still read first (#344).
	Waiting bool

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
	out, _, err := scanProjects(root, nil, time.Now().Add(-DefaultLiveWindow))
	return out, err
}

// cachedInfo is one peeked transcript plus the file identity it was read from.
// A single Stat per file — the one the scan already does to skip empty
// transcripts — answers whether that read is still good.
type cachedInfo struct {
	size    int64
	modTime time.Time
	info    SessionInfo
	// wide says the peek that produced this info read the whole tail for a
	// question rather than one window. A file peeked while it was still
	// moving is cached narrow, and a file that then goes quiet never moves
	// again — so without this the widening only ever ran after a restart,
	// never for the session you walked away from while compass was up,
	// which is the field finding's own timeline (round 60).
	wide bool
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
func scanProjects(root string, prev map[string]cachedInfo, quiet time.Time) ([]SessionInfo, map[string]cachedInfo, error) {
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
			wide := fi.ModTime().Before(quiet)
			if c, ok := prev[path]; ok && c.size == fi.Size() && c.modTime.Equal(fi.ModTime()) && (c.wide || !wide) {
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
			// A file that has not moved since the quiet mark is one whose
			// question is load-bearing — nothing else can keep it live —
			// so its ask walk gets the whole widening (round 59). The
			// crossing is what re-opens it: a file peeked narrow while it
			// was moving is read again, once, when it falls quiet.
			peek(&info, fi.Size(), wide)
			out = append(out, info)
			next[path] = cachedInfo{size: fi.Size(), modTime: fi.ModTime(), info: info, wide: wide}
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
func peek(info *SessionInfo, size int64, wide bool) {
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
	tail := peekTailState(f, size, wide)
	if !tail.at.IsZero() {
		info.LastEventAt = tail.at // the mtime was only a stand-in
	}
	if tail.cwd != "" {
		info.CWD, info.GitBranch = tail.cwd, tail.branch
	}
	if tail.name != "" {
		info.Name = tail.name // the newest rename wins
	}
	info.Asked, info.AskedAt = tail.asked, tail.askedAt
	if info.Asked && info.AskedAt.IsZero() {
		info.AskedAt = info.LastEventAt
	}
}

// tailState is what the last few kilobytes of a transcript say about a session
// right now: when it last spoke, and where it was standing when it did.
type tailState struct {
	at      time.Time
	cwd     string
	branch  string
	located bool // a line of this session's own named a cwd
	name    string

	// asked is the verdict of the ask walk (seeAsk), settled is whether it
	// has its answer — a line that settled it, or the end of what it was
	// given to read — and answered are the tool calls
	// whose results are already in the file — collected on the way back, so
	// a call still out is one no result has answered.
	asked    bool
	askedAt  time.Time
	settled  bool
	answered map[string]bool
	// heldMsg is a turn whose call is still out, waiting to see whether an
	// older line of the same turn is the question. Claude Code writes one
	// content block per line and repeats the message id across them, so
	// the batch #344's either-order rule is about is never one line: the
	// walk meets the Task first and would settle on it, archiving a
	// question the machine calls needs-you. heldAt is the clock that
	// decision was reached at (#348).
	heldMsg string
	// busy is a subagent writing under whatever the walk finds next: the
	// session is doing something, so its words are not a question you owe
	// — but a question the harness is holding open outranks that, as the
	// machine's own rules do (round 61).
	busy bool
}

func peekHead(f *os.File, info *SessionInfo) {
	titleRank := titleNone
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
		if ev.Name != "" {
			info.Name = ev.Name
		}
		if titleRank < titleProse && ev.Type == transcript.EventUser {
			if title, rank := promptTitle(ev); rank > titleRank {
				info.Title, titleRank = title, rank
				info.Relayed = rank == titleRelay
			}
		}
		if info.CWD != "" && info.GitBranch != "" && titleRank == titleProse && !info.StartedAt.IsZero() {
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
func peekTailState(f *os.File, size int64, wide bool) tailState {
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
		out.endHeld()
		if !wide {
			// A file that is still being written is live on the recency
			// door whatever this walk decides, so its ask gets one window
			// and the widening is left to the location, which is what it
			// was written for.
			out.settled = true
		}
		if (out.located && out.settled) || start == 0 {
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
		if out.name == "" && ev.Name != "" {
			out.name = ev.Name // walking backwards: the newest rename
		}
		out.seeAsk(ev)
		if !out.at.IsZero() && out.located && out.settled {
			return
		}
	}
}

// seeAsk walks one line of the backwards scan and, on the first line that
// settles it, says whether this transcript ends on a question nobody has
// answered. It reads the file the way the state machine reads the fold, so the
// two agree on what "needs you" means:
//
//   - a person's words are the answer to anything above them — nothing waits;
//   - a call still out (no result for it further down the file) is work in
//     flight, except AskUserQuestion, which is the model asking in as many
//     words;
//   - otherwise the model's last words decide it, by rule 4's own test.
//
// The harness's own user turns ("Continue from where you left off.") settle
// nothing: nobody typed them, so they answer nothing — exactly as the machine
// treats them. A line with neither words nor a call is skipped for the same
// reason, and the walk keeps going back.
func (t *tailState) seeAsk(ev transcript.Event) {
	if t.settled {
		return
	}
	if ev.IsSidechain {
		// A subagent's conversation is not this session speaking, so its
		// words never open the door — but a subagent writing at all is
		// this session being busy, which is how the machine reads it too,
		// and a walk that read past it would call a session with an agent
		// in flight a question you owe (round 59).
		//
		// It marks rather than settles. The machine's rule 2 — a question
		// the harness is holding open — precedes its rule about a turn in
		// flight, so a session that asked you in as many words and
		// dispatched an agent in the same breath is needs-you while the
		// agent runs; a walk that stopped at the agent's first line
		// archived it (round 61).
		if ev.Type == transcript.EventUser && strings.TrimSpace(ev.Text) != "" {
			t.busy = true
		}
		return
	}
	switch ev.Type {
	case transcript.EventUser:
		if t.heldMsg != "" {
			// Anything of yours or the harness's below the held turn means
			// the turn is over and its call is the last word in it.
			t.settleAsk(false, time.Time{})
			return
		}
		for _, r := range ev.ToolResults {
			if t.answered == nil {
				t.answered = make(map[string]bool)
			}
			t.answered[r.ToolUseID] = true
		}
		if strings.TrimSpace(ev.Text) != "" && !ev.Machinery() {
			t.settleAsk(false, time.Time{})
		}
	case transcript.EventAssistant:
		// The whole message decides, not its first block — and a message
		// is not a line. Claude Code writes one block per line and repeats
		// `message.id` across them, so a turn that asked you something and
		// dispatched an agent in the same breath is two lines, the agent's
		// first. Settling on it archived a question the machine calls
		// needs-you, which is the either-order rule failing on the only
		// shape the harness actually writes (#344 round 59, measured and
		// fixed in #348).
		if t.heldMsg != "" && ev.MessageID != t.heldMsg {
			t.settleAsk(false, time.Time{}) // the held turn ended: its call stands
			return
		}
		out := 0
		for _, u := range ev.ToolUses {
			if t.answered[u.ID] {
				continue
			}
			if u.Name == state.AskUserQuestion {
				t.settleAsk(true, ev.Timestamp)
				return
			}
			out++
		}
		if out > 0 || t.busy {
			if out > 0 && ev.MessageID != "" && !t.busy {
				// Hold it while the rest of this turn is still to come:
				// an older line of the same message may be the question,
				// and rule 2 precedes rule 3.
				t.heldMsg = ev.MessageID
				return
			}
			t.settleAsk(false, time.Time{}) // a call still out: work in flight
			return
		}
		if t.heldMsg != "" {
			// Still inside the held turn — a line of thinking or narration
			// between its calls decides nothing.
			return
		}
		if len(ev.ToolUses) > 0 {
			// Every call in it came back, so the model's words here are not
			// the last word in the file — the results are, and the turn is
			// the model's to continue. The machine calls that working or
			// hung; a door that read the text would call it a question
			// nobody was ever asked.
			t.settleAsk(false, time.Time{})
			return
		}
		if strings.TrimSpace(ev.Text) == "" {
			return
		}
		// A refused call is not the model asking: nothing you type into the
		// pane clears a 403, and `g` skips those for the same reason.
		t.settleAsk(!t.busy && !ev.APIError && state.EndsWithQuestion(ev.Text), ev.Timestamp)
	}
}

// endHeld closes a turn the walk was still inside when it ran out of file:
// nothing older is coming, so the call it was holding is the last word.
func (t *tailState) endHeld() {
	if t.heldMsg != "" && !t.settled {
		t.settleAsk(false, time.Time{})
	}
}

// settleAsk records the walk's verdict and stops it.
func (t *tailState) settleAsk(asked bool, at time.Time) {
	t.asked, t.askedAt, t.settled = asked, at, true
	if !asked {
		t.askedAt = time.Time{}
	}
}

// Title ranks. A session's title is the strongest-ranked user turn seen so
// far, not the first: the first thing most people type is "/model", and a row
// that says so nine times over says nothing (2026-09-02 dogfood).
const (
	titleNone  = iota
	titleSlash // a bare slash command: "/model", "/compact"
	titleRelay // another session's message, relayed: the ask a worker gets (#97)
	titleArgs  // a slash command with arguments: "/kickoff probe the prefix tree"
	titleProse // words a person wrote
)

// promptTitle reduces a user turn to the one line worth showing, clipped to
// titleMax runes, and says how much it is worth. It returns titleNone for
// anything that is not a person talking: the fleet asks its rows what you
// asked for, and the harness's own turns are not an answer to that.
func promptTitle(ev transcript.Event) (string, int) {
	if ev.Machinery() {
		return "", titleNone
	}
	if ev.Relayed() {
		if title := clipTitle(ev.RelayBody()); title != "" {
			return title, titleRelay
		}
		return "", titleNone
	}
	if cmd, ok := transcript.SlashCommand(ev.Text); ok {
		rank := titleSlash
		if strings.Contains(cmd, " ") {
			rank = titleArgs
		}
		return clipTitle(cmd), rank
	}
	title := clipTitle(ev.Text)
	if title == "" {
		return "", titleNone
	}
	return title, titleProse
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
