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
  loginAsClient: (newToken: string) => Promise<void>;
  returnToAdmin: () => Promise<void>;
}

const useAuthStore = create<AuthState>((set, get) => ({
  // State
  user: null,
  token: localStorage.getItem('token'),
  adminToken: localStorage.getItem('admin_token'),
  isLoading: !!localStorage.getItem('token'),
  
  // Computed
  isAuthenticated: () => !!get().token,
  isAdmin: () => {
    const user = get().user
    return user?.role === 'superadmin' || user?.role === 'admin'
  },
  isSuperAdmin: () => get().user?.role === 'superadmin',
  
  // Actions
  login: async (email, password) => {
    const response = await authAPI.login(email, password)
    const { token, user } = response.data
    
    localStorage.setItem('token', token)
    set({ token, user })
    
    return user
  },
  
  logout: async () => {
    try {
      await authAPI.logout()
    } catch (error) {
      // Ignored
    }
    
    localStorage.removeItem('token')
    set({ token: null, user: null })
  },
  
  fetchUser: async () => {
    const token = get().token
    if (!token) {
      set({ isLoading: false })
      return
    }

    set({ isLoading: true })
    try {
      const response = await authAPI.me()
      set({ user: response.data, isLoading: false })
    } catch (error: unknown) {
      const axiosError = error as AxiosError
      const status = axiosError?.response?.status
      if (status === 401 || status === 403) {
        localStorage.removeItem('token')
        set({ token: null, user: null, isLoading: false })
      } else {
        set({ isLoading: false })
      }
    }
  },
  
  loginAsClient: async (newToken: string) => {
    const currentToken = get().token
    if (currentToken && !get().adminToken) {
      localStorage.setItem('admin_token', currentToken)
      set({ adminToken: currentToken })
    }
    
    localStorage.setItem('token', newToken)
    set({ token: newToken })
    await get().fetchUser()
  },
  
  returnToAdmin: async () => {
    const adminToken = get().adminToken
    if (adminToken) {
      localStorage.setItem('token', adminToken)
      localStorage.removeItem('admin_token')
      set({ token: adminToken, adminToken: null })
      await get().fetchUser()
    }
  },
}))

export default useAuthStore
