import React from 'react';

export default function LoadingScreen() {
  return (
    <div className="min-h-screen flex flex-col items-center justify-center bg-[#0a0a0c] text-white relative overflow-hidden font-sans">
      {/* Dynamic Ambient Background */}
      <div className="absolute inset-0 pointer-events-none">
        <div className="absolute top-1/4 left-1/4 w-[500px] h-[500px] bg-purple-600/10 rounded-full blur-[120px] animate-pulse" />
        <div className="absolute bottom-1/4 right-1/4 w-[500px] h-[500px] bg-indigo-600/10 rounded-full blur-[120px] animate-pulse-slow" />
      </div>

      <div className="relative z-10 flex flex-col items-center">
        {/* Sleek Logo Container */}
        <div className="relative mb-12 group">
          <div className="absolute -inset-4 bg-gradient-to-tr from-purple-600 to-indigo-600 rounded-3xl blur-2xl opacity-20 group-hover:opacity-40 transition-opacity animate-pulse-slow"></div>
          <div className="relative w-24 h-24 bg-[#111114] border border-white/10 rounded-3xl flex items-center justify-center shadow-2xl overflow-hidden">
            {/* Glossy Overlay */}
            <div className="absolute inset-0 bg-gradient-to-br from-white/5 to-transparent"></div>
            
            <span className="text-3xl font-black tracking-tighter bg-gradient-to-br from-white to-slate-400 bg-clip-text text-transparent">
              LP
            </span>
            
            {/* Internal Scanning Light */}
            <div className="absolute inset-0 w-full h-full bg-gradient-to-r from-transparent via-purple-500/10 to-transparent skew-x-12 translate-x-[-150%] animate-[scan_2s_infinite]"></div>
          </div>
        </div>

        {/* Brand & Status */}
        <div className="text-center space-y-3">
          <h1 className="text-2xl font-bold tracking-[0.2em] text-white uppercase opacity-90">
            Laravel PaaS
          </h1>
          <div className="flex items-center justify-center gap-3">
            <span className="w-1.5 h-1.5 rounded-full bg-purple-500 animate-ping"></span>
            <p className="text-slate-500 text-[10px] font-bold tracking-[0.3em] uppercase">
              Initializing Core Services
            </p>
          </div>
        </div>

        {/* Minimal Progress Bar */}
        <div className="w-40 h-[2px] bg-white/5 rounded-full mt-12 overflow-hidden relative">
          <div className="absolute inset-y-0 left-0 bg-gradient-to-r from-purple-600 to-indigo-600 w-full rounded-full origin-left animate-[progress_1.5s_ease-in-out_infinite]" />
        </div>
      </div>
      
      <style>{`
        @keyframes scan {
          100% { transform: translateX(150%) skewX(12deg); }
        }
        @keyframes progress {
          0% { transform: scaleX(0); opacity: 0; }
          40% { transform: scaleX(0.6); opacity: 1; }
          100% { transform: scaleX(1); opacity: 0; }
        }
        .animate-pulse-slow {
          animation: pulse 4s cubic-bezier(0.4, 0, 0.6, 1) infinite;
        }
        @keyframes pulse {
          0%, 100% { opacity: 0.1; transform: scale(1); }
          50% { opacity: 0.2; transform: scale(1.1); }
        }
      `}</style>
    </div>
  );
}
