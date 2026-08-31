# M7 Contract — the trail flows with the conversation

Binding API contract, same rules as M0–M6.

Dogfood findings:

1. **The trail runs backwards.** Newest-on-top (decision #1, chosen before
   anyone had used it) fights the thing it describes: a conversation reads
   downward. Reversed — oldest at top, newest at the bottom, pinned to the
   bottom so the latest is always on screen without scrolling.
2. **The trail does not scroll.** `render` never scrolled; it *dropped* rows
   until the trail fit, so the older half of a long journey was silently
   discarded and the Lv2 cursor could walk into rows that were never drawn.
3. **The trail should drive the conversation** ("like a minimap"): moving the
   cursor should move the conversation with it, so a row on the rail tells you
   *where in the transcript* you are.

## Direction (reverses SPEC decision #1)

Time flows downward. Top to bottom: the opening prompt, then each leg in the
order it happened, later prompts where they fall, HEAD last among what has
happened, and the ghost todos below HEAD — the future is further down, not
further up. A branch still hangs directly under the leg it forked from. The
rail's end cap marks the journey's start, at the top.

## Scrolling

```go
type TrailOpts struct {
    // …existing fields…
    Scroll int  // first rendered row; clamped to the document
    Pinned bool // ignore Scroll and show the last screenful (the default)
}

// TrailLines returns the trail's full document — every row, uncropped — so the
// caller can measure it, scroll it, and keep a cursor inside it.
func TrailLines(tr journey.Trail, o TrailOpts) []string
```

- `RenderTrail` keeps its signature and now crops a viewport out of
  `TrailLines` instead of dropping rows. **The height budget is gone**: nothing
  is discarded any more, because everything can be reached.
- `Pinned` is the default and survives new data: while pinned, the panel always
  shows the last screenful, so a growing trail keeps its newest row on screen.
- Scrolling up unpins. Reaching the bottom again re-pins, so the common case
  needs no key at all.
- ui: at Lv1 `ctrl+d`/`ctrl+u` scroll the trail a half page and `G` re-pins; at
  Lv2 cursor movement scrolls only as far as it must to keep the cursor's row
  visible, and moving onto the last row re-pins. `j`/`k` at Lv1 still walk the
  fleet — the trail is not the object there.

## The middle panel follows the trail

- **Lv1** — the live mirror, unchanged: watching a session run.
- **Lv2** — the reader, anchored to the cursor's row (`ReaderAnchor` on that
  row's `Time`), re-anchored on every cursor move. It takes no keys: `j`/`k`
  drive the trail, and the conversation follows.
- **Lv3** — the same reader, now holding focus: `j`/`k`, `ctrl+d`/`ctrl+u`,
  `g`/`G`, `Space`, `/`, `n`/`N` act on the document; the trail cursor stays
  where it was, still marking the place.
- Below the three-column width the middle panel is dropped as today; the trail
  keeps working, and Lv3 shows the reader in the trail's place.

## Enter means one thing

`Enter` attaches to the live session at **every** level. Lv2 no longer needs it
to open the reader — the reader is already there, following the cursor. `Tab`
deepens, `Shift+Tab`/`Esc` surface, `Enter` goes.

## Test contract

| # | Scenario | Expects |
|---|----------|---------|
| T69 | `TrailLines` order: prompt, legs, HEAD, ghosts | oldest first, HEAD last of what happened, ghosts after it; a branch directly under its fork leg |
| T70 | Golden 38×20, reversed: the M2 fixture trail | the SPEC mockup upside down — prompt at top, HEAD at the bottom |
| T71 | Pinned viewport: a trail longer than the panel | the LAST screenful, whatever `Scroll` says; growing the trail keeps the newest row visible |
| T72 | Scroll + unpin: scroll up, add a leg → the view does NOT jump; scroll back to the bottom → pinned again | scrolling section |
| T73 | Nothing is dropped: every row of a 200-row trail is reachable by scrolling | scrolling section |
| T74 | Lv2 cursor keeps its row visible, scrolling only as far as needed; last row re-pins | scrolling section |
| T75 | Lv2 reader follows the cursor: moving to another leg re-anchors the middle panel to that leg's moment | middle-panel section |
| T76 | Lv3 keys drive the reader, not the trail; the trail cursor does not move | middle-panel section |
| T77 | Enter attaches at Lv1, Lv2 and Lv3 alike | Enter section |

Goldens under `testdata/golden/`, ASCII profile forced. All offline.
