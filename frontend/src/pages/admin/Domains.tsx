import { useState, useEffect, useCallback } from 'react'
import { toast } from 'sonner'
import { domainsAPI } from '../../services/api'
import {
  Globe,
  CheckCircle2,
  AlertCircle,
  AlertTriangle,
  Clock,
  Loader2,
  ExternalLink,
  Search,
  ShieldCheck
} from 'lucide-react'
import useTranslation from '../../lib/useTranslation'
import { Badge } from '@/components/ui/badge'
import { Card } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Input } from '@/components/ui/input'
import { CustomDomain } from '../../types'
import { FrameworkIcon } from '../../components/FrameworkIcon'

const StatusBadge = ({ status }: { status?: string }) => {
  const { t } = useTranslation()
  const cleanStatus = status || 'pending'
  const label = t(`domains.status.${cleanStatus}`) || cleanStatus

  let color = 'text-amber-500 border-amber-500/30 bg-amber-500/10'
  let Icon = Clock

  if (['active', 'ssl_active'].includes(cleanStatus)) {
    color = 'text-emerald-500 border-emerald-500/30 bg-emerald-500/10'
    Icon = CheckCircle2
  } else if (['dns_verified', 'ssl_queued', 'ssl_provisioning', 'renewal_pending'].includes(cleanStatus)) {
    color = 'text-cyan-500 border-cyan-500/30 bg-cyan-500/10'
    Icon = Loader2
  } else if (['degraded'].includes(cleanStatus)) {
    color = 'text-amber-500 border-amber-500/30 bg-amber-500/10'
    Icon = AlertTriangle
  } else if (['error', 'renewal_failed', 'ssl_failed'].includes(cleanStatus)) {
    color = 'text-rose-500 border-rose-500/30 bg-rose-500/10'
    Icon = AlertCircle
  }

  return (
    <Badge variant="outline" className={`gap-1.5 flex items-center w-fit text-[10px] uppercase font-bold tracking-wider px-2.5 py-1 rounded-md ${color}`}>
      <Icon className={`w-3.5 h-3.5 ${['ssl_queued', 'ssl_provisioning', 'renewal_pending'].includes(cleanStatus) ? 'animate-spin' : ''}`} />
      {label}
    </Badge>
  )
}

const HealthBadge = ({ health, error }: { health?: string, error?: string }) => {
  const { t } = useTranslation()
  if (!health || health === 'unknown') return null

  const isHealthy = health === 'healthy'
  const color = isHealthy ? 'text-emerald-500 border-emerald-500/30 bg-emerald-500/10' : 'text-rose-500 border-rose-500/30 bg-rose-500/10'
  const label = t(`domains.health.${health}`) || health

  return (
    <div className="flex items-center gap-2">
      <Badge variant="outline" className={`gap-1.5 flex items-center w-fit text-[10px] uppercase font-bold tracking-wider px-2.5 py-1 rounded-md ${color}`}>
        <ShieldCheck className="w-3.5 h-3.5" />
        {label}
      </Badge>
      {!isHealthy && error && error !== 'none' && (
        <span className="text-[10px] text-rose-500 font-medium truncate max-w-xs bg-rose-500/10 px-2.5 py-1 rounded-md border border-rose-500/20">{error}</span>
      )}
    </div>
  )
}

const AdminDomains = () => {
  const { t } = useTranslation()
  const [domains, setDomains] = useState<CustomDomain[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [search, setSearch] = useState('')

  const fetchData = useCallback(async () => {
    setIsLoading(true)
    try {
      const response = await domainsAPI.listAll()
      setDomains(response.data.data || [])
    } catch (error) {
      toast.error(t('common.loadError'))
    } finally {
      setIsLoading(false)
    }
  }, [t])

  useEffect(() => {
    fetchData()
  }, [fetchData])

  const filteredDomains = domains.filter(d =>
    d.domain.toLowerCase().includes(search.toLowerCase()) ||
    (d.project && d.project.name.toLowerCase().includes(search.toLowerCase())) ||
    (d.project && d.project.user && d.project.user.name.toLowerCase().includes(search.toLowerCase()))
  )

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-6 pb-4 border-b">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">{t('admin.domains.title')}</h1>
          <p className="text-muted-foreground max-w-2xl">
            {t('admin.domains.desc')}
          </p>
        </div>
      </div>

      <div className="flex items-center gap-4 bg-card p-4 border rounded-lg shadow-sm">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
          <Input
            placeholder={t('common.search')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-10"
          />
        </div>
      </div>

      {isLoading ? (
        <div className="flex flex-col items-center justify-center py-40 gap-6">
          <Loader2 className="w-12 h-12 text-primary animate-spin" />
          <p className="text-xs font-bold uppercase tracking-widest text-muted-foreground animate-pulse">{t('common.loading')}</p>
        </div>
      ) : filteredDomains.length === 0 ? (
        <Card className="p-24 text-center flex flex-col items-center max-w-xl mx-auto border-dashed">
          <div className="w-20 h-20 bg-muted rounded-full flex items-center justify-center mb-6">
            <Globe className="w-10 h-10 text-muted-foreground opacity-50" />
          </div>
          <h2 className="text-2xl font-bold tracking-tight mb-2">{t('domains.noDomains')}</h2>
        </Card>
      ) : (
        <div className="bg-card border border-border/50 rounded-xl overflow-hidden shadow-[0_2px_8px_rgba(0,0,0,0.04)]">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/15 hover:bg-muted/15 border-b border-border/40">
                <TableHead className="w-[34%] pl-6 text-[9px] font-bold uppercase tracking-widest text-muted-foreground/60">{t('domains.domainName')}</TableHead>
                <TableHead className="w-[26%] text-[9px] font-bold uppercase tracking-widest text-muted-foreground/60">{t('domains.owner')}</TableHead>
                <TableHead className="w-[26%] text-[9px] font-bold uppercase tracking-widest text-muted-foreground/60">{t('domains.linkedProject')}</TableHead>
                <TableHead className="w-[14%] pr-6 text-[9px] font-bold uppercase tracking-widest text-muted-foreground/60">{t('common.status')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredDomains.map((domain) => (
                <TableRow key={domain.id} className="group hover:bg-muted/20 border-b border-border/30 last:border-b-0 transition-colors">
                  <TableCell className="py-4 pl-6 font-medium">
                    <div className="flex items-center gap-3">
                      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border bg-muted/20 text-muted-foreground/80 shadow-sm transition-all group-hover:border-primary/15 group-hover:bg-primary/5 group-hover:text-primary">
                        <Globe className="h-3.5 w-3.5" />
                      </div>
                      <div className="flex min-w-0 flex-col text-left">
                        <span className="truncate text-[13px] font-semibold text-foreground/90">{domain.domain}</span>
                        <a
                          href={`https://${domain.domain}`}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="flex w-fit items-center gap-1 text-[9px] text-muted-foreground hover:text-primary hover:underline underline-offset-2 mt-0.5"
                        >
                          {t('common.url')} <ExternalLink className="w-2.5 h-2.5" />
                        </a>
                        {domain.config_hash && (
                          <span className="text-[9px] font-mono text-muted-foreground/40 mt-0.5">
                            SHA256:{domain.config_hash.substring(0, 8)}
                          </span>
                        )}
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="py-4">
                    <div className="flex items-center gap-2">
                      <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border bg-muted/30 text-[9px] font-bold text-muted-foreground transition-colors group-hover:border-primary/10">
                        {domain.project?.user?.name?.substring(0, 2).toUpperCase() || 'NA'}
                      </div>
                      <div className="flex min-w-0 flex-col text-left">
                        <span className="truncate text-[13px] font-medium text-foreground/90">{domain.project?.user?.name || t('common.unassigned')}</span>
                        <span className="truncate text-[9px] text-muted-foreground">{domain.project?.user?.email || '-'}</span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="py-4">
                    {domain.project ? (
                      <div className="flex items-center gap-2.5">
                        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border bg-muted/20 transition-colors group-hover:border-primary/10">
                          <FrameworkIcon framework={domain.project.framework} variant="plain" className="h-4 w-4" />
                        </div>
                        <div className="flex min-w-0 flex-col text-left">
                          <span className="truncate text-xs font-semibold text-foreground/95">{domain.project.name}</span>
                          <span className="truncate text-[9px] text-muted-foreground">
                            {domain.project.framework || domain.project.subdomain}
                          </span>
                        </div>
                      </div>
                    ) : (
                      <span className="text-xs text-destructive italic">{t('common.unassigned')}</span>
                    )}
                  </TableCell>
                  <TableCell className="py-4 pr-6 text-left">
                    <div className="flex flex-wrap items-center gap-2">
                      <StatusBadge status={domain.status} />
                      {['active', 'ssl_active', 'degraded'].includes(domain.status) && (
                        <HealthBadge health={domain.health_status} error={domain.error_message || (domain.error_code !== 'none' ? domain.error_code : undefined)} />
                      )}
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}

export default AdminDomains
