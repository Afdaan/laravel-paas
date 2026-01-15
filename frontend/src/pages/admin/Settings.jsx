// ===========================================
// Admin Settings Page
// ===========================================

import { useState, useEffect } from 'react'
import toast from 'react-hot-toast'
import { settingsAPI } from '../../services/api'

function AdminSettings() {
  const [settings, setSettings] = useState({})
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  
  useEffect(() => {
    fetchSettings()
  }, [])
  
  const fetchSettings = async () => {
    try {
      const response = await settingsAPI.list()
      setSettings(response.data.map || {})
    } catch (error) {
      toast.error('Failed to load settings')
    } finally {
      setIsLoading(false)
    }
  }
  
  const handleChange = (key, value) => {
    setSettings(prev => ({ ...prev, [key]: value }))
  }
  
  const handleSave = async () => {
    setIsSaving(true)
    try {
      await settingsAPI.update(settings)
      toast.success('Settings saved successfully')
    } catch (error) {
      toast.error('Failed to save settings')
    } finally {
      setIsSaving(false)
    }
  }
  
  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="animate-spin rounded-full h-8 w-8 border-t-2 border-b-2 border-primary-500"></div>
      </div>
    )
  }
  
  return (
    <div className="space-y-6 max-w-2xl">
      {/* Header */}
      <div>
        <h1 className="text-2xl font-bold text-white">Platform Settings</h1>
        <p className="text-slate-400">Configure resource limits and system behavior</p>
      </div>
      
      {/* Domain Settings */}
      <div className="card p-6">
        <h2 className="text-lg font-semibold text-white mb-4">Domain Configuration</h2>
        <div className="space-y-4">
          <div>
            <label className="block text-sm text-slate-300 mb-1">Base Domain</label>
            <input
              type="text"
              value={settings.base_domain || ''}
              onChange={(e) => handleChange('base_domain', e.target.value)}
              className="w-full px-4 py-2 border"
              placeholder="example.com"
            />
            <p className="text-sm text-slate-500 mt-1">
              Student projects will use subdomains: project-name.{settings.base_domain || 'example.com'}
            </p>
          </div>
        </div>
      </div>
      
      {/* Project Limits */}
      <div className="card p-6">
        <h2 className="text-lg font-semibold text-white mb-4">Project Limits</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm text-slate-300 mb-1">Max Projects per Student</label>
            <input
              type="number"
              min="1"
              max="10"
              value={settings.max_projects_per_user || 3}
              onChange={(e) => handleChange('max_projects_per_user', e.target.value)}
              className="w-full px-4 py-2 border"
            />
          </div>
          <div>
            <label className="block text-sm text-slate-300 mb-1">Project Expiry (days)</label>
            <input
              type="number"
              min="0"
              value={settings.project_expiry_days || 30}
              onChange={(e) => handleChange('project_expiry_days', e.target.value)}
              className="w-full px-4 py-2 border"
            />
            <p className="text-sm text-slate-500 mt-1">Set to 0 for no expiry</p>
          </div>
        </div>
      </div>
      
      {/* Resource Limits */}
      <div className="card p-6">
        <h2 className="text-lg font-semibold text-white mb-4">Resource Limits (per container)</h2>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm text-slate-300 mb-1">CPU Limit (%)</label>
            <input
              type="number"
              min="10"
              max="100"
              value={settings.cpu_limit_percent || 50}
              onChange={(e) => handleChange('cpu_limit_percent', e.target.value)}
              className="w-full px-4 py-2 border"
            />
            <p className="text-sm text-slate-500 mt-1">Percentage of CPU cores</p>
          </div>
          <div>
            <label className="block text-sm text-slate-300 mb-1">Memory Limit (MB)</label>
            <input
              type="number"
              min="128"
              max="2048"
              step="128"
              value={settings.memory_limit_mb || 512}
              onChange={(e) => handleChange('memory_limit_mb', e.target.value)}
              className="w-full px-4 py-2 border"
            />
            <p className="text-sm text-slate-500 mt-1">RAM limit per container</p>
          </div>
        </div>
      </div>
      
      {/* Save Button */}
      <div className="flex justify-end">
        <button
          onClick={handleSave}
          disabled={isSaving}
          className="btn btn-primary disabled:opacity-50"
        >
          {isSaving ? 'Saving...' : 'Save Settings'}
        </button>
      </div>
    </div>
  )
}

export default AdminSettings
