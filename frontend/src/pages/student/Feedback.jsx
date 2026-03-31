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
  Terminal,
  Bug,
  Wrench,
  ChevronDown
} from 'lucide-react'

const StudentFeedback = () => {
  const [feedback, setFeedback] = useState([])
  const [isLoading, setIsLoading] = useState(true)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [isDropdownOpen, setIsDropdownOpen] = useState(false)
  
  const [formData, setFormData] = useState({
    title: '',
    content: '',
    type: 'suggestion'
  })

  const typeOptions = [
    { value: 'suggestion', label: 'Feature Suggestion', icon: Lightbulb, color: 'text-indigo-400', bg: 'bg-indigo-500/10' },
    { value: 'bug', label: 'Bug Report', icon: Bug, color: 'text-rose-400', bg: 'bg-rose-500/10' },
    { value: 'trouble', label: 'System Issue', icon: AlertTriangle, color: 'text-amber-400', bg: 'bg-amber-500/10' },
  ]

  const activeType = typeOptions.find(opt => opt.value === formData.type) || typeOptions[0]

  const fetchFeedback = useCallback(async () => {
    setIsLoading(true)
    try {
      const res = await feedbackAPI.listOwn()
      setFeedback(res.data || [])
    } catch (error) {
      console.error('Failed to load feedback:', error)
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
      return toast.error('Please fill in all fields')
    }

    setIsSubmitting(true)
    try {
      await feedbackAPI.submit(formData)
      toast.success('Support ticket created')
      setFormData({ title: '', content: '', type: 'suggestion' })
      fetchFeedback()
    } catch (error) {
      console.error('Feedback creation error:', error)
      const errorMsg = error.response?.data?.error || error.message || 'Failed to create feedback'
      toast.error(typeof errorMsg === 'string' ? errorMsg : 'Failed to create feedback')
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
          <h1 className="text-5xl font-black text-white tracking-tighter mb-4 italic">Support <span className="text-transparent bg-clip-text bg-gradient-to-r from-indigo-400 to-purple-400">Hub</span></h1>
          <p className="text-slate-400 text-lg font-medium">Send us your feedback or report bugs to our team.</p>
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
                       Support Subject
                    </label>
                    <input 
                      type="text" 
                      className="w-full bg-black/40 border border-white/10 rounded-2xl px-6 py-5 text-white font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all outline-none"
                      placeholder="e.g., Issue with project setup"
                      value={formData.title}
                      onChange={e => setFormData({...formData, title: e.target.value})}
                    />
                  </div>

                  <div className="grid grid-cols-1 md:grid-cols-2 gap-10">
                    <div className="space-y-3">
                      <label className="text-[10px] font-black text-slate-500 uppercase tracking-[0.2em] ml-1 flex items-center gap-2">
                         <Layout size={12} className="text-purple-400" />
                         Category
                      </label>
                      <div className="relative">
                        <button
                          type="button"
                          onClick={() => setIsDropdownOpen(!isDropdownOpen)}
                          className="w-full bg-black/40 border border-white/10 rounded-2xl px-6 py-5 text-white font-medium focus:outline-none focus:ring-2 focus:ring-purple-500/20 focus:border-purple-500/40 transition-all outline-none flex items-center justify-between group"
                        >
                          <div className="flex items-center gap-3">
                            <activeType.icon className={`w-5 h-5 ${activeType.color}`} />
                            <span>{activeType.label}</span>
                          </div>
                          <ChevronDown className={`w-4 h-4 text-slate-500 transition-transform duration-300 ${isDropdownOpen ? 'rotate-180' : ''}`} />
                        </button>

                        {isDropdownOpen && (
                          <>
                            <div 
                              className="fixed inset-0 z-40" 
                              onClick={() => setIsDropdownOpen(false)}
                            ></div>
                            <div className="absolute top-full left-0 w-full mt-3 bg-[#0d0d12] border border-white/10 rounded-2xl p-2 z-50 shadow-[0_20px_50px_rgba(0,0,0,0.5)] animate-in fade-in slide-in-from-top-2 duration-200">
                              {typeOptions.map((option) => (
                                <button
                                  key={option.value}
                                  type="button"
                                  onClick={() => {
                                    setFormData({ ...formData, type: option.value })
                                    setIsDropdownOpen(false)
                                  }}
                                  className={`w-full flex items-center gap-4 px-4 py-4 rounded-xl transition-all ${
                                    formData.type === option.value 
                                      ? 'bg-white/5 text-white' 
                                      : 'text-slate-400 hover:bg-white/[0.02] hover:text-slate-200'
                                  }`}
                                >
                                  <div className={`w-10 h-10 rounded-lg ${option.bg} flex items-center justify-center`}>
                                    <option.icon className={`w-5 h-5 ${option.color}`} />
                                  </div>
                                  <div className="text-left">
                                    <p className="text-sm font-bold tracking-tight">{option.label}</p>
                                    <p className="text-[10px] text-slate-500 font-medium uppercase tracking-widest mt-0.5">
                                      {option.value === 'suggestion' ? 'New Feature' : option.value === 'bug' ? 'Bug Report' : 'Technical Issue'}
                                    </p>
                                  </div>
                                </button>
                              ))}
                            </div>
                          </>
                        )}
                      </div>
                    </div>

                    <div className="flex items-center">
                        <div className="p-6 rounded-2xl bg-indigo-500/5 border border-indigo-500/10 flex items-center gap-4 w-full">
                            <Zap className="w-5 h-5 text-indigo-400" />
                            <p className="text-[11px] text-slate-500 font-medium leading-relaxed italic">Your contribution helps us maintain high uptime across the <span className="text-indigo-400 font-black">platform</span>.</p>
                        </div>
                    </div>
                  </div>

                  <div className="space-y-3">
                    <label className="text-[10px] font-black text-slate-500 uppercase tracking-[0.2em] ml-1 flex items-center gap-2">
                       <Terminal size={12} className="text-rose-400" />
                       Details
                    </label>
                    <textarea 
                      rows="6"
                      className="w-full bg-black/40 border border-white/10 rounded-3xl px-6 py-6 text-white font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/20 focus:border-indigo-500/40 transition-all outline-none resize-none"
                      placeholder="Describe the issue or suggestion..."
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
                    {isSubmitting ? 'Sending...' : 'Submit Feedback'}
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
                  <p className="text-[10px] font-black uppercase tracking-widest">Loading History</p>
                </div>
              ) : feedback.length === 0 ? (
                <div className="p-16 text-center bg-white/[0.01] border border-dashed border-white/5 rounded-3xl flex flex-col items-center gap-5">
                  <div className="w-16 h-16 rounded-2xl bg-white/5 flex items-center justify-center">
                    <MessageSquare className="w-8 h-8 text-slate-800" />
                  </div>
                  <p className="text-slate-600 text-xs font-bold uppercase tracking-widest">No Feedback Found</p>
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
                        {item.type === 'bug' ? <Bug className="w-3.5 h-3.5" /> : 
                         item.type === 'trouble' ? <AlertTriangle className="w-3.5 h-3.5" /> : 
                         <Lightbulb className="w-3.5 h-3.5" />}
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

