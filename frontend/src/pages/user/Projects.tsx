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
import { DEFAULT_RUNTIME_VERSIONS } from '../../lib/runtimes'
import { Project } from '../../types'

const getFrameworkLabel = (framework?: string, fallback?: string) => {
  if (!framework || framework === 'Other') return fallback || ''
  return framework
}

const isLaravelFramework = (framework?: string) => (framework || '').toLowerCase().includes('laravel')

type FrameworkTone = {
  chip: string
  divider: string
}

const getFrameworkTone = (framework?: string): FrameworkTone => {
  const fw = (framework || '').toLowerCase().trim()

  if (fw.includes('laravel') || fw.includes('php')) {
    return {
      chip: 'border-rose-500/20 bg-rose-500/10 text-rose-600 dark:text-rose-300',
      divider: 'from-rose-500/0 via-rose-500/60 to-rose-500/0',
    }
  }

  if (fw === 'go' || fw.includes('golang')) {
    return {
      chip: 'border-cyan-500/20 bg-cyan-500/10 text-cyan-700 dark:text-cyan-300',
      divider: 'from-cyan-500/0 via-cyan-500/60 to-cyan-500/0',
    }
  }

  if (fw.includes('python') || fw.includes('django') || fw.includes('flask')) {
    return {
      chip: 'border-amber-500/20 bg-amber-500/10 text-amber-700 dark:text-amber-300',
      divider: 'from-amber-500/0 via-amber-500/60 to-amber-500/0',
    }
  }

  if (fw.includes('next') || fw.includes('node') || fw.includes('express')) {
    return {
      chip: 'border-slate-500/20 bg-slate-500/10 text-slate-700 dark:border-white/15 dark:bg-white/10 dark:text-white',
      divider: 'from-slate-500/0 via-slate-500/50 to-slate-500/0 dark:from-white/0 dark:via-white/50 dark:to-white/0',
    }
  }

  return {
    chip: 'border-primary/20 bg-primary/10 text-primary',
    divider: 'from-primary/0 via-primary/50 to-primary/0',
  }
}

const StatusBadge = ({ status }: { status: Project['status'] }) => {
  const { t } = useTranslation()
  const configs: Record<Project['status'], { badge: string, dot: string, label: string }> = {
    pending: { badge: 'border-amber-500/20 bg-amber-500/10 text-amber-700 dark:text-amber-300', dot: 'bg-amber-500', label: t('status.pending') },
    queued: { badge: 'border-purple-500/20 bg-purple-500/10 text-purple-700 dark:text-purple-300', dot: 'bg-purple-500', label: t('status.queued') },
    deploying: { badge: 'border-blue-500/20 bg-blue-500/10 text-blue-700 dark:text-blue-300', dot: 'bg-blue-500', label: t('status.building') },
    building: { badge: 'border-blue-500/20 bg-blue-500/10 text-blue-700 dark:text-blue-300', dot: 'bg-blue-500', label: t('status.building') },
    running: { badge: 'border-emerald-500/20 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300', dot: 'bg-emerald-500', label: t('status.running') },
    failed: { badge: 'border-rose-500/20 bg-rose-500/10 text-rose-700 dark:text-rose-300', dot: 'bg-rose-500', label: t('status.failed') },
    error: { badge: 'border-rose-500/20 bg-rose-500/10 text-rose-700 dark:text-rose-300', dot: 'bg-rose-500', label: t('status.failed') },
    stopped: { badge: 'border-slate-500/20 bg-slate-500/10 text-slate-700 dark:text-slate-300', dot: 'bg-slate-500', label: t('status.stopped') },
    restarting: { badge: 'border-indigo-500/20 bg-indigo-500/10 text-indigo-700 dark:text-indigo-300', dot: 'bg-indigo-500', label: t('status.restarting') },
  }

  const config = configs[status] || configs.pending

  return (
    <Badge variant="outline" className={cn('inline-flex w-fit items-center gap-2 rounded-full px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wider', config.badge)}>
      <span className={cn('h-2 w-2 shrink-0 rounded-full', config.dot)} aria-hidden="true" />
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
  const isLaravel = isLaravelFramework(project.framework)
  const projectHost = project.url ? project.url.replace(/^https?:\/\//, '') : project.subdomain
  const runtimeLabel = isLaravel ? t('projectDetail.metrics.php') : t('projectDetail.metrics.framework')
  const phpVersion = project.php_version?.trim() || DEFAULT_RUNTIME_VERSIONS.php
  const runtimeValue = isLaravel
    ? `${t('projectDetail.settings.version')} ${phpVersion}`
    : getFrameworkLabel(project.framework, t('common.general'))
  const databaseValue = project.database_name
    ? getEngineDisplayName(project.database_instance?.engine)
    : t('projectDetail.metrics.inactive')

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
      className="group relative flex h-full cursor-pointer overflow-hidden rounded-2xl border border-border/50 bg-card/95 p-0 py-0 outline-none transition-colors duration-150 hover:border-border/80 hover:bg-card focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background dark:bg-card/85"
    >
      <CardContent className="relative z-10 flex h-full flex-col p-6">
        <div className="mb-8 flex items-start justify-between gap-4">
          <div className="relative">
            <FrameworkIcon framework={project.framework} variant="tile" className="h-11 w-11" />
          </div>
          <StatusBadge status={project.status} />
        </div>

        <div className="mb-6 min-w-0 flex-1">
          <h3 className="mb-3 truncate text-xl font-bold tracking-tight" title={project.name}>
            {project.name}
          </h3>
          <div className="mb-3">
            <Badge variant="outline" className={cn('gap-1.5 rounded-full px-2.5 py-1 text-[10px] font-semibold uppercase tracking-wider', tone.chip)}>
              <FrameworkIcon framework={project.framework} variant="plain" className="h-3.5 w-3.5" />
              {getFrameworkLabel(project.framework, t('common.general'))}
            </Badge>
          </div>
          <a
            href={project.url}
            target="_blank"
            rel="noopener noreferrer"
            onClick={(e) => e.stopPropagation()}
            className="inline-flex max-w-full items-center gap-2 rounded-lg border border-border/50 bg-background/50 px-2.5 py-1.5 font-mono text-xs text-muted-foreground transition-colors duration-150 hover:border-border hover:bg-muted hover:text-foreground dark:bg-background/20"
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

          <div className="flex shrink-0 items-center gap-1 rounded-full border border-border/60 bg-background/80 p-1 transition-opacity duration-150 sm:opacity-80 group-hover:opacity-100 group-focus-within:opacity-100 dark:bg-background/40">
            <RestartButton
              projectId={project.uid}
              status={project.status}
              variant="ghost"
              size="icon"
              className="h-9 w-9 rounded-full text-muted-foreground transition-colors duration-150 hover:text-foreground"
              onStarted={() => onActionStarted(project.uid, 'restarting')}
              onSuccess={onSuccess}
            />
            <RedeployButton
              projectId={project.uid}
              status={project.status}
              variant="ghost"
              size="icon"
              showOptions={false}
              className="h-9 w-9 rounded-full text-muted-foreground transition-colors duration-150 hover:text-foreground"
              onStarted={() => onActionStarted(project.uid, 'queued')}
              onSuccess={onSuccess}
            />
            <Button
              variant="ghost"
              size="icon"
              onClick={(e) => onDelete(project.uid, e)}
              className="h-9 w-9 rounded-full text-muted-foreground transition-colors duration-150 hover:bg-destructive/10 hover:text-destructive"
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
              className="h-9 w-9 rounded-full"
              title={project.name}
              aria-label={project.name}
            >
              <ArrowRight className="h-4 w-4" />
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
