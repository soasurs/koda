import { create } from '@bufbuild/protobuf'
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { SessionTurn } from '@/components/sessions/session-turn'
import {
  EventSchema,
  Role,
  ToolCallSchema,
  TurnFailureSchema,
  TurnFailureStage,
  TurnSchema,
  TurnStatus,
} from '@/gen/koda/v1/service_pb'
import { AllProviders } from '@/test/providers'

vi.mock('@/components/markdown-text', () => ({
  default: ({ text }: { text: string }) => <span>{text}</span>,
}))

afterEach(cleanup)

describe('SessionTurn', () => {
  it('collapses the original activity before the final assistant message', () => {
    const turn = {
      id: 'turn-1',
      events: [
        create(EventSchema, {
          id: '1',
          turnId: 'turn-1',
          message: { role: Role.USER, text: 'make a change' },
        }),
        create(EventSchema, {
          id: '2',
          turnId: 'turn-1',
          message: {
            role: Role.ASSISTANT,
            text: 'I will inspect the file.',
            toolCalls: [
              create(ToolCallSchema, {
                id: 'call-1',
                name: 'read_file',
                argumentsJson: '{"path":"src/app.tsx"}',
              }),
            ],
          },
        }),
        create(EventSchema, {
          id: '3',
          turnId: 'turn-1',
          message: {
            role: Role.TOOL,
            toolResponse: { toolCallId: 'call-1' },
          },
        }),
        create(EventSchema, {
          id: '4',
          turnId: 'turn-1',
          message: { role: Role.ASSISTANT, text: 'The change is complete.' },
        }),
      ],
    }
    const props = {
      canRevise: false,
      isEditing: false,
      isRewinding: false,
      onEditCancel: vi.fn(),
      onEditStart: vi.fn(),
      onEditSubmit: vi.fn(),
      onRetry: vi.fn(),
      turn,
    }
    const { rerender } = render(
      <AllProviders>
        <SessionTurn {...props} isRunning />
      </AllProviders>,
    )

    expect(screen.queryByText('Earlier activity')).not.toBeInTheDocument()
    expect(screen.getByText('I will inspect the file.')).toBeInTheDocument()
    expect(screen.getByText('The change is complete.')).toBeInTheDocument()

    rerender(
      <AllProviders>
        <SessionTurn {...props} isRunning={false} />
      </AllProviders>,
    )

    const earlierActivity = screen
      .getByText('Earlier activity')
      .closest('details')
    expect(earlierActivity).not.toHaveAttribute('open')
    expect(earlierActivity).toContainElement(
      screen.getByText('I will inspect the file.'),
    )
    expect(earlierActivity).toContainElement(screen.getByText('1 tool step'))
    expect(screen.getByText('The change is complete.')).not.toBeNull()
  })

  it('displays structured failure status', () => {
    const turn = {
      id: 'turn-1',
      events: [],
      metadata: create(TurnSchema, {
        id: 'turn-1',
        status: TurnStatus.FAILED,
        failure: create(TurnFailureSchema, {
          code: 'provider_unavailable',
          message: 'Provider is temporarily unavailable',
          stage: TurnFailureStage.PROVIDER,
        }),
      }),
    }

    render(
      <AllProviders>
        <SessionTurn
          canRevise={false}
          isEditing={false}
          isRewinding={false}
          isRunning={false}
          onEditCancel={vi.fn()}
          onEditStart={vi.fn()}
          onEditSubmit={vi.fn()}
          onRetry={vi.fn()}
          turn={turn}
        />
      </AllProviders>,
    )

    expect(screen.getByText('Turn failed')).toBeInTheDocument()
    expect(
      screen.getByText('Provider is temporarily unavailable'),
    ).toBeInTheDocument()
  })
})
