import {
  ChevronRight,
  Copy,
  LoaderCircle,
  Pencil,
  RotateCcw,
  Send,
  X,
} from 'lucide-react'
import { memo, useCallback, useEffect, useRef, useState } from 'react'

import { Button } from '@/components/ui/button'
import { EventView, ReasoningView } from '@/components/sessions/session-message'
import { ToolGroup } from '@/components/sessions/tool-activity'
import { Role } from '@/gen/koda/v1/service_pb'
import { eventText, groupTurnActivities, type Turn } from '@/lib/session-turns'

type SendShortcut = 'enter' | 'shift-enter' | 'command-enter'

const sendShortcutStorageKey = 'koda-studio-send-shortcut'

function loadSendShortcut(): SendShortcut {
  const stored = window.localStorage.getItem(sendShortcutStorageKey)
  return stored === 'shift-enter' || stored === 'command-enter'
    ? stored
    : 'enter'
}

function matchesSendShortcut(
  event: React.KeyboardEvent<HTMLTextAreaElement>,
  shortcut: SendShortcut,
) {
  if (event.key !== 'Enter' || event.nativeEvent.isComposing) return false
  switch (shortcut) {
    case 'shift-enter':
      return event.shiftKey && !event.metaKey && !event.ctrlKey && !event.altKey
    case 'command-enter':
      return event.metaKey && !event.shiftKey && !event.ctrlKey && !event.altKey
    default:
      return (
        !event.shiftKey && !event.metaKey && !event.ctrlKey && !event.altKey
      )
  }
}

export const SessionTurn = memo(function SessionTurn({
  canRevise,
  isEditing,
  isRunning,
  isRewinding,
  onEditCancel,
  onEditStart,
  onEditSubmit,
  onRetry,
  turn,
}: {
  canRevise: boolean
  isEditing: boolean
  isRunning: boolean
  isRewinding: boolean
  onEditCancel: () => void
  onEditStart: () => void
  onEditSubmit: (text: string) => void
  onRetry: () => void
  turn: Turn
}) {
  const userEvents = turn.events.filter(
    (event) => event.message?.role === Role.USER,
  )
  const activities = groupTurnActivities(turn.events)
  const earlierActivities = activities.slice(0, -1)
  const finalActivity = activities.at(-1)
  const lastAssistantEvent = [...turn.events]
    .reverse()
    .find(
      (event) =>
        event.message?.role === Role.ASSISTANT && eventText(event).trim(),
    )
  const lastAssistantText = lastAssistantEvent
    ? eventText(lastAssistantEvent)
    : ''
  const lastUserEvent = [...userEvents].reverse()[0]
  const initialEditText = lastUserEvent ? eventText(lastUserEvent) : ''

  return (
    <section className="space-y-6 border-b border-border/70 pb-8 last:border-b-0">
      {isEditing ? (
        <InlineEditComposer
          initialText={initialEditText}
          onCancel={onEditCancel}
          onSubmit={onEditSubmit}
        />
      ) : (
        userEvents.map((event, index) => (
          <EventView
            event={event}
            key={event.id || `${turn.id}-user-${index}`}
          />
        ))
      )}
      {canRevise && !isEditing && (
        <div className="-mt-3 flex justify-end gap-1">
          <Button
            aria-label="Edit message"
            disabled={isRewinding}
            onClick={onEditStart}
            size="icon"
            title="Edit message"
            variant="ghost"
          >
            <Pencil className="size-3.5" />
          </Button>
        </div>
      )}
      {isRunning ? (
        activities.map((activity, index) => (
          <ActivityView
            activity={activity}
            key={activity.assistant.id || `${turn.id}-activity-${index}`}
          />
        ))
      ) : (
        <>
          {earlierActivities.length > 0 && (
            <EarlierActivityDetails activities={earlierActivities} />
          )}
          {finalActivity && <ActivityView activity={finalActivity} />}
        </>
      )}
      {canRevise && !isEditing && (
        <div className="-mt-3 ml-9 flex items-center gap-1">
          <CopyButton text={lastAssistantText} />
          <Button
            aria-label="Retry turn"
            disabled={isRewinding}
            onClick={onRetry}
            size="icon"
            title="Retry turn"
            variant="ghost"
          >
            {isRewinding ? (
              <LoaderCircle className="size-3.5 animate-spin" />
            ) : (
              <RotateCcw className="size-3.5" />
            )}
          </Button>
        </div>
      )}
    </section>
  )
}, areTurnPropsEqual)

function areTurnPropsEqual(prev: TurnProps, next: TurnProps): boolean {
  return (
    prev.turn.id === next.turn.id &&
    prev.turn.events.length === next.turn.events.length &&
    prev.turn.events[prev.turn.events.length - 1] ===
      next.turn.events[next.turn.events.length - 1] &&
    prev.canRevise === next.canRevise &&
    prev.isEditing === next.isEditing &&
    prev.isRunning === next.isRunning &&
    prev.isRewinding === next.isRewinding
  )
}

type TurnProps = {
  canRevise: boolean
  isEditing: boolean
  isRunning: boolean
  isRewinding: boolean
  onEditCancel: () => void
  onEditStart: () => void
  onEditSubmit: (text: string) => void
  onRetry: () => void
  turn: Turn
}

type TurnActivity = ReturnType<typeof groupTurnActivities>[number]

function ActivityView({ activity }: { activity: TurnActivity }) {
  return (
    <div className="space-y-3">
      <ReasoningView reasoning={activity.assistant.message?.reasoning} />
      <EventView event={activity.assistant} />
      <ToolGroup assistant={activity.assistant} toolEvents={activity.tools} />
    </div>
  )
}

function EarlierActivityDetails({
  activities,
}: {
  activities: TurnActivity[]
}) {
  return (
    <details className="group/earlier-activity">
      <summary className="ml-9 flex w-fit cursor-pointer list-none items-center gap-1.5 text-xs font-medium text-muted-foreground hover:text-foreground">
        <ChevronRight className="size-3 transition-transform group-open/earlier-activity:rotate-90" />
        Earlier activity
      </summary>
      <div className="mt-4 space-y-4">
        {activities.map((activity, index) => (
          <ActivityView
            activity={activity}
            key={activity.assistant.id || `earlier-activity-${index}`}
          />
        ))}
      </div>
    </details>
  )
}

function InlineEditComposer({
  initialText,
  onCancel,
  onSubmit,
}: {
  initialText: string
  onCancel: () => void
  onSubmit: (text: string) => void
}) {
  const [input, setInput] = useState(initialText)
  const [sendShortcut] = useState<SendShortcut>(loadSendShortcut)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    textareaRef.current?.focus()
  }, [])

  function submit() {
    const trimmed = input.trim()
    if (!trimmed) return
    onSubmit(trimmed)
  }

  const sendShortcutLabel =
    sendShortcut === 'shift-enter'
      ? 'Shift + Enter'
      : sendShortcut === 'command-enter'
        ? '⌘ + Enter'
        : 'Enter'

  return (
    <div className="flex justify-end">
      <div className="w-full max-w-[85%] space-y-2">
        <div className="rounded-xl border border-border bg-card shadow-xl focus-within:border-ring">
          <textarea
            ref={textareaRef}
            aria-label="Edit message"
            className="max-h-48 min-h-20 w-full resize-none bg-transparent px-4 py-3 text-sm leading-6 text-foreground outline-none placeholder:text-muted-foreground"
            onChange={(event) => setInput(event.target.value)}
            onKeyDown={(event) => {
              if (matchesSendShortcut(event, sendShortcut)) {
                event.preventDefault()
                submit()
              }
              if (event.key === 'Escape') {
                event.preventDefault()
                onCancel()
              }
            }}
            value={input}
          />
          <div className="flex items-center justify-between px-2.5 pb-2.5">
            <Button
              aria-label="Cancel editing"
              onClick={onCancel}
              size="icon"
              title="Cancel editing"
              variant="ghost"
            >
              <X className="size-4" />
            </Button>
            <Button
              aria-label="Send"
              className="rounded-md bg-primary text-primary-foreground hover:bg-primary/90"
              disabled={!input.trim()}
              onClick={submit}
              size="icon"
              title={`Send (${sendShortcutLabel})`}
            >
              <Send className="size-4" />
            </Button>
          </div>
        </div>
      </div>
    </div>
  )
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = useCallback(() => {
    if (!text) return
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }, [text])

  return (
    <Button
      aria-label="Copy response"
      disabled={!text}
      onClick={handleCopy}
      size="icon"
      title="Copy response"
      variant="ghost"
    >
      {copied ? (
        <span className="text-[10px] font-medium text-green-400">Copied!</span>
      ) : (
        <Copy className="size-3.5" />
      )}
    </Button>
  )
}
