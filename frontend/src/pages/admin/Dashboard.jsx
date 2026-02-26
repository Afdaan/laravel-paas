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
            <div className="w-12 h-12 bg-blue-600/20 rounded-xl flex items-center justify-center text-blue-400">
              <svg className="w-6 h-6" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z" />
              </svg>
            </div>
          </div>
        </div>
        
        <div className="card p-6">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-slate-400 text-sm">Total Projects</p>
              <p className="text-3xl font-bold text-white mt-1">{stats.total_projects}</p>
            </div>
            <div className="w-12 h-12 bg-purple-600/20 rounded-xl flex items-center justify-center text-purple-400">
              <svg className="w-6 h-6" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M20.25 7.5L12 12L3.75 7.5M12 12V21m-6.75-13.5L12 3l6.75 4.5M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-19.5 0A2.25 2.25 0 004.5 15h15a2.25 2.25 0 002.25-2.25" />
              </svg>
            </div>
          </div>
        </div>
        
        <div className="card p-6">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-slate-400 text-sm">Running</p>
              <p className="text-3xl font-bold text-emerald-400 mt-1">{stats.running_projects}</p>
            </div>
            <div className="w-12 h-12 bg-emerald-600/20 rounded-xl flex items-center justify-center text-emerald-400">
              <svg className="w-6 h-6" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M9 12.75L11.25 15 15 9.75M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
              </svg>
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
            <div className="w-12 h-12 bg-primary-600/20 rounded-xl flex items-center justify-center text-primary-400">
              <svg className="w-6 h-6" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 21a9.041 9.041 0 01-2.427-.328 12.035 12.035 0 01-5.264-3.551 12.035 12.035 0 01-3.551-5.264A9.041 9.041 0 011 9.573a9.041 9.041 0 01.328-2.427 12.035 12.035 0 013.551-5.264 12.035 12.035 0 015.264-3.551A9.041 9.041 0 0112 1a9.041 9.041 0 012.427.328 12.035 12.035 0 015.264 3.551 12.035 12.035 0 013.551 5.264 9.041 9.041 0 01.328 2.427 9.041 9.041 0 01-.328 2.427 12.035 12.035 0 01-3.551 5.264 12.035 12.035 0 01-5.264 3.551A9.041 9.041 0 0112 21z" />
                <path strokeLinecap="round" strokeLinejoin="round" d="M12 1a9 9 0 00-9 9m9-9a9 9 0 019 9m-9-9v18m0-18C8.5 1 5.5 4 5.5 10s3 9 6.5 9m0-18c3.5 0 6.5 3 6.5 9s-3 9-6.5 9M1.2 12.5h21.6M2.5 7h19M2.5 17h19" />
              </svg>
            </div>
          </div>
        </div>
      </div>
      
      {/* Quick Actions */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <Link to="/admin/users" className="card p-6 hover:border-primary-500 transition-colors group">
          <div className="flex items-center gap-4">
            <div className="w-12 h-12 bg-blue-600/20 rounded-xl flex items-center justify-center group-hover:bg-blue-600/30 transition-colors text-blue-400">
              <svg className="w-6 h-6" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M18 18.72a9.094 9.094 0 003.741-.479 3 3 0 00-4.682-2.72m.94 3.198l.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0112 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 016 18.719m12 0a5.971 5.971 0 00-.941-3.197m0 0A5.995 5.995 0 0012 12.75a5.995 5.995 0 00-5.058 2.772m0 0a3 3 0 00-4.681 2.72 8.986 8.986 0 003.74.477m.94-3.197a5.971 5.971 0 00-.94 3.197M15 6.75a3 3 0 11-6 0 3 3 0 016 0zm6 2.25a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0zm-13.5 0a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0z" />
              </svg>
            </div>
            <div>
              <h3 className="text-white font-semibold">Manage Users</h3>
              <p className="text-slate-400 text-sm">Add students & import from Excel</p>
            </div>
          </div>
        </Link>
        
        <Link to="/admin/projects" className="card p-6 hover:border-primary-500 transition-colors group">
          <div className="flex items-center gap-4">
            <div className="w-12 h-12 bg-purple-600/20 rounded-xl flex items-center justify-center group-hover:bg-purple-600/30 transition-colors text-purple-400">
              <svg className="w-6 h-6" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M2.25 13.5h3.86a2.25 2.25 0 012.012 1.244l.256.512a2.25 2.25 0 002.013 1.244h3.218a2.25 2.25 0 002.013-1.244l.256-.512a2.25 2.25 0 012.013-1.244h3.859m-19.5.375a3 3 0 003 3h15a3 3 0 003-3m-18-6h15a3 3 0 013 3v.375m-18-3.375A3 3 0 003 10.875v.375m18-3.375V6a3 3 0 00-3-3H6a3 3 0 00-3 3v.375m3 0V6" />
              </svg>
            </div>
            <div>
              <h3 className="text-white font-semibold">All Projects</h3>
              <p className="text-slate-400 text-sm">View & manage all deployments</p>
            </div>
          </div>
        </Link>
        
        <Link to="/admin/settings" className="card p-6 hover:border-primary-500 transition-colors group">
          <div className="flex items-center gap-4">
            <div className="w-12 h-12 bg-emerald-600/20 rounded-xl flex items-center justify-center group-hover:bg-emerald-600/30 transition-colors text-emerald-400">
              <svg className="w-6 h-6" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z" />
                <path strokeLinecap="round" strokeLinejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
              </svg>
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
