# compass 🧭

**See where every Claude session has been, where it is, and where it's headed — without leaving the terminal.**

You run five Claude Code sessions at once. One is quietly refactoring, one is stuck
waiting for a permission you didn't see, one finished twenty minutes ago, and two are…
doing something. `compass` is the sidecar panel that answers, at a glance:

- **What journey did it take?** — a git-graph-style *trail* of the session's work
- **What is it doing right now?** — live state: working / needs you / idle / stuck
- **What adventure is next?** — Claude's own plan, rendered as ghost waypoints ahead

Your sessions stay exactly where they are — **real, untouched Claude Code CLIs in
your own tmux**. compass runs in its own terminal tab (or a tmux window of its own)
and watches all of them at once.

```
┌ compass ──────────────────────────────────────────────────────────────────────────────────┐
│ FLEET · live          │ ⌁ dev:1.0 · live                 │ TRAIL · api             [Lv1]  │
│ dev                   │                                  │ ◉ "fix the 401 bug"       38m  │
│▸1 ● api    fixing  3m │  ● I'll fix the token refresh    │ ╷                              │
│    :1.0 · auth-fx     │    bug. Let me look at the       │ ◆ scout  auth module map  31m  │
│ 2 ● webapp 18✓ 2✗     │    middleware first…             │ │                              │
│    :2.1 · main        │                                  │ ◆ build  refresh middlware 25m │
│ ops                 ▲ │  ⏺ Read(src/auth/middleware.py)  │ ├─◈ agent scouted payments     │
│ 3 ▲ infra  needs you! │  ⏺ Bash(pytest tests/auth -x)    │ ◆ test   pytest 18✓ 2✗    12m  │
│    :0.0 · tf/vpc      │    ⎿ 18 passed, 2 failed…        │ │                              │
│ elsewhere             │                                  │ ● fix    token refresh   ← 3m  │
│ 4 ○ docs   idle   22m │  ✻ Churning… (23s · esc to       │ ┊                              │
│    no pane · main     │    interrupt)                    │ ◌ test   full suite            │
│                       │                                  │ ┊                              │
│ 268 archived · A      │  the real pane, mirrored live    │ ◌ ship   open PR               │
│ j/k move · enter attach (prefix d returns) · g grab · ? help · q quit                     │
└───────────────────────────────────────────────────────────────────────────────────────────┘
```

On a wide terminal compass opens on **the board**: every session's trail side by
side, urgent ones first, each bright while it has something you haven't read and
dim once it's history. Under each name is the session as it stands: what it is
doing this minute and for how long (`● build  wiring the filter    for 1h`, with
`◈3 out` when it has agents out), the question it is asking you, the call it is
hung on, or — once it has gone quiet — how it came out (`✓ shipped 4m ago`,
`✗ red 18✓ 2✗ · shipped on red`). A trail longer than its column is drawn
without the air between legs, with the hour on the rail where it turns. `Tab`
opens one trail; `Shift+Tab` comes back. The deck
above is that one trail with the **live mirror** switched on (`m`): the selected
session's actual tmux pane, streamed read-only via `capture-pane` — you watch the
real CLI render, but compass owns no PTY. When you want to *type*, `Enter` hands you the terminal:
outside tmux compass suspends and attaches, so the pane is a real PTY with a real
keyboard and your own prefix + `d` brings the deck back; inside tmux your client
just switches. `g` grabs whichever session has been waiting on you longest and
attaches to it. Nothing to manage: compass never creates or owns tmux sessions,
windows, or panes.

The trail on the right reads like the conversation does — oldest at the top, the
newest work at the bottom, and it stays pinned there so the latest is always on
screen. `Tab` (Lv2) unfolds each leg; `Tab` again (Lv3) opens the conversation
itself, anchored to whatever trail row your cursor is on: the trail is a minimap,
the transcript is the code.

## Principles

1. **The CLI is sacred, and so is your tmux.** Sessions are the real `claude` binary
   in panes you own. compass never wraps, proxies, or re-renders the CLI, and never
   creates or manages tmux sessions — it only observes, plus one keypress-gated
   action: `Enter`, which hands you the session's own terminal.
2. **Three keypresses, max.** Any session, any zoom level, any answer — reachable in
   ≤3 keypresses from anywhere. This is a hard constraint, tested in CI.
3. **Zero config, read-only.** compass watches the JSONL transcripts Claude Code
   already writes (`~/.claude/projects/…`). No hooks required, no API keys, nothing
   installed into your sessions. Delete compass and nothing changes.
4. **Heuristics first, AI second.** The trail renders instantly and offline from
   deterministic classification. A Haiku *narrator* (through your existing `claude`
   auth — subscription or Bedrock) enriches labels in the background — on by
   default, batched and cached so each leg is narrated exactly once.
5. **Useful first, beautiful always.** Calm, quiet, Apple-grade restraint. Color and
   glyphs reinforce meaning but never carry it alone — everything reads in pure
   monochrome. Nothing blinks, and nothing rings a bell: attention is visual (amber
   sort, tab-title badge), which is exactly what survives an SSH hop.

## The three zoom levels

| Level | One Tab away | Shows |
|-------|--------------|-------|
| **Lv1 — Trail** | default | The journey as a git graph: scout → build → test → fix, subagents as branches, plan as ghost nodes |
| **Lv2 — Waypoints** | `Tab` | Legs expanded: each bug, each test run (18✓ 2✗), files touched, commits, subagent findings |
| **Lv3 — Deep dive** | `Tab` `Tab` | The reader takes focus: scroll, unfold tool output, search. `a` at any level hands you **ask the trail** — a Claude grounded in this session's full history |

## Using it

```
go build -o compass ./cmd/compass    # Go 1.24+; a single static binary

compass                              # the deck, full screen — run it in its own terminal tab
compass -readonly                    # observe only: Enter no longer attaches
compass -narrator off                # heuristic labels only, no claude calls
compass -live-within 0               # only sessions tmux is holding count as live
compass status                       # one-shot fleet summary, e.g. "▲1 ●2 ○1"
compass panes                        # diagnostic: which pane holds which session
```

### Beside a session, not instead of it

`Enter` hands you a session's terminal, but then the deck is gone. To keep the
panels on screen while you type, put compass in a pane of your own tmux — it is
a consumer of your multiplexer, so this is just a split:

```
tmux split-window -h -l 70 'compass'    # fleet + trail beside the CLI
tmux split-window -h -l 46 'compass'    # the trail alone, for a narrow strip
```

compass fits itself to the pane. Below 110 columns there is no board and no
mirror — the real CLI is right there — and it shows the fleet beside the trail. Below 62 it shows the trail alone: the header keeps the
fleet's alarm (`▲2 ●1 ○3`) and the trail's title names whichever session `j`/`k`
has landed on.

One keypress, in your own `.tmux.conf`:

```
bind C-g split-window -h -l 70 'compass'
```

Everything is optional configuration — compass runs with none. `~/.config/compass/config.toml`:

```toml
root = "~/.claude"      # the Claude home to observe ($COMPASS_ROOT and -root override)
narrator = "haiku"      # narration model; "off" disables
readonly = false        # true keeps compass's hands off tmux entirely
live_within = "5m"      # a paneless session counts as live this long; "0" = tmux only
```

For the fleet summary in every tmux session, add to your own `.tmux.conf`:

```
set -g status-right '#(compass status) · %H:%M'
```

`compass status` is a fresh process every time tmux draws the bar, so it keeps
a small cache (`~/.cache/compass/resume.json`) of where it had read to in each
live transcript and what it had concluded there. Without it every invocation
re-reads every live session from byte zero. Deleting the file costs one slow
run and nothing else.

### Keys

| Key | |
|-----|---|
| `1`–`9` | select a session |
| `Enter` | go to it: compass hands you the session's terminal (prefix + `d` returns) |
| `Tab` / `Shift+Tab` | zoom: trail → waypoints → the conversation itself |
| `j`/`k` | move — the fleet at Lv1, the trail's rows at Lv2 (the conversation follows), the reader at Lv3 |
| `g` | grab the session that has waited on you longest, and go to it |
| `A` | browse the archive: every past session, grouped by project |
| `a` | ask the trail: a historian `claude` takes the terminal, briefed on this session's transcript; exit returns |
| `ctrl+d`/`ctrl+u` | half a page: the trail at Lv1, the reader at Lv3 |
| `G` | back to the present — the newest row, at any level |
| `[` / `]` | previous / next prompt — the chapters of a trail, at Lv1 and Lv2 |
| `Space` `/` `n`/`N` | Lv3: unfold a result · search · walk the matches |
| `?` | help |

## Design docs

- [`docs/SPEC.md`](docs/SPEC.md) — product spec: UX model, keymap, states, visual language, decision log
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — technical design: data sources, engine, stack, milestones
- [`docs/dev/`](docs/dev/) — the per-milestone API contracts the code and tests were built against
