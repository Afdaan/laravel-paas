// ===========================================
// Admin Images Page
// ===========================================

import { useState, useEffect, memo, useCallback, useMemo } from 'react'
import { systemAPI } from '../../services/api'
import toast from 'react-hot-toast'

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
        const interval = setInterval(fetchData, 10000)
        return () => clearInterval(interval)
    }, [fetchData])

    const handlePrune = async () => {
        if (!window.confirm('Are you sure you want to prune unused images?')) return
        setIsPruning(true)
        try {
            await systemAPI.prune()
            toast.success('Pruning complete')
            fetchData()
        } catch (error) {
            toast.error('Pruning failed')
        } finally {
            setIsPruning(false)
        }
    }

    const filteredImages = useMemo(() => {
        return data.images.filter(img => 
            img.repository.toLowerCase().includes(searchQuery.toLowerCase()) ||
            img.tag.toLowerCase().includes(searchQuery.toLowerCase())
        )
    }, [data.images, searchQuery])

    const stats = useMemo(() => {
        const total = data.images.length
        let totalSize = 0
        // Helper to parse size like "467.97MB"
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
            <div className="flex flex-col items-center justify-center h-[60vh]">
                <div className="relative">
                    <div className="absolute -inset-4 bg-purple-500/20 rounded-full blur-xl animate-pulse"></div>
                    <div className="w-12 h-12 border-2 border-purple-500/20 border-t-purple-500 rounded-full animate-spin relative z-10"></div>
                </div>
            </div>
        )
    }

    return (
        <div className="min-h-screen bg-[#0a0a0c] text-slate-200 font-sans selection:bg-purple-500/30">
            {/* Header Area */}
            <div className="flex flex-col xl:flex-row xl:items-center justify-between mb-10 gap-6">
                <div>
                    <h1 className="text-4xl font-black text-white tracking-tight mb-1">Images</h1>
                    <p className="text-slate-500 font-medium">View and Manage your Container Images</p>
                </div>

                <div className="flex items-center gap-4 bg-[#111114] border border-white/[0.03] p-1.5 rounded-2xl">
                    <div className="flex items-center gap-2 px-4 py-2 border-r border-white/[0.05]">
                        <div className="w-2 h-2 rounded-full bg-blue-500 shadow-[0_0_10px_rgba(59,130,246,0.5)]"></div>
                        <span className="text-[10px] font-bold uppercase tracking-widest text-slate-400">{stats.total} Total Images</span>
                    </div>
                    <div className="flex items-center gap-2 px-4 py-2">
                        <div className="w-2 h-2 rounded-full bg-orange-500 shadow-[0_0_10px_rgba(249,115,22,0.5)]"></div>
                        <span className="text-[10px] font-bold uppercase tracking-widest text-slate-400">{stats.totalSize} Total Size</span>
                    </div>
                </div>

                <div className="flex items-center gap-3">
                    <button className="px-5 py-2.5 bg-[#111114] hover:bg-[#1a1a1e] border border-white/5 rounded-xl text-sm font-bold text-white transition-all flex items-center gap-2 shadow-lg">
                       <DownloadIcon />
                       Pull Image
                    </button>
                    <button className="px-5 py-2.5 bg-[#111114] hover:bg-[#1a1a1e] border border-white/5 rounded-xl text-sm font-bold text-white transition-all flex items-center gap-2 shadow-lg">
                       <UploadIcon />
                       Upload Image
                    </button>
                    <button onClick={handlePrune} disabled={isPruning} className="px-5 py-2.5 bg-red-500/10 hover:bg-red-500/20 border border-red-500/20 rounded-xl text-sm font-bold text-red-500 transition-all flex items-center gap-2 shadow-lg disabled:opacity-50">
                       <PruneIcon />
                       {isPruning ? 'Pruning...' : 'Prune Unused'}
                    </button>
                </div>
            </div>

            {/* Toolbar */}
            <div className="flex items-center justify-between mb-6 bg-[#111114]/50 backdrop-blur-md border border-white/[0.03] p-4 rounded-2xl shadow-xl">
                <div className="flex items-center gap-4 flex-1 max-w-xl">
                    <div className="relative flex-1 group">
                        <div className="absolute inset-y-0 left-0 pl-4 flex items-center pointer-events-none">
                            <SearchIcon className="text-slate-500 group-focus-within:text-purple-400 transition-colors" />
                        </div>
                        <input 
                            type="text"
                            placeholder="Search images..."
                            value={searchQuery}
                            onChange={(e) => setSearchQuery(e.target.value)}
                            className="w-full bg-[#0a0a0c] border border-white/5 rounded-xl py-2.5 pl-11 pr-4 text-sm focus:outline-none focus:ring-2 focus:ring-purple-500/20 focus:border-purple-500/40 transition-all placeholder:text-slate-600"
                        />
                    </div>
                    <button className="px-4 py-2.5 bg-[#111114] border border-white/5 rounded-xl text-sm font-bold text-slate-400 flex items-center gap-2 hover:text-white transition-colors">
                        <UsageIcon />
                        Usage
                    </button>
                    <button className="px-4 py-2.5 bg-[#111114] border border-white/5 rounded-xl text-sm font-bold text-slate-400 flex items-center gap-2 hover:text-white transition-colors">
                        <UpdateIcon />
                        Updates
                    </button>
                </div>

                <div className="flex items-center gap-2">
                    <button className="p-2.5 rounded-xl bg-purple-500/10 text-purple-400">
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
                                <th className="px-6 py-5">
                                    <input type="checkbox" className="rounded bg-[#0a0a0c] border-white/10 text-purple-500 focus:ring-offset-0 focus:ring-0" />
                                </th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500">Repository</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500 text-center">Tags</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500 text-center">Status</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500">Used By</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500 text-center">Updates</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500 text-center">Vulnerabilities</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500">Size</th>
                                <th className="px-6 py-5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-500">Created</th>
                                <th className="px-6 py-5"></th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-white/[0.02]">
                            {filteredImages.map((img, i) => (
                                <tr key={i} className="group hover:bg-white/[0.01] transition-all duration-300">
                                    <td className="px-6 py-4">
                                        <input type="checkbox" className="rounded bg-[#0a0a0c] border-white/10 text-purple-500 focus:ring-offset-0 focus:ring-0" />
                                    </td>
                                    <td className="px-6 py-4">
                                        <div className="flex items-center gap-3">
                                            <div className="w-9 h-9 rounded-xl bg-slate-800/50 border border-white/5 flex items-center justify-center text-slate-400 group-hover:border-purple-500/30 group-hover:bg-purple-500/5 transition-all">
                                                <ImageIcon />
                                            </div>
                                            <div>
                                                <span className="text-[13px] font-bold text-white group-hover:text-purple-400 transition-colors truncate max-w-[200px] block">{img.repository}</span>
                                                <span className="text-[9px] text-slate-600 font-mono tracking-tighter uppercase">{img.id?.substring(7, 19)}</span>
                                            </div>
                                        </div>
                                    </td>
                                    <td className="px-6 py-4 text-center">
                                        <span className="px-2 py-0.5 rounded-lg bg-white/5 border border-white/[0.05] text-[10px] font-black text-slate-400">{img.tag}</span>
                                    </td>
                                    <td className="px-6 py-4 text-center">
                                        <span className={`px-2.5 py-1 rounded-lg text-[10px] font-black uppercase tracking-widest border ${
                                            img.status === 'In Use' 
                                            ? 'bg-teal-500/10 text-teal-400 border-teal-500/20' 
                                            : 'bg-slate-500/10 text-slate-500 border-white/5'
                                        }`}>
                                            {img.status}
                                        </span>
                                    </td>
                                    <td className="px-6 py-4">
                                        <div className="flex items-center gap-2">
                                            <div className="w-5 h-5 rounded-full bg-slate-800 border border-white/5 flex items-center justify-center text-[10px] text-slate-500">
                                                <UserSmallIcon />
                                            </div>
                                            <span className="text-[11px] text-slate-400 font-medium truncate max-w-[150px]">{img.repository.split('/').pop() || 'System'}</span>
                                        </div>
                                    </td>
                                    <td className="px-6 py-4 text-center">
                                        <div className="flex justify-center">
                                            <div className="w-5 h-5 rounded-full bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center">
                                                <CheckIcon className="w-3 h-3 text-emerald-500" />
                                            </div>
                                        </div>
                                    </td>
                                    <td className="px-6 py-4 text-center">
                                        <div className="flex justify-center">
                                            <SearchIcon className="w-4 h-4 text-slate-700" />
                                        </div>
                                    </td>
                                    <td className="px-6 py-4">
                                        <span className="text-[11px] text-slate-400 font-mono tracking-tighter">{img.size_human}</span>
                                    </td>
                                    <td className="px-6 py-4">
                                        <span className="text-[11px] text-slate-500 whitespace-nowrap">Feb 18, 2026 8:03 PM</span>
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
                <span>Showing {filteredImages.length} of {data.images.length} item(s).</span>
                <div className="flex items-center gap-6">
                    <div className="flex items-center gap-3">
                        <span>Rows per page</span>
                        <select className="bg-[#111114] border border-white/5 rounded-lg px-2 py-1 focus:outline-none focus:ring-0">
                            <option>20</option>
                            <option>50</option>
                            <option>All</option>
                        </select>
                    </div>
                    <div className="flex items-center gap-2">
                        <span>Page 1 of 1</span>
                        <div className="flex items-center gap-1 ml-4">
                             <button className="p-1.5 hover:bg-white/5 rounded-lg" disabled>&lsaquo;</button>
                             <button className="p-1.5 hover:bg-white/5 rounded-lg" disabled>&rsaquo;</button>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    )
}

// Icons
const DownloadIcon = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
    </svg>
)

const UploadIcon = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-8l-4-4m0 0L8 8m4-4v12" />
    </svg>
)

const PruneIcon = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
    </svg>
)

const UsageIcon = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 002 2h2a2 2 0 002-2" />
    </svg>
)

const UpdateIcon = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
    </svg>
)

const SearchIcon = ({ className }) => (
    <svg className={className} fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
    </svg>
)

const RowViewIcon = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2.5} d="M4 6h16M4 12h16M4 18h16" />
    </svg>
)

const ImageIcon = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
    </svg>
)

const UserSmallIcon = () => (
    <svg className="w-3 h-3" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z" />
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

export default memo(AdminImages)
