import { ExternalLink, LayoutGrid, Globe, Code, GitBranch, Settings, ArrowUpRight, ShieldAlert } from 'lucide-react'
import useTranslation from '@/lib/useTranslation'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Project } from '@/types'
import { FrameworkIcon } from '@/components/FrameworkIcon'

interface OverviewTabProps {
  project: Project
  projectUrl: string
  isLaravelProject: boolean
  activeCommit: { sha: string; shortSha: string; message: string } | null
  onTabChange: (tab: string) => void
}

export function OverviewTab({ project, projectUrl, isLaravelProject, activeCommit, onTabChange }: OverviewTabProps) {
  const { t } = useTranslation()

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        <Card className="lg:col-span-2">
          <CardHeader className="pb-4">
            <CardTitle className="text-sm font-bold uppercase tracking-widest flex items-center gap-2">
              <LayoutGrid className="w-4 h-4 text-primary" />
              {t('projectDetail.overview.connectionInfo')}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-3">
            {/* Production / Subdomain URL */}
            <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between p-4 bg-muted/50 rounded-xl border gap-4 group hover:border-primary/20 transition-colors">
              <div className="flex items-center gap-4">
                <div className="p-2.5 bg-emerald-500/10 rounded-lg text-emerald-600 border border-emerald-500/20">
                  <Globe className="w-5 h-5" />
                </div>
                <div>
                  <div className="font-bold text-sm">{t('projectDetail.overview.productionUrl')}</div>
                  <div className="text-[10px] text-muted-foreground font-medium uppercase tracking-wider">{t('projectDetail.overview.webAccess')} · SSL Enabled</div>
                </div>
              </div>
              <a
                href={projectUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="flex items-center gap-1.5 text-primary hover:underline text-sm font-mono truncate max-w-xs group/link"
              >
                {projectUrl}
                <ExternalLink className="w-3 h-3 shrink-0 opacity-0 group-hover/link:opacity-100 transition-opacity" />
              </a>
            </div>

            {/* Custom Domains */}
            {project.custom_domains && project.custom_domains.length > 0 && (
              <div className="space-y-2">
                {project.custom_domains
                  .filter((d) => ['active', 'ssl_active', 'dns_verified'].includes(d.status))
                  .map((d) => (
                    <div
                      key={d.id}
                      className="flex flex-col sm:flex-row items-start sm:items-center justify-between p-4 bg-primary/5 rounded-xl border border-primary/15 gap-4 group hover:border-primary/30 hover:bg-primary/10 transition-colors"
                    >
                      <div className="flex items-center gap-4">
                        <div className="p-2.5 bg-primary/10 rounded-lg text-primary border border-primary/20">
                          <Globe className="w-5 h-5" />
                        </div>
                        <div>
                          <div className="flex items-center gap-2">
                            <span className="font-bold text-sm">{d.domain}</span>
                            <Badge
                              variant="outline"
                              className="text-[9px] font-bold uppercase tracking-widest px-1.5 py-0 h-4 text-primary border-primary/30 bg-primary/10"
                            >
                              Custom Domain
                            </Badge>
                          </div>
                          <div className="text-[10px] text-muted-foreground font-medium uppercase tracking-wider mt-0.5">
                            {d.status === 'ssl_active' ? 'SSL Active' : 'Active'} · Verified
                          </div>
                        </div>
                      </div>
                      <a
                        href={`https://${d.domain}`}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="flex items-center gap-1.5 text-primary hover:underline text-sm font-mono truncate max-w-xs group/link"
                      >
                        https://{d.domain}
                        <ExternalLink className="w-3 h-3 shrink-0 opacity-0 group-hover/link:opacity-100 transition-opacity" />
                      </a>
                    </div>
                  ))}

                {/* Pending domains hint */}
                {project.custom_domains.filter((d) => !['active', 'ssl_active', 'dns_verified'].includes(d.status)).length > 0 && (
                  <Button
                    variant="outline"
                    onClick={() => onTabChange('domains')}
                    className="w-full flex items-center justify-between px-4 py-2.5 h-auto rounded-xl border-dashed border-amber-500/20 bg-amber-500/5 text-amber-500 hover:bg-amber-500/10 hover:border-amber-500/30 group"
                  >
                    <span className="text-[11px] font-semibold">
                      {project.custom_domains.filter((d) => !['active', 'ssl_active', 'dns_verified'].includes(d.status)).length} domain
                      {project.custom_domains.filter((d) => !['active', 'ssl_active', 'dns_verified'].includes(d.status)).length > 1 ? 's' : ''} pending verification
                    </span>
                    <ExternalLink className="w-3.5 h-3.5 opacity-60 group-hover:opacity-100 transition-opacity" />
                  </Button>
                )}
              </div>
            )}

            {/* Empty state: invite to add domains */}
            {(!project.custom_domains || project.custom_domains.length === 0) && (
              <Button
                variant="outline"
                onClick={() => onTabChange('domains')}
                className="w-full flex items-center justify-between px-4 py-2.5 h-auto rounded-xl border-dashed border-muted-foreground/15 bg-muted/20 text-muted-foreground hover:border-primary/20 hover:text-primary hover:bg-primary/5 group"
              >
                <span className="text-[11px] font-medium">Add a custom domain</span>
                <ExternalLink className="w-3.5 h-3.5 opacity-40 group-hover:opacity-80 transition-opacity" />
              </Button>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-4">
            <CardTitle className="text-sm font-bold uppercase tracking-widest flex items-center gap-2">
              <Code className="w-4 h-4 text-primary" />
              {t('projectDetail.overview.repository')}
            </CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="p-3 rounded-lg bg-muted border">
              <label className="text-[9px] font-bold text-muted-foreground uppercase tracking-widest mb-1 block">{t('projectDetail.overview.uri')}</label>
              <div className="text-xs font-mono truncate">{project.github_url || project.repository_url}</div>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <div className="p-3 rounded-lg bg-muted border flex items-center justify-between">
                <div>
                  <label className="text-[9px] font-bold text-muted-foreground uppercase tracking-widest mb-1 block">{t('projectDetail.overview.branch')}</label>
                  <div className="flex items-center gap-1.5 font-bold text-xs">
                    <GitBranch className="w-3 h-3 text-primary" />
                    {project.branch || 'main'}
                  </div>
                </div>
                <Button
                  variant="ghost"
                  size="icon"
                  className="w-7 h-7 hover:bg-muted-foreground/10 text-muted-foreground hover:text-primary transition-colors"
                  onClick={() => onTabChange('settings')}
                  title={t('projectDetail.actions.changeBranch') || 'Change Branch'}
                >
                  <Settings className="w-3.5 h-3.5" />
                </Button>
              </div>
              <div className="p-3 rounded-lg bg-muted border">
                <label className="text-[9px] font-bold text-muted-foreground uppercase tracking-widest mb-1 block">{t('projectDetail.settings.version')}</label>
                <div className="flex items-center gap-1.5 font-bold text-xs uppercase">
                  <FrameworkIcon framework={project.framework} variant="plain" className="w-3.5 h-3.5" />
                  {isLaravelProject 
                    ? (project.laravel_version || 'Laravel 10')
                    : (project.framework && project.framework !== 'Other' ? `${project.framework} ${project.language_version || ''}` : t('common.general'))}
                </div>
              </div>
            </div>
            {activeCommit && (
              <div className="p-3 rounded-lg bg-muted border">
                <div className="flex items-center justify-between mb-1.5">
                  <label className="text-[9px] font-bold text-muted-foreground uppercase tracking-widest block">
                    {t('projectDetail.runtime.activeCommit') || 'Active Commit'}
                  </label>
                  <Button
                    variant="link"
                    size="sm"
                    onClick={() => onTabChange('runtime')}
                    className="text-[10px] font-bold text-primary hover:text-primary/80 h-auto p-0 gap-0.5 group"
                    title={t('projectDetail.runtime.goToCheckpointsTooltip') || 'Go to Deployment Checkpoints'}
                  >
                    {t('projectDetail.runtime.goToCheckpoints') || 'Go to Checkpoints'}
                    <ArrowUpRight className="w-3 h-3 transition-transform group-hover:translate-x-0.5 group-hover:-translate-y-0.5" />
                  </Button>
                </div>
                <div className="flex items-start gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => onTabChange('runtime')}
                    className="group shrink-0 font-mono font-bold text-[10px] bg-primary/10 hover:bg-primary/20 text-primary h-auto px-1.5 py-0.5 border-primary/20 gap-1"
                    title={t('projectDetail.runtime.goToCheckpointsTooltip') || 'View in Deployment Checkpoints'}
                  >
                    {activeCommit.shortSha}
                    <ArrowUpRight className="w-3 h-3 text-primary/70 group-hover:text-primary transition-colors" />
                  </Button>
                  {activeCommit.message ? (
                    <span className="min-w-0 text-[11px] leading-5 text-foreground/85 font-medium break-words" title={activeCommit.message}>
                      {activeCommit.message}
                    </span>
                  ) : (
                    <span className="text-[10px] text-muted-foreground italic">
                      {t('projectDetail.runtime.noCommitMessage') || 'No commit message found'}
                    </span>
                  )}
                </div>
                <div className="mt-2 rounded-md border border-border/60 bg-background/60 px-2 py-1.5">
                  <div className="mb-0.5 text-[8px] font-bold uppercase tracking-widest text-muted-foreground">
                    {t('projectDetail.runtime.fullCommitSha') || 'Full Commit SHA'}
                  </div>
                  <code className="block break-all text-[10px] leading-4 text-foreground/80">
                    {activeCommit.sha}
                  </code>
                </div>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      {project.error_log && (
        <Card className="border-destructive/20 bg-destructive/5 overflow-hidden">
          <CardHeader className="bg-destructive/10 py-3">
            <CardTitle className="text-xs font-bold text-destructive flex items-center gap-2 uppercase tracking-widest">
              <ShieldAlert className="w-4 h-4" />
              {t('projectDetail.overview.deployError')}
            </CardTitle>
          </CardHeader>
          <CardContent className="pt-4">
            <pre className="text-[11px] text-destructive/90 overflow-auto max-h-48 whitespace-pre-wrap font-mono leading-relaxed bg-black/5 p-4 rounded-lg">{project.error_log}</pre>
          </CardContent>
        </Card>
      )}
    </div>
  )
}
