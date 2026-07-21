import { TriangleAlert } from 'lucide-react'

import { useI18n, type TKey } from '@/app/i18n'
import type { Turn } from '@/lib/session-turns'
import {
  TurnFailureStage,
  TurnReason,
  TurnStatus,
} from '@/gen/koda/v1/service_pb'

const FAILURE_LOCATION_KEYS = {
  [TurnFailureStage.UNSPECIFIED]: '' as const,
  [TurnFailureStage.AGENT]: 'session.turn.failure.location.agent' as const,
  [TurnFailureStage.PROVIDER]:
    'session.turn.failure.location.provider' as const,
  [TurnFailureStage.TOOL]: 'session.turn.failure.location.tool' as const,
  [TurnFailureStage.PERSISTENCE]:
    'session.turn.failure.location.storage' as const,
  [TurnFailureStage.CONSUMER]: 'session.turn.failure.location.client' as const,
}

function interruptionLabel(
  t: ReturnType<typeof useI18n>['t'],
  reason: TurnReason,
) {
  switch (reason) {
    case TurnReason.CANCELED:
      return t('session.turn.interruption.canceled')
    case TurnReason.DEADLINE_EXCEEDED:
      return t('session.turn.interruption.deadline')
    case TurnReason.CONSUMER_STOPPED:
      return t('session.turn.interruption.consumer')
    case TurnReason.ABANDONED:
      return t('session.turn.interruption.abandoned')
    default:
      return t('session.turn.interruption.default')
  }
}

function failureLabel(
  t: ReturnType<typeof useI18n>['t'],
  code = '',
  stage = TurnFailureStage.UNSPECIFIED,
) {
  const locationKey = FAILURE_LOCATION_KEYS[stage]
  const location = locationKey ? t(locationKey as TKey) : ''
  const normalizedCode = code.replaceAll('_', ' ').trim()
  if (normalizedCode && location) return `${normalizedCode} · ${location}`
  if (normalizedCode) return normalizedCode
  return location
    ? t('session.turn.failure.inLocation', { location })
    : t('session.turn.failure.generic')
}

export function TurnStatusView({ turn }: { turn: Turn }) {
  const { t } = useI18n()
  const metadata = turn.metadata
  if (
    !metadata ||
    (metadata.status !== TurnStatus.FAILED &&
      metadata.status !== TurnStatus.INTERRUPTED)
  ) {
    return null
  }
  const failed = metadata.status === TurnStatus.FAILED
  const title = failed
    ? t('session.turn.failed')
    : t('session.turn.interrupted')
  const detail = failed
    ? metadata.failure?.message ||
      failureLabel(t, metadata.failure?.code, metadata.failure?.stage)
    : interruptionLabel(t, metadata.reason)
  return (
    <div
      className="ml-9 flex items-start gap-3 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm"
      role="status"
    >
      <TriangleAlert className="mt-0.5 size-4 shrink-0 text-destructive" />
      <div>
        <div className="font-medium text-foreground">{title}</div>
        <div className="mt-0.5 text-xs text-muted-foreground">{detail}</div>
      </div>
    </div>
  )
}
