import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { act } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { toast } from 'sonner'
import Login from './Login'

const mockLogin = vi.fn()

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
    dismiss: vi.fn(),
  },
}))

vi.mock('../stores/authStore', () => ({
  default: (selector: (state: any) => any) =>
    selector({
      login: mockLogin,
      token: null,
      user: null,
    }),
}))

vi.mock('../services/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../services/api')>()
  return {
    ...actual,
    systemAPI: {
      getInitStatus: vi.fn().mockResolvedValue({ data: { is_initialized: true } }),
    },
  }
})

vi.mock('@/components/ThemeProvider', () => ({
  useTheme: () => ({ theme: 'dark', setTheme: vi.fn() }),
}))

const messages: Record<string, string> = {
  'landing.primaryNavigation': 'Primary navigation',
  'landing.language.toggle': 'Toggle language',
  'landing.language.label': 'Language',
  'common.theme': 'Theme',
  'common.loading': 'Loading...',
  'login.title': 'Sign in',
  'login.subtitle': 'Enter your credentials',
  'login.email': 'Email',
  'login.emailPlaceholder': 'you@example.com',
  'login.password': 'Password',
  'login.passwordPlaceholder': 'Enter your password',
  'login.signIn': 'Sign in',
  'login.loggingIn': 'Signing in...',
  'login.showPassword': 'Show password',
  'login.hidePassword': 'Hide password',
  'login.emailRequired': 'Email is required.',
  'login.passwordRequired': 'Password is required.',
  'login.failed': 'Sign in failed.',
  'login.rateLimited': 'Too many login attempts. Try again in {{seconds}} seconds.',
  'login.retryIn': 'Try again in {{seconds}}s',
  'login.welcomeBack': 'Welcome back, {{name}}!',
}

vi.mock('../lib/useTranslation', () => ({
  default: () => ({
    language: 'en',
    setLanguage: vi.fn(),
    t: (key: string, data?: Record<string, unknown>) => {
      let value = messages[key] || key
      Object.entries(data || {}).forEach(([name, replacement]) => {
        value = value.replace(`{{${name}}}`, String(replacement))
      })
      return value
    },
  }),
}))

describe('Login 429 rate limit countdown and unlock flow', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    vi.clearAllMocks()

    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation((query) => ({
        matches: false,
        media: query,
        onchange: null,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.clearAllMocks()
  })

  it('completes the full 429 -> countdown -> unlock flow', async () => {
    const rateLimitError = {
      response: {
        status: 429,
        headers: { 'retry-after': '3' },
        data: { retry_after: 3, error: 'Too many login attempts' },
      },
    }

    mockLogin.mockRejectedValueOnce(rateLimitError)

    render(
      <MemoryRouter>
        <Login />
      </MemoryRouter>
    )

    const emailInput = screen.getByPlaceholderText('you@example.com')
    const passwordInput = screen.getByPlaceholderText('Enter your password')
    const submitButton = screen.getByRole('button', { name: /sign in/i })

    fireEvent.change(emailInput, { target: { value: 'user@example.com' } })
    fireEvent.change(passwordInput, { target: { value: 'secret123' } })

    await act(async () => {
      fireEvent.click(submitButton)
    })

    expect(mockLogin).toHaveBeenCalledTimes(1)

    // Rate limited state
    expect(toast.error).toHaveBeenCalledWith(
      'Too many login attempts. Try again in 3 seconds.',
      expect.objectContaining({ id: 'login-rate-limit' })
    )
    expect(screen.getByRole('button', { name: /try again in 3s/i })).toBeDisabled()

    // Advance 1 second: countdown to 2s
    await act(async () => {
      vi.advanceTimersByTime(1000)
    })
    expect(screen.getByRole('button', { name: /try again in 2s/i })).toBeDisabled()
    expect(toast.error).toHaveBeenCalledWith(
      'Too many login attempts. Try again in 2 seconds.',
      expect.objectContaining({ id: 'login-rate-limit' })
    )

    // Advance 1 second: countdown to 1s
    await act(async () => {
      vi.advanceTimersByTime(1000)
    })
    expect(screen.getByRole('button', { name: /try again in 1s/i })).toBeDisabled()

    // Advance 1 second: timer expires and unlocks
    await act(async () => {
      vi.advanceTimersByTime(1000)
    })

    // Unlocked
    const unlockedButton = screen.getByRole('button', { name: /sign in/i })
    expect(unlockedButton).not.toBeDisabled()
    expect(toast.dismiss).toHaveBeenCalledWith('login-rate-limit')

    // Second login attempt succeeds
    mockLogin.mockResolvedValueOnce({ id: 1, name: 'Alex' })
    await act(async () => {
      fireEvent.click(unlockedButton)
    })

    expect(mockLogin).toHaveBeenCalledTimes(2)
    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith('Welcome back, Alex!')
    })
  })

  it('immediately unlocks on visibilitychange when tab was in background past the deadline', async () => {
    const rateLimitError = {
      response: {
        status: 429,
        headers: { 'retry-after': '5' },
        data: { retry_after: 5 },
      },
    }

    mockLogin.mockRejectedValueOnce(rateLimitError)

    render(
      <MemoryRouter>
        <Login />
      </MemoryRouter>
    )

    fireEvent.change(screen.getByPlaceholderText('you@example.com'), { target: { value: 'user@example.com' } })
    fireEvent.change(screen.getByPlaceholderText('Enter your password'), { target: { value: 'secret123' } })

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /sign in/i }))
    })

    expect(screen.getByRole('button', { name: /try again in 5s/i })).toBeDisabled()

    // Simulate device sleep or background tab: time jumps ahead 6 seconds
    await act(async () => {
      vi.advanceTimersByTime(6000)
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true })
      document.dispatchEvent(new Event('visibilitychange'))
    })

    const unlockedButton = screen.getByRole('button', { name: /sign in/i })
    expect(unlockedButton).not.toBeDisabled()
    expect(toast.dismiss).toHaveBeenCalledWith('login-rate-limit')
  })

  it('blocks submission while rate-limited and warns user', async () => {
    const rateLimitError = {
      response: {
        status: 429,
        headers: { 'retry-after': '10' },
        data: { retry_after: 10 },
      },
    }

    mockLogin.mockRejectedValueOnce(rateLimitError)

    render(
      <MemoryRouter>
        <Login />
      </MemoryRouter>
    )

    fireEvent.change(screen.getByPlaceholderText('you@example.com'), { target: { value: 'user@example.com' } })
    fireEvent.change(screen.getByPlaceholderText('Enter your password'), { target: { value: 'secret123' } })

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /sign in/i }))
    })

    expect(mockLogin).toHaveBeenCalledTimes(1)

    // Try submitting again via form submission while locked
    const form = screen.getByPlaceholderText('you@example.com').closest('form')!
    await act(async () => {
      fireEvent.submit(form)
    })

    // mockLogin should not be called again
    expect(mockLogin).toHaveBeenCalledTimes(1)
    expect(toast.error).toHaveBeenCalledWith(
      expect.stringMatching(/Too many login attempts/),
      expect.objectContaining({ id: 'login-rate-limit' })
    )
  })
})
