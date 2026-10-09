# compass-trail (Claude Code mod, early)

This session's trail inside Claude Code itself: a **Trail** pane with the
journey, and one line above the prompt with how the tests, ships and agents
stand. No board and no fleet: a mod sees only its own session.

The journey is not reimplemented here. The mod runs
`compass trail -session <id> -follow` (see `cmd/compass/trail.go`) and draws
the NDJSON snapshots it streams.

## Where it finds compass

It asks each of these `compass -version` and takes the first whose answer
ends `(trail 1)`, the snapshot shape this mod reads:

1. the mod's **compass binary** setting (`/config`, or `pluginConfigs` in
   settings.json): an absolute path, for a compass kept anywhere
2. `COMPASS_BIN`, from the environment Claude Code starts in
3. `bin/compass` inside the mod
4. `compass` on PATH
5. `~/.local/bin/compass`, `~/go/bin/compass`, `/usr/local/bin/compass`,
   `/opt/homebrew/bin/compass`: where the README's installs put it, which an
   app started from the Dock often has no PATH to

An older compass, one that predates `trail`, is never started; the pane says
which binary it found and to install a newer one.

On a shared server, one install serves everyone:

```sh
sudo install compass /usr/local/bin/compass
```

```sh
go build -o mods/compass-trail/bin/compass ./cmd/compass
claude --plugin-dir mods/compass-trail
claude plugin test mods/compass-trail
```

- `/trail` opens the pane
- `/trail-band` hides or shows the line above the prompt

Closed legs are named by Haiku through the session's own model access, one
batch of up to 20 legs per snapshot that has unnamed ones, each leg once
(`hooks/narrate.ts`, the deck narrator's instruction). The evidence is the
leg's `acts` from the snapshot: each tool call in one line.
