# Performance — what was slow, what was done, how to check

The deck redraws on every keypress and every second, and at launch it reads
every live transcript from its first byte. A frame is cheap on a short
session and was not on a long one: the cost of drawing scaled with the
length of the journey, not with the size of the screen; and the launch
scaled with the bytes on disk, one core at a time. This file records each
measurement and its fix, so the next slow keypress or slow start is profiled
against a known baseline rather than guessed at.

## 1. How to measure

```sh
# per-keypress cost: a j/k walk on a grown very-long scene, 200x50
go test ./internal/ui -run XXX -bench Walk -benchtime 20x

# where a keypress goes
go test ./internal/ui -run XXX -bench TrailWalkLv2_3000 -benchtime 20x \
  -cpuprofile cpu.out -o ui.test && go tool pprof -top -cum ui.test cpu.out
```

```sh
# launch: the fleet's first poll cold, the board's first poll, and the
# fleet's first poll warm from a resume cache — 8 live sessions of ~43MB
# each (342MB) and 150 archived, written by startupHome
go test ./internal/ui -run XXX -bench Startup -benchtime 3x
go test ./internal/ui -run XXX -bench StartupRefresh -benchtime 2x \
  -cpuprofile cpu.out -o ui.test && go tool pprof -top -cum ui.test cpu.out
```

`internal/ui/perf_bench_test.go` holds the keypress benchmarks and
`internal/ui/startup_bench_test.go` the launch ones. `longStand` takes the
`very-long` scene and grows its lead session to N legs with `dayLongTrail`
(the scene's own builder, in `scenario_test.go`), then presses keys as
`sceneModel` does: `"1", "tab"` lands on the trail with the reader beside it
(Lv2), a further `"tab", "G"` in the reader (Lv3), `"1"` alone on the board.
Each iteration presses `k` or `j` and draws one frame. Widths and heights are
the deck's, so a number here is a number the person at the keyboard feels.

A budget: one frame at 60 Hz is 16 ms. A keypress under ~20 ms reads as
instant; above ~80 ms the cursor visibly trails the key.

## 2. The keypress on a long trail (round 69)

**Symptom.** `j`/`k` on the trail lagged once the conversation was long. The
reader beside it was suspected; it was not (the reader's document has been
cached since round 68, `readerCache`). The trail was.

**Measured, before** (ms per keypress, 200x50):

| | 160 legs | 1,000 legs | 3,000 legs |
|---|---|---|---|
| Lv2 trail + reader | 78 | 560 | 2,100 |
| Lv3 reader `j`/`k` | | | 695 |
| Board | | | 550 |

**Causes, in order of weight.**

1. **The trail document was rebuilt on every call, and called six to ten
   times a frame.** `trailDoc` renders every row of the journey through
   lipgloss. The column, the title's hidden-leg count, the footer's chapter
   keys, the cursor-visibility check, the day clause and the summary block
   each asked for it independently. Nothing was shared between them.
2. **A trail longer than its panel built the document three times per
   call.** `trailDoc` tries the column with lane heads, then without, then
   dense (no air between rows), each attempt a full build, the first two
   thrown away.
3. **The wait-on-you sum was quadratic.** `promptWaits` called `promptWait`
   per prompt, and `promptWait` walked every leg — prompts × legs — and the
   summary block, the footer and the board verdict each asked for it several
   times a frame.
4. **The trail's cursor re-flattened the reader.** `anchorOn` called
   `ReaderAnchor(m.events, …)`, which flattens the whole conversation, rather
   than reading the reader's cached document.
5. **`TrailRows`** (the selectable rows) was recomputed at sixteen call
   sites per keypress.

**Fixes.**

- `trailDoc` is split into `trailDocBare`, the document without its cursor,
  and `withTrailCursor`, which inverts the one row the cursor stands on. The
  cursor changes exactly one row, so the bare document can outlive the key.
- `internal/ui/trailmemo.go`: the Model memoizes bare documents. The key is
  every `TrailOpts` field except `Cursor`, `Scroll` and `Pinned` (walked by
  reflection, so a field added later is in the key without anyone remembering
  it), with `Labels` and `Agents` by size and identity, plus the trail's own
  identity — slice backing-array pointers, lengths, the last leg and the last
  prompt (`trailProbe`). The memo is retired on every message that is not a
  key (poll, tick, resize, narrator) and in every `Set*`; a key keeps it. A
  key that swaps the trail in (selecting a session, the board's Tab) misses on
  the trail's identity rather than retiring the other columns. The board's
  columns share the memo the same way. `trailMemoCap` bounds it, since `Now`
  is in the key and a tick a second would otherwise grow it forever.
- `trailDocBare` goes straight to the dense, headless shape when the node
  count already exceeds the panel height: every node is at least one row in
  every mode, so the two shapes it would have tried first cannot fit.
- `promptWaitsEach` computes every prompt's wait in one sorted pass over the
  legs' and lanes' ends plus a binary search per prompt. `promptWaits` and
  the trail builder use it.
- `anchorOn` anchors off `m.doc(width)` (the cached reader document) when the
  reader is on the lead's conversation.
- `selRows` memoizes `TrailRows` on the Model under the same retirement rule.

**Measured, after:**

| | 160 legs | 1,000 legs | 3,000 legs |
|---|---|---|---|
| Lv2 trail + reader | 3 | 9 | 21 |
| Lv3 reader `j`/`k` | | | 12 |
| Board | | | 18 |

**What is left** at 3,000 legs (~20 ms): the summary block's counts
(`blockRows`, several calls a frame, each re-sorting for `promptWaitsEach`),
the footer's chapter keys, and the memo key's own formatting (~3 ms across a
frame's ten lookups). None of it is quadratic; the next step, if it is ever
needed, is memoizing `blockRows` under the same key.

## 3. The reader's document (round 68)

For the record: the reader flattened its whole conversation twice per key —
once for the anchor, once for the frame — 300 ms a `j` at the 20,000-event
cap. `readerCache` (`Model.doc`) keeps the flattened document between keys,
retired on a fold, a width change, a new event count, or a lane switch;
`renderReaderDoc` and `anchorRow` draw and search through it. 2 ms after.

## 4. The launch (round 70)

**Symptom.** Two to three seconds from `compass` to a fleet, then another
second before the board's columns had trails.

**Where it went.** The deck's first poll knows no fleet, so it polls no
column: `Manager.Refresh` peeks every transcript, then replays every live one
through its state machine — one after another, on one goroutine. Its
`fleetMsg` lands, the board draws with no trails, and the *next* tick's poll
— a second later — replays every column's transcript a second time, into its
feed's segmenter, again one after another. Two full parses of every live
byte, serial, with a second of nothing between them. Inside the parse:

1. **`Tailer.Poll` read the file with `io.ReadAll`**, which grows its buffer
   from 512 bytes by doubling. On a 20MB replay the copies and the garbage
   they left were 12 of the 26 seconds the poll took under the profiler.
2. **`ParseLine` decoded each line in four nested `json.Unmarshal` calls** —
   the line into `RawMessage`s, the message into `RawMessage`s, the content
   into blocks, a result's content into its string. Every `RawMessage` is a
   copy of its subtree and every nested `Unmarshal` a second validation of
   it, so a 20KB tool result was copied three times and scanned eight before
   `resultText` clamped it to 4KB. `toolUseResult`, which carries the same
   output a second time, was copied whole into every result's `Meta`, whose
   one reader is the task id in a `TaskCreate` result.
3. **Nothing ran on a second core.** The manager's entries and the feeds are
   independent; the manager walked them in a loop, and `feedStore.poll` held
   the store's one lock for the whole replay.
4. **The deck never used the resume cache** `compass status` has had since
   round 61: every launch replayed every live session from byte zero.

**Fixes.**

- `Tailer.Poll` allocates the buffer once, at the size the stat already
  reported, and reads with `ReadAt`.
- `ParseLine` decodes in one typed pass: `message` is a struct, not a
  `RawMessage`; the two `content` fields that vary by shape (`contentField`)
  decide by their first byte and decode only what is there; `toolUseResult`
  (`metaField`) is kept only up to `metaCap` (8KB). A field of the wrong
  type is skipped by the decoder and no longer fails the line — only text
  that is not JSON is malformed.
- `Manager.Refresh` splits its loop: the first pass decides who is live and
  wakes them, `pollAll` replays every live tailer side by side (one worker
  per core), and the second pass judges them. Each entry's tailer, machine
  and outcomes are its own, so nothing is shared between workers.
- `feedStore.poll` holds the store's lock only to find the feed, then the
  feed's own; `pollEach` polls the board's columns one worker per core, and
  `refresh` uses it.
- The first `fleetMsg` that brings a fleet fires the next poll at once
  rather than leaving it to the tick, so the board's trails follow the
  fleet by the time of one replay, not one replay plus a second.
- `main` gives the deck the resume cache `status` uses, and saves it on the
  way out: a warm launch resumes every live tailer at the last mark and
  restores each machine from its fold, and the scan opens no transcript
  whose size and mtime have not moved.

**Measured** (4 cores; 8 live sessions of ~43MB, 150 archived; wall time):

| | before | after |
|---|---|---|
| fleet's first poll, cold | 9.9 s | 1.3 s |
| board's first poll (every column replayed) | 10.3 s | 1.2 s |
| fleet's first poll, warm (resume cache) | — (the deck had none) | 0.035 s |
| idle wait between the fleet landing and the board's poll | one tick (1 s) | none |

**What is left.** The board's columns are still a second parse of every live
transcript — the manager's tailer and the feed's read the same bytes for
different consumers (the machine and the segmenter). Sharing one read would
halve the cold launch again; it means one tailer per live session with two
consumers and a hand-over when a session leaves the fleet, and is not done.
Within the parse, `encoding/json` scans each value twice (validate, then
decode) and the polymorphic `content` subtree twice more; a hand-rolled or
third-party decoder is the next step if the parse itself is ever the
bottleneck.

## 5. Rules that fall out of this

- **A document is built once per change, not once per question.** Anything
  that walks every leg or every event belongs behind a memo keyed on what it
  reads, retired where that state changes.
- **The cursor is an overlay.** Nothing that depends only on the cursor's
  position may force a rebuild of what does not.
- **Nothing per-row inside a per-row loop.** `promptWait` inside the prompt
  loop was the quadratic; sort once and search.
- **Measure at the deck's own sizes with the deck's own key path.** The
  benchmarks press keys through `Update` and draw through `View`; a profile of
  `trailDoc` alone would have missed that it was called ten times.
- **A byte is read once and decoded once.** A `RawMessage` is a copy and a
  nested `Unmarshal` a second scan; a growing buffer is a copy per doubling.
  Size the buffer from the stat and type the decode.
- **Independent files are read side by side.** Anything that walks every
  live transcript at once is a worker per core, not a loop.
