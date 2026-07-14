import { useQuery } from '@tanstack/react-query'
import { ChevronLeft, Folder, FolderOpen, LoaderCircle } from 'lucide-react'
import { useState } from 'react'

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
  const [path, setPath] = useState(initialPath)
  const directoryQuery = useQuery({
    queryKey: ['directories', path],
    queryFn: () => kodaClient.listDirectories({ path }),
  })

  return (
    <Modal
      description="Browse directories on the machine running Koda."
      onClose={onClose}
      title="Choose workspace"
      wide
    >
      <div className="border-b border-border px-5 py-3">
        <div className="flex items-center gap-2 rounded-md border border-input bg-background px-3 py-2">
          <FolderOpen className="size-4 shrink-0 text-muted-foreground" />
          <span className="min-w-0 truncate font-mono text-xs text-foreground">
            {directoryQuery.data?.path || path || 'Home'}
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
              <button
                className="flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                onClick={() => setPath(directoryQuery.data.parentPath)}
                type="button"
              >
                <ChevronLeft className="size-4" />
                Parent directory
              </button>
            )}
            {directoryQuery.data.directories.map((directory) => (
              <button
                className="flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm text-foreground hover:bg-accent hover:text-accent-foreground"
                key={directory.path}
                onClick={() => setPath(directory.path)}
                type="button"
              >
                <Folder className="size-4 shrink-0 text-muted-foreground" />
                <span className="truncate">{directory.name}</span>
              </button>
            ))}
            {!directoryQuery.data.parentPath &&
              directoryQuery.data.directories.length === 0 && (
                <p className="px-3 py-10 text-center text-sm text-muted-foreground">
                  No child directories
                </p>
              )}
          </div>
        )}
      </div>

      <footer className="flex justify-end gap-2 border-t border-border px-5 py-4">
        <button className="button-secondary" onClick={onClose} type="button">
          Cancel
        </button>
        <button
          className="button-primary"
          disabled={!directoryQuery.data}
          onClick={() => {
            if (directoryQuery.data) onSelect(directoryQuery.data.path)
          }}
          type="button"
        >
          Select this directory
        </button>
      </footer>
    </Modal>
  )
}
