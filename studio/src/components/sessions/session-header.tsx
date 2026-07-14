import type { Session } from '@/gen/koda/v1/service_pb'
import { SidebarExpandButton } from '@/components/layout/sidebar-expand-button'

export function SessionHeader({ session }: { session: Session }) {
  return (
    <header className="flex h-12 shrink-0 items-center gap-3 border-b border-border px-4 sm:px-6">
      <SidebarExpandButton />
      <div className="min-w-0 flex-1">
        <h1 className="truncate text-sm font-medium">
          {session.title || session.workdir.split('/').pop() || 'Untitled'}
        </h1>
      </div>
    </header>
  )
}
