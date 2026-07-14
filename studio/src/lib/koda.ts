import type {
  MCPServer,
  MCPServerSummary,
  Provider,
  Session,
  Skill,
  SkillSummary,
} from '@/gen/koda/v1/service_pb'
import { kodaClient } from '@/lib/connect'

export const kodaKeys = {
  sessions: ['sessions'] as const,
  session: (sessionId: string) => ['session', sessionId] as const,
  events: (sessionId: string) => ['events', sessionId] as const,
  providers: ['providers'] as const,
  models: (providerId: string) => ['models', providerId] as const,
  skills: ['skills'] as const,
  skill: (name: string) => ['skill', name] as const,
  mcpServers: ['mcp-servers'] as const,
  mcpServer: (id: string) => ['mcp-server', id] as const,
}

export async function listSessions(): Promise<Session[]> {
  const response = await kodaClient.listSessions({ limit: 200 })
  return response.sessions
}

export async function listProviders(): Promise<Provider[]> {
  const response = await kodaClient.listProviders({})
  return response.providers
}

export async function listSkills(): Promise<SkillSummary[]> {
  const response = await kodaClient.listSkills({})
  return response.skills
}

export async function getSkill(name: string): Promise<Skill> {
  const response = await kodaClient.getSkill({ name })
  if (!response.skill) {
    throw new Error(`Koda returned no definition for skill ${name}`)
  }
  return response.skill
}

export async function listMCPServers(): Promise<MCPServerSummary[]> {
  const response = await kodaClient.listMCPServers({})
  return response.servers
}

export async function getMCPServer(id: string): Promise<MCPServer> {
  const response = await kodaClient.getMCPServer({ id })
  if (!response.server) {
    throw new Error(`Koda returned no definition for MCP server ${id}`)
  }
  return response.server
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
