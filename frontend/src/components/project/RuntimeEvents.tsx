import React from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Activity } from 'lucide-react'
import { cn } from '@/lib/utils'
import { DeploymentEvent } from '@/types'

interface RuntimeEventsProps {
  runtimeEvents: DeploymentEvent[];
  t: (key: string, params?: Record<string, string | number>) => string;
}

export const RuntimeEvents: React.FC<RuntimeEventsProps> = ({ runtimeEvents, t }) => {
  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-bold uppercase tracking-widest text-muted-foreground/80 flex items-center gap-2">
          <Activity className="w-4 h-4 text-primary" />
          {t('projectDetail.runtime.events') || 'System Lifecycle Events'}
        </h3>
        {runtimeEvents.length > 0 && (
          <Badge variant="outline" className="text-[10px] font-bold tabular-nums border-border/50 text-muted-foreground/80">
            {runtimeEvents.length}
          </Badge>
        )}
      </div>

      <Card className="bg-card/40 border-border/40 backdrop-blur-md">
        <CardContent className="p-4">
          {runtimeEvents.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12 text-center">
              <Activity className="w-8 h-8 text-muted-foreground/30 mb-2" />
              <p className="text-xs text-muted-foreground font-medium">{t('projectDetail.runtime.noEvents') || 'No events recorded'}</p>
            </div>
          ) : (
            // Fixed-height internal-scroll viewport — keeps the panel bounded regardless of
            // event count so the page never grows tall or jumps when many events stream in.
            <div className="relative">
              <div className="max-h-[calc(100vh-18rem)] overflow-y-auto custom-scrollbar pr-1">
                <div className="relative pl-4 border-l border-border/50 space-y-5 py-1">
                  {runtimeEvents.map((evt, idx) => {
                    const isError = ['oom_killed', 'crashed', 'deployment_failed'].includes(evt.event_type ?? '')
                    const isWarning = ['auto_healing_restart'].includes(evt.event_type ?? '')
                    const bulletColor = isError
                      ? 'bg-rose-500 shadow-[0_0_10px_rgba(244,63,94,0.4)]'
                      : isWarning
                        ? 'bg-amber-500 shadow-[0_0_10px_rgba(245,158,11,0.4)]'
                        : 'bg-emerald-500 shadow-[0_0_10px_rgba(16,185,129,0.4)]'

                    return (
                      <div key={idx} className="relative group/evt transition-all duration-200">
                        <div className={cn("absolute -left-[21.5px] top-1.5 w-2.5 h-2.5 rounded-full border-2 border-background z-10", bulletColor)} />

                        <div className="space-y-1">
                          <div className="flex items-center justify-between">
                            <span className="font-bold text-[10px] uppercase tracking-wider text-foreground/80">
                              {(evt.event_type ?? 'unknown').replace(/_/g, ' ')}
                            </span>
                            <span className="text-[8px] text-muted-foreground/60">
                              {new Date(evt.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}
                            </span>
                          </div>
                          <p className="text-[11px] text-muted-foreground leading-normal" title={evt.message}>
                            {evt.message}
                          </p>
                        </div>
                      </div>
                    )
                  })}
                </div>
              </div>

              {/* Bottom fade mask cues that more events lie below the fold */}
              <div className="pointer-events-none absolute bottom-0 inset-x-0 h-8 bg-gradient-to-t from-card to-transparent" />
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
