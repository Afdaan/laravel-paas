    // ===========================================
// Admin Images Page
// ===========================================

import { useState, useEffect, memo } from 'react'
import { systemAPI } from '../../services/api'

const AdminImages = () => {
    const [images, setImages] = useState([])
    const [isLoading, setIsLoading] = useState(true)

    const fetchImages = async () => {
        try {
            const res = await systemAPI.getStats()
            setImages(res.data.images || [])
        } catch (error) {
            console.error('Failed to fetch images:', error)
        } finally {
            setIsLoading(false)
        }
    }

    useEffect(() => {
        fetchImages()
        const interval = setInterval(fetchImages, 10000)
        return () => clearInterval(interval)
    }, [])

    if (isLoading && images.length === 0) {
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
                    <h1 className="text-3xl font-bold text-white tracking-tight">Docker Images</h1>
                    <p className="text-slate-500 mt-1">Local registry and cached images</p>
                </div>
                <button 
                  onClick={fetchImages}
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
                                <th className="px-6 py-4">Repository</th>
                                <th className="px-6 py-4 text-center">Status</th>
                                <th className="px-6 py-4 text-center">Tag</th>
                                <th className="px-6 py-4 text-right">Size</th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-white/[0.03]">
                            {images.map((img, i) => (
                                <tr key={i} className="group hover:bg-white/[0.01] transition-colors">
                                    <td className="px-6 py-4">
                                        <div className="flex flex-col">
                                            <span className="text-xs font-semibold text-white">{img.repository}</span>
                                            <span className="text-[10px] text-slate-600 font-mono mt-0.5 uppercase">{img.id?.substring(7, 19)}</span>
                                        </div>
                                    </td>
                                    <td className="px-6 py-4 text-center">
                                        <span className={`px-2 py-1 rounded text-[10px] font-black uppercase tracking-widest ${
                                            img.status === 'In Use' ? 'bg-cyan-500/10 text-cyan-500 border border-cyan-500/20' : 'bg-slate-500/10 text-slate-500 border border-slate-500/20'
                                        }`}>
                                            {img.status}
                                        </span>
                                    </td>
                                    <td className="px-6 py-4 text-center">
                                        <span className="text-xs text-slate-400 font-medium">{img.tag}</span>
                                    </td>
                                    <td className="px-6 py-4 text-right font-mono">
                                        <span className="text-xs font-bold text-slate-300 tracking-tighter">{img.size_human}</span>
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
                {images.length === 0 && (
                    <div className="p-20 text-center text-slate-600 font-medium">No images found.</div>
                )}
            </div>
        </div>
    )
}

export default memo(AdminImages)
