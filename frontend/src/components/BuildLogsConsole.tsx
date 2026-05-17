import { useState, useEffect, useMemo, useRef } from 'react'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { projectsAPI } from '../services/api'
import { Terminal, Copy, Activity } from 'lucide-react'
import { toast } from 'sonner'
import useTranslation from '@/lib/useTranslation'
import ConfirmationModal from './ConfirmationModal'
import { cn } from '@/lib/utils'
import { Project, DeploymentEvent } from '@/types'

interface BuildLogsConsoleProps {
  projectId: string | number
  status?: string
  project?: Project
}

const BuildLogsConsole = ({ projectId, status, project }: BuildLogsConsoleProps) => {
  const { t } = useTranslation()
  const [logs, setLogs] = useState<string>('')
  const [events, setEvents] = useState<DeploymentEvent[]>([])
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

    const fetchData = async () => {
      try {
        const [logsRes, eventsRes] = await Promise.all([
          projectsAPI.buildLogs(projectId).catch(() => ({ data: { logs: '' } })),
          projectsAPI.getDeploymentEvents(projectId).catch(() => ({ data: [] }))
        ])
        if (isMounted) {
          if (logsRes.data?.logs) setLogs(logsRes.data.logs)
          if (Array.isArray(eventsRes.data)) setEvents(eventsRes.data)
        }
      } catch (error) {
        if (isMounted) {
          console.error('Failed to fetch build data:', error)
        }
      }
    }

    // Initial fetch
    fetchData()

    // Poll every 3 seconds (slightly slower to save CPU) while active
    const interval = setInterval(fetchData, 3000)

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
    <div className="grid grid-cols-1 lg:grid-cols-3 h-[600px] bg-zinc-950 rounded-xl overflow-hidden shadow-2xl border border-white/10">
      {/* Left Column: Timeline */}
      <div className="lg:col-span-1 border-b lg:border-b-0 lg:border-r border-white/10 bg-zinc-900/40 flex flex-col h-full overflow-hidden">
        <div className="bg-zinc-900/80 px-4 py-3 border-b border-white/5 flex items-center justify-between shrink-0">
          <div className="text-[10px] uppercase font-bold tracking-widest text-zinc-300 flex items-center gap-2">
            <Activity className="w-3.5 h-3.5 text-blue-400" />
            {t('projectDetail.tabs.timeline') || 'Deployment Timeline'}
          </div>
          {project?.deployment_status && (
            <Badge variant="outline" className={cn(
              "text-[10px] uppercase font-mono py-0.5 px-2",
              project.deployment_status === 'completed' ? "text-emerald-400 border-emerald-400/30 bg-emerald-500/10" :
              project.deployment_status === 'failed' ? "text-rose-400 border-rose-400/30 bg-rose-500/10" :
              "text-blue-400 border-blue-400/30 bg-blue-500/10 animate-pulse"
            )}>
              {project.deployment_status} {project.deployment_progress != null ? `(${project.deployment_progress}%)` : ''}
            </Badge>
          )}
        </div>
        <div className="p-5 flex-1 overflow-y-auto custom-scrollbar">
          {events.length === 0 ? (
            <div className="text-center py-12 text-zinc-600 text-xs italic">
              No deployment events recorded yet.
            </div>
          ) : (
            <div className="relative border-l border-zinc-800 ml-3 pl-5 space-y-6">
              {events.map((ev, idx) => (
                <div key={ev.id || idx} className="relative group">
                  <div className={cn(
                    "absolute -left-[25px] top-1 w-2.5 h-2.5 rounded-full border border-zinc-950 transition-transform group-hover:scale-125",
                    ev.status === 'completed' ? "bg-emerald-500" :
                    ev.status === 'failed' ? "bg-rose-500" :
                    "bg-blue-500 animate-pulse"
                  )} />
                  <div className="flex items-center justify-between mb-1">
                    <span className="text-xs font-bold text-zinc-200 capitalize tracking-wide">
                      {ev.step_name.replace(/_/g, ' ')}
                    </span>
                    <span className="text-[10px] font-mono text-zinc-500">
                      {new Date(ev.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                    </span>
                  </div>
                  <p className="text-[11px] text-zinc-400 leading-relaxed font-sans break-words">{ev.message}</p>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>

      {/* Right Column: Console */}
      <Card className="lg:col-span-2 bg-zinc-950 text-white border-none overflow-hidden flex flex-col h-full gap-0 py-0 rounded-none shadow-none">
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
      </Card>
      <ConfirmationModal
        isOpen={isConfirmOpen}
        onClose={() => setIsConfirmOpen(false)}
        onConfirm={confirmClear}
        title={t('projectDetail.actions.clear')}
        message={t('common.confirmClearLogs') || 'Confirm clearing build logs? New logs will still appear.'}
        type="warning"
        confirmText={t('common.confirm')}
      />
    </div>
  )
}

export default BuildLogsConsole
