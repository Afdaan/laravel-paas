// ===========================================
// Admin Networks Page (Infrastructure)
// ===========================================

import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '../../services/api'
import { 
  Plus, 
  RotateCw, 
  ChevronUp, 
  Share2, 
  MoreHorizontal,
  Selector,
  Activity,
  Zap,
  ShieldCheck,
  Globe
} from 'lucide-react'

const AdminNetworks = () => {
    const [data, setData] = useState({
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
        const total = data.networks.length
        const unused = data.networks.filter(n => n.status === 'Unused').length
        return { total, unused }
    }, [data.networks])

    const getDriverColor = (driver) => {
        switch(driver.toLowerCase()) {
            case 'bridge': return 'bg-blue-500/5 text-blue-400 border-blue-500/20'
            case 'host': return 'bg-orange-500/5 text-orange-400 border-orange-500/20'
            case 'overlay': return 'bg-purple-500/5 text-purple-400 border-purple-500/20'
            default: return 'bg-slate-500/5 text-slate-400 border-white/5'
        }
    }

    if (isLoading && data.networks.length === 0) {
        return (
            <div className="flex flex-col items-center justify-center h-[60vh] gap-6">
                <div className="w-16 h-16 border-4 border-indigo-500/10 border-t-indigo-500 rounded-full animate-spin"></div>
                <p className="text-slate-500 text-[10px] font-black uppercase tracking-[0.3em] animate-pulse">Mapping Virtual Networks</p>
            </div>
        )
    }

    return (
        <div className="space-y-12 animate-pop-in relative h-full">
            {/* Background Glows */}
            <div className="absolute top-0 right-0 w-[40vw] h-[40vw] bg-indigo-600/5 blur-[120px] rounded-full pointer-events-none z-0"></div>

            <div className="relative z-10">
                {/* Header Area */}
                <div className="flex flex-col xl:flex-row xl:items-end justify-between mb-12 gap-8">
                    <div>
                        <h1 className="text-5xl font-black text-white tracking-tighter mb-4">Virtual <span className="text-transparent bg-clip-text bg-gradient-to-r from-emerald-400 to-teal-400">Networks</span></h1>
                        <p className="text-slate-400 text-lg font-medium">Software-defined networking and isolated packet orchestration.</p>
                    </div>

                    <div className="flex items-center gap-6">
                        <div className="flex items-center gap-4 bg-white/[0.02] border border-white/10 p-2 rounded-2xl backdrop-blur-md">
                            <div className="flex items-center gap-3 px-6 py-2 border-r border-white/5">
                                <div className="w-2.5 h-2.5 rounded-full bg-emerald-500 shadow-[0_0_15px_rgba(16,185,129,0.5)] animate-pulse"></div>
                                <span className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-400">{stats.total} Active Pipes</span>
                            </div>
                            <div className="flex items-center gap-3 px-6 py-2">
                                <div className="w-2.5 h-2.5 rounded-full bg-slate-500 shadow-[0_0_15px_rgba(100,116,139,0.5)]"></div>
                                <span className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-400">{stats.unused} Standby</span>
                            </div>
                        </div>

                        <div className="flex items-center gap-4">
                            <button className="btn btn-secondary py-3 px-6 text-sm font-black uppercase tracking-widest shadow-xl shadow-indigo-500/10 active:scale-95 transition-all">
                               <Plus className="w-4 h-4" />
                               Provision Network
                            </button>
                            <button onClick={fetchData} className="w-12 h-12 flex items-center justify-center bg-white/[0.02] hover:bg-white/[0.05] border border-white/10 rounded-2xl text-white transition-all shadow-lg active:rotate-180 transition-transform duration-500">
                               <RotateCw className="w-4 h-4" />
                            </button>
                        </div>
                    </div>
                </div>

                {/* Table Area */}
                <div className="card-glass overflow-hidden border-white/10 shadow-[0_20px_50px_rgba(0,0,0,0.3)] bg-white/[0.02]">
                    <div className="overflow-x-auto">
                        <table className="premium-table">
                            <thead>
                                <tr>
                                    <th className="w-12 text-center">
                                        <div className="flex items-center justify-center">
                                            <input type="checkbox" className="w-4 h-4 rounded-md border-white/10 bg-black/40 text-indigo-500 focus:ring-0 focus:ring-offset-0" />
                                        </div>
                                    </th>
                                    <th>Interface Identity</th>
                                    <th className="text-center">Connection State</th>
                                    <th className="text-center">Protocol / Driver</th>
                                    <th className="text-center">Exposure Scope</th>
                                    <th className="text-right">Action</th>
                                </tr>
                            </thead>
                            <tbody>
                                {data.networks.map((n) => (
                                    <tr key={n.id} className="group hover:bg-white/[0.03]">
                                        <td className="text-center">
                                            <div className="flex items-center justify-center">
                                                <input type="checkbox" className="w-4 h-4 rounded-md border-white/10 bg-black/40 text-indigo-500 focus:ring-0 focus:ring-offset-0" />
                                            </div>
                                        </td>
                                        <td>
                                            <div className="flex items-center gap-5">
                                                <div className="w-12 h-12 rounded-2xl bg-white/[0.03] border border-white/10 flex items-center justify-center text-slate-400 group-hover:border-emerald-500/40 group-hover:text-emerald-400 group-hover:bg-emerald-500/5 transition-all duration-500">
                                                    <Share2 className="w-6 h-6" />
                                                </div>
                                                <div className="flex flex-col">
                                                    <span className="text-sm font-black text-white group-hover:text-emerald-400 transition-colors uppercase tracking-tight">{n.name}</span>
                                                    <span className="text-[10px] text-slate-600 font-mono tracking-widest">{n.id.substring(0, 12)}</span>
                                                </div>
                                            </div>
                                        </td>
                                        <td className="text-center">
                                            <span className={`px-4 py-1.5 rounded-full border text-[9px] font-black uppercase tracking-widest inline-flex items-center gap-2 ${
                                                n.status === 'In Use' 
                                                ? 'bg-emerald-500/5 text-emerald-400 border-emerald-500/20' 
                                                : 'bg-indigo-500/5 text-indigo-400 border-indigo-500/20'
                                            }`}>
                                                <div className={`w-1 h-1 rounded-full ${n.status === 'In Use' ? 'bg-emerald-400 animate-pulse' : 'bg-indigo-400'}`} />
                                                {n.status === 'In Use' ? 'Routed' : 'Isolated'}
                                            </span>
                                        </td>
                                        <td className="text-center">
                                            <span className={`px-3 py-1 rounded-lg text-[9px] font-black uppercase tracking-widest border ${getDriverColor(n.driver)}`}>
                                                {n.driver}
                                            </span>
                                        </td>
                                        <td className="text-center">
                                            <span className="px-3 py-1 rounded-lg bg-emerald-500/5 text-emerald-500 border border-emerald-500/20 text-[9px] font-black uppercase tracking-widest inline-flex items-center gap-2">
                                                <Globe className="w-3 h-3" />
                                                {n.scope}
                                            </span>
                                        </td>
                                        <td className="text-right">
                                            <button className="p-2.5 hover:bg-white/10 rounded-xl text-slate-600 hover:text-white transition-all active:scale-90">
                                                <MoreHorizontal className="w-5 h-5" />
                                            </button>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                </div>

                {/* Pagination */}
                <div className="mt-8 flex flex-col md:flex-row items-center justify-between gap-6 px-10 py-8 card-glass bg-white/[0.01] border-white/5">
                    <div className="flex items-center gap-3">
                        <Activity className="w-4 h-4 text-emerald-500" />
                        <span className="text-[10px] font-black text-slate-500 uppercase tracking-widest">Showing {data.networks.length} virtual interfaces of cluster state.</span>
                    </div>
                    <div className="flex items-center gap-10">
                        <div className="flex items-center gap-4">
                            <span className="text-[10px] font-black text-slate-600 uppercase tracking-widest">Rows per page</span>
                            <select className="bg-black/40 border border-white/10 rounded-xl px-4 py-2 text-[10px] font-black text-slate-400 outline-none focus:border-indigo-500 transition-all">
                                <option>20</option>
                            </select>
                        </div>
                        <div className="flex items-center gap-6">
                            <span className="text-[10px] font-black text-slate-400 uppercase tracking-widest">Index 1 of 1</span>
                            <div className="flex items-center gap-2">
                                 <button className="w-10 h-10 flex items-center justify-center rounded-xl bg-white/[0.02] border border-white/5 text-slate-800 cursor-not-allowed">&lsaquo;</button>
                                 <button className="w-10 h-10 flex items-center justify-center rounded-xl bg-white/[0.02] border border-white/5 text-slate-800 cursor-not-allowed">&rsaquo;</button>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    )
}

export default memo(AdminNetworks)

