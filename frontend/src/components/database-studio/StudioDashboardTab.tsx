import { useState, useEffect, useCallback, useRef } from 'react'
import { toast } from 'sonner'
import {
  TrendingUp,
  Activity,
  ShieldCheck,
  Key,
  RefreshCw,
  Lock,
  Copy,
  EyeOff,
  Eye,
  ArrowRightLeft,
  Terminal,
  ArrowRight
} from 'lucide-react'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { databaseAPI, projectsAPI } from '../../services/api'
import { useStudio } from './StudioContext'
import { DatabaseEngineIcon, getEngineDisplayName } from './utils'
import { Project } from '../../types'

export function StudioDashboardTab() {
  const {
    id,
    dbOverview,
    backups,
    metrics,
    isActionLoading,
    setIsActionLoading,
    loadStudioData,
    triggerConfirmation,
    setActiveTab,
    t
  } = useStudio()

  const pollIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    return () => {
      if (pollIntervalRef.current) {
        clearInterval(pollIntervalRef.current)
      }
    }
  }, [])

  const [credentialsTab, setCredentialsTab] = useState<'env' | 'uri' | 'pdo'>('env')
  const [userProjects, setUserProjects] = useState<Project[]>([])
  const [selectedTargetProject, setSelectedTargetProject] = useState<string>('')
  const [showTransferModal, setShowTransferModal] = useState(false)
  const [chartPoints, setChartPoints] = useState<number[]>([15, 22, 18, 35, 28, 42, 38, 55, 48, 65])
  const [revealPassword, setRevealPassword] = useState(false)

  // Simulate database query/load activity chart points
  useEffect(() => {
    const interval = setInterval(() => {
      setChartPoints(prev => {
        const next = [...prev.slice(1)]
        const newVal = Math.floor(Math.random() * 75) + 10
        next.push(newVal)
        return next
      })
    }, 2500)
    return () => clearInterval(interval)
  }, [])

  const fetchProjectsForTransfer = useCallback(async () => {
    try {
      const res = await projectsAPI.listOwn()
      const data = res.data.data || []
      const filtered = data.filter((p: Project) => p.uid !== id && p.id !== Number(id))
      setUserProjects(filtered)
    } catch (e) {
      console.error(e)
    }
  }, [id])

  useEffect(() => {
    if (showTransferModal) {
      fetchProjectsForTransfer()
    }
  }, [showTransferModal, fetchProjectsForTransfer])

  const handleTransferSubmit = async () => {
    if (!id || !selectedTargetProject) return
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.transfer(id, selectedTargetProject)
      toast.success(res.data.message || t('databaseStudio.dashboard.transferModal.success'))
      setShowTransferModal(false)
      window.location.reload()
    } catch (error) {
      const err = error as { response?: { data?: { error?: string } } }
      toast.error(err.response?.data?.error || t('databaseStudio.dashboard.transferModal.failed'))
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleRotateCredentials = () => {
    if (!id) return
    triggerConfirmation({
      title: t('databaseStudio.dashboard.actions.rotateCredentials'),
      message: t('databaseStudio.dashboard.confirmRotate'),
      type: 'danger',
      confirmText: t('databaseStudio.dashboard.actions.rotateCredentials'),
      onConfirm: async () => {
        if (isActionLoading) return
        setIsActionLoading(true)
        try {
          const res = await databaseAPI.rotateCredentials(id)
          const jobId = res.data.job_id

          if (jobId) {
            toast.info(t('databaseStudio.dashboard.actions.rotatingInProgress') || "Rotation in progress. Zero-downtime container swap is executing...")
            
            let attempts = 0
            // Poll project deployment status until completion
            pollIntervalRef.current = setInterval(async () => {
              attempts++
              if (attempts > 40) { // 40 attempts * 1.5s = 60s max polling timeout
                if (pollIntervalRef.current) {
                  clearInterval(pollIntervalRef.current)
                  pollIntervalRef.current = null
                }
                setIsActionLoading(false)
                toast.warning(t('databaseStudio.dashboard.actions.rotateTimeout') || "Operation is taking longer than expected. Please check the project status panel.")
                loadStudioData()
                return
              }
              try {
                const projRes = await projectsAPI.get(id)
                const project = projRes.data.data
                
                if (project) {
                  const status = project.deployment_status
                  if (status === 'completed' || status === 'failed' || status === 'cancelled') {
                    if (pollIntervalRef.current) {
                      clearInterval(pollIntervalRef.current)
                      pollIntervalRef.current = null
                    }
                    setIsActionLoading(false)
                    if (status === 'completed') {
                      toast.success(t('databaseStudio.dashboard.actions.rotateSuccess') || "Database credentials rotated and container swapped successfully!")
                    } else {
                      toast.error(t('databaseStudio.dashboard.actions.rotateJobFailed') || "Container swap failed. Please check build/deployment logs.")
                    }
                    loadStudioData()
                  }
                }
              } catch (e) {
                if (pollIntervalRef.current) {
                  clearInterval(pollIntervalRef.current)
                  pollIntervalRef.current = null
                }
                setIsActionLoading(false)
                loadStudioData()
              }
            }, 1500)
          } else {
            toast.success(res.data.message)
            setIsActionLoading(false)
            loadStudioData()
          }
        } catch (error) {
          const err = error as { response?: { data?: { error?: string } } }
          toast.error(err.response?.data?.error || t('databaseStudio.errors.rotateFailed'))
          setIsActionLoading(false)
        }
      }
    })
  }

  const handleRestartPool = async () => {
    if (!id) return
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.restartDatabase(id)
      toast.success(res.data.message)
    } catch (error) {
      const err = error as { response?: { data?: { error?: string } }; message?: string }
      toast.error(t('databaseStudio.errors.testConnectionFailed') + ': ' + (err.response?.data?.error || err.message))
    } finally {
      setIsActionLoading(false)
    }
  }

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
    toast.success(t('common.copySuccess'))
  }

  const instanceStatus = dbOverview?.status || 'active'
  const isSuspended = instanceStatus === 'suspended'

  return (
    <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
      {/* Left / Main Overview */}
      <div className="lg:col-span-2 space-y-8">
        {/* Metric Grid */}
        <div className="grid grid-cols-2 md:grid-cols-3 gap-5">
          <Card className="p-5 flex flex-col gap-2 hover:border-primary/20 transition-all duration-300">
            <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.metrics.engine')}</span>
            <span className="text-sm font-bold text-foreground flex items-center gap-2 h-6">
              <DatabaseEngineIcon engine={dbOverview?.engine} />
              {getEngineDisplayName(dbOverview?.engine)}
            </span>
          </Card>

          <Card className="p-5 flex flex-col gap-2 hover:border-primary/20 transition-all duration-300">
            <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.metrics.version')}</span>
            <span className="text-xs font-mono font-bold text-foreground truncate flex items-center h-6" title={dbOverview?.version}>
              {dbOverview?.version || 'Unknown'}
            </span>
          </Card>

          <Card className="p-5 flex flex-col gap-2 hover:border-primary/20 transition-all duration-300">
            <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.metrics.diskUsed')}</span>
            <span className="text-sm font-mono font-bold text-foreground flex items-center gap-2 h-6">
              <Activity className="w-4 h-4 text-primary shrink-0" />
              {dbOverview?.size || '0 KB'}
            </span>
          </Card>

          <Card className="p-5 flex flex-col gap-2 hover:border-primary/20 transition-all duration-300">
            <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.metrics.tables')}</span>
            <span className="text-sm font-mono font-bold text-foreground flex items-center gap-2 h-6">
              <Activity className="w-4 h-4 text-primary shrink-0" />
              {dbOverview?.table_count || 0}
            </span>
          </Card>

          <Card className="p-5 flex flex-col gap-2 hover:border-primary/20 transition-all duration-300">
            <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.activity.totalRecords')}</span>
            <span className="text-sm font-mono font-bold text-foreground flex items-center gap-2 h-6">
              <Activity className="w-4 h-4 text-primary shrink-0" />
              {dbOverview?.row_count != null ? dbOverview.row_count.toLocaleString() : 0}
            </span>
          </Card>

          <Card className="p-5 flex flex-col gap-2 hover:border-primary/20 transition-all duration-300 cursor-pointer" onClick={() => setActiveTab('backups')}>
            <span className="text-[10px] uppercase font-bold tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.activity.backupSnapshots')}</span>
            <span className="text-sm font-mono font-bold text-foreground flex items-center gap-2 h-6 hover:text-primary transition-colors">
              <RefreshCw className="w-4 h-4 text-primary shrink-0" />
              {backups.length} {backups.length === 1 ? t('databaseStudio.dashboard.activity.snapshotOne') : t('databaseStudio.dashboard.activity.snapshotMany')}
            </span>
          </Card>
        </div>

        {/* Real-time DB Utilization & Health Chart */}
        <Card className="p-6">
          <div className="flex items-center justify-between mb-5">
            <div className="space-y-1">
              <h3 className="font-extrabold text-sm uppercase tracking-wider text-muted-foreground flex items-center gap-2">
                <TrendingUp className="w-4 h-4 text-primary" />
                {t('databaseStudio.dashboard.activity.title')}
              </h3>
              <p className="text-xs text-muted-foreground/80">{t('databaseStudio.dashboard.activity.desc')}</p>
            </div>
            
            <div className="flex items-center gap-1.5 bg-emerald-500/10 border border-emerald-500/20 px-2.5 py-0.5 rounded-full">
              <span className="relative flex h-1.5 w-1.5">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-1.5 w-1.5 bg-emerald-500"></span>
              </span>
              <span className="text-[9px] font-extrabold text-emerald-600 uppercase tracking-wider">
                {t('databaseStudio.dashboard.metrics.realtime') || 'Real-time'}
              </span>
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-3 gap-6 items-center">
            {/* SVG Live Utilization Wave */}
            <div className="md:col-span-2 relative h-40 bg-muted/10 dark:bg-muted/5 rounded-xl border border-border/40 overflow-hidden flex items-end">
              <svg className="w-full h-full" viewBox="0 0 500 150" preserveAspectRatio="none">
                <defs>
                  <linearGradient id="chartGrad" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor="var(--primary)" stopOpacity="0.25" />
                    <stop offset="100%" stopColor="var(--primary)" stopOpacity="0.00" />
                  </linearGradient>
                </defs>
                {/* Area path */}
                <path
                  d={`M 0,150 ` + chartPoints.map((val, idx) => `L ${(idx / (chartPoints.length - 1)) * 500},${150 - (val / 100) * 150}`).join(' ') + ` L 500,150 Z`}
                  fill="url(#chartGrad)"
                  className="transition-all duration-1000 ease-in-out"
                />
                {/* Line path */}
                <path
                  d={chartPoints.map((val, idx) => (idx === 0 ? 'M' : 'L') + ` ${(idx / (chartPoints.length - 1)) * 500},${150 - (val / 100) * 150}`).join(' ')}
                  fill="none"
                  stroke="var(--primary)"
                  strokeWidth="2.5"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  className="transition-all duration-1000 ease-in-out"
                />
              </svg>
              <div className="absolute top-3 left-4 text-[10px] font-mono font-bold text-muted-foreground/60">
                {t('databaseStudio.dashboard.activity.throughput')}
              </div>
            </div>

            {/* DB Health Summary / Details */}
            <div className="space-y-4 text-xs">
              <div className="border border-border/60 rounded-xl p-3.5 bg-muted/5 space-y-3">
                <h4 className="font-extrabold text-[10px] text-muted-foreground uppercase tracking-wider flex items-center gap-1.5">
                  <ShieldCheck className="w-3.5 h-3.5 text-emerald-500" />
                  {t('databaseStudio.dashboard.sre.title')}
                </h4>
                <div className="space-y-1.5 font-medium text-muted-foreground/90">
                  <div className="flex justify-between">
                    <span>{t('databaseStudio.dashboard.credentials.hostInternal')}</span>
                    <span className="font-mono text-[10px] font-bold text-foreground truncate max-w-[100px]" title={dbOverview?.host}>{dbOverview?.host || 'localhost'}</span>
                  </div>
                  <div className="flex justify-between">
                    <span>{t('databaseStudio.dashboard.credentials.portLabel')}</span>
                    <span className="font-mono text-foreground font-bold">{dbOverview?.port}</span>
                  </div>
                  <div className="flex justify-between">
                    <span>{t('databaseStudio.dashboard.credentials.dosLimit')}</span>
                    <span className="font-bold text-foreground">{t('databaseStudio.dashboard.credentials.threads')}</span>
                  </div>
                  <div className="flex justify-between">
                    <span>{t('databaseStudio.dashboard.credentials.timeoutLabel')}</span>
                    <span className="font-bold text-foreground">{t('databaseStudio.dashboard.credentials.timeoutSre')}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </Card>

        {/* Connection Credentials Workspace */}
        <Card className="p-6 space-y-6">
          <div className="flex items-center justify-between border-b border-border/40 pb-4">
            <div className="space-y-1">
              <h3 className="font-extrabold text-base flex items-center gap-2">
                <Key className="w-5 h-5 text-primary" />
                {t('databaseStudio.dashboard.credentials.title')}
              </h3>
              <p className="text-xs text-muted-foreground">{t('databaseStudio.dashboard.credentials.subtitle')}</p>
            </div>
            <Button
              variant="outline"
              size="xs"
              onClick={handleRotateCredentials}
              disabled={isActionLoading || isSuspended}
              className="font-bold border-primary/20 hover:border-primary shrink-0 gap-1.5 h-8 text-xs cursor-pointer"
              style={{ cursor: 'pointer' }}
            >
              <RefreshCw className={cn("w-3.5 h-3.5", isActionLoading && "animate-spin")} />
              {t('databaseStudio.dashboard.actions.rotateCredentials')}
            </Button>
          </div>

          {/* Private isolated lock warning banner */}
          <div className="bg-primary/5 border border-primary/20 rounded-xl p-3.5 flex items-start gap-3 text-xs text-muted-foreground leading-normal">
            <Lock className="w-4 h-4 text-primary shrink-0 mt-0.5 animate-pulse" />
            <div className="space-y-0.5">
              <span className="font-bold text-foreground">{t('databaseStudio.dashboard.credentials.privateAccessTitle')}</span>
              <p className="text-muted-foreground/80">{t('databaseStudio.dashboard.credentials.privateAccessDesc')}</p>
            </div>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-6 items-stretch">
            {/* Left: Raw credentials fields */}
            <div className="space-y-4">
              <div className="space-y-1.5">
                <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.credentials.host')}</span>
                <div className="flex items-center justify-between p-3 rounded-xl border bg-muted/10 font-mono text-xs">
                  <span className="truncate text-foreground/90 font-semibold">{dbOverview?.host || 'localhost'}</span>
                  <button onClick={() => copyToClipboard(dbOverview?.host || '')} className="text-muted-foreground hover:text-foreground cursor-pointer" style={{ cursor: 'pointer' }}>
                    <Copy className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>

              <div className="space-y-1.5">
                <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.credentials.port')}</span>
                <div className="flex items-center justify-between p-3 rounded-xl border bg-muted/10 font-mono text-xs">
                  <span className="text-foreground/90 font-semibold">{dbOverview?.port || 3306}</span>
                  <button onClick={() => copyToClipboard(String(dbOverview?.port || 3306))} className="text-muted-foreground hover:text-foreground cursor-pointer" style={{ cursor: 'pointer' }}>
                    <Copy className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>

              <div className="space-y-1.5">
                <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.credentials.databaseName')}</span>
                <div className="flex items-center justify-between p-3 rounded-xl border bg-muted/10 font-mono text-xs">
                  <span className="truncate text-foreground/90 font-semibold">{dbOverview?.database || ''}</span>
                  <button onClick={() => copyToClipboard(dbOverview?.database || '')} className="text-muted-foreground hover:text-foreground cursor-pointer" style={{ cursor: 'pointer' }}>
                    <Copy className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>

              <div className="space-y-1.5">
                <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.credentials.username')}</span>
                <div className="flex items-center justify-between p-3 rounded-xl border bg-muted/10 font-mono text-xs">
                  <span className="truncate text-foreground/90 font-semibold">{dbOverview?.username || ''}</span>
                  <button onClick={() => copyToClipboard(dbOverview?.username || '')} className="text-muted-foreground hover:text-foreground cursor-pointer" style={{ cursor: 'pointer' }}>
                    <Copy className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>

              <div className="space-y-1.5">
                <span className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">{t('databaseStudio.dashboard.credentials.password')}</span>
                <div className="flex items-center justify-between p-3 rounded-xl border bg-muted/10 font-mono text-xs">
                  <input 
                    type={revealPassword ? "text" : "password"} 
                    value={dbOverview?.password || ''} 
                    readOnly 
                    className="bg-transparent border-none outline-none focus:ring-0 flex-1 min-w-0 pr-4 font-mono text-xs text-foreground/90 font-semibold"
                  />
                  <div className="flex items-center gap-3 shrink-0">
                    <button onClick={() => setRevealPassword(!revealPassword)} className="text-muted-foreground hover:text-foreground cursor-pointer" style={{ cursor: 'pointer' }}>
                      {revealPassword ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                    </button>
                    <button onClick={() => copyToClipboard(dbOverview?.password || '')} className="text-muted-foreground hover:text-foreground cursor-pointer" style={{ cursor: 'pointer' }}>
                      <Copy className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
              </div>
            </div>

            {/* Right: Tabbed connection string generator */}
            <div className="flex flex-col border border-border/60 rounded-xl bg-muted/5 overflow-hidden">
              <div className="flex border-b border-border/40 p-1 bg-muted/20">
                {(['env', 'uri', 'pdo'] as const).map(tab => (
                  <button
                    key={tab}
                    onClick={() => setCredentialsTab(tab)}
                    className={cn(
                      "flex-1 py-1.5 text-[10px] font-bold uppercase tracking-wider rounded-lg transition-all cursor-pointer",
                      credentialsTab === tab 
                        ? "bg-background text-primary shadow-sm border border-border/30" 
                        : "text-muted-foreground hover:text-foreground"
                    )}
                    style={{ cursor: 'pointer' }}
                  >
                    {tab === 'env' ? t('databaseStudio.dashboard.credentials.tabEnv') : tab === 'uri' ? t('databaseStudio.dashboard.credentials.tabUri') : t('databaseStudio.dashboard.credentials.tabPdo')}
                  </button>
                ))}
              </div>

              <div className="flex-1 p-4 flex flex-col justify-between min-h-[220px]">
                <div className="flex-1 font-mono text-xs whitespace-pre-wrap select-all overflow-y-auto max-h-[200px] text-foreground/80 leading-relaxed scrollbar-thin">
                  {credentialsTab === 'env' && (
                    `DB_CONNECTION=${(dbOverview?.engine || 'mysql').toLowerCase().includes('post') ? 'pgsql' : 'mysql'}
DB_HOST=${dbOverview?.host || 'localhost'}
DB_PORT=${dbOverview?.port || ((dbOverview?.engine || 'mysql').toLowerCase().includes('post') ? 5432 : 3306)}
DB_DATABASE=${dbOverview?.database || ''}
DB_USERNAME=${dbOverview?.username || ''}
DB_PASSWORD=${revealPassword ? (dbOverview?.password || '') : '••••••••••••••••'}`
                  )}
                  {credentialsTab === 'uri' && (
                    `${(dbOverview?.engine || 'mysql').toLowerCase().includes('post') ? 'postgresql' : 'mysql'}://${dbOverview?.username || ''}:${revealPassword ? (dbOverview?.password || '') : '••••••••'}@${dbOverview?.host || 'localhost'}:${dbOverview?.port || ((dbOverview?.engine || 'mysql').toLowerCase().includes('post') ? 5432 : 3306)}/${dbOverview?.database || ''}`
                  )}
                  {credentialsTab === 'pdo' && (
                    `$pdo = new PDO(
  "${(dbOverview?.engine || 'mysql').toLowerCase().includes('post') ? 'pgsql' : 'mysql'}:host=${dbOverview?.host || 'localhost'};port=${dbOverview?.port || ((dbOverview?.engine || 'mysql').toLowerCase().includes('post') ? 5432 : 3306)};dbname=${dbOverview?.database || ''}",
  "${dbOverview?.username || ''}",
  "${revealPassword ? (dbOverview?.password || '') : '••••••••'}"
);`
                  )}
                </div>

                <Button
                  variant="outline"
                  size="xs"
                  onClick={() => {
                    let text = ''
                    if (credentialsTab === 'env') {
                      text = `DB_CONNECTION=${(dbOverview?.engine || 'mysql').toLowerCase().includes('post') ? 'pgsql' : 'mysql'}\nDB_HOST=${dbOverview?.host || 'localhost'}\nDB_PORT=${dbOverview?.port || ((dbOverview?.engine || 'mysql').toLowerCase().includes('post') ? 5432 : 3306)}\nDB_DATABASE=${dbOverview?.database || ''}\nDB_USERNAME=${dbOverview?.username || ''}\nDB_PASSWORD=${dbOverview?.password || ''}`
                    } else if (credentialsTab === 'uri') {
                      text = `${(dbOverview?.engine || 'mysql').toLowerCase().includes('post') ? 'postgresql' : 'mysql'}://${dbOverview?.username || ''}:${dbOverview?.password || ''}@${dbOverview?.host || 'localhost'}:${dbOverview?.port || ((dbOverview?.engine || 'mysql').toLowerCase().includes('post') ? 5432 : 3306)}/${dbOverview?.database || ''}`
                    } else {
                      text = `$pdo = new PDO(\n  "${(dbOverview?.engine || 'mysql').toLowerCase().includes('post') ? 'pgsql' : 'mysql'}:host=${dbOverview?.host || 'localhost'};port=${dbOverview?.port || ((dbOverview?.engine || 'mysql').toLowerCase().includes('post') ? 5432 : 3306)};dbname=${dbOverview?.database || ''}",\n  "${dbOverview?.username || ''}",\n  "${dbOverview?.password || ''}"\n);`
                    }
                    copyToClipboard(text)
                  }}
                  className="mt-4 font-bold border-primary/20 hover:border-primary shrink-0 gap-1.5 h-8 text-xs cursor-pointer w-full"
                  style={{ cursor: 'pointer' }}
                >
                  <Copy className="w-3.5 h-3.5" />
                  {t('databaseStudio.dashboard.credentials.copyConfig')}
                </Button>
              </div>
            </div>
          </div>
        </Card>
      </div>

      {/* Right Sidebar - Admin Controls & Telemetry */}
      <div className="space-y-8">
        <Card className="p-5 space-y-5">
          <h4 className="font-extrabold text-sm uppercase tracking-wide border-b pb-2 flex items-center gap-2">
            <Activity className="w-4.5 h-4.5" />
            {t('databaseStudio.dashboard.activity.panelTitle')}
          </h4>
          
          <div className="space-y-4">
            {/* Status Indicator */}
            <div className="flex justify-between items-center text-xs">
              <span className="font-bold text-muted-foreground uppercase tracking-wider">{t('databaseStudio.dashboard.metrics.status')}:</span>
              <span className={cn(
                "px-2.5 py-0.5 rounded-full text-[10px] font-extrabold uppercase tracking-wide border",
                isSuspended 
                  ? "bg-destructive/10 text-destructive border-destructive/20 animate-pulse" 
                  : "bg-emerald-500/10 text-emerald-500 border-emerald-500/20"
              )}>
                {instanceStatus}
              </span>
            </div>

            {metrics && (() => {
              const connectionRatio = metrics.active_connections / 15;
              const connectionColor = connectionRatio > 0.8 
                ? 'bg-destructive' 
                : connectionRatio > 0.6 
                  ? 'bg-amber-500' 
                  : 'bg-primary';

              const storageRatio = metrics.size_kb / 1048576;
              const storageColor = storageRatio > 0.8 
                ? 'bg-destructive' 
                : storageRatio > 0.6 
                  ? 'bg-amber-500' 
                  : 'bg-primary';

              return (
                <div className="space-y-4 border-t pt-4">
                  {/* Active Connections */}
                  <div className="space-y-2">
                    <div className="flex justify-between items-center text-xs">
                      <span className="font-bold text-muted-foreground uppercase tracking-wider">{t('databaseStudio.dashboard.activity.connections')}</span>
                      <span className="font-mono text-xs font-bold text-foreground">
                        {metrics.active_connections} <span className="text-muted-foreground/60">/ 15</span>
                      </span>
                    </div>
                    <div className="h-1.5 w-full bg-muted rounded-full overflow-hidden">
                      <div 
                         className={cn("h-full transition-all duration-500", connectionColor)} 
                         style={{ width: `${Math.min(connectionRatio * 100, 100)}%` }}
                      />
                    </div>
                  </div>

                  {/* Storage Usage */}
                  <div className="space-y-2">
                    <div className="flex justify-between items-center text-xs">
                      <span className="font-bold text-muted-foreground uppercase tracking-wider">{t('databaseStudio.dashboard.activity.storageUsage')}</span>
                      <span className="font-mono text-xs font-bold text-foreground">
                        {dbOverview?.size || '0 KB'} <span className="text-muted-foreground/60">/ 1 GB</span>
                      </span>
                    </div>
                    <div className="h-1.5 w-full bg-muted rounded-full overflow-hidden">
                      <div 
                         className={cn("h-full transition-all duration-500", storageColor)} 
                         style={{ width: `${Math.min(storageRatio * 100, 100)}%` }}
                      />
                    </div>
                  </div>
                </div>
              );
            })()}

            {/* Operations Buttons */}
            <div className="space-y-2 border-t pt-4">
              <Button
                variant="outline"
                className="w-full text-xs font-bold gap-2 hover:bg-muted cursor-pointer"
                style={{ cursor: 'pointer' }}
                onClick={handleRestartPool}
                disabled={isActionLoading || isSuspended}
              >
                <RefreshCw className="w-3.5 h-3.5" />
                {t('databaseStudio.dashboard.actions.testConnection')}
              </Button>

              <Button
                variant="outline"
                className="w-full text-xs font-bold gap-2 hover:bg-muted text-foreground border border-border cursor-pointer"
                style={{ cursor: 'pointer' }}
                onClick={() => setShowTransferModal(true)}
                disabled={isActionLoading || isSuspended}
              >
                <ArrowRightLeft className="w-3.5 h-3.5 text-primary" />
                {t('databaseStudio.dashboard.activity.btnTransfer')}
              </Button>
            </div>
          </div>
        </Card>

        {/* Quick Actions Panel */}
        <Card className="p-5 space-y-4">
          <h4 className="font-extrabold text-sm uppercase tracking-wide border-b pb-2 flex items-center gap-2">
            <Terminal className="w-4 h-4" />
            {t('databaseStudio.dashboard.activity.toolsTitle')}
          </h4>
          <div className="grid grid-cols-1 gap-2.5">
            <button
              onClick={() => setActiveTab('query')}
              disabled={isSuspended}
              className="group w-full p-3 rounded-lg border border-border/80 hover:bg-muted/40 text-left transition-all text-xs font-semibold text-muted-foreground hover:text-foreground flex items-center justify-between cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
              style={{ cursor: 'pointer' }}
            >
              <span>{t('databaseStudio.dashboard.activity.sqlEditor')}</span>
              <ArrowRight className="w-3.5 h-3.5 text-primary group-hover:translate-x-0.5 transition-transform" />
            </button>

            <button
              onClick={() => setActiveTab('tables')}
              disabled={isSuspended}
              className="group w-full p-3 rounded-lg border border-border/80 hover:bg-muted/40 text-left transition-all text-xs font-semibold text-muted-foreground hover:text-foreground flex items-center justify-between cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
              style={{ cursor: 'pointer' }}
            >
              <span>{t('databaseStudio.dashboard.activity.tableExplorer')}</span>
              <ArrowRight className="w-3.5 h-3.5 text-primary group-hover:translate-x-0.5 transition-transform" />
            </button>

            <button
              onClick={() => setActiveTab('structure')}
              disabled={isSuspended}
              className="group w-full p-3 rounded-lg border border-border/80 hover:bg-muted/40 text-left transition-all text-xs font-semibold text-muted-foreground hover:text-foreground flex items-center justify-between cursor-pointer disabled:opacity-50 disabled:cursor-not-allowed"
              style={{ cursor: 'pointer' }}
            >
              <span>{t('databaseStudio.dashboard.activity.schemaArchitect')}</span>
              <ArrowRight className="w-3.5 h-3.5 text-primary group-hover:translate-x-0.5 transition-transform" />
            </button>
          </div>
        </Card>
      </div>

      {/* Transfer Database Modal */}
      {showTransferModal && (
        <Dialog open={showTransferModal} onOpenChange={(open: boolean) => !open && setShowTransferModal(false)}>
          <DialogContent className="sm:max-w-md bg-card border border-border/80 rounded-xl shadow-2xl">
            <DialogHeader className="pb-2 border-b border-border/40">
              <DialogTitle className="text-lg font-extrabold flex items-center gap-2 text-foreground/90">
                <ArrowRightLeft className="w-5 h-5 text-primary" />
                {t('databaseStudio.dashboard.transferModal.title')}
              </DialogTitle>
              <DialogDescription className="text-xs text-muted-foreground">
                {t('databaseStudio.dashboard.transferModal.desc')}
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-4 pt-3">
              <div className="space-y-1.5">
                <Label className="text-[10px] font-bold uppercase tracking-wider text-muted-foreground">
                  {t('databaseStudio.dashboard.transferModal.labelProject')}
                </Label>
                <Select
                  value={selectedTargetProject}
                  onValueChange={(val) => setSelectedTargetProject(val || '')}
                >
                  <SelectTrigger className="w-full h-10 px-3 rounded-xl border border-border/70 bg-background/50 text-xs font-semibold text-left justify-between">
                    <SelectValue placeholder={t('databaseStudio.dashboard.transferModal.placeholderSelect')}>
                      {(value) => {
                        if (!value) return null
                        const selectedProject = userProjects.find(p => p.uid === value)
                        return selectedProject ? selectedProject.name : value
                      }}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent align="start" alignItemWithTrigger={false} className="min-w-[var(--radix-select-trigger-width)] p-1 bg-popover/98 backdrop-blur-lg border border-border/80 rounded-xl shadow-2xl max-h-72">
                    {userProjects.length === 0 ? (
                      <div className="py-2 px-3 text-xs text-muted-foreground text-center">
                        {t('databaseStudio.dashboard.transferModal.noProjects')}
                      </div>
                    ) : (
                      userProjects.map(proj => (
                        <SelectItem key={proj.uid} value={proj.uid} className="py-2.5 pl-3 cursor-pointer">
                          <div className="flex min-w-0 flex-col gap-0.5 text-left">
                            <span className="truncate text-sm font-medium leading-none">{proj.name}</span>
                            <span className="truncate text-[10px] leading-none text-muted-foreground">{proj.subdomain}</span>
                          </div>
                        </SelectItem>
                      ))
                    )}
                  </SelectContent>
                </Select>
              </div>

              <div className="flex gap-2.5 pt-2 border-t border-border/40">
                <Button 
                  onClick={handleTransferSubmit} 
                  disabled={isActionLoading || !selectedTargetProject} 
                  className="font-bold flex-1 rounded-xl cursor-pointer"
                  style={{ cursor: 'pointer' }}
                >
                  {isActionLoading ? t('common.executing') : t('databaseStudio.dashboard.transferModal.btnSubmit')}
                </Button>
                <Button 
                  type="button" 
                  onClick={() => setShowTransferModal(false)} 
                  variant="outline" 
                  className="font-bold flex-1 rounded-xl cursor-pointer"
                  style={{ cursor: 'pointer' }}
                >
                  {t('common.cancel')}
                </Button>
              </div>
            </div>
          </DialogContent>
        </Dialog>
      )}
    </div>
  )
}
