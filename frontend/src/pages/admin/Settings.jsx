// ===========================================
// Admin Platform Settings Page
// ===========================================

import { useState, useEffect, useCallback } from 'react'
import toast from 'react-hot-toast'
import { settingsAPI } from '../../services/api'
import { 
  Settings, 
  Globe, 
  Shield, 
  Activity, 
  Cpu, 
  Database, 
  Save, 
  AlertCircle,
  Server,
  Network,
  Lock,
  Zap,
  Clock,
  Layout
} from 'lucide-react'

const AdminSettings = () => {
    const [settings, setSettings] = useState({})
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
    
    const handleChange = (key, value) => {
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
                 <div className="relative">
                    <div className="absolute -inset-8 bg-indigo-500/10 rounded-full blur-2xl animate-pulse"></div>
                    <div className="w-16 h-16 border-4 border-indigo-500/10 border-t-indigo-500 rounded-full animate-spin relative z-10"></div>
                </div>
                <p className="text-slate-500 text-[10px] font-black uppercase tracking-[0.3em] animate-pulse">Retrieving System Manifest</p>
            </div>
        )
    }

    return (
        <div className="space-y-12 animate-pop-in relative h-full pb-20">
            {/* Background Glows */}
            <div className="absolute top-0 right-0 w-[50vw] h-[50vw] bg-indigo-600/5 blur-[140px] rounded-full pointer-events-none z-0"></div>

            <div className="relative z-10">
                <div className="flex flex-col xl:flex-row xl:items-end justify-between mb-16 gap-8">
                    <div>
                        <h1 className="text-5xl font-black text-white tracking-tighter mb-4 italic">Core <span className="text-transparent bg-clip-text bg-gradient-to-r from-indigo-400 to-purple-400">Parameters</span></h1>
                        <p className="text-slate-400 text-lg font-medium">Configure global orchestration limits, resource pooling, and network topology.</p>
                    </div>

                    <button
                        onClick={handleSave}
                        disabled={isSaving}
                        className="btn btn-primary py-4 px-10 text-sm font-black uppercase tracking-widest active:scale-95 transition-all shadow-[0_15px_30px_rgba(99,102,241,0.25)] flex items-center gap-3 disabled:opacity-50"
                    >
                        {isSaving ? (
                            <div className="w-4 h-4 border-2 border-white/20 border-t-white rounded-full animate-spin" />
                        ) : (
                            <Save className="w-4 h-4" />
                        )}
                        {isSaving ? 'Syncing...' : 'Deploy Changes'}
                    </button>
                </div>

                <div className="grid grid-cols-1 lg:grid-cols-2 gap-10">
                    {/* Domain Infrastructure */}
                    <div className="card-glass p-10 bg-white/[0.01] border-white/10 group overflow-hidden relative transition-all duration-500 hover:bg-white/[0.03] hover:border-white/20">
                         <div className="absolute top-0 right-0 w-48 h-48 bg-indigo-500/5 blur-[80px] pointer-events-none" />
                        
                         <div className="flex items-center gap-4 mb-10">
                            <div className="w-14 h-14 bg-indigo-500/10 border border-indigo-500/20 rounded-2xl flex items-center justify-center text-indigo-400 group-hover:scale-110 transition-transform shadow-xl">
                                <Globe className="w-7 h-7" />
                            </div>
                            <div>
                                <h2 className="text-2xl font-black text-white uppercase tracking-tight">Domain Topology</h2>
                                <p className="text-xs text-slate-500 font-bold tracking-widest uppercase mt-1">Network Identity Routing</p>
                            </div>
                        </div>

                        <div className="space-y-8">
                            <div className="space-y-3">
                                <div className="flex items-center justify-between">
                                    <label className="text-[10px] font-black text-slate-500 uppercase tracking-[0.2em] ml-1 flex items-center gap-2">
                                        <Server size={12} className="text-indigo-400" />
                                        Platform Core FQDN
                                    </label>
                                    <span className="text-[8px] font-black text-indigo-500 bg-indigo-500/5 px-2 py-0.5 rounded uppercase tracking-widest border border-indigo-500/10">Read-Only Context</span>
                                </div>
                                <input
                                    type="text"
                                    value={settings.base_domain || ''}
                                    onChange={(e) => handleChange('base_domain', e.target.value)}
                                    className="w-full bg-black/40 border border-white/10 rounded-2xl px-5 py-4 text-white font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all outline-none"
                                    placeholder="paas.afdaan.io"
                                />
                                <p className="text-[11px] text-slate-600 font-medium italic pl-1 flex items-center gap-2">
                                    <AlertCircle size={10} /> Basic routing for the orchestrator dashboard and centralized API.
                                </p>
                            </div>

                            <div className="space-y-3">
                                <label className="text-[10px] font-black text-slate-500 uppercase tracking-[0.2em] ml-1 flex items-center gap-2">
                                    <Network size={12} className="text-purple-400" />
                                    Deployment Pool Domain
                                </label>
                                <input
                                    type="text"
                                    value={settings.project_domain || ''}
                                    onChange={(e) => handleChange('project_domain', e.target.value)}
                                    className="w-full bg-black/40 border border-white/10 rounded-2xl px-5 py-4 text-white font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all outline-none"
                                    placeholder="hosting.school.com"
                                />
                                <div className="p-4 rounded-xl bg-purple-500/5 border border-purple-500/10 flex items-center gap-4">
                                     <div className="text-[10px] font-black text-purple-400 uppercase tracking-widest whitespace-nowrap">Wildcard Resolve:</div>
                                     <div className="flex-1 text-[11px] font-mono text-slate-400 truncate">
                                        *.{settings.project_domain || 'example.com'} 
                                     </div>
                                </div>
                            </div>
                        </div>
                    </div>

                    {/* Operational Limits */}
                    <div className="card-glass p-10 bg-white/[0.01] border-white/10 group overflow-hidden relative transition-all duration-500 hover:bg-white/[0.03] hover:border-white/20">
                         <div className="absolute top-0 right-0 w-48 h-48 bg-emerald-500/5 blur-[80px] pointer-events-none" />

                         <div className="flex items-center gap-4 mb-10">
                            <div className="w-14 h-14 bg-emerald-500/10 border border-emerald-500/20 rounded-2xl flex items-center justify-center text-emerald-400 group-hover:scale-110 transition-transform shadow-xl">
                                <Shield className="w-7 h-7" />
                            </div>
                            <div>
                                <h2 className="text-2xl font-black text-white uppercase tracking-tight">Security Limits</h2>
                                <p className="text-xs text-slate-500 font-bold tracking-widest uppercase mt-1">Resource Quota Enforcement</p>
                            </div>
                        </div>

                        <div className="grid grid-cols-1 md:grid-cols-2 gap-8">
                            <div className="space-y-3">
                                <label className="text-[10px] font-black text-slate-500 uppercase tracking-[0.2em] ml-1 flex items-center gap-2">
                                    <Layout size={12} className="text-emerald-400" />
                                    Identity Instance Quota
                                </label>
                                <div className="relative group/input">
                                    <input
                                        type="number"
                                        min="1"
                                        max="10"
                                        value={settings.max_projects_per_user || 3}
                                        onChange={(e) => handleChange('max_projects_per_user', e.target.value)}
                                        className="w-full bg-black/40 border border-white/10 rounded-2xl px-5 py-4 text-white font-medium focus:outline-none focus:ring-2 focus:ring-emerald-500/20 focus:border-emerald-500/40 transition-all outline-none appearance-none"
                                    />
                                    <div className="absolute inset-y-0 right-5 flex items-center pointer-events-none text-slate-700 font-bold text-xs uppercase">Items</div>
                                </div>
                            </div>
                            <div className="space-y-3">
                                <label className="text-[10px] font-black text-slate-500 uppercase tracking-[0.2em] ml-1 flex items-center gap-2">
                                    <Clock size={12} className="text-amber-400" />
                                    Cluster Expiry Cycle
                                </label>
                                <div className="relative group/input">
                                    <input
                                        type="number"
                                        min="0"
                                        value={settings.project_expiry_days || 30}
                                        onChange={(e) => handleChange('project_expiry_days', e.target.value)}
                                        className="w-full bg-black/40 border border-white/10 rounded-2xl px-5 py-4 text-white font-medium focus:outline-none focus:ring-2 focus:ring-amber-500/20 focus:border-amber-500/40 transition-all outline-none appearance-none"
                                    />
                                    <div className="absolute inset-y-0 right-5 flex items-center pointer-events-none text-slate-700 font-bold text-xs uppercase">Days</div>
                                </div>
                            </div>
                        </div>

                        <div className="mt-8 p-6 rounded-2xl bg-white/[0.02] border border-white/5 space-y-4">
                             <div className="flex items-center gap-3">
                                <Activity className="w-4 h-4 text-emerald-500" />
                                <span className="text-[10px] font-black text-white uppercase tracking-widest">Active Enforcement Policy</span>
                             </div>
                             <p className="text-xs text-slate-500 font-medium leading-relaxed italic">
                                Setting project expiry to <span className="text-emerald-400 font-black">0</span> disables the automatic reclamation cycle. It is recommended to keep this at <span className="text-emerald-400 font-black">30</span> for optimal resource density.
                             </p>
                        </div>
                    </div>

                    {/* Compute Specs */}
                    <div className="card-glass p-10 bg-white/[0.01] border-white/10 group overflow-hidden relative transition-all duration-500 hover:bg-white/[0.03] hover:border-white/20 lg:col-span-2">
                         <div className="absolute bottom-0 right-0 w-80 h-80 bg-rose-500/5 blur-[120px] pointer-events-none" />

                         <div className="flex items-center gap-4 mb-10">
                            <div className="w-14 h-14 bg-rose-500/10 border border-rose-500/20 rounded-2xl flex items-center justify-center text-rose-400 group-hover:scale-110 transition-transform shadow-xl">
                                <Zap className="w-7 h-7" />
                            </div>
                            <div>
                                <h2 className="text-2xl font-black text-white uppercase tracking-tight">Standard Instance Compute</h2>
                                <p className="text-xs text-slate-500 font-bold tracking-widest uppercase mt-1">Default Provisioning Profile</p>
                            </div>
                        </div>

                        <div className="grid grid-cols-1 md:grid-cols-2 gap-12">
                            <div className="space-y-6">
                                <div className="flex items-center justify-between">
                                    <label className="text-[10px] font-black text-slate-500 uppercase tracking-[0.2em] ml-1 flex items-center gap-2">
                                        <Cpu size={12} className="text-rose-400" />
                                        CPU Hard Limit
                                    </label>
                                    <span className="text-[14px] font-black text-white uppercase tracking-tight">{settings.cpu_limit_percent || 50}% Cores</span>
                                </div>
                                <input
                                    type="range"
                                    min="10"
                                    max="100"
                                    value={settings.cpu_limit_percent || 50}
                                    onChange={(e) => handleChange('cpu_limit_percent', e.target.value)}
                                    className="w-full h-2 bg-white/5 rounded-full appearance-none cursor-pointer accent-rose-500"
                                />
                                <div className="flex justify-between text-[9px] font-black text-slate-700 uppercase tracking-widest px-1">
                                    <span>Low Priority (10%)</span>
                                    <span>Burst (100%)</span>
                                </div>
                            </div>

                            <div className="space-y-6">
                                <div className="flex items-center justify-between">
                                    <label className="text-[10px] font-black text-slate-500 uppercase tracking-[0.2em] ml-1 flex items-center gap-2">
                                        <Database size={12} className="text-indigo-400" />
                                        Memory Hard Limit
                                    </label>
                                    <span className="text-[14px] font-black text-white uppercase tracking-tight">{settings.memory_limit_mb || 512} MB RAM</span>
                                </div>
                                <input
                                    type="range"
                                    min="128"
                                    max="2048"
                                    step="128"
                                    value={settings.memory_limit_mb || 512}
                                    onChange={(e) => handleChange('memory_limit_mb', e.target.value)}
                                    className="w-full h-2 bg-white/5 rounded-full appearance-none cursor-pointer accent-indigo-500"
                                />
                                <div className="flex justify-between text-[9px] font-black text-slate-700 uppercase tracking-widest px-1">
                                    <span>128 MB</span>
                                    <span>2.0 GB</span>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    )
}

export default AdminSettings

