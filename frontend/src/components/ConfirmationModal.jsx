import { Fragment } from 'react'
import { AlertTriangle, Info, AlertOctagon, X } from 'lucide-react'

export default function ConfirmationModal({ isOpen, onClose, onConfirm, title, message, type = 'danger', confirmText = 'Confirm', cancelText = 'Cancel' }) {
  if (!isOpen) return null

  const configs = {
    danger: {
      bg: 'bg-rose-500/10',
      border: 'border-rose-500/20',
      text: 'text-rose-400',
      btn: 'bg-rose-600 hover:bg-rose-500 text-slate-900 dark:text-white',
      icon: AlertOctagon
    },
    warning: {
      bg: 'bg-amber-500/10',
      border: 'border-amber-500/20',
      text: 'text-amber-400',
      btn: 'bg-white text-black hover:bg-slate-200',
      icon: AlertTriangle
    },
    info: {
      bg: 'bg-indigo-500/10',
      border: 'border-indigo-500/20',
      text: 'text-indigo-400',
      btn: 'bg-white text-black hover:bg-slate-200',
      icon: Info
    }
  }

  const config = configs[type] || configs.danger
  const Icon = config.icon

  return (
    <div className="fixed inset-0 z-[200] flex items-center justify-center p-6 animate-in fade-in duration-300">
      {/* Precision Backdrop */}
      <div 
        className="fixed inset-0 bg-black/60 transition-opacity duration-300" 
        onClick={onClose}
      />

      {/* Control Panel */}
      <div className="relative w-full max-w-[320px] bg-[#09090b] border border-slate-300 dark:border-white/10 rounded-[2.5rem] overflow-hidden shadow-2xl shadow-black animate-in zoom-in-95 slide-in-from-bottom-6 duration-300">
        <button 
          onClick={onClose}
          className="absolute top-5 right-5 p-1.5 rounded-lg text-slate-600 hover:text-slate-900 dark:text-white hover:bg-slate-100 dark:bg-white/5 transition-colors"
        >
          <X size={14} />
        </button>

        <div className="p-8 text-center flex flex-col items-center">
          <div className={`w-12 h-12 rounded-xl flex items-center justify-center mb-6 ${config.bg} border ${config.border} ${config.text} shadow-lg shadow-black/40`}>
            <Icon size={20} strokeWidth={2.5} />
          </div>
          
          <h3 className="text-xl font-black text-slate-900 dark:text-white tracking-tighter mb-3 leading-none lowercase">
            {title}
          </h3>
          
          <p className="text-slate-600 dark:text-slate-400 text-[10px] font-bold leading-relaxed uppercase tracking-[0.2em] max-w-[200px]">
            {message}
          </p>
        </div>
        
        <div className="p-8 pt-2 flex flex-col gap-2.5">
          <button
            type="button"
            className={`w-full py-4 text-[9px] font-black uppercase tracking-[0.35em] rounded-xl transition-all active:scale-[0.97] ${config.btn}`}
            onClick={() => {
              onConfirm()
              onClose()
            }}
          >
            {confirmText}
          </button>
          <button
            type="button"
            className="w-full py-4 text-[9px] font-black uppercase tracking-[0.35em] rounded-xl bg-slate-100 dark:bg-slate-100 dark:bg-white/5 text-slate-600 dark:text-slate-400 hover:text-slate-900 dark:text-white hover:bg-white/[0.06] transition-all border border-slate-200 dark:border-white/5"
            onClick={onClose}
          >
            {cancelText}
          </button>
        </div>
      </div>
    </div>
  )
}

