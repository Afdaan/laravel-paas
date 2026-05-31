import { useState, useEffect, useCallback } from 'react'
import { useParams } from 'react-router-dom'
import { toast } from 'sonner'
import {
  Database,
  RefreshCw,
  Shield,
  ShieldAlert,
  AlertTriangle
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import useTranslation from '@/lib/useTranslation'
import ConfirmationModal from '@/components/ConfirmationModal'
import { databaseAPI } from '../../services/api'
import { DatabaseBackup, DatabaseInstance, DatabaseMetrics } from '../../types'

// Import modular subcomponents
import { StudioProvider, ConfirmationModalOptions, SchemaTable } from '@/components/database-studio/StudioContext'
import { StudioDashboardTab } from '@/components/database-studio/StudioDashboardTab'
import { StudioTablesTab } from '@/components/database-studio/StudioTablesTab'
import { StudioStructureTab } from '@/components/database-studio/StudioStructureTab'
import { StudioQueryTab } from '@/components/database-studio/StudioQueryTab'
import { StudioBackupsTab } from '@/components/database-studio/StudioBackupsTab'

interface DatabaseStudioProps {
  projectId?: string | number | null;
  embedded?: boolean;
}

function DatabaseStudio({ projectId = null, embedded = false }: DatabaseStudioProps) {
  const params = useParams<{ id: string }>()
  const id = projectId || params.id
  const { t } = useTranslation()
  
  const [activeTab, setActiveTab] = useState<'dashboard' | 'tables' | 'structure' | 'query' | 'backups'>('dashboard')
  const [isLoading, setIsLoading] = useState(true)
  const [isActionLoading, setIsActionLoading] = useState(false)
  
  // Data states
  const [dbOverview, setDbOverview] = useState<DatabaseInstance | null>(null)
  const [schemaData, setSchemaData] = useState<SchemaTable[]>([])
  const [backups, setBackups] = useState<DatabaseBackup[]>([])
  const [metrics, setMetrics] = useState<DatabaseMetrics | null>(null)

  // Confirmation Modal state
  const [confirmModal, setConfirmModal] = useState<{
    isOpen: boolean;
    title: string;
    message: string;
    type: 'danger' | 'warning' | 'info';
    confirmText?: string;
    cancelText?: string;
    onConfirm: () => void;
  }>({
    isOpen: false,
    title: '',
    message: '',
    type: 'danger',
    onConfirm: () => {}
  })

  const triggerConfirmation = (options: ConfirmationModalOptions) => {
    setConfirmModal({
      isOpen: true,
      ...options
    })
  }

  const closeConfirmation = () => {
    setConfirmModal(prev => ({ ...prev, isOpen: false }))
  }

  // Load complete studio dataset
  const loadStudioData = useCallback(async (silent = true) => {
    if (!id) return
    if (!silent) {
      setIsLoading(true)
    } else {
      setIsActionLoading(true)
    }
    try {
      const overviewRes = await databaseAPI.getOverview(id)
      setDbOverview(overviewRes.data)
      
      const schemaRes = await databaseAPI.getSchema(id)
      const tables = schemaRes.data.tables || []
      setSchemaData(tables)
      
      const backupsRes = await databaseAPI.listBackups(id)
      setBackups(backupsRes.data.backups || [])
      
      const metricsRes = await databaseAPI.getMetrics(id)
      setMetrics(metricsRes.data)
    } catch (error) {
      toast.error(t('databaseStudio.errors.connectFailed'))
    } finally {
      if (!silent) {
        setIsLoading(false)
      } else {
        setIsActionLoading(false)
      }
    }
  }, [id, t])

  useEffect(() => {
    loadStudioData(false)
  }, [loadStudioData])

  const handleToggleStatus = (suspend: boolean) => {
    if (!id) return
    triggerConfirmation({
      title: suspend ? t('databaseStudio.dashboard.actions.suspendDatabase') : t('databaseStudio.dashboard.actions.resumeDatabase'),
      message: suspend ? t('databaseStudio.dashboard.confirmSuspend') : t('databaseStudio.dashboard.confirmResume'),
      type: suspend ? 'danger' : 'info',
      confirmText: suspend ? t('databaseStudio.dashboard.actions.suspendDatabase') : t('databaseStudio.dashboard.actions.resumeDatabase'),
      onConfirm: async () => {
        setIsActionLoading(true)
        try {
          const res = await databaseAPI.updateStatus(id, suspend)
          toast.success(res.data.message)
          loadStudioData()
        } catch (error) {
          toast.error(t('databaseStudio.errors.updateStatusFailed'))
        } finally {
          setIsActionLoading(false)
        }
      }
    })
  }

  if (isLoading) {
    return (
      <div className="flex flex-col items-center justify-center min-h-[500px] gap-4">
        <LoaderSpinner className="w-12 h-12 text-primary animate-spin" />
        <p className="text-muted-foreground text-sm font-semibold tracking-wide uppercase">{t('databaseStudio.dashboard.connecting')}</p>
      </div>
    )
  }

  const instanceStatus = dbOverview?.status || 'active'
  const isSuspended = instanceStatus === 'suspended'

  const studioContextValue = {
    id: id || '',
    dbOverview,
    schemaData,
    backups,
    metrics,
    isActionLoading,
    setIsActionLoading,
    loadStudioData,
    triggerConfirmation,
    setActiveTab,
    t
  }

  return (
    <StudioProvider value={studioContextValue}>
      <div className="space-y-8 animate-in fade-in duration-500 pb-20">
        {/* Studio Header */}
        {!embedded && (
          <div className="flex flex-col md:flex-row items-start md:items-center justify-between gap-4 border-b border-border/40 pb-6">
            <div className="space-y-1.5">
              <div className="flex items-center gap-3">
                <div className="w-10 h-10 rounded-xl bg-primary/10 flex items-center justify-center text-primary border border-primary/20">
                  <Database className="w-5 h-5" />
                </div>
                <div>
                  <h1 className="text-3xl font-extrabold tracking-tight">
                    {t('databaseStudio.dashboard.title').split(' ')[0]} <span className="text-primary italic">{t('databaseStudio.dashboard.title').split(' ')[1]}</span>
                  </h1>
                  <p className="text-muted-foreground text-xs uppercase tracking-widest font-bold">{t('databaseStudio.dashboard.subtitle')}</p>
                </div>
              </div>
            </div>

            <div className="flex items-center gap-3 shrink-0">
              <Button
                variant="outline"
                size="sm"
                onClick={() => loadStudioData(true)}
                disabled={isActionLoading}
                className="gap-2 h-10 cursor-pointer"
                style={{ cursor: 'pointer' }}
              >
                <RefreshCw className={cn("w-4 h-4", isActionLoading && "animate-spin")} />
                {t('databaseStudio.dashboard.actions.syncState')}
              </Button>

              {isSuspended ? (
                <Button
                  variant="default"
                  size="sm"
                  onClick={() => handleToggleStatus(false)}
                  disabled={isActionLoading}
                  className="gap-2 h-10 bg-emerald-600 hover:bg-emerald-700 text-white font-bold cursor-pointer"
                  style={{ cursor: 'pointer' }}
                >
                  <Shield className="w-4 h-4" />
                  {t('databaseStudio.dashboard.actions.resumeDatabase')}
                </Button>
              ) : (
                <Button
                  variant="destructive"
                  size="sm"
                  onClick={() => handleToggleStatus(true)}
                  disabled={isActionLoading}
                  className="gap-2 h-10 font-bold cursor-pointer"
                  style={{ cursor: 'pointer' }}
                >
                  <ShieldAlert className="w-4 h-4 cursor-pointer" style={{ cursor: 'pointer' }} />
                  {t('databaseStudio.dashboard.actions.suspendDatabase')}
                </Button>
              )}
            </div>
          </div>
        )}

        {/* Tab Navigation */}
        <div className="flex border-b border-border/60 p-1 bg-muted/20 rounded-xl w-fit">
          {(['dashboard', 'tables', 'structure', 'query', 'backups'] as const).map((tab) => (
            <button
              key={tab}
              onClick={() => setActiveTab(tab)}
              className={cn(
                "px-5 py-2.5 rounded-lg text-sm font-bold capitalize transition-all duration-200 cursor-pointer",
                activeTab === tab 
                  ? "bg-background text-primary shadow-sm border border-border/40" 
                  : "text-muted-foreground hover:text-foreground"
              )}
              style={{ cursor: 'pointer' }}
            >
              {t(`databaseStudio.tabs.${tab}`)}
            </button>
          ))}
        </div>

        {/* Database Warning if Suspended */}
        {isSuspended && (
          <Card className="border-destructive/30 bg-destructive/5 p-5 animate-in slide-in-from-top-2 duration-300">
            <div className="flex items-start gap-4">
              <div className="p-2.5 bg-destructive/10 rounded-lg text-destructive shrink-0">
                <AlertTriangle className="w-5 h-5" />
              </div>
              <div>
                <h4 className="font-extrabold uppercase tracking-wide text-destructive text-sm">{t('databaseStudio.dashboard.suspendedTitle')}</h4>
                <p className="text-muted-foreground text-xs mt-1 leading-relaxed">
                  {t('databaseStudio.dashboard.suspendedDesc')}
                </p>
              </div>
            </div>
          </Card>
        )}

        {/* Tab Contents */}
        <div className="mt-8">
          {activeTab === 'dashboard' && <StudioDashboardTab />}
          {activeTab === 'tables' && <StudioTablesTab />}
          {activeTab === 'structure' && <StudioStructureTab />}
          {activeTab === 'query' && <StudioQueryTab />}
          {activeTab === 'backups' && <StudioBackupsTab />}
        </div>

        {/* Structured Confirmation Modal Overlay */}
        <ConfirmationModal
          isOpen={confirmModal.isOpen}
          onClose={closeConfirmation}
          onConfirm={confirmModal.onConfirm}
          title={confirmModal.title}
          message={confirmModal.message}
          type={confirmModal.type}
          confirmText={confirmModal.confirmText || t('common.confirm')}
          cancelText={confirmModal.cancelText || t('common.cancel')}
        />
      </div>
    </StudioProvider>
  )
}

function LoaderSpinner({ className }: { className?: string }) {
  return (
    <svg 
      className={className} 
      xmlns="http://www.w3.org/2000/svg" 
      fill="none" 
      viewBox="0 0 24 24"
    >
      <circle 
        className="opacity-25" 
        cx="12" 
        cy="12" 
        r="10" 
        stroke="currentColor" 
        strokeWidth="4"
      />
      <path 
        className="opacity-75" 
        fill="currentColor" 
        d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
      />
    </svg>
  )
}

export default DatabaseStudio
