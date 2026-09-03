import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ArrowDownRight, ArrowUpRight, Calendar, Check, Copy, CreditCard, ReceiptText, RefreshCw, Search, WalletCards } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { TablePagination } from '@/components/ui/table-pagination'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import type { BillingRequestState } from '@/lib/billing-ui'
import { serverPagination } from '@/lib/pagination'
import { billingAPI } from '@/services/api'
import type { BillingOverview } from '@/types'
import { getInvoiceNumber } from './utils'
import { useBillingFormatters } from './useBillingFormatters'
import { StatusBadge } from './StatusBadge'

type Invoice = BillingOverview['invoices'][number]
type TopupItem = BillingOverview['topups'][number]
type LedgerItem = BillingOverview['wallet']['ledger_entries'][number]

type BillingHistorySectionProps = {
  overview: BillingRequestState<BillingOverview>
  copiedInvoiceID: number | null
  handleCopyInvoiceNumber: (invoiceNumber: string, id: number) => void
  setSelectedInvoice: (invoice: Invoice) => void
  reconcileTopup: (topupID: number) => Promise<void>
  onPayPendingTopup?: (topup: TopupItem) => void
}

export const BillingHistorySection = memo(function BillingHistorySection({
  overview,
  copiedInvoiceID,
  handleCopyInvoiceNumber,
  setSelectedInvoice,
  reconcileTopup,
  onPayPendingTopup,
}: BillingHistorySectionProps) {
  const { t, formatCredits, formatDate, formatMoney, formatStatus, translateLedgerType } = useBillingFormatters()
  const [activeTab, setActiveTab] = useState('wallet_activity')

  const tRef = useRef(t)
  tRef.current = t

  const invoiceReqSeq = useRef(0)
  const topupReqSeq = useRef(0)
  const ledgerReqSeq = useRef(0)

  // Invoices Server Pagination
  const [invoicePage, setInvoicePage] = useState(1)
  const [invoiceLimit, setInvoiceLimit] = useState(10)
  const [invoiceSearch, setInvoiceSearch] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [invoiceStatusFilter, setInvoiceStatusFilter] = useState<'all' | 'paid' | 'payment_due'>('all')
  const [serverInvoices, setServerInvoices] = useState<{ data: Invoice[]; total: number; page: number; limit: number; search: string; status: string } | null>(null)
  const [loadingInvoices, setLoadingInvoices] = useState(false)
  const [invoiceError, setInvoiceError] = useState<string | null>(null)
  const invoiceAbortControllerRef = useRef<AbortController | null>(null)

  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(invoiceSearch)
    }, 300)
    return () => clearTimeout(timer)
  }, [invoiceSearch])

  useEffect(() => {
    return () => {
      if (invoiceAbortControllerRef.current) {
        invoiceAbortControllerRef.current.abort()
      }
    }
  }, [])

  // Topups Server Pagination
  const [topupPage, setTopupPage] = useState(1)
  const [topupLimit, setTopupLimit] = useState(10)
  const [serverTopups, setServerTopups] = useState<{ data: TopupItem[]; total: number; page: number; limit: number } | null>(null)
  const [loadingTopups, setLoadingTopups] = useState(false)
  const [topupError, setTopupError] = useState<string | null>(null)

  // Ledger Server Pagination
  const [ledgerPage, setLedgerPage] = useState(1)
  const [ledgerLimit, setLedgerLimit] = useState(10)
  const [serverLedger, setServerLedger] = useState<{ data: LedgerItem[]; total: number; page: number; limit: number } | null>(null)
  const [loadingLedger, setLoadingLedger] = useState(false)
  const [ledgerError, setLedgerError] = useState<string | null>(null)

  const fetchInvoices = useCallback(async (p: number, l: number, s: string, stat: 'all' | 'paid' | 'payment_due') => {
    if (invoiceAbortControllerRef.current) {
      invoiceAbortControllerRef.current.abort()
    }
    const controller = new AbortController()
    invoiceAbortControllerRef.current = controller
    const seq = ++invoiceReqSeq.current
    setLoadingInvoices(true)
    try {
      const res = await billingAPI.invoices({
        page: p,
        limit: l,
        search: s.trim() || undefined,
        status: stat !== 'all' ? stat : undefined,
      }, { signal: controller.signal })
      if (seq === invoiceReqSeq.current) {
        if (res.data) {
          setServerInvoices({
            data: res.data.data,
            total: res.data.total,
            page: p,
            limit: l,
            search: s,
            status: stat,
          })
          setInvoiceError(null)
        }
      }
    } catch (err: any) {
      if (err?.name === 'CanceledError' || err?.name === 'AbortError' || err?.code === 'ERR_CANCELED') {
        return
      }
      if (seq === invoiceReqSeq.current) {
        setInvoiceError(tRef.current('billing.failedToLoadInvoices'))
      }
    } finally {
      if (seq === invoiceReqSeq.current) {
        setLoadingInvoices(false)
      }
    }
  }, [])

  const fetchTopups = useCallback(async (p: number, l: number) => {
    const seq = ++topupReqSeq.current
    setLoadingTopups(true)
    try {
      const res = await billingAPI.topups({ page: p, limit: l })
      if (seq === topupReqSeq.current) {
        if (res.data) {
          setServerTopups({
            data: res.data.data,
            total: res.data.total,
            page: p,
            limit: l,
          })
          setTopupError(null)
        }
      }
    } catch {
      if (seq === topupReqSeq.current) {
        setTopupError(tRef.current('billing.failedToLoadTopups'))
      }
    } finally {
      if (seq === topupReqSeq.current) {
        setLoadingTopups(false)
      }
    }
  }, [])

  const fetchLedger = useCallback(async (p: number, l: number) => {
    const seq = ++ledgerReqSeq.current
    setLoadingLedger(true)
    try {
      const res = await billingAPI.ledger({ page: p, limit: l })
      if (seq === ledgerReqSeq.current) {
        if (res.data) {
          setServerLedger({
            data: res.data.data,
            total: res.data.total,
            page: p,
            limit: l,
          })
          setLedgerError(null)
        }
      }
    } catch {
      if (seq === ledgerReqSeq.current) {
        setLedgerError(tRef.current('billing.failedToLoadLedger'))
      }
    } finally {
      if (seq === ledgerReqSeq.current) {
        setLoadingLedger(false)
      }
    }
  }, [])

  useEffect(() => {
    if (overview.status === 'success') {
      if (activeTab === 'invoices') {
        void fetchInvoices(invoicePage, invoiceLimit, debouncedSearch, invoiceStatusFilter)
      } else if (activeTab === 'topups') {
        void fetchTopups(topupPage, topupLimit)
      } else if (activeTab === 'wallet_activity') {
        void fetchLedger(ledgerPage, ledgerLimit)
      }
    }
  }, [
    activeTab,
    invoicePage,
    invoiceLimit,
    debouncedSearch,
    invoiceStatusFilter,
    topupPage,
    topupLimit,
    ledgerPage,
    ledgerLimit,
    overview,
    fetchInvoices,
    fetchTopups,
    fetchLedger,
  ])

  const handleSearchChange = (val: string) => {
    setInvoiceSearch(val)
    setInvoicePage(1)
  }

  const handleStatusFilterChange = (val: 'all' | 'paid' | 'payment_due') => {
    setInvoiceStatusFilter(val)
    setInvoicePage(1)
  }

  const isInvoiceFilterActive = debouncedSearch.trim() !== '' || invoiceStatusFilter !== 'all'

  const displayedInvoices = useMemo(() => {
    if (
      serverInvoices &&
      serverInvoices.page === invoicePage &&
      serverInvoices.limit === invoiceLimit &&
      serverInvoices.search === debouncedSearch &&
      serverInvoices.status === invoiceStatusFilter
    ) {
      return serverInvoices.data
    }
    if (!isInvoiceFilterActive && overview.status === 'success') {
      const start = (invoicePage - 1) * invoiceLimit
      return overview.data.invoices.slice(start, start + invoiceLimit)
    }
    return []
  }, [serverInvoices, overview, invoicePage, invoiceLimit, debouncedSearch, invoiceStatusFilter, isInvoiceFilterActive])

  const totalInvoices =
    serverInvoices &&
    serverInvoices.search === debouncedSearch &&
    serverInvoices.status === invoiceStatusFilter
      ? serverInvoices.total
      : (!isInvoiceFilterActive && overview.status === 'success' ? overview.data.invoices.length : 0)

  const invoicePaging = serverPagination(
    invoicePage,
    invoiceLimit,
    totalInvoices,
    setInvoicePage,
    setInvoiceLimit,
  )

  const displayedTopups = useMemo(() => {
    if (serverTopups && serverTopups.page === topupPage && serverTopups.limit === topupLimit) {
      return serverTopups.data
    }
    if (overview.status === 'success') {
      const start = (topupPage - 1) * topupLimit
      return overview.data.topups.slice(start, start + topupLimit)
    }
    return []
  }, [serverTopups, overview, topupPage, topupLimit])

  const totalTopups = serverTopups?.total ?? (overview.status === 'success' ? overview.data.topups.length : 0)
  const topupPaging = serverPagination(
    topupPage,
    topupLimit,
    totalTopups,
    setTopupPage,
    setTopupLimit,
  )

  const displayedLedger = useMemo(() => {
    if (serverLedger && serverLedger.page === ledgerPage && serverLedger.limit === ledgerLimit) {
      return serverLedger.data
    }
    if (overview.status === 'success') {
      const start = (ledgerPage - 1) * ledgerLimit
      return overview.data.wallet.ledger_entries.slice(start, start + ledgerLimit)
    }
    return []
  }, [serverLedger, overview, ledgerPage, ledgerLimit])

  const totalLedger = serverLedger?.total ?? (overview.status === 'success' ? overview.data.wallet.ledger_entries.length : 0)
  const ledgerPaging = serverPagination(
    ledgerPage,
    ledgerLimit,
    totalLedger,
    setLedgerPage,
    setLedgerLimit,
  )

  const invoiceMetrics = useMemo(() => {
    if (overview.status !== 'success') return { totalCreditsInvoiced: 0 }
    return {
      totalCreditsInvoiced:
        overview.data.total_invoiced_credits ??
        overview.data.invoices.reduce((total, invoice) => total + (invoice.total_credits || 0), 0),
    }
  }, [overview])

  const ledgerRows = useMemo(
    () =>
      displayedLedger.map((entry, index) => {
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
    [displayedLedger, formatCredits, formatDate, translateLedgerType, t],
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
          <Tabs value={activeTab} onValueChange={setActiveTab} className="space-y-4">
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
              {ledgerError && (
                <div className="rounded-lg border border-destructive/20 bg-destructive/10 p-3 text-xs text-destructive flex items-center justify-between">
                  <span>{ledgerError}</span>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => void fetchLedger(ledgerPage, ledgerLimit)}
                    className="h-7 text-xs shadow-none hover:bg-background"
                  >
                    <RefreshCw className="size-3 mr-1" /> {t('billing.retry')}
                  </Button>
                </div>
              )}
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
                    {overview.status === 'success' && displayedLedger.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={4} className="text-center py-8 text-xs text-muted-foreground">{t('billing.noWalletActivity')}</TableCell>
                      </TableRow>
                    )}
                    {ledgerRows}
                  </TableBody>
                </Table>
                <TablePagination state={ledgerPaging} disabled={overview.status === 'loading' || loadingLedger} />
              </div>
            </TabsContent>

            {/* Invoices Tab */}
            <TabsContent value="invoices" className="space-y-4">
              {invoiceError && (
                <div className="rounded-lg border border-destructive/20 bg-destructive/10 p-3 text-xs text-destructive flex items-center justify-between">
                  <span>{invoiceError}</span>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => void fetchInvoices(invoicePage, invoiceLimit, invoiceSearch, invoiceStatusFilter)}
                    className="h-7 text-xs shadow-none hover:bg-background"
                  >
                    <RefreshCw className="size-3 mr-1" /> {t('billing.retry')}
                  </Button>
                </div>
              )}

              {/* Financial Quick Metrics */}
              {overview.status === 'success' && (
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
              {overview.status === 'success' && (
                <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-3 pt-1">
                  <div className="relative max-w-xs flex-1">
                    <Search className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground" />
                    <Input
                      placeholder={t('billing.searchInvoices')}
                      value={invoiceSearch}
                      onChange={(e) => handleSearchChange(e.target.value)}
                      className="pl-8 text-xs h-8 bg-card"
                    />
                  </div>

                  <div className="inline-flex items-center gap-px rounded-md bg-muted/30 p-px text-[10px] ring-1 ring-border/50">
                    <button
                      type="button"
                      onClick={() => handleStatusFilterChange('all')}
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
                      onClick={() => handleStatusFilterChange('paid')}
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
                      onClick={() => handleStatusFilterChange('payment_due')}
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
                    {overview.status === 'success' && displayedInvoices.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={6} className="text-center py-8 text-xs text-muted-foreground">
                          {isInvoiceFilterActive ? t('billing.noMatchingInvoices') : t('billing.noInvoices')}
                        </TableCell>
                      </TableRow>
                    )}
                    {overview.status === 'success' && displayedInvoices.map((invoice) => {
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
                <TablePagination state={invoicePaging} disabled={overview.status === 'loading' || loadingInvoices} />
              </div>
            </TabsContent>

            {/* Top-ups Tab */}
            <TabsContent value="topups" className="space-y-4">
              {topupError && (
                <div className="rounded-lg border border-destructive/20 bg-destructive/10 p-3 text-xs text-destructive flex items-center justify-between">
                  <span>{topupError}</span>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => void fetchTopups(topupPage, topupLimit)}
                    className="h-7 text-xs shadow-none hover:bg-background"
                  >
                    <RefreshCw className="size-3 mr-1" /> {t('billing.retry')}
                  </Button>
                </div>
              )}
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
                    {overview.status === 'success' && displayedTopups.length === 0 && (
                      <TableRow>
                        <TableCell colSpan={5} className="text-center py-8 text-xs text-muted-foreground">{t('billing.noTopups')}</TableCell>
                      </TableRow>
                    )}
                    {overview.status === 'success' && displayedTopups.map((topup) => (
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
                <TablePagination state={topupPaging} disabled={overview.status === 'loading' || loadingTopups} />
              </div>
            </TabsContent>
          </Tabs>
        </CardContent>
      </Card>
  )
})
