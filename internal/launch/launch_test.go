package launch

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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

func TestWindowName(t *testing.T) {
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
		if got := c.r.WindowName(); got != c.want {
			t.Errorf("WindowName(%+v) = %q, want %q", c.r, got, c.want)
		}
	}
}

func TestSlugIsBounded(t *testing.T) {
	if got := Slug("supercalifragilistic expialidocious antidisestablishmentarian x"); len(got) > 32 || strings.HasSuffix(got, "-") {
		t.Errorf("Slug = %q", got)
	}
}

// fakeTmux records the one new-window call and says where it landed.
type fakeTmux struct{ calls [][]string }

func (f *fakeTmux) Output(args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	return []byte("%9 dev:3.0\n"), nil
}

func TestLaunchOpensTheWindowWhereAsked(t *testing.T) {
	dir := t.TempDir()
	tm := &fakeTmux{}
	res, err := Launch(tm, RealGit{}, Request{Session: "dev", Dir: dir, Harness: Builtin[0], Prompt: "  fix the 401  "})
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
	if _, err := Launch(tm, RealGit{}, Request{Session: "dev", Dir: filepath.Join(t.TempDir(), "gone"), Harness: Builtin[1]}); err == nil {
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
	res, err := Launch(tm, RealGit{}, Request{Session: "dev", Dir: sub, Name: "rate", Harness: Builtin[1], Worktree: true})
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
	res2, err := Launch(tm, RealGit{}, Request{Session: "dev", Dir: res.Dir, Name: "next", Harness: Builtin[1], Worktree: true})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(realBase, "api.worktrees", "next", "pkg", "auth"); res2.Dir != want {
		t.Errorf("Dir = %q, want %q", res2.Dir, want)
	}

	// The same branch twice is git's refusal, and no window.
	n := len(tm.calls)
	if _, err := Launch(tm, RealGit{}, Request{Session: "dev", Dir: repo, Name: "rate", Harness: Builtin[1], Worktree: true}); err == nil || !strings.Contains(err.Error(), "already exists") {
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
