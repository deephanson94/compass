# M1 Contract — segmenter, Lv1 trail, pane discovery, reveal, live mirror

Binding API contract for milestone M1, same rules as M0: implementation and tests
are written in parallel against this document; deviations amend the contract first.

M1 goal (ARCHITECTURE §7): *the journey renders at a glance and the real CLI is
visible without leaving compass.* Offline, no AI calls, no narrator.

New packages: `internal/journey` (segmenter + trail model), `internal/tmuxop`
(pane discovery, capture, reveal — all behind fakeable interfaces; CI has no tmux).
Extended: `internal/ui` (three-column deck, trail graph, mirror), `cmd/compass`.

## package journey

```go
type Class int

const (
    Scout Class = iota
    Design
    Build
    Fix
    Test
    Ship
    Docs
)

func (c Class) String() string // "scout","design","build","fix","test","ship","docs"
```

### Vote classification

```go
// Classify returns the class vote for one event and whether it votes at all.
// Non-substantive events and text-only assistant turns do not vote.
func Classify(ev transcript.Event) (Class, bool)
```

Vote table (first matching rule wins, per tool_use; an event with several
tool_uses votes once per tool_use):

| Tool | Vote |
|------|------|
| `Read`, `Grep`, `Glob`, `WebFetch`, `WebSearch`, `Explore` | Scout |
| `Agent` | no vote (it forks a branch, see below) |
| `EnterPlanMode`, `ExitPlanMode` | Design |
| `Edit`, `Write`, `NotebookEdit` with `file_path` ending `.md`/`.rst`/`.txt` | Docs |
| `Edit`, `Write`, `NotebookEdit` otherwise | Build |
| `Bash`, command matches test runners (`pytest`, `go test`, `jest`, `vitest`, `cargo test`, `npm test`, `yarn test`, `make test`, `rspec`, `phpunit`, `mvn test`, `gradle test`, `tox`, `unittest`) | Test |
| `Bash`, command matches `git commit`, `git push`, `git tag`, `gh pr`, `gh release` | Ship |
| `Bash`, first word in `ls cat head tail grep rg find fd wc tree stat file which` | Scout |
| `Bash` otherwise | Build |
| any other tool | no vote |

Command matching is on the first line of `input.command`, substring for the
multi-word patterns, first-word for the read-only list.

### Trail model

```go
type Prompt struct {
    Text string    // first line, max 60 runes, "…" if cut
    At   time.Time
}

type Leg struct {
    Class   Class
    Label   string    // heuristic, lowercase, ≤24 runes (see below)
    Start   time.Time // first vote's event
    End     time.Time // last vote's event
    Votes   int       // number of votes folded in
    Files   []string  // distinct file basenames touched, first-seen order, cap 5
    Current bool      // still open — this is HEAD (at most one, the last leg)
}

type Branch struct {
    ToolUseID string    // the Agent tool_use id
    Label     string    // Agent input "description" field, else "agent"
    Start     time.Time
    End       time.Time // zero until Done
    Done      bool      // its tool_result has been observed
    AfterLeg  int       // index into Legs of the leg open when the fork happened; -1 if none yet
}

type Trail struct {
    Prompts  []Prompt // every substantive user prompt, oldest first
    Legs     []Leg    // oldest first
    Branches []Branch // oldest first
}

type Segmenter struct{ /* opaque */ }

func NewSegmenter() *Segmenter
func (s *Segmenter) Observe(ev transcript.Event) // file order
func (s *Segmenter) Trail() Trail                // snapshot; Legs[last].Current=true iff a leg is open
```

### Segmentation rules

1. A leg opens on the first vote after start, after a boundary, or after a leg
   closes. Its class is the vote's class.
2. **Boundary**: a substantive user prompt closes the current leg immediately
   (and is appended to `Prompts`). An `Agent` tool_use opens a `Branch`
   (recording `AfterLeg`) but does NOT close the leg — a branch is not a class
   change. The only leg-closers are prompts and rules 3–4.
3. **Strong votes** (`Test`, `Ship`, `Design`): a strong vote whose class differs
   from the open leg's class closes it and opens a new leg of that class.
4. **Weak votes** (`Scout`, `Build`, `Docs`, `Fix`): a differing weak vote is
   *pressure*; three consecutive differing votes of the same class close the leg
   and open the new one (the new leg's `Start` is the first of the three).
   Fewer than three consecutive: the votes fold into the open leg (hysteresis).
5. **Fix upgrade**: a `Build` leg becomes class `Fix` (retroactively, whole leg)
   when either (a) an `IsError` tool_result is observed while it is open, or
   (b) it immediately follows a `Test` leg during which an `IsError` tool_result
   was observed.
6. `Files`: basenames from `file_path` (Edit/Write/Read/NotebookEdit inputs) of
   the leg's voting tool_uses.
7. `Label` heuristic: most-frequent entry of `Files` (ties → first seen); if no
   files, for `Test` the runner's first word (`pytest`, `go`, …); for `Ship` the
   git/gh subcommand (`commit`, `push`, `pr`); else `""` (renderer falls back to
   the bare class verb).

## package tmuxop

Everything runs through two fakeable seams; nothing in this package touches the
real system except the two `Real*` implementations.

```go
// Runner executes `tmux <args...>` and returns stdout.
type Runner interface {
    Output(args ...string) ([]byte, error)
}

// RealRunner shells out to the tmux binary on PATH.
type RealRunner struct{}

// Proc reads process relationships; RealProc walks /proc.
type Proc interface {
    Children(pid int) []int   // direct children
    Comm(pid int) string      // process name, e.g. "claude"
    Cwd(pid int) string       // "" if unreadable
}

type RealProc struct{}

type Pane struct {
    Target  string // "dev:1.0" (session:window.pane)
    ID      string // "%5"
    PID     int
    Path    string // pane_current_path
    Command string // pane_current_command
}

// ListPanes runs: list-panes -a -F "#{session_name}:#{window_index}.#{pane_index}\t#{pane_id}\t#{pane_pid}\t#{pane_current_path}\t#{pane_current_command}"
// A tmux error (no server) returns (nil, nil) — a machine without tmux is normal.
func ListPanes(r Runner) ([]Pane, error)

// ClaudeCwd walks the descendants of pid (breadth-first, depth ≤ 6) for the
// first process whose Comm is "claude" and returns its Cwd.
func ClaudeCwd(p Proc, pid int) (string, bool)

// MapSessions pairs sessions to panes: a session matches a pane whose claude
// descendant's cwd equals the session's CWD. When several sessions share a cwd,
// they are paired to matching panes in order (sessions by LastEventAt desc,
// panes by Target asc); leftovers stay unmapped. Returns sessionID → Pane.
func MapSessions(sessions []fleet.SessionInfo, panes []Pane, p Proc) map[string]Pane

// Capture runs: capture-pane -p -e -J -t <paneID>  and returns the raw
// ANSI-laden screen text.
func Capture(r Runner, paneID string) (string, error)

// Reveal focuses the pane in the user's tmux: select-window -t <session:window>
// then select-pane -t <paneID>.
func Reveal(r Runner, target, paneID string) error
```

## package ui — M1 additions

```go
// SetPanes gives the model the sessionID → tmuxop.Pane mapping (location line
// in fleet entries, mirror source, reveal target).
func (m *Model) SetPanes(panes map[string]tmuxop.Pane)

// SetTrail hands the model the selected session's trail for the right panel.
func (m *Model) SetTrail(tr journey.Trail)

// SetMirror hands the model the latest captured frame for the selected session
// ("" = no pane; the panel then shows the latest-activity fallback).
func (m *Model) SetMirror(frame string)
```

Layout: width ≥ 110 → three columns: fleet 30 | mirror flex (min 40) | trail 38.
Width < 110 → fleet 30 | trail flex (mirror hidden). Trail panel per SPEC §2.1
mockups: newest on top; glyphs `◉` prompt, `◆` closed leg, `●` open leg (HEAD),
`◈` branch (`├─◈` fork off the rail, label + `⋯`/`✓`); relative times
right-aligned dim; class colors per SPEC §4 (monochrome-legible without them).
Renderer contract (for goldens): `RenderTrail(tr journey.Trail, now time.Time,
width, height int) string` exported from `internal/ui`.

Mirror: header `⌁ <target> · live` (dim), content = captured frame, each line
width-cropped ANSI-safely (`charmbracelet/x/ansi`, already in the dep tree),
bottom-aligned to the panel (a terminal's action is at the bottom). Fallback
(no pane): header `⌁ no pane · from transcript`, body = session Activity +
Reason + title, dim.

Keys: `Enter` on a mapped session → `tmuxop.Reveal`; `g` → select oldest
needs-you AND reveal if mapped. `tmux_actions=readonly` (config M4; for M1 a
`-readonly` CLI flag) disables both writes. Tab still shows the zoom notice
(Lv2 is M2).

App wiring: capture ticks at 200ms for the selected session only when a pane is
mapped; pane re-listing every 5s; trail rebuilt only when its session's tailer
delivered events. cmd/compass gains `-readonly`.

## Test contract (T-numbers continue from M0)

| # | Scenario | Expects |
|---|----------|---------|
| T19 | Classify: full vote table | every row above, incl. Bash first-word vs substring rules |
| T20 | scout→build transition with 2 stray votes then 3 consecutive | hysteresis: stray folds, 3rd consecutive splits, Start = first of the three |
| T21 | strong vote (Test) mid-Build | immediate split |
| T22 | Fix upgrade (a): error result during Build leg | whole leg Class=Fix |
| T23 | Fix upgrade (b): failing Test leg then Build leg | second leg Class=Fix |
| T24 | prompts: two user prompts | both in Prompts, leg closed at 2nd |
| T25 | branch: Agent tool_use then its result | Branch Done, AfterLeg correct, leg NOT split |
| T26 | Label heuristics: files / pytest / git push / none | per rule 7 |
| T27 | ListPanes: fake runner output incl. paths with spaces | parsed Panes; tmux error → (nil,nil) |
| T28 | ClaudeCwd: fake proc tree, claude 2 levels deep | found; absent → false |
| T29 | MapSessions: 2 sessions same cwd, 2 panes; 1 unmatched session | deterministic pairing per contract |
| T30 | Capture/Reveal: fake runner records exact args | arg vectors match contract strings |
| T31 | RenderTrail golden: the SPEC mockup trail (prompt, 3 legs, 1 branch, HEAD) at 38×20 | golden file |
| T32 | deck golden 120×30: three columns with fake mirror ANSI content | golden; mirror cropped safely |
| T33 | deck golden 80×24 unchanged shape: mirror hidden | golden (fleet+trail) |
| T34 | mirror fallback (no pane) | "from transcript" body |

Goldens under `testdata/golden/`, ASCII profile forced as in M0. All offline.

## Addendum — resolved during implementation

- **tmux sanitizes tabs for detached clients** (verified on tmux 3.4): command
  output for a client not attached to a session has `\t` rewritten to `_`, so the
  contract's tab-separated `list-panes` rows arrive tab-less in compass's primary
  deployment (a terminal tab outside tmux). The `-F` string stays as specified;
  `parsePane` first parses tab rows, then falls back to an anchored regexp
  (`^(.+?)_(%\d+)_(\d+)_(.*)_([^_]*)$`) keyed on the `%id` and numeric-pid fields.
  Target/ID/PID are exact in both paths; only an underscore inside a *command*
  name can smudge the Path/Command split in the fallback, which nothing
  load-bearing reads.
- **UI resolutions**: at 110–115 cols the mirror takes the remainder below its
  40-col preference (30+38+gutters leave 34; the floor holds from 116 up).
  `RenderTrail` renders the bare graph; the `TRAIL · <name> [Lv1]` title is the
  deck column's. A branch hangs directly under its fork node, replacing the `│`
  connector; `AfterLeg == -1` branches sit at the rail's foot; `⋯`/`✓` share the
  age column. The M0 detail card is retired — its content lives on as the
  mirror's no-pane fallback. Dim `FLEET` header added per the §2.5 mockup. Tab
  and action feedback share one dim footer note channel, cleared on keypress.
  The mirror never paints a frame for an unmapped or newly selected session.
- Pressure votes migrate (see the M1 test-reconciliation commit): a completed
  streak carries Start/Votes/Files into the leg it opens; a dying streak
  settles into the interrupted leg; `Trail()` displays an in-progress streak as
  part of the open leg. Labels are lowercase; `Files` keep verbatim basenames.
- `ListPanes` treats any Runner error as no-tmux → `(nil, nil)` (missing binary
  included). `ClaudeCwd` with an unreadable cwd → `("", false)`; pid itself is
  never a candidate (depth 1–6 = descendants). `MapSessions`: `LastEventAt` ties
  break on ID asc; empty-CWD sessions skipped; panes without a claude descendant
  are never candidates; returns a non-nil map. `Reveal` on a dotless target uses
  it as the window target directly.
