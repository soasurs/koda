import type { ReactNode } from 'react'

import { I18nProvider } from '@/app/i18n'
import { PreferencesProvider } from '@/app/preferences-provider'
import { ThemeProvider } from '@/app/theme'

export function AllProviders({ children }: { children: ReactNode }) {
  return (
    <ThemeProvider>
      <I18nProvider>
        <PreferencesProvider>{children}</PreferencesProvider>
      </I18nProvider>
    </ThemeProvider>
  )
}
