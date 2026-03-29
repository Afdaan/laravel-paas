// ===========================================
// Admin Feedback Management Page
// ===========================================

import { useState, useEffect, memo, useCallback } from 'react'
import { feedbackAPI } from '../../services/api'
import toast from 'react-hot-toast'
import { 
  MessageSquare, 
  Check, 
  RotateCw, 
  Filter, 
  Trash2, 
  User, 
  Clock, 
  ShieldAlert, 
  Sparkles, 
  Bug,
  MoreHorizontal,
  ChevronRight,
  AlertTriangle,
  Lightbulb,
  Search
} from 'lucide-react'

const AdminFeedback = () => {
  const [feedback, setFeedback] = useState([])
  const [isLoading, setIsLoading] = useState(true)
  const [filterStatus, setFilterStatus] = useState('')
  const [filterType, setFilterType] = useState('')

  const fetchFeedback = useCallback(async () => {
    setIsLoading(true)
    try {
      const params = {}
      if (filterStatus) params.status = filterStatus
      if (filterType) params.type = filterType
      
      const res = await feedbackAPI.listAll(params)
      setFeedback(res.data || [])
    } catch (error) {
      console.error('Failed to fetch feedback:', error)
      toast.error('Could not load feedback registry')
    } finally {
      setIsLoading(false)
    }
  }, [filterStatus, filterType])

  useEffect(() => {
    fetchFeedback()
  }, [fetchFeedback])

  const handleUpdateStatus = async (id, status) => {
    try {
      await feedbackAPI.updateStatus(id, status)
      toast.success('Registry state updated')
      fetchFeedback()
    } catch (error) {
      toast.error('State update failed')
    }
  }

  const handleDelete = async (id) => {
    if (!window.confirm('Purge this feedback record? This cannot be undone.')) return
    try {
      await feedbackAPI.delete(id)
      toast.success('Record purged')
      fetchFeedback()
    } catch (error) {
      toast.error('Purge operation failed')
    }
  }

  return (
    <div className="space-y-12 animate-pop-in relative h-full">
      {/* Background Glows */}
      <div className="absolute top-0 right-0 w-[50vw] h-[50vw] bg-indigo-600/5 blur-[140px] rounded-full pointer-events-none z-0"></div>

      <div className="relative z-10">
        <div className="flex flex-col xl:flex-row xl:items-end justify-between mb-12 gap-8">
            <div>
              <h1 className="text-5xl font-black text-white tracking-tighter mb-4">Client <span className="text-transparent bg-clip-text bg-gradient-to-r from-indigo-400 to-purple-400">Intelligence</span></h1>
              <p className="text-slate-400 text-lg font-medium">Reviewing bug reports, architecture suggestions, and student sentiment analysis.</p>
            </div>

            <div className="flex items-center gap-4">
                <div className="flex items-center gap-3 bg-white/[0.02] border border-white/10 p-2 rounded-2xl backdrop-blur-md">
                    <div className="flex items-center gap-3 px-6 py-2 border-r border-white/5">
                        <MessageSquare className="w-4 h-4 text-indigo-500" />
                        <span className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-400">{feedback.length} Submissions</span>
                    </div>
                    <div className="flex items-center gap-3 px-6 py-2">
                        <ShieldAlert className="w-4 h-4 text-rose-500" />
                        <span className="text-[10px] font-black uppercase tracking-[0.2em] text-slate-400">{feedback.filter(f => f.type === 'bug').length} Critical</span>
                    </div>
                </div>

                <button onClick={fetchFeedback} className="w-12 h-12 flex items-center justify-center rounded-2xl bg-white/[0.02] border border-white/10 text-white hover:bg-white/[0.05] transition-all active:rotate-180 duration-500 transition-transform shadow-xl backdrop-blur-md">
                   <RotateCw className="w-4 h-4" />
                </button>
            </div>
        </div>

        {/* Filters */}
        <div className="flex flex-col md:flex-row items-center justify-between mb-12 gap-6 bg-white/[0.02] border border-white/10 p-4 rounded-3xl backdrop-blur-md shadow-2xl">
            <div className="flex items-center gap-4 flex-1 w-full max-w-2xl">
                <div className="relative flex-1 group">
                    <div className="absolute inset-y-0 left-0 pl-5 flex items-center pointer-events-none">
                        <Search className="w-4 h-4 text-slate-500 group-focus-within:text-indigo-400 transition-colors" />
                    </div>
                    <input 
                        type="text"
                        placeholder="Filter intel by keyword or student identity..."
                        className="w-full bg-black/40 border border-white/5 rounded-2xl py-3.5 pl-12 pr-5 text-sm font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all placeholder:text-slate-600 outline-none"
                    />
                </div>
            </div>

            <div className="flex items-center gap-4">
                <select 
                    className="bg-black/40 border border-white/10 rounded-2xl px-6 py-3.5 text-[10px] font-black uppercase tracking-widest text-slate-400 outline-none focus:border-indigo-500 transition-all cursor-pointer"
                    value={filterStatus}
                    onChange={e => setFilterStatus(e.target.value)}
                >
                    <option value="">Status: All States</option>
                    <option value="pending">Queued</option>
                    <option value="in_review">In Review</option>
                    <option value="resolved">Patched</option>
                </select>
                <select 
                    className="bg-black/40 border border-white/10 rounded-2xl px-6 py-3.5 text-[10px] font-black uppercase tracking-widest text-slate-400 outline-none focus:border-indigo-500 transition-all cursor-pointer"
                    value={filterType}
                    onChange={e => setFilterType(e.target.value)}
                >
                    <option value="">Category: All Intel</option>
                    <option value="suggestion">💡 Suggestion</option>
                    <option value="bug">🐛 Bug Report</option>
                    <option value="trouble">⚠️ Infrastructure</option>
                </select>
            </div>
        </div>

        {isLoading ? (
            <div className="flex flex-col items-center justify-center h-80 gap-6">
                 <div className="relative">
                    <div className="absolute -inset-8 bg-indigo-500/10 rounded-full blur-2xl animate-pulse"></div>
                    <div className="w-16 h-16 border-4 border-indigo-500/10 border-t-indigo-500 rounded-full animate-spin relative z-10"></div>
                </div>
                <p className="text-slate-500 text-[10px] font-black uppercase tracking-[0.3em] animate-pulse">Syncing Global Intel Registry</p>
            </div>
        ) : feedback.length === 0 ? (
            <div className="py-32 flex flex-col items-center justify-center text-center card-glass bg-white/[0.01] border-white/10">
                 <div className="w-24 h-24 bg-white/[0.02] border border-white/5 rounded-[2.5rem] flex items-center justify-center mb-8 shadow-2xl">
                    <Sparkles className="w-10 h-10 text-slate-800" />
                </div>
                <h3 className="text-[10px] font-black uppercase tracking-[0.4em] text-slate-600">No telemetry records found for current manifest</h3>
                <p className="text-slate-700 text-sm mt-4 font-medium">The feedback registry is currently clear of pending alerts.</p>
            </div>
        ) : (
            <div className="grid grid-cols-1 xl:grid-cols-2 gap-8">
            {feedback.map(item => (
                <FeedbackCard 
                key={item.id} 
                item={item} 
                onUpdate={handleUpdateStatus} 
                onDelete={handleDelete}
                />
            ))}
            </div>
        )}
      </div>
    </div>
  )
}

const FeedbackCard = memo(({ item, onUpdate, onDelete }) => {
  const typeConfigs = {
    bug: { color: 'text-rose-400', icon: Bug, glow: 'bg-rose-500/10', label: 'Critical Bug' },
    trouble: { color: 'text-amber-400', icon: AlertTriangle, glow: 'bg-amber-500/10', label: 'Infra Issue' },
    suggestion: { color: 'text-indigo-400', icon: Lightbulb, glow: 'bg-indigo-500/10', label: 'Suggestion' },
  }

  const config = typeConfigs[item.type] || typeConfigs.suggestion
  const Icon = config.icon

  return (
    <div className="card-glass bg-white/[0.01] border-white/10 group relative overflow-hidden transition-all duration-500 hover:bg-white/[0.03] hover:border-white/20 p-8">
      {/* Background Accent */}
      <div className={`absolute top-0 right-0 w-48 h-48 blur-[100px] pointer-events-none transition-opacity duration-700 opacity-10 group-hover:opacity-20 ${config.glow}`} />

      <div className="relative flex flex-col h-full z-10">
        <div className="flex items-start justify-between mb-8">
          <div className="flex items-center gap-4">
             <div className="w-12 h-12 bg-white/[0.03] border border-white/10 rounded-2xl flex items-center justify-center text-white text-lg font-black group-hover:border-indigo-500/40 group-hover:bg-indigo-500/5 transition-all duration-500 shadow-xl">
              {item.user?.name?.charAt(0).toUpperCase()}
            </div>
            <div>
              <p className="text-sm font-black text-white uppercase tracking-tight group-hover:text-indigo-400 transition-colors">{item.user?.name}</p>
              <div className="flex items-center gap-2 mt-1">
                 <User className="w-3 h-3 text-slate-600" />
                 <p className="text-[10px] text-slate-500 font-medium tracking-tight whitespace-nowrap overflow-hidden text-ellipsis max-w-[150px]">{item.user?.email}</p>
              </div>
            </div>
          </div>

          <div className="flex flex-col items-end gap-2">
            <span className={`text-[9px] font-black uppercase tracking-[0.2em] px-4 py-1.5 rounded-full border ${
              item.status === 'resolved' ? 'bg-emerald-500/5 text-emerald-400 border-emerald-500/20' : 
              item.status === 'in_review' ? 'bg-indigo-500/5 text-indigo-400 border-indigo-500/20' : 'bg-slate-500/5 text-slate-500 border-white/5'
            }`}>
              {item.status.replace('_', ' ')}
            </span>
            <div className="flex items-center gap-1.5 text-slate-700">
                <Clock size={10} />
                <span className="text-[8px] font-black uppercase tracking-widest">{new Date(item.created_at).toLocaleDateString()}</span>
            </div>
          </div>
        </div>

        <div className="space-y-6 mb-10">
          <div className="flex items-center gap-3">
            <div className={`w-8 h-8 rounded-xl bg-white/[0.02] border border-white/5 flex items-center justify-center ${config.color} shadow-lg`}>
                <Icon size={14} />
            </div>
            <h3 className="text-xl font-black text-white tracking-tighter leading-tight bg-clip-text truncate max-w-[80%]">{item.title}</h3>
          </div>
          <div className="relative group/msg">
              <div className="absolute -inset-0.5 bg-gradient-to-r from-indigo-500/20 to-purple-500/20 rounded-2xl blur opacity-0 group-hover/msg:opacity-100 transition duration-500" />
              <p className="relative text-sm text-slate-400 leading-relaxed bg-black/40 p-6 rounded-2xl border border-white/5 font-medium italic min-h-[100px]">
                "{item.content}"
              </p>
          </div>
        </div>

        <div className="mt-auto pt-8 border-t border-white/5 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <button 
              onClick={() => onUpdate(item.id, 'in_review')}
              className="px-5 py-2.5 bg-indigo-500/10 hover:bg-indigo-500/20 text-indigo-400 text-[10px] font-black uppercase tracking-widest rounded-xl transition-all border border-indigo-500/10 active:scale-95"
            >
              Analyze
            </button>
            <button 
              onClick={() => onUpdate(item.id, 'resolved')}
              className="px-5 py-2.5 bg-emerald-500/10 hover:bg-emerald-500/20 text-emerald-400 text-[10px] font-black uppercase tracking-widest rounded-xl transition-all border border-emerald-500/10 active:scale-95"
            >
              Resolve
            </button>
          </div>

          <div className="flex items-center gap-4">
            <button 
                onClick={() => onDelete(item.id)}
                className="w-10 h-10 flex items-center justify-center bg-rose-500/5 hover:bg-rose-500/10 text-rose-500/40 hover:text-rose-500 rounded-xl transition-all border border-white/5 active:scale-90"
                title="Purge Intel"
            >
                <Trash2 size={16} />
            </button>
          </div>
        </div>
      </div>
    </div>
  )
})

export default AdminFeedback
