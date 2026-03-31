// ===========================================
// Student Projects Listing (Fleet View)
// ===========================================

import { useState, useEffect, useCallback } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import toast from 'react-hot-toast'
import { projectsAPI } from '../../services/api'
import { 
  Plus, 
  Rocket, 
  ExternalLink, 
  RefreshCw, 
  Trash2, 
  Clock, 
  CheckCircle2, 
  Activity, 
  AlertCircle, 
  PauseCircle,
  Server,
  Database,
  Globe,
  Terminal,
  Cpu,
  Layers,
  ArrowRight
} from 'lucide-react'
import ConfirmationModal from '../../components/ConfirmationModal'

const StatusBadge = ({ status }) => {
  const configs = {
    pending: { color: 'text-amber-400 border-amber-400/20 bg-amber-400/5', icon: Clock, label: 'Queued' },
    building: { color: 'text-blue-400 border-blue-400/20 bg-blue-400/5', icon: Activity, label: 'Orchestrating', pulse: true },
    running: { color: 'text-emerald-400 border-emerald-400/20 bg-emerald-400/5', icon: CheckCircle2, label: 'Active' },
    failed: { color: 'text-rose-400 border-rose-400/20 bg-rose-400/5', icon: AlertCircle, label: 'Breach' },
    stopped: { color: 'text-slate-600 dark:text-slate-400 border-slate-300 dark:border-white/10 bg-slate-100 dark:bg-white/5', icon: PauseCircle, label: 'Hibernating' },
  }

  const config = configs[status] || configs.pending
  const Icon = config.icon

  return (
    <div className={`flex items-center gap-2 px-3 py-1 rounded-full border text-[9px] font-black uppercase tracking-widest ${config.color} backdrop-blur-md`}>
      <Icon className={`w-3 h-3 ${config.pulse ? 'animate-spin' : ''}`} />
      {config.label}
    </div>
  )
}

const StudentProjects = () => {
  const navigate = useNavigate()
  const [projects, setProjects] = useState([])
  const [isLoading, setIsLoading] = useState(true)
  
  const [confirmModal, setConfirmModal] = useState({
    isOpen: false,
    title: '',
    message: '',
    type: 'danger',
    onConfirm: () => {},
    confirmText: 'Confirm'
  })

  const fetchProjects = useCallback(async () => {
    setIsLoading(true)
    try {
      const response = await projectsAPI.listOwn()
      setProjects(response.data.data || [])
    } catch (error) {
      toast.error('Failed to load projects')
    } finally {
      setIsLoading(false)
    }
  }, [])
  
  useEffect(() => {
    fetchProjects()
  }, [fetchProjects])
  
  const handleRedeploy = async (id, e) => {
    e.preventDefault()
    e.stopPropagation()
    
    setConfirmModal({
      isOpen: true,
      title: 'Redeploy Project?',
      message: 'This will rebuild your project. It may be temporarily unavailable during the process.',
      type: 'warning',
      confirmText: 'Redeploy',
      onConfirm: () => {
        toast.promise(
          projectsAPI.redeploy(id),
          {
            loading: 'Redeploying project...',
            success: 'Redeploy started',
            error: 'Failed to redeploy'
          }
        ).then(fetchProjects)
      }
    })
  }
  
  const handleDelete = async (id, e) => {
    e.preventDefault()
    e.stopPropagation()
    
    setConfirmModal({
      isOpen: true,
      title: 'Delete Project?',
      message: 'This will permanently delete this project and all its data. This action cannot be undone.',
      type: 'danger',
      confirmText: 'Delete Project',
      onConfirm: async () => {
        try {
          await projectsAPI.delete(id)
          toast.success('Project Deleted')
          fetchProjects()
        } catch (error) {
          toast.error('Delete Failed')
        }
      }
    })
  }
  
  return (
    <div className="space-y-16 animate-pop-in relative">
      <ConfirmationModal 
        isOpen={confirmModal.isOpen}
        onClose={() => setConfirmModal(prev => ({ ...prev, isOpen: false }))}
        {...confirmModal}
      />

      {/* Background Glow */}
      

      {/* Header Container */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-10 relative z-10">
        <div>
          <h1 className="text-5xl font-black text-slate-900 dark:text-white tracking-tighter mb-4 italic text-transparent bg-clip-text pr-2 pb-1 bg-gradient-to-r from-indigo-400 to-purple-400">Projects</h1>
          <p className="text-slate-600 dark:text-slate-400 text-lg font-medium leading-relaxed max-w-2xl">
            Manage and monitor all your projects in our <span className="text-slate-900 dark:text-white">modern</span> dashboard interface.
          </p>
        </div>
        <Link to="/projects/new" className="btn btn-primary px-10 py-5 text-sm font-black uppercase tracking-[0.25em] shadow-[0_15px_30px_rgba(99,102,241,0.3)] flex items-center gap-4 group">
          <Plus className="w-5 h-5 group-hover:rotate-90 transition-transform duration-500" />
          New Project
        </Link>
      </div>
      
      {/* Grid Architecture */}
      {isLoading ? (
        <div className="flex flex-col items-center justify-center py-40 gap-6 opacity-30">
          <div className="w-12 h-12 border-4 border-indigo-500/20 border-t-indigo-500 rounded-full animate-spin"></div>
          <p className="text-[10px] font-black uppercase tracking-[0.4em]">Loading Projects...</p>
        </div>
      ) : projects.length === 0 ? (
        <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm border-dashed p-32 text-center flex flex-col items-center max-w-xl mx-auto border-slate-200 dark:border-white/5 bg-slate-50 dark:bg-slate-100 dark:bg-white/5">
          <div className="w-24 h-24 bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 rounded-[2.5rem] flex items-center justify-center mb-10 text-slate-700">
            <Rocket className="w-12 h-12" />
          </div>
          <h2 className="text-3xl font-black text-slate-900 dark:text-white tracking-tight mb-4 lowercase italic">The list is <span className="text-indigo-400">empty.</span></h2>
          <p className="text-slate-600 dark:text-slate-400 mb-12 font-medium leading-relaxed">You have no active projects. Create your first project to begin monitoring.</p>
          <Link to="/projects/new" className="btn btn-primary w-full py-6 text-sm font-black uppercase tracking-widest shadow-xl">
            Create Project
          </Link>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-10 relative z-10 pb-20">
          {projects.map((project) => (
            <div 
              key={project.id} 
              onClick={() => navigate(`/projects/${project.id}`)}
              className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm group p-0 overflow-hidden flex flex-col h-full hover:bg-slate-50 dark:bg-slate-100 dark:bg-white/5 hover:border-slate-300 dark:border-white/20 transition-all duration-500 hover:-translate-y-2 cursor-pointer"
            >
              <div className="p-10 flex flex-col h-full bg-gradient-to-br from-white/[0.02] to-transparent">
                <div className="flex items-start justify-between mb-10">
                  <div className="w-16 h-16 rounded-2xl bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400 group-hover:scale-110 transition-transform shadow-xl">
                     <span className="text-2xl font-black italic">{project.name.charAt(0).toUpperCase()}</span>
                  </div>
                  <StatusBadge status={project.status} />
                </div>

                <div className="mb-10 min-h-[80px]">
                  <h3 className="font-black text-slate-900 dark:text-white text-2xl tracking-tighter group-hover:text-indigo-400 transition-colors uppercase truncate mb-2">
                    {project.name}
                  </h3>
                  <a 
                    href={`http://${project.subdomain}.paas.local`}
                    target="_blank"
                    rel="noopener noreferrer"
                    onClick={(e) => e.stopPropagation()}
                    className="flex items-center gap-3 text-slate-600 dark:text-slate-400 font-mono text-[10px] tracking-widest bg-slate-100 dark:bg-white/5 px-3 py-1.5 rounded-lg w-fit border border-slate-200 dark:border-white/5 hover:border-indigo-500/40 hover:text-indigo-400 transition-all group/link"
                  >
                    <Globe className="w-3.5 h-3.5 text-indigo-400 group-hover/link:animate-pulse" />
                    {project.subdomain}.paas.local
                    <ExternalLink className="w-3 h-3 opacity-50 group-hover/link:opacity-100 transition-opacity" />
                  </a>
                </div>

                <div className="space-y-6 py-8 border-y border-slate-200 dark:border-white/5">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3 text-slate-600 dark:text-slate-400">
                      <div className="p-2 bg-slate-100 dark:bg-white/5 rounded-lg border border-slate-200 dark:border-white/5">
                        <Cpu className="w-3.5 h-3.5" />
                      </div>
                      <span className="text-[10px] font-black uppercase tracking-[0.2em]">Environment</span>
                    </div>
                    <span className="text-[10px] font-black text-slate-900 dark:text-white bg-slate-200 dark:bg-white/10 px-3 py-1 rounded-md border border-slate-300 dark:border-white/10 group-hover:bg-indigo-500/20 group-hover:border-indigo-500/30 transition-all">PHP {project.php_version || '8.2'}</span>
                  </div>
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3 text-slate-600 dark:text-slate-400">
                      <div className="p-2 bg-slate-100 dark:bg-white/5 rounded-lg border border-slate-200 dark:border-white/5">
                        <Database className="w-3.5 h-3.5" />
                      </div>
                      <span className="text-[10px] font-black uppercase tracking-[0.2em]">Database</span>
                    </div>
                    <span className="text-[10px] font-black text-indigo-400 uppercase tracking-widest">{project.database_name ? 'Active' : 'No Database'}</span>
                  </div>
                </div>

                <div className="mt-10 flex items-center justify-between">
                   <div className="flex flex-col">
                      <span className="text-[8px] text-slate-600 font-black uppercase tracking-[0.2em] mb-1">Created Date</span>
                      <span className="text-[11px] font-black text-slate-600 dark:text-slate-400 uppercase">{new Date(project.created_at).toLocaleDateString()}</span>
                   </div>
                   
                    <div className="flex gap-3">
                      <button 
                        onClick={(e) => handleRedeploy(project.id, e)}
                        className="w-11 h-11 flex items-center justify-center rounded-xl bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white hover:bg-indigo-500/40 hover:border-indigo-500/50 transition-all shadow-lg active:scale-95"
                        title="Init Redeploy"
                      >
                         <RefreshCw className="w-4 h-4" />
                      </button>
                      <button 
                        onClick={(e) => handleDelete(project.id, e)}
                        className="w-11 h-11 flex items-center justify-center rounded-xl bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 text-slate-600 dark:text-slate-400 hover:text-rose-400 hover:bg-rose-500/20 hover:border-rose-500/30 transition-all shadow-lg active:scale-95"
                        title="Decommission"
                      >
                         <Trash2 className="w-4 h-4" />
                      </button>
                      <div className="w-11 h-11 flex items-center justify-center rounded-xl bg-indigo-500/10 border border-indigo-500/20 text-indigo-400 group-hover:bg-indigo-500 group-hover:text-white transition-all shadow-xl group-hover:scale-110">
                         <ArrowRight className="w-5 h-5 transition-transform" />
                      </div>
                   </div>
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

