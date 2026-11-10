import { describe, expect, it } from 'vitest'

import { parseFailure } from './deploymentFailure'

describe('parseFailure', () => {
  it('splits the apperr prefix from the sentence', () => {
    expect(parseFailure('[INVALID_BASE_DIRECTORY] Configured base directory does not exist.'))
      .toEqual({
        code: 'INVALID_BASE_DIRECTORY',
        message: 'Configured base directory does not exist.',
        remedy: 'settings',
      })
  })

  it('routes configuration failures away from retry', () => {
    // Retrying cannot change any of these; they are Settings-tab fields.
    for (const code of [
      'INVALID_BASE_DIRECTORY',
      'PORT_RESOLUTION_REQUIRED',
      'INVALID_RUNTIME_IMAGE',
      'NO_START_COMMAND',
    ]) {
      expect(parseFailure(`[${code}] broken`)?.remedy).toBe('settings')
    }
  })

  it('routes build and migration failures to the log', () => {
    for (const code of [
      'BUILD_FAILED',
      'DOCKER_BUILD_FAILED',
      'DOCKER_RUN_FAILED',
      'MIGRATION_FAILED',
      'MIGRATION_SCHEMA_CONFLICT',
    ]) {
      expect(parseFailure(`[${code}] broken`)?.remedy).toBe('logs')
    }
  })

  it('keeps retry for a code it does not recognize', () => {
    // New backend codes must not route the user somewhere arbitrary.
    const parsed = parseFailure('[SOME_FUTURE_CODE] something went wrong')
    expect(parsed).toEqual({
      code: 'SOME_FUTURE_CODE',
      message: 'something went wrong',
      remedy: 'retry',
    })
  })

  it('keeps retry for a message with no code at all', () => {
    expect(parseFailure('Initial deployment could not be queued. Retry deployment.'))
      .toEqual({
        message: 'Initial deployment could not be queued. Retry deployment.',
        remedy: 'retry',
      })
  })

  it('preserves the multi-line recommendation block the worker appends', () => {
    const raw = '[BUILD_FAILED] composer install failed\n\nRunara Recommendation:\n- Check PHP 8.3'
    const parsed = parseFailure(raw)
    expect(parsed?.message).toBe('composer install failed\n\nRunara Recommendation:\n- Check PHP 8.3')
  })

  it('ignores bracketed text that is not a code', () => {
    // Lowercase, spaces, and too-short tokens are ordinary prose.
    expect(parseFailure('[see logs] it broke')?.code).toBeUndefined()
    expect(parseFailure('[AB] it broke')?.code).toBeUndefined()
  })

  it('falls back to the raw text when the code is the whole message', () => {
    expect(parseFailure('[BUILD_FAILED]')?.message).toBe('[BUILD_FAILED]')
  })

  it('returns null for nothing to show', () => {
    expect(parseFailure(undefined)).toBeNull()
    expect(parseFailure('')).toBeNull()
    expect(parseFailure('   ')).toBeNull()
  })
})
