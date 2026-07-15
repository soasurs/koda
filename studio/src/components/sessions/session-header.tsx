import type { Session } from '@/gen/koda/v1/service_pb'
import { SidebarExpandButton } from '@/components/layout/sidebar-expand-button'

export function SessionHeader({ session }: { session: Session }) {
  return (
    <header className="flex h-12 shrink-0 items-center gap-3 border-b border-border px-4 sm:px-6">
      <SidebarExpandButton />
      <div className="min-w-0 flex-1">
        <h1 className="truncate text-sm font-medium">
          {session.title || session.workdir.split('/').pop() || 'Untitled'}
        </h1>
      </div>
      <ContextWindowUsage session={session} />
    </header>
  )
}

function ContextWindowUsage({ session }: { session: Session }) {
  const usage = session.contextUsage
  if (!usage || usage.windowTokens <= 0n) return null

  if (!usage.measured) {
    return (
      <div
        className="shrink-0 text-xs text-muted-foreground"
        title={`Context usage unavailable · ${formatTokens(usage.windowTokens)} window`}
      >
        Context — / {formatTokens(usage.windowTokens)}
      </div>
    )
  }

  const used = usage.usedTokens < 0n ? 0n : usage.usedTokens
  const remaining = used >= usage.windowTokens ? 0n : usage.windowTokens - used
  const percentage = Math.round(
    (Number(used) / Number(usage.windowTokens)) * 100,
  )
  const meterPercentage = Math.min(Math.max(percentage, 0), 100)
  const label = `${formatExactTokens(used)} tokens used, ${formatExactTokens(remaining)} remaining, ${percentage}% of ${formatExactTokens(usage.windowTokens)}`

  return (
    <div
      aria-label={label}
      aria-valuemax={100}
      aria-valuemin={0}
      aria-valuenow={meterPercentage}
      className="flex shrink-0 items-center gap-2"
      role="meter"
      title={label}
    >
      <div className="hidden h-1.5 w-20 overflow-hidden rounded-full bg-muted sm:block">
        <div
          className="h-full rounded-full bg-primary transition-[width]"
          style={{ width: `${meterPercentage}%` }}
        />
      </div>
      <span className="whitespace-nowrap text-xs text-muted-foreground">
        {formatTokens(used)} used · {formatTokens(remaining)} left ·{' '}
        {percentage}% of {formatTokens(usage.windowTokens)}
      </span>
    </div>
  )
}

const compactTokenFormatter = new Intl.NumberFormat('en-US', {
  notation: 'compact',
  maximumFractionDigits: 1,
})

const exactTokenFormatter = new Intl.NumberFormat('en-US')

function formatTokens(tokens: bigint) {
  return compactTokenFormatter.format(tokens)
}

function formatExactTokens(tokens: bigint) {
  return exactTokenFormatter.format(tokens)
}
