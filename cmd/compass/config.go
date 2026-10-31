package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deephanson94/compass/internal/launch"
	"github.com/deephanson94/compass/internal/tmuxop"
)

// config is what ~/.config/compass/config.toml may set. Every entry has a
// flag; a flag given on the command line always wins. The file is a flat
// `key = "value"` list — the TOML subset a hand and a text editor produce.
// The keys are the configKeys table (configkeys.go). A line compass cannot
// use is skipped and said in Warnings: compass runs fine with no
// configuration at all, and a typo is reported rather than silently lost.
type config struct {
	Root       string // root = "~/.claude"
	Narrator   string // narrator = "haiku" | "off" | any claude model
	Readonly   bool   // readonly = true (enter no longer attaches)
	LiveWithin string // live_within = "5m" ("0" = tmux panes only)
	Mirror     bool   // mirror = true (the live mirror opens at Lv1; m toggles it)
	Board      string // board = "cards" | "columns" (the default)
	StillAfter string // still_after = "10m": the silence a card calls still working
	AttnAfter  string // attn_after = "1h": the silence a card calls needs attn
	CardWidth  int    // card_width = "84": a card's least width
	CardsRow   int    // cards_per_row = "3": cards in a row (0 = by the width)
	CardTint   string // card_tint = "#2c3640": a card's ground; its bar is lit from it

	// Dim is the grey the quiet rows are drawn in: `dim = "#c0c0c0"`, or a
	// palette index. compass picks a neutral grey that clears 4.4:1 against
	// the darkest popular dark theme; this is for the terminal, the theme or
	// the pair of eyes that wants more. $COMPASS_DIM overrides the file.
	Dim string

	// Replies are the quick replies `r` offers, one `reply = "…"` line each,
	// in order; up to nine. None configured means compass's own three.
	Replies []string

	// Hook is a command run on the moments that matter while nobody is
	// looking: `hook = "tmux display-message \"$COMPASS_SESSION: $COMPASS_EVENT\""`.
	// COMPASS_EVENT is needs_you, api_error, stuck, circling or agents_back;
	// COMPASS_SESSION, COMPASS_TMUX and COMPASS_DETAIL say which and what.
	Hook string

	// Harnesses are what `n` and `compass new` can run besides Claude Code
	// and a shell, one `harness = "name: command {prompt}"` line each; a
	// line naming a built-in replaces it (launch.Harnesses).
	Harnesses []launch.Harness

	// Dirs are the directories `n` offers right after the selected card's,
	// one `dir = "~/work/api"` line each, in order.
	Dirs []string

	// What the `n` panel opens on, and what `compass new` defaults to where
	// it has the same flag: new_run = "claude", new_as = "window" | "pane" |
	// "session", new_worktree = true, new_then = "go" | "stay". A value
	// compass cannot read is ignored, so the built-in default stands.
	NewRun      string
	NewAs       tmuxop.Place
	NewWorktree bool
	NewThen     string

	// Jump is the tmux command `t` runs once the terminal is tmux's:
	// `jump = "choose-tree -Zs"` (the default), or whatever your own
	// `prefix j` is bound to. tmux parses it, quotes and `;` included.
	Jump string

	// VerdictPattern is the closing line a session says how it came out on
	// (state.DefaultVerdictPattern unless set; "off" ignores verdicts).
	VerdictPattern string

	Path     string          // the file read, or the one that would be
	Exists   bool            // whether there was a file there
	Warnings []string        // "line 12: unknown key …", one per line not taken
	seen     map[string]bool // the keys the file set, by their table name
}

// has says whether the file set the key — false is a default, not a choice.
func (c config) has(name string) bool { return c.seen[name] }

// configPath is where the config file is: $COMPASS_CONFIG, else
// <user config dir>/compass/config.toml. "" when neither can be worked out.
func configPath() string {
	if p := os.Getenv("COMPASS_CONFIG"); p != "" {
		return p
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "compass", "config.toml")
}

// launchConfig is what the launch panel and `compass new` take from the file.
func (c config) launchConfig() launch.Config {
	return launch.Config{
		Harnesses: launch.Harnesses(c.Harnesses), Dirs: c.Dirs,
		Run: c.NewRun, Place: c.NewAs, Worktree: c.NewWorktree, Then: c.NewThen,
	}
}

// loadConfig reads the config file if there is one. $COMPASS_CONFIG overrides
// the location (and makes the file testable). The return is named so that the
// deferred block below has the last word on every path out of the function,
// including the two that give up before reading anything.
func loadConfig() (c config) {
	// The environment is read whether or not there is a file to read: a grey
	// you cannot see is the one setting you want to try without first finding
	// out where the config file goes.
	defer func() {
		if env := os.Getenv("COMPASS_DIM"); env != "" {
			c.Dim = env
		}
	}()

	c.Path = configPath()
	if c.Path == "" {
		return
	}
	f, err := os.Open(c.Path)
	if err != nil {
		return
	}
	defer f.Close()
	c.Exists = true
	c.seen = map[string]bool{}

	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		key, value, ok := configLine(sc.Text())
		if !ok {
			continue
		}
		k, known := configKeyNamed(key)
		if !known {
			w := fmt.Sprintf("line %d: unknown key %q", n, key)
			if s := suggestKey(key); s != "" {
				w += " (did you mean " + s + "?)"
			}
			c.Warnings = append(c.Warnings, w)
			continue
		}
		if why := k.set(&c, value); why != "" {
			c.Warnings = append(c.Warnings, fmt.Sprintf("line %d: %s = %q: %s", n, key, value, why))
			continue
		}
		c.seen[k.name] = true
	}
	c.Warnings = append(c.Warnings, configCheck(&c)...)
	return
}

// unquote reads a value: one quoted string, or the bare words up to a
// comment. Inside "…" a \" is a quote and a \\ a backslash, as in TOML;
// '…' is taken literally. Whatever follows the closing quote — the
// `# a favourite for n's dir row` the README's own example carries — is
// not the value. Trimming every quote off both ends instead kept that
// comment in the value, and ate a quote a hook or a jump ended on.
func unquote(v string) string {
	if v == "" {
		return v
	}
	switch v[0] {
	case '\'':
		if end := strings.IndexByte(v[1:], '\''); end >= 0 {
			return v[1 : 1+end]
		}
	case '"':
		var b strings.Builder
		for i := 1; i < len(v); i++ {
			switch c := v[i]; {
			case c == '\\' && i+1 < len(v) && (v[i+1] == '"' || v[i+1] == '\\'):
				b.WriteByte(v[i+1])
				i++
			case c == '"':
				return b.String()
			default:
				b.WriteByte(c)
			}
		}
	}
	// Bare, or a quote never closed: a comment starts at a # after a space,
	// so `dim = #9a9a9a` stays a colour.
	for i := 1; i < len(v); i++ {
		if v[i] == '#' && (v[i-1] == ' ' || v[i-1] == '\t') {
			return strings.TrimSpace(v[:i])
		}
	}
	return v
}

// configLine parses one `key = "value"` row; comments and blanks are nothing.
func configLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	value = unquote(strings.TrimSpace(value))
	if key == "" || value == "" {
		return "", "", false
	}
	return key, value, true
}
