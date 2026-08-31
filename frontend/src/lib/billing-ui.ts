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

export function createAdjustmentIdempotencyKey(): string {
  return `adj-${createTopupIdempotencyKey()}`
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

// Decimal digits in each currency's minor unit. IDR is zero-decimal, so its minor unit
// equals its major unit; USD is stored in cents.
const CURRENCY_MINOR_UNITS: Record<string, number> = { IDR: 0, USD: 2 }

export const SUPPORTED_CURRENCIES = Object.keys(CURRENCY_MINOR_UNITS)

export function currencyMinorUnitDigits(currency: string): number {
  return CURRENCY_MINOR_UNITS[currency] ?? 2
}

// Backend stores money in minor units; Intl currency formatting expects major units.
// Unknown currencies fall back to 2 decimals — the conservative guess, since assuming
// zero would display a cents amount as if it were 100x larger.
export function toMajorUnits(amountMinor: number, currency: string): number {
  return amountMinor / 10 ** currencyMinorUnitDigits(currency)
}
