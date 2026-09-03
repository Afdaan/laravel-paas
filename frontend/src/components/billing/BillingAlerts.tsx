import { memo } from 'react'
import { AlertTriangle, RefreshCw } from 'lucide-react'

import { Card, CardContent } from '@/components/ui/card'
import type { AttentionResource } from './types'
import { useBillingFormatters } from './useBillingFormatters'

type BillingAlertsProps = {
  staleWarning: boolean
  attentionResources: AttentionResource[]
  showLowBalance: boolean
}

export const BillingAlerts = memo(function BillingAlerts({ staleWarning, attentionResources, showLowBalance }: BillingAlertsProps) {
  const { t, formatDate, formatStatus, formatResourceDisplayName } = useBillingFormatters()

  return (
    <>
      {staleWarning && (
        <Card className="border-amber-500/40 bg-amber-500/10 shadow-xs" role="status" aria-live="polite">
          <CardContent className="flex items-start gap-3 p-4 text-sm text-amber-800 dark:text-amber-300">
            <RefreshCw className="mt-0.5 size-5 shrink-0 animate-spin text-amber-600 dark:text-amber-400" />
            <div>
              <strong>{t('billing.staleData')}</strong>
            </div>
          </CardContent>
        </Card>
      )}

      {attentionResources.length > 0 && (
        <Card className="border-destructive/40 bg-destructive/10 shadow-xs" role="alert" aria-live="assertive">
          <CardContent className="flex items-start gap-3 p-4 text-sm text-destructive dark:text-red-400">
            <AlertTriangle className="mt-0.5 size-5 shrink-0 text-destructive" />
            <div>
              <strong className="font-bold">{t('billing.paymentRequired')}</strong>{' '}
              {attentionResources
                .map((resource) =>
                  resource.oldest_due_at
                    ? `${formatResourceDisplayName(resource.resource_name, resource.resource_type)}, ${t('billing.dueOn', { date: formatDate(resource.oldest_due_at) })} (${formatStatus(resource.status)})`
                    : `${formatResourceDisplayName(resource.resource_name, resource.resource_type)} (${formatStatus(resource.status)})`,
                )
                .join('; ')}{' '}
              {t('billing.paymentRequiredDescription')}
            </div>
          </CardContent>
        </Card>
      )}

      {showLowBalance && (
        <Card className="border-amber-500/40 bg-gradient-to-r from-amber-500/10 via-amber-500/5 to-transparent shadow-xs" role="status">
          <CardContent className="flex items-start gap-3 p-4 text-sm text-amber-800 dark:text-amber-300">
            <AlertTriangle className="mt-0.5 size-5 shrink-0 text-amber-600 dark:text-amber-400" />
            <div>
              <strong className="font-bold">{t('billing.lowBalance')}</strong> {t('billing.lowBalanceDescription')}
            </div>
          </CardContent>
        </Card>
      )}
    </>
  )
})
