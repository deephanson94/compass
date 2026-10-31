package tmuxop

import (
	"fmt"
	"strconv"
	"strings"
)

// Sessions lists the tmux sessions on the server, in tmux's own order. A
// tmux error (no server, no binary) is an empty list, as for ListPanes.
func Sessions(r Runner) []string {
	out, err := r.Output("list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if line != "" {
			names = append(names, line)
		}
	}
	return names
}

// Place is where Open puts new work: a window in a session, a pane split
// beside one, or a tmux session of its own.
type Place int

const (
	PlaceWindow Place = iota
	PlacePane
	PlaceSession
)

// Places is every Place, in the order a panel offers them: the default first.
var Places = []Place{PlaceWindow, PlacePane, PlaceSession}

func (p Place) String() string {
	switch p {
	case PlacePane:
		return "pane"
	case PlaceSession:
		return "session"
	}
	return "window"
}

// ParsePlace reads a Place by its String.
func ParsePlace(s string) (Place, bool) {
	for _, p := range Places {
		if p.String() == s {
			return p, true
		}
	}
	return PlaceWindow, false
}

// openFormat is where the work landed: the pane id first, since it never
// holds a space, so the target after it may (a session name can). Not a
// tab: tmux 3.4 prints a control character in -F output as "_".
const openFormat = "#{pane_id} #{session_name}:#{window_index}.#{pane_index}"

// Open puts new work on the tmux server and returns its one pane. It is the
// deck's third write, and like the other two it runs only from a keypress
// (or from `compass new`, which is one). It creates what it was asked for
// and nothing more, and once it is open compass treats it like any other
// pane: it never closes, renames or rearranges it.
//
// target says where, by place: for a window, the existing session it opens
// in; for a pane, the pane it splits beside (side by side); for a session,
// nothing — the session is new, and named name.
//
// -d leaves the user's focus where it was; -P -F prints where the work
// landed, so the caller can find the pane without waiting for the next
// list-panes. dir is the working directory, name the window's (and a new
// session's) name; "" lets tmux name it. argv is the command, passed to tmux
// as separate arguments after "--": tmux runs two or more of them directly,
// with no shell between, so a prompt's quotes and dollars arrive as typed.
// No argv is the user's default shell.
func Open(r Runner, place Place, target, dir, name string, argv []string) (Pane, error) {
	return OpenWith(r, place, target, dir, name, nil, argv)
}

// OpenWith is Open with environment variables set in the new pane, one
// "KEY=value" each, as tmux's own -e sets them: in the process tmux starts,
// and nowhere else. A watcher's restricted claude needs a few of its own
// (a model provider, a region) that the user's shell does not carry.
func OpenWith(r Runner, place Place, target, dir, name string, env, argv []string) (Pane, error) {
	var args []string
	switch place {
	case PlaceWindow:
		if target == "" {
			return Pane{}, fmt.Errorf("no tmux session to open a window in")
		}
		args = []string{"new-window", "-d", "-P", "-F", openFormat, "-t", "=" + target + ":"}
	case PlacePane:
		if target == "" {
			return Pane{}, fmt.Errorf("no pane to split beside")
		}
		args = []string{"split-window", "-d", "-h", "-P", "-F", openFormat, "-t", target}
		name = "" // a split joins a window that already has its name
	case PlaceSession:
		if name == "" {
			return Pane{}, fmt.Errorf("a new tmux session needs a name")
		}
		args = []string{"new-session", "-d", "-P", "-F", openFormat, "-s", name}
	default:
		return Pane{}, fmt.Errorf("no such place: %d", place)
	}
	if dir != "" {
		args = append(args, "-c", dir)
	}
	if name != "" {
		args = append(args, "-n", name)
	}
	for _, kv := range env {
		args = append(args, "-e", kv)
	}
	if len(argv) > 0 {
		args = append(args, "--")
		args = append(args, argv...)
	}
	out, err := r.Output(args...)
	if err != nil {
		return Pane{}, err
	}
	id, at, ok := strings.Cut(strings.TrimSpace(string(out)), " ")
	if !ok || !strings.HasPrefix(id, "%") {
		return Pane{}, fmt.Errorf("tmux said %q, not where the %s opened", strings.TrimSpace(string(out)), place)
	}
	return Pane{Target: at, ID: id, Window: name}, nil
}

// LastActivePane is the pane a person was looking at when they last left
// tmux: the active pane of the active window of the session attached most
// recently. Outside tmux, compass hands the terminal over (Enter, t, a
// launch that goes there) and gets it back when the person detaches,
// wherever their own keys took them meanwhile; this is where that was.
// "" when tmux cannot say (no server, nothing ever attached).
func LastActivePane(r Runner) string {
	out, err := r.Output("list-panes", "-a", "-F", "#{session_last_attached} #{window_active}#{pane_active} #{pane_id}")
	if err != nil {
		return ""
	}
	best, pane := int64(0), ""
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 3 || f[1] != "11" {
			continue
		}
		at, err := strconv.ParseInt(f[0], 10, 64)
		if err != nil || at <= best {
			continue
		}
		best, pane = at, f[2]
	}
	return pane
}
