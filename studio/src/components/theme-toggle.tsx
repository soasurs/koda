import { Monitor, Moon, Sun } from 'lucide-react'

import { useTheme, type ThemePreference } from '@/app/theme-context'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

const themeIcons = {
  system: Monitor,
  light: Sun,
  dark: Moon,
}

export function ThemeToggle({ compact = false }: { compact?: boolean }) {
  const { preference, setPreference } = useTheme()
  const Icon = themeIcons[preference]

  return (
    <label className="relative flex items-center gap-2 text-xs text-neutral-500">
      <Icon className="size-3.5 shrink-0" />
      {!compact && <span>Theme</span>}
      <Select
        onValueChange={(value) => setPreference(value as ThemePreference)}
        value={preference}
      >
        <SelectTrigger
          aria-label="Theme"
          className="h-auto border-none bg-transparent px-0 py-0 pr-4 text-xs text-neutral-500 hover:text-neutral-200 [&>svg]:size-3"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="system">System</SelectItem>
          <SelectItem value="light">Light</SelectItem>
          <SelectItem value="dark">Dark</SelectItem>
        </SelectContent>
      </Select>
    </label>
  )
}
