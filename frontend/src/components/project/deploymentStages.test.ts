import { describe, expect, it } from 'vitest'

import {
  DEPLOY_STEPS,
  formatDuration,
  getActiveStep,
  getFailureMessage,
  getLiveMessage,
  getStepStates,
  stepFromProgress,
  deriveStepDurations,
} from './deploymentStages'
import type { Project } from '@/types'

/** Every value the backend enum can hold (shared/models/models.go). */
const ALL_STATUSES = [
  'queued', 'preparing', 'cloning', 'building', 'starting',
  'healthchecking', 'migrating', 'promoting', 'cleanup', 'completed',
  'failed', 'rollback', 'cancelled',
] as const

function project(overrides: Partial<Project> = {}) {
  return { deployment_status: 'building', ...overrides } as Project
}

describe('getActiveStep', () => {
  it('walks the steps in the order the worker emits them', () => {
    // Progress values are the ones deployment_worker.go actually writes.
    expect(getActiveStep('queued', 0)).toBe(0)
    expect(getActiveStep('preparing', 10)).toBe(0)
    expect(getActiveStep('cloning', 20)).toBe(0)
    expect(getActiveStep('building', 35)).toBe(1)
    expect(getActiveStep('starting', 50)).toBe(2)
    expect(getActiveStep('healthchecking', 65)).toBe(2)
    expect(getActiveStep('migrating', 75)).toBe(3)
    expect(getActiveStep('promoting', 85)).toBe(3)
    expect(getActiveStep('cleanup', 95)).toBe(3)
    expect(getActiveStep('completed', 100)).toBe(DEPLOY_STEPS.length)
  })

  it('falls back to progress when the status says nothing about position', () => {
    // `failed` loses the stage, but the worker preserves the last progress it wrote.
    expect(getActiveStep('failed', 35)).toBe(1)
    expect(getActiveStep('rollback', 65)).toBe(2)
    expect(getActiveStep('cancelled', 85)).toBe(3)
  })

  it('never returns an out-of-range index for any backend status', () => {
    for (const status of ALL_STATUSES) {
      const step = getActiveStep(status, 0)
      expect(step).toBeGreaterThanOrEqual(0)
      expect(step).toBeLessThanOrEqual(DEPLOY_STEPS.length)
    }
  })

  it('treats an unknown status as a progress reading', () => {
    expect(getActiveStep('some-future-stage', 75)).toBe(3)
    expect(getActiveStep(undefined, 35)).toBe(1)
  })
})

describe('stepFromProgress', () => {
  it('maps each emitted progress value into its step', () => {
    expect(stepFromProgress(0)).toBe(0)
    expect(stepFromProgress(20)).toBe(0)
    expect(stepFromProgress(35)).toBe(1)
    expect(stepFromProgress(50)).toBe(2)
    expect(stepFromProgress(75)).toBe(3)
    expect(stepFromProgress(100)).toBe(DEPLOY_STEPS.length)
  })
})

describe('getStepStates', () => {
  it('completes everything behind the active step', () => {
    expect(getStepStates('deploying', 2)).toEqual(['complete', 'complete', 'active', 'pending'])
  })

  it('marks the active step failed rather than the whole pipeline', () => {
    expect(getStepStates('failed', 1)).toEqual(['complete', 'failed', 'pending', 'pending'])
  })

  it('completes every step once the project is ready', () => {
    expect(getStepStates('ready', 0)).toEqual(['complete', 'complete', 'complete', 'complete'])
  })

  it('distinguishes a stalled worker from an active one', () => {
    expect(getStepStates('deploying', 1, true)).toEqual(['complete', 'stalled', 'pending', 'pending'])
  })
})

describe('formatDuration', () => {
  it('drops the minute component under a minute', () => {
    expect(formatDuration(0)).toBe('0s')
    expect(formatDuration(59)).toBe('59s')
  })

  it('pads seconds so the width stays stable while ticking', () => {
    expect(formatDuration(60)).toBe('1m 00s')
    expect(formatDuration(134)).toBe('2m 14s')
  })

  it('switches to hours past 60 minutes', () => {
    expect(formatDuration(3_600)).toBe('1h 00m')
    expect(formatDuration(3_900)).toBe('1h 05m')
  })

  it('clamps negatives, which a clock skew between fetches can produce', () => {
    expect(formatDuration(-5)).toBe('0s')
  })
})

describe('getLiveMessage', () => {
  it('shows the message of whichever stage is running', () => {
    expect(getLiveMessage(project({ deployment_message: 'Commit a1b2c3d: fix rounding' })))
      .toBe('Commit a1b2c3d: fix rounding')
    expect(getLiveMessage(project({
      deployment_status: 'cloning',
      deployment_message: 'Cloning https://github.com/example/app (main)',
    }))).toBe('Cloning https://github.com/example/app (main)')
  })

  it('stays quiet on terminal stages, where the title already says the outcome', () => {
    for (const status of ['completed', 'failed', 'rollback', 'cancelled'] as const) {
      expect(getLiveMessage(project({ deployment_status: status, deployment_message: 'anything' })))
        .toBeUndefined()
    }
  })

  it('stays quiet when the worker has written nothing yet', () => {
    expect(getLiveMessage(project({ deployment_message: undefined }))).toBeUndefined()
    expect(getLiveMessage(project({ deployment_status: undefined }))).toBeUndefined()
  })
})

describe('getFailureMessage', () => {
  it('prefers error_log, which carries the sanitized failure', () => {
    expect(getFailureMessage(project({ error_log: 'build failed', deployment_message: 'stale' })))
      .toBe('build failed')
  })

  it('falls back to the message when the deploy never reached a worker', () => {
    expect(getFailureMessage(project({ deployment_message: 'Could not be queued.' })))
      .toBe('Could not be queued.')
  })
})

describe('deriveStepDurations', () => {
  const at = (s: number) => new Date(1_700_000_000_000 + s * 1000).toISOString()
  const event = (seq: number, to: string, seconds: number, job = 'job-1') => ({
    id: seq, project_id: 1, job_id: job, sequence_number: seq,
    state_to: to, created_at: at(seconds),
  } as never)

  // Newest first, matching what the endpoint returns.
  const FEED = [
    event(6, 'completed', 192),
    event(5, 'promoting', 174),
    event(4, 'healthchecking', 136),
    event(3, 'starting', 130),
    event(2, 'building', 19),
    event(1, 'queued', 0),
  ]

  it('measures each step from its first event to the next step\'s', () => {
    // The four spans tile the whole deploy: 19 + 111 + 44 + 18 = 192s.
    expect(deriveStepDurations(FEED, 'job-1', at(192)))
      .toEqual([19, 111, 44, 18])
  })

  it('ignores events belonging to a previous job', () => {
    const mixed = [...FEED, event(9, 'building', 400, 'job-0')]
    expect(deriveStepDurations(mixed, 'job-1', at(192))).toEqual([19, 111, 44, 18])
  })

  it('leaves the running step open until the deploy finishes', () => {
    const running = FEED.slice(2) // up to healthchecking
    expect(deriveStepDurations(running, 'job-1')).toEqual([19, 111, null, null])
  })

  it('returns nulls for steps the deploy never reached', () => {
    expect(deriveStepDurations([event(1, 'queued', 0)], 'job-1', at(30)))
      .toEqual([30, null, null, null])
  })

  it('reads through a skipped step to the next one that reported', () => {
    // `starting` can be skipped: building -> healthchecking is a legal transition.
    const skipped = [event(3, 'promoting', 100), event(2, 'building', 10), event(1, 'queued', 0)]
    expect(deriveStepDurations(skipped, 'job-1', at(120))).toEqual([10, 90, null, 20])
  })

  it('is inert without a job id or events', () => {
    expect(deriveStepDurations(FEED, undefined, at(192))).toEqual([null, null, null, null])
    expect(deriveStepDurations([], 'job-1', at(192))).toEqual([null, null, null, null])
  })

  it('discards a negative span rather than rendering a backwards duration', () => {
    const skewed = [event(2, 'building', 50), event(1, 'queued', 100)]
    expect(deriveStepDurations(skewed, 'job-1', at(200))[0]).toBeNull()
  })
})
