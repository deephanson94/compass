import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { Snapshot } from '../types'
import { candidates, judge } from './locate'
import { digests, INSTRUCTION, parse } from './narrate'
import { classColor, DIM, isLightTheme, pick, STUCK, WORKING } from './palette'
import { band, block, lines, text, trail } from './view'
import type { BandPart, Row } from './view'

const PANE = 'compass-trail'
const TITLE = 'Trail'

const snap = atom({ plugin: 'compass-trail', key: 'snap' } as const, null)
const problem = atom({ plugin: 'compass-trail', key: 'problem' } as const, null)
const isBandHidden = atom({ plugin: 'compass-trail', key: 'isBandHidden' } as const, false)
const labels = atom({ plugin: 'compass-trail', key: 'labels' } as const, {})
// Which of the deck's two palettes to draw with: Claude Code's own theme
// setting, read at start and followed when /config changes it.
const isLight = atom({ plugin: 'compass-trail', key: 'isLight' } as const, false)

// One narration in flight at a time; the keys the last failed batch asked
// about sit out the next one, so a broken call is not repeated every snapshot.
let isNarrating = false
let cooling = new Set<string>()

/** Names the snapshot's closed, unnamed legs in one model call. */
async function narrate($: EngineInterface, s: Snapshot): Promise<void> {
  if (isNarrating) return
  const skip = cooling
  cooling = new Set()
  const batch = digests(s, await read($, labels), skip)
  if (batch.length === 0) return
  isNarrating = true
  try {
    const r = await $.model.complete({
      model: 'haiku',
      prompt: INSTRUCTION + JSON.stringify(batch),
      maxTokens: 2000,
      timeoutMs: 60_000,
    })
    const named = r.isAnswered ? parse(r.text, batch) : {}
    trace($, `narrate asked=${batch.length} named=${Object.keys(named).length}${r.isAnswered ? '' : ` reason=${r.reason}`}`)
    if (Object.keys(named).length > 0) await update($, labels, was => ({ ...was, ...named }))
    for (const d of batch) if (!(d.key in named)) cooling.add(d.key)
  } catch (err) {
    trace($, `narrate failed: ${String(err)}`)
    for (const d of batch) cooling.add(d.key)
  } finally {
    isNarrating = false
  }
}

// Diagnostics while the mod is young: what the engine and its clients asked
// of it, written beside the mod so a session can read why nothing shows.
const seen: string[] = []
// A surface's first draw of each component is worth a line; every redraw after
// it would rewrite the file on each snapshot.
const drawn = new Set<string>()
function traceFirstDraw($: EngineInterface, what: string): void {
  if (drawn.has(what)) return
  drawn.add(what)
  trace($, what)
}

function trace($: EngineInterface, what: string): void {
  seen.push(`${new Date().toISOString()} ${what}`)
  void $.fs.write(`${$.plugin.root}/diag.log`, seen.slice(-200).join('\n') + '\n').catch(() => {})
}

// PROBE (temporary): does $.session.messages() shrink the moment a rewind
// point is picked, or only when the next prompt is sent? An immediate trail
// update after /rewind rests on the first. Each line says how many messages
// the session holds and what the last one is; diag.log gets a line on every
// slash command, and whenever the count changes.
let probed = ''
async function probe($: EngineInterface, why: string, always = false): Promise<void> {
  try {
    const ms = await $.session.messages()
    const last = ms.at(-1)
    const tools = last?.toolUses?.map(u => u.tool_use_id).join(',') ?? ''
    const sum = `n=${ms.length} last=${last?.role ?? '-'}:${JSON.stringify((last?.text ?? '').slice(0, 40))}${tools ? ` tools=${tools}` : ''}`
    if (!always && sum === probed) return
    probed = sum
    trace($, `probe ${why} ${sum}`)
  } catch (err) {
    trace($, `probe ${why} failed: ${String(err)}`)
  }
}

/**
 * A row's colours under the deck's rules: the class hue on the glyph and
 * verb only, a prompt bold and uncoloured, a failing test's name in the
 * alarm red, the quiet rows in the neutral grey.
 */
function rowPaint(row: Row, light: boolean) {
  const dim = pick(DIM, light)
  const lead = row.lead ?? 0
  const lit = row.tone === 'ghost' || row.tone === 'note' || row.tone === 'lane' ? dim : row.cls === undefined && row.tone === 'red' ? pick(STUCK, light) : undefined
  return {
    lead: row.text.slice(0, lead),
    rest: row.text.slice(lead),
    leadColor: classColor(row.cls, light),
    restColor: lit,
    isBold: row.tone === 'head' || row.tone === 'prompt',
    dim,
  }
}

function partPaint(part: BandPart, light: boolean) {
  const lead = part.lead ?? 0
  const tone = { red: pick(STUCK, light), good: pick(WORKING, light), note: pick(DIM, light) }[part.tone]
  return { lead: part.text.slice(0, lead), rest: part.text.slice(lead), leadColor: classColor(part.cls, light) ?? tone, restColor: tone }
}

/**
 * The first candidate that answers `-version` with the trail protocol this
 * mod reads, or why none did: a missing file and a too-old compass are told
 * apart, so the message says which to fix.
 */
async function locate($: EngineInterface, configured: string | undefined): Promise<{ bin: string } | { why: string }> {
  const home = await $.env.get('HOME')
  const fromEnv = await $.env.get('COMPASS_BIN')
  const tooOld: string[] = []
  for (const bin of candidates($.plugin.root, home, [configured, fromEnv])) {
    try {
      const r = await $.process.run([bin, '-version'], { timeoutMs: 5000 })
      const v = judge(r.exitCode, r.stdout)
      trace($, `locate ${bin}: ${v.ok ? `ok ${v.version}` : v.why}`)
      if (v.ok) return { bin }
      tooOld.push(`${bin}: ${v.why}`)
    } catch {
      // Not there, or not runnable: the next place, without a word.
    }
  }
  if (tooOld.length > 0) return { why: `${tooOld.join('; ')}. Install a newer compass` }
  return {
    why: 'no compass found. Set the mod\'s compass path, or COMPASS_BIN, to the binary, or put it on PATH (e.g. /usr/local/bin)',
  }
}

/**
 * Follows this session's transcript through `compass trail -follow`. The
 * loop is the child's life; a reload of the module kills it and
 * session.start starts the next.
 */
async function follow($: EngineInterface, configured: string | undefined): Promise<void> {
  const found = await locate($, configured)
  if ('why' in found) {
    await update($, problem, () => found.why)
    return
  }
  const session = await $.session.id()
  let buffer = ''
  try {
    const child = $.process.spawn({ argv: [found.bin, 'trail', '-session', session, '-follow'] })
    for await (const chunk of child) {
      if (chunk.stream === 'stderr') {
        await update($, problem, () => chunk.text.trim())
        continue
      }
      const { done, rest } = lines(buffer + chunk.text)
      buffer = rest
      const latest = done.at(-1)
      if (latest === undefined) continue
      const next = JSON.parse(latest) as Snapshot
      await update($, snap, () => next)
      await update($, problem, () => null)
      void narrate($, next)
    }
    await update($, problem, () => `${found.bin} trail stopped`)
  } catch (err) {
    await update($, problem, () => `${found.bin} trail failed: ${String(err)}`)
  }
}

export const register: Register = (on, options) => {
  const configured = typeof options.compassPath === 'string' ? options.compassPath : undefined

  on('session.start', async ($, e, next) => {
    const started = await next(e)
    const theme = (await $.config.list()).find(row => row.key === 'theme')?.value
    await update($, isLight, () => isLightTheme(theme))
    trace($, `session.start theme=${String(theme)} surface=${e.surface} surfaces=${(await $.session.surfaces()).join(',') || 'none'}`)
    await $.command.register({ name: 'trail', description: 'Show this session’s compass trail in a pane' })
    await $.command.register({ name: 'trail-band', description: 'Show or hide the compass line above the prompt' })
    void follow($, configured)
    // PROBE: a double-Esc rewind runs no command; a slow watch catches it.
    try {
      $.clock.every(3000, () => void probe($, 'tick'))
    } catch (err) {
      trace($, `probe tick unavailable: ${String(err)}`)
    }
    // A pane opened unasked waits on a narrow screen or a surface that seats
    // none; say which, once, so a missing pane is never a mystery.
    const opened = await $.ui.open({ id: PANE, title: TITLE })
    trace($, `ui.open isPlaced=${opened.isPlaced}${opened.isPlaced ? '' : ` reason=${opened.reason}`}`)
    $.ui.log(
      opened.isPlaced
        ? `Trail pane open (${e.surface ?? 'no surface'})`
        : `Trail pane waits: ${opened.reason}. /trail opens it`,
    )
    return started
  })

  on('command.run', { command: 'trail' }, async $ => {
    const opened = await $.ui.open({ id: PANE, title: TITLE })
    // Which clients draw this session: a pane is only seen on one of these.
    const surfaces = (await $.session.surfaces()).join(', ') || 'none'
    trace($, `/trail isPlaced=${opened.isPlaced} surfaces=${surfaces}`)
    // No client draws mod UI here (a cloud session viewed from an app that
    // does not attach): the command's own reply is the one place it shows.
    if (surfaces === 'none') {
      const s = await read($, snap)
      return { text: s === null ? 'no trail read yet.' : '\n' + text(s, await read($, labels)) }
    }
    const state = opened.isPlaced ? 'Trail pane open.' : `Trail pane waits: ${opened.reason}.`
    return { text: `${state} Drawing on: ${surfaces}.` }
  })

  // PROBE: every slash command, before and after it runs; after /rewind,
  // twice a second for a minute, so the log shows when the picked point
  // reaches the session's messages.
  on('command.run', async ($, e, next) => {
    await probe($, `/${e.command} before`, true)
    const ran = await next(e)
    await probe($, `/${e.command} after`, true)
    if (e.command === 'rewind') {
      void (async () => {
        for (let i = 0; i < 120; i++) {
          await $.clock.sleep(500)
          await probe($, `/rewind +${(i + 1) / 2}s`)
        }
      })()
    }
    return ran
  })

  on('config.set', async ($, e, next) => {
    const set = await next(e)
    if (e.key === 'theme') await update($, isLight, () => isLightTheme(e.value))
    return set
  })

  on('session.attach', async ($, e, next) => {
    const joined = await next(e)
    trace($, `session.attach surface=${e.surface} client=${e.clientId}`)
    $.ui.log(`a ${e.surface} client attached`)
    return joined
  })

  on('command.run', { command: 'trail-band' }, async $ => {
    const hidden = await update($, isBandHidden, was => !was)
    return { text: hidden ? 'Compass line hidden.' : 'Compass line shown.' }
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    traceFirstDraw($, `render Pane surface=${e.surface} placement=${e.props.placement}`)
    const { Box, Text } = $.ui.resolve(e)
    const s = await read($, snap)
    const why = await read($, problem)
    const light = await read($, isLight)
    const grey = pick(DIM, light)

    if (s === null) {
      return (
        <Box flexDirection="column">
          <Text color={grey}>{why ?? 'Reading the transcript…'}</Text>
        </Box>
      )
    }

    const head = block(s)
    const body = trail(s, await read($, labels))
    const room = Math.max(3, (e.viewport?.rows ?? 24) - 4 - head.length - (head.length ? 1 : 0))
    const shown = body.slice(-room)
    const line = (row: Row) => {
      const p = rowPaint(row, light)
      return (
        <Box key={row.key} flexDirection="row" justifyContent="space-between">
          <Box flexDirection="row" flexShrink={1}>
            {p.lead !== '' && (
              <Text color={p.leadColor} bold={p.isBold}>
                {p.lead}
              </Text>
            )}
            <Text wrap="truncate-end" color={p.restColor} bold={p.isBold}>
              {p.rest}
            </Text>
          </Box>
          <Text color={p.dim}>{row.right ? ` ${row.right}` : ''}</Text>
        </Box>
      )
    }

    return (
      <Box flexDirection="column">
        {head.map(line)}
        {head.length > 0 && <Text color={grey}>{' '}</Text>}
        {body.length === 0 && <Text color={grey}>No legs yet: the trail starts with the first tool call.</Text>}
        {body.length > shown.length && <Text color={grey}>{`↑ ${body.length - shown.length} earlier`}</Text>}
        {shown.map(line)}
        {why !== null && <Text color="warning">{why}</Text>}
      </Box>
    )
  })

  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    traceFirstDraw($, `render AbovePrompt surface=${e.surface}`)
    const s = await read($, snap)
    if (s === null || e.props.hasSurvey || (await read($, isBandHidden))) return next(e)
    const parts = band(s, await read($, labels))
    if (parts.length === 0) return next(e)

    const { Box, Text } = $.ui.resolve(e)
    const light = await read($, isLight)
    return (
      <Box flexDirection="row" flexWrap="wrap">
        {parts.map((part: BandPart, i) => {
          const p = partPaint(part, light)
          return (
            <Box key={part.key} flexDirection="row">
              {i > 0 && <Text color={pick(DIM, light)}>{'  ·  '}</Text>}
              {p.lead !== '' && <Text color={p.leadColor}>{p.lead}</Text>}
              <Text wrap="truncate-end" color={p.restColor}>
                {p.rest}
              </Text>
            </Box>
          )
        })}
      </Box>
    )
  })
}
