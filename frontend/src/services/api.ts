// ===========================================
// API Service
// ===========================================
// Centralized API calls with axios
// ===========================================

import axios, { InternalAxiosRequestConfig, AxiosError, AxiosRequestConfig } from 'axios'
import type { BillingOverview, BillingStatus, TopupPackage, BillingCatalogSpec } from '@/types'

// Create axios instance
const api = axios.create({
  baseURL: '/api',
  withCredentials: true,
  headers: {
    'Content-Type': 'application/json',
  },
})

export const getCSRFToken = () => {
  if (typeof document === 'undefined' || !document.cookie) return ''
  const cookies = document.cookie.split(';').map((c) => c.trim())
  const prodPrefix = '__Host-paas_csrf='
  const devPrefix = 'paas_csrf='
  const targetPrefix = import.meta.env.PROD ? prodPrefix : devPrefix

  const exactMatch = cookies.find((row) => row.startsWith(targetPrefix))
  if (exactMatch) {
    const val = exactMatch.substring(targetPrefix.length)
    return val ? decodeURIComponent(val) : ''
  }
  const fallbackPrefix = import.meta.env.PROD ? devPrefix : prodPrefix
  const fallbackMatch = cookies.find((row) => row.startsWith(fallbackPrefix))
  if (fallbackMatch) {
    const val = fallbackMatch.substring(fallbackPrefix.length)
    return val ? decodeURIComponent(val) : ''
  }
  return ''
}

// Request interceptor - add CSRF token for cookie-authenticated unsafe requests
api.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const method = (config.method || 'get').toUpperCase()
  if (!['GET', 'HEAD', 'OPTIONS'].includes(method) && config.headers) {
    const csrfToken = getCSRFToken()
    if (csrfToken) {
      if (typeof (config.headers as any).set === 'function') {
        (config.headers as any).set('X-CSRF-Token', csrfToken)
      } else {
        config.headers['X-CSRF-Token'] = csrfToken
      }
    }
  }
  return config
})

export function isSessionExpiredError(
  status: number | undefined,
  errorData: unknown,
  url: string | undefined
): boolean {
  if (status !== 401) return false

  let code: string | undefined
  if (typeof errorData === 'object' && errorData !== null) {
    code = (errorData as { code?: string }).code
  } else if (typeof errorData === 'string') {
    try {
      const parsed = JSON.parse(errorData)
      code = parsed?.code
    } catch {
      // not JSON
    }
  }

  // If wrong password / credentials during re-auth, login, or registration, session is not expired
  const isAuthRoute =
    url?.endsWith('/auth/re-auth') ||
    url?.endsWith('/auth/login') ||
    url?.endsWith('/auth/register')

  if (isAuthRoute && (code === 'AUTH_FAILED' || code === 'INVALID_CREDENTIALS')) {
    return false
  }

  return true
}

// Response interceptor - handle errors
api.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const { config, response } = error
    if (!config) return Promise.reject(error)

    // Global 401 Unauthorized handling (skip login/re-auth credential failure AUTH_FAILED)
    if (isSessionExpiredError(response?.status, response?.data, config.url)) {
      if (!window.location.pathname.startsWith('/login') && !window.location.pathname.startsWith('/setup')) {
        window.dispatchEvent(new Event('auth:expired'))
      }
    }

    // Global 403 handling (RECENT_AUTH_REQUIRED vs CSRF_FAILED / IMPERSONATION_FORBIDDEN / FORBIDDEN)
    const status = response?.status
    const errorData = response?.data as { code?: string; message?: string; error?: string } | string | undefined

    const isAuthRoute =
      config.url?.endsWith('/auth/re-auth') ||
      config.url?.endsWith('/auth/login') ||
      config.url?.endsWith('/auth/logout')

    const isAlreadyRetried = Boolean((config as any)._isReauthRetry)

    let hasRecentAuthCode = false
    if (typeof errorData === 'object' && errorData !== null) {
      hasRecentAuthCode = errorData.code === 'RECENT_AUTH_REQUIRED'
    } else if (typeof errorData === 'string') {
      try {
        const parsed = JSON.parse(errorData)
        hasRecentAuthCode = parsed?.code === 'RECENT_AUTH_REQUIRED'
      } catch {
        hasRecentAuthCode = errorData.includes('RECENT_AUTH_REQUIRED')
      }
    }

    const isRecentAuthRequired =
      status === 403 &&
      !isAuthRoute &&
      !isAlreadyRetried &&
      hasRecentAuthCode

    if (isRecentAuthRequired) {
      const retryConfig = { ...config } as InternalAxiosRequestConfig & { _isReauthRetry?: boolean }
      retryConfig._isReauthRetry = true

      return new Promise((resolve, reject) => {
        let settled = false

        const cleanup = () => {
          window.removeEventListener('auth:reauthenticated', handleReauthSuccess)
          window.removeEventListener('auth:reauth_cancelled', handleReauthCancelled)
          window.removeEventListener('auth:expired', handleAuthExpired)
        }

        const handleReauthSuccess = async () => {
          if (settled) return
          settled = true
          cleanup()
          try {
            const freshCsrf = getCSRFToken()
            if (freshCsrf && retryConfig.headers) {
              if (typeof (retryConfig.headers as any).set === 'function') {
                (retryConfig.headers as any).set('X-CSRF-Token', freshCsrf)
              } else {
                retryConfig.headers['X-CSRF-Token'] = freshCsrf
              }
            }
            const retryRes = await api(retryConfig)
            resolve(retryRes)
          } catch (retryErr) {
            reject(retryErr)
          }
        }

        const handleReauthCancelled = () => {
          if (settled) return
          settled = true
          cleanup()
          reject(error)
        }

        const handleAuthExpired = () => {
          if (settled) return
          settled = true
          cleanup()
          reject(error)
        }

        window.addEventListener('auth:reauthenticated', handleReauthSuccess)
        window.addEventListener('auth:reauth_cancelled', handleReauthCancelled)
        window.addEventListener('auth:expired', handleAuthExpired)

        window.dispatchEvent(new Event('auth:recent_auth_required'))
      })
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
    if (!error.response && !error.request?.status && !axios.isCancel(error)) {
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

  returnToAdmin: () =>
    api.post('/auth/return-to-admin'),

  updateProfile: (data: { name: string; email: string; password?: string }) =>
    api.put('/auth/profile', data),

  reauthenticate: (password: string) =>
    api.post('/auth/re-auth', { password }),
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
  // User endpoints
  listOwn: () =>
    api.get('/projects'),

  create: (data: unknown) =>
    api.post('/projects', data),

  get: (id: number | string) =>
    api.get(`/projects/${id}`),

  redeploy: (id: number | string, clean?: boolean) =>
    api.post(`/projects/${id}/redeploy`, null, { params: { clean: clean ? 'true' : 'false' } }),

  stop: (id: number | string) =>
    api.post(`/projects/${id}/stop`),

  start: (id: number | string) =>
    api.post(`/projects/${id}/start`),

  restart: (id: number | string) =>
    api.post(`/projects/${id}/restart`),

  rollback: (id: number | string, commitSHA: string) =>
    api.post(`/projects/${id}/rollback`, { commit_sha: commitSHA }),

  update: (id: number | string, data: unknown) =>
    api.put(`/projects/${id}`, data),

  delete: (id: number | string) =>
    api.delete(`/projects/${id}`),

  logs: (id: number | string, lines: number = 100, type: string = 'web') =>
    api.get(`/projects/${id}/logs`, { params: { lines, type } }),

  stats: (id: number | string) =>
    api.get(`/projects/${id}/stats`),

  runConsoleCommand: (id: number | string, command: string) =>
    api.post(`/projects/${id}/console`, { command }),

  getEnv: (id: number | string) =>
    api.get(`/projects/${id}/env`),

  updateEnv: (id: number | string, content: string) =>
    api.put(`/projects/${id}/env`, { content }),

  buildLogs: (id: number | string) =>
    api.get(`/projects/${id}/build-logs`),

  getDeploymentEvents: (id: number | string, all = false, options?: AxiosRequestConfig) =>
    api.get(`/projects/${id}/deployment-events`, { params: { all }, ...options }),

  listBranches: (id: number | string) =>
    api.get(`/projects/${id}/branches`),

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

  getDomainEvents: (id: number | string, domainId: number | string) =>
    api.get(`/projects/${id}/domains/${domainId}/events`),

  // Admin endpoints
  listAll: (params: Record<string, unknown> = {}) =>
    api.get('/admin/projects', { params }),

  listStats: (options?: AxiosRequestConfig) =>
    api.get('/admin/projects/stats', options),

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
// Database API (User Database Management)
// ===========================================

export const databaseAPI = {
  // Get database credentials
  getCredentials: (projectId: number | string) =>
    api.post(`/projects/${projectId}/database/credentials`),

  // Rotate credentials
  rotateCredentials: (projectId: number | string) =>
    api.post(`/projects/${projectId}/database/rotate-credentials`),


  // Suspend or resume database status
  updateStatus: (projectId: number | string, suspend: boolean) =>
    api.post(`/projects/${projectId}/database/status`, { suspend }),

  // Get overview statistics
  getOverview: (projectId: number | string) =>
    api.get(`/projects/${projectId}/database/overview`),

  // Get visual schema metadata
  getSchema: (projectId: number | string) =>
    api.get(`/projects/${projectId}/database/schema`),

  // Execute Table Designer Action
  executeDesigner: (projectId: number | string, data: unknown) =>
    api.post(`/projects/${projectId}/database/designer`, data),

  // List backup snapshots
  listBackups: (projectId: number | string) =>
    api.get(`/projects/${projectId}/database/backups`),

  // Create manual backup snapshot
  createBackup: (projectId: number | string) =>
    api.post(`/projects/${projectId}/database/backups`),

  // Restore backup state
  restoreBackup: (projectId: number | string, backupId: number | string) =>
    api.post(`/projects/${projectId}/database/backups/${backupId}/restore`),

  // Prune backup snapshot
  deleteBackup: (projectId: number | string, backupId: number | string) =>
    api.delete(`/projects/${projectId}/database/backups/${backupId}`),

  // Download backup snapshot file
  downloadBackup: (projectId: number | string, backupId: number | string) =>
    api.get(`/projects/${projectId}/database/backups/${backupId}/download`, { responseType: 'blob' }),

  // Get real-time connection metrics
  getMetrics: (projectId: number | string) =>
    api.get(`/projects/${projectId}/database/metrics`),

  // Transfer database ownership to another project
  transfer: (projectId: number | string, targetProjectUid: string) =>
    api.post(`/projects/${projectId}/database/transfer`, { target_project_id: targetProjectUid }),

  // List all tables (Fallback/Legacy)
  listTables: (projectId: number | string) =>
    api.get(`/projects/${projectId}/database/tables`),

  // Get table structure (columns) (Fallback/Legacy)
  getStructure: (projectId: number | string, tableName: string) =>
    api.get(`/projects/${projectId}/database/tables/${tableName}`),

  // Get table data with pagination (Fallback/Legacy)
  getData: (projectId: number | string, tableName: string, page: number = 1, limit: number = 50) =>
    api.get(`/projects/${projectId}/database/tables/${tableName}/data`, {
      params: { page, limit }
    }),

  // Delete row securely using primary key (Fallback/Legacy)
  deleteRow: (projectId: number | string, tableName: string, primaryKey: string, value: unknown) =>
    api.delete(`/projects/${projectId}/database/tables/${tableName}/rows`, {
      data: { primary_key: primaryKey, value }
    }),

  // Update row securely using primary key
  updateRow: (projectId: number | string, tableName: string, primaryKey: string, value: unknown, updates: Record<string, unknown>, config?: import('axios').AxiosRequestConfig) =>
    api.put(`/projects/${projectId}/database/tables/${tableName}/rows`, {
      primary_key: primaryKey, value, updates
    }, config),

  // Execute SQL query (Fallback/Legacy)
  query: (projectId: number | string, sql: string, config?: import('axios').AxiosRequestConfig) =>
    api.post(`/projects/${projectId}/database/query`, { query: sql }, config),

  // Export database as SQL file (Fallback/Legacy)
  export: (projectId: number | string) =>
    api.get(`/projects/${projectId}/database/export`, { responseType: 'blob' }),

  // Import SQL (Fallback/Legacy)
  import: (projectId: number | string, sql: string) =>
    api.post(`/projects/${projectId}/database/import`, { sql }),

  // Reset database (drop all tables) (Fallback/Legacy)
  reset: (projectId: number | string) =>
    api.post(`/projects/${projectId}/database/reset`),

  // Centralized Database Endpoints
  listOwn: () =>
    api.get('/databases'),

  attach: (dbUid: string, projectUid: string) =>
    api.post(`/databases/${dbUid}/attach`, { project_uid: projectUid }),

  detach: (dbUid: string) =>
    api.post(`/databases/${dbUid}/detach`),

  resetInstance: (dbUid: string) =>
    api.post(`/databases/${dbUid}/reset`),

  reinstallInstance: (dbUid: string) =>
    api.post(`/databases/${dbUid}/reinstall`),

  create: (data: { engine: string; name: string; username: string; password: string; billable_spec_id: number }) =>
    api.post('/databases', data),

  delete: (uid: string) =>
    api.delete(`/databases/${uid}`),

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

  transfer: (projectId: number | string, domainId: number | string, targetProjectUid: string) =>
    api.post(`/projects/${projectId}/domains/${domainId}/transfer`, { target_project_uid: targetProjectUid }),
}

export const billingAPI = {
  getProfile: () => api.get('/billing/profile'),
  updateProfile: (data: any) => api.put('/billing/profile', data),
  getUserProfileAdmin: (userID: number) => api.get(`/admin/billing/users/${userID}/billing-profile`),
  catalog: () =>
    api.get<{ specs: BillingCatalogSpec[]; packages: TopupPackage[] }>('/billing/catalog'),

  // Public, no auth required - used by the marketing landing page.
  publicCatalog: () =>
    api.get<{ specs: BillingCatalogSpec[]; packages: TopupPackage[] }>('/public/billing/catalog'),

  overview: () =>
    api.get<BillingOverview>('/billing/overview'),

  invoices: (params: { page?: number; limit?: number; search?: string; status?: string } = {}, config: { signal?: AbortSignal } = {}) =>
    api.get<{ data: BillingOverview['invoices']; page: number; limit: number; total: number }>('/billing/invoices', { params, ...config }),

  topups: (params: { page?: number; limit?: number } = {}) =>
    api.get<{ data: BillingOverview['topups']; page: number; limit: number; total: number }>('/billing/topups', { params }),

  ledger: (params: { page?: number; limit?: number } = {}) =>
    api.get<{ data: BillingOverview['wallet']['ledger_entries']; page: number; limit: number; total: number }>('/billing/wallet/ledger', { params }),

  status: () =>
    api.get<BillingStatus[]>('/billing/status'),

  createTopup: (topupPackageID: number, idempotencyKey: string, amount?: number) =>
    api.post<import('@/types').TopupResponse>('/billing/topups', { topup_package_id: topupPackageID, ...(amount ? { amount } : {}) }, { headers: { 'Idempotency-Key': idempotencyKey } }),

  reconcileTopup: (topupID: number) =>
    api.post<import('@/types').TopupResponse>(`/billing/topups/${topupID}/reconcile`),

  reconcileTopupByRef: (topupRef: string) =>
    api.post<import('@/types').TopupResponse>(`/billing/topups/by-ref/${encodeURIComponent(topupRef)}/reconcile`),

  updateAutoRenew: (resourceId: number, resourceType: 'project' | 'database', autoRenew: boolean) =>
    api.put('/billing/resources/auto-renew', { resource_id: resourceId, resource_type: resourceType, auto_renew: autoRenew }),

  payDueResource: (resourceId: number, resourceType: 'project' | 'database') =>
    api.post(`/billing/resources/${resourceType}/${resourceId}/pay`),

  adminCatalog: () => api.get('/admin/billing/catalog'),
  adminSuspensions: () => api.get('/admin/billing/suspensions'),
  adminWallet: (userID: number) => api.get(`/admin/billing/wallets/${userID}`),
  adminWallets: (params: { page?: number; limit?: number } = {}) => api.get('/admin/billing/wallets', { params }),
  adminInvoices: (params: { page?: number; limit?: number } = {}) => api.get('/admin/billing/invoices', { params }),
  adminTopups: (params: { page?: number; limit?: number } = {}) => api.get('/admin/billing/topups', { params }),
  createSpec: (data: unknown) => api.post('/admin/billing/specs', data),
  createTopupPackage: (data: unknown) => api.post('/admin/billing/topup-packages', data),
  updateTopupPackage: (id: number, data: unknown) => api.put(`/admin/billing/topup-packages/${id}`, data),
  adjustWalletCredits: (userID: number, data: unknown, idempotencyKey: string) =>
    api.post(`/admin/billing/wallets/${userID}/credits`, data, { headers: { 'Idempotency-Key': idempotencyKey } }),
  updatePaymentProvider: (data: { provider: string; reason: string }) =>
    api.put('/admin/billing/payment-provider', data),
}

export const secretStoreAPI = {
  list: () =>
    api.get('/secretstores'),

  get: (id: number | string) =>
    api.get(`/secretstores/${id}`),

  create: (data: { name: string; description?: string }) =>
    api.post('/secretstores', data),

  update: (id: number | string, data: { name: string; description?: string }) =>
    api.put(`/secretstores/${id}`, data),

  delete: (id: number | string) =>
    api.delete(`/secretstores/${id}`),

  // Variable Items inside store
  createItem: (storeId: number | string, data: { key: string; value: string }) =>
    api.post(`/secretstores/${storeId}/items`, data),

  updateItem: (storeId: number | string, itemId: number | string, data: { value: string }) =>
    api.put(`/secretstores/${storeId}/items/${itemId}`, data),

  deleteItem: (storeId: number | string, itemId: number | string) =>
    api.delete(`/secretstores/${storeId}/items/${itemId}`),

  revealItemValue: (storeId: number | string, itemId: number | string, version?: number | string) =>
    api.post(`/secretstores/${storeId}/items/${itemId}/reveal`, null, { params: version ? { version } : {} }),

  getItemHistory: (storeId: number | string, itemId: number | string) =>
    api.get(`/secretstores/${storeId}/items/${itemId}/history`),

  // Bindings
  listBindings: (storeId: number | string) =>
    api.get(`/secretstores/${storeId}/bindings`),

  addBinding: (storeId: number | string, data: { project_uid: string; environment: string }) =>
    api.post(`/secretstores/${storeId}/bindings`, data),

  removeBinding: (storeId: number | string, bindingId: number | string) =>
    api.delete(`/secretstores/${storeId}/bindings/${bindingId}`),

  // Import/Export
  exportStore: (storeId: number | string) =>
    api.post(`/secretstores/${storeId}/export`),

  importStore: (storeId: number | string, data: { secrets: Record<string, string> }) =>
    api.post(`/secretstores/${storeId}/import`, data),

  // Admin audit logs
  adminListAll: () =>
    api.get('/admin/secretstores'),

  adminDisable: (id: number | string, disable: boolean) =>
    api.put(`/admin/secretstores/${id}/disable`, { disable }),

  adminListLogs: () =>
    api.get('/admin/secretstores/logs'),
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

export const githubAPI = {
  listInstallations: () =>
    api.get('/github/installations'),

  linkInstallation: (installationId: number | string) =>
    api.post('/github/installations/link', { installation_id: Number(installationId) }),

  listRepositories: (installationId: number | string) =>
    api.get(`/github/installations/${installationId}/repositories`),

  listBranches: (owner: string, repo: string, installationId?: number | string) =>
    api.get(`/github/repositories/${owner}/${repo}/branches`, {
      params: installationId ? { installation_id: installationId } : undefined,
    }),
}

export default api
