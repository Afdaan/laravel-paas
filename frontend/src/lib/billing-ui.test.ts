import { describe, expect, it } from 'vitest'
import { hasLowCreditBalance, retainOnRequestFailure, topupIdempotencyKey } from './billing-ui'

describe('billing UI guards', () => {
  it('retains prior financial data when a request fails', () => {
    const current = { balance_credits: 250 }
    const result = retainOnRequestFailure<{ balance_credits: number }>({ status: 'rejected', reason: new Error('offline') }, current)

    expect(result).toEqual({ value: current, failed: true })
  })

  it('uses one checkout idempotency key per top-up package', () => {
    const keys = new Map<number, string>()
    let created = 0
    const createKey = () => `key-${++created}`

    expect(topupIdempotencyKey(keys, 10, createKey)).toBe('key-1')
    expect(topupIdempotencyKey(keys, 10, createKey)).toBe('key-1')
    expect(topupIdempotencyKey(keys, 11, createKey)).toBe('key-2')
    expect(created).toBe(2)
  })

  it('warns when positive credits cannot cover the next active-resource charge', () => {
    expect(hasLowCreditBalance(20, 75)).toBe(true)
    expect(hasLowCreditBalance(75, 75)).toBe(false)
    expect(hasLowCreditBalance(20, 0)).toBe(false)
  })

  it('extracts nested value through transform when request succeeds', () => {
    const result = retainOnRequestFailure<{ packages: number[] }>(
      { status: 'fulfilled', value: { packages: [1, 2] } },
      null,
      (value) => value.packages,
    )
    expect(result.failed).toBe(false)
    expect(result.value).toEqual([1, 2])
  })
})
