package state

import (
	"regexp"
	"strings"
)

// A verdict is a line a session ends its turn on to say how its work came
// out, for a reader that is not watching: a PR watcher's review session
// closes with `VERDICT: needs-you: changes the public C header` or
// `VERDICT: auto-ok`. Without it the board saw both as a finished turn —
// idle — and the one review that needed a decision sat among the ones that
// did not.
//
// The pattern is matched against the last non-empty line of the model's
// final message. A match whose `reason` group (or, without one, its first
// group) is non-empty needs you, with that text as the reason; any other
// match is done.

// DefaultVerdictPattern is the PR watcher's own form.
const DefaultVerdictPattern = `^VERDICT:\s*(?:needs-you:\s*(?P<reason>.+?)|auto-ok)\s*$`

// The verdict values a Snapshot carries.
const (
	VerdictNeedsYou = "needs-you"
	VerdictDone     = "done"
)

var verdictRE = regexp.MustCompile(DefaultVerdictPattern)

// SetVerdictPattern replaces the pattern; "off" turns verdicts off. An
// unreadable pattern is an error and leaves the one in force.
func SetVerdictPattern(p string) error {
	if p == "off" {
		verdictRE = nil
		return nil
	}
	re, err := regexp.Compile(p)
	if err != nil {
		return err
	}
	verdictRE = re
	return nil
}

// Verdict reads a message's closing line: VerdictNeedsYou and its reason,
// VerdictDone, or "" when the message ends on no verdict.
func Verdict(text string) (verdict, reason string) {
	if verdictRE == nil {
		return "", ""
	}
	line := lastLine(text)
	m := verdictRE.FindStringSubmatch(line)
	if m == nil {
		return "", ""
	}
	group := 1
	if i := verdictRE.SubexpIndex("reason"); i > 0 {
		group = i
	}
	if group < len(m) {
		if r := strings.TrimSpace(m[group]); r != "" {
			return VerdictNeedsYou, r
		}
	}
	return VerdictDone, ""
}

// lastLine is text's last non-empty line, with the markdown a model
// decorates a closing line with peeled off both ends: **VERDICT: …**,
// `VERDICT: …`, a bullet.
func lastLine(text string) string {
	lines := strings.Split(strings.TrimRight(text, " \t\r\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		l = strings.TrimLeft(l, "-*> ")
		l = strings.Trim(l, "*_`")
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}
