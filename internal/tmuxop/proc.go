package tmuxop

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Proc reads process relationships; RealProc walks /proc.
type Proc interface {
	Children(pid int) []int // direct children
	Comm(pid int) string    // process name, e.g. "claude"
	Cwd(pid int) string     // "" if unreadable
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

func procPath(pid int, name string) string {
	return filepath.Join("/proc", strconv.Itoa(pid), name)
}

// claudeComm is the process name the CLI runs under.
const claudeComm = "claude"

// claudeDepth bounds the descendant walk. A pane's shell wraps the CLI in a
// handful of processes at most; anything deeper is somebody else's tree.
const claudeDepth = 6

// ClaudeCwd walks the descendants of pid (breadth-first, depth ≤ 6) for the
// first process whose Comm is "claude" and returns its Cwd. The bool is false
// when no claude runs under pid, or when its cwd is unreadable — either way
// there is nothing to match a session against.
func ClaudeCwd(p Proc, pid int) (string, bool) {
	seen := map[int]bool{pid: true}
	frontier := p.Children(pid)
	for depth := 1; depth <= claudeDepth && len(frontier) > 0; depth++ {
		var next []int
		for _, kid := range frontier {
			if seen[kid] {
				continue
			}
			seen[kid] = true
			if p.Comm(kid) == claudeComm {
				cwd := p.Cwd(kid)
				return cwd, cwd != ""
			}
			if depth < claudeDepth {
				next = append(next, p.Children(kid)...)
			}
		}
		frontier = next
	}
	return "", false
}
