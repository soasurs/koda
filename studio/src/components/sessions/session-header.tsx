import { PanelLeftOpen } from 'lucide-react'

import type { Session } from '@/gen/koda/v1/service_pb'
import { useSidebar } from '@/components/layout/sidebar-context'

export function SessionHeader({ session }: { session: Session }) {
  const { collapsed, setCollapsed } = useSidebar()

  return (
    <header className="flex h-12 shrink-0 items-center gap-3 border-b border-neutral-800 px-4 sm:px-6">
      {collapsed && (
        <button
          aria-label="Expand sidebar"
          className="icon-button"
          onClick={() => {
            setCollapsed(false)
            window.localStorage.setItem(
              'koda-studio-sidebar-collapsed',
              'false',
            )
          }}
          type="button"
        >
          <PanelLeftOpen className="size-4" aria-hidden="true" />
        </button>
      )}
      <div className="min-w-0 flex-1">
        <h1 className="truncate text-sm font-medium">
          {session.title || session.workdir.split('/').pop() || 'Untitled'}
        </h1>
      </div>
    </header>
  )
}
