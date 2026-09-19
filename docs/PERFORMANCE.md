# Render performance — what was slow, what was done, how to check

The deck redraws on every keypress and every second. A frame is cheap on a
short session and was not on a long one: the cost of drawing scaled with the
length of the journey, not with the size of the screen. This file records
each measurement and its fix, so the next slow keypress is profiled against a
known baseline rather than guessed at.

## 1. How to measure

```sh
# per-keypress cost: a j/k walk on a grown very-long scene, 200x50
go test ./internal/ui -run XXX -bench Walk -benchtime 20x

# where a keypress goes
go test ./internal/ui -run XXX -bench TrailWalkLv2_3000 -benchtime 20x \
  -cpuprofile cpu.out -o ui.test && go tool pprof -top -cum ui.test cpu.out
```

`internal/ui/perf_bench_test.go` holds the benchmarks. `longStand` takes the
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

## 4. Rules that fall out of this

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
