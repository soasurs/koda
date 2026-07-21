import { ClipboardList, ChevronUp, Hammer } from 'lucide-react'

import { useI18n } from '@/app/i18n'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { AgentMode } from '@/gen/koda/v1/service_pb'

export function ModeSelector({
  disabled,
  mode,
  onModeChange,
}: {
  disabled: boolean
  mode: AgentMode
  onModeChange: (mode: AgentMode) => void
}) {
  const { t } = useI18n()
  return (
    <div className="relative">
      <Select
        disabled={disabled}
        onValueChange={(value) => onModeChange(Number(value) as AgentMode)}
        value={String(mode)}
      >
        <SelectTrigger className="inline-flex h-auto w-auto items-center gap-1 whitespace-nowrap rounded-md border border-border bg-background py-1.5 pl-3 pr-7 text-xs font-medium text-foreground hover:border-border/80 [&>svg]:hidden">
          <SelectValue />
        </SelectTrigger>
        <ChevronUp className="pointer-events-none absolute right-2 top-1/2 size-3 -translate-y-1/2 text-muted-foreground" />
        <SelectContent side="top">
          <SelectItem value={String(AgentMode.BUILD)}>
            <span className="flex items-center gap-2">
              <Hammer className="size-4 shrink-0" />
              {t('session.composer.mode.build')}
            </span>
          </SelectItem>
          <SelectItem value={String(AgentMode.PLAN)}>
            <span className="flex items-center gap-2">
              <ClipboardList className="size-4 shrink-0" />
              {t('session.composer.mode.plan')}
            </span>
          </SelectItem>
        </SelectContent>
      </Select>
    </div>
  )
}
