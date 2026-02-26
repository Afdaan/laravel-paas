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
            <div className="w-12 h-12 bg-primary-600/20 rounded-xl flex items-center justify-center text-primary-400">
              <svg className="w-6 h-6" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M2.25 13.5h3.86a2.25 2.25 0 012.012 1.244l.256.512a2.25 2.25 0 002.013 1.244h3.218a2.25 2.25 0 002.013-1.244l.256-.512a2.25 2.25 0 012.013-1.244h3.859m-19.5.375a3 3 0 003 3h15a3 3 0 003-3m-18-6h15a3 3 0 013 3v.375m-18-3.375A3 3 0 003 10.875v.375m18-3.375V6a3 3 0 00-3-3H6a3 3 0 00-3 3v.375m3 0V6" />
              </svg>
            </div>
          </div>
        </div>
        
        <div className="card p-6">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-slate-400 text-sm">Running</p>
              <p className="text-3xl font-bold text-emerald-400 mt-1">{runningProjects}</p>
            </div>
            <div className="w-12 h-12 bg-emerald-600/20 rounded-xl flex items-center justify-center text-emerald-400">
              <svg className="w-6 h-6" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75L11.25 15 15 9.75M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
            </div>
          </div>
        </div>
        
        <div className="card p-6">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-slate-400 text-sm">Quick Action</p>
              <Link 
                to="/projects/new" 
                className="btn btn-primary mt-2 inline-block px-4 py-1 text-sm font-semibold rounded-lg"
              >
                + New Project
              </Link>
            </div>
            <div className="w-12 h-12 bg-purple-600/20 rounded-xl flex items-center justify-center text-purple-400">
              <svg className="w-6 h-6" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M15.59 14.37a6 6 0 01-5.84 7.38v-4.8m5.84-2.58a14.98 14.98 0 006.16-12.12A14.98 14.98 0 009.631 8.41m5.96 5.96a14.926 14.926 0 01-5.841 2.58m-.119-8.54a6 6 0 00-7.381 5.84h4.8m2.581-5.84a14.927 14.927 0 00-2.58 5.84m2.699-2.7c-.91.91-1.076 2.05-.365 2.71c.71.71 1.85.545 2.76-.365" />
              </svg>
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
            <div className="w-16 h-16 bg-slate-800 rounded-full flex items-center justify-center mx-auto mb-4 text-slate-500">
              <svg className="w-8 h-8" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M20.25 7.5L12 12L3.75 7.5M12 12V21m-6.75-13.5L12 3l6.75 4.5M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-19.5 0A2.25 2.25 0 004.5 15h15a2.25 2.25 0 002.25-2.25" />
              </svg>
            </div>
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
