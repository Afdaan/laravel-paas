import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { AxiosError } from 'axios'
import api, { getCSRFToken, isSessionExpiredError } from './api'

describe('getCSRFToken helper', () => {
  const originalCookie = document.cookie

  afterEach(() => {
    Object.defineProperty(document, 'cookie', {
      writable: true,
      value: originalCookie,
    })
  })

  it('extracts paas_csrf token from document.cookie with varying whitespace', () => {
    Object.defineProperty(document, 'cookie', {
      writable: true,
      value: 'other=123; paas_csrf=secret-csrf-token-1; test=abc',
    })
    expect(getCSRFToken()).toBe('secret-csrf-token-1')
  })

  it('extracts __Host-paas_csrf token when present', () => {
    Object.defineProperty(document, 'cookie', {
      writable: true,
      value: '__Host-paas_csrf=prod-csrf-token-99; session=xyz',
    })
    expect(getCSRFToken()).toBe('prod-csrf-token-99')
  })

  it('returns empty string when cookie is empty or CSRF cookie is absent', () => {
    Object.defineProperty(document, 'cookie', {
      writable: true,
      value: 'session=xyz; user_id=1',
    })
    expect(getCSRFToken()).toBe('')
  })
})
describe('isSessionExpiredError helper', () => {
  it('returns false for 401 on /auth/re-auth with AUTH_FAILED', () => {
    expect(isSessionExpiredError(401, { code: 'AUTH_FAILED', error: 'Invalid password' }, '/api/auth/re-auth')).toBe(false)
  })

  it('returns false for 401 on /auth/login with AUTH_FAILED', () => {
    expect(isSessionExpiredError(401, { code: 'AUTH_FAILED', error: 'Invalid email or password' }, '/api/auth/login')).toBe(false)
  })

  it('returns true for 401 on /auth/re-auth with TOKEN_INVALID', () => {
    expect(isSessionExpiredError(401, { code: 'TOKEN_INVALID', error: 'Invalid session' }, '/api/auth/re-auth')).toBe(true)
  })

  it('returns true for 401 on ordinary protected routes', () => {
    expect(isSessionExpiredError(401, { code: 'UNAUTHORIZED', error: 'Unauthorized' }, '/api/projects')).toBe(true)
  })

  it('returns false for non-401 status codes', () => {
    expect(isSessionExpiredError(403, { code: 'FORBIDDEN' }, '/api/projects')).toBe(false)
    expect(isSessionExpiredError(500, { code: 'INTERNAL_ERROR' }, '/api/projects')).toBe(false)
  })
})

describe('API interceptor recent-auth 403 recovery', () => {
  const originalAdapter = api.defaults.adapter

  beforeEach(() => {
    vi.clearAllMocks()
    Object.defineProperty(document, 'cookie', {
      writable: true,
      value: 'paas_csrf=initial-csrf-token',
    })
  })

  afterEach(() => {
    api.defaults.adapter = originalAdapter
    vi.restoreAllMocks()
  })

  it('triggers auth:recent_auth_required on 403 RECENT_AUTH_REQUIRED and retries once with fresh CSRF token upon reauth', async () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent')
    let callCount = 0
    const configs: any[] = []

    api.defaults.adapter = async (config) => {
      callCount++
      configs.push(config)
      if (callCount === 1) {
        const error = new AxiosError('Request failed with status code 403', 'ERR_BAD_REQUEST', config, null, {
          status: 403,
          statusText: 'Forbidden',
          headers: {},
          config,
          data: { code: 'RECENT_AUTH_REQUIRED', error: 'Recent password authentication is required' },
        })
        return Promise.reject(error)
      }

      return {
        data: { success: true },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }

    // Listen for recent-auth event, update cookie, and fire reauthenticated event
    const handleRecentAuth = () => {
      Object.defineProperty(document, 'cookie', {
        writable: true,
        value: 'paas_csrf=fresh-csrf-token-after-reauth',
      })
      window.dispatchEvent(new Event('auth:reauthenticated'))
    }
    window.addEventListener('auth:recent_auth_required', handleRecentAuth, { once: true })

    const response = await api.put('/admin/billing/payment-provider', { provider: 'pakasir' }, {
      headers: { 'Idempotency-Key': 'idemp-123' } as any,
    })

    expect(response.data).toEqual({ success: true })
    expect(callCount).toBe(2)
    expect(dispatchSpy).toHaveBeenCalledWith(expect.objectContaining({ type: 'auth:recent_auth_required' }))

    // Check that retried request had the fresh CSRF token and preserved Idempotency-Key
    const secondCallConfig = configs[1]
    const csrfInHeaders = secondCallConfig.headers['X-CSRF-Token'] || (typeof secondCallConfig.headers.get === 'function' ? secondCallConfig.headers.get('X-CSRF-Token') : '')
    expect(csrfInHeaders).toBe('fresh-csrf-token-after-reauth')
    expect(secondCallConfig.headers['Idempotency-Key'] || secondCallConfig.headers.get?.('Idempotency-Key')).toBe('idemp-123')
    expect(secondCallConfig._isReauthRetry).toBe(true)
  })

  it('stops without loop if retried request receives 403 RECENT_AUTH_REQUIRED again', async () => {
    let callCount = 0

    api.defaults.adapter = async (config) => {
      callCount++
      const error = new AxiosError('Request failed with status code 403', 'ERR_BAD_REQUEST', config, null, {
        status: 403,
        statusText: 'Forbidden',
        headers: {},
        config,
        data: { code: 'RECENT_AUTH_REQUIRED', error: 'Recent password authentication is required' },
      })
      return Promise.reject(error)
    }

    const handleRecentAuth = () => {
      window.dispatchEvent(new Event('auth:reauthenticated'))
    }
    window.addEventListener('auth:recent_auth_required', handleRecentAuth)

    await expect(api.put('/admin/billing/payment-provider', { provider: 'pakasir' })).rejects.toThrow()
    // Should call once for original, once for retry, and STOP (callCount = 2, not infinite)
    expect(callCount).toBe(2)

    window.removeEventListener('auth:recent_auth_required', handleRecentAuth)
  })

  it('rejects original request immediately if re-auth is cancelled', async () => {
    api.defaults.adapter = async (config) => {
      const error = new AxiosError('Request failed with status code 403', 'ERR_BAD_REQUEST', config, null, {
        status: 403,
        statusText: 'Forbidden',
        headers: {},
        config,
        data: { code: 'RECENT_AUTH_REQUIRED', error: 'Recent password authentication is required' },
      })
      return Promise.reject(error)
    }

    const handleRecentAuth = () => {
      window.dispatchEvent(new Event('auth:reauth_cancelled'))
    }
    window.addEventListener('auth:recent_auth_required', handleRecentAuth, { once: true })

    await expect(api.put('/admin/billing/payment-provider', { provider: 'pakasir' })).rejects.toThrow()
  })

  it('does NOT retry previously cancelled request if auth:reauthenticated fires later', async () => {
    let callCount = 0
    api.defaults.adapter = async (config) => {
      callCount++
      const error = new AxiosError('Request failed with status code 403', 'ERR_BAD_REQUEST', config, null, {
        status: 403,
        statusText: 'Forbidden',
        headers: {},
        config,
        data: { code: 'RECENT_AUTH_REQUIRED', error: 'Recent password authentication is required' },
      })
      return Promise.reject(error)
    }

    const handleRecentAuth = () => {
      window.dispatchEvent(new Event('auth:reauth_cancelled'))
    }
    window.addEventListener('auth:recent_auth_required', handleRecentAuth, { once: true })

    await expect(api.put('/admin/billing/payment-provider', { provider: 'pakasir' })).rejects.toThrow()
    expect(callCount).toBe(1)

    // Now fire auth:reauthenticated later
    window.dispatchEvent(new Event('auth:reauthenticated'))
    // Call count must remain 1 (no retry executed)
    expect(callCount).toBe(1)
  })

  it('rejects original request immediately if auth:expired fires while waiting', async () => {
    api.defaults.adapter = async (config) => {
      const error = new AxiosError('Request failed with status code 403', 'ERR_BAD_REQUEST', config, null, {
        status: 403,
        statusText: 'Forbidden',
        headers: {},
        config,
        data: { code: 'RECENT_AUTH_REQUIRED', error: 'Recent password authentication is required' },
      })
      return Promise.reject(error)
    }

    const handleRecentAuth = () => {
      window.dispatchEvent(new Event('auth:expired'))
    }
    window.addEventListener('auth:recent_auth_required', handleRecentAuth, { once: true })

    await expect(api.put('/admin/billing/payment-provider', { provider: 'pakasir' })).rejects.toThrow()
  })

  it('does NOT trigger re-auth modal on CSRF_FAILED, IMPERSONATION_FORBIDDEN, or FORBIDDEN', async () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent')

    for (const code of ['CSRF_FAILED', 'IMPERSONATION_FORBIDDEN', 'FORBIDDEN', 'ORIGIN_FAILED']) {
      dispatchSpy.mockClear()
      api.defaults.adapter = async (config) => {
        const error = new AxiosError('Request failed with status code 403', 'ERR_BAD_REQUEST', config, null, {
          status: 403,
          statusText: 'Forbidden',
          headers: {},
          config,
          data: { code, error: `Forbidden: ${code}` },
        })
        return Promise.reject(error)
      }

      await expect(api.put('/admin/billing/payment-provider', { provider: 'pakasir' })).rejects.toThrow()
      expect(dispatchSpy).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'auth:recent_auth_required' }))
    }
  })

  it('handles concurrent 403 challenges and retries all waiting requests upon reauth', async () => {
    let callCount = 0

    api.defaults.adapter = async (config) => {
      callCount++
      if (!config._isReauthRetry) {
        const error = new AxiosError('Request failed with status code 403', 'ERR_BAD_REQUEST', config, null, {
          status: 403,
          statusText: 'Forbidden',
          headers: {},
          config,
          data: { code: 'RECENT_AUTH_REQUIRED', error: 'Recent password authentication is required' },
        })
        return Promise.reject(error)
      }

      return {
        data: { path: config.url },
        status: 200,
        statusText: 'OK',
        headers: {},
        config,
      }
    }

    const handleRecentAuth = () => {
      setTimeout(() => {
        Object.defineProperty(document, 'cookie', {
          writable: true,
          value: 'paas_csrf=new-token-concurrent',
        })
        window.dispatchEvent(new Event('auth:reauthenticated'))
      }, 10)
    }
    window.addEventListener('auth:recent_auth_required', handleRecentAuth)

    // Fire 2 concurrent requests
    const [res1, res2] = await Promise.all([
      api.put('/admin/billing/payment-provider', { provider: 'pakasir' }),
      api.post('/admin/billing/wallets/1/credits', { credits: 100 }, { headers: { 'Idempotency-Key': 'concurrent-key' } as any }),
    ])

    expect(res1.data).toEqual({ path: '/admin/billing/payment-provider' })
    expect(res2.data).toEqual({ path: '/admin/billing/wallets/1/credits' })
    expect(callCount).toBe(4) // 2 initial + 2 retries

    window.removeEventListener('auth:recent_auth_required', handleRecentAuth)
  })

  it('does NOT dispatch auth:expired on 401 AUTH_FAILED when submitting re-auth', async () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent')

    api.defaults.adapter = async (config) => {
      const error = new AxiosError('Request failed with status code 401', 'ERR_BAD_REQUEST', config, null, {
        status: 401,
        statusText: 'Unauthorized',
        headers: {},
        config,
        data: { code: 'AUTH_FAILED', error: 'Invalid email or password' },
      })
      return Promise.reject(error)
    }

    await expect(api.post('/auth/re-auth', { password: 'wrong-password' })).rejects.toThrow()
    expect(dispatchSpy).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'auth:expired' }))
  })

  it('dispatches auth:expired on 401 for ordinary protected routes or invalid token', async () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent')

    api.defaults.adapter = async (config) => {
      const error = new AxiosError('Request failed with status code 401', 'ERR_BAD_REQUEST', config, null, {
        status: 401,
        statusText: 'Unauthorized',
        headers: {},
        config,
        data: { code: 'TOKEN_INVALID', error: 'Invalid or expired session' },
      })
      return Promise.reject(error)
    }

    await expect(api.get('/projects')).rejects.toThrow()
    expect(dispatchSpy).toHaveBeenCalledWith(expect.objectContaining({ type: 'auth:expired' }))
  })
})
