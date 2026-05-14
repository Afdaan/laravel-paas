import { useState, useEffect, useMemo, useRef } from 'react'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { projectsAPI } from '../services/api'
import { Terminal, Copy } from 'lucide-react'
import { toast } from 'sonner'
import useTranslation from '@/lib/useTranslation'
import ConfirmationModal from './ConfirmationModal'

interface BuildLogsConsoleProps {
  projectId: string | number
  status?: string
}

const BuildLogsConsole = ({ projectId, status }: BuildLogsConsoleProps) => {
  const { t } = useTranslation()
  const [logs, setLogs] = useState<string>('')
  const [clearedLength, setClearedLength] = useState(0)
  const [isConfirmOpen, setIsConfirmOpen] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)

  // Limit to last 500 lines for performance
  const logLines = useMemo(() => {
    if (!logs) return []
    const visibleLogs = clearedLength > 0 ? logs.substring(clearedLength) : logs
    const lines = visibleLogs.split('\n').filter(l => l.trim() !== '' || l === '')
    return lines.length > 500 ? lines.slice(-500) : lines
  }, [logs, clearedLength])

  const lineOffset = useMemo(() => {
    if (!logs) return 0
    const visibleLogs = clearedLength > 0 ? logs.substring(clearedLength) : logs
    const lines = visibleLogs.split('\n').filter(l => l.trim() !== '' || l === '')
    return lines.length > 500 ? lines.length - 500 : 0
  }, [logs, clearedLength])

  useEffect(() => {
    let isMounted = true
    const controller = new AbortController()

    const fetchBuildLogs = async () => {
      try {
        const response = await projectsAPI.buildLogs(projectId)
        if (isMounted && response.data.logs) {
          setLogs(response.data.logs)
        }
      } catch (error) {
        if (isMounted) {
          console.error('Failed to fetch build logs:', error)
        }
      }
    }

    // Initial fetch
    fetchBuildLogs()

    // Poll every 3 seconds (slightly slower to save CPU) while active
    const interval = setInterval(fetchBuildLogs, 3000)

    return () => {
      isMounted = false
      controller.abort()
      clearInterval(interval)
    }
  }, [projectId])

  useEffect(() => {
    // Auto scroll to bottom when logs update
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [logLines])

  const copyToClipboard = () => {
    navigator.clipboard.writeText(logs)
    toast.success(t('common.copySuccess'))
  }

  const scrollToBottom = () => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }

  const handleClear = () => {
    setIsConfirmOpen(true)
  }

  const confirmClear = () => {
    setClearedLength(logs.length)
    toast.success(t('common.success'))
  }

  return (
    <Card className="bg-zinc-950 text-white border-none overflow-hidden flex flex-col h-[600px] gap-0 py-0 shadow-2xl">
      <CardHeader className="bg-zinc-900/80 px-4 py-3 border-b border-white/5 flex flex-row items-center justify-between shrink-0">
        <div className="flex items-center gap-3">
          <div className="flex gap-1.5 mr-2">
            <div className="w-2.5 h-2.5 rounded-full bg-rose-500/80 shadow-[0_0_8px_rgba(244,63,94,0.2)]" />
            <div className="w-2.5 h-2.5 rounded-full bg-amber-500/80 shadow-[0_0_8px_rgba(245,158,11,0.2)]" />
            <div className="w-2.5 h-2.5 rounded-full bg-emerald-500/80 shadow-[0_0_8px_rgba(16,185,129,0.2)]" />
          </div>
          <div className="text-[10px] uppercase font-bold tracking-widest text-zinc-400 flex items-center gap-2">
            <Terminal className="w-3.5 h-3.5" />
            {t('projectDetail.messages.buildLogs')}
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={copyToClipboard}
            className="p-1.5 hover:bg-white/10 rounded-md text-zinc-500 hover:text-white cursor-pointer transition-colors"
            title="Copy Logs"
          >
            <Copy className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={scrollToBottom}
            className="p-1.5 hover:bg-white/10 rounded-md text-zinc-500 hover:text-white cursor-pointer transition-colors"
            title="Scroll to Bottom"
          >
            <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="m7 15 5 5 5-5" /><path d="m7 9 5 5 5-5" /></svg>
          </button>
          <div className="w-px h-3 bg-white/10 mx-1" />
          <button
            onClick={handleClear}
            className="text-[10px] uppercase font-bold text-zinc-600 hover:text-rose-400 px-2 cursor-pointer transition-colors"
          >
            {t('projectDetail.actions.clear')}
          </button>
        </div>
      </CardHeader>
      <CardContent className="flex-1 p-0 overflow-hidden bg-zinc-950">
        <div
          ref={scrollRef}
          className="h-full overflow-y-auto p-6 font-mono text-[11px] leading-relaxed text-zinc-300 custom-scrollbar selection:bg-primary/30"
        >
          {logLines.length > 0 ? logLines.map((line: string, i: number) => (
            <div key={i} className="flex gap-4 group py-0.5 px-2 rounded -mx-2 hover:bg-white/[0.05] transition-colors">
              <span className="shrink-0 text-zinc-800 select-none w-8 text-right font-light">{lineOffset + i + 1}</span>
              <span className="whitespace-pre-wrap break-all">{line}</span>
            </div>
          )) : (status === 'queued' || status === 'building') ? (
            <div className="flex flex-col gap-1 opacity-70 mt-2">
              <div className="flex gap-4 group py-0.5 px-2 rounded -mx-2">
                <span className="shrink-0 text-zinc-800 select-none w-8 text-right font-light">1</span>
                <span className="text-blue-400">System <span className="text-zinc-500">Preparing build environment...</span></span>
              </div>
              <div className="flex gap-4 group py-0.5 px-2 rounded -mx-2">
                <span className="shrink-0 text-zinc-800 select-none w-8 text-right font-light">2</span>
                <span className="text-blue-400 animate-pulse">System <span className="text-zinc-500">Retrieving project source code and configuration...</span></span>
              </div>
            </div>
          ) : (
            <div className="h-full flex flex-col items-center justify-center opacity-10 gap-4">
              <Terminal size={48} />
              <p className="uppercase tracking-[0.4em] font-bold text-xs animate-pulse">
                {t('projectDetail.messages.streamingLogs')}
              </p>
            </div>
          )}
        </div>
      </CardContent>
      <ConfirmationModal
        isOpen={isConfirmOpen}
        onClose={() => setIsConfirmOpen(false)}
        onConfirm={confirmClear}
        title={t('projectDetail.actions.clear')}
        message={t('common.confirmClearLogs') || 'Confirm clearing build logs? New logs will still appear.'}
        type="warning"
        confirmText={t('common.confirm')}
      />
    </Card>
  )
}

export default BuildLogsConsole
