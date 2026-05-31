import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { Toaster } from './components/ui/sonner'
import { toast } from 'sonner'
import App from './App'
import ErrorBoundary from './components/ErrorBoundary'
import { ThemeProvider } from './components/ThemeProvider'
import './index.css'

// Wrap toast functions globally to prevent duplicate/spam notifications
const originalToast = {
  success: toast.success.bind(toast),
  error: toast.error.bind(toast),
  info: toast.info.bind(toast),
  warning: toast.warning.bind(toast)
}

const activeToasts = new Map<string, { id: string | number; timestamp: number; timeoutId: any }>()

const preventSpam = (message: any, originalFn: Function, options: any = {}) => {
  const msgStr = typeof message === 'string' ? message : String(message || '')
  const now = Date.now()
  const existing = activeToasts.get(msgStr)

  if (existing) {
    clearTimeout(existing.timeoutId)

    // If the exact same message was triggered within 3.5 seconds, update the existing toast
    if (now - existing.timestamp < 3500) {
      const id = originalFn(message, { ...options, id: existing.id })
      const timeoutId = setTimeout(() => {
        activeToasts.delete(msgStr)
      }, 4000)
      activeToasts.set(msgStr, { id, timestamp: now, timeoutId })
      return id
    }
  }

  const id = originalFn(message, options)
  const timeoutId = setTimeout(() => {
    activeToasts.delete(msgStr)
  }, 4000)
  activeToasts.set(msgStr, { id, timestamp: now, timeoutId })

  return id
}

toast.success = (message: any, options: any) => preventSpam(message, originalToast.success, options)
toast.error = (message: any, options: any) => preventSpam(message, originalToast.error, options)
toast.info = (message: any, options: any) => preventSpam(message, originalToast.info, options)
toast.warning = (message: any, options: any) => preventSpam(message, originalToast.warning, options)

const root = document.getElementById('root') as HTMLElement

ReactDOM.createRoot(root).render(
  <React.StrictMode>
    <BrowserRouter>
      <ErrorBoundary>
        <ThemeProvider defaultTheme="dark" storageKey="vite-ui-theme">
          <App />
          <Toaster 
            position="top-center"
            expand={false}
            visibleToasts={3}
          />
        </ThemeProvider>
      </ErrorBoundary>
    </BrowserRouter>
  </React.StrictMode>
)
