import { ResponseError } from 'types'

/**
 * Error reporting is disabled for SupaDash.
 * Sentry is not used in the self-hosted environment.
 * All capture functions are no-ops that preserve the original API surface.
 */

export function captureCriticalError(
  error: ResponseError | Error | { message: string },
  context: string
): void {
  // No-op: Sentry disabled for SupaDash
  if (process.env.NODE_ENV === 'development' && error.message) {
    console.warn(`[SupaDash CriticalError] [${context}]`, error.message)
  }
}
