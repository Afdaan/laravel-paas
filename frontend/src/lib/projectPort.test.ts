import { describe, expect, it } from 'vitest'
import { formatProjectPort, isValidProjectPort } from './projectPort'

describe('project port helpers', () => {
  it('accepts auto, worker-only, and valid TCP ports', () => {
    expect(isValidProjectPort('')).toBe(true)
    expect(isValidProjectPort(0)).toBe(true)
    expect(isValidProjectPort(65535)).toBe(true)
    expect(isValidProjectPort(-1)).toBe(false)
    expect(isValidProjectPort(65536)).toBe(false)
    expect(isValidProjectPort(1.5)).toBe(false)
  })

  it('formats the three runtime port states explicitly', () => {
    expect(formatProjectPort(null, '8000', 'Auto', 'Worker-only')).toBe('Auto (8000)')
    expect(formatProjectPort(null, undefined, 'Auto', 'Worker-only')).toBe('Auto')
    expect(formatProjectPort(0, undefined, 'Auto', 'Worker-only')).toBe('Worker-only')
    expect(formatProjectPort(3000, '8000', 'Auto', 'Worker-only')).toBe('3000')
  })
})
