import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { projectsAPI } from '../../services/api'
import useAuthStore from '../../stores/authStore'
import {
  Rocket,
  Activity,
  CheckCircle2,
  Package,
  ExternalLink,
  ArrowRight,
  Plus,
  Clock,
  AlertCircle,
  PauseCircle,
  Zap,
  Layout,
  Terminal,
  ChevronRight,
  Loader2
} from 'lucide-react'
import { buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

interface ProjectData {
  id: number;
  name: string;
  status: string;
  subdomain: string;
  created_at: string;
}

// Status badge component
function StatusBadge({ status }: { status: string }) {
  const configs: Record<string, any> = {
    pending: { color: 'text-amber-600 bg-amber-500/10 border-amber-500/20', icon: Clock, label: 'In Queue' },
    building: { color: 'text-blue-600 bg-blue-500/10 border-blue-500/20', icon: Loader2, label: 'Building', pulse: true },
    running: { color: 'text-emerald-600 bg-emerald-500/10 border-emerald-500/20', icon: CheckCircle2, label: 'Running' },
    failed: { color: 'text-rose-600 bg-rose-500/10 border-rose-500/20', icon: AlertCircle, label: 'Failed' },
    stopped: { color: 'text-slate-600 bg-slate-500/10 border-slate-500/20 dark:text-slate-400', icon: PauseCircle, label: 'Stopped' },
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

function StudentDashboard() {
  const { user } = useAuthStore()
  const [projects, setProjects] = useState<ProjectData[]>([])
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    fetchProjects()
  }, [])

  const fetchProjects = async () => {
    try {
      const response = await projectsAPI.listOwn()
      setProjects(response.data.data || [])
    } catch (error) {
      console.error('Failed to fetch projects:', error)
    } finally {
      setIsLoading(false)
    }
  }

  const runningProjects = projects.filter(p => p.status === 'running').length
  const totalProjects = projects.length

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      {/* Welcome Header */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-6 pb-4 border-b">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">Dashboard</h1>
          <p className="text-muted-foreground">
            Welcome back, <span className="text-foreground font-bold">{user?.name?.split(' ')[0] || 'Student'}</span>. You currently have <span className="text-primary font-bold">{runningProjects} active projects</span>.
          </p>
        </div>
        <Link to="/projects/new" className={cn(buttonVariants({ variant: "default" }))}>
          <Plus className="w-4 h-4 mr-2" />
          New Project
        </Link>
      </div>

      {/* Core Stats */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
        <StatCard
          label="Total Projects"
          value={totalProjects}
          icon={Package}
        />
        <StatCard
          label="Running Projects"
          value={runningProjects}
          icon={Activity}
          suffix={`/ ${totalProjects}`}
        />
        <Card className="bg-primary/5 hover:bg-primary/10 transition-colors border-primary/20 group relative overflow-hidden">
          <CardContent className="p-6 flex flex-col justify-between h-full relative z-10">
            <div>
              <Zap className="w-8 h-8 text-primary mb-4" />
              <h3 className="text-lg font-bold tracking-tight mb-1">Need Help?</h3>
              <p className="text-muted-foreground text-sm">Get Technical support</p>
            </div>
            <Link to="/feedback" className="flex items-center gap-2 text-foreground font-semibold text-sm hover:text-primary transition-colors mt-6 group-hover:gap-3">
              Support Ticket <ArrowRight className="w-4 h-4" />
            </Link>
          </CardContent>
          <div className="absolute right-0 bottom-0 pointer-events-none opacity-10 transform translate-x-1/4 translate-y-1/4 transition-transform group-hover:scale-110">
            <Zap className="w-48 h-48" />
          </div>
        </Card>
      </div>

      {/* Recent Activity Table */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between pb-6">
          <div className="flex items-center gap-4">
            <div className="w-10 h-10 rounded-lg bg-muted border flex items-center justify-center">
              <Layout className="w-5 h-5 text-muted-foreground" />
            </div>
            <div>
              <CardTitle className="text-xl">Recent Projects</CardTitle>
              <CardDescription>Latest activity</CardDescription>
            </div>
          </div>
          <Link to="/projects" className={cn(buttonVariants({ variant: "ghost", size: "sm" }), "hidden sm:flex gap-1")}>
            Browse All <ChevronRight className="w-4 h-4" />
          </Link>
        </CardHeader>

        {isLoading ? (
          <div className="p-24 flex flex-col items-center justify-center gap-6">
            <Loader2 className="w-10 h-10 text-primary animate-spin" />
            <p className="text-muted-foreground text-sm font-semibold uppercase tracking-widest animate-pulse">Loading Projects...</p>
          </div>
        ) : projects.length === 0 ? (
          <div className="p-24 text-center flex flex-col items-center max-w-sm mx-auto">
            <div className="w-20 h-20 bg-muted border rounded-full flex items-center justify-center mb-6">
              <Rocket className="w-10 h-10 text-muted-foreground opacity-50" />
            </div>
            <h4 className="text-xl font-bold tracking-tight mb-2">No Projects Found</h4>
            <p className="text-muted-foreground text-sm mb-8">You have no active projects yet. Create your first project to get started.</p>
            <Link to="/projects/new" className={cn(buttonVariants({ variant: "default" }), "w-full")}>
              Create Your First Project
            </Link>
          </div>
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Project Name</TableHead>
                  <TableHead>URL</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Date</TableHead>
                  <TableHead className="text-right">Action</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {projects.slice(0, 5).map((project) => (
                  <TableRow key={project.id}>
                    <TableCell>
                      <div className="flex items-center gap-4">
                        <div className="w-10 h-10 bg-muted border rounded-lg flex items-center justify-center font-bold text-foreground">
                          {project.name.charAt(0).toUpperCase()}
                        </div>
                        <div>
                          <span className="font-semibold text-sm max-w-[200px] truncate block">{project.name}</span>
                          <div className="flex items-center gap-1.5 mt-0.5 text-muted-foreground">
                            <Terminal className="w-3 h-3" />
                            <span className="text-xs font-mono">laravel</span>
                          </div>
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      {project.status === 'running' ? (
                        <a
                          href={`https://${project.subdomain}.${window.location.hostname}`}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="flex items-center gap-1.5 text-muted-foreground hover:text-primary transition-colors"
                        >
                          <span className="font-mono text-xs">{project.subdomain}</span>
                          <ExternalLink className="w-3.5 h-3.5" />
                        </a>
                      ) : (
                        <span className="text-muted-foreground font-mono text-xs italic">Inactive</span>
                      )}
                    </TableCell>
                    <TableCell><StatusBadge status={project.status} /></TableCell>
                    <TableCell>
                      <div className="flex flex-col">
                        <span className="text-sm font-medium">{new Date(project.created_at).toLocaleDateString()}</span>
                        <span className="text-xs text-muted-foreground">{new Date(project.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}</span>
                      </div>
                    </TableCell>
                    <TableCell className="text-right">
                      <Link to={`/projects/${project.id}`} className={cn(buttonVariants({ variant: "outline", size: "sm" }))}>
                        Details
                      </Link>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </Card>
    </div>
  )
}

function StatCard({ label, value, icon: Icon, suffix }: { label: string, value: number, icon: any, suffix?: string }) {
  return (
    <Card className="hover:border-primary/30 transition-colors group">
      <CardContent className="p-6">
        <div className="flex justify-between items-start">
          <div>
            <p className="text-sm font-medium text-muted-foreground mb-1 mt-1">{label}</p>
            <div className="flex items-baseline gap-2">
              <h3 className="text-4xl font-bold tracking-tight">{value}</h3>
              {suffix && <span className="text-xl text-muted-foreground font-semibold">{suffix}</span>}
            </div>
          </div>
          <div className="w-12 h-12 rounded-xl bg-muted border flex items-center justify-center group-hover:scale-110 transition-transform text-muted-foreground">
            <Icon className="w-6 h-6" />
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

export default StudentDashboard
