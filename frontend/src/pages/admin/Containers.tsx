import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '../../services/api'
import {
  Search,
  LayoutGrid,
  Box,
  MoreHorizontal,
  Activity,
  Cpu,
  HardDrive,
  Terminal,
  Loader2,
  ListFilter,
  CheckCircle2,
  XCircle
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
  const [data, setData] = useState<{ containers: ContainerData[], system: any }>({
    containers: [],
    system: null
  })
  const [isLoading, setIsLoading] = useState(true)
  const [searchQuery, setSearchQuery] = useState('')

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
    return data.containers.filter(c =>
      (c.names[0] || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
      c.image.toLowerCase().includes(searchQuery.toLowerCase())
    )
  }, [data.containers, searchQuery])

  const stats = useMemo(() => {
    const total = data.containers.length
    const running = data.containers.filter(c => c.state === 'running').length
    const stopped = total - running
    return { total, running, stopped }
  }, [data.containers])

  if (isLoading && data.containers.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-[60vh] gap-6">
        <Loader2 className="w-12 h-12 text-primary animate-spin" />
        <p className="text-muted-foreground text-xs font-bold uppercase tracking-widest animate-pulse">Orchestrating Container State</p>
      </div>
    )
  }

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6 pb-4 border-b">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">Containers</h1>
          <p className="text-muted-foreground">Manage and monitor all running project instances.</p>
        </div>

        <div className="flex items-center gap-6">
          <div className="flex items-center gap-2 bg-muted/30 border p-2 rounded-xl text-xs font-bold uppercase tracking-widest text-muted-foreground">
            <div className="flex items-center gap-2 px-3 border-r">
              <div className="w-2.5 h-2.5 rounded-full bg-indigo-500 shadow-sm" />
              {stats.total} Total
            </div>
            <div className="flex items-center gap-2 px-3 border-r">
              <div className="w-2.5 h-2.5 rounded-full bg-emerald-500 shadow-sm animate-pulse" />
              {stats.running} Active
            </div>
            <div className="flex items-center gap-2 px-3">
              <div className="w-2.5 h-2.5 rounded-full bg-rose-500 shadow-sm" />
              {stats.stopped} Offline
            </div>
          </div>
        </div>
      </div>

      <Card>
        <div className="p-4 border-b flex flex-col md:flex-row items-center justify-between gap-4">
          <div className="relative flex-1 w-full max-w-xl">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder="Filter active instances by name or manifest..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-9 w-full"
            />
          </div>
          <div className="flex items-center gap-3">
            <Button variant="outline" size="sm" className="hidden md:flex">
              <ListFilter className="w-4 h-4 mr-2" /> Policy
            </Button>
            <Button variant="outline" size="sm" className="text-indigo-600 border-indigo-200 bg-indigo-50 hover:bg-indigo-100">
              <LayoutGrid className="w-4 h-4 mr-2" /> Matrix View
            </Button>
          </div>
        </div>

        <div className="overflow-x-auto min-h-[400px]">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-12 text-center">
                  <Checkbox />
                </TableHead>
                <TableHead>Instance Detail</TableHead>
                <TableHead>State</TableHead>
                <TableHead>Health Requirements</TableHead>
                <TableHead>Resource Load</TableHead>
                <TableHead>Gateway Ports</TableHead>
                <TableHead className="text-right">Action</TableHead>
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
                      <span className="font-semibold text-sm">No active instances in cluster</span>
                    </div>
                  </TableCell>
                </TableRow>
              ) : filteredContainers.map((c) => (
                <TableRow key={c.id}>
                  <TableCell className="text-center">
                    <Checkbox />
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-4">
                      <div className="w-10 h-10 rounded-lg flex items-center justify-center bg-muted text-muted-foreground">
                        <Box className="w-5 h-5" />
                      </div>
                      <div className="flex flex-col">
                        <span className="text-sm font-semibold truncate max-w-[200px]">
                          {c.names[0]?.replace('/', '') || c.id.substring(0, 12)}
                        </span>
                        <span className="text-xs text-muted-foreground font-mono truncate max-w-[200px]">{c.image}</span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    {c.state === 'running' ? (
                      <Badge variant="outline" className="text-emerald-600 border-emerald-500/40 bg-emerald-500/10"><div className="w-1.5 h-1.5 rounded-full bg-emerald-500 mr-1.5 animate-pulse" /> Active</Badge>
                    ) : (
                      <Badge variant="destructive" className="bg-rose-500/10 text-rose-600 border-rose-500/20 hover:bg-rose-500/20"><div className="w-1.5 h-1.5 rounded-full bg-rose-500 mr-1.5" /> Offline</Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-col gap-1.5 text-xs text-muted-foreground">
                      <div className="flex items-center gap-1.5">
                        {c.state === 'running' ? <CheckCircle2 className="w-3.5 h-3.5 text-emerald-500" /> : <XCircle className="w-3.5 h-3.5 text-rose-500" />}
                        Liveness Check
                      </div>
                      <div className="flex items-center gap-1.5">
                        {c.state === 'running' ? <CheckCircle2 className="w-3.5 h-3.5 text-emerald-500" /> : <XCircle className="w-3.5 h-3.5 text-rose-500" />}
                        Readiness Probe
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-col gap-2 w-32">
                      <div className="flex items-center justify-between text-[10px] font-bold uppercase tracking-widest text-muted-foreground">
                        <div className="flex items-center gap-1"><Cpu className="w-3 h-3" /> CPU</div>
                        <span>{(c.cpu_percent || 0).toFixed(1)}%</span>
                      </div>
                      <Progress value={Math.min(c.cpu_percent || 0, 100)} className="h-1.5" />
                      <div className="flex items-center justify-between text-[10px] font-bold uppercase tracking-widest text-muted-foreground pt-1">
                        <div className="flex items-center gap-1"><HardDrive className="w-3 h-3" /> MEM</div>
                        <span>{(c.memory_usage || 0).toFixed(1)}MB</span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-col gap-1.5">
                      <span className="text-xs font-mono font-bold text-muted-foreground">{c.ip_address || 'Unassigned'}</span>
                      <div className="flex flex-wrap gap-1">
                        {c.ports?.slice(0, 2).map((p, i) => (
                          <Badge key={i} variant="secondary" className="text-[10px] font-mono px-1.5 py-0">
                            {p}
                          </Badge>
                        ))}
                        {c.ports?.length > 2 && <span className="text-[10px] text-muted-foreground font-bold">+{c.ports.length - 2}</span>}
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="text-right">
                    <DropdownMenu>
                      <DropdownMenuTrigger className="inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium ring-offset-background transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 hover:bg-accent hover:text-accent-foreground h-8 w-8">
                        <span className="sr-only">Open menu</span>
                        <MoreHorizontal className="h-4 w-4" />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        {/* <DropdownMenuLabel>Actions</DropdownMenuLabel> */}
                        <DropdownMenuItem>View Logs</DropdownMenuItem>
                        <DropdownMenuItem>Execute Shell</DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem className={c.state === 'running' ? "text-destructive" : ""}>
                          {c.state === 'running' ? 'Force Stop' : 'Start Instance'}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>

        <div className="p-4 border-t flex flex-col md:flex-row items-center justify-between gap-4 bg-muted/10">
          <div className="flex items-center gap-2 text-sm text-muted-foreground font-medium">
            <Activity className="w-4 h-4 text-primary" />
            Showing {filteredContainers.length} global cluster nodes.
          </div>
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <span className="text-xs font-bold text-muted-foreground uppercase tracking-widest">Rows per page</span>
              <Select defaultValue="all">
                <SelectTrigger className="w-[80px] h-8">
                  <SelectValue placeholder="All" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">All</SelectItem>
                  <SelectItem value="10">10</SelectItem>
                  <SelectItem value="20">20</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center gap-2 text-sm font-semibold">
              <Button variant="outline" size="sm" disabled>Previous</Button>
              <Button variant="outline" size="sm" disabled>Next</Button>
            </div>
          </div>
        </div>
      </Card>
    </div>
  )
}

export default memo(AdminContainers)

