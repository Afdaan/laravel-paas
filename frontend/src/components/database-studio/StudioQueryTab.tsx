import { useState, useEffect } from 'react'
import { toast } from 'sonner'
import {
  Play,
  Plus,
  Activity
} from 'lucide-react'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { databaseAPI } from '../../services/api'
import { useStudio } from './StudioContext'
import { formatCellValue } from './utils'

interface QueryResult {
  columns?: string[];
  rows?: Record<string, unknown>[];
  rows_affected?: number;
  time_ms?: number;
  error?: string;
}

export function StudioQueryTab() {
  const {
    id,
    dbOverview,
    isActionLoading,
    setIsActionLoading,
    triggerConfirmation,
    t
  } = useStudio()

  const [sqlQuery, setSqlQuery] = useState('SELECT * FROM users LIMIT 10;')
  const [queryResult, setQueryResult] = useState<QueryResult | null>(null)
  const [queryHistory, setQueryHistory] = useState<string[]>([])

  // Load query history on mount
  useEffect(() => {
    if (!id) return
    try {
      const saved = localStorage.getItem(`db_query_history_${id}`)
      setQueryHistory(saved ? JSON.parse(saved) : [])
    } catch (e) {
      setQueryHistory([])
    }
  }, [id])

  const handleExecuteSQL = async () => {
    if (!id || !sqlQuery.trim()) return
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.query(id, sqlQuery)
      setQueryResult(res.data)
      
      setQueryHistory(prev => {
        const newHistory = [sqlQuery, ...prev.filter(q => q !== sqlQuery).slice(0, 49)]
        try {
          localStorage.setItem(`db_query_history_${id}`, JSON.stringify(newHistory))
        } catch (e) {
          console.error(e)
        }
        return newHistory
      })
      
      toast.success(t('databaseStudio.query.successToast'))
    } catch (error) {
      const err = error as { response?: { data?: { error?: string } }; message?: string }
      const errMsg = err.response?.data?.error || err.message || 'Unknown error'
      setQueryResult({ error: errMsg })
      toast.error(t('databaseStudio.query.failedToast') + ': ' + errMsg)
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleClearQueryHistory = () => {
    triggerConfirmation({
      title: t('databaseStudio.query.history.clearConfirmTitle'),
      message: t('databaseStudio.query.history.clearConfirmDesc'),
      type: 'danger',
      confirmText: t('databaseStudio.query.history.clearConfirmAction'),
      onConfirm: () => {
        setQueryHistory([])
        try {
          localStorage.removeItem(`db_query_history_${id}`)
        } catch (e) {
          console.error(e)
        }
      }
    })
  }

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text)
    toast.success(t('common.copySuccess'))
  }

  const isSuspended = dbOverview?.status === 'suspended'

  return (
    <div className="grid grid-cols-1 lg:grid-cols-4 gap-6 items-stretch animate-in fade-in duration-300">
      {/* Left Column: SQL Workspace */}
      <Card className="lg:col-span-3 p-6 flex flex-col overflow-hidden gap-5">
        <div className="flex items-center justify-between border-b pb-4">
          <div className="flex items-center gap-3">
            <Play className="w-5 h-5 text-primary" />
            <div>
              <h3 className="font-extrabold text-base">{t('databaseStudio.query.title')}</h3>
              <p className="text-muted-foreground text-xs">{t('databaseStudio.query.subtitle')}</p>
            </div>
          </div>
          {!isSuspended && (
            <Button
              onClick={handleExecuteSQL}
              disabled={isActionLoading || !sqlQuery.trim()}
              className="font-bold gap-1.5 h-10 px-4 rounded-xl bg-primary hover:bg-primary/90 text-primary-foreground shadow-sm shrink-0 cursor-pointer"
              style={{ cursor: 'pointer' }}
            >
              <Play className="w-4 h-4" />
              {t('databaseStudio.query.runQuery')}
            </Button>
          )}
        </div>

        {isSuspended ? (
          <div className="py-12 text-center text-muted-foreground text-sm font-semibold uppercase tracking-wide">
            {t('databaseStudio.dashboard.suspendedWarning')}
          </div>
        ) : (
          <div className="space-y-5 flex-1 flex flex-col min-h-0">
            {/* Editor Area */}
            <div className="flex-1 min-h-[160px] relative border border-border/80 rounded-xl overflow-hidden bg-background/20 focus-within:border-primary/45 transition-colors">
              <textarea
                value={sqlQuery}
                onChange={(e) => setSqlQuery(e.target.value)}
                placeholder={t('databaseStudio.query.queryPlaceholder')}
                className="w-full h-full p-4 font-mono text-xs bg-transparent border-none outline-none focus:ring-0 resize-none text-foreground/90 leading-relaxed scrollbar-thin"
              />
            </div>

            {/* Results Grid / Output Logs */}
            <div className="border-t pt-5 flex-1 flex flex-col min-h-0">
              <h4 className="font-extrabold text-xs text-muted-foreground uppercase tracking-wider mb-3.5 flex items-center gap-2">
                <Activity className="w-4 h-4" />
                {t('databaseStudio.query.outputHeader')}
              </h4>

              <div className="flex-1 overflow-auto border border-border/70 rounded-xl bg-background/30 max-h-[300px] scrollbar-thin">
                {queryResult ? (
                  queryResult.error ? (
                    <div className="p-5 font-mono text-xs text-destructive bg-destructive/5 rounded-xl border border-destructive/10 leading-relaxed whitespace-pre-wrap select-all">
                      <span className="font-bold uppercase text-[10px] tracking-wider block mb-1">{t('databaseStudio.query.errorLabel')}</span>
                      {queryResult.error}
                    </div>
                  ) : queryResult.columns && queryResult.rows ? (
                    queryResult.rows.length === 0 ? (
                      <div className="p-8 text-center text-xs text-muted-foreground italic font-semibold">
                        {t('databaseStudio.query.noRecords')}
                      </div>
                    ) : (
                      <table className="w-full text-left border-collapse text-xs font-medium">
                        <thead>
                          <tr className="bg-muted border-b border-border/80 text-[10px] font-bold uppercase tracking-widest text-muted-foreground sticky top-0 z-10">
                            {queryResult.columns.map((col: string) => (
                              <th key={col} className="py-3 px-4 font-mono font-semibold bg-muted">{col}</th>
                            ))}
                          </tr>
                        </thead>
                        <tbody>
                          {queryResult.rows.map((row: Record<string, unknown>, idx: number) => (
                            <tr key={idx} className="border-b border-border/40 hover:bg-muted/15">
                              {queryResult.columns?.map((col: string) => (
                                <td key={col} className="py-3 px-4 font-mono whitespace-nowrap overflow-hidden text-ellipsis max-w-[200px]" title={String(row[col] ?? '')}>
                                  {formatCellValue(row[col])}
                                </td>
                              ))}
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    )
                  ) : (
                    <div className="p-5 font-mono text-xs text-emerald-500 bg-emerald-500/5 rounded-xl border border-emerald-500/10 flex flex-col gap-1">
                      <span className="font-bold text-[10px] uppercase tracking-widest">{t('databaseStudio.query.runQuery')}</span>
                      <span>{t('databaseStudio.query.successMsg', { count: queryResult.rows_affected || 0 })}</span>
                      {queryResult.time_ms && (
                        <span className="text-[10px] text-muted-foreground/80 mt-1">
                          {t('databaseStudio.query.queryDuration')}: {queryResult.time_ms.toFixed(2)} ms
                        </span>
                      )}
                    </div>
                  )
                ) : (
                  <div className="p-10 text-center text-xs text-muted-foreground/50 italic font-semibold">
                    {t('databaseStudio.query.awaiting')}
                  </div>
                )}
              </div>
            </div>
          </div>
        )}
      </Card>

      {/* Right Column: Query Templates & History */}
      <div className="lg:col-span-1 space-y-6 flex flex-col min-h-0">
        {/* SQL Templates Card */}
        {!isSuspended && (
          <Card className="p-5 space-y-4 shrink-0">
            <h4 className="font-extrabold text-xs uppercase tracking-wider text-muted-foreground border-b pb-2">
              {t('databaseStudio.query.templates.label')}
            </h4>
            <div className="flex flex-col gap-2.5">
              <button
                onClick={() => setSqlQuery('SELECT * FROM users LIMIT 10;')}
                className="w-full p-2.5 rounded-lg border border-border/80 hover:bg-muted/40 text-left transition-all text-xs font-semibold text-muted-foreground hover:text-foreground flex items-center justify-between cursor-pointer"
                style={{ cursor: 'pointer' }}
              >
                <span>{t('databaseStudio.query.templates.select')}</span>
                <Plus className="w-3.5 h-3.5 text-primary rotate-45" />
              </button>

              <button
                onClick={() => setSqlQuery('SELECT COUNT(*) as count FROM users;')}
                className="w-full p-2.5 rounded-lg border border-border/80 hover:bg-muted/40 text-left transition-all text-xs font-semibold text-muted-foreground hover:text-foreground flex items-center justify-between cursor-pointer"
                style={{ cursor: 'pointer' }}
              >
                <span>{t('databaseStudio.query.templates.count')}</span>
                <Plus className="w-3.5 h-3.5 text-primary rotate-45" />
              </button>

              <button
                onClick={() => setSqlQuery('SELECT * FROM users WHERE email LIKE \'%@gmail.com\' ORDER BY id DESC LIMIT 5;')}
                className="w-full p-2.5 rounded-lg border border-border/80 hover:bg-muted/40 text-left transition-all text-xs font-semibold text-muted-foreground hover:text-foreground flex items-center justify-between cursor-pointer"
                style={{ cursor: 'pointer' }}
              >
                <span>{t('databaseStudio.query.templates.filter')}</span>
                <Plus className="w-3.5 h-3.5 text-primary rotate-45" />
              </button>
            </div>
          </Card>
        )}

        {/* History Catalog Card */}
        <Card className="p-5 flex-1 flex flex-col min-h-0">
          <div className="flex items-center justify-between border-b pb-2 mb-3 shrink-0">
            <h4 className="font-extrabold text-xs uppercase tracking-wider text-muted-foreground">
              {t('databaseStudio.query.history.title')}
            </h4>
            {queryHistory.length > 0 && !isSuspended && (
              <button
                onClick={handleClearQueryHistory}
                className="text-[10px] font-bold text-destructive hover:underline cursor-pointer"
                style={{ cursor: 'pointer' }}
              >
                {t('databaseStudio.query.history.clearAll')}
              </button>
            )}
          </div>

          <div className="flex-1 overflow-y-auto space-y-2 pr-1 scrollbar-thin max-h-[300px]">
            {isSuspended ? (
              <div className="text-center py-6 text-xs text-muted-foreground/50 italic font-semibold">{t('databaseStudio.dashboard.suspendedTitle')}</div>
            ) : queryHistory.length === 0 ? (
              <div className="text-center py-6 text-xs text-muted-foreground/50 italic font-semibold">{t('databaseStudio.query.history.emptyHistory')}</div>
            ) : (
              queryHistory.map((query, idx) => (
                <div key={idx} className="p-3 border rounded-xl bg-muted/5 flex flex-col gap-2.5 relative group hover:border-primary/20 transition-all">
                  <div className="font-mono text-[10px] break-all whitespace-pre-wrap select-all text-foreground/80 leading-normal max-h-[70px] overflow-hidden">
                    {query}
                  </div>
                  <div className="flex items-center gap-2 justify-end pt-1 border-t border-border/40">
                    <button
                      onClick={() => setSqlQuery(query)}
                      className="text-[9px] font-black uppercase text-primary hover:underline cursor-pointer"
                      style={{ cursor: 'pointer' }}
                    >
                      {t('databaseStudio.query.history.btnRestore')}
                    </button>
                    <button
                      onClick={() => copyToClipboard(query)}
                      className="text-[9px] font-black uppercase text-muted-foreground hover:text-foreground cursor-pointer"
                      style={{ cursor: 'pointer' }}
                    >
                      {t('databaseStudio.query.history.btnCopy')}
                    </button>
                  </div>
                </div>
              ))
            )}
          </div>
        </Card>
      </div>
    </div>
  )
}
