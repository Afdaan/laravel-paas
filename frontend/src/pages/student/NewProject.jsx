// ===========================================
// New Project Page
// ===========================================

import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import toast from 'react-hot-toast'
import { projectsAPI } from '../../services/api'

function StudentNewProject() {
  const navigate = useNavigate()
  const [isLoading, setIsLoading] = useState(false)
  const [formData, setFormData] = useState({
    name: '',
    github_url: '',
    branch: '',
    branch: '',
    database_name: '',
    queue_enabled: false,
  })
  
  const handleChange = (e) => {
    const { name, value, type, checked } = e.target
    setFormData(prev => ({ 
        ...prev, 
        [name]: type === 'checkbox' ? checked : value 
    }))
    
    // Auto-generate database name from project name
    if (name === 'name') {
      const dbName = value.toLowerCase()
        .replace(/[^a-z0-9]+/g, '_')
        .replace(/^_|_$/g, '')
      setFormData(prev => ({ ...prev, database_name: dbName }))
    }
  }
  
  const handleSubmit = async (e) => {
    e.preventDefault()
    setIsLoading(true)
    
    try {
      const response = await projectsAPI.create(formData)
      toast.success('Project deployment started!')
      navigate(`/projects/${response.data.project.id}`)
    } catch (error) {
      toast.error(error.response?.data?.error || 'Failed to create project')
    } finally {
      setIsLoading(false)
    }
  }
  
  return (
    <div className="max-w-2xl mx-auto">
      {/* Header */}
      <div className="mb-8">
        <h1 className="text-2xl font-bold text-white">Deploy New Project</h1>
        <p className="text-slate-400 mt-1">
          Enter your GitHub repository URL to deploy a Laravel application
        </p>
      </div>
      
      {/* Form */}
      <form onSubmit={handleSubmit} className="card p-8 space-y-6">
        {/* Project Name */}
        <div>
          <label htmlFor="name" className="block text-sm font-medium text-slate-300 mb-2">
            Project Name
          </label>
          <input
            id="name"
            name="name"
            type="text"
            value={formData.name}
            onChange={handleChange}
            className="w-full px-4 py-3 border"
            placeholder="My Laravel App"
            required
          />
          <p className="text-sm text-slate-500 mt-1">
            A friendly name for your project
          </p>
        </div>
        
        {/* GitHub URL */}
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          <div className="md:col-span-2">
            <label htmlFor="github_url" className="block text-sm font-medium text-slate-300 mb-2">
              GitHub Repository URL
            </label>
            <input
              id="github_url"
              name="github_url"
              type="url"
              value={formData.github_url}
              onChange={handleChange}
              className="w-full px-4 py-3 border"
              placeholder="https://github.com/username/repository"
              required
            />
            <p className="text-sm text-slate-500 mt-1">
               Public repository containing Laravel project
            </p>
          </div>
          
          <div>
            <label htmlFor="branch" className="block text-sm font-medium text-slate-300 mb-2">
              Branch
            </label>
            <input
              id="branch"
              name="branch"
              type="text"
              value={formData.branch}
              onChange={handleChange}
              className="w-full px-4 py-3 border"
              placeholder="main"
            />
             <p className="text-sm text-slate-500 mt-1">
               Default: main
            </p>
          </div>
        </div>
        
        {/* Database Name */}
        <div>
          <label htmlFor="database_name" className="block text-sm font-medium text-slate-300 mb-2">
            Database Name
          </label>
          <input
            id="database_name"
            name="database_name"
            type="text"
            value={formData.database_name}
            onChange={handleChange}
            className="w-full px-4 py-3 border"
            placeholder="my_laravel_app"
            pattern="[a-z0-9_]+"
            required
          />
          <p className="text-sm text-slate-500 mt-1">
            Lowercase letters, numbers, and underscores only
          </p>
        </div>

        {/* Queue Worker Checkbox */}
        <div className="flex items-start gap-3 p-4 bg-slate-800 rounded-lg border border-slate-700">
           <div className="flex items-center h-5">
              <input 
                 id="queue_enabled"
                 name="queue_enabled"
                 type="checkbox"
                 checked={formData.queue_enabled}
                 onChange={handleChange}
                 className="w-4 h-4 rounded border-slate-600 bg-slate-700 text-primary-500 focus:ring-primary-500 focus:ring-offset-slate-800"
              />
           </div>
           <div>
              <label htmlFor="queue_enabled" className="block text-sm font-medium text-white">Enable Queue Worker (Optional)</label>
              <p className="text-sm text-slate-400">
                 Automatically run <code>php artisan queue:work</code> using <strong>database</strong> driver. 
                 <span className="block text-amber-500 text-xs mt-1">Make sure to run <code>php artisan queue:table && php artisan migrate</code> in your project.</span>
              </p>
           </div>
        </div>
        
        {/* Info Box */}
        <div className="bg-primary-600/10 border border-primary-600/30 rounded-lg p-4">
          <h3 className="text-primary-400 font-medium mb-2">What happens next?</h3>
          <ul className="text-sm text-slate-400 space-y-1">
            <li>• Your repository will be cloned</li>
            <li>• Laravel version will be auto-detected</li>
            <li>• A database will be created and configured</li>
            <li>• Your app will be built and deployed with HTTPS</li>
          </ul>
        </div>
        
        {/* Submit */}
        <div className="flex gap-4">
          <button
            type="button"
            onClick={() => navigate(-1)}
            className="btn btn-secondary"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={isLoading}
            className="btn btn-primary flex-1 disabled:opacity-50"
          >
            {isLoading ? (
              <span className="flex items-center justify-center gap-2">
                <svg className="animate-spin h-5 w-5" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" fill="none" />
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                </svg>
                Deploying...
              </span>
            ) : (
              '🚀 Deploy Project'
            )}
          </button>
        </div>
      </form>
    </div>
  )
}

export default StudentNewProject
