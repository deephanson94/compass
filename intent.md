# intent — compass

This file exists so `dokime check` can read the four headings below. It
restates nothing: each section points at the document that governs, and
dokime checks only that the headings are present. If a line here and its
source ever disagree, the source wins and this file is wrong.

## Goals

See where every Claude session has been, where it is, and where it is headed,
without leaving the terminal: the trail, the live state, and the plan as ghost
waypoints. README.md, opening paragraphs and "The three levels".

## Non-goals

docs/SPEC.md §5, "What compass is not": not a session manager that owns your
processes (tmux owns them; compass only observes, plus the keypress-gated
actions `Enter`, `r` and ask, and the event hook when the config names it); not
a tmux layout tool; not a replacement chat UI; not a metrics dashboard; not
multi-agent-vendor.

## Acceptance criteria

.github/workflows/ci.yml on every pull request: `gofmt -l .` prints nothing,
`go vet ./...`, `go build ./...`, and `go test ./... -count=1 -timeout 40m`
pass. docs/PROCESS.md §2 is the developer form of the same commands.

## Invariants

README.md, Principles 1 to 5: the CLI is sacred and so is your tmux; three
keypresses, max, tested in CI; zero config, read-only; heuristics first, AI
second; useful first, beautiful always. docs/PROCESS.md §1, Constraints:
keymap with no new keys, no new levels, depth at most 3; monochrome save the
three state accents; truncate, never wrap; every line answers a question.
Every change is pinned by a test that fails when it is reverted and recorded
as a numbered decision in docs/SPEC.md §7.
