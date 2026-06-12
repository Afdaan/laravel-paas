import { useState, useEffect, useMemo, useRef, useReducer } from 'react'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { projectsAPI } from '../services/api'
import { Terminal, Copy, Activity } from 'lucide-react'
import { toast } from 'sonner'
import useTranslation from '@/lib/useTranslation'
import ConfirmationModal from './ConfirmationModal'
import { cn } from '@/lib/utils'
import { Project, DeploymentEvent } from '@/types'
import {
  appendBuildLogLines,
  clearVisibleBuildLogs,
  initialBuildLogsState,
  MAX_RETAINED_BUILD_LOG_LINES,
  mergeBuildLogSnapshot,
  splitLogSnapshot,
  type BuildLogsSnapshot,
  type BuildLogsState,
} from '@/lib/buildLogsState'

const renderLogLine = (line: string) => {
  const trimmed = line.trim()

  // 1. Stage / step header starting with ">>"
  if (trimmed.startsWith('>>')) {
    const idx = line.indexOf('>>')
    const prefix = line.substring(0, idx)
    const content = line.substring(idx + 2)
    return (
      <span className="font-semibold text-sky-400">
        {prefix && <span className="text-zinc-500">{prefix}</span>}
        <span className="text-sky-500 font-bold select-none mr-1.5">&gt;&gt;</span>
        <span>{content}</span>
      </span>
    )
  }

  // 2. Success step starting with "✓"
  if (trimmed.startsWith('✓')) {
    const idx = line.indexOf('✓')
    const prefix = line.substring(0, idx)
    const content = line.substring(idx + 1)
    return (
      <span className="text-emerald-400 font-medium">
        {prefix && <span className="text-zinc-500">{prefix}</span>}
        <span className="text-emerald-500 font-bold select-none mr-1.5">✓</span>
        <span>{content}</span>
      </span>
    )
  }

  // 3. Success step starting with "[SUCCESS]" or containing build summary success
  if (trimmed.startsWith('[SUCCESS]') || (trimmed.startsWith('[BUILD SUMMARY]') && trimmed.toLowerCase().includes('successfully'))) {
    return (
      <span className="text-emerald-400 font-semibold">
        {line}
      </span>
    )
  }

  // 4. Error lines starting with "✗" or containing "error" (e.g. "[ERROR]", "error:")
  if (
    trimmed.startsWith('✗') ||
    trimmed.startsWith('[ERROR]') ||
    trimmed.toLowerCase().includes('error:') ||
    trimmed.toLowerCase().includes('failed')
  ) {
    // Only color as error if it's not a success line
    if (!trimmed.toLowerCase().includes('successfully') && !trimmed.toLowerCase().includes('success')) {
      return (
        <span className="text-rose-400 font-medium">
          {line}
        </span>
      )
    }
  }

  // 5. Warning lines
  if (trimmed.startsWith('[WARNING]') || trimmed.toLowerCase().includes('warning:')) {
    return (
      <span className="text-amber-400 font-medium">
        {line}
      </span>
    )
  }

  // 6. Section dividers (e.g. === or ---)
  if (trimmed.startsWith('===') || trimmed.startsWith('---')) {
    return <span className="text-zinc-600 font-light select-none tracking-widest">{line}</span>
  }

  // 7. General info logs
  if (trimmed.startsWith('INFO')) {
    return (
      <span className="text-zinc-300">
        <span className="text-zinc-500 font-medium mr-1.5">INFO</span>
        {line.substring(4)}
      </span>
    )
  }

  // 8. Fallback standard line
  return <span>{line}</span>
}

interface BuildLogsConsoleProps {
  projectId: string | number
  status?: string
  project?: Project
  onDeploymentEvent?: (event: DeploymentEvent) => void
}

type BuildLogsResponse = {
  logs?: string
  job_id?: string
  available?: boolean
  placeholder?: boolean
}

type LiveBuildLogPayload = {
  job_id?: string
  line?: string
  logs?: string
}

type BuildLogsAction =
  | { type: 'reset' }
  | { type: 'hydrate'; state: BuildLogsState }
  | { type: 'merge_snapshot'; snapshot: BuildLogsSnapshot }
  | { type: 'append_lines'; lines: string[] }
  | { type: 'clear_visible' }

const buildLogsReducer = (state: BuildLogsState, action: BuildLogsAction): BuildLogsState => {
  switch (action.type) {
    case 'reset':
      return initialBuildLogsState
    case 'hydrate':
      return action.state
    case 'merge_snapshot':
      return mergeBuildLogSnapshot(state, action.snapshot)
    case 'append_lines':
      return appendBuildLogLines(state, action.lines)
    case 'clear_visible':
      return clearVisibleBuildLogs(state)
  }
}

const getBuildLogsCacheKey = (projectId: string | number, jobId?: string) => {
  return `build-logs:${String(projectId)}:${jobId || 'latest'}`
}

const readCachedBuildLogs = (cacheKey: string): BuildLogsState | null => {
  try {
    const value = window.sessionStorage.getItem(cacheKey)
    if (!value) return null

    const parsed = JSON.parse(value) as Partial<BuildLogsState>
    if (!Array.isArray(parsed.lines)) return null

    const lines = parsed.lines
      .filter((line): line is string => typeof line === 'string')
      .slice(-MAX_RETAINED_BUILD_LOG_LINES)
    const droppedCount = parsed.lines.length - lines.length
    const clearedCount = typeof parsed.clearedCount === 'number' ? parsed.clearedCount : 0

    return {
      lines,
      clearedCount: Math.max(0, clearedCount - droppedCount),
    }
  } catch {
    return null
  }
}

const writeCachedBuildLogs = (cacheKey: string, state: BuildLogsState) => {
  try {
    window.sessionStorage.setItem(cacheKey, JSON.stringify(state))
  } catch {
    // Storage can be unavailable or full; the API/SSE rehydration path remains authoritative.
  }
}

// Realtime Safeguard: Reject stale logs from previous jobs to prevent console pollution.
const isBuildLogForActiveJob = (incomingJobId?: string, activeJobId?: string) => {
  return !incomingJobId || !activeJobId || incomingJobId === activeJobId
}

const toBuildLogsSnapshot = (data?: BuildLogsResponse, activeJobId?: string): BuildLogsSnapshot => {
  if (!isBuildLogForActiveJob(data?.job_id, activeJobId)) {
    return {
      lines: [],
      available: false,
    }
  }

  const rawLogs = data?.logs || ''

  return {
    lines: rawLogs ? splitLogSnapshot(rawLogs) : [],
    available: data?.available === true || (data?.available === undefined && rawLogs.length > 0 && data?.placeholder !== true),
  }
}

const BuildLogsConsole = ({ projectId, status, project, onDeploymentEvent }: BuildLogsConsoleProps) => {
  const { t } = useTranslation()
  const [logState, dispatchLogs] = useReducer(buildLogsReducer, initialBuildLogsState)
  const [events, setEvents] = useState<DeploymentEvent[]>([])
  const [clearedEventMaxId, setClearedEventMaxId] = useState<number>(-1)
  const [isConfirmOpen, setIsConfirmOpen] = useState(false)
  const [isTimelineConfirmOpen, setIsTimelineConfirmOpen] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)
  const logsCacheKey = useMemo(
    () => getBuildLogsCacheKey(projectId, project?.deployment_job_id),
    [projectId, project?.deployment_job_id]
  )
  const activeJobId = project?.deployment_job_id || ''

  // Limit to last 500 lines for performance
  const logLines = useMemo(() => {
    const visibleLogs = logState.clearedCount > 0 ? logState.lines.slice(logState.clearedCount) : logState.lines
    return visibleLogs.length > 500 ? visibleLogs.slice(-500) : visibleLogs
  }, [logState])

  const lineOffset = useMemo(() => {
    const visibleLogs = logState.clearedCount > 0 ? logState.lines.slice(logState.clearedCount) : logState.lines
    return visibleLogs.length > 500 ? visibleLogs.length - 500 : 0
  }, [logState])

  const visibleEvents = useMemo(() => {
    return events.filter(ev => {
      const type = (ev.event_type || '').toLowerCase();
      const stepName = (ev.step_name || '').toLowerCase();
      if (type.includes('lease') || stepName.includes('lease')) {
        return false;
      }
      return (ev.id || 0) > clearedEventMaxId;
    });
  }, [events, clearedEventMaxId])



  const isDeploying = useMemo(() => {
    return Boolean(project?.deployment_status && !['completed', 'failed', 'rollback', 'cancelled'].includes(project.deployment_status))
  }, [project?.deployment_status])

  useEffect(() => {
    const cachedState = readCachedBuildLogs(logsCacheKey)
    dispatchLogs(cachedState ? { type: 'hydrate', state: cachedState } : { type: 'reset' })
    setEvents([])
    setClearedEventMaxId(-1)
  }, [logsCacheKey])

  useEffect(() => {
    if (logState.lines.length === 0) return

    const timeout = window.setTimeout(() => {
      writeCachedBuildLogs(logsCacheKey, logState)
    }, 500)

    return () => {
      window.clearTimeout(timeout)
    }
  }, [logsCacheKey, logState])

  // 1. Fetch static logs once if deployment is NOT active
  useEffect(() => {
    if (isDeploying) return
    
    let isMounted = true
    
    const fetchStaticLogs = async () => {
      try {
        const [logsRes, eventsRes] = await Promise.all([
          projectsAPI.buildLogs(projectId).catch(() => ({ data: { logs: '' } })),
          projectsAPI.getDeploymentEvents(projectId).catch(() => ({ data: [] }))
        ])
        
        if (!isMounted) return
        
        dispatchLogs({ type: 'merge_snapshot', snapshot: toBuildLogsSnapshot(logsRes.data, activeJobId) })
        if (Array.isArray(eventsRes.data)) {
          setEvents(eventsRes.data)
          if (eventsRes.data.length > 0 && onDeploymentEvent) {
            const sorted = [...eventsRes.data].sort((a, b) => (b.id || 0) - (a.id || 0))
            onDeploymentEvent(sorted[0])
          }
        } else {
          setEvents([])
        }
      } catch (err) {
        console.error('Failed to fetch initial build logs:', err)
      }
    }

    fetchStaticLogs()

    return () => {
      isMounted = false
    }
  }, [projectId, project?.deployment_job_id, activeJobId, isDeploying, onDeploymentEvent])

  // 2. Stream logs/events if deployment is active
  useEffect(() => {
    if (!isDeploying) return

    let isMounted = true
    let logsEventSource: EventSource | null = null
    let eventsEventSource: EventSource | null = null
    let pollingInterval: ReturnType<typeof setInterval> | null = null
    let isPollingFallbackActive = false

    // Fallback: standard API polling
    const startPollingFallback = () => {
      if (isPollingFallbackActive) return
      isPollingFallbackActive = true
      if (pollingInterval) clearInterval(pollingInterval)
      
      const fetchData = async () => {
        try {
          const [logsRes, eventsRes] = await Promise.all([
            projectsAPI.buildLogs(projectId).catch(() => ({ data: { logs: '' } })),
            projectsAPI.getDeploymentEvents(projectId).catch(() => ({ data: [] }))
          ])
          if (!isMounted) return
          dispatchLogs({ type: 'merge_snapshot', snapshot: toBuildLogsSnapshot(logsRes.data, activeJobId) })
          if (Array.isArray(eventsRes.data)) setEvents(eventsRes.data)
        } catch (error) {
          console.error('Failed to fetch build data during polling:', error)
        }
      }

      fetchData()
      pollingInterval = setInterval(fetchData, 3000)
    }

    const connectSSE = async () => {
      try {
        const token = localStorage.getItem('token') || ''
        const res = await fetch('/api/auth/stream-token', {
          method: 'POST',
          headers: { 'Authorization': `Bearer ${token}` }
        })
        if (!res.ok) {
          throw new Error('Failed to get stream token')
        }
        if (!isMounted) return
        const data = await res.json()
        const streamToken = data.token

        if (!isMounted) return

        // 1. Build logs connection
        const logsUrl = `/api/projects/${projectId}/build-logs/stream?stream_token=${encodeURIComponent(streamToken)}`
        logsEventSource = new EventSource(logsUrl)

        logsEventSource.addEventListener('initial_logs', (e) => {
          if (!isMounted) return
          try {
            const initialLogs = JSON.parse(e.data) as string | LiveBuildLogPayload
            const incomingJobId = typeof initialLogs === 'string' ? undefined : initialLogs.job_id
            if (!isBuildLogForActiveJob(incomingJobId, activeJobId)) return
            const rawLogs = typeof initialLogs === 'string' ? initialLogs : initialLogs.logs || ''
            dispatchLogs({
              type: 'merge_snapshot',
              snapshot: {
                lines: rawLogs ? splitLogSnapshot(rawLogs) : [],
                available: rawLogs.length > 0,
              },
            })
          } catch (err) {
            console.error('Failed to parse initial logs:', err)
          }
        })

        logsEventSource.addEventListener('log', (e) => {
          if (!isMounted) return
          try {
            const newLogLine = JSON.parse(e.data) as string | LiveBuildLogPayload
            const incomingJobId = typeof newLogLine === 'string' ? undefined : newLogLine.job_id
            if (!isBuildLogForActiveJob(incomingJobId, activeJobId)) return
            const rawLine = typeof newLogLine === 'string' ? newLogLine : newLogLine.line || ''
            if (!rawLine) return
            const lines = rawLine.split('\n')
            dispatchLogs({ type: 'append_lines', lines })
          } catch (err) {
            console.error('Failed to parse log line:', err)
          }
        })

        logsEventSource.onerror = (err) => {
          console.warn('Logs SSE connection error, falling back to polling:', err)
          if (isMounted) {
            logsEventSource?.close()
            logsEventSource = null
            startPollingFallback()
          }
        }

        // 2. Deployment events connection
        const eventsUrl = `/api/projects/${projectId}/deployment-events/stream?stream_token=${encodeURIComponent(streamToken)}`
        eventsEventSource = new EventSource(eventsUrl)

        eventsEventSource.addEventListener('initial_events', (e) => {
          if (!isMounted) return
          try {
            const initialEvents = JSON.parse(e.data)
            setEvents(initialEvents)
            if (initialEvents.length > 0 && onDeploymentEvent) {
              const sorted = [...initialEvents].sort((a, b) => (b.id || 0) - (a.id || 0))
              onDeploymentEvent(sorted[0])
            }
          } catch (err) {
            console.error('Failed to parse initial events:', err)
          }
        })

        eventsEventSource.addEventListener('deployment_event', (e) => {
          if (!isMounted) return
          try {
            const newEvent = JSON.parse(e.data) as DeploymentEvent
            setEvents(prev => {
              if (prev.some(ev => ev.id === newEvent.id && newEvent.id !== 0)) {
                return prev
              }
              const updated = [...prev, newEvent]
              if (onDeploymentEvent) {
                onDeploymentEvent(newEvent)
              }
              return updated
            })
          } catch (err) {
            console.error('Failed to parse deployment event:', err)
          }
        })

        eventsEventSource.onerror = (err) => {
          console.warn('Events SSE connection error, falling back to polling:', err)
          if (isMounted) {
            eventsEventSource?.close()
            eventsEventSource = null
            startPollingFallback()
          }
        }

      } catch (err) {
        console.warn('Failed to initialize SSE, falling back to polling:', err)
        if (isMounted) {
          startPollingFallback()
        }
      }
    }

    connectSSE()

    return () => {
      isMounted = false
      if (logsEventSource) {
        logsEventSource.close()
      }
      if (eventsEventSource) {
        eventsEventSource.close()
      }
      if (pollingInterval) {
        clearInterval(pollingInterval)
      }
    }
  }, [projectId, onDeploymentEvent, project?.deployment_job_id, isDeploying])

  useEffect(() => {
    // Auto scroll to bottom when logs update
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [logLines])

  const copyToClipboard = () => {
    const visibleLogs = logState.clearedCount > 0 ? logState.lines.slice(logState.clearedCount) : logState.lines
    navigator.clipboard.writeText(visibleLogs.join('\n'))
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
    dispatchLogs({ type: 'clear_visible' })
    toast.success(t('common.success'))
  }

  const confirmTimelineClear = () => {
    const maxId = events.length > 0 ? Math.max(...events.map(e => e.id || 0)) : -1
    setClearedEventMaxId(maxId)
    toast.success(t('common.success'))
  }

  return (
    <div className="grid grid-cols-1 lg:grid-cols-3 h-[600px] bg-zinc-950 rounded-xl overflow-hidden shadow-2xl border border-white/10">
      {/* Left Column: Timeline */}
      <div className="lg:col-span-1 border-b lg:border-b-0 lg:border-r border-white/10 bg-zinc-900/40 flex flex-col h-full overflow-hidden">
        <div className="bg-zinc-900/80 px-4 py-3 border-b border-white/5 flex items-center justify-between shrink-0">
          <div className="text-[10px] uppercase font-bold tracking-widest text-zinc-300 flex items-center gap-2 font-sans">
            <Activity className="w-3.5 h-3.5 text-blue-400" />
            {t('projectDetail.tabs.timeline') || 'Deployment Timeline'}
          </div>
          <div className="flex items-center gap-2">
            {project?.deployment_status && (
              <Badge variant="outline" className={cn(
                "text-[10px] uppercase font-mono py-0.5 px-2",
                project.deployment_status === 'completed' ? "text-emerald-400 border-emerald-400/30 bg-emerald-500/10" :
                project.deployment_status === 'failed' ? "text-rose-400 border-rose-400/30 bg-rose-500/10" :
                "text-blue-400 border-blue-400/30 bg-blue-500/10"
              )}>
                {project.deployment_status} {project.deployment_progress != null ? `(${project.deployment_progress}%)` : ''}
              </Badge>
            )}
            {visibleEvents.length > 0 && (
              <>
                <div className="w-px h-3 bg-white/10 mx-1" />
                <button
                  onClick={() => setIsTimelineConfirmOpen(true)}
                  className="text-[10px] uppercase font-bold text-zinc-600 hover:text-rose-400 px-1 cursor-pointer transition-colors font-sans"
                  title="Clear Timeline"
                >
                  {t('projectDetail.actions.clear')}
                </button>
              </>
            )}
          </div>
        </div>
        <div className="p-5 flex-1 overflow-y-auto custom-scrollbar">
          {visibleEvents.length === 0 ? (
            <div className="text-center py-12 text-zinc-500 text-xs font-sans">
              No deployment events recorded yet.
            </div>
          ) : (
            <div className="relative border-l border-zinc-800 ml-3 pl-5 space-y-6 font-sans">
              {visibleEvents.map((ev, idx) => (
                <div key={ev.id || idx} className="relative group">
                  <div className={cn(
                    "absolute -left-[25px] top-1 w-2.5 h-2.5 rounded-full border border-zinc-950 transition-transform group-hover:scale-125",
                    (ev.status === 'completed' || ev.state_to === 'completed') ? "bg-emerald-500" :
                    (ev.status === 'failed' || ev.state_to === 'failed' || Boolean(ev.error)) ? "bg-rose-500" :
                    "bg-blue-500 animate-pulse"
                  )} />
                  <div className="flex items-center justify-between mb-1 font-sans">
                    <span className="text-xs font-bold text-zinc-200 capitalize tracking-wide font-sans">
                      {String(ev.step_name || ev.event_type || 'system_event').replace(/_/g, ' ')}
                    </span>
                    <span className="text-[10px] font-mono text-zinc-500">
                      {new Date(ev.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                    </span>
                  </div>
                  <p className="text-[11px] text-zinc-400 leading-relaxed font-sans break-words font-medium">
                    {(() => {
                      if (ev.error) return ev.error;
                      const type = ev.event_type || '';
                      const payload = ev.payload || '';
                      
                      if (type === 'deployment_completed') {
                        const shortHash = payload && payload.length >= 7 ? payload.substring(0, 7) : payload;
                        return `Deployment completed successfully. Active commit: ${shortHash}`;
                      }
                      if (type === 'rollback_completed') {
                        const shortHash = payload && payload.length >= 7 ? payload.substring(0, 7) : payload;
                        return `Rollback completed. Reverted to commit: ${shortHash}`;
                      }
                      if (type === 'deployment_skipped_existing_image') {
                        const shortHash = payload && payload.length >= 7 ? payload.substring(0, 7) : payload;
                        return `Deployment skipped. Image is already up to date for commit: ${shortHash}`;
                      }
                      
                      return ev.message || payload || '';
                    })()}
                  </p>
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
                <span className="whitespace-pre-wrap break-all">{renderLogLine(line)}</span>
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
      <ConfirmationModal
        isOpen={isTimelineConfirmOpen}
        onClose={() => setIsTimelineConfirmOpen(false)}
        onConfirm={confirmTimelineClear}
        title={t('projectDetail.actions.clear')}
        message={t('common.confirmClearLogs') || 'Confirm clearing timeline events? New events will still appear.'}
        type="warning"
        confirmText={t('common.confirm')}
      />
    </div>
  )
}

export default BuildLogsConsole
