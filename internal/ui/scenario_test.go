package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/deephanson94/compass/internal/fleet"
	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/state"
	"github.com/deephanson94/compass/internal/tmuxop"
	"github.com/deephanson94/compass/internal/transcript"
)

// Scenarios are whole fleets, built to be lived in rather than asserted on:
// the deck driven headlessly through the same Update path the keys take, so
// a reviewer — human or otherwise — can walk a workflow frame by frame.
//
//   COMPASS_SCENARIO_OUT=/tmp/scenes go test ./internal/ui -run Scenario
//
// writes one walkthrough per scenario and width. COMPASS_SCENARIO_KEYS
// ("j,j,tab,3,shift+tab") replaces the canonical key script with your own.
// Without the env var the test still runs every scenario at every width and
// holds the deck to its width discipline, so the fixtures cannot rot.

type scene struct {
	name     string
	story    string   // what the person at the keyboard is in the middle of
	extra    []string // keys pressed after the canonical walkthrough, for this scene's own case
	sessions []fleet.Session
	trails   map[string]journey.Trail
	panes    map[string]tmuxop.Pane
	order    []tmuxop.Pane
}

var sceneNow = fixtureBase.Add(6 * time.Hour)

// ---------------------------------------------------------------- builders

type legSpec struct {
	class journey.Class
	label string
	dur   time.Duration
	files []string
	tests string // "18✓ 2✗" makes a parsed run; "" none
	fails []string
}

// trailOf lays legs end to end from start, each after the last, with the
// prompt at the top and HEAD open when current is true.
func trailOf(start time.Time, prompt string, current bool, legs ...legSpec) journey.Trail {
	tr := journey.Trail{Prompts: []journey.Prompt{{Text: prompt, At: start}}}
	at := start.Add(time.Minute)
	for i, l := range legs {
		leg := journey.Leg{Class: l.class, Label: l.label, Start: at, End: at.Add(l.dur), Files: l.files, Votes: 3}
		if l.tests != "" {
			leg.Waypoints = append(leg.Waypoints, journey.Waypoint{Kind: journey.WaypointTestRun, Text: runSummary(l.tests), Short: l.tests, At: leg.End})
		}
		for _, f := range l.fails {
			runs := 1
			for _, earlier := range tr.Legs {
				for _, w := range earlier.Waypoints {
					if w.Kind == journey.WaypointTestFail && w.Text == f {
						runs++ // what the segmenter would have counted
					}
				}
			}
			leg.Waypoints = append(leg.Waypoints, journey.Waypoint{Kind: journey.WaypointTestFail, Text: f, Runs: runs, At: leg.End})
		}
		if l.class == journey.Ship {
			// A real ship leg carries its commit, and is named by it.
			leg.Waypoints = append(leg.Waypoints, journey.Waypoint{Kind: journey.WaypointCommit, Text: commitSubject(prompt) + " (" + l.label + ")", At: leg.End})
		}
		if current && i == len(legs)-1 {
			leg.Current = true
		}
		tr.Legs = append(tr.Legs, leg)
		at = leg.End.Add(30 * time.Second)
	}
	return tr
}

// commitSubject is the commit a session with this prompt would land.
func commitSubject(prompt string) string {
	words := strings.Fields(prompt)
	if len(words) > 6 {
		words = words[:6]
	}
	return strings.Join(words, " ")
}

// runSummary spells a badge the way the parser would: "18✓ 2✗" → "18 passed · 2 failed".
func runSummary(short string) string {
	var parts []string
	for _, f := range strings.Fields(short) {
		switch {
		case strings.HasSuffix(f, "✓"):
			parts = append(parts, strings.TrimSuffix(f, "✓")+" passed")
		case strings.HasSuffix(f, "✗"):
			parts = append(parts, strings.TrimSuffix(f, "✗")+" failed")
		}
	}
	return strings.Join(parts, " · ")
}

// eventsFor writes a plausible conversation behind a trail, so Lv3 has a
// document to read: the prompts as user turns; for every leg the model's
// own sentence, the tool call it made with its arguments, and the tool's
// result — output for a run, the file for a read, an error for a failure —
// so folding, scrolling and search have something to act on.
func eventsFor(tr journey.Trail) []transcript.Event {
	return eventsBehind(tr, "")
}

// eventsBehind is eventsFor with the session's activity: the call in flight
// on a live session, written as a tool_use no result has answered.
// withCompactions marks the moments a day-long session ran out of context.
func withCompactions(tr journey.Trail, at ...time.Time) journey.Trail {
	tr.Compactions = append(tr.Compactions, at...)
	return tr
}

func eventsBehind(tr journey.Trail, activity string) []transcript.Event {
	var evs []transcript.Event
	n := 0
	add := func(ev transcript.Event) {
		n++
		ev.UUID = fmt.Sprintf("e%d", n)
		ev.SessionID = "s"
		ev.CWD = "/home/user/src" // where the paths below are rooted, as a real line carries it
		evs = append(evs, ev)
	}
	for _, p := range tr.Prompts {
		add(transcript.Event{Type: transcript.EventUser, Timestamp: p.At, Text: p.Text})
	}
	for i, l := range tr.Legs {
		id := fmt.Sprintf("toolu_%d", i)
		file := l.Label
		if len(l.Files) > 0 {
			file = l.Files[0]
		}
		var said, tool, input, result string
		isErr := false
		switch l.Class {
		case journey.Scout:
			said = "Let me look at " + l.Label + " before changing anything."
			tool, input = "Read", fmt.Sprintf(`{"file_path":"/home/user/src/%s"}`, file)
			result = fmt.Sprintf("     1\tpackage main\n     2\t\n     3\timport \"os\"\n     4\t\n     5\tfunc main() {\n     6\t\t// %s\n     7\t}", l.Label)
		case journey.Design:
			said = "Two ways to do this. The narrower one keeps the current shape; the wider one touches every caller. I'll take the narrower one unless you say otherwise."
			tool, input = "AskUserQuestion", `{"questions":[{"question":"Narrow or wide?","options":[{"label":"narrow"},{"label":"wide"}]}]}`
			result = "narrow"
			if l.Current && strings.Contains(activity, "?") {
				// The open question is added below, unanswered; this leg
				// is that question, not one before it.
				continue
			}
		case journey.Build:
			said = "Writing " + l.Label + "."
			tool, input = "Edit", fmt.Sprintf(`{"file_path":"/home/user/src/%s","old_string":"return nil","new_string":"return s.store.Save(ctx, rec)"}`, file)
			result = "The file /home/user/src/" + file + " has been updated."
		case journey.Fix:
			said = "The failure is " + l.Label + ". Fixing it at the source rather than in the test."
			tool, input = "Edit", fmt.Sprintf(`{"file_path":"/home/user/src/%s","old_string":"time.Now()","new_string":"time.Now().UTC()"}`, file)
			result = "The file /home/user/src/" + file + " has been updated."
		case journey.Test:
			said = "Running the suite."
			tool, input = "Bash", `{"command":"pytest tests/ -x -q"}`
			result = "............................................................\n" + runSummaryFor(l) + " in 4.21s"
			if strings.HasPrefix(l.Label, "go test") {
				// The leg says go test; the reader should not say pytest.
				tool, input = "Bash", `{"command":"go test ./..."}`
				result = "ok  \tgithub.com/example/cli\t4.21s\n" + runSummaryFor(l)
			}
			isErr = strings.Contains(legBadge(l), "✗")
		case journey.Ship:
			said = "Committing."
			tool, input = "Bash", `{"command":"git add -A && git commit -q -m '`+l.Label+`' && git push -u origin HEAD"}`
			result = "[feat 3f2a9c1] " + l.Label + "\n 3 files changed, 42 insertions(+), 7 deletions(-)"
		case journey.Docs:
			said = "Writing it down in " + l.Label + "."
			tool, input = "Write", fmt.Sprintf(`{"file_path":"/home/user/src/%s","content":"# %s\n"}`, file, l.Label)
			result = "The file /home/user/src/" + file + " has been created."
		}
		add(transcript.Event{Type: transcript.EventAssistant, Timestamp: l.Start, Text: said,
			ToolUses: []transcript.ToolUse{{ID: id, Name: tool, Input: json.RawMessage(input)}}})
		at := l.End
		if at.IsZero() {
			at = l.Start.Add(time.Minute)
		}
		add(transcript.Event{Type: transcript.EventUser, Timestamp: at,
			ToolResults: []transcript.ToolResult{{ToolUseID: id, IsError: isErr, Text: result}}})
		for _, w := range l.Waypoints {
			if w.Kind == journey.WaypointTestFail {
				add(transcript.Event{Type: transcript.EventAssistant, Timestamp: w.At, Text: "FAILED " + w.Text + " — AssertionError: expected 200, got 401"})
			}
		}
	}
	for _, b := range tr.Branches {
		add(transcript.Event{Type: transcript.EventAssistant, Timestamp: b.Start, Text: "I'll send an agent to " + strings.ToLower(b.Label[:1]) + b.Label[1:] + " while I carry on.",
			ToolUses: []transcript.ToolUse{{ID: b.ToolUseID, Name: "Agent", Input: json.RawMessage(fmt.Sprintf(`{"description":%q,"run_in_background":true}`, b.Label))}}})
		if b.Done {
			add(transcript.Event{Type: transcript.EventUser, Timestamp: b.End,
				ToolResults: []transcript.ToolResult{{ToolUseID: b.ToolUseID, Text: b.Report}}})
		}
	}
	if strings.Contains(activity, "API Error:") {
		// The gateway refusing the call, as the CLI writes it: the last
		// thing in the conversation, flagged and synthetic.
		at := sceneNow.Add(-2 * time.Minute)
		if n := len(tr.Legs); n > 0 && tr.Legs[n-1].End.After(at) {
			at = tr.Legs[n-1].End.Add(time.Second)
		}
		add(transcript.Event{Type: transcript.EventAssistant, Timestamp: at, Text: activity, APIError: true, Status: 403, ErrorKey: "authentication_failed"})
	}
	if q := activity; q != "" && !strings.HasPrefix(q, "Bash: ") && strings.Contains(q, "?") {
		// The open question, at the very end, unanswered: what HEAD names
		// is what the reader ends on.
		at := sceneNow.Add(-4 * time.Minute)
		if n := len(tr.Legs); n > 0 {
			at = tr.Legs[n-1].Start.Add(time.Minute)
		}
		question, options := q, []string{}
		if i := strings.Index(q, " ["); i >= 0 {
			question = q[:i]
			options = strings.Split(strings.Trim(q[i+2:], "[]"), " / ")
		}
		var opts []string
		for _, o := range options {
			opts = append(opts, fmt.Sprintf(`{"label":%q}`, o))
		}
		add(transcript.Event{Type: transcript.EventAssistant, Timestamp: at, Text: "I need a decision before I change the rule.",
			ToolUses: []transcript.ToolUse{{ID: "toolu_ask", Name: "AskUserQuestion", Input: json.RawMessage(fmt.Sprintf(`{"questions":[{"question":%q,"options":[%s]}]}`, question, strings.Join(opts, ",")))}}})
	}
	if cmd := strings.TrimPrefix(activity, "Bash: "); cmd != activity && cmd != "" {
		// The hung call, at the very end — after the leg's own result, as
		// a transcript has it: what HEAD names is what the reader ends on.
		at := sceneNow.Add(-4 * time.Minute)
		if n := len(tr.Legs); n > 0 {
			at = tr.Legs[n-1].Start.Add(time.Minute)
			if end := tr.Legs[n-1].End; !end.IsZero() {
				at = end.Add(30 * time.Second)
			}
		}
		add(transcript.Event{Type: transcript.EventAssistant, Timestamp: at, Text: "Running the backfill over every shard.",
			ToolUses: []transcript.ToolUse{{ID: "toolu_hung", Name: "Bash", Input: json.RawMessage(fmt.Sprintf(`{"command":%q}`, cmd))}}})
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].Timestamp.Before(evs[j].Timestamp) })
	return evs
}

// runSummaryFor is pytest's last line for a leg's newest run.
func runSummaryFor(l journey.Leg) string {
	for i := len(l.Waypoints) - 1; i >= 0; i-- {
		if w := l.Waypoints[i]; w.Kind == journey.WaypointTestRun {
			return w.Text
		}
	}
	return "no tests ran"
}

func withPrompt(tr journey.Trail, at time.Time, text string) journey.Trail {
	tr.Prompts = append(tr.Prompts, journey.Prompt{Text: text, At: at})
	return tr
}

func withBranch(tr journey.Trail, afterLeg int, label string, start time.Time, done bool, report string) journey.Trail {
	b := journey.Branch{ToolUseID: fmt.Sprintf("a%d", len(tr.Branches)+1), Label: label, Start: start, AfterLeg: afterLeg, Done: done, Report: report}
	if done {
		b.End = start.Add(4 * time.Minute)
	}
	tr.Branches = append(tr.Branches, b)
	return tr
}

func withTasks(tr journey.Trail, tasks ...journey.Task) journey.Trail {
	tr.Tasks = append(tr.Tasks, tasks...)
	return tr
}

func sess(id, name, cwd, branch, title string, st state.State, since time.Time, class journey.Class, outcome, reason, activity string) fleet.Session {
	return fleet.Session{
		Info: fleet.SessionInfo{ID: id, TranscriptPath: sessionKey(id), ProjectSlug: "-home-user-" + name,
			CWD: cwd, GitBranch: branch, Title: title, StartedAt: since.Add(-2 * time.Hour), LastEventAt: since},
		Snap:  state.Snapshot{State: st, Since: since, Reason: reason, Activity: activity},
		Live:  true,
		Class: class, HasClass: true, Outcome: outcome,
	}
}

func gone(id, name, title string, at time.Time) fleet.Session {
	n := 0
	for _, r := range id {
		n += int(r)
	}
	branch := []string{"main", "fix/" + name + "-timeouts", "feat/" + name + "-v2", "chore/deps", "spike/" + name}[n%5]
	return fleet.Session{
		Info: fleet.SessionInfo{ID: id, TranscriptPath: sessionKey(id), ProjectSlug: "-home-user-" + name,
			CWD: "/home/user/" + name, GitBranch: branch, Title: title, StartedAt: at.Add(-time.Hour), LastEventAt: at},
		Snap: archivedSnap(at),
	}
}

// pastTrail is the journey behind an archived session: what it was asked,
// a few legs, and how it came out — the archive is browsable, not a list of
// names over "nothing yet".
func pastTrail(s fleet.Session) journey.Trail {
	n := 0
	for _, r := range s.Info.ID {
		n += int(r)
	}
	file := []string{"client.go", "sched.go", "auth.py", "release.md", "import.go"}[n%5]
	verdict := []string{"40✓", "12✓ 1✗", "212✓", "", "9✓"}[n%5]
	legs := []legSpec{
		{journey.Scout, "the " + strings.TrimSuffix(strings.TrimSuffix(file, ".go"), ".py") + " path", 6 * time.Minute, []string{file}, "", nil},
		{journey.Build, file, 20 * time.Minute, []string{file}, "", nil},
	}
	if verdict != "" {
		legs = append(legs, legSpec{journey.Test, "go test", 3 * time.Minute, nil, verdict, nil})
	}
	if n%3 == 0 {
		legs = append(legs, legSpec{journey.Ship, "commit", 2 * time.Minute, nil, "", nil})
	}
	return trailOf(s.Info.LastEventAt.Add(-time.Hour), s.Info.Title, false, legs...)
}

// pastTitles are what the archived sessions were asked for: a fleet of
// "archived job N" told the reviewers nothing about the archive.
var pastTitles = []string{
	"why does the nightly build take 40 minutes", "add retries to the s3 client", "rename Customer to Account everywhere",
	"upgrade to go 1.24", "write the incident postmortem", "flaky test in the scheduler", "cut the 2.3 release",
	"make the cli print json", "remove the legacy auth path", "profile the import", "document the webhook contract",
	"port the login screen", "clean the exploration notebooks", "migrate the docs to the new theme", "update the on-call runbook",
}

func pane(target, id string, pid int) tmuxop.Pane {
	return tmuxop.Pane{Target: target, ID: id, PID: pid, Command: "claude"}
}

// paneMap pairs sessions with panes in the order given; a "" target leaves
// that session paneless (the fleet's `elsewhere`).
func paneMap(ids []string, targets []string) (map[string]tmuxop.Pane, []tmuxop.Pane) {
	m := map[string]tmuxop.Pane{}
	var order []tmuxop.Pane
	for i, id := range ids {
		if targets[i] == "" {
			continue
		}
		p := pane(targets[i], fmt.Sprintf("%%%d", i+1), 1000+i)
		m[sessionKey(id)] = p
		order = append(order, p)
	}
	return m, order
}

// ---------------------------------------------------------------- scenes

// Twelve sessions, one moving. The afternoon after a morning of fanning out:
// most of the fleet finished hours or days ago, one is still going, and two
// finished recently and have not been read.
func sceneManyIdle() scene {
	n := sceneNow
	ids := []string{"etl", "api", "webapp", "billing", "mobile", "cli", "notebooks", "docs-site", "perf", "migrate", "ops-runbook", "infra"}
	tr := map[string]journey.Trail{}
	var ss []fleet.Session

	ss = append(ss, sess("etl", "etl", "/home/user/etl", "feat/dedupe", "dedupe the nightly load", state.Working, n.Add(-40*time.Second), journey.Build, "", "tool call in flight", "Edit: loader.py"))
	tr[sessionKey("etl")] = trailOf(n.Add(-3*time.Hour), "dedupe the nightly load without a full rescan", true,
		legSpec{journey.Scout, "loader.py and the dedupe notes", 12 * time.Minute, []string{"loader.py"}, "", nil},
		legSpec{journey.Design, "a bloom filter over row hashes", 9 * time.Minute, []string{"DESIGN.md"}, "", nil},
		legSpec{journey.Build, "bloom.py", 41 * time.Minute, []string{"bloom.py", "loader.py"}, "", nil},
		legSpec{journey.Test, "pytest", 6 * time.Minute, nil, "212✓ 3✗", []string{"test_dedupe_overlap"}},
		legSpec{journey.Fix, "overlap at the shard boundary", 25 * time.Minute, []string{"bloom.py"}, "", nil},
		legSpec{journey.Test, "pytest", 5 * time.Minute, nil, "215✓", nil},
		legSpec{journey.Build, "loader.py", 20 * time.Minute, []string{"loader.py"}, "", nil},
	)
	tr[sessionKey("etl")] = withTasks(tr[sessionKey("etl")],
		journey.Task{ID: "1", Subject: "Add the bloom filter", Status: "completed"},
		journey.Task{ID: "2", Subject: "Wire it into the loader", Active: "Wiring the filter into the loader", Status: "in_progress"},
		journey.Task{ID: "3", Subject: "Backfill last week's shards", Status: "pending"},
		journey.Task{ID: "4", Subject: "Write the runbook entry", Status: "pending"})

	ss = append(ss, sess("api", "api", "/home/user/api", "claude/auth-fx", "fix the 401 bug", state.Idle, n.Add(-9*time.Minute), journey.Test, "1216✓", "turn complete", "idle"))
	tr[sessionKey("api")] = trailOf(n.Add(-2*time.Hour), "fix the 401 on token refresh", false,
		legSpec{journey.Scout, "middleware.py", 8 * time.Minute, []string{"middleware.py"}, "", nil},
		legSpec{journey.Build, "tokens.py", 22 * time.Minute, []string{"tokens.py"}, "", nil},
		legSpec{journey.Test, "pytest", 7 * time.Minute, nil, "1214✓ 2✗", []string{"test_refresh_expired", "test_refresh_revoked"}},
		legSpec{journey.Fix, "expiry compared in local time", 14 * time.Minute, []string{"tokens.py"}, "", nil},
		legSpec{journey.Test, "pytest", 7 * time.Minute, nil, "1216✓", nil},
		legSpec{journey.Ship, "commit", 2 * time.Minute, nil, "", nil},
	)

	ss = append(ss, sess("webapp", "webapp", "/home/user/webapp", "main", "flake in the checkout suite", state.Idle, n.Add(-52*time.Minute), journey.Fix, "18✓ 2✗", "turn complete", "idle"))
	tr[sessionKey("webapp")] = trailOf(n.Add(-90*time.Minute), "the checkout suite flakes on CI but not locally", false,
		legSpec{journey.Scout, "the CI logs", 6 * time.Minute, []string{"ci.log"}, "", nil},
		legSpec{journey.Test, "pytest", 4 * time.Minute, nil, "18✓ 2✗", []string{"test_checkout_total"}},
		legSpec{journey.Fix, "a timezone in the fixture", 11 * time.Minute, []string{"conftest.py", "tokens.py"}, "", nil},
		legSpec{journey.Test, "pytest", 4 * time.Minute, nil, "18✓ 2✗", []string{"test_checkout_total"}},
	)

	idle := []struct {
		id, title string
		age       time.Duration
		class     journey.Class
	}{
		{"billing", "reconcile the invoice totals", 5 * time.Hour, journey.Build},
		{"mobile", "port the login screen", 26 * time.Hour, journey.Build},
		{"cli", "add --json to every command", 2 * 24 * time.Hour, journey.Docs},
		{"notebooks", "clean the exploration notebooks", 3 * 24 * time.Hour, journey.Scout},
		{"docs-site", "migrate the docs to the new theme", 4 * 24 * time.Hour, journey.Docs},
		{"perf", "profile the hot loop", 5 * 24 * time.Hour, journey.Scout},
		{"migrate", "write the migration for the users table", 6 * 24 * time.Hour, journey.Build},
		{"ops-runbook", "update the on-call runbook", 8 * 24 * time.Hour, journey.Docs},
		{"infra", "tighten the vpc security groups", 10 * 24 * time.Hour, journey.Design},
	}
	for i, s := range idle {
		ss = append(ss, sess(s.id, s.id, "/home/user/"+s.id, "main", s.title, state.Idle, n.Add(-s.age), s.class, "", "turn complete", "idle"))
		scout := []string{"the invoice model", "LoginScreen.tsx", "cmd/root.go", "the notebooks dir", "the theme config", "the hot loop", "the users schema", "the runbook", "the security groups"}[i]
		made := []string{"invoice.py", "LoginScreen.tsx", "cmd/json.go", "nb/cleanup.py", "theme.toml", "loop.go", "007_users.sql", "runbook.md", "sg.tf"}[i]
		verdict := []string{"212✓", "18✓", "40✓", "", "9✓", "3✓ 1✗", "12✓", "", "40✓"}[i]
		legs := []legSpec{
			{journey.Scout, scout, 10 * time.Minute, []string{scout}, "", nil},
			{s.class, made, time.Duration(12+7*i) * time.Minute, []string{made}, "", nil},
		}
		if verdict != "" {
			legs = append(legs, legSpec{journey.Test, "go test", 3 * time.Minute, nil, verdict, nil})
		}
		if i%3 == 0 {
			legs = append(legs, legSpec{journey.Ship, "commit", 2 * time.Minute, nil, "", nil})
		}
		tr[sessionKey(s.id)] = trailOf(n.Add(-s.age-time.Hour), s.title, false, legs...)
	}
	for i := 0; i < 300; i++ {
		g := gone(fmt.Sprintf("a-%03d", i), []string{"api", "webapp", "etl", "infra"}[i%4], pastTitles[i%len(pastTitles)], n.Add(-time.Duration(i+1)*3*time.Hour))
		ss = append(ss, g)
		tr[g.Info.Key()] = pastTrail(g)
	}
	panes, order := paneMap(ids, []string{"work:0.0", "work:1.0", "work:2.0", "work:3.0", "side:0.0", "side:1.0", "", "side:2.0", "ops:0.0", "ops:1.0", "", "ops:2.0"})
	return scene{name: "many-idle", story: "Twelve sessions from a morning of fanning out; one still moving, two finished recently and unread, the rest done hours or days ago.", sessions: ss, trails: tr, panes: panes, order: order}
}

// Seven sessions, four of them alive: one asking a question, two working —
// one with a red suite — and one that has gone quiet mid-turn.
func sceneFewOngoing() scene {
	n := sceneNow
	tr := map[string]journey.Trail{}
	var ss []fleet.Session

	ss = append(ss, sess("infra", "infra", "/home/user/infra", "tf/vpc", "tighten the vpc security groups", state.NeedsYou, n.Add(-4*time.Minute), journey.Design, "", "waiting on your answer", "Open port 22 to the office CIDR only, or keep the bastion? [office CIDR / keep bastion]"))
	tr[sessionKey("infra")] = trailOf(n.Add(-35*time.Minute), "tighten the vpc security groups without breaking the bastion", true,
		legSpec{journey.Scout, "main.tf and the bastion rules", 14 * time.Minute, []string{"main.tf", "bastion.tf"}, "", nil},
		legSpec{journey.Design, "AskUserQuestion", 4 * time.Minute, nil, "", nil},
	)
	ss = append(ss, sess("api", "api", "/home/user/api", "claude/auth-fx", "fix the 401 bug", state.Working, n.Add(-3*time.Minute), journey.Fix, "1214✓ 2✗", "tool call in flight", "Edit: tokens.py"))
	defer func() {
		tr[sessionKey("api")] = withTasks(tr[sessionKey("api")],
			journey.Task{ID: "1", Subject: "Find why refresh returns 401", Status: "completed"},
			journey.Task{ID: "2", Subject: "Fix the expiry comparison", Active: "Fixing the expiry comparison", Status: "in_progress"},
			journey.Task{ID: "3", Subject: "Re-run the auth suite", Status: "pending"},
			journey.Task{ID: "4", Subject: "Commit and open the PR", Status: "pending"})
	}()
	tr[sessionKey("api")] = trailOf(n.Add(-50*time.Minute), "fix the 401 on token refresh", true,
		legSpec{journey.Scout, "middleware.py", 8 * time.Minute, []string{"middleware.py"}, "", nil},
		legSpec{journey.Build, "tokens.py", 22 * time.Minute, []string{"tokens.py"}, "", nil},
		legSpec{journey.Test, "pytest", 7 * time.Minute, nil, "1214✓ 2✗", []string{"test_refresh_expired", "test_refresh_revoked"}},
		legSpec{journey.Fix, "expiry compared in local time", 3 * time.Minute, []string{"tokens.py"}, "", nil},
	)
	ss = append(ss, sess("webapp", "webapp", "/home/user/webapp", "main", "flake in the checkout suite", state.Working, n.Add(-70*time.Second), journey.Test, "18✓ 2✗", "tool call in flight", "Bash: pytest tests/checkout -x"))
	tr[sessionKey("webapp")] = trailOf(n.Add(-25*time.Minute), "the checkout suite flakes on CI but not locally", true,
		legSpec{journey.Scout, "the CI logs", 6 * time.Minute, []string{"ci.log"}, "", nil},
		legSpec{journey.Test, "pytest", 4 * time.Minute, nil, "18✓ 2✗", []string{"test_checkout_total"}},
		legSpec{journey.Fix, "a timezone in the fixture", 8 * time.Minute, []string{"conftest.py", "tokens.py"}, "", nil},
		legSpec{journey.Test, "pytest", 70 * time.Second, nil, "", nil},
	)
	etl := sess("etl", "etl", "/home/user/etl", "feat/dedupe", "dedupe the nightly load", state.Stuck, n.Add(-6*time.Minute), journey.Build, "", "no output for 4m mid-turn", "Bash: python backfill.py --all")
	etl.Info.LastEventAt = n.Add(-4 * time.Minute) // the silence the reason counts
	ss = append(ss, etl)
	tr[sessionKey("etl")] = trailOf(n.Add(-40*time.Minute), "backfill last week's shards", true,
		legSpec{journey.Scout, "backfill.py", 5 * time.Minute, []string{"backfill.py"}, "", nil},
		legSpec{journey.Build, "backfill.py", 20 * time.Minute, []string{"backfill.py"}, "", nil},
		legSpec{journey.Build, "backfill.py", 6 * time.Minute, []string{"backfill.py"}, "", nil},
	)
	for _, s := range []struct {
		id, title string
		age       time.Duration
	}{{"billing", "reconcile the invoice totals", 3 * time.Hour}, {"docs-site", "migrate the docs to the new theme", 26 * time.Hour}, {"cli", "add --json to every command", 3 * 24 * time.Hour}} {
		ss = append(ss, sess(s.id, s.id, "/home/user/"+s.id, "main", s.title, state.Idle, n.Add(-s.age), journey.Build, "40✓", "turn complete", "idle"))
		tr[sessionKey(s.id)] = trailOf(n.Add(-s.age-time.Hour), s.title, false,
			legSpec{journey.Scout, "the relevant files", 10 * time.Minute, []string{"a.go"}, "", nil},
			legSpec{journey.Build, "the change", 30 * time.Minute, []string{"b.go"}, "", nil},
			legSpec{journey.Test, "go test", 3 * time.Minute, nil, "40✓", nil},
		)
	}
	for i := 0; i < 40; i++ {
		g := gone(fmt.Sprintf("a-%03d", i), "api", pastTitles[i%len(pastTitles)], n.Add(-time.Duration(i+1)*5*time.Hour))
		ss = append(ss, g)
		tr[g.Info.Key()] = pastTrail(g)
	}
	panes, order := paneMap([]string{"infra", "api", "webapp", "etl", "billing", "docs-site", "cli"}, []string{"ops:0.0", "work:0.0", "work:1.0", "work:2.0", "side:0.0", "", "side:1.0"})
	return scene{name: "few-ongoing", extra: []string{"5", "x", "A", "A"}, story: "Four sessions alive at once: one asking a question, two working (one with a red suite), one gone quiet mid-turn; three finished earlier.", sessions: ss, trails: tr, panes: panes, order: order}
}

// Three sessions that delegate: one with three background agents still out,
// one whose agents came back with findings, one that is itself a teammate's
// plan being worked through.
func sceneSubagents() scene {
	n := sceneNow
	tr := map[string]journey.Trail{}
	var ss []fleet.Session

	ss = append(ss, sess("porter", "porter", "/home/user/porter", "main", "/kickoff porter_tui — the delegated port", state.Working, n.Add(-2*time.Minute), journey.Build, "20✓", "tool call in flight", "Agent: score encoder gates"))
	t := trailOf(n.Add(-3*time.Hour), "/kickoff porter_tui — the delegated port is running in the harness", true,
		legSpec{journey.Scout, "reviewed two prs and gates", 12 * time.Minute, []string{"gates.md"}, "", nil},
		legSpec{journey.Build, "s6e_encoder.py", 30 * time.Minute, []string{"s6e_encoder.py"}, "", nil},
		legSpec{journey.Test, "pytest", 5 * time.Minute, nil, "20✓", nil},
		legSpec{journey.Build, "implement s6e encoder tests", 25 * time.Minute, []string{"test_s6e.py"}, "", nil},
	)
	// The second prompt and the compaction fall between the test and the
	// last build — the build the lanes fork from — so the column reads in
	// time order.
	t.Prompts = append(t.Prompts, journey.Prompt{Text: "the encoder is in; now the tests, then measure DLA", At: n.Add(-133 * time.Minute)})
	t.Compactions = []time.Time{n.Add(-134 * time.Minute)}
	t = withBranch(t, 0, "Score encoder gates vs oracle defects", n.Add(-165*time.Minute), true, "3 defects found against the oracle; two are the same root cause")
	t = withBranch(t, 3, "Measure moe_by_andy DLA on dx6", n.Add(-20*time.Minute), false, "")
	t = withBranch(t, 3, "Review /auto-resume skill design", n.Add(-18*time.Minute), false, "")
	t = withBranch(t, 3, "Red-team the plugin architecture", n.Add(-15*time.Minute), false, "")
	t.Legs[len(t.Legs)-1].End = n.Add(-15 * time.Minute) // its last own word: the dispatch, 15m ago
	tr[sessionKey("porter")] = withTasks(t,
		journey.Task{ID: "1", Subject: "Score encoder gates", Status: "completed"},
		journey.Task{ID: "2", Subject: "Measure DLA on dx6", Active: "Measuring DLA on dx6", Status: "in_progress", Owner: "measurer"},
		journey.Task{ID: "3", Subject: "Fold findings into DESIGN.md", Status: "pending"})

	ss = append(ss, sess("harness", "harness", "/home/user/harness", "main", "look at our kickoff skill", state.Idle, n.Add(-30*time.Minute), journey.Docs, "", "turn complete", "idle"))
	t = trailOf(n.Add(-2*time.Hour), "look at our kickoff skill and the delegation contract", false,
		legSpec{journey.Scout, "the skill and three transcripts", 15 * time.Minute, []string{"SKILL.md"}, "", nil},
		legSpec{journey.Design, "a delegation contract", 20 * time.Minute, []string{"CONTRACT.md"}, "", nil},
		legSpec{journey.Docs, "SKILL.md", 18 * time.Minute, []string{"SKILL.md"}, "", nil},
	)
	t = withBranch(t, 0, "Read every kickoff transcript from last week", n.Add(-110*time.Minute), true, "Seven of nine kickoffs re-ran the same setup; the skill should cache it")
	t = withBranch(t, 0, "Compare the contract against the SDK docs", n.Add(-108*time.Minute), true, "The SDK renamed teammate to agent in 2.1; the contract still says teammate")
	t = withBranch(t, 1, "Draft the contract's failure section", n.Add(-80*time.Minute), true, "")
	tr[sessionKey("harness")] = t

	ss = append(ss, sess("redteam", "redteam", "/home/user/harness", "main", "Red-teaming plugin/extensibility architecture", state.Working, n.Add(-30*time.Second), journey.Scout, "", "tool call in flight", "Read: plugins/loader.go"))
	tr[sessionKey("redteam")] = withTasks(trailOf(n.Add(-14*time.Minute), "Red-team the plugin architecture; report the top three risks", true,
		legSpec{journey.Scout, "plugins/loader.go", 9 * time.Minute, []string{"loader.go"}, "", nil},
		legSpec{journey.Scout, "the manifest schema", 4 * time.Minute, []string{"schema.json"}, "", nil},
	), journey.Task{ID: "2", Subject: "Red-team plugin architecture", Active: "Red-teaming plugin/extensibility architecture", Status: "in_progress", Owner: "plugin-arch-redteam"})

	ss = append(ss, sess("cli", "cli", "/home/user/cli", "main", "add --json to every command", state.Idle, n.Add(-2*24*time.Hour), journey.Build, "40✓", "turn complete", "idle"))
	tr[sessionKey("cli")] = trailOf(n.Add(-49*time.Hour), "add --json to every command", false,
		legSpec{journey.Build, "the flag", 30 * time.Minute, []string{"main.go"}, "", nil},
		legSpec{journey.Test, "go test", 3 * time.Minute, nil, "40✓", nil},
	)
	panes, order := paneMap([]string{"porter", "harness", "redteam", "cli"}, []string{"tinker:0.0", "harness:0.0", "harness:1.0", "tools:0.0"})
	return scene{name: "subagents", story: "Sessions that delegate: one with three background agents still out and one back with a finding; one whose agents all reported; one that is itself a teammate working a shared task list.", sessions: ss, trails: tr, panes: panes, order: order}
}

// Two very long sessions — a day of work each, every class, dozens of prompts
// — beside a short one, so the trail's viewport, the ticks and the density
// of Lv2 are what is on trial.
func sceneVeryLong() scene {
	n := sceneNow
	tr := map[string]journey.Trail{}
	var ss []fleet.Session

	long := func(id string, legs int, start time.Time, current bool) journey.Trail {
		project := id // the commits are the session's own, not another's
		classes := []journey.Class{journey.Scout, journey.Build, journey.Test, journey.Fix, journey.Test, journey.Build, journey.Docs, journey.Ship, journey.Scout, journey.Design}
		labels := map[journey.Class][]string{
			journey.Scout:  {"the router", "the session store", "how tokens are minted", "the audit log", "the old migration"},
			journey.Build:  {"router.py", "store.py", "tokens.py", "audit.py", "migrate_007.py"},
			journey.Test:   {"pytest", "pytest", "pytest tests/auth", "pytest -x"},
			journey.Fix:    {"a nil session on logout", "expiry in local time", "the audit row order", "a flaky ordering"},
			journey.Docs:   {"CHANGELOG.md", "README.md", "docs/auth.md"},
			journey.Ship:   {"commit", "push", "open the pr"},
			journey.Design: {"the refresh flow", "where the audit log lives"},
		}
		tr := journey.Trail{Prompts: []journey.Prompt{{Text: "rebuild session handling end to end: router, store, tokens, audit", At: start}}}
		at := start.Add(time.Minute)
		fails := 0
		for i := 0; i < legs; i++ {
			c := classes[i%len(classes)]
			names := labels[c]
			leg := journey.Leg{Class: c, Label: names[i%len(names)], Start: at, End: at.Add(time.Duration(3+i%9) * time.Minute), Votes: 4}
			switch c {
			case journey.Test:
				switch i % 3 {
				case 0:
					fails++ // the same test, red again: Runs is what the segmenter would count
					leg.Waypoints = []journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "310 passed · 2 failed", Short: "310✓ 2✗", At: leg.End}, {Kind: journey.WaypointTestFail, Text: "test_logout_nil_session", Runs: fails, At: leg.End}}
				case 1:
					leg.Waypoints = []journey.Waypoint{{Kind: journey.WaypointTestRun, Text: "312 passed", Short: "312✓", At: leg.End}}
				default:
					// a run whose output never parsed: a tick on the rail
				}
			case journey.Ship:
				subject := map[string]string{
					"commit":      project + ": " + []string{"route sessions through the store", "mint tokens from the store", "audit every refresh", "migrate the old sessions", "drop the legacy path"}[(i/10)%5],
					"push":        project + ": push " + []string{"feat/sessions", "feat/audit", "feat/migrate"}[(i/10)%3],
					"open the pr": project + ": open the pr for review",
				}[leg.Label]
				leg.Waypoints = []journey.Waypoint{{Kind: journey.WaypointCommit, Text: subject, At: leg.End}}
			case journey.Build:
				leg.Files = []string{leg.Label}
			case journey.Fix:
				leg.Files = []string{[]string{"router.py", "tokens.py", "audit.py", "tests/test_order.py"}[i%4]}
			case journey.Docs:
				leg.Files = []string{leg.Label}
			}
			if current && i == legs-1 {
				leg.Current = true
			}
			tr.Legs = append(tr.Legs, leg)
			at = leg.End.Add(30 * time.Second)
			if i%14 == 13 {
				// A prompt comes after the session sat waiting for it — six
				// to twelve minutes here, as a person at a fleet does.
				wait := time.Duration(6+(i/14)%3*3) * time.Minute
				tr.Prompts = append(tr.Prompts, journey.Prompt{Text: []string{"ok keep going", "now the audit log", "fix all the failures first", "yes and update the docs", "please continue — our quota is back"}[(i/14)%5], At: leg.End.Add(wait)})
				at = leg.End.Add(wait + 30*time.Second)
			}
		}
		return tr
	}
	ss = append(ss, sess("auth", "auth", "/home/user/auth", "feat/sessions", "rebuild session handling end to end", state.Working, n.Add(-90*time.Second), journey.Build, "312✓", "tool call in flight", "Edit: audit.py"))
	tr[sessionKey("auth")] = withCompactions(withTasks(long("auth", 160, n.Add(-22*time.Hour), true),
		journey.Task{ID: "1", Subject: "Router", Status: "completed"}, journey.Task{ID: "2", Subject: "Store", Status: "completed"},
		journey.Task{ID: "3", Subject: "Tokens", Status: "completed"}, journey.Task{ID: "4", Subject: "Audit log", Active: "Wiring the audit log", Status: "in_progress"},
		journey.Task{ID: "5", Subject: "Migrate existing sessions", Status: "pending"}, journey.Task{ID: "6", Subject: "Docs and changelog", Status: "pending"},
		journey.Task{ID: "7", Subject: "Open the PR", Status: "pending"}), n.Add(-14*time.Hour), n.Add(-2*time.Hour))
	ss = append(ss, sess("etl", "etl", "/home/user/etl", "feat/dedupe", "dedupe the nightly load", state.Idle, n.Add(-3*time.Hour), journey.Ship, "", "turn complete", "idle"))
	tr[sessionKey("etl")] = long("etl", 120, n.Add(-20*time.Hour), false)
	ss = append(ss, sess("cli", "cli", "/home/user/cli", "main", "add --json to every command", state.Idle, n.Add(-40*time.Minute), journey.Test, "40✓", "turn complete", "idle"))
	tr[sessionKey("cli")] = trailOf(n.Add(-2*time.Hour), "add --json to every command", false,
		legSpec{journey.Build, "the flag", 30 * time.Minute, []string{"main.go"}, "", nil},
		legSpec{journey.Test, "go test", 3 * time.Minute, nil, "40✓", nil},
	)
	panes, order := paneMap([]string{"auth", "etl", "cli"}, []string{"work:0.0", "work:1.0", "tools:0.0"})
	return scene{name: "very-long", story: "Two sessions a day long each — 160 and 120 legs, every class, prompts every dozen legs, a plan with pending steps — beside a short one.", sessions: ss, trails: tr, panes: panes, order: order}
}

func allScenes() []scene {
	return []scene{sceneManyIdle(), sceneFewOngoing(), sceneSubagents(), sceneVeryLong(), sceneFirstSession(), sceneAlarmStorm(), sceneFleetHygiene()}
}

// quota is the refusal a session dead on its daily limit carries, in the
// words the CLI prints (the words of this fixture are its own).
const quota = "Please run /login · API Error: 403 your daily quota is exhausted, contact your administrator to increase it"

// The first five minutes: one session, just prompted, nothing done yet.
// Every empty state, every refused key, on the smallest terminal too.
func sceneFirstSession() scene {
	n := sceneNow
	tr := map[string]journey.Trail{}
	var ss []fleet.Session
	ss = append(ss, sess("hello", "hello", "/home/user/hello", "main", "add a --version flag", state.Working, n.Add(-40*time.Second), journey.Scout, "", "starting turn", "thinking…"))
	ss[0].Info.StartedAt = n.Add(-50 * time.Second)
	tr[sessionKey("hello")] = journey.Trail{Prompts: []journey.Prompt{{Text: "add a --version flag", At: n.Add(-50 * time.Second)}}}
	panes, order := paneMap([]string{"hello"}, []string{"main:0.0"})
	return scene{name: "first-session", story: "Someone opened compass for the first time, fifty seconds after typing their first prompt into their first session. No history, no archive, one column.", sessions: ss, trails: tr, panes: panes, order: order}
}

// The alarm storm: three sessions dead on quota at once, one asking a
// question, one hung, one going round the same failure, and one fine.
// The question is which to touch first.
func sceneAlarmStorm() scene {
	n := sceneNow
	tr := map[string]journey.Trail{}
	var ss []fleet.Session
	dead := func(id, prompt string, since time.Duration, legs ...legSpec) {
		s := sess(id, id, "/home/user/"+id, "main", prompt, state.NeedsYou, n.Add(-since), journey.Build, "", "api error 403 · authentication_failed", quota)
		s.Snap.APIError = true
		ss = append(ss, s)
		tr[sessionKey(id)] = trailOf(n.Add(-since-40*time.Minute), prompt, false, legs...)
	}
	dead("billing", "reconcile the invoice totals", 18*time.Minute,
		legSpec{journey.Scout, "the invoice model", 6 * time.Minute, []string{"invoice.py"}, "", nil},
		legSpec{journey.Build, "invoice.py", 20 * time.Minute, []string{"invoice.py"}, "", nil})
	dead("mobile", "fix the login crash on cold start", 9*time.Minute,
		legSpec{journey.Scout, "the crash log", 4 * time.Minute, []string{"crash.log"}, "", nil},
		legSpec{journey.Fix, "a nil session on cold start", 15 * time.Minute, []string{"login.swift"}, "", nil},
		legSpec{journey.Test, "xcodebuild test", 8 * time.Minute, nil, "40✓ 1✗", []string{"testColdStart"}})
	dead("docs-site", "regenerate the api reference", 2*time.Minute,
		legSpec{journey.Build, "the generator", 30 * time.Minute, []string{"gen.py"}, "", nil})
	ss = append(ss, sess("infra", "infra", "/home/user/infra", "tf/vpc", "tighten the vpc security groups", state.NeedsYou, n.Add(-7*time.Minute), journey.Design, "", "question", "Open port 22 to the office CIDR only, or keep the bastion? [office CIDR / keep bastion]"))
	tr[sessionKey("infra")] = trailOf(n.Add(-30*time.Minute), "tighten the vpc security groups without breaking the bastion", true,
		legSpec{journey.Scout, "main.tf and the bastion rules", 14 * time.Minute, []string{"main.tf"}, "", nil},
		legSpec{journey.Design, "Open port 22 to the office CIDR only, or keep the bastion?", 7 * time.Minute, nil, "", nil})
	ss = append(ss, sess("etl", "etl", "/home/user/etl", "feat/backfill", "backfill last week's shards", state.Stuck, n.Add(-6*time.Minute), journey.Build, "", "no output for 6m mid-turn", "Bash: python backfill.py --all"))
	ss[len(ss)-1].Info.LastEventAt = n.Add(-6 * time.Minute)
	tr[sessionKey("etl")] = trailOf(n.Add(-50*time.Minute), "backfill last week's shards", true,
		legSpec{journey.Scout, "the shard layout", 8 * time.Minute, []string{"shards.py"}, "", nil},
		legSpec{journey.Build, "backfill.py", 30 * time.Minute, []string{"backfill.py"}, "", nil})
	ss = append(ss, sess("api", "api", "/home/user/api", "claude/auth-fx", "fix the 401 on token refresh", state.Working, n.Add(-30*time.Second), journey.Fix, "18✓ 2✗", "tool call in flight", "Edit: tokens.py"))
	loop := []legSpec{legSpec{journey.Scout, "middleware.py", 5 * time.Minute, []string{"middleware.py"}, "", nil}}
	for i := 0; i < 4; i++ {
		loop = append(loop,
			legSpec{journey.Fix, []string{"the expiry check", "the clock skew", "the refresh window", "the revoked path"}[i], 6 * time.Minute, []string{"tokens.py"}, "", nil},
			legSpec{journey.Test, "pytest", 3 * time.Minute, nil, "18✓ 2✗", []string{"test_refresh_expired"}})
	}
	loop = append(loop, legSpec{journey.Fix, "tokens.py", 2 * time.Minute, []string{"tokens.py"}, "", nil})
	tr[sessionKey("api")] = trailOf(n.Add(-80*time.Minute), "fix the 401 on token refresh", true, loop...)
	ss = append(ss, sess("cli", "cli", "/home/user/cli", "main", "add --json to every command", state.Working, n.Add(-20*time.Second), journey.Build, "", "tool call in flight", "Edit: main.go"))
	tr[sessionKey("cli")] = trailOf(n.Add(-25*time.Minute), "add --json to every command", true,
		legSpec{journey.Scout, "the command table", 5 * time.Minute, []string{"main.go"}, "", nil},
		legSpec{journey.Build, "the flag", 15 * time.Minute, []string{"main.go"}, "", nil})
	panes, order := paneMap([]string{"billing", "mobile", "docs-site", "infra", "etl", "api", "cli"}, []string{"work:0.0", "work:1.0", "work:2.0", "ops:0.0", "work:3.0", "work:4.0", "tools:0.0"})
	return scene{name: "alarm-storm", extra: []string{"4", "r", "1"}, story: "Seven sessions and everything went wrong at once: three died on the daily quota, one is asking a question, one has hung, one is going round the same failing test for the fourth time, one is fine. Which first?", sessions: ss, trails: tr, panes: panes, order: order}
}

// Fleet hygiene: two live sessions with one name in one tmux session, a
// session outside tmux inside the live window, a session whose pane
// closed and is over the archive cliff, a relayed message as a prompt,
// and forty archived sessions with the same four names.
func sceneFleetHygiene() scene {
	n := sceneNow
	tr := map[string]journey.Trail{}
	var ss []fleet.Session
	ss = append(ss, sess("harness-a", "harness", "/home/user/harness", "main", "add watch driver tests", state.Working, n.Add(-2*time.Minute), journey.Test, "", "tool call in flight", "Bash: pytest -q"))
	tr[sessionKey("harness-a")] = trailOf(n.Add(-40*time.Minute), "add watch driver tests", true,
		legSpec{journey.Build, "watch_driver.py", 20 * time.Minute, []string{"watch_driver.py"}, "", nil},
		legSpec{journey.Test, "pytest", 3 * time.Minute, nil, "", nil})
	ss = append(ss, sess("harness-b", "harness", "/home/user/harness", "spike/resume", "/resume", state.Idle, n.Add(-2*time.Hour), journey.Scout, "", "turn complete", "idle"))
	tr[sessionKey("harness-b")] = trailOf(n.Add(-2*time.Hour-10*time.Minute), "/resume", false,
		legSpec{journey.Scout, "the resume skill", 8 * time.Minute, []string{"SKILL.md"}, "", nil})
	ss = append(ss, sess("relay", "porter", "/home/user/porter", "main", "Another Claude session sent a message: the encoder is in, run the gates", state.Working, n.Add(-90*time.Second), journey.Test, "", "tool call in flight", "Bash: pytest tests/gates -q"))
	tr[sessionKey("relay")] = trailOf(n.Add(-12*time.Minute), "Another Claude session sent a message: the encoder is in, run the gates", true,
		legSpec{journey.Scout, "the gates", 4 * time.Minute, []string{"gates.md"}, "", nil},
		legSpec{journey.Test, "pytest tests/gates", 5 * time.Minute, nil, "", nil})
	ss = append(ss, sess("nopane", "notebooks", "/home/user/notebooks", "main", "clean the eda notebook", state.Idle, n.Add(-3*time.Minute), journey.Docs, "", "turn complete", "idle"))
	tr[sessionKey("nopane")] = trailOf(n.Add(-25*time.Minute), "clean the eda notebook", false,
		legSpec{journey.Docs, "eda.ipynb", 18 * time.Minute, []string{"eda.ipynb"}, "", nil})
	ss = append(ss, gone("closed", "api", "the pane I closed half an hour ago", n.Add(-32*time.Minute)))
	tr[sessionKey("closed")] = trailOf(n.Add(-70*time.Minute), "fix the 401 on token refresh", false,
		legSpec{journey.Fix, "tokens.py", 20 * time.Minute, []string{"tokens.py"}, "", nil},
		legSpec{journey.Test, "pytest", 5 * time.Minute, nil, "1216✓", nil})
	titles := []string{"fix the 401 on token refresh", "why does the nightly build take 40 minutes", "port the client to the new sdk", "add --json to every command", "reconcile the state file"}
	for i := 0; i < 40; i++ {
		g := gone(fmt.Sprintf("h-%03d", i), []string{"api", "harness", "porter", "notebooks"}[i%4], titles[i%len(titles)], n.Add(-time.Duration(i+2)*5*time.Hour))
		ss = append(ss, g)
		tr[g.Info.Key()] = pastTrail(g)
	}
	panes, order := paneMap([]string{"harness-a", "harness-b", "relay", "nopane"}, []string{"harness:1.0", "harness:0.0", "tinker:0.0", ""})
	return scene{name: "fleet-hygiene", extra: []string{"4", "r", "esc"}, story: "Two live sessions called harness in tmux session harness, a lead messaging another session, a session with no pane, a session whose pane closed half an hour ago, and forty archived sessions wearing the same four names.", sessions: ss, trails: tr, panes: panes, order: order}
}

// ---------------------------------------------------------------- driver

// sceneModel opens the deck on a scene as the first refresh would leave it.
func sceneModel(sc scene, w, h int) *Model {
	fleet.SortFleet(sc.sessions) // the order Refresh would hand over
	m := New(nil)
	m.runner = &recordingTmux{} // a send in the walkthrough presses no real key
	m.SetSize(w, h)
	m.SetSessions(sc.sessions, sceneNow)
	m.SetPanes(sc.panes)
	m.SetPaneOrder(sc.order)
	m.loaded = false // the refresh below is the first, as a launch has it
	// The second live session was looked at partway through its trail —
	// halfway, or eight legs from the end of a long one, so the read-line
	// falls where a pinned column can show it: its column carries the
	// read-line and the digest.
	m.seen = map[string]time.Time{}
	live := 0
	for _, s := range sc.sessions {
		tr := sc.trails[s.Info.Key()]
		if !s.Live || len(tr.Legs) <= 2 {
			continue
		}
		if live++; live == 2 || live == 3 {
			at := len(tr.Legs) / 2
			if len(tr.Legs) > 24 {
				at = len(tr.Legs) - 8
			}
			m.seen[s.Info.Key()] = tr.Legs[at].Start.Add(time.Second)
		}
	}
	first := ""
	if order := m.viewOrder(); len(order) > 0 {
		first = sc.sessions[order[0]].Info.Key() // the board's first column, as a launch selects
	}
	m.point(first)
	// Two polls, as a launch has them: the first is the baseline and the
	// board; the second installs the selected session's trail and events.
	for i := 0; i < 2; i++ {
		m.Update(fleetMsg{sessions: sc.sessions, at: sceneNow, trailFor: first, hasTrail: true,
			trail: sc.trails[first], events: eventsBehind(sc.trails[first], sc.activity(first)), trails: sc.trails})
	}
	return m
}

// activity is the selected session's call in flight, for the reader.
func (sc scene) activity(key string) string {
	for _, s := range sc.sessions {
		if s.Info.Key() == key {
			return s.Snap.Activity
		}
	}
	return ""
}

// pressKey sends one key by name — "tab", "shift+tab", "esc", "enter",
// "ctrl+d", "ctrl+u", or a rune.
func pressKey(m *Model, key string) {
	switch key {
	case "tab":
		m.Update(tea.KeyMsg{Type: tea.KeyTab})
	case "shift+tab":
		m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	case "esc":
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	case "enter":
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	case "ctrl+d":
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	case "ctrl+u":
		m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	case "space":
		m.Update(tea.KeyMsg{Type: tea.KeySpace})
	default:
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if cmd != nil {
			// A reply's send runs off the loop and reports back: play the
			// report in, so the trace it leaves is on the next frame.
			if msg := cmd(); msg != nil {
				if _, ok := msg.(replyDoneMsg); ok {
					m.Update(msg)
				}
			}
		}
	}
}

// canonicalKeys is the walkthrough every scenario gets: the board, a move,
// down into one trail and its legs and the reader, back out, a jump by
// number, the archive and back, the mirror toggle and the help.
var canonicalKeys = []string{"r", "1", "/", "pytest", "enter", "esc", "tab", "ctrl+u", "ctrl+u", "[", "]", "G", "tab", "[", "]", "k", "k", "tab", "j", "j", "shift+tab", "shift+tab", "shift+tab", "j", "r", "t", "go on", "enter", "tab", "m", "m", "tab", "[", "]", "shift+tab", "shift+tab", "x", "A", "A", "m", "?"}

func walkthrough(sc scene, w, h int, keys []string) string {
	var b strings.Builder
	if len(sc.extra) > 0 {
		// The canonical walkthrough ends on the help: close it first.
		keys = append(append(append([]string(nil), keys...), "esc"), sc.extra...)
	}
	m := sceneModel(sc, w, h)
	fmt.Fprintf(&b, "SCENARIO %s · %dx%d\n%s\n\n", sc.name, w, h, sc.story)
	fmt.Fprintf(&b, "=== opening frame (Lv%d) ===\n%s\n\n", m.level, m.View())
	for _, k := range keys {
		pressKey(m, k)
		poll(m, sc)
		fmt.Fprintf(&b, "=== after %q (Lv%d, selected %s) ===\n%s\n\n", k, m.level, selectedName(m), m.View())
	}
	return b.String()
}

// poll is the refresh that follows every keypress in the real deck: the
// selected session's trail and conversation land again, so a session just
// selected has its reader by the next frame rather than "reading…" for good.
func poll(m *Model, sc scene) {
	key := m.selectedKey
	tr, ok := sc.trails[key]
	if !ok {
		return
	}
	m.Update(fleetMsg{sessions: m.sessions, at: m.now, trailFor: key, hasTrail: true,
		trail: tr, events: eventsBehind(tr, sc.activity(key)), trails: sc.trails})
}

func selectedName(m *Model) string {
	if s, ok := m.selected(); ok {
		return sessionName(s.Info)
	}
	return "—"
}

// TestScenarioWalkthrough holds every scenario at every width to the deck's
// width discipline, and writes the walkthroughs out when asked.
func TestScenarioWalkthrough(t *testing.T) {
	forceASCII(t)
	out := os.Getenv("COMPASS_SCENARIO_OUT")
	keys := canonicalKeys
	if custom := os.Getenv("COMPASS_SCENARIO_KEYS"); custom != "" {
		keys = strings.Split(custom, ",")
	}
	if out != "" {
		if err := os.MkdirAll(out, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, sc := range allScenes() {
		for _, size := range [][2]int{{80, 24}, {100, 30}, {120, 34}, {152, 40}, {220, 48}} {
			w, h := size[0], size[1]
			m := sceneModel(sc, w, h)
			for _, k := range append([]string{}, keys...) {
				frame := m.View()
				for _, line := range strings.Split(frame, "\n") {
					if x := lipgloss.Width(line); x > w {
						t.Errorf("%s %dx%d before %q: line runs past the terminal (%d): %q", sc.name, w, h, k, x, line)
					}
				}
				pressKey(m, k)
				poll(m, sc)
			}
			if out != "" {
				path := filepath.Join(out, fmt.Sprintf("%s-%dx%d.txt", sc.name, w, h))
				if err := os.WriteFile(path, []byte(walkthrough(sc, w, h, keys)), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
