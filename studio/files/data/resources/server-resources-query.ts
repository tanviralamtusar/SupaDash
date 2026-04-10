import { useQuery, UseQueryOptions } from '@tanstack/react-query'
import { get } from 'data/fetchers'
import { ResponseError } from 'types'

export type ServerCapacity = {
  total_cpu: number
  total_memory_mb: number
  used_cpu: number
  used_memory_mb: number
  free_cpu: number
  free_memory_mb: number
  project_count: number
  utilization_percent: number
}

export type BurstPoolStatus = {
  total_pool_mb: number
  used_pool_mb: number
  free_pool_mb: number
  utilization_percent: number
  active_bursts: number
  eligible_count: number
}

export type ServerCapacityResponse = {
  capacity: ServerCapacity
  burst_pool: BurstPoolStatus
}

export async function getServerCapacity(signal?: AbortSignal) {
  const { data, error } = await get('/supadash/server/capacity', { signal })
  if (error) throw error
  return data as ServerCapacityResponse
}

export type ServerCapacityData = Awaited<ReturnType<typeof getServerCapacity>>

export const useServerCapacityQuery = <TData = ServerCapacityData>(
  options: UseQueryOptions<ServerCapacityData, ResponseError, TData> = {} as any
) => {
  const { queryKey, ...otherOptions } = options

  return useQuery<ServerCapacityData, ResponseError, TData>({
    queryKey: ['server-capacity', ...(queryKey as any || [])],
    queryFn: ({ signal }) => getServerCapacity(signal),
    staleTime: 10 * 1000,
    refetchInterval: 10 * 1000,
    ...otherOptions,
  })
}
