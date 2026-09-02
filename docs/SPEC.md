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
for subagents, merges when they return. **Time flows downward**: the opening prompt
at the top, the newest work at the bottom, pinned there so the latest is always on
screen (decided — see §7, #12, which reversed the original git-log ordering after
the first dogfood).

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
into where the session is *going*. Past above, present at the fold, future below it in
dashed strokes — the plan is simply further down the same road.

### 2.2 Legs: the Lv1 classification

Every session's activity segments into **legs** — contiguous spans dominated by one
class of work. Seven classes, fixed, each with one verb — and the verb is written on
the row, narrated or not: colour reinforces the class, it never carries it alone (§4).

`WAIT` below is the odd one out and was never built as a leg: waiting is a session
*state*, not a span of work, and it shows in the fleet column as `▲` or `○`.

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

### 2.3 Four zoom levels, one key

`Tab` zooms in, `Shift+Tab` zooms out. That's the whole navigation model.

**Lv0 — Board.** Every trail that fits, side by side: one column per session, urgent first, bright while it has something unread in it (#16). The default view from 110 columns up; `Tab` opens the selected column as Lv1.

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
- **Right half: ask the trail.** Press `a` and compass suspends its TUI and runs a
  real `claude` session in its own terminal — the way `git commit` hands you your
  editor; quitting it drops you back into compass — pre-loaded with this session's
  transcript
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

### 2.5 One mode, one binary

**The board (Lv0).** On a terminal wide enough, the deck opens on every trail
that fits, side by side — one column per session in the fleet's order, each
under its own fleet row, bright while it has something unread in it and dim
once it is history (decision #16). `Tab` opens the selected column as the single
trail below; the three-panel deck drawn here is that trail with the mirror
switched on (`m`, decision #15).

**The deck.** `compass` runs full-screen in its own terminal tab — *outside*
tmux. It sees every Claude session on the machine at once (transcripts aren't
tmux-scoped), across all your tmux sessions and even bare-shell sessions. Three
panels: fleet on the left, the **live mirror** of the selected session's pane in the
middle, its trail on the right. Lv3 splits the trail area.

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

**The mirror is glass, not a terminal.** It streams the selected session's tmux pane
via `capture-pane` polling — the same mechanism tmux's own `choose-tree` preview
uses — so you watch the *actual* CLI exactly as it renders, colors and all, while
compass still owns no PTY and handles no input. Keystrokes always happen in the real
pane, so `Enter` hands you that pane's own terminal (§3). Sessions with no pane show
their latest transcript activity in the mirror's place. The mirror is off by default
(#15): `m` opens it on the single trail from 110 columns; below that the deck is fleet +
trail at every level but Lv3.

The **reader** is a level, not a panel (#15): Lv2 is the trail unfolded with a cursor on
it, and Lv3 opens the conversation anchored to that cursor.

Each fleet entry shows *where the session lives* (`dev:1.0` = tmux session `dev`,
window 1, pane 0) so you always know where to go.

**Beside a session.** There is no second mode, but the deck fits the pane it is
given, and a user who splits their own tmux gets the sidecar the early sketch was
reaching for: from 62 columns the mirror closes (the real CLI is next door) and the
fleet sits beside the trail; below 62 the trail has it alone, with the header
carrying the fleet's alarm and the trail's title naming the selected session. compass
still creates no split of its own — the user does, and compass renders to fit.

**compass never manages tmux.** It creates no sessions, windows, or panes — ever
(decided — see §7). You own your multiplexer; compass reads it (`list-panes`,
`capture-pane`) and performs exactly one opt-outable write action, only on an
explicit keypress: **`Enter`** (hand the terminal to the session you asked for).
Ask-the-trail doesn't touch tmux at all — it runs in compass's own terminal (see §3).

## 3. Keymap — and the three-keypress proof

Global keys (work at every level):

| Key | Action |
|-----|--------|
| `1`–`9` | select session N — its trail renders immediately |
| `Tab` / `Shift+Tab` | zoom in / out (Lv1 ⇄ Lv2 ⇄ Lv3) |
| `j`/`k` or `↓`/`↑` | move: the fleet at Lv1, the trail's rows at Lv2 (the conversation follows), the reader at Lv3 |
| `Enter` | **go to it**: compass hands over the terminal. Outside tmux it suspends and attaches (your own prefix + `d` returns); inside tmux the client switches. One meaning at every level (#13). |
| `g` | grab the oldest needs-you session: select it *and* go to it, one key |
| `G` | back to the present — the newest row (#12) |
| `a` | ask the trail: compass suspends its TUI and runs the historian — a real `claude` grounded in this session's transcript — in compass's own terminal; `exit`/`Ctrl-D` returns to compass |
| `A` | browse the archive |
| `Space` | Lv3 reader: fold/unfold a tool output |
| `/`, `n`/`N` | Lv3 reader: search, walk the matches |
| `ctrl+d`/`ctrl+u` | half a page: the trail at Lv1, the reader at Lv3 (at Lv2 the cursor is what the viewport follows, so the cursor is what moves) |
| `?` | help overlay |
| `Esc` | zoom out one level |
| `q` | at Lv1: quit compass; deeper: zoom out |

Note what's *absent*: no "new session" key, no kill, no rename — compass doesn't
manage sessions or tmux (§2.5). `Enter` (and the `g` that ends in one) is its
only tmux write, behind an explicit keypress and disableable with `-readonly` /
`readonly = true`.

The constraint, proven:

| "I want to…" | Keys | Count |
|--------------|------|-------|
| see what session 3 is doing | `3` | 1 |
| unblock whichever session needs me | `g` — it selects *and* hands over the terminal | 1 |
| see why webapp's tests fail | `2` `Tab` `Tab` from the board, `2` `Tab` from a trail | 2 |
| read the exact moment a bug was fixed | `2` `Tab` `Tab` `Tab` from the board (j/k on the legs aim the reader; aiming is not depth) | 3 |
| ask a session why it made a choice | `2` `a` | 2 |
| get to session 3's actual CLI | `3` `Enter` | 2 |

Rule of the constraint: **every destination is ≤3 *depth* keypresses**; selecting a
session (`1`–`9`, `g`, `j`/`k`) is aiming, not depth, and nothing may hide behind a
fourth level. From the board the trail is one `Tab`, the legs two, the reader three. Any feature that can't fit this dies or moves to config.

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
  dropped. Truncate with `…`, never wrap. The trail earns its 38–44 columns; extra
  width is spent on the panels, never on chrome.
- **Useful before pretty**: color and glyphs are reinforcement, never the only
  carrier of meaning. Everything must read correctly in pure monochrome — glyph
  shape (`●▲○◍`) and position carry state on their own. `NO_COLOR` and dumb-glyph
  (`* ! o x`) fallbacks are first-class, not afterthoughts (matters over SSH and in
  odd terminals).
- **Empty states are designed**: a fresh session shows `◉` and its prompt, plus
  "scouting will appear here" in dim text — never a blank panel.

## 5. What compass is not

- Not a session manager that owns your processes — tmux owns them; compass only
  observes (plus the two keypress-gated actions: `Enter`, ask).
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

## 7. Decision log

| # | Question | Decision (2026-08-30) |
|---|----------|-----------------------|
| 1 | Trail direction | ~~**Newest on top**, like `git log`.~~ **Reversed by #12** after dogfooding. |
| 2 | Lv3 real estate | **50/50 split** of the trail area. Superseded in practice by #13: from Lv2 the reader already owns the middle panel, and Lv3 gives it focus rather than more columns. |
| 3 | Narrator budget | **On by default (opt-out)**, no hard cap — Haiku is cheap enough. Caching and batching stay (efficiency, not cost policing). `narrator = "off"` in config for those who want it. |
| 4 | Fleet scope | **Everything on the machine, one view.** compass runs standalone (own terminal tab, outside tmux) in deck mode and observes all tmux sessions + bare-shell sessions. compass is a pure *consumer* of tmux — it never creates or manages tmux sessions; the user keeps full control of their multiplexer. |
| 5 | Bell policy | **No bells.** Visual-only: amber sort + age in panel, OSC tab-title badge (`⌂ compass ▲2`), optional `compass status` for the user's own tmux status-right. Bells over SSH (e.g. Windows Terminal → SSH → Linux) are unreliable and annoying — permanently out. |
| 6 | Color usage | **Useful first.** Glyph shape and position carry all meaning; color reinforces. Monochrome/`NO_COLOR`/plain-ASCII fallbacks are first-class. |
| 7 | Historian placement | **Suspend-and-exec.** `a` suspends the compass TUI and runs the historian `claude` in compass's own terminal (like `git commit` → editor); exit returns to compass. No tmux involvement — asks are one-off. Leaves `Enter` as compass's only tmux write action. |
| 8 | Fleet density at 10+ sessions | **Two-line entries confirmed** against the M1 golden frames (status line + dim location line). Revisit only if a 15+-session fleet feels cramped in practice. |
| 9 | Live CLI in deck mode | **Yes — the middle panel is a live mirror** of the selected session's pane, streamed read-only via `capture-pane` polling. Glass, not a terminal: compass renders the pane's screen but never takes input for it; interaction goes through `Enter`, which hands over the real terminal (M6). Collapses on narrow terminals. (Possible future: an explicit interact mode forwarding keys via `send-keys` — parked, not designed.) |
| 10 | Fleet liveness (2026-08-31, first dogfood: 280 transcripts vs ~15 real sessions) | **The fleet means "something can need you."** Live = in a tmux pane, plus a small recency door (default 5m, `-live-within 0` shuts it) so a pane-matching miss can never hide a session that is writing its transcript right now. Everything else is the archive: never tailed, never amber, one `A` away, fully browsable (trail/reader/ask all work there). |
| 12 | Trail direction, revisited (2026-08-31, dogfood) | **Time flows downward** — oldest at top, newest at the bottom, **pinned** to the bottom so the latest needs no keys. Newest-on-top was chosen before anyone had used it and fought the thing it describes: a conversation reads downward. Ghost todos moved below HEAD with it (the future is further down, not further up). The trail also became a real viewport — it used to *drop* rows to fit, silently discarding the older half of a long journey. |
| 13 | The trail drives the conversation | From Lv2 the middle panel is the **reader anchored to the trail cursor**, re-anchored on every move: the trail is a minimap, the conversation is the code. Lv1 keeps the live mirror (watching), Lv3 hands the reader focus (reading). `Enter` therefore means one thing at every level — go to the live session. *Superseded by #15: Lv2 has no middle panel; the reader is Lv3.* |
| 14 | What a row says (2026-09-01, dogfood) | **Result, not process.** A fleet row's second line is the last thing the session *finished* — `1216✓ 2✗`, a commit — falling back to the call in flight only when nothing has. "Bash: pytest tests/auth -x" answers whether a session is busy, which the glyph already did; the counts answer whether it is going well, which nothing did. The state word goes with it on the two states the glyph tells you about anyway (`working`, `idle`), and stays on the two it must not let you miss (`needs you`, `stuck`) — the width freed goes to the session's name. And §2.2's own `18✓ 2✗` badge returns to Lv1 legs: the leg has always carried the parsed run, and Lv1 rendered the label and dropped it, putting the one number that says whether a leg went well two keypresses away. Target: a reader spends ~80% of their time at Lv1, so Lv1 has to answer without being left. |
| 16 | The board (2026-09-02, dogfood) | **Lv0: every trail that fits, side by side.** Dropping the mirror left a wide monitor showing one trail and a list of names, and the person looking at it worried about everything it was not showing — the mirror had been answering that worry, badly. The board answers it: the fleet at its 30-column floor, then as many trail columns as fit at 34 or wider, in the fleet's own order (needs-you, stuck, working, idle by recency), the selected session always among them. A column is bright while it has something to read — working, needs-you, or finished within the day and not opened since; `Tab` or `Enter` on it marks it read — and dim once it is history (the first dogfood: "porter finished two minutes ago and it's already dim"). The fleet list is not drawn on the board: its rows are the column headers, and the sessions without a column are named in a strip along the bottom. It is the default view on any terminal wide enough; `Tab` opens the selected column as the single trail, `Shift+Tab` from the trail returns. Board → reader is three `Tab`s, so §3's depth rule holds. The fleet keeps its two-line rows (§6's one-line idea is not needed: the board wants the fleet's width, not its height). Every column is polled each tick — the feeds already existed per session; only the selected one was being read. |
| 15 | The middle panel (2026-09-02, dogfood) | **Off by default; `m` brings the mirror back.** The deck is fleet + trail at Lv1 and Lv2, and the reader joins them at Lv3 — the reader is a *level*, not a panel. Two things decided it. The dogfooding happened under 110 columns, where the mirror never opened, and nobody missed it: the CLI it mirrors is one `Enter` away, in your own tmux. And the reason to want it — checking that a leg was classified right — is served better by the Lv2→Lv3 reader, which opens on exactly the tool calls the leg was built from, than by a glimpse of the CLI's last screenful. The columns go to the trail, which is where labels, reports and results were being truncated. `m` toggles the mirror at Lv1 (`-mirror` / `mirror = true` starts it on), and capture-pane only runs while it is on screen. The tripwire use of capture-pane (ARCHITECTURE §2.4) is untouched: detection never needed a panel. |
| 11 | Fleet grouping | **Live view groups by tmux session → window** (tmux's own order; amber floats within its group, header carries a `▲` echo; `elsewhere` for live-unmapped; headers dropped when degenerate). **Archive groups by project directory, newest first** — "I start sessions in the respective directory." `1`–`9` stay flat; `g` stays one key. |
