import { useMemo } from 'react'

import { useI18n } from '@/app/i18n'
import type { Session } from '@/gen/koda/v1/service_pb'
import { SidebarExpandButton } from '@/components/layout/sidebar-expand-button'

export function SessionHeader({ session }: { session: Session }) {
  const { t } = useI18n()
  return (
    <header className="flex h-12 shrink-0 items-center gap-3 border-b border-border px-4 sm:px-6">
      <SidebarExpandButton />
      <div className="min-w-0 flex-1">
        <h1 className="truncate text-sm font-medium">
          {session.title ||
            session.workdir.split('/').pop() ||
            t('session.header.untitled')}
        </h1>
      </div>
      <ContextWindowUsage session={session} />
    </header>
  )
}

function useTokenFormatters(locale: string) {
  return useMemo(
    () => ({
      compact: new Intl.NumberFormat(locale, {
        notation: 'compact',
        maximumFractionDigits: 1,
      }),
      exact: new Intl.NumberFormat(locale),
    }),
    [locale],
  )
}

function ContextWindowUsage({ session }: { session: Session }) {
  const { t, locale } = useI18n()
  const { compact: compactFormatter, exact: exactFormatter } =
    useTokenFormatters(locale)
  const usage = session.contextUsage
  if (!usage || usage.windowTokens <= 0n) return null

  const windowCompact = compactFormatter.format(usage.windowTokens)

  if (!usage.measured) {
    return (
      <div
        className="shrink-0 text-xs text-muted-foreground"
        title={t('session.header.contextUsageUnavailable', {
          window: windowCompact,
        })}
      >
        {t('session.header.contextShort', { window: windowCompact })}
      </div>
    )
  }

  const used = usage.usedTokens < 0n ? 0n : usage.usedTokens
  const remaining = used >= usage.windowTokens ? 0n : usage.windowTokens - used
  const percentage = Math.round(
    (Number(used) / Number(usage.windowTokens)) * 100,
  )
  const meterPercentage = Math.min(Math.max(percentage, 0), 100)
  const label = t('session.header.usageLabel', {
    used: exactFormatter.format(used),
    remaining: exactFormatter.format(remaining),
    percentage: String(percentage),
    window: exactFormatter.format(usage.windowTokens),
  })

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
        {t('session.header.usageSummary', {
          used: compactFormatter.format(used),
          remaining: compactFormatter.format(remaining),
          percentage: String(percentage),
          window: compactFormatter.format(usage.windowTokens),
        })}
      </span>
    </div>
  )
}
