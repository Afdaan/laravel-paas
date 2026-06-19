import React, { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { projectsAPI, githubAPI, databaseAPI } from '../../services/api'
import { AxiosError } from 'axios'
import useTranslation from '../../lib/useTranslation'
import { GithubAppInstallation, GithubRepository, DatabaseInstance } from '../../types'
import {
  Rocket,
  Database,
  Settings,
  Info,
  ChevronLeft,
  ArrowRight,
  Loader2,
  Layout,
  Terminal,
  Play,
  GitBranch,
  ExternalLink
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import { Select, SelectContent, SelectItem, SelectTrigger } from '@/components/ui/select'
import { siMysql, siPostgresql } from 'simple-icons'

const GithubIcon = (props: React.SVGProps<SVGSVGElement>) => (
  <svg
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    strokeWidth="2"
    strokeLinecap="round"
    strokeLinejoin="round"
    {...props}
  >
    <path d="M15 22v-4a4.8 4.8 0 0 0-1-3.5c3 0 6-2 6-5.5.08-1.25-.27-2.48-1-3.5.28-1.15.28-2.35 0-3.5 0 0-1 0-3 1.5-2.64-.5-5.36-.5-8 0C6 2 5 2 5 2c-.3 1.15-.3 2.35 0 3.5A5.403 5.403 0 0 0 4 9c0 3.5 3 5.5 6 5.5-.39.49-.68 1.05-.85 1.65-.17.6-.22 1.23-.15 1.85v4" />
    <path d="M9 18c-4.51 2-5-2-7-2" />
  </svg>
)

interface NewProjectForm {
  name: string;
  github_url: string;
  branch: string;
  database_name: string;
  database_username: string;
  database_password: string;
  base_directory: string;
  build_command: string;
  start_command: string;
  port: number | '';
  queue_enabled: boolean;
  enable_database: boolean;
  database_option: 'none' | 'sqlite' | 'new' | 'existing' | 'external';
  database_engine: 'mysql' | 'postgresql';
  existing_database_uid: string;
  github_installation_id?: number;
  github_repo_owner?: string;
  github_repo_name?: string;
}

interface ValidationErrors {
  name?: string;
  github_url?: string;
  database_name?: string;
  existing_database_uid?: string;
}

const databaseEngines = [
  { value: 'mysql', label: 'MySQL (8.0)', icon: siMysql },
  { value: 'postgresql', label: 'PostgreSQL (15)', icon: siPostgresql },
] as const

type DatabaseEngineOption = (typeof databaseEngines)[number]

const DatabaseEngineBadge = ({ engine, className }: { engine: DatabaseEngineOption; className?: string }) => {
  const fillColor = `#${engine.icon.hex}`

  return (
    <div
      className={cn(
        'flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border shadow-sm transition-colors duration-200',
        engine.value === 'mysql' && 'border-amber-500/20 bg-amber-500/10 text-amber-500',
        engine.value === 'postgresql' && 'border-sky-500/20 bg-sky-500/10 text-sky-500',
        className
      )}
    >
      <svg viewBox='0 0 24 24' aria-hidden='true' className='h-4.5 w-4.5 drop-shadow-sm'>
        <path fill={fillColor} d={engine.icon.path} />
      </svg>
    </div>
  )
}

function UserNewProject() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [isLoading, setIsLoading] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)

  const [connectionMode, setConnectionMode] = useState<'app' | 'manual'>('app')
  const [installations, setInstallations] = useState<GithubAppInstallation[]>([])
  const [repositories, setRepositories] = useState<GithubRepository[]>([])
  const [branches, setBranches] = useState<{ name: string }[]>([])
  const [selectedInstallationId, setSelectedInstallationId] = useState<string>('')
  const [selectedRepoFullName, setSelectedRepoFullName] = useState<string>('')
  const [isGithubLoading, setIsGithubLoading] = useState<boolean>(false)

  const [formData, setFormData] = useState<NewProjectForm>({
    name: '',
    github_url: '',
    branch: '',
    database_name: '',
    database_username: '',
    database_password: '',
    base_directory: '',
    build_command: '',
    start_command: '',
    port: '',
    queue_enabled: false,
    enable_database: true,
    database_option: 'new',
    database_engine: 'mysql',
    existing_database_uid: '',
  })
  const [validationErrors, setValidationErrors] = useState<ValidationErrors>({})
  const [existingDatabases, setExistingDatabases] = useState<DatabaseInstance[]>([])
  const [isLoadingDatabases, setIsLoadingDatabases] = useState(false)

  const generatePassword = useCallback(() => {
    const safeChars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!$%-+*='
    let pass = ''
    for (let i = 0; i < 20; i++) {
      pass += safeChars.charAt(Math.floor(Math.random() * safeChars.length))
    }
    return pass
  }, [])

  const loadExistingDatabases = useCallback(async () => {
    setIsLoadingDatabases(true)
    try {
      const response = await databaseAPI.listOwn()
      const dbs = response.data.databases || []
      const unattached = dbs.filter((db: any) => db.project_id === null && db.status !== 'deleted')
      setExistingDatabases(unattached)
      if (unattached.length > 0) {
        setFormData(prev => ({
          ...prev,
          existing_database_uid: unattached[0].uid
        }))
      }
    } catch {
      toast.error('Failed to load existing databases')
    } finally {
      setIsLoadingDatabases(false)
    }
  }, [])

  useEffect(() => {
    const generated = generatePassword()
    setFormData(prev => ({
      ...prev,
      database_password: generated
    }))
  }, [generatePassword])

  useEffect(() => {
    if (formData.database_option === 'existing') {
      loadExistingDatabases()
    }
  }, [formData.database_option, loadExistingDatabases])

  const repoQuerySeq = React.useRef(0)
  const branchQuerySeq = React.useRef(0)
  const isLinkingRef = React.useRef(false)
  const revokedToastShown = React.useRef(false)
  const selectedInstallationIdRef = React.useRef('')
  const loadInstallationsRef = React.useRef<(triggerRepoLoad?: boolean) => Promise<void>>(async () => {})

  // Keep ref in sync with state to prevent stale closures in async and focus callbacks
  React.useEffect(() => {
    selectedInstallationIdRef.current = selectedInstallationId
  }, [selectedInstallationId])

  const currentInst = React.useMemo(() =>
    installations.find(inst => String(inst.installation_id) === selectedInstallationId),
    [installations, selectedInstallationId]
  )

  const currentRepo = React.useMemo(() =>
    repositories.find(r => r.full_name === selectedRepoFullName),
    [repositories, selectedRepoFullName]
  )

  const currentBranch = React.useMemo(() =>
    branches.find(b => b.name === formData.branch),
    [branches, formData.branch]
  )

  const loadRepositories = useCallback(async (installationId: string) => {
    const currentSeq = ++repoQuerySeq.current
    setIsGithubLoading(true)
    try {
      const response = await githubAPI.listRepositories(installationId)
      if (currentSeq === repoQuerySeq.current) {
        setRepositories(response.data.data || [])
        revokedToastShown.current = false
      }
    } catch (err) {
      if (currentSeq === repoQuerySeq.current) {
        const axiosError = err as AxiosError<{ error: string, code?: string }>
        const errorCode = axiosError.response?.data?.code
        const isRevoked = axiosError.response?.status === 404 || errorCode === 'INSTALLATION_REVOKED'

        setRepositories([])
        if (isRevoked) {
          // Deselect the revoked installation to prevent retry loops
          setSelectedInstallationId('')
          if (!revokedToastShown.current) {
            revokedToastShown.current = true
            toast.error(t('newProject.errors.installationRevoked'))
          }
          // Access via ref to avoid circular dependency between callbacks
          loadInstallationsRef.current(false)
        } else {
          toast.error(t('newProject.errors.failedToLoadRepos'))
        }
      }
    } finally {
      if (currentSeq === repoQuerySeq.current) {
        setIsGithubLoading(false)
      }
    }
  }, [t])

  const loadInstallations = useCallback(async (triggerRepoLoad = false) => {
    setIsGithubLoading(true)
    try {
      const response = await githubAPI.listInstallations()
      const insts: GithubAppInstallation[] = response.data.data || []
      setInstallations(insts)

      if (insts.length > 0) {
        const currentId = selectedInstallationIdRef.current
        const isStillValid = insts.some(inst => String(inst.installation_id) === currentId)

        if (currentId && isStillValid) {
          if (triggerRepoLoad) {
            loadRepositories(currentId)
          }
        } else {
          // Select first installation by default if none selected or the previous was uninstalled
          const firstInstId = String(insts[0].installation_id)
          setSelectedInstallationId(firstInstId)
          if (triggerRepoLoad) {
            loadRepositories(firstInstId)
          }
        }
      } else {
        setSelectedInstallationId('')
        setRepositories([])
      }
    } catch {
      // Installation list fetch failed — silently degrade, user can retry via UI
    } finally {
      setIsGithubLoading(false)
    }
  }, [loadRepositories])

  // Keep ref in sync so callbacks and effects always access the latest version
  // without needing to re-register listeners or re-trigger mount effects.
  React.useEffect(() => {
    loadInstallationsRef.current = loadInstallations
  }, [loadInstallations])

  // Mount-only: read URL params and either link a GitHub installation or
  // load the installations list. Uses refs to avoid re-running on deps change.
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const installationId = params.get('installation_id')
    if (installationId) {
      if (isLinkingRef.current) return
      isLinkingRef.current = true

      const linkGithub = async () => {
        setIsGithubLoading(true)
        try {
          await githubAPI.linkInstallation(installationId)
          toast.success(t('newProject.githubConnected') || 'GitHub App connected successfully')
          const cleanUrl = window.location.pathname
          window.history.replaceState({}, document.title, cleanUrl)
          await loadInstallationsRef.current(true)
        } catch {
          toast.error(t('newProject.errors.failedToLink'))
        } finally {
          setIsGithubLoading(false)
        }
      }
      linkGithub()
    } else {
      loadInstallationsRef.current(true)
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Stable focus listener — syncs installations when user returns from GitHub.
  // Uses ref so the listener never needs to be re-attached.
  useEffect(() => {
    const handleFocus = () => {
      loadInstallationsRef.current(false)
    }
    window.addEventListener('focus', handleFocus)
    return () => {
      window.removeEventListener('focus', handleFocus)
    }
  }, [])

  const loadBranches = async (owner: string, repo: string, currentReposList = repositories) => {
    const currentSeq = ++branchQuerySeq.current
    setIsGithubLoading(true)
    try {
      const response = await githubAPI.listBranches(owner, repo, selectedInstallationId)
      if (currentSeq === branchQuerySeq.current) {
        if (response.data.warning) {
          toast.warning(response.data.warning)
        }
        const raw = response.data.data || []
        const normalized = raw.map((b: string | { name: string }) => {
          if (typeof b === 'string') {
            return { name: b }
          }
          return { name: b?.name || '' }
        })
        setBranches(normalized)
        const repoDetails = currentReposList.find(r => r.full_name === `${owner}/${repo}`)
        const defaultBranch = repoDetails?.default_branch || 'main'
        setFormData(prev => ({ ...prev, branch: defaultBranch }))
      }
    } catch (err) {
      if (currentSeq === branchQuerySeq.current) {
        const axiosError = err as AxiosError<{ error: string, code?: string }>
        const errorCode = axiosError.response?.data?.code
        const isRevoked = axiosError.response?.status === 404 || errorCode === 'INSTALLATION_REVOKED'
        if (isRevoked) {
          setSelectedInstallationId('')
          setBranches([])
          if (!revokedToastShown.current) {
            revokedToastShown.current = true
            toast.error(t('newProject.errors.installationRevoked'))
          }
          loadInstallations(false)
        } else {
          setBranches([])
          toast.error(t('newProject.errors.failedToLoadBranches'))
        }
      }
    } finally {
      if (currentSeq === branchQuerySeq.current) {
        setIsGithubLoading(false)
      }
    }
  }

  const handleRepoValueChange = (fullName: string) => {
    setSelectedRepoFullName(fullName)
    if (!fullName) {
      setFormData(prev => ({
        ...prev,
        github_url: '',
        github_repo_owner: undefined,
        github_repo_name: undefined,
      }))
      setBranches([])
      return
    }

    const [owner, repoName] = fullName.split('/')
    const selectedRepo = repositories.find(r => r.full_name === fullName)
    if (selectedRepo) {
      const pName = selectedRepo.name
        .replace(/[-_]+/g, ' ')
        .replace(/\b\w/g, c => c.toUpperCase())

      const dbName = selectedRepo.name.toLowerCase()
        .replace(/[^a-z0-9]+/g, '_')
        .replace(/^_|_$/g, '')

      const dbUser = `dbuser_${dbName.slice(0, 25)}`

      setFormData(prev => ({
        ...prev,
        name: pName,
        github_url: selectedRepo.html_url,
        database_name: dbName,
        database_username: dbUser,
        github_installation_id: Number(selectedInstallationId),
        github_repo_owner: owner,
        github_repo_name: repoName,
      }))

      loadBranches(owner, repoName)
    }
  }

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, value, type, checked } = e.target
    const newValue = type === 'checkbox' ? checked : value

    setFormData(prev => ({
      ...prev,
      [name]: newValue
    }))

    if (name === 'name') {
      const dbName = value.toLowerCase()
        .replace(/[^a-z0-9]+/g, '_')
        .replace(/^_|_$/g, '')
      const dbUser = `dbuser_${dbName.slice(0, 25)}`
      setFormData(prev => ({
        ...prev,
        database_name: dbName,
        database_username: dbUser
      }))
    }

    if (validationErrors[name as keyof ValidationErrors]) {
      setValidationErrors(prev => ({ ...prev, [name]: undefined }))
    }
  }

  const validateForm = () => {
    const errors: ValidationErrors = {}
    if (!formData.name.trim()) errors.name = t('common.validation.required', { field: t('newProject.displayName') })
    if (!formData.github_url.trim()) errors.github_url = t('common.validation.required', { field: t('newProject.repoUrl') })
    if (formData.database_option === 'new' && !formData.database_name.trim()) {
      errors.database_name = t('common.validation.required', { field: t('newProject.dbName') })
    }
    if (formData.database_option === 'existing' && !formData.existing_database_uid) {
      errors.existing_database_uid = t('newProject.dbConfig.existing.placeholder')
    }

    setValidationErrors(errors)
    return Object.keys(errors).length === 0
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (isLoading) return // Block duplicate concurrent submits

    if (!validateForm()) return

    setIsLoading(true)
    setSubmitError(null)

    try {
      const payload = {
        ...formData,
        port: formData.port === '' ? undefined : Number(formData.port),
      }
      const response = await projectsAPI.create(payload)
      toast.success(t('common.success'))
      navigate(`/projects/${response.data.project.uid}`)
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ error: string }>
      let errorMsg = axiosError.response?.data?.error || t('common.actionFailed')

      if (errorMsg === 'Project limit reached' || axiosError.response?.status === 403) {
        errorMsg = t('newProject.restrictedDesc')
      }

      setSubmitError(errorMsg)
      toast.error(errorMsg)
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div className="max-w-3xl mx-auto space-y-10 pb-20 animate-in fade-in duration-500">
      <div className="flex flex-col gap-4">
        <Button
          variant="outline"
          size="sm"
          onClick={() => navigate(-1)}
          className="w-fit gap-2"
        >
          <ChevronLeft className="w-4 h-4" />
          {t('newProject.back')}
        </Button>

        <div className="space-y-2">
          <h1 className="text-4xl font-bold tracking-tight">{t('newProject.title').split(' ')[0]} <span className="text-primary italic">{t('newProject.title').split(' ')[1]}</span></h1>
          <p className="text-muted-foreground text-lg">{t('newProject.subtitle')}</p>
        </div>
      </div>

      {submitError && (
        <Card className="border-destructive/20 bg-destructive/5 p-6 animate-in zoom-in-95">
          <div className="flex flex-col items-center justify-center text-center gap-3">
            <div className="w-12 h-12 bg-destructive/10 rounded-full flex items-center justify-center text-destructive">
              <Info className="w-6 h-6" />
            </div>
            <h3 className="text-lg font-bold uppercase tracking-tight">{t('newProject.restricted')}</h3>
            <p className="text-sm font-medium text-destructive/80 max-w-lg">{submitError}</p>
          </div>
        </Card>
      )}

      <form onSubmit={handleSubmit} className="max-w-3xl mx-auto space-y-8">
        <div className="space-y-8">
          <Card className="p-8">
            <div className="space-y-8">
              {/* Git Connection Method */}
              <div className="space-y-4">
                <div className="flex items-center gap-3">
                  <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center text-primary">
                    <GithubIcon className="w-4 h-4" />
                  </div>
                  <Label className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                    {t('newProject.gitSource')}
                  </Label>
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <Button
                    variant="outline"
                    type="button"
                    onClick={() => {
                      setConnectionMode('app')
                      setFormData(prev => ({
                        ...prev,
                        github_url: '',
                        github_installation_id: undefined,
                        github_repo_owner: undefined,
                        github_repo_name: undefined
                      }))
                      setSelectedRepoFullName('')
                      setBranches([])
                    }}
                    className={cn(
                      "flex items-center justify-center gap-3 p-4 h-auto rounded-xl text-sm font-semibold",
                      connectionMode === 'app'
                        ? "border-primary bg-primary/5 text-primary"
                        : "border-border hover:border-muted-foreground/30 hover:bg-muted/10 text-muted-foreground"
                    )}
                  >
                    <GithubIcon className="w-5 h-5" />
                    {t('newProject.githubApp')}
                  </Button>
                  <Button
                    variant="outline"
                    type="button"
                    onClick={() => {
                      setConnectionMode('manual')
                      setFormData(prev => ({
                        ...prev,
                        github_installation_id: undefined,
                        github_repo_owner: undefined,
                        github_repo_name: undefined
                      }))
                    }}
                    className={cn(
                      "flex items-center justify-center gap-3 p-4 h-auto rounded-xl text-sm font-semibold",
                      connectionMode === 'manual'
                        ? "border-primary bg-primary/5 text-primary"
                        : "border-border hover:border-muted-foreground/30 hover:bg-muted/10 text-muted-foreground"
                    )}
                  >
                    <Terminal className="w-5 h-5" />
                    {t('newProject.manualGitUrl')}
                  </Button>
                </div>
              </div>

              {/* GitHub App Connection Section */}
              {connectionMode === 'app' && (
                <div className="space-y-6 border-t pt-6">
                  {isGithubLoading && installations.length === 0 ? (
                    <div className="flex flex-col items-center justify-center py-12 gap-3 text-muted-foreground">
                      <Loader2 className="w-8 h-8 animate-spin text-primary" />
                      <p className="text-sm">{ t('newProject.accessingGithub')}</p>
                    </div>
                  ) : installations.length === 0 ? (
                    <div className="border border-dashed border-border rounded-xl p-8 text-center space-y-4 bg-muted/5">
                      <div className="w-12 h-12 rounded-xl bg-muted flex items-center justify-center mx-auto text-muted-foreground">
                        <GithubIcon className="w-6 h-6" />
                      </div>
                      <div className="space-y-1">
                        <h4 className="font-bold text-base">{t('newProject.connectGithub')}</h4>
                        <p className="text-sm text-muted-foreground max-w-md mx-auto">
                          {t('newProject.connectGithubDesc')}
                        </p>
                      </div>
                      <Button
                        type="button"
                        onClick={() => {
                          const appUrl = import.meta.env.VITE_GITHUB_APP_URL || 'https://github.com/apps/laravel-paas-local'
                          window.open(`${appUrl}/installations/new`, '_blank')
                        }}
                        className="gap-2 mx-auto"
                      >
                        <GithubIcon className="w-4 h-4" />
                        {t('newProject.configureGithubApp')}
                        <ExternalLink className="w-3.5 h-3.5" />
                      </Button>
                    </div>
                  ) : (
                    <div className="space-y-6">
                      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                        {/* GitHub Account Selector */}
                        <div className="space-y-2">
                          <Label htmlFor="installation_id" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                            {t('newProject.githubAccount')}
                          </Label>
                          <div className="relative">
                            <Select
                              value={selectedInstallationId}
                              onValueChange={(val) => {
                                if (val) {
                                  setSelectedInstallationId(val)
                                  loadRepositories(val)
                                  setSelectedRepoFullName('')
                                  setBranches([])
                                  setFormData(prev => ({
                                    ...prev,
                                    github_url: '',
                                    github_installation_id: undefined,
                                    github_repo_owner: undefined,
                                    github_repo_name: undefined,
                                    branch: '',
                                  }))
                                }
                              }}
                            >
                              <SelectTrigger className="w-full h-11 px-4 rounded-xl border border-border/60 hover:border-border dark:hover:border-foreground/20 bg-background/50 hover:bg-background/80 text-sm font-medium transition-all duration-200 outline-none focus:outline-none focus:ring-1 focus:ring-primary/20 focus:border-primary cursor-pointer flex items-center justify-between data-[size=default]:h-11 data-[size=default]:py-0 data-[size=default]:pr-4 data-[size=default]:pl-4">
                                <div className="flex items-center gap-2.5 text-left flex-1 min-w-0 pr-4">
                                  {currentInst ? (
                                    <>
                                      {currentInst.avatar_url ? (
                                        <img src={currentInst.avatar_url} alt={currentInst.account_name} className="w-5.5 h-5.5 rounded-full border border-border/40 shrink-0" />
                                      ) : (
                                        <GithubIcon className="w-4.5 h-4.5 text-muted-foreground shrink-0" />
                                      )}
                                      <span className="truncate text-foreground/90 font-medium">{currentInst.account_name}</span>
                                    </>
                                  ) : (
                                    <span className="text-muted-foreground/60">{t('newProject.selectAccount')}</span>
                                  )}
                                </div>
                              </SelectTrigger>
                              <SelectContent align="start" alignItemWithTrigger={false} className="bg-popover/98 backdrop-blur-lg border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72">
                                {installations.map(inst => (
                                  <SelectItem key={inst.installation_id} value={String(inst.installation_id)} className="rounded-lg py-2.5 px-3 cursor-pointer transition-colors focus:bg-accent/80 hover:bg-accent/40">
                                    <div className="flex items-center gap-3">
                                      {inst.avatar_url ? (
                                        <img src={inst.avatar_url} alt={inst.account_name} className="w-6 h-6 rounded-full border border-border/40 shrink-0" />
                                      ) : (
                                        <GithubIcon className="w-5 h-5 text-muted-foreground shrink-0" />
                                      )}
                                      <span className="font-medium text-foreground/90 text-sm">{inst.account_name}</span>
                                    </div>
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          </div>
                        </div>

                        {/* Configure App Quick Link */}
                        <div className="flex items-end">
                          <Button
                            type="button"
                            variant="outline"
                            onClick={() => {
                              const appUrl = import.meta.env.VITE_GITHUB_APP_URL || 'https://github.com/apps/laravel-paas-local'
                              window.open(`${appUrl}/installations/new`, '_blank')
                            }}
                            className="w-full h-11 gap-2 rounded-xl"
                          >
                            <Settings className="w-4 h-4" />
                            {t('newProject.configureInstallations')}
                            <ExternalLink className="w-3.5 h-3.5" />
                          </Button>
                        </div>
                      </div>

                      {/* Repository Selector */}
                      <div className="space-y-2">
                        <Label htmlFor="repo_select" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                          {t('newProject.repository')}
                        </Label>
                        <div className="relative">
                            <Select
                              value={selectedRepoFullName}
                              onValueChange={(val) => handleRepoValueChange(val || '')}
                            >
                              <SelectTrigger className="w-full h-11 px-4 rounded-xl border border-border/60 hover:border-border dark:hover:border-foreground/20 bg-background/50 hover:bg-background/80 text-sm font-medium transition-all duration-200 outline-none focus:outline-none focus:ring-1 focus:ring-primary/20 focus:border-primary cursor-pointer flex items-center justify-between data-[size=default]:h-11 data-[size=default]:py-0 data-[size=default]:pr-4 data-[size=default]:pl-4">
                                <div className="flex items-center gap-2.5 text-left flex-1 min-w-0 pr-4">
                                  {currentRepo ? (
                                    <div className="flex items-center justify-between w-full">
                                      <div className="flex items-center gap-2.5 min-w-0">
                                        <div className="w-5 h-5 rounded-md bg-primary/10 flex items-center justify-center text-primary shrink-0 border border-primary/20">
                                          <GithubIcon className="w-3.5 h-3.5" />
                                        </div>
                                        <span className="truncate font-semibold text-foreground/90 text-sm">{currentRepo.full_name}</span>
                                      </div>
                                      <span className={cn(
                                        "text-[9px] px-2 py-0.5 rounded-full font-bold uppercase tracking-wider shrink-0 border ml-2 transition-all duration-200",
                                        currentRepo.private
                                          ? "bg-amber-500/10 text-amber-500 border-amber-500/20"
                                          : "bg-emerald-500/10 text-emerald-500 border-emerald-500/20"
                                      )}>
                                        {currentRepo.private ? t('newProject.privateBadge') : t('newProject.publicBadge')}
                                      </span>
                                    </div>
                                  ) : (
                                    <span className="text-muted-foreground/60">{t('newProject.selectRepo')}</span>
                                  )}
                                </div>
                              </SelectTrigger>
                              <SelectContent align="start" alignItemWithTrigger={false} className="bg-popover/98 backdrop-blur-lg border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72">
                                {repositories.map(repo => (
                                  <SelectItem key={repo.id} value={repo.full_name} className="rounded-lg py-2.5 px-3 cursor-pointer transition-colors focus:bg-accent/80 hover:bg-accent/40">
                                    <div className="flex items-center justify-between w-full gap-4">
                                      <div className="flex items-center gap-3 min-w-0">
                                        <div className="w-6 h-6 rounded-md bg-primary/10 flex items-center justify-center text-primary shrink-0 border border-primary/10">
                                          <GithubIcon className="w-3.75 h-3.75" />
                                        </div>
                                        <span className="font-medium text-foreground/90 text-sm truncate">{repo.full_name}</span>
                                      </div>
                                      <span className={cn(
                                        "text-[9px] px-2 py-0.5 rounded-full font-bold uppercase tracking-wider shrink-0 border",
                                        repo.private
                                          ? "bg-amber-500/10 text-amber-500 border-amber-500/20"
                                          : "bg-emerald-500/10 text-emerald-500 border-emerald-500/20"
                                      )}>
                                        {repo.private ? t('newProject.privateBadge') : t('newProject.publicBadge')}
                                      </span>
                                    </div>
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          {isGithubLoading && (
                            <div className="absolute inset-y-0 right-0 flex items-center px-10 pointer-events-none">
                              <Loader2 className="w-4 h-4 animate-spin text-primary" />
                            </div>
                          )}
                        </div>
                      </div>

                      {/* Dynamic Branch Selector */}
                      {selectedRepoFullName && (
                        <div className="space-y-2 border-t pt-4">
                          <div className="flex items-center gap-3">
                            <div className="w-6 h-6 rounded bg-muted flex items-center justify-center text-muted-foreground">
                              <GitBranch className="w-3.5 h-3.5" />
                            </div>
                            <Label htmlFor="branch_select" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                              {t('newProject.targetBranch')}
                            </Label>
                          </div>
                            <Select
                              value={formData.branch}
                              onValueChange={(val) => setFormData(prev => ({ ...prev, branch: val || '' }))}
                            >
                              <SelectTrigger className="w-full h-11 px-4 rounded-xl border border-border/60 hover:border-border dark:hover:border-foreground/20 bg-background/50 hover:bg-background/80 text-sm font-medium transition-all duration-200 outline-none focus:outline-none focus:ring-1 focus:ring-primary/20 focus:border-primary cursor-pointer flex items-center justify-between data-[size=default]:h-11 data-[size=default]:py-0 data-[size=default]:pr-4 data-[size=default]:pl-4">
                                <div className="flex items-center gap-2.5 text-left flex-1 min-w-0 pr-4">
                                  {currentBranch ? (
                                    <>
                                      <GitBranch className="w-4 h-4 text-primary shrink-0" />
                                      <span className="font-mono text-xs text-foreground/90 font-medium">{currentBranch.name}</span>
                                    </>
                                  ) : (
                                    <span className="text-muted-foreground/60">{t('newProject.selectBranch')}</span>
                                  )}
                                </div>
                              </SelectTrigger>
                              <SelectContent align="start" alignItemWithTrigger={false} className="bg-popover/98 backdrop-blur-lg border border-border/80 rounded-xl shadow-2xl p-1.5 max-h-72">
                                {branches.map(b => (
                                  <SelectItem key={b.name} value={b.name} className="rounded-lg py-2.5 px-3 cursor-pointer transition-colors focus:bg-accent/80 hover:bg-accent/40">
                                    <div className="flex items-center gap-3">
                                      <GitBranch className="w-4 h-4 text-muted-foreground shrink-0" />
                                      <span className="font-mono text-xs text-foreground/90">{b.name}</span>
                                    </div>
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          <p className="text-[10px] text-muted-foreground italic pl-1">
                            {t('newProject.branchCiCdDesc')}
                          </p>
                        </div>
                      )}
                    </div>
                  )}
                </div>
              )}

              {/* Manual URL Connection Section */}
              {connectionMode === 'manual' && (
                <div className="grid grid-cols-1 md:grid-cols-2 gap-8 border-t pt-6">
                  {/* Repository URL */}
                  <div className="space-y-4">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-lg bg-muted flex items-center justify-center text-muted-foreground">
                        <Terminal className="w-4 h-4" />
                      </div>
                      <Label htmlFor="github_url" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                        {t('newProject.repoUrl')}
                      </Label>
                    </div>
                    <Input
                      id="github_url"
                      name="github_url"
                      type="url"
                      value={formData.github_url}
                      onChange={handleChange}
                      placeholder={t('newProject.repoPlaceholder') || ''}
                      className={cn(validationErrors.github_url && "border-destructive focus-visible:ring-destructive")}
                    />
                    {validationErrors.github_url && (
                      <p className="text-xs text-destructive font-medium pl-1">{validationErrors.github_url}</p>
                    )}
                  </div>

                  {/* Manual Branch */}
                  <div className="space-y-4">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-lg bg-muted flex items-center justify-center text-muted-foreground">
                        <Settings className="w-4 h-4" />
                      </div>
                      <Label htmlFor="branch" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                        {t('newProject.branch')}
                      </Label>
                    </div>
                    <Input
                      id="branch"
                      name="branch"
                      value={formData.branch}
                      onChange={handleChange}
                      placeholder={t('newProject.branchPlaceholder')}
                    />
                  </div>
                </div>
              )}

              {/* Project Name (Only shown or activated once source is selected/populated) */}
              {(connectionMode === 'manual' || selectedRepoFullName) && (
                <div className="space-y-6 border-t pt-6 animate-in slide-in-from-bottom-2 duration-300">
                  {/* Project Name */}
                  <div className="space-y-4">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center text-primary">
                        <Rocket className="w-4 h-4" />
                      </div>
                      <Label htmlFor="name" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                        {t('newProject.displayName')}
                      </Label>
                    </div>
                    <Input
                      id="name"
                      name="name"
                      value={formData.name}
                      onChange={handleChange}
                      placeholder={t('newProject.namePlaceholder') || ''}
                      className={cn(validationErrors.name && "border-destructive focus-visible:ring-destructive")}
                    />
                    {validationErrors.name && (
                      <p className="text-xs text-destructive font-medium pl-1">{validationErrors.name}</p>
                    )}
                  </div>

                  {/* Database Settings */}
                  <div className="space-y-6 border-t pt-6">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center text-primary">
                        <Database className="w-4 h-4" />
                      </div>
                      <div className="flex flex-col">
                        <Label className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                          {t('newProject.dbConfig.title')}
                        </Label>
                        <span className="text-[10px] text-muted-foreground">{t('newProject.dbConfig.subtitle')}</span>
                      </div>
                    </div>

                    {/* Database Options Cards */}
                    <div className="space-y-4">
                      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                        {[
                          { value: 'none', key: 'none' as const, badgeClass: "border-border bg-muted/20 text-muted-foreground" },
                          { value: 'sqlite', key: 'sqlite' as const, badgeClass: "border-emerald-500/20 bg-emerald-500/10 text-emerald-500" },
                          { value: 'new', key: 'new' as const, badgeClass: "border-primary/20 bg-primary/10 text-primary" },
                        ].map((opt) => (
                          <Card
                            key={opt.value}
                            className={cn(
                              "p-4 cursor-pointer border transition-all duration-200 flex flex-col justify-between hover:shadow-md",
                              formData.database_option === opt.value
                                ? "border-primary bg-primary/5 ring-1 ring-primary/25"
                                : "border-border/80 bg-background/50"
                            )}
                            onClick={() => setFormData(prev => ({ ...prev, database_option: opt.value as any }))}
                          >
                            <div className="space-y-2">
                              <div className="flex items-center justify-between">
                                <span className="font-bold text-sm text-foreground">{t(`newProject.dbConfig.options.${opt.key}.label`)}</span>
                                <div className={cn(
                                  "w-4 h-4 rounded-full border flex items-center justify-center",
                                  formData.database_option === opt.value ? "border-primary" : "border-muted-foreground"
                                )}>
                                  {formData.database_option === opt.value && (
                                    <div className="w-2.5 h-2.5 rounded-full bg-primary" />
                                  )}
                                </div>
                              </div>
                              <p className="text-xs text-muted-foreground leading-relaxed">{t(`newProject.dbConfig.options.${opt.key}.desc`)}</p>
                            </div>
                            <div className="mt-4">
                              <span className={cn(
                                "text-[9px] font-bold px-2 py-0.5 rounded border uppercase tracking-wider",
                                opt.badgeClass
                              )}>
                                {t(`newProject.dbConfig.options.${opt.key}.badge`)}
                              </span>
                            </div>
                          </Card>
                        ))}
                      </div>

                      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                        {[
                          { value: 'existing', key: 'existing' as const, badgeClass: "border-amber-500/20 bg-amber-500/10 text-amber-500" },
                          { value: 'external', key: 'external' as const, badgeClass: "border-purple-500/20 bg-purple-500/10 text-purple-500" },
                        ].map((opt) => (
                          <Card
                            key={opt.value}
                            className={cn(
                              "p-4 cursor-pointer border transition-all duration-200 flex flex-col justify-between hover:shadow-md",
                              formData.database_option === opt.value
                                ? "border-primary bg-primary/5 ring-1 ring-primary/25"
                                : "border-border/80 bg-background/50"
                            )}
                            onClick={() => setFormData(prev => ({ ...prev, database_option: opt.value as any }))}
                          >
                            <div className="space-y-2">
                              <div className="flex items-center justify-between">
                                <span className="font-bold text-sm text-foreground">{t(`newProject.dbConfig.options.${opt.key}.label`)}</span>
                                <div className={cn(
                                  "w-4 h-4 rounded-full border flex items-center justify-center",
                                  formData.database_option === opt.value ? "border-primary" : "border-muted-foreground"
                                )}>
                                  {formData.database_option === opt.value && (
                                    <div className="w-2.5 h-2.5 rounded-full bg-primary" />
                                  )}
                                </div>
                              </div>
                              <p className="text-xs text-muted-foreground leading-relaxed">{t(`newProject.dbConfig.options.${opt.key}.desc`)}</p>
                            </div>
                            <div className="mt-4">
                              <span className={cn(
                                "text-[9px] font-bold px-2 py-0.5 rounded border uppercase tracking-wider",
                                opt.badgeClass
                              )}>
                                {t(`newProject.dbConfig.options.${opt.key}.badge`)}
                              </span>
                            </div>
                          </Card>
                        ))}
                      </div>
                    </div>

                    {/* New Managed DB Section */}
                    {formData.database_option === 'new' && (
                      <div className="p-5 rounded-xl border border-primary/20 bg-primary/5/30 backdrop-blur-sm space-y-5 animate-in fade-in slide-in-from-top-2 duration-300">
                        <Label className="text-xs font-bold uppercase tracking-widest text-primary">
                          {t('newProject.dbConfig.configureNew.title')}
                        </Label>

                        <div className="space-y-2">
                          <Label className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                            {t('newProject.dbConfig.configureNew.engine')}
                          </Label>
                          <div className="grid grid-cols-2 gap-3">
                            {databaseEngines.map((engine) => (
                              <div
                                key={engine.value}
                                onClick={() => setFormData(prev => ({ ...prev, database_engine: engine.value }))}
                                className={cn(
                                  "flex items-center gap-3 p-3 rounded-xl border cursor-pointer transition-all duration-200",
                                  formData.database_engine === engine.value
                                    ? "border-primary bg-primary/10"
                                    : "border-border bg-background/50 hover:bg-muted/30"
                                )}
                              >
                                <DatabaseEngineBadge engine={engine} />
                                <span className="text-sm font-semibold">{engine.label}</span>
                              </div>
                            ))}
                          </div>
                        </div>

                        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                          <div className="space-y-2">
                            <Label htmlFor="database_name" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                              Database Name
                            </Label>
                            <Input
                              id="database_name"
                              name="database_name"
                              value={formData.database_name}
                              onChange={handleChange}
                              placeholder="my_database"
                              className={cn("h-11 rounded-xl bg-background", validationErrors.database_name && "border-destructive focus-visible:ring-destructive")}
                            />
                            {validationErrors.database_name && (
                              <p className="text-xs text-destructive font-medium pl-1">{validationErrors.database_name}</p>
                            )}
                          </div>

                          <div className="space-y-2">
                            <Label htmlFor="database_username" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                              Username
                            </Label>
                            <Input
                              id="database_username"
                              name="database_username"
                              value={formData.database_username}
                              onChange={(e) => setFormData(prev => ({ ...prev, database_username: e.target.value }))}
                              placeholder="db_user"
                              className="h-11 rounded-xl bg-background"
                            />
                          </div>
                        </div>

                        <div className="space-y-2">
                          <Label htmlFor="database_password" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                            {t('newProject.dbConfig.configureNew.password')}
                          </Label>
                          <div className="flex gap-2">
                            <Input
                              id="database_password"
                              name="database_password"
                              value={formData.database_password}
                              onChange={(e) => setFormData(prev => ({ ...prev, database_password: e.target.value }))}
                              placeholder="Password"
                              className="h-11 rounded-xl bg-background flex-1"
                            />
                            <Button
                              type="button"
                              variant="outline"
                              onClick={() => setFormData(prev => ({ ...prev, database_password: generatePassword() }))}
                              className="h-11 px-4 rounded-xl border-border bg-background hover:bg-muted/30 font-semibold gap-1 text-xs"
                            >
                              {t('newProject.dbConfig.configureNew.generate')}
                            </Button>
                          </div>
                        </div>

                        <div className="p-3 bg-primary/10 rounded-xl border border-primary/20 flex gap-2.5 items-start">
                          <Info className="w-4 h-4 text-primary shrink-0 mt-0.5" />
                          <span className="text-[10px] text-muted-foreground leading-relaxed">
                            {t('newProject.dbConfig.configureNew.info')}
                          </span>
                        </div>
                      </div>
                    )}

                    {/* Existing Database Section */}
                    {formData.database_option === 'existing' && (
                      <div className="p-5 rounded-xl border border-amber-500/20 bg-amber-500/5/30 backdrop-blur-sm space-y-4 animate-in fade-in slide-in-from-top-2 duration-300">
                        <Label className="text-xs font-bold uppercase tracking-widest text-amber-500">
                          {t('newProject.dbConfig.existing.title')}
                        </Label>
                        {validationErrors.existing_database_uid && (
                          <p className="text-xs text-destructive font-medium pl-1">{validationErrors.existing_database_uid}</p>
                        )}

                        {isLoadingDatabases ? (
                          <div className="flex items-center justify-center p-6 gap-2 text-muted-foreground">
                            <Loader2 className="w-4 h-4 animate-spin text-amber-500" />
                            <span className="text-sm font-medium">{t('newProject.dbConfig.existing.loading')}</span>
                          </div>
                        ) : existingDatabases.length === 0 ? (
                          <div className="text-center p-6 border border-dashed rounded-xl space-y-1">
                            <p className="text-sm font-semibold text-foreground/80">{t('newProject.dbConfig.existing.emptyTitle')}</p>
                            <p className="text-xs text-muted-foreground">{t('newProject.dbConfig.existing.emptyDesc')}</p>
                          </div>
                        ) : (
                          <div className="space-y-3">
                            <Label className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                              {t('newProject.dbConfig.existing.select')}
                            </Label>
                            <div className="space-y-2">
                              {existingDatabases.map((db) => (
                                <div
                                  key={db.id}
                                  onClick={() => {
                                    setFormData(prev => ({ ...prev, existing_database_uid: db.uid }))
                                    if (validationErrors.existing_database_uid) {
                                      setValidationErrors(prev => ({ ...prev, existing_database_uid: undefined }))
                                    }
                                  }}
                                  className={cn(
                                    "flex items-center justify-between p-3.5 rounded-xl border cursor-pointer transition-all duration-200",
                                    formData.existing_database_uid === db.uid
                                      ? "border-amber-500 bg-amber-500/10"
                                      : "border-border bg-background/50 hover:bg-muted/30"
                                  )}
                                >
                                  <div className="flex items-center gap-3">
                                    <div className="w-2.5 h-2.5 rounded-full bg-emerald-500 animate-pulse" />
                                    <div>
                                      <div className="font-mono text-sm font-bold text-foreground">{db.name}</div>
                                      <div className="text-[10px] text-muted-foreground mt-0.5">Engine: {db.engine.toUpperCase()} · {t('databaseManager.unattached')}</div>
                                    </div>
                                  </div>
                                  <div className="flex items-center gap-3">
                                    <span className={cn(
                                      "text-[9px] font-bold px-2 py-0.5 rounded border uppercase tracking-wider",
                                      db.engine === 'mysql' ? "border-amber-500/20 bg-amber-500/10 text-amber-500" : "border-sky-500/20 bg-sky-500/10 text-sky-500"
                                    )}>
                                      {db.engine}
                                    </span>
                                    <div className={cn(
                                      "w-4 h-4 rounded-full border flex items-center justify-center",
                                      formData.existing_database_uid === db.uid ? "border-amber-500" : "border-muted-foreground"
                                    )}>
                                      {formData.existing_database_uid === db.uid && (
                                        <div className="w-2.5 h-2.5 rounded-full bg-amber-500" />
                                      )}
                                    </div>
                                  </div>
                                </div>
                              ))}
                            </div>
                          </div>
                        )}

                        <div className="p-3 bg-amber-500/10 rounded-xl border border-amber-500/20 flex gap-2.5 items-start">
                          <Info className="w-4 h-4 text-amber-500 shrink-0 mt-0.5" />
                          <span className="text-[10px] text-muted-foreground leading-relaxed">
                            {t('newProject.dbConfig.existing.info')}
                          </span>
                        </div>
                      </div>
                    )}

                    {/* External Database Section */}
                    {formData.database_option === 'external' && (
                      <div className="p-5 rounded-xl border border-purple-500/20 bg-purple-500/5/30 backdrop-blur-sm space-y-4 animate-in fade-in slide-in-from-top-2 duration-300">
                        <Label className="text-xs font-bold uppercase tracking-widest text-purple-500">
                          {t('newProject.dbConfig.external.title')}
                        </Label>
                        <div className="flex gap-3 items-start">
                          <ExternalLink className="w-5 h-5 text-purple-500 shrink-0 mt-0.5" />
                          <p className="text-xs text-muted-foreground leading-relaxed">
                            {t('newProject.dbConfig.external.desc')}
                          </p>
                        </div>

                        <div className="space-y-2 border-t border-purple-500/10 pt-3">
                          <Label className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground">
                            {t('newProject.dbConfig.external.vars')}
                          </Label>
                          <div className="flex flex-wrap gap-1.5">
                            {['DB_CONNECTION', 'DB_HOST', 'DB_PORT', 'DB_DATABASE', 'DB_USERNAME', 'DB_PASSWORD'].map((val) => (
                              <span key={val} className="text-[9px] font-mono font-bold px-2 py-0.5 rounded border bg-muted/30 border-border text-muted-foreground uppercase tracking-wider">
                                {val}
                              </span>
                            ))}
                          </div>
                        </div>
                      </div>
                    )}
                  </div>

                  {/* Base Directory */}
                  <div className="space-y-4">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-lg bg-muted flex items-center justify-center text-muted-foreground">
                        <Layout className="w-4 h-4" />
                      </div>
                      <Label htmlFor="base_directory" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                        {t('newProject.baseDir')}
                      </Label>
                    </div>
                    <Input
                      id="base_directory"
                      name="base_directory"
                      value={formData.base_directory}
                      onChange={handleChange}
                      placeholder={t('newProject.baseDirPlaceholder')}
                    />
                    <p className="text-[10px] text-muted-foreground italic pl-1 uppercase tracking-wider">{t('newProject.baseDirDesc')}</p>
                  </div>

                  {/* Custom Commands */}
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-8 border-t pt-6">
                    <div className="space-y-4">
                      <div className="flex items-center gap-3">
                        <div className="w-8 h-8 rounded-lg bg-primary/5 flex items-center justify-center text-primary/70">
                          <Terminal className="w-4 h-4" />
                        </div>
                        <Label htmlFor="build_command" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                          {t('projectDetail.settings.buildCommand')}
                        </Label>
                      </div>
                      <Input
                        id="build_command"
                        name="build_command"
                        value={formData.build_command}
                        onChange={handleChange}
                        placeholder="e.g. npm run build"
                        className="font-mono text-xs"
                      />
                      <p className="text-[10px] text-muted-foreground italic pl-1">{t('projectDetail.settings.buildCommandDesc')}</p>
                    </div>

                    <div className="space-y-4">
                      <div className="flex items-center gap-3">
                        <div className="w-8 h-8 rounded-lg bg-primary/5 flex items-center justify-center text-primary/70">
                          <Play className="w-4 h-4" />
                        </div>
                        <Label htmlFor="start_command" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                          {t('projectDetail.settings.startCommand')}
                        </Label>
                      </div>
                      <Input
                        id="start_command"
                        name="start_command"
                        value={formData.start_command}
                        onChange={handleChange}
                        placeholder="e.g. php artisan serve"
                        className="font-mono text-xs"
                      />
                      <p className="text-[10px] text-muted-foreground italic pl-1">{t('projectDetail.settings.startCommandDesc')}</p>
                    </div>

                    <div className="space-y-4">
                      <div className="flex items-center gap-3">
                        <div className="w-8 h-8 rounded-lg bg-primary/5 flex items-center justify-center text-primary/70">
                          <Play className="w-4 h-4" />
                        </div>
                        <Label htmlFor="port" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                          {t('projectDetail.settings.internalPort')}
                        </Label>
                      </div>
                      <Input
                        id="port"
                        name="port"
                        type="number"
                        value={formData.port}
                        onChange={(e) => setFormData(prev => ({ ...prev, port: e.target.value ? parseInt(e.target.value) : '' }))}
                        placeholder="e.g. 5005"
                        className="font-mono text-xs"
                      />
                      <p className="text-[10px] text-muted-foreground italic pl-1">{t('projectDetail.settings.internalPortDesc')}</p>
                    </div>
                  </div>
                </div>
              )}
            </div>
          </Card>

          {/* Submit Button (Only shown/enabled once source is selected/populated) */}
          {(connectionMode === 'manual' || selectedRepoFullName) && (
            <Button
              type="submit"
              disabled={isLoading || (formData.database_option === 'existing' && isLoadingDatabases)}
              className="w-full h-16 text-lg font-bold gap-3"
            >
              {isLoading ? (
                <>
                  <Loader2 className="w-6 h-6 animate-spin" />
                  {t('newProject.initializing')}
                </>
              ) : (
                <>
                  <Rocket className="w-6 h-6" />
                  {t('newProject.initialize')}
                  <ArrowRight className="w-6 h-6" />
                </>
              )}
            </Button>
          )}
        </div>
      </form>
    </div>
  )
}

export default UserNewProject
