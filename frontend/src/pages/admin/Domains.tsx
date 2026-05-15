import { useState, useEffect, useCallback } from 'react'
import { toast } from 'sonner'
import { domainsAPI } from '../../services/api'
import { 
  Globe, 
  CheckCircle2, 
  AlertCircle, 
  Clock, 
  Loader2,
  ExternalLink,
  Search
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

const StatusBadge = ({ status }: { status: CustomDomain['status'] }) => {
  const { t } = useTranslation()
  const configs = {
    pending: { color: 'text-amber-600 border-amber-500/20 bg-amber-500/10', icon: Clock, label: t('status.pending') },
    active: { color: 'text-emerald-600 border-emerald-500/20 bg-emerald-500/10', icon: CheckCircle2, label: t('status.ready') },
    error: { color: 'text-rose-600 border-rose-500/20 bg-rose-500/10', icon: AlertCircle, label: t('status.failed') },
  }

  const config = configs[status] || configs.pending
  const Icon = config.icon

  return (
    <Badge variant="outline" className={`gap-1.5 flex w-fit ${config.color}`}>
      <Icon className="w-3 h-3" />
      {config.label}
    </Badge>
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
    d.project?.name.toLowerCase().includes(search.toLowerCase()) ||
    d.project?.user?.name.toLowerCase().includes(search.toLowerCase())
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
        <div className="bg-card border rounded-lg overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/40 hover:bg-muted/40">
                <TableHead className="w-[34%] pl-6 text-[10px] font-bold uppercase tracking-widest text-muted-foreground/70">{t('domains.domainName')}</TableHead>
                <TableHead className="w-[26%] text-[10px] font-bold uppercase tracking-widest text-muted-foreground/70">{t('domains.owner')}</TableHead>
                <TableHead className="w-[26%] text-[10px] font-bold uppercase tracking-widest text-muted-foreground/70">{t('domains.linkedProject')}</TableHead>
                <TableHead className="w-[14%] pr-6 text-[10px] font-bold uppercase tracking-widest text-muted-foreground/70">{t('common.status')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filteredDomains.map((domain) => (
                <TableRow key={domain.id} className="hover:bg-muted/25 transition-colors">
                  <TableCell className="py-4 pl-6 font-medium">
                    <div className="flex items-center gap-3">
                      <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border bg-muted/30 text-muted-foreground">
                        <Globe className="h-4 w-4" />
                      </div>
                      <div className="flex min-w-0 flex-col">
                        <span className="truncate text-sm font-semibold">{domain.domain}</span>
                        <a 
                          href={`https://${domain.domain}`} 
                          target="_blank" 
                          rel="noopener noreferrer"
                          className="flex w-fit items-center gap-1 text-[10px] text-muted-foreground hover:text-primary"
                        >
                          {t('common.url')} <ExternalLink className="w-2.5 h-2.5" />
                        </a>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="py-4">
                    <div className="flex items-center gap-2">
                      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-secondary text-[10px] font-bold">
                        {domain.project?.user?.name?.substring(0, 2).toUpperCase() || 'NA'}
                      </div>
                      <div className="flex min-w-0 flex-col">
                        <span className="truncate text-sm font-medium">{domain.project?.user?.name || t('common.unassigned')}</span>
                        <span className="truncate text-[10px] text-muted-foreground">{domain.project?.user?.email || '-'}</span>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="py-4">
                    {domain.project ? (
                      <div className="flex items-center gap-3">
                        <FrameworkIcon framework={domain.project.framework} variant="compact" className="h-8 w-8 shrink-0" />
                        <div className="flex min-w-0 flex-col">
                          <span className="truncate text-sm font-medium text-foreground/90">{domain.project.name}</span>
                          <span className="truncate text-[10px] text-muted-foreground">
                            {domain.project.framework || domain.project.subdomain || domain.project.uid}
                          </span>
                        </div>
                      </div>
                    ) : (
                      <span className="text-xs text-destructive italic">{t('common.unassigned')}</span>
                    )}
                  </TableCell>
                  <TableCell className="py-4 pr-6">
                    <StatusBadge status={domain.status} />
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
