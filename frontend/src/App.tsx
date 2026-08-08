// ===========================================
// Main App Component
// ===========================================
// Handles routing and layout
// ===========================================

import { lazy, Suspense, useEffect, useRef, useState } from 'react'
import { Routes, Route, Navigate, useLocation, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'

// Stores & API Services
import useAuthStore from './stores/authStore'
import { systemAPI, settingsAPI } from './services/api'

// Libs & Hooks
import useTranslation from './lib/useTranslation'

// Layouts & Global Components
import DashboardLayout from './components/DashboardLayout'
import ReauthModal from './components/ReauthModal' 
import LoadingScreen from './components/LoadingScreen'
import Setup from './pages/Setup'

// Lazy loaded pages for performance
const Landing = lazy(() => import('./pages/Landing'))
const Login = lazy(() => import('./pages/Login'))
const UserDashboard = lazy(() => import('./pages/user/Dashboard'))
const UserProjects = lazy(() => import('./pages/user/Projects'))
const UserNewProject = lazy(() => import('./pages/user/NewProject'))
const UserProjectDetail = lazy(() => import('./pages/user/ProjectDetail'))
const DatabaseManager = lazy(() => import('./pages/user/DatabaseStudio'))
const AdminDashboard = lazy(() => import('./pages/admin/Dashboard'))
const AdminUsers = lazy(() => import('./pages/admin/Users'))
const AdminProjects = lazy(() => import('./pages/admin/Projects'))
const AdminSettings = lazy(() => import('./pages/admin/Settings'))
const AdminContainers = lazy(() => import('./pages/admin/Containers'))
const AdminImages = lazy(() => import('./pages/admin/Images'))
const AdminNetworks = lazy(() => import('./pages/admin/Networks'))
const AdminVolumes = lazy(() => import('./pages/admin/Volumes'))
const UserDatabases = lazy(() => import('./pages/user/Databases'))
const UserFeedback = lazy(() => import('./pages/user/Feedback'))
const AdminFeedback = lazy(() => import('./pages/admin/Feedback'))
const AdminDatabases = lazy(() => import('./pages/admin/Databases'))
const AdminDeploymentQueue = lazy(() => import('./pages/admin/DeploymentQueue'))
const UserDomains = lazy(() => import('./pages/user/Domains'))
const AdminDomains = lazy(() => import('./pages/admin/Domains'))
const UserSettings = lazy(() => import('./pages/user/Settings').then(module => ({ default: module.UserSettings })))
const UserSecretStore = lazy(() => import('./pages/user/SecretStoreDashboard'))
const UserBilling = lazy(() => import('./pages/user/Billing'))
const AdminSecretStore = lazy(() => import('./pages/admin/AdminSecretStoreExplorer'))
const AdminBilling = lazy(() => import('./pages/admin/Billing'))


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

const ACTIVITY_THROTTLE_MS = 10_000

function App() {
  const { t } = useTranslation()
  const { fetchUser, logout, token, user } = useAuthStore()
  const navigate = useNavigate()
  const [isInitialized, setIsInitialized] = useState<boolean | null>(null)
  const idleTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const lastActivityAtRef = useRef(0)

  const [settings, setSettings] = useState<Record<string, string> | null>(null)

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
    fetchUser()
  }, [fetchUser])

  useEffect(() => {
    if (token && (user?.role === 'admin' || user?.role === 'superadmin')) {
      settingsAPI.list().then(res => setSettings(res.data.map))
    }
  }, [token, user])

  useEffect(() => {
    const timeoutMinutes = settings?.admin_idle_timeout ? parseInt(settings.admin_idle_timeout) : 15
    const IDLE_TIMEOUT = timeoutMinutes * 60 * 1000

    const handleIdleLogout = () => {
      const currentToken = useAuthStore.getState().token
      if (currentToken) {
        void logout()
        toast.error(t('common.sessionExpired'), { id: 'session-expired-toast' })
        navigate('/login', { state: { from: window.location.pathname }, replace: true })
      }
    }

    const resetTimer = () => {
      if (idleTimerRef.current) clearTimeout(idleTimerRef.current)
      idleTimerRef.current = setTimeout(handleIdleLogout, IDLE_TIMEOUT)
    }

    const handleActivity = () => {
      const now = Date.now()
      if (now - lastActivityAtRef.current < ACTIVITY_THROTTLE_MS) return

      lastActivityAtRef.current = now
      resetTimer()
    }

    if (!token) {
      if (idleTimerRef.current) clearTimeout(idleTimerRef.current)
      idleTimerRef.current = null
      return
    }

    const passiveEvents = ['pointermove', 'scroll', 'touchstart'] as const
    const activeEvents = ['pointerdown', 'keydown'] as const

    passiveEvents.forEach(event => window.addEventListener(event, handleActivity, { passive: true }))
    activeEvents.forEach(event => window.addEventListener(event, handleActivity))
    resetTimer()

    return () => {
      if (idleTimerRef.current) clearTimeout(idleTimerRef.current)
      idleTimerRef.current = null
      passiveEvents.forEach(event => window.removeEventListener(event, handleActivity))
      activeEvents.forEach(event => window.removeEventListener(event, handleActivity))
    }
  }, [token, t, settings, navigate, logout])

  useEffect(() => {
    const handleExpired = () => {
      const currentToken = useAuthStore.getState().token
      if (currentToken) {
        useAuthStore.setState({ token: null, user: null, adminToken: null, isLoading: false })
        toast.error(t('common.sessionExpired'), { id: 'session-expired-toast' })
        navigate('/login', { state: { from: window.location.pathname }, replace: true })
      }
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
  }, [t, navigate])

  if (isInitialized === null) {
    return <LoadingScreen />
  }

  if (isInitialized === false && window.location.pathname !== '/setup') {
    return <Navigate to="/setup" replace />
  }

  return (
    <>
      <ReauthModal />
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

        {/* User Routes */}
        <Route element={
          <ProtectedRoute>
            <DashboardLayout />
          </ProtectedRoute>
        }>
          <Route path="/dashboard" element={<UserDashboard />} />
          <Route path="/projects" element={<UserProjects />} />
          <Route path="/projects/new" element={<UserNewProject />} />
          <Route path="/projects/:uid" element={<UserProjectDetail />} />
          <Route path="/databases" element={<UserDatabases />} />
          <Route path="/billing" element={<UserBilling />} />
          <Route path="/domains" element={<UserDomains />} />
          <Route path="/projects/:uid/database" element={<DatabaseManager />} />
          <Route path="/feedback" element={<UserFeedback />} />
          <Route path="/settings" element={<UserSettings />} />
          <Route path="/secretstores" element={<UserSecretStore />} />
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
          <Route path="domains" element={<AdminDomains />} />
          <Route path="queue" element={<AdminDeploymentQueue />} />
          <Route path="secretstores" element={<AdminSecretStore />} />
          <Route path="billing" element={<AdminBilling />} />
        </Route>

        {/* Fallback */}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Suspense>
    </>
  )
}

export default App
