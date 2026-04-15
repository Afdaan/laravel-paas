import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '@/services/api'
import useTranslation from '@/lib/useTranslation'
import {
    Plus,
    RotateCw,
    HardDrive,
    MoreHorizontal,
    Loader2,
    Trash2,
    Info,
    ChevronLeft,
    ChevronRight,
    ChevronsLeft,
    ChevronsRight,
    Search
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { Input } from '@/components/ui/input'
import { Card } from '@/components/ui/card'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { toast } from 'sonner'

interface VolumeData {
    name: string;
    driver: string;
    mount_point: string;
    status: string;
    size?: string;
}

const AdminVolumes = () => {
    const { t } = useTranslation()
    const [data, setData] = useState<{ volumes: VolumeData[] }>({
        volumes: []
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
            console.error('Failed to fetch volumes:', error)
            toast.error(t('admin.volumes.syncError'))
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
        const volumes = data?.volumes || []
        const total = volumes.length
        const unused = volumes.filter(v => v.status === 'Unused' || v.status === 'Orphaned').length
        return { total, unused }
    }, [data])

    const filteredVolumes = useMemo(() => {
        const list = data?.volumes || []
        const filtered = list.filter(v =>
            (v.name || '').toLowerCase().includes(searchQuery.toLowerCase())
        )

        if (limit === 'all') return filtered
        const numLimit = parseInt(limit)
        return filtered.slice((page - 1) * numLimit, page * numLimit)
    }, [data, searchQuery, limit, page])

    const handleDelete = async (name: string) => {
        if (!window.confirm(t('admin.volumes.confirmPurge', { name }))) return
        try {
            await systemAPI.deleteVolume(name)
            toast.success(t('admin.volumes.purgeInitiated'))
            fetchData()
        } catch (error) {
            toast.error(t('admin.volumes.purgeError'))
        }
    }

    if (isLoading && (!data?.volumes || data.volumes.length === 0)) {
        return (
            <div className="flex flex-col items-center justify-center h-[60vh] gap-6">
                <Loader2 className="w-12 h-12 text-primary animate-spin" />
                <p className="text-muted-foreground text-xs font-bold uppercase tracking-widest animate-pulse">{t('common.loading')}</p>
            </div>
        )
    }

    return (
        <div className="space-y-8 animate-in fade-in duration-500 pb-10">
            {/* Header Area */}
            <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6">
                <div>
                    <h1 className="text-3xl font-bold tracking-tight mb-2">{t('admin.volumes.title')}</h1>
                    <p className="text-muted-foreground">{t('admin.volumes.desc')}</p>
                </div>

                <div className="flex items-center gap-6">
                    <div className="flex items-center gap-2 bg-muted/30 border p-2 rounded-xl text-xs font-bold uppercase tracking-widest text-muted-foreground">
                        <div className="flex items-center gap-2 px-4 border-r">
                            <div className="w-2.5 h-2.5 rounded-full bg-blue-500 shadow-sm animate-pulse"></div>
                            <span className="ml-1">{stats.total} {t('admin.volumes.active')}</span>
                        </div>
                        <div className="flex items-center gap-2 px-4 shadow-sm">
                            <div className="w-2.5 h-2.5 rounded-full bg-amber-500 shadow-sm"></div>
                            <span className="ml-1">{stats.unused} {t('admin.volumes.orphaned')}</span>
                        </div>
                    </div>

                    <div className="flex items-center gap-2">
                        <Button className="h-9">
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
                                <TableHead className="h-12 px-4 text-xs font-semibold uppercase tracking-wider">{t('admin.volumes.identity')}</TableHead>
                                <TableHead className="h-12 px-4 text-center text-xs font-semibold uppercase tracking-wider w-32">{t('common.status')}</TableHead>
                                <TableHead className="h-12 px-4 text-center text-xs font-semibold uppercase tracking-wider w-32">{t('admin.volumes.capacity')}</TableHead>
                                <TableHead className="h-12 px-4 text-center text-xs font-semibold uppercase tracking-wider w-40">{t('admin.volumes.orchestrator')}</TableHead>
                                <TableHead className="h-12 pl-4 pr-6 text-right text-xs font-semibold uppercase tracking-wider w-24">{t('common.actions')}</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {(!filteredVolumes || filteredVolumes.length === 0) ? (
                                <TableRow>
                                    <TableCell colSpan={6} className="h-64 text-center">
                                        <div className="flex flex-col items-center justify-center text-muted-foreground">
                                            <div className="w-16 h-16 bg-muted/50 rounded-full flex items-center justify-center mb-4">
                                                <HardDrive className="w-8 h-8 opacity-50" />
                                            </div>
                                            <span className="font-semibold text-sm">{t('admin.volumes.noVolumes')}</span>
                                        </div>
                                    </TableCell>
                                </TableRow>
                            ) : (
                                filteredVolumes.map((v) => (
                                    <TableRow key={v.name} className="hover:bg-muted/20">
                                        <TableCell className="px-4 py-3 text-center">
                                            <Checkbox />
                                        </TableCell>
                                        <TableCell className="px-4 py-3">
                                            <div className="flex items-center gap-4">
                                                <div className="w-10 h-10 rounded-lg bg-muted/50 flex items-center justify-center text-muted-foreground">
                                                    <HardDrive className="w-5 h-5" />
                                                </div>
                                                <div className="flex flex-col">
                                                    <span className="text-sm font-semibold truncate max-w-[300px] uppercase tracking-tight">{v.name}</span>
                                                    {v.name && v.name.length > 0 && (
                                                        <span className="text-[10px] text-muted-foreground font-mono tracking-widest">
                                                          {v.name.length > 16 ? `${v.name.substring(0, 16)}...` : v.name}
                                                        </span>
                                                    )}
                                                </div>
                                            </div>
                                        </TableCell>
                                        <TableCell className="px-4 py-3">
                                            <Badge variant="outline" className={`gap-1.5 ${v.status === 'In Use'
                                                ? 'bg-emerald-500/10 text-emerald-600 border-emerald-500/40'
                                                : 'bg-indigo-500/10 text-indigo-600 border-indigo-500/40'
                                                }`}>
                                                <div className={`w-1.5 h-1.5 rounded-full ${v.status === 'In Use' ? 'bg-emerald-500 animate-pulse' : 'bg-indigo-500'}`} />
                                                {v.status === 'In Use' ? t('status.inUse') : (v.status || t('status.ready'))}
                                            </Badge>
                                        </TableCell>
                                        <TableCell className="px-4 py-3 text-center">
                                            <Badge variant="secondary" className="font-mono text-[10px] bg-muted/50 px-2">
                                                {v.size || 'N/A'}
                                            </Badge>
                                        </TableCell>
                                        <TableCell className="px-4 py-3 text-center">
                                            <Badge variant="outline" className="text-[10px] uppercase font-bold text-muted-foreground bg-muted/30">
                                                {v.driver}
                                            </Badge>
                                        </TableCell>
                                        <TableCell className="pl-4 pr-6 py-3 text-right">
                                            <DropdownMenu>
                                                <DropdownMenuTrigger>
                                                    <Button variant="ghost" size="icon" className="h-8 w-8 hover:bg-muted/50">
                                                        <MoreHorizontal className="w-4 h-4" />
                                                    </Button>
                                                </DropdownMenuTrigger>
                                                <DropdownMenuContent align="end" className="w-48">
                                                    <DropdownMenuItem className="gap-2 cursor-pointer">
                                                        <Info className="w-4 h-4 text-muted-foreground" /> {t('admin.volumes.inspectConfig')}
                                                    </DropdownMenuItem>
                                                    <DropdownMenuSeparator />
                                                    <DropdownMenuItem className="text-destructive gap-2 cursor-pointer" onClick={() => handleDelete(v.name)}>
                                                        <Trash2 className="w-4 h-4" /> {t('admin.volumes.purgeVolume')}
                                                    </DropdownMenuItem>
                                                </DropdownMenuContent>
                                            </DropdownMenu>
                                        </TableCell>
                                    </TableRow>
                                ))
                            )}
                        </TableBody>
                    </Table>
                </div>

                <div className="p-4 border-t flex flex-col sm:flex-row justify-between items-center gap-4 bg-muted/10">
                    <div className="flex items-center gap-6">
                        <div className="flex items-center gap-2 text-sm text-muted-foreground font-medium">
                            <Info className="w-4 h-4 text-primary" />
                            Showing {filteredVolumes.length} nodes.
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

export default memo(AdminVolumes)
