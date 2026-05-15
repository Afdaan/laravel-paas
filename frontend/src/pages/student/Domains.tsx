import { useState, useEffect, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { toast } from 'sonner'
import { domainsAPI, projectsAPI } from '../../services/api'
import { 
  Globe, 
  Trash2, 
  CheckCircle2, 
  AlertCircle, 
  Clock, 
  ArrowRightLeft,
  Loader2,
  ExternalLink,
  FolderGit2,
  MoreVertical
} from 'lucide-react'
import useTranslation from '../../lib/useTranslation'
import ConfirmationModal from '../../components/ConfirmationModal'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent } from '@/components/ui/card'
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
      await domainsAPI.transfer(transferModal.domain.id, transferModal.targetProjectId)
      toast.success(t('domains.transferSuccess'))
      setTransferModal(prev => ({ ...prev, isOpen: false }))
      fetchData()
    } catch (error) {
      toast.error(t('common.error'))
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
        <DialogContent className="p-0 sm:max-w-[460px] overflow-hidden">
          <DialogHeader>
            <div className="px-5 pt-5">
              <DialogTitle className="flex items-center gap-2 text-base">
                <ArrowRightLeft className="w-4 h-4 text-primary" />
                {t('domains.transfer')}
              </DialogTitle>
            </div>
            <DialogDescription className="px-5 text-xs leading-relaxed">
              {t('domains.transferDesc')}
            </DialogDescription>
          </DialogHeader>
          <div className="px-5 pb-5 pt-2 space-y-5">
            <div className="space-y-3">
              <label className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground/70 flex items-center gap-2">
                <Globe size={12} className="text-primary/50" />
                {t('domains.domainName')}
              </label>
              <Card className="border-border/70 bg-muted/20 shadow-none">
                <CardContent className="flex items-center justify-between gap-3 p-3">
                  <span className="truncate font-mono text-sm text-foreground/90">{transferModal.domain?.domain}</span>
                  <Globe className="h-4 w-4 shrink-0 text-muted-foreground/50" />
                </CardContent>
              </Card>
            </div>

            <div className="space-y-3">
              <label className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground/70 flex items-center gap-2">
                <FolderGit2 size={12} className="text-primary/50" />
                {t('domains.selectTarget')}
              </label>
              <Select 
                value={transferModal.targetProjectId} 
                onValueChange={(val) => setTransferModal(prev => ({ ...prev, targetProjectId: val || '' }))}
              >
                <SelectTrigger className="h-11 w-full border-border/70 bg-background px-3 hover:bg-muted/30 transition-colors">
                  <SelectValue placeholder={t('domains.selectTarget')} />
                </SelectTrigger>
                <SelectContent align="start" className="min-w-[var(--radix-select-trigger-width)] p-1">
                  {projects
                    .filter(p => p.id !== transferModal.domain?.project_id)
                    .map(p => (
                      <SelectItem key={p.id} value={p.id.toString()} className="py-2.5 pl-2 pr-8">
                        <div className="flex min-w-0 flex-col gap-0.5">
                          <span className="truncate text-sm font-medium leading-none">{p.name}</span>
                          <span className="truncate text-[10px] leading-none text-muted-foreground">{p.subdomain}</span>
                        </div>
                      </SelectItem>
                    ))
                  }
                  {projects.filter(p => p.id !== transferModal.domain?.project_id).length === 0 && (
                    <div className="p-4 text-center text-xs text-muted-foreground">
                      No other projects available
                    </div>
                  )}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter className="mt-0">
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
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">{t('student.domains.title')}</h1>
          <p className="text-muted-foreground max-w-2xl">
            {t('student.domains.desc')}
          </p>
        </div>
      </div>

      {isLoading ? (
        <div className="flex flex-col items-center justify-center py-40 gap-6">
          <Loader2 className="w-12 h-12 text-primary animate-spin" />
          <p className="text-xs font-bold uppercase tracking-widest text-muted-foreground animate-pulse">{t('common.loading')}</p>
        </div>
      ) : domains.length === 0 ? (
        <Card className="p-24 text-center flex flex-col items-center max-w-xl mx-auto border-dashed">
          <div className="w-20 h-20 bg-muted rounded-full flex items-center justify-center mb-6">
            <Globe className="w-10 h-10 text-muted-foreground opacity-50" />
          </div>
          <h2 className="text-2xl font-bold tracking-tight mb-2">{t('domains.noDomains')}</h2>
          <p className="text-muted-foreground mb-8 max-w-sm">{t('domains.noDomainsDesc')}</p>
        </Card>
      ) : (
        <div className="bg-card border rounded-lg overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/30 hover:bg-muted/30 border-b-0">
                <TableHead className="w-[42%] pl-6 font-bold text-[10px] uppercase tracking-widest text-muted-foreground/70">{t('domains.domainName')}</TableHead>
                <TableHead className="w-[30%] font-bold text-[10px] uppercase tracking-widest text-muted-foreground/70">{t('domains.linkedProject')}</TableHead>
                <TableHead className="w-[16%] font-bold text-[10px] uppercase tracking-widest text-muted-foreground/70">{t('common.status')}</TableHead>
                <TableHead className="w-[12%] pr-6 text-right font-bold text-[10px] uppercase tracking-widest text-muted-foreground/70">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {domains.map((domain) => (
                <TableRow key={domain.id} className="group transition-colors">
                  <TableCell className="font-medium py-4 pl-6">
                    <div className="flex items-center gap-3">
                      <div className="w-8 h-8 rounded bg-primary/10 flex items-center justify-center text-primary">
                        <Globe className="w-4 h-4" />
                      </div>
                      <div className="flex flex-col">
                        <span>{domain.domain}</span>
                        <a 
                          href={`https://${domain.domain}`} 
                          target="_blank" 
                          rel="noopener noreferrer"
                          className="text-[10px] text-muted-foreground hover:text-primary flex items-center gap-1 w-fit"
                        >
                          {t('common.url')} <ExternalLink className="w-2.5 h-2.5" />
                        </a>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell>
                    {domain.project ? (
                      <Link 
                        to={`/projects/${domain.project.uid}`}
                        className="flex items-center gap-2 text-sm text-muted-foreground hover:text-primary transition-colors"
                      >
                        <FolderGit2 className="w-4 h-4" />
                        {domain.project.name}
                      </Link>
                    ) : (
                      <span className="text-xs text-destructive italic">{t('common.unassigned')}</span>
                    )}
                  </TableCell>
                  <TableCell>
                    <StatusBadge status={domain.status} />
                  </TableCell>
                  <TableCell className="pr-6">
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
