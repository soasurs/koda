import { en } from '@/app/i18n/locales/en'
import { zhCN } from '@/app/i18n/locales/zh-CN'
import { zhTW } from '@/app/i18n/locales/zh-TW'

export type Locale = 'en' | 'zh-CN' | 'zh-TW'

export const dictionaries = {
  en,
  'zh-CN': zhCN,
  'zh-TW': zhTW,
} as const

export type TKey = keyof typeof en

export const defaultLocale: Locale = 'en'

export const supportedLocales: Locale[] = ['en', 'zh-CN', 'zh-TW']

export const localeLabels: Record<Locale, string> = {
  en: 'English',
  'zh-CN': '中文(简体)',
  'zh-TW': '中文(繁體)',
}
export function translate(
  locale: Locale,
  key: TKey,
  params?: Record<string, string | number>,
): string {
  const dict = dictionaries[locale] ?? dictionaries[defaultLocale]
  let value = dict[key] ?? dictionaries[defaultLocale][key] ?? String(key)
  if (params) {
    for (const [name, replacement] of Object.entries(params)) {
      value = value.replace(
        new RegExp(`\\{${name}\\}`, 'g'),
        String(replacement),
      )
    }
  }
  return value
}
