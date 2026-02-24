// ===========================================
// Main App Component
// ===========================================
// Handles routing and layout
// ===========================================

import { useEffect } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import useAuthStore from './stores/authStore'

// Layouts
import DashboardLayout from './components/DashboardLayout'
import LoadingScreen from './components/LoadingScreen'

// Pages
import Landing from './pages/Landing'
import Login from './pages/Login'
import StudentDashboard from './pages/student/Dashboard'
import StudentProjects from './pages/student/Projects'
import StudentNewProject from './pages/student/NewProject'
import StudentProjectDetail from './pages/student/ProjectDetail'
import DatabaseManager from './pages/student/DatabaseManager'
import AdminDashboard from './pages/admin/Dashboard'
import AdminUsers from './pages/admin/Users'
import AdminProjects from './pages/admin/Projects'
import AdminSettings from './pages/admin/Settings'
import StudentDatabases from './pages/student/Databases'

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
  const { fetchUser, token } = useAuthStore()
  
  useEffect(() => {
    if (token) {
      fetchUser()
    }
  }, [token, fetchUser])
  
  return (
    <Routes>
      {/* Public Routes */}
      <Route path="/" element={<Landing />} />
      <Route path="/login" element={<Login />} />
      
      {/* Student Routes */}
      <Route path="/" element={
        <ProtectedRoute>
          <DashboardLayout />
        </ProtectedRoute>
      }>
        <Route path="dashboard" element={<StudentDashboard />} />
        <Route path="projects" element={<StudentProjects />} />
        <Route path="projects/new" element={<StudentNewProject />} />
        <Route path="projects/:id" element={<StudentProjectDetail />} />
        <Route path="databases" element={<StudentDatabases />} />
        <Route path="projects/:id/database" element={<DatabaseManager />} />
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
        <Route path="settings" element={<AdminSettings />} />
      </Route>
      
      {/* Fallback */}
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

export default App
