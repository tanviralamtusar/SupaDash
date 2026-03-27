import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { get, put, handleError } from 'data/fetchers'
import type { ResponseError } from 'types'

export type ProjectResources = {
  project_ref: string
  plan: string
  cpu_limit: number
  cpu_reservation: number
  memory_limit_mb: number
  memory_reservation_mb: number
  burst_eligible: boolean
  burst_priority: number
}

export type ResourceSnapshot = {
  timestamp: string
  cpu_usage: number
  memory_usage: number
  disk_usage: number
}

export type ResourceRecommendation = {
  id: number
  project_ref: string
  type: string
  priority: number
  message: string
  action_plan: string
  status: string
  created_at: string
}

export type ProjectAnalysis = {
  project_ref: string
  snapshot_count: number
  snapshots: ResourceSnapshot[]
  recommendations: ResourceRecommendation[]
}

export async function getProjectResources(projectRef: string, signal?: AbortSignal) {
  if (!projectRef) throw new Error('projectRef is required')

  const { data, error } = await get(`/supadash/projects/{ref}/resources`, {
    params: {
      path: { ref: projectRef },
    },
    signal,
  })

  if (error) handleError(error)
  return data as ProjectResources
}

export const useProjectResourcesQuery = (projectRef: string) =>
  useQuery({
    queryKey: ['projects', projectRef, 'resources'],
    queryFn: ({ signal }) => getProjectResources(projectRef, signal),
    enabled: !!projectRef,
  })

export async function getProjectAnalysis(projectRef: string, signal?: AbortSignal) {
  if (!projectRef) throw new Error('projectRef is required')

  const { data, error } = await get(`/supadash/projects/{ref}/analysis`, {
    params: {
      path: { ref: projectRef },
    },
    signal,
  })

  if (error) handleError(error)
  return data as ProjectAnalysis
}

export const useProjectAnalysisQuery = (projectRef: string) =>
  useQuery({
    queryKey: ['projects', projectRef, 'analysis'],
    queryFn: ({ signal }) => getProjectAnalysis(projectRef, signal),
    enabled: !!projectRef,
    refetchInterval: 10000, // Refetch every 10s for real-time feel
  })

export async function updateProjectResources({
  projectRef,
  plan,
  cpu_limit,
  memory_limit_mb,
}: {
  projectRef: string
  plan: string
  cpu_limit: number
  memory_limit_mb: number
}) {
  const { data, error } = await put(`/supadash/projects/{ref}/resources`, {
    params: {
      path: { ref: projectRef },
    },
    body: {
      plan,
      cpu_limit,
      memory_limit_mb,
    },
  })

  if (error) handleError(error)
  return data as ProjectResources
}

export const useUpdateProjectResourcesMutation = () => {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (vars: Parameters<typeof updateProjectResources>[0]) => updateProjectResources(vars),
    onSuccess: (data, vars) => {
      queryClient.setQueryData(['projects', vars.projectRef, 'resources'], data)
    },
  })
}
