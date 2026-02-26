// ===========================================
// Admin Containers Page
// ===========================================

import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '../../services/api'
import toast from 'react-hot-toast'
import { Link } from 'react-router-dom'

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
        const interval = setInterval(fetchData, 5000)
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
            <div className="flex flex-col items-center justify-center h-[60vh]">
                <div className="relative">
                    <div className="absolute -inset-4 bg-indigo-500/20 rounded-full blur-xl animate-pulse"></div>
                    <div className="w-12 h-12 border-2 border-indigo-500/20 border-t-indigo-500 rounded-full animate-spin relative z-10"></div>
                </div>
            </div>
        )
    }

    return (
        <div className="min-h-screen bg-[#0a0a0c] text-slate-200 font-sans selection:bg-indigo-500/30">
            {/* Header Area */}
            <div className="flex flex-col xl:flex-row xl:items-center justify-between mb-10 gap-6">
                <div>
                    <h1 className="text-4xl font-black text-white tracking-tight mb-1">Containers</h1>
                    <p className="text-slate-500 font-medium">View and Manage your Containers</p>
                </div>

                <div className="flex items-center gap-4 bg-[#111114] border border-white/[0.03] p-1.5 rounded-2xl">
                    <div className="flex items-center gap-2 px-4 py-2 border-r border-white/[0.05]">
                        <div className="w-2 h-2 rounded-full bg-indigo-500 shadow-[0_0_10px_rgba(99,102,241,0.5)]"></div>
                        <span className="text-[10px] font-bold uppercase tracking-widest text-slate-400">{stats.total} Total</span>
                    </div>
                    <div className="flex items-center gap-2 px-4 py-2 border-r border-white/[0.05]">
                        <div className="w-2 h-2 rounded-full bg-emerald-500 shadow-[0_0_10px_rgba(16,185,129,0.5)]"></div>
                        <span className="text-[10px] font-bold uppercase tracking-widest text-slate-400">{stats.running} Running</span>
                    </div>
                    <div className="flex items-center gap-2 px-4 py-2">
                        <div className="w-2 h-2 rounded-full bg-red-500 shadow-[0_0_10px_rgba(239,68,68,0.5)]"></div>
                        <span className="text-[10px] font-bold uppercase tracking-widest text-slate-400">{stats.stopped} Stopped</span>
                    </div>
                </div>

                <div className="flex items-center gap-3">
                    <button className="px-5 py-2.5 bg-[#111114] hover:bg-[#1a1a1e] border border-white/5 rounded-xl text-sm font-bold text-white transition-all flex items-center gap-2 shadow-lg group">
                       <PlusIcon />
                       Create Container
                    </button>
                    <button className="px-5 py-2.5 bg-[#111114] hover:bg-[#1a1a1e] border border-white/5 rounded-xl text-sm font-bold text-white transition-all flex items-center gap-2 shadow-lg">
                       <UpdateIcon />
                       Update Containers
                    </button>
                    <button onClick={fetchData} className="p-2.5 bg-[#111114] hover:bg-[#1a1a1e] border border-white/5 rounded-xl text-white transition-all shadow-lg active:scale-95">
                       <RefreshIcon />
                    </button>
                </div>
            </div>

            {/* Toolbar */}
            <div className="flex items-center justify-between mb-6 bg-[#111114]/50 backdrop-blur-md border border-white/[0.03] p-4 rounded-2xl shadow-xl">
                <div className="flex items-center gap-4 flex-1 max-w-xl">
                    <div className="relative flex-1 group">
                        <div className="absolute inset-y-0 left-0 pl-4 flex items-center pointer-events-none">
                            <SearchIcon className="text-slate-500 group-focus-within:text-indigo-400 transition-colors" />
                        </div>
                        <input 
                            type="text"
                            placeholder="Search containers..."
                            value={searchQuery}
                            onChange={(e) => setSearchQuery(e.target.value)}
                            className="w-full bg-[#0a0a0c] border border-white/5 rounded-xl py-2.5 pl-11 pr-4 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all placeholder:text-slate-600"
                        />
                    </div>
                    <button className="px-4 py-2.5 bg-[#111114] border border-white/5 rounded-xl text-sm font-bold text-slate-400 flex items-center gap-2 hover:text-white transition-colors">
                        <FilterIcon />
                        Updates
                    </button>
                </div>

                <div className="flex items-center gap-2">
                    <button className="p-2.5 rounded-xl bg-indigo-500/10 text-indigo-400">
                        <RowViewIcon />
                    </button>
                </div>
            </div>

            {/* Table Area */}
            <div className="bg-[#111114] border border-white/[0.03] rounded-3xl overflow-hidden shadow-2xl relative">
                <div className="overflow-x-auto">
                    <table className="w-full text-left border-collapse">
                        <thead>
                            <tr className="bg-white/[0.01] border-b border-white/[0.03]">
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500">
                                    <input type="checkbox" className="rounded bg-[#0a0a0c] border-white/10 text-indigo-500 focus:ring-offset-0 focus:ring-0" />
                                </th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500">Name</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500">Image</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500">State</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500 text-center">Updates</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500">CPU Usage</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500">Memory Usage</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500">Status</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500">IP Address</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500">Ports</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500">Created</th>
                                <th className="px-6 py-5"></th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-white/[0.02]">
                            {filteredContainers.map((c) => (
                                <tr key={c.id} className="group hover:bg-white/[0.01] transition-all duration-300">
                                    <td className="px-6 py-4">
                                        <input type="checkbox" className="rounded bg-[#0a0a0c] border-white/10 text-indigo-500 focus:ring-offset-0 focus:ring-0" />
                                    </td>
                                    <td className="px-6 py-4">
                                        <div className="flex items-center gap-3">
                                            <div className="w-9 h-9 rounded-xl bg-slate-800/50 border border-white/5 flex items-center justify-center text-slate-400 group-hover:border-indigo-500/30 group-hover:bg-indigo-500/5 transition-all">
                                                <BoxIcon />
                                            </div>
                                            <span className="text-[13px] font-bold text-white group-hover:text-indigo-400 transition-colors truncate max-w-[140px]">{c.names[0] || c.id.substring(0, 12)}</span>
                                        </div>
                                    </td>
                                    <td className="px-6 py-4">
                                        <span className="text-[11px] text-slate-500 font-medium truncate max-w-[180px] block">{c.image}</span>
                                    </td>
                                    <td className="px-6 py-4">
                                        <span className={`px-2.5 py-1 rounded-lg text-[10px] font-black uppercase tracking-widest border ${
                                            c.state === 'running' 
                                            ? 'bg-emerald-500/10 text-emerald-500 border-emerald-500/20 shadow-[0_0_15px_rgba(16,185,129,0.05)]' 
                                            : 'bg-red-500/10 text-red-500 border-red-500/20 shadow-[0_0_15px_rgba(239,68,68,0.05)]'
                                        }`}>
                                            {c.state}
                                        </span>
                                    </td>
                                    <td className="px-6 py-4 text-center">
                                        <div className="flex justify-center">
                                            <div className="w-5 h-5 rounded-full bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center">
                                                <CheckIcon className="w-3 h-3 text-emerald-500" />
                                            </div>
                                        </div>
                                    </td>
                                    <td className="px-6 py-4">
                                        <div className="flex items-center gap-3 w-28">
                                            <div className="flex-1 h-1 bg-white/5 rounded-full overflow-hidden">
                                                <div className="h-full bg-slate-500/40 rounded-full" style={{ width: `${Math.min(c.cpu_percent || 0, 100)}%` }}></div>
                                            </div>
                                            <span className="text-[11px] font-mono text-slate-400">{(c.cpu_percent || 0).toFixed(1)}%</span>
                                        </div>
                                    </td>
                                    <td className="px-6 py-4">
                                        <div className="flex items-center gap-3 w-32">
                                            <div className="flex-1 h-1 bg-white/5 rounded-full overflow-hidden">
                                                <div className="h-full bg-indigo-500/50 rounded-full" style={{ width: `${Math.min((c.memory_usage || 0) / 1024 * 10, 100)}%` }}></div>
                                            </div>
                                            <span className="text-[11px] font-mono text-slate-400">{(c.memory_usage || 0).toFixed(1)} MB</span>
                                        </div>
                                    </td>
                                    <td className="px-6 py-4">
                                        <span className="text-[11px] text-slate-400 font-medium whitespace-nowrap">{c.status}</span>
                                    </td>
                                    <td className="px-6 py-4">
                                        <span className="text-[11px] text-slate-500 font-mono tracking-tighter">{c.ip_address || '-'}</span>
                                    </td>
                                    <td className="px-6 py-4">
                                        <div className="flex flex-wrap gap-1">
                                            {c.ports && c.ports.length > 0 ? c.ports.map((p, i) => (
                                                <span key={i} className="px-2 py-0.5 rounded-md bg-indigo-500/5 border border-indigo-500/10 text-[9px] font-black text-indigo-400/80 whitespace-nowrap">
                                                    {p}
                                                </span>
                                            )) : <span className="text-[10px] text-slate-600 italic">No ports</span>}
                                        </div>
                                    </td>
                                    <td className="px-6 py-4 transition-all">
                                        <span className="text-[11px] text-slate-500 whitespace-nowrap">{new Date(c.created_at).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}</span>
                                    </td>
                                    <td className="px-6 py-4 text-right">
                                        <button className="p-2 hover:bg-white/5 rounded-lg text-slate-600 hover:text-white transition-all">
                                            <MoreIcon />
                                        </button>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
                {filteredContainers.length === 0 && (
                    <div className="p-20 text-center bg-white/[0.01]">
                        <div className="w-16 h-16 bg-white/5 rounded-2xl flex items-center justify-center mx-auto mb-6">
                            <BoxIcon className="w-8 h-8 text-slate-600" />
                        </div>
                        <h3 className="text-white font-bold mb-1">No containers found</h3>
                        <p className="text-slate-500 text-sm">Try adjusting your search or refresh the list.</p>
                    </div>
                )}
            </div>
            
            <div className="mt-8 flex items-center justify-between text-[11px] text-slate-500 font-bold uppercase tracking-widest px-4">
                <span>Showing {filteredContainers.length} of {data.containers.length} item(s).</span>
                <div className="flex items-center gap-6">
                    <div className="flex items-center gap-3">
                        <span>Rows per page</span>
                        <select className="bg-[#111114] border border-white/5 rounded-lg px-2 py-1 focus:outline-none focus:ring-0">
                            <option>All</option>
                            <option>10</option>
                            <option>20</option>
                        </select>
                    </div>
                    <div className="flex items-center gap-2">
                        <span>Page 1 of 1</span>
                        <div className="flex items-center gap-1 ml-4">
                            <button className="p-1.5 hover:bg-white/5 rounded-lg disabled:opacity-30" disabled>&laquo;</button>
                            <button className="p-1.5 hover:bg-white/5 rounded-lg disabled:opacity-30" disabled>&lsaquo;</button>
                            <button className="p-1.5 hover:bg-white/5 rounded-lg disabled:opacity-30" disabled>&rsaquo;</button>
                            <button className="p-1.5 hover:bg-white/5 rounded-lg disabled:opacity-30" disabled>&raquo;</button>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    )
}

// Icons
const PlusIcon = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
    </svg>
)

const UpdateIcon = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
    </svg>
)

const RefreshIcon = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
    </svg>
)

const SearchIcon = ({ className }) => (
    <svg className={`w-4 h-4 ${className}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
    </svg>
)

const FilterIcon = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M3 4h18M7 9h10M10 14h4M12 19h.01" />
    </svg>
)

const RowViewIcon = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M4 6h16M4 12h16M4 18h16" />
    </svg>
)

const BoxIcon = ({ className = "w-4 h-4" }) => (
    <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" />
    </svg>
)

const CheckIcon = ({ className }) => (
    <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={3} d="M5 13l4 4L19 7" />
    </svg>
)

const MoreIcon = () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M5 12h.01M12 12h.01M19 12h.01M6 12a1 1 0 11-2 0 1 1 0 012 0zm7 0a1 1 0 11-2 0 1 1 0 012 0zm7 0a1 1 0 11-2 0 1 1 0 012 0z" />
    </svg>
)

export default memo(AdminContainers)
