import { Outlet, NavLink, useNavigate, useLocation } from 'react-router-dom'
import useAuthStore from '../stores/authStore'
import useTranslation from '../lib/useTranslation'
import { LanguageSwitcher } from './LanguageSwitcher'
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
import { useTheme } from './ThemeProvider'
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
  const { t } = useTranslation()
  const { user, logout, adminToken, returnToAdmin } = useAuthStore()
  const { theme, setTheme } = useTheme()
  const navigate = useNavigate()
  const location = useLocation()
  
  const isDark = theme === 'dark' || (theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches)
  
  const handleLogout = async () => {
    await logout()
    navigate('/login')
  }

  const handleReturnToAdmin = async () => {
    await returnToAdmin()
    navigate('/admin/users')
  }
  
  const navItems = isAdmin
    ? {
        management: [
          { to: '/admin/dashboard', icon: Icons.Dashboard, label: t('common.dashboard') },
          { to: '/admin/users', icon: Icons.Users, label: t('common.users') },
          { to: '/admin/projects', icon: Icons.Projects, label: t('common.projects') },
          { to: '/admin/databases', icon: Icons.Database, label: t('common.databases') },
          { to: '/admin/settings', icon: Icons.Settings, label: t('common.settings') },
        ],
        resources: [
          { to: '/admin/containers', icon: Icons.Containers, label: t('common.containers') },
          { to: '/admin/images', icon: Icons.Images, label: t('common.images') },
          { to: '/admin/networks', icon: Icons.Networks, label: t('common.networks') },
          { to: '/admin/volumes', icon: Icons.Volumes, label: t('common.volumes') },
        ]
      }
    : {
        management: [
          { to: '/dashboard', icon: Icons.Dashboard, label: t('common.dashboard') },
          { to: '/projects', icon: Icons.Projects, label: t('common.projects') },
          { to: '/databases', icon: Icons.Database, label: t('common.databases') },
        ]
      }

  const userInitials = user?.name ? user.name.substring(0, 2).toUpperCase() : t('common.initialsFallback')
  
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
            <h1 className="text-xl font-bold tracking-tighter">PaaS</h1>
          </div>
          
          <Badge variant={isAdmin ? "destructive" : "secondary"} className="w-full justify-center">
            {isAdmin ? <ShieldCheck className="w-3 h-3 mr-1" /> : <Zap className="w-3 h-3 mr-1" />}
            {isAdmin ? t('common.globalAdmin') : t('common.authenticatedHub')}
          </Badge>
        </div>
        
        {/* Navigation Registry */}
        <nav className="flex-1 px-4 py-4 space-y-6 overflow-y-auto">
          {/* Main Group */}
          <div className="space-y-1">
            <h4 className="px-2 text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">{t('common.main')}</h4>
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
                <h4 className="px-2 text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">{t('common.infrastructure')}</h4>
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
              <Button render={<NavLink to="/projects/new" />} className="w-full justify-start" size="sm">
                <Plus className="w-4 h-4 mr-2" />
                {t('common.newProject')}
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
              <Button variant="ghost" className="w-full justify-start text-xs h-8" render={<NavLink to="/admin" />}>
                <ArrowRightLeft className="w-3 h-3 mr-2" />
                {t('common.adminPanel')}
              </Button>
            )}
            
            {isAdmin && (
              <Button variant="ghost" className="w-full justify-start text-xs h-8" render={<NavLink to="/dashboard" />}>
                <ArrowRightLeft className="w-3 h-3 mr-2" />
                {t('common.studentView')}
              </Button>
            )}
            
            <Button 
               variant="ghost" 
               className="w-full justify-start text-xs h-8 text-destructive hover:text-destructive hover:bg-destructive/10"
               onClick={handleLogout}
            >
              <LogOut className="w-3 h-3 mr-2" />
              {t('common.logout')}
            </Button>
          </div>
        </div>
      </aside>
      
      {/* Content Stream Area */}
      <div className="flex-1 flex flex-col min-w-0 overflow-hidden bg-background">
        {/* Impersonation Banner */}
        {adminToken && (
          <div className="bg-destructive/10 text-destructive px-8 py-2.5 border-b border-destructive/20 flex items-center justify-between text-sm w-full font-medium shadow-sm z-40">
            <div className="flex items-center gap-2">
              <ShieldCheck className="w-4 h-4" />
              {t('auth.impersonating')} <span className="font-bold">{user?.name}</span>
            </div>
            <Button size="sm" variant="destructive" onClick={handleReturnToAdmin} className="h-8 text-xs font-semibold px-4 cursor-pointer">
              <LogOut className="w-3.5 h-3.5 mr-2" />
              {t('auth.returnToAdmin')}
            </Button>
          </div>
        )}

        {/* Orbital Header */}
        <header className="h-16 flex items-center justify-between px-8 border-b">
           <div></div>
           
           <div className="flex items-center gap-4">
             <LanguageSwitcher />

             <Button
                variant="outline"
                size="icon"
                onClick={() => setTheme(isDark ? 'light' : 'dark')}
                title={t('common.theme')}
             >
                {isDark ? <Sun className="w-4 h-4" /> : <Moon className="w-4 h-4" />}
             </Button>
             
             <Button
                variant={location.pathname.includes('feedback') ? "default" : "outline"}
                size="icon"
                render={<NavLink to={isAdmin ? "/admin/feedback" : "/feedback"} title={t('common.feedback')} />}
                className="relative"
             >
                <MessageSquare className="w-4 h-4" />
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
