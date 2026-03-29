// ===========================================
// Admin Containers Page (Infrastructure)
// ===========================================

import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '../../services/api'
import toast from 'react-hot-toast'
import { Link } from 'react-router-dom'
import { 
  Plus, 
  CloudUpload, 
  RotateCw, 
  Search, 
  Filter, 
  LayoutGrid, 
  Box, 
  Check, 
  MoreHorizontal, 
  Activity, 
  Cpu, 
  HardDrive,
  Zap,
  Terminal,
  ShieldAlert
} from 'lucide-react'

const AdminContainers = () => {
    const [data, setData] = useState({
        containers: [],
        system: null
    })
    const [isLoading, setIsLoading] = useState(true)
    const [searchQuery, setSearchQuery] = useState('')

    const fetchData = useCallback(async () => {
        try {
            const res = await systemAPI.getStats()
            setData(res.data)
        } catch (error) {
            console.error('Failed to fetch containers:', error)
        } finally {
            setIsLoading(false)
        }
    }, [])

    useEffect(() => {
        fetchData()
        const interval = setInterval(fetchData, 8000)
        return () => clearInterval(interval)
    }, [fetchData])

    const filteredContainers = useMemo(() => {
        return data.containers.filter(c => 
            (c.names[0] || '').toLowerCase().includes(searchQuery.toLowerCase()) ||
            c.image.toLowerCase().includes(searchQuery.toLowerCase())
        )
    }, [data.containers, searchQuery])

    const stats = useMemo(() => {
        const total = data.containers.length
        const running = data.containers.filter(c => c.state === 'running').length
        const stopped = total - running
        return { total, running, stopped }
    }, [data.containers])

    if (isLoading && data.containers.length === 0) {
        return (
            <div className="flex flex-col items-center justify-center h-[60vh] gap-6">
                <div className="relative">
                    <div className="absolute -inset-8 bg-indigo-500/10 rounded-full blur-2xl animate-pulse"></div>
                    <div className="w-16 h-16 border-4 border-indigo-500/10 border-t-indigo-500 rounded-full animate-spin relative z-10"></div>
                </div>
                <p className="text-slate-500 text-[10px] font-black uppercase tracking-[0.3em] animate-pulse">Orchestrating Container State</p>
            </div>
        )
    }

    return (
        <div className="space-y-12 animate-pop-in relative h-full">
            {/* Background Glows */}
            <div className="absolute top-0 right-0 w-[50vw] h-[50vw] bg-indigo-600/5 blur-[140px] rounded-full pointer-events-none z-0"></div>

            <div className="relative z-10">
                {/* Header Area */}
                <div className="flex flex-col xl:flex-row xl:items-end justify-between mb-12 gap-8">
                    <div>
                        <h1 className="text-5xl font-black text-white tracking-tighter mb-4">Instance <span className="text-transparent bg-clip-text bg-gradient-to-r from-indigo-400 to-purple-400">Orchestrator</span></h1>
                        <p className="text-slate-400 text-lg font-medium">Real-time container lifecycle and resource allocation management.</p>
                    </div>

                    <div className="flex items-center gap-6">
                        <div className="flex items-center gap-4 bg-white/[0.02] border border-white/10 p-2 rounded-2xl backdrop-blur-md">
                            <div className="flex items-center gap-3 px-6 py-2 border-r border-white/5">
                                <div className="w-2.5 h-2.5 rounded-full bg-indigo-500 shadow-[0_0_15px_rgba(99,102,241,0.5)]"></div>
                                <span className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-400">{stats.total} Scaled</span>
                            </div>
                            <div className="flex items-center gap-3 px-6 py-2 border-r border-white/5">
                                <div className="w-2.5 h-2.5 rounded-full bg-emerald-500 shadow-[0_0_15px_rgba(16,185,129,0.5)] animate-pulse"></div>
                                <span className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-400">{stats.running} Awake</span>
                            </div>
                            <div className="flex items-center gap-3 px-6 py-2">
                                <div className="w-2.5 h-2.5 rounded-full bg-rose-500 shadow-[0_0_15px_rgba(244,63,94,0.5)]"></div>
                                <span className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-400">{stats.stopped} Halted</span>
                            </div>
                        </div>

                        <div className="flex items-center gap-4">
                            <button className="btn btn-secondary py-3 px-6 text-sm font-black uppercase tracking-widest active:scale-95 transition-all">
                               <Plus className="w-4 h-4" />
                               Spawn Instance
                            </button>
                            <button onClick={fetchData} className="w-12 h-12 flex items-center justify-center rounded-2xl bg-white/[0.02] border border-white/10 text-white hover:bg-white/[0.05] transition-all active:rotate-180 duration-500 transition-transform">
                               <RotateCw className="w-4 h-4" />
                            </button>
                        </div>
                    </div>
                </div>

                {/* Toolbar */}
                <div className="flex flex-col md:flex-row items-center justify-between mb-8 gap-6 bg-white/[0.02] border border-white/10 p-4 rounded-3xl backdrop-blur-md shadow-2xl">
                    <div className="flex items-center gap-4 flex-1 w-full max-w-2xl">
                        <div className="relative flex-1 group">
                            <div className="absolute inset-y-0 left-0 pl-5 flex items-center pointer-events-none">
                                <Search className="w-4 h-4 text-slate-500 group-focus-within:text-indigo-400 transition-colors" />
                            </div>
                            <input 
                                type="text"
                                placeholder="Filter active instances by name or manifest..."
                                value={searchQuery}
                                onChange={(e) => setSearchQuery(e.target.value)}
                                className="w-full bg-black/40 border border-white/5 rounded-2xl py-3.5 pl-12 pr-5 text-sm font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all placeholder:text-slate-600 outline-none"
                            />
                        </div>
                        <button className="hidden xl:flex items-center gap-3 px-5 py-3.5 bg-white/[0.02] border border-white/10 rounded-2xl text-[10px] font-black uppercase tracking-widest text-slate-400 hover:text-white transition-all">
                            <Filter className="w-3.5 h-3.5" />
                            Policy
                        </button>
                    </div>

                    <div className="flex items-center gap-3">
                        <button className="flex items-center gap-3 px-5 py-3.5 bg-indigo-500/10 border border-indigo-500/20 rounded-2xl text-[10px] font-black uppercase tracking-widest text-indigo-400 hover:bg-indigo-500/20 transition-all">
                            <LayoutGrid className="w-4 h-4" />
                            Matrix View
                        </button>
                    </div>
                </div>

                {/* Table Area */}
                <div className="card-glass overflow-hidden border-white/10 shadow-[0_30px_60px_rgba(0,0,0,0.4)] bg-white/[0.01]">
                    <div className="overflow-x-auto">
                        <table className="premium-table">
                            <thead>
                                <tr>
                                    <th className="w-12 text-center">
                                        <div className="flex items-center justify-center">
                                            <input type="checkbox" className="w-4 h-4 rounded-md border-white/10 bg-black/40 text-indigo-500 focus:ring-0 focus:ring-offset-0" />
                                        </div>
                                    </th>
                                    <th>Instance Detail</th>
                                    <th>State</th>
                                    <th className="text-center">Maintenance</th>
                                    <th>Res. Load</th>
                                    <th>Gateway Index</th>
                                    <th className="text-right">Action</th>
                                </tr>
                            </thead>
                            <tbody>
                                {filteredContainers.map((c) => (
                                    <tr key={c.id} className="group hover:bg-white/[0.03]">
                                        <td className="text-center">
                                            <div className="flex items-center justify-center">
                                                <input type="checkbox" className="w-4 h-4 rounded-md border-white/10 bg-black/40 text-indigo-500 focus:ring-0 focus:ring-offset-0" />
                                            </div>
                                        </td>
                                        <td>
                                            <div className="flex items-center gap-5">
                                                <div className="w-12 h-12 rounded-2xl bg-white/[0.03] border border-white/10 flex items-center justify-center text-slate-400 group-hover:border-indigo-500/40 group-hover:text-indigo-400 group-hover:bg-indigo-500/5 transition-all duration-500">
                                                    <Box className="w-6 h-6" />
                                                </div>
                                                <div className="flex flex-col">
                                                    <span className="text-sm font-black text-white group-hover:text-indigo-400 transition-colors uppercase tracking-tight truncate max-w-[180px]">{c.names[0]?.replace('/', '') || c.id.substring(0, 12)}</span>
                                                    <span className="text-[10px] text-slate-600 font-mono tracking-widest truncate max-w-[180px]">{c.image}</span>
                                                </div>
                                            </div>
                                        </td>
                                        <td>
                                            <span className={`px-4 py-1.5 rounded-full border text-[9px] font-black uppercase tracking-widest inline-flex items-center gap-2 ${
                                                c.state === 'running' 
                                                ? 'bg-emerald-500/5 text-emerald-400 border-emerald-500/20' 
                                                : 'bg-rose-500/5 text-rose-400 border-rose-500/20'
                                            }`}>
                                                <div className={`w-1 h-1 rounded-full ${c.state === 'running' ? 'bg-emerald-400 animate-pulse shadow-[0_0_8px_rgba(16,185,129,0.5)]' : 'bg-rose-400'}`} />
                                                {c.state === 'running' ? 'Active' : 'Offline'}
                                            </span>
                                        </td>
                                        <td className="text-center">
                                            <div className="flex justify-center">
                                                <div className="w-6 h-6 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center shadow-[0_0_10px_rgba(16,185,129,0.1)]">
                                                    <Check className="w-3.5 h-3.5 text-emerald-500" />
                                                </div>
                                            </div>
                                        </td>
                                        <td>
                                            <div className="flex flex-col gap-3 w-40">
                                                <div className="flex items-center justify-between text-[8px] font-black uppercase tracking-widest text-slate-500">
                                                     <div className="flex items-center gap-1.5"><Cpu className="w-3 h-3" /> CPU</div>
                                                     <span className="text-slate-400">{(c.cpu_percent || 0).toFixed(1)}%</span>
                                                </div>
                                                <div className="h-1 bg-white/5 rounded-full overflow-hidden">
                                                     <div className="h-full bg-gradient-to-r from-indigo-500 to-purple-500 rounded-full transition-all duration-700 shadow-[0_0_8px_rgba(99,102,241,0.3)]" style={{ width: `${Math.min(c.cpu_percent || 0, 100)}%` }}></div>
                                                </div>
                                                <div className="flex items-center justify-between text-[8px] font-black uppercase tracking-widest text-slate-500">
                                                     <div className="flex items-center gap-1.5"><HardDrive className="w-3 h-3" /> MEM</div>
                                                     <span className="text-slate-400">{(c.memory_usage || 0).toFixed(1)}MB</span>
                                                </div>
                                            </div>
                                        </td>
                                        <td>
                                            <div className="flex flex-col gap-1.5">
                                                <span className="text-[10px] font-mono font-black text-slate-400 tracking-tighter">{c.ip_address || 'Unassigned'}</span>
                                                <div className="flex flex-wrap gap-1">
                                                    {c.ports?.slice(0, 2).map((p, i) => (
                                                        <span key={i} className="px-2 py-0.5 rounded bg-indigo-500/5 border border-indigo-500/10 text-[8px] font-black text-indigo-400 tracking-tighter">
                                                            {p}
                                                        </span>
                                                    ))}
                                                    {c.ports?.length > 2 && <span className="text-[8px] text-slate-600 font-black">+{c.ports.length - 2}</span>}
                                                </div>
                                            </div>
                                        </td>
                                        <td className="text-right">
                                            <button className="p-3 hover:bg-white/10 rounded-2xl text-slate-600 hover:text-white transition-all active:scale-90">
                                                <MoreHorizontal className="w-5 h-5" />
                                            </button>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>

                    {filteredContainers.length === 0 && (
                        <div className="py-32 flex flex-col items-center justify-center text-center">
                             <div className="w-24 h-24 bg-white/[0.02] border border-white/5 rounded-[2.5rem] flex items-center justify-center mb-8 shadow-2xl">
                                <Terminal className="w-10 h-10 text-slate-800" />
                            </div>
                            <h3 className="text-[10px] font-black uppercase tracking-[0.4em] text-slate-600">No active instances in cluster</h3>
                        </div>
                    )}
                </div>

                {/* Pagination */}
                <div className="mt-8 flex flex-col md:flex-row items-center justify-between gap-6 px-10 py-8 card-glass bg-white/[0.01] border-white/5">
                    <div className="flex items-center gap-3">
                        <Activity className="w-4 h-4 text-indigo-500" />
                        <span className="text-[10px] font-black text-slate-500 uppercase tracking-widest">Showing {filteredContainers.length} of global cluster state.</span>
                    </div>
                    <div className="flex items-center gap-10">
                        <div className="flex items-center gap-4">
                            <span className="text-[10px] font-black text-slate-600 uppercase tracking-widest">Rows per index</span>
                            <select className="bg-black/40 border border-white/10 rounded-xl px-4 py-2 text-[10px] font-black text-slate-400 outline-none focus:border-indigo-500 transition-all cursor-pointer">
                                <option>All</option>
                                <option>10</option>
                                <option>20</option>
                            </select>
                        </div>
                        <div className="flex items-center gap-6">
                            <span className="text-[10px] font-black text-slate-400 uppercase tracking-widest">Page 1 of 1</span>
                            <div className="flex items-center gap-2">
                                 <button className="w-10 h-10 flex items-center justify-center rounded-xl bg-white/[0.02] border border-white/5 text-slate-800 cursor-not-allowed transition-all">&lsaquo;</button>
                                 <button className="w-10 h-10 flex items-center justify-center rounded-xl bg-white/[0.02] border border-white/5 text-slate-800 cursor-not-allowed transition-all">&rsaquo;</button>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    )
}

export default memo(AdminContainers)

