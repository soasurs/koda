import type {
  Event,
  Input,
  Part,
  Turn as DurableTurn,
} from '@/gen/koda/v1/service_pb'
import { Role } from '@/gen/koda/v1/service_pb'

import {
  bytesToDataURL,
  type ComposerAttachment,
  type ComposerInput,
} from '@/lib/composer-attachments'

export type Turn = { id: string; events: Event[]; metadata?: DurableTurn }
export type TurnActivity = { assistant: Event; tools: Event[] }

export function groupEventsByTurn(
  events: Event[],
  durableTurns: DurableTurn[] = [],
): Turn[] {
  const turns: Turn[] = durableTurns.map((metadata) => ({
    id: metadata.id,
    events: [],
    metadata,
  }))
  const turnIndexes = new Map<string, number>()
  durableTurns.forEach((turn, index) => turnIndexes.set(turn.id, index))
  for (const event of events) {
    const turnId = event.turnId || `event-${event.id}`
    let index = turnIndexes.get(turnId)
    if (index === undefined) {
      index = turns.length
      turnIndexes.set(turnId, index)
      turns.push({ id: event.turnId, events: [] })
    }
    turns[index].events.push(event)
  }
  return turns
}

export function mergeConversationEvents(
  persistedEvents: Event[],
  liveEvents: Event[],
  optimisticUserEvent?: Event,
): Event[] {
  const persistedEventIDs = new Set(
    persistedEvents.map((event) => event.id).filter(Boolean),
  )
  const hasPersistedUserEvent = Boolean(
    optimisticUserEvent?.turnId &&
    persistedEvents.some(
      (event) =>
        event.turnId === optimisticUserEvent.turnId &&
        event.message?.role === Role.USER,
    ),
  )

  return [
    ...persistedEvents,
    ...(optimisticUserEvent && !hasPersistedUserEvent
      ? [optimisticUserEvent]
      : []),
    ...liveEvents.filter(
      (event) => !event.id || !persistedEventIDs.has(event.id),
    ),
  ]
}

export function groupTurnActivities(events: Event[]): TurnActivity[] {
  const activities: TurnActivity[] = []
  for (const event of events) {
    if (event.message?.role === Role.ASSISTANT) {
      activities.push({ assistant: event, tools: [] })
    } else if (event.message?.role === Role.TOOL) {
      activities.at(-1)?.tools.push(event)
    }
  }
  return activities
}

export function eventText(event?: Event) {
  const message = event?.message
  if (!message) return ''
  return (
    message.parts
      .filter((part) => part.content.case === 'text')
      .map((part) => (part.content.case === 'text' ? part.content.value : ''))
      .join('\n') || message.text
  )
}

export function inputText(input?: Input) {
  return (
    input?.parts
      .filter((part) => part.content.case === 'text')
      .map((part) => (part.content.case === 'text' ? part.content.value : ''))
      .join('\n') ?? ''
  )
}

export function eventParts(event?: Event): Part[] {
  return event?.message?.parts ?? []
}

export function inputToComposerInput(
  input?: { parts: Part[] } | null,
  fallbackText = '',
): ComposerInput {
  if (!input) return { text: fallbackText, attachments: [] }
  const text: string[] = []
  const attachments: ComposerAttachment[] = []
  for (const part of input.parts) {
    switch (part.content.case) {
      case 'text':
        text.push(part.content.value)
        break
      case 'image': {
        const image = part.content.value
        if (image.source.case === 'data') {
          attachments.push({
            id: crypto.randomUUID(),
            mimeType: image.mimeType || 'application/octet-stream',
            data: image.source.value,
            previewUrl: bytesToDataURL(
              image.source.value,
              image.mimeType || 'application/octet-stream',
            ),
            name: 'image',
          })
        }
        break
      }
    }
  }
  return { text: text.join('\n') || fallbackText, attachments }
}
