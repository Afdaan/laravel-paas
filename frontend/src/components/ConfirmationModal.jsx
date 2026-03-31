import { Fragment } from 'react'
import { AlertTriangle, Info, AlertOctagon, X } from 'lucide-react'

export default function ConfirmationModal({ isOpen, onClose, onConfirm, title, message, type = 'danger', confirmText = 'Confirm', cancelText = 'Cancel' }) {
  if (!isOpen) return null

  const configs = {
    danger: {
      bg: 'bg-rose-500/10',
      border: 'border-rose-500/20',
      text: 'text-rose-600 dark:text-rose-400',
      btn: 'bg-rose-500 hover:bg-rose-600 text-white',
      icon: AlertOctagon
    },
    warning: {
      bg: 'bg-amber-500/10',
      border: 'border-amber-500/20',
      text: 'text-amber-600 dark:text-amber-400',
      btn: 'bg-indigo-600 dark:bg-slate-100 hover:bg-indigo-700 dark:hover:bg-white text-white dark:text-slate-900',
      icon: AlertTriangle
    },
    info: {
      bg: 'bg-indigo-500/10',
      border: 'border-indigo-500/20',
      text: 'text-indigo-600 dark:text-indigo-400',
      btn: 'bg-indigo-600 dark:bg-slate-100 hover:bg-indigo-700 dark:hover:bg-white text-white dark:text-slate-900',
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
      <div className="relative w-full max-w-[400px] bg-white dark:bg-slate-900 border border-slate-200 dark:border-slate-800 rounded-3xl overflow-hidden shadow-2xl animate-in zoom-in-95 slide-in-from-bottom-6 duration-300">
        <button 
          onClick={onClose}
          className="absolute top-4 right-4 p-2 rounded-xl text-slate-400 hover:text-slate-600 dark:hover:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-800 transition-colors"
        >
          <X size={18} />
        </button>

        <div className="p-8 pb-6 flex flex-col items-center text-center">
          <div className={`w-14 h-14 rounded-full flex items-center justify-center mb-5 ${config.bg} border ${config.border} ${config.text}`}>
            <Icon size={24} strokeWidth={2} />
          </div>
          
          <h3 className="text-xl font-bold text-slate-900 dark:text-white mb-2">
            {title}
          </h3>
          
          <p className="text-slate-500 dark:text-slate-400 text-sm leading-relaxed max-w-sm">
            {message}
          </p>
        </div>
        
        <div className="p-8 pt-0 flex gap-3 w-full">
          <button
            type="button"
            className="flex-1 py-3 px-4 text-sm font-semibold rounded-xl bg-slate-100 dark:bg-slate-800 text-slate-700 dark:text-slate-300 hover:bg-slate-200 dark:hover:bg-slate-700 transition-colors"
            onClick={onClose}
          >
            {cancelText}
          </button>
          <button
            type="button"
            className={`flex-1 py-3 px-4 text-sm font-semibold rounded-xl transition-all active:scale-[0.98] ${config.btn}`}
            onClick={() => {
              onConfirm()
              onClose()
            }}
          >
            {confirmText}
          </button>
        </div>
      </div>
    </div>
  )
}

