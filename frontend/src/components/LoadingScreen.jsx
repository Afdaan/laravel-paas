import React from 'react';

export default function LoadingScreen() {
  return (
    <div className="min-h-screen flex flex-col items-center justify-center bg-slate-900 text-white relative overflow-hidden">
      {/* Background Ambience */}
      <div className="absolute inset-0 overflow-hidden pointer-events-none">
        <div className="absolute top-0 left-0 w-full h-full bg-gradient-to-br from-slate-900 via-slate-800 to-slate-900 z-0" />
        <div className="absolute top-[-10%] left-[-10%] w-[40%] h-[40%] bg-primary-600/10 rounded-full blur-[100px] animate-pulse" />
        <div className="absolute bottom-[-10%] right-[-10%] w-[40%] h-[40%] bg-purple-600/10 rounded-full blur-[100px] animate-pulse delay-700" />
      </div>

      <div className="z-10 flex flex-col items-center">
        {/* Animated Logo */}
        <div className="relative w-20 h-20 mb-8">
           <div className="absolute inset-0 bg-primary-500 rounded-2xl animate-ping opacity-20"></div>
           <div className="relative w-full h-full bg-primary-600 rounded-2xl flex items-center justify-center shadow-2xl shadow-primary-500/30">
             <span className="text-4xl animate-bounce">🚀</span>
           </div>
        </div>

        {/* Text */}
        <h1 className="text-3xl font-bold tracking-tight mb-2 bg-gradient-to-r from-white to-slate-400 bg-clip-text text-transparent">
          Laravel PaaS
        </h1>
        <p className="text-slate-500 text-sm font-medium tracking-widest uppercase">
          Initializing Environment
        </p>

        {/* Custom Progress Bar */}
        <div className="w-48 h-1 bg-slate-800 rounded-full mt-8 overflow-hidden relative">
          <div className="absolute inset-0 bg-gradient-to-r from-transparent via-primary-500 to-transparent w-[50%] animate-[shimmer_1.5s_infinite] translate-x-[-100%]" />
        </div>
        
        <style>{`
          @keyframes shimmer {
            100% { transform: translateX(200%); }
          }
        `}</style>
      </div>
    </div>
  );
}
