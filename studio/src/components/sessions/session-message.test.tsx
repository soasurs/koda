import { create } from '@bufbuild/protobuf'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { EventView } from '@/components/sessions/session-message'
import { EventSchema, Role } from '@/gen/koda/v1/service_pb'

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
})
