import { useI18n, localeLabels, supportedLocales } from '@/app/i18n'
import type { SendShortcut } from '@/app/preferences-context'
import { usePreferences } from '@/app/preferences-context-value'
import { SettingsLayout } from '@/components/settings/settings-layout'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { useTheme, type ThemePreference } from '@/app/theme-context'

function SettingRow({
  description,
  htmlFor,
  label,
  children,
}: {
  description?: string
  htmlFor: string
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="flex items-center justify-between gap-4 py-3">
      <div className="min-w-0">
        <Label
          className="text-sm font-medium text-foreground"
          htmlFor={htmlFor}
        >
          {label}
        </Label>
        {description && (
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {description}
          </p>
        )}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  )
}

function SettingSection({
  children,
  title,
}: {
  children: React.ReactNode
  title: string
}) {
  return (
    <section className="rounded-lg border border-border p-5">
      <h2 className="text-sm font-semibold text-foreground">{title}</h2>
      <div className="mt-3 divide-y divide-border">{children}</div>
    </section>
  )
}

export function GeneralSettingsPage() {
  const { t, locale, setLocale } = useI18n()
  const { preference, setPreference } = useTheme()
  const {
    expandReasoning,
    expandToolCalls,
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
        </SettingSection>
      </div>
    </SettingsLayout>
  )
}
