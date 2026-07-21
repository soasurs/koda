import { CircleStop, Paperclip, Send } from 'lucide-react'
import {
  memo,
  useEffect,
  useRef,
  useState,
  type DragEvent,
  type RefObject,
} from 'react'

import { useI18n } from '@/app/i18n'
import { usePreferences } from '@/app/preferences-context-value'
import { Button } from '@/components/ui/button'
import { SessionModelPicker } from '@/components/sessions/session-model-picker'
import { ModeSelector } from '@/components/sessions/mode-selector'
import { PermissionSelectors } from '@/components/sessions/permission-selectors'
import { SendShortcutPicker } from '@/components/sessions/send-shortcut-picker'
import { AttachmentPreview } from '@/components/sessions/attachment-preview'
import { useAttachments } from '@/components/sessions/use-attachments'
import { matchesSendShortcut } from '@/lib/send-shortcut'
import type { Session } from '@/gen/koda/v1/service_pb'
import { AgentMode, FileAccess, ShellAccess } from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'
import type { ComposerInput } from '@/lib/composer-attachments'
import { isComposerInputEmpty } from '@/lib/composer-attachments'

export const SessionComposer = memo(function SessionComposer({
  initialInput,
  inputRef,
  isRunning,
  mode,
  onModeChange,
  onRun,
  onStop,
  runError,
  session,
}: {
  initialInput: ComposerInput
  inputRef: RefObject<HTMLTextAreaElement | null>
  isRunning: boolean
  mode: AgentMode
  onModeChange: (mode: AgentMode) => void
  onRun: (input: ComposerInput) => void
  onStop: () => void
  runError: string
  session: Session
}) {
  const { t } = useI18n()
  const { sendShortcut } = usePreferences()
  const [input, setInput] = useState(initialInput.text)
  const [isDragOver, setIsDragOver] = useState(false)
  const [fileAccess, setFileAccess] = useState(session.fileAccess)
  const [shellAccess, setShellAccess] = useState(session.shellAccess)
  const wasRunningRef = useRef(false)
  const {
    attachments,
    setAttachments,
    error,
    setError,
    fileInputRef,
    addFiles,
    removeAttachment,
    handlePaste,
    handleFileInputChange,
  } = useAttachments(initialInput.attachments)

  useEffect(() => {
    if (wasRunningRef.current && !isRunning) {
      requestAnimationFrame(() => inputRef.current?.focus())
    }
    wasRunningRef.current = isRunning
  }, [isRunning, inputRef])

  function updatePermission(
    kind: 'fileAccess' | 'shellAccess',
    value: FileAccess | ShellAccess,
  ) {
    if (isRunning) return
    if (kind === 'fileAccess') setFileAccess(value as FileAccess)
    else setShellAccess(value as ShellAccess)
    kodaClient
      .updateSession({ sessionId: session.id, [kind]: value })
      .catch(() => {
        setFileAccess(session.fileAccess)
        setShellAccess(session.shellAccess)
      })
  }

  function handleDrop(event: DragEvent<HTMLDivElement>) {
    event.preventDefault()
    setIsDragOver(false)
    if (event.dataTransfer.files.length === 0) return
    void addFiles(event.dataTransfer.files)
  }

  function handleDragOver(event: DragEvent<HTMLDivElement>) {
    if (event.dataTransfer.types.includes('Files')) {
      event.preventDefault()
      setIsDragOver(true)
    }
  }

  function handleDragLeave(event: DragEvent<HTMLDivElement>) {
    if (event.currentTarget === event.target) setIsDragOver(false)
  }

  function submit() {
    if (isRunning) return
    const payload: ComposerInput = { text: input, attachments }
    if (isComposerInputEmpty(payload)) return
    setInput('')
    setAttachments([])
    setError('')
    onRun(payload)
  }

  const canSubmit = !isComposerInputEmpty({ text: input, attachments })

  return (
    <footer className="shrink-0 bg-linear-to-t from-background via-background to-transparent px-4 pb-4 pt-2 sm:px-6">
      <div className="mx-auto max-w-4xl">
        {runError && <p className="error-box mb-3">{runError}</p>}
        <div
          className={`relative rounded-xl border bg-card shadow-xl focus-within:border-ring ${
            isDragOver ? 'border-primary border-dashed' : 'border-border'
          }`}
          onDragLeave={handleDragLeave}
          onDragOver={handleDragOver}
          onDrop={handleDrop}
        >
          {attachments.length > 0 && (
            <div className="flex flex-wrap gap-2 px-3 pt-3">
              {attachments.map((att) => (
                <AttachmentPreview
                  att={att}
                  key={att.id}
                  onRemove={() => removeAttachment(att.id)}
                  removeLabel={t('session.composer.attachment.remove')}
                />
              ))}
            </div>
          )}
          {error && (
            <p className="mx-3 mt-2 text-xs text-destructive">{error}</p>
          )}
          {isDragOver && (
            <div className="pointer-events-none absolute inset-0 flex items-center justify-center">
              <span className="rounded-md bg-primary/10 px-3 py-1.5 text-sm font-medium text-primary">
                {t('session.composer.attachment.dropHere')}
              </span>
            </div>
          )}
          <textarea
            ref={inputRef}
            aria-label={t('session.composer.message')}
            className="max-h-48 min-h-20 w-full resize-none bg-transparent px-4 py-3 text-sm leading-6 text-foreground outline-none placeholder:text-muted-foreground"
            disabled={isRunning}
            onChange={(event) => setInput(event.target.value)}
            onKeyDown={(event) => {
              if (matchesSendShortcut(event, sendShortcut)) {
                event.preventDefault()
                submit()
              }
            }}
            onPaste={handlePaste}
            placeholder={t('session.composer.placeholder')}
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
            <div className="flex items-center gap-1.5">
              <Button
                aria-label={t('session.composer.attach')}
                disabled={isRunning}
                onClick={() => fileInputRef.current?.click()}
                size="icon"
                title={t('session.composer.attach')}
                variant="ghost"
              >
                <Paperclip className="size-4" />
              </Button>
              <ModeSelector
                disabled={false}
                mode={mode}
                onModeChange={onModeChange}
              />
              <PermissionSelectors
                disabled={isRunning}
                fileAccess={fileAccess}
                onFileAccessChange={(value) =>
                  updatePermission('fileAccess', value)
                }
                onShellAccessChange={(value) =>
                  updatePermission('shellAccess', value)
                }
                shellAccess={shellAccess}
              />
            </div>
            <div className="flex items-center gap-1.5">
              <SessionModelPicker
                disabled={isRunning}
                key={session.id}
                session={session}
              />
              {isRunning ? (
                <Button
                  aria-label={t('session.composer.stop')}
                  onClick={onStop}
                  size="icon"
                >
                  <CircleStop className="size-4" />
                </Button>
              ) : (
                <div className="flex overflow-hidden rounded-md bg-primary text-primary-foreground">
                  <Button
                    aria-label={t('session.composer.send')}
                    className="border-0 bg-transparent hover:bg-primary/90 rounded-none"
                    disabled={!canSubmit}
                    onClick={submit}
                    size="icon"
                    title={`${t('session.composer.send')} (${sendShortcutLabel(sendShortcut, t)})`}
                  >
                    <Send
                      className={`size-4 ${canSubmit ? '' : 'opacity-50'}`}
                    />
                  </Button>
                  <SendShortcutPicker inputRef={inputRef} />
                </div>
              )}
            </div>
          </div>
        </div>
        <p className="mt-2 text-center text-[11px] text-muted-foreground">
          {t('session.composer.disclaimer')}
        </p>
      </div>
    </footer>
  )
})

function sendShortcutLabel(
  shortcut: import('@/app/preferences-context').SendShortcut,
  t: ReturnType<typeof useI18n>['t'],
): string {
  switch (shortcut) {
    case 'shift-enter':
      return t('settings.general.conversation.sendShortcut.shiftEnter')
    case 'command-enter':
      return t('settings.general.conversation.sendShortcut.commandEnter')
    default:
      return t('settings.general.conversation.sendShortcut.enter')
  }
}
