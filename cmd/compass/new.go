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
// binding or a script. It opens one window and prints where.
//
//	compass new -C ~/work/api -n rate explore per-tenant rate limits
//	compass new -with shell -C ~/work/api
func runNew(args []string, cfg config) int {
	harnesses := launch.Harnesses(cfg.Harnesses)
	var names []string
	for _, h := range harnesses {
		names = append(names, h.Name)
	}
	fs := flag.NewFlagSet("compass new", flag.ContinueOnError)
	dir := fs.String("C", ".", "the project directory the window opens in")
	session := fs.String("t", "", "the tmux session the window opens in (default: the one you are in, else the first)")
	name := fs.String("n", "", "the window's name, and the worktree's branch (default: the prompt's first words)")
	with := fs.String("with", harnesses[0].Name, "what the window runs: "+strings.Join(names, ", "))
	worktree := fs.Bool("worktree", false, "check out a new branch in a worktree beside the repository and open there")
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
	runner := tmuxop.RealRunner{}
	if *session == "" {
		*session = currentSession(runner)
	}
	res, err := launch.Launch(runner, launch.RealGit{}, launch.Request{
		Session: *session, Dir: *dir, Name: *name, Harness: h, Prompt: prompt, Worktree: *worktree,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "compass new:", firstLineOf(err.Error()))
		return 1
	}
	fmt.Printf("%s · %s %s · %s\n", res.Name, h.Name, res.Pane.Target, res.Dir)
	return 0
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
