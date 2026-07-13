import type { Provider, Session } from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'

export const kodaKeys = {
  sessions: ['sessions'] as const,
  session: (sessionId: string) => ['session', sessionId] as const,
  events: (sessionId: string) => ['events', sessionId] as const,
  providers: ['providers'] as const,
  models: (providerId: string) => ['models', providerId] as const,
}

export async function listSessions(): Promise<Session[]> {
  const response = await kodaClient.listSessions({ limit: 200 })
  return response.sessions
}

export async function listProviders(): Promise<Provider[]> {
  const response = await kodaClient.listProviders({})
  return response.providers
}

export function replaceSession(
  sessions: Session[] | undefined,
  session: Session,
): Session[] | undefined {
  return sessions?.map((current) =>
    current.id === session.id ? session : current,
  )
}

export function errorMessage(error: unknown): string {
  if (error instanceof Error) {
    if (/failed to fetch/i.test(error.message)) {
      return 'Cannot reach Koda. Make sure the local service is running.'
    }
    return error.message
  }
  return 'Koda returned an unexpected error'
}
