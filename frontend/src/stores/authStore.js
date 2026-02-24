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
      // Ignore logout errors
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
    
    try {
      const response = await authAPI.me()
      set({ user: response.data, isLoading: false })
    } catch (error) {
      localStorage.removeItem('token')
      set({ token: null, user: null, isLoading: false })
    }
  },
}))

export default useAuthStore
