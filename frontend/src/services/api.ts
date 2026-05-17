// ===========================================
// API Service
// ===========================================
// Centralized API calls with axios
// ===========================================

import axios, { InternalAxiosRequestConfig, AxiosError } from 'axios'

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
  async (error: AxiosError) => {
    const { config, response } = error
    if (!config) return Promise.reject(error)
    
    const wasAuthenticated = !!localStorage.getItem('token')
    
    // Global 401 Unauthorized handling
    if (response?.status === 401 && wasAuthenticated) {
      localStorage.removeItem('token')
      window.dispatchEvent(new Event('auth:expired'))
    }
    
    // Global Server Updating/Swap handling (502 Bad Gateway / 503 Service Unavailable)
    if (response?.status === 502 || response?.status === 503) {
      window.dispatchEvent(new Event('system:updating'))
      
      // Auto-Retry Logic: Retry up to 3 times if it's a transient server error
      const retryConfig = config as InternalAxiosRequestConfig & { _retryCount?: number }
      retryConfig._retryCount = retryConfig._retryCount || 0
      if (retryConfig._retryCount < 3) {
        retryConfig._retryCount++
        console.warn(`System swapping detected (HTTP ${response.status}). Retrying request... (${retryConfig._retryCount}/3)`)
        
        // Wait 1.5s before retrying to give backend time to swap
        await new Promise(resolve => setTimeout(resolve, 1500))
        return api(retryConfig)
      }
    }
    
    // Global connection error handling (No response received)
    if (!error.response && !error.request?.status) {
      window.dispatchEvent(new Event('system:offline'))
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
  list: (params: Record<string, unknown> = {}) => 
    api.get('/admin/users', { params }),
  
  get: (id: number | string) => 
    api.get(`/admin/users/${id}`),
  
  create: (data: unknown) => 
    api.post('/admin/users', data),
  
  update: (id: number | string, data: unknown) => 
    api.put(`/admin/users/${id}`, data),
  
  delete: (id: number | string) => 
    api.delete(`/admin/users/${id}`),

  loginAs: (id: number | string) =>
    api.post(`/admin/users/${id}/login-as`),
  
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
  
  update: (settings: unknown) => 
    api.put('/admin/settings', { settings }),
}

// ===========================================
// Projects API
// ===========================================


export const projectsAPI = {
  // Student endpoints
  listOwn: () => 
    api.get('/projects'),
  
  create: (data: unknown) => 
    api.post('/projects', data),
  
  get: (id: number | string) => 
    api.get(`/projects/${id}`),
  
  redeploy: (id: number | string) => 
    api.post(`/projects/${id}/redeploy`),
  
  stop: (id: number | string) =>
    api.post(`/projects/${id}/stop`),
  
  start: (id: number | string) =>
    api.post(`/projects/${id}/start`),
  
  restart: (id: number | string) =>
    api.post(`/projects/${id}/restart`),
  
  update: (id: number | string, data: unknown) =>
    api.put(`/projects/${id}`, data),
  
  delete: (id: number | string) => 
    api.delete(`/projects/${id}`),
  
  logs: (id: number | string, lines: number = 100, type: string = 'web') => 
    api.get(`/projects/${id}/logs`, { params: { lines, type } }),
  
  stats: (id: number | string) => 
    api.get(`/projects/${id}/stats`),

  runArtisan: (id: number | string, command: string) =>
    api.post(`/projects/${id}/artisan`, { command }),

  getEnv: (id: number | string) =>
    api.get(`/projects/${id}/env`),

  updateEnv: (id: number | string, content: string) =>
    api.put(`/projects/${id}/env`, { content }),
  
  buildLogs: (id: number | string) =>
    api.get(`/projects/${id}/build-logs`),
  
  getDeploymentEvents: (id: number | string) =>
    api.get(`/projects/${id}/deployment-events`),
  
  // Custom Domain endpoints
  listDomains: (id: number | string) =>
    api.get(`/projects/${id}/domains`),

  addDomain: (id: number | string, domain: string) =>
    api.post(`/projects/${id}/domains`, { domain }),

  removeDomain: (id: number | string, domainId: number | string) =>
    api.delete(`/projects/${id}/domains/${domainId}`),

  verifyDomain: (id: number | string, domainId: number | string) =>
    api.post(`/projects/${id}/domains/${domainId}/verify`),
  
  getDomainDiagnostic: (id: number | string, domainId: number | string) =>
    api.get(`/projects/${id}/domains/${domainId}/diagnostic`),
  
  // Admin endpoints
  listAll: (params: Record<string, unknown> = {}) => 
    api.get('/admin/projects', { params }),

  listStats: () => 
    api.get('/admin/projects/stats'),
  
  adminStats: () => 
    api.get('/admin/stats'),

  getQueueStats: () => 
    api.get('/admin/queue/stats'),
  
  cancelQueueJob: (id: number | string) =>
    api.post(`/admin/queue/cancel/${id}`),

  requeueJob: (id: number | string) =>
    api.post(`/admin/queue/requeue/${id}`),
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
  
  // Delete row securely using primary key
  deleteRow: (projectId: number | string, tableName: string, primaryKey: string, value: unknown) => 
    api.delete(`/projects/${projectId}/database/tables/${tableName}/rows`, { 
      data: { primary_key: primaryKey, value } 
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

  // Admin endpoints
  adminListAll: () => 
    api.get('/admin/databases'),
}

// ===========================================
// Feedback API
// ===========================================

export const feedbackAPI = {
  submit: (data: unknown) => 
    api.post('/feedback', data),
  
  listOwn: () => 
    api.get('/feedback'),
  
  // Admin endpoints
  listAll: (params: Record<string, unknown> = {}) => 
    api.get('/admin/feedback', { params }),
  
  updateStatus: (id: number | string, status: string) => 
    api.put(`/admin/feedback/${id}/status`, { status }),
  
  delete: (id: number | string) => 
    api.delete(`/admin/feedback/${id}`),
}

export const domainsAPI = {
  listOwn: () =>
    api.get('/domains'),

  listAll: () =>
    api.get('/admin/domains'),

  transfer: (domainId: number | string, targetProjectId: number | string) =>
    api.post(`/projects/0/domains/${domainId}/transfer`, { target_project_id: targetProjectId }),
}

export const systemAPI = {
  getStats: () => 
    api.get('/admin/system/stats'),
  
  prune: () => 
    api.post('/admin/system/prune'),

  deleteVolume: (name: string) =>
    api.delete(`/admin/system/volumes/${name}`),
  
  getInitStatus: () => 
    api.get('/system/init-status'),
  
  initialize: (data: unknown) => 
    api.post('/system/initialize', data),
}

export default api
