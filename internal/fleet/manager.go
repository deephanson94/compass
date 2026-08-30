package fleet

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/transcript"
)

// entry is one tracked session: what we know, where we are in its file, and the
// machine folding its events.
type entry struct {
	info     SessionInfo
	tailer   *transcript.Tailer
	machine  *state.Machine
	sawEvent bool // once events carry timestamps, they beat the file's mtime
}

// Manager owns the fleet: discovery, one tailer and one state machine per
// session, and the display ordering. It is safe for concurrent use.
type Manager struct {
	mu       sync.Mutex
	root     string
	sessions map[string]*entry
}

// NewManager returns a Manager watching the given Claude home directory.
func NewManager(root string) *Manager {
	return &Manager{root: root, sessions: make(map[string]*entry)}
}

// Root is the Claude home directory this Manager watches.
func (m *Manager) Root() string { return m.root }

// Refresh re-discovers sessions, polls each tailer, feeds the machines and
// returns the fleet in display order: needs-you (longest waiting first), stuck
// (longest first), working (most recent activity first), idle (most recent
// first).
func (m *Manager) Refresh(now time.Time) ([]Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	infos, err := Discover(m.root)
	if err != nil {
		return nil, err
	}

	live := make(map[string]bool, len(infos))
	out := make([]Session, 0, len(infos))
	for _, info := range infos {
		live[info.ID] = true
		e := m.sessions[info.ID]
		if e == nil || e.tailer.Path() != info.TranscriptPath {
			e = &entry{
				info:    info,
				tailer:  transcript.NewTailer(info.TranscriptPath),
				machine: state.NewMachine(),
			}
			m.sessions[info.ID] = e
		}
		e.merge(info)

		// A session whose file we cannot read keeps its last known state rather
		// than vanishing from the fleet.
		if events, err := e.tailer.Poll(); err == nil {
			for _, ev := range events {
				e.machine.Observe(ev)
				e.absorb(ev)
			}
		}

		out = append(out, Session{Info: e.info, Snap: e.machine.Evaluate(now)})
	}

	for id := range m.sessions {
		if !live[id] {
			delete(m.sessions, id)
		}
	}

	sortFleet(out)
	return out, nil
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
func (e *entry) absorb(ev transcript.Event) {
	if ev.CWD != "" {
		e.info.CWD = ev.CWD
	}
	if ev.GitBranch != "" {
		e.info.GitBranch = ev.GitBranch
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
func (m *Manager) StatusLine(now time.Time) string {
	sessions, err := m.Refresh(now)
	if err != nil {
		return "○ all quiet"
	}

	counts := map[state.State]int{}
	for _, s := range sessions {
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
