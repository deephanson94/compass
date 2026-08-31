# M6 Contract — go to the session; one session, one identity

Binding API contract, same rules as M0–M5: implementation and tests are written
in parallel against this document; deviations amend it first.

Two field findings from the first real dogfood, after the M5 pane-detection fix
(commit `eea3dd1`):

1. **You cannot type.** The mirror is `capture-pane` output — glass, by design
   (decision #9) — and `Enter` only moved tmux's focus somewhere the user
   could not see. Decided with the user: **Enter goes to the session.**
2. **Session ID is not unique.** Two transcripts on the dogfood machine share
   one session id under different project slugs (a session that changes
   directory writes under the new slug). compass keys identity by id
   everywhere, so duplicates drew two selection markers, shared one pane, and
   fought over a single tailer — an archived session appeared live.

## Identity: the transcript path

```go
// Key identifies a session uniquely. The session id does not: one id can own
// transcripts under several project slugs. The path always does.
func (i SessionInfo) Key() string   // == i.TranscriptPath
```

Every map, selection and cache that keys a session keys it by `Key()`:

| Was | Becomes |
|-----|---------|
| `Manager.sessions map[id]*entry` | keyed by `Key()` |
| `Manager.MarkPaneMapped(ids map[string]bool)` | takes keys |
| `tmuxop.MapSessions(...) map[string]Pane` | keyed by `Key()` |
| `narrator.LegKey(sessionID, leg)` | `LegKey(key, leg)` — signature unchanged, callers pass the key |
| `ui` `selectedID`, `restSelID`, `panes`, feed store, `narrated` | keyed by `Key()` |

Rules:
- The session id stays in `SessionInfo.ID` and stays what the *reader*, the
  historian preamble and `claude --resume` use. It is a label, not a key.
- Two entries sharing an id are two sessions: both render, each with its own
  pane, tailer, trail and selection. Neither may borrow the other's anything.
- `MapSessions` still pairs by cwd; when several sessions share a cwd the
  existing rule stands (newest first to lowest pane target), now with keys.
- Narrated labels cached under the old id-based keys are simply never hit
  again; the cache is regenerable and no migration is required.

## Enter goes to the session

```go
// Attach hands the terminal to a pane. Outside tmux that means attaching this
// terminal to the pane's session; inside tmux it means switching this client
// to it — the same intent, the shape the situation allows.
//
// Both select the window and pane first, so the client lands where the caller
// pointed. Pure construction: it builds the command and starts nothing.
func Attach(target, paneID string, insideTmux bool) *exec.Cmd
```

- Outside (`$TMUX` empty): `tmux select-window -t <sess:win>`, `select-pane -t
  <paneID>`, then `attach-session -t <sess>` — expressed as ONE command so the
  focus is set before the terminal is handed over (`tmux ... \; ... \; ...`).
- Inside (`$TMUX` set): the same two selects, then `switch-client -t <sess>`.
  Nothing is suspended; the user's client simply moves.
- ui: `Enter` at Lv1 on a mapped session runs it — outside tmux through
  `tea.ExecProcess` (compass suspends, the CLI has the terminal, detaching with
  the user's own prefix `d` returns to compass exactly as it was); inside tmux
  as a plain command. Unmapped session → the note it has today. `-readonly`
  refuses, with a note: compass issues no tmux command that changes state.
- The footer says what Enter does and how to come back: outside tmux,
  `enter attach (prefix d returns)`.
- `Enter` at Lv2 still opens the reader at that row's moment (unchanged), and
  `g` still grabs the longest-waiting session — now attaching to it.
- `reveal` is retired: attach subsumes it, and a focus change nobody sees was
  the thing that made Enter feel dead.

## Test contract (T-numbers continue)

| # | Scenario | Expects |
|---|----------|---------|
| T62 | Two SessionInfos, same ID, different slugs | distinct `Key()`; Manager tracks both, each with its own state; neither inherits the other's liveness |
| T63 | `MarkPaneMapped` + `MapSessions` under duplicate ids | only the keyed session is live/mapped; the twin stays archived and paneless |
| T64 | ui: duplicate ids in the fleet | exactly one `▸`, exactly one row shows the pane; selecting one does not select the other |
| T65 | `Attach` outside tmux | one `tmux` command carrying select-window, select-pane and `attach-session -t <sess>`, in that order |
| T66 | `Attach` inside tmux | same selects, then `switch-client -t <sess>`; never `attach-session` |
| T67 | Enter on a mapped session builds the attach command; unmapped → note; `-readonly` → note, no command | ui section |
| T68 | narrator: `LegKey` distinguishes two same-id sessions | different keys for the same leg under different transcript paths |

Goldens under `testdata/golden/`, ASCII profile forced. All offline: no test
starts tmux or attaches anything.
