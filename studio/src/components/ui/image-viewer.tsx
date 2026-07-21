import { X } from 'lucide-react'
import { useEffect, useState } from 'react'

import { cn } from '@/lib/utils'

type ImageViewerProps = {
  alt: string
  className?: string
  src: string
}

export function ImageViewer({ alt, className, src }: ImageViewerProps) {
  const [open, setOpen] = useState(false)

  useEffect(() => {
    if (!open) return
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [open])

  return (
    <>
      <button
        aria-label={`View ${alt} enlarged`}
        className="block cursor-zoom-in rounded-md focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
        onClick={() => setOpen(true)}
        type="button"
      >
        <img alt={alt} className={className} src={src} />
      </button>
      {open && (
        <div
          aria-label={alt}
          aria-modal="true"
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 p-4"
          onClick={() => setOpen(false)}
          role="dialog"
        >
          <button
            aria-label="Close image preview"
            className="absolute right-4 top-4 z-10 flex size-9 items-center justify-center rounded-full bg-black/50 text-white hover:bg-black/70 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-white"
            onClick={() => setOpen(false)}
            type="button"
          >
            <X className="size-5" />
          </button>
          <img
            alt={alt}
            className={cn(
              'max-h-[calc(100vh-2rem)] max-w-full object-contain',
              'relative z-[1]',
            )}
            onClick={(event) => event.stopPropagation()}
            src={src}
          />
        </div>
      )}
    </>
  )
}
