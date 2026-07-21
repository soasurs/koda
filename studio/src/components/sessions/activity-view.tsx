import { EventView, ReasoningView } from '@/components/sessions/session-message'
import { ToolGroup } from '@/components/sessions/tool-activity'
import type { groupTurnActivities } from '@/lib/session-turns'

type TurnActivity = ReturnType<typeof groupTurnActivities>[number]

export function ActivityView({ activity }: { activity: TurnActivity }) {
  return (
    <div className="space-y-3">
      <ReasoningView reasoning={activity.assistant.message?.reasoning} />
      <EventView event={activity.assistant} />
      <ToolGroup assistant={activity.assistant} toolEvents={activity.tools} />
    </div>
  )
}
