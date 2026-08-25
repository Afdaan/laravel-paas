import { CalendarClock, Database, FolderGit2 } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import type { BillingRequestState } from '@/lib/billing-ui'
import type { BillingOverview } from '@/types'
import type { PendingRenewChange } from './types'
import { useBillingFormatters } from './useBillingFormatters'

type ResourceBillingCardProps = {
  overview: BillingRequestState<BillingOverview>
  renewLoading: Record<string, boolean>
  setPendingRenewChange: (change: PendingRenewChange) => void
}

export function ResourceBillingCard({ overview, renewLoading, setPendingRenewChange }: ResourceBillingCardProps) {
  const { t, formatCredits, formatDate, formatStatus, statusVariant, formatResourceDisplayName } = useBillingFormatters()

  return (
    <Card className="border-border/60 shadow-sm">
        <CardHeader className="pb-4">
          <CardTitle className="flex items-center gap-2 text-xl font-bold">
            <CalendarClock className="size-5 text-primary" />
            {t('billing.resourceBilling')}
          </CardTitle>
          <CardDescription>{t('billing.resourceBillingDescription')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {overview.status === 'loading' && <Skeleton className="h-24 rounded-xl" />}
          {overview.status === 'error' && <p className="text-sm text-muted-foreground">{t('billing.unavailable')}</p>}
          {overview.status === 'success' && overview.data.resources.length === 0 && (
            <p className="text-sm text-muted-foreground">{t('billing.noBillableResources')}</p>
          )}
          {overview.status === 'success' &&
            overview.data.resources.map((resource) => (
              <div
                key={`${resource.resource_type}-${resource.resource_id}`}
                className="flex flex-col gap-4 rounded-xl border border-border/60 bg-muted/20 p-4 sm:flex-row sm:items-center sm:justify-between"
              >
                <div className="min-w-0 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge
                      variant="outline"
                      className="bg-background/80 font-mono text-[10px] uppercase tracking-wider text-muted-foreground"
                    >
                      {resource.resource_type === 'project' ? (
                        <>
                          <FolderGit2 className="mr-1 size-3 text-cyan-500" />
                          {t('billing.resourceTypes.project')}
                        </>
                      ) : (
                        <>
                          <Database className="mr-1 size-3 text-purple-500" />
                          {t('billing.resourceTypes.database')}
                        </>
                      )}
                    </Badge>
                    <p className="truncate font-semibold text-foreground">
                      {formatResourceDisplayName(resource.resource_name, resource.resource_type)}
                    </p>
                    <Badge variant={statusVariant(resource.status)}>{formatStatus(resource.status)}</Badge>
                  </div>
                  <p className="text-sm text-muted-foreground">
                    {resource.spec_name} · {formatCredits(resource.monthly_credits)} {t('billing.credits')} / {t('billing.month')}
                    {resource.resource_type === 'project' && resource.cpu_millicores && resource.memory_mb ? (
                      <span className="ml-1 text-xs text-muted-foreground/80">
                        ({resource.cpu_millicores}m CPU · {resource.memory_mb} MB RAM)
                      </span>
                    ) : null}
                    {resource.resource_type === 'database' && (resource.engine || resource.storage_gb) ? (
                      <span className="ml-1 text-xs text-muted-foreground/80">
                        ({[resource.engine?.toUpperCase(), resource.storage_gb ? `${resource.storage_gb} GB` : null]
                          .filter(Boolean)
                          .join(' · ')})
                      </span>
                    ) : null}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {t('billing.currentPeriod', {
                      start: formatDate(resource.current_period_start),
                      end: formatDate(resource.next_invoice_at),
                    })}
                  </p>
                </div>
                <div className="flex flex-col gap-2 sm:items-end">
                  <p className="text-sm font-medium text-foreground">
                    {resource.status === 'active'
                      ? t('billing.renewsOn', { date: formatDate(resource.next_invoice_at) })
                      : t('billing.renewalPaymentDue', { date: formatDate(resource.next_invoice_at) })}
                  </p>
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-muted-foreground">{t('billing.autoRenew')}</span>
                    <Switch
                      id={`auto-renew-${resource.resource_type}-${resource.resource_id}`}
                      checked={resource.auto_renew}
                      disabled={renewLoading[`${resource.resource_type}-${resource.resource_id}`]}
                      onCheckedChange={(checked) => {
                        setPendingRenewChange({
                          resource_id: resource.resource_id,
                          resource_type: resource.resource_type,
                          resource_name: formatResourceDisplayName(resource.resource_name, resource.resource_type),
                          target_auto_renew: checked,
                        })
                      }}
                    />
                  </div>
                </div>
              </div>
            ))}
        </CardContent>
    </Card>
  )
}
