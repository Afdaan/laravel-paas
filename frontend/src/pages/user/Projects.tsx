import React, { useState, useEffect, useCallback, useRef } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { projectsAPI } from '../../services/api'
import {
  Plus,
  Rocket,
  ExternalLink,
  Trash2,
  Clock,
  CheckCircle2,
  AlertCircle,
  PauseCircle,
  Database,
  Globe,
  Cpu,
  ArrowRight,
  Loader2
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

const StatusBadge = ({ status }: { status: Project['status'] }) => {
  const { t } = useTranslation()
  const configs: Record<string, { color: string, icon: React.ElementType, label: string, pulse?: boolean }> = {
    pending: { color: 'text-amber-600 border-amber-500/20 bg-amber-500/10', icon: Clock, label: t('status.pending') },
    queued: { color: 'text-purple-600 border-purple-500/20 bg-purple-500/10', icon: Clock, label: t('status.queued') },
    building: { color: 'text-blue-600 border-blue-500/20 bg-blue-500/10', icon: Loader2, label: t('status.building'), pulse: true },
    running: { color: 'text-emerald-600 border-emerald-500/20 bg-emerald-500/10', icon: CheckCircle2, label: t('status.running') },
    failed: { color: 'text-rose-600 border-rose-500/20 bg-rose-500/10', icon: AlertCircle, label: t('status.failed') },
    stopped: { color: 'text-slate-600 border-slate-500/20 bg-slate-500/10 dark:text-slate-400', icon: PauseCircle, label: t('status.stopped') },
    restarting: { color: 'text-indigo-600 border-indigo-500/20 bg-indigo-500/10', icon: Loader2, label: t('status.restarting'), pulse: true },
  }

  const config = configs[status] || configs.pending
  const Icon = config.icon

  return (
    <Badge variant="outline" className={`gap-1.5 flex w-fit ${config.color}`}>
      <Icon className={`w-3 h-3 ${config.pulse ? 'animate-spin' : ''}`} />
      {config.label}
    </Badge>
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
            <Card
              key={project.uid}
              onClick={() => navigate(`/projects/${project.uid}`)}
              className="group flex flex-col h-full hover:border-primary/30 transition-all cursor-pointer overflow-hidden border-border/50"
            >
              <CardContent className="p-6 flex flex-col h-full">
                <div className="flex items-start justify-between mb-8">
                  <div className="transition-transform duration-300 group-hover:scale-105">
                     <FrameworkIcon framework={project.framework} variant="tile" className="h-11 w-11" />
                  </div>
                  <StatusBadge status={project.status} />
                </div>

                <div className="mb-6 flex-1">
                  <h3 className="font-bold text-xl tracking-tight mb-3 truncate" title={project.name}>
                    {project.name}
                  </h3>
                  <div className="mb-3">
                    <Badge variant="outline" className="gap-1.5 bg-muted/40 border-border/60 text-[10px] uppercase tracking-wider font-semibold">
                      <FrameworkIcon framework={project.framework} variant="plain" className="w-3.5 h-3.5" />
                      {getFrameworkLabel(project.framework, t('common.general'))}
                    </Badge>
                  </div>
                  <a
                    href={project.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    onClick={(e) => e.stopPropagation()}
                    className="flex items-center gap-2 text-muted-foreground font-mono text-xs hover:text-primary transition-colors bg-muted/50 px-2 py-1.5 rounded-md hover:bg-muted w-fit max-w-full"
                  >
                    <Globe className="w-3.5 h-3.5 shrink-0" />
                    <span className="truncate">{project.url ? project.url.replace(/^https?:\/\//, '') : project.subdomain}</span>
                    <ExternalLink className="w-3 h-3 shrink-0" />
                  </a>
                </div>

                <div className="space-y-4 py-6 border-y border-border/50 bg-muted/10 -mx-6 px-6">
                  <div className="flex items-center justify-between text-sm">
                    <div className="flex items-center gap-2 text-muted-foreground">
                      <Cpu className="w-4 h-4" />
                      <span>{project.framework === 'Laravel' ? t('projectDetail.metrics.php') : t('projectDetail.metrics.framework')}</span>
                    </div>
                      <Badge variant="secondary" className="font-mono text-[10px]">
                        {project.framework === 'Laravel'
                          ? `${t('projectDetail.settings.version')} ${project.php_version}`
                          : getFrameworkLabel(project.framework, t('common.general'))}
                      </Badge>
                  </div>
                  <div className="flex items-center justify-between text-sm">
                    <div className="flex items-center gap-2 text-muted-foreground">
                      <Database className="w-4 h-4" />
                      <span>{t('projectDetail.metrics.db')}</span>
                    </div>
                    <span className="text-xs font-semibold text-primary">
                      {project.database_name
                        ? getEngineDisplayName(project.database_instance?.engine)
                        : t('projectDetail.metrics.inactive')}
                    </span>
                  </div>
                </div>

                <div className="mt-6 flex items-center justify-between">
                   <div className="flex flex-col gap-1">
                      <span className="text-[10px] text-muted-foreground uppercase tracking-widest font-semibold">{t('common.date')}</span>
                      <span className="text-xs font-medium">{new Date(project.created_at).toLocaleDateString()}</span>
                   </div>

                  <div className="flex gap-2">
                      <RestartButton
                        projectId={project.uid}
                        status={project.status}
                        variant="outline"
                        size="icon"
                        className="h-9 w-9 text-muted-foreground hover:text-foreground"
                        onStarted={() => onActionStarted(project.uid, 'restarting')}
                        onSuccess={fetchProjects}
                      />
                      <RedeployButton
                        projectId={project.uid}
                        status={project.status}
                        variant="outline"
                        size="icon"
                        showOptions={false}
                        className="h-9 w-9 text-muted-foreground hover:text-foreground"
                        onStarted={() => onActionStarted(project.uid, 'queued')}
                        onSuccess={fetchProjects}
                      />
                      <Button
                        variant="outline"
                        size="icon"
                        onClick={(e) => handleDelete(project.uid, e)}
                        className="h-9 w-9 text-muted-foreground hover:text-destructive hover:bg-destructive/10 hover:border-destructive/30 cursor-pointer"
                        style={{ cursor: 'pointer' }}
                        title={t('projectDetail.actions.delete')}
                      >
                         <Trash2 className="w-4 h-4 cursor-pointer" style={{ cursor: 'pointer' }} />
                      </Button>
                      <Button size="icon" className="h-9 w-9">
                         <ArrowRight className="w-4 h-4" />
                      </Button>
                   </div>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}

export default UserProjects
