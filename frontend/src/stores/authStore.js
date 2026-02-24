// ===========================================
// Auth Store (Zustand)
// ===========================================
// Global auth state management
// ===========================================

import { create } from 'zustand'
import { authAPI } from '../services/api'

const useAuthStore = create((set, get) => ({
  // State
  user: null,
  token: localStorage.getItem('token'),
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
    } catch (error) {
      const status = error?.response?.status
      if (status === 401 || status === 403) {
        localStorage.removeItem('token')
        set({ token: null, user: null, isLoading: false })
      } else {
        set({ isLoading: false })
      }
    }
  },
}))

export default useAuthStore
