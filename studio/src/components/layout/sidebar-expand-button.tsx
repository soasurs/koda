import { PanelLeftOpen } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { useSidebar } from '@/components/layout/sidebar-context'

export function SidebarExpandButton() {
  const { collapsed, setCollapsed } = useSidebar()

  if (!collapsed) return null

  return (
    <Button
      aria-label="Expand sidebar"
      onClick={() => {
        setCollapsed(false)
        window.localStorage.setItem('koda-studio-sidebar-collapsed', 'false')
      }}
      size="icon"
      variant="ghost"
    >
      <PanelLeftOpen className="size-4" aria-hidden="true" />
    </Button>
  )
}
