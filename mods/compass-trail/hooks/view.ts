// What the pane and the band say, worked out from a snapshot as plain rows:
// no elements here, so a test reads it without mounting anything. Glyphs
// follow the deck's (◉ prompt, ◆ leg, ● HEAD, ◈ lane, ◌ ghost); every row
// also says it in words, so it reads in monochrome.

import type { Leg, Snapshot, Sum } from '../types'
import { legKey } from './narrate'

/** Names the narrator gave closed legs, by legKey. */
export type Labels = Readonly<Record<string, string>>

/** A leg's label: the narrator's when it has one, else the heuristic's. */
const labelOf = (leg: Leg, labels: Labels) => labels[legKey(leg)] ?? leg.label

export type Tone = 'prompt' | 'done' | 'head' | 'red' | 'lane' | 'ghost' | 'note'

/**
 * One line of the pane. `lead` is how many characters of `text` open it in
 * the leg class's colour (`cls`): the glyph and the verb, as the deck tints.
 */
export type Row = { key: string; text: string; right: string; tone: Tone; cls?: string; lead?: number }

/** The coloured opening of a leg-like row: "◆ build " is glyph, space, verb padded. */
const leadOf = (cls: string) => 2 + Math.max(cls.length, 6)

const ms = (iso: string) => Date.parse(iso)

/** "now", "4m", "2h", "3d": how long since `from`, at the snapshot's clock. */
export function ago(from: string, now: string): string {
  const s = Math.max(0, (ms(now) - ms(from)) / 1000)
  if (s < 60) return 'now'
  if (s < 3600) return `${Math.floor(s / 60)}m`
  if (s < 86400) return `${Math.floor(s / 3600)}h`
  return `${Math.floor(s / 86400)}d`
}

/** "45s", "12m", "3h45m". */
export function span(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s`
  const m = Math.floor(seconds / 60)
  if (m < 60) return `${m}m`
  return `${Math.floor(m / 60)}h${String(m % 60).padStart(2, '0')}m`
}

const pad = (s: string, n: number) => (s.length >= n ? s : s + ' '.repeat(n - s.length))

/** The block: one row per class with more than one leg, the longest first. */
export function block(snap: Snapshot): Row[] {
  return Object.entries(snap.counts)
    .filter(([, sum]) => sum.legs > 1)
    .sort(([, a], [, b]) => b.seconds - a.seconds)
    .map(([cls, sum]: [string, Sum]) => ({
      key: `block-${cls}`,
      text: `◆ ${pad(cls, 6)} ${sum.legs} legs${sum.red ? ` · ${sum.red} red` : ''}`,
      right: span(sum.seconds),
      tone: sum.red ? 'red' : 'note',
      cls,
      lead: leadOf(cls),
    }))
}

/** The trail, oldest first: prompts, legs and lanes by time, then the plan's ghosts. */
export function trail(snap: Snapshot, labels: Labels = {}): Row[] {
  const items: { at: number; order: number; rows: Row[] }[] = []

  snap.prompts.forEach((p, i) => {
    const who = p.teammate ? `${p.teammate}: ` : p.relayed ? 'relayed: ' : ''
    items.push({
      at: ms(p.at),
      order: 0,
      rows: [{ key: `p${i}`, text: `◉ "${who}${p.text}"`, right: ago(p.at, snap.now), tone: 'prompt' }],
    })
  })

  snap.legs.forEach((leg, i) => {
    const run = [...(leg.waypoints ?? [])].reverse().find(w => w.kind === 'testRun' || w.kind === 'commit')
    const isRed = (leg.waypoints ?? []).some(w => w.kind === 'testFail')
    const badge = run?.short ? `${run.short} ` : ''
    const rows: Row[] = [
      {
        key: `l${i}`,
        text: `${leg.current ? '●' : '◆'} ${pad(leg.class, 6)} ${badge}${labelOf(leg, labels)}`,
        right: leg.current ? `← ${ago(leg.start, snap.now)}` : ago(leg.start, snap.now),
        tone: leg.current ? 'head' : isRed ? 'red' : 'done',
        cls: leg.class,
        lead: leadOf(leg.class),
      },
    ]
    if (leg.current) {
      for (const [j, w] of (leg.waypoints ?? []).entries()) {
        if (w.kind !== 'testFail') continue
        const loop = w.runs && w.runs > 1 ? ` · ${w.runs} legs` : ''
        rows.push({ key: `l${i}w${j}`, text: `  ✗ ${w.text}${loop}`, right: '', tone: 'red' })
      }
    }
    items.push({ at: ms(leg.start), order: 1, rows })
  })

  snap.branches.forEach((b, i) => {
    const rows: Row[] = [
      {
        key: `b${i}`,
        text: `├─◈ ${b.label}`,
        right: b.done && b.end ? `✓ ${ago(b.end, snap.now)} ago` : `⋯ ${ago(b.start, snap.now)} out`,
        tone: 'lane',
      },
    ]
    if (b.done && b.report) rows.push({ key: `b${i}r`, text: `│   ${b.report}`, right: '', tone: 'note' })
    items.push({ at: ms(b.start), order: 2, rows })
  })

  items.sort((a, b) => a.at - b.at || a.order - b.order)
  const rows = items.flatMap(item => item.rows)

  for (const task of snap.tasks) {
    if (task.status === 'completed') continue
    const now = task.status === 'in_progress' ? ' (now)' : ''
    rows.push({ key: `t${task.id}`, text: `◌ ${task.subject}${now}`, right: '', tone: 'ghost' })
  }
  return rows
}

export type BandPart = { key: string; text: string; tone: 'red' | 'good' | 'note'; cls?: string; lead?: number }

/**
 * The line above the prompt: the latest test run (and whether code moved
 * since), the latest ship, the agents still out. Empty when none has
 * anything to say, and the band then draws nothing.
 */
export function band(snap: Snapshot, labels: Labels = {}): BandPart[] {
  const parts: BandPart[] = []
  const out = snap.outcome

  if (out?.kind === 'testRun') {
    const isRed = (out.short ?? out.text).includes('✗') || /fail/.test(out.text)
    // A write after the run, by its own clock where compass gives one: a
    // leg's class says what most of it was, and one shell write in a
    // scout leg is still an edit.
    const edited = snap.lastEdit
      ? ms(snap.lastEdit) > ms(out.at)
      : snap.legs.some(l => ['build', 'fix', 'docs'].includes(l.class) && ms(l.start) > ms(out.at))
    parts.push({
      key: 'test',
      cls: 'test',
      lead: 6,
      text: `◆ test ${isRed ? 'red' : 'green'} ${out.short ?? out.text}${edited ? ' · edited since' : ''}`,
      tone: isRed ? 'red' : 'good',
    })
  }
  if (out?.kind === 'commit') {
    parts.push({ key: 'ship', text: `✓ shipped ${ago(out.at, snap.now)} ago`, tone: 'good' })
  }

  const outLanes = snap.branches.filter(b => !b.done)
  if (outLanes.length > 0) {
    const oldest = outLanes.reduce((a, b) => (ms(a.start) <= ms(b.start) ? a : b))
    parts.push({ key: 'lanes', text: `◈${outLanes.length} out · oldest ${ago(oldest.start, snap.now)}`, tone: 'note' })
  }

  // HEAD always leads once there is a leg: the band is where the trail is
  // seen when no pane is seated.
  const head = snap.legs.at(-1)
  if (head !== undefined) {
    const glyph = head.current ? '●' : '◆'
    const legs = `${snap.legs.length} leg${snap.legs.length === 1 ? '' : 's'}`
    parts.unshift({
      key: 'head',
      text: `${glyph} ${head.class} ${labelOf(head, labels)}`.trimEnd() + ` · ${legs}`,
      tone: 'note',
      cls: head.class,
      lead: 2 + head.class.length,
    })
  }
  return parts
}

/** Takes complete lines off the front of `buffer`: the lines, and what is left. */
export function lines(buffer: string): { done: string[]; rest: string } {
  const cut = buffer.lastIndexOf('\n')
  if (cut < 0) return { done: [], rest: buffer }
  return { done: buffer.slice(0, cut).split('\n').filter(Boolean), rest: buffer.slice(cut + 1) }
}

/**
 * The pane as plain text, for a session no surface draws: the block, a
 * blank line, the last `room` trail rows, each right column aligned.
 */
export function text(snap: Snapshot, labels: Labels = {}, room = 40, width = 72): string {
  const fmt = (r: Row) => {
    const gap = Math.max(1, width - r.text.length - r.right.length)
    return r.right ? r.text + ' '.repeat(gap) + r.right : r.text
  }
  const head = block(snap).map(fmt)
  const body = trail(snap, labels)
  const shown = body.slice(-room)
  const cut = body.length > shown.length ? [`↑ ${body.length - shown.length} earlier`] : []
  const strip = band(snap, labels).map(p => p.text).join('  ·  ')
  return [...head, ...(head.length ? [''] : []), ...cut, ...shown.map(fmt), ...(strip ? ['', strip] : [])].join('\n')
}
