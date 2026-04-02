// ===========================================
// Student Dashboard Page
// ===========================================

import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { projectsAPI } from '../../services/api'
import useAuthStore from '../../stores/authStore'
import { 
  Rocket, 
  Activity, 
  CheckCircle2, 
  Package, 
  ExternalLink, 
  ArrowRight,
  Plus,
  Clock,
  AlertCircle,
  PauseCircle,
  Zap,
  Layout,
  Terminal,
  ChevronRight
} from 'lucide-react'

// Status badge component
function StatusBadge({ status }) {
  const configs = {
    pending: { color: 'text-amber-400', border: 'border-amber-400/20', bg: 'bg-amber-400/10', icon: Clock, label: 'In Queue' },
    building: { color: 'text-blue-400', border: 'border-blue-400/20', bg: 'bg-blue-400/10', icon: Activity, label: 'Building', pulse: true },
    running: { color: 'text-emerald-400', border: 'border-emerald-400/20', bg: 'bg-emerald-400/10', icon: CheckCircle2, label: 'Running' },
    failed: { color: 'text-rose-400', border: 'border-rose-400/20', bg: 'bg-rose-400/10', icon: AlertCircle, label: 'Failed' },
    stopped: { color: 'text-slate-600 dark:text-slate-400', border: 'border-slate-400/20', bg: 'bg-slate-400/10', icon: PauseCircle, label: 'Stopped' },
  }

  const config = configs[status] || configs.pending
  const Icon = config.icon
  
  return (
    <span className={`px-4 py-1.5 rounded-full border text-[10px] font-black uppercase tracking-widest flex items-center gap-2 w-fit ${config.color} ${config.border} ${config.bg}`}>
      <Icon className={`w-3 h-3 ${config.pulse ? 'animate-spin' : ''}`} />
      {config.label}
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
    <div className="space-y-12 animate-pop-in relative">
      

      {/* Welcome Header */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-8 relative z-10">
        <div>
          <h1 className="text-5xl font-black text-slate-900 dark:text-white tracking-tighter mb-4 italic text-transparent bg-clip-text pr-2 pb-1 bg-gradient-to-r from-indigo-400 via-purple-400 to-indigo-400 animate-gradient-x">
            Dashboard
          </h1>
          <p className="text-slate-600 dark:text-slate-400 text-lg font-medium max-w-xl leading-relaxed">
            Welcome back, <span className="text-slate-900 dark:text-white font-bold">{user?.name?.split(' ')[0]}</span>. You currently have <span className="text-indigo-400 font-bold">{runningProjects} active projects</span>.
          </p>
        </div>
        <Link to="/projects/new" className="btn btn-primary shadow-[0_0_30px_rgba(99,102,241,0.2)]">
          <Plus className="w-5 h-5" />
          New Project
        </Link>
      </div>
      
      {/* Core Stats */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-8 relative z-10">
        <StatCard 
          label="Total Projects" 
          value={totalProjects} 
          icon={Package} 
          color="indigo" 
        />
        <StatCard 
          label="Running Projects" 
          value={runningProjects} 
          icon={Activity} 
          color="emerald" 
          suffix={`/ ${totalProjects}`}
        />
        <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm p-8 bg-gradient-to-br from-indigo-600/10 to-transparent border-slate-300 dark:border-white/10 group hover:scale-[1.02] transition-all duration-500 overflow-hidden relative">
           
           <div className="relative z-10 h-full flex flex-col justify-between">
              <div>
                <Zap className="w-8 h-8 text-indigo-400 mb-4" />
                <h3 className="text-xl font-black text-slate-900 dark:text-white uppercase tracking-tight">Need Help?</h3>
                <p className="text-slate-600 dark:text-slate-400 text-xs font-bold uppercase tracking-widest mt-1">Get Technical support</p>
              </div>
              <Link to="/feedback" className="flex items-center gap-2 text-slate-900 dark:text-white font-black text-xs uppercase tracking-[0.2em] group-hover:gap-4 transition-all mt-6">
                 Support Ticket <ArrowRight className="w-4 h-4 text-indigo-400" />
              </Link>
           </div>
        </div>
      </div>
      
      {/* Recent Activity Table */}
      <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm overflow-hidden border-slate-300 dark:border-white/10 relative z-10">
        <div className="p-10 border-b border-slate-200 dark:border-white/5 flex items-center justify-between bg-slate-50 dark:bg-slate-100 dark:bg-white/5">
          <div className="flex items-center gap-4">
             <div className="w-10 h-10 rounded-xl bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 flex items-center justify-center">
                <Layout className="w-5 h-5 text-slate-600 dark:text-slate-400" />
             </div>
             <div>
                <h2 className="text-xl font-black text-slate-900 dark:text-white tracking-tight uppercase">Recent Projects</h2>
                <p className="text-slate-600 dark:text-slate-400 text-[10px] font-black uppercase tracking-[0.2em]">Latest activity</p>
             </div>
          </div>
          <Link to="/projects" className="group flex items-center gap-2 text-[10px] font-black uppercase tracking-widest text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white transition-all">
            Browse All <ChevronRight className="w-4 h-4 group-hover:translate-x-1 transition-transform" />
          </Link>
        </div>
        
        {isLoading ? (
          <div className="p-32 flex flex-col items-center justify-center gap-6">
            <div className="w-12 h-12 border-4 border-indigo-500/20 border-t-indigo-500 rounded-full animate-spin"></div>
            <p className="text-slate-600 dark:text-slate-400 text-[10px] font-black uppercase tracking-widest animate-pulse">Loading Projects...</p>
          </div>
        ) : projects.length === 0 ? (
          <div className="p-32 text-center flex flex-col items-center max-w-sm mx-auto">
            <div className="w-24 h-24 bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 rounded-[2.5rem] flex items-center justify-center mb-8">
              <Rocket className="w-12 h-12 text-slate-700" />
            </div>
            <h4 className="text-2xl font-black text-slate-900 dark:text-white tracking-tight mb-3">No Projects</h4>
            <p className="text-slate-600 dark:text-slate-400 font-medium mb-10 text-sm">You have no active projects yet. Create your first project to get started.</p>
            <Link to="/projects/new" className="btn btn-primary w-full py-5">
              Create Your First Project
            </Link>
          </div>
        ) : (
          <div className="table-container">
            <table className="premium-table">
              <thead>
                <tr>
                  <th>Project Name</th>
                  <th>URL</th>
                  <th>Status</th>
                  <th>Date</th>
                  <th className="text-right">Action</th>
                </tr>
              </thead>
              <tbody>
                {projects.slice(0, 5).map((project) => (
                  <tr key={project.id} className="group">
                    <td>
                       <div className="flex items-center gap-4">
                          <div className="w-10 h-10 bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 rounded-xl flex items-center justify-center text-slate-600 dark:text-slate-400 font-black text-xs uppercase group-hover:border-indigo-500/30 group-hover:text-indigo-400 transition-all">
                             {project.name.charAt(0)}
                          </div>
                          <div>
                            <span className="font-black text-sm text-slate-900 dark:text-white group-hover:text-indigo-400 transition-colors uppercase tracking-tight">{project.name}</span>
                            <div className="flex items-center gap-1.5 mt-0.5 opacity-40">
                               <Terminal className="w-3 h-3" />
                               <span className="text-[10px] font-mono italic">laravel-v10-pro</span>
                            </div>
                          </div>
                       </div>
                    </td>
                    <td>
                      {project.status === 'running' ? (
                        <a 
                          href={`https://${project.subdomain}.${window.location.hostname}`}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="flex items-center gap-2 group/link"
                        >
                          <span className="font-mono text-[11px] text-slate-600 dark:text-slate-400 group-hover/link:text-indigo-400 transition-colors">{project.subdomain}</span>
                          <ExternalLink className="w-3.5 h-3.5 text-slate-700 group-hover/link:text-indigo-400 group-hover/link:translate-x-0.5 group-hover/link:-translate-y-0.5 transition-all" />
                        </a>
                      ) : (
                        <span className="text-slate-800 font-mono text-[11px] italic uppercase">Inactive</span>
                      )}
                    </td>
                    <td><StatusBadge status={project.status} /></td>
                    <td>
                        <div className="flex flex-col">
                           <span className="text-slate-600 dark:text-slate-400 font-bold text-xs">{new Date(project.created_at).toLocaleDateString()}</span>
                           <span className="text-[9px] text-slate-600 uppercase font-black tracking-widest mt-0.5">{new Date(project.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
                        </div>
                    </td>
                    <td className="text-right">
                      <Link 
                        to={`/projects/${project.id}`}
                        className="btn bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 hover:border-indigo-500/20 hover:bg-indigo-500/10 text-slate-600 dark:text-slate-400 hover:text-indigo-400 py-2 px-4 text-[10px] uppercase font-black tracking-widest"
                      >
                        Details
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

function StatCard({ label, value, icon: Icon, color, suffix }) {
  const colors = {
    indigo: 'text-indigo-400 bg-indigo-500/10 border-indigo-500/20',
    emerald: 'text-emerald-400 bg-emerald-500/10 border-emerald-500/20',
  }
  
  return (
    <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm p-8 group hover:scale-[1.02] transition-all duration-500 border-slate-300 dark:border-white/10 bg-slate-50 dark:bg-slate-100 dark:bg-white/5">
       <div className="flex justify-between items-start">
          <div>
             <p className="text-slate-600 dark:text-slate-400 text-[10px] font-black uppercase tracking-[0.2em] mb-3">{label}</p>
             <div className="flex items-baseline gap-2">
                <h3 className="text-5xl font-black text-slate-900 dark:text-white tracking-tighter">{value}</h3>
                {suffix && <span className="text-slate-600 font-black text-lg">{suffix}</span>}
             </div>
          </div>
          <div className={`w-16 h-16 rounded-2xl flex items-center justify-center transition-all duration-500 group-hover:rotate-12 ${colors[color] || colors.indigo}`}>
             <Icon className="w-8 h-8" />
          </div>
       </div>
    </div>
  )
}

export default StudentDashboard
