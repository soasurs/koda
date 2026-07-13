import type { Event, Input, ToolCall } from '@/gen/koda/v1/service_pb'
import { Role } from '@/gen/koda/v1/service_pb'

export type Turn = { id: string; events: Event[] }
export type TurnActivity = { assistant: Event; tools: Event[] }

export function groupEventsByTurn(events: Event[]): Turn[] {
  const turns: Turn[] = []
  const turnIndexes = new Map<string, number>()
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

const toolLabels: Record<string, string> = {
  ask_questions: 'Ask questions',
  create_file: 'Create file',
  edit_file: 'Edit file',
  find_files: 'Find files',
  list_directory: 'List directory',
  read_file: 'Read file',
  run_shell: 'Run command',
  search_text: 'Search text',
  write_file: 'Write file',
}

export function toolCallPresentation(toolCall: ToolCall) {
  return toolPresentation(toolCall.name, toolCall.argumentsJson)
}

export function toolPresentation(name: string, argumentsJson: string) {
  const label =
    toolLabels[name] ??
    name
      .split(/[_-]+/)
      .filter(Boolean)
      .map((part) => part[0]?.toUpperCase() + part.slice(1))
      .join(' ')

  let detail = ''
  try {
    const input = JSON.parse(argumentsJson) as Record<string, unknown>
    const value =
      input.path ?? input.command ?? input.pattern ?? input.query ?? input.globs
    if (Array.isArray(value)) detail = value.join(', ')
    else if (typeof value === 'string') detail = value
  } catch {
    // A malformed argument payload should not prevent the call from rendering.
  }

  return { label, detail }
}
