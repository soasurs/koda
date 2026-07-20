import { createContext, useContext } from 'react'

import type { PreferencesContextValue } from '@/app/preferences-context'

export const PreferencesContext = createContext<PreferencesContextValue | null>(
  null,
)

export function usePreferences(): PreferencesContextValue {
  const context = useContext(PreferencesContext)
  if (!context)
    throw new Error('usePreferences must be used within PreferencesProvider')
  return context
}
