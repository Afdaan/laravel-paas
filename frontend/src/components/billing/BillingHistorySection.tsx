import { useMemo, useState } from 'react'
import { ArrowDownRight, ArrowUpRight, Calendar, Check, Copy, CreditCard, ReceiptText, RefreshCw, Search, WalletCards } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { TablePagination } from '@/components/ui/table-pagination'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { BillingRequestState } from '@/lib/billing-ui'
import { usePagination } from '@/lib/pagination'
import type { BillingOverview } from '@/types'
import { getInvoiceNumber } from './utils'
import { useBillingFormatters } from './useBillingFormatters'
import { StatusBadge } from './StatusBadge'

type Invoice = BillingOverview['invoices'][number]

type BillingHistorySectionProps = {
  overview: BillingRequestState<BillingOverview>
  copiedInvoiceID: number | null
  handleCopyInvoiceNumber: (invoiceNumber: string, id: number) => void
  setSelectedInvoice: (invoice: Invoice) => void
  reconcileTopup: (topupID: number) => Promise<void>
  onPayPendingTopup?: (topup: BillingOverview['topups'][number]) => void
}

export function BillingHistorySection({
  overview,
  copiedInvoiceID,
  handleCopyInvoiceNumber,
  setSelectedInvoice,
  reconcileTopup,
  onPayPendingTopup,
}: BillingHistorySectionProps) {
  const { t, formatCredits, formatDate, formatMoney, formatStatus, translateLedgerType } = useBillingFormatters()
  const [invoiceSearch, setInvoiceSearch] = useState('')
  const [invoiceStatusFilter, setInvoiceStatusFilter] = useState<'all' | 'paid' | 'payment_due'>('all')
  const filteredInvoices = useMemo(() => {
    if (overview.status !== 'success') return []
    const search = invoiceSearch.toLowerCase().trim()
    return overview.data.invoices.filter((invoice) => {
      const matchesSearch =
        !search ||
        getInvoiceNumber(invoice).toLowerCase().includes(search) ||
        invoice.id.toString().includes(search)
      return matchesSearch && (invoiceStatusFilter === 'all' || invoice.status === invoiceStatusFilter)
    })
  }, [overview, invoiceSearch, invoiceStatusFilter])
  const invoicePage = usePagination(filteredInvoices.length)
  const topupPage = usePagination(overview.status === 'success' ? overview.data.topups.length : 0)
  const ledgerEntries = useMemo(
    () => (overview.status === 'success' ? overview.data.wallet.ledger_entries : []),
    [overview],
  )
  const ledgerPage = usePagination(ledgerEntries.length)
  const invoiceMetrics = useMemo(() => {
    if (overview.status !== 'success') return { totalCreditsInvoiced: 0 }
    return {
      totalCreditsInvoiced: overview.data.invoices.reduce((total, invoice) => total + (invoice.total_credits || 0), 0),
    }
  }, [overview])
  const ledgerRows = useMemo(
    () =>
      ledgerEntries.slice(ledgerPage.start, ledgerPage.end).map((entry, index) => {
        const isCredit = entry.amount_credits >= 0
        return (
          <TableRow key={index} className="h-11 border-b border-border/30 transition-colors last:border-0 hover:bg-muted/30">
            <TableCell className="pl-4 text-xs text-foreground">
              <div className="flex items-center gap-2.5">
                <div className={`size-6 rounded-full flex items-center justify-center shrink-0 ${
                  isCredit
                    ? 'bg-emerald-500/10 border border-emerald-500/20 text-emerald-500'
                    : 'bg-amber-500/10 border border-amber-500/20 text-amber-500'
                }`}>
                  {isCredit ? <ArrowUpRight className="size-3.5" /> : <ArrowDownRight className="size-3.5" />}
                </div>
                <span className="text-[11px] font-medium capitalize text-foreground/90">{translateLedgerType(entry.type)}</span>
              </div>
            </TableCell>
            <TableCell className="text-xs">
              <span className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-[10px] font-medium tabular-nums ${
                isCredit
                  ? 'text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 border border-emerald-500/20'
                  : 'text-amber-600 dark:text-amber-400 bg-amber-500/10 border border-amber-500/20'
              }`}>
                {isCredit ? `+${formatCredits(entry.amount_credits)}` : formatCredits(entry.amount_credits)} {t('billing.credits')}
              </span>
            </TableCell>
            <TableCell className="text-[11px] font-medium tabular-nums text-foreground/85">
              {formatCredits(entry.balance_after)} {t('billing.credits')}
            </TableCell>
            <TableCell className="pr-4 text-right text-[11px] tabular-nums text-muted-foreground">
              {formatDate(entry.created_at)}
            </TableCell>
          </TableRow>
        )
      }),
    [ledgerEntries, ledgerPage.start, ledgerPage.end, formatCredits, formatDate, translateLedgerType, t],
  )

  return (
      <Card className="border-border/60 shadow-xs">
        <CardHeader className="border-b border-border/40 pb-4">
          <CardTitle className="text-lg font-bold flex items-center gap-2 text-foreground">
            <ReceiptText className="size-4.5 text-primary" />
            {t('billing.history')}
          </CardTitle>
          <CardDescription className="text-xs text-muted-foreground">
            {t('billing.historyDescription')}
          </CardDescription>
        </CardHeader>

        <CardContent className="pt-6">
          <Tabs defaultValue="wallet_activity" className="space-y-4">
            <TabsList className="grid w-full grid-cols-3 max-w-md h-9 text-xs">
              <TabsTrigger value="wallet_activity" className="text-xs">
                <WalletCards className="size-3.5 mr-1.5" />
                {t('billing.walletActivity')}
              </TabsTrigger>
              <TabsTrigger value="invoices" className="text-xs">
                <ReceiptText className="size-3.5 mr-1.5" />
                {t('billing.invoices')}
              </TabsTrigger>
              <TabsTrigger value="topups" className="text-xs">
                <CreditCard className="size-3.5 mr-1.5" />
                {t('billing.topups')}
              </TabsTrigger>
            </TabsList>

            {/* Wallet Activity Tab */}
            <TabsContent value="wallet_activity" className="space-y-4">
              <div className="rounded-xl border border-border/60 overflow-hidden bg-card/50">
                <Table>
                  <TableHeader className="bg-muted/40">
                    <TableRow className="border-b border-border/40 hover:bg-transparent">
                      <TableHead className="w-[32%] pl-4 text-[11px] font-medium tracking-[0.01em] text-muted-foreground/80">{t('billing.transactionType')}</TableHead>
                      <TableHead className="w-[24%] text-[11px] font-medium tracking-[0.01em] text-muted-foreground/80">{t('billing.amount')}</TableHead>
                      <TableHead className="w-[24%] text-[11px] font-medium tracking-[0.01em] text-muted-foreground/80">{t('billing.balanceAfterHeader')}</TableHead>
                      <TableHead className="w-[20%] pr-4 text-right text-[11px] font-medium tracking-[0.01em] text-muted-foreground/80">{t('billing.date')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {overview.status === 'loading' && (
                      <TableRow>
                        <TableCell colSpan={4} className="text-center py-6 text-xs text-muted-foreground">{t('billing.loadingWalletHistory')}</TableCell>
                      </TableRow>
                    )}
                    {overview.status === 'success' && overview.data.wallet.ledger_entries.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={4} className="text-center py-8 text-xs text-muted-foreground">{t('billing.noWalletActivity')}</TableCell>
                      </TableRow>
                    )}
                    {ledgerRows}
                  </TableBody>
                </Table>
                <TablePagination state={ledgerPage} disabled={overview.status === 'loading'} />
              </div>
            </TabsContent>

            {/* Invoices Tab */}
            <TabsContent value="invoices" className="space-y-4">
              {/* Financial Quick Metrics */}
              {overview.status === 'success' && overview.data.invoices.length > 0 && (
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                  <div className="rounded-xl border border-border/60 bg-muted/20 p-3.5 flex flex-col justify-between">
                    <span className="text-xs font-medium text-muted-foreground">
                      {t('billing.totalInvoiced')}
                    </span>
                    <div className="mt-1.5 flex items-baseline gap-2">
                      <span className="text-xl font-bold tracking-tight text-foreground tabular-nums">
                        {formatCredits(invoiceMetrics.totalCreditsInvoiced)}
                      </span>
                      <span className="text-[10px] font-medium text-muted-foreground uppercase">{t('billing.credits')}</span>
                    </div>

                  </div>

                  <div className="rounded-xl border border-border/60 bg-muted/20 p-3.5 flex flex-col justify-between">
                    <span className="text-xs font-medium text-muted-foreground">
                      {t('billing.activeSubscriptions')}
                    </span>
                    <div className="mt-1.5 flex items-baseline gap-2">
                      <span className="text-xl font-bold tracking-tight text-foreground tabular-nums">
                        {overview.data.resources.length}
                      </span>
                      <span className="text-[10px] font-medium text-muted-foreground">{t('billing.activeServicesCount', { count: overview.data.resources.length })}</span>
                    </div>
                    <span className="text-[11px] text-muted-foreground mt-0.5">
                      Auto-renew: {overview.data.resources.filter((r) => r.auto_renew).length}
                    </span>
                  </div>

                  <div className="rounded-xl border border-border/60 bg-muted/20 p-3.5 flex flex-col justify-between">
                    <span className="text-xs font-medium text-muted-foreground">
                      {t('billing.upcomingCharges')}
                    </span>
                    <div className="mt-1.5 flex items-baseline gap-2">
                      <span className="text-xl font-bold tracking-tight text-foreground tabular-nums">
                        {formatCredits(overview.data.upcoming_required_credits)}
                      </span>
                      <span className="text-[10px] font-medium text-muted-foreground uppercase">{t('billing.credits')}</span>
                    </div>

                  </div>
                </div>
              )}

              {/* Table Controls (Search & Status Filter) */}
              {overview.status === 'success' && overview.data.invoices.length > 0 && (
                <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 pt-1">
                  <div className="relative max-w-xs flex-1">
                    <Search className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground" />
                    <Input
                      placeholder={t('billing.searchInvoices')}
                      value={invoiceSearch}
                      onChange={(e) => setInvoiceSearch(e.target.value)}
                      className="pl-8 text-xs h-8 bg-card"
                    />
                  </div>

                  <div className="inline-flex items-center gap-px rounded-md bg-muted/30 p-px text-[10px] ring-1 ring-border/50">
                    <button
                      type="button"
                      onClick={() => setInvoiceStatusFilter('all')}
                      className={`h-7 rounded-[5px] px-2.5 font-medium transition-colors outline-none focus-visible:ring-1 focus-visible:ring-ring ${
                        invoiceStatusFilter === 'all'
                          ? 'bg-background text-foreground'
                          : 'text-muted-foreground hover:bg-background/50 hover:text-foreground'
                      }`}
                    >
                      {t('billing.allStatuses')}
                    </button>
                    <button
                      type="button"
                      onClick={() => setInvoiceStatusFilter('paid')}
                      className={`h-7 rounded-[5px] px-2.5 font-medium transition-colors outline-none focus-visible:ring-1 focus-visible:ring-ring ${
                        invoiceStatusFilter === 'paid'
                          ? 'bg-background text-foreground'
                          : 'text-muted-foreground hover:bg-background/50 hover:text-foreground'
                      }`}
                    >
                      {formatStatus('paid')}
                    </button>
                    <button
                      type="button"
                      onClick={() => setInvoiceStatusFilter('payment_due')}
                      className={`h-7 rounded-[5px] px-2.5 font-medium transition-colors outline-none focus-visible:ring-1 focus-visible:ring-ring ${
                        invoiceStatusFilter === 'payment_due'
                          ? 'bg-background text-foreground'
                          : 'text-muted-foreground hover:bg-background/50 hover:text-foreground'
                      }`}
                    >
                      {formatStatus('payment_due')}
                    </button>
                  </div>
                </div>
              )}

              {/* Invoices Table */}
              <div className="rounded-xl border border-border/60 overflow-hidden bg-card/50">
                <Table>
                  <TableHeader className="bg-muted/40">
                    <TableRow className="border-b border-border/40 hover:bg-transparent">
                      <TableHead className="w-[18%] pl-4 text-[11px] font-medium tracking-[0.01em] text-muted-foreground/80">
                        {t('billing.invoiceNumber')}
                      </TableHead>
                      <TableHead className="w-[26%] text-[11px] font-medium tracking-[0.01em] text-muted-foreground/80">
                        {t('billing.servicePeriod')}
                      </TableHead>
                      <TableHead className="w-[20%] text-[11px] font-medium tracking-[0.01em] text-muted-foreground/80">
                        {t('billing.totalCharged')}
                      </TableHead>
                      <TableHead className="w-[14%] text-[11px] font-medium tracking-[0.01em] text-muted-foreground/80">
                        {t('billing.invoiceStatus')}
                      </TableHead>
                      <TableHead className="w-[12%] text-[11px] font-medium tracking-[0.01em] text-muted-foreground/80">
                        {t('billing.periodLabel')}
                      </TableHead>
                      <TableHead className="w-[10%] pr-4 text-right text-[11px] font-medium tracking-[0.01em] text-muted-foreground/80">
                        {t('billing.viewInvoice')}
                      </TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {overview.status === 'loading' && (
                      <TableRow>
                        <TableCell colSpan={6} className="text-center py-6 text-xs text-muted-foreground">{t('billing.loadingInvoices')}</TableCell>
                      </TableRow>
                    )}
                    {overview.status === 'success' && overview.data.invoices.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={6} className="text-center py-8 text-xs text-muted-foreground">{t('billing.noInvoices')}</TableCell>
                      </TableRow>
                    )}
                    {overview.status === 'success' && overview.data.invoices.length > 0 && filteredInvoices.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={6} className="text-center py-8 text-xs text-muted-foreground">
                          {t('billing.noMatchingInvoices')}
                        </TableCell>
                      </TableRow>
                    )}
                    {overview.status === 'success' && filteredInvoices.slice(invoicePage.start, invoicePage.end).map((invoice) => {
                      const invNumber = getInvoiceNumber(invoice)
                      return (
                        <TableRow key={invoice.id} className="group h-11 border-b border-border/30 transition-colors last:border-0 hover:bg-muted/30">
                          {/* Invoice Number */}
                          <TableCell className="pl-4 font-mono text-[11px] font-medium tracking-[-0.01em] text-foreground/90">
                            <div className="flex items-center gap-1.5">
                              <ReceiptText className="size-3.5 shrink-0 text-muted-foreground/70" />
                              <span className="truncate">{invNumber}</span>
                              <Button
                                variant="ghost"
                                size="icon"
                                className="size-5 text-muted-foreground/60 hover:text-foreground opacity-0 group-hover:opacity-100 transition-opacity"
                                onClick={() => handleCopyInvoiceNumber(invNumber, invoice.id)}
                                title={t('billing.copyInvoiceNumber')}
                              >
                                {copiedInvoiceID === invoice.id ? (
                                  <Check className="size-2.5 text-emerald-500" />
                                ) : (
                                  <Copy className="size-2.5" />
                                )}
                              </Button>
                            </div>
                          </TableCell>

                          {/* Period */}
                          <TableCell className="text-[11px] tabular-nums text-foreground/90">
                            <div className="flex items-center gap-1.5">
                              <Calendar className="size-3 text-muted-foreground shrink-0" />
                              <span className="font-medium">
                                {formatDate(invoice.period_start)} - {formatDate(invoice.period_end)}
                              </span>
                            </div>
                          </TableCell>

                          {/* Credits Charged & IDR Equivalent */}
                          <TableCell className="text-xs tabular-nums">
                            <div>
                              <span className="font-semibold text-foreground">
                                {formatCredits(invoice.total_credits)} {t('billing.credits')}
                              </span>

                            </div>
                          </TableCell>

                          {/* Status */}
                          <TableCell className="text-xs">
                            <StatusBadge status={invoice.status} className="text-[11px]" />
                          </TableCell>

                          {/* Date */}
                          <TableCell className="text-[11px] tabular-nums text-muted-foreground">
                            {invoice.paid_at
                              ? t('billing.paidOn', { date: formatDate(invoice.paid_at) })
                              : invoice.due_at
                                ? t('billing.dueOn', { date: formatDate(invoice.due_at) })
                                : formatDate(invoice.created_at)}
                          </TableCell>

                          {/* Action Button */}
                          <TableCell className="text-xs text-right pr-4">
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => setSelectedInvoice(invoice)}
                              className="h-7 gap-1 px-2.5 text-[11px] font-medium text-foreground shadow-none hover:bg-muted/80"
                            >
                              <ReceiptText className="size-3 text-muted-foreground" />
                              {t('billing.viewInvoice')}
                            </Button>
                          </TableCell>
                        </TableRow>
                      )
                    })}
                  </TableBody>
                </Table>
                <TablePagination state={invoicePage} disabled={overview.status === 'loading'} />
              </div>
            </TabsContent>

            {/* Top-ups Tab */}
            <TabsContent value="topups" className="space-y-4">
              <div className="rounded-xl border border-border/60 overflow-hidden bg-card/50">
                <Table>
                  <TableHeader className="bg-muted/40">
                    <TableRow className="border-b border-border/40 hover:bg-transparent">
                      <TableHead className="w-[18%] pl-4 text-[11px] font-medium tracking-[0.01em] text-muted-foreground/80">{t('billing.orderId')}</TableHead>
                      <TableHead className="w-[20%] text-[11px] font-medium tracking-[0.01em] text-muted-foreground/80">{t('billing.creditsPurchased')}</TableHead>
                      <TableHead className="w-[20%] text-[11px] font-medium tracking-[0.01em] text-muted-foreground/80">{t('billing.amountPaid')}</TableHead>
                      <TableHead className="w-[18%] text-[11px] font-medium tracking-[0.01em] text-muted-foreground/80">{t('billing.status')}</TableHead>
                      <TableHead className="w-[24%] pr-4 text-right text-[11px] font-medium tracking-[0.01em] text-muted-foreground/80">{t('billing.date')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {overview.status === 'loading' && (
                      <TableRow>
                        <TableCell colSpan={5} className="text-center py-6 text-xs text-muted-foreground">{t('billing.loadingTopups')}</TableCell>
                      </TableRow>
                    )}
                    {overview.status === 'success' && overview.data.topups.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={5} className="text-center py-8 text-xs text-muted-foreground">{t('billing.noTopups')}</TableCell>
                      </TableRow>
                    )}
                    {overview.status === 'success' && overview.data.topups.slice(topupPage.start, topupPage.end).map((topup) => (
                      <TableRow key={topup.id} className="h-11 border-b border-border/30 transition-colors last:border-0 hover:bg-muted/30">
                        <TableCell className="pl-4 font-mono text-[11px] font-medium tracking-[-0.01em] text-foreground/90">
                          #topup-{topup.id}
                        </TableCell>
                        <TableCell className="text-xs">
                          <span className="inline-flex items-center rounded-full border border-emerald-500/20 bg-emerald-500/10 px-2.5 py-0.5 text-[10px] font-medium tabular-nums text-emerald-600 dark:text-emerald-400">
                            +{formatCredits(topup.credits)} {t('billing.credits')}
                          </span>
                        </TableCell>
                        <TableCell className="text-xs font-semibold tabular-nums text-foreground">
                          {formatMoney(topup.amount_minor, topup.currency)}
                        </TableCell>
                        <TableCell className="text-xs">
                          <StatusBadge status={topup.status} className="text-[11px]" />
                        </TableCell>
                        <TableCell className="pr-4 text-right text-[11px] tabular-nums text-muted-foreground">
                          <div className="flex items-center justify-end gap-2">
                            <span>{topup.paid_at ? t('billing.paidOn', { date: formatDate(topup.paid_at) }) : formatDate(topup.created_at)}</span>
                            {topup.status === 'pending' && (
                              <div className="flex items-center gap-1.5">
                                {onPayPendingTopup && (topup.payment_token || topup.payment_url) && (
                                  <Button
                                    size="sm"
                                    onClick={() => onPayPendingTopup(topup)}
                                    className="h-7 gap-1.5 px-2.5 text-[11px] font-medium"
                                  >
                                    <CreditCard className="size-3" aria-hidden="true" />
                                    {t('billing.payNow')}
                                  </Button>
                                )}
                                <Button
                                  size="icon-sm"
                                  variant="ghost"
                                  onClick={() => void reconcileTopup(topup.id)}
                                  className="text-muted-foreground hover:text-foreground"
                                  title={t('billing.checkStatus')}
                                >
                                  <RefreshCw className="size-3.5" aria-hidden="true" />
                                  <span className="sr-only">{t('billing.checkStatus')}</span>
                                </Button>
                              </div>
                            )}
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
                <TablePagination state={topupPage} disabled={overview.status === 'loading'} />
              </div>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>
  )
}
