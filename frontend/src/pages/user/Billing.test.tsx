import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, fireEvent, cleanup } from '@testing-library/react'
import { act } from 'react'
import Billing from './Billing'
import { isValidPhoneNumber } from '@/components/billing/utils'
import { billingAPI } from '@/services/api'

vi.mock('@/lib/usePolling', () => ({ usePolling: vi.fn() }))

vi.mock('@/services/api', () => ({
  billingAPI: {
    overview: vi.fn(),
    catalog: vi.fn(),
    status: vi.fn(),
    createTopup: vi.fn(),
    reconcileTopup: vi.fn(),
    reconcileTopupByRef: vi.fn(),
    updateAutoRenew: vi.fn(),
    payDueResource: vi.fn(),
    getProfile: vi.fn(),
    updateProfile: vi.fn(),
  },
}))

vi.mock('@/lib/useTranslation', () => ({
  default: () => ({
    t: (key: string, data?: Record<string, unknown>) => {
      const map: Record<string, string> = {
        'billing.nav': 'Billing',
        'billing.title': 'Billing',
        'billing.description': 'Description',
        'billing.balance': 'Balance',
        'billing.credits': 'credits',
        'billing.unavailable': 'Unavailable',
        'billing.balanceDescription': 'Description',
        'billing.addCredits': 'Add Credits',
        'billing.addCreditsDescription': 'Add credits',
        'billing.openingCheckout': 'Opening',
        'billing.choosePackage': 'Choose',
        'billing.noPackages': 'No packages',
        'billing.catalogLoadFailed': 'Catalog load failed',
        'billing.customTopupLabel': 'Enter amount (IDR)',
        'billing.customTopupButton': 'Top up',
        'billing.customTopupHint': 'Min Rp 10,000',
        'billing.refresh': 'Refresh',
        'billing.paymentRequired': 'Payment required',
        'billing.paymentRequiredDescription': 'desc',
        'billing.dueOn': 'Due on {{date}}',
        'billing.lowBalance': 'Low balance',
        'billing.lowBalanceDescription': 'Low balance description',
        'billing.staleData': 'Showing previously loaded data. Refresh to update.',
        'billing.resourceTypes.project': 'Project',
        'billing.resourceTypes.database': 'Database',
        'billing.unnamedService': '{{type}} Service',
        'billing.resourceBilling': 'Resource billing',
        'billing.resourceBillingDescription': 'Resource billing description',
        'billing.noBillableResources': 'No active billable resources.',
        'billing.renewsOn': 'Renews on {{date}}',
        'billing.periodEndsOn': 'Billing period ends on {{date}}',
        'billing.renewalPaymentDue': 'Renewal payment due since {{date}}',
        'billing.month': 'month',
        'billing.currentPeriod': 'Current period: {{start}} to {{end}}',
        'billing.unpaidPeriod': 'Unpaid period: {{start}} to {{end}}',
        'billing.payDueNow': 'Pay now',
        'billing.overduePaymentSuccess': 'Payment complete',
        'billing.overdueInsufficientCredits': 'Insufficient credits',
        'billing.overduePaymentFailed': 'Payment failed',
        'billing.autoRenew': 'Auto-renew',
        'billing.autoRenewEnabled': 'Auto-renew enabled',
        'billing.autoRenewRateLimited': 'Too many auto-renew toggle requests. Please wait a moment before trying again.',
        'billing.confirmChange': 'Confirm',
        'billing.confirmTopupTitle': 'Confirm Credit Purchase',
        'billing.confirmTopupDescription': 'Please review your top-up details before proceeding to the payment gateway.',
        'billing.fixedPackage': 'Fixed Package',
        'billing.customPackage': 'Custom Amount',
        'billing.creditsToAdd': 'Credits to Add',
        'billing.packagePrice': 'Credit Price',
        'billing.balanceProgression': 'Wallet Balance',
        'billing.conversionRate': 'Conversion rate',
        'billing.conversionRateValue': '1 Credit = {{rate}}',
        'billing.checkoutNotice': 'Payment method & provider fees (if applicable) are finalized at checkout.',
        'billing.proceedToPayment': 'Proceed to Payment',
        'billing.cancelTopup': 'Cancel',
        'billing.paymentSecurityNote': 'Payment security note',
        'billing.paymentSuccess': 'Payment confirmed! Your wallet has been credited.',
        'billing.paymentPending': 'Payment is still being processed.',
        'billing.paymentVerifyFailed': 'Failed to verify payment',
        'billing.paymentFailed': 'Payment failed or expired.',
        'billing.paymentEnded': 'Payment session ended.',
        'billing.copyInvoiceNumber': 'Copy invoice number',
        'billing.viewInvoice': 'View Invoice',
        'billing.printInvoice': 'Print',
        'billing.invoiceStatementTitle': 'Credit Usage Statement',
        'billing.statementDisclaimer': 'Internal credit statement.',
        'billing.payNow': 'Pay',
        'billing.completePayment': 'Pay Now',
        'billing.profile.companyName': 'Full Name / Company Name',
        'billing.profile.email': 'Billing Email',
        'billing.profile.phone': 'Phone Number',
        'billing.paymentDialogTitle': 'Top-up Payment',
        'billing.paymentDialogDescription': 'Complete the payment to credit your wallet.',
        'billing.scanQris': 'Scan QRIS via Mobile Banking / E-Wallet',
        'billing.paymentCode': 'Payment Code',
        'billing.totalBill': 'Total Amount',
        'billing.openPaymentLink': 'Open Payment Link',
        'billing.checkingStatus': 'Checking...',
        'billing.checkPaymentStatus': 'Check Payment Status',
        'billing.historyDescription': 'Complete transaction ledger.',
        'billing.transactionType': 'Transaction Type',
        'billing.amount': 'Amount',
        'billing.balanceAfterHeader': 'Balance After',
        'billing.date': 'Date',
        'billing.orderId': 'Order ID',
        'billing.creditsPurchased': 'Credits Purchased',
        'billing.amountPaid': 'Amount Paid',
        'billing.status': 'Status',
        'billing.loadingWalletHistory': 'Loading wallet history...',
        'billing.loadingInvoices': 'Loading invoices...',
        'billing.loadingTopups': 'Loading top-ups...',
        'billing.invoices': 'Invoices',
        'billing.topups': 'Top-ups',
        'billing.walletActivity': 'Wallet activity',
        'billing.viewReceipt': 'View Receipt',
        'billing.noInvoices': 'No invoices yet.',
        'billing.totalInvoiced': 'Total invoiced',
        'billing.activeSubscriptions': 'Active subscriptions',
        'billing.upcomingCharges': 'Upcoming charges',
        'billing.searchInvoices': 'Search invoices...',
        'billing.allStatuses': 'All statuses',
        'billing.invoiceNumber': 'Invoice No.',
        'billing.servicePeriod': 'Billing period',
        'billing.totalCharged': 'Total charged',
        'billing.invoiceStatus': 'Invoice status',
        'billing.periodLabel': 'Period',
        'billing.paidOn': 'Paid {{date}}',
        'common.dashboard': 'Dashboard',
      }
      const base = map[key] ?? key
      if (!data) return base
      return Object.entries(data).reduce((s, [k, v]) => s.replace(`{{${k}}}`, String(v)), base)
    },
    language: 'en',
  }),
}))

const mockCatalog = {
  specs: [],
  packages: [
    { id: 1, credits: 100, amount_minor: 100000, currency: 'IDR', sort_order: 1 },
    { id: 2, credits: 250, amount_minor: 225000, currency: 'IDR', sort_order: 2 },
  ],
}

const mockOverview = {
  wallet: { balance_credits: 500, ledger_entries: [] },
  invoices: [],
  topups: [],
  resources: [],
  upcoming_required_credits: 1000,
}

const mockProfile = {
  company_name: 'PT Acme Corp',
  tax_id: '01.234.567.8-901.000',
  email: 'billing@acme.com',
  phone: '+628123456789',
  address_line1: 'Jalan Sudirman 1',
  city: 'Jakarta',
  postal_code: '12190',
  country: 'ID',
}

describe('Billing page', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    window.history.replaceState({}, '', '/billing')
    ;(billingAPI.getProfile as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockProfile })
  })

  afterEach(() => {
    cleanup()
    vi.useRealTimers()
    vi.clearAllMocks()
    window.history.replaceState({}, '', '/billing')
  })

  it('keeps independent fetch states and shows stale warning on polling failure', async () => {
    ;(billingAPI.overview as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({ data: mockOverview })
      .mockResolvedValueOnce({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({ data: mockCatalog })
      .mockRejectedValueOnce(new Error('offline'))
    ;(billingAPI.status as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({ data: [] })
      .mockResolvedValueOnce({ data: [] })

    render(<Billing />)

    await waitFor(() => expect(billingAPI.overview).toHaveBeenCalledTimes(1))
    const balanceElements = await screen.findAllByText(/500/, { exact: false })
    expect(balanceElements[0]).toBeInTheDocument()

    const refreshButton = screen.getByRole('button', { name: /Refresh/i })
    await act(async () => fireEvent.click(refreshButton))
    await waitFor(() => expect(screen.getByText(/Showing previously loaded data/i)).toBeInTheDocument())
  })

  it('does not trigger a request loop', async () => {
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })

    render(<Billing />)

    await waitFor(() => expect(billingAPI.overview).toHaveBeenCalledTimes(1))

    await act(async () => {
      vi.advanceTimersByTime(500)
    })

    expect(billingAPI.overview).toHaveBeenCalledTimes(1)
  })

  it('reconciles a Pakasir top-up after payment redirect using topup_ref', async () => {
    window.history.replaceState({}, '', '/billing?payment_return=pakasir&topup_ref=topup-abcdef0123456789abcdef0123456789')
    ;(billingAPI.reconcileTopupByRef as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { id: 42, status: 'paid' },
    })
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })

    render(<Billing />)

    await waitFor(() =>
      expect(billingAPI.reconcileTopupByRef).toHaveBeenCalledWith('topup-abcdef0123456789abcdef0123456789'),
    )
    await waitFor(() => expect(billingAPI.overview).toHaveBeenCalledTimes(1))
    expect(window.location.search).toBe('')
  })

  it('reconciles a legacy Pakasir top-up after payment redirect with topup_id', async () => {
    window.history.replaceState({}, '', '/billing?payment_return=pakasir&topup_id=42')
    ;(billingAPI.reconcileTopup as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { id: 42, status: 'paid' },
    })
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })

    render(<Billing />)

    await waitFor(() => expect(billingAPI.reconcileTopup).toHaveBeenCalledWith(42))
    await waitFor(() => expect(billingAPI.overview).toHaveBeenCalledTimes(1))
    expect(window.location.search).toBe('')
  })

  it('shows the period end instead of promising renewal when auto-renew is disabled', async () => {
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: {
        ...mockOverview,
        resources: [
          {
            resource_id: 7,
            resource_type: 'project',
            resource_name: 'Storefront',
            spec_name: 'Starter',
            monthly_credits: 75,
            status: 'active',
           current_period_start: '2026-08-01T00:00:00Z',
           next_invoice_at: '2026-09-01T00:00:00Z',
            auto_renew: false,
          },
        ],
      },
    })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })

    render(<Billing />)

   expect(await screen.findByText('Storefront')).toBeInTheDocument()
   expect(screen.getByText('Current period: Aug 1, 2026 to Sep 1, 2026')).toBeInTheDocument()
   expect(screen.getByText('Billing period ends on Sep 1, 2026')).toBeInTheDocument()
   expect(screen.queryByText('Renews on Sep 1, 2026')).not.toBeInTheDocument()
    expect(screen.getByRole('switch')).toBeInTheDocument()
    expect(screen.getByRole('switch')).not.toBeChecked()
  })

  it('reuses the idempotency key when an ambiguous checkout attempt fails', async () => {
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })
    ;(billingAPI.createTopup as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('offline'))

    render(<Billing />)

    await waitFor(() => screen.getByText(/^100$/))

    const packageButton = screen.getByText(/^100$/).closest('button') as HTMLButtonElement
    await act(async () => fireEvent.click(packageButton))

    const proceedButton = await screen.findByRole('button', { name: /Proceed to Payment/i })
    await act(async () => fireEvent.click(proceedButton))

    await waitFor(() => expect(billingAPI.createTopup).toHaveBeenCalledTimes(1))

    const firstKey = (billingAPI.createTopup as ReturnType<typeof vi.fn>).mock.calls[0][1]
    expect(firstKey).toMatch(/^[0-9a-f]{32}$/)

    // Attempting again in the confirmation modal
    await act(async () => fireEvent.click(proceedButton))

    await waitFor(() => expect(billingAPI.createTopup).toHaveBeenCalledTimes(2))

    const secondKey = (billingAPI.createTopup as ReturnType<typeof vi.fn>).mock.calls[1][1]
    expect(secondKey).toBe(firstKey)
  })

  it('toggles auto-renew and refreshes the overview', async () => {
    ;(billingAPI.overview as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({
        data: {
          ...mockOverview,
          resources: [
            {
              resource_id: 7,
              resource_type: 'project',
              resource_name: 'Storefront',
              spec_name: 'Starter',
              monthly_credits: 75,
              status: 'active',
              current_period_start: '2026-08-01T00:00:00Z',
              next_invoice_at: '2026-09-01T00:00:00Z',
              auto_renew: false,
            },
          ],
        },
      })
      .mockResolvedValueOnce({
        data: {
          ...mockOverview,
          resources: [
            {
              resource_id: 7,
              resource_type: 'project',
              resource_name: 'Storefront',
              spec_name: 'Starter',
              monthly_credits: 75,
              status: 'active',
              current_period_start: '2026-08-01T00:00:00Z',
              next_invoice_at: '2026-09-01T00:00:00Z',
              auto_renew: true,
            },
          ],
        },
      })
    ;(billingAPI.updateAutoRenew as ReturnType<typeof vi.fn>).mockResolvedValue({})
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })

    render(<Billing />)

   const toggle = await screen.findByRole('switch')
   expect(toggle).not.toBeChecked()

   await act(async () => fireEvent.click(toggle))
    const confirmButton = await screen.findByRole('button', { name: 'Confirm' })
    await act(async () => fireEvent.click(confirmButton))

   await waitFor(() => expect(billingAPI.updateAutoRenew).toHaveBeenCalledWith(7, 'project', true))
   await waitFor(() => expect(toggle).toBeChecked())
 })

  it('blocks topup when billing profile is incomplete', async () => {
    ;(billingAPI.getProfile as ReturnType<typeof vi.fn>).mockResolvedValue({ data: { company_name: '', email: '' } })
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })

    render(<Billing />)

    await waitFor(() => screen.getByText(/^100$/))

    const packageButton = screen.getByText(/^100$/).closest('button') as HTMLButtonElement
    await act(async () => fireEvent.click(packageButton))

    expect(billingAPI.createTopup).not.toHaveBeenCalled()
  })

  it('scrolls the main container, not the shell, when a top-up is blocked', async () => {
    ;(billingAPI.getProfile as ReturnType<typeof vi.fn>).mockResolvedValue({ data: { company_name: '', email: '' } })
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })

    // The dashboard shell wraps <main> in overflow-hidden boxes. scrollIntoView
    // would scroll those too and drag the sidebar/header out of view.
    const scrollIntoView = vi.fn()
    Element.prototype.scrollIntoView = scrollIntoView
    const main = document.createElement('main')
    main.id = 'main-content'
    const scrollTo = vi.fn()
    main.scrollTo = scrollTo as unknown as typeof main.scrollTo
    document.body.appendChild(main)

    render(<Billing />)
    await waitFor(() => screen.getByText(/^100$/))

    const packageButton = screen.getByText(/^100$/).closest('button') as HTMLButtonElement
    await act(async () => fireEvent.click(packageButton))

    expect(scrollTo).toHaveBeenCalled()
    expect(scrollIntoView).not.toHaveBeenCalled()
    main.remove()
  })

  it('validates phone numbers correctly for Indonesian and international formats', () => {
    expect(isValidPhoneNumber('081234567890', 'ID')).toBe(true)
    expect(isValidPhoneNumber('+6281234567890', 'ID')).toBe(true)
    expect(isValidPhoneNumber('6281234567890', 'ID')).toBe(true)
    expect(isValidPhoneNumber('81234567890', 'ID')).toBe(true)
    expect(isValidPhoneNumber('123', 'ID')).toBe(false)
  })

  it('renders valid user project name containing hash without masking and falls back to localized unnamed service when empty', async () => {
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: {
        ...mockOverview,
        resources: [
          {
            resource_id: 42,
            resource_type: 'project',
            resource_name: 'Project #42',
            spec_name: 'Starter',
            monthly_credits: 75,
            status: 'active',
            current_period_start: '2026-08-01T00:00:00Z',
            next_invoice_at: '2026-09-01T00:00:00Z',
            auto_renew: false,
          },
          {
            resource_id: 99,
            resource_type: 'database',
            resource_name: '',
            spec_name: 'Starter DB',
            monthly_credits: 100,
            status: 'active',
            current_period_start: '2026-08-01T00:00:00Z',
            next_invoice_at: '2026-09-01T00:00:00Z',
            auto_renew: false,
          },
        ],
      },
    })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })

    render(<Billing />)

    // Explicit name with hash must be displayed intact
    expect(await screen.findByText('Project #42')).toBeInTheDocument()
    // Empty resource name must fall back to localized unnamed service
    expect(await screen.findByText('Database Service')).toBeInTheDocument()
  })

  it('opens top-up confirmation modal when a package is clicked and allows cancelling', async () => {
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })

    render(<Billing />)

    await waitFor(() => screen.getByText(/^100$/))

    const packageButton = screen.getByText(/^100$/).closest('button') as HTMLButtonElement
    await act(async () => fireEvent.click(packageButton))

    expect(await screen.findByText('Confirm Credit Purchase')).toBeInTheDocument()
    expect(screen.getByText('+100')).toBeInTheDocument()
    expect(screen.getByText('Fixed Package')).toBeInTheDocument()

    const cancelButton = screen.getByRole('button', { name: 'Cancel' })
    await act(async () => fireEvent.click(cancelButton))

    expect(billingAPI.createTopup).not.toHaveBeenCalled()
  })

  it('opens top-up confirmation modal for custom amount input and proceeds with topup', async () => {
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })
    ;(billingAPI.createTopup as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { id: 99, payment_url: 'https://checkout.pakasir.com/pay/test' },
    })

    render(<Billing />)

    const input = await screen.findByLabelText(/Enter amount \(IDR\)/i)
    await act(async () => {
      fireEvent.change(input, { target: { value: '50000' } })
    })

    const topupButton = screen.getByRole('button', { name: 'Top up' })
    await act(async () => fireEvent.click(topupButton))

    expect(await screen.findByText('Confirm Credit Purchase')).toBeInTheDocument()
    expect(screen.getByText('+50')).toBeInTheDocument()
    expect(screen.getByText('Custom Amount')).toBeInTheDocument()

    const proceedButton = screen.getByRole('button', { name: /Proceed to Payment/i })
    await act(async () => fireEvent.click(proceedButton))

    await waitFor(() => expect(billingAPI.createTopup).toHaveBeenCalledWith(0, expect.any(String), 50000))
  })

  it('derives custom credit conversion accurately when active package rate is not Rp 1,000', async () => {
    const customRateCatalog = {
      specs: [],
      packages: [
        { id: 10, credits: 50, amount_minor: 100000, currency: 'IDR', sort_order: 1 }, // Rp 2,000 per credit
      ],
    }
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: customRateCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })
    ;(billingAPI.createTopup as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { id: 101, payment_url: 'https://checkout.pakasir.com/pay/test' },
    })

    render(<Billing />)

    const input = await screen.findByLabelText(/Enter amount \(IDR\)/i)
    await act(async () => {
      fireEvent.change(input, { target: { value: '50000' } }) // 50,000 / 2,000 = 25 credits
    })

    const topupButton = screen.getByRole('button', { name: 'Top up' })
    await act(async () => fireEvent.click(topupButton))

    expect(await screen.findByText('Confirm Credit Purchase')).toBeInTheDocument()
    // At Rp 2,000/credit, Rp 50,000 gives 25 credits
    expect(screen.getByText('+25')).toBeInTheDocument()
    expect(screen.getByText('Custom Amount')).toBeInTheDocument()
    // Wallet progression: 500 + 25 = 525
    expect(screen.getByText('525 credits')).toBeInTheDocument()

    const proceedButton = screen.getByRole('button', { name: /Proceed to Payment/i })
    await act(async () => fireEvent.click(proceedButton))

    await waitFor(() => expect(billingAPI.createTopup).toHaveBeenCalledWith(0, expect.any(String), 50000))
  })

  it('enables Rp 10,000 and confirms 6 credits with active Rp 1,500 rate, and keeps Rp 10,500 disabled', async () => {
    const rate1500Catalog = {
      specs: [],
      packages: [
        { id: 11, credits: 10, amount_minor: 15000, currency: 'IDR', sort_order: 1 }, // 15,000 / 10 = Rp 1,500 per credit
      ],
    }
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: rate1500Catalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })
    ;(billingAPI.createTopup as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { id: 103, payment_url: 'https://checkout.pakasir.com/pay/test' },
    })

    render(<Billing />)

    const input = await screen.findByLabelText(/Enter amount \(IDR\)/i)
    const topupButton = screen.getByRole('button', { name: 'Top up' })

    // 10,500 is not a multiple of 1,000 -> Top up button must be disabled
    await act(async () => {
      fireEvent.change(input, { target: { value: '10500' } })
    })
    expect(topupButton).toBeDisabled()

    // 10,000 is a multiple of 1,000 -> Top up button must be enabled
    await act(async () => {
      fireEvent.change(input, { target: { value: '10000' } })
    })
    expect(topupButton).not.toBeDisabled()

    await act(async () => fireEvent.click(topupButton))

    expect(await screen.findByText('Confirm Credit Purchase')).toBeInTheDocument()
    // 10,000 / 1,500 = floor(6.666...) = 6 credits
    expect(screen.getByText('+6')).toBeInTheDocument()
    expect(screen.getByText('Custom Amount')).toBeInTheDocument()
    // Wallet balance: 500 + 6 = 506 credits
    expect(screen.getByText('506 credits')).toBeInTheDocument()

    const proceedButton = screen.getByRole('button', { name: /Proceed to Payment/i })
    await act(async () => fireEvent.click(proceedButton))

    await waitFor(() => expect(billingAPI.createTopup).toHaveBeenCalledWith(0, expect.any(String), 10000))
  })

  it('confirms top-up without making unsupported zero-fee guarantees before checkout', async () => {
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })
    ;(billingAPI.createTopup as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: {
        id: 102,
        credits: 100,
        amount_minor: 100000,
        currency: 'IDR',
        payment_token: '00020101021226590014ID.LINKAJA.WWW0118936009180000000000020300051450001083100012345678905204581253033605802ID5911LARAVELPAAS6007JAKARTA61051219062070703A016304ABCD',
        status: 'pending',
      },
    })

    render(<Billing />)

    await waitFor(() => screen.getByText(/^100$/))

    const packageButton = screen.getByText(/^100$/).closest('button') as HTMLButtonElement
    await act(async () => fireEvent.click(packageButton))

    // Ensure pre-checkout confirmation does NOT promise Rp 0 fee or exact total
    expect(screen.queryByText(/Rp 0 \(Free\)/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Total Payment/i)).not.toBeInTheDocument()
    expect(screen.getByText(/Credit Price/i)).toBeInTheDocument()
    expect(screen.getByText(/Payment method & provider fees/i)).toBeInTheDocument()

    const proceedButton = screen.getByRole('button', { name: /Proceed to Payment/i })
    await act(async () => fireEvent.click(proceedButton))

    // QRIS Payment Modal should open
    expect(await screen.findByText('Top-up Payment')).toBeInTheDocument()
  })

  it('shows oldest_due_at for suspended resource and never shows future next_invoice_at as overdue', async () => {
    // Suspended since Aug 19; next_invoice_at is Sep 19 (future) — must NOT appear as overdue
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: {
        ...mockOverview,
        resources: [
          {
            resource_id: 5,
            resource_type: 'project',
            resource_name: 'SuspendedApp',
            spec_name: 'Starter',
            monthly_credits: 75,
            status: 'suspended',
            current_period_start: '2026-08-01T00:00:00Z',
            next_invoice_at: '2026-09-19T00:00:00Z', // future date — must NOT appear as due date
            payment_due_period_start: '2026-08-19T00:00:00Z',
            payment_due_period_end: '2026-09-19T00:00:00Z',
            auto_renew: true,
          },
        ],
      },
    })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: [
        {
          resource_id: 5,
          resource_type: 'project',
          status: 'suspended',
          oldest_due_at: '2026-08-19T00:00:00Z', // this is the real overdue date
          payment_due_days: 6,
        },
      ],
    })

    render(<Billing />)

    await screen.findByText('SuspendedApp')
    // oldest_due_at (Aug 19) should appear as the overdue date
    expect(screen.getByText('Renewal payment due since Aug 19, 2026')).toBeInTheDocument()
    expect(screen.getByText('Unpaid period: Aug 19, 2026 to Sep 19, 2026')).toBeInTheDocument()
    expect(screen.queryByText('Current period: Aug 1, 2026 to Sep 19, 2026')).not.toBeInTheDocument()
    // future next_invoice_at (Sep 19) must NOT appear as the overdue/payment-due date label
    // (it can still appear in the currentPeriod row as the period end, which is fine)
    expect(screen.queryByText('Renewal payment due since Sep 19, 2026')).not.toBeInTheDocument()
    expect(screen.queryByText('Renews on Sep 19, 2026')).not.toBeInTheDocument()

    await act(async () => fireEvent.click(screen.getByRole('button', { name: 'Pay now' })))
    await waitFor(() => expect(billingAPI.payDueResource).toHaveBeenCalledWith(5, 'project'))
  })

  it('shows neutral payment required copy when oldest_due_at is absent for suspended resource', async () => {
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: {
        ...mockOverview,
        resources: [
          {
            resource_id: 6,
            resource_type: 'database',
            resource_name: 'MainDB',
            spec_name: 'Starter DB',
            monthly_credits: 50,
            status: 'payment_due',
            current_period_start: '2026-08-01T00:00:00Z',
            next_invoice_at: '2026-09-01T00:00:00Z',
            auto_renew: false,
          },
        ],
      },
    })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    // Status entry has no oldest_due_at
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: [
        {
          resource_id: 6,
          resource_type: 'database',
          status: 'payment_due',
          payment_due_days: 0,
        },
      ],
    })

    render(<Billing />)

    await screen.findByText('MainDB')
    // Should show neutral copy, not a fabricated date
    const paymentRequiredElements = screen.getAllByText('Payment required')
    expect(paymentRequiredElements.length).toBeGreaterThan(0)
    // Must not show the future next_invoice_at as a due-date label
    expect(screen.queryByText('Renews on Sep 1, 2026')).not.toBeInTheDocument()
    expect(screen.queryByText(/Renewal payment due since Sep 1, 2026/)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Pay now' })).toBeInTheDocument()
  })

  it('does not render any credits*1000 IDR equivalent in invoice receipt dialog', async () => {
    const invoiceWithItems = {
      ...mockOverview,
      invoices: [
        {
          id: 1,
          period_start: '2026-07-01T00:00:00Z',
          period_end: '2026-08-01T00:00:00Z',
          total_credits: 250,
          status: 'paid',
          paid_at: '2026-08-01T00:00:00Z',
          created_at: '2026-07-01T00:00:00Z',
          items: [
            {
              id: 1,
              billable_resource_id: 1,
              resource_type: 'project' as const,
              resource_name: 'StorefrontInvoiceItem',
              spec_id: 1,
              spec_name: 'Starter',
              description: 'Monthly billing',
              credits: 250,
            },
          ],
        },
      ],
    }
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: invoiceWithItems })
    // Use a non-Rp1000 rate catalog to ensure IDR is not computed from credits
    const nonStandardCatalog = {
      specs: [],
      packages: [{ id: 5, credits: 10, amount_minor: 15000, currency: 'IDR', sort_order: 1 }],
    }
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: nonStandardCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })

    render(<Billing />)

    // Wait for the page to finish loading (balance should show)
    await waitFor(() => screen.getByText(/500/))

    // Navigate to Invoices tab to expose the table
    const invoicesTab = screen.getByRole('tab', { name: /Invoices/i })
    await act(async () => fireEvent.click(invoicesTab))

    // Click View Receipt button for the invoice
    const viewReceiptBtn = await screen.findByRole('button', { name: /View Invoice/i })
    await act(async () => fireEvent.click(viewReceiptBtn))

    // The invoice dialog must not show any Rp IDR value derived from credits
    // 250 * 1000 = 250,000; 250 * 1500 = 375,000 — neither should appear
    expect(screen.queryByText(/Rp 250,000/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/Rp 375,000/i)).not.toBeInTheDocument()
    expect(screen.queryByText(/≈ Rp/)).not.toBeInTheDocument()
  })

  it('polls reconcile every 5 seconds when payment modal is open and cleans up on close', async () => {
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })
    ;(billingAPI.createTopup as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: {
        id: 55,
        credits: 100,
        amount_minor: 100000,
        currency: 'IDR',
        status: 'pending',
        payment_token: '00020101021226590014ID.LINKAJA.WWW0118936009180000000000020300051450001083100012345678905204581253033605802ID5911LARAVELPAAS6007JAKARTA61051219062070703A016304ABCD',
      },
    })
    // reconcileTopup returns pending each time (modal should stay open)
    ;(billingAPI.reconcileTopup as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { id: 55, status: 'pending' },
    })
    // overview called after reconcile still returns pending top-up
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: {
        ...mockOverview,
        topups: [{ id: 55, credits: 100, amount_minor: 100000, currency: 'IDR', status: 'pending', created_at: '2026-08-25T00:00:00Z' }],
      },
    })

    render(<Billing />)

    // Open payment modal via QRIS top-up
    await waitFor(() => screen.getByText(/^100$/))
    const packageButton = screen.getByText(/^100$/).closest('button') as HTMLButtonElement
    await act(async () => fireEvent.click(packageButton))
    const proceedButton = await screen.findByRole('button', { name: /Proceed to Payment/i })
    await act(async () => fireEvent.click(proceedButton))

    // Modal should be open
    expect(await screen.findByText('Top-up Payment')).toBeInTheDocument()

    const reconcileCallsBefore = (billingAPI.reconcileTopup as ReturnType<typeof vi.fn>).mock.calls.length

    // Advance 5 seconds → first poll tick
    await act(async () => { vi.advanceTimersByTime(5_000) })
    await waitFor(() =>
      expect((billingAPI.reconcileTopup as ReturnType<typeof vi.fn>).mock.calls.length).toBeGreaterThan(reconcileCallsBefore),
    )

    // Advance another 5 seconds → second poll tick
    const callsAfterFirst = (billingAPI.reconcileTopup as ReturnType<typeof vi.fn>).mock.calls.length
    await act(async () => { vi.advanceTimersByTime(5_000) })
    await waitFor(() =>
      expect((billingAPI.reconcileTopup as ReturnType<typeof vi.fn>).mock.calls.length).toBeGreaterThan(callsAfterFirst),
    )

    // Close modal — polling must stop. Dialog close button has aria-label="Close" from Radix.
    const closeBtns = screen.getAllByRole('button')
    const closeBtn = closeBtns.find((b) => /close/i.test(b.getAttribute('aria-label') ?? ''))
    if (closeBtn) {
      await act(async () => fireEvent.click(closeBtn))
      const callsAfterClose = (billingAPI.reconcileTopup as ReturnType<typeof vi.fn>).mock.calls.length
      await act(async () => { vi.advanceTimersByTime(10_000) })
      expect((billingAPI.reconcileTopup as ReturnType<typeof vi.fn>).mock.calls.length).toBe(callsAfterClose)
    }
  })

  it('retries billing profile load when first attempt fails with 5xx', async () => {
    // First call: network error (5xx simulation)
    ;(billingAPI.getProfile as ReturnType<typeof vi.fn>)
      .mockRejectedValueOnce(new Error('Server error'))
      .mockResolvedValueOnce({ data: mockProfile })

    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })

    render(<Billing />)
    // First load: getProfile fails — profile should stay empty
    await waitFor(() => expect(billingAPI.getProfile).toHaveBeenCalledTimes(1))
    const refreshButton = screen.getByRole('button', { name: /Refresh/i })
    await waitFor(() => expect(refreshButton).not.toBeDisabled())
    expect(screen.queryByDisplayValue('PT Acme Corp')).not.toBeInTheDocument()

    // Simulate a retry by clicking the Refresh button, which calls load() again.
    // hasLoadedProfileRef.current stays false after the first error, so getProfile
    // will be called again on the next load() invocation.
    await act(async () => fireEvent.click(refreshButton))

    await waitFor(() => expect(billingAPI.getProfile).toHaveBeenCalledTimes(2))
    // After successful retry, the profile should populate
    expect(await screen.findByDisplayValue('PT Acme Corp')).toBeInTheDocument()
  })

  it('stops modal polling immediately on terminal failure status like failed or expired', async () => {
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })
    ;(billingAPI.createTopup as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: {
        id: 88,
        credits: 100,
        amount_minor: 100000,
        currency: 'IDR',
        status: 'pending',
        payment_token: '00020101021226590014ID.LINKAJA.WWW0118936009180000000000020300051450001083100012345678905204581253033605802ID5911LARAVELPAAS6007JAKARTA61051219062070703A016304ABCD',
      },
    })
    // First poll returns failed status (terminal)
    ;(billingAPI.reconcileTopup as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { id: 88, status: 'failed' },
    })

    render(<Billing />)
    await waitFor(() => screen.getByText(/^100$/))
    const packageButton = screen.getByText(/^100$/).closest('button') as HTMLButtonElement
    await act(async () => fireEvent.click(packageButton))
    const proceedButton = await screen.findByRole('button', { name: /Proceed to Payment/i })
    await act(async () => fireEvent.click(proceedButton))

    expect(await screen.findByText('Top-up Payment')).toBeInTheDocument()

    // Advance timer 5 seconds to trigger poll tick
    await act(async () => { vi.advanceTimersByTime(5_000) })

    // Modal should close upon terminal failure
    await waitFor(() => expect(screen.queryByText('Top-up Payment')).not.toBeInTheDocument())

    const callsAfterTerminal = (billingAPI.reconcileTopup as ReturnType<typeof vi.fn>).mock.calls.length

    // Further timer advancement must NOT trigger any more reconcile requests
    await act(async () => { vi.advanceTimersByTime(15_000) })
    expect((billingAPI.reconcileTopup as ReturnType<typeof vi.fn>).mock.calls.length).toBe(callsAfterTerminal)
  })

  it('stops modal polling immediately on terminal status refunded or chargeback', async () => {
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })
    ;(billingAPI.createTopup as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: {
        id: 89,
        credits: 100,
        amount_minor: 100000,
        currency: 'IDR',
        status: 'pending',
        payment_token: '00020101021226590014ID.LINKAJA.WWW0118936009180000000000020300051450001083100012345678905204581253033605802ID5911LARAVELPAAS6007JAKARTA61051219062070703A016304ABCD',
      },
    })
    ;(billingAPI.reconcileTopup as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: { id: 89, status: 'refunded' },
    })

    render(<Billing />)
    await waitFor(() => screen.getByText(/^100$/))
    const packageButton = screen.getByText(/^100$/).closest('button') as HTMLButtonElement
    await act(async () => fireEvent.click(packageButton))
    const proceedButton = await screen.findByRole('button', { name: /Proceed to Payment/i })
    await act(async () => fireEvent.click(proceedButton))

    expect(await screen.findByText('Top-up Payment')).toBeInTheDocument()

    await act(async () => { vi.advanceTimersByTime(5_000) })
    await waitFor(() => expect(screen.queryByText('Top-up Payment')).not.toBeInTheDocument())

    const callsAfterTerminal = (billingAPI.reconcileTopup as ReturnType<typeof vi.fn>).mock.calls.length
    await act(async () => { vi.advanceTimersByTime(15_000) })
    expect((billingAPI.reconcileTopup as ReturnType<typeof vi.fn>).mock.calls.length).toBe(callsAfterTerminal)
  })

  it('isolates stale async response so an earlier top-up response does not close a newer modal', async () => {
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })

    // Top-up A (id=101) deferred promise
    let resolveReconcileA: ((value: any) => void) | null = null
    const reconcileAPromise = new Promise((resolve) => {
      resolveReconcileA = resolve
    })

    ;(billingAPI.createTopup as ReturnType<typeof vi.fn>)
      .mockResolvedValueOnce({
        data: {
          id: 101,
          credits: 100,
          amount_minor: 100000,
          currency: 'IDR',
          status: 'pending',
          payment_token: '00020101021226590014ID.LINKAJA.WWW0118936009180000000000020300051450001083100012345678905204581253033605802ID5911LARAVELPAAS6007JAKARTA61051219062070703A016304ABCD',
        },
      })
      .mockResolvedValueOnce({
        data: {
          id: 102,
          credits: 250,
          amount_minor: 225000,
          currency: 'IDR',
          status: 'pending',
          payment_token: '00020101021226590014ID.LINKAJA.WWW0118936009180000000000020300051450001083100012345678905204581253033605802ID5911LARAVELPAAS6007JAKARTA61051219062070703A016304ABCD',
        },
      })

    ;(billingAPI.reconcileTopup as ReturnType<typeof vi.fn>).mockImplementation((id: number) => {
      if (id === 101) return reconcileAPromise
      return Promise.resolve({ data: { id: 102, status: 'pending' } })
    })

    render(<Billing />)

    // Open Top-up A
    await waitFor(() => screen.getByText(/^100$/))
    const package100 = screen.getByText(/^100$/).closest('button') as HTMLButtonElement
    await act(async () => fireEvent.click(package100))
    const proceed1 = await screen.findByRole('button', { name: /Proceed to Payment/i })
    await act(async () => fireEvent.click(proceed1))

    expect(await screen.findByText('Top-up Payment')).toBeInTheDocument()

    // Trigger poll for A (reconcile for 101 is now in-flight)
    await act(async () => { vi.advanceTimersByTime(5_000) })
    expect(billingAPI.reconcileTopup).toHaveBeenCalledWith(101)

    // Close modal A by clicking close button (which has text 'Close')
    const closeBtn = screen.getByRole('button', { name: /close/i })
    await act(async () => fireEvent.click(closeBtn))
    await waitFor(() => expect(screen.queryByText('Top-up Payment')).not.toBeInTheDocument())

    // Now open Top-up B (id=102)
    const package250 = screen.getByText(/^250$/).closest('button') as HTMLButtonElement
    await act(async () => fireEvent.click(package250))
    const proceed2 = await screen.findByRole('button', { name: /Proceed to Payment/i })
    await act(async () => fireEvent.click(proceed2))

    expect(await screen.findByText('Top-up Payment')).toBeInTheDocument()

    // Now resolve Top-up A's deferred promise with 'paid'
    await act(async () => {
      resolveReconcileA!({ data: { id: 101, status: 'paid' } })
    })

    // Modal B (id=102) must still remain open! It should NOT be closed by stale response A
    expect(screen.getByText('Top-up Payment')).toBeInTheDocument()
  })

  it('preserves user dirty profile form inputs across background retries when initial fetch fails', async () => {
    // Initial fetch fails with 5xx
    ;(billingAPI.getProfile as ReturnType<typeof vi.fn>)
      .mockRejectedValueOnce(new Error('500 Internal Server Error'))
      .mockResolvedValue({
        data: {
          company_name: 'Server Side Corp',
          tax_id: '99.999.999.9-999.000',
          email: 'server@corp.com',
          phone: '+628999999999',
          address_line1: 'Server Address 123',
          city: 'Bandung',
          postal_code: '40111',
          country: 'ID',
        },
      })

    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })

    render(<Billing />)

    await waitFor(() => expect(billingAPI.getProfile).toHaveBeenCalledTimes(1))
    const refreshBtn = screen.getByRole('button', { name: /Refresh/i })
    await waitFor(() => expect(refreshBtn).not.toBeDisabled())

    // User types into the company name field (marking form dirty)
    const nameInput = screen.getByLabelText(/Full Name \/ Company Name/i)
    await act(async () => {
      fireEvent.change(nameInput, { target: { value: 'My In-Progress Edit' } })
    })
    expect(nameInput).toHaveValue('My In-Progress Edit')

    // Simulate background retry via Refresh button
    await act(async () => fireEvent.click(refreshBtn))
    await waitFor(() => expect(billingAPI.getProfile).toHaveBeenCalledTimes(2))

    // Form input MUST NOT be overwritten by the server's 'Server Side Corp' response
    expect(nameInput).toHaveValue('My In-Progress Edit')
    expect(screen.queryByDisplayValue('Server Side Corp')).not.toBeInTheDocument()
  })

  it('displays authoritative suspended status from billing status even when overview status is stale active', async () => {
    // Overview has stale status 'active'
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: {
        ...mockOverview,
        resources: [
          {
            resource_id: 10,
            resource_type: 'project',
            resource_name: 'StaleActiveApp',
            spec_name: 'Starter',
            monthly_credits: 75,
            status: 'active', // stale overview snapshot
            current_period_start: '2026-08-01T00:00:00Z',
            next_invoice_at: '2026-09-19T00:00:00Z',
            auto_renew: true,
          },
        ],
      },
    })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    // Status endpoint has fresh authoritative status 'suspended'
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({
      data: [
        {
          resource_id: 10,
          resource_type: 'project',
          status: 'suspended',
          oldest_due_at: '2026-08-19T00:00:00Z',
          payment_due_days: 6,
        },
      ],
    })

    render(<Billing />)

    await screen.findByText('StaleActiveApp')

    // The card must show effective suspended status and overdue date from /billing/status
    expect(screen.getByText('Renewal payment due since Aug 19, 2026')).toBeInTheDocument()
    expect(screen.queryByText('Renews on Sep 19, 2026')).not.toBeInTheDocument()
  })

  it('allows user to resume and pay for a pending top-up directly from top-up history table', async () => {
    const overviewWithPendingTopup = {
      ...mockOverview,
      topups: [
        {
          id: 52,
          credits: 10,
          amount_minor: 10000,
          currency: 'IDR',
          status: 'pending',
          payment_token: '00020101021226590014ID.LINKAJA.WWW0118936009180000000000020300051450001083100012345678905204581253033605802ID5911LARAVELPAAS6007JAKARTA61051219062070703A016304ABCD',
          payment_url: 'https://app.pakasir.com/pay/52',
          created_at: '2026-08-25T00:00:00Z',
        },
      ],
    }

    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: overviewWithPendingTopup })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })

    render(<Billing />)

    // Switch to Top-ups tab
    const topupsTab = await screen.findByRole('tab', { name: /Top-ups/i })
    await act(async () => fireEvent.click(topupsTab))

    // #topup-52 should be listed
    expect(await screen.findByText('#topup-52')).toBeInTheDocument()

    // Pay button should be visible for pending topup with payment token/URL
    const payButton = screen.getByRole('button', { name: /^Pay$/i })
    expect(payButton).toBeInTheDocument()

    // Clicking Pay should open the PaymentDialog modal
    await act(async () => fireEvent.click(payButton))

    expect(await screen.findByText('Top-up Payment')).toBeInTheDocument()
  })
})
