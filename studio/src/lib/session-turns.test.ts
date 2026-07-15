import { create } from '@bufbuild/protobuf'
import { describe, expect, it } from 'vitest'

import {
  EventSchema,
  InputSchema,
  Role,
  ToolCallSchema,
} from '@/gen/koda/v1/service_pb'
import {
  eventText,
  groupEventsByTurn,
  groupTurnActivities,
  inputText,
  mergeConversationEvents,
  toolCallPresentation,
} from '@/lib/session-turns'

describe('session turn helpers', () => {
  it('groups conversation events by turn in encounter order', () => {
    const events = [
      create(EventSchema, {
        id: '1',
        turnId: 'turn-a',
        message: { role: Role.USER, text: 'first' },
      }),
      create(EventSchema, {
        id: '2',
        turnId: 'turn-a',
        message: { role: Role.ASSISTANT, text: 'response' },
      }),
      create(EventSchema, {
        id: '3',
        turnId: 'turn-b',
        message: { role: Role.USER, text: 'second' },
      }),
    ]

    const turns = groupEventsByTurn(events)

    expect(turns.map((turn) => turn.id)).toEqual(['turn-a', 'turn-b'])
    expect(turns[0]?.events).toHaveLength(2)
    expect(eventText(turns[0]?.events[0])).toBe('first')
  })

  it('shows an optimistic user event until its persisted event arrives', () => {
    const optimistic = create(EventSchema, {
      turnId: 'turn-a',
      message: { role: Role.USER, text: 'first message' },
    })
    const assistant = create(EventSchema, {
      id: '2',
      turnId: 'turn-a',
      message: { role: Role.ASSISTANT, text: 'response' },
    })

    expect(mergeConversationEvents([], [assistant], optimistic)).toEqual([
      optimistic,
      assistant,
    ])

    const persisted = create(EventSchema, {
      id: '1',
      turnId: 'turn-a',
      message: { role: Role.USER, text: 'first message' },
    })
    expect(
      mergeConversationEvents([persisted, assistant], [assistant], optimistic),
    ).toEqual([persisted, assistant])
  })

  it('restores text parts from an undone input', () => {
    const input = create(InputSchema, {
      parts: [
        { content: { case: 'text', value: 'one' } },
        { content: { case: 'text', value: 'two' } },
      ],
    })

    expect(inputText(input)).toBe('one\ntwo')
  })

  it('associates tool results with the preceding assistant step', () => {
    const events = [
      create(EventSchema, {
        id: '1',
        turnId: 'turn-a',
        message: { role: Role.ASSISTANT, text: 'inspect' },
      }),
      create(EventSchema, {
        id: '2',
        turnId: 'turn-a',
        message: { role: Role.TOOL },
      }),
      create(EventSchema, {
        id: '3',
        turnId: 'turn-a',
        message: { role: Role.ASSISTANT, text: 'continue' },
      }),
      create(EventSchema, {
        id: '4',
        turnId: 'turn-a',
        message: { role: Role.TOOL },
      }),
    ]

    const activities = groupTurnActivities(events)

    expect(activities).toHaveLength(2)
    expect(eventText(activities[0]?.assistant)).toBe('inspect')
    expect(activities[0]?.tools).toHaveLength(1)
    expect(eventText(activities[1]?.assistant)).toBe('continue')
    expect(activities[1]?.tools).toHaveLength(1)
  })

  it('creates a friendly tool label and concise detail', () => {
    expect(
      toolCallPresentation(
        create(ToolCallSchema, {
          name: 'read_file',
          argumentsJson: JSON.stringify({ path: 'src/app.tsx', maxChars: 200 }),
        }),
      ),
    ).toEqual({ label: 'Read file', detail: 'src/app.tsx' })

    expect(
      toolCallPresentation(
        create(ToolCallSchema, {
          name: 'web_fetch',
          argumentsJson: JSON.stringify({
            url: 'https://example.com/docs',
            maxChars: 200,
          }),
        }),
        true,
      ),
    ).toEqual({
      label: 'Fetched web page',
      detail: 'https://example.com/docs',
    })

    expect(
      toolCallPresentation(
        create(ToolCallSchema, {
          name: 'custom_tool',
          argumentsJson: '{}',
        }),
      ),
    ).toEqual({ label: 'Custom Tool', detail: '' })
  })
})
