package tmuxop

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/deephanson94/compass/internal/fleet"
)

// Pane is one pane on the tmux server.
type Pane struct {
	Target  string // "dev:1.0" (session:window.pane)
	ID      string // "%5"
	PID     int
	Command string // pane_current_command
	Window  string // window_name — what the user called it, e.g. "porter-test"
}

// paneFormat is the -F string ListPanes asks tmux for. The separator is a tab
// because every field here may contain spaces.
//
// window_name comes last on purpose: it is the one field a user writes freely,
// so it is the one allowed to contain the separator that the sanitized form
// falls back to (see parsePane). pane_current_path is deliberately absent —
// nothing reads it, and being an arbitrary string in the middle of the row it
// was the thing that made the row ambiguous.
const paneFormat = "#{session_name}:#{window_index}.#{pane_index}\t#{pane_id}\t#{pane_pid}\t#{pane_current_command}\t#{window_name}"

// paneFields is how many tab-separated columns paneFormat produces.
const paneFields = 5

// ListPanes runs: list-panes -a -F <paneFormat>
// A tmux error (no server, no binary) returns (nil, nil) — a machine without
// tmux is normal, and compass works fine without one. Rows it cannot read are
// skipped rather than failing the whole list.
func ListPanes(r Runner) ([]Pane, error) {
	out, err := r.Output("list-panes", "-a", "-F", paneFormat)
	if err != nil {
		return nil, nil
	}

	var panes []Pane
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		pane, ok := parsePane(line)
		if !ok {
			continue
		}
		// Sessions in a tmux group share their windows, so `list-panes -a`
		// reports every pane once per session in the group — the same pane id,
		// the same pid, under two or more targets. They are one pane holding
		// one claude, and compass must see one: handed both, it pairs a second
		// session to the second copy, and because winning a pane is what marks
		// a session live (M5), a long-dead session is dragged into the live
		// fleet wearing the mirror of a pane it is not in.
		//
		// The first address tmux gives wins, which keeps its own ordering — the
		// order the fleet groups by.
		if seen[pane.ID] {
			continue
		}
		seen[pane.ID] = true
		panes = append(panes, pane)
	}
	return panes, nil
}

// sanitizedRow matches a row whose separators tmux replaced with underscores
// (see parsePane). It anchors on the two fields that cannot be mistaken for
// anything else — the pane id "%5" and the numeric pid — which lets a session
// name contain underscores of its own. What follows is the command, which is a
// process name and has none in practice, and then the window name, which takes
// the whole rest of the line: "pixie_tuiZ" is a real window name and must
// survive intact.
var sanitizedRow = regexp.MustCompile(`^(.+?)_(%\d+)_(\d+)_([^_]*)_(.*)$`)

// parsePane reads one -F row. Anything that does not have the shape tmux was
// asked for is dropped: a half-written row must not become a half-true pane.
//
// tmux prints command output through its own sanitizer for a client that is not
// attached to a session — which is exactly compass in deck mode, in its own
// terminal tab — and that sanitizer turns the tabs paneFormat asks for into
// underscores. Rows arrive tab-separated from inside tmux and underscored from
// outside it, so both shapes are read here.
func parsePane(line string) (Pane, bool) {
	f := strings.SplitN(line, "\t", paneFields)
	if len(f) == 1 {
		if m := sanitizedRow.FindStringSubmatch(line); m != nil {
			f = m[1:]
		}
	}
	if len(f) != paneFields {
		return Pane{}, false
	}
	pid, err := strconv.Atoi(f[2])
	if err != nil {
		return Pane{}, false
	}
	if f[0] == "" || f[1] == "" {
		return Pane{}, false
	}
	return Pane{Target: f[0], ID: f[1], PID: pid, Command: f[3], Window: safeLabel(f[4])}, true
}

// safeLabel strips control characters out of text tmux hands back. A window
// name is whatever the user typed into `rename-window`, and ESC is a legal
// character there — compass draws that name into a TUI, so an escape sequence
// arriving from a pane title would repaint the deck. Nothing legible is lost:
// the names people give windows are words.
func safeLabel(s string) string {
	if strings.IndexFunc(s, unicode.IsControl) < 0 {
		return s
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// MapSessions pairs sessions to panes: a session matches a pane whose claude
// descendant's cwd equals the session's CWD.
//
// A cwd is not an identity — people run many sessions from one directory over
// time — so when several sessions claim one, the pane goes to the session that
// can actually be *in* it. A session that stopped writing before that pane's
// claude even started is not running in it, whatever its cwd says; it is only
// considered once every session that could be in the pane has one. Without
// that rule a long-dead session wins a live pane on a stale address, and —
// because winning a pane is what marks a session live (M5) — it then stays in
// the live fleet forever, wearing a mirror of somebody else's work.
//
// The rule ranks, it never vetoes: a session resumed a moment ago has not
// written anything since its pane started, and it is still that pane's session
// when nothing else competes for it.
//
// Returns SessionInfo.Key() → Pane. Keying by the transcript path rather than
// the session id is what keeps two same-id sessions from sharing one pane —
// the pane belongs to the transcript that won it (docs/dev/M6-CONTRACT.md).
func MapSessions(sessions []fleet.SessionInfo, panes []Pane, p Proc) map[string]Pane {
	out := make(map[string]Pane)
	if len(sessions) == 0 || len(panes) == 0 {
		return out
	}

	ordered := append([]Pane(nil), panes...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Target < ordered[j].Target })

	// One /proc walk per pane, not per session-pane pair.
	var found []claudePane
	byCwd := make(map[string][]claudePane)
	for _, pane := range ordered {
		cwd, pid, ok := ClaudeIn(p, pane.PID)
		if !ok {
			continue // a pane with no claude in it is not a location
		}
		cp := claudePane{pane: pane, cwd: cwd, since: p.StartTime(pid)}
		found = append(found, cp)
		byCwd[cwd] = append(byCwd[cwd], cp)
	}
	if len(found) == 0 {
		return out
	}

	// A session answers to two addresses: where it is now, and where it was
	// opened. /proc reports the second, so that is usually the one that meets a
	// pane — but a session resumed somewhere new has the opposite shape, and
	// listing both costs nothing.
	contenders := make(map[string][]fleet.SessionInfo, len(byCwd))
	for _, s := range sessions {
		for _, cwd := range addressesOf(s) {
			if _, ok := byCwd[cwd]; ok {
				contenders[cwd] = append(contenders[cwd], s)
			}
		}
	}
	for cwd := range contenders {
		sort.SliceStable(contenders[cwd], func(i, j int) bool {
			a, b := contenders[cwd][i], contenders[cwd][j]
			if !a.LastEventAt.Equal(b.LastEventAt) {
				return a.LastEventAt.After(b.LastEventAt)
			}
			return a.Key() < b.Key()
		})
	}

	// Youngest claude first. Offering panes in Target order instead lets an old
	// pane eat the one live session before a young pane — which competes for
	// the same session and has far fewer candidates — ever gets asked, and the
	// dead session then lands on the young pane: exactly the weld this rule
	// exists to prevent. The youngest pane is the most constrained, so it
	// chooses first. A pane whose claude age is unknown constrains nothing and
	// sorts last, which leaves the all-unknown case in Target order.
	byAge := append([]claudePane(nil), found...)
	sort.SliceStable(byAge, func(i, j int) bool { return byAge[j].since.Before(byAge[i].since) })

	taken := map[string]bool{}
	claim := func(cp claudePane, plausible bool) bool {
		for _, s := range contenders[cp.cwd] {
			if taken[s.Key()] {
				continue
			}
			if plausible && !couldBeIn(s, cp) {
				continue
			}
			out[s.Key()], taken[s.Key()] = cp.pane, true
			return true
		}
		return false
	}

	// Both passes run across every pane before the next begins: a pane that
	// could still be paired plausibly must not lose its session to some other
	// pane's fallback.
	var free []claudePane
	for _, cp := range byAge {
		if !claim(cp, true) {
			free = append(free, cp)
		}
	}
	for _, cp := range free {
		claim(cp, false)
	}
	return out
}

// addressesOf is where a session may be found: where it is now, and where it
// was opened. They are the same until it changes directory.
func addressesOf(s fleet.SessionInfo) []string {
	switch {
	case s.CWD == "" && s.OriginCWD == "":
		return nil
	case s.OriginCWD == "" || s.OriginCWD == s.CWD:
		return []string{s.CWD}
	case s.CWD == "":
		return []string{s.OriginCWD}
	default:
		// The origin first: a claude process keeps its launch directory, so
		// that is the address /proc will be reporting.
		return []string{s.OriginCWD, s.CWD}
	}
}

// claudePane is a pane and the age of the claude running in it. A zero `since`
// means /proc would not say, which is never held against anyone.
type claudePane struct {
	pane  Pane
	cwd   string // the cwd of the claude inside it, as /proc reports it
	since time.Time
}

// procSlack forgives the last tick of arithmetic between a transcript's own
// clock and boot-time-plus-jiffies. The gap this decides is hours or days.
const procSlack = 2 * time.Second

// couldBeIn reports whether a session can be the one running in a pane: it
// must have spoken since that pane's claude started. An unreadable start time
// says nothing, so it disqualifies nobody.
//
// A start time that is badly wrong — a kernel whose USER_HZ is not the 100
// assumed in StartTime — scales with uptime, pushes every pane into the future
// and makes every session implausible everywhere. That much is self-limiting:
// pass 1 assigns nothing, pass 2 assigns by recency, and the pairing is what
// it was before this rule existed.
//
// A uniform *offset* is not so kind. A btime that jumps — a clock step, a
// resumed VM snapshot — inflates every `since` equally, which can push the
// young panes past every session while leaving the old ones reachable, and
// pass 1 will then pair the wrong way round. Nothing here detects that, and a
// per-pane clamp would not help: the skew is in the anchor, not the pane.
func couldBeIn(s fleet.SessionInfo, cp claudePane) bool {
	if cp.since.IsZero() {
		return true
	}
	// A session with no time at all is NOT treated as plausible with
	// everything. It sorts last among contenders, so calling it plausible
	// would let it take the only pane from a session ranked above it — and
	// leave that one with nothing, which is the one thing this rule may never
	// do. Ranked last and implausible, it waits for pass 2 instead.
	// (Discovery seeds LastEventAt from the file's mtime, so in practice this
	// is unreachable; it is the asymmetry that is worth not having.)
	return !s.LastEventAt.Before(cp.since.Add(-procSlack))
}
