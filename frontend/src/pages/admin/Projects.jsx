// ===========================================
// Admin Projects Page (Cluster Management)
// ===========================================

import { useState, useEffect, useCallback, useMemo } from 'react'
import { Link } from 'react-router-dom'
import toast from 'react-hot-toast'
import { projectsAPI } from '../../services/api'
import { 
  ExternalLink, 
  Globe, 
  User, 
  Layers, 
  Search, 
  ChevronLeft, 
  ChevronRight,
  Activity,
  Cpu,
  HardDrive,
  Info,
  Box,
  Monitor,
  RefreshCw
} from 'lucide-react'

const StatusBadge = ({ status }) => {
  const configs = {
    running: { 
      color: 'bg-emerald-500/5 text-emerald-400 border-emerald-500/20 shadow-[0_0_10px_rgba(16,185,129,0.1)]', 
      label: 'Operational', 
      glow: 'bg-emerald-400' 
    },
    building: { 
      color: 'bg-indigo-500/5 text-indigo-400 border-indigo-500/20 animate-pulse', 
      label: 'Orchestrating', 
      glow: 'bg-indigo-400' 
    },
    pending: { 
      color: 'bg-amber-500/5 text-amber-400 border-amber-500/20', 
      label: 'Queued', 
      glow: 'bg-amber-400' 
    },
    failed: { 
      color: 'bg-rose-500/5 text-rose-400 border-rose-500/20', 
      label: 'Degraded', 
      glow: 'bg-rose-400' 
    },
    stopped: { 
      color: 'bg-slate-500/5 text-slate-600 dark:text-slate-400 border-slate-200 dark:border-white/5', 
      label: 'Halted', 
      glow: 'bg-slate-500' 
    },
  }

  const config = configs[status] || configs.pending

  return (
    <span className={`px-4 py-1.5 rounded-full border text-[9px] font-black uppercase tracking-widest inline-flex items-center gap-2 ${config.color}`}>
      <div className={`w-1 h-1 rounded-full ${config.glow} ${status === 'running' || status === 'building' ? 'animate-pulse' : ''}`} />
      {config.label}
    </span>
  )
}

const ResourceBar = ({ label, value, max, icon: Icon, suffix = '' }) => {
  const percentage = max ? Math.min((value / max) * 100, 100) : Math.min(value, 100)
  
  let color = 'from-emerald-500 to-teal-500'
  if (percentage > 85) color = 'from-rose-500 to-pink-500'
  else if (percentage > 60) color = 'from-amber-500 to-orange-500'

  return (
    <div className="flex flex-col gap-2 w-40 group/res">
      <div className="flex items-center justify-between text-[8px] font-black uppercase tracking-widest text-slate-600 dark:text-slate-400 group-hover/res:text-slate-600 dark:text-slate-400 transition-colors">
        <div className="flex items-center gap-1.5">
           <Icon className="w-3 h-3" />
           {label}
        </div>
        <span className="text-slate-600 dark:text-slate-400 font-mono">
          {typeof value === 'number' ? value.toFixed(1) : value}{suffix}
        </span>
      </div>
      <div className="h-1 w-full bg-slate-100 dark:bg-slate-100 dark:bg-white/5 rounded-full overflow-hidden border border-white/[0.02]">
        <div 
          className={`h-full bg-gradient-to-r ${color} transition-all duration-1000 ease-out shadow-[0_0_8px_rgba(0,0,0,0.5)]`}
          style={{ width: `${percentage}%` }}
        />
      </div>
    </div>
  )
}

const AdminProjects = () => {
  const [projects, setProjects] = useState([])
  const [stats, setStats] = useState({})
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  
  const fetchProjects = useCallback(async () => {
    setIsLoading(true)
    try {
      const response = await projectsAPI.listAll({ page, search, status: statusFilter, limit: 12 })
      setProjects(response.data.data || [])
      setTotal(response.data.total || 0)
    } catch (error) {
      toast.error('Failed to index projects')
    } finally {
      setIsLoading(false)
    }
  }, [page, search, statusFilter])

  useEffect(() => {
    fetchProjects()
  }, [fetchProjects])

  // Poll stats every 8 seconds
  useEffect(() => {
    const fetchStats = async () => {
      try {
        const response = await projectsAPI.listStats()
        setStats(response.data.stats || {})
      } catch (error) {
        console.error("Telemetry failure", error)
      }
    }

    fetchStats()
    const interval = setInterval(fetchStats, 8000)
    return () => clearInterval(interval)
  }, [])
  
  const totalPages = Math.ceil(total / 12)
  
  return (
    <div className="space-y-12 animate-pop-in relative h-full">
      {/* Background Glows */}
      

      <div className="relative z-10">
          {/* Header */}
          <div className="flex flex-col xl:flex-row xl:items-end justify-between mb-12 gap-8">
            <div>
              <h1 className="text-5xl font-black text-slate-900 dark:text-white tracking-tighter mb-4 italic text-transparent bg-clip-text pr-2 pb-1 bg-gradient-to-r from-indigo-400 to-purple-400">Projects</h1>
              <p className="text-slate-600 dark:text-slate-400 text-lg font-medium">Manage all projects and system resources across the platform.</p>
            </div>

            <div className="flex items-center gap-4 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 p-2 rounded-2xl backdrop-blur-md">
                 <div className="flex items-center gap-3 px-6 py-2 border-r border-slate-200 dark:border-white/5">
                    <Activity className="w-4 h-4 text-emerald-500" />
                    <span className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-600 dark:text-slate-400">{total} Monitored</span>
                 </div>
                 <div className="flex items-center gap-3 px-6 py-2">
                    <Monitor className="w-4 h-4 text-indigo-500" />
                    <span className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-600 dark:text-slate-400">{projects.filter(p => p.status === 'running').length} Active Nodes</span>
                 </div>
            </div>
          </div>
          
          {/* Toolbar */}
          <div className="flex flex-col md:flex-row items-center justify-between mb-8 gap-6 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 p-4 rounded-3xl backdrop-blur-md shadow-2xl">
            <div className="flex items-center gap-4 flex-1 w-full max-w-2xl">
                <div className="relative flex-1 group">
                    <div className="absolute inset-y-0 left-0 pl-5 flex items-center pointer-events-none">
                        <Search className="w-4 h-4 text-slate-600 dark:text-slate-400 group-focus-within:text-indigo-400 transition-colors" />
                    </div>
                    <input 
                        type="text"
                        placeholder="Search projects, domains, owners..."
                        value={search}
                        onChange={(e) => setSearch(e.target.value)}
                        className="w-full bg-black/40 border border-slate-200 dark:border-white/5 rounded-2xl py-3.5 pl-12 pr-5 text-sm font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all placeholder:text-slate-600 outline-none"
                    />
                </div>
            </div>

            <div className="flex items-center gap-4">
                <select
                  value={statusFilter}
                  onChange={(e) => setStatusFilter(e.target.value)}
                  className="bg-black/40 border border-slate-300 dark:border-white/10 rounded-2xl px-6 py-3.5 text-[10px] font-black uppercase tracking-widest text-slate-600 dark:text-slate-400 outline-none focus:border-indigo-500 transition-all cursor-pointer"
                >
                  <option value="">Status: All Lifecycle</option>
                  <option value="running">In Production</option>
                  <option value="building">Provisioning</option>
                  <option value="pending">Queued</option>
                  <option value="failed">Degraded</option>
                  <option value="stopped">Halted</option>
                </select>

                <button onClick={fetchProjects} className="w-12 h-12 flex items-center justify-center rounded-2xl bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 text-slate-900 dark:text-white hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5 transition-all active:scale-95">
                   <RefreshCw className="w-4 h-4" />
                </button>
            </div>
          </div>
          
          {/* Projects Table */}
          <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm overflow-hidden border-slate-300 dark:border-white/10 shadow-[0_30px_60px_rgba(0,0,0,0.4)] bg-slate-50 dark:bg-slate-100 dark:bg-white/5">
            <div className="overflow-x-auto">
                <table className="premium-table">
                  <thead>
                    <tr>
                      <th>Project</th>
                      <th>Owner</th>
                      <th>Status</th>
                      <th>Resource Usage</th>
                      <th>Laravel Version</th>
                      <th className="text-right">Action</th>
                    </tr>
                  </thead>
                  <tbody>
                    {projects.map((project) => {
                      const hasStats = stats[project.id]
                      return (
                        <tr key={project.id} className="group hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5">
                          <td>
                            <div className="flex items-center gap-5">
                              <div className="w-12 h-12 rounded-2xl bg-slate-100 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 flex items-center justify-center text-slate-600 dark:text-slate-400 group-hover:border-indigo-500/40 group-hover:text-indigo-400 group-hover:bg-indigo-500/5 transition-all duration-500 overflow-hidden relative">
                                {project.status === 'running' && (
                                    <div className="absolute inset-0 bg-gradient-to-tr from-emerald-500/5 to-transparent opacity-0 group-hover:opacity-100 transition-opacity" />
                                )}
                                <Globe className="w-6 h-6 z-10" />
                              </div>
                              <div className="flex flex-col">
                                <div className="flex items-center gap-2">
                                  <span className="text-sm font-black text-slate-900 dark:text-white group-hover:text-indigo-400 transition-colors uppercase tracking-tight truncate max-w-[180px]">{project.name}</span>
                                  {project.status === 'running' && project.url && (
                                    <a 
                                      href={project.url} 
                                      target="_blank" 
                                      rel="noopener noreferrer"
                                      className="text-slate-600 hover:text-emerald-400 transition-all active:scale-90"
                                      title="Open live website"
                                    >
                                      <ExternalLink size={14} />
                                    </a>
                                  )}
                                </div>
                                <span className="text-[10px] text-slate-600 font-mono tracking-widest truncate max-w-[180px]">{project.subdomain}</span>
                              </div>
                            </div>
                          </td>
                          <td>
                            <div className="flex items-center gap-3">
                              <div className="w-7 h-7 rounded-full bg-slate-100 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 flex items-center justify-center text-slate-600 dark:text-slate-400 group-hover:text-slate-900 dark:text-white transition-colors">
                                 <User className="w-3.5 h-3.5" />
                              </div>
                              <div className="flex flex-col">
                                <span className="text-[11px] font-black text-slate-700 dark:text-slate-300 uppercase tracking-tight">{project.user?.name || 'Cluster Sync'}</span>
                                <span className="text-[9px] text-slate-600 font-medium lowercase tracking-tighter">{project.user?.email}</span>
                              </div>
                            </div>
                          </td>
                          <td><StatusBadge status={project.status} /></td>
                          <td className="space-y-3">
                            {project.status === 'running' && hasStats ? (
                              <div className="flex items-center gap-6">
                                <ResourceBar label="CPU" value={hasStats.cpu_percent} icon={Cpu} suffix="%" />
                                <ResourceBar label="MEM" value={hasStats.memory_mb} max={hasStats.memory_max_mb} icon={HardDrive} suffix="MB" />
                              </div>
                            ) : project.status === 'running' ? (
                              <div className="flex items-center gap-3 text-slate-600 text-[9px] font-black uppercase tracking-widest pl-2">
                                <Activity className="w-3.5 h-3.5 animate-pulse text-indigo-500/40" />
                                Awaiting Sync...
                              </div>
                            ) : (
                              <div className="h-0.5 w-12 bg-slate-100 dark:bg-white/5 rounded-full" />
                            )}
                          </td>
                          <td>
                            <div className="flex items-center gap-2">
                                <div className="w-7 h-7 rounded-xl bg-slate-100 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 flex items-center justify-center text-[10px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-tighter group-hover:border-rose-500/20 group-hover:text-rose-400 transition-all">
                                    L
                                </div>
                                <span className="text-[11px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-widest">
                                  {project.laravel_version ? `v${project.laravel_version}` : 'Static'}
                                </span>
                            </div>
                          </td>
                          <td className="text-right">
                            <Link 
                              to={`/projects/${project.id}`}
                              className="px-6 py-2 rounded-xl bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 text-[10px] font-black uppercase tracking-[0.2em] text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5 hover:border-slate-300 dark:border-white/10 transition-all active:scale-95 inline-block"
                            >
                              View Details →
                            </Link>
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
            </div>

            {projects.length === 0 && (
                <div className="py-32 flex flex-col items-center justify-center text-center">
                     <div className="w-24 h-24 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 rounded-[2.5rem] flex items-center justify-center mb-8 shadow-2xl">
                        <Box className="w-10 h-10 text-slate-800" />
                    </div>
                    <h3 className="text-[10px] font-black uppercase tracking-[0.4em] text-slate-600">No projects found in the system</h3>
                </div>
            )}
            
            {/* Pagination Area */}
            {totalPages > 1 && (
              <div className="p-10 border-t border-slate-200 dark:border-white/5 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 flex flex-col md:flex-row justify-between items-center gap-8">
                <div className="flex items-center gap-3">
                    <Info className="w-4 h-4 text-indigo-500" />
                    <span className="text-[10px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-widest">Showing {(page - 1) * 12 + 1} to {Math.min(page * 12, total)} of {total} indexed services.</span>
                </div>
                <div className="flex items-center gap-3 bg-black/40 border border-slate-200 dark:border-white/5 p-1.5 rounded-2xl">
                  <button
                    onClick={() => setPage(p => Math.max(1, p - 1))}
                    disabled={page === 1}
                    className="w-12 h-12 flex items-center justify-center rounded-xl bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 text-slate-900 dark:text-white hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5 disabled:opacity-20 disabled:cursor-not-allowed transition-all"
                  >
                    <ChevronLeft size={20} />
                  </button>
                  <div className="px-6 font-mono text-xs font-black text-indigo-400">
                     {page} <span className="text-slate-700 mx-2">/</span> {totalPages}
                  </div>
                  <button
                    onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                    disabled={page === totalPages}
                    className="w-12 h-12 flex items-center justify-center rounded-xl bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 text-slate-900 dark:text-white hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5 disabled:opacity-20 disabled:cursor-not-allowed transition-all"
                  >
                    <ChevronRight size={20} />
                  </button>
                </div>
              </div>
            )}
          </div>
      </div>
    </div>
  )
}

export default AdminProjects
