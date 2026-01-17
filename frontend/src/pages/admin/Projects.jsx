// ===========================================
// Admin Projects Page
// ===========================================

import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import toast from 'react-hot-toast'
import { projectsAPI } from '../../services/api'

function StatusBadge({ status }) {
  const statusClasses = {
    pending: 'badge-pending',
    building: 'badge-building',
    running: 'badge-running',
    failed: 'badge-failed',
    stopped: 'badge-stopped',
  }
  return <span className={`badge ${statusClasses[status]}`}>{status}</span>
}


function ResourceBar({ label, value, max, unit, suffix = '' }) {
  const percentage = max ? Math.min((value / max) * 100, 100) : Math.min(value, 100)
  
  let color = 'bg-emerald-500'
  if (percentage > 80) color = 'bg-red-500'
  else if (percentage > 50) color = 'bg-amber-500'

  return (
    <div className="w-32 text-xs">
      <div className="flex justify-between mb-0.5">
        <span className="text-slate-500 font-medium">{label}</span>
        <span className="text-slate-300">
          {typeof value === 'number' ? value.toFixed(1) : value}{suffix}
          {max ? <span className="text-slate-500">/{max.toFixed(0)}{suffix}</span> : ''}
        </span>
      </div>
      <div className="h-1.5 w-full bg-slate-700/50 rounded-full overflow-hidden">
        <div 
          className={`h-full ${color} transition-all duration-500`}
          style={{ width: `${percentage}%` }}
        />
      </div>
    </div>
  )
}

function AdminProjects() {
  const [projects, setProjects] = useState([])
  const [stats, setStats] = useState({})
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  
  useEffect(() => {
    fetchProjects()
  }, [page, search, statusFilter])

  // Poll stats every 5 seconds
  useEffect(() => {
    const fetchStats = async () => {
      try {
        const response = await projectsAPI.listStats()
        setStats(response.data.stats || {})
      } catch (error) {
        console.error("Failed to fetch stats", error)
      }
    }

    fetchStats() // Initial fetch
    const interval = setInterval(fetchStats, 5000)
    return () => clearInterval(interval)
  }, [])
  
  const fetchProjects = async () => {
    setIsLoading(true)
    try {
      const response = await projectsAPI.listAll({ page, search, status: statusFilter, limit: 10 })
      setProjects(response.data.data || [])
      setTotal(response.data.total || 0)
    } catch (error) {
      toast.error('Failed to fetch projects')
    } finally {
      setIsLoading(false)
    }
  }
  
  const totalPages = Math.ceil(total / 10)
  
  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold text-white">All Projects</h1>
        <p className="text-slate-400">View and manage all deployed projects</p>
      </div>
      
      {/* Filters */}
      <div className="flex gap-4">
        <input
          type="text"
          placeholder="Search projects..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="flex-1 px-4 py-2 border rounded-lg bg-slate-800 border-slate-700 text-white focus:outline-none focus:border-primary-500"
        />
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          className="px-4 py-2 border rounded-lg bg-slate-800 border-slate-700 text-white focus:outline-none focus:border-primary-500"
        >
          <option value="">All Status</option>
          <option value="running">Running</option>
          <option value="building">Building</option>
          <option value="pending">Pending</option>
          <option value="failed">Failed</option>
          <option value="stopped">Stopped</option>
        </select>
      </div>
      
      {/* Projects Table */}
      <div className="card overflow-hidden">
        {isLoading ? (
          <div className="p-12 text-center">
            <div className="animate-spin rounded-full h-8 w-8 border-t-2 border-b-2 border-primary-500 mx-auto"></div>
          </div>
        ) : projects.length === 0 ? (
          <div className="p-12 text-center text-slate-400">No projects found</div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b border-slate-700 text-left">
                  <th className="p-4 text-slate-400 font-medium text-sm">PROJECT</th>
                  <th className="p-4 text-slate-400 font-medium text-sm">OWNER</th>
                  <th className="p-4 text-slate-400 font-medium text-sm">STATUS</th>
                  <th className="p-4 text-slate-400 font-medium text-sm">RESOURCES</th>
                  <th className="p-4 text-slate-400 font-medium text-sm">LARAVEL</th>
                  <th className="p-4 text-slate-400 font-medium text-sm">CREATED</th>
                  <th className="p-4"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-700">
                {projects.map((project) => {
                  const hasStats = stats[project.id]
                  return (
                    <tr key={project.id} className="hover:bg-slate-800/50">
                      <td className="p-4">
                        <div>
                          <p className="font-medium text-white">{project.name}</p>
                          <p className="text-xs text-slate-500 font-mono">{project.subdomain}</p>
                        </div>
                      </td>
                      <td className="p-4">
                        <div>
                          <p className="text-slate-300 text-sm">{project.user?.name || 'Unknown'}</p>
                          <p className="text-xs text-slate-500">{project.user?.email}</p>
                        </div>
                      </td>
                      <td className="p-4"><StatusBadge status={project.status} /></td>
                      <td className="p-4 space-y-2">
                        {project.status === 'running' && hasStats ? (
                          <>
                            <ResourceBar label="CPU" value={hasStats.cpu_percent} suffix="%" />
                            <ResourceBar label="RAM" value={hasStats.memory_mb} max={hasStats.memory_max_mb} suffix="MB" />
                          </>
                        ) : project.status === 'running' ? (
                          <div className="text-xs text-slate-500 animate-pulse">Loading stats...</div>
                        ) : (
                          <div className="text-xs text-slate-600">-</div>
                        )}
                      </td>
                      <td className="p-4 text-slate-400 text-sm">
                        {project.laravel_version ? `v${project.laravel_version}` : '-'}
                      </td>
                      <td className="p-4 text-slate-400 text-sm">
                        {new Date(project.created_at).toLocaleDateString()}
                      </td>
                      <td className="p-4 text-right">
                        <Link 
                          to={`/projects/${project.id}`}
                          className="text-primary-400 hover:text-primary-300 text-sm font-medium"
                        >
                          View →
                        </Link>
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
        
        {/* Pagination */}
        {totalPages > 1 && (
          <div className="p-4 border-t border-slate-700 flex justify-between items-center">
            <p className="text-slate-400 text-sm">
              Showing {(page - 1) * 10 + 1} to {Math.min(page * 10, total)} of {total}
            </p>
            <div className="flex gap-2">
              <button
                onClick={() => setPage(p => Math.max(1, p - 1))}
                disabled={page === 1}
                className="btn btn-secondary text-sm disabled:opacity-50"
              >
                Previous
              </button>
              <button
                onClick={() => setPage(p => Math.min(totalPages, p + 1))}
                disabled={page === totalPages}
                className="btn btn-secondary text-sm disabled:opacity-50"
              >
                Next
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}

export default AdminProjects
