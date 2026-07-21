import { describe, expect, it, vi } from 'vitest'

import {
  MAX_ATTACHMENT_BYTES,
  attachmentToPart,
  fileToAttachment,
  isComposerInputEmpty,
} from '@/lib/composer-attachments'
import { ImageDetail } from '@/gen/koda/v1/service_pb'

function makeFile(options: {
  name?: string
  type?: string
  size?: number
  contents?: Uint8Array
}): File {
  const type = options.type ?? 'image/png'
  const contents =
    options.contents ?? new Uint8Array(options.size ?? 1024).fill(0x42)
  const file = new File([contents as BlobPart], options.name ?? 'image.png', {
    type,
  })
  return file
}

describe('fileToAttachment', () => {
  it('converts an image file into a composer attachment', async () => {
    const contents = new Uint8Array([1, 2, 3, 4])
    const result = await fileToAttachment(
      makeFile({ name: 'photo.png', type: 'image/png', contents }),
    )
    if (!('id' in result)) throw new Error('expected attachment')
    expect(result.mimeType).toBe('image/png')
    expect(result.name).toBe('photo.png')
    expect(Array.from(result.data)).toEqual([1, 2, 3, 4])
    expect(result.previewUrl).toMatch(/^blob:/)
    expect(result.id).toBeTruthy()
  })

  it('rejects non-image files', async () => {
    const result = await fileToAttachment(
      makeFile({ name: 'doc.pdf', type: 'application/pdf' }),
    )
    expect(result).toEqual({ reason: 'not-image', name: 'doc.pdf' })
  })

  it('rejects files exceeding the size limit', async () => {
    const result = await fileToAttachment(
      makeFile({
        name: 'big.png',
        type: 'image/png',
        size: MAX_ATTACHMENT_BYTES + 1,
      }),
    )
    expect(result).toEqual({ reason: 'too-large', name: 'big.png' })
  })

  it('rejects empty payloads', async () => {
    const result = await fileToAttachment(
      makeFile({
        name: 'empty.png',
        type: 'image/png',
        contents: new Uint8Array(0),
      }),
    )
    expect(result).toEqual({ reason: 'read-failed', name: 'empty.png' })
  })

  it('reports a read failure when arrayBuffer rejects', async () => {
    const failing = {
      name: 'broken.png',
      type: 'image/png',
      size: 1024,
      arrayBuffer: () => Promise.reject(new Error('disk gone')),
    } as unknown as File
    const result = await fileToAttachment(failing)
    expect(result).toEqual({ reason: 'read-failed', name: 'broken.png' })
  })
})

describe('attachmentToPart', () => {
  it('builds an image part carrying inline data with AUTO detail', async () => {
    const attachment = await fileToAttachment(
      makeFile({ contents: new Uint8Array([1, 2, 3]) }),
    )
    if (!('id' in attachment)) throw new Error('expected attachment')

    const part = attachmentToPart(attachment)
    expect(part.content.case).toBe('image')
    if (part.content.case !== 'image') throw new Error('expected image part')
    expect(part.content.value.source.case).toBe('data')
    if (part.content.value.source.case !== 'data')
      throw new Error('expected data source')
    expect(Array.from(part.content.value.source.value)).toEqual([1, 2, 3])
    expect(part.content.value.mimeType).toBe('image/png')
    expect(part.content.value.detail).toBe(ImageDetail.AUTO)
  })
})

describe('isComposerInputEmpty', () => {
  it('treats whitespace-only text with no attachments as empty', () => {
    expect(isComposerInputEmpty({ text: '   \n', attachments: [] })).toBe(true)
  })

  it('treats attachments without text as non-empty', () => {
    expect(
      isComposerInputEmpty({
        text: '',
        attachments: [
          {
            id: 'a',
            mimeType: 'image/png',
            data: new Uint8Array([1]),
            previewUrl: 'blob:x',
            name: 'a.png',
          },
        ],
      }),
    ).toBe(false)
  })

  it('treats text with no attachments as non-empty', () => {
    expect(isComposerInputEmpty({ text: 'hello', attachments: [] })).toBe(false)
  })
})

describe('URL lifecycle', () => {
  it('does not throw when revoking a known preview url', async () => {
    const revokeSpy = vi.spyOn(URL, 'revokeObjectURL')
    const attachment = await fileToAttachment(
      makeFile({ contents: new Uint8Array([1]) }),
    )
    if (!('id' in attachment)) throw new Error('expected attachment')
    URL.revokeObjectURL(attachment.previewUrl)
    expect(revokeSpy).toHaveBeenCalledWith(attachment.previewUrl)
    revokeSpy.mockRestore()
  })
})
