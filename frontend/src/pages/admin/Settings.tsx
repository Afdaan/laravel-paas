import React, { useState, useEffect, useCallback } from 'react'
import { toast } from 'sonner'
import { settingsAPI } from '../../services/api'
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
  Loader2
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Slider } from '@/components/ui/slider'

interface PlatformSettings {
  base_domain?: string;
  project_domain?: string;
  max_projects_per_user?: number;
  project_expiry_days?: number;
  cpu_limit_percent?: number;
  memory_limit_mb?: number;
}

const AdminSettings = () => {
  const [settings, setSettings] = useState<PlatformSettings>({})
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  
  const fetchSettings = useCallback(async () => {
    setIsLoading(true)
    try {
      const response = await settingsAPI.list()
      setSettings(response.data.map || {})
    } catch (error) {
      toast.error('Failed to load system state')
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchSettings()
  }, [fetchSettings])
  
  const handleChange = (key: keyof PlatformSettings, value: any) => {
    setSettings(prev => ({ ...prev, [key]: value }))
  }
  
  const handleSave = async () => {
    setIsSaving(true)
    try {
      await settingsAPI.update(settings)
      toast.success('System configuration synchronized')
    } catch (error) {
      toast.error('Manifest update failed')
    } finally {
      setIsSaving(false)
    }
  }
  
  if (isLoading) {
    return (
      <div className="flex flex-col items-center justify-center h-80 gap-6">
        <Loader2 className="w-12 h-12 text-primary animate-spin" />
        <p className="text-muted-foreground text-xs font-bold uppercase tracking-widest animate-pulse">Retrieving System Manifest</p>
      </div>
    )
  }

  return (
    <div className="space-y-8 animate-in fade-in duration-500 pb-20">
      <div className="flex flex-col xl:flex-row xl:items-end justify-between gap-6 pb-4 border-b">
        <div>
          <h1 className="text-3xl font-bold tracking-tight mb-2">Core Parameters</h1>
          <p className="text-muted-foreground">Configure global orchestration limits, resource pooling, and network topology.</p>
        </div>

        <Button
          onClick={handleSave}
          disabled={isSaving}
          size="lg"
          className="w-full xl:w-auto"
        >
          {isSaving ? <Loader2 className="w-4 h-4 mr-2 animate-spin" /> : <Save className="w-4 h-4 mr-2" />}
          {isSaving ? 'Syncing...' : 'Deploy Changes'}
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
              <CardTitle className="text-xl">Domain Topology</CardTitle>
              <p className="text-xs text-muted-foreground font-semibold uppercase tracking-wider mt-1">Network Identity Routing</p>
            </div>
          </CardHeader>
          <CardContent className="space-y-6 pt-6">
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <Label className="flex items-center gap-2 text-xs uppercase tracking-wider text-muted-foreground font-semibold">
                  <Server size={14} className="text-indigo-500" />
                  Platform Core FQDN
                </Label>
                <span className="text-[10px] font-bold text-indigo-500 bg-indigo-500/10 px-2 py-0.5 rounded border border-indigo-500/20 uppercase tracking-widest">Global URL</span>
              </div>
              <Input
                value={settings.base_domain || ''}
                onChange={(e) => handleChange('base_domain', e.target.value)}
                placeholder="paas.example.com"
              />
              <p className="text-xs text-muted-foreground flex items-center gap-2">
                <AlertCircle size={14} /> Basic routing for the orchestrator dashboard and centralized API.
              </p>
            </div>

            <div className="space-y-3">
              <Label className="flex items-center gap-2 text-xs uppercase tracking-wider text-muted-foreground font-semibold">
                <Network size={14} className="text-purple-500" />
                Deployment Pool Domain
              </Label>
              <Input
                value={settings.project_domain || ''}
                onChange={(e) => handleChange('project_domain', e.target.value)}
                placeholder="projects.example.com"
              />
              <div className="p-3 rounded-lg bg-purple-500/5 border border-purple-500/10 flex items-center gap-3">
                <div className="text-xs font-bold text-purple-600 dark:text-purple-400 uppercase tracking-widest">Wildcard Resolve:</div>
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
              <CardTitle className="text-xl">Security Limits</CardTitle>
              <p className="text-xs text-muted-foreground font-semibold uppercase tracking-wider mt-1">Resource Quota Enforcement</p>
            </div>
          </CardHeader>
          <CardContent className="space-y-6 pt-6">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
              <div className="space-y-3">
                <Label className="flex items-center gap-2 text-xs uppercase tracking-wider text-muted-foreground font-semibold">
                  <Layout size={14} className="text-emerald-500" />
                  Identity Quota
                </Label>
                <div className="relative">
                  <Input
                    type="number"
                    min="1"
                    max="10"
                    value={settings.max_projects_per_user || 3}
                    onChange={(e) => handleChange('max_projects_per_user', parseInt(e.target.value) || 3)}
                    className="pr-16"
                  />
                  <div className="absolute inset-y-0 right-3 flex items-center pointer-events-none text-muted-foreground font-semibold text-xs transition-opacity opacity-50 uppercase">Projects</div>
                </div>
              </div>
              <div className="space-y-3">
                <Label className="flex items-center gap-2 text-xs uppercase tracking-wider text-muted-foreground font-semibold">
                  <Clock size={14} className="text-amber-500" />
                  Cluster Expiry Cycle
                </Label>
                <div className="relative">
                  <Input
                    type="number"
                    min="0"
                    value={settings.project_expiry_days || 30}
                    onChange={(e) => handleChange('project_expiry_days', parseInt(e.target.value) || 0)}
                    className="pr-16"
                  />
                  <div className="absolute inset-y-0 right-3 flex items-center pointer-events-none text-muted-foreground font-semibold text-xs opacity-50 uppercase">Days</div>
                </div>
              </div>
            </div>

            <div className="p-4 rounded-lg bg-muted/50 border space-y-2">
              <div className="flex items-center gap-2">
                <Activity className="w-4 h-4 text-emerald-500" />
                <span className="text-xs font-bold uppercase tracking-widest">Active Enforcement Policy</span>
              </div>
              <p className="text-sm text-muted-foreground leading-relaxed">
                Setting project expiry to <span className="text-emerald-500 font-bold">0</span> disables the automatic reclamation cycle. It is recommended to keep this at <span className="text-emerald-500 font-bold">30</span> for optimal resource density.
              </p>
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
              <CardTitle className="text-xl">Standard Instance Compute</CardTitle>
              <p className="text-xs text-muted-foreground font-semibold uppercase tracking-wider mt-1">Default Provisioning Profile</p>
            </div>
          </CardHeader>
          <CardContent className="grid grid-cols-1 md:grid-cols-2 gap-10 pt-8">
            <div className="space-y-6 bg-background/50 p-6 rounded-xl border">
              <div className="flex items-center justify-between">
                <Label className="flex items-center gap-2 text-xs uppercase tracking-wider text-muted-foreground font-semibold">
                  <Cpu size={16} className="text-rose-500" />
                  CPU Hard Limit
                </Label>
                <span className="text-lg font-bold text-foreground">{settings.cpu_limit_percent || 50}% <span className="text-sm text-muted-foreground">Cores</span></span>
              </div>
              <Slider 
                value={[settings.cpu_limit_percent || 50]}
                min={10}
                max={100}
                step={5}
                onValueChange={(val: number[]) => handleChange('cpu_limit_percent', val[0])}
                className="py-4"
              />
              <div className="flex justify-between text-xs font-semibold text-muted-foreground uppercase tracking-widest">
                <span>Low Priority (10%)</span>
                <span>Burst (100%)</span>
              </div>
            </div>

            <div className="space-y-6 bg-background/50 p-6 rounded-xl border">
              <div className="flex items-center justify-between">
                <Label className="flex items-center gap-2 text-xs uppercase tracking-wider text-muted-foreground font-semibold">
                  <Database size={16} className="text-indigo-500" />
                  Memory Hard Limit
                </Label>
                <span className="text-lg font-bold text-foreground">{settings.memory_limit_mb || 512} <span className="text-sm text-muted-foreground">MB RAM</span></span>
              </div>
              <Slider 
                value={[settings.memory_limit_mb || 512]}
                min={128}
                max={2048}
                step={128}
                onValueChange={(val: number[]) => handleChange('memory_limit_mb', val[0])}
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

