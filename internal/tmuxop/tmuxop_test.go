package tmuxop_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/tmuxop"
)

// base keeps every timestamp in these tests off the wall clock.
var base = time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

func at(offset time.Duration) time.Time { return base.Add(offset) }

// ---------------------------------------------------------------- fakes

// fakeRunner stands in for the tmux binary: it records every arg vector it was
// handed and replays scripted results in call order.
type fakeRunner struct {
	calls   [][]string
	outputs [][]byte
	errs    []error
}

func (f *fakeRunner) Output(args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string(nil), args...))
	i := len(f.calls) - 1
	var out []byte
	var err error
	if i < len(f.outputs) {
		out = f.outputs[i]
	}
	if i < len(f.errs) {
		err = f.errs[i]
	}
	return out, err
}

func (f *fakeRunner) assertCalls(t *testing.T, want ...[]string) {
	t.Helper()
	if len(f.calls) != len(want) {
		t.Fatalf("runner saw %d call(s), want %d:\n%s", len(f.calls), len(want), f.dump())
	}
	for i := range want {
		if !reflect.DeepEqual(f.calls[i], want[i]) {
			t.Errorf("call %d args =\n  %q\nwant\n  %q", i, f.calls[i], want[i])
		}
	}
}

func (f *fakeRunner) dump() string {
	var b strings.Builder
	for i, c := range f.calls {
		fmt.Fprintf(&b, "  [%d] %q\n", i, c)
	}
	if b.Len() == 0 {
		return "  (no calls)\n"
	}
	return b.String()
}

// fakeProc is a process tree in three maps. Anything absent is simply unknown,
// exactly as an unreadable /proc entry would be.
type fakeProc struct {
	children map[int][]int
	comm     map[int]string
	cmdline  map[int]string
	cwd      map[int]string
}

// Children hands back a copy: the walker must never be able to reorder or
// truncate the tree it is reading.
func (p *fakeProc) Children(pid int) []int {
	return append([]int(nil), p.children[pid]...)
}

func (p *fakeProc) Comm(pid int) string    { return p.comm[pid] }
func (p *fakeProc) Cmdline(pid int) string { return p.cmdline[pid] }
func (p *fakeProc) Cwd(pid int) string     { return p.cwd[pid] }

// chain builds pid → pid+1 → … depth links below root, naming the deepest one.
func chain(root, depth int, leafComm, leafCwd string) *fakeProc {
	p := &fakeProc{
		children: map[int][]int{},
		comm:     map[int]string{root: "zsh"},
		cwd:      map[int]string{},
	}
	pid := root
	for i := 0; i < depth; i++ {
		p.children[pid] = []int{pid + 1}
		p.comm[pid+1] = "sh"
		pid++
	}
	p.comm[pid] = leafComm
	p.cwd[pid] = leafCwd
	return p
}

// ---------------------------------------------------------------- T27

// listPanesFormat is the -F argument the contract fixes. The fields are
// TAB-separated because the parser splits the output on tabs.
const listPanesFormat = "#{session_name}:#{window_index}.#{pane_index}\t#{pane_id}\t#{pane_pid}\t#{pane_current_path}\t#{pane_current_command}"

func listPanesArgs() []string {
	return []string{"list-panes", "-a", "-F", listPanesFormat}
}

// T27 — ListPanes issues exactly the contract's command and parses well-formed
// rows, including a pane_current_path containing spaces.
func TestT27ListPanesParsesOutput(t *testing.T) {
	out := strings.Join([]string{
		"dev:1.0\t%5\t12345\t/home/user/compass\tclaude",
		"dev:1.1\t%6\t12346\t/home/user/my project/src\tzsh",
		"work:0.0\t%9\t2\t/\tnode server.js",
	}, "\n") + "\n"

	r := &fakeRunner{outputs: [][]byte{[]byte(out)}}
	panes, err := tmuxop.ListPanes(r)
	if err != nil {
		t.Fatalf("ListPanes: unexpected error %v", err)
	}

	// The exact command the contract specifies, and only it.
	r.assertCalls(t, listPanesArgs())

	want := []tmuxop.Pane{
		{Target: "dev:1.0", ID: "%5", PID: 12345, Path: "/home/user/compass", Command: "claude"},
		{Target: "dev:1.1", ID: "%6", PID: 12346, Path: "/home/user/my project/src", Command: "zsh"},
		{Target: "work:0.0", ID: "%9", PID: 2, Path: "/", Command: "node server.js"},
	}
	if !reflect.DeepEqual(panes, want) {
		t.Errorf("ListPanes =\n  %+v\nwant\n  %+v", panes, want)
	}
}

// Malformed rows are dropped; the well-formed ones around them survive.
func TestT27ListPanesSkipsMalformedRows(t *testing.T) {
	out := strings.Join([]string{
		"dev:1.0\t%5\t12345\t/home/user/compass\tclaude",
		"this row has no tabs at all",
		"dev:1.1\t%6\t12346", // too few fields
		"",                   // blank line
		"   ",                // whitespace line
		"dev:2.0\t%7\t777\t/w\tvim",
	}, "\n")

	r := &fakeRunner{outputs: [][]byte{[]byte(out)}}
	panes, err := tmuxop.ListPanes(r)
	if err != nil {
		t.Fatalf("ListPanes: unexpected error %v", err)
	}
	want := []tmuxop.Pane{
		{Target: "dev:1.0", ID: "%5", PID: 12345, Path: "/home/user/compass", Command: "claude"},
		{Target: "dev:2.0", ID: "%7", PID: 777, Path: "/w", Command: "vim"},
	}
	if !reflect.DeepEqual(panes, want) {
		t.Errorf("ListPanes =\n  %+v\nwant\n  %+v", panes, want)
	}
}

// A machine with no tmux server is normal, not an error.
func TestT27ListPanesTmuxErrorIsNotAnError(t *testing.T) {
	r := &fakeRunner{
		outputs: [][]byte{[]byte("no server running on /tmp/tmux-1000/default\n")},
		errs:    []error{errors.New("exit status 1")},
	}
	panes, err := tmuxop.ListPanes(r)
	if err != nil {
		t.Errorf("ListPanes err = %v, want nil: a machine without tmux is normal", err)
	}
	if panes != nil {
		t.Errorf("ListPanes = %+v, want nil", panes)
	}
	r.assertCalls(t, listPanesArgs())
}

func TestT27ListPanesEmptyOutput(t *testing.T) {
	for _, out := range []string{"", "\n", "   \n"} {
		r := &fakeRunner{outputs: [][]byte{[]byte(out)}}
		panes, err := tmuxop.ListPanes(r)
		if err != nil {
			t.Errorf("ListPanes(%q): err = %v, want nil", out, err)
		}
		if len(panes) != 0 {
			t.Errorf("ListPanes(%q) = %+v, want no panes", out, panes)
		}
	}
}

// ---------------------------------------------------------------- T28

// T28 — the claude process usually sits a couple of levels under the pane's
// shell; ClaudeCwd finds it and reports its cwd.
func TestT28ClaudeCwdFindsClaudeTwoLevelsDeep(t *testing.T) {
	p := &fakeProc{
		children: map[int][]int{
			100: {200, 201},
			201: {300},
			200: {},
			300: {},
		},
		comm: map[int]string{100: "zsh", 200: "node", 201: "bash", 300: "claude"},
		cwd:  map[int]string{100: "/home/user", 200: "/tmp", 300: "/home/user/compass"},
	}

	cwd, ok := tmuxop.ClaudeCwd(p, 100)
	if !ok {
		t.Fatalf("ClaudeCwd = (%q, false), want it to find the claude grandchild", cwd)
	}
	if cwd != "/home/user/compass" {
		t.Errorf("ClaudeCwd = %q, want %q", cwd, "/home/user/compass")
	}
}

// No claude anywhere in the subtree: not found.
func TestT28ClaudeCwdAbsent(t *testing.T) {
	p := &fakeProc{
		children: map[int][]int{100: {200, 201}, 201: {300}},
		comm:     map[int]string{100: "zsh", 200: "node", 201: "bash", 300: "vim"},
		cwd:      map[int]string{300: "/home/user/compass"},
	}
	if cwd, ok := tmuxop.ClaudeCwd(p, 100); ok {
		t.Errorf("ClaudeCwd = (%q, true), want (\"\", false)", cwd)
	} else if cwd != "" {
		t.Errorf("ClaudeCwd cwd = %q, want %q when not found", cwd, "")
	}

	// An empty tree, and a pid nothing knows about.
	empty := &fakeProc{children: map[int][]int{}, comm: map[int]string{}, cwd: map[int]string{}}
	if _, ok := tmuxop.ClaudeCwd(empty, 4242); ok {
		t.Errorf("ClaudeCwd on an unknown pid returned ok=true")
	}
}

// The walk is bounded at depth 6.
func TestT28ClaudeCwdDepthBound(t *testing.T) {
	tests := []struct {
		depth   int
		wantOK  bool
		wantCwd string
	}{
		{1, true, "/w/d1"},
		{5, true, "/w/d5"},
		{6, true, "/w/d6"}, // the boundary is inclusive: depth ≤ 6
		{7, false, ""},
		{9, false, ""},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("depth %d", tc.depth), func(t *testing.T) {
			p := chain(100, tc.depth, "claude", fmt.Sprintf("/w/d%d", tc.depth))
			cwd, ok := tmuxop.ClaudeCwd(p, 100)
			if ok != tc.wantOK {
				t.Fatalf("ClaudeCwd at depth %d = (%q, %v), want ok=%v", tc.depth, cwd, ok, tc.wantOK)
			}
			if ok && cwd != tc.wantCwd {
				t.Errorf("ClaudeCwd = %q, want %q", cwd, tc.wantCwd)
			}
			if !ok && cwd != "" {
				t.Errorf("ClaudeCwd cwd = %q, want %q when not found", cwd, "")
			}
		})
	}
}

// Breadth-first: the shallowest claude wins, and among equals the first child.
func TestT28ClaudeCwdIsBreadthFirst(t *testing.T) {
	t.Run("shallower wins", func(t *testing.T) {
		p := &fakeProc{
			children: map[int][]int{100: {200, 201}, 200: {300}, 300: {400}},
			comm:     map[int]string{100: "zsh", 200: "bash", 300: "sh", 400: "claude", 201: "claude"},
			cwd:      map[int]string{400: "/w/deep", 201: "/w/shallow"},
		}
		cwd, ok := tmuxop.ClaudeCwd(p, 100)
		if !ok || cwd != "/w/shallow" {
			t.Errorf("ClaudeCwd = (%q, %v), want the depth-1 claude at /w/shallow", cwd, ok)
		}
	})

	t.Run("siblings in child order", func(t *testing.T) {
		p := &fakeProc{
			children: map[int][]int{100: {201, 202}},
			comm:     map[int]string{100: "zsh", 201: "claude", 202: "claude"},
			cwd:      map[int]string{201: "/w/first", 202: "/w/second"},
		}
		cwd, ok := tmuxop.ClaudeCwd(p, 100)
		if !ok || cwd != "/w/first" {
			t.Errorf("ClaudeCwd = (%q, %v), want the first-listed child at /w/first", cwd, ok)
		}
	})
}

// A cyclic tree (a fake can build one; so can a racy /proc read) must still
// terminate — the depth bound guarantees it.
func TestT28ClaudeCwdTerminatesOnCycles(t *testing.T) {
	p := &fakeProc{
		children: map[int][]int{100: {200}, 200: {100, 300}, 300: {200}},
		comm:     map[int]string{100: "zsh", 200: "bash", 300: "claude"},
		cwd:      map[int]string{300: "/w/cycle"},
	}
	done := make(chan struct{})
	var cwd string
	var ok bool
	go func() {
		cwd, ok = tmuxop.ClaudeCwd(p, 100)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("ClaudeCwd did not terminate on a cyclic process tree")
	}
	if !ok || cwd != "/w/cycle" {
		t.Errorf("ClaudeCwd = (%q, %v), want (%q, true)", cwd, ok, "/w/cycle")
	}
}

// ---------------------------------------------------------------- T29

// mapProc is the tree behind the T29 panes: each pane pid owns one claude child.
func mapProc() *fakeProc {
	return &fakeProc{
		children: map[int][]int{
			500: {501}, // pane dev:1.0
			900: {901}, // pane dev:2.0
			300: {301}, // pane dev:0.0 — decoy
			700: {701}, // pane dev:3.0 — claude elsewhere
			800: {},    // pane dev:4.0 — no claude at all
		},
		comm: map[int]string{
			500: "zsh", 501: "claude",
			900: "zsh", 901: "claude",
			300: "zsh", 301: "claude",
			700: "zsh", 701: "claude",
			800: "vim",
		},
		cwd: map[int]string{
			501: "/w/app",
			901: "/w/app",
			301: "/w/elsewhere", // its PANE path says /w/app; its claude disagrees
			701: "/w/elsewhere",
		},
	}
}

// sessionAt builds a fixture session. Identity is the transcript path (M6), so
// every fixture carries one — two sessions with no path would collide in the
// returned map under the same empty key.
func sessionAt(id, cwd string, last time.Duration) fleet.SessionInfo {
	return fleet.SessionInfo{
		ID: id, TranscriptPath: keyOf(id), CWD: cwd, LastEventAt: at(last),
	}
}

// keyOf is the key MapSessions returns a pairing under: the transcript path.
func keyOf(id string) string { return "/x/projects/-w-app/" + id + ".jsonl" }

// T29 — two sessions share a cwd and two panes match: newest session (by
// LastEventAt) takes the lowest Target. A session with no matching pane is
// simply absent from the map.
func TestT29MapSessionsDeterministicPairing(t *testing.T) {
	sessions := []fleet.SessionInfo{
		sessionAt("s-old", "/w/app", 1*time.Minute),
		sessionAt("s-new", "/w/app", 10*time.Minute),
		sessionAt("s-lonely", "/w/nowhere", 5*time.Minute),
	}
	// Deliberately out of Target order, and with a decoy whose pane_current_path
	// matches but whose claude descendant is somewhere else entirely.
	panes := []tmuxop.Pane{
		{Target: "dev:2.0", ID: "%9", PID: 900, Path: "/w/app", Command: "claude"},
		{Target: "dev:0.0", ID: "%1", PID: 300, Path: "/w/app", Command: "claude"},
		{Target: "dev:1.0", ID: "%5", PID: 500, Path: "/w/app", Command: "claude"},
		{Target: "dev:3.0", ID: "%7", PID: 700, Path: "/w/app", Command: "claude"},
		{Target: "dev:4.0", ID: "%8", PID: 800, Path: "/w/app", Command: "vim"},
	}

	got := tmuxop.MapSessions(sessions, panes, mapProc())

	if len(got) != 2 {
		t.Fatalf("MapSessions returned %d pairs, want 2: %+v", len(got), got)
	}
	if got[keyOf("s-new")].Target != "dev:1.0" {
		t.Errorf("s-new → %q, want dev:1.0 (newest session takes the lowest matching Target)", got[keyOf("s-new")].Target)
	}
	if got[keyOf("s-old")].Target != "dev:2.0" {
		t.Errorf("s-old → %q, want dev:2.0", got[keyOf("s-old")].Target)
	}
	if _, ok := got[keyOf("s-lonely")]; ok {
		t.Errorf("s-lonely → %+v, want it left unmapped", got[keyOf("s-lonely")])
	}
	// The whole Pane travels, not just the target.
	if got[keyOf("s-new")].ID != "%5" || got[keyOf("s-new")].PID != 500 {
		t.Errorf("s-new → %+v, want the full pane record for %%5", got[keyOf("s-new")])
	}
}

// The pairing must not depend on map or slice iteration luck.
func TestT29MapSessionsIsDeterministic(t *testing.T) {
	sessions := []fleet.SessionInfo{
		sessionAt("s-old", "/w/app", 1*time.Minute),
		sessionAt("s-new", "/w/app", 10*time.Minute),
	}
	panes := []tmuxop.Pane{
		{Target: "dev:2.0", ID: "%9", PID: 900, Path: "/w/app"},
		{Target: "dev:1.0", ID: "%5", PID: 500, Path: "/w/app"},
	}
	for i := 0; i < 50; i++ {
		got := tmuxop.MapSessions(sessions, panes, mapProc())
		if got[keyOf("s-new")].ID != "%5" || got[keyOf("s-old")].ID != "%9" {
			t.Fatalf("run %d: s-new → %q, s-old → %q; want %%5 and %%9 every time",
				i, got[keyOf("s-new")].ID, got[keyOf("s-old")].ID)
		}
	}
}

// More sessions than panes: the leftovers stay unmapped, and it is the oldest
// ones that lose.
func TestT29MapSessionsLeftoversStayUnmapped(t *testing.T) {
	sessions := []fleet.SessionInfo{
		sessionAt("s1", "/w/app", 1*time.Minute),
		sessionAt("s2", "/w/app", 2*time.Minute),
		sessionAt("s3", "/w/app", 3*time.Minute),
	}
	panes := []tmuxop.Pane{{Target: "dev:1.0", ID: "%5", PID: 500, Path: "/w/app"}}

	got := tmuxop.MapSessions(sessions, panes, mapProc())
	if len(got) != 1 {
		t.Fatalf("MapSessions returned %d pairs, want 1: %+v", len(got), got)
	}
	if _, ok := got[keyOf("s3")]; !ok {
		t.Errorf("MapSessions = %+v, want the newest session (s3) to win the only pane", got)
	}
}

// Nothing to pair: an empty (or nil) map, never a panic.
func TestT29MapSessionsEmptyInputs(t *testing.T) {
	if got := tmuxop.MapSessions(nil, nil, mapProc()); len(got) != 0 {
		t.Errorf("MapSessions(nil, nil) = %+v, want empty", got)
	}
	sessions := []fleet.SessionInfo{{ID: "s1", CWD: "/w/app", LastEventAt: at(0)}}
	if got := tmuxop.MapSessions(sessions, nil, mapProc()); len(got) != 0 {
		t.Errorf("MapSessions with no panes = %+v, want empty", got)
	}
	panes := []tmuxop.Pane{{Target: "dev:1.0", ID: "%5", PID: 500, Path: "/w/app"}}
	if got := tmuxop.MapSessions(nil, panes, mapProc()); len(got) != 0 {
		t.Errorf("MapSessions with no sessions = %+v, want empty", got)
	}
	// A session with no cwd matches nothing, even a pane whose claude is unreadable.
	blank := []fleet.SessionInfo{{ID: "s-blank", CWD: "", LastEventAt: at(0)}}
	if got := tmuxop.MapSessions(blank, panes, mapProc()); len(got) != 0 {
		t.Errorf("MapSessions with a cwd-less session = %+v, want empty", got)
	}
}

// ---------------------------------------------------------------- T30

// T30 — Capture's arg vector is fixed by the contract, and it returns the raw
// screen text untouched.
func TestT30CaptureArgsAndOutput(t *testing.T) {
	const frame = "\x1b[32m✔ 12 passed\x1b[0m\n\x1b[2m~/compass\x1b[0m $ "
	r := &fakeRunner{outputs: [][]byte{[]byte(frame)}}

	got, err := tmuxop.Capture(r, "%5")
	if err != nil {
		t.Fatalf("Capture: unexpected error %v", err)
	}
	r.assertCalls(t, []string{"capture-pane", "-p", "-e", "-J", "-t", "%5"})
	if got != frame {
		t.Errorf("Capture = %q, want the raw frame %q", got, frame)
	}
}

func TestT30CaptureReportsErrors(t *testing.T) {
	r := &fakeRunner{
		outputs: [][]byte{[]byte("can't find pane: %5\n")},
		errs:    []error{errors.New("exit status 1")},
	}
	if _, err := tmuxop.Capture(r, "%5"); err == nil {
		t.Errorf("Capture err = nil, want the tmux failure reported")
	}
	r.assertCalls(t, []string{"capture-pane", "-p", "-e", "-J", "-t", "%5"})
}

// T30 — Reveal selects the window, then the pane, in that order.
func TestT30RevealArgs(t *testing.T) {
	tests := []struct {
		target     string
		paneID     string
		wantWindow string
	}{
		{"dev:1.0", "%5", "dev:1"},
		{"dev:12.3", "%7", "dev:12"},
		{"work:0.0", "%1", "work:0"},
	}
	for _, tc := range tests {
		t.Run(tc.target, func(t *testing.T) {
			r := &fakeRunner{}
			if err := tmuxop.Reveal(r, tc.target, tc.paneID); err != nil {
				t.Fatalf("Reveal: unexpected error %v", err)
			}
			r.assertCalls(t,
				[]string{"select-window", "-t", tc.wantWindow},
				[]string{"select-pane", "-t", tc.paneID},
			)
		})
	}
}

func TestT30RevealReportsErrors(t *testing.T) {
	t.Run("select-window fails", func(t *testing.T) {
		r := &fakeRunner{errs: []error{errors.New("can't find window: dev:1")}}
		if err := tmuxop.Reveal(r, "dev:1.0", "%5"); err == nil {
			t.Errorf("Reveal err = nil, want the select-window failure reported")
		}
	})

	t.Run("select-pane fails", func(t *testing.T) {
		r := &fakeRunner{errs: []error{nil, errors.New("can't find pane: %5")}}
		if err := tmuxop.Reveal(r, "dev:1.0", "%5"); err == nil {
			t.Errorf("Reveal err = nil, want the select-pane failure reported")
		}
	})
}

// ---------------------------------------------------------------- seams

// The fakes in this file must satisfy the contract's interfaces — if they stop
// compiling, the seams moved.
var (
	_ tmuxop.Runner = (*fakeRunner)(nil)
	_ tmuxop.Proc   = (*fakeProc)(nil)
	_ tmuxop.Runner = &tmuxop.RealRunner{}
	_ tmuxop.Proc   = &tmuxop.RealProc{}
)

// ---------------------------------------------------------------- T28b

// A natively-installed CLI answers to its own name; an npm install is a Node
// script, so it runs as `node …/claude-code/cli.js` and only argv says what it
// is. Both are sessions (found in the field: every session read "(no pane)" on
// an npm install because only the native shape was recognised).
func TestClaudeCwdFindsEveryInstallShape(t *testing.T) {
	cases := []struct {
		name    string
		comm    string
		cmdline string
		want    bool
	}{
		{"native binary", "claude", "claude --resume", true},
		{"npm global", "node",
			"node /home/u/.nvm/versions/node/v22.4.0/lib/node_modules/@anthropic-ai/claude-code/cli.js", true},
		{"npm local", "node", "node /w/node_modules/.bin/claude --model opus", true},
		{"bun", "bun", "bun /w/node_modules/@anthropic-ai/claude-code/cli.js", true},
		{"deno", "deno", "deno run -A /w/claude-code/cli.js", true},
		{"exec'd wrapper", "sh", "/usr/local/bin/claude", true},

		// Nothing that merely mentions the word is a session.
		{"editor holding a note", "vim", "vim claude-notes.md", false},
		{"unrelated node app", "node", "node server.js", false},
		{"grep for the word", "grep", "grep -r claude .", false},
		{"shell alone", "zsh", "-zsh", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakeProc{
				children: map[int][]int{100: {101}},
				comm:     map[int]string{100: "zsh", 101: tc.comm},
				cmdline:  map[int]string{100: "-zsh", 101: tc.cmdline},
				cwd:      map[int]string{101: "/w/api"},
			}
			cwd, ok := tmuxop.ClaudeCwd(p, 100)
			if ok != tc.want {
				t.Fatalf("ClaudeCwd found=%v (cwd %q), want found=%v — comm %q, argv %q",
					ok, cwd, tc.want, tc.comm, tc.cmdline)
			}
			if tc.want && cwd != "/w/api" {
				t.Errorf("cwd = %q, want /w/api", cwd)
			}
		})
	}
}
