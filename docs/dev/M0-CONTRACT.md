# M0 Contract — tailer + state machine + fleet (deck skeleton)

This is the binding API contract for milestone M0. The implementation and the test
suite are written in parallel against this document; if reality forces a deviation,
the contract gets amended first (one place, one truth).

M0 goal (ARCHITECTURE §7): *state detection is trustworthy — the amber dot is never
wrong.* No trail graph, no narrator, no tmux yet. Deliverables: `compass` TUI (deck
layout: fleet left, session detail card right), `compass status` one-shot.

## Repo layout

```
go.mod                      module github.com/deephanson94/compass   (go 1.24)
cmd/compass/main.go         CLI entry: default TUI, `status` subcommand, -root flag
internal/transcript/        event types, ParseLine, Tailer
internal/state/             per-session state machine
internal/fleet/             discovery + Manager (owns tailers + machines)
internal/ui/                bubbletea deck UI
testdata/                   fixtures (owned by the test suite)
```

Dependencies: `charmbracelet/bubbletea`, `charmbracelet/lipgloss` only (no bubbles,
no fsnotify in M0 — polling). Stdlib everywhere else.

## package transcript

```go
type EventType string

const (
    EventUser       EventType = "user"
    EventAssistant  EventType = "assistant"
    EventAttachment EventType = "attachment"
    EventQueueOp    EventType = "queue-operation"
    EventUnknown    EventType = "unknown" // any other type: fail-soft, never error
)

type ToolUse struct {
    ID    string          // tool_use block id
    Name  string          // e.g. "Bash", "Read", "AskUserQuestion"
    Input json.RawMessage // raw input object (may be nil)
}

type ToolResult struct {
    ToolUseID string
    IsError   bool
}

type Event struct {
    Type        EventType
    UUID        string
    ParentUUID  string
    Timestamp   time.Time // zero if absent/unparseable
    SessionID   string
    CWD         string
    GitBranch   string
    Version     string
    IsSidechain bool
    Text        string       // assistant: all text blocks joined "\n"; user: string content (empty if content is a block array)
    ToolUses    []ToolUse    // assistant tool_use blocks, in order
    ToolResults []ToolResult // tool_result blocks inside user-type lines
}

// ParseLine parses one JSONL line. Unknown `type` values return Event{Type:
// EventUnknown} with whatever common fields parsed, and a nil error. Only invalid
// JSON returns an error.
func ParseLine(line []byte) (Event, error)
```

Parsing notes (verified against real transcripts, ARCHITECTURE §2):
- `message.content` is either a plain string (user prompts) or an array of blocks
  (`text`, `thinking`, `tool_use`, `tool_result`). Handle both. Ignore `thinking`
  blocks for `Text`.
- Top-level fields: `type`, `uuid`, `parentUuid`, `timestamp` (RFC3339),
  `sessionId`, `cwd`, `gitBranch`, `version`, `isSidechain`.
- `queue-operation` lines have `content` (string) at top level, no `message`.

```go
type Tailer struct { /* opaque */ }

func NewTailer(path string) *Tailer

// Poll reads newly appended bytes since the last call and returns the parsed
// events of every COMPLETE line ("\n"-terminated). Semantics:
//   - a trailing partial line is NOT consumed; it is returned by a later Poll
//     once its newline arrives
//   - malformed-JSON lines are skipped (Skipped() counts them), never returned,
//     never abort the batch
//   - if the file shrank below the stored offset (truncate/rotate), reset offset
//     to 0 and read from the start
//   - a missing file returns (nil, nil) — the session may not have flushed yet
func (t *Tailer) Poll() ([]Event, error)

func (t *Tailer) Skipped() int
```

## package state

```go
type State int

const (
    Working State = iota
    NeedsYou
    Idle
    Stuck
)

func (s State) String() string // "working", "needs-you", "idle", "stuck"

const StuckAfter = 90 * time.Second

type Snapshot struct {
    State    State
    Since    time.Time // timestamp of the event that established this condition
    Reason   string    // short human phrase, e.g. "turn ended with a question"
    Activity string    // hint, e.g. `Bash: pytest tests/auth -x`, `reading middleware.py`, "thinking…", "idle"
}

type Machine struct { /* opaque */ }

func NewMachine() *Machine
func (m *Machine) Observe(ev transcript.Event) // feed events in file order
func (m *Machine) Evaluate(now time.Time) Snapshot
```

### State rules (exhaustive, in precedence order)

Definitions: a ToolUse is **pending** if no later ToolResult with a matching
`ToolUseID` has been observed. An event is **substantive** if it is `user` with
non-empty Text (a real prompt) or `assistant`. `attachment`/`queue-operation`/
`unknown`/tool-result-only-user lines refresh `lastEventAt` but are not substantive.

1. **No substantive events observed** → `Idle`, reason `"no activity yet"`.
2. **A pending `AskUserQuestion` tool_use exists** → `NeedsYou`, reason
   `"waiting on your answer"`. (Immediate — no quiet threshold.)
3. **Any other pending tool_use exists** (mid-turn):
   - `now − lastEventAt < StuckAfter` → `Working`
   - else → `Stuck`, reason `"no output for <dur> mid-turn"` (dur rounded to s/m).
4. **No pending tool_use, last substantive event is `assistant`**:
   - **if a tool_result was observed after that assistant event**, the turn is
     still in flight — the model owes its next beat (amended post-review; the
     original rule mis-reported this window as idle). Same timing split as rule 3:
     `Working`, reason `"processing results"`, activity `"thinking…"` — or `Stuck`.
   - else the turn is complete: `Text`, right-trimmed of whitespace and markdown
     decoration (`*`, `_`, `` ` ``, `)`), ends with `?` → `NeedsYou`, reason
     `"turn ended with a question"`
   - else → `Idle`, reason `"turn complete"`.
5. **Last substantive event is a `user` prompt** (model hasn't replied yet):
   - same timing split as rule 3: `Working` (reason `"starting turn"`) or `Stuck`.

`Activity` derivation: from the most recent tool_use — `Bash` → `Bash: <input.command,
first line, max 40 cols>`; `Read`/`Edit`/`Write` → `<verb>ing <basename of
input.file_path>`; other tools → tool name. If the last assistant event had text and
no tool_use mid-turn → `"thinking…"`. At Idle → `"idle"`.

`Since`: rule 2/3/5 → timestamp of the event that opened the wait (the tool_use /
prompt); rule 4 → timestamp of the closing assistant event. Fallback: `lastEventAt`.

## package fleet

```go
type SessionInfo struct {
    ID             string    // session uuid (filename stem)
    TranscriptPath string
    ProjectSlug    string    // directory name under projects/
    CWD            string
    GitBranch      string
    Title          string    // first user prompt: first line, max 80 runes, "…" if cut
    StartedAt      time.Time // first event timestamp
    LastEventAt    time.Time // last event timestamp (file mtime as fallback)
}

type Session struct {
    Info SessionInfo
    Snap state.Snapshot
}

// Discover scans <root>/projects/<slug>/*.jsonl. It does NOT recurse into session
// subdirectories (subagents are M1). Empty files are skipped. Result sorted by
// LastEventAt descending. A missing root or projects dir returns (nil, nil).
func Discover(root string) ([]SessionInfo, error)

type Manager struct { /* opaque */ }

func NewManager(root string) *Manager

// Refresh re-discovers sessions, Polls each tailer, feeds machines, and returns
// the fleet sorted for display: NeedsYou (longest-waiting first), Stuck (longest
// first), Working (most recent activity first), Idle (most recent first).
func (m *Manager) Refresh(now time.Time) ([]Session, error)

// StatusLine renders the one-shot summary for `compass status` / tmux status-right:
// counts in fleet-sort order, zero counts omitted, e.g. "▲1 ◍1 ●3" ; "○ all quiet"
// when nothing is working or waiting.
func (m *Manager) StatusLine(now time.Time) string
```

## package ui (M0 scope)

```go
// Run starts the full-screen deck TUI: fleet list left (~34 cols), detail card
// right (selected session: title, state+reason, activity, age, cwd, branch,
// transcript path). 1s tick calls Manager.Refresh. Emits OSC 2 tab title
// "⌂ compass ▲N" / "⌂ compass" on state-count changes.
func Run(mgr *fleet.Manager) error
```

Keys (M0): `1`–`9` select · `j`/`k`/arrows move · `g` select oldest needs-you ·
`?` help overlay · `q`/`ctrl+c` quit. `Tab` shows a one-line "trail arrives in M1"
notice in the detail card. Glyphs: `●` working, `▲` needs-you, `○` idle, `◍` stuck
(color reinforces: green/amber/dim/red — but layout must read in monochrome;
respect NO_COLOR via lipgloss defaults).

## cmd/compass

- `compass` → TUI (deck). `-root` flag overrides the Claude home (default:
  `$COMPASS_ROOT`, else `~/.claude`). Missing/empty root → friendly empty state,
  not an error.
- `compass status` → `Manager.StatusLine` to stdout, exit 0. Same `-root`.

## Test suite contract (testdata + *_test.go)

Scenario fixtures live in `testdata/`; tests may also build JSONL inline. Required
coverage (each a named test):

| # | Scenario | Expects |
|---|----------|---------|
| T1 | assistant tool_use (Bash) pending, 5s quiet | Working, Activity `Bash: …` |
| T2 | completed turn, text ends `…proceed?` | NeedsYou, "turn ended with a question" |
| T3 | pending AskUserQuestion, 0s quiet | NeedsYou (immediate) |
| T4 | completed turn, statement text | Idle |
| T5 | pending Bash, 120s quiet | Stuck |
| T6 | lone user prompt 3s ago | Working "starting turn" |
| T7 | lone user prompt 120s ago | Stuck |
| T8 | tailer: append in 3 chunks incl. a split mid-line | partial line held, then delivered; no event lost or duplicated |
| T9 | tailer: malformed JSON line between valid lines | skipped, Skipped()==1, valid events delivered |
| T10 | tailer: truncation (size < offset) | offset reset, events re-read |
| T11 | ParseLine: user content as block array (tool_result) | Text empty, ToolResults populated |
| T12 | ParseLine: unknown type `"atis-latch"` | EventUnknown, nil error |
| T13 | Discover: 2 projects, 3 sessions, one empty file, one subagent dir | 3 sessions, subagents not recursed, sorted desc |
| T14 | Manager.Refresh ordering: one of each state | NeedsYou, Stuck, Working, Idle |
| T15 | StatusLine: mixed fleet / all-idle | `▲1 ◍1 ●2` shape / `○ all quiet` |
| T16 | ui.View() golden: 80×24, NO_COLOR, 3-session fleet | stable snapshot (strip trailing spaces); update via `-update` flag |
| T17 | markdown-decorated question `**ok?**` | NeedsYou (trim rule) |
| T18 | tool_result returned 5s ago, model's reply not yet written | Working "processing results" (NOT idle); at 120s → Stuck |

Golden files under `testdata/golden/`. All tests must pass with `go test ./...`
offline (no network, no real `~/.claude`).

## Addendum — ambiguities resolved during implementation (now pinned by tests)

- **Discover enrichment**: `CWD`/`GitBranch`/`Title`/`StartedAt` come from a bounded
  head read (≤64 lines); `LastEventAt` from a bounded 64 KB backwards tail scan
  skipping timestamp-less bookkeeping lines; file mtime only as fallback.
- **StatusLine**: all four counts in fleet order with zeros omitted (idle shown as
  `○N` alongside actives); `○ all quiet` whenever nothing is needs-you, stuck, or
  working — including an empty fleet.
- **Reasons**: rule 3 Working = `"tool call in flight"`; rule 4 amended-branch
  Working = `"processing results"`; rule 4 NeedsYou activity = `"awaiting your
  reply"`; rule 5 reuses the `"no output for <dur> mid-turn"` stuck phrasing.
- **Since with multiple pending tool_uses**: the oldest pending one; `Activity`
  from the most recent tool_use.
- **Sort keys**: needs-you/stuck by `Snap.Since` ascending (longest wait first);
  working/idle by `Info.LastEventAt` descending; ties by session ID.
- **Blank transcript lines** are dropped silently — not counted by `Skipped()`.
- **`queue-operation`** fills `Event.Text` from its top-level `content` but is
  never substantive.
- **ui testability**: `New(mgr) *Model`, `SetSize(w,h)`, `SetSessions(s, now)`,
  `View()` are exported precisely so T16 can golden-test a frame; goldens are
  rendered under a forced ASCII color profile.
