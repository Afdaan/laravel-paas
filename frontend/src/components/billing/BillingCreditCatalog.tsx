import type { ChangeEvent } from 'react'
import { ArrowUpRight, CalendarClock, Coins, PlusCircle, WalletCards } from 'lucide-react'
import { motion } from 'framer-motion'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import type { BillingRequestState } from '@/lib/billing-ui'
import type { BillingOverview, TopupPackage } from '@/types'
import { useBillingFormatters } from './useBillingFormatters'

type BillingCreditCatalogProps = {
  overview: BillingRequestState<BillingOverview>
  packages: BillingRequestState<TopupPackage[]>
  topupPackageID: number | null
  customAmount: string
  customAmountNum: number
  customCredits: number
  customValid: boolean
  handleInitiatePackageTopup: (topupPackage: TopupPackage) => void
  handleCustomAmountChange: (event: ChangeEvent<HTMLInputElement>) => void
  handleInitiateCustomTopup: () => void
}

export function BillingCreditCatalog({
  overview,
  packages,
  topupPackageID,
  customAmount,
  customAmountNum,
  customCredits,
  customValid,
  handleInitiatePackageTopup,
  handleCustomAmountChange,
  handleInitiateCustomTopup,
}: BillingCreditCatalogProps) {
  const { t, language, formatNumber, formatCredits, formatMoney } = useBillingFormatters()

  return (
    <Card className="overflow-hidden border-border/60 shadow-xs transition-all hover:border-border/80">
        <CardHeader className="bg-muted/20 border-b border-border/40 pb-4">
          <div>
            <CardTitle className="text-lg font-bold flex items-center gap-2 text-foreground">
              <WalletCards className="size-4.5 text-primary" />
              {t('billing.creditsOverview')}
            </CardTitle>
            <CardDescription className="mt-0.5 text-xs text-muted-foreground">{t('billing.balanceDescription')}</CardDescription>
          </div>
        </CardHeader>
        <CardContent className="space-y-6 pt-6">
          {/* Stat Cards (Double-Bezel Hardware Architecture) */}
          <div className="grid gap-4 sm:grid-cols-2">
            {overview.status === 'loading' &&
              Array.from({ length: 2 }).map((_, index) => <Skeleton key={index} className="h-28 rounded-xl" />)}
            {overview.status === 'error' && (
              <div className="col-span-full rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-center text-sm text-destructive">
                {t('billing.unavailable')}
              </div>
            )}
            {overview.status === 'success' && (
              <>
                {/* Balance Card - Double Bezel Shell */}
                <div className="group relative overflow-hidden rounded-2xl border border-primary/20 bg-primary/5 p-1.5 transition-all hover:border-primary/40">
                  <div className="rounded-xl border border-primary/10 bg-background/80 p-5 backdrop-blur-sm">
                    <div className="flex items-center justify-between">
                      <p className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">{t('billing.balance')}</p>
                      <div className="flex size-7 items-center justify-center rounded-lg bg-primary/10 text-primary">
                        <WalletCards className="size-3.5" />
                      </div>
                    </div>
                    <div className="mt-3 flex items-baseline gap-2">
                      <p 
                        className="text-3xl font-bold tracking-tight text-foreground tabular-nums"
                        title={`${formatNumber(overview.data.wallet.balance_credits)} Credits`}
                      >
                        {formatCredits(overview.data.wallet.balance_credits)}
                        <span className="ml-2 font-sans text-xs font-semibold uppercase tracking-widest text-muted-foreground">{t('billing.credits')}</span>
                      </p>
                    </div>
                  </div>
                </div>

                {/* Upcoming Charges Card - Double Bezel Shell */}
                <div className="group relative overflow-hidden rounded-2xl border border-border/50 bg-muted/20 p-1.5 transition-all hover:border-border">
                  <div className="rounded-xl border border-border/40 bg-card/90 p-5">
                    <div className="flex items-center justify-between">
                      <p className="text-[11px] font-bold uppercase tracking-wider text-muted-foreground">{t('billing.upcomingCharges')}</p>
                      <div className="flex size-7 items-center justify-center rounded-lg bg-blue-500/10 text-blue-500">
                        <CalendarClock className="size-3.5" />
                      </div>
                    </div>
                    <div className="mt-3 flex items-baseline gap-2">
                      <p 
                        className="text-3xl font-bold tracking-tight text-foreground tabular-nums"
                        title={`${formatNumber(overview.data.upcoming_required_credits)} Credits`}
                      >
                        {formatCredits(overview.data.upcoming_required_credits)}
                        <span className="ml-2 font-sans text-xs font-semibold uppercase tracking-widest text-muted-foreground">{t('billing.credits')}</span>
                      </p>
                    </div>
                  </div>
                </div>
              </>
            )}
          </div>

          {/* Add Credits Packages Section */}
          <div className="pt-2">
            <div className="mb-4 flex items-center justify-between">
              <div>
                <h3 className="text-sm font-bold text-foreground flex items-center gap-2">
                  <PlusCircle className="size-4 text-primary" />
                  {t('billing.addCredits')}
                </h3>
                <p className="mt-0.5 max-w-2xl text-xs leading-relaxed text-muted-foreground">{t('billing.addCreditsDescription')}</p>
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
                      className={`group relative flex flex-col rounded-xl border p-5 text-left transition-all duration-200 hover:-translate-y-0.5 hover:shadow-sm active:scale-[0.98] disabled:opacity-60 ${
                        isPopular
                          ? 'border-primary/40 bg-primary/[0.04] shadow-xs hover:border-primary/60'
                          : 'border-border/60 bg-card hover:border-border/80'
                      }`}
                      disabled={topupPackageID !== null}
                      onClick={() => handleInitiatePackageTopup(pkg)}
                    >
                      {isPopular && (
                        <span className="absolute -top-2.5 right-3 rounded-full bg-primary/15 px-2.5 py-0.5 text-[10px] font-bold uppercase tracking-wider text-primary ring-1 ring-primary/30">
                          {t('billing.bestValue')}
                        </span>
                      )}

                      <div>
                        <div className="text-2xl font-bold tracking-tight text-foreground tabular-nums">
                          {formatCredits(pkg.credits)}
                        </div>
                        <div className="mt-0.5 text-xs font-medium text-muted-foreground">{t('billing.credits')}</div>
                      </div>

                      <div className="mt-auto flex items-center justify-between border-t border-border/40 pt-3 mt-4">
                        <span className="text-xs font-bold text-foreground">{formatMoney(pkg.amount_minor, pkg.currency)}</span>
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

            {/* Custom Amount Box */}
            <div className="mt-5 rounded-xl border border-border/60 bg-muted/20 p-4">
              <label htmlFor="custom-amount" className="mb-2.5 flex items-center gap-2 text-xs font-semibold text-foreground">
                <Coins className="size-3.5 text-muted-foreground" />
                {t('billing.customTopupLabel')}
              </label>
              <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
                <div className="relative flex-1">
                  <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-xs font-medium text-muted-foreground">Rp</span>
                  <Input
                    id="custom-amount"
                    type="text"
                    inputMode="numeric"
                    placeholder="50.000"
                    className="pl-9 text-xs tabular-nums h-9"
                    value={customAmount}
                    onChange={handleCustomAmountChange}
                    disabled={topupPackageID !== null}
                  />
                </div>
                {customAmountNum > 0 && customValid && (
                  <span className="shrink-0 text-xs font-medium tabular-nums text-muted-foreground">
                    = {customCredits.toLocaleString(language)} {t('billing.credits')}
                  </span>
                )}
                <Button
                  size="sm"
                  disabled={!customValid || topupPackageID !== null}
                  onClick={handleInitiateCustomTopup}
                  className="shrink-0 h-9 px-4 text-xs font-medium"
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
  )
}
