// ===========================================
// Admin Volumes Page
// ===========================================

import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '../../services/api'

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
        const interval = setInterval(fetchData, 10000)
        return () => clearInterval(interval)
    }, [fetchData])

    const stats = useMemo(() => {
        const total = data.volumes.length
        const unused = data.volumes.filter(v => v.status === 'Unused').length
        return { total, unused }
    }, [data.volumes])

    if (isLoading && data.volumes.length === 0) {
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
                    <h1 className="text-4xl font-black text-white tracking-tight mb-1">Volumes</h1>
                    <p className="text-slate-500 font-medium">Manage your Docker volumes</p>
                </div>

                <div className="flex items-center gap-4 bg-[#111114] border border-white/[0.03] p-1.5 rounded-2xl">
                    <div className="flex items-center gap-2 px-4 py-2 border-r border-white/[0.05]">
                        <div className="w-2 h-2 rounded-full bg-blue-500 shadow-[0_0_10px_rgba(59,130,246,0.5)]"></div>
                        <span className="text-[10px] font-bold uppercase tracking-widest text-slate-400">{stats.total} Total Volumes</span>
                    </div>
                    <div className="flex items-center gap-2 px-4 py-2">
                        <div className="w-2 h-2 rounded-full bg-orange-500 shadow-[0_0_10px_rgba(249,115,22,0.5)]"></div>
                        <span className="text-[10px] font-bold uppercase tracking-widest text-slate-400">{stats.unused} Unused Volumes</span>
                    </div>
                </div>

                <div className="flex items-center gap-3">
                    <button className="px-5 py-2.5 bg-[#111114] hover:bg-[#1a1a1e] border border-white/5 rounded-xl text-sm font-bold text-white transition-all flex items-center gap-2 shadow-lg">
                       <PlusIcon />
                       Create Volume
                    </button>
                    <button onClick={fetchData} className="p-2.5 bg-[#111114] hover:bg-[#1a1a1e] border border-white/5 rounded-xl text-white transition-all shadow-lg">
                       <RefreshIcon />
                    </button>
                </div>
            </div>

            {/* Table Area */}
            <div className="bg-[#111114] border border-white/[0.03] rounded-3xl overflow-hidden shadow-2xl min-h-[400px]">
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
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500 leading-none text-center">Status</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500 leading-none text-center">Size</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500 leading-none text-center">Created</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500 leading-none text-center">Driver</th>
                                <th className="px-6 py-5"></th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-white/[0.02]">
                            {data.volumes.map((v, i) => (
                                <tr key={i} className="group hover:bg-white/[0.01] transition-all duration-300">
                                    <td className="px-6 py-4 w-10">
                                        <input type="checkbox" className="rounded bg-[#0a0a0c] border-white/10 text-indigo-500 focus:ring-offset-0 focus:ring-0" />
                                    </td>
                                    <td className="px-6 py-4">
                                        <div className="flex items-center gap-3">
                                            <div className="w-9 h-9 rounded-xl bg-slate-800/50 border border-white/5 flex items-center justify-center text-slate-400 group-hover:border-indigo-500/30 group-hover:bg-indigo-500/5 transition-all">
                                                <VolumeIcon />
                                            </div>
                                            <span className="text-[13px] font-bold text-white group-hover:text-indigo-400 transition-colors truncate max-w-[300px]">{v.name}</span>
                                        </div>
                                    </td>
                                    <td className="px-6 py-4 text-center">
                                        <span className={`px-2.5 py-1 rounded-lg text-[10px] font-black uppercase tracking-widest border ${
                                            v.status === 'In Use' 
                                            ? 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20' 
                                            : 'bg-indigo-500/10 text-indigo-400 border-indigo-500/20'
                                        }`}>
                                            {v.status || 'Active'}
                                        </span>
                                    </td>
                                    <td className="px-6 py-4 text-center text-slate-500 text-[11px] font-mono">
                                        {v.size || 'N/A'}
                                    </td>
                                    <td className="px-6 py-4 text-center text-slate-500 text-[11px]">
                                        -
                                    </td>
                                    <td className="px-6 py-4 text-center">
                                        <span className="px-2.5 py-1 rounded-lg bg-white/5 border border-white/5 text-[10px] font-black uppercase tracking-widest text-slate-500">
                                            {v.driver}
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

                {data.volumes.length === 0 && (
                    <div className="absolute inset-0 flex flex-col items-center justify-center pointer-events-none opacity-50">
                        <div className="w-20 h-20 bg-white/5 rounded-3xl flex items-center justify-center mb-6">
                            <VolumeIcon className="w-10 h-10 text-slate-700" />
                        </div>
                        <h3 className="text-xl font-bold text-slate-500">No results found.</h3>
                    </div>
                )}
            </div>

            <div className="mt-8 flex items-center justify-between text-[11px] text-slate-500 font-bold uppercase tracking-widest px-4">
                <span>Showing {data.volumes.length} of {data.volumes.length} item(s).</span>
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

const VolumeIcon = ({ className = "w-5 h-5" }) => (
    <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" />
    </svg>
)

const MoreIcon = () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M5 12h.01M12 12h.01M19 12h.01" />
    </svg>
)

export default memo(AdminVolumes)
