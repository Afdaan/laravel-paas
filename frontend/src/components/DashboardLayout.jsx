// ===========================================
// Dashboard Layout Component
// ===========================================
// Sidebar navigation and main content area
// ===========================================

import { Outlet, NavLink, useNavigate } from 'react-router-dom'
import useAuthStore from '../stores/authStore'

// Icons (inline SVG for simplicity)
const Icons = {
  Dashboard: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25a2.25 2.25 0 01-2.25 2.25h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25h-2.25a2.25 2.25 0 01-2.25-2.25v-2.25z" />
    </svg>
  ),
  Projects: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M2.25 12.75V12A2.25 2.25 0 014.5 9.75h15A2.25 2.25 0 0121.75 12v.75m-19.5 0A2.25 2.25 0 004.5 15h15a2.25 2.25 0 002.25-2.25m-19.5 0v.25A2.25 2.25 0 004.5 17.5h15a2.25 2.25 0 002.25-2.25v-.25m-19.5 0V12a2.25 2.25 0 012.25-2.25h15A2.25 2.25 0 0121.75 12v.75m-19.5 0A2.25 2.25 0 004.5 15h15a2.25 2.25 0 002.25-2.25" />
      <path strokeLinecap="round" strokeLinejoin="round" d="M20.25 7.5L12 12L3.75 7.5M12 12V21m-6.75-13.5L12 3l6.75 4.5" />
    </svg>
  ),
  Users: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M15 19.128a9.38 9.38 0 002.625.372 9.337 9.337 0 004.121-.952 4.125 4.125 0 00-7.533-2.493M15 19.128v-.003c0-1.113-.285-2.16-.786-3.07M15 19.128v.106A12.318 12.318 0 018.624 21c-2.331 0-4.512-.645-6.374-1.766l-.001-.109a6.375 6.375 0 0111.964-3.07M12 6.375a3.375 3.375 0 11-6.75 0 3.375 3.375 0 016.75 0zm8.25 2.25a2.625 2.625 0 11-5.25 0 2.625 2.625 0 015.25 0z" />
    </svg>
  ),
  Settings: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M4.5 12a7.5 7.5 0 0015 0m-15 0a7.5 7.5 0 1115 0m-15 0H3m16.5 0H21m-7.5 7.5V21m0-16.5V3m-5.196 4.738L6.336 5.603m9.328 12.794l2.03-1.172m-11.358 0l-2.03-1.172m9.328-12.794l2.03 1.172" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  ),
  Logout: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M15.75 9V5.25A2.25 2.25 0 0013.5 3h-6a2.25 2.25 0 00-2.25 2.25v13.5A2.25 2.25 0 007.5 21h6a2.25 2.25 0 002.25-2.25V15m3 0l3-3m0 0l-3-3m3 3H9" />
    </svg>
  ),
  Plus: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
    </svg>
  ),
  Database: () => (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M20.25 6.375c0 2.278-3.694 4.125-8.25 4.125S3.75 8.653 3.75 6.375m16.5 0c0-2.278-3.694-4.125-8.25-4.125S3.75 4.097 3.75 6.375m16.5 0v11.25c0 2.278-3.694 4.125-8.25 4.125s-8.25-1.847-8.25-4.125V6.375m16.5 0v3.75m-16.5-3.75v3.75m16.5 0v3.75M3.75 10.125v3.75m16.5 0v3.75M3.75 13.875v3.75" />
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
          { to: '/admin/dashboard#containers', icon: Icons.Plus, label: 'Containers' },
          { to: '/admin/dashboard#images', icon: Icons.Database, label: 'Images' },
          { to: '/admin/dashboard#networks', icon: Icons.Projects, label: 'Networks' },
          { to: '/admin/dashboard#volumes', icon: Icons.Database, label: 'Volumes' },
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
