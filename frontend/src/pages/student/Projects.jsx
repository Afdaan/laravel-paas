// ===========================================
// Student Projects List Page
// ===========================================

import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import toast from 'react-hot-toast'
import { projectsAPI } from '../../services/api'

function StatusDot({ status }) {
  const styles = {
    running: 'bg-emerald-500',
    building: 'bg-blue-500 animate-pulse',
    failed: 'bg-red-500',
    stopped: 'bg-slate-500',
    pending: 'bg-slate-500',
  }
  return <div className={`w-2.5 h-2.5 rounded-full ${styles[status] || styles.pending}`} />
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
  
  const handleRedeploy = async (id, e) => {
    e.preventDefault()
    if (!confirm('Are you sure you want to redeploy this project?')) return

    toast.promise(
      projectsAPI.redeploy(id),
      {
        loading: 'Starting redeploy...',
        success: 'Redeploy started',
        error: 'Failed to redeploy'
      }
    ).then(fetchProjects)
  }
  
  const handleDelete = async (id, e) => {
    e.preventDefault()
    if (!confirm('DANGER: This will permanently delete the project and all its data. Continue?')) return
    
    try {
      await projectsAPI.delete(id)
      toast.success('Project deleted')
      fetchProjects()
    } catch (error) {
      toast.error('Delete failed')
    }
  }
  
  return (
    <div className="space-y-6 max-w-7xl mx-auto">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-slate-800 pb-6">
        <div>
          <h1 className="text-3xl font-bold text-white tracking-tight">My Workloads</h1>
          <p className="text-slate-400 mt-1">Manage your deployed applications</p>
        </div>
        <Link to="/projects/new" className="btn btn-primary shadow-lg shadow-primary-500/20">
          <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="mr-2"><line x1="12" x2="12" y1="5" y2="19"/><line x1="5" x2="19" y1="12" y2="12"/></svg>
          Deploy New
        </Link>
      </div>
      
      {/* Projects Grid */}
      {isLoading ? (
        <div className="flex justify-center p-12">
          <div className="animate-spin rounded-full h-8 w-8 border-t-2 border-b-2 border-primary-500"></div>
        </div>
      ) : projects.length === 0 ? (
        <div className="card p-12 text-center border-dashed border-2 border-slate-700 bg-transparent">
          <div className="text-6xl mb-4 grayscale opacity-50">📦</div>
          <h2 className="text-xl font-semibold text-white mb-2">No active workloads</h2>
          <p className="text-slate-400 mb-6">Deploy a Laravel application to get started.</p>
          <Link to="/projects/new" className="btn btn-primary">
            Create Project
          </Link>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {projects.map((project) => (
            <Link 
              key={project.id} 
              to={`/projects/${project.id}`}
              className="card p-0 group hover:border-primary-500/50 transition-all duration-300 overflow-hidden"
            >
              <div className="p-5">
                <div className="flex items-start justify-between mb-4">
                  <div className="flex items-center gap-3">
                     <StatusDot status={project.status} />
                     <div>
                       <h3 className="font-bold text-white text-lg group-hover:text-primary-400 transition-colors">{project.name}</h3>
                       <div className="text-xs text-slate-500 font-mono">{project.subdomain}</div>
                     </div>
                  </div>
                  {project.status === 'running' && (
                    <a 
                      href={project.url}
                      target="_blank"
                      rel="noopener noreferrer" 
                      onClick={(e) => e.stopPropagation()}
                      className="text-slate-400 hover:text-primary-400 p-1"
                    >
                       <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><polyline points="15 3 21 3 21 9"/><line x1="10" x2="21" y1="14" y2="3"/></svg>
                    </a>
                  )}
                </div>

                <div className="grid grid-cols-2 gap-4 py-4 border-t border-b border-slate-800/50">
                   <div>
                      <div className="text-xs text-slate-500 uppercase font-semibold">PHP Version</div>
                      <div className="text-sm text-slate-300 flex items-center gap-1">
                        {project.php_version || 'Detecting'}
                        {project.is_manual_version && <span className="text-amber-500" title="Manual">•</span>}
                      </div>
                   </div>
                   <div>
                      <div className="text-xs text-slate-500 uppercase font-semibold">Database</div>
                      <div className="text-sm text-slate-300 truncate">{project.database_name}</div>
                   </div>
                </div>

                <div className="flex items-center justify-between mt-4 pt-1">
                   <div className="text-xs text-slate-500">
                      {new Date(project.created_at).toLocaleDateString()}
                   </div>
                   <div className="flex gap-2 opacity-0 group-hover:opacity-100 transition-opacity">
                      <button 
                        onClick={(e) => handleRedeploy(project.id, e)}
                        className="p-1.5 hover:bg-slate-700 rounded text-slate-400 hover:text-white"
                        title="Redeploy"
                      >
                         <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 12a9 9 0 0 0-9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/><path d="M3 12a9 9 0 0 0 9 9 9.75 9.75 0 0 0 6.74-2.74L21 16"/><path d="M21 21v-5h-5"/></svg>
                      </button>
                      <button 
                        onClick={(e) => handleDelete(project.id, e)}
                        className="p-1.5 hover:bg-slate-700 rounded text-slate-400 hover:text-red-400"
                        title="Delete"
                      >
                         <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/><line x1="10" y1="11" x2="10" y2="17"/><line x1="14" y1="11" x2="14" y2="17"/></svg>
                      </button>
                   </div>
                </div>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}

export default StudentProjects
