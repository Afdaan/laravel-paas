// ===========================================
// Admin Dashboard Page
// ===========================================

import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { projectsAPI, settingsAPI } from '../../services/api'

function AdminDashboard() {
  const [stats, setStats] = useState({
    total_projects: 0,
    running_projects: 0,
    total_students: 0,
  })
  const [settings, setSettings] = useState({})
  const [isLoading, setIsLoading] = useState(true)
  
  useEffect(() => {
    fetchData()
  }, [])
  
  const fetchData = async () => {
    try {
      const [statsRes, settingsRes] = await Promise.all([
        projectsAPI.adminStats(),
        settingsAPI.list(),
      ])
      setStats(statsRes.data)
      setSettings(settingsRes.data.map || {})
    } catch (error) {
      console.error('Failed to fetch data:', error)
    } finally {
      setIsLoading(false)
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
    <div className="space-y-8">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold text-white">Admin Dashboard</h1>
        <p className="text-slate-400 mt-1">
          Overview of the Laravel PaaS platform
        </p>
      </div>
      
      {/* Stats Cards */}
      <div className="grid grid-cols-1 md:grid-cols-4 gap-6">
        <div className="card p-6">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-slate-400 text-sm">Total Students</p>
              <p className="text-3xl font-bold text-white mt-1">{stats.total_students}</p>
            </div>
            <div className="w-12 h-12 bg-blue-600/20 rounded-xl flex items-center justify-center">
              <span className="text-2xl">👨‍🎓</span>
            </div>
          </div>
        </div>
        
        <div className="card p-6">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-slate-400 text-sm">Total Projects</p>
              <p className="text-3xl font-bold text-white mt-1">{stats.total_projects}</p>
            </div>
            <div className="w-12 h-12 bg-purple-600/20 rounded-xl flex items-center justify-center">
              <span className="text-2xl">📦</span>
            </div>
          </div>
        </div>
        
        <div className="card p-6">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-slate-400 text-sm">Running</p>
              <p className="text-3xl font-bold text-emerald-400 mt-1">{stats.running_projects}</p>
            </div>
            <div className="w-12 h-12 bg-emerald-600/20 rounded-xl flex items-center justify-center">
              <span className="text-2xl">✅</span>
            </div>
          </div>
        </div>
        
        <div className="card p-6">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-slate-400 text-sm">Base Domain</p>
              <p className="text-lg font-bold text-primary-400 mt-1 truncate">
                {settings.base_domain || 'Not set'}
              </p>
            </div>
            <div className="w-12 h-12 bg-primary-600/20 rounded-xl flex items-center justify-center">
              <span className="text-2xl">🌐</span>
            </div>
          </div>
        </div>
      </div>
      
      {/* Quick Actions */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <Link to="/admin/users" className="card p-6 hover:border-primary-500 transition-colors group">
          <div className="flex items-center gap-4">
            <div className="w-12 h-12 bg-blue-600/20 rounded-xl flex items-center justify-center group-hover:bg-blue-600/30 transition-colors">
              <span className="text-2xl">👥</span>
            </div>
            <div>
              <h3 className="text-white font-semibold">Manage Users</h3>
              <p className="text-slate-400 text-sm">Add students & import from Excel</p>
            </div>
          </div>
        </Link>
        
        <Link to="/admin/projects" className="card p-6 hover:border-primary-500 transition-colors group">
          <div className="flex items-center gap-4">
            <div className="w-12 h-12 bg-purple-600/20 rounded-xl flex items-center justify-center group-hover:bg-purple-600/30 transition-colors">
              <span className="text-2xl">📁</span>
            </div>
            <div>
              <h3 className="text-white font-semibold">All Projects</h3>
              <p className="text-slate-400 text-sm">View & manage all deployments</p>
            </div>
          </div>
        </Link>
        
        <Link to="/admin/settings" className="card p-6 hover:border-primary-500 transition-colors group">
          <div className="flex items-center gap-4">
            <div className="w-12 h-12 bg-emerald-600/20 rounded-xl flex items-center justify-center group-hover:bg-emerald-600/30 transition-colors">
              <span className="text-2xl">⚙️</span>
            </div>
            <div>
              <h3 className="text-white font-semibold">Settings</h3>
              <p className="text-slate-400 text-sm">Configure limits & domain</p>
            </div>
          </div>
        </Link>
      </div>
      
      {/* Current Settings Overview */}
      <div className="card">
        <div className="p-6 border-b border-slate-700">
          <h2 className="text-xl font-semibold text-white">Current Configuration</h2>
        </div>
        <div className="p-6 grid grid-cols-2 md:grid-cols-4 gap-4">
          <div>
            <p className="text-slate-400 text-sm">Max Projects/User</p>
            <p className="text-white font-semibold">{settings.max_projects_per_user || '3'}</p>
          </div>
          <div>
            <p className="text-slate-400 text-sm">Project Expiry</p>
            <p className="text-white font-semibold">{settings.project_expiry_days || '30'} days</p>
          </div>
          <div>
            <p className="text-slate-400 text-sm">CPU Limit</p>
            <p className="text-white font-semibold">{settings.cpu_limit_percent || '50'}%</p>
          </div>
          <div>
            <p className="text-slate-400 text-sm">Memory Limit</p>
            <p className="text-white font-semibold">{settings.memory_limit_mb || '512'} MB</p>
          </div>
        </div>
      </div>
    </div>
  )
}

export default AdminDashboard
