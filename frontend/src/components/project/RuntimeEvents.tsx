import React, { useState } from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Activity, ChevronDown, ChevronUp } from 'lucide-react'
import { cn } from '@/lib/utils'

interface RuntimeEventsProps {
  runtimeEvents: any[];
  t: (key: string, params?: any) => string;
}

export const RuntimeEvents: React.FC<RuntimeEventsProps> = ({ runtimeEvents, t }) => {
  const [isExpanded, setIsExpanded] = useState(false);
  const DEFAULT_VISIBLE_COUNT = 5;

  const visibleEvents = isExpanded ? runtimeEvents : runtimeEvents.slice(0, DEFAULT_VISIBLE_COUNT);

  return (
    <div className="space-y-6">
      <h3 className="text-sm font-bold uppercase tracking-widest text-muted-foreground/80 flex items-center gap-2">
        <Activity className="w-4 h-4 text-primary" />
        {t('projectDetail.runtime.events') || 'System Lifecycle Events'}
      </h3>

      <Card className="bg-card/40 border-border/40 backdrop-blur-md">
        <CardContent className="p-4 space-y-4">
          {runtimeEvents.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-12 text-center">
              <Activity className="w-8 h-8 text-muted-foreground/30 mb-2" />
              <p className="text-xs text-muted-foreground font-medium">{t('projectDetail.runtime.noEvents') || 'No events recorded'}</p>
            </div>
          ) : (
            <>
              <div className="relative pl-4 border-l border-border/50 space-y-5 py-1 transition-all duration-300">
                {visibleEvents.map((evt, idx) => {
                  const isError = ['oom_killed', 'crashed', 'deployment_failed'].includes(evt.event_type)
                  const isWarning = ['auto_healing_restart', 'scale_to_zero', 'sleeping'].includes(evt.event_type)
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
                            {evt.event_type.replace(/_/g, ' ')}
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

              {runtimeEvents.length > DEFAULT_VISIBLE_COUNT && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="w-full text-xs text-muted-foreground hover:text-foreground mt-2 border border-dashed border-border/40 hover:bg-muted/50 rounded-lg py-2 flex items-center justify-center gap-1.5 transition-all duration-300 active:scale-[0.98]"
                  onClick={() => setIsExpanded(!isExpanded)}
                >
                  {isExpanded ? (
                    <>
                      {t('common.showLess') || 'Show Less'} <ChevronUp size={14} />
                    </>
                  ) : (
                    <>
                      {t('common.showMore') || 'Show All Events'} ({runtimeEvents.length}) <ChevronDown size={14} />
                    </>
                  )}
                </Button>
              )}
            </>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
