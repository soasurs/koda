import { ChevronUp } from 'lucide-react'
import type { RefObject } from 'react'

import { useI18n } from '@/app/i18n'
import type { SendShortcut } from '@/app/preferences-context'
import { usePreferences } from '@/app/preferences-context-value'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

export function SendShortcutPicker({
  inputRef,
}: {
  inputRef: RefObject<HTMLTextAreaElement | null>
}) {
  const { t } = useI18n()
  const { sendShortcut, setPreference } = usePreferences()

  const shortcutOptions: { label: string; shortcut: SendShortcut }[] = [
    {
      label: t('settings.general.conversation.sendShortcut.enter'),
      shortcut: 'enter',
    },
    {
      label: t('settings.general.conversation.sendShortcut.shiftEnter'),
      shortcut: 'shift-enter',
    },
    {
      label: t('settings.general.conversation.sendShortcut.commandEnter'),
      shortcut: 'command-enter',
    },
  ]
  const sendShortcutLabel = shortcutOptions.find(
    (option) => option.shortcut === sendShortcut,
  )!.label

  function selectSendShortcut(shortcut: SendShortcut) {
    setPreference('sendShortcut', shortcut)
    requestAnimationFrame(() => inputRef.current?.focus())
  }

  return (
    <DropdownMenu
      onOpenChange={(open) => {
        if (!open) requestAnimationFrame(() => inputRef.current?.focus())
      }}
    >
      <DropdownMenuTrigger asChild>
        <Button
          aria-label={t('session.composer.chooseSendShortcut')}
          className="border-0 border-l border-primary-foreground/30 bg-transparent text-primary-foreground/70 hover:bg-primary/90 hover:text-primary-foreground rounded-none"
          size="icon"
          title={`${t('session.composer.chooseSendShortcut')}: ${sendShortcutLabel}`}
        >
          <ChevronUp className="size-3" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" side="top" sideOffset={8}>
        <DropdownMenuLabel>{t('session.composer.sendWith')}</DropdownMenuLabel>
        <DropdownMenuRadioGroup
          onValueChange={(value) => selectSendShortcut(value as SendShortcut)}
          value={sendShortcut}
        >
          {shortcutOptions.map((option) => (
            <DropdownMenuRadioItem
              key={option.shortcut}
              value={option.shortcut}
            >
              {option.label}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
