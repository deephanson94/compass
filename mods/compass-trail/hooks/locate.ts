// Finding a compass that speaks the trail protocol this mod reads. Every
// candidate is asked `-version` first: it is the one question every compass
// answers without starting anything, and an older compass handed `trail`
// would open its full-screen deck instead of answering.

/** The snapshot shape this mod reads (cmd/compass/trail.go, trailProtocol). */
export const PROTOCOL = 1

/**
 * Where to look, in order: what the person named (the mod's setting, then
 * COMPASS_BIN, e.g. a server-wide one in /etc/environment), the binary the
 * mod ships, PATH, then the places the README's installs put it, which a
 * desktop app's PATH often lacks.
 */
export function candidates(root: string, home: string | undefined, named: readonly (string | undefined)[]): string[] {
  const out = [...named.filter((p): p is string => !!p && p.trim() !== '').map(p => p.trim()), `${root}/bin/compass`, 'compass']
  if (home) out.push(`${home}/.local/bin/compass`, `${home}/go/bin/compass`)
  out.push('/usr/local/bin/compass', '/opt/homebrew/bin/compass')
  return [...new Set(out)]
}

export type Verdict = { ok: true; version: string } | { ok: false; why: string }

/** Reads a `-version` answer: "compass v0.4.0 (trail 1)". */
export function judge(exitCode: number, stdout: string): Verdict {
  const line = stdout.trim().split('\n')[0] ?? ''
  if (exitCode !== 0 || !line.startsWith('compass ')) return { ok: false, why: 'not a compass binary' }
  const version = line.split(' ')[1] ?? '?'
  const m = /\(trail (\d+)\)/.exec(line)
  if (!m) return { ok: false, why: `compass ${version} is older than the trail subcommand` }
  const n = Number(m[1])
  if (n !== PROTOCOL) return { ok: false, why: `compass ${version} speaks trail ${n}, this mod reads trail ${PROTOCOL}` }
  return { ok: true, version }
}
