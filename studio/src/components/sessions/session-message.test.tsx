import { create } from '@bufbuild/protobuf'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { EventView } from '@/components/sessions/session-message'
import { EventSchema, ImageSchema, Role } from '@/gen/koda/v1/service_pb'
import { ImageDetail } from '@/gen/koda/v1/service_pb'

vi.mock('@/components/markdown-text', () => ({
  default: ({ text }: { text: string }) => <span>{text}</span>,
}))

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

describe('EventView', () => {
  it('shows a localized timestamp for a durable message', () => {
    const date = new Date(2026, 6, 15, 14, 32)
    vi.useFakeTimers()
    vi.setSystemTime(new Date(2026, 6, 15, 18, 0))

    const { container } = render(
      <EventView
        event={create(EventSchema, {
          createdAt: BigInt(date.getTime()),
          message: { role: Role.USER, text: 'hello' },
        })}
      />,
    )

    expect(screen.getByText('hello')).toBeInTheDocument()
    expect(container.querySelector('time')).toHaveAttribute(
      'datetime',
      date.toISOString(),
    )
    expect(container.querySelector('time')).toHaveTextContent(
      new Intl.DateTimeFormat(undefined, {
        hour: '2-digit',
        minute: '2-digit',
      }).format(date),
    )
  })

  it('does not show a timestamp for a transient message', () => {
    const { container } = render(
      <EventView
        event={create(EventSchema, {
          message: { role: Role.ASSISTANT, text: 'streaming' },
        })}
      />,
    )

    expect(container.querySelector('time')).not.toBeInTheDocument()
  })

  it('renders image parts alongside text for user messages', async () => {
    const { container } = render(
      <EventView
        event={create(EventSchema, {
          id: 'event-1',
          message: {
            role: Role.USER,
            text: 'look at this',
            parts: [
              { content: { case: 'text', value: 'look at this' } },
              {
                content: {
                  case: 'image',
                  value: create(ImageSchema, {
                    source: {
                      case: 'data',
                      value: new Uint8Array([0x89, 0x50, 0x4e, 0x47]),
                    },
                    mimeType: 'image/png',
                    detail: ImageDetail.AUTO,
                  }),
                },
              },
            ],
          },
        })}
      />,
    )

    expect(screen.getByText('look at this')).toBeInTheDocument()
    await waitFor(() => {
      const img = container.querySelector('img')
      expect(img).toBeInTheDocument()
      expect(img?.getAttribute('src')).toMatch(/^data:image\/png;base64,/)
    })
  })

  it('renders only an image when the user message has no text', async () => {
    const { container } = render(
      <EventView
        event={create(EventSchema, {
          id: 'event-2',
          message: {
            role: Role.USER,
            parts: [
              {
                content: {
                  case: 'image',
                  value: create(ImageSchema, {
                    source: {
                      case: 'data',
                      value: new Uint8Array([1, 2, 3]),
                    },
                    mimeType: 'image/png',
                    detail: ImageDetail.AUTO,
                  }),
                },
              },
            ],
          },
        })}
      />,
    )

    await waitFor(() => {
      expect(container.querySelector('img')).toBeInTheDocument()
    })
    expect(container.textContent).not.toContain('undefined')
  })
})
