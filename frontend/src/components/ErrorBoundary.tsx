import React, { ErrorInfo, ReactNode } from 'react'
import { ShieldAlert, RefreshCw, Home } from 'lucide-react'
import { Button } from '@/components/ui/button'

interface Props {
  children?: ReactNode;
}

interface State {
  hasError: boolean;
  error: Error | null;
}

class ErrorBoundary extends React.Component<Props, State> {
  constructor(props: Props) {
    super(props)
    this.state = { hasError: false, error: null }
  }

  static getDerivedStateFromError(error: Error): State {
    // If it's a chunk load error (happens during new deployments), trigger a reload
    const isChunkError = error.name === 'ChunkLoadError' ||
                        error.message?.includes('Loading chunk') ||
                        error.message?.includes('Failed to fetch dynamically imported module');

    if (isChunkError) {
      const hasReloaded = sessionStorage.getItem('chunk-error-reload');
      if (!hasReloaded) {
        sessionStorage.setItem('chunk-error-reload', 'true');
        window.location.reload();
        return { hasError: false, error: null };
      }
    }

    return { hasError: true, error }
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error('Uncaught error:', error, errorInfo)

    // Clear the reload flag after a successful catch or if we didn't reload
    setTimeout(() => sessionStorage.removeItem('chunk-error-reload'), 5000);
  }

  render() {
    if (this.state.hasError) {
      // Check if it's a potential network/deployment error
      const isNetworkError = this.state.error?.message?.includes('502') ||
                             this.state.error?.message?.includes('503') ||
                             this.state.error?.message?.includes('Network Error') ||
                             this.state.error?.message?.includes('Failed to fetch');

      return (
        <div className="flex flex-col items-center justify-center min-h-screen bg-background text-foreground p-8">
          <div className="w-20 h-20 bg-rose-500/10 border border-rose-500/20 rounded-3xl flex items-center justify-center mb-10 shadow-[0_0_50px_rgba(244,63,94,0.1)]">
            <ShieldAlert className="w-10 h-10 text-rose-500" />
          </div>
          <h1 className="text-4xl font-bold tracking-tight text-foreground mb-4">
            System <span className="text-rose-500">{isNetworkError ? 'Updating' : 'Error'}</span>
          </h1>
          <p className="text-muted-foreground text-center max-w-md mb-12 leading-relaxed">
            {isNetworkError
              ? "We're currently updating the system to give you a better experience. Everything will be back in a few seconds. Do not worry, your progress is saved."
              : "The application encountered an unexpected error. Your path and session are preserved. Please try refreshing or returning to the dashboard."}
          </p>
          <div className="flex flex-col sm:flex-row gap-4">
            <Button
              onClick={() => window.location.reload()}
              size="lg"
              className="gap-2"
            >
              <RefreshCw className="w-4 h-4" /> {isNetworkError ? 'Reconnect Now' : 'Try Again'}
            </Button>
            <Button
              variant="outline"
              size="lg"
              onClick={() => window.location.href = '/dashboard'}
              className="gap-2"
            >
              <Home className="w-4 h-4" /> Dashboard
            </Button>
          </div>

          <div className="mt-8 text-[10px] text-muted-foreground font-mono uppercase tracking-[0.2em]">
            Stuck at: {window.location.pathname}
          </div>

          {import.meta.env.DEV && (
            <div className="mt-16 w-full max-w-2xl overflow-hidden bg-muted border rounded-2xl shadow-sm border-rose-500/20">
              <div className="px-6 py-3 border-b border-rose-500/10 bg-rose-500/5 text-rose-400 text-[10px] font-bold uppercase tracking-widest">Debug Stream</div>
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
