// Package journey turns a session's transcript into the trail the panel draws:
// a chain of legs — contiguous spans of one kind of work — plus the subagent
// branches that fork off them. Everything here is deterministic, offline and
// cheap: a vote table over tool calls, folded into legs by a small state
// machine with hysteresis so one stray Read never renames a build.
package journey

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/deephanson94/compass/internal/transcript"
)

// Class is the kind of work a leg is made of (SPEC §2.2).
type Class int

// The seven classes the segmenter can produce. WAIT from the spec is a session
// state, not a leg, and lives in package state.
const (
	Scout Class = iota
	Design
	Build
	Fix
	Test
	Ship
	Docs
)

// String returns the lowercase name of the class.
func (c Class) String() string {
	switch c {
	case Scout:
		return "scout"
	case Design:
		return "design"
	case Build:
		return "build"
	case Fix:
		return "fix"
	case Test:
		return "test"
	case Ship:
		return "ship"
	case Docs:
		return "docs"
	default:
		return "unknown"
	}
}

// strong reports whether a single differing vote of this class is enough to
// split a leg (rule 3). Strong beats are unmistakable phase changes: a test
// run, a commit, a plan. Everything else only applies pressure (rule 4).
func strong(c Class) bool {
	return c == Test || c == Ship || c == Design
}

// agentTool forks a branch instead of voting: it is a different lane of the
// journey, not a change of class on this one.
const agentTool = "Agent"

// scoutTools only look at things.
var scoutTools = map[string]bool{
	"Read":      true,
	"Grep":      true,
	"Glob":      true,
	"WebFetch":  true,
	"WebSearch": true,
	"Explore":   true,
}

// planTools bracket plan mode, where the session is designing rather than doing.
var planTools = map[string]bool{
	"EnterPlanMode": true,
	"ExitPlanMode":  true,
}

// writeTools put bytes on disk: Docs when the path reads like prose, else Build.
var writeTools = map[string]bool{
	"Edit":         true,
	"Write":        true,
	"NotebookEdit": true,
}

// docExts are the extensions that make a write documentation.
var docExts = map[string]bool{".md": true, ".rst": true, ".txt": true}

// testRunners are matched as substrings of a Bash command's first line. The
// pattern's first word doubles as the leg's label ("pytest", "go", "cargo").
var testRunners = []string{
	"pytest",
	"go test",
	"jest",
	"vitest",
	"cargo test",
	"npm test",
	"yarn test",
	"make test",
	"rspec",
	"phpunit",
	"mvn test",
	"gradle test",
	"tox",
	"unittest",
}

// shipCommands are matched as substrings too; the pattern's second word is the
// label ("commit", "push", "pr", "tag", "release").
var shipCommands = []string{
	"git commit",
	"git push",
	"git tag",
	"gh pr",
	"gh release",
}

// readOnlyCommands are matched on the command's first word only — `find` is
// scouting, but `xargs find`-style pipelines are not worth guessing about.
var readOnlyCommands = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true, "grep": true,
	"rg": true, "find": true, "fd": true, "wc": true, "tree": true,
	"stat": true, "file": true, "which": true,
}

// vote is one tool_use's opinion, carrying the crumbs a leg needs for its
// label: the file it touched and, for Test/Ship, the command word.
type vote struct {
	class   Class
	file    string // basename of the tool's file_path, "" if none
	keyword string // "pytest", "go", "commit", "push"… "" if none
}

// bashInput, pathInput and agentInput are the only three shapes of tool input
// the segmenter reads. Decoding into these instead of a map keeps replaying a
// long transcript cheap.
type bashInput struct {
	Command string `json:"command"`
}

type pathInput struct {
	FilePath string `json:"file_path"`
}

type agentInput struct {
	Description string `json:"description"`
}

// Classify returns the class vote for one event and whether it votes at all.
// Non-substantive events and text-only assistant turns do not vote; an event
// with several tool_uses is reported by its first voting one (the segmenter
// itself folds every tool_use separately, see Observe).
func Classify(ev transcript.Event) (Class, bool) {
	for _, use := range ev.ToolUses {
		if v, ok := classifyUse(use); ok {
			return v.class, true
		}
	}
	return Scout, false
}

// classifyUse applies the vote table to a single tool_use, first matching rule
// wins.
func classifyUse(use transcript.ToolUse) (vote, bool) {
	switch {
	case scoutTools[use.Name]:
		return vote{class: Scout, file: basenameOf(use.Input)}, true
	case use.Name == agentTool:
		return vote{}, false
	case planTools[use.Name]:
		return vote{class: Design}, true
	case writeTools[use.Name]:
		file := basenameOf(use.Input)
		if docExts[strings.ToLower(filepath.Ext(file))] {
			return vote{class: Docs, file: file}, true
		}
		return vote{class: Build, file: file}, true
	case use.Name == "Bash":
		return classifyCommand(commandOf(use.Input)), true
	default:
		return vote{}, false
	}
}

// classifyCommand reads a shell command the way a glance would: is it running
// the tests, is it shipping, is it just looking around, or is it work?
func classifyCommand(cmd string) vote {
	line := firstLine(cmd)
	lower := strings.ToLower(line)

	for _, pattern := range testRunners {
		if strings.Contains(lower, pattern) {
			return vote{class: Test, keyword: firstWord(pattern)}
		}
	}
	for _, pattern := range shipCommands {
		if strings.Contains(lower, pattern) {
			return vote{class: Ship, keyword: lastWord(pattern)}
		}
	}
	if readOnlyCommands[firstWord(lower)] {
		return vote{class: Scout}
	}
	return vote{class: Build}
}

// branchLabel is the Agent call's own description, the one line it already
// wrote about what it went off to do.
func branchLabel(input json.RawMessage) string {
	if len(input) == 0 {
		return "agent"
	}
	var in agentInput
	if err := json.Unmarshal(input, &in); err != nil {
		return "agent"
	}
	if label := firstLine(in.Description); label != "" {
		return label
	}
	return "agent"
}

func commandOf(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var in bashInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ""
	}
	return in.Command
}

// basenameOf pulls the file_path out of a tool input and keeps only its last
// element — the trail has no room for directories.
func basenameOf(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var in pathInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ""
	}
	if in.FilePath == "" {
		return ""
	}
	return filepath.Base(in.FilePath)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}

func lastWord(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexAny(s, " \t"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// clip truncates to max display runes, marking the cut with an ellipsis.
func clip(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return strings.TrimRight(string(r[:max-1]), " ") + "…"
}
