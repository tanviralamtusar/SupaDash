import { useQuery } from '@tanstack/react-query'
import type { components } from 'data/api'
import { get, handleError } from 'data/fetchers'
import type { ResponseError, UseCustomQueryOptions } from 'types'
import { organizationKeys } from './keys'

export type OrganizationMembersVariables = {
  slug?: string
}

export type Member = components['schemas']['Member']
export interface OrganizationMember extends Member {
  invited_at?: string
  invited_id?: number
}

// Map SupaDash Go backend roles to Studio numeric role IDs
const ROLE_ID_MAP: Record<string, number> = {
  'owner': 1,
  'admin': 2,
  'developer': 3,
  'viewer': 4,
}

export async function getOrganizationMembers(
  { slug }: OrganizationMembersVariables,
  signal?: AbortSignal
) {
  if (!slug) throw new Error('slug is required')

  const { data, error } = await get('/organizations/{slug}/team', {
    params: { path: { slug } },
    signal,
  })

  if (error) handleError(error)

  // Map backend members to Studio OrganizationMember structure
  const orgMembers = (data as any[]).map((m) => ({
    gotrue_id: m.GotrueID,
    primary_email: m.Email,
    username: m.Username || m.Email.split('@')[0],
    role_ids: [ROLE_ID_MAP[m.Role.toLowerCase()] || 3], // Default to Developer if unknown
    mfa_enabled: false,
    created_at: m.CreatedAt,
    // [Tanvir] In SupaDash, we currently don't separate "Invites" from "Members" at the UI level
    // so we just return them all as members for now.
  }))

  return orgMembers as OrganizationMember[]
}

export type OrganizationMembersData = Awaited<ReturnType<typeof getOrganizationMembers>>
export type OrganizationMembersError = ResponseError

export const useOrganizationMembersQuery = <TData = OrganizationMembersData>(
  { slug }: OrganizationMembersVariables,
  {
    enabled = true,
    ...options
  }: UseCustomQueryOptions<OrganizationMembersData, OrganizationMembersError, TData> = {}
) =>
  useQuery<OrganizationMembersData, OrganizationMembersError, TData>({
    queryKey: organizationKeys.members(slug),
    queryFn: ({ signal }) => getOrganizationMembers({ slug }, signal),
    enabled: enabled && typeof slug !== 'undefined',
    ...options,
  })
