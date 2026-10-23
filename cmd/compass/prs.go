package main

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/prstate"
	"github.com/deephanson94/compass/internal/tmuxop"
)

// printPRs is `compass prs`: one poll, now, whatever the interval, and
// everything it learned — who tea answers as, which checkouts each live
// session works in, what each search listed, every pull request and its
// verdict, and what went wrong. It is how a PR that does not show is
// found: the link, the search, or the judgement.
func printPRs(w io.Writer, sessions []fleet.Session, tr *prstate.Tracker, now time.Time) {
	if tr == nil {
		fmt.Fprintln(w, "PR state: off — -pr-interval is 0")
		return
	}
	dirs := liveDirs(sessions)
	err := tr.Poll(dirs, now)
	r := tr.Report()

	if r.Off != "" {
		fmt.Fprintln(w, "PR state: off —", r.Off)
		return
	}
	fmt.Fprintf(w, "tea answers as %s\n", r.Me)
	fmt.Fprintf(w, "searches: %d opened by you · %d asking your review · %d you reviewed\n", r.Found[0], r.Found[1], r.Found[2])

	// Which pull request each session shows, by key.
	shows := tr.States(now)
	fmt.Fprintf(w, "\nlive sessions (%d):\n", len(dirs))
	if len(dirs) == 0 {
		fmt.Fprintln(w, "  none — no Claude session sits in a tmux pane or wrote in the last -live-within (5m)")
	}
	var keys []string
	name := map[string]string{}
	for _, s := range sessions {
		if s.Live {
			keys = append(keys, s.Info.Key())
			name[s.Info.Key()] = sessionLabel(s)
		}
	}
	sort.Slice(keys, func(i, j int) bool { return name[keys[i]] < name[keys[j]] })
	for _, key := range keys {
		fmt.Fprintf(w, "  %s\n", name[key])
		fmt.Fprintf(w, "    looks in: %s\n", strings.Join(dirs[key], ", "))
		links := r.Links[key]
		if len(links) == 0 {
			fmt.Fprintln(w, "    checkouts: none — no directory above is in a git checkout with an origin")
		}
		for _, l := range links {
			fmt.Fprintf(w, "    checkout: %s @ %s\n", l.Repo, l.Branch)
		}
		if sts := shows[key]; len(sts) > 0 {
			fmt.Fprintf(w, "    shows: %s\n", prstate.Summary(sts, now))
		} else if len(links) > 0 {
			fmt.Fprintln(w, "    shows: nothing — no open PR has one of these repo @ branch pairs")
		}
	}

	fmt.Fprintf(w, "\npull requests (%d):\n", len(r.PRs))
	if len(r.PRs) == 0 {
		fmt.Fprintln(w, "  none")
	}
	for _, p := range r.PRs {
		verdict := p.Line(now)
		if p.Ignored {
			verdict = "not yours to review (never asked, never commented) — not shown"
		}
		fmt.Fprintf(w, "  %s#%d @ %s\n    %s\n", p.Repo, p.Number, p.HeadRef, verdict)
	}

	if len(r.Unlinked) > 0 {
		fmt.Fprintf(w, "\nyour PRs not shown — no live session works in their repository (%d):\n", len(r.Unlinked))
		fmt.Fprintf(w, "  %s\n", strings.Join(r.Unlinked, ", "))
	}

	if len(r.Errors) > 0 || err != nil {
		fmt.Fprintln(w, "\nerrors:")
		for _, e := range r.Errors {
			fmt.Fprintln(w, "  "+e)
		}
	}
	fmt.Fprintf(w, "\nstatus line: %s\n", strings.TrimSpace(prstate.Segment(livePRs(sessions, shows))+" "+prstate.ReviewSegment(tr.Reviews(now))))
}

// sessionLabel is how the diagnostic names a session: the name it was given,
// else its project directory, with its id to tell twins apart.
func sessionLabel(s fleet.Session) string {
	n := s.Info.Name
	if n == "" {
		n = filepath.Base(s.Info.CWD)
	}
	id := s.Info.ID
	if len(id) > 8 {
		id = id[:8]
	}
	return n + " (" + id + ")"
}

// livePaned is the fleet as the deck sees it: a session sitting in a tmux
// pane is live however long it has been quiet, not only one that wrote in
// the last few minutes. The fleet is read, paired with the panes, and read
// again with the pairing.
func livePaned(mgr *fleet.Manager, now time.Time) ([]fleet.Session, error) {
	sessions, err := mgr.Refresh(now)
	if err != nil {
		return nil, err
	}
	panes, err := tmuxop.ListPanes(tmuxop.RealRunner{})
	if err != nil || len(panes) == 0 {
		return sessions, nil // no tmux: the recency door is all there is
	}
	infos := make([]fleet.SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		infos = append(infos, s.Info)
	}
	keys := map[string]bool{}
	for key := range tmuxop.MapSessions(infos, panes, tmuxop.RealProc{}) {
		keys[key] = true
	}
	mgr.MarkPaneMapped(keys)
	return mgr.Refresh(now)
}
