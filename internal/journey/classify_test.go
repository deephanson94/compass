package journey_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/deephanson94/compass/internal/journey"
	"github.com/deephanson94/compass/internal/transcript"
)

// base is the instant every event in this package hangs off; offsets below are
// relative to it so tests never touch the wall clock.
var base = time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)

func at(offset time.Duration) time.Time { return base.Add(offset) }

// ---------------------------------------------------------------- builders
//
// These are shared with segmenter_test.go (same test package).

// rawJSON marshals a map into a tool_use input, so no test ever hand-escapes
// a quote inside a shell command.
func rawJSON(v map[string]string) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

func cmdInput(cmd string) json.RawMessage   { return rawJSON(map[string]string{"command": cmd}) }
func pathInput(path string) json.RawMessage { return rawJSON(map[string]string{"file_path": path}) }

// use builds an assistant event carrying exactly one tool_use.
func use(offset time.Duration, id, name string, input json.RawMessage) transcript.Event {
	return uses(offset, transcript.ToolUse{ID: id, Name: name, Input: input})
}

// uses builds an assistant event carrying several tool_use blocks in order.
func uses(offset time.Duration, tus ...transcript.ToolUse) transcript.Event {
	return transcript.Event{
		Type: transcript.EventAssistant, UUID: "a", SessionID: "s",
		Timestamp: at(offset),
		ToolUses:  tus,
	}
}

func bash(offset time.Duration, id, cmd string) transcript.Event {
	return use(offset, id, "Bash", cmdInput(cmd))
}

func read(offset time.Duration, id, path string) transcript.Event {
	return use(offset, id, "Read", pathInput(path))
}

func edit(offset time.Duration, id, path string) transcript.Event {
	return use(offset, id, "Edit", pathInput(path))
}

func writeFile(offset time.Duration, id, path string) transcript.Event {
	return use(offset, id, "Write", pathInput(path))
}

// agent builds an Agent tool_use; an empty description omits the field entirely.
func agent(offset time.Duration, id, description string) transcript.Event {
	in := map[string]string{"prompt": "go and look at the payment code"}
	if description != "" {
		in["description"] = description
	}
	return use(offset, id, "Agent", rawJSON(in))
}

func prompt(offset time.Duration, text string) transcript.Event {
	return transcript.Event{
		Type: transcript.EventUser, UUID: "u", SessionID: "s",
		Timestamp: at(offset), Text: text,
	}
}

func say(offset time.Duration, text string) transcript.Event {
	return transcript.Event{
		Type: transcript.EventAssistant, UUID: "a", SessionID: "s",
		Timestamp: at(offset), Text: text,
	}
}

func result(offset time.Duration, id string, isErr bool) transcript.Event {
	return transcript.Event{
		Type: transcript.EventUser, UUID: "u", SessionID: "s",
		Timestamp:   at(offset),
		ToolResults: []transcript.ToolResult{{ToolUseID: id, IsError: isErr}},
	}
}

// ---------------------------------------------------------------- T19

// classifyCase is one row of the contract's vote table.
type classifyCase struct {
	name   string
	ev     transcript.Event
	want   journey.Class
	wantOK bool
}

func runClassifyCases(t *testing.T, cases []classifyCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := journey.Classify(tc.ev)
			if ok != tc.wantOK {
				t.Fatalf("Classify(%s) voted = %v, want %v (got class %v)", tc.name, ok, tc.wantOK, got)
			}
			if ok && got != tc.want {
				t.Errorf("Classify(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// bashCase is the common shape: one Bash tool_use, one expected class.
func bashCase(cmd string, want journey.Class) classifyCase {
	return classifyCase{name: cmd, ev: bash(0, "tu", cmd), want: want, wantOK: true}
}

// T19 — the tool rows of the vote table: read-only tools, plan mode, docs vs
// build edits, and the tools that do not vote at all.
func TestT19ClassifyToolRows(t *testing.T) {
	runClassifyCases(t, []classifyCase{
		// Read/Grep/Glob/WebFetch/WebSearch/Explore → Scout.
		{"Read", read(0, "tu", "/w/auth.go"), journey.Scout, true},
		{"Grep", use(0, "tu", "Grep", rawJSON(map[string]string{"pattern": "func "})), journey.Scout, true},
		{"Glob", use(0, "tu", "Glob", rawJSON(map[string]string{"pattern": "**/*.go"})), journey.Scout, true},
		{"WebFetch", use(0, "tu", "WebFetch", rawJSON(map[string]string{"url": "https://example.test"})), journey.Scout, true},
		{"WebSearch", use(0, "tu", "WebSearch", rawJSON(map[string]string{"query": "go interfaces"})), journey.Scout, true},
		{"Explore", use(0, "tu", "Explore", rawJSON(map[string]string{"query": "where does auth live"})), journey.Scout, true},

		// Agent forks a branch; it is never a class vote.
		{"Agent with description", agent(0, "tu", "scout payment flows"), 0, false},
		{"Agent without description", agent(0, "tu", ""), 0, false},

		// Plan mode → Design.
		{"EnterPlanMode", use(0, "tu", "EnterPlanMode", json.RawMessage(`{}`)), journey.Design, true},
		{"ExitPlanMode", use(0, "tu", "ExitPlanMode", rawJSON(map[string]string{"plan": "1. rewrite the tailer"})), journey.Design, true},

		// Prose extensions → Docs.
		{"Edit .md", edit(0, "tu", "/w/docs/SPEC.md"), journey.Docs, true},
		{"Write .rst", writeFile(0, "tu", "/w/docs/index.rst"), journey.Docs, true},
		{"NotebookEdit .txt", use(0, "tu", "NotebookEdit", pathInput("/w/notes.txt")), journey.Docs, true},
		{"Write .md at repo root", writeFile(0, "tu", "README.md"), journey.Docs, true},

		// Everything else those tools touch → Build.
		{"Edit .go", edit(0, "tu", "/w/internal/parse/parser.go"), journey.Build, true},
		{"Write .py", writeFile(0, "tu", "/w/svc/handler.py"), journey.Build, true},
		{"NotebookEdit .ipynb", use(0, "tu", "NotebookEdit", pathInput("/w/explore.ipynb")), journey.Build, true},
		// The extension test is on the END of the path, not anywhere in it.
		{"Edit md.go is not docs", edit(0, "tu", "/w/internal/md.go"), journey.Build, true},
		{"Edit .md.go is not docs", edit(0, "tu", "/w/render.md.go"), journey.Build, true},
		{"Edit .txtx is not docs", edit(0, "tu", "/w/fixture.txtx"), journey.Build, true},
		{"Edit with no file_path", use(0, "tu", "Edit", json.RawMessage(`{"old_string":"a","new_string":"b"}`)), journey.Build, true},

		// "any other tool → no vote".
		{"TodoWrite", use(0, "tu", "TodoWrite", json.RawMessage(`{"todos":[]}`)), 0, false},
		{"AskUserQuestion", use(0, "tu", "AskUserQuestion", rawJSON(map[string]string{"question": "ship it?"})), 0, false},
		{"Task", use(0, "tu", "Task", rawJSON(map[string]string{"prompt": "x"})), 0, false},
		{"BashOutput", use(0, "tu", "BashOutput", rawJSON(map[string]string{"bash_id": "1"})), 0, false},
		{"KillShell", use(0, "tu", "KillShell", rawJSON(map[string]string{"shell_id": "1"})), 0, false},
		{"Skill", use(0, "tu", "Skill", rawJSON(map[string]string{"skill": "pdf"})), 0, false},
		{"mcp tool", use(0, "tu", "mcp__github__list_issues", json.RawMessage(`{}`)), 0, false},
		{"unknown tool name", use(0, "tu", "SomeFutureTool", json.RawMessage(`{}`)), 0, false},
		// Case matters: tool names are exact.
		{"lowercase read is not Read", use(0, "tu", "read", pathInput("/w/a.go")), 0, false},
	})
}

// T19 — Bash test-runner substrings. Matching is a substring of the first line,
// so a runner named anywhere in the command wins the row.
func TestT19ClassifyBashTestRunners(t *testing.T) {
	runClassifyCases(t, []classifyCase{
		bashCase("pytest tests/auth -x", journey.Test),
		bashCase("go test ./... -run TestSegmenter", journey.Test),
		// "cargo test" happens to contain the literal "go test"; both rows vote
		// Test, so substring matching stays honest either way.
		bashCase("cargo test --all-features", journey.Test),
		bashCase("npx jest --watch=false", journey.Test),
		bashCase("npx vitest run", journey.Test),
		bashCase("npm test", journey.Test),
		bashCase("npm test -- --coverage", journey.Test),
		bashCase("yarn test --coverage", journey.Test),
		bashCase("make test", journey.Test),
		bashCase("bundle exec rspec spec/models", journey.Test),
		bashCase("./vendor/bin/phpunit --testdox", journey.Test),
		bashCase("mvn test -q", journey.Test),
		bashCase("gradle test --info", journey.Test),
		bashCase("tox -e py311", journey.Test),
		bashCase("python -m unittest discover", journey.Test),

		// Near-misses: none of these contain a runner pattern.
		bashCase("npm run build", journey.Build),
		bashCase("npm run test:watch", journey.Build), // "npm test" is not a substring of this
		bashCase("go build ./...", journey.Build),
		bashCase("go vet ./...", journey.Build),
		bashCase("cargo build --release", journey.Build),
		bashCase("make build", journey.Build),
		bashCase("yarn install", journey.Build),
	})
}

// T19 — Bash ship patterns and the git/gh commands that are NOT ship.
func TestT19ClassifyBashShipPatterns(t *testing.T) {
	runClassifyCases(t, []classifyCase{
		bashCase(`git commit -m "wip: auth"`, journey.Ship),
		bashCase("git commit --amend --no-edit", journey.Ship),
		bashCase("git push origin main", journey.Ship),
		bashCase("git push --force-with-lease", journey.Ship),
		bashCase("git tag -a v1.2.0 -m release", journey.Ship),
		bashCase("gh pr create --fill", journey.Ship),
		bashCase("gh pr merge 42 --squash", journey.Ship),
		bashCase("gh release create v1.2.0 --notes x", journey.Ship),

		// git/gh verbs outside the ship list fall through to "Bash otherwise".
		bashCase("git status --short", journey.Build),
		bashCase("git diff --stat", journey.Build),
		bashCase("git log --oneline -20", journey.Build),
		bashCase("gh issue list --limit 5", journey.Build),
		bashCase("gh repo view", journey.Build),
	})
}

// T19 — the read-only first-word list. This row is first-word, never substring.
func TestT19ClassifyBashReadOnlyFirstWord(t *testing.T) {
	runClassifyCases(t, []classifyCase{
		bashCase("ls -la internal/", journey.Scout),
		bashCase("cat README", journey.Scout),
		bashCase("head -n 20 main.go", journey.Scout),
		bashCase("tail -f server.log", journey.Scout),
		bashCase("grep -rn TODO internal/", journey.Scout),
		bashCase("rg --files", journey.Scout),
		bashCase("find . -name '*.jsonl'", journey.Scout),
		bashCase("fd -e go", journey.Scout),
		bashCase("wc -l main.go", journey.Scout),
		bashCase("tree -L 2", journey.Scout),
		bashCase("stat main.go", journey.Scout),
		bashCase("file bin/compass", journey.Scout),
		bashCase("which claude", journey.Scout),
		// Bare, no arguments.
		bashCase("ls", journey.Scout),

		// A first word that merely STARTS with a read-only word is not that word.
		bashCase("lsof -i :8080", journey.Build),
		bashCase("catalog --list", journey.Build),
		bashCase("findutils --version", journey.Build),
		bashCase("statx /w", journey.Build),
		// ...and one that merely contains it somewhere.
		bashCase("./ls", journey.Build),
		bashCase("xargs ls", journey.Build),
		bashCase("docker ps", journey.Build),

		// "Bash otherwise" → Build.
		bashCase("mkdir -p internal/journey", journey.Build),
		bashCase("python manage.py runserver", journey.Build),
		bashCase("curl -s https://example.test", journey.Build),
		bashCase("gofmt -l .", journey.Build),
	})
}

// T19 — first matching rule wins: the table's order is test runners, then ship
// patterns, then the read-only first word, then Build. These commands satisfy
// two rows at once and must land on the earlier one.
func TestT19ClassifyBashRuleOrder(t *testing.T) {
	runClassifyCases(t, []classifyCase{
		// Runner substring beats the ship pattern.
		{`git commit mentioning "go test"`, bash(0, "tu", `git commit -m "add go test to CI"`), journey.Test, true},
		// Runner substring beats the read-only first word.
		{`grep for "pytest"`, bash(0, "tu", `grep -rn "pytest" .`), journey.Test, true},
		{"ls then npm test", bash(0, "tu", "ls tests && npm test"), journey.Test, true},
		// Ship pattern beats the read-only first word.
		{`cat piped into grep "git push"`, bash(0, "tu", `cat Makefile | grep "git push"`), journey.Ship, true},
		{"find with git tag in it", bash(0, "tu", `find . -name '*.sh' -exec grep -l "git tag" {} +`), journey.Ship, true},
	})
}

// T19 — command matching looks at the FIRST LINE only; everything after the
// first newline is invisible to the vote.
func TestT19ClassifyBashFirstLineOnly(t *testing.T) {
	runClassifyCases(t, []classifyCase{
		{"first line wins over a runner below",
			bash(0, "tu", "cd /w/service && ./configure\npytest -x"), journey.Build, true},
		{"runner on the first line, ship below",
			bash(0, "tu", "pytest -x\ngit push origin main"), journey.Test, true},
		{"read-only first line, danger below",
			bash(0, "tu", "ls -la\nrm -rf /tmp/scratch"), journey.Scout, true},
		{"ship on the first line, runner below",
			bash(0, "tu", "git push origin main\npytest -x"), journey.Ship, true},
		{"heredoc body is not the command",
			bash(0, "tu", "cat <<'EOF' > /w/notes\nmake test\nEOF"), journey.Scout, true},
	})
}

// T19 — events that carry no tool_use at all never vote.
func TestT19ClassifyNonVotingEvents(t *testing.T) {
	runClassifyCases(t, []classifyCase{
		{"assistant text only", say(0, "I'll start by reading the tailer."), 0, false},
		{"assistant text ending in a question", say(0, "Shall I ship it?"), 0, false},
		{"assistant with empty text", say(0, ""), 0, false},
		{"user prompt", prompt(0, "add caching to the token store"), 0, false},
		{"user tool_result", result(0, "tu", false), 0, false},
		{"user error tool_result", result(0, "tu", true), 0, false},
		{"attachment", transcript.Event{Type: transcript.EventAttachment, Timestamp: at(0)}, 0, false},
		{"queue operation", transcript.Event{Type: transcript.EventQueueOp, Timestamp: at(0), Text: "queued: run the tests"}, 0, false},
		{"unknown line", transcript.Event{Type: transcript.EventUnknown, Timestamp: at(0)}, 0, false},
		{"zero event", transcript.Event{}, 0, false},
	})
}

// ---------------------------------------------------------------- Class

// The renderer and the goldens depend on these exact strings.
func TestT19ClassStrings(t *testing.T) {
	want := map[journey.Class]string{
		journey.Scout:  "scout",
		journey.Design: "design",
		journey.Build:  "build",
		journey.Fix:    "fix",
		journey.Test:   "test",
		journey.Ship:   "ship",
		journey.Docs:   "docs",
	}
	for c, s := range want {
		if got := c.String(); got != s {
			t.Errorf("Class(%d).String() = %q, want %q", int(c), got, s)
		}
	}
	// Scout is the zero value, and the seven classes are distinct.
	if journey.Scout != 0 {
		t.Errorf("Scout = %d, want the zero value 0", int(journey.Scout))
	}
	seen := map[string]bool{}
	for c := range want {
		if seen[c.String()] {
			t.Errorf("duplicate Class string %q", c.String())
		}
		seen[c.String()] = true
	}
}
