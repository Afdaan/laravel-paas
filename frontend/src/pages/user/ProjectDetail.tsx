import { useState, useEffect, useMemo, useRef, useCallback } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { toast } from 'sonner'
import useTranslation from '../../lib/useTranslation'
import {
  RefreshCw,
  ExternalLink,
  Trash2,
  Play,
  Power,
  Box,
  AlertTriangle,
  Loader2,
  MemoryStick,
  Terminal as TerminalIcon,
  Globe,
  Cpu,
  Database as DatabaseIcon
} from 'lucide-react'
import axios, { AxiosError } from 'axios'
import { projectsAPI, githubAPI } from '../../services/api'
import { Project, ProjectStats, DeploymentEvent } from '../../types'
interface GitHubInstallation {
  installation_id: number
  account_name: string
  avatar_url?: string
}

interface GitHubRepo {
  id: number
  name: string
  full_name: string
  html_url: string
}

import ConfirmationModal from '../../components/ConfirmationModal'
import DatabaseStudio from './DatabaseStudio'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { usePolling } from '@/lib/usePolling'
import { cn } from '@/lib/utils'
import { DEFAULT_RUNTIME_VERSIONS } from '@/lib/runtimes'
import { FrameworkIcon } from '../../components/FrameworkIcon'
import { RedeployButton } from '../../components/project/RedeployButton'
import ProjectConsole from '../../components/project/ProjectConsole'
import { RestartButton } from '../../components/project/RestartButton'
import { EnvironmentEditor } from '../../components/project/EnvironmentEditor'
import { RuntimeTab } from '../../components/project/RuntimeTab'
import { OverviewTab } from '../../components/project/detail/OverviewTab'
import { LogsTab } from '../../components/project/detail/LogsTab'
import { BuildTab } from '../../components/project/detail/BuildTab'
import { DomainsTab } from '../../components/project/detail/DomainsTab'
import { SettingsTab } from '../../components/project/detail/SettingsTab'
import { PROJECT_DETAIL_TABS, isProjectDetailTab } from '../../components/project/detail/tabs'


const ESCAPE_CHAR = String.fromCharCode(27)
const ANSI_ESCAPE_PATTERN = new RegExp(`(?:${ESCAPE_CHAR}\\[[0-?]*[ -/]*[@-~]|${ESCAPE_CHAR}[@-_])`, 'g')

function normalizeLogLine(line: string) {
  return line.replace(ANSI_ESCAPE_PATTERN, '').replace(/\r/g, '')
}

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
  const [isDeleting, setIsDeleting] = useState(false)
  const [searchParams, setSearchParams] = useSearchParams()
  const activeTab = searchParams.get('tab') || (() => {
    const hash = window.location.hash.replace('#', '')
    return isProjectDetailTab(hash) ? hash : 'project'
  })()
  const [openedBuildTabProjectUid, setOpenedBuildTabProjectUid] = useState(activeTab === 'build' ? uid : undefined)
  const setActiveTab = useCallback((tab: string) => {
    setSearchParams(prev => {
      prev.set('tab', tab)
      // Explicitly delete known sub-tab state to prevent leakage without destroying global params
      prev.delete('dbTab')
      prev.delete('table')
      return prev
    }, { replace: true })
  }, [setSearchParams])
  useEffect(() => {
    if (activeTab === 'build' && uid) {
      setOpenedBuildTabProjectUid(uid)
    }
  }, [activeTab, uid])
  const [logType, setLogType] = useState<'web' | 'worker'>('web')
  const logsEndRef = useRef<HTMLDivElement>(null)
  const isActionPendingRef = useRef(false)
  const activeProjectUidRef = useRef<string | null>(uid || null)
  const fetchProjectSeqRef = useRef(0)

  const [branchInput, setBranchInput] = useState('')
  const [baseDirInput, setBaseDirInput] = useState('')
  const [buildCommandInput, setBuildCommandInput] = useState('')
  const [startCommandInput, setStartCommandInput] = useState('')
  const [portInput, setPortInput] = useState<number | ''>('')
  const [nodeVersionInput, setNodeVersionInput] = useState('')
  const [phpVersionInput, setPhpVersionInput] = useState('')
  const [workerCommandInput, setWorkerCommandInput] = useState('')
  const [queueEnabledInput, setQueueEnabledInput] = useState(false)
  const [languageVersionInput, setLanguageVersionInput] = useState('')
  const [settingsProjectUid, setSettingsProjectUid] = useState<string | null>(null)
  const settingsProjectUidRef = useRef<string | null>(null)
  const [isSavingSettings, setIsSavingSettings] = useState(false)
  const [branchesList, setBranchesList] = useState<string[]>([])
  const [isFetchingBranches, setIsFetchingBranches] = useState(false)
  const [forceManualInput, setForceManualInput] = useState(false)

  // Git Connection states
  const [githubUrlInput, setGithubUrlInput] = useState('')
  const [githubInstallationIdInput, setGithubInstallationIdInput] = useState<number | null>(null)
  const [githubRepoOwnerInput, setGithubRepoOwnerInput] = useState('')
  const [githubRepoNameInput, setGithubRepoNameInput] = useState('')
  const [gitConnectionMode, setGitConnectionMode] = useState<'manual' | 'github_app'>('manual')

  const [githubInstallations, setGithubInstallations] = useState<GitHubInstallation[]>([])
  const [isGithubInstallationsLoading, setIsGithubInstallationsLoading] = useState(false)
  const [githubRepos, setGithubRepos] = useState<GitHubRepo[]>([])
  const [isGithubReposLoading, setIsGithubReposLoading] = useState(false)

  // Prepend current installation to dropdown items so that it shows as selected even if loading connected installations fails.
  const memoizedGithubInstallations = useMemo<GitHubInstallation[]>(() => {
    if (!project || !project.github_installation_id) {
      return githubInstallations
    }
    const instId = project.github_installation_id
    const alreadyExists = githubInstallations.some(i => i.installation_id === instId)
    if (alreadyExists) {
      return githubInstallations
    }
    const synthesized: GitHubInstallation = {
      installation_id: instId,
      // avatar_url intentionally omitted — this is a degraded-state entry shown when
      // installation listing fails. The Github icon fallback is correct here.
      account_name: project.github_repo_owner || 'Connected Account',
    }
    return [synthesized, ...githubInstallations]
  }, [githubInstallations, project])

  // Prepend current repository to dropdown items so that it remains selected and visible if repository fetching fails.
  const memoizedGithubRepos = useMemo<GitHubRepo[]>(() => {
    if (!project || !project.github_repo_owner || !project.github_repo_name) {
      return githubRepos
    }
    const fullName = `${project.github_repo_owner}/${project.github_repo_name}`
    const alreadyExists = githubRepos.some(r => r.full_name.toLowerCase() === fullName.toLowerCase())
    if (alreadyExists) {
      return githubRepos
    }
    const synthesized: GitHubRepo = {
      id: -1,
      name: project.github_repo_name,
      full_name: fullName,
      html_url: project.github_url || `https://github.com/${fullName}`,
    }
    return [synthesized, ...githubRepos]
  }, [githubRepos, project])

  const [clearedLogsMap, setClearedLogsMap] = useState<Record<string, string>>({})

  const [runtimeEvents, setRuntimeEvents] = useState<DeploymentEvent[]>([])
  const [isRollingBack, setIsRollingBack] = useState(false)
  const [rollbackCommitSHA, setRollbackCommitSHA] = useState('')

  useEffect(() => {
    if (activeTab === 'settings') {
      let cancelled = false
      setIsGithubInstallationsLoading(true)

      githubAPI.listInstallations()
        .then(response => {
          if (cancelled) return
          setGithubInstallations(response.data.data || [])
        })
        .catch((err: AxiosError) => {
          if (cancelled) return
          const status = err?.response?.status
          if (status !== 404 && status !== 403) {
            console.error('Failed to load GitHub installations', err)
          }
        })
        .finally(() => {
          if (cancelled) return
          setIsGithubInstallationsLoading(false)
        })

      return () => {
        cancelled = true
      }
    }
  }, [activeTab])

  useEffect(() => {
    if (activeTab === 'settings' && githubInstallationIdInput) {
      const requestedInstallationId = githubInstallationIdInput
      let cancelled = false

      setIsGithubReposLoading(true)
      githubAPI.listRepositories(requestedInstallationId)
        .then(response => {
          if (cancelled) return

          setGithubRepos(response.data.data || [])
        })
        .catch(err => {
          if (cancelled) return

          const status = err?.response?.status
          if (status !== 404 && status !== 403) {
            console.error('Failed to load GitHub repositories', err)
          }
          setGithubRepos([])
        })
        .finally(() => {
          if (cancelled) return

          setIsGithubReposLoading(false)
        })

      return () => {
        cancelled = true
      }
    }

    setGithubRepos([])
    setIsGithubReposLoading(false)
  }, [activeTab, githubInstallationIdInput])

  useEffect(() => {
    if (activeTab === 'settings' && gitConnectionMode === 'github_app' && githubRepoOwnerInput && githubRepoNameInput) {
      let cancelled = false

      setIsFetchingBranches(true)
      githubAPI.listBranches(githubRepoOwnerInput, githubRepoNameInput, githubInstallationIdInput || undefined)
        .then(res => {
          if (cancelled) return

          const raw = res.data.data || []
          setBranchesList(raw.map((b: string | { name: string }) => typeof b === 'string' ? b : b.name))
        })
        .catch(() => {
          if (cancelled) return

          setBranchesList([])
        })
        .finally(() => {
          if (cancelled) return

          setIsFetchingBranches(false)
        })

      return () => {
        cancelled = true
      }
    }
  }, [activeTab, gitConnectionMode, githubRepoOwnerInput, githubRepoNameInput, githubInstallationIdInput])

  const fetchRuntimeEvents = useCallback(async (signal?: AbortSignal) => {
    if (!uid) return
    try {
      const response = await projectsAPI.getDeploymentEvents(uid, true, { signal })
      setRuntimeEvents(response.data)
    } catch (err) {
      // Ignore abort errors
      if (axios.isCancel(err) || (err as Error).name === 'CanceledError') return;
      setRuntimeEvents([])
    }
  }, [uid])

  useEffect(() => {
    const controller = new AbortController()
    fetchRuntimeEvents(controller.signal)
    return () => controller.abort()
  }, [fetchRuntimeEvents, project?.last_commit_hash])

  useEffect(() => {
    if (activeTab === 'runtime') {
      const controller = new AbortController()
      const interval = setInterval(() => {
        fetchRuntimeEvents(controller.signal)
      }, 5000)
      return () => {
        clearInterval(interval)
        controller.abort()
      }
    }
  }, [activeTab, fetchRuntimeEvents])

  // Support deep linking to tabs (e.g. ?tab=build or #build)
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const tabParam = params.get('tab')
    if (!tabParam) {
      const hash = window.location.hash.replace('#', '')
      if (isProjectDetailTab(hash)) {
        setActiveTab(hash)
      }
    }
  }, [setActiveTab])

  // Fallback database tab to overview if the project does not have a database instance provisioned
  useEffect(() => {
    if (activeTab === 'database' && project && !project.database_instance) {
      setActiveTab('project')
    }
  }, [activeTab, project, setActiveTab])



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
      message: t('projectDetail.runtime.confirmRollbackMsg') || 'Are you sure you want to rollback to this commit? If the image is cached locally, it will perform an instant swap. Otherwise, it will trigger an automated rebuild.',
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
      if (list.length >= 10) return;
      if (evt.event_type === 'deployment_completed' || evt.event_type === 'rollback_completed' || evt.event_type === 'deployment_skipped_existing_image') {
        const sha = evt.payload || evt.message
        // Validate as a real 40-char hex SHA — prevents arbitrary messages from becoming fake checkpoints
        const isValidSha = sha && /^[0-9a-f]{40}$/i.test(sha)
        if (isValidSha && !seen.has(sha)) {
          seen.add(sha)
          const commitMsg = (evt.job_id && commitMsgMap.get(evt.job_id)) || evt.message || t('projectDetail.runtime.deploymentVersion', { version: sha.substring(0, 7) })
          list.push({
            sha,
            time: new Date(evt.created_at).toLocaleString(),
            message: commitMsg
          })
        }
      }
    })
    return list
  }, [runtimeEvents, t])

  const activeCommit = useMemo(() => {
    if (!project?.last_commit_hash) return null

    const activeSha = project.last_commit_hash.trim().toLowerCase()
    const checkpoint = checkpoints.find(cp => {
      const checkpointSha = cp.sha.trim().toLowerCase()
      return (
        checkpointSha === activeSha ||
        checkpointSha.startsWith(activeSha) ||
        activeSha.startsWith(checkpointSha) ||
        checkpointSha.substring(0, 7) === activeSha.substring(0, 7)
      )
    })

    return {
      sha: project.last_commit_hash,
      shortSha: project.last_commit_hash.substring(0, 7),
      message: checkpoint?.message?.trim() || '',
    }
  }, [checkpoints, project?.last_commit_hash])

  const isNodeRelated = ['Node.js', 'Next.js', 'Vite', 'React', 'Vue', 'Nuxt.js', 'Svelte', 'Angular', 'TypeScript'].includes(project?.framework || '')

  const consecutiveErrorsRef = useRef(0)
  const [consecutiveErrors, setConsecutiveErrorsState] = useState(0)
  const setConsecutiveErrors = useCallback((val: number | ((prev: number) => number)) => {
    if (typeof val === 'function') {
      setConsecutiveErrorsState(prev => {
        const next = val(prev)
        consecutiveErrorsRef.current = next
        return next
      })
    } else {
      consecutiveErrorsRef.current = val
      setConsecutiveErrorsState(val)
    }
  }, [])

  // Derivate tabs that should be visible based on active service configurations
  const visibleTabs = useMemo(() => {
    return PROJECT_DETAIL_TABS.filter((tab) => {
      if (tab.value === 'database') {
        return !!project?.database_instance
      }
      return true
    })
  }, [project?.database_instance])

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

  const fetchBranches = useCallback(async (showToast = false) => {
    if (!uid) return
    const requestedUid = uid
    setIsFetchingBranches(true)
    try {
      const response = await projectsAPI.listBranches(requestedUid)
      if (activeProjectUidRef.current !== requestedUid) return

      if (response.data.warning && showToast) {
        toast.warning(response.data.warning)
      }

      const raw = response.data.data || []
      setBranchesList(raw.map((b: string | { name: string }) => typeof b === 'string' ? b : b.name))
      if (showToast) {
        toast.success(t('projectDetail.settings.syncSuccess') || 'Branches synchronized successfully')
      }
    } catch {
      if (activeProjectUidRef.current !== requestedUid) return

      setBranchesList([]) // Fallback to manual text input on failure
      if (showToast) {
        toast.error(t('projectDetail.settings.syncFailed') || 'Failed to synchronize branches')
      }
    } finally {
      if (activeProjectUidRef.current === requestedUid) {
        setIsFetchingBranches(false)
      }
    }
  }, [uid, t])

  const fetchProject = useCallback(async (forceUpdate = false) => {
    if (!uid || isDeleting) return
    const requestedUid = uid
    const currentSeq = ++fetchProjectSeqRef.current
    try {
      const response = await projectsAPI.get(requestedUid)
      if (activeProjectUidRef.current !== requestedUid) return
      if (currentSeq !== fetchProjectSeqRef.current) return

      // Prevent stale polling overwrites during optimistic action phases unless explicitly forced
      if (typeof forceUpdate !== 'boolean') forceUpdate = false // Handle accidental event objects
      if (!forceUpdate && isActionPendingRef.current) return

      setProject(response.data)
      setConsecutiveErrors(0)
    } catch (error: unknown) {
      if (activeProjectUidRef.current !== requestedUid) return
      if (currentSeq !== fetchProjectSeqRef.current) return

      const axiosError = error as AxiosError<{ error: string }>
      if (axiosError.response?.status === 401) {
        navigate('/login')
        return
      }

      if (axiosError.response?.status === 404) {
        if (isDeleting) return
        toast.error(t('projectDetail.messages.notFound') || 'Project not found')
        // Permission checks
        navigate('/projects')
        return
      }

      const nextCount = consecutiveErrorsRef.current + 1
      setConsecutiveErrors(nextCount)

      toast.error(t('common.error'), {
        id: 'project-load-error',
        description: nextCount >= 2 ? t('common.pollingPaused') : undefined
      })
    } finally {
      if (activeProjectUidRef.current === requestedUid && currentSeq === fetchProjectSeqRef.current) {
        setIsLoading(false)
      }
    }
  }, [uid, navigate, t, isDeleting, setConsecutiveErrors])

  const handleDeploymentEvent = useCallback(() => {
    fetchProject(true)
  }, [fetchProject])

  // Poll project details periodically during active deployments/restarts
  useEffect(() => {
    if (!project) return
    const isDeploying = ['queued', 'pending', 'building', 'restarting'].includes(project.status) ||
      Boolean(project.deployment_status && !['completed', 'failed', 'rollback', 'cancelled'].includes(project.deployment_status))
    if (isDeploying) {
      const interval = setInterval(() => {
        fetchProject(true)
      }, 10000)
      return () => clearInterval(interval)
    }
  }, [project, fetchProject])

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

  const lastStatsRef = useRef<string | null>(null)
  const fetchStats = useCallback(async () => {
    if (!uid) return
    try {
      const response = await projectsAPI.stats(uid)
      const next = JSON.stringify(response.data)
      if (next !== lastStatsRef.current) {
        lastStatsRef.current = next
        setStats(response.data)
      }
    } catch (error) {
      if (lastStatsRef.current !== null) {
        lastStatsRef.current = null
        setStats(null)
      }
    }
  }, [uid])



  usePolling(() => {
    if (isDeleting) return
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


  const { visibleLogLines, logOffset } = useMemo(() => {
    if (!logs || activeTab !== 'logs') return { visibleLogLines: [], logOffset: 0 }

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

    const lines = visibleLogs.split('\n').map(normalizeLogLine).filter(l => l.trim() !== '' || l === '')
    const slicedLines = lines.length > 300 ? lines.slice(-300) : lines
    const offset = lines.length > 300 ? lines.length - 300 : 0

    return { visibleLogLines: slicedLines, logOffset: offset }
  }, [logs, clearedLogsMap, logType, activeTab])

  const visibleLogsText = useMemo(() => visibleLogLines.join('\n'), [visibleLogLines])



  const handleClearLogs = useCallback(() => {
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
  }, [t, logType, logs])



  const onActionStarted = (status: Project['status'] = 'queued') => {
    isActionPendingRef.current = true
    setProject(prev => prev ? ({ ...prev, status }) : null)
  }

  const onDeployStarted = () => {
    isActionPendingRef.current = true
    setProject(prev => prev ? ({
      ...prev,
      status: 'building',
      deployment_status: 'queued',
      deployment_progress: 0,
    }) : null)
  }

  const onDeployQueued = ({ jobId }: { jobId: string }) => {
    setProject(prev => prev ? ({
      ...prev,
      deployment_job_id: jobId,
    }) : null)
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


  const applyProjectSettings = useCallback((nextProject: Project) => {
    setBranchInput(nextProject.branch || '')
    setBaseDirInput(nextProject.base_directory || '')
    setBuildCommandInput(nextProject.build_command || '')
    setStartCommandInput(nextProject.start_command || '')
    setPortInput(nextProject.port === null ? '' : nextProject.port)
    setNodeVersionInput(nextProject.node_version || DEFAULT_RUNTIME_VERSIONS.node)
    setPhpVersionInput(nextProject.php_version || DEFAULT_RUNTIME_VERSIONS.php)

    let defaultLangVersion = ''
    if (nextProject.framework === 'Python') defaultLangVersion = DEFAULT_RUNTIME_VERSIONS.python
    else if (nextProject.framework === 'Go') defaultLangVersion = DEFAULT_RUNTIME_VERSIONS.go
    setLanguageVersionInput(nextProject.language_version || defaultLangVersion)

    setWorkerCommandInput(nextProject.worker_command || '')
    setQueueEnabledInput(nextProject.queue_enabled || false)
    setGithubUrlInput(nextProject.github_url || '')
    setGithubInstallationIdInput(nextProject.github_installation_id || null)
    setGithubRepoOwnerInput(nextProject.github_repo_owner || '')
    setGithubRepoNameInput(nextProject.github_repo_name || '')
    setGitConnectionMode(nextProject.github_installation_id ? 'github_app' : 'manual')
    settingsProjectUidRef.current = nextProject.uid
    setSettingsProjectUid(nextProject.uid)
  }, [])

  const isSettingsDirty = useMemo(() => {
    if (!project) return false
    if (settingsProjectUid !== project.uid) return false

    let defaultLangVersion = ''
    if (project.framework === 'Python') defaultLangVersion = DEFAULT_RUNTIME_VERSIONS.python
    else if (project.framework === 'Go') defaultLangVersion = DEFAULT_RUNTIME_VERSIONS.go

    return branchInput !== (project.branch || '') ||
      baseDirInput !== (project.base_directory || '') ||
      buildCommandInput !== (project.build_command || '') ||
      startCommandInput !== (project.start_command || '') ||
      portInput !== (project.port === null ? '' : project.port) ||
      nodeVersionInput !== (project.node_version || DEFAULT_RUNTIME_VERSIONS.node) ||
      phpVersionInput !== (project.php_version || DEFAULT_RUNTIME_VERSIONS.php) ||
      workerCommandInput !== (project.worker_command || '') ||
      queueEnabledInput !== (project.queue_enabled || false) ||
      languageVersionInput !== (project.language_version || defaultLangVersion) ||
      githubUrlInput !== (project.github_url || '') ||
      githubInstallationIdInput !== (project.github_installation_id || null) ||
      githubRepoOwnerInput !== (project.github_repo_owner || '') ||
      githubRepoNameInput !== (project.github_repo_name || '')
  }, [
    project, settingsProjectUid, branchInput, baseDirInput, buildCommandInput, startCommandInput, portInput,
    nodeVersionInput, phpVersionInput, workerCommandInput, queueEnabledInput,
    languageVersionInput, githubUrlInput, githubInstallationIdInput, githubRepoOwnerInput, githubRepoNameInput
  ])

  const handleResetSettings = () => {
    if (!project) return
    applyProjectSettings(project)
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
          const payload: Record<string, unknown> = {
            branch: branchInput,
            base_directory: baseDirInput,
            build_command: buildCommandInput,
            start_command: startCommandInput,
            port: portInput === '' ? null : portInput,
            worker_command: workerCommandInput,
            queue_enabled: queueEnabledInput,
            github_url: githubUrlInput,
            github_installation_id: gitConnectionMode === 'github_app' ? githubInstallationIdInput : null,
            github_repo_owner: gitConnectionMode === 'github_app' ? githubRepoOwnerInput : '',
            github_repo_name: gitConnectionMode === 'github_app' ? githubRepoNameInput : ''
          }

          let defaultLangVersion = ''
          if (project.framework === 'Python') defaultLangVersion = DEFAULT_RUNTIME_VERSIONS.python
          else if (project.framework === 'Go') defaultLangVersion = DEFAULT_RUNTIME_VERSIONS.go

          // Only send runtime versions if they have been explicitly customized from the persisted settings values
          if (nodeVersionInput !== (project.node_version || DEFAULT_RUNTIME_VERSIONS.node)) {
            payload.node_version = nodeVersionInput
          }
          if (phpVersionInput !== (project.php_version || DEFAULT_RUNTIME_VERSIONS.php)) {
            payload.php_version = phpVersionInput
          }
          if (languageVersionInput !== (project.language_version || defaultLangVersion)) {
            payload.language_version = languageVersionInput
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
    activeProjectUidRef.current = uid || null
    settingsProjectUidRef.current = null
    setSettingsProjectUid(null)

    // Clear stale data from previous project immediately so settings never render with
    // another project's branch, commands, Git source, or runtime defaults.
    setProject(null)
    setStats(null)
    lastStatsRef.current = null
    setLogs('')
    setRuntimeEvents([])
    setBranchesList([])
    setGithubRepos([])
    setForceManualInput(false)
    setBranchInput('')
    setBaseDirInput('')
    setBuildCommandInput('')
    setStartCommandInput('')
    setPortInput('')
    setNodeVersionInput(DEFAULT_RUNTIME_VERSIONS.node)
    setPhpVersionInput(DEFAULT_RUNTIME_VERSIONS.php)
    setWorkerCommandInput('')
    setQueueEnabledInput(false)
    setLanguageVersionInput('')
    setGithubUrlInput('')
    setGithubInstallationIdInput(null)
    setGithubRepoOwnerInput('')
    setGithubRepoNameInput('')
    setGitConnectionMode('manual')
    setIsLoading(true)
    fetchProject(true)
    fetchBranches(false)
  }, [uid, fetchProject, fetchBranches])

  useEffect(() => {
    if (project && settingsProjectUidRef.current !== project.uid) {
      applyProjectSettings(project)
      fetchBranches()
    }
  }, [project, applyProjectSettings, fetchBranches])

  useEffect(() => {
    if (project && !isSettingsDirty && settingsProjectUid === project.uid) {
      applyProjectSettings(project)
    }
  }, [project, isSettingsDirty, settingsProjectUid, applyProjectSettings])

  const handleDelete = () => {
    if (!uid) return
    setConfirmModal({
      title: t('projectDetail.messages.deleteConfirm'),
      message: t('projectDetail.messages.deleteDesc'),
      type: 'danger',
      confirmText: t('projectDetail.actions.delete'),
      isOpen: true,
      onConfirm: () => {
        setConfirmModal(prev => ({ ...prev, isOpen: false }))
        setIsDeleting(true)
        toast.promise(
          projectsAPI.delete(uid)
            .then(() => {
              navigate('/projects')
            })
            .catch((err) => {
              setIsDeleting(false)
              throw err
            }),
          {
            loading: t('projectDetail.messages.deleting') || 'Deleting project...',
            success: t('projectDetail.messages.deleteSuccess') || 'Project deleted successfully',
            error: t('projectDetail.messages.deleteFailed') || 'Failed to delete project',
          }
        )
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

      {/* Restarting Banner */}
      {project.status === 'restarting' && (
        <Card className="border-amber-500/20 bg-amber-500/5 p-6 mb-6">
          <div className="flex flex-col sm:flex-row items-center gap-6 text-center sm:text-left">
            <div className="w-12 h-12 rounded-xl flex items-center justify-center shrink-0 bg-amber-500/20 text-amber-500">
              <RefreshCw className="w-6 h-6 animate-spin" />
            </div>
            <div className="flex-1">
              <h3 className="text-lg font-bold">
                {t('status.restarting')}...
              </h3>
              <p className="text-sm text-muted-foreground">
                {t('projectDetail.messages.restartingDesc')}
              </p>
            </div>
          </div>
        </Card>
      )}

      {/* Building Banner */}
      {project.status !== 'restarting' && (isDeploying || project.deployment_status === 'failed' || project.status === 'failed') && (
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
            onQueued={onDeployQueued}
            onSuccess={() => {
              isActionPendingRef.current = false
              fetchProject(true)
            }}
            onError={() => {
              isActionPendingRef.current = false
              fetchProject(true)
            }}
          />
          <Button
            variant="outline"
            size="icon"
            onClick={handleDelete}
            className="text-destructive hover:bg-destructive/10 hover:border-destructive/30 cursor-pointer"
            style={{ cursor: 'pointer' }}
          >
            <Trash2 className="w-4 h-4 cursor-pointer" style={{ cursor: 'pointer' }} />
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
          icon={MemoryStick}
        />
        <MetricCard
          title={t('projectDetail.metrics.framework')}
          value={frameworkLabel}
          subtext={frameworkDetail}
          renderIcon={(className) => <FrameworkIcon framework={project.framework} variant="plain" className={className} />}
        />
        {project.database_instance ? (
          <MetricCard
            title={t('projectDetail.metrics.db')}
            value={project.database_instance.engine === 'postgresql' ? 'PostgreSQL' : 'MySQL'}
            subtext={project.database_instance.name}
            icon={DatabaseIcon}
          />
        ) : (
          <Card className="bg-card/25 border-dashed border-border/60 backdrop-blur-sm overflow-hidden group hover:border-primary/20 hover:shadow-[0_0_20px_rgba(var(--primary),0.02)] transition-all duration-500 relative flex flex-col justify-between p-5 min-h-[120px]">
            <div className="flex items-center justify-between">
              <span className="text-[10px] font-bold uppercase tracking-[0.2em] text-muted-foreground/50">{t('projectDetail.metrics.db')}</span>
              <div className="inline-flex h-9 w-9 items-center justify-center rounded-xl border border-border/30 bg-muted/10">
                <DatabaseIcon className="w-4 h-4 text-muted-foreground/45" />
              </div>
            </div>
            <div className="space-y-1 mt-auto">
              <div className="text-[15px] font-bold tracking-tight text-muted-foreground/80">{t('common.none')}</div>
              <div className="text-[9px] text-muted-foreground/50 font-bold uppercase tracking-wider leading-relaxed">
                {t('projectDetail.metrics.externalDbHint')}
              </div>
            </div>
          </Card>
        )}
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
        <div className="w-full overflow-x-auto pb-2 -mb-2 custom-scrollbar hide-scrollbar-on-mobile">
          <TabsList className="bg-muted/40 p-1.5 rounded-xl border border-border/40 inline-flex min-w-max shadow-sm mb-1">
            {visibleTabs.map((tab) => {
              const Icon = tab.icon
              return (
                <TabsTrigger key={tab.value} value={tab.value} className="flex items-center gap-2 data-active:!text-primary">
                  <Icon className="w-4 h-4" />
                  {t(tab.labelKey)}
                </TabsTrigger>
              )
            })}
          </TabsList>
        </div>

        <TabsContent value="project" className="pt-4">
          <OverviewTab
            project={project}
            projectUrl={projectUrl}
            isLaravelProject={isLaravelProject}
            activeCommit={activeCommit}
            onTabChange={setActiveTab}
          />
        </TabsContent>

        <TabsContent value="console" className="pt-0">
          <ProjectConsole uid={uid || ''} project={project} />
        </TabsContent>

        <TabsContent value="environment" className="pt-0">
          <EnvironmentEditor
            uid={uid || ''}
            onSave={() => fetchProject()}
            hasDatabaseInstance={!!project?.database_instance}
            project={project}
          />
        </TabsContent>

        {!!project.database_instance && (
          <TabsContent value="database" className="pt-0">
            <DatabaseStudio embedded={true} projectId={uid} />
          </TabsContent>
        )}

        <TabsContent value="logs" className="pt-0">
          <LogsTab
            project={project}
            logType={logType}
            setLogType={setLogType}
            visibleLogLines={visibleLogLines}
            visibleLogsText={visibleLogsText}
            logOffset={logOffset}
            logsEndRef={logsEndRef}
            onClearLogs={handleClearLogs}
            onRefreshLogs={fetchLogs}
          />
        </TabsContent>

        <TabsContent value="build" className="pt-0" keepMounted>
          {openedBuildTabProjectUid === uid && project && (
            <BuildTab
              project={project}
              onDeploymentEvent={handleDeploymentEvent}
            />
          )}
        </TabsContent>

        <TabsContent value="settings" className="pt-0">
          {project && (
            <SettingsTab
              project={project}
              frameworkLabel={frameworkLabel}
              isNodeRelated={isNodeRelated}
              phpVersionInput={phpVersionInput}
              setPhpVersionInput={setPhpVersionInput}
              queueEnabledInput={queueEnabledInput}
              setQueueEnabledInput={setQueueEnabledInput}
              nodeVersionInput={nodeVersionInput}
              setNodeVersionInput={setNodeVersionInput}
              languageVersionInput={languageVersionInput}
              setLanguageVersionInput={setLanguageVersionInput}
              workerCommandInput={workerCommandInput}
              setWorkerCommandInput={setWorkerCommandInput}
              buildCommandInput={buildCommandInput}
              setBuildCommandInput={setBuildCommandInput}
              startCommandInput={startCommandInput}
              setStartCommandInput={setStartCommandInput}
              portInput={portInput}
              setPortInput={setPortInput}
              branchesList={branchesList}
              branchInput={branchInput}
              setBranchInput={setBranchInput}
              forceManualInput={forceManualInput}
              setForceManualInput={setForceManualInput}
              isFetchingBranches={isFetchingBranches}
              fetchBranches={fetchBranches}
              baseDirInput={baseDirInput}
              setBaseDirInput={setBaseDirInput}
              gitConnectionMode={gitConnectionMode}
              setGitConnectionMode={setGitConnectionMode}
              isGithubInstallationsLoading={isGithubInstallationsLoading}
              memoizedGithubInstallations={memoizedGithubInstallations}
              githubInstallationIdInput={githubInstallationIdInput}
              setGithubInstallationIdInput={setGithubInstallationIdInput}
              setGithubRepoOwnerInput={setGithubRepoOwnerInput}
              setGithubRepoNameInput={setGithubRepoNameInput}
              setGithubUrlInput={setGithubUrlInput}
              isGithubReposLoading={isGithubReposLoading}
              memoizedGithubRepos={memoizedGithubRepos}
              githubRepoNameInput={githubRepoNameInput}
              githubRepoOwnerInput={githubRepoOwnerInput}
              githubUrlInput={githubUrlInput}
              isSettingsDirty={isSettingsDirty}
              isSavingSettings={isSavingSettings}
              handleResetSettings={handleResetSettings}
              handleSaveSettings={handleSaveSettings}
            />
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
          {project && (
            <DomainsTab
              project={project}
              onDomainsChanged={fetchProject}
              isActive={activeTab === 'domains'}
            />
          )}
        </TabsContent>
      </Tabs>
    </div>
  )
}


export default UserProjectDetail
