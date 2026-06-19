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

// Brand-aware engine styling for list avatars/badges (Postgres = blue, MySQL = amber)
const ENGINE_META = {
  postgresql: { glyph: 'PG', avatar: 'bg-blue-500/10 text-blue-500 border-blue-500/20', badge: 'border-transparent bg-blue-500/10 text-blue-500' },
  mysql: { glyph: 'My', avatar: 'bg-amber-500/10 text-amber-500 border-amber-500/20', badge: 'border-transparent bg-amber-500/10 text-amber-500' },
} as const

const engineMeta = (engine: string) => ENGINE_META[engine as keyof typeof ENGINE_META] ?? ENGINE_META.postgresql

// One database entry in the sidebar list — accent bar (selected) + engine avatar.
function DbRow({ db, selected, onSelect, t }: {
  db: DatabaseInstance
  selected: boolean
  onSelect: (id: number) => void
  t: (key: string) => string
}) {
  const meta = engineMeta(db.engine)
  const attached = db.project_id !== null
  return (
    <button
      onClick={() => onSelect(db.id)}
      className={cn(
        "relative w-full text-left pl-3.5 pr-3 py-2.5 rounded-lg transition-all border group focus:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1",
        selected
          ? 'bg-muted/40 border-primary/20 shadow-sm'
          : 'border-transparent hover:bg-muted/40 hover:border-border/40'
      )}
    >
      {/* Left accent bar marks the selected row (Linear/Vercel pattern) */}
      <span className={cn(
        "absolute left-0 top-1/2 -translate-y-1/2 h-7 w-0.5 rounded-full bg-primary transition-opacity",
        selected ? 'opacity-100' : 'opacity-0'
      )} />
      <div className="flex items-center gap-2.5">
        <span className={cn(
          "shrink-0 w-8 h-8 rounded-lg border flex items-center justify-center text-[10px] font-bold tracking-tight",
          meta.avatar
        )}>
          {meta.glyph}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <span className={cn(
              "font-bold text-xs tracking-tight truncate",
              selected ? 'text-primary' : 'text-foreground'
            )}>
              {db.name}
            </span>
            <span className={cn(
              "shrink-0 w-1.5 h-1.5 rounded-full",
              db.status === 'active' ? 'bg-emerald-500' : 'bg-rose-500'
            )} />
          </div>
          <div className="mt-1 flex items-center gap-1.5 text-[10px] font-medium text-muted-foreground">
            {attached && db.project ? (
              <Badge variant="outline" className="h-4 px-1.5 py-0 text-[10px] font-semibold border-primary/20 bg-primary/5 text-primary max-w-[150px] truncate">
                {db.project.name}
              </Badge>
            ) : (
              <span className="text-muted-foreground/60">{t('databaseManager.unattached')}</span>
            )}
          </div>
        </div>
      </div>
    </button>
  )
}

// Section header + rows for a sidebar group (Unattached / Attached).
function DbSection({ title, count, dbs, selectedDbId, onSelect, t }: {
  title: string
  count: number
  dbs: DatabaseInstance[]
  selectedDbId: number | null
  onSelect: (id: number) => void
  t: (key: string) => string
}) {
  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2 px-2 pb-1">
        <span className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground/70">{title}</span>
        <span className="text-[10px] font-semibold text-muted-foreground/50">{count}</span>
        <span className="flex-1 h-px bg-border/60" />
      </div>
      <div className="space-y-1">
        {dbs.map(db => (
          <DbRow key={db.id} db={db} selected={selectedDbId === db.id} onSelect={onSelect} t={t} />
        ))}
      </div>
    </div>
  )
}

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
  const [showAttachModal, setShowAttachModal] = useState(false)
  const [showDetachModal, setShowDetachModal] = useState(false)
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [showRevealModal, setShowRevealModal] = useState(false)
  const [showDeleteModal, setShowDeleteModal] = useState(false)
  const [confirmText, setConfirmText] = useState('')
  const [selectedProjectId, setSelectedProjectId] = useState<string>('')
  const [redeployProjectUid, setRedeployProjectUid] = useState<string>('')

  // Form states for standalone DB creation
  const [createEngine, setCreateEngine] = useState<'mysql' | 'postgres'>('mysql')
  const [createName, setCreateName] = useState('')
  const [createUsername, setCreateUsername] = useState('')
  const [createPassword, setCreatePassword] = useState('')
  const [createdInstance, setCreatedInstance] = useState<{
    name: string
    username: string
    password: string
    host: string
    port: number
    engine: string
  } | null>(null)

  const fetchData = useCallback(async () => {
    try {
      const [dbRes, projRes] = await Promise.all([
        databaseAPI.listOwn(),
        projectsAPI.listOwn()
      ])
      const dbs = (dbRes.data.databases || []).filter((db: DatabaseInstance) => db.status !== 'deleted')
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
    db.status !== 'deleted' && (
      db.name.toLowerCase().includes(search.toLowerCase()) ||
      db.engine.toLowerCase().includes(search.toLowerCase()) ||
      (db.project_id && db.project && db.project.name.toLowerCase().includes(search.toLowerCase()))
    )
  )

  // Split into unattached / attached sections for the sidebar
  const unattachedDbs = filteredDbs.filter(db => db.project_id === null)
  const attachedDbs = filteredDbs.filter(db => db.project_id !== null)

  // Projects that do not have a database attached
  const attachableProjects = projects.filter(p => !p.database_instance)

  // Attach database handler
  const handleAttach = async () => {
    if (!selectedDb || !selectedProjectId) return
    setIsActionLoading(true)
    try {
      await databaseAPI.attach(selectedDb.uid, selectedProjectId)
      toast.success(t('common.success'))

      // Prompt for redeploy
      setRedeployProjectUid(selectedProjectId)
      setShowRedeployModal(true)
      setSelectedProjectId('')

      // Refresh list
      await fetchData()
    } catch (error: unknown) {
      const err = error as { response?: { data?: { error?: string } } };
      toast.error(err.response?.data?.error || t('common.error'))
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
      await databaseAPI.detach(selectedDb.uid)
      toast.success(t('common.success'))

      if (projectUid) {
        setRedeployProjectUid(projectUid)
        setShowRedeployModal(true)
      }

      await fetchData()
    } catch (error: unknown) {
      const err = error as { response?: { data?: { error?: string } } };
      toast.error(err.response?.data?.error || t('common.error'))
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
      await databaseAPI.resetInstance(selectedDb.uid)
      toast.success(t('databaseManager.querySuccess'))
      setShowResetModal(false)
      setConfirmText('')
      await fetchData()
    } catch (error: unknown) {
      const err = error as { response?: { data?: { error?: string } } };
      toast.error(err.response?.data?.error || t('common.error'))
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
      const res = await databaseAPI.reinstallInstance(selectedDb.uid)
      toast.success(t('databaseManager.backupSuccess'))
      setShowReinstallModal(false)
      setConfirmText('')

      if (res.data.redeployRequired && selectedDb.project_id && selectedDb.project?.uid) {
        setRedeployProjectUid(selectedDb.project.uid)
        setShowRedeployModal(true)
      }

      await fetchData()
    } catch (error: unknown) {
      const err = error as { response?: { data?: { error?: string } } };
      toast.error(err.response?.data?.error || t('common.error'))
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
    } catch (error: unknown) {
      const err = error as { response?: { data?: { error?: string } } };
      toast.error(err.response?.data?.error || t('common.error'))
    } finally {
      setIsActionLoading(false)
    }
  }

  const generatePassword = () => {
    const allowedChars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!$%^&*_+='
    const len = allowedChars.length
    const maxValid = 256 - (256 % len)
    let password = ''
    let count = 0
    while (password.length < 20 && count < 1000) {
      const tempArray = new Uint8Array(20 - password.length)
      window.crypto.getRandomValues(tempArray)
      for (let i = 0; i < tempArray.length; i++) {
        if (tempArray[i] < maxValid) {
          password += allowedChars[tempArray[i] % len]
        }
      }
      count++
    }
    if (password.length < 20) {
      const fallbackArray = new Uint8Array(20)
      window.crypto.getRandomValues(fallbackArray)
      for (let i = 0; i < 20; i++) {
        password += allowedChars[fallbackArray[i] % len]
      }
    }
    setCreatePassword(password)
  }

  const handleCreateDatabase = async (e: React.FormEvent) => {
    e.preventDefault()

    const dbNameRegex = /^[a-z][a-z0-9_]{1,63}$/
    if (!dbNameRegex.test(createName)) {
      toast.error(t('databaseManager.dbNameInvalid') || 'Database name must start with a letter and contain lowercase alphanumeric/underscore')
      return
    }
    if (createName.length < 2 || createName.length > 64) {
      toast.error(t('databaseManager.dbNameLength') || 'Database name must be between 2 and 64 characters')
      return
    }
    const reservedDbs = ['mysql', 'postgres', 'information_schema', 'performance_schema', 'sys', 'template0', 'template1']
    if (reservedDbs.includes(createName)) {
      toast.error(t('databaseManager.dbNameReserved') || 'Database name is reserved')
      return
    }

    const usernameRegex = /^[a-z][a-z0-9_]*$/
    if (!usernameRegex.test(createUsername)) {
      toast.error(t('databaseManager.usernameInvalid') || 'Username must start with a letter and contain lowercase alphanumeric/underscore')
      return
    }
    if (createUsername.length < 2 || createUsername.length > 32) {
      toast.error(t('databaseManager.usernameLength') || 'Username must be between 2 and 32 characters')
      return
    }
    const reservedUsers = ['root', 'admin', 'postgres', 'mysql', 'superuser', 'replication']
    if (reservedUsers.includes(createUsername)) {
      toast.error(t('databaseManager.usernameReserved') || 'Username is reserved')
      return
    }

    if (createPassword.length < 12 || createPassword.length > 128) {
      toast.error(t('databaseManager.passwordLength') || 'Password must be between 12 and 128 characters')
      return
    }
    if (!/[A-Z]/.test(createPassword) || !/[a-z]/.test(createPassword) || !/[0-9]/.test(createPassword)) {
      toast.error(t('databaseManager.passwordComplexity') || 'Password must contain uppercase, lowercase, and a number')
      return
    }
    if (/[\s@#/?]/.test(createPassword)) {
      toast.error(t('databaseManager.passwordForbiddenChars') || 'Password cannot contain space, @, #, /, ?')
      return
    }

    setIsActionLoading(true)
    try {
      const res = await databaseAPI.create({
        engine: createEngine,
        name: createName,
        username: createUsername,
        password: createPassword
      })

      const dbInst = res.data.database
      setCreatedInstance({
        name: createName,
        username: createUsername,
        password: createPassword,
        host: dbInst.host || 'localhost',
        port: dbInst.port || (createEngine === 'mysql' ? 3306 : 5432),
        engine: createEngine
      })

      setCreateName('')
      setCreateUsername('')
      setCreatePassword('')
      setCreateEngine('mysql')
      setShowCreateModal(false)
      setShowRevealModal(true)

      await fetchData()
      if (dbInst && dbInst.id) {
        setSelectedDbId(dbInst.id)
      }
    } catch (error: unknown) {
      const err = error as { response?: { data?: { error?: string } } }
      toast.error(err.response?.data?.error || t('common.error'))
    } finally {
      setIsActionLoading(false)
    }
  }

  const handleDeleteDatabase = async () => {
    if (!selectedDb) return
    if (selectedDb.project_id !== null) {
      toast.error(t('databaseManager.cannotDeleteAttached'))
      return
    }
    if (confirmText !== selectedDb.name) {
      toast.error(t('common.error'))
      return
    }

    setIsActionLoading(true)
    try {
      await databaseAPI.delete(selectedDb.uid)
      toast.success(t('databaseManager.deleteSuccess'))
      setShowDeleteModal(false)
      setConfirmText('')

      // Clear selectedDbId and trigger fetch
      setSelectedDbId(null)
      await fetchData()
    } catch (error: unknown) {
      const err = error as { response?: { data?: { error?: string } } }
      toast.error(err.response?.data?.error || t('common.error'))
    } finally {
      setIsActionLoading(false)
    }
  }

  if (isLoading) {
    return (
      <div className="min-h-[60vh] flex flex-col items-center justify-center gap-4">
        <Loader2 className="w-10 h-10 text-primary animate-spin" />
        <p className="text-muted-foreground font-bold uppercase tracking-widest text-[10px] ">{t('databaseManager.loading')}</p>
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
            <div className="flex items-center justify-between gap-3 mb-2">
              <div className="flex items-center gap-3">
                <div className="w-9 h-9 rounded-lg bg-primary/10 border border-primary/20 flex items-center justify-center text-primary shrink-0">
                  <DbIcon className="w-4 h-4" />
                </div>
                <div>
                  <h2 className="text-sm font-bold tracking-tight uppercase">{t('common.databases')}</h2>
                  <p className="text-[10px] font-semibold text-muted-foreground uppercase tracking-wider">{t('databaseManager.activeInstances')}</p>
                </div>
              </div>
            </div>
            <Button
              onClick={() => {
                setCreateEngine('mysql')
                setCreateName('')
                setCreateUsername('')
                setCreatePassword('')
                setShowCreateModal(true)
              }}
              size="sm"
              className="w-full text-xs font-bold uppercase gap-1.5 mb-2 shadow-sm"
            >
              <DbIcon className="w-3.5 h-3.5 shrink-0" />
              {t('databaseManager.newDatabase')}
            </Button>
            <div className="relative mt-2">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-muted-foreground" />
              <Input
                placeholder={t('databaseManager.searchSchema')}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-9 h-9 text-xs"
              />
            </div>
          </CardHeader>

          <CardContent className="flex-1 overflow-y-auto p-1.5 pt-2 scrollbar-thin">
            {filteredDbs.length > 0 ? (
              <div className="space-y-4">
                {unattachedDbs.length > 0 && (
                  <DbSection
                    title={t('databaseManager.sectionUnattached')}
                    count={unattachedDbs.length}
                    dbs={unattachedDbs}
                    selectedDbId={selectedDbId}
                    onSelect={setSelectedDbId}
                    t={t}
                  />
                )}
                {attachedDbs.length > 0 && (
                  <DbSection
                    title={t('databaseManager.sectionAttached')}
                    count={attachedDbs.length}
                    dbs={attachedDbs}
                    selectedDbId={selectedDbId}
                    onSelect={setSelectedDbId}
                    t={t}
                  />
                )}
              </div>
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
                  <h1 className="text-xl font-semibold tracking-tight text-foreground">{selectedDb.name}</h1>
                  <Badge variant={selectedDb.status === 'active' ? 'default' : 'destructive'} className="text-[10px] font-medium uppercase">
                    {selectedDb.status}
                  </Badge>
                  <Badge variant="outline" className={cn(
                    "text-[10px] font-medium uppercase",
                    selectedDb.engine === 'mysql'
                      ? 'border-transparent bg-amber-500/10 text-amber-500'
                      : 'border-transparent bg-blue-500/10 text-blue-500'
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
                      onClick={() => setShowDetachModal(true)}
                      disabled={isActionLoading}
                      className="text-xs font-medium uppercase text-rose-500 hover:text-rose-600 border-rose-500/20 hover:bg-rose-500/10 bg-transparent transition-colors"
                    >
                      {t('databaseManager.detach')}
                    </Button>
                    <Link
                      to={`/projects/${selectedDb.project.uid}?tab=database`}
                      className={cn(buttonVariants({ variant: "default", size: "sm" }), "text-xs font-medium uppercase flex items-center gap-1.5 shadow-sm")}
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
                      onClick={() => setShowAttachModal(true)}
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
                      disabled={selectedDb.status === 'deleted'}
                      className="text-xs font-bold uppercase gap-1.5 disabled:opacity-50"
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
                      disabled={selectedDb.status === 'deleted'}
                      className="text-xs font-bold uppercase gap-1.5 disabled:opacity-50"
                    >
                      <RefreshCw className="w-3.5 h-3.5 shrink-0" />
                      {t('databaseManager.reinstall')}
                    </Button>

                    <Button
                      variant="destructive"
                      size="sm"
                      disabled={selectedDb.project_id !== null || selectedDb.status === 'deleted'}
                      onClick={() => {
                        setConfirmText('')
                        setShowDeleteModal(true)
                      }}
                      className="text-xs font-bold uppercase gap-1.5 disabled:opacity-50"
                      title={selectedDb.project_id !== null ? t('databaseManager.cannotDeleteAttached') : undefined}
                    >
                      <Trash2 className="w-3.5 h-3.5 shrink-0" />
                      {t('databaseManager.deleteDatabaseBtn')}
                    </Button>
                  </div>
                  {selectedDb.project_id !== null && (
                    <p className="text-[10px] text-muted-foreground italic mt-1">
                      * {t('databaseManager.cannotDeleteAttached')}
                    </p>
                  )}
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

      {/* Detach Database Confirmation Dialog */}
      <Dialog open={showDetachModal} onOpenChange={setShowDetachModal}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <div className="flex items-center gap-3 text-destructive mb-2">
              <AlertTriangle className="w-5 h-5" />
              <DialogTitle className="text-base font-semibold">{t("databaseManager.detachConfirm")}</DialogTitle>
            </div>
            <DialogDescription className="text-xs leading-relaxed">
              {t("databaseManager.detachConfirmDesc")}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2">
            <Button variant="ghost" onClick={() => setShowDetachModal(false)} className="text-xs font-medium">
              {t("common.cancel")}
            </Button>
            <Button variant="destructive" onClick={() => { setShowDetachModal(false); handleDetach(); }} disabled={isActionLoading} className="text-xs font-medium">
              {t("databaseManager.detach")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Attach Database Confirmation Dialog */}
      <Dialog open={showAttachModal} onOpenChange={setShowAttachModal}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <div className="flex items-center gap-3 text-primary mb-2">
              <DbIcon className="w-5 h-5" />
              <DialogTitle className="text-base font-semibold">{t("databaseManager.attachConfirm")}</DialogTitle>
            </div>
            <DialogDescription className="text-xs leading-relaxed">
              {t("databaseManager.attachConfirmDesc")}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2">
            <Button variant="ghost" onClick={() => setShowAttachModal(false)} className="text-xs font-medium">
              {t("common.cancel")}
            </Button>
            <Button onClick={() => { setShowAttachModal(false); handleAttach(); }} disabled={isActionLoading} className="text-xs font-medium">
              {t("databaseManager.attach")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Confirmation Dialog: Reinstall DB */}
      <Dialog open={showReinstallModal} onOpenChange={setShowReinstallModal}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <div className="flex items-center gap-3 text-destructive mb-2">
              <AlertTriangle className="w-6 h-6" />
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

      {/* Create Database Standalone Dialog */}
      <Dialog open={showCreateModal} onOpenChange={setShowCreateModal}>
        <DialogContent className="max-w-lg">
          <form onSubmit={handleCreateDatabase}>
            <DialogHeader>
              <div className="flex items-center gap-3 text-primary mb-2">
                <DbIcon className="w-5 h-5" />
                <DialogTitle className="text-base font-semibold">{t("databaseManager.createDatabaseTitle")}</DialogTitle>
              </div>
              <DialogDescription className="text-xs leading-relaxed">
                {t("databaseManager.createDatabaseDesc")}
              </DialogDescription>
            </DialogHeader>

            <div className="py-4 space-y-4">
              {/* Engine Selection */}
              <div className="space-y-2">
                <label className="text-xs font-semibold text-muted-foreground uppercase tracking-wider block">
                  {t("databaseManager.engineLabel")}
                </label>
                <Select
                  value={createEngine}
                  onValueChange={(val) => setCreateEngine(val as 'mysql' | 'postgres')}
                >
                  <SelectTrigger className="w-full text-xs">
                    {createEngine === 'mysql' ? 'MySQL (v8.0)' : 'PostgreSQL (v15)'}
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="mysql" className="text-xs font-medium">MySQL (v8.0)</SelectItem>
                    <SelectItem value="postgres" className="text-xs font-medium">PostgreSQL (v15)</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {/* Database Name */}
              <div className="space-y-1.5">
                <label className="text-xs font-semibold text-muted-foreground uppercase tracking-wider block">
                  {t("databaseManager.databaseNameLabel")} <span className="text-destructive">*</span>
                </label>
                <Input
                  value={createName}
                  onChange={(e) => setCreateName(e.target.value.toLowerCase())}
                  placeholder={t("databaseManager.databaseNamePlaceholder")}
                  className="h-9 text-xs"
                  required
                />
                <p className="text-[10px] text-muted-foreground">
                  Lowercase, alphanumeric & underscore only. Max 64 chars.
                </p>
              </div>

              {/* Username */}
              <div className="space-y-1.5">
                <label className="text-xs font-semibold text-muted-foreground uppercase tracking-wider block">
                  {t("databaseManager.usernameLabel")} <span className="text-destructive">*</span>
                </label>
                <Input
                  value={createUsername}
                  onChange={(e) => setCreateUsername(e.target.value.toLowerCase())}
                  placeholder={t("databaseManager.usernamePlaceholder")}
                  className="h-9 text-xs"
                  required
                />
                <p className="text-[10px] text-muted-foreground">
                  Lowercase, alphanumeric & underscore. Max 32 chars. Must start with a letter.
                </p>
              </div>

              {/* Password */}
              <div className="space-y-1.5">
                <label className="text-xs font-semibold text-muted-foreground uppercase tracking-wider block">
                  {t("databaseManager.passwordLabel")} <span className="text-destructive">*</span>
                </label>
                <div className="flex gap-2">
                  <Input
                    value={createPassword}
                    onChange={(e) => setCreatePassword(e.target.value)}
                    placeholder={t("databaseManager.passwordPlaceholder")}
                    className="h-9 text-xs font-mono"
                    required
                  />
                  <Button
                    type="button"
                    variant="outline"
                    onClick={generatePassword}
                    className="h-9 text-xs font-semibold px-3 shrink-0"
                  >
                    Generate
                  </Button>
                </div>
                <p className="text-[10px] text-muted-foreground">
                  Min 12 chars. Must contain uppercase, lowercase, and a number. Cannot contain spaces or @, #, /, ?.
                </p>
              </div>
            </div>

            <DialogFooter className="gap-2">
              <Button type="button" variant="ghost" onClick={() => setShowCreateModal(false)} className="text-xs font-medium">
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={isActionLoading} className="text-xs font-medium">
                {t("databaseManager.createDatabaseBtn")}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* One-Time Password Reveal Dialog */}
      <Dialog open={showRevealModal} onOpenChange={(open) => {
        if (!open) {
          setCreatedInstance(null)
        }
        setShowRevealModal(open)
      }}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <div className="flex items-center gap-3 text-amber-500 mb-2">
              <AlertTriangle className="w-5 h-5 animate-pulse" />
              <DialogTitle className="text-base font-semibold">Credentials Created Successfully</DialogTitle>
            </div>
            <DialogDescription className="text-xs leading-relaxed text-amber-500/95 font-medium">
              Save this password now! Once this dialog is closed, you will not be able to retrieve it again.
            </DialogDescription>
          </DialogHeader>

          {createdInstance && (
            <div className="py-4 space-y-3">
              <div className="rounded-lg bg-muted/40 border border-border/80 p-3 space-y-2.5 font-mono text-xs">
                <div className="flex justify-between items-center py-1 border-b border-border/40">
                  <span className="text-muted-foreground text-[10px] uppercase font-semibold">Engine</span>
                  <span className="text-foreground font-semibold">{createdInstance.engine.toUpperCase()}</span>
                </div>
                <div className="flex justify-between items-center py-1 border-b border-border/40">
                  <span className="text-muted-foreground text-[10px] uppercase font-semibold">Host</span>
                  <span className="text-foreground font-semibold">{createdInstance.host}</span>
                </div>
                <div className="flex justify-between items-center py-1 border-b border-border/40">
                  <span className="text-muted-foreground text-[10px] uppercase font-semibold">Port</span>
                  <span className="text-foreground font-semibold">{createdInstance.port}</span>
                </div>
                <div className="flex justify-between items-center py-1 border-b border-border/40">
                  <span className="text-muted-foreground text-[10px] uppercase font-semibold">Database</span>
                  <span className="text-foreground font-semibold">{createdInstance.name}</span>
                </div>
                <div className="flex justify-between items-center py-1 border-b border-border/40">
                  <span className="text-muted-foreground text-[10px] uppercase font-semibold">Username</span>
                  <span className="text-foreground font-semibold">{createdInstance.username}</span>
                </div>
                <div className="flex justify-between items-center py-1">
                  <span className="text-muted-foreground text-[10px] uppercase font-semibold">Password</span>
                  <span className="text-foreground font-semibold select-all bg-amber-500/10 px-1 rounded">{createdInstance.password}</span>
                </div>
              </div>

              <Button
                variant="outline"
                size="sm"
                className="w-full text-xs font-medium"
                onClick={() => {
                  const data = `Engine: ${createdInstance.engine.toUpperCase()}\nHost: ${createdInstance.host}\nPort: ${createdInstance.port}\nDatabase: ${createdInstance.name}\nUsername: ${createdInstance.username}\nPassword: ${createdInstance.password}`
                  navigator.clipboard.writeText(data)
                  toast.success(t('databaseManager.copiedAll') || 'All credentials copied to clipboard!')
                }}
              >
                <Copy className="w-3.5 h-3.5 mr-2 shrink-0" />
                {t('databaseManager.copyAll')}
              </Button>
            </div>
          )}

          <DialogFooter>
            <Button
              className="w-full text-xs font-medium"
              onClick={() => {
                setCreatedInstance(null)
                setShowRevealModal(false)
              }}
            >
              I've saved my credentials
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Confirmation Dialog: Delete DB */}
      <Dialog open={showDeleteModal} onOpenChange={setShowDeleteModal}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <div className="flex items-center gap-3 text-destructive mb-2">
              <AlertTriangle className="w-5 h-5" />
              <DialogTitle className="text-base font-semibold">{t('databaseManager.deleteConfirmTitle')}</DialogTitle>
            </div>
            <DialogDescription className="text-xs leading-relaxed">
              {t('databaseManager.deleteConfirmDesc', { name: selectedDb?.name || '' })}
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
            <Button variant="ghost" onClick={() => setShowDeleteModal(false)} className="text-xs font-medium">
              {t('common.cancel')}
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeleteDatabase}
              disabled={confirmText !== selectedDb?.name || isActionLoading}
              className="text-xs font-medium"
            >
              {t('databaseManager.deleteDatabaseBtn')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

    </div>
  )
}
