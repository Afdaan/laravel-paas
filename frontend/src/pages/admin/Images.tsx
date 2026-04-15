import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '@/services/api'
import useTranslation from '@/lib/useTranslation'
import { toast } from 'sonner'
import {
  Download,
  Trash2,
  Search,
  BarChart2,
  RefreshCw,
  Box,
  User,
  MoreHorizontal,
  ShieldCheck,
  Loader2,
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

interface ImageData {
  id: string;
  repository: string;
  tag: string;
  status: string;
  size_human: string;
}

const AdminImages = () => {
  const { t } = useTranslation()
  const [data, setData] = useState<{ images: ImageData[], system: any }>({
    images: [],
    system: null
  })
  const [isLoading, setIsLoading] = useState(true)
  const [isPruning, setIsPruning] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')
  const [limit, setLimit] = useState('all')
  const [page, setPage] = useState(1)

  const fetchData = useCallback(async () => {
    try {
      const res = await systemAPI.getStats()
      setData(res.data)
    } catch (error) {
      console.error('Failed to fetch images:', error)
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

  const handlePrune = useCallback(async () => {
    if (!window.confirm(t('admin.purgeMessage'))) return
    setIsPruning(true)
    try {
      await systemAPI.prune()
      toast.success(t('admin.optiSuccess'))
      fetchData()
    } catch (error) {
      toast.error(t('admin.optiFailed'))
    } finally {
      setIsPruning(false)
    }
  }, [fetchData, t])

  const filteredImages = useMemo(() => {
    const list = data?.images || []
    const filtered = list.filter(img =>
      (img.repository || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
      (img.tag || '').toLowerCase().includes(searchQuery.toLowerCase())
    )

    if (limit === 'all') return filtered
    const numLimit = parseInt(limit)
    return filtered.slice((page - 1) * numLimit, page * numLimit)
  }, [data, searchQuery, limit, page])

  const stats = useMemo(() => {
    const images = data?.images || []
    const total = images.length
    let totalSize = 0
    images.forEach(img => {
      const match = (img.size_human || '').match(/(\d+\.?\d*)\s*(GB|MB|KB|B)/i)
      if (match) {
        let val = parseFloat(match[1])
        const unit = match[2].toUpperCase()
        if (unit === 'GB') val *= 1024
        if (unit === 'KB') val /= 1024
        if (unit === 'B') val /= 1024 / 1024
        totalSize += val
      }
    })
    return { total, totalSize: (totalSize / 1024).toFixed(2) + ' GB' }
  }, [data])

  if (isLoading && (!data?.images || data.images.length === 0)) {
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
          <h1 className="text-3xl font-bold tracking-tight mb-2">{t('admin.images.title')}</h1>
          <p className="text-muted-foreground">{t('admin.images.desc')}</p>
        </div>

        <div className="flex items-center gap-6">
          <div className="flex items-center gap-2 bg-muted/30 border p-2 rounded-xl text-xs font-bold uppercase tracking-widest text-muted-foreground">
            <div className="flex items-center gap-2 px-4 border-r">
              <div className="w-2.5 h-2.5 rounded-full bg-blue-500 shadow-sm animate-pulse" />
              <span className="ml-1">{stats.total} {t('common.images')}</span>
            </div>
            <div className="flex items-center gap-2 px-4 shadow-sm">
              <div className="w-2.5 h-2.5 rounded-full bg-indigo-500 shadow-sm" />
              <span className="ml-1">{stats.totalSize} {t('common.storage')}</span>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" className="h-9">
              <Download className="w-4 h-4 mr-2" /> {t('admin.pullImage')}
            </Button>
            <Button disabled={isPruning} variant="destructive" size="sm" onClick={handlePrune} className="h-9">
              {isPruning ? <Loader2 className="w-4 h-4 mr-2 animate-spin" /> : <Trash2 className="w-4 h-4 mr-2" />}
              {isPruning ? t('admin.optimizing') : t('admin.pruneUnused')}
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
            <Button variant="outline" size="sm" className="hidden xl:flex h-9">
              <BarChart2 className="w-4 h-4 mr-2" /> {t('common.analytics')}
            </Button>
            <Button variant="outline" size="icon" onClick={fetchData} className="w-10 h-10" title="Refresh">
              <RefreshCw className="w-4 h-4 text-muted-foreground" />
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
                <TableHead className="h-12 px-4 text-xs font-semibold uppercase tracking-wider">{t('admin.images.repository')}</TableHead>
                <TableHead className="h-12 px-4 text-center text-xs font-semibold uppercase tracking-wider">{t('admin.images.tag')}</TableHead>
                <TableHead className="h-12 px-4 text-center text-xs font-semibold uppercase tracking-wider">{t('admin.images.lifecycle')}</TableHead>
                <TableHead className="h-12 px-4 text-xs font-semibold uppercase tracking-wider">{t('admin.images.orchestratedBy')}</TableHead>
                <TableHead className="h-12 px-4 text-center text-xs font-semibold uppercase tracking-wider">{t('admin.images.scan')}</TableHead>
                <TableHead className="h-12 px-4 text-center text-xs font-semibold uppercase tracking-wider">{t('admin.images.size')}</TableHead>
                <TableHead className="h-12 pl-4 pr-6 text-right text-xs font-semibold uppercase tracking-wider">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredImages.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={8} className="h-64 text-center">
                    <div className="flex flex-col items-center justify-center text-muted-foreground">
                      <div className="w-16 h-16 bg-muted/50 rounded-full flex items-center justify-center mb-4">
                        <Search className="w-8 h-8 opacity-50" />
                      </div>
                      <span className="font-semibold text-sm">{t('common.noData')}</span>
                    </div>
                  </TableCell>
                </TableRow>
              ) : filteredImages.map((img, i) => (
                <TableRow key={img.id || i} className="hover:bg-muted/20">
                  <TableCell className="px-4 py-3 text-center">
                    <Checkbox />
                  </TableCell>
                  <TableCell className="px-4 py-3">
                    <div className="flex items-center gap-4">
                      <div className="w-10 h-10 rounded-lg flex items-center justify-center bg-muted/50 text-muted-foreground">
                        <Box className="w-5 h-5" />
                      </div>
                      <div className="flex flex-col">
                        <span className="text-sm font-semibold truncate max-w-[200px] uppercase">
                          {img.repository}
                        </span>
                        <span className="text-xs text-muted-foreground font-mono truncate max-w-[200px]">
                          {img.id && img.id.length > 7 ? img.id.substring(7, 19) : 'untagged'}
                        </span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="px-4 py-3 text-center">
                    <Badge variant="secondary" className="px-2 py-0.5 text-[10px] font-mono bg-muted/50">
                      {img.tag}
                    </Badge>
                  </TableCell>
                  <TableCell className="px-4 py-3 text-center">
                    {img.status === 'In Use' ? (
                      <Badge variant="outline" className="text-blue-600 border-blue-500/40 bg-blue-500/10">
                        <div className="w-1.5 h-1.5 rounded-full bg-blue-500 mr-1.5 animate-pulse" /> {t('admin.used')}
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="text-muted-foreground font-medium">
                        <div className="w-1.5 h-1.5 rounded-full bg-slate-400 mr-1.5" /> {t('admin.unused')}
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell className="px-4 py-3">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-full bg-muted/50 flex items-center justify-center text-muted-foreground">
                        <User className="w-4 h-4" />
                      </div>
                      <span className="text-xs font-bold text-muted-foreground uppercase tracking-tight">
                        {img.repository?.split('/').pop() || 'System Arc'}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell className="px-4 py-3 text-center">
                    <div className="flex justify-center">
                      <div className="w-8 h-8 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center">
                        <ShieldCheck className="w-4 h-4 text-emerald-600" />
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="px-4 py-3 text-center">
                    <Badge variant="secondary" className="font-mono bg-muted/50 text-[10px] px-2">
                      {img.size_human}
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
                          <Info className="w-4 h-4 text-muted-foreground" /> {t('common.inspect')}
                        </DropdownMenuItem>
                        <DropdownMenuItem className="gap-2 cursor-pointer">
                          <RotateCw className="w-4 h-4 text-muted-foreground" /> {t('admin.images.retag')}
                        </DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem className="text-destructive gap-2 cursor-pointer">
                          <Trash2 className="w-4 h-4" /> {t('admin.images.deleteImage')}
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
              Showing {filteredImages.length} of {stats.total} nodes.
            </div>
            <div className="flex items-center space-x-2">
              <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">Rows per page</p>
              <Select value={limit} onValueChange={(val) => { if (val) { setLimit(val); setPage(1); } }}>
                <SelectTrigger className="h-8 w-[82px] justify-between">
                  <SelectValue placeholder={t('common.all')}>
                    {limit === 'all' ? t('common.all') : limit}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent side="top" align="end" className="min-w-[100px]">
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

export default memo(AdminImages)
