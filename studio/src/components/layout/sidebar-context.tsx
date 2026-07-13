import { createContext, useContext } from 'react'

export interface SidebarState {
  collapsed: boolean
  setCollapsed: (collapsed: boolean) => void
}

export const SidebarContext = createContext<SidebarState | null>(null)

export function useSidebar(): SidebarState {
  const ctx = useContext(SidebarContext)
  if (!ctx) throw new Error('useSidebar must be used within AppShell')
  return ctx
}
