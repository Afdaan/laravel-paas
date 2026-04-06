// ===========================================
// Admin Dashboard (PaaS Infrastructure)
// ===========================================

import { useState, useEffect, memo, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { systemAPI } from '../../services/api'
import { toast } from 'sonner'
import ConfirmationModal from '../../components/ConfirmationModal'
import { 
  RefreshCw, 
  Cpu, 
  HardDrive, 
  Box, 
  Image as ImageIcon, 
  Network, 
  Layers,
  Activity,
  ShieldAlert,
  ChevronRight,
  Zap,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
export interface SystemStats {
  system: any;
  containers: any[];
  images: any[];
  networks: any[];
  volumes: any[];
  recentProjects: any[];
}

function AdminDashboard() {
  const [data, setData] = useState<SystemStats>({
    system: null,
    containers: [],
    images: [],
    networks: [],
    volumes: [],
    recentProjects: []
  })
  const [isLoading, setIsLoading] = useState(true)
  const [isPruning, setIsPruning] = useState(false)
  const [isPruneModalOpen, setIsPruneModalOpen] = useState(false)

  const fetchData = useCallback(async () => {
    try {
      const res = await systemAPI.getStats()
      setData(res.data)
    } catch (error) {
      console.error('Failed to fetch system stats:', error)
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 15000)
    return () => clearInterval(interval)
  }, [fetchData])

  const handlePrune = () => {
    setIsPruneModalOpen(true)
  }

  const confirmPrune = async () => {
    setIsPruning(true)
    try {
      await systemAPI.prune()
      toast.success('System purged of unused assets')
      fetchData()
    } catch (error) {
      toast.error('Clean operation failed')
    } finally {
      setIsPruning(false)
    }
  }

  const formatBytes = (bytes: number) => {
    if (!bytes || bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
  }

  if (isLoading && !data.system) {
    return (
      <div className="min-h-[60vh] flex flex-col items-center justify-center gap-6">
        <RefreshCw className="w-8 h-8 animate-spin text-primary" />
        <p className="text-muted-foreground font-semibold uppercase tracking-widest text-xs animate-pulse">Loading Dashboard</p>
      </div>
    )
  }

  const system = data?.system || null
  const containers = data?.containers || []
  const images = data?.images || []
  const networks = data?.networks || []
  const volumes = data?.volumes || []

  return (
    <div className="space-y-8 animate-in fade-in duration-500">
      <Header onRefresh={fetchData} onPrune={handlePrune} isPruning={isPruning} />
      
      <SystemOverview 
        system={system} 
        containers={containers} 
        images={images} 
        networks={networks} 
        volumes={volumes} 
        formatBytes={formatBytes} 
      />
      
      <div className="grid grid-cols-1 xl:grid-cols-2 gap-8">
        <ResourceTable 
          title="Live Workload Containers" 
          subtitle="Active running instances"
          icon={Box}
          data={containers}
          type="containers"
          viewAllPath="/admin/containers"
        />

        <ResourceTable 
          title="Local Image Snapshots" 
          subtitle="Cached docker images"
          icon={ImageIcon}
          data={images}
          type="images"
          viewAllPath="/admin/images"
        />
      </div>

      <ConfirmationModal 
        isOpen={isPruneModalOpen}
        onClose={() => setIsPruneModalOpen(false)}
        onConfirm={confirmPrune}
        title="Execute System Purge?"
        message="This will permanently delete all inactive images and orphaned volumes. This operation will free up local storage but cannot be rolled back."
        confirmText="Initialize Cleanup"
        type="danger"
      />
    </div>
  )
}

interface HeaderProps {
  onRefresh: () => void;
  onPrune: () => void;
  isPruning: boolean;
}

const Header = memo(({ onRefresh, onPrune, isPruning }: HeaderProps) => (
  <div className="flex flex-col md:flex-row md:items-end justify-between gap-6">
    <div>
      <h1 className="text-3xl font-bold tracking-tight mb-2">Platform Dashboard</h1>
      <p className="text-muted-foreground max-w-2xl">
        Monitoring global infrastructure state and resource orchestration across the student cluster.
      </p>
    </div>
    
    <div className="flex items-center gap-4">
      <Button variant="outline" onClick={onRefresh}>
        <RefreshCw className="w-4 h-4 mr-2" />
        Refresh
      </Button>
      <Button variant="destructive" onClick={onPrune} disabled={isPruning}>
        <ShieldAlert className="w-4 h-4 mr-2" />
        {isPruning ? 'Cleaning...' : 'Purge Registry'}
      </Button>
    </div>
  </div>
))

interface SystemOverviewProps {
  system: any;
  containers: any[];
  images: any[];
  networks: any[];
  volumes: any[];
  formatBytes: (bytes: number) => string;
}

const SystemOverview = memo(({ system, containers, images, networks, volumes, formatBytes }: SystemOverviewProps) => {
  const memUsage = system ? (system.memory_used / system.memory_total) * 100 : 0
  
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
      <StatCard 
        title="CPU Load" 
        value={`${(system?.cpu_usage || 0).toFixed(1)}%`}
        detail={`${system?.cpu_cores || 1} CPU Cores`}
        progress={Math.min(system?.cpu_usage || 0, 100)}
        icon={Cpu}
      />

      <StatCard 
        title="Compute RAM" 
        value={formatBytes(system?.memory_used || 0)}
        detail={`of ${formatBytes(system?.memory_total || 0)} total`}
        progress={memUsage}
        icon={Activity}
      />

      <StatCard 
        title="System Resources" 
        value={(images?.length || 0) + (containers?.length || 0)}
        detail={`${containers?.length || 0} Containers / ${images?.length || 0} Images`}
        progress={100}
        icon={Layers}
      />
      
      <div className="lg:col-span-3 grid grid-cols-1 sm:grid-cols-3 gap-6">
        <SmallStat icon={Network} label="Networks" value={networks?.length || 0} />
        <SmallStat icon={HardDrive} label="Volumes" value={volumes?.length || 0} />
        <SmallStat icon={Box} label="Docker Engine" value={system?.docker_version || 'N/A'} />
      </div>
    </div>
  )
})

interface StatCardProps {
  title: string;
  value: string | number;
  detail: string;
  progress: number;
  icon: any;
}

const StatCard = ({ title, value, detail, progress, icon: Icon }: StatCardProps) => {
  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">{title}</CardTitle>
        <Icon className="w-4 h-4 text-muted-foreground" />
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-bold">{value}</div>
        <p className="text-xs text-muted-foreground mt-1 mb-4">{detail}</p>
        <div className="h-2 w-full bg-secondary rounded-full overflow-hidden">
          <div 
            className="h-full bg-primary transition-all duration-1000" 
            style={{ width: `${progress}%` }}
          />
        </div>
      </CardContent>
    </Card>
  )
}

interface SmallStatProps {
  icon: any;
  label: string;
  value: string | number;
}

const SmallStat = ({ icon: Icon, label, value }: SmallStatProps) => {
  return (
    <Card>
      <CardContent className="flex items-center justify-between p-6">
        <div className="flex items-center gap-4">
          <div className="w-10 h-10 rounded-md bg-primary/10 flex items-center justify-center text-primary">
            <Icon className="w-5 h-5" />
          </div>
          <div>
            <p className="text-sm font-medium text-muted-foreground">{label}</p>
            <p className="text-2xl font-bold">{value}</p>
          </div>
        </div>
        <Zap className="w-5 h-5 text-muted-foreground/30" />
      </CardContent>
    </Card>
  )
}

interface ResourceTableProps {
  title: string;
  subtitle: string;
  icon: any;
  data: any[];
  type: string;
  viewAllPath: string;
}

const ResourceTable = memo(({ title, subtitle, icon: Icon, data, type, viewAllPath }: ResourceTableProps) => (
  <Card className="flex flex-col">
    <CardHeader className="flex flex-row items-center justify-between">
      <div className="flex items-center flex-row gap-4">
        <Icon className="w-5 h-5 text-muted-foreground" />
        <div>
          <CardTitle>{title}</CardTitle>
          <CardDescription>{subtitle}</CardDescription>
        </div>
      </div>
      <Button variant="ghost" size="sm" render={<Link to={viewAllPath} />}>
        View All <ChevronRight className="w-4 h-4 ml-1" />
      </Button>
    </CardHeader>
    <CardContent className="p-0 border-t flex-1 overflow-hidden">
      {(!data || data.length === 0) ? (
        <div className="p-12 text-center flex flex-col items-center justify-center h-full text-muted-foreground">
          <Icon className="w-8 h-8 mb-4 opacity-50" />
          <p className="text-sm font-medium uppercase tracking-widest">No {type} Found</p>
        </div>
      ) : (
        <Table className="table-fixed">
          {type === 'containers' ? (
            <ContainerTableBody data={data} />
          ) : (
            <ImageTableBody data={data} />
          )}
        </Table>
      )}
    </CardContent>
  </Card>
))

const ContainerTableBody = memo(({ data }: { data: any[] }) => (
  <>
    <TableHeader>
      <TableRow>
        <TableHead className="w-[30%]">Identity</TableHead>
        <TableHead className="w-[35%]">Protocol</TableHead>
        <TableHead className="w-[15%] text-center">Status</TableHead>
        <TableHead className="w-[20%] text-right">Uptime</TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      {data.slice(0, 8).map((c) => (
        <TableRow key={c.id}>
          <TableCell>
            <div className="font-medium truncate">
              {c.names[0] || c.id.substring(0, 8)}
            </div>
          </TableCell>
          <TableCell>
            <span className="text-xs text-muted-foreground truncate block">{c.image}</span>
          </TableCell>
          <TableCell className="text-center">
            <Badge variant={c.state === 'running' ? 'default' : 'destructive'} className="capitalize">
              {c.state}
            </Badge>
          </TableCell>
          <TableCell className="text-right text-xs text-muted-foreground truncate">
            {c.status}
          </TableCell>
        </TableRow>
      ))}
    </TableBody>
  </>
))

const ImageTableBody = memo(({ data }: { data: any[] }) => (
  <>
    <TableHeader>
      <TableRow>
        <TableHead className="w-[35%]">Repository</TableHead>
        <TableHead className="w-[20%] text-center">Status</TableHead>
        <TableHead className="w-[20%] text-center">Tag</TableHead>
        <TableHead className="w-[25%] text-right">Size</TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      {data.slice(0, 8).map((img, i) => (
        <TableRow key={i}>
          <TableCell>
            <div className="font-medium truncate">{img.repository}</div>
            <div className="text-[10px] text-muted-foreground font-mono">{img.id?.substring(7, 19)}</div>
          </TableCell>
          <TableCell className="text-center">
            <Badge variant={img.status === 'In Use' ? 'default' : 'secondary'}>
              {img.status}
            </Badge>
          </TableCell>
          <TableCell className="text-center font-mono text-xs truncate">
            {img.tag}
          </TableCell>
          <TableCell className="text-right text-xs text-muted-foreground">
            {img.size_human}
          </TableCell>
        </TableRow>
      ))}
    </TableBody>
  </>
))

export default AdminDashboard


