package fleet

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// processAlive answers whether the process that wrote a registry entry is
// still the process running under that pid.
//
// A signal of zero is the portable half: it delivers nothing and reports
// whether the pid exists. That alone would be fooled by pid reuse — the
// registry outlives the session where the entry was not cleaned up, and
// the kernel hands the number to somebody else — so where the kernel says
// when a process started, that is compared too. The registry's `procStart`
// is field 22 of /proc/<pid>/stat verbatim, in clock ticks since boot,
// which is exactly the field that makes a pid unambiguous.
//
// Where the start cannot be read — no /proc, a process owned by somebody
// else — the pid's own existence stands. Being wrong there keeps a session
// on the board that could have left it, which is the side of this to be
// wrong on.
func processAlive(pid int, procStart string) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := p.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	if procStart == "" {
		return true
	}
	started, ok := procStarted(pid)
	if !ok {
		return true
	}
	return started == procStart
}

// procStarted reads field 22 of /proc/<pid>/stat — the process's start in
// clock ticks since boot. The comm field is parenthesised and may itself
// contain spaces and parentheses, so the split is on the last ")":
// everything after it begins at field 3, which puts starttime at index 19.
func procStarted(pid int) (string, bool) {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return "", false
	}
	i := strings.LastIndexByte(string(raw), ')')
	if i < 0 {
		return "", false
	}
	fields := strings.Fields(string(raw)[i+1:])
	if len(fields) < 20 {
		return "", false
	}
	return fields[19], true
}
