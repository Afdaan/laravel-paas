import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { Link } from 'react-router-dom'
import { projectsAPI } from '../../services/api'
import useAuthStore from '../../stores/authStore'
import useTranslation from '../../lib/useTranslation'
import { toast } from 'sonner'
import {
  Rocket,
  CheckCircle2,
  Package,
  ExternalLink,
  Plus,
  Clock,
  AlertCircle,
  PauseCircle,
  Zap,
  Layout,
  ChevronRight,
  Loader2
} from 'lucide-react'
import { usePolling } from '../../lib/usePolling'
import { buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import { FrameworkIcon } from '@/components/FrameworkIcon'

interface ProjectData {
  id: number;
  uid: string;
  name: string;
  status: string;
  subdomain: string;
  url: string;
  framework: string;
  database_name?: string;
  created_at: string;
}

// Status badge component
function StatusBadge({ status }: { status: string }) {
  const { t } = useTranslation()
  const configs: Record<string, { color: string, icon: React.ElementType, label: string, pulse?: boolean }> = {
    pending: { color: 'text-amber-600 bg-amber-500/10 border-amber-500/20', icon: Clock, label: t('status.pending') },
    building: { color: 'text-blue-600 bg-blue-500/10 border-blue-500/20', icon: Loader2, label: t('status.building'), pulse: true },
    running: { color: 'text-emerald-600 bg-emerald-500/10 border-emerald-500/20', icon: CheckCircle2, label: t('status.running') },
    failed: { color: 'text-rose-600 bg-rose-500/10 border-rose-500/20', icon: AlertCircle, label: t('status.failed') },
    stopped: { color: 'text-slate-600 bg-slate-500/10 border-slate-500/20 dark:text-slate-400', icon: PauseCircle, label: t('status.stopped') },
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

function UserDashboard() {
  const { t } = useTranslation()
  const { user } = useAuthStore()
  const [projects, setProjects] = useState<ProjectData[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const isFirstLoad = useRef(true)

  const fetchDashboardData = useCallback(async () => {
    if (isFirstLoad.current) {
      setIsLoading(true)
    }
    try {
      const response = await projectsAPI.listOwn()
      setProjects(response.data.data || [])
    } catch (error) {
      toast.error(t('common.loadError'), { id: 'dashboard-load-error' })
    } finally {
      setIsLoading(false)
      isFirstLoad.current = false
    }
  }, [t])

  useEffect(() => {
    fetchDashboardData()
  }, [fetchDashboardData])

  // Poll for updates every 10 seconds
  usePolling(fetchDashboardData, 10000)

  const runningProjects = projects.filter(p => p.status === 'running').length
  const totalProjects = projects?.length || 0

  // Top framework breakdown for the stat strip — gives the row real signal instead of two bare counts
  const frameworkBreakdown = useMemo(() => {
    const counts = new Map<string, number>()
    for (const p of projects) {
      const fw = p.framework || t('common.general')
      counts.set(fw, (counts.get(fw) || 0) + 1)
    }
    return [...counts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 3)
  }, [projects, t])

  return (
    <div className="space-y-6 animate-in fade-in duration-500 pb-10">
      {/* Welcome Header */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-6 pb-4 border-b">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">
            {t('dashboard.welcome')}, {user?.name}
          </h1>
          <p className="text-muted-foreground">
            {t('dashboard.welcomeUser', { name: user?.name?.split(' ')[0] || t('common.user') })}. {t('dashboard.projectStats', { count: runningProjects })}.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Link to="/feedback" className={cn(buttonVariants({ variant: "ghost" }), "gap-2 text-muted-foreground hover:text-foreground")}>
            <Zap className="w-4 h-4" />
            <span className="hidden sm:inline">{t('dashboard.needHelp')}</span>
          </Link>
          <Link to="/projects/new" className={cn(buttonVariants({ variant: "default" }))}>
            <Plus className="w-4 h-4 mr-2" />
            {t('common.newProject')}
          </Link>
        </div>
      </div>

      {/* Compact Stat Strip */}
      <Card className="bg-card/50 border-border/50">
        <CardContent className="flex flex-wrap items-center gap-x-6 gap-y-3 px-5 py-3.5">
          <div className="flex items-center gap-2.5">
            <Package className="w-4 h-4 text-muted-foreground shrink-0" />
            <span className="text-lg font-bold tracking-tight tabular-nums">{totalProjects}</span>
            <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">{t('dashboard.totalProjects')}</span>
          </div>

          <div className="h-5 w-px bg-border/60" />

          <div className="flex items-center gap-2.5">
            <span className="relative flex h-2 w-2 shrink-0">
              {runningProjects > 0 && <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-500 opacity-60" />}
              <span className={cn("relative inline-flex rounded-full h-2 w-2", runningProjects > 0 ? "bg-emerald-500" : "bg-muted-foreground/40")} />
            </span>
            <span className="text-lg font-bold tracking-tight tabular-nums">{runningProjects}<span className="text-sm text-muted-foreground font-semibold">/{totalProjects}</span></span>
            <span className="text-xs font-medium text-muted-foreground uppercase tracking-wider">{t('dashboard.runningProjects')}</span>
          </div>

          {frameworkBreakdown.length > 0 && (
            <>
              <div className="hidden sm:block h-5 w-px bg-border/60" />
              <div className="hidden sm:flex items-center gap-x-5 gap-y-2 flex-wrap">
                {frameworkBreakdown.map(([fw, count]) => (
                  <span key={fw} className="flex items-center gap-2 text-xs text-muted-foreground">
                    <FrameworkIcon framework={fw} variant="plain" className="w-3.5 h-3.5 shrink-0" />
                    <span className="font-mono">{fw}</span>
                    <span className="font-bold text-foreground/80 tabular-nums">{count}</span>
                  </span>
                ))}
              </div>
            </>
          )}
        </CardContent>
      </Card>

      {/* Recent Activity Table */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between pb-6">
          <div className="flex items-center gap-4">
            <div className="w-10 h-10 rounded-lg bg-muted border flex items-center justify-center">
              <Layout className="w-5 h-5 text-muted-foreground" />
            </div>
            <div>
              <CardTitle className="text-xl">{t('dashboard.recentProjects')}</CardTitle>
              <CardDescription>{t('dashboard.latestActivity')}</CardDescription>
            </div>
          </div>
          <Link to="/projects" className={cn(buttonVariants({ variant: "ghost", size: "sm" }), "hidden sm:flex gap-1")}>
            {t('dashboard.browseAll')} <ChevronRight className="w-4 h-4" />
          </Link>
        </CardHeader>

        {isLoading ? (
          <div className="p-24 flex flex-col items-center justify-center gap-6">
            <Loader2 className="w-10 h-10 text-primary animate-spin" />
            <p className="text-muted-foreground text-sm font-semibold uppercase tracking-widest animate-pulse">{t('dashboard.loadingProjects')}</p>
          </div>
        ) : (!projects || projects.length === 0) ? (
          <div className="p-24 text-center flex flex-col items-center max-w-sm mx-auto">
            <div className="w-20 h-20 bg-muted border rounded-full flex items-center justify-center mb-6">
              <Rocket className="w-10 h-10 text-muted-foreground opacity-50" />
            </div>
            <h4 className="text-xl font-bold tracking-tight mb-2">{t('dashboard.noProjectsFound')}</h4>
            <p className="text-muted-foreground text-sm mb-8">{t('dashboard.noProjectsDesc')}</p>
            <Link to="/projects/new" className={cn(buttonVariants({ variant: "default" }), "w-full")}>
              {t('dashboard.createFirstProject')}
            </Link>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('common.projectName')}</TableHead>
                  <TableHead>{t('common.url')}</TableHead>
                  <TableHead>{t('common.status')}</TableHead>
                  <TableHead>{t('common.date')}</TableHead>
                  <TableHead className="text-right">{t('common.actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(projects || []).slice(0, 5).map((project) => (
                  <TableRow key={project.uid}>
                    <TableCell>
                      <div className="flex items-center gap-4">
                        <Link
                          to={`/projects/${project.uid}`}
                          className="flex h-10 w-10 items-center justify-center rounded-lg border bg-muted transition-colors hover:border-primary/40 hover:bg-primary/10 focus:outline-none focus:ring-2 focus:ring-primary/20"
                          title={project.name}
                        >
                          <FrameworkIcon framework={project.framework} variant="compact" className="w-7 h-7" />
                        </Link>
                        <div>
                          <Link
                            to={`/projects/${project.uid}`}
                            className="block max-w-[200px] truncate text-sm font-semibold transition-colors hover:text-primary focus:outline-none focus:text-primary"
                            title={project.name}
                          >
                            {project.name}
                          </Link>
                          <div className="flex items-center gap-1.5 mt-0.5 text-muted-foreground">
                            <FrameworkIcon framework={project.framework} variant="plain" className="w-3 h-3" />
                            <span className="text-xs font-mono">{project.framework || t('common.general')}</span>
                          </div>
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      {project.status === 'running' ? (
                        <a
                          href={`https://${project.subdomain}.${window.location.hostname}`}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="flex items-center gap-1.5 text-muted-foreground hover:text-primary transition-colors"
                        >
                          <span className="font-mono text-xs">{project.subdomain}</span>
                          <ExternalLink className="w-3.5 h-3.5" />
                        </a>
                      ) : (
                        <span className="text-muted-foreground font-mono text-xs italic">{t('status.inactive')}</span>
                      )}
                    </TableCell>
                    <TableCell><StatusBadge status={project.status} /></TableCell>
                    <TableCell>
                      <div className="flex flex-col">
                        <span className="text-sm font-medium">{new Date(project.created_at).toLocaleDateString()}</span>
                        <span className="text-xs text-muted-foreground">{new Date(project.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
                      </div>
                    </TableCell>
                    <TableCell className="text-right">
                      <Link to={`/projects/${project.uid}`} className={cn(buttonVariants({ variant: "outline", size: "sm" }))}>
                        {t('common.details')}
                      </Link>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </Card>
    </div>
  )
}

export default UserDashboard
