import { useI18n } from '@/app/i18n'
import type { ShellOutput } from '@/lib/shell-output'

export function ShellOutputView({ output }: { output: ShellOutput }) {
  const { t } = useI18n()
  return (
    <div className="mb-3 overflow-hidden rounded-md border border-border bg-background font-mono text-[11px]">
      <div className="flex items-center justify-between border-b border-border px-3 py-2 text-muted-foreground">
        <span>{t('session.tool.output.title')}</span>
        <span>
          {t('session.tool.output.exit', { code: output.exitCode })}
          {output.truncated ? t('session.tool.output.truncated') : ''}
        </span>
      </div>
      {output.stdout || output.stderr ? (
        <div className="max-h-72 overflow-auto p-3 leading-5">
          {output.stdout && (
            <pre className="whitespace-pre-wrap text-foreground">
              {output.stdout}
            </pre>
          )}
          {output.stderr && (
            <pre className="whitespace-pre-wrap text-red-400">
              {output.stderr}
            </pre>
          )}
        </div>
      ) : (
        <p className="px-3 py-2 text-muted-foreground">
          {t('session.tool.output.empty')}
        </p>
      )}
    </div>
  )
}
