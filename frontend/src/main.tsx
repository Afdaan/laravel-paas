import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { Toaster } from './components/ui/sonner'
import { toast, type ExternalToast } from 'sonner'
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

const activeToasts = new Map<string, { id: string | number; timestamp: number; timeoutId: ReturnType<typeof setTimeout> }>()

type ToastMessage = Parameters<typeof toast.success>[0]

const preventSpam = (
  message: ToastMessage,
  originalFn: (message: ToastMessage, options?: ExternalToast) => string | number,
  options: ExternalToast = {}
) => {
  const msgStr = typeof message === 'string' 
    ? message 
    : (typeof message === 'function' ? 'functional-toast' : String(message || ''))
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

toast.success = (message: ToastMessage, options?: ExternalToast) => preventSpam(message, originalToast.success, options)
toast.error = (message: ToastMessage, options?: ExternalToast) => preventSpam(message, originalToast.error, options)
toast.info = (message: ToastMessage, options?: ExternalToast) => preventSpam(message, originalToast.info, options)
toast.warning = (message: ToastMessage, options?: ExternalToast) => preventSpam(message, originalToast.warning, options)

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
            richColors
            toastOptions={{
              className: 'gap-3 font-medium rounded-xl',
              style: { padding: '12px 16px' }
            }}
          />
        </ThemeProvider>
      </ErrorBoundary>
    </BrowserRouter>
  </React.StrictMode>
)
