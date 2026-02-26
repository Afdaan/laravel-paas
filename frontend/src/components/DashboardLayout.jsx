// ===========================================
// Dashboard Layout Component
// ===========================================
// Sidebar navigation and main content area
// ===========================================

import { Outlet, NavLink, useNavigate } from 'react-router-dom'
import useAuthStore from '../stores/authStore'

// Icons (inline SVG for simplicity)
const Icons = {
  Containers: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M21 7.5l-9-5.25L3 7.5m18 0l-9 5.25m9-5.25v9l-9 5.25M3 7.5l9 5.25M3 7.5v9l9 5.25m0-5.25v9" />
    </svg>
  ),
  Images: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M13.5 16.875h3.375m0 0h3.375m-3.375 0V13.5m0 3.375v3.375M6 10.5h2.25a2.25 2.25 0 002.25-2.25V6a2.25 2.25 0 00-2.25-2.25H6A2.25 2.25 0 003.75 6v2.25A2.25 2.25 0 006 10.5zm0 9.75h2.25A2.25 2.25 0 0010.5 18v-2.25a2.25 2.25 0 00-2.25-2.25H6a2.25 2.25 0 00-2.25 2.25V18A2.25 2.25 0 006 20.25zm9.75-9.75H18a2.25 2.25 0 002.25-2.25V6A2.25 2.25 0 0018 3.75h-2.25A2.25 2.25 0 0013.5 6v2.25a2.25 2.25 0 002.25 2.25z" />
    </svg>
  ),
  Networks: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m13.35-.622l1.757-1.757a4.5 4.5 0 00-6.364-6.364l-4.5 4.5a4.5 4.5 0 001.242 7.244" />
    </svg>
  ),
  Volumes: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M20.25 7.5l-.625 10.632a2.25 2.25 0 01-2.247 2.118H6.622a2.25 2.25 0 01-2.247-2.118L3.75 7.5M10 11.25h4M3.375 7.5h17.25c.621 0 1.125-.504 1.125-1.125v-1.5c0-.621-.504-1.125-1.125-1.125H3.375c-.621 0-1.125.504-1.125 1.125v1.5c0 .621.504 1.125 1.125 1.125z" />
    </svg>
  ),
}


function DashboardLayout({ isAdmin = false }) {
  const { user, logout } = useAuthStore()
  const navigate = useNavigate()
  
  const handleLogout = async () => {
    await logout()
    navigate('/login')
  }
  
  // Navigation items based on role
  const navItems = isAdmin
    ? {
        management: [
          { to: '/admin/dashboard', icon: Icons.Dashboard, label: 'Dashboard' },
          { to: '/admin/users', icon: Icons.Users, label: 'Users' },
          { to: '/admin/projects', icon: Icons.Projects, label: 'Projects' },
          { to: '/admin/settings', icon: Icons.Settings, label: 'Settings' },
        ],
        resources: [
          { to: '/admin/containers', icon: Icons.Containers, label: 'Containers' },
          { to: '/admin/images', icon: Icons.Images, label: 'Images' },
          { to: '/admin/networks', icon: Icons.Networks, label: 'Networks' },
          { to: '/admin/volumes', icon: Icons.Volumes, label: 'Volumes' },
        ]
      }
    : {
        management: [
          { to: '/dashboard', icon: Icons.Dashboard, label: 'Dashboard' },
          { to: '/projects', icon: Icons.Projects, label: 'My Projects' },
          { to: '/databases', icon: Icons.Database, label: 'Databases' },
        ]
      }
  
  return (
    <div className="flex min-h-screen bg-[#0a0a0c]">
      {/* Sidebar */}
      <aside className="w-64 bg-[#0f0f12] border-r border-white/5 flex flex-col shadow-2xl z-50">
        {/* Logo */}
        <div className="p-8 pb-4">
          <h1 className="text-2xl font-black text-white flex items-center gap-3 tracking-tighter">
            <span className="w-10 h-10 bg-gradient-to-br from-purple-600 to-indigo-600 rounded-xl flex items-center justify-center text-lg shadow-lg shadow-purple-500/20">
              LP
            </span>
            <span className="uppercase tracking-[0.2em] text-sm font-bold opacity-80">PaaS</span>
          </h1>
          {isAdmin && (
            <div className="flex items-center gap-2 mt-4 ml-1">
                <div className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse"></div>
                <span className="text-[10px] text-slate-500 uppercase font-black tracking-widest">
                    Local Docker
                </span>
            </div>
          )}
        </div>
        
        {/* Navigation */}
        <nav className="flex-1 p-6 space-y-8 overflow-y-auto">
          {/* Management Section */}
          <div className="space-y-2">
            <p className="text-[10px] font-bold text-slate-700 uppercase tracking-[0.2em] mb-4 ml-1">Management</p>
            {navItems.management.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                className={({ isActive }) =>
                  `flex items-center gap-3 px-4 py-3 rounded-xl transition-all duration-300 group ${
                    isActive
                      ? 'bg-gradient-to-r from-purple-600/10 to-transparent text-purple-400 border-l-2 border-purple-500'
                      : 'text-slate-500 hover:text-slate-300 hover:bg-white/[0.02]'
                  }`
                }
              >
                <span className="transition-transform group-hover:scale-110">
                  <item.icon />
                </span>
                <span className="text-sm font-semibold tracking-wide">{item.label}</span>
              </NavLink>
            ))}
          </div>

          {/* Resources Section (Admin only) */}
          {isAdmin && navItems.resources && (
            <div className="space-y-2">
                <p className="text-[10px] font-bold text-slate-700 uppercase tracking-[0.2em] mb-4 ml-1">Resources</p>
                {navItems.resources.map((item) => (
                <NavLink
                    key={item.to}
                    to={item.to}
                    className={({ isActive }) =>
                    `flex items-center gap-3 px-4 py-3 rounded-xl transition-all duration-300 group ${
                        isActive
                        ? 'bg-gradient-to-r from-purple-600/10 to-transparent text-purple-400 border-l-2 border-purple-500'
                        : 'text-slate-500 hover:text-slate-300 hover:bg-white/[0.02]'
                    }`
                    }
                >
                    <span className="transition-transform group-hover:scale-110">
                    <item.icon />
                    </span>
                    <span className="text-sm font-semibold tracking-wide">{item.label}</span>
                </NavLink>
                ))}
            </div>
          )}
          
          {/* New Project Button (Students only) */}
          {!isAdmin && (
            <NavLink
              to="/projects/new"
              className="flex items-center justify-center gap-3 px-4 py-3 rounded-xl bg-gradient-to-br from-emerald-600 to-teal-700 text-white font-bold text-sm shadow-lg shadow-emerald-500/10 hover:brightness-110 transition-all mt-8"
            >
              <Icons.Plus />
              NEW PROJECT
            </NavLink>
          )}
        </nav>
        
        {/* User Info & Logout */}
        <div className="p-6 border-t border-white/5 bg-[#0a0a0c]/50">
          <div className="flex items-center gap-3 mb-6 p-2 rounded-xl bg-white/[0.02] border border-white/5">
            <div className="w-10 h-10 bg-gradient-to-br from-slate-700 to-slate-800 rounded-full flex items-center justify-center text-white font-bold ring-2 ring-white/5">
              {user?.name?.charAt(0)?.toUpperCase() || 'U'}
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-xs font-bold text-white truncate uppercase tracking-tight">{user?.name}</p>
              <p className="text-[10px] text-slate-500 truncate">{user?.email}</p>
            </div>
          </div>
          
          <div className="space-y-1">
            {/* Switch to Admin (if admin viewing student dashboard) */}
            {!isAdmin && (user?.role === 'superadmin' || user?.role === 'admin') && (
              <NavLink
                to="/admin"
                className="flex items-center gap-3 px-4 py-2 rounded-lg text-slate-500 hover:text-slate-300 transition-colors text-xs font-bold uppercase tracking-widest"
              >
                <Icons.Settings />
                Admin Panel
              </NavLink>
            )}
            
            {/* Switch to Student (if admin) */}
            {isAdmin && (
              <NavLink
                to="/dashboard"
                className="flex items-center gap-3 px-4 py-2 rounded-lg text-slate-500 hover:text-slate-300 transition-colors text-xs font-bold uppercase tracking-widest"
              >
                <Icons.Dashboard />
                Student View
              </NavLink>
            )}
            
            <button
              onClick={handleLogout}
              className="flex items-center gap-3 px-4 py-2 rounded-lg text-slate-600 hover:text-red-400 transition-all w-full text-xs font-bold uppercase tracking-widest group"
            >
              <span className="group-hover:translate-x-1 transition-transform">
                <Icons.Logout />
              </span>
              Logout
            </button>
          </div>
        </div>
      </aside>
      
      {/* Main Content */}
      <main className="flex-1 p-8 overflow-auto">
        <Outlet />
      </main>
    </div>
  )
}

export default DashboardLayout
