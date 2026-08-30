package ui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/deephanson94/compass/internal/fleet"
)

// askBin is the binary the historian runs in. compass never speaks to an API:
// it hands the terminal to the user's own `claude`, with the auth they already
// have (decision log #7).
const askBin = "claude"

// askDoneMsg comes back when the historian exits and compass has the terminal
// again.
type askDoneMsg struct{ err error }

// BuildAsk constructs the historian: a real `claude` session, started in the
// session's own working directory, whose system prompt tells it that it is
// reading somebody else's journey rather than continuing it. Pure — it builds
// the command and starts nothing.
func BuildAsk(info fleet.SessionInfo) *exec.Cmd {
	cmd := exec.Command(askBin, "--append-system-prompt", askPreamble(info))
	cmd.Dir = info.CWD
	return cmd
}

// askPreamble is the historian's brief. It names the session so the model knows
// whose past it is holding, points at the transcript, and asks for the two
// things that make an answer checkable: read the record first, and cite the
// times it read.
func askPreamble(info fleet.SessionInfo) string {
	var b strings.Builder
	b.WriteString("You are the historian of one Claude Code session — its reader, not its author.\n\n")
	fmt.Fprintf(&b, "session: %s\n", quoteOr(info.Title, "(untitled)"))
	fmt.Fprintf(&b, "branch: %s\n", orDash(info.GitBranch))
	fmt.Fprintf(&b, "working directory: %s\n", orDash(info.CWD))
	fmt.Fprintf(&b, "state: last active %s (started %s)\n", stamp(info.LastEventAt), stamp(info.StartedAt))
	fmt.Fprintf(&b, "transcript: %s\n\n", orDash(info.TranscriptPath))
	b.WriteString("Read the transcript first, before answering anything. It is JSONL — one " +
		"event per line, oldest first — and it is the only record of what this session did.\n\n")
	b.WriteString("Then answer questions about that journey: what it tried, what it abandoned " +
		"and why, what a reviewer should look at. Cite timestamps from the transcript for " +
		"anything you claim. If the transcript does not say, say so rather than guessing. " +
		"You are here to explain the past, so do not edit files or change anything.")
	return b.String()
}

func quoteOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return `"` + s + `"`
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	return s
}

func stamp(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.UTC().Format(time.RFC3339)
}

// ask suspends the deck and hands the terminal to the historian, the way
// `git commit` hands you your editor (decision log #7). A machine without the
// CLI gets a note, never a crash.
func (m *Model) ask() tea.Cmd {
	s, ok := m.selected()
	if !ok {
		m.note = "no session to ask about"
		return nil
	}
	cmd := BuildAsk(s.Info)
	if cmd.Err != nil {
		// exec.Command resolves the binary eagerly, so a missing CLI is known
		// before anything is suspended.
		m.note = "no `claude` on PATH — ask the trail needs it"
		return nil
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return askDoneMsg{err: err} })
}
