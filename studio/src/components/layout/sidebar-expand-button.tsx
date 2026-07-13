import { PanelLeftOpen } from 'lucide-react'

import { useSidebar } from '@/components/layout/sidebar-context'

export function SidebarExpandButton() {
  const { collapsed, setCollapsed } = useSidebar()

  if (!collapsed) return null

  return (
    <button
      aria-label="Expand sidebar"
      className="icon-button shrink-0"
      onClick={() => {
        setCollapsed(false)
        window.localStorage.setItem('koda-studio-sidebar-collapsed', 'false')
      }}
      type="button"
    >
      <PanelLeftOpen className="size-4" aria-hidden="true" />
    </button>
  )
}
