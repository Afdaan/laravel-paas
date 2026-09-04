import { memo, useMemo } from 'react'
import { CalendarClock, Database, FolderGit2, Loader2 } from 'lucide-react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Switch } from '@/components/ui/switch'
import type { BillingRequestState } from '@/lib/billing-ui'
import { usePagination } from '@/lib/pagination'
import { TablePagination } from '@/components/ui/table-pagination'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { BillingOverview, BillingStatus } from '@/types'
import type { PendingRenewChange } from './types'
import { useBillingFormatters } from './useBillingFormatters'
import { StatusBadge } from './StatusBadge'

type ResourceBillingCardProps = {
  overview: BillingRequestState<BillingOverview>
  /** Billing status array from /api/billing/status, used for oldest_due_at on non-active resources. */
  statuses: BillingRequestState<BillingStatus[]>
  renewLoading: Record<string, boolean>
  paymentLoading: Record<string, boolean>
  payDueResource: (resourceID: number, resourceType: 'project' | 'database') => Promise<void>
  setPendingRenewChange: (change: PendingRenewChange) => void
}

export const ResourceBillingCard = memo(function ResourceBillingCard({ overview, statuses, renewLoading, paymentLoading, payDueResource, setPendingRenewChange }: ResourceBillingCardProps) {
  const { t, formatCredits, formatDate, formatResourceDisplayName } = useBillingFormatters()

  const statusLookup = useMemo(() => {
    const lookup: Record<string, BillingStatus> = {}
    if (statuses.status === 'success') {
      for (const status of statuses.data) {
        lookup[`${status.resource_type}:${status.resource_id}`] = status
      }
    }
    return lookup
  }, [statuses])

  const resources = overview.status === 'success' ? overview.data.resources : []
  const resourcePaging = usePagination(resources.length)
  const displayedResources = resources.slice(resourcePaging.start, resourcePaging.end)

  return (
    <Card className="border-border/60 shadow-sm">
      <CardHeader className="pb-4">
        <CardTitle className="flex items-center gap-2 text-xl font-bold">
          <CalendarClock className="size-5 text-primary" />
          {t('billing.resourceBilling')}
        </CardTitle>
        <CardDescription>{t('billing.resourceBillingDescription')}</CardDescription>
      </CardHeader>
      <CardContent className="p-0">
        {overview.status === 'loading' && <Skeleton className="mx-4 mb-4 h-40 rounded-xl" />}
        {overview.status === 'error' && (
          <p className="px-4 pb-5 text-sm text-muted-foreground">{t('billing.unavailable')}</p>
        )}
        {overview.status === 'success' && resources.length === 0 && (
          <p className="px-4 pb-5 text-sm text-muted-foreground">{t('billing.noBillableResources')}</p>
        )}
        {overview.status === 'success' && resources.length > 0 && (
          <div className="overflow-hidden border-t border-border/40">
            <Table className="min-w-[980px]">
              <TableHeader className="bg-muted/20">
                <TableRow className="border-border/40 hover:bg-transparent">
                  <TableHead className="w-[24%] pl-4 text-[11px] text-muted-foreground/80">
                    {t('billing.resource')}
                  </TableHead>
                  <TableHead className="w-[20%] text-[11px] text-muted-foreground/80">
                    {t('billing.plan')}
                  </TableHead>
                  <TableHead className="w-[13%] text-[11px] text-muted-foreground/80">
                    {t('billing.status')}
                  </TableHead>
                  <TableHead className="w-[29%] text-[11px] text-muted-foreground/80">
                    {t('billing.servicePeriod')}
                  </TableHead>
                  <TableHead className="w-[14%] pr-4 text-right text-[11px] text-muted-foreground/80">
                    {t('common.actions')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {displayedResources.map((resource) => {
              const billingStatus = statusLookup[`${resource.resource_type}:${resource.resource_id}`]
              // Use billing status as authoritative effective status when present to prevent
              // contradictory displays during partial/stale snapshot fetches.
              const effectiveStatus = billingStatus?.status ?? resource.status
              const isNonActive = effectiveStatus === 'payment_due' || effectiveStatus === 'suspended'
              const paymentDuePeriod = resource.payment_due_period_start && resource.payment_due_period_end
                ? { start: resource.payment_due_period_start, end: resource.payment_due_period_end }
                : null
              const resourceKey = `${resource.resource_type}-${resource.resource_id}`
              const resourceName = formatResourceDisplayName(resource.resource_name, resource.resource_type)
              const periodLabel = isNonActive
                ? paymentDuePeriod
                  ? t('billing.unpaidPeriod', {
                      start: formatDate(paymentDuePeriod.start),
                      end: formatDate(paymentDuePeriod.end),
                    })
                  : null
                : t('billing.currentPeriod', {
                    start: formatDate(resource.current_period_start),
                    end: formatDate(resource.next_invoice_at),
                  })

              const dueDateLabel = (() => {
                if (!isNonActive) {
                  if (!resource.auto_renew) {
                    return t('billing.periodEndsOn', { date: formatDate(resource.next_invoice_at) })
                  }
                  return t('billing.renewsOn', { date: formatDate(resource.next_invoice_at) })
                }
                const oldestDueAt = billingStatus?.oldest_due_at
                if (oldestDueAt) {
                  return t('billing.renewalPaymentDue', { date: formatDate(oldestDueAt) })
                }
                return t('billing.paymentRequired')
              })()

              return (
                <TableRow
                  key={`${resource.resource_type}-${resource.resource_id}`}
                  className="border-border/40"
                >
                  <TableCell className="min-w-[220px] py-3 pl-4">
                    <div className="flex items-center gap-3">
                      <div className="flex size-8 shrink-0 items-center justify-center rounded-lg border border-border/50 bg-muted/40">
                        {resource.resource_type === 'project' ? (
                          <FolderGit2 className="size-4 text-cyan-500" />
                        ) : (
                          <Database className="size-4 text-purple-500" />
                        )}
                      </div>
                      <div className="min-w-0">
                        <p className="max-w-[180px] truncate text-xs font-semibold text-foreground" title={resourceName}>
                          {resourceName}
                        </p>
                      <Badge
                        variant="outline"
                          className="mt-1 h-5 bg-background/80 px-1.5 font-mono text-[9px] uppercase tracking-wider text-muted-foreground"
                      >
                          {t(`billing.resourceTypes.${resource.resource_type}`)}
                      </Badge>
                      </div>
                    </div>
                  </TableCell>
                  <TableCell className="min-w-[190px] py-3">
                    <p className="text-xs font-medium text-foreground">{resource.spec_name}</p>
                    <p className="mt-0.5 text-[11px] text-muted-foreground">
                      {formatCredits(resource.monthly_credits)} {t('billing.credits')} / {t('billing.month')}
                    </p>
                      {resource.resource_type === 'project' && resource.cpu_millicores && resource.memory_mb ? (
                      <p className="mt-0.5 text-[10px] text-muted-foreground/70">
                        {resource.cpu_millicores}m CPU · {resource.memory_mb} MB RAM
                      </p>
                      ) : null}
                      {resource.resource_type === 'database' && (resource.engine || resource.storage_gb) ? (
                      <p className="mt-0.5 text-[10px] text-muted-foreground/70">
                        {[resource.engine?.toUpperCase(), resource.storage_gb ? `${resource.storage_gb} GB` : null]
                            .filter(Boolean)
                          .join(' · ')}
                      </p>
                      ) : null}
                  </TableCell>
                  <TableCell className="py-3">
                    <StatusBadge status={effectiveStatus} />
                  </TableCell>
                  <TableCell className="min-w-[280px] py-3">
                    <p className="text-xs font-medium text-foreground">{dueDateLabel}</p>
                    {periodLabel && <p className="mt-1 text-[11px] text-muted-foreground">{periodLabel}</p>}
                  </TableCell>
                  <TableCell className="py-3 pr-4">
                    <div className="flex items-center justify-end gap-3">
                      {isNonActive && (
                        <Button
                          type="button"
                          size="sm"
                          className="h-8 w-fit px-3 text-xs font-medium"
                          disabled={paymentLoading[resourceKey]}
                          onClick={() => void payDueResource(resource.resource_id, resource.resource_type)}
                        >
                          {paymentLoading[resourceKey] && <Loader2 className="size-3.5 animate-spin" />}
                          {t('billing.payDueNow')}
                        </Button>
                      )}
                      <div className="flex items-center gap-1.5">
                        <span className="text-[10px] text-muted-foreground">{t('billing.autoRenew')}</span>
                        <Switch
                          id={`auto-renew-${resource.resource_type}-${resource.resource_id}`}
                          aria-label={`${t('billing.autoRenew')}: ${resourceName}`}
                          checked={resource.auto_renew}
                          disabled={renewLoading[resourceKey]}
                          onCheckedChange={(checked) => {
                            setPendingRenewChange({
                              resource_id: resource.resource_id,
                              resource_type: resource.resource_type,
                              resource_name: resourceName,
                              target_auto_renew: checked,
                            })
                          }}
                        />
                      </div>
                    </div>
                  </TableCell>
                </TableRow>
              )
                })}
              </TableBody>
            </Table>
            <TablePagination state={resourcePaging} />
          </div>
        )}
      </CardContent>
    </Card>
  )
})
