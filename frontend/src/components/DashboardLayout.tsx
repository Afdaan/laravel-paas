import { Outlet, NavLink, useNavigate, useLocation } from 'react-router-dom'
import { useState, useEffect } from 'react'
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
  ShieldCheck,
  Zap,
  Sun,
  Moon
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import { Badge } from '@/components/ui/badge'

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

interface DashboardLayoutProps {
  isAdmin?: boolean;
}

function DashboardLayout({ isAdmin = false }: DashboardLayoutProps) {
  const { user, logout } = useAuthStore()
  const navigate = useNavigate()
  const location = useLocation()
  
  const [isDark, setIsDark] = useState(() => {
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

  const userInitials = user?.name ? user.name.substring(0, 2).toUpperCase() : 'US'
  
  return (
    <div className="flex h-screen bg-background text-foreground overflow-hidden font-sans">
      {/* Sidebar Interface */}
      <aside className="w-64 border-r bg-card flex flex-col z-50">
        {/* Logo Branding */}
        <div className="p-6">
          <div className="flex items-center gap-3 mb-4 cursor-pointer" onClick={() => navigate('/')}>
            <div className="w-10 h-10 bg-primary rounded-lg flex items-center justify-center text-primary-foreground shadow-sm">
               <span className="font-bold tracking-tighter text-sm">LP</span>
            </div>
            <div>
              <h1 className="text-xl font-bold tracking-tight">PaaS <span className="text-muted-foreground font-normal">Core</span></h1>
            </div>
          </div>
          
          <Badge variant={isAdmin ? "destructive" : "secondary"} className="w-full justify-center">
            {isAdmin ? <ShieldCheck className="w-3 h-3 mr-1" /> : <Zap className="w-3 h-3 mr-1" />}
            {isAdmin ? 'Global Admin' : 'Authenticated Hub'}
          </Badge>
        </div>
        
        {/* Navigation Registry */}
        <nav className="flex-1 px-4 py-4 space-y-6 overflow-y-auto">
          {/* Main Group */}
          <div className="space-y-1">
            <h4 className="px-2 text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Main</h4>
            {navItems.management.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                className={({ isActive }) =>
                  `flex items-center justify-between px-3 py-2 text-sm font-medium rounded-md transition-colors ${
                    isActive
                      ? 'bg-secondary text-secondary-foreground'
                      : 'text-muted-foreground hover:bg-secondary/50 hover:text-foreground'
                  }`
                }
              >
                <div className="flex items-center gap-3">
                  <item.icon className="w-4 h-4" />
                  <span>{item.label}</span>
                </div>
              </NavLink>
            ))}
          </div>

          {/* Infrastructure Group (Admin Only) */}
          {isAdmin && navItems.resources && (
            <div className="space-y-1 pt-4">
                <h4 className="px-2 text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Infrastructure</h4>
                {navItems.resources.map((item) => (
                <NavLink
                    key={item.to}
                    to={item.to}
                    className={({ isActive }) =>
                      `flex items-center justify-between px-3 py-2 text-sm font-medium rounded-md transition-colors ${
                        isActive
                          ? 'bg-secondary text-secondary-foreground'
                          : 'text-muted-foreground hover:bg-secondary/50 hover:text-foreground'
                      }`
                    }
                >
                    <div className="flex items-center gap-3">
                      <item.icon className="w-4 h-4" />
                      <span>{item.label}</span>
                    </div>
                </NavLink>
                ))}
            </div>
          )}
          
          {/* Action Trigger (Student Only) */}
          {!isAdmin && (
            <div className="pt-4">
              <Button asChild className="w-full justify-start" size="sm">
                <NavLink to="/projects/new">
                  <Plus className="w-4 h-4 mr-2" />
                  New Project
                </NavLink>
              </Button>
            </div>
          )}
        </nav>
        
        <div className="p-4 border-t">
          <div className="flex items-center gap-3 mb-4 rounded-md p-2 hover:bg-muted transition-colors">
            <Avatar className="w-9 h-9">
              <AvatarFallback className="bg-primary/10 text-primary">{userInitials}</AvatarFallback>
            </Avatar>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium leading-none truncate">{user?.name}</p>
              <p className="text-xs text-muted-foreground truncate mt-1">{user?.email}</p>
            </div>
          </div>
          
          <div className="space-y-1">
            {!isAdmin && (user?.role === 'superadmin' || user?.role === 'admin') && (
              <Button variant="ghost" className="w-full justify-start text-xs h-8" asChild>
                <NavLink to="/admin">
                  <ArrowRightLeft className="w-3 h-3 mr-2" />
                  Admin Panel
                </NavLink>
              </Button>
            )}
            
            {isAdmin && (
              <Button variant="ghost" className="w-full justify-start text-xs h-8" asChild>
                <NavLink to="/dashboard">
                  <ArrowRightLeft className="w-3 h-3 mr-2" />
                  Student View
                </NavLink>
              </Button>
            )}
            
            <Button 
               variant="ghost" 
               className="w-full justify-start text-xs h-8 text-destructive hover:text-destructive hover:bg-destructive/10"
               onClick={handleLogout}
            >
              <LogOut className="w-3 h-3 mr-2" />
              Logout
            </Button>
          </div>
        </div>
      </aside>
      
      {/* Content Stream Area */}
      <div className="flex-1 flex flex-col min-w-0 overflow-hidden bg-background">
        {/* Orbital Header */}
        <header className="h-16 flex items-center justify-between px-8 border-b">
           <div></div>
           
           <div className="flex items-center gap-4">
             <Button
                variant="outline"
                size="icon"
                onClick={() => setIsDark(!isDark)}
             >
                {isDark ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
             </Button>
             
             <Button
                variant={location.pathname.includes('feedback') ? "default" : "outline"}
                size="icon"
                asChild
                className="relative"
             >
               <NavLink to={isAdmin ? "/admin/feedback" : "/feedback"} title="Feedback">
                  <MessageSquare className="w-4 h-4" />
               </NavLink>
             </Button>
           </div>
        </header>

        {/* Global Main Stream */}
        <main className="flex-1 p-8 overflow-auto">
          <Outlet />
        </main>
      </div>
    </div>
  )
}

export default DashboardLayout
