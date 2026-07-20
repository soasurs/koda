import {
  createRootRoute,
  createRoute,
  createRouter,
  redirect,
} from '@tanstack/react-router'

import { AppShell } from '@/components/layout/app-shell'
import { HomePage } from '@/pages/home-page'
import { GeneralSettingsPage } from '@/pages/general-settings-page'
import { MCPSettingsPage } from '@/pages/mcp-settings-page'
import { ProviderSettingsPage } from '@/pages/provider-settings-page'
import { SessionPage } from '@/pages/session-page'
import { SessionSettingsPage } from '@/pages/session-settings-page'
import { SkillSettingsPage } from '@/pages/skill-settings-page'

const rootRoute = createRootRoute({
  component: AppShell,
})

const homeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: HomePage,
})

const settingsIndexRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings',
  beforeLoad: () => {
    throw redirect({ to: '/settings/general' })
  },
})

const generalSettingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/general',
  component: GeneralSettingsPage,
})

const providerSettingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/providers',
  component: ProviderSettingsPage,
})

const skillSettingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/skills',
  component: SkillSettingsPage,
})

const mcpSettingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/mcp',
  component: MCPSettingsPage,
})

const sessionSettingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/sessions',
  component: SessionSettingsPage,
})

const sessionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/sessions/$sessionId',
  component: SessionPage,
})

const routeTree = rootRoute.addChildren([
  homeRoute,
  sessionRoute,
  settingsIndexRoute,
  generalSettingsRoute,
  providerSettingsRoute,
  sessionSettingsRoute,
  mcpSettingsRoute,
  skillSettingsRoute,
])

export const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
