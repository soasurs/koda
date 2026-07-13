import {
  createRootRoute,
  createRoute,
  createRouter,
} from '@tanstack/react-router'

import { AppShell } from '@/components/layout/app-shell'
import { HomePage } from '@/pages/home-page'
import { ProviderSettingsPage } from '@/pages/provider-settings-page'
import { SessionPage } from '@/pages/session-page'

const rootRoute = createRootRoute({
  component: AppShell,
})

const homeRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: HomePage,
})

const providerSettingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings/providers',
  component: ProviderSettingsPage,
})

const sessionRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/sessions/$sessionId',
  component: SessionPage,
})

const routeTree = rootRoute.addChildren([
  homeRoute,
  sessionRoute,
  providerSettingsRoute,
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
