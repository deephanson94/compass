// The narrator: names closed legs the heuristic left blank or vague, the way
// the deck's does (internal/narrator), through the session's own model access
// instead of a `claude -p` of its own. Each leg is named once; HEAD is still
// changing and keeps its heuristic label until it closes.

import type { Leg, Snapshot } from '../types'

/** A leg's identity: its start never moves once it is closed. */
export const legKey = (leg: Leg) => `${leg.start}/${leg.class}`

/** At most this many legs per call: a batch stays one quick, cheap answer. */
export const BATCH = 20

/** How wide a label may read, as the deck's: one row beside the class. */
const LABEL = 32

export type Digest = {
  key: string
  class: string
  label: string
  files: string[]
  waypoints: string[]
  acts: string[]
  prompt: string
}

// The deck's instruction (internal/narrator/runner.go), plus the acts.
export const INSTRUCTION = `You name units of coding work.

Below is a JSON array of legs from one Claude Code session. Each leg has a key,
a class of work, the heuristic label it currently shows, the files it touched,
its waypoints (test runs, bugs, commits), its acts (the tool calls it made, one
line each) and the user prompt it came from.

Return ONLY a JSON array of {"key","label"} objects, one per input leg, in the
same order. No prose, no explanation, no code fences.

Rules for each label:
- at most 5 words
- lowercase, unless a word is a proper noun
- name the work that was done, not the class it belongs to

LEGS:
`

/**
 * The batch to ask about: closed legs with no name yet and not held back,
 * newest first, since those are the rows on screen.
 */
export function digests(snap: Snapshot, named: Record<string, string>, skip: ReadonlySet<string>): Digest[] {
  const out: Digest[] = []
  for (let i = snap.legs.length - 1; i >= 0 && out.length < BATCH; i--) {
    const leg = snap.legs[i]!
    const key = legKey(leg)
    if (leg.current || key in named || skip.has(key)) continue
    const asked = snap.prompts.filter(p => Date.parse(p.at) <= Date.parse(leg.start)).at(-1)
    out.push({
      key,
      class: leg.class,
      label: leg.label,
      files: leg.files ?? [],
      waypoints: (leg.waypoints ?? []).map(w => w.text),
      acts: leg.acts ?? [],
      prompt: (asked?.text ?? '').slice(0, 120),
    })
  }
  return out
}

/**
 * The model's answer as labels by key: only keys that were asked, only
 * labels that fit, never a bare class name (which says nothing the class
 * column does not).
 */
export function parse(reply: string, asked: readonly Digest[]): Record<string, string> {
  const start = reply.indexOf('[')
  const end = reply.lastIndexOf(']')
  if (start < 0 || end <= start) return {}
  let rows: unknown
  try {
    rows = JSON.parse(reply.slice(start, end + 1))
  } catch {
    return {}
  }
  if (!Array.isArray(rows)) return {}
  const classOf = new Map(asked.map(d => [d.key, d.class]))
  const out: Record<string, string> = {}
  for (const row of rows) {
    if (typeof row !== 'object' || row === null) continue
    const { key, label } = row as { key?: unknown; label?: unknown }
    if (typeof key !== 'string' || typeof label !== 'string' || !classOf.has(key)) continue
    const text = label.trim().replace(/\.$/, '')
    if (text === '' || text.toLowerCase() === classOf.get(key)) continue
    out[key] = [...text].length > LABEL ? [...text].slice(0, LABEL - 1).join('') + '…' : text
  }
  return out
}
