import { useState, useEffect, useCallback } from 'react'
import { toast } from 'sonner'
import { settingsAPI } from '../../services/api'
import useTranslation from '../../lib/useTranslation'
import { 
  Globe, 
  Shield, 
  Activity, 
  Cpu, 
  Database, 
  Save, 
  AlertCircle,
  Server,
  Network,
  Clock,
  Layout,
  Zap,
  Loader2,
  RefreshCw
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Slider } from '@/components/ui/slider'
import { NumberStepper } from '@/components/ui/number-stepper'

interface PlatformSettings {
  base_domain?: string;
  project_domain?: string;
  max_projects_per_user?: number;
  project_expiry_days?: number;
  cpu_limit_percent?: number;
  memory_limit_mb?: number;
  admin_idle_timeout?: number;
  max_concurrent_builds?: number;
}

const AdminSettings = () => {
  const { t } = useTranslation()
  const [settings, setSettings] = useState<PlatformSettings>({})
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  
  const fetchSettings = useCallback(async () => {
    setIsLoading(true)
    try {
      const response = await settingsAPI.list()
      setSettings(response.data.map || {})
    } catch (error) {
      toast.error(t('admin.settings.loading'))
    } finally {
      setIsLoading(false)
    }
  }, [t])

  useEffect(() => {
    fetchSettings()
  }, [fetchSettings])
  
  const handleChange = (key: keyof PlatformSettings, value: any) => {
    setSettings(prev => ({ ...prev, [key]: value }))
  }
  
  const handleSave = async () => {
    setIsSaving(true)
    try {
      // Convert all values to strings as backend expects map[string]string
      const payload: Record<string, string> = {}
      Object.entries(settings).forEach(([key, value]) => {
        if (value !== undefined && value !== null) {
          payload[key] = String(value)
        }
      })
      
      await settingsAPI.update(payload)
      toast.success(t('admin.settings.success'))
    } catch (error) {
      toast.error(t('admin.settings.failed'))
    } finally {
      setIsSaving(false)
    }
  }
  
  if (isLoading) {
    return (
      <div className="flex flex-col items-center justify-center h-80 gap-6">
        <Loader2 className="w-12 h-12 text-primary animate-spin" />
        <p className="text-muted-foreground text-xs font-bold uppercase tracking-widest animate-pulse">{t('common.loading')}</p>
      </div>
    )
  }

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-20">
      <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6 pb-4 border-b">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">{t('admin.settings.title')}</h1>
          <p className="text-muted-foreground">{t('admin.settings.desc')}</p>
        </div>

        <Button
          onClick={handleSave}
          disabled={isSaving}
          size="lg"
          className="w-full xl:w-auto"
        >
          {isSaving ? <Loader2 className="w-4 h-4 mr-2 animate-spin" /> : <Save className="w-4 h-4 mr-2" />}
          {isSaving ? t('admin.settings.syncing') : t('admin.settings.deployChanges')}
        </Button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        {/* Domain Infrastructure */}
        <Card className="overflow-hidden relative group border-indigo-500/20 bg-gradient-to-br from-background to-indigo-500/5">
          <CardHeader className="flex flex-row items-center gap-4 pb-2">
            <div className="w-12 h-12 bg-indigo-500/10 rounded-xl flex items-center justify-center text-indigo-500">
              <Globe className="w-6 h-6" />
            </div>
            <div>
              <CardTitle className="text-xl">{t('admin.settings.domainTopology')}</CardTitle>
              <p className="text-xs text-muted-foreground font-semibold uppercase tracking-wider mt-1">{t('admin.settings.networkRouting')}</p>
            </div>
          </CardHeader>
          <CardContent className="space-y-6 pt-6">
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <Label className="flex items-center gap-2 text-xs uppercase tracking-wider text-muted-foreground font-semibold">
                  <Server size={14} className="text-indigo-500" />
                  {t('admin.settings.coreFqdn')}
                </Label>
                <span className="text-[10px] font-bold text-indigo-500 bg-indigo-500/10 px-2 py-0.5 rounded border border-indigo-500/20 uppercase tracking-widest">{t('admin.settings.globalUrl')}</span>
              </div>
              <Input
                value={settings.base_domain || ''}
                onChange={(e) => handleChange('base_domain', e.target.value)}
                placeholder={t('admin.settings.coreFqdnPlaceholder')}
              />
              <p className="text-xs text-muted-foreground flex items-center gap-2">
                <AlertCircle size={14} /> {t('admin.settings.coreFqdnDesc')}
              </p>
            </div>

            <div className="space-y-3">
              <Label className="flex items-center gap-2 text-xs uppercase tracking-wider text-muted-foreground font-semibold">
                <Network size={14} className="text-purple-500" />
                {t('admin.settings.deploymentPool')}
              </Label>
              <Input
                value={settings.project_domain || ''}
                onChange={(e) => handleChange('project_domain', e.target.value)}
                placeholder={t('admin.settings.projectPoolPlaceholder')}
              />
              <div className="p-3 rounded-lg bg-purple-500/5 border border-purple-500/10 flex items-center gap-3">
                <div className="text-xs font-bold text-purple-600 dark:text-purple-400 uppercase tracking-widest">{t('admin.settings.wildcardResolve')}:</div>
                <div className="flex-1 text-sm font-mono text-muted-foreground truncate">
                  *.{settings.project_domain || 'example.com'} 
                </div>
              </div>
            </div>
          </CardContent>
        </Card>

        {/* Operational Limits */}
        <Card className="overflow-hidden relative group border-emerald-500/20 bg-gradient-to-br from-background to-emerald-500/5">
          <CardHeader className="flex flex-row items-center gap-4 pb-2">
            <div className="w-12 h-12 bg-emerald-500/10 rounded-xl flex items-center justify-center text-emerald-500">
              <Shield className="w-6 h-6" />
            </div>
            <div>
              <CardTitle className="text-xl">{t('admin.settings.securityLimits')}</CardTitle>
              <p className="text-xs text-muted-foreground font-semibold uppercase tracking-wider mt-1">{t('admin.settings.resourceQuota')}</p>
            </div>
          </CardHeader>
          <CardContent className="space-y-6 pt-6">
            <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-6">
              <div className="flex flex-col justify-between p-5 rounded-xl bg-background/50 border border-emerald-500/10 hover:border-emerald-500/30 transition-colors min-h-[120px]">
                <Label className="flex items-start gap-2 text-[10px] uppercase font-bold tracking-widest text-muted-foreground leading-tight">
                  <Layout size={14} className="text-emerald-500 shrink-0 mt-0.5" />
                  <span>{t('admin.settings.identityQuota')}</span>
                </Label>
                <div className="mt-4">
                  <NumberStepper
                    min={1}
                    max={10}
                    value={settings.max_projects_per_user || 3}
                    onChange={(val) => handleChange('max_projects_per_user', val)}
                    unit={t('admin.settings.projects')}
                  />
                </div>
              </div>
              
              <div className="flex flex-col justify-between p-5 rounded-xl bg-background/50 border border-emerald-500/10 hover:border-emerald-500/30 transition-colors min-h-[120px]">
                <Label className="flex items-start gap-2 text-[10px] uppercase font-bold tracking-widest text-muted-foreground leading-tight">
                  <Clock size={14} className="text-amber-500 shrink-0 mt-0.5" />
                  <span>{t('admin.settings.expiryCycle')}</span>
                </Label>
                <div className="mt-4">
                  <NumberStepper
                    min={0}
                    value={settings.project_expiry_days || 30}
                    onChange={(val) => handleChange('project_expiry_days', val)}
                    unit={t('admin.settings.days')}
                  />
                </div>
              </div>

              <div className="flex flex-col justify-between p-5 rounded-xl bg-background/50 border border-emerald-500/10 hover:border-emerald-500/30 transition-colors min-h-[120px]">
                <Label className="flex items-start gap-2 text-[10px] uppercase font-bold tracking-widest text-muted-foreground leading-tight">
                  <Shield size={14} className="text-rose-500 shrink-0 mt-0.5" />
                  <span>{t('admin.settings.inactivityTimeout')}</span>
                </Label>
                <div className="mt-4">
                  <NumberStepper
                    min={1}
                    value={settings.admin_idle_timeout || 15}
                    onChange={(val) => handleChange('admin_idle_timeout', val)}
                    unit={t('admin.settings.mins')}
                  />
                </div>
              </div>

              <div className="flex flex-col justify-between p-5 rounded-xl bg-background/50 border border-emerald-500/10 hover:border-emerald-500/30 transition-colors min-h-[120px]">
                <Label className="flex items-start gap-2 text-[10px] uppercase font-bold tracking-widest text-muted-foreground leading-tight">
                  <RefreshCw size={14} className="text-blue-500 shrink-0 mt-0.5" />
                  <span>{t('admin.settings.concurrentBuilds')}</span>
                </Label>
                <div className="mt-4">
                  <NumberStepper
                    min={1}
                    max={8}
                    value={settings.max_concurrent_builds || 3}
                    onChange={(val) => handleChange('max_concurrent_builds', val)}
                    unit={t('admin.settings.workers')}
                  />
                </div>
              </div>
            </div>

            <div className="p-4 rounded-lg bg-muted/50 border space-y-2">
              <div className="flex items-center gap-2">
                <Activity className="w-4 h-4 text-emerald-500" />
                <span className="text-xs font-bold uppercase tracking-widest">{t('admin.settings.enforcementPolicy')}</span>
              </div>
              <p className="text-sm text-muted-foreground leading-relaxed" dangerouslySetInnerHTML={{ __html: t('admin.settings.enforcementDesc') }} />
            </div>
          </CardContent>
        </Card>

        {/* Compute Specs */}
        <Card className="lg:col-span-2 overflow-hidden relative group border-rose-500/20 bg-gradient-to-br from-background to-rose-500/5">
          <CardHeader className="flex flex-row items-center gap-4 pb-2">
            <div className="w-12 h-12 bg-rose-500/10 rounded-xl flex items-center justify-center text-rose-500">
              <Zap className="w-6 h-6" />
            </div>
            <div>
              <CardTitle className="text-xl">{t('admin.settings.computeSpecs')}</CardTitle>
              <p className="text-xs text-muted-foreground font-semibold uppercase tracking-wider mt-1">{t('admin.settings.provisioningProfile')}</p>
            </div>
          </CardHeader>
          <CardContent className="grid grid-cols-1 md:grid-cols-2 gap-10 pt-8">
            <div className="space-y-6 bg-background/50 p-6 rounded-xl border">
              <div className="flex items-center justify-between">
                <Label className="flex items-center gap-2 text-xs uppercase tracking-wider text-muted-foreground font-semibold">
                  <Cpu size={16} className="text-rose-500" />
                  {t('admin.settings.cpuLimit')}
                </Label>
                <span className="text-lg font-bold text-foreground">{settings.cpu_limit_percent || 50}% <span className="text-sm text-muted-foreground">{t('admin.settings.cores')}</span></span>
              </div>
              <Slider 
                value={[settings.cpu_limit_percent || 50]}
                min={10}
                max={100}
                step={5}
                onValueChange={(val: any) => handleChange('cpu_limit_percent', Array.isArray(val) ? val[0] : val)}
                className="py-4"
              />
              <div className="flex justify-between text-xs font-semibold text-muted-foreground uppercase tracking-widest">
                <span>{t('admin.settings.lowPriority')}</span>
                <span>{t('admin.settings.burst')}</span>
              </div>
            </div>

            <div className="space-y-6 bg-background/50 p-6 rounded-xl border">
              <div className="flex items-center justify-between">
                <Label className="flex items-center gap-2 text-xs uppercase tracking-wider text-muted-foreground font-semibold">
                  <Database size={16} className="text-indigo-500" />
                  {t('admin.settings.memoryLimit')}
                </Label>
                <span className="text-lg font-bold text-foreground">{settings.memory_limit_mb || 512} <span className="text-sm text-muted-foreground">{t('admin.settings.mbRam')}</span></span>
              </div>
              <Slider 
                value={[settings.memory_limit_mb || 512]}
                min={128}
                max={2048}
                step={128}
                onValueChange={(val: any) => handleChange('memory_limit_mb', Array.isArray(val) ? val[0] : val)}
                className="py-4"
              />
              <div className="flex justify-between text-xs font-semibold text-muted-foreground uppercase tracking-widest">
                <span>128 MB</span>
                <span>2048 MB</span>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

export default AdminSettings

