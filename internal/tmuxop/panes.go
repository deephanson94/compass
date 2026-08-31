package tmuxop

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/deephanson94/compass/internal/fleet"
)

// Pane is one pane on the tmux server.
type Pane struct {
	Target  string // "dev:1.0" (session:window.pane)
	ID      string // "%5"
	PID     int
	Path    string // pane_current_path
	Command string // pane_current_command
}

// paneFormat is the -F string ListPanes asks tmux for. The separator is a tab
// because working directories routinely contain spaces.
const paneFormat = "#{session_name}:#{window_index}.#{pane_index}\t#{pane_id}\t#{pane_pid}\t#{pane_current_path}\t#{pane_current_command}"

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
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		pane, ok := parsePane(line)
		if !ok {
			continue
		}
		panes = append(panes, pane)
	}
	return panes, nil
}

// sanitizedRow matches a row whose separators tmux replaced with underscores
// (see parsePane). It anchors on the two fields that cannot be mistaken for
// anything else — the pane id "%5" and the numeric pid — and then splits the
// tail at its last underscore, so a path containing underscores still lands in
// Path. Only a command containing one can smudge that split, and Command is a
// label; Target, ID and PID, which everything else hangs off, stay exact.
var sanitizedRow = regexp.MustCompile(`^(.+?)_(%\d+)_(\d+)_(.*)_([^_]*)$`)

// parsePane reads one -F row. Anything that does not have the shape tmux was
// asked for is dropped: a half-written row must not become a half-true pane.
//
// tmux prints command output through its own sanitizer for a client that is not
// attached to a session — which is exactly compass in deck mode, in its own
// terminal tab — and that sanitizer turns the tabs paneFormat asks for into
// underscores. Rows arrive tab-separated from inside tmux and underscored from
// outside it, so both shapes are read here.
func parsePane(line string) (Pane, bool) {
	f := strings.Split(line, "\t")
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
	return Pane{Target: f[0], ID: f[1], PID: pid, Path: f[3], Command: f[4]}, true
}

// MapSessions pairs sessions to panes: a session matches a pane whose claude
// descendant's cwd equals the session's CWD. When several sessions share a cwd,
// they are paired to matching panes in order (sessions by LastEventAt desc,
// panes by Target asc); leftovers stay unmapped.
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
	byCwd := make(map[string][]Pane)
	for _, pane := range ordered {
		cwd, ok := ClaudeCwd(p, pane.PID)
		if !ok {
			continue // a pane with no claude in it is not a location
		}
		byCwd[cwd] = append(byCwd[cwd], pane)
	}
	if len(byCwd) == 0 {
		return out
	}

	ranked := append([]fleet.SessionInfo(nil), sessions...)
	sort.SliceStable(ranked, func(i, j int) bool {
		a, b := ranked[i], ranked[j]
		if !a.LastEventAt.Equal(b.LastEventAt) {
			return a.LastEventAt.After(b.LastEventAt)
		}
		return a.ID < b.ID
	})

	for _, s := range ranked {
		if s.CWD == "" {
			continue
		}
		free := byCwd[s.CWD]
		if len(free) == 0 {
			continue // no pane left at this cwd: the session stays unmapped
		}
		out[s.Key()] = free[0]
		byCwd[s.CWD] = free[1:]
	}
	return out
}
