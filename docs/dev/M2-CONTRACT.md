# M2 Contract — Lv2 waypoints, ghost todos, Tab zoom

Binding API contract for milestone M2, same rules as M0/M1: implementation and
tests are written in parallel against this document; deviations amend it first.

M2 goal (ARCHITECTURE §7): *Tab-zoom feels like focus, not navigation.* Legs
unfold their detail — parsed test results, bug signatures, commits, subagent
reports — and the trail gains its future: Claude's own todos as ghost waypoints
ahead of HEAD. Still offline, no AI calls.

## package transcript (already landed with this contract)

`ToolResult` gains `Text string`: the result's text content (plain string or
joined text blocks), clamped to 2048 bytes at each end with a `\n…\n` elision
line, cut on rune boundaries. Empty for non-text results.

## package journey — waypoints

```go
type WaypointKind int

const (
    WaypointTestRun  WaypointKind = iota // "18 passed · 2 failed"
    WaypointTestFail                     // one failing test's name
    WaypointBug                          // first line of a distinct error
    WaypointCommit                       // commit subject or PR URL
)

type Waypoint struct {
    Kind WaypointKind
    Text string // ≤60 runes, no glyph/prefix — the renderer decorates
    At   time.Time
}
```

`Leg` gains `Waypoints []Waypoint` (oldest first, **cap 8 per leg**, overflow
dropped silently). `Branch` gains `Report string` — the first non-empty line of
the Agent tool_result's Text, ≤60 runes, `""` until Done.

### Extraction rules

1. **Attachment**: a result's waypoints attach to the leg open when the result
   arrived; if none is open, the last leg; if no legs, they are dropped.
   Waypoints never migrate with pressure votes — they stay where they landed.
2. **Runner memory**: the segmenter remembers each Test/Ship vote's tool_use ID
   and matched keyword; only those results get test/ship parsing. All other
   results can contribute only Bug waypoints.
3. **Test parsing** on the result Text, first matching family wins:
   - *pytest*: summary counts (`N failed`, `N passed`, `N error(s)` on one
     line) → one `WaypointTestRun` `"N passed · M failed"` — zero parts
     omitted, `"N passed"` alone when nothing failed. Lines `FAILED <path>::<name>`
     → `WaypointTestFail` per test name (the `<name>` part), dedupe, cap 3.
   - *go test*: lines `--- FAIL: <TestName>` → `WaypointTestFail` each (cap 3,
     dedupe); `WaypointTestRun` is `"N failing"` when any, else `"ok"` when a
     line starts with `ok`.
   - *jest/vitest*: `Tests: … N failed … M passed …` → `"M passed · N failed"`;
     failing names from `✕ <name>` or `× <name>` lines, cap 3.
   - *cargo test*: `test result: ok|FAILED. N passed; M failed` → same compose;
     failing names from `test <name> ... FAILED`, cap 3.
   - Nothing matches: `IsError` → `WaypointTestRun` `"failed"`; clean → no
     waypoint.
4. **Bug**: an `IsError` result (any tool) attaching to a Build or Fix leg →
   `WaypointBug` with the first non-empty line of its Text (trimmed, ≤60
   runes). Dedupe by exact text per leg, cap 3 bugs per leg.
5. **Commit/PR** (Ship-voted results only): a line matching `[<ref> <hash>]`
   (git commit output) → `WaypointCommit` with the subject after `] `; a line
   containing `github.com/` and `/pull/` → `WaypointCommit` with the URL token.
6. **Branch.Report** per the type doc above (rule 2's memory covers Agent IDs).

## package todo (new)

```go
type Status string // "pending", "in_progress", "completed" — kept verbatim

type Item struct {
    Text   string
    Status Status
}

// Read loads the session's todo list: scans <root>/todos/ for *.json files
// whose name contains sessionID; when several match, the newest mtime wins.
// Items parse from a JSON array of objects — Text from "content" (fallback
// "activeForm"), Status from "status" — order preserved. Missing dir or no
// match → (nil, nil); a malformed file is skipped, not an error.
func Read(root, sessionID string) ([]Item, error)
```

## package ui — M2 additions

```go
// RenderTrail gains the plan and a zoom level (1 or 2). SIGNATURE CHANGE from
// M1 — the M1 call sites and goldens are updated with it.
func RenderTrail(tr journey.Trail, todos []todo.Item, now time.Time, width, height, level int) string

func (m *Model) SetTodos(items []todo.Item)
```

- **Ghosts (both levels)**: pending todos render above HEAD as `◌ <text>` rows
  with a dashed `┊` rail. The FIRST pending item is the next action and sits
  nearest HEAD; later plan items stack upward. At most 4 ghosts; more collapse
  into one dim `┊ +N more` row at the top. `in_progress`/`completed` items are
  not drawn (HEAD and the legs already tell that story).
- **Lv2**: each leg unfolds its waypoints beneath its node on the rail:
  `│  ├ <text>` rows, last one `│  └ <text>`. Renderer decoration: TestFail →
  `✗ ` prefix; Bug → numbered `bug1 `, `bug2 ` per leg in At order; TestRun and
  Commit undecorated. A Build/Fix/Docs leg with ≥2 Files appends a final
  synthetic row `touched <a> · <b> · <c>` (from Leg.Files, no extractor). A
  branch node at Lv2 shows its Report (if any) as its own `│  └ <report>` row.
  Height budget: trim waypoint rows before leg rows, HEAD's waypoints last.
- **Keys**: `Tab` zooms 1→2 (at 2: footer note "deep dive (Lv3) arrives in
  M3"); `Shift+Tab` zooms 2→1 (at 1: no-op). Trail title shows `[Lv1]`/`[Lv2]`.
- App wiring: `todo.Read` for the selected session on the 1s tick (root from
  `-root`), fed via `SetTodos`.

## Test contract (T-numbers continue)

| # | Scenario | Expects |
|---|----------|---------|
| T35 | ToolResult.Text: string content; block array; >4KB clamp with a multibyte rune at each cut; elision marker present | per transcript section |
| T36 | pytest: counts compose, zero parts omitted, FAILED names, dedupe, cap 3 | rule 3 |
| T37 | go test: FAIL names, "N failing", "ok"; only Test-voted results parsed | rules 2–3 |
| T38 | jest and cargo families; unmatched+IsError → "failed"; unmatched clean → none | rule 3 |
| T39 | Bug: first line, ≤60 runes, dedupe, cap 3, only on Build/Fix legs; Scout leg gets none | rule 4 |
| T40 | Commit subject and PR URL from Ship results; non-Ship results never Commit | rule 5 |
| T41 | Branch.Report: first non-empty line, ≤60, "" until Done | rule 6 |
| T42 | todo.Read: content/activeForm fallback, status verbatim, newest-mtime wins, malformed skipped, missing → (nil,nil) | todo section |
| T43 | RenderTrail Lv2 golden 38×24: legs with test counts, ✗ fails, numbered bugs, touched row, branch report | ui section |
| T44 | ghosts golden at Lv1: 2 pending todos above HEAD, first-pending nearest HEAD; and a 6-todo case collapsing to `+2 more` | ui section |

Waypoint cap/attachment edge tests (cap 8, attach to last leg when none open,
no-legs drop) fold into T36–T40. Goldens under `testdata/golden/`, ASCII
profile forced. All offline.
