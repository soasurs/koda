import { Code, ConnectError } from '@connectrpc/connect'

export function retryDelay(signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = window.setTimeout(resolve, 250)
    signal.addEventListener(
      'abort',
      () => {
        window.clearTimeout(timer)
        resolve()
      },
      { once: true },
    )
  })
}

export function isRetriable(error: unknown): boolean {
  switch (ConnectError.from(error).code) {
    case Code.Unknown:
    case Code.Canceled:
    case Code.DeadlineExceeded:
    case Code.Aborted:
    case Code.Unavailable:
      return true
    default:
      return false
  }
}
