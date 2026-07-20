import type { MCPServerSummary } from '@/gen/koda/v1/service_pb'
import { getMCPServer, listMCPServers } from '@/lib/koda'

export type MCPToolMeta = { serverName: string; originalName: string }

let cache = new Map<string, MCPToolMeta>()
let ready = false

/** Populate the cache from the current MCP server configuration. */
export async function refreshMCPToolCache(): Promise<void> {
  try {
    const servers: MCPServerSummary[] = await listMCPServers()
    const next = new Map<string, MCPToolMeta>()
    for (const summary of servers) {
      try {
        const server = await getMCPServer(summary.id)
        for (const tool of server.tools) {
          next.set(tool.name, {
            serverName: server.name,
            originalName: tool.originalName,
          })
        }
      } catch {
        // Skip servers that fail to load; stale entries are not carried over.
      }
    }
    cache = next
    ready = true
  } catch {
    // If listing fails, leave the cache as-is.
  }
}

/** Look up display metadata for one MCP tool, or undefined on miss. */
export function lookupMCPTool(namespacedName: string): MCPToolMeta | undefined {
  return cache.get(namespacedName)
}

/** Whether the cache has been populated at least once. */
export function isMCPToolCacheReady(): boolean {
  return ready
}
