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
  positive: 'border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 [&>i]:bg-emerald-500',
  attention: 'border-amber-500/25 bg-amber-500/10 text-amber-600 dark:text-amber-400 [&>i]:bg-amber-500',
  negative: 'border-destructive/25 bg-destructive/10 text-destructive [&>i]:bg-destructive',
  neutral: 'border-border bg-muted/50 text-muted-foreground [&>i]:bg-muted-foreground',
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

export function StatusBadge({
  status,
  className,
  dot = true,
}: {
  status: string
  className?: string
  dot?: boolean
}) {
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
        'inline-flex h-[22px] w-fit shrink-0 items-center gap-1.5 rounded-md border px-2',
        'font-mono text-[10px] font-semibold uppercase tracking-wider whitespace-nowrap',
        TONES[STATUS_TONES[status] ?? 'neutral'],
        className,
      )}
    >
      {dot && <i className="size-1.5 shrink-0 rounded-full" />}
      {label}
    </span>
  )
}
