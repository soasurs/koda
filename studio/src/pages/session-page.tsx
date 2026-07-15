import { useQuery } from '@tanstack/react-query'
import { useParams } from '@tanstack/react-router'
import {
  Archive,
  Check,
  LoaderCircle,
  Sparkles,
  TriangleAlert,
} from 'lucide-react'
import { Fragment, useMemo, useState } from 'react'

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
import type {
  CompactionProgress,
  CompactionStatus,
} from '@/gen/koda/v1/service_pb'
import { CompactionProgressStage } from '@/gen/koda/v1/service_pb'
import { TurnStatus } from '@/gen/koda/v1/service_pb'
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
    queryFn: async () => await kodaClient.listEvents({ sessionId }),
  })
  const history = eventsQuery.data
  const sessionRun = useSessionRun(sessionId, history?.events ?? [])
  const [editingTurnId, setEditingTurnId] = useState('')
  const turns = useMemo(
    () => groupEventsByTurn(sessionRun.events, history?.turns),
    [history?.turns, sessionRun.events],
  )
  const compactionBoundaryIndex = useMemo(() => {
    const count = Number(history?.compaction?.compactedEventCount ?? 0n)
    if (count <= 0) return -1
    const boundaryEvent =
      history?.events[Math.min(count, history.events.length) - 1]
    if (!boundaryEvent) return -1
    return turns.findIndex((turn) =>
      turn.events.some((event) => event.id === boundaryEvent.id),
    )
  }, [history, turns])
  const scrollContent = useMemo(
    () => [
      eventsQuery.data,
      sessionRun.events.length,
      sessionRun.partialReasoning,
      sessionRun.partialText,
      sessionRun.compactionProgress,
      sessionRun.approvals,
      sessionRun.questionPrompts,
    ],
    [
      eventsQuery.data,
      sessionRun.events.length,
      sessionRun.partialReasoning,
      sessionRun.partialText,
      sessionRun.compactionProgress,
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
                <Fragment key={turn.id || `turn-${index}`}>
                  <SessionTurn
                    canRevise={
                      !sessionRun.isRunning &&
                      Boolean(turn.id) &&
                      turn.id === history?.undoableTurnId
                    }
                    isEditing={editingTurnId === turn.id}
                    isRunning={
                      index === turns.length - 1 && sessionRun.isRunning
                    }
                    isRewinding={sessionRun.rewindingTurnId === turn.id}
                    onEditCancel={() => setEditingTurnId('')}
                    onEditStart={() => setEditingTurnId(turn.id ?? '')}
                    onEditSubmit={(text) => {
                      setEditingTurnId('')
                      void sessionRun.editLastTurn(turn.id ?? '', text)
                    }}
                    onRetry={(input) => {
                      if (
                        turn.metadata?.status === TurnStatus.FAILED ||
                        turn.metadata?.status === TurnStatus.INTERRUPTED
                      ) {
                        void sessionRun.runStable(input)
                        return
                      }
                      void sessionRun.rewindLastTurn(turn.id, true)
                    }}
                    turn={turn}
                  />
                  {index === compactionBoundaryIndex && history?.compaction && (
                    <CompactionBoundary compaction={history.compaction} />
                  )}
                </Fragment>
              ))}
              {sessionRun.compactionProgress && (
                <CompactionActivity progress={sessionRun.compactionProgress} />
              )}
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

function CompactionActivity({ progress }: { progress: CompactionProgress }) {
  const completed = progress.stage === CompactionProgressStage.COMPLETED
  const failed = progress.stage === CompactionProgressStage.FAILED
  const detail = completed
    ? `${formatTokens(progress.sourceTokens)} source tokens · ${formatTokens(progress.estimatedTokensAfter)} estimated after`
    : failed
      ? 'Continuing with existing context'
      : `${formatTokens(progress.contextTokens)} tokens in current context`

  return (
    <div
      className="ml-9 flex items-center gap-3 text-sm text-muted-foreground"
      data-testid="compaction-progress"
      role="status"
    >
      {completed ? (
        <Check className="size-4 text-foreground" />
      ) : failed ? (
        <TriangleAlert className="size-4" />
      ) : (
        <LoaderCircle className="size-4 animate-spin" />
      )}
      <div>
        <div className="font-medium text-foreground">
          {completed
            ? `Context compacted · generation ${progress.generation}`
            : failed
              ? 'Context compaction failed'
              : 'Compacting earlier context…'}
        </div>
        <div className="mt-0.5 text-xs">{detail}</div>
      </div>
    </div>
  )
}

const compactTokenFormatter = new Intl.NumberFormat('en-US', {
  maximumFractionDigits: 1,
  notation: 'compact',
})

function formatTokens(tokens: bigint) {
  return compactTokenFormatter.format(tokens)
}

function CompactionBoundary({ compaction }: { compaction: CompactionStatus }) {
  const createdAt = new Date(Number(compaction.createdAt))
  const title = [
    `Generation ${compaction.generation}`,
    `${compaction.compactedEventCount} earlier events summarized`,
    `${compaction.sourceTokens} source tokens`,
    `${compaction.estimatedTokensAfter} estimated tokens after compaction`,
    compaction.modelId ? `Model: ${compaction.modelId}` : '',
    Number.isNaN(createdAt.getTime()) ? '' : createdAt.toLocaleString(),
  ]
    .filter(Boolean)
    .join(' · ')

  return (
    <div
      className="flex items-center gap-3 py-1 text-xs text-muted-foreground"
      data-testid="compaction-boundary"
      title={title}
    >
      <div className="h-px flex-1 bg-border" />
      <span className="flex items-center gap-1.5 whitespace-nowrap rounded-full border border-border bg-muted/50 px-2.5 py-1">
        <Archive className="size-3" />
        Earlier context compacted · generation {compaction.generation}
      </span>
      <div className="h-px flex-1 bg-border" />
    </div>
  )
}

function EmptyConversation() {
  return (
    <div className="py-20 text-center">
      <div className="mx-auto flex size-11 items-center justify-center rounded-xl border border-border bg-muted">
        <Sparkles className="size-5 text-muted-foreground" />
      </div>
      <h2 className="mt-4 text-sm font-medium">Ready to work</h2>
      <p className="mt-2 text-sm text-muted-foreground">
        Ask Koda to inspect, plan, or change this workspace.
      </p>
    </div>
  )
}

function CenteredLoader() {
  return (
    <div className="flex h-56 items-center justify-center">
      <LoaderCircle className="size-5 animate-spin text-muted-foreground" />
    </div>
  )
}
