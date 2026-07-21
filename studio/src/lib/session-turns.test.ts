import { create } from '@bufbuild/protobuf'
import { describe, expect, it } from 'vitest'
import {
  EventSchema,
  ImageSchema,
  InputSchema,
  ToolCallSchema,
  TurnSchema,
} from '@/gen/koda/v1/service_pb'
import { ImageDetail, Role, TurnStatus } from '@/gen/koda/v1/service_pb'
import { dictionaries } from '@/app/i18n/dictionaries'
import {
  eventParts,
  eventText,
  groupEventsByTurn,
  groupTurnActivities,
  inputText,
  inputToComposerInput,
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

  it('keeps durable turns that have no events', () => {
    const metadata = create(TurnSchema, {
      id: 'turn-failed',
      status: TurnStatus.FAILED,
    })

    const turns = groupEventsByTurn([], [metadata])

    expect(turns).toEqual([{ id: 'turn-failed', events: [], metadata }])
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

  it('exposes all parts of an event message', () => {
    const event = create(EventSchema, {
      id: '1',
      message: {
        role: Role.USER,
        parts: [
          { content: { case: 'text', value: 'hi' } },
          {
            content: {
              case: 'image',
              value: create(ImageSchema, {
                source: { case: 'data', value: new Uint8Array([1, 2]) },
                mimeType: 'image/png',
                detail: ImageDetail.AUTO,
              }),
            },
          },
        ],
      },
    })

    expect(eventParts(event)).toHaveLength(2)
    expect(eventParts(event)[0]?.content.case).toBe('text')
    expect(eventParts(event)[1]?.content.case).toBe('image')
  })

  it('returns an empty part list when the event has no message', () => {
    expect(eventParts(create(EventSchema, { id: '1' }))).toEqual([])
  })

  it('converts an undone input with text and image data into a composer input', () => {
    const input = create(InputSchema, {
      parts: [
        { content: { case: 'text', value: 'look at this' } },
        {
          content: {
            case: 'image',
            value: create(ImageSchema, {
              source: { case: 'data', value: new Uint8Array([9, 9, 9]) },
              mimeType: 'image/png',
              detail: ImageDetail.HIGH,
            }),
          },
        },
      ],
    })

    const composerInput = inputToComposerInput(input)
    expect(composerInput.text).toBe('look at this')
    expect(composerInput.attachments).toHaveLength(1)
    expect(composerInput.attachments[0]?.mimeType).toBe('image/png')
    expect(Array.from(composerInput.attachments[0]?.data ?? [])).toEqual([
      9, 9, 9,
    ])
    expect(composerInput.attachments[0]?.previewUrl).toMatch(
      /^data:image\/png;base64,/,
    )
    expect(composerInput.attachments[0]?.id).toBeTruthy()
  })

  it('uses fallback text when an event has no text parts', () => {
    const composerInput = inputToComposerInput({ parts: [] }, 'legacy text')
    expect(composerInput.text).toBe('legacy text')
    expect(composerInput.attachments).toEqual([])
  })

  it('drops url-based image parts when restoring a composer input', () => {
    const input = create(InputSchema, {
      parts: [
        {
          content: {
            case: 'image',
            value: create(ImageSchema, {
              source: { case: 'url', value: 'https://example.com/a.png' },
              mimeType: 'image/png',
              detail: ImageDetail.AUTO,
            }),
          },
        },
      ],
    })

    expect(inputToComposerInput(input)).toEqual({
      text: '',
      attachments: [],
    })
  })

  it('returns an empty composer input for undefined input', () => {
    expect(inputToComposerInput(undefined)).toEqual({
      text: '',
      attachments: [],
    })
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
    const t = (key: string) =>
      dictionaries.en[key as keyof typeof dictionaries.en] ?? key
    expect(
      toolCallPresentation(
        t,
        create(ToolCallSchema, {
          name: 'read_file',
          argumentsJson: JSON.stringify({ path: 'src/app.tsx', maxChars: 200 }),
        }),
      ),
    ).toEqual({ label: 'Read file', detail: 'src/app.tsx' })

    expect(
      toolCallPresentation(
        t,
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
        t,
        create(ToolCallSchema, {
          name: 'custom_tool',
          argumentsJson: '{}',
        }),
      ),
    ).toEqual({ label: 'Custom Tool', detail: '' })

    expect(
      toolCallPresentation(
        t,
        create(ToolCallSchema, {
          name: 'load_skill',
          argumentsJson: JSON.stringify({ name: 'go-pr-review' }),
        }),
      ),
    ).toEqual({ label: 'Load skill', detail: 'go-pr-review' })

    expect(
      toolCallPresentation(
        t,
        create(ToolCallSchema, {
          name: 'load_skill',
          argumentsJson: JSON.stringify({ name: 'go-pr-review' }),
        }),
        true,
      ),
    ).toEqual({ label: 'Loaded skill', detail: 'go-pr-review' })

    expect(
      toolCallPresentation(
        t,
        create(ToolCallSchema, {
          name: 'mcp__duckduckgo__search',
          argumentsJson: JSON.stringify({ query: 'golang' }),
        }),
      ),
    ).toEqual({ label: 'MCP Duckduckgo \u203A Search', detail: 'golang' })

    expect(
      toolCallPresentation(
        t,
        create(ToolCallSchema, {
          name: 'mcp__github__get_file_contents',
          argumentsJson: JSON.stringify({ path: '/README.md' }),
        }),
      ),
    ).toEqual({
      label: 'MCP Github \u203A Get File Contents',
      detail: '/README.md',
    })
  })
})
