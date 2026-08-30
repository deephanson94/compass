# compass 🧭

**See where every Claude session has been, where it is, and where it's headed — without leaving the terminal.**

You run five Claude Code sessions at once. One is quietly refactoring, one is stuck
waiting for a permission you didn't see, one finished twenty minutes ago, and two are…
doing something. `compass` is the sidecar panel that answers, at a glance:

- **What journey did it take?** — a git-graph-style *trail* of the session's work
- **What is it doing right now?** — live state: working / needs you / idle / stuck
- **What adventure is next?** — Claude's own plan, rendered as ghost waypoints ahead

Your sessions stay exactly where they are — **real, untouched Claude Code CLIs in
your own tmux**. compass runs in its own terminal tab and watches all of them at
once (a narrow `--sidecar` mode exists if you'd rather dock it inside a tmux pane).

```
┌ compass ────────────────────────────────────────────────────────────────┐
│  FLEET                        │  TRAIL · api                     [Lv1]  │
│  1 ● api      fixing auth  3m │  ┊                                      │
│      dev:1.0 · claude/auth-fx │  ◌ ship   open PR                       │
│  2 ● webapp   tests 18✓ 2✗    │  ◌ test   full suite                    │
│      dev:2.1 · main           │  ┊                                      │
│  3 ▲ infra    needs you (2m!) │  ● fix    token refresh          ← 3m   │
│      ops:0.0 · tf/vpc         │  │                                      │
│  4 ○ docs     idle        22m │  ◆ test   pytest 18✓ 2✗           12m   │
│      (no pane) · main         │  ├─◈ agent scouted payment flows        │
│  5 ◍ etl      stuck? (8m)     │  ◆ build  refresh middleware      25m   │
│      data:1.2 · feat/loader   │  │                                      │
│                               │  ◆ scout  auth module map         31m   │
│                               │  ╵                                      │
│                               │  ◉ "fix the 401 bug"              38m   │
│  Tab zoom · Enter reveal · a ask · g needs-you · ? help                 │
└─────────────────────────────────────────────────────────────────────────┘
```

Press `3` to see infra's trail; `Enter` reveals its pane in *your* tmux (`ops:0.0`),
already focused when you switch over. `g` grabs whichever session has been waiting
on you longest. Nothing to manage: compass is a pure consumer of tmux — it never
creates or owns sessions, windows, or panes.

## Principles

1. **The CLI is sacred, and so is your tmux.** Sessions are the real `claude` binary
   in panes you own. compass never wraps, proxies, or re-renders the CLI, and never
   creates or manages tmux sessions — it only observes, plus two keypress-gated
   actions (reveal a pane, open an "ask" window).
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
| **Lv3 — Deep dive** | `Tab` `Tab` | Split panel: pretty transcript reader + **ask the trail** — an interactive Claude grounded in this session's full history |

## Status

📐 Design phase. Read the docs and argue with us:

- [`docs/SPEC.md`](docs/SPEC.md) — product spec: UX model, keymap, states, visual language
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — technical design: data sources, engine, stack, milestones
