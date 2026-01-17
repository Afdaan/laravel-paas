// ===========================================
// API Service
// ===========================================
// Centralized API calls with axios
// ===========================================

import axios from 'axios'

// Create axios instance
const api = axios.create({
  baseURL: '/api',
  headers: {
    'Content-Type': 'application/json',
  },
})

// Request interceptor - add auth token
api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// Response interceptor - handle errors
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response?.status === 401) {
      localStorage.removeItem('token')
      window.location.href = '/login'
    }
    return Promise.reject(error)
  }
)

// ===========================================
// Auth API
// ===========================================

export const authAPI = {
  login: (email, password) => 
    api.post('/auth/login', { email, password }),
  
  logout: () => 
    api.post('/auth/logout'),
  
  me: () => 
    api.get('/auth/me'),
}

// ===========================================
// Users API (Admin)
// ===========================================

export const usersAPI = {
  list: (params = {}) => 
    api.get('/admin/users', { params }),
  
  get: (id) => 
    api.get(`/admin/users/${id}`),
  
  create: (data) => 
    api.post('/admin/users', data),
  
  update: (id, data) => 
    api.put(`/admin/users/${id}`, data),
  
  delete: (id) => 
    api.delete(`/admin/users/${id}`),
  
  importExcel: (file) => {
    const formData = new FormData()
    formData.append('file', file)
    return api.post('/admin/users/import', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
  },
}

// ===========================================
// Settings API (Admin)
// ===========================================

export const settingsAPI = {
  list: () => 
    api.get('/admin/settings'),
  
  update: (settings) => 
    api.put('/admin/settings', { settings }),
}

// ===========================================
// Projects API
// ===========================================

export const projectsAPI = {
  // Student endpoints
  listOwn: () => 
    api.get('/projects'),
  
  create: (data) => 
    api.post('/projects', data),
  
  get: (id) => 
    api.get(`/projects/${id}`),
  
  redeploy: (id) => 
    api.post(`/projects/${id}/redeploy`),
  
  update: (id, data) =>
    api.put(`/projects/${id}`, data),
  
  delete: (id) => 
    api.delete(`/projects/${id}`),
  
  logs: (id, lines = 100) => 
    api.get(`/projects/${id}/logs`, { params: { lines } }),
  
  stats: (id) => 
    api.get(`/projects/${id}/stats`),

  runArtisan: (id, command) =>
    api.post(`/projects/${id}/artisan`, { command }),

  getEnv: (id) =>
    api.get(`/projects/${id}/env`),

  updateEnv: (id, content) =>
    api.put(`/projects/${id}/env`, { content }),
  
  // Admin endpoints
  listAll: (params = {}) => 
    api.get('/admin/projects', { params }),

  listStats: () => 
    api.get('/admin/projects/stats'),
  
  adminStats: () => 
    api.get('/admin/stats'),
}

// ===========================================
// Database API (Student Database Management)
// ===========================================

export const databaseAPI = {
  // Get database credentials
  getCredentials: (projectId) => 
    api.get(`/projects/${projectId}/database/credentials`),
  
  // List all tables
  listTables: (projectId) => 
    api.get(`/projects/${projectId}/database/tables`),
  
  // Get table structure (columns)
  getStructure: (projectId, tableName) => 
    api.get(`/projects/${projectId}/database/tables/${tableName}`),
  
  // Get table data with pagination
  getData: (projectId, tableName, page = 1, limit = 50) => 
    api.get(`/projects/${projectId}/database/tables/${tableName}/data`, { 
      params: { page, limit } 
    }),
  
  // Execute SQL query
  query: (projectId, sql) => 
    api.post(`/projects/${projectId}/database/query`, { query: sql }),
  
  // Export database as SQL file
  export: (projectId) => 
    api.get(`/projects/${projectId}/database/export`, { responseType: 'blob' }),
  
  // Import SQL
  import: (projectId, sql) => 
    api.post(`/projects/${projectId}/database/import`, { sql }),
  
  // Reset database (drop all tables)
  reset: (projectId) => 
    api.post(`/projects/${projectId}/database/reset`),
}

export default api
