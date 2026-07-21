import { Archive } from 'lucide-react'

import { useI18n } from '@/app/i18n'
import type { CompactionStatus } from '@/gen/koda/v1/service_pb'
import { useTokenFormatter } from '@/lib/use-token-formatter'

export function CompactionBoundary({
  compaction,
}: {
  compaction: CompactionStatus
}) {
  const { t, locale } = useI18n()
  const createdAt = new Date(Number(compaction.createdAt))
  const tokenFormatter = useTokenFormatter(locale)
  const title = [
    t('session.compaction.boundary.titleGeneration', {
      generation: String(compaction.generation),
    }),
    t('session.compaction.boundary.titleEvents', {
      count: String(compaction.compactedEventCount),
    }),
    t('session.compaction.boundary.titleSourceTokens', {
      count: tokenFormatter.format(compaction.sourceTokens),
    }),
    t('session.compaction.boundary.titleEstimatedTokens', {
      count: tokenFormatter.format(compaction.estimatedTokensAfter),
    }),
    compaction.modelId
      ? t('session.compaction.boundary.titleModel', {
          modelId: compaction.modelId,
        })
      : '',
    Number.isNaN(createdAt.getTime()) ? '' : createdAt.toLocaleString(),
  ]
    .filter(Boolean)
    .join(' · ')

  return (
    <div
      className="flex items-center gap-3 py-1 text-xs text-muted-foreground"
      data-testid="compaction-boundary"
      title={title}
    >
      <div className="h-px flex-1 bg-border" />
      <span className="flex items-center gap-1.5 whitespace-nowrap rounded-full border border-border bg-muted/50 px-2.5 py-1">
        <Archive className="size-3" />
        {t('session.compaction.boundary.label', {
          generation: String(compaction.generation),
        })}
      </span>
      <div className="h-px flex-1 bg-border" />
    </div>
  )
}
