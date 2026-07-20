export type SendShortcut = 'enter' | 'shift-enter' | 'command-enter'

export type Preferences = {
  expandReasoning: boolean
  expandToolCalls: boolean
  sendShortcut: SendShortcut
}

export type PreferencesContextValue = Preferences & {
  setPreference: <K extends keyof Preferences>(
    key: K,
    value: Preferences[K],
  ) => void
}

export const defaultPreferences: Preferences = {
  expandReasoning: false,
  expandToolCalls: false,
  sendShortcut: 'enter',
}
