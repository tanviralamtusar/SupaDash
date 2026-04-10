import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import { post, handleError } from 'data/fetchers'
import type { ResponseError, UseCustomMutationOptions } from 'types'
import { organizationKeys } from '../organizations/keys'

export type OrganizationInvitationCreateVariables = {
  slug: string
  email: string
  roleId: number
}

// Map Studio numeric role IDs back to SupaDash Go backend strings
const ROLE_NAME_MAP: Record<number, string> = {
  1: 'owner',
  2: 'admin',
  3: 'developer',
  4: 'viewer',
}

export async function createOrganizationInvitation({
  slug,
  email,
  roleId,
}: OrganizationInvitationCreateVariables) {
  const { data, error } = await post('/organizations/{slug}/team/invite' as any, {
    params: { path: { slug } },
    body: {
      email,
      role: ROLE_NAME_MAP[roleId] || 'developer',
    },
  })
  if (error) handleError(error)
  return data
}

type OrganizationInvitationCreateData = Awaited<ReturnType<typeof createOrganizationInvitation>>

export const useOrganizationCreateInvitationMutation = ({
  onSuccess,
  onError,
  ...options
}: Omit<
  UseCustomMutationOptions<
    OrganizationInvitationCreateData,
    ResponseError,
    OrganizationInvitationCreateVariables
  >,
  'mutationFn'
> = {}) => {
  const queryClient = useQueryClient()

  return useMutation<
    OrganizationInvitationCreateData,
    ResponseError,
    OrganizationInvitationCreateVariables
  >({
    mutationFn: (vars) => createOrganizationInvitation(vars),
    async onSuccess(data, variables, context) {
      const { slug } = variables
      await queryClient.invalidateQueries({ queryKey: organizationKeys.members(slug) })
      await onSuccess?.(data, variables, context)
    },
    async onError(data, variables, context) {
      if (onError === undefined) {
        toast.error(`Failed to send invitation: ${data.message}`)
      } else {
        onError(data, variables, context)
      }
    },
    ...options,
  })
}
