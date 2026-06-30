import { memo } from 'react'
import { Activity, Copy, ChevronsDown, RefreshCw } from 'lucide-react'
import { Card, CardHeader } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { toast } from 'sonner'
import useTranslation from '@/lib/useTranslation'
import { Project } from '@/types'
import { cn } from '@/lib/utils'

interface LogsTabProps {
  project: Project
  logType: 'web' | 'worker'
  setLogType: (type: 'web' | 'worker') => void
  visibleLogLines: string[]
  visibleLogsText: string
  logOffset: number
  logsEndRef: React.RefObject<HTMLDivElement | null>
  onClearLogs: () => void
  onRefreshLogs: () => void
}

export const LogsTab = memo(function LogsTab({
  project,
  logType,
  setLogType,
  visibleLogLines,
  visibleLogsText,
  logOffset,
  logsEndRef,
  onClearLogs,
  onRefreshLogs,
}: LogsTabProps) {
  const { t } = useTranslation()

  return (
    <Card className="bg-zinc-950 text-zinc-300 border-none overflow-hidden flex flex-col h-[600px] gap-0 py-0 shadow-2xl">
      <CardHeader className="bg-zinc-900/50 px-4 py-3 border-b border-white/5 flex flex-row items-center justify-between backdrop-blur-md">
        <div className="flex items-center gap-3">
          <div className="flex gap-1.5 mr-2">
            <div className="w-2.5 h-2.5 rounded-full bg-rose-500/80 shadow-[0_0_8px_rgba(244,63,94,0.4)]" />
            <div className="w-2.5 h-2.5 rounded-full bg-amber-500/80 shadow-[0_0_8px_rgba(245,158,11,0.4)]" />
            <div className="w-2.5 h-2.5 rounded-full bg-emerald-500/80 shadow-[0_0_8px_rgba(16,185,129,0.4)]" />
          </div>
          <div className="text-[10px] uppercase font-bold tracking-widest text-zinc-400 flex items-center gap-2">
            <Activity className="w-3.5 h-3.5 text-primary" />
            {t('projectDetail.logs.header')}
          </div>
          <div className="h-4 w-px bg-white/10 mx-2" />
          <div className="flex bg-zinc-950 p-0.5 rounded-md border border-white/5">
            <Button
              variant={logType === 'web' ? "default" : "ghost"}
              size="xs"
              onClick={() => setLogType('web')}
              className={cn(
                "px-3 py-1 rounded text-[9px] font-bold uppercase tracking-wider",
                logType !== 'web' && "text-zinc-500 hover:text-zinc-300"
              )}
            >
              {t('projectDetail.logs.web')}
            </Button>
            <Button
              variant={logType === 'worker' ? "default" : "ghost"}
              size="xs"
              onClick={() => setLogType('worker')}
              disabled={!project.worker_container_id && !project.queue_enabled}
              className={cn(
                "px-3 py-1 rounded text-[9px] font-bold uppercase tracking-wider",
                logType !== 'worker' && "text-zinc-500 hover:text-zinc-300"
              )}
            >
              {t('projectDetail.logs.worker')}
            </Button>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="icon"
            onClick={() => { navigator.clipboard.writeText(visibleLogsText); toast.success(t('common.copySuccess')) }}
            className="h-7 w-7 text-zinc-500 hover:text-white hover:bg-white/10"
            title={t('projectDetail.logs.copy')}
          >
            <Copy className="w-3.5 h-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => {
              const el = document.getElementById('runtime-logs-scroll');
              if (el) el.scrollTop = el.scrollHeight;
            }}
            className="h-7 w-7 text-zinc-500 hover:text-white hover:bg-white/10"
            title={t('projectDetail.logs.scrollToBottom')}
          >
            <ChevronsDown className="w-3.5 h-3.5" />
          </Button>
          <div className="w-px h-3 bg-white/10 mx-1" />
          <Button variant="ghost" size="xs" onClick={onClearLogs} className="text-[10px] uppercase font-bold text-zinc-600 hover:text-rose-400 cursor-pointer">{t('projectDetail.actions.clear')}</Button>
          <Button variant="ghost" size="xs" onClick={onRefreshLogs} className="h-6 w-6 cursor-pointer"><RefreshCw size={12} /></Button>
        </div>
      </CardHeader>
      <div id="runtime-logs-scroll" className="flex-1 p-6 overflow-auto font-mono text-[11px] leading-relaxed custom-scrollbar bg-zinc-950">
        {visibleLogLines.length > 0 ? visibleLogLines.map((line: string, i: number) => {
          const isTimestamp = /^\d{4}-\d{2}-\d{2}/.test(line) || /^\[\d{2}-\w{3}-\d{4}/.test(line)
          return (
            <div key={i} className="flex gap-4 group py-0.5 px-2 rounded -mx-2 hover:bg-white/[0.05] transition-colors">
              <span className="shrink-0 text-zinc-800 select-none w-8 text-right font-light">{logOffset + i + 1}</span>
              <span className="whitespace-pre-wrap font-mono">
                {isTimestamp ? (
                  <>
                    <span className="text-zinc-600 mr-2">{line.split(' ')[0]}</span>
                    <span>{line.split(' ').slice(1).join(' ')}</span>
                  </>
                ) : line}
              </span>
            </div>
          )
        }) : (
          <div className="h-full flex flex-col items-center justify-center opacity-10 gap-4">
            <Activity size={48} />
            <p className="uppercase tracking-[0.4em] font-bold text-xs">{t('projectDetail.logs.waiting')}</p>
          </div>
        )}
        <div ref={logsEndRef as React.LegacyRef<HTMLDivElement>} />
      </div>
    </Card>
  )
})
