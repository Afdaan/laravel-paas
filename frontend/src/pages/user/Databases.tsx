import {
  PackageOpen,
  Database as DbIcon,
  Search,
  ArrowRight,
  Loader2,
  RefreshCw,
  Trash2,
  Settings,
  AlertTriangle,
  ExternalLink,
  Layers,
  Lock,
  Server,
  HardDrive,
  Copy,
  Check
} from 'lucide-react'
import { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { databaseAPI, projectsAPI } from '../../services/api'
import { Project, DatabaseInstance } from '../../types'
import useTranslation from '../../lib/useTranslation'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Button, buttonVariants } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger } from '@/components/ui/select'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'

export default function Databases() {
  const { t } = useTranslation()
  const [databases, setDatabases] = useState<DatabaseInstance[]>([])
  const [projects, setProjects] = useState<Project[]>([])
  const [selectedDbId, setSelectedDbId] = useState<number | null>(null)
  const [isLoading, setIsLoading] = useState(true)
  const [isActionLoading, setIsActionLoading] = useState(false)
  const [search, setSearch] = useState('')
  const [copiedField, setCopiedField] = useState<string | null>(null)

  // Dialog states
  const [showResetModal, setShowResetModal] = useState(false)
  const [showReinstallModal, setShowReinstallModal] = useState(false)
  const [showRedeployModal, setShowRedeployModal] = useState(false)
  const [confirmText, setConfirmText] = useState('')
  const [selectedProjectId, setSelectedProjectId] = useState<string>('')
  const [redeployProjectUid, setRedeployProjectUid] = useState<string>('')

  const fetchData = useCallback(async () => {
    try {
      const [dbRes, projRes] = await Promise.all([
        databaseAPI.listOwn(),
        projectsAPI.listOwn()
      ])
      const dbs = dbRes.data.databases || []
      const projs = projRes.data.data || []
      setDatabases(dbs)
      setProjects(projs)

      // Auto-select first database if none selected
      if (dbs.length > 0 && !selectedDbId) {
        setSelectedDbId(dbs[0].id)
      }
    } catch (error) {
      toast.error(t('common.error'))
    } finally {
      setIsLoading(false)
    }
  }, [t, selectedDbId])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const handleCopy = (text: string, field: string) => {
    navigator.clipboard.writeText(text)
    setCopiedField(field)
    toast.success(t('databaseManager.copied', { label: field }))
    setTimeout(() => setCopiedField(null), 2000)
  }

  const selectedDb = databases.find(db => db.id === selectedDbId)

  // Filter databases
  const filteredDbs = databases.filter(db =>
    db.name.toLowerCase().includes(search.toLowerCase()) ||
    db.engine.toLowerCase().includes(search.toLowerCase()) ||
    (db.project_id && db.project && db.project.name.toLowerCase().includes(search.toLowerCase()))
  )

  // Projects that do not have a database attached
  const attachableProjects = projects.filter(p => !p.database_instance)

  // Attach database handler
  const handleAttach = async () => {
    if (!selectedDb || !selectedProjectId) return
    setIsActionLoading(true)
    try {
      await databaseAPI.attach(selectedDb.id, selectedProjectId)
      toast.success(t('common.success'))

      // Prompt for redeploy
      setRedeployProjectUid(selectedProjectId)
      setShowRedeployModal(true)
      setSelectedProjectId('')

      // Refresh list
      await fetchData()
    } catch (error: any) {
      toast.error(error.response?.data?.error || t('common.error'))
    } finally {
      setIsActionLoading(false)
    }
  }

  // Detach database handler
  const handleDetach = async () => {
    if (!selectedDb) return
    setIsActionLoading(true)
    try {
      const projectUid = selectedDb.project_id ? selectedDb.project?.uid : undefined
      await databaseAPI.detach(selectedDb.id)
      toast.success(t('common.success'))

      if (projectUid) {
        setRedeployProjectUid(projectUid)
        setShowRedeployModal(true)
      }

      await fetchData()
    } catch (error: any) {
      toast.error(error.response?.data?.error || t('common.error'))
    } finally {
      setIsActionLoading(false)
    }
  }

  // Reset database handler
  const handleReset = async () => {
    if (!selectedDb) return
    if (confirmText !== selectedDb.name) {
      toast.error(t('common.error'))
      return
    }
    setIsActionLoading(true)
    try {
      await databaseAPI.resetInstance(selectedDb.id)
      toast.success(t('databaseManager.querySuccess'))
      setShowResetModal(false)
      setConfirmText('')
      await fetchData()
    } catch (error: any) {
      toast.error(error.response?.data?.error || t('common.error'))
    } finally {
      setIsActionLoading(false)
    }
  }

  // Reinstall database handler
  const handleReinstall = async () => {
    if (!selectedDb) return
    if (confirmText !== selectedDb.name) {
      toast.error(t('common.error'))
      return
    }
    setIsActionLoading(true)
    try {
      const res = await databaseAPI.reinstallInstance(selectedDb.id)
      toast.success(t('databaseManager.backupSuccess'))
      setShowReinstallModal(false)
      setConfirmText('')

      if (res.data.redeployRequired && selectedDb.project_id && selectedDb.project?.uid) {
        setRedeployProjectUid(selectedDb.project.uid)
        setShowRedeployModal(true)
      }

      await fetchData()
    } catch (error: any) {
      toast.error(error.response?.data?.error || t('common.error'))
    } finally {
      setIsActionLoading(false)
    }
  }

  // Redeploy trigger handler
  const handleRedeploy = async () => {
    if (!redeployProjectUid) return
    setIsActionLoading(true)
    try {
      await projectsAPI.redeploy(redeployProjectUid)
      toast.success(t('common.success'))
      setShowRedeployModal(false)
    } catch (error: any) {
      toast.error(error.response?.data?.error || t('common.error'))
    } finally {
      setIsActionLoading(false)
    }
  }

  if (isLoading) {
    return (
      <div className="min-h-[60vh] flex flex-col items-center justify-center gap-4">
        <Loader2 className="w-10 h-10 text-primary animate-spin" />
        <p className="text-muted-foreground font-bold uppercase tracking-widest text-[10px] animate-pulse">{t('databaseManager.loading')}</p>
      </div>
    )
  }

  if (databases.length === 0) {
    return (
      <div className="min-h-[60vh] flex flex-col items-center justify-center gap-6 animate-in fade-in duration-500">
        <div className="w-20 h-20 rounded-3xl bg-muted border flex items-center justify-center">
          <PackageOpen className="w-10 h-10 text-muted-foreground" />
        </div>
        <div className="text-center max-w-sm space-y-2">
          <h3 className="text-2xl font-bold tracking-tight">
            {t('databaseManager.noDbsFound')}
          </h3>
          <p className="text-muted-foreground text-sm font-medium leading-relaxed italic">
            {t('databaseManager.noDbsDesc')}
          </p>
        </div>
        <Link to="/projects/new" className={cn(buttonVariants({ variant: "default" }), "mt-4")}>
          {t('databaseManager.createProjectWithDb')}
        </Link>
      </div>
    )
  }

  return (
    <div className="h-[calc(100vh-140px)] flex flex-col lg:flex-row gap-6 animate-in fade-in duration-500">

      {/* Sidebar - Database List */}
      <div className="w-full lg:w-72 flex-shrink-0 flex flex-col">
        <Card className="flex flex-col overflow-hidden h-full pt-0 shadow-sm">
          <CardHeader className="bg-muted/30 border-b py-4">
            <div className="flex items-center gap-3">
              <div className="w-9 h-9 rounded-lg bg-primary/10 border border-primary/20 flex items-center justify-center text-primary shrink-0">
                <DbIcon className="w-4 h-4" />
              </div>
              <div>
                <h2 className="text-sm font-bold tracking-tight uppercase">{t('common.databases')}</h2>
                <p className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider">{t('databaseManager.activeInstances')}</p>
              </div>
            </div>
            <div className="relative mt-3">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
              <Input
                placeholder={t('databaseManager.searchSchema')}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-9 h-9 text-xs"
              />
            </div>
          </CardHeader>

          <CardContent className="flex-1 overflow-y-auto p-1.5 space-y-1 pt-2 scrollbar-thin">
            {filteredDbs.length > 0 ? (
              filteredDbs.map(db => (
                <button
                  key={db.id}
                  onClick={() => setSelectedDbId(db.id)}
                  className={cn(
                    "w-full text-left p-3 rounded-lg transition-all border group focus:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
                    selectedDbId === db.id
                      ? 'bg-primary/10 border-primary/30 shadow-sm ring-1 ring-primary/10'
                      : 'border-transparent hover:bg-muted/80 hover:border-border/50'
                  )}
                >
                  <div className="flex items-center justify-between mb-1.5">
                    <span className={cn(
                      "font-bold text-xs tracking-tight truncate max-w-[150px]",
                      selectedDbId === db.id ? 'text-primary' : 'text-foreground'
                    )}>
                      {db.name}
                    </span>
                    <span className="flex items-center gap-1.5 shrink-0">
                      <span className={cn(
                        "w-1.5 h-1.5 rounded-full",
                        db.status === 'active' ? 'bg-emerald-500 animate-pulse' : 'bg-rose-500'
                      )} />
                      <Badge variant="outline" className={cn(
                        "text-[9px] font-bold uppercase px-1.5 py-0 h-4",
                        db.engine === 'mysql'
                          ? 'border-amber-500/20 bg-amber-500/5 text-amber-500'
                          : 'border-blue-500/20 bg-blue-500/5 text-blue-500'
                      )}>
                        {db.engine}
                      </Badge>
                    </span>
                  </div>

                  <div className="flex items-center justify-between pt-1.5 border-t border-border/50 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                    <span>{db.project_id !== null ? t('databaseManager.attachedTo') : t('databaseManager.unattached')}</span>
                    <span className={cn("truncate max-w-[100px] font-semibold", db.project_id !== null ? "text-primary" : "text-muted-foreground/50")}>
                      {db.project_id !== null && db.project ? db.project.name : '—'}
                    </span>
                  </div>
                </button>
              ))
            ) : (
              <div className="text-center py-12 text-muted-foreground font-semibold uppercase tracking-widest text-xs italic">
                {t('databaseManager.noClusters')}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Main Content - Database Details */}
      <Card className="flex-1 overflow-hidden flex flex-col relative shadow-sm">
        {selectedDb ? (
          <div className="flex-1 overflow-auto p-6 space-y-6 scrollbar-thin">
            {/* Header section */}
            <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 pb-5 border-b border-border/40">
              <div className="space-y-1.5">
                <div className="flex items-center gap-2.5 flex-wrap">
                  <h1 className="text-xl font-black tracking-tight text-foreground">{selectedDb.name}</h1>
                  <Badge variant={selectedDb.status === 'active' ? 'default' : 'destructive'} className="text-[10px] font-bold uppercase tracking-wide">
                    {selectedDb.status}
                  </Badge>
                  <Badge variant="outline" className={cn(
                    "text-[10px] font-bold uppercase tracking-wide",
                    selectedDb.engine === 'mysql'
                      ? 'border-amber-500/30 bg-amber-500/5 text-amber-500'
                      : 'border-blue-500/30 bg-blue-500/5 text-blue-500'
                  )}>
                    {selectedDb.engine}
                  </Badge>
                </div>
                <p className="text-xs text-muted-foreground flex items-center gap-1.5">
                  <Server className="w-3.5 h-3.5 shrink-0" />
                  {selectedDb.project_id !== null && selectedDb.project ? (
                    <>
                      {t('databaseManager.attachedTo')}{' '}
                      <Link to={`/projects/${selectedDb.project.uid}`} className="text-primary hover:underline font-semibold">
                        {selectedDb.project.name}
                      </Link>
                    </>
                  ) : (
                    t('databaseManager.unattached')
                  )}
                </p>
              </div>

              {/* Header Actions */}
              <div className="flex items-center gap-2 shrink-0">
                {selectedDb.project_id !== null && selectedDb.project ? (
                  <>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={handleDetach}
                      disabled={isActionLoading}
                      className="text-xs font-bold uppercase text-rose-500 border-rose-500/20 hover:bg-rose-500/5"
                    >
                      {t('databaseManager.detach')}
                    </Button>
                    <Link
                      to={`/projects/${selectedDb.project.uid}?tab=database`}
                      className={cn(buttonVariants({ variant: "default", size: "sm" }), "text-xs font-bold uppercase flex items-center gap-2")}
                    >
                      {t('databaseManager.openStudio')}
                      <ExternalLink className="w-3.5 h-3.5 shrink-0" />
                    </Link>
                  </>
                ) : (
                  <div className="flex items-center gap-2 w-full md:w-auto">
                    <Select
                      value={selectedProjectId}
                      onValueChange={(val) => setSelectedProjectId(val || '')}
                    >
                      <SelectTrigger className="w-[200px] h-8 px-3 text-xs">
                        <div className="flex items-center gap-2 text-left flex-1 min-w-0 pr-4">
                          {(() => {
                            const p = attachableProjects.find(proj => proj.uid === selectedProjectId)
                            return p ? (
                              <span className="truncate font-medium text-foreground text-xs">{p.name}</span>
                            ) : (
                              <span className="text-muted-foreground text-xs">{t('databaseManager.selectProject')}</span>
                            )
                          })()}
                        </div>
                      </SelectTrigger>
                      <SelectContent align="start" alignItemWithTrigger={false} className="bg-popover border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72 min-w-[var(--anchor-width)] w-[var(--anchor-width)]">
                        {attachableProjects.map(p => (
                          <SelectItem key={p.uid} value={p.uid} className="rounded-lg py-2 px-3 cursor-pointer">
                            <div className="flex flex-col text-left">
                              <span className="font-semibold text-foreground text-xs">{p.name}</span>
                              {p.subdomain && (
                                <span className="text-[10px] text-muted-foreground font-mono">{p.subdomain}</span>
                              )}
                            </div>
                          </SelectItem>
                        ))}
                        {attachableProjects.length === 0 && (
                          <div className="text-center py-4 text-xs text-muted-foreground">
                            {t("databaseManager.noAttachableProjects")}
                          </div>
                        )}
                      </SelectContent>
                    </Select>
                    <Button
                      onClick={handleAttach}
                      disabled={!selectedProjectId || isActionLoading}
                      size="sm"
                      className="text-xs font-bold uppercase"
                    >
                      {t('databaseManager.attach')}
                    </Button>
                  </div>
                )}
              </div>
            </div>

            {/* Connection Information */}
            <div className="space-y-3">
              <h3 className="text-xs font-black uppercase tracking-widest text-muted-foreground flex items-center gap-2">
                <Lock className="w-3.5 h-3.5 shrink-0 text-primary" />
                {t('databaseManager.credsTitle')}
              </h3>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                {[
                  { label: t('databaseManager.networkHost'), value: selectedDb.host, key: 'Host' },
                  { label: t('databaseManager.port'), value: String(selectedDb.port), key: 'Port' },
                  { label: t('databaseManager.dbName'), value: selectedDb.name, key: 'Database' },
                  { label: t('databaseManager.userName'), value: selectedDb.username, key: 'Username' }
                ].map(item => (
                  <Card key={item.key} className="group hover:border-primary/20 transition-colors">
                    <CardContent className="p-4 flex items-center justify-between gap-3">
                      <div className="min-w-0 flex-1">
                        <p className="text-xs text-muted-foreground font-medium mb-1.5">{item.label}</p>
                        <p className="font-mono text-sm font-semibold mt-1 text-foreground truncate">{item.value}</p>
                      </div>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => handleCopy(item.value, item.label)}
                        className="shrink-0 text-muted-foreground hover:text-foreground opacity-0 group-hover:opacity-100 transition-opacity"
                      >
                        {copiedField === item.label ? (
                          <Check className="w-3.5 h-3.5 text-emerald-500" />
                        ) : (
                          <Copy className="w-3.5 h-3.5" />
                        )}
                      </Button>
                    </CardContent>
                  </Card>
                ))}
              </div>
            </div>

            {/* Stats Grid */}
            <div className="grid grid-cols-2 md:grid-cols-4 gap-3">
              <Card className="hover:border-primary/20 transition-colors">
                <CardContent className="p-4 space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">{t("databaseManager.engine")}</span>
                    <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center">
                      <Layers className="w-4 h-4 shrink-0 text-primary" />
                    </div>
                  </div>
                  <p className="text-lg font-bold tracking-tight uppercase text-foreground">
                    {selectedDb.engine}
                  </p>
                </CardContent>
              </Card>

              <Card className="hover:border-primary/20 transition-colors">
                <CardContent className="p-4 space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">{t("databaseManager.storage")}</span>
                    <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center">
                      <HardDrive className="w-4 h-4 shrink-0 text-primary" />
                    </div>
                  </div>
                  <p className="text-lg font-bold tracking-tight text-foreground">
                    1.0 GB
                  </p>
                  <p className="text-[10px] text-muted-foreground font-medium">
                    {t("databaseManager.allocatedLimit")}
                  </p>
                </CardContent>
              </Card>

              <Card className="hover:border-primary/20 transition-colors">
                <CardContent className="p-4 space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">{t("databaseManager.tablesLabel")}</span>
                    <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center">
                      <DbIcon className="w-4 h-4 shrink-0 text-primary" />
                    </div>
                  </div>
                  <p className="text-lg font-bold tracking-tight text-foreground">
                    {selectedDb.table_count || '—'}
                  </p>
                  <p className="text-[10px] text-muted-foreground font-medium">
                    {t("databaseManager.totalTables")}
                  </p>
                </CardContent>
              </Card>

              <Card className="hover:border-primary/20 transition-colors">
                <CardContent className="p-4 space-y-2">
                  <div className="flex items-center justify-between">
                    <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">{t("databaseManager.connections")}</span>
                    <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center">
                      <RefreshCw className="w-4 h-4 shrink-0 text-primary" />
                    </div>
                  </div>
                  <p className="text-lg font-bold tracking-tight text-foreground">
                    {selectedDb.connection_count || 0}
                  </p>
                  <p className="text-[10px] text-muted-foreground font-medium">
                    {t("databaseManager.activeSessions")}
                  </p>
                </CardContent>
              </Card>
            </div>

            {/* Danger Zone */}
            <div className="pt-4 border-t border-border/40">
              <Card className="border-destructive/20 bg-destructive/5 shadow-none">
                <CardContent className="p-5 space-y-4">
                  <div className="flex items-center gap-2.5">
                    <div className="w-8 h-8 rounded-lg bg-destructive/10 flex items-center justify-center">
                      <AlertTriangle className="w-4 h-4 shrink-0 text-destructive" />
                    </div>
                    <div>
                      <h3 className="font-bold text-sm text-destructive uppercase tracking-wide">{t("databaseManager.dangerZone")}</h3>
                      <p className="text-xs text-muted-foreground mt-0.5">{t("databaseManager.destructiveActions")}</p>
                    </div>
                  </div>

                  <div className="flex flex-wrap gap-2 pt-1">
                    <Button
                      variant="destructive"
                      size="sm"
                      onClick={() => {
                        setConfirmText('')
                        setShowResetModal(true)
                      }}
                      className="text-xs font-bold uppercase gap-1.5"
                    >
                      <Trash2 className="w-3.5 h-3.5 shrink-0" />
                      {t('databaseManager.reset')}
                    </Button>

                    <Button
                      variant="destructive"
                      size="sm"
                      onClick={() => {
                        setConfirmText('')
                        setShowReinstallModal(true)
                      }}
                      className="text-xs font-bold uppercase gap-1.5"
                    >
                      <RefreshCw className="w-3.5 h-3.5 shrink-0" />
                      {t('databaseManager.reinstall')}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            </div>
          </div>
        ) : (
          <div className="h-full flex flex-col items-center justify-center text-muted-foreground gap-4 opacity-40">
            <div className="w-16 h-16 rounded-2xl bg-muted border flex items-center justify-center">
              <ArrowRight className="w-6 h-6 rotate-90 lg:rotate-0" />
            </div>
            <p className="text-[10px] font-bold uppercase tracking-[0.3em]">{t('databaseManager.selectTarget')}</p>
          </div>
        )}
      </Card>

      {/* Confirmation Dialog: Reset DB */}
      <Dialog open={showResetModal} onOpenChange={setShowResetModal}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <div className="flex items-center gap-3 text-destructive mb-2">
              <AlertTriangle className="w-5 h-5" />
              <DialogTitle className="text-base font-semibold">{t('databaseManager.resetConfirm')}</DialogTitle>
            </div>
            <DialogDescription className="text-xs leading-relaxed">
              {t('databaseManager.resetDesc')}
            </DialogDescription>
          </DialogHeader>

          <div className="py-4 space-y-2">
            <p className="text-xs text-muted-foreground font-medium mb-1.5">
              {t('databaseManager.typeToConfirm', { name: selectedDb?.name || '' })}
            </p>
            <Input
              value={confirmText}
              onChange={(e) => setConfirmText(e.target.value)}
              placeholder={selectedDb?.name}
              className="h-9 text-xs font-mono"
            />
          </div>

          <DialogFooter>
            <Button variant="ghost" onClick={() => setShowResetModal(false)} className="text-xs font-medium">
              {t('common.cancel')}
            </Button>
            <Button
              variant="destructive"
              onClick={handleReset}
              disabled={confirmText !== selectedDb?.name || isActionLoading}
              className="text-xs font-medium"
            >
              {t('databaseManager.resetAction')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Confirmation Dialog: Reinstall DB */}
      <Dialog open={showReinstallModal} onOpenChange={setShowReinstallModal}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <div className="flex items-center gap-3 text-destructive mb-2">
              <AlertTriangle className="w-6 h-6 animate-pulse" />
              <DialogTitle className="text-base font-semibold">{t('databaseManager.reinstallConfirm')}</DialogTitle>
            </div>
            <DialogDescription className="text-xs leading-relaxed">
              {t('databaseManager.reinstallDesc')}
            </DialogDescription>
          </DialogHeader>

          <div className="py-4 space-y-2">
            <p className="text-xs text-muted-foreground font-medium mb-1.5">
              {t('databaseManager.typeToConfirm', { name: selectedDb?.name || '' })}
            </p>
            <Input
              value={confirmText}
              onChange={(e) => setConfirmText(e.target.value)}
              placeholder={selectedDb?.name}
              className="h-9 text-xs font-mono"
            />
          </div>

          <DialogFooter>
            <Button variant="ghost" onClick={() => setShowReinstallModal(false)} className="text-xs font-medium">
              {t('common.cancel')}
            </Button>
            <Button
              variant="destructive"
              onClick={handleReinstall}
              disabled={confirmText !== selectedDb?.name || isActionLoading}
              className="text-xs font-medium"
            >
              {t('databaseManager.reinstallAction')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Redeploy Confirmation Dialog */}
      <Dialog open={showRedeployModal} onOpenChange={setShowRedeployModal}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <div className="flex items-center gap-3 text-primary mb-2">
              <Settings className="w-6 h-6 animate-spin-slow" />
              <DialogTitle className="text-base font-semibold">{t("databaseManager.redeployTitle")}</DialogTitle>
            </div>
            <DialogDescription className="text-xs leading-relaxed">
              {t('databaseManager.redeployConfirm')}
            </DialogDescription>
          </DialogHeader>

          <DialogFooter className="gap-2">
            <Button variant="ghost" onClick={() => setShowRedeployModal(false)} className="text-xs font-medium">
              {t('databaseManager.later')}
            </Button>
            <Button
              onClick={handleRedeploy}
              disabled={isActionLoading}
              className="text-xs font-medium"
            >
              {t('databaseManager.redeployNow')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

    </div>
  )
}
