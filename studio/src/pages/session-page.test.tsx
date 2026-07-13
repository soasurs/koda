import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { SessionPage } from '@/pages/session-page'

const route = vi.hoisted(() => ({ sessionId: 'session-1' }))

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
      : { data: [], isError: false, isPending: false },
}))

vi.mock('@/components/sessions/use-session-run', async () => {
  const { useState } = await import('react')
  return {
    useSessionRun: (sessionId: string) => {
      const [sourceSessionId] = useState(sessionId)
      return {
        approvals: [],
        clearApproval: vi.fn(),
        clearQuestionPrompt: vi.fn(),
        editLastTurn: vi.fn(),
        events: [
          {
            id: 'event-1',
            sessionId: sourceSessionId,
            turnId: 'turn-1',
          },
        ],
        inputRef: { current: null },
        inputRevision: 0,
        isRunning: true,
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
  SessionTurn: ({ turn }: { turn: { events: { sessionId: string }[] } }) => (
    <div data-testid="turn-session">{turn.events[0]?.sessionId}</div>
  ),
}))

vi.mock('@/components/sessions/use-follow-latest', () => ({
  useFollowLatest: () => ({
    containerRef: { current: null },
    onScroll: vi.fn(),
  }),
}))

describe('SessionPage', () => {
  it('remounts transient run state when the route session changes', () => {
    const view = render(<SessionPage />)
    expect(screen.getByTestId('turn-session')).toHaveTextContent('session-1')

    route.sessionId = 'session-2'
    view.rerender(<SessionPage />)

    expect(screen.getByTestId('turn-session')).toHaveTextContent('session-2')
  })
})
