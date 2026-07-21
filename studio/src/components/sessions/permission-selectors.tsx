import {
  Eye,
  FilePen,
  Folder,
  Globe,
  ShieldQuestion,
  Terminal,
  Zap,
} from 'lucide-react'

import { useI18n } from '@/app/i18n'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { FileAccess, ShellAccess } from '@/gen/koda/v1/service_pb'

export function PermissionSelectors({
  disabled,
  fileAccess,
  onFileAccessChange,
  shellAccess,
  onShellAccessChange,
}: {
  disabled: boolean
  fileAccess: FileAccess
  onFileAccessChange: (value: FileAccess) => void
  shellAccess: ShellAccess
  onShellAccessChange: (value: ShellAccess) => void
}) {
  const { t } = useI18n()

  return (
    <span className="relative inline-flex rounded-md border border-border">
      <Select
        disabled={disabled}
        onValueChange={(value) =>
          onFileAccessChange(Number(value) as FileAccess)
        }
        value={String(fileAccess)}
      >
        <SelectTrigger className="inline-flex h-auto w-auto items-center gap-1 whitespace-nowrap rounded-none rounded-l-md border-0 border-r border-border bg-background px-2.5 py-1.5 text-xs font-medium text-foreground hover:bg-muted/50 [&>svg:last-child]:rotate-180">
          <Folder className="size-3.5 shrink-0 text-muted-foreground" />
          <SelectValue />
        </SelectTrigger>
        <SelectContent side="top">
          <SelectItem value={String(FileAccess.WORKSPACE_READ)}>
            <span className="flex items-center gap-2">
              <Eye className="size-4 shrink-0" />
              {t('session.composer.fileAccess.read')}
            </span>
          </SelectItem>
          <SelectItem value={String(FileAccess.WORKSPACE_WRITE)}>
            <span className="flex items-center gap-2">
              <FilePen className="size-4 shrink-0" />
              {t('session.composer.fileAccess.write')}
            </span>
          </SelectItem>
          <SelectItem value={String(FileAccess.UNRESTRICTED)}>
            <span className="flex items-center gap-2">
              <Globe className="size-4 shrink-0" />
              {t('session.composer.fileAccess.full')}
            </span>
          </SelectItem>
        </SelectContent>
      </Select>
      <Select
        disabled={disabled}
        onValueChange={(value) =>
          onShellAccessChange(Number(value) as ShellAccess)
        }
        value={String(shellAccess)}
      >
        <SelectTrigger className="inline-flex h-auto w-auto items-center gap-1 whitespace-nowrap rounded-none rounded-r-md border-0 bg-background px-2.5 py-1.5 text-xs font-medium text-foreground hover:bg-muted/50 [&>svg:last-child]:rotate-180">
          <Terminal className="size-3.5 shrink-0 text-muted-foreground" />
          <SelectValue />
        </SelectTrigger>
        <SelectContent side="top">
          <SelectItem value={String(ShellAccess.APPROVAL_REQUIRED)}>
            <span className="flex items-center gap-2">
              <ShieldQuestion className="size-4 shrink-0" />
              {t('session.composer.shellAccess.ask')}
            </span>
          </SelectItem>
          <SelectItem value={String(ShellAccess.UNRESTRICTED)}>
            <span className="flex items-center gap-2">
              <Zap className="size-4 shrink-0" />
              {t('session.composer.shellAccess.free')}
            </span>
          </SelectItem>
        </SelectContent>
      </Select>
    </span>
  )
}
