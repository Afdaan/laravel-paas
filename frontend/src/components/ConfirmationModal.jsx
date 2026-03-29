import { Fragment } from 'react'
import { AlertTriangle, Info, AlertOctagon, X } from 'lucide-react'

export default function ConfirmationModal({ isOpen, onClose, onConfirm, title, message, type = 'danger', confirmText = 'Confirm', cancelText = 'Cancel' }) {
  if (!isOpen) return null

  const configs = {
    danger: {
      bg: 'bg-rose-500/10',
      border: 'border-rose-500/20',
      text: 'text-rose-400',
      btn: 'bg-rose-600 hover:bg-rose-500 text-white',
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
      <div className="relative w-full max-w-sm bg-[#09090b] border border-white/5 rounded-[2rem] overflow-hidden shadow-2xl shadow-black animate-in zoom-in-95 slide-in-from-bottom-4 duration-300">
        <button 
          onClick={onClose}
          className="absolute top-6 right-6 p-2 rounded-xl text-slate-500 hover:text-white hover:bg-white/5 transition-colors"
        >
          <X size={16} />
        </button>

        <div className="p-10 text-center flex flex-col items-center">
          <div className={`w-14 h-14 rounded-2xl flex items-center justify-center mb-8 ${config.bg} border ${config.border} ${config.text} shadow-xl`}>
            <Icon size={24} strokeWidth={2.5} />
          </div>
          
          <h3 className="text-2xl font-black text-white tracking-tighter mb-4 leading-none lowercase">
            {title}
          </h3>
          
          <p className="text-slate-500 text-xs font-medium leading-relaxed uppercase tracking-widest max-w-[240px]">
            {message}
          </p>
        </div>
        
        <div className="p-10 pt-0 flex flex-col gap-3">
          <button
            type="button"
            className={`w-full py-4 text-[10px] font-black uppercase tracking-[0.3em] rounded-xl transition-all active:scale-[0.98] shadow-2xl shadow-white/5 ${config.btn}`}
            onClick={() => {
              onConfirm()
              onClose()
            }}
          >
            {confirmText}
          </button>
          <button
            type="button"
            className="w-full py-4 text-[10px] font-black uppercase tracking-[0.3em] rounded-xl bg-white/5 text-slate-400 hover:text-white hover:bg-white/10 transition-all border border-white/5"
            onClick={onClose}
          >
            {cancelText}
          </button>
        </div>
      </div>
    </div>
  )
}

