import { useState, useEffect } from 'react'
import { toast } from 'sonner'
import { projectsAPI } from '../../services/api'
import useTranslation from '../../lib/useTranslation'
import { usePolling } from '../../lib/usePolling'
import {
  RefreshCw,
  Clock,
  CheckCircle2,
  AlertCircle,
  Activity,
  Package
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { cn, formatDistanceToNow } from '@/lib/utils'
import { useNavigate } from 'react-router-dom'

interface QueuedJob {
  project_id: number;
  project_name: string;
  email: string;
  type: string;
  enqueued_at: string;
  attempts: number;
}

interface ActiveBuild {
  id: number;
  name: string;
  status: string;
  updated_at: string;
  user: {
    name: string;
    email: string;
  };
}

const DeploymentQueue = () => {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [stats, setStats] = useState<any>(null)
  const [activeBuilds, setActiveBuilds] = useState<ActiveBuild[]>([])
  const [queuedJobs, setQueuedJobs] = useState<QueuedJob[]>([])
  const [isLoading, setIsLoading] = useState(true)

  const fetchData = async () => {
    try {
      const response = await projectsAPI.getQueueStats()
      setStats(response.data.stats)
      setActiveBuilds(response.data.active || [])
      setQueuedJobs(response.data.queued || [])
    } catch (error) {
      toast.error(t('common.loadError'))
    } finally {
      setIsLoading(false)
    }
  }

  // Initial fetch
  useEffect(() => {
    fetchData()
  }, [])

  // Poll every 5 seconds
  usePolling(fetchData, 5000)

  const formatPreciseTime = (dateString: string) => {
    const date = new Date(dateString)
    const time = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })
    const isToday = date.toDateString() === new Date().toDateString()

    if (isToday) return time
    return `${date.toLocaleDateString([], { month: 'short', day: 'numeric' })} ${time}`
  }

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-6 pb-4 border-b">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">{t('admin.queue.title')}</h1>
          <p className="text-muted-foreground max-w-2xl">
            {t('admin.queue.desc')}
          </p>
        </div>
        <Button variant="outline" size="sm" onClick={fetchData} className="gap-2 bg-background/50 backdrop-blur-sm hover:bg-muted transition-all">
          <RefreshCw className={cn("w-4 h-4", isLoading && "animate-spin")} />
          {t('admin.refresh')}
        </Button>
      </div>

      {/* Summary Cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-6">
        <StatCard
          label={t('admin.queue.summary.active')}
          value={activeBuilds.length}
          icon={Activity}
          color="text-blue-500"
        />
        <StatCard
          label={t('admin.queue.summary.queued')}
          value={queuedJobs.length}
          icon={Clock}
          color="text-amber-500"
        />
        <StatCard
          label={t('admin.queue.summary.processed')}
          value={stats?.processed || 0}
          icon={CheckCircle2}
          color="text-emerald-500"
        />
        <StatCard
          label={t('admin.queue.summary.failed')}
          value={stats?.failed || 0}
          icon={AlertCircle}
          color="text-rose-500"
        />
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-8">
        {/* Active Builds */}
        <Card className="overflow-hidden border-none shadow-xl bg-card/50 backdrop-blur-md ring-1 ring-white/10">
          <CardHeader className="flex flex-row items-center justify-between pb-6 bg-muted/30 border-b">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 rounded-2xl bg-blue-500/10 border border-blue-500/20 flex items-center justify-center shadow-sm">
                <Activity className="w-6 h-6 text-blue-500" />
              </div>
              <div>
                <CardTitle className="text-xl font-bold tracking-tight">{t('admin.queue.activeBuilds')}</CardTitle>
                <CardDescription className="text-xs font-semibold uppercase tracking-widest opacity-60">
                  {t('admin.queue.activeDesc')}
                </CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent className="p-0">
            {activeBuilds.length === 0 ? (
              <EmptyState message={t('admin.queue.noActive')} icon={Activity} />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/50 border-b hover:bg-muted/50">
                    <TableHead className="py-4 font-bold text-xs uppercase tracking-widest">{t('admin.queue.table.project')}</TableHead>
                    <TableHead className="py-4 font-bold text-xs uppercase tracking-widest">{t('admin.queue.table.owner')}</TableHead>
                    <TableHead className="py-4 font-bold text-xs uppercase tracking-widest">{t('admin.queue.table.started')}</TableHead>
                    <TableHead className="py-4 font-bold text-xs uppercase tracking-widest text-right">{t('admin.queue.table.status')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {activeBuilds.map((project) => (
                    <TableRow key={project.id} className="cursor-pointer hover:bg-muted/30 border-muted/20 transition-colors" onClick={() => navigate(`/projects/${project.id}`)}>
                      <TableCell className="py-4">
                        <div className="flex items-center gap-3">
                          <div className="w-8 h-8 rounded-lg bg-blue-500/5 border border-blue-500/10 flex items-center justify-center">
                            <Package className="w-4 h-4 text-blue-500/70" />
                          </div>
                          <span className="font-bold text-sm tracking-tight">{project.name}</span>
                        </div>
                      </TableCell>
                      <TableCell className="py-4">
                        <div className="flex flex-col">
                          <span className="text-xs font-bold">{project.user?.name}</span>
                          <span className="text-[10px] text-muted-foreground/80 font-medium">{project.user?.email}</span>
                        </div>
                      </TableCell>
                      <TableCell className="py-4">
                        <div className="flex flex-col">
                          <span className="text-xs font-mono font-bold text-primary/80">
                            {formatPreciseTime(project.updated_at)}
                          </span>
                          <span className="text-[10px] text-muted-foreground">
                            {formatDistanceToNow(new Date(project.updated_at), { addSuffix: true })}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell className="py-4 text-right">
                        <Badge variant="outline" className="text-[10px] font-bold uppercase tracking-wider text-blue-500 bg-blue-500/10 border-blue-500/20 gap-2 px-3 py-1 animate-pulse">
                          <div className="w-1.5 h-1.5 rounded-full bg-blue-500 animate-ping" />
                          {t('status.building')}
                        </Badge>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>

        {/* Waiting Queue */}
        <Card className="overflow-hidden border-none shadow-xl bg-card/50 backdrop-blur-md ring-1 ring-white/10">
          <CardHeader className="flex flex-row items-center justify-between pb-6 bg-muted/30 border-b">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 rounded-2xl bg-amber-500/10 border border-amber-500/20 flex items-center justify-center shadow-sm">
                <Clock className="w-6 h-6 text-amber-500" />
              </div>
              <div>
                <CardTitle className="text-xl font-bold tracking-tight">{t('admin.queue.waitingJobs')}</CardTitle>
                <CardDescription className="text-xs font-semibold uppercase tracking-widest opacity-60">
                  {t('admin.queue.waitingDesc')}
                </CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent className="p-0">
            {queuedJobs.length === 0 ? (
              <EmptyState message={t('admin.queue.noQueued')} icon={Clock} />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/50 border-b hover:bg-muted/50">
                    <TableHead className="py-4 font-bold text-xs uppercase tracking-widest">{t('admin.queue.table.project')}</TableHead>
                    <TableHead className="py-4 font-bold text-xs uppercase tracking-widest">{t('admin.queue.table.type')}</TableHead>
                    <TableHead className="py-4 font-bold text-xs uppercase tracking-widest text-right">{t('admin.queue.table.enqueued')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {queuedJobs.map((job, i) => (
                    <TableRow key={`${job.project_id}-${i}`} className="hover:bg-muted/30 border-muted/20 transition-colors">
                      <TableCell className="py-4">
                        <div className="flex flex-col">
                          <span className="font-bold text-sm tracking-tight">{job.project_name || `Project #${job.project_id}`}</span>
                          <span className="text-[10px] text-muted-foreground/80 font-medium">{job.email || '...'}</span>
                        </div>
                      </TableCell>
                      <TableCell className="py-4">
                        <Badge variant="secondary" className="capitalize text-[10px] font-bold px-2 py-0.5 bg-muted border border-muted-foreground/10">
                          {job.type}
                        </Badge>
                      </TableCell>
                      <TableCell className="py-4 text-right">
                        <div className="flex flex-col items-end">
                          <span className="text-xs font-mono font-bold text-amber-500/80" title={new Date(job.enqueued_at).toLocaleString()}>
                            {formatPreciseTime(job.enqueued_at)}
                          </span>
                          <span className="text-[10px] text-muted-foreground">
                            {formatDistanceToNow(new Date(job.enqueued_at), { addSuffix: true })}
                          </span>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

function StatCard({ label, value, icon: Icon, color }: { label: string, value: number, icon: any, color: string }) {
  return (
    <Card className="hover:border-primary/40 transition-all group overflow-hidden border shadow-sm hover:shadow-md bg-card/30 backdrop-blur-sm">
      <CardContent className="p-6 relative">
        <div className="absolute -right-4 -bottom-4 opacity-5 group-hover:opacity-10 transition-opacity">
          <Icon className={cn("w-24 h-24", color)} />
        </div>
        <div className="flex justify-between items-start relative z-10">
          <div>
            <p className="text-[10px] font-bold uppercase tracking-[0.2em] text-muted-foreground mb-4">{label}</p>
            <div className="text-3xl font-black tracking-tighter">{value}</div>
          </div>
          <div className={cn("w-12 h-12 rounded-xl bg-muted border flex items-center justify-center group-hover:scale-110 group-hover:-rotate-6 transition-all shadow-sm", color)}>
            <Icon className="w-6 h-6" />
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function EmptyState({ message, icon: Icon = Package }: { message: string, icon?: any }) {
  return (
    <div className="p-16 text-center flex flex-col items-center gap-4 transition-opacity duration-1000">
      <div className="w-16 h-16 rounded-full bg-muted/50 flex items-center justify-center mb-2">
        <Icon className="w-8 h-8 text-muted-foreground/40" />
      </div>
      <div>
        <p className="text-xs font-bold uppercase tracking-[0.25em] text-muted-foreground/60">{message}</p>
      </div>
    </div>
  )
}

export default DeploymentQueue
