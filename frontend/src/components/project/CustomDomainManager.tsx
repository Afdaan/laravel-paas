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
import { Badge } from '@/components/ui/badge'
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

  const fetchDomains = useCallback(async () => {
    try {
      const res = await projectsAPI.listDomains(projectId)
      setDomains(res.data.data || [])
    } catch (error) {
      toast.error(t('common.error'))
    } finally {
      setIsLoading(false)
    }
  }, [projectId, t])

  useEffect(() => {
    fetchDomains()
  }, [fetchDomains])

  const handleAddDomain = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!newDomain.trim()) return

    setIsAdding(true)
    try {
      await projectsAPI.addDomain(projectId, newDomain.trim())
      setNewDomain('')
      fetchDomains()
      toast.success(t('common.success'))
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ message: string }>
      toast.error(axiosError.response?.data?.message || t('common.error'))
    } finally {
      setIsAdding(false)
    }
  }

  const handleRemoveDomain = async (domainId: number) => {
    try {
      await projectsAPI.removeDomain(projectId, domainId)
      fetchDomains()
      toast.success(t('common.success'))
    } catch (error) {
      toast.error(t('common.error'))
    }
  }

  const handleVerifyDomain = async (domainId: number) => {
    setVerifyingId(domainId)
    try {
      await projectsAPI.verifyDomain(projectId, domainId)
      toast.success(t('common.success'))
      fetchDomains()
    } catch (error: unknown) {
      const axiosError = error as AxiosError<{ error: { message: string } }>
      toast.error(axiosError.response?.data?.error?.message || t('common.error'))
      fetchDomains() // Refresh status even on error
    } finally {
      setVerifyingId(null)
    }
  }

  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <Label className="text-xs uppercase tracking-widest text-muted-foreground">{t('projectDetail.settings.customDomain') || 'Custom Domain'}</Label>
        <p className="text-[10px] text-muted-foreground leading-relaxed">
          {t('projectDetail.settings.customDomainDesc') || `Point your own domain to ${subdomain}.${window.location.hostname}`}
        </p>
      </div>

      <form onSubmit={handleAddDomain} className="flex gap-2">
        <Input
          value={newDomain}
          onChange={(e) => setNewDomain(e.target.value)}
          placeholder="e.g. www.my-awesome-site.com"
          className="h-10 text-xs flex-1"
          disabled={isAdding}
        />
        <Button 
          type="submit" 
          disabled={!newDomain.trim() || isAdding}
          className="gap-2 h-10"
        >
          {isAdding ? <RefreshCw className="w-4 h-4 animate-spin" /> : <Plus className="w-4 h-4" />}
          {t('common.add') || 'Add'}
        </Button>
      </form>

      {isLoading ? (
        <div className="flex justify-center p-4">
          <RefreshCw className="w-5 h-5 animate-spin text-muted-foreground" />
        </div>
      ) : domains.length > 0 ? (
        <div className="space-y-3">
          {domains.map((domain) => (
            <Card key={domain.id} className="bg-muted/30 border-muted-foreground/20">
              <CardContent className="p-4 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
                <div className="space-y-1 flex-1">
                  <div className="flex items-center gap-2">
                    <Globe className="w-4 h-4 text-muted-foreground" />
                    <span className="font-medium text-sm">{domain.domain}</span>
                  </div>
                  <div className="flex items-center gap-2">
                    {domain.status === 'active' ? (
                      <Badge variant="outline" className="bg-emerald-500/10 text-emerald-500 border-emerald-500/20 gap-1 text-[10px]">
                        <CheckCircle2 className="w-3 h-3" />
                        {t('status.active') || 'Active'}
                      </Badge>
                    ) : domain.status === 'error' ? (
                      <Badge variant="outline" className="bg-rose-500/10 text-rose-500 border-rose-500/20 gap-1 text-[10px]">
                        <XCircle className="w-3 h-3" />
                        {t('status.error') || 'Error'}
                      </Badge>
                    ) : (
                      <Badge variant="outline" className="bg-amber-500/10 text-amber-500 border-amber-500/20 gap-1 text-[10px]">
                        <AlertCircle className="w-3 h-3" />
                        {t('status.pending') || 'Pending'}
                      </Badge>
                    )}
                  </div>
                </div>

                <div className="flex items-center gap-2 w-full sm:w-auto">
                  {domain.status !== 'active' && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => handleVerifyDomain(domain.id)}
                      disabled={verifyingId === domain.id}
                      className="gap-2 text-xs flex-1 sm:flex-none"
                    >
                      {verifyingId === domain.id ? (
                        <RefreshCw className="w-3.5 h-3.5 animate-spin" />
                      ) : (
                        <RefreshCw className="w-3.5 h-3.5" />
                      )}
                      {t('common.verify') || 'Verify'}
                    </Button>
                  )}
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => handleRemoveDomain(domain.id)}
                    className="text-rose-500 hover:text-rose-600 hover:bg-rose-500/10"
                  >
                    <Trash2 className="w-4 h-4" />
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      ) : null}

      <div className="p-4 rounded-xl border border-blue-500/20 bg-blue-500/5 mt-4">
        <h4 className="text-xs font-bold text-blue-500 mb-2 flex items-center gap-2">
          <AlertCircle className="w-4 h-4" />
          DNS Configuration
        </h4>
        <p className="text-[11px] text-muted-foreground leading-relaxed">
          To connect your domain, configure your DNS provider (Cloudflare, GoDaddy, etc.) with a CNAME record pointing to <strong className="text-foreground">{subdomain}.{window.location.hostname}</strong>. After DNS propagation, click Verify.
        </p>
      </div>
    </div>
  )
}
