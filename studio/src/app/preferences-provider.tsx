import { useCallback, useMemo, useState, type ReactNode } from 'react'

import {
  defaultPreferences,
  type Preferences,
  type PreferencesContextValue,
  type SendShortcut,
} from '@/app/preferences-context'
import { PreferencesContext } from '@/app/preferences-context-value'

const storageKey = 'koda-studio-preferences'
const legacySendShortcutKey = 'koda-studio-send-shortcut'

function isSendShortcut(value: unknown): value is SendShortcut {
  return (
    value === 'enter' || value === 'shift-enter' || value === 'command-enter'
  )
}

function loadPreferences(): Preferences {
  const stored = window.localStorage.getItem(storageKey)
  let parsed: Partial<Preferences> = {}
  if (stored) {
    try {
      parsed = JSON.parse(stored) as Partial<Preferences>
    } catch {
      parsed = {}
    }
  } else {
    // One-time migration from the legacy send-shortcut storage key.
    const legacy = window.localStorage.getItem(legacySendShortcutKey)
    if (isSendShortcut(legacy)) {
      parsed.sendShortcut = legacy
      window.localStorage.removeItem(legacySendShortcutKey)
    }
  }

  const sendShortcut = isSendShortcut(parsed.sendShortcut)
    ? parsed.sendShortcut
    : defaultPreferences.sendShortcut

  const preferences: Preferences = {
    expandReasoning:
      typeof parsed.expandReasoning === 'boolean'
        ? parsed.expandReasoning
        : defaultPreferences.expandReasoning,
    expandToolCalls:
      typeof parsed.expandToolCalls === 'boolean'
        ? parsed.expandToolCalls
        : defaultPreferences.expandToolCalls,
    sendShortcut,
  }

  // Persist the resolved preferences so a migrated legacy value lands in the
  // new storage key and the legacy key can be safely removed.
  if (!stored) {
    window.localStorage.setItem(storageKey, JSON.stringify(preferences))
  }

  return preferences
}

export function PreferencesProvider({ children }: { children: ReactNode }) {
  const [preferences, setPreferences] = useState<Preferences>(loadPreferences)

  const setPreference = useCallback(
    <K extends keyof Preferences>(key: K, value: Preferences[K]) => {
      setPreferences((current) => {
        const next = { ...current, [key]: value }
        window.localStorage.setItem(storageKey, JSON.stringify(next))
        return next
      })
    },
    [],
  )

  const value = useMemo<PreferencesContextValue>(
    () => ({ ...preferences, setPreference }),
    [preferences, setPreference],
  )

  return <PreferencesContext value={value}>{children}</PreferencesContext>
}
