package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/deephanson94/compass/internal/tmuxop"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := `# compass configuration
root = "~/claude-home"
narrator = 'off'
readonly = true
live_within = "90s"
dim = "#c0c0c0"
mystery = "ignored"
this line is not a setting
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPASS_CONFIG", path)
	t.Setenv("COMPASS_DIM", "")

	c := loadConfig()
	if c.Root != "~/claude-home" {
		t.Errorf("Root = %q", c.Root)
	}
	if c.Narrator != "off" {
		t.Errorf("Narrator = %q", c.Narrator)
	}
	if !c.Readonly {
		t.Error("Readonly = false, want true")
	}
	if c.LiveWithin != "90s" {
		t.Errorf("LiveWithin = %q", c.LiveWithin)
	}
	if c.Dim != "#c0c0c0" {
		t.Errorf("Dim = %q", c.Dim)
	}
}

// The grey is the one setting you reach for because you cannot read the
// screen, so it is reachable without first finding out where the config file
// lives — and it wins over the file when both say something.
func TestDimComesFromTheEnvironmentWithOrWithoutAFile(t *testing.T) {
	t.Setenv("COMPASS_CONFIG", filepath.Join(t.TempDir(), "absent.toml"))
	t.Setenv("COMPASS_DIM", "#d0d0d0")
	if c := loadConfig(); c.Dim != "#d0d0d0" {
		t.Errorf("with no config file, Dim = %q, want #d0d0d0", c.Dim)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("dim = \"#404040\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPASS_CONFIG", path)
	if c := loadConfig(); c.Dim != "#d0d0d0" {
		t.Errorf("the environment did not win over the file: Dim = %q", c.Dim)
	}
	t.Setenv("COMPASS_DIM", "")
	if c := loadConfig(); c.Dim != "#404040" {
		t.Errorf("with the environment empty the file should stand: Dim = %q", c.Dim)
	}
}

func TestLoadConfigMissingFileIsNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent.toml")
	t.Setenv("COMPASS_CONFIG", path)
	t.Setenv("COMPASS_DIM", "")
	// Nothing set, nothing to warn about; only where it looked is known.
	if c := loadConfig(); !reflect.DeepEqual(c, config{Path: path}) {
		t.Errorf("loadConfig() on a missing file = %+v, want only its path", c)
	}
}

func TestConfigLine(t *testing.T) {
	cases := []struct {
		in       string
		key, val string
		ok       bool
	}{
		{`narrator = "haiku"`, "narrator", "haiku", true},
		{`readonly=true`, "readonly", "true", true},
		{`  root = '~/x'  `, "root", "~/x", true},
		{`# a comment`, "", "", false},
		{``, "", "", false},
		{`bare words`, "", "", false},
		{`dim = "#9a9a9a"`, "dim", "#9a9a9a", true},
		{`= "orphan"`, "", "", false},
		{`empty = ""`, "", "", false},
		{`jump = "display-popup -E 'tmux-jump'"`, "jump", "display-popup -E 'tmux-jump'", true},
		{`hook = "tmux display-message \"$COMPASS_EVENT\""`, "hook", `tmux display-message "$COMPASS_EVENT"`, true},
		{`jump = 'choose-tree -Zs'`, "jump", "choose-tree -Zs", true},
		{`dir = "~/work/api"      # a favourite for n's dir row`, "dir", "~/work/api", true},
		{`readonly = true   # keep hands off tmux`, "readonly", "true", true},
		{`dim = #9a9a9a`, "dim", "#9a9a9a", true},
		{`empty = ""  # nothing`, "", "", false},
	}
	for _, tc := range cases {
		key, val, ok := configLine(tc.in)
		if key != tc.key || val != tc.val || ok != tc.ok {
			t.Errorf("configLine(%q) = %q,%q,%v want %q,%q,%v", tc.in, key, val, ok, tc.key, tc.val, tc.ok)
		}
	}
}

// Quick replies come one `reply = "…"` line each, in order, at most nine;
// none means the stock three.
func TestLoadConfigReplies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	var lines []string
	for i := 0; i < 11; i++ {
		lines = append(lines, `reply = "line `+string(rune('a'+i))+`"`)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPASS_CONFIG", path)
	c := loadConfig()
	if len(c.Replies) != 9 || c.Replies[0] != "line a" || c.Replies[8] != "line i" {
		t.Errorf("Replies = %q, want the first nine in order", c.Replies)
	}
}

// Each harness line is one more thing `n` can run; a line compass cannot
// read is ignored like any other.
func TestLoadConfigHarnesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `harness = "opencode: opencode run {prompt}"
harness = "not a harness"
harness = "claude: claude --model opus -n {name} {prompt}"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPASS_CONFIG", path)
	c := loadConfig()
	if len(c.Harnesses) != 2 || c.Harnesses[0].Name != "opencode" || c.Harnesses[1].Name != "claude" {
		t.Fatalf("Harnesses = %+v", c.Harnesses)
	}
	if got := strings.Join(c.Harnesses[1].Command, " "); got != "claude --model opus -n {name} {prompt}" {
		t.Errorf("claude = %q", got)
	}
}

// Each dir line is one favourite for `n`, in the file's order.
func TestLoadConfigDirs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("dir = \"~/work/api\"\ndir = '/srv/infra'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPASS_CONFIG", path)
	if got := loadConfig().launchConfig().Dirs; !reflect.DeepEqual(got, []string{"~/work/api", "/srv/infra"}) {
		t.Errorf("Dirs = %q", got)
	}
}

// The new_* lines set what the `n` panel opens on, and jump is handed to
// tmux whole. A value compass cannot read leaves the built-in default.
func TestLoadConfigNewDefaultsAndJump(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := `new_run = "shell"
new_as = "session"
new_worktree = true
new_then = "go"
jump = "display-popup -E 'tmux-jump ; true'"
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPASS_CONFIG", path)
	c := loadConfig()
	lc := c.launchConfig()
	if lc.Run != "shell" || lc.Place != tmuxop.PlaceSession || !lc.Worktree || lc.Then != "go" {
		t.Errorf("launch defaults = %+v", lc)
	}
	if c.Jump != "display-popup -E 'tmux-jump ; true'" {
		t.Errorf("Jump = %q", c.Jump)
	}

	if err := os.WriteFile(path, []byte("new_as = \"tab\"\nnew_then = \"maybe\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if lc := loadConfig().launchConfig(); lc.Place != tmuxop.PlaceWindow || lc.Then != "" {
		t.Errorf("unreadable values were kept: %+v", lc)
	}
}
