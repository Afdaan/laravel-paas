// ===========================================
// Student Feedback & Manifesting Support
// ===========================================

import { useState, useEffect, useCallback } from 'react'
import { feedbackAPI } from '../../services/api'
import toast from 'react-hot-toast'
import { 
  MessageSquare, 
  Send, 
  History, 
  CheckCircle2, 
  AlertTriangle, 
  Lightbulb,
  ArrowRight,
  User,
  ShieldCheck,
  Zap,
  Clock,
  Layout,
  Terminal
} from 'lucide-react'

const StudentFeedback = () => {
  const [feedback, setFeedback] = useState([])
  const [isLoading, setIsLoading] = useState(true)
  const [isSubmitting, setIsSubmitting] = useState(false)
  
  const [formData, setFormData] = useState({
    title: '',
    content: '',
    type: 'suggestion'
  })

  const fetchFeedback = useCallback(async () => {
    setIsLoading(true)
    try {
      const res = await feedbackAPI.listOwn()
      setFeedback(res.data || [])
    } catch (error) {
      console.error('Failed to sync response manifest:', error)
    } finally {
      setIsLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchFeedback()
  }, [fetchFeedback])

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!formData.title || !formData.content) {
      return toast.error('Please complete the manifest fields')
    }

    setIsSubmitting(true)
    try {
      await feedbackAPI.submit(formData)
      toast.success('Feedback uplink established')
      setFormData({ title: '', content: '', type: 'suggestion' })
      fetchFeedback()
    } catch (error) {
      toast.error('Transmission failed')
    } finally {
      setIsSubmitting(false)
    }
  }

  const getTypeIcon = (type) => {
    switch(type) {
      case 'bug': return <ShieldCheck className="w-3.5 h-3.5" />;
      case 'trouble': return <AlertTriangle className="w-3.5 h-3.5" />;
      default: return <Lightbulb className="w-3.5 h-3.5" />;
    }
  }

  const getStatusColor = (status) => {
    switch(status) {
      case 'resolved': return 'text-emerald-400 border-emerald-400/20 bg-emerald-400/5';
      case 'in_progress': return 'text-amber-400 border-amber-400/20 bg-amber-400/5';
      default: return 'text-slate-500 border-white/10 bg-white/5';
    }
  }

  return (
    <div className="space-y-16 animate-pop-in relative">
      {/* Background Ambience */}
      <div className="absolute top-0 right-0 w-[40vw] h-[40vw] bg-indigo-600/5 blur-[120px] rounded-full pointer-events-none z-0"></div>

      <div className="relative z-10">
        <div className="mb-16">
          <h1 className="text-5xl font-black text-white tracking-tighter mb-4 italic">Platform <span className="text-transparent bg-clip-text bg-gradient-to-r from-indigo-400 to-purple-400">Intelligence</span></h1>
          <p className="text-slate-400 text-lg font-medium">Contribute to the cluster evolution or report anomalies to our engineering fleet.</p>
        </div>

        <div className="grid grid-cols-1 xl:grid-cols-12 gap-12">
          {/* Submission Interface */}
          <div className="xl:col-span-8 space-y-10">
            <div className="card-glass p-10 bg-white/[0.01] border-white/10 group overflow-hidden relative transition-all duration-500 hover:bg-white/[0.02]">
                <div className="absolute top-0 right-0 w-32 h-32 bg-indigo-500/5 blur-3xl pointer-events-none"></div>
                
                <form onSubmit={handleSubmit} className="space-y-10">
                  <div className="space-y-3">
                    <label className="text-[10px] font-black text-slate-500 uppercase tracking-[0.2em] ml-1 flex items-center gap-2">
                       <MessageSquare size={12} className="text-indigo-400" />
                       Manifest Subject
                    </label>
                    <input 
                      type="text" 
                      className="w-full bg-black/40 border border-white/10 rounded-2xl px-6 py-5 text-white font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all outline-none"
                      placeholder="e.g., Performance spike in container provisioning"
                      value={formData.title}
                      onChange={e => setFormData({...formData, title: e.target.value})}
                    />
                  </div>

                  <div className="grid grid-cols-1 md:grid-cols-2 gap-10">
                    <div className="space-y-3">
                      <label className="text-[10px] font-black text-slate-500 uppercase tracking-[0.2em] ml-1 flex items-center gap-2">
                         <Layout size={12} className="text-purple-400" />
                         Logic Classification
                      </label>
                      <div className="relative">
                        <select 
                          className="w-full bg-black/40 border border-white/10 rounded-2xl px-6 py-5 text-white font-medium focus:outline-none focus:ring-2 focus:ring-purple-500/20 focus:border-purple-500/40 transition-all outline-none appearance-none"
                          value={formData.type}
                          onChange={e => setFormData({...formData, type: e.target.value})}
                        >
                          <option value="suggestion">💡 Evolution Suggestion</option>
                          <option value="bug">🐛 Anomaly Report</option>
                          <option value="trouble">⚠️ Critical Failure</option>
                        </select>
                        <div className="absolute inset-y-0 right-6 flex items-center pointer-events-none text-slate-500 italic text-[10px] font-black uppercase tracking-widest">Select Mode</div>
                      </div>
                    </div>

                    <div className="flex items-center">
                        <div className="p-6 rounded-2xl bg-indigo-500/5 border border-indigo-500/10 flex items-center gap-4 w-full">
                            <Zap className="w-5 h-5 text-indigo-400" />
                            <p className="text-[11px] text-slate-500 font-medium leading-relaxed italic">Your contribution helps us maintain a <span className="text-indigo-400 font-black">99.9% uptime</span> across the student cluster architecture.</p>
                        </div>
                    </div>
                  </div>

                  <div className="space-y-3">
                    <label className="text-[10px] font-black text-slate-500 uppercase tracking-[0.2em] ml-1 flex items-center gap-2">
                       <Terminal size={12} className="text-rose-400" />
                       Detailed Manifest
                    </label>
                    <textarea 
                      rows="6"
                      className="w-full bg-black/40 border border-white/10 rounded-3xl px-6 py-6 text-white font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all outline-none resize-none"
                      placeholder="Describe the technical details of your encounter..."
                      value={formData.content}
                      onChange={e => setFormData({...formData, content: e.target.value})}
                    ></textarea>
                  </div>

                  <button 
                    type="submit"
                    disabled={isSubmitting}
                    className="btn btn-primary w-full py-6 text-sm font-black uppercase tracking-[0.3em] shadow-[0_15px_30px_rgba(99,102,241,0.25)] flex items-center justify-center gap-4 group/btn disabled:opacity-50"
                  >
                    {isSubmitting ? (
                      <div className="w-5 h-5 border-2 border-white/20 border-t-white rounded-full animate-spin" />
                    ) : (
                      <Send className="w-4 h-4 group-hover:translate-x-1 group-hover:-translate-y-1 transition-transform" />
                    )}
                    {isSubmitting ? 'ESTABLISHING UPLINK...' : 'TRANSMIT MANIFEST'}
                  </button>
                </form>
            </div>
          </div>

          {/* Activity Log */}
          <div className="xl:col-span-4 space-y-8">
            <div className="flex items-center justify-between ml-2">
                 <div className="flex items-center gap-2">
                    <History className="w-4 h-4 text-slate-500" />
                    <h3 className="text-xs font-black text-slate-500 uppercase tracking-[0.2em]">Activity Log</h3>
                 </div>
                 <span className="text-[10px] font-black text-indigo-400 bg-indigo-500/5 px-2 py-0.5 rounded uppercase tracking-widest">{feedback.length} Entries</span>
            </div>

            <div className="space-y-6 max-h-[850px] overflow-y-auto pr-2 custom-scrollbar">
              {isLoading ? (
                <div className="flex flex-col items-center justify-center p-20 gap-4 opacity-30">
                  <div className="w-10 h-10 border-4 border-indigo-500/20 border-t-indigo-500 rounded-full animate-spin"></div>
                  <p className="text-[10px] font-black uppercase tracking-widest">Syncing History</p>
                </div>
              ) : feedback.length === 0 ? (
                <div className="p-16 text-center bg-white/[0.01] border border-dashed border-white/5 rounded-3xl flex flex-col items-center gap-5">
                  <div className="w-16 h-16 rounded-2xl bg-white/5 flex items-center justify-center">
                    <MessageSquare className="w-8 h-8 text-slate-800" />
                  </div>
                  <p className="text-slate-600 text-xs font-bold uppercase tracking-widest">Manifest Void Detected</p>
                </div>
              ) : (
                feedback.map(item => (
                  <div key={item.id} className="card-glass p-8 bg-white/[0.01] border-white/5 group hover:bg-white/[0.02] transition-all duration-300 relative overflow-hidden">
                    {/* Status Glow */}
                    <div className={`absolute -right-4 -top-4 w-12 h-12 blur-2xl opacity-20 pointer-events-none ${
                        item.type === 'bug' ? 'bg-red-500' : 
                        item.type === 'trouble' ? 'bg-amber-500' : 'bg-indigo-500'
                    }`}></div>

                    <div className="flex items-center justify-between mb-6">
                      <div className={`flex items-center gap-2 px-3 py-1 rounded-lg border text-[9px] font-black uppercase tracking-widest ${
                        item.type === 'bug' ? 'text-rose-400 border-rose-400/20 bg-rose-400/5' : 
                        item.type === 'trouble' ? 'text-amber-400 border-amber-400/20 bg-amber-400/5' : 'text-indigo-400 border-indigo-400/20 bg-indigo-400/5'
                      }`}>
                        {getTypeIcon(item.type)}
                        {item.type}
                      </div>
                      <div className={`px-2.5 py-1 rounded-full border text-[8px] font-black uppercase tracking-[0.1em] ${getStatusColor(item.status)}`}>
                        {item.status.replace('_', ' ')}
                      </div>
                    </div>

                    <h4 className="text-base font-black text-white truncate mb-2 group-hover:text-indigo-400 transition-colors uppercase tracking-tight">{item.title}</h4>
                    <p className="text-xs text-slate-500 font-medium leading-relaxed italic line-clamp-3 mb-6 pr-4">"{item.content}"</p>
                    
                    <div className="flex items-center justify-between pt-6 border-t border-white/5">
                        <div className="flex items-center gap-2 text-slate-700">
                            <Clock className="w-3.5 h-3.5" />
                            <span className="text-[10px] font-black uppercase tracking-widest">{new Date(item.created_at).toLocaleDateString()}</span>
                        </div>
                        <div className="flex items-center gap-2">
                            <div className="w-6 h-6 rounded-full bg-white/5 border border-white/10 flex items-center justify-center">
                                <User className="w-3 h-3 text-slate-500" />
                            </div>
                        </div>
                    </div>
                  </div>
                ))
              )}
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}

export default StudentFeedback

