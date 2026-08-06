import { describe, expect, it } from 'vitest'
import { createTopupIdempotencyKey, hasLowCreditBalance, nextBillingRequestState } from './billing-ui'

describe('billing UI guards', () => {
  it('retains prior financial data when polling fails', () => {
    const current = { status: 'success' as const, data: { balance_credits: 250 } }
    const result = nextBillingRequestState<{ balance_credits: number }, { balance_credits: number }>(
      { status: 'rejected', reason: new Error('offline') },
      current,
      (value) => value,
    )

    expect(result).toEqual(current)
  })

  it('generates a fresh per-attempt idempotency key', () => {
    const first = createTopupIdempotencyKey()
    const second = createTopupIdempotencyKey()

    expect(first).not.toBe(second)
    expect(first).toMatch(/^[0-9a-f]{32}$/)
    expect(second).toMatch(/^[0-9a-f]{32}$/)
  })

  it('warns when positive credits cannot cover the next active-resource charge', () => {
    expect(hasLowCreditBalance(20, 75)).toBe(true)
    expect(hasLowCreditBalance(75, 75)).toBe(false)
    expect(hasLowCreditBalance(20, 0)).toBe(false)
  })

  it('stores selected response data with an explicit success state', () => {
    const result = nextBillingRequestState<{ packages: number[] }, number[]>(
      { status: 'fulfilled', value: { packages: [1, 2] } },
      { status: 'loading' },
      (value) => value.packages,
    )
    expect(result).toEqual({ status: 'success', data: [1, 2] })
  })
})
