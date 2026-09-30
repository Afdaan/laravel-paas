import type { DeploymentEvent, Project } from '@/types'
import type { ProjectCreationPhase } from './projectCreationContext'

/**
 * Pipeline steps shown on the deployment page. Four buckets over the worker's
 * finer-grained deployment_status values — see STEP_BY_STATUS for the mapping.
 */
export const DEPLOY_STEPS = ['source', 'build', 'startup', 'release'] as const
export type DeployStep = typeof DEPLOY_STEPS[number]

export type DeployStepState = 'complete' | 'active' | 'stalled' | 'failed' | 'pending'

/**
 * deployment_status -> step index. Ordering follows what the worker actually
 * emits on a deploy (deployment_worker.go): queued(0) preparing(10) cloning(20)
 * building(35) starting(50) healthchecking(65) migrating(75) promoting(85)
 * completed(100). `cleanup` only runs when a previous container exists, so
 * first deploys skip it.
 *
 * Terminal failure states are deliberately absent: `failed` tells us nothing
 * about where it stopped, so those fall through to deployment_progress.
 */
const STEP_BY_STATUS: Record<string, number> = {
  queued: 0,
  preparing: 0,
  cloning: 0,
  building: 1,
  starting: 2,
  healthchecking: 2,
  migrating: 3,
  promoting: 3,
  cleanup: 3,
  completed: DEPLOY_STEPS.length,
}

/**
 * Locates a step from deployment_progress. Only used when the status itself is
 * uninformative — on failure the worker preserves the last progress value it
 * wrote, which is the only surviving clue to where the deploy stopped.
 */
export function stepFromProgress(progress: number): number {
  if (progress >= 100) return DEPLOY_STEPS.length
  if (progress >= 75) return 3
  if (progress >= 50) return 2
  if (progress >= 21) return 1
  return 0
}

export function getActiveStep(status: string | undefined | null, progress: number): number {
  const mapped = status ? STEP_BY_STATUS[status] : undefined
  return mapped ?? stepFromProgress(progress)
}

export function getStepStates(
  phase: ProjectCreationPhase,
  activeStep: number,
  stalled = false,
): DeployStepState[] {
  return DEPLOY_STEPS.map((_, index) => {
    if (phase === 'ready') return 'complete'
    if (index < activeStep) return 'complete'
    if (index > activeStep) return 'pending'
    if (phase === 'failed') return 'failed'
    return stalled ? 'stalled' : 'active'
  })
}

/** Progress never moves backwards mid-deploy, but the data can: the watchdog's
 *  startup recovery re-queues an in-flight job, taking preparing(10) to
 *  queued(0). Clamping keeps the bar from animating in reverse. */
export function clampProgress(value?: number) {
  return Math.min(100, Math.max(0, Math.round(value ?? 0)))
}

export function formatDuration(totalSeconds: number): string {
  const seconds = Math.max(0, Math.floor(totalSeconds))
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ${String(seconds % 60).padStart(2, '0')}s`
  return `${Math.floor(minutes / 60)}h ${String(minutes % 60).padStart(2, '0')}m`
}

/**
 * Status line for the running step. Every in-progress stage now writes prose
 * ("Cloning <url> (<branch>)", "Commit <hash>: <subject>"), so the message is
 * shown as-is while the deploy runs.
 *
 * Terminal stages are excluded: their message describes an outcome the page
 * already states in the title, and repeating it in a step row reads as though
 * that step were still reporting.
 */
const TERMINAL_STATUSES = ['completed', 'failed', 'rollback', 'cancelled']

export function getLiveMessage(project: Project): string | undefined {
  const status = project.deployment_status
  if (!status || TERMINAL_STATUSES.includes(status)) return undefined
  return project.deployment_message || undefined
}

/**
 * Failure text. error_log and deployment_message hold the same sanitized string
 * on a real worker failure, but error_log is absent when the deployment was
 * never picked up, so deployment_message is the fallback rather than the
 * preference.
 */
export function getFailureMessage(project: Project): string | undefined {
  return project.error_log || project.deployment_message || undefined
}

/**
 * Per-step wall time, derived from the deployment event log.
 *
 * The events carry a `duration_ms` column, but nothing writes it — it is always
 * zero. Timings therefore come from the gap between consecutive `created_at`
 * values: a step starts when its first event lands and ends when the next
 * step's first event does. The final step is closed by the deployment's own
 * finish time, and is left open while the deploy is still running.
 *
 * Returns seconds per step, with `null` where no event was recorded.
 */
export function deriveStepDurations(
  events: DeploymentEvent[],
  jobId: string | undefined,
  finishedAt?: string,
): (number | null)[] {
  const empty = DEPLOY_STEPS.map(() => null as number | null)
  if (!jobId || events.length === 0) return empty

  // The endpoint returns newest first and may span several jobs.
  const marks = events
    .filter(event => event.job_id === jobId && event.state_to)
    .map(event => ({
      step: STEP_BY_STATUS[event.state_to as string],
      at: new Date(event.created_at).getTime(),
      sequence: event.sequence_number,
    }))
    .filter(mark => mark.step !== undefined && !Number.isNaN(mark.at))
    .sort((a, b) => a.at - b.at || a.sequence - b.sequence)

  if (marks.length === 0) return empty

  // First sighting of each step, including the one past the last (`completed`),
  // which closes the final step.
  const firstSeen = new Map<number, number>()
  for (const mark of marks) {
    if (!firstSeen.has(mark.step)) firstSeen.set(mark.step, mark.at)
  }

  const end = finishedAt ? new Date(finishedAt).getTime() : NaN

  return DEPLOY_STEPS.map((_, index) => {
    const start = firstSeen.get(index)
    if (start === undefined) return null

    let stop: number | undefined
    for (let next = index + 1; next <= DEPLOY_STEPS.length; next++) {
      const candidate = firstSeen.get(next)
      if (candidate !== undefined) { stop = candidate; break }
    }
    if (stop === undefined && !Number.isNaN(end)) stop = end
    if (stop === undefined || stop < start) return null

    return (stop - start) / 1000
  })
}
