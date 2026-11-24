import { describe, expect, it } from 'vitest'
import { formatProjectPort, isValidProjectPort, projectPortInputValue, requiresWorkerOnlyConfirmation } from './projectPort'

describe('project port helpers', () => {
  it('accepts auto, worker-only, and valid TCP ports', () => {
    expect(isValidProjectPort('')).toBe(true)
    expect(isValidProjectPort(0)).toBe(true)
    expect(isValidProjectPort(65535)).toBe(true)
    expect(isValidProjectPort(-1)).toBe(false)
    expect(isValidProjectPort(65536)).toBe(false)
    expect(isValidProjectPort(1.5)).toBe(false)
  })

  it('normalizes omitted auto ports to an empty input', () => {
    expect(projectPortInputValue(undefined)).toBe('')
    expect(projectPortInputValue(null)).toBe('')
    expect(projectPortInputValue(0)).toBe(0)
    expect(projectPortInputValue(3000)).toBe(3000)
  })

  it('formats the three runtime port states explicitly', () => {
    expect(formatProjectPort(null, '8000', 'Auto', 'Worker-only')).toBe('Auto (8000)')
    expect(formatProjectPort(null, undefined, 'Auto', 'Worker-only')).toBe('Auto')
    expect(formatProjectPort(0, undefined, 'Auto', 'Worker-only')).toBe('Worker-only')
    expect(formatProjectPort(3000, '8000', 'Auto', 'Worker-only')).toBe('3000')
  })

  it('requires confirmation only for worker-only port zero', () => {
    expect(requiresWorkerOnlyConfirmation(0)).toBe(true)
    expect(requiresWorkerOnlyConfirmation('')).toBe(false)
    expect(requiresWorkerOnlyConfirmation(3000)).toBe(false)
  })
})
