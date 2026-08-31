package main

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/tmuxop"
)

// printPanes is `compass panes`: the pane→session mapping, shown. When the
// live fleet looks emptier than tmux does, this says which half is missing —
// the panes tmux reports, the claude each one holds, and the session that
// pairs with it. Read-only, like everything else compass does to tmux.
func printPanes(w io.Writer, mgr *fleet.Manager, root string, now time.Time) {
	runner := tmuxop.RealRunner{}
	proc := tmuxop.RealProc{}

	panes, err := tmuxop.ListPanes(runner)
	if err != nil {
		fmt.Fprintln(w, "tmux:", err)
		return
	}
	if len(panes) == 0 {
		fmt.Fprintln(w, "no tmux panes — is a server running? (tmux list-panes -a)")
		return
	}

	infos, err := fleet.Discover(root)
	if err != nil {
		fmt.Fprintln(w, "discover:", err)
		return
	}
	mapped := tmuxop.MapSessions(infos, panes, proc)

	// The pairing is keyed by transcript path (M6 contract); the report names
	// each session by its id, which is the label a human recognises and what
	// `claude --resume` takes. Inverted to pane-first: the way tmux lists them
	// and the way the fleet groups them.
	ids := make(map[string]string, len(infos))
	for _, info := range infos {
		ids[info.Key()] = info.ID
	}
	byPane := make(map[string]string, len(mapped))
	for key, pane := range mapped {
		byPane[pane.ID] = ids[key]
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PANE\tCOMMAND\tCLAUDE CWD\tSESSION")
	for _, pane := range panes {
		cwd, ok := tmuxop.ClaudeCwd(proc, pane.PID)
		claude := cwd
		if !ok {
			claude = "—"
		}
		session := byPane[pane.ID]
		if session == "" {
			session = "—"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", pane.Target, pane.Command, claude, session)
	}
	tw.Flush()

	fmt.Fprintf(w, "\n%d panes · %d transcripts · %d paired\n", len(panes), len(infos), len(mapped))
	if len(mapped) == 0 {
		fmt.Fprintln(w, "\nnothing paired. compass finds a session by looking for a claude")
		fmt.Fprintln(w, "process under each pane and matching its cwd to the transcript's.")
		fmt.Fprintln(w, "A blank CLAUDE CWD column means no claude was found under that pane;")
		fmt.Fprintln(w, "a filled one that pairs with nothing means no transcript names it.")
	}
}
