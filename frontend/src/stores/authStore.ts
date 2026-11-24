// ===========================================
// Auth Store (Zustand)
// ===========================================
// Global auth state management
// ===========================================

import { create } from 'zustand'
import { AxiosError } from 'axios'
import { authAPI } from '../services/api'
import { User } from '../types'

interface AuthState {
  user: User | null;
  token: string | null;
  adminToken: string | null;
  isLoading: boolean;
  isAuthenticated: () => boolean;
  isAdmin: () => boolean;
  isSuperAdmin: () => boolean;
  login: (email: string, password: string) => Promise<User>;
  logout: () => Promise<void>;
  fetchUser: () => Promise<void>;
  expireSession: () => void;
  loginAsClient: () => Promise<void>;
  returnToAdmin: () => Promise<void>;
}

const SESSION_MARKER = 'authenticated'
const IMPERSONATION_MARKER = 'impersonating'

let fetchUserPromise: Promise<void> | null = null
let sessionEpoch = 0

// Any session transition (login/logout/impersonate/expire) invalidates
// in-flight fetchUser writes so stale responses cannot overwrite newer state.
const invalidatePendingFetches = () => {
  sessionEpoch++
  fetchUserPromise = null
}

const useAuthStore = create<AuthState>((set, get) => ({
  // State
  user: null,
  token: null,
  adminToken: null,
  isLoading: true,

  // Computed
  isAuthenticated: () => !!get().token,
  isAdmin: () => {
    const user = get().user
    return user?.role === 'superadmin' || user?.role === 'admin'
  },
  isSuperAdmin: () => get().user?.role === 'superadmin',

  // Actions
  login: async (email, password) => {
    invalidatePendingFetches()
    const response = await authAPI.login(email, password)
    const { user } = response.data

    set({ token: SESSION_MARKER, user, adminToken: null, isLoading: false })

    return user
  },

  logout: async () => {
    invalidatePendingFetches()
    try {
      await authAPI.logout()
    } catch (error) {
      // Ignored
    }

    set({ token: null, user: null, adminToken: null, isLoading: false })
  },

  fetchUser: async () => {
    if (fetchUserPromise) return fetchUserPromise
    const epoch = sessionEpoch
    const isStale = () => epoch !== sessionEpoch
    fetchUserPromise = (async () => {
      set({ isLoading: true })
      try {
        const response = await authAPI.me()
        if (isStale()) return
        const isImpersonating = response.headers['x-impersonating'] === 'true'
        set({
          token: SESSION_MARKER,
          adminToken: isImpersonating ? IMPERSONATION_MARKER : null,
          user: response.data,
          isLoading: false,
        })
      } catch (error: unknown) {
        const axiosError = error as AxiosError
        const status = axiosError?.response?.status
        if (isStale()) return
        if (status === 401 || status === 403) {
          set({ token: null, user: null, adminToken: null, isLoading: false })
        } else {
          set({ isLoading: false })
        }
      }
    })().finally(() => {
      fetchUserPromise = null
    })
    return fetchUserPromise
  },

  expireSession: () => {
    invalidatePendingFetches()
    set({ token: null, user: null, adminToken: null, isLoading: false })
  },

  loginAsClient: async () => {
    invalidatePendingFetches()
    set({ token: SESSION_MARKER, adminToken: IMPERSONATION_MARKER })
    await get().fetchUser()
  },

  returnToAdmin: async () => {
    await authAPI.returnToAdmin()
    invalidatePendingFetches()
    set({ token: SESSION_MARKER, adminToken: null })
    await get().fetchUser()
  },
}))

export default useAuthStore
