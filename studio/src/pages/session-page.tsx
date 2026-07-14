import { useQuery } from '@tanstack/react-query'
import { useParams } from '@tanstack/react-router'
import { LoaderCircle, Sparkles } from 'lucide-react'
import { useMemo, useState } from 'react'

import { SessionComposer } from '@/components/sessions/session-composer'
import { SessionHeader } from '@/components/sessions/session-header'
import {
  AssistantText,
  ReasoningView,
} from '@/components/sessions/session-message'
import { ApprovalCard, QuestionCard } from '@/components/sessions/run-prompts'
import { SessionTurn } from '@/components/sessions/session-turn'
import { useFollowLatest } from '@/components/sessions/use-follow-latest'
import { useSessionRun } from '@/components/sessions/use-session-run'
import { kodaClient } from '@/lib/connect'
import { errorMessage, kodaKeys } from '@/lib/koda'
import { groupEventsByTurn } from '@/lib/session-turns'

export function SessionPage() {
  const { sessionId } = useParams({ from: '/sessions/$sessionId' })
  return <SessionContent key={sessionId} sessionId={sessionId} />
}

function SessionContent({ sessionId }: { sessionId: string }) {
  const sessionQuery = useQuery({
    queryKey: kodaKeys.session(sessionId),
    queryFn: async () => (await kodaClient.getSession({ sessionId })).session,
  })
  const eventsQuery = useQuery({
    queryKey: kodaKeys.events(sessionId),
    queryFn: async () => (await kodaClient.listEvents({ sessionId })).events,
  })
  const sessionRun = useSessionRun(sessionId, eventsQuery.data ?? [])
  const [editingTurnId, setEditingTurnId] = useState('')
  const turns = useMemo(
    () => groupEventsByTurn(sessionRun.events),
    [sessionRun.events],
  )
  const scrollContent = useMemo(
    () => [
      eventsQuery.data,
      sessionRun.events.length,
      sessionRun.partialReasoning,
      sessionRun.partialText,
      sessionRun.approvals,
      sessionRun.questionPrompts,
    ],
    [
      eventsQuery.data,
      sessionRun.events.length,
      sessionRun.partialReasoning,
      sessionRun.partialText,
      sessionRun.approvals,
      sessionRun.questionPrompts,
    ],
  )
  const { containerRef, onScroll } = useFollowLatest<HTMLElement>(
    scrollContent,
    sessionId,
  )

  if (sessionQuery.isPending) return <CenteredLoader />

  const session = sessionQuery.data
  if (sessionQuery.isError || !session) {
    return (
      <div className="p-8">
        <p className="error-box">
          {sessionQuery.isError
            ? errorMessage(sessionQuery.error)
            : 'Session not found'}
        </p>
      </div>
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <SessionHeader session={session} />

      <main
        className="min-h-0 flex-1 overflow-y-auto"
        onScroll={onScroll}
        ref={containerRef}
      >
        <div className="mx-auto w-full max-w-4xl px-4 pt-8 sm:px-6">
          {eventsQuery.isPending ? (
            <CenteredLoader />
          ) : eventsQuery.isError ? (
            <p className="error-box">{errorMessage(eventsQuery.error)}</p>
          ) : sessionRun.events.length === 0 &&
            !sessionRun.partialReasoning &&
            !sessionRun.partialText ? (
            <EmptyConversation />
          ) : (
            <div className="space-y-7">
              {turns.map((turn, index) => (
                <SessionTurn
                  canRevise={
                    index === turns.length - 1 &&
                    !sessionRun.isRunning &&
                    Boolean(turn.id)
                  }
                  isEditing={editingTurnId === turn.id}
                  isRewinding={sessionRun.rewindingTurnId === turn.id}
                  key={turn.id || `turn-${index}`}
                  onEditCancel={() => setEditingTurnId('')}
                  onEditStart={() => setEditingTurnId(turn.id ?? '')}
                  onEditSubmit={(text) => {
                    setEditingTurnId('')
                    void sessionRun.editLastTurn(turn.id ?? '', text)
                  }}
                  onRetry={() => void sessionRun.rewindLastTurn(turn.id, true)}
                  turn={turn}
                />
              ))}
              {(sessionRun.partialReasoning || sessionRun.partialText) && (
                <div className="space-y-3">
                  <ReasoningView
                    key={
                      sessionRun.partialText
                        ? 'stream-finished'
                        : 'stream-active'
                    }
                    reasoning={sessionRun.partialReasoning}
                    streaming={Boolean(
                      sessionRun.partialReasoning && !sessionRun.partialText,
                    )}
                  />
                  {sessionRun.partialText && (
                    <AssistantText text={sessionRun.partialText} streaming />
                  )}
                </div>
              )}
            </div>
          )}

          {sessionRun.approvals.map((approval) => (
            <ApprovalCard
              approval={approval}
              key={approval.id}
              onResolved={() => sessionRun.clearApproval(approval.id)}
            />
          ))}
          {sessionRun.questionPrompts.map((prompt) => (
            <QuestionCard
              key={prompt.id}
              onResolved={() => sessionRun.clearQuestionPrompt(prompt.id)}
              prompt={prompt}
            />
          ))}
        </div>
      </main>

      <SessionComposer
        initialInput={sessionRun.restoredInput}
        inputRef={sessionRun.inputRef}
        isRunning={sessionRun.isRunning}
        key={`${session.id}:${sessionRun.inputRevision}`}
        mode={sessionRun.mode}
        onModeChange={sessionRun.setMode}
        onRun={sessionRun.runStable}
        onStop={sessionRun.stop}
        runError={sessionRun.runError}
        session={session}
      />
    </div>
  )
}

function EmptyConversation() {
  return (
    <div className="py-20 text-center">
      <div className="mx-auto flex size-11 items-center justify-center rounded-xl border border-neutral-800 bg-neutral-900">
        <Sparkles className="size-5 text-neutral-400" />
      </div>
      <h2 className="mt-4 text-sm font-medium">Ready to work</h2>
      <p className="mt-2 text-sm text-neutral-600">
        Ask Koda to inspect, plan, or change this workspace.
      </p>
    </div>
  )
}

function CenteredLoader() {
  return (
    <div className="flex h-56 items-center justify-center">
      <LoaderCircle className="size-5 animate-spin text-neutral-600" />
    </div>
  )
}
