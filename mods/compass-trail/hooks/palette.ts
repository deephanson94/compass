// The deck's colours (internal/ui/styles.go), value for value, so the mod and
// the deck read alike. One cool hue per leg class tints the glyph and the
// verb only; the warm end belongs to the alarms; greys are neutral (r=g=b)
// so they stay grey on every terminal. A prompt is never coloured.

export type Shade = { light: string; dark: string }

export const CLASS: Readonly<Record<string, Shade>> = {
  scout: { light: '#0e7490', dark: '#22d3ee' }, // cyan — looking around
  design: { light: '#6d28d9', dark: '#a78bfa' }, // violet — thinking
  build: { light: '#1d4ed8', dark: '#60a5fa' }, // blue — making
  fix: { light: '#a21caf', dark: '#e879f9' }, // fuchsia — repairing
  test: { light: '#0f766e', dark: '#2dd4bf' }, // teal — checking
  ship: { light: '#4d7c0f', dark: '#a3e635' }, // lime — landing it
  docs: { light: '#475569', dark: '#94a3b8' }, // slate — writing it down
}

export const DIM: Shade = { light: '#6e6e6e', dark: '#9a9a9a' }
export const WORKING: Shade = { light: '#15803d', dark: '#4ade80' }
export const STUCK: Shade = { light: '#b91c1c', dark: '#f87171' }

/** One shade for the theme in use; a light theme is any Claude Code calls light. */
export const pick = (shade: Shade, isLight: boolean) => (isLight ? shade.light : shade.dark)

/** The class's colour, or none for a class the deck has no hue for. */
export const classColor = (cls: string | undefined, isLight: boolean) => {
  const shade = cls === undefined ? undefined : CLASS[cls]
  return shade === undefined ? undefined : pick(shade, isLight)
}

/** Whether a Claude Code theme setting names a light theme ("light", "light-daltonized", ...). */
export const isLightTheme = (theme: unknown) => typeof theme === 'string' && theme.toLowerCase().includes('light')
