// ===========================================
// Admin Containers Page
// ===========================================

import { useState, useEffect, memo } from 'react'
import { systemAPI } from '../../services/api'
import toast from 'react-hot-toast'

const ContainerIcon = memo(() => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" />
    </svg>
))

const AdminContainers = () => {
    const [containers, setContainers] = useState([])
    const [isLoading, setIsLoading] = useState(true)

    const fetchContainers = async () => {
        try {
            const res = await systemAPI.getStats()
            setContainers(res.data.containers || [])
        } catch (error) {
            console.error('Failed to fetch containers:', error)
        } finally {
            setIsLoading(false)
        }
    }

    useEffect(() => {
        fetchContainers()
        const interval = setInterval(fetchContainers, 5000)
        return () => clearInterval(interval)
    }, [])

    if (isLoading && containers.length === 0) {
        return (
            <div className="flex flex-col items-center justify-center h-[60vh]">
                <div className="w-12 h-12 border-4 border-purple-500/20 border-t-purple-500 rounded-full animate-spin"></div>
            </div>
        )
    }

    return (
        <div className="min-h-screen bg-[#0a0a0c] text-slate-200">
            <div className="flex items-center justify-between mb-8">
                <div>
                    <h1 className="text-3xl font-bold text-white tracking-tight">Containers</h1>
                    <p className="text-slate-500 mt-1">Manage running and stopped containers</p>
                </div>
                <button 
                  onClick={fetchContainers}
                  className="px-4 py-2 bg-[#1a1a1e] hover:bg-[#252529] border border-white/5 rounded-lg text-sm font-medium transition-all"
                >
                  Refresh
                </button>
            </div>

            <div className="bg-[#111114] border border-white/[0.03] rounded-2xl overflow-hidden shadow-2xl">
                <div className="overflow-x-auto">
                    <table className="w-full text-left">
                        <thead className="text-[10px] uppercase tracking-[0.15em] text-slate-600 font-bold border-b border-white/[0.03]">
                            <tr>
                                <th className="px-6 py-4 text-[#0ea5e9]">Name</th>
                                <th className="px-6 py-4">Image</th>
                                <th className="px-6 py-4 text-center">State</th>
                                <th className="px-6 py-4 text-right">Status</th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-white/[0.03]">
                            {containers.map((c) => (
                                <tr key={c.id} className="group hover:bg-white/[0.01] transition-colors">
                                    <td className="px-6 py-4">
                                        <div className="flex items-center gap-3">
                                            <div className="w-8 h-8 rounded bg-slate-800 flex items-center justify-center text-slate-500 font-mono text-[10px]">
                                                {c.names[0]?.substring(0, 2).toUpperCase() || '??'}
                                            </div>
                                            <span className="text-xs font-semibold text-white">{c.names[0]}</span>
                                        </div>
                                    </td>
                                    <td className="px-6 py-4">
                                        <span className="text-xs text-slate-500 font-mono italic">{c.image}</span>
                                    </td>
                                    <td className="px-6 py-4 text-center">
                                        <span className={`px-2.5 py-1 rounded-full text-[10px] font-bold uppercase tracking-wider ${
                                            c.state === 'running' ? 'bg-emerald-500/10 text-emerald-500 border border-emerald-500/20' : 'bg-red-500/10 text-red-500 border border-red-500/20'
                                        }`}>
                                            {c.state}
                                        </span>
                                    </td>
                                    <td className="px-6 py-4 text-right">
                                        <span className="text-xs text-slate-400">{c.status}</span>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
                {containers.length === 0 && (
                    <div className="p-20 text-center">
                        <p className="text-slate-600 font-medium">No containers found on this host.</p>
                    </div>
                )}
            </div>
        </div>
    )
}

export default memo(AdminContainers)
