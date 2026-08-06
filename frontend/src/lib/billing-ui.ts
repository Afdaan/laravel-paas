export type BillingRequestState<T> =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'success'; data: T }
  | { status: 'error'; error: string }

export function nextBillingRequestState<TResponse, T>(
  result: PromiseSettledResult<TResponse>,
  current: BillingRequestState<T>,
  select: (value: TResponse) => T,
): BillingRequestState<T> {
  if (result.status === 'fulfilled') {
    return { status: 'success', data: select(result.value) }
  }
  if (current.status === 'success') return current
  if (current.status === 'loading' || current.status === 'idle') {
    return { status: 'error', error: 'request_failed' }
  }
  return current
}

export function createTopupIdempotencyKey(): string {
  const random = new Uint8Array(16)
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    crypto.getRandomValues(random)
  } else {
    for (let i = 0; i < random.length; i++) {
      random[i] = Math.floor(Math.random() * 256)
    }
  }
  return Array.from(random)
    .map((byte) => byte.toString(16).padStart(2, '0'))
    .join('')
}

export function hasLowCreditBalance(balanceCredits: number, upcomingRequiredCredits: number): boolean {
  return upcomingRequiredCredits > 0 && balanceCredits < upcomingRequiredCredits
}
