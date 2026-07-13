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
