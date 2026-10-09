// Package journey turns a session's transcript into the trail the panel draws:
// a chain of legs — contiguous spans of one kind of work — plus the subagent
// branches that fork off them. Everything here is deterministic, offline and
// cheap: a vote table over tool calls, folded into legs by a small state
// machine with hysteresis so one stray Read never renames a build.
package journey

import (
	"encoding/json"
	"path/filepath"
	"regexp"
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

// readOnlyCommands are the verbs that only look: a command line is scouting
// when every stage of it (split on |, &&, || and ;) opens with one of these
// and nothing writes a file. Anything else is work, which is the safe side
// to err on: a build mistaken for a look hides work, a look mistaken for a
// build only names it too eagerly.
var readOnlyCommands = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true, "grep": true,
	"rg": true, "find": true, "fd": true, "wc": true, "tree": true,
	"stat": true, "file": true, "which": true,
	// Asking the machine, not changing it: the environment, the process
	// table, where we are, what a file holds.
	"env": true, "printenv": true, "pwd": true, "echo": true, "jq": true,
	"less": true, "more": true, "diff": true, "cmp": true, "du": true,
	"df": true, "ps": true, "uname": true, "date": true, "whoami": true,
	"id": true, "hostname": true, "sort": true, "uniq": true, "cut": true,
	"tr": true, "awk": true, "sed": true, "xxd": true, "od": true,
	"basename": true, "dirname": true, "realpath": true, "readlink": true,
	"type": true, "command": true, "true": true,
}

// readOnlySub are the tools whose first word is not enough: `git status`
// looks, `git checkout` does not.
var readOnlySub = map[string]map[string]bool{
	"git": {"status": true, "log": true, "diff": true, "show": true, "blame": true,
		"branch": true, "remote": true, "rev-parse": true, "ls-files": true,
		"grep": true, "shortlog": true, "describe": true, "reflog": true},
	"go": {"list": true, "env": true, "version": true, "doc": true},
}

// shellStages splits a command line on its separators: |, ||, && and ;.
var shellStages = regexp.MustCompile(`\|\||&&|\||;`)

// looksOnly reports whether every stage of a command line only looks.
func looksOnly(line string) bool {
	for _, stage := range shellStages.Split(line, -1) {
		fields := strings.Fields(stage)
		if len(fields) == 0 {
			continue
		}
		verb := strings.ToLower(fields[0])
		if readOnlyCommands[verb] {
			continue
		}
		if subs, ok := readOnlySub[verb]; ok && len(fields) > 1 && subs[strings.ToLower(fields[1])] {
			continue
		}
		return false
	}
	return true
}

// preamble is what a command line opens with before its work: variable
// assignments and a change of directory, each ended by ; or &&.
var preamble = regexp.MustCompile(`^\s*(?:(?:[A-Za-z_][A-Za-z0-9_]*=(?:'[^']*'|"[^"]*"|\S*)|cd\s+\S+)\s*(?:;|&&)\s*)+`)

// CommandCore is a command without its preamble: "M=/long/path; cat $M/x"
// reads "cat $M/x", which is what the call was for.
func CommandCore(cmd string) string {
	if core := preamble.ReplaceAllString(cmd, ""); strings.TrimSpace(core) != "" {
		return core
	}
	return cmd
}

// vote is one tool_use's opinion, carrying the crumbs a leg needs for its
// label: the file it touched and, for Test/Ship, the command word.
type vote struct {
	class   Class
	file    string // basename of the tool's file_path, "" if none
	keyword string // "pytest", "go", "commit", "push"… "" if none
	id      string // the tool_use id, so its result can be recognised (M2 rule 2)
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
	// A teammate spawn names the teammate: Claude Code's agent teams put
	// it in `name`, and the id its messages come back under (#395) is
	// that name. `teammate_id` is read too, for a harness that spells
	// it as the envelope does. Empty for a plain subagent.
	Name       string `json:"name"`
	TeammateID string `json:"teammate_id"`
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

// classifyUse votes on a single tool_use and stamps the vote with the call's
// id, so the segmenter can recognise the result when it comes back.
func classifyUse(use transcript.ToolUse) (vote, bool) {
	v, ok := voteFor(use)
	if !ok {
		return vote{}, false
	}
	v.id = use.ID
	return v, true
}

// voteFor is the vote table itself, first matching rule wins.
func voteFor(use transcript.ToolUse) (vote, bool) {
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
	line := firstLine(CommandCore(cmd))
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
	// A command that writes a file is work whatever its verb: `cat >>
	// README.md <<EOF` starts with a reader's word and is an edit.
	if file, ok := WrittenFile(line); ok {
		if docExts[strings.ToLower(filepath.Ext(file))] {
			return vote{class: Docs, file: file}
		}
		return vote{class: Build, file: file}
	}
	if looksOnly(line) {
		return vote{class: Scout}
	}
	return vote{class: Build}
}

// shellWrite finds where a command line writes: a redirect (> or >>) to a
// path, `tee [-a] path`, or `sed -i … path`. A redirect onto a descriptor
// (2>&1) or into /dev/null writes nothing anyone keeps.
var (
	redirectTo = regexp.MustCompile(`(?:^|[^0-9&>])>>?\s*([^\s&|;<>()]+)`)
	teeTo      = regexp.MustCompile(`\btee\s+(?:-a\s+)?([^\s&|;<>()-][^\s&|;<>()]*)`)
	sedInPlace = regexp.MustCompile(`\bsed\s+-i\S*\s.*?([^\s'"&|;<>()]+)\s*$`)
)

// WrittenFile is the basename of the file a shell command line writes, if
// it writes one: what the vote table and the trail's edit clock both read.
func WrittenFile(line string) (string, bool) {
	for _, m := range redirectTo.FindAllStringSubmatch(line, -1) {
		if target := m[1]; target != "/dev/null" && !strings.HasPrefix(target, "&") {
			return filepath.Base(strings.Trim(target, `"'`)), true
		}
	}
	for _, re := range []*regexp.Regexp{teeTo, sedInPlace} {
		if m := re.FindStringSubmatch(line); m != nil {
			return filepath.Base(m[1]), true
		}
	}
	return "", false
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

// branchTeammate is the teammate a spawn's input names, or "" for a
// subagent that is not a teammate. It is the join a teammate's relayed
// report closes its lane by (#395): read off the input at the fork,
// never guessed from a label.
func branchTeammate(input json.RawMessage) string {
	if len(input) == 0 {
		return ""
	}
	var in agentInput
	if err := json.Unmarshal(input, &in); err != nil {
		return ""
	}
	if id := strings.TrimSpace(in.Name); id != "" {
		return id
	}
	return strings.TrimSpace(in.TeammateID)
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
