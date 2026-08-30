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
for subagents, merges when they return. **New at the top**, like `git log` (decided —
see §7).

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

**Lv3 — Deep dive.** The trail area splits 50/50 (decided — see §7):

- **Left half: the reader.** The full conversation rendered like `bat` renders code —
  syntax-highlighted tool calls, folded tool outputs (unfold with `Space`), search
  with `/`. Entering Lv3 from a selected Lv2 waypoint lands scrolled to *that moment*.
- **Right half: ask the trail.** Press `a` and compass opens a real `claude` session
  (in a new, labeled window of that session's own tmux session — again, the actual
  CLI, in *your* tmux) pre-loaded with this session's transcript
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
blocked session impossible to miss. Attention is **visual-only by default** — no bells
(bells over SSH are exactly the trouble we're avoiding; decided — see §7). Three
escalating cues, all silent:

1. **In panel** — amber sessions sort to the top of the fleet with an age counter.
2. **Tab title** — compass sets its terminal tab title via OSC escape:
   `⌂ compass ▲2` when two sessions need you, `⌂ compass` when all calm. Windows
   Terminal, iTerm, GNOME Terminal etc. all render this on the *unfocused* tab, and
   it travels fine over SSH.
3. **tmux status bar (optional)** — `compass status` is a one-shot subcommand that
   prints a compact fleet summary (`●3 ▲1`); drop `#(compass status)` into your own
   `status-right` and every tmux session shows it. compass never touches your tmux
   config itself.

### 2.5 Two display modes, one binary

**Deck mode (default).** `compass` runs full-screen in its own terminal tab — *outside*
tmux. It sees every Claude session on the machine at once (transcripts aren't
tmux-scoped), across all your tmux sessions and even bare-shell sessions. Fleet on the
left, selected session's trail on the right; Lv3 splits the trail area.

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

Each fleet entry shows *where the session lives* (`dev:1.0` = tmux session `dev`,
window 1, pane 0) so you always know where to go.

**Sidecar mode.** `compass --sidecar` renders the original narrow (≈42-col) panel for
people who arrange it inside a tmux pane next to one CLI. Same engine, condensed
layout, fleet as a strip on top.

**compass never manages tmux.** It creates no sessions, windows, or panes for its own
layout (decided — see §7). You own your multiplexer; compass reads it (`list-panes`,
`capture-pane`) and performs exactly two opt-outable write actions, both only on an
explicit keypress: **reveal** (focus the pane you asked to jump to) and **ask the
trail** (open the historian in a new, clearly-labeled window — see §3).

## 3. Keymap — and the three-keypress proof

Global keys (work at every level):

| Key | Action |
|-----|--------|
| `1`–`9` | select session N — its trail renders immediately |
| `Tab` / `Shift+Tab` | zoom in / out (Lv1 ⇄ Lv2 ⇄ Lv3) |
| `j`/`k` or `↓`/`↑` | move between nodes / waypoints / lines |
| `Enter` | **reveal**: focus the selected session's pane in *your* tmux (`select-window` + `select-pane`), so switching to your tmux tab lands on it; on a Lv2 waypoint: open that moment in the Lv3 reader |
| `g` | grab the oldest needs-you session: select it *and* reveal its pane, one key |
| `a` | ask the trail: open the historian — a real `claude` — in a new, labeled window (`ask:api`) of the tmux session where that Claude lives; if it has no pane, compass shows the exact command to copy instead |
| `Space` | Lv3 reader: fold/unfold a tool output |
| `/` | search (Lv3 reader) |
| `?` | help overlay |
| `Esc` | zoom out one level |
| `q` | at Lv1: quit compass; deeper: zoom out |

Note what's *absent*: no "new session" key, no kill, no rename — compass doesn't
manage sessions or tmux (§2.5). Reveal and ask are its only tmux writes, each behind
an explicit keypress and disableable in config (`tmux_actions = "readonly"`).

The constraint, proven:

| "I want to…" | Keys | Count |
|--------------|------|-------|
| see what session 3 is doing | `3` | 1 |
| unblock whichever session needs me | `g`, then switch terminal tab — cursor is already on the right pane | 1 |
| see why webapp's tests fail | `2` `Tab` | 2 |
| read the exact moment a bug was fixed | `2` `Tab` … `Enter` (j/k to aim are free-aim, not depth) | 3 |
| ask a session why it made a choice | `2` `a` | 2 |
| get to session 3's actual CLI | `3` `Enter`, switch terminal tab | 2 |

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
- **Density**: every line answers a question; anything that doesn't is dimmed or
  dropped. Truncate with `…`, never wrap. Sidecar mode earns its 38–44 columns;
  deck mode spends its extra width on the trail, never on chrome.
- **Useful before pretty**: color and glyphs are reinforcement, never the only
  carrier of meaning. Everything must read correctly in pure monochrome — glyph
  shape (`●▲○◍`) and position carry state on their own. `NO_COLOR` and dumb-glyph
  (`* ! o x`) fallbacks are first-class, not afterthoughts (matters over SSH and in
  odd terminals).
- **Empty states are designed**: a fresh session shows `◉` and its prompt, plus
  "scouting will appear here" in dim text — never a blank panel.

## 5. What compass is not

- Not a session manager that owns your processes — tmux owns them; compass only
  observes (plus the two keypress-gated actions: reveal, ask).
- Not a tmux layout tool — it never creates sessions, windows, or panes for itself.
- Not a replacement chat UI — input always happens in the real CLI.
- Not a metrics dashboard — no token graphs, cost meters, or charts in v1. The trail
  is a narrative, not analytics.
- Not multi-agent-vendor — Claude-only first (subscription + Bedrock). The engine
  isolates the transcript adapter so others can come later.

## 6. Open questions (cooking together)

1. **Deck fleet density** — with many sessions (10+), does the two-line fleet entry
   (status + location) get cramped? Fallback: one-line entries with location revealed
   on selection. Decide with real data once M0 renders.
2. **Historian window placement** — `a` opens `ask:name` in that session's tmux
   session. Alternative: always open in a dedicated `compass-ask` tmux session so
   asks never clutter work sessions. (Both are windows the *user* owns and can kill;
   neither makes compass a layout manager.)

## 7. Decision log

| # | Question | Decision (2026-08-30) |
|---|----------|-----------------------|
| 1 | Trail direction | **Newest on top**, like `git log`. |
| 2 | Lv3 real estate | **50/50 split** of the trail area (deck) / widened panel (sidecar). |
| 3 | Narrator budget | **On by default (opt-out)**, no hard cap — Haiku is cheap enough. Caching and batching stay (efficiency, not cost policing). `narrator = "off"` in config for those who want it. |
| 4 | Fleet scope | **Everything on the machine, one view.** compass runs standalone (own terminal tab, outside tmux) in deck mode and observes all tmux sessions + bare-shell sessions. compass is a pure *consumer* of tmux — it never creates or manages tmux sessions; the user keeps full control of their multiplexer. |
| 5 | Bell policy | **No bells.** Visual-only: amber sort + age in panel, OSC tab-title badge (`⌂ compass ▲2`), optional `compass status` for the user's own tmux status-right. Bells over SSH (e.g. Windows Terminal → SSH → Linux) are unreliable and annoying — permanently out. |
| 6 | Color usage | **Useful first.** Glyph shape and position carry all meaning; color reinforces. Monochrome/`NO_COLOR`/plain-ASCII fallbacks are first-class. |
