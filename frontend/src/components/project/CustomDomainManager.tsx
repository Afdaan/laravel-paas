import { useState, useEffect, useCallback } from 'react'
import { toast } from 'sonner'
import { Globe, Plus, Trash2, CheckCircle2, XCircle, AlertCircle, RefreshCw } from 'lucide-react'
import useTranslation from '@/lib/useTranslation'
import { projectsAPI } from '@/services/api'
import { CustomDomain } from '@/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Card, CardContent } from '@/components/ui/card'
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
  
  // Common multi-part TLDs (could be expanded or moved to a config file)
  const multiPartTLDs = ['co.id', 'my.id', 'ac.id', 'sch.id', 'biz.id', 'or.id', 'go.id', 'net.id', 'co.uk', 'org.uk', 'com.au']
  const lastTwo = parts.slice(-2).join('.')
  const rootPartsCount = multiPartTLDs.includes(lastTwo) ? 3 : 2
  
  return parts.length > rootPartsCount ? parts.slice(0, -rootPartsCount).join('.') : '@'
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
  const [diagnosticData, setDiagnosticData] = useState<Record<number, any>>({})

  const fetchDomains = useCallback(async () => {
    try {
      const res = await projectsAPI.listDomains(projectId)
      const domainList = res.data.data || []
      setDomains(domainList)
      
      // Auto-select first non-active domain for guidance if nothing selected
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

  // New: Auto-fetch diagnostic when a domain is selected
  useEffect(() => {
    if (selectedDomainId) {
      handleFetchDiagnostic(selectedDomainId)
    }
  }, [selectedDomainId])

  const handleFetchDiagnostic = async (domainId: number) => {
    setIsDiagnosing(domainId)
    try {
      const res = await projectsAPI.getDomainDiagnostic(projectId, domainId)
      setDiagnosticData((prev: Record<number, any>) => ({
        ...prev,
        [domainId]: res.data.data
      }))
    } catch (error) {
      console.error("Diagnostic failed", error)
    } finally {
      setIsDiagnosing(null)
    }
  }

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
      await projectsAPI.verifyDomain(projectId, domainId)
      toast.success(t('common.success'))
      fetchDomains()
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ error: { message: string } }>
      toast.error(axiosError.response?.data?.error?.message || t('common.error'))
      fetchDomains()
    } finally {
      setVerifyingId(null)
    }
  }

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <Label className="text-xs uppercase tracking-widest text-muted-foreground font-semibold">
          {t('projectDetail.settings.customDomain')}
        </Label>
        <p className="text-[10px] text-muted-foreground leading-relaxed">
          {t('projectDetail.settings.customDomainDesc')}
        </p>
      </div>

      <form onSubmit={handleAddDomain} className="flex gap-2">
        <Input
          value={newDomain}
          onChange={(e) => setNewDomain(e.target.value)}
          placeholder="e.g. www.my-awesome-site.com"
          className="h-10 text-xs flex-1 bg-background/50 border-muted-foreground/20 focus:border-primary/50 transition-all"
          disabled={isAdding}
        />
        <Button 
          type="submit" 
          disabled={!newDomain.trim() || isAdding}
          className="gap-2 h-10 px-6 shadow-lg shadow-primary/10 transition-all hover:scale-[1.02]"
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
            const isActive = domain.status === 'active'
            const isError = domain.status === 'error'

            return (
              <Card 
                key={domain.id} 
                className={`group border-muted-foreground/10 transition-all duration-300 cursor-pointer overflow-hidden ${
                  isSelected ? 'ring-1 ring-primary/30 bg-muted/40 shadow-xl' : 'bg-muted/10 hover:bg-muted/20'
                }`}
                onClick={() => setSelectedDomainId(isSelected ? null : domain.id)}
              >
                <CardContent className="p-0">
                  <div className="p-4 flex items-center justify-between gap-4">
                    <div className="flex items-center gap-3">
                      <div className={`p-2 rounded-lg transition-colors ${isSelected ? 'bg-primary/10 text-primary' : 'bg-background/50 text-muted-foreground group-hover:text-foreground'}`}>
                        <Globe className="w-4 h-4" />
                      </div>
                      <div className="space-y-0.5">
                        <span className="font-semibold text-sm tracking-tight">{domain.domain}</span>
                        <div className="flex items-center gap-2">
                          {isActive ? (
                            <div className="flex items-center gap-1 text-[10px] text-emerald-500 font-medium uppercase tracking-tighter">
                              <CheckCircle2 className="w-3 h-3" />
                              {t('common.active')}
                            </div>
                          ) : isError ? (
                            <div className="flex items-center gap-1 text-[10px] text-rose-500 font-medium uppercase tracking-tighter">
                              <XCircle className="w-3 h-3" />
                              {t('common.error')}
                            </div>
                          ) : (
                            <div className="flex items-center gap-1 text-[10px] text-amber-500 font-medium uppercase tracking-tighter">
                              <AlertCircle className="w-3 h-3" />
                              {t('status.pending')}
                            </div>
                          )}
                        </div>
                        <div className="text-[9px] text-muted-foreground/50 font-medium uppercase tracking-widest mt-1 pl-1">
                          Last sync: {domain.updated_at ? new Date(domain.updated_at).toLocaleTimeString() : 'Never'}
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-2">
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

                  {/* Verification Instruction Panel */}
                  <div 
                    className={`transition-all duration-300 ease-in-out border-t border-muted-foreground/5 bg-background/30 ${
                      isSelected ? 'max-h-[1000px] opacity-100' : 'max-h-0 opacity-0'
                    }`}
                  >
                    <div className="p-4 space-y-6">
                      {/* Instructions Header */}
                      <div className="flex items-center justify-between">
                        <div className="space-y-0.5">
                          <h4 className="text-xs font-bold uppercase tracking-widest text-foreground flex items-center gap-2">
                            <AlertCircle className="w-3 h-3 text-primary" />
                            DNS Configuration Guide
                          </h4>
                          <p className="text-[10px] text-muted-foreground">Add these records to your DNS provider (Cloudflare, GoDaddy, etc.)</p>
                        </div>
                        {diagnosticData[domain.id] && (
                          <div className={`px-2 py-1 rounded text-[9px] font-bold uppercase tracking-tighter ${diagnosticData[domain.id]?.is_match ? 'bg-emerald-500/10 text-emerald-500' : 'bg-amber-500/10 text-amber-500'}`}>
                            {diagnosticData[domain.id]?.is_match ? 'Configured' : 'Action Required'}
                          </div>
                        )}
                      </div>

                      <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                        <div className="space-y-1.5 p-3 rounded-xl bg-muted/30 border border-muted-foreground/5 transition-all hover:bg-muted/40">
                          <Label className="text-[9px] uppercase text-muted-foreground font-bold tracking-widest">{t('common.type')}</Label>
                          <div className="text-sm font-mono font-bold text-primary">CNAME</div>
                        </div>
                        
                        <div 
                          className="space-y-1.5 p-3 rounded-xl bg-primary/5 border border-primary/10 transition-all hover:bg-primary/10 cursor-pointer group/box relative overflow-hidden"
                          onClick={(e) => {
                            e.stopPropagation()
                            const host = diagnosticData[domain.id]?.expected_host || getDNSHost(domain.domain)
                            navigator.clipboard.writeText(host)
                            toast.success(t('common.copied'))
                          }}
                        >
                          <div className="flex items-center justify-between relative z-10">
                            <Label className="text-[9px] uppercase text-primary/70 font-bold tracking-widest cursor-pointer">{t('common.host')}</Label>
                            <span className="text-[9px] text-primary/40 font-medium opacity-0 group-hover/box:opacity-100 transition-opacity uppercase">{t('common.copy')}</span>
                          </div>
                          <div className="text-sm font-mono font-bold text-foreground truncate relative z-10">
                            {diagnosticData[domain.id]?.expected_host || getDNSHost(domain.domain)}
                          </div>
                          <div className="absolute top-0 right-0 p-1 opacity-10 group-hover/box:opacity-20 transition-opacity">
                            <Plus className="w-8 h-8 -mr-2 -mt-2" />
                          </div>
                        </div>

                        <div 
                          className="space-y-1.5 p-3 rounded-xl bg-primary/5 border border-primary/10 transition-all hover:bg-primary/10 cursor-pointer group/box relative overflow-hidden"
                          onClick={(e) => {
                            e.stopPropagation()
                            const target = diagnosticData[domain.id]?.expected_value || (projectUrl ? projectUrl.replace('https://', '').replace('http://', '') : `${subdomain}.${window.location.hostname}`)
                            navigator.clipboard.writeText(target)
                            toast.success(t('common.copied'))
                          }}
                        >
                          <div className="flex items-center justify-between relative z-10">
                            <Label className="text-[9px] uppercase text-primary/70 font-bold tracking-widest cursor-pointer">{t('common.value')}</Label>
                            <span className="text-[9px] text-primary/40 font-medium opacity-0 group-hover/box:opacity-100 transition-opacity uppercase">{t('common.copy')}</span>
                          </div>
                          <div className="text-sm font-mono font-bold text-foreground truncate relative z-10">
                            {diagnosticData[domain.id]?.expected_value || (projectUrl ? projectUrl.replace('https://', '').replace('http://', '') : `${subdomain}.${window.location.hostname}`)}
                          </div>
                          <div className="absolute top-0 right-0 p-1 opacity-10 group-hover/box:opacity-20 transition-opacity">
                            <RefreshCw className="w-8 h-8 -mr-2 -mt-2" />
                          </div>
                        </div>
                      </div>

                      {/* LIVE DIAGNOSTIC SECTION (MXToolbox Style) */}
                      <div className="space-y-3 pt-2 border-t border-muted-foreground/5">
                        <div className="flex items-center justify-between">
                          <div className="flex items-center gap-2 text-[10px] font-bold uppercase tracking-widest text-muted-foreground">
                            <div className={`w-1.5 h-1.5 rounded-full ${isDiagnosing === domain.id ? 'bg-primary animate-ping' : 'bg-primary/40'}`} />
                            Live DNS Diagnostic
                          </div>
                          <button 
                            onClick={(e) => {
                              e.stopPropagation();
                              handleFetchDiagnostic(domain.id);
                            }}
                            disabled={isDiagnosing === domain.id}
                            className="text-[9px] font-bold text-primary hover:underline uppercase tracking-tighter disabled:opacity-50"
                          >
                            {isDiagnosing === domain.id ? 'Scanning...' : 'Refresh Status'}
                          </button>
                        </div>

                        {diagnosticData[domain.id] ? (
                          <div className="space-y-2 rounded-xl bg-muted/20 border border-muted-foreground/5 p-4 overflow-hidden relative">
                            <div className="grid grid-cols-1 md:grid-cols-2 gap-4 relative z-10">
                              <div className="space-y-1">
                                <div className="text-[8px] uppercase text-muted-foreground font-bold tracking-widest">Current CNAME</div>
                                <div className={`text-xs font-mono font-medium ${diagnosticData[domain.id]?.current_cname ? 'text-foreground' : 'text-muted-foreground italic'}`}>
                                  {diagnosticData[domain.id]?.current_cname || 'None detected'}
                                </div>
                              </div>
                              <div className="space-y-1">
                                <div className="text-[8px] uppercase text-muted-foreground font-bold tracking-widest">Current A Records</div>
                                <div className="text-xs font-mono font-medium flex flex-wrap gap-1">
                                  {diagnosticData[domain.id]?.current_ips?.length ? diagnosticData[domain.id]?.current_ips.map((ip: string) => (
                                    <span key={ip} className="px-1.5 py-0.5 rounded bg-muted-foreground/10">{ip}</span>
                                  )) : <span className="text-muted-foreground italic">None detected</span>}
                                </div>
                              </div>
                            </div>

                            <div className={`mt-3 pt-3 border-t border-muted-foreground/5 flex items-start gap-2 text-[11px] ${diagnosticData[domain.id]?.is_match ? 'text-emerald-500' : 'text-amber-500'}`}>
                              {diagnosticData[domain.id]?.is_match ? <CheckCircle2 className="w-4 h-4 shrink-0" /> : <XCircle className="w-4 h-4 shrink-0" />}
                              <span className="font-medium leading-tight">{diagnosticData[domain.id]?.message}</span>
                            </div>
                            
                            {/* Ambient status glow */}
                            <div className={`absolute top-0 right-0 w-32 h-32 blur-[60px] opacity-10 rounded-full -mr-16 -mt-16 ${diagnosticData[domain.id]?.is_match ? 'bg-emerald-500' : 'bg-amber-500'}`} />
                          </div>
                        ) : (
                          <div className="flex flex-col items-center justify-center py-6 bg-muted/10 rounded-xl border border-dashed border-muted-foreground/10 text-center">
                            <RefreshCw className={`w-5 h-5 mb-2 text-muted-foreground/20 ${isDiagnosing === domain.id ? 'animate-spin' : ''}`} />
                            <p className="text-[10px] text-muted-foreground">Click 'Refresh Status' to scan live DNS records</p>
                          </div>
                        )}
                      </div>

                      <Button
                        variant="outline"
                        className="w-full h-auto py-4 bg-emerald-500/5 border-emerald-500/20 hover:bg-emerald-500/10 hover:border-emerald-500/40 text-emerald-500 transition-all duration-300 rounded-lg group overflow-hidden relative"
                        onClick={(e) => handleVerifyDomain(e, domain.id)}
                        disabled={verifyingId === domain.id}
                      >
                        <div className="relative z-10 flex flex-col items-center gap-1">
                          <div className="flex items-center gap-2">
                            <RefreshCw className={`w-3.5 h-3.5 ${verifyingId === domain.id ? 'animate-spin' : 'group-hover:rotate-180 transition-transform duration-700'}`} />
                            <span className="text-xs font-bold uppercase tracking-[0.2em]">{t('common.verify')}</span>
                          </div>
                          <span className="text-[9px] font-medium opacity-60 uppercase tracking-widest">Verify DNS propagation</span>
                        </div>
                        {verifyingId === domain.id && (
                          <div className="absolute inset-0 bg-emerald-500/10 animate-pulse" />
                        )}
                      </Button>
                    </div>
                  </div>
                </CardContent>
              </Card>
            )
          })}
        </div>
      ) : (
        <div className="flex flex-col items-center justify-center p-12 bg-muted/5 rounded-2xl border border-dashed border-muted-foreground/20 text-center space-y-3">
          <div className="p-3 bg-muted/10 rounded-full text-muted-foreground/40">
            <Globe className="w-8 h-8" />
          </div>
          <div className="space-y-1">
            <p className="text-sm font-medium text-muted-foreground">{t('projectDetail.settings.noCustomDomain') || 'No custom domains added yet'}</p>
            <p className="text-[11px] text-muted-foreground/60">{t('projectDetail.settings.addDomainPrompt') || 'Add a domain to access your project via a custom URL.'}</p>
          </div>
        </div>
      )}
    </div>
  )
}
