import { Link } from '@tanstack/react-router'
import { Archive, Ellipsis, MessageSquare, Pencil } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from '@/components/ui/context-menu'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import type { Session } from '@/gen/koda/v1/service_pb'

export function SessionListItem({
  archiving,
  onArchive,
  onRename,
  session,
}: {
  archiving: boolean
  onArchive: () => void
  onRename: () => void
  session: Session
}) {
  const label = session.title || 'Untitled session'

  return (
    <div className="group/session relative min-w-0">
      <ContextMenu>
        <ContextMenuTrigger asChild>
          <Link
            activeProps={{
              className: 'bg-accent text-accent-foreground',
            }}
            className="flex min-w-0 items-center gap-2 rounded-md px-2.5 py-2 pr-9 text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
            params={{ sessionId: session.id }}
            to="/sessions/$sessionId"
          >
            <MessageSquare className="size-3.5 shrink-0" />
            <span className="truncate">{label}</span>
          </Link>
        </ContextMenuTrigger>
        <ContextMenuContent>
          <ContextMenuItem onSelect={onRename}>
            <Pencil />
            Rename
          </ContextMenuItem>
          <ContextMenuSeparator />
          <ContextMenuItem disabled={archiving} onSelect={onArchive}>
            <Archive />
            Archive
          </ContextMenuItem>
        </ContextMenuContent>
      </ContextMenu>

      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            aria-label={`Actions for ${label}`}
            className="absolute right-1 top-1 opacity-0 group-hover/session:opacity-100 group-focus-within/session:opacity-100 data-[state=open]:opacity-100"
            disabled={archiving}
            size="icon-xs"
            variant="ghost"
          >
            <Ellipsis aria-hidden="true" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" side="right">
          <DropdownMenuItem onSelect={onRename}>
            <Pencil />
            Rename
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem disabled={archiving} onSelect={onArchive}>
            <Archive />
            Archive
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
