import React from 'react'
import { Card, CardContent, CardDescription, CardHeader, CardTitle, Badge } from 'ui'
import { Server, Activity, Database, Cpu } from 'lucide-react'
import { useServerCapacityQuery } from 'data/resources/server-resources-query'
import { ResourceGauge } from './ResourceGauge'
import { LoadingLine } from 'ui'

export const ServerOverview = () => {
  const { data, isLoading, error } = useServerCapacityQuery()

  if (isLoading) return <LoadingLine />
  if (error) return <div className="p-4 text-red-500">Error loading server capacity</div>
  if (!data) return null

  const { capacity, burst_pool } = data

  return (
    <div className="space-y-6">
      <div className="grid grid-cols-1 md:grid-cols-4 gap-4">
        <Card className="bg-surface-100 border-control shadow-sm">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Total Projects</CardTitle>
            <Database className="h-4 w-4 text-brand" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{capacity.project_count}</div>
            <p className="text-xs text-foreground-lighter uppercase tracking-wider">Provisioned projects</p>
          </CardContent>
        </Card>
        <Card className="bg-surface-100 border-control shadow-sm">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Global CPU</CardTitle>
            <Cpu className="h-4 w-4 text-brand" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{capacity.used_cpu.toFixed(1)} / {capacity.total_cpu}</div>
            <p className="text-xs text-foreground-lighter uppercase tracking-wider">Allocated cores</p>
          </CardContent>
        </Card>
        <Card className="bg-surface-100 border-control shadow-sm">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">System RAM</CardTitle>
            <Activity className="h-4 w-4 text-brand" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{(capacity.used_memory_mb / 1024).toFixed(1)}GB / {(capacity.total_memory_mb / 1024).toFixed(0)}GB</div>
            <p className="text-xs text-foreground-lighter uppercase tracking-wider">Allocated memory</p>
          </CardContent>
        </Card>
        <Card className="bg-surface-100 border-control shadow-sm">
          <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
            <CardTitle className="text-sm font-medium">Burst Status</CardTitle>
            <Server className="h-4 w-4 text-brand" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold">{burst_pool.active_bursts}</div>
            <p className="text-xs text-foreground-lighter uppercase tracking-wider">Projects currently bursting</p>
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card className="border-control shadow-sm">
          <CardHeader>
            <CardTitle>System Load</CardTitle>
            <CardDescription>Global resource allocation across all projects</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col items-center justify-center p-6 space-y-8">
            <div className="grid grid-cols-1 md:grid-cols-2 gap-12 w-full">
              <ResourceGauge 
                value={capacity.utilization_percent} 
                label="Overall Load"
                displayValue={`${capacity.used_memory_mb}MB`}
                unit=" Used"
              />
              <ResourceGauge 
                value={(capacity.used_cpu / capacity.total_cpu) * 100} 
                label="CPU Allocation"
                displayValue={capacity.used_cpu.toFixed(1)}
                unit=" Cores"
              />
            </div>
            <div className="w-full space-y-2 pt-4">
              <div className="flex justify-between text-xs text-foreground-lighter uppercase tracking-wider">
                <span>Free Memory</span>
                <span className="font-mono">{capacity.free_memory_mb} MB</span>
              </div>
              <div className="relative h-2 w-full overflow-hidden rounded-full bg-surface-200">
                <div 
                  className="h-full bg-brand transition-all duration-500"
                  style={{ width: `${capacity.utilization_percent}%` }}
                />
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="border-control shadow-sm">
          <CardHeader>
            <div className="flex items-center justify-between">
              <div>
                <CardTitle>Burst Pool Status</CardTitle>
                <CardDescription>Shared RAM pool for project elasticity</CardDescription>
              </div>
              <Badge variant={burst_pool.utilization_percent > 80 ? 'destructive' : 'default'}>
                {burst_pool.utilization_percent.toFixed(1)}% Busy
              </Badge>
            </div>
          </CardHeader>
          <CardContent className="space-y-6">
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1">
                <p className="text-xs text-foreground-lighter uppercase tracking-wider">Total Pool</p>
                <p className="text-xl font-bold">{burst_pool.total_pool_mb} MB</p>
              </div>
              <div className="space-y-1">
                <p className="text-xs text-foreground-lighter uppercase tracking-wider">Available</p>
                <p className="text-xl font-bold text-emerald-500">{burst_pool.free_pool_mb} MB</p>
              </div>
              <div className="space-y-1">
                <p className="text-xs text-foreground-lighter uppercase tracking-wider">In Use</p>
                <p className="text-xl font-bold text-brand">{burst_pool.used_pool_mb} MB</p>
              </div>
              <div className="space-y-1">
                <p className="text-xs text-foreground-lighter uppercase tracking-wider">Eligible Projects</p>
                <p className="text-xl font-bold">{burst_pool.eligible_count}</p>
              </div>
            </div>

            <div className="space-y-2 pt-4 border-t border-control">
              <div className="flex justify-between text-xs text-foreground-lighter uppercase tracking-wider">
                <span>Pool Utilization</span>
                <span>{burst_pool.utilization_percent.toFixed(1)}%</span>
              </div>
              <div className="relative h-3 w-full overflow-hidden rounded-full bg-surface-200">
                <div 
                  className="h-full bg-brand transition-all duration-500"
                  style={{ width: `${burst_pool.utilization_percent}%` }}
                />
              </div>
              <p className="text-xs text-foreground-lighter italic mt-2">
                The burst pool allows projects to exceed their guaranteed memory reservation when the server has idle capacity.
              </p>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
