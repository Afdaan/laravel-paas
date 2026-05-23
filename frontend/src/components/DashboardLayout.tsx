import { useEffect, useMemo, useState, useCallback } from 'react'
import { Outlet, NavLink, useNavigate, useLocation } from 'react-router-dom'
import useAuthStore from '../stores/authStore'
import useTranslation from '../lib/useTranslation'
import { projectsAPI } from '../services/api'
import { Project } from '../types'
import { FrameworkIcon } from './FrameworkIcon'
import { LanguageSwitcher } from './LanguageSwitcher'
import { usePolling } from '@/lib/usePolling'
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
  Sun,
  Moon,
  Globe,
  PanelLeftClose,
  PanelLeftOpen,
  ChevronDown
} from 'lucide-react'
import { useTheme } from './ThemeProvider'
import { Button } from '@/components/ui/button'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

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
  Domains: Globe,
}

interface DashboardLayoutProps {
  isAdmin?: boolean;
}

const projectStatusTone = (status?: Project['status']) => {
  switch (status) {
    case 'running':
      return 'bg-emerald-500'
    case 'building':
    case 'deploying':
    case 'queued':
    case 'pending':
    case 'restarting':
      return 'bg-amber-500'
    case 'failed':
    case 'error':
      return 'bg-rose-500'
    default:
      return 'bg-muted-foreground'
  }
}

function DashboardLayout({ isAdmin = false }: DashboardLayoutProps) {
  const { t } = useTranslation()
  const { user, logout, adminToken, returnToAdmin } = useAuthStore()
  const { theme, setTheme } = useTheme()
  const navigate = useNavigate()
  const location = useLocation()
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState<boolean>(() => {
    return localStorage.getItem('paas-sidebar-collapsed') === 'true'
  })
  const [sidebarWidth, setSidebarWidth] = useState<number>(() => {
    const saved = localStorage.getItem('paas-sidebar-width')
    if (saved) {
      const parsed = parseInt(saved, 10)
      if (!isNaN(parsed) && parsed >= 150 && parsed <= 320) {
        return parsed
      }
    }
    return 256
  })
  const [isDragging, setIsDragging] = useState(false)
  const [isHovered, setIsHovered] = useState(false)
  const isVisualExpanded = !isSidebarCollapsed || isHovered

  useEffect(() => {
    if (!isSidebarCollapsed) {
      setIsHovered(false)
    }
  }, [isSidebarCollapsed])

  const [projects, setProjects] = useState<Project[]>([])
  const [currentProject, setCurrentProject] = useState<Project | null>(null)
  const [isProjectsLoading, setIsProjectsLoading] = useState(false)

  useEffect(() => {
    localStorage.setItem('paas-sidebar-collapsed', String(isSidebarCollapsed))
  }, [isSidebarCollapsed])

  useEffect(() => {
    localStorage.setItem('paas-sidebar-width', String(sidebarWidth))
  }, [sidebarWidth])

  const isDark = theme === 'dark' || (theme === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches)
  const projectRouteMatch = location.pathname.match(/^\/projects\/([^/]+)/)
  const activeProjectUID = projectRouteMatch?.[1]
  const showProjectSwitcher = !isAdmin && !!activeProjectUID && activeProjectUID !== 'new'
  
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
          { to: '/admin/domains', icon: Icons.Domains, label: t('common.domains') },
          { to: '/admin/settings', icon: Icons.Settings, label: t('common.settings') },
        ],
        resources: [
          { to: '/admin/queue', icon: Icons.Dashboard, label: t('admin.queue.title') },
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
          { to: '/domains', icon: Icons.Domains, label: t('common.domains') },
        ]
      }

  const userInitials = user?.name ? user.name.substring(0, 2).toUpperCase() : t('common.initialsFallback')
  const isAdminBrowsingAsAdmin = isAdmin && !adminToken

  const startResizing = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    setIsDragging(true)
  }, [])

  useEffect(() => {
    if (!isDragging) return

    const handleMouseMove = (e: MouseEvent) => {
      const newWidth = e.clientX
      if (newWidth < 150) {
        setIsSidebarCollapsed(true)
      } else {
        setIsSidebarCollapsed(false)
        setSidebarWidth(Math.min(Math.max(newWidth, 200), 320))
      }
    }

    const handleMouseUp = () => {
      setIsDragging(false)
    }

    document.body.style.userSelect = 'none'
    document.body.style.cursor = 'col-resize'

    window.addEventListener('mousemove', handleMouseMove)
    window.addEventListener('mouseup', handleMouseUp)
    return () => {
      document.body.style.userSelect = ''
      document.body.style.cursor = ''
      window.removeEventListener('mousemove', handleMouseMove)
      window.removeEventListener('mouseup', handleMouseUp)
    }
  }, [isDragging])
  const activeProject = useMemo(
    () => projects.find((project) => project.uid === activeProjectUID || String(project.id) === activeProjectUID) || currentProject,
    [projects, currentProject, activeProjectUID]
  )

  useEffect(() => {
    if (!showProjectSwitcher || !activeProjectUID) {
      setCurrentProject(null)
      setProjects([])
      return
    }

    let isMounted = true
    setIsProjectsLoading(true)

    const loadProjects = async () => {
      try {
        const currentRes = await projectsAPI.get(activeProjectUID)
        if (!isMounted) return

        const fetchedCurrent = currentRes.data as Project
        setCurrentProject(fetchedCurrent)

        const listRes = isAdminBrowsingAsAdmin
          ? await projectsAPI.listAll({ page: 1, limit: 100 })
          : await projectsAPI.listOwn()
        if (!isMounted) return

        let nextProjects = (listRes.data.data || []) as Project[]
        if (isAdminBrowsingAsAdmin) {
          nextProjects = nextProjects.filter((project) => project.user_id === fetchedCurrent.user_id)
        }

        const hasCurrent = nextProjects.some((project) => project.uid === fetchedCurrent.uid || project.id === fetchedCurrent.id)
        if (!hasCurrent) {
          nextProjects = [fetchedCurrent, ...nextProjects]
        }

        setProjects(nextProjects)
      } catch (error) {
        if (isMounted) {
          setCurrentProject(null)
          setProjects([])
        }
      } finally {
        if (isMounted) setIsProjectsLoading(false)
      }
    }

    loadProjects()

    return () => {
      isMounted = false
    }
  }, [showProjectSwitcher, activeProjectUID, isAdminBrowsingAsAdmin, user?.id])

  usePolling(() => {
    if (!showProjectSwitcher || !activeProjectUID || isProjectsLoading) return

    const refreshStatuses = async () => {
      try {
        const currentRes = await projectsAPI.get(activeProjectUID)
        const updatedCurrent = currentRes.data as Project
        setCurrentProject(updatedCurrent)

        const listRes = isAdminBrowsingAsAdmin
          ? await projectsAPI.listAll({ page: 1, limit: 100 })
          : await projectsAPI.listOwn()
        
        let nextProjects = (listRes.data.data || []) as Project[]
        if (isAdminBrowsingAsAdmin) {
          nextProjects = nextProjects.filter((project) => project.user_id === updatedCurrent.user_id)
        }

        const hasCurrent = nextProjects.some((project) => project.uid === updatedCurrent.uid || project.id === updatedCurrent.id)
        if (!hasCurrent) {
          nextProjects = [updatedCurrent, ...nextProjects]
        }

        setProjects(nextProjects)
      } catch {
        // Silently fail during polling
      }
    }

    refreshStatuses()
  }, showProjectSwitcher ? 8000 : null)
  
  return (
    <div className="flex h-screen bg-background text-foreground overflow-hidden font-sans">
      {/* Sidebar Interface */}
      <div 
        style={{ width: isSidebarCollapsed ? 72 : sidebarWidth }}
        className={`relative shrink-0 z-50 ${isDragging ? '' : 'transition-[width] duration-300 ease-in-out'}`}
      >
        <aside 
          style={{ width: !isVisualExpanded ? 72 : sidebarWidth }}
          onMouseEnter={() => {
            if (isSidebarCollapsed) {
              setIsHovered(true)
            }
          }}
          onMouseLeave={() => {
            setIsHovered(false)
          }}
          className={`absolute left-0 top-0 bottom-0 border-r bg-card flex flex-col z-50 select-none shrink-0 ${
            isDragging ? '' : 'transition-[width] duration-300 ease-in-out'
          } ${isHovered ? 'shadow-2xl' : ''}`}
        >
          {/* Resize Handle */}
          <div
            className={`absolute right-0 top-0 bottom-0 w-1.5 cursor-col-resize hover:bg-primary/40 transition-colors z-50 ${
              isDragging ? 'bg-primary/60 w-2' : ''
            }`}
            onMouseDown={startResizing}
            title={t('common.dragToResize')}
          />

          {/* Logo Branding */}
          <div className="relative px-3 py-4 flex items-center h-[72px]">
            <button
              type="button"
              className="flex min-w-0 items-center rounded-lg p-2 text-left transition-all duration-300 hover:bg-muted justify-start"
              onClick={() => {
                if (!isVisualExpanded) {
                  setIsSidebarCollapsed(false);
                } else {
                  navigate(isAdmin ? '/admin/dashboard' : '/dashboard');
                }
              }}
              title="PaaS"
              style={{ width: isVisualExpanded ? 'calc(100% - 32px)' : '40px' }}
            >
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-sm">
                <span className="text-xs font-bold tracking-tighter">LP</span>
              </div>
              <div className={`min-w-0 text-left transition-all ease-in-out ${
                isVisualExpanded 
                  ? 'opacity-100 max-w-[150px] ml-3 duration-300 delay-100' 
                  : 'opacity-0 max-w-0 overflow-hidden ml-0 duration-75'
              }`}>
                <h1 className="truncate text-base font-semibold tracking-tight whitespace-nowrap">PaaS</h1>
                <p className="truncate text-[10px] font-medium text-muted-foreground whitespace-nowrap">
                  {isAdmin ? t('common.globalAdmin') : t('common.student')}
                </p>
              </div>
            </button>

            <div className={`absolute right-3 transition-all duration-300 ${
              isVisualExpanded ? 'opacity-100 scale-100 pointer-events-auto' : 'opacity-0 scale-75 pointer-events-none'
            }`}>
              <Button
                variant="ghost"
                size="icon-sm"
                onClick={() => {
                  const nextCollapsed = !isSidebarCollapsed;
                  setIsSidebarCollapsed(nextCollapsed);
                  setIsHovered(false);
                }}
                title={isSidebarCollapsed ? "Pin sidebar" : "Collapse sidebar"}
                className="text-muted-foreground hover:text-foreground shrink-0"
              >
                {isSidebarCollapsed ? <PanelLeftOpen className="h-4 w-4" /> : <PanelLeftClose className="h-4 w-4" />}
              </Button>
            </div>
          </div>

          {!isAdmin && (
            <div className="px-3 pb-3">
              <Button 
                render={<NavLink to="/projects/new" title={t('common.newProject')} />} 
                className="h-9 w-full transition-all duration-300 justify-start pl-4 pr-2" 
                size="sm"
              >
                <Plus className="h-4 w-4 shrink-0" />
                <span className={`truncate whitespace-nowrap transition-all ease-in-out ${
                  isVisualExpanded 
                    ? 'opacity-100 max-w-[150px] ml-2 duration-300 delay-100' 
                    : 'opacity-0 max-w-0 overflow-hidden ml-0 duration-75'
                }`}>
                  {t('common.newProject')}
                </span>
              </Button>
            </div>
          )}

          {/* Navigation Registry */}
          <nav className="flex-1 overflow-y-auto px-3 py-3 space-y-5">
            {/* Main Group */}
            <div className="space-y-1">
              <h4 className={`px-2 text-[10px] font-semibold text-muted-foreground uppercase tracking-wider transition-all duration-300 ${
                isVisualExpanded 
                  ? 'opacity-100 max-h-[20px] pb-1' 
                  : 'opacity-0 max-h-0 pb-0 overflow-hidden'
              }`}>
                {t('common.main')}
              </h4>
              {navItems.management.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  title={item.label}
                  className={({ isActive }) =>
                    `flex h-9 items-center rounded-md text-sm font-medium transition-all duration-300 justify-start pl-4 pr-2 ${
                      isActive
                         ? 'bg-secondary text-secondary-foreground shadow-sm'
                         : 'text-muted-foreground hover:bg-secondary/60 hover:text-foreground'
                    }`
                  }
                >
                  <div className="flex items-center w-full gap-2.5">
                    <item.icon className="h-4 w-4 shrink-0" />
                    <span className={`truncate whitespace-nowrap transition-all ease-in-out ${
                      isVisualExpanded 
                        ? 'opacity-100 max-w-[180px] ml-2.5 duration-300 delay-100' 
                        : 'opacity-0 max-w-0 overflow-hidden ml-0 duration-75'
                    }`}>
                      {item.label}
                    </span>
                  </div>
                </NavLink>
              ))}
            </div>

            {/* Infrastructure Group (Admin Only) */}
            {isAdmin && navItems.resources && (
              <div className="space-y-1 pt-2">
                <h4 className={`px-2 text-[10px] font-semibold text-muted-foreground uppercase tracking-wider transition-all duration-300 ${
                  isVisualExpanded 
                    ? 'opacity-100 max-h-[20px] pb-1' 
                    : 'opacity-0 max-h-0 pb-0 overflow-hidden'
                }`}>
                  {t('common.infrastructure')}
                </h4>
                {navItems.resources.map((item) => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    title={item.label}
                    className={({ isActive }) =>
                      `flex h-9 items-center rounded-md text-sm font-medium transition-all duration-300 justify-start pl-4 pr-2 ${
                        isActive
                           ? 'bg-secondary text-secondary-foreground shadow-sm'
                           : 'text-muted-foreground hover:bg-secondary/60 hover:text-foreground'
                      }`
                    }
                  >
                    <div className="flex items-center w-full gap-2.5">
                      <item.icon className="h-4 w-4 shrink-0" />
                      <span className={`truncate whitespace-nowrap transition-all ease-in-out ${
                        isVisualExpanded 
                          ? 'opacity-100 max-w-[180px] ml-2.5 duration-300 delay-100' 
                          : 'opacity-0 max-w-0 overflow-hidden ml-0 duration-75'
                      }`}>
                        {item.label}
                      </span>
                    </div>
                  </NavLink>
                ))}
              </div>
            )}
          </nav>
          
          <div className="p-3 border-t">
            <div className="mb-2 flex items-center rounded-md transition-all duration-300 hover:bg-muted p-1.5 gap-3">
              <Avatar className="h-9 w-9 shrink-0">
                <AvatarFallback className="bg-primary/10 text-primary">{userInitials}</AvatarFallback>
              </Avatar>
              <div className={`flex-1 min-w-0 text-left transition-all ease-in-out ${
                isVisualExpanded 
                  ? 'opacity-100 max-w-[180px] duration-300 delay-100' 
                  : 'opacity-0 max-w-0 overflow-hidden duration-75'
              }`}>
                <p className="text-sm font-medium leading-none truncate whitespace-nowrap">{user?.name}</p>
                <p className="text-xs text-muted-foreground truncate mt-1 whitespace-nowrap">{user?.email}</p>
              </div>
            </div>
            
            <div className="space-y-1">
              {!isAdmin && (user?.role === 'superadmin' || user?.role === 'admin') && (
                <Button 
                  variant="ghost" 
                  className="h-8 w-full text-xs transition-all duration-300 justify-start pl-4 pr-2" 
                  render={<NavLink to="/admin" title={t('common.adminPanel')} />}
                >
                  <ArrowRightLeft className="h-3.5 w-3.5 shrink-0" />
                  <span className={`truncate whitespace-nowrap transition-all ease-in-out ${
                    isVisualExpanded 
                      ? 'opacity-100 max-w-[150px] ml-2 duration-300 delay-100' 
                      : 'opacity-0 max-w-0 overflow-hidden ml-0 duration-75'
                  }`}>
                    {t('common.adminPanel')}
                  </span>
                </Button>
              )}
              
              {isAdmin && (
                <Button 
                  variant="ghost" 
                  className="h-8 w-full text-xs transition-all duration-300 justify-start pl-4 pr-2" 
                  render={<NavLink to="/dashboard" title={t('common.studentView')} />}
                >
                  <ArrowRightLeft className="h-3.5 w-3.5 shrink-0" />
                  <span className={`truncate whitespace-nowrap transition-all ease-in-out ${
                    isVisualExpanded 
                      ? 'opacity-100 max-w-[150px] ml-2 duration-300 delay-100' 
                      : 'opacity-0 max-w-0 overflow-hidden ml-0 duration-75'
                  }`}>
                    {t('common.studentView')}
                  </span>
                </Button>
              )}
              
              <Button 
                 variant="ghost" 
                 className="h-8 w-full text-xs text-destructive hover:text-destructive hover:bg-destructive/10 transition-all duration-300 justify-start pl-4 pr-2"
                 onClick={handleLogout}
                 title={t('common.logout')}
              >
                <LogOut className="h-3.5 w-3.5 shrink-0" />
                <span className={`truncate whitespace-nowrap transition-all ease-in-out ${
                  isVisualExpanded 
                    ? 'opacity-100 max-w-[150px] ml-2 duration-300 delay-100' 
                    : 'opacity-0 max-w-0 overflow-hidden ml-0 duration-75'
                }`}>
                  {t('common.logout')}
                </span>
              </Button>
            </div>
          </div>
        </aside>
      </div>
      
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
        <header className="h-16 flex items-center justify-between gap-4 px-8 border-b">
           <div className="flex min-w-0 items-center">
             {showProjectSwitcher && (
               <DropdownMenu>
                 <DropdownMenuTrigger className="group flex h-11 min-w-0 max-w-[380px] items-center gap-3 rounded-lg border border-border bg-card px-3.5 py-2 text-left shadow-sm transition-colors hover:bg-muted/60 focus:outline-none focus:ring-2 focus:ring-primary/20">
                   <FrameworkIcon framework={activeProject?.framework} variant="compact" className="h-8 w-8 shrink-0" />
                   <div className="min-w-0 flex-1">
                     <div className="truncate text-sm font-semibold leading-tight">
                       {activeProject?.name || activeProjectUID}
                     </div>
                     <div className="mt-0.5 flex items-center gap-1.5 text-[10px] font-medium text-muted-foreground">
                       <span className={`h-1.5 w-1.5 rounded-full ${projectStatusTone(activeProject?.status)}`} />
                       <span className="truncate uppercase tracking-wide">
                         {activeProject?.status || (isProjectsLoading ? t('common.loading') : 'Project')}
                       </span>
                     </div>
                   </div>
                   <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground transition-transform group-data-[popup-open]:rotate-180" />
                 </DropdownMenuTrigger>

                 <DropdownMenuContent align="start" className="w-[360px] p-1.5">
                   <div className="px-2 py-1.5 text-[10px] font-bold uppercase tracking-widest text-muted-foreground">
                     {t('common.projects')}
                   </div>
                   {projects.length > 0 ? (
                     projects.map((project) => {
                       const isActive = project.uid === activeProjectUID || String(project.id) === activeProjectUID || project.uid === activeProject?.uid
                       return (
                         <DropdownMenuItem
                           key={project.uid}
                           onClick={() => navigate(`/projects/${project.uid}`)}
                           className={`flex cursor-pointer items-center gap-3 rounded-md px-2.5 py-2.5 ${isActive ? 'bg-accent text-accent-foreground' : ''}`}
                         >
                           <FrameworkIcon framework={project.framework} variant="compact" className="h-8 w-8 shrink-0" />
                           <div className="min-w-0 flex-1">
                             <div className="truncate text-sm font-medium leading-none">{project.name}</div>
                             <div className="mt-1 flex items-center gap-1.5 text-[10px] text-muted-foreground">
                               <span className={`h-1.5 w-1.5 rounded-full ${projectStatusTone(project.status)}`} />
                               <span className="truncate">{project.subdomain || project.uid}</span>
                             </div>
                           </div>
                           {isActive && (
                             <span className="text-[10px] font-bold uppercase tracking-wider text-primary">
                               Active
                             </span>
                           )}
                         </DropdownMenuItem>
                       )
                     })
                   ) : (
                     <div className="px-2 py-6 text-center text-xs text-muted-foreground">
                       {isProjectsLoading ? t('common.loading') : t('common.noData')}
                     </div>
                   )}
                   <div className="mt-1 border-t pt-1">
                     <DropdownMenuItem
                       onClick={() => navigate('/projects/new')}
                       className="cursor-pointer gap-2 rounded-md px-2.5 py-2 text-sm"
                     >
                       <Plus className="h-4 w-4" />
                       {t('common.newProject')}
                     </DropdownMenuItem>
                   </div>
                 </DropdownMenuContent>
               </DropdownMenu>
             )}
           </div>
           
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
