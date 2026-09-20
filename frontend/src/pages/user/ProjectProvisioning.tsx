import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { toast } from 'sonner'
import {
  AlertTriangle,
  ArrowLeft,
  CheckCircle2,
  Circle,
  FileText,
  Loader2,
  RefreshCw,
} from 'lucide-react'

import useTranslation from '@/lib/useTranslation'
import { usePolling } from '@/lib/usePolling'
import { projectsAPI } from '@/services/api'
import type { Project } from '@/types'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Progress, ProgressLabel, ProgressValue } from '@/components/ui/progress'
import { cn } from '@/lib/utils'
import { getProjectCreationPhase, type ProjectCreationPhase } from '@/components/project/projectCreationContext'

type StepState = 'complete' | 'active' | 'pending' | 'failed'

function clampProgress(value?: number) {
  return Math.min(100, Math.max(0, value ?? 0))
}

function ProvisioningStep({ label, state }: { label: string; state: StepState }) {
  const icon = state === 'complete'
    ? <CheckCircle2 className="size-4" aria-hidden="true" />
    : state === 'active'
      ? <Loader2 className="size-4 animate-spin" aria-hidden="true" />
      : state === 'failed'
        ? <AlertTriangle className="size-4" aria-hidden="true" />
        : <Circle className="size-4" aria-hidden="true" />

  return (
    <div className="flex items-center gap-3 py-4 first:pt-0 last:pb-0">
      <div className={cn(
        'flex size-7 shrink-0 items-center justify-center rounded-full border bg-background',
        state === 'complete' && 'border-emerald-500/30 text-emerald-500',
        state === 'active' && 'border-primary/30 text-primary',
        state === 'failed' && 'border-destructive/30 text-destructive',
        state === 'pending' && 'border-border text-muted-foreground/40',
      )}>
        {icon}
      </div>
      <span className={cn(
        'text-sm font-medium',
        state === 'pending' ? 'text-muted-foreground/60' : 'text-foreground',
      )}>
        {label}
      </span>
    </div>
  )
}

function getStepStates(phase: ProjectCreationPhase): [StepState, StepState, StepState] {
  if (phase === 'ready') return ['complete', 'complete', 'complete']
  if (phase === 'failed') return ['complete', 'failed', 'pending']
  return ['complete', 'active', 'pending']
}

export default function ProjectProvisioning() {
  const { uid } = useParams<{ uid: string }>()
  const { language, t } = useTranslation()
  const [project, setProject] = useState<Project | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [loadError, setLoadError] = useState(false)
  const [isRetrying, setIsRetrying] = useState(false)
  const requestSequence = useRef(0)

  const fetchProject = useCallback(async (silent = false) => {
    if (!uid) return
    const sequence = ++requestSequence.current
    if (!silent) setIsLoading(true)

    try {
      const response = await projectsAPI.get(uid)
      if (sequence !== requestSequence.current) return
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

  const progress = phase === 'ready' ? 100 : clampProgress(project?.deployment_progress)
  const stepStates = getStepStates(phase)
  const statusLabels = useMemo<Record<ProjectCreationPhase, string>>(() => ({
    deploying: t('projectDetail.provisioning.statusDeploying'),
    ready: t('projectDetail.provisioning.statusReady'),
    failed: t('projectDetail.provisioning.statusFailed'),
  }), [t])
  const title = phase === 'ready'
    ? t('projectDetail.provisioning.readyTitle', { name: project?.name || '' })
    : phase === 'failed'
      ? t('projectDetail.provisioning.failedTitle', { name: project?.name || '' })
      : t('projectDetail.provisioning.deployingTitle', { name: project?.name || '' })
  const description = phase === 'ready'
    ? t('projectDetail.provisioning.readyDesc')
    : phase === 'failed'
      ? t('projectDetail.provisioning.failedDesc')
      : t('projectDetail.provisioning.deployingDesc')

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
      } : current)
      toast.success(t('projectDetail.provisioning.retryStarted'))
      await fetchProject(true)
    } catch {
      toast.error(t('projectDetail.provisioning.retryFailed'))
    } finally {
      setIsRetrying(false)
    }
  }

  if (isLoading) {
    return (
      <div className="flex min-h-[calc(100dvh-12rem)] items-center justify-center" role="status" aria-live="polite">
        <div className="flex items-center gap-3 text-sm font-medium text-muted-foreground">
          <Loader2 className="size-5 animate-spin text-foreground" aria-hidden="true" />
          {t('projectDetail.provisioning.loading')}
        </div>
      </div>
    )
  }

  if (loadError || !project) {
    return (
      <div className="mx-auto flex min-h-[calc(100dvh-12rem)] w-full max-w-3xl items-center">
        <Card className="w-full shadow-none">
          <CardContent className="flex flex-col items-start gap-4 p-6 sm:p-8">
            <AlertTriangle className="size-6 text-destructive" aria-hidden="true" />
            <div>
              <h1 className="text-xl font-semibold tracking-tight">{t('projectDetail.provisioning.loadFailed')}</h1>
              <p className="mt-2 text-sm leading-6 text-muted-foreground">{t('projectDetail.provisioning.loadFailedDesc')}</p>
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

  const startedAt = project.deployment_started_at
    ? new Date(project.deployment_started_at).toLocaleString(language === 'id' ? 'id-ID' : 'en-US')
    : null

  return (
    <div className="mx-auto w-full max-w-5xl pb-16">
      <Button variant="ghost" size="sm" render={<Link to="/projects" />} className="mb-6 -ml-2 text-muted-foreground">
        <ArrowLeft className="size-4" aria-hidden="true" />
        {t('projectDetail.provisioning.backToProjects')}
      </Button>

      <Card className="overflow-hidden gap-0 py-0 shadow-none">
        <CardContent className="p-0">
          <div className="grid lg:grid-cols-[minmax(0,1fr)_20rem]">
            <section className="p-6 sm:p-8 lg:p-10">
              <div className="flex flex-wrap items-center gap-3">
                <Badge variant={phase === 'failed' ? 'destructive' : 'outline'} className={cn(
                  phase === 'ready' && 'border-emerald-500/30 bg-emerald-500/5 text-emerald-600 dark:text-emerald-400',
                  phase === 'deploying' && 'border-primary/25 bg-primary/5 text-primary',
                )}>
                  {phase === 'deploying' && <Loader2 className="animate-spin" aria-hidden="true" />}
                  {phase === 'ready' && <CheckCircle2 aria-hidden="true" />}
                  {phase === 'failed' && <AlertTriangle aria-hidden="true" />}
                  {statusLabels[phase]}
                </Badge>
                <span className="text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground">
                  {t('projectDetail.provisioning.eyebrow')}
                </span>
              </div>

              <h1 className="mt-6 max-w-2xl text-2xl font-semibold tracking-tight text-balance sm:text-3xl">
                {title}
              </h1>
              <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground text-pretty">
                {description}
              </p>

              <div className="mt-8 max-w-2xl">
                <Progress value={progress} aria-label={t('projectDetail.provisioning.progress')}>
                  <ProgressLabel>{t('projectDetail.provisioning.progress')}</ProgressLabel>
                  <ProgressValue>{() => `${progress}%`}</ProgressValue>
                </Progress>
              </div>

              <div className="mt-7 border-l-2 border-border pl-4" role="status" aria-live="polite">
                <p className="text-xs font-medium uppercase tracking-[0.14em] text-muted-foreground">
                  {t('projectDetail.provisioning.currentActivity')}
                </p>
                <p className="mt-2 text-sm font-medium leading-6 text-foreground">
                  {project.deployment_message || t('projectDetail.provisioning.waitingMessage')}
                </p>
              </div>

              <div className="mt-8 flex flex-wrap gap-3">
                {phase === 'ready' && (
                  <Button render={<Link to={`/projects/${uid}`} />}>
                    {t('projectDetail.provisioning.openProject')}
                  </Button>
                )}
                {(phase === 'deploying' || phase === 'failed') && (
                  <Button variant={phase === 'failed' ? 'default' : 'outline'} render={<Link to={`/projects/${uid}?tab=build`} />}>
                    <FileText className="size-4" aria-hidden="true" />
                    {t('projectDetail.provisioning.viewBuildLogs')}
                  </Button>
                )}
                {phase === 'failed' && (
                  <Button type="button" variant="outline" onClick={() => void handleRetry()} disabled={isRetrying}>
                    <RefreshCw className={cn('size-4', isRetrying && 'animate-spin')} aria-hidden="true" />
                    {t('projectDetail.provisioning.retryDeployment')}
                  </Button>
                )}
                <Button type="button" variant="ghost" onClick={() => void fetchProject(true)} disabled={isRetrying}>
                  <RefreshCw className="size-4" aria-hidden="true" />
                  {t('projectDetail.provisioning.refresh')}
                </Button>
              </div>
            </section>

            <aside className="border-t bg-muted/20 px-6 py-6 lg:border-l lg:border-t-0 lg:px-7 lg:py-9">
              <div className="divide-y">
                <ProvisioningStep label={t('projectDetail.provisioning.stepCreated')} state={stepStates[0]} />
                <ProvisioningStep label={t('projectDetail.provisioning.stepDeployment')} state={stepStates[1]} />
                <ProvisioningStep label={t('projectDetail.provisioning.stepReady')} state={stepStates[2]} />
              </div>

              <dl className="mt-8 space-y-5 border-t pt-6 text-sm">
                <div>
                  <dt className="text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
                    {t('projectDetail.provisioning.deploymentId')}
                  </dt>
                  <dd className="mt-1.5 break-all font-mono text-xs text-foreground">
                    {project.deployment_job_id || t('projectDetail.provisioning.pendingId')}
                  </dd>
                </div>
                {startedAt && (
                  <div>
                    <dt className="text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
                      {t('projectDetail.provisioning.startedAt')}
                    </dt>
                    <dd className="mt-1.5 text-sm text-foreground">{startedAt}</dd>
                  </div>
                )}
              </dl>
            </aside>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
