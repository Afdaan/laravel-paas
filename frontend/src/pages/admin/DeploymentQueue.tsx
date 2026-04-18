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
import { cn } from '@/lib/utils'

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
  user: {
    name: string;
    email: string;
  };
}

const DeploymentQueue = () => {
  const { t } = useTranslation()
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
        <Button variant="outline" size="sm" onClick={fetchData} className="gap-2">
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
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-6">
            <div className="flex items-center gap-4">
              <div className="w-10 h-10 rounded-lg bg-blue-500/10 border border-blue-500/20 flex items-center justify-center">
                <Activity className="w-5 h-5 text-blue-500" />
              </div>
              <div>
                <CardTitle className="text-xl">{t('admin.queue.activeBuilds')}</CardTitle>
                <CardDescription>Projects currently being built</CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent className="p-0 border-t">
            {activeBuilds.length === 0 ? (
              <EmptyState message={t('admin.queue.noActive')} />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/30">
                    <TableHead>{t('admin.queue.table.project')}</TableHead>
                    <TableHead>{t('admin.queue.table.owner')}</TableHead>
                    <TableHead className="text-right">{t('admin.queue.table.status')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {activeBuilds.map((project) => (
                    <TableRow key={project.id}>
                      <TableCell>
                        <div className="flex items-center gap-3">
                          <Package className="w-4 h-4 text-muted-foreground" />
                          <span className="font-semibold text-sm">{project.name}</span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-col">
                          <span className="text-xs font-medium">{project.user?.name}</span>
                          <span className="text-[10px] text-muted-foreground">{project.user?.email}</span>
                        </div>
                      </TableCell>
                      <TableCell className="text-right">
                        <Badge variant="outline" className="text-blue-500 bg-blue-500/10 border-blue-500/20 gap-1.5 animate-pulse">
                          <RefreshCw className="w-3 h-3 animate-spin" />
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
        <Card>
          <CardHeader className="flex flex-row items-center justify-between pb-6">
            <div className="flex items-center gap-4">
              <div className="w-10 h-10 rounded-lg bg-amber-500/10 border border-amber-500/20 flex items-center justify-center">
                <Clock className="w-5 h-5 text-amber-500" />
              </div>
              <div>
                <CardTitle className="text-xl">{t('admin.queue.waitingJobs')}</CardTitle>
                <CardDescription>Jobs waiting for a free worker</CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent className="p-0 border-t">
            {queuedJobs.length === 0 ? (
              <EmptyState message={t('admin.queue.noQueued')} />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow className="bg-muted/30">
                    <TableHead>{t('admin.queue.table.project')}</TableHead>
                    <TableHead>{t('admin.queue.table.type')}</TableHead>
                    <TableHead className="text-right">{t('admin.queue.table.enqueued')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {queuedJobs.map((job, i) => (
                    <TableRow key={`${job.project_id}-${i}`}>
                      <TableCell>
                        <div className="flex flex-col">
                          <span className="font-semibold text-sm">{job.project_name || `Project #${job.project_id}`}</span>
                          <span className="text-[10px] text-muted-foreground">{job.email || '...'}</span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <Badge variant="secondary" className="capitalize text-[10px]">
                          {job.type}
                        </Badge>
                      </TableCell>
                      <TableCell className="text-right">
                        <span className="text-xs font-mono text-muted-foreground">
                          {new Date(job.enqueued_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                        </span>
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
    <Card className="hover:border-primary/30 transition-colors group">
      <CardContent className="p-6">
        <div className="flex justify-between items-start">
          <div>
            <p className="text-xs font-bold uppercase tracking-widest text-muted-foreground mb-4">{label}</p>
            <div className="text-3xl font-bold tracking-tight">{value}</div>
          </div>
          <div className={cn("w-12 h-12 rounded-xl bg-muted border flex items-center justify-center group-hover:scale-110 transition-transform", color)}>
            <Icon className="w-6 h-6" />
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

function EmptyState({ message }: { message: string }) {
  return (
    <div className="p-12 text-center flex flex-col items-center gap-3 opacity-50">
      <Package className="w-8 h-8 text-muted-foreground" />
      <p className="text-sm font-medium uppercase tracking-widest">{message}</p>
    </div>
  )
}

export default DeploymentQueue
