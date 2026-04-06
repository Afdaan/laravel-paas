import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import { Toaster } from 'sonner'
import App from './App'
import ErrorBoundary from './components/ErrorBoundary'
import './index.css'

const root = document.getElementById('root') as HTMLElement

ReactDOM.createRoot(root).render(
  <React.StrictMode>
    <BrowserRouter>
      <ErrorBoundary>
        <App />
      </ErrorBoundary>
      <Toaster 
        position="top-center"
        toastOptions={{
          style: {
            background: '#09090b',
            color: '#f8fafc',
            border: '1px solid rgba(255, 255, 255, 0.1)',
            borderRadius: '0.75rem',
          }
        }}
      />
    </BrowserRouter>
  </React.StrictMode>
)
