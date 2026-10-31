package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/deephanson94/compass/internal/launch"
	"github.com/deephanson94/compass/internal/tmuxop"
)

// runNew is `compass new`: the deck's `n` panel as a command, for a tmux
// binding or a script. It opens one window, pane or session and prints where.
//
//	compass new -C ~/work/api -n rate explore per-tenant rate limits
//	compass new -with shell -as pane -C ~/work/api
func runNew(args []string, cfg config) int {
	lc := cfg.launchConfig()
	harnesses := lc.Harnesses
	run := harnesses[0].Name
	if _, ok := launch.Find(harnesses, lc.Run); ok {
		run = lc.Run
	}
	var names []string
	for _, h := range harnesses {
		names = append(names, h.Name)
	}
	fs := flag.NewFlagSet("compass new", flag.ContinueOnError)
	dir := fs.String("C", ".", "the project directory the work opens in")
	as := fs.String("as", lc.Place.String(), "where the work opens: window (in an existing session), pane (split beside one) or session (a new one)")
	target := fs.String("t", "", "for a window, the session it opens in (default: the one you are in, else the first);\nfor a pane, the pane it splits beside (default: the one you are in)")
	name := fs.String("n", "", "the window's name, a new session's, and the worktree's branch (default: the prompt's first words)")
	with := fs.String("with", run, "what the work runs: "+strings.Join(names, ", "))
	worktree := fs.Bool("worktree", lc.Worktree, "check out a new branch in a worktree beside the repository and open there")
	mkdir := fs.Bool("mkdir", false, "create the -C directory, empty, if it is not there")
	var env envFlag
	fs.Var(&env, "e", "KEY=VALUE set in the new pane's environment (tmux -e); repeat for more")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: compass new [flags] [prompt…]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	h, ok := launch.Find(harnesses, *with)
	if !ok {
		fmt.Fprintf(os.Stderr, "compass new: no harness %q (try %s)\n", *with, strings.Join(names, ", "))
		return 2
	}
	prompt := strings.Join(fs.Args(), " ")
	if prompt != "" && !h.TakesPrompt() {
		fmt.Fprintf(os.Stderr, "compass new: %s takes no prompt\n", h.Name)
		return 2
	}
	place, ok := tmuxop.ParsePlace(*as)
	if !ok {
		fmt.Fprintf(os.Stderr, "compass new: -as %q is no place (try window, pane or session)\n", *as)
		return 2
	}
	runner := tmuxop.RealRunner{}
	if *target == "" {
		switch place {
		case tmuxop.PlaceWindow:
			*target = currentSession(runner)
		case tmuxop.PlacePane:
			*target = currentPane(runner)
		}
	}
	res, err := launch.Launch(runner, launch.RealGit{}, launch.Request{
		Place: place, Target: *target, Dir: *dir, Name: *name, Harness: h, Prompt: prompt, Worktree: *worktree, MakeDir: *mkdir, Env: env,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "compass new:", firstLineOf(err.Error()))
		return 1
	}
	fmt.Printf("%s · %s %s · %s\n", res.Name, h.Name, res.Pane.Target, res.Dir)
	return 0
}

// envFlag is -e, given once per variable: KEY=VALUE, the key a shell
// variable's name. A value may hold anything, "=" included.
type envFlag []string

func (e *envFlag) String() string { return strings.Join(*e, " ") }

func (e *envFlag) Set(v string) error {
	key, _, ok := strings.Cut(v, "=")
	if !ok || key == "" || strings.IndexFunc(key, func(r rune) bool {
		return !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) >= 0 || key[0] >= '0' && key[0] <= '9' {
		return fmt.Errorf("want KEY=VALUE, got %q", v)
	}
	*e = append(*e, v)
	return nil
}

// currentSession is the tmux session this command runs in, else the
// server's first, else none.
func currentSession(r tmuxop.Runner) string {
	if os.Getenv("TMUX") != "" {
		if out, err := r.Output("display-message", "-p", "#{session_name}"); err == nil {
			if s := strings.TrimSpace(string(out)); s != "" {
				return s
			}
		}
	}
	if all := tmuxop.Sessions(r); len(all) > 0 {
		return all[0]
	}
	return ""
}

// currentPane is the pane this command runs in, or none outside tmux: a
// split has no sensible default anywhere else.
func currentPane(r tmuxop.Runner) string {
	if os.Getenv("TMUX") == "" {
		return ""
	}
	out, err := r.Output("display-message", "-p", "#{pane_id}")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
