import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, fireEvent, cleanup } from '@testing-library/react'
import { act } from 'react'
import Billing, { isValidPhoneNumber } from './Billing'
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
        'billing.renewalPaymentDue': 'Renewal payment due since {{date}}',
       'billing.month': 'month',
       'billing.currentPeriod': 'Current period: {{start}} to {{end}}',
        'billing.autoRenew': 'Auto-renew',
        'billing.autoRenewEnabled': 'Auto-renew enabled',
        'billing.autoRenewRateLimited': 'Too many auto-renew toggle requests. Please wait a moment before trying again.',
        'billing.confirmChange': 'Confirm',
        'billing.confirmTopupTitle': 'Confirm Credit Purchase',
        'billing.confirmTopupDescription': 'Please review your top-up details before proceeding to the payment gateway.',
        'billing.confirmTopupBadge': 'Top-Up Confirmation',
        'billing.confirmPackageType': 'Package Type',
        'billing.fixedPackage': 'Fixed Package',
        'billing.customPackage': 'Custom Amount',
        'billing.creditsToAdd': 'Credits to Add',
        'billing.packagePrice': 'Credit Price',
        'billing.currentBalance': 'Current balance',
        'billing.estimatedBalanceAfter': 'Balance after top-up',
        'billing.balanceProgression': 'Wallet Balance',
        'billing.conversionRate': 'Conversion rate',
        'billing.conversionRateValue': '1 Credit = {{rate}}',
        'billing.checkoutNotice': 'Payment method & provider fees (if applicable) are finalized at checkout.',
        'billing.proceedToPayment': 'Proceed to Payment',
        'billing.cancelTopup': 'Cancel',
        'billing.paymentSecurityTitle': 'Secure Provider Checkout',
        'billing.paymentSecurityNote': 'Payment security note',
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

  it('shows each resource renewal date', async () => {
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
   expect(screen.getByText('Renews on Sep 1, 2026')).toBeInTheDocument()
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
    expect(await screen.findByText('Pembayaran Top-up')).toBeInTheDocument()
  })
})
