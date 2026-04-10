import type { PropsWithChildren } from 'react'

/**
 * PageTelemetry is disabled for SupaDash.
 * This is a pass-through component that renders its children without
 * sending any page view or telemetry data to external services.
 */
const PageTelemetry = ({ children }: PropsWithChildren<{}>) => {
  // Page telemetry disabled for SupaDash — no-op
  return <>{children}</>
}

export default PageTelemetry
