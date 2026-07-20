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

import { useI18n } from '@/app/i18n'
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
import { useRunCompletionNotification } from '@/components/sessions/use-run-completion-notification'
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
  const { t } = useI18n()
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

  const sessionLabel = sessionQuery.data?.title || t('session.untitled')
  useRunCompletionNotification(
    sessionRun.isRunning,
    t('session.notification.completed', { label: sessionLabel }),
  )
  if (sessionQuery.isPending) return <CenteredLoader />

  const session = sessionQuery.data
  if (sessionQuery.isError || !session) {
    return (
      <div className="p-8">
        <p className="error-box">
          {sessionQuery.isError
            ? errorMessage(sessionQuery.error)
            : t('session.notFound')}
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
          {eventsQuery.isPending && !sessionRun.isRunning ? (
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
  const { t, locale } = useI18n()
  const completed = progress.stage === CompactionProgressStage.COMPLETED
  const failed = progress.stage === CompactionProgressStage.FAILED
  const tokenFormatter = useTokenFormatter(locale)
  const detail = completed
    ? t('session.compaction.detail.completed', {
        sourceTokens: tokenFormatter.format(progress.sourceTokens),
        estimatedTokens: tokenFormatter.format(progress.estimatedTokensAfter),
      })
    : failed
      ? t('session.compaction.continuing')
      : t('session.compaction.detail.inProgress', {
          contextTokens: tokenFormatter.format(progress.contextTokens),
        })

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
            ? t('session.compaction.completed', {
                generation: String(progress.generation),
              })
            : failed
              ? t('session.compaction.failed')
              : t('session.compaction.inProgress')}
        </div>
        <div className="mt-0.5 text-xs">{detail}</div>
      </div>
    </div>
  )
}

function useTokenFormatter(locale: string) {
  return useMemo(
    () =>
      new Intl.NumberFormat(locale, {
        maximumFractionDigits: 1,
        notation: 'compact',
      }),
    [locale],
  )
}

function CompactionBoundary({ compaction }: { compaction: CompactionStatus }) {
  const { t, locale } = useI18n()
  const createdAt = new Date(Number(compaction.createdAt))
  const tokenFormatter = useTokenFormatter(locale)
  const title = [
    t('session.compaction.boundary.titleGeneration', {
      generation: String(compaction.generation),
    }),
    t('session.compaction.boundary.titleEvents', {
      count: String(compaction.compactedEventCount),
    }),
    t('session.compaction.boundary.titleSourceTokens', {
      count: tokenFormatter.format(compaction.sourceTokens),
    }),
    t('session.compaction.boundary.titleEstimatedTokens', {
      count: tokenFormatter.format(compaction.estimatedTokensAfter),
    }),
    compaction.modelId
      ? t('session.compaction.boundary.titleModel', {
          modelId: compaction.modelId,
        })
      : '',
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
        {t('session.compaction.boundary.label', {
          generation: String(compaction.generation),
        })}
      </span>
      <div className="h-px flex-1 bg-border" />
    </div>
  )
}

function EmptyConversation() {
  const { t } = useI18n()
  return (
    <div className="py-20 text-center">
      <div className="mx-auto flex size-11 items-center justify-center rounded-xl border border-border bg-muted">
        <Sparkles className="size-5 text-muted-foreground" />
      </div>
      <h2 className="mt-4 text-sm font-medium">{t('session.empty.title')}</h2>
      <p className="mt-2 text-sm text-muted-foreground">
        {t('session.empty.body')}
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
