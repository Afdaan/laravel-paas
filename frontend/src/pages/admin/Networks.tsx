import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '../../services/api'
import {
  Plus,
  RotateCw,
  Share2,
  MoreHorizontal,
  Activity,
  Globe,
  Loader2
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'

interface NetworkData {
  id: string;
  name: string;
  status: string;
  driver: string;
  scope: string;
}

const AdminNetworks = () => {
  const [data, setData] = useState<{ networks: NetworkData[] }>({
    networks: []
  })
  const [isLoading, setIsLoading] = useState(true)

  const fetchData = useCallback(async () => {
    try {
      const res = await systemAPI.getStats()
      setData(res.data)
    } catch (error) {
      console.error('Failed to fetch networks:', error)
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 15000)
    return () => clearInterval(interval)
  }, [fetchData])

  const stats = useMemo(() => {
    const networks = data?.networks || []
    const total = networks.length
    const unused = networks.filter(n => n.status === 'Unused').length
    return { total, unused }
  }, [data])

  const getDriverColor = (driver: string) => {
    switch (driver.toLowerCase()) {
      case 'bridge': return 'border-blue-500/20 bg-blue-500/10 text-blue-600'
      case 'host': return 'border-orange-500/20 bg-orange-500/10 text-orange-600'
      case 'overlay': return 'border-purple-500/20 bg-purple-500/10 text-purple-600'
      default: return 'border-slate-200 bg-slate-100 text-slate-600'
    }
  }

  if (isLoading && (!data?.networks || data.networks.length === 0)) {
    return (
      <div className="flex flex-col items-center justify-center h-[60vh] gap-6">
        <Loader2 className="w-12 h-12 text-primary animate-spin" />
        <p className="text-muted-foreground text-xs font-bold uppercase tracking-widest animate-pulse">Mapping Virtual Networks</p>
      </div>
    )
  }

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6 pb-4 border-b">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">Networks</h1>
          <p className="text-muted-foreground">Manage isolated networks for student projects.</p>
        </div>

        <div className="flex items-center gap-6">
          <div className="flex items-center gap-2 bg-muted/30 border p-2 rounded-xl text-xs font-bold uppercase tracking-widest text-muted-foreground">
            <div className="flex items-center gap-2 px-3 border-r">
              <div className="w-2.5 h-2.5 rounded-full bg-emerald-500 shadow-sm animate-pulse" />
              {stats.total} Active Pipes
            </div>
            <div className="flex items-center gap-2 px-3">
              <div className="w-2.5 h-2.5 rounded-full bg-slate-400 shadow-sm" />
              {stats.unused} Standby
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Button size="sm">
              <Plus className="w-4 h-4 mr-2" /> New Network
            </Button>
            <Button variant="outline" size="icon" onClick={fetchData} className="w-9 h-9">
              <RotateCw className="w-4 h-4 text-muted-foreground" />
            </Button>
          </div>
        </div>
      </div>

      <Card>
        <div className="overflow-x-auto min-h-[400px]">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-12 text-center">
                  <Checkbox />
                </TableHead>
                <TableHead>Interface Identity</TableHead>
                <TableHead className="text-center">Connection State</TableHead>
                <TableHead className="text-center">Protocol / Driver</TableHead>
                <TableHead className="text-center">Exposure Scope</TableHead>
                <TableHead className="text-right">Action</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(!data?.networks || data.networks.length === 0) ? (
                <TableRow>
                  <TableCell colSpan={6} className="h-64 text-center">
                    <div className="flex flex-col items-center justify-center text-muted-foreground">
                      <div className="w-16 h-16 bg-muted/50 rounded-full flex items-center justify-center mb-4">
                        <Share2 className="w-8 h-8 opacity-50" />
                      </div>
                      <span className="font-semibold text-sm">No networks mapped in cluster</span>
                    </div>
                  </TableCell>
                </TableRow>
              ) : (
                data.networks.map((n) => (
                <TableRow key={n.id}>
                  <TableCell className="text-center">
                    <Checkbox />
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-4">
                      <div className="w-10 h-10 rounded-lg flex items-center justify-center bg-muted text-muted-foreground">
                        <Share2 className="w-5 h-5" />
                      </div>
                      <div className="flex flex-col">
                        <span className="text-sm font-semibold truncate max-w-[200px] uppercase">
                          {n.name}
                        </span>
                        <span className="text-xs text-muted-foreground font-mono truncate max-w-[200px]">
                          {n.id.substring(0, 12)}
                        </span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="text-center">
                    {n.status === 'In Use' ? (
                      <Badge variant="outline" className="text-emerald-600 border-emerald-500/40 bg-emerald-500/10">
                        <div className="w-1.5 h-1.5 rounded-full bg-emerald-500 mr-1.5 animate-pulse" /> Routed
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="text-indigo-600 border-indigo-500/40 bg-indigo-500/10">
                        <div className="w-1.5 h-1.5 rounded-full bg-indigo-500 mr-1.5" /> Isolated
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell className="text-center">
                    <Badge variant="outline" className={`font-mono text-[10px] ${getDriverColor(n.driver)}`}>
                      {n.driver}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-center">
                    <Badge variant="secondary" className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 hover:bg-emerald-500/20 font-mono text-[10px] gap-1">
                      <Globe className="w-3 h-3" />
                      {n.scope}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-right">
                    <DropdownMenu>
                      <DropdownMenuTrigger className="inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium ring-offset-background transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 hover:bg-accent hover:text-accent-foreground h-8 w-8">
                        <span className="sr-only">Open menu</span>
                        <MoreHorizontal className="h-4 w-4" />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem>Inspect Configuration</DropdownMenuItem>
                        <DropdownMenuItem>Connect Container</DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem className="text-destructive">Disconnect Network</DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              )))}
            </TableBody>
          </Table>
        </div>

        <div className="p-4 border-t flex flex-col md:flex-row items-center justify-between gap-4 bg-muted/10">
          <div className="flex items-center gap-2 text-sm text-muted-foreground font-medium">
            <Activity className="w-4 h-4 text-emerald-500" />
            Showing {data?.networks?.length || 0} virtual interfaces found in cluster.
          </div>
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <span className="text-xs font-bold text-muted-foreground uppercase tracking-widest">Rows per page</span>
              <Select defaultValue="20">
                <SelectTrigger className="w-[80px] h-8">
                  <SelectValue placeholder="20" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="20">20</SelectItem>
                  <SelectItem value="50">50</SelectItem>
                  <SelectItem value="all">All</SelectItem>
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

export default memo(AdminNetworks)

