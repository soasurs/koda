import { LoaderCircle } from 'lucide-react'

export function CenteredLoader() {
  return (
    <div className="flex h-56 items-center justify-center">
      <LoaderCircle className="size-5 animate-spin text-muted-foreground" />
    </div>
  )
}
