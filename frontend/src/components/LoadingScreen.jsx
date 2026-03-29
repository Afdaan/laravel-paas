import React from 'react';

export default function LoadingScreen() {
  return (
    <div className="min-h-screen flex flex-col items-center justify-center bg-[#09090b] text-[#f8fafc] font-sans antialiased">
      <div className="relative z-10 flex flex-col items-center max-w-[280px] w-full">
        {/* Minimalist Brand Identity */}
        <div className="mb-12">
          <div className="w-12 h-12 bg-white text-black rounded-lg flex items-center justify-center text-xl font-black tracking-tighter shadow-2xl">
            LP
          </div>
        </div>

        {/* Progress System */}
        <div className="w-full space-y-6">
          <div className="flex items-center justify-between px-1">
             <span className="text-[10px] font-bold uppercase tracking-[0.2em] text-slate-500">Establishing Uplink</span>
             <span className="text-[10px] font-mono text-indigo-400">82%</span>
          </div>
          
          <div className="h-[2px] w-full bg-white/5 rounded-full overflow-hidden">
             <div className="h-full bg-indigo-500 w-[82%] animate-[loading_2s_ease-in-out_infinite]"></div>
          </div>
          
          <p className="text-center text-[10px] text-slate-600 font-medium uppercase tracking-widest mt-4">
            Synchronizing cluster assets...
          </p>
        </div>
      </div>
      
      <style>{`
        @keyframes loading {
          0% { transform: translateX(-100%); }
          50% { transform: translateX(0); }
          100% { transform: translateX(100%); }
        }
      `}</style>
    </div>
  );
}
