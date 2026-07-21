import { Check, LoaderCircle, TriangleAlert } from 'lucide-react'

import { useI18n } from '@/app/i18n'
import type { CompactionProgress } from '@/gen/koda/v1/service_pb'
import { CompactionProgressStage } from '@/gen/koda/v1/service_pb'
import { useTokenFormatter } from '@/lib/use-token-formatter'

export function CompactionActivity({
  progress,
}: {
  progress: CompactionProgress
}) {
  const { t, locale } = useI18n()
  const completed = progress.stage === CompactionProgressStage.COMPLETED
  const failed = progress.stage === CompactionProgressStage.FAILED
  const tokenFormatter = useTokenFormatter(locale)
  const detail = completed
    ? t('session.compaction.detail.completed', {
        sourceTokens: tokenFormatter.format(progress.sourceTokens),
        estimatedTokens: tokenFormatter.format(progress.estimatedTokensAfter),
      })
    : failed
      ? t('session.compaction.continuing')
      : t('session.compaction.detail.inProgress', {
          contextTokens: tokenFormatter.format(progress.contextTokens),
        })

  return (
    <div
      className="ml-9 flex items-center gap-3 text-sm text-muted-foreground"
      data-testid="compaction-progress"
      role="status"
    >
      {completed ? (
        <Check className="size-4 text-foreground" />
      ) : failed ? (
        <TriangleAlert className="size-4" />
      ) : (
        <LoaderCircle className="size-4 animate-spin" />
      )}
      <div>
        <div className="font-medium text-foreground">
          {completed
            ? t('session.compaction.completed', {
                generation: String(progress.generation),
              })
            : failed
              ? t('session.compaction.failed')
              : t('session.compaction.inProgress')}
        </div>
        <div className="mt-0.5 text-xs">{detail}</div>
      </div>
    </div>
  )
}
