import type {
  Event,
  Input,
  ToolCall,
  Turn as DurableTurn,
} from '@/gen/koda/v1/service_pb'
import { Role } from '@/gen/koda/v1/service_pb'
import type { TKey } from '@/app/i18n'

import { lookupMCPTool } from '@/lib/mcp-tools'
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

// --- Tool detail extraction (non-translated) ---

export function toolDetail(argumentsJson: string): string {
  try {
    const input = JSON.parse(argumentsJson) as Record<string, unknown>
    const value =
      input.name ??
      input.path ??
      input.url ??
      input.command ??
      input.pattern ??
      input.query ??
      input.globs
    if (Array.isArray(value)) return value.join(', ')
    if (typeof value === 'string') return value
    return ''
  } catch {
    return ''
  }
}

// MCP tool names are namespaced as mcp__<serverID>__<toolName>.
// mcpToolLabel returns a human-readable label for MCP tools,
// or an empty string if the name does not follow the MCP convention.
function mcpToolLabel(name: string): string {
  const meta = lookupMCPTool(name)
  if (meta)
    return `MCP ${meta.serverName} \u203A ${humanize(meta.originalName)}`
  const prefix = 'mcp__'
  if (!name.startsWith(prefix)) return ''
  const inner = name.slice(prefix.length)
  const sep = inner.indexOf('__')
  if (sep < 0) {
    // Degenerate: mcp__something — treat as a single name.
    return 'MCP ' + humanize(inner)
  }
  const server = humanize(inner.slice(0, sep))
  const tool = humanize(inner.slice(sep + 2))
  return `MCP ${server} \u203A ${tool}`
}

function humanize(value: string): string {
  return value
    .split(/[_-]+/)
    .filter(Boolean)
    .map((part) => part[0]?.toUpperCase() + part.slice(1))
    .join(' ')
}
// --- Tool label translation ---

export function toolLabel(
  t: (key: TKey, params?: Record<string, string | number>) => string,
  name: string,
  past = false,
): string {
  const presentKey = `tools.name.${name}` as TKey
  const pastKey = `tools.namePast.${name}` as TKey
  if (past) {
    const translated = t(pastKey)
    if (translated !== pastKey) return translated
  }
  const translated = t(presentKey)
  if (translated !== presentKey) return translated
  // Fallback: human-readable from the tool name
  const mcpLabel = mcpToolLabel(name)
  if (mcpLabel) return mcpLabel
  return humanize(name)
}

// --- Convenience wrappers ---

export function toolCallPresentation(
  t: (key: TKey, params?: Record<string, string | number>) => string,
  toolCall: ToolCall,
  past = false,
): { label: string; detail: string } {
  return {
    label: toolLabel(t, toolCall.name, past),
    detail: toolDetail(toolCall.argumentsJson),
  }
}

export function toolPresentation(
  t: (key: TKey, params?: Record<string, string | number>) => string,
  name: string,
  argumentsJson: string,
  past = false,
): { label: string; detail: string } {
  return {
    label: toolLabel(t, name, past),
    detail: toolDetail(argumentsJson),
  }
}
