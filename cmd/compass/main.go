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
	"github.com/deephanson94/compass/internal/ui"
)

func main() {
	args := os.Args[1:]

	// A subcommand, when present, comes first: `compass status -root …`.
	sub := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub, args = args[0], args[1:]
	}

	fs := flag.NewFlagSet("compass", flag.ExitOnError)
	root := fs.String("root", defaultRoot(), "Claude home directory to observe")
	fs.Usage = usage(fs)
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	mgr := fleet.NewManager(*root)

	switch sub {
	case "status":
		fmt.Println(mgr.StatusLine(time.Now()))
	case "help":
		fs.Usage()
	default:
		if err := ui.Run(mgr); err != nil {
			fmt.Fprintln(os.Stderr, "compass:", err)
			os.Exit(1)
		}
	}
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
		fmt.Fprintln(fs.Output(), "\nflags:")
		fs.PrintDefaults()
	}
}
