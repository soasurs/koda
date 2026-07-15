import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { SessionPage } from '@/pages/session-page'

const route = vi.hoisted(() => ({
  history: undefined as
    | {
        events: {
          id: string
          sessionId: string
          turnId: string
        }[]
        undoableTurnId: string
        compaction?: {
          generation: bigint
          compactedEventCount: bigint
          sourceTokens: bigint
          estimatedTokensAfter: bigint
          modelId: string
          createdAt: bigint
        }
      }
    | undefined,
  sessionId: 'session-1',
}))

vi.mock('@tanstack/react-router', () => ({
  useParams: () => ({ sessionId: route.sessionId }),
}))

vi.mock('@tanstack/react-query', () => ({
  useQuery: ({ queryKey }: { queryKey: string[] }) =>
    queryKey[0] === 'session'
      ? {
          data: {
            id: route.sessionId,
            title: route.sessionId,
            workdir: '/workspace',
          },
          isError: false,
          isPending: false,
        }
      : {
          data: route.history ?? { events: [], undoableTurnId: '' },
          isError: false,
          isPending: false,
        },
}))

vi.mock('@/components/sessions/use-session-run', async () => {
  const { useState } = await import('react')
  return {
    useSessionRun: (
      sessionId: string,
      persistedEvents: { id: string; sessionId: string; turnId: string }[],
    ) => {
      const [sourceSessionId] = useState(sessionId)
      return {
        approvals: [],
        clearApproval: vi.fn(),
        clearQuestionPrompt: vi.fn(),
        editLastTurn: vi.fn(),
        events:
          persistedEvents.length > 0
            ? persistedEvents
            : [
                {
                  id: 'event-1',
                  sessionId: sourceSessionId,
                  turnId: 'turn-1',
                },
              ],
        inputRef: { current: null },
        inputRevision: 0,
        isRunning: false,
        mode: 1,
        partialReasoning: '',
        partialText: '',
        questionPrompts: [],
        restoredInput: '',
        rewindLastTurn: vi.fn(),
        rewindingTurnId: '',
        run: vi.fn(),
        runError: '',
        setMode: vi.fn(),
        stop: vi.fn(),
      }
    },
  }
})

vi.mock('@/components/sessions/session-composer', () => ({
  SessionComposer: () => null,
}))

vi.mock('@/components/sessions/session-header', () => ({
  SessionHeader: () => null,
}))

vi.mock('@/components/sessions/session-turn', () => ({
  SessionTurn: ({
    canRevise,
    turn,
  }: {
    canRevise: boolean
    turn: { id: string; events: { sessionId: string }[] }
  }) => (
    <div data-can-revise={canRevise} data-testid={`turn-${turn.id}`}>
      {turn.events[0]?.sessionId}
    </div>
  ),
}))

vi.mock('@/components/sessions/use-follow-latest', () => ({
  useFollowLatest: () => ({
    containerRef: { current: null },
    onScroll: vi.fn(),
  }),
}))

describe('SessionPage', () => {
  beforeEach(() => {
    route.history = undefined
    route.sessionId = 'session-1'
  })

  it('remounts transient run state when the route session changes', () => {
    const view = render(<SessionPage />)
    expect(screen.getByTestId('turn-turn-1')).toHaveTextContent('session-1')

    route.sessionId = 'session-2'
    view.rerender(<SessionPage />)

    expect(screen.getByTestId('turn-turn-1')).toHaveTextContent('session-2')
    view.unmount()
  })

  it('shows the compaction boundary and only revises the active tail', () => {
    route.history = {
      events: [
        {
          id: 'event-1',
          sessionId: 'session-1',
          turnId: 'turn-1',
        },
        {
          id: 'event-2',
          sessionId: 'session-1',
          turnId: 'turn-2',
        },
      ],
      undoableTurnId: 'turn-2',
      compaction: {
        generation: 3n,
        compactedEventCount: 1n,
        sourceTokens: 200_000n,
        estimatedTokensAfter: 12_000n,
        modelId: 'gpt-5.6',
        createdAt: 1_784_025_600_000n,
      },
    }

    render(<SessionPage />)

    expect(screen.getByTestId('compaction-boundary')).toHaveTextContent(
      'generation 3',
    )
    expect(screen.getByTestId('turn-turn-1')).toHaveAttribute(
      'data-can-revise',
      'false',
    )
    expect(screen.getByTestId('turn-turn-2')).toHaveAttribute(
      'data-can-revise',
      'true',
    )
  })
})
