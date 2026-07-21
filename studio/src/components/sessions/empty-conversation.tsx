import { Sparkles } from 'lucide-react'

import { useI18n } from '@/app/i18n'

export function EmptyConversation() {
  const { t } = useI18n()
  return (
    <div className="py-20 text-center">
      <div className="mx-auto flex size-11 items-center justify-center rounded-xl border border-border bg-muted">
        <Sparkles className="size-5 text-muted-foreground" />
      </div>
      <h2 className="mt-4 text-sm font-medium">{t('session.empty.title')}</h2>
      <p className="mt-2 text-sm text-muted-foreground">
        {t('session.empty.body')}
      </p>
    </div>
  )
}
