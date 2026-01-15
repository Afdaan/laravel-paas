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

function AdminProjects() {
  const [projects, setProjects] = useState([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  
  useEffect(() => {
    fetchProjects()
  }, [page, search, statusFilter])
  
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
          className="flex-1 px-4 py-2 border"
        />
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          className="px-4 py-2 border"
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
          <table>
            <thead>
              <tr className="border-b border-slate-700">
                <th>Project</th>
                <th>Owner</th>
                <th>Status</th>
                <th>Laravel</th>
                <th>Created</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {projects.map((project) => (
                <tr key={project.id}>
                  <td>
                    <div>
                      <p className="font-medium text-white">{project.name}</p>
                      <p className="text-sm text-slate-500">{project.subdomain}</p>
                    </div>
                  </td>
                  <td>
                    <p className="text-slate-300">{project.user?.name || 'Unknown'}</p>
                    <p className="text-sm text-slate-500">{project.user?.email}</p>
                  </td>
                  <td><StatusBadge status={project.status} /></td>
                  <td className="text-slate-400">
                    {project.laravel_version ? `v${project.laravel_version}` : '-'}
                  </td>
                  <td className="text-slate-400">
                    {new Date(project.created_at).toLocaleDateString()}
                  </td>
                  <td>
                    <Link 
                      to={`/projects/${project.id}`}
                      className="text-primary-400 hover:text-primary-300"
                    >
                      View →
                    </Link>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
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
