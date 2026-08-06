import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { AlertTriangle, CreditCard, ReceiptText, RefreshCw, WalletCards } from 'lucide-react'
import { toast } from 'sonner'
import { billingAPI } from '@/services/api'
import { hasLowCreditBalance, retainOnRequestFailure, topupIdempotencyKey } from '@/lib/billing-ui'
import { usePolling } from '@/lib/usePolling'
import useTranslation from '@/lib/useTranslation'
import type { BillingOverview, BillingStatus, TopupPackage } from '@/types'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

type RequestState<T> =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'success'; data: T }
  | { status: 'error'; error: string }

export default function Billing() {
  const { t, language } = useTranslation()
  const [overview, setOverview] = useState<RequestState<BillingOverview>>('idle')
  const [packages, setPackages] = useState<RequestState<TopupPackage[]>>('idle')
  const [statuses, setStatuses] = useState<RequestState<BillingStatus[]>>('idle')
  const [topupPackageID, setTopupPackageID] = useState<number | null>(null)
  const topupKeys = useRef(new Map<number, string>())
  const loadInFlight = useRef(false)

  const formatNumber = useCallback(
    (value: number) => new Intl.NumberFormat(language === 'id' ? 'id-ID' : 'en-US').format(value),
    [language],
  )
  const formatCredits = useCallback((credits: number) => formatNumber(credits), [formatNumber])
  const formatDate = useCallback(
    (value?: string) =>
      value
        ? new Intl.DateTimeFormat(language === 'id' ? 'id-ID' : 'en-US', { dateStyle: 'medium' }).format(
            new Date(value),
          )
        : '—',
    [language],
  )
  const formatMoney = useCallback(
    (amountMinor: number, currency: string) =>
      new Intl.NumberFormat(language === 'id' ? 'id-ID' : 'en-US', {
        style: 'currency',
        currency,
        maximumFractionDigits: 0,
      }).format(amountMinor),
    [language],
  )
  const formatStatus = useCallback(
    (status: string) => {
      const translated = t(`billing.statuses.${status}`)
      return translated === `billing.statuses.${status}` ? status.replaceAll('_', ' ') : translated
    },
    [t],
  )
  const statusVariant = useCallback(
    (status: string) =>
      status === 'paid' || status === 'active'
        ? 'secondary'
        : status === 'suspended' || status === 'payment_due'
          ? 'destructive'
          : 'outline',
    [],
  )
  const translateLedgerType = useCallback(
    (type: string) => {
      const key = `billing.ledgerTypes.${type}`
      const translated = t(key)
      return translated === key ? type.replaceAll('_', ' ') : translated
    },
    [t],
  )

  const load = useCallback(async () => {
    if (loadInFlight.current) return
    loadInFlight.current = true

    const markLoading = <T,>(setter: React.Dispatch<React.SetStateAction<RequestState<T>>>) => {
      setter((current) => (current.status === 'success' ? current : { status: 'loading' }))
    }

    markLoading(setOverview)
    markLoading(setPackages)
    markLoading(setStatuses)

    const [overviewResult, catalogResult, statusResult] = await Promise.allSettled([
      billingAPI.overview(),
      billingAPI.catalog(),
      billingAPI.status(),
    ])

    setOverview(
      retainOnRequestFailure<BillingOverview>(overviewResult, overview.status === 'success' ? overview.data : null),
    )
    setPackages(
      retainOnRequestFailure<TopupPackage[]>(
        catalogResult,
        packages.status === 'success' ? packages.data : null,
        (value) => value.packages,
      ),
    )
    setStatuses(
      retainOnRequestFailure<BillingStatus[]>(statusResult, statuses.status === 'success' ? statuses.data : null),
    )

    loadInFlight.current = false
  }, [overview, packages, statuses])

  useEffect(() => {
    setOverview({ status: 'loading' })
    setPackages({ status: 'loading' })
    setStatuses({ status: 'loading' })
    void load()
  }, [load])

  usePolling(() => void load(), 30_000)

  const attentionResources = useMemo(() => {
    if (statuses.status !== 'success') return []
    return statuses.data.filter(({ status }) => status === 'payment_due' || status === 'suspended')
  }, [statuses])

  const startTopup = async (packageID: number) => {
    setTopupPackageID(packageID)
    try {
      const idempotencyKey = topupIdempotencyKey(topupKeys.current, packageID, () => crypto.randomUUID())
      const response = await billingAPI.createTopup(packageID, idempotencyKey)
      if (!response.data.payment_url) throw new Error(t('billing.paymentSessionUnavailable'))
      window.location.assign(response.data.payment_url)
    } catch {
      toast.error(t('billing.topupStartFailed'))
    } finally {
      setTopupPackageID(null)
    }
  }

  const reconcileTopup = async (topupID: number) => {
    try {
      await billingAPI.reconcileTopup(topupID)
      await load()
      toast.success(t('billing.statusRefreshed'))
    } catch {
      toast.error(t('billing.statusRefreshFailed'))
    }
  }

  const balanceData = overview.status === 'success' ? overview.data.wallet.balance_credits : null
  const upcomingCredits = overview.status === 'success' ? overview.data.upcoming_required_credits : null
  const showLowBalance =
    balanceData !== null &&
    upcomingCredits !== null &&
    hasLowCreditBalance(balanceData, upcomingCredits) &&
    attentionResources.length === 0

  return (
    <div className="mx-auto max-w-6xl space-y-6 pb-10">
      <div className="flex flex-col gap-3 border-b pb-5 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <p className="text-sm font-medium text-primary">{t('billing.nav')}</p>
          <h1 className="text-2xl font-semibold tracking-tight">{t('billing.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('billing.description')}</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void load()} disabled={loadInFlight.current}>
          <RefreshCw className={loadInFlight.current ? 'animate-spin' : ''} /> {t('billing.refresh')}
        </Button>
      </div>

      {attentionResources.length > 0 && (
        <Card className="border-destructive/40 bg-destructive/5" role="alert" aria-live="assertive">
          <CardContent className="flex gap-3 pt-6 text-sm">
            <AlertTriangle className="mt-0.5 size-5 shrink-0 text-destructive" />
            <div>
              <strong>{t('billing.paymentRequired')}</strong>{' '}
              {attentionResources
                .map((resource) =>
                  resource.oldest_due_at
                    ? `${resource.resource_type} #${resource.resource_id}, ${t('billing.dueOn', { date: formatDate(resource.oldest_due_at) })} (${formatStatus(resource.status)})`
                    : `${resource.resource_type} #${resource.resource_id} (${formatStatus(resource.status)})`,
                )
                .join('; ')}{' '}
              {t('billing.paymentRequiredDescription')}
            </div>
          </CardContent>
        </Card>
      )}

      {showLowBalance && (
        <Card className="border-amber-500/40 bg-amber-500/5" role="status">
          <CardContent className="flex gap-3 pt-6 text-sm">
            <AlertTriangle className="mt-0.5 size-5 shrink-0 text-amber-600" />
            <div>
              <strong>{t('billing.lowBalance')}</strong> {t('billing.lowBalanceDescription')}
            </div>
          </CardContent>
        </Card>
      )}

      <div className="grid gap-4 md:grid-cols-[1.1fr_1.9fr]">
        <Card>
          <CardHeader>
            <CardDescription>{t('billing.balance')}</CardDescription>
            {overview.status === 'loading' && <Skeleton className="mt-2 h-10 w-40" />}
            {overview.status === 'error' && (
              <CardTitle className="text-lg text-destructive">{t('billing.unavailable')}</CardTitle>
            )}
            {overview.status === 'success' && (
              <CardTitle className="flex items-center gap-2 text-3xl">
                <WalletCards className="size-6 text-primary" />
                {formatCredits(overview.data.wallet.balance_credits)} {t('billing.credits')}
              </CardTitle>
            )}
          </CardHeader>
          {overview.status === 'success' && (
            <CardContent className="text-sm text-muted-foreground">{t('billing.balanceDescription')}</CardContent>
          )}
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t('billing.addCredits')}</CardTitle>
            <CardDescription>{t('billing.addCreditsDescription')}</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
            {packages.status === 'loading' &&
              Array.from({ length: 3 }).map((_, index) => <Skeleton key={index} className="h-28 rounded-lg" />)}
            {packages.status === 'error' && (
              <p className="col-span-full text-sm text-destructive">{t('billing.catalogLoadFailed')}</p>
            )}
            {packages.status === 'success' &&
              packages.data.map((pkg) => (
                <button
                  key={pkg.id}
                  type="button"
                  className="rounded-lg border p-4 text-left transition-colors hover:border-primary/50 hover:bg-muted/50 disabled:opacity-60"
                  disabled={topupPackageID !== null}
                  onClick={() => void startTopup(pkg.id)}
                >
                  <div className="font-semibold">
                    {formatCredits(pkg.credits)} {t('billing.credits')}
                  </div>
                  <div className="mt-1 text-sm text-muted-foreground">{formatMoney(pkg.amount_minor, pkg.currency)}</div>
                  <div className="mt-3 flex items-center gap-1 text-xs font-medium text-primary">
                    <CreditCard className="size-3.5" />
                    {topupPackageID === pkg.id ? t('billing.openingCheckout') : t('billing.choosePackage')}
                  </div>
                </button>
              ))}
            {packages.status === 'success' && packages.data.length === 0 && (
              <p className="col-span-full text-sm text-muted-foreground">{t('billing.noPackages')}</p>
            )}
          </CardContent>
        </Card>
      </div>

      <section className="grid gap-6 lg:grid-cols-2">
        <HistoryCard
          title={t('billing.invoices')}
          icon={<ReceiptText className="size-4" />}
          empty={overview.status === 'success' ? t('billing.noInvoices') : t('billing.invoicesUnavailable')}
          state={overview.status === 'loading' ? 'loading' : overview.status === 'error' ? 'error' : 'success'}
          rows={
            overview.status === 'success'
              ? overview.data.invoices.map((invoice) => ({
                  id: invoice.id,
                  title: `${formatDate(invoice.period_start)} – ${formatDate(invoice.period_end)}`,
                  detail: `${formatCredits(invoice.total_credits)} ${t('billing.credits')}`,
                  status: invoice.status,
                  date: invoice.paid_at
                    ? t('billing.paidOn', { date: formatDate(invoice.paid_at) })
                    : invoice.due_at
                      ? t('billing.dueOn', { date: formatDate(invoice.due_at) })
                      : formatDate(invoice.created_at),
                }))
              : []
          }
          formatStatus={formatStatus}
          statusVariant={statusVariant}
        />
        <HistoryCard
          title={t('billing.topups')}
          icon={<CreditCard className="size-4" />}
          empty={overview.status === 'success' ? t('billing.noTopups') : t('billing.topupsUnavailable')}
          state={overview.status === 'loading' ? 'loading' : overview.status === 'error' ? 'error' : 'success'}
          onReconcile={reconcileTopup}
          reconcileLabel={t('billing.checkStatus')}
          rows={
            overview.status === 'success'
              ? overview.data.topups.map((topup) => ({
                  id: topup.id,
                  title: `${formatCredits(topup.credits)} ${t('billing.credits')}`,
                  detail: formatMoney(topup.amount_minor, topup.currency),
                  status: topup.status,
                  date: topup.paid_at ? t('billing.paidOn', { date: formatDate(topup.paid_at) }) : formatDate(topup.created_at),
                }))
              : []
          }
          formatStatus={formatStatus}
          statusVariant={statusVariant}
        />
      </section>

      {overview.status !== 'idle' && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">{t('billing.walletActivity')}</CardTitle>
            <CardDescription>{t('billing.walletActivityDescription')}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {overview.status === 'loading' &&
              Array.from({ length: 3 }).map((_, index) => <Skeleton key={index} className="h-12 w-full" />)}
            {overview.status === 'error' && (
              <p className="text-sm text-destructive">{t('billing.walletActivityUnavailable')}</p>
            )}
            {overview.status === 'success' &&
              overview.data.wallet.ledger_entries.map((entry, index) => (
                <div
                  key={`${entry.created_at}-${index}`}
                  className="flex items-center justify-between gap-3 border-b pb-3 last:border-0"
                >
                  <div>
                    <p className="text-sm font-medium">{translateLedgerType(entry.type)}</p>
                    <p className="text-xs text-muted-foreground">
                      {formatDate(entry.created_at)} · {t('billing.balanceAfter', { balance: formatCredits(entry.balance_after) })}
                    </p>
                  </div>
                  <span
                    className={
                      entry.amount_credits >= 0
                        ? 'text-sm font-semibold text-emerald-600'
                        : 'text-sm font-semibold text-destructive'
                    }
                  >
                    {entry.amount_credits >= 0 ? '+' : ''}
                    {formatCredits(entry.amount_credits)}
                  </span>
                </div>
              ))}
            {overview.status === 'success' && overview.data.wallet.ledger_entries.length === 0 && (
              <p className="text-sm text-muted-foreground">{t('billing.noWalletActivity')}</p>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  )
}

function HistoryCard({
  title,
  icon,
  empty,
  state,
  rows,
  formatStatus,
  statusVariant,
  onReconcile,
  reconcileLabel,
}: {
  title: string
  icon: ReactNode
  empty: string
  state: 'loading' | 'error' | 'success'
  rows: Array<{ id: number; title: string; detail: string; status: string; date: string }>
  formatStatus: (status: string) => string
  statusVariant: (status: string) => 'secondary' | 'destructive' | 'outline'
  onReconcile?: (id: number) => Promise<void>
  reconcileLabel?: string
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          {icon} {title}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        {state === 'loading' && Array.from({ length: 3 }).map((_, index) => <Skeleton key={index} className="h-14 w-full" />)}
        {state === 'error' && <p className="text-sm text-destructive">{empty}</p>}
        {state === 'success' &&
          rows.map((row) => (
            <div key={row.id} className="flex items-start justify-between gap-3 border-b pb-3 last:border-0">
              <div className="min-w-0">
                <p className="text-sm font-medium">{row.title}</p>
                <p className="text-xs text-muted-foreground">{row.detail}</p>
              </div>
              <div className="flex shrink-0 flex-col items-end gap-1">
                <Badge variant={statusVariant(row.status)}>{formatStatus(row.status)}</Badge>
                <span className="text-xs text-muted-foreground">{row.date}</span>
                {onReconcile && row.status === 'pending' && reconcileLabel && (
                  <Button variant="ghost" size="sm" className="h-auto py-0 text-xs" onClick={() => void onReconcile(row.id)}>
                    {reconcileLabel}
                  </Button>
                )}
              </div>
            </div>
          ))}
        {state === 'success' && rows.length === 0 && <p className="text-sm text-muted-foreground">{empty}</p>}
      </CardContent>
    </Card>
  )
}
