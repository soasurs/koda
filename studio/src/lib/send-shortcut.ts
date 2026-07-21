import type { SendShortcut } from '@/app/preferences-context'

export function matchesSendShortcut(
  event: React.KeyboardEvent<HTMLTextAreaElement>,
  shortcut: SendShortcut,
) {
  if (event.key !== 'Enter' || event.nativeEvent.isComposing) return false
  switch (shortcut) {
    case 'shift-enter':
      return event.shiftKey && !event.metaKey && !event.ctrlKey && !event.altKey
    case 'command-enter':
      return event.metaKey && !event.shiftKey && !event.ctrlKey && !event.altKey
    default:
      return (
        !event.shiftKey && !event.metaKey && !event.ctrlKey && !event.altKey
      )
  }
}
