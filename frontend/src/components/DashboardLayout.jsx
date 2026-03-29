// ===========================================
// Dashboard Layout (Global Framework)
// ===========================================

import { Outlet, NavLink, useNavigate, useLocation } from 'react-router-dom'
import useAuthStore from '../stores/authStore'
import {
  LayoutDashboard,
  FolderGit2,
  Users,
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
  Zap
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
    <div className="flex h-screen bg-[#030305] overflow-hidden relative">
      {/* Background Ambient Glows */}
      <div className="fixed inset-0 pointer-events-none z-0">
        <div className="absolute top-[-10%] left-[-10%] w-[40vw] h-[40vw] rounded-full bg-indigo-900/10 blur-[120px] mix-blend-screen opacity-40"></div>
        <div className="absolute bottom-[-10%] right-[-10%] w-[30vw] h-[30vw] rounded-full bg-purple-900/10 blur-[120px] mix-blend-screen opacity-40"></div>
      </div>

      {/* Sidebar Interface */}
      <aside className="w-72 bg-[#050507]/40 backdrop-blur-2xl border-r border-white/5 flex flex-col relative z-50">
        {/* Logo Branding */}
        <div className="p-10 pb-6">
          <div className="flex items-center gap-4 mb-2">
            <div className="w-12 h-12 bg-gradient-to-br from-indigo-500 to-purple-600 rounded-2xl flex items-center justify-center text-xl shadow-2xl shadow-indigo-500/20 active:scale-95 transition-transform cursor-pointer" onClick={() => navigate('/')}>
               <span className="font-black text-white tracking-tighter">LP</span>
            </div>
            <div>
              <h1 className="text-xl font-black text-white tracking-tighter uppercase italic">PaaS <span className="text-indigo-400">Core</span></h1>
              <p className="text-[9px] font-black text-slate-500 uppercase tracking-[0.3em] -mt-1">Framework v2.0</p>
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
                      : 'text-slate-500 hover:text-white hover:bg-white/[0.03] hover:border-white/10 border border-transparent'
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
                          : 'text-slate-500 hover:text-white hover:bg-white/[0.03] hover:border-white/10 border border-transparent'
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
        <div className="p-8 border-t border-white/5 bg-black/20 backdrop-blur-3xl">
          <div className="flex items-center gap-4 mb-8 p-3 rounded-2xl bg-white/[0.02] border border-white/5 hover:border-white/10 transition-colors">
            <div className="w-12 h-12 bg-gradient-to-br from-slate-700 to-indigo-900 rounded-xl flex items-center justify-center text-white font-black italic shadow-xl border border-white/10">
              {user?.name?.charAt(0)?.toUpperCase() || 'U'}
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-[11px] font-black text-white truncate uppercase tracking-tighter leading-tight">{user?.name}</p>
              <p className="text-[9px] text-slate-500 truncate font-mono mt-0.5">{user?.email}</p>
            </div>
          </div>
          
          <div className="space-y-1">
            {!isAdmin && (user?.role === 'superadmin' || user?.role === 'admin') && (
              <NavLink
                to="/admin"
                className="flex items-center gap-3 px-5 py-2.5 rounded-xl text-slate-500 hover:text-indigo-400 hover:bg-indigo-500/5 transition-all text-[10px] font-black uppercase tracking-widest group"
              >
                <ArrowRightLeft className="w-4 h-4 group-hover:rotate-180 transition-transform duration-500" />
                Admin Panel
              </NavLink>
            )}
            
            {isAdmin && (
              <NavLink
                to="/dashboard"
                className="flex items-center gap-3 px-5 py-2.5 rounded-xl text-slate-500 hover:text-emerald-400 hover:bg-emerald-500/5 transition-all text-[10px] font-black uppercase tracking-widest group"
              >
                <ArrowRightLeft className="w-4 h-4 group-hover:-rotate-180 transition-transform duration-500" />
                Student View
              </NavLink>
            )}
            
            <button
              onClick={handleLogout}
              className="flex items-center gap-3 px-5 py-2.5 rounded-xl text-slate-600 hover:text-rose-500 hover:bg-rose-500/5 transition-all w-full text-[10px] font-black uppercase tracking-widest group"
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
             <NavLink 
              to={isAdmin ? "/admin/feedback" : "/feedback"}
              className={({ isActive }) => 
                `relative p-3.5 rounded-2xl transition-all duration-500 group ${
                  isActive 
                  ? 'bg-indigo-500 text-white shadow-[0_10px_25px_rgba(99,102,241,0.4)]' 
                  : 'bg-white/5 border border-white/5 text-slate-500 hover:text-white hover:bg-white/10'
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
          <div className="absolute inset-0 pointer-events-none opacity-[0.03] bg-[url('https://grainy-gradients.vercel.app/noise.svg')]"></div>
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
          background: rgba(255, 255, 255, 0.05);
          border-radius: 10px;
        }
        .premium-scrollbar::-webkit-scrollbar-thumb:hover {
          background: rgba(255, 255, 255, 0.1);
        }
      `}</style>
    </div>
  )
}

export default DashboardLayout
