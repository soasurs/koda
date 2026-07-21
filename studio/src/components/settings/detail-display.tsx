export function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {label}
      </dt>
      <dd className="mt-1 break-words text-foreground">{value}</dd>
    </div>
  )
}

export function DetailList({
  label,
  values,
}: {
  label: string
  values: string[]
}) {
  return (
    <section className="mt-6">
      <h4 className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {label}
      </h4>
      <div className="mt-2 flex flex-wrap gap-2">
        {values.map((value) => (
          <code
            className="rounded border border-border bg-muted px-2 py-1 text-xs text-muted-foreground"
            key={value}
          >
            {value}
          </code>
        ))}
      </div>
    </section>
  )
}
