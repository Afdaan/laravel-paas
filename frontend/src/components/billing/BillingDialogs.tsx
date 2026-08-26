import type { Dispatch, SetStateAction } from 'react'
import { AlertTriangle, ArrowRight, Check, Copy, Database, ExternalLink, FolderGit2, Printer, ReceiptText, RefreshCw, ShieldCheck } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import type { BillingRequestState } from '@/lib/billing-ui'
import type { BillingOverview, BillingProfile, TopupResponse } from '@/types'
import type { PendingRenewChange, PendingTopup } from './types'
import { getInvoiceNumber } from './utils'
import { useBillingFormatters } from './useBillingFormatters'
import { StatusBadge } from './StatusBadge'

type Invoice = BillingOverview['invoices'][number]

export function RenewConfirmationDialog({
  pendingRenewChange,
  setPendingRenewChange,
  confirmRenewChange,
}: {
  pendingRenewChange: PendingRenewChange | null
  setPendingRenewChange: Dispatch<SetStateAction<PendingRenewChange | null>>
  confirmRenewChange: () => Promise<void>
}) {
  const { t } = useBillingFormatters()

  return (
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
          <Button onClick={() => void confirmRenewChange()}>{t('billing.confirmChange')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

export function ProfileRequiredDialog({
  showProfilePrompt,
  setShowProfilePrompt,
  scrollToBillingProfile,
}: {
  showProfilePrompt: boolean
  setShowProfilePrompt: Dispatch<SetStateAction<boolean>>
  scrollToBillingProfile: () => void
}) {
  const { t } = useBillingFormatters()

  return (
      <Dialog open={showProfilePrompt} onOpenChange={setShowProfilePrompt}>
        <DialogContent className="sm:max-w-md border-border">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2 text-base font-bold text-amber-500">
              <AlertTriangle className="size-4.5" />
              {t('billing.profile.profileRequiredTitle')}
            </DialogTitle>
            <DialogDescription className="text-xs text-muted-foreground pt-1.5 leading-relaxed">
              {t('billing.profile.profileRequiredDesc')}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="pt-2">
            <Button
              size="sm"
              onClick={() => {
                setShowProfilePrompt(false)
                scrollToBillingProfile()
              }}
              className="font-semibold text-xs"
            >
              {t('billing.profile.completeProfileBtn')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
  )
}

export function TopupConfirmationDialog({
  pendingTopup,
  setPendingTopup,
  topupPackageID,
  overview,
  conversionRateFormatted,
  handleConfirmTopup,
}: {
  pendingTopup: PendingTopup | null
  setPendingTopup: Dispatch<SetStateAction<PendingTopup | null>>
  topupPackageID: number | null
  overview: BillingRequestState<BillingOverview>
  conversionRateFormatted: string
  handleConfirmTopup: () => Promise<void>
}) {
  const { t, formatCredits, formatMoney } = useBillingFormatters()

  return (
      <Dialog open={pendingTopup !== null} onOpenChange={(open) => !open && setPendingTopup(null)}>
        <DialogContent className="sm:max-w-[440px] p-0 overflow-hidden bg-card text-foreground rounded-xl">
          {pendingTopup && (
            <div>
              <div className="px-6 pt-6 pb-5">
                <DialogTitle className="text-base font-semibold tracking-tight">
                  {t('billing.confirmTopupTitle')}
                </DialogTitle>
                <DialogDescription className="mt-1 text-xs leading-relaxed text-muted-foreground">
                  {t('billing.confirmTopupDescription')}
                </DialogDescription>
              </div>

              {/* Ledger */}
              <div className="border-y border-border/60 bg-muted/20">
                <div className="flex items-start justify-between px-6 py-5">
                  <div>
                    <span className="text-xs font-medium text-muted-foreground">
                      {t('billing.creditsToAdd')}
                    </span>
                    <div className="mt-1.5 flex items-baseline gap-1.5">
                      <span className="text-3xl font-semibold tracking-tight tabular-nums">
                        +{formatCredits(pendingTopup.credits)}
                      </span>
                      <span className="text-sm text-muted-foreground">{t('billing.credits')}</span>
                    </div>
                  </div>
                  <Badge variant="secondary" className="mt-0.5 shrink-0 text-[10px] font-medium px-2 py-0.5">
                    {pendingTopup.type === 'package' ? t('billing.fixedPackage') : t('billing.customPackage')}
                  </Badge>
                </div>

                <dl className="divide-y divide-border/50 border-t border-border/50 px-6 text-xs">
                  <div className="flex items-center justify-between py-3">
                    <dt className="text-muted-foreground">{t('billing.packagePrice')}</dt>
                    <dd className="font-semibold tabular-nums">
                      {formatMoney(pendingTopup.amountMinor, pendingTopup.currency)}
                    </dd>
                  </div>
                  <div className="flex items-center justify-between py-3">
                    <dt className="text-muted-foreground">{t('billing.conversionRate')}</dt>
                    <dd className="tabular-nums">
                      {t('billing.conversionRateValue', { rate: conversionRateFormatted })}
                    </dd>
                  </div>
                  <div className="flex items-center justify-between py-3">
                    <dt className="text-muted-foreground">{t('billing.balanceProgression')}</dt>
                    <dd className="flex items-center gap-1.5 tabular-nums">
                      <span className="text-muted-foreground">
                        {formatCredits(overview.status === 'success' ? overview.data.wallet.balance_credits : 0)}
                      </span>
                      <ArrowRight className="size-3 shrink-0 text-muted-foreground/60" aria-hidden="true" />
                      <span className="font-semibold">
                        {formatCredits((overview.status === 'success' ? overview.data.wallet.balance_credits : 0) + pendingTopup.credits)} {t('billing.credits')}
                      </span>
                    </dd>
                  </div>
                </dl>
              </div>

              <div className="flex items-start gap-2 px-6 py-4 text-[11px] leading-relaxed text-muted-foreground">
                <ShieldCheck className="mt-px size-3.5 shrink-0" aria-hidden="true" />
                <p>
                  {t('billing.paymentSecurityNote')} {t('billing.checkoutNotice')}
                </p>
              </div>

              <div className="flex flex-col-reverse gap-2 border-t border-border/60 px-6 py-4 sm:flex-row sm:justify-end">
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => setPendingTopup(null)}
                  disabled={topupPackageID !== null}
                  className="text-xs font-medium"
                >
                  {t('billing.cancelTopup')}
                </Button>
                <Button
                  size="sm"
                  onClick={() => void handleConfirmTopup()}
                  disabled={topupPackageID !== null}
                  className="gap-1.5 text-xs font-semibold"
                >
                  {topupPackageID !== null && <RefreshCw className="size-3 animate-spin" aria-hidden="true" />}
                  {topupPackageID !== null ? t('billing.openingCheckout') : t('billing.proceedToPayment')}
                </Button>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>
  )
}

export function PaymentDialog({
  activePaymentModal,
  setActivePaymentModal,
  checkingPaymentStatus,
  handleCheckStatusModal,
}: {
  activePaymentModal: TopupResponse | null
  setActivePaymentModal: Dispatch<SetStateAction<TopupResponse | null>>
  checkingPaymentStatus: boolean
  handleCheckStatusModal: () => Promise<void>
}) {
  const { t, formatMoney } = useBillingFormatters()
  const activePaymentURL = activePaymentModal?.payment_url
  // Sandbox tokens prefix the EMV payload with junk ("THIS.IS.JUST.AN.EXAMPLE...000201..."),
  // so match the EMV header anywhere instead of requiring it at position 0.
  const qrisPayload = activePaymentModal?.payment_token?.match(/000201.*/s)?.[0]

  return (
      <Dialog open={!!activePaymentModal} onOpenChange={(open) => !open && setActivePaymentModal(null)}>
        <DialogContent className="gap-0 overflow-hidden border-border/70 p-0 sm:max-w-[420px]">
          <DialogHeader className="px-6 pb-4 pt-6">
            <DialogTitle className="text-center text-base font-semibold tracking-tight">
              {t('billing.paymentDialogTitle')}
            </DialogTitle>
            <DialogDescription className="mx-auto max-w-xs text-center text-xs leading-relaxed text-muted-foreground">
              {t('billing.paymentDialogDescription')}
            </DialogDescription>
          </DialogHeader>

          {activePaymentModal && (
            <div className="flex flex-col items-center gap-4 px-6 pb-5">
              {qrisPayload ? (
                <div className="flex flex-col items-center gap-2.5">
                  {/* QR needs a literal white quiet zone in both themes; semantic tokens would invert it. */}
                  <div className="rounded-2xl bg-muted/60 p-1.5 ring-1 ring-border/60">
                    <div className="rounded-xl bg-white p-3 shadow-[inset_0_1px_0_rgba(255,255,255,0.8)]">
                      <QRCodeSVG value={qrisPayload} size={176} level="M" className="size-44" />
                    </div>
                  </div>
                  <p className="text-center text-xs text-muted-foreground">{t('billing.scanQris')}</p>
                </div>
              ) : activePaymentModal.payment_token ? (
                <div className="w-full min-w-0 rounded-xl border border-border/60 bg-muted/20 p-4 text-center">
                  <p className="mb-1.5 text-xs text-muted-foreground">
                    {t('billing.paymentCode')}
                  </p>
                  <p className="font-mono text-sm font-semibold break-all text-foreground">
                    {activePaymentModal.payment_token}
                  </p>
                </div>
              ) : null}

              <div className="w-full rounded-xl bg-muted/35 px-4 py-3 text-center ring-1 ring-border/50">
                <p className="text-xs text-muted-foreground">{t('billing.totalBill')}</p>
                <p className="mt-0.5 text-2xl font-semibold tracking-tight tabular-nums text-foreground">
                  {formatMoney(activePaymentModal.amount_minor, activePaymentModal.currency)}
                </p>
              </div>

              <p className="inline-flex items-center gap-2 text-xs text-muted-foreground" role="status" aria-live="polite">
                <span className="size-1.5 rounded-full bg-amber-500" aria-hidden="true" />
                {t('billing.autoCheckingStatus')}
                <span className="waiting-ellipsis" aria-hidden="true">...</span>
              </p>
            </div>
          )}

          <DialogFooter
            className={activePaymentURL
              ? 'mx-0 mb-0 grid grid-cols-2 gap-2 rounded-none border-t border-border/60 bg-muted/30 px-6 py-4 sm:grid-cols-2'
              : 'mx-0 mb-0 flex justify-center rounded-none border-t border-border/60 bg-muted/30 px-6 py-4 sm:justify-center'}
          >
            <Button
              variant="outline"
              size="sm"
              onClick={handleCheckStatusModal}
              disabled={checkingPaymentStatus}
              className={activePaymentURL ? 'gap-1.5 shadow-none' : 'min-w-36 gap-1.5 shadow-none'}
            >
              <RefreshCw className={`size-3 shrink-0 ${checkingPaymentStatus ? 'animate-spin' : ''}`} aria-hidden="true" />
              {checkingPaymentStatus ? t('billing.checkingStatus') : t('billing.checkStatus')}
            </Button>
            {activePaymentURL && (
              // Opens in a new tab so the dialog stays mounted and keeps polling for the paid status.
              <Button
                size="sm"
                onClick={() => window.open(activePaymentURL, '_blank', 'noopener,noreferrer')}
                className="gap-1.5"
              >
                {t('billing.completePayment')}
                <ExternalLink className="size-3" aria-hidden="true" />
              </Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
  )
}

export function InvoiceDialog({
  selectedInvoice,
  setSelectedInvoice,
  profile,
  copiedInvoiceID,
  handleCopyInvoiceNumber,
}: {
  selectedInvoice: Invoice | null
  setSelectedInvoice: Dispatch<SetStateAction<Invoice | null>>
  profile: BillingProfile
  copiedInvoiceID: number | null
  handleCopyInvoiceNumber: (invoiceNumber: string, id: number) => void
}) {
  const {
    t,
    formatCredits,
    formatDate,
    formatResourceDisplayName,
  } = useBillingFormatters()

  return (
      <Dialog open={selectedInvoice !== null} onOpenChange={(open) => !open && setSelectedInvoice(null)}>
        <DialogContent className="sm:max-w-2xl max-h-[90vh] overflow-y-auto p-0 border-border/80 bg-card text-foreground">
          {selectedInvoice && (
            <div>
              {/* Modal Top Bar */}
              <div className="flex items-center justify-between pl-5 pr-11 py-2.5 border-b border-border/60 bg-muted/30">
                <div className="flex items-center gap-2">
                  <ReceiptText className="size-4 text-primary" />
                  <span className="font-mono text-xs font-bold text-foreground">
                    {getInvoiceNumber(selectedInvoice)}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-6 text-muted-foreground hover:text-foreground"
                    onClick={() => handleCopyInvoiceNumber(getInvoiceNumber(selectedInvoice), selectedInvoice.id)}
                    title={t('billing.copyInvoiceNumber')}
                  >
                    {copiedInvoiceID === selectedInvoice.id ? (
                      <Check className="size-3 text-emerald-500" />
                    ) : (
                      <Copy className="size-3" />
                    )}
                  </Button>
                </div>
                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => window.print()}
                    className="h-7 px-2.5 text-xs font-medium text-muted-foreground hover:text-foreground rounded-lg border-border/60 bg-background/60 hover:bg-background shadow-none transition-all gap-1.5"
                  >
                    <Printer className="size-3.5 opacity-70" />
                    {t('billing.printInvoice')}
                  </Button>
                </div>
              </div>

              {/* Official Printable Invoice Document */}
              <div className="p-6 space-y-6 text-foreground bg-card">
                {/* Document Header */}
                <div className="flex flex-col sm:flex-row justify-between gap-4 pb-6 border-b border-border/60">
                  <div>
                    <div className="flex items-center gap-2">
                      <div className="size-7 rounded-lg bg-primary/10 flex items-center justify-center text-primary font-bold text-xs">
                        <ReceiptText className="size-4" />
                      </div>
                      <h2 className="text-base font-bold tracking-tight text-foreground">
                        {t('billing.invoiceStatementTitle')}
                      </h2>
                    </div>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {t('billing.statementDisclaimer')}
                    </p>
                  </div>

                  <div className="sm:text-right space-y-1">
                    <span className="inline-flex items-center gap-1 text-xs font-medium text-muted-foreground px-2 py-0.5 rounded-md bg-muted/40 border border-border/60">
                      {t('billing.invoiceNumber')}
                    </span>
                    <h3 className="text-base font-mono font-bold tracking-tight text-foreground pt-0.5">
                      {getInvoiceNumber(selectedInvoice)}
                    </h3>
                    <p className="text-xs text-muted-foreground">
                      {t('billing.servicePeriod')}: <span className="font-medium text-foreground">{formatDate(selectedInvoice.period_start)} – {formatDate(selectedInvoice.period_end)}</span>
                    </p>
                    {selectedInvoice.paid_at && (
                      <p className="text-xs text-emerald-600 dark:text-emerald-400 font-medium">
                        {t('billing.paidOn', { date: formatDate(selectedInvoice.paid_at) })}
                      </p>
                    )}
                  </div>
                </div>

                {/* Billed To & Payment Meta */}
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 p-4 rounded-xl border border-border/60 bg-muted/20">
                  <div>
                    <p className="text-xs font-medium text-muted-foreground">
                      {t('billing.billedTo')}:
                    </p>
                    <h4 className="mt-0.5 text-xs font-bold text-foreground">
                      {profile.company_name || profile.email || '—'}
                    </h4>
                    <div className="mt-1 space-y-0.5 text-[11px] text-muted-foreground">
                      {profile.tax_id && (
                        <p>
                          <span className="font-medium">{t('billing.profile.taxId')}:</span>{' '}
                          <span className="font-mono text-foreground">{profile.tax_id}</span>
                        </p>
                      )}
                      {profile.address_line1 && (
                        <p>
                          {profile.address_line1}, {profile.city} {profile.postal_code}
                        </p>
                      )}
                      {profile.email && <p>{profile.email}</p>}
                    </div>
                  </div>

                  <div className="sm:text-right space-y-2">
                    <div>
                      <p className="text-xs font-medium text-muted-foreground">
                        {t('billing.invoiceStatus')}
                      </p>
                      <div className="mt-1 flex sm:justify-end">
                        <StatusBadge status={selectedInvoice.status} />
                      </div>
                    </div>
                    <div>
                      <p className="text-xs font-medium text-muted-foreground">
                        {t('billing.paymentMethod')}
                      </p>
                      <p className="text-xs font-medium text-foreground">{t('billing.paymentMethodWallet')}</p>
                    </div>
                  </div>
                </div>

                {/* Itemized Line Items Table */}
                <div className="space-y-2">
                  <h4 className="text-xs font-medium text-muted-foreground">
                    {t('billing.itemDescription')}
                  </h4>
                  <div className="rounded-xl border border-border/60 overflow-hidden bg-card">
                    <Table>
                      <TableHeader className="bg-muted/40">
                        <TableRow className="border-b border-border/40">
                          <TableHead className="text-[10px] font-bold uppercase text-muted-foreground pl-4">
                            {t('billing.itemDescription')}
                          </TableHead>
                          <TableHead className="text-[10px] font-bold uppercase text-muted-foreground">
                            {t('billing.periodLabel')}
                          </TableHead>
                          <TableHead className="text-[10px] font-bold uppercase text-muted-foreground text-right">
                            {t('billing.credits')}
                          </TableHead>

                        </TableRow>
                      </TableHeader>
                      <TableBody>
                        {selectedInvoice.items && selectedInvoice.items.length > 0 ? (
                          selectedInvoice.items.map((item) => (
                            <TableRow key={item.id} className="border-b border-border/30 last:border-0">
                              <TableCell className="pl-4 py-2.5">
                                <div className="flex items-center gap-2">
                                  {item.resource_type === 'project' ? (
                                    <FolderGit2 className="size-3.5 text-cyan-500 shrink-0" />
                                  ) : (
                                    <Database className="size-3.5 text-purple-500 shrink-0" />
                                  )}
                                  <div>
                                    <p className="text-xs font-semibold text-foreground">
                                      {formatResourceDisplayName(item.resource_name, item.resource_type)}
                                    </p>
                                    <p className="text-[10px] text-muted-foreground">
                                      {item.spec_name || item.description}
                                    </p>
                                  </div>
                                </div>
                              </TableCell>
                              <TableCell className="text-[11px] text-muted-foreground font-mono">
                                1 {t('billing.month')}
                              </TableCell>
                              <TableCell className="text-xs font-bold text-foreground text-right font-mono tabular-nums">
                                {formatCredits(item.credits)} {t('billing.credits')}
                              </TableCell>

                            </TableRow>
                          ))
                        ) : (
                          <TableRow className="border-b border-border/30 last:border-0">
                            <TableCell className="pl-4 py-3">
                              <div className="flex items-center gap-2">
                                <ReceiptText className="size-3.5 text-primary shrink-0" />
                                <div>
                                  <p className="text-xs font-semibold text-foreground">{t('billing.resourceBilling')}</p>
                                  <p className="text-[10px] text-muted-foreground">{t('billing.resourceBillingDescription')}</p>
                                </div>
                              </div>
                            </TableCell>
                            <TableCell className="text-[11px] text-muted-foreground font-mono">
                              {formatDate(selectedInvoice.period_start)} – {formatDate(selectedInvoice.period_end)}
                            </TableCell>
                            <TableCell className="text-xs font-bold text-foreground text-right font-mono tabular-nums">
                              {formatCredits(selectedInvoice.total_credits)} {t('billing.credits')}
                            </TableCell>

                          </TableRow>
                        )}
                      </TableBody>
                    </Table>
                  </div>
                </div>

                {/* Subtotal & Total Box */}
                <div className="flex justify-end pt-1">
                  <div className="w-full sm:w-72 space-y-2 rounded-xl border border-border/60 bg-muted/20 p-3.5 text-xs">
                    <div className="flex justify-between text-muted-foreground">
                      <span>{t('billing.subtotal')}</span>
                      <span className="font-mono font-medium text-foreground tabular-nums">
                        {formatCredits(selectedInvoice.total_credits)} {t('billing.credits')}
                      </span>
                    </div>
                    <div className="flex justify-between text-muted-foreground">
                      <span>{t('billing.taxRate')}</span>
                      <span className="font-mono font-medium text-foreground">0</span>
                    </div>
                    <div className="border-t border-border/40 pt-2 flex justify-between items-baseline">
                      <span className="font-bold text-foreground uppercase tracking-wider text-[11px]">
                        {t('billing.totalCharged')}
                      </span>
                      <div className="text-right">
                        <div className="text-sm font-bold text-primary font-mono tabular-nums">
                          {formatCredits(selectedInvoice.total_credits)} {t('billing.credits')}
                        </div>

                      </div>
                    </div>
                  </div>
                </div>

                {/* Legal Electronic Stamp Footer */}
                <div className="pt-3 border-t border-border/40 text-center text-[10px] text-muted-foreground leading-relaxed">
                  {t('billing.statementDisclaimer')}
                </div>
              </div>
            </div>
          )}
        </DialogContent>
      </Dialog>
  )
}
