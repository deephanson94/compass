package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/transcript"
)

// A subagent's own transcript — Claude Code writes each one beside the
// lead's, under <session-id>/subagents/agent-<id>.jsonl with a meta.json
// naming the Agent call it answers — is the only evidence of whether the
// agent is alive. The lead's transcript is silent while its agents are out,
// so every clock the deck had for a lane was the lead's silence restated
// (#49). This file reads the agents' files: paired to a lane by the call's
// id, never by its description, and rendered only where a file was read.

// agentLive is what one agent's transcript says right now: when it last
// wrote, and the state machine's verdict on it — working with its call in
// flight, or hung past the threshold, by the same rules a session is judged.
type agentLive struct {
	Wrote  time.Time      // the last event's timestamp; zero when nothing is written yet
	Snap   state.Snapshot // Working or Stuck, with the call in flight as its activity
	Events []transcript.Event
}

// agentSilent says the agent's file has gone quiet past the threshold.
func (a agentLive) silent() bool { return a.Snap.State == state.Stuck }

// agentDir is where a session's subagent transcripts live: beside the
// transcript, under a directory named by its id.
func agentDir(transcriptPath string) string {
	if transcriptPath == "" {
		return ""
	}
	return filepath.Join(strings.TrimSuffix(transcriptPath, ".jsonl"), "subagents")
}

// pairAgents reads the meta files in dir and maps each Agent call's id to
// its transcript. An unreadable directory pairs nothing: a lane with no
// file renders as it always did.
func pairAgents(dir string) map[string]string {
	if dir == "" {
		return nil
	}
	metas, err := filepath.Glob(filepath.Join(dir, "agent-*.meta.json"))
	if err != nil || len(metas) == 0 {
		return nil
	}
	out := make(map[string]string, len(metas))
	for _, meta := range metas {
		raw, err := os.ReadFile(meta)
		if err != nil {
			continue
		}
		var m struct {
			ToolUseID string `json:"toolUseId"`
		}
		if json.Unmarshal(raw, &m) != nil || m.ToolUseID == "" {
			continue
		}
		jsonl := strings.TrimSuffix(meta, ".meta.json") + ".jsonl"
		if _, err := os.Stat(jsonl); err == nil {
			out[m.ToolUseID] = jsonl
		}
	}
	return out
}

// agentFeed is one agent's tailer and the machine it feeds: the feed store
// keeps it beside the session's own feed, for as long as the session's.
type agentFeed struct {
	polled time.Time
	tailer *transcript.Tailer
	mach   *state.Machine
	wrote  time.Time
	events []transcript.Event
	keeps  bool
}

// pollAgent reads what the agent has written since the last call and
// returns its liveness as of now. wantEvents keeps the conversation, for
// the lane the reader is on.
func (fs *feedStore) pollAgent(key, toolUseID, path string, now time.Time, wantEvents bool) agentLive {
	if fs == nil || key == "" || toolUseID == "" || path == "" {
		return agentLive{}
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.agents == nil {
		fs.agents = make(map[string]*agentFeed)
	}
	id := key + "\x00" + toolUseID
	f := fs.agents[id]
	if f == nil || f.tailer.Path() != path || (wantEvents && !f.keeps) {
		f = &agentFeed{tailer: transcript.NewTailer(path), mach: state.NewMachine(), keeps: wantEvents}
		fs.agents[id] = f
	}
	f.polled = time.Now()
	events, err := f.tailer.Poll()
	if err == nil {
		for _, ev := range events {
			f.mach.Observe(ev)
			if ev.Timestamp.After(f.wrote) {
				f.wrote = ev.Timestamp
			}
		}
		if f.keeps {
			for _, ev := range events {
				// The agent's own conversation is its main chain from
				// where the reader stands: the flag that hides it from
				// the lead's document comes off.
				ev.IsSidechain = false
				f.events = append(f.events, ev)
			}
			if over := len(f.events) - feedEventCap; over > 0 {
				f.events = append([]transcript.Event(nil), f.events[over:]...)
			}
		}
	}
	live := agentLive{Wrote: f.wrote, Snap: f.mach.Evaluate(now)}
	if f.keeps {
		live.Events = f.events
	}
	return live
}

// agentsFor is what the deck knows of a session's agents: nil when it read
// no file for it.
func (m *Model) agentsFor(key string) map[string]agentLive {
	return m.agents[key]
}

// laneWanted is the lane whose conversation the reader wants kept: the one
// the reader is open on, else the one the Lv2 cursor stands on.
func (m *Model) laneWanted() string {
	if m.readerLane != "" {
		return m.readerLane
	}
	if m.level == levelWaypoints && m.cursor >= 0 {
		if rows := TrailRows(m.trail, m.level); m.cursor < len(rows) && rows[m.cursor].Kind == "branch" {
			return rows[m.cursor].Lane
		}
	}
	return ""
}

// laneHead is the one row an open lane's own file earns beneath it: the
// call in flight and when the agent last wrote, in the words a HEAD uses
// — "● Bash python dla.py …  wrote 40s ago", "◍ Bash pytest -x  silent
// 12m" — or that nothing is written yet. "" when no file was read.
func laneHead(a agentLive, ok bool, now time.Time) (glyph, text, clock string) {
	if !ok {
		return "", "", ""
	}
	if a.Wrote.IsZero() {
		return "⋯", "nothing written yet", ""
	}
	act := strings.TrimSpace(a.Snap.Activity)
	if act == "" || act == "idle" {
		act = "thinking…"
	}
	if a.silent() {
		return "◍", act, "silent " + relAge(now, a.Wrote)
	}
	return "●", act, "wrote " + relAge(now, a.Wrote) + " ago"
}

// lanesLive adds up the open lanes' own clocks for a parked HEAD: how many
// are silent and the longest silence, else how recently the newest wrote.
// ok is false when no lane has a file, and the old clause stands.
func lanesLive(agents map[string]agentLive, lanes []string, now time.Time) (silent int, longest time.Duration, newest time.Duration, ok bool) {
	for _, id := range lanes {
		a, found := agents[id]
		if !found {
			continue
		}
		ok = true
		if a.silent() {
			silent++
			if d := now.Sub(a.Wrote); d > longest {
				longest = d
			}
		}
		if !a.Wrote.IsZero() {
			if d := now.Sub(a.Wrote); newest == 0 || d < newest {
				newest = d
			}
		}
	}
	return silent, longest, newest, ok
}

// lanesClause is the parked HEAD's second half from its agents' own files:
// "1 silent 12m" — the hung one is the only actionable fact, so it
// displaces the rest whole — else "newest 40s ago". "" when no file
// was read.
func lanesClause(agents map[string]agentLive, lanes []string, now time.Time) string {
	silent, longest, newest, ok := lanesLive(agents, lanes, now)
	switch {
	case !ok:
		return ""
	case silent > 0:
		return fmt.Sprintf("%d silent %s", silent, state.ShortDuration(longest))
	case newest > 0:
		return "newest " + state.ShortDuration(newest) + " ago"
	}
	return "nothing written yet"
}
