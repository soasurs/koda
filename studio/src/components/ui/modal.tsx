import { X } from 'lucide-react'
import { useId, type ReactNode } from 'react'

type ModalProps = {
  children: ReactNode
  description?: string
  onClose: () => void
  title: string
  wide?: boolean
}

export function Modal({
  children,
  description,
  onClose,
  title,
  wide = false,
}: ModalProps) {
  const titleId = useId()

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm"
      role="presentation"
      onMouseDown={(event) => {
        if (event.currentTarget === event.target) onClose()
      }}
    >
      <section
        aria-labelledby={titleId}
        aria-modal="true"
        className={`max-h-[90svh] w-full overflow-y-auto rounded-xl border border-border bg-popover shadow-2xl ${wide ? 'max-w-2xl' : 'max-w-lg'}`}
        role="dialog"
      >
        <header className="flex items-start justify-between border-b border-border px-5 py-4">
          <div>
            <h2 id={titleId} className="font-medium text-foreground">
              {title}
            </h2>
            {description && (
              <p className="mt-1 text-sm text-muted-foreground">
                {description}
              </p>
            )}
          </div>
          <button
            aria-label="Close"
            className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-accent-foreground"
            onClick={onClose}
            type="button"
          >
            <X className="size-4" />
          </button>
        </header>
        {children}
      </section>
    </div>
  )
}
