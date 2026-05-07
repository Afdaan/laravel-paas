import { useState, useEffect, useMemo, useRef, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import useTranslation from '../../lib/useTranslation'
import {
  RefreshCw,
  ExternalLink,
  Trash2,
  Cpu,
  Activity,
  Play,
  Power,
  Layout,
  Terminal as TerminalIcon,
  Code,
  Globe,
  Database as DatabaseIcon,
  ShieldAlert,
  Box,
  AlertTriangle,
  GitBranch,
  Eye,
  EyeOff,
  Loader2,
  Save,
  Copy,
  Blocks,
  Code2,
  CheckCircle2
} from 'lucide-react'
import { AxiosError } from 'axios'
import { projectsAPI, databaseAPI } from '../../services/api'
import { Project, ProjectStats } from '../../types'
import ConfirmationModal from '../../components/ConfirmationModal'
import DatabaseManager from './DatabaseManager'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { usePolling } from '@/lib/usePolling'
import { cn } from '@/lib/utils'
import { Switch } from '@/components/ui/switch'
import { FrameworkIcon } from '../../components/FrameworkIcon'
import BuildLogsConsole from '@/components/BuildLogsConsole'

// Status Indicator Component
function StatusIndicator({ status }: { status: string }) {
  const { t } = useTranslation()
  const styles: Record<string, { color: string, label: string, pulse?: boolean }> = {
    running: { color: 'text-emerald-500 bg-emerald-500/10 border-emerald-500/20', label: t('status.running') },
    building: { color: 'text-blue-500 bg-blue-500/10 border-blue-500/20', label: t('status.building'), pulse: true },
    queued: { color: 'text-purple-500 bg-purple-500/10 border-purple-500/20', label: t('status.queued') || 'Queued' },
    failed: { color: 'text-rose-500 bg-rose-500/10 border-rose-500/20', label: t('status.failed') },
    pending: { color: 'text-amber-500 bg-amber-500/10 border-amber-500/20', label: t('status.pending') },
    stopped: { color: 'text-slate-500 bg-slate-500/10 border-slate-500/20 dark:text-slate-400', label: t('status.stopped') },
  }

  const current = styles[status] || styles.pending

  return (
    <Badge variant="outline" className={cn("gap-2 py-1 px-3", current.color)}>
      <div className={cn("w-2 h-2 rounded-full bg-current", current.pulse && "animate-pulse")} />
      <span className="text-[10px] uppercase font-bold tracking-wider">{current.label}</span>
    </Badge>
  )
}

function MetricCard({ title, value, subtext, icon: Icon, renderIcon, colorClass }: { title: string, value: string, subtext?: string, icon?: React.ElementType, renderIcon?: (className: string) => React.ReactNode, colorClass?: string }) {
  return (
    <Card className="bg-card/50 border-border/40 backdrop-blur-md overflow-hidden group hover:border-primary/40 hover:shadow-[0_0_20px_rgba(var(--primary),0.05)] transition-all duration-500 relative">
      <div className={cn("absolute top-0 left-0 w-1 h-full opacity-0 group-hover:opacity-100 transition-opacity duration-500", colorClass?.replace('text-', 'bg-'))} />
      <CardContent className="p-5">
        <div className="flex items-center justify-between mb-4">
          <span className="text-[10px] font-bold uppercase tracking-[0.2em] text-muted-foreground/70 group-hover:text-muted-foreground transition-colors">{title}</span>
          <div className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-border/50 bg-muted/30 shadow-inner group-hover:scale-110 transition-transform duration-500">
            {renderIcon ? renderIcon("w-5 h-5") : Icon && <Icon className={cn("w-4 h-4 transition-colors", colorClass || "text-muted-foreground")} />}
          </div>
        </div>
        <div className="space-y-1">
          <div className="text-2xl font-bold tracking-tight text-foreground/90 group-hover:text-foreground transition-colors">{value}</div>
          {subtext && <div className="text-[9px] text-muted-foreground/60 font-bold uppercase tracking-widest">{subtext}</div>}
        </div>
      </CardContent>
    </Card>
  )
}

function StudentProjectDetail() {
  const { t } = useTranslation()
  const { uid } = useParams<{ uid: string }>()
  const navigate = useNavigate()
  const [project, setProject] = useState<Project | null>(null)
  const [logs, setLogs] = useState('')
  const [stats, setStats] = useState<ProjectStats | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [activeTab, setActiveTab] = useState('project')
  const [logType, setLogType] = useState<'web' | 'worker'>('web')
  const logsEndRef = useRef<HTMLDivElement>(null)

  const [envContent, setEnvContent] = useState('')
  const [consoleOutput, setConsoleOutput] = useState('')
  const [consoleCommand, setConsoleCommand] = useState('')
  const [isExecuting, setIsExecuting] = useState(false)
  const [isEnvHidden, setIsEnvHidden] = useState(true)
  const [isSavingEnv, setIsSavingEnv] = useState(false)
  const [credentials, setCredentials] = useState<Record<string, string> | null>(null)
  const [branchInput, setBranchInput] = useState('')
  const [baseDirInput, setBaseDirInput] = useState('')
  const [buildCommandInput, setBuildCommandInput] = useState('')
  const [startCommandInput, setStartCommandInput] = useState('')
  const [nodeVersionInput, setNodeVersionInput] = useState('')
  const [phpVersionInput, setPhpVersionInput] = useState('')
  const [runtimeImageInput, setRuntimeImageInput] = useState('')
  const [workerCommandInput, setWorkerCommandInput] = useState('')
  const [queueEnabledInput, setQueueEnabledInput] = useState(false)
  const [languageVersionInput, setLanguageVersionInput] = useState('')
  const [isSavingSettings, setIsSavingSettings] = useState(false)

  const isNodeRelated = ['Node.js', 'Next.js', 'Vite', 'React', 'Vue', 'Nuxt.js', 'Svelte', 'Angular', 'TypeScript'].includes(project?.framework || '')

  const [confirmModal, setConfirmModal] = useState({
    isOpen: false,
    title: '',
    message: '' as React.ReactNode,
    type: 'danger' as 'danger' | 'warning' | 'info',
    onConfirm: () => { },
    confirmText: t('common.confirm')
  })

  const [consecutiveErrors, setConsecutiveErrors] = useState(0)
  const frameworkDetail = useMemo(() => {
    if (!project) return t('projectDetail.metrics.managedStack')
    if (project.framework === 'Laravel') {
      return project.php_version ? `PHP ${project.php_version.replace('.dynamic', '')}` : 'PHP Stack'
    }
    const isNode = ['Node.js', 'Next.js', 'Vite', 'React', 'Vue', 'Nuxt.js', 'Svelte', 'Angular', 'TypeScript'].includes(project.framework || '')
    if (isNode) {
      return project.node_version ? `Node.js ${project.node_version}` : 'Node.js Stack'
    }
    if (project.language_version) {
      return `${project.framework} ${project.language_version}`
    }
    return t('projectDetail.metrics.managedStack')
  }, [project, t])

  const deployLocked = project?.status === 'queued' || project?.status === 'pending' || project?.status === 'building'

  const fetchProject = useCallback(async () => {
    if (!uid) return
    try {
      const response = await projectsAPI.get(uid)
      setProject(response.data)
      setBranchInput(response.data.branch || '')
      setBaseDirInput(response.data.base_directory || '')
      setConsecutiveErrors(0)
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ error: string }>
      if (axiosError.response?.status === 401) {
        navigate('/login')
        return
      }

      if (axiosError.response?.status === 404) {
        toast.error(t('projectDetail.messages.notFound') || 'Project not found')
        // Permission checks
        navigate('/projects')
        return
      }

      setConsecutiveErrors(prev => prev + 1)

      toast.error(t('common.error'), {
        id: 'project-load-error',
        description: consecutiveErrors >= 2 ? t('common.pollingPaused') : undefined
      })
    } finally {
      setIsLoading(false)
    }
  }, [uid, navigate, t, consecutiveErrors])

  const fetchLogs = useCallback(async () => {
    if (!uid) return
    try {
      const response = await projectsAPI.logs(uid, 200, logType)
      setLogs(response.data.logs)
      if (logsEndRef.current) {
        logsEndRef.current.scrollIntoView({ behavior: 'auto' })
      }
    } catch (error) {
      // Quietly fail for logs polling
    }
  }, [uid, logType])

  const fetchStats = useCallback(async () => {
    if (!uid) return
    try {
      const response = await projectsAPI.stats(uid)
      setStats(response.data)
    } catch (error) {
      // Stats may not be available for non-running containers
      setStats(null)
    }
  }, [uid])

  const fetchEnv = useCallback(async () => {
    if (!uid) return
    try {
      const response = await projectsAPI.getEnv(uid)
      setEnvContent(response.data.content)
    } catch (error) {
      toast.error(t('common.error'))
    }
  }, [uid, t])

  const fetchCredentials = useCallback(async () => {
    if (!uid) return
    try {
      const response = await databaseAPI.getCredentials(uid)
      setCredentials(response.data)
    } catch (error) {
      // Credentials only available if DB is ready
    }
  }, [uid])

  usePolling(() => {
    if (consecutiveErrors < 3) {
      fetchProject()
      if (project?.status === 'running') {
        fetchStats()
      }
    }
  }, 5000)

  usePolling(() => {
    if (activeTab === 'logs' && project?.container_id && consecutiveErrors < 3) {
      fetchLogs()
    }
  }, activeTab === 'logs' ? 5000 : null)

  useEffect(() => {
    if (activeTab === 'logs' && project?.container_id) {
      setLogs('')
      fetchLogs()
    }
  }, [logType, activeTab, project?.container_id, fetchLogs])

  useEffect(() => {
    if (activeTab === 'environment') {
      fetchEnv()
    }
    if (activeTab === 'database') {
      fetchCredentials()
    }
  }, [activeTab, uid, fetchEnv, fetchCredentials])

  const logLines = useMemo(() => {
    if (!logs) return []
    const lines = logs.split('\n')
    return lines.length > 500 ? lines.slice(-500) : lines
  }, [logs])

  const logOffset = useMemo(() => {
    if (!logs) return 0
    const lines = logs.split('\n')
    return lines.length > 500 ? lines.length - 500 : 0
  }, [logs])

  const consoleLines = useMemo(() => {
    if (!consoleOutput) return []
    const lines = consoleOutput.split('\n')
    return lines.length > 500 ? lines.slice(-500) : lines
  }, [consoleOutput])

  const consoleOffset = useMemo(() => {
    if (!consoleOutput) return 0
    const lines = consoleOutput.split('\n')
    return lines.length > 500 ? lines.length - 500 : 0
  }, [consoleOutput])


  const handleConsoleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!uid || !consoleCommand.trim()) return

    setIsExecuting(true)
    setConsoleOutput(prev => prev + `\n$ php artisan ${consoleCommand}\n`)

    try {
      const response = await projectsAPI.runArtisan(uid, consoleCommand)
      setConsoleOutput(prev => prev + response.data.output + '\n')
      setConsoleCommand('')
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ output: string }>
      const errOut = axiosError.response?.data?.output || axiosError.message
      setConsoleOutput(prev => prev + `Error: ${errOut}\n`)
    } finally {
      setIsExecuting(false)
    }
  }

  const handleRedeploy = () => {
    if (!uid) return
    if (deployLocked) {
      toast.message(t('projectDetail.messages.buildTitle'), {
        description: `${t('projectDetail.actions.redeploy')} (${t(`status.${project?.status}`)})`,
      })
      return
    }
    setConfirmModal({
      title: t('projectDetail.messages.redeployConfirm'),
      message: t('projectDetail.messages.redeployDesc'),
      type: 'warning',
      confirmText: t('projectDetail.actions.redeploy'),
      isOpen: true,
      onConfirm: () => {
        setProject(prev => prev ? ({ ...prev, status: 'queued' }) : null)
        toast.promise(
          projectsAPI.redeploy(uid),
          {
            loading: t('common.loading'),
            success: t('projectDetail.actions.redeployStarted'),
            error: t('common.error'),
          }
        )
      }
    })
  }

  const handleStop = async () => {
    if (!uid) return
    try {
      await toast.promise(
        projectsAPI.stop(uid),
        {
          loading: t('common.loading'),
          success: t('projectDetail.actions.stop'),
          error: t('common.error'),
        },
      )
      fetchProject()
    } catch (error: unknown) {
      const axiosError = error as AxiosError
      if (axiosError?.response?.status === 404) {
        toast.error(t('projectDetail.messages.stopUnavailable'))
      }
    }
  }

  const handleStart = async () => {
    if (!uid) return
    try {
      await toast.promise(
        projectsAPI.start(uid),
        {
          loading: t('common.loading'),
          success: t('projectDetail.actions.start'),
          error: t('common.error'),
        },
      )
      fetchProject()
    } catch (error: unknown) {
      const axiosError = error as AxiosError
      if (axiosError?.response?.status === 404) {
        toast.error(t('projectDetail.messages.startUnavailable'))
      }
    }
  }

  const handleSaveEnv = async () => {
    if (!uid) return
    setIsSavingEnv(true)
    try {
      await projectsAPI.updateEnv(uid, envContent)
      toast.success(t('common.success'))
      projectsAPI.redeploy(uid).then(() => fetchProject())
    } catch (error) {
      toast.error(t('common.error'))
    } finally {
      setIsSavingEnv(false)
    }
  }

  const isSettingsDirty = useMemo(() => {
    if (!project) return false
    return branchInput !== (project.branch || '') ||
      baseDirInput !== (project.base_directory || '') ||
      buildCommandInput !== (project.build_command || '') ||
      startCommandInput !== (project.start_command || '') ||
      nodeVersionInput !== (project.node_version || '20') ||
      phpVersionInput !== (project.php_version || '8.2') ||
      runtimeImageInput !== (project.runtime_image || 'alpine') ||
      workerCommandInput !== (project.worker_command || '') ||
      queueEnabledInput !== (project.queue_enabled || false) ||
      languageVersionInput !== (project.language_version || '')
  }, [project, branchInput, baseDirInput, buildCommandInput, startCommandInput, nodeVersionInput, phpVersionInput, runtimeImageInput, workerCommandInput, queueEnabledInput, languageVersionInput])

  const handleResetSettings = () => {
    if (!project) return
    setBranchInput(project.branch || '')
    setBaseDirInput(project.base_directory || '')
    setBuildCommandInput(project.build_command || '')
    setStartCommandInput(project.start_command || '')
    setNodeVersionInput(project.node_version || '20')
    setPhpVersionInput(project.php_version || '8.2')
    setRuntimeImageInput(project.runtime_image || 'alpine')
    setWorkerCommandInput(project.worker_command || '')
    setQueueEnabledInput(project.queue_enabled || false)
    setLanguageVersionInput(project.language_version || '')
    toast.info(t('common.resetSuccess') || 'Settings reset to original values')
  }

  const handleSaveSettings = async () => {
    if (!uid || !project) return

    setConfirmModal({
      title: t('common.confirm'),
      message: t('projectDetail.settings.redeployWarning'),
      type: 'warning',
      confirmText: t('common.save'),
      isOpen: true,
      onConfirm: async () => {
        setIsSavingSettings(true)
        try {
          const payload = {
            branch: branchInput,
            base_directory: baseDirInput,
            build_command: buildCommandInput,
            start_command: startCommandInput,
            node_version: nodeVersionInput,
            php_version: phpVersionInput,
            runtime_image: runtimeImageInput,
            worker_command: workerCommandInput,
            queue_enabled: queueEnabledInput,
            language_version: languageVersionInput
          }
          await projectsAPI.update(uid, payload)
          toast.success(t('common.success'))
          await projectsAPI.redeploy(uid)
          fetchProject()
        } catch (error: unknown) {
          toast.error(t('common.error'))
        } finally {
          setIsSavingSettings(false)
        }
      }
    })
  }

  useEffect(() => {
    if (project) {
      setBranchInput(project.branch || '')
      setBaseDirInput(project.base_directory || '')
      setBuildCommandInput(project.build_command || '')
      setStartCommandInput(project.start_command || '')
      setNodeVersionInput(project.node_version || '20')
      setPhpVersionInput(project.php_version || '8.2')
      setRuntimeImageInput(project.runtime_image || 'alpine')
      setWorkerCommandInput(project.worker_command || '')
      setQueueEnabledInput(project.queue_enabled || false)
      setLanguageVersionInput(project.language_version || '')
    }
  }, [project])

  const handleDelete = () => {
    if (!uid) return
    setConfirmModal({
      title: t('projectDetail.messages.deleteConfirm'),
      message: t('projectDetail.messages.deleteDesc'),
      type: 'danger',
      confirmText: t('projectDetail.actions.delete'),
      isOpen: true,
      onConfirm: () => {
        toast.promise(
          projectsAPI.delete(uid),
          {
            loading: t('common.loading'),
            success: t('common.success'),
            error: t('common.error'),
          }
        )
        navigate('/projects')
      }
    })
  }

  if (isLoading) {
    return (
      <div className="flex flex-col items-center justify-center h-screen gap-4">
        <Loader2 className="w-10 h-10 text-primary animate-spin" />
        <div className="text-muted-foreground animate-pulse font-medium">{t('common.loading')}</div>
      </div>
    )
  }

  if (!project) return null
  const projectUrl = project.url || `https://${project.subdomain}.${window.location.hostname}`
  const isLaravelProject = project.framework === 'Laravel'
  const isStopped = project.status === 'stopped'
  const frameworkLabel = project.framework && project.framework !== 'Other' ? project.framework : t('common.general')

  return (
    <div className="space-y-6 max-w-7xl mx-auto pb-20 animate-in fade-in duration-500">
      <ConfirmationModal
        onClose={() => setConfirmModal(prev => ({ ...prev, isOpen: false }))}
        {...confirmModal}
      />

      {/* Building Banner */}
      {(project.status === 'building' || project.status === 'failed') && (
        <Card className={cn(
          "border-blue-500/20 bg-blue-500/5 p-6 mb-6",
          project.status === 'building' && "border-blue-500/30 bg-blue-500/10",
          project.status === 'failed' && "border-rose-500/20 bg-rose-500/5"
        )}>
          <div className="flex flex-col sm:flex-row items-center gap-6 text-center sm:text-left">
            <div className={cn(
              "w-12 h-12 rounded-xl flex items-center justify-center shrink-0",
              project.status === 'building' ? "bg-blue-500/20 text-blue-500" : "bg-rose-500/20 text-rose-500"
            )}>
              {project.status === 'building' ? <Box className="w-6 h-6" /> : <AlertTriangle className="w-6 h-6" />}
            </div>
            <div className="flex-1">
              <h3 className={cn(
                "text-lg font-bold",
                project.status === 'failed' && "text-rose-500"
              )}>
                {project.status === 'building' ? t('projectDetail.messages.buildTitle') : t('projectDetail.overview.deployError')}
              </h3>
              <p className="text-sm text-muted-foreground">
                {project.status === 'building'
                  ? t('projectDetail.messages.buildDesc')
                  : t('projectDetail.messages.failedDesc')}
              </p>
            </div>
            <div className="flex gap-3">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setActiveTab('build')}
                className={cn(
                  "gap-2 font-bold uppercase tracking-wider text-[10px]",
                  project.status === 'building' ? "border-blue-500/30 hover:bg-blue-500/10" : "border-rose-500/30 hover:bg-rose-500/10"
                )}
              >
                <TerminalIcon className="w-3.5 h-3.5" />
                {t('projectDetail.tabs.build')}
              </Button>
            </div>
          </div>
        </Card>
      )}

      {/* Header */}
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-6 pb-6 border-b">
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-3">
            <h1 className="text-3xl font-bold tracking-tight">{project.name}</h1>
            <StatusIndicator status={project.status} />
          </div>
          <div className="flex flex-wrap items-center gap-4">
            <div className="px-3 py-1.5 rounded-lg bg-muted border flex items-center gap-2">
              <Globe className="w-4 h-4 text-muted-foreground" />
              <span className="text-muted-foreground font-mono text-xs">{project.subdomain}</span>
              {project.status === 'running' && (
                <a
                  href={projectUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="ml-1 inline-flex h-6 w-6 items-center justify-center rounded-md text-primary transition-colors hover:bg-muted"
                >
                  <ExternalLink className="w-3 h-3" />
                </a>
              )}
            </div>
            <Badge variant="outline" className="gap-1.5 bg-muted/50 border-border/50">
              <FrameworkIcon framework={project.framework} variant="plain" className="w-3.5 h-3.5" />
              {frameworkLabel}
            </Badge>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          {isStopped ? (
            <Button
              variant="outline"
              onClick={handleStart}
              className="gap-2 border-emerald-500/30 hover:bg-emerald-500/10 text-emerald-500"
            >
              <Play className="w-4 h-4" />
              {t('projectDetail.actions.start')}
            </Button>
          ) : (
            <Button
              variant="outline"
              onClick={handleStop}
              disabled={deployLocked}
              className={cn("gap-2", deployLocked && "opacity-40")}
            >
              <Power className="w-4 h-4" />
              {t('projectDetail.actions.stop')}
            </Button>
          )}

          <Button
            variant="outline"
            onClick={handleRedeploy}
            disabled={deployLocked}
            className={cn("gap-2", deployLocked && "opacity-40")}
            title={
              deployLocked
                ? `${t('projectDetail.actions.redeploy')} (${t(`status.${project.status}`)})`
                : t('projectDetail.actions.redeploy')
            }
          >
            <RefreshCw className={cn("w-4 h-4", project.status === 'building' && "animate-spin")} />
            {t('projectDetail.actions.redeploy')}
          </Button>
          <Button variant="outline" size="icon" onClick={handleDelete} className="text-destructive hover:bg-destructive/10 hover:border-destructive/30">
            <Trash2 className="w-4 h-4" />
          </Button>
        </div>
      </div>

      {/* Stats Row */}
      <div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
        <MetricCard
          title={t('projectDetail.metrics.cpu')}
          value={stats ? `${stats.cpu_percent.toFixed(1)}%` : '0%'}
          subtext={t('projectDetail.metrics.load')}
          colorClass="text-blue-500"
          icon={Cpu}
        />
        <MetricCard
          title={t('projectDetail.metrics.memory')}
          value={stats ? `${stats.memory_mb.toFixed(0)} MB` : '0 MB'}
          subtext={t('projectDetail.metrics.unitOf', { total: stats?.memory_max_mb?.toFixed(0) || 512, unit: 'MB' })}
          colorClass="text-emerald-500"
          icon={Activity}
        />
        <MetricCard
          title={t('projectDetail.metrics.framework')}
          value={frameworkLabel}
          subtext={frameworkDetail}
          renderIcon={(className) => <FrameworkIcon framework={project.framework} variant="plain" className={className} />}
        />
        <MetricCard
          title={t('projectDetail.metrics.db')}
          value="MySQL"
          subtext={project.database_name || t('projectDetail.metrics.noDb')}
          icon={DatabaseIcon}
        />
        <MetricCard
          title={isLaravelProject ? t('projectDetail.metrics.queue') : t('projectDetail.metrics.backgroundService')}
          value={
            isLaravelProject
              ? (project.queue_enabled ? t('projectDetail.metrics.active') : t('projectDetail.metrics.inactive'))
              : (project.worker_command ? t('projectDetail.metrics.active') : t('projectDetail.metrics.inactive'))
          }
          subtext={
            isLaravelProject
              ? (project.queue_enabled ? t('projectDetail.metrics.background') : t('projectDetail.metrics.direct'))
              : (project.worker_command ? t('projectDetail.metrics.customCommand') : t('projectDetail.metrics.notConfigured'))
          }
          colorClass={
            isLaravelProject
              ? (project.queue_enabled ? 'text-emerald-500' : '')
              : (project.worker_command ? 'text-emerald-500' : '')
          }
          icon={RefreshCw}
        />
      </div>

      {/* Tabs */}
      <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
        <TabsList className="bg-muted p-1 rounded-lg w-fit overflow-x-auto">
          <TabsTrigger value="project">{t('projectDetail.tabs.overview')}</TabsTrigger>
          <TabsTrigger value="console">{t('projectDetail.tabs.console')}</TabsTrigger>
          <TabsTrigger value="environment">{t('projectDetail.tabs.secrets')}</TabsTrigger>
          <TabsTrigger value="database">{t('projectDetail.tabs.database')}</TabsTrigger>
          <TabsTrigger value="logs">{t('projectDetail.tabs.logs')}</TabsTrigger>
          <TabsTrigger value="build">{t('projectDetail.tabs.build')}</TabsTrigger>
          <TabsTrigger value="settings">{t('projectDetail.tabs.settings')}</TabsTrigger>
        </TabsList>

        <TabsContent value="project" className="pt-0">
          <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
            <Card className="lg:col-span-2">
              <CardHeader className="pb-4">
                <CardTitle className="text-sm font-bold uppercase tracking-widest flex items-center gap-2">
                  <Layout className="w-4 h-4 text-primary" />
                  {t('projectDetail.overview.connectionInfo')}
                </CardTitle>
              </CardHeader>
              <CardContent>
                <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between p-4 bg-muted/50 rounded-xl border gap-4">
                  <div className="flex items-center gap-4">
                    <div className="p-2.5 bg-emerald-500/10 rounded-lg text-emerald-600 border border-emerald-500/20">
                      <Globe className="w-5 h-5" />
                    </div>
                    <div>
                      <div className="font-bold text-sm">{t('projectDetail.overview.productionUrl')}</div>
                      <div className="text-[10px] text-muted-foreground font-medium uppercase tracking-wider">{t('projectDetail.overview.webAccess')}</div>
                    </div>
                  </div>
                  <a href={projectUrl} target="_blank" className="text-primary hover:underline text-sm font-mono truncate max-w-xs">{projectUrl}</a>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader className="pb-4">
                <CardTitle className="text-sm font-bold uppercase tracking-widest flex items-center gap-2">
                  <Code className="w-4 h-4 text-primary" />
                  {t('projectDetail.overview.repository')}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="p-3 rounded-lg bg-muted border">
                  <label className="text-[9px] font-bold text-muted-foreground uppercase tracking-widest mb-1 block">{t('projectDetail.overview.uri')}</label>
                  <div className="text-xs font-mono truncate">{project.github_url || project.repository_url}</div>
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div className="p-3 rounded-lg bg-muted border">
                    <label className="text-[9px] font-bold text-muted-foreground uppercase tracking-widest mb-1 block">{t('projectDetail.overview.branch')}</label>
                    <div className="flex items-center gap-1.5 font-bold text-xs">
                      <GitBranch className="w-3 h-3 text-primary" />
                      {project.branch || 'main'}
                    </div>
                  </div>
                  <div className="p-3 rounded-lg bg-muted border">
                    <label className="text-[9px] font-bold text-muted-foreground uppercase tracking-widest mb-1 block">{t('projectDetail.settings.version')}</label>
                    <div className="flex items-center gap-1.5 font-bold text-xs uppercase">
                      <FrameworkIcon framework={project.framework} variant="plain" className="w-3.5 h-3.5" />
                      {isLaravelProject 
                        ? (project.laravel_version || 'Laravel 10')
                        : (project.framework && project.framework !== 'Other' ? `${project.framework} ${project.language_version || ''}` : t('common.general'))}
                    </div>
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>

          {project.error_log && (
            <Card className="border-destructive/20 bg-destructive/5 overflow-hidden">
              <CardHeader className="bg-destructive/10 py-3">
                <CardTitle className="text-xs font-bold text-destructive flex items-center gap-2 uppercase tracking-widest">
                  <ShieldAlert className="w-4 h-4" />
                  {t('projectDetail.overview.deployError')}
                </CardTitle>
              </CardHeader>
              <CardContent className="pt-4">
                <pre className="text-[11px] text-destructive/90 overflow-auto max-h-48 whitespace-pre-wrap font-mono leading-relaxed bg-black/5 p-4 rounded-lg">{project.error_log}</pre>
              </CardContent>
            </Card>
          )}
        </TabsContent>

        <TabsContent value="console" className="pt-0">
          <Card className="bg-zinc-950 text-white border-none overflow-hidden flex flex-col h-[600px] gap-0 py-0 shadow-2xl">
            <CardHeader className="bg-zinc-900/50 px-4 py-3 border-b border-white/5 flex flex-row items-center justify-between backdrop-blur-md">
              <div className="flex items-center gap-3">
                <div className="flex gap-1.5 mr-2">
                  <div className="w-2.5 h-2.5 rounded-full bg-rose-500/80 shadow-[0_0_8px_rgba(244,63,94,0.4)]" />
                  <div className="w-2.5 h-2.5 rounded-full bg-amber-500/80 shadow-[0_0_8px_rgba(245,158,11,0.4)]" />
                  <div className="w-2.5 h-2.5 rounded-full bg-emerald-500/80 shadow-[0_0_8px_rgba(16,185,129,0.4)]" />
                </div>
                <div className="text-[10px] uppercase font-bold tracking-widest text-zinc-400 flex items-center gap-2">
                  <TerminalIcon className="w-3.5 h-3.5" />
                  {project.framework === 'Laravel' ? 'Artisan Console' : 'Terminal Console'}
                </div>
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => navigator.clipboard.writeText(consoleOutput)}
                  className="p-1.5 hover:bg-white/10 rounded-md transition-colors text-zinc-500 hover:text-white"
                  title="Copy Logs"
                >
                  <Copy className="w-3.5 h-3.5" />
                </button>
                <button
                  onClick={() => {
                    const el = document.getElementById('console-scroll-area');
                    if (el) el.scrollTop = el.scrollHeight;
                  }}
                  className="p-1.5 hover:bg-white/10 rounded-md transition-colors text-zinc-500 hover:text-white"
                  title="Scroll to Bottom"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="m7 15 5 5 5-5" /><path d="m7 9 5 5 5-5" /></svg>
                </button>
                <div className="w-px h-3 bg-white/10 mx-1" />
                <Button variant="ghost" size="xs" onClick={() => setConsoleOutput('')} className="text-[10px] uppercase font-bold text-zinc-600 hover:text-rose-400">{t('projectDetail.actions.clear')}</Button>
              </div>
            </CardHeader>

            <div id="console-scroll-area" className="flex-1 p-6 overflow-auto font-mono text-[11px] text-zinc-300 custom-scrollbar bg-zinc-950/50">
              <div className="text-amber-400/80 mb-6 flex flex-col gap-2 border-b border-white/5 pb-4">
                <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-wider">
                  <AlertTriangle size={14} />
                  <span>{project.framework === 'Laravel' ? 'Prefix: php artisan' : 'Security Advisory'}</span>
                </div>
                <p className="text-[10px] text-zinc-500 leading-relaxed max-w-2xl italic">
                  {project.framework === 'Laravel'
                    ? 'All commands are automatically prefixed with \'php artisan\'. Only specific artisan commands are allowed for security.'
                    : 'Use standard CLI commands. Commands are executed in the project root. Dangerous operations are restricted.'}
                </p>
              </div>

              {consoleLines.length > 0 ? consoleLines.map((line: string, i: number) => (
                <div key={i} className={cn("flex gap-4 group py-0.5 px-2 rounded -mx-2 hover:bg-white/[0.02]", line.startsWith('$') ? "text-primary mt-2 font-bold" : "text-zinc-400")}>
                  <span className="shrink-0 text-zinc-800 select-none w-6 text-right font-light">{consoleOffset + i + 1}</span>
                  <span className="break-all whitespace-pre-wrap">{line}</span>
                </div>
              )) : (
                <div className="h-full flex flex-col items-center justify-center opacity-10 gap-4">
                  <TerminalIcon size={48} />
                  <p className="uppercase tracking-[0.4em] font-bold text-xs">{t('projectDetail.console.terminalReady')}</p>
                </div>
              )}
              {isExecuting && <div className="mt-4 flex items-center gap-2 text-primary animate-pulse font-bold text-[10px] uppercase tracking-widest bg-primary/5 p-2 rounded border border-primary/20 w-fit"><RefreshCw className="w-3 h-3 animate-spin" /> {t('common.executing')}</div>}
              <div ref={logsEndRef} />
            </div>

            <form onSubmit={handleConsoleSubmit} className="p-4 bg-zinc-900/80 border-t border-white/5 flex gap-3">
              {project.framework === 'Laravel' && (
                <div className="flex items-center px-4 bg-zinc-800 rounded font-mono text-[10px] font-bold text-zinc-500 border border-white/5">php artisan</div>
              )}
              <Input
                value={consoleCommand}
                onChange={e => setConsoleCommand(e.target.value)}
                placeholder={project.framework === 'Laravel' ? 'migrate --seed' : 'npm run build'}
                disabled={isExecuting}
                className="flex-1 bg-zinc-800/50 border-white/10 text-white font-mono text-xs focus-visible:ring-1 focus-visible:ring-primary h-10 shadow-inner"
              />
              <Button type="submit" disabled={isExecuting || !consoleCommand.trim()} size="sm" className="h-10 px-6 font-bold uppercase tracking-widest text-[10px]">{t('projectDetail.actions.execute')}</Button>
            </form>
          </Card>
        </TabsContent>

        <TabsContent value="environment" className="pt-0">
          <Card className="flex flex-col h-[600px] overflow-hidden border-border/50 shadow-sm">
            <CardHeader className="pb-4 flex flex-row items-center justify-between border-b border-border">
              <div>
                <CardTitle className="text-lg flex items-center gap-2">
                  <ShieldAlert className="w-5 h-5 text-primary" />
                  {t('projectDetail.tabs.secrets')}
                </CardTitle>
                <CardDescription className="text-xs">{t('projectDetail.secrets.desc')}</CardDescription>
              </div>
              <div className="flex items-center gap-3">
                <Button variant="outline" size="sm" onClick={() => setIsEnvHidden(!isEnvHidden)} className="h-9">
                  {isEnvHidden ? <Eye className="w-3.5 h-3.5 mr-2" /> : <EyeOff className="w-3.5 h-3.5 mr-2" />}
                  <span className="text-[10px] font-bold uppercase tracking-wider">{isEnvHidden ? t('projectDetail.actions.reveal') : t('projectDetail.actions.hide')}</span>
                </Button>
                <Button
                  size="sm"
                  onClick={() => {
                    setConfirmModal({
                      title: 'Simpan perubahan .env?',
                      message: 'Container akan dijalankan ulang (redeploy) untuk menerapkan konfigurasi baru.',
                      type: 'warning',
                      confirmText: 'Simpan & Redeploy',
                      isOpen: true,
                      onConfirm: handleSaveEnv
                    })
                  }}
                  disabled={isSavingEnv || isEnvHidden}
                  className="h-9 px-6 bg-primary hover:bg-primary/90"
                >
                  {isSavingEnv ? <Loader2 className="w-3.5 h-3.5 mr-2 animate-spin" /> : <Save className="w-3.5 h-3.5 mr-2" />}
                  <span className="text-[10px] font-bold uppercase tracking-wider">{t('common.save')}</span>
                </Button>
              </div>
            </CardHeader>
            <div className="flex-1 relative bg-muted/30">
              <Textarea
                value={envContent}
                onChange={e => setEnvContent(e.target.value)}
                readOnly={isEnvHidden}
                spellCheck={false}
                className={cn(
                  "absolute inset-0 h-full w-full rounded-none border-none p-10 font-mono text-[11px] leading-relaxed resize-none bg-transparent focus-visible:ring-0 custom-scrollbar",
                  isEnvHidden && "blur-md select-none opacity-30"
                )}
                placeholder={t('projectDetail.secrets.placeholder')}
              />
              {isEnvHidden && (
                <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
                  <div className="px-6 py-3 bg-card/80 border border-border backdrop-blur-md rounded-full shadow-lg flex items-center gap-3">
                    <ShieldAlert className="w-4 h-4 text-primary" />
                    <span className="text-[10px] font-bold tracking-[0.2em] uppercase text-muted-foreground">{t('projectDetail.secrets.locked')}</span>
                  </div>
                </div>
              )}
            </div>
            <div className="p-4 bg-amber-500/5 text-amber-600 dark:text-amber-500/80 text-[9px] font-bold uppercase tracking-[0.15em] border-t border-border/50 flex items-center justify-center gap-3">
              <AlertTriangle size={14} className="animate-pulse" /> {t('projectDetail.secrets.redeployNote')}
            </div>
          </Card>
        </TabsContent>

        <TabsContent value="database" className="pt-0">
          <div className="space-y-6">
            <Card>
              <CardHeader>
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-4">
                    <div className="w-10 h-10 bg-primary/10 rounded-lg flex items-center justify-center text-primary">
                      <DatabaseIcon className="w-5 h-5" />
                    </div>
                    <div>
                      <CardTitle className="text-lg">{t('projectDetail.database.creds')}</CardTitle>
                      <CardDescription>{t('projectDetail.database.params')}</CardDescription>
                    </div>
                  </div>
                  <Badge variant="outline" className="text-emerald-500 bg-emerald-500/5 border-emerald-500/20 gap-1.5 uppercase tracking-wider text-[10px]">
                    <ShieldAlert className="w-3 h-3" /> {t('projectDetail.database.privateAccess')}
                  </Badge>
                </div>
              </CardHeader>
              <CardContent>
                <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                  <CredentialRow label="Host" value={credentials?.host || "paas-mysql.cluster.local"} />
                  <CredentialRow label="Schema" value={credentials?.database || project.database_name || '...'} />
                  <CredentialRow label="Username" value={credentials?.username || project.database_name || '...'} />
                  <CredentialRow label="Password" value={credentials?.password || project.database_name || '...'} isSecret />
                </div>
              </CardContent>
            </Card>
            <DatabaseManager embedded={true} projectId={uid} />
          </div>
        </TabsContent>

        <TabsContent value="logs" className="pt-0">
          <Card className="bg-zinc-950 text-zinc-300 border-none overflow-hidden flex flex-col h-[600px] gap-0 py-0 shadow-2xl">
            <CardHeader className="bg-zinc-900/50 px-4 py-3 border-b border-white/5 flex flex-row items-center justify-between backdrop-blur-md">
              <div className="flex items-center gap-3">
                <div className="flex gap-1.5 mr-2">
                  <div className="w-2.5 h-2.5 rounded-full bg-rose-500/80 shadow-[0_0_8px_rgba(244,63,94,0.4)]" />
                  <div className="w-2.5 h-2.5 rounded-full bg-amber-500/80 shadow-[0_0_8px_rgba(245,158,11,0.4)]" />
                  <div className="w-2.5 h-2.5 rounded-full bg-emerald-500/80 shadow-[0_0_8px_rgba(16,185,129,0.4)]" />
                </div>
                <div className="text-[10px] uppercase font-bold tracking-widest text-zinc-400 flex items-center gap-2">
                  <Activity className="w-3.5 h-3.5 text-primary" />
                  {t('projectDetail.logs.header')}
                </div>
                <div className="h-4 w-px bg-white/10 mx-2" />
                <div className="flex bg-zinc-950 p-0.5 rounded-md border border-white/5">
                  <button
                    onClick={() => setLogType('web')}
                    className={cn(
                      "px-3 py-1 rounded text-[9px] font-bold uppercase tracking-wider transition-all",
                      logType === 'web' ? "bg-primary text-primary-foreground shadow-lg" : "text-zinc-500 hover:text-zinc-300"
                    )}
                  >
                    {t('projectDetail.logs.web')}
                  </button>
                  <button
                    onClick={() => setLogType('worker')}
                    disabled={!project.worker_container_id && !project.queue_enabled}
                    className={cn(
                      "px-3 py-1 rounded text-[9px] font-bold uppercase tracking-wider transition-all",
                      logType === 'worker' ? "bg-primary text-primary-foreground shadow-lg" : "text-zinc-500 hover:text-zinc-300",
                      (!project.worker_container_id && !project.queue_enabled) && "opacity-30 cursor-not-allowed"
                    )}
                  >
                    {t('projectDetail.logs.worker')}
                  </button>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <button
                  onClick={() => navigator.clipboard.writeText(logs)}
                  className="p-1.5 hover:bg-white/10 rounded-md transition-colors text-zinc-500 hover:text-white"
                  title="Copy Logs"
                >
                  <Copy className="w-3.5 h-3.5" />
                </button>
                <button
                  onClick={() => {
                    const el = document.getElementById('runtime-logs-scroll');
                    if (el) el.scrollTop = el.scrollHeight;
                  }}
                  className="p-1.5 hover:bg-white/10 rounded-md transition-colors text-zinc-500 hover:text-white"
                  title="Scroll to Bottom"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="m7 15 5 5 5-5" /><path d="m7 9 5 5 5-5" /></svg>
                </button>
                <div className="w-px h-3 bg-white/10 mx-1" />
                <Button variant="ghost" size="xs" onClick={() => setLogs('')} className="text-[10px] uppercase font-bold text-zinc-600 hover:text-rose-400">{t('projectDetail.actions.clear')}</Button>
                <Button variant="ghost" size="xs" onClick={fetchLogs} className="h-6 w-6"><RefreshCw size={12} /></Button>
              </div>
            </CardHeader>
            <div id="runtime-logs-scroll" className="flex-1 p-6 overflow-auto font-mono text-[11px] leading-relaxed custom-scrollbar bg-zinc-950">
              {logLines.length > 0 ? logLines.map((line: string, i: number) => {
                const isTimestamp = /^\d{4}-\d{2}-\d{2}/.test(line) || /^\[\d{2}-\w{3}-\d{4}/.test(line)
                return (
                  <div key={i} className="flex gap-4 group py-0.5 px-2 rounded -mx-2 hover:bg-white/[0.02]">
                    <span className="shrink-0 text-zinc-800 select-none w-8 text-right font-light">{logOffset + i + 1}</span>
                    <span className="whitespace-pre-wrap font-mono">
                      {isTimestamp ? (
                        <>
                          <span className="text-zinc-600 mr-2">{line.split(' ')[0]}</span>
                          <span>{line.split(' ').slice(1).join(' ')}</span>
                        </>
                      ) : line}
                    </span>
                  </div>
                )
              }) : (
                <div className="h-full flex flex-col items-center justify-center opacity-10 gap-4">
                  <Activity size={48} />
                  <p className="uppercase tracking-[0.4em] font-bold text-xs">{t('projectDetail.logs.waiting')}</p>
                </div>
              )}
              <div ref={logsEndRef} />
            </div>
          </Card>
        </TabsContent>

        <TabsContent value="build" className="pt-0">
          {activeTab === 'build' && project && <BuildLogsConsole projectId={project.uid} />}
        </TabsContent>

        <TabsContent value="settings" className="pt-0">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <Card>
              <CardHeader>
                <div className="flex items-center gap-4">
                  <div className="w-10 h-10 bg-primary/10 rounded-lg flex items-center justify-center text-primary">
                    {project.framework === 'Laravel' ? <Code2 className="w-5 h-5" /> : <Blocks className="w-5 h-5" />}
                  </div>
                  <div>
                    <CardTitle className="text-lg">
                      {project.framework === 'Laravel' ? t('projectDetail.settings.phpTitle') : t('projectDetail.settings.frameworkStack', { framework: frameworkLabel })}
                    </CardTitle>
                    <CardDescription>
                      {project.framework === 'Laravel' ? t('projectDetail.settings.phpVersion') : 'Configure your application environment'}
                    </CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="space-y-6">
                {project.framework === 'Laravel' ? (
                  <>
                    <div className="space-y-2">
                      <Label className="text-xs uppercase tracking-widest text-muted-foreground">{t('projectDetail.settings.version')}</Label>
                      <Select
                        value={phpVersionInput}
                        onValueChange={(val) => setPhpVersionInput(val || '8.2')}
                      >
                        <SelectTrigger className="h-12 border-muted-foreground/20">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {['8.1', '8.2', '8.3', '8.4'].map(v => (
                            <SelectItem key={v} value={v}>PHP {v} {v === '8.4' ? t('projectDetail.settings.latest') : ''}</SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>

                    <div className="flex items-center justify-between p-4 rounded-xl border bg-muted/20">
                      <div className="space-y-1">
                        <Label className="text-sm font-bold">{t('projectDetail.metrics.queue')}</Label>
                        <p className="text-[10px] text-muted-foreground leading-relaxed">
                          {t('projectDetail.settings.queueHandles')}
                        </p>
                      </div>
                      <Switch
                        checked={queueEnabledInput}
                        onCheckedChange={setQueueEnabledInput}
                      />
                    </div>
                  </>
                ) : (
                  <div className="space-y-6">
                    {isNodeRelated && (
                      <div className="space-y-2">
                        <Label className="text-xs uppercase tracking-widest text-muted-foreground">{t('projectDetail.settings.nodeVersion')}</Label>
                        <Select
                          value={nodeVersionInput}
                          onValueChange={(val) => setNodeVersionInput(val || '20')}
                        >
                          <SelectTrigger className="h-12 border-muted-foreground/20">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {[
                              { v: '18', l: 'Node.js 18 (LTS)' },
                              { v: '20', l: 'Node.js 20 (LTS)' },
                              { v: '22', l: 'Node.js 22 (Current)' }
                            ].map(item => (
                              <SelectItem key={item.v} value={item.v}>{item.l}</SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                    )}

                    {project.framework === 'Go' && (
                      <div className="space-y-2">
                        <Label className="text-xs uppercase tracking-widest text-muted-foreground">Go Version</Label>
                        <Select
                          value={languageVersionInput}
                          onValueChange={(val) => setLanguageVersionInput(val || '1.22')}
                        >
                          <SelectTrigger className="h-12 border-muted-foreground/20">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {['1.20', '1.21', '1.22'].map(v => (
                              <SelectItem key={v} value={v}>Go {v}</SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                    )}

                    {project.framework === 'Python' && (
                      <div className="space-y-2">
                        <Label className="text-xs uppercase tracking-widest text-muted-foreground">Python Version</Label>
                        <Select
                          value={languageVersionInput}
                          onValueChange={(val) => setLanguageVersionInput(val || '3.11')}
                        >
                          <SelectTrigger className="h-12 border-muted-foreground/20">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            {['3.9', '3.10', '3.11', '3.12'].map(v => (
                              <SelectItem key={v} value={v}>Python {v}</SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                    )}

                    <div className="p-4 rounded-xl border bg-muted/20 space-y-4">
                      <div className="flex items-center justify-between">
                        <div className="space-y-1">
                          <Label className="text-sm font-bold">{t('projectDetail.metrics.backgroundService')}</Label>
                          <p className="text-[10px] text-muted-foreground leading-relaxed">
                            Run a secondary process for background tasks
                          </p>
                        </div>
                        <Switch
                          checked={workerCommandInput !== ''}
                          onCheckedChange={(checked: boolean) => !checked && setWorkerCommandInput('')}
                        />
                      </div>

                      {workerCommandInput !== undefined && (
                        <div className="space-y-2 pt-2 border-t border-border">
                          <Label className="text-[10px] uppercase tracking-wider text-muted-foreground">{t('projectDetail.metrics.customCommand')}</Label>
                          <div className="flex gap-2">
                            <Input
                              value={workerCommandInput}
                              onChange={(e) => setWorkerCommandInput(e.target.value)}
                              placeholder="e.g. npm run worker"
                              className="h-9 text-xs font-mono"
                            />
                          </div>
                        </div>
                      )}
                    </div>

                    {isNodeRelated && (
                      <div className="space-y-4 pt-2 border-t">
                        <div className="space-y-2">
                          <Label className="text-xs uppercase tracking-widest text-muted-foreground">{t('projectDetail.settings.buildCommand')}</Label>
                          <div className="flex gap-2">
                            <Input
                              value={buildCommandInput}
                              onChange={(e) => setBuildCommandInput(e.target.value)}
                              placeholder="e.g. npm run build"
                              className="h-10 text-xs font-mono"
                            />
                          </div>
                          <p className="text-[9px] text-muted-foreground italic">{t('projectDetail.settings.buildCommandDesc')}</p>
                        </div>

                        <div className="space-y-2">
                          <Label className="text-xs uppercase tracking-widest text-muted-foreground">{t('projectDetail.settings.startCommand')}</Label>
                          <div className="flex gap-2">
                            <Input
                              value={startCommandInput}
                              onChange={(e) => setStartCommandInput(e.target.value)}
                              placeholder="e.g. node dist/main.js"
                              className="h-10 text-xs font-mono"
                            />
                          </div>
                          <p className="text-[9px] text-muted-foreground italic">{t('projectDetail.settings.startCommandDesc')}</p>
                        </div>
                      </div>
                    )}
                  </div>
                )}

                <p className="text-[10px] text-muted-foreground italic pl-1 flex items-center gap-1.5 mt-2">
                  <AlertTriangle size={10} className="text-amber-500" /> {t('projectDetail.settings.redeployWarning')}
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <div className="flex items-center gap-4">
                  <div className="w-10 h-10 bg-primary/10 rounded-lg flex items-center justify-center text-primary">
                    <Box className="w-5 h-5" />
                  </div>
                  <div>
                    <CardTitle className="text-lg">{t('projectDetail.settings.baseImageTitle')}</CardTitle>
                    <CardDescription>{t('projectDetail.settings.baseImageDesc')}</CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div
                    onClick={() => setRuntimeImageInput('alpine')}
                    className={cn(
                      "p-4 rounded-xl border-2 cursor-pointer transition-all duration-200",
                      runtimeImageInput === 'alpine' ? "border-primary bg-primary/5 shadow-[0_0_15px_rgba(var(--primary),0.1)]" : "border-muted bg-muted/20 hover:border-primary/30"
                    )}
                  >
                    <div className="flex items-center justify-between mb-2">
                      <h4 className="font-bold text-sm">Alpine Linux</h4>
                      {runtimeImageInput === 'alpine' ? (
                        <CheckCircle2 className="w-4 h-4 text-primary fill-primary/20" />
                      ) : (
                        <div className="w-4 h-4 rounded-full border-2 border-muted" />
                      )}
                    </div>
                    <p className="text-[10px] text-muted-foreground leading-relaxed">
                      Ultralight (~200MB). Recommended for most apps.
                    </p>
                  </div>

                  <div
                    onClick={() => setRuntimeImageInput('debian')}
                    className={cn(
                      "p-4 rounded-xl border-2 cursor-pointer transition-all duration-200",
                      runtimeImageInput === 'debian' ? "border-primary bg-primary/5 shadow-[0_0_15px_rgba(var(--primary),0.1)]" : "border-muted bg-muted/20 hover:border-primary/30"
                    )}
                  >
                    <div className="flex items-center justify-between mb-2">
                      <h4 className="font-bold text-sm">Debian Slim</h4>
                      {runtimeImageInput === 'debian' ? (
                        <CheckCircle2 className="w-4 h-4 text-primary fill-primary/20" />
                      ) : (
                        <div className="w-4 h-4 rounded-full border-2 border-muted" />
                      )}
                    </div>
                    <p className="text-[10px] text-muted-foreground leading-relaxed">
                      Better compatibility (~600MB). Use for complex requirements.
                    </p>
                  </div>
                </div>
                <p className="text-[10px] text-muted-foreground italic pl-1 flex items-center gap-1.5">
                  <AlertTriangle size={10} className="text-amber-500" /> {t('projectDetail.settings.redeployWarning')}
                </p>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <div className="flex items-center gap-4">
                  <div className="w-10 h-10 bg-blue-500/10 rounded-lg flex items-center justify-center text-blue-600">
                    <RefreshCw className="w-5 h-5" />
                  </div>
                  <div>
                    <CardTitle className="text-lg">{t('projectDetail.settings.branchTitle')}</CardTitle>
                    <CardDescription>{t('projectDetail.settings.branchDesc')}</CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="space-y-6">
                <div className="space-y-2">
                  <Label className="text-xs uppercase tracking-widest text-muted-foreground">{t('projectDetail.settings.branchTitle')}</Label>
                  <div className="flex gap-2">
                    <Input
                      value={branchInput}
                      onChange={(e) => setBranchInput(e.target.value)}
                      placeholder={t('projectDetail.settings.branchPlaceholder')}
                      className="h-10"
                    />
                  </div>
                  <p className="text-[10px] text-muted-foreground italic pl-1 flex items-center gap-1.5 mt-2">
                    <AlertTriangle size={10} className="text-amber-500" /> {t('projectDetail.settings.redeployWarning')}
                  </p>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <div className="flex items-center gap-4">
                  <div className="w-10 h-10 bg-muted/50 rounded-lg flex items-center justify-center text-muted-foreground">
                    <Layout className="w-5 h-5" />
                  </div>
                  <div>
                    <CardTitle className="text-lg">{t('newProject.baseDir')}</CardTitle>
                    <CardDescription>{t('newProject.baseDirDesc')}</CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="space-y-6">
                <div className="space-y-2">
                  <Label className="text-xs uppercase tracking-widest text-muted-foreground">{t('newProject.baseDir')}</Label>
                  <div className="flex gap-2">
                    <Input
                      value={baseDirInput}
                      onChange={(e) => setBaseDirInput(e.target.value)}
                      placeholder={t('newProject.baseDirPlaceholder')}
                      className="h-10"
                    />
                  </div>
                  <p className="text-[10px] text-muted-foreground italic pl-1 flex items-center gap-1.5 mt-2">
                    <AlertTriangle size={10} className="text-amber-500" /> {t('projectDetail.settings.redeployWarning')}
                  </p>
                </div>
              </CardContent>
            </Card>
          </div>

          {/* Floating Save Action Bar for Settings */}
          {activeTab === 'settings' && isSettingsDirty && (
            <div className="fixed bottom-10 left-1/2 -translate-x-1/2 z-50 animate-in fade-in slide-in-from-bottom-10 duration-500">
              <div className="relative group">
                {/* Glow effect background */}
                <div className="absolute -inset-1 bg-gradient-to-r from-primary/50 to-blue-500/50 rounded-[22px] blur-xl opacity-20 group-hover:opacity-40 transition duration-1000 group-hover:duration-200"></div>

                <Card className="relative bg-zinc-950/90 backdrop-blur-2xl border-white/10 shadow-[0_20px_50px_rgba(0,0,0,0.5)] overflow-hidden min-w-[360px] rounded-[20px]">
                  <div className="absolute top-0 left-0 w-full h-[1px] bg-gradient-to-r from-transparent via-white/20 to-transparent" />

                  <CardContent className="p-3 flex items-center justify-between gap-8">
                    <div className="flex items-center gap-4 pl-3">
                      <div className="relative flex h-2 w-2">
                        <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-primary opacity-75"></span>
                        <span className="relative inline-flex rounded-full h-2 w-2 bg-primary"></span>
                      </div>
                      <div className="flex flex-col">
                        <span className="text-[10px] font-black uppercase tracking-[0.2em] text-white/90 leading-tight">Configuration Changed</span>
                        <span className="text-[9px] text-white/40 font-bold uppercase tracking-widest">{t('common.settings')}</span>
                      </div>
                    </div>

                    <div className="flex items-center gap-2 pr-1">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={handleResetSettings}
                        disabled={isSavingSettings}
                        className="text-[10px] font-black uppercase tracking-widest h-10 px-4 text-white/50 hover:text-white hover:bg-white/5 transition-all"
                      >
                        {t('common.cancel')}
                      </Button>
                      <Button
                        size="sm"
                        onClick={handleSaveSettings}
                        disabled={isSavingSettings}
                        className="relative group/btn bg-white text-black hover:bg-white/90 h-10 px-6 rounded-full font-black text-[10px] uppercase tracking-wider transition-all overflow-hidden"
                      >
                        {isSavingSettings ? (
                          <Loader2 className="w-3.5 h-3.5 animate-spin" />
                        ) : (
                          <div className="flex items-center gap-2">
                            <Save className="w-3.5 h-3.5" />
                            <span>{t('common.save')}</span>
                          </div>
                        )}
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              </div>
            </div>
          )}
        </TabsContent>
      </Tabs>
    </div>
  )
}

function CredentialRow({ label, value, isSecret = false }: { label: string, value: string, isSecret?: boolean }) {
  const { t } = useTranslation()
  const copy = () => {
    navigator.clipboard.writeText(value)
    toast.success(t('common.copySuccess'))
  }

  return (
    <div className="p-4 rounded-xl bg-muted/50 border hover:border-primary/20 transition-colors group">
      <div className="flex items-center justify-between mb-2">
        <span className="text-[9px] font-bold text-muted-foreground uppercase tracking-widest">{label}</span>
        <Button variant="ghost" size="icon" className="h-6 w-6 opacity-0 group-hover:opacity-100 transition-opacity" onClick={copy}>
          <Copy className="w-3 h-3" />
        </Button>
      </div>
      <div className={cn("text-xs font-mono truncate font-bold", isSecret && "opacity-30 select-none")}>
        {isSecret ? "••••••••••••••••" : value}
      </div>
    </div>
  )
}

export default StudentProjectDetail
