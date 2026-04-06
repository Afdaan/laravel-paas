import React, { useState, useEffect, useCallback } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { toast } from 'sonner'
import { projectsAPI } from '../../services/api'
import { 
  Plus, 
  Rocket, 
  ExternalLink, 
  RefreshCw, 
  Trash2, 
  Clock, 
  CheckCircle2, 
  
  AlertCircle, 
  PauseCircle,
  Database,
  Globe,
  Cpu,
  ArrowRight,
  Loader2
} from 'lucide-react'
import ConfirmationModal from '../../components/ConfirmationModal'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

interface ProjectData {
  id: number;
  name: string;
  status: string;
  subdomain: string;
  url: string;
  created_at: string;
  php_version: string;
  laravel_version: string;
  database_name: string;
  branch: string;
}

const StatusBadge = ({ status }: { status: string }) => {
  const configs: Record<string, any> = {
    pending: { color: 'text-amber-600 border-amber-500/20 bg-amber-500/10', icon: Clock, label: 'Queued' },
    building: { color: 'text-blue-600 border-blue-500/20 bg-blue-500/10', icon: Loader2, label: 'Building', pulse: true },
    running: { color: 'text-emerald-600 border-emerald-500/20 bg-emerald-500/10', icon: CheckCircle2, label: 'Running' },
    failed: { color: 'text-rose-600 border-rose-500/20 bg-rose-500/10', icon: AlertCircle, label: 'Failed' },
    stopped: { color: 'text-slate-600 border-slate-500/20 bg-slate-500/10 dark:text-slate-400', icon: PauseCircle, label: 'Offline' },
  }

  const config = configs[status] || configs.pending
  const Icon = config.icon

  return (
    <Badge variant="outline" className={`gap-1.5 flex w-fit ${config.color}`}>
      <Icon className={`w-3 h-3 ${config.pulse ? 'animate-spin' : ''}`} />
      {config.label}
    </Badge>
  )
}

const StudentProjects = () => {
  const navigate = useNavigate()
  const [projects, setProjects] = useState<ProjectData[]>([])
  const [isLoading, setIsLoading] = useState(true)
  
  const [confirmModal, setConfirmModal] = useState({
    isOpen: false,
    title: '',
    message: '',
    type: 'danger' as 'danger' | 'warning' | 'info',
    onConfirm: () => {},
    confirmText: 'Confirm'
  })

  const fetchProjects = useCallback(async () => {
    setIsLoading(true)
    try {
      const response = await projectsAPI.listOwn()
      setProjects(response.data.data || [])
    } catch (error) {
      toast.error('Failed to load projects')
    } finally {
      setIsLoading(false)
    }
  }, [])
  
  useEffect(() => {
    fetchProjects()
  }, [fetchProjects])
  
  const handleRedeploy = async (id: number, e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    
    setConfirmModal({
      isOpen: true,
      title: 'Redeploy Project?',
      message: 'This will rebuild your project. It may be temporarily unavailable during the process.',
      type: 'warning',
      confirmText: 'Redeploy',
      onConfirm: () => {
        toast.promise(
          projectsAPI.redeploy(id),
          {
            loading: 'Redeploying project...',
            success: 'Redeploy started',
            error: 'Failed to redeploy'
          }
        )
        fetchProjects()
      }
    })
  }
  
  const handleDelete = async (id: number, e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    
    setConfirmModal({
      isOpen: true,
      title: 'Delete Project?',
      message: 'This will permanently delete this project and all its data. This action cannot be undone.',
      type: 'danger',
      confirmText: 'Delete Project',
      onConfirm: async () => {
        try {
          await projectsAPI.delete(id)
          toast.success('Project Deleted')
          fetchProjects()
        } catch (error) {
          toast.error('Delete Failed')
        }
      }
    })
  }
  
  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      <ConfirmationModal 
        onClose={() => setConfirmModal(prev => ({ ...prev, isOpen: false }))}
        {...confirmModal}
      />

      {/* Header Container */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-6 pb-4 border-b">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">Projects</h1>
          <p className="text-muted-foreground max-w-2xl">
            Manage and monitor all your projects in our modern dashboard interface.
          </p>
        </div>
        <Link to="/projects/new" className={cn(buttonVariants({ variant: "default", size: "lg" }), "w-full md:w-auto font-semibold")}>
          <Plus className="w-5 h-5 mr-2" />
          New Project
        </Link>
      </div>
      
      {/* Grid Architecture */}
      {isLoading ? (
        <div className="flex flex-col items-center justify-center py-40 gap-6 opacity-80">
          <Loader2 className="w-12 h-12 text-primary animate-spin" />
          <p className="text-xs font-bold uppercase tracking-widest text-muted-foreground animate-pulse">Loading Projects...</p>
        </div>
      ) : (!projects || projects.length === 0) ? (
        <Card className="p-24 text-center flex flex-col items-center max-w-xl mx-auto border-dashed">
          <div className="w-20 h-20 bg-muted rounded-full flex items-center justify-center mb-6">
            <Rocket className="w-10 h-10 text-muted-foreground opacity-50" />
          </div>
          <h2 className="text-2xl font-bold tracking-tight mb-2">The list is empty.</h2>
          <p className="text-muted-foreground mb-8 max-w-sm">You have no active projects. Create your first project to begin monitoring.</p>
          <Link to="/projects/new" className={cn(buttonVariants({ variant: "default", size: "lg" }), "w-full sm:w-auto")}>
            Create Project
          </Link>
        </Card>
      ) : (
        <div className="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 gap-6 pb-12">
          {projects.map((project) => (
            <Card 
              key={project.id} 
              onClick={() => navigate(`/projects/${project.id}`)}
              className="group flex flex-col h-full hover:border-primary/30 transition-all cursor-pointer overflow-hidden border-border/50"
            >
              <CardContent className="p-6 flex flex-col h-full">
                <div className="flex items-start justify-between mb-8">
                  <div className="w-12 h-12 rounded-lg bg-primary/10 border border-primary/20 flex items-center justify-center text-primary group-hover:bg-primary group-hover:text-primary-foreground transition-all">
                     <span className="text-xl font-bold uppercase">{project.name.charAt(0)}</span>
                  </div>
                  <StatusBadge status={project.status} />
                </div>

                <div className="mb-6 flex-1">
                  <h3 className="font-bold text-xl tracking-tight mb-3 truncate" title={project.name}>
                    {project.name}
                  </h3>
                  <a 
                    href={project.url}
                    target="_blank"
                    rel="noopener noreferrer"
                    onClick={(e) => e.stopPropagation()}
                    className="flex items-center gap-2 text-muted-foreground font-mono text-xs hover:text-primary transition-colors bg-muted/50 px-2 py-1.5 rounded-md hover:bg-muted w-fit max-w-full"
                  >
                    <Globe className="w-3.5 h-3.5 shrink-0" />
                    <span className="truncate">{project.url ? project.url.replace(/^https?:\/\//, '') : project.subdomain}</span>
                    <ExternalLink className="w-3 h-3 shrink-0" />
                  </a>
                </div>

                <div className="space-y-4 py-6 border-y border-border/50 bg-muted/10 -mx-6 px-6">
                  <div className="flex items-center justify-between text-sm">
                    <div className="flex items-center gap-2 text-muted-foreground">
                      <Cpu className="w-4 h-4" />
                      <span>Environment</span>
                    </div>
                    <Badge variant="secondary" className="font-mono text-[10px]">
                      PHP {project.php_version || '8.2'}
                    </Badge>
                  </div>
                  <div className="flex items-center justify-between text-sm">
                    <div className="flex items-center gap-2 text-muted-foreground">
                      <Database className="w-4 h-4" />
                      <span>Database</span>
                    </div>
                    <span className="text-xs font-semibold text-primary">
                      {project.database_name ? 'Active' : 'No DB'}
                    </span>
                  </div>
                </div>

                <div className="mt-6 flex items-center justify-between">
                   <div className="flex flex-col gap-1">
                      <span className="text-[10px] text-muted-foreground uppercase tracking-widest font-semibold">Created</span>
                      <span className="text-xs font-medium">{new Date(project.created_at).toLocaleDateString()}</span>
                   </div>
                   
                    <div className="flex gap-2">
                      <Button
                        variant="outline"
                        size="icon"
                        onClick={(e) => handleRedeploy(project.id, e)}
                        className="h-9 w-9 text-muted-foreground hover:text-foreground"
                        title="Init Redeploy"
                      >
                         <RefreshCw className="w-4 h-4" />
                      </Button>
                      <Button
                        variant="outline"
                        size="icon"
                        onClick={(e) => handleDelete(project.id, e)}
                        className="h-9 w-9 text-muted-foreground hover:text-destructive hover:bg-destructive/10 hover:border-destructive/30"
                        title="Decommission"
                      >
                         <Trash2 className="w-4 h-4" />
                      </Button>
                      <Button size="icon" className="h-9 w-9">
                         <ArrowRight className="w-4 h-4" />
                      </Button>
                   </div>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  )
}

export default StudentProjects

