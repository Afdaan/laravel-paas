import { useState, useEffect, useRef } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import useTranslation from '../../lib/useTranslation'
import {
  RefreshCw,
  ExternalLink,
  Trash2,
  Cpu,
  Activity,
  Zap,
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
  Copy
} from 'lucide-react'
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
import { Switch } from '@/components/ui/switch'
import { usePolling } from '@/lib/usePolling'
import { cn } from '@/lib/utils'

// Status Indicator Component
function StatusIndicator({ status }: { status: string }) {
  const { t } = useTranslation()
  const styles: Record<string, any> = {
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

function MetricCard({ title, value, subtext, icon: Icon, colorClass }: { title: string, value: string, subtext?: string, icon: any, colorClass?: string }) {
  return (
    <Card className="hover:border-primary/30 transition-colors">
      <CardContent className="p-6">
        <div className="flex justify-between items-start mb-4">
          <p className="text-muted-foreground text-[10px] font-bold uppercase tracking-widest">{title}</p>
          <Icon className={cn("w-4 h-4", colorClass || "text-primary")} />
        </div>
        <div>
          <div className="text-2xl font-bold tracking-tight">{value}</div>
          {subtext && <div className="text-[10px] text-muted-foreground font-medium mt-1 uppercase truncate">{subtext}</div>}
        </div>
      </CardContent>
    </Card>
  )
}

function StudentProjectDetail() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [project, setProject] = useState<Project | null>(null)
  const [logs, setLogs] = useState('')
  const [stats, setStats] = useState<ProjectStats | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [activeTab, setActiveTab] = useState('project')
  const logsEndRef = useRef<HTMLDivElement>(null)

  const [envContent, setEnvContent] = useState('')
  const [consoleOutput, setConsoleOutput] = useState('')
  const [consoleCommand, setConsoleCommand] = useState('')
  const [isExecuting, setIsExecuting] = useState(false)
  const [isEnvHidden, setIsEnvHidden] = useState(true)
  const [isSavingEnv, setIsSavingEnv] = useState(false)
  const [credentials, setCredentials] = useState<any>(null)

  const [confirmModal, setConfirmModal] = useState({
    isOpen: false,
    title: '',
    message: '' as React.ReactNode,
    type: 'danger' as 'danger' | 'warning' | 'info',
    onConfirm: () => { },
    confirmText: t('common.confirm')
  })

  const [consecutiveErrors, setConsecutiveErrors] = useState(0)

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
    if (activeTab === 'environment') {
      fetchEnv()
    }
    if (activeTab === 'database') {
      fetchCredentials()
    }
  }, [activeTab, id])

  const fetchProject = async () => {
    if (!id) return
    try {
      const response = await projectsAPI.get(id)
      setProject(response.data)
      setConsecutiveErrors(0)
    } catch (error: any) {
      if (error.response?.status === 401) {
        navigate('/login')
        return
      }

      if (error.response?.status === 404) {
        toast.error(t('projectDetail.messages.notFound') || 'Project not found')
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
  }

  const fetchLogs = async () => {
    if (!id) return
    try {
      const response = await projectsAPI.logs(id, 200)
      setLogs(response.data.logs)
      if (logsEndRef.current) {
        logsEndRef.current.scrollIntoView({ behavior: 'smooth' })
      }
    } catch (error) { }
  }

  const fetchStats = async () => {
    if (!id) return
    try {
      const response = await projectsAPI.stats(id)
      setStats(response.data)
    } catch (error) {
      setStats(null)
    }
  }

  const fetchEnv = async () => {
    if (!id) return
    try {
      const response = await projectsAPI.getEnv(id)
      setEnvContent(response.data.content)
    } catch (error) {
      toast.error(t('common.error'))
    }
  }

  const fetchCredentials = async () => {
    if (!id) return
    try {
       const response = await databaseAPI.getCredentials(id)
       setCredentials(response.data)
    } catch (error) {}
  }

  const handleConsoleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!id || !consoleCommand.trim()) return

    setIsExecuting(true)
    setConsoleOutput(prev => prev + `\n$ php artisan ${consoleCommand}\n`)

    try {
      const response = await projectsAPI.runArtisan(id, consoleCommand)
      setConsoleOutput(prev => prev + response.data.output + '\n')
      setConsoleCommand('')
    } catch (error: any) {
      const errOut = error.response?.data?.output || error.message
      setConsoleOutput(prev => prev + `Error: ${errOut}\n`)
    } finally {
      setIsExecuting(false)
    }
  }

  const handleRedeploy = () => {
    if (!id) return
    setConfirmModal({
      title: t('projectDetail.messages.redeployConfirm'),
      message: t('projectDetail.messages.redeployDesc'),
      type: 'warning',
      confirmText: t('projectDetail.actions.redeploy'),
      isOpen: true,
      onConfirm: () => {
        setProject(prev => prev ? ({ ...prev, status: 'queued' }) : null)
        toast.promise(
          projectsAPI.redeploy(id),
          {
            loading: t('common.loading'),
            success: t('projectDetail.actions.redeployStarted'),
            error: t('common.error'),
          }
        )
      }
    })
  }

  const handleUpdatePHP = async (newVersion: string | null) => {
    if (!id || !newVersion) return
    setConfirmModal({
      title: t('projectDetail.messages.updatePHPConfirm', { version: newVersion }),
      message: t('projectDetail.messages.redeployDesc'),
      type: 'warning',
      confirmText: t('common.confirm'),
      isOpen: true,
      onConfirm: async () => {
        try {
          await projectsAPI.update(id, { php_version: newVersion })
          setProject(prev => prev ? ({ ...prev, php_version: newVersion, is_manual_version: true }) : null)
          toast.success(t('common.success'))
          projectsAPI.redeploy(id).then(() => fetchProject())
        } catch (err) {
          toast.error(t('common.error'))
        }
      }
    })
  }

  const handleSaveEnv = async () => {
    if (!id) return
    setIsSavingEnv(true)
    try {
      await projectsAPI.updateEnv(id, envContent)
      toast.success(t('common.success'))
      projectsAPI.redeploy(id).then(() => fetchProject())
    } catch (error) {
      toast.error(t('common.error'))
    } finally {
      setIsSavingEnv(false)
    }
  }

  const handleUpdateQueue = async (checked: boolean) => {
    if (!id) return
    try {
      await projectsAPI.update(id, { queue_enabled: checked })
      setProject(prev => prev ? ({ ...prev, queue_enabled: checked }) : null)

      const message = checked
        ? t('projectDetail.messages.queueEnabled')
        : t('projectDetail.messages.queueDisabled')

      toast.success(message)
      projectsAPI.redeploy(id).then(() => fetchProject())
    } catch (error) {
      toast.error(t('common.error'))
    }
  }

  const handleDelete = () => {
    if (!id) return
    setConfirmModal({
      title: t('projectDetail.messages.deleteConfirm'),
      message: t('projectDetail.messages.deleteDesc'),
      type: 'danger',
      confirmText: t('projectDetail.actions.delete'),
      isOpen: true,
      onConfirm: () => {
        toast.promise(
          projectsAPI.delete(id),
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

  return (
    <div className="space-y-6 max-w-7xl mx-auto pb-20 animate-in fade-in duration-500">
      <ConfirmationModal
        onClose={() => setConfirmModal(prev => ({ ...prev, isOpen: false }))}
        {...confirmModal}
      />

      {/* Building Banner */}
      {project.status === 'building' && (
        <Card className="border-blue-500/20 bg-blue-500/5 p-6 animate-pulse">
          <div className="flex items-center gap-6">
            <div className="w-12 h-12 bg-blue-500/20 rounded-xl flex items-center justify-center text-blue-500 shrink-0">
              <Box className="w-6 h-6" />
            </div>
            <div className="flex-1">
              <h3 className="text-lg font-bold">{t('projectDetail.messages.buildTitle')}</h3>
              <p className="text-sm text-muted-foreground">{t('projectDetail.messages.buildDesc')}</p>
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
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-6 w-6 ml-1"
                  render={
                    <a href={projectUrl} target="_blank" rel="noopener noreferrer">
                      <ExternalLink className="w-3 h-3 text-primary" />
                    </a>
                  }
                />
              )}
            </div>
            <Badge variant="outline" className="gap-1.5 bg-muted/50 border-border/50">
              <Code className="w-3 h-3" />
              v1.4.2
            </Badge>
          </div>
        </div>

        <div className="flex gap-3">
          <Button variant="outline" onClick={handleRedeploy} className="gap-2">
            <RefreshCw className="w-4 h-4" />
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
          title={t('projectDetail.metrics.php')}
          value={project.php_version ? `PHP ${project.php_version.replace('.dynamic', '')}` : '...'}
          subtext={project.is_manual_version ? t('projectDetail.metrics.customRuntime') : t('projectDetail.metrics.standardRuntime')}
          icon={Zap}
        />
        <MetricCard
          title={t('projectDetail.metrics.db')}
          value="MySQL"
          subtext={project.database_name || t('projectDetail.metrics.noDb')}
          icon={DatabaseIcon}
        />
        <MetricCard
          title={t('projectDetail.metrics.queue')}
          value={project.queue_enabled ? t('projectDetail.metrics.active') : t('projectDetail.metrics.inactive')}
          subtext={project.queue_enabled ? t('projectDetail.metrics.background') : t('projectDetail.metrics.direct')}
          colorClass={project.queue_enabled ? 'text-emerald-500' : ''}
          icon={RefreshCw}
        />
      </div>

      {/* Tabs */}
      <Tabs defaultValue="project" onValueChange={setActiveTab} className="w-full">
        <TabsList className="bg-muted p-1 rounded-lg w-fit overflow-x-auto">
          <TabsTrigger value="project">{t('projectDetail.tabs.overview')}</TabsTrigger>
          <TabsTrigger value="console">{t('projectDetail.tabs.console')}</TabsTrigger>
          <TabsTrigger value="environment">{t('projectDetail.tabs.secrets')}</TabsTrigger>
          <TabsTrigger value="database">{t('projectDetail.tabs.database')}</TabsTrigger>
          <TabsTrigger value="logs">{t('projectDetail.tabs.logs')}</TabsTrigger>
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
                    <label className="text-[9px] font-bold text-muted-foreground uppercase tracking-widest mb-1 block">{t('projectDetail.overview.runtime')}</label>
                    <div className="flex items-center gap-1.5 font-bold text-xs uppercase">
                      <Zap className="w-3 h-3 text-amber-500" />
                      {project.laravel_version ? project.laravel_version : 'Laravel 10'}
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
          <Card className="bg-black text-white border-zinc-800 overflow-hidden flex flex-col h-[600px] gap-0 py-0">
            <CardHeader className="bg-zinc-900 px-4 py-3 border-b border-white/10 flex flex-row items-center justify-between">
              <div className="flex items-center gap-3">
                <div className="flex gap-1.5 mr-2">
                  <div className="w-2.5 h-2.5 rounded-full bg-red-500/80" />
                  <div className="w-2.5 h-2.5 rounded-full bg-amber-500/80" />
                  <div className="w-2.5 h-2.5 rounded-full bg-green-500/80" />
                </div>
                <div className="text-[10px] uppercase font-bold tracking-widest text-zinc-400 flex items-center gap-2">
                  <TerminalIcon className="w-3.5 h-3.5" /> {t('projectDetail.console.header')}
                </div>
              </div>
              <Button variant="ghost" size="xs" onClick={() => setConsoleOutput('')} className="text-[10px] uppercase font-bold text-zinc-500 hover:text-white">{t('projectDetail.actions.clear')}</Button>
            </CardHeader>

            <div className="flex-1 p-4 overflow-auto font-mono text-xs text-zinc-300 custom-scrollbar bg-zinc-950/50">
              <div className="text-amber-400/80 mb-4 flex items-center gap-2 border-b border-white/10 pb-2">
                <AlertTriangle size={12} />
                <span>{t('projectDetail.console.artisanPrefix')}</span>
              </div>
              {consoleOutput ? consoleOutput.split('\n').map((line, i) => (
                <div key={i} className={cn("flex gap-4", line.startsWith('$') ? "text-primary mt-1" : "text-zinc-400")}>
                  <span className="shrink-0 text-zinc-700 select-none w-6 text-right">{i + 1}</span>
                  <span className="break-all whitespace-pre-wrap">{line}</span>
                </div>
              )) : (
                <div className="h-full flex flex-col items-center justify-center opacity-10 gap-4">
                  <TerminalIcon size={48} />
                  <p className="uppercase tracking-[0.3em] font-bold">{t('projectDetail.console.terminalReady')}</p>
                </div>
              )}
              {isExecuting && <div className="mt-4 flex items-center gap-2 text-primary animate-pulse"><RefreshCw className="w-3 h-3 animate-spin" /> {t('common.executing')}</div>}
              <div ref={logsEndRef} />
            </div>

            <form onSubmit={handleConsoleSubmit} className="p-4 bg-zinc-900 border-t border-white/10 flex gap-3">
              <div className="flex items-center px-3 bg-zinc-800 rounded font-mono text-xs text-zinc-500">php artisan</div>
              <Input
                value={consoleCommand}
                onChange={e => setConsoleCommand(e.target.value)}
                placeholder="migrate --seed"
                disabled={isExecuting}
                className="flex-1 bg-zinc-800 border-none text-white font-mono text-xs focus-visible:ring-1 focus-visible:ring-primary h-9"
              />
              <Button type="submit" disabled={isExecuting || !consoleCommand.trim()} size="sm">{t('projectDetail.actions.execute')}</Button>
            </form>
          </Card>
        </TabsContent>

        <TabsContent value="environment" className="pt-0">
          <Card className="flex flex-col h-[600px] overflow-hidden">
            <CardHeader className="pb-4 flex flex-row items-center justify-between">
              <div>
                <CardTitle className="text-lg">{t('projectDetail.tabs.secrets')}</CardTitle>
                <CardDescription>{t('projectDetail.secrets.desc')}</CardDescription>
              </div>
              <div className="flex items-center gap-3">
                <Button variant="outline" size="sm" onClick={() => setIsEnvHidden(!isEnvHidden)}>
                  {isEnvHidden ? <Eye className="w-4 h-4 mr-2" /> : <EyeOff className="w-4 h-4 mr-2" />}
                  {isEnvHidden ? t('projectDetail.actions.reveal') : t('projectDetail.actions.hide')}
                </Button>
                <Button size="sm" onClick={handleSaveEnv} disabled={isSavingEnv || isEnvHidden}>
                  {isSavingEnv ? <Loader2 className="w-4 h-4 mr-2 animate-spin" /> : <Save className="w-4 h-4 mr-2" />}
                  {t('common.save')}
                </Button>
              </div>
            </CardHeader>
            <div className="flex-1 relative bg-muted/20">
              <Textarea
                value={envContent}
                onChange={e => setEnvContent(e.target.value)}
                readOnly={isEnvHidden}
                spellCheck={false}
                className={cn(
                  "absolute inset-0 h-full w-full rounded-none border-none p-8 font-mono text-xs leading-relaxed resize-none bg-transparent focus-visible:ring-0",
                  isEnvHidden && "blur-sm select-none opacity-50"
                )}
                placeholder={t('projectDetail.secrets.placeholder')}
              />
              {isEnvHidden && (
                <div className="absolute inset-0 flex items-center justify-center pointer-events-none">
                  <Badge variant="secondary" className="px-4 py-2 border shadow-lg tracking-widest uppercase">{t('projectDetail.secrets.locked')}</Badge>
                </div>
              )}
            </div>
            <div className="p-3 bg-amber-500/5 text-amber-600 text-[10px] font-bold uppercase tracking-widest border-t flex items-center justify-center gap-2">
              <AlertTriangle size={12} /> {t('projectDetail.secrets.redeployNote')}
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
            <DatabaseManager embedded={true} projectId={id} />
          </div>
        </TabsContent>

        <TabsContent value="logs" className="pt-0">
          <Card className="bg-black text-zinc-300 border-zinc-800 overflow-hidden flex flex-col h-[600px] gap-0 py-0">
            <CardHeader className="bg-zinc-900 px-4 py-3 border-b border-white/10 flex flex-row items-center justify-between">
              <div className="text-[10px] uppercase font-bold tracking-widest text-zinc-400 flex items-center gap-2">
                <Activity className="w-3.5 h-3.5 text-primary" /> {t('projectDetail.logs.header')}
              </div>
              <Button variant="ghost" size="xs" onClick={fetchLogs} className="h-6 w-6"><RefreshCw size={12} /></Button>
            </CardHeader>
            <div className="flex-1 p-4 overflow-auto font-mono text-xs leading-relaxed custom-scrollbar bg-black/40">
              {logs ? logs.split('\n').map((line, i) => (
                <div key={i} className="flex gap-4 hover:bg-white/5 py-0.5 px-2 rounded -mx-2">
                  <span className="shrink-0 text-zinc-700 select-none w-8 text-right">{i + 1}</span>
                  <span className="whitespace-pre-wrap">{line}</span>
                </div>
              )) : (
                <div className="h-full flex flex-col items-center justify-center opacity-10 gap-4">
                  <Activity size={48} />
                  <p className="uppercase tracking-[0.3em] font-bold">{t('projectDetail.logs.waiting')}</p>
                </div>
              )}
              <div ref={logsEndRef} />
            </div>
          </Card>
        </TabsContent>

        <TabsContent value="settings" className="pt-0">
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <Card>
              <CardHeader>
                <div className="flex items-center gap-4">
                  <div className="w-10 h-10 bg-primary/10 rounded-lg flex items-center justify-center text-primary">
                    <Zap className="w-5 h-5" />
                  </div>
                  <div>
                    <CardTitle className="text-lg">{t('projectDetail.settings.phpTitle')}</CardTitle>
                    <CardDescription>{t('projectDetail.settings.phpVersion')}</CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent className="space-y-6">
                <div className="space-y-2">
                  <Label className="text-xs uppercase tracking-widest text-muted-foreground">{t('projectDetail.settings.version')}</Label>
                  <Select
                    value={project.php_version?.split('.dynamic')[0] || '8.2'}
                    onValueChange={(value) => handleUpdatePHP(value)}
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
                  <p className="text-[10px] text-muted-foreground italic pl-1 flex items-center gap-1.5 mt-2">
                    <AlertTriangle size={10} className="text-amber-500" /> {t('projectDetail.settings.redeployWarning')}
                  </p>
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <div className="flex items-center gap-4">
                  <div className="w-10 h-10 bg-emerald-500/10 rounded-lg flex items-center justify-center text-emerald-600">
                    <RefreshCw className="w-5 h-5" />
                  </div>
                  <div>
                    <CardTitle className="text-lg">{t('projectDetail.metrics.queue')}</CardTitle>
                    <CardDescription>{t('projectDetail.settings.queueConfig')}</CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                <div className="flex items-center justify-between p-4 bg-muted/50 rounded-xl border">
                  <div className="space-y-1">
                    <div className="flex items-center gap-2">
                      <code className="bg-muted px-1.5 py-0.5 rounded text-xs font-mono font-bold text-primary border border-primary/10">php artisan queue:work</code>
                    </div>
                    <p className="text-[10px] text-muted-foreground font-medium uppercase tracking-wider">{t('projectDetail.settings.queueHandles')}</p>
                  </div>
                  <div className="flex items-center gap-4">
                    <span className={cn(
                      "text-[10px] font-bold uppercase tracking-wider px-2 py-1 rounded-md border transition-colors whitespace-nowrap",
                      project.queue_enabled
                        ? "text-emerald-500 bg-emerald-500/10 border-emerald-500/20"
                        : "text-muted-foreground bg-muted/50 border-transparent"
                    )}>
                      {project.queue_enabled ? t('common.enabled') : t('common.disabled')}
                    </span>
                    <Switch
                      checked={project.queue_enabled}
                      onCheckedChange={handleUpdateQueue}
                    />
                  </div>
                </div>
              </CardContent>
            </Card>
          </div>
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
