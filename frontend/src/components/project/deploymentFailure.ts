/**
 * Deployment failures carry a machine code. `apperr.AppError.Error()` formats
 * as "[CODE] message" (shared/apperr), and the worker writes that string into
 * deployment_message / error_log verbatim.
 *
 * The code is worth parsing because it says whether retrying can possibly
 * help. A misconfigured base directory fails identically on every retry; the
 * fix is a settings change. Offering "Retry deployment" as the primary action
 * there sends the user in a circle.
 */

/** Where the user has to go to make the next attempt different. */
export type FailureRemedy = 'settings' | 'logs' | 'retry'

export interface ParsedFailure {
  /** Present only when the message carried a recognizable "[CODE]" prefix. */
  code?: string
  /** The message with the code prefix removed. */
  message: string
  remedy: FailureRemedy
}

/**
 * Codes reachable from the deploy path (worker/, shared/infrastructure/docker/).
 * Anything absent from this map keeps the retry-first behaviour, so an
 * unrecognized or newly added code never routes the user somewhere useless.
 */
const REMEDY_BY_CODE: Record<string, FailureRemedy> = {
  // Project configuration. Every one of these is a field in the Settings tab,
  // and none of them changes by retrying.
  INVALID_BASE_DIRECTORY: 'settings',
  PORT_RESOLUTION_REQUIRED: 'settings',
  INVALID_RUNTIME_IMAGE: 'settings',
  NO_START_COMMAND: 'settings',

  // The failure is in the application's own build or migration output, which
  // is what the build log holds.
  BUILD_FAILED: 'logs',
  DOCKER_BUILD_FAILED: 'logs',
  DOCKER_RUN_FAILED: 'logs',
  MIGRATION_FAILED: 'logs',
  MIGRATION_SCHEMA_CONFLICT: 'logs',
}

const CODE_PREFIX = /^\[([A-Z][A-Z0-9_]{2,63})\]\s*/

export function parseFailure(raw?: string | null): ParsedFailure | null {
  const text = raw?.trim()
  if (!text) return null

  const match = CODE_PREFIX.exec(text)
  if (!match) return { message: text, remedy: 'retry' }

  const code = match[1]
  const message = text.slice(match[0].length).trim()

  return {
    code,
    // A message that is nothing but a code still has to render something.
    message: message || text,
    remedy: REMEDY_BY_CODE[code] ?? 'retry',
  }
}
