import { describe, it, expect, beforeEach, vi } from 'vitest'

const mockAuthAPI = {
  me: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  returnToAdmin: vi.fn(),
}

vi.mock('../services/api', () => ({
  authAPI: mockAuthAPI,
}))

describe('useAuthStore session concurrency & epoch guard', () => {
  beforeEach(async () => {
    vi.clearAllMocks()
    const { default: useAuthStore } = await import('./authStore')
    useAuthStore.setState({
      user: null,
      token: null,
      adminToken: null,
      isLoading: false,
    })
  })

  it('deduplicates in-flight fetchUser calls to a single authAPI.me request', async () => {
    const { default: useAuthStore } = await import('./authStore')
    let resolveMe: (value: any) => void = () => {}
    mockAuthAPI.me.mockReturnValue(
      new Promise((res) => {
        resolveMe = res
      })
    )

    const call1 = useAuthStore.getState().fetchUser()
    const call2 = useAuthStore.getState().fetchUser()

    expect(mockAuthAPI.me).toHaveBeenCalledTimes(1)

    resolveMe({
      data: { id: 1, email: 'user@example.com', name: 'User', role: 'user' },
      headers: {},
    })

    await Promise.all([call1, call2])
    expect(useAuthStore.getState().user?.email).toBe('user@example.com')
    expect(useAuthStore.getState().token).toBe('authenticated')
  })

  it('discards stale fetchUser resolve if login() transitions the session in-flight', async () => {
    const { default: useAuthStore } = await import('./authStore')
    let resolveMe: (value: any) => void = () => {}
    mockAuthAPI.me.mockReturnValue(
      new Promise((res) => {
        resolveMe = res
      })
    )

    // Stale fetch starts (anonymous)
    const pendingFetch = useAuthStore.getState().fetchUser()

    // User logs in while old fetch is still in flight
    mockAuthAPI.login.mockResolvedValue({
      data: { user: { id: 2, email: 'real@example.com', name: 'Real', role: 'admin' } },
    })
    await useAuthStore.getState().login('real@example.com', 'pass')
    expect(useAuthStore.getState().user?.email).toBe('real@example.com')

    // Stale fetch finally resolves with old/stale response
    resolveMe({
      data: { id: 99, email: 'stale@example.com', name: 'Stale', role: 'user' },
      headers: {},
    })
    await pendingFetch

    // Must NOT be overwritten by the stale resolve
    expect(useAuthStore.getState().user?.email).toBe('real@example.com')
  })

  it('discards stale fetchUser 401 rejection if login() transitions the session in-flight', async () => {
    const { default: useAuthStore } = await import('./authStore')
    let rejectMe: (reason: any) => void = () => {}
    mockAuthAPI.me.mockReturnValue(
      new Promise((_, rej) => {
        rejectMe = rej
      })
    )

    const pendingFetch = useAuthStore.getState().fetchUser()

    mockAuthAPI.login.mockResolvedValue({
      data: { user: { id: 2, email: 'real@example.com', name: 'Real', role: 'admin' } },
    })
    await useAuthStore.getState().login('real@example.com', 'pass')

    // Old fetch returns 401
    const err = new Error('Unauthorized') as any
    err.response = { status: 401 }
    rejectMe(err)
    await pendingFetch

    // Must NOT be cleared to null by the stale 401
    expect(useAuthStore.getState().token).toBe('authenticated')
    expect(useAuthStore.getState().user?.email).toBe('real@example.com')
  })

  it('expireSession invalidates pending in-flight fetchUser and clears state', async () => {
    const { default: useAuthStore } = await import('./authStore')
    let resolveMe: (value: any) => void = () => {}
    mockAuthAPI.me.mockReturnValue(
      new Promise((res) => {
        resolveMe = res
      })
    )

    const pendingFetch = useAuthStore.getState().fetchUser()
    useAuthStore.getState().expireSession()

    expect(useAuthStore.getState().token).toBeNull()
    expect(useAuthStore.getState().user).toBeNull()

    // Old fetch succeeds after session expired — must not resurrect the session
    resolveMe({
      data: { id: 1, email: 'zombie@example.com', name: 'Zombie', role: 'user' },
      headers: {},
    })
    await pendingFetch

    expect(useAuthStore.getState().token).toBeNull()
    expect(useAuthStore.getState().user).toBeNull()
  })
})
