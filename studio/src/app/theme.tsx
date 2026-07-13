import { useEffect, useState, type ReactNode } from 'react'

import { ThemeContext, type ThemePreference } from '@/app/theme-context'
const storageKey = 'koda-studio-theme'

function applyTheme(preference: ThemePreference) {
  const dark =
    preference === 'dark' ||
    (preference === 'system' &&
      window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', dark)
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [preference, setPreferenceState] = useState<ThemePreference>(() => {
    const stored = window.localStorage.getItem(storageKey)
    return stored === 'light' || stored === 'dark' ? stored : 'system'
  })

  useEffect(() => {
    applyTheme(preference)
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const handleChange = () => {
      if (preference === 'system') applyTheme('system')
    }
    media.addEventListener('change', handleChange)
    return () => media.removeEventListener('change', handleChange)
  }, [preference])

  function setPreference(value: ThemePreference) {
    setPreferenceState(value)
    window.localStorage.setItem(storageKey, value)
    applyTheme(value)
  }

  return (
    <ThemeContext value={{ preference, setPreference }}>
      {children}
    </ThemeContext>
  )
}
