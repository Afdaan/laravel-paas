import { useState, useEffect, useCallback, useRef } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { projectsAPI } from '../../services/api'
import useTranslation from '../../lib/useTranslation'
import { Project } from '../../types'
import {
  ExternalLink,
  Globe,
  User,
  Search,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
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

const StatusBadge = ({ status, t }: { status: Project['status'], t: any }) => {
  switch (status) {
    case 'running':
      return <Badge variant="outline" className="text-emerald-600 border-emerald-500/40 bg-emerald-500/10"><div className="w-1.5 h-1.5 rounded-full bg-emerald-500 mr-1.5 animate-pulse" /> {t('status.running')}</Badge>
    case 'building':
      return <Badge variant="outline" className="text-indigo-600 border-indigo-500/40 bg-indigo-500/10"><div className="w-1.5 h-1.5 rounded-full bg-indigo-500 mr-1.5 animate-pulse" /> {t('status.building')}</Badge>
    case 'pending':
      return <Badge variant="outline" className="text-amber-600 border-amber-500/40 bg-amber-500/10"><div className="w-1.5 h-1.5 rounded-full bg-amber-500 mr-1.5" /> {t('status.pending')}</Badge>
    case 'failed':
      return <Badge variant="destructive" className="bg-destructive/10 text-destructive hover:bg-destructive/20"><div className="w-1.5 h-1.5 rounded-full bg-destructive mr-1.5" /> {t('status.failed')}</Badge>
    case 'stopped':
    default:
      return <Badge variant="secondary"><div className="w-1.5 h-1.5 rounded-full bg-muted-foreground mr-1.5" /> {t('status.stopped')}</Badge>
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
  const { t } = useTranslation()
  const [projects, setProjects] = useState<Project[]>([])
  const [stats, setStats] = useState<Record<string, ProjectStats>>({})
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [limit, setLimit] = useState(10)
  const [isLoading, setIsLoading] = useState(true)
  const isFirstLoad = useRef(true)

  const fetchProjects = useCallback(async (forced = false) => {
    if (isFirstLoad.current || forced) {
      setIsLoading(true)
    }
    
    try {
      const statusQuery = statusFilter === 'all' ? '' : statusFilter
      const response = await projectsAPI.listAll({ page, search, status: statusQuery, limit })
      setProjects(response.data.data || [])
      setTotal(response.data.total || 0)
    } catch (error) {
      toast.error(t('admin.projects.loadError'))
    } finally {
      setIsLoading(false)
      isFirstLoad.current = false
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

  const totalPages = Math.ceil(total / limit)

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">{t('admin.projects.title')}</h1>
          <p className="text-muted-foreground">{t('admin.projects.desc')}</p>
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
              placeholder={t('common.search')}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="pl-9"
            />
          </div>

          <div className="flex items-center gap-3 w-full md:w-auto">
            <div className="w-full md:w-48">
              <Select value={statusFilter} onValueChange={(val) => setStatusFilter(val || 'all')}>
                <SelectTrigger className={'w-full'}>
                  <SelectValue placeholder={`Status: ${t('status.running')}`} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">Status: All Lifecycle</SelectItem>
                  <SelectItem value="running">{t('status.running')}</SelectItem>
                  <SelectItem value="building">{t('status.building')}</SelectItem>
                  <SelectItem value="pending">{t('status.pending')}</SelectItem>
                  <SelectItem value="failed">{t('status.failed')}</SelectItem>
                  <SelectItem value="stopped">{t('status.stopped')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Button variant="outline" size="icon" onClick={() => fetchProjects(true)} title="Force Sync">
              <RefreshCw className="w-4 h-4" />
            </Button>
          </div>
        </div>

        <div className="overflow-x-auto">
          <Table className="min-w-[1100px] table-fixed">
            <TableHeader>
              <TableRow className="bg-muted/20 hover:bg-muted/20">
                <TableHead className="h-12 w-[32%] pl-6 pr-4 text-xs font-semibold uppercase tracking-wider">{t('common.projectName')}</TableHead>
                <TableHead className="h-12 w-[18%] px-4 text-xs font-semibold uppercase tracking-wider">Owner</TableHead>
                <TableHead className="h-12 w-[14%] px-4 text-xs font-semibold uppercase tracking-wider">Framework</TableHead>
                <TableHead className="h-12 w-[12%] px-4 text-center text-xs font-semibold uppercase tracking-wider">{t('common.status')}</TableHead>
                <TableHead className="h-12 w-[16%] px-4 text-center text-xs font-semibold uppercase tracking-wider">Resource Usage</TableHead>
                <TableHead className="h-12 w-[8%] pl-4 pr-6 text-right text-xs font-semibold uppercase tracking-wider">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={6} className="h-32 text-center text-muted-foreground font-medium uppercase tracking-widest text-xs">
                    {t('common.loading')}
                  </TableCell>
                </TableRow>
              ) : (!projects || projects.length === 0) ? (
                <TableRow>
                  <TableCell colSpan={6} className="h-64 text-center">
                    <div className="flex flex-col items-center justify-center text-muted-foreground">
                      <div className="w-16 h-16 bg-muted/50 rounded-full flex items-center justify-center mb-4">
                        <Box className="w-8 h-8 opacity-50" />
                      </div>
                      <span className="font-semibold text-sm">{t('common.noData')}</span>
                    </div>
                  </TableCell>
                </TableRow>
              ) : projects.map((project) => {
                const hasStats = stats?.[project.id]
                return (
                  <TableRow key={project.id} className="hover:bg-muted/20">
                    <TableCell className="pl-6 pr-4 py-3">
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
                    <TableCell className="px-4 py-3">
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
                    <TableCell className="px-4 py-3">
                      <div className="flex items-center gap-2">
                        <Badge variant="outline" className="font-mono bg-muted/30">
                          {project.laravel_version ? `Laravel ${project.laravel_version}` : 'Static/Unknown'}
                        </Badge>
                      </div>
                    </TableCell>
                    <TableCell className="px-4 py-3 text-center">
                      <StatusBadge status={project.status} t={t} />
                    </TableCell>
                    <TableCell className="px-4 py-3 text-center">
                      {project.status === 'running' && hasStats ? (
                        <div className="inline-flex items-center gap-4 justify-center">
                          <ResourceBar label="CPU" value={hasStats.cpu_percent} icon={Cpu} suffix="%" />
                          <ResourceBar label="MEM" value={hasStats.memory_mb} max={hasStats.memory_max_mb} icon={HardDrive} suffix="MB" />
                        </div>
                      ) : project.status === 'running' ? (
                        <div className="inline-flex items-center gap-2 text-xs font-semibold text-muted-foreground">
                          <Activity className="w-3.5 h-3.5 animate-pulse text-primary/50" />
                          Awaiting Sync...
                        </div>
                      ) : (
                        <span className="text-muted-foreground/50">—</span>
                      )}
                    </TableCell>
                    <TableCell className="pl-4 pr-6 py-3 text-right">
                      <Link
                        to={`/projects/${project.id}`}
                      >
                        <Button variant="outline" size="sm" className="h-8">
                          {t('common.details')}
                        </Button>
                      </Link>
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>

        {totalPages > 0 && (
          <div className="p-4 border-t flex flex-col sm:flex-row justify-between items-center gap-4 bg-muted/10">
            <div className="flex items-center gap-6">
              <div className="flex items-center gap-2 text-sm text-muted-foreground font-medium">
                <Info className="w-4 h-4 text-primary" />
                Showing {(page - 1) * limit + 1} to {Math.min(page * limit, total)} of {total} nodes.
              </div>
              <div className="flex items-center space-x-2">
                <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Rows per page</p>
                <Select
                  value={limit.toString()}
                  onValueChange={(value) => {
                    setLimit(Number(value))
                    setPage(1)
                  }}
                >
                  <SelectTrigger size="sm" className="h-8 w-[82px] justify-between">
                    <SelectValue placeholder={limit} />
                  </SelectTrigger>
                  <SelectContent
                    side="top"
                    align="end"
                    className="min-w-[120px] max-h-[220px] rounded-lg p-1 shadow-lg"
                  >
                    {[10, 15, 20, 25, 30, 40, 50, 75, 100].map((pageSize) => (
                      <SelectItem
                        key={pageSize}
                        value={`${pageSize}`}
                        className="rounded-md py-1.5 px-2 text-sm"
                      >
                        {pageSize}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="flex items-center space-x-6 lg:space-x-8">
              <div className="flex w-[100px] items-center justify-center text-sm font-medium">
                Page {page} of {Math.max(1, totalPages)}
              </div>
              <div className="flex items-center space-x-2">
                <Button
                  variant="outline"
                  className="hidden h-8 w-8 p-0 lg:flex"
                  onClick={() => setPage(1)}
                  disabled={page === 1}
                >
                  <span className="sr-only">Go to first page</span>
                  <ChevronsLeft className="h-4 w-4" />
                </Button>
                <Button
                  variant="outline"
                  className="h-8 w-8 p-0"
                  onClick={() => setPage(p => Math.max(1, p - 1))}
                  disabled={page === 1}
                >
                  <span className="sr-only">Go to previous page</span>
                  <ChevronLeft className="h-4 w-4" />
                </Button>
                <Button
                  variant="outline"
                  className="h-8 w-8 p-0"
                  onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                  disabled={page === totalPages || totalPages === 0}
                >
                  <span className="sr-only">Go to next page</span>
                  <ChevronRight className="h-4 w-4" />
                </Button>
                <Button
                  variant="outline"
                  className="hidden h-8 w-8 p-0 lg:flex"
                  onClick={() => setPage(totalPages)}
                  disabled={page === totalPages || totalPages === 0}
                >
                  <span className="sr-only">Go to last page</span>
                  <ChevronsRight className="h-4 w-4" />
                </Button>
              </div>
            </div>
          </div>
        )}
      </Card>
    </div>
  )
}

export default AdminProjects
