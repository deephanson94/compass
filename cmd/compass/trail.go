package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/transcript"
)

// runTrail is `compass trail`: one session's journey as JSON, for a reader
// that draws it somewhere other than the deck (a Claude Code mod, a script).
// Without -follow it prints one snapshot and exits; with it, a snapshot per
// line (NDJSON) each time the transcript grows, plus a heartbeat so a reader
// that shows "4m ago" has a reason to redraw.
func runTrail(args []string) int {
	fs := flag.NewFlagSet("compass trail", flag.ContinueOnError)
	root := fs.String("root", defaultRoot(), "Claude home directory to look in")
	session := fs.String("session", "", "session id: the transcript file's name, without .jsonl")
	path := fs.String("transcript", "", "transcript path, instead of -session")
	follow := fs.Bool("follow", false, "keep reading: one snapshot per line as the transcript grows")
	poll := fs.Duration("poll", 500*time.Millisecond, "with -follow, how often the transcript is read")
	beat := fs.Duration("heartbeat", 30*time.Second, "with -follow, the longest gap between snapshots")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *path == "" && *session == "" {
		fmt.Fprintln(os.Stderr, "compass trail: -session or -transcript is required")
		return 2
	}
	// A session's file appears with its first prompt, which can be after a
	// mod starts us: until it does, every read looks for it again.
	resolve := func() bool {
		if *path != "" {
			return true
		}
		*path = findTranscript(expandHome(*root), *session)
		return *path != ""
	}

	out := bufio.NewWriter(os.Stdout)
	var t *transcript.Tailer
	seg := journey.NewSegmenter()
	outs := journey.NewOutcomes()
	read := func() (bool, error) {
		if t == nil {
			if !resolve() {
				return false, nil
			}
			t = transcript.NewTailer(*path)
		}
		evs, err := t.Poll()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		for _, ev := range evs {
			seg.Observe(ev)
			outs.Observe(ev)
		}
		return len(evs) > 0, nil
	}
	emit := func() error {
		if err := writeSnapshot(out, *path, seg, outs, time.Now()); err != nil {
			return err
		}
		return out.Flush()
	}

	if _, err := read(); err != nil {
		fmt.Fprintln(os.Stderr, "compass trail:", err)
		return 1
	}
	if err := emit(); err != nil {
		return 1
	}
	if !*follow {
		return 0
	}
	last := time.Now()
	for {
		time.Sleep(*poll)
		grew, err := read()
		if err != nil {
			fmt.Fprintln(os.Stderr, "compass trail:", err)
			return 1
		}
		if !grew && time.Since(last) < *beat {
			continue
		}
		// A reader that closed the pipe is done with us.
		if err := emit(); err != nil {
			return 0
		}
		last = time.Now()
	}
}

// findTranscript is the session's file under projects/, whichever project
// directory holds it, or "" while no project holds it yet.
func findTranscript(root, id string) string {
	matches, _ := filepath.Glob(filepath.Join(root, "projects", "*", id+".jsonl"))
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

// The snapshot's shape is the contract with every reader outside Go: names
// are spelled out, times are RFC 3339, and nothing is pre-rendered — the
// reader picks glyphs and widths for the room it has.
type trailSnapshot struct {
	Transcript string             `json:"transcript"`
	Now        time.Time          `json:"now"`
	Prompts    []snapPrompt       `json:"prompts"`
	Legs       []snapLeg          `json:"legs"`
	Branches   []snapBranch       `json:"branches"`
	Tasks      []snapTask         `json:"tasks"`
	Outcome    *snapOutcome       `json:"outcome,omitempty"`
	Counts     map[string]snapSum `json:"counts"`
}

type snapPrompt struct {
	Text     string    `json:"text"`
	At       time.Time `json:"at"`
	Relayed  bool      `json:"relayed,omitempty"`
	Teammate string    `json:"teammate,omitempty"`
}

type snapLeg struct {
	Class     string         `json:"class"`
	Label     string         `json:"label"`
	Start     time.Time      `json:"start"`
	End       time.Time      `json:"end"`
	Current   bool           `json:"current,omitempty"`
	Files     []string       `json:"files,omitempty"`
	Waypoints []snapWaypoint `json:"waypoints,omitempty"`
}

type snapWaypoint struct {
	Kind  string    `json:"kind"`
	Text  string    `json:"text"`
	Short string    `json:"short,omitempty"`
	At    time.Time `json:"at"`
	Runs  int       `json:"runs,omitempty"`
}

type snapBranch struct {
	Label    string     `json:"label"`
	Start    time.Time  `json:"start"`
	End      *time.Time `json:"end,omitempty"`
	Done     bool       `json:"done"`
	AfterLeg int        `json:"afterLeg"`
	Report   string     `json:"report,omitempty"`
}

type snapTask struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Active  string `json:"active,omitempty"`
	Status  string `json:"status"`
}

type snapOutcome struct {
	Kind  string    `json:"kind"`
	Text  string    `json:"text"`
	Short string    `json:"short,omitempty"`
	At    time.Time `json:"at"`
}

// snapSum is one class's line of the block: how many legs, how long, and for
// test how many of its runs came back red.
type snapSum struct {
	Legs    int     `json:"legs"`
	Seconds float64 `json:"seconds"`
	Red     int     `json:"red,omitempty"`
}

func waypointKind(k journey.WaypointKind) string {
	switch k {
	case journey.WaypointTestRun:
		return "testRun"
	case journey.WaypointTestFail:
		return "testFail"
	case journey.WaypointBug:
		return "bug"
	case journey.WaypointCommit:
		return "commit"
	}
	return "unknown"
}

func snapshot(path string, tr journey.Trail, out *journey.Outcomes, now time.Time) trailSnapshot {
	s := trailSnapshot{
		Transcript: path,
		Now:        now,
		Prompts:    []snapPrompt{},
		Legs:       []snapLeg{},
		Branches:   []snapBranch{},
		Tasks:      []snapTask{},
		Counts:     map[string]snapSum{},
	}
	for _, p := range tr.Prompts {
		s.Prompts = append(s.Prompts, snapPrompt{Text: p.Text, At: p.At, Relayed: p.Relayed, Teammate: p.Teammate})
	}
	for _, l := range tr.Legs {
		leg := snapLeg{Class: l.Class.String(), Label: l.Label, Start: l.Start, End: l.End, Current: l.Current, Files: l.Files}
		red := false
		for _, w := range l.Waypoints {
			leg.Waypoints = append(leg.Waypoints, snapWaypoint{Kind: waypointKind(w.Kind), Text: w.Text, Short: w.Short, At: w.At, Runs: w.Runs})
			if w.Kind == journey.WaypointTestFail {
				red = true
			}
		}
		s.Legs = append(s.Legs, leg)

		sum := s.Counts[leg.Class]
		sum.Legs++
		sum.Seconds += l.End.Sub(l.Start).Seconds()
		if red {
			sum.Red++
		}
		s.Counts[leg.Class] = sum
	}
	for _, b := range tr.Branches {
		br := snapBranch{Label: b.Label, Start: b.Start, Done: b.Done, AfterLeg: b.AfterLeg, Report: b.Report}
		if b.Done {
			end := b.End
			br.End = &end
		}
		s.Branches = append(s.Branches, br)
	}
	for _, t := range tr.Tasks {
		if t.Status == "deleted" {
			continue
		}
		s.Tasks = append(s.Tasks, snapTask{ID: t.ID, Subject: t.Subject, Active: t.Active, Status: t.Status})
	}
	if o, ok := out.Latest(); ok {
		s.Outcome = &snapOutcome{Kind: waypointKind(o.Kind), Text: o.Text, Short: o.Short, At: o.At}
	}
	return s
}

func writeSnapshot(w io.Writer, path string, seg *journey.Segmenter, out *journey.Outcomes, now time.Time) error {
	return json.NewEncoder(w).Encode(snapshot(path, seg.Trail(), out, now))
}
