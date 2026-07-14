import { create } from '@bufbuild/protobuf'
import { useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import type {
  Event,
  QuestionPrompt,
  Session,
  ToolApproval,
} from '@/gen/koda/v1/service_pb'
import { AgentMode, EventSchema, Role } from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'
import { errorMessage, kodaKeys, replaceSession } from '@/lib/koda'
import { inputText, mergeConversationEvents } from '@/lib/session-turns'

export function useSessionRun(sessionId: string, persistedEvents: Event[]) {
  const queryClient = useQueryClient()
  const inputRef = useRef<HTMLTextAreaElement>(null)
  const abortRef = useRef<AbortController | null>(null)
  const [composerInput, setComposerInput] = useState({
    revision: 0,
    value: '',
  })
  const [mode, setMode] = useState(AgentMode.BUILD)
  const [isRunning, setIsRunning] = useState(false)
  const [rewindingTurnId, setRewindingTurnId] = useState('')
  const [runError, setRunError] = useState('')
  const [optimisticUserEvent, setOptimisticUserEvent] = useState<Event | null>(
    null,
  )
  const [liveEvents, setLiveEvents] = useState<Event[]>([])
  const [partialReasoning, setPartialReasoning] = useState('')
  const [partialText, setPartialText] = useState('')
  const [approvals, setApprovals] = useState<ToolApproval[]>([])
  const [questionPrompts, setQuestionPrompts] = useState<QuestionPrompt[]>([])

  useEffect(
    () => () => {
      abortRef.current?.abort()
    },
    [],
  )

  async function invalidateSession() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: kodaKeys.events(sessionId) }),
      queryClient.invalidateQueries({ queryKey: kodaKeys.session(sessionId) }),
      queryClient.invalidateQueries({ queryKey: kodaKeys.sessions }),
    ])
  }

  async function run(input: string) {
    const text = input.trim()
    if (!text || isRunning) return

    setComposerInput((current) => ({
      revision: current.revision + 1,
      value: '',
    }))
    setRunError('')
    setOptimisticUserEvent(
      create(EventSchema, {
        sessionId,
        turnId: `pending-${sessionId}`,
        author: 'user',
        message: { role: Role.USER, text },
      }),
    )
    setLiveEvents([])
    setPartialReasoning('')
    setPartialText('')
    setIsRunning(true)
    const abortController = new AbortController()
    abortRef.current = abortController

    try {
      const stream = kodaClient.run(
        {
          sessionId,
          input: { parts: [{ content: { case: 'text', value: text } }] },
          mode,
        },
        { signal: abortController.signal },
      )

      for await (const frame of stream) {
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
              setLiveEvents((current) => [...current, event])
              if (event.message?.role === Role.ASSISTANT) {
                setPartialReasoning('')
                setPartialText('')
              }
            }
            break
          }
          case 'approval': {
            const approval = frame.payload.value
            setApprovals((current) => [...current, approval])
            break
          }
          case 'questionPrompt': {
            const prompt = frame.payload.value
            setQuestionPrompts((current) => [...current, prompt])
            break
          }
          case 'completed': {
            const completedSession = frame.payload.value.session
            if (completedSession) {
              queryClient.setQueryData(
                kodaKeys.session(sessionId),
                completedSession,
              )
              queryClient.setQueryData<Session[]>(
                kodaKeys.sessions,
                (sessions) => replaceSession(sessions, completedSession),
              )
            }
            break
          }
        }
      }

      await invalidateSession()
      setLiveEvents([])
    } catch (error) {
      if (abortController.signal.aborted) {
        setOptimisticUserEvent(null)
        setLiveEvents([])
        setPartialReasoning('')
        setPartialText('')
        await queryClient.invalidateQueries({
          queryKey: kodaKeys.events(sessionId),
        })
      } else {
        setOptimisticUserEvent(null)
        setRunError(errorMessage(error))
      }
    } finally {
      setIsRunning(false)
      setApprovals([])
      setQuestionPrompts([])
      abortRef.current = null
    }
  }

  async function rewindLastTurn(turnId: string, retry: boolean) {
    if (isRunning || rewindingTurnId) return
    setRewindingTurnId(turnId)
    setRunError('')
    try {
      const response = await kodaClient.undoLastMessage({ sessionId })
      if (response.turnId !== turnId) {
        throw new Error('Koda removed a different turn; reload before retrying')
      }
      const input = inputText(response.input)
      setComposerInput((current) => ({
        revision: current.revision + 1,
        value: input,
      }))
      await invalidateSession()
      if (retry) {
        await run(input)
      } else {
        requestAnimationFrame(() => inputRef.current?.focus())
      }
    } catch (error) {
      setRunError(errorMessage(error))
    } finally {
      setRewindingTurnId('')
    }
  }

  async function editLastTurn(turnId: string, newText: string) {
    if (isRunning || rewindingTurnId) return
    setRewindingTurnId(turnId)
    setRunError('')
    try {
      const response = await kodaClient.undoLastMessage({ sessionId })
      if (response.turnId !== turnId) {
        throw new Error('Koda removed a different turn; reload before retrying')
      }
      await invalidateSession()
      await run(newText)
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
  const stop = useCallback(() => abortRef.current?.abort(), [])

  const runRef = useRef(run)
  useEffect(() => {
    runRef.current = run
  })
  const stableRun = useCallback((input: string): void => {
    runRef.current(input)
  }, [])

  return {
    approvals,
    clearApproval,
    clearQuestionPrompt,
    editLastTurn,
    events,
    inputRef,
    inputRevision: composerInput.revision,
    isRunning,
    mode,
    partialReasoning,
    partialText,
    questionPrompts,
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
