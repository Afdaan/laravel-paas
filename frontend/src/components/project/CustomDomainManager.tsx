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
}

export function CustomDomainManager({ projectId, subdomain }: CustomDomainManagerProps) {
  const { t } = useTranslation()
  const [domains, setDomains] = useState<CustomDomain[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [newDomain, setNewDomain] = useState('')
  const [isAdding, setIsAdding] = useState(false)
  const [verifyingId, setVerifyingId] = useState<number | null>(null)
  const [selectedDomainId, setSelectedDomainId] = useState<number | null>(null)

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
                              {t('status.active')}
                            </div>
                          ) : isError ? (
                            <div className="flex items-center gap-1 text-[10px] text-rose-500 font-medium uppercase tracking-tighter">
                              <XCircle className="w-3 h-3" />
                              {t('status.error')}
                            </div>
                          ) : (
                            <div className="flex items-center gap-1 text-[10px] text-amber-500 font-medium uppercase tracking-tighter">
                              <AlertCircle className="w-3 h-3" />
                              {t('status.pending')}
                            </div>
                          )}
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
                      isSelected ? 'max-h-[500px] opacity-100' : 'max-h-0 opacity-0'
                    }`}
                  >
                    <div className="p-4 space-y-4">
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                        <div className="space-y-1.5 p-3 rounded-lg bg-muted/30 border border-muted-foreground/5 transition-all hover:bg-muted/40">
                          <Label className="text-[10px] uppercase text-muted-foreground font-bold tracking-widest">{t('common.type')}</Label>
                          <div className="text-sm font-mono font-bold text-primary">CNAME</div>
                        </div>
                        <div className="space-y-1.5 p-3 rounded-lg bg-muted/30 border border-muted-foreground/5 transition-all hover:bg-muted/40">
                          <Label className="text-[10px] uppercase text-muted-foreground font-bold tracking-widest">{t('common.host')}</Label>
                          <div className="flex items-center justify-between">
                            <div className="text-sm font-mono font-bold text-foreground">
                              {(() => {
                                const parts = domain.domain.split('.')
                                if (parts.length > 2) {
                                  return parts.slice(0, -2).join('.')
                                }
                                return '@'
                              })()}
                            </div>
                            <Button 
                              variant="ghost" 
                              size="sm" 
                              className="h-6 px-2 text-[10px] hover:bg-primary/10"
                              onClick={(e) => {
                                e.stopPropagation()
                                const parts = domain.domain.split('.')
                                const host = parts.length > 2 ? parts.slice(0, -2).join('.') : '@'
                                navigator.clipboard.writeText(host)
                                toast.success(t('common.copied'))
                              }}
                            >
                              {t('common.copy')}
                            </Button>
                          </div>
                        </div>
                      </div>
                      
                      <div className="space-y-1.5 p-3 rounded-lg bg-primary/5 border border-primary/10 transition-all hover:bg-primary/10">
                        <Label className="text-[10px] uppercase text-primary/70 font-bold tracking-widest">{t('common.value')}</Label>
                        <div className="flex items-center justify-between">
                          <div className="text-sm font-mono font-bold text-foreground truncate pr-4">
                            {subdomain}.{window.location.hostname}
                          </div>
                          <Button 
                            variant="ghost" 
                            size="sm" 
                            className="h-6 px-2 text-[10px] hover:bg-primary/20"
                            onClick={(e) => {
                              e.stopPropagation()
                              navigator.clipboard.writeText(`${subdomain}.${window.location.hostname}`)
                              toast.success(t('common.copied'))
                            }}
                          >
                            {t('common.copy')}
                          </Button>
                        </div>
                      </div>

                      <div className="flex items-start gap-2 p-2 rounded bg-amber-500/5 text-[10px] text-muted-foreground leading-tight">
                        <RefreshCw className="w-3 h-3 mt-0.5 animate-pulse text-amber-500 shrink-0" />
                        <span>{t('projectDetail.settings.dnsPropagationDesc')}</span>
                      </div>
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
