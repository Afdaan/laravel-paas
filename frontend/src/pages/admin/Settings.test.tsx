import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { act } from 'react'
import AdminSettings from './Settings'
import { settingsAPI, billingAPI } from '@/services/api'
import useAuthStore from '@/stores/authStore'

vi.mock('@/services/api', () => ({
  settingsAPI: {
    list: vi.fn(),
    update: vi.fn(),
  },
  billingAPI: {
    updatePaymentProvider: vi.fn(),
  },
}))

const mockT = (key: string) => key

vi.mock('@/lib/useTranslation', () => ({
  default: () => ({
    t: mockT,
  }),
}))

describe('AdminSettings Payment Provider and Generic Save', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.setState({
      user: {
        id: 1,
        name: 'superadmin',
        email: 'superadmin@example.com',
        role: 'superadmin',
      },
    })
    vi.mocked(settingsAPI.list).mockResolvedValue({
      data: {
        map: {
          base_domain: 'example.com',
          project_domain: 'apps.example.com',
          admin_idle_timeout: 15,
          max_concurrent_builds: 3,
          default_payment_provider: 'pakasir',
        },
      },
    } as any)
    vi.mocked(settingsAPI.update).mockResolvedValue({ data: { message: 'success' } } as any)
    vi.mocked(billingAPI.updatePaymentProvider).mockResolvedValue({ data: { success: true } } as any)
  })

  afterEach(() => {
    vi.clearAllMocks()
  })

  it('excludes default_payment_provider, base_domain, and project_domain from generic settings save', async () => {
    render(<AdminSettings />)

    await waitFor(() => {
      expect(screen.queryByText('common.loading')).not.toBeInTheDocument()
    })

    const saveButton = await screen.findByRole('button', { name: /admin.settings.deployChanges/i })
    await act(async () => {
      fireEvent.click(saveButton)
    })

    await waitFor(() => {
      expect(settingsAPI.update).toHaveBeenCalledTimes(1)
    })

    const payload = vi.mocked(settingsAPI.update).mock.calls[0][0] as Record<string, string>
    expect(payload).not.toHaveProperty('default_payment_provider')
    expect(payload).not.toHaveProperty('base_domain')
    expect(payload).not.toHaveProperty('project_domain')
    expect(payload).toHaveProperty('admin_idle_timeout', '15')
    expect(payload).toHaveProperty('max_concurrent_builds', '3')
  })

  it('switches payment provider via dedicated finance endpoint with audit reason', async () => {
    render(<AdminSettings />)

    await waitFor(() => {
      expect(screen.queryByText('common.loading')).not.toBeInTheDocument()
    })

    const select = screen.getByLabelText(/Active Provider/i) as HTMLSelectElement
    expect(select.value).toBe('pakasir')

    // Change provider to midtrans
    await act(async () => {
      fireEvent.change(select, { target: { value: 'midtrans' } })
    })

    // Confirmation dialog should appear
    await waitFor(() => {
      expect(screen.getByText('Confirm Payment Gateway Switch')).toBeInTheDocument()
    })

    const reasonInput = screen.getByPlaceholderText(/e\.g\. Gateway maintenance failover/i)
    const confirmButton = screen.getByRole('button', { name: /Confirm Switch/i })

    // Confirm button is disabled without reason
    expect(confirmButton).toBeDisabled()

    await act(async () => {
      fireEvent.change(reasonInput, { target: { value: 'Switching to Midtrans for maintenance' } })
    })
    expect(confirmButton).not.toBeDisabled()

    await act(async () => {
      fireEvent.click(confirmButton)
    })

    await waitFor(() => {
      expect(billingAPI.updatePaymentProvider).toHaveBeenCalledWith({
        provider: 'midtrans',
        reason: 'Switching to Midtrans for maintenance',
      })
    })
  })

  it('disables payment provider dropdown for non-superadmin users', async () => {
    useAuthStore.setState({
      user: {
        id: 2,
        name: 'regularadmin',
        email: 'admin@example.com',
        role: 'admin',
      },
    })

    render(<AdminSettings />)

    await waitFor(() => {
      expect(screen.queryByText('common.loading')).not.toBeInTheDocument()
    })

    const select = screen.getByLabelText(/Active Provider/i) as HTMLSelectElement
    expect(select).toBeDisabled()
    expect(screen.getByText('Superadmin Required')).toBeInTheDocument()
  })

  it('does not treat empty or invalid provider as saved Pakasir and allows configuring it', async () => {
    vi.mocked(settingsAPI.list).mockResolvedValue({
      data: {
        map: {
          admin_idle_timeout: 15,
          default_payment_provider: '', // Corrupt / unconfigured empty row
        },
      },
    } as any)

    render(<AdminSettings />)

    await waitFor(() => {
      expect(screen.queryByText('common.loading')).not.toBeInTheDocument()
    })

    const select = screen.getByLabelText(/Active Provider/i) as HTMLSelectElement
    expect(select.value).toBe('')

    // Superadmin selects Pakasir to configure it
    await act(async () => {
      fireEvent.change(select, { target: { value: 'pakasir' } })
    })

    // Confirmation dialog should appear to configure Pakasir
    await waitFor(() => {
      expect(screen.getByText('Confirm Payment Gateway Switch')).toBeInTheDocument()
    })

    const reasonInput = screen.getByPlaceholderText(/e\.g\. Gateway maintenance failover/i)
    const confirmButton = screen.getByRole('button', { name: /Confirm Switch/i })

    await act(async () => {
      fireEvent.change(reasonInput, { target: { value: 'Initial Pakasir configuration' } })
    })

    await act(async () => {
      fireEvent.click(confirmButton)
    })

    await waitFor(() => {
      expect(billingAPI.updatePaymentProvider).toHaveBeenCalledWith({
        provider: 'pakasir',
        reason: 'Initial Pakasir configuration',
      })
    })
  })

  it('completes provider switch flow when API request resolves after re-auth challenge', async () => {
    let callCount = 0
    vi.mocked(billingAPI.updatePaymentProvider).mockImplementation(async () => {
      callCount++
      if (callCount === 1) {
        // First attempt rejected with RECENT_AUTH_REQUIRED
        return new Promise((resolve) => {
          const handleReauth = () => {
            window.removeEventListener('auth:reauthenticated', handleReauth)
            // Retry resolves
            resolve({ data: { success: true } } as any)
          }
          window.addEventListener('auth:reauthenticated', handleReauth)
          window.dispatchEvent(new Event('auth:recent_auth_required'))
        })
      }
      return { data: { success: true } } as any
    })

    render(<AdminSettings />)

    await waitFor(() => {
      expect(screen.queryByText('common.loading')).not.toBeInTheDocument()
    })

    const select = screen.getByLabelText(/Active Provider/i) as HTMLSelectElement
    await act(async () => {
      fireEvent.change(select, { target: { value: 'midtrans' } })
    })

    const reasonInput = await screen.findByPlaceholderText(/e\.g\. Gateway maintenance failover/i)
    const confirmButton = screen.getByRole('button', { name: /Confirm Switch/i })

    await act(async () => {
      fireEvent.change(reasonInput, { target: { value: 'Failover' } })
    })

    await act(async () => {
      fireEvent.click(confirmButton)
    })

    // Simulate successful password reauthentication in modal
    act(() => {
      window.dispatchEvent(new Event('auth:reauthenticated'))
    })

    await waitFor(() => {
      expect(screen.queryByText('Confirm Payment Gateway Switch')).not.toBeInTheDocument()
      expect(select.value).toBe('midtrans')
    })
  })
})
