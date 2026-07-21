import { ChevronRight } from 'lucide-react'

import { useI18n } from '@/app/i18n'
import { ActivityView } from '@/components/sessions/activity-view'
import type { groupTurnActivities } from '@/lib/session-turns'

type TurnActivity = ReturnType<typeof groupTurnActivities>[number]

export function EarlierActivityDetails({
  activities,
}: {
  activities: TurnActivity[]
}) {
  const { t } = useI18n()
  return (
    <details className="group/earlier-activity">
      <summary className="ml-9 flex w-fit cursor-pointer list-none items-center gap-1.5 text-xs font-medium text-muted-foreground hover:text-foreground">
        <ChevronRight className="size-3 transition-transform group-open/earlier-activity:rotate-90" />
        {t('session.turn.earlierActivity')}
      </summary>
      <div className="mt-4 space-y-4">
        {activities.map((activity, index) => (
          <ActivityView
            activity={activity}
            key={activity.assistant.id || `earlier-activity-${index}`}
          />
        ))}
      </div>
    </details>
  )
}
