# compass 🧭

**See where every Claude session has been, where it is, and where it's headed — without leaving the terminal.**

You run five Claude Code sessions at once. One is quietly refactoring, one is stuck
waiting for a permission you didn't see, one finished twenty minutes ago, and two are…
doing something. `compass` is the sidecar panel that answers, at a glance:

- **What journey did it take?** — a git-graph-style *trail* of the session's work
- **What is it doing right now?** — live state: working / needs you / idle / stuck
- **What adventure is next?** — Claude's own plan, rendered as ghost waypoints ahead

The main panel stays the **real, untouched Claude Code CLI** — zero re-implementation,
zero new muscle memory. compass lives beside it in tmux.

```
┌ tmux ────────────────────────────────────┬──────────────────────────────┐
│                                          │ ⌂ compass          ● 2 ▲ 1   │
│  the real `claude` CLI — untouched       │ ─────────────────────────────│
│                                          │ 1 ● api      fixing auth bug │
│  ● I'll fix the token refresh bug.       │ 2 ● webapp   tests 18✓ 2✗    │
│    First let me look at the middleware…  │ 3 ▲ infra    needs you (2m)  │
│                                          │ 4 ○ docs     idle            │
│  ⏺ Read(src/auth/middleware.py)          │ ─────────────────────────────│
│  ⏺ Bash(pytest tests/auth -x)            │  TRAIL · api           [Lv1] │
│    ...                                   │  ┊                           │
│                                          │  ◌ ship   open PR            │
│                                          │  ◌ test   full suite         │
│                                          │  ┊                           │
│                                          │  ● fix    token refresh ← 3m │
│                                          │  │                           │
│                                          │  ◆ test   pytest 18✓ 2✗  12m │
│                                          │  ├─◈ agent scouted payments  │
│                                          │  ◆ build  refresh middleware │
│                                          │  │                           │
│                                          │  ◆ scout  auth module map    │
│                                          │  ╵                           │
│                                          │  ◉ "fix the 401 bug"     38m │
│                                          │ ─────────────────────────────│
│ >                                        │ Tab zoom · 1-4 jump · a ask  │
└──────────────────────────────────────────┴──────────────────────────────┘
```

## Principles

1. **The CLI is sacred.** The left panel is the real `claude` binary in a real tmux
   pane. compass never wraps, proxies, or re-renders it.
2. **Three keypresses, max.** Any session, any zoom level, any answer — reachable in
   ≤3 keypresses from anywhere. This is a hard constraint, tested in CI.
3. **Zero config, read-only.** compass watches the JSONL transcripts Claude Code
   already writes (`~/.claude/projects/…`). No hooks required, no API keys, nothing
   installed into your sessions. Delete compass and nothing changes.
4. **Heuristics first, AI second.** The trail renders instantly and offline from
   deterministic classification. A Haiku *narrator* (through your existing `claude`
   auth — subscription or Bedrock) enriches labels in the background, budget-capped.
5. **Beautiful or it doesn't ship.** Calm, quiet, Apple-grade restraint. Color means
   something; motion means something; nothing blinks for attention it hasn't earned.

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
