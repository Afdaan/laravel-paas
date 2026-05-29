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
  Settings,
  Loader2,
  Save,
  Copy,
  Blocks,
  ArrowUpRight,
  Code2
} from 'lucide-react'
import { AxiosError } from 'axios'
import { projectsAPI, databaseAPI } from '../../services/api'
import { Project, ProjectStats, DeploymentEvent } from '../../types'
import ConfirmationModal from '../../components/ConfirmationModal'
import DatabaseStudio from './DatabaseStudio'
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
import { RedeployButton } from '../../components/project/RedeployButton'
import { RestartButton } from '../../components/project/RestartButton'
import { EnvironmentEditor } from '../../components/project/EnvironmentEditor'
import { CustomDomainManager } from '../../components/project/CustomDomainManager'
import { RuntimeTab } from '../../components/project/RuntimeTab'

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
    restarting: { color: 'text-indigo-500 bg-indigo-500/10 border-indigo-500/20', label: t('status.restarting'), pulse: true },
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



function UserProjectDetail() {
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
  const isActionPendingRef = useRef(false)

  const [consoleOutput, setConsoleOutput] = useState('')
  const [consoleCommand, setConsoleCommand] = useState('')
  const [isExecuting, setIsExecuting] = useState(false)
  const [credentials, setCredentials] = useState<Record<string, string> | null>(null)
  const [branchInput, setBranchInput] = useState('')
  const [baseDirInput, setBaseDirInput] = useState('')
  const [buildCommandInput, setBuildCommandInput] = useState('')
  const [startCommandInput, setStartCommandInput] = useState('')
  const [nodeVersionInput, setNodeVersionInput] = useState('')
  const [phpVersionInput, setPhpVersionInput] = useState('')
  const [workerCommandInput, setWorkerCommandInput] = useState('')
  const [queueEnabledInput, setQueueEnabledInput] = useState(false)
  const [languageVersionInput, setLanguageVersionInput] = useState('')
  const [isSavingSettings, setIsSavingSettings] = useState(false)
  
  const [consoleClearedLength, setConsoleClearedLength] = useState(0)
  const [clearedLogsMap, setClearedLogsMap] = useState<Record<string, string>>({})

  const [runtimeEvents, setRuntimeEvents] = useState<DeploymentEvent[]>([])
  const [isRollingBack, setIsRollingBack] = useState(false)
  const [rollbackCommitSHA, setRollbackCommitSHA] = useState('')

  const fetchRuntimeEvents = useCallback(async () => {
    if (!uid) return
    try {
      const response = await projectsAPI.getDeploymentEvents(uid, true)
      setRuntimeEvents(response.data)
    } catch (err) {
      setRuntimeEvents([])
    }
  }, [uid])

  useEffect(() => {
    if (activeTab === 'runtime') {
      fetchRuntimeEvents()
      const interval = setInterval(fetchRuntimeEvents, 5000)
      return () => clearInterval(interval)
    }
  }, [activeTab, fetchRuntimeEvents])

  // Support deep linking to tabs (e.g. ?tab=build or #build)
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const tabParam = params.get('tab')
    const allowedTabs = ['project', 'runtime', 'console', 'environment', 'database', 'logs', 'build', 'domains', 'settings']
    if (tabParam && allowedTabs.includes(tabParam)) {
      setActiveTab(tabParam)
    } else {
      const hash = window.location.hash.replace('#', '')
      if (hash && allowedTabs.includes(hash)) {
        setActiveTab(hash)
      }
    }
  }, [])

  const handleRollback = async (commitSHA: string) => {
    if (!uid) return
    setIsRollingBack(true)
    try {
      const response = await projectsAPI.rollback(uid, commitSHA)
      toast.success(t('projectDetail.runtime.rollbackSuccess', { type: response.data.type }) || `Rollback initiated successfully (${response.data.type})`)
      fetchProject(true)
      fetchRuntimeEvents()
    } catch (error: unknown) {
      const axiosErr = error as { response?: { data?: { error?: string } }; message?: string }
      const errMsg = axiosErr.response?.data?.error || axiosErr.message || 'Unknown error'
      toast.error(t('projectDetail.runtime.rollbackFailed', { error: errMsg }) || `Failed to initiate rollback: ${errMsg}`)
    } finally {
      setIsRollingBack(false)
    }
  }

  const [confirmModal, setConfirmModal] = useState({
    isOpen: false,
    title: '',
    message: '' as React.ReactNode,
    type: 'danger' as 'danger' | 'warning' | 'info',
    onConfirm: () => { },
    confirmText: t('common.confirm')
  })

  const triggerRollbackConfirm = (commitSHA: string) => {
    setConfirmModal({
      title: t('projectDetail.runtime.confirmRollback') || 'Confirm Rollback',
      message: t('projectDetail.runtime.confirmRollbackMsg') || 'Are you sure you want to rollback to this commit? If the image is cached locally, it will perform a zero-downtime hot-swap. Otherwise, it will trigger an automated rebuild.',
      type: 'warning',
      confirmText: t('projectDetail.runtime.rollbackBtn') || 'Rollback',
      isOpen: true,
      onConfirm: () => {
        handleRollback(commitSHA)
      }
    })
  }

  const checkpoints = useMemo(() => {
    const seen = new Set<string>()
    const list: { sha: string, time: string, message: string }[] = []
    
    // Create a map of jobId -> commitMessage from "building_image" events
    const commitMsgMap = new Map<string, string>()
    runtimeEvents.forEach(evt => {
      if (evt.event_type === 'building_image' && evt.job_id) {
        // evt.payload looks like: "Commit abcd123: Add custom domain setup"
        // Let's extract everything after the colon
        const parts = evt.payload ? evt.payload.split(':') : []
        if (parts.length > 1) {
          commitMsgMap.set(evt.job_id, parts.slice(1).join(':').trim())
        }
      }
    })
    
    runtimeEvents.forEach(evt => {
      if (evt.event_type === 'deployment_completed' || evt.event_type === 'rollback_completed' || evt.event_type === 'deployment_skipped_existing_image') {
        const sha = evt.payload || evt.message
        // Validate as a real 40-char hex SHA — prevents arbitrary messages from becoming fake checkpoints
        const isValidSha = sha && /^[0-9a-f]{40}$/i.test(sha)
        if (isValidSha && !seen.has(sha)) {
          seen.add(sha)
          const commitMsg = (evt.job_id && commitMsgMap.get(evt.job_id)) || evt.message || `Deployment version ${sha.substring(0, 7)}`
          list.push({
            sha,
            time: new Date(evt.created_at).toLocaleString(),
            message: commitMsg
          })
        }
      }
    })
    return list
  }, [runtimeEvents])

  const isNodeRelated = ['Node.js', 'Next.js', 'Vite', 'React', 'Vue', 'Nuxt.js', 'Svelte', 'Angular', 'TypeScript'].includes(project?.framework || '')

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

  const isDeploying = Boolean(project?.deployment_status && !['completed', 'failed', 'rollback', 'cancelled'].includes(project.deployment_status))
  const deployLocked = isDeploying || project?.status === 'queued' || project?.status === 'pending' || project?.status === 'building' || project?.status === 'restarting'
  
  const deploymentPhase = useMemo(() => {
    if (!project?.deployment_status || !isDeploying) return null
    const status = project.deployment_status
    if (['queued', 'preparing', 'cloning'].includes(status)) return { label: 'Preparing', phase: 'build' }
    if (['building', 'provisioning'].includes(status)) return { label: 'Building', phase: 'build' }
    if (['starting', 'healthchecking', 'migrating'].includes(status)) return { label: 'Starting Container', phase: 'startup' }
    if (['promoting', 'cleanup'].includes(status)) return { label: 'Finalizing', phase: 'finalize' }
    return { label: 'Deploying', phase: 'build' }
  }, [project?.deployment_status, isDeploying])
  
  const displayStatus = deploymentPhase ? 'building' : project?.status

  const fetchProject = useCallback(async (forceUpdate = false) => {
    if (!uid) return
    try {
      const response = await projectsAPI.get(uid)
      // Prevent stale polling overwrites during optimistic action phases unless explicitly forced
      if (typeof forceUpdate !== 'boolean') forceUpdate = false // Handle accidental event objects
      if (!forceUpdate && isActionPendingRef.current) return
      
      setProject(response.data)
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

  const handleDeploymentEvent = useCallback(() => {
    fetchProject(true)
  }, [fetchProject])

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
    if (activeTab === 'database') {
      fetchCredentials()
    }
  }, [activeTab, uid, fetchCredentials])

  const { visibleLogLines, logOffset } = useMemo(() => {
    if (!logs) return { visibleLogLines: [], logOffset: 0 }
    
    const clearedForType = clearedLogsMap[logType] || ''
    let visibleLogs = logs
    if (clearedForType) {
      const clearedLines = clearedForType.trimEnd().split('\n')
      const currentLines = logs.trimEnd().split('\n')
      let overlapLines = 0
      const maxCheck = Math.min(clearedLines.length, currentLines.length)
      
      for (let k = maxCheck; k > 0; k--) {
        let match = true
        for (let i = 0; i < k; i++) {
          if (clearedLines[clearedLines.length - k + i] !== currentLines[i]) {
            match = false
            break
          }
        }
        if (match) {
          overlapLines = k
          break
        }
      }
      visibleLogs = currentLines.slice(overlapLines).join('\n')
    }

    const lines = visibleLogs.split('\n').filter(l => l.trim() !== '' || l === '')
    const slicedLines = lines.length > 500 ? lines.slice(-500) : lines
    const offset = lines.length > 500 ? lines.length - 500 : 0
    
    return { visibleLogLines: slicedLines, logOffset: offset }
  }, [logs, clearedLogsMap, logType])

  const consoleLines = useMemo(() => {
    if (!consoleOutput) return []
    const visibleOutput = consoleClearedLength > 0 ? consoleOutput.substring(consoleClearedLength) : consoleOutput
    const lines = visibleOutput.split('\n').filter(l => l.trim() !== '' || l === '')
    return lines.length > 500 ? lines.slice(-500) : lines
  }, [consoleOutput, consoleClearedLength])

  const consoleOffset = useMemo(() => {
    if (!consoleOutput) return 0
    const visibleOutput = consoleClearedLength > 0 ? consoleOutput.substring(consoleClearedLength) : consoleOutput
    const lines = visibleOutput.split('\n').filter(l => l.trim() !== '' || l === '')
    return lines.length > 500 ? lines.length - 500 : 0
  }, [consoleOutput, consoleClearedLength])


  const handleClearConsole = () => {
    setConfirmModal({
      title: t('projectDetail.actions.clear'),
      message: t('common.confirmClearLogs') || 'Confirm clearing the console? New logs will still appear.',
      type: 'warning',
      confirmText: t('common.confirm'),
      isOpen: true,
      onConfirm: () => {
        setConsoleClearedLength(consoleOutput.length)
        toast.success(t('common.success'))
      }
    })
  }

  const handleClearLogs = () => {
    setConfirmModal({
      title: t('projectDetail.actions.clear'),
      message: t('common.confirmClearLogs') || 'Confirm clearing the logs? New logs will still appear.',
      type: 'warning',
      confirmText: t('common.confirm'),
      isOpen: true,
      onConfirm: () => {
        setClearedLogsMap(prev => ({ ...prev, [logType]: logs }))
        toast.success(t('common.success'))
      }
    })
  }

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
  
  const onActionStarted = (status: Project['status'] = 'queued') => {
    isActionPendingRef.current = true
    setProject(prev => prev ? ({ ...prev, status }) : null)
  }

  const onDeployStarted = () => {
    isActionPendingRef.current = true
    setProject(prev => prev ? ({ ...prev, status: 'building', deployment_status: 'queued', deployment_progress: 0 }) : null)
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
      isActionPendingRef.current = false
      fetchProject(true)
    } catch (error: unknown) {
      isActionPendingRef.current = false
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
      isActionPendingRef.current = false
      fetchProject(true)
    } catch (error: unknown) {
      isActionPendingRef.current = false
      const axiosError = error as AxiosError
      if (axiosError?.response?.status === 404) {
        toast.error(t('projectDetail.messages.startUnavailable'))
      }
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
      workerCommandInput !== (project.worker_command || '') ||
      queueEnabledInput !== (project.queue_enabled || false) ||
      languageVersionInput !== (project.language_version || '')
  }, [project, branchInput, baseDirInput, buildCommandInput, startCommandInput, nodeVersionInput, phpVersionInput, workerCommandInput, queueEnabledInput, languageVersionInput])

  const handleResetSettings = () => {
    if (!project) return
    setBranchInput(project.branch || '')
    setBaseDirInput(project.base_directory || '')
    setBuildCommandInput(project.build_command || '')
    setStartCommandInput(project.start_command || '')
    setNodeVersionInput(project.node_version || '20')
    setPhpVersionInput(project.php_version || '8.2')
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

  const settingsInitialized = useRef(false)

  useEffect(() => {
    if (project && !settingsInitialized.current) {
      setBranchInput(project.branch || '')
      setBaseDirInput(project.base_directory || '')
      setBuildCommandInput(project.build_command || '')
      setStartCommandInput(project.start_command || '')
      setNodeVersionInput(project.node_version || '20')
      setPhpVersionInput(project.php_version || '8.2')
      setWorkerCommandInput(project.worker_command || '')
      setQueueEnabledInput(project.queue_enabled || false)
      setLanguageVersionInput(project.language_version || '')
      settingsInitialized.current = true
    }
  }, [project])

  useEffect(() => {
    if (project && !isSettingsDirty && settingsInitialized.current) {
      setBranchInput(project.branch || '')
      setBaseDirInput(project.base_directory || '')
      setBuildCommandInput(project.build_command || '')
      setStartCommandInput(project.start_command || '')
      setNodeVersionInput(project.node_version || '20')
      setPhpVersionInput(project.php_version || '8.2')
      setWorkerCommandInput(project.worker_command || '')
      setQueueEnabledInput(project.queue_enabled || false)
      setLanguageVersionInput(project.language_version || '')
    }
  }, [project, isSettingsDirty])

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
      {(isDeploying || project.deployment_status === 'failed' || project.status === 'failed') && (
        <Card className={cn(
          "border-blue-500/20 bg-blue-500/5 p-6 mb-6",
          isDeploying && "border-blue-500/30 bg-blue-500/10",
          (project.deployment_status === 'failed' || project.status === 'failed') && "border-rose-500/20 bg-rose-500/5"
        )}>
          <div className="flex flex-col sm:flex-row items-center gap-6 text-center sm:text-left">
            <div className={cn(
              "w-12 h-12 rounded-xl flex items-center justify-center shrink-0",
              isDeploying ? "bg-blue-500/20 text-blue-500" : "bg-rose-500/20 text-rose-500"
            )}>
              {isDeploying ? <Box className="w-6 h-6" /> : <AlertTriangle className="w-6 h-6" />}
            </div>
            <div className="flex-1">
              <h3 className={cn(
                "text-lg font-bold",
                (project.deployment_status === 'failed' || project.status === 'failed') && "text-rose-500"
              )}>
                {isDeploying 
                  ? (deploymentPhase ? `${deploymentPhase.label}...` : t('projectDetail.messages.buildTitle'))
                  : t('projectDetail.overview.deployError')}
              </h3>
              <p className="text-sm text-muted-foreground">
                {isDeploying
                  ? (deploymentPhase?.phase === 'startup' 
                      ? 'Container is starting up and running health checks...'
                      : deploymentPhase?.phase === 'finalize'
                      ? 'Finalizing deployment and cleaning up old versions...'
                      : t('projectDetail.messages.buildDesc'))
                  : t('projectDetail.messages.failedDesc')}
              </p>
              {isDeploying && project.deployment_progress != null && (
                <div className="mt-3 w-full bg-muted/50 rounded-full h-1.5 overflow-hidden">
                  <div 
                    className="h-full bg-blue-500 rounded-full transition-all duration-500 ease-out"
                    style={{ width: `${Math.min(project.deployment_progress, 100)}%` }}
                  />
                </div>
              )}
            </div>
            <div className="flex gap-3">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setActiveTab('build')}
                className={cn(
                  "gap-2 font-bold uppercase tracking-wider text-[10px]",
                  isDeploying ? "border-blue-500/30 hover:bg-blue-500/10" : "border-rose-500/30 hover:bg-rose-500/10"
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
            {!isDeploying && <StatusIndicator status={displayStatus || project.status} />}
            {project.deployment_status && project.deployment_status !== 'completed' && (
              <Badge variant="outline" className={cn(
                "gap-2 py-1 px-3 flex items-center",
                project.deployment_status === 'failed' ? "text-rose-500 bg-rose-500/10 border-rose-500/20" :
                project.deployment_status === 'rollback' ? "text-amber-500 bg-amber-500/10 border-amber-500/20" :
                "text-blue-500 bg-blue-500/10 border-blue-500/20"
              )}>
                <div className={cn(
                  "w-2 h-2 rounded-full",
                  project.deployment_status === 'failed' ? "bg-rose-500" :
                  project.deployment_status === 'rollback' ? "bg-amber-500" :
                  "bg-blue-500 animate-spin"
                )} />
                <span className="text-[10px] uppercase font-bold tracking-wider">
                  {deploymentPhase ? deploymentPhase.label : project.deployment_status} {project.deployment_progress != null ? `(${project.deployment_progress}%)` : ''}
                </span>
              </Badge>
            )}
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
            {project.custom_domains && project.custom_domains.length > 0 && (
              <a 
                href={`https://${project.custom_domains[0].domain}`} 
                target="_blank" 
                rel="noopener noreferrer"
                className="group/domain"
              >
                <Badge variant="outline" className="gap-1.5 bg-primary/10 text-primary border-primary/20 cursor-pointer hover:bg-primary/20 transition-colors">
                  <Globe className="w-3.5 h-3.5" />
                  {project.custom_domains[0].domain}
                  {project.custom_domains.length > 1 && ` (+${project.custom_domains.length - 1})`}
                  <ExternalLink className="w-2.5 h-2.5 opacity-40 group-hover/domain:opacity-100 transition-opacity" />
                </Badge>
              </a>
            )}
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

          <RestartButton
            projectId={uid || ''}
            status={project.status}
            onStarted={() => onActionStarted('restarting')}
            onSuccess={() => {
              isActionPendingRef.current = false
              fetchProject(true)
            }}
          />

          <RedeployButton
            projectId={uid || ''}
            status={project.status}
            deploymentStatus={project.deployment_status}
            onStarted={onDeployStarted}
            onSuccess={() => {
              isActionPendingRef.current = false
              fetchProject(true)
            }}
            onError={() => {
              isActionPendingRef.current = false
              fetchProject(true)
            }}
          />
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
          <TabsTrigger value="runtime">{t('projectDetail.tabs.runtime')}</TabsTrigger>
          <TabsTrigger value="console">{t('projectDetail.tabs.console')}</TabsTrigger>
          <TabsTrigger value="environment">{t('projectDetail.tabs.secrets')}</TabsTrigger>
          <TabsTrigger value="database">{t('projectDetail.tabs.database')}</TabsTrigger>
          <TabsTrigger value="logs">{t('projectDetail.tabs.logs')}</TabsTrigger>
          <TabsTrigger value="build">{t('projectDetail.tabs.build')}</TabsTrigger>
          <TabsTrigger value="domains">{t('projectDetail.tabs.domains')}</TabsTrigger>
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
              <CardContent className="space-y-3">
                {/* Production / Subdomain URL */}
                <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between p-4 bg-muted/50 rounded-xl border gap-4 group hover:border-primary/20 transition-colors">
                  <div className="flex items-center gap-4">
                    <div className="p-2.5 bg-emerald-500/10 rounded-lg text-emerald-600 border border-emerald-500/20">
                      <Globe className="w-5 h-5" />
                    </div>
                    <div>
                      <div className="font-bold text-sm">{t('projectDetail.overview.productionUrl')}</div>
                      <div className="text-[10px] text-muted-foreground font-medium uppercase tracking-wider">{t('projectDetail.overview.webAccess')} · SSL Enabled</div>
                    </div>
                  </div>
                  <a
                    href={projectUrl}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex items-center gap-1.5 text-primary hover:underline text-sm font-mono truncate max-w-xs group/link"
                  >
                    {projectUrl}
                    <ExternalLink className="w-3 h-3 shrink-0 opacity-0 group-hover/link:opacity-100 transition-opacity" />
                  </a>
                </div>

                {/* Custom Domains */}
                {project.custom_domains && project.custom_domains.length > 0 && (
                  <div className="space-y-2">
                    {project.custom_domains
                      .filter((d) => ['active', 'ssl_active', 'dns_verified'].includes(d.status))
                      .map((d) => (
                        <div
                          key={d.id}
                          className="flex flex-col sm:flex-row items-start sm:items-center justify-between p-4 bg-primary/5 rounded-xl border border-primary/15 gap-4 group hover:border-primary/30 hover:bg-primary/10 transition-colors"
                        >
                          <div className="flex items-center gap-4">
                            <div className="p-2.5 bg-primary/10 rounded-lg text-primary border border-primary/20">
                              <Globe className="w-5 h-5" />
                            </div>
                            <div>
                              <div className="flex items-center gap-2">
                                <span className="font-bold text-sm">{d.domain}</span>
                                <Badge
                                  variant="outline"
                                  className="text-[9px] font-bold uppercase tracking-widest px-1.5 py-0 h-4 text-primary border-primary/30 bg-primary/10"
                                >
                                  Custom Domain
                                </Badge>
                              </div>
                              <div className="text-[10px] text-muted-foreground font-medium uppercase tracking-wider mt-0.5">
                                {d.status === 'ssl_active' ? 'SSL Active' : 'Active'} · Verified
                              </div>
                            </div>
                          </div>
                          <a
                            href={`https://${d.domain}`}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="flex items-center gap-1.5 text-primary hover:underline text-sm font-mono truncate max-w-xs group/link"
                          >
                            https://{d.domain}
                            <ExternalLink className="w-3 h-3 shrink-0 opacity-0 group-hover/link:opacity-100 transition-opacity" />
                          </a>
                        </div>
                      ))}

                    {/* Pending domains hint */}
                    {project.custom_domains.filter((d) => !['active', 'ssl_active', 'dns_verified'].includes(d.status)).length > 0 && (
                      <button
                        onClick={() => setActiveTab('domains')}
                        className="w-full flex items-center justify-between px-4 py-2.5 rounded-xl border border-dashed border-amber-500/20 bg-amber-500/5 text-amber-500 hover:bg-amber-500/10 hover:border-amber-500/30 transition-colors cursor-pointer group"
                      >
                        <span className="text-[11px] font-semibold">
                          {project.custom_domains.filter((d) => !['active', 'ssl_active', 'dns_verified'].includes(d.status)).length} domain
                          {project.custom_domains.filter((d) => !['active', 'ssl_active', 'dns_verified'].includes(d.status)).length > 1 ? 's' : ''} pending verification
                        </span>
                        <ExternalLink className="w-3.5 h-3.5 opacity-60 group-hover:opacity-100 transition-opacity" />
                      </button>
                    )}
                  </div>
                )}

                {/* Empty state: invite to add domains */}
                {(!project.custom_domains || project.custom_domains.length === 0) && (
                  <button
                    onClick={() => setActiveTab('domains')}
                    className="w-full flex items-center justify-between px-4 py-2.5 rounded-xl border border-dashed border-muted-foreground/15 bg-muted/20 text-muted-foreground hover:border-primary/20 hover:text-primary hover:bg-primary/5 transition-colors cursor-pointer group"
                  >
                    <span className="text-[11px] font-medium">Add a custom domain</span>
                    <ExternalLink className="w-3.5 h-3.5 opacity-40 group-hover:opacity-80 transition-opacity" />
                  </button>
                )}
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
                  <div className="p-3 rounded-lg bg-muted border flex items-center justify-between">
                    <div>
                      <label className="text-[9px] font-bold text-muted-foreground uppercase tracking-widest mb-1 block">{t('projectDetail.overview.branch')}</label>
                      <div className="flex items-center gap-1.5 font-bold text-xs">
                        <GitBranch className="w-3 h-3 text-primary" />
                        {project.branch || 'main'}
                      </div>
                    </div>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="w-7 h-7 hover:bg-muted-foreground/10 text-muted-foreground hover:text-primary transition-colors"
                      onClick={() => setActiveTab('settings')}
                      title={t('projectDetail.actions.changeBranch') || 'Change Branch'}
                    >
                      <Settings className="w-3.5 h-3.5" />
                    </Button>
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
                {project.last_commit_hash && (
                  <div className="p-3 rounded-lg bg-muted border">
                    <div className="flex items-center justify-between mb-1.5">
                      <label className="text-[9px] font-bold text-muted-foreground uppercase tracking-widest block">Active Commit</label>
                      <button
                        onClick={() => setActiveTab('runtime')}
                        className="text-[10px] font-bold text-primary hover:text-primary/80 hover:underline flex items-center gap-0.5 transition-all group cursor-pointer"
                        title={t('projectDetail.runtime.goToCheckpointsTooltip') || 'Go to Deployment Checkpoints'}
                      >
                        {t('projectDetail.runtime.goToCheckpoints') || 'Go to Checkpoints'}
                        <ArrowUpRight className="w-3 h-3 transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5" />
                      </button>
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                      <button
                        onClick={() => setActiveTab('runtime')}
                        className="group flex items-center gap-1 font-mono font-bold text-[10px] bg-primary/10 hover:bg-primary/20 text-primary px-1.5 py-0.5 rounded border border-primary/20 transition-all hover:scale-105"
                        title={t('projectDetail.runtime.goToCheckpointsTooltip') || 'View in Deployment Checkpoints'}
                      >
                        {project.last_commit_hash.substring(0, 7)}
                        <ArrowUpRight className="w-3 h-3 text-primary/70 group-hover:text-primary transition-colors" />
                      </button>
                      {checkpoints.find(cp => cp.sha === project.last_commit_hash)?.message ? (
                        <span className="text-[11px] text-foreground/80 font-medium truncate max-w-[280px]" title={checkpoints.find(cp => cp.sha === project.last_commit_hash)?.message}>
                          — {checkpoints.find(cp => cp.sha === project.last_commit_hash)?.message}
                        </span>
                      ) : (
                        <span className="text-[10px] text-muted-foreground italic">No commit message found</span>
                      )}
                    </div>
                  </div>
                )}
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
                  onClick={() => { navigator.clipboard.writeText(consoleOutput); toast.success(t('common.copySuccess')) }}
                  className="p-1.5 hover:bg-white/10 rounded-md transition-colors text-zinc-500 hover:text-white cursor-pointer"
                  title="Copy Logs"
                >
                  <Copy className="w-3.5 h-3.5" />
                </button>
                <button
                  onClick={() => {
                    const el = document.getElementById('console-scroll-area');
                    if (el) el.scrollTop = el.scrollHeight;
                  }}
                  className="p-1.5 hover:bg-white/10 rounded-md transition-colors text-zinc-500 hover:text-white cursor-pointer"
                  title="Scroll to Bottom"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="m7 15 5 5 5-5" /><path d="m7 9 5 5 5-5" /></svg>
                </button>
                <div className="w-px h-3 bg-white/10 mx-1" />
                <Button variant="ghost" size="xs" onClick={handleClearConsole} className="text-[10px] uppercase font-bold text-zinc-600 hover:text-rose-400 cursor-pointer">{t('projectDetail.actions.clear')}</Button>
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
                <div key={i} className={cn("flex gap-4 group py-0.5 px-2 rounded -mx-2 hover:bg-white/[0.05] transition-colors", line.startsWith('$') ? "text-primary mt-2 font-bold" : "text-zinc-400")}>
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
          <EnvironmentEditor 
            uid={uid || ''} 
            onSave={() => projectsAPI.redeploy(uid || '').then(() => fetchProject())} 
          />
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
            <DatabaseStudio embedded={true} projectId={uid} />
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
                  onClick={() => { navigator.clipboard.writeText(logs); toast.success(t('common.copySuccess')) }}
                  className="p-1.5 hover:bg-white/10 rounded-md transition-colors text-zinc-500 hover:text-white cursor-pointer"
                  title="Copy Logs"
                >
                  <Copy className="w-3.5 h-3.5" />
                </button>
                <button
                  onClick={() => {
                    const el = document.getElementById('runtime-logs-scroll');
                    if (el) el.scrollTop = el.scrollHeight;
                  }}
                  className="p-1.5 hover:bg-white/10 rounded-md transition-colors text-zinc-500 hover:text-white cursor-pointer"
                  title="Scroll to Bottom"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="m7 15 5 5 5-5" /><path d="m7 9 5 5 5-5" /></svg>
                </button>
                <div className="w-px h-3 bg-white/10 mx-1" />
                <Button variant="ghost" size="xs" onClick={handleClearLogs} className="text-[10px] uppercase font-bold text-zinc-600 hover:text-rose-400 cursor-pointer">{t('projectDetail.actions.clear')}</Button>
                <Button variant="ghost" size="xs" onClick={fetchLogs} className="h-6 w-6 cursor-pointer"><RefreshCw size={12} /></Button>
              </div>
            </CardHeader>
            <div id="runtime-logs-scroll" className="flex-1 p-6 overflow-auto font-mono text-[11px] leading-relaxed custom-scrollbar bg-zinc-950">
              {visibleLogLines.length > 0 ? visibleLogLines.map((line: string, i: number) => {
                const isTimestamp = /^\d{4}-\d{2}-\d{2}/.test(line) || /^\[\d{2}-\w{3}-\d{4}/.test(line)
                return (
                  <div key={i} className="flex gap-4 group py-0.5 px-2 rounded -mx-2 hover:bg-white/[0.05] transition-colors">
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
          {activeTab === 'build' && project && (
            <BuildLogsConsole 
              key={project.deployment_job_id || 'no-job'} 
              projectId={project.uid} 
              status={project.status} 
              project={project} 
              onDeploymentEvent={handleDeploymentEvent} 
            />
          )}
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


                  </div>
                )}


                {/* Common Custom Commands Area */}
                <div className="space-y-4 pt-6 mt-6 border-t border-dashed">
                  <div className="space-y-2">
                    <Label className="text-xs uppercase tracking-widest text-muted-foreground flex items-center gap-2">
                      <Code2 size={14} className="text-primary" />
                      {t('projectDetail.settings.buildCommand')}
                    </Label>
                    <Textarea
                      value={buildCommandInput}
                      onChange={(e) => setBuildCommandInput(e.target.value)}
                      placeholder="e.g. npm install && npm run build"
                      className="min-h-[80px] text-xs font-mono border-muted-foreground/20"
                    />
                    <p className="text-[9px] text-muted-foreground italic">{t('projectDetail.settings.buildCommandDesc')}</p>
                  </div>

                  <div className="space-y-2">
                    <Label className="text-xs uppercase tracking-widest text-muted-foreground flex items-center gap-2">
                      <Play size={14} className="text-primary" />
                      {t('projectDetail.settings.startCommand')}
                    </Label>
                    <Input
                      value={startCommandInput}
                      onChange={(e) => setStartCommandInput(e.target.value)}
                      placeholder={project.framework === 'Laravel' ? 'Leave empty for default PHP-FPM' : 'e.g. node dist/main.js'}
                      className="h-10 text-xs font-mono border-muted-foreground/20"
                    />
                    <p className="text-[9px] text-muted-foreground italic">{t('projectDetail.settings.startCommandDesc')}</p>
                  </div>
                </div>

                <p className="text-[10px] text-muted-foreground italic pl-1 flex items-center gap-1.5 mt-2">
                  <AlertTriangle size={10} className="text-amber-500" /> {t('projectDetail.settings.redeployWarning')}
                </p>
              </CardContent>
            </Card>

            <div className="space-y-6">
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
                <CardContent className="space-y-4">
                  <div className="space-y-2">
                    <Label className="text-[10px] uppercase tracking-widest text-muted-foreground font-semibold">{t('projectDetail.settings.branchTitle')}</Label>
                    <div className="flex gap-2">
                      <Input
                        value={branchInput}
                        onChange={(e) => setBranchInput(e.target.value)}
                        placeholder={t('projectDetail.settings.branchPlaceholder')}
                        className="h-9 max-w-[240px] bg-muted/20 border-muted-foreground/10 focus:border-primary/30 transition-all text-xs"
                      />
                    </div>
                    <p className="text-[9px] text-muted-foreground/60 italic pl-0.5 flex items-center gap-1.5 mt-1">
                      <AlertTriangle size={10} className="text-amber-500/50" /> {t('projectDetail.settings.redeployWarning')}
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
                <CardContent className="space-y-4">
                  <div className="space-y-2">
                    <Label className="text-[10px] uppercase tracking-widest text-muted-foreground font-semibold">{t('newProject.baseDir')}</Label>
                    <div className="flex gap-2">
                      <Input
                        value={baseDirInput}
                        onChange={(e) => setBaseDirInput(e.target.value)}
                        placeholder={t('newProject.baseDirPlaceholder')}
                        className="h-9 max-w-[240px] bg-muted/20 border-muted-foreground/10 focus:border-primary/30 transition-all text-xs"
                      />
                    </div>
                    <p className="text-[9px] text-muted-foreground/60 italic pl-0.5 flex items-center gap-1.5 mt-1">
                      <AlertTriangle size={10} className="text-amber-500/50" /> {t('projectDetail.settings.redeployWarning')}
                    </p>
                  </div>
                </CardContent>
              </Card>
            </div>
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

        <TabsContent value="runtime" className="pt-0">
          {project && (
            <RuntimeTab
              project={project}
              checkpoints={checkpoints}
              isRollingBack={isRollingBack}
              rollbackCommitSHA={rollbackCommitSHA}
              setRollbackCommitSHA={setRollbackCommitSHA}
              triggerRollbackConfirm={triggerRollbackConfirm}
              runtimeEvents={runtimeEvents}
              t={t}
            />
          )}
        </TabsContent>

        <TabsContent value="domains" className="pt-0">
          <Card>
            <CardHeader>
              <div className="flex items-center gap-4">
                <div className="w-10 h-10 bg-primary/10 rounded-lg flex items-center justify-center text-primary">
                  <Globe className="w-5 h-5" />
                </div>
                <div>
                  <CardTitle className="text-lg">{t('projectDetail.settings.customDomain') || 'Custom Domain'}</CardTitle>
                  <CardDescription>{t('projectDetail.settings.customDomainDesc') || 'Manage custom domains for your project'}</CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-6">
              {project && <CustomDomainManager projectId={project.id} subdomain={project.subdomain!} projectUrl={project.url} onDomainsChanged={fetchProject} />}
            </CardContent>
          </Card>
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

export default UserProjectDetail
