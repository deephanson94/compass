package launch

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/deephanson94/compass/internal/tmuxop"
)

func TestArgvFillsAndDropsPlaceholders(t *testing.T) {
	claude := Builtin[0]
	for _, c := range []struct {
		h            Harness
		name, prompt string
		want         []string
	}{
		{claude, "rate", `map the "middleware" first`, []string{"claude", "-n", "rate", `map the "middleware" first`}},
		{claude, "rate", "", []string{"claude", "-n", "rate"}},
		{Harness{Command: []string{"tool", "--prompt={prompt}", "--title", "{name}"}}, "x", "", []string{"tool", "--title", "x"}},
		{Harness{Name: "shell"}, "x", "ignored", nil},
	} {
		if got := c.h.Argv(c.name, c.prompt); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v.Argv(%q, %q) = %q, want %q", c.h.Command, c.name, c.prompt, got, c.want)
		}
	}
}

func TestParseHarness(t *testing.T) {
	h, ok := ParseHarness("opencode: opencode run  {prompt}")
	if !ok || h.Name != "opencode" || !reflect.DeepEqual(h.Command, []string{"opencode", "run", "{prompt}"}) {
		t.Errorf("ParseHarness = %+v, %v", h, ok)
	}
	if h, ok := ParseHarness("zsh:"); !ok || len(h.Command) != 0 || h.TakesPrompt() {
		t.Errorf("a bare name is a shell: %+v, %v", h, ok)
	}
	for _, bad := range []string{"no colon here", ": claude", "two words: x"} {
		if _, ok := ParseHarness(bad); ok {
			t.Errorf("ParseHarness(%q) accepted", bad)
		}
	}
}

// The config's own harness by a built-in's name replaces it where it
// stands, so `claude` stays the default with whatever flags it was given.
func TestHarnessesOverride(t *testing.T) {
	got := Harnesses([]Harness{
		{Name: "claude", Command: []string{"claude", "--model", "opus", "{prompt}"}},
		{Name: "opencode", Command: []string{"opencode"}},
	})
	var names []string
	for _, h := range got {
		names = append(names, h.Name)
	}
	if !reflect.DeepEqual(names, []string{"claude", "shell", "opencode"}) {
		t.Errorf("names = %q", names)
	}
	if got[0].Command[1] != "--model" {
		t.Errorf("claude was not replaced: %q", got[0].Command)
	}
	if Builtin[0].Command[1] != "-n" {
		t.Error("the built-ins themselves changed")
	}
}

func TestTaskName(t *testing.T) {
	claude, shell := Builtin[0], Builtin[1]
	for _, c := range []struct {
		r    Request
		want string
	}{
		{Request{Name: " mine ", Harness: claude, Prompt: "whatever"}, "mine"},
		{Request{Harness: claude, Prompt: "Explore adding per-tenant rate limits"}, "explore-adding-per-tenant"},
		{Request{Harness: claude, Prompt: "日本語だけ"}, "claude"},
		{Request{Harness: shell, Prompt: "a prompt a shell never sees"}, "shell"},
	} {
		if got := c.r.TaskName(); got != c.want {
			t.Errorf("TaskName(%+v) = %q, want %q", c.r, got, c.want)
		}
	}
}

func TestSlugIsBounded(t *testing.T) {
	if got := Slug("supercalifragilistic expialidocious antidisestablishmentarian x"); len(got) > 32 || strings.HasSuffix(got, "-") {
		t.Errorf("Slug = %q", got)
	}
}

// fakeTmux is a server with the session "dev" and its pane %2. It answers
// the questions Launch asks first, and records what Launch opened in calls.
type fakeTmux struct{ calls [][]string }

func (f *fakeTmux) Output(args ...string) ([]byte, error) {
	switch args[0] {
	case "has-session":
		if args[2] == "=dev" {
			return nil, nil
		}
		return nil, errors.New("can't find session")
	case "display-message":
		if args[3] == "%2" {
			return []byte("%2\n"), nil
		}
		return nil, errors.New("can't find pane")
	}
	f.calls = append(f.calls, args)
	return []byte("%9 dev:3.0\n"), nil
}

func TestLaunchOpensTheWindowWhereAsked(t *testing.T) {
	dir := t.TempDir()
	tm := &fakeTmux{}
	res, err := Launch(tm, RealGit{}, Request{Target: "dev", Dir: dir, Harness: Builtin[0], Prompt: "  fix the 401  "})
	if err != nil {
		t.Fatal(err)
	}
	if res.Pane.ID != "%9" || res.Dir != dir || res.Name != "fix-the-401" {
		t.Errorf("res = %+v", res)
	}
	args := tm.calls[0]
	if got := args[len(args)-4:]; !reflect.DeepEqual(got, []string{"claude", "-n", "fix-the-401", "fix the 401"}) {
		t.Errorf("command = %q", got)
	}
}

func TestLaunchRefusesAMissingDirectory(t *testing.T) {
	tm := &fakeTmux{}
	if _, err := Launch(tm, RealGit{}, Request{Target: "dev", Dir: filepath.Join(t.TempDir(), "gone"), Harness: Builtin[1]}); err == nil {
		t.Fatal("want an error")
	}
	if len(tm.calls) != 0 {
		t.Error("tmux was asked anyway")
	}
}

// A worktree against a real repository: a new branch, checked out beside
// the main checkout, and the window opens at the same depth it was asked
// from — also when asked from inside another worktree.
func TestLaunchInAWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	base := t.TempDir()
	repo := filepath.Join(base, "api")
	sub := filepath.Join(repo, "pkg", "auth")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.go"), []byte("package auth\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"add", "."},
		{"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "init"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	tm := &fakeTmux{}
	res, err := Launch(tm, RealGit{}, Request{Target: "dev", Dir: sub, Name: "rate", Harness: Builtin[1], Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	realBase, _ := filepath.EvalSymlinks(base)
	want := filepath.Join(realBase, "api.worktrees", "rate", "pkg", "auth")
	if res.Dir != want {
		t.Errorf("Dir = %q, want %q", res.Dir, want)
	}
	if _, err := os.Stat(filepath.Join(want, "a.go")); err != nil {
		t.Errorf("the worktree is not checked out: %v", err)
	}
	if out, _ := exec.Command("git", "-C", want, "branch", "--show-current").Output(); strings.TrimSpace(string(out)) != "rate" {
		t.Errorf("branch = %q, want rate", out)
	}

	// From inside that worktree, the next one is its sibling.
	res2, err := Launch(tm, RealGit{}, Request{Target: "dev", Dir: res.Dir, Name: "next", Harness: Builtin[1], Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(realBase, "api.worktrees", "next", "pkg", "auth"); res2.Dir != want {
		t.Errorf("Dir = %q, want %q", res2.Dir, want)
	}

	// The same branch twice is git's refusal, and no window.
	n := len(tm.calls)
	if _, err := Launch(tm, RealGit{}, Request{Target: "dev", Dir: repo, Name: "rate", Harness: Builtin[1], Worktree: true}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("err = %v, want git's word on the branch", err)
	}
	if len(tm.calls) != n {
		t.Error("a window opened for a failed worktree")
	}
}

func TestWorktreeOutsideARepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	if _, err := Worktree(RealGit{}, t.TempDir(), "x"); err == nil || !strings.Contains(err.Error(), "not in a git repository") {
		t.Errorf("err = %v", err)
	}
}

// Each place opens with its own tmux verb, and a place tmux would refuse
// is refused before anything is made — a worktree included.
func TestLaunchPlaces(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		r       Request
		verb    string
		refused string
	}{
		{Request{Place: tmuxop.PlacePane, Target: "%2"}, "split-window", ""},
		{Request{Place: tmuxop.PlaceSession, Name: "rate"}, "new-session", ""},
		{Request{Place: tmuxop.PlaceSession, Name: "dev"}, "", `session "dev" already exists`},
		{Request{Place: tmuxop.PlacePane, Target: "%404"}, "", "no pane %404"},
		{Request{Place: tmuxop.PlaceWindow, Target: "gone"}, "", `no tmux session "gone"`},
	} {
		tm := &fakeTmux{}
		c.r.Dir, c.r.Harness = dir, Builtin[1]
		if c.refused != "" {
			// dir is no repository, so a worktree tried would fail with
			// git's word, not the refusal's.
			c.r.Worktree = true
			_, err := Launch(tm, RealGit{}, c.r)
			if err == nil || !strings.Contains(err.Error(), c.refused) {
				t.Errorf("%v %q: err = %v, want %q", c.r.Place, c.r.Target+c.r.Name, err, c.refused)
			}
			if len(tm.calls) != 0 {
				t.Errorf("%v: opened %q after a refusal", c.r.Place, tm.calls)
			}
			continue
		}
		if _, err := Launch(tm, RealGit{}, c.r); err != nil {
			t.Fatalf("%v: %v", c.r.Place, err)
		}
		if got := tm.calls[0][0]; got != c.verb {
			t.Errorf("%v opened with %q, want %q", c.r.Place, got, c.verb)
		}
	}
}

// A directory that is not there is refused unless asked for; asked for, it
// is made — but only once tmux has agreed, and never under a worktree.
func TestLaunchMakesADirectoryOnlyWhenAsked(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "thing")
	tm := &fakeTmux{}
	if _, err := Launch(tm, RealGit{}, Request{Target: "dev", Dir: dir, Harness: Builtin[1]}); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("err = %v, want a refusal", err)
	}
	if _, err := Launch(tm, RealGit{}, Request{Target: "dev", Dir: dir, Harness: Builtin[1], MakeDir: true, Worktree: true}); err == nil || !strings.Contains(err.Error(), "no repository") {
		t.Errorf("err = %v, want the worktree refused", err)
	}
	if _, err := Launch(tm, RealGit{}, Request{Target: "gone", Dir: dir, Harness: Builtin[1], MakeDir: true}); err == nil {
		t.Error("a refused target: want an error")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("a refused launch left %s behind", dir)
	}
	res, err := Launch(tm, RealGit{}, Request{Target: "dev", Dir: dir, Harness: Builtin[1], MakeDir: true})
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() || res.Dir != dir {
		t.Errorf("dir not made: %v (res %+v)", err, res)
	}

	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Launch(tm, RealGit{}, Request{Target: "dev", Dir: file, Harness: Builtin[1], MakeDir: true}); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("a file: err = %v", err)
	}
	if _, err := Launch(tm, RealGit{}, Request{Target: "dev", Dir: "  ", Harness: Builtin[1], MakeDir: true}); err == nil {
		t.Error("no directory: want an error")
	}
}
