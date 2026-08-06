import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, fireEvent, cleanup } from '@testing-library/react'
import { act } from 'react'
import Billing from './Billing'
import { billingAPI } from '@/services/api'

vi.mock('@/lib/usePolling', () => ({ usePolling: vi.fn() }))

vi.mock('@/services/api', () => ({
  billingAPI: {
    overview: vi.fn(),
    catalog: vi.fn(),
    status: vi.fn(),
    createTopup: vi.fn(),
    reconcileTopup: vi.fn(),
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
  upcoming_required_credits: 1000,
}

describe('Billing page', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
  })

  afterEach(() => {
    cleanup()
    vi.useRealTimers()
    vi.clearAllMocks()
    window.location.href = ''
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
    expect(screen.getByText(/500 credits/i)).toBeInTheDocument()

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

  it('reuses the idempotency key when an ambiguous checkout attempt fails', async () => {
    ;(billingAPI.overview as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockOverview })
    ;(billingAPI.catalog as ReturnType<typeof vi.fn>).mockResolvedValue({ data: mockCatalog })
    ;(billingAPI.status as ReturnType<typeof vi.fn>).mockResolvedValue({ data: [] })
    ;(billingAPI.createTopup as ReturnType<typeof vi.fn>).mockRejectedValue(new Error('offline'))

    render(<Billing />)

    await waitFor(() => screen.getByText(/100 credits/i))

    const packageButton = screen.getByText(/100 credits/i).closest('button') as HTMLButtonElement
    await act(async () => fireEvent.click(packageButton))

    await waitFor(() => expect(billingAPI.createTopup).toHaveBeenCalledTimes(1))

    const firstKey = (billingAPI.createTopup as ReturnType<typeof vi.fn>).mock.calls[0][1]
    expect(firstKey).toMatch(/^[0-9a-f]{32}$/)

    await act(async () => fireEvent.click(packageButton))

    await waitFor(() => expect(billingAPI.createTopup).toHaveBeenCalledTimes(2))

    const secondKey = (billingAPI.createTopup as ReturnType<typeof vi.fn>).mock.calls[1][1]
    expect(secondKey).toBe(firstKey)
  })
})
