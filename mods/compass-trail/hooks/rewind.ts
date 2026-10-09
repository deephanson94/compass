// Seeing a rewind as it happens. /rewind (or a double Esc) writes nothing to
// the transcript until the next prompt, but the session's own messages
// shrink the moment a point is picked: the first message gone is the prompt
// rewound to. Measured on Claude Code 2.1.295: 25 messages before /rewind,
// 19 six seconds later, with no prompt sent in between.

import type { Cut } from '../types'

/** One message as the watch compares it: who, what, and which tool calls. */
export type Sig = string

export type Said = { role: string; text: string; toolUses?: readonly { tool_use_id: string }[] }

export const sig = (m: Said): Sig => `${m.role}\u0000${m.text}\u0000${(m.toolUses ?? []).map(u => u.tool_use_id).join(',')}`

/**
 * The rewind between two reads of the session's messages, if there was one:
 * the list got shorter and what is left is the old list's beginning, and the
 * first message gone is a prompt with words in it. A /compact shortens the
 * list too, but what is left is a summary, not the old beginning.
 */
export function rewindBetween(before: readonly Sig[], after: readonly Said[], at: string): Cut | null {
  const n = after.length
  if (n >= before.length) return null
  if (n > 0 && sig(after[n - 1]!) !== before[n - 1]) return null
  const [role, text] = before[n]!.split('\u0000')
  const prompt = (text ?? '').trim().split('\n')[0]?.trim() ?? ''
  if (role !== 'user' || prompt === '') return null
  return { at, prompt }
}
