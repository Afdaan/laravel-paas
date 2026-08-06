export function retainOnRequestFailure<T>(
  result: PromiseSettledResult<T>,
  current: T | null,
  transform?: (value: T) => T,
): { value: T | null; failed: boolean } {
  if (result.status === 'fulfilled') {
    const value = transform ? transform(result.value) : result.value
    return { value, failed: false }
  }
  return { value: current, failed: true }
}

export function topupIdempotencyKey(
  keys: Map<number, string>,
  packageID: number,
  createKey: () => string,
): string {
  const existing = keys.get(packageID)
  if (existing) return existing

  const key = createKey()
  keys.set(packageID, key)
  return key
}

export function hasLowCreditBalance(balanceCredits: number, upcomingRequiredCredits: number): boolean {
  return upcomingRequiredCredits > 0 && balanceCredits < upcomingRequiredCredits
}
