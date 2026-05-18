import { useState, useEffect, useCallback, useMemo } from 'react'
import { toast } from 'sonner'
import { Globe, Plus, Trash2, CheckCircle2, AlertCircle, RefreshCw, ExternalLink, ShieldCheck, Server, Clock, Loader2, Activity, Terminal, FileText } from 'lucide-react'
import useTranslation from '@/lib/useTranslation'
import { projectsAPI } from '@/services/api'
import { CustomDomain, DomainDiagnostic, DomainEvent } from '@/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { AxiosError } from 'axios'

interface CustomDomainManagerProps {
  projectId: number | string
  subdomain: string
  projectUrl?: string
}

// Helper to calculate DNS Host (Dynamic & Supports Multi-part TLDs)
const getDNSHost = (domain: string): string => {
  const parts = domain.split('.')
  if (parts.length <= 2) return '@'
  
  const multiPartTLDs = ['co.id', 'my.id', 'ac.id', 'sch.id', 'biz.id', 'or.id', 'go.id', 'net.id', 'co.uk', 'org.uk', 'com.au']
  const lastTwo = parts.slice(-2).join('.')
  const rootPartsCount = multiPartTLDs.includes(lastTwo) ? 3 : 2
  
  return parts.length > rootPartsCount ? parts.slice(0, -rootPartsCount).join('.') : '@'
}

// Cached lightweight Date Formatter for audit logs to eliminate Intl instantiation lag in loops
const dateFormatter = new Intl.DateTimeFormat('en-US', {
  year: 'numeric',
  month: 'short',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false
})

const StatusBadge = ({ status }: { status: string }) => {
  const { t } = useTranslation()
  const cleanStatus = status || 'pending'
  const label = t(`domains.status.${cleanStatus}`) || cleanStatus

  let color = 'text-amber-500 border-amber-500/30 bg-amber-500/10'
  let Icon = Clock

  if (['active', 'ssl_active', 'dns_verified'].includes(cleanStatus)) {
    color = 'text-emerald-500 border-emerald-500/30 bg-emerald-500/10'
    Icon = CheckCircle2
  } else if (['ssl_queued', 'ssl_provisioning', 'renewal_pending'].includes(cleanStatus)) {
    color = 'text-cyan-500 border-cyan-500/30 bg-cyan-500/10'
    Icon = Loader2
  } else if (['error', 'degraded', 'renewal_failed'].includes(cleanStatus)) {
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
      {!isHealthy && error && (
        <span className="text-[10px] text-rose-500 font-medium truncate max-w-xs bg-rose-500/10 px-2.5 py-1 rounded-md border border-rose-500/20">{error}</span>
      )}
    </div>
  )
}

export function CustomDomainManager({ projectId, subdomain, projectUrl }: CustomDomainManagerProps) {
  const { t } = useTranslation()
  const [domains, setDomains] = useState<CustomDomain[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [newDomain, setNewDomain] = useState('')
  const [isAdding, setIsAdding] = useState(false)
  const [verifyingId, setVerifyingId] = useState<number | null>(null)
  const [selectedDomainId, setSelectedDomainId] = useState<number | null>(null)
  const [isDiagnosing, setIsDiagnosing] = useState<number | null>(null)
  const [diagnosticData, setDiagnosticData] = useState<Record<number, DomainDiagnostic>>({})
  
  const [eventsModal, setEventsModal] = useState<{
    isOpen: boolean;
    domain: CustomDomain | null;
    events: DomainEvent[];
    isLoading: boolean;
  }>({
    isOpen: false,
    domain: null,
    events: [],
    isLoading: false,
  })

  const fetchDomains = useCallback(async () => {
    try {
      const res = await projectsAPI.listDomains(projectId)
      const domainList = res.data.data || []
      setDomains(domainList)
      
      if (!selectedDomainId && domainList.length > 0) {
        const firstPending = domainList.find((d: CustomDomain) => d.status !== 'active')
        if (firstPending) setSelectedDomainId(firstPending.id)
      }
    } catch (error) {
      toast.error(t('common.error'))
    } finally {
      setIsLoading(false)
    }
  }, [projectId, t, selectedDomainId])

  useEffect(() => {
    fetchDomains()
  }, [fetchDomains])

  const handleFetchDiagnostic = useCallback(async (domainId: number) => {
    setIsDiagnosing(domainId)
    try {
      const res = await projectsAPI.getDomainDiagnostic(projectId, domainId)
      setDiagnosticData((prev: Record<number, DomainDiagnostic>) => ({
        ...prev,
        [domainId]: res.data.data
      }))
    } catch (error) {
      console.error("Diagnostic failed", error)
    } finally {
      setIsDiagnosing(null)
    }
  }, [projectId])

  useEffect(() => {
    if (selectedDomainId) {
      handleFetchDiagnostic(selectedDomainId)
    }
  }, [selectedDomainId, handleFetchDiagnostic])

  // Real-time project-wide EventSource connection for live domain list state and audit log streaming
  useEffect(() => {
    if (!projectId) return;

    let eventSource: EventSource | null = null;
    let isSubscribed = true;

    const connectSSE = async () => {
      try {
        const token = localStorage.getItem('token') || '';
        const res = await fetch('/api/auth/stream-token', {
          method: 'POST',
          headers: { 'Authorization': `Bearer ${token}` }
        });
        if (!res.ok || !isSubscribed) return;
        const data = await res.json();
        const streamToken = data.token;

        const sseUrl = `/api/projects/${projectId}/domains/events/stream?stream_token=${encodeURIComponent(streamToken)}`;
        eventSource = new EventSource(sseUrl);

        eventSource.addEventListener('domain_event', (e) => {
          try {
            const eventData = JSON.parse(e.data);
            const updatedDomainId = eventData.domain_id;
            if (!updatedDomainId) return;

            // Update real-time domain status in the list
            setDomains(prevDomains => {
              return prevDomains.map(d => {
                if (d.id === updatedDomainId) {
                  return {
                    ...d,
                    status: eventData.state_to || d.status,
                    error_code: eventData.error_code || d.error_code,
                    error_message: eventData.message || eventData.payload || d.error_message,
                  };
                }
                return d;
              });
            });

            if (eventData.state_to === 'active' || eventData.state_to === 'ssl_active' || String(eventData.event_type || '').startsWith('healthcheck_')) {
              projectsAPI.listDomains(projectId).then((res) => {
                setDomains(res.data.data || [])
              }).catch(() => {})
            }

            // If audit log modal is open for this domain, append live event
            setEventsModal(prev => {
              if (prev.domain && prev.domain.id === updatedDomainId) {
                if (prev.events.some(ev => ev.id === eventData.id)) return prev;
                return {
                  ...prev,
                  events: [eventData, ...prev.events]
                };
              }
              return prev;
            });
          } catch (err) {
            console.error("Failed to parse project SSE event", err);
          }
        });

        eventSource.addEventListener('overflow', () => {
          console.warn("Subscriber buffer overflow detected, initiating SSE reconnection");
          eventSource?.close();
          if (isSubscribed) setTimeout(connectSSE, 2000);
        });

        eventSource.onerror = (err) => {
          console.error("Project SSE connection error", err);
          eventSource?.close();
          if (isSubscribed) setTimeout(connectSSE, 5000);
        };
      } catch (err) {
        if (isSubscribed) setTimeout(connectSSE, 5000);
      }
    };

    connectSSE();

    return () => {
      isSubscribed = false;
      if (eventSource) eventSource.close();
    };
  }, [projectId]);

  const handleAddDomain = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newDomain.trim()) return

    setIsAdding(true)
    try {
      const res = await projectsAPI.addDomain(projectId, newDomain.trim())
      setNewDomain('')
      fetchDomains()
      if (res.data?.data?.id) {
        setSelectedDomainId(res.data.data.id)
      }
      toast.success(t('common.success'))
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ message: string }>
      toast.error(axiosError.response?.data?.message || t('common.error'))
    } finally {
      setIsAdding(false)
    }
  }

  const handleRemoveDomain = async (e: React.MouseEvent, domainId: number) => {
    e.stopPropagation()
    try {
      await projectsAPI.removeDomain(projectId, domainId)
      fetchDomains()
      if (selectedDomainId === domainId) setSelectedDomainId(null)
      toast.success(t('common.success'))
    } catch (error) {
      toast.error(t('common.error'))
    }
  }

  const handleVerifyDomain = async (e: React.MouseEvent, domainId: number) => {
    e.stopPropagation()
    setVerifyingId(domainId)
    try {
      const res = await projectsAPI.verifyDomain(projectId, domainId)
      if (res.data?.error) {
        toast.error(res.data.error.message || t('common.error'))
      } else {
        toast.success(t('common.success'))
      }
      fetchDomains()
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ error: { message: string } }>
      toast.error(axiosError.response?.data?.error?.message || t('common.error'))
      fetchDomains()
    } finally {
      setVerifyingId(null)
    }
  }

  const handleOpenEvents = async (e: React.MouseEvent, domain: CustomDomain) => {
    e.stopPropagation()
    setEventsModal({ isOpen: true, domain, events: [], isLoading: true })
    try {
      const res = await projectsAPI.getDomainEvents(projectId, domain.id)
      setEventsModal(prev => ({ ...prev, events: res.data.data || [], isLoading: false }))
    } catch (error) {
      toast.error(t('common.error'))
      setEventsModal(prev => ({ ...prev, isLoading: false }))
    }
  }

  // Memoize rendered events with virtualized content-visibility to eliminate DOM lag
  const renderedEvents = useMemo(() => {
    if (!eventsModal.events?.length) return null

    return eventsModal.events.map((event, i) => {
      let formattedDate = ''
      try {
        formattedDate = dateFormatter.format(new Date(event.created_at))
      } catch (e) {
        formattedDate = String(event.created_at)
      }

      return (
        <div key={event.id || i} className="relative pl-6 group" style={{ contentVisibility: 'auto', containIntrinsicSize: '0 120px' }}>
          <div className={`absolute -left-[7px] top-2.5 w-3.5 h-3.5 rounded-full border-2 bg-background group-hover:scale-125 transition-transform duration-200 shadow-sm ${
            event.event_type?.includes('recovered') || event.event_type?.includes('active') || event.event_type === 'healthcheck_recovered'
              ? 'border-emerald-500 group-hover:bg-emerald-500'
              : event.event_type?.includes('degraded') || event.event_type?.includes('failed') || event.event_type?.includes('error')
              ? 'border-rose-500 group-hover:bg-rose-500'
              : 'border-primary group-hover:bg-primary'
          }`} />
          <div className="space-y-2 bg-muted/20 p-4 rounded-xl border border-border/60 hover:bg-muted/30 transition-colors text-left shadow-sm">
            <div className="flex items-center justify-between gap-2">
              <span className={`text-xs font-bold flex items-center gap-2 ${
                event.event_type?.includes('recovered') || event.event_type?.includes('active') || event.event_type === 'healthcheck_recovered'
                  ? 'text-emerald-400'
                  : event.event_type?.includes('degraded') || event.event_type?.includes('failed') || event.event_type?.includes('error')
                  ? 'text-rose-400'
                  : 'text-foreground'
              }`}>
                <Terminal className={`w-3.5 h-3.5 ${
                  event.event_type?.includes('recovered') || event.event_type?.includes('active') || event.event_type === 'healthcheck_recovered'
                    ? 'text-emerald-400'
                    : event.event_type?.includes('degraded') || event.event_type?.includes('failed') || event.event_type?.includes('error')
                    ? 'text-rose-400'
                    : 'text-primary'
                }`} />
                {event.event_type}
              </span>
              <span className="text-[10px] text-muted-foreground font-mono">
                {formattedDate}
              </span>
            </div>
            <p className="text-xs font-mono text-muted-foreground leading-relaxed bg-background/60 p-3 rounded-lg border border-border/40 overflow-x-auto whitespace-pre-wrap break-words max-h-40">{event.message || event.payload}</p>
            {event.state_from && event.state_to && (
              <div className="flex items-center gap-2 pt-1 text-[10px] font-mono font-medium">
                <span className="text-muted-foreground">{t('domains.events.transition') || 'State Transition'}:</span>
                <span className="px-2 py-0.5 rounded bg-muted/50 border text-muted-foreground">{event.state_from}</span>
                <span className="text-muted-foreground">→</span>
                <span className="px-2 py-0.5 rounded bg-primary/10 text-primary border border-primary/20 font-bold">{event.state_to}</span>
              </div>
            )}
            {event.error_code && event.error_code !== 'none' && (
              <div className="pt-1">
                <Badge variant="outline" className="text-rose-500 border-rose-500/30 bg-rose-500/10 text-[10px] font-mono px-2.5 py-0.5 font-bold">
                  {event.error_code}
                </Badge>
              </div>
            )}
          </div>
        </div>
      )
    })
  }, [eventsModal.events, t])

  return (
    <div className="space-y-6">
      {/* Domain Events / Audit Log Modal */}
      <Dialog open={eventsModal.isOpen} onOpenChange={(open) => setEventsModal(prev => ({ ...prev, isOpen: open }))}>
        <DialogContent className="sm:max-w-[680px] max-h-[85vh] flex flex-col overflow-hidden bg-card/98 backdrop-blur-md border-border/60 p-0 shadow-2xl rounded-2xl sm:rounded-3xl">
          <DialogHeader className="p-6 pb-5 border-b bg-muted/30 relative">
            <div className="flex items-start gap-4 pr-10">
              <div className="w-11 h-11 rounded-2xl bg-primary/10 flex shrink-0 items-center justify-center text-primary border border-primary/20 shadow-inner mt-0.5">
                <Activity className="w-5 h-5" />
              </div>
              <div className="space-y-1.5 text-left">
                <DialogTitle className="text-base font-bold text-foreground flex flex-wrap items-center gap-3">
                  <span>{t('domains.events.title') || 'Reconciliation Audit Log'}</span>
                  <div className="flex items-center gap-1.5 text-[10px] text-emerald-500 font-mono bg-emerald-500/10 border border-emerald-500/20 px-2.5 py-0.5 rounded-full tracking-wider font-bold shadow-sm">
                    <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
                    LIVE STREAM
                  </div>
                </DialogTitle>
                <DialogDescription className="text-xs text-muted-foreground leading-normal max-w-lg">
                  <span className="font-semibold text-foreground">{eventsModal.domain?.domain}</span> • {t('domains.events.desc') || 'Chronological registry of all domain state transitions'}
                </DialogDescription>
              </div>
            </div>
          </DialogHeader>
          <div className="flex-1 overflow-y-auto p-6 space-y-4">
            {eventsModal.isLoading ? (
              <div className="flex flex-col items-center justify-center py-20 gap-3">
                <Loader2 className="w-8 h-8 text-primary animate-spin" />
                <span className="text-xs text-muted-foreground font-semibold uppercase tracking-wider">{t('common.loading')}</span>
              </div>
            ) : eventsModal.events.length === 0 ? (
              <div className="text-center py-16 bg-muted/10 rounded-2xl border border-dashed border-border text-muted-foreground/60 text-xs">
                {t('domains.events.noEvents') || 'No audit events logged yet.'}
              </div>
            ) : (
              <div className="relative border-l-2 border-muted/60 ml-4 space-y-6 py-2">
                {renderedEvents}
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>

      <div className="space-y-1">
        <Label className="text-xs uppercase tracking-wider text-muted-foreground font-bold">
          {t('projectDetail.settings.customDomain')}
        </Label>
        <p className="text-xs text-muted-foreground leading-relaxed">
          {t('projectDetail.settings.customDomainDesc')}
        </p>
      </div>

      <form onSubmit={handleAddDomain} className="flex flex-col sm:flex-row gap-3">
        <div className="relative flex-1">
          <Globe className="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground/60" />
          <Input
            value={newDomain}
            onChange={(e) => setNewDomain(e.target.value)}
            placeholder="e.g. www.my-awesome-site.com"
            className="h-11 pl-10 text-sm bg-background/50 border-muted-foreground/20 focus:border-primary/50 transition-all rounded-xl shadow-sm"
            disabled={isAdding}
          />
        </div>
        <Button 
          type="submit" 
          disabled={!newDomain.trim() || isAdding}
          className="gap-2 h-11 px-6 shadow-lg shadow-primary/10 transition-all hover:scale-[1.02] rounded-xl font-semibold"
        >
          {isAdding ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Plus className="w-4 h-4" />}
          {t('common.add')}
        </Button>
      </form>

      {isLoading ? (
        <div className="flex justify-center p-8">
          <RefreshCw className="w-6 h-6 animate-spin text-primary/40" />
        </div>
      ) : domains.length > 0 ? (
        <div className="space-y-3">
          {domains.map((domain) => {
            const isSelected = selectedDomainId === domain.id
            const isActive = domain.status === 'active' || domain.status === 'ssl_active'

            return (
              <Card 
                key={domain.id} 
                className={`group border-muted-foreground/10 transition-all duration-300 cursor-pointer overflow-hidden ${
                  isSelected ? 'ring-1 ring-primary/30 bg-muted/40 shadow-xl' : 'bg-muted/10 hover:bg-muted/20'
                }`}
                onClick={() => setSelectedDomainId(isSelected ? null : domain.id)}
              >
                <CardContent className="p-0">
                  <div className="p-5 flex items-center justify-between gap-4">
                    <div className="flex items-center gap-4">
                      <div className={`p-3.5 rounded-2xl transition-all border ${isSelected ? 'bg-primary/10 text-primary border-primary/20 shadow-sm' : 'bg-background text-muted-foreground border-border group-hover:text-foreground'}`}>
                        <Globe className="w-5 h-5" />
                      </div>
                      <div className="space-y-2">
                        <div className="flex items-center gap-3">
                          <span className="font-bold text-base tracking-tight text-foreground">{domain.domain}</span>
                          {isActive && (
                            <a 
                              href={`https://${domain.domain}`} 
                              target="_blank" 
                              rel="noopener noreferrer"
                              onClick={(e) => e.stopPropagation()}
                              className="inline-flex h-7 w-7 items-center justify-center rounded-lg text-muted-foreground hover:text-primary hover:bg-primary/10 transition-all border border-transparent hover:border-primary/20"
                              title="Visit Domain"
                            >
                              <ExternalLink className="w-4 h-4" />
                            </a>
                          )}
                          {domain.config_hash && (
                            <span className="text-[10px] font-mono text-muted-foreground/70 bg-muted/60 px-2.5 py-0.5 rounded-md border border-border">
                              SHA256:{domain.config_hash.substring(0, 8)}
                            </span>
                          )}
                        </div>
                        <div className="flex flex-wrap items-center gap-2">
                          <StatusBadge status={domain.status} />
                          <HealthBadge health={domain.health_status} error={domain.error_message || (domain.error_code !== 'none' ? domain.error_code : undefined)} />
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-2">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={(e) => handleOpenEvents(e, domain)}
                        className="h-8 gap-1.5 text-xs text-muted-foreground hover:text-primary hover:bg-primary/10"
                      >
                        <FileText className="w-3.5 h-3.5" />
                        <span className="hidden sm:inline">Audit Log</span>
                      </Button>
                      {!isActive && (
                        <Button
                          variant={isSelected ? "default" : "outline"}
                          size="sm"
                          onClick={(e) => handleVerifyDomain(e, domain.id)}
                          disabled={verifyingId === domain.id}
                          className={`h-8 gap-2 text-xs transition-all ${isSelected ? 'shadow-lg shadow-primary/20' : ''}`}
                        >
                          <RefreshCw className={`w-3 h-3 ${verifyingId === domain.id ? 'animate-spin' : ''}`} />
                          {t('common.verify')}
                        </Button>
                      )}
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={(e) => handleRemoveDomain(e, domain.id)}
                        className="h-8 w-8 p-0 text-muted-foreground hover:text-rose-500 hover:bg-rose-500/10 transition-colors"
                      >
                        <Trash2 className="w-4 h-4" />
                      </Button>
                    </div>
                  </div>

                  <div 
                    className={`transition-all duration-300 ease-in-out border-t border-muted-foreground/5 bg-background/30 ${
                      isSelected ? 'max-h-[1000px] opacity-100' : 'max-h-0 opacity-0'
                    }`}
                  >
                    <div className="p-4 space-y-6">
                      <div className="flex items-center justify-between">
                        <div className="space-y-1">
                          <h4 className="text-xs font-bold uppercase tracking-widest text-foreground flex items-center gap-2">
                            <AlertCircle className="w-3.5 h-3.5 text-primary" />
                            {t('domains.dnsGuide.title') || 'DNS Configuration Guide'}
                          </h4>
                          <p className="text-xs text-muted-foreground">{t('domains.dnsGuide.desc') || 'Add these records to your DNS provider (Cloudflare, GoDaddy, etc.)'}</p>
                        </div>
                        {diagnosticData[domain.id] && (
                          <div className={`px-3 py-1 rounded-lg text-[10px] font-bold uppercase tracking-wider border ${diagnosticData[domain.id]?.is_match ? 'bg-emerald-500/10 text-emerald-500 border-emerald-500/20' : 'bg-amber-500/10 text-amber-500 border-amber-500/20'}`}>
                            {diagnosticData[domain.id]?.is_match ? (t('domains.dnsGuide.configured') || 'Configured') : (t('domains.dnsGuide.actionRequired') || 'Action Required')}
                          </div>
                        )}
                      </div>

                      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                        <div className="space-y-2 p-4 rounded-2xl bg-background/60 border border-border transition-all hover:border-border/80 shadow-sm">
                          <Label className="text-[10px] uppercase text-muted-foreground font-bold tracking-widest">{t('common.type')}</Label>
                          <div className="text-sm font-mono font-bold text-primary bg-primary/10 w-fit px-2.5 py-1 rounded-md border border-primary/20">CNAME</div>
                        </div>
                        
                        <div 
                          className="space-y-2 p-4 rounded-2xl bg-background/60 border border-border transition-all hover:bg-primary/5 hover:border-primary/30 cursor-pointer group/box relative overflow-hidden shadow-sm"
                          onClick={(e) => {
                            e.stopPropagation()
                            const host = diagnosticData[domain.id]?.expected_host || getDNSHost(domain.domain)
                            navigator.clipboard.writeText(host)
                            toast.success(t('common.copied'))
                          }}
                        >
                          <div className="flex items-center justify-between relative z-10">
                            <Label className="text-[10px] uppercase text-muted-foreground font-bold tracking-widest cursor-pointer group-hover/box:text-primary transition-colors">{t('common.host')}</Label>
                            <span className="text-[10px] text-primary font-bold opacity-0 group-hover/box:opacity-100 transition-all uppercase tracking-wider">{t('common.copy')}</span>
                          </div>
                          <div className="text-sm font-mono font-bold text-foreground truncate relative z-10 pr-6">
                            {diagnosticData[domain.id]?.expected_host || getDNSHost(domain.domain)}
                          </div>
                          <Plus className="absolute -bottom-2 -right-2 w-12 h-12 text-primary/5 group-hover/box:text-primary/10 transition-colors rotate-12" />
                        </div>

                        <div 
                          className="space-y-2 p-4 rounded-2xl bg-background/60 border border-border transition-all hover:bg-primary/5 hover:border-primary/30 cursor-pointer group/box relative overflow-hidden shadow-sm"
                          onClick={(e) => {
                            e.stopPropagation()
                            const target = diagnosticData[domain.id]?.expected_value || (projectUrl ? projectUrl.replace('https://', '').replace('http://', '') : `${subdomain}.${window.location.hostname}`)
                            navigator.clipboard.writeText(target)
                            toast.success(t('common.copied'))
                          }}
                        >
                          <div className="flex items-center justify-between relative z-10">
                            <Label className="text-[10px] uppercase text-muted-foreground font-bold tracking-widest cursor-pointer group-hover/box:text-primary transition-colors">{t('common.value')}</Label>
                            <span className="text-[10px] text-primary font-bold opacity-0 group-hover/box:opacity-100 transition-all uppercase tracking-wider">{t('common.copy')}</span>
                          </div>
                          <div className="text-sm font-mono font-bold text-foreground truncate relative z-10 pr-6">
                            {diagnosticData[domain.id]?.expected_value || (projectUrl ? projectUrl.replace('https://', '').replace('http://', '') : `${subdomain}.${window.location.hostname}`)}
                          </div>
                          <RefreshCw className="absolute -bottom-2 -right-2 w-12 h-12 text-primary/5 group-hover/box:text-primary/10 transition-colors -rotate-12" />
                        </div>
                      </div>

                      <div className="space-y-6 pt-4 border-t border-border/60">
                        <div className="flex items-center justify-between px-1">
                          <div className="flex items-center gap-2.5">
                            <div className="relative flex h-2 w-2">
                              <span className={`animate-ping absolute inline-flex h-full w-full rounded-full opacity-75 ${isDiagnosing === domain.id ? 'bg-primary' : 'bg-primary/20'}`}></span>
                              <span className={`relative inline-flex rounded-full h-2 w-2 ${isDiagnosing === domain.id ? 'bg-primary' : 'bg-primary/40'}`}></span>
                            </div>
                            <span className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground/80">{t('domains.dnsGuide.networkPath') || 'Network Path Integrity'}</span>
                          </div>
                          <button 
                            onClick={(e) => {
                              e.stopPropagation();
                              handleFetchDiagnostic(domain.id);
                            }}
                            disabled={isDiagnosing === domain.id}
                            className="text-[10px] font-bold text-primary hover:text-primary/80 transition-colors uppercase tracking-widest disabled:opacity-50 flex items-center gap-1.5 cursor-pointer"
                          >
                            <RefreshCw className={`w-3 h-3 ${isDiagnosing === domain.id ? 'animate-spin' : ''}`} />
                            {isDiagnosing === domain.id ? (t('domains.dnsGuide.scanning') || 'Scanning...') : (t('domains.dnsGuide.rescan') || 'Re-scan')}
                          </button>
                        </div>

                        {diagnosticData[domain.id] ? (
                          <div className="relative py-8 px-4 rounded-3xl bg-muted/5 border border-muted-foreground/5 overflow-hidden">
                            <div className="absolute top-1/2 left-0 w-full h-[1px] bg-muted-foreground/10 -translate-y-1/2" />
                            
                            <div className="flex items-center justify-between relative z-10 max-w-sm mx-auto">
                              <div className="flex flex-col items-center gap-3">
                                <div className={`w-12 h-12 rounded-2xl flex items-center justify-center border transition-all duration-500 ${isDiagnosing === domain.id ? 'bg-primary/10 border-primary/30 animate-pulse' : 'bg-background border-muted-foreground/10'}`}>
                                  <Globe className={`w-5 h-5 ${isDiagnosing === domain.id ? 'text-primary' : 'text-muted-foreground'}`} />
                                </div>
                                <span className="text-[9px] font-bold uppercase tracking-wider text-muted-foreground">{t('domains.dnsGuide.domain') || 'Domain'}</span>
                              </div>

                              <div className="flex-1 px-2 relative">
                                <div className={`h-[2px] w-full rounded-full overflow-hidden ${diagnosticData[domain.id]?.current_cname ? 'bg-emerald-500/20' : 'bg-muted-foreground/10'}`}>
                                  <div className={`h-full bg-emerald-500 transition-all duration-[2000ms] ${diagnosticData[domain.id]?.current_cname ? 'w-full' : 'w-0'}`} />
                                </div>
                              </div>

                              <div className="flex flex-col items-center gap-3">
                                <div className={`w-12 h-12 rounded-2xl flex items-center justify-center border transition-all duration-500 ${diagnosticData[domain.id]?.current_cname ? 'bg-emerald-500/10 border-emerald-500/30' : 'bg-background border-muted-foreground/10'}`}>
                                  <div className="relative">
                                    <ShieldCheck className={`w-5 h-5 ${diagnosticData[domain.id]?.current_cname ? 'text-emerald-500' : 'text-muted-foreground'}`} />
                                    {diagnosticData[domain.id]?.current_cname && <span className="absolute -top-1 -right-1 flex h-2 w-2">
                                      <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                                      <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
                                    </span>}
                                  </div>
                                </div>
                                <span className="text-[9px] font-bold uppercase tracking-wider text-muted-foreground">{t('domains.dnsGuide.dnsOk') || 'DNS OK'}</span>
                              </div>

                              <div className="flex-1 px-2">
                                <div className={`h-[2px] w-full rounded-full overflow-hidden ${diagnosticData[domain.id]?.is_match ? 'bg-emerald-500/20' : 'bg-muted-foreground/10'}`}>
                                  <div className={`h-full bg-emerald-500 transition-all duration-[2000ms] delay-500 ${diagnosticData[domain.id]?.is_match ? 'w-full' : 'w-0'}`} />
                                </div>
                              </div>

                              <div className="flex flex-col items-center gap-3">
                                <div className={`w-12 h-12 rounded-2xl flex items-center justify-center border transition-all duration-500 ${diagnosticData[domain.id]?.is_match ? 'bg-emerald-500/20 border-emerald-500/50 shadow-[0_0_20px_rgba(16,185,129,0.2)]' : 'bg-background border-muted-foreground/10'}`}>
                                  <Server className={`w-5 h-5 ${diagnosticData[domain.id]?.is_match ? 'text-emerald-500' : 'text-muted-foreground'}`} />
                                </div>
                                <span className="text-[9px] font-bold uppercase tracking-wider text-muted-foreground">{t('domains.dnsGuide.edge') || 'PaaS Edge'}</span>
                              </div>
                            </div>

                            <div className="mt-6 text-center space-y-1">
                              <div className={`text-[10px] font-black uppercase tracking-[0.2em] ${diagnosticData[domain.id]?.is_match ? 'text-emerald-500' : 'text-amber-500'}`}>
                                {diagnosticData[domain.id]?.is_match ? (t('domains.dnsGuide.statusActive') || 'STATUS: ACTIVE') : (t('domains.dnsGuide.statusPending') || 'STATUS: PENDING')}
                              </div>
                              <p className="text-[10px] text-muted-foreground/60 max-w-[200px] mx-auto leading-tight">{diagnosticData[domain.id]?.message}</p>
                            </div>
                          </div>
                        ) : (
                          <div className="flex flex-col items-center justify-center py-12 bg-muted/5 rounded-[2rem] border border-dashed border-muted-foreground/10 text-center group/empty hover:bg-muted/10 transition-all">
                            <div className="relative mb-4">
                              <div className="p-4 bg-muted/10 rounded-2xl group-hover/empty:scale-110 transition-transform">
                                <RefreshCw className={`w-6 h-6 text-muted-foreground/20 ${isDiagnosing === domain.id ? 'animate-spin' : ''}`} />
                              </div>
                            </div>
                            <p className="text-[11px] font-bold text-muted-foreground/40 uppercase tracking-[0.25em]">{t('domains.dnsGuide.initScan') || 'Initialize System Scan'}</p>
                          </div>
                        )}
                      </div>

                      <div className="flex justify-center pt-2">
                        <Button
                          variant="outline"
                          className="w-fit h-auto py-3 px-10 bg-emerald-500/5 border-emerald-500/20 hover:bg-emerald-500/10 hover:border-emerald-500/40 text-emerald-500 transition-all duration-500 rounded-xl group overflow-hidden relative"
                          onClick={(e) => handleVerifyDomain(e, domain.id)}
                          disabled={verifyingId === domain.id}
                        >
                          <div className="relative z-10 flex items-center gap-3">
                            <div className={`p-1 rounded-full bg-emerald-500/10 group-hover:bg-emerald-500/20 transition-colors ${verifyingId === domain.id ? 'animate-spin' : ''}`}>
                              <RefreshCw className="w-3.5 h-3.5" />
                            </div>
                            <span className="text-[11px] font-black uppercase tracking-[0.3em]">{t('common.verify')}</span>
                          </div>
                        </Button>
                      </div>
                    </div>
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center p-16 bg-muted/5 rounded-[2rem] border border-dashed border-muted-foreground/10 text-center space-y-4">
          <div className="p-4 bg-muted/10 rounded-3xl text-muted-foreground/20">
            <Globe className="w-10 h-10" />
          </div>
          <div className="space-y-1.5">
            <h3 className="text-sm font-bold text-muted-foreground tracking-tight">{t('projectDetail.settings.noCustomDomain') || 'No custom domains added yet'}</h3>
            <p className="text-[11px] text-muted-foreground/50 max-w-[200px] leading-relaxed">{t('projectDetail.settings.addDomainPrompt') || 'Add a domain to access your project via a custom URL.'}</p>
          </div>
        </div>
      )}
    </div>
  )
}
