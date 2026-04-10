import { useQuery } from '@tanstack/react-query'
import type { ResponseError, UseCustomQueryOptions } from 'types'
import { organizationKeys } from './keys'

export const FIXED_ROLE_ORDER = ['Owner', 'Administrator', 'Developer', 'Read-only']

export type OrganizationRolesVariables = { slug?: string }

export type OrganizationRole = {
  id: number
  name: string
  description?: string | null
  projects: { ref: string; name: string }[]
  base_role_id?: number
}

export type OrganizationRolesResponse = {
  org_scoped_roles: OrganizationRole[]
  project_scoped_roles: OrganizationRole[]
}

// Fixed roles matching SupaDash Go backend expectations and Studio UI order
export const SUPADASH_ROLES: OrganizationRole[] = [
  { id: 1, name: 'Owner', description: 'Full access to all resources and settings.', projects: [] },
  { id: 2, name: 'Administrator', description: 'Can manage most settings and members.', projects: [] },
  { id: 3, name: 'Developer', description: 'Can manage project resources and code.', projects: [] },
  { id: 4, name: 'Read-only', description: 'Can view resources but not make changes.', projects: [] },
]

export async function getOrganizationRoles(
  { slug }: OrganizationRolesVariables,
  signal?: AbortSignal
) {
  if (!slug) throw new Error('slug is required')

  // In SupaDash, roles are currently fixed and don't require a backend fetch
  // to avoid unnecessary complexity for now.
  return {
    org_scoped_roles: SUPADASH_ROLES,
    project_scoped_roles: [],
  } as OrganizationRolesResponse
}

export type OrganizationRolesData = Awaited<ReturnType<typeof getOrganizationRoles>>
export type OrganizationRolesError = ResponseError

export const useOrganizationRolesV2Query = <TData = OrganizationRolesData>(
  { slug }: OrganizationRolesVariables,
  {
    enabled = true,
    ...options
  }: UseCustomQueryOptions<OrganizationRolesData, OrganizationRolesError, TData> = {}
) =>
  useQuery<OrganizationRolesData, OrganizationRolesError, TData>({
    queryKey: organizationKeys.rolesV2(slug),
    queryFn: ({ signal }) => getOrganizationRoles({ slug }, signal),
    enabled: enabled && typeof slug !== 'undefined',
    select: (data) => {
      return {
        ...data,
        org_scoped_roles: (data as OrganizationRolesResponse).org_scoped_roles.sort((a, b) => {
          return FIXED_ROLE_ORDER.indexOf(a.name) - FIXED_ROLE_ORDER.indexOf(b.name)
        }),
      } as any
    },
    ...options,
  })
