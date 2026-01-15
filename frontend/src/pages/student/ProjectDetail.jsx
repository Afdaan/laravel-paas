// ===========================================
// Project Detail Page
// ===========================================

import { useState, useEffect } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
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

function StudentProjectDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [project, setProject] = useState(null)
  const [logs, setLogs] = useState('')
  const [stats, setStats] = useState(null)
  const [isLoading, setIsLoading] = useState(true)
  const [activeTab, setActiveTab] = useState('overview')
  
  useEffect(() => {
    fetchProject()
  }, [id])
  
  useEffect(() => {
    if (activeTab === 'logs' && project?.container_id) {
      fetchLogs()
    }
    if (activeTab === 'stats' && project?.container_id) {
      fetchStats()
    }
  }, [activeTab, project])
  
  const fetchProject = async () => {
    try {
      const response = await projectsAPI.get(id)
      setProject(response.data)
    } catch (error) {
      toast.error('Project not found')
      navigate('/projects')
    } finally {
      setIsLoading(false)
    }
  }
  
  const fetchLogs = async () => {
    try {
      const response = await projectsAPI.logs(id, 200)
      setLogs(response.data.logs)
    } catch (error) {
      setLogs('Failed to fetch logs')
    }
  }
  
  const fetchStats = async () => {
    try {
      const response = await projectsAPI.stats(id)
      setStats(response.data)
    } catch (error) {
      setStats(null)
    }
  }
  
  const handleRedeploy = async () => {
    if (!confirm('Are you sure you want to redeploy?')) return
    
    try {
      await projectsAPI.redeploy(id)
      toast.success('Redeployment started')
      fetchProject()
    } catch (error) {
      toast.error(error.response?.data?.error || 'Redeploy failed')
    }
  }
  
  const handleDelete = async () => {
    if (!confirm('Are you sure? This will delete all project data.')) return
    
    try {
      await projectsAPI.delete(id)
      toast.success('Project deleted')
      navigate('/projects')
    } catch (error) {
      toast.error(error.response?.data?.error || 'Delete failed')
    }
  }
  
  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-t-2 border-b-2 border-primary-500"></div>
      </div>
    )
  }
  
  if (!project) return null
  
  const projectUrl = `https://${project.subdomain}.${window.location.hostname}`
  
  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-start justify-between">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold text-white">{project.name}</h1>
            <StatusBadge status={project.status} />
          </div>
          <p className="text-slate-400 mt-1">{project.subdomain}</p>
        </div>
        
        <div className="flex gap-2">
          {project.status === 'running' && (
            <a 
              href={projectUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="btn btn-primary"
            >
              Open Site ↗
            </a>
          )}
          <Link to={`/projects/${id}/database`} className="btn btn-secondary">
            🗄️ Database
          </Link>
          <button onClick={handleRedeploy} className="btn btn-secondary">
            Redeploy
          </button>
          <button onClick={handleDelete} className="btn btn-danger">
            Delete
          </button>
        </div>
      </div>
      
      {/* Tabs */}
      <div className="border-b border-slate-700">
        <nav className="flex gap-6">
          {['overview', 'logs', 'stats'].map((tab) => (
            <button
              key={tab}
              onClick={() => setActiveTab(tab)}
              className={`py-3 px-1 border-b-2 font-medium text-sm transition-colors ${
                activeTab === tab
                  ? 'border-primary-500 text-primary-400'
                  : 'border-transparent text-slate-400 hover:text-white'
              }`}
            >
              {tab.charAt(0).toUpperCase() + tab.slice(1)}
            </button>
          ))}
        </nav>
      </div>
      
      {/* Tab Content */}
      {activeTab === 'overview' && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          {/* Project Info */}
          <div className="card p-6">
            <h2 className="text-lg font-semibold text-white mb-4">Project Information</h2>
            <dl className="space-y-3">
              <div className="flex justify-between">
                <dt className="text-slate-400">GitHub URL</dt>
                <dd className="text-white">
                  <a href={project.github_url} target="_blank" className="text-primary-400 hover:underline">
                    {project.github_url.split('/').slice(-2).join('/')}
                  </a>
                </dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-slate-400">Laravel Version</dt>
                <dd className="text-white">{project.laravel_version || 'Detecting...'}</dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-slate-400">PHP Version</dt>
                <dd className="text-white">{project.php_version || 'Detecting...'}</dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-slate-400">Database</dt>
                <dd className="text-white font-mono">{project.database_name}</dd>
              </div>
              <div className="flex justify-between">
                <dt className="text-slate-400">Created</dt>
                <dd className="text-white">{new Date(project.created_at).toLocaleString()}</dd>
              </div>
            </dl>
          </div>
          
          {/* URL & Access */}
          <div className="card p-6">
            <h2 className="text-lg font-semibold text-white mb-4">Access</h2>
            <div className="space-y-4">
              <div>
                <label className="text-slate-400 text-sm">Project URL</label>
                <div className="flex items-center gap-2 mt-1">
                  <input 
                    type="text" 
                    value={projectUrl}
                    readOnly
                    className="flex-1 bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-white"
                  />
                  <button 
                    onClick={() => {
                      navigator.clipboard.writeText(projectUrl)
                      toast.success('Copied!')
                    }}
                    className="btn btn-secondary"
                  >
                    Copy
                  </button>
                </div>
              </div>
              
              {project.error_log && (
                <div className="bg-red-600/10 border border-red-600/30 rounded-lg p-4">
                  <h3 className="text-red-400 font-medium mb-2">Error</h3>
                  <pre className="text-sm text-red-300 whitespace-pre-wrap">
                    {project.error_log}
                  </pre>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
      
      {activeTab === 'logs' && (
        <div className="card">
          <div className="p-4 border-b border-slate-700 flex justify-between items-center">
            <h2 className="font-semibold text-white">Container Logs</h2>
            <button onClick={fetchLogs} className="btn btn-secondary text-sm">
              Refresh
            </button>
          </div>
          <div className="p-4 bg-slate-900 rounded-b-xl max-h-96 overflow-auto">
            <pre className="text-sm text-slate-300 font-mono whitespace-pre-wrap">
              {logs || 'No logs available'}
            </pre>
          </div>
        </div>
      )}
      
      {activeTab === 'stats' && (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <div className="card p-6">
            <h2 className="text-lg font-semibold text-white mb-4">CPU Usage</h2>
            <div className="text-4xl font-bold text-primary-400">
              {stats?.cpu_percent?.toFixed(1) || '0'}%
            </div>
            <div className="mt-2 h-2 bg-slate-700 rounded-full overflow-hidden">
              <div 
                className="h-full bg-primary-500 transition-all"
                style={{ width: `${stats?.cpu_percent || 0}%` }}
              />
            </div>
          </div>
          
          <div className="card p-6">
            <h2 className="text-lg font-semibold text-white mb-4">Memory Usage</h2>
            <div className="text-4xl font-bold text-emerald-400">
              {stats?.memory_mb?.toFixed(0) || '0'} MB
            </div>
            <p className="text-slate-400 mt-1">
              of {stats?.memory_max_mb?.toFixed(0) || '512'} MB limit
            </p>
            <div className="mt-2 h-2 bg-slate-700 rounded-full overflow-hidden">
              <div 
                className="h-full bg-emerald-500 transition-all"
                style={{ width: `${((stats?.memory_mb || 0) / (stats?.memory_max_mb || 512)) * 100}%` }}
              />
            </div>
          </div>
          
          <div className="md:col-span-2">
            <button onClick={fetchStats} className="btn btn-secondary">
              Refresh Stats
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

export default StudentProjectDetail
