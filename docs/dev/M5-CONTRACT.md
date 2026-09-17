# M5 Contract — the honest fleet: liveness, tmux grouping, the archive

Binding API contract, same rules as M0–M3: implementation and tests are written
in parallel against this document; deviations amend it first.

Field finding (first real dogfood): `~/.claude/projects/` is an archive — 280+
transcripts against ~15 actually-running sessions — and the fleet presented the
archive as the fleet. M5 makes the fleet mean *something can need you*, groups
it the way the user thinks (tmux session → window), and turns the archive into
a browsable feature instead of noise. Decisions (user, 2026-08-31): live =
"tmux has it"; archive grouped by project directory ("where matters — I start
sessions in the respective directory").

## package fleet — liveness

```go
type Session struct {
    Info SessionInfo
    Snap state.Snapshot
    Live bool
}

// MarkPaneMapped tells the manager which sessions currently sit in a tmux pane
// (the ui feeds this after every MapSessions). The zero state — never called,
// or an empty map — means no panes are known.
func (m *Manager) MarkPaneMapped(ids map[string]bool)

// SetLiveWindow sets the recency door: a session with no pane still counts as
// live while now−LastEventAt ≤ d. Default 5 minutes; 0 closes the door (panes
// only) — and with it the ask door below. The door exists because pane matching
// is a heuristic — a session that is WRITING ITS TRANSCRIPT RIGHT NOW must never
// be hidden by a matching miss.
func (m *Manager) SetLiveWindow(d time.Duration)
```

Rules:
1. **live** = pane-mapped ∪ (LastEventAt within the live window) ∪ **asked**.
   Everything else is **archived**.

   **The ask door (#344, 2026-09-15).** A session whose transcript's last word
   is the model's, asking you something nobody has answered, is live however
   long ago it asked. `SessionInfo.Asked` is read off the file in the same tail
   peek that dates it, so it survives the pane closing and compass restarting —
   which is the point, since a question you forgot is one you were not
   watching. It reads the file the way the machine reads the fold: a person's
   words settle it (nothing waits), a call still out is work in flight unless
   it is `AskUserQuestion`, and otherwise rule 4's own test decides. The
   harness's own turns settle nothing, and a refused call is not a question.
   The door clears itself — reply, and the last word is yours; and it closes
   when the session does. A question is read off the file, so it outlives the
   pane and compass's own restart, but `/exit` writes nothing to the file and
   a question in a session nobody is running is one you already answered by
   leaving. `<root>/sessions/<pid>.json` is Claude Code's own registry of
   running sessions and says which those are; where it cannot be read at all
   the door is what it was, because "I cannot say" must not archive anything
   (#374).

   The walk reads a file that has gone quiet whole, and gives one window to
   a file still being written — that one is live on the recency door whatever
   the walk decides (#345). A message decides as a whole, not by its first
   block: an unanswered `AskUserQuestion` anywhere in it opens the door, any
   other call still out is work in flight, and a turn whose calls all came
   back is one the model is still in the middle of. A subagent's lines never
   open the door and any of them closes it, which is how the machine reads
   them too.

   **A message is not a line, and its lines are not always adjacent.** Claude
   Code writes one content block per line and repeats `message.id` across
   them, and on a turn that calls several times it writes each call's result
   between the calls. So "the whole message" is every line carrying that id,
   `user` lines in the gaps and all: the walk holds a turn it is inside until
   a line of a different message arrives or the file runs out, and a bare
   `tool_result` line records the call it answers without ending anything
   (#373). A turn whose calls all came back is read to its start for the same
   reason, because the question may be in an earlier line of it. "The file
   runs out" is the file, not the window: the walk reads backwards in 64KB
   windows, a hold that outlives one is what tells it to widen, and each
   line is walked exactly once however many windows it takes — re-reading a
   line under a hold it was first read without changes the answer, so the
   verdict must not depend on where a boundary falls. A subagent writing in
   the lead's own file does not refuse the hold either: a question the
   harness is holding open is rule 2, an agent in flight is rule 3, and
   refusing it made the guarantee depend on which block of a turn the
   harness wrote first. A question further back than sixteen windows is not
   found, and the session archives; a single line that long is outside what
   this walk reads.

   **Where a subagent's lines live.** Discovery reads one project directory
   deep and skips every entry that is a directory, so the transcripts it opens
   are `projects/<slug>/<id>.jsonl` and never `<id>/subagents/agent-*.jsonl`.
   A lead whose agents are doing all the writing is therefore dated by an
   mtime nobody refreshes, and `isSidechain` lines in the lead's own file —
   which the rule above is written for — are rarer than the rule suggests.

   **`Waiting` implies needs-you, and it is enforced rather than hoped for**
   (#346): the walk reads the end of a file where the machine folds all of
   it, so where the two differ the machine wins — the session goes back to
   the archive and the door does not knock again until the transcript grows.
   A door the fold refused is remembered on the entry, not re-litigated
   every second.

   Such a session is flagged `Waiting` when nothing else would have kept it:
   no pane, outside the window. `Waiting` is the fleet's word for "you walked
   away from this one", and it sorts under everything happening today — the
   live alarms and the work in flight — and over what is merely idle (#345:
   an archive where one session in four ends on a question would otherwise
   take every column the board has). `StatusLine` leaves it out: the bar
   answers "is anything happening", and a question from last week is not. The
   board ranks it the same way, says `waiting 9d` on its row, counts it in a
   chip of its own rather than among the alarms, lets `x` put it down for
   good, and sweeps the pile with `X` (docs/SPEC.md #344, #345).

   The other store answers the same question of its own rows
   (`internal/opencode/asked.go`): one fleet, one rule, or the word means
   two things in one list. It reads what that store's own reader reads — a
   part left `pending` is a call the reader skips, so the door skips it too
   (#346).
2. Only live sessions are tailed and state-machined per Refresh. An archived
   session's Snap is always `{Idle, Since: LastEventAt, Reason: "archived",
   Activity: "idle"}` — the archive can never be amber, so `g` and the
   attention chips stay truthful by construction. A session holding a question
   open is amber because it is *live*, not because the archive grew a state:
   it is tailed and state-machined like any other live session, and the amber
   is its machine's own verdict.
3. A session crossing archive→live (its pane appears, or its file grows) gets
   a tailer from scratch (full replay, as any first sight); live→archive drops
   its tailer and machine.
4. Refresh returns live sessions first (today's order: needs-you longest-wait,
   stuck, working, idle) and then archived sessions, newest LastEventAt first.
5. `StatusLine` counts LIVE sessions only; "○ all quiet" when no live session
   is working or waiting.

### Discovery caching (the 280-file perf fix)

Discover currently re-reads a bounded head+tail of every transcript on every
call — ~1s cadence × 280 files. Add a cache keyed by path: when a file's
(size, mtime) is unchanged, reuse the previous SessionInfo without opening it.
The cache lives on the Manager (Discover the free function stays uncached —
its one-shot callers don't loop).

## package tmuxop — pane order

`MapSessions` keeps its map. The ui additionally needs tmux's own ordering
(group order = the order `ctrl-b s` shows), which `list-panes -a` already
emits: session by session, windows and panes in index order. No new tmux
calls; the ui simply keeps the `[]Pane` slice it already gets from `ListPanes`
alongside the map.

## package ui — the grouped fleet and the archive

Two fleet views, toggled by `A` (global key, any level, not while searching):

**Live view (default).** Sessions grouped by tmux session name; groups in the
order the panes list first mentions them; inside a group, window.pane index
order (parse the numerics from `Pane.Target`), with one exception — a
needs-you or stuck session floats to the top of ITS group (never out of it).
Live-but-unmapped sessions form a final group `elsewhere`. When `elsewhere`
would be the only group (no tmux at all — a container, a bare server), its
header is dropped: a degenerate tree is a list.

```
 FLEET · live                    │
 dev                             │
▸1 ● api      fixing auth     3m │
    :1.0 · claude/auth-fx        │
 2 ● webapp   tests 18✓ 2✗   40s │
    :2.1 · main                  │
 ops                          ▲  │
 3 ▲ infra    needs you       2m │
    :0.0 · tf/vpc                │
 elsewhere                       │
 4 ○ scratch  idle           22m │
    no pane · main               │
                                 │
 268 archived · A browses        │
```

- Group headers: dim, unnumbered, unselectable; a group containing a needs-you
  or stuck session carries a right-aligned `▲`/`◍` echo.
- The location line drops the now-redundant session prefix: `:1.0 · <branch>`
  mapped, `no pane · <branch>` in elsewhere.
- Numbering `1`–`9` runs flat down the rendered session order, groups ignored;
  `j`/`k` skips headers. `g` is unchanged (and cannot land on the archive —
  rule 2 above).
- The last fleet row is the dim archive count: `N archived · A browses`
  (absent when N is 0).

**Archive view (`A`).** Same column, headed `FLEET · archive`. Grouped by
project — the last segment of `Info.CWD` (fall back to the slug), groups
ordered by their newest member, newest first inside. Entries are one line +
dim branch line, `○` glyph, age = LastEventAt. Everything works there — trail,
Tab zoom, reader, `a` — reading an old journey is the point. `A` returns to
live; the footer keymap says so. Selection is remembered per view.

**Fleet scrolling (both views).** The column follows the selection: moving
below the last visible row scrolls the list, headers scroll with their group,
the selected session's two rows are always fully visible. (The live view
usually fits; the archive never does.)

Three-keypress audit: any live session `1`–`9` (1) · unblock `g` (1) · browse
an old journey `A` then aim (1 depth) · back `A` (1).

## cmd/compass

`-live-within` flag (default `5m`, `0` = panes only) and `live_within` in the
config file, threaded to `SetLiveWindow`.

## Test contract (T-numbers continue)

| # | Scenario | Expects |
|---|----------|---------|
| T54 | Manager partition: pane-mapped stale session → live; unmapped fresh (within window) → live; unmapped old → archived; Live flags; order live-block then archive newest-first | fleet rules 1, 4 |
| T55 | Archived Snap is always the archived-idle snapshot even when its transcript's last shape would read NeedsYou; crossing archive→live re-reads and the real state appears; StatusLine ignores the archive | fleet rules 2, 3, 5 |
| T56 | Discovery cache: Refresh twice, unchanged files → identical results; touch one file with new content → only its info changes; mtime/size key respected | caching section |
| T57 | SetLiveWindow(0) → panes only; default window admits a fresh unmapped session | fleet rule 1 |
| T58 | Live-view golden 120×30: two groups + elsewhere + archive count line, amber float inside its group, header echo, `:w.p` location lines | ui section |
| T59 | Archive-view golden: `A` toggle, project groups newest-first, `FLEET · archive` header, footer hint; toggle back restores live selection | ui section |
| T60 | Fleet scrolling: selection walked past the fold keeps the selected pair visible; headers travel with groups | ui section |
| T61 | Group order = first-occurrence order of the pane list; window.pane numeric order inside (10 > 9 proves numeric, not lexical) | ui section |

Goldens under `testdata/golden/`, ASCII profile forced. All offline.

## Addendum — resolved during implementation (pinned by tests)

- The recency door is **inclusive**: exactly `now − LastEventAt == d` is live.
- `SetLiveWindow` clamps a negative duration to 0 (panes only) — it is user
  input via `-live-within` and `live_within`.
- `MarkPaneMapped` copies; the Manager never retains the caller's map. A later
  call **replaces** (no union); nil or empty clears the mapping. Unknown ids
  are harmless.
- Archive ties (equal `LastEventAt`) break on ID ascending, matching Discover.
- A zero `LastEventAt` never holds the door open.
- Exclusion beats liveness: an excluded session is absent, not archived, even
  when pane-mapped.
- `sleep()` (live→archive) clears the tailer's event-beats-mtime latch, so an
  archived session's `LastEventAt` can subsequently be raised by file mtime
  until it wakes and replays; `LastEventAt` never moves backwards.
- The pre-M5 ordering and exclusion tests run behind a wide-open live window
  (`liveManager` helper) — liveness is not their subject; the StatusLine tests
  run STOCK with in-window fixtures, because they pin the shipped status line.
- Discovery caching is pinned behaviorally plus one deliberate white-box probe
  (same size+mtime, changed bytes → old info retained) that documents the
  (size, mtime) key; rewrite that probe if the key ever legitimately changes.
