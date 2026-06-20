import React, { useState, useEffect, useCallback, useRef } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { projectsAPI } from '../../services/api'
import {
  Plus,
  Rocket,
  ExternalLink,
  Trash2,
  Database,
  Globe,
  Cpu,
  ArrowRight,
  Loader2,
  CalendarDays
} from 'lucide-react'
import useTranslation from '../../lib/useTranslation'
import ConfirmationModal from '../../components/ConfirmationModal'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { usePolling } from '@/lib/usePolling'
import { cn } from '@/lib/utils'
import { FrameworkIcon } from '../../components/FrameworkIcon'
import { RedeployButton } from '../../components/project/RedeployButton'
import { RestartButton } from '../../components/project/RestartButton'
import { getEngineDisplayName } from '../../components/database-studio/utils'
import { Project } from '../../types'

const getFrameworkLabel = (framework?: string, fallback?: string) => {
  if (!framework || framework === 'Other') return fallback || ''
  return framework
}

type FrameworkTone = {
  spotlight: string
  surface: string
  border: string
  chip: string
  divider: string
}

const getFrameworkTone = (framework?: string): FrameworkTone => {
  const fw = (framework || '').toLowerCase().trim()

  if (fw.includes('laravel') || fw.includes('php')) {
    return {
      spotlight: 'text-rose-500',
      surface: 'bg-rose-500/15',
      border: 'ring-rose-500/20',
      chip: 'border-rose-500/20 bg-rose-500/10 text-rose-600 dark:text-rose-300',
      divider: 'from-rose-500/0 via-rose-500/60 to-rose-500/0',
    }
  }

  if (fw === 'go' || fw.includes('golang')) {
    return {
      spotlight: 'text-cyan-500',
      surface: 'bg-cyan-500/15',
      border: 'ring-cyan-500/20',
      chip: 'border-cyan-500/20 bg-cyan-500/10 text-cyan-700 dark:text-cyan-300',
      divider: 'from-cyan-500/0 via-cyan-500/60 to-cyan-500/0',
    }
  }

  if (fw.includes('python') || fw.includes('django') || fw.includes('flask')) {
    return {
      spotlight: 'text-amber-500',
      surface: 'bg-amber-500/15',
      border: 'ring-amber-500/20',
      chip: 'border-amber-500/20 bg-amber-500/10 text-amber-700 dark:text-amber-300',
      divider: 'from-amber-500/0 via-amber-500/60 to-amber-500/0',
    }
  }

  if (fw.includes('next') || fw.includes('node') || fw.includes('express')) {
    return {
      spotlight: 'text-slate-500 dark:text-white',
      surface: 'bg-slate-500/15 dark:bg-white/10',
      border: 'ring-slate-500/20 dark:ring-white/15',
      chip: 'border-slate-500/20 bg-slate-500/10 text-slate-700 dark:border-white/15 dark:bg-white/10 dark:text-white',
      divider: 'from-slate-500/0 via-slate-500/50 to-slate-500/0 dark:from-white/0 dark:via-white/50 dark:to-white/0',
    }
  }

  return {
    spotlight: 'text-primary',
    surface: 'bg-primary/10',
    border: 'ring-primary/20',
    chip: 'border-primary/20 bg-primary/10 text-primary',
    divider: 'from-primary/0 via-primary/50 to-primary/0',
  }
}

const StatusBadge = ({ status }: { status: Project['status'] }) => {
  const { t } = useTranslation()
  const configs: Record<Project['status'], { badge: string, dot: string, label: string, pulse?: boolean }> = {
    pending: { badge: 'border-amber-500/20 bg-amber-500/10 text-amber-700 dark:text-amber-300', dot: 'bg-amber-500', label: t('status.pending') },
    queued: { badge: 'border-purple-500/20 bg-purple-500/10 text-purple-700 dark:text-purple-300', dot: 'bg-purple-500', label: t('status.queued'), pulse: true },
    deploying: { badge: 'border-blue-500/20 bg-blue-500/10 text-blue-700 dark:text-blue-300', dot: 'bg-blue-500', label: t('status.building'), pulse: true },
    building: { badge: 'border-blue-500/20 bg-blue-500/10 text-blue-700 dark:text-blue-300', dot: 'bg-blue-500', label: t('status.building'), pulse: true },
    running: { badge: 'border-emerald-500/20 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300', dot: 'bg-emerald-500', label: t('status.running'), pulse: true },
    failed: { badge: 'border-rose-500/20 bg-rose-500/10 text-rose-700 dark:text-rose-300', dot: 'bg-rose-500', label: t('status.failed') },
    error: { badge: 'border-rose-500/20 bg-rose-500/10 text-rose-700 dark:text-rose-300', dot: 'bg-rose-500', label: t('status.failed') },
    stopped: { badge: 'border-slate-500/20 bg-slate-500/10 text-slate-700 dark:text-slate-300', dot: 'bg-slate-500', label: t('status.stopped') },
    restarting: { badge: 'border-indigo-500/20 bg-indigo-500/10 text-indigo-700 dark:text-indigo-300', dot: 'bg-indigo-500', label: t('status.restarting'), pulse: true },
  }

  const config = configs[status] || configs.pending

  return (
    <Badge variant="outline" className={cn('inline-flex w-fit items-center gap-2 rounded-full px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wider shadow-sm backdrop-blur-sm', config.badge)}>
      <span className="relative flex h-2 w-2 shrink-0" aria-hidden="true">
        {config.pulse && <span className={cn('absolute inline-flex h-full w-full animate-ping rounded-full opacity-60', config.dot)} />}
        <span className={cn('relative inline-flex h-2 w-2 rounded-full', config.dot)} />
      </span>
      {config.label}
    </Badge>
  )
}

type ProjectCardProps = {
  project: Project
  onNavigate: (uid: string) => void
  onDelete: (uid: string, e: React.MouseEvent) => void
  onActionStarted: (uid: string, status?: Project['status']) => void
  onSuccess: () => void
}

function ProjectCard({ project, onNavigate, onDelete, onActionStarted, onSuccess }: ProjectCardProps) {
  const { t } = useTranslation()
  const tone = getFrameworkTone(project.framework)
  const projectHost = project.url ? project.url.replace(/^https?:\/\//, '') : project.subdomain
  const runtimeLabel = project.framework === 'Laravel' ? t('projectDetail.metrics.php') : t('projectDetail.metrics.framework')
  const runtimeValue = project.framework === 'Laravel'
    ? (project.php_version ? `${t('projectDetail.settings.version')} ${project.php_version}` : t('projectDetail.metrics.inactive'))
    : getFrameworkLabel(project.framework, t('common.general'))
  const databaseValue = project.database_name
    ? getEngineDisplayName(project.database_instance?.engine)
    : t('projectDetail.metrics.inactive')

  const handlePointerMove = (event: React.PointerEvent<HTMLDivElement>) => {
    const bounds = event.currentTarget.getBoundingClientRect()
    event.currentTarget.style.setProperty('--spotlight-x', `${event.clientX - bounds.left}px`)
    event.currentTarget.style.setProperty('--spotlight-y', `${event.clientY - bounds.top}px`)
  }

  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>) => {
    if (event.currentTarget !== event.target) return
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      onNavigate(project.uid)
    }
  }

  return (
    <Card
      role="link"
      tabIndex={0}
      aria-label={project.name}
      onClick={() => onNavigate(project.uid)}
      onKeyDown={handleKeyDown}
      onPointerMove={handlePointerMove}
      className={cn(
        'group relative flex h-full cursor-pointer overflow-hidden rounded-2xl border border-border/50 bg-card/80 p-0 py-0 shadow-sm shadow-black/5 outline-none transition-[transform,box-shadow,border-color,background-color] duration-300 ease-out hover:-translate-y-0.5 hover:border-border/80 hover:bg-card hover:shadow-xl hover:shadow-black/10 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background dark:bg-card/70 dark:shadow-black/30 dark:hover:shadow-black/40',
        tone.border
      )}
    >
      <div
        className={cn('pointer-events-none absolute inset-0 opacity-0 transition-opacity duration-300 group-hover:opacity-10 group-focus-visible:opacity-10', tone.spotlight)}
        style={{ background: 'radial-gradient(420px circle at var(--spotlight-x,50%) var(--spotlight-y,50%), currentColor, transparent 72%)' }}
        aria-hidden="true"
      />
      <div className={cn('pointer-events-none absolute inset-0 rounded-2xl opacity-0 ring-1 ring-inset transition-opacity duration-300 group-hover:opacity-100 group-focus-visible:opacity-100', tone.border)} aria-hidden="true" />
      <div className={cn('pointer-events-none absolute inset-x-8 top-0 h-px bg-gradient-to-r', tone.divider)} aria-hidden="true" />

      <CardContent className="relative z-10 flex h-full flex-col p-6">
        <div className="mb-8 flex items-start justify-between gap-4">
          <div className="relative transition-transform duration-300 ease-out group-hover:scale-[1.04]">
            <div className={cn('absolute -inset-2 rounded-2xl blur-xl opacity-40 transition-opacity duration-300 group-hover:opacity-70', tone.surface)} aria-hidden="true" />
            <FrameworkIcon framework={project.framework} variant="tile" className="relative h-11 w-11" />
          </div>
          <StatusBadge status={project.status} />
        </div>

        <div className="mb-6 min-w-0 flex-1">
          <h3 className="mb-3 truncate text-xl font-bold tracking-tight" title={project.name}>
            {project.name}
          </h3>
          <div className="mb-3">
            <Badge variant="outline" className={cn('gap-1.5 rounded-full px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wider shadow-sm backdrop-blur-sm', tone.chip)}>
              <FrameworkIcon framework={project.framework} variant="plain" className="h-3.5 w-3.5" />
              {getFrameworkLabel(project.framework, t('common.general'))}
            </Badge>
          </div>
          <a
            href={project.url}
            target="_blank"
            rel="noopener noreferrer"
            onClick={(e) => e.stopPropagation()}
            className="inline-flex max-w-full items-center gap-2 rounded-lg border border-border/50 bg-background/50 px-2.5 py-1.5 font-mono text-xs text-muted-foreground shadow-sm shadow-black/5 transition-[border-color,background-color,color] duration-150 hover:border-border hover:bg-muted hover:text-foreground dark:bg-background/20"
            title={projectHost}
          >
            <Globe className="h-3.5 w-3.5 shrink-0" />
            <span className="truncate">{projectHost}</span>
            <ExternalLink className="h-3 w-3 shrink-0" />
          </a>
        </div>

        <div className="-mx-6 grid grid-cols-2 divide-x divide-border/50 border-y border-border/50 bg-muted/20 dark:bg-muted/10 dark:divide-white/5 dark:border-white/5">
          <div className="min-w-0 p-4">
            <div className="mb-2 flex items-center gap-1.5 text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
              <Cpu className="h-3.5 w-3.5 shrink-0" />
              <span className="truncate">{runtimeLabel}</span>
            </div>
            <p className="truncate text-xs font-semibold tabular-nums text-foreground" title={runtimeValue}>
              {runtimeValue}
            </p>
          </div>
          <div className="min-w-0 p-4">
            <div className="mb-2 flex items-center gap-1.5 text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
              <Database className="h-3.5 w-3.5 shrink-0" />
              <span className="truncate">{t('projectDetail.metrics.db')}</span>
            </div>
            <p className={cn('truncate text-xs font-semibold tabular-nums', project.database_name ? 'text-foreground' : 'text-muted-foreground')} title={databaseValue}>
              {databaseValue}
            </p>
          </div>
        </div>

        <div className="mt-6 flex items-center justify-between gap-4">
          <div className="flex min-w-0 items-center gap-2 text-muted-foreground">
            <CalendarDays className="h-4 w-4 shrink-0" />
            <div className="min-w-0">
              <span className="block text-[10px] font-semibold uppercase tracking-widest">{t('common.date')}</span>
              <span className="block truncate text-xs font-medium tabular-nums text-foreground">
                {new Date(project.created_at).toLocaleDateString()}
              </span>
            </div>
          </div>

          <div className="flex shrink-0 items-center gap-1 rounded-full border border-border/60 bg-background/80 p-1 shadow-sm shadow-black/10 backdrop-blur-md transition-[opacity,transform,border-color,background-color] duration-200 sm:translate-x-1 sm:opacity-70 group-hover:translate-x-0 group-hover:opacity-100 group-focus-within:translate-x-0 group-focus-within:opacity-100 dark:bg-background/40 dark:shadow-black/30">
            <RestartButton
              projectId={project.uid}
              status={project.status}
              variant="ghost"
              size="icon"
              className="h-9 w-9 rounded-full text-muted-foreground transition-transform hover:scale-[1.03] hover:text-foreground active:scale-[0.97]"
              onStarted={() => onActionStarted(project.uid, 'restarting')}
              onSuccess={onSuccess}
            />
            <RedeployButton
              projectId={project.uid}
              status={project.status}
              variant="ghost"
              size="icon"
              showOptions={false}
              className="h-9 w-9 rounded-full text-muted-foreground transition-transform hover:scale-[1.03] hover:text-foreground active:scale-[0.97]"
              onStarted={() => onActionStarted(project.uid, 'queued')}
              onSuccess={onSuccess}
            />
            <Button
              variant="ghost"
              size="icon"
              onClick={(e) => onDelete(project.uid, e)}
              className="h-9 w-9 rounded-full text-muted-foreground transition-transform hover:scale-[1.03] hover:bg-destructive/10 hover:text-destructive active:scale-[0.97]"
              title={t('projectDetail.actions.delete')}
              aria-label={t('projectDetail.actions.delete')}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
            <Button
              size="icon"
              onClick={(e) => {
                e.preventDefault()
                e.stopPropagation()
                onNavigate(project.uid)
              }}
              className="h-9 w-9 rounded-full transition-transform hover:scale-[1.03] active:scale-[0.97]"
              title={project.name}
              aria-label={project.name}
            >
              <ArrowRight className="h-4 w-4 transition-transform duration-150 group-hover:translate-x-0.5" />
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

const UserProjects = () => {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [projects, setProjects] = useState<Project[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const isFirstLoad = useRef(true)

  const [confirmModal, setConfirmModal] = useState({
    isOpen: false,
    title: '',
    message: '',
    type: 'danger' as 'danger' | 'warning' | 'info',
    onConfirm: () => {},
    confirmText: t('common.save')
  })

  const fetchProjects = useCallback(async () => {
    if (isFirstLoad.current) {
      setIsLoading(true)
    }

    try {
      const response = await projectsAPI.listOwn()
      setProjects(response.data.data || [])
    } catch (error) {
      toast.error(t('common.loadError'), { id: 'projects-load-error' })
    } finally {
      setIsLoading(false)
      isFirstLoad.current = false
    }
  }, [t])

  useEffect(() => {
    fetchProjects()
  }, [fetchProjects])

  // Poll for status updates every 5 seconds
  usePolling(fetchProjects, 5000)
  const onActionStarted = (uid: string, status: Project['status'] = 'queued') => {
    setProjects(prev => prev.map(p => p.uid === uid ? { ...p, status } : p))
  }


  const handleDelete = async (uid: string, e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()

    setConfirmModal({
      isOpen: true,
      title: t('projectDetail.messages.deleteConfirm'),
      message: t('projectDetail.messages.deleteDesc'),
      type: 'danger',
      confirmText: t('common.delete'),
      onConfirm: async () => {
        try {
          await projectsAPI.delete(uid)
          toast.success(t('common.success'))
          fetchProjects()
        } catch (error: unknown) {
          toast.error(t('common.error'))
        }
      }
    })
  }

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      <ConfirmationModal
        onClose={() => setConfirmModal(prev => ({ ...prev, isOpen: false }))}
        {...confirmModal}
      />

      {/* Header Container */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-6 pb-4 border-b">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">{t('user.projects.title')}</h1>
          <p className="text-muted-foreground max-w-2xl">
            {t('user.projects.desc')}
          </p>
        </div>
        <Link to="/projects/new" className={cn(buttonVariants({ variant: "default", size: "lg" }), "w-full md:w-auto font-semibold")}>
          <Plus className="w-5 h-5 mr-2" />
          {t('common.newProject')}
        </Link>
      </div>

      {/* Grid Architecture */}
      {isLoading ? (
        <div className="flex flex-col items-center justify-center py-40 gap-6 opacity-80">
          <Loader2 className="w-12 h-12 text-primary animate-spin" />
          <p className="text-xs font-bold uppercase tracking-widest text-muted-foreground animate-pulse">{t('dashboard.loadingProjects')}</p>
        </div>
      ) : (!projects || projects.length === 0) ? (
        <Card className="p-24 text-center flex flex-col items-center max-w-xl mx-auto border-dashed">
          <div className="w-20 h-20 bg-muted rounded-full flex items-center justify-center mb-6">
            <Rocket className="w-10 h-10 text-muted-foreground opacity-50" />
          </div>
          <h2 className="text-2xl font-bold tracking-tight mb-2">{t('dashboard.noProjectsFound')}</h2>
          <p className="text-muted-foreground mb-8 max-w-sm">{t('dashboard.noProjectsDesc')}</p>
          <Link to="/projects/new" className={cn(buttonVariants({ variant: "default", size: "lg" }), "w-full sm:w-auto")}>
            {t('common.newProject')}
          </Link>
        </Card>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 gap-6 pb-12">
          {projects.map((project) => (
            <ProjectCard
              key={project.uid}
              project={project}
              onNavigate={(uid) => navigate(`/projects/${uid}`)}
              onDelete={handleDelete}
              onActionStarted={onActionStarted}
              onSuccess={fetchProjects}
            />
          ))}
        </div>
      )}
    </div>
  )
}

export default UserProjects
