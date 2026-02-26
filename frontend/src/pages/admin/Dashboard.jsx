// ===========================================
// Admin Dashboard (PaaS Style)
// ===========================================

import { useState, useEffect } from 'react'
import { systemAPI, projectsAPI } from '../../services/api'
import toast from 'react-hot-toast'

function AdminDashboard() {
  const [data, setData] = useState({
    system: null,
    containers: [],
    images: [],
  })
  const [isLoading, setIsLoading] = useState(true)
  const [isPruning, setIsPruning] = useState(false)

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 10000) // Auto refresh every 10s
    return () => clearInterval(interval)
  }, [])

  const fetchData = async () => {
    try {
      const res = await systemAPI.getStats()
      setData(res.data)
    } catch (error) {
      console.error('Failed to fetch system stats:', error)
    } finally {
      setIsLoading(false)
    }
  }

  const handlePrune = async () => {
    if (!window.confirm('Are you sure you want to prune the system? This will remove unused images and volumes.')) return
    
    setIsPruning(true)
    try {
      await systemAPI.prune()
      toast.success('System pruned successfully')
      fetchData()
    } catch (error) {
      toast.error('Failed to prune system')
    } finally {
      setIsPruning(false)
    }
  }

  // Format bytes to human readable
  const formatBytes = (bytes) => {
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
  }

  if (isLoading && !data.system) {
    return (
      <div className="flex flex-col items-center justify-center h-screen bg-[#0a0a0c]">
        <div className="relative w-20 h-20">
            <div className="absolute inset-0 rounded-full border-4 border-purple-500/20"></div>
            <div className="absolute inset-0 rounded-full border-4 border-t-purple-500 animate-spin"></div>
        </div>
        <p className="mt-4 text-purple-400 font-medium animate-pulse">Initializing PaaS Monitor...</p>
      </div>
    )
  }

  const { system, containers, images } = data

  return (
    <div className="min-h-screen bg-[#0a0a0c] text-slate-200 p-4 lg:p-8 font-sans">
      <Header onRefresh={fetchData} onPrune={handlePrune} isPruning={isPruning} />
      <SystemOverview system={system} formatBytes={formatBytes} />
      
      <div className="grid grid-cols-1 xl:grid-cols-2 gap-8">
        <ResourceTable 
          title="Containers" 
          subtitle="Recent active containers"
          icon={<ContainerIcon />}
          data={containers}
          type="containers"
        />

        <ResourceTable 
          title="Images" 
          subtitle="Largest local images"
          icon={<ImageIcon />}
          data={images}
          type="images"
        />
      </div>
    </div>
  )
}

// ===========================================
// Helper Components
// ===========================================

function Header({ onRefresh, onPrune, isPruning }) {
  return (
    <div className="flex flex-col md:flex-row md:items-center justify-between mb-8 gap-4">
      <div>
        <h1 className="text-3xl font-bold text-white tracking-tight">Dashboard</h1>
        <p className="text-slate-500 mt-1">Overview of your Container Environment</p>
      </div>
      
      <div className="flex items-center gap-3">
        <button 
          onClick={onRefresh}
          className="px-4 py-2 bg-[#1a1a1e] hover:bg-[#252529] border border-white/5 rounded-lg text-sm font-medium transition-all flex items-center gap-2"
        >
          <RefreshIcon />
          Refresh
        </button>
        <button 
          onClick={onPrune}
          disabled={isPruning}
          className="px-4 py-2 bg-red-500/10 hover:bg-red-500/20 text-red-400 border border-red-500/20 rounded-lg text-sm font-medium transition-all flex items-center gap-2"
        >
          <PruneIcon />
          {isPruning ? 'Pruning...' : 'Prune System'}
        </button>
      </div>
    </div>
  )
}

function SystemOverview({ system, formatBytes }) {
  const memUsagePath = (system?.memory_used / system?.memory_total) * 100 || 0
  const diskUsagePath = (system?.disk_used / system?.disk_total) * 100 || 0

  return (
    <div className="mb-10">
      <h2 className="text-lg font-semibold text-white mb-4">System Overview</h2>
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <StatCard 
          title="CPU Usage" 
          subtitle="Processor utilization"
          value={`${(system?.cpu_usage || 0).toFixed(1)}%`}
          capacity={`${system?.cpu_cores || 1} CPUs`}
          percent={Math.min(system?.cpu_usage || 0, 100)}
          color="purple"
          icon={<CPUIcon />}
          meta={`Usage: ${(system?.cpu_usage || 0).toFixed(1)}%`}
          metaAlt="Load: Low"
        />

        <StatCard 
          title="Memory Usage" 
          subtitle="RAM utilization"
          value={formatBytes(system?.memory_used || 0)}
          capacity={formatBytes(system?.memory_total || 0)}
          percent={memUsagePath}
          color="blue"
          icon={<MemoryIcon />}
          meta={`Usage: ${memUsagePath.toFixed(1)}%`}
          metaAlt={`Free: ${formatBytes((system?.memory_total - system?.memory_used) || 0)}`}
        />

        <StatCard 
          title="Disk Usage" 
          subtitle={`Storage utilization at ${system?.disk_path}`}
          value={formatBytes(system?.disk_used || 0)}
          capacity={formatBytes(system?.disk_total || 0)}
          percent={diskUsagePath}
          color="orange"
          icon={<DiskIcon />}
          meta={`Usage: ${diskUsagePath.toFixed(1)}%`}
          metaAlt={`Free: ${formatBytes((system?.disk_total - system?.disk_used) || 0)}`}
        />
      </div>
    </div>
  )
}

function StatCard({ title, subtitle, value, capacity, percent, color, icon, meta, metaAlt }) {
    const gradients = {
        purple: "from-purple-500 to-indigo-500",
        blue: "from-blue-500 to-cyan-500",
        orange: "from-orange-500 to-amber-500"
    }
    const iconBgs = {
        purple: "bg-purple-500/10 text-purple-400",
        blue: "bg-blue-500/10 text-blue-400",
        orange: "bg-orange-500/10 text-orange-400"
    }

    return (
        <div className="bg-[#111114] border border-white/[0.03] rounded-2xl p-6 shadow-2xl relative overflow-hidden group">
            <div className="flex items-center gap-4 mb-6">
                <div className={`w-12 h-12 rounded-xl flex items-center justify-center ${iconBgs[color]}`}>
                    {icon}
                </div>
                <div>
                    <h3 className="text-sm font-medium text-slate-400">{title}</h3>
                    <p className="text-xs text-slate-500 uppercase tracking-wider">{subtitle}</p>
                </div>
            </div>
            
            <div className="space-y-3">
                <div className="flex justify-between items-end">
                    <span className="text-2xl font-bold text-white">{value}</span>
                    <span className="text-xs text-slate-500">Capacity <span className="text-slate-300 ml-1">{capacity}</span></span>
                </div>
                <div className="h-2 bg-white/5 rounded-full overflow-hidden">
                    <div 
                        className={`h-full bg-gradient-to-r ${gradients[color]} rounded-full transition-all duration-1000`}
                        style={{ width: `${percent}%` }}
                    ></div>
                </div>
                <div className="flex justify-between text-[10px] text-slate-500 font-medium">
                    <span>{meta}</span>
                    <span>{metaAlt}</span>
                </div>
            </div>
        </div>
    )
}

function ResourceTable({ title, subtitle, icon, data, type }) {
    return (
        <div className="bg-[#111114] border border-white/[0.03] rounded-2xl overflow-hidden flex flex-col shadow-2xl">
            <div className="p-6 border-b border-white/[0.03] flex items-center justify-between">
                <div className="flex items-center gap-3">
                    <div className="w-10 h-10 rounded-lg bg-indigo-500/10 flex items-center justify-center text-indigo-400">
                        {icon}
                    </div>
                    <div>
                        <h2 className="text-lg font-bold text-white leading-tight">{title}</h2>
                        <p className="text-xs text-slate-500">{subtitle}</p>
                    </div>
                </div>
                <button className="text-xs font-semibold text-indigo-400 hover:text-indigo-300 transition-colors uppercase tracking-widest">View All</button>
            </div>
            
            <div className="overflow-x-auto">
                <table className="w-full text-left">
                    {type === 'containers' ? (
                        <ContainerTableBody data={data} />
                    ) : (
                        <ImageTableBody data={data} />
                    )}
                </table>
            </div>
            {data.length === 0 && (
                <div className="p-12 text-center text-slate-600">No {type} found.</div>
            )}
        </div>
    )
}

function ContainerTableBody({ data }) {
    return (
        <>
            <thead className="text-[10px] uppercase tracking-[0.15em] text-slate-600 font-bold border-b border-white/[0.03]">
                <tr>
                    <th className="px-6 py-4">Name</th>
                    <th className="px-6 py-4">Image</th>
                    <th className="px-6 py-4 text-center">State</th>
                    <th className="px-6 py-4 text-right">Status</th>
                </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.03]">
                {data.slice(0, 10).map((c) => (
                    <tr key={c.id} className="group hover:bg-white/[0.01] transition-colors">
                        <td className="px-6 py-4">
                            <div className="flex items-center gap-3">
                                <div className="w-8 h-8 rounded bg-slate-800 flex items-center justify-center text-slate-500 font-mono text-[10px]">
                                    {c.names[0]?.substring(0, 2).toUpperCase() || '??'}
                                </div>
                                <span className="text-xs font-medium text-slate-300 truncate max-w-[150px]">{c.names[0] || c.id.substring(0, 12)}</span>
                            </div>
                        </td>
                        <td className="px-6 py-4">
                            <span className="text-xs text-slate-500 font-mono italic">{c.image}</span>
                        </td>
                        <td className="px-6 py-4">
                            <div className="flex justify-center">
                                <span className={`px-2.5 py-1 rounded-full text-[10px] font-bold uppercase tracking-wider ${
                                    c.state === 'running' ? 'bg-emerald-500/10 text-emerald-500' : 'bg-red-500/10 text-red-500'
                                }`}>
                                    {c.state}
                                </span>
                            </div>
                        </td>
                        <td className="px-6 py-4 text-right">
                            <span className="text-xs text-slate-400">{c.status}</span>
                        </td>
                    </tr>
                ))}
            </tbody>
        </>
    )
}

function ImageTableBody({ data }) {
    return (
        <>
            <thead className="text-[10px] uppercase tracking-[0.15em] text-slate-600 font-bold border-b border-white/[0.03]">
                <tr>
                    <th className="px-6 py-4">Repository</th>
                    <th className="px-6 py-4 text-center">Status</th>
                    <th className="px-6 py-4 text-center">Tag</th>
                    <th className="px-6 py-4 text-right">Size</th>
                </tr>
            </thead>
            <tbody className="divide-y divide-white/[0.03]">
                {data.slice(0, 10).map((img, i) => (
                    <tr key={i} className="group hover:bg-white/[0.01] transition-colors">
                        <td className="px-6 py-4">
                            <div className="flex flex-col">
                                <span className="text-xs font-medium text-slate-300">{img.repository}</span>
                                <span className="text-[10px] text-slate-600 font-mono mt-0.5">{img.id?.substring(7, 19)}</span>
                            </div>
                        </td>
                        <td className="px-6 py-4 text-center">
                            <span className={`px-2 py-0.5 rounded text-[10px] font-bold ${
                                img.status === 'In Use' ? 'bg-teal-500/10 text-teal-500' : 'bg-slate-500/10 text-slate-500'
                            }`}>
                                {img.status}
                            </span>
                        </td>
                        <td className="px-6 py-4 text-center">
                            <span className="text-xs text-slate-500">{img.tag}</span>
                        </td>
                        <td className="px-6 py-4 text-right">
                            <span className="text-xs font-mono text-slate-400 tracking-tighter">{img.size_human}</span>
                        </td>
                    </tr>
                ))}
            </tbody>
        </>
    )
}

// ===========================================
// Icons
// ===========================================

const ContainerIcon = () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4" />
    </svg>
)

const ImageIcon = () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16l4.586-4.586a2 2 0 012.828 0L16 16m-2-2l1.586-1.586a2 2 0 012.828 0L20 14m-6-6h.01M6 20h12a2 2 0 002-2V6a2 2 0 00-2-2H6a2 2 0 00-2 2v12a2 2 0 002 2z" />
    </svg>
)

const RefreshIcon = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
    </svg>
)

const PruneIcon = () => (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
    </svg>
)

const CPUIcon = () => (
    <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" />
    </svg>
)

const MemoryIcon = () => (
    <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19.428 15.428a2 2 0 00-1.022-.547l-2.387-.477a6 6 0 00-3.86.517l-.318.158a6 6 0 01-3.86.517L6.05 15.21a2 2 0 00-1.022.547l-2.387 2.387a2 2 0 002.828 2.828l2.387-2.387a2 2 0 011.414-.586l2.387.477a6 6 0 003.86-.517l.318-.158a6 6 0 013.86-.517l2.387.477a2 2 0 002.828-2.828l-2.387-2.387z" />
    </svg>
)

const DiskIcon = () => (
    <svg className="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
    </svg>
)

export default AdminDashboard
