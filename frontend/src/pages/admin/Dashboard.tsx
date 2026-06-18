// ===========================================
// Admin Dashboard (PaaS Infrastructure)
// ===========================================

import { useState, useEffect, memo, useCallback } from 'react'
import { Link } from 'react-router-dom'
import useTranslation from '../../lib/useTranslation'
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
  Globe,
  Server,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
export interface DockerContainer {
  id: string;
  names: string[];
  image: string;
  state: string;
  status: string;
}

export interface DockerImage {
  id: string;
  repository: string;
  tag: string;
  status: string;
  size_human: string;
}

export interface SystemInfo {
  memory_used: number;
  memory_total: number;
  cpu_usage: number;
  cpu_cores: number;
  os: string;
  is_docker: boolean;
}

export interface SystemStats {
  system: SystemInfo | null;
  containers: DockerContainer[];
  images: DockerImage[];
  networks: unknown[];
  volumes: unknown[];
  recentProjects: unknown[];
}

// Helper to simplify verbose OS names
const simplifyOS = (os: string | undefined) => {
  if (!os) return 'Linux'
  return os
    .replace(/GNU\/Linux/i, '')
    .replace(/\(.*\)/g, '') // Remove (bookworm), (jammy), etc.
    .replace(/Enterprise Linux/i, 'EL')
    .replace(/Red Hat/i, 'RHEL')
    .trim()
}

const AdminDashboard = () => {
  const { t } = useTranslation()
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
      toast.error(t('common.loadError'))
      console.error('Failed to fetch system stats:', error)
    } finally {
      setIsLoading(false)
    }
  }, [t])

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
      toast.success(t('admin.purgeSuccess'))
      fetchData()
    } catch (error) {
      toast.error(t('admin.purgeFailed'))
    } finally {
      setIsPruning(false)
    }
  }

  const formatBytes = (bytes: number) => {
    if (!bytes || bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    const num = parseFloat((bytes / Math.pow(k, i)).toFixed(2))
    return num.toLocaleString(undefined, { maximumFractionDigits: 2 }) + ' ' + sizes[i]
  }

  if (isLoading && !data.system) {
    return (
      <div className="min-h-[60vh] flex flex-col items-center justify-center gap-4">
        <RefreshCw className="w-6 h-6 animate-spin text-primary" />
        <p className="text-muted-foreground font-medium uppercase tracking-wider text-[10px] animate-pulse">{t('common.loading')}</p>
      </div>
    )
  }

  const system = data?.system || null
  const containers = data?.containers || []
  const images = data?.images || []
  const networks = data?.networks || []
  const volumes = data?.volumes || []

  return (
    <div className="space-y-5 animate-in fade-in duration-500 pb-10">
      <Header
        t={t}
        onRefresh={fetchData}
        onPrune={handlePrune}
        isPruning={isPruning}
      />

      <SystemOverview
        t={t}
        system={system}
        containers={containers}
        images={images}
        networks={networks}
        volumes={volumes}
        formatBytes={formatBytes}
      />

      <div className="grid grid-cols-1 xl:grid-cols-2 gap-4">
        <ResourceTable
          t={t}
          title={t('admin.liveWorkload')}
          subtitle={t('admin.activeRunning')}
          icon={Box}
          data={containers}
          type="containers"
          viewAllPath="/admin/containers"
        />

        <ResourceTable
          t={t}
          title={t('admin.localImage')}
          subtitle={t('admin.cachedDocker')}
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
        title={t('admin.purgeTitle')}
        message={t('admin.purgeMessage')}
        confirmText={t('admin.initCleanup')}
        type="danger"
      />
    </div>
  )
}

interface HeaderProps {
  t: (key: string, data?: Record<string, string | number>) => string;
  onRefresh: () => void;
  onPrune: () => void;
  isPruning: boolean;
}

const Header = memo(({ t, onRefresh, onPrune, isPruning }: HeaderProps) => (
  <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 border-b border-border/40 pb-4">
    <div className="flex flex-col">
      <div className="flex items-center gap-2 mb-1">
        <h1 className="text-xl font-bold tracking-tight bg-clip-text text-transparent bg-gradient-to-r from-foreground to-foreground/70">{t('admin.platformDashboard')}</h1>
        <Badge variant="outline" className="h-5 px-1.5 text-[9px] font-bold uppercase tracking-widest bg-primary/5 text-primary border-primary/20 animate-pulse">
          {t('common.statusLive.live')}
        </Badge>
      </div>
      <p className="text-xs text-muted-foreground mt-0.5">
        {t('admin.adminDesc')}
      </p>
    </div>

    <div className="flex items-center gap-2">
      <Button variant="outline" size="sm" className="h-8 text-xs px-3" onClick={onRefresh}>
        <RefreshCw className="w-3.5 h-3.5 mr-1.5" />
        {t('admin.refresh')}
      </Button>
      <Button variant="destructive" size="sm" className="h-8 text-xs px-3" onClick={onPrune} disabled={isPruning}>
        <ShieldAlert className="w-3.5 h-3.5 mr-1.5" />
        {isPruning ? t('admin.cleaning') : t('admin.purgeRegistry')}
      </Button>
    </div>
  </div>
))

interface SystemOverviewProps {
  t: (key: string, data?: Record<string, string | number>) => string;
  system: SystemInfo | null;
  containers: DockerContainer[];
  images: DockerImage[];
  networks: unknown[];
  volumes: unknown[];
  formatBytes: (bytes: number) => string;
}

const SystemOverview = memo(({ t, system, containers, images, networks, volumes, formatBytes }: SystemOverviewProps) => {
  const memUsage = system ? (system.memory_used / system.memory_total) * 100 : 0

  return (
    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
      <StatCard
        title={t('admin.cpuLoad')}
        value={`${(system?.cpu_usage || 0).toLocaleString(undefined, { maximumFractionDigits: 1 })}%`}
        detail={t('admin.cpuCoresDetail', { count: system?.cpu_cores || 1 })}
        progress={Math.min(system?.cpu_usage || 0, 100)}
        icon={Cpu}
      />

      <StatCard
        title={t('admin.computeRam')}
        value={formatBytes(system?.memory_used || 0)}
        detail={t('admin.ofTotal', { total: formatBytes(system?.memory_total || 0) })}
        progress={memUsage}
        icon={Activity}
      />

      <StatCard
        title={t('admin.systemResources')}
        value={(images?.length || 0) + (containers?.length || 0)}
        detail={t('admin.resourcesDetail', {
          containers: containers?.length || 0,
          images: images?.length || 0
        })}
        progress={100}
        icon={Layers}
      />

      <div className="lg:col-span-3 grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <SmallStat icon={Network} label={t('common.networks')} value={networks?.length || 0} />
        <SmallStat icon={HardDrive} label={t('common.volumes')} value={volumes?.length || 0} />
        <SmallStat icon={Globe} label={t('admin.system.os')} value={simplifyOS(system?.os)} />
        <SmallStat
          icon={system?.is_docker ? Box : Server}
          label={t('admin.system.mode')}
          value={system?.is_docker ? 'Docker' : 'Bare Metal'}
          badge={system?.is_docker ? 'Virtual' : 'Host'}
        />
      </div>
    </div>
  )
})

interface StatCardProps {
  title: string;
  value: string | number;
  detail: string;
  progress: number;
  icon: React.ElementType;
}

const StatCard = ({ title, value, detail, progress, icon: Icon }: StatCardProps) => {
  let displayValue = value;
  let displayUnit = "";

  if (typeof value === 'string') {
    const match = value.match(/^([\d.,]+)\s*([a-zA-Z%]+)$/);
    if (match) {
      displayValue = match[1];
      displayUnit = match[2];
    }
  }

  return (
    <Card className="group shadow-sm border-border/50 bg-background/50 backdrop-blur-sm hover:shadow-md hover:border-primary/20 transition-all duration-300">
      <CardHeader className="flex flex-row items-center justify-between p-4 pb-1">
        <CardTitle className="text-[11px] font-semibold text-muted-foreground uppercase tracking-widest">{title}</CardTitle>
        <div className="p-1.5 rounded-md bg-muted/50 text-muted-foreground group-hover:text-primary group-hover:bg-primary/10 transition-colors">
          <Icon className="w-3.5 h-3.5" />
        </div>
      </CardHeader>
      <CardContent className="p-4 pt-0">
        <div className="flex items-baseline gap-2 mb-1">
          <div className="flex items-baseline gap-1">
            <span className="text-3xl font-extrabold tracking-tighter tabular-nums bg-clip-text text-transparent bg-gradient-to-b from-foreground to-foreground/80">{displayValue}</span>
            {displayUnit && <span className="text-xs font-bold text-muted-foreground/70 tracking-tight">{displayUnit}</span>}
          </div>
          <p className="text-[10px] font-medium text-muted-foreground/60 ml-auto bg-muted/30 px-2 py-0.5 rounded-full">{detail}</p>
        </div>
        <div className="h-2 w-full bg-secondary/30 rounded-full overflow-hidden mt-4 p-0.5">
          <div
            className="h-full rounded-full bg-gradient-to-r from-primary/80 via-primary to-primary shadow-[0_0_8px_rgba(var(--primary),0.4)] transition-all duration-1000 ease-in-out"
            style={{ width: `${progress}%` }}
          />
        </div>
      </CardContent>
    </Card>
  )
}

interface SmallStatProps {
  icon: React.ElementType;
  label: string;
  value: string | number;
}

const SmallStat = ({ icon: Icon, label, value, badge }: SmallStatProps & { badge?: string }) => {
  return (
    <Card className="group shadow-sm border-border/50 bg-background/40 backdrop-blur-sm hover:shadow-md hover:border-primary/20 transition-all duration-300">
      <CardContent className="flex items-center gap-3 p-3.5">
        <div className="w-9 h-9 shrink-0 rounded-lg bg-primary/10 flex items-center justify-center text-primary group-hover:scale-110 transition-transform">
          <Icon className="w-4.5 h-4.5" />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-1.5">
            <p className="text-[9px] font-bold text-muted-foreground uppercase tracking-[0.15em] leading-tight truncate">
              {label}
            </p>
            {badge && (
              <span className="text-[8px] font-bold px-1 rounded bg-primary/10 text-primary uppercase shrink-0">
                {badge}
              </span>
            )}
          </div>
          <p className="text-sm font-bold leading-tight mt-1 text-foreground truncate" title={String(value)}>
            {value}
          </p>
        </div>
        <Zap className="w-4 h-4 shrink-0 text-primary/20 group-hover:text-primary/40 transition-colors ml-auto" />
      </CardContent>
    </Card>
  )
}

interface ResourceTableProps {
  t: (key: string, data?: Record<string, string | number>) => string;
  title: string;
  subtitle: string;
  icon: React.ElementType;
  data: (DockerContainer | DockerImage)[];
  type: 'containers' | 'images';
  viewAllPath: string;
}

const ResourceTable = memo(({ t, title, subtitle, icon: Icon, data, type, viewAllPath }: ResourceTableProps) => (
  <Card className="flex flex-col shadow-sm border-border/50 overflow-hidden">
    <CardHeader className="flex flex-row items-center justify-between p-3.5 border-b border-border/40 bg-muted/20">
      <div className="flex items-center flex-row gap-3">
        <div className="p-1.5 bg-background border border-border/50 rounded text-muted-foreground">
          <Icon className="w-3.5 h-3.5" />
        </div>
        <div>
          <CardTitle className="text-sm font-semibold leading-tight">{title}</CardTitle>
          <CardDescription className="text-[10px] leading-tight mt-0.5">{subtitle}</CardDescription>
        </div>
      </div>
      <Button variant="ghost" size="sm" className="h-7 text-[10px] px-2 text-muted-foreground hover:text-foreground" render={<Link to={viewAllPath} />}>
        {t('admin.viewAll')} <ChevronRight className="w-3 h-3 ml-1" />
      </Button>
    </CardHeader>
    <CardContent className="p-0 flex-1 overflow-hidden">
      {(!data || data.length === 0) ? (
        <div className="p-8 text-center flex flex-col items-center justify-center h-full text-muted-foreground">
          <Icon className="w-6 h-6 mb-3 opacity-30" />
          <p className="text-[10px] font-medium uppercase tracking-widest">
            {type === 'containers' ? t('admin.networks.noContainers') : t('admin.networks.noImages')}
          </p>
        </div>
      ) : (
        <Table className="table-fixed">
          {type === 'containers' ? (
            <ContainerTableBody data={data as DockerContainer[]} t={t} />
          ) : (
            <ImageTableBody data={data as DockerImage[]} t={t} />
          )}
        </Table>
      )}
    </CardContent>
  </Card>
))

const ContainerTableBody = memo(({ data, t }: { data: DockerContainer[], t: (key: string, data?: Record<string, string | number>) => string }) => {
  const formatName = (name: string) => {
    if (!name) return '';
    const n = name.startsWith('/') ? name.substring(1) : name;
    return n.replace(/^paas-(project-)?/, '');
  };

  return (
  <>
    <TableHeader>
      <TableRow className="hover:bg-transparent border-b border-border/60 bg-muted/30">
        <TableHead className="h-9 py-2 text-[11px] font-semibold text-muted-foreground/80 tracking-wide w-[35%]">{t('admin.networks.identity')}</TableHead>
        <TableHead className="h-9 py-2 text-[11px] font-semibold text-muted-foreground/80 tracking-wide w-[30%]">{t('admin.networks.protocol')}</TableHead>
        <TableHead className="h-9 py-2 text-[11px] font-semibold text-muted-foreground/80 tracking-wide w-[15%] text-center">{t('common.status')}</TableHead>
        <TableHead className="h-9 py-2 text-[11px] font-semibold text-muted-foreground/80 tracking-wide w-[20%] text-right pr-8">{t('admin.networks.uptime')}</TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      {data.slice(0, 8).map((c: DockerContainer) => (
        <TableRow key={c.id} className="group hover:bg-muted/30 border-b border-border/40 transition-colors h-11">
          <TableCell className="align-middle py-2">
            <div className="flex items-center text-xs font-medium truncate text-foreground/90 group-hover:text-foreground" title={c.names?.[0]}>
              {formatName(c.names?.[0]) || c.id?.substring(0, 8)}
            </div>
          </TableCell>
          <TableCell className="align-middle py-2">
            <div className="flex items-center text-[11px] text-muted-foreground truncate">
              {c.image}
            </div>
          </TableCell>
          <TableCell className="text-center align-middle py-2">
            <div className="flex items-center justify-center">
              <Badge
                variant="outline"
                className={c.state === 'running'
                  ? "h-[22px] px-2.5 tracking-tight text-[10px] font-semibold rounded-full bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border-transparent uppercase"
                  : "h-[22px] px-2.5 tracking-tight text-[10px] font-semibold rounded-full bg-muted text-muted-foreground border-transparent uppercase"}
              >
                {c.state === 'running' ? t('status.running') : (c.state === 'exited' ? t('status.stopped') : c.state)}
              </Badge>
            </div>
          </TableCell>
          <TableCell className="text-right align-middle py-2 pr-8">
            <div className="flex items-center justify-end text-[10.5px] text-muted-foreground/80 font-mono">
              {c.status}
            </div>
          </TableCell>
        </TableRow>
      ))}
    </TableBody>
  </>
  )
})

const ImageTableBody = memo(({ data, t }: { data: DockerImage[], t: (key: string, data?: Record<string, string | number>) => string }) => {
  const formatRepo = (repo: string) => {
    if (!repo) return '';
    return repo.replace(/^paas-(project-)?/, '');
  };

  return (
  <>
    <TableHeader>
      <TableRow className="hover:bg-transparent border-b border-border/60 bg-muted/30">
        <TableHead className="h-9 py-2 text-[11px] font-semibold text-muted-foreground/80 tracking-wide w-[35%]">{t('admin.images.repository')}</TableHead>
        <TableHead className="h-9 py-2 text-[11px] font-semibold text-muted-foreground/80 tracking-wide w-[20%] text-center">{t('common.status')}</TableHead>
        <TableHead className="h-9 py-2 text-[11px] font-semibold text-muted-foreground/80 tracking-wide w-[20%] text-center">{t('admin.images.tag')}</TableHead>
        <TableHead className="h-9 py-2 text-[11px] font-semibold text-muted-foreground/80 tracking-wide w-[25%] text-right pr-8">{t('admin.images.size')}</TableHead>
      </TableRow>
    </TableHeader>
    <TableBody>
      {data.slice(0, 8).map((img: DockerImage, i: number) => (
        <TableRow key={i} className="group hover:bg-muted/30 border-b border-border/40 transition-colors h-11">
          <TableCell className="align-middle py-2">
            <div className="flex flex-col justify-center">
              <div className="text-xs font-medium truncate text-foreground/90 group-hover:text-foreground leading-tight mb-0.5" title={img.repository}>
                {formatRepo(img.repository)}
              </div>
              <div className="text-[10px] text-muted-foreground/60 font-mono leading-none">
                {img.id?.substring(7, 19)}
              </div>
            </div>
          </TableCell>
          <TableCell className="text-center align-middle py-2">
            <div className="flex items-center justify-center">
              <Badge
                variant="outline"
                className={img.status === 'In Use'
                  ? "h-[22px] px-2.5 tracking-tight text-[10px] font-semibold rounded-full bg-emerald-500/10 text-emerald-700 dark:text-emerald-400 border-transparent uppercase"
                  : "h-[22px] px-2.5 tracking-tight text-[10px] font-semibold rounded-full bg-muted text-muted-foreground border-transparent uppercase"}
              >
                {img.status === 'In Use' ? t('status.inUse') : img.status}
              </Badge>
            </div>
          </TableCell>
          <TableCell className="text-center align-middle py-2">
            <div className="flex items-center justify-center text-[10.5px] font-mono text-muted-foreground/80 truncate">
              {img.tag}
            </div>
          </TableCell>
          <TableCell className="text-right align-middle py-2 pr-8">
            <div className="flex items-center justify-end text-[10.5px] text-muted-foreground/80 font-mono">
              {img.size_human}
            </div>
          </TableCell>
        </TableRow>
      ))}
    </TableBody>
  </>
  )
})

export default AdminDashboard
