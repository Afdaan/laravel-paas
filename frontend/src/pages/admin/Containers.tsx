import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '../../services/api'
import useTranslation from '../../lib/useTranslation'
import {
  Search,
  Box,
  MoreHorizontal,
  Cpu,
  HardDrive,
  Terminal,
  Loader2,
  ListFilter,
  CheckCircle2,
  XCircle,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  Info,
  RotateCw
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Progress } from '@/components/ui/progress'

interface ContainerData {
  id: string;
  names: string[];
  image: string;
  state: string;
  cpu_percent: number;
  memory_usage: number;
  ip_address: string;
  ports: string[];
}

const AdminContainers = () => {
  const { t } = useTranslation()
  const [data, setData] = useState<{ containers: ContainerData[], system: unknown }>({
    containers: [],
    system: null
  })
  const [isLoading, setIsLoading] = useState(true)
  const [searchQuery, setSearchQuery] = useState('')
  const [limit, setLimit] = useState('all')
  const [page, setPage] = useState(1)

  const fetchData = useCallback(async () => {
    try {
      const res = await systemAPI.getStats()
      setData(res.data)
    } catch (error) {
      console.error('Failed to fetch containers:', error)
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 8000)
    return () => clearInterval(interval)
  }, [fetchData])

  const filteredContainers = useMemo(() => {
    const containers = data?.containers || []
    const filtered = containers.filter(c =>
      (c.names[0] || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
      c.image.toLowerCase().includes(searchQuery.toLowerCase())
    )
    
    if (limit === 'all') return filtered
    const numLimit = parseInt(limit)
    return filtered.slice((page - 1) * numLimit, page * numLimit)
  }, [data, searchQuery, limit, page])

  const stats = useMemo(() => {
    const containers = data?.containers || []
    const total = containers.length
    const running = containers.filter(c => c.state === 'running').length
    const stopped = total - running
    return { total, running, stopped }
  }, [data])

  if (isLoading && (!data?.containers || data.containers.length === 0)) {
    return (
      <div className="flex flex-col items-center justify-center h-[60vh] gap-6">
        <Loader2 className="w-12 h-12 text-primary animate-spin" />
        <p className="text-muted-foreground text-xs font-bold uppercase tracking-widest animate-pulse">{t('common.loading')}</p>
      </div>
    )
  }

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">{t('admin.containers.title')}</h1>
          <p className="text-muted-foreground">{t('admin.containers.desc')}</p>
        </div>

        <div className="flex items-center gap-6">
          <div className="flex items-center gap-2 bg-muted/30 border p-2 rounded-xl text-xs font-bold uppercase tracking-widest text-muted-foreground">
            <div className="flex items-center gap-2 px-4 border-r">
              <div className="w-2.5 h-2.5 rounded-full bg-indigo-500 shadow-sm" />
              <span className="ml-1">{stats.total} {t('common.total')}</span>
            </div>
            <div className="flex items-center gap-2 px-4 border-r">
              <div className="w-2.5 h-2.5 rounded-full bg-emerald-500 shadow-sm animate-pulse" />
              <span className="ml-1">{stats.running} {t('common.active')}</span>
            </div>
            <div className="flex items-center gap-2 px-4">
              <div className="w-2.5 h-2.5 rounded-full bg-rose-500 shadow-sm" />
              <span className="ml-1">{stats.stopped} {t('common.offline')}</span>
            </div>
          </div>
        </div>
      </div>

      <Card>
        <div className="p-6 border-b flex flex-col md:flex-row items-center justify-between gap-4">
          <div className="relative flex-1 w-full max-w-2xl">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder={t('common.search')}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-9 w-full"
            />
          </div>
          <div className="flex items-center gap-3 w-full md:w-auto">
            <Button variant="outline" size="sm" className="hidden md:flex h-9">
              <ListFilter className="w-4 h-4 mr-2" /> {t('common.policy')}
            </Button>
            <Button variant="outline" size="icon" onClick={fetchData} className="w-9 h-9" title="Refresh">
              <RotateCw className="w-4 h-4 text-muted-foreground" />
            </Button>
          </div>
        </div>

        <div className="overflow-x-auto">
          <Table className="min-w-[1100px] table-fixed">
            <TableHeader>
              <TableRow className="bg-muted/20 hover:bg-muted/20">
                <TableHead className="w-10 px-4 text-center">
                  <Checkbox />
                </TableHead>
                <TableHead className="w-[26%] h-12 px-4 text-xs font-semibold uppercase tracking-wider">{t('admin.containers.instanceDetail')}</TableHead>
                <TableHead className="w-[12%] h-12 px-4 text-xs font-semibold uppercase tracking-wider">{t('common.status')}</TableHead>
                <TableHead className="w-[20%] h-12 px-4 text-xs font-semibold uppercase tracking-wider">{t('admin.containers.health')}</TableHead>
                <TableHead className="w-[18%] h-12 px-4 text-xs font-semibold uppercase tracking-wider">{t('admin.containers.resourceLoad')}</TableHead>
                <TableHead className="w-[16%] h-12 px-4 text-xs font-semibold uppercase tracking-wider">{t('admin.containers.gatewayPorts')}</TableHead>
                <TableHead className="w-[8%] h-12 pl-4 pr-6 text-right text-xs font-semibold uppercase tracking-wider">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredContainers.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={7} className="h-64 text-center">
                    <div className="flex flex-col items-center justify-center text-muted-foreground">
                      <div className="w-16 h-16 bg-muted/50 rounded-full flex items-center justify-center mb-4">
                        <Terminal className="w-8 h-8 opacity-50" />
                      </div>
                      <span className="font-semibold text-sm">{t('common.noData')}</span>
                    </div>
                  </TableCell>
                </TableRow>
              ) : filteredContainers.map((c: ContainerData) => (
                <TableRow key={c.id} className="hover:bg-muted/20">
                  <TableCell className="px-4 py-3 text-center">
                    <Checkbox />
                  </TableCell>

                  {/* Instance Detail */}
                  <TableCell className="px-4 py-4">
                    <div className="flex items-center gap-3">
                      <div className={`w-9 h-9 shrink-0 rounded-lg flex items-center justify-center ${c.state === 'running' ? 'bg-indigo-500/10 text-indigo-500' : 'bg-muted/50 text-muted-foreground'}`}>
                        <Box className="w-4 h-4" />
                      </div>
                      <div className="flex flex-col min-w-0">
                        <span className="text-sm font-semibold truncate">
                          {c.names[0]?.replace('/', '') || c.id.substring(0, 12)}
                        </span>
                        <span className="text-[11px] text-muted-foreground font-mono truncate">{c.image}</span>
                      </div>
                    </div>
                  </TableCell>

                  {/* Status */}
                  <TableCell className="px-4 py-4">
                    {c.state === 'running' ? (
                      <Badge variant="outline" className="text-emerald-600 border-emerald-500/30 bg-emerald-500/10 gap-1.5">
                        <div className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
                        {t('status.running')}
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="text-rose-500 border-rose-500/30 bg-rose-500/10 gap-1.5">
                        <div className="w-1.5 h-1.5 rounded-full bg-rose-500" />
                        {t('status.stopped')}
                      </Badge>
                    )}
                  </TableCell>

                  {/* Health */}
                  <TableCell className="px-4 py-4">
                    <div className="flex flex-col gap-1.5">
                      <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
                        {c.state === 'running'
                          ? <CheckCircle2 className="w-3.5 h-3.5 shrink-0 text-emerald-500" />
                          : <XCircle className="w-3.5 h-3.5 shrink-0 text-rose-400" />}
                        <span>{t('admin.containers.livenessCheck')}</span>
                      </div>
                      <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
                        {c.state === 'running'
                          ? <CheckCircle2 className="w-3.5 h-3.5 shrink-0 text-emerald-500" />
                          : <XCircle className="w-3.5 h-3.5 shrink-0 text-rose-400" />}
                        <span>{t('admin.containers.readinessProbe')}</span>
                      </div>
                    </div>
                  </TableCell>

                  {/* Resource Load */}
                  <TableCell className="px-4 py-4">
                    <div className="flex flex-col gap-2">
                      <div className="flex items-center justify-between text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                        <div className="flex items-center gap-1.5"><Cpu className="w-3 h-3" /> CPU</div>
                        <span className="font-mono tabular-nums">{(c.cpu_percent || 0).toFixed(1)}%</span>
                      </div>
                      <Progress value={Math.min(c.cpu_percent || 0, 100)} className="h-1" />
                      <div className="flex items-center justify-between text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                        <div className="flex items-center gap-1.5"><HardDrive className="w-3 h-3" /> MEM</div>
                        <span className="font-mono tabular-nums">{(c.memory_usage || 0).toFixed(1)} MB</span>
                      </div>
                    </div>
                  </TableCell>

                  {/* Gateway Ports */}
                  <TableCell className="px-4 py-4">
                    <div className="flex flex-col gap-1.5">
                      <span className="text-[11px] font-mono font-medium text-muted-foreground">{c.ip_address || t('common.unassigned')}</span>
                      <div className="flex flex-wrap gap-1 max-w-[180px]">
                        {c.ports?.slice(0, 2).map((p, i) => (
                          <Badge key={i} variant="secondary" className="text-[10px] font-mono px-1.5 py-0 h-5 bg-muted/60 truncate max-w-[140px]" title={p}>
                            {p}
                          </Badge>
                        ))}
                        {c.ports?.length > 2 && (
                          <Badge variant="outline" className="text-[10px] h-5 px-1.5 border-dashed opacity-60">
                            +{c.ports.length - 2}
                          </Badge>
                        )}
                      </div>
                    </div>
                  </TableCell>

                  {/* Actions */}
                  <TableCell className="pl-4 pr-6 py-4 text-right">
                    <DropdownMenu>
                      <DropdownMenuTrigger>
                        <Button variant="ghost" size="icon" className="h-8 w-8 hover:bg-muted/50">
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-48">
                        <DropdownMenuItem className="gap-2 cursor-pointer">
                          <Info className="w-4 h-4 text-muted-foreground" /> {t('common.details')}
                        </DropdownMenuItem>
                        <DropdownMenuItem className="gap-2 cursor-pointer">
                          <Terminal className="w-4 h-4 text-muted-foreground" /> {t('common.executeShell')}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem className={c.state === 'running' ? "text-destructive gap-2 cursor-pointer" : "gap-2 cursor-pointer text-emerald-600"}>
                          {c.state === 'running' ? t('admin.containers.forceStop') : t('admin.containers.startInstance')}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>

        <div className="p-4 border-t flex flex-col sm:flex-row justify-between items-center gap-4 bg-muted/10">
          <div className="flex items-center gap-6">
            <div className="flex items-center gap-2 text-sm text-muted-foreground font-medium">
              <Info className="w-4 h-4 text-primary" />
              Showing {filteredContainers.length} of {stats.total} nodes.
            </div>
            <div className="flex items-center space-x-2">
              <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Rows per page</p>
              <Select value={limit} onValueChange={(val) => { if (val) { setLimit(val); setPage(1); } }}>
                <SelectTrigger className="h-8 w-[82px] justify-between">
                  <SelectValue placeholder={t('common.all')}>
                    {limit === 'all' ? t('common.all') : limit}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent side="top" align="end" alignItemWithTrigger={false} className="min-w-[100px] bg-popover/98 backdrop-blur-lg border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72">
                  <SelectItem value="all">{t('common.all')}</SelectItem>
                  <SelectItem value="10">10</SelectItem>
                  <SelectItem value="25">25</SelectItem>
                  <SelectItem value="50">50</SelectItem>
                  <SelectItem value="100">100</SelectItem>
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="flex items-center space-x-2">
            <Button variant="outline" className="h-8 w-8 p-0" disabled>
              <ChevronsLeft className="h-4 w-4" />
            </Button>
            <Button variant="outline" className="h-8 w-8 p-0" disabled>
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <Button variant="outline" className="h-8 w-8 p-0" disabled>
              <ChevronRight className="h-4 w-4" />
            </Button>
            <Button variant="outline" className="h-8 w-8 p-0" disabled>
              <ChevronsRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </Card>
    </div>
  )
}

export default memo(AdminContainers)

