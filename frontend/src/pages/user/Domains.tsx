import { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { domainsAPI, projectsAPI } from '../../services/api'
import { 
  Globe, 
  Trash2, 
  ArrowRightLeft,
  Loader2,
  ExternalLink,
  MoreVertical
} from 'lucide-react'
import useTranslation from '../../lib/useTranslation'
import ConfirmationModal from '../../components/ConfirmationModal'
import { Button } from '@/components/ui/button'
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
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { CustomDomain, Project } from '../../types'
import { FrameworkIcon } from '../../components/FrameworkIcon'

const StatusBadge = ({ status }: { status?: string }) => {
  const { t } = useTranslation()
  const cleanStatus = status || 'pending'
  const label = t(`domains.status.${cleanStatus}`) || cleanStatus

  let dotColor = 'bg-amber-500 shadow-[0_0_8px_rgba(245,158,11,0.5)]'
  let textColor = 'text-amber-500/90'
  let bgColor = 'bg-amber-500/5 border-amber-500/15'

  if (['active', 'ssl_active', 'dns_verified'].includes(cleanStatus)) {
    dotColor = 'bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.5)]'
    textColor = 'text-emerald-500/90'
    bgColor = 'bg-emerald-500/5 border-emerald-500/15'
  } else if (['ssl_queued', 'ssl_provisioning', 'renewal_pending'].includes(cleanStatus)) {
    dotColor = 'bg-cyan-500 shadow-[0_0_8px_rgba(6,182,212,0.5)]'
    textColor = 'text-cyan-500/90'
    bgColor = 'bg-cyan-500/5 border-cyan-500/15'
  } else if (['degraded'].includes(cleanStatus)) {
    dotColor = 'bg-amber-500 shadow-[0_0_8px_rgba(245,158,11,0.5)]'
    textColor = 'text-amber-500/90'
    bgColor = 'bg-amber-500/5 border-amber-500/15'
  } else if (['error', 'renewal_failed', 'ssl_failed'].includes(cleanStatus)) {
    dotColor = 'bg-rose-500 shadow-[0_0_8px_rgba(239,68,68,0.5)]'
    textColor = 'text-rose-500/90'
    bgColor = 'bg-rose-500/5 border-rose-500/15'
  }

  const isSpinning = ['ssl_queued', 'ssl_provisioning', 'renewal_pending'].includes(cleanStatus)

  return (
    <Badge 
      variant="outline" 
      className={`gap-2 items-center flex w-fit text-[10px] uppercase font-bold tracking-wider px-2.5 py-1 rounded-full border ${bgColor} ${textColor} transition-all duration-300`}
    >
      <span className={`w-1.5 h-1.5 rounded-full ${dotColor} ${isSpinning ? 'animate-pulse' : ''}`} />
      <span>{label}</span>
    </Badge>
  )
}

const HealthBadge = ({ health, error }: { health?: string, error?: string }) => {
  const { t } = useTranslation()
  if (!health || health === 'unknown') return null

  const isHealthy = health === 'healthy'
  const dotColor = isHealthy ? 'bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.5)]' : 'bg-rose-500 shadow-[0_0_8px_rgba(239,68,68,0.5)]'
  const textColor = isHealthy ? 'text-emerald-500/90' : 'text-rose-500/90'
  const bgColor = isHealthy ? 'bg-emerald-500/5 border-emerald-500/15' : 'bg-rose-500/5 border-rose-500/15'
  const label = t(`domains.health.${health}`) || health

  return (
    <div className="flex items-center gap-2 mt-1.5">
      <Badge 
        variant="outline" 
        className={`gap-2 items-center flex w-fit text-[9px] uppercase font-bold tracking-wider px-2 py-0.5 rounded-full border ${bgColor} ${textColor}`}
      >
        <span className={`w-1 h-1 rounded-full ${dotColor}`} />
        {label}
      </Badge>
      {!isHealthy && error && error !== 'none' && (
        <span className="text-[10px] text-rose-500 font-medium truncate max-w-xs bg-rose-500/5 px-2 py-0.5 rounded border border-rose-500/10">{error}</span>
      )}
    </div>
  )
}

const Domains = () => {
  const { t } = useTranslation()
  const [domains, setDomains] = useState<CustomDomain[]>([])
  const [projects, setProjects] = useState<Project[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [isTransferring, setIsTransferring] = useState(false)
  
  const [transferModal, setTransferModal] = useState<{
    isOpen: boolean;
    domain?: CustomDomain;
    targetProjectId: string;
  }>({
    isOpen: false,
    targetProjectId: ''
  })

  const [confirmModal, setConfirmModal] = useState({
    isOpen: false,
    title: '',
    message: '',
    onConfirm: () => {},
  })

  const fetchData = useCallback(async () => {
    setIsLoading(true)
    try {
      const [domainsRes, projectsRes] = await Promise.all([
        domainsAPI.listOwn(),
        projectsAPI.listOwn()
      ])
      setDomains(domainsRes.data.data || [])
      setProjects(projectsRes.data.data || [])
    } catch (error) {
      toast.error(t('common.loadError'))
    } finally {
      setIsLoading(false)
    }
  }, [t])

  useEffect(() => {
    fetchData()

    // Poll every 4 seconds to sync domain status and routing states silently
    const interval = setInterval(() => {
      const quietFetch = async () => {
        try {
          const [domainsRes, projectsRes] = await Promise.all([
            domainsAPI.listOwn(),
            projectsAPI.listOwn()
          ])
          setDomains(domainsRes.data.data || [])
          setProjects(projectsRes.data.data || [])
        } catch (error) {
          // Silent catch to keep UI experience clean and uninterrupted
        }
      }
      quietFetch()
    }, 4000)

    return () => clearInterval(interval)
  }, [fetchData])

  const handleRemove = (domain: CustomDomain) => {
    setConfirmModal({
      isOpen: true,
      title: t('domains.confirmDelete'),
      message: domain.domain,
      onConfirm: async () => {
        try {
          await projectsAPI.removeDomain(domain.project_id, domain.id)
          toast.success(t('common.deleteSuccess'))
          fetchData()
        } catch (error) {
          toast.error(t('common.error'))
        }
      }
    })
  }

  const handleTransfer = async () => {
    if (!transferModal.domain || !transferModal.targetProjectId) return
    
    setIsTransferring(true)
    try {
      await domainsAPI.transfer(
        transferModal.domain.project_id,
        transferModal.domain.id,
        transferModal.targetProjectId
      )
      toast.success(t('domains.transferSuccess'))
      setTransferModal(prev => ({ ...prev, isOpen: false }))
      fetchData()
    } catch (error) {
      const err = error as { response?: { data?: { code?: string; message?: string } } }
      const errCode = err.response?.data?.code
      const errMsg = errCode ? t(`domains.errors.${errCode}`) : err.response?.data?.message
      toast.error(errMsg || t('common.error'))
    } finally {
      setIsTransferring(false)
    }
  }

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-10">
      <ConfirmationModal 
        onClose={() => setConfirmModal(prev => ({ ...prev, isOpen: false }))}
        {...confirmModal}
        type="danger"
        confirmText={t('common.delete')}
      />

      {/* Transfer Modal */}
      <Dialog open={transferModal.isOpen} onOpenChange={(open) => setTransferModal(prev => ({ ...prev, isOpen: open }))}>
        <DialogContent className="sm:max-w-[440px]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-base text-left">
              <ArrowRightLeft className="w-4 h-4 text-primary" />
              {t('domains.transfer')}
            </DialogTitle>
            <DialogDescription className="text-xs leading-relaxed text-left">
              {t('domains.transferDesc')}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-5 text-left">
            {/* Domain being transferred */}
            <div className="space-y-1.5">
              <label className="text-xs font-medium text-muted-foreground">
                {t('domains.domainName')}
              </label>
              <div className="flex items-center gap-3 rounded-md border bg-muted/30 px-3 py-2.5">
                <Globe className="h-4 w-4 shrink-0 text-muted-foreground" />
                <span className="truncate font-mono text-sm leading-none flex items-center">{transferModal.domain?.domain}</span>
              </div>
            </div>

            {/* Target project selector */}
            <div className="space-y-1.5 pb-3">
              <label className="text-xs font-medium text-muted-foreground">
                {t('domains.selectTarget')}
              </label>
              <Select 
                value={transferModal.targetProjectId} 
                onValueChange={(val) => setTransferModal(prev => ({ ...prev, targetProjectId: val || '' }))}
              >
                <SelectTrigger className="h-10 w-full">
                  <SelectValue placeholder={t('domains.selectTarget')}>
                    {(value) => {
                      if (!value) return null
                      const selectedProject = projects.find(p => p.id.toString() === value)
                      return selectedProject ? selectedProject.name : value
                    }}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent align="start" alignItemWithTrigger={false} className="min-w-[var(--radix-select-trigger-width)] p-1">
                  {projects
                    .filter(p => p.id !== transferModal.domain?.project_id)
                    .map(p => (
                      <SelectItem key={p.id} value={p.id.toString()} label={p.name} className="py-2.5 pl-3">
                        <div className="flex min-w-0 flex-col gap-0.5 text-left">
                          <span className="truncate text-sm font-medium leading-none">{p.name}</span>
                          <span className="truncate text-[10px] leading-none text-muted-foreground">{p.subdomain}</span>
                        </div>
                      </SelectItem>
                    ))
                  }
                  {projects.filter(p => p.id !== transferModal.domain?.project_id).length === 0 && (
                    <div className="p-4 text-center text-xs text-muted-foreground">
                      {t('domains.noOtherProjects')}
                    </div>
                  )}
                </SelectContent>
              </Select>
            </div>
          </div>

          <DialogFooter>
            <Button variant="outline" onClick={() => setTransferModal(prev => ({ ...prev, isOpen: false }))}>
              {t('common.cancel')}
            </Button>
            <Button 
              onClick={handleTransfer} 
              disabled={!transferModal.targetProjectId || isTransferring}
            >
              {isTransferring && <Loader2 className="w-4 h-4 mr-2 animate-spin" />}
              {t('domains.transferAction')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Header */}
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-6 pb-4 border-b">
        <div className="text-left">
          <h1 className="text-3xl font-bold tracking-tight mb-2">{t('student.domains.title')}</h1>
          <p className="text-muted-foreground max-w-2xl">
            {t('student.domains.desc')}
          </p>
        </div>
      </div>

      {isLoading ? (
        <div className="space-y-3">
          {[1, 2, 3].map(i => (
            <div key={i} className="flex items-center gap-4 rounded-lg border bg-card p-4 animate-pulse">
              <div className="h-8 w-8 rounded bg-muted" />
              <div className="flex-1 space-y-2">
                <div className="h-4 w-48 rounded bg-muted" />
                <div className="h-3 w-24 rounded bg-muted" />
              </div>
              <div className="h-5 w-16 rounded-full bg-muted" />
            </div>
          ))}
        </div>
      ) : domains.length === 0 ? (
        <Card className="py-16 px-8 text-center flex flex-col items-center max-w-md mx-auto border-dashed">
          <div className="w-14 h-14 bg-muted rounded-full flex items-center justify-center mb-4">
            <Globe className="w-7 h-7 text-muted-foreground" />
          </div>
          <h2 className="text-lg font-semibold tracking-tight mb-1">{t('domains.noDomains')}</h2>
          <p className="text-sm text-muted-foreground max-w-xs">{t('domains.noDomainsDesc')}</p>
        </Card>
      ) : (
        <div className="bg-card border border-border/50 rounded-xl overflow-hidden shadow-[0_2px_8px_rgba(0,0,0,0.04)]">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/15 hover:bg-muted/15 border-b border-border/40">
                <TableHead className="w-[36%] pl-6 font-bold text-[9px] uppercase tracking-widest text-muted-foreground/60">{t('domains.domainName')}</TableHead>
                <TableHead className="w-[30%] font-bold text-[9px] uppercase tracking-widest text-muted-foreground/60">{t('domains.linkedProject')}</TableHead>
                <TableHead className="w-[22%] font-bold text-[9px] uppercase tracking-widest text-muted-foreground/60">{t('common.status')}</TableHead>
                <TableHead className="w-[12%] pr-6 text-right font-bold text-[9px] uppercase tracking-widest text-muted-foreground/60">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {domains.map((domain) => (
                <TableRow key={domain.id} className="group hover:bg-muted/20 border-b border-border/30 last:border-b-0 transition-colors">
                  <TableCell className="py-4 pl-6 font-medium">
                    <div className="flex items-center gap-3">
                      <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border bg-muted/20 text-muted-foreground/80 shadow-sm transition-all group-hover:border-primary/15 group-hover:bg-primary/5 group-hover:text-primary">
                        <Globe className="w-3.5 h-3.5" />
                      </div>
                      <div className="flex flex-col text-left">
                        <span className="text-[13px] font-semibold text-foreground/90">{domain.domain}</span>
                        <a 
                          href={`https://${domain.domain}`} 
                          target="_blank" 
                          rel="noopener noreferrer"
                          className="text-[9px] text-muted-foreground hover:text-primary hover:underline underline-offset-2 flex items-center gap-1 w-fit mt-0.5"
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
                  <TableCell className="text-left py-4">
                    {domain.project ? (
                      <Link 
                        to={`/projects/${domain.project.uid}`}
                        className="flex items-center gap-2.5 text-sm text-muted-foreground hover:text-primary transition-colors group/link"
                      >
                        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md border bg-muted/20 group-hover/link:border-primary/20 group-hover/link:bg-primary/5 transition-all">
                          <FrameworkIcon framework={domain.project.framework} variant="plain" className="h-4 w-4" />
                        </div>
                        <div className="flex flex-col text-left">
                          <span className="truncate text-xs font-semibold text-foreground/95 group-hover/link:text-primary transition-colors">{domain.project.name}</span>
                          <span className="truncate text-[9px] text-muted-foreground">{domain.project.subdomain}</span>
                        </div>
                      </Link>
                    ) : (
                      <span className="text-xs text-destructive italic">{t('common.unassigned')}</span>
                    )}
                  </TableCell>
                  <TableCell className="text-left py-4">
                    <StatusBadge status={domain.status} />
                    {['active', 'ssl_active', 'degraded'].includes(domain.status) && (
                      <HealthBadge health={domain.health_status} error={domain.error_message || (domain.error_code !== 'none' ? domain.error_code : undefined)} />
                    )}
                  </TableCell>
                  <TableCell className="pr-6 py-4">
                    <div className="flex justify-end">
                      <DropdownMenu>
                        <DropdownMenuTrigger className="flex h-8 w-8 items-center justify-center rounded-md border border-transparent text-muted-foreground opacity-70 transition-colors hover:border-border hover:bg-muted hover:text-foreground group-hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-primary/20">
                          <MoreVertical className="w-4 h-4" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-48 p-1.5">
                          <DropdownMenuItem 
                            className="gap-3 py-2 cursor-pointer focus:bg-primary/5 focus:text-primary transition-colors"
                            onClick={() => setTransferModal({ isOpen: true, domain, targetProjectId: '' })}
                          >
                            <ArrowRightLeft className="w-4 h-4" />
                            <span className="font-medium text-xs">{t('domains.transfer')}</span>
                          </DropdownMenuItem>
                          <DropdownMenuItem 
                            className="gap-3 py-2 cursor-pointer text-destructive focus:text-destructive focus:bg-destructive/5 transition-colors"
                            onClick={() => handleRemove(domain)}
                          >
                            <Trash2 className="w-4 h-4" />
                            <span className="font-medium text-xs">{t('common.delete')}</span>
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
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

export default Domains
