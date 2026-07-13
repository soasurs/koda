import { create } from '@bufbuild/protobuf'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useRef, useState } from 'react'

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
  const [approval, setApproval] = useState<ToolApproval | null>(null)
  const [questionPrompt, setQuestionPrompt] = useState<QuestionPrompt | null>(
    null,
  )

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
          case 'approval':
            setApproval(frame.payload.value)
            break
          case 'questionPrompt':
            setQuestionPrompt(frame.payload.value)
            break
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
      setApproval(null)
      setQuestionPrompt(null)
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

  return {
    approval,
    clearApproval: () => setApproval(null),
    clearQuestionPrompt: () => setQuestionPrompt(null),
    editLastTurn,
    events,
    inputRef,
    inputRevision: composerInput.revision,
    isRunning,
    mode,
    partialReasoning,
    partialText,
    questionPrompt,
    restoredInput: composerInput.value,
    rewindLastTurn,
    rewindingTurnId,
    run,
    runError,
    setMode,
    stop: () => abortRef.current?.abort(),
  }
}
