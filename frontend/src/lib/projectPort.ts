export function isValidProjectPort(port: number | ''): boolean {
  return port === '' || (Number.isInteger(port) && port >= 0 && port <= 65535)
}

export function formatProjectPort(
  port: number | null | undefined,
  internalPort: string | undefined,
  autoLabel: string,
  workerOnlyLabel: string,
): string {
  if (port === 0) return workerOnlyLabel
  if (typeof port === 'number') return String(port)
  return internalPort ? `${autoLabel} (${internalPort})` : autoLabel
}
