import React from 'react';
import { Loader2 } from 'lucide-react';

export default function LoadingScreen() {
  return (
    <div className="min-h-screen flex flex-col items-center justify-center bg-background text-foreground font-sans antialiased">
      <div className="relative z-10 flex flex-col items-center max-w-[280px] w-full pt-20">
        {/* Progress System */}
        <div className="w-full flex flex-col items-center justify-center space-y-6">
          <Loader2 className="h-8 w-8 animate-spin text-primary" />
          <p className="text-center text-sm text-muted-foreground font-medium uppercase tracking-widest mt-4">
            Synchronizing cluster assets...
          </p>
        </div>
      </div>
    </div>
  );
}
