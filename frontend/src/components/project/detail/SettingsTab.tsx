import { Code2, Blocks, AlertTriangle, Play, RefreshCw, GitBranch, LayoutGrid, Globe, Loader2, Save, ExternalLink, Database as DbIcon, Settings } from 'lucide-react'
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
import useTranslation from '@/lib/useTranslation'
import { cn } from '@/lib/utils'
import { RUNTIME_VERSIONS, DEFAULT_RUNTIME_VERSIONS } from '@/lib/runtimes'
import { Project, DatabaseInstance } from '@/types'
import { useState, useEffect } from 'react'
import { databaseAPI, projectsAPI } from '@/services/api'
import { toast } from 'sonner'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'

export interface GitHubInstallationOption {
  installation_id: number
  account_name: string
  avatar_url?: string
}

export interface GitHubRepoOption {
  id: number
  name: string
  full_name: string
  html_url: string
}

const Github = (props: React.SVGProps<SVGSVGElement>) => (
  <svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    className={props.className}
    style={props.style}
    width={props.width || "1em"}
    height={props.height || "1em"}
    {...props}
  >
    <path d="M15 22v-4a4.8 4.8 0 0 0-1-3.5c3 0 6-2 6-5.5.08-1.25-.27-2.48-1-3.5.28-1.15.28-2.35 0-3.5 0 0-1 0-3 1.5-2.64-.5-5.36-.5-8 0C6 2 5 2 5 2c-.3 1.15-.3 2.35 0 3.5A5.403 5.403 0 0 0 4 9c0 3.5 3 5.5 6 5.5-.39.49-.68 1.05-.85 1.65-.17.6-.22 1.23-.15 1.85v4" />
    <path d="M9 18c-4.51 2-5-2-7-2" />
  </svg>
)

interface SettingsTabProps {
  project: Project
  frameworkLabel: string
  isNodeRelated: boolean
  phpVersionInput: string
  setPhpVersionInput: (val: string) => void
  queueEnabledInput: boolean
  setQueueEnabledInput: (val: boolean) => void
  nodeVersionInput: string
  setNodeVersionInput: (val: string) => void
  languageVersionInput: string
  setLanguageVersionInput: (val: string) => void
  workerCommandInput: string
  setWorkerCommandInput: (val: string) => void
  buildCommandInput: string
  setBuildCommandInput: (val: string) => void
  startCommandInput: string
  setStartCommandInput: (val: string) => void
  portInput: number | ''
  setPortInput: (val: number | '') => void
  branchesList: string[]
  branchInput: string
  setBranchInput: (val: string) => void
  forceManualInput: boolean
  setForceManualInput: (val: boolean) => void
  isFetchingBranches: boolean
  fetchBranches: (force: boolean) => void
  baseDirInput: string
  setBaseDirInput: (val: string) => void
  gitConnectionMode: 'github_app' | 'manual'
  setGitConnectionMode: (val: 'github_app' | 'manual') => void
  isGithubInstallationsLoading: boolean
  memoizedGithubInstallations: GitHubInstallationOption[]
  githubInstallationIdInput: number | null
  setGithubInstallationIdInput: (val: number | null) => void
  setGithubRepoOwnerInput: (val: string) => void
  setGithubRepoNameInput: (val: string) => void
  setGithubUrlInput: (val: string) => void
  isGithubReposLoading: boolean
  memoizedGithubRepos: GitHubRepoOption[]
  githubRepoNameInput: string
  githubRepoOwnerInput: string
  githubUrlInput: string
  isSettingsDirty: boolean
  isSavingSettings: boolean
  handleResetSettings: () => void
  handleSaveSettings: () => void
}

export function SettingsTab({
  project,
  frameworkLabel,
  isNodeRelated,
  phpVersionInput,
  setPhpVersionInput,
  queueEnabledInput,
  setQueueEnabledInput,
  nodeVersionInput,
  setNodeVersionInput,
  languageVersionInput,
  setLanguageVersionInput,
  workerCommandInput,
  setWorkerCommandInput,
  buildCommandInput,
  setBuildCommandInput,
  startCommandInput,
  setStartCommandInput,
  portInput,
  setPortInput,
  branchesList,
  branchInput,
  setBranchInput,
  forceManualInput,
  setForceManualInput,
  isFetchingBranches,
  fetchBranches,
  baseDirInput,
  setBaseDirInput,
  gitConnectionMode,
  setGitConnectionMode,
  isGithubInstallationsLoading,
  memoizedGithubInstallations,
  githubInstallationIdInput,
  setGithubInstallationIdInput,
  setGithubRepoOwnerInput,
  setGithubRepoNameInput,
  setGithubUrlInput,
  isGithubReposLoading,
  memoizedGithubRepos,
  githubRepoNameInput,
  githubRepoOwnerInput,
  githubUrlInput,
  isSettingsDirty,
  isSavingSettings,
  handleResetSettings,
  handleSaveSettings,
}: SettingsTabProps) {
  const { t } = useTranslation()
  const [databasesList, setDatabasesList] = useState<DatabaseInstance[]>([])
  const [selectedDbId, setSelectedDbId] = useState<string>('')
  const [isDbActionLoading, setIsDbActionLoading] = useState(false)
  const [showDbRedeployModal, setShowDbRedeployModal] = useState(false)

  useEffect(() => {
    const fetchDbs = async () => {
      try {
        const res = await databaseAPI.listOwn()
        const dbs = res.data.databases || []
        // Filter to only show databases that are not attached to any project
        setDatabasesList(dbs.filter((db: DatabaseInstance) => !db.project_id))
      } catch (err) {
        console.error("Failed to load databases list", err)
      }
    }
    fetchDbs()
  }, [])

  if (!project) return null

  return (
    <>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
        <Card>
          <CardHeader>
            <div className="flex items-center gap-4">
              <div className="w-10 h-10 bg-primary/10 rounded-lg flex items-center justify-center text-primary">
                {project.framework === 'Laravel' ? <Code2 className="w-5 h-5" /> : <Blocks className="w-5 h-5" />}
              </div>
              <div>
                <CardTitle className="text-lg">
                  {project.framework === 'Laravel' ? t('projectDetail.settings.phpTitle') : t('projectDetail.settings.frameworkStack', { framework: frameworkLabel })}
                </CardTitle>
                <CardDescription>
                  {project.framework === 'Laravel' ? t('projectDetail.settings.phpVersion') : 'Configure your application environment'}
                </CardDescription>
              </div>
            </div>
          </CardHeader>
          <CardContent className="space-y-6">
            {project.framework === 'Laravel' ? (
              <>
                <div className="space-y-2">
                  <Label className="text-xs uppercase tracking-widest text-muted-foreground">{t('projectDetail.settings.version')}</Label>
                  <Select
                    value={phpVersionInput}
                    onValueChange={(val) => setPhpVersionInput(val || DEFAULT_RUNTIME_VERSIONS.php)}
                  >
                    <SelectTrigger className="h-12 border-muted-foreground/20">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent align="start" alignItemWithTrigger={false} className="w-[180px] bg-popover border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72">
                      {RUNTIME_VERSIONS.php.map(v => (
                        <SelectItem key={v.value} value={v.value}>{v.label} {v.isLatest ? t('projectDetail.settings.latest') : ''}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                <div className="flex items-center justify-between p-4 rounded-xl border bg-muted/20">
                  <div className="space-y-1">
                    <Label className="text-sm font-bold">{t('projectDetail.metrics.queue')}</Label>
                    <p className="text-[10px] text-muted-foreground leading-relaxed">
                      {t('projectDetail.settings.queueHandles')}
                    </p>
                  </div>
                  <Switch
                    checked={queueEnabledInput}
                    onCheckedChange={setQueueEnabledInput}
                  />
                </div>
              </>
            ) : (
              <div className="space-y-6">
                {isNodeRelated && (
                  <div className="space-y-2">
                    <Label className="text-xs uppercase tracking-widest text-muted-foreground">{t('projectDetail.settings.nodeVersion')}</Label>
                    <Select
                      value={nodeVersionInput}
                      onValueChange={(val) => setNodeVersionInput(val || DEFAULT_RUNTIME_VERSIONS.node)}
                    >
                      <SelectTrigger className="h-12 border-muted-foreground/20">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent align="start" alignItemWithTrigger={false} className="w-[180px] bg-popover border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72">
                        {RUNTIME_VERSIONS.node.map(item => (
                          <SelectItem key={item.value} value={item.value}>{item.label}</SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                )}

                {project.framework === 'Go' && (
                  <div className="space-y-2">
                    <Label className="text-xs uppercase tracking-widest text-muted-foreground">Go Version</Label>
                    <Select
                      value={languageVersionInput}
                      onValueChange={(val) => setLanguageVersionInput(val || DEFAULT_RUNTIME_VERSIONS.go)}
                    >
                      <SelectTrigger className="h-12 border-muted-foreground/20">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent align="start" alignItemWithTrigger={false} className="w-[180px] bg-popover border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72">
                        {RUNTIME_VERSIONS.go.map(v => (
                          <SelectItem key={v.value} value={v.value}>{v.label}</SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                )}

                {project.framework === 'Python' && (
                  <div className="space-y-2">
                    <Label className="text-xs uppercase tracking-widest text-muted-foreground">Python Version</Label>
                    <Select
                      value={languageVersionInput}
                      onValueChange={(val) => setLanguageVersionInput(val || DEFAULT_RUNTIME_VERSIONS.python)}
                    >
                      <SelectTrigger className="h-12 border-muted-foreground/20">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent align="start" alignItemWithTrigger={false} className="w-[180px] bg-popover border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72">
                        {RUNTIME_VERSIONS.python.map(v => (
                          <SelectItem key={v.value} value={v.value}>{v.label}</SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                )}

                <div className="p-4 rounded-xl border bg-muted/20 space-y-4">
                  <div className="flex items-center justify-between">
                    <div className="space-y-1">
                      <Label className="text-sm font-bold">{t('projectDetail.metrics.backgroundService')}</Label>
                      <p className="text-[10px] text-muted-foreground leading-relaxed">
                        Run a secondary process for background tasks
                      </p>
                    </div>
                    <Switch
                      checked={workerCommandInput !== ''}
                      onCheckedChange={(checked: boolean) => !checked && setWorkerCommandInput('')}
                    />
                  </div>

                  {workerCommandInput !== undefined && workerCommandInput !== '' && (
                    <div className="space-y-2 pt-2 border-t border-border">
                      <Label className="text-[10px] uppercase tracking-wider text-muted-foreground">{t('projectDetail.metrics.customCommand')}</Label>
                      <div className="flex gap-2">
                        <Input
                          value={workerCommandInput}
                          onChange={(e) => setWorkerCommandInput(e.target.value)}
                          placeholder="e.g. npm run worker"
                          className="h-9 text-xs font-mono"
                        />
                      </div>
                    </div>
                  )}
                </div>
              </div>
            )}

            <p className="text-[10px] text-muted-foreground italic pl-1 flex items-center gap-1.5 mt-2">
              <AlertTriangle size={10} className="text-amber-500" /> {t('projectDetail.settings.redeployWarning')}
            </p>

            {/* Common Custom Commands Area */}
            <div className="space-y-4 pt-6 mt-6 border-t border-dashed">
              <div className="space-y-2">
                <Label className="text-xs uppercase tracking-widest text-muted-foreground flex items-center gap-2">
                  <Code2 size={14} className="text-primary" />
                  {t('projectDetail.settings.buildCommand')}
                </Label>
                <Textarea
                  value={buildCommandInput}
                  onChange={(e) => setBuildCommandInput(e.target.value)}
                  placeholder="e.g. npm install && npm run build"
                  className="min-h-[80px] text-xs font-mono border-muted-foreground/20"
                />
                <p className="text-[9px] text-muted-foreground italic">{t('projectDetail.settings.buildCommandDesc')}</p>
              </div>

              <div className="space-y-2">
                <Label className="text-xs uppercase tracking-widest text-muted-foreground flex items-center gap-2">
                  <Play size={14} className="text-primary" />
                  {t('projectDetail.settings.startCommand')}
                </Label>
                <Input
                  value={startCommandInput}
                  onChange={(e) => setStartCommandInput(e.target.value)}
                  placeholder={project.framework === 'Laravel' ? 'Leave empty for default PHP-FPM' : 'e.g. node dist/main.js'}
                  className="h-10 text-xs font-mono border-muted-foreground/20"
                />
                <p className="text-[9px] text-muted-foreground italic">{t('projectDetail.settings.startCommandDesc')}</p>
              </div>

              <div className="space-y-2 col-span-2">
                <Label className="text-xs uppercase text-muted-foreground tracking-wider font-semibold flex items-center gap-2">
                  <Play className="w-3 h-3" />
                  {t('projectDetail.settings.internalPort')}
                </Label>
                <Input
                  className="font-mono text-xs bg-zinc-900/50 border-zinc-800 focus:border-zinc-700"
                  placeholder="e.g. 5005"
                  type="number"
                  value={portInput === '' ? '' : portInput}
                  onChange={(e) => setPortInput(e.target.value ? parseInt(e.target.value) : '')}
                />
                <p className="text-[9px] text-muted-foreground italic">{t('projectDetail.settings.internalPortDesc')}</p>
              </div>
            </div>

          </CardContent>
        </Card>

        <div className="space-y-6">
          <Card>
            <CardHeader>
              <div className="flex items-center gap-4">
                <div className="w-10 h-10 bg-blue-500/10 rounded-lg flex items-center justify-center text-blue-600">
                  <RefreshCw className="w-5 h-5" />
                </div>
                <div>
                  <CardTitle className="text-lg">{t('projectDetail.settings.branchTitle')}</CardTitle>
                  <CardDescription>{t('projectDetail.settings.branchDesc')}</CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label className="text-[10px] uppercase tracking-widest text-muted-foreground font-semibold">{t('projectDetail.settings.branchTitle')}</Label>
                <div className="flex items-center gap-2 max-w-[290px]">
                  {branchesList.length > 0 && !forceManualInput ? (
                    <div className="relative flex-1">
                      <Select
                        value={branchInput}
                        onValueChange={(val) => setBranchInput(val || '')}
                      >
                        <SelectTrigger className="w-full h-9 px-3 rounded-lg border border-muted-foreground/15 hover:border-muted-foreground/30 bg-muted/5 hover:bg-muted/10 text-xs font-mono transition-all duration-200 outline-none focus:outline-none focus:ring-1 focus:ring-primary/20 focus:border-primary cursor-pointer flex items-center justify-between data-[size=default]:h-9 data-[size=default]:py-0 data-[size=default]:pr-3 data-[size=default]:pl-3">
                          <div className="flex items-center gap-2 text-left flex-1 min-w-0 pr-4">
                            <GitBranch className="w-3.5 h-3.5 text-primary shrink-0" />
                            <span className="truncate text-foreground/90 font-medium">{branchInput || t('newProject.selectBranch') || 'Select a branch'}</span>
                          </div>
                        </SelectTrigger>
                        <SelectContent align="start" alignItemWithTrigger={false} className="bg-popover/98 backdrop-blur-lg border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72">
                          {branchesList.map(branchName => (
                            <SelectItem key={branchName} value={branchName} className="rounded-lg py-2 px-3 cursor-pointer transition-colors focus:bg-accent/80 hover:bg-accent/40">
                              <div className="flex items-center gap-2">
                                <GitBranch className="w-3.5 h-3.5 text-muted-foreground shrink-0" />
                                <span className="font-mono text-xs text-foreground/90">{branchName}</span>
                              </div>
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  ) : (
                    <Input
                      value={branchInput}
                      onChange={(e) => setBranchInput(e.target.value)}
                      placeholder={t('projectDetail.settings.branchPlaceholder')}
                      className="h-9 flex-1 bg-muted/20 border-muted-foreground/10 focus:border-primary/30 transition-all text-xs font-mono"
                    />
                  )}

                  <Button
                    type="button"
                    variant="outline"
                    size="icon"
                    onClick={() => fetchBranches(true)}
                    disabled={isFetchingBranches}
                    className="h-9 w-9 shrink-0 rounded-lg hover:bg-muted/10 transition-all cursor-pointer flex items-center justify-center border border-muted-foreground/10"
                    title={t('projectDetail.settings.syncBranches') || 'Sync Branches'}
                  >
                    <RefreshCw className={cn("w-3.5 h-3.5 text-muted-foreground", isFetchingBranches && "animate-spin text-primary")} />
                  </Button>
                </div>
                {branchesList.length > 0 && (
                  <Button
                    variant="link"
                    type="button"
                    size="sm"
                    onClick={() => setForceManualInput(!forceManualInput)}
                    className="text-[9px] text-primary font-semibold h-auto p-0 mt-1 pl-0.5"
                  >
                    {forceManualInput
                      ? t('projectDetail.settings.useDropdown') || 'Select branch from list'
                      : t('projectDetail.settings.typeManually') || 'Use manual text input'
                    }
                  </Button>
                )}
                <p className="text-[9px] text-muted-foreground/60 italic pl-0.5 flex items-center gap-1.5 mt-1">
                  <AlertTriangle size={10} className="text-amber-500/50" /> {t('projectDetail.settings.redeployWarningBranch')}
                </p>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <div className="flex items-center gap-4">
                <div className="w-10 h-10 bg-muted/50 rounded-lg flex items-center justify-center text-muted-foreground">
                  <LayoutGrid className="w-5 h-5" />
                </div>
                <div>
                  <CardTitle className="text-lg">{t('newProject.baseDir')}</CardTitle>
                  <CardDescription>{t('newProject.baseDirDesc')}</CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label className="text-[10px] uppercase tracking-widest text-muted-foreground font-semibold">{t('newProject.baseDir')}</Label>
                <div className="flex gap-2">
                  <Input
                    value={baseDirInput}
                    onChange={(e) => setBaseDirInput(e.target.value)}
                    placeholder={t('newProject.baseDirPlaceholder')}
                    className="h-9 max-w-[240px] bg-muted/20 border-muted-foreground/10 focus:border-primary/30 transition-all text-xs"
                  />
                </div>
                <p className="text-[9px] text-muted-foreground/60 italic pl-0.5 flex items-center gap-1.5 mt-1">
                  <AlertTriangle size={10} className="text-amber-500/50" /> {t('projectDetail.settings.redeployWarningDirectory')}
                </p>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <div className="flex items-center gap-4">
                <div className="w-10 h-10 bg-primary/10 rounded-lg flex items-center justify-center text-primary">
                  <DbIcon className="w-5 h-5" />
                </div>
                <div>
                  <CardTitle className="text-lg">{t('common.databases')}</CardTitle>
                  <CardDescription>
                    {t("databaseManager.attachDetachDesc")}
                  </CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {project.database_instance ? (
                <div className="flex items-center justify-between p-4 rounded-xl border bg-primary/5 border-primary/20">
                  <div className="space-y-1">
                    <Label className="text-sm font-bold flex items-center gap-2">
                      <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
                      {project.database_instance.name}
                    </Label>
                    <Badge variant="outline" className={cn(
                      "text-[9px] font-black uppercase px-2 py-0.5",
                      project.database_instance.engine === 'mysql'
                        ? 'border-amber-500/20 bg-amber-500/5 text-amber-500'
                        : 'border-blue-500/20 bg-blue-500/5 text-blue-500'
                    )}>
                      {project.database_instance.engine}
                    </Badge>
                  </div>
                  <Button
                    variant="outline"
                    onClick={async () => {
                      if (!project.database_instance) return
                      setIsDbActionLoading(true)
                      try {
                        await databaseAPI.detach(project.database_instance.uid)
                        toast.success(t('common.success'))
                        setShowDbRedeployModal(true)
                      } catch (err: unknown) {
                        const error = err as { response?: { data?: { error?: string } } };
                        toast.error(error.response?.data?.error || t('common.error'))
                      } finally {
                        setIsDbActionLoading(false)
                      }
                    }}
                    disabled={isDbActionLoading}
                    className="text-xs font-bold uppercase tracking-widest text-rose-500 border-rose-500/20 hover:bg-rose-500/5"
                  >
                    {t('databaseManager.detach')}
                  </Button>
                </div>
              ) : (
                <div className="space-y-4">
                  <div className="border border-dashed border-border rounded-xl p-4 text-center bg-muted/5">
                    <p className="text-xs text-muted-foreground font-semibold">
                      {t('databaseManager.noDatabaseAttached')}
                    </p>
                    <p className="text-[10px] text-muted-foreground/60 mt-1 italic">
                      {t('databaseManager.attachDesc')}
                    </p>
                  </div>

                  <div className="flex items-center gap-2">
                    <Select
                      value={selectedDbId}
                      onValueChange={(val) => setSelectedDbId(val || '')}
                    >
                      <SelectTrigger className="flex-1 h-9 px-3 text-xs bg-background/50 border-border hover:border-border/80">
                        <div className="flex items-center gap-2 text-left flex-1 min-w-0 pr-4">
                          {(() => {
                            const db = databasesList.find(d => d.uid === selectedDbId)
                            return db ? (
                              <span className="truncate font-semibold text-foreground/90 text-xs">{db.name}</span>
                            ) : (
                              <span className="text-muted-foreground/60 text-xs">{t('databaseManager.selectDatabase')}</span>
                            )
                          })()}
                        </div>
                      </SelectTrigger>
                      <SelectContent align="start" alignItemWithTrigger={false} className="bg-popover border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72 min-w-[var(--anchor-width)] w-[var(--anchor-width)]">
                        {databasesList.map(db => (
                          <SelectItem key={db.id} value={db.uid} className="rounded-lg py-2 px-3 cursor-pointer">
                            <div className="flex items-center gap-2 text-left">
                              <span className="font-semibold text-foreground text-xs">{db.name}</span>
                              <Badge variant="outline" className={cn(
                                "text-[8px] font-black uppercase px-1.5 py-0 shrink-0",
                                db.engine === 'mysql'
                                  ? 'border-amber-500/20 bg-amber-500/5 text-amber-500'
                                  : 'border-blue-500/20 bg-blue-500/5 text-blue-500'
                              )}>
                                {db.engine}
                              </Badge>
                            </div>
                          </SelectItem>
                        ))}
                        {databasesList.length === 0 && (
                          <div className="text-center py-4 text-[10px] uppercase font-bold text-muted-foreground">
                            {t("databaseManager.noUnattachedDbs")}
                          </div>
                        )}
                      </SelectContent>
                    </Select>
                    <Button
                      onClick={async () => {
                        if (!selectedDbId) return
                        setIsDbActionLoading(true)
                        try {
                          await databaseAPI.attach(selectedDbId, project.uid)
                          toast.success(t('common.success'))
                          setShowDbRedeployModal(true)
                          setSelectedDbId('')
                        } catch (err: unknown) {
                          const error = err as { response?: { data?: { error?: string } } };
                          toast.error(error.response?.data?.error || t('common.error'))
                        } finally {
                          setIsDbActionLoading(false)
                        }
                      }}
                      disabled={!selectedDbId || isDbActionLoading}
                      size="sm"
                      className="text-xs font-bold uppercase tracking-widest h-9"
                    >
                      {t('databaseManager.attach')}
                    </Button>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>


          <Card>
            <CardHeader>
              <div className="flex items-center gap-4">
                <div className="w-10 h-10 bg-primary/10 rounded-lg flex items-center justify-center text-primary">
                  <Github className="w-5 h-5" />
                </div>
                <div>
                  <CardTitle className="text-lg">{t('user.settings.gitConfigTitle')}</CardTitle>
                  <CardDescription>{t('user.settings.gitConfigDesc')}</CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent className="space-y-6">
              {/* Connection Mode Toggle */}
              <div className="flex rounded-lg p-1 bg-muted/20 border border-muted-foreground/10 gap-1">
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => setGitConnectionMode('github_app')}
                  className={cn(
                    "flex-1 py-2 px-3 text-xs font-bold rounded-md transition-all duration-200 cursor-pointer flex items-center justify-center gap-2 h-auto hover:bg-transparent",
                    gitConnectionMode === 'github_app'
                      ? "bg-card text-primary shadow-sm border border-border/40 font-extrabold hover:text-primary hover:bg-card"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                  style={{ cursor: 'pointer' }}
                >
                  <Github className="w-3.5 h-3.5" />
                  GitHub App
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => setGitConnectionMode('manual')}
                  className={cn(
                    "flex-1 py-2 px-3 text-xs font-bold rounded-md transition-all duration-200 cursor-pointer flex items-center justify-center gap-2 h-auto hover:bg-transparent",
                    gitConnectionMode === 'manual'
                      ? "bg-card text-primary shadow-sm border border-border/40 font-extrabold hover:text-primary hover:bg-card"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                  style={{ cursor: 'pointer' }}
                >
                  <Globe className="w-3.5 h-3.5" />
                  Manual Git URL
                </Button>
              </div>

              {/* Mode 1: GitHub App */}
              {gitConnectionMode === 'github_app' && (
                <div className="space-y-4">
                  {isGithubInstallationsLoading ? (
                    <div className="flex items-center justify-center py-6 gap-2 text-muted-foreground">
                      <Loader2 className="w-4 h-4 animate-spin text-primary" />
                      <span className="text-xs">{t('projectDetail.settings.loadingAccounts')}</span>
                    </div>
                  ) : memoizedGithubInstallations.length > 0 ? (
                    <div className="space-y-4">
                      {/* Installation Select */}
                      <div className="space-y-2">
                        <Label className="text-[10px] uppercase tracking-widest text-muted-foreground font-semibold">{t('projectDetail.settings.githubAccount')}</Label>
                        <Select
                          value={githubInstallationIdInput ? String(githubInstallationIdInput) : ''}
                          onValueChange={(val) => {
                            const idNum = Number(val)
                            setGithubInstallationIdInput(idNum)
                            setGithubRepoOwnerInput('')
                            setGithubRepoNameInput('')
                            setGithubUrlInput('')
                          }}
                        >
                          <SelectTrigger className="w-full h-10 px-3 bg-muted/20 border-muted-foreground/15 text-xs">
                            <div className="flex items-center gap-2 text-left flex-1 min-w-0 pr-4">
                              {(() => {
                                const inst = memoizedGithubInstallations.find(i => i.installation_id === githubInstallationIdInput)
                                if (inst) {
                                  return (
                                    <>
                                      {inst.avatar_url ? (
                                        <img src={inst.avatar_url} alt={inst.account_name} className="w-4 h-4 rounded-full border border-border/40 shrink-0" />
                                      ) : (
                                        <Github className="w-4 h-4 text-muted-foreground shrink-0" />
                                      )}
                                      <span className="truncate text-foreground/90 font-medium">{inst.account_name}</span>
                                    </>
                                  )
                                }
                                return <span className="text-muted-foreground/60">{t('projectDetail.settings.selectAccount')}</span>
                              })()}
                            </div>
                          </SelectTrigger>
                          <SelectContent align="start" className="bg-popover border border-border rounded-xl shadow-2xl p-1 max-h-72">
                            {memoizedGithubInstallations.map((inst) => (
                              <SelectItem key={inst.installation_id} value={String(inst.installation_id)} className="rounded-lg py-2 cursor-pointer">
                                <div className="flex items-center gap-2">
                                  {inst.avatar_url ? (
                                    <img src={inst.avatar_url} alt={inst.account_name} className="w-4 h-4 rounded-full" />
                                  ) : (
                                    <Github className="w-4 h-4" />
                                  )}
                                  <span>{inst.account_name}</span>
                                </div>
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>

                      {/* Repository Select */}
                      {githubInstallationIdInput && (
                        <div className="space-y-2">
                          <Label className="text-[10px] uppercase tracking-widest text-muted-foreground font-semibold">{t('projectDetail.settings.repositoryLabel')}</Label>
                          {isGithubReposLoading ? (
                            <div className="flex items-center gap-2 text-xs text-muted-foreground py-2 pl-1">
                              <Loader2 className="w-3.5 h-3.5 animate-spin text-primary" />
                              <span>{t('projectDetail.settings.loadingRepos')}</span>
                            </div>
                          ) : memoizedGithubRepos.length > 0 ? (
                            <Select
                              value={githubRepoNameInput ? `${githubRepoOwnerInput}/${githubRepoNameInput}` : ''}
                              onValueChange={(val) => {
                                if (!val) return
                                const parts = val.split('/')
                                if (parts.length === 2) {
                                  setGithubRepoOwnerInput(parts[0])
                                  setGithubRepoNameInput(parts[1])

                                  // Find selected repo details to set URL
                                  const repoDetail = memoizedGithubRepos.find(r => r.full_name === val)
                                  if (repoDetail) {
                                    setGithubUrlInput(repoDetail.html_url)
                                  }
                                }
                              }}
                            >
                              <SelectTrigger className="w-full h-10 px-3 bg-muted/20 border-muted-foreground/15 text-xs font-mono">
                                <SelectValue placeholder={t('projectDetail.settings.selectRepo')} />
                              </SelectTrigger>
                              <SelectContent align="start" className="bg-popover border border-border rounded-xl shadow-2xl p-1 max-h-72">
                                {memoizedGithubRepos.map((repo) => (
                                  <SelectItem key={repo.id} value={repo.full_name} className="rounded-lg py-2 cursor-pointer font-mono text-xs">
                                    <span>{repo.name}</span>
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          ) : (
                            <p className="text-xs text-muted-foreground italic pl-1">{t('projectDetail.settings.noRepos')}</p>
                          )}
                        </div>
                      )}
                    </div>
                  ) : (
                    <div className="border border-dashed border-border rounded-xl p-6 text-center space-y-3 bg-muted/5">
                      <p className="text-xs text-muted-foreground max-w-sm mx-auto">
                        {t('projectDetail.settings.noAccounts')}
                      </p>
                      <Button
                        type="button"
                        onClick={() => {
                          const appUrl = import.meta.env.VITE_GITHUB_APP_URL || 'https://github.com/apps/laravel-paas-local'
                          window.open(`${appUrl}/installations/new`, '_blank')
                        }}
                        className="gap-2 h-9 rounded-lg mx-auto text-xs"
                      >
                        <Github className="w-3.5 h-3.5" />
                        Configure GitHub App
                        <ExternalLink className="w-3 h-3" />
                      </Button>
                    </div>
                  )}
                </div>
              )}

              {/* Mode 2: Manual Git URL */}
              {gitConnectionMode === 'manual' && (
                <div className="space-y-4">
                  <div className="space-y-2">
                    <Label className="text-[10px] uppercase tracking-widest text-muted-foreground font-semibold">Repository Git URL</Label>
                    <Input
                      value={githubUrlInput}
                      onChange={(e) => setGithubUrlInput(e.target.value)}
                      placeholder="e.g. git@github.com:username/repository.git"
                      className="h-10 text-xs bg-muted/20 border-muted-foreground/10 focus:border-primary/30 transition-all font-mono"
                    />
                    <p className="text-[9px] text-muted-foreground italic leading-relaxed pl-0.5 mt-1">
                      Note: For private repositories, make sure the server's SSH key is added to the repository's deploy keys.
                    </p>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </div>

      {isSettingsDirty && (
        <div className="fixed bottom-10 left-1/2 -translate-x-1/2 z-40 animate-in fade-in slide-in-from-bottom-10 duration-500">
          <div className="relative group">
            <div className="absolute -inset-1 bg-gradient-to-r from-primary/50 to-blue-500/50 rounded-[22px] blur-xl opacity-20 group-hover:opacity-40 transition duration-1000 group-hover:duration-200"></div>

            <Card className="relative bg-zinc-950/90 backdrop-blur-2xl border-white/10 shadow-[0_20px_50px_rgba(0,0,0,0.5)] overflow-hidden min-w-[360px] rounded-[20px]">
              <div className="absolute top-0 left-0 w-full h-[1px] bg-gradient-to-r from-transparent via-white/20 to-transparent" />

              <CardContent className="p-3 flex items-center justify-between gap-8">
                <div className="flex items-center gap-4 pl-3">
                  <div className="relative flex h-2 w-2">
                    <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-primary opacity-75"></span>
                    <span className="relative inline-flex rounded-full h-2 w-2 bg-primary"></span>
                  </div>
                  <div className="flex flex-col">
                    <span className="text-[10px] font-black uppercase tracking-[0.2em] text-white/90 leading-tight">Configuration Changed</span>
                    <span className="text-[9px] text-white/40 font-bold uppercase tracking-widest">{t('common.settings')}</span>
                  </div>
                </div>

                <div className="flex items-center gap-2 pr-1">
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={handleResetSettings}
                    disabled={isSavingSettings}
                    className="text-[10px] font-black uppercase tracking-widest h-10 px-4 text-white/50 hover:text-white hover:bg-white/5 transition-all"
                  >
                    {t('common.cancel')}
                  </Button>
                  <Button
                    size="sm"
                    onClick={handleSaveSettings}
                    disabled={isSavingSettings}
                    className="relative group/btn bg-white text-black hover:bg-white/90 h-10 px-6 rounded-full font-black text-[10px] uppercase tracking-wider transition-all overflow-hidden"
                  >
                    {isSavingSettings ? (
                      <Loader2 className="w-3.5 h-3.5 animate-spin" />
                    ) : (
                      <div className="flex items-center gap-2">
                        <Save className="w-3.5 h-3.5" />
                        <span>{t('common.save')}</span>
                      </div>
                    )}
                  </Button>
                </div>
              </CardContent>
            </Card>
          </div>
        </div>
      )}

      {/* Redeploy Confirmation Dialog */}
      <Dialog open={showDbRedeployModal} onOpenChange={(open) => {
        if (!open) {
          setShowDbRedeployModal(false)
          window.location.reload()
        }
      }}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <div className="flex items-center gap-3 text-primary mb-2">
              <Settings className="w-6 h-6 animate-spin-slow" />
              <DialogTitle className="text-lg font-bold">{t("databaseManager.redeployTitle")}</DialogTitle>
            </div>
            <DialogDescription className="text-xs leading-relaxed">
              {t('databaseManager.redeployConfirm')}
            </DialogDescription>
          </DialogHeader>

          <DialogFooter className="gap-2">
            <Button
              variant="ghost"
              onClick={() => {
                setShowDbRedeployModal(false)
                window.location.reload()
              }}
              className="text-xs font-bold uppercase tracking-wider"
            >
              {t('databaseManager.later')}
            </Button>
            <Button
              onClick={async () => {
                setIsDbActionLoading(true)
                try {
                  await projectsAPI.redeploy(project.uid)
                  toast.success(t('common.success'))
                  setShowDbRedeployModal(false)
                  window.location.reload()
                } catch (error: unknown) {
                  const err = error as { response?: { data?: { error?: string } } };
                  toast.error(err.response?.data?.error || t('common.error'))
                } finally {
                  setIsDbActionLoading(false)
                }
              }}
              disabled={isDbActionLoading}
              className="text-xs font-bold uppercase tracking-widest"
            >
              {t('databaseManager.redeployNow')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
