import React, { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '../../services/api'
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
  Zap,
  Loader2
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'

interface ImageData {
  id: string;
  repository: string;
  tag: string;
  status: string;
  size_human: string;
}

const AdminImages = () => {
  const [data, setData] = useState<{ images: ImageData[], system: any }>({
    images: [],
    system: null
  })
  const [isLoading, setIsLoading] = useState(true)
  const [isPruning, setIsPruning] = useState(false)
  const [searchQuery, setSearchQuery] = useState('')

  const fetchData = useCallback(async () => {
    try {
      const res = await systemAPI.getStats()
      setData(res.data)
    } catch (error) {
      console.error('Failed to fetch images:', error)
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 15000)
    return () => clearInterval(interval)
  }, [fetchData])

  const handlePrune = useCallback(async () => {
    if (!window.confirm('Are you sure you want to purge unused image layers? This action is irreversible.')) return
    setIsPruning(true)
    try {
      await systemAPI.prune()
      toast.success('Registry optimization complete')
      fetchData()
    } catch (error) {
      toast.error('Optimization failed')
    } finally {
      setIsPruning(false)
    }
  }, [fetchData])

  const filteredImages = useMemo(() => {
    return data.images.filter(img => 
      img.repository.toLowerCase().includes(searchQuery.toLowerCase()) ||
      img.tag.toLowerCase().includes(searchQuery.toLowerCase())
    )
  }, [data.images, searchQuery])

  const stats = useMemo(() => {
    const total = data.images.length
    let totalSize = 0
    data.images.forEach(img => {
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
  }, [data.images])

  if (isLoading && data.images.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-[60vh] gap-6">
        <Loader2 className="w-12 h-12 text-primary animate-spin" />
        <p className="text-muted-foreground text-xs font-bold uppercase tracking-widest animate-pulse">Indexing Image Registry</p>
      </div>
    )
  }

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6 pb-4 border-b">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">Images</h1>
          <p className="text-muted-foreground">Manage and optimize project image snapshots.</p>
        </div>

        <div className="flex items-center gap-6">
          <div className="flex items-center gap-2 bg-muted/30 border p-2 rounded-xl text-xs font-bold uppercase tracking-widest text-muted-foreground">
            <div className="flex items-center gap-2 px-3 border-r">
              <div className="w-2.5 h-2.5 rounded-full bg-blue-500 shadow-sm animate-pulse" />
              {stats.total} Images
            </div>
            <div className="flex items-center gap-2 px-3">
              <div className="w-2.5 h-2.5 rounded-full bg-indigo-500 shadow-sm" />
              {stats.totalSize} Storage
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm">
              <Download className="w-4 h-4 mr-2" /> Pull Image
            </Button>
            <Button disabled={isPruning} variant="destructive" size="sm" onClick={handlePrune}>
              {isPruning ? <Loader2 className="w-4 h-4 mr-2 animate-spin" /> : <Trash2 className="w-4 h-4 mr-2" />}
              {isPruning ? 'Optimizing...' : 'Prune Unused'}
            </Button>
          </div>
        </div>
      </div>

      <Card>
        <div className="p-4 border-b flex flex-col md:flex-row items-center justify-between gap-4">
          <div className="relative flex-1 w-full max-w-xl">
            <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <Input 
              placeholder="Search registry manifests..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="pl-9 w-full"
            />
          </div>
          <div className="flex items-center gap-3">
            <Button variant="outline" size="sm" className="hidden xl:flex">
              <BarChart2 className="w-4 h-4 mr-2" /> Analytics
            </Button>
            <Button variant="outline" size="icon" onClick={fetchData} className="w-9 h-9">
              <RefreshCw className="w-4 h-4 text-muted-foreground" />
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
                <TableHead>Image Repository</TableHead>
                <TableHead className="text-center">Tag</TableHead>
                <TableHead className="text-center">Lifecycle</TableHead>
                <TableHead>Orchestrated By</TableHead>
                <TableHead className="text-center">Scan</TableHead>
                <TableHead className="text-center">Size</TableHead>
                <TableHead className="text-right">Action</TableHead>
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
                      <span className="font-semibold text-sm">No manifests found in current context</span>
                    </div>
                  </TableCell>
                </TableRow>
              ) : filteredImages.map((img, i) => (
                <TableRow key={img.id || i}>
                  <TableCell className="text-center">
                    <Checkbox />
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-4">
                      <div className="w-10 h-10 rounded-lg flex items-center justify-center bg-muted text-muted-foreground">
                        <Box className="w-5 h-5" />
                      </div>
                      <div className="flex flex-col">
                        <span className="text-sm font-semibold truncate max-w-[200px] uppercase">
                          {img.repository}
                        </span>
                        <span className="text-xs text-muted-foreground font-mono truncate max-w-[200px]">
                          {img.id?.substring(7, 19) || 'untagged'}
                        </span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="text-center">
                    <Badge variant="secondary" className="px-2 py-0.5 text-[10px] font-mono">
                      {img.tag}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-center">
                    {img.status === 'In Use' ? (
                      <Badge variant="outline" className="text-blue-600 border-blue-500/40 bg-blue-500/10">
                        <div className="w-1.5 h-1.5 rounded-full bg-blue-500 mr-1.5 animate-pulse" /> {img.status}
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="text-muted-foreground">
                        <div className="w-1.5 h-1.5 rounded-full bg-slate-400 mr-1.5" /> {img.status || 'Unused'}
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-3">
                      <div className="w-6 h-6 rounded-full bg-muted flex items-center justify-center text-muted-foreground">
                        <User className="w-3 h-3" />
                      </div>
                      <span className="text-xs font-bold text-muted-foreground uppercase tracking-tight">
                        {img.repository?.split('/').pop() || 'System Arc'}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell className="text-center">
                    <div className="flex justify-center">
                      <div className="w-6 h-6 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center">
                        <ShieldCheck className="w-3.5 h-3.5 text-emerald-600" />
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="text-center">
                    <Badge variant="secondary" className="font-mono bg-muted text-[10px]">
                      {img.size_human}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-right">
                    <DropdownMenu>
                      <DropdownMenuTrigger className="inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium ring-offset-background transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 hover:bg-accent hover:text-accent-foreground h-8 w-8">
                        <span className="sr-only">Open menu</span>
                        <MoreHorizontal className="h-4 w-4" />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuLabel>Actions</DropdownMenuLabel>
                        <DropdownMenuItem>Inspect</DropdownMenuItem>
                        <DropdownMenuItem>Re-tag</DropdownMenuItem>
                        <DropdownMenuSeparator />
                        <DropdownMenuItem className="text-destructive">Delete Image</DropdownMenuItem>
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
            <Zap className="w-4 h-4 text-blue-500" />
            Showing {filteredImages.length} results of registry state.
          </div>
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              <span className="text-xs font-bold text-muted-foreground uppercase tracking-widest">Rows per page</span>
              <Select defaultValue="all">
                <SelectTrigger className="w-[80px] h-8">
                  <SelectValue placeholder="All" />
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

export default memo(AdminImages)

