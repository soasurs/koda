import { Link } from '@tanstack/react-router'
import {
  Archive,
  Blocks,
  Network,
  Settings2,
  SlidersHorizontal,
} from 'lucide-react'
import type { ReactNode } from 'react'

import { useI18n } from '@/app/i18n'
import { SidebarExpandButton } from '@/components/layout/sidebar-expand-button'

export function SettingsLayout({
  active,
  children,
}: {
  active: 'general' | 'providers' | 'skills' | 'mcp' | 'sessions'
  children: ReactNode
}) {
  const { t } = useI18n()
  const itemClass = (selected: boolean) =>
    `flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition ${
      selected
        ? 'bg-accent text-accent-foreground'
        : 'text-muted-foreground hover:bg-accent/60 hover:text-foreground'
    }`

  return (
    <section className="mx-auto flex h-full w-full max-w-6xl flex-col px-5 pt-8 sm:px-8 sm:pt-10">
      <div className="flex shrink-0 items-start gap-3 border-b border-border pb-6">
        <SidebarExpandButton />
        <div className="min-w-0 flex-1">
          <p className="text-xs font-medium uppercase tracking-[0.18em] text-muted-foreground">
            Koda Studio
          </p>
          <h1 className="mt-2 text-xl font-semibold tracking-tight">
            Settings
          </h1>
          <p className="mt-2 max-w-xl text-sm leading-6 text-muted-foreground">
            Inspect process-wide capabilities and configure your local Koda
            service.
          </p>
        </div>
      </div>

      <div className="grid min-h-0 flex-1 grid-rows-[auto_minmax(0,1fr)] gap-8 pt-6 md:grid-cols-[12rem_minmax(0,1fr)] md:grid-rows-[minmax(0,1fr)]">
        <nav aria-label="Settings" className="grid gap-1 self-start">
          <Link
            aria-current={active === 'general' ? 'page' : undefined}
            className={itemClass(active === 'general')}
            to="/settings/general"
          >
            <SlidersHorizontal className="size-4" aria-hidden="true" />
            {t('settings.general.title')}
          </Link>
          <Link
            aria-current={active === 'providers' ? 'page' : undefined}
            className={itemClass(active === 'providers')}
            to="/settings/providers"
          >
            <Settings2 className="size-4" aria-hidden="true" />
            Providers
          </Link>
          <Link
            aria-current={active === 'sessions' ? 'page' : undefined}
            className={itemClass(active === 'sessions')}
            to="/settings/sessions"
          >
            <Archive className="size-4" aria-hidden="true" />
            Sessions
          </Link>
          <Link
            aria-current={active === 'mcp' ? 'page' : undefined}
            className={itemClass(active === 'mcp')}
            to="/settings/mcp"
          >
            <Network className="size-4" aria-hidden="true" />
            MCP
          </Link>
          <Link
            aria-current={active === 'skills' ? 'page' : undefined}
            className={itemClass(active === 'skills')}
            to="/settings/skills"
          >
            <Blocks className="size-4" aria-hidden="true" />
            Skills
          </Link>
        </nav>

        <div className="min-h-0 min-w-0 overflow-y-auto pb-8 pr-1 sm:pb-10">
          {children}
        </div>
      </div>
    </section>
  )
}
