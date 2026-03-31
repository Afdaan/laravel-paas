// ===========================================
// Project Detail Page (Rancher Style)
// ===========================================

import { useState, useEffect, useRef } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import toast from 'react-hot-toast'
import { 
  RefreshCw,
  ExternalLink,
  Trash2,
  Cpu,
  Activity,
  Zap,
  Layout,
  Terminal as TerminalIcon,
  Code,
  Globe,
  Database as DatabaseIcon,
  Settings as SettingsIcon,
  Maximize2,
  ChevronRight,
  ShieldAlert,
  Save,
  Clock,
  Box,
  AlertTriangle,
  GitBranch,
  Copy
} from 'lucide-react'
import { projectsAPI } from '../../services/api'
import DatabaseManager from './DatabaseManager'
import ConfirmationModal from '../../components/ConfirmationModal'

// Status Indicator Component
function StatusIndicator({ status }) {
  const styles = {
    running: { bg: 'bg-emerald-500', text: 'text-emerald-500', label: 'Active', shadow: 'shadow-[0_0_15px_rgba(16,185,129,0.4)]' },
    building: { bg: 'bg-blue-500', text: 'text-blue-500', label: 'Building', pulse: true, shadow: 'shadow-[0_0_15px_rgba(59,130,246,0.4)]' },
    failed: { bg: 'bg-rose-500', text: 'text-rose-500', label: 'Failed' },
    pending: { bg: 'bg-amber-500', text: 'text-amber-500', label: 'Queued', bounce: true, shadow: 'shadow-[0_0_15px_rgba(245,158,11,0.4)]' },
    stopped: { bg: 'bg-slate-500', text: 'text-slate-600 dark:text-slate-400', label: 'Offline' },
  }
  
  const current = styles[status] || styles.pending
  
  return (
    <div className="flex items-center gap-3 px-4 py-1.5 rounded-full bg-slate-100 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 backdrop-blur-md">
      <div className={`w-2 h-2 rounded-full ${current.bg} ${current.pulse ? 'animate-pulse' : ''} ${current.bounce ? 'animate-bounce' : ''} ${current.shadow || ''}`} />
      <span className={`text-[10px] font-black uppercase tracking-[0.2em] ${current.text}`}>{current.label}</span>
    </div>
  )
}

function MetricCard({ title, value, subtext, color = 'primary', icon: Icon }) {
  const colors = {
    primary: 'text-indigo-400 border-indigo-400/20',
    emerald: 'text-emerald-400 border-emerald-400/20',
    blue: 'text-blue-400 border-blue-400/20',
    rose: 'text-rose-400 border-rose-400/20',
  }

  return (
    <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm p-6 group hover:border-slate-300 dark:border-white/20 transition-all duration-300">
      <div className="flex justify-between items-start mb-4">
        <p className="text-slate-600 dark:text-slate-400 text-[10px] font-black uppercase tracking-[0.2em]">{title}</p>
        {Icon && <Icon className={`w-4 h-4 ${colors[color].split(' ')[0]}`} />}
      </div>
      <div>
        <div className={`text-2xl font-black text-slate-900 dark:text-white tracking-tighter`}>{value}</div>
        {subtext && <div className="text-[10px] font-bold text-slate-600 dark:text-slate-400 mt-1 uppercase tracking-wider">{subtext}</div>}
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
  const [activeTab, setActiveTab] = useState('project')
  const logsEndRef = useRef(null)
  
  // New features state
  const [envContent, setEnvContent] = useState('')
  const [consoleOutput, setConsoleOutput] = useState('')
  const [consoleCommand, setConsoleCommand] = useState('')
  const [isExecuting, setIsExecuting] = useState(false)
  const [isSavingEnv, setIsSavingEnv] = useState(false)
  const [isPhpDropdownOpen, setIsPhpDropdownOpen] = useState(false)
  
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
            loading: 'Starting deployment...',
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
        <div className="text-slate-600 dark:text-slate-400 animate-pulse">Loading project configuration...</div>
      </div>
    )
  }
  
  if (!project) return null
  const projectUrl = project.url
  
  return (
    <div className="space-y-6 max-w-7xl mx-auto pb-40">
      <ConfirmationModal 
        isOpen={confirmModal.isOpen}
        onClose={() => setConfirmModal(prev => ({ ...prev, isOpen: false }))}
        {...confirmModal}
      />
      
      {/* Building Banner */}
      {project.status === 'building' && (
        <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm border-blue-500/20 bg-blue-500/10 p-10 relative overflow-hidden group">
           <div className="absolute inset-0 bg-gradient-to-r from-blue-500/0 via-blue-500/5 to-blue-500/0 animate-shimmer"></div>
           <div className="relative flex flex-col md:flex-row items-center gap-8">
              <div className="w-20 h-20 bg-blue-500/20 rounded-3xl flex items-center justify-center text-blue-400 border border-blue-500/30 shadow-[0_0_30px_rgba(59,130,246,0.3)] animate-pulse">
                 <Box className="w-10 h-10" />
              </div>
              <div className="text-center md:text-left flex-1">
                 <h3 className="text-2xl font-black text-slate-900 dark:text-white tracking-tight mb-2">Setting Up Your Project</h3>
                 <p className="text-blue-100/60 text-base leading-relaxed max-w-2xl">
                   We are currently building your project environment, setting up the PHP version, and creating the internet connection. Live metrics will initialize once the build is complete.
                 </p>
              </div>
              <div className="flex items-center gap-2 px-4 py-2 rounded-xl bg-blue-500/20 border border-blue-500/30 text-blue-400 text-xs font-black uppercase tracking-widest">
                 <RefreshCw className="w-4 h-4 animate-spin" />
                 Building
              </div>
           </div>
        </div>
      )}
      
      {/* Header / Top Bar */}
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-8 border-b border-slate-200 dark:border-white/5 pb-10">
        <div className="space-y-4">
          <div className="flex flex-wrap items-center gap-4">
             <h1 className="text-5xl font-black text-slate-900 dark:text-white tracking-tighter">{project.name}</h1>
             <StatusIndicator status={project.status} />
          </div>
          <div className="flex items-center gap-4">
            <div className="px-3 py-1.5 rounded-lg bg-slate-100 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 flex items-center gap-3 group">
              <Globe className="w-4 h-4 text-slate-600 dark:text-slate-400" />
              <span className="text-slate-600 dark:text-slate-400 font-mono text-sm tracking-tight">{project.subdomain}</span>
              {project.status === 'running' && (
                <a 
                  href={projectUrl} 
                  target="_blank" 
                  rel="noopener noreferrer" 
                  className="w-8 h-8 rounded-md bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 flex items-center justify-center text-indigo-400 hover:bg-indigo-500 hover:text-white transition-all shadow-lg"
                  title="Open live site"
                >
                  <ExternalLink className="w-4 h-4" />
                </a>
              )}
            </div>
            <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-slate-100 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 text-slate-600 dark:text-slate-400 text-xs font-bold uppercase tracking-widest">
               <Code className="w-3.5 h-3.5" />
               v1.4.2
            </div>
          </div>
        </div>
        
        <div className="flex gap-4">
           <button 
             onClick={handleRedeploy} 
             className="btn btn-secondary py-3 px-8 text-sm font-black uppercase tracking-widest group"
           >
             <RefreshCw className="w-4 h-4 group-hover:rotate-180 transition-transform duration-500" />
             Redeploy Project
           </button>
           <button 
             onClick={handleDelete} 
             className="w-12 h-12 rounded-2xl bg-rose-500/10 border border-rose-500/20 flex items-center justify-center text-rose-500 hover:bg-rose-500 hover:text-white transition-all group"
             title="Destroy Resource"
           >
             <Trash2 className="w-5 h-5 group-hover:scale-110 transition-transform" />
           </button>
        </div>
      </div>

      {/* Stats Row */}
      <div className="grid grid-cols-2 lg:grid-cols-5 gap-6">
         <MetricCard 
            title="CPU Usage" 
            value={stats ? `${stats.cpu_percent.toFixed(1)}%` : '0%'} 
            subtext="Real-time Load"
            color="blue"
            icon={Cpu}
         />
         <MetricCard 
            title="Memory" 
            value={stats ? `${stats.memory_mb.toFixed(0)} MB` : '0 MB'} 
            subtext={`of ${stats?.memory_max_mb?.toFixed(0) || 512} MB`}
            color="emerald"
            icon={Activity}
         />
         <MetricCard 
            title="PHP Environment" 
            value={project.php_version?.replace('.dynamic', '') || '...'} 
            subtext={project.is_manual_version ? 'Custom Runtime' : 'Standard Runtime'}
            color="primary"
            icon={Zap}
         />
         <MetricCard 
            title="Core Database" 
            value="MySQL" 
            subtext={project.database_name}
            color="primary"
            icon={DatabaseIcon}
         />
         <MetricCard 
            title="Queue Worker" 
            value={project.queue_enabled ? 'Running' : 'Offline'} 
            subtext={project.queue_enabled ? 'Background Processing' : 'Direct Dispatch'}
            color={project.queue_enabled ? 'emerald' : 'primary'}
            icon={RefreshCw}
         />
      </div>

      {/* Tabs Layout */}
      <div>
        <div className="flex gap-2 bg-slate-100 dark:bg-white/5 p-1.5 rounded-2xl w-fit mb-10 overflow-x-auto backdrop-blur-md border border-slate-300 dark:border-white/10">
           {['project', 'console', 'environment', 'database', 'logs', 'settings'].map(tab => (
             <button
               key={tab}
               onClick={() => setActiveTab(tab)}
               className={`px-8 py-2.5 rounded-xl text-xs font-black uppercase tracking-widest transition-all whitespace-nowrap ${
                 activeTab === tab 
                 ? 'bg-white text-black shadow-[0_0_20px_rgba(255,255,255,0.2)]' 
                 : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white hover:bg-slate-100 dark:bg-white/5'
               }`}
             >
               {tab}
             </button>
           ))}
        </div>

        {/* Tab Content */}
        <div className="animate-fade-in relative z-10">
          
          {/* Project Tab */}
          {activeTab === 'project' && (
             <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
                <div className="lg:col-span-2 space-y-8">
                   <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm p-0 overflow-hidden bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border-slate-300 dark:border-white/10">
                      <div className="p-6 border-b border-slate-200 dark:border-white/5 bg-slate-100 dark:bg-slate-100 dark:bg-white/5">
                         <h3 className="text-sm font-black text-slate-900 dark:text-white uppercase tracking-widest flex items-center gap-3">
                           <Layout className="w-4 h-4 text-indigo-400" />
                           Connection Info
                         </h3>
                      </div>
                      <div className="p-8">
                         <div className="flex items-center justify-between p-6 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 rounded-2xl border border-slate-200 dark:border-white/5 group hover:border-indigo-500/30 transition-all">
                            <div className="flex items-center gap-5">
                               <div className="p-3 bg-emerald-500/10 rounded-2xl text-emerald-400 border border-emerald-500/20 shadow-[0_0_15px_rgba(16,185,129,0.2)]">
                                  <Globe className="w-6 h-6" />
                               </div>
                               <div>
                                  <div className="font-black text-slate-900 dark:text-white uppercase tracking-tight">Production URL</div>
                                  <div className="text-[10px] font-bold text-slate-600 dark:text-slate-400 uppercase tracking-widest mt-1">Web Access • SSL Enabled</div>
                               </div>
                            </div>
                            <a 
                              href={projectUrl} 
                              target="_blank" 
                              className="text-indigo-400 hover:text-slate-900 dark:text-white font-mono text-sm underline-offset-8 hover:underline transition-all"
                            >
                              {projectUrl}
                            </a>
                         </div>
                      </div>
                   </div>

                   {project.error_log && (
                      <div className="bg-rose-500/10 border border-rose-500/20 rounded-2xl p-8 relative overflow-hidden">
                         <div className="absolute top-0 right-0 p-4 opacity-10">
                            <ShieldAlert className="w-16 h-16 text-rose-500" />
                         </div>
                         <h3 className="text-rose-400 font-black uppercase tracking-[0.2em] text-xs mb-4 flex items-center gap-3">
                           <ShieldAlert className="w-4 h-4" />
                           Deployment Error
                         </h3>
                         <div className="bg-black/40 rounded-xl p-6 border border-slate-200 dark:border-white/5">
                            <pre className="text-xs text-rose-200/80 whitespace-pre-wrap font-mono leading-relaxed">{project.error_log}</pre>
                         </div>
                      </div>
                   )}
                </div>

                <div className="space-y-6">
                   <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm p-8 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border-slate-300 dark:border-white/10">
                      <h3 className="text-sm font-black text-slate-900 dark:text-white uppercase tracking-widest mb-8 flex items-center gap-3">
                        <Code className="w-4 h-4 text-indigo-400" />
                        Git Repository
                      </h3>
                      <div className="space-y-6">
                         <div className="p-4 rounded-xl bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5">
                            <label className="text-[10px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-widest mb-2 block">Repository URI</label>
                            <div className="flex items-center gap-3">
                               <div className="w-10 h-10 rounded-xl bg-slate-100 dark:bg-white/5 flex items-center justify-center text-slate-600 dark:text-slate-400">
                                  <Globe className="w-5 h-5" />
                               </div>
                               <div className="text-sm text-slate-700 dark:text-slate-300 font-mono truncate">{project.github_url}</div>
                            </div>
                         </div>
                         <div className="grid grid-cols-2 gap-4">
                            <div className="p-4 rounded-xl bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5">
                               <label className="text-[10px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-widest mb-2 block">Active Branch</label>
                               <div className="flex items-center gap-3 text-slate-900 dark:text-white">
                                  <GitBranch className="w-4 h-4 text-indigo-400" />
                                  <span className="text-sm font-black uppercase tracking-tight">{project.branch || 'main'}</span>
                               </div>
                            </div>
                            <div className="p-4 rounded-xl bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5">
                               <label className="text-[10px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-widest mb-2 block">Framework</label>
                               <div className="flex items-center gap-3 text-slate-900 dark:text-white">
                                  <Zap className="w-4 h-4 text-amber-400" />
                                  <span className="text-sm font-black uppercase tracking-tight">Laravel {project.laravel_version || '10.x'}</span>
                               </div>
                            </div>
                         </div>
                      </div>
                   </div>
                </div>
             </div>
          )}

          {/* Console Tab */}
          {activeTab === 'console' && (
            <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm p-0 overflow-hidden flex flex-col h-[650px] border-slate-300 dark:border-white/10 bg-black/40 shadow-2xl relative">
              <div className="p-4 bg-slate-100 dark:bg-slate-100 dark:bg-white/5 border-b border-slate-200 dark:border-white/5 flex items-center justify-between backdrop-blur-md relative z-10">
                <div className="flex items-center gap-4">
                  <div className="flex gap-1.5 px-2">
                     <div className="w-3 h-3 rounded-full bg-rose-500/50 border border-rose-500/20 shadow-[0_0_10px_rgba(244,63,94,0.2)]"/>
                     <div className="w-3 h-3 rounded-full bg-amber-500/50 border border-amber-500/20 shadow-[0_0_10px_rgba(245,158,11,0.2)]"/>
                     <div className="w-3 h-3 rounded-full bg-emerald-500/50 border border-emerald-500/20 shadow-[0_0_10px_rgba(16,185,129,0.2)]"/>
                  </div>
                  <div className="h-4 w-px bg-slate-200 dark:bg-white/10 mx-2" />
                  <div className="flex items-center gap-2 text-slate-600 dark:text-slate-400 font-mono text-[10px] uppercase tracking-widest font-black">
                    <TerminalIcon size={12} className="text-indigo-400" />
                    Artisan Terminal
                  </div>
                </div>
                <div className="flex items-center gap-4">
                    <span className="text-[8px] font-black text-emerald-500 bg-emerald-500/5 px-2 py-0.5 rounded uppercase tracking-widest border border-emerald-500/10 animate-pulse">Socket Active</span>
                    <button onClick={() => setConsoleOutput('')} className="text-slate-600 hover:text-slate-900 dark:text-white transition-colors text-[10px] font-black uppercase tracking-widest flex items-center gap-2">
                        Clear Cache
                    </button>
                </div>
              </div>

              <div className="flex-1 p-8 overflow-auto font-mono text-sm text-slate-700 dark:text-slate-300 whitespace-pre-wrap flex flex-col gap-1 custom-scrollbar scroll-smooth">
                <div className="text-slate-600 mb-6 flex items-center gap-3 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 p-4 rounded-xl border border-slate-200 dark:border-white/5">
                    <ShieldAlert size={14} className="text-amber-500" />
                    <span className="text-[10px] font-black uppercase tracking-widest">Type Artisan commands below. 'php artisan' prefix is added.</span>
                </div>
                {consoleOutput ? consoleOutput.split('\n').map((line, i) => (
                    <div key={i} className="flex gap-4 group">
                        <span className="text-slate-700 text-[10px] select-none w-8 shrink-0 text-right group-hover:text-slate-600 dark:text-slate-400 transition-colors">{i + 1}</span>
                        <span className={`${line.includes('$') ? 'text-indigo-400 font-black' : 'text-slate-600 dark:text-slate-400 font-medium'}`}>{line}</span>
                    </div>
                )) : (
                    <div className="flex flex-col items-center justify-center h-full gap-5 opacity-20">
                        <TerminalIcon size={60} className="text-slate-600 dark:text-slate-400" />
                        <p className="text-xs font-black uppercase tracking-[0.4em]">Ready</p>
                    </div>
                )}
                {isExecuting && (
                    <div className="flex items-center gap-3 text-indigo-400 font-black text-xs uppercase tracking-widest mt-4 pl-12 animate-pulse">
                        <RefreshCw size={12} className="animate-spin" />
                        Running...
                    </div>
                )}
              </div>

              <form onSubmit={handleConsoleSubmit} className="p-6 bg-slate-100 dark:bg-slate-100 dark:bg-white/5 border-t border-slate-200 dark:border-white/5 flex gap-4 backdrop-blur-xl relative z-10">
                <div className="flex items-center px-5 bg-black/40 border border-slate-300 dark:border-white/10 rounded-xl text-slate-600 dark:text-slate-400 font-mono text-xs font-black uppercase tracking-widest select-none">
                  php artisan
                </div>
                <div className="flex-1 relative group">
                    <input 
                      type="text" 
                      value={consoleCommand}
                      onChange={(e) => setConsoleCommand(e.target.value)}
                      placeholder="Enter command (e.g., migrate --seed)..."
                      className="w-full bg-black/40 border border-slate-300 dark:border-white/10 rounded-xl px-6 py-4 text-slate-900 dark:text-white font-mono text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all outline-none"
                      autoFocus
                    />
                </div>
                <button 
                  type="submit" 
                  disabled={isExecuting || !consoleCommand.trim()}
                  className="btn btn-primary px-10 py-4 text-xs font-black uppercase tracking-[0.2em] shadow-lg shadow-indigo-500/20 disabled:opacity-50 active:scale-95 transition-all"
                >
                  {isExecuting ? 'Running...' : 'Run Command'}
                </button>
              </form>
            </div>
          )}

          {/* Environment Tab */}
          {activeTab === 'environment' && (
            <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm p-0 overflow-hidden h-[650px] flex flex-col border-slate-300 dark:border-white/10 bg-slate-50 dark:bg-slate-100 dark:bg-white/5">
               <div className="p-8 border-b border-slate-200 dark:border-white/5 bg-slate-100 dark:bg-slate-100 dark:bg-white/5 flex justify-between items-center backdrop-blur-md">
                  <div className="flex items-center gap-4">
                     <div className="w-12 h-12 rounded-2xl bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400">
                        <SettingsIcon className="w-6 h-6" />
                     </div>
                     <div>
                        <h3 className="text-xl font-black text-slate-900 dark:text-white uppercase tracking-tight">Environment (.env)</h3>
                        <p className="text-[10px] text-slate-600 dark:text-slate-400 font-black uppercase tracking-widest mt-0.5">Application Secrets & Variables</p>
                     </div>
                  </div>
                  <button 
                    onClick={handleSaveEnv}
                    disabled={isSavingEnv}
                    className="btn btn-primary px-8 py-4 text-xs font-black uppercase tracking-widest flex items-center gap-3 disabled:opacity-50 shadow-lg shadow-indigo-500/20"
                  >
                    {isSavingEnv ? (
                        <div className="w-4 h-4 border-2 border-slate-300 dark:border-white/20 border-t-white rounded-full animate-spin" />
                    ) : (
                        <Save className="w-4 h-4" />
                    )}
                    {isSavingEnv ? 'Saving...' : 'Save Secrets'}
                  </button>
               </div>
               <div className="flex-1 relative bg-black/20">
                 <textarea
                   value={envContent}
                   onChange={(e) => setEnvContent(e.target.value)}
                   className="absolute inset-0 w-full h-full bg-transparent text-slate-700 dark:text-slate-300 font-mono text-sm p-10 focus:outline-none resize-none custom-scrollbar selection:bg-indigo-500/30 leading-relaxed"
                   spellCheck="false"
                   placeholder="# Define your application secrets here..."
                 />
                 <div className="absolute top-0 right-0 w-64 h-64 bg-indigo-500/5 blur-[100px] pointer-events-none rounded-full" />
               </div>
               <div className="p-5 bg-indigo-500/5 text-indigo-400 text-[10px] font-black uppercase tracking-[0.2em] border-t border-slate-200 dark:border-white/5 flex items-center justify-center gap-4 backdrop-blur-md">
                  <AlertTriangle className="w-4 h-4 animate-pulse" /> 
                  Saving changes will redeploy your project immediately.
               </div>
            </div>
          )}

          {/* Database Tab */}
          {activeTab === 'database' && (
             <div className="min-h-[650px] animate-pop-in space-y-8">
                <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm p-10 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border-slate-300 dark:border-white/10 group transition-all duration-500 hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5 hover:border-slate-300 dark:border-white/20 relative">
                   <div className="flex flex-col md:flex-row md:items-center justify-between gap-8 mb-12">
                      <div className="flex items-center gap-4">
                        <div className="w-14 h-14 bg-indigo-500/10 border border-indigo-500/20 rounded-2xl flex items-center justify-center text-indigo-400 group-hover:scale-110 transition-transform shadow-xl">
                          <DatabaseIcon className="w-7 h-7" />
                        </div>
                        <div>
                          <h3 className="text-2xl font-black text-slate-900 dark:text-white uppercase tracking-tight">Database Credentials</h3>
                          <p className="text-xs text-slate-600 dark:text-slate-400 font-bold tracking-widest uppercase mt-1">Database Access Info</p>
                        </div>
                      </div>
                      <div className="flex items-center gap-3 bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 px-4 py-2 rounded-xl text-[10px] font-black uppercase tracking-widest text-slate-600 dark:text-slate-400">
                         <ShieldAlert className="w-4 h-4 text-emerald-500" />
                         Private Network Isolation Active
                      </div>
                   </div>

                   <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
                       <CredentialBox 
                        label="Database Host" 
                        value="paas-mysql.cluster.local" 
                        onCopy={() => copyToClipboard('paas-mysql')} 
                      />
                      <CredentialBox 
                        label="Database Port" 
                        value="3306" 
                      />
                      <CredentialBox 
                        label="Database Name" 
                        value={project.database_name} 
                        onCopy={() => copyToClipboard(project.database_name)} 
                      />
                      <CredentialBox 
                        label="Database User" 
                        value={project.database_name} 
                        onCopy={() => copyToClipboard(project.database_name)} 
                      />
                      <CredentialBox 
                        label="Database Password" 
                        value="••••••••••••••••" 
                        isSecret={true}
                        onCopy={() => {
                            copyToClipboard(project.database_name);
                            toast.success('Password copied');
                        }} 
                      />
                      <div className="sm:col-span-1 border border-dashed border-slate-200 dark:border-white/5 rounded-2xl flex items-center justify-center p-6 text-center group/opt">
                         <p className="text-[10px] font-black text-slate-700 uppercase tracking-widest group-hover/opt:text-slate-600 dark:text-slate-400 transition-colors">Credential rotation policy: Manual Only</p>
                      </div>
                   </div>
                </div>

                <DatabaseManager embedded={true} projectId={id} />
             </div>
          )}

          {/* Logs Tab */}
          {activeTab === 'logs' && (
            <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm bg-black/40 border-slate-300 dark:border-white/10 overflow-hidden relative">
               <div className="flex items-center justify-between p-6 bg-slate-100 dark:bg-slate-100 dark:bg-white/5 border-b border-slate-200 dark:border-white/5 backdrop-blur-md">
                  <div className="flex items-center gap-5">
                      <div className="flex gap-1.5 px-2">
                         <div className="w-3 h-3 rounded-full bg-rose-500/50 border border-rose-500/20 shadow-[0_0_10px_rgba(244,63,94,0.2)]"/>
                         <div className="w-3 h-3 rounded-full bg-amber-500/50 border border-amber-500/20 shadow-[0_0_10px_rgba(245,158,11,0.2)]"/>
                         <div className="w-3 h-3 rounded-full bg-emerald-500/50 border border-emerald-500/20 shadow-[0_0_10px_rgba(16,185,129,0.2)]"/>
                      </div>
                      <div className="h-4 w-px bg-slate-200 dark:bg-white/10 mx-2" />
                      <div className="text-[10px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-widest flex items-center gap-2">
                        <Activity size={12} className="text-indigo-400" />
                        Console Logs
                      </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <span className="text-[10px] font-black text-slate-700 uppercase tracking-widest italic">Node: {project.container_id?.substring(0,12) || 'awaiting_id'}</span>
                    <button onClick={fetchLogs} className="w-8 h-8 rounded-lg bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 flex items-center justify-center text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white transition-all">
                        <RefreshCw size={14} />
                    </button>
                  </div>
               </div>
               <div className="p-8 h-[650px] overflow-auto font-mono text-xs leading-relaxed custom-scrollbar bg-black/10">
                  {logs ? (
                    logs.split('\n').filter(l => l.trim()).map((line, i) => (
                      <div key={i} className="flex gap-6 group hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5 px-4 py-1.5 rounded-lg transition-colors border border-transparent hover:border-slate-200 dark:border-white/5">
                        <span className="text-slate-700 text-[10px] select-none w-8 shrink-0 text-right group-hover:text-slate-600 dark:text-slate-400 transition-colors">{(i+1)}</span>
                        <span className="text-slate-600 dark:text-slate-400 font-medium">{line}</span>
                      </div>
                    ))
                  ) : (
                    <div className="flex flex-col items-center justify-center h-full gap-5 opacity-20">
                        <Activity size={60} className="text-slate-600 dark:text-slate-400" />
                        <p className="text-xs font-black uppercase tracking-[0.4em]">Streaming Logs...</p>
                    </div>
                  )}
                  <div ref={logsEndRef} />
               </div>
               <div className="absolute top-0 right-0 w-[30vw] h-[30vw] bg-indigo-500/5 blur-[120px] pointer-events-none rounded-full" />
            </div>
          )}

          {/* Settings Tab */}
          {activeTab === 'settings' && (
              <div className="grid grid-cols-1 md:grid-cols-2 gap-10">
                <div className={`bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm p-10 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border-slate-300 dark:border-white/10 group transition-all duration-500 hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5 hover:border-slate-300 dark:border-white/20 relative ${isPhpDropdownOpen ? 'z-50' : 'z-10'}`}>
                   
                   
                   <div className="flex items-center gap-4 mb-10">
                      <div className="w-14 h-14 bg-indigo-500/10 border border-indigo-500/20 rounded-2xl flex items-center justify-center text-indigo-400 group-hover:scale-110 transition-transform shadow-xl">
                        <Zap className="w-7 h-7" />
                      </div>
                      <div>
                        <h3 className="text-2xl font-black text-slate-900 dark:text-white uppercase tracking-tight">PHP Settings</h3>
                        <p className="text-xs text-slate-600 dark:text-slate-400 font-bold tracking-widest uppercase mt-1">PHP Version Settings</p>
                      </div>
                   </div>

                   <div className="space-y-8">
                      <div className="space-y-3">
                         <label className="text-[10px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-[0.2em] ml-1 flex items-center gap-2">Platform Runtime (PHP)</label>
                         <div className="relative">
                            <button
                              type="button"
                              onClick={() => setIsPhpDropdownOpen(!isPhpDropdownOpen)}
                              className="w-full bg-black/40 border border-slate-300 dark:border-white/10 rounded-2xl px-6 py-5 text-slate-900 dark:text-white font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all outline-none flex items-center justify-between group"
                            >
                               <div className="flex items-center gap-4">
                                  <div className="w-8 h-8 rounded-lg bg-indigo-500/10 flex items-center justify-center text-indigo-400 font-black text-[10px]">
                                     {project.php_version?.replace('dynamic', '').replace('.fpm', '') || '8.2'}
                                  </div>
                                  <span className="text-sm font-black tracking-tight uppercase">PHP {project.php_version?.replace('dynamic', '').replace('.fpm', '') || '8.2'}</span>
                               </div>
                               <div className="text-slate-700 font-bold text-[10px] uppercase tracking-widest flex items-center gap-2">
                                  {isPhpDropdownOpen ? 'Close' : 'Select Version'}
                                  <ChevronRight className={`w-3 h-3 transition-transform duration-300 ${isPhpDropdownOpen ? 'rotate-90' : ''}`} />
                               </div>
                            </button>

                            {isPhpDropdownOpen && (
                              <>
                                <div className="fixed inset-0 z-[60]" onClick={() => setIsPhpDropdownOpen(false)}></div>
                                <div className="absolute top-full left-0 w-full mt-3 bg-[#09090b] border border-slate-300 dark:border-white/10 rounded-3xl p-3 z-[100] shadow-[0_30px_60px_rgba(0,0,0,0.8)] animate-in fade-in slide-in-from-top-2 duration-200">
                                   {[8.0, 8.1, 8.2, 8.3, 8.4].map((v) => (
                                     <button
                                       key={v}
                                       type="button"
                                       onClick={() => {
                                         handleUpdatePHP(v.toFixed(1))
                                         setIsPhpDropdownOpen(false)
                                       }}
                                       className={`w-full flex items-center justify-between px-5 py-4 rounded-xl transition-all group/opt ${
                                         (project.php_version?.includes(v.toFixed(1)) || (!project.php_version && v === 8.2))
                                           ? 'bg-slate-100 dark:bg-white/5 text-slate-900 dark:text-white' 
                                           : 'text-slate-600 dark:text-slate-400 hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5 hover:text-slate-900 dark:text-white'
                                       }`}
                                     >
                                       <div className="flex items-center gap-4">
                                          <div className={`w-10 h-10 rounded-xl flex items-center justify-center font-black text-[11px] transition-all ${
                                            (project.php_version?.includes(v.toFixed(1)) || (!project.php_version && v === 8.2))
                                              ? 'bg-indigo-500 text-slate-900 dark:text-white'
                                              : 'bg-slate-100 dark:bg-white/5 text-slate-600 dark:text-slate-400 group-hover/opt:bg-slate-200 dark:bg-white/10 group-hover/opt:text-slate-900 dark:text-white'
                                          }`}>
                                             {v.toFixed(1)}
                                          </div>
                                          <div className="text-left">
                                             <p className="text-xs font-black uppercase tracking-tight">PHP {v.toFixed(1)}</p>
                                             <p className="text-[9px] text-slate-600 font-bold uppercase tracking-widest mt-0.5">{v === 8.4 ? 'Latest Stable' : 'Older Version'}</p>
                                          </div>
                                       </div>
                                       {v.toFixed(1) === '8.4' && (
                                         <div className="px-2 py-0.5 rounded-md bg-emerald-500/10 border border-emerald-500/20 text-emerald-500 text-[8px] font-black uppercase tracking-widest">
                                            Recommended
                                         </div>
                                       )}
                                     </button>
                                   ))}
                                </div>
                              </>
                            )}
                         </div>
                         <p className="text-[11px] text-slate-600 font-medium italic pl-1 flex items-center gap-2 mt-3">
                            <AlertTriangle size={10} className="text-amber-500" /> Changing PHP version will redeploy your project.
                         </p>
                      </div>
                   </div>
                </div>

                <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm p-10 bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border-slate-300 dark:border-white/10 group transition-all duration-500 hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5 hover:border-slate-300 dark:border-white/20 relative">
                   
                   
                   <div className="flex items-center gap-4 mb-10">
                      <div className="w-14 h-14 bg-emerald-500/10 border border-emerald-500/20 rounded-2xl flex items-center justify-center text-emerald-400 group-hover:scale-110 transition-transform shadow-xl">
                        <RefreshCw className="w-7 h-7" />
                      </div>
                      <div>
                        <h3 className="text-2xl font-black text-slate-900 dark:text-white uppercase tracking-tight">Queue Settings</h3>
                        <p className="text-xs text-slate-600 dark:text-slate-400 font-bold tracking-widest uppercase mt-1">Background Processing</p>
                      </div>
                   </div>

                   <div className="space-y-10">
                      <div className="flex items-center justify-between p-8 rounded-3xl bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10">
                          <div>
                              <h4 className="text-sm font-black text-slate-900 dark:text-white uppercase tracking-widest mb-1">Queue Worker</h4>
                              <p className="text-[11px] text-slate-600 dark:text-slate-400 font-medium italic">Run background jobs (queue:work)</p>
                          </div>
                          <button 
                             onClick={() => handleUpdateQueue(!project.queue_enabled)}
                             className={`w-14 h-7 rounded-full p-1.5 cursor-pointer transition-all duration-500 border-none focus:ring-0 shadow-lg ${project.queue_enabled ? 'bg-emerald-500' : 'bg-slate-700'}`}
                          >
                             <div className={`w-4 h-4 rounded-full bg-white transition-transform duration-300 shadow-md transform ${project.queue_enabled ? 'translate-x-7' : 'translate-x-0'}`} />
                          </button>
                      </div>
                      
                      <div className="p-6 rounded-2xl bg-emerald-500/5 border border-emerald-500/10 flex items-start gap-4">
                         <div className="w-10 h-10 rounded-xl bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center text-emerald-400 shrink-0">
                            <Box className="w-5 h-5" />
                         </div>
                         <p className="text-[11px] text-slate-600 dark:text-slate-400 font-medium leading-relaxed italic">
                            Enabling the <span className="text-emerald-400 font-black">Queue Worker</span> allows tasks to run in the background.
                         </p>
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

function CredentialBox({ label, value, onCopy, isSecret = false }) {
  return (
    <div className="p-8 rounded-[2.5rem] bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 group transition-all duration-500 hover:border-indigo-500/20 hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5">
       <label className="text-[9px] font-black text-slate-700 uppercase tracking-[0.3em] mb-6 block group-hover:text-indigo-400 transition-colors">{label}</label>
       <div className="flex items-center justify-between gap-6">
          <code className={`font-mono text-xs text-slate-200 truncate font-black tracking-tight ${isSecret ? 'tracking-[0.6em] opacity-20 select-none' : ''}`}>{value}</code>
          {onCopy && (
            <button 
              onClick={onCopy} 
              className="w-12 h-12 rounded-2xl bg-slate-50 dark:bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 text-slate-600 hover:text-slate-900 dark:text-white hover:bg-indigo-500 hover:border-indigo-400 transition-all shadow-xl flex items-center justify-center shrink-0 active:scale-90"
              title="Copy to clipboard"
            >
              <Copy className="w-5 h-5 transition-transform group-hover/btn:scale-110" />
            </button>
          )}
       </div>
    </div>
  )
}

