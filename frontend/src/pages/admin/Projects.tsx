import { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { projectsAPI } from '../../services/api'
import { Project } from '../../types'
import {
  ExternalLink,
  Globe,
  User,
  Search,
  ChevronLeft,
  ChevronRight,
  Activity,
  Cpu,
  HardDrive,
  Info,
  Box,
  Monitor,
  RefreshCw
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import { Progress, ProgressTrack, ProgressIndicator } from '@/components/ui/progress'

// Add stats interface
interface ProjectStats {
  cpu_percent: number;
  memory_mb: number;
  memory_max_mb: number;
}

const StatusBadge = ({ status }: { status: Project['status'] }) => {
  switch (status) {
    case 'running':
      return <Badge variant="outline" className="text-emerald-600 border-emerald-500/40 bg-emerald-500/10"><div className="w-1.5 h-1.5 rounded-full bg-emerald-500 mr-1.5 animate-pulse" /> Operational</Badge>
    case 'building':
      return <Badge variant="outline" className="text-indigo-600 border-indigo-500/40 bg-indigo-500/10"><div className="w-1.5 h-1.5 rounded-full bg-indigo-500 mr-1.5 animate-pulse" /> Orchestrating</Badge>
    case 'pending':
      return <Badge variant="outline" className="text-amber-600 border-amber-500/40 bg-amber-500/10"><div className="w-1.5 h-1.5 rounded-full bg-amber-500 mr-1.5" /> Queued</Badge>
    case 'failed':
      return <Badge variant="destructive" className="bg-destructive/10 text-destructive hover:bg-destructive/20"><div className="w-1.5 h-1.5 rounded-full bg-destructive mr-1.5" /> Degraded</Badge>
    case 'stopped':
    default:
      return <Badge variant="secondary"><div className="w-1.5 h-1.5 rounded-full bg-muted-foreground mr-1.5" /> Halted</Badge>
  }
}

const ResourceBar = ({ label, value, max, icon: Icon, suffix = '' }: { label: string, value: number, max?: number, icon: any, suffix?: string }) => {
  const percentage = max ? Math.min((value / max) * 100, 100) : Math.min(value, 100)

  let colorClass = 'bg-emerald-500'
  if (percentage > 85) colorClass = 'bg-destructive'
  else if (percentage > 60) colorClass = 'bg-amber-500'

  return (
    <div className="flex flex-col gap-1.5 w-full min-w-[120px] max-w-[150px]">
      <div className="flex items-center justify-between text-xs font-semibold text-muted-foreground">
        <div className="flex items-center gap-1">
          <Icon className="w-3.5 h-3.5" />
          {label}
        </div>
        <span className="font-mono text-[10px]">
          {Number.isFinite(value) && !Number.isInteger(value) ? value.toFixed(1) : value}{suffix}
        </span>
      </div>
      <Progress value={percentage}>
        <ProgressTrack className="h-1.5 bg-muted/50">
          <ProgressIndicator className={colorClass} />
        </ProgressTrack>
      </Progress>
    </div>
  )
}

const AdminProjects = () => {
  const [projects, setProjects] = useState<Project[]>([])
  const [stats, setStats] = useState<Record<string, ProjectStats>>({})
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [isLoading, setIsLoading] = useState(true)

  const fetchProjects = useCallback(async () => {
    setIsLoading(true)
    try {
      const statusQuery = statusFilter === 'all' ? '' : statusFilter
      const response = await projectsAPI.listAll({ page, search, status: statusQuery, limit: 12 })
      setProjects(response.data.data || [])
      setTotal(response.data.total || 0)
    } catch (error) {
      toast.error('Failed to index projects')
    } finally {
      setIsLoading(false)
    }
  }, [page, search, statusFilter])

  useEffect(() => {
    fetchProjects()
  }, [fetchProjects])

  // Poll stats every 8 seconds
  useEffect(() => {
    const fetchStats = async () => {
      try {
        const response = await projectsAPI.listStats()
        setStats(response.data.stats || {})
      } catch (error) {
        console.error("Telemetry failure", error)
      }
    }

    fetchStats()
    const interval = setInterval(fetchStats, 8000)
    return () => clearInterval(interval)
  }, [])

  const totalPages = Math.ceil(total / 12)

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">Projects Cluster</h1>
          <p className="text-muted-foreground">Manage all user projects and system resources across the platform.</p>
        </div>

        <div className="flex items-center gap-4 bg-muted/30 border p-2 rounded-xl">
          <div className="flex items-center gap-2 px-4 border-r">
            <Activity className="w-4 h-4 text-emerald-500" />
            <span className="text-xs font-bold uppercase tracking-widest text-muted-foreground">{total} Monitored</span>
          </div>
          <div className="flex items-center gap-2 px-4 shadow-sm">
            <Monitor className="w-4 h-4 text-primary" />
            <span className="text-xs font-bold uppercase tracking-widest text-muted-foreground">{(projects || []).filter(p => p.status === 'running').length} Active Nodes</span>
          </div>
        </div>
      </div>

      <Card>
        <div className="p-6 border-b flex flex-col md:flex-row items-center justify-between gap-4">
          <div className="relative flex-1 w-full max-w-2xl">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="Search projects, domains, owners..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-9"
            />
          </div>

          <div className="flex items-center gap-3 w-full md:w-auto">
            <div className="w-full md:w-48">
              <Select value={statusFilter} onValueChange={(val) => setStatusFilter(val || 'all')}>
                <SelectTrigger className={'w-full'}>
                  <SelectValue placeholder="Status: All Lifecycle" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Status: All Lifecycle</SelectItem>
                  <SelectItem value="running">In Production</SelectItem>
                  <SelectItem value="building">Provisioning</SelectItem>
                  <SelectItem value="pending">Queued</SelectItem>
                  <SelectItem value="failed">Degraded</SelectItem>
                  <SelectItem value="stopped">Halted</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Button variant="outline" size="icon" onClick={fetchProjects} title="Force Sync">
              <RefreshCw className="w-4 h-4" />
            </Button>
          </div>
        </div>

        <div className="overflow-x-auto min-h-[400px]">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Project</TableHead>
                <TableHead>Owner</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Resource Usage</TableHead>
                <TableHead>Framework</TableHead>
                <TableHead className="text-right">Action</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={6} className="h-32 text-center text-muted-foreground font-medium uppercase tracking-widest text-xs">
                    Syncing Cluster State...
                  </TableCell>
                </TableRow>
              ) : (!projects || projects.length === 0) ? (
                <TableRow>
                  <TableCell colSpan={6} className="h-64 text-center">
                    <div className="flex flex-col items-center justify-center text-muted-foreground">
                      <div className="w-16 h-16 bg-muted/50 rounded-full flex items-center justify-center mb-4">
                        <Box className="w-8 h-8 opacity-50" />
                      </div>
                      <span className="font-semibold text-sm">No projects found in the system.</span>
                    </div>
                  </TableCell>
                </TableRow>
              ) : projects.map((project) => {
                const hasStats = stats?.[project.id]
                return (
                  <TableRow key={project.id}>
                    <TableCell>
                      <div className="flex items-center gap-4">
                        <div className={`w-10 h-10 rounded-lg flex items-center justify-center ${project.status === 'running' ? 'bg-emerald-500/10 text-emerald-600' : 'bg-muted/50 text-muted-foreground'
                          }`}>
                          <Globe className="w-5 h-5" />
                        </div>
                        <div className="flex flex-col">
                          <div className="flex items-center gap-2">
                            <span className="text-sm font-semibold truncate max-w-[180px]">{project.name}</span>
                            {project.status === 'running' && project.url && (
                              <a
                                href={project.url}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="text-muted-foreground hover:text-emerald-500 transition-colors"
                              >
                                <ExternalLink className="w-3.5 h-3.5" />
                              </a>
                            )}
                          </div>
                          <span className="text-xs text-muted-foreground font-mono truncate max-w-[200px]">{project.subdomain}</span>
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-3">
                        <div className="w-8 h-8 rounded-full bg-muted/50 flex items-center justify-center text-muted-foreground">
                          <User className="w-4 h-4" />
                        </div>
                        <div className="flex flex-col">
                          <span className="text-xs font-bold uppercase tracking-tight">{project.user?.name || 'Cluster Sync'}</span>
                          <span className="text-xs text-muted-foreground">{project.user?.email}</span>
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      <StatusBadge status={project.status} />
                    </TableCell>
                    <TableCell>
                      {project.status === 'running' && hasStats ? (
                        <div className="flex items-center gap-6">
                          <ResourceBar label="CPU" value={hasStats.cpu_percent} icon={Cpu} suffix="%" />
                          <ResourceBar label="MEM" value={hasStats.memory_mb} max={hasStats.memory_max_mb} icon={HardDrive} suffix="MB" />
                        </div>
                      ) : project.status === 'running' ? (
                        <div className="flex items-center gap-2 text-xs font-semibold text-muted-foreground">
                          <Activity className="w-3.5 h-3.5 animate-pulse text-primary/50" />
                          Awaiting Sync...
                        </div>
                      ) : (
                        <span className="text-muted-foreground/50">—</span>
                      )}
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <Badge variant="outline" className="font-mono bg-muted/30">
                          {project.laravel_version ? `Laravel v${project.laravel_version}` : 'Static/Unknown'}
                        </Badge>
                      </div>
                    </TableCell>
                    <TableCell className="text-right">
                      <Link
                        to={`/projects/${project.id}`}
                      >
                        <Button variant="outline" size="sm">
                          View Details
                        </Button>
                      </Link>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>

        {totalPages > 1 && (
          <div className="p-4 border-t flex flex-col sm:flex-row justify-between items-center gap-4 bg-muted/10">
            <div className="flex items-center gap-2 text-sm text-muted-foreground font-medium">
              <Info className="w-4 h-4 text-primary" />
              Showing {(page - 1) * 12 + 1} to {Math.min(page * 12, total)} of {total} nodes.
            </div>
            <div className="flex items-center gap-2">
              <Button variant="outline" size="icon" onClick={() => setPage(p => Math.max(1, p - 1))} disabled={page === 1}>
                <ChevronLeft className="w-4 h-4" />
              </Button>
              <div className="text-sm font-semibold px-4 border rounded-md h-10 flex items-center justify-center min-w-[4rem] bg-background">
                {page} / {totalPages}
              </div>
              <Button variant="outline" size="icon" onClick={() => setPage(p => Math.min(totalPages, p + 1))} disabled={page === totalPages}>
                <ChevronRight className="w-4 h-4" />
              </Button>
            </div>
          </div>
        )}
      </Card>
    </div>
  )
}

export default AdminProjects
