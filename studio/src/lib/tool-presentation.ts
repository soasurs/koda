import type { TKey } from '@/app/i18n'
import { lookupMCPTool } from '@/lib/mcp-tools'

export function toolDetail(argumentsJson: string): string {
  try {
    const input = JSON.parse(argumentsJson) as Record<string, unknown>
    const value =
      input.name ??
      input.path ??
      input.url ??
      input.command ??
      input.pattern ??
      input.query ??
      input.globs
    if (Array.isArray(value)) return value.join(', ')
    if (typeof value === 'string') return value
    return ''
  } catch {
    return ''
  }
}

function mcpToolLabel(name: string): string {
  const meta = lookupMCPTool(name)
  if (meta)
    return `MCP ${meta.serverName} \u203A ${humanize(meta.originalName)}`
  const prefix = 'mcp__'
  if (!name.startsWith(prefix)) return ''
  const inner = name.slice(prefix.length)
  const sep = inner.indexOf('__')
  if (sep < 0) {
    return 'MCP ' + humanize(inner)
  }
  const server = humanize(inner.slice(0, sep))
  const tool = humanize(inner.slice(sep + 2))
  return `MCP ${server} \u203A ${tool}`
}

function humanize(value: string): string {
  return value
    .split(/[_-]+/)
    .filter(Boolean)
    .map((part) => part[0]?.toUpperCase() + part.slice(1))
    .join(' ')
}

export function toolLabel(
  t: (key: TKey, params?: Record<string, string | number>) => string,
  name: string,
  past = false,
): string {
  const presentKey = `tools.name.${name}` as TKey
  const pastKey = `tools.namePast.${name}` as TKey
  if (past) {
    const translated = t(pastKey)
    if (translated !== pastKey) return translated
  }
  const translated = t(presentKey)
  if (translated !== presentKey) return translated
  const mcpLabel = mcpToolLabel(name)
  if (mcpLabel) return mcpLabel
  return humanize(name)
}

export function toolCallPresentation(
  t: (key: TKey, params?: Record<string, string | number>) => string,
  toolCall: { name: string; argumentsJson: string },
  past = false,
): { label: string; detail: string } {
  return {
    label: toolLabel(t, toolCall.name, past),
    detail: toolDetail(toolCall.argumentsJson),
  }
}

export function toolPresentation(
  t: (key: TKey, params?: Record<string, string | number>) => string,
  name: string,
  argumentsJson: string,
  past = false,
): { label: string; detail: string } {
  return {
    label: toolLabel(t, name, past),
    detail: toolDetail(argumentsJson),
  }
}
