// ===========================================
// Dashboard Layout (Global Framework)
// ===========================================

import { Outlet, NavLink, useNavigate, useLocation } from 'react-router-dom'
import { useState, useEffect } from 'react'
import useAuthStore from '../stores/authStore'
import {
  LayoutDashboard,
  FolderGit2,
  Users,
  User,
  Settings,
  LogOut,
  Plus,
  Database,
  Box,
  Image as ImageIcon,
  Network,
  HardDrive,
  MessageSquare,
  ArrowRightLeft,
  ChevronRight,
  ShieldCheck,
  Zap,
  Sun,
  Moon
} from 'lucide-react'

const Icons = {
  Dashboard: LayoutDashboard,
  Projects: FolderGit2,
  Users: Users,
  Settings: Settings,
  Logout: LogOut,
  Plus: Plus,
  Database: Database,
  Containers: Box,
  Images: ImageIcon,
  Networks: Network,
  Volumes: HardDrive,
  Feedback: MessageSquare,
}

function DashboardLayout({ isAdmin = false }) {
  const { user, logout } = useAuthStore()
  const navigate = useNavigate()
  const location = useLocation()
  
  const [isDark, setIsDark] = useState(() => {
    // Default to dark mode for standard Laravel PaaS appearance
    if (typeof window !== 'undefined') {
      const savedTheme = localStorage.getItem('theme')
      if (savedTheme) {
        return savedTheme === 'dark'
      }
      return window.matchMedia('(prefers-color-scheme: dark)').matches
    }
    return true
  })

  useEffect(() => {
    if (isDark) {
      document.documentElement.classList.add('dark')
      localStorage.setItem('theme', 'dark')
    } else {
      document.documentElement.classList.remove('dark')
      localStorage.setItem('theme', 'light')
    }
  }, [isDark])
  
  const handleLogout = async () => {
    await logout()
    navigate('/login')
  }
  
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
          { to: '/projects', icon: Icons.Projects, label: 'Projects' },
          { to: '/databases', icon: Icons.Database, label: 'Databases' },
        ]
      }
  
  return (
    <div className="flex h-screen bg-slate-50 dark:bg-slate-900 overflow-hidden relative">
      {/* Background Ambient Glows */}
      <div className="fixed inset-0 pointer-events-none z-0">
        
        
      </div>

      {/* Sidebar Interface */}
      <aside className="w-72 bg-white dark:bg-slate-800 backdrop-blur-2xl border-r border-slate-200 dark:border-white/5 flex flex-col relative z-50">
        {/* Logo Branding */}
        <div className="p-10 pb-6">
          <div className="flex items-center gap-4 mb-2">
            <div className="w-12 h-12 bg-gradient-to-br from-indigo-500 to-purple-600 rounded-2xl flex items-center justify-center text-xl shadow-2xl shadow-indigo-500/20 active:scale-95 transition-transform cursor-pointer" onClick={() => navigate('/')}>
               <span className="font-black text-white tracking-tighter">LP</span>
            </div>
            <div>
              <h1 className="text-xl font-black text-slate-900 dark:text-white tracking-tighter uppercase italic">PaaS <span className="text-indigo-400">Core</span></h1>
              <p className="text-[9px] font-black text-slate-600 dark:text-slate-400 uppercase tracking-[0.3em] -mt-1">Framework v2.0</p>
            </div>
          </div>
          
          {isAdmin ? (
            <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-indigo-500/10 border border-indigo-500/20 mt-4">
                <ShieldCheck className="w-3 h-3 text-indigo-400" />
                <span className="text-[9px] text-indigo-400 uppercase font-black tracking-widest leading-none">Global Admin Privileges</span>
            </div>
          ) : (
             <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-emerald-500/10 border border-emerald-500/20 mt-4">
                <Zap className="w-3 h-3 text-emerald-400" />
                <span className="text-[9px] text-emerald-400 uppercase font-black tracking-widest leading-none">Authenticated Hub</span>
            </div>
          )}
        </div>
        
        {/* Navigation Registry */}
        <nav className="flex-1 px-6 py-6 space-y-10 overflow-y-auto premium-scrollbar">
          {/* Main Group */}
          <div className="space-y-1.5">
            <p className="text-[10px] font-black text-slate-600 uppercase tracking-[0.3em] mb-4 ml-4">Main</p>
            {navItems.management.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                className={({ isActive }) =>
                  `flex items-center justify-between px-5 py-3.5 rounded-2xl transition-all duration-300 group ${
                    isActive
                      ? 'bg-indigo-500/10 text-indigo-400 border border-indigo-500/30 shadow-[0_0_20px_rgba(99,102,241,0.1)]'
                      : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5 hover:border-slate-300 dark:border-white/10 border border-transparent'
                  }`
                }
              >
                <div className="flex items-center gap-4">
                  <item.icon className={`w-5 h-5 transition-transform duration-500 group-hover:scale-110 group-active:scale-95`} />
                  <span className="text-xs font-black tracking-tight uppercase">{item.label}</span>
                </div>
                <ChevronRight className="w-3 h-3 opacity-0 group-hover:opacity-100 group-hover:translate-x-1 transition-all" />
              </NavLink>
            ))}
          </div>

          {/* Infrastructure Group (Admin Only) */}
          {isAdmin && navItems.resources && (
            <div className="space-y-1.5 pt-4">
                <p className="text-[10px] font-black text-slate-600 uppercase tracking-[0.3em] mb-4 ml-4">Infrastructure</p>
                {navItems.resources.map((item) => (
                <NavLink
                    key={item.to}
                    to={item.to}
                    className={({ isActive }) =>
                      `flex items-center justify-between px-5 py-3.5 rounded-2xl transition-all duration-300 group ${
                        isActive
                          ? 'bg-purple-500/10 text-purple-400 border border-purple-500/30 shadow-[0_0_20px_rgba(168,85,247,0.1)]'
                          : 'text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5 hover:border-slate-300 dark:border-white/10 border border-transparent'
                      }`
                    }
                >
                    <div className="flex items-center gap-4">
                      <item.icon className="w-5 h-5 transition-transform group-hover:rotate-12" />
                      <span className="text-xs font-black tracking-tight uppercase">{item.label}</span>
                    </div>
                    <ChevronRight className="w-3 h-3 opacity-0 group-hover:opacity-100 transition-all" />
                </NavLink>
                ))}
            </div>
          )}
          
          {/* Action Trigger (Student Only) */}
          {!isAdmin && (
            <NavLink
              to="/projects/new"
              className="flex items-center justify-center gap-3 px-6 py-4 rounded-2xl bg-indigo-500 text-white font-black text-[11px] uppercase tracking-widest shadow-[0_10px_20px_rgba(99,102,241,0.2)] hover:bg-indigo-400 hover:scale-[1.02] transform transition-all active:scale-95 mt-10"
            >
              <Plus className="w-5 h-5" />
              New Project
            </NavLink>
          )}
        </nav>
        
        {/* Identity & Protocol */}
        <div className="p-8 border-t border-slate-200 dark:border-white/5 bg-slate-50/50 dark:bg-black/20 backdrop-blur-3xl">
          <div className="flex items-center gap-4 mb-8 p-3 rounded-2xl bg-white dark:bg-white/5 border border-slate-200 dark:border-white/5 hover:border-slate-300 dark:hover:border-white/10 transition-colors shadow-sm">
            <div className="w-12 h-12 bg-gradient-to-br from-indigo-500 to-indigo-900 rounded-xl flex items-center justify-center text-white shadow-md border border-indigo-400 dark:border-white/10">
              <User className="w-5 h-5 opacity-90" strokeWidth={2.5} />
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-[11px] font-black text-slate-900 dark:text-white truncate uppercase tracking-tighter leading-tight">{user?.name}</p>
              <p className="text-[9px] text-slate-500 dark:text-slate-400 truncate font-mono mt-0.5">{user?.email}</p>
            </div>
          </div>
          
          <div className="space-y-1">
            {!isAdmin && (user?.role === 'superadmin' || user?.role === 'admin') && (
              <NavLink
                to="/admin"
                className="flex items-center gap-3 px-5 py-2.5 rounded-xl text-slate-600 dark:text-slate-400 hover:text-indigo-600 dark:hover:text-indigo-400 hover:bg-indigo-50 dark:hover:bg-indigo-500/10 transition-all text-[10px] font-black uppercase tracking-widest group"
              >
                <ArrowRightLeft className="w-4 h-4 group-hover:rotate-180 transition-transform duration-500" />
                Admin Panel
              </NavLink>
            )}
            
            {isAdmin && (
              <NavLink
                to="/dashboard"
                className="flex items-center gap-3 px-5 py-2.5 rounded-xl text-slate-600 dark:text-slate-400 hover:text-emerald-600 dark:hover:text-emerald-400 hover:bg-emerald-50 dark:hover:bg-emerald-500/10 transition-all text-[10px] font-black uppercase tracking-widest group"
              >
                <ArrowRightLeft className="w-4 h-4 group-hover:-rotate-180 transition-transform duration-500" />
                Student View
              </NavLink>
            )}
            
            <button
              onClick={handleLogout}
              className="flex items-center gap-3 px-5 py-2.5 rounded-xl text-slate-600 dark:text-slate-400 hover:text-rose-600 dark:hover:text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-500/10 transition-all w-full text-[10px] font-black uppercase tracking-widest group"
            >
              <LogOut className="w-4 h-4 group-hover:translate-x-1 transition-transform" />
              Logout
            </button>
          </div>
        </div>
      </aside>
      
      {/* Content Stream Area */}
      <div className="flex-1 flex flex-col min-w-0 overflow-hidden relative z-10">
        {/* Orbital Header */}
        <header className="h-24 flex items-center justify-between px-10 bg-transparent relative z-20">
           <div>
              {/* Optional page title context could go here if passed via props */}
           </div>
           
           <div className="flex items-center gap-6">
             <button
                onClick={() => setIsDark(!isDark)}
                className="relative p-3.5 rounded-2xl transition-all duration-300 bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white hover:bg-slate-200 dark:bg-white/10 shadow-sm"
                title="Toggle Theme"
             >
                {isDark ? <Sun className="w-5 h-5 transition-transform" /> : <Moon className="w-5 h-5 transition-transform" />}
             </button>
             <NavLink 
              to={isAdmin ? "/admin/feedback" : "/feedback"}
              className={({ isActive }) => 
                `relative p-3.5 rounded-2xl transition-all duration-500 group ${
                  isActive 
                  ? 'bg-indigo-500 text-slate-900 dark:text-white shadow-[0_10px_25px_rgba(99,102,241,0.4)]' 
                  : 'bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white hover:bg-slate-200 dark:bg-white/10'
                }`
              }
              title={isAdmin ? "Feedback Console" : "Contact Engineers"}
             >
                <MessageSquare className="w-5 h-5 group-hover:scale-110 transition-transform" />
                {/* Priority Indicator */}
                <span className={`absolute top-2 right-2 w-2 h-2 rounded-full border-2 border-black/80 shadow-lg animate-pulse ${location.pathname.includes('feedback') ? 'bg-white' : 'bg-indigo-500'}`}></span>
             </NavLink>
           </div>
        </header>

        {/* Global Main Stream */}
        <main className="flex-1 px-10 pb-10 overflow-auto premium-scrollbar relative">
          <div key={location.pathname} className="animate-pop-in h-full">
            <Outlet />
          </div>
          
          {/* Subtle noise texture overlay */}
          
        </main>
      </div>

      <style>{`
        .premium-scrollbar::-webkit-scrollbar {
          width: 4px;
        }
        .premium-scrollbar::-webkit-scrollbar-track {
          background: transparent;
        }
        .premium-scrollbar::-webkit-scrollbar-thumb {
          background: rgba(100, 116, 139, 0.2);
          border-radius: 10px;
        }
        .dark .premium-scrollbar::-webkit-scrollbar-thumb {
          background: rgba(255, 255, 255, 0.05);
        }
        .premium-scrollbar::-webkit-scrollbar-thumb:hover {
          background: rgba(100, 116, 139, 0.3);
        }
        .dark .premium-scrollbar::-webkit-scrollbar-thumb:hover {
          background: rgba(255, 255, 255, 0.1);
        }
      `}</style>
    </div>
  )
}

export default DashboardLayout
