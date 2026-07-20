import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'

import { I18nContext } from '@/app/i18n/i18n-context'
import {
  defaultLocale,
  supportedLocales,
  translate,
  type Locale,
  type TKey,
} from '@/app/i18n/dictionaries'

const storageKey = 'koda-studio-locale'

function detectInitialLocale(): Locale {
  const stored = window.localStorage.getItem(storageKey)
  if (stored && (supportedLocales as readonly string[]).includes(stored)) {
    return stored as Locale
  }
  const navLocale = window.navigator.language
  if (navLocale === 'zh-TW' || navLocale.startsWith('zh-TW')) return 'zh-TW'
  if (navLocale === 'zh-CN' || navLocale.startsWith('zh-CN')) return 'zh-CN'
  if (navLocale.startsWith('zh')) return 'zh-CN'
  return defaultLocale
}

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(detectInitialLocale)

  useEffect(() => {
    document.documentElement.lang = locale
  }, [locale])

  const setLocale = useCallback((value: Locale) => {
    setLocaleState(value)
    window.localStorage.setItem(storageKey, value)
    document.documentElement.lang = value
  }, [])

  const t = useCallback(
    (key: TKey, params?: Record<string, string | number>) =>
      translate(locale, key, params),
    [locale],
  )

  const value = useMemo(
    () => ({ locale, setLocale, t }),
    [locale, setLocale, t],
  )

  return <I18nContext value={value}>{children}</I18nContext>
}
