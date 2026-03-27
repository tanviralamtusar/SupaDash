import { cn } from 'ui'

interface ResourceGaugeProps {
  label: string
  value: number
  unit?: string
  displayValue?: string | number
  className?: string
}

export const ResourceGauge = ({ label, value, unit, displayValue, className }: ResourceGaugeProps) => {
  // Simple indicator color based on value
  const getStatusColor = (val: number) => {
    if (val > 90) return 'bg-red-500'
    if (val > 75) return 'bg-amber-500'
    return 'bg-emerald-500'
  }

  return (
    <div className={cn('space-y-2', className)}>
      <div className="flex justify-between items-end">
        <span className="text-xs font-medium text-foreground-lighter uppercase tracking-wider">{label}</span>
        <span className="text-sm font-mono font-medium tabular-nums">
          {displayValue ?? Math.round(value)}
          {unit ?? '%'}
        </span>
      </div>
      <div className="relative h-2 w-full overflow-hidden rounded-full bg-surface-200 dark:bg-surface-300">
        <div
          className={cn('h-full transition-all duration-500 ease-in-out', getStatusColor(value))}
          style={{ width: `${Math.min(100, Math.max(0, value))}%` }}
        />
      </div>
    </div>
  )
}
