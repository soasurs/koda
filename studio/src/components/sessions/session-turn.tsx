import { LoaderCircle, Pencil, RotateCcw } from 'lucide-react'
import { memo, useMemo } from 'react'

import { useI18n } from '@/app/i18n'
import { Button } from '@/components/ui/button'
import { CopyButton } from '@/components/ui/copy-button'
import { EventView } from '@/components/sessions/session-message'
import { InlineEditComposer } from '@/components/sessions/inline-edit-composer'
import { TurnStatusView } from '@/components/sessions/turn-status-view'
import { EarlierActivityDetails } from '@/components/sessions/earlier-activity-details'
import { ActivityView } from '@/components/sessions/activity-view'
import { Role } from '@/gen/koda/v1/service_pb'
import type { ComposerInput } from '@/lib/composer-attachments'
import {
  eventText,
  groupTurnActivities,
  inputToComposerInput,
  type Turn,
} from '@/lib/session-turns'

export const SessionTurn = memo(function SessionTurn({
  canRevise,
  isEditing,
  isRunning,
  isRewinding,
  onEditCancel,
  onEditStart,
  onEditSubmit,
  onRetry,
  turn,
}: {
  canRevise: boolean
  isEditing: boolean
  isRunning: boolean
  isRewinding: boolean
  onEditCancel: () => void
  onEditStart: () => void
  onEditSubmit: (input: ComposerInput) => void
  onRetry: (input: ComposerInput) => void
  turn: Turn
}) {
  const { t } = useI18n()
  const userEvents = turn.events.filter(
    (event) => event.message?.role === Role.USER,
  )
  const activities = groupTurnActivities(turn.events)
  const earlierActivities = activities.slice(0, -1)
  const finalActivity = activities.at(-1)
  const lastAssistantEvent = [...turn.events]
    .reverse()
    .find(
      (event) =>
        event.message?.role === Role.ASSISTANT && eventText(event).trim(),
    )
  const lastAssistantText = lastAssistantEvent
    ? eventText(lastAssistantEvent)
    : ''
  const initialEditInput = useMemo(() => {
    const lastUserEvent = [...turn.events]
      .reverse()
      .find((event) => event.message?.role === Role.USER)
    return lastUserEvent
      ? inputToComposerInput(
          { parts: lastUserEvent.message?.parts ?? [] },
          lastUserEvent.message?.text ?? '',
        )
      : { text: '', attachments: [] }
  }, [turn.events])

  return (
    <section className="space-y-6 border-b border-border/70 pb-8 last:border-b-0">
      {isEditing ? (
        <InlineEditComposer
          initialInput={initialEditInput}
          onCancel={onEditCancel}
          onSubmit={onEditSubmit}
        />
      ) : (
        userEvents.map((event, index) => (
          <EventView
            event={event}
            key={event.id || `${turn.id}-user-${index}`}
          />
        ))
      )}
      {canRevise && !isEditing && (
        <div className="-mt-3 flex justify-end gap-1">
          <Button
            aria-label={t('session.turn.editMessage')}
            disabled={isRewinding}
            onClick={onEditStart}
            size="icon"
            title={t('session.turn.editMessage')}
            variant="ghost"
          >
            <Pencil className="size-3.5" />
          </Button>
        </div>
      )}
      {isRunning ? (
        activities.map((activity, index) => (
          <ActivityView
            activity={activity}
            key={activity.assistant.id || `${turn.id}-activity-${index}`}
          />
        ))
      ) : (
        <>
          {earlierActivities.length > 0 && (
            <EarlierActivityDetails activities={earlierActivities} />
          )}
          {finalActivity && <ActivityView activity={finalActivity} />}
        </>
      )}
      {!isRunning && <TurnStatusView turn={turn} />}
      {canRevise && !isEditing && (
        <div className="-mt-3 ml-9 flex items-center gap-1">
          <CopyButton text={lastAssistantText} />
          <Button
            aria-label={t('session.turn.retryTurn')}
            disabled={isRewinding}
            onClick={() => onRetry(initialEditInput)}
            size="icon"
            title={t('session.turn.retryTurn')}
            variant="ghost"
          >
            {isRewinding ? (
              <LoaderCircle className="size-3.5 animate-spin" />
            ) : (
              <RotateCcw className="size-3.5" />
            )}
          </Button>
        </div>
      )}
    </section>
  )
}, areTurnPropsEqual)

function areTurnPropsEqual(prev: TurnProps, next: TurnProps): boolean {
  return (
    prev.turn.id === next.turn.id &&
    prev.turn.events.length === next.turn.events.length &&
    prev.turn.events[prev.turn.events.length - 1] ===
      next.turn.events[next.turn.events.length - 1] &&
    prev.turn.metadata === next.turn.metadata &&
    prev.canRevise === next.canRevise &&
    prev.isEditing === next.isEditing &&
    prev.isRunning === next.isRunning &&
    prev.isRewinding === next.isRewinding
  )
}

type TurnProps = {
  canRevise: boolean
  isEditing: boolean
  isRunning: boolean
  isRewinding: boolean
  onEditCancel: () => void
  onEditStart: () => void
  onEditSubmit: (input: ComposerInput) => void
  onRetry: (input: ComposerInput) => void
  turn: Turn
}
