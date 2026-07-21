import { useEffect, useRef, useState, type ChangeEvent } from 'react'

import { useI18n, type TKey } from '@/app/i18n'
import type { ComposerAttachment } from '@/lib/composer-attachments'
import {
  fileToAttachment,
  revokeAttachment,
  revokeAttachments,
  type AttachmentLoadError,
} from '@/lib/composer-attachments'

export function useAttachments(initialAttachments: ComposerAttachment[]) {
  const { t } = useI18n()
  const [attachments, setAttachments] = useState(initialAttachments)
  const [error, setError] = useState('')
  const fileInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    return () => revokeAttachments(initialAttachments)
  }, [initialAttachments])

  function reportErrors(errors: AttachmentLoadError[]) {
    if (errors.length === 0) {
      setError('')
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
    setError(t(key, { name: first.name }))
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
    reportErrors(errors)
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

  return {
    attachments,
    setAttachments,
    error,
    setError,
    fileInputRef,
    addFiles,
    removeAttachment,
    reportErrors,
    handlePaste,
    handleFileInputChange,
  }
}
