// ===========================================
// Admin Dashboard (PaaS Infrastructure)
// ===========================================

import { useState, useEffect, memo, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { systemAPI } from '../../services/api'
import toast from 'react-hot-toast'
import ConfirmationModal from '../../components/ConfirmationModal'
import { 
  RefreshCw, 
  Trash2, 
  Cpu, 
  HardDrive, 
  Box, 
  Image as ImageIcon, 
  Network, 
  Layers,
  Activity,
  Server,
  Database,
  ShieldAlert,
  Monitor,
  ChevronRight,
  Database as DbIcon,
  Search,
  Zap
} from 'lucide-react'

function AdminDashboard() {
  const [data, setData] = useState({
    system: null,
    containers: [],
    images: [],
    networks: [],
    volumes: [],
    recentProjects: []
  })
  const [isLoading, setIsLoading] = useState(true)
  const [isPruning, setIsPruning] = useState(false)
  const [isPruneModalOpen, setIsPruneModalOpen] = useState(false)

  const fetchData = useCallback(async () => {
    try {
      const res = await systemAPI.getStats()
      setData(res.data)
    } catch (error) {
      console.error('Failed to fetch system stats:', error)
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 15000)
    return () => clearInterval(interval)
  }, [fetchData])

  const handlePrune = () => {
    setIsPruneModalOpen(true)
  }

  const confirmPrune = async () => {
    setIsPruning(true)
    try {
      await systemAPI.prune()
      toast.success('System purged of unused assets')
      fetchData()
    } catch (error) {
      toast.error('Clean operation failed')
    } finally {
      setIsPruning(false)
    }
  }

  const formatBytes = (bytes) => {
    if (bytes === 0) return '0 B'
    const k = 1024
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
    const i = Math.floor(Math.log(bytes) / Math.log(k))
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
  }

  if (isLoading && !data.system) {
    return (
      <div className="min-h-[60vh] flex flex-col items-center justify-center gap-6">
        <div className="w-16 h-16 border-4 border-indigo-500/10 border-t-indigo-500 rounded-full animate-spin"></div>
        <p className="text-slate-600 dark:text-slate-400 font-black uppercase tracking-[0.3em] text-[10px] animate-pulse">Loading Dashboard</p>
      </div>
    )
  }

  const { system, containers, images, networks, volumes } = data

  return (
    <div className="space-y-12 animate-pop-in relative h-full">
      {/* Background Glows */}
      
      
      
      <div className="relative z-10 space-y-12">
        <Header onRefresh={fetchData} onPrune={handlePrune} isPruning={isPruning} />
        
        <SystemOverview 
          system={system} 
          containers={containers} 
          images={images} 
          networks={networks} 
          volumes={volumes} 
          formatBytes={formatBytes} 
        />
        
        <div className="grid grid-cols-1 xl:grid-cols-2 gap-8">
          <ResourceTable 
            title="Containers" 
            subtitle="Live workload containers"
            icon={Box}
            data={containers}
            type="containers"
            viewAllPath="/admin/containers"
          />

          <ResourceTable 
            title="Images" 
            subtitle="Local image snapshots"
            icon={ImageIcon}
            data={images}
            type="images"
            viewAllPath="/admin/images"
          />
        </div>
      </div>

      <ConfirmationModal 
        isOpen={isPruneModalOpen}
        onClose={() => setIsPruneModalOpen(false)}
        onConfirm={confirmPrune}
        title="Execute System Purge?"
        message="This will permanently delete all inactive images and orphaned volumes. This operation will free up local storage but cannot be rolled back."
        confirmText="Initialize Cleanup"
        type="danger"
      />
    </div>
  )
}

const Header = memo(({ onRefresh, onPrune, isPruning }) => (
  <div className="flex flex-col md:flex-row md:items-end justify-between gap-8 mb-12">
    <div>
      <h1 className="text-5xl font-black text-slate-900 dark:text-white tracking-tighter mb-4 italic text-transparent bg-clip-text pr-2 pb-1 bg-gradient-to-r from-indigo-400 to-purple-400">Dashboard</h1>
      <p className="text-slate-600 dark:text-slate-400 text-lg font-medium max-w-2xl leading-relaxed">
        Monitoring global infrastructure state and resource orchestration across the student cluster.
      </p>
    </div>
    
    <div className="flex items-center gap-4">
      <button 
        onClick={onRefresh}
        className="btn btn-secondary py-3 px-6 text-sm font-black uppercase tracking-widest"
      >
        <RefreshCw className="w-4 h-4" />
        Refresh
      </button>
      <button 
        onClick={onPrune}
        disabled={isPruning}
        className="btn bg-rose-500/10 border border-rose-500/20 text-rose-400 hover:bg-rose-500/20 py-3 px-6 text-sm font-black uppercase tracking-widest disabled:opacity-50"
      >
        <ShieldAlert className="w-4 h-4" />
        {isPruning ? 'Cleaning...' : 'Purge DB'}
      </button>
    </div>
  </div>
))

const SystemOverview = memo(({ system, containers, images, networks, volumes, formatBytes }) => {
  const memUsage = (system?.memory_used / system?.memory_total) * 100 || 0
  
  return (
    <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-8">
      <StatCard 
        title="CPU Load" 
        value={`${(system?.cpu_usage || 0).toFixed(1)}%`}
        detail={`${system?.cpu_cores || 1} CPU Cores`}
        progress={Math.min(system?.cpu_usage || 0, 100)}
        icon={Cpu}
        color="indigo"
      />

      <StatCard 
        title="Compute RAM" 
        value={formatBytes(system?.memory_used || 0)}
        detail={`of ${formatBytes(system?.memory_total || 0)} total available`}
        progress={memUsage}
        icon={Activity}
        color="purple"
      />

      <StatCard 
        title="System Resources" 
        value={images?.length + containers?.length || 0}
        detail={`${containers?.length || 0} Containers / ${images?.length || 0} Images`}
        progress={100}
        icon={Layers}
        color="fuchsia"
      />
      
      <div className="md:col-span-2 xl:col-span-3 grid grid-cols-1 md:grid-cols-3 gap-8">
        <SmallStat icon={Network} label="Networks" value={networks?.length || 0} color="indigo" />
        <SmallStat icon={HardDrive} label="Volumes" value={volumes?.length || 0} color="purple" />
        <SmallStat icon={Monitor} label="Status" value={system?.os_platform || 'Linux'} color="fuchsia" />
      </div>
    </div>
  )
})

const StatCard = ({ title, value, detail, progress, icon: Icon, color }) => {
  const colors = {
    indigo: "from-indigo-500 to-blue-500 text-indigo-400",
    purple: "from-purple-500 to-indigo-500 text-purple-400",
    fuchsia: "from-fuchsia-500 to-purple-500 text-fuchsia-400"
  }

  return (
    <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm p-10 group hover:scale-[1.02] transition-all duration-500 border-slate-300 dark:border-white/10 relative overflow-hidden bg-slate-50 dark:bg-slate-100 dark:bg-white/5">
      <div className="flex justify-between items-start mb-8">
        <div>
          <p className="text-slate-600 dark:text-slate-400 text-[10px] font-black uppercase tracking-[0.2em] mb-2">{title}</p>
          <h3 className="text-4xl font-black text-slate-900 dark:text-white tracking-tighter">{value}</h3>
        </div>
        <div className={`w-14 h-14 rounded-2xl bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 flex items-center justify-center ${colors[color].split(' ')[2]}`}>
          <Icon className="w-7 h-7" />
        </div>
      </div>
      
      <div className="space-y-3">
        <div className="flex justify-between text-[10px] font-black uppercase tracking-[0.2em]">
          <span className="text-slate-600">Utilization</span>
          <span className="text-slate-600 dark:text-slate-400">{detail}</span>
        </div>
        <div className="h-2 bg-black/40 rounded-full overflow-hidden border border-slate-200 dark:border-white/5">
          <div 
            className={`h-full bg-gradient-to-r ${colors[color].split(' ').slice(0,2).join(' ')} transition-all duration-1000 shadow-[0_0_15px_rgba(99,102,241,0.3)]`} 
            style={{ width: `${progress}%` }}
          />
        </div>
      </div>
    </div>
  )
}

const SmallStat = ({ icon: Icon, label, value, color }) => {
  const colors = {
    indigo: "text-indigo-400 border-indigo-400/20 bg-indigo-400/5",
    purple: "text-purple-400 border-purple-400/20 bg-purple-400/5",
    fuchsia: "text-fuchsia-400 border-fuchsia-400/20 bg-fuchsia-400/5"
  }
  
  return (
    <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm p-8 flex items-center justify-between border-slate-300 dark:border-white/10 group hover:bg-slate-100 dark:bg-slate-100 dark:bg-white/5 transition-all duration-300 bg-slate-50 dark:bg-slate-100 dark:bg-white/5">
      <div className="flex items-center gap-6">
        <div className={`w-12 h-12 rounded-2xl flex items-center justify-center transition-all ${colors[color] || colors.indigo}`}>
          <Icon className="w-6 h-6" />
        </div>
        <div>
          <p className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-600 dark:text-slate-400 mb-1">{label}</p>
          <p className="text-xl font-black text-slate-900 dark:text-white uppercase tracking-tight">{value}</p>
        </div>
      </div>
      <Zap className="w-5 h-5 text-slate-800 group-hover:text-indigo-400/20 transition-colors" />
    </div>
  )
}

const ResourceTable = memo(({ title, subtitle, icon: Icon, data, type, viewAllPath }) => (
  <div className="bg-white dark:bg-slate-800 border-slate-200 dark:border-slate-700 rounded-2xl shadow-sm overflow-hidden border-slate-300 dark:border-white/10 relative z-10 bg-slate-50 dark:bg-slate-100 dark:bg-white/5">
    <div className="p-10 border-b border-slate-200 dark:border-white/5 flex items-center justify-between bg-slate-100 dark:bg-slate-100 dark:bg-white/5">
      <div className="flex items-center gap-5">
        <div className="w-12 h-12 rounded-2xl bg-slate-100 dark:bg-white/5 border border-slate-300 dark:border-white/10 flex items-center justify-center text-indigo-400">
          <Icon className="w-6 h-6" />
        </div>
        <div>
          <h2 className="text-2xl font-black text-slate-900 dark:text-white tracking-tight uppercase">{title}</h2>
          <p className="text-slate-600 dark:text-slate-400 text-[10px] font-black uppercase tracking-[0.2em]">{subtitle}</p>
        </div>
      </div>
      <Link to={viewAllPath} className="group flex items-center gap-2 text-[10px] font-black uppercase tracking-widest text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white transition-all">
        Full Registry <ChevronRight className="w-4 h-4 group-hover:translate-x-1 transition-transform" />
      </Link>
    </div>
    
    <div className="table-container">
      <table className="premium-table">
        {type === 'containers' ? (
          <ContainerTableBody data={data} />
        ) : (
          <ImageTableBody data={data} />
        )}
      </table>
    </div>
    {data.length === 0 && (
      <div className="p-24 text-center flex flex-col items-center">
        <div className="w-20 h-20 bg-slate-100 dark:bg-white/5 rounded-3xl flex items-center justify-center mb-8 border border-slate-200 dark:border-white/5">
          <Icon className="w-10 h-10 text-slate-800" />
        </div>
        <p className="text-slate-600 dark:text-slate-400 text-[10px] font-black uppercase tracking-[0.3em]">No {type} isolated in registry</p>
      </div>
    )}
  </div>
))

const ContainerTableBody = memo(({ data }) => (
  <>
    <thead>
      <tr>
        <th>Identity</th>
        <th>Image Protocol</th>
        <th className="text-center">Active Logic</th>
        <th className="text-right">Up-Time</th>
      </tr>
    </thead>
    <tbody>
      {data.slice(0, 8).map((c) => (
        <tr key={c.id} className="group">
          <td>
            <div className="flex items-center gap-4">
              <div className="w-8 h-8 rounded-lg bg-indigo-500/10 border border-indigo-500/20 flex items-center justify-center text-indigo-400 font-bold text-[10px]">
                {c.names[0]?.substring(0, 2).toUpperCase() || '??'}
              </div>
              <span className="font-black text-sm text-slate-900 dark:text-white group-hover:text-indigo-400 transition-colors uppercase tracking-tight truncate max-w-[120px]">
                {c.names[0] || c.id.substring(0, 8)}
              </span>
            </div>
          </td>
          <td><span className="font-mono text-[10px] text-slate-600 dark:text-slate-400 italic uppercase">{c.image}</span></td>
          <td className="text-center">
            <span className={`px-4 py-1.5 rounded-full border text-[9px] font-black uppercase tracking-widest inline-flex items-center gap-2 ${c.state === 'running' ? 'text-emerald-400 border-emerald-400/20 bg-emerald-400/5' : 'text-rose-400 border-rose-400/20 bg-rose-400/5'}`}>
              <div className={`w-1.5 h-1.5 rounded-full ${c.state === 'running' ? 'bg-emerald-400 animate-pulse' : 'bg-rose-400'}`} />
              {c.state}
            </span>
          </td>
          <td className="text-right font-mono text-[11px] text-slate-600 dark:text-slate-400 font-bold">{c.status}</td>
        </tr>
      ))}
    </tbody>
  </>
))

const ImageTableBody = memo(({ data }) => (
  <>
    <thead>
      <tr>
        <th>Architecture</th>
        <th className="text-center">Lifecycle</th>
        <th className="text-center">Revision</th>
        <th className="text-right">Weight</th>
      </tr>
    </thead>
    <tbody>
      {data.slice(0, 8).map((img, i) => (
        <tr key={i} className="group">
          <td>
            <div className="flex flex-col">
              <span className="font-black text-sm text-slate-900 dark:text-white uppercase tracking-tight group-hover:text-indigo-400 transition-colors">{img.repository}</span>
              <span className="text-[9px] text-slate-700 font-mono font-bold tracking-widest">{img.id?.substring(7, 19)}</span>
            </div>
          </td>
          <td className="text-center">
            <span className={`px-4 py-1.5 rounded-full border text-[9px] font-black uppercase tracking-widest ${img.status === 'In Use' ? 'text-indigo-400 border-indigo-400/20 bg-indigo-400/5' : 'text-slate-600 dark:text-slate-400 border-slate-500/20 bg-slate-500/5'}`}>
              {img.status}
            </span>
          </td>
          <td className="text-center font-mono text-[11px] text-slate-600 dark:text-slate-400 font-bold uppercase">{img.tag}</td>
          <td className="text-right font-mono text-[11px] text-slate-600 dark:text-slate-400 font-black">{img.size_human}</td>
        </tr>
      ))}
    </tbody>
  </>
))

export default AdminDashboard


