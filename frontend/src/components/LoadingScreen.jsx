import React from 'react';

export default function LoadingScreen() {
  return (
    <div className="min-h-screen flex flex-col items-center justify-center bg-[#030305] text-white relative overflow-hidden font-sans">
      {/* Dynamic Ambient Background Elements */}
      <div className="absolute inset-0 pointer-events-none z-0">
        <div className="absolute top-[20%] left-[30%] w-[300px] h-[300px] rounded-full bg-indigo-600/30 blur-[100px] mix-blend-screen opacity-60 animate-pulse-slow"></div>
        <div className="absolute bottom-[20%] right-[30%] w-[250px] h-[250px] rounded-full bg-fuchsia-600/30 blur-[100px] mix-blend-screen opacity-60 animate-pulse-slow font-delay-200"></div>
      </div>

      <div className="relative z-10 flex flex-col items-center animate-pop-in">
        {/* Sleek Logo Container */}
        <div className="relative mb-10 group">
          <div className="absolute -inset-4 bg-gradient-to-tr from-indigo-500 to-fuchsia-600 rounded-3xl blur-2xl opacity-30 animate-pulse-slow"></div>
          <div className="relative w-24 h-24 bg-[#0a0a0c] border border-white/10 rounded-3xl flex items-center justify-center shadow-2xl overflow-hidden backdrop-blur-xl">
            {/* Glossy Overlay */}
            <div className="absolute inset-0 bg-gradient-to-br from-white/10 to-transparent"></div>
            
            <span className="text-4xl font-black tracking-tighter bg-gradient-to-br from-white to-slate-300 bg-clip-text text-transparent">
              LP
            </span>
          </div>
        </div>

        {/* Brand & Status */}
        <div className="text-center space-y-4">
          <h1 className="text-2xl font-bold tracking-tight text-white/90">
            Laravel PaaS
          </h1>
          <div className="flex items-center justify-center gap-3 bg-white/5 border border-white/5 px-4 py-2 rounded-full backdrop-blur-sm">
            <span className="w-2 h-2 rounded-full bg-indigo-500 animate-pulse"></span>
            <p className="text-slate-300 text-xs font-semibold tracking-wide">
              Loading your workspace...
            </p>
          </div>
        </div>
      </div>
      
      <style>{`
        @keyframes pop-in {
          0% { opacity: 0; transform: scale(0.95); }
          100% { opacity: 1; transform: scale(1); }
        }
        .animate-pop-in {
          animation: pop-in 0.5s cubic-bezier(0.16, 1, 0.3, 1) both;
        }
        .animate-pulse-slow {
          animation: pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite;
        }
        @keyframes pulse {
          0%, 100% { opacity: 0.3; transform: scale(1); }
          50% { opacity: 0.6; transform: scale(1.05); }
        }
      `}</style>
    </div>
  );
}
