import { useQuery } from '@tanstack/react-query'
import { Link, Outlet } from '@tanstack/react-router'
import {
  Bot,
  ChevronRight,
  Folder,
  LoaderCircle,
  MessageSquare,
  PanelLeftClose,
  Plus,
  Settings2,
} from 'lucide-react'
import { useState } from 'react'

import { CreateSessionDialog } from '@/components/sessions/create-session-dialog'
import { SidebarContext } from '@/components/layout/sidebar-context'
import { ThemeToggle } from '@/components/theme-toggle'
import { errorMessage, kodaKeys, listSessions } from '@/lib/koda'

const sidebarCollapsedKey = 'koda-studio-sidebar-collapsed'

function loadSidebarCollapsed(): boolean {
  return window.localStorage.getItem(sidebarCollapsedKey) === 'true'
}

export function AppShell() {
  const [sidebarCollapsed, setSidebarCollapsed] = useState(loadSidebarCollapsed)
  const [createSessionWorkdir, setCreateSessionWorkdir] = useState<
    string | undefined
  >()
  const sessionsQuery = useQuery({
    queryKey: kodaKeys.sessions,
    queryFn: listSessions,
  })
  const sessionGroups = Object.values(
    (sessionsQuery.data ?? []).reduce<
      Record<
        string,
        { name: string; path: string; sessions: typeof sessionsQuery.data }
      >
    >((groups, session) => {
      const group = groups[session.workdir] ?? {
        name: session.workdir.split('/').pop() || session.workdir,
        path: session.workdir,
        sessions: [],
      }
      group.sessions?.push(session)
      groups[session.workdir] = group
      return groups
    }, {}),
  )

  return (
    <SidebarContext.Provider
      value={{ collapsed: sidebarCollapsed, setCollapsed: setSidebarCollapsed }}
    >
      <div
        className={`grid h-svh grid-cols-1 overflow-hidden bg-neutral-950 text-neutral-100 ${sidebarCollapsed ? '' : 'md:grid-cols-[17rem_minmax(0,1fr)]'}`}
      >
        <aside
          className={`hidden h-svh border-r border-neutral-800/80 bg-neutral-950 ${sidebarCollapsed ? '' : 'md:flex md:flex-col'}`}
        >
          <div className="flex h-12 items-center gap-2 border-b border-neutral-800/80 px-4">
            <div className="flex size-7 items-center justify-center rounded-md bg-neutral-100 text-neutral-950">
              <Bot className="size-4" aria-hidden="true" />
            </div>
            <span className="text-sm font-semibold tracking-tight">
              Koda Studio
            </span>
            <button
              aria-label="Collapse sidebar"
              className="icon-button ml-auto"
              onClick={() => {
                const next = !sidebarCollapsed
                setSidebarCollapsed(next)
                window.localStorage.setItem(sidebarCollapsedKey, String(next))
              }}
              type="button"
            >
              <PanelLeftClose className="size-4" aria-hidden="true" />
            </button>
          </div>

          <div className="p-3">
            <button
              className="button-primary w-full"
              onClick={() => setCreateSessionWorkdir('')}
              type="button"
            >
              <Plus className="size-4" aria-hidden="true" />
              New session
            </button>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto px-3 pb-3">
            <p className="px-2.5 pb-2 text-[11px] font-medium uppercase tracking-wider text-neutral-600">
              Projects
            </p>
            {sessionsQuery.isPending ? (
              <LoaderCircle className="mx-auto mt-4 size-4 animate-spin text-neutral-600" />
            ) : sessionsQuery.isError ? (
              <p className="px-2.5 text-xs leading-5 text-red-400">
                {errorMessage(sessionsQuery.error)}
              </p>
            ) : sessionsQuery.data.length === 0 ? (
              <p className="px-2.5 text-xs leading-5 text-neutral-600">
                No sessions yet
              </p>
            ) : (
              <div className="space-y-1">
                {sessionGroups.map((group) => (
                  <div className="relative" key={group.path}>
                    <details className="group/project" open>
                      <summary
                        className="flex min-w-0 cursor-pointer list-none items-center gap-2 rounded-md py-2 pl-2.5 pr-16 text-xs font-medium text-neutral-400 transition-colors hover:bg-neutral-900 hover:text-neutral-200 [&::-webkit-details-marker]:hidden"
                        title={group.path}
                      >
                        <ChevronRight className="size-3 shrink-0 transition-transform group-open/project:rotate-90" />
                        <Folder className="size-3.5 shrink-0" />
                        <span className="truncate">{group.name}</span>
                      </summary>
                      <div className="ml-4 space-y-0.5 border-l border-neutral-800 pl-2">
                        {group.sessions?.map((session) => (
                          <Link
                            activeProps={{
                              className: 'bg-neutral-900 text-neutral-100',
                            }}
                            className="flex min-w-0 items-center gap-2 rounded-md px-2.5 py-2 text-sm text-neutral-500 hover:bg-neutral-900 hover:text-neutral-200"
                            key={session.id}
                            params={{ sessionId: session.id }}
                            to="/sessions/$sessionId"
                          >
                            <MessageSquare className="size-3.5 shrink-0" />
                            <span className="truncate">
                              {session.title || 'Untitled session'}
                            </span>
                          </Link>
                        ))}
                      </div>
                    </details>
                    <div className="absolute right-1.5 top-1 flex items-center gap-0.5">
                      <span className="px-1 text-[10px] text-neutral-600">
                        {group.sessions?.length}
                      </span>
                      <button
                        aria-label={`New session in ${group.name}`}
                        className="icon-button size-6"
                        onClick={() => setCreateSessionWorkdir(group.path)}
                        title={`New session in ${group.path}`}
                        type="button"
                      >
                        <Plus className="size-3.5" aria-hidden="true" />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          <div className="flex items-center justify-between border-t border-neutral-800/80 px-4 py-3">
            <ThemeToggle />
            <Link
              aria-label="Settings"
              className="icon-button"
              to="/settings/providers"
            >
              <Settings2 className="size-4" aria-hidden="true" />
            </Link>
          </div>
        </aside>

        <main className="flex min-h-0 min-w-0 flex-col overflow-hidden">
          {!sidebarCollapsed && (
            <header className="flex h-14 shrink-0 items-center justify-between border-b border-neutral-800/80 px-4 md:hidden">
              <div className="flex items-center gap-2">
                <span className="text-sm font-semibold">Koda Studio</span>
              </div>
              <button
                className="button-secondary px-2.5 py-1.5"
                onClick={() => setCreateSessionWorkdir('')}
                type="button"
              >
                <Plus className="size-4" />
                New
              </button>
              <ThemeToggle compact />
            </header>
          )}
          <div className="min-h-0 flex-1">
            <Outlet />
          </div>
        </main>
        {createSessionWorkdir !== undefined && (
          <CreateSessionDialog
            initialWorkdir={createSessionWorkdir}
            onClose={() => setCreateSessionWorkdir(undefined)}
          />
        )}
      </div>
    </SidebarContext.Provider>
  )
}
