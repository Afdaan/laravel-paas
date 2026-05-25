import React, { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { projectsAPI, githubAPI } from '../../services/api'
import { AxiosError } from 'axios'
import useTranslation from '../../lib/useTranslation'
import { GithubAppInstallation, GithubRepository } from '../../types'
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
  Github,
  GitBranch,
  ExternalLink,
  Lock,
  Globe,
  RefreshCw
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'

interface NewProjectForm {
  name: string;
  github_url: string;
  branch: string;
  database_name: string;
  base_directory: string;
  build_command: string;
  start_command: string;
  queue_enabled: boolean;
  github_installation_id?: number;
  github_repo_owner?: string;
  github_repo_name?: string;
}

interface ValidationErrors {
  name?: string;
  github_url?: string;
  database_name?: string;
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
    base_directory: '',
    build_command: '',
    start_command: '',
    queue_enabled: false,
  })
  const [validationErrors, setValidationErrors] = useState<ValidationErrors>({})

  const loadRepositories = async (installationId: string) => {
    setIsGithubLoading(true)
    try {
      const response = await githubAPI.listRepositories(installationId)
      setRepositories(response.data.data || [])
    } catch (err) {
      console.error(err)
      toast.error('Failed to load repositories')
    } finally {
      setIsGithubLoading(false)
    }
  }

  const loadInstallations = async () => {
    setIsGithubLoading(true)
    try {
      const response = await githubAPI.listInstallations()
      const insts = response.data.data || []
      setInstallations(insts)
      if (insts.length > 0) {
        const firstInstId = String(insts[0].installation_id)
        setSelectedInstallationId(firstInstId)
        loadRepositories(firstInstId)
      }
    } catch (err) {
      console.error(err)
    } finally {
      setIsGithubLoading(false)
    }
  }

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const installationId = params.get('installation_id')
    if (installationId) {
      const linkGithub = async () => {
        setIsGithubLoading(true)
        try {
          await githubAPI.linkInstallation(installationId)
          toast.success(t('newProject.githubConnected') || 'GitHub App connected successfully')
          const cleanUrl = window.location.pathname
          window.history.replaceState({}, document.title, cleanUrl)
          await loadInstallations()
        } catch (err) {
          console.error(err)
          toast.error('Failed to link GitHub installation')
        } finally {
          setIsGithubLoading(false)
        }
      }
      linkGithub()
    } else {
      loadInstallations()
    }
  }, [])

  const loadBranches = async (owner: string, repo: string, currentReposList = repositories) => {
    setIsGithubLoading(true)
    try {
      const response = await githubAPI.listBranches(owner, repo)
      setBranches(response.data.data || [])
      const repoDetails = currentReposList.find(r => r.full_name === `${owner}/${repo}`)
      const defaultBranch = repoDetails?.default_branch || 'main'
      setFormData(prev => ({ ...prev, branch: defaultBranch }))
    } catch (err) {
      console.error(err)
      toast.error('Failed to load branches')
    } finally {
      setIsGithubLoading(false)
    }
  }

  const handleRepoChange = (e: React.ChangeEvent<HTMLSelectElement>) => {
    const fullName = e.target.value
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

      setFormData(prev => ({
        ...prev,
        name: pName,
        github_url: selectedRepo.html_url,
        database_name: dbName,
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
      setFormData(prev => ({ ...prev, database_name: dbName }))
    }

    if (validationErrors[name as keyof ValidationErrors]) {
      setValidationErrors(prev => ({ ...prev, [name]: undefined }))
    }
  }

  const validateForm = () => {
    const errors: ValidationErrors = {}
    if (!formData.name.trim()) errors.name = t('common.validation.required', { field: t('newProject.displayName') })
    if (!formData.github_url.trim()) errors.github_url = t('common.validation.required', { field: t('newProject.repoUrl') })
    if (!formData.database_name.trim()) errors.database_name = t('common.validation.required', { field: t('newProject.dbName') })

    setValidationErrors(errors)
    return Object.keys(errors).length === 0
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()

    if (!validateForm()) return

    setIsLoading(true)
    setSubmitError(null)

    try {
      const response = await projectsAPI.create(formData)
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
                    <Github className="w-4 h-4" />
                  </div>
                  <Label className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                    Git Source
                  </Label>
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <button
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
                      "flex items-center justify-center gap-3 p-4 rounded-xl border text-sm font-semibold transition-all duration-200",
                      connectionMode === 'app'
                        ? "border-primary bg-primary/5 text-primary"
                        : "border-border hover:border-muted-foreground/30 hover:bg-muted/10 text-muted-foreground"
                    )}
                  >
                    <Github className="w-5 h-5" />
                    GitHub App
                  </button>
                  <button
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
                      "flex items-center justify-center gap-3 p-4 rounded-xl border text-sm font-semibold transition-all duration-200",
                      connectionMode === 'manual'
                        ? "border-primary bg-primary/5 text-primary"
                        : "border-border hover:border-muted-foreground/30 hover:bg-muted/10 text-muted-foreground"
                    )}
                  >
                    <Terminal className="w-5 h-5" />
                    Manual Git URL
                  </button>
                </div>
              </div>

              {/* GitHub App Connection Section */}
              {connectionMode === 'app' && (
                <div className="space-y-6 border-t pt-6">
                  {isGithubLoading && installations.length === 0 ? (
                    <div className="flex flex-col items-center justify-center py-12 gap-3 text-muted-foreground">
                      <Loader2 className="w-8 h-8 animate-spin text-primary" />
                      <p className="text-sm">Accessing GitHub configurations...</p>
                    </div>
                  ) : installations.length === 0 ? (
                    <div className="border border-dashed border-border rounded-xl p-8 text-center space-y-4 bg-muted/5">
                      <div className="w-12 h-12 rounded-xl bg-muted flex items-center justify-center mx-auto text-muted-foreground">
                        <Github className="w-6 h-6" />
                      </div>
                      <div className="space-y-1">
                        <h4 className="font-bold text-base">Connect your GitHub Account</h4>
                        <p className="text-sm text-muted-foreground max-w-md mx-auto">
                          Install our GitHub App to easily import your repositories and enable automatic deployments on git push.
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
                        <Github className="w-4 h-4" />
                        Configure GitHub App
                        <ExternalLink className="w-3.5 h-3.5" />
                      </Button>
                    </div>
                  ) : (
                    <div className="space-y-6">
                      <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                        {/* GitHub Account Selector */}
                        <div className="space-y-2">
                          <Label htmlFor="installation_id" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                            GitHub Account
                          </Label>
                          <div className="relative">
                            <select
                              id="installation_id"
                              value={selectedInstallationId}
                              onChange={(e) => {
                                setSelectedInstallationId(e.target.value)
                                loadRepositories(e.target.value)
                              }}
                              className="w-full h-11 px-4 rounded-xl border border-border bg-background text-sm font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-primary focus:border-primary appearance-none cursor-pointer"
                            >
                              {installations.map(inst => (
                                <option key={inst.installation_id} value={inst.installation_id}>
                                  {inst.account_name}
                                </option>
                              ))}
                            </select>
                            <div className="pointer-events-none absolute inset-y-0 right-0 flex items-center px-4 text-muted-foreground">
                              <RefreshCw className="w-4 h-4 animate-pulse" />
                            </div>
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
                            Configure Installations
                            <ExternalLink className="w-3.5 h-3.5" />
                          </Button>
                        </div>
                      </div>

                      {/* Repository Selector */}
                      <div className="space-y-2">
                        <Label htmlFor="repo_select" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                          Repository
                        </Label>
                        <div className="relative">
                          <select
                            id="repo_select"
                            value={selectedRepoFullName}
                            onChange={handleRepoChange}
                            className="w-full h-11 px-4 rounded-xl border border-border bg-background text-sm font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-primary focus:border-primary appearance-none cursor-pointer"
                          >
                            <option value="">Select a repository to import</option>
                            {repositories.map(repo => (
                              <option key={repo.id} value={repo.full_name}>
                                {repo.full_name} {repo.private ? '(Private)' : '(Public)'}
                              </option>
                            ))}
                          </select>
                          {isGithubLoading && (
                            <div className="absolute inset-y-0 right-0 flex items-center px-4">
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
                              Target Branch
                            </Label>
                          </div>
                          <select
                            id="branch_select"
                            value={formData.branch}
                            onChange={(e) => setFormData(prev => ({ ...prev, branch: e.target.value }))}
                            className="w-full h-11 px-4 rounded-xl border border-border bg-background text-sm font-medium transition-colors focus:outline-none focus:ring-2 focus:ring-primary focus:border-primary cursor-pointer"
                          >
                            <option value="">Select a branch</option>
                            {branches.map(b => (
                              <option key={b.name} value={b.name}>
                                {b.name}
                              </option>
                            ))}
                          </select>
                          <p className="text-[10px] text-muted-foreground italic pl-1">
                            Commits pushed to this branch will trigger automated CI/CD deployments.
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
                  <div className="space-y-4">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center text-primary">
                        <Database className="w-4 h-4" />
                      </div>
                      <Label htmlFor="database_name" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                        {t('newProject.dbName')}
                      </Label>
                    </div>
                    <Input
                      id="database_name"
                      name="database_name"
                      value={formData.database_name}
                      onChange={handleChange}
                      placeholder={t('newProject.dbName')}
                      className={cn(validationErrors.database_name && "border-destructive focus-visible:ring-destructive")}
                    />
                    <p className="text-[10px] text-muted-foreground italic pl-1 uppercase tracking-wider">{t('newProject.dbAutoDesc')}</p>
                    {validationErrors.database_name && (
                      <p className="text-xs text-destructive font-medium pl-1">{validationErrors.database_name}</p>
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
                  </div>
                </div>
              )}
            </div>
          </Card>

          {/* Submit Button (Only shown/enabled once source is selected/populated) */}
          {(connectionMode === 'manual' || selectedRepoFullName) && (
            <Button
              type="submit"
              disabled={isLoading}
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
