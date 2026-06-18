import { toast } from 'sonner'
import {
  History,
  Plus,
  Download,
  RefreshCw,
  Trash2
} from 'lucide-react'
import { Card } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { databaseAPI } from '../../services/api'
import { useStudio } from './StudioContext'
import { formatHumanDatetime } from './utils'
import { cn } from '@/lib/utils'

export function StudioBackupsTab() {
  const {
    id,
    dbOverview,
    backups,
    loadStudioData,
    isActionLoading,
    setIsActionLoading,
    triggerConfirmation,
    t
  } = useStudio()

  const handleCreateBackup = async () => {
    if (!id) return
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.createBackup(id)
      toast.success(res.data.message)
      loadStudioData()
    } catch (error) {
      toast.error(t('databaseStudio.errors.createBackupFailed'))
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleRestoreBackup = (backupId: number) => {
    if (!id) return
    triggerConfirmation({
      title: t('databaseStudio.backups.confirmRestoreTitle'),
      message: t('databaseStudio.backups.confirmRestoreDesc'),
      type: 'danger',
      confirmText: t('databaseStudio.backups.confirmRestoreAction'),
      onConfirm: async () => {
        setIsActionLoading(true)
        try {
          const res = await databaseAPI.restoreBackup(id, backupId)
          toast.success(res.data.message)
          loadStudioData()
        } catch (error) {
          toast.error(t('databaseStudio.errors.restoreBackupFailed'))
        } finally {
          setIsActionLoading(false)
        }
      }
    })
  }

  const handleDeleteBackup = (backupId: number) => {
    if (!id) return
    triggerConfirmation({
      title: t('databaseStudio.backups.confirmDeleteTitle'),
      message: t('databaseStudio.backups.confirmDeleteDesc'),
      type: 'danger',
      confirmText: t('databaseStudio.backups.confirmDeleteAction'),
      onConfirm: async () => {
        setIsActionLoading(true)
        try {
          const res = await databaseAPI.deleteBackup(id, backupId)
          toast.success(res.data.message)
          loadStudioData()
        } catch (error) {
          toast.error(t('databaseStudio.errors.deleteBackupFailed'))
        } finally {
          setIsActionLoading(false)
        }
      }
    })
  }

  const handleDownloadBackup = async (backupId: number, filename: string) => {
    if (!id) return
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.downloadBackup(id, backupId)
      const blob = new Blob([res.data], { type: 'application/octet-stream' })
      const url = window.URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.setAttribute('download', filename)
      document.body.appendChild(link)
      link.click()
      link.remove()
      window.URL.revokeObjectURL(url)
      toast.success(t('common.success') || 'Download started successfully')
    } catch (error) {
      toast.error(t('databaseStudio.errors.downloadBackupFailed'))
    } finally {
      setIsActionLoading(false)
    }
  }

  const isSuspended = dbOverview?.status === 'suspended'

  return (
    <div className="grid grid-cols-1 lg:grid-cols-4 gap-6 items-stretch animate-in fade-in duration-300">
      {/* Left Column: Backup List */}
      <Card className="lg:col-span-3 p-6 flex flex-col overflow-hidden gap-5">
        <div className="flex items-center justify-between border-b pb-4">
          <div className="flex items-center gap-3">
            <History className="w-5 h-5 text-primary" />
            <div>
              <h3 className="font-extrabold text-base">{t('databaseStudio.backups.title')}</h3>
              <p className="text-muted-foreground text-xs">{t('databaseStudio.backups.desc')}</p>
            </div>
          </div>
          {!isSuspended && (
            <Button
              onClick={handleCreateBackup}
              disabled={isActionLoading}
              className="font-bold gap-1.5 h-10 px-4 rounded-xl bg-primary hover:bg-primary/90 text-primary-foreground shadow-sm shrink-0 cursor-pointer"
              style={{ cursor: 'pointer' }}
            >
              <Plus className="w-4 h-4" />
              {t('databaseStudio.backups.captureBtn')}
            </Button>
          )}
        </div>

        {isSuspended ? (
          <div className="py-12 text-center text-muted-foreground text-sm font-semibold uppercase tracking-wide">
            {t('databaseStudio.dashboard.suspendedWarning')}
          </div>
        ) : (
          <div className="space-y-4 flex-1 flex flex-col min-h-0">
            <div className="overflow-x-auto border border-border/80 rounded-xl bg-background/30 max-h-[420px] flex-1">
              <table className="w-full text-left border-collapse text-xs font-medium">
                <thead>
                  <tr className="bg-muted border-b border-border/80 text-[10px] font-bold uppercase tracking-widest text-muted-foreground sticky top-0 z-10">
                    <th className="py-3.5 px-4 bg-muted">{t('databaseStudio.tables.actionHeader')}</th>
                    <th className="py-3.5 px-4 bg-muted">File Name</th>
                    <th className="py-3.5 px-4 bg-muted">Size</th>
                    <th className="py-3.5 px-4 bg-muted">Date</th>
                  </tr>
                </thead>
                <tbody>
                  {backups.length === 0 ? (
                    <tr>
                      <td colSpan={4} className="py-12 text-center text-muted-foreground italic font-semibold">
                        <div className="flex flex-col items-center justify-center gap-2">
                          <History className="w-8 h-8 text-muted-foreground/40" />
                          <span>{t('databaseStudio.backups.empty')}</span>
                          <span className="text-[10px] text-muted-foreground font-normal max-w-sm leading-normal">
                            {t('databaseStudio.backups.emptyDesc')}
                          </span>
                        </div>
                      </td>
                    </tr>
                  ) : (
                    backups.map(backup => (
                      <tr key={backup.id} className="border-b border-border/40 hover:bg-muted/15">
                        <td className="py-3.5 px-4 shrink-0">
                          <div className="flex items-center gap-2">
                            <Button
                              variant="outline"
                              size="icon"
                              onClick={() => handleDownloadBackup(backup.id, backup.name)}
                              disabled={isActionLoading}
                              className="h-8 w-8 hover:bg-muted/50 cursor-pointer"
                              title={t('databaseStudio.backups.actions.download')}
                            >
                              <Download className="w-3.5 h-3.5 text-muted-foreground" />
                            </Button>
                            <Button
                              variant="outline"
                              size="icon"
                              onClick={() => handleRestoreBackup(backup.id)}
                              disabled={isActionLoading}
                              className="h-8 w-8 hover:bg-muted/50 text-emerald-500 hover:text-emerald-600 border border-emerald-500/20 hover:border-emerald-500 cursor-pointer"
                              title={t('databaseStudio.backups.actions.restore')}
                            >
                              <RefreshCw className="w-3.5 h-3.5" />
                            </Button>
                            <Button
                              variant="outline"
                              size="icon"
                              onClick={() => handleDeleteBackup(backup.id)}
                              disabled={isActionLoading}
                              className="h-8 w-8 hover:bg-muted/50 text-destructive border border-destructive/20 hover:border-destructive cursor-pointer"
                              title={t('databaseStudio.backups.actions.prune')}
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </Button>
                          </div>
                        </td>
                        <td className="py-3.5 px-4 font-mono text-foreground font-bold truncate max-w-[200px]" title={backup.name}>
                          {backup.name}
                        </td>
                        <td className="py-3.5 px-4 font-mono text-muted-foreground">
                          {backup.size || '0 B'}
                        </td>
                        <td className="py-3.5 px-4 font-mono text-muted-foreground">
                          {formatHumanDatetime(backup.created_at)}
                        </td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </Card>

      {/* Right Column: Capacity gauge */}
      <div className="lg:col-span-1 space-y-6 flex flex-col min-h-0">
        <Card className="p-5 space-y-4 shrink-0">
          <h4 className="font-extrabold text-xs uppercase tracking-wider text-muted-foreground border-b pb-2">
            {t('databaseStudio.backups.retentionTitle')}
          </h4>

          <div className="space-y-4">
            <div className="space-y-2">
              <div className="flex justify-between items-center text-xs">
                <span className="font-bold text-muted-foreground uppercase tracking-wider">{t('databaseStudio.backups.catalogCapacity')}</span>
                <span className="font-mono text-xs font-bold text-foreground">
                  {backups.length} <span className="text-muted-foreground/60">/ 5 {t('databaseStudio.backups.snapshotsLabel')}</span>
                </span>
              </div>
              <div className="h-1.5 w-full bg-muted rounded-full overflow-hidden">
                <div
                   className={cn("h-full transition-all duration-500 bg-primary", backups.length >= 5 ? 'bg-destructive' : backups.length >= 4 ? 'bg-amber-500' : 'bg-primary')}
                   style={{ width: `${Math.min((backups.length / 5) * 100, 100)}%` }}
                />
              </div>
            </div>

            <p className="text-[10px] text-muted-foreground leading-normal font-medium">
              {t('databaseStudio.backups.retentionDesc')}
            </p>
          </div>
        </Card>
      </div>
    </div>
  )
}
