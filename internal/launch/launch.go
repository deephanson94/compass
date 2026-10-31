// Package launch starts new work: a window in one of the user's tmux
// sessions (or a split, or a session of its own), in a project directory, running a harness — Claude Code, a bare
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

// Harness is what new work runs. Command is an argv template: a token
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

// Config is what the config file says about launching: the harnesses
// beyond the built-ins, the directories to offer first, and what the `n`
// panel opens on before the person touches a row.
type Config struct {
	Harnesses []Harness
	Dirs      []string     // favourites, as written ("~" allowed), in order
	Run       string       // new_run: the harness the panel opens on; "" = the first
	Place     tmuxop.Place // new_as: window (the zero value), pane or session
	Worktree  bool         // new_worktree: the worktree row starts on
	Then      string       // new_then: "go" or "stay"; "" = go for what never gets a card, stay for claude
}

// Request is one launch, as a person asked for it.
type Request struct {
	Place    tmuxop.Place // a window (the default), a pane, or a tmux session of its own
	Target   string       // for a window, the existing session; for a pane, the pane to split beside
	Dir      string       // the project directory; "~" is the home directory
	Name     string       // the window's, a new session's and the worktree branch's name; "" = from the prompt
	Harness  Harness
	Prompt   string
	Worktree bool     // give the work a git worktree of its own, off Dir's HEAD
	MakeDir  bool     // create Dir, empty, when it is not there
	Env      []string // "KEY=value" set in the new pane's process (tmux -e)
}

// TaskName is the name the window, a new session, the worktree's branch and
// a claude session share: the one asked for, else the prompt's first words,
// else the harness's own name.
func (r Request) TaskName() string {
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
	Dir  string // the directory the work opened in: the worktree's, if one was made
	Name string
}

// Launch makes the directory or the worktree, when asked for one, and opens
// the work where the request places it. Everything that can refuse is asked
// before anything is written: a directory or a worktree made for a session
// name already taken would be left behind with nothing in it.
func Launch(t tmuxop.Runner, g Git, r Request) (Result, error) {
	if strings.TrimSpace(r.Dir) == "" {
		return Result{}, fmt.Errorf("no directory to open in")
	}
	dir, err := ExpandHome(r.Dir)
	if err != nil {
		return Result{}, err
	}
	missing, err := Missing(dir)
	if err != nil {
		return Result{}, err
	}
	if missing && !r.MakeDir {
		return Result{}, fmt.Errorf("%s does not exist", r.Dir)
	}
	if missing && r.Worktree {
		return Result{}, fmt.Errorf("%s is new: there is no repository to make a worktree of", r.Dir)
	}
	name := r.TaskName()
	if err := placeable(t, r.Place, r.Target, name); err != nil {
		return Result{}, err
	}
	if missing {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Result{}, err
		}
	}
	if r.Worktree {
		if dir, err = Worktree(g, dir, name); err != nil {
			return Result{}, err
		}
	}
	pane, err := tmuxop.OpenWith(t, r.Place, r.Target, dir, name, r.Env, r.Harness.Argv(name, strings.TrimSpace(r.Prompt)))
	if err != nil {
		return Result{}, err
	}
	return Result{Pane: pane, Dir: dir, Name: name}, nil
}

// Missing says whether dir is not there yet. Something there that is not a
// directory is an error, not a directory to make.
func Missing(dir string) (bool, error) {
	fi, err := os.Stat(dir)
	switch {
	case os.IsNotExist(err):
		return true, nil
	case err != nil:
		return false, err
	case !fi.IsDir():
		return false, fmt.Errorf("%s is not a directory", dir)
	}
	return false, nil
}

// placeable asks tmux whether the place is there to be used: the session a
// window opens in, the pane a split goes beside, and — the other way round —
// no session already by the name a new one would take.
func placeable(t tmuxop.Runner, place tmuxop.Place, target, name string) error {
	switch place {
	case tmuxop.PlaceWindow:
		if target == "" {
			return fmt.Errorf("no tmux session to open a window in")
		}
		if _, err := t.Output("has-session", "-t", "="+target); err != nil {
			return fmt.Errorf("no tmux session %q", target)
		}
	case tmuxop.PlacePane:
		if target == "" {
			return fmt.Errorf("no pane to split beside")
		}
		if _, err := t.Output("display-message", "-p", "-t", target, "#{pane_id}"); err != nil {
			return fmt.Errorf("no pane %s to split beside", target)
		}
	case tmuxop.PlaceSession:
		if _, err := t.Output("has-session", "-t", "="+name); err == nil {
			return fmt.Errorf("a tmux session %q already exists", name)
		}
	}
	return nil
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
