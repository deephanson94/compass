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
	"regexp"
	"strings"
	"time"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/transcript"
)

// trailProtocol numbers the snapshot's shape for readers outside Go. It
// goes up when a field a reader relies on changes meaning or goes away;
// an added field leaves it alone. `compass -version` prints it.
const trailProtocol = 1

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
	acts := &actLog{}
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
			acts.observe(ev)
		}
		return len(evs) > 0, nil
	}
	emit := func() error {
		if err := writeSnapshot(out, *path, seg, outs, acts, time.Now()); err != nil {
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
	// Acts are what the leg did, in words: a Bash call's description or
	// command, a file it edited or read. The evidence a narrator names the
	// leg from when files and waypoints say nothing — a leg that edits
	// through a heredoc touches no file the vote table can see.
	Acts []string `json:"acts,omitempty"`
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

func snapshot(path string, tr journey.Trail, out *journey.Outcomes, acts *actLog, now time.Time) trailSnapshot {
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
		leg := snapLeg{Class: l.Class.String(), Label: l.Label, Start: l.Start, End: l.End, Current: l.Current, Files: l.Files, Acts: acts.within(l.Start, l.End)}
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

func writeSnapshot(w io.Writer, path string, seg *journey.Segmenter, out *journey.Outcomes, acts *actLog, now time.Time) error {
	return json.NewEncoder(w).Encode(snapshot(path, seg.Trail(), out, acts, now))
}

// actLog keeps the main thread's tool calls as one line each, in file order,
// for the legs to claim by time.
type actLog struct {
	acts []act
}

type act struct {
	at   time.Time
	text string
}

// maxActs bounds the log: a day-long session makes thousands of calls, and
// a leg claims only its own.
const maxActs = 4000

// maxLegActs is how many distinct acts one leg carries: enough to name it,
// few enough to batch many legs into one narration.
const maxLegActs = 8

func (a *actLog) observe(ev transcript.Event) {
	if ev.IsSidechain {
		return
	}
	for _, use := range ev.ToolUses {
		if text := actText(use); text != "" {
			a.acts = append(a.acts, act{at: ev.Timestamp, text: text})
		}
	}
	if len(a.acts) > maxActs {
		a.acts = append([]act(nil), a.acts[len(a.acts)-maxActs:]...)
	}
}

// within is the distinct acts in [start, end], oldest first.
func (a *actLog) within(start, end time.Time) []string {
	var out []string
	seen := map[string]bool{}
	for _, x := range a.acts {
		if x.at.Before(start) || x.at.After(end) || seen[x.text] {
			continue
		}
		seen[x.text] = true
		out = append(out, x.text)
		if len(out) == maxLegActs {
			break
		}
	}
	return out
}

// preamble is what a command line opens with before its work: variable
// assignments and a change of directory, each ended by ; or &&.
var preamble = regexp.MustCompile(`^\s*(?:(?:[A-Za-z_][A-Za-z0-9_]*=(?:'[^']*'|"[^"]*"|\S*)|cd\s+\S+)\s*(?:;|&&)\s*)+`)

// commandCore is a command without its preamble: "M=/long/path; cat $M/x"
// reads "cat $M/x", which says what the call was for.
func commandCore(cmd string) string {
	if core := preamble.ReplaceAllString(cmd, ""); strings.TrimSpace(core) != "" {
		return core
	}
	return cmd
}

// pathLike is a file named in a command: a word with an extension of
// letters, the way scripts name the files they write.
var pathLike = regexp.MustCompile(`[\w./~-]+\.(?:go|ts|tsx|js|py|rs|java|rb|sh|json|ya?ml|toml|md|sql|css|html)\b`)

// namedFiles is up to three distinct basenames a command names: a heredoc's
// first line says nothing, the files it opens say what it changed.
func namedFiles(cmd string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range pathLike.FindAllString(cmd, -1) {
		base := filepath.Base(m)
		if seen[base] {
			continue
		}
		seen[base] = true
		out = append(out, base)
		if len(out) == 3 {
			break
		}
	}
	return out
}

// actText is one tool call in words. A Bash call's own description says
// what it was for, so it wins over the command it ran.
func actText(use transcript.ToolUse) string {
	var in struct {
		Command     string `json:"command"`
		Description string `json:"description"`
		FilePath    string `json:"file_path"`
		Pattern     string `json:"pattern"`
		Skill       string `json:"skill"`
	}
	_ = json.Unmarshal(use.Input, &in)
	clip := func(s string) string {
		s = strings.TrimSpace(strings.SplitN(s, "\n", 2)[0])
		if r := []rune(s); len(r) > 70 {
			return string(r[:69]) + "…"
		}
		return s
	}
	switch use.Name {
	case "Bash":
		if in.Description != "" {
			return clip(in.Description)
		}
		act := clip("$ " + commandCore(in.Command))
		if names := namedFiles(in.Command); len(names) > 0 {
			act += " [" + strings.Join(names, ", ") + "]"
		}
		return act
	case "Edit", "MultiEdit", "Write", "NotebookEdit":
		return "edit " + filepath.Base(in.FilePath)
	case "Read":
		return "read " + filepath.Base(in.FilePath)
	case "Grep", "Glob":
		return clip(strings.ToLower(use.Name) + " " + in.Pattern)
	case "Skill":
		return "skill " + in.Skill
	case "Agent", "Task", "TaskCreate", "TaskUpdate", "TaskList", "TaskGet", "ToolSearch":
		return ""
	}
	return use.Name
}
