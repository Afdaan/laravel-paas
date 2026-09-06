import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { act, useEffect } from 'react'
import { MemoryRouter, useNavigate } from 'react-router-dom'
import ReauthModal from './ReauthModal'
import { authAPI } from '@/services/api'
import useAuthStore from '@/stores/authStore'

vi.mock('@/services/api', () => ({
  authAPI: {
    reauthenticate: vi.fn(),
  },
}))

const mockT = (key: string) => {
  const map: Record<string, string> = {
    'common.reauthTitle': 'Confirm Password',
    'common.reauthDesc': 'Session check. Re-enter password to continue.',
    'common.reauthPasswordLabel': 'Password',
    'common.reauthSubmit': 'Confirm',
    'common.reauthCancel': 'Cancel',
    'common.reauthFailed': 'Incorrect password.',
  }
  return map[key] || key
}

vi.mock('@/lib/useTranslation', () => ({
  default: () => ({
    t: mockT,
  }),
}))

describe('ReauthModal Component', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    useAuthStore.setState({
      token: 'valid-session-marker',
      user: {
        id: 1,
        name: 'superadmin',
        email: 'superadmin@example.com',
        role: 'superadmin',
      },
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders closed initially and opens on auth:recent_auth_required event', async () => {
    render(
      <MemoryRouter>
        <ReauthModal />
      </MemoryRouter>
    )

    expect(screen.queryByText('Confirm Password')).not.toBeInTheDocument()

    act(() => {
      window.dispatchEvent(new Event('auth:recent_auth_required'))
    })

    await waitFor(() => {
      expect(screen.getByText('Confirm Password')).toBeInTheDocument()
    })
  })

  it('submits password, calls reauthenticate, closes dialog, and dispatches auth:reauthenticated', async () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent')
    vi.mocked(authAPI.reauthenticate).mockResolvedValue({ data: {} } as any)

    render(
      <MemoryRouter>
        <ReauthModal />
      </MemoryRouter>
    )

    act(() => {
      window.dispatchEvent(new Event('auth:recent_auth_required'))
    })

    await waitFor(() => {
      expect(screen.getByText('Confirm Password')).toBeInTheDocument()
    })

    const passwordInput = screen.getByLabelText('Password')
    const confirmButton = screen.getByRole('button', { name: 'Confirm' })

    act(() => {
      fireEvent.change(passwordInput, { target: { value: 'my-super-secret' } })
    })

    await act(async () => {
      fireEvent.click(confirmButton)
    })

    await waitFor(() => {
      expect(authAPI.reauthenticate).toHaveBeenCalledWith('my-super-secret')
      expect(dispatchSpy).toHaveBeenCalledWith(expect.objectContaining({ type: 'auth:reauthenticated' }))
      expect(screen.queryByText('Confirm Password')).not.toBeInTheDocument()
    })
  })

  it('displays error message, keeps modal open, and does NOT expire session on wrong password (401 AUTH_FAILED)', async () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent')
    vi.mocked(authAPI.reauthenticate).mockRejectedValue({
      response: { status: 401, data: { code: 'AUTH_FAILED', error: 'Invalid email or password' } },
    })

    render(
      <MemoryRouter>
        <ReauthModal />
      </MemoryRouter>
    )

    act(() => {
      window.dispatchEvent(new Event('auth:recent_auth_required'))
    })

    const passwordInput = await screen.findByLabelText('Password')
    const confirmButton = screen.getByRole('button', { name: 'Confirm' })

    act(() => {
      fireEvent.change(passwordInput, { target: { value: 'wrong-pass' } })
    })

    await act(async () => {
      fireEvent.click(confirmButton)
    })

    await waitFor(() => {
      expect(screen.getByText('Invalid email or password')).toBeInTheDocument()
      expect(dispatchSpy).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'auth:reauthenticated' }))
      expect(dispatchSpy).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'auth:expired' }))
      expect(dispatchSpy).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'auth:reauth_cancelled' }))
      expect(screen.getByText('Confirm Password')).toBeInTheDocument()
      expect(useAuthStore.getState().token).toBe('valid-session-marker')
    })
  })

  it('opens modal successfully without premature cancellation when app mounted at /login and navigated to /admin/billing before challenge', async () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent')

    let navigateFn: (to: string) => void = () => {}
    function NavigationApp() {
      const navigate = useNavigate()
      useEffect(() => {
        navigateFn = navigate
      }, [navigate])
      return <ReauthModal />
    }

    render(
      <MemoryRouter initialEntries={['/login']}>
        <NavigationApp />
      </MemoryRouter>
    )

    // First navigate from /login to /admin/billing
    act(() => {
      navigateFn('/admin/billing')
    })

    // Now dispatch recent_auth_required at /admin/billing
    act(() => {
      window.dispatchEvent(new Event('auth:recent_auth_required'))
    })

    await waitFor(() => {
      expect(screen.getByText('Confirm Password')).toBeInTheDocument()
    })

    // Modal must remain open and MUST NOT prematurely cancel
    expect(dispatchSpy).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'auth:reauth_cancelled' }))
    expect(screen.getByText('Confirm Password')).toBeInTheDocument()
  })

  it('cancels re-auth and closes dialog when user navigates away to another authenticated route after opening challenge on navigated route', async () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent')

    let navigateFn: (to: string) => void = () => {}
    function NavigationApp() {
      const navigate = useNavigate()
      useEffect(() => {
        navigateFn = navigate
      }, [navigate])
      return <ReauthModal />
    }

    render(
      <MemoryRouter initialEntries={['/login']}>
        <NavigationApp />
      </MemoryRouter>
    )

    // Mount at /login, then navigate to /admin/billing
    act(() => {
      navigateFn('/admin/billing')
    })

    // Dispatch challenge at /admin/billing
    act(() => {
      window.dispatchEvent(new Event('auth:recent_auth_required'))
    })

    await waitFor(() => {
      expect(screen.getByText('Confirm Password')).toBeInTheDocument()
    })

    // Now navigate away to a different route e.g. /projects
    act(() => {
      navigateFn('/projects')
    })

    await waitFor(() => {
      expect(dispatchSpy).toHaveBeenCalledWith(expect.objectContaining({ type: 'auth:reauth_cancelled' }))
      expect(screen.queryByText('Confirm Password')).not.toBeInTheDocument()
    })
  })

  it('dispatches auth:reauth_cancelled and closes dialog when Cancel is clicked', async () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent')

    render(
      <MemoryRouter>
        <ReauthModal />
      </MemoryRouter>
    )

    act(() => {
      window.dispatchEvent(new Event('auth:recent_auth_required'))
    })

    await waitFor(() => {
      expect(screen.getByText('Confirm Password')).toBeInTheDocument()
    })

    const cancelButton = screen.getByRole('button', { name: 'Cancel' })
    act(() => {
      fireEvent.click(cancelButton)
    })

    await waitFor(() => {
      expect(dispatchSpy).toHaveBeenCalledWith(expect.objectContaining({ type: 'auth:reauth_cancelled' }))
      expect(screen.queryByText('Confirm Password')).not.toBeInTheDocument()
    })
  })

  it('preserves entered password if another concurrent auth:recent_auth_required event is dispatched while open', async () => {
    render(
      <MemoryRouter>
        <ReauthModal />
      </MemoryRouter>
    )

    act(() => {
      window.dispatchEvent(new Event('auth:recent_auth_required'))
    })

    const passwordInput = (await screen.findByLabelText('Password')) as HTMLInputElement
    act(() => {
      fireEvent.change(passwordInput, { target: { value: 'in-progress-typing' } })
    })
    expect(passwordInput.value).toBe('in-progress-typing')

    // Second concurrent event
    act(() => {
      window.dispatchEvent(new Event('auth:recent_auth_required'))
    })

    expect(passwordInput.value).toBe('in-progress-typing')
  })

  it('closes dialog and dispatches auth:reauth_cancelled when auth:expired event fires', async () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent')

    render(
      <MemoryRouter>
        <ReauthModal />
      </MemoryRouter>
    )

    act(() => {
      window.dispatchEvent(new Event('auth:recent_auth_required'))
    })

    await waitFor(() => {
      expect(screen.getByText('Confirm Password')).toBeInTheDocument()
    })

    act(() => {
      window.dispatchEvent(new Event('auth:expired'))
    })

    await waitFor(() => {
      expect(dispatchSpy).toHaveBeenCalledWith(expect.objectContaining({ type: 'auth:reauth_cancelled' }))
      expect(screen.queryByText('Confirm Password')).not.toBeInTheDocument()
    })
  })
})
