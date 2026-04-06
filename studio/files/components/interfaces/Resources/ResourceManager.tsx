import React, { useState, useEffect } from 'react'
import { 
  Button, 
  Card, 
  CardContent, 
  CardDescription, 
  CardHeader, 
  CardTitle,
  Alert_Shadcn_ as Alert,
  AlertTitle_Shadcn_ as AlertTitle,
  AlertDescription_Shadcn_ as AlertDescription,
  LoadingLine,
  Slider,
  Separator,
} from 'ui'
import { Cpu, HardDrive, Layout, RefreshCw, Zap } from 'lucide-react'
import { useProjectAnalysisQuery, useProjectResourcesQuery, useUpdateProjectResourcesMutation } from 'data/resources/project-resources-query'
import { ResourceGauge } from './ResourceGauge'
import { ServiceUsageTable } from './ServiceUsageTable'
import { formatBytes } from 'lib/helpers'
import { toast } from 'sonner'

interface ResourceManagerProps {
  projectRef: string
}

const PLANS = [
  { id: 'free', name: 'Free', cpu: 0.25, memory: 512, description: 'Shared resources, best for development' },
  { id: 'pro', name: 'Pro', cpu: 1.0, memory: 2048, description: 'Dedicated resources for production' },
  { id: 'enterprise', name: 'Enterprise', cpu: 4.0, memory: 8192, description: 'High-performance dedicated infrastructure' },
]

export const ResourceManager = ({ projectRef: ref }: ResourceManagerProps) => {
  const { data: resources, isLoading: isLoadingResources } = useProjectResourcesQuery({ projectRef: ref })
  const { data: analysis, isLoading: isLoadingAnalysis } = useProjectAnalysisQuery({ projectRef: ref })
  const { mutate: updateResources, isLoading: isUpdating } = useUpdateProjectResourcesMutation()

  const [selectedPlan, setSelectedPlan] = useState<string>('free')
  const [cpuLimit, setCpuLimit] = useState<number>(0.25)
  const [memoryLimit, setMemoryLimit] = useState<number>(512)

  useEffect(() => {
    if (resources) {
      setCpuLimit(resources.cpu_limit)
      setMemoryLimit(resources.memory_limit_mb)
      setSelectedPlan(resources.plan || 'free')
    }
  }, [resources])

  const handleSave = () => {
    updateResources(
      {
        projectRef: ref,
        plan: selectedPlan,
        cpu_limit: cpuLimit,
        memory_limit_mb: memoryLimit,
      },
      {
        onSuccess: () => {
          toast.success('Resource limits updated successfully')
        },
        onError: (error: any) => {
          toast.error(`Failed to update resources: ${error.message}`)
        },
      }
    )
  }

  const snapshots = analysis?.snapshots ?? []
  const recommendations = analysis?.recommendations ?? []

  // Calculate current aggregates
  const latestSnapshots = snapshots.reduce((acc: any, s: any) => {
    if (!acc[s.service_name] || new Date(s.timestamp) > new Date(acc[s.service_name].timestamp)) {
      acc[s.service_name] = s
    }
    return acc
  }, {})

  const totalCPU = Object.values(latestSnapshots).reduce((sum: number, s: any) => sum + (s.cpu_usage_percent?.Float64 ?? s.cpu_usage_percent ?? 0), 0)
  const totalMemory = Object.values(latestSnapshots).reduce((sum: number, s: any) => sum + (s.memory_usage_bytes?.Int64 ?? s.memory_usage_bytes ?? 0), 0)

  const cpuLimitFromResources = resources?.cpu_limit ?? 1
  const memoryLimitBytes = (resources?.memory_limit_mb ?? 1024) * 1024 * 1024

  const cpuUsagePercent = (totalCPU / (cpuLimitFromResources * 100)) * 100
  const memoryUsagePercent = (totalMemory / memoryLimitBytes) * 100

  if (isLoadingResources || isLoadingAnalysis) {
    return (
      <div className="flex flex-col gap-4 p-4">
        <LoadingLine />
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-8">
      {/* Overview Stats */}
      <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
        <Card className="bg-surface-100 shadow-sm border-control">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <Cpu className="h-4 w-4 text-brand" />
              CPU Usage (Total)
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ResourceGauge
              label={`${totalCPU.toFixed(1)}% of ${cpuLimitFromResources} vCPU`}
              value={cpuUsagePercent}
            />
          </CardContent>
        </Card>

        <Card className="bg-surface-100 shadow-sm border-control">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <RefreshCw className="h-4 w-4 text-brand" />
              Memory Usage (Total)
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ResourceGauge
              label={`${formatBytes(totalMemory)} of ${formatBytes(memoryLimitBytes)}`}
              value={memoryUsagePercent}
            />
          </CardContent>
        </Card>

        <Card className="bg-surface-100 shadow-sm border-control">
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium flex items-center gap-2">
              <Layout className="h-4 w-4 text-brand" />
              Active Plan
            </CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex flex-col gap-1">
              <span className="text-2xl font-bold text-foreground capitalize">{resources?.plan?.toLowerCase()}</span>
              <span className="text-xs text-foreground-lighter uppercase tracking-wider">
                {resources?.burst_eligible ? 'Burst Eligible' : 'Fixed Quota'}
              </span>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Recommendations */}
      {recommendations.length > 0 && (
        <div className="flex flex-col gap-4">
          <h3 className="text-lg font-medium">Recommendations</h3>
          <div className="grid grid-cols-1 gap-4">
            {recommendations.map((rec: any) => (
              <Alert key={rec.id} variant={rec.severity === 'critical' ? 'destructive' : 'default'} className="bg-surface-100 border-control">
                <Zap className="h-4 w-4" />
                <AlertTitle className="font-bold">{rec.title}</AlertTitle>
                <AlertDescription className="flex flex-col gap-2">
                  <p>{rec.description}</p>
                  <div className="flex gap-2 mt-2">
                    <Button size="tiny" type="default">View details</Button>
                    <Button size="tiny" type="outline">Dismiss</Button>
                  </div>
                </AlertDescription>
              </Alert>
            ))}
          </div>
        </div>
      )}

      {/* Scaling Controls */}
      <Card className="border-control shadow-sm">
        <CardHeader>
          <CardTitle>Scaling & Limits</CardTitle>
          <CardDescription>Adjust your project hardware on the fly</CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
            {PLANS.map((plan) => (
              <div
                key={plan.id}
                className={`p-4 border rounded-lg cursor-pointer transition-colors ${
                  selectedPlan === plan.id ? 'border-primary bg-primary/10' : 'hover:bg-muted'
                }`}
                onClick={() => {
                  setSelectedPlan(plan.id)
                  setCpuLimit(plan.cpu)
                  setMemoryLimit(plan.memory)
                }}
              >
                <p className="font-bold">{plan.name}</p>
                <p className="text-xs text-muted-foreground">{plan.description}</p>
              </div>
            ))}
          </div>

          <Separator />

          <div className="space-y-4">
            <div className="space-y-2">
              <div className="flex justify-between items-center">
                <label className="text-sm font-medium">CPU Limit (Cores)</label>
                <span className="text-sm font-mono">{cpuLimit} Cores</span>
              </div>
              <Slider
                value={[cpuLimit]}
                min={0.1}
                max={8}
                step={0.1}
                onValueChange={(val) => {
                  setCpuLimit(val[0])
                  setSelectedPlan('custom')
                }}
              />
            </div>

            <div className="space-y-2">
              <div className="flex justify-between items-center">
                <label className="text-sm font-medium">Memory Limit (MB)</label>
                <span className="text-sm font-mono">{memoryLimit} MB</span>
              </div>
              <Slider
                value={[memoryLimit]}
                min={128}
                max={16384}
                step={128}
                onValueChange={(val) => {
                  setMemoryLimit(val[0])
                  setSelectedPlan('custom')
                }}
              />
            </div>
          </div>

          <div className="flex justify-end pt-4">
            <Button
              loading={isUpdating}
              onClick={handleSave}
              disabled={cpuLimit === resources?.cpu_limit && memoryLimit === resources?.memory_limit_mb && selectedPlan === resources?.plan}
            >
              Apply Changes
            </Button>
          </div>
        </CardContent>
      </Card>

      {/* Service Breakdown */}
      <Card className="border-control shadow-sm">
        <CardHeader>
          <CardTitle>Service Breakdown</CardTitle>
          <CardDescription>Real-time usage per project container</CardDescription>
        </CardHeader>
        <CardContent>
          <ServiceUsageTable snapshots={analysis?.snapshots || []} />
        </CardContent>
      </Card>
    </div>
  )
}

export default ResourceManager
