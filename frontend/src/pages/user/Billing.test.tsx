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
        'billing.refresh': 'Refresh',
        'billing.paymentRequired': 'Payment required',
        'billing.paymentRequiredDescription': 'desc',
        'billing.dueOn': 'Due on {{date}}',
        'billing.lowBalance': 'Low balance',
        'billing.lowBalanceDescription': 'Low balance description',
        'billing.staleData': 'Showing previously loaded data. Refresh to update.',
        'billing.resourceTypes.project': 'project',
        'billing.resourceTypes.database': 'database',
        'billing.resourceBilling': 'Resource billing',
        'billing.resourceBillingDescription': 'Resource billing description',
        'billing.noBillableResources': 'No active billable resources.',
        'billing.renewsOn': 'Renews on {{date}}',
        'billing.renewalPaymentDue': 'Renewal payment due since {{date}}',
       'billing.month': 'month',
       'billing.currentPeriod': 'Current period: {{start}} to {{end}}',
        'billing.autoRenew': 'Auto-renew',
        'billing.autoRenewEnabled': 'Auto-renew enabled',
        'billing.autoRenewDisabled': 'Auto-renew disabled',
        'billing.autoRenewFailed': 'Could not update auto-renew',
        'billing.autoRenewRateLimited': 'Too many auto-renew toggle requests. Please wait a moment before trying again.',
        'billing.confirmChange': 'Confirm',
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
    expect(screen.getAllByText(/500/, { exact: false })[0]).toBeInTheDocument()

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

  it('reconciles a Pakasir top-up after payment redirect', async () => {
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

    await waitFor(() => expect(billingAPI.createTopup).toHaveBeenCalledTimes(1))

    const firstKey = (billingAPI.createTopup as ReturnType<typeof vi.fn>).mock.calls[0][1]
    expect(firstKey).toMatch(/^[0-9a-f]{32}$/)

    await act(async () => fireEvent.click(packageButton))

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
})
