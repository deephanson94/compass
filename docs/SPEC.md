# compass — Product Spec

> Status: draft v0.1 · design phase · everything here is up for debate

## 1. The problem

Running many Claude Code sessions in parallel is the new normal. The failure mode is
not that sessions do bad work — it's that the human loses the plot:

- **Past** — *"what journey did this session take?"* You scroll back through 400 lines
  of tool output trying to reconstruct what happened.
- **Present** — *"is it working, or waiting on me?"* A session blocked on a permission
  prompt looks identical to one deep in thought. Minutes die silently.
- **Future** — *"what adventure is it going to do next?"* Claude has a plan (it keeps
  todos), but that plan is invisible unless you interrupt and ask.

## 2. The shape of the answer

A **sidecar panel** next to the real Claude Code CLI. Not a replacement UI, not a
wrapper — a compass you glance at. Three ideas make it work:

### 2.1 The Trail is a git graph

Sessions don't produce a flat log — they produce a **branching journey**, and the
transcript data proves it: subagents are literal sidechains on disk that fork from the
main conversation and merge back with a result. So we render the journey exactly the
way developers already read branching history: a vertical rail of nodes, branch lanes
for subagents, merges when they return. **New at the top**, like `git log`.

Node vocabulary:

| Glyph | Meaning |
|-------|---------|
| `◉` | journey start — the user's prompt, quoted |
| `◆` | completed leg (colored by class) |
| `●` | HEAD — the leg in progress *right now* (breathing animation) |
| `◈` | subagent branch (forks off, merges back with its finding) |
| `◌` | ghost waypoint — *planned* work from Claude's own todo list, dashed lane |
| `▲` | attention marker — a question, permission prompt, or failure lives here |

The ghost nodes are the compass part: the trail doesn't stop at HEAD, it fades ahead
into where the session is *going*. Past below, present at the fold, future above it in
dashed strokes.

### 2.2 Legs: the Lv1 classification

Every session's activity segments into **legs** — contiguous spans dominated by one
class of work. Eight classes, fixed, each with one color and one verb:

| Class | Verb | Detected from (heuristics) |
|-------|-------|---------------------------|
| `SCOUT` | scouting | Read/Grep/Glob/WebFetch dominance, Explore agents |
| `DESIGN` | designing | plan mode, ExitPlanMode, long text turns, spec/doc writes before code |
| `BUILD` | building | Edit/Write on source files, new files |
| `FIX` | fixing | edit-run-edit loops, error strings in tool results, test-fail→edit cycles |
| `TEST` | testing | test runner invocations (pytest/jest/go test/cargo test…), parsed results |
| `SHIP` | shipping | git commit/push, PR creation, CI watching |
| `DOCS` | writing | .md edits, comments, READMEs |
| `WAIT` | waiting | no activity: idle, or blocked on the human |

Deterministic, offline, instant. AI never gates rendering.

### 2.3 Three zoom levels, one key

`Tab` zooms in, `Shift+Tab` zooms out. That's the whole navigation model.

**Lv1 — Trail.** The git graph of legs. Five to twelve nodes for a typical session.
Answerable in one glance: what phases happened, in what order, where is it now, what's
planned.

**Lv2 — Waypoints.** Each leg unfolds its detail — the same graph, expanded:

```
◆ TEST  pytest tests/auth                          12m
│  ├ 18 passed · 2 failed · 1.2s
│  ├ ✗ test_refresh_expired_token   AssertionError
│  └ ✗ test_refresh_revoked_token   AssertionError
│
◆ FIX   two bugs in token refresh                  15m
│  ├ bug1  syntax error in middleware.py:88        ✓ fixed
│  ├ bug2  expiry compared in local time, not UTC  ✓ fixed
│  └ touched  middleware.py · tokens.py · conftest.py
```

Waypoints are extracted per class: test runs get parsed pass/fail counts and failing
test names; FIX legs get one line per distinct bug; SCOUT legs get the questions
answered ("how do TokenStore and SessionCache interact"); SHIP legs get commit
subjects and PR links; subagent nodes get the agent's final-report one-liner.

**Lv3 — Deep dive.** The panel splits (or takes over the tmux window — user choice):

- **Left half: the reader.** The full conversation rendered like `bat` renders code —
  syntax-highlighted tool calls, folded tool outputs (unfold with `Space`), search
  with `/`. Entering Lv3 from a selected Lv2 waypoint lands scrolled to *that moment*.
- **Right half: ask the trail.** Press `a` and compass spawns a real `claude` session
  (in a tmux split — again, the actual CLI) pre-loaded with this session's transcript
  and a system prompt making it the session's historian: *"why did you delete
  conftest.py?"*, *"what approaches did you try before this one?"*, *"summarize what
  a reviewer should look at."* It's Claude interrogating Claude's own journey.

### 2.4 The Fleet

The top strip of the panel is always the **fleet**: every live session, one line each.

```
1 ● api      fixing token refresh              3m
2 ● webapp   testing · 18✓ 2✗                 40s
3 ▲ infra    needs you — permission (2m!)
4 ○ docs     idle — finished "update readme"  22m
```

Session state machine (per session, derived from transcript activity):

| State | Glyph | Definition | Panel behavior |
|-------|-------|-----------|----------------|
| **working** | `●` (green) | assistant turn in flight; transcript growing | calm; live verb from current leg |
| **needs you** | `▲` (amber) | turn stopped on a question or permission prompt | rises to top of fleet; age counter; optional bell |
| **idle** | `○` (dim) | turn complete, awaiting next prompt | shows what it finished |
| **stuck** | `◍` (red) | "working" with no transcript growth beyond threshold, or repeating failure loop | flagged with duration |

**needs-you is the whole game.** The single most valuable thing compass does is make a
blocked session impossible to miss. Amber sessions sort to the top, show how long
they've waited, and (optionally) fire a terminal bell / desktop notification.

## 3. Keymap — and the three-keypress proof

Global keys (work at every level):

| Key | Action |
|-----|--------|
| `1`–`9` | jump fleet: select session N *and* switch the left tmux pane to it |
| `Tab` / `Shift+Tab` | zoom in / out (Lv1 ⇄ Lv2 ⇄ Lv3) |
| `j`/`k` or `↓`/`↑` | move between nodes / waypoints / lines |
| `Enter` | act on selection (open waypoint's moment; in fleet: focus session) |
| `a` | ask the trail (spawns historian split) — from any level |
| `Space` | Lv3 reader: fold/unfold a tool output |
| `/` | search (Lv3 reader) |
| `g` | jump to a needs-you session (cycles amber sessions) |
| `n` | new session (tmux window with `claude` + panel follows it) |
| `?` | help overlay |
| `q` / `Esc` | zoom out; at Lv1, return focus to the CLI pane |

The constraint, proven:

| "I want to…" | Keys | Count |
|--------------|------|-------|
| see what session 3 is doing | `3` | 1 |
| unblock whichever session needs me | `g` (focus lands in the real CLI, answer it) | 1 |
| see why webapp's tests fail | `2` `Tab` | 2 |
| read the exact moment a bug was fixed | `2` `Tab` … `Enter` (j/k to aim are free-aim, not depth) | 3 |
| ask a session why it made a choice | `2` `a` | 2 |
| start a fresh session | `n` | 1 |

Rule of the constraint: **every destination is ≤3 *depth* keypresses**; `j`/`k`
aiming within a list doesn't count against depth, and nothing may hide behind a
fourth level. Any feature that can't fit this dies or moves to config.

## 4. Visual language

- **Palette**: one hue per leg class, muted (Tailwind-500-ish saturation on dark,
  600 on light); amber and red reserved *exclusively* for needs-you and stuck. If
  everything is fine, the panel contains no warm colors at all — you can see "all
  calm" from across the room.
- **Typography**: box-drawing rails, Nerd-Font-optional (pure-unicode fallback glyphs
  built in). Right-aligned relative timestamps (`3m`, `40s`) in dim text.
- **Motion**: exactly one animation — the HEAD node breathes (500ms ease, glyph
  alternation `●`/`◐`). Everything else moves only when data moves.
- **Density**: the panel earns its 38–44 columns. Every line answers a question;
  anything that doesn't is dimmed or dropped. Truncate with `…`, never wrap.
- **Empty states are designed**: a fresh session shows `◉` and its prompt, plus
  "scouting will appear here" in dim text — never a blank panel.

## 5. What compass is not

- Not a session manager that owns your processes — tmux owns them; compass arranges
  and observes.
- Not a replacement chat UI — input always happens in the real CLI.
- Not a metrics dashboard — no token graphs, cost meters, or charts in v1. The trail
  is a narrative, not analytics.
- Not multi-agent-vendor — Claude-only first (subscription + Bedrock). The engine
  isolates the transcript adapter so others can come later.

## 6. Open questions (cooking together)

1. **Trail direction** — newest-on-top (git log, spec'd above) vs newest-on-bottom
   (chat order, matches the CLI's own flow). Needs a side-by-side mock.
2. **Lv3 real estate** — split the 40-col panel (cramped) vs temporarily zoom the
   panel to half the window vs new tmux window that swaps back on `q`. Current lean:
   panel expands to 50% while in Lv3, restores on exit — tmux makes this cheap.
3. **Narrator budget** — default cap for Haiku label calls (per session? per hour?)
   and whether narration is opt-in or opt-out on Bedrock (where tokens are real money).
4. **Fleet scope** — all projects on the machine, or current tmux session's projects
   by default with `A` to widen?
5. **Bell policy** — needs-you notification: silent amber only, terminal bell, or
   `notify-send`? Probably a config with amber-only default.
