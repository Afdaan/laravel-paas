// ===========================================
// Global Database Manager
// ===========================================
// Manage databases for all projects in one place
// ===========================================

import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import toast from 'react-hot-toast'
import { projectsAPI } from '../../services/api'
import DatabaseManager from './DatabaseManager'

export default function Databases() {
  const [projects, setProjects] = useState([])
  const [selectedProjectId, setSelectedProjectId] = useState(null)
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    fetchProjects()
  }, [])

  const fetchProjects = async () => {
    try {
      const response = await projectsAPI.list()
      const data = response.data.data || []
      setProjects(data)
      // Auto-select first project if available
      if (data.length > 0) {
        setSelectedProjectId(data[0].ID)
      }
    } catch (error) {
      toast.error('Failed to load projects')
    } finally {
      setIsLoading(false)
    }
  }

  const selectedProject = projects.find(p => p.ID === Number(selectedProjectId))

  if (isLoading) {
    return (
      <div className="flex flex-col items-center justify-center h-96">
        <div className="animate-spin rounded-full h-12 w-12 border-t-2 border-b-2 border-primary-500"></div>
      </div>
    )
  }

  if (projects.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-96 gap-4">
        <div className="w-16 h-16 rounded-full bg-slate-800 flex items-center justify-center text-3xl opacity-50">📭</div>
        <h3 className="text-xl font-bold text-white">No Projects Found</h3>
        <p className="text-slate-400">Create a project first to manage its database.</p>
      </div>
    )
  }

  return (
    <div className="max-w-7xl mx-auto pb-20 space-y-6">
      <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b border-slate-700 pb-6">
          <div>
            <h1 className="text-3xl font-bold text-white tracking-tight flex items-center gap-3">
              <span className="i-lucide-database text-primary-500"></span>
              Databases
            </h1>
            <p className="text-slate-400 mt-1 font-mono text-sm">
              Manage database tables, queries, and backups across all your projects.
            </p>
          </div>
          
          <div className="flex items-center gap-3 bg-slate-800 p-1.5 rounded-lg border border-slate-700">
             <span className="text-slate-400 text-sm font-medium px-2">Project:</span>
             <select 
               value={selectedProjectId || ''}
               onChange={(e) => setSelectedProjectId(Number(e.target.value))}
               className="bg-slate-900 border-none rounded text-white text-sm focus:ring-0 cursor-pointer py-1.5 pl-3 pr-8 min-w-[200px]"
             >
                {projects.map(p => (
                   <option key={p.ID} value={p.ID}>{p.name} ({p.database_name})</option>
                ))}
             </select>
          </div>
      </div>

      {selectedProjectId ? (
        <div className="animate-fade-in">
           <DatabaseManager embedded={false} projectId={selectedProjectId} />
        </div>
      ) : (
        <div className="text-center py-20 text-slate-500">
           Select a project to view its database.
        </div>
      )}
    </div>
  )
}
