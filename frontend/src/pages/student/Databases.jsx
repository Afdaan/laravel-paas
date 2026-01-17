// ===========================================
// Global Database Manager
// ===========================================
// Manage databases for all projects in one place
// ===========================================

import { useState, useEffect } from 'react'
import toast from 'react-hot-toast'
import { projectsAPI } from '../../services/api'
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
      // Auto-select first project if available
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

  // Render Loading State
  if (isLoading) {
    return (
      <div className="flex flex-col items-center justify-center h-[calc(100vh-100px)]">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-primary-500"></div>
      </div>
    )
  }

  // Render Empty State
  if (projects.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-[calc(100vh-100px)] gap-4">
        <div className="w-20 h-20 rounded-full bg-slate-800 flex items-center justify-center text-4xl opacity-50">📭</div>
        <div className="text-center">
          <h3 className="text-xl font-bold text-white">No Projects Found</h3>
          <p className="text-slate-400 mt-2">Create a project first to manage its database.</p>
        </div>
      </div>
    )
  }

  return (
    <div className="h-[calc(100vh-100px)] flex flex-col md:flex-row gap-6 pb-6">
      
      {/* Sidebar - Project List */}
      <div className="w-full md:w-80 flex-shrink-0 flex flex-col gap-4">
         <div className="bg-slate-800 p-4 rounded-xl border border-slate-700 shadow-sm">
            <h2 className="text-lg font-bold text-white mb-1 flex items-center gap-2">
              <span className="i-lucide-database text-primary-500"></span> Databases
            </h2>
            <p className="text-slate-400 text-xs mb-4">Select a project to manage</p>
            
            <div className="relative">
              <input 
                type="text" 
                placeholder="Search projects..." 
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="w-full bg-slate-900 border border-slate-700 rounded-lg py-2 pl-9 pr-3 text-sm text-white focus:outline-none focus:border-primary-500"
              />
              <span className="absolute left-3 top-2.5 text-slate-500 i-lucide-search text-xs">🔍</span>
            </div>
         </div>

         <div className="flex-1 overflow-y-auto bg-slate-800 rounded-xl border border-slate-700 shadow-sm p-2 space-y-1">
            {filteredProjects.length > 0 ? (
              filteredProjects.map(p => (
                <button
                  key={p.id}
                  onClick={() => setSelectedProjectId(p.id)}
                  className={`w-full text-left p-3 rounded-lg transition-all border ${
                    selectedProjectId === p.id 
                    ? 'bg-primary-600/10 border-primary-600/50' 
                    : 'border-transparent hover:bg-slate-700/50'
                  }`}
                >
                  <div className="flex items-center justify-between mb-1">
                    <span className={`font-semibold text-sm ${selectedProjectId === p.id ? 'text-primary-400' : 'text-slate-200'}`}>
                      {p.name}
                    </span>
                    <span className={`w-2 h-2 rounded-full ${p.status === 'running' ? 'bg-emerald-500' : 'bg-slate-500'}`} />
                  </div>
                  <div className="flex items-center gap-1 text-xs text-slate-500 font-mono truncate">
                    <span className="i-lucide-database w-3 h-3"></span> {p.database_name}
                  </div>
                </button>
              ))
            ) : (
              <div className="text-center py-10 text-slate-500 text-sm">
                No matching projects
              </div>
            )}
         </div>
      </div>

      {/* Main Content - Database Manager */}
      <div className="flex-1 bg-slate-800 rounded-xl border border-slate-700 shadow-sm overflow-hidden flex flex-col">
        {selectedProject ? (
          <div className="flex-1 overflow-auto p-1">
             <DatabaseManager embedded={true} projectId={selectedProjectId} />
          </div>
        ) : (
          <div className="h-full flex flex-col items-center justify-center text-slate-500 gap-4 opacity-75">
             <div className="w-16 h-16 rounded-full bg-slate-700/50 flex items-center justify-center text-3xl">👈</div>
             <p>Select a project from the sidebar</p>
          </div>
        )}
      </div>

    </div>
  )
}
