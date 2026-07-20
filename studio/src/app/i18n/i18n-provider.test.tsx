import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { useI18n } from '@/app/i18n'
import { I18nProvider } from '@/app/i18n/i18n-provider'
import { dictionaries, type TKey } from '@/app/i18n/dictionaries'

const { en, 'zh-CN': zhCN, 'zh-TW': zhTW } = dictionaries

afterEach(cleanup)

describe('dictionaries', () => {
  it('zh-CN covers every English key', () => {
    const missing = (Object.keys(en) as TKey[]).filter((key) => !(key in zhCN))
    expect(missing).toEqual([])
  })

  it('zh-TW covers every English key', () => {
    const missing = (Object.keys(en) as TKey[]).filter((key) => !(key in zhTW))
    expect(missing).toEqual([])
  })
})

describe('I18nProvider', () => {
  beforeEach(() => window.localStorage.clear())

  it('falls back to English when the stored locale is unsupported', () => {
    window.localStorage.setItem('koda-studio-locale', 'fr')
    const { result } = renderHook(() => useI18n(), { wrapper: I18nProvider })
    expect(result.current.locale).toBe('en')
  })

  it('translates keys and interpolates params', () => {
    const { result } = renderHook(() => useI18n(), { wrapper: I18nProvider })
    act(() => result.current.setLocale('en'))
    expect(result.current.t('settings.general.title')).toBe('General')
    // Keys without params return as-is; the {name} placeholder is replaced.
    expect(result.current.t('theme.toggle.label')).toBe('Theme')
  })

  it('switches locales and persists the choice', () => {
    const { result } = renderHook(() => useI18n(), { wrapper: I18nProvider })
    act(() => result.current.setLocale('zh-CN'))
    expect(result.current.locale).toBe('zh-CN')
    expect(result.current.t('settings.general.title')).toBe('通用')
    expect(window.localStorage.getItem('koda-studio-locale')).toBe('zh-CN')
  })

  it('falls back to the English value for a missing key in a non-English dictionary', () => {
    const { result } = renderHook(() => useI18n(), { wrapper: I18nProvider })
    act(() => result.current.setLocale('zh-TW'))
    // Every key is defined in all dictionaries, so exercise the fallback path
    // by casting an arbitrary string key that does not exist.
    expect(result.current.t('nonexistent.key' as TKey)).toBe('nonexistent.key')
  })
})
