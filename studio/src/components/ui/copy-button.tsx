import { Copy } from 'lucide-react'
import { useCallback, useState } from 'react'

import { useI18n } from '@/app/i18n'
import { Button } from '@/components/ui/button'

export function CopyButton({ text }: { text: string }) {
  const { t } = useI18n()
  const [copied, setCopied] = useState(false)

  const handleCopy = useCallback(() => {
    if (!text) return
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }, [text])

  return (
    <Button
      aria-label={t('session.turn.copyResponse')}
      disabled={!text}
      onClick={handleCopy}
      size="icon"
      title={t('session.turn.copyResponse')}
      variant="ghost"
    >
      {copied ? (
        <span className="text-[10px] font-medium text-green-400">
          {t('session.turn.copied')}
        </span>
      ) : (
        <Copy className="size-3.5" />
      )}
    </Button>
  )
}
