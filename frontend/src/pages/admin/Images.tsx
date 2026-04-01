// ===========================================
// Admin Images Page (Infrastructure)
// ===========================================

import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '../../services/api'
import toast from 'react-hot-toast'
import { 
  Download, 
  Upload, 
  Trash2, 
  Search, 
  BarChart2, 
  RefreshCw, 
  Layers,
  Box,
  User,
  Check,
  MoreHorizontal,
  ShieldCheck,
  Zap,
  Info
} from 'lucide-react'

const AdminImages = () => {
    const [data, setData] = useState({
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
                <div className="relative">
                    
                    <div className="w-16 h-16 border-4 border-indigo-500/10 border-t-indigo-500 rounded-full animate-spin relative z-10"></div>
                </div>
                <p className="text-slate-600 dark:text-slate-400 text-[10px] font-black uppercase tracking-[0.3em] animate-pulse">Indexing Image Registry</p>
            </div>
        )
    }

    return (
        <div className="space-y-12 animate-pop-in relative h-full">
            {/* Background Glows */}
            

            <div className="relative z-10">
                {/* Header Area */}
                <div className="flex flex-col xl:flex-row xl:items-end justify-between mb-12 gap-8">
                    <div>
                        <h1 className="text-5xl font-black text-slate-900 dark:text-white tracking-tighter mb-4 italic text-transparent bg-clip-text pr-2 pb-1 bg-gradient-to-r from-blue-400 to-indigo-400">Images</h1>
                        <p className="text-slate-600 dark:text-slate-400 text-lg font-medium">Manage and optimize project image snapshots.</p>
                    </div>

                    <div className="flex items-center gap-6">
                        <div className="flex items-center gap-4 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 p-2 rounded-2xl backdrop-blur-md">
                            <div className="flex items-center gap-3 px-6 py-2 border-r border-slate-200 dark:border-white/5">
                                <div className="w-2.5 h-2.5 rounded-full bg-blue-500 shadow-[0_0_15px_rgba(59,130,246,0.5)] animate-pulse"></div>
                                <span className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-600 dark:text-slate-400">{stats.total} Images</span>
                            </div>
                            <div className="flex items-center gap-3 px-6 py-2">
                                <div className="w-2.5 h-2.5 rounded-full bg-indigo-500 shadow-[0_0_15px_rgba(99,102,241,0.5)]"></div>
                                <span className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-600 dark:text-slate-400">{stats.totalSize} Storage</span>
                            </div>
                        </div>

                        <div className="flex items-center gap-4">
                            <button className="btn btn-secondary py-3 px-6 text-sm font-black uppercase tracking-widest active:scale-95 transition-all">
                               <Download className="w-4 h-4" />
                               Pull Image
                            </button>
                            <button onClick={handlePrune} disabled={isPruning} className="btn bg-rose-500/10 hover:bg-rose-500/20 border-rose-500/20 text-rose-500 py-3 px-6 text-sm font-black uppercase tracking-widest disabled:opacity-50 active:scale-95 transition-all">
                               <Trash2 className="w-4 h-4" />
                               {isPruning ? 'Optimizing...' : 'Prune Unused'}
                            </button>
                        </div>
                    </div>
                </div>

                {/* Toolbar */}
                <div className="flex flex-col md:flex-row items-center justify-between mb-8 gap-6 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 p-4 rounded-3xl backdrop-blur-md shadow-2xl">
                    <div className="flex items-center gap-4 flex-1 w-full max-w-2xl">
                        <div className="relative flex-1 group">
                            <div className="absolute inset-y-0 left-0 pl-5 flex items-center pointer-events-none">
                                <Search className="w-4 h-4 text-slate-600 dark:text-slate-400 group-focus-within:text-blue-400 transition-colors" />
                            </div>
                            <input 
                                type="text"
                                placeholder="Search registry manifests..."
                                value={searchQuery}
                                onChange={(e) => setSearchQuery(e.target.value)}
                                className="w-full bg-black/40 border border-slate-200 dark:border-white/5 rounded-2xl py-3.5 pl-12 pr-5 text-sm font-medium focus:outline-none focus:ring-2 focus:ring-blue-500/20 focus:border-blue-500/40 transition-all placeholder:text-slate-600 outline-none"
                            />
                        </div>
                        <button className="hidden xl:flex items-center gap-3 px-5 py-3.5 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 rounded-2xl text-[10px] font-black uppercase tracking-widest text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white transition-all">
                            <BarChart2 className="w-4 h-4" />
                            Analytics
                        </button>
                    </div>

                    <div className="flex items-center gap-3">
                        <button onClick={fetchData} className="w-12 h-12 flex items-center justify-center rounded-2xl bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white transition-all">
                            <RefreshCw className="w-4 h-4" />
                        </button>
                    </div>
                </div>

                {/* Table Area */}
                <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm overflow-hidden border-slate-300 dark:border-white/10 shadow-[0_20px_50px_rgba(0,0,0,0.3)] bg-slate-50 dark:bg-slate-100 dark:bg-white/5">
                    <div className="overflow-x-auto">
                        <table className="premium-table">
                            <thead>
                                <tr>
                                    <th className="w-12 text-center">
                                        <div className="flex items-center justify-center">
                                            <input type="checkbox" className="w-4 h-4 rounded-md border-slate-300 dark:border-white/10 bg-black/40 text-blue-500 focus:ring-0 focus:ring-offset-0" />
                                        </div>
                                    </th>
                                    <th>Image Repository</th>
                                    <th className="text-center">Tag</th>
                                    <th className="text-center">Lifecycle</th>
                                    <th>Orchestrated By</th>
                                    <th className="text-center">Scan</th>
                                    <th className="text-center">Size</th>
                                    <th className="text-right">Action</th>
                                </tr>
                            </thead>
                            <tbody>
                                {filteredImages.map((img, i) => (
                                    <tr key={i} className="group hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5">
                                        <td className="text-center">
                                            <div className="flex items-center justify-center">
                                                <input type="checkbox" className="w-4 h-4 rounded-md border-slate-300 dark:border-white/10 bg-black/40 text-blue-500 focus:ring-0 focus:ring-offset-0" />
                                            </div>
                                        </td>
                                        <td>
                                            <div className="flex items-center gap-5">
                                                <div className="w-12 h-12 rounded-2xl bg-slate-100 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 flex items-center justify-center text-slate-600 dark:text-slate-400 group-hover:border-blue-500/40 group-hover:text-blue-400 group-hover:bg-blue-500/5 transition-all duration-500">
                                                    <Box className="w-6 h-6" />
                                                </div>
                                                <div className="flex flex-col">
                                                    <span className="text-sm font-black text-slate-900 dark:text-white group-hover:text-blue-400 transition-colors uppercase tracking-tight">{img.repository}</span>
                                                    <span className="text-[10px] text-slate-600 font-mono tracking-widest">{img.id?.substring(7, 19)}</span>
                                                </div>
                                            </div>
                                        </td>
                                        <td className="text-center">
                                            <span className="px-3 py-1.5 rounded-xl bg-slate-100 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 text-[10px] font-black text-slate-600 dark:text-slate-400 uppercase">{img.tag}</span>
                                        </td>
                                        <td className="text-center">
                                            <span className={`px-4 py-1.5 rounded-full border text-[9px] font-black uppercase tracking-widest inline-flex items-center gap-2 ${
                                                img.status === 'In Use' 
                                                ? 'bg-blue-500/5 text-blue-400 border-blue-500/20' 
                                                : 'bg-slate-500/5 text-slate-600 dark:text-slate-400 border-slate-200 dark:border-white/5'
                                            }`}>
                                                <div className={`w-1 h-1 rounded-full ${img.status === 'In Use' ? 'bg-blue-400 animate-pulse shadow-[0_0_5px_rgba(59,130,246,0.5)]' : 'bg-slate-500'}`} />
                                                {img.status}
                                            </span>
                                        </td>
                                        <td>
                                            <div className="flex items-center gap-3">
                                                <div className="w-6 h-6 rounded-full bg-slate-100 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 flex items-center justify-center text-slate-600 dark:text-slate-400">
                                                    <User className="w-3 h-3" />
                                                </div>
                                                <span className="text-[11px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-tight">{img.repository.split('/').pop() || 'System Arc'}</span>
                                            </div>
                                        </td>
                                        <td className="text-center">
                                            <div className="flex justify-center">
                                                <div className="w-6 h-6 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center shadow-[0_0_10px_rgba(16,185,129,0.1)]">
                                                    <ShieldCheck className="w-3.5 h-3.5 text-emerald-500" />
                                                </div>
                                            </div>
                                        </td>
                                        <td className="text-center">
                                            <span className="text-[11px] font-mono font-black text-slate-600 dark:text-slate-400 bg-slate-100 dark:bg-slate-100 dark:bg-white/5 px-3 py-1 rounded-lg border border-slate-200 dark:border-white/5 tracking-tighter">
                                                {img.size_human}
                                            </span>
                                        </td>
                                        <td className="text-right">
                                            <button className="p-2.5 hover:bg-slate-200 dark:bg-white/10 rounded-xl text-slate-600 hover:text-slate-900 dark:text-white transition-all active:scale-95">
                                                <MoreHorizontal className="w-5 h-5" />
                                            </button>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>

                    {filteredImages.length === 0 && (
                        <div className="py-32 flex flex-col items-center justify-center text-center">
                             <div className="w-24 h-24 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 rounded-[2.5rem] flex items-center justify-center mb-8 shadow-2xl">
                                <Search className="w-10 h-10 text-slate-800" />
                            </div>
                            <h3 className="text-[10px] font-black uppercase tracking-[0.4em] text-slate-600">No manifests found in current context</h3>
                        </div>
                    )}
                </div>

                {/* Pagination */}
                <div className="mt-8 flex flex-col md:flex-row items-center justify-between gap-6 px-10 py-8 bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border-slate-200 dark:border-white/5">
                    <div className="flex items-center gap-3">
                        <Zap className="w-4 h-4 text-blue-500" />
                        <span className="text-[10px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-widest leading-none">Showing {filteredImages.length} results of registry state.</span>
                    </div>
                    <div className="flex items-center gap-10">
                        <div className="flex items-center gap-4">
                            <span className="text-[10px] font-black text-slate-600 uppercase tracking-widest">Rows per page</span>
                            <select className="bg-black/40 border border-slate-300 dark:border-white/10 rounded-xl px-4 py-2 text-[10px] font-black text-slate-600 dark:text-slate-400 outline-none focus:border-blue-500 transition-all">
                                <option>20</option>
                                <option>50</option>
                                <option>All</option>
                            </select>
                        </div>
                        <div className="flex items-center gap-6">
                            <span className="text-[10px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-widest">Index 1 of 1</span>
                            <div className="flex items-center gap-2">
                                 <button className="w-10 h-10 flex items-center justify-center rounded-xl bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 text-slate-800 cursor-not-allowed transition-all">&lsaquo;</button>
                                 <button className="w-10 h-10 flex items-center justify-center rounded-xl bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 text-slate-800 cursor-not-allowed transition-all">&rsaquo;</button>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    )
}

export default memo(AdminImages)

