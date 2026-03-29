import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { Toaster } from 'react-hot-toast'
import App from './App'
import ErrorBoundary from './components/ErrorBoundary'
import './index.css'

ReactDOM.createRoot(document.getElementById('root')).render(
  <React.StrictMode>
    <BrowserRouter>
      <ErrorBoundary>
        <App />
      </ErrorBoundary>
      <Toaster 
        position="top-center"
        toastOptions={{
          duration: 4000,
          style: {
            background: '#09090b',
            color: '#f8fafc',
            border: '1px solid rgba(255, 255, 255, 0.05)',
            borderRadius: '1rem',
            padding: '12px 16px',
            fontSize: '13px',
            fontWeight: '600',
            letterSpacing: '-0.01em',
            boxShadow: '0 25px 50px -12px rgba(0, 0, 0, 0.5)',
          },
          success: {
            iconTheme: { primary: '#10b981', secondary: '#09090b' }
          },
          error: {
            iconTheme: { primary: '#ef4444', secondary: '#09090b' }
          }
        }}
      />
    </BrowserRouter>
  </React.StrictMode>,
)
