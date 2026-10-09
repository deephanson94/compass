# compass-trail (Claude Code mod, early)

This session's trail inside Claude Code itself: a **Trail** pane with the
journey, and one line above the prompt with how the tests, ships and agents
stand. No board and no fleet: a mod sees only its own session.

The journey is not reimplemented here. The mod runs
`compass trail -session <id> -follow` (see `cmd/compass/trail.go`) and draws
the NDJSON snapshots it streams. It looks for the binary at `bin/compass`
inside the mod first, then `compass` on PATH.

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
