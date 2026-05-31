import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '@/services/api'
import useTranslation from '@/lib/useTranslation'
import {
  Plus,
  RotateCw,
  Share2,
  MoreHorizontal,
  Globe,
  Loader2,
  ChevronLeft,
  ChevronRight,
  ChevronsLeft,
  ChevronsRight,
  Info,
  Search,
  Unplug
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { toast } from 'sonner'

interface NetworkData {
  id: string;
  name: string;
  status: string;
  driver: string;
  scope: string;
}

const AdminNetworks = () => {
  const { t } = useTranslation()
  const [data, setData] = useState<{ networks: NetworkData[] }>({
    networks: []
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
      console.error('Failed to fetch networks:', error)
      toast.error(t('common.loadError'))
    } finally {
      setIsLoading(false)
    }
  }, [t])

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 15000)
    return () => clearInterval(interval)
  }, [fetchData])

  const stats = useMemo(() => {
    const networks = data?.networks || []
    const total = networks.length
    const unused = networks.filter(n => n.status === 'Unused' || n.status === 'Idle').length
    return { total, unused }
  }, [data])

  const filteredNetworks = useMemo(() => {
    const list = data?.networks || []
    const filtered = list.filter(n =>
      (n.name || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
      (n.id || '').toLowerCase().includes(searchQuery.toLowerCase())
    )

    if (limit === 'all') return filtered
    const numLimit = parseInt(limit)
    return filtered.slice((page - 1) * numLimit, page * numLimit)
  }, [data, searchQuery, limit, page])

  const getDriverColor = (driver: string) => {
    switch (driver?.toLowerCase()) {
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
        <p className="text-muted-foreground text-xs font-bold uppercase tracking-widest animate-pulse">{t('common.loading')}</p>
      </div>
    )
  }

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">{t('admin.networks.title')}</h1>
          <p className="text-muted-foreground">{t('admin.networks.desc')}</p>
        </div>

        <div className="flex items-center gap-6">
          <div className="flex items-center gap-2 bg-muted/30 border p-2 rounded-xl text-xs font-bold uppercase tracking-widest text-muted-foreground">
            <div className="flex items-center gap-2 px-4 border-r">
              <div className="w-2.5 h-2.5 rounded-full bg-emerald-500 shadow-sm animate-pulse" />
              <span className="ml-1">{stats.total} {t('common.active')}</span>
            </div>
            <div className="flex items-center gap-2 px-4 shadow-sm">
              <div className="w-2.5 h-2.5 rounded-full bg-slate-400 shadow-sm" />
              <span className="ml-1">{stats.unused} {t('common.offline')}</span>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Button size="sm" className="h-9">
              <Plus className="w-4 h-4 mr-2" /> {t('common.create')}
            </Button>
            <Button variant="outline" size="icon" onClick={fetchData} className="w-9 h-9" title="Refresh">
              <RotateCw className="w-4 h-4 text-muted-foreground" />
            </Button>
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
             <Button variant="outline" size="icon" onClick={fetchData} className="w-10 h-10" title="Refresh">
              <RotateCw className="w-4 h-4 text-muted-foreground" />
            </Button>
          </div>
        </div>

        <div className="overflow-x-auto">
          <Table className="min-w-[1100px] table-fixed">
            <TableHeader>
              <TableRow className="bg-muted/20 hover:bg-muted/20">
                <TableHead className="w-12 px-4 text-center">
                  <Checkbox />
                </TableHead>
                <TableHead className="h-12 px-4 text-xs font-semibold uppercase tracking-wider">{t('admin.networks.interfaceIdentity')}</TableHead>
                <TableHead className="h-12 px-4 text-center text-xs font-semibold uppercase tracking-wider">{t('admin.networks.connectionState')}</TableHead>
                <TableHead className="h-12 px-4 text-center text-xs font-semibold uppercase tracking-wider">{t('admin.networks.protocolDriver')}</TableHead>
                <TableHead className="h-12 px-4 text-center text-xs font-semibold uppercase tracking-wider">{t('admin.networks.exposureScope')}</TableHead>
                <TableHead className="h-12 pl-4 pr-6 text-right text-xs font-semibold uppercase tracking-wider">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(!filteredNetworks || filteredNetworks.length === 0) ? (
                <TableRow>
                  <TableCell colSpan={6} className="h-64 text-center">
                    <div className="flex flex-col items-center justify-center text-muted-foreground">
                      <div className="w-16 h-16 bg-muted/50 rounded-full flex items-center justify-center mb-4">
                        <Share2 className="w-8 h-8 opacity-50" />
                      </div>
                      <span className="font-semibold text-sm">{t('common.noData')}</span>
                    </div>
                  </TableCell>
                </TableRow>
              ) : (
                filteredNetworks.map((n) => (
                <TableRow key={n.id} className="hover:bg-muted/20">
                  <TableCell className="px-4 py-3 text-center">
                    <Checkbox />
                  </TableCell>
                  <TableCell className="px-4 py-3">
                    <div className="flex items-center gap-4">
                      <div className="w-10 h-10 rounded-lg flex items-center justify-center bg-muted/50 text-muted-foreground">
                        <Share2 className="w-5 h-5" />
                      </div>
                      <div className="flex flex-col">
                        <span className="text-sm font-semibold truncate max-w-[200px] uppercase tracking-tight">
                          {n.name}
                        </span>
                        <span className="text-[10px] text-muted-foreground font-mono truncate max-w-[200px]">
                          {n.id && n.id.length >= 12 ? n.id.substring(0, 12) : n.id}
                        </span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="px-4 py-3 text-center">
                    {n.status === 'In Use' ? (
                      <Badge variant="outline" className="text-emerald-600 border-emerald-500/40 bg-emerald-500/10">
                        <div className="w-1.5 h-1.5 rounded-full bg-emerald-500 mr-1.5 animate-pulse" /> {t('admin.networks.routed')}
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="text-indigo-600 border-indigo-500/40 bg-indigo-500/10">
                        <div className="w-1.5 h-1.5 rounded-full bg-indigo-500 mr-1.5" /> {t('admin.networks.isolated')}
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell className="px-4 py-3 text-center">
                    <Badge variant="outline" className={`font-mono text-[10px] bg-muted/30 ${getDriverColor(n.driver)}`}>
                      {n.driver}
                    </Badge>
                  </TableCell>
                  <TableCell className="px-4 py-3 text-center">
                    <Badge variant="secondary" className="bg-emerald-500/10 text-emerald-600 border-emerald-500/20 hover:bg-emerald-500/20 font-mono text-[10px] gap-1 px-2">
                      <Globe className="w-3 h-3" />
                      {n.scope}
                    </Badge>
                  </TableCell>
                  <TableCell className="pl-4 pr-6 py-3 text-right">
                    <DropdownMenu>
                      <DropdownMenuTrigger>
                        <Button variant="ghost" size="icon" className="h-8 w-8 hover:bg-muted/50">
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-48">
                        <DropdownMenuItem className="gap-2 cursor-pointer">
                          <Info className="w-4 h-4 text-muted-foreground" /> {t('admin.networks.inspectConfig')}
                        </DropdownMenuItem>
                        <DropdownMenuItem className="gap-2 cursor-pointer">
                          <Plus className="w-4 h-4 text-muted-foreground" /> {t('admin.networks.connectContainer')}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem className="text-destructive gap-2 cursor-pointer">
                          <Unplug className="w-4 h-4" /> {t('admin.networks.disconnectNetwork')}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </TableCell>
                </TableRow>
              )))}
            </TableBody>
          </Table>
        </div>

        <div className="p-4 border-t flex flex-col sm:flex-row justify-between items-center gap-4 bg-muted/10">
          <div className="flex items-center gap-6">
            <div className="flex items-center gap-2 text-sm text-muted-foreground font-medium">
              <Info className="w-4 h-4 text-primary" />
              Showing {filteredNetworks.length} nodes.
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

export default memo(AdminNetworks)
