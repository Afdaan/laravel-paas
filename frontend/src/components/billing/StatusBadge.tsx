import { cn } from '@/lib/utils'
import useTranslation from '@/lib/useTranslation'

/**
 * Single source of truth for how a billing status looks.
 *
 * Admin and user pages each had their own `statusVariant` mapping onto Badge
 * variants, plus per-call-site emerald overrides — so `paid` was grey in one
 * table and green in another. One tone map, one component, everywhere.
 */
type Tone = 'positive' | 'attention' | 'negative' | 'neutral'

const TONES: Record<Tone, string> = {
  positive: 'border-emerald-600/20 bg-emerald-600/8 text-emerald-700 dark:border-emerald-400/20 dark:bg-emerald-400/8 dark:text-emerald-300',
  attention: 'border-amber-600/20 bg-amber-600/8 text-amber-700 dark:border-amber-400/20 dark:bg-amber-400/8 dark:text-amber-300',
  negative: 'border-destructive/20 bg-destructive/8 text-destructive',
  neutral: 'border-border bg-muted/40 text-muted-foreground',
}

const STATUS_TONES: Record<string, Tone> = {
  paid: 'positive',
  active: 'positive',
  pending: 'attention',
  payment_due: 'negative',
  suspended: 'negative',
  failed: 'negative',
  chargeback: 'negative',
  partial_chargeback: 'negative',
  void: 'neutral',
  expired: 'neutral',
  refunded: 'neutral',
  partial_refund: 'neutral',
  inactive: 'neutral',
}

export function StatusBadge({ status, className }: { status: string; className?: string }) {
  const { t } = useTranslation()
  // Statuses live under two translation namespaces; fall back to the raw key.
  const invoice = t(`billing.statuses.${status}`)
  const topup = t(`billing.topupStatuses.${status}`)
  const label =
    invoice !== `billing.statuses.${status}`
      ? invoice
      : topup !== `billing.topupStatuses.${status}`
        ? topup
        : status.replace(/_/g, ' ')

  return (
    <span
      className={cn(
        'inline-flex h-[22px] w-fit shrink-0 items-center rounded border px-2',
        'text-xs font-medium capitalize whitespace-nowrap',
        TONES[STATUS_TONES[status] ?? 'neutral'],
        className,
      )}
    >
      {label}
    </span>
  )
}
