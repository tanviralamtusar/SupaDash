import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from 'ui'
import { formatBytes } from 'lib/helpers'

interface ServiceUsageTableProps {
  snapshots: any[]
}

export const ServiceUsageTable = ({ snapshots }: ServiceUsageTableProps) => {
  // Group latest snapshots by service name
  const latestByService = snapshots.reduce((acc: any, s: any) => {
    if (!acc[s.service_name] || new Date(s.timestamp) > new Date(acc[s.service_name].timestamp)) {
      acc[s.service_name] = s
    }
    return acc
  }, {})

  const services = Object.values(latestByService).sort((a: any, b: any) => 
    a.service_name.localeCompare(b.service_name)
  )

  return (
    <div className="rounded-md border border-control overflow-hidden">
      <Table>
        <TableHeader className="bg-surface-100">
          <TableRow>
            <TableHead className="w-[200px] py-2">Service</TableHead>
            <TableHead className="py-2">Status</TableHead>
            <TableHead className="text-right py-2">CPU</TableHead>
            <TableHead className="text-right py-2">Memory</TableHead>
            <TableHead className="text-right py-2">Restarts</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {services.map((s: any) => {
            const cpuVal = s.cpu_usage_percent?.Float64 ?? s.cpu_usage_percent ?? 0
            const memVal = s.memory_usage_bytes?.Int64 ?? s.memory_usage_bytes ?? 0
            
            return (
              <TableRow key={s.service_name} className="hover:bg-surface-100 transition-colors">
                <TableCell className="font-medium py-2">{s.service_name}</TableCell>
                <TableCell className="py-2">
                  <div className="flex items-center gap-2">
                    <div className={`h-2 w-2 rounded-full ${s.container_status === 'running' ? 'bg-emerald-500' : 'bg-surface-300'}`} />
                    <span className="capitalize text-xs text-foreground-light">{s.container_status}</span>
                    {s.oom_killed && (
                      <span className="text-[10px] bg-red-100 text-red-600 px-1 rounded border border-red-200 uppercase font-bold">OOM</span>
                    )}
                  </div>
                </TableCell>
                <TableCell className="text-right font-mono text-xs py-2">
                  {cpuVal.toFixed(1)}%
                </TableCell>
                <TableCell className="text-right font-mono text-xs py-2">
                  {formatBytes(memVal)}
                </TableCell>
                <TableCell className="text-right font-mono text-xs py-2">
                  {s.restart_count ?? 0}
                </TableCell>
              </TableRow>
            )
          })}
          {services.length === 0 && (
            <TableRow>
              <TableCell colSpan={5} className="text-center py-8 text-foreground-lighter italic">
                No active services detected in this project
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  )
}
