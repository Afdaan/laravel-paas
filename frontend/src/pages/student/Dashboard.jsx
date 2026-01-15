// ===========================================
// Student Dashboard Page
// ===========================================

import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { projectsAPI } from '../../services/api'
import useAuthStore from '../../stores/authStore'

// Status badge component
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

function StudentDashboard() {
  const { user } = useAuthStore()
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
      console.error('Failed to fetch projects:', error)
    } finally {
      setIsLoading(false)
    }
  }
  
  const runningProjects = projects.filter(p => p.status === 'running').length
  const totalProjects = projects.length
  
  return (
    <div className="space-y-8">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold text-white">
          Welcome back, {user?.name?.split(' ')[0]}! 👋
        </h1>
        <p className="text-slate-400 mt-1">
          Here's an overview of your deployed projects.
        </p>
      </div>
      
      {/* Stats Cards */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <div className="card p-6">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-slate-400 text-sm">Total Projects</p>
              <p className="text-3xl font-bold text-white mt-1">{totalProjects}</p>
            </div>
            <div className="w-12 h-12 bg-primary-600/20 rounded-xl flex items-center justify-center">
              <span className="text-2xl">📦</span>
            </div>
          </div>
        </div>
        
        <div className="card p-6">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-slate-400 text-sm">Running</p>
              <p className="text-3xl font-bold text-emerald-400 mt-1">{runningProjects}</p>
            </div>
            <div className="w-12 h-12 bg-emerald-600/20 rounded-xl flex items-center justify-center">
              <span className="text-2xl">✅</span>
            </div>
          </div>
        </div>
        
        <div className="card p-6">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-slate-400 text-sm">Quick Action</p>
              <Link 
                to="/projects/new" 
                className="btn btn-primary mt-2 inline-block"
              >
                + New Project
              </Link>
            </div>
            <div className="w-12 h-12 bg-purple-600/20 rounded-xl flex items-center justify-center">
              <span className="text-2xl">🚀</span>
            </div>
          </div>
        </div>
      </div>
      
      {/* Recent Projects */}
      <div className="card">
        <div className="p-6 border-b border-slate-700">
          <div className="flex items-center justify-between">
            <h2 className="text-xl font-semibold text-white">My Projects</h2>
            <Link to="/projects" className="text-primary-400 hover:text-primary-300 text-sm">
              View all →
            </Link>
          </div>
        </div>
        
        {isLoading ? (
          <div className="p-12 text-center">
            <div className="animate-spin rounded-full h-8 w-8 border-t-2 border-b-2 border-primary-500 mx-auto"></div>
          </div>
        ) : projects.length === 0 ? (
          <div className="p-12 text-center">
            <div className="text-4xl mb-4">📭</div>
            <p className="text-slate-400">No projects yet</p>
            <Link to="/projects/new" className="btn btn-primary mt-4 inline-block">
              Deploy your first project
            </Link>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table>
              <thead>
                <tr className="border-b border-slate-700">
                  <th>Name</th>
                  <th>URL</th>
                  <th>Status</th>
                  <th>Created</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                {projects.slice(0, 5).map((project) => (
                  <tr key={project.id}>
                    <td className="font-medium text-white">{project.name}</td>
                    <td>
                      {project.status === 'running' ? (
                        <a 
                          href={`https://${project.subdomain}.${window.location.hostname}`}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="text-primary-400 hover:text-primary-300"
                        >
                          {project.subdomain}
                        </a>
                      ) : (
                        <span className="text-slate-500">{project.subdomain}</span>
                      )}
                    </td>
                    <td><StatusBadge status={project.status} /></td>
                    <td className="text-slate-400">
                      {new Date(project.created_at).toLocaleDateString()}
                    </td>
                    <td>
                      <Link 
                        to={`/projects/${project.id}`}
                        className="text-primary-400 hover:text-primary-300"
                      >
                        Details →
                      </Link>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

export default StudentDashboard
