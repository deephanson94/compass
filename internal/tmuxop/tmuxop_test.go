package tmuxop_test

import (
	"errors"
	"fmt"
	"os"
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
	started  map[int]time.Time
}

// Children hands back a copy: the walker must never be able to reorder or
// truncate the tree it is reading.
func (p *fakeProc) Children(pid int) []int {
	return append([]int(nil), p.children[pid]...)
}

func (p *fakeProc) Comm(pid int) string    { return p.comm[pid] }
func (p *fakeProc) Cmdline(pid int) string { return p.cmdline[pid] }
func (p *fakeProc) Cwd(pid int) string     { return p.cwd[pid] }

// StartTime defaults to the zero time — unreadable — which is what most of
// these fixtures want: a fake that says nothing about age must not change how
// anything is paired.
func (p *fakeProc) StartTime(pid int) time.Time { return p.started[pid] }

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
const listPanesFormat = "#{session_name}:#{window_index}.#{pane_index}\t#{pane_id}\t#{pane_pid}\t#{pane_current_command}\t#{window_name}"

func listPanesArgs() []string {
	return []string{"list-panes", "-a", "-F", listPanesFormat}
}

// T27 — ListPanes issues exactly the contract's command and parses well-formed
// rows, including a pane_current_path containing spaces.
func TestT27ListPanesParsesOutput(t *testing.T) {
	out := strings.Join([]string{
		"dev:1.0\t%5\t12345\tclaude\tcode",
		"dev:1.1\t%6\t12346\tzsh\tcode",
		"work:0.0\t%9\t2\tnode server.js\tsrv",
	}, "\n") + "\n"

	r := &fakeRunner{outputs: [][]byte{[]byte(out)}}
	panes, err := tmuxop.ListPanes(r)
	if err != nil {
		t.Fatalf("ListPanes: unexpected error %v", err)
	}

	// The exact command the contract specifies, and only it.
	r.assertCalls(t, listPanesArgs())

	want := []tmuxop.Pane{
		{Target: "dev:1.0", ID: "%5", PID: 12345, Command: "claude", Window: "code"},
		{Target: "dev:1.1", ID: "%6", PID: 12346, Command: "zsh", Window: "code"},
		{Target: "work:0.0", ID: "%9", PID: 2, Command: "node server.js", Window: "srv"},
	}
	if !reflect.DeepEqual(panes, want) {
		t.Errorf("ListPanes =\n  %+v\nwant\n  %+v", panes, want)
	}
}

// Malformed rows are dropped; the well-formed ones around them survive.
func TestT27ListPanesSkipsMalformedRows(t *testing.T) {
	out := strings.Join([]string{
		"dev:1.0\t%5\t12345\tclaude\tcode",
		"this row has no tabs at all",
		"dev:1.1\t%6\t12346", // too few fields
		"",                   // blank line
		"   ",                // whitespace line
		"dev:2.0\t%7\t777\tvim\tedit",
	}, "\n")

	r := &fakeRunner{outputs: [][]byte{[]byte(out)}}
	panes, err := tmuxop.ListPanes(r)
	if err != nil {
		t.Fatalf("ListPanes: unexpected error %v", err)
	}
	want := []tmuxop.Pane{
		{Target: "dev:1.0", ID: "%5", PID: 12345, Command: "claude", Window: "code"},
		{Target: "dev:2.0", ID: "%7", PID: 777, Command: "vim", Window: "edit"},
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
		{Target: "dev:2.0", ID: "%9", PID: 900, Command: "claude"},
		{Target: "dev:0.0", ID: "%1", PID: 300, Command: "claude"},
		{Target: "dev:1.0", ID: "%5", PID: 500, Command: "claude"},
		{Target: "dev:3.0", ID: "%7", PID: 700, Command: "claude"},
		{Target: "dev:4.0", ID: "%8", PID: 800, Command: "vim"},
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
		{Target: "dev:2.0", ID: "%9", PID: 900},
		{Target: "dev:1.0", ID: "%5", PID: 500},
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
	panes := []tmuxop.Pane{{Target: "dev:1.0", ID: "%5", PID: 500}}

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
	panes := []tmuxop.Pane{{Target: "dev:1.0", ID: "%5", PID: 500}}
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

// mapNow is "now" for the pairing tests below, expressed in the same base
// clock sessionAt uses. `ago` reads the way the fleet does — a session's last
// word was so long ago — which at(offset) alone does not.
var mapNow = 100 * time.Hour

func ago(d time.Duration) time.Duration { return mapNow - d }

// The dogfood bug, in one test. A cwd is not an identity: people run many
// sessions out of one directory over the months. A session that went quiet two
// days ago cannot be the one running in a pane whose claude started an hour
// ago — and it must not win that pane just because it also claims the address.
//
// This is the failure the screenshot showed: the dead session took the pane,
// which marked it live (M5), so the fleet showed a two-day-old session wearing
// a live mirror of somebody else's work — while the session actually in the
// pane sat in `elsewhere` with no pane at all.
func TestDeadSessionCannotClaimALivePane(t *testing.T) {
	p := &fakeProc{
		children: map[int][]int{700: {701}},
		comm:     map[int]string{700: "zsh", 701: "claude"},
		cwd:      map[int]string{701: "/w/app"},
		started:  map[int]time.Time{701: at(ago(time.Hour))}, // this claude began an hour ago
	}
	panes := []tmuxop.Pane{{Target: "tinker:0.0", ID: "%1", PID: 700, Command: "claude"}}

	// The dead one is listed first and would win any first-match scan.
	sessions := []fleet.SessionInfo{
		sessionAt("s-dead", "/w/app", ago(48*time.Hour)),
		sessionAt("s-live", "/w/app", ago(25*time.Second)),
	}

	got := tmuxop.MapSessions(sessions, panes, p)
	if _, ok := got[keyOf("s-dead")]; ok {
		t.Errorf("a session two days quiet took a pane whose claude started an hour ago: %+v",
			got[keyOf("s-dead")])
	}
	if got[keyOf("s-live")].Target != "tinker:0.0" {
		t.Errorf("the session actually in the pane got %+v, want tinker:0.0", got[keyOf("s-live")])
	}
}

// But the rule ranks, it never vetoes. A session resumed a moment ago has not
// written a line since its pane's claude started, and it is still that pane's
// session when nothing else is competing for it.
func TestResumedSessionKeepsItsPane(t *testing.T) {
	p := &fakeProc{
		children: map[int][]int{700: {701}},
		comm:     map[int]string{700: "zsh", 701: "claude"},
		cwd:      map[int]string{701: "/w/app"},
		started:  map[int]time.Time{701: at(ago(time.Minute))}, // just launched
	}
	panes := []tmuxop.Pane{{Target: "dev:0.0", ID: "%1", PID: 700, Command: "claude"}}
	sessions := []fleet.SessionInfo{sessionAt("s-resumed", "/w/app", ago(3*time.Hour))}

	got := tmuxop.MapSessions(sessions, panes, p)
	if got[keyOf("s-resumed")].Target != "dev:0.0" {
		t.Errorf("a resumed session lost its own pane: %+v", got)
	}
}

// Two panes at one cwd, one live session and one long dead: the live session
// takes the pane it could be in, and the dead one may have the other. What it
// must never do is take the live one.
func TestPlausibleSessionGetsFirstPick(t *testing.T) {
	p := &fakeProc{
		children: map[int][]int{700: {701}, 800: {801}},
		comm:     map[int]string{700: "zsh", 701: "claude", 800: "zsh", 801: "claude"},
		cwd:      map[int]string{701: "/w/app", 801: "/w/app"},
		started: map[int]time.Time{
			701: at(ago(10 * time.Minute)), // tinker:0.0 — young
			801: at(ago(72 * time.Hour)),   // tinker:1.0 — older than both sessions
		},
	}
	panes := []tmuxop.Pane{
		{Target: "tinker:0.0", ID: "%1", PID: 700, Command: "claude"},
		{Target: "tinker:1.0", ID: "%2", PID: 800, Command: "claude"},
	}
	sessions := []fleet.SessionInfo{
		sessionAt("s-dead", "/w/app", ago(48*time.Hour)),
		sessionAt("s-live", "/w/app", ago(25*time.Second)),
	}

	got := tmuxop.MapSessions(sessions, panes, p)
	if got[keyOf("s-live")].Target != "tinker:0.0" {
		t.Errorf("the live session got %q, want the young pane tinker:0.0", got[keyOf("s-live")].Target)
	}
	if got[keyOf("s-dead")].Target != "tinker:1.0" {
		t.Errorf("the dead session got %q, want the pane it could be in", got[keyOf("s-dead")].Target)
	}
}

// tmux run from OUTSIDE any session — which is exactly where compass lives,
// its own terminal tab — prints through a sanitizer that turns the tabs the
// format asks for into underscores. These rows are verbatim tmux 3.4 output
// captured that way, with the window names from a real fleet: "pixie_tuiZ" and
// "ts_feasibility" carry underscores of their own, and a session named
// "tinker-sub1" proves the anchor is the pane id, not the first separator.
func TestPanesSurviveTheOutsideTmuxSanitizer(t *testing.T) {
	r := &fakeRunner{outputs: [][]byte{[]byte(strings.Join([]string{
		"tinker:0.0_%0_4126_claude_claude",
		"tinker:3.0_%12_4200_bash_pixie_tuiZ",
		"tinker-sub1:5.0_%31_4310_node_ts_feasibility",
		"bash:0.1_%2_4001_bash_port-fwd-",
	}, "\n"))}}

	got, err := tmuxop.ListPanes(r)
	if err != nil {
		t.Fatalf("ListPanes: %v", err)
	}
	want := []tmuxop.Pane{
		{Target: "tinker:0.0", ID: "%0", PID: 4126, Command: "claude", Window: "claude"},
		{Target: "tinker:3.0", ID: "%12", PID: 4200, Command: "bash", Window: "pixie_tuiZ"},
		{Target: "tinker-sub1:5.0", ID: "%31", PID: 4310, Command: "node", Window: "ts_feasibility"},
		{Target: "bash:0.1", ID: "%2", PID: 4001, Command: "bash", Window: "port-fwd-"},
	}
	if len(got) != len(want) {
		t.Fatalf("ListPanes returned %d panes, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("pane %d =\n  %+v\nwant\n  %+v", i, got[i], want[i])
		}
	}
}

// StartTime is read from real /proc, so it gets a real test: the process
// running this test started moments ago, and pid 1 no later than it did.
func TestStartTimeReadsProc(t *testing.T) {
	p := tmuxop.RealProc{}
	self := p.StartTime(os.Getpid())
	if self.IsZero() {
		t.Fatal("could not read this process's own start time from /proc")
	}
	if age := time.Since(self); age < 0 || age > time.Hour {
		t.Errorf("this test process claims to be %v old", age)
	}
	if init := p.StartTime(1); !init.IsZero() && init.After(self) {
		t.Errorf("pid 1 started at %v, after this process at %v", init, self)
	}
	if got := p.StartTime(0); !got.IsZero() {
		t.Errorf("StartTime(0) = %v, want the zero time", got)
	}
}
