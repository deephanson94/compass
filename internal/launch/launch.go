// Package launch starts new work: a window in one of the user's tmux
// sessions, in a project directory, running a harness — Claude Code, a bare
// shell, or any CLI the config names — with the first prompt already handed
// to it. Optionally the work gets a git worktree of its own first.
//
// It is the one place a launch is decided. `compass new` and the deck's `n`
// panel both fill in a Request and call Launch; neither builds a command of
// its own.
package launch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/deephanson94/compass/internal/tmuxop"
)

// Harness is what a new window runs. Command is an argv template: a token
// that holds {prompt} or {name} has the value put in its place, and a token
// whose placeholder has no value is dropped whole — so `claude {prompt}`
// with no prompt is `claude`, and `tool --prompt={prompt}` is `tool`. An
// empty Command is the user's own shell, which takes no prompt.
type Harness struct {
	Name    string
	Command []string
}

// Builtin are the harnesses compass knows without being told: Claude Code,
// named after the window so the fleet calls it that too, and a shell.
var Builtin = []Harness{
	{Name: "claude", Command: []string{"claude", "-n", "{name}", "{prompt}"}},
	{Name: "shell"},
}

// TakesPrompt says whether anything the panel types as a prompt reaches the
// harness at all.
func (h Harness) TakesPrompt() bool {
	for _, tok := range h.Command {
		if strings.Contains(tok, "{prompt}") {
			return true
		}
	}
	return false
}

// Argv is the command with name and prompt in their places.
func (h Harness) Argv(name, prompt string) []string {
	var out []string
	for _, tok := range h.Command {
		if (strings.Contains(tok, "{name}") && name == "") || (strings.Contains(tok, "{prompt}") && prompt == "") {
			continue
		}
		tok = strings.ReplaceAll(tok, "{name}", name)
		tok = strings.ReplaceAll(tok, "{prompt}", prompt)
		out = append(out, tok)
	}
	return out
}

// ParseHarness reads one config line, `opencode: opencode run {prompt}`: a
// name, a colon, and the command split on whitespace. There is no quoting —
// a harness that needs it is a script on PATH. A name with nothing after the
// colon is a shell under that name.
func ParseHarness(line string) (Harness, bool) {
	name, cmd, ok := strings.Cut(line, ":")
	name = strings.TrimSpace(name)
	if !ok || name == "" || strings.ContainsAny(name, " \t") {
		return Harness{}, false
	}
	return Harness{Name: name, Command: strings.Fields(cmd)}, true
}

// Harnesses is the built-ins with the config's own over them: a harness the
// config names like a built-in replaces it where it stands, any other comes
// after. The first is the default.
func Harnesses(extra []Harness) []Harness {
	out := append([]Harness(nil), Builtin...)
	for _, h := range extra {
		replaced := false
		for i := range out {
			if out[i].Name == h.Name {
				out[i], replaced = h, true
				break
			}
		}
		if !replaced {
			out = append(out, h)
		}
	}
	return out
}

// Find is the harness by its name.
func Find(hs []Harness, name string) (Harness, bool) {
	for _, h := range hs {
		if h.Name == name {
			return h, true
		}
	}
	return Harness{}, false
}

// Request is one launch, as a person asked for it.
type Request struct {
	Session  string // the tmux session the window opens in; it must exist
	Dir      string // the project directory; "~" is the home directory
	Name     string // the window's name, the worktree's branch; "" = from the prompt
	Harness  Harness
	Prompt   string
	Worktree bool // give the work a git worktree of its own, off Dir's HEAD
}

// WindowName is the name the window, the session and the worktree's branch
// share: the one asked for, else the prompt's first words, else the
// harness's own name.
func (r Request) WindowName() string {
	if n := strings.TrimSpace(r.Name); n != "" {
		return n
	}
	if r.Harness.TakesPrompt() {
		if s := Slug(r.Prompt); s != "" {
			return s
		}
	}
	return r.Harness.Name
}

// Result is where a launch landed.
type Result struct {
	Pane tmuxop.Pane
	Dir  string // the directory the window opened in: the worktree's, if one was made
	Name string
}

// Launch makes the worktree, when asked for one, and opens the window.
func Launch(t tmuxop.Runner, g Git, r Request) (Result, error) {
	dir, err := ExpandHome(r.Dir)
	if err != nil {
		return Result{}, err
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return Result{}, fmt.Errorf("%s is not a directory", r.Dir)
	}
	name := r.WindowName()
	if r.Worktree {
		if dir, err = Worktree(g, dir, name); err != nil {
			return Result{}, err
		}
	}
	pane, err := tmuxop.NewWindow(t, r.Session, dir, name, r.Harness.Argv(name, strings.TrimSpace(r.Prompt)))
	if err != nil {
		return Result{}, err
	}
	return Result{Pane: pane, Dir: dir, Name: name}, nil
}

// Slug is a prompt's first words as a name: lower case, letters and digits,
// joined by dashes — "Explore adding per-tenant rate limits" is
// "explore-adding-per-tenant". Empty when the prompt has no such words.
func Slug(prompt string) string {
	const maxWords, maxLen = 4, 32
	words := strings.FieldsFunc(strings.ToLower(prompt), func(r rune) bool {
		return !(r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)))
	})
	if len(words) > maxWords {
		words = words[:maxWords]
	}
	s := strings.Join(words, "-")
	if len(s) > maxLen {
		s = strings.TrimRight(s[:maxLen], "-")
	}
	return s
}

// ExpandHome turns a leading "~" into the home directory.
func ExpandHome(p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, p[1:]), nil
}
