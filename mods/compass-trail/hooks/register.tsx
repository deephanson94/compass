import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { Snapshot } from '../types'
import { band, block, lines, trail } from './view'
import type { BandPart, Row } from './view'

const PANE = 'compass-trail'
const TITLE = 'Trail'

const snap = atom({ plugin: 'compass-trail', key: 'snap' } as const, null)
const problem = atom({ plugin: 'compass-trail', key: 'problem' } as const, null)
const isBandHidden = atom({ plugin: 'compass-trail', key: 'isBandHidden' } as const, false)

const rowColor = { prompt: 'claude', done: undefined, head: 'text', red: 'error', lane: 'suggestion', ghost: 'inactive', note: 'subtle' } as const
const partColor = { red: 'error', good: 'success', note: 'subtle' } as const

/**
 * Follows this session's transcript through `compass trail -follow`: the
 * binary the mod ships first, then one on PATH. The loop is the child's life;
 * a reload of the module kills it and session.start starts the next.
 */
async function follow($: EngineInterface): Promise<void> {
  const session = await $.session.id()
  const tried: string[] = []

  for (const bin of [`${$.plugin.root}/bin/compass`, 'compass']) {
    let buffer = ''
    let sawOutput = false
    try {
      const child = $.process.spawn({ argv: [bin, 'trail', '-session', session, '-follow'] })
      for await (const chunk of child) {
        if (chunk.stream === 'stderr') {
          await update($, problem, () => chunk.text.trim())
          continue
        }
        sawOutput = true
        const { done, rest } = lines(buffer + chunk.text)
        buffer = rest
        const latest = done.at(-1)
        if (latest === undefined) continue
        const next = JSON.parse(latest) as Snapshot
        await update($, snap, () => next)
        await update($, problem, () => null)
      }
      if (sawOutput) return
      tried.push(`${bin}: exited without a snapshot`)
    } catch (err) {
      tried.push(`${bin}: ${String(err)}`)
    }
  }
  await update($, problem, () => `compass did not start (${tried.join('; ')})`)
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    const started = await next(e)
    await $.command.register({ name: 'trail', description: 'Show this session’s compass trail in a pane' })
    await $.command.register({ name: 'trail-band', description: 'Show or hide the compass line above the prompt' })
    void follow($)
    // A pane opened unasked waits on a narrow screen or a surface that seats
    // none; say which, once, so a missing pane is never a mystery.
    const opened = await $.ui.open({ id: PANE, title: TITLE })
    $.ui.log(
      opened.isPlaced
        ? `compass-trail: Trail pane open (${e.surface ?? 'no surface'})`
        : `compass-trail: Trail pane waits: ${opened.reason}. /trail opens it`,
    )
    return started
  })

  on('command.run', { command: 'trail' }, async $ => {
    const opened = await $.ui.open({ id: PANE, title: TITLE })
    return { text: opened.isPlaced ? 'Trail pane open.' : `Trail pane waits: ${opened.reason}` }
  })

  on('command.run', { command: 'trail-band' }, async $ => {
    const hidden = await update($, isBandHidden, was => !was)
    return { text: hidden ? 'Compass line hidden.' : 'Compass line shown.' }
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { Box, Text } = $.ui.resolve(e)
    const s = await read($, snap)
    const why = await read($, problem)

    if (s === null) {
      return (
        <Box flexDirection="column">
          <Text dimColor>{why ?? 'Reading the transcript…'}</Text>
        </Box>
      )
    }

    const head = block(s)
    const body = trail(s)
    const room = Math.max(3, (e.viewport?.rows ?? 24) - 4 - head.length - (head.length ? 1 : 0))
    const shown = body.slice(-room)
    const line = (row: Row) => (
      <Box key={row.key} flexDirection="row" justifyContent="space-between">
        <Text wrap="truncate-end" color={rowColor[row.tone]} dimColor={row.tone === 'ghost' || row.tone === 'note'} bold={row.tone === 'head'}>
          {row.text}
        </Text>
        <Text dimColor>{row.right ? ` ${row.right}` : ''}</Text>
      </Box>
    )

    return (
      <Box flexDirection="column">
        {head.map(line)}
        {head.length > 0 && <Text dimColor>{' '}</Text>}
        {body.length === 0 && <Text dimColor>No legs yet: the trail starts with the first tool call.</Text>}
        {body.length > shown.length && <Text dimColor>{`↑ ${body.length - shown.length} earlier`}</Text>}
        {shown.map(line)}
        {why !== null && <Text color="warning">{why}</Text>}
      </Box>
    )
  })

  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    const s = await read($, snap)
    if (s === null || e.props.hasSurvey || (await read($, isBandHidden))) return next(e)
    const parts = band(s)
    if (parts.length === 0) return next(e)

    const { Box, Text } = $.ui.resolve(e)
    return (
      <Box flexDirection="row" flexWrap="wrap">
        {parts.map((part: BandPart, i) => (
          <Text key={part.key} wrap="truncate-end" color={partColor[part.tone]} dimColor={part.tone === 'note'}>
            {(i > 0 ? '  ·  ' : '') + part.text}
          </Text>
        ))}
      </Box>
    )
  })
}
