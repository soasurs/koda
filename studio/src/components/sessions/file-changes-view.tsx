import type { FileChange } from '@/gen/koda/v1/service_pb'
import { DiffLineKind } from '@/gen/koda/v1/service_pb'
import { useI18n } from '@/app/i18n'

export function FileChangesView({ changes }: { changes: FileChange[] }) {
  const { t } = useI18n()
  return (
    <div className="mb-3 space-y-3 overflow-hidden rounded-md border border-border bg-background">
      {changes.map((change, changeIndex) => (
        <div key={`${change.path}-${changeIndex}`}>
          <div className="flex items-center justify-between border-b border-border px-3 py-2 font-mono text-[11px] text-muted-foreground">
            <span className="truncate">{change.path}</span>
            {change.truncated && (
              <span className="shrink-0 text-muted-foreground">
                {t('session.tool.diff.truncated')}
              </span>
            )}
          </div>
          <div className="overflow-x-auto py-1 font-mono text-[11px] leading-5">
            {change.hunks.map((hunk, hunkIndex) => (
              <div key={`${hunk.oldStart}-${hunk.newStart}-${hunkIndex}`}>
                <div className="diff-hunk">
                  @@ -{hunk.oldStart} +{hunk.newStart} @@
                </div>
                {hunk.lines.map((line, lineIndex) => {
                  const added = line.kind === DiffLineKind.ADDED
                  const removed = line.kind === DiffLineKind.REMOVED
                  return (
                    <div
                      className={`diff-line ${
                        added
                          ? 'diff-line-added'
                          : removed
                            ? 'diff-line-removed'
                            : ''
                      }`}
                      key={`${line.oldLine}-${line.newLine}-${lineIndex}`}
                    >
                      <span className="diff-line-number">
                        {line.oldLine || ''}
                      </span>
                      <span className="diff-line-number">
                        {line.newLine || ''}
                      </span>
                      <span className="diff-line-code">
                        <span className="diff-marker">
                          {added ? '+' : removed ? '-' : ' '}
                        </span>
                        {line.content}
                      </span>
                    </div>
                  )
                })}
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
