export function SettingRow({
  description,
  htmlFor,
  label,
  children,
}: {
  description?: string
  htmlFor: string
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="flex items-center justify-between gap-4 py-3">
      <div className="min-w-0">
        <label
          className="text-sm font-medium text-foreground"
          htmlFor={htmlFor}
        >
          {label}
        </label>
        {description && (
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {description}
          </p>
        )}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  )
}

export function SettingSection({
  children,
  title,
}: {
  children: React.ReactNode
  title: string
}) {
  return (
    <section className="rounded-lg border border-border p-5">
      <h2 className="text-sm font-semibold text-foreground">{title}</h2>
      <div className="mt-3 divide-y divide-border">{children}</div>
    </section>
  )
}
