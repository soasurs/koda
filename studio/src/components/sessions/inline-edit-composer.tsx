import { Paperclip, Send, X } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'

import { useI18n } from '@/app/i18n'
import { usePreferences } from '@/app/preferences-context-value'
import { ImageViewer } from '@/components/ui/image-viewer'
import { Button } from '@/components/ui/button'
import { useAttachments } from '@/components/sessions/use-attachments'
import { matchesSendShortcut } from '@/lib/send-shortcut'
import type { ComposerInput } from '@/lib/composer-attachments'
import { isComposerInputEmpty } from '@/lib/composer-attachments'

export function InlineEditComposer({
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
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const {
    attachments,
    error,
    fileInputRef,
    removeAttachment,
    handlePaste,
    handleFileInputChange,
  } = useAttachments(initialInput.attachments)

  useEffect(() => {
    textareaRef.current?.focus()
  }, [])

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
          {error && (
            <p className="mx-3 mt-2 text-xs text-destructive">{error}</p>
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
