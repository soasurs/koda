import {
  ChevronUp,
  CircleStop,
  ClipboardList,
  Eye,
  FilePen,
  Folder,
  Globe,
  Hammer,
  Paperclip,
  Send,
  ShieldQuestion,
  Terminal,
  X,
  Zap,
} from 'lucide-react'
import {
  memo,
  useEffect,
  useRef,
  useState,
  type ChangeEvent,
  type DragEvent,
  type RefObject,
} from 'react'

import { useI18n, type TKey } from '@/app/i18n'
import type { SendShortcut } from '@/app/preferences-context'
import { usePreferences } from '@/app/preferences-context-value'
import { ImageViewer } from '@/components/ui/image-viewer'
import { Button } from '@/components/ui/button'
import { SessionModelPicker } from '@/components/sessions/session-model-picker'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { Session } from '@/gen/koda/v1/service_pb'
import { AgentMode, FileAccess, ShellAccess } from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'
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
  const { sendShortcut, setPreference } = usePreferences()
  const [input, setInput] = useState(initialInput.text)
  const [attachments, setAttachments] = useState<ComposerAttachment[]>(
    initialInput.attachments,
  )
  const [attachmentError, setAttachmentError] = useState('')
  const [isDragOver, setIsDragOver] = useState(false)
  const [fileAccess, setFileAccess] = useState(session.fileAccess)
  const [shellAccess, setShellAccess] = useState(session.shellAccess)
  const wasRunningRef = useRef(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (wasRunningRef.current && !isRunning) {
      requestAnimationFrame(() => inputRef.current?.focus())
    }
    wasRunningRef.current = isRunning
  }, [isRunning, inputRef])

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

  function handleFileInputChange(event: ChangeEvent<HTMLInputElement>) {
    if (event.target.files && event.target.files.length > 0) {
      void addFiles(event.target.files)
    }
    event.target.value = ''
  }

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

  const shortcutOptions: { label: string; shortcut: SendShortcut }[] = [
    {
      label: t('settings.general.conversation.sendShortcut.enter'),
      shortcut: 'enter',
    },
    {
      label: t('settings.general.conversation.sendShortcut.shiftEnter'),
      shortcut: 'shift-enter',
    },
    {
      label: t('settings.general.conversation.sendShortcut.commandEnter'),
      shortcut: 'command-enter',
    },
  ]
  const sendShortcutLabel = shortcutOptions.find(
    (option) => option.shortcut === sendShortcut,
  )!.label

  function selectSendShortcut(shortcut: SendShortcut) {
    setPreference('sendShortcut', shortcut)
    requestAnimationFrame(() => inputRef.current?.focus())
  }

  function submit() {
    if (isRunning) return
    const payload: ComposerInput = { text: input, attachments }
    if (isComposerInputEmpty(payload)) return
    setInput('')
    setAttachments([])
    setAttachmentError('')
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
          {attachmentError && (
            <p className="mx-3 mt-2 text-xs text-destructive">
              {attachmentError}
            </p>
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
              <div className="relative">
                <Select
                  disabled={false}
                  onValueChange={(value) =>
                    onModeChange(Number(value) as AgentMode)
                  }
                  value={String(mode)}
                >
                  <SelectTrigger className="inline-flex h-auto w-auto items-center gap-1 whitespace-nowrap rounded-md border border-border bg-background py-1.5 pl-3 pr-7 text-xs font-medium text-foreground hover:border-border/80 [&>svg]:hidden">
                    <SelectValue />
                  </SelectTrigger>
                  <ChevronUp className="pointer-events-none absolute right-2 top-1/2 size-3 -translate-y-1/2 text-muted-foreground" />
                  <SelectContent side="top">
                    <SelectItem value={String(AgentMode.BUILD)}>
                      <span className="flex items-center gap-2">
                        <Hammer className="size-4 shrink-0" />
                        {t('session.composer.mode.build')}
                      </span>
                    </SelectItem>
                    <SelectItem value={String(AgentMode.PLAN)}>
                      <span className="flex items-center gap-2">
                        <ClipboardList className="size-4 shrink-0" />
                        {t('session.composer.mode.plan')}
                      </span>
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <span className="relative inline-flex rounded-md border border-border">
                <Select
                  disabled={isRunning}
                  onValueChange={(value) =>
                    updatePermission('fileAccess', Number(value) as FileAccess)
                  }
                  value={String(fileAccess)}
                >
                  <SelectTrigger className="inline-flex h-auto w-auto items-center gap-1 whitespace-nowrap rounded-none rounded-l-md border-0 border-r border-border bg-background px-2.5 py-1.5 text-xs font-medium text-foreground hover:bg-muted/50 [&>svg:last-child]:rotate-180">
                    <Folder className="size-3.5 shrink-0 text-muted-foreground" />
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent side="top">
                    <SelectItem value={String(FileAccess.WORKSPACE_READ)}>
                      <span className="flex items-center gap-2">
                        <Eye className="size-4 shrink-0" />
                        {t('session.composer.fileAccess.read')}
                      </span>
                    </SelectItem>
                    <SelectItem value={String(FileAccess.WORKSPACE_WRITE)}>
                      <span className="flex items-center gap-2">
                        <FilePen className="size-4 shrink-0" />
                        {t('session.composer.fileAccess.write')}
                      </span>
                    </SelectItem>
                    <SelectItem value={String(FileAccess.UNRESTRICTED)}>
                      <span className="flex items-center gap-2">
                        <Globe className="size-4 shrink-0" />
                        {t('session.composer.fileAccess.full')}
                      </span>
                    </SelectItem>
                  </SelectContent>
                </Select>
                <Select
                  disabled={isRunning}
                  onValueChange={(value) =>
                    updatePermission(
                      'shellAccess',
                      Number(value) as ShellAccess,
                    )
                  }
                  value={String(shellAccess)}
                >
                  <SelectTrigger className="inline-flex h-auto w-auto items-center gap-1 whitespace-nowrap rounded-none rounded-r-md border-0 bg-background px-2.5 py-1.5 text-xs font-medium text-foreground hover:bg-muted/50 [&>svg:last-child]:rotate-180">
                    <Terminal className="size-3.5 shrink-0 text-muted-foreground" />
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent side="top">
                    <SelectItem value={String(ShellAccess.APPROVAL_REQUIRED)}>
                      <span className="flex items-center gap-2">
                        <ShieldQuestion className="size-4 shrink-0" />
                        {t('session.composer.shellAccess.ask')}
                      </span>
                    </SelectItem>
                    <SelectItem value={String(ShellAccess.UNRESTRICTED)}>
                      <span className="flex items-center gap-2">
                        <Zap className="size-4 shrink-0" />
                        {t('session.composer.shellAccess.free')}
                      </span>
                    </SelectItem>
                  </SelectContent>
                </Select>
              </span>
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
                    title={`${t('session.composer.send')} (${sendShortcutLabel})`}
                  >
                    <Send
                      className={`size-4 ${canSubmit ? '' : 'opacity-50'}`}
                    />
                  </Button>
                  <DropdownMenu
                    onOpenChange={(open) => {
                      if (!open)
                        requestAnimationFrame(() => inputRef.current?.focus())
                    }}
                  >
                    <DropdownMenuTrigger asChild>
                      <Button
                        aria-label={t('session.composer.chooseSendShortcut')}
                        className="border-0 border-l border-primary-foreground/30 bg-transparent text-primary-foreground/70 hover:bg-primary/90 hover:text-primary-foreground rounded-none"
                        size="icon"
                        title={`${t('session.composer.chooseSendShortcut')}: ${sendShortcutLabel}`}
                      >
                        <ChevronUp className="size-3" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" side="top" sideOffset={8}>
                      <DropdownMenuLabel>
                        {t('session.composer.sendWith')}
                      </DropdownMenuLabel>
                      <DropdownMenuRadioGroup
                        onValueChange={(value) =>
                          selectSendShortcut(value as SendShortcut)
                        }
                        value={sendShortcut}
                      >
                        {shortcutOptions.map((option) => (
                          <DropdownMenuRadioItem
                            key={option.shortcut}
                            value={option.shortcut}
                          >
                            {option.label}
                          </DropdownMenuRadioItem>
                        ))}
                      </DropdownMenuRadioGroup>
                    </DropdownMenuContent>
                  </DropdownMenu>
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

function AttachmentPreview({
  att,
  onRemove,
  removeLabel,
}: {
  att: ComposerAttachment
  onRemove: () => void
  removeLabel: string
}) {
  return (
    <div className="group relative overflow-hidden rounded-md border border-border bg-background">
      <ImageViewer
        alt={att.name}
        className="size-16 object-cover"
        src={att.previewUrl}
      />
      <button
        aria-label={removeLabel}
        className="absolute right-0.5 top-0.5 flex size-5 items-center justify-center rounded-full bg-background/90 text-foreground opacity-0 transition-opacity group-hover:opacity-100"
        onClick={onRemove}
        type="button"
      >
        <X className="size-3" />
      </button>
    </div>
  )
}
