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

import DatabaseManager from './DatabaseManager'
import ConfirmationModal from '../../components/ConfirmationModal'

function StudentProjectDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [project, setProject] = useState(null)
  const [logs, setLogs] = useState('')
  const [stats, setStats] = useState(null)
  const [isLoading, setIsLoading] = useState(true)
  const [activeTab, setActiveTab] = useState('workload')
  const logsEndRef = useRef(null)
  
  // New features state
  const [envContent, setEnvContent] = useState('')
  const [consoleOutput, setConsoleOutput] = useState('')
  const [consoleCommand, setConsoleCommand] = useState('')
  const [isExecuting, setIsExecuting] = useState(false)
  const [isSavingEnv, setIsSavingEnv] = useState(false)
  
  // Modal State
  const [confirmModal, setConfirmModal] = useState({
    isOpen: false,
    title: '',
    message: '',
    type: 'danger',
    onConfirm: () => {},
    confirmText: 'Confirm'
  })

  const openConfirm = (opts) => {
    setConfirmModal({ ...opts, isOpen: true })
  }

  // Polling for status updates and stats
  useEffect(() => {
    fetchProject()
    const interval = setInterval(() => {
      fetchProject()
      // Always fetch stats if project is running (regardless of active tab)
      if (project?.status === 'running') {
        fetchStats()
      }
    }, 5000)
    return () => clearInterval(interval)
  }, [id, project?.status])
  
  // Fetch logs when tab is active
  useEffect(() => {
    if (activeTab === 'logs' && project?.container_id) {
      fetchLogs()
      const interval = setInterval(fetchLogs, 5000)
      return () => clearInterval(interval)
    }
  }, [activeTab, project])

  // Fetch Env when tab is active
  useEffect(() => {
    if (activeTab === 'environment') {
      fetchEnv()
    }
  }, [activeTab, id])

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

  const fetchEnv = async () => {
    try {
      const response = await projectsAPI.getEnv(id)
      setEnvContent(response.data.content)
    } catch (error) {
      toast.error('Failed to load .env file')
    }
  }

  const handleSaveEnv = async () => {
    setIsSavingEnv(true)
    try {
      await projectsAPI.updateEnv(id, envContent)
      toast.success('Environment variables updated')
    } catch (error) {
      toast.error('Failed to save .env file')
    } finally {
      setIsSavingEnv(false)
    }
  }

  const handleConsoleSubmit = async (e) => {
    e.preventDefault()
    if (!consoleCommand.trim()) return

    setIsExecuting(true)
    setConsoleOutput(prev => prev + `\n$ php artisan ${consoleCommand}\n`)
    
    try {
      const response = await projectsAPI.runArtisan(id, consoleCommand)
      setConsoleOutput(prev => prev + response.data.output + '\n')
      setConsoleCommand('')
    } catch (error) {
      const errOut = error.response?.data?.output || error.message
      setConsoleOutput(prev => prev + `Error: ${errOut}\n`)
    } finally {
      setIsExecuting(false)
    }
  }
  
  const handleRedeploy = async () => {
    openConfirm({
      title: 'Redeploy Project?',
      message: 'This will rebuild your container. The application will be briefly unavailable during deployment.',
      type: 'warning',
      confirmText: 'Redeploy Now',
      onConfirm: () => {
        toast.promise(
          projectsAPI.redeploy(id),
          {
            loading: 'Initiating deployment...',
            success: () => {
              fetchProject()
              return 'Deployment started in background'
            },
            error: 'Failed to start deployment',
          }
        )
      }
    })
  }
  
  const handleUpdatePHP = async (newVersion) => {
    openConfirm({
      title: `Update PHP to ${newVersion}?`,
      message: `Changing the PHP version requires a complete rebuild of your container. Your site will be redeployed immediately.`,
      type: 'warning',
      confirmText: 'Update & Redeploy',
      onConfirm: async () => {
        try {
          await projectsAPI.update(id, { php_version: newVersion })
          setProject(prev => ({ ...prev, php_version: newVersion, is_manual_version: true }))
          toast((t) => (
            <div className="flex flex-col gap-2">
              <span className="font-semibold">PHP Version updated</span>
              <span className="text-xs">System will now rebuild your project with PHP {newVersion}</span>
            </div>
          ))
          projectsAPI.redeploy(id).then(() => fetchProject())
        } catch (err) {
          toast.error('Failed to update PHP version')
        }
      }
    })
  }

  const handleUpdateQueue = async (enabled) => {
    openConfirm({
      title: `${enabled ? 'Enable' : 'Disable'} Queue Worker?`,
      message: `Changing queue worker configuration requires a complete rebuild of your container. Your site will be redeployed immediately.`,
      type: 'warning',
      confirmText: enabled ? 'Enable & Redeploy' : 'Disable & Redeploy',
      onConfirm: async () => {
        try {
          await projectsAPI.update(id, { queue_enabled: enabled })
          setProject(prev => ({ ...prev, queue_enabled: enabled }))
          toast.success(`Queue Worker ${enabled ? 'Enabled' : 'Disabled'}`)
          projectsAPI.redeploy(id).then(() => fetchProject())
        } catch (err) {
          toast.error('Failed to update settings')
        }
      }
    })
  }
  
  const handleDelete = async () => {
    openConfirm({
      title: 'Delete Project Permanently?',
      message: 'This action cannot be undone. All project files, database, and configurations will be permanently destroyed.',
      type: 'danger',
      confirmText: 'Yes, Delete it',
      onConfirm: () => {
        toast.promise(
          projectsAPI.delete(id),
          {
            loading: 'Deleting resources...',
            success: 'Project deleted successfully',
            error: 'Failed to delete project',
          }
        ).then(() => navigate('/projects'))
      }
    })
  }

  const copyToClipboard = (text) => {
    navigator.clipboard.writeText(text)
    toast.success('Copied to clipboard')
  }
  
  if (isLoading) {
    return (
      <div className="flex flex-col items-center justify-center h-screen gap-4">
        <div className="relative">
          <div className="animate-spin rounded-full h-16 w-16 border-t-2 border-b-2 border-primary-500"></div>
          <div className="absolute inset-0 flex items-center justify-center text-xs font-bold text-primary-500">PaaS</div>
        </div>
        <div className="text-slate-400 animate-pulse">Loading project configuration...</div>
      </div>
    )
  }
  
  if (!project) return null
  const projectUrl = project.url
  
  return (
    <div className="space-y-6 max-w-7xl mx-auto pb-20">
      <ConfirmationModal 
        isOpen={confirmModal.isOpen}
        onClose={() => setConfirmModal(prev => ({ ...prev, isOpen: false }))}
        {...confirmModal}
      />
      
      {/* Building Banner */}
      {project.status === 'building' && (
        <div className="bg-blue-600/10 border border-blue-500/30 rounded-xl p-6 relative overflow-hidden">
           <div className="absolute inset-0 bg-blue-500/5 animate-pulse-slow"></div>
           <div className="relative flex items-center gap-4">
              <div className="p-3 bg-blue-500/20 rounded-full text-blue-400 animate-spin-slow">
                 <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"/><polyline points="3.27 6.96 12 12.01 20.73 6.96"/><line x1="12" y1="22.08" x2="12" y2="12"/></svg>
              </div>
              <div>
                 <h3 className="text-lg font-bold text-blue-400">Building Application...</h3>
                 <p className="text-blue-200/60 text-sm">
                   We are compiling your container, installing dependencies, and configuring the environment. 
                   <br/>Container logs will be available once the build process completes.
                 </p>
              </div>
           </div>
        </div>
      )}
      
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
      <div className="grid grid-cols-2 lg:grid-cols-5 gap-4">
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
         <MetricCard 
            title="Queue Worker" 
            value={project.queue_enabled ? 'Active' : 'Disabled'} 
            subtext={project.queue_enabled ? 'Database Driver' : 'Sync Driver'}
            color={project.queue_enabled ? 'emerald' : 'primary'}
         />
      </div>

      {/* Tabs Layout */}
      <div>
        <div className="flex gap-1 bg-slate-800/50 p-1 rounded-lg w-fit mb-6 overflow-x-auto">
           {['workload', 'console', 'environment', 'database', 'logs', 'settings'].map(tab => (
             <button
               key={tab}
               onClick={() => setActiveTab(tab)}
               className={`px-6 py-2 rounded-md text-sm font-medium transition-all whitespace-nowrap ${
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
                {/* ... existing Workload content ... */} 
                {/* Re-inserting existing workload content for context, but truncated for brevity in replacement if needed. 
                    However, since I'm replacing the whole component logic block, I should ensure I don't lose the existing Workload UI. 
                    The user implementation will paste the full block.
                */}
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
                            <label className="text-xs text-slate-500 uppercase font-medium">Branch</label>
                            <div className="flex items-center gap-2">
                               <span className="i-lucide-git-branch text-slate-500"></span>
                               <span className="text-sm text-white font-mono bg-slate-800 px-2 py-0.5 rounded">{project.branch || 'main'}</span>
                            </div>
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

          {/* Console Tab */}
          {activeTab === 'console' && (
            <div className="card p-0 overflow-hidden flex flex-col h-[600px]">
              <div className="p-3 bg-slate-900 border-b border-slate-800 flex items-center justify-between">
                <div className="flex gap-2">
                   <div className="w-3 h-3 rounded-full bg-red-500"/>
                   <div className="w-3 h-3 rounded-full bg-yellow-500"/>
                   <div className="w-3 h-3 rounded-full bg-green-500"/>
                </div>
                <span className="text-xs text-slate-500 font-mono">php artisan runner</span>
              </div>
              <div className="flex-1 bg-black p-4 overflow-auto font-mono text-sm text-slate-300 whitespace-pre-wrap">
                <div className="text-slate-500 mb-2"># enter artisan command without 'php artisan' prefix (e.g. 'migrate')</div>
                {consoleOutput}
                {isExecuting && <div className="text-primary-400 animate-pulse">Running...</div>}
              </div>
              <form onSubmit={handleConsoleSubmit} className="p-3 bg-slate-800 border-t border-slate-700 flex gap-2">
                <div className="flex items-center px-3 bg-slate-900 rounded text-slate-400 font-mono text-sm select-none">
                  php artisan
                </div>
                <input 
                  type="text" 
                  value={consoleCommand}
                  onChange={(e) => setConsoleCommand(e.target.value)}
                  placeholder="command..."
                  className="flex-1 bg-transparent text-white font-mono text-sm focus:outline-none"
                  autoFocus
                />
                <button 
                  type="submit" 
                  disabled={isExecuting || !consoleCommand.trim()}
                  className="px-4 py-1.5 bg-primary-600 text-white text-xs font-medium rounded hover:bg-primary-500 disabled:opacity-50"
                >
                  Run
                </button>
              </form>
            </div>
          )}

          {/* Environment Tab */}
          {activeTab === 'environment' && (
            <div className="card p-0 overflow-hidden h-[600px] flex flex-col">
               <div className="p-4 border-b border-slate-700 bg-slate-800/50 flex justify-between items-center">
                  <h3 className="font-semibold text-white">Environment Variables (.env)</h3>
                  <button 
                    onClick={handleSaveEnv}
                    disabled={isSavingEnv}
                    className="btn btn-primary text-sm py-1.5"
                  >
                    {isSavingEnv ? 'Saving...' : 'Save Changes'}
                  </button>
               </div>
               <div className="flex-1 relative">
                 <textarea
                   value={envContent}
                   onChange={(e) => setEnvContent(e.target.value)}
                   className="absolute inset-0 w-full h-full bg-slate-900 text-slate-300 font-mono text-sm p-4 focus:outline-none resize-none"
                   spellCheck="false"
                 />
               </div>
               <div className="p-2 bg-yellow-500/10 text-yellow-500 text-xs px-4 border-t border-slate-800">
                  ⚠️ Changing environment variables may require a redeployment to take full effect.
               </div>
            </div>
          )}

          {/* Database Tab */}
          {activeTab === 'database' && (
             <div className="min-h-[600px]">
                <DatabaseManager embedded={true} projectId={id} />
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
                   <h3 className="font-semibold text-white mb-4">Background Worker</h3>
                   <div>
                      <div className="flex items-center justify-between mb-2">
                          <label className="block text-sm text-slate-400">Queue Worker Status</label>
                          <button 
                             onClick={() => handleUpdateQueue(!project.queue_enabled)}
                             className={`w-12 h-6 rounded-full p-1 cursor-pointer transition-colors border-none focus:ring-0 ${project.queue_enabled ? 'bg-emerald-500' : 'bg-slate-700'}`}
                          >
                             <div className={`w-4 h-4 rounded-full bg-white transition-transform shadow-sm ${project.queue_enabled ? 'translate-x-6' : 'translate-x-0'}`} />
                          </button>
                      </div>
                      <div className="text-xs text-slate-500 mt-3 space-y-1">
                         <p>Activating this runs <code>php artisan queue:work</code> (database driver).</p>
                         <p className="text-amber-500/80">⚠️ Changing this triggers a redeploy.</p>
                      </div>
                   </div>
                </div>

                <div className="card p-6">
                   <h3 className="font-semibold text-white mb-4">Database Credentials</h3>
                   <div className="space-y-4">
                      <div className="grid grid-cols-2 gap-4">
                         <div>
                            <label className="text-xs text-slate-500 uppercase font-medium">Host</label>
                            <div className="flex items-center gap-2 mt-1">
                               <code className="flex-1 font-mono text-sm text-white bg-slate-900 px-3 py-2 rounded">paas-mysql</code>
                               <button onClick={() => copyToClipboard('paas-mysql')} className="p-2 hover:bg-slate-800 rounded text-slate-400">
                                  <span className="i-lucide-copy">📋</span>
                               </button>
                            </div>
                         </div>
                         <div>
                            <label className="text-xs text-slate-500 uppercase font-medium">Port</label>
                            <div className="flex items-center gap-2 mt-1">
                               <code className="flex-1 font-mono text-sm text-white bg-slate-900 px-3 py-2 rounded">3306</code>
                            </div>
                         </div>
                      </div>
                      
                      <div>
                         <label className="text-xs text-slate-500 uppercase font-medium">Database Name</label>
                         <div className="flex items-center gap-2 mt-1">
                            <code className="flex-1 font-mono text-sm text-white bg-slate-900 px-3 py-2 rounded">{project.database_name}</code>
                            <button onClick={() => copyToClipboard(project.database_name)} className="p-2 hover:bg-slate-800 rounded text-slate-400">
                               <span className="i-lucide-copy">📋</span>
                            </button>
                         </div>
                      </div>

                      <div>
                         <label className="text-xs text-slate-500 uppercase font-medium">User</label>
                         <div className="flex items-center gap-2 mt-1">
                            <code className="flex-1 font-mono text-sm text-white bg-slate-900 px-3 py-2 rounded">{project.database_name}</code>
                            <button onClick={() => copyToClipboard(project.database_name)} className="p-2 hover:bg-slate-800 rounded text-slate-400">
                               <span className="i-lucide-copy">📋</span>
                            </button>
                         </div>
                      </div>

                      <div>
                         <label className="text-xs text-slate-500 uppercase font-medium">Password</label>
                         <div className="flex items-center gap-2 mt-1">
                            <code className="flex-1 font-mono text-sm text-white bg-slate-900 px-3 py-2 rounded tracking-widest">••••••••••••</code>
                            <button onClick={() => copyToClipboard(project.database_name)} className="p-2 hover:bg-slate-800 rounded text-slate-400" title="Copy Password">
                               <span className="i-lucide-copy">📋</span>
                            </button>
                         </div>
                         <p className="text-xs text-slate-500 mt-1">Password is same as database name</p>
                      </div>
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
