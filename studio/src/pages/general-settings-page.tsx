import { useI18n, localeLabels, supportedLocales } from '@/app/i18n'
import type { SendShortcut } from '@/app/preferences-context'
import { usePreferences } from '@/app/preferences-context-value'
import { SettingsLayout } from '@/components/settings/settings-layout'
import {
  SettingRow,
  SettingSection,
} from '@/components/settings/setting-controls'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { useTheme, type ThemePreference } from '@/app/theme-context'

export function GeneralSettingsPage() {
  const { t, locale, setLocale } = useI18n()
  const { preference, setPreference } = useTheme()
  const {
    expandReasoning,
    expandToolCalls,
    notifyOnCompletion,
    sendShortcut,
    setPreference: setPref,
  } = usePreferences()

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

  return (
    <SettingsLayout active="general">
      <div className="space-y-6">
        <SettingSection title={t('settings.general.appearance.title')}>
          <SettingRow
            htmlFor="theme-select"
            label={t('settings.general.appearance.theme')}
          >
            <Select
              onValueChange={(value) => setPreference(value as ThemePreference)}
              value={preference}
            >
              <SelectTrigger className="h-8 w-32 text-xs" id="theme-select">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="system">
                  {t('settings.general.appearance.theme.system')}
                </SelectItem>
                <SelectItem value="light">
                  {t('settings.general.appearance.theme.light')}
                </SelectItem>
                <SelectItem value="dark">
                  {t('settings.general.appearance.theme.dark')}
                </SelectItem>
              </SelectContent>
            </Select>
          </SettingRow>
          <SettingRow
            htmlFor="language-select"
            label={t('settings.general.appearance.language')}
          >
            <Select
              onValueChange={(value) => setLocale(value as typeof locale)}
              value={locale}
            >
              <SelectTrigger className="h-8 w-40 text-xs" id="language-select">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {supportedLocales.map((value) => (
                  <SelectItem key={value} value={value}>
                    {localeLabels[value]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingRow>
        </SettingSection>

        <SettingSection title={t('settings.general.conversation.title')}>
          <SettingRow
            description={t(
              'settings.general.conversation.expandReasoning.description',
            )}
            htmlFor="expand-reasoning"
            label={t('settings.general.conversation.expandReasoning')}
          >
            <Switch
              checked={expandReasoning}
              id="expand-reasoning"
              onCheckedChange={(checked) =>
                setPref('expandReasoning', Boolean(checked))
              }
            />
          </SettingRow>
          <SettingRow
            description={t(
              'settings.general.conversation.expandToolCalls.description',
            )}
            htmlFor="expand-tool-calls"
            label={t('settings.general.conversation.expandToolCalls')}
          >
            <Switch
              checked={expandToolCalls}
              id="expand-tool-calls"
              onCheckedChange={(checked) =>
                setPref('expandToolCalls', Boolean(checked))
              }
            />
          </SettingRow>
          <SettingRow
            htmlFor="send-shortcut"
            label={t('settings.general.conversation.sendShortcut')}
          >
            <Select
              onValueChange={(value) =>
                setPref('sendShortcut', value as SendShortcut)
              }
              value={sendShortcut}
            >
              <SelectTrigger className="h-8 w-40 text-xs" id="send-shortcut">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {shortcutOptions.map((option) => (
                  <SelectItem key={option.shortcut} value={option.shortcut}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </SettingRow>
          <SettingRow
            description={t(
              'settings.general.conversation.notifyOnCompletion.description',
            )}
            htmlFor="notify-on-completion"
            label={t('settings.general.conversation.notifyOnCompletion')}
          >
            <Switch
              checked={notifyOnCompletion}
              id="notify-on-completion"
              onCheckedChange={(checked) =>
                setPref('notifyOnCompletion', Boolean(checked))
              }
            />
          </SettingRow>
        </SettingSection>
      </div>
    </SettingsLayout>
  )
}
