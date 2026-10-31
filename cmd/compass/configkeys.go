package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/deephanson94/compass/internal/launch"
	"github.com/deephanson94/compass/internal/tmuxop"
)

// The built-in defaults the flags fall back to. They live here, beside the
// keys, so `compass config` and the starter file name the same values the
// deck runs with.
const (
	defaultNarrator   = "haiku"
	defaultLiveWithin = "5m" // the recency door: a paneless session is live while it spoke this recently (M5 contract)
	defaultBoard      = "cards"
	defaultStillAfter = "10m"
	defaultAttnAfter  = "1h"
)

// configKey is one key the config file may hold. The table below is the
// only list of them: loadConfig reads the file through it, `compass config`
// prints it, `compass config init` writes it out commented, and a key that
// is not in it is reported as a typo. A new setting is one entry here and
// one field on config — nothing else has to learn its name.
type configKey struct {
	name    string
	aliases []string // old names still read
	section string   // the heading it is listed under
	help    string   // one line: what it does and what it takes
	// def is the value compass uses when the file says nothing, as the
	// file would spell it; "" is "none" — compass's own behaviour, which
	// help describes.
	def string
	// example is what the starter file offers where there is no default
	// to show: a key with neither is written as `key = ""`.
	example string
	repeat  bool // one line per value, in order (reply, dir, harness)
	// set reads one value into c. A non-empty return is why the value was
	// not taken; the line is then ignored and the built-in default stands.
	set func(c *config, value string) string
	// get is what c holds for the key, one string per value; nil or empty
	// when the file did not set it.
	get func(c config) []string
}

var configKeys = []configKey{
	{name: "root", section: "fleet", def: "~/.claude",
		help: "the Claude home directory to observe ($COMPASS_ROOT and -root override it)",
		set:  func(c *config, v string) string { c.Root = v; return "" },
		get:  func(c config) []string { return one(c.Root) }},
	{name: "narrator", section: "fleet", def: defaultNarrator,
		help: `the model that names the trail's legs: "haiku", any claude model, or "off"`,
		set:  func(c *config, v string) string { c.Narrator = v; return "" },
		get:  func(c config) []string { return one(c.Narrator) }},
	{name: "readonly", section: "fleet", def: "false",
		help: "true keeps compass's hands off tmux: no attach, no reply, no n, no t",
		set:  boolInto(func(c *config) *bool { return &c.Readonly }),
		get:  func(c config) []string { return oneBool(c.Readonly, c.has("readonly")) }},
	{name: "live_within", section: "fleet", def: defaultLiveWithin,
		help: `how recently a paneless session must have spoken to count as live; "0" = tmux panes only`,
		set:  durationInto(func(c *config) *string { return &c.LiveWithin }),
		get:  func(c config) []string { return one(c.LiveWithin) }},
	{name: "still_after", section: "fleet", def: defaultStillAfter,
		help: "the silence after which a working card says still working",
		set:  durationInto(func(c *config) *string { return &c.StillAfter }),
		get:  func(c config) []string { return one(c.StillAfter) }},
	{name: "attn_after", aliases: []string{"hung_after"}, section: "fleet", def: defaultAttnAfter,
		help: "the silence after which a card says needs attn (hung_after is its old name)",
		set:  durationInto(func(c *config) *string { return &c.AttnAfter }),
		get:  func(c config) []string { return one(c.AttnAfter) }},
	{name: "hook", section: "fleet", example: `tmux display-message "compass: $COMPASS_SESSION $COMPASS_EVENT"`,
		help: "a command run when a session crosses a line while nobody is looking ($COMPASS_EVENT, $COMPASS_SESSION, $COMPASS_TMUX, $COMPASS_DETAIL)",
		set:  func(c *config, v string) string { c.Hook = v; return "" },
		get:  func(c config) []string { return one(c.Hook) }},

	{name: "board", section: "board", def: defaultBoard,
		help: `the board's shape: "cards" (today, newest first) or "columns" (every trail, oldest first)`,
		set: func(c *config, v string) string {
			if v != "cards" && v != "columns" {
				return "want cards or columns"
			}
			c.Board = v
			return ""
		},
		get: func(c config) []string { return one(c.Board) }},
	{name: "card_width", section: "board", def: "84",
		help: "a card's least width; lower it for more cards across",
		set:  intInto(func(c *config) *int { return &c.CardWidth }),
		get:  func(c config) []string { return oneInt(c.CardWidth, c.has("card_width")) }},
	{name: "cards_per_row", section: "board", def: "0",
		help: "cards in a row; 0 = as many as card_width allows (+ and - change it live)",
		set:  intInto(func(c *config) *int { return &c.CardsRow }),
		get:  func(c config) []string { return oneInt(c.CardsRow, c.has("cards_per_row")) }},
	{name: "card_tint", section: "board", example: "#1f2b30",
		help: "a card's ground inside its border, #rrggbb; unset = a step off your terminal's",
		set:  func(c *config, v string) string { c.CardTint = v; return "" },
		get:  func(c config) []string { return one(c.CardTint) }},
	{name: "dim", section: "board", example: "#9a9a9a",
		help: "the grey of the quiet rows: #rrggbb, #rgb or a palette index 0-255 ($COMPASS_DIM overrides it)",
		set:  func(c *config, v string) string { c.Dim = v; return "" },
		get:  func(c config) []string { return one(c.Dim) }},
	{name: "mirror", section: "board", def: "false",
		help: "true opens the live tmux pane beside the trail (m toggles it)",
		set:  boolInto(func(c *config) *bool { return &c.Mirror }),
		get:  func(c config) []string { return oneBool(c.Mirror, c.has("mirror")) }},
	{name: "reply", section: "board", repeat: true, example: "please continue",
		help: "a stock line r offers, one line each, up to nine; any replace compass's own",
		set: func(c *config, v string) string {
			if len(c.Replies) >= 9 {
				return "r offers nine at most"
			}
			c.Replies = append(c.Replies, v)
			return ""
		},
		get: func(c config) []string { return c.Replies }},

	{name: "new_run", section: "new work (n, compass new)", def: "claude",
		help: "what n's panel starts on: claude, shell, or a harness's name",
		set:  func(c *config, v string) string { c.NewRun = v; return "" }, // checked against the harnesses once all are read
		get:  func(c config) []string { return one(c.NewRun) }},
	{name: "new_as", section: "new work (n, compass new)", def: "window",
		help: "where new work opens: window, pane (a split beside the card's) or session",
		set: func(c *config, v string) string {
			p, ok := tmuxop.ParsePlace(v)
			if !ok {
				return "want window, pane or session"
			}
			c.NewAs = p
			return ""
		},
		get: func(c config) []string {
			if !c.has("new_as") {
				return nil
			}
			return []string{c.NewAs.String()}
		}},
	{name: "new_worktree", section: "new work (n, compass new)", def: "false",
		help: "true starts the worktree row on: a new branch beside the repo",
		set:  boolInto(func(c *config) *bool { return &c.NewWorktree }),
		get:  func(c config) []string { return oneBool(c.NewWorktree, c.has("new_worktree")) }},
	{name: "new_then", section: "new work (n, compass new)", example: "go",
		help: "go takes you to the new pane, stay keeps you on the board; unset = go for a shell or harness, stay for claude",
		set: func(c *config, v string) string {
			if v != "go" && v != "stay" {
				return "want go or stay"
			}
			c.NewThen = v
			return ""
		},
		get: func(c config) []string { return one(c.NewThen) }},
	{name: "dir", section: "new work (n, compass new)", repeat: true, example: "~/work/api",
		help: "a favourite directory for n's dir row (★), one line each, offered in this order",
		set:  func(c *config, v string) string { c.Dirs = append(c.Dirs, v); return "" },
		get:  func(c config) []string { return c.Dirs }},
	{name: "harness", section: "new work (n, compass new)", repeat: true, example: "opencode: opencode run {prompt}",
		help: "something n can run besides claude and a shell: a name, a colon, the command ({prompt}, {name})",
		set: func(c *config, v string) string {
			h, ok := launch.ParseHarness(v)
			if !ok {
				return "want name: command"
			}
			c.Harnesses = append(c.Harnesses, h)
			return ""
		},
		get: func(c config) []string {
			var out []string
			for _, h := range c.Harnesses {
				out = append(out, h.Name+": "+strings.Join(h.Command, " "))
			}
			return out
		}},
	{name: "jump", section: "new work (n, compass new)", def: tmuxop.DefaultJump,
		help: "the tmux command t runs once tmux has the terminal; copy your own prefix binding's here",
		set:  func(c *config, v string) string { c.Jump = v; return "" },
		get:  func(c config) []string { return one(c.Jump) }},
}

// configKeyNamed is the key a file line names, by its name or an old one.
func configKeyNamed(name string) (configKey, bool) {
	for _, k := range configKeys {
		if k.name == name {
			return k, true
		}
		for _, a := range k.aliases {
			if a == name {
				return k, true
			}
		}
	}
	return configKey{}, false
}

// configCheck is what can only be judged once the whole file is read: a
// value that names another key's.
func configCheck(c *config) []string {
	var out []string
	if c.NewRun != "" {
		if _, ok := launch.Find(launch.Harnesses(c.Harnesses), c.NewRun); !ok {
			var names []string
			for _, h := range launch.Harnesses(c.Harnesses) {
				names = append(names, h.Name)
			}
			out = append(out, fmt.Sprintf("new_run = %q: no such harness (want %s)", c.NewRun, strings.Join(names, ", ")))
			c.NewRun = ""
		}
	}
	return out
}

// suggestKey is the known key a mistyped one most likely meant: the
// nearest by edit distance, within two edits, or one the typo begins or
// ends. "" when nothing is close.
func suggestKey(typo string) string {
	best, bestD := "", 3
	for _, k := range configKeys {
		for _, n := range append([]string{k.name}, k.aliases...) {
			d := editDistance(typo, n)
			if len(typo) >= 3 && (strings.HasPrefix(n, typo) || strings.HasPrefix(typo, n)) && d > 1 {
				d = 2 // a key cut short or run on is as good a guess as two typos
			}
			if d < bestD {
				best, bestD = k.name, d
			}
		}
	}
	return best
}

// editDistance is the Levenshtein distance between two short ASCII words.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func one(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}

func oneBool(b, set bool) []string {
	if !set {
		return nil
	}
	return []string{strconv.FormatBool(b)}
}

func oneInt(n int, set bool) []string {
	if !set {
		return nil
	}
	return []string{strconv.Itoa(n)}
}

func boolInto(field func(c *config) *bool) func(*config, string) string {
	return func(c *config, v string) string {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return "want true or false"
		}
		*field(c) = b
		return ""
	}
}

func intInto(field func(c *config) *int) func(*config, string) string {
	return func(c *config, v string) string {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return "want a whole number"
		}
		*field(c) = n
		return ""
	}
}

func durationInto(field func(c *config) *string) func(*config, string) string {
	return func(c *config, v string) string {
		if _, err := time.ParseDuration(v); err != nil {
			return `want a duration like "90s", "10m" or "1h"`
		}
		*field(c) = v
		return ""
	}
}
