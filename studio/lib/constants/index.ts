// Ignore barrel file rule here since it's just exporting more constants
// eslint-disable-next-line barrel-files/avoid-re-export-all
export * from './infrastructure'

/**
 * SupaDash always runs in platform mode.
 * This is hardcoded to `true` to enable all platform features
 * (multi-project management, auth, settings, observability, etc.)
 * without relying on an env var.
 */
export const IS_PLATFORM = true as boolean

/**
 * Indicates that the app is running in a test environment (E2E tests).
 * Set via NEXT_PUBLIC_NODE_ENV=test in the generateLocalEnv.js script.
 */
export const IS_TEST_ENV = process.env.NEXT_PUBLIC_NODE_ENV === 'test'

/**
 * Default home page after login — always the projects list.
 */
export const DEFAULT_HOME = '/projects'

/**
 * API_URL resolves to the management API endpoint.
 * In SupaDash, this always comes from the NEXT_PUBLIC_API_URL env var,
 * falling back to a relative `/api` path for the local proxy.
 */
export const API_URL = process.env.NEXT_PUBLIC_API_URL || '/api'

export const PG_META_URL = process.env.PLATFORM_PG_META_URL
export const BASE_PATH = process.env.NEXT_PUBLIC_BASE_PATH ?? ''

/**
 * @deprecated use DATETIME_FORMAT
 */
export const DATE_FORMAT = 'YYYY-MM-DDTHH:mm:ssZ'

// should be used for all dayjs formattings shown to the user. Includes timezone info.
export const DATETIME_FORMAT = 'DD MMM YYYY, HH:mm:ss (ZZ)'

export const GOTRUE_ERRORS = {
  UNVERIFIED_GITHUB_USER: 'Error sending confirmation mail',
}

export const STRIPE_PUBLIC_KEY =
  process.env.NEXT_PUBLIC_STRIPE_PUBLIC_KEY || 'pk_test_XVwg5IZH3I9Gti98hZw6KRzd00v5858heG'

export const POSTHOG_URL = process.env.NEXT_PUBLIC_POSTHOG_URL || ''

export const USAGE_APPROACHING_THRESHOLD = 0.75

export const DOCS_URL = process.env.NEXT_PUBLIC_DOCS_URL || 'https://docs.supadash.com'

export const OPT_IN_TAGS = {
  AI_SQL: 'AI_SQL_GENERATOR_OPT_IN',
  AI_DATA: 'AI_DATA_GENERATOR_OPT_IN',
  AI_LOG: 'AI_LOG_GENERATOR_OPT_IN',
}

export const GB = 1024 * 1024 * 1024
export const MB = 1024 * 1024
export const KB = 1024

export const UUID_REGEX = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
