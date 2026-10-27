package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/state"
)

// printWhy is `compass why`: for every live session, the verdict and the
// facts it was reached from — the calls the transcript holds open, and the
// last line the machine counted as someone speaking. A card that says
// "needs attn" over a session waiting on nobody is the machine misreading a
// line; this names the line.
func printWhy(w io.Writer, mgr *fleet.Manager, sessions []fleet.Session, now time.Time) {
	n := 0
	for _, s := range sessions {
		if !s.Live {
			continue
		}
		n++
		name := s.Info.Name
		if name == "" {
			name = filepath.Base(s.Info.CWD)
		}
		snap := s.Snap
		fmt.Fprintf(w, "%s  [%s]  %s\n", name, s.Info.GitBranch, s.Info.ID)
		fmt.Fprintf(w, "  state    %s%s · %q\n", snap.State, whyFlags(s), snap.Reason)
		fmt.Fprintf(w, "  file     %s\n", s.Info.TranscriptPath)
		fmt.Fprintf(w, "  last     line %s ago\n", whyAge(now, s.Info.LastEventAt))
		f, ok := mgr.Fold(s.Info.Key())
		if !ok {
			fmt.Fprintln(w)
			continue
		}
		for _, p := range f.Pending {
			fmt.Fprintf(w, "  open     %s %s, called %s ago · %s\n", p.Use.Name, p.Use.ID, whyAge(now, p.At), state.ActivityFor(p.Use))
		}
		if f.AwaitingModel {
			fmt.Fprintf(w, "  open     tool results in %s ago; the model has not answered\n", whyAge(now, f.AwaitingSince))
		}
		if f.SawSubstantive {
			fmt.Fprintf(w, "  beat     %s, %s ago: %q\n", f.SubstantiveKind, whyAge(now, f.SubstantiveAt), whyClip(f.SubstantiveText, 160))
		}
		fmt.Fprintln(w)
	}
	if n == 0 {
		fmt.Fprintln(w, "no live sessions")
	}
}

// whyFlags is the snapshot's qualifiers, as words.
func whyFlags(s fleet.Session) string {
	var out []string
	snap := s.Snap
	for _, f := range []struct {
		on   bool
		word string
	}{
		{snap.Open, "open"}, {snap.Late, "late"}, {snap.Exited, "exited"},
		{snap.Interrupted, "interrupted"}, {snap.APIError, "api-error"}, {s.Waiting, "waiting"},
	} {
		if f.on {
			out = append(out, f.word)
		}
	}
	if snap.Pending != "" {
		out = append(out, "pending="+snap.Pending)
	}
	if len(out) == 0 {
		return ""
	}
	return " (" + strings.Join(out, ", ") + ")"
}

func whyAge(now, t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	return state.ShortDuration(now.Sub(t))
}

func whyClip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
