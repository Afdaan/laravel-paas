// ===========================================
// Student Projects List Page
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
  
  return (
    <span className={`badge ${statusClasses[status] || 'badge-pending'}`}>
      {status}
    </span>
  )
}

function StudentProjects() {
  const [projects, setProjects] = useState([])
  const [isLoading, setIsLoading] = useState(true)
  
  useEffect(() => {
    fetchProjects()
  }, [])
  
  const fetchProjects = async () => {
    try {
      const response = await projectsAPI.listOwn()
      setProjects(response.data.data || [])
    } catch (error) {
      toast.error('Failed to fetch projects')
    } finally {
      setIsLoading(false)
    }
  }
  
  const handleRedeploy = async (id) => {
    if (!confirm('Are you sure you want to redeploy this project?')) return
    
    try {
      await projectsAPI.redeploy(id)
      toast.success('Redeployment started')
      fetchProjects()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Redeploy failed')
    }
  }
  
  const handleDelete = async (id) => {
    if (!confirm('Are you sure you want to delete this project? This action cannot be undone.')) return
    
    try {
      await projectsAPI.delete(id)
      toast.success('Project deleted')
      fetchProjects()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Delete failed')
    }
  }
  
  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-white">My Projects</h1>
          <p className="text-slate-400">Manage your deployed Laravel applications</p>
        </div>
        <Link to="/projects/new" className="btn btn-primary">
          + New Project
        </Link>
      </div>
      
      {/* Projects Grid */}
      {isLoading ? (
        <div className="p-12 text-center">
          <div className="animate-spin rounded-full h-8 w-8 border-t-2 border-b-2 border-primary-500 mx-auto"></div>
        </div>
      ) : projects.length === 0 ? (
        <div className="card p-12 text-center">
          <div className="text-6xl mb-4">🚀</div>
          <h2 className="text-xl font-semibold text-white mb-2">No projects yet</h2>
          <p className="text-slate-400 mb-6">Deploy your first Laravel application</p>
          <Link to="/projects/new" className="btn btn-primary">
            Create Project
          </Link>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {projects.map((project) => (
            <div key={project.id} className="card overflow-hidden">
              {/* Status Bar */}
              <div className={`h-1 ${
                project.status === 'running' ? 'bg-emerald-500' :
                project.status === 'building' ? 'bg-blue-500 animate-pulse' :
                project.status === 'failed' ? 'bg-red-500' :
                'bg-slate-600'
              }`} />
              
              <div className="p-6">
                <div className="flex items-start justify-between mb-4">
                  <div>
                    <h3 className="text-lg font-semibold text-white">{project.name}</h3>
                    <p className="text-sm text-slate-400">{project.subdomain}</p>
                  </div>
                  <StatusBadge status={project.status} />
                </div>
                
                {/* Info */}
                <div className="space-y-2 text-sm text-slate-400 mb-4">
                  {project.laravel_version && (
                    <p>Laravel {project.laravel_version} • PHP {project.php_version}</p>
                  )}
                  <p>Database: {project.database_name}</p>
                  <p>Created: {new Date(project.created_at).toLocaleDateString()}</p>
                </div>
                
                {/* Actions */}
                <div className="flex gap-2">
                  <Link 
                    to={`/projects/${project.id}`}
                    className="btn btn-secondary flex-1 text-center text-sm"
                  >
                    Details
                  </Link>
                  
                  {project.status === 'running' && (
                    <a 
                      href={`https://${project.subdomain}.${window.location.hostname}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="btn btn-primary text-sm"
                    >
                      Open ↗
                    </a>
                  )}
                </div>
                
                {/* Quick Actions */}
                <div className="flex gap-2 mt-3 pt-3 border-t border-slate-700">
                  <button 
                    onClick={() => handleRedeploy(project.id)}
                    className="text-sm text-primary-400 hover:text-primary-300"
                  >
                    Redeploy
                  </button>
                  <span className="text-slate-600">|</span>
                  <button 
                    onClick={() => handleDelete(project.id)}
                    className="text-sm text-red-400 hover:text-red-300"
                  >
                    Delete
                  </button>
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export default StudentProjects
