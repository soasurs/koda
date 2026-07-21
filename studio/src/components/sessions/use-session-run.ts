import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import type {
  CompactionProgress,
  Event,
  Part,
  QuestionPrompt,
  Session,
  ToolApproval,
  RunResponse,
} from '@/gen/koda/v1/service_pb'
import {
  AgentMode,
  EventSchema,
  PartSchema,
  Role,
  RunState,
} from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'
import type { ComposerInput } from '@/lib/composer-attachments'
import {
  attachmentToPart,
  emptyComposerInput,
  isComposerInputEmpty,
  revokeAttachments,
} from '@/lib/composer-attachments'
import { errorMessage, kodaKeys, replaceSession } from '@/lib/koda'
import {
  inputToComposerInput,
  mergeConversationEvents,
} from '@/lib/session-turns'

export function useSessionRun(sessionId: string, persistedEvents: Event[]) {
  const queryClient = useQueryClient()
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const abortRef = useRef<AbortController | null>(null)
  const activeRunIdRef = useRef('')
  const lastSequenceRef = useRef(0n)
  const terminalRef = useRef(false)
  const [composerInput, setComposerInput] = useState<{
    revision: number
    value: ComposerInput
  }>({ revision: 0, value: emptyComposerInput })
  const [mode, setMode] = useState(AgentMode.BUILD)
  const [isRunning, setIsRunning] = useState(false)
  const [isCheckingRun, setIsCheckingRun] = useState(true)
  const [rewindingTurnId, setRewindingTurnId] = useState('')
  const [runError, setRunError] = useState('')
  const [optimisticUserEvent, setOptimisticUserEvent] = useState<Event | null>(
    null,
  )
  const hasOptimisticUserEvent = optimisticUserEvent !== null
  const [liveEvents, setLiveEvents] = useState<Event[]>([])
  const [partialReasoning, setPartialReasoning] = useState('')
  const [partialText, setPartialText] = useState('')
  const [compactionProgress, setCompactionProgress] =
    useState<CompactionProgress | null>(null)
  const [approvals, setApprovals] = useState<ToolApproval[]>([])
  const [questionPrompts, setQuestionPrompts] = useState<QuestionPrompt[]>([])

  useEffect(
    () => () => {
      abortRef.current?.abort()
    },
    [],
  )

  const invalidateSession = useCallback(async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: kodaKeys.events(sessionId) }),
      queryClient.invalidateQueries({ queryKey: kodaKeys.session(sessionId) }),
      queryClient.invalidateQueries({ queryKey: kodaKeys.sessions }),
    ])
  }, [queryClient, sessionId])

  const handleFrame = useCallback(
    (frame: RunResponse) => {
      if (frame.sequence > 0n && frame.sequence <= lastSequenceRef.current) {
        return
      }
      if (frame.runId) activeRunIdRef.current = frame.runId
      if (frame.sequence > lastSequenceRef.current) {
        lastSequenceRef.current = frame.sequence
      }
      if (frame.payload.case !== 'terminated') setRunError('')
      switch (frame.payload.case) {
        case 'event': {
          const event = frame.payload.value
          if (event.turnId) {
            setOptimisticUserEvent((current) =>
              current && current.turnId !== event.turnId
                ? create(EventSchema, {
                    sessionId: current.sessionId,
                    turnId: event.turnId,
                    author: current.author,
                    createdAt: current.createdAt,
                    message: current.message,
                  })
                : current,
            )
          }
          if (event.partial) {
            if (event.message?.reasoning) {
              setPartialReasoning(
                (current) => current + event.message!.reasoning,
              )
            }
            if (event.message?.text) {
              setPartialText((current) => current + event.message!.text)
            }
          } else {
            setLiveEvents((current) =>
              current.some(({ id }) => id && id === event.id)
                ? current
                : [...current, event],
            )
            if (event.message?.role === Role.ASSISTANT) {
              setPartialReasoning('')
              setPartialText('')
            }
          }
          break
        }
        case 'approval': {
          const approval = frame.payload.value
          setApprovals((current) =>
            current.some(({ id }) => id === approval.id)
              ? current
              : [...current, approval],
          )
          break
        }
        case 'questionPrompt': {
          const prompt = frame.payload.value
          setQuestionPrompts((current) =>
            current.some(({ id }) => id === prompt.id)
              ? current
              : [...current, prompt],
          )
          break
        }
        case 'compactionProgress':
          setCompactionProgress(frame.payload.value)
          break
        case 'completed': {
          terminalRef.current = true
          const completedSession = frame.payload.value.session
          if (completedSession) {
            queryClient.setQueryData(
              kodaKeys.session(sessionId),
              completedSession,
            )
            queryClient.setQueryData<Session[]>(kodaKeys.sessions, (sessions) =>
              replaceSession(sessions, completedSession),
            )
          }
          break
        }
        case 'terminated':
          terminalRef.current = true
          if (frame.payload.value.state === RunState.FAILED) {
            setRunError(frame.payload.value.failure?.message || 'Run failed')
          }
          break
        case 'interactionResolved': {
          const resolved = frame.payload.value.interaction
          if (resolved.case === 'approvalId') {
            setApprovals((current) =>
              current.filter(({ id }) => id !== resolved.value),
            )
          } else if (resolved.case === 'questionPromptId') {
            setQuestionPrompts((current) =>
              current.filter(({ id }) => id !== resolved.value),
            )
          }
          break
        }
      }
    },
    [queryClient, sessionId],
  )

  const watchUntilTerminal = useCallback(
    async (runId: string, abortController: AbortController) => {
      let watchedRunId = runId
      while (!terminalRef.current && !abortController.signal.aborted) {
        try {
          const stream = kodaClient.watchRun(
            {
              runId: watchedRunId,
              afterSequence: lastSequenceRef.current,
            },
            { signal: abortController.signal },
          )
          for await (const response of stream) {
            if (response.frame) handleFrame(response.frame)
          }
        } catch (error) {
          if (abortController.signal.aborted) return
          if (ConnectError.from(error).code === Code.NotFound) {
            const active = await kodaClient.getActiveRun(
              { sessionId },
              { signal: abortController.signal },
            )
            if (!active.run) {
              terminalRef.current = true
              return
            }
            if (active.run.runId !== watchedRunId) {
              watchedRunId = active.run.runId
              activeRunIdRef.current = watchedRunId
              lastSequenceRef.current = 0n
              setApprovals(active.run.approvals)
              setQuestionPrompts(active.run.questionPrompts)
            }
          } else if (!isRetriable(error)) {
            throw error
          }
        }
        if (!terminalRef.current) {
          await retryDelay(abortController.signal)
        }
      }
    },
    [handleFrame, sessionId],
  )

  useEffect(() => {
    const abortController = new AbortController()
    abortRef.current = abortController
    void (async () => {
      try {
        let response
        for (;;) {
          try {
            response = await kodaClient.getActiveRun(
              { sessionId },
              { signal: abortController.signal },
            )
            setRunError('')
            break
          } catch (error) {
            if (abortController.signal.aborted) return
            if (!isRetriable(error)) throw error
            setRunError(errorMessage(error))
            await retryDelay(abortController.signal)
          }
        }
        const active = response.run
        setIsCheckingRun(false)
        if (!active) return
        activeRunIdRef.current = active.runId
        lastSequenceRef.current = 0n
        terminalRef.current = false
        setApprovals(active.approvals)
        setQuestionPrompts(active.questionPrompts)
        setIsRunning(true)
        await watchUntilTerminal(active.runId, abortController)
        await invalidateSession()
        setLiveEvents([])
      } catch (error) {
        if (!abortController.signal.aborted) setRunError(errorMessage(error))
      } finally {
        if (!abortController.signal.aborted) {
          setIsCheckingRun(false)
          setIsRunning(false)
          setCompactionProgress(null)
          setApprovals([])
          setQuestionPrompts([])
          activeRunIdRef.current = ''
          abortRef.current = null
        }
      }
    })()
    return () => abortController.abort()
  }, [invalidateSession, sessionId, watchUntilTerminal])

  async function run(input: ComposerInput) {
    if (isRunning || isCheckingRun) return
    if (isComposerInputEmpty(input)) return

    const parts: Part[] = []
    const trimmedText = input.text.trim()
    if (trimmedText) {
      parts.push(
        create(PartSchema, {
          content: { case: 'text' as const, value: trimmedText },
        }),
      )
    }
    for (const att of input.attachments) {
      parts.push(attachmentToPart(att))
    }

    setComposerInput((current) => ({
      revision: current.revision + 1,
      value: emptyComposerInput,
    }))
    setRunError('')
    setOptimisticUserEvent(
      create(EventSchema, {
        sessionId,
        turnId: `pending-${sessionId}`,
        author: 'user',
        createdAt: BigInt(Date.now()),
        message: { role: Role.USER, text: trimmedText, parts },
      }),
    )
    setLiveEvents([])
    setPartialReasoning('')
    setPartialText('')
    setCompactionProgress(null)
    setIsRunning(true)
    const abortController = new AbortController()
    abortRef.current = abortController
    activeRunIdRef.current = ''
    lastSequenceRef.current = 0n
    terminalRef.current = false

    const request = {
      sessionId,
      input: { parts },
      mode,
      clientRequestId: crypto.randomUUID(),
    }

    try {
      while (!terminalRef.current && !abortController.signal.aborted) {
        try {
          const stream = kodaClient.run(request, {
            signal: abortController.signal,
          })
          for await (const frame of stream) handleFrame(frame)
        } catch (error) {
          if (abortController.signal.aborted) return
          if (terminalRef.current) break
          setRunError(errorMessage(error))
          if (!isRetriable(error)) {
            if (!activeRunIdRef.current) setOptimisticUserEvent(null)
            terminalRef.current = true
            break
          }
        }
        if (terminalRef.current) break
        if (activeRunIdRef.current) {
          await watchUntilTerminal(activeRunIdRef.current, abortController)
        } else {
          await retryDelay(abortController.signal)
        }
      }

      if (activeRunIdRef.current) {
        await invalidateSession()
        setLiveEvents([])
      }
    } catch (error) {
      setOptimisticUserEvent(null)
      setLiveEvents([])
      setPartialReasoning('')
      setPartialText('')
      if (activeRunIdRef.current) await invalidateSession()
      if (!abortController.signal.aborted) {
        setRunError(errorMessage(error))
      }
    } finally {
      revokeAttachments(input.attachments)
      if (!abortController.signal.aborted) {
        setIsRunning(false)
        setCompactionProgress(null)
        setApprovals([])
        setQuestionPrompts([])
        abortRef.current = null
        activeRunIdRef.current = ''
        requestAnimationFrame(() => inputRef.current?.focus())
      }
    }
  }

  async function rewindLastTurn(turnId: string, retry: boolean) {
    if (isRunning || isCheckingRun || rewindingTurnId) return
    setRewindingTurnId(turnId)
    setRunError('')
    try {
      const response = await kodaClient.undoLastMessage({
        sessionId,
        expectedTurnId: turnId,
      })
      if (response.turnId !== turnId) {
        throw new Error('Koda removed a different turn; reload before retrying')
      }
      const restored = inputToComposerInput(response.input)
      setComposerInput((current) => ({
        revision: current.revision + 1,
        value: restored,
      }))
      await invalidateSession()
      if (retry) {
        await run(restored)
      } else {
        requestAnimationFrame(() => inputRef.current?.focus())
      }
    } catch (error) {
      setRunError(errorMessage(error))
    } finally {
      setRewindingTurnId('')
    }
  }

  async function editLastTurn(turnId: string, newInput: ComposerInput) {
    if (isRunning || isCheckingRun || rewindingTurnId) return
    if (isComposerInputEmpty(newInput)) return
    setRewindingTurnId(turnId)
    setRunError('')
    try {
      const response = await kodaClient.undoLastMessage({
        sessionId,
        expectedTurnId: turnId,
      })
      if (response.turnId !== turnId) {
        throw new Error('Koda removed a different turn; reload before retrying')
      }
      const previous = inputToComposerInput(response.input)
      revokeAttachments(previous.attachments)
      await invalidateSession()
      await run(newInput)
    } catch (error) {
      setRunError(errorMessage(error))
    } finally {
      setRewindingTurnId('')
    }
  }

  const events = useMemo(
    () =>
      mergeConversationEvents(
        persistedEvents,
        liveEvents,
        optimisticUserEvent ?? undefined,
      ),
    [persistedEvents, liveEvents, optimisticUserEvent],
  )

  const clearApproval = useCallback(
    (approvalId: string) =>
      setApprovals((current) =>
        current.filter((approval) => approval.id !== approvalId),
      ),
    [],
  )
  const clearQuestionPrompt = useCallback(
    (promptId: string) =>
      setQuestionPrompts((current) =>
        current.filter((prompt) => prompt.id !== promptId),
      ),
    [],
  )
  const stop = useCallback(() => {
    void (async () => {
      let runId = activeRunIdRef.current
      if (!runId) {
        const active = await kodaClient.getActiveRun({ sessionId })
        runId = active.run?.runId ?? ''
      }
      if (runId) await kodaClient.cancelRun({ runId })
    })().catch((error: unknown) => setRunError(errorMessage(error)))
  }, [sessionId])

  const runRef = useRef(run)
  useEffect(() => {
    runRef.current = run
  })
  const stableRun = useCallback((input: ComposerInput): void => {
    runRef.current(input)
  }, [])

  return {
    approvals,
    clearApproval,
    clearQuestionPrompt,
    compactionProgress,
    editLastTurn,
    events,
    inputRef,
    inputRevision: composerInput.revision,
    isRunning: isRunning || isCheckingRun,
    mode,
    partialReasoning,
    partialText,
    questionPrompts,
    hasOptimisticUserEvent,
    restoredInput: composerInput.value,
    rewindLastTurn,
    rewindingTurnId,
    run,
    runError,
    runStable: stableRun,
    setMode,
    stop,
  }
}

function retryDelay(signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = window.setTimeout(resolve, 250)
    signal.addEventListener(
      'abort',
      () => {
        window.clearTimeout(timer)
        resolve()
      },
      { once: true },
    )
  })
}

function isRetriable(error: unknown): boolean {
  switch (ConnectError.from(error).code) {
    case Code.Unknown:
    case Code.Canceled:
    case Code.DeadlineExceeded:
    case Code.Aborted:
    case Code.Unavailable:
      return true
    default:
      return false
  }
}
