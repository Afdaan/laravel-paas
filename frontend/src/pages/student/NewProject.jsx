// ===========================================
// New Project Page
// ===========================================

import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import toast from 'react-hot-toast'
import { projectsAPI } from '../../services/api'
import { 
  Rocket, 
  Github, 
  Database, 
  Settings, 
  Activity, 
  Info,
  ChevronLeft,
  ArrowRight,
  ShieldCheck,
  Zap,
  Cpu,
  RefreshCw
} from 'lucide-react'

function StudentNewProject() {
  const navigate = useNavigate()
  const [isLoading, setIsLoading] = useState(false)
  const [submitError, setSubmitError] = useState(null)
  const [formData, setFormData] = useState({
    name: '',
    github_url: '',
    branch: '',
    database_name: '',
    queue_enabled: false,
  })
  const [validationErrors, setValidationErrors] = useState({})
  
  const handleChange = (e) => {
    const { name, value, type, checked } = e.target
    setFormData(prev => ({ 
        ...prev, 
        [name]: type === 'checkbox' ? checked : value 
    }))
    
    if (name === 'name') {
      const dbName = value.toLowerCase()
        .replace(/[^a-z0-9]+/g, '_')
        .replace(/^_|_$/g, '')
      setFormData(prev => ({ ...prev, database_name: dbName }))
    }

    if (validationErrors[name]) {
      setValidationErrors(prev => ({ ...prev, [name]: null }))
    }
  }
  
  const validateForm = () => {
    const errors = {}
    if (!formData.name.trim()) errors.name = 'Project name is required'
    if (!formData.github_url.trim()) errors.github_url = 'Repository URL is required'
    if (!formData.database_name.trim()) errors.database_name = 'Database name is required'
    
    setValidationErrors(errors)
    return Object.keys(errors).length === 0
  }
  
  const handleSubmit = async (e) => {
    e.preventDefault()
    
    if (!validateForm()) return
    
    setIsLoading(true)
    setSubmitError(null)
    
    try {
      const response = await projectsAPI.create(formData)
      toast.success('Project Created', {
        duration: 5000,
        style: {
          borderRadius: '16px',
          background: '#111114',
          color: '#fff',
          border: '1px solid rgba(255,255,255,0.08)',
        },
      })
      navigate(`/projects/${response.data.project.id}`)
    } catch (error) {
      let errorMsg = error.response?.data?.error || 'Failed to create project'
      
      // Intercept technical backend error and provide actionable human-readable feedback
      if (errorMsg === 'Project limit reached' || error.response?.status === 403) {
        errorMsg = 'Project limit reached. Please delete an existing project from your dashboard before creating a new one.'
      }
      
      setSubmitError(errorMsg)
      
    } finally {
      setIsLoading(false)
    }
  }
  
  return (
    <div className="max-w-4xl mx-auto space-y-10 relative">
      {/* Background Orbs */}
      <div className="absolute top-0 left-1/2 -translate-x-1/2 w-[60vw] h-[60vw] bg-indigo-600/5 blur-[120px] rounded-full pointer-events-none z-0"></div>

      <div className="relative z-10">
        <button 
          onClick={() => navigate(-1)}
          className="flex items-center gap-2 text-slate-500 hover:text-white transition-colors mb-6 group bg-white/5 px-4 py-2 rounded-xl border border-white/5"
        >
          <ChevronLeft className="w-4 h-4 group-hover:-translate-x-1 transition-transform" />
          <span className="text-xs font-black uppercase tracking-widest">Back to projects</span>
        </button>

        <div className="mb-10 animate-pop-in">
          <h1 className="text-5xl font-black text-white tracking-tighter mb-4 italic">New <span className="text-transparent bg-clip-text bg-gradient-to-r from-indigo-400 to-purple-400">Project</span></h1>
          <p className="text-slate-400 font-medium text-lg">Scale your Laravel application in seconds with automated cloud deployment.</p>
        </div>
        
        {submitError && (
          <div className="mb-8 p-6 rounded-2xl bg-rose-500/10 border border-rose-500/30 flex flex-col items-center justify-center text-center animate-pop-in">
             <div className="w-12 h-12 bg-rose-500/20 rounded-full flex items-center justify-center mb-4 text-rose-400">
                <Info className="w-6 h-6" />
             </div>
             <h3 className="text-lg font-black text-white tracking-tight uppercase mb-2">Notice</h3>
             <p className="text-rose-200/80 font-medium leading-relaxed max-w-lg">{submitError}</p>
          </div>
        )}
        
        <form onSubmit={handleSubmit} noValidate className="grid grid-cols-1 lg:grid-cols-3 gap-8 animate-pop-in" style={{ animationDelay: '100ms' }}>
          <div className="lg:col-span-2 space-y-8">
            <div className="card-glass p-10 space-y-8 border-white/10">
              {/* Project Name */}
              <div className="space-y-4">
                <div className="flex items-center gap-3 mb-2">
                  <div className="w-8 h-8 rounded-lg bg-indigo-500/10 flex items-center justify-center text-indigo-400 border border-indigo-500/20">
                    <Rocket className="w-4 h-4" />
                  </div>
                  <label htmlFor="name" className="text-sm font-black text-white uppercase tracking-widest">
                    Project Name
                  </label>
                </div>
                <input
                  id="name"
                  name="name"
                  type="text"
                  value={formData.name}
                  onChange={handleChange}
                  className={`input-field py-4 ${validationErrors.name ? '!border-rose-500/50 focus:!border-rose-500 focus:!ring-rose-500/20' : ''}`}
                  placeholder="e.g., Stellar Marketing API"
                />
                {validationErrors.name && (
                   <p className="text-[10px] text-rose-400 font-bold uppercase tracking-widest pl-1 mt-1 animate-pop-in">{validationErrors.name}</p>
                )}
              </div>
              
              {/* GitHub Settings */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
                <div className="space-y-4">
                  <div className="flex items-center gap-3 mb-2">
                    <div className="w-8 h-8 rounded-lg bg-slate-500/10 flex items-center justify-center text-slate-400 border border-white/5">
                      <Github className="w-4 h-4" />
                    </div>
                    <label htmlFor="github_url" className="text-sm font-black text-white uppercase tracking-widest">
                      Repository URL
                    </label>
                  </div>
                  <input
                    id="github_url"
                    name="github_url"
                    type="url"
                    value={formData.github_url}
                    onChange={handleChange}
                    className={`input-field ${validationErrors.github_url ? '!border-rose-500/50 focus:!border-rose-500 focus:!ring-rose-500/20' : ''}`}
                    placeholder="https://github.com/org/repo"
                  />
                  {validationErrors.github_url && (
                     <p className="text-[10px] text-rose-400 font-bold uppercase tracking-widest pl-1 mt-1 animate-pop-in">{validationErrors.github_url}</p>
                  )}
                </div>
                
                <div className="space-y-4">
                  <div className="flex items-center gap-3 mb-2">
                    <div className="w-8 h-8 rounded-lg bg-slate-500/10 flex items-center justify-center text-slate-400 border border-white/5">
                      <Settings className="w-4 h-4" />
                    </div>
                    <label htmlFor="branch" className="text-sm font-black text-white uppercase tracking-widest">
                      Git Branch
                    </label>
                  </div>
                  <input
                    id="branch"
                    name="branch"
                    type="text"
                    value={formData.branch}
                    onChange={handleChange}
                    className="input-field"
                    placeholder="main"
                  />
                </div>
              </div>
              
              {/* Database Settings */}
              <div className="space-y-4">
                <div className="flex items-center gap-3 mb-2">
                  <div className="w-8 h-8 rounded-lg bg-indigo-500/10 flex items-center justify-center text-indigo-400 border border-indigo-500/20">
                    <Database className="w-4 h-4" />
                  </div>
                  <label htmlFor="database_name" className="text-sm font-black text-white uppercase tracking-widest">
                    Database Name
                  </label>
                </div>
                <input
                  id="database_name"
                  name="database_name"
                  type="text"
                  value={formData.database_name}
                  onChange={handleChange}
                  className={`input-field ${validationErrors.database_name ? '!border-rose-500/50 focus:!border-rose-500 focus:!ring-rose-500/20' : ''}`}
                  placeholder="database_name"
                  pattern="[a-z0-9_]+"
                />
                {validationErrors.database_name && (
                   <p className="text-[10px] text-rose-400 font-bold uppercase tracking-widest pl-1 mt-1 animate-pop-in">{validationErrors.database_name}</p>
                )}
              </div>

              {/* Queue Worker */}
              <div className={`p-6 rounded-2xl border transition-all duration-300 ${formData.queue_enabled ? 'bg-indigo-500/5 border-indigo-500/30' : 'bg-white/5 border-white/5 opacity-60 hover:opacity-100'}`}>
                <div className="flex items-start gap-4">
                  <div className="flex items-center h-6 mt-1">
                    <input 
                      id="queue_enabled"
                      name="queue_enabled"
                      type="checkbox"
                      checked={formData.queue_enabled}
                      onChange={handleChange}
                      className="w-5 h-5 rounded-lg border-white/20 bg-black/40 text-indigo-500 focus:ring-0 cursor-pointer"
                    />
                  </div>
                  <div>
                    <label htmlFor="queue_enabled" className="block text-sm font-black text-white uppercase tracking-widest cursor-pointer">Enable Queue Worker</label>
                    <p className="text-slate-500 text-xs mt-1 font-medium italic">Enables background processes for your application.</p>
                  </div>
                </div>
              </div>
            </div>
            
            <button
              type="submit"
              disabled={isLoading}
              className="btn btn-primary w-full py-6 text-xl"
            >
              {isLoading ? (
                <span className="flex items-center justify-center gap-3">
                  <RefreshCw className="w-6 h-6 animate-spin" />
                  Deploying...
                </span>
              ) : (
                <span className="flex items-center justify-center gap-3">
                  <Rocket className="w-6 h-6" />
                  Create Project
                  <ArrowRight className="w-6 h-6" />
                </span>
              )}
            </button>
          </div>

          <div className="space-y-6">
            <div className="card-glass p-8 border-white/10 space-y-8 bg-gradient-to-br from-white/[0.03] to-transparent">
               <div className="flex items-center gap-3 pb-6 border-b border-white/5">
                <ShieldCheck className="w-6 h-6 text-indigo-400" />
                <h3 className="text-sm font-black text-white uppercase tracking-[0.2em]">Project Setup</h3>
              </div>
              
              <ul className="space-y-6">
                <PipelineStep 
                  icon={Github} 
                  title="Git Clone" 
                  desc="Cloning your repository branch." 
                />
                <PipelineStep 
                  icon={Activity} 
                  title="PHP Version" 
                  desc="Detecting PHP and Laravel versions." 
                />
                <PipelineStep 
                  icon={Database} 
                  title="Database" 
                  desc="Creating your database instance." 
                />
                <PipelineStep 
                  icon={Zap} 
                  title="Networking" 
                  desc="Setting up secure URL and SSL." 
                />
                <PipelineStep 
                  icon={Cpu} 
                  title="Resources" 
                  desc="Allocating CPU and Memory." 
                />
              </ul>

              <div className="p-4 rounded-xl bg-white/5 border border-white/5 flex items-start gap-3">
                <Info className="w-4 h-4 text-slate-500 mt-0.5" />
                <p className="text-[10px] text-slate-500 font-bold uppercase tracking-wider leading-relaxed">
                  Your application will be reachable at your subdomain via secure HTTPS protocol.
                </p>
              </div>
            </div>
          </div>
        </form>
      </div>
    </div>
  )
}

function PipelineStep({ icon: Icon, title, desc }) {
  return (
    <li className="flex gap-4 group">
      <div className="w-10 h-10 rounded-xl bg-white/5 border border-white/5 flex items-center justify-center text-slate-400 group-hover:text-indigo-400 transition-colors shrink-0">
        <Icon className="w-5 h-5" />
      </div>
      <div>
        <h4 className="text-xs font-black text-white uppercase tracking-widest mb-1">{title}</h4>
        <p className="text-[11px] text-slate-500 font-medium leading-normal">{desc}</p>
      </div>
    </li>
  )
}

export default StudentNewProject
