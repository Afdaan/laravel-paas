// ===========================================
// Student Feedback Page
// ===========================================

import { useState, useEffect } from 'react'
import { feedbackAPI } from '../../services/api'
import toast from 'react-hot-toast'

function StudentFeedback() {
  const [feedback, setFeedback] = useState([])
  const [isLoading, setIsLoading] = useState(true)
  const [isSubmitting, setIsSubmitting] = useState(false)
  
  const [formData, setFormData] = useState({
    title: '',
    content: '',
    type: 'suggestion'
  })

  useEffect(() => {
    fetchFeedback()
  }, [])

  const fetchFeedback = async () => {
    try {
      const res = await feedbackAPI.listOwn()
      setFeedback(res.data || [])
    } catch (error) {
      console.error('Failed to fetch feedback:', error)
    } finally {
      setIsLoading(false)
    }
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    if (!formData.title || !formData.content) {
      return toast.error('Please fill in all fields')
    }

    setIsSubmitting(true)
    try {
      await feedbackAPI.submit(formData)
      toast.success('Feedback submitted successfully!')
      setFormData({ title: '', content: '', type: 'suggestion' })
      fetchFeedback()
    } catch (error) {
      toast.error('Failed to submit feedback')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="max-w-4xl mx-auto space-y-8">
      <div>
        <h1 className="text-3xl font-bold text-white">Feedback & Support</h1>
        <p className="text-slate-400 mt-2">Have a suggestion or found a bug? Let us know!</p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
        {/* Form */}
        <div className="lg:col-span-2 space-y-6">
          <div className="card bg-[#111114] border-white/[0.03]">
            <form onSubmit={handleSubmit} className="space-y-6">
              <div className="space-y-2">
                <label className="text-sm font-semibold text-slate-300">Subject</label>
                <input 
                  type="text" 
                  className="input w-full"
                  placeholder="e.g., Feature Suggestion: Darker mode"
                  value={formData.title}
                  onChange={e => setFormData({...formData, title: e.target.value})}
                />
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                <div className="space-y-2">
                  <label className="text-sm font-semibold text-slate-300">Category</label>
                  <select 
                    className="input w-full"
                    value={formData.type}
                    onChange={e => setFormData({...formData, type: e.target.value})}
                  >
                    <option value="suggestion">💡 Suggestion</option>
                    <option value="bug">🐛 Bug Report</option>
                    <option value="trouble">⚠️ Technical Issue</option>
                  </select>
                </div>
              </div>

              <div className="space-y-2">
                <label className="text-sm font-semibold text-slate-300">Details</label>
                <textarea 
                  rows="5"
                  className="input w-full resize-none"
                  placeholder="Provide as much detail as possible..."
                  value={formData.content}
                  onChange={e => setFormData({...formData, content: e.target.value})}
                ></textarea>
              </div>

              <button 
                type="submit"
                disabled={isSubmitting}
                className="btn btn-primary w-full py-3 bg-gradient-to-r from-purple-600 to-indigo-600 text-white font-bold tracking-wide shadow-lg shadow-purple-500/20 disabled:opacity-50"
              >
                {isSubmitting ? 'SUBMITTING...' : 'SEND FEEDBACK'}
              </button>
            </form>
          </div>
        </div>

        {/* Status Tracker */}
        <div className="space-y-4">
          <h3 className="text-sm font-bold text-slate-500 uppercase tracking-widest ml-2">Your History</h3>
          <div className="space-y-4">
            {isLoading ? (
              <div className="flex justify-center p-8">
                <div className="animate-spin rounded-full h-6 w-6 border-t-2 border-indigo-500"></div>
              </div>
            ) : feedback.length === 0 ? (
              <div className="p-8 text-center bg-white/[0.01] border border-dashed border-white/5 rounded-2xl">
                <p className="text-slate-600 text-sm">No previous entries</p>
              </div>
            ) : (
              feedback.map(item => (
                <div key={item.id} className="card p-4 bg-[#1a1a1e] border-white/[0.02]">
                  <div className="flex items-start justify-between mb-2">
                    <span className={`text-[10px] font-black uppercase px-2 py-0.5 rounded-full ${
                      item.type === 'bug' ? 'bg-red-500/10 text-red-400' : 
                      item.type === 'trouble' ? 'bg-amber-500/10 text-amber-400' : 'bg-blue-500/10 text-blue-400'
                    }`}>
                      {item.type}
                    </span>
                    <span className={`text-[10px] font-black uppercase ${
                      item.status === 'resolved' ? 'text-emerald-500' : 'text-slate-500'
                    }`}>
                      {item.status.replace('_', ' ')}
                    </span>
                  </div>
                  <h4 className="text-sm font-bold text-white truncate">{item.title}</h4>
                  <p className="text-[11px] text-slate-500 mt-1 line-clamp-2">{item.content}</p>
                  <p className="text-[9px] text-slate-700 mt-3">
                    {new Date(item.created_at).toLocaleDateString()}
                  </p>
                </div>
              ))
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

export default StudentFeedback
