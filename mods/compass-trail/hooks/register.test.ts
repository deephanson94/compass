import type { RenderElement } from 'claude-code'
import { expect, test } from 'claude-code/testing'

import type { Snapshot } from '../types'
import { digests, legKey, parse } from './narrate'
import { band, lines, text, trail } from './view'

const T0 = '2026-08-30T12:00:00Z'
const at = (min: number) => new Date(Date.parse(T0) + min * 60_000).toISOString()

const SNAP: Snapshot = {
  transcript: '/x.jsonl',
  now: at(20),
  prompts: [{ text: 'fix the auth tests', at: at(0) }],
  legs: [
    { class: 'scout', label: 'auth.py', start: at(1), end: at(2) },
    {
      class: 'test',
      label: 'pytest',
      start: at(3),
      end: at(4),
      waypoints: [
        { kind: 'testRun', text: '18 passed · 2 failed', short: '18✓ 2✗', at: at(4) },
        { kind: 'testFail', text: 'test_expiry', at: at(4), runs: 2 },
      ],
    },
    { class: 'fix', label: 'refresh.py', start: at(5), end: at(18), current: true },
  ],
  branches: [{ label: 'map the payments module', start: at(6), done: false, afterLeg: 2 }],
  tasks: [{ id: '1', subject: 'Run the full suite', status: 'pending' }],
  outcome: { kind: 'testRun', text: '18 passed · 2 failed', short: '18✓ 2✗', at: at(4) },
  counts: { scout: { legs: 1, seconds: 60 }, test: { legs: 1, seconds: 60, red: 1 }, fix: { legs: 1, seconds: 780 } },
}

test('the trail reads oldest first, HEAD marked, the plan as ghosts', async () => {
  const rows = trail(SNAP)
  expect(rows.map(r => r.tone)).toEqual(['prompt', 'done', 'red', 'head', 'lane', 'ghost'])
  expect(rows[3]?.text).toContain('● fix')
  expect(rows[3]?.right).toBe('← 15m')
  expect(rows[4]?.right).toBe('⋯ 14m out')
})

test('the band says the run, that code moved since, and the lane out', async () => {
  const text = band(SNAP).map(p => p.text).join(' | ')
  expect(text).toBe('● fix refresh.py · 3 legs | ◆ test red 18✓ 2✗ · edited since | ◈1 out · oldest 14m')
  expect(band({ ...SNAP, legs: [], outcome: undefined, branches: [] })).toEqual([])
})

test('as text, the trail ends on HEAD and the band line', async () => {
  const out = text(SNAP, {}, 40, 50).split('\n')
  expect(out.at(-3)).toContain('◌ Run the full suite')
  expect(out.at(-1)).toContain('◆ test red 18✓ 2✗')
  expect(out.find(l => l.startsWith('● fix'))?.endsWith('← 15m')).toBe(true)
})

test('the narrator asks about closed unnamed legs, newest first, and keeps only fitting answers', async () => {
  const asked = digests(SNAP, { [legKey(SNAP.legs[0]!)]: 'mapped auth' }, new Set())
  expect(asked.map(d => d.class)).toEqual(['test'])
  expect(asked[0]?.prompt).toBe('fix the auth tests')

  const key = asked[0]!.key
  const reply = '```json\n[{"key":"' + key + '","label":"ran the auth suite."},{"key":"other","label":"x"}]\n```'
  expect(parse(reply, asked)).toEqual({ [key]: 'ran the auth suite' })
  expect(parse('[{"key":"' + key + '","label":"test"}]', asked)).toEqual({})
  expect(parse('no json here', asked)).toEqual({})
})

test('a narrated label replaces the heuristic one on a closed leg', async () => {
  const rows = trail(SNAP, { [legKey(SNAP.legs[1]!)]: 'ran the auth suite' })
  expect(rows[2]?.text).toBe('◆ test   18✓ 2✗ ran the auth suite')
})

test('a chunk cut mid-line keeps the tail for the next one', async () => {
  expect(lines('{"a":1}\n{"b"')).toEqual({ done: ['{"a":1}'], rest: '{"b"' })
})

test('the band draws what compass streams, on every surface that has one', async ($, on) => {
  // Beneath the mod stands what the engine would do: a session id, and a
  // compass that writes one snapshot cut across two pieces, then exits.
  on('session.start', async (_, e) => ({ cwd: e.cwd }))
  on('session.id', async () => ({ value: 'sess-1' }))
  on('session.surfaces', async () => ({ value: ['terminal'] as const }))
  on('fs.write', async () => ({ value: undefined }))
  on('command.register', async (_, e) => ({ value: { command: e.name } }))
  on('ui.open', async () => ({ value: { isPlaced: true } as const }))
  on('ui.render', async () => h('Box', {}) as RenderElement)
  let narrated!: () => void
  const isNarrated = new Promise<void>(resolve => (narrated = resolve))
  // The labels landing is the narration done: wait on the write itself.
  on('state.set', async ($, e, next) => {
    const set = await next(e)
    if (JSON.stringify(e).includes('"labels"')) narrated()
    return set
  })
  on('model.complete', async (_, e) => {
    const asked = JSON.parse(String(e.prompt).slice(String(e.prompt).indexOf('['))) as { key: string }[]
    const text = JSON.stringify(asked.map(d => ({ key: d.key, label: `named ${d.key.split('/')[1]}` })))
    const usage = { input_tokens: 1, output_tokens: 1, cache_read_input_tokens: 0, cache_creation_input_tokens: 0 }
    return { value: { isAnswered: true as const, text, usage } }
  })
  let streamed!: () => void
  const isStreamed = new Promise<void>(resolve => (streamed = resolve))
  on('process.spawn', async function* () {
    const json = JSON.stringify(SNAP) + '\n'
    yield { stream: 'stdout' as const, text: json.slice(0, 40) }
    yield { stream: 'stdout' as const, text: json.slice(40) }
    streamed()
    return { value: { code: 0, signal: null } }
  })
  await $.session.start({ cwd: '/', surface: 'terminal', isInteractive: true })
  await isStreamed
  await isNarrated

  for (const surface of ['terminal', 'desktop'] as const) {
    const ui = await $.ui.mount({
      plugin: 'compass-trail',
      surface,
      component: 'AbovePrompt',
      props: { hasSurvey: false, isWorking: true, maxRows: 4, bodyColumns: 100, scroll: { offset: 0, bodyRows: 4 }, view: {} },
    })
    const found = await ui.find({ type: 'Text', text: /test red/ })
    expect(found?.text).toContain('18✓ 2✗')
    await ui.unmount()

    const pane = await $.ui.mount({
      plugin: 'compass-trail',
      surface,
      component: 'Pane',
      requestId: 'compass-trail',
      props: { title: 'Trail', isFocused: false, bodyColumns: 40, placement: 'dock', scroll: { offset: 0, bodyRows: 20 }, view: {} },
      viewport: { columns: 40, rows: 24 },
    })
    expect((await pane.find({ type: 'Text', text: /^● fix/ }))?.text).toContain('refresh.py')
    expect(await pane.find({ type: 'Text', text: /◌ Run the full suite/ })).toBeDefined()
    expect(await pane.find({ type: 'Text', text: /scout +named scout/ })).toBeDefined()
    await pane.unmount()
  }
})
