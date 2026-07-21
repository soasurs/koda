export function EventTime({
  className = '',
  timestamp,
}: {
  className?: string
  timestamp: bigint
}) {
  if (timestamp <= 0n) return null

  const date = new Date(Number(timestamp))
  const now = new Date()
  const sameYear = date.getFullYear() === now.getFullYear()
  const sameDay =
    sameYear &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate()
  const label = new Intl.DateTimeFormat(undefined, {
    ...(sameDay
      ? {}
      : {
          month: 'numeric',
          day: 'numeric',
          ...(sameYear ? {} : { year: 'numeric' }),
        }),
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
  const title = new Intl.DateTimeFormat(undefined, {
    dateStyle: 'full',
    timeStyle: 'medium',
  }).format(date)

  return (
    <time
      className={`text-[11px] leading-4 text-muted-foreground ${className}`}
      dateTime={date.toISOString()}
      title={title}
    >
      {label}
    </time>
  )
}
