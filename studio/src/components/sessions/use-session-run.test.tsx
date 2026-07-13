import { create } from '@bufbuild/protobuf'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { useSessionRun } from '@/components/sessions/use-session-run'
import {
  QuestionPromptSchema,
  RunResponseSchema,
  ToolApprovalSchema,
} from '@/gen/koda/v1/service_pb'

const { runMock } = vi.hoisted(() => ({ runMock: vi.fn() }))

vi.mock('@/lib/connect', () => ({
  kodaClient: { run: runMock },
}))

afterEach(() => {
  runMock.mockReset()
})

describe('useSessionRun interactions', () => {
  it('keeps concurrent approvals until each one is resolved', async () => {
    const stream = controlledStream([
      approvalFrame('approval-1'),
      approvalFrame('approval-2'),
      approvalFrame('approval-3'),
    ])
    runMock.mockReturnValue(stream.responses)
    const { result } = renderSessionRun()

    let runPromise: Promise<void>
    act(() => {
      runPromise = result.current.run('check the workspace')
    })
    await waitFor(() => {
      expect(result.current.approvals.map(({ id }) => id)).toEqual([
        'approval-1',
        'approval-2',
        'approval-3',
      ])
    })

    act(() => result.current.clearApproval('approval-2'))
    expect(result.current.approvals.map(({ id }) => id)).toEqual([
      'approval-1',
      'approval-3',
    ])

    stream.finish()
    await act(async () => runPromise)
  })

  it('keeps concurrent question prompts until each one is resolved', async () => {
    const stream = controlledStream([
      questionFrame('prompt-1'),
      questionFrame('prompt-2'),
    ])
    runMock.mockReturnValue(stream.responses)
    const { result } = renderSessionRun()

    let runPromise: Promise<void>
    act(() => {
      runPromise = result.current.run('ask me questions')
    })
    await waitFor(() => {
      expect(result.current.questionPrompts.map(({ id }) => id)).toEqual([
        'prompt-1',
        'prompt-2',
      ])
    })

    act(() => result.current.clearQuestionPrompt('prompt-1'))
    expect(result.current.questionPrompts.map(({ id }) => id)).toEqual([
      'prompt-2',
    ])

    stream.finish()
    await act(async () => runPromise)
  })
})

function renderSessionRun() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  )
  return renderHook(() => useSessionRun('session-1', []), { wrapper })
}

function approvalFrame(id: string) {
  return create(RunResponseSchema, {
    payload: {
      case: 'approval',
      value: create(ToolApprovalSchema, { id }),
    },
  })
}

function questionFrame(id: string) {
  return create(RunResponseSchema, {
    payload: {
      case: 'questionPrompt',
      value: create(QuestionPromptSchema, { id }),
    },
  })
}

function controlledStream(frames: ReturnType<typeof approvalFrame>[]) {
  let finish = () => {}
  const done = new Promise<void>((resolve) => {
    finish = resolve
  })
  async function* responses() {
    for (const frame of frames) yield frame
    await done
  }
  return { finish, responses: responses() }
}
