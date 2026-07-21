import { create } from '@bufbuild/protobuf'

import {
  ImageDetail,
  ImageSchema,
  PartSchema,
  type Part,
} from '@/gen/koda/v1/service_pb'

export const MAX_ATTACHMENT_BYTES = 10 * 1024 * 1024

export type ComposerAttachment = {
  id: string
  mimeType: string
  data: Uint8Array
  previewUrl: string
  name: string
}

export type ComposerInput = {
  text: string
  attachments: ComposerAttachment[]
}

export const emptyComposerInput: ComposerInput = { text: '', attachments: [] }

export function isComposerInputEmpty(input: ComposerInput): boolean {
  return !input.text.trim() && input.attachments.length === 0
}

export type AttachmentLoadError = {
  reason: 'not-image' | 'too-large' | 'read-failed'
  name: string
}

export async function fileToAttachment(
  file: File,
): Promise<ComposerAttachment | AttachmentLoadError> {
  if (!file.type.startsWith('image/')) {
    return { reason: 'not-image', name: file.name }
  }
  if (file.size > MAX_ATTACHMENT_BYTES) {
    return { reason: 'too-large', name: file.name }
  }
  try {
    const buffer = await file.arrayBuffer()
    if (buffer.byteLength === 0) {
      return { reason: 'read-failed', name: file.name }
    }
    return {
      id: crypto.randomUUID(),
      mimeType: file.type || 'application/octet-stream',
      data: new Uint8Array(buffer),
      previewUrl: URL.createObjectURL(file),
      name: file.name,
    }
  } catch {
    return { reason: 'read-failed', name: file.name }
  }
}
export function bytesToDataURL(data: Uint8Array, mimeType: string): string {
  let binary = ''
  const chunkSize = 0x8000
  for (let offset = 0; offset < data.length; offset += chunkSize) {
    binary += String.fromCharCode(...data.subarray(offset, offset + chunkSize))
  }
  return `data:${mimeType};base64,${btoa(binary)}`
}

export function attachmentToPart(att: ComposerAttachment): Part {
  return create(PartSchema, {
    content: {
      case: 'image',
      value: create(ImageSchema, {
        source: { case: 'data', value: att.data },
        mimeType: att.mimeType,
        detail: ImageDetail.AUTO,
      }),
    },
  })
}

export function revokeAttachment(att: ComposerAttachment): void {
  if (att.previewUrl.startsWith('blob:')) URL.revokeObjectURL(att.previewUrl)
}

export function revokeAttachments(attachments: ComposerAttachment[]): void {
  for (const att of attachments) URL.revokeObjectURL(att.previewUrl)
}
