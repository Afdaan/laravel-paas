import { useCallback, useEffect, useId, useMemo, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import {
  AlertTriangle,
  ArrowLeft,
  ArrowUpRight,
  Check,
  Copy,
  ExternalLink,
  Settings,
  FileText,
  GitBranch,
  RefreshCw,
  X,
} from 'lucide-react'

import useTranslation from '@/lib/useTranslation'
import { usePolling } from '@/lib/usePolling'
import { useNow } from '@/lib/useNow'
import { projectsAPI } from '@/services/api'
import type { DeploymentEvent, Project } from '@/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Spinner } from '@/components/ui/spinner'
import { siGithub, siGitlab } from 'simple-icons'
import { Tooltip } from '@base-ui/react/tooltip'
import { cn } from '@/lib/utils'
import { FrameworkIcon } from '@/components/FrameworkIcon'
import { DatabaseEngineIcon } from '@/components/database-studio/utils'
import { getProjectCreationPhase } from '@/components/project/projectCreationContext'
import { parseFailure } from '@/components/project/deploymentFailure'
import {
  clampProgress,
  DEPLOY_STEPS,
  deriveStepDurations,
  formatDuration,
  getActiveStep,
  getFailureMessage,
  getLiveMessage,
  getStepStates,
  type DeployStepState,
} from '@/components/project/deploymentStages'

/** Workers refresh deployment_heartbeat_at on a 30s ticker
 *  (deployment_worker.go). Five missed beats is late enough to be real and
 *  early enough to land before the server's own 3-minute lease reaper. */
const HEARTBEAT_SILENCE_MS = 150_000

/** Lines of build output shown inline. The full log is one click away, so this
 *  only has to answer "what is it doing right now". */
const BUILD_TAIL_LINES = 6

function serverHeartbeatAge(timestamp: string | undefined, serverNow: number): number {
  if (!timestamp) return 0
  const age = serverNow - new Date(timestamp).getTime()
  if (Number.isNaN(age) || age < 0) return 0
  return age
}

// --- Pipeline progress meter ---
// Four segments mirroring the four steps below, so the header badge doubles as
// a map of the card without needing to scroll to it.
function StepMeter({ states }: { states: DeployStepState[] }) {
  return (
    <span className="inline-flex shrink-0 items-center gap-px" aria-hidden="true">
      {states.map((state, index) => (
        <span
          key={DEPLOY_STEPS[index]}
          className={cn(
            'relative h-[3px] w-2 overflow-hidden rounded-full bg-current',
            state === 'pending' && 'opacity-15',
            state === 'complete' && 'opacity-85',
            state === 'failed' && 'opacity-100',
            (state === 'active' || state === 'stalled') && 'bg-current/20 deploy-segment',
            state === 'stalled' && 'deploy-segment-stalled',
          )}
        />
      ))}
    </span>
  )
}

// --- Step rail indicator ---
function StepDot({ state }: { state: DeployStepState }) {
  const running = state === 'active' || state === 'stalled'

  return (
    <span
      className={cn(
        'relative flex size-[18px] shrink-0 items-center justify-center rounded-full border',
        state === 'complete' && 'border-emerald-500/35 text-emerald-600 dark:text-emerald-400',
        state === 'failed' && 'border-destructive/40 text-destructive',
        state === 'active' && 'border-border text-foreground',
        state === 'stalled' && 'border-border text-amber-600 dark:text-amber-400',
        state === 'pending' && 'border-border text-muted-foreground/45',
      )}
    >
      {running && (
        <svg viewBox="0 0 20 20" className="absolute -inset-px size-[calc(100%+2px)] -rotate-90">
          <circle
            cx="10"
            cy="10"
            r="9"
            pathLength="58"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            className={cn('deploy-arc', state === 'stalled' && 'deploy-arc-stalled')}
          />
        </svg>
      )}
      {state === 'complete' && <Check className="size-2.5" strokeWidth={3} aria-hidden="true" />}
      {state === 'failed' && <X className="size-2.5" strokeWidth={2.5} aria-hidden="true" />}
      {running && <span className="size-1.5 rounded-full bg-current" />}
      {state === 'pending' && <span className="size-[5px] rounded-full bg-current" />}
    </span>
  )
}

function PipelineStep({ label, state, detail, description, duration }: {
  label: string
  state: DeployStepState
  /** What this step is reporting right now, when it has something to say. */
  detail?: string
  /** What the step does. Shown whenever there is no live detail, so a row is
   *  never just a label: a queued step explains what is coming, and a finished
   *  one still says what it did. */
  description?: string
  duration?: string
}) {
  const body = detail || description
  const isLive = Boolean(detail)
  return (
    <li className="grid grid-cols-[18px_minmax(0,1fr)] gap-3 px-4 py-3">
      <div className="flex justify-center pt-px"><StepDot state={state} /></div>
      {/* Narrow screens give the detail its own line under the label rather than
          squeezing it between the label and the duration, where a two-word
          message was wrapping into three lines. */}
      <div className="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-0.5 sm:gap-y-1">
        <span className={cn(
          'order-1 text-[13px] sm:min-w-[6.5rem]',
          state === 'pending' ? 'text-muted-foreground' : 'font-medium text-foreground',
        )}>{label}</span>
        {duration && (
          <span className="order-2 ml-auto font-mono text-[11px] tabular-nums text-muted-foreground sm:order-3">{duration}</span>
        )}
        {body && (
          <span className={cn(
            'order-3 min-w-0 basis-full break-words text-xs sm:order-2 sm:basis-auto sm:flex-1',
            state === 'failed' && isLive ? 'text-destructive'
              : state === 'active' && isLive ? 'text-foreground'
                : 'text-muted-foreground',
          )}>{body}</span>
        )}
      </div>
    </li>
  )
}

function MetaCell({ label, children, icon, secondary }: {
  label: string
  children: React.ReactNode
  /** A small mark before the value, only where it identifies something. */
  icon?: React.ReactNode
  /** Supporting detail under the value, set smaller and muted. */
  secondary?: React.ReactNode
}) {
  return (
    <div className="min-w-0 border-t px-4 py-2.5 first:border-t-0 sm:border-t-0 sm:border-l sm:first:border-l-0">
      <dt className="mb-0.5 text-xs text-muted-foreground">{label}</dt>
      <dd className="min-w-0 text-[13px] text-foreground">
        <div className="flex min-w-0 items-center gap-1.5">
          {icon && <span className="flex size-3.5 shrink-0 items-center justify-center">{icon}</span>}
          <div className="min-w-0 truncate">{children}</div>
        </div>
        {secondary && <div className="mt-0.5 truncate font-mono text-[11px] text-muted-foreground">{secondary}</div>}
      </dd>
    </div>
  )
}

/** Monochrome on purpose: beside a commit subject the host is context, not a logo. */
function RepoHostIcon({ url }: { url?: string }) {
  const icon = url?.includes('github.com') ? siGithub : url?.includes('gitlab') ? siGitlab : null
  if (!icon) return null
  return (
    <svg viewBox="0 0 24 24" aria-label={icon.title} role="img" className="size-3.5 text-muted-foreground">
      <path fill="currentColor" d={icon.path} />
    </svg>
  )
}

const DATABASE_ENGINE_LABEL: Record<string, string> = { mysql: 'MySQL', postgresql: 'PostgreSQL' }

// ponytail: GitHub and GitLab both resolve /commit/<sha>; Bitbucket uses
// /commits/ and lands on a 404 until a host-aware mapping is worth adding.
function commitUrl(repositoryUrl: string | undefined, hash: string | undefined) {
  if (!repositoryUrl || !hash || !/^https?:\/\//.test(repositoryUrl)) return null
  return `${repositoryUrl.replace(/\/+$/, '').replace(/\.git$/, '')}/commit/${hash}`
}

export default function ProjectDeployment() {
  const warningTooltipId = useId()
  const { uid } = useParams<{ uid: string }>()
  const { language, t } = useTranslation()
  const [project, setProject] = useState<Project | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [isRetrying, setIsRetrying] = useState(false)
  const [urlCopied, setUrlCopied] = useState(false)
  const [serverClock, setServerClock] = useState<{ time: number; receivedAt: number } | null>(null)
  const requestSequence = useRef(0)

  const fetchProject = useCallback(async (silent = false) => {
    if (!uid) return
    const sequence = ++requestSequence.current
    if (!silent) setIsLoading(true)

    try {
      const response = await projectsAPI.get(uid)
      if (sequence !== requestSequence.current) return
      const receivedAt = performance.now()
      const serverTime = Date.parse(response.data.server_time || '')
      setServerClock(Number.isFinite(serverTime)
        ? { time: serverTime, receivedAt }
        : null)
      setProject(response.data)
      setLoadError(false)
    } catch {
      if (sequence !== requestSequence.current || silent) return
      setLoadError(true)
    } finally {
      if (sequence === requestSequence.current && !silent) setIsLoading(false)
    }
  }, [uid])

  useEffect(() => {
    setProject(null)
    setLoadError(false)
    void fetchProject()
  }, [fetchProject])

  const phase = project ? getProjectCreationPhase(project) : 'deploying'
  usePolling(() => void fetchProject(true), project && phase === 'deploying' ? 2_000 : null)

  // Progress only ever climbs within one job. Watchdog recovery re-queues an
  // in-flight deploy, which moves preparing(10) back to queued(0) and would
  // otherwise run the bar backwards.
  const [progressFloor, setProgressFloor] = useState(0)
  const jobId = project?.deployment_job_id
  useEffect(() => { setProgressFloor(0) }, [jobId])

  const reported = clampProgress(project?.deployment_progress)
  const progress = phase === 'ready' ? 100 : Math.max(progressFloor, reported)
  useEffect(() => {
    setProgressFloor(current => (reported > current ? reported : current))
  }, [reported])

  const heartbeat = project?.deployment_heartbeat_at
  const waitingForPickup = project?.deployment_status === 'queued'
  const pickedUpAt = waitingForPickup ? undefined : project?.deployment_started_at
  const enqueuedAt = project?.deployment_enqueued_at
  const finishedAt = project?.deployment_finished_at
  const clockRunning = phase === 'deploying' && Boolean(enqueuedAt || heartbeat)
  useNow(clockRunning)
  const observedAt = performance.now()
  const now = serverClock ? serverClock.time + observedAt - serverClock.receivedAt : NaN

  const heartbeatSeen = useRef<{ jobId?: string; value?: string; status?: string; at: number }>({ at: 0 })
  if (jobId !== heartbeatSeen.current.jobId || heartbeat !== heartbeatSeen.current.value || project?.deployment_status !== heartbeatSeen.current.status) {
    const isFirstReading = jobId !== heartbeatSeen.current.jobId || !heartbeatSeen.current.value
    heartbeatSeen.current = {
      jobId,
      value: heartbeat,
      status: project?.deployment_status,
      at: observedAt - (isFirstReading && serverClock ? serverHeartbeatAge(heartbeat, now) : 0),
    }
  }

  const silentFor = heartbeat && phase === 'deploying' && !waitingForPickup ? Math.max(0, observedAt - heartbeatSeen.current.at) : 0
  const stalled = silentFor >= HEARTBEAT_SILENCE_MS

  const elapsed = useMemo(() => {
    if (!enqueuedAt) return null
    const start = new Date(enqueuedAt).getTime()
    if (Number.isNaN(start)) return null
    const end = phase === 'deploying' ? now : finishedAt ? new Date(finishedAt).getTime() : NaN
    if (Number.isNaN(end)) return null
    return formatDuration((end - start) / 1000)
  }, [enqueuedAt, finishedAt, phase, now])

  // Visible only once it is worth reading: a deploy picked up immediately
  // should not grow a "waited 0s" line.
  const queuedFor = useMemo(() => {
    if (!project?.deployment_enqueued_at || !pickedUpAt) return null
    const waited = (new Date(pickedUpAt).getTime() - new Date(project.deployment_enqueued_at).getTime()) / 1000
    if (Number.isNaN(waited) || waited < 5) return null
    return formatDuration(waited)
  }, [project?.deployment_enqueued_at, pickedUpAt])

  // Per-step timings come from the event log. Refetched when the stage moves,
  // which is the only moment a new event can have landed, so this does not ride
  // along with the 2s project poll.
  const [events, setEvents] = useState<DeploymentEvent[]>([])
  const deploymentStatus = project?.deployment_status
  useEffect(() => {
    if (!uid || !jobId) return
    let cancelled = false
    projectsAPI.getDeploymentEvents(uid)
      .then(response => { if (!cancelled) setEvents(response.data || []) })
      // Durations are a nicety; the pipeline reads fine without them.
      .catch(() => { if (!cancelled) setEvents([]) })
    return () => { cancelled = true }
  }, [uid, jobId, deploymentStatus])

  // The worker records the commit subject only on the build event, as
  // "Commit <hash>: <subject>"; the project row keeps just the hash.
  const commitSubject = useMemo(() => {
    const build = [...events].reverse().find(event => event.job_id === jobId && event.event_type === 'building_image')
    return build?.payload?.match(/^Commit [0-9a-f]+: (.+)$/s)?.[1]
  }, [events, jobId])

  const stepDurations = useMemo(
    () => deriveStepDurations(events, jobId, finishedAt),
    [events, jobId, finishedAt],
  )

  // Inline build output. Polled only while the build is the running step - the
  // one stretch where progress is pinned (the worker writes no value between
  // 35 and 50) and deployment_message changes once, so nothing else on the page
  // moves. Fetched once more on failure, where the last lines are the point.
  const [buildTail, setBuildTail] = useState<string | null>(null)
  const isBuilding = deploymentStatus === 'building'
  const showsBuildTail = isBuilding || phase === 'failed'

  // Responses are sequenced the same way project fetches are: a request issued
  // while the build was running can resolve after the stage has moved on, and
  // would otherwise re-populate output the effect below just cleared. The
  // counter also advances on uid/job change, so a previous project's log cannot
  // land in the new one.
  const buildTailSequence = useRef(0)
  const fetchBuildTail = useCallback(async () => {
    if (!uid) return
    const sequence = ++buildTailSequence.current
    try {
      const response = await projectsAPI.buildLogs(uid, BUILD_TAIL_LINES)
      if (sequence !== buildTailSequence.current) return
      setBuildTail(response.data?.placeholder ? null : response.data?.logs || null)
    } catch {
      // The log is supplementary; the pipeline reads fine without it.
      if (sequence !== buildTailSequence.current) return
      setBuildTail(null)
    }
  }, [uid])

  usePolling(() => void fetchBuildTail(), isBuilding ? 3_000 : null)

  // Keyed on the job as well as the stage: a retry produces a new job whose
  // output must not inherit the previous one's. fetchBuildTail bumps the
  // sequence itself, so re-running here invalidates anything still in flight.
  useEffect(() => {
    if (showsBuildTail) {
      void fetchBuildTail()
      return
    }
    buildTailSequence.current++
    setBuildTail(null)
  }, [showsBuildTail, fetchBuildTail, jobId])

  const activeStep = getActiveStep(project?.deployment_status, reported)
  const stepStates = getStepStates(phase, activeStep)

  const title = phase === 'ready'
    ? t('projectDetail.provisioning.readyTitle', { name: project?.name || '' })
    : phase === 'failed'
      ? t('projectDetail.provisioning.failedTitle', { name: project?.name || '' })
      : t('projectDetail.provisioning.deployingTitle', { name: project?.name || '' })

  const handleRetry = async () => {
    if (!uid || isRetrying) return
    setIsRetrying(true)
    try {
      const response = await projectsAPI.redeploy(uid)
      setProject(current => current ? {
        ...current,
        status: 'queued',
        deployment_status: 'queued',
        deployment_job_id: response.data.job_id || current.deployment_job_id,
        deployment_message: t('projectDetail.provisioning.retryStarted'),
        deployment_progress: 0,
        deployment_enqueued_at: undefined,
        deployment_started_at: undefined,
        deployment_finished_at: undefined,
        deployment_heartbeat_at: undefined,
        error_log: undefined,
      } : current)
      toast.success(t('projectDetail.provisioning.retryStarted'))
      await fetchProject(true)
    } catch {
      toast.error(t('projectDetail.provisioning.retryFailed'))
    } finally {
      setIsRetrying(false)
    }
  }

  const projectUrl = project?.url || (project?.subdomain ? `https://${project.subdomain}` : null)

  const handleCopyUrl = () => {
    if (!projectUrl) return
    navigator.clipboard.writeText(projectUrl)
    setUrlCopied(true)
    setTimeout(() => setUrlCopied(false), 1500)
  }

  // --- Loading ---
  if (isLoading) {
    return (
      <div className="flex min-h-[calc(100dvh-12rem)] items-center justify-center" role="status" aria-live="polite">
        <div className="flex items-center gap-3 text-sm font-medium text-muted-foreground">
          <Spinner className="size-5" />
          {t('projectDetail.provisioning.loading')}
        </div>
      </div>
    )
  }

  // --- Error ---
  if (loadError || !project) {
    return (
      <div className="mx-auto flex min-h-[calc(100dvh-12rem)] w-full max-w-3xl items-center">
        <Card className="w-full shadow-none">
          <CardContent className="flex flex-col items-start gap-4 p-6 sm:p-8">
            <AlertTriangle className="size-5 text-destructive" aria-hidden="true" />
            <div>
              <h1 className="text-lg font-semibold tracking-tight">{t('projectDetail.provisioning.loadFailed')}</h1>
              <p className="mt-1.5 text-sm leading-6 text-muted-foreground">{t('projectDetail.provisioning.loadFailedDesc')}</p>
            </div>
            <Button type="button" variant="outline" onClick={() => void fetchProject()}>
              <RefreshCw className="size-4" aria-hidden="true" />
              {t('common.retry')}
            </Button>
          </CardContent>
        </Card>
      </div>
    )
  }

  const shortCommit = project.last_commit_hash?.slice(0, 7)
  const repositoryUrl = project.github_url || project.repository_url
  const shortCommitUrl = commitUrl(repositoryUrl, project.last_commit_hash)
  const liveMessage = getLiveMessage(project)
  const failure = phase === 'failed' ? parseFailure(getFailureMessage(project)) : null
  // A project that has never served traffic has a subdomain but nothing behind
  // it, so the metadata card names it without inviting a click into a 502.
  const domainIsLive = phase === 'ready' || Boolean(project.deployment_finished_at && phase !== 'failed')
  const stageLabel = phase === 'ready'
    ? t('projectDetail.provisioning.statusLive')
    : phase === 'failed'
      ? t('projectDetail.provisioning.statusFailed').toLowerCase()
      : !project.deployment_status || project.deployment_status === 'queued'
        ? t('projectDetail.provisioning.noSignal')
        : project.deployment_status

  const timestampLabel = phase === 'ready'
    ? t('projectDetail.provisioning.finishedAt')
    : phase === 'failed'
      ? t('projectDetail.provisioning.failedAt')
      : t('projectDetail.provisioning.queuedAt')
  const timestampValue = phase === 'deploying' ? enqueuedAt || pickedUpAt : finishedAt || pickedUpAt || enqueuedAt
  const timestamp = timestampValue
    ? new Date(timestampValue).toLocaleString(language === 'id' ? 'id-ID' : 'en-US', {
      month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
    })
    : null

  // "Pending" promises a value that is still on its way. After a failure
  // nothing more is coming, so the cells say so instead.
  const unknownValue = phase === 'failed'
    ? t('projectDetail.provisioning.notDetected')
    : t('projectDetail.provisioning.pending')

  const commitLink = shortCommit && (shortCommitUrl ? (
    <a
      href={shortCommitUrl}
      target="_blank"
      rel="noopener noreferrer"
      className="underline-offset-[3px] hover:text-foreground hover:underline"
    >{shortCommit}</a>
  ) : shortCommit)

  // The framework leads; the language runtime and port are detail. Node
  // frameworks are recognized by node_version, which the worker only fills
  // for them, so this needs no copy of the worker's framework list.
  const phpVersion = project.php_version?.replace('.dynamic', '')
  const framework = project.framework || project.detected_framework
  const runtimeLabel = framework === 'Laravel'
    ? `Laravel${project.laravel_version ? ` ${project.laravel_version}` : ''}`
    : !framework
      ? phpVersion && `PHP ${phpVersion}`
      : !project.node_version && project.language_version
        ? `${framework} ${project.language_version}`
        : framework
  const runtimeDetail = [
    framework && phpVersion && `PHP ${phpVersion}`,
    project.node_version && `Node.js ${project.node_version}`,
    project.internal_port && t('projectDetail.provisioning.runtimePort', { port: project.internal_port }),
    project.cpu_limit && `${project.cpu_limit} vCPU`,
    project.memory_limit,
  ].filter(Boolean).join(' · ') || undefined
  // No runtime detected yet means no mark, rather than FrameworkIcon's globe.
  const runtimeIcon = framework || (phpVersion ? 'php' : undefined)

  // A project created without a database has nothing coming, so it says so
  // rather than sitting on "Pending" forever.
  const database = project.database_instance
  const databaseOption = project.database_option
  const databaseName = database?.name || project.database_name
  const databaseIsName = Boolean(databaseName) && databaseOption !== 'sqlite' && databaseOption !== 'external'
  const databaseLabel = databaseOption === 'none'
    ? t('projectDetail.provisioning.noDatabase')
    : databaseOption === 'sqlite'
      ? 'SQLite'
      : databaseOption === 'external'
        ? t('projectDetail.provisioning.externalDatabase')
        : databaseName || unknownValue
  const databaseHasValue = databaseOption === 'sqlite' || databaseOption === 'external' || Boolean(databaseIsName)
  const databaseDetail = databaseIsName && database
    ? [DATABASE_ENGINE_LABEL[database.engine] || database.engine, database.version].filter(Boolean).join(' ')
    : undefined

  const pipelineStatus = phase === 'ready'
    ? t('projectDetail.provisioning.pipelineCompleted')
    : phase === 'failed'
      ? t('projectDetail.provisioning.pipelineFailedAt', { step: activeStep + 1 })
      : t('projectDetail.provisioning.pipelineStep', { current: Math.min(activeStep + 1, DEPLOY_STEPS.length), total: DEPLOY_STEPS.length })

  const stepDetails: (string | undefined)[] = DEPLOY_STEPS.map(() => undefined)
  // Source keeps a standing description; every other step only says something
  // while it is the one running.
  stepDetails[0] = queuedFor
    ? t('projectDetail.provisioning.queuedFor', { duration: queuedFor })
    : shortCommit
      ? `${t('projectDetail.provisioning.stepSourceDetail')} ${shortCommit}`
      : project.branch
        ? `${t('projectDetail.provisioning.stepSourceDetail')} ${project.branch}`
        : undefined
  if (liveMessage && activeStep < DEPLOY_STEPS.length) {
    stepDetails[activeStep] = liveMessage
  }

  return (
    <div className="mx-auto w-full max-w-5xl pb-16">
      <Button variant="ghost" size="sm" render={<Link to="/projects" />} className="mb-1 -ml-2 text-muted-foreground">
        <ArrowLeft className="size-4" aria-hidden="true" />
        {t('projectDetail.provisioning.backToProjects')}
      </Button>

      {/* Header: title on the left, status and actions on the right */}
      <div className="mb-5 flex flex-wrap items-center justify-between gap-x-4 gap-y-3">
        <h1 className={cn(
          'text-2xl font-semibold tracking-tight',
          phase === 'failed' && 'text-destructive',
        )}>{title}</h1>

        <div className="flex flex-wrap items-center gap-2">
          <Badge
            variant="outline"
            aria-live="polite"
            className={cn(
              'h-8 gap-1.5 rounded-full px-2.5 text-xs font-medium',
              phase === 'deploying' && 'bg-muted',
              phase === 'ready' && 'border-emerald-500/30 bg-emerald-500/[0.07] text-emerald-600 dark:text-emerald-400',
              phase === 'failed' && 'border-destructive/30 bg-destructive/[0.07] text-destructive',
            )}
          >
            <StepMeter states={stepStates} />
            <span className="font-mono">{stageLabel}</span>
          </Badge>

          {/* The badge reports state rather than offering an action. On a narrow
              screen this break puts it on its own line so the buttons wrap
              among themselves, without stretching the pill to fill the row. */}
          <span className="basis-full sm:hidden" aria-hidden="true" />

          {projectUrl && phase === 'ready' && (
            <Button variant="outline" size="sm" onClick={handleCopyUrl}>
              <Copy className="size-3.5" aria-hidden="true" />
              {urlCopied ? t('projectDetail.provisioning.copied') : t('projectDetail.provisioning.copyUrl')}
            </Button>
          )}
          {phase === 'ready' && (
            <Button size="sm" render={<Link to={`/projects/${uid}`} />}>
              {t('projectDetail.provisioning.openProject')}
            </Button>
          )}
          {(phase === 'deploying' || phase === 'failed') && (
            <Button variant="outline" size="sm" render={<Link to={`/projects/${uid}?tab=build`} />}>
              <FileText className="size-3.5" aria-hidden="true" />
              {t('projectDetail.provisioning.viewBuildLogs')}
            </Button>
          )}
          {phase === 'failed' && (
            <Button
              size="sm"
              type="button"
              // Retry leads only when it can plausibly change the outcome. For a
              // configuration failure it stays available but steps aside.
              variant={failure?.remedy === 'settings' ? 'outline' : 'default'}
              onClick={() => void handleRetry()}
              disabled={isRetrying}
            >
              {isRetrying ? <Spinner className="size-3.5" /> : <RefreshCw className="size-3.5" aria-hidden="true" />}
              {t('projectDetail.provisioning.retryDeployment')}
            </Button>
          )}
          {phase === 'failed' && failure?.remedy === 'settings' && (
            <Button size="sm" render={<Link to={`/projects/${uid}?tab=settings`} />}>
              <Settings className="size-3.5" aria-hidden="true" />
              {t('projectDetail.provisioning.openSettings')}
            </Button>
          )}
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={() => void fetchProject(true)}
            disabled={isRetrying}
            aria-label={t('projectDetail.provisioning.refresh')}
            title={t('projectDetail.provisioning.refresh')}
            className="text-muted-foreground"
          >
            <RefreshCw className="size-3.5" aria-hidden="true" />
          </Button>
        </div>
      </div>

      {/* Build output — only while the build runs, and on failure */}
      {buildTail && showsBuildTail && (
        <Card className="mb-3 gap-0 overflow-hidden py-0 shadow-none">
          <div className="flex items-center justify-between gap-4 border-b px-4 py-2.5">
            <span className="text-[13px] font-medium">{t('projectDetail.provisioning.buildOutput')}</span>
            <Link
              to={`/projects/${uid}?tab=build`}
              className="inline-flex items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
            >
              {t('projectDetail.provisioning.openFullLogs')}
              <ExternalLink className="size-3" aria-hidden="true" />
            </Link>
          </div>
          <pre
            className="overflow-x-auto bg-muted/30 px-4 py-3 font-mono text-[11px] leading-[1.8] whitespace-pre-wrap break-all text-muted-foreground"
            aria-live="polite"
          >{buildTail}</pre>
        </Card>
      )}

      {/* Pipeline */}
      <Card className="mb-3 gap-0 overflow-hidden py-0 shadow-none">
        {/* Seated on the card's top edge rather than under the header. Directly
            below a left-aligned label, a part-width rule reads as a selected-tab
            indicator no matter how the track is coloured. */}
        <div
          className="h-[3px] bg-muted"
          role="progressbar"
          aria-valuenow={progress}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-label={t('projectDetail.provisioning.progress')}
        >
          <div
            className={cn(
              'h-full transition-[width] duration-500 ease-out motion-reduce:transition-none',
              phase === 'ready' ? 'bg-emerald-500' : phase === 'failed' ? 'bg-destructive' : 'bg-foreground',
            )}
            style={{ width: `${progress}%` }}
          />
        </div>

        <div className="flex flex-wrap items-center justify-between gap-4 px-4 pb-3 pt-3.5">
          <span className="text-[13px] font-medium">{t('projectDetail.provisioning.pipelineTitle')}</span>
          <span className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
            {pipelineStatus}
            <span aria-hidden="true">·</span>
            <span className="tabular-nums">{progress}%</span>
            <span aria-hidden="true">·</span>
            <span>
              {t('projectDetail.provisioning.totalElapsed')}{' '}
              <span className="font-mono tabular-nums">{elapsed ?? '—'}</span>
            </span>
          </span>
        </div>

        <ul className="divide-y">
          {DEPLOY_STEPS.map((step, index) => (
            <PipelineStep
              key={step}
              label={t(`projectDetail.provisioning.step_${step}`)}
              state={stepStates[index]}
              detail={stepDetails[index]}
              description={t(`projectDetail.provisioning.desc_${step}`)}
              duration={stepDurations[index] !== null ? formatDuration(stepDurations[index]!) : undefined}
            />
          ))}
        </ul>

        {stalled && (
          <div className="border-t px-4 py-2">
            <Tooltip.Root>
              <Tooltip.Trigger aria-describedby={warningTooltipId} className="inline-flex items-center gap-1.5 rounded-sm text-xs text-amber-600 outline-none focus-visible:ring-2 focus-visible:ring-ring dark:text-amber-400">
                <AlertTriangle className="size-3.5 shrink-0" aria-hidden="true" />
                {t('projectDetail.provisioning.updateDelayed')}
              </Tooltip.Trigger>
              <Tooltip.Portal>
                <Tooltip.Positioner sideOffset={6} className="z-50">
                  <Tooltip.Popup id={warningTooltipId} role="tooltip" className="max-w-xs rounded-md border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-sm">
                    {t('projectDetail.provisioning.heartbeatLost', { duration: formatDuration(silentFor / 1000) })}
                  </Tooltip.Popup>
                </Tooltip.Positioner>
              </Tooltip.Portal>
            </Tooltip.Root>
          </div>
        )}

        {failure && (
          <div className="flex gap-2.5 border-t bg-destructive/[0.05] px-4 py-3">
            <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-destructive" aria-hidden="true" />
            {/* The worker appends a recommendation block to failure messages,
                so this is multi-line and must keep its breaks. The machine code
                is dropped here and shown in the footer, where it stays
                copyable for support without leading the sentence. */}
            {/* Prose, so it is set in the body face. The worker's own log lines
                stay monospaced in the build output card above; mixing the two
                made a plain sentence read like terminal spill. */}
            <p className="min-w-0 whitespace-pre-line break-words text-xs leading-relaxed text-destructive">
              {failure.message}
            </p>
          </div>
        )}

        <div className="flex flex-wrap items-center justify-between gap-4 border-t bg-muted/45 px-4 py-2.5">
          <span className="flex items-center gap-2 font-mono text-[11px] text-muted-foreground">
            {project.deployment_job_id || t('projectDetail.provisioning.pendingId')}
            {failure?.code && (
              <>
                <span aria-hidden="true">·</span>
                <span className="text-destructive/80">{failure.code}</span>
              </>
            )}
          </span>
          {timestamp && (
            <span className="font-mono text-[11px] text-muted-foreground">{timestampLabel} {timestamp}</span>
          )}
        </div>
      </Card>

      {/* Metadata. The domain is what the user came for once a deploy lands, so
          it gets its own row; the rest is reference and sits in a strip styled
          like the pipeline card's footer. */}
      <Card className="gap-0 overflow-hidden py-0 shadow-none">
        <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2.5 px-4 py-3.5">
          <div className="min-w-0">
            <div className="mb-0.5 text-xs text-muted-foreground">{t('projectDetail.provisioning.metaDomain')}</div>
            {/* `anywhere` lets a hostname wrap at its own separators rather
                than mid-label, which `break-all` does. */}
            <div className={cn(
              'font-mono text-[15px] [overflow-wrap:anywhere]',
              projectUrl && domainIsLive ? 'text-foreground' : 'text-muted-foreground',
            )}>
              {projectUrl ? projectUrl.replace(/^https?:\/\//, '') : unknownValue}
            </div>
          </div>
          {/* A project that never served traffic gets a reason, not a link
              into a 502. */}
          {projectUrl && (domainIsLive ? (
            <Button
              variant="outline"
              size="sm"
              render={<a href={projectUrl} target="_blank" rel="noopener noreferrer" />}
            >
              {t('projectDetail.provisioning.openSite')}
              <ArrowUpRight className="size-3.5" aria-hidden="true" />
            </Button>
          ) : phase === 'deploying' && (
            <span className="text-xs text-muted-foreground">{t('projectDetail.provisioning.domainPendingHint')}</span>
          ))}
        </div>

        <dl className="grid grid-cols-1 border-t bg-muted/45 sm:grid-cols-3">
          <MetaCell
            label={t('projectDetail.provisioning.metaSource')}
            icon={<RepoHostIcon url={repositoryUrl} />}
            secondary={(
              <>
                <GitBranch className="mr-1 inline size-3 align-[-2px]" aria-hidden="true" />
                {project.branch || 'main'}
                {commitSubject && commitLink && (
                  <>
                    <span className="px-1">·</span>
                    {commitLink}
                  </>
                )}
              </>
            )}
          >
            {commitSubject
              ? <span title={commitSubject}>{commitSubject}</span>
              : commitLink
                ? <span className="font-mono">{commitLink}</span>
                : <span className="text-muted-foreground">{unknownValue}</span>}
          </MetaCell>

          <MetaCell
            label={t('projectDetail.provisioning.metaRuntime')}
            icon={runtimeIcon && <FrameworkIcon framework={runtimeIcon} variant="plain" className="size-3.5" />}
            secondary={runtimeDetail}
          >
            <span className={runtimeLabel ? undefined : 'text-muted-foreground'}>{runtimeLabel || unknownValue}</span>
          </MetaCell>

          <MetaCell
            label={t('projectDetail.provisioning.metaDatabase')}
            icon={databaseDetail && database && <DatabaseEngineIcon engine={database.engine} className="size-3.5" />}
            secondary={databaseOption === 'none' ? (
              <Link
                to={`/projects/${uid}?tab=settings`}
                className="font-sans text-xs text-foreground/80 underline-offset-[3px] transition-colors hover:text-foreground hover:underline"
              >
                {t('projectDetail.provisioning.attachDatabase')} →
              </Link>
            ) : databaseDetail}
          >
            <span className={cn(databaseIsName && 'font-mono', !databaseHasValue && 'text-muted-foreground')}>
              {databaseLabel}
            </span>
          </MetaCell>
        </dl>
      </Card>
    </div>
  )
}
