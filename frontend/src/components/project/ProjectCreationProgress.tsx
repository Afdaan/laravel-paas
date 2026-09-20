import type { ReactNode } from 'react'
import { AlertTriangle, CheckCircle2, Circle, Loader2, Rocket, X } from 'lucide-react'

import useTranslation from '@/lib/useTranslation'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { ProjectCreationPhase } from './projectCreationContext'

export type { ProjectCreationPhase }

export function ProjectCreationLoading({ projectName }: { projectName?: string }) {
  const { t } = useTranslation()
  const name = projectName || t('projectDetail.messages.creationProjectFallback')

  return (
    <div className="mx-auto flex min-h-[calc(100dvh-12rem)] w-full max-w-3xl items-center justify-center py-8" role="status" aria-live="polite">
      <div className="w-full overflow-hidden rounded-2xl border border-emerald-500/20 bg-card shadow-sm">
        <div className="h-1 w-full bg-muted">
          <div className="h-full w-2/3 animate-pulse bg-emerald-500" />
        </div>

        <div className="p-6 sm:p-8">
          <div className="flex flex-col gap-6 sm:flex-row sm:items-start">
            <div className="relative flex size-14 shrink-0 items-center justify-center rounded-2xl border border-emerald-500/20 bg-emerald-500/10 text-emerald-500">
              <Rocket className="size-6" aria-hidden="true" />
              <span className="absolute -right-1 -top-1 flex size-5 items-center justify-center rounded-full border-2 border-card bg-emerald-500 text-white">
                <CheckCircle2 className="size-3" aria-hidden="true" />
              </span>
            </div>

            <div className="min-w-0 flex-1">
              <p className="text-xs font-semibold uppercase tracking-[0.18em] text-emerald-500">
                {t('projectDetail.messages.creationCreated')}
              </p>
              <h1 className="mt-2 text-2xl font-bold tracking-tight sm:text-3xl">
                {t('projectDetail.messages.creationPreparingTitle', { name })}
              </h1>
              <p className="mt-3 max-w-2xl text-sm leading-relaxed text-muted-foreground">
                {t('projectDetail.messages.creationPreparingDesc')}
              </p>
            </div>
          </div>

          <div className="mt-8 divide-y rounded-xl border bg-muted/20 px-4">
            <CreationStep icon={<CheckCircle2 className="size-4" />} label={t('projectDetail.messages.creationCreated')} state="complete" />
            <CreationStep icon={<Loader2 className="size-4 animate-spin" />} label={t('projectDetail.messages.creationDeployment')} state="active" />
            <CreationStep icon={<Circle className="size-4" />} label={t('projectDetail.messages.creationReady')} state="pending" />
          </div>
        </div>
      </div>
    </div>
  )
}

function CreationStep({ icon, label, state }: { icon: ReactNode; label: string; state: 'complete' | 'active' | 'pending' }) {
  return (
    <div className="flex items-center gap-3 py-3.5">
      <div className={cn(
        'flex size-8 shrink-0 items-center justify-center rounded-lg border',
        state === 'complete' && 'border-emerald-500/20 bg-emerald-500/10 text-emerald-500',
        state === 'active' && 'border-blue-500/20 bg-blue-500/10 text-blue-500',
        state === 'pending' && 'border-border bg-background text-muted-foreground/50',
      )}>
        {icon}
      </div>
      <span className={cn('text-sm font-medium', state === 'pending' && 'text-muted-foreground')}>
        {label}
      </span>
    </div>
  )
}

export function ProjectCreationBanner({ phase, onDismiss }: { phase: ProjectCreationPhase; onDismiss: () => void }) {
  const { t } = useTranslation()
  const content = {
    deploying: {
      icon: <Loader2 className="size-5 animate-spin" />,
      title: t('projectDetail.messages.creationSuccessTitle'),
      description: t('projectDetail.messages.creationSuccessDesc'),
      tone: 'border-blue-500/20 bg-blue-500/5 text-blue-500',
    },
    ready: {
      icon: <CheckCircle2 className="size-5" />,
      title: t('projectDetail.messages.creationReadyTitle'),
      description: t('projectDetail.messages.creationReadyDesc'),
      tone: 'border-emerald-500/20 bg-emerald-500/5 text-emerald-500',
    },
    failed: {
      icon: <AlertTriangle className="size-5" />,
      title: t('projectDetail.messages.creationFailedTitle'),
      description: t('projectDetail.messages.creationFailedDesc'),
      tone: 'border-rose-500/20 bg-rose-500/5 text-rose-500',
    },
  }[phase]

  return (
    <div className={cn('flex items-start gap-4 rounded-xl border p-4 sm:p-5', content.tone)} role="status" aria-live="polite">
      <div className="flex size-10 shrink-0 items-center justify-center rounded-xl border border-current/15 bg-background/70">
        {content.icon}
      </div>
      <div className="min-w-0 flex-1">
        <h2 className="font-semibold tracking-tight text-foreground">{content.title}</h2>
        <p className="mt-1 text-sm leading-relaxed text-muted-foreground">{content.description}</p>
      </div>
      <Button type="button" variant="ghost" size="icon" onClick={onDismiss} className="size-8 shrink-0 text-muted-foreground" aria-label={t('common.close')}>
        <X className="size-4" aria-hidden="true" />
      </Button>
    </div>
  )
}
