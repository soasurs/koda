import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, Folder, FolderOpen, LoaderCircle } from 'lucide-react'
import { useState } from 'react'

import { useI18n } from '@/app/i18n'
import { Button } from '@/components/ui/button'
import { Modal } from '@/components/ui/modal'
import { kodaClient } from '@/lib/connect'
import { errorMessage } from '@/lib/koda'

type DirectoryPickerProps = {
  initialPath?: string
  onClose: () => void
  onSelect: (path: string) => void
}

export function DirectoryPicker({
  initialPath = '',
  onClose,
  onSelect,
}: DirectoryPickerProps) {
  const { t } = useI18n()
  const [path, setPath] = useState(initialPath)
  const directoryQuery = useQuery({
    queryKey: ['directories', path],
    queryFn: () => kodaClient.listDirectories({ path }),
  })

  return (
    <Modal
      description={t('directory.picker.description')}
      onClose={onClose}
      title={t('directory.picker.title')}
      wide
    >
      <div className="border-b border-border px-5 py-3">
        <div className="flex items-center gap-2 rounded-md border border-input bg-background px-3 py-2">
          <FolderOpen className="size-4 shrink-0 text-muted-foreground" />
          <span className="min-w-0 truncate font-mono text-xs text-foreground">
            {directoryQuery.data?.path || path || t('directory.picker.home')}
          </span>
        </div>
      </div>

      <div className="min-h-72 px-3 py-3">
        {directoryQuery.isPending ? (
          <div className="flex h-64 items-center justify-center text-muted-foreground">
            <LoaderCircle className="size-5 animate-spin" />
          </div>
        ) : directoryQuery.isError ? (
          <div className="error-box m-2">
            {errorMessage(directoryQuery.error)}
          </div>
        ) : (
          <div className="space-y-1">
            {directoryQuery.data.parentPath && (
              <Button
                className="h-auto w-full justify-start px-3 py-2.5"
                onClick={() => setPath(directoryQuery.data.parentPath)}
                variant="ghost"
              >
                <ChevronLeft className="size-4" />
                {t('directory.picker.parent')}
              </Button>
            )}
            {directoryQuery.data.directories.map((directory) => (
              <Button
                className="h-auto w-full justify-start px-3 py-2.5"
                key={directory.path}
                onClick={() => setPath(directory.path)}
                variant="ghost"
              >
                <Folder className="size-4 shrink-0 text-muted-foreground" />
                <span className="truncate">{directory.name}</span>
              </Button>
            ))}
            {!directoryQuery.data.parentPath &&
              directoryQuery.data.directories.length === 0 && (
                <p className="px-3 py-10 text-center text-sm text-muted-foreground">
                  {t('directory.picker.noChildren')}
                </p>
              )}
          </div>
        )}
      </div>

      <footer className="flex justify-end gap-2 border-t border-border px-5 py-4">
        <Button onClick={onClose} variant="outline">
          {t('directory.picker.cancel')}
        </Button>
        <Button
          disabled={!directoryQuery.data}
          onClick={() => {
            if (directoryQuery.data) onSelect(directoryQuery.data.path)
          }}
        >
          {t('directory.picker.select')}
        </Button>
      </footer>
    </Modal>
  )
}
