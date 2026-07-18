import { ProviderType } from '@/gen/koda/v1/service_pb'

export const providerTypeLabels: Record<ProviderType, string> = {
  [ProviderType.UNSPECIFIED]: 'Select an API',
  [ProviderType.ANTHROPIC]: 'Anthropic Messages',
  [ProviderType.OPENAI_CHAT_COMPLETIONS]: 'OpenAI Chat Completions',
  [ProviderType.GEMINI]: 'Gemini',
  [ProviderType.DEEPSEEK]: 'DeepSeek',
  [ProviderType.OPENAI_RESPONSES]: 'OpenAI Responses',
}

export const editableProviderTypes = [
  ProviderType.ANTHROPIC,
  ProviderType.OPENAI_CHAT_COMPLETIONS,
  ProviderType.OPENAI_RESPONSES,
  ProviderType.GEMINI,
  ProviderType.DEEPSEEK,
]

const tokenFormatter = new Intl.NumberFormat('en-US')
const maxInt64 = 9223372036854775807n

export function parseContextWindowTokens(input: string): bigint | null {
  const value = input.trim()
  if (!value) return 0n
  if (!/^[1-9]\d*$/.test(value)) return null
  const tokens = BigInt(value)
  return tokens <= maxInt64 ? tokens : null
}

export function formatContextWindowTokens(tokens: bigint): string {
  return tokenFormatter.format(tokens)
}
