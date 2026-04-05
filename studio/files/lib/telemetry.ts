import type { User } from '@supabase/supabase-js'
import { useRouter } from 'next/compat'
import { useEffect, useRef } from 'react'
import { API_URL } from './constants'

export interface TelemetryProps {
  screenResolution?: string
  language?: string
}

/**
 * Telemetry sending is disabled for SupaDash.
 * These are no-op stubs to prevent unnecessary API calls that would
 * hit nonexistent endpoints on the SupaDash backend.
 */
const sendEvent = (
  event: {
    category: string
    action: string
    label: string
    value?: string
  },
  gaProps?: TelemetryProps
) => {
  // Telemetry disabled for SupaDash — no-op
  return
}

const sendIdentify = (user: User, gaProps?: TelemetryProps) => {
  // Telemetry disabled for SupaDash — no-op
  return null
}

const sendActivity = (
  event: {
    activity: string
    source: string
    projectRef?: string
    orgSlug?: string
    data?: object
  },
  gaProps?: TelemetryProps
) => {
  // Telemetry disabled for SupaDash — no-op
  return null
}

const Telemetry = {
  sendEvent,
  sendIdentify,
  sendActivity,
}

export { Telemetry }
export default Telemetry
