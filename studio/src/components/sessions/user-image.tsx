import { useMemo } from 'react'

import { bytesToDataURL } from '@/lib/composer-attachments'
import type { Part } from '@/gen/koda/v1/service_pb'
import { ImageViewer } from '@/components/ui/image-viewer'

export function UserImage({ part }: { part: Part }) {
  const image = part.content.case === 'image' ? part.content.value : null
  const source = image?.source ?? null

  const dataURL = useMemo(() => {
    if (!image || !source || source.case !== 'data') return ''
    return bytesToDataURL(source.value, image.mimeType || 'image/png')
  }, [image, source])

  if (!image || !source) return null
  const src = source.case === 'url' ? source.value : dataURL
  if (!src) return null

  return (
    <ImageViewer
      alt="User attachment"
      className="max-h-48 rounded-xl border border-border object-contain"
      src={src}
    />
  )
}
