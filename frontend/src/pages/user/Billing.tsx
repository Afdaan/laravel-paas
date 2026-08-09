import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import {
  AlertTriangle,
  ArrowUpRight,
  CalendarClock,
  Coins,
  CreditCard,
  Database,
  FolderGit2,
  PlusCircle,
  ReceiptText,
  RefreshCw,
  Sparkles,
  WalletCards,
} from 'lucide-react'
import { motion } from 'framer-motion'
import { toast } from 'sonner'
import { billingAPI } from '@/services/api'
import axios from 'axios'
import {
  createTopupIdempotencyKey,
  hasLowCreditBalance,
  nextBillingRequestState,
  toMajorUnits,
  type BillingRequestState,
} from '@/lib/billing-ui'
import { usePolling } from '@/lib/usePolling'
import useTranslation from '@/lib/useTranslation'
import type { BillingOverview, BillingStatus, TopupPackage } from '@/types'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

export default function Billing() {
  const { t, language } = useTranslation()
  const [overview, setOverview] = useState<BillingRequestState<BillingOverview>>({ status: 'idle' })
  const [packages, setPackages] = useState<BillingRequestState<TopupPackage[]>>({ status: 'idle' })
  const [statuses, setStatuses] = useState<BillingRequestState<BillingStatus[]>>({ status: 'idle' })
  const [renewLoading, setRenewLoading] = useState<Record<string, boolean>>({})
  const [pendingRenewChange, setPendingRenewChange] = useState<{
    resource_id: number
    resource_type: 'project' | 'database'
    resource_name: string
    target_auto_renew: boolean
  } | null>(null)
  const [topupPackageID, setTopupPackageID] = useState<number | null>(null)
  const [customAmount, setCustomAmount] = useState('')
  const [staleWarning, setStaleWarning] = useState(false)
  const topupKeys = useRef(new Map<number, string>())
  const loadInFlight = useRef(false)
  const overviewRef = useRef(overview)
  const packagesRef = useRef(packages)
  const statusesRef = useRef(statuses)

  useEffect(() => {
    overviewRef.current = overview
    packagesRef.current = packages
    statusesRef.current = statuses
  })

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
      }).format(toMajorUnits(amountMinor, currency)),
    [language],
  )
  const formatStatus = useCallback(
    (status: string) => {
      const translated = t(`billing.statuses.${status}`)
      if (translated !== `billing.statuses.${status}`) return translated
      const topup = t(`billing.topupStatuses.${status}`)
      if (topup !== `billing.topupStatuses.${status}`) return topup
      return status.replace(/_/g, ' ')
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
      return translated === key ? type.replace(/_/g, ' ') : translated
    },
    [t],
  )
  const translateResourceType = useCallback(
    (type: string) => {
      const key = `billing.resourceTypes.${type}`
      const translated = t(key)
      return translated === key ? type : translated
    },
    [t],
  )

  const didLoadInitial = useRef(false)

  const load = useCallback(async () => {
    if (loadInFlight.current) return
    loadInFlight.current = true

    const markLoading = <T,>(setter: React.Dispatch<React.SetStateAction<BillingRequestState<T>>>) => {
      setter((current) => (current.status === 'success' ? current : { status: 'loading' }))
    }

    markLoading(setOverview)
    markLoading(setPackages)
    markLoading(setStatuses)

    const currentOverview = overview
    const currentPackages = packages
    const currentStatuses = statuses

    const [overviewResult, catalogResult, statusResult] = await Promise.allSettled([
      billingAPI.overview(),
      billingAPI.catalog(),
      billingAPI.status(),
    ])

    const nextOverview = nextBillingRequestState(overviewResult, currentOverview, (response) => response.data)
    const nextPackages = nextBillingRequestState(catalogResult, currentPackages, (response) => response.data.packages)
    const nextStatuses = nextBillingRequestState(statusResult, currentStatuses, (response) => response.data)
    setOverview(nextOverview)
    setPackages(nextPackages)
    setStatuses(nextStatuses)
    const hasStaleData =
      (overviewResult.status === 'rejected' && nextOverview.status === 'success') ||
      (catalogResult.status === 'rejected' && nextPackages.status === 'success') ||
      (statusResult.status === 'rejected' && nextStatuses.status === 'success')
    setStaleWarning(hasStaleData)

    loadInFlight.current = false
  }, [overview, packages, statuses])

  useEffect(() => {
    if (didLoadInitial.current) return
    didLoadInitial.current = true
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
      const idempotencyKey = topupKeys.current.get(packageID) ?? createTopupIdempotencyKey()
      topupKeys.current.set(packageID, idempotencyKey)
      const response = await billingAPI.createTopup(packageID, idempotencyKey)
      if (!response.data.payment_url) {
        topupKeys.current.delete(packageID)
        throw new Error(t('billing.paymentSessionUnavailable'))
      }
      topupKeys.current.delete(packageID)
      window.location.assign(response.data.payment_url)
    } catch (error) {
      if (axios.isAxiosError(error) && error.response) {
        topupKeys.current.delete(packageID)
      }
      toast.error(t('billing.topupStartFailed'))
    } finally {
      setTopupPackageID(null)
    }
  }

  const customAmountNum = parseInt(customAmount) || 0
  const customCredits = Math.floor(customAmountNum / 1000)
  const customValid = customAmountNum >= 10_000 && customAmountNum <= 10_000_000 && customAmountNum % 1000 === 0

  const startCustomTopup = async () => {
    setTopupPackageID(-1)
    try {
      const idempotencyKey = createTopupIdempotencyKey()
      const response = await billingAPI.createTopup(0, idempotencyKey, customAmountNum)
      if (!response.data.payment_url) {
        throw new Error(t('billing.paymentSessionUnavailable'))
      }
      setCustomAmount('')
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
    <div className="mx-auto max-w-6xl space-y-8 pb-12 animate-in fade-in duration-500">
      {/* Header section with gradient accent */}
      <div className="flex flex-col gap-4 border-b pb-6 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <div className="flex items-center gap-2">
            <span className="inline-flex items-center gap-1 rounded-full bg-primary/10 px-2.5 py-0.5 text-xs font-semibold text-primary">
              <Coins className="size-3.5" />
              {t('billing.nav')}
            </span>
          </div>
          <h1 className="mt-1 text-3xl font-extrabold tracking-tight sm:text-4xl">{t('billing.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('billing.description')}</p>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => void load()}
          disabled={loadInFlight.current}
          className="self-start shadow-sm transition-all hover:bg-muted sm:self-auto"
        >
          <RefreshCw className={`mr-2 size-3.5 ${loadInFlight.current ? 'animate-spin' : ''}`} />
          {t('billing.refresh')}
        </Button>
      </div>

      {/* Warnings & Alerts */}
      {staleWarning && (
        <Card className="border-amber-500/40 bg-amber-500/10 shadow-sm" role="status" aria-live="polite">
          <CardContent className="flex items-start gap-3 p-4 text-sm text-amber-800 dark:text-amber-300">
            <RefreshCw className="mt-0.5 size-5 shrink-0 animate-spin text-amber-600 dark:text-amber-400" />
            <div>
              <strong>{t('billing.staleData')}</strong>
            </div>
          </CardContent>
        </Card>
      )}

      {attentionResources.length > 0 && (
        <Card className="border-destructive/40 bg-destructive/10 shadow-sm" role="alert" aria-live="assertive">
          <CardContent className="flex items-start gap-3 p-4 text-sm text-destructive dark:text-red-400">
            <AlertTriangle className="mt-0.5 size-5 shrink-0 text-destructive" />
            <div>
              <strong className="font-bold">{t('billing.paymentRequired')}</strong>{' '}
              {attentionResources
                .map((resource) =>
                  resource.oldest_due_at
                    ? `${translateResourceType(resource.resource_type)} #${resource.resource_id}, ${t('billing.dueOn', { date: formatDate(resource.oldest_due_at) })} (${formatStatus(resource.status)})`
                    : `${translateResourceType(resource.resource_type)} #${resource.resource_id} (${formatStatus(resource.status)})`,
                )
                .join('; ')}{' '}
              {t('billing.paymentRequiredDescription')}
            </div>
          </CardContent>
        </Card>
      )}

      {showLowBalance && (
        <Card className="border-amber-500/40 bg-gradient-to-r from-amber-500/10 via-amber-500/5 to-transparent shadow-sm" role="status">
          <CardContent className="flex items-start gap-3 p-4 text-sm text-amber-800 dark:text-amber-300">
            <AlertTriangle className="mt-0.5 size-5 shrink-0 text-amber-600 dark:text-amber-400" />
            <div>
              <strong className="font-bold">{t('billing.lowBalance')}</strong> {t('billing.lowBalanceDescription')}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Main Billing Overview Card */}
      <Card className="overflow-hidden border-border/60 shadow-md transition-all hover:shadow-lg">
        <CardHeader className="bg-muted/30 border-b border-border/40 pb-4">
          <div className="flex items-center justify-between">
            <div>
              <CardTitle className="text-xl font-bold flex items-center gap-2">
                <WalletCards className="size-5 text-primary" />
                {t('billing.creditsOverview')}
              </CardTitle>
              <CardDescription className="mt-1">{t('billing.balanceDescription')}</CardDescription>
            </div>
            <Badge variant="outline" className="hidden sm:inline-flex bg-background/50 border-primary/20 text-primary">
              <Sparkles className="mr-1 size-3" /> Auto-Renewable
            </Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-8 pt-6">
          {/* Stat Cards */}
          <div className="grid gap-5 sm:grid-cols-2">
            {overview.status === 'loading' &&
              Array.from({ length: 2 }).map((_, index) => <Skeleton key={index} className="h-28 rounded-xl" />)}
            {overview.status === 'error' && (
              <div className="col-span-full rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-center text-sm text-destructive">
                {t('billing.unavailable')}
              </div>
            )}
            {overview.status === 'success' && (
              <>
                {/* Balance Card */}
                <div className="relative overflow-hidden rounded-xl border border-primary/30 bg-gradient-to-br from-primary/10 via-primary/5 to-transparent p-5 shadow-sm transition-all hover:border-primary/50">
                  <div className="flex items-center justify-between">
                    <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t('billing.balance')}</p>
                    <div className="rounded-full bg-primary/10 p-2 text-primary">
                      <WalletCards className="size-4" />
                    </div>
                  </div>
                  <div className="mt-3 flex items-baseline gap-2">
                    <p 
                      className="font-mono text-3xl font-bold tracking-tight text-foreground tabular-nums"
                      title={`${formatNumber(overview.data.wallet.balance_credits)} Credits`}
                    >
                      {formatCredits(overview.data.wallet.balance_credits)}
                      <span className="ml-1.5 font-sans text-xs font-semibold uppercase tracking-widest text-muted-foreground">{t('billing.credits')}</span>
                    </p>

                  </div>
                </div>

                {/* Upcoming Charges Card */}
                <div className="relative overflow-hidden rounded-xl border border-border/60 bg-card p-5 shadow-sm transition-all hover:border-border">
                  <div className="flex items-center justify-between">
                    <p className="text-xs font-semibold uppercase tracking-wider text-muted-foreground">{t('billing.upcomingCharges')}</p>
                    <div className="rounded-full bg-primary/10 p-2 text-primary">
                      <CalendarClock className="size-4" />
                    </div>
                  </div>
                  <div className="mt-3 flex items-baseline gap-2">
                    <p 
                      className="font-mono text-3xl font-bold tracking-tight text-foreground tabular-nums"
                      title={`${formatNumber(overview.data.upcoming_required_credits)} Credits`}
                    >
                      {formatCredits(overview.data.upcoming_required_credits)}
                      <span className="ml-1.5 font-sans text-xs font-semibold uppercase tracking-widest text-muted-foreground">{t('billing.credits')}</span>
                    </p>

                  </div>
                </div>
              </>
            )}
          </div>

          {/* Add Credits Packages Section */}
          <div className="pt-2">
            <div className="mb-4 flex items-center justify-between">
              <div>
                <h3 className="text-base font-bold text-foreground flex items-center gap-2">
                  <PlusCircle className="size-4 text-primary" />
                  {t('billing.addCredits')}
                </h3>
                <p className="mt-1 max-w-2xl text-xs leading-relaxed text-muted-foreground">{t('billing.addCreditsDescription')}</p>
              </div>
            </div>

            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
              {packages.status === 'loading' &&
                Array.from({ length: 4 }).map((_, index) => <Skeleton key={index} className="h-36 rounded-xl" />)}
              {packages.status === 'error' && (
                <p className="col-span-full text-sm text-destructive">{t('billing.catalogLoadFailed')}</p>
              )}
              {packages.status === 'success' &&
                packages.data.map((pkg, idx) => {
                  const isPopular = idx === 1 || packages.data.length === 1
                  return (
                    <motion.button
                      key={pkg.id}
                      type="button"
                      initial={{ opacity: 0, y: 8 }}
                      animate={{ opacity: 1, y: 0 }}
                      transition={{ type: 'spring', stiffness: 120, damping: 20, delay: idx * 0.05 }}
                      className={`group relative flex flex-col rounded-xl border p-5 text-left transition-all duration-200 hover:-translate-y-0.5 hover:shadow-md active:scale-[0.98] disabled:opacity-60 ${
                        isPopular
                          ? 'border-primary/40 bg-primary/[0.03] shadow-sm hover:border-primary/60'
                          : 'border-border/60 bg-card hover:border-border'
                      }`}
                      disabled={topupPackageID !== null}
                      onClick={() => void startTopup(pkg.id)}
                    >
                      {isPopular && (
                        <span className="absolute -top-2.5 right-3 rounded-full bg-primary/10 px-2.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-primary ring-1 ring-primary/20">
                          {t('billing.bestValue')}
                        </span>
                      )}

                      <div>
                        <div className="font-mono text-2xl font-bold tracking-tight text-foreground tabular-nums">
                          {formatCredits(pkg.credits)}
                        </div>
                        <div className="mt-0.5 text-xs font-medium text-muted-foreground">{t('billing.credits')}</div>
                      </div>

                      <div className="mt-auto flex items-center justify-between border-t border-border/40 pt-3 mt-4">
                        <span className="text-sm font-semibold text-muted-foreground">{formatMoney(pkg.amount_minor, pkg.currency)}</span>
                        <span className="flex items-center gap-1 text-xs font-medium text-primary">
                          {topupPackageID === pkg.id ? t('billing.openingCheckout') : t('billing.choosePackage')}
                          <ArrowUpRight className="size-3 transition-transform duration-200 group-hover:translate-x-0.5 group-hover:-translate-y-0.5" />
                        </span>
                      </div>
                    </motion.button>
                  )
                })}
              {packages.status === 'success' && packages.data.length === 0 && (
                <p className="col-span-full rounded-xl border border-dashed p-6 text-center text-sm text-muted-foreground">
                  {t('billing.noPackages')}
                </p>
              )}
            </div>

            {/* Custom Amount */}
            <div className="mt-6 rounded-xl border border-border/60 bg-card p-5">
              <label htmlFor="custom-amount" className="mb-3 flex items-center gap-2 text-sm font-semibold text-foreground">
                <Coins className="size-4 text-muted-foreground" />
                {t('billing.customTopupLabel')}
              </label>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
                <div className="relative flex-1">
                  <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-xs font-medium text-muted-foreground">Rp</span>
                  <Input
                    id="custom-amount"
                    type="number"
                    min={10000}
                    max={10000000}
                    step={1000}
                    placeholder="50000"
                    className="pl-9 font-mono tabular-nums"
                    value={customAmount}
                    onChange={(e) => setCustomAmount(e.target.value)}
                    disabled={topupPackageID !== null}
                  />
                </div>
                {customAmountNum > 0 && customValid && (
                  <span className="shrink-0 font-mono text-sm font-semibold tabular-nums text-foreground">
                    = {customCredits.toLocaleString(language)} {t('billing.credits')}
                  </span>
                )}
                <Button
                  size="sm"
                  disabled={!customValid || topupPackageID !== null}
                  onClick={() => void startCustomTopup()}
                  className="shrink-0"
                >
                  {topupPackageID === -1 ? t('billing.openingCheckout') : t('billing.customTopupButton')}
                </Button>
              </div>
              {customAmount && !customValid && (
                <p className="mt-2 text-xs text-muted-foreground">{t('billing.customTopupHint')}</p>
              )}
            </div>
          </div>
        </CardContent>
      </Card>

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
                      {resource.resource_name || `${translateResourceType(resource.resource_type)} #${resource.resource_id}`}
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
                          resource_name:
                            resource.resource_name ||
                            `${translateResourceType(resource.resource_type)} #${resource.resource_id}`,
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

      {/* Billing & Wallet History Section */}

      {/* Auto-Renew Confirmation Modal */}
      <Dialog open={pendingRenewChange !== null} onOpenChange={(open) => !open && setPendingRenewChange(null)}>
        <DialogContent className="sm:max-w-[425px]">
          <DialogHeader>
            <DialogTitle>
              {pendingRenewChange?.target_auto_renew
                ? t('billing.enableAutoRenewTitle')
                : t('billing.disableAutoRenewTitle')}
            </DialogTitle>
            <DialogDescription className="pt-2 leading-relaxed">
              {pendingRenewChange?.target_auto_renew
                ? t('billing.enableAutoRenewDescription', { name: pendingRenewChange?.resource_name ?? '' })
                : t('billing.disableAutoRenewDescription', { name: pendingRenewChange?.resource_name ?? '' })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="mt-4 gap-2 sm:gap-0">
            <Button variant="outline" onClick={() => setPendingRenewChange(null)}>
              {t('billing.cancel')}
            </Button>
            <Button
              onClick={async () => {
                if (!pendingRenewChange) return
                const { resource_id, resource_type, target_auto_renew } = pendingRenewChange
                const key = `${resource_type}-${resource_id}`
                setPendingRenewChange(null)
                setRenewLoading((prev) => ({ ...prev, [key]: true }))
                try {
                  await billingAPI.updateAutoRenew(resource_id, resource_type, target_auto_renew)
                  await load()
                  toast.success(
                    target_auto_renew ? t('billing.autoRenewEnabled') : t('billing.autoRenewDisabled'),
                  )
                } catch (error) {
                  if (axios.isAxiosError(error)) {
                    const status = error.response?.status
                    const serverMessage = error.response?.data?.error || error.response?.data?.message
                    if (status === 429) {
                      toast.error(t('billing.autoRenewRateLimited'))
                    } else if (serverMessage) {
                      toast.error(serverMessage)
                    } else {
                      toast.error(t('billing.autoRenewFailed'))
                    }
                  } else {
                    toast.error(t('billing.autoRenewFailed'))
                  }
                } finally {
                  setRenewLoading((prev) => ({ ...prev, [key]: false }))
                }
              }}
            >
              {t('billing.confirmChange')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <section className="space-y-5">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-bold tracking-tight text-foreground">{t('billing.history')}</h2>
        </div>

        <div className="grid gap-6 lg:grid-cols-3">
          <HistoryCard
            title={t('billing.invoices')}
            icon={<ReceiptText className="size-4 text-primary" />}
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
            icon={<CreditCard className="size-4 text-primary" />}
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
          <HistoryCard
            title={t('billing.walletActivity')}
            icon={<WalletCards className="size-4 text-primary" />}
            empty={overview.status === 'success' ? t('billing.noWalletActivity') : t('billing.walletActivityUnavailable')}
            state={overview.status === 'loading' ? 'loading' : overview.status === 'error' ? 'error' : 'success'}
            rows={
              overview.status === 'success'
                ? overview.data.wallet.ledger_entries.map((entry, index) => ({
                    id: index,
                    title: translateLedgerType(entry.type),
                    detail: `${formatDate(entry.created_at)} · ${t('billing.balanceAfter', { balance: formatCredits(entry.balance_after) })}`,
                    status: entry.amount_credits >= 0 ? 'credit' : 'debit',
                    date: '',
                    amount: entry.amount_credits,
                  }))
                : []
            }
            formatStatus={formatStatus}
            statusVariant={statusVariant}
            amountColumn
            formatAmount={formatCredits}
          />
        </div>
      </section>
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
  amountColumn,
  formatAmount,
}: {
  title: string
  icon: ReactNode
  empty: string
  state: 'loading' | 'error' | 'success'
  rows: Array<{ id: number; title: string; detail: string; status: string; date: string; amount?: number }>
  formatStatus: (status: string) => string
  statusVariant: (status: string) => 'secondary' | 'destructive' | 'outline'
  onReconcile?: (id: number) => Promise<void>
  reconcileLabel?: string
  amountColumn?: boolean
  formatAmount?: (value: number) => string
}) {
  return (
    <Card className="border-border/60 shadow-sm transition-all hover:shadow-md">
      <CardHeader className="bg-muted/20 border-b border-border/40 pb-3">
        <CardTitle className="flex items-center gap-2 text-base font-bold text-foreground">
          {icon} {title}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3 pt-4">
        {state === 'loading' && Array.from({ length: 3 }).map((_, index) => <Skeleton key={index} className="h-14 w-full rounded-lg" />)}
        {state === 'error' && <p className="text-sm text-destructive">{empty}</p>}
        {state === 'success' &&
          rows.map((row, rowIndex) => (
            <motion.div
              key={row.id}
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ type: 'spring', stiffness: 140, damping: 22, delay: Math.min(rowIndex, 8) * 0.04 }}
              className="-mx-1.5 flex items-start justify-between gap-3 rounded-lg border-b border-border/30 p-1.5 pb-3 transition-colors last:border-0 hover:bg-muted/40"
            >
              <div className="min-w-0">
                <p className="text-sm font-semibold text-foreground">{row.title}</p>
                <p className="text-xs text-muted-foreground">{row.detail}</p>
              </div>
              <div className="flex shrink-0 flex-col items-end gap-1">
                {amountColumn && typeof row.amount === 'number' && formatAmount ? (
                  <span className={`font-mono text-sm font-bold tabular-nums ${row.amount >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-destructive'}`}>
                    {row.amount >= 0 ? '+' : ''}{formatAmount(row.amount)}
                  </span>
                ) : (
                  <Badge variant={statusVariant(row.status)} className="capitalize font-medium text-[11px] px-2 py-0.5">
                    {formatStatus(row.status)}
                  </Badge>
                )}
                <span className="text-[11px] text-muted-foreground">{row.date}</span>
                {onReconcile && row.status === 'pending' && reconcileLabel && (
                  <Button variant="ghost" size="sm" className="h-auto py-0.5 px-2 text-xs font-semibold text-primary hover:bg-primary/10" onClick={() => void onReconcile(row.id)}>
                    {reconcileLabel}
                  </Button>
                )}
              </div>
            </motion.div>
          ))}
        {state === 'success' && rows.length === 0 && <p className="text-sm text-muted-foreground py-2 text-center">{empty}</p>}
      </CardContent>
    </Card>
  )
}
