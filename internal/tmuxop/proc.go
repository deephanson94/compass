package tmuxop

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Proc reads process relationships; RealProc walks /proc.
type Proc interface {
	Children(pid int) []int // direct children
	Comm(pid int) string    // process name, e.g. "claude" — or "node" for an npm install
	Cmdline(pid int) string // argv, NUL-separators turned to spaces; "" if unreadable
	Cwd(pid int) string     // "" if unreadable

	// StartTime is when the process began. Zero means unreadable — which is
	// ordinary, and never a reason to disbelieve anything else about it.
	StartTime(pid int) time.Time
}

// RealProc answers from /proc. Every read is best-effort: a process that exits
// mid-walk is ordinary, not an error.
type RealProc struct{}

// Children returns the direct children of pid, from
// /proc/<pid>/task/<tid>/children when the kernel exposes it, otherwise by
// scanning /proc for processes whose parent is pid.
func (RealProc) Children(pid int) []int {
	if pid <= 0 {
		return nil
	}
	if kids, ok := childrenFromTasks(pid); ok {
		return kids
	}
	return childrenByScan(pid)
}

// Comm is the process name, without the trailing newline /proc keeps on it.
func (RealProc) Comm(pid int) string {
	if pid <= 0 {
		return ""
	}
	b, err := os.ReadFile(procPath(pid, "comm"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Cwd is the process's working directory, "" if unreadable — another user's
// process, or one that just exited.
func (RealProc) Cwd(pid int) string {
	if pid <= 0 {
		return ""
	}
	dir, err := os.Readlink(procPath(pid, "cwd"))
	if err != nil {
		return ""
	}
	return dir
}

// childrenFromTasks reads the per-thread children lists. Each file holds
// space-separated pids. ok is false when the kernel does not publish them, so
// the caller can fall back to scanning.
func childrenFromTasks(pid int) ([]int, bool) {
	tasks, err := os.ReadDir(procPath(pid, "task"))
	if err != nil {
		return nil, false
	}
	var (
		out  []int
		read bool
	)
	for _, t := range tasks {
		b, err := os.ReadFile(filepath.Join(procPath(pid, "task"), t.Name(), "children"))
		if err != nil {
			continue
		}
		read = true
		for _, f := range strings.Fields(string(b)) {
			if kid, err := strconv.Atoi(f); err == nil {
				out = append(out, kid)
			}
		}
	}
	return out, read
}

// childrenByScan walks /proc looking for processes whose ppid is pid. It is the
// fallback for kernels built without CONFIG_PROC_CHILDREN.
func childrenByScan(pid int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var out []int
	for _, e := range entries {
		cand, err := strconv.Atoi(e.Name())
		if err != nil || cand <= 0 {
			continue // not a process directory
		}
		b, err := os.ReadFile(procPath(cand, "stat"))
		if err != nil {
			continue
		}
		if ppid, ok := statPPID(string(b)); ok && ppid == pid {
			out = append(out, cand)
		}
	}
	return out
}

// statPPID pulls the parent pid out of a /proc/<pid>/stat line. The comm field
// is parenthesised and may itself contain spaces and parentheses, so the fields
// are only reliable after the last ')'.
func statPPID(stat string) (int, bool) {
	i := strings.LastIndexByte(stat, ')')
	if i < 0 {
		return 0, false
	}
	fields := strings.Fields(stat[i+1:])
	if len(fields) < 2 {
		return 0, false
	}
	ppid, err := strconv.Atoi(fields[1]) // state, then ppid
	if err != nil {
		return 0, false
	}
	return ppid, true
}

// Cmdline reads a process's argv with its NUL separators turned into spaces.
func (RealProc) Cmdline(pid int) string {
	raw, err := os.ReadFile(procPath(pid, "cmdline"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(string(raw), "\x00", " "))
}

// StartTime reads field 22 of /proc/<pid>/stat — the process's start, in clock
// ticks since boot — and anchors it to the boot time in /proc/stat.
//
// The comm field is the parenthesised second column and may itself contain
// spaces and parentheses, so the split is on the LAST ")": everything after it
// begins at field 3, which puts starttime at index 19.
func (RealProc) StartTime(pid int) time.Time {
	if pid <= 0 {
		return time.Time{}
	}
	raw, err := os.ReadFile(procPath(pid, "stat"))
	if err != nil {
		return time.Time{}
	}
	i := strings.LastIndexByte(string(raw), ')')
	if i < 0 {
		return time.Time{}
	}
	fields := strings.Fields(string(raw)[i+1:])
	if len(fields) < 20 {
		return time.Time{}
	}
	ticks, err := strconv.ParseInt(fields[19], 10, 64)
	if err != nil {
		return time.Time{}
	}
	boot := bootTime()
	if boot.IsZero() {
		return time.Time{}
	}
	return boot.Add(time.Duration(ticks) * time.Second / clockTicks)
}

// clockTicks is USER_HZ, which reading sysconf(_SC_CLK_TCK) would answer
// exactly. It is 100 on every Linux build in practice, and the margin this
// feeds — days, not milliseconds — does not turn on the difference.
const clockTicks = 100

// bootTime is read once: it does not change while compass runs, and every pane
// on the machine anchors to the same one.
var bootTime = sync.OnceValue(func() time.Time {
	raw, err := os.ReadFile(filepath.Join("/proc", "stat"))
	if err != nil {
		return time.Time{}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		rest, ok := strings.CutPrefix(line, "btime ")
		if !ok {
			continue
		}
		secs, err := strconv.ParseInt(strings.TrimSpace(rest), 10, 64)
		if err != nil {
			return time.Time{}
		}
		return time.Unix(secs, 0)
	}
	return time.Time{}
})

func procPath(pid int, name string) string {
	return filepath.Join("/proc", strconv.Itoa(pid), name)
}

// claudeComm is the process name a natively-installed CLI runs under. An npm
// install is a Node script, so it runs under the interpreter's name instead
// and only its argv says what it is — see isClaude.
const claudeComm = "claude"

// interpreters are the runtimes a script-installed CLI hides behind.
var interpreters = map[string]bool{"node": true, "bun": true, "deno": true}

// isClaude decides whether a process is a Claude Code CLI. Two installs, two
// shapes: the native binary answers to its own name, while `npm i -g
// @anthropic-ai/claude-code` runs as `node …/claude-code/cli.js`, where the
// name is the interpreter's and the evidence is in argv.
//
// The argv test is deliberately narrow — an editor holding claude-notes.md
// must not look like a session — so it asks for an argument that is the CLI
// itself: a path ending in /claude, or the package's own script.
func isClaude(p Proc, pid int) bool {
	comm := p.Comm(pid)
	if comm == claudeComm {
		return true
	}
	args := strings.Fields(p.Cmdline(pid))
	if len(args) == 0 {
		return false
	}
	if filepath.Base(args[0]) == claudeComm {
		return true // a wrapper exec'd under another name
	}
	if !interpreters[comm] {
		return false // only a runtime can be hiding a CLI in its argv
	}
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if strings.Contains(arg, "claude-code") {
			return true // the package's own script, whatever it is called
		}
		if strings.Contains(arg, "/") && filepath.Base(arg) == claudeComm {
			return true // a path to the CLI, not somebody saying the word
		}
	}
	return false
}

// claudeDepth bounds the descendant walk. A pane's shell wraps the CLI in a
// handful of processes at most; anything deeper is somebody else's tree.
const claudeDepth = 6

// ClaudeCwd finds the Claude Code process a pane is running — the pane's own
// process when tmux was handed the CLI directly (`tmux new-window claude`),
// otherwise the first one among its descendants (breadth-first, depth ≤ 6),
// which is the usual shape: a shell you typed `claude` into. The bool is false
// when no claude runs there, or when its cwd is unreadable — either way there
// is nothing to match a session against.
func ClaudeCwd(p Proc, pid int) (string, bool) {
	cwd, _, ok := ClaudeIn(p, pid)
	return cwd, ok
}

// ClaudeIn is ClaudeCwd plus the pid it found the claude at, so a caller can
// ask how long that claude has been running. A session cannot be the one
// living in a pane if it stopped writing before that pane's claude started.
func ClaudeIn(p Proc, pid int) (cwd string, claudePID int, ok bool) {
	if isClaude(p, pid) {
		cwd := p.Cwd(pid)
		return cwd, pid, cwd != ""
	}
	seen := map[int]bool{pid: true}
	frontier := p.Children(pid)
	for depth := 1; depth <= claudeDepth && len(frontier) > 0; depth++ {
		var next []int
		for _, kid := range frontier {
			if seen[kid] {
				continue
			}
			seen[kid] = true
			if isClaude(p, kid) {
				cwd := p.Cwd(kid)
				return cwd, kid, cwd != ""
			}
			if depth < claudeDepth {
				next = append(next, p.Children(kid)...)
			}
		}
		frontier = next
	}
	return "", 0, false
}
