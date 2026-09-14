# How compass is built

The product is judged on frames, not on code. Every change is made, rendered
at five terminal sizes, reviewed by a panel of simulated operators, pinned by a
test that fails when the change is reverted, and recorded as a numbered
decision in `docs/SPEC.md`. This file says how that loop runs so it can be run
the same way again.

## 1. The pieces

| Piece | Where | What it is |
|---|---|---|
| Scenes | `internal/ui/scenario_test.go` | Fixture fleets, one per operator persona: `second-day`, `subagents`, `two-tools`, `alarm-storm`, `fleet-hygiene`, plus `first-session`, `few-ongoing`, `many-idle`, `very-long`. Each has sessions, trails, panes, agents and an `extra` key list for its own case. |
| Walkthrough | `TestScenarioWalkthrough` | Presses `canonicalKeys`, then `esc`, then the scene's `extra` keys, polling after every key as the real deck does, and writes one frame per key at 80x24, 100x30, 120x34, 152x40 and 220x48. |
| Corpus | `$SCRATCH/scenes/<scene>-<w>x<h>.txt` | The rendered frames. Every review cites `file:line` in it. |
| Goldens | `testdata/golden/*.txt` | Whole-frame snapshots of fixed views. Regenerated with `-update`; a golden that moves is a change to explain. |
| Pins | `internal/ui/round<N>_test.go` | One test per fold, written so that reverting the fold makes it fail. |
| Decisions | `docs/SPEC.md`, the table at the end | One row per round: what was folded, what was held and why, what was checked and cleared. |
| Constraints | `docs/SPEC.md` §3, §4, §5 | Keymap (no new keys, no new levels, depth ≤ 3), monochrome save the three state accents, truncate never wrap, every line answers a question. A reviewer may not propose past them. |

## 2. Commands

```sh
# the suite (go.mod pins 1.24.7; the local toolchain must be used)
GOTOOLCHAIN=local go test -short ./...   # the developer run: under two minutes
GOTOOLCHAIN=local go test ./... -timeout 40m   # the whole suite, sweeps included, as CI runs it; the sweeps run side by side (#325), -parallel N sets how many
COMPASS_SWEEP_COLOUR=1 GOTOOLCHAIN=local go test ./internal/ui -timeout 40m   # the sweeps walk colour on as well (#324), one at a time; one corpus-wide parity pin holds it otherwise
GOTOOLCHAIN=local go vet ./...

# regenerate goldens after a deliberate frame change
GOTOOLCHAIN=local go test ./internal/ui -update -run Golden

# render the corpus (all scenes, five widths)
S=/path/to/scratch
rm -rf $S/scenes && mkdir -p $S/scenes
COMPASS_SCENARIO_OUT=$S/scenes GOTOOLCHAIN=local go test ./internal/ui -run Scenario -count=1

# render one custom key route
COMPASS_SCENARIO_KEYS=tab,G,k,tab COMPASS_SCENARIO_OUT=$S/route \
  GOTOOLCHAIN=local go test ./internal/ui -run Scenario -count=1

# before every commit: the owner keeps a short list of words this public
# repository must never contain (outside the repo). git grep for them, and
# commit only when it prints nothing.
```

## 3. The panel

Each round spawns one reviewer per persona still out, in parallel, as
background agents (`model: "opus"`). The prompt is the same for every persona
except the persona paragraph, the previous report path and the output path. It
gives:

- the repository, branch and HEAD, and the rule that the working tree is never
  edited: mutations are made in a `git archive HEAD | tar -x` copy or with
  `-overlay`;
- the corpus location and how to re-render it, and how to render a custom key
  route;
- the constraints, and the decisions table rows to read before writing;
- the method, in order: **machine check** (every frame exactly its height, no
  row over its width in display cells, header identity at the same cell, no
  clip inside a number or after a dot, slash, dash, comma or bare separator);
  **the previous report item by item**, each MET / NOT MET / HELD-ACCEPTED with
  a frame cited, and for a fold, the pin reverted in a copy and the failing
  test named (a fold with no failing test is NOT MET); **new findings ranked,
  THE ONE THING first**, one defect per item with the frame, the measurement
  and the row the reviewer would draw instead; **a verdict line**, exactly
  `good to go` or `not yet — <the one thing>`.

A watcher loop waits for the report files; the reports are read from their
findings section onward.

Personas, and the question each asks of a frame:

| Persona | Scene | Question |
|---|---|---|
| second-day newcomer | `second-day`, `first-session` | Day two: one session live, yesterday's dozen behind it. Can I get back the one I left two hours ago without the archive? |
| subagents operator | `subagents` | Which agents are out, which came back with what, which went silent, without attaching? |
| two-tools operator | `two-tools` | Two Claude Code and two OpenCode sessions, two in one directory. Which row is which tool, on which model? |
| alarm-storm operator | `alarm-storm`, `very-long` | Three dead on quota, one asking, one hung, one looping, one fine. Which first? |
| fleet-hygiene operator | `fleet-hygiene`, `many-idle` | Namesakes, a session with no pane, a pane that closed, forty archived sessions wearing four names. |

## 4. The fold

For each report, in this order:

1. Read the findings. Lift the reviewer's prototype if one was left in the
   scratch directory (they usually leave a `fix<N>` copy and a `pin<N>.go`);
   otherwise write the change.
2. Decide each item: **fold** (change plus pin), **hold with a reason** (the
   reason goes in the decision row and must be one a frame could refute), or
   **record** (true, no change asked). A reviewer's "not a defect I would
   block on" is still an item to decide.
3. Write the pin. It must assert the fact on the frame or the route, not a
   proxy for it: three folds in this history shipped with pins that stayed
   green on the revert, and each cost a round. Prefer walking the walkthrough's
   own keys with `poll` after each, at every width the fold touches.
4. Run the suite, update goldens if a frame moved on purpose, vet, and the
   sensitive-word grep.
5. Append the decision row to `docs/SPEC.md`: what was folded and why, what
   was held and the reason, what was checked and cleared so the next round
   does not re-raise it.
6. Commit with a message that says what changed in the product's own words
   (no model names in any repository artifact), push, re-render the corpus,
   and spawn the next round for every persona not yet good to go.

The loop ends when every persona has said `good to go`. A persona that has
said it is not re-spawned unless a later fold touches its scene.

## 5. Conventions the reviewers hold you to

- A change lands with its pin in the same commit. "The pin is not a chore after
  the fold; it is the fold."
- A width rule is a proxy; the frame is the fact. Gate on what is drawn
  (`layout` gives no fleet, the sub-row's own fit test fails), not on a column
  count, wherever the two can disagree.
- Say a thing once per frame. Where two rows would carry one clause, the row
  with the room keeps it and the other sheds it, and the decision row names
  which.
- Clauses shed whole, last first; the name and the way out go last of all.
- The help owes a row to every mark a frame draws, at the width that draws it.
- Every held item names the frame that would refute it. A hold with no reason
  is a defect the next round will raise again.

## 6. Releasing

A version is a tag, and only a tag. There are two ways to write that tag.

From the **Actions tab**, which needs nothing installed and no push rights on
tags: Actions → **release** → **Run workflow** → keep the branch on `main`, type
the version (`v0.1.0`) into the one input, run it. The `cut` job checks the
input is a version (`v` then `MAJOR.MINOR.PATCH`, an optional suffix) and that
the tag is not already on origin, runs the whole suite on the commit the branch
is at, and only then writes the annotated tag and publishes the release. It
publishes the release itself rather than waiting for the tag to do it: a tag
pushed with the workflow's own token starts no workflow run, so the `release`
job never sees that tag.

Or from **a shell**, where the tag can be pushed:

```sh
git tag -a v0.1.0 -m "the deck, the trail and the reader"
git push origin v0.1.0
```

Either way the suite runs on the commit before the version is published — the
`cut` job runs it before it writes the tag, and the `release` job runs it on the
tag before it builds. A tag that cannot pass
`go test ./... -count=1 -timeout 40m` is never published.

Then goreleaser (`.goreleaser.yaml`) builds four binaries (linux and darwin,
amd64 and arm64), a `checksums.txt`, and the release itself. The notes are the
commit subjects since the previous tag, oldest first, with `tests:` and `docs:`
subjects left out: the log is the round-by-round record, the notes are what
changed for someone running the deck. The `edge` tag is ignored when the notes
look for that previous version (`git.ignore_tags` in `.goreleaser.yaml`): it
names the latest build of main, not a version, so it is no boundary for a
release's notes.

Versions start at **v0.1.0** and stay on 0.x while the SPEC still moves, and
a 0.x release is an ordinary one — the major version says the ground is still
moving, so no flag has to. `prerelease: auto` marks a release as a pre-release
only when the tag says so itself (`v0.2.0-rc1`, `v1.0.0-beta.1`); `edge` is the
only standing pre-release.

Between tags nobody builds from source. Every push to main refreshes one
pre-release named `edge`: the same four binaries, its tag and release deleted
and recreated at the merge commit, wearing `edge-<short sha>` where a version
would be. That job does not run the suite again — ci.yml ran it on the pull
request — it only builds.

`compass -version` prints whichever build you are on: `dev` from a working
tree, `edge-<short sha>` from main, `vX.Y.Z` from a tag.

To see what a release would contain without publishing one:

```sh
goreleaser check
goreleaser release --snapshot --clean --skip=publish   # the artifacts land in dist/
```

## 7. dokime

This repo is governed by dokime: `intent.md` is the agreement, `units/` is the
ledger, `python3 tools/dokime.py check` is the boundary check, and its `next:`
line names the command that applies. Open a unit before work
(`python3 tools/dokime.py open <name> --condition "run: ..."`) and end every
commit message with the trailer `Unit: <name>` in the final trailer block.
Handoffs are optional and in whatever form this repo already keeps. Never edit
`intent.md` or a unit's done_condition without asking.
