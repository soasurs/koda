import { Monitor, Moon, Sun } from 'lucide-react'

import { useI18n } from '@/app/i18n'
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
  const { t } = useI18n()
  const Icon = themeIcons[preference]

  return (
    <label className="relative flex items-center gap-2 text-xs text-muted-foreground">
      <Icon className="size-3.5 shrink-0" />
      {!compact && <span>{t('theme.toggle.label')}</span>}
      <Select
        onValueChange={(value) => setPreference(value as ThemePreference)}
        value={preference}
      >
        <SelectTrigger
          aria-label={t('theme.toggle.ariaLabel')}
          className="h-auto border-none bg-transparent px-0 py-0 pr-4 text-xs text-muted-foreground hover:text-foreground [&>svg]:size-3"
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="system">
            {t('settings.general.appearance.theme.system')}
          </SelectItem>
          <SelectItem value="light">
            {t('settings.general.appearance.theme.light')}
          </SelectItem>
          <SelectItem value="dark">
            {t('settings.general.appearance.theme.dark')}
          </SelectItem>
        </SelectContent>
      </Select>
    </label>
  )
}
