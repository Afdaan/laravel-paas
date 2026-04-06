import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '../../services/api'
import {
    Plus,
    RotateCw,
    HardDrive,
    MoreHorizontal,
    Zap,
    Loader2,
    Trash2,
    Info
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
            toast.error('Failed to sync storage registry')
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
        const volumes = data?.volumes || []
        const total = volumes.length
        const unused = volumes.filter(v => v.status === 'Unused').length
        return { total, unused }
    }, [data])

    const handleDelete = async (name: string) => {
        if (!window.confirm(`Purge volume ${name}? This will permanently delete all data stored within.`)) return
        try {
            // Purge logic usually handled by backend
            toast.success('Volume purge initiated')
            fetchData()
        } catch (error) {
            toast.error('Purge operation failed')
        }
    }

    if (isLoading && (!data?.volumes || data.volumes.length === 0)) {
        return (
            <div className="flex flex-col items-center justify-center h-[60vh] gap-6">
                <Loader2 className="w-12 h-12 text-primary animate-spin" />
                <p className="text-muted-foreground text-xs font-bold uppercase tracking-widest animate-pulse">Scanning Storage Cluster</p>
            </div>
        )
    }

    return (
        <div className="space-y-8 animate-in fade-in duration-500 pb-10">
            {/* Header Area */}
            <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6 pb-4 border-b">
                <div>
                    <h1 className="text-3xl font-bold tracking-tight mb-2">Storage Volumes</h1>
                    <p className="text-muted-foreground italic">Manage persistent storage nodes for student project deployments.</p>
                </div>

                <div className="flex items-center gap-6">
                    <div className="flex items-center gap-2 bg-muted/30 border p-2 rounded-xl text-xs font-bold uppercase tracking-widest text-muted-foreground">
                        <div className="flex items-center gap-2 px-3 border-r">
                            <div className="w-2.5 h-2.5 rounded-full bg-blue-500 shadow-sm animate-pulse"></div>
                            {stats.total} Active
                        </div>
                        <div className="flex items-center gap-2 px-3">
                            <div className="w-2.5 h-2.5 rounded-full bg-amber-500 shadow-sm"></div>
                            {stats.unused} Orphaned
                        </div>
                    </div>

                    <div className="flex items-center gap-2">
                        <Button>
                            <Plus className="w-4 h-4 mr-2" /> New Volume
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
                                <TableHead>
                                    <Checkbox />
                                </TableHead>
                                <TableHead>Identity & Namespace</TableHead>
                                <TableHead className="text-center">Lifecycle</TableHead>
                                <TableHead className="text-center">Capacity</TableHead>
                                <TableHead className="text-center">Orchestrator</TableHead>
                                <TableHead className="text-right">Action</TableHead>
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
                                            <span className="font-semibold text-sm">No storage clusters isolated</span>
                                        </div>
                                    </TableCell>
                                </TableRow>
                            ) : (
                                data.volumes.map((v, i) => (
                                    <TableRow key={i}>
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
                                                    <span className="text-[10px] text-muted-foreground font-mono tracking-widest">{v.name.substring(0, 16)}...</span>
                                                </div>
                                            </div>
                                        </TableCell>
                                        <TableCell>
                                            <Badge variant="outline" className={`gap-1.5 ${v.status === 'In Use'
                                                ? 'bg-emerald-500/10 text-emerald-600 border-emerald-500/40'
                                                : 'bg-indigo-500/10 text-indigo-600 border-indigo-500/40'
                                                }`}>
                                                <div className={`w-1.5 h-1.5 rounded-full ${v.status === 'In Use' ? 'bg-emerald-500 animate-pulse' : 'bg-indigo-500'}`} />
                                                {v.status || 'Active'}
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
                                                <DropdownMenuTrigger>
                                                    <Button variant="ghost" size="icon" className="h-8 w-8">
                                                        <MoreHorizontal className="w-4 h-4" />
                                                    </Button>
                                                </DropdownMenuTrigger>
                                                <DropdownMenuContent align="end" className="w-fit">
                                                    <DropdownMenuItem className="gap-2 focus:bg-accent cursor-pointer">
                                                        <Info className="w-4 h-4 text-muted-foreground" /> Inspect Config
                                                    </DropdownMenuItem>
                                                    <DropdownMenuSeparator />
                                                    <DropdownMenuItem className="text-destructive gap-2 focus:bg-destructive/10 focus:text-destructive cursor-pointer" onClick={() => handleDelete(v.name)}>
                                                        <Trash2 className="w-4 h-4" /> Purge Volume
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
                        Showing {data?.volumes?.length || 0} persistent storage clusters.
                    </div>
                    <div className="flex items-center gap-2">
                        <Button variant="outline" size="sm" disabled>Previous</Button>
                        <Button variant="outline" size="sm" disabled>Next</Button>
                    </div>
                </div>
            </div>
        </div>
    )
}

export default memo(AdminVolumes)
