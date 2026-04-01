import React from 'react'
import { ShieldAlert, RefreshCw, Home } from 'lucide-react'

class ErrorBoundary extends React.Component {
  constructor(props) {
    super(props)
    this.state = { hasError: false, error: null }
  }

  static getDerivedStateFromError(error) {
    return { hasError: true, error }
  }

  componentDidCatch(error, errorInfo) {
    console.error('Uncaught error:', error, errorInfo)
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex flex-col items-center justify-center min-h-screen bg-slate-50 dark:bg-slate-900 text-slate-200 p-8">
          <div className="w-20 h-20 bg-rose-500/10 border border-rose-500/20 rounded-3xl flex items-center justify-center mb-10 shadow-[0_0_50px_rgba(244,63,94,0.1)]">
            <ShieldAlert className="w-10 h-10 text-rose-500" />
          </div>
          <h1 className="text-4xl font-black text-slate-900 dark:text-white tracking-tighter mb-4 italic">System <span className="text-rose-500">Error</span></h1>
          <p className="text-slate-600 dark:text-slate-400 text-center max-w-md mb-12 font-medium leading-relaxed">
            The application encountered an unexpected error. Please try refreshing the page or returning to the dashboard.
          </p>
          <div className="flex flex-col sm:flex-row gap-5">
            <button 
              onClick={() => window.location.reload()}
              className="px-10 py-5 bg-white text-black font-black uppercase tracking-widest rounded-2xl transition-all hover:scale-105 flex items-center gap-3"
            >
              <RefreshCw className="w-5 h-5" /> Try Again
            </button>
            <button 
              onClick={() => window.location.href = '/dashboard'}
              className="px-10 py-5 bg-slate-100 dark:bg-white/5 border border-slate-200 dark:border-white/5 text-slate-600 dark:text-slate-400 font-black uppercase tracking-widest rounded-2xl transition-all hover:bg-slate-200 dark:bg-white/10 hover:text-slate-900 dark:text-white flex items-center gap-3"
            >
              <Home className="w-5 h-5" /> Back to Dashboard
            </button>
          </div>
          {process.env.NODE_ENV === 'development' && (
            <div className="mt-16 w-full max-w-2xl overflow-hidden bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm border-rose-500/20 bg-rose-500/5">
              <div className="px-6 py-3 border-b border-rose-500/10 bg-rose-500/5 text-rose-400 text-[10px] font-black uppercase tracking-widest">Debug Stream</div>
              <pre className="p-8 font-mono text-xs text-rose-400/80 overflow-auto whitespace-pre-wrap">
                {this.state.error?.toString()}
              </pre>
            </div>
          )}
        </div>
      )
    }

    return this.props.children
  }
}

export default ErrorBoundary
