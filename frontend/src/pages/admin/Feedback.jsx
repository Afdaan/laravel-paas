// ===========================================
// Admin Feedback Management Page
// ===========================================

import { useState, useEffect, memo } from 'react'
import { feedbackAPI } from '../../services/api'
import toast from 'react-hot-toast'

function AdminFeedback() {
  const [feedback, setFeedback] = useState([])
  const [isLoading, setIsLoading] = useState(true)
  const [filterStatus, setFilterStatus] = useState('')
  const [filterType, setFilterType] = useState('')

  useEffect(() => {
    fetchFeedback()
  }, [filterStatus, filterType])

  const fetchFeedback = async () => {
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
  }

  const handleUpdateStatus = async (id, status) => {
    try {
      await feedbackAPI.updateStatus(id, status)
      toast.success('Status updated')
      fetchFeedback()
    } catch (error) {
      toast.error('Failed to update status')
    }
  }

  const handleDelete = async (id) => {
    if (!window.confirm('Delete this feedback entry?')) return
    try {
      await feedbackAPI.delete(id)
      toast.success('Entry removed')
      fetchFeedback()
    } catch (error) {
      toast.error('Failed to delete')
    }
  }

  return (
    <div className="space-y-8">
      <div className="flex flex-col md:flex-row md:items-end justify-between gap-6">
        <div>
          <h1 className="text-3xl font-bold text-white tracking-tight">Client Feedback</h1>
          <p className="text-slate-400 mt-2">Manage suggestions and bug reports from users.</p>
        </div>

        <div className="flex items-center gap-4">
          <select 
            className="input text-xs font-bold"
            value={filterStatus}
            onChange={e => setFilterStatus(e.target.value)}
          >
            <option value="">All Status</option>
            <option value="pending">Pending</option>
            <option value="in_review">In Review</option>
            <option value="resolved">Resolved</option>
          </select>
          <select 
            className="input text-xs font-bold"
            value={filterType}
            onChange={e => setFilterType(e.target.value)}
          >
            <option value="">All Categories</option>
            <option value="suggestion">💡 Suggestion</option>
            <option value="bug">🐛 Bug</option>
            <option value="trouble">⚠️ Trouble</option>
          </select>
        </div>
      </div>

      {isLoading ? (
        <div className="flex flex-col items-center justify-center h-64 bg-white/[0.01] rounded-3xl border border-white/5">
          <div className="w-10 h-10 border-4 border-indigo-500/20 border-t-indigo-500 rounded-full animate-spin"></div>
        </div>
      ) : feedback.length === 0 ? (
        <div className="p-20 text-center bg-white/[0.01] border border-dashed border-white/10 rounded-3xl">
          <p className="text-slate-600 font-bold uppercase tracking-widest">No feedback found</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 xl:grid-cols-2 gap-6">
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
  )
}

const FeedbackCard = memo(({ item, onUpdate, onDelete }) => {
  return (
    <div className="card bg-[#111114] border-white/[0.03] group relative overflow-hidden">
      {/* Background Accent */}
      <div className={`absolute top-0 right-0 w-32 h-32 blur-[80px] opacity-10 pointer-events-none ${
        item.type === 'bug' ? 'bg-red-500' : 
        item.type === 'trouble' ? 'bg-amber-500' : 'bg-blue-500'
      }`} />

      <div className="relative flex flex-col h-full">
        <div className="flex items-start justify-between mb-4">
          <div className="flex items-center gap-3">
             <div className="w-10 h-10 bg-slate-800 rounded-xl flex items-center justify-center text-white font-bold ring-2 ring-white/5 shadow-inner">
              {item.user?.name?.charAt(0).toUpperCase()}
            </div>
            <div>
              <p className="text-xs font-black text-white uppercase tracking-tight">{item.user?.name}</p>
              <p className="text-[10px] text-slate-500">{item.user?.email}</p>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <span className={`text-[10px] font-black uppercase px-2.5 py-1 rounded-lg ${
              item.status === 'resolved' ? 'bg-emerald-500/10 text-emerald-400' : 
              item.status === 'in_review' ? 'bg-indigo-500/10 text-indigo-400' : 'bg-slate-500/10 text-slate-400'
            }`}>
              {item.status.replace('_', ' ')}
            </span>
          </div>
        </div>

        <div className="space-y-2 mb-6">
          <div className="flex items-center gap-2">
            <span className={`text-[9px] font-black uppercase px-1.5 py-0.5 rounded ${
              item.type === 'bug' ? 'bg-red-600 text-white' : 
              item.type === 'trouble' ? 'bg-amber-500 text-white' : 'bg-blue-600 text-white'
            }`}>
              {item.type}
            </span>
            <h3 className="text-lg font-bold text-white tracking-tight">{item.title}</h3>
          </div>
          <p className="text-sm text-slate-400 leading-relaxed bg-white/[0.02] p-4 rounded-xl border border-white/[0.03]">
            {item.content}
          </p>
        </div>

        <div className="mt-auto pt-6 border-t border-white/[0.03] flex items-center justify-between">
          <div className="flex items-center gap-2">
            <button 
              onClick={() => onUpdate(item.id, 'in_review')}
              className="px-3 py-1.5 bg-indigo-600/10 hover:bg-indigo-600/20 text-indigo-400 text-[10px] font-black uppercase tracking-widest rounded-lg transition-all"
            >
              Review
            </button>
            <button 
              onClick={() => onUpdate(item.id, 'resolved')}
              className="px-3 py-1.5 bg-emerald-500/10 hover:bg-emerald-500/20 text-emerald-400 text-[10px] font-black uppercase tracking-widest rounded-lg transition-all"
            >
              Resolve
            </button>
          </div>

          <div className="flex items-center gap-4">
            <span className="text-[10px] text-slate-600 font-bold uppercase tracking-widest">
                {new Date(item.created_at).toLocaleDateString()}
            </span>
            <button 
                onClick={() => onDelete(item.id)}
                className="p-2 bg-red-500/10 hover:bg-red-500/20 text-red-400 rounded-lg transition-all"
            >
                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                </svg>
            </button>
          </div>
        </div>
      </div>
    </div>
  )
})

export default AdminFeedback
