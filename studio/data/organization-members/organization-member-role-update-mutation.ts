import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { put, handleError } from 'data/fetchers'
import type { ResponseError, UseCustomMutationOptions } from 'types'
import { organizationKeys } from '../organizations/keys'

export type OrganizationMemberRoleUpdateVariables = {
  slug: string
  gotrueId: string
  roleId: number
  skipInvalidation?: boolean
}

// Map Studio numeric role IDs back to SupaDash Go backend strings
const ROLE_NAME_MAP: Record<number, string> = {
  1: 'owner',
  2: 'admin',
  3: 'developer',
  4: 'viewer',
}

export async function updateOrganizationMemberRole({
  slug,
  gotrueId,
  roleId,
}: OrganizationMemberRoleUpdateVariables) {
  const { data, error } = await put('/organizations/{slug}/team/{id}' as any, {
    params: { path: { slug, id: gotrueId } },
    body: {
      role: ROLE_NAME_MAP[roleId] || 'developer',
    },
  })
  if (error) handleError(error)
  return data
}

type OrganizationMemberRoleUpdateData = Awaited<ReturnType<typeof updateOrganizationMemberRole>>

export const useOrganizationMemberRoleUpdateMutation = ({
  onSuccess,
  onError,
  ...options
}: Omit<
  UseCustomMutationOptions<
    OrganizationMemberRoleUpdateData,
    ResponseError,
    OrganizationMemberRoleUpdateVariables
  >,
  'mutationFn'
> = {}) => {
  const queryClient = useQueryClient()

  return useMutation<
    OrganizationMemberRoleUpdateData,
    ResponseError,
    OrganizationMemberRoleUpdateVariables
  >({
    mutationFn: (vars) => updateOrganizationMemberRole(vars),
    async onSuccess(data, variables, context) {
      const { slug, skipInvalidation } = variables
      if (!skipInvalidation) {
        await queryClient.invalidateQueries({ queryKey: organizationKeys.members(slug) })
      }
      await onSuccess?.(data, variables, context)
    },
    async onError(data, variables, context) {
      if (onError === undefined) {
        toast.error(`Failed to update role: ${data.message}`)
      } else {
        onError(data, variables, context)
      }
    },
    ...options,
  })
}
