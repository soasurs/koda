import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { useSessionRun } from '@/components/sessions/use-session-run'
import type { RunResponse } from '@/gen/koda/v1/service_pb'
import {
  CompactionProgressSchema,
  CompactionProgressStage,
  InputSchema,
  QuestionPromptSchema,
  RunResponseSchema,
  RunCompletedSchema,
  RunSnapshotSchema,
  ToolApprovalSchema,
  UndoLastMessageResponseSchema,
  WatchRunResponseSchema,
} from '@/gen/koda/v1/service_pb'

const {
  cancelRunMock,
  getActiveRunMock,
  runMock,
  undoLastMessageMock,
  watchRunMock,
} = vi.hoisted(() => ({
  cancelRunMock: vi.fn(),
  getActiveRunMock: vi.fn(),
  runMock: vi.fn(),
  undoLastMessageMock: vi.fn(),
  watchRunMock: vi.fn(),
}))

vi.mock('@/lib/connect', () => ({
  kodaClient: {
    cancelRun: cancelRunMock,
    getActiveRun: getActiveRunMock,
    run: runMock,
    undoLastMessage: undoLastMessageMock,
    watchRun: watchRunMock,
  },
}))

afterEach(() => {
  cancelRunMock.mockReset()
  getActiveRunMock.mockReset()
  runMock.mockReset()
  undoLastMessageMock.mockReset()
  watchRunMock.mockReset()
})

describe('useSessionRun interactions', () => {
  it('reattaches to an active run and stops it explicitly', async () => {
    getActiveRunMock.mockResolvedValue({
      run: create(RunSnapshotSchema, {
        runId: 'run-1',
        sessionId: 'session-1',
        approvals: [create(ToolApprovalSchema, { id: 'approval-1' })],
      }),
    })
    const stream = controlledWatchStream([])
    watchRunMock.mockReturnValue(stream.responses)
    cancelRunMock.mockResolvedValue({})
    const { result, unmount } = renderSessionRun(false)

    await waitFor(() => {
      expect(result.current.isRunning).toBe(true)
      expect(result.current.approvals[0]?.id).toBe('approval-1')
    })
    act(() => result.current.stop())
    expect(cancelRunMock).toHaveBeenCalledWith({ runId: 'run-1' })

    unmount()
    expect(cancelRunMock).toHaveBeenCalledTimes(1)
    stream.finish()
  })

  it('retries a failed watch stream from the last sequence', async () => {
    getActiveRunMock.mockResolvedValue({
      run: create(RunSnapshotSchema, {
        runId: 'run-1',
        sessionId: 'session-1',
      }),
    })
    const recovered = controlledWatchStream([approvalFrame('approval-1')])
    watchRunMock
      .mockReturnValueOnce(failingStream())
      .mockReturnValueOnce(recovered.responses)
    const { result, unmount } = renderSessionRun(false)

    await waitFor(() => {
      expect(watchRunMock).toHaveBeenCalledTimes(2)
      expect(result.current.approvals[0]?.id).toBe('approval-1')
    })
    expect(watchRunMock.mock.calls[1]?.[0]).toEqual({
      runId: 'run-1',
      afterSequence: 0n,
    })

    unmount()
    recovered.finish()
  })

  it('does not retry a permanent admission error', async () => {
    runMock.mockReturnValue(
      failingStream(new ConnectError('session is busy', Code.AlreadyExists)),
    )
    const { result } = renderSessionRun()
    await waitFor(() => expect(result.current.isRunning).toBe(false))

    await act(async () => result.current.run('hello'))

    expect(runMock).toHaveBeenCalledTimes(1)
    expect(result.current.runError).toContain('session is busy')
  })

  it('keeps concurrent approvals until each one is resolved', async () => {
    const stream = controlledStream([
      approvalFrame('approval-1'),
      approvalFrame('approval-2'),
      approvalFrame('approval-3'),
    ])
    runMock.mockReturnValue(stream.responses)
    const { result } = renderSessionRun()
    await waitFor(() => expect(result.current.isRunning).toBe(false))

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
    await waitFor(() => expect(result.current.isRunning).toBe(false))

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

  it('tracks compaction progress while the run is active', async () => {
    const stream = controlledStream([
      compactionFrame(CompactionProgressStage.STARTED),
      compactionFrame(CompactionProgressStage.COMPLETED),
    ])
    runMock.mockReturnValue(stream.responses)
    const { result } = renderSessionRun()
    await waitFor(() => expect(result.current.isRunning).toBe(false))

    let runPromise: Promise<void>
    act(() => {
      runPromise = result.current.run('continue')
    })
    await waitFor(() => {
      expect(result.current.compactionProgress?.stage).toBe(
        CompactionProgressStage.COMPLETED,
      )
      expect(result.current.compactionProgress?.generation).toBe(2n)
    })

    stream.finish()
    await act(async () => runPromise)
    expect(result.current.compactionProgress).toBeNull()
  })

  it('removes an interrupted turn before running its edited message', async () => {
    undoLastMessageMock.mockResolvedValue(
      create(UndoLastMessageResponseSchema, {
        turnId: 'turn-interrupted',
        deletedEventCount: 2n,
        input: create(InputSchema, {
          parts: [{ content: { case: 'text', value: 'original message' } }],
        }),
      }),
    )
    runMock.mockReturnValue(finishedStream())
    const { result } = renderSessionRun()
    await waitFor(() => expect(result.current.isRunning).toBe(false))

    await act(async () => {
      await result.current.editLastTurn('turn-interrupted', 'edited message')
    })

    expect(undoLastMessageMock).toHaveBeenCalledWith({
      sessionId: 'session-1',
      expectedTurnId: 'turn-interrupted',
    })
    expect(runMock).toHaveBeenCalledWith(
      expect.objectContaining({
        sessionId: 'session-1',
        input: {
          parts: [{ content: { case: 'text', value: 'edited message' } }],
        },
      }),
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
    expect(undoLastMessageMock.mock.invocationCallOrder[0]).toBeLessThan(
      runMock.mock.invocationCallOrder[0],
    )
  })
})

function renderSessionRun(defaultActiveRun = true) {
  if (defaultActiveRun) getActiveRunMock.mockResolvedValue({})
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

function compactionFrame(stage: CompactionProgressStage) {
  return create(RunResponseSchema, {
    payload: {
      case: 'compactionProgress',
      value: create(CompactionProgressSchema, {
        stage,
        generation: 2n,
        contextTokens: 208_000n,
        sourceTokens: 192_000n,
        estimatedTokensAfter: 32_000n,
      }),
    },
  })
}

function controlledStream(frames: RunResponse[]) {
  let finish = () => {}
  const done = new Promise<void>((resolve) => {
    finish = resolve
  })
  async function* responses() {
    for (const frame of frames) yield frame
    await done
    yield completedFrame()
  }
  return { finish, responses: responses() }
}

async function* finishedStream() {
  yield completedFrame()
}

async function* failingStream(
  error: Error = new Error('temporary disconnect'),
): AsyncGenerator<never> {
  yield await Promise.reject(error)
}

function completedFrame() {
  return create(RunResponseSchema, {
    payload: {
      case: 'completed',
      value: create(RunCompletedSchema),
    },
  })
}

function controlledWatchStream(frames: RunResponse[]) {
  const stream = controlledStream(frames)
  async function* responses() {
    for await (const frame of stream.responses) {
      yield create(WatchRunResponseSchema, { frame })
    }
  }
  return { finish: stream.finish, responses: responses() }
}
