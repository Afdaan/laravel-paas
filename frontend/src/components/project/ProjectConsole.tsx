import React, { useState, useEffect, useRef, useMemo } from 'react'
import { Terminal as TerminalIcon, Copy, AlertTriangle, RefreshCw } from 'lucide-react'
import { AxiosError } from 'axios'
import { toast } from 'sonner'
import useTranslation from '../../lib/useTranslation'
import { Card, CardHeader } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import ConfirmationModal from '../ConfirmationModal'
import { projectsAPI } from '../../services/api'
import { Project } from '../../types'
import { cn } from '@/lib/utils'

interface ProjectConsoleProps {
  uid: string
  project: Project
}

export default function ProjectConsole({ uid, project }: ProjectConsoleProps) {
  const { t } = useTranslation()
  const isLaravelProject = project.framework === 'Laravel'
  const [consoleCommand, setConsoleCommand] = useState('')
  const [consoleOutput, setConsoleOutput] = useState('')
  const [consoleClearedLength, setConsoleClearedLength] = useState(0)
  const [isExecuting, setIsExecuting] = useState(false)
  const logsEndRef = useRef<HTMLDivElement>(null)

  const [confirmModal, setConfirmModal] = useState({
    isOpen: false,
    title: '',
    message: '' as React.ReactNode,
    type: 'danger' as 'danger' | 'warning' | 'info',
    onConfirm: () => { },
    confirmText: t('common.confirm')
  })

  const consoleLines = useMemo(() => {
    if (!consoleOutput) return []
    const visibleOutput = consoleClearedLength > 0 ? consoleOutput.substring(consoleClearedLength) : consoleOutput
    const lines = visibleOutput.split('\n').filter(l => l.trim() !== '' || l === '')
    return lines.length > 500 ? lines.slice(-500) : lines
  }, [consoleOutput, consoleClearedLength])

  const consoleOffset = useMemo(() => {
    if (!consoleOutput) return 0
    const visibleOutput = consoleClearedLength > 0 ? consoleOutput.substring(consoleClearedLength) : consoleOutput
    const lines = visibleOutput.split('\n').filter(l => l.trim() !== '' || l === '')
    return lines.length > 500 ? lines.length - 500 : 0
  }, [consoleOutput, consoleClearedLength])

  useEffect(() => {
    if (logsEndRef.current) {
      logsEndRef.current.scrollIntoView({ behavior: 'auto' })
    }
  }, [consoleLines])

  const handleClearConsole = () => {
    setConfirmModal({
      title: t('projectDetail.actions.clear'),
      message: t('common.confirmClearLogs') || 'Confirm clearing the console? New logs will still appear.',
      type: 'warning',
      confirmText: t('common.confirm'),
      isOpen: true,
      onConfirm: () => {
        setConsoleClearedLength(consoleOutput.length)
        toast.success(t('common.success'))
      }
    })
  }

  const executeConsoleCommand = async (cmd: string) => {
    setIsExecuting(true)
    setConsoleOutput(prev => prev + `\n$ ${isLaravelProject ? `php artisan ${cmd}` : cmd}\n`)

    try {
      const response = await projectsAPI.runArtisan(uid, cmd)
      setConsoleOutput(prev => prev + response.data.output + '\n')
      setConsoleCommand('')
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ output?: string; error?: string }>
      const errOut = axiosError.response?.data?.error || axiosError.response?.data?.output || axiosError.message
      setConsoleOutput(prev => prev + `Error: ${errOut}\n`)
    } finally {
      setIsExecuting(false)
    }
  }

  const handleConsoleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!uid || !consoleCommand.trim()) return

    const cmd = consoleCommand.trim()
    const isDestructive = isLaravelProject && (cmd === 'migrate:fresh' || cmd.startsWith('migrate:fresh '))

    if (isDestructive) {
      setConfirmModal({
        title: t('projectDetail.console.destructiveTitle') || 'Destructive Command Warning',
        message: t('projectDetail.console.destructiveMessage') || 'Warning: Running this command will drop all tables and delete all data!',
        type: 'danger',
        confirmText: t('projectDetail.console.destructiveConfirm') || 'Yes, proceed',
        isOpen: true,
        onConfirm: () => {
          setConfirmModal(prev => ({ ...prev, isOpen: false }))
          executeConsoleCommand(cmd)
        }
      })
    } else {
      executeConsoleCommand(cmd)
    }
  }

  return (
    <>
      <ConfirmationModal
        isOpen={confirmModal.isOpen}
        onClose={() => setConfirmModal(prev => ({ ...prev, isOpen: false }))}
        onConfirm={confirmModal.onConfirm}
        title={confirmModal.title}
        message={confirmModal.message}
        type={confirmModal.type}
        confirmText={confirmModal.confirmText}
      />

      <Card className="bg-zinc-950 text-white border-none overflow-hidden flex flex-col h-[600px] gap-0 py-0 shadow-2xl">
        <CardHeader className="bg-zinc-900/50 px-4 py-3 border-b border-white/5 flex flex-row items-center justify-between backdrop-blur-md">
          <div className="flex items-center gap-3">
            <div className="flex gap-1.5 mr-2">
              <div className="w-2.5 h-2.5 rounded-full bg-rose-500/80 shadow-[0_0_8px_rgba(244,63,94,0.4)]" />
              <div className="w-2.5 h-2.5 rounded-full bg-amber-500/80 shadow-[0_0_8px_rgba(245,158,11,0.4)]" />
              <div className="w-2.5 h-2.5 rounded-full bg-emerald-500/80 shadow-[0_0_8px_rgba(16,185,129,0.4)]" />
            </div>
            <div className="text-[10px] uppercase font-bold tracking-widest text-zinc-400 flex items-center gap-2">
              <TerminalIcon className="w-3.5 h-3.5" />
              {project.framework === 'Laravel' ? t('projectDetail.console.header') : 'Terminal Console'}
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => { navigator.clipboard.writeText(consoleOutput); toast.success(t('common.copySuccess')) }}
              className="p-1.5 hover:bg-white/10 rounded-md transition-colors text-zinc-500 hover:text-white cursor-pointer"
              title="Copy Logs"
            >
              <Copy className="w-3.5 h-3.5" />
            </button>
            <button
              onClick={() => {
                const el = document.getElementById('console-scroll-area');
                if (el) el.scrollTop = el.scrollHeight;
              }}
              className="p-1.5 hover:bg-white/10 rounded-md transition-colors text-zinc-500 hover:text-white cursor-pointer"
              title="Scroll to Bottom"
            >
              <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="m7 15 5 5 5-5" /><path d="m7 9 5 5 5-5" /></svg>
            </button>
            <div className="w-px h-3 bg-white/10 mx-1" />
            <Button variant="ghost" size="xs" onClick={handleClearConsole} className="text-[10px] uppercase font-bold text-zinc-600 hover:text-rose-400 cursor-pointer">{t('projectDetail.actions.clear')}</Button>
          </div>
        </CardHeader>

        <div id="console-scroll-area" className="flex-1 p-6 overflow-auto font-mono text-[11px] text-zinc-300 custom-scrollbar bg-zinc-950/50">
          <div className="text-amber-400/80 mb-6 flex flex-col gap-2 border-b border-white/5 pb-4">
            <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-wider">
              <AlertTriangle size={14} />
              <span>{isLaravelProject ? 'Prefix: php artisan' : 'Security Advisory'}</span>
            </div>
            <p className="text-[10px] text-zinc-500 leading-relaxed max-w-2xl italic">
              {isLaravelProject
                ? t('projectDetail.console.artisanPrefix')
                : 'Use standard CLI commands. Commands are executed in the project root. Dangerous operations are restricted.'}
            </p>
          </div>

          {consoleLines.length > 0 ? consoleLines.map((line: string, i: number) => (
            <div key={i} className={cn("flex gap-4 group py-0.5 px-2 rounded -mx-2 hover:bg-white/[0.05] transition-colors", line.startsWith('$') ? "text-primary mt-2 font-bold" : "text-zinc-400")}>
              <span className="shrink-0 text-zinc-800 select-none w-6 text-right font-light">{consoleOffset + i + 1}</span>
              <span className="break-all whitespace-pre-wrap">{line}</span>
            </div>
          )) : (
            <div className="h-full flex flex-col items-center justify-center opacity-10 gap-4">
              <TerminalIcon size={48} />
              <p className="uppercase tracking-[0.4em] font-bold text-xs">{t('projectDetail.console.terminalReady')}</p>
            </div>
          )}
          {isExecuting && <div className="mt-4 flex items-center gap-2 text-primary animate-pulse font-bold text-[10px] uppercase tracking-widest bg-primary/5 p-2 rounded border border-primary/20 w-fit"><RefreshCw className="w-3 h-3 animate-spin" /> {t('common.executing')}</div>}
          <div ref={logsEndRef} />
        </div>

        <form onSubmit={handleConsoleSubmit} className="p-4 bg-zinc-900/80 border-t border-white/5 flex gap-3">
          {isLaravelProject && (
            <div className="flex items-center px-4 bg-zinc-800 rounded font-mono text-[10px] font-bold text-zinc-500 border border-white/5">php artisan</div>
          )}
          <Input
            value={consoleCommand}
            onChange={e => setConsoleCommand(e.target.value)}
            placeholder={isLaravelProject ? 'migrate --seed' : 'npm run build'}
            disabled={isExecuting}
            className="flex-1 bg-zinc-800/50 border-white/10 text-white font-mono text-xs focus-visible:ring-1 focus-visible:ring-primary h-10 shadow-inner"
          />
          <Button type="submit" disabled={isExecuting || !consoleCommand.trim()} size="sm" className="h-10 px-6 font-bold uppercase tracking-widest text-[10px]">{t('projectDetail.actions.execute')}</Button>
        </form>
      </Card>
    </>
  )
}
