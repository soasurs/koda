import { X } from 'lucide-react'

import { ImageViewer } from '@/components/ui/image-viewer'
import type { ComposerAttachment } from '@/lib/composer-attachments'

export function AttachmentPreview({
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
