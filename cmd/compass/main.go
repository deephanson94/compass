// Command compass observes Claude Code sessions: a full-screen deck by
// default, or a one-shot `compass status` line for your own tmux status bar.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/narrator"
	"github.com/deephanson94/compass/internal/ui"
)

// defaultLiveWithin is the recency door's default width: a session with no
// tmux pane still counts as live while it spoke this recently (M5 contract).
const defaultLiveWithin = "5m"

func main() {
	args := os.Args[1:]

	// A subcommand, when present, comes first: `compass status -root …`.
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}

	cfg := loadConfig()
	rootDefault := defaultRoot()
	if cfg.Root != "" {
		rootDefault = expandHome(cfg.Root)
	}
	narratorDefault := "haiku"
	if cfg.Narrator != "" {
		narratorDefault = cfg.Narrator
	}
	// An unreadable live_within is ignored like any other bad config line; a bad
	// -live-within on the command line is a typo worth stopping for.
	liveDefault := defaultLiveWithin
	if _, err := time.ParseDuration(cfg.LiveWithin); err == nil {
		liveDefault = cfg.LiveWithin
	}

	fs := flag.NewFlagSet("compass", flag.ExitOnError)
	root := fs.String("root", rootDefault, "Claude home directory to observe")
	readonly := fs.Bool("readonly", cfg.Readonly, "never write to tmux: enter no longer attaches")
	model := fs.String("narrator", narratorDefault, `narration model for leg labels ("off" disables)`)
	liveWithin := fs.String("live-within", liveDefault,
		`how recently a paneless session must have spoken to count as live ("0" = tmux panes only)`)
	fs.Usage = usage(fs)
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	window, err := time.ParseDuration(*liveWithin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "compass: -live-within %q is not a duration (try 5m, 90s, 0)\n", *liveWithin)
		os.Exit(2)
	}

	mgr := fleet.NewManager(*root)
	mgr.SetLiveWindow(window)
	build := buildNarrator(mgr, *root, *model)

	switch sub {
	case "status":
		fmt.Println(mgr.StatusLine(time.Now()))
	case "panes":
		printPanes(os.Stdout, mgr, *root, time.Now())
	case "help":
		fs.Usage()
	default:
		if err := ui.Run(mgr, *readonly, build); err != nil {
			fmt.Fprintln(os.Stderr, "compass:", err)
			os.Exit(1)
		}
	}
}

// buildNarrator assembles the narration service: the claude CLI in headless
// mode, working out of a compass-private dir the fleet is told to ignore, and
// a label cache beside it. "off", or an unusable cache path, leaves the trail
// on its heuristic labels — narration is polish, never a requirement.
func buildNarrator(mgr *fleet.Manager, root, model string) func(notify func()) ui.Narrator {
	if model == "off" {
		return nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = root
	}
	dir := filepath.Join(base, "compass", "narrator")
	mgr.ExcludeCWD(dir)

	cache, err := narrator.OpenCache(filepath.Join(base, "compass", "labels.jsonl"))
	if err != nil {
		return nil
	}
	runner := &narrator.CLIRunner{Model: model, Dir: dir}
	return func(notify func()) ui.Narrator {
		return narrator.New(runner, cache, notify)
	}
}

// expandHome turns a leading ~/ into the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

// defaultRoot is $COMPASS_ROOT, else ~/.claude.
func defaultRoot() string {
	if r := os.Getenv("COMPASS_ROOT"); r != "" {
		return r
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude"
	}
	return filepath.Join(home, ".claude")
}

func usage(fs *flag.FlagSet) func() {
	return func() {
		fmt.Fprintln(fs.Output(), "compass — a panel next to your agents")
		fmt.Fprintln(fs.Output(), "\n  compass           the deck: every session, one glance")
		fmt.Fprintln(fs.Output(), "  compass status    one-shot fleet summary (▲1 ◍1 ●3)")
		fmt.Fprintln(fs.Output(), "  compass panes     which tmux pane holds which session (diagnostic)")
		fmt.Fprintln(fs.Output(), "\nflags:")
		fs.PrintDefaults()
	}
}
