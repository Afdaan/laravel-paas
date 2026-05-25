import { Box, GitBranch, Loader2 } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { Project, DeploymentEvent } from '@/types'
import { RuntimeEvents } from './RuntimeEvents'

// Status Indicator Component (Visual Indicator for container states)
function StatusIndicator({ status, label }: { status: string; label: string }) {
  const styles: Record<string, { color: string; pulse?: boolean }> = {
    running: { color: 'text-emerald-500 bg-emerald-500/10 border-emerald-500/20' },
    building: { color: 'text-blue-500 bg-blue-500/10 border-blue-500/20', pulse: true },
    queued: { color: 'text-purple-500 bg-purple-500/10 border-purple-500/20' },
    failed: { color: 'text-rose-500 bg-rose-500/10 border-rose-500/20' },
    pending: { color: 'text-amber-500 bg-amber-500/10 border-amber-500/20' },
    stopped: { color: 'text-slate-500 bg-slate-500/10 border-slate-500/20 dark:text-slate-400' },
    restarting: { color: 'text-indigo-500 bg-indigo-500/10 border-indigo-500/20', pulse: true },
  }

  const current = styles[status] || styles.pending

  return (
    <Badge variant="outline" className={cn("gap-2 py-1 px-3 border/60 font-semibold text-[10px]", current.color)}>
      <div className={cn("w-2 h-2 rounded-full bg-current", current.pulse && "animate-pulse")} />
      <span className="uppercase font-bold tracking-wider">{label}</span>
    </Badge>
  )
}

interface RuntimeTabProps {
  project: Project
  checkpoints: { sha: string; time: string; message: string }[]
  isRollingBack: boolean
  rollbackCommitSHA: string
  setRollbackCommitSHA: (sha: string) => void
  triggerRollbackConfirm: (sha: string) => void
  runtimeEvents: DeploymentEvent[]
  t: (key: string, options?: any) => string
}

export function RuntimeTab({
  project,
  checkpoints,
  isRollingBack,
  rollbackCommitSHA,
  setRollbackCommitSHA,
  triggerRollbackConfirm,
  runtimeEvents,
  t,
}: RuntimeTabProps) {
  return (
    <div className="space-y-6 animate-in fade-in duration-300">
      <div className="flex flex-col gap-1 border-b border-border/40 pb-4 mb-6">
        <h2 className="text-xl font-bold tracking-tight text-foreground/90">{t('projectDetail.runtime.title')}</h2>
        <p className="text-xs text-muted-foreground">{t('projectDetail.runtime.subtitle')}</p>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left Column: Container Resource Management and Rollbacks */}
        <div className="lg:col-span-2 space-y-6">
          <div className="flex items-center justify-between">
            <h3 className="text-sm font-bold uppercase tracking-widest text-muted-foreground/80 flex items-center gap-2">
              <Box className="w-4 h-4 text-primary" />
              {t('projectDetail.runtime.containers')}
            </h3>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {/* Web Service Container Role Card */}
            <Card className="bg-card/45 border-border/40 backdrop-blur-md relative overflow-hidden group hover:border-primary/30 transition-all duration-300">
              <CardHeader className="pb-3">
                <div className="flex items-start justify-between">
                  <div className="space-y-1">
                    <span className="text-[10px] font-black tracking-widest text-primary uppercase">web</span>
                    <CardTitle className="text-sm font-bold">{t('projectDetail.runtime.webRole')}</CardTitle>
                  </div>
                  <StatusIndicator status={project.status} label={t(`status.${project.status}`) || project.status} />
                </div>
              </CardHeader>
              <CardContent className="space-y-3.5 pt-0 text-xs">
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <span className="text-muted-foreground">{t('projectDetail.runtime.cpuLimit')}</span>
                  <span className="font-bold text-foreground/90">{project.cpu_limit ? `${project.cpu_limit} Cores` : '0.5 Cores'}</span>
                </div>
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <span className="text-muted-foreground">{t('projectDetail.runtime.memoryLimit')}</span>
                  <span className="font-bold text-foreground/90">{project.memory_limit ? project.memory_limit : '512 MB'}</span>
                </div>
                <div className="flex justify-between items-center border-b border-border/30 pb-2">
                  <span className="text-muted-foreground">{t('projectDetail.runtime.port')}</span>
                  <span className="font-bold text-foreground/90">{project.internal_port ? project.internal_port : 'Auto (8000)'}</span>
                </div>
                <div className="flex justify-between items-center">
                  <span className="text-muted-foreground">Container ID</span>
                  <code className="bg-muted px-1.5 py-0.5 rounded font-mono text-[10px] text-foreground/80">
                    {project.container_id ? project.container_id.substring(0, 12) : '-'}
                  </code>
                </div>
              </CardContent>
            </Card>

            {/* Background Worker Service Role Card (if enabled/configured) */}
            {(project.framework === 'Laravel' ? project.queue_enabled : !!project.worker_command) && (
              <Card className="bg-card/45 border-border/40 backdrop-blur-md relative overflow-hidden group hover:border-primary/30 transition-all duration-300">
                <CardHeader className="pb-3">
                  <div className="flex items-start justify-between">
                    <div className="space-y-1">
                      <span className="text-[10px] font-black tracking-widest text-indigo-400 uppercase">worker</span>
                      <CardTitle className="text-sm font-bold">{t('projectDetail.runtime.workerRole')}</CardTitle>
                    </div>
                    <StatusIndicator
                      status={project.worker_container_id ? 'running' : 'stopped'}
                      label={project.worker_container_id ? t('status.running') : t('status.stopped')}
                    />
                  </div>
                </CardHeader>
                <CardContent className="space-y-3.5 pt-0 text-xs">
                  <div className="flex justify-between items-center border-b border-border/30 pb-2">
                    <span className="text-muted-foreground">{t('projectDetail.runtime.cpuLimit')}</span>
                    <span className="font-bold text-foreground/90">{project.cpu_limit ? `${project.cpu_limit} Cores` : '0.5 Cores'}</span>
                  </div>
                  <div className="flex justify-between items-center border-b border-border/30 pb-2">
                    <span className="text-muted-foreground">{t('projectDetail.runtime.memoryLimit')}</span>
                    <span className="font-bold text-foreground/90">{project.memory_limit ? project.memory_limit : '512 MB'}</span>
                  </div>
                  <div className="flex justify-between items-center border-b border-border/30 pb-2">
                    <span className="text-muted-foreground">Command</span>
                    <span className="font-mono text-[10px] text-foreground/90 truncate max-w-[120px]" title={project.worker_command || 'queue:work'}>
                      {project.worker_command || 'queue:work'}
                    </span>
                  </div>
                  <div className="flex justify-between items-center">
                    <span className="text-muted-foreground">Container ID</span>
                    <code className="bg-muted px-1.5 py-0.5 rounded font-mono text-[10px] text-foreground/80">
                      {project.worker_container_id ? project.worker_container_id.substring(0, 12) : '-'}
                    </code>
                  </div>
                </CardContent>
              </Card>
            )}
          </div>

          {/* Rollback Checkpoints Card */}
          <Card className="bg-card/40 border-border/40 backdrop-blur-md">
            <CardHeader className="pb-3 border-b border-border/40">
              <div className="flex items-center gap-3">
                <div className="w-8 h-8 bg-indigo-500/10 rounded-lg flex items-center justify-center text-indigo-400">
                  <GitBranch className="w-4 h-4" />
                </div>
                <div>
                  <CardTitle className="text-sm font-bold">{t('projectDetail.runtime.rollbacks')}</CardTitle>
                  <CardDescription className="text-[10px]">
                    Select any successfully completed build checkpoint to instantly rollback the container hot-swap.
                  </CardDescription>
                </div>
              </div>
            </CardHeader>
            <CardContent className="p-0 text-xs">
              <div className="divide-y divide-border/30 max-h-[300px] overflow-y-auto">
                {checkpoints.length === 0 ? (
                  <div className="p-6 text-center text-muted-foreground">{t('projectDetail.runtime.noEvents')}</div>
                ) : (
                  checkpoints.map((cp, idx) => (
                    <div key={idx} className="p-4 flex items-center justify-between hover:bg-muted/10 transition-colors group/row">
                      <div className="space-y-1">
                        <div className="flex items-center gap-2">
                          <code className="font-mono font-bold text-foreground text-[10px] bg-muted px-1.5 py-0.5 rounded">
                            {cp.sha.substring(0, 7)}
                          </code>
                          {project.last_commit_hash === cp.sha && (
                            <Badge variant="outline" className="text-[8px] px-1.5 py-0 border-emerald-500/30 bg-emerald-500/10 text-emerald-500 font-bold uppercase tracking-wider">
                              Active Version
                            </Badge>
                          )}
                        </div>
                        <p className="text-foreground/80 font-medium max-w-[320px] truncate" title={cp.message}>
                          {cp.message}
                        </p>
                        <span className="text-[9px] text-muted-foreground/60">{cp.time}</span>
                      </div>

                      <Button
                        size="sm"
                        variant="outline"
                        disabled={project.last_commit_hash === cp.sha || isRollingBack}
                        onClick={() => triggerRollbackConfirm(cp.sha)}
                        className="h-8 rounded-full text-[10px] font-bold uppercase tracking-wider border-border/50 hover:bg-primary hover:text-white transition-all opacity-80 group-hover/row:opacity-100"
                      >
                        {t('projectDetail.runtime.rollbackBtn')}
                      </Button>
                    </div>
                  ))
                )}
              </div>

              {/* Custom Commit Rollback Input Block */}
              <div className="p-4 border-t border-border/40 flex gap-3 bg-muted/20">
                <Input
                  placeholder="Or enter any custom Commit SHA..."
                  value={rollbackCommitSHA}
                  onChange={(e) => setRollbackCommitSHA(e.target.value)}
                  className="h-9 text-xs rounded-lg"
                />
                <Button
                  size="sm"
                  disabled={rollbackCommitSHA.length < 7 || isRollingBack}
                  onClick={() => triggerRollbackConfirm(rollbackCommitSHA)}
                  className="h-9 px-4 rounded-lg font-bold text-xs uppercase tracking-wider bg-primary text-primary-foreground hover:bg-primary/95"
                >
                  {isRollingBack ? (
                    <Loader2 className="w-3.5 h-3.5 animate-spin" />
                  ) : (
                    t('projectDetail.runtime.rollbackBtn')
                  )}
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>

        {/* Right Column: Runtime Activity Event Stream */}
        <RuntimeEvents runtimeEvents={runtimeEvents} t={t} />
      </div>
    </div>
  )
}
