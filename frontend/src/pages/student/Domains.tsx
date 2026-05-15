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
        <DialogContent className="sm:max-w-[425px]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <ArrowRightLeft className="w-5 h-5 text-primary" />
              {t('domains.transfer')}
            </DialogTitle>
            <DialogDescription>
              {t('domains.transferDesc')}
            </DialogDescription>
          </DialogHeader>
          <div className="py-6 space-y-4">
             <div className="space-y-2">
                <label className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t('domains.domainName')}</label>
                <div className="p-3 bg-muted rounded-md font-mono text-sm">{transferModal.domain?.domain}</div>
             </div>
             <div className="space-y-2">
                <label className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t('domains.selectTarget')}</label>
                <Select 
                  value={transferModal.targetProjectId} 
                  onValueChange={(val) => setTransferModal(prev => ({ ...prev, targetProjectId: val || '' }))}
                >
                  <SelectTrigger>
                    <SelectValue placeholder={t('domains.selectTarget')} />
                  </SelectTrigger>
                  <SelectContent>
                    {projects
                      .filter(p => p.id !== transferModal.domain?.project_id)
                      .map(p => (
                        <SelectItem key={p.id} value={p.id.toString()}>
                          {p.name} ({p.subdomain})
                        </SelectItem>
                      ))
                    }
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
              <TableRow className="bg-muted/50 hover:bg-muted/50">
                <TableHead className="w-[40%] font-semibold">{t('domains.domainName')}</TableHead>
                <TableHead className="w-[30%] font-semibold">{t('domains.linkedProject')}</TableHead>
                <TableHead className="w-[15%] font-semibold">{t('common.status')}</TableHead>
                <TableHead className="w-[15%] text-right font-semibold">{t('common.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {domains.map((domain) => (
                <TableRow key={domain.id} className="group transition-colors">
                  <TableCell className="font-medium py-4">
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
                  <TableCell className="text-right">
                    <DropdownMenu>
                      <DropdownMenuTrigger className="h-8 w-8 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center rounded-md hover:bg-muted text-muted-foreground hover:text-foreground focus:outline-none focus:ring-2 focus:ring-primary/20">
                        <MoreVertical className="w-4 h-4" />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem 
                          className="gap-2"
                          onClick={() => setTransferModal({ isOpen: true, domain, targetProjectId: '' })}
                        >
                          <ArrowRightLeft className="w-4 h-4" />
                          {t('domains.transfer')}
                        </DropdownMenuItem>
                        <DropdownMenuItem 
                          className="gap-2 text-destructive focus:text-destructive"
                          onClick={() => handleRemove(domain)}
                        >
                          <Trash2 className="w-4 h-4" />
                          {t('common.delete')}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
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
