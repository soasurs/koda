import {
  ChevronRight,
  Copy,
  LoaderCircle,
  Paperclip,
  Pencil,
  RotateCcw,
  Send,
  TriangleAlert,
  X,
} from 'lucide-react'
import {
  memo,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ChangeEvent,
} from 'react'

import { useI18n, type TKey } from '@/app/i18n'
import { ImageViewer } from '@/components/ui/image-viewer'
import { Button } from '@/components/ui/button'
import { EventView, ReasoningView } from '@/components/sessions/session-message'
import { ToolGroup } from '@/components/sessions/tool-activity'
import { usePreferences } from '@/app/preferences-context-value'
import {
  Role,
  TurnFailureStage,
  TurnReason,
  TurnStatus,
} from '@/gen/koda/v1/service_pb'
import type {
  ComposerAttachment,
  ComposerInput,
} from '@/lib/composer-attachments'
import {
  fileToAttachment,
  isComposerInputEmpty,
  revokeAttachment,
  revokeAttachments,
  type AttachmentLoadError,
} from '@/lib/composer-attachments'
import {
  eventText,
  groupTurnActivities,
  inputToComposerInput,
  type Turn,
} from '@/lib/session-turns'

type SendShortcut = 'enter' | 'shift-enter' | 'command-enter'

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
  onEditSubmit: (input: ComposerInput) => void
  onRetry: (input: ComposerInput) => void
  turn: Turn
}) {
  const { t } = useI18n()
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
  const initialEditInput = useMemo(() => {
    const lastUserEvent = [...turn.events]
      .reverse()
      .find((event) => event.message?.role === Role.USER)
    return lastUserEvent
      ? inputToComposerInput(
          { parts: lastUserEvent.message?.parts ?? [] },
          lastUserEvent.message?.text ?? '',
        )
      : { text: '', attachments: [] }
  }, [turn.events])

  return (
    <section className="space-y-6 border-b border-border/70 pb-8 last:border-b-0">
      {isEditing ? (
        <InlineEditComposer
          initialInput={initialEditInput}
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
            aria-label={t('session.turn.editMessage')}
            disabled={isRewinding}
            onClick={onEditStart}
            size="icon"
            title={t('session.turn.editMessage')}
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
      {!isRunning && <TurnStatusView turn={turn} />}
      {canRevise && !isEditing && (
        <div className="-mt-3 ml-9 flex items-center gap-1">
          <CopyButton text={lastAssistantText} />
          <Button
            aria-label={t('session.turn.retryTurn')}
            disabled={isRewinding}
            onClick={() => onRetry(initialEditInput)}
            size="icon"
            title={t('session.turn.retryTurn')}
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
    prev.turn.metadata === next.turn.metadata &&
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
  onEditSubmit: (input: ComposerInput) => void
  onRetry: (input: ComposerInput) => void
  turn: Turn
}

function TurnStatusView({ turn }: { turn: Turn }) {
  const { t } = useI18n()
  const metadata = turn.metadata
  if (
    !metadata ||
    (metadata.status !== TurnStatus.FAILED &&
      metadata.status !== TurnStatus.INTERRUPTED)
  ) {
    return null
  }
  const failed = metadata.status === TurnStatus.FAILED
  const title = failed
    ? t('session.turn.failed')
    : t('session.turn.interrupted')
  const detail = failed
    ? metadata.failure?.message ||
      failureLabel(t, metadata.failure?.code, metadata.failure?.stage)
    : interruptionLabel(t, metadata.reason)
  return (
    <div
      className="ml-9 flex items-start gap-3 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm"
      role="status"
    >
      <TriangleAlert className="mt-0.5 size-4 shrink-0 text-destructive" />
      <div>
        <div className="font-medium text-foreground">{title}</div>
        <div className="mt-0.5 text-xs text-muted-foreground">{detail}</div>
      </div>
    </div>
  )
}

function interruptionLabel(
  t: ReturnType<typeof useI18n>['t'],
  reason: TurnReason,
) {
  switch (reason) {
    case TurnReason.CANCELED:
      return t('session.turn.interruption.canceled')
    case TurnReason.DEADLINE_EXCEEDED:
      return t('session.turn.interruption.deadline')
    case TurnReason.CONSUMER_STOPPED:
      return t('session.turn.interruption.consumer')
    case TurnReason.ABANDONED:
      return t('session.turn.interruption.abandoned')
    default:
      return t('session.turn.interruption.default')
  }
}

const FAILURE_LOCATION_KEYS = {
  [TurnFailureStage.UNSPECIFIED]: '' as const,
  [TurnFailureStage.AGENT]: 'session.turn.failure.location.agent' as const,
  [TurnFailureStage.PROVIDER]:
    'session.turn.failure.location.provider' as const,
  [TurnFailureStage.TOOL]: 'session.turn.failure.location.tool' as const,
  [TurnFailureStage.PERSISTENCE]:
    'session.turn.failure.location.storage' as const,
  [TurnFailureStage.CONSUMER]: 'session.turn.failure.location.client' as const,
}

function failureLabel(
  t: ReturnType<typeof useI18n>['t'],
  code = '',
  stage = TurnFailureStage.UNSPECIFIED,
) {
  const locationKey = FAILURE_LOCATION_KEYS[stage]
  const location = locationKey ? t(locationKey as TKey) : ''
  const normalizedCode = code.replaceAll('_', ' ').trim()
  if (normalizedCode && location) return `${normalizedCode} · ${location}`
  if (normalizedCode) return normalizedCode
  return location
    ? t('session.turn.failure.inLocation', { location })
    : t('session.turn.failure.generic')
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
  const { t } = useI18n()
  return (
    <details className="group/earlier-activity">
      <summary className="ml-9 flex w-fit cursor-pointer list-none items-center gap-1.5 text-xs font-medium text-muted-foreground hover:text-foreground">
        <ChevronRight className="size-3 transition-transform group-open/earlier-activity:rotate-90" />
        {t('session.turn.earlierActivity')}
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
  initialInput,
  onCancel,
  onSubmit,
}: {
  initialInput: ComposerInput
  onCancel: () => void
  onSubmit: (input: ComposerInput) => void
}) {
  const { t } = useI18n()
  const { sendShortcut } = usePreferences()
  const [input, setInput] = useState(initialInput.text)
  const [attachments, setAttachments] = useState<ComposerAttachment[]>(
    initialInput.attachments,
  )
  const [attachmentError, setAttachmentError] = useState('')
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    textareaRef.current?.focus()
  }, [])

  useEffect(() => {
    return () => revokeAttachments(initialInput.attachments)
  }, [initialInput.attachments])

  function reportAttachmentErrors(errors: AttachmentLoadError[]) {
    if (errors.length === 0) {
      setAttachmentError('')
      return
    }
    const first = errors[0]
    if (!first) return
    const key: TKey =
      first.reason === 'not-image'
        ? 'session.composer.attachment.notImage'
        : first.reason === 'too-large'
          ? 'session.composer.attachment.tooLarge'
          : 'session.composer.attachment.readFailed'
    setAttachmentError(t(key, { name: first.name }))
  }

  async function addFiles(files: FileList | File[]) {
    const incoming = Array.from(files)
    const results = await Promise.all(
      incoming.map((file) => fileToAttachment(file)),
    )
    const accepted: ComposerAttachment[] = []
    const errors: AttachmentLoadError[] = []
    for (const result of results) {
      if ('id' in result) accepted.push(result)
      else errors.push(result)
    }
    if (accepted.length > 0) {
      setAttachments((current) => [...current, ...accepted])
    }
    reportAttachmentErrors(errors)
  }

  function removeAttachment(id: string) {
    setAttachments((current) => {
      const target = current.find((att) => att.id === id)
      if (target) revokeAttachment(target)
      return current.filter((att) => att.id !== id)
    })
  }

  function handlePaste(event: React.ClipboardEvent<HTMLTextAreaElement>) {
    const files = Array.from(event.clipboardData.items)
      .filter((item) => item.kind === 'file')
      .map((item) => item.getAsFile())
      .filter((file): file is File => file !== null)
    if (files.length === 0) return
    event.preventDefault()
    void addFiles(files)
  }

  function handleFileInputChange(event: ChangeEvent<HTMLInputElement>) {
    if (event.target.files && event.target.files.length > 0) {
      void addFiles(event.target.files)
    }
    event.target.value = ''
  }

  function submit() {
    const payload: ComposerInput = { text: input, attachments }
    if (isComposerInputEmpty(payload)) return
    onSubmit(payload)
  }

  const canSubmit = !isComposerInputEmpty({ text: input, attachments })

  const sendShortcutLabel =
    sendShortcut === 'shift-enter'
      ? t('session.turn.shortcut.shiftEnter')
      : sendShortcut === 'command-enter'
        ? t('session.turn.shortcut.commandEnter')
        : t('session.turn.shortcut.enter')

  return (
    <div className="flex justify-end">
      <div className="w-full max-w-[85%] space-y-2">
        <div className="rounded-xl border border-border bg-card shadow-xl focus-within:border-ring">
          {attachments.length > 0 && (
            <div className="flex flex-wrap gap-2 px-3 pt-3">
              {attachments.map((att) => (
                <div
                  className="group relative overflow-hidden rounded-md border border-border bg-background"
                  key={att.id}
                >
                  <ImageViewer
                    alt={att.name}
                    className="size-16 object-cover"
                    src={att.previewUrl}
                  />
                  <button
                    aria-label={t('session.composer.attachment.remove')}
                    className="absolute right-0.5 top-0.5 flex size-5 items-center justify-center rounded-full bg-background/90 text-foreground opacity-0 transition-opacity group-hover:opacity-100"
                    onClick={() => removeAttachment(att.id)}
                    type="button"
                  >
                    <X className="size-3" />
                  </button>
                </div>
              ))}
            </div>
          )}
          {attachmentError && (
            <p className="mx-3 mt-2 text-xs text-destructive">
              {attachmentError}
            </p>
          )}
          <textarea
            ref={textareaRef}
            aria-label={t('session.turn.editMessage')}
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
            onPaste={handlePaste}
            value={input}
          />
          <input
            accept="image/*"
            className="hidden"
            multiple
            onChange={handleFileInputChange}
            ref={fileInputRef}
            type="file"
          />
          <div className="flex items-center justify-between px-2.5 pb-2.5">
            <div className="flex items-center gap-1">
              <Button
                aria-label={t('session.composer.attach')}
                onClick={() => fileInputRef.current?.click()}
                size="icon"
                title={t('session.composer.attach')}
                variant="ghost"
              >
                <Paperclip className="size-4" />
              </Button>
              <Button
                aria-label={t('session.turn.cancelEditing')}
                onClick={onCancel}
                size="icon"
                title={t('session.turn.cancelEditing')}
                variant="ghost"
              >
                <X className="size-4" />
              </Button>
            </div>
            <Button
              aria-label={t('session.turn.send')}
              className="rounded-md bg-primary text-primary-foreground hover:bg-primary/90"
              disabled={!canSubmit}
              onClick={submit}
              size="icon"
              title={`${t('session.turn.send')} (${sendShortcutLabel})`}
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
  const { t } = useI18n()
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
      aria-label={t('session.turn.copyResponse')}
      disabled={!text}
      onClick={handleCopy}
      size="icon"
      title={t('session.turn.copyResponse')}
      variant="ghost"
    >
      {copied ? (
        <span className="text-[10px] font-medium text-green-400">
          {t('session.turn.copied')}
        </span>
      ) : (
        <Copy className="size-3.5" />
      )}
    </Button>
  )
}
