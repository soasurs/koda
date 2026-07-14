import { Copy, LoaderCircle, Pencil, RotateCcw, Send, X } from 'lucide-react'
import { memo, useCallback, useEffect, useRef, useState } from 'react'

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
  isRewinding,
  onEditCancel,
  onEditStart,
  onEditSubmit,
  onRetry,
  turn,
}: {
  canRevise: boolean
  isEditing: boolean
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
          <button
            aria-label="Edit message"
            className="icon-button"
            disabled={isRewinding}
            onClick={onEditStart}
            title="Edit message"
            type="button"
          >
            <Pencil className="size-3.5" />
          </button>
        </div>
      )}
      {activities.map((activity, index) => (
        <div
          className="space-y-3"
          key={activity.assistant.id || `${turn.id}-activity-${index}`}
        >
          <ReasoningView reasoning={activity.assistant.message?.reasoning} />
          <EventView event={activity.assistant} />
          <ToolGroup
            assistant={activity.assistant}
            key={`${activity.assistant.id}-${activity.tools.length}`}
            toolEvents={activity.tools}
          />
        </div>
      ))}
      {canRevise && !isEditing && (
        <div className="-mt-3 ml-9 flex items-center gap-1">
          <CopyButton text={lastAssistantText} />
          <button
            aria-label="Retry turn"
            className="icon-button"
            disabled={isRewinding}
            onClick={onRetry}
            title="Retry turn"
            type="button"
          >
            {isRewinding ? (
              <LoaderCircle className="size-3.5 animate-spin" />
            ) : (
              <RotateCcw className="size-3.5" />
            )}
          </button>
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
    prev.isRewinding === next.isRewinding
  )
}

type TurnProps = {
  canRevise: boolean
  isEditing: boolean
  isRewinding: boolean
  onEditCancel: () => void
  onEditStart: () => void
  onEditSubmit: (text: string) => void
  onRetry: () => void
  turn: Turn
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
            <button
              aria-label="Cancel editing"
              className="icon-button"
              onClick={onCancel}
              title="Cancel editing"
              type="button"
            >
              <X className="size-4" />
            </button>
            <button
              aria-label="Send"
              className="flex size-8 items-center justify-center rounded-md bg-primary text-primary-foreground hover:bg-primary/90"
              disabled={!input.trim()}
              onClick={submit}
              title={`Send (${sendShortcutLabel})`}
              type="button"
            >
              <Send className="size-4" />
            </button>
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
    <button
      aria-label="Copy response"
      className="icon-button"
      disabled={!text}
      onClick={handleCopy}
      title="Copy response"
      type="button"
    >
      {copied ? (
        <span className="text-[10px] font-medium text-green-400">Copied!</span>
      ) : (
        <Copy className="size-3.5" />
      )}
    </button>
  )
}
