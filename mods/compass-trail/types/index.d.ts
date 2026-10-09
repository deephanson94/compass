// The snapshot `compass trail -follow` writes, one per line. Its Go side is
// cmd/compass/trail.go in the compass repository; the two change together.

export type Waypoint = {
  kind: 'testRun' | 'testFail' | 'bug' | 'commit' | 'unknown'
  text: string
  short?: string
  at: string
  runs?: number
}

export type Leg = {
  class: string
  label: string
  start: string
  end: string
  current?: boolean
  files?: string[]
  waypoints?: Waypoint[]
  acts?: string[]
}

export type Branch = {
  label: string
  start: string
  end?: string
  done: boolean
  afterLeg: number
  report?: string
}

export type Prompt = { text: string; at: string; relayed?: boolean; teammate?: string }

export type Task = { id: string; subject: string; active?: string; status: string }

export type Outcome = { kind: Waypoint['kind']; text: string; short?: string; at: string }

export type Sum = { legs: number; seconds: number; red?: number }

export type Snapshot = {
  transcript: string
  now: string
  prompts: Prompt[]
  legs: Leg[]
  branches: Branch[]
  tasks: Task[]
  outcome?: Outcome
  /** The main thread's latest write: an edit tool or a shell write. */
  lastEdit?: string
  counts: Record<string, Sum>
}

declare module 'claude-code' {
  interface PluginState {
    'compass-trail': {
      snap: Snapshot | null
      problem: string | null
      isBandHidden: boolean
      labels: Record<string, string>
      isLight: boolean
    }
  }
}
