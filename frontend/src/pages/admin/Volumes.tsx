import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '@/services/api'
import useTranslation from '@/lib/useTranslation'
import {
    Plus,
    RotateCw,
    HardDrive,
    MoreHorizontal,
    Zap,
    Loader2,
    Trash2,
    Info,
    ChevronLeft,
    ChevronRight
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
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
            <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6 pb-4 border-b">
                <div>
                    <h1 className="text-3xl font-bold tracking-tight mb-2">{t('admin.volumes.title')}</h1>
                    <p className="text-muted-foreground">{t('admin.volumes.desc')}</p>
                </div>

                <div className="flex items-center gap-6">
                    <div className="flex items-center gap-2 bg-muted/30 border p-2 rounded-xl text-xs font-bold uppercase tracking-widest text-muted-foreground">
                        <div className="flex items-center gap-2 px-3 border-r">
                            <div className="w-2.5 h-2.5 rounded-full bg-blue-500 shadow-sm animate-pulse"></div>
                            {stats.total} {t('admin.volumes.active')}
                        </div>
                        <div className="flex items-center gap-2 px-3">
                            <div className="w-2.5 h-2.5 rounded-full bg-amber-500 shadow-sm"></div>
                            {stats.unused} {t('admin.volumes.orphaned')}
                        </div>
                    </div>

                    <div className="flex items-center gap-2">
                        <Button>
                            <Plus className="w-4 h-4 mr-2" /> {t('common.create')}
                        </Button>
                        <Button variant="outline" size="icon" onClick={fetchData} className="w-9 h-9">
                            <RotateCw className="w-4 h-4 text-muted-foreground" />
                        </Button>
                    </div>
                </div>
            </div>

            {/* Table Area */}
            <div className="overflow-hidden rounded-md border">
                <div className="overflow-x-auto">
                    <Table>
                        <TableHeader>
                            <TableRow>
                                <TableHead className="w-10">
                                    <Checkbox />
                                </TableHead>
                                <TableHead>{t('admin.volumes.identity')}</TableHead>
                                <TableHead className="text-center w-32">{t('common.status')}</TableHead>
                                <TableHead className="text-center w-32">{t('admin.volumes.capacity')}</TableHead>
                                <TableHead className="text-center w-40">{t('admin.volumes.orchestrator')}</TableHead>
                                <TableHead className="text-right w-20">{t('common.actions')}</TableHead>
                            </TableRow>
                        </TableHeader>
                        <TableBody>
                            {(!data?.volumes || data.volumes.length === 0) ? (
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
                                data.volumes.map((v) => (
                                    <TableRow key={v.name}>
                                        <TableCell className="text-center">
                                            <Checkbox />
                                        </TableCell>
                                        <TableCell>
                                            <div className="flex items-center gap-4">
                                                <div className="w-10 h-10 rounded-lg bg-muted flex items-center justify-center text-muted-foreground">
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
                                        <TableCell>
                                            <Badge variant="outline" className={`gap-1.5 ${v.status === 'In Use'
                                                ? 'bg-emerald-500/10 text-emerald-600 border-emerald-500/40'
                                                : 'bg-indigo-500/10 text-indigo-600 border-indigo-500/40'
                                                }`}>
                                                <div className={`w-1.5 h-1.5 rounded-full ${v.status === 'In Use' ? 'bg-emerald-500 animate-pulse' : 'bg-indigo-500'}`} />
                                                {v.status === 'In Use' ? t('status.inUse') : (v.status || t('status.ready'))}
                                            </Badge>
                                        </TableCell>
                                        <TableCell className="text-center">
                                            <Badge variant="secondary" className="font-mono text-xs">
                                                {v.size || 'N/A'}
                                            </Badge>
                                        </TableCell>
                                        <TableCell className="text-center">
                                            <Badge variant="outline" className="text-[10px] uppercase font-bold text-muted-foreground bg-muted/30">
                                                {v.driver}
                                            </Badge>
                                        </TableCell>
                                        <TableCell className="text-right">
                                            <DropdownMenu>
                                                <DropdownMenuTrigger render={<Button variant="ghost" size="icon" className="h-8 w-8" />}>
                                                    <MoreHorizontal className="w-4 h-4" />
                                                </DropdownMenuTrigger>
                                                <DropdownMenuContent align="end" className="w-fit">
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

                <div className="p-4 border-t flex flex-col md:flex-row items-center justify-between gap-4 bg-muted/10">
                    <div className="flex items-center gap-2 text-sm text-muted-foreground font-medium">
                        <Zap className="w-4 h-4 text-primary" />
                        {t('common.total')}: {data?.volumes?.length || 0} {t('admin.volumes.clusters')}
                    </div>
                    <div className="flex items-center gap-2">
                        <Button variant="outline" size="sm" disabled>
                            <ChevronLeft className="w-4 h-4 mr-1" /> {t('common.previous')}
                        </Button>
                        <Button variant="outline" size="sm" disabled>
                            {t('common.next')} <ChevronRight className="w-4 h-4 ml-1" />
                        </Button>
                    </div>
                </div>
            </div>
        </div>
    )
}

export default memo(AdminVolumes)
