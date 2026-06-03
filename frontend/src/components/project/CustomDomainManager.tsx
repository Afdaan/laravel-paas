import { useState, useEffect, useCallback, useMemo } from 'react'
import { toast } from 'sonner'
import { Globe, Plus, Trash2, AlertCircle, RefreshCw, ExternalLink, Loader2, Activity, Terminal, FileText } from 'lucide-react'
import useTranslation from '@/lib/useTranslation'
import { projectsAPI } from '@/services/api'
import { CustomDomain, DomainEvent } from '@/types'
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
import ConfirmationModal from '@/components/ConfirmationModal'

interface CustomDomainManagerProps {
  projectId: number | string
  subdomain: string
  projectUrl?: string
  onDomainsChanged?: () => void
}

// Helper to calculate DNS Host (Dynamic & Supports Multi-part TLDs)
const getDNSHost = (domain: string): string => {
  const parts = domain.split('.')
  if (parts.length === 1) return domain
  if (parts.length === 2) return '@'
  
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
    <div className="flex items-center gap-2">
      <Badge 
        variant="outline" 
        className={`gap-2 items-center flex w-fit text-[9px] uppercase font-bold tracking-wider px-2 py-0.5 rounded-full border ${bgColor} ${textColor}`}
      >
        <span className={`w-1 h-1 rounded-full ${dotColor}`} />
        {label}
      </Badge>
      {!isHealthy && error && (
        <span className="text-[10px] text-rose-500 font-medium truncate max-w-xs bg-rose-500/5 px-2 py-0.5 rounded border border-rose-500/10">{error}</span>
      )}
    </div>
  )
}

// Helper to translate raw database domain event types and technical messages into elegant, highly understandable, but technically sound explanations.
const getRefinedEvent = (eventType: string, rawMessage: string, t: (key: string, data?: Record<string, string | number>) => string) => {
  const type = (eventType || '').toLowerCase()
  const msg = (rawMessage || '').toLowerCase()

  // Default values
  let title = t(`domains.events.types.${type}.title`)
  let desc = t(`domains.events.types.${type}.desc`)

  // Fallback to auto-formatted event type if translation missing
  if (title === `domains.events.types.${type}.title`) {
    title = eventType.replace(/_/g, ' ')
  }
  if (desc === `domains.events.types.${type}.desc`) {
    desc = rawMessage
  }

  // Handle special healthcheck_failed nested messages with dynamic interpolation
  if (type === 'healthcheck_failed') {
    if (msg.includes('503') || msg.includes('502') || msg.includes('504')) {
      desc = t('domains.events.types.healthcheck_failed.httpDesc', { error: rawMessage })
    } else if (msg.includes('dns') || msg.includes('resolve') || msg.includes('layer 1')) {
      desc = t('domains.events.types.healthcheck_failed.dnsDesc')
    } else {
      desc = t('domains.events.types.healthcheck_failed.genericDesc', { error: rawMessage })
    }
  }

  return { title, desc }
}

export function CustomDomainManager({ projectId, subdomain, projectUrl, onDomainsChanged }: CustomDomainManagerProps) {
  const { t, language } = useTranslation()
  const [domains, setDomains] = useState<CustomDomain[]>([])

  // Helper to extract base domain and compute centralized CNAME target
  const getCentralCNAME = () => {
    const host = projectUrl ? projectUrl.replace('https://', '').replace('http://', '') : `${subdomain}.${window.location.hostname}`;
    const prefix = `${subdomain}.`;
    if (host.startsWith(prefix)) {
      return `cname.${host.substring(prefix.length)}`;
    }
    return `cname.${host}`;
  };

  const [isLoading, setIsLoading] = useState(true)
  const [newDomain, setNewDomain] = useState('')
  const [isAdding, setIsAdding] = useState(false)
  const [verifyingIds, setVerifyingIds] = useState<Record<number, boolean>>({})
  const [selectedDomainId, setSelectedDomainId] = useState<number | null>(null)

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

  const [confirmModal, setConfirmModal] = useState({
    isOpen: false,
    title: '',
    message: '',
    onConfirm: () => {},
  })

  const fetchDomains = useCallback(async () => {
    try {
      const res = await projectsAPI.listDomains(projectId)
      const domainList = res.data.data || []
      setDomains(prev => {
        return domainList.map((newD: CustomDomain) => {
          const existing = prev.find(d => d.id === newD.id)
          if (existing && existing.current_sequence != null && newD.current_sequence != null && newD.current_sequence < existing.current_sequence) {
            if (process.env.NODE_ENV === 'development') {
              console.log("Ignoring stale domain list fetch for", newD.id, newD.current_sequence, "vs existing", existing.current_sequence)
            }
            return existing
          }
          return newD
        })
      })
      
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



  // Real-time project-wide EventSource connection for live domain list state and audit log streaming
  useEffect(() => {
    if (!projectId) return;

    let eventSource: EventSource | null = null;
    let isSubscribed = true;
    let reconnectDelay = 1000;
    const maxReconnectDelay = 30000;
    let reconnectTimer: NodeJS.Timeout | null = null;

    const scheduleReconnect = () => {
      if (!isSubscribed) return;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      // Add randomized jitter to prevent herd effects on server restarts
      const jitter = Math.random() * 1000;
      reconnectTimer = setTimeout(() => {
        connectSSE();
        // Exponential backoff
        reconnectDelay = Math.min(reconnectDelay * 2, maxReconnectDelay);
      }, reconnectDelay + jitter);
    };

    const connectSSE = async () => {
      try {
        const token = localStorage.getItem('token') || '';
        const res = await fetch('/api/auth/stream-token', {
          method: 'POST',
          headers: { 'Authorization': `Bearer ${token}` }
        });
        if (!res.ok || !isSubscribed) {
          scheduleReconnect();
          return;
        }
        const data = await res.json();
        const streamToken = data.token;

        const sseUrl = `/api/projects/${projectId}/domains/events/stream?stream_token=${encodeURIComponent(streamToken)}`;
        eventSource = new EventSource(sseUrl);

        eventSource.onopen = () => {
          // Reset reconnect delay on successful connection
          reconnectDelay = 1000;
        };

        eventSource.addEventListener('domain_event', (e) => {
          try {
            const eventData = JSON.parse(e.data);
            const updatedDomainId = eventData.domain_id;
            if (!updatedDomainId) return;

            if (eventData.event_type === 'domain_transferred') {
              projectsAPI.listDomains(projectId).then((res) => {
                setDomains(res.data.data || [])
                onDomainsChanged?.()
              }).catch(() => {})
              return;
            }

            // Update real-time domain status in the list
            setDomains(prevDomains => {
              return prevDomains.map(d => {
                if (d.id === updatedDomainId) {
                  if (eventData.sequence_number != null && d.current_sequence != null && eventData.sequence_number < d.current_sequence) {
                    if (process.env.NODE_ENV === 'development') {
                      console.log("Ignoring stale SSE domain event for", d.id, eventData.sequence_number, "vs existing", d.current_sequence);
                    }
                    return d;
                  }
                  return {
                    ...d,
                    status: eventData.state_to || d.status,
                    error_code: eventData.error_code || d.error_code,
                    error_message: eventData.message || eventData.payload || d.error_message,
                    current_sequence: eventData.sequence_number != null ? eventData.sequence_number : d.current_sequence,
                  };
                }
                return d;
              });
            });
            onDomainsChanged?.();

            if (eventData.state_to === 'active' || eventData.state_to === 'ssl_active' || String(eventData.event_type || '').startsWith('healthcheck_')) {
              projectsAPI.listDomains(projectId).then((res) => {
                const domainList = res.data.data || []
                setDomains(prev => {
                  return domainList.map((newD: CustomDomain) => {
                    const existing = prev.find(d => d.id === newD.id)
                    if (existing && existing.current_sequence != null && newD.current_sequence != null && newD.current_sequence < existing.current_sequence) {
                      return existing
                    }
                    return newD
                  })
                })
                onDomainsChanged?.()
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
          if (process.env.NODE_ENV === 'development') {
            console.warn("Subscriber buffer overflow detected, initiating SSE reconnection");
          }
          eventSource?.close();
          scheduleReconnect();
        });

        eventSource.onerror = (err) => {
          if (process.env.NODE_ENV === 'development') {
            console.error("Project SSE connection error", err);
          }
          eventSource?.close();
          scheduleReconnect();
        };
      } catch (err) {
        scheduleReconnect();
      }
    };

    connectSSE();

    return () => {
      isSubscribed = false;
      if (reconnectTimer) clearTimeout(reconnectTimer);
      if (eventSource) eventSource.close();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [projectId]);

  // Helper to format and display error toasts with premium Title/Description layout
  const showErrorToast = useCallback((rawMessage: string) => {
    // Matches e.g. "[VERIFICATION_FAILED] DNS propagation..." -> group 1: "VERIFICATION_FAILED", group 2: "DNS propagation..."
    const match = rawMessage.match(/^\[([A-Z0-9_]+)\]\s*(.*)$/)
    if (match) {
      const code = match[1]
      const content = match[2]
      
      const title = code
        .toLowerCase()
        .split('_')
        .map(word => word.charAt(0).toUpperCase() + word.slice(1))
        .join(' ')
        
      toast.error(title, {
        description: content
      })
    } else {
      toast.error(t('common.error'), {
        description: rawMessage
      })
    }
  }, [t])

  const handleAddDomain = async (e: React.FormEvent) => {
    e.preventDefault()
    const trimmedDomain = newDomain.trim()
    if (!trimmedDomain) return

    // Strict FQDN formatting check to prevent config injections and malformed routing records
    const domainRegex = /^[a-zA-Z0-9.-]+$/
    if (!domainRegex.test(trimmedDomain) || trimmedDomain.startsWith('.') || trimmedDomain.endsWith('.') || trimmedDomain.startsWith('-') || trimmedDomain.endsWith('-') || trimmedDomain.includes('..')) {
      toast.error(t('domains.errors.invalidFormat') || 'Invalid Domain Format', {
        description: t('domains.errors.invalidCharacters') || 'Domain names can only contain letters, numbers, dots, and hyphens.'
      })
      return
    }

    setIsAdding(true)
    try {
      const res = await projectsAPI.addDomain(projectId, trimmedDomain)
      setNewDomain('')
      fetchDomains()
      onDomainsChanged?.()
      if (res.data?.data?.id) {
        setSelectedDomainId(res.data.data.id)
      }
      toast.success(t('domains.added') || 'Domain Added', {
        description: t('domains.addedDesc') || 'Your custom domain has been successfully registered.'
      })
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ error: string; code: string }>
      const errCode = axiosError.response?.data?.code
      let errorMsg = t('common.error')
      if (errCode && t(`domains.errors.${errCode}`) !== `domains.errors.${errCode}`) {
        errorMsg = t(`domains.errors.${errCode}`)
      } else if (axiosError.response?.data?.error) {
        errorMsg = axiosError.response.data.error
      }
      showErrorToast(errorMsg)
    } finally {
      setIsAdding(false)
    }
  }

  const handleRemoveDomain = (e: React.MouseEvent, domain: CustomDomain) => {
    e.stopPropagation()
    setConfirmModal({
      isOpen: true,
      title: t('domains.confirmDelete'),
      message: domain.domain,
      onConfirm: async () => {
        try {
          await projectsAPI.removeDomain(projectId, domain.id)
          fetchDomains()
          onDomainsChanged?.()
          if (selectedDomainId === domain.id) setSelectedDomainId(null)
          toast.success(t('domains.removed') || 'Domain Removed', {
            description: t('domains.removedDesc') || 'The custom domain has been successfully detached.'
          })
        } catch (error) {
          toast.error(t('common.error'), {
            description: t('domains.errors.removeFailed') || 'Failed to remove custom domain.'
          })
        }
      }
    })
  }

  const handleVerifyDomain = async (e: React.MouseEvent, domainId: number) => {
    e.stopPropagation()
    setVerifyingIds(prev => ({ ...prev, [domainId]: true }))

    const dom = domains.find(d => d.id === domainId)
    const isAlreadyActive = dom?.status === 'active' || dom?.status === 'ssl_active'

    try {
      const res = await projectsAPI.verifyDomain(projectId, domainId)
      if (res.data?.error) {
        showErrorToast(res.data.error.message || t('common.error'))
      } else {
        if (isAlreadyActive) {
          toast.success(t('domains.healthcheckTriggered') || 'Health Check Triggered', {
            description: t('domains.healthcheckTriggeredDesc') || 'Background health check has been initiated. Status will update shortly.'
          })
        } else {
          toast.success(t('domains.verified') || 'Domain Verified', {
            description: t('domains.verifiedDesc') || 'DNS configuration verified successfully.'
          })
        }
      }
      fetchDomains()
      onDomainsChanged?.()
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ error: { message: string } }>
      showErrorToast(axiosError.response?.data?.error?.message || t('common.error'))
      fetchDomains()
      onDomainsChanged?.()
    } finally {
      setVerifyingIds(prev => {
        const next = { ...prev }
        delete next[domainId]
        return next
      })
    }
  }

  const handleOpenEvents = async (e: React.MouseEvent, domain: CustomDomain) => {
    e.stopPropagation()
    setEventsModal({ isOpen: true, domain, events: [], isLoading: true })
    try {
      const res = await projectsAPI.getDomainEvents(projectId, domain.id)
      setEventsModal(prev => ({ ...prev, events: res.data.data || [], isLoading: false }))
    } catch (error) {
      toast.error(t('common.error'), {
        description: t('domains.errors.eventsFailed') || 'Failed to fetch domain event logs.'
      })
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

      const refined = getRefinedEvent(event.event_type, event.message || event.payload || '', t)

      return (
        <div key={event.id || i} className="relative pl-6 group" style={{ contentVisibility: 'auto', containIntrinsicSize: '0 120px' }}>
          {/* Timeline Dot */}
          <div className={`absolute -left-[7px] top-2.5 w-3.5 h-3.5 rounded-full border-2 bg-background group-hover:scale-125 transition-transform duration-200 shadow-sm ${
            event.event_type?.includes('recovered') || event.event_type?.includes('active') || event.event_type === 'healthcheck_recovered'
              ? 'border-emerald-500 group-hover:bg-emerald-500'
              : event.event_type?.includes('degraded') || event.event_type?.includes('failed') || event.event_type?.includes('error')
              ? 'border-rose-500 group-hover:bg-rose-500'
              : 'border-primary group-hover:bg-primary'
          }`} />
          
          <div className="space-y-2.5 bg-muted/20 p-4 rounded-xl border border-border/60 hover:bg-muted/30 transition-colors text-left shadow-sm">
            {/* Header: Title and Timestamp */}
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
                {refined.title}
              </span>
              <span className="text-[10px] text-muted-foreground font-mono">
                {formattedDate}
              </span>
            </div>

            {/* Clear, refined description for general users */}
            <p className="text-xs text-foreground/90 leading-relaxed font-sans font-medium">
              {refined.desc}
            </p>

            {/* Technical logs collapsed/shown in a structured, professional small monospace block */}
            {(event.message || event.payload) && (
              <div className="mt-2.5 space-y-1">
                <span className="text-[9px] uppercase tracking-wider font-bold text-muted-foreground/60 block font-mono">
                  {language === 'id' ? 'LOG TEKNIS ASLI' : 'RAW TECHNICAL LOG'}
                </span>
                <p className="text-[10px] font-mono text-muted-foreground leading-normal bg-background/50 p-2.5 rounded-lg border border-border/30 overflow-x-auto whitespace-pre-wrap break-words max-h-24 select-all">
                  {event.message || event.payload}
                </p>
              </div>
            )}

            {/* State Transition and errors */}
            <div className="flex flex-wrap items-center gap-x-4 gap-y-2 pt-1">
              {event.state_from && event.state_to && (
                <div className="flex items-center gap-2 text-[10px] font-mono font-medium">
                  <span className="text-muted-foreground">{t('domains.events.transition') || 'State Transition'}:</span>
                  <span className="px-1.5 py-0.5 rounded bg-muted border text-muted-foreground/80">{event.state_from}</span>
                  <span className="text-muted-foreground">→</span>
                  <span className="px-1.5 py-0.5 rounded bg-primary/10 text-primary border border-primary/20 font-bold">{event.state_to}</span>
                </div>
              )}
              {event.error_code && event.error_code !== 'none' && (
                <Badge variant="outline" className="text-rose-500 border-rose-500/30 bg-rose-500/10 text-[9px] font-mono px-2 py-0.2 font-bold">
                  {event.error_code}
                </Badge>
              )}
            </div>
          </div>
        </div>
      )
    })
  }, [eventsModal.events, t, language])

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
                  <span>{t('domains.events.title') || 'Domain Connection & SSL Setup Log'}</span>
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
                          {['active', 'ssl_active', 'degraded'].includes(domain.status) && (
                            <HealthBadge health={domain.health_status} error={domain.error_message || (domain.error_code !== 'none' ? domain.error_code : undefined)} />
                          )}
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
                      {isActive ? (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={(e) => handleVerifyDomain(e, domain.id)}
                          disabled={!!verifyingIds[domain.id]}
                          className="h-8 gap-2 text-xs transition-all text-muted-foreground hover:text-primary hover:bg-primary/10 border-border"
                          title="Trigger instant health check"
                        >
                          <RefreshCw className={`w-3 h-3 ${verifyingIds[domain.id] ? 'animate-spin' : ''}`} />
                          {verifyingIds[domain.id] ? t('domains.checking') : t('domains.checkHealth')}
                        </Button>
                      ) : (
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={(e) => handleVerifyDomain(e, domain.id)}
                          disabled={!!verifyingIds[domain.id]}
                          className="h-8 gap-2 text-xs transition-all text-emerald-500 border-emerald-500/30 bg-emerald-500/5 hover:bg-emerald-500/10 hover:border-emerald-500/50"
                        >
                          <RefreshCw className={`w-3 h-3 ${verifyingIds[domain.id] ? 'animate-spin' : ''}`} />
                          {verifyingIds[domain.id] ? t('domains.verifying') : t('common.verify')}
                        </Button>
                      )}
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={(e) => handleRemoveDomain(e, domain)}
                        className="h-8 w-8 p-0 text-muted-foreground hover:text-rose-500 hover:bg-rose-500/10 transition-colors cursor-pointer"
                        style={{ cursor: 'pointer' }}
                      >
                        <Trash2 className="w-4 h-4 cursor-pointer" style={{ cursor: 'pointer' }} />
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
                            const host = getDNSHost(domain.domain)
                            navigator.clipboard.writeText(host)
                            toast.success(t('common.copied'))
                          }}
                        >
                          <div className="flex items-center justify-between relative z-10">
                            <Label className="text-[10px] uppercase text-muted-foreground font-bold tracking-widest cursor-pointer group-hover/box:text-primary transition-colors">{t('common.host')}</Label>
                            <span className="text-[10px] text-primary font-bold opacity-0 group-hover/box:opacity-100 transition-all uppercase tracking-wider">{t('common.copy')}</span>
                          </div>
                          <div className="text-sm font-mono font-bold text-foreground truncate relative z-10 pr-6">
                            {getDNSHost(domain.domain)}
                          </div>
                          <Plus className="absolute -bottom-2 -right-2 w-12 h-12 text-primary/5 group-hover/box:text-primary/10 transition-colors rotate-12" />
                        </div>

                        <div 
                          className="space-y-2 p-4 rounded-2xl bg-background/60 border border-border transition-all hover:bg-primary/5 hover:border-primary/30 cursor-pointer group/box relative overflow-hidden shadow-sm"
                          onClick={(e) => {
                            e.stopPropagation()
                            const target = getCentralCNAME()
                            navigator.clipboard.writeText(target)
                            toast.success(t('common.copied'))
                          }}
                        >
                          <div className="flex items-center justify-between relative z-10">
                            <Label className="text-[10px] uppercase text-muted-foreground font-bold tracking-widest cursor-pointer group-hover/box:text-primary transition-colors">{t('common.value')}</Label>
                            <span className="text-[10px] text-primary font-bold opacity-0 group-hover/box:opacity-100 transition-all uppercase tracking-wider">{t('common.copy')}</span>
                          </div>
                          <div className="text-sm font-mono font-bold text-foreground truncate relative z-10 pr-6">
                            {getCentralCNAME()}
                          </div>
                          <RefreshCw className="absolute -bottom-2 -right-2 w-12 h-12 text-primary/5 group-hover/box:text-primary/10 transition-colors -rotate-12" />
                        </div>
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

      <ConfirmationModal 
        onClose={() => setConfirmModal(prev => ({ ...prev, isOpen: false }))}
        {...confirmModal}
        type="danger"
        confirmText={t('common.delete') || 'Delete'}
      />
    </div>
  )
}
