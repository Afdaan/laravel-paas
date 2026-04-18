// ===========================================
// Main App Component
// ===========================================
// Handles routing and layout
// ===========================================

import { useEffect, useState, lazy, Suspense } from 'react'
import { Routes, Route, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'

// Stores & API Services
import useAuthStore from './stores/authStore'
import { systemAPI, settingsAPI } from './services/api'

// Libs & Hooks
import useTranslation from './lib/useTranslation'

// Layouts & Global Components
import DashboardLayout from './components/DashboardLayout'
import LoadingScreen from './components/LoadingScreen'
import Setup from './pages/Setup'

// Lazy loaded pages for performance
const Landing = lazy(() => import('./pages/Landing'))
const Login = lazy(() => import('./pages/Login'))
const StudentDashboard = lazy(() => import('./pages/student/Dashboard'))
const StudentProjects = lazy(() => import('./pages/student/Projects'))
const StudentNewProject = lazy(() => import('./pages/student/NewProject'))
const StudentProjectDetail = lazy(() => import('./pages/student/ProjectDetail'))
const DatabaseManager = lazy(() => import('./pages/student/DatabaseManager'))
const AdminDashboard = lazy(() => import('./pages/admin/Dashboard'))
const AdminUsers = lazy(() => import('./pages/admin/Users'))
const AdminProjects = lazy(() => import('./pages/admin/Projects'))
const AdminSettings = lazy(() => import('./pages/admin/Settings'))
const AdminContainers = lazy(() => import('./pages/admin/Containers'))
const AdminImages = lazy(() => import('./pages/admin/Images'))
const AdminNetworks = lazy(() => import('./pages/admin/Networks'))
const AdminVolumes = lazy(() => import('./pages/admin/Volumes'))
const StudentDatabases = lazy(() => import('./pages/student/Databases'))
const StudentFeedback = lazy(() => import('./pages/student/Feedback'))
const AdminFeedback = lazy(() => import('./pages/admin/Feedback'))
const AdminDatabases = lazy(() => import('./pages/admin/Databases'))
const AdminDeploymentQueue = lazy(() => import('./pages/admin/DeploymentQueue'))

// Protected Route Component
interface ProtectedRouteProps {
  children: React.ReactNode;
  requireAdmin?: boolean;
}

function ProtectedRoute({ children, requireAdmin = false }: ProtectedRouteProps) {
  const { token, user, isLoading } = useAuthStore()
  const location = useLocation()
  const isAdmin = user?.role === 'superadmin' || user?.role === 'admin'

  if (isLoading) {
    return <LoadingScreen />
  }

  if (!token) {
    // Save current path to state so we can return here after login
    return <Navigate to="/login" replace state={{ from: location }} />
  }

  if (requireAdmin && !isAdmin) {
    return <Navigate to="/dashboard" replace />
  }

  return children
}

function App() {
  const { t } = useTranslation()
  const { fetchUser, token, user } = useAuthStore()
  const navigate = useNavigate()
  const [isInitialized, setIsInitialized] = useState<boolean | null>(null)

  const [settings, setSettings] = useState<any>(null)

  useEffect(() => {
    // Check if system is initialized
    systemAPI.getInitStatus()
      .then(res => {
        setIsInitialized(res.data.is_initialized)
      })
      .catch((err) => {
        console.error('Failed to check init status', err)
        // If it fails (maybe server still starting), try again later or assume true to avoid blocks
        setIsInitialized(true) 
      })
  }, [])

  useEffect(() => {
    if (token && !user) {
      fetchUser()
    }
  }, [token, user, fetchUser])

  useEffect(() => {
    if (token && (user?.role === 'admin' || user?.role === 'superadmin')) {
      settingsAPI.list().then(res => setSettings(res.data.map))
    }
  }, [token, user])

  useEffect(() => {
    let idleTimer: NodeJS.Timeout
    const timeoutMinutes = settings?.admin_idle_timeout || 15
    const IDLE_TIMEOUT = timeoutMinutes * 60 * 1000 

    const handleIdleLogout = () => {
      if (token) {
        useAuthStore.setState({ token: null, user: null, isLoading: false })
        localStorage.removeItem('token')
        toast.error(t('common.sessionExpired'), { id: 'user-idle-timeout' })
        navigate('/login', { state: { from: window.location.pathname }, replace: true })
      }
    }

    const resetTimer = () => {
      if (idleTimer) clearTimeout(idleTimer)
      idleTimer = setTimeout(handleIdleLogout, IDLE_TIMEOUT)
    }

    // Set up listeners for all authenticated users
    if (token) {
      const events = ['mousedown', 'mousemove', 'keypress', 'scroll', 'touchstart']
      events.forEach(event => window.addEventListener(event, resetTimer))
      resetTimer() // Initialize timer

      return () => {
        if (idleTimer) clearTimeout(idleTimer)
        events.forEach(event => window.removeEventListener(event, resetTimer))
      }
    }
  }, [token, t, settings])

  useEffect(() => {
    const handleExpired = () => {
      useAuthStore.setState({ token: null, user: null, isLoading: false })
      toast.error(t('common.sessionExpired'), { id: 'auth-expired' })
      navigate('/login', { state: { from: window.location.pathname }, replace: true })
    }

    const handleOffline = () => {
      toast.error(t('system.offline'), {
        id: 'system-offline',
        description: t('system.offlineDesc'),
        action: {
          label: t('common.retry'),
          onClick: () => window.location.reload()
        }
      })
    }

    const handleUpdating = () => {
      toast.info(t('system.updating'), {
        id: 'system-updating',
        description: t('system.updatingDesc'),
        duration: 8000,
        action: {
          label: t('common.reload'),
          onClick: () => window.location.reload()
        }
      })
    }

    window.addEventListener('auth:expired', handleExpired)
    window.addEventListener('system:offline', handleOffline)
    window.addEventListener('system:updating', handleUpdating)
    
    return () => {
      window.removeEventListener('auth:expired', handleExpired)
      window.removeEventListener('system:offline', handleOffline)
      window.removeEventListener('system:updating', handleUpdating)
    }
  }, [t])

  if (isInitialized === null) {
    return <LoadingScreen />
  }

  if (isInitialized === false && window.location.pathname !== '/setup') {
    return <Navigate to="/setup" replace />
  }

  return (
    <Suspense fallback={<LoadingScreen />}>
      <Routes>
        {/* Public Routes */}
        <Route path="/" element={<Landing />} />
        <Route path="/login" element={<Login />} />
        <Route 
          path="/setup" 
          element={
            isInitialized ? (
              <Navigate to="/login" replace />
            ) : (
              <Setup onComplete={() => setIsInitialized(true)} />
            )
          } 
        />

        {/* Student Routes */}
        <Route element={
          <ProtectedRoute>
            <DashboardLayout />
          </ProtectedRoute>
        }>
          <Route path="/dashboard" element={<StudentDashboard />} />
          <Route path="/projects" element={<StudentProjects />} />
          <Route path="/projects/new" element={<StudentNewProject />} />
          <Route path="/projects/:id" element={<StudentProjectDetail />} />
          <Route path="/databases" element={<StudentDatabases />} />
          <Route path="/projects/:id/database" element={<DatabaseManager />} />
          <Route path="/feedback" element={<StudentFeedback />} />
        </Route>

        {/* Admin Routes */}
        <Route path="/admin" element={
          <ProtectedRoute requireAdmin>
            <DashboardLayout isAdmin />
          </ProtectedRoute>
        }>
          <Route index element={<Navigate to="/admin/dashboard" replace />} />
          <Route path="dashboard" element={<AdminDashboard />} />
          <Route path="users" element={<AdminUsers />} />
          <Route path="projects" element={<AdminProjects />} />
          <Route path="containers" element={<AdminContainers />} />
          <Route path="images" element={<AdminImages />} />
          <Route path="networks" element={<AdminNetworks />} />
          <Route path="volumes" element={<AdminVolumes />} />
          <Route path="settings" element={<AdminSettings />} />
          <Route path="feedback" element={<AdminFeedback />} />
          <Route path="databases" element={<AdminDatabases />} />
          <Route path="queue" element={<AdminDeploymentQueue />} />
        </Route>

        {/* Fallback */}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Suspense>
  )
}

export default App
