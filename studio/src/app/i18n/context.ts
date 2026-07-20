import type { TKey, Locale } from '@/app/i18n/dictionaries'

export type I18nContextValue = {
  locale: Locale
  setLocale: (locale: Locale) => void
  t: (key: TKey, params?: Record<string, string | number>) => string
}

export type { TKey, Locale }
