package fleet

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/transcript"
)

// entry is one tracked session: what we know, where we are in its file, and the
// machine folding its events. An archived entry keeps only what we know — the
// tailer and the machine are what a live session needs.
type entry struct {
	info     SessionInfo
	tailer   *transcript.Tailer
	machine  *state.Machine
	sawEvent bool // once events carry timestamps, they beat the file's mtime

	// class is the kind of work the session's latest classifiable event
	// belongs to — the trail's own vocabulary, so the fleet and the trail
	// describe a session the same way. Only live sessions have one: an
	// archived session is not doing anything.
	class    journey.Class
	hasClass bool
}

// DefaultLiveWindow is the recency door a new Manager opens: a session with no
// pane still counts as live while its transcript moved this recently. Pane
// matching is a heuristic, and a session writing its transcript right now must
// never be hidden by a matching miss.
const DefaultLiveWindow = 5 * time.Minute

// Manager owns the fleet: discovery, one tailer and one state machine per live
// session, and the display ordering. It is safe for concurrent use.
type Manager struct {
	mu   sync.Mutex
	root string

	// sessions is one entry per tracked session, keyed by SessionInfo.Key() —
	// the transcript path. Two transcripts sharing a session id are two
	// sessions here, each with its own tailer, machine and verdict.
	sessions map[string]*entry

	// excluded are cleaned absolute CWDs whose sessions the fleet refuses to
	// show — compass's own narration dir, so it never watches itself narrate.
	excluded map[string]bool

	// paneMapped are the keys of the sessions the ui last found in a tmux
	// pane, and liveWindow is the recency door for the rest
	// (docs/dev/M5-CONTRACT.md).
	paneMapped map[string]bool
	liveWindow time.Duration

	// resume, when set, lets a live session be picked up where an earlier
	// process stopped reading rather than replayed from byte zero.
	resume *ResumeCache

	// cache is the last discovery scan, keyed by transcript path: 280 archived
	// transcripts are stat'ed every second, not re-read.
	cache map[string]cachedInfo
}

// NewManager returns a Manager watching the given Claude home directory.
func NewManager(root string) *Manager {
	return &Manager{
		root:       root,
		sessions:   make(map[string]*entry),
		liveWindow: DefaultLiveWindow,
	}
}

// Root is the Claude home directory this Manager watches.
func (m *Manager) Root() string { return m.root }

// MarkPaneMapped tells the manager which sessions currently sit in a tmux
// pane; the ui feeds it after every MapSessions. The map is keyed by
// SessionInfo.Key(), so a pane makes exactly the session holding it live —
// never a twin that happens to share its id. A pane makes a session live
// however long it has been quiet. The zero state — never called, or an empty
// map — means no panes are known, and only the recency door admits anyone.
func (m *Manager) MarkPaneMapped(keys map[string]bool) {
	// Copied, not kept: the ui's map is the ui's to reuse.
	mapped := make(map[string]bool, len(keys))
	for key, ok := range keys {
		if ok && key != "" {
			mapped[key] = true
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(mapped) == 0 {
		m.paneMapped = nil
		return
	}
	m.paneMapped = mapped
}

// SetLiveWindow sets the recency door: a paneless session still counts as live
// while now−LastEventAt ≤ d; 0 closes the door (panes only).
func (m *Manager) SetLiveWindow(d time.Duration) {
	if d < 0 {
		d = 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.liveWindow = d
}

// isLive answers rule 1: a session is live if tmux has it, or if its
// transcript moved inside the window. Caller holds the mutex.
func (m *Manager) isLive(info SessionInfo, now time.Time) bool {
	if m.paneMapped[info.Key()] {
		return true
	}
	if m.liveWindow <= 0 || info.LastEventAt.IsZero() {
		return false // the door is shut, or there is nothing to hold it open
	}
	return !info.LastEventAt.Before(now.Add(-m.liveWindow))
}

// UseResumeCache tells the Manager to pick live sessions up where an earlier
// process left them, instead of replaying each one. Call Save on the returned
// cache when the process is done with it; a Manager with no cache behaves
// exactly as before.
func (m *Manager) UseResumeCache(c *ResumeCache) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resume = c
}

// ExcludeCWD hides sessions whose CWD is path (the narrator's Dir): compass
// must never watch itself narrate. Paths are compared cleaned and absolute, so
// the caller can pass whatever form it has. An already-tracked session at that
// path drops out on the next Refresh.
func (m *Manager) ExcludeCWD(path string) {
	path = normalizeDir(path)
	if path == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.excluded == nil {
		m.excluded = make(map[string]bool)
	}
	m.excluded[path] = true
}

// isExcluded reports whether a session's CWD is on the exclusion list. Caller
// holds the mutex.
func (m *Manager) isExcluded(cwd string) bool {
	if len(m.excluded) == 0 || cwd == "" {
		return false
	}
	return m.excluded[normalizeDir(cwd)]
}

// normalizeDir puts a directory in the one form the comparison uses: cleaned
// and absolute where the filesystem can say so, cleaned where it cannot.
func normalizeDir(path string) string {
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}

// Refresh re-discovers sessions, polls each live tailer, feeds the machines and
// returns the fleet in display order: the live block first — needs-you (longest
// waiting first), stuck (longest first), working (most recent activity first),
// idle (most recent first) — then the archive, newest last event first.
//
// Only live sessions are tailed and state-machined. The archive is real and
// readable but it can never be amber, which is what keeps `g` and the attention
// chips truthful by construction (docs/dev/M5-CONTRACT.md).
func (m *Manager) Refresh(now time.Time) ([]Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// A fresh process starts from whatever the last one wrote down; a
	// long-lived one is already warm and the seed is a no-op after the first
	// pass.
	if m.cache == nil {
		m.cache = m.resume.seed()
	}
	infos, cache, err := scanProjects(m.root, m.cache)
	if err != nil {
		return nil, err
	}
	m.cache = cache
	m.resume.keepScan(cache)

	kept := make(map[string]bool, len(infos))
	out := make([]Session, 0, len(infos))
	archive := make([]Session, 0, len(infos))
	for _, info := range infos {
		if m.isExcluded(info.CWD) {
			continue // never tracked, so the next sweep also forgets it
		}
		key := info.Key()
		kept[key] = true
		e := m.sessions[key]
		if e == nil {
			e = &entry{}
			m.sessions[key] = e
		}
		e.merge(info)

		live := m.isLive(e.info, now)
		if live {
			// Waking from the archive means a tailer from scratch: the whole
			// file replays, exactly as it does at first sight.
			if e.tailer == nil {
				m.wake(key, e)
			}
			// A session whose file we cannot read keeps its last known state
			// rather than vanishing from the fleet.
			if events, err := e.tailer.Poll(); err == nil {
				for _, ev := range events {
					e.machine.Observe(ev)
					e.absorb(ev)
				}
			}
		} else {
			e.sleep()
		}

		// Discovery reads the head of the file; the events may name a different
		// cwd, and an excluded one only has to be seen once to disqualify.
		if m.isExcluded(e.info.CWD) {
			delete(kept, key)
			continue
		}

		if live {
			m.resume.record(key, ResumePoint{Mark: e.tailer.Mark(), Fold: e.machine.Fold()})
			out = append(out, Session{
				Info: e.info, Snap: e.machine.Evaluate(now), Live: true,
				Class: e.class, HasClass: e.hasClass,
			})
		} else {
			archive = append(archive, Session{Info: e.info, Snap: archivedSnap(e.info)})
		}
	}

	for key := range m.sessions {
		if !kept[key] {
			delete(m.sessions, key)
		}
	}
	m.resume.retain(kept)

	sortFleet(out)
	sortArchive(archive)
	return append(out, archive...), nil
}

// wake gives an entry what a live session needs.
//
// Without a resume cache both are built from scratch, and the session replays
// its whole transcript — correct, and on a long session the most expensive
// thing compass does. With one, the tailer starts where the last process
// stopped and the machine starts from what it had concluded there, so only the
// appended bytes are read. The tailer refuses a mark that no longer fits the
// file, and a refused mark simply means the replay happens after all.
func (m *Manager) wake(key string, e *entry) {
	e.tailer = transcript.NewTailer(e.info.TranscriptPath)
	if p, ok := m.resume.point(key); ok && e.tailer.Resume(p.Mark) {
		e.machine = state.RestoreMachine(p.Fold)
		e.sawEvent = !p.Fold.LastEventAt.IsZero()
		return
	}
	e.machine = state.NewMachine()
}

// sleep drops the tailer's offset and the machine's fold — memory an archived
// session has no use for — and keeps everything we know about it. Losing
// sawEvent is the point too: with nothing tailing, the file's own clock is the
// only clock, and it is the one that will wake this session up again.
func (e *entry) sleep() {
	e.tailer = nil
	e.machine = nil
	e.sawEvent = false
	e.hasClass = false
}

// archivedSnap is the whole verdict an archived session gets: it did something
// once, and it is not doing anything now.
func archivedSnap(info SessionInfo) state.Snapshot {
	return state.Snapshot{
		State:    state.Idle,
		Since:    info.LastEventAt,
		Reason:   "archived",
		Activity: "idle",
	}
}

// merge folds a fresh Discover result into what the entry already knows.
// Discovery reads the head of the file; the tailer reads the rest, and what it
// has seen wins.
func (e *entry) merge(info SessionInfo) {
	e.info.ID = info.ID
	e.info.TranscriptPath = info.TranscriptPath
	e.info.ProjectSlug = info.ProjectSlug
	if e.info.CWD == "" {
		e.info.CWD = info.CWD
	}
	if e.info.OriginCWD == "" {
		e.info.OriginCWD = info.OriginCWD
	}
	if e.info.GitBranch == "" {
		e.info.GitBranch = info.GitBranch
	}
	if e.info.Title == "" {
		e.info.Title = info.Title
	}
	if e.info.StartedAt.IsZero() {
		e.info.StartedAt = info.StartedAt
	}
	if !e.sawEvent && info.LastEventAt.After(e.info.LastEventAt) {
		e.info.LastEventAt = info.LastEventAt // file mtime, until events say otherwise
	}
}

// absorb folds an event's identity fields into what we know about the session.
//
// This is the live path's authority on where a session is — discovery only
// speaks once, at first sight — so it answers the location question the same
// way peekTailState does, and for the same reasons. A sidechain line is a
// subagent's own conversation, so it moves nothing; and cwd and branch travel
// together, because Claude Code writes an empty branch outside a git
// repository and taking them separately would keep the branch of a directory
// the session has left.
//
// Timestamps are a different question: a subagent writing right now is this
// session being busy, so every line moves the clock.
func (e *entry) absorb(ev transcript.Event) {
	// The class the trail would put this moment in. It is the same vocabulary
	// Lv1 uses, folded per event rather than segmented into legs — the fleet
	// only needs to know what a session is doing right now, not how its work
	// divided up. A subagent's tools are its own, not this session's.
	if !ev.IsSidechain {
		if c, ok := journey.Classify(ev); ok {
			e.class, e.hasClass = c, true
		}
	}
	if !ev.IsSidechain && ev.CWD != "" {
		e.info.CWD, e.info.GitBranch = ev.CWD, ev.GitBranch
		if e.info.OriginCWD == "" {
			e.info.OriginCWD = ev.CWD
		}
	}
	if !ev.Timestamp.IsZero() {
		if e.info.StartedAt.IsZero() || ev.Timestamp.Before(e.info.StartedAt) {
			e.info.StartedAt = ev.Timestamp
		}
		if !e.sawEvent || ev.Timestamp.After(e.info.LastEventAt) {
			e.info.LastEventAt = ev.Timestamp
		}
		e.sawEvent = true
	}
	if e.info.Title == "" && ev.Type == transcript.EventUser {
		if title := promptTitle(ev.Text); title != "" {
			e.info.Title = title
		}
	}
}

// rank orders the states for display: the ones that want something from you
// first, the calm ones last.
func rank(s state.State) int {
	switch s {
	case state.NeedsYou:
		return 0
	case state.Stuck:
		return 1
	case state.Working:
		return 2
	default:
		return 3
	}
}

func sortFleet(ss []Session) {
	sort.SliceStable(ss, func(i, j int) bool {
		a, b := ss[i], ss[j]
		ra, rb := rank(a.Snap.State), rank(b.Snap.State)
		if ra != rb {
			return ra < rb
		}
		if ra <= 1 {
			// Waiting states: the longest wait rises to the top.
			if !a.Snap.Since.Equal(b.Snap.Since) {
				return a.Snap.Since.Before(b.Snap.Since)
			}
		} else {
			// Calm states: the most recent activity first.
			if !a.Info.LastEventAt.Equal(b.Info.LastEventAt) {
				return a.Info.LastEventAt.After(b.Info.LastEventAt)
			}
		}
		return a.Info.ID < b.Info.ID
	})
}

// sortArchive orders what nothing is waiting on: most recently touched first,
// which is the order you would go looking through it.
func sortArchive(ss []Session) {
	sort.SliceStable(ss, func(i, j int) bool {
		a, b := ss[i], ss[j]
		if !a.Info.LastEventAt.Equal(b.Info.LastEventAt) {
			return a.Info.LastEventAt.After(b.Info.LastEventAt)
		}
		return a.Info.ID < b.Info.ID
	})
}

// Glyphs carry state on their own, so the panel reads in pure monochrome.
const (
	GlyphWorking  = "●"
	GlyphNeedsYou = "▲"
	GlyphIdle     = "○"
	GlyphStuck    = "◍"
)

// Glyph is the one-rune mark for a state.
func Glyph(s state.State) string {
	switch s {
	case state.NeedsYou:
		return GlyphNeedsYou
	case state.Stuck:
		return GlyphStuck
	case state.Working:
		return GlyphWorking
	default:
		return GlyphIdle
	}
}

// StatusLine renders the one-shot summary for `compass status` and for a tmux
// status-right: counts in fleet order, zero counts omitted, e.g. "▲1 ◍1 ●3".
// A fleet with nothing waiting or working reads "○ all quiet" — the point of
// the line is attention, and an all-idle machine wants none.
//
// Only live sessions are counted. An archive of 280 transcripts is not a
// status.
func (m *Manager) StatusLine(now time.Time) string {
	sessions, err := m.Refresh(now)
	if err != nil {
		return "○ all quiet"
	}

	counts := map[state.State]int{}
	for _, s := range sessions {
		if !s.Live {
			continue
		}
		counts[s.Snap.State]++
	}
	if counts[state.NeedsYou]+counts[state.Stuck]+counts[state.Working] == 0 {
		return "○ all quiet"
	}

	var parts []string
	for _, st := range []state.State{state.NeedsYou, state.Stuck, state.Working, state.Idle} {
		if n := counts[st]; n > 0 {
			parts = append(parts, Glyph(st)+itoa(n))
		}
	}
	return strings.Join(parts, " ")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
