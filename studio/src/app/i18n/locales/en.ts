export const en = {
  'settings.general.title': 'General',
  'settings.general.description':
    'Manage appearance and conversation defaults for Koda Studio.',
  'settings.general.appearance.title': 'Appearance',
  'settings.general.appearance.theme': 'Theme',
  'settings.general.appearance.theme.system': 'System',
  'settings.general.appearance.theme.light': 'Light',
  'settings.general.appearance.theme.dark': 'Dark',
  'settings.general.appearance.language': 'Language',
  'settings.general.conversation.title': 'Conversation',
  'settings.general.conversation.expandReasoning':
    'Expand reasoning by default',
  'settings.general.conversation.expandReasoning.description':
    'Show the assistant reasoning panel expanded when a turn starts.',
  'settings.general.conversation.expandToolCalls':
    'Expand tool calls by default',
  'settings.general.conversation.expandToolCalls.description':
    'Show the tool call group expanded when tool calls run.',
  'settings.general.conversation.sendShortcut': 'Send shortcut',
  'settings.general.conversation.sendShortcut.enter': 'Enter',
  'settings.general.conversation.sendShortcut.shiftEnter': 'Shift + Enter',
  'settings.general.conversation.sendShortcut.commandEnter': 'Command + Enter',
  // Keys used by General page infrastructure only; remaining strings are migrated
  // in subsequent commits.
  'theme.toggle.label': 'Theme',
  'theme.toggle.ariaLabel': 'Theme',
} satisfies Record<string, string>

export type EnDictionary = Record<keyof typeof en, string>
