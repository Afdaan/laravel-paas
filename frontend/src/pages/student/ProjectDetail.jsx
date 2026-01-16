// ===========================================
// Project Detail Page (Rancher Style)
// ===========================================

import { useState, useEffect, useRef } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import toast from 'react-hot-toast'
import { projectsAPI } from '../../services/api'

// Status Indicator Component
function StatusIndicator({ status }) {
  const styles = {
    running: { bg: 'bg-emerald-500', text: 'text-emerald-500', label: 'Active' },
    building: { bg: 'bg-blue-500', text: 'text-blue-500', label: 'Building', pulse: true },
    failed: { bg: 'bg-red-500', text: 'text-red-500', label: 'Failed' },
    pending: { bg: 'bg-slate-500', text: 'text-slate-500', label: 'Pending' },
    stopped: { bg: 'bg-slate-500', text: 'text-slate-500', label: 'Stopped' },
  }
  
  const current = styles[status] || styles.pending
  
  return (
    <div className="flex items-center gap-2 px-3 py-1 rounded-full bg-slate-800 border border-slate-700">
      <div className={`w-2.5 h-2.5 rounded-full ${current.bg} ${current.pulse ? 'animate-pulse' : ''}`} />
      <span className={`text-sm font-medium ${current.text}`}>{current.label}</span>
    </div>
  )
}

function MetricCard({ title, value, subtext, color = 'primary' }) {
  const colors = {
    primary: 'text-primary-400',
    emerald: 'text-emerald-400',
    blue: 'text-blue-400',
  }

  return (
    <div className="card p-4 flex flex-col justify-between h-full bg-slate-800/50 border-slate-700">
      <h3 className="text-slate-400 text-sm font-medium uppercase tracking-wider">{title}</h3>
      <div className="mt-2">
        <div className={`text-2xl font-bold ${colors[color] || colors.primary}`}>{value}</div>
        {subtext && <div className="text-xs text-slate-500 mt-1">{subtext}</div>}
      </div>
    </div>
  )
}

function StudentProjectDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [project, setProject] = useState(null)
  const [logs, setLogs] = useState('')
  const [stats, setStats] = useState(null)
  const [isLoading, setIsLoading] = useState(true)
  const [activeTab, setActiveTab] = useState('workload')
  const logsEndRef = useRef(null)
  
  // Polling for status updates
  useEffect(() => {
    fetchProject()
    const interval = setInterval(() => {
      if (project?.status === 'building' || activeTab === 'stats') {
        fetchProject()
        if (activeTab === 'stats') fetchStats()
      }
    }, 5000)
    return () => clearInterval(interval)
  }, [id, activeTab, project?.status])
  
  // Fetch logs when tab is active
  useEffect(() => {
    if (activeTab === 'logs' && project?.container_id) {
      fetchLogs()
      const interval = setInterval(fetchLogs, 5000)
      return () => clearInterval(interval)
    }
  }, [activeTab, project])

  const fetchProject = async () => {
    try {
      const response = await projectsAPI.get(id)
      setProject(response.data)
    } catch (error) {
      toast.error('Could not load project details')
      if (error.response?.status === 404) navigate('/projects')
    } finally {
      setIsLoading(false)
    }
  }
  
  const fetchLogs = async () => {
    try {
      const response = await projectsAPI.logs(id, 200)
      setLogs(response.data.logs)
      // Auto-scroll to bottom
      if (logsEndRef.current) {
        logsEndRef.current.scrollIntoView({ behavior: 'smooth' })
      }
    } catch (error) {
       // Silent fail for logs
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
    toast.promise(
      projectsAPI.redeploy(id),
      {
        loading: 'Starting deployment...',
        success: () => {
          fetchProject()
          return 'Deployment started! Check logs for progress.'
        },
        error: 'Failed to start deployment',
      }
    )
  }
  
  const handleUpdatePHP = async (newVersion) => {
    try {
      await projectsAPI.update(id, { php_version: newVersion })
      setProject(prev => ({ ...prev, php_version: newVersion, is_manual_version: true }))
      toast((t) => (
        <div className="flex flex-col gap-2">
          <span>PHP Version set to <b>{newVersion}</b></span>
          <button 
            onClick={() => {
              handleRedeploy()
              toast.dismiss(t.id)
            }}
            className="btn btn-sm btn-primary py-1 px-2 text-xs"
          >
            Redeploy Now
          </button>
        </div>
      ), { duration: 5000, icon: '⚠️' })
    } catch (err) {
      toast.error('Failed to update PHP version')
    }
  }
  
  const handleDelete = async () => {
    if (!confirm('Are you sure? This will permanently delete your project and data.')) return
    
    toast.promise(
      projectsAPI.delete(id),
      {
        loading: 'Deleting project...',
        success: 'Project deleted successfully',
        error: 'Failed to delete project',
      }
    ).then(() => navigate('/projects'))
  }
  
  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-primary-500"></div>
      </div>
    )
  }
  
  if (!project) return null
  const projectUrl = project.url
  
  return (
    <div className="space-y-6 max-w-7xl mx-auto pb-20">
      
      {/* Header / Top Bar */}
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b border-slate-700 pb-6">
        <div>
          <div className="flex items-center gap-4 mb-2">
             <h1 className="text-3xl font-bold text-white tracking-tight">{project.name}</h1>
             <StatusIndicator status={project.status} />
          </div>
          <div className="flex items-center gap-2 text-slate-400 font-mono text-sm">
             <span>{project.subdomain}</span>
             {project.status === 'running' && (
                <a href={projectUrl} target="_blank" rel="noopener noreferrer" className="text-primary-400 hover:text-primary-300">
                  <span className="i-lucide-external-link"></span> ↗
                </a>
             )}
          </div>
        </div>
        
        <div className="flex gap-3">
           <button onClick={handleRedeploy} className="btn btn-secondary flex items-center gap-2">
             <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M3 21v-5h5"/></svg>
             Redeploy
           </button>
           <button onClick={handleDelete} className="btn btn-danger-outline flex items-center gap-2 px-3">
             <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/><line x1="10" x2="10" y1="11" y2="17"/><line x1="14" x2="14" y1="11" y2="17"/></svg>
           </button>
        </div>
      </div>

      {/* Stats Row */}
      <div className="grid grid-cols-2 md:grid-cols-4 gap-4">
         <MetricCard 
            title="CPU Usage" 
            value={stats ? `${stats.cpu_percent.toFixed(1)}%` : '0%'} 
            subtext="of 50% limit"
            color="blue"
         />
         <MetricCard 
            title="Memory" 
            value={stats ? `${stats.memory_mb.toFixed(0)} MB` : '0 MB'} 
            subtext={`of ${stats?.memory_max_mb?.toFixed(0) || 512} MB`}
            color="emerald"
         />
         <MetricCard 
            title="PHP Version" 
            value={project.php_version?.replace('.dynamic', '') || '...'} 
            subtext={project.is_manual_version ? 'Manual Override' : 'Auto Detected'}
            color="primary"
         />
         <MetricCard 
            title="Database" 
            value="MySQL" 
            subtext={project.database_name}
            color="primary"
         />
      </div>

      {/* Tabs Layout */}
      <div>
        <div className="flex gap-1 bg-slate-800/50 p-1 rounded-lg w-fit mb-6">
           {['workload', 'logs', 'settings'].map(tab => (
             <button
               key={tab}
               onClick={() => setActiveTab(tab)}
               className={`px-6 py-2 rounded-md text-sm font-medium transition-all ${
                 activeTab === tab 
                 ? 'bg-slate-700 text-white shadow-sm' 
                 : 'text-slate-400 hover:text-slate-200'
               }`}
             >
               {tab.charAt(0).toUpperCase() + tab.slice(1)}
             </button>
           ))}
        </div>

        {/* Tab Content */}
        <div className="animate-fade-in">
          
          {/* Workload Tab */}
          {activeTab === 'workload' && (
             <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
                <div className="lg:col-span-2 space-y-6">
                   <div className="card p-0 overflow-hidden">
                      <div className="p-4 border-b border-slate-700 bg-slate-800/50">
                         <h3 className="font-semibold text-white">Application Endpoints</h3>
                      </div>
                      <div className="p-4">
                         <div className="flex items-center justify-between p-3 bg-slate-800/30 rounded border border-slate-700">
                            <div className="flex items-center gap-3">
                               <div className="p-2 bg-emerald-500/10 rounded text-emerald-500">
                                  <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" x2="22" y1="12" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
                               </div>
                               <div>
                                  <div className="font-medium text-white">Main Entrypoint</div>
                                  <div className="text-xs text-slate-400">Port 80 • SSL Auto</div>
                               </div>
                            </div>
                            <a href={projectUrl} target="_blank" className="text-primary-400 hover:underline text-sm font-mono">{projectUrl}</a>
                         </div>
                      </div>
                   </div>

                   {project.error_log && (
                      <div className="bg-red-950/30 border border-red-500/30 rounded-lg p-4">
                         <h3 className="text-red-400 font-medium mb-1 flex items-center gap-2">
                           <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" x2="12" y1="8" y2="12"/><line x1="12" x2="12.01" y1="16" y2="16"/></svg>
                           Deployment Error
                         </h3>
                         <pre className="text-xs text-red-200/80 whitespace-pre-wrap mt-2 font-mono bg-black/20 p-2 rounded">{project.error_log}</pre>
                      </div>
                   )}
                </div>

                <div className="space-y-6">
                   <div className="card p-6">
                      <h3 className="font-semibold text-white mb-4">Repository</h3>
                      <div className="space-y-3">
                         <div>
                            <label className="text-xs text-slate-500 uppercase font-medium">Git URL</label>
                            <div className="text-sm text-slate-300 break-all">{project.github_url}</div>
                         </div>
                         <div>
                            <label className="text-xs text-slate-500 uppercase font-medium">Laravel Version</label>
                            <div className="text-sm text-white">{project.laravel_version || 'Unknown'}</div>
                         </div>
                      </div>
                   </div>
                </div>
             </div>
          )}

          {/* Logs Tab */}
          {activeTab === 'logs' && (
            <div className="card bg-black border-slate-800 overflow-hidden">
               <div className="flex items-center justify-between p-2 bg-slate-900 border-b border-slate-800">
                  <div className="flex gap-2 px-2">
                     <div className="w-3 h-3 rounded-full bg-red-500"/>
                     <div className="w-3 h-3 rounded-full bg-yellow-500"/>
                     <div className="w-3 h-3 rounded-full bg-green-500"/>
                  </div>
                  <span className="text-xs text-slate-500 font-mono">container: {project.container_id?.substring(0,12) || 'unknown'}</span>
               </div>
               <div className="p-4 h-[600px] overflow-auto font-mono text-sm">
                  {logs ? (
                    logs.split('\n').map((line, i) => (
                      <div key={i} className="text-slate-300 hover:bg-slate-800/50 px-1 rounded">
                        <span className="text-slate-600 mr-2 select-none">{(i+1).toString().padStart(3, ' ')}</span>
                        {line}
                      </div>
                    ))
                  ) : (
                    <div className="text-slate-600 italic">Waiting for logs...</div>
                  )}
                  <div ref={logsEndRef} />
               </div>
            </div>
          )}

          {/* Settings Tab */}
          {activeTab === 'settings' && (
             <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div className="card p-6">
                   <h3 className="font-semibold text-white mb-4">Runtime Configuration</h3>
                   <div>
                      <label className="block text-sm text-slate-400 mb-1">PHP Version</label>
                      <select 
                        value={project.php_version?.replace('dynamic', '').replace('.fpm', '') || '8.2'}
                        onChange={(e) => handleUpdatePHP(e.target.value)}
                        className="w-full bg-slate-900 border border-slate-700 rounded-lg px-3 py-2 text-white focus:outline-none focus:border-primary-500"
                      >
                         <option value="8.0">PHP 8.0</option>
                         <option value="8.1">PHP 8.1</option>
                         <option value="8.2">PHP 8.2</option>
                         <option value="8.3">PHP 8.3</option>
                         <option value="8.4">PHP 8.4 (Latest)</option>
                      </select>
                      <p className="text-xs text-slate-500 mt-2">
                         Changing the PHP version requires a redeployment.
                      </p>
                   </div>
                </div>

                <div className="card p-6">
                   <h3 className="font-semibold text-white mb-4">Database Credentials</h3>
                   <div className="space-y-3">
                      <div>
                         <label className="text-xs text-slate-500 uppercase">Host</label>
                         <div className="font-mono text-white bg-slate-900 px-2 py-1 rounded">mysql</div>
                      </div>
                      <div>
                         <label className="text-xs text-slate-500 uppercase">Database Name</label>
                         <div className="font-mono text-white bg-slate-900 px-2 py-1 rounded">{project.database_name}</div>
                      </div>
                      <div>
                         <label className="text-xs text-slate-500 uppercase">User</label>
                         <div className="font-mono text-white bg-slate-900 px-2 py-1 rounded">{project.database_name}</div>
                      </div>
                      <div>
                         <label className="text-xs text-slate-500 uppercase">Password</label>
                         <div className="font-mono text-white bg-slate-900 px-2 py-1 rounded">*** (Same as DB Name)</div>
                      </div>
                   </div>
                   <div className="mt-4">
                      <Link to={`/projects/${id}/database`} className="btn btn-secondary w-full justify-center">
                         Open Database Manager
                      </Link>
                   </div>
                </div>
             </div>
          )}

        </div>
      </div>
    </div>
  )
}

export default StudentProjectDetail
