// ===========================================
// Global Database Manager
// ===========================================

import { useState, useEffect } from 'react'
import toast from 'react-hot-toast'
import { projectsAPI } from '../../services/api'
import { 
  PackageOpen, 
  Database as DbIcon, 
  Search, 
  ArrowRight,
  Layout,
  Terminal,
  Activity
} from 'lucide-react'
import DatabaseManager from './DatabaseManager'

export default function Databases() {
  const [projects, setProjects] = useState([])
  const [selectedProjectId, setSelectedProjectId] = useState(null)
  const [isLoading, setIsLoading] = useState(true)
  const [search, setSearch] = useState('')

  useEffect(() => {
    fetchProjects()
  }, [])

  const fetchProjects = async () => {
    try {
      const response = await projectsAPI.listOwn()
      const data = response.data.data || []
      setProjects(data)
      if (data.length > 0) {
        setSelectedProjectId(data[0].id)
      }
    } catch (error) {
      toast.error('Failed to load projects')
    } finally {
      setIsLoading(false)
    }
  }

  const filteredProjects = projects.filter(p => 
    p.name.toLowerCase().includes(search.toLowerCase()) || 
    p.database_name.toLowerCase().includes(search.toLowerCase())
  )

  const selectedProject = projects.find(p => p.id === Number(selectedProjectId))

  if (isLoading) {
    return (
      <div className="min-h-[60vh] flex flex-col items-center justify-center">
        <div className="w-10 h-10 border-4 border-indigo-500/20 border-t-indigo-500 rounded-full animate-spin mb-4"></div>
        <p className="text-slate-500 font-bold uppercase tracking-widest text-[10px] animate-pulse">Accessing Data Cluster...</p>
      </div>
    )
  }

  if (projects.length === 0) {
    return (
      <div className="min-h-[60vh] flex flex-col items-center justify-center gap-6 animate-pop-in">
        <div className="w-20 h-20 rounded-[2.5rem] bg-white/5 border border-white/5 flex items-center justify-center">
          <PackageOpen className="w-10 h-10 text-slate-700" />
        </div>
        <div className="text-center max-w-sm">
          <h3 className="text-2xl font-black text-white tracking-tight">No Clusters Found</h3>
          <p className="text-slate-500 mt-2 font-medium">Provision a project workload first to initialize its corresponding database instance.</p>
        </div>
      </div>
    )
  }

  return (
    <div className="h-[calc(100vh-140px)] flex flex-col md:flex-row gap-8 animate-pop-in">
      
      {/* Sidebar - Project List */}
      <div className="w-full md:w-80 flex-shrink-0 flex flex-col gap-6">
         <div className="card-glass p-6 border-white/10">
            <div className="flex items-center gap-3 mb-6">
              <div className="w-10 h-10 rounded-xl bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400">
                <DbIcon className="w-5 h-5" />
              </div>
              <div>
                <h2 className="text-lg font-black text-white tracking-tight uppercase">Clusters</h2>
                <p className="text-slate-500 text-[10px] font-black uppercase tracking-widest">Global Fleet</p>
              </div>
            </div>
            
            <div className="relative group">
              <input 
                type="text" 
                placeholder="Search workloads..." 
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="input-field py-3 pl-10 text-xs font-bold uppercase tracking-widest"
              />
              <Search className="absolute left-3.5 top-3.5 w-4 h-4 text-slate-500 group-focus-within:text-indigo-400 transition-colors" />
            </div>
         </div>

         <div className="flex-1 overflow-y-auto card-glass p-2 space-y-2 border-white/5">
            {filteredProjects.length > 0 ? (
              filteredProjects.map(p => (
                <button
                  key={p.id}
                  onClick={() => setSelectedProjectId(p.id)}
                  className={`w-full text-left p-4 rounded-2xl transition-all duration-300 border group ${
                    selectedProjectId === p.id 
                    ? 'bg-indigo-500/10 border-indigo-500/30' 
                    : 'border-transparent hover:bg-white/[0.03] hover:border-white/10'
                  }`}
                >
                  <div className="flex items-center justify-between mb-2">
                    <span className={`font-black text-xs uppercase tracking-tight ${selectedProjectId === p.id ? 'text-indigo-400' : 'text-slate-300'}`}>
                      {p.name}
                    </span>
                    <div className={`w-1.5 h-1.5 rounded-full ${p.status === 'running' ? 'bg-emerald-500 animate-pulse' : 'bg-slate-700'}`} />
                  </div>
                  <div className="flex items-center gap-2 text-[10px] text-slate-500 font-mono italic truncate">
                    <Terminal className="w-3 h-3 group-hover:text-indigo-400 transition-colors" />
                    {p.database_name}
                  </div>
                </button>
              ))
            ) : (
              <div className="text-center py-12 text-slate-600 font-bold uppercase tracking-widest text-[10px]">
                No clusters matching filter
              </div>
            )}
         </div>
      </div>

      {/* Main Content - Database Manager */}
      <div className="flex-1 card-glass border-white/10 overflow-hidden flex flex-col bg-white/[0.01]">
        {selectedProject ? (
          <div className="flex-1 overflow-auto">
             <DatabaseManager embedded={true} projectId={selectedProjectId} />
          </div>
        ) : (
          <div className="h-full flex flex-col items-center justify-center text-slate-600 gap-6 opacity-50 animate-pulse">
             <div className="w-20 h-20 rounded-[2.5rem] bg-white/[0.02] border border-white/5 flex items-center justify-center">
               <ArrowRight className="w-8 h-8 -rotate-90 md:rotate-0" />
             </div>
             <p className="text-xs font-black uppercase tracking-[0.3em]">Awaiting Selection</p>
          </div>
        )}
      </div>

    </div>
  )
}

