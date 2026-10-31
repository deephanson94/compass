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
│ FLEET · live          │ ⌁ dev:1.0 · live                 │ TRAIL · api           [trail]  │
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
dim once it's history (and marked `unread` in words, so monochrome reads it
too). Under each name is how the journey stands — the suite,
the ship, the agents (`✗ red 18✓ 2✗ · edited since`, `✓ shipped 4m ago`,
`◈3 out · oldest 20m`) — or, for a session that needs you, the question itself.
HEAD at the bottom of each column wears the fleet's glyph: `● build  wiring the
filter    for 1h` while it works, `◍ build  Bash: python backfill.py --all
silent 4m` when it has hung, `▲ design asks you   waiting 4m` with the question
spelled out beneath, `● build  encoder.py   ◈3 out 20m · quiet 15m` when it is
parked on agents it sent out — and the fleet row says the same sentence, and
keeps it over its label when it is narrow. A subagent lane reads `⋯ 20m out`,
`✓ 7m ago` with its finding beneath, or `⌀ 7m ago` when it came back with
nothing; a lane whose label begins another live session's prompt is marked
`→3`, a hedge that `3` goes and checks. A trail
longer than its column is drawn without the air between legs, with the hour on
the rail where it turns; a tall board packs its columns into bands, each as tall
as its tallest trail. `Tab` opens a session; `Shift+Tab` comes back. The deck
above is that one trail with the **live mirror** switched on (`m`): the selected
session's actual tmux pane, streamed read-only via `capture-pane` — you watch the
real CLI render, but compass owns no PTY. When you want to *type*, `Enter` hands you the terminal:
outside tmux compass suspends and attaches, so the pane is a real PTY with a real
keyboard and your own prefix + `d` brings the deck back; inside tmux your client
just switches. `g` grabs whichever session has been waiting on you longest and
attaches to it. Nothing to manage: compass creates nothing in tmux on its
own. The one thing it opens is new work, when you ask for it with `n` (below):
a window by default, a split pane or a tmux session if you choose. From then
on it is yours like anything else in tmux.

A session that ended its turn on a question — *which of these should I do next?*
— keeps its place on the board until you answer it, however long that takes and
whether or not its pane is still open: the question is read off the transcript
itself, so closing the pane or quitting compass does not lose it, and replying is
what clears it. So does ending the session — `/exit`, a closed terminal, a reboot —
because a question in a session nobody is running is one you already answered by
leaving. Questions from before today sit under the alarms of
the moment, `asks you · waiting 2d` under the name, and `g` takes today's first.
When the pile is one you are not going to work through, `x` puts one down for
good and `X` takes every one of them off the board — `A`, then `X`, brings them
all back.

The trail reads like the conversation does — oldest at the top, the newest work
at the bottom, and it stays pinned there so the latest is always on screen.
Above it the day is added up: its legs counted by class — `◆ build 32 legs
3h45m`, `◆ test 32 legs · 10 red`, the subagent lanes, the wait on you — one row
per class with more than one leg, and the trail takes the rows that are left. Every
column of the board wears the same block over its trail; a short trail of one
of each wears none.
`Tab` opens one session: the trail on the left with a cursor on the present,
and to its right the conversation itself, anchored to whatever trail row the
cursor is on — the trail is a minimap, the transcript is the code. `Tab` again
hands the keys to the conversation.

## Install

Take a binary from the [releases page](https://github.com/deephanson94/compass/releases)
— one archive per version (`vX.Y.Z`):

```sh
tar xzf compass_<version>_<os>_<arch>.tar.gz   # linux/darwin, amd64/arm64
install compass ~/.local/bin/
compass -version                               # says which build this is
```

Or build it from source, with Go 1.24 or newer:

```sh
go install github.com/deephanson94/compass/cmd/compass@latest   # the newest tag
```

## Principles

1. **The CLI is sacred, and so is your tmux.** Sessions are the real `claude` binary
   in panes you own. compass never wraps, proxies, or re-renders the CLI, and never
   creates or manages tmux sessions — it only observes, plus the keypress-gated
   writes: `Enter`, which hands you the session's own terminal; `r`, which
   types a line, a menu's digit, or the escape that stop is, into its pane; and
   `n`, which opens new work (a window, a split pane or a tmux session) and
   leaves it to you.
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

## The three levels

| Level | Keys | Shows |
|-------|------|-------|
| **Board** | opens here | Every session as a card, urgent first: its badge, its goal, today's work newest first. `h`/`l` across, numbers jump, `Enter` attaches |
| **Session** | `Tab` | That column expanded: its header card, the journey as a git graph with a cursor on the present, and beside it the conversation following the cursor — or the live tmux pane, with `m`. `j`/`k` walk the legs, `h`/`l` slide to the next session, `[ ]` step your prompts |
| **Reader** | `Tab` `Tab` | The keys move into the conversation: scroll, unfold tool output, search, `[ ]` between your turns. `a` at any level hands you **ask the trail** — a Claude grounded in this session's full history |

The board is drawn as cards, for a wide screen with many sessions: each card a
bordered panel at least 84 cells wide (`card_width`), the rows shared evenly
(`cards_per_row`, or `+`/`-` live), as tall as the screen gives it. A card is the
fleet row; a coloured badge with a short reason beside it (below), the session's
goal (one line the narrator writes once a turn is over, the first prompt until
then) and what it is doing now; today's tally; and
today's work newest first, grouped by turn, so the breathing HEAD row is right
under the goal. `p` pins a card to the board's front; the pin goes when the
session leaves the live fleet. `board = "columns"` (or `-board columns`) is the
board as it was: every session's whole trail side by side, oldest first.

A card's badge is one of six statuses, and the dim word after it says why:

| Status | Means | Reason beside it |
|---|---|---|
| **working** | a turn is in flight and writing | `subagent` when the work is a subagent's |
| **still working** | a turn silent 10m–1h (`still_after`) | what it waits on: `subagent`, `MCP`, `web`, `workflow`, `think` (the model owes a reply), `start` (the prompt has no reply yet) |
| **needs you** | a question, a call held for your permission, or a verdict that needs you | `approve` for the permission prompt, `verdict` for a verdict |
| **needs attn** | silent past 1h (`attn_after`), a shell command past its timeout, a loop, or an API error | the same words, or `timeout`, `loop ×4`, `API 403` |
| **exited** | a turn in flight whose Claude Code process is gone | `mid-turn` |
| **idle** | the turn is over | `✓ shipped`, `✓ done` (a verdict that needs nothing), `cancel` (Esc), `bg job` (an agent still writing), `new` |

A **verdict** is a closing line a session writes for a reader who isn't watching,
such as a PR watcher's review: `VERDICT: needs-you: changes the public C header`
or `VERDICT: auto-ok`. When the model's last message ends on one, needs-you is
**needs you** with that reason on the card's ▲ row (and `g`, the hook and the
status line count it), and auto-ok is **idle** `✓ done`. A session nothing is
tailing that ended on needs-you stays on the board, as an unanswered question
does. `verdict_pattern` in the config sets another form.

Silence counts from the session's last line, or its subagent's. An Edit, Write or
Read out for 90 seconds is held behind a permission prompt; a shell command is
judged by its own timeout. Test results are the trail's to show, not the card's.

**Quiet cards.** A board that a PR watcher fills can hold ten finished reviews
beside the three sessions you're working in. Set `compact_sessions = "pr"` (the
tmux sessions whose idle sessions are quiet) and/or `idle_compact = "1d"`, and a
quiet session draws as one row under the cards instead of a card:

```
quiet · 3 · 195 archived · A browses
6 ○ pr130_review · ✓ done · ⌁ pr:0                                   2d
7 ○ pr_review_gemma · idle · ⌁ pr:1                                  1d
```

It keeps its digit. Selecting it draws its card again, and so does anything it
owes you: a ▲ question or verdict, a hang, a loop, work starting. A pinned
session is never quiet. Where the screen is too short for the rows, the strip
names them as it names any session without a card.

Below 110 columns there is no board: the deck is the fleet list beside one trail, `Tab` puts a cursor on it, and a third `Tab` gives the reader the whole width.

## Using it

```
go build -o compass ./cmd/compass    # Go 1.24+; a single static binary

compass                              # the deck, full screen — run it in its own terminal tab
compass -readonly                    # observe only: Enter no longer attaches
compass -narrator off                # heuristic labels only, no claude calls
compass -board columns               # the column board: every trail whole, oldest first
compass -live-within 0               # only sessions tmux is holding count as live
                                     #   (an unanswered question stops holding one too)
compass -opencode-db ""              # leave OpenCode's sessions out (default: its store, if present)
compass -pr-interval 0               # don't ask Gitea (through `tea`) about each session's pull request
compass -tea-login work              # which tea login to ask as (default: tea's own)
compass status                       # one-shot fleet summary, e.g. "▲1 ●2 ○1 PR▲1 RV▲2"
compass prs                          # diagnostic: poll Gitea now — each session's checkouts and pull requests
compass panes                        # diagnostic: which pane holds which session
compass new -C ~/work/api explore per-tenant rate limits
                                     # a new tmux window running claude with that prompt
                                     #   (-as pane|session, -with shell, -worktree, -n NAME, -t TARGET; see below)
```

### Starting new work

`n` opens a panel over the board. The common launch is `n`, the prompt, `enter`:
everything else arrives filled in from the selected card.

```
┌ new window ──────────────────────────────────────────────────────┐
│   run       ‹ claude ›                                           │
│   as        ‹ window ›                                           │
│   in tmux   ‹ dev ›                                              │
│   dir       ~/work/api                                           │
│   name      explore-per-tenant-rate                              │
│   worktree  off                                                  │
│   then      ‹ stay ›  opens in the background                    │
│                                                                  │
│ › explore per-tenant rate limits in the middleware; map it first▏│
│                                                                  │
│ tab next · ←/→ choose · enter opens · esc closes                 │
└──────────────────────────────────────────────────────────────────┘
```

- **run**: `claude`, a `shell`, or any harness the config names (below). ←/→ cycles.
- **as**: where the work opens. ←/→ cycles.
  - `window` (default): a new window in an existing tmux session.
  - `pane`: a side-by-side split beside the selected card's own pane. A card
    with no pane can't be split, and the panel says so.
  - `session`: a new detached tmux session, named like the window.
- **in tmux**: for a window, the session it opens in. It defaults to the
  selected card's session, and ←/→ cycles through the others. For a pane the
  row shows the pane being split; for a session, the new session's name.
  With no tmux session running at all, the panel starts on `session`.
- **dir**: where the selected card works. ←/→ walks your favourites (`dir`
  lines in the config, marked ★), then the directories the fleet has worked
  in, most recent first. You can also type a path (`ctrl+u` clears the row).
  A path that doesn't exist yet gets a line of its own under the row,
  `↳ new · enter creates it, empty`, and Enter makes it with `mkdir -p`.
  Nothing is created if tmux refuses the launch, and a new folder can't take
  a worktree because there is no repo to branch from.
- **name**: the window's name. It is also passed as `claude -n`, so the card is
  called the same thing, and it is the worktree's branch. It opens on the tmux
  session the work goes into and an underscore (`build_`), and follows the
  `in tmux` row until you change the part in front of it or clear it. Left as
  just the prefix, or empty, the prompt's first words finish it
  (`build_fix-the-flaky-test`). A new tmux session has no prefix.
- **worktree**: off by default; space or ←/→ turns it on. On means a new branch
  checked out in `<repo>.worktrees/<name>` beside the main checkout, off its HEAD.
  The window then opens at the same depth in that worktree as the directory you
  picked. It works the same for every harness.

- **then**: `stay` opens the work in the background and keeps you on the
  board. `go` hands you the new pane right away, the way `Enter` hands over a
  card's: outside tmux compass suspends and attaches, inside tmux your client
  switches, and `prefix d` brings the board back. It defaults to `go` for
  anything that never gets a card (a shell, a harness other than `claude`)
  and to `stay` for `claude`. Space or ←/→ changes it, and a choice you make
  sticks when you change `run`.

The panel's starting choices come from the config's `new_run`, `new_as`,
`new_worktree` and `new_then` lines (below). `new_as = "pane"` on a card with
no pane opens on `window`. `compass new` takes `new_run`, `new_as` and
`new_worktree` as the defaults of `-with`, `-as` and `-worktree`.

The prompt goes to the harness as an argument, not typed into the pane, so it
arrives whole: quotes, `$` and all. With `then: stay` the work opens in the
background, and the deck stays where it was. compass checks the tmux target before it makes a
worktree: a session name already taken or a pane that is gone is refused
before anything is written to disk. When the new session joins the fleet, the deck selects it,
unless you have moved the selection in the meantime. `Enter` takes you there.
`readonly` turns `n` off. In the reader, `n` keeps its search meaning.

`compass new` is the same launch from a shell or a tmux binding. `-as pane` or
`-as session` picks the place. `-mkdir` creates a `-C` directory that isn't
there yet; without it, a missing directory is refused. `-t` is the session for a window and the pane
for a split. `-e KEY=VALUE`, repeatable, sets a variable in the new pane's
environment only (tmux's own `-e`), for a harness that needs one your shell
doesn't carry. Inside tmux, both default to where you are:

```
bind N command-prompt -p "prompt:" "run-shell -b 'compass new -C \"#{pane_current_path}\" -- \"%%%\"'"
```

`%%%` is the reply with its quotes escaped. The reply then sits in double
quotes for the shell, so a `$` or a backtick in it is still the shell's to
expand. Type it in the `n` panel if you want it kept literal.

### OpenCode too

If OpenCode is on the machine, its sessions join the fleet: compass reads its
store (`~/.local/share/opencode/opencode.db`, or `-opencode-db path`) read-only and
shows each session the way it shows a Claude Code one — same states, same trail,
same reader. Where two tools share a fleet the tag row says which is which and
what model last answered: `claude · opus-4-1`, `opencode · sonnet-4-5`.

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

Everything is optional configuration — compass runs with none. The file is
`config.toml` in your user config directory: `~/.config/compass/` on Linux,
`~/Library/Application Support/compass/` on macOS, or wherever
`$COMPASS_CONFIG` points. You don't have to remember the keys:

- `compass config` prints where the file is, every key with the value in
  effect (yours, or the default), what it does, and any line compass could not
  use.
- `compass config init` writes a starter file with every key commented out at
  its default. It never overwrites a file that is there.
- A line compass can't use is skipped and reported: on stderr, and once in the
  board's footer until the first key. An unknown key comes with a guess
  (`unknown key "new_thn" (did you mean new_then?)`), and a bad value with
  what the key takes.

Every key, with an example value:

```toml
root = "~/.claude"      # the Claude home to observe ($COMPASS_ROOT and -root override)
narrator = "haiku"      # narration model; "off" disables
board = "cards"         # the default; "columns" for every trail whole, oldest first
still_after = "10m"     # a card calls an open turn still working after this much silence
attn_after = "1h"       # and needs attn after this much (hung_after is its old name)
card_width = "84"       # a card's least width; lower it for more cards across
cards_per_row = "3"     # or say how many cards a row; + and - change it live
card_tint = "#1f2b30"   # a card's ground inside its border; a step off your terminal's
compact_sessions = "pr" # tmux sessions whose idle sessions give up their card for one
                        #   row under the cards (comma-separated names)
idle_compact = "1d"     # any session idle this long does the same; "0" (the default) never
readonly = false        # true keeps compass's hands off tmux entirely
mirror = false          # true opens the live tmux pane beside the trail; m toggles it
live_within = "5m"      # a paneless session counts as live this long; "0" = tmux only
                        # (a session holding an unanswered question stays live
                        #  whatever this says while it is running, unless it says "0")
reply = "please continue"   # the stock lines `r` offers, one per line, up to nine
reply = "report status"
hook = "tmux display-message \"compass: $COMPASS_SESSION $COMPASS_EVENT\""
                        # run when a session crosses a line while nobody is looking:
                        # COMPASS_EVENT is needs_you, api_error, stuck, circling,
                        # agents_back or shipped_on_red; COMPASS_SESSION, COMPASS_TMUX,
                        # COMPASS_DETAIL say which. Once per crossing, at most once
                        # every ten minutes per session and event; agents_back fires
                        # once per set of lanes and names the empties. The first
                        # refresh is a baseline and fires nothing.
verdict_pattern = "^VERDICT:\\s*(?:needs-you:\\s*(?P<reason>.+?)|auto-ok)\\s*$"
                        # a session's closing line that says how it came out
                        # (this is the default): a match whose reason group is
                        # non-empty is needs you with that reason, any other
                        # match is done. "off" ignores verdicts
dim = "#9a9a9a"         # the grey the quiet rows are drawn in — ages, leg counts,
                        # the work line, the rails. compass's own default clears
                        # 4.4:1 against the darkest popular dark theme; raise it
                        # for a low-contrast theme or a bright room, lower it to
                        # push the quiet rows further back. A `#rrggbb`, a `#rgb`
                        # or a palette index 0–255; anything else is ignored.
                        # $COMPASS_DIM and -dim override it.
dir = "~/work/api"      # a favourite for n's dir row, one line each, offered in
dir = "~/scratch"       #   this order after the selected card's directory (★);
                        #   one that doesn't exist yet is offered as a new folder
harness = "opencode: opencode run {prompt}"
                        # what `n` and `compass new` can run besides claude and a
                        # shell, one line each: a name, a colon, the command.
                        # {prompt} and {name} are put in their places; a word
                        # whose placeholder is empty is dropped whole. No quoting:
                        # split on spaces. A line named claude or shell replaces
                        # the built-in, e.g. "claude: claude --model opus -n {name} {prompt}"
new_run = "claude"      # what n's panel opens on: run (a harness's name),
new_as = "window"       #   as (window, pane or session), worktree (true or
new_worktree = false    #   false) and then (go or stay; left out, it is go
new_then = "go"         #   for a shell or a harness and stay for claude)
jump = "choose-tree -Zs"
                        # the tmux command `t` runs once the terminal is tmux's
                        # (this is the default). Copy what your own prefix
                        # binding runs to get the same picker from the board;
                        # tmux parses it, quotes and `;` included (tmux 3.2+)
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
| `1`–`9` | select a session. Past nine sessions the numbers go on: `12` is `1` then `2` within 0.7s. `1` alone still selects at once |
| `Enter` | go to it: compass hands you the session's terminal (prefix + `d` returns). Back on the board, the card selected is the one whose pane you left from, wherever your own keys took you meanwhile; a pane with no card leaves the selection where it was |
| `Tab` / `Shift+Tab` | zoom: board → session → the conversation itself |
| `j`/`k` | move — the trail's legs in a session (the conversation follows), the reader's lines once the keys are in it |
| `h`/`l` | across the board's columns, or to the neighbouring session |
| `m` | the live tmux pane beside the trail instead of the conversation; sticks until pressed again |
| `t` | jump to any tmux session, not only the ones with a card: the terminal goes to tmux with its session tree up (`jump = "…"` in the config runs your own picker instead), and prefix + `d` returns. It is your `prefix` binding for a board that runs outside tmux, where tmux never sees the key. Off under `-readonly` |
| `g` | grab the session that has waited on you longest, and go to it |
| `A` | browse the archive: every past session, grouped by project. `A` again comes back where it was pressed; `Shift+Tab` or `Esc` on the archive's list is the live board |
| `a` | ask the trail: a historian `claude` takes the terminal, briefed on this session's transcript; exit returns |
| `ctrl+d`/`ctrl+u` | half a page: the trail, or the reader once the keys are in it |
| `G` | back to the present — the newest row, at any level |
| `[` / `]` | previous / next prompt — the chapters of a trail (your prompts; a teammate's relayed report is drawn but is no chapter); in the counts, the previous / next group; in the reader, your turns, marked and named as it lands on them |
| `n` | new work: a panel to open `claude` with a prompt, a shell, or a configured harness as a new window in one of your tmux sessions (the default), a split beside the selected card's pane, or a new tmux session, optionally in a fresh git worktree. Defaults come from the selected card; `enter` opens, `esc` closes |
| `x` | take the selected session off the board (a test, a `/resume` you are done with). The archive lists it under its own header, name and pane kept; `x` there brings it back. A session that is asking, needs attn, circling or dead on the API stays, and the footer says so |
| `Space` `/` `n`/`N` | reader: unfold a result · search · walk the matches |
| `Space` | in a session: the counts above the trail (`◆ build  6 legs`) open into their legs — `k` off the trail's first row climbs onto them, `Space` opens the class or the lanes under the cursor and folds it again. A leg there is that leg: the conversation follows it, `Tab` reads it, `Enter` attaches |
| `s` | in a session: jump to the counts, and back to the trail — each side keeps the row you left |
| `/` | on the board, the list or the archive: search the fleet — name, opening prompt, branch, any prompt, a leg, a file a leg touched. The header says `/query · 3 of 40`; `esc` clears it |
| `r` | reply: a panel beside the selected session names it, says what it is doing, and offers — the options of the question it is sitting on (sent as the CLI menu's own digit), the stock lines ("please continue", "report status"; the quota one only where a quota was hit; `reply = "…"` in the config replaces them), and **stop** (escape, which interrupts the turn). A session dead on the API is offered the remedy its refusal names (`/login`, typed as those bytes) under its own head, and the quota line named as what it is — a turn, for once the quota is back. A digit acts; `t` opens a line to type. The board carries `↪ sent "…" · 2m ago` (or `↪ answered 1 · "…"`) until the transcript shows the prompt landed. Off under `-readonly` |
| `?` | help |

## Design docs

- [`docs/SPEC.md`](docs/SPEC.md) — product spec: UX model, keymap, states, visual language, decision log
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — technical design: data sources, engine, stack, milestones
- [`docs/dev/`](docs/dev/) — the per-milestone API contracts the code and tests were built against
- [`docs/PROCESS.md`](docs/PROCESS.md) — how it is built: scenes, the rendered corpus, the operator panel, the fold-and-pin loop, and the decision log
- [`docs/PERFORMANCE.md`](docs/PERFORMANCE.md) — performance: what a keypress on a long trail and a launch over a big fleet cost, what fixed each, and how to measure them again
