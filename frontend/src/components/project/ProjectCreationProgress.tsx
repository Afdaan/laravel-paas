import type { ReactNode } from 'react'
import { AlertTriangle, CheckCircle2, Circle, Loader2, X } from 'lucide-react'

import useTranslation from '@/lib/useTranslation'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import type { ProjectCreationPhase } from './projectCreationContext'

export type { ProjectCreationPhase }

export function ProjectCreationLoading({ projectName }: { projectName?: string }) {
  const { t } = useTranslation()
  const name = projectName || t('projectDetail.messages.creationProjectFallback')

  return (
    <div className="mx-auto flex min-h-[calc(100dvh-12rem)] w-full max-w-4xl items-center py-8" role="status" aria-live="polite">
      <Card className="w-full gap-0 py-0 shadow-none">
        <CardContent className="p-0">
          <div className="grid lg:grid-cols-[minmax(0,1fr)_18rem]">
            <div className="p-6 sm:p-8 lg:p-10">
              <div className="inline-flex items-center gap-2 text-sm font-medium text-foreground">
                <Loader2 className="size-3.5 animate-spin" aria-hidden="true" />
                {t('projectDetail.messages.creationInProgress')}
              </div>

              <h1 className="mt-5 max-w-2xl text-2xl font-semibold tracking-tight text-balance sm:text-3xl">
                {t('projectDetail.messages.creationPreparingTitle', { name })}
              </h1>
              <p className="mt-3 max-w-xl text-sm leading-6 text-muted-foreground text-pretty">
                {t('projectDetail.messages.creationPreparingDesc')}
              </p>
            </div>

            <div className="border-t bg-muted/20 px-6 py-5 lg:border-l lg:border-t-0 lg:px-7 lg:py-8">
              <div className="divide-y">
                <CreationStep icon={<CheckCircle2 className="size-3.5" />} label={t('projectDetail.messages.creationCreated')} state="complete" />
                <CreationStep icon={<Loader2 className="size-3.5 animate-spin" />} label={t('projectDetail.messages.creationDeployment')} state="active" />
                <CreationStep icon={<Circle className="size-3.5" />} label={t('projectDetail.messages.creationReady')} state="pending" />
              </div>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

function CreationStep({ icon, label, state }: { icon: ReactNode; label: string; state: 'complete' | 'active' | 'pending' }) {
  return (
    <div className="flex items-center gap-3 py-4 first:pt-0 last:pb-0">
      <div className={cn(
        'flex size-6 shrink-0 items-center justify-center rounded-full border bg-background',
        state === 'complete' && 'border-emerald-500/30 text-emerald-500',
        state === 'active' && 'border-foreground/20 text-foreground',
        state === 'pending' && 'border-border text-muted-foreground/40',
      )}>
        {icon}
      </div>
      <span className={cn(
        'text-sm font-medium',
        state === 'active' && 'text-foreground',
        state === 'pending' && 'text-muted-foreground/60',
      )}>
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
