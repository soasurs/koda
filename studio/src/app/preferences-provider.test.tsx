import { act, cleanup, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'

import { usePreferences } from '@/app/preferences-context-value'
import { PreferencesProvider } from '@/app/preferences-provider'
import { defaultPreferences } from '@/app/preferences-context'

afterEach(cleanup)

describe('PreferencesProvider', () => {
  beforeEach(() => window.localStorage.clear())

  it('provides defaults when nothing is stored', () => {
    const { result } = renderHook(() => usePreferences(), {
      wrapper: PreferencesProvider,
    })
    expect(result.current.expandReasoning).toBe(
      defaultPreferences.expandReasoning,
    )
    expect(result.current.expandToolCalls).toBe(
      defaultPreferences.expandToolCalls,
    )
    expect(result.current.sendShortcut).toBe(defaultPreferences.sendShortcut)
  })

  it('persists a preference change', () => {
    const { result } = renderHook(() => usePreferences(), {
      wrapper: PreferencesProvider,
    })
    act(() => result.current.setPreference('expandReasoning', true))
    act(() => result.current.setPreference('sendShortcut', 'command-enter'))

    const stored = JSON.parse(
      window.localStorage.getItem('koda-studio-preferences') ?? '{}',
    ) as Record<string, unknown>
    expect(stored.expandReasoning).toBe(true)
    expect(stored.sendShortcut).toBe('command-enter')
  })

  it('migrates the legacy send-shortcut key on first load', () => {
    window.localStorage.setItem('koda-studio-send-shortcut', 'shift-enter')
    const { result } = renderHook(() => usePreferences(), {
      wrapper: PreferencesProvider,
    })
    expect(result.current.sendShortcut).toBe('shift-enter')
    expect(window.localStorage.getItem('koda-studio-send-shortcut')).toBeNull()
    const stored = JSON.parse(
      window.localStorage.getItem('koda-studio-preferences') ?? '{}',
    ) as Record<string, unknown>
    expect(stored.sendShortcut).toBe('shift-enter')
  })

  it('ignores the legacy key once the new key exists', () => {
    window.localStorage.setItem(
      'koda-studio-preferences',
      JSON.stringify({ ...defaultPreferences, sendShortcut: 'enter' }),
    )
    window.localStorage.setItem('koda-studio-send-shortcut', 'shift-enter')
    const { result } = renderHook(() => usePreferences(), {
      wrapper: PreferencesProvider,
    })
    expect(result.current.sendShortcut).toBe('enter')
  })
})
