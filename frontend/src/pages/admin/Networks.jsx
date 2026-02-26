// ===========================================
// Admin Networks Page
// ===========================================

import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '../../services/api'

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
        const interval = setInterval(fetchData, 10000)
        return () => clearInterval(interval)
    }, [fetchData])

    const stats = useMemo(() => {
        const total = data.networks.length
        const unused = data.networks.filter(n => n.status === 'Unused').length
        return { total, unused }
    }, [data.networks])

    const getDriverColor = (driver) => {
        switch(driver.toLowerCase()) {
            case 'bridge': return 'bg-blue-500/10 text-blue-400 border-blue-500/20'
            case 'host': return 'bg-orange-500/10 text-orange-400 border-orange-500/20'
            case 'overlay': return 'bg-purple-500/10 text-purple-400 border-purple-500/20'
            default: return 'bg-slate-500/10 text-slate-400 border-white/5'
        }
    }

    if (isLoading && data.networks.length === 0) {
        return (
            <div className="flex flex-col items-center justify-center h-[60vh]">
                <div className="w-12 h-12 border-2 border-indigo-500/20 border-t-indigo-500 rounded-full animate-spin"></div>
            </div>
        )
    }

    return (
        <div className="min-h-screen bg-[#0a0a0c] text-slate-200 font-sans">
            {/* Header Area */}
            <div className="flex flex-col xl:flex-row xl:items-center justify-between mb-10 gap-6">
                <div>
                    <h1 className="text-4xl font-black text-white tracking-tight mb-1">Networks</h1>
                    <p className="text-slate-500 font-medium">Manage your Docker networks</p>
                </div>

                <div className="flex items-center gap-4 bg-[#111114] border border-white/[0.03] p-1.5 rounded-2xl">
                    <div className="flex items-center gap-2 px-4 py-2 border-r border-white/[0.05]">
                        <div className="w-2 h-2 rounded-full bg-blue-500 shadow-[0_0_10px_rgba(59,130,246,0.5)]"></div>
                        <span className="text-[10px] font-bold uppercase tracking-widest text-slate-400">{stats.total} Total Networks</span>
                    </div>
                    <div className="flex items-center gap-2 px-4 py-2">
                        <div className="w-2 h-2 rounded-full bg-orange-500 shadow-[0_0_10px_rgba(249,115,22,0.5)]"></div>
                        <span className="text-[10px] font-bold uppercase tracking-widest text-slate-400">{stats.unused} Unused Networks</span>
                    </div>
                </div>

                <div className="flex items-center gap-3">
                    <button className="px-5 py-2.5 bg-[#111114] hover:bg-[#1a1a1e] border border-white/5 rounded-xl text-sm font-bold text-white transition-all flex items-center gap-2 shadow-lg">
                       <PlusIcon />
                       Create Network
                    </button>
                    <button onClick={fetchData} className="p-2.5 bg-[#111114] hover:bg-[#1a1a1e] border border-white/5 rounded-xl text-white transition-all shadow-lg">
                       <RefreshIcon />
                    </button>
                </div>
            </div>

            {/* Table Area */}
            <div className="bg-[#111114] border border-white/[0.03] rounded-3xl overflow-hidden shadow-2xl">
                <div className="overflow-x-auto">
                    <table className="w-full text-left border-collapse">
                        <thead>
                            <tr className="bg-white/[0.01] border-b border-white/[0.03]">
                                <th className="px-6 py-5">
                                    <input type="checkbox" className="rounded bg-[#0a0a0c] border-white/10 text-indigo-500 focus:ring-offset-0 focus:ring-0" />
                                </th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500 leading-none">
                                    <div className="flex items-center gap-1">Name <ArrowUpIcon /></div>
                                </th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500 leading-none">
                                    <div className="flex items-center gap-1 justify-center">Status <SelectorIcon /></div>
                                </th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500 leading-none">
                                    <div className="flex items-center gap-1 justify-center">Driver <SelectorIcon /></div>
                                </th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500 leading-none">
                                    <div className="flex items-center gap-1 justify-center">Scope <SelectorIcon /></div>
                                </th>
                                <th className="px-6 py-5"></th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-white/[0.02]">
                            {data.networks.map((n) => (
                                <tr key={n.id} className="group hover:bg-white/[0.01] transition-all duration-300">
                                    <td className="px-6 py-4 w-10">
                                        <input type="checkbox" className="rounded bg-[#0a0a0c] border-white/10 text-indigo-500 focus:ring-offset-0 focus:ring-0" />
                                    </td>
                                    <td className="px-6 py-4">
                                        <span className="text-[13px] font-bold text-white group-hover:text-indigo-400 transition-colors uppercase tracking-tight">{n.name}</span>
                                    </td>
                                    <td className="px-6 py-4 text-center">
                                        <span className={`px-2.5 py-1 rounded-lg text-[10px] font-black uppercase tracking-widest border ${
                                            n.status === 'In Use' 
                                            ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20' 
                                            : 'bg-indigo-500/10 text-indigo-400 border-indigo-500/20'
                                        }`}>
                                            {n.status === 'In Use' ? 'In Use' : 'Predefined'}
                                        </span>
                                    </td>
                                    <td className="px-6 py-4 text-center">
                                        <span className={`px-2.5 py-1 rounded-lg text-[10px] font-black uppercase tracking-widest border ${getDriverColor(n.driver)}`}>
                                            {n.driver}
                                        </span>
                                    </td>
                                    <td className="px-6 py-4 text-center">
                                        <span className="px-2.5 py-1 rounded-lg bg-emerald-500/10 text-emerald-500 border border-emerald-500/20 text-[10px] font-black uppercase tracking-widest">
                                            {n.scope}
                                        </span>
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
            </div>

            <div className="mt-8 flex items-center justify-between text-[11px] text-slate-500 font-bold uppercase tracking-widest px-4">
                <span>Showing {data.networks.length} of {data.networks.length} item(s).</span>
                <div className="flex items-center gap-6">
                    <div className="flex items-center gap-3">
                        <span>Rows per page</span>
                        <select className="bg-[#111114] border border-white/5 rounded-lg px-2 py-1">
                            <option>20</option>
                        </select>
                    </div>
                    <div className="flex items-center gap-2">
                        <span>Page 1 of 1</span>
                        <div className="flex items-center gap-1 ml-4">
                             <button className="p-1.5 opacity-30 cursor-not-allowed">&lsaquo;</button>
                             <button className="p-1.5 opacity-30 cursor-not-allowed">&rsaquo;</button>
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

const RefreshIcon = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
    </svg>
)

const ArrowUpIcon = () => (
    <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M5 15l7-7 7 7" />
    </svg>
)

const SelectorIcon = () => (
    <svg className="w-3 h-3 text-slate-700" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M8 9l4-4 4 4m0 6l-4 4-4-4" />
    </svg>
)

const MoreIcon = () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M5 12h.01M12 12h.01M19 12h.01" />
    </svg>
)

export default memo(AdminNetworks)
