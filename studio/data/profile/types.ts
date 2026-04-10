import type { components } from 'data/api'

export type Profile = components['schemas']['ProfileResponse'] & {
  profileImageUrl?: string
  totp_enabled?: boolean
  mfa_enabled?: boolean
}
