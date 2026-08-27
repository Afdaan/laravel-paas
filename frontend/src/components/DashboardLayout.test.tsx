import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react'
import { act } from 'react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import DashboardLayout from './DashboardLayout'
import { billingAPI } from '../services/api'

vi.mock('@/lib/usePolling', () => ({ usePolling: vi.fn() }))

vi.mock('@/lib/useTranslation', () => ({
  default: () => ({ t: (key: string) => key, language: 'en' }),
}))

vi.mock('@/components/ThemeProvider', () => ({
  useTheme: () => ({ theme: 'light', setTheme: vi.fn() }),
}))

vi.mock('@/stores/authStore', () => ({
  default: () => ({
    user: { name: 'A User', email: 'user@example.com', role: 'user' },
    logout: vi.fn(),
    adminToken: null,
    returnToAdmin: vi.fn(),
  }),
}))

describe('DashboardLayout mobile drawer', () => {
  let mobileMatches = false

  beforeEach(() => {
    mobileMatches = true
    vi.stubGlobal(
      'localStorage',
      { getItem: vi.fn(), setItem: vi.fn() },
    )
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: query.includes('max-width: 767px') ? mobileMatches : false,
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    })
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
    vi.clearAllMocks()
  })

  const renderLayout = () =>
    render(
      <MemoryRouter initialEntries={['/dashboard']}>
        <Routes>
          <Route path="/" element={<DashboardLayout />}>
            <Route path="dashboard" element={<div data-testid="outlet">Outlet</div>} />
          </Route>
        </Routes>
      </MemoryRouter>,
    )

  it('keeps sidebar inert when closed and toggles inert on open/close', async () => {
    renderLayout()

    const sidebar = document.querySelector('[role="dialog"]')?.parentElement as HTMLElement
    const main = document.getElementById('main-content') as HTMLElement
    const openButton = screen.getByLabelText(/openNavigation/i)

    expect(sidebar).toHaveAttribute('inert')
    expect(main).not.toHaveAttribute('inert')

    await act(async () => fireEvent.click(openButton))

    expect(sidebar).not.toHaveAttribute('inert')
    expect(main).toHaveAttribute('inert')

    await act(async () => fireEvent.keyDown(document, { key: 'Escape' }))

    expect(sidebar).toHaveAttribute('inert')
    expect(main).not.toHaveAttribute('inert')
  })

  it('traps focus and restores focus on close', async () => {
    renderLayout()

    const openButton = screen.getByLabelText(/openNavigation/i)
    await act(async () => openButton.focus())
    await act(async () => fireEvent.click(openButton))

    const aside = document.getElementById('mobile-navigation') as HTMLElement
    const focusable = Array.from(
      aside.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), input, select, textarea, [tabindex]:not([tabindex="-1"])',
      ),
    )
    expect(focusable.length).toBeGreaterThan(1)
    const [first, last] = [focusable[0], focusable[focusable.length - 1]]

    await waitFor(() => expect(document.activeElement).toBe(first))

    await act(async () => last.focus())
    await act(async () => fireEvent.keyDown(document, { key: 'Tab' }))

    expect(document.activeElement).toBe(first)

    await act(async () => fireEvent.keyDown(document, { key: 'Escape' }))

    expect(document.activeElement).toBe(openButton)
  })

  it('shows payment-due resources in the global header', async () => {
    vi.spyOn(billingAPI, 'overview').mockResolvedValue({
      data: { wallet: { balance_credits: 500 } },
    } as never)
    vi.spyOn(billingAPI, 'status').mockResolvedValue({
      data: [
        { resource_id: 1, resource_type: 'project', status: 'payment_due', payment_due_days: 1 },
        { resource_id: 2, resource_type: 'database', status: 'suspended', payment_due_days: 8 },
      ],
    } as never)

    renderLayout()

    expect(await screen.findByRole('button', { name: 'billing.reviewOverdueBilling' })).toHaveTextContent(
      'billing.paymentDueCount',
    )
  })
})
