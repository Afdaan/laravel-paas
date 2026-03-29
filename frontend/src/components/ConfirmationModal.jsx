import { Fragment } from 'react'
import { AlertTriangle, Info, AlertOctagon } from 'lucide-react'

export default function ConfirmationModal({ isOpen, onClose, onConfirm, title, message, type = 'danger', confirmText = 'Confirm', cancelText = 'Cancel' }) {
  if (!isOpen) return null

  const configs = {
    danger: {
      bg: 'bg-rose-500/10',
      border: 'border-rose-500/20',
      text: 'text-rose-400',
      btn: 'bg-rose-600 hover:bg-rose-700 text-white',
      icon: AlertOctagon
    },
    warning: {
      bg: 'bg-amber-500/10',
      border: 'border-amber-500/20',
      text: 'text-amber-400',
      btn: 'btn-primary',
      icon: AlertTriangle
    },
    info: {
      bg: 'bg-indigo-500/10',
      border: 'border-indigo-500/20',
      text: 'text-indigo-400',
      btn: 'btn-primary',
      icon: Info
    }
  }

  const config = configs[type] || configs.danger
  const Icon = config.icon

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center p-4">
      {/* Backdrop */}
      <div 
        className="fixed inset-0 bg-black/60 backdrop-blur-md transition-opacity duration-300" 
        onClick={onClose}
      />

      {/* Modal Panel */}
      <div className="relative w-full max-w-md card-glass border-white/10 overflow-hidden shadow-[0_0_50px_rgba(0,0,0,0.5)] animate-pop-in">
        <div className="p-8">
          <div className="flex flex-col items-center text-center">
            <div className={`w-16 h-16 rounded-2xl flex items-center justify-center mb-6 ${config.bg} border ${config.border} ${config.text}`}>
              <Icon className="w-8 h-8" />
            </div>
            
            <h3 className="text-2xl font-black text-white tracking-tight mb-2">
              {title}
            </h3>
            
            <p className="text-slate-400 text-sm font-medium leading-relaxed">
              {message}
            </p>
          </div>
        </div>
        
        <div className="p-6 bg-white/[0.02] border-t border-white/[0.03] flex flex-col gap-3">
          <button
            type="button"
            className={`btn w-full py-4 text-base font-black uppercase tracking-widest ${config.btn}`}
            onClick={() => {
              onConfirm()
              onClose()
            }}
          >
            {confirmText}
          </button>
          <button
            type="button"
            className="btn btn-secondary w-full py-4 text-base font-black uppercase tracking-widest"
            onClick={onClose}
          >
            {cancelText}
          </button>
        </div>
      </div>
    </div>
  )
}

