// ===========================================
// API Service
// ===========================================
// Centralized API calls with axios
// ===========================================

import axios, { InternalAxiosRequestConfig } from 'axios'

// Create axios instance
const api = axios.create({
  baseURL: '/api',
  headers: {
    'Content-Type': 'application/json',
  },
})

// Request interceptor - add auth token
api.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const token = localStorage.getItem('token')
  if (token && config.headers) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// Response interceptor - handle errors
api.interceptors.response.use(
  (response) => response,
  (error) => {
    const wasAuthenticated = !!error.config?.headers?.Authorization
    if (error.response?.status === 401 && wasAuthenticated) {
      localStorage.removeItem('token')
      window.dispatchEvent(new Event('auth:expired'))
    }
    return Promise.reject(error)
  }
)

// ===========================================
// Auth API
// ===========================================

export const authAPI = {
  login: (email: string, password: string) => 
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
  list: (params: Record<string, any> = {}) => 
    api.get('/admin/users', { params }),
  
  get: (id: number | string) => 
    api.get(`/admin/users/${id}`),
  
  create: (data: any) => 
    api.post('/admin/users', data),
  
  update: (id: number | string, data: any) => 
    api.put(`/admin/users/${id}`, data),
  
  delete: (id: number | string) => 
    api.delete(`/admin/users/${id}`),
  
  importExcel: (file: File) => {
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
  
  update: (settings: any) => 
    api.put('/admin/settings', { settings }),
}

// ===========================================
// Projects API
// ===========================================

export const projectsAPI = {
  // Student endpoints
  listOwn: () => 
    api.get('/projects'),
  
  create: (data: any) => 
    api.post('/projects', data),
  
  get: (id: number | string) => 
    api.get(`/projects/${id}`),
  
  redeploy: (id: number | string) => 
    api.post(`/projects/${id}/redeploy`),
  
  update: (id: number | string, data: any) =>
    api.put(`/projects/${id}`, data),
  
  delete: (id: number | string) => 
    api.delete(`/projects/${id}`),
  
  logs: (id: number | string, lines: number = 100) => 
    api.get(`/projects/${id}/logs`, { params: { lines } }),
  
  stats: (id: number | string) => 
    api.get(`/projects/${id}/stats`),

  runArtisan: (id: number | string, command: string) =>
    api.post(`/projects/${id}/artisan`, { command }),

  getEnv: (id: number | string) =>
    api.get(`/projects/${id}/env`),

  updateEnv: (id: number | string, content: string) =>
    api.put(`/projects/${id}/env`, { content }),
  
  // Admin endpoints
  listAll: (params: Record<string, any> = {}) => 
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
  getCredentials: (projectId: number | string) => 
    api.get(`/projects/${projectId}/database/credentials`),
  
  // List all tables
  listTables: (projectId: number | string) => 
    api.get(`/projects/${projectId}/database/tables`),
  
  // Get table structure (columns)
  getStructure: (projectId: number | string, tableName: string) => 
    api.get(`/projects/${projectId}/database/tables/${tableName}`),
  
  // Get table data with pagination
  getData: (projectId: number | string, tableName: string, page: number = 1, limit: number = 50) => 
    api.get(`/projects/${projectId}/database/tables/${tableName}/data`, { 
      params: { page, limit } 
    }),
  
  // Execute SQL query
  query: (projectId: number | string, sql: string) => 
    api.post(`/projects/${projectId}/database/query`, { query: sql }),
  
  // Export database as SQL file
  export: (projectId: number | string) => 
    api.get(`/projects/${projectId}/database/export`, { responseType: 'blob' }),
  
  // Import SQL
  import: (projectId: number | string, sql: string) => 
    api.post(`/projects/${projectId}/database/import`, { sql }),
  
  // Reset database (drop all tables)
  reset: (projectId: number | string) => 
    api.post(`/projects/${projectId}/database/reset`),
}

// ===========================================
// Feedback API
// ===========================================

export const feedbackAPI = {
  submit: (data: any) => 
    api.post('/feedback', data),
  
  listOwn: () => 
    api.get('/feedback'),
  
  // Admin endpoints
  listAll: (params: Record<string, any> = {}) => 
    api.get('/admin/feedback', { params }),
  
  updateStatus: (id: number | string, status: string) => 
    api.put(`/admin/feedback/${id}/status`, { status }),
  
  delete: (id: number | string) => 
    api.delete(`/admin/feedback/${id}`),
}

export const systemAPI = {
  getStats: () => 
    api.get('/admin/system/stats'),
  
  prune: () => 
    api.post('/admin/system/prune'),
  
  getInitStatus: () => 
    api.get('/system/init-status'),
  
  initialize: (data: any) => 
    api.post('/system/initialize', data),
}

export default api
