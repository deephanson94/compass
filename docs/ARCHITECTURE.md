# compass — Technical Design

> Status: draft v0.1 · companion to [SPEC.md](SPEC.md)

## 1. The big bet: observe, don't own

There are two ways to build "a panel next to your agents":

- **Own the terminals** (herdr's approach): be a multiplexer, hold every agent's PTY,
  detect state by pattern-matching the screen tail. Powerful, but you've rebuilt tmux,
  you're screen-scraping semantics out of pixels, and users must move their whole
  workflow into your world.
- **Observe the record** (compass): Claude Code already writes a complete, structured,
  live-appended record of everything it does. Read that. Let tmux keep owning
  terminals — the user already lives there.

Compass is a **read-only observer and pure tmux consumer**. Consequences:

- Works with sessions started *anywhere* (another terminal, `claude` in a bare shell,
  even Claude Code on the web synced later) — if it wrote a transcript, it has a trail.
- Zero risk: compass can't crash your session, eat your keystrokes, or corrupt a PTY.
- Semantic depth screen-scraping can't reach: exact tool calls, arguments, results,
  subagent forks, todo state, git branch, timestamps — already parsed, already typed.

## 2. Data sources (verified against Claude Code v2.x on 2026-08-30)

### 2.1 Main transcript — the trail's spine

`~/.claude/projects/<cwd-slug>/<session-uuid>.jsonl` — one JSON object per line,
appended live during the turn (not batched at turn end). Relevant line types:

| `type` | Carries |
|--------|---------|
| `user` | the human's prompt; also tool results returned to the model (`message.content[].tool_result`) |
| `assistant` | model output: `message.content[]` blocks — `text`, `thinking`, `tool_use` (name + input) |
| `attachment` | environment snapshots (cwd, git branch, worktree state) |
| `queue-operation` | prompt enqueue/dequeue (user typed while Claude worked) |

Every line has `uuid`, `parentUuid` (the chain), `timestamp`, `sessionId`, `cwd`,
`gitBranch`, `isSidechain`, `version`. The `parentUuid` chain is a literal DAG.

### 2.2 Subagent transcripts — the branches

`~/.claude/projects/<slug>/<session-uuid>/subagents/agent-<id>.jsonl` (+
`agent-<id>.meta.json`). Each subagent is a separate file with `isSidechain: true`
and its own chain. Fork point = the `tool_use` (Agent) in the main chain; merge
point = its tool result. **The git-graph is not a metaphor — it's the on-disk shape.**

### 2.3 Todos — the ghost waypoints

`~/.claude/todos/<session>*.json` — Claude's own task list (pending / in_progress /
completed). This is the "adventure it's going to do": pending items render as `◌`
ghost nodes ahead of HEAD; in_progress maps to HEAD's label; completions animate a
ghost solidifying into the trail.

### 2.4 Screen tail — the needs-you tripwire (herdr's one trick, borrowed cheaply)

Permission prompts and interactive questions are UI-level; they may stall the
transcript without explaining themselves in it. For sessions compass has mapped to a
tmux pane (§4.1), poll `tmux capture-pane -p -t <pane> | tail -n 12` at ~1Hz *only
when* that session's transcript has gone quiet mid-turn, and match the prompt
patterns (`Do you want to…`, `❯ 1. Yes`, `Esc to interrupt`, spinner glyphs). No PTY
ownership, ~zero cost, and it's the difference between "amber within a second" and
"amber after a timeout guess". Transcript remains the source of truth for everything
else; capture-pane is a tripwire, not a parser.

Fallback ladder for sessions with no mappable pane: transcript-quiet heuristics
(last event shape + mtime age) → optional `Notification`/`Stop` hooks the user can
install later for perfect signals. Hooks are an enhancement, never a requirement.

## 3. Engine pipeline

```
 ~/.claude/projects/**.jsonl ─┐
 ~/.claude/todos/*.json ──────┤  fsnotify
 tmux capture-pane (tripwire) ┘     │
                                    ▼
                              ┌──────────┐   append-only, per-session
                              │  tailer  │──► event stream (typed structs)
                              └──────────┘
                                    ▼
                              ┌──────────┐   pure function: events → legs
                              │segmenter │──► Lv1 legs + Lv2 waypoints
                              └──────────┘   (deterministic, offline, instant)
                                    ▼
                              ┌──────────┐   async, batched, budget-capped
                              │ narrator │──► leg/waypoint labels via
                              └──────────┘   `claude -p --model haiku`
                                    ▼
                              ┌──────────┐
                              │  store   │──► in-mem model + label cache
                              └──────────┘   (~/.local/share/compass/cache)
                                    ▼
                                 renderer (the panel)
```

### 3.1 Segmenter (the heart — and it's just code)

A streaming classifier: each event gets a class vote (tool name, Bash command regex,
file extension, error strings in results); a small state machine merges votes into
legs with hysteresis (don't flip class on one stray Read during a BUILD leg; do
split when the dominant class shifts for >N events or a strong boundary appears —
plan-mode exit, test run, git commit, subagent spawn, new user prompt).

Waypoint extractors are per-class plugins over a leg's events:

- `TEST`: parse runner output in tool results — pytest/jest/vitest/go test/cargo
  test summaries → counts + failing test names. (Regex table, unit-tested against
  fixture outputs.)
- `FIX`: cluster edit→run→error cycles; one waypoint per distinct error signature.
- `SHIP`: commit subjects from `git commit` results, PR URLs from tool results.
- `SCOUT`: files/dirs read, grouped; subagent reports.
- Subagent nodes: `meta.json` + final text block of the sidechain = the merge label.

Everything above ships in v1 with **zero AI calls** — the panel is fully functional
offline (heuristic labels like `fix · middleware.py` instead of prose).

### 3.2 Narrator (the polish — through the user's own auth)

Labels like *"bug2: expiry compared in local time, not UTC"* need a model. Key move:
**shell out to `claude -p` headless with `--model haiku`** instead of talking to any
API directly. It inherits whatever auth the user already has — subscription OAuth or
Bedrock — so compass needs no keys, no config, and no billing surface of its own.

- Batched: one call summarizes all unlabeled legs/waypoints of a session (send the
  leg's condensed event digest, get back short labels as JSON).
- Cached by leg-boundary uuid — a leg is narrated once, ever.
- **On by default, no hard cap** (SPEC §7): Haiku is cheap, and batching + caching
  already bound the call volume to roughly one call per leg boundary. `narrator =
  "off"` in config for anyone who disagrees. Panel shows heuristic labels until
  narration lands, then upgrades in place.

### 3.3 State machine (per session)

```
            transcript growing
   ┌────────────────────────────► WORKING ◄──────────┐
   │                                │                │
   │            quiet mid-turn +    │ quiet mid-turn │ transcript
   │            prompt pattern on   │ >90s, no       │ resumes
   ▼            screen tail         ▼ pattern        │
 IDLE ◄── turn completed ── NEEDS-YOU        STUCK ──┘
   (last assistant msg shown)   (age counter, sort-to-top, optional bell)
```

`turn completed` = final assistant text with no pending tool_use (plus `Stop` hook
when installed). A completed turn *ending in a question* also lands in NEEDS-YOU.

## 4. tmux — consumed, never managed

Design ruling (SPEC §7): compass creates no tmux sessions, windows, or panes for its
own layout. It runs standalone in its own terminal tab (deck mode) or inside a pane
*the user* made (sidecar mode), and talks to the tmux server only as a client:
reads freely, writes only the two keypress-gated actions below. All tmux client
commands work from outside tmux — they hit the server socket, so a compass tab that
isn't "in" tmux still sees all five of your tmux sessions.

### 4.1 Pane discovery: mapping Claude sessions ↔ tmux panes

Sessions are discovered from transcripts (not tmux), then *located*:

1. `tmux list-panes -a -F '#{session_name}:#{window_index}.#{pane_index} #{pane_id}
   #{pane_pid} #{pane_current_path} #{pane_current_command}'` — every pane on the
   server, refreshed lazily (a few seconds' staleness is fine for location labels).
2. For each pane, walk `/proc` descendants of `pane_pid` looking for a `claude`
   process; read its `/proc/<pid>/cwd`.
3. Match cwd → project slug → the transcript file in that slug whose mtime says
   "this one is live". Ambiguity (two sessions, same repo, same pane? impossible;
   same repo, two panes) resolves by process start time vs session start.

Result per session: `dev:1.0` shown in the fleet, a target for reveal and the
capture-pane tripwire. Sessions with no pane (bare shell, other machine, exited CLI
with `--resume` available) still get full trails — just no location line and no
tripwire.

### 4.2 The two write actions (both opt-outable: `tmux_actions = "readonly"`)

- **Reveal** (`Enter`/`g`): `tmux select-window -t dev:1` + `select-pane -t %5` in
  the session's own tmux session. compass changes *which pane is focused where you
  already work*; it never moves, resizes, or creates anything.
- **Ask the trail** (`a`): `tmux new-window -t dev: -n 'ask:api' claude
  --append-system-prompt <historian preamble>` … pointing the historian at the
  transcript path to read. The answer engine is Claude Code itself — same auth, same
  UI. The window belongs to the user's tmux session; they kill it like any window.
  No mappable tmux target → compass prints the exact command to copy instead.

### 4.3 Outbound cues (no bells — SPEC §7)

- **Tab title**: on every state change, emit `OSC 2` (`⌂ compass ▲2`) so the
  terminal tab shows fleet health while unfocused. Works over SSH; Windows
  Terminal renders it on the tab.
- **`compass status`**: one-shot subcommand printing `●3 ▲1` for the user to embed
  in their own tmux `status-right` via `#(compass status)`. Reads the same state
  the panel does (cheap: transcript mtimes + last-line peek; no full parse).
  compass never edits `.tmux.conf`.

## 5. Stack

**Go + bubbletea + lipgloss** (charmbracelet stack).

| Considered | Verdict |
|-----------|---------|
| Rust + ratatui | Herdr's choice; excellent, but we don't need PTY performance (we own no PTYs) and iteration speed wins the design phase. Revisit only if render perf disappoints. |
| TS + Ink | Same stack as Claude Code itself, but weakest TUI perf of the three and heaviest runtime; our AI calls go through the `claude` binary anyway, so no SDK pull. |
| Go + bubbletea | Single static binary, `fsnotify` tailing, `exec` for tmux/claude, lipgloss is *built* for the Apple-restraint aesthetic, richest TUI component ecosystem (bubbles). **Chosen.** |

Supporting choices: no daemon in v1 (the panel process is the watcher; state
rebuilds from disk in <1s on start, so nothing is lost by quitting); label cache as
a plain JSONL file; config at `~/.config/compass/config.toml` (zero-config default).

## 6. Performance & privacy

- Transcripts can reach tens of MB: tail incrementally (remember offsets), parse
  only appended bytes; full-file parse only on first sight, in a goroutine per file.
- Fleet scan = one directory walk of `~/.claude/projects` at start + fsnotify after.
- Render at most 15fps; idle panel draws only on data/tick (battery-polite).
- Read-only everywhere except its own cache dir. No network calls except through
  the user's `claude` binary. No telemetry, ever.

## 7. Milestones

| | Deliverable | Proves |
|--|-------------|--------|
| **M0** | tailer + state machine + fleet strip (no graph, no AI) | state detection is trustworthy — the amber dot is never wrong |
| **M1** | segmenter + Lv1 trail graph + pane discovery + reveal/`g` | the journey renders and reads at a glance, offline |
| **M2** | Lv2 waypoints (test parser, fix clusters, subagent branches) + ghost todos | Tab-zoom feels like focus, not navigation |
| **M3** | narrator (haiku labels) + Lv3 reader + ask-the-trail | the panel becomes conversational |
| **M4** | polish: breathing HEAD, themes, bell policy, config, docs, demo GIF | ship it |

M0 is deliberately boring: if compass only ever shipped the fleet strip with a
correct amber dot, it would already earn its screen columns.

## 8. Risks

| Risk | Mitigation |
|------|-----------|
| Transcript format is undocumented and shifts between Claude Code versions | version field is on every line; adapter layer with per-version quirks; fixtures recorded per CC release; fail-soft (unknown lines are skipped, trail degrades, never crashes) |
| Permission prompts invisible in transcript | capture-pane tripwire (§2.4) + optional hooks |
| Narrator cost surprises | batching + per-leg caching bound call volume structurally; `narrator = "off"` exists; heuristic labels are always complete |
| Panel too narrow for Lv2 richness | Lv3 widening pattern proven in tmux; truncation rules in SPEC §4 |
| herdr ships our roadmap | different bet (observe vs own); our moat is trail semantics from transcripts, which screen-scraping cannot reach |
