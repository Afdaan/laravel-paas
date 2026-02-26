// ===========================================
// Main App Component
// ===========================================
// Handles routing and layout
// ===========================================

import { useEffect, lazy, Suspense } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import useAuthStore from './stores/authStore'

// Layouts
import DashboardLayout from './components/DashboardLayout'
import LoadingScreen from './components/LoadingScreen'

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

// Protected Route Component
function ProtectedRoute({ children, requireAdmin = false }) {
  const { token, user, isLoading } = useAuthStore()
  const isAdmin = user?.role === 'superadmin' || user?.role === 'admin'
  
  if (isLoading) {
    return <LoadingScreen />
  }
  
  if (!token) {
    return <Navigate to="/login" replace />
  }
  
  if (requireAdmin && !isAdmin) {
    return <Navigate to="/dashboard" replace />
  }
  
  return children
}

function App() {
  const { fetchUser, token, user } = useAuthStore()
  const logout = useAuthStore((state) => state.logout)

  useEffect(() => {
    if (token && !user) {
      fetchUser()
    }
  }, [])

  useEffect(() => {
    const handleExpired = () => {
      useAuthStore.setState({ token: null, user: null, isLoading: false })
    }
    window.addEventListener('auth:expired', handleExpired)
    return () => window.removeEventListener('auth:expired', handleExpired)
  }, [])
  
  return (
    <Suspense fallback={<LoadingScreen />}>
      <Routes>
        {/* Public Routes */}
        <Route path="/" element={<Landing />} />
        <Route path="/login" element={<Login />} />
        
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
        </Route>
        
        {/* Fallback */}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Suspense>
  )
}

export default App
