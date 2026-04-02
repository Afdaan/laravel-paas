// ===========================================
// Admin Volumes Page (Infrastructure)
// ===========================================

import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '../../services/api'
import { 
  Plus, 
  RotateCw, 
  ChevronUp, 
  HardDrive, 
  MoreHorizontal,
  ChevronRight,
  ShieldCheck,
  AlertTriangle,
  Search,
  Zap
} from 'lucide-react'

const AdminVolumes = () => {
    const [data, setData] = useState({
        volumes: []
    })
    const [isLoading, setIsLoading] = useState(true)

    const fetchData = useCallback(async () => {
        try {
            const res = await systemAPI.getStats()
            setData(res.data)
        } catch (error) {
            console.error('Failed to fetch volumes:', error)
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
        const total = data.volumes.length
        const unused = data.volumes.filter(v => v.status === 'Unused').length
        return { total, unused }
    }, [data.volumes])

    if (isLoading && data.volumes.length === 0) {
        return (
            <div className="flex flex-col items-center justify-center h-[60vh] gap-6">
                <div className="w-16 h-16 border-4 border-indigo-500/10 border-t-indigo-500 rounded-full animate-spin"></div>
                <p className="text-slate-600 dark:text-slate-400 text-[10px] font-black uppercase tracking-[0.3em] animate-pulse">Scanning Storage Cluster</p>
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
                        <h1 className="text-5xl font-black text-slate-900 dark:text-white tracking-tighter mb-4 italic text-transparent bg-clip-text pr-2 pb-1 bg-gradient-to-r from-blue-400 to-indigo-400">Volumes</h1>
                        <p className="text-slate-600 dark:text-slate-400 text-lg font-medium">Manage persistent storage volumes for projects.</p>
                    </div>

                    <div className="flex items-center gap-6">
                        <div className="flex items-center gap-4 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 p-2 rounded-2xl backdrop-blur-md">
                            <div className="flex items-center gap-3 px-6 py-2 border-r border-slate-200 dark:border-white/5">
                                <div className="w-2.5 h-2.5 rounded-full bg-blue-500 shadow-[0_0_15px_rgba(59,130,246,0.5)] animate-pulse"></div>
                                <span className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-600 dark:text-slate-400">{stats.total} Active</span>
                            </div>
                            <div className="flex items-center gap-3 px-6 py-2">
                                <div className="w-2.5 h-2.5 rounded-full bg-amber-500 shadow-[0_0_15px_rgba(249,115,22,0.5)]"></div>
                                <span className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-600 dark:text-slate-400">{stats.unused} Orphaned</span>
                            </div>
                        </div>

                        <div className="flex items-center gap-4">
                            <button className="btn btn-secondary py-3 px-6 text-sm font-black uppercase tracking-widest shadow-xl shadow-indigo-500/10 active:scale-95 transition-all">
                               <Plus className="w-4 h-4" />
                               New Volume
                            </button>
                            <button onClick={fetchData} className="w-12 h-12 flex items-center justify-center bg-slate-50 dark:bg-slate-100 dark:bg-white/5 hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 rounded-2xl text-slate-900 dark:text-white transition-all shadow-lg active:rotate-180 transition-transform duration-500">
                               <RotateCw className="w-4 h-4" />
                            </button>
                        </div>
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
                                            <input type="checkbox" className="w-4 h-4 rounded-md border-slate-300 dark:border-white/10 bg-black/40 text-indigo-500 focus:ring-0 focus:ring-offset-0" />
                                        </div>
                                    </th>
                                    <th>Identity & Namespace</th>
                                    <th className="text-center">Lifecycle</th>
                                    <th className="text-center">Capacity</th>
                                    <th className="text-center">Revision</th>
                                    <th className="text-center">Orchestrator</th>
                                    <th className="text-right">Action</th>
                                </tr>
                            </thead>
                            <tbody>
                                {data.volumes.map((v, i) => (
                                    <tr key={i} className="group hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5">
                                        <td className="text-center">
                                            <div className="flex items-center justify-center">
                                                <input type="checkbox" className="w-4 h-4 rounded-md border-slate-300 dark:border-white/10 bg-black/40 text-indigo-500 focus:ring-0 focus:ring-offset-0" />
                                            </div>
                                        </td>
                                        <td>
                                            <div className="flex items-center gap-5">
                                                <div className="w-12 h-12 rounded-2xl bg-slate-100 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 flex items-center justify-center text-slate-600 dark:text-slate-400 group-hover:border-blue-500/40 group-hover:text-blue-400 group-hover:bg-blue-500/5 transition-all duration-500">
                                                    <HardDrive className="w-6 h-6" />
                                                </div>
                                                <div className="flex flex-col">
                                                    <span className="text-sm font-black text-slate-900 dark:text-white group-hover:text-blue-400 transition-colors truncate max-w-[300px] uppercase tracking-tight">{v.name}</span>
                                                    <span className="text-[10px] text-slate-600 font-mono tracking-widest">{v.name.substring(0, 12)}...</span>
                                                </div>
                                            </div>
                                        </td>
                                        <td className="text-center">
                                            <span className={`px-4 py-1.5 rounded-full border text-[9px] font-black uppercase tracking-widest inline-flex items-center gap-2 ${
                                                v.status === 'In Use' 
                                                ? 'bg-emerald-500/5 text-emerald-400 border-emerald-500/20' 
                                                : 'bg-indigo-500/5 text-indigo-400 border-indigo-500/20'
                                            }`}>
                                                <div className={`w-1 h-1 rounded-full ${v.status === 'In Use' ? 'bg-emerald-400 animate-pulse' : 'bg-indigo-400'}`} />
                                                {v.status || 'Active'}
                                            </span>
                                        </td>
                                        <td className="text-center">
                                            <span className="text-[11px] font-mono font-black text-slate-600 dark:text-slate-400 bg-slate-100 dark:bg-slate-100 dark:bg-white/5 px-3 py-1 rounded-lg border border-slate-200 dark:border-white/5">
                                                {v.size || 'N/A'}
                                            </span>
                                        </td>
                                        <td className="text-center text-slate-600 dark:text-slate-400 text-[11px] font-black">
                                            -
                                        </td>
                                        <td className="text-center">
                                            <span className="px-3 py-1 rounded-lg bg-black/40 border border-slate-200 dark:border-white/5 text-[9px] font-black uppercase tracking-[0.2em] text-slate-600 dark:text-slate-400">
                                                {v.driver}
                                            </span>
                                        </td>
                                        <td className="text-right">
                                            <button className="p-2.5 hover:bg-slate-200 dark:bg-white/10 rounded-xl text-slate-600 hover:text-slate-900 dark:text-white transition-all active:scale-90">
                                                <MoreHorizontal className="w-5 h-5" />
                                            </button>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>

                    {data.volumes.length === 0 && (
                        <div className="py-32 flex flex-col items-center justify-center text-center">
                            <div className="w-24 h-24 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 rounded-[2.5rem] flex items-center justify-center mb-8 shadow-2xl">
                                <HardDrive className="w-10 h-10 text-slate-800" />
                            </div>
                            <h3 className="text-[10px] font-black uppercase tracking-[0.4em] text-slate-600">No storage clusters isolated</h3>
                        </div>
                    )}
                </div>

                {/* Pagination */}
                <div className="mt-8 flex flex-col md:flex-row items-center justify-between gap-6 px-10 py-8 bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border-slate-200 dark:border-white/5">
                    <div className="flex items-center gap-3">
                        <Zap className="w-4 h-4 text-indigo-500" />
                        <span className="text-[10px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-widest">Showing {data.volumes.length} results of cluster state.</span>
                    </div>
                    <div className="flex items-center gap-10">
                        <div className="flex items-center gap-4">
                            <span className="text-[10px] font-black text-slate-600 uppercase tracking-widest">Rows per page</span>
                            <select className="bg-black/40 border border-slate-300 dark:border-white/10 rounded-xl px-4 py-2 text-[10px] font-black text-slate-600 dark:text-slate-400 outline-none focus:border-indigo-500 transition-all">
                                <option>20</option>
                            </select>
                        </div>
                        <div className="flex items-center gap-6">
                            <span className="text-[10px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-widest">Index 1 of 1</span>
                            <div className="flex items-center gap-2">
                                 <button className="w-10 h-10 flex items-center justify-center rounded-xl bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 text-slate-800 cursor-not-allowed">&lsaquo;</button>
                                 <button className="w-10 h-10 flex items-center justify-center rounded-xl bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 text-slate-800 cursor-not-allowed">&rsaquo;</button>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    )
}

export default memo(AdminVolumes)

