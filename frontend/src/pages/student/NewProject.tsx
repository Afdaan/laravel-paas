import React, { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { projectsAPI } from '../../services/api'
import {
  Rocket,
  Database,
  Settings,
  Activity,
  Info,
  ChevronLeft,
  ArrowRight,
  ShieldCheck,
  Zap,
  Cpu,
  Loader2
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'

interface NewProjectForm {
  name: string;
  github_url: string;
  branch: string;
  database_name: string;
  queue_enabled: boolean;
}

interface ValidationErrors {
  name?: string;
  github_url?: string;
  database_name?: string;
}

function StudentNewProject() {
  const navigate = useNavigate()
  const [isLoading, setIsLoading] = useState(false)
  const [submitError, setSubmitError] = useState<string | null>(null)
  const [formData, setFormData] = useState<NewProjectForm>({
    name: '',
    github_url: '',
    branch: '',
    database_name: '',
    queue_enabled: false,
  })
  const [validationErrors, setValidationErrors] = useState<ValidationErrors>({})

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

  const handleSwitchChange = (checked: boolean) => {
    setFormData(prev => ({ ...prev, queue_enabled: checked }))
  }

  const validateForm = () => {
    const errors: ValidationErrors = {}
    if (!formData.name.trim()) errors.name = 'Project name is required'
    if (!formData.github_url.trim()) errors.github_url = 'Repository URL is required'
    if (!formData.database_name.trim()) errors.database_name = 'Database name is required'

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
      toast.success('Project Created Successfully')
      navigate(`/projects/${response.data.project.id}`)
    } catch (error: any) {
      let errorMsg = error.response?.data?.error || 'Failed to create project'

      if (errorMsg === 'Project limit reached' || error.response?.status === 403) {
        errorMsg = 'Project limit reached. Please delete an existing project before creating a new one.'
      }

      setSubmitError(errorMsg)
      toast.error(errorMsg)
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div className="max-w-5xl mx-auto space-y-10 pb-20 animate-in fade-in duration-500">
      <div className="flex flex-col gap-4">
        <Button
          variant="outline"
          size="sm"
          onClick={() => navigate(-1)}
          className="w-fit gap-2"
        >
          <ChevronLeft className="w-4 h-4" />
          Back to Projects
        </Button>

        <div className="space-y-2">
          <h1 className="text-4xl font-bold tracking-tight">New <span className="text-primary italic">Project</span></h1>
          <p className="text-muted-foreground text-lg">Scale your Laravel application in seconds with automated cloud deployment.</p>
        </div>
      </div>

      {submitError && (
        <Card className="border-destructive/20 bg-destructive/5 p-6 animate-in zoom-in-95">
          <div className="flex flex-col items-center justify-center text-center gap-3">
            <div className="w-12 h-12 bg-destructive/10 rounded-full flex items-center justify-center text-destructive">
              <Info className="w-6 h-6" />
            </div>
            <h3 className="text-lg font-bold uppercase tracking-tight">Deployment Restricted</h3>
            <p className="text-sm font-medium text-destructive/80 max-w-lg">{submitError}</p>
          </div>
        </Card>
      )}

      <form onSubmit={handleSubmit} className="grid grid-cols-1 lg:grid-cols-3 gap-8">
        <div className="lg:col-span-2 space-y-8">
          <Card className="p-8">
            <div className="space-y-8">
              {/* Project Name */}
              <div className="space-y-4">
                <div className="flex items-center gap-3">
                  <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center text-primary">
                    <Rocket className="w-4 h-4" />
                  </div>
                  <Label htmlFor="name" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                    Project Display Name
                  </Label>
                </div>
                <Input
                  id="name"
                  name="name"
                  value={formData.name}
                  onChange={handleChange}
                  placeholder="e.g. Stellar Marketing API"
                  className={cn(validationErrors.name && "border-destructive focus-visible:ring-destructive")}
                />
                {validationErrors.name && (
                  <p className="text-xs text-destructive font-medium pl-1">{validationErrors.name}</p>
                )}
              </div>

              {/* GitHub Settings */}
              <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
                <div className="space-y-4">
                  <div className="flex items-center gap-3">
                    <div className="w-8 h-8 rounded-lg bg-muted flex items-center justify-center text-muted-foreground">
                      <Rocket className="w-4 h-4" />
                    </div>
                    <Label htmlFor="github_url" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                      Repository URL
                    </Label>
                  </div>
                  <Input
                    id="github_url"
                    name="github_url"
                    type="url"
                    value={formData.github_url}
                    onChange={handleChange}
                    placeholder="https://github.com/org/repo"
                    className={cn(validationErrors.github_url && "border-destructive focus-visible:ring-destructive")}
                  />
                  {validationErrors.github_url && (
                    <p className="text-xs text-destructive font-medium pl-1">{validationErrors.github_url}</p>
                  )}
                </div>

                <div className="space-y-4">
                  <div className="flex items-center gap-3">
                    <div className="w-8 h-8 rounded-lg bg-muted flex items-center justify-center text-muted-foreground">
                      <Settings className="w-4 h-4" />
                    </div>
                    <Label htmlFor="branch" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                      Git Branch
                    </Label>
                  </div>
                  <Input
                    id="branch"
                    name="branch"
                    value={formData.branch}
                    onChange={handleChange}
                    placeholder="main"
                  />
                </div>
              </div>

              {/* Database Settings */}
              <div className="space-y-4">
                <div className="flex items-center gap-3">
                  <div className="w-8 h-8 rounded-lg bg-primary/10 flex items-center justify-center text-primary">
                    <Database className="w-4 h-4" />
                  </div>
                  <Label htmlFor="database_name" className="text-xs font-bold uppercase tracking-widest text-muted-foreground">
                    MySQL Database Name
                  </Label>
                </div>
                <Input
                  id="database_name"
                  name="database_name"
                  value={formData.database_name}
                  onChange={handleChange}
                  placeholder="database_name"
                  className={cn(validationErrors.database_name && "border-destructive focus-visible:ring-destructive")}
                />
                <p className="text-[10px] text-muted-foreground italic pl-1 uppercase tracking-wider">Generated automatically from project name.</p>
                {validationErrors.database_name && (
                  <p className="text-xs text-destructive font-medium pl-1">{validationErrors.database_name}</p>
                )}
              </div>

              {/* Queue Worker */}
              <div className={cn(
                "p-6 rounded-xl border transition-all duration-300 flex items-center justify-between",
                formData.queue_enabled ? "bg-primary/5 border-primary/30" : "bg-muted/50"
              )}>
                <div className="space-y-1">
                  <Label htmlFor="queue_enabled" className="text-sm font-bold uppercase tracking-widest cursor-pointer">Enable Queue Worker</Label>
                  <p className="text-muted-foreground text-xs font-medium italic">Enables background 'php artisan queue:work' process.</p>
                </div>
                <Switch
                  id="queue_enabled"
                  checked={formData.queue_enabled}
                  onCheckedChange={handleSwitchChange}
                />
              </div>
            </div>
          </Card>

          <Button
            type="submit"
            disabled={isLoading}
            className="w-full h-16 text-lg font-bold gap-3"
          >
            {isLoading ? (
              <>
                <Loader2 className="w-6 h-6 animate-spin" />
                Initializing Environment...
              </>
            ) : (
              <>
                <Rocket className="w-6 h-6" />
                Initialize Deployment
                <ArrowRight className="w-6 h-6" />
              </>
            )}
          </Button>
        </div>

        <div className="space-y-6">
          <Card className="p-8 space-y-8 bg-muted/20">
            <div className="flex items-center gap-3 pb-6 border-b">
              <ShieldCheck className="w-6 h-6 text-primary" />
              <h3 className="text-sm font-bold uppercase tracking-[0.2em]">Deployment Pipeline</h3>
            </div>

            <ul className="space-y-6">
              <PipelineStep
                icon={Rocket}
                title="Git Integration"
                desc="Automated source code isolation and cloning."
              />
              <PipelineStep
                icon={Activity}
                title="Runtime Detection"
                desc="PHP and Laravel version analysis."
              />
              <PipelineStep
                icon={Database}
                title="Resource Provisioning"
                desc="Dedicated database and schema creation."
              />
              <PipelineStep
                icon={Zap}
                title="Network Mesh"
                desc="SSL termination and subdomain routing."
              />
              <PipelineStep
                icon={Cpu}
                title="Compute Allocation"
                desc="Isolated container resource mapping."
              />
            </ul>

            <div className="p-4 rounded-lg bg-background border flex items-start gap-3">
              <Info className="w-4 h-4 text-primary mt-0.5" />
              <p className="text-[10px] text-muted-foreground font-bold uppercase tracking-wider leading-relaxed">
                Your application will be served over encrypted TLS (HTTPS) on our global edge network.
              </p>
            </div>
          </Card>
        </div>
      </form>
    </div>
  )
}

function PipelineStep({ icon: Icon, title, desc }: { icon: any, title: string, desc: string }) {
  return (
    <li className="flex gap-4 group">
      <div className="w-10 h-10 rounded-xl bg-background border flex items-center justify-center text-muted-foreground group-hover:text-primary transition-colors shrink-0">
        <Icon className="w-5 h-5" />
      </div>
      <div>
        <h4 className="text-xs font-bold uppercase tracking-widest mb-1">{title}</h4>
        <p className="text-[11px] text-muted-foreground font-medium leading-normal">{desc}</p>
      </div>
    </li>
  )
}

export default StudentNewProject
