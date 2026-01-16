import { Fragment, useRef, useState } from 'react'

export default function ConfirmationModal({ isOpen, onClose, onConfirm, title, message, type = 'danger', confirmText = 'Confirm', cancelText = 'Cancel' }) {
  if (!isOpen) return null

  const colors = {
    danger: {
      iconBg: 'bg-red-500/10',
      iconText: 'text-red-500', 
      button: 'btn-danger',
    },
    warning: {
      iconBg: 'bg-amber-500/10',
      iconText: 'text-amber-500',
      button: 'btn-primary', 
    },
    info: {
      iconBg: 'bg-blue-500/10',
      iconText: 'text-blue-500',
      button: 'btn-primary',
    }
  }

  const style = colors[type] || colors.danger

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 sm:p-0">
      {/* Backdrop */}
      <div 
        className="fixed inset-0 bg-black/80 backdrop-blur-sm transition-opacity" 
        onClick={onClose}
      />

      {/* Modal Panel */}
      <div className="relative transform overflow-hidden rounded-xl bg-slate-900 border border-slate-800 text-left shadow-2xl transition-all sm:my-8 sm:w-full sm:max-w-lg animate-scale-in">
        <div className="px-4 pb-4 pt-5 sm:p-6 sm:pb-4">
          <div className="sm:flex sm:items-start">
            <div className={`mx-auto flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-full ${style.iconBg} sm:mx-0 sm:h-10 sm:w-10`}>
              {type === 'danger' && (
                <svg className={`h-6 w-6 ${style.iconText}`} fill="none" viewBox="0 0 24 24" strokeWidth="1.5" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z" />
                </svg>
              )}
              {type === 'warning' && (
                <svg className={`h-6 w-6 ${style.iconText}`} fill="none" viewBox="0 0 24 24" strokeWidth="1.5" stroke="currentColor">
                   <path strokeLinecap="round" strokeLinejoin="round" d="M12 9v3.75m9-.75a9 9 0 11-18 0 9 9 0 0118 0zm-9 3.75h.008v.008H12v-.008z" />
                </svg>
              )}
              {type === 'info' && (
                <svg className={`h-6 w-6 ${style.iconText}`} fill="none" viewBox="0 0 24 24" strokeWidth="1.5" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M11.25 11.25l.041-.02a.75.75 0 011.063.852l-.708 2.836a.75.75 0 001.063.853l.041-.021M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9-3.75h.008v.008H12V8.25z" />
                </svg>
              )}
            </div>
            <div className="mt-3 text-center sm:ml-4 sm:mt-0 sm:text-left">
              <h3 className="text-lg font-semibold leading-6 text-white text-shadow-glow">
                {title}
              </h3>
              <div className="mt-2">
                <p className="text-sm text-slate-400">
                  {message}
                </p>
              </div>
            </div>
          </div>
        </div>
        <div className="bg-slate-900/50 px-4 py-3 sm:flex sm:flex-row-reverse sm:px-6 border-t border-slate-800">
          <button
            type="button"
            className={`btn ${style.button} w-full sm:w-auto sm:ml-3 shadow-lg`}
            onClick={() => {
              onConfirm()
              onClose()
            }}
          >
            {confirmText}
          </button>
          <button
            type="button"
            className="mt-3 sm:mt-0 btn btn-secondary w-full sm:w-auto"
            onClick={onClose}
          >
            {cancelText}
          </button>
        </div>
      </div>
    </div>
  )
}
